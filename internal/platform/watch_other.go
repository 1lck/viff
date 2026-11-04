//go:build !darwin

package platform

import (
	"time"

	"viff/internal/domain"
)

// Watch falls back to a bounded check on platforms without a native watcher
// wired up yet: the Git directory every 2 seconds and the working tree every
// 6 seconds. The design records native inotify/ReadDirectoryChangesW
// watchers as follow-up work.
func Watch(repo domain.Repo, changed func(meta bool)) (Watcher, error) {
	return startPolling(repo, 2*time.Second, 3, changed), nil
}
