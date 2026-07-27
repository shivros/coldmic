package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const configPathEnv = "COLDMIC_CONFIG_FILE"

// Config stores runtime configuration for the tracer bullet.
type Config struct {
	Deepgram     DeepgramConfig     `json:"deepgram" yaml:"deepgram"`
	Audio        AudioConfig        `json:"audio" yaml:"audio"`
	Rules        RulesConfig        `json:"rules" yaml:"rules"`
	Session      SessionConfig      `json:"session" yaml:"session"`
	Conversation ConversationConfig `json:"conversation" yaml:"conversation"`
	Continuous   ContinuousConfig   `json:"continuous" yaml:"continuous"`
	TTS          TTSConfig          `json:"tts" yaml:"tts"`
	Daemon       DaemonConfig       `json:"daemon" yaml:"daemon"`
}

type DaemonConfig struct {
	Addr         string `json:"addr" yaml:"addr"`
	URL          string `json:"url" yaml:"url"`
	ToggleCompat bool   `json:"toggleCompat" yaml:"toggle_compat"`
	Debug        bool   `json:"debug" yaml:"debug"`
}

type DeepgramConfig struct {
	APIKey      string `json:"apiKey" yaml:"api_key"`
	APIBaseURL  string `json:"apiBaseUrl" yaml:"api_base_url"`
	Model       string `json:"model" yaml:"model"`
	Language    string `json:"language" yaml:"language"`
	SmartFormat bool   `json:"smartFormat" yaml:"smart_format"`
}

type AudioConfig struct {
	RecorderCommand string `json:"recorderCommand" yaml:"recorder_command"`
	InputFormat     string `json:"inputFormat" yaml:"input_format"`
	InputDevice     string `json:"inputDevice" yaml:"input_device"`
	SampleRate      int    `json:"sampleRate" yaml:"sample_rate"`
	Channels        int    `json:"channels" yaml:"channels"`
}

type RulesConfig struct {
	Path           string `json:"path" yaml:"path"`
	IterationLimit int    `json:"iterationLimit" yaml:"iteration_limit"`
}

type SessionConfig struct {
	ChunkSize      int           `json:"chunkSize" yaml:"chunk_size"`
	StreamingGrace time.Duration `json:"streamingGrace" yaml:"streaming_grace"`
}

type ConversationConfig struct {
	Provider       string        `json:"provider" yaml:"provider"`
	BaseURL        string        `json:"baseUrl" yaml:"base_url"`
	APIKey         string        `json:"apiKey" yaml:"api_key"`
	Model          string        `json:"model" yaml:"model"`
	SystemPrompt   string        `json:"systemPrompt" yaml:"system_prompt"`
	Stream         bool          `json:"stream" yaml:"stream"`
	Timeout        time.Duration `json:"timeout" yaml:"timeout"`
	MaxHistory     int           `json:"maxHistory" yaml:"max_history"`
	StopPhrases    []string      `json:"stopPhrases" yaml:"stop_phrases"`
	SilenceTimeout time.Duration `json:"silenceTimeout" yaml:"silence_timeout"`
}

type ContinuousConfig struct {
	WakePhrases  []string       `json:"wakePhrases" yaml:"wake_phrases"`
	VADEngine    string         `json:"vadEngine" yaml:"vad_engine"`       // "silero" (default) or "energy"
	VADThreshold float64        `json:"vadThreshold" yaml:"vad_threshold"` // Used by EnergyVAD (RMS threshold) and SileroVAD (speech probability threshold)
	SilenceMs    int            `json:"silenceMs" yaml:"silence_ms"`
	FrameMs      int            `json:"frameMs" yaml:"frame_ms"`
	LocalSTT     LocalSTTConfig `json:"localStt" yaml:"local_stt"`
}

type LocalSTTConfig struct {
	Provider string `json:"provider" yaml:"provider"` // "whispercpp" or empty (disabled)
	Model    string `json:"model" yaml:"model"`       // whisper model filename, e.g. "ggml-tiny.en.bin"
}

type TTSConfig struct {
	Engine      string `json:"engine" yaml:"engine"`
	Voice       string `json:"voice" yaml:"voice"`
	Rate        string `json:"rate" yaml:"rate"`
	Volume      string `json:"volume" yaml:"volume"`
	PlaybackCmd string `json:"playbackCmd" yaml:"playback_cmd"`
}

