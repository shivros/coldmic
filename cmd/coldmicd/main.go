package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coldmic/internal/bootstrap"
	"coldmic/internal/daemon"
	"coldmic/internal/debuglog"
	"coldmic/internal/ports"
)

func main() {
	addrDefault := envOrDefault("COLDMIC_DAEMON_ADDR", "127.0.0.1:4317")
	addr := flag.String("addr", addrDefault, "daemon bind address")
	flag.Parse()

	var eventSink ports.EventSink = daemon.NoopEventSink{}
	if debuglog.Enabled() {
		eventSink = daemon.LoggingEventSink{}
	}

	services, err := bootstrap.Build(eventSink, daemon.SystemClipboard{})
	if err != nil {
		log.Fatalf("coldmicd bootstrap failed: %v", err)
	}
	debuglog.Printf(
		"config provider=deepgram model=%s language=%q smart_format=%t audio_format=%s audio_device=%s sample_rate=%d channels=%d rules_file=%q chunk_size=%d streaming_grace_ms=%d api_key_set=%t",
		services.Config.Deepgram.Model,
		services.Config.Deepgram.Language,
		services.Config.Deepgram.SmartFormat,
		services.Config.Audio.InputFormat,
		services.Config.Audio.InputDevice,
		services.Config.Audio.SampleRate,
		services.Config.Audio.Channels,
		services.Config.Rules.Path,
		services.Config.Session.ChunkSize,
		services.Config.Session.StreamingGrace/time.Millisecond,
		services.Config.Deepgram.APIKey != "",
	)

	api := daemon.NewAPI(services.Session)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("coldmicd listening on %s", *addr)
		if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	case err := <-errCh:
		if err != nil {
			log.Fatalf("coldmicd server failed: %v", err)
		}
	}

	fmt.Println("coldmicd stopped")
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
