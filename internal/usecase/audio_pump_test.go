package usecase

import (
	"errors"
	"io"
	"testing"
	"time"

	"coldmic/internal/domain"
)

func TestPumpAudioChunksReportsSendError(t *testing.T) {
	t.Parallel()

	audio := &fakeAudioSession{chunks: [][]byte{[]byte("abc")}}
	stream := &sendErrStream{err: errors.New("send failed")}
	events := &fakeEventSink{}
	done := make(chan struct{})

	go pumpAudioChunks(audio, stream, 256, events, done)
	<-done

	errs := events.snapshotErrors()
	if len(errs) == 0 || errs[0].code != domain.ErrorCodeAudioStream {
		t.Fatalf("expected audio stream error")
	}
}

func TestPumpAudioChunksReportsReadError(t *testing.T) {
	t.Parallel()

	audio := &errorAudioSession{err: errors.New("read failed")}
	stream := &sendErrStream{}
	events := &fakeEventSink{}
	done := make(chan struct{})

	go pumpAudioChunks(audio, stream, 256, events, done)
	<-done

	errs := events.snapshotErrors()
	if len(errs) == 0 || errs[0].code != domain.ErrorCodeAudioStream {
		t.Fatalf("expected audio stream error")
	}
}

func TestWaitForStreamTimeoutClosesSession(t *testing.T) {
	t.Parallel()

	stream := &blockingWaitStream{done: make(chan struct{}), waitErr: errors.New("closed")}
	err := waitForStream(stream, 10*time.Millisecond)
	if err == nil || err.Error() != "closed" {
		t.Fatalf("expected closed error, got %v", err)
	}
	if stream.closeCalls == 0 {
		t.Fatalf("expected close to be called on timeout")
	}
}

type sendErrStream struct {
	err error
}

func (s *sendErrStream) SendAudio(_ []byte) error { return s.err }
func (s *sendErrStream) CloseSend() error         { return nil }
func (s *sendErrStream) Events() <-chan domain.TranscriptEvent {
	ch := make(chan domain.TranscriptEvent)
	close(ch)
	return ch
}
func (s *sendErrStream) Wait() error  { return nil }
func (s *sendErrStream) Close() error { return nil }

type errorAudioSession struct {
	err error
}

func (s *errorAudioSession) Read(_ []byte) (int, error) { return 0, s.err }
func (s *errorAudioSession) Close() error               { return nil }
func (s *errorAudioSession) Stop() error                { return nil }

type blockingWaitStream struct {
	done       chan struct{}
	waitErr    error
	closeCalls int
}

func (s *blockingWaitStream) SendAudio(_ []byte) error { return nil }
func (s *blockingWaitStream) CloseSend() error         { return nil }
func (s *blockingWaitStream) Events() <-chan domain.TranscriptEvent {
	ch := make(chan domain.TranscriptEvent)
	close(ch)
	return ch
}
func (s *blockingWaitStream) Wait() error {
	<-s.done
	return s.waitErr
}
func (s *blockingWaitStream) Close() error {
	s.closeCalls++
	close(s.done)
	return nil
}

var _ io.ReadCloser = (*errorAudioSession)(nil)
