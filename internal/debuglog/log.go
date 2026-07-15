package debuglog

import (
	"log"
	"os"
	"strings"
	"sync"

	"coldmic/internal/config"
)

var (
	enabledOnce sync.Once
	enabled     bool
)

func Enabled() bool {
	enabledOnce.Do(func() {
		cfg, err := config.Load()
		if err == nil && cfg.Daemon.Debug {
			enabled = true
			return
		}
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
