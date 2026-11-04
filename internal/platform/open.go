package platform

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Reveal shows path in the system file manager.
func Reveal(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,", filepath.FromSlash(path)).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}

// OpenWith opens path with a configured external command ("code", "idea"),
// or the system default when command is empty.
func OpenWith(command, path string, line int) error {
	if strings.TrimSpace(command) == "" {
		switch runtime.GOOS {
		case "darwin":
			return exec.Command("open", path).Start()
		case "windows":
			return exec.Command("cmd", "/c", "start", "", path).Start()
		}
		return exec.Command("xdg-open", path).Start()
	}
	fields := strings.Fields(command)
	args := append(fields[1:], path)
	return exec.Command(fields[0], args...).Start()
}

// OpenTerminal opens a terminal in dir.
func OpenTerminal(dir string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Terminal", dir).Start()
	case "windows":
		cmd := exec.Command("cmd", "/c", "start", "cmd")
		cmd.Dir = dir
		return cmd.Start()
	}
	for _, t := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
		if p, err := exec.LookPath(t); err == nil {
			cmd := exec.Command(p)
			cmd.Dir = dir
			return cmd.Start()
		}
	}
	return exec.ErrNotFound
}
