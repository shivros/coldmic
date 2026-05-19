package audio

import (
	"math"
	"testing"
)

func TestEnergyVADSilence(t *testing.T) {
	t.Parallel()

	vad := NewEnergyVAD(500)
	// All-zero frame = silence.
	frame := make([]byte, 960) // 30 ms @ 16 kHz × 2 bytes
	prob, err := vad.Process(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prob != 0.0 {
		t.Fatalf("expected 0.0 for silence, got %f", prob)
	}
}

func TestEnergyVADSpeech(t *testing.T) {
	t.Parallel()

	vad := NewEnergyVAD(500)
	// Frame with large sample values = speech.
	frame := make([]byte, 960)
	for i := range frame {
		frame[i] = 0x7F // max positive for every byte — high energy
	}
	prob, err := vad.Process(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prob != 1.0 {
		t.Fatalf("expected 1.0 for speech, got %f", prob)
	}
}

func TestEnergyVADThresholdBoundary(t *testing.T) {
	t.Parallel()

	vad := NewEnergyVAD(500)
	// Construct a frame whose RMS is exactly at the threshold.
	// RMS = sqrt(sum(sample^2) / N) = threshold => sum/N = threshold^2
	// Use a single non-zero sample in a short frame.
	// With 1 sample at value V: RMS = |V| = threshold => V = 500
	// Frame: 2 bytes encoding int16(500)
	frame := []byte{0xF4, 0x01} // 500 in LE int16
	prob, err := vad.Process(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// At exactly the threshold we expect 0.0 (needs to exceed).
	if prob != 0.0 {
		t.Fatalf("expected 0.0 at threshold boundary, got %f", prob)
	}

	// Just above threshold.
	frame2 := []byte{0xF5, 0x01} // 501
	prob2, err := vad.Process(frame2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prob2 != 1.0 {
		t.Fatalf("expected 1.0 above threshold, got %f", prob2)
	}
}

func TestEnergyVADEmptyFrame(t *testing.T) {
	t.Parallel()

	vad := NewEnergyVAD(500)
	prob, err := vad.Process(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prob != 0.0 {
		t.Fatalf("expected 0.0 for nil frame, got %f", prob)
	}
}

func TestEnergyVADResetIsNoop(t *testing.T) {
	t.Parallel()

	vad := NewEnergyVAD(500)
	vad.Reset() // should not panic
}

func TestEnergyVADDefaultThreshold(t *testing.T) {
	t.Parallel()

	vad := NewEnergyVAD(0) // should default to 500
	frame := make([]byte, 960)
	prob, _ := vad.Process(frame)
	if prob != 0.0 {
		t.Fatalf("silence should be 0.0 with default threshold")
	}
}

func TestEnergyVADRejectsNonSpeech(t *testing.T) {
	t.Parallel()

	// Low-amplitude noise should not trigger.
	vad := NewEnergyVAD(500)
	frame := make([]byte, 960)
	for i := 0; i < len(frame); i += 2 {
		// Small values: ±10
		val := int16(10 * math.Sin(float64(i)))
		frame[i] = byte(val & 0xFF)
		frame[i+1] = byte(val >> 8)
	}
	prob, err := vad.Process(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prob != 0.0 {
		t.Fatalf("low-amplitude noise should not be speech, got %f", prob)
	}
}
