package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadConfigFileAndEnvPrecedence(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "coldmic", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	data := []byte(`
deepgram:
  model: nova-file
  smart_format: false
audio:
  input_device: file-mic
  sample_rate: 44100
session:
  streaming_grace: "250ms"
conversation:
  timeout: "45s"
  stop_phrases: ["enough", "stop now"]
continuous:
  wake_phrases: ["hey computer"]
  vad_engine: energy
  vad_threshold: 350
tts:
  voice: "en-US-JennyNeural"
daemon:
  url: "http://127.0.0.1:5555"
  toggle_compat: true
  debug: true
`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("DEEPGRAM_MODEL", "nova-env")
	t.Setenv("COLDMIC_AUDIO_INPUT_DEVICE", "env-mic")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Deepgram.Model != "nova-env" {
		t.Fatalf("expected env to override file model, got %q", cfg.Deepgram.Model)
	}
	if cfg.Audio.InputDevice != "env-mic" {
		t.Fatalf("expected env to override file input device, got %q", cfg.Audio.InputDevice)
	}
	if cfg.Audio.SampleRate != 44100 || cfg.Session.StreamingGrace != 250*time.Millisecond {
		t.Fatalf("expected file config values, got audio=%+v session=%+v", cfg.Audio, cfg.Session)
	}
	if cfg.Conversation.Timeout != 45*time.Second || cfg.Conversation.StopPhrases[0] != "enough" {
		t.Fatalf("expected conversation values from file, got %+v", cfg.Conversation)
	}
	if cfg.Continuous.VADEngine != "energy" || cfg.Continuous.WakePhrases[0] != "hey computer" {
		t.Fatalf("expected continuous values from file, got %+v", cfg.Continuous)
	}
	if cfg.Daemon.URL != "http://127.0.0.1:5555" || !cfg.Daemon.ToggleCompat || !cfg.Daemon.Debug {
		t.Fatalf("expected daemon values from file, got %+v", cfg.Daemon)
	}
}

func TestLoadInvalidConfigFileReturnsHumanReadableError(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "coldmic", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("conversation:\n  timeout: soon\n"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	t.Setenv("HOME", home)

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid config error")
	}
	if !strings.Contains(err.Error(), "conversation.timeout") {
		t.Fatalf("expected field-specific error, got %v", err)
	}
}

func TestLoadUnknownConfigFieldReturnsHumanReadableError(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "coldmic", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("audio:\n  sample_rte: 16000\n"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	t.Setenv("HOME", home)

	_, err := Load()
	if err == nil {
		t.Fatalf("expected unknown field error")
	}
	if !strings.Contains(err.Error(), "sample_rte") {
		t.Fatalf("expected unknown field name in error, got %v", err)
	}
}

func TestLoadExplicitMissingConfigFileReturnsError(t *testing.T) {
	home := t.TempDir()
	missing := filepath.Join(home, "missing.yaml")
	t.Setenv("HOME", home)
	t.Setenv("COLDMIC_CONFIG_FILE", missing)

	_, err := Load()
	if err == nil {
		t.Fatalf("expected missing explicit config file error")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("expected missing path in error, got %v", err)
	}
}

func TestWriteTemplateCreatesDefaultConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("default path failed: %v", err)
	}
	if err := WriteTemplate(""); err != nil {
		t.Fatalf("write template failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected template at %s: %v", path, err)
	}
}

