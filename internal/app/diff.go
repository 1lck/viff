package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// DiffSource names what a diff compares.
type DiffSource struct {
	Path    string
	OldPath string // the path on the old side when renamed
	// Area is set for working-tree diffs.
	Area domain.Area
	// Working is true for a working-tree diff (staged, unstaged, untracked or
	// conflict); otherwise Commit and Parent name the two revisions.
	Working bool
	Commit  string
	Parent  string // "" for a root commit
	Kind    domain.ChangeKind
}

// Key identifies a source for caching and tab identity.
func (s DiffSource) Key() string {
	if s.Working {
		return fmt.Sprintf("w%d:%s", s.Area, s.Path)
	}
	return "c" + s.Commit + ":" + s.Parent + ":" + s.Path
}

// Side is one text of a diff with its derived lines.
type Side struct {
	Title   string
	Content gitcli.Content
	Lines   []string
	NoEOL   bool
	Text    bool // decoded as text (not binary, not too large)
	// LineEnding is "LF" or "CRLF".
	LineEnding string
}

// DiffResult is a loaded, computed diff.
type DiffResult struct {
	Source   DiffSource
	Old, New Side
	Blocks   []domain.Block
	// Inline holds word ranges for each modified block, indexed like Blocks.
	InlineOld [][][]domain.Range
	InlineNew [][][]domain.Range
	Mode      domain.WhitespaceMode
	// Notice explains a diff that cannot be shown as text.
	Notice string
	// ModeChange is "100644 → 100755" when only the mode changed.
	ModeChange string
	Loaded     time.Time
	// Conflict is set for an unmerged path.
	Conflict *ConflictView
}

// Identical reports a diff without changed blocks.
func (d *DiffResult) Identical() bool { return len(d.Blocks) == 0 && d.Notice == "" }

// ConflictView is the read-only three-way view of an unmerged path.
type ConflictView struct {
	Conflict    domain.Conflict
	Ours        Side
	Base        Side
	Theirs      Side
	Current     Side
	OursLabel   string
	TheirsLabel string
	Markers     domain.MarkerParse
	// Resolved is true when the working file holds no markers any more but
	// Git still lists the path as unmerged.
	Resolved bool
}

// LoadDiff computes a diff in the background and calls done on the UI thread.
// A result that arrives after gen changed is not delivered.
func (w *Workspace) LoadDiff(src DiffSource, mode domain.WhitespaceMode, done func(*DiffResult, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	op := rs.Operation
	branch := rs.CurrentBranch()
	conflict, isConflict := rs.Conflict(src.Path)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		var res *DiffResult
		var err error
		if src.Working && src.Area == domain.AreaConflicts && isConflict {
			res, err = loadConflict(ctx, w.Git, repo, conflict, op, branch)
		} else {
			res, err = loadDiff(ctx, w.Git, repo, src, mode)
		}
		w.post(func() {
			if w.Repo != rs {
				return
			}
			if res != nil {
				res.Loaded = w.Now()
			}
			done(res, err)
		})
	}()
}

func loadDiff(ctx context.Context, git Git, repo domain.Repo, src DiffSource, mode domain.WhitespaceMode) (*DiffResult, error) {
	oldRev, newRev, oldTitle, newTitle := revisions(src)
	oldPath := src.Path
	if src.OldPath != "" {
		oldPath = src.OldPath
	}
	res := &DiffResult{Source: src, Mode: mode}
	var err error
	if oldRev.Kind != "" {
		res.Old.Content, err = git.Read(ctx, repo, oldRev, oldPath)
		if err != nil {
			return nil, err
		}
	}
	res.New.Content, err = git.Read(ctx, repo, newRev, src.Path)
	if err != nil {
		return nil, err
	}
	res.Old.Title, res.New.Title = oldTitle, newTitle
	finishDiff(res)
	return res, nil
}

// revisions maps a source to the two revisions compared.
func revisions(src DiffSource) (oldRev, newRev gitcli.Revision, oldTitle, newTitle string) {
	if src.Working {
		switch src.Area {
		case domain.AreaStaged:
			return gitcli.RevHead, gitcli.RevIndex, "HEAD", "Staged"
		case domain.AreaUntracked:
			return gitcli.Revision{}, gitcli.RevWorktree, "", "Your version"
		}
		return gitcli.RevIndex, gitcli.RevWorktree, "Staged", "Your version"
	}
	if src.Commit == "" {
		// A revision compared with the working tree.
		newRev, newTitle = gitcli.RevWorktree, "Your version"
	} else {
		newRev = gitcli.RevCommit(src.Commit)
		newTitle = domain.ShortHash(src.Commit)
	}
	if src.Parent != "" {
		oldRev = gitcli.RevCommit(src.Parent)
		oldTitle = domain.ShortHash(src.Parent)
	}
	return
}

