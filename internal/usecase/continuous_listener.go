package usecase

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"coldmic/internal/audio"
	"coldmic/internal/debuglog"
	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

// ListenerEventKind classifies events from the continuous listener.
type ListenerEventKind int

const (
	ListenerEventWakePhrase ListenerEventKind = iota
	ListenerEventTranscript
	ListenerEventVADSpeechStart
	ListenerEventVADSpeechEnd
)

// ListenerEvent is emitted by the continuous listener during operation.
type ListenerEvent struct {
	Kind      ListenerEventKind
	Text      string
	SessionID string
	Timestamp time.Time
}

// ContinuousListenerConfig controls continuous listening behaviour.
type ContinuousListenerConfig struct {
	WakePhrases  []string
	VADThreshold float64
	SilenceMs    int
	FrameMs      int
	Audio        ports.AudioConfig
	Streaming    ports.StreamingConfig
	ChunkSize    int
}

type audioRead struct {
	data []byte
	err  error
}

// ContinuousListener orchestrates VAD-gated continuous listening.
// It captures microphone audio, feeds frames through a VAD, streams speech
// segments to the transcription provider, and emits wake-phrase-matched
// transcripts on its Events channel.
//
// When a LocalSTT provider is set, the listener uses two-phase transcription:
// speech audio is buffered and transcribed locally first. Only if a wake phrase
// is detected does the listener engage the cloud TranscriptionProvider.
// When LocalSTT is nil, the listener streams all speech to the cloud provider
// directly (original single-phase behavior).
type ContinuousListener struct {
	audio    ports.AudioCapture
	vad      audio.VAD
	provider ports.TranscriptionProvider
	localSTT ports.LocalSTT // optional: local STT for wake phrase pre-filtering
	events   ports.EventSink
	cfg      ContinuousListenerConfig

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc

	outCh  chan ListenerEvent
	nextID uint64
}

// NewContinuousListener creates a new continuous listener.
// localSTT is optional — when non-nil, enables two-phase transcription
// (local wake check before cloud STT). When nil, uses single-phase cloud STT.
func NewContinuousListener(
	audioCap ports.AudioCapture,
	vad audio.VAD,
	provider ports.TranscriptionProvider,
	localSTT ports.LocalSTT,
	events ports.EventSink,
	cfg ContinuousListenerConfig,
) *ContinuousListener {
	if cfg.ChunkSize < 256 {
		cfg.ChunkSize = 4096
	}
	if cfg.SilenceMs <= 0 {
		cfg.SilenceMs = 800
	}
	if cfg.FrameMs <= 0 {
		cfg.FrameMs = 30
	}
	return &ContinuousListener{
		audio:    audioCap,
		vad:      vad,
		provider: provider,
		localSTT: localSTT,
		events:   events,
		cfg:      cfg,
		outCh:    make(chan ListenerEvent, 32),
	}
}

// Events returns the channel on which listener events are emitted.
func (l *ContinuousListener) Events() <-chan ListenerEvent {
	return l.outCh
}

