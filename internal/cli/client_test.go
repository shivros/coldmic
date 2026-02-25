package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientStartSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/session/start" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"recording","active":true}}`))
	}))
	defer server.Close()

	status, err := NewClient(server.URL).Start(context.Background())
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if status.State != "recording" || !status.Active {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestClientStopReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"error":"no active recording session"}`))
	}))
	defer server.Close()

	_, _, err := NewClient(server.URL).Stop(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != http.StatusConflict {
		t.Fatalf("unexpected code: %d", httpErr.StatusCode)
	}
}

func TestClientTranscriptSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/session/transcript/latest" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"captured":"2026-02-25T12:00:00Z","result":{"rawTranscript":"raw","finalTranscript":"final","copied":true}}`))
	}))
	defer server.Close()

	capturedAt, result, err := NewClient(server.URL).Transcript(context.Background())
	if err != nil {
		t.Fatalf("transcript failed: %v", err)
	}
	if capturedAt.IsZero() || result.FinalTranscript != "final" {
		t.Fatalf("unexpected payload: %v %+v", capturedAt, result)
	}
}

func TestClientStatusDecodeFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).Status(context.Background())
	if err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestHTTPErrorString(t *testing.T) {
	t.Parallel()

	if got := (HTTPError{StatusCode: 418}).Error(); got == "" {
		t.Fatalf("expected message")
	}
	if got := (HTTPError{StatusCode: 500, Message: "boom"}).Error(); got != "boom" {
		t.Fatalf("unexpected message: %s", got)
	}
}
