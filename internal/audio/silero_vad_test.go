package audio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInt16LEToFloat32(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []float32
	}{
		{
			name:  "empty",
			input: []byte{},
			want:  nil,
		},
		{
			name:  "zero sample",
			input: []byte{0x00, 0x00},
			want:  []float32{0.0},
		},
		{
			name:  "max positive",
			input: []byte{0xFF, 0x7F},
			want:  []float32{float32(32767) / 32768.0},
		},
		{
			name:  "max negative",
			input: []byte{0x00, 0x80},
			want:  []float32{float32(-32768) / 32768.0},
		},
		{
			name:  "two samples",
			input: []byte{0x00, 0x00, 0xFF, 0x7F},
			want:  []float32{0.0, float32(32767) / 32768.0},
		},
		{
			name:  "odd number of bytes ignores trailing byte",
			input: []byte{0x00, 0x00, 0xFF},
			want:  []float32{0.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := int16LEToFloat32(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len(got) = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("sample[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSileroVADNew(t *testing.T) {
	tmpDir := t.TempDir()
	v, err := NewSileroVAD(tmpDir, 0.5)
	if err != nil {
		t.Fatalf("NewSileroVAD: %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil SileroVAD")
	}
	if v.threshold != 0.5 {
		t.Errorf("threshold = %v, want 0.5", v.threshold)
	}
	// Model should have been downloaded.
	if _, err := os.Stat(filepath.Join(tmpDir, "silero_vad.onnx")); os.IsNotExist(err) {
		t.Error("model file was not downloaded")
	}
}

func TestSileroVADNewDefaultThreshold(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{0, sileroDefaultThreshold},
		{1, sileroDefaultThreshold},
		{-1, sileroDefaultThreshold},
		{1.5, sileroDefaultThreshold},
		{0.5, 0.5},
		{0.3, 0.3},
		{0.8, 0.8},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			tmpDir := t.TempDir()
			v, err := NewSileroVAD(tmpDir, tt.input)
			if err != nil {
				t.Fatalf("NewSileroVAD: %v", err)
			}
			if v.threshold != tt.expected {
				t.Errorf("threshold = %v, want %v", v.threshold, tt.expected)
			}
		})
	}
}

func TestSileroVADReset(t *testing.T) {
	tmpDir := t.TempDir()
	v, err := NewSileroVAD(tmpDir, 0.5)
	if err != nil {
		t.Fatalf("NewSileroVAD: %v", err)
	}
	// Set some non-zero state.
	for i := range v.stateData {
		v.stateData[i] = 0.5
	}
	v.Reset()
	for i, s := range v.stateData {
		if s != 0 {
			t.Errorf("stateData[%d] = %v, want 0 after Reset", i, s)
		}
	}
}

func TestSileroVADProcessNotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	v, err := NewSileroVAD(tmpDir, 0.5)
	if err != nil {
		t.Fatalf("NewSileroVAD: %v", err)
	}
	frame := make([]byte, 1024) // 512 samples
	_, err = v.Process(frame)
	if err == nil {
		t.Error("expected error when Process called before Init")
	}
}

func TestSileroVADClose(t *testing.T) {
	tmpDir := t.TempDir()
	v, err := NewSileroVAD(tmpDir, 0.5)
	if err != nil {
		t.Fatalf("NewSileroVAD: %v", err)
	}
	// Close without Init should be safe.
	v.Close()
	if v.initialized {
		t.Error("expected initialized=false after Close")
	}
}

func TestFindONNXRuntimeLib(t *testing.T) {
	// This test just checks the function runs without panicking.
	// It may or may not find the library depending on the system.
	_ = findONNXRuntimeLib()
}

func TestSileroVADE2E(t *testing.T) {
	// End-to-end test: only runs if ONNX Runtime is available.
	libPath := findONNXRuntimeLib()
	if libPath == "" {
		t.Skip("ONNX Runtime shared library not found, skipping E2E test")
	}

	tmpDir := t.TempDir()
	v, err := NewSileroVAD(tmpDir, 0.5)
	if err != nil {
		t.Fatalf("NewSileroVAD: %v", err)
	}
	defer v.Close()

	if err := v.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Test silence (zeros) — should have very low probability.
	silence := make([]byte, 1024) // 512 samples of zero
	prob, err := v.Process(silence)
	if err != nil {
		t.Fatalf("Process silence: %v", err)
	}
	if prob > 0.1 {
		t.Errorf("silence probability = %v, expected < 0.1", prob)
	}
	t.Logf("silence probability: %.6f", prob)

	// Test with non-zero data (won't be real speech, but should still work).
	noise := make([]byte, 1024)
	for i := 0; i < len(noise); i++ {
		noise[i] = byte(i * 7 % 256)
	}
	prob, err = v.Process(noise)
	if err != nil {
		t.Fatalf("Process noise: %v", err)
	}
	// Just check it returns a valid probability.
	if prob < 0 || prob > 1 {
		t.Errorf("noise probability = %v, expected [0, 1]", prob)
	}
	t.Logf("noise probability: %.6f", prob)

	// Test Reset works.
	v.Reset()
	prob, err = v.Process(silence)
	if err != nil {
		t.Fatalf("Process after reset: %v", err)
	}
	if prob > 0.1 {
		t.Errorf("post-reset silence probability = %v, expected < 0.1", prob)
	}

	// Test Process with non-standard frame sizes.
	// Shorter than 512 samples (padded).
	shortFrame := make([]byte, 512) // 256 samples
	prob, err = v.Process(shortFrame)
	if err != nil {
		t.Fatalf("Process short frame: %v", err)
	}
	t.Logf("short frame probability: %.6f", prob)

	// Longer than 512 samples (chunked).
	longFrame := make([]byte, 2048) // 1024 samples = 2 chunks
	prob, err = v.Process(longFrame)
	if err != nil {
		t.Fatalf("Process long frame: %v", err)
	}
	t.Logf("long frame probability: %.6f", prob)
}
