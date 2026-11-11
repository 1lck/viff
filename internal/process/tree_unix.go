//go:build unix

package process

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// RunTree owns a process group until cancellation or command completion,
// including descendants left behind when Git exits before git-lfs.
func RunTree(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	kill := func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	if cmd.Cancel != nil {
		cmd.Cancel = kill
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	if cleanupErr := kill(); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrProcessDone) {
		return errors.Join(err, fmt.Errorf("kill process group: %w", cleanupErr))
	}
	return err
}
