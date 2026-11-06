package app

import (
	"viff/internal/domain"
	"viff/internal/vcs"
)

// backends holds one adapter per supported version control system, in the
// order they are probed. It is written once by the composition root (or by a
// test) and read afterwards, so it needs no lock: only the field assignment
// before the workspace runs is a write.
//
// Order matters when a directory is ambiguous. A Git working tree never
// contains a .svn directory and vice versa, so in practice exactly one probe
// succeeds; the list order only decides the impossible case.
type backends []probeTarget

type probeTarget struct {
	kind domain.Kind
	// prober answers "does this directory belong to me" without starting a
	// process.
	prober vcs.Prober
	// backend is the adapter that serves repositories of this kind.
	backend Git
}

// SetBackends registers the adapters the workspace may use. Passing the Git
// adapter alone keeps the previous single-backend behaviour, which is what
// every existing test relies on.
func (w *Workspace) SetBackends(list ...Git) {
	w.backends = w.backends[:0]
	for _, b := range list {
		if b == nil {
			continue
		}
		prober, ok := b.(vcs.Prober)
		if !ok {
			continue
		}
		w.backends = append(w.backends, probeTarget{kind: kindOfProber(prober), prober: prober, backend: b})
	}
}

// kindOfProber maps a prober's name onto the repository kind it claims.
func kindOfProber(p vcs.Prober) domain.Kind {
	if p.Name() == "Subversion" {
		return domain.KindSVN
	}
	return domain.KindGit
}

// pickBackend chooses the adapter for dir by a purely local probe. It never
// starts a process and never fails: a directory belonging to no known system
// falls back to the first registered backend, whose Discover then reports the
// real error to the user.
func (w *Workspace) pickBackend(dir string) Git {
	for _, t := range w.backends {
		if t.prober.Detect(dir) {
			return t.backend
		}
	}
	if w.Git != nil {
		return w.Git
	}
	if len(w.backends) > 0 {
		return w.backends[0].backend
	}
	return nil
}

// backendForRoot picks the adapter for a repository that is not active, used
// when refreshing the rows of the repository list.
func (w *Workspace) backendForRoot(root string) Git { return w.pickBackend(root) }

// gitBackend is the adapter that serves Git repositories, whichever repository
// is open. Creating a repository and pointing Viff at a git executable are
// about Git, not about the backend of the working copy in front of the user.
func (w *Workspace) gitBackend() Git {
	for _, t := range w.backends {
		if t.kind == domain.KindGit {
			return t.backend
		}
	}
	// A workspace that registered no backends keeps the single-adapter setup,
	// where w.Git is the git runner.
	if len(w.backends) == 0 {
		return w.Git
	}
	return nil
}

// SetGitPath pushes the configured git executable to the Git adapter.
//
// It exists for the same reason SetSVNPath does: w.Git is whichever adapter
// serves the open repository, so saving settings while a Subversion working
// copy is open would set the Git path on the Subversion runner. stat accepts
// any existing file, so the check would pass and every svn command would then
// run `git --non-interactive info --xml`.
func (w *Workspace) SetGitPath(path string) {
	if b := w.gitBackend(); b != nil {
		b.SetPath(path)
	}
}

// SetSVNPath pushes the configured svn executable to the Subversion adapter.
//
// It exists because the registered backends are what the settings page can
// reach: w.Git is whichever adapter serves the open repository, so writing
// through it would set the Git path on a Subversion runner.
func (w *Workspace) SetSVNPath(path string) {
	for _, t := range w.backends {
		if t.kind != domain.KindSVN {
			continue
		}
		t.backend.SetPath(path)
	}
}

// cliNamer is implemented by an adapter that can name its own executable, so
// the console and the event log never print one tool's name over the other's
// commands.
type cliNamer interface {
	CLIName() string
}

// toolName is the executable name to print for b, defaulting to git.
func toolName(b Git) string {
	if n, ok := b.(cliNamer); ok {
		if name := n.CLIName(); name != "" {
			return name
		}
	}
	return "git"
}

// useBackend makes b the active adapter. It runs on the UI thread, before any
// command for the newly opened repository starts, so no goroutine observes the
// field changing.
func (w *Workspace) useBackend(b Git) {
	if b == nil || b == w.Git {
		return
	}
	b.SetCommandLog(w.logCommand)
	w.Git = b
	w.ConsoleTool = toolName(b)
	w.Caps = CapabilitiesOf(b)
	if w.Settings != nil {
		w.ApplyGitSettings()
	}
}

// BackendName reports which version control system serves the open repository,
// for the status bar and the diagnostics summary. It is "" when no repository
// is open.
func (w *Workspace) BackendName() string {
	if w.Repo == nil {
		return ""
	}
	if p, ok := w.Git.(vcs.Prober); ok {
		return p.Name()
	}
	return ""
}
