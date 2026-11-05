//go:build !windows

package process

import (
	"os/exec"
	"testing"
)

func TestHideConsoleWindowLeavesCommandAlone(t *testing.T) {
	cmd := exec.Command("git")
	HideConsoleWindow(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatalf("SysProcAttr = %+v, want nil outside Windows", cmd.SysProcAttr)
	}
}
