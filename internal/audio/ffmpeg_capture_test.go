package audio

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coldmic/internal/ports"
)

func TestFFMPEGCaptureStartReadAndStop(t *testing.T) {
	t.Parallel()

	script := writeScript(t, "capture.sh", "#!/usr/bin/env bash\nprintf 'hello'\nsleep 2\n")
	capture := NewFFMPEGCapture(script)

	session, err := capture.Start(context.Background(), ports.AudioConfig{})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	buf := make([]byte, 8)
	n, readErr := session.Read(buf)
	if n <= 0 {
		t.Fatalf("expected audio bytes, got n=%d err=%v", n, readErr)
	}
	if !strings.Contains(string(buf[:n]), "hello") {
		t.Fatalf("unexpected bytes: %q", string(buf[:n]))
	}

	if err := session.Stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestFFMPEGCaptureStartEarlyExit(t *testing.T) {
	t.Parallel()

	script := writeScript(t, "fail.sh", "#!/usr/bin/env bash\necho 'boom' 1>&2\nexit 1\n")
	capture := NewFFMPEGCapture(script)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := capture.Start(ctx, ports.AudioConfig{})
	if err == nil {
		t.Fatalf("expected early exit error")
	}
	if !strings.Contains(err.Error(), "exited before capture started") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeStopErrExitErrorIsIgnored(t *testing.T) {
	t.Parallel()

	err := exec.Command("bash", "-lc", "exit 1").Run()
	if err == nil {
		t.Fatalf("expected command to fail")
	}
	if got := normalizeStopErr(err); got != nil {
		t.Fatalf("expected nil for exit error, got %v", got)
	}
}

func TestStringsTrimSpaceSafe(t *testing.T) {
	t.Parallel()

	if got := stringsTrimSpaceSafe("  hi\n"); got != "hi" {
		t.Fatalf("unexpected trim result: %q", got)
	}
}

func writeScript(t *testing.T, name string, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	return path
}
