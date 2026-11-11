//go:build unix

package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// RunTree puts a context-bound command in a separate process group so
// canceling Git also stops the git-lfs scan it launched.
func RunTree(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
	}
	return cmd.Run()
}
