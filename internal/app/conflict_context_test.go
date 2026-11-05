package app

import (
	"strconv"
	"strings"
	"testing"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

const (
	testOursLabel   = "main (HEAD)"
	testTheirsLabel = "feature/search (abc1234) Add search"
)

// conflictViewFromText builds the view the conflict tab would show for a
// working-tree file: the marker parse comes from the file itself, so the test
// exercises the real block ranges instead of hand-written indexes.
func conflictViewFromText(t *testing.T, path, text string) *ConflictView {
	t.Helper()
	lines, noEOL := domain.SplitLines(text)
	cv := &ConflictView{
		Conflict: domain.Conflict{Path: path, Kind: domain.ConflictBothModified},
		Current: Side{
			Content:    gitcli.Content{Exists: true},
			Lines:      lines,
			Text:       true,
			LineEnding: domain.DetectLineEnding(text),
			NoEOL:      noEOL,
		},
		OursLabel:   testOursLabel,
		TheirsLabel: testTheirsLabel,
		Markers:     domain.ParseConflictMarkers(lines),
	}
	if !cv.Markers.Reliable {
		t.Fatalf("test fixture markers are unreliable for:\n%s", text)
	}
	return cv
}

func mergeOperation() domain.Operation {
	return domain.Operation{
		Kind:            domain.OpMerge,
		Incoming:        "abc1234def",
		IncomingName:    "feature/search",
		IncomingSubject: "Add search",
	}
}

const diff3File = `package store

<<<<<<< HEAD
func Put() { s.m[k] = v }
||||||| base
func Put() { s.m[k] = v; s.n++ }
=======
func Put(k string, v int) { s.m[k] = v }
>>>>>>> feature/search
`

const mergeFile = `line before
<<<<<<< HEAD
ours one
ours two
=======
theirs one
>>>>>>> feature/search
line after
`

func TestConflictContextText(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		labels   [2]string
		noLabels bool
		op       domain.Operation
		block    int
		want     []string
		notWant  []string
	}{
		{
			name:  "diff3 block carries base and source labels",
			file:  diff3File,
			op:    mergeOperation(),
			block: ConflictContextAll,
			want: []string{
				"- Repository: /tmp/repo\n",
				"- File: internal/store/store.go\n",
				"- Operation: merge\n",
				"- Ours (stage 2): main (HEAD)\n",
				"- Theirs (stage 3): feature/search (abc1234) Add search\n",
				"- Conflict type: Both modified\n",
				"- Conflict style: diff3 or zdiff3 (base sections included)\n",
				"- Line endings: LF, final newline: yes\n",
				"- Conflict blocks: 1\n",
				"## Instructions",
				"Remove every conflict marker",
				"## Conflict block 1 of 1 (working-tree lines 3-9)",
				"`<<<<<<<` ours, `|||||||` base, `=======`, `>>>>>>>` theirs",
				"### ours (stage 2, line 4) - main (HEAD)",
				"func Put() { s.m[k] = v }",
				"### base (stage 1, line 6)",
				"func Put() { s.m[k] = v; s.n++ }",
				"### theirs (stage 3, line 8) - feature/search (abc1234) Add search",
				"func Put(k string, v int) { s.m[k] = v }",
			},
			notWant: []string{"(no content on this side)", "Viff truncated"},
		},
		{
			name:  "merge style has no base section",
			file:  mergeFile,
			op:    mergeOperation(),
			block: ConflictContextAll,
			want: []string{
				"- Conflict style: merge (no base sections)\n",
				"`<<<<<<<` ours, `=======`, `>>>>>>>` theirs",
				"## Conflict block 1 of 1 (working-tree lines 2-7)",
				"### ours (stage 2, lines 3-4) - main (HEAD)",
				"ours one\nours two\n",
				"### theirs (stage 3, line 6) - feature/search (abc1234) Add search",
				"theirs one",
			},
			notWant: []string{"### base"},
		},
		{
			name:  "every block is numbered",
			file:  "a\n<<<<<<< HEAD\none\n=======\ntwo\n>>>>>>> other\nmid\n<<<<<<< HEAD\nthree\n=======\nfour\n>>>>>>> other\nz\n",
			op:    mergeOperation(),
			block: ConflictContextAll,
			want: []string{
				"- Conflict blocks: 2\n",
				"## Conflict block 1 of 2",
				"## Conflict block 2 of 2",
				"\none\n",
				"\nthree\n",
				"\ntwo\n",
				"\nfour\n",
			},
		},
		{
			name:  "one block only when asked",
			file:  "a\n<<<<<<< HEAD\none\n=======\ntwo\n>>>>>>> other\nmid\n<<<<<<< HEAD\nthree\n=======\nfour\n>>>>>>> other\nz\n",
			op:    mergeOperation(),
			block: 1,
			want: []string{
				"- Conflict blocks: 2; this text includes block 2 only\n",
				"- Only the block below is included in this text; the other blocks of the file are not shown.",
				"## Conflict block 2 of 2",
				"three",
				"four",
			},
			notWant: []string{"## Conflict block 1 of 2", "### ours (stage 2, line 3)", "### theirs (stage 3, line 5)"},
		},
		{
			name:     "missing sources are not invented",
			file:     mergeFile,
			noLabels: true,
			block:    ConflictContextAll,
			op:       domain.Operation{},
			want: []string{
				"- Operation: none reported by Git (unmerged entries only)\n",
				"- Ours (stage 2): unknown (Git did not report a branch or commit)\n",
				"- Theirs (stage 3): unknown (Git did not report a branch or commit)\n",
				"### ours (stage 2, lines 3-4)\n",
			},
			notWant: []string{"main (HEAD)", "feature/search"},
		},
		{
			name:  "empty side is named instead of faked",
			file:  "before\n<<<<<<< HEAD\n=======\ntheirs line\n>>>>>>> other\n",
			op:    mergeOperation(),
			block: ConflictContextAll,
			want: []string{
				"### ours (stage 2, no lines) - main (HEAD)",
				"(no content on this side)",
				"theirs line",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cv := conflictViewFromText(t, "internal/store/store.go", tc.file)
			if tc.noLabels {
				cv.OursLabel, cv.TheirsLabel = "", ""
			}
			text := ConflictContextText(cv, ConflictContextOptions{RepoRoot: "/tmp/repo", Operation: tc.op, Block: tc.block})
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Fatalf("context is missing %q:\n%s", want, text)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(text, notWant) {
					t.Fatalf("context unexpectedly contains %q:\n%s", notWant, text)
				}
			}
		})
	}
}

