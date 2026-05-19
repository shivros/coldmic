package edge_tts

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewProviderDefaults(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{})
	if p.cfg.Command != "edge-tts" {
		t.Fatalf("unexpected command: %q", p.cfg.Command)
	}
	if p.cfg.Voice != "en-US-AriaNeural" {
		t.Fatalf("unexpected voice: %q", p.cfg.Voice)
	}
	if p.cfg.Rate != "+0%" {
		t.Fatalf("unexpected rate: %q", p.cfg.Rate)
	}
	if p.cfg.Volume != "+0%" {
		t.Fatalf("unexpected volume: %q", p.cfg.Volume)
	}
	if p.cfg.PlaybackCmd != "ffplay" {
		t.Fatalf("unexpected playback cmd: %q", p.cfg.PlaybackCmd)
	}
}

func TestNewProviderCustomConfig(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{
		Command:     "/usr/local/bin/edge-tts",
		Voice:       "en-GB-SoniaNeural",
		Rate:        "+20%",
		Volume:      "+10%",
		PlaybackCmd: "paplay",
	})
	if p.cfg.Command != "/usr/local/bin/edge-tts" {
		t.Fatalf("unexpected command: %q", p.cfg.Command)
	}
	if p.cfg.Voice != "en-GB-SoniaNeural" {
		t.Fatalf("unexpected voice: %q", p.cfg.Voice)
	}
	if p.cfg.PlaybackCmd != "paplay" {
		t.Fatalf("unexpected playback: %q", p.cfg.PlaybackCmd)
	}
}

func TestSynthesizeEmptyText(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{Command: "nonexistent-binary"})
	audio, err := p.Synthesize(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if audio != nil {
		t.Fatalf("expected nil audio for empty text")
	}
}

func TestPlayEmptyText(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{Command: "nonexistent-binary"})
	if err := p.Play(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSynthesizeCommandNotFound(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{Command: "definitely-not-edge-tts-xyz-123"})
	_, err := p.Synthesize(context.Background(), "hello world")
	if err == nil {
		t.Fatalf("expected error when edge-tts is not found")
	}
	if !strings.Contains(err.Error(), "edge-tts") {
		t.Fatalf("expected error to mention edge-tts, got: %v", err)
	}
}

func TestPlayCommandNotFound(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{Command: "definitely-not-edge-tts-xyz-123"})
	err := p.Play(context.Background(), "hello world")
	if err == nil {
		t.Fatalf("expected error when edge-tts is not found")
	}
}

func TestSynthesizeWithMockCommand(t *testing.T) {
	t.Parallel()

	// Create a temporary script that simulates edge-tts output
	tmpDir := t.TempDir()
	mockTTS := filepath.Join(tmpDir, "edge-tts")
	mockAudio := []byte("fake-mp3-data-header")

	script := "#!/bin/sh\n" +
		"# Parse out --text arg for validation\n" +
		"echo -n '" + string(mockAudio) + "'\n"
	if err := os.WriteFile(mockTTS, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}

	p := NewProvider(Config{Command: mockTTS})
	audio, err := p.Synthesize(context.Background(), "test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(audio, mockAudio) {
		t.Fatalf("expected %q, got %q", mockAudio, audio)
	}
}