// SideFrom decodes content into lines.
func SideFrom(title string, c gitcli.Content) Side {
	s := Side{Title: title, Content: c}
	if c.Exists && !c.Binary && !c.TooLarge && !c.Submodule {
		data := c.Data
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		text := string(data)
		if !utf8.ValidString(text) {
			text = latin1(data)
		}
		s.Lines, s.NoEOL = domain.SplitLines(text)
		s.LineEnding = domain.DetectLineEnding(text)
		s.Text = true
	}
	if !c.Exists {
		s.Text = true // an absent side is an empty text
	}
	return s
}

// latin1 decodes bytes that are not UTF-8 so they can still be shown.
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func finishDiff(res *DiffResult) {
	res.Old = SideFrom(res.Old.Title, res.Old.Content)
	res.New = SideFrom(res.New.Title, res.New.Content)
	o, n := res.Old.Content, res.New.Content
	switch {
	case o.Binary || n.Binary:
		res.Notice = binaryNotice(o, n)
		return
	case o.TooLarge || n.TooLarge:
		res.Notice = fmt.Sprintf("File is too large to compare (%s). Open it in an external tool.", sizeText(max(o.Size, n.Size)))
		return
	case o.Submodule || n.Submodule:
		res.Notice = "Submodule change. Open the submodule to see its history."
		return
	}
	res.Blocks = domain.DiffSides(domain.PatchSide{Lines: res.Old.Lines, NoFinalNewline: res.Old.NoEOL},
		domain.PatchSide{Lines: res.New.Lines, NoFinalNewline: res.New.NoEOL}, res.Mode)
	res.InlineOld = make([][][]domain.Range, len(res.Blocks))
	res.InlineNew = make([][][]domain.Range, len(res.Blocks))
	for i, b := range res.Blocks {
		if b.Kind() != domain.ChangeModified || b.OldEnd-b.OldStart > 400 || b.NewEnd-b.NewStart > 400 {
			continue
		}
		res.InlineOld[i], res.InlineNew[i] = domain.InlineDiff(res.Old.Lines[b.OldStart:b.OldEnd], res.New.Lines[b.NewStart:b.NewEnd], res.Mode)
	}
	if len(res.Blocks) == 0 && o.Exists && n.Exists && bytes.Equal(o.Data, n.Data) && res.Old.Text {
		res.Notice = ""
	}
}

func binaryNotice(o, n gitcli.Content) string {
	switch {
	case !o.Exists:
		return fmt.Sprintf("Binary file added (%s)", sizeText(n.Size))
	case !n.Exists:
		return fmt.Sprintf("Binary file deleted (%s)", sizeText(o.Size))
	case bytes.Equal(o.Data, n.Data) && o.Data != nil:
		return "Binary files are identical"
	}
	return fmt.Sprintf("Binary files differ (%s → %s)", sizeText(o.Size), sizeText(n.Size))
}

func sizeText(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func loadConflict(ctx context.Context, git Git, repo domain.Repo, c domain.Conflict, op domain.Operation, branch string) (*DiffResult, error) {
	cv := &ConflictView{Conflict: c}
	cv.OursLabel, cv.TheirsLabel = op.SideLabels(branch)
	read := func(n int, title string) (Side, error) {
		if _, ok := c.Stage(n); !ok {
			return Side{Title: title, Text: true}, nil
		}
		content, err := git.Read(ctx, repo, gitcli.RevStage(n), c.Path)
		if err != nil {
			return Side{}, err
		}
		return SideFrom(title, content), nil
	}
	var err error
	if cv.Ours, err = read(2, cv.OursLabel); err != nil {
		return nil, err
	}
	if cv.Base, err = read(1, "Base"); err != nil {
		return nil, err
	}
	if cv.Theirs, err = read(3, cv.TheirsLabel); err != nil {
		return nil, err
	}
	cur, err := git.Read(ctx, repo, gitcli.RevWorktree, c.Path)
	if err != nil {
		return nil, err
	}
	cv.Current = SideFrom("Current file (read-only)", cur)
	if cv.Current.Text && cur.Exists {
		cv.Markers = domain.ParseConflictMarkers(cv.Current.Lines)
		cv.Resolved = cv.Markers.Reliable && len(cv.Markers.Blocks) == 0
	}
	res := &DiffResult{Source: DiffSource{Path: c.Path, Working: true, Area: domain.AreaConflicts}, Conflict: cv}
	// The two-way part of the view compares ours with theirs.
	res.Old, res.New = cv.Ours, cv.Theirs
	if cv.Ours.Text && cv.Theirs.Text && !cv.Ours.Content.Binary && !cv.Theirs.Content.Binary {
		res.Blocks = domain.DiffSides(domain.PatchSide{Lines: cv.Ours.Lines, NoFinalNewline: cv.Ours.NoEOL},
			domain.PatchSide{Lines: cv.Theirs.Lines, NoFinalNewline: cv.Theirs.NoEOL}, domain.WhitespaceExact)
	} else {
		res.Notice = "Binary conflict: choose a version with your external tool, then stage the file."
	}
	return res, nil
}

// ErrStale reports that a file changed after its diff was shown; the action
// was not applied and the diff is reloaded.
var ErrStale = errors.New("the file changed after the diff was shown; review the new diff and try again")
