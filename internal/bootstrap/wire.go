package bootstrap

import (
	"fmt"

	"coldmic/internal/audio"
	"coldmic/internal/config"
	"coldmic/internal/ports"
	"coldmic/internal/providers/deepgram"
	"coldmic/internal/providers/edge_tts"
	"coldmic/internal/providers/openai"
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
	vad := audio.NewEnergyVAD(cfg.Continuous.VADThreshold)
	listener := usecase.NewContinuousListener(
		audioCap,
		vad,
		transcriptionProvider,
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