// DefaultPath returns the default YAML configuration path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("could not determine home directory")
	}
	if override := strings.TrimSpace(os.Getenv(configPathEnv)); override != "" {
		return override, nil
	}
	return filepath.Join(home, ".config", "coldmic", "config.yaml"), nil
}

// Load resolves configuration from defaults, YAML config, and environment variables.
// Precedence is: environment variables > config file > defaults. CLI flags are applied
// by individual command parsers above the returned Config where those flags exist.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, errors.New("could not determine home directory")
	}

	cfg := defaults(home)

	configPath, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	if err := applyConfigFile(&cfg, configPath); err != nil {
		return Config{}, err
	}
	applyEnv(&cfg)
	if err := validateEnvOverrides(); err != nil {
		return Config{}, err
	}
	normalize(&cfg)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// WriteTemplate writes a documented YAML template to path. Existing files are not overwritten.
func WriteTemplate(path string) error {
	if strings.TrimSpace(path) == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return err
		}
		path = defaultPath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("config file already exists: %s", path)
		}
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(templateYAML)
	if err != nil {
		return fmt.Errorf("write config template: %w", err)
	}
	return nil
}

// SetAudioInputDevice persists the selected audio input device in the YAML config.
func SetAudioInputDevice(path string, device string) error {
	device = strings.TrimSpace(device)
	if device == "" {
		return errors.New("audio input device must not be empty")
	}
	if strings.TrimSpace(path) == "" {
		defaultPath, err := DefaultPath()
		if err != nil {
			return err
		}
		path = defaultPath
	}

	root := yaml.Node{Kind: yaml.MappingNode}
	if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse config file %s: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config file %s: %w", path, err)
	}

	if root.Kind == 0 {
		root.Kind = yaml.MappingNode
	}
	mapping := &root
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.MappingNode})
		}
		mapping = root.Content[0]
	}
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("config file %s must contain a YAML mapping", path)
	}
	setNestedString(mapping, []string{"audio", "input_device"}, device)

	data, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file %s: %w", path, err)
	}
	return nil
}

// MarshalYAML renders a resolved config as documented YAML.
func MarshalYAML(cfg Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// RedactSecrets returns a copy safe to print in terminals and issue comments.
func RedactSecrets(cfg Config) Config {
	redacted := cfg
	if redacted.Deepgram.APIKey != "" {
		redacted.Deepgram.APIKey = "[redacted]"
	}
	if redacted.Conversation.APIKey != "" {
		redacted.Conversation.APIKey = "[redacted]"
	}
	return redacted
}

// Validate checks resolved configuration for values that would make startup fail later.
func Validate(cfg Config) error {
	var problems []string
	if cfg.Deepgram.APIBaseURL == "" {
		problems = append(problems, "deepgram.api_base_url must not be empty")
	}
	if cfg.Audio.RecorderCommand == "" {
		problems = append(problems, "audio.recorder_command must not be empty")
	}
	if cfg.Audio.SampleRate <= 0 {
		problems = append(problems, "audio.sample_rate must be greater than 0")
	}
	if cfg.Audio.Channels <= 0 {
		problems = append(problems, "audio.channels must be greater than 0")
	}
	if cfg.Rules.IterationLimit <= 0 {
		problems = append(problems, "rules.iteration_limit must be greater than 0")
	}
	if cfg.Session.ChunkSize < 256 {
		problems = append(problems, "session.chunk_size must be at least 256")
	}
	if cfg.Conversation.Provider == "" {
		problems = append(problems, "conversation.provider must not be empty")
	}
	if cfg.Conversation.BaseURL == "" {
		problems = append(problems, "conversation.base_url must not be empty")
	}
	if cfg.Conversation.Model == "" {
		problems = append(problems, "conversation.model must not be empty")
	}
	if cfg.Conversation.Timeout <= 0 {
		problems = append(problems, "conversation.timeout must be greater than 0")
	}
	if cfg.Conversation.MaxHistory < 0 {
		problems = append(problems, "conversation.max_history must be 0 or greater")
	}
	if cfg.Conversation.SilenceTimeout <= 0 {
		problems = append(problems, "conversation.silence_timeout must be greater than 0")
	}
	if cfg.Continuous.VADEngine != "silero" && cfg.Continuous.VADEngine != "energy" {
		problems = append(problems, "continuous.vad_engine must be one of: silero, energy")
	}
	if cfg.Continuous.VADThreshold <= 0 {
		problems = append(problems, "continuous.vad_threshold must be greater than 0")
	}
	if cfg.Continuous.SilenceMs <= 0 {
		problems = append(problems, "continuous.silence_ms must be greater than 0")
	}
	if cfg.Continuous.FrameMs <= 0 {
		problems = append(problems, "continuous.frame_ms must be greater than 0")
	}
	if cfg.TTS.Engine == "" {
		problems = append(problems, "tts.engine must not be empty")
	}
	if cfg.TTS.PlaybackCmd == "" {
		problems = append(problems, "tts.playback_cmd must not be empty")
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(problems, "; "))
	}
	return nil
}

