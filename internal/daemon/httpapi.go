package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"coldmic/internal/domain"
)

const (
	contentTypeJSON = "application/json"
)

type API struct {
	service SessionService
}

func NewAPI(service SessionService) *API {
	return &API{service: service}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/session/start", a.handleStart)
	mux.HandleFunc("/v1/session/stop", a.handleStop)
	mux.HandleFunc("/v1/session/abort", a.handleAbort)
	mux.HandleFunc("/v1/session/status", a.handleStatus)
	mux.HandleFunc("/v1/session/transcript/latest", a.handleLatestTranscript)
	return mux
}

func (a *API) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	if err := a.service.Start(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, StatusResponse{OK: true, Status: a.service.Status()})
}

func (a *API) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result, err := a.service.Stop(ctx)
	if err != nil {
		if errors.Is(err, domain.ErrNoActiveSession) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, StopResponse{OK: true, Status: a.service.Status(), Result: result})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{OK: true, Status: a.service.Status()})
}

func (a *API) handleAbort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	if err := a.service.Abort(); err != nil {
		if errors.Is(err, domain.ErrNoActiveSession) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{OK: true, Status: a.service.Status()})
}

func (a *API) handleLatestTranscript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	latest, err := a.service.LastTranscript()
	if err != nil {
		if errors.Is(err, domain.ErrNoTranscriptAvailable) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, LatestTranscriptResponse{
		OK:       true,
		Captured: latest.CapturedAt,
		Result:   latest.Result,
	})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{OK: false, Error: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
