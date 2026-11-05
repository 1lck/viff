package app

import (
	"context"
	"time"
)

// WorktreeFiles lists the files of the working tree (index plus untracked
// files, ignored files excluded) in the background and delivers them on the UI
// thread. Git stays the source of truth: the Files browser asks again whenever
// it opens or refreshes, and the workspace keeps no cached path list. A result
// that arrives after the repository changed is dropped instead of being shown
// for the wrong repository.
//
// This is a read query: it takes no write lock and does not trigger a refresh.
func (w *Workspace) WorktreeFiles(done func([]string, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		list, err := w.Git.WorktreeFiles(ctx, repo)
		w.post(func() {
			if w.Repo == rs {
				done(list, err)
			}
		})
	}()
}
