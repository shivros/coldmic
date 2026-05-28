package whispercpp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeWhisperScript is a shell script that mimics whisper-cli output.
const fakeWhisperScript = `#!/bin/sh
cat
echo ""
echo "Hello Alice."
`

func TestPCMToWAV(t *testing.T) {
	pcm := make([]byte, 3200) // 100ms of 16kHz mono 16-bit
	wav, err := pcmToWAV(pcm, 16000, 1)
	if err != nil {
		t.Fatalf("pcmToWAV: %v", err)
	}

	// Check RIFF header.
	if string(wav[:4]) != "RIFF" {
		t.Errorf("expected RIFF header, got %q", wav[:4])
	}
	if string(wav[8:12]) != "WAVE" {
		t.Errorf("expected WAVE format, got %q", wav[8:12])
	}
	// Check data chunk.
	if string(wav[36:40]) != "data" {
		t.Errorf("expected data chunk, got %q", wav[36:40])
	}
	// Data size should match PCM length.
	dataSize := uint32(wav[40]) | uint32(wav[41])<<8 | uint32(wav[42])<<16 | uint32(wav[43])<<24
	if int(dataSize) != len(pcm) {
		t.Errorf("data size = %d, want %d", dataSize, len(pcm))
	}
}

func TestPCMToWAVEmpty(t *testing.T) {
	wav, err := pcmToWAV([]byte{}, 16000, 1)
	if err != nil {
		t.Fatalf("pcmToWAV empty: %v", err)
	}
	// Should be 44 bytes header with 0 data.
	if len(wav) != 44 {
		t.Errorf("empty WAV length = %d, want 44", len(wav))
	}
}

func TestParseOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "simple transcript",
			output: "\nHello Alice.",
			want:   "Hello Alice.",
		},
		{
			name:   "multi-line transcript",
			output: "\nHello Alice. How are you?",
			want:   "Hello Alice. How are you?",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "whisper-cli header then transcript",
			output: "whisper-cli: processing...\n\nHello world.",
			want:   "Hello world.",
		},
		{
			name:   "whitespace only",
			output: "  \n  \n  ",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOutput(tt.output)
			if got != tt.want {
				t.Errorf("parseOutput(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestTranscribeEmptyAudio(t *testing.T) {
	p := &Provider{
		cacheDir:   t.TempDir(),
		modelName:  "ggml-tiny.en.bin",
		binaryPath: "/usr/bin/true",
	}
	text, err := p.Transcribe(context.Background(), []byte{})
	if err != nil {
		t.Fatalf("Transcribe empty: %v", err)
	}
	if text != "" {
		t.Errorf("Transcribe empty = %q, want empty", text)
	}
}

func TestTranscribeWithFakeBinary(t *testing.T) {
	// Create a fake whisper-cli binary.
	tmpDir := t.TempDir()
	binName := "whisper-cli"
	if runtime.GOOS == "windows" {
		binName = "whisper-cli.exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	script := "#!/bin/sh\ncat > /dev/null\necho ''\necho 'Hello Alice.'\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	// Create a fake model file (doesn't need to be real for subprocess test).
	modelPath := filepath.Join(tmpDir, "ggml-tiny.en.bin")
	if err := os.WriteFile(modelPath, []byte("fake model"), 0o644); err != nil {
		t.Fatalf("write fake model: %v", err)
	}

	p := &Provider{
		cacheDir:   tmpDir,
		modelName:  "ggml-tiny.en.bin",
		binaryPath: binPath,
	}

	// Generate some fake PCM audio.
	pcm := make([]byte, 3200)

	text, err := p.Transcribe(context.Background(), pcm)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	// The fake binary outputs "Hello Alice."
	if !strings.Contains(text, "Hello Alice") {
		t.Errorf("Transcribe = %q, want to contain 'Hello Alice'", text)
	}
}

func TestTranscribeContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	// Use a binary that blocks indefinitely.
	binPath := filepath.Join(tmpDir, "whisper-cli")
	script := "#!/bin/sh\nsleep 30\n"
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write blocking binary: %v", err)
	}
	// Settle after write to avoid text-file-busy on some platforms.
	time.Sleep(50 * time.Millisecond)

	modelPath := filepath.Join(tmpDir, "ggml-tiny.en.bin")
	if err := os.WriteFile(modelPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fake model: %v", err)
	}

	p := &Provider{
		cacheDir:   tmpDir,
		modelName:  "ggml-tiny.en.bin",
		binaryPath: binPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := p.Transcribe(ctx, make([]byte, 3200))
		done <- err
	}()

	select {
	case <-done:
		// CommandContext kills the process on context expiry.
		// On some platforms the zombie sleep may linger, but the call returns.
	case <-time.After(10 * time.Second):
		t.Fatal("Transcribe hung — subprocess not killed by context cancellation")
	}
}

func TestInitBinaryNotFound(t *testing.T) {
	p := NewProvider(Config{
		CacheDir: t.TempDir(),
		Model:    "ggml-tiny.en.bin",
	})
	err := p.Init()
	if err == nil {
		t.Fatal("expected error when binary not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}
