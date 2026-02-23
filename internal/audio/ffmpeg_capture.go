package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"coldmic/internal/ports"
)

// FFMPEGCapture streams microphone PCM audio using ffmpeg.
type FFMPEGCapture struct {
	command string
}

func NewFFMPEGCapture(command string) *FFMPEGCapture {
	if command == "" {
		command = "ffmpeg"
	}
	return &FFMPEGCapture{command: command}
}

func (c *FFMPEGCapture) Start(ctx context.Context, cfg ports.AudioConfig) (ports.AudioSession, error) {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 16000
	}
	if cfg.Channels <= 0 {
		cfg.Channels = 1
	}
	if cfg.InputFormat == "" {
		cfg.InputFormat = "pulse"
	}
	if cfg.InputDevice == "" {
		cfg.InputDevice = "default"
	}

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "warning",
		"-f", cfg.InputFormat,
		"-i", cfg.InputDevice,
		"-ac", strconv.Itoa(cfg.Channels),
		"-ar", strconv.Itoa(cfg.SampleRate),
		"-f", "s16le",
		"-",
	}

	cmd := exec.CommandContext(ctx, c.command, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create ffmpeg stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
		close(waitErr)
	}()

	select {
	case err := <-waitErr:
		if err != nil {
			return nil, fmt.Errorf("ffmpeg exited before capture started: %w: %s", err, stringsTrimSpaceSafe(stderr.String()))
		}
		return nil, errors.New("ffmpeg exited before capture started")
	case <-time.After(250 * time.Millisecond):
	}

	return &ffmpegSession{
		stdout:  stdout,
		stderr:  &stderr,
		process: cmd.Process,
		waitErr: waitErr,
	}, nil
}

type ffmpegSession struct {
	stdout io.ReadCloser
	stderr *bytes.Buffer

	process *os.Process
	waitErr <-chan error

	stopOnce sync.Once
	stopErr  error
}

func (s *ffmpegSession) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *ffmpegSession) Close() error {
	return s.Stop()
}

func (s *ffmpegSession) Stop() error {
	s.stopOnce.Do(func() {
		if s.process != nil {
			_ = s.process.Signal(os.Interrupt)
		}

		select {
		case err, ok := <-s.waitErr:
			if ok {
				s.stopErr = normalizeStopErr(err)
			}
		case <-time.After(1200 * time.Millisecond):
			if s.process != nil {
				_ = s.process.Kill()
			}
			err, ok := <-s.waitErr
			if ok {
				s.stopErr = normalizeStopErr(err)
			}
		}

		if closeErr := s.stdout.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			if s.stopErr == nil {
				s.stopErr = closeErr
			}
		}

		if s.stopErr != nil && s.stderr != nil && s.stderr.Len() > 0 {
			s.stopErr = fmt.Errorf("%w: %s", s.stopErr, stringsTrimSpaceSafe(s.stderr.String()))
		}
	})

	return s.stopErr
}

func normalizeStopErr(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}

func stringsTrimSpaceSafe(input string) string {
	if input == "" {
		return input
	}
	return string(bytes.TrimSpace([]byte(input)))
}