func TestLoadInvalidEnvValuesReturnHumanReadableError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COLDMIC_SAMPLE_RATE", "bad")
	t.Setenv("COLDMIC_CHANNELS", "-1")
	t.Setenv("COLDMIC_RULE_ITERATION_LIMIT", "0")
	t.Setenv("COLDMIC_AUDIO_CHUNK_SIZE", "5")
	t.Setenv("COLDMIC_STREAMING_GRACE_MS", "bad")
	t.Setenv("DEEPGRAM_SMART_FORMAT", "not-bool")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid environment config error")
	}
	for _, want := range []string{"COLDMIC_SAMPLE_RATE", "COLDMIC_CHANNELS", "COLDMIC_RULE_ITERATION_LIMIT", "COLDMIC_AUDIO_CHUNK_SIZE", "COLDMIC_STREAMING_GRACE_MS", "DEEPGRAM_SMART_FORMAT"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %s in error, got %v", want, err)
		}
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
	if cfg.Continuous.VADThreshold != 0.5 {
		t.Fatalf("expected default VADThreshold=0.5, got %v", cfg.Continuous.VADThreshold)
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

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid environment config error")
	}
	for _, want := range []string{"COLDMIC_VAD_THRESHOLD", "COLDMIC_VAD_SILENCE_MS", "COLDMIC_VAD_FRAME_MS"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %s in error, got %v", want, err)
		}
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
		t.Fatalf("expected MaxHistory=20, got %d", cfg.Conversation.MaxHistory)
	}
}

func TestSetAudioInputDeviceCreatesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := SetAudioInputDevice(path, "usb-mic"); err != nil {
		t.Fatalf("SetAudioInputDevice failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if !strings.Contains(string(data), "input_device: usb-mic") {
		t.Fatalf("expected persisted input device, got:\n%s", string(data))
	}
}

func TestSetAudioInputDevicePreservesOtherConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := []byte("deepgram:\n  model: nova-3\naudio:\n  input_format: pulse\n")
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	if err := SetAudioInputDevice(path, "alsa_input.usb"); err != nil {
		t.Fatalf("SetAudioInputDevice failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "model: nova-3") || !strings.Contains(text, "input_format: pulse") || !strings.Contains(text, "input_device: alsa_input.usb") {
		t.Fatalf("expected preserved config and input device, got:\n%s", text)
	}
}

func TestSetAudioInputDeviceRejectsBlankSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := SetAudioInputDevice(path, "   ")
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("expected blank device error, got %v", err)
	}
}

func TestSetAudioInputDeviceRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("audio: ["), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	err := SetAudioInputDevice(path, "usb-mic")
	if err == nil || !strings.Contains(err.Error(), "parse config file") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestMarshalYAMLAndRedactSecrets(t *testing.T) {
	cfg := Config{
		Deepgram:     DeepgramConfig{APIKey: "deep-secret", APIBaseURL: "https://example.com", Model: "nova"},
		Conversation: ConversationConfig{APIKey: "chat-secret", Provider: "openai", BaseURL: "https://chat.example.com", Model: "gpt", Timeout: time.Second, SilenceTimeout: time.Second},
		Audio:        AudioConfig{RecorderCommand: "ffmpeg", SampleRate: 16000, Channels: 1},
		Rules:        RulesConfig{IterationLimit: 1},
		Continuous:   ContinuousConfig{VADEngine: "silero", VADThreshold: 500, SilenceMs: 800, FrameMs: 30},
		TTS:          TTSConfig{Engine: "edge-tts", PlaybackCmd: "ffplay"},
	}

	redacted := RedactSecrets(cfg)
	if redacted.Deepgram.APIKey != "[redacted]" || redacted.Conversation.APIKey != "[redacted]" {
		t.Fatalf("expected secrets redacted, got deepgram=%q conversation=%q", redacted.Deepgram.APIKey, redacted.Conversation.APIKey)
	}
	data, err := MarshalYAML(redacted)
	if err != nil {
		t.Fatalf("MarshalYAML failed: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "api_key: '[redacted]'") || strings.Contains(text, "deep-secret") || strings.Contains(text, "chat-secret") {
		t.Fatalf("expected redacted YAML, got:\n%s", text)
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

	_, err := Load()
	if err == nil {
		t.Fatalf("expected invalid environment config error")
	}
	if !strings.Contains(err.Error(), "COLDMIC_CONVERSATION_TIMEOUT") {
		t.Fatalf("expected timeout env key in error, got %v", err)
	}
}
