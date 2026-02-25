package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"coldmic/internal/domain"
)

func TestPrintStatus(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printStatus(&buf, domain.Status{State: domain.SessionStateIdle, Active: false})
	if !strings.Contains(buf.String(), "state=idle") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestPrintStopResult(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printStopResult(&buf, domain.Status{State: domain.SessionStateIdle, Active: false}, domain.StopResult{FinalTranscript: "final", Copied: true})
	if !strings.Contains(buf.String(), "final") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestPrintTranscript(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printTranscript(&buf, time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC), domain.StopResult{FinalTranscript: "final", Copied: true})
	if !strings.Contains(buf.String(), "captured_at=") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}