func TestPlayWithMockCommands(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Mock edge-tts that produces fake audio
	mockTTS := filepath.Join(tmpDir, "edge-tts")
	ttsScript := "#!/bin/sh\necho -n 'fake-audio-data'"
	if err := os.WriteFile(mockTTS, []byte(ttsScript), 0o755); err != nil {
		t.Fatalf("failed to create mock tts: %v", err)
	}

	// Mock ffplay that reads stdin and exits
	mockPlayer := filepath.Join(tmpDir, "ffplay")
	playerScript := "#!/bin/sh\ncat > /dev/null\nexit 0\n"
	if err := os.WriteFile(mockPlayer, []byte(playerScript), 0o755); err != nil {
		t.Fatalf("failed to create mock player: %v", err)
	}

	p := NewProvider(Config{
		Command:     mockTTS,
		PlaybackCmd: mockPlayer,
	})

	if err := p.Play(context.Background(), "hello world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSynthesizeContextCancellation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Mock edge-tts that sleeps forever
	mockTTS := filepath.Join(tmpDir, "edge-tts")
	script := "#!/bin/sh\nsleep 30\n"
	if err := os.WriteFile(mockTTS, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}

	p := NewProvider(Config{Command: mockTTS})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := p.Synthesize(ctx, "this will be cancelled")
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
	// context.DeadlineExceeded or context.Canceled both acceptable
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got: %v", err)
	}
}

func TestPlayContextCancellation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Mock edge-tts that produces output quickly
	mockTTS := filepath.Join(tmpDir, "edge-tts")
	ttsScript := "#!/bin/sh\necho -n 'audio'\n"
	if err := os.WriteFile(mockTTS, []byte(ttsScript), 0o755); err != nil {
		t.Fatalf("failed to create mock tts: %v", err)
	}

	// Mock player that sleeps forever
	mockPlayer := filepath.Join(tmpDir, "ffplay")
	playerScript := "#!/bin/sh\ncat > /dev/null\nsleep 30\n"
	if err := os.WriteFile(mockPlayer, []byte(playerScript), 0o755); err != nil {
		t.Fatalf("failed to create mock player: %v", err)
	}

	p := NewProvider(Config{
		Command:     mockTTS,
		PlaybackCmd: mockPlayer,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := p.Play(ctx, "this will be cancelled during playback")
	if err == nil {
		t.Fatalf("expected error from cancelled context")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got: %v", err)
	}
}

func TestSynthesizeViaPipeEmptyText(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{Command: "nonexistent"})
	audio, err := p.SynthesizeViaPipe(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if audio != nil {
		t.Fatalf("expected nil audio for empty text")
	}
}

func TestSynthesizeViaPipeMock(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mockTTS := filepath.Join(tmpDir, "edge-tts")
	mockAudio := []byte("pipe-audio-data")

	// Script that reads stdin and writes mock audio to stdout
	script := "#!/bin/sh\ncat > /dev/null\necho -n '" + string(mockAudio) + "'\n"
	if err := os.WriteFile(mockTTS, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}

	p := NewProvider(Config{Command: mockTTS})
	audio, err := p.SynthesizeViaPipe(context.Background(), "test via pipe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(audio, mockAudio) {
		t.Fatalf("expected %q, got %q", mockAudio, audio)
	}
}

func TestIsAvailable(t *testing.T) {
	t.Parallel()

	p := NewProvider(Config{Command: "definitely-not-edge-tts-xyz-123"})
	if p.IsAvailable() {
		t.Fatalf("expected nonexistent binary to not be available")
	}

	// Use a command that definitely exists
	p2 := NewProvider(Config{Command: "sh"})
	if !p2.IsAvailable() {
		t.Fatalf("expected sh to be available")
	}
}

func TestSynthesizeNoAudioOutput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mockTTS := filepath.Join(tmpDir, "edge-tts")

	// Mock that produces no stdout
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(mockTTS, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}

	p := NewProvider(Config{Command: mockTTS})
	_, err := p.Synthesize(context.Background(), "test")
	if err == nil {
		t.Fatalf("expected error when no audio produced")
	}
	if !strings.Contains(err.Error(), "no audio output") {
		t.Fatalf("expected 'no audio output' error, got: %v", err)
	}
}

