//go:build !windows

package loop

import (
	"os"
	"os/exec"
	"syscall"
)

// isolateProcess puts the child process in its own process group so that
// SIGINT from Ctrl+C is not forwarded to it. Ralph handles graceful stop itself.
func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessTree sends SIGKILL to the entire process group of proc so that
// Claude's own child processes (bash tools, etc.) die with it rather than
// becoming orphans. Falls back to killing just proc when the pgid lookup fails.
func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	if pgid, err := syscall.Getpgid(proc.Pid); err == nil {
		return syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return proc.Kill()
}
