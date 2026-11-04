// Package app holds Viff's use cases: opening repositories, querying their
// state, running Git commands as tasks, and turning file-system changes into
// refreshes. Git stays the source of truth: every snapshot here is a cache
// that is rebuilt by asking Git again.
//
// Threading: all state is read and changed on the UI thread. Background work
// runs in goroutines that only call Git and then post a closure that applies
// the result on the UI thread (Drain). A result carries the generation it was
// started for; one that arrives after the repository or the selection
// changed is dropped instead of overwriting the newer view.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// Git is the subset of the Git adapter the use cases need. It is satisfied by
// *gitcli.Runner; tests may pass the same runner with an isolated environment.
type Git = *gitcli.Runner

// Notification is a balloon shown to the user.
type Notification struct {
	ID      uint64
	Kind    NoticeKind
	Title   string
	Body    string
	Time    time.Time
	Actions []NoticeAction
	Sticky  bool
}

// NoticeKind is the severity of a notification.
type NoticeKind uint8

const (
	NoticeInfo NoticeKind = iota
	NoticeSuccess
	NoticeWarning
	NoticeError
)

// NoticeAction is a link in a notification.
type NoticeAction struct {
	Title string
	Run   func()
}

// ConsoleEntry is one Git command shown in the Console tab.
type ConsoleEntry struct {
	Time    time.Time
	Command string
	Output  string
	Failed  bool
	Took    time.Duration
}

// Task is a running command shown in the status bar.
type Task struct {
	ID       uint64
	Title    string
	Progress string
	Started  time.Time
	cancel   context.CancelFunc
	Write    bool
}

// Workspace is the application state for one window.
type Workspace struct {
	Git      Git
	Settings *Settings

	// Repo is the open repository, nil on the welcome screen.
	Repo *RepoState

	// Repositories is the explicit multi-repository list, in the order the user
	// added them. Only the active repository (Repo) runs Git commands and has a
	// file watcher; the other rows show their last known status and are
	// refreshed by RefreshRepositories.
	Repositories []RepositorySummary
	// ActiveRoot is the canonical root of the active repository, "" when none
	// is open. It is persisted with the list, so a restart can restore the
	// previous selection.
	ActiveRoot string
	// WorkspaceWarning reports a workspace file that could not be used as is
	// (unreadable, or written by a newer Viff and therefore read-only).
	WorkspaceWarning string
	// ChangeLists is the per-repository change-list bookkeeping. It is created
	// lazily by the change-list backend (changelists.go); a workspace that never
	// shows change lists keeps it nil.
	ChangeLists *ChangeListManager
	// changeListSyncWarned keeps a failing rename sync from notifying on every
	// status refresh; the next success clears it.
	changeListSyncWarned bool
	// workspaceSaveWarned keeps a failing workspace write from notifying on
	// every list change; the next success clears it.
	workspaceSaveWarned bool

	Notifications []Notification
	Console       []ConsoleEntry
	Tasks         []*Task

	// repos is the private state behind Repositories; see repositories.go.
	repos repositoriesState
	// nextGen is the repository generation counter. It only ever grows: a
	// generation is never reused after a repository is closed or removed, so a
	// stale view can never match a newer repository.
	nextGen uint64

	invalidate func()
	mu         sync.Mutex
	pending    []func()
	nextID     uint64
	watcher    Watcher
	// Watch starts file-system watching for a repository; set by the
	// composition root so platform code stays out of this package.
	Watch func(repo domain.Repo, changed func(meta bool)) (Watcher, error)
	// ConfirmCheckout asks the UI whether to stash local changes and retry a
	// checkout Git blocked. It runs on the UI thread and returns true when the
	// user accepted; nil reports the failure instead of offering the choice.
	// Changes are never stashed without this confirmation.
	ConfirmCheckout func(CheckoutBlocked) bool
	// Now is the clock (replaceable in tests).
	Now func() time.Time
}

// Watcher stops a file-system watch.
type Watcher interface{ Close() }

// NewWorkspace creates the state. invalidate is called from any goroutine
// when results are waiting; the UI then calls Drain on its thread.
func NewWorkspace(git Git, settings *Settings, invalidate func()) *Workspace {
	w := &Workspace{Git: git, Settings: settings, invalidate: invalidate, Now: time.Now}
	git.Log = w.logCommand
	w.loadRepositories()
	w.ApplyGitSettings()
	return w
}

// post queues fn to run on the UI thread.
func (w *Workspace) post(fn func()) {
	w.mu.Lock()
	w.pending = append(w.pending, fn)
	w.mu.Unlock()
	if w.invalidate != nil {
		w.invalidate()
	}
}

// Drain applies queued results. It returns whether anything changed.
func (w *Workspace) Drain() bool {
	w.mu.Lock()
	fns := w.pending
	w.pending = nil
	w.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
	return len(fns) > 0
}

// Pending reports queued results (for tests that wait for background work).
func (w *Workspace) Pending() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.pending) > 0
}