func defaults(home string) Config {
	defaultRules := filepath.Join(home, ".config", "coldmic", "substitutions.rules")
	hyprRules := filepath.Join(home, ".config", "hypr", "whisper-substitutions.rules")
	return Config{
		Deepgram: DeepgramConfig{
			APIBaseURL:  "https://api.deepgram.com/v1",
			Model:       "nova-2",
			SmartFormat: true,
		},
		Audio: AudioConfig{
			RecorderCommand: "ffmpeg",
			InputFormat:     "pulse",
			InputDevice:     "default",
			SampleRate:      16000,
			Channels:        1,
		},
		Rules: RulesConfig{
			Path:           firstExisting(defaultRules, hyprRules),
			IterationLimit: 30,
		},
		Session: SessionConfig{
			ChunkSize:      4096,
			StreamingGrace: time.Second,
		},
		Conversation: ConversationConfig{
			Provider:       "openai",
			BaseURL:        "https://api.openai.com/v1",
			Model:          "gpt-4o",
			SystemPrompt:   "You are a helpful voice assistant.",
			Stream:         true,
			Timeout:        30 * time.Second,
			MaxHistory:     20,
			StopPhrases:    parseStopPhrases("thanks alice,that's all,goodbye,bye alice,stop"),
			SilenceTimeout: 30 * time.Second,
		},
		Continuous: ContinuousConfig{
			WakePhrases:  parseWakePhrases("hey alice,alice"),
			VADEngine:    "silero",
			VADThreshold: 500,
			SilenceMs:    800,
			FrameMs:      30,
			LocalSTT: LocalSTTConfig{
				Model: "ggml-tiny.en.bin",
			},
		},
		TTS: TTSConfig{
			Engine:      "edge-tts",
			Voice:       "en-US-AriaNeural",
			Rate:        "+0%",
			Volume:      "+0%",
			PlaybackCmd: "ffplay",
		},
		Daemon: DaemonConfig{
			Addr: "127.0.0.1:4317",
			URL:  "http://127.0.0.1:4317",
		},
	}
}

type fileConfig struct {
	Deepgram     *fileDeepgram     `yaml:"deepgram"`
	Audio        *fileAudio        `yaml:"audio"`
	Rules        *fileRules        `yaml:"rules"`
	Session      *fileSession      `yaml:"session"`
	Conversation *fileConversation `yaml:"conversation"`
	Continuous   *fileContinuous   `yaml:"continuous"`
	TTS          *fileTTS          `yaml:"tts"`
	Daemon       *fileDaemon       `yaml:"daemon"`
}

type fileDaemon struct {
	Addr         *string `yaml:"addr"`
	URL          *string `yaml:"url"`
	ToggleCompat *bool   `yaml:"toggle_compat"`
	Debug        *bool   `yaml:"debug"`
}

type fileDeepgram struct {
	APIKey      *string `yaml:"api_key"`
	APIBaseURL  *string `yaml:"api_base_url"`
	Model       *string `yaml:"model"`
	Language    *string `yaml:"language"`
	SmartFormat *bool   `yaml:"smart_format"`
}

type fileAudio struct {
	RecorderCommand *string `yaml:"recorder_command"`
	InputFormat     *string `yaml:"input_format"`
	InputDevice     *string `yaml:"input_device"`
	SampleRate      *int    `yaml:"sample_rate"`
	Channels        *int    `yaml:"channels"`
}

type fileRules struct {
	Path           *string `yaml:"path"`
	IterationLimit *int    `yaml:"iteration_limit"`
}

