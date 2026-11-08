package viffui

import (
	"os"

	"github.com/vivaui/viva/app"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// Patches carry changes between checkouts without touching the network. The
// change list writes what Git would diff; the Git tool window applies a saved
// patch to the working tree or to the index.

// patchTargets splits a selection into the paths one patch can hold: tracked
// files that are all staged or all unstaged. A conflict cannot be written to
// a patch and an untracked file has nothing to diff against; a selection that
// mixes both areas would need two patches, so it is not offered.
func patchTargets(sel []changeRow) (paths []string, staged, ok bool) {
	var stagedPaths, unstagedPaths []string
	seen := map[string]bool{}
	add := func(dst *[]string, p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		*dst = append(*dst, p)
	}
	for _, r := range sel {
		switch r.area {
		case domain.AreaConflicts, domain.AreaUntracked:
			continue
		case domain.AreaStaged:
			// A rename needs both names, or the patch copies the file instead.
			add(&stagedPaths, r.entry.OrigPath)
			add(&stagedPaths, r.entry.Path)
		default:
			add(&unstagedPaths, r.entry.Path)
		}
	}
	switch {
	case len(stagedPaths) > 0 && len(unstagedPaths) > 0:
		return nil, false, false
	case len(stagedPaths) > 0:
		return stagedPaths, true, true
	case len(unstagedPaths) > 0:
		return unstagedPaths, false, true
	}
	return nil, false, false
}

// savePatch asks where to write a patch of the selection and writes it.
func (w *Window) savePatch(paths []string, staged bool) {
	rs := w.repo()
	if rs == nil {
		return
	}
	name := rs.Repo.Name() + "-changes.patch"
	if staged {
		name = rs.Repo.Name() + "-staged.patch"
	}
	dest, err := app.SaveFile(app.SaveFileOptions{
		Title:            "Save Patch",
		SuggestedName:    name,
		DefaultExtension: "patch",
		Filters:          []app.FileFilter{{Name: w.t(i18n.PatchFilter), Pattern: "*.patch"}},
	})
	if err != nil || dest == "" {
		return
	}
	w.writePatch(dest, paths, staged)
}

// writePatch builds the patch in the background and saves it, then offers to
// show the file. It never writes a partial file: the bytes come from one Git
// run and appear only after it succeeded.
func (w *Window) writePatch(dest string, paths []string, staged bool) {
	w.ws.CreatePatch(paths, staged, func(patch []byte, err error) {
		if err != nil {
			w.ws.NotifyError(w.t(i18n.PatchCannotCreate), err)
			return
		}
		if len(patch) == 0 {
			w.ws.Notify(vapp.NoticeInfo, w.t(i18n.PatchNoneCreated), w.t(i18n.PatchNoneBody))
			return
		}
		if err := os.WriteFile(dest, patch, 0o644); err != nil {
			w.ws.Notify(vapp.NoticeError, w.t(i18n.PatchCannotWrite), err.Error())
			return
		}
		w.ws.Notify(vapp.NoticeSuccess, w.t(i18n.PatchCreated), dest,
			vapp.NoticeAction{Title: w.t(i18n.ShowInFileManager), Run: func() { revealPath(dest) }})
	})
}

// gitWindowMenu is the Git tool window's action menu.
func (w *Window) gitWindowMenu() []item {
	if w.repo() == nil || w.readOnly() {
		return nil
	}
	items := []item{
		{title: w.t(i18n.PatchApply), icon: icons.Patch, run: func() { w.choosePatch(false) }},
		{title: w.t(i18n.PatchApplyIndex), icon: icons.Patch, run: func() { w.choosePatch(true) }},
	}
	// The submodule actions share this one menu.
	items = append(append(items, sep()), w.gitMenu()...)
	if w.repo().Shallow {
		items = append(items, sep(), item{title: w.t(i18n.PatchFetchFull), icon: icons.Fetch, run: func() { w.fetchFullHistory() }})
	}
	return items
}

// choosePatch asks for a saved patch and applies it.
func (w *Window) choosePatch(cached bool) {
	if w.guardReadOnly() {
		return
	}
	paths, err := app.OpenFiles(app.OpenFileOptions{
		Title: "Apply Patch",
		Filters: []app.FileFilter{
			{Name: "Patch files", Pattern: "*.patch"},
			{Name: "Diff files", Pattern: "*.diff"},
		},
	})
	if err != nil || len(paths) == 0 {
		return
	}
	w.applyPatch(paths[0], cached)
}

// applyPatch hands the file to the workspace, which reports the outcome: a
// failure keeps Git's own message and changes nothing.
func (w *Window) applyPatch(path string, cached bool) {
	w.ws.ApplyPatchFile(path, cached, nil)
}
