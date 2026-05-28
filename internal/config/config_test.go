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

func TestLoadContinuousConfigDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Continuous.VADEngine != "silero" {
		t.Fatalf("expected default VADEngine=silero, got %q", cfg.Continuous.VADEngine)
	}
	if cfg.Continuous.VADThreshold != 500 {
		t.Fatalf("expected default VADThreshold=500, got %v", cfg.Continuous.VADThreshold)
	}
	if cfg.Continuous.SilenceMs != 800 {
		t.Fatalf("expected default SilenceMs=800, got %d", cfg.Continuous.SilenceMs)
	}
	if cfg.Continuous.FrameMs != 30 {
		t.Fatalf("expected default FrameMs=30, got %d", cfg.Continuous.FrameMs)
	}
	if len(cfg.Continuous.WakePhrases) != 2 || cfg.Continuous.WakePhrases[0] != "hey alice" {
		t.Fatalf("expected default wake phrases, got %v", cfg.Continuous.WakePhrases)
	}
}

func TestLoadContinuousConfigOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COLDMIC_VAD_ENGINE", "energy")
	t.Setenv("COLDMIC_VAD_THRESHOLD", "250")
	t.Setenv("COLDMIC_VAD_SILENCE_MS", "500")
	t.Setenv("COLDMIC_VAD_FRAME_MS", "48")
	t.Setenv("COLDMIC_WAKE_PHRASES", "hey computer,ok google")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Continuous.VADEngine != "energy" {
		t.Fatalf("expected VADEngine=energy, got %q", cfg.Continuous.VADEngine)
	}
	if cfg.Continuous.VADThreshold != 250 {
		t.Fatalf("expected VADThreshold=250, got %v", cfg.Continuous.VADThreshold)
	}
	if cfg.Continuous.SilenceMs != 500 {
		t.Fatalf("expected SilenceMs=500, got %d", cfg.Continuous.SilenceMs)
	}
	if cfg.Continuous.FrameMs != 48 {
		t.Fatalf("expected FrameMs=48, got %d", cfg.Continuous.FrameMs)
	}
	if len(cfg.Continuous.WakePhrases) != 2 || cfg.Continuous.WakePhrases[0] != "hey computer" {
		t.Fatalf("expected custom wake phrases, got %v", cfg.Continuous.WakePhrases)
	}
}

func TestLoadContinuousConfigInvalidValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COLDMIC_VAD_THRESHOLD", "not-a-number")
	t.Setenv("COLDMIC_VAD_SILENCE_MS", "bad")
	t.Setenv("COLDMIC_VAD_FRAME_MS", "bad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	// Should fall back to defaults for invalid values.
	if cfg.Continuous.VADThreshold != 500 {
		t.Fatalf("expected default VADThreshold for bad input, got %v", cfg.Continuous.VADThreshold)
	}
	if cfg.Continuous.SilenceMs != 800 {
		t.Fatalf("expected default SilenceMs for bad input, got %d", cfg.Continuous.SilenceMs)
	}
	if cfg.Continuous.FrameMs != 30 {
		t.Fatalf("expected default FrameMs for bad input, got %d", cfg.Continuous.FrameMs)
	}
}

func TestLoadTTSConfigDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.TTS.Engine != "edge-tts" {
		t.Fatalf("expected default TTS engine, got %q", cfg.TTS.Engine)
	}
	if cfg.TTS.Voice != "en-US-AriaNeural" {
		t.Fatalf("expected default TTS voice, got %q", cfg.TTS.Voice)
	}
	if cfg.TTS.PlaybackCmd != "ffplay" {
		t.Fatalf("expected default playback cmd, got %q", cfg.TTS.PlaybackCmd)
	}
}

func TestLoadConversationConfigDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Conversation.Provider != "openai" {
		t.Fatalf("expected default provider openai, got %q", cfg.Conversation.Provider)
	}
	if cfg.Conversation.MaxHistory != 20 {
		t.Fatalf("expected default MaxHistory=20, got %d", cfg.Conversation.MaxHistory)
	}
}

func TestEnvOrDefaultDuration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COLDMIC_CONVERSATION_TIMEOUT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Conversation.SilenceTimeout != 10*time.Second {
		t.Fatalf("expected 10s timeout, got %v", cfg.Conversation.SilenceTimeout)
	}
}

func TestEnvOrDefaultDurationInvalid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COLDMIC_CONVERSATION_TIMEOUT", "not-a-duration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.Conversation.SilenceTimeout != 30*time.Second {
		t.Fatalf("expected default 30s timeout for bad input, got %v", cfg.Conversation.SilenceTimeout)
	}
}