type fileSession struct {
	ChunkSize      *int    `yaml:"chunk_size"`
	StreamingGrace *string `yaml:"streaming_grace"`
}

type fileConversation struct {
	Provider       *string  `yaml:"provider"`
	BaseURL        *string  `yaml:"base_url"`
	APIKey         *string  `yaml:"api_key"`
	Model          *string  `yaml:"model"`
	SystemPrompt   *string  `yaml:"system_prompt"`
	Stream         *bool    `yaml:"stream"`
	Timeout        *string  `yaml:"timeout"`
	MaxHistory     *int     `yaml:"max_history"`
	StopPhrases    []string `yaml:"stop_phrases"`
	SilenceTimeout *string  `yaml:"silence_timeout"`
}

type fileContinuous struct {
	WakePhrases  []string      `yaml:"wake_phrases"`
	VADEngine    *string       `yaml:"vad_engine"`
	VADThreshold *float64      `yaml:"vad_threshold"`
	SilenceMs    *int          `yaml:"silence_ms"`
	FrameMs      *int          `yaml:"frame_ms"`
	LocalSTT     *fileLocalSTT `yaml:"local_stt"`
}

type fileLocalSTT struct {
	Provider *string `yaml:"provider"`
	Model    *string `yaml:"model"`
}

type fileTTS struct {
	Engine      *string `yaml:"engine"`
	Voice       *string `yaml:"voice"`
	Rate        *string `yaml:"rate"`
	Volume      *string `yaml:"volume"`
	PlaybackCmd *string `yaml:"playback_cmd"`
}

func applyConfigFile(cfg *Config, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if strings.TrimSpace(os.Getenv(configPathEnv)) != "" {
				return fmt.Errorf("config file %s does not exist", path)
			}
			return nil
		}
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	var fc fileConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&fc); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	return applyFileConfig(cfg, fc)
}

