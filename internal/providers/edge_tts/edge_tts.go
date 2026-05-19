package edge_tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"coldmic/internal/debuglog"
	"coldmic/internal/ports"
)

// Config controls edge-tts subprocess behaviour.
type Config struct {
	Command     string // path to edge-tts binary (default: "edge-tts")
	Voice       string // voice name (default: "en-US-AriaNeural")
	Rate        string // rate adjustment e.g. "+0%" (default: "+0%")
	Volume      string // volume adjustment e.g. "+0%" (default: "+0%")
	PlaybackCmd string // audio player (default: "ffplay")
}

// Provider implements ports.TextToSpeech using the edge-tts CLI tool.
type Provider struct {
	cfg Config
}

// NewProvider creates a new edge-tts provider with the given config.
// Fills in defaults for any zero-valued fields.
func NewProvider(cfg Config) *Provider {
	if cfg.Command == "" {
		cfg.Command = "edge-tts"
	}
	if cfg.Voice == "" {
		cfg.Voice = "en-US-AriaNeural"
	}
	if cfg.Rate == "" {
		cfg.Rate = "+0%"
	}
	if cfg.Volume == "" {
		cfg.Volume = "+0%"
	}
	if cfg.PlaybackCmd == "" {
		cfg.PlaybackCmd = "ffplay"
	}
	return &Provider{cfg: cfg}
}

// Synthesize generates mp3 audio from text using the edge-tts subprocess.
func (p *Provider) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if text == "" {
		return nil, nil
	}

	args := []string{
		"--voice", p.cfg.Voice,
		"--rate", p.cfg.Rate,
		"--volume", p.cfg.Volume,
		"--text", text,
		"--write-media", "-", // write mp3 to stdout
	}

	debuglog.Printf("edge-tts synthesize voice=%s rate=%s text_len=%d", p.cfg.Voice, p.cfg.Rate, len(text))

	cmd := exec.CommandContext(ctx, p.cfg.Command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("edge-tts failed: %w: %s", err, stderr.String())
	}

	audio := stdout.Bytes()
	if len(audio) == 0 {
		return nil, errors.New("edge-tts produced no audio output")
	}

	debuglog.Printf("edge-tts synthesize complete bytes=%d", len(audio))
	return audio, nil
}

// Play synthesizes text and plays it through the system audio output.
// Blocks until playback completes or the context is cancelled.
func (p *Provider) Play(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}

	audio, err := p.Synthesize(ctx, text)
	if err != nil {
		return err
	}

	return p.playAudio(ctx, audio)
}

func (p *Provider) playAudio(ctx context.Context, audio []byte) error {
	var cmd *exec.Cmd
	var stderr bytes.Buffer

	switch p.cfg.PlaybackCmd {
	case "ffplay":
		cmd = exec.CommandContext(ctx, "ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", "-i", "pipe:0")
		cmd.Stdin = bytes.NewReader(audio)
		cmd.Stderr = &stderr
	default:
		// paplay, aplay, or any custom command — pipe audio to stdin
		cmd = exec.CommandContext(ctx, p.cfg.PlaybackCmd)
		cmd.Stdin = bytes.NewReader(audio)
		cmd.Stderr = &stderr
	}

	debuglog.Printf("playback cmd=%s audio_bytes=%d", p.cfg.PlaybackCmd, len(audio))

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return fmt.Errorf("playback failed: %w: %s", err, stderr.String())
	}

	debuglog.Printf("playback complete")
	return nil
}

// compile-time check
var _ ports.TextToSpeech = (*Provider)(nil)

// SynthesizeViaPipe is an alternative synthesize implementation that pipes
// text to edge-tts via stdin instead of command arguments, useful for very
// long text that may exceed shell argument limits.
func (p *Provider) SynthesizeViaPipe(ctx context.Context, text string) ([]byte, error) {
	if text == "" {
		return nil, nil
	}

	args := []string{
		"--voice", p.cfg.Voice,
		"--rate", p.cfg.Rate,
		"--volume", p.cfg.Volume,
		"--write-media", "-",
	}

	cmd := exec.CommandContext(ctx, p.cfg.Command, args...)
	cmd.Stdin = bytes.NewReader([]byte(text))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	debuglog.Printf("edge-tts synthesize-pipe voice=%s text_len=%d", p.cfg.Voice, len(text))

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("edge-tts pipe failed: %w: %s", err, stderr.String())
	}

	audio := stdout.Bytes()
	if len(audio) == 0 {
		return nil, errors.New("edge-tts produced no audio output")
	}

	return audio, nil
}

// IsAvailable checks whether the edge-tts binary is in PATH.
func (p *Provider) IsAvailable() bool {
	_, err := exec.LookPath(p.cfg.Command)
	return err == nil
}

// ReadAll is a helper for reading all audio from an io.Reader.
func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
