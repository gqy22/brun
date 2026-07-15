//go:build linux

package main

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// configureManagedCommand gives every benchmark subprocess its own process
// group. Cancelling the context then stops the complete command tree instead
// of only /usr/bin/time or the immediate shell.
func configureManagedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pid := cmd.Process.Pid
		err := syscall.Kill(-pid, syscall.SIGTERM)
		if err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		time.AfterFunc(2*time.Second, func() {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		})
		return nil
	}
	cmd.WaitDelay = 3 * time.Second
}