func applyFileConfig(cfg *Config, fc fileConfig) error {
	if fc.Deepgram != nil {
		setString(fc.Deepgram.APIKey, &cfg.Deepgram.APIKey)
		setString(fc.Deepgram.APIBaseURL, &cfg.Deepgram.APIBaseURL)
		setString(fc.Deepgram.Model, &cfg.Deepgram.Model)
		setString(fc.Deepgram.Language, &cfg.Deepgram.Language)
		setBool(fc.Deepgram.SmartFormat, &cfg.Deepgram.SmartFormat)
	}
	if fc.Audio != nil {
		setString(fc.Audio.RecorderCommand, &cfg.Audio.RecorderCommand)
		setString(fc.Audio.InputFormat, &cfg.Audio.InputFormat)
		setString(fc.Audio.InputDevice, &cfg.Audio.InputDevice)
		setInt(fc.Audio.SampleRate, &cfg.Audio.SampleRate)
		setInt(fc.Audio.Channels, &cfg.Audio.Channels)
	}
	if fc.Rules != nil {
		if fc.Rules.Path != nil && strings.TrimSpace(*fc.Rules.Path) != "" {
			cfg.Rules.Path = expandUserPath(strings.TrimSpace(*fc.Rules.Path))
		}
		setInt(fc.Rules.IterationLimit, &cfg.Rules.IterationLimit)
	}
	if fc.Session != nil {
		setInt(fc.Session.ChunkSize, &cfg.Session.ChunkSize)
		if fc.Session.StreamingGrace != nil {
			d, err := time.ParseDuration(*fc.Session.StreamingGrace)
			if err != nil {
				return fmt.Errorf("session.streaming_grace must be a duration like 1s or 250ms: %w", err)
			}
			cfg.Session.StreamingGrace = d
		}
	}
	if fc.Conversation != nil {
		setString(fc.Conversation.Provider, &cfg.Conversation.Provider)
		setString(fc.Conversation.BaseURL, &cfg.Conversation.BaseURL)
		setString(fc.Conversation.APIKey, &cfg.Conversation.APIKey)
		setString(fc.Conversation.Model, &cfg.Conversation.Model)
		setString(fc.Conversation.SystemPrompt, &cfg.Conversation.SystemPrompt)
		setBool(fc.Conversation.Stream, &cfg.Conversation.Stream)
		if fc.Conversation.Timeout != nil {
			d, err := time.ParseDuration(*fc.Conversation.Timeout)
			if err != nil {
				return fmt.Errorf("conversation.timeout must be a duration like 30s: %w", err)
			}
			cfg.Conversation.Timeout = d
		}
		setInt(fc.Conversation.MaxHistory, &cfg.Conversation.MaxHistory)
		if fc.Conversation.StopPhrases != nil {
			cfg.Conversation.StopPhrases = normalizePhrases(fc.Conversation.StopPhrases)
		}
		if fc.Conversation.SilenceTimeout != nil {
			d, err := time.ParseDuration(*fc.Conversation.SilenceTimeout)
			if err != nil {
				return fmt.Errorf("conversation.silence_timeout must be a duration like 30s: %w", err)
			}
			cfg.Conversation.SilenceTimeout = d
		}
	}
	if fc.Continuous != nil {
		if fc.Continuous.WakePhrases != nil {
			cfg.Continuous.WakePhrases = normalizePhrases(fc.Continuous.WakePhrases)
		}
		setString(fc.Continuous.VADEngine, &cfg.Continuous.VADEngine)
		setFloat(fc.Continuous.VADThreshold, &cfg.Continuous.VADThreshold)
		setInt(fc.Continuous.SilenceMs, &cfg.Continuous.SilenceMs)
		setInt(fc.Continuous.FrameMs, &cfg.Continuous.FrameMs)
		if fc.Continuous.LocalSTT != nil {
			setString(fc.Continuous.LocalSTT.Provider, &cfg.Continuous.LocalSTT.Provider)
			setString(fc.Continuous.LocalSTT.Model, &cfg.Continuous.LocalSTT.Model)
		}
	}
	if fc.TTS != nil {
		setString(fc.TTS.Engine, &cfg.TTS.Engine)
		setString(fc.TTS.Voice, &cfg.TTS.Voice)
		setString(fc.TTS.Rate, &cfg.TTS.Rate)
		setString(fc.TTS.Volume, &cfg.TTS.Volume)
		setString(fc.TTS.PlaybackCmd, &cfg.TTS.PlaybackCmd)
	}
	if fc.Daemon != nil {
		setString(fc.Daemon.Addr, &cfg.Daemon.Addr)
		setString(fc.Daemon.URL, &cfg.Daemon.URL)
		setBool(fc.Daemon.ToggleCompat, &cfg.Daemon.ToggleCompat)
		setBool(fc.Daemon.Debug, &cfg.Daemon.Debug)
	}
	return nil
}

