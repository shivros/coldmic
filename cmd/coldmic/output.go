package main

import (
	"fmt"
	"io"
	"time"

	"coldmic/internal/domain"
)

func printStatus(w io.Writer, status domain.Status) {
	if status.Message == "" {
		fmt.Fprintf(w, "state=%s active=%t\n", status.State, status.Active)
		return
	}
	fmt.Fprintf(w, "state=%s active=%t message=%s\n", status.State, status.Active, status.Message)
}

func printStopResult(w io.Writer, status domain.Status, result domain.StopResult) {
	fmt.Fprintf(w, "state=%s active=%t copied=%t\n", status.State, status.Active, result.Copied)
	fmt.Fprintln(w, result.FinalTranscript)
}

func printTranscript(w io.Writer, capturedAt time.Time, result domain.StopResult) {
	fmt.Fprintf(w, "captured_at=%s copied=%t\n", printTranscriptTime(capturedAt), result.Copied)
	fmt.Fprintln(w, result.FinalTranscript)
}
