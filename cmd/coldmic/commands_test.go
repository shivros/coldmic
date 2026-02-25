package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunCommandUnknown(t *testing.T) {
	t.Parallel()

	code, err := runCommand("wat", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected exit code: %d", code)
	}
}

func TestRunCommandHelp(t *testing.T) {
	t.Parallel()

	code, err := runCommand("help", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected exit code: %d", code)
	}
}

func TestRunStartAndStatusAndStopAndTranscript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/session/start":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"recording","active":true}}`))
		case "/v1/session/status":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"recording","active":true}}`))
		case "/v1/session/stop":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"idle","active":false},"result":{"rawTranscript":"raw","finalTranscript":"final","copied":true}}`))
		case "/v1/session/transcript/latest":
			_, _ = w.Write([]byte(`{"ok":true,"captured":"2026-02-25T12:00:00Z","result":{"rawTranscript":"raw","finalTranscript":"final","copied":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	stdout, stderr := captureStdPipes(t)
	defer restoreStdPipes(stdout, stderr)

	args := []string{"--daemon-url", server.URL, "--json"}

	code, err := runStart(args)
	if err != nil || code != exitOK {
		t.Fatalf("runStart failed: code=%d err=%v", code, err)
	}

	code, err = runStatus(args)
	if err != nil || code != exitOK {
		t.Fatalf("runStatus failed: code=%d err=%v", code, err)
	}

	code, err = runStop(args)
	if err != nil || code != exitOK {
		t.Fatalf("runStop failed: code=%d err=%v", code, err)
	}

	code, err = runTranscript(args)
	if err != nil || code != exitOK {
		t.Fatalf("runTranscript failed: code=%d err=%v", code, err)
	}
}

func TestParseCommonFlagsError(t *testing.T) {
	t.Parallel()

	_, err := parseCommonFlags("status", []string{"--json=maybe"})
	if err == nil {
		t.Fatalf("expected flag parse error")
	}
}

func captureStdPipes(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	oldOut := os.Stdout
	oldErr := os.Stderr

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = outW
	os.Stderr = errW

	t.Cleanup(func() {
		os.Stdout = oldOut
		os.Stderr = oldErr
		_ = outW.Close()
		_ = errW.Close()
		_, _ = io.ReadAll(outR)
		_, _ = io.ReadAll(errR)
		_ = outR.Close()
		_ = errR.Close()
	})
	return oldOut, oldErr
}

func restoreStdPipes(oldOut *os.File, oldErr *os.File) {
	os.Stdout = oldOut
	os.Stderr = oldErr
}

func TestPrintTranscriptTime(t *testing.T) {
	t.Parallel()
	got := printTranscriptTime(mustTime("2026-02-25T12:00:00Z"))
	if !strings.Contains(got, "2026-02-25T12:00:00Z") {
		t.Fatalf("unexpected time: %s", got)
	}
}

func mustTime(value string) time.Time {
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return ts
}