// TestConflictContextLineLimitTruncatesNamedly checks that an oversized block
// is cut at the limit and says how much was left out, per section.
func TestConflictContextLineLimitTruncatesNamedly(t *testing.T) {
	var file strings.Builder
	file.WriteString("<<<<<<< HEAD\n")
	for i := 0; i < 260; i++ {
		file.WriteString("ours line ")
		file.WriteString(strconv.Itoa(i))
		file.WriteByte('\n')
	}
	file.WriteString("=======\n")
	file.WriteString("theirs line\n")
	file.WriteString(">>>>>>> other\n")
	cv := conflictViewFromText(t, "big.txt", file.String())

	text := ConflictContextText(cv, ConflictContextOptions{RepoRoot: "/tmp/repo", Operation: mergeOperation(), Block: ConflictContextAll})
	if !strings.Contains(text, "Viff truncated this section at the 200-line / 20 KB block limit; 60 lines not shown") {
		t.Fatalf("ours truncation note missing:\n%s", tail(text))
	}
	// The block budget is spent by ours, so theirs is named as left out too
	// rather than vanishing without a word.
	if !strings.Contains(text, "Viff truncated this section at the 200-line / 20 KB block limit; 1 line not shown") {
		t.Fatalf("theirs truncation note missing:\n%s", tail(text))
	}
	if strings.Contains(text, "ours line 259") {
		t.Fatal("text claims to contain a line past the limit")
	}
}

