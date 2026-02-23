package usecase

import (
	"errors"
	"fmt"
	"io"
	"time"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

func pumpAudioChunks(
	audio ports.AudioSession,
	stream ports.StreamingSession,
	chunkSize int,
	events ports.EventSink,
	done chan struct{},
) {
	defer close(done)

	if chunkSize < 256 {
		chunkSize = 4096
	}

	buf := make([]byte, chunkSize)
	for {
		n, err := audio.Read(buf)
		if n > 0 {
			if sendErr := stream.SendAudio(buf[:n]); sendErr != nil {
				events.SessionError(domain.ErrorCodeAudioStream, fmt.Sprintf("failed to stream audio: %v", sendErr))
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				events.SessionError(domain.ErrorCodeAudioStream, fmt.Sprintf("audio capture error: %v", err))
			}
			return
		}
	}
}

func waitForStream(session ports.StreamingSession, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = session.Close()
		return <-done
	}
}
