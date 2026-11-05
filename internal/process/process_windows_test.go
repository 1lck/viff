//go:build windows

package process

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestHideConsoleWindowSetsConsoleFlags(t *testing.T) {
	cmd := exec.Command("git")
	HideConsoleWindow(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, console window would flash")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow is false")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW set", cmd.SysProcAttr.CreationFlags)
	}
}

func TestHideConsoleWindowKeepsExistingCreationFlags(t *testing.T) {
	const createNewProcessGroup = 0x00000200
	cmd := exec.Command("git")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
	HideConsoleWindow(cmd)
	if got := cmd.SysProcAttr.CreationFlags; got&createNewProcessGroup == 0 {
		t.Errorf("CreationFlags = %#x, existing CREATE_NEW_PROCESS_GROUP bit was lost", got)
	}
	if got := cmd.SysProcAttr.CreationFlags; got&createNoWindow == 0 {
		t.Errorf("CreationFlags = %#x, want CREATE_NO_WINDOW set", got)
	}
}

func TestHideConsoleWindowAcceptsNilCommand(t *testing.T) {
	HideConsoleWindow(nil)
}
