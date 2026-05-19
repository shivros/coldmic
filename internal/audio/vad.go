package audio

import (
	"encoding/binary"
	"math"
)

// VAD reports the speech probability for a single PCM frame.
type VAD interface {
	// Process analyses a 16-bit LE PCM frame and returns a speech probability [0,1].
	Process(frame []byte) (float64, error)
	// Reset clears internal state so the next call starts fresh.
	Reset()
}

// EnergyVAD implements VAD using RMS energy thresholding on 16-bit LE PCM audio.
// It is a lightweight alternative to model-based detectors (Silero, etc.) and
// requires no external dependencies. Swap implementations via the VAD interface.
type EnergyVAD struct {
	threshold float64 // RMS energy threshold in arbitrary units
}

// NewEnergyVAD creates an energy-based VAD.
// threshold is the minimum RMS energy to consider a frame as speech.
// A reasonable starting value for 16 kHz mono 16-bit audio is ~500.
func NewEnergyVAD(threshold float64) *EnergyVAD {
	if threshold <= 0 {
		threshold = 500
	}
	return &EnergyVAD{threshold: threshold}
}

// Process computes the RMS energy of frame and returns 1.0 if it exceeds the
// threshold, 0.0 otherwise. Frames must contain an even number of bytes
// (16-bit samples).
func (v *EnergyVAD) Process(frame []byte) (float64, error) {
	n := len(frame) / 2 // number of 16-bit samples
	if n == 0 {
		return 0, nil
	}

	var sum float64
	for i := 0; i < n; i++ {
		sample := float64(int16(binary.LittleEndian.Uint16(frame[i*2:])))
		sum += sample * sample
	}
	rms := math.Sqrt(sum / float64(n))

	if rms > v.threshold {
		return 1.0, nil
	}
	return 0.0, nil
}

// Reset is a no-op for energy-based VAD (no internal state).
func (v *EnergyVAD) Reset() {}
