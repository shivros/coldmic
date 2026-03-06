package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMainHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	dashIndex := -1
	for i, arg := range os.Args {
		if arg == "--" {
			dashIndex = i
			break
		}
	}

	if dashIndex >= 0 {
		os.Args = append([]string{"coldmic"}, os.Args[dashIndex+1:]...)
	} else {
		os.Args = []string{"coldmic"}
	}

	main()
}

func TestMainNoArgsToggleOffExitGeneric(t *testing.T) {
	output, code := runMainSubprocess(t, nil, map[string]string{
		"COLDMIC_TOGGLE_COMPAT": "false",
	})
	if code != exitGeneric {
		t.Fatalf("expected exit %d, got %d", exitGeneric, code)
	}
	if !strings.Contains(output, "missing command") {
		t.Fatalf("expected missing command error, got: %s", output)
	}
}

func TestMainUnknownCommandExitGeneric(t *testing.T) {
	output, code := runMainSubprocess(t, []string{"wat"}, nil)
	if code != exitGeneric {
		t.Fatalf("expected exit %d, got %d", exitGeneric, code)
	}
	if !strings.Contains(output, "unknown command: wat") {
		t.Fatalf("expected unknown command error, got: %s", output)
	}
}

func TestMainHelpExitOK(t *testing.T) {
	output, code := runMainSubprocess(t, []string{"help"}, nil)
	if code != exitOK {
		t.Fatalf("expected exit %d, got %d", exitOK, code)
	}
	if !strings.Contains(output, "Usage: coldmic") {
		t.Fatalf("expected usage output, got: %s", output)
	}
}

func runMainSubprocess(t *testing.T, args []string, env map[string]string) (string, int) {
	t.Helper()

	cmdArgs := []string{"-test.run=TestMainHelperProcess", "--"}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(os.Args[0], cmdArgs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(output), exitErr.ExitCode()
	}
	t.Fatalf("subprocess failed: %v", err)
	return "", -1
}
