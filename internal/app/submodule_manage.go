package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
)

// submodule_manage.go holds the write use cases behind the Submodules dialog:
// adding a submodule, deinitializing its working tree, and removing it. Each
// one runs the real Git command, reports Git's error instead of a success, and
// lets the normal refresh read the state back from Git.

// AddSubmodule registers url at path (derived from the URL when empty) and
// checks it out. Git writes .gitmodules and stages the submodule entry; the
// user commits it. done runs on the UI thread with Git's error, nil only when
// Git actually added the submodule.
func (w *Workspace) AddSubmodule(url, path string, done func(error)) {
	if w.Repo == nil {
		if done != nil {
			done(errors.New(w.tr(i18n.AppErrorNoRepositoryOpen)))
		}
		return
	}
	url = strings.TrimSpace(url)
	path = strings.TrimSpace(path)
	if path == "" {
		path = SubmodulePathFromURL(url)
	}
	var runErr error
	w.Run("Add submodule", func(ctx context.Context, repo domain.Repo, progress func(string)) error {
		sm, serr := w.submodules()
		if serr != nil {
			return serr
		}
		return sm.AddSubmodule(ctx, repo, url, path, progress)
	}, RunOptions{
		Success:     w.tr(i18n.AppNoticeSubmoduleAdded),
		SuccessBody: path,
		// The dialog shows Git's message itself, next to the fields the user
		// can correct, so the default balloon would only repeat it.
		OnError: func(err error) bool { runErr = err; return true },
		Always: func() {
			if done != nil {
				done(runErr)
			}
		},
	})
}

// RemoveSubmodule deinitializes the submodule and removes its gitlink and
// .gitmodules entry, staging the change. With force false Git's refusal to
// discard local changes is passed on instead of being overridden; the caller
// must confirm that first and call again with force.
func (w *Workspace) RemoveSubmodule(path string, force bool, done func(error)) {
	w.submoduleWrite("Remove submodule", w.tr(i18n.AppNoticeSubmoduleRemoved), path, func(ctx context.Context, repo domain.Repo, progress func(string)) error {
		sm, serr := w.submodules()
		if serr != nil {
			return serr
		}
		return sm.RemoveSubmodule(ctx, repo, path, force)
	}, done)
}

// DeinitSubmodule empties the submodule's working tree, keeping the gitlink
// and .gitmodules entry so Update can check it out again. Like removal, local
// changes are refused unless the caller confirmed force.
func (w *Workspace) DeinitSubmodule(path string, force bool, done func(error)) {
	w.submoduleWrite("Deinit submodule", w.tr(i18n.AppNoticeSubmoduleDeinitialized), path, func(ctx context.Context, repo domain.Repo, progress func(string)) error {
		sm, serr := w.submodules()
		if serr != nil {
			return serr
		}
		return sm.DeinitSubmodule(ctx, repo, path, force)
	}, done)
}

// submoduleWrite runs one submodule write with the generic task bookkeeping:
// write lock, progress, refresh from Git afterwards, and done on the UI thread
// with the real result.
func (w *Workspace) submoduleWrite(title, success, path string, op Op, done func(error)) {
	if w.Repo == nil {
		if done != nil {
			done(errors.New(w.tr(i18n.AppErrorNoRepositoryOpen)))
		}
		return
	}
	var runErr error
	w.Run(title, op, RunOptions{
		Success:     success,
		SuccessBody: path,
		OnError:     func(err error) bool { runErr = err; return true },
		Always: func() {
			if done != nil {
				done(runErr)
			}
		},
	})
}

// DirectSubmodules reports the submodule paths the open repository records in
// its own .gitmodules. The Submodules dialog lists nested ones too, but only
// a repository can deinit or remove the submodules it records.
func (w *Workspace) DirectSubmodules(done func(map[string]bool, error)) {
	rs := w.Repo
	if rs == nil {
		if done != nil {
			done(nil, errors.New(w.tr(i18n.AppErrorNoRepositoryOpen)))
		}
		return
	}
	repo := rs.Repo
	go func() {
		sm, serr := w.submodules()
		if serr != nil {
			w.post(func() {
				if w.Repo == rs {
					done(nil, serr)
				}
			})
			return
		}
		paths, err := sm.DirectSubmodulePaths(context.Background(), repo)
		w.post(func() {
			if w.Repo != rs {
				return
			}
			done(paths, err)
		})
	}()
}

// SubmoduleDirty reports on the UI thread whether deinit or removal would
// discard local work in the submodule. It reads Git, so the confirmation the
// dialog shows reflects the repository now, not the list it loaded earlier.
func (w *Workspace) SubmoduleDirty(path string, done func(bool, error)) {
	rs := w.Repo
	if rs == nil {
		if done != nil {
			done(false, errors.New(w.tr(i18n.AppErrorNoRepositoryOpen)))
		}
		return
	}
	repo := rs.Repo
	go func() {
		sm, serr := w.submodules()
		if serr != nil {
			w.post(func() {
				if w.Repo == rs {
					done(false, serr)
				}
			})
			return
		}
		dirty, err := sm.SubmoduleDirty(context.Background(), repo, path)
		w.post(func() {
			if w.Repo != rs {
				return
			}
			done(dirty, err)
		})
	}()
}

// SubmodulePathFromURL is the path Git would choose for a URL: the last
// component without a trailing .git. It is empty when no usable path follows,
// for example for a URL that is only a host. The result always passes
// SubmodulePathProblem, so it can be offered as the field's suggestion.
func SubmodulePathFromURL(url string) string {
	u := filepath.ToSlash(strings.TrimSpace(url))
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	if u == "" || u == "." || u == ".." {
		return ""
	}
	if _, err := gitcli.CleanSubmodulePath(u); err != nil {
		return ""
	}
	return u
}

// SubmoduleURLProblem explains why url cannot be added; "" when it looks
// usable. The final word stays with Git, which reports clone failures.
func SubmoduleURLProblem(url string) string {
	u := strings.TrimSpace(url)
	switch {
	case u == "":
		return "Enter the URL or local path of the repository to add."
	case strings.HasPrefix(u, "-"):
		return "A repository URL cannot start with '-'."
	}
	return ""
}

// SubmodulePathProblem explains why path cannot be used for a submodule inside
// the open repository; "" when it can. The same check runs in the Git adapter
// before the command starts, so a hand-built call cannot act outside the
// checkout either.
func SubmodulePathProblem(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return "Enter a path for the submodule, for example deps/lib."
	}
	if _, err := gitcli.CleanSubmodulePath(p); err != nil {
		return err.Error()
	}
	return ""
}
