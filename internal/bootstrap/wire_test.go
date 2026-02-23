package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"coldmic/internal/domain"
)

func TestBuildSuccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DEEPGRAM_API_KEY", "test-key")

	services, err := Build(noopEventSink{}, noopClipboard{})
	if err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if services.Controller == nil {
		t.Fatalf("expected controller")
	}
}

func TestBuildFailsOnInvalidRules(t *testing.T) {
	home := t.TempDir()
	rules := filepath.Join(home, "bad.rules")
	if err := os.WriteFile(rules, []byte("not a valid rule\n"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("COLDMIC_RULES_FILE", rules)

	_, err := Build(noopEventSink{}, noopClipboard{})
	if err == nil {
		t.Fatalf("expected build error due to invalid rules")
	}
}

type noopEventSink struct{}

func (noopEventSink) SessionStateChanged(_ domain.SessionState, _ domain.SessionStateReason) {}
func (noopEventSink) PartialTranscript(_ string)                                             {}
func (noopEventSink) FinalTranscript(_, _ string)                                            {}
func (noopEventSink) SessionError(_ domain.ErrorCode, _ string)                              {}

type noopClipboard struct{}

func (noopClipboard) SetText(_ context.Context, _ string) error { return nil }
