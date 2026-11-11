package viffui

import (
	"bytes"
	"testing"

	vapp "viff/internal/app"
	"viff/internal/domain"
	"viff/internal/gitcli"
)

func editableConflictView() *vapp.ConflictView {
	text := "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> other\n"
	lines, noEOL := domain.SplitLines(text)
	return &vapp.ConflictView{
		Conflict: domain.Conflict{Path: "conflict.txt", Kind: domain.ConflictBothModified},
		Current: vapp.Side{
			Content: gitcli.Content{Exists: true, Data: []byte(text), Size: int64(len(text))},
			Lines:   lines, NoEOL: noEOL, Text: true, LineEnding: "LF",
		},
		Markers: domain.ParseConflictMarkers(lines),
	}
}

// TestConflictEditableContent pins the read-only fallbacks at the data level:
// the same checks gate the editor before a tab can offer it.
func TestConflictEditableContent(t *testing.T) {
	if !conflictEditableContent(editableConflictView()) {
		t.Fatal("a plain text conflict with complete markers is not editable")
	}
	withBOM := editableConflictView()
	withBOM.Current.Content.Data = append([]byte{0xEF, 0xBB, 0xBF}, withBOM.Current.Content.Data...)
	if !conflictEditableContent(withBOM) {
		t.Fatal("a UTF-8 BOM makes a conflict uneditable")
	}

	cases := []struct {
		name   string
		mutate func(*vapp.ConflictView)
	}{
		{"nil", nil},
		{"binary", func(cv *vapp.ConflictView) { cv.Current.Content.Binary = true }},
		{"too large", func(cv *vapp.ConflictView) { cv.Current.Content.TooLarge = true }},
		{"submodule", func(cv *vapp.ConflictView) { cv.Current.Content.Submodule = true }},
		{"symlink", func(cv *vapp.ConflictView) { cv.Current.Content.Symlink = true }},
		{"missing", func(cv *vapp.ConflictView) { cv.Current.Content.Exists = false }},
		{"not text", func(cv *vapp.ConflictView) { cv.Current.Text = false }},
		{"non-textual kind", func(cv *vapp.ConflictView) { cv.Conflict.Kind = domain.ConflictDeletedByUs }},
		{"unreliable", func(cv *vapp.ConflictView) { cv.Markers.Reliable = false }},
		{"no blocks", func(cv *vapp.ConflictView) { cv.Markers.Blocks = nil }},
		{"invalid UTF-8", func(cv *vapp.ConflictView) { cv.Current.Content.Data = append(cv.Current.Content.Data, 0xff) }},
		{"over the edit limit", func(cv *vapp.ConflictView) {
			cv.Current.Content.Data = bytes.Repeat([]byte("x"), editContentLimit+1)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.mutate == nil {
				if conflictEditableContent(nil) {
					t.Fatal("nil view is editable")
				}
				return
			}
			cv := editableConflictView()
			c.mutate(cv)
			if conflictEditableContent(cv) {
				t.Fatalf("%s conflict is editable", c.name)
			}
		})
	}
}
