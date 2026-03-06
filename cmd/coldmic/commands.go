package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
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

type SessionClient interface {
	Start(ctx context.Context) (domain.Status, error)
	Stop(ctx context.Context) (domain.Status, domain.StopResult, error)
	Abort(ctx context.Context) (domain.Status, error)
	Status(ctx context.Context) (domain.Status, error)
	Transcript(ctx context.Context) (time.Time, domain.StopResult, error)
}

type sessionClientFactory func(daemonURL string) SessionClient

type configProvider interface {
	DaemonURL() string
	ToggleCompatEnabled() bool
}

type envConfigProvider struct{}

func (envConfigProvider) DaemonURL() string {
	return envOrDefault("COLDMIC_DAEMON_URL", "http://127.0.0.1:4317")
}

func (envConfigProvider) ToggleCompatEnabled() bool {
	return toggleCompatFromEnv("COLDMIC_TOGGLE_COMPAT")
}

type commandSpec struct {
	name    string
	summary string
	handler func(args []string) (int, error)
}

type CommandRunner struct {
	clientFactory sessionClientFactory
	config        configProvider
	stdout        io.Writer
	stderr        io.Writer

	commands     map[string]commandSpec
	commandOrder []string
}

func NewCommandRunner(factory sessionClientFactory, cfg configProvider, stdout io.Writer, stderr io.Writer) *CommandRunner {
	if factory == nil {
		factory = func(daemonURL string) SessionClient {
			return coldcli.NewClient(daemonURL)
		}
	}
	if cfg == nil {
		cfg = envConfigProvider{}
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	r := &CommandRunner{
		clientFactory: factory,
		config:        cfg,
		stdout:        stdout,
		stderr:        stderr,
		commands:      make(map[string]commandSpec),
	}
	r.registerCommands()
	return r
}

func (r *CommandRunner) registerCommands() {
	r.register("start", "Start a recording session", r.runStart)
	r.register("stop", "Stop recording and output final transcript", r.runStop)
	r.register("abort", "Abort recording and discard captured audio", r.runAbort)
	r.register("status", "Show current recording state", r.runStatus)
	r.register("transcript", "Show latest final transcript", r.runTranscript)
	r.register("help", "Show this help text", r.runHelp)
	r.commands["-h"] = r.commands["help"]
	r.commands["--help"] = r.commands["help"]
}

func (r *CommandRunner) register(name string, summary string, handler func(args []string) (int, error)) {
	r.commands[name] = commandSpec{name: name, summary: summary, handler: handler}
	r.commandOrder = append(r.commandOrder, name)
}

func (r *CommandRunner) Run(cmd string, args []string) (int, error) {
	spec, ok := r.commands[cmd]
	if !ok {
		r.printUsage()
		return exitGeneric, fmt.Errorf("unknown command: %s", cmd)
	}
	return spec.handler(args)
}

func (r *CommandRunner) RunNoCommand() (int, error) {
	if !r.config.ToggleCompatEnabled() {
		r.printUsage()
		return exitGeneric, fmt.Errorf("missing command")
	}

	status, err := r.clientFactory(r.config.DaemonURL()).Status(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}
	if status.Active {
		return r.runStop(nil)
	}
	return r.runStart(nil)
}

func runCommand(cmd string, args []string) (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).Run(cmd, args)
}

func runNoCommand() (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).RunNoCommand()
}

func runStart(args []string) (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).runStart(args)
}

func runStop(args []string) (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).runStop(args)
}

func runAbort(args []string) (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).runAbort(args)
}

func runStatus(args []string) (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).runStatus(args)
}

func runTranscript(args []string) (int, error) {
	return NewCommandRunner(nil, nil, nil, nil).runTranscript(args)
}

func (r *CommandRunner) runHelp(_ []string) (int, error) {
	r.printUsage()
	return exitOK, nil
}

func (r *CommandRunner) runStart(args []string) (int, error) {
	cfg, err := r.parseCommonFlags("start", args)
	if err != nil {
		return exitGeneric, err
	}

	status, err := r.clientFactory(cfg.daemonURL).Start(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(r.stdout, cliStatusOutput{Status: status})
	} else {
		printStatus(r.stdout, status)
	}
	return exitOK, nil
}

