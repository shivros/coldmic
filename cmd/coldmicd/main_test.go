package main

import "testing"

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("COLDMIC_DAEMON_ADDR", "127.0.0.1:9000")
	if got := envOrDefault("COLDMIC_DAEMON_ADDR", "127.0.0.1:4317"); got != "127.0.0.1:9000" {
		t.Fatalf("unexpected value: %s", got)
	}
	if got := envOrDefault("COLDMIC_MISSING_ENV", "fallback"); got != "fallback" {
		t.Fatalf("unexpected fallback: %s", got)
	}
}
