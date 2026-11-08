package viffui

import (
	"path/filepath"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/sshpath"
)

// readOnly reports that the active repository is opened over SSH and accepts
// no writes in this version. The same flag hides write actions and labels the
// window, so a control cannot be visible while the application layer would
// refuse it.
func (w *Window) readOnly() bool { return w.ws.ReadOnly() }

// repoPath is the copy-friendly absolute location of a repository-relative
// path: a local path or user@host:/path for an SSH repository.
func (w *Window) repoPath(rel string) string {
	rs := w.repo()
	if rs == nil {
		return rel
	}
	if rs.Repo.IsSSH() {
		return sshpath.DisplayPath(rs.Repo.Root, rel)
	}
	return filepath.Join(rs.Repo.Root, filepath.FromSlash(rel))
}

// repoRootLabel is the secondary line for a repository: the home-shortened
// path locally, user@host:path for a remote one.
func repoRootLabel(root string) string {
	if sshpath.Is(root) {
		return sshpath.Display(root)
	}
	return shortenHome(root)
}

// repoRootName is the short name shown in lists and badges. filepath.Base also
// works for a URL, but Name keeps the employee path segment for a server root.
func repoRootName(root string) string {
	if sshpath.Is(root) {
		return sshpath.Name(root)
	}
	return filepath.Base(root)
}

// repoRootHint is the full identity shown in a tooltip.
func repoRootHint(root string) string {
	if sshpath.Is(root) {
		return sshpath.Display(root) + "\n" + root
	}
	return root
}

// guardReadOnly is the common refusal for a write dialog that a shortcut or a
// menu could still reach. Returning true means the caller must stop; the
// notification says where the change belongs.
func (w *Window) guardReadOnly() bool {
	if !w.readOnly() {
		return false
	}
	w.ws.Notify(vapp.NoticeInfo, w.t(i18n.ChromeStatusReadOnly), w.t(i18n.WorkspaceTipReadOnly))
	return true
}
