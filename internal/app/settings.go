package app

import (
	"viff/internal/gitcli"
	"viff/internal/storage"
)

// Settings is the persisted configuration.
type Settings = storage.Settings

// ApplyGitSettings copies the Git settings into the runner. Commands started
// afterwards use them; a command already running keeps the values it started
// with, so editing settings never changes a command halfway through.
//
// It runs when a workspace is created and whenever the UI saves settings,
// which is what makes a change effective on the next command rather than on
// the next start.
func (w *Workspace) ApplyGitSettings() {
	w.Git.SetConfig(gitcli.Config{
		SSHCommand:       w.Settings.SSHCommand,
		GPGProgram:       w.Settings.GPGProgram,
		SigningKey:       w.Settings.SigningKey,
		SignFormat:       w.Settings.SignFormat,
		CommitSigning:    w.Settings.CommitSigning,
		CredentialHelper: w.Settings.CredentialHelper,
	})
}
