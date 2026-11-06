//go:build !darwin

package platform

import (
	"time"

	"viff/internal/domain"
	"viff/internal/wslpath"
)

// Watch falls back to a bounded check on platforms without a native watcher
// wired up yet: the Git directory every 2 seconds and the working tree every
// 6 seconds. The design records native inotify/ReadDirectoryChangesW
// watchers as follow-up work.
//
// A WSL repository is watched more slowly: every poll round-trips the 9P file
// system behind \\wsl.localhost, where a stat costs orders of magnitude more
// than on a local disk. Five seconds keeps changes visible while leaving the
// redirector room; tuning belongs to a measurement, not a guess (ADR 0037).
func Watch(repo domain.Repo, changed func(meta bool)) (Watcher, error) {
	interval, worktreeEvery := 2*time.Second, 3
	if wslpath.Is(repo.Root) {
		interval, worktreeEvery = 5*time.Second, 3
	}
	return startPolling(repo, interval, worktreeEvery, changed), nil
}
