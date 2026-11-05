// Package pty runs the user's login shell on a pseudo-terminal of this
// machine. Output is buffered for the window thread to drain, input is queued
// without blocking it, and the owner is woken when something arrives. The
// pseudo-terminal itself is a small per-platform backend: a pty on Unix,
// ConPTY on Windows.
//
// Derived from Motty (https://github.com/t8y2/motty), Copyright 2026 Motty
// contributors, Apache-2.0. Modified for Viff: the Windows Subsystem for Linux
// and the directory-report integration are not included, and shell discovery is
// left to the caller.
package pty

import (
	"errors"
	"fmt"
)

const (
	// maxPendingOutput is how much output may wait for the window thread before
	// the reader pauses, which in turn pauses the program through the pty.
	maxPendingOutput = 1 << 20
	// maxPendingInput is how much typed or pasted input may wait to be written.
	maxPendingInput = 4 << 20
)

// Config describes the shell to start.
type Config struct {
	// Cols and Rows are the terminal size in cells.
	Cols, Rows int
	// Shell is the program to run. Empty means the platform's default shell.
	Shell string
	// Args starts the program with specific arguments. Nil preserves the
	// platform's normal interactive shell behavior.
	Args []string
	// Dir is the working directory. Empty means the user's home directory.
	Dir string
	// Env is added to the environment the shell inherits.
	Env []string
	// Notify is called after output arrives and when the shell ends, from any
	// goroutine; it must not block.
	Notify func()
}

// ExitError is why a shell ended with a nonzero status.
type ExitError struct{ Code int }

func (err *ExitError) Error() string   { return fmt.Sprintf("shell exited with status %d", err.Code) }
func (err *ExitError) ExitStatus() int { return err.Code }

// ErrNoShell reports that the configured program could not be started.
var ErrNoShell = errors.New("pty: no shell to start")