func (r *CommandRunner) runStop(args []string) (int, error) {
	cfg, err := r.parseCommonFlags("stop", args)
	if err != nil {
		return exitGeneric, err
	}

	status, result, err := r.clientFactory(cfg.daemonURL).Stop(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(r.stdout, cliStopOutput{Status: status, Result: result})
	} else {
		printStopResult(r.stdout, status, result)
	}
	return exitOK, nil
}

func (r *CommandRunner) runAbort(args []string) (int, error) {
	cfg, err := r.parseCommonFlags("abort", args)
	if err != nil {
		return exitGeneric, err
	}

	status, err := r.clientFactory(cfg.daemonURL).Abort(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(r.stdout, cliStatusOutput{Status: status})
	} else {
		printStatus(r.stdout, status)
	}
	return exitOK, nil
}

func (r *CommandRunner) runStatus(args []string) (int, error) {
	cfg, err := r.parseStatusFlags(args)
	if err != nil {
		return exitGeneric, err
	}

	status, err := r.clientFactory(cfg.daemonURL).Status(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.checkOnly {
		if status.Active {
			return exitOK, nil
		}
		return exitGeneric, nil
	}

	if cfg.outputJSON {
		writeJSON(r.stdout, cliStatusOutput{Status: status})
	} else {
		printStatus(r.stdout, status)
	}
	return exitOK, nil
}

func (r *CommandRunner) runTranscript(args []string) (int, error) {
	cfg, err := r.parseCommonFlags("transcript", args)
	if err != nil {
		return exitGeneric, err
	}

	capturedAt, result, err := r.clientFactory(cfg.daemonURL).Transcript(context.Background())
	if err != nil {
		return mapErrorToExitCode(err), err
	}

	if cfg.outputJSON {
		writeJSON(r.stdout, cliTranscriptOutput{CapturedAt: capturedAt, Result: result})
	} else {
		printTranscript(r.stdout, capturedAt, result)
	}
	return exitOK, nil
}

type commonFlags struct {
	daemonURL  string
	outputJSON bool
}

type statusFlags struct {
	commonFlags
	checkOnly bool
}

func parseCommonFlags(name string, args []string) (*commonFlags, error) {
	return NewCommandRunner(nil, nil, nil, nil).parseCommonFlags(name, args)
}

func (r *CommandRunner) parseCommonFlags(name string, args []string) (*commonFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(r.stderr)

	cfg := &commonFlags{}
	fs.StringVar(&cfg.daemonURL, "daemon-url", r.config.DaemonURL(), "coldmic daemon base URL")
	fs.BoolVar(&cfg.outputJSON, "json", false, "emit JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (r *CommandRunner) parseStatusFlags(args []string) (*statusFlags, error) {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(r.stderr)

	cfg := &statusFlags{}
	fs.StringVar(&cfg.daemonURL, "daemon-url", r.config.DaemonURL(), "coldmic daemon base URL")
	fs.BoolVar(&cfg.outputJSON, "json", false, "emit JSON output")
	fs.BoolVar(&cfg.checkOnly, "check", false, "exit 0 when active, 1 when idle")
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
	NewCommandRunner(nil, nil, nil, nil).printUsage()
}

func (r *CommandRunner) printUsage() {
	fmt.Fprintln(r.stdout, "Usage: coldmic <command> [flags]")
	fmt.Fprintln(r.stdout, "")
	fmt.Fprintln(r.stdout, "Commands:")
	for _, name := range r.commandOrder {
		spec := r.commands[name]
		fmt.Fprintf(r.stdout, "  %-10s %s\n", spec.name, spec.summary)
	}
	fmt.Fprintln(r.stdout, "")
	fmt.Fprintln(r.stdout, "Global flags per command:")
	fmt.Fprintln(r.stdout, "  --daemon-url URL  Daemon URL (default: COLDMIC_DAEMON_URL or http://127.0.0.1:4317)")
	fmt.Fprintln(r.stdout, "  --json            Emit JSON output")
	fmt.Fprintln(r.stdout, "")
	fmt.Fprintln(r.stdout, "Status flags:")
	fmt.Fprintln(r.stdout, "  --check           Exit 0 when active, 1 when idle (no output)")
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

func toggleCompatEnabled() bool {
	return toggleCompatFromEnv("COLDMIC_TOGGLE_COMPAT")
}

func toggleCompatFromEnv(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	return strings.EqualFold(value, "true")
}
