package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	coldcli "coldmic/internal/cli"
	"coldmic/internal/domain"
)

const (
	exitOK            = 0
	exitGeneric       = 1
	exitDaemonOffline = 2
	exitConflict      = 3
	exitNotFound      = 4
)

func runCommand(cmd string, args []string) (int, error) {
	switch cmd {
	case "start":
		return runStart(args)
	case "stop":
		return runStop(args)
	case "status":
		return runStatus(args)
	case "transcript":
		return runTranscript(args)
	case "help", "-h", "--help":
		printUsage()
		return exitOK, nil
	default:
		printUsage()
		return exitGeneric, fmt.Errorf("unknown command: %s", cmd)
	}
}

func runStart(args []string) (int, error) {
	cfg, err := parseCommonFlags("start", args)
	if err != nil {
		return exitGeneric, err
	}

	status, err := coldcli.NewClient(cfg.daemonURL).Start(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(os.Stdout, cliStatusOutput{Status: status})
	} else {
		printStatus(os.Stdout, status)
	}
	return exitOK, nil
}

func runStop(args []string) (int, error) {
	cfg, err := parseCommonFlags("stop", args)
	if err != nil {
		return exitGeneric, err
	}

	status, result, err := coldcli.NewClient(cfg.daemonURL).Stop(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(os.Stdout, cliStopOutput{Status: status, Result: result})
	} else {
		printStopResult(os.Stdout, status, result)
	}
	return exitOK, nil
}

func runStatus(args []string) (int, error) {
	cfg, err := parseCommonFlags("status", args)
	if err != nil {
		return exitGeneric, err
	}

	status, err := coldcli.NewClient(cfg.daemonURL).Status(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(os.Stdout, cliStatusOutput{Status: status})
	} else {
		printStatus(os.Stdout, status)
	}
	return exitOK, nil
}

func runTranscript(args []string) (int, error) {
	cfg, err := parseCommonFlags("transcript", args)
	if err != nil {
		return exitGeneric, err
	}

	capturedAt, result, err := coldcli.NewClient(cfg.daemonURL).Transcript(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(os.Stdout, cliTranscriptOutput{CapturedAt: capturedAt, Result: result})
	} else {
		printTranscript(os.Stdout, capturedAt, result)
	}
	return exitOK, nil
}

type commonFlags struct {
	daemonURL  string
	outputJSON bool
}

func parseCommonFlags(name string, args []string) (*commonFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	cfg := &commonFlags{}
	defaultURL := envOrDefault("COLDMIC_DAEMON_URL", "http://127.0.0.1:4317")
	fs.StringVar(&cfg.daemonURL, "daemon-url", defaultURL, "coldmic daemon base URL")
	fs.BoolVar(&cfg.outputJSON, "json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cfg, nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func printUsage() {
	fmt.Println("Usage: coldmic <command> [flags]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  start       Start a recording session")
	fmt.Println("  stop        Stop recording and output final transcript")
	fmt.Println("  status      Show current recording state")
	fmt.Println("  transcript  Show latest final transcript")
	fmt.Println("")
	fmt.Println("Global flags per command:")
	fmt.Println("  --daemon-url URL  Daemon URL (default: COLDMIC_DAEMON_URL or http://127.0.0.1:4317)")
	fmt.Println("  --json            Emit JSON output")
}

func printTranscriptTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

type cliStatusOutput struct {
	Status domain.Status `json:"status"`
}

type cliStopOutput struct {
	Status domain.Status     `json:"status"`
	Result domain.StopResult `json:"result"`
}

type cliTranscriptOutput struct {
	CapturedAt time.Time         `json:"capturedAt"`
	Result     domain.StopResult `json:"result"`
}
