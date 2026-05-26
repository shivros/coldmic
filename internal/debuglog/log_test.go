package debuglog

import (
	"sync"
	"testing"
)

func resetDebuglogForTest() {
	enabledOnce = sync.Once{}
	enabled = false
}

func TestEnabledFalseByDefault(t *testing.T) {
	resetDebuglogForTest()
	t.Setenv("COLDMIC_DEBUG", "")
	if Enabled() {
		t.Fatal("expected debug logging disabled by default")
	}
}

func TestEnabledTrueValues(t *testing.T) {
	truthy := []string{"1", "true", "yes", "on", "debug", "TRUE", " Yes "}
	for _, v := range truthy {
		resetDebuglogForTest()
		t.Setenv("COLDMIC_DEBUG", v)
		if !Enabled() {
			t.Fatalf("expected Enabled() true for %q", v)
		}
	}
}

func TestPrintfNoPanicWhenDisabledOrEnabled(t *testing.T) {
	resetDebuglogForTest()
	t.Setenv("COLDMIC_DEBUG", "")
	Printf("disabled")

	resetDebuglogForTest()
	t.Setenv("COLDMIC_DEBUG", "1")
	Printf("enabled %d", 42)
}