func applyEnv(cfg *Config) {
	setEnvString("DEEPGRAM_API_KEY", &cfg.Deepgram.APIKey)
	setEnvString("DEEPGRAM_API_BASE", &cfg.Deepgram.APIBaseURL)
	setEnvString("DEEPGRAM_MODEL", &cfg.Deepgram.Model)
	setEnvString("DEEPGRAM_LANGUAGE", &cfg.Deepgram.Language)
	setEnvBool("DEEPGRAM_SMART_FORMAT", &cfg.Deepgram.SmartFormat)

	setEnvString("COLDMIC_FFMPEG_COMMAND", &cfg.Audio.RecorderCommand)
	setEnvString("COLDMIC_AUDIO_INPUT_FORMAT", &cfg.Audio.InputFormat)
	setEnvFirstString([]string{"COLDMIC_AUDIO_INPUT_DEVICE", "DEEPGRAM_PULSE_SOURCE", "WHISPER_PULSE_SOURCE"}, &cfg.Audio.InputDevice)
	setEnvPositiveInt("COLDMIC_SAMPLE_RATE", &cfg.Audio.SampleRate, 1)
	setEnvPositiveInt("COLDMIC_CHANNELS", &cfg.Audio.Channels, 1)

	setEnvString("COLDMIC_RULES_FILE", &cfg.Rules.Path)
	setEnvPositiveInt("COLDMIC_RULE_ITERATION_LIMIT", &cfg.Rules.IterationLimit, 1)

	setEnvPositiveInt("COLDMIC_AUDIO_CHUNK_SIZE", &cfg.Session.ChunkSize, 256)
	setEnvFirstNonNegativeInt([]string{"COLDMIC_STREAMING_GRACE_MS", "DEEPGRAM_STREAMING_GRACE_MS"}, &cfg.Session.StreamingGrace)

	setEnvString("COLDMIC_CONVERSATION_BACKEND", &cfg.Conversation.Provider)
	setEnvString("COLDMIC_BACKEND_BASE_URL", &cfg.Conversation.BaseURL)
	setEnvString("COLDMIC_BACKEND_API_KEY", &cfg.Conversation.APIKey)
	setEnvString("COLDMIC_BACKEND_MODEL", &cfg.Conversation.Model)
	setEnvString("COLDMIC_BACKEND_SYSTEM_PROMPT", &cfg.Conversation.SystemPrompt)
	setEnvBool("COLDMIC_BACKEND_STREAM", &cfg.Conversation.Stream)
	setEnvDuration("COLDMIC_BACKEND_TIMEOUT", &cfg.Conversation.Timeout)
	setEnvNonNegativeInt("COLDMIC_BACKEND_MAX_HISTORY", &cfg.Conversation.MaxHistory)
	if raw := strings.TrimSpace(os.Getenv("COLDMIC_STOP_PHRASES")); raw != "" {
		cfg.Conversation.StopPhrases = parseStopPhrases(raw)
	}
	setEnvDuration("COLDMIC_CONVERSATION_TIMEOUT", &cfg.Conversation.SilenceTimeout)

	if raw := strings.TrimSpace(os.Getenv("COLDMIC_WAKE_PHRASES")); raw != "" {
		cfg.Continuous.WakePhrases = parseWakePhrases(raw)
	}
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("COLDMIC_VAD_ENGINE"))); value == "silero" || value == "energy" {
		cfg.Continuous.VADEngine = value
	}
	setEnvFloatPositive("COLDMIC_VAD_THRESHOLD", &cfg.Continuous.VADThreshold)
	setEnvPositiveInt("COLDMIC_VAD_SILENCE_MS", &cfg.Continuous.SilenceMs, 1)
	setEnvPositiveInt("COLDMIC_VAD_FRAME_MS", &cfg.Continuous.FrameMs, 1)
	setEnvString("COLDMIC_LOCAL_STT", &cfg.Continuous.LocalSTT.Provider)
	setEnvString("COLDMIC_LOCAL_STT_MODEL", &cfg.Continuous.LocalSTT.Model)

	setEnvString("COLDMIC_TTS_ENGINE", &cfg.TTS.Engine)
	setEnvString("COLDMIC_TTS_VOICE", &cfg.TTS.Voice)
	setEnvString("COLDMIC_TTS_RATE", &cfg.TTS.Rate)
	setEnvString("COLDMIC_TTS_VOLUME", &cfg.TTS.Volume)
	setEnvString("COLDMIC_TTS_PLAYBACK_CMD", &cfg.TTS.PlaybackCmd)

	setEnvString("COLDMIC_DAEMON_ADDR", &cfg.Daemon.Addr)
	setEnvString("COLDMIC_DAEMON_URL", &cfg.Daemon.URL)
	setEnvBool("COLDMIC_TOGGLE_COMPAT", &cfg.Daemon.ToggleCompat)
	setEnvBool("COLDMIC_DEBUG", &cfg.Daemon.Debug)
}

func normalize(cfg *Config) {
	cfg.Conversation.StopPhrases = normalizePhrases(cfg.Conversation.StopPhrases)
	cfg.Continuous.WakePhrases = normalizePhrases(cfg.Continuous.WakePhrases)
	cfg.Continuous.VADEngine = strings.ToLower(strings.TrimSpace(cfg.Continuous.VADEngine))
}

func setNestedString(root *yaml.Node, path []string, value string) {
	current := root
	for i, key := range path {
		if i == len(path)-1 {
			setMappingScalar(current, key, value)
			return
		}
		next := getMappingValue(current, key)
		if next == nil || next.Kind != yaml.MappingNode {
			next = &yaml.Node{Kind: yaml.MappingNode}
			setMappingNode(current, key, next)
		}
		current = next
	}
}

