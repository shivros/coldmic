package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"coldmic/internal/audio"
	"coldmic/internal/config"
	"coldmic/internal/ports"
	"coldmic/internal/providers/deepgram"
	"coldmic/internal/providers/edge_tts"
	"coldmic/internal/providers/openai"
	"coldmic/internal/providers/whispercpp"
	"coldmic/internal/rules"
	"coldmic/internal/usecase"
)

// Services is the assembled runtime graph.
type Services struct {
	Controller             *usecase.SessionController
	ConversationController *usecase.ConversationController
	Session                *usecase.SessionService
	Config                 config.Config
	Conversation           ports.ConversationBackend
}

// Build wires all backend dependencies for the current runtime.
func Build(eventSink ports.EventSink, clipboard ports.Clipboard) (Services, error) {
	cfg, err := config.Load()
	if err != nil {
		return Services{}, err
	}

	rulesEngine, err := rules.NewEngine(cfg.Rules.Path, cfg.Rules.IterationLimit)
	if err != nil {
		return Services{}, err
	}

	audioCap := audio.NewFFMPEGCapture(cfg.Audio.RecorderCommand)
	transcriptionProvider := deepgram.NewProvider(deepgram.Config{
		APIKey:      cfg.Deepgram.APIKey,
		APIBaseURL:  cfg.Deepgram.APIBaseURL,
		Model:       cfg.Deepgram.Model,
		Language:    cfg.Deepgram.Language,
		SmartFormat: cfg.Deepgram.SmartFormat,
	})

	controller := usecase.NewSessionController(
		audioCap,
		transcriptionProvider,
		rulesEngine,
		clipboard,
		eventSink,
		usecase.Config{
			Audio: ports.AudioConfig{
				SampleRate:  cfg.Audio.SampleRate,
				Channels:    cfg.Audio.Channels,
				InputFormat: cfg.Audio.InputFormat,
				InputDevice: cfg.Audio.InputDevice,
			},
			Streaming: ports.StreamingConfig{
				SampleRate:     cfg.Audio.SampleRate,
				Channels:       cfg.Audio.Channels,
				Encoding:       "linear16",
				InterimResults: true,
			},
			ChunkSize:      cfg.Session.ChunkSize,
			StreamingGrace: cfg.Session.StreamingGrace,
		},
	)

	var conversationBackend ports.ConversationBackend
	switch cfg.Conversation.Provider {
	case "openai", "":
		conversationBackend = openai.NewProvider(openai.Config{
			BaseURL:      cfg.Conversation.BaseURL,
			APIKey:       cfg.Conversation.APIKey,
			Model:        cfg.Conversation.Model,
			SystemPrompt: cfg.Conversation.SystemPrompt,
			Stream:       cfg.Conversation.Stream,
			Timeout:      cfg.Conversation.Timeout,
			MaxHistory:   cfg.Conversation.MaxHistory,
		})
	default:
		return Services{}, fmt.Errorf("unknown conversation backend provider: %q", cfg.Conversation.Provider)
	}

	// Wire continuous listener with VAD.
	var vad audio.VAD
	switch cfg.Continuous.VADEngine {
	case "silero":
		home, _ := os.UserHomeDir()
		modelDir := filepath.Join(home, ".cache", "coldmic")
		sileroVAD, createErr := audio.NewSileroVAD(modelDir, 0.5)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "warning: silero VAD creation failed (%v), falling back to energy VAD\n", createErr)
			vad = audio.NewEnergyVAD(500)
		} else if initErr := sileroVAD.Init(); initErr != nil {
			fmt.Fprintf(os.Stderr, "warning: silero VAD init failed (%v), falling back to energy VAD\n", initErr)
			vad = audio.NewEnergyVAD(500)
		} else {
			vad = sileroVAD
		}
	case "energy":
		vad = audio.NewEnergyVAD(cfg.Continuous.VADThreshold)
	default:
		fmt.Fprintf(os.Stderr, "warning: unknown VAD engine %q, using energy VAD\n", cfg.Continuous.VADEngine)
		vad = audio.NewEnergyVAD(cfg.Continuous.VADThreshold)
	}
	// Wire local STT provider (optional).
	var localSTT ports.LocalSTT
	if cfg.Continuous.LocalSTT.Provider != "" {
		home, _ := os.UserHomeDir()
		cacheDir := filepath.Join(home, ".cache", "coldmic")
		switch cfg.Continuous.LocalSTT.Provider {
		case "whispercpp":
			stt := whispercpp.NewProvider(whispercpp.Config{
				CacheDir: cacheDir,
				Model:    cfg.Continuous.LocalSTT.Model,
			})
			if initErr := stt.Init(); initErr != nil {
				fmt.Fprintf(os.Stderr, "warning: local STT init failed (%v), using cloud-only transcription\n", initErr)
			} else {
				localSTT = stt
			}
		default:
			fmt.Fprintf(os.Stderr, "warning: unknown local STT provider %q, using cloud-only transcription\n", cfg.Continuous.LocalSTT.Provider)
		}
	}
	listener := usecase.NewContinuousListener(
		audioCap,
		vad,
		transcriptionProvider,
		localSTT,
		eventSink,
		usecase.ContinuousListenerConfig{
			WakePhrases:  cfg.Continuous.WakePhrases,
			VADThreshold: cfg.Continuous.VADThreshold,
			SilenceMs:    cfg.Continuous.SilenceMs,
			FrameMs:      cfg.Continuous.FrameMs,
			Audio: ports.AudioConfig{
				SampleRate:  cfg.Audio.SampleRate,
				Channels:    cfg.Audio.Channels,
				InputFormat: cfg.Audio.InputFormat,
				InputDevice: cfg.Audio.InputDevice,
			},
			Streaming: ports.StreamingConfig{
				SampleRate:     cfg.Audio.SampleRate,
				Channels:       cfg.Audio.Channels,
				Encoding:       "linear16",
				InterimResults: true,
			},
			ChunkSize: cfg.Session.ChunkSize,
		},
	)

	// Wire TTS provider.
	var ttsProvider ports.TextToSpeech
	switch cfg.TTS.Engine {
	case "edge-tts", "":
		ttsProvider = edge_tts.NewProvider(edge_tts.Config{
			Command:     "edge-tts",
			Voice:       cfg.TTS.Voice,
			Rate:        cfg.TTS.Rate,
			Volume:      cfg.TTS.Volume,
			PlaybackCmd: cfg.TTS.PlaybackCmd,
		})
	default:
		return Services{}, fmt.Errorf("unknown TTS engine: %q", cfg.TTS.Engine)
	}

	// Wire conversation controller.
	conversationCtrl := usecase.NewConversationController(
		listener,
		conversationBackend,
		ttsProvider,
		eventSink,
		usecase.ConversationControllerConfig{
			StopPhrases:    cfg.Conversation.StopPhrases,
			SilenceTimeout: cfg.Conversation.SilenceTimeout,
		},
	)

	return Services{
		Controller:             controller,
		ConversationController: conversationCtrl,
		Session:                usecase.NewSessionServiceWithConversation(controller, listener, conversationCtrl),
		Config:                 cfg,
		Conversation:           conversationBackend,
	}, nil
}
