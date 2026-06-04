//go:build !windows

package whispercpp

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup puts cmd in its own process group so we can kill the
// entire tree on cancellation (including grandchild processes).
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the whole process group rooted at cmd.
func killProcessGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
