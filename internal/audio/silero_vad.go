package audio

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	"coldmic/internal/debuglog"
)

const (
	// SileroVADModelURL is the download URL for the Silero VAD ONNX model.
	SileroVADModelURL = "https://github.com/snakers4/silero-vad/raw/master/src/silero_vad/data/silero_vad.onnx"

	// sileroSampleSize is the number of float32 samples per frame for Silero VAD at 16kHz.
	// The model expects exactly 512 samples (32ms at 16kHz).
	sileroSampleSize = 512

	// sileroSampleRate is the expected sample rate.
	sileroSampleRate = 16000

	// sileroDefaultThreshold is the default speech probability threshold.
	sileroDefaultThreshold = 0.5
)

// SileroVAD implements VAD using the Silero ONNX neural network model.
// It provides significantly better speech/noise discrimination than EnergyVAD,
// correctly rejecting music, ambient noise, and other non-speech sounds.
//
// The Silero model requires ONNX Runtime. If the shared library is not found,
// NewSileroVAD returns an error — callers should fall back to EnergyVAD.
type SileroVAD struct {
	modelPath string
	session   *ort.DynamicAdvancedSession
	inputData []float32
	stateData []float32
	srData    []int64
	srShape   ort.Shape
	threshold float64
	mu        sync.Mutex
	initialized bool
	onnxCleanup func()
}

// NewSileroVAD creates a Silero-based VAD.
// modelDir is the directory to store/load the ONNX model (e.g., ~/.cache/coldmic).
// threshold is the speech probability threshold (0.0–1.0, default 0.5).
func NewSileroVAD(modelDir string, threshold float64) (*SileroVAD, error) {
	if threshold <= 0 || threshold >= 1 {
		threshold = sileroDefaultThreshold
	}
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		return nil, fmt.Errorf("silero vad: create model dir: %w", err)
	}

	modelPath := filepath.Join(modelDir, "silero_vad.onnx")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		debuglog.Printf("silero vad: model not found at %s, downloading...", modelPath)
		if err := downloadFile(modelPath, SileroVADModelURL); err != nil {
			return nil, fmt.Errorf("silero vad: download model: %w", err)
		}
		debuglog.Printf("silero vad: model downloaded to %s", modelPath)
	}

	v := &SileroVAD{
		modelPath: modelPath,
		threshold: threshold,
		inputData: make([]float32, sileroSampleSize),
		stateData: make([]float32, 2*1*128), // [2, 1, 128] flattened
		srData:    []int64{sileroSampleRate},
		srShape:   ort.Shape{1},
	}

	return v, nil
}

// Init initializes the ONNX Runtime session. Must be called before Process.
// This is separate from NewSileroVAD so the constructor can succeed even if
// ONNX Runtime is unavailable — callers can then fall back to EnergyVAD if
// Init fails.
func (v *SileroVAD) Init() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.initialized {
		return nil
	}

	// Find ONNX Runtime shared library.
	libPath := findONNXRuntimeLib()
	if libPath == "" {
		return fmt.Errorf("silero vad: libonnxruntime.so not found; install onnxruntime or place libonnxruntime.so in ~/.cache/coldmic/")
	}

	debuglog.Printf("silero vad: using onnxruntime at %s", libPath)
	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("silero vad: init onnxruntime: %w", err)
	}
	v.onnxCleanup = func() { ort.DestroyEnvironment() }

	session, err := ort.NewDynamicAdvancedSession(
		v.modelPath,
		[]string{"input", "state", "sr"},
		[]string{"output", "stateN"},
		nil,
	)
	if err != nil {
		if v.onnxCleanup != nil {
			v.onnxCleanup()
			v.onnxCleanup = nil
		}
		return fmt.Errorf("silero vad: create session: %w", err)
	}

	v.session = session
	v.initialized = true
	debuglog.Printf("silero vad: initialized successfully")
	return nil
}