func setMappingScalar(mapping *yaml.Node, key string, value string) {
	setMappingNode(mapping, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func setMappingNode(mapping *yaml.Node, key string, value *yaml.Node) {
	if mapping.Kind != yaml.MappingNode {
		mapping.Kind = yaml.MappingNode
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func getMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func parseWakePhrases(raw string) []string { return normalizePhrases(strings.Split(raw, ",")) }

func parseStopPhrases(raw string) []string { return normalizePhrases(strings.Split(raw, ",")) }

func normalizePhrases(values []string) []string {
	var result []string
	for _, p := range values {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func setString(src *string, dst *string) {
	if src != nil {
		*dst = strings.TrimSpace(*src)
	}
}

func expandUserPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func setBool(src *bool, dst *bool) {
	if src != nil {
		*dst = *src
	}
}

func setInt(src *int, dst *int) {
	if src != nil {
		*dst = *src
	}
}

func setFloat(src *float64, dst *float64) {
	if src != nil {
		*dst = *src
	}
}

func setEnvString(key string, dst *string) {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		*dst = value
	}
}

func setEnvFirstString(keys []string, dst *string) {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			*dst = value
			return
		}
	}
}

func setEnvPositiveInt(key string, dst *int, minimum int) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err == nil && parsed >= minimum {
		*dst = parsed
	}
}

func setEnvNonNegativeInt(key string, dst *int) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err == nil && parsed >= 0 {
		*dst = parsed
	}
}

func setEnvBool(key string, dst *bool) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on", "debug":
		*dst = true
	case "0", "false", "no", "off":
		*dst = false
	}
}

func setEnvDuration(key string, dst *time.Duration) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	d, err := time.ParseDuration(value)
	if err == nil {
		*dst = d
	}
}

func setEnvFirstNonNegativeInt(keys []string, dst *time.Duration) {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed >= 0 {
			*dst = time.Duration(parsed) * time.Millisecond
			return
		}
	}
}

func setEnvFloatPositive(key string, dst *float64) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err == nil && parsed > 0 {
		*dst = parsed
	}
}

func validateEnvOverrides() error {
	var problems []string
	validateBoolEnv(&problems, "DEEPGRAM_SMART_FORMAT")
	validateBoolEnv(&problems, "COLDMIC_BACKEND_STREAM")
	validateBoolEnv(&problems, "COLDMIC_TOGGLE_COMPAT")
	validateBoolEnv(&problems, "COLDMIC_DEBUG")

	validatePositiveIntEnv(&problems, "COLDMIC_SAMPLE_RATE", 1)
	validatePositiveIntEnv(&problems, "COLDMIC_CHANNELS", 1)
	validatePositiveIntEnv(&problems, "COLDMIC_RULE_ITERATION_LIMIT", 1)
	validatePositiveIntEnv(&problems, "COLDMIC_AUDIO_CHUNK_SIZE", 256)
	validateNonNegativeIntEnv(&problems, "COLDMIC_BACKEND_MAX_HISTORY")
	validateNonNegativeIntEnv(&problems, "COLDMIC_STREAMING_GRACE_MS")
	validateNonNegativeIntEnv(&problems, "DEEPGRAM_STREAMING_GRACE_MS")
	validatePositiveIntEnv(&problems, "COLDMIC_VAD_SILENCE_MS", 1)
	validatePositiveIntEnv(&problems, "COLDMIC_VAD_FRAME_MS", 1)

	validateDurationEnv(&problems, "COLDMIC_BACKEND_TIMEOUT")
	validateDurationEnv(&problems, "COLDMIC_CONVERSATION_TIMEOUT")
	validatePositiveFloatEnv(&problems, "COLDMIC_VAD_THRESHOLD")
	validateEnumEnv(&problems, "COLDMIC_VAD_ENGINE", "silero", "energy")

	if len(problems) > 0 {
		return fmt.Errorf("invalid environment config: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validateBoolEnv(problems *[]string, key string) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return
	}
	switch value {
	case "1", "true", "yes", "on", "debug", "0", "false", "no", "off":
		return
	default:
		*problems = append(*problems, fmt.Sprintf("%s must be a boolean", key))
	}
}

func validatePositiveIntEnv(problems *[]string, key string, minimum int) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer >= %d", key, minimum))
	}
}

func validateNonNegativeIntEnv(problems *[]string, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		*problems = append(*problems, fmt.Sprintf("%s must be an integer >= 0", key))
	}
}

func validateDurationEnv(problems *[]string, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	if _, err := time.ParseDuration(value); err != nil {
		*problems = append(*problems, fmt.Sprintf("%s must be a duration like 30s or 250ms", key))
	}
}

