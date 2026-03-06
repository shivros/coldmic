package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"coldmic/internal/domain"
)

func TestAPIStart(t *testing.T) {
	t.Parallel()
	svc := &fakeService{status: domain.Status{State: domain.SessionStateRecording, Active: true}}
	api := NewAPI(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/session/start", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
	if svc.startCalls != 1 {
		t.Fatalf("expected start call")
	}
}

func TestAPIStartMethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{})

	req := httptest.NewRequest(http.MethodGet, "/v1/session/start", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIStartInternalError(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{startErr: errors.New("boom")})

	req := httptest.NewRequest(http.MethodPost, "/v1/session/start", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIStopConflict(t *testing.T) {
	t.Parallel()
	svc := &fakeService{stopErr: domain.ErrNoActiveSession}
	api := NewAPI(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/session/stop", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIStopMethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{})

	req := httptest.NewRequest(http.MethodGet, "/v1/session/stop", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIStopInternalError(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{stopErr: errors.New("boom")})

	req := httptest.NewRequest(http.MethodPost, "/v1/session/stop", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIAbortConflict(t *testing.T) {
	t.Parallel()
	svc := &fakeService{abortErr: domain.ErrNoActiveSession}
	api := NewAPI(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/session/abort", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIAbortMethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{})

	req := httptest.NewRequest(http.MethodGet, "/v1/session/abort", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIAbortInternalError(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{abortErr: errors.New("boom")})

	req := httptest.NewRequest(http.MethodPost, "/v1/session/abort", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIAbortSuccess(t *testing.T) {
	t.Parallel()
	svc := &fakeService{status: domain.Status{State: domain.SessionStateIdle, Active: false}}
	api := NewAPI(svc)

	req := httptest.NewRequest(http.MethodPost, "/v1/session/abort", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
	if svc.abortCalls != 1 {
		t.Fatalf("expected abort call")
	}
}

func TestAPIStatusSuccess(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{status: domain.Status{State: domain.SessionStateIdle}})

	req := httptest.NewRequest(http.MethodGet, "/v1/session/status", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPIStatusMethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{})

	req := httptest.NewRequest(http.MethodPost, "/v1/session/status", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPILatestTranscriptNotFound(t *testing.T) {
	t.Parallel()
	svc := &fakeService{lastErr: domain.ErrNoTranscriptAvailable}
	api := NewAPI(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/session/transcript/latest", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPILatestTranscriptMethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{})

	req := httptest.NewRequest(http.MethodPost, "/v1/session/transcript/latest", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPILatestTranscriptInternalError(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeService{lastErr: errors.New("boom")})

	req := httptest.NewRequest(http.MethodGet, "/v1/session/transcript/latest", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected code: %d", rec.Code)
	}
}

func TestAPILatestTranscriptSuccess(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	svc := &fakeService{
		latest: domain.LatestTranscript{
			CapturedAt: now,
			Result: domain.StopResult{
				RawTranscript:   "raw",
				FinalTranscript: "final",
				Copied:          true,
			},
		},
	}
	api := NewAPI(svc)

	req := httptest.NewRequest(http.MethodGet, "/v1/session/transcript/latest", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected code: %d", rec.Code)
	}

	var body LatestTranscriptResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok=true")
	}
	if body.Captured.IsZero() {
		t.Fatalf("expected capture timestamp")
	}
}

type fakeService struct {
	startCalls int
	abortCalls int
	startErr   error
	stopErr    error
	abortErr   error
	lastErr    error
	status     domain.Status
	stopResult domain.StopResult
	latest     domain.LatestTranscript
}

func (f *fakeService) Start(context.Context) error {
	f.startCalls++
	return f.startErr
}

func (f *fakeService) Stop(context.Context) (domain.StopResult, error) {
	if f.stopErr != nil {
		return domain.StopResult{}, f.stopErr
	}
	return f.stopResult, nil
}

func (f *fakeService) Abort() error {
	f.abortCalls++
	return f.abortErr
}

func (f *fakeService) Status() domain.Status {
	return f.status
}

func (f *fakeService) LastTranscript() (domain.LatestTranscript, error) {
	if f.lastErr != nil {
		return domain.LatestTranscript{}, f.lastErr
	}
	if f.latest.CapturedAt.IsZero() {
		return domain.LatestTranscript{}, errors.New("missing")
	}
	return f.latest, nil
}