// TestConflictContextSizeLimitTruncatesNamedly checks the 20 KB per-block cap
// with lines too wide for the line cap to be what cuts them.
func TestConflictContextSizeLimitTruncatesNamedly(t *testing.T) {
	wide := strings.Repeat("x", 400)
	var file strings.Builder
	file.WriteString("<<<<<<< HEAD\n")
	for i := 0; i < 80; i++ {
		file.WriteString(wide)
		file.WriteByte('\n')
	}
	file.WriteString("=======\ntheirs\n>>>>>>> other\n")
	cv := conflictViewFromText(t, "wide.txt", file.String())

	text := ConflictContextText(cv, ConflictContextOptions{RepoRoot: "/tmp/repo", Operation: mergeOperation(), Block: ConflictContextAll})
	if !strings.Contains(text, "Viff truncated this section at the 200-line / 20 KB block limit; 29 lines not shown") {
		t.Fatalf("size truncation note missing:\n%s", tail(text))
	}
}

// TestConflictContextWholeTextLimit checks that many blocks stop at the
// 200 KB whole-text cap with a note naming what was left out.
func TestConflictContextWholeTextLimit(t *testing.T) {
	wide := strings.Repeat("y", 200)
	var file strings.Builder
	for i := 0; i < 12; i++ {
		file.WriteString("<<<<<<< HEAD\n")
		for j := 0; j < 120; j++ {
			file.WriteString(wide)
			file.WriteByte('\n')
		}
		file.WriteString("=======\ntheirs\n>>>>>>> other\n")
	}
	cv := conflictViewFromText(t, "many.txt", file.String())
	if got := len(cv.Markers.Blocks); got != 12 {
		t.Fatalf("fixture blocks = %d, want 12", got)
	}

	text, truncated := conflictContext(cv, ConflictContextOptions{RepoRoot: "/tmp/repo", Operation: mergeOperation(), Block: ConflictContextAll})
	if !truncated {
		t.Fatal("conflictContext did not report the whole-text truncation")
	}
	if !strings.Contains(text, "## Context truncated") {
		t.Fatalf("whole-text truncation note missing:\n%s", tail(text))
	}
	if len(text) > maxContextBytes {
		t.Fatalf("context = %d bytes, over the %d byte cap", len(text), maxContextBytes)
	}
	if n := strings.Count(text, "## Conflict block "); n >= 12 {
		t.Fatalf("kept %d of 12 blocks despite the cap", n)
	}
}

// TestConflictContextFenceSurvivesBackticks checks that file text cannot end
// the fence and turn the rest of the context into prose.
func TestConflictContextFenceSurvivesBackticks(t *testing.T) {
	cv := conflictViewFromText(t, "doc.md", "```\n<<<<<<< HEAD\na ``` fence\n=======\n````\n>>>>>>> other\n")
	text := ConflictContextText(cv, ConflictContextOptions{RepoRoot: "/tmp/repo", Operation: mergeOperation(), Block: ConflictContextAll})
	if !strings.Contains(text, "`````text") {
		t.Fatalf("fence does not outgrow the backticks in the content:\n%s", text)
	}
}

func TestConflictContextEdgeCases(t *testing.T) {
	if got := ConflictContextText(nil, ConflictContextOptions{}); got != "" {
		t.Fatalf("nil view = %q, want empty", got)
	}

	// A file with no markers still says so instead of pretending to have a
	// block; the viewer disables the action in this state.
	cv := conflictViewFromText(t, "plain.txt", "no markers here\n")
	text := ConflictContextText(cv, ConflictContextOptions{RepoRoot: "", Operation: domain.Operation{}})
	for _, want := range []string{
		"- Repository: (unknown)\n",
		"- Conflict blocks: none in the working-tree file\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("context is missing %q:\n%s", want, text)
		}
	}

	// A stale index copies every block rather than an empty text, and the
	// scope line says what was copied.
	cv = conflictViewFromText(t, "two.txt", "a\n<<<<<<< HEAD\none\n=======\ntwo\n>>>>>>> other\nb\n<<<<<<< HEAD\nthree\n=======\nfour\n>>>>>>> other\n")
	text = ConflictContextText(cv, ConflictContextOptions{RepoRoot: "/tmp/repo", Block: 7})
	if !strings.Contains(text, "## Conflict block 1 of 2") || !strings.Contains(text, "## Conflict block 2 of 2") {
		t.Fatalf("stale block index did not copy every block:\n%s", text)
	}
}

// tail keeps a failure readable when the assertion is about a large text.
func tail(s string) string {
	const n = 1200
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
