package whispercpp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"coldmic/internal/debuglog"
)

const (
	// modelBaseURL is the HuggingFace repository for ggml whisper models.
	modelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

	// defaultModel is the default whisper model for wake phrase detection.
	defaultModel = "ggml-tiny.en.bin"
)

// Provider implements ports.LocalSTT using whisper.cpp as a subprocess.
type Provider struct {
	cacheDir   string // directory for model storage (~/.cache/coldmic)
	modelName  string // model filename, e.g. "ggml-tiny.en.bin"
	binaryPath string // path to whisper-cli binary (empty = look on PATH)
	binaryName string // name of the binary to search for
}

// Config configures the whisper.cpp provider.
type Config struct {
	CacheDir   string // directory for model storage
	Model      string // model filename (e.g. "ggml-tiny.en.bin")
	BinaryPath string // explicit path to whisper-cli (empty = auto-detect)
}

// NewProvider creates a new whisper.cpp subprocess provider.
func NewProvider(cfg Config) *Provider {
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}
	// Normalize short model names (e.g. "tiny.en" → "ggml-tiny.en.bin").
	if !strings.HasPrefix(model, "ggml-") {
		model = "ggml-" + model
	}
	if !strings.HasSuffix(model, ".bin") {
		model = model + ".bin"
	}

	// Determine binary name per platform.
	binName := "whisper-cli"
	if runtime.GOOS == "windows" {
		binName = "whisper-cli.exe"
	}

	return &Provider{
		cacheDir:   cfg.CacheDir,
		modelName:  model,
		binaryPath: cfg.BinaryPath,
		binaryName: binName,
	}
}

// Init ensures the model file is available. If the whisper-cli binary
// is not found, Init returns an error — it must be installed separately
// (package manager or manual build).
func (p *Provider) Init() error {
	// Resolve binary path.
	if p.binaryPath == "" {
		path, err := exec.LookPath(p.binaryName)
		if err != nil {
			return fmt.Errorf("whispercpp: %s not found on PATH: %w", p.binaryName, err)
		}
		p.binaryPath = path
		debuglog.Printf("whispercpp: using binary at %s", p.binaryPath)
	}

	// Ensure model file exists.
	if err := os.MkdirAll(p.cacheDir, 0o755); err != nil {
		return fmt.Errorf("whispercpp: create cache dir: %w", err)
	}

	modelPath := filepath.Join(p.cacheDir, p.modelName)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		debuglog.Printf("whispercpp: model not found at %s, downloading...", modelPath)
		url := fmt.Sprintf("%s/%s", modelBaseURL, p.modelName)
		if err := downloadFile(modelPath, url); err != nil {
			return fmt.Errorf("whispercpp: download model: %w", err)
		}
		debuglog.Printf("whispercpp: model downloaded to %s", modelPath)
	}

	return nil
}

// Transcribe converts a PCM audio buffer to text using whisper.cpp.
// audioPCM is 16-bit little-endian mono at 16kHz.
// Returns empty string for silent/unintelligible audio (no error).
func (p *Provider) Transcribe(ctx context.Context, audioPCM []byte) (string, error) {
	if len(audioPCM) == 0 {
		return "", nil
	}

	modelPath := filepath.Join(p.cacheDir, p.modelName)

	// Build WAV data from raw PCM.
	wavData, err := pcmToWAV(audioPCM, 16000, 1)
	if err != nil {
		return "", fmt.Errorf("whispercpp: build wav: %w", err)
	}

	// Run whisper-cli with audio on stdin.
	args := []string{
		"--model", modelPath,
		"--language", "en",
		"--output-text",
		"--no-timestamps",
		"-f", "-",
	}

	cmd := exec.CommandContext(ctx, p.binaryPath, args...)
	cmd.Stdin = bytes.NewReader(wavData)
	// Kill the entire process group on context cancellation.
	// Without this, child processes (e.g., sleep inside a shell script) survive.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	debuglog.Printf("whispercpp: running %s %v", p.binaryPath, args)

	if err := cmd.Run(); err != nil {
		// Context cancellation is expected, wrap cleanly.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("whispercpp: run: %w\nstderr: %s", err, stderr.String())
	}

	// Parse output: whisper-cli outputs the transcript after a blank line.
	text := parseOutput(stdout.String())
	debuglog.Printf("whispercpp: transcript=%q", text)

	return text, nil
}

// pcmToWAV wraps raw PCM data in a WAV header.
func pcmToWAV(pcm []byte, sampleRate int, channels int) ([]byte, error) {
	dataSize := uint32(len(pcm))
	headerSize := uint32(44) // standard WAV header size
	fileSize := headerSize + dataSize

	buf := new(bytes.Buffer)

	// RIFF header.
	if _, err := buf.Write([]byte("RIFF")); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, fileSize-8); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte("WAVE")); err != nil {
		return nil, err
	}

	// fmt chunk.
	if _, err := buf.Write([]byte("fmt ")); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(16)); err != nil { // chunk size
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(1)); err != nil { // PCM format
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(channels)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return nil, err
	}
	byteRate := uint32(sampleRate * channels * 2)
	if err := binary.Write(buf, binary.LittleEndian, byteRate); err != nil {
		return nil, err
	}
	blockAlign := uint16(channels * 2)
	if err := binary.Write(buf, binary.LittleEndian, blockAlign); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(16)); err != nil { // bits per sample
		return nil, err
	}

	// data chunk.
	if _, err := buf.Write([]byte("data")); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, dataSize); err != nil {
		return nil, err
	}
	if _, err := buf.Write(pcm); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// parseOutput extracts the transcript text from whisper-cli stdout.
// whisper-cli with --output-text prints the transcript after a newline separator.
// The format is typically:
//
//	[some header lines]
//
//	<transcript text>
func parseOutput(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Find the last non-empty line(s) that form the transcript.
	// With --output-text --no-timestamps, output is typically just the transcript.
	var transcript []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			transcript = nil // reset — transcript comes after blank line
			continue
		}
		transcript = append(transcript, trimmed)
	}
	if len(transcript) == 0 {
		return ""
	}
	return strings.Join(transcript, " ")
}

// downloadFile downloads a file from url to path.
func downloadFile(path, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(path)
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
