package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesRulesFallbackOrder(t *testing.T) {
	home := t.TempDir()
	coldmicRules := filepath.Join(home, ".config", "coldmic", "substitutions.rules")
	hyprRules := filepath.Join(home, ".config", "hypr", "whisper-substitutions.rules")

	if err := os.MkdirAll(filepath.Dir(hyprRules), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(hyprRules, []byte("a => b\n"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("COLDMIC_RULES_FILE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Rules.Path != hyprRules {
		t.Fatalf("expected hypr fallback, got %q", cfg.Rules.Path)
	}

	if err := os.MkdirAll(filepath.Dir(coldmicRules), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(coldmicRules, []byte("a => c\n"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg2.Rules.Path != coldmicRules {
		t.Fatalf("expected coldmic rules priority, got %q", cfg2.Rules.Path)
	}
}

func TestLoadRespectsOverridesAndFallbacks(t *testing.T) {
	home := t.TempDir()
	rules := filepath.Join(home, "my.rules")
	if err := os.WriteFile(rules, []byte("x => y\n"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("DEEPGRAM_API_KEY", "test-key")
	t.Setenv("DEEPGRAM_API_BASE", "https://example.com/v1")
	t.Setenv("DEEPGRAM_MODEL", "nova-3")
	t.Setenv("DEEPGRAM_LANGUAGE", "en")
	t.Setenv("DEEPGRAM_SMART_FORMAT", "false")
	t.Setenv("COLDMIC_FFMPEG_COMMAND", "my-ffmpeg")
	t.Setenv("COLDMIC_AUDIO_INPUT_FORMAT", "alsa")
	t.Setenv("COLDMIC_AUDIO_INPUT_DEVICE", "mic0")
	t.Setenv("COLDMIC_SAMPLE_RATE", "22050")
	t.Setenv("COLDMIC_CHANNELS", "2")
	t.Setenv("COLDMIC_RULES_FILE", rules)
	t.Setenv("COLDMIC_RULE_ITERATION_LIMIT", "42")
	t.Setenv("COLDMIC_AUDIO_CHUNK_SIZE", "512")
	t.Setenv("COLDMIC_STREAMING_GRACE_MS", "25")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Deepgram.APIKey != "test-key" || cfg.Deepgram.APIBaseURL != "https://example.com/v1" {
		t.Fatalf("unexpected deepgram config: %+v", cfg.Deepgram)
	}
	if cfg.Deepgram.Model != "nova-3" || cfg.Deepgram.Language != "en" || cfg.Deepgram.SmartFormat {
		t.Fatalf("unexpected deepgram model/language/smart format: %+v", cfg.Deepgram)
	}
	if cfg.Audio.RecorderCommand != "my-ffmpeg" || cfg.Audio.InputFormat != "alsa" || cfg.Audio.InputDevice != "mic0" {
		t.Fatalf("unexpected audio config: %+v", cfg.Audio)
	}
	if cfg.Audio.SampleRate != 22050 || cfg.Audio.Channels != 2 {
		t.Fatalf("unexpected sample/channels: %+v", cfg.Audio)
	}
	if cfg.Rules.Path != rules || cfg.Rules.IterationLimit != 42 {
		t.Fatalf("unexpected rules config: %+v", cfg.Rules)
	}
	if cfg.Session.ChunkSize != 512 || cfg.Session.StreamingGrace != 25*time.Millisecond {
		t.Fatalf("unexpected session config: %+v", cfg.Session)
	}
}

func TestLoadInvalidNumericValuesFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COLDMIC_SAMPLE_RATE", "bad")
	t.Setenv("COLDMIC_CHANNELS", "-1")
	t.Setenv("COLDMIC_RULE_ITERATION_LIMIT", "0")
	t.Setenv("COLDMIC_AUDIO_CHUNK_SIZE", "5")
	t.Setenv("COLDMIC_STREAMING_GRACE_MS", "bad")
	t.Setenv("DEEPGRAM_SMART_FORMAT", "not-bool")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Audio.SampleRate != 16000 {
		t.Fatalf("expected default sample rate, got %d", cfg.Audio.SampleRate)
	}
	if cfg.Audio.Channels != 1 {
		t.Fatalf("expected default channels, got %d", cfg.Audio.Channels)
	}
	if cfg.Rules.IterationLimit != 30 {
		t.Fatalf("expected default iteration limit, got %d", cfg.Rules.IterationLimit)
	}
	if cfg.Session.ChunkSize != 4096 {
		t.Fatalf("expected chunk size fallback, got %d", cfg.Session.ChunkSize)
	}
	if cfg.Session.StreamingGrace != time.Second {
		t.Fatalf("expected default grace, got %s", cfg.Session.StreamingGrace)
	}
	if !cfg.Deepgram.SmartFormat {
		t.Fatalf("expected default smart format true")
	}
}
