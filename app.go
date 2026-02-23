package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"coldmic/internal/bootstrap"
	"coldmic/internal/config"
	"coldmic/internal/domain"
	"coldmic/internal/usecase"
)

const (
	eventSession = "coldmic:session"
	eventPartial = "coldmic:partial"
	eventFinal   = "coldmic:final"
	eventError   = "coldmic:error"
)

// App is the Wails application root.
type App struct {
	ctx context.Context

	controller *usecase.SessionController
	cfg        config.Config
	bootErr    error
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	services, err := bootstrap.Build(a, &wailsClipboard{})
	if err != nil {
		a.bootErr = err
		a.SessionError(domain.ErrorCodeStartup, err.Error())
		return
	}

	a.cfg = services.Config
	a.controller = services.Controller
	a.SessionStateChanged(domain.SessionStateIdle, domain.SessionReasonMicCold)
}

// StartPTT starts push-to-talk recording.
func (a *App) StartPTT() (domain.Status, error) {
	if err := a.requireReady(); err != nil {
		return domain.Status{}, err
	}
	if err := a.controller.Start(a.ctx); err != nil {
		a.SessionError(domain.ErrorCodeTranscription, err.Error())
		return domain.Status{}, err
	}
	return a.controller.Status(), nil
}

// StopPTT stops recording and returns processed transcript output.
func (a *App) StopPTT() (domain.StopResult, error) {
	if err := a.requireReady(); err != nil {
		return domain.StopResult{}, err
	}
	result, err := a.controller.Stop(a.ctx)
	if err != nil {
		a.SessionError(domain.ErrorCodeTranscription, err.Error())
		return domain.StopResult{}, err
	}
	return result, nil
}

// AbortPTT discards an in-progress recording.
func (a *App) AbortPTT() error {
	if err := a.requireReady(); err != nil {
		return err
	}
	if err := a.controller.Abort(); err != nil {
		if errors.Is(err, usecase.ErrNoActiveSession) {
			return nil
		}
		a.SessionError(domain.ErrorCodeTranscription, err.Error())
		return err
	}
	return nil
}

// GetStatus returns the current session status.
func (a *App) GetStatus() domain.Status {
	if a.controller == nil {
		if a.bootErr != nil {
			return domain.Status{State: domain.SessionStateError, Active: false, Message: a.bootErr.Error()}
		}
		return domain.Status{State: domain.SessionStateIdle, Active: false}
	}
	return a.controller.Status()
}

// GetRuntimeInfo returns non-sensitive config for the UI.
func (a *App) GetRuntimeInfo() map[string]string {
	if a.bootErr != nil {
		return map[string]string{"error": a.bootErr.Error()}
	}

	return map[string]string{
		"provider":         "Deepgram",
		"model":            a.cfg.Deepgram.Model,
		"language":         a.cfg.Deepgram.Language,
		"rulesFile":        a.cfg.Rules.Path,
		"audioInput":       a.cfg.Audio.InputDevice,
		"audioInputFormat": a.cfg.Audio.InputFormat,
	}
}

func (a *App) requireReady() error {
	if a.bootErr != nil {
		return a.bootErr
	}
	if a.controller == nil {
		return fmt.Errorf("application is not initialized")
	}
	return nil
}

// SessionStateChanged emits session lifecycle updates to the frontend.
func (a *App) SessionStateChanged(state domain.SessionState, reason domain.SessionStateReason) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventSession, map[string]string{
		"state":   string(state),
		"reason":  string(reason),
		"message": sessionReasonMessage(reason),
	})
}

// PartialTranscript emits live partial transcript text.
func (a *App) PartialTranscript(text string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventPartial, map[string]string{"text": text})
}

// FinalTranscript emits final transcript output.
func (a *App) FinalTranscript(raw string, transformed string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventFinal, map[string]string{
		"raw":         raw,
		"transformed": transformed,
	})
}

// SessionError emits backend errors to the UI.
func (a *App) SessionError(code domain.ErrorCode, detail string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, eventError, map[string]string{
		"code":    string(code),
		"message": errorMessage(code, detail),
		"detail":  detail,
	})
}

func sessionReasonMessage(reason domain.SessionStateReason) string {
	switch reason {
	case domain.SessionReasonMicCold:
		return "Mic cold"
	case domain.SessionReasonRecordingStarted:
		return "Recording started"
	case domain.SessionReasonRecordingRestarted:
		return "Recording restarted; previous capture discarded"
	case domain.SessionReasonTranscribing:
		return "Recording stopped. Transcribing..."
	case domain.SessionReasonTranscriptCopied:
		return "Transcript copied to clipboard"
	case domain.SessionReasonTranscriptReadyClipboardFailed:
		return "Transcript ready (clipboard write failed)"
	case domain.SessionReasonRecordingDiscarded:
		return "Recording discarded"
	case domain.SessionReasonNoTranscript:
		return "No transcript captured"
	case domain.SessionReasonTranscriptionFailed:
		return "Transcription failed"
	case domain.SessionReasonRulesFailed:
		return "Rules processing failed"
	default:
		return ""
	}
}

func errorMessage(code domain.ErrorCode, detail string) string {
	switch code {
	case domain.ErrorCodeStartup:
		return "Startup failed"
	case domain.ErrorCodeAudioStop:
		return "Audio stop issue"
	case domain.ErrorCodeAudioStream:
		return "Audio streaming issue"
	case domain.ErrorCodeClipboard:
		return "Clipboard write failed"
	case domain.ErrorCodeRules:
		return "Rules processing failed"
	case domain.ErrorCodeTranscription:
		return "Transcription error"
	default:
		if detail == "" {
			return "Unknown error"
		}
		return detail
	}
}

type wailsClipboard struct{}

func (c *wailsClipboard) SetText(ctx context.Context, text string) error {
	return runtime.ClipboardSetText(ctx, text)
}
