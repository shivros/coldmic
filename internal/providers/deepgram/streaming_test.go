package deepgram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

func TestNewProviderDefaults(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{})
	if p.cfg.APIBaseURL != "https://api.deepgram.com/v1" {
		t.Fatalf("unexpected base url: %q", p.cfg.APIBaseURL)
	}
	if p.cfg.Model != "nova-2" {
		t.Fatalf("unexpected model: %q", p.cfg.Model)
	}
}

func TestProviderStartStreamingRequiresAPIKey(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{APIKey: ""})
	_, err := p.StartStreaming(context.Background(), ports.StreamingConfig{})
	if err == nil {
		t.Fatalf("expected missing key error")
	}
}

func TestBuildListenURLDefaults(t *testing.T) {
	t.Parallel()

	url, err := buildListenURL(Config{APIBaseURL: "https://api.deepgram.com/v1", Model: "nova-2"}, ports.StreamingConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(url, "wss://api.deepgram.com/v1/listen") {
		t.Fatalf("unexpected ws url: %s", url)
	}
	if !strings.Contains(url, "encoding=linear16") {
		t.Fatalf("expected default encoding in url: %s", url)
	}
	if !strings.Contains(url, "sample_rate=16000") {
		t.Fatalf("expected default sample_rate in url: %s", url)
	}
	if !strings.Contains(url, "channels=1") {
		t.Fatalf("expected default channels in url: %s", url)
	}
}

func TestBuildListenURLWithLanguageAndSmartFormat(t *testing.T) {
	t.Parallel()

	url, err := buildListenURL(
		Config{APIBaseURL: "http://localhost:8080/v1", Model: "m", Language: "en-US", SmartFormat: true},
		ports.StreamingConfig{Encoding: "linear16", SampleRate: 8000, Channels: 2, InterimResults: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(url, "ws://localhost:8080/v1/listen") {
		t.Fatalf("unexpected ws url: %s", url)
	}
	if !strings.Contains(url, "language=en-US") {
		t.Fatalf("expected language in url: %s", url)
	}
	if !strings.Contains(url, "smart_format=true") {
		t.Fatalf("expected smart_format in url: %s", url)
	}
}

func TestBuildListenURLInvalidBase(t *testing.T) {
	t.Parallel()

	_, err := buildListenURL(Config{APIBaseURL: ":// bad"}, ports.StreamingConfig{})
	if err == nil {
		t.Fatalf("expected invalid base url error")
	}
}

func TestExtractTranscript(t *testing.T) {
	t.Parallel()

	r1 := deepgramResponse{}
	r1.Channel.Alternatives = append(r1.Channel.Alternatives, struct {
		Transcript string "json:\"transcript\""
	}{Transcript: " channel "})
	if got := extractTranscript(r1); got != "channel" {
		t.Fatalf("unexpected transcript from channel: %q", got)
	}

	r2 := deepgramResponse{}
	r2.Results.Channels = append(r2.Results.Channels, struct {
		Alternatives []struct {
			Transcript string "json:\"transcript\""
		} "json:\"alternatives\""
	}{
		Alternatives: []struct {
			Transcript string "json:\"transcript\""
		}{{Transcript: "results"}},
	})
	if got := extractTranscript(r2); got != "results" {
		t.Fatalf("unexpected transcript from results: %q", got)
	}

	if got := extractTranscript(deepgramResponse{}); got != "" {
		t.Fatalf("expected empty transcript, got %q", got)
	}
}

func TestStreamingSessionSendAudioClosed(t *testing.T) {
	t.Parallel()

	s := &streamingSession{sendClosed: true}
	if err := s.SendAudio([]byte("x")); err == nil {
		t.Fatalf("expected closed error")
	}
}

func TestStreamingSessionCloseSendIsIdempotent(t *testing.T) {
	t.Parallel()

	s := &streamingSession{audio: make(chan []byte, 1)}
	if err := s.CloseSend(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.CloseSend(); err != nil {
		t.Fatalf("unexpected second error: %v", err)
	}
}

func TestStreamingSessionSetErrIgnoresCloseErrors(t *testing.T) {
	t.Parallel()

	s := &streamingSession{}
	s.setErr(&websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "closed"})
	if s.waitErr() != nil {
		t.Fatalf("expected close error to be ignored")
	}

	s.setErr(errors.New("boom"))
	if s.waitErr() == nil || s.waitErr().Error() != "boom" {
		t.Fatalf("expected non-close error to be captured")
	}
}

func TestStreamingSessionSetErrIgnoresWrappedCloseErrors(t *testing.T) {
	t.Parallel()

	s := &streamingSession{}
	s.setErr(fmt.Errorf("failed to read provider event: %w", &websocket.CloseError{
		Code: websocket.CloseNormalClosure,
		Text: "closed",
	}))
	if s.waitErr() != nil {
		t.Fatalf("expected wrapped close error to be ignored")
	}
}

func TestStreamingSessionSetErrIgnoresClosedConnectionErrors(t *testing.T) {
	t.Parallel()

	testCases := []error{
		fmt.Errorf("failed to close stream: %w", net.ErrClosed),
		fmt.Errorf("failed to send audio: %w", websocket.ErrCloseSent),
	}

	for _, err := range testCases {
		s := &streamingSession{}
		s.setErr(err)
		if s.waitErr() != nil {
			t.Fatalf("expected %q to be ignored", err)
		}
	}
}

func TestStreamingSessionSetErrFirstWins(t *testing.T) {
	t.Parallel()

	s := &streamingSession{}
	s.setErr(errors.New("first"))
	s.setErr(errors.New("second"))
	if s.waitErr() == nil || s.waitErr().Error() != "first" {
		t.Fatalf("expected first error to win")
	}
}

func TestStreamingSessionEventsReturnsChannel(t *testing.T) {
	t.Parallel()

	ch := make(chan domain.TranscriptEvent, 1)
	s := &streamingSession{events: ch}
	if got := s.Events(); got != ch {
		t.Fatalf("expected same events channel")
	}
}

func TestStreamingSessionWaitReturnsStoredError(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	s := &streamingSession{done: done}
	s.setErr(errors.New("boom"))
	close(done)
	if err := s.Wait(); err == nil || err.Error() != "boom" {
		t.Fatalf("expected stored error, got %v", err)
	}
}

func TestStreamingSessionEmit(t *testing.T) {
	t.Parallel()

	t.Run("writes when channel has room", func(t *testing.T) {
		s := &streamingSession{events: make(chan domain.TranscriptEvent, 1), done: make(chan struct{})}
		event := domain.TranscriptEvent{Kind: domain.TranscriptKindPartial, Text: "hi"}
		s.emit(event)
		got := <-s.events
		if got.Text != "hi" || got.Kind != domain.TranscriptKindPartial {
			t.Fatalf("unexpected event: %+v", got)
		}
	})

	t.Run("drops when done is closed", func(t *testing.T) {
		done := make(chan struct{})
		close(done)
		s := &streamingSession{events: make(chan domain.TranscriptEvent), done: done}
		s.emit(domain.TranscriptEvent{Text: "ignored"})
	})

	t.Run("drops when channel full", func(t *testing.T) {
		ch := make(chan domain.TranscriptEvent, 1)
		ch <- domain.TranscriptEvent{Text: "first"}
		s := &streamingSession{events: ch, done: make(chan struct{})}
		s.emit(domain.TranscriptEvent{Text: "second"})
		got := <-ch
		if got.Text != "first" {
			t.Fatalf("expected original event to remain, got %+v", got)
		}
	})
}

func TestTruncateForLog(t *testing.T) {
	t.Parallel()

	if got := truncateForLog("hello", 0); got != "hello" {
		t.Fatalf("expected original for max<=0, got %q", got)
	}
	if got := truncateForLog("hello", 10); got != "hello" {
		t.Fatalf("expected original when under max, got %q", got)
	}
	if got := truncateForLog("hello world", 5); got != "hello..." {
		t.Fatalf("unexpected truncation: %q", got)
	}
}

func TestIsExpectedShutdownErrFalseCases(t *testing.T) {
	t.Parallel()

	if isExpectedShutdownErr(errors.New("boom")) {
		t.Fatal("plain error should not be expected shutdown")
	}
	if isExpectedShutdownErr(&websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "bad"}) {
		t.Fatal("abnormal close should not be expected shutdown")
	}
}

func TestStreamingSessionWaitNoErrorAndSetErrNil(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	s := &streamingSession{done: done}
	s.setErr(nil)
	close(done)
	if err := s.Wait(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
