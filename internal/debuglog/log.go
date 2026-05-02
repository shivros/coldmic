package debuglog

import (
	"log"
	"os"
	"strings"
	"sync"
)

var (
	enabledOnce sync.Once
	enabled     bool
)

func Enabled() bool {
	enabledOnce.Do(func() {
		switch strings.ToLower(strings.TrimSpace(os.Getenv("COLDMIC_DEBUG"))) {
		case "1", "true", "yes", "on", "debug":
			enabled = true
		}
	})
	return enabled
}

func Printf(format string, args ...any) {
	if !Enabled() {
		return
	}
	log.Printf("debug: "+format, args...)
}
