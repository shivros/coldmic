package usecase

import (
	"context"
	"errors"
	"sync"
	"time"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

var ErrNoActiveSession = errors.New("no active recording session")

// Config controls tracer-bullet recording behavior.
type Config struct {
	Audio          ports.AudioConfig
	Streaming      ports.StreamingConfig
	ChunkSize      int
	StreamingGrace time.Duration
}

// SessionController orchestrates push-to-talk recording and transcription.
type SessionController struct {
	audio     ports.AudioCapture
	provider  ports.TranscriptionProvider
	events    ports.EventSink
	finalizer transcriptFinalizer
	cfg       Config

	mu      sync.Mutex
	current *activeSession
}

func NewSessionController(
	audio ports.AudioCapture,
	provider ports.TranscriptionProvider,
	rules ports.RulesEngine,
	clipboard ports.Clipboard,
	events ports.EventSink,
	cfg Config,
) *SessionController {
	if cfg.ChunkSize < 256 {
		cfg.ChunkSize = 4096
	}
	return &SessionController{
		audio:     audio,
		provider:  provider,
		events:    events,
		finalizer: newTranscriptFinalizer(rules, clipboard, events),
		cfg:       cfg,
	}
}

// Start begins a new capture/transcription session.
func (c *SessionController) Start(ctx context.Context) error {
	var previous *activeSession

	c.mu.Lock()
	if c.current != nil {
		previous = c.current
		c.current = nil
	}
	c.mu.Unlock()

	if previous != nil {
		c.stopSession(previous)
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	stream, err := c.provider.StartStreaming(sessionCtx, c.cfg.Streaming)
	if err != nil {
		cancel()
		return err
	}

	audioSession, err := c.audio.Start(sessionCtx, c.cfg.Audio)
	if err != nil {
		_ = stream.Close()
		cancel()
		return err
	}

	active := &activeSession{
		cancel:     cancel,
		audio:      audioSession,
		stream:     stream,
		state:      domain.SessionStateRecording,
		aggregator: newTranscriptAggregator(),
		eventsDone: make(chan struct{}),
		audioDone:  make(chan struct{}),
	}

	c.mu.Lock()
	c.current = active
	c.mu.Unlock()

	go consumeTranscriptionEvents(active.stream, active.aggregator, c.events, active.eventsDone)
	go pumpAudioChunks(active.audio, active.stream, c.cfg.ChunkSize, c.events, active.audioDone)

	reason := domain.SessionReasonRecordingStarted
	if previous != nil {
		reason = domain.SessionReasonRecordingRestarted
	}
	c.events.SessionStateChanged(domain.SessionStateRecording, reason)
	return nil
}

// Stop ends an active session and returns the final transcript.
func (c *SessionController) Stop(ctx context.Context) (domain.StopResult, error) {
	active, err := c.getCurrent()
	if err != nil {
		return domain.StopResult{}, err
	}

	active.setState(domain.SessionStateStopping)
	c.events.SessionStateChanged(domain.SessionStateStopping, domain.SessionReasonTranscribing)

	if err := active.audio.Stop(); err != nil {
		c.events.SessionError(domain.ErrorCodeAudioStop, "failed to stop audio capture cleanly")
	}

	if c.cfg.StreamingGrace > 0 {
		timer := time.NewTimer(c.cfg.StreamingGrace)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}

	_ = active.stream.CloseSend()
	streamErr := waitForStream(active.stream, 4*time.Second)
	<-active.eventsDone
	<-active.audioDone

	raw := active.aggregator.Raw()
	if raw == "" && streamErr != nil {
		c.events.SessionError(domain.ErrorCodeTranscription, streamErr.Error())
		c.finishSession(active, domain.SessionStateError, domain.SessionReasonTranscriptionFailed)
		return domain.StopResult{}, streamErr
	}
	if raw == "" {
		c.finishSession(active, domain.SessionStateIdle, domain.SessionReasonNoTranscript)
		return domain.StopResult{}, errors.New("no transcript captured")
	}

	result, reason, err := c.finalizer.Finalize(ctx, raw)
	if err != nil {
		c.finishSession(active, domain.SessionStateError, reason)
		return domain.StopResult{}, err
	}

	c.events.FinalTranscript(result.RawTranscript, result.FinalTranscript)
	c.finishSession(active, domain.SessionStateIdle, reason)
	return result, nil
}

// Abort cancels and discards an active session without transcription.
func (c *SessionController) Abort() error {
	active, err := c.getCurrent()
	if err != nil {
		return err
	}

	c.stopSession(active)
	c.finishSession(active, domain.SessionStateIdle, domain.SessionReasonRecordingDiscarded)
	return nil
}

// Status returns the current backend status.
func (c *SessionController) Status() domain.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return domain.Status{State: domain.SessionStateIdle, Active: false}
	}
	state := c.current.getState()
	return domain.Status{State: state, Active: state != domain.SessionStateIdle}
}

func (c *SessionController) getCurrent() (*activeSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return nil, ErrNoActiveSession
	}
	return c.current, nil
}

func (c *SessionController) stopSession(active *activeSession) {
	active.cancel()
	_ = active.audio.Stop()
	_ = active.stream.Close()
	<-active.eventsDone
	<-active.audioDone
}

func (c *SessionController) finishSession(active *activeSession, state domain.SessionState, reason domain.SessionStateReason) {
	active.cancel()
	active.setState(state)

	c.mu.Lock()
	if c.current == active {
		c.current = nil
	}
	c.mu.Unlock()

	c.events.SessionStateChanged(state, reason)
}
