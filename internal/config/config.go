package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config stores runtime configuration for the tracer bullet.
type Config struct {
	Deepgram DeepgramConfig
	Audio    AudioConfig
	Rules    RulesConfig
	Session  SessionConfig
}

type DeepgramConfig struct {
	APIKey      string
	APIBaseURL  string
	Model       string
	Language    string
	SmartFormat bool
}

type AudioConfig struct {
	RecorderCommand string
	InputFormat     string
	InputDevice     string
	SampleRate      int
	Channels        int
}

type RulesConfig struct {
	Path           string
	IterationLimit int
}

type SessionConfig struct {
	ChunkSize      int
	StreamingGrace time.Duration
}

// Load resolves configuration from environment variables and sensible defaults.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, errors.New("could not determine home directory")
	}

	defaultRules := filepath.Join(home, ".config", "coldmic", "substitutions.rules")
	hyprRules := filepath.Join(home, ".config", "hypr", "whisper-substitutions.rules")
	rulesPath := strings.TrimSpace(os.Getenv("COLDMIC_RULES_FILE"))
	if rulesPath == "" {
		rulesPath = firstExisting(defaultRules, hyprRules)
	}

	cfg := Config{
		Deepgram: DeepgramConfig{
			APIKey:      strings.TrimSpace(os.Getenv("DEEPGRAM_API_KEY")),
			APIBaseURL:  envOrDefault("DEEPGRAM_API_BASE", "https://api.deepgram.com/v1"),
			Model:       envOrDefault("DEEPGRAM_MODEL", "nova-2"),
			Language:    strings.TrimSpace(os.Getenv("DEEPGRAM_LANGUAGE")),
			SmartFormat: envOrDefaultBool("DEEPGRAM_SMART_FORMAT", true),
		},
		Audio: AudioConfig{
			RecorderCommand: envOrDefault("COLDMIC_FFMPEG_COMMAND", "ffmpeg"),
			InputFormat:     envOrDefault("COLDMIC_AUDIO_INPUT_FORMAT", "pulse"),
			InputDevice: firstNonEmpty(
				os.Getenv("COLDMIC_AUDIO_INPUT_DEVICE"),
				os.Getenv("DEEPGRAM_PULSE_SOURCE"),
				os.Getenv("WHISPER_PULSE_SOURCE"),
				"default",
			),
			SampleRate: envOrDefaultInt("COLDMIC_SAMPLE_RATE", 16000),
			Channels:   envOrDefaultInt("COLDMIC_CHANNELS", 1),
		},
		Rules: RulesConfig{
			Path:           rulesPath,
			IterationLimit: envOrDefaultInt("COLDMIC_RULE_ITERATION_LIMIT", 30),
		},
		Session: SessionConfig{
			ChunkSize:      envOrDefaultInt("COLDMIC_AUDIO_CHUNK_SIZE", 4096),
			StreamingGrace: time.Duration(firstNonNegativeInt("COLDMIC_STREAMING_GRACE_MS", "DEEPGRAM_STREAMING_GRACE_MS", 1000)) * time.Millisecond,
		},
	}

	if cfg.Audio.SampleRate <= 0 {
		cfg.Audio.SampleRate = 16000
	}
	if cfg.Audio.Channels <= 0 {
		cfg.Audio.Channels = 1
	}
	if cfg.Rules.IterationLimit <= 0 {
		cfg.Rules.IterationLimit = 30
	}
	if cfg.Session.ChunkSize < 256 {
		cfg.Session.ChunkSize = 4096
	}

	return cfg, nil
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envOrDefaultInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOrDefaultBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func firstNonNegativeInt(primary string, secondary string, fallback int) int {
	for _, key := range []string{primary, secondary} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil && parsed >= 0 {
			return parsed
		}
	}
	return fallback
}
