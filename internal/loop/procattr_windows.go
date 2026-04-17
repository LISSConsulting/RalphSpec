//go:build windows

package loop

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// isolateProcess puts the child process in a new process group so that
// console Ctrl+C is not forwarded to it. Ralph handles graceful stop itself.
func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessTree terminates proc and its descendants. Windows has no native
// kill-by-pgid primitive, so we shell out to `taskkill /T /F` which walks the
// parent-child chain. Falls back to proc.Kill() when taskkill is unavailable.
func killProcessTree(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	cmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", proc.Pid), "/T", "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err == nil {
		return nil
	}
	return proc.Kill()
}
