package deepgram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestStreamingSessionClose(t *testing.T) {
	t.Parallel()

	_, client, server := setupWebSocketPair(t)
	defer server.Close()

	events := make(chan domain.TranscriptEvent, 64)
	audio := make(chan []byte, 32)
	done := make(chan struct{})

	s := &streamingSession{
		conn:   client,
		events: events,
		audio:  audio,
		done:   done,
	}

	s.wg.Add(2)
	go s.readLoop()
	go s.writeLoop()
	go func() {
		s.wg.Wait()
		close(events)
		close(done)
		_ = client.Close()
	}()

	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestStreamingSessionWriteLoopSendsAudioAndCloseStream(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	serverResult := make(chan []byte, 2)
	closeStreamMsg := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.BinaryMessage {
				serverResult <- msg
			} else if msgType == websocket.TextMessage {
				closeStreamMsg <- string(msg)
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	events := make(chan domain.TranscriptEvent, 64)
	audio := make(chan []byte, 32)
	done := make(chan struct{})

	s := &streamingSession{
		conn:   client,
		events: events,
		audio:  audio,
		done:   done,
	}

	s.wg.Add(1)
	go s.writeLoop()
	go func() {
		s.wg.Wait()
		close(events)
		close(done)
		_ = client.Close()
	}()

	// Send an audio chunk.
	audio <- []byte("audio-data")
	close(audio)

	// Verify server received the binary chunk.
	select {
	case msg := <-serverResult:
		if string(msg) != "audio-data" {
			t.Fatalf("unexpected binary message: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for binary message")
	}

	// Verify server received the CloseStream message.
	select {
	case msg := <-closeStreamMsg:
		if !strings.Contains(msg, "CloseStream") {
			t.Fatalf("expected CloseStream message, got: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CloseStream message")
	}
}

func TestStreamingSessionReadLoopParsesResponses(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send partial response.
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			`{"type":"Results","is_final":false,"speech_final":false,"channel":{"alternatives":[{"transcript":"hello"}]}}`))

		// Send final response.
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			`{"type":"Results","is_final":true,"speech_final":true,"channel":{"alternatives":[{"transcript":"hello world"}]}}`))

		// Send error response.
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			`{"type":"Error","message":"test error"}`))

		// Wait for client to disconnect.
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	events := make(chan domain.TranscriptEvent, 64)
	audio := make(chan []byte, 32)
	done := make(chan struct{})

	s := &streamingSession{
		conn:   client,
		events: events,
		audio:  audio,
		done:   done,
	}

	s.wg.Add(1)
	go s.readLoop()
	go func() {
		s.wg.Wait()
		close(events)
		close(done)
		_ = client.Close()
	}()

	// Read partial event.
	select {
	case evt := <-events:
		if evt.Kind != domain.TranscriptKindPartial || evt.Text != "hello" {
			t.Fatalf("unexpected partial event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partial event")
	}

	// Read final event.
	select {
	case evt := <-events:
		if evt.Kind != domain.TranscriptKindFinal || evt.Text != "hello world" || !evt.IsSpeechFinal {
			t.Fatalf("unexpected final event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final event")
	}

	// Read error event (emitted as final with empty text).
	select {
	case evt := <-events:
		if evt.Kind != domain.TranscriptKindFinal || !evt.IsSpeechFinal {
			t.Fatalf("unexpected error event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error event")
	}
}

func TestStreamingSessionReadLoopIgnoresNonJSON(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send non-JSON binary message.
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`not-json`))

		// Then a valid response.
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			`{"type":"Results","is_final":true,"speech_final":true,"channel":{"alternatives":[{"transcript":"valid"}]}}`))

		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	events := make(chan domain.TranscriptEvent, 64)
	audio := make(chan []byte, 32)
	done := make(chan struct{})

	s := &streamingSession{
		conn:   client,
		events: events,
		audio:  audio,
		done:   done,
	}

	s.wg.Add(1)
	go s.readLoop()
	go func() {
		s.wg.Wait()
		close(events)
		close(done)
		_ = client.Close()
	}()

	// Should get the valid response only, non-JSON is ignored.
	select {
	case evt := <-events:
		if evt.Text != "valid" {
			t.Fatalf("unexpected event: %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestStreamingSessionWriteLoopError(t *testing.T) {
	t.Parallel()

	// Create a session with an already-closed connection to trigger write error.
	events := make(chan domain.TranscriptEvent, 64)
	audio := make(chan []byte, 32)
	done := make(chan struct{})

	// Use a closed connection.
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		if conn != nil {
			conn.Close()
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, _ := websocket.DefaultDialer.Dial(wsURL, nil)
	if client != nil {
		client.Close()
	}

	s := &streamingSession{
		conn:   client,
		events: events,
		audio:  audio,
		done:   done,
	}

	// Send audio to trigger write.
	audio <- []byte("will-fail")
	close(audio)

	s.wg.Add(1)
	go s.writeLoop()
	go func() {
		s.wg.Wait()
		close(events)
		close(done)
	}()

	// Wait for completion.
	select {
	case <-done:
		// Expected: write loop exited with error.
	case <-time.After(2 * time.Second):
		t.Fatal("write loop did not finish")
	}
}

func setupWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, *httptest.Server) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	var serverConn *websocket.Conn
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
		}
		close(ready)
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial failed: %v", err)
	}

	<-ready
	return serverConn, clientConn, server
}
