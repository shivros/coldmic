package cli

import (
	"context"
	"errors"
	"net"
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

func TestClientAbortSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/session/abort" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"idle","active":false}}`))
	}))
	defer server.Close()

	status, err := NewClient(server.URL).Abort(context.Background())
	if err != nil {
		t.Fatalf("abort failed: %v", err)
	}
	if status.State != "idle" || status.Active {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestClientAbortReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"error":"no active recording session"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL).Abort(context.Background())
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

func TestClientCallPayloadEncodeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	c := NewClient(server.URL)
	err := c.call(context.Background(), http.MethodPost, "/v1/session/start", func() {}, &envelope{})
	if err == nil {
		t.Fatalf("expected encode error")
	}
}

func TestClientCallRequestBuildError(t *testing.T) {
	t.Parallel()

	c := NewClient("://bad")
	_, err := c.Status(context.Background())
	if err == nil {
		t.Fatalf("expected request build error")
	}
}

func TestClientCallTransportError(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	c := NewClient("http://" + addr)
	_, err = c.Status(context.Background())
	if err == nil {
		t.Fatalf("expected transport error")
	}
}

func TestClientCallDefaultHTTPErrorBranch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	err := c.call(context.Background(), http.MethodGet, "/x", nil, &struct{}{})
	if err == nil {
		t.Fatalf("expected HTTP error")
	}
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.Message != "request failed" {
		t.Fatalf("unexpected message: %s", httpErr.Message)
	}
}

func TestClientCallPayloadSetsJSONHeader(t *testing.T) {
	t.Parallel()

	var gotContentType string
	var gotAccept string
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	err := c.call(context.Background(), http.MethodPost, "/payload", map[string]string{"a": "b"}, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("unexpected method: %s", gotMethod)
	}
	if gotAccept != "application/json" {
		t.Fatalf("unexpected accept header: %s", gotAccept)
	}
	if gotContentType != "application/json" {
		t.Fatalf("unexpected content-type header: %s", gotContentType)
	}
}
