//go:build windows

package process

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: the child runs with a console that has
// no window, so Windows never paints one.
const createNoWindow = 0x08000000

// HideConsoleWindow keeps Windows from creating a visible console window for a
// console child process. The flags are merged into an existing SysProcAttr so
// callers that set application-specific attributes keep them.
func HideConsoleWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	attr := cmd.SysProcAttr
	if attr == nil {
		attr = &syscall.SysProcAttr{}
		cmd.SysProcAttr = attr
	}
	attr.HideWindow = true
	attr.CreationFlags |= createNoWindow
}
