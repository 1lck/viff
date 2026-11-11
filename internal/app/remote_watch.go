package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"viff/internal/domain"
	"viff/internal/i18n"
)

// A remote repository has no file-system events: the server may run hooks,
// editors and other terminals, and none of that reaches this process. The
// watcher therefore polls one cheap `git status` and asks for a full refresh
// only when the answer changed. The interval is deliberately conservative;
// it is a network round trip, unlike a local event.
const (
	remotePollEvery   = 5 * time.Second
	remotePollTimeout = 20 * time.Second
	// Refs change without a working-tree change (a tag or branch created on
	// the server), and `git status` cannot see that. A full metadata refresh
	// every sixth poll keeps the branch/tag list and operation state moving
	// without paying the full query set every five seconds.
	remoteMetaEvery = 6
)

type remoteWatcher struct {
	once   sync.Once
	stop   chan struct{}
	cancel context.CancelFunc
}

func (p *remoteWatcher) Close() {
	p.once.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		close(p.stop)
	})
}

// startRemoteWatch replaces the file watcher of an SSH repository. It returns
// the same Watcher contract so closing a repository or switching away stops
// the goroutine exactly like a local watch.
func (w *Workspace) startRemoteWatch(repo domain.Repo) Watcher {
	base, cancel := context.WithCancel(context.Background())
	p := &remoteWatcher{stop: make(chan struct{}), cancel: cancel}
	state := w.Repo
	git := w.Git
	pollEvery := w.remotePollEvery
	if pollEvery <= 0 {
		pollEvery = remotePollEvery
	}
	metaEvery := w.remoteMetaEvery
	if metaEvery <= 0 {
		metaEvery = remoteMetaEvery
	}
	last := ""
	if state != nil && state.StatusLoaded {
		last = remoteStatusKey(state.Status)
	}
	go func() {
		ticker := time.NewTicker(pollEvery)
		defer ticker.Stop()
		notified := false
		ticks := 0
		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
			}
			ctx, cancelStatus := context.WithTimeout(base, remotePollTimeout)
			status, err := git.Status(ctx, repo)
			cancelStatus()
			if err != nil {
				if base.Err() != nil {
					return
				}
				if !notified {
					notified = true
					w.post(func() {
						if w.Repo == state {
							w.NotifyError(w.tr(i18n.AppNoticeRemoteRefreshFailed), err)
						}
					})
				}
				continue
			}
			notified = false
			ticks++
			key := remoteStatusKey(status)
			if key == last && ticks%metaEvery != 0 {
				continue
			}
			started := make(chan bool, 1)
			w.post(func() {
				if w.Repo != state {
					started <- false
					return
				}
				started <- w.refreshIfIdle(true)
			})
			select {
			case ok := <-started:
				if key != last && ok {
					last = key
				}
			case <-p.stop:
				return
			}
		}
	}()
	return p
}

// remoteStatusKey is the complete observable state of a status snapshot: a
// change in any entry, mode, origin or branch counter means the view must be
// rebuilt. A hash would be smaller, but the string is bounded by the number of
// changed files and avoids a collision argument for no measurable cost.
func remoteStatusKey(s domain.Status) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\x1f%s\x1f%t\x1f%s\x1f%d\x1f%d\x1f%t\x1f%d\x1f%x",
		s.Branch.Head, s.Branch.Branch, s.Branch.Detached, s.Branch.Upstream,
		s.Branch.Ahead, s.Branch.Behind, s.Branch.HasAB, s.Branch.Stashes, s.IndexKey)
	for _, e := range s.Entries {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "%s\x1f%s\x1f%c\x1f%c\x1f%c\x1f%s\x1f%s\x1f%s\x1f%s",
			e.Path, e.OrigPath, e.Index, e.Worktree, e.Props, e.Submodule,
			e.ModeHead, e.ModeIndex, e.ModeWork)
	}
	return b.String()
}
