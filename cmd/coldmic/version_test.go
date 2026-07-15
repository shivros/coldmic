package main

import (
	"bytes"
	"strings"
	"testing"

	"coldmic/internal/version"
)

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := NewCommandRunner(nil, nil, &stdout, &stderr)

	code, err := r.runVersion(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != exitOK {
		t.Fatalf("expected exit code %d, got %d", exitOK, code)
	}

	output := stdout.String()
	if !strings.Contains(output, version.Version) {
		t.Errorf("expected output to contain version %q, got %q", version.Version, output)
	}
	if !strings.Contains(output, "coldmic") {
		t.Errorf("expected output to contain 'coldmic', got %q", output)
	}
}

func TestVersionCommandRegistered(t *testing.T) {
	r := NewCommandRunner(nil, nil, nil, nil)

	aliases := []string{"version", "--version", "-v"}
	for _, alias := range aliases {
		spec, ok := r.commands[alias]
		if !ok {
			t.Errorf("command alias %q not registered", alias)
			continue
		}
		if spec.handler == nil {
			t.Errorf("command alias %q has nil handler", alias)
		}
	}
}
