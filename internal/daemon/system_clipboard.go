package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

var (
	clipboardCommandsFn   = clipboardCommands
	lookPathFn            = exec.LookPath
	runClipboardCommandFn = runClipboardCommand
)

// SystemClipboard writes transcript text to the host clipboard in daemon mode.
type SystemClipboard struct{}

func (SystemClipboard) SetText(ctx context.Context, text string) error {
	candidates := clipboardCommandsFn()
	var lastErr error

	for _, candidate := range candidates {
		if _, err := lookPathFn(candidate[0]); err != nil {
			lastErr = err
			continue
		}
		if err := runClipboardCommandFn(ctx, candidate, text); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = errors.New("no clipboard command available")
	}
	return fmt.Errorf("clipboard unavailable: %w", lastErr)
}

func clipboardCommands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{{"clip.exe"}}
	default:
		return [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
		}
	}
}

func runClipboardCommand(ctx context.Context, args []string, text string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	_, writeErr := io.WriteString(stdin, text)
	closeErr := stdin.Close()
	waitErr := cmd.Wait()

	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return waitErr
}