// Process analyses a 16-bit LE PCM frame and returns a speech probability [0,1].
// The frame is expected to be audio at 16kHz mono 16-bit PCM. If the frame
// contains exactly 512 samples (1024 bytes), it is processed in one pass.
// Otherwise, the data is chunked or padded as needed.
func (v *SileroVAD) Process(frame []byte) (float64, error) {
	if !v.initialized {
		return 0, fmt.Errorf("silero vad: not initialized, call Init() first")
	}

	samples := int16LEToFloat32(frame)
	if len(samples) == 0 {
		return 0, nil
	}

	// If we got exactly 512 samples, process directly.
	if len(samples) == sileroSampleSize {
		return v.processChunk(samples)
	}

	// For frames smaller than 512, pad with zeros.
	if len(samples) < sileroSampleSize {
		padded := make([]float32, sileroSampleSize)
		copy(padded, samples)
		return v.processChunk(padded)
	}

	// Multiple chunks: take max probability.
	var maxProb float64
	for offset := 0; offset+sileroSampleSize <= len(samples); offset += sileroSampleSize {
		chunk := samples[offset : offset+sileroSampleSize]
		p, err := v.processChunk(chunk)
		if err != nil {
			return 0, err
		}
		if p > maxProb {
			maxProb = p
		}
	}
	return maxProb, nil
}

// processChunk runs inference on exactly 512 float32 samples.
func (v *SileroVAD) processChunk(samples []float32) (float64, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	copy(v.inputData, samples)

	inputTensor, err := ort.NewTensor(ort.Shape{1, sileroSampleSize}, v.inputData)
	if err != nil {
		return 0, fmt.Errorf("silero vad: input tensor: %w", err)
	}
	defer inputTensor.Destroy()

	stateTensor, err := ort.NewTensor(ort.Shape{2, 1, 128}, v.stateData)
	if err != nil {
		return 0, fmt.Errorf("silero vad: state tensor: %w", err)
	}
	defer stateTensor.Destroy()

	srTensor, err := ort.NewTensor(v.srShape, v.srData)
	if err != nil {
		return 0, fmt.Errorf("silero vad: sr tensor: %w", err)
	}
	defer srTensor.Destroy()

	outputTensor, err := ort.NewEmptyTensor[float32](ort.Shape{1, 1})
	if err != nil {
		return 0, fmt.Errorf("silero vad: output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	stateNTensor, err := ort.NewEmptyTensor[float32](ort.Shape{2, 1, 128})
	if err != nil {
		return 0, fmt.Errorf("silero vad: stateN tensor: %w", err)
	}
	defer stateNTensor.Destroy()

	err = v.session.Run(
		[]ort.Value{inputTensor, stateTensor, srTensor},
		[]ort.Value{outputTensor, stateNTensor},
	)
	if err != nil {
		return 0, fmt.Errorf("silero vad: inference: %w", err)
	}

	// Update hidden state for next frame.
	copy(v.stateData, stateNTensor.GetData())

	prob := float64(outputTensor.GetData()[0])
	return prob, nil
}

// Reset clears internal state so the next call starts fresh.
func (v *SileroVAD) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.stateData {
		v.stateData[i] = 0
	}
}

// Close releases ONNX Runtime resources. Call when shutting down.
func (v *SileroVAD) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.session != nil {
		v.session.Destroy()
		v.session = nil
	}
	if v.onnxCleanup != nil {
		v.onnxCleanup()
		v.onnxCleanup = nil
	}
	v.initialized = false
}

// int16LEToFloat32 converts 16-bit little-endian PCM bytes to float32 samples
// normalized to [-1.0, 1.0].
func int16LEToFloat32(data []byte) []float32 {
	n := len(data) / 2
	if n == 0 {
		return nil
	}
	result := make([]float32, n)
	for i := 0; i < n; i++ {
		sample := int16(data[i*2]) | int16(data[i*2+1])<<8
		result[i] = float32(sample) / 32768.0
	}
	return result
}

// findONNXRuntimeLib searches for the ONNX Runtime shared library in common locations.
// Returns empty string if not found.
func findONNXRuntimeLib() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/usr/lib/libonnxruntime.so",
		"/usr/local/lib/libonnxruntime.so",
		"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".cache", "coldmic", "libonnxruntime.so"),
			filepath.Join(home, ".local", "lib", "libonnxruntime.so"),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// downloadFile downloads a file from url to path.
func downloadFile(path, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(path)
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
