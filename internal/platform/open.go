package platform

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"viff/internal/process"
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

// OpenURL opens an http(s) address in the system browser. Only those two
// schemes are accepted: the address comes from a repository's remote, which a
// clone controls, and handing a file: or a custom scheme to the opener would let
// that remote run something else. The URL is passed as one argument, never
// through a shell.
func OpenURL(raw string) error {
	u, err := webURL(raw)
	if err != nil {
		return err
	}
	addr := u.String()
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", addr).Start()
	case "windows":
		// rundll32 hands the address to the registered protocol handler in the
		// user's browser without opening a console window first.
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", addr)
		process.HideConsoleWindow(cmd)
		err = cmd.Start()
	default:
		err = exec.Command("xdg-open", addr).Start()
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", addr, err)
	}
	return nil
}

// webURL is the check OpenURL applies before it starts anything: an http(s)
// address with a host. It is separate so a test can cover the accepted forms
// without opening a browser.
func webURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("open url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("open url: %q is not an http(s) address", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("open url: %q names no host", raw)
	}
	return u, nil
}

// OpenWith opens path with a configured external command ("code", "idea"),
// or the system default when command is empty.
func OpenWith(command, path string, line int) error {
	if strings.TrimSpace(command) == "" {
		switch runtime.GOOS {
		case "darwin":
			return exec.Command("open", path).Start()
		case "windows":
			cmd := exec.Command("cmd", "/c", "start", "", path)
			process.HideConsoleWindow(cmd)
			return cmd.Start()
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
		// Hide only the wrapper console; "start" opens the requested window.
		process.HideConsoleWindow(cmd)
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