func validatePositiveFloatEnv(problems *[]string, key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 {
		*problems = append(*problems, fmt.Sprintf("%s must be a number greater than 0", key))
	}
}

func validateEnumEnv(problems *[]string, key string, allowed ...string) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return
	}
	for _, candidate := range allowed {
		if value == candidate {
			return
		}
	}
	*problems = append(*problems, fmt.Sprintf("%s must be one of: %s", key, strings.Join(allowed, ", ")))
}

const templateYAML = `# ColdMIC configuration
# Precedence: CLI flags > environment variables > this file > defaults.
# Environment variables remain supported for backward compatibility.

deepgram:
  api_key: "" # DEEPGRAM_API_KEY
  api_base_url: "https://api.deepgram.com/v1" # DEEPGRAM_API_BASE
  model: "nova-2" # DEEPGRAM_MODEL
  language: "" # DEEPGRAM_LANGUAGE
  smart_format: true # DEEPGRAM_SMART_FORMAT

audio:
  recorder_command: "ffmpeg" # COLDMIC_FFMPEG_COMMAND
  input_format: "pulse" # COLDMIC_AUDIO_INPUT_FORMAT
  input_device: "default" # COLDMIC_AUDIO_INPUT_DEVICE, DEEPGRAM_PULSE_SOURCE, WHISPER_PULSE_SOURCE
  sample_rate: 16000 # COLDMIC_SAMPLE_RATE
  channels: 1 # COLDMIC_CHANNELS

rules:
  path: "" # COLDMIC_RULES_FILE; empty uses ~/.config/coldmic then ~/.config/hypr fallback
  iteration_limit: 30 # COLDMIC_RULE_ITERATION_LIMIT

session:
  chunk_size: 4096 # COLDMIC_AUDIO_CHUNK_SIZE
  streaming_grace: "1s" # COLDMIC_STREAMING_GRACE_MS / DEEPGRAM_STREAMING_GRACE_MS

conversation:
  provider: "openai" # COLDMIC_CONVERSATION_BACKEND
  base_url: "https://api.openai.com/v1" # COLDMIC_BACKEND_BASE_URL
  api_key: "" # COLDMIC_BACKEND_API_KEY
  model: "gpt-4o" # COLDMIC_BACKEND_MODEL
  system_prompt: "You are a helpful voice assistant." # COLDMIC_BACKEND_SYSTEM_PROMPT
  stream: true # COLDMIC_BACKEND_STREAM
  timeout: "30s" # COLDMIC_BACKEND_TIMEOUT
  max_history: 20 # COLDMIC_BACKEND_MAX_HISTORY
  stop_phrases: ["thanks alice", "that's all", "goodbye", "bye alice", "stop"] # COLDMIC_STOP_PHRASES
  silence_timeout: "30s" # COLDMIC_CONVERSATION_TIMEOUT

continuous:
  wake_phrases: ["hey alice", "alice"] # COLDMIC_WAKE_PHRASES
  vad_engine: "silero" # COLDMIC_VAD_ENGINE; one of: silero, energy
  vad_threshold: 0.5 # COLDMIC_VAD_THRESHOLD (Silero speech probability 0-1; use higher for EnergyVAD RMS)
  silence_ms: 800 # COLDMIC_VAD_SILENCE_MS
  frame_ms: 30 # COLDMIC_VAD_FRAME_MS
  local_stt:
    provider: "" # COLDMIC_LOCAL_STT; set to whispercpp to enable
    model: "ggml-tiny.en.bin" # COLDMIC_LOCAL_STT_MODEL

tts:
  engine: "edge-tts" # COLDMIC_TTS_ENGINE
  voice: "en-US-AriaNeural" # COLDMIC_TTS_VOICE
  rate: "+0%" # COLDMIC_TTS_RATE
  volume: "+0%" # COLDMIC_TTS_VOLUME
  playback_cmd: "ffplay" # COLDMIC_TTS_PLAYBACK_CMD

daemon:
  addr: "127.0.0.1:4317" # COLDMIC_DAEMON_ADDR
  url: "http://127.0.0.1:4317" # COLDMIC_DAEMON_URL
  toggle_compat: false # COLDMIC_TOGGLE_COMPAT
  debug: false # COLDMIC_DEBUG
`
