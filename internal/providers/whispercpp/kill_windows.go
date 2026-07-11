//go:build windows

package whispercpp

import "os/exec"

// configureProcessGroup is a no-op on Windows — process groups work differently.
func configureProcessGroup(_ *exec.Cmd) {}

// killProcessGroup kills the direct process on Windows. Process-group signals
// are not supported; child processes must handle cleanup via their own means.
func killProcessGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