func (w *Workspace) id() uint64 {
	w.nextID++
	return w.nextID
}

const consoleLimit = 400

// logCommand records commands in the console. It runs on worker goroutines.
func (w *Workspace) logCommand(dir string, args []string, took time.Duration, err error) {
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "status", "log", "for-each-ref", "cat-file", "show", "diff-tree", "rev-parse", "ls-files", "stash", "reflog", "remote", "config", "worktree", "merge-base", "diff":
		if err == nil && (len(args) < 2 || args[0] != "stash" || args[1] == "list") {
			return // reads are not console noise unless they fail
		}
	}
	entry := ConsoleEntry{Time: time.Now(), Command: "git " + strings.Join(quoteArgs(args), " "), Took: took}
	if err != nil {
		entry.Failed = true
		var ge *gitcli.Error
		if errors.As(err, &ge) {
			entry.Output = strings.TrimSpace(ge.Stderr + "\n" + ge.Stdout)
		} else {
			entry.Output = err.Error()
		}
	}
	w.post(func() {
		w.Console = append(w.Console, entry)
		if len(w.Console) > consoleLimit {
			w.Console = append([]ConsoleEntry(nil), w.Console[len(w.Console)-consoleLimit:]...)
		}
	})
}

func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\n\"'") {
			out[i] = fmt.Sprintf("%q", a)
		} else {
			out[i] = a
		}
	}
	return out
}

// Notify adds a notification.
func (w *Workspace) Notify(kind NoticeKind, title, body string, actions ...NoticeAction) {
	n := Notification{ID: w.id(), Kind: kind, Title: title, Body: body, Time: w.Now(), Actions: actions}
	w.Notifications = append(w.Notifications, n)
	if len(w.Notifications) > 50 {
		w.Notifications = w.Notifications[len(w.Notifications)-50:]
	}
}

// Dismiss removes a notification.
func (w *Workspace) Dismiss(id uint64) {
	for i, n := range w.Notifications {
		if n.ID == id {
			w.Notifications = append(w.Notifications[:i:i], w.Notifications[i+1:]...)
			return
		}
	}
}

// warnChangeListSync reports a failed change-list rename sync once per failure
// streak. SyncChangeListRenames runs after every successful status refresh, so
// a persistent failure would otherwise repeat the warning on every refresh.
func (w *Workspace) warnChangeListSync(err error) {
	if err == nil {
		w.changeListSyncWarned = false
		return
	}
	if w.changeListSyncWarned {
		return
	}
	w.changeListSyncWarned = true
	w.Notify(NoticeWarning, "Change lists", err.Error())
}

// NotifyError shows a Git failure with its message.
func (w *Workspace) NotifyError(title string, err error) {
	if err == nil {
		return
	}
	if gitcli.KindOf(err) == gitcli.ErrCancelled {
		w.Notify(NoticeInfo, title, "Cancelled")
		return
	}
	w.Notify(NoticeError, title, ErrorText(err))
}

// ErrorText is the message of an error suitable for the UI, with a hint for
// failures the user can act on.
func ErrorText(err error) string {
	var ge *gitcli.Error
	msg := err.Error()
	if errors.As(err, &ge) {
		msg = ge.Message()
	}
	msg = strings.TrimPrefix(msg, "fatal: ")
	msg = strings.TrimPrefix(msg, "error: ")
	switch gitcli.KindOf(err) {
	case gitcli.ErrAuth:
		msg += "\n\nAuthentication failed. Viff uses your Git credential helper and SSH agent; configure them in a terminal and try again."
	case gitcli.ErrLocked:
		msg += "\n\nAnother Git process is using this repository. Wait for it to finish; remove the .lock file only if no Git process is running."
	case gitcli.ErrNoGit:
		msg = "Git was not found. Install Git or set its path in Settings."
	}
	if len(msg) > 2000 {
		msg = msg[:2000] + "…"
	}
	return msg
}

// startTask registers a task; the returned done function removes it.
func (w *Workspace) startTask(title string, write bool) (*Task, context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{ID: w.id(), Title: title, Started: w.Now(), cancel: cancel, Write: write}
	w.Tasks = append(w.Tasks, t)
	return t, ctx, func() {
		cancel()
		for i, x := range w.Tasks {
			if x == t {
				w.Tasks = append(w.Tasks[:i:i], w.Tasks[i+1:]...)
				break
			}
		}
	}
}

// CancelTask cancels a running task. Cancelling a write stops the process;
// it does not undo what Git already did, and the state is queried again.
func (w *Workspace) CancelTask(id uint64) {
	for _, t := range w.Tasks {
		if t.ID == id && t.cancel != nil {
			t.cancel()
		}
	}
}

// Busy reports a running write task.
func (w *Workspace) Busy() bool {
	for _, t := range w.Tasks {
		if t.Write {
			return true
		}
	}
	return false
}

// SetProgress updates a task's progress text from a worker goroutine.
func (w *Workspace) setProgress(t *Task, line string) {
	w.post(func() { t.Progress = line })
}
