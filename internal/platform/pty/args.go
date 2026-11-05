package pty

import "strings"

// ArgsFor is how an interactive session starts the chosen program: Git Bash
// needs its login startup files, or /etc/profile and the bundled tools that live
// beside the binary are missing from the session.
func ArgsFor(program string) []string {
	name := program
	if cut := strings.LastIndexAny(name, `\/`); cut >= 0 {
		name = name[cut+1:]
	}
	if strings.EqualFold(name, "bash.exe") {
		return []string{"--login", "-i"}
	}
	return nil
}
