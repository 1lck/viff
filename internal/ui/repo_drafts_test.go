package viffui

import (
	"path/filepath"
	"testing"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/storage"
)

func TestRepositorySwitchPreservesIndependentCommitDrafts(t *testing.T) {
	ws := vapp.NewWorkspace(&gitcli.Runner{}, storage.Load(filepath.Join(t.TempDir(), "settings.json")), nil)
	w := New(ws)
	t.Cleanup(func() { w.flushSettings(); ws.Close() })
	switchTo := func(root string, gen uint64) {
		ws.Repo = &vapp.RepoState{Repo: domain.Repo{Root: root}, Gen: gen}
		w.syncRepo()
	}
	switchTo("/repo-a", 1)
	setText(w.cm.msg, "A draft\n\nDetails")
	switchTo("/repo-b", 2)
	if got := w.cm.msg.String(); got != "" {
		t.Fatalf("new repository inherited draft: %q", got)
	}
	setText(w.cm.msg, "B draft")
	switchTo("/repo-a", 3)
	if got := w.cm.msg.String(); got != "A draft\n\nDetails" {
		t.Fatalf("lost A draft: %q", got)
	}
	// Amend's loaded HEAD message must not become a normal commit draft.
	w.cm.amend, w.cm.saved = true, "A draft\n\nDetails"
	setText(w.cm.msg, "Amended HEAD message")
	switchTo("/repo-b", 4)
	if got := w.cm.msg.String(); got != "B draft" {
		t.Fatalf("lost B draft: %q", got)
	}
	switchTo("/repo-a", 5)
	if got := w.cm.msg.String(); got != "A draft\n\nDetails" || w.cm.amend {
		t.Fatalf("amend leaked: %q, %v", got, w.cm.amend)
	}
	// Clearing after a successful commit must also clear the stored draft.
	setText(w.cm.msg, "")
	switchTo("/repo-b", 6)
	switchTo("/repo-a", 7)
	if got := w.cm.msg.String(); got != "" {
		t.Fatalf("committed draft resurrected: %q", got)
	}
}