func TestPlayAudioCustomPlaybackCmd(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Custom player that just consumes stdin
	mockPlayer := filepath.Join(tmpDir, "custom-player")
	script := "#!/bin/sh\ncat > /dev/null\nexit 0\n"
	if err := os.WriteFile(mockPlayer, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock player: %v", err)
	}

	p := NewProvider(Config{PlaybackCmd: mockPlayer})
	err := p.playAudio(context.Background(), []byte("fake audio"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlayAudioPlaybackFails(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Player that exits with error
	mockPlayer := filepath.Join(tmpDir, "fail-player")
	script := "#!/bin/sh\necho 'playback error' >&2\nexit 1\n"
	if err := os.WriteFile(mockPlayer, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock player: %v", err)
	}

	p := NewProvider(Config{PlaybackCmd: mockPlayer})
	err := p.playAudio(context.Background(), []byte("fake audio"))
	if err == nil {
		t.Fatalf("expected playback error")
	}
	if !strings.Contains(err.Error(), "playback failed") {
		t.Fatalf("expected playback failed error, got: %v", err)
	}
}

// compile-time interface check
func TestProviderImplementsPort(t *testing.T) {
	t.Parallel()

	var _ interface {
		Synthesize(ctx context.Context, text string) ([]byte, error)
		Play(ctx context.Context, text string) error
	} = NewProvider(Config{})
}

func TestSynthesizeEdgeTTSMockWithArgs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mockTTS := filepath.Join(tmpDir, "edge-tts")
	// Script that verifies --voice, --rate, --volume, --text, --write-media args are passed
	script := `#!/bin/sh
has_voice=0
has_rate=0
has_volume=0
has_text=0
has_write_media=0
for arg in "$@"; do
	case "$arg" in
		--voice) has_voice=1 ;;
		--rate) has_rate=1 ;;
		--volume) has_volume=1 ;;
		--text) has_text=1 ;;
		--write-media) has_write_media=1 ;;
	esac
done
if [ "$has_voice" = "0" ] || [ "$has_rate" = "0" ] || [ "$has_volume" = "0" ] || [ "$has_text" = "0" ] || [ "$has_write_media" = "0" ]; then
	echo "missing required args" >&2
	exit 1
fi
echo -n "mock-audio"
`
	if err := os.WriteFile(mockTTS, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}

	p := NewProvider(Config{Command: mockTTS, Voice: "en-US-GuyNeural", Rate: "-10%", Volume: "+5%"})
	audio, err := p.Synthesize(context.Background(), "verify args")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(audio) != "mock-audio" {
		t.Fatalf("expected 'mock-audio', got %q", audio)
	}
}

func TestLookPathUsedByIsAvailable(t *testing.T) {
	t.Parallel()

	// "true" is a standard Unix utility that always exists
	p := NewProvider(Config{Command: "true"})
	if !p.IsAvailable() {
		t.Fatalf("expected 'true' to be found in PATH")
	}

	p2 := NewProvider(Config{Command: "/nonexistent/path/binary"})
	if p2.IsAvailable() {
		t.Fatalf("expected nonexistent path to not be available")
	}
}

// Ensure exec.CommandContext kills the process on context cancel.
func TestContextCancelKillsProcess(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mockTTS := filepath.Join(tmpDir, "edge-tts")

	// Script that creates a pid file so we can verify it gets killed
	pidFile := filepath.Join(tmpDir, "tts.pid")
	script := "#!/bin/sh\necho $$ > " + pidFile + "\nsleep 30\n"
	if err := os.WriteFile(mockTTS, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to create mock: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	p := NewProvider(Config{Command: mockTTS})
	_, err := p.Synthesize(ctx, "should be killed")
	if err == nil {
		t.Fatalf("expected timeout error")
	}

	// Give a moment for process cleanup
	time.Sleep(100 * time.Millisecond)

	// Read the PID that was written and verify it's no longer running
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("failed to read pid file: %v", err)
	}
	pid := strings.TrimSpace(string(data))

	// Check that the process is no longer running
	checkCmd := exec.Command("kill", "-0", pid)
	if checkCmd.Run() == nil {
		t.Fatalf("expected process %s to be killed but it's still running", pid)
	}
}
