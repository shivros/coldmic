package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	coldcli "coldmic/internal/cli"
	"coldmic/internal/domain"
)

func TestRunCommandUnknown(t *testing.T) {

	code, err := runCommand("wat", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected exit code: %d", code)
	}
}

func TestRunCommandHelp(t *testing.T) {

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
		case "/v1/session/abort":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"idle","active":false}}`))
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

	code, err = runAbort(args)
	if err != nil || code != exitOK {
		t.Fatalf("runAbort failed: code=%d err=%v", code, err)
	}
}

func TestParseCommonFlagsError(t *testing.T) {

	_, err := parseCommonFlags("status", []string{"--json=maybe"})
	if err == nil {
		t.Fatalf("expected flag parse error")
	}
}

func TestRunAbortConflict(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/abort" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"error":"no active recording session"}`))
	}))
	defer server.Close()

	code, err := runAbort([]string{"--daemon-url", server.URL})
	if err == nil {
		t.Fatalf("expected abort conflict error")
	}
	if code != exitConflict {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestRunStatusCheckActive(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"recording","active":true}}`))
	}))
	defer server.Close()

	output := captureOutput(t, func() {
		code, err := runStatus([]string{"--daemon-url", server.URL, "--check"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != exitOK {
			t.Fatalf("unexpected code: %d", code)
		}
	})
	if output != "" {
		t.Fatalf("expected no output, got: %q", output)
	}
}

func TestRunStatusCheckIdle(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"idle","active":false}}`))
	}))
	defer server.Close()

	output := captureOutput(t, func() {
		code, err := runStatus([]string{"--daemon-url", server.URL, "--check"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if code != exitGeneric {
			t.Fatalf("unexpected code: %d", code)
		}
	})
	if output != "" {
		t.Fatalf("expected no output, got: %q", output)
	}
}

func TestRunNoCommandToggleOff(t *testing.T) {
	t.Setenv("COLDMIC_TOGGLE_COMPAT", "false")

	code, err := runNoCommand()
	if err == nil {
		t.Fatalf("expected missing command error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestRunNoCommandToggleOnIdleStarts(t *testing.T) {
	t.Setenv("COLDMIC_TOGGLE_COMPAT", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/session/status":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"idle","active":false}}`))
		case "/v1/session/start":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"recording","active":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("COLDMIC_DAEMON_URL", server.URL)

	code, err := runNoCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestRunNoCommandToggleOnRecordingStops(t *testing.T) {
	t.Setenv("COLDMIC_TOGGLE_COMPAT", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/session/status":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"recording","active":true}}`))
		case "/v1/session/stop":
			_, _ = w.Write([]byte(`{"ok":true,"status":{"state":"idle","active":false},"result":{"rawTranscript":"raw","finalTranscript":"final","copied":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("COLDMIC_DAEMON_URL", server.URL)

	code, err := runNoCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected code: %d", code)
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

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	_ = w.Close()
	os.Stdout = old

	return <-done
}

func TestToggleCompatEnabled(t *testing.T) {
	t.Setenv("COLDMIC_TOGGLE_COMPAT", "TrUe")
	if !toggleCompatEnabled() {
		t.Fatalf("expected toggle compat enabled")
	}

	t.Setenv("COLDMIC_TOGGLE_COMPAT", "1")
	if toggleCompatEnabled() {
		t.Fatalf("expected toggle compat disabled")
	}

	t.Setenv("COLDMIC_TOGGLE_COMPAT", "")
	if toggleCompatEnabled() {
		t.Fatalf("expected toggle compat disabled")
	}
}

func TestRunNoCommandMapsStatusError(t *testing.T) {
	t.Setenv("COLDMIC_TOGGLE_COMPAT", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":false,"error":"no active recording session"}`))
	}))
	defer server.Close()
	t.Setenv("COLDMIC_DAEMON_URL", server.URL)

	code, err := runNoCommand()
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitConflict {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerUsesInjectedClientForStart(t *testing.T) {
	client := &fakeSessionClient{
		startStatus: domain.Status{State: domain.SessionStateRecording, Active: true},
	}
	cfg := fakeConfig{daemonURL: "http://fake-daemon"}

	var gotURL string
	runner := NewCommandRunner(func(daemonURL string) SessionClient {
		gotURL = daemonURL
		return client
	}, cfg, &bytes.Buffer{}, io.Discard)

	code, err := runner.Run("start", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected code: %d", code)
	}
	if gotURL != cfg.daemonURL {
		t.Fatalf("expected daemon URL %q, got %q", cfg.daemonURL, gotURL)
	}
	if client.startCalls != 1 {
		t.Fatalf("expected start call")
	}
}

func TestCommandRunnerRegistryUnknown(t *testing.T) {
	var out bytes.Buffer
	runner := NewCommandRunner(func(string) SessionClient { return &fakeSessionClient{} }, fakeConfig{}, &out, io.Discard)

	code, err := runner.Run("does-not-exist", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected code: %d", code)
	}
	if !strings.Contains(out.String(), "Usage: coldmic") {
		t.Fatalf("expected usage output")
	}
}

func TestCommandRunnerNoCommandToggleOnActiveStops(t *testing.T) {
	client := &fakeSessionClient{
		statusStatus: domain.Status{State: domain.SessionStateRecording, Active: true},
		stopStatus:   domain.Status{State: domain.SessionStateIdle, Active: false},
		stopResult:   domain.StopResult{FinalTranscript: "final"},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{daemonURL: "http://fake", toggleCompat: true},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.RunNoCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected code: %d", code)
	}
	if client.statusCalls != 1 || client.stopCalls != 1 {
		t.Fatalf("expected status+stop calls, got status=%d stop=%d", client.statusCalls, client.stopCalls)
	}
}

func TestCommandRunnerNoCommandToggleOnIdleStarts(t *testing.T) {
	client := &fakeSessionClient{
		statusStatus: domain.Status{State: domain.SessionStateIdle, Active: false},
		startStatus:  domain.Status{State: domain.SessionStateRecording, Active: true},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{daemonURL: "http://fake", toggleCompat: true},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.RunNoCommand()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected code: %d", code)
	}
	if client.statusCalls != 1 || client.startCalls != 1 {
		t.Fatalf("expected status+start calls, got status=%d start=%d", client.statusCalls, client.startCalls)
	}
}

func TestCommandRunnerNoCommandToggleOff(t *testing.T) {
	var out bytes.Buffer
	runner := NewCommandRunner(
		func(string) SessionClient { return &fakeSessionClient{} },
		fakeConfig{toggleCompat: false},
		&out,
		io.Discard,
	)

	code, err := runner.RunNoCommand()
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected code: %d", code)
	}
	if !strings.Contains(out.String(), "Usage: coldmic") {
		t.Fatalf("expected usage output")
	}
}

func TestCommandRunnerNoCommandMapsClientError(t *testing.T) {
	client := &fakeSessionClient{
		statusErr: coldcli.HTTPError{StatusCode: http.StatusConflict, Message: "no active recording session"},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{toggleCompat: true},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.RunNoCommand()
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitConflict {
		t.Fatalf("unexpected code: %d", code)
	}
}

type fakeConfig struct {
	daemonURL    string
	toggleCompat bool
}

func (f fakeConfig) DaemonURL() string {
	if f.daemonURL == "" {
		return "http://default-daemon"
	}
	return f.daemonURL
}

func (f fakeConfig) ToggleCompatEnabled() bool {
	return f.toggleCompat
}

type fakeSessionClient struct {
	startCalls      int
	stopCalls       int
	abortCalls      int
	statusCalls     int
	transcriptCalls int

	startStatus  domain.Status
	stopStatus   domain.Status
	stopResult   domain.StopResult
	abortStatus  domain.Status
	statusStatus domain.Status
	transcriptAt time.Time
	transcript   domain.StopResult

	startErr      error
	stopErr       error
	abortErr      error
	statusErr     error
	transcriptErr error
}

func (f *fakeSessionClient) Start(context.Context) (domain.Status, error) {
	f.startCalls++
	if f.startErr != nil {
		return domain.Status{}, f.startErr
	}
	return f.startStatus, nil
}

func (f *fakeSessionClient) Stop(context.Context) (domain.Status, domain.StopResult, error) {
	f.stopCalls++
	if f.stopErr != nil {
		return domain.Status{}, domain.StopResult{}, f.stopErr
	}
	return f.stopStatus, f.stopResult, nil
}

func (f *fakeSessionClient) Abort(context.Context) (domain.Status, error) {
	f.abortCalls++
	if f.abortErr != nil {
		return domain.Status{}, f.abortErr
	}
	return f.abortStatus, nil
}

func (f *fakeSessionClient) Status(context.Context) (domain.Status, error) {
	f.statusCalls++
	if f.statusErr != nil {
		return domain.Status{}, f.statusErr
	}
	return f.statusStatus, nil
}

func (f *fakeSessionClient) Transcript(context.Context) (time.Time, domain.StopResult, error) {
	f.transcriptCalls++
	if f.transcriptErr != nil {
		return time.Time{}, domain.StopResult{}, f.transcriptErr
	}
	if f.transcriptAt.IsZero() {
		return time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC), f.transcript, nil
	}
	return f.transcriptAt, f.transcript, nil
}

func TestCommandRunnerNoCommandPropagatesStatusError(t *testing.T) {
	client := &fakeSessionClient{statusErr: errors.New("boom")}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{toggleCompat: true},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.RunNoCommand()
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerStartMapsClientError(t *testing.T) {
	client := &fakeSessionClient{
		startErr: coldcli.HTTPError{StatusCode: http.StatusConflict, Message: "conflict"},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.Run("start", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitConflict {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerStopMapsClientError(t *testing.T) {
	client := &fakeSessionClient{
		stopErr: coldcli.HTTPError{StatusCode: http.StatusNotFound, Message: "missing"},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.Run("stop", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitNotFound {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerAbortMapsClientError(t *testing.T) {
	client := &fakeSessionClient{
		abortErr: coldcli.HTTPError{StatusCode: http.StatusConflict, Message: "conflict"},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.Run("abort", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitConflict {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerTranscriptMapsClientError(t *testing.T) {
	client := &fakeSessionClient{
		transcriptErr: coldcli.HTTPError{StatusCode: http.StatusNotFound, Message: "missing"},
	}
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.Run("transcript", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitNotFound {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerStatusParseError(t *testing.T) {
	runner := NewCommandRunner(
		func(string) SessionClient { return &fakeSessionClient{} },
		fakeConfig{},
		&bytes.Buffer{},
		io.Discard,
	)

	code, err := runner.Run("status", []string{"--check=maybe"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if code != exitGeneric {
		t.Fatalf("unexpected code: %d", code)
	}
}

func TestCommandRunnerStatusJSONOutput(t *testing.T) {
	client := &fakeSessionClient{
		statusStatus: domain.Status{State: domain.SessionStateRecording, Active: true},
	}
	var out bytes.Buffer
	runner := NewCommandRunner(
		func(string) SessionClient { return client },
		fakeConfig{},
		&out,
		io.Discard,
	)

	code, err := runner.Run("status", []string{"--json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("unexpected code: %d", code)
	}
	if !strings.Contains(out.String(), "\"status\"") {
		t.Fatalf("expected JSON output, got: %s", out.String())
	}
}

func TestPrintUsageWrapper(t *testing.T) {
	output := captureOutput(t, func() {
		printUsage()
	})
	if !strings.Contains(output, "Usage: coldmic") {
		t.Fatalf("expected usage output")
	}
}
