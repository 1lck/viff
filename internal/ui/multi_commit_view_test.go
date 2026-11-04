package viffui

import (
	"testing"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// TestMultiCommitViewDropsStaleResult exercises the selection guard around the
// multi-commit query without a window: a result for another key or another
// request sequence must never replace the current view.
func TestMultiCommitViewDropsStaleResult(t *testing.T) {
	var v multiCommitView
	seq := v.begin("a|b|#1")
	if v.key != "a|b|#1" || !v.loading {
		t.Fatalf("begin state = %+v", v)
	}
	if v.apply("a|b|#1", seq+1, vapp.MultiCommitSummary{}) {
		t.Fatal("result from another request sequence was accepted")
	}
	if v.apply("a|c|#1", seq, vapp.MultiCommitSummary{}) {
		t.Fatal("result for another selection was accepted")
	}
	sum := vapp.MultiCommitSummary{Commits: []domain.Commit{{Hash: "h"}}}
	if !v.apply("a|b|#1", seq, sum) {
		t.Fatal("current result was refused")
	}
	if v.loading || v.summary == nil || v.summary.Commits[0].Hash != "h" {
		t.Fatalf("applied state = %+v", v)
	}

	// A newer selection supersedes a request that is still in flight.
	old := v.seq
	v.begin("x|y|#1")
	if v.apply("a|b|#1", old, vapp.MultiCommitSummary{}) {
		t.Fatal("superseded result was accepted")
	}
	if v.summary != nil || !v.loading {
		t.Fatalf("new selection state = %+v", v)
	}

	// Returning to one commit clears the aggregate.
	v.stop()
	if v.key != "" || v.summary != nil || v.loading {
		t.Fatalf("stopped state = %+v", v)
	}
}

// TestMultiCommitSelectionKey checks that the key follows the selection order
// and the repository generation, so a reopened repository or a different
// selection cannot reuse an old aggregate.
func TestMultiCommitSelectionKey(t *testing.T) {
	rs := &vapp.RepoState{Gen: 1}
	a := domain.Commit{Hash: "aaaa"}
	b := domain.Commit{Hash: "bbbb"}
	ab := multiCommitSelectionKey(rs, []domain.Commit{a, b})
	ba := multiCommitSelectionKey(rs, []domain.Commit{b, a})
	if ab == ba {
		t.Fatal("selection order must change the key")
	}
	rs.Gen = 2
	if again := multiCommitSelectionKey(rs, []domain.Commit{a, b}); again == ab {
		t.Fatal("repository generation must change the key")
	}
}
