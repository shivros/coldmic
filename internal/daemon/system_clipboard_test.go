package daemon

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestSystemClipboardSetTextFallsBackToSecondCommand(t *testing.T) {
	restore := stubClipboardDeps()
	defer restore()

	var attempted [][]string
	clipboardCommandsFn = func() [][]string {
		return [][]string{{"cmd-a"}, {"cmd-b"}}
	}
	lookPathFn = func(file string) (string, error) {
		if file == "cmd-a" {
			return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
		}
		return "/usr/bin/" + file, nil
	}
	runClipboardCommandFn = func(_ context.Context, args []string, _ string) error {
		attempted = append(attempted, args)
		return nil
	}

	if err := (SystemClipboard{}).SetText(context.Background(), "hi"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(attempted) != 1 || attempted[0][0] != "cmd-b" {
		t.Fatalf("unexpected attempts: %+v", attempted)
	}
}

func TestSystemClipboardSetTextReturnsErrorWhenNoCommandWorks(t *testing.T) {
	restore := stubClipboardDeps()
	defer restore()

	clipboardCommandsFn = func() [][]string {
		return [][]string{{"cmd-a"}}
	}
	lookPathFn = func(file string) (string, error) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	runClipboardCommandFn = func(_ context.Context, _ []string, _ string) error {
		return nil
	}

	err := (SystemClipboard{}).SetText(context.Background(), "hi")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSystemClipboardSetTextReturnsCommandFailure(t *testing.T) {
	restore := stubClipboardDeps()
	defer restore()

	clipboardCommandsFn = func() [][]string {
		return [][]string{{"cmd-a"}}
	}
	lookPathFn = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
	runClipboardCommandFn = func(_ context.Context, _ []string, _ string) error {
		return errors.New("write failed")
	}

	err := (SystemClipboard{}).SetText(context.Background(), "hi")
	if err == nil || err.Error() == "" {
		t.Fatalf("expected wrapped command error")
	}
}

func TestClipboardCommandsNotEmpty(t *testing.T) {
	t.Parallel()

	candidates := clipboardCommands()
	if len(candidates) == 0 {
		t.Fatalf("expected clipboard command candidates")
	}
}

func TestNoopEventSinkMethods(t *testing.T) {
	t.Parallel()

	var sink NoopEventSink
	sink.SessionStateChanged("idle", "mic_cold")
	sink.PartialTranscript("partial")
	sink.FinalTranscript("raw", "final", "session-1")
	sink.SessionError("transcription", "detail")
}

func TestRunClipboardCommand(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	err := runClipboardCommand(context.Background(), []string{"sh", "-c", "cat >/dev/null"}, "hello")
	if err != nil {
		t.Fatalf("unexpected runClipboardCommand error: %v", err)
	}
}

func stubClipboardDeps() func() {
	originalCommands := clipboardCommandsFn
	originalLookPath := lookPathFn
	originalRun := runClipboardCommandFn

	return func() {
		clipboardCommandsFn = originalCommands
		lookPathFn = originalLookPath
		runClipboardCommandFn = originalRun
	}
}