// Start begins continuous VAD-gated listening. Blocks until the context is
// cancelled or an audio/provider error occurs. Call in a goroutine.
func (l *ContinuousListener) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return domain.ErrContinuousActive
	}
	ctx, l.cancel = context.WithCancel(ctx)
	l.running = true
	l.mu.Unlock()

	defer func() {
		l.mu.Lock()
		l.running = false
		l.mu.Unlock()
	}()

	debuglog.Printf("continuous listener starting vad_threshold=%.1f silence_ms=%d frame_ms=%d wake_phrases=%v local_stt=%v",
		l.cfg.VADThreshold, l.cfg.SilenceMs, l.cfg.FrameMs, l.cfg.WakePhrases, l.localSTT != nil)

	// Start continuous audio capture.
	audioSession, err := l.audio.Start(ctx, l.cfg.Audio)
	if err != nil {
		debuglog.Printf("continuous listener audio start failed: %v", err)
		return fmt.Errorf("continuous listener: audio start: %w", err)
	}
	defer func() {
		_ = audioSession.Stop()
	}()

	debuglog.Printf("continuous listener audio capture started")

	l.events.SessionStateChanged(domain.SessionStateContinuous, domain.SessionReasonRecordingStarted)

	// Frame size in bytes: FrameMs ms × sample_rate × channels × 2 (16-bit).
	frameSamples := l.cfg.Audio.SampleRate * l.cfg.Audio.Channels * l.cfg.FrameMs / 1000
	frameBytes := frameSamples * 2
	if frameBytes < 256 {
		frameBytes = 256
	}

	silenceDuration := time.Duration(l.cfg.SilenceMs) * time.Millisecond
	chunkSize := l.cfg.ChunkSize

	// Async audio reader so we can select on ctx.Done().
	readCh := make(chan audioRead, 1)
	buf := make([]byte, chunkSize)
	go func() {
		for {
			n, err := audioSession.Read(buf)
			// Copy data since we reuse buf.
			var data []byte
			if n > 0 {
				data = make([]byte, n)
				copy(data, buf[:n])
			}
			readCh <- audioRead{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	// Read loop state.
	var (
		speechActive  bool
		silenceTimer  *time.Timer
		silenceCh     <-chan time.Time
		streamSession ports.StreamingSession
		aggregator    *transcriptAggregator
		eventsDone    chan struct{}
		audioBuf      []byte
		sessionID     string
	)

	nextSessionID := func() string {
		l.mu.Lock()
		l.nextID++
		id := fmt.Sprintf("continuous-%d", l.nextID)
		l.mu.Unlock()
		return id
	}

	// beginSpeech starts either buffering (two-phase) or streaming (single-phase).
	beginSpeech := func() {
		if speechActive {
			return
		}
		speechActive = true
		sessionID = nextSessionID()

		l.emit(ListenerEvent{
			Kind:      ListenerEventVADSpeechStart,
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
		})

		if l.localSTT != nil {
			// Two-phase: buffer audio locally, don't start cloud STT yet.
			// audioBuf was already populated by the read loop with the triggering chunk.
			debuglog.Printf("continuous listener: speech detected (two-phase), buffering session=%s", sessionID)
			return
		}

		// Single-phase: start cloud streaming immediately.
		debuglog.Printf("continuous listener: speech detected, starting transcription session=%s", sessionID)

		stream, err := l.provider.StartStreaming(ctx, l.cfg.Streaming)
		if err != nil {
			debuglog.Printf("continuous listener: provider start failed: %v", err)
			l.events.SessionError(domain.ErrorCodeTranscription, fmt.Sprintf("continuous: provider start: %v", err))
			speechActive = false
			return
		}
		streamSession = stream
		aggregator = newTranscriptAggregator()
		eventsDone = make(chan struct{})

		go consumeTranscriptionEvents(streamSession, aggregator, l.events, eventsDone)

		// Send buffered audio.
		if len(audioBuf) > 0 {
			_ = streamSession.SendAudio(audioBuf)
		}
	}

	// endSpeech handles end of speech segment.
	// For two-phase: runs local STT, checks wake phrase, optionally starts cloud STT.
	// For single-phase: closes the cloud stream and processes the transcript.
	// It runs in a goroutine to avoid blocking the read loop.
	endSpeech := func() {
		if !speechActive {
			return
		}
		speechActive = false

		if l.localSTT != nil {
			// Two-phase: transcribe locally first.
			// Snapshot mutable variables before launching goroutine (fix race).
			bufSnapshot := make([]byte, len(audioBuf))
			copy(bufSnapshot, audioBuf)
			sidSnapshot := sessionID
			go func() {
				debuglog.Printf("continuous listener: speech ended (two-phase), running local STT session=%s audio=%d bytes", sidSnapshot, len(bufSnapshot))

				localText, err := l.localSTT.Transcribe(ctx, bufSnapshot)
				if err != nil {
					debuglog.Printf("continuous listener: local STT failed: %v, falling back to cloud", err)
					// Fallback: start cloud session with buffered audio.
					l.startCloudFromBuffer(ctx, bufSnapshot, sidSnapshot)
					return
				}

				if localText == "" {
					debuglog.Printf("continuous listener: local STT empty session=%s, discarding", sidSnapshot)
					l.emit(ListenerEvent{
						Kind:      ListenerEventVADSpeechEnd,
						SessionID: sidSnapshot,
						Timestamp: time.Now().UTC(),
					})
					l.vad.Reset()
					return
				}

				debuglog.Printf("continuous listener: local STT transcript=%q session=%s", localText, sidSnapshot)

				// Check wake phrase on local transcript.
				if text, ok := l.matchWakePhrase(localText); ok {
					debuglog.Printf("continuous listener: wake phrase matched (local) session=%s text=%q", sidSnapshot, text)
					// Emit wake phrase event immediately (before cloud call).
					l.emit(ListenerEvent{
						Kind:      ListenerEventWakePhrase,
						Text:      text,
						SessionID: sidSnapshot,
						Timestamp: time.Now().UTC(),
					})
					// Start cloud session with buffered audio for high-quality transcription.
					l.startCloudFromBuffer(ctx, bufSnapshot, sidSnapshot)
				} else {
					debuglog.Printf("continuous listener: no wake phrase match (local) session=%s raw=%q", sidSnapshot, localText)
					// Discard — no cloud API call needed.
					l.emit(ListenerEvent{
						Kind:      ListenerEventVADSpeechEnd,
						SessionID: sidSnapshot,
						Timestamp: time.Now().UTC(),
					})
				}
				l.vad.Reset()
			}()
			return
		}

		// Single-phase: close the cloud stream.
		if streamSession == nil {
			return
		}

		debuglog.Printf("continuous listener: speech ended, closing transcription session=%s", sessionID)

		// Snapshot mutable variables before launching goroutine (fix race).
		snapStream := streamSession
		snapAgg := aggregator
		snapDone := eventsDone
		sidSnapshot := sessionID

		go func() {
			_ = snapStream.CloseSend()
			_ = waitForStream(snapStream, 4*time.Second)
			<-snapDone

			raw := snapAgg.Raw()
			if raw == "" {
				debuglog.Printf("continuous listener: no transcript for session=%s", sidSnapshot)
				return
			}

			// Check wake phrase.
			if text, ok := l.matchWakePhrase(raw); ok {
				debuglog.Printf("continuous listener: wake phrase matched session=%s text=%q", sidSnapshot, text)
				l.emit(ListenerEvent{
					Kind:      ListenerEventWakePhrase,
					Text:      text,
					SessionID: sidSnapshot,
					Timestamp: time.Now().UTC(),
				})
			} else {
				debuglog.Printf("continuous listener: no wake phrase match session=%s raw=%q", sidSnapshot, raw)
			}

			l.emit(ListenerEvent{
				Kind:      ListenerEventVADSpeechEnd,
				SessionID: sidSnapshot,
				Timestamp: time.Now().UTC(),
			})

			// Reset VAD for next utterance.
			l.vad.Reset()
		}()
	}

	for {
		select {
		case <-ctx.Done():
			if speechActive && streamSession != nil {
				_ = streamSession.Close()
			}
			debuglog.Printf("continuous listener stopped")
			return ctx.Err()
		case <-silenceCh:
			endSpeech()
			silenceTimer = nil
			silenceCh = nil
		case rd := <-readCh:
			if rd.err != nil {
				if rd.err != io.EOF {
					debuglog.Printf("continuous listener: audio read error: %v", rd.err)
					if speechActive {
						endSpeech()
					}
					return fmt.Errorf("continuous listener: audio read: %w", rd.err)
				}
				// EOF: audio session ended cleanly.
				if speechActive {
					endSpeech()
				}
				debuglog.Printf("continuous listener: audio session ended")
				return nil
			}

			chunk := rd.data

			// In two-phase mode, buffer audio instead of streaming to cloud.
			if l.localSTT != nil && speechActive {
				audioBuf = append(audioBuf, chunk...)
			}

			// Feed audio to active stream if we're in single-phase speech.
			if l.localSTT == nil && speechActive && streamSession != nil {
				_ = streamSession.SendAudio(chunk)
			}

			// Process VAD on frame-sized sub-chunks.
			for offset := 0; offset < len(chunk); offset += frameBytes {
				end := offset + frameBytes
				if end > len(chunk) {
					end = len(chunk)
				}
				frame := chunk[offset:end]

				prob, _ := l.vad.Process(frame)
				if prob > 0.5 {
					// Speech detected.
					if !speechActive {
						audioBuf = make([]byte, len(chunk))
						copy(audioBuf, chunk)
						beginSpeech()
					}
					// Reset silence timer.
					if silenceTimer != nil {
						silenceTimer.Stop()
					}
					silenceTimer = time.NewTimer(silenceDuration)
					silenceCh = silenceTimer.C
				}
			}
		}
	}
}

// Stop terminates continuous listening.
func (l *ContinuousListener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cancel != nil {
		l.cancel()
	}
}

// Running reports whether the listener is active.
func (l *ContinuousListener) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running
}

func (l *ContinuousListener) emit(evt ListenerEvent) {
	select {
	case l.outCh <- evt:
	default:
		debuglog.Printf("continuous listener: event channel full, dropping event kind=%d", evt.Kind)
	}
}

// matchWakePhrase checks if text starts with any configured wake phrase
// (case-insensitive). Returns the remaining text and true if matched.
func (l *ContinuousListener) matchWakePhrase(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, phrase := range l.cfg.WakePhrases {
		if strings.HasPrefix(lower, phrase) {
			// Use rune-aware slicing to handle multi-byte characters correctly.
			runes := []rune(text)
			phraseRunes := len([]rune(phrase))
			if phraseRunes > len(runes) {
				continue
			}
			remaining := strings.TrimSpace(string(runes[phraseRunes:]))
			// Strip leading comma or punctuation after wake phrase.
			remaining = strings.TrimPrefix(remaining, ",")
			remaining = strings.TrimSpace(remaining)
			return remaining, true
		}
	}
	return "", false
}

// startCloudFromBuffer starts a cloud streaming session and replays buffered
// audio, consuming the transcription result. Used in two-phase mode after local
// STT detects a wake phrase. Runs synchronously — caller should invoke from a goroutine.
func (l *ContinuousListener) startCloudFromBuffer(ctx context.Context, audioData []byte, sessionID string) {
	stream, err := l.provider.StartStreaming(ctx, l.cfg.Streaming)
	if err != nil {
		debuglog.Printf("continuous listener: cloud fallback start failed session=%s: %v", sessionID, err)
		l.events.SessionError(domain.ErrorCodeTranscription, fmt.Sprintf("continuous: cloud start: %v", err))
		return
	}
	defer stream.Close()

	aggregator := newTranscriptAggregator()
	eventsDone := make(chan struct{})
	go consumeTranscriptionEvents(stream, aggregator, l.events, eventsDone)

	debuglog.Printf("continuous listener: cloud session started session=%s, replaying %d bytes", sessionID, len(audioData))
	_ = stream.SendAudio(audioData)
	_ = stream.CloseSend()
	_ = waitForStream(stream, 4*time.Second)
	<-eventsDone

	raw := aggregator.Raw()
	if raw != "" {
		debuglog.Printf("continuous listener: cloud transcript session=%s text=%q", sessionID, raw)
	} else {
		debuglog.Printf("continuous listener: cloud transcript empty session=%s", sessionID)
	}

	l.emit(ListenerEvent{
		Kind:      ListenerEventVADSpeechEnd,
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
	})
}
