package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"viff/internal/ai"
	"viff/internal/domain"
)

const (
	maxDiffChars  = 4000
	maxDiffFiles  = 8
	recentCommits = 5
	// aiTimeout bounds one model request together with the Git call that
	// assembles its prompt, so a stalled endpoint cannot keep the task alive
	// forever.
	aiTimeout = time.Minute
)

// commitSnapshot is the Git context of one prompt, copied on the UI thread.
// The request runs in the background, where reading the live RepoState would
// race a refresh that replaces it.
type commitSnapshot struct {
	repo   domain.Repo
	staged bool
	branch string
	files  []domain.StatusEntry
	paths  []string
	recent []string
}

// GenerateCommitMessage collects the Git context, asks the configured endpoint
// for a message and reports it on the workspace goroutine. previous carries the
// last message so a second run asks for a different wording.
func (w *Workspace) GenerateCommitMessage(ctx context.Context, cfg ai.Config, previous string, done func(string, error)) {
	snap, rs, err := w.snapshotCommit()
	if err != nil {
		w.post(func() { done("", err) })
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(ctx, aiTimeout)
		defer cancel()
		var message string
		user, err := w.commitPrompt(ctx, snap, previous)
		if err == nil {
			message, err = (&ai.Client{}).CommitMessage(ctx, cfg, user)
		}
		w.post(func() {
			// A result for a repository that closed or changed in the meantime
			// would fill the wrong commit box.
			if w.Repo != rs {
				return
			}
			done(message, err)
		})
	}()
}

// ListAIModels asks the configured endpoint for the model names it serves and
// reports them on the workspace goroutine.
func (w *Workspace) ListAIModels(ctx context.Context, cfg ai.Config, done func([]string, error)) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, aiTimeout)
		defer cancel()
		names, err := (&ai.Client{}).ListModels(ctx, cfg)
		w.post(func() { done(names, err) })
	}()
}

// snapshotCommit copies the branch, the recent commits and the changed files
// from the workspace. Staged changes are preferred; the working tree is used
// when nothing is staged. It returns the RepoState it read so the caller can
// drop a result for a repository that is no longer open.
func (w *Workspace) snapshotCommit() (*commitSnapshot, *RepoState, error) {
	rs := w.Repo
	if rs == nil {
		return nil, nil, fmt.Errorf("no repository is open")
	}
	staged, _, _, conflicts := rs.Status.Counts()
	if conflicts > 0 {
		return nil, rs, fmt.Errorf("resolve the conflicting files first")
	}
	useStaged := staged > 0

	snap := &commitSnapshot{repo: rs.Repo, staged: useStaged}
	for _, entry := range rs.Status.Entries {
		if entry.Ignored() || isLockFile(entry.Path) {
			continue
		}
		if entry.Staged() != useStaged {
			continue
		}
		snap.files = append(snap.files, entry)
		snap.paths = append(snap.paths, entry.Path)
	}
	if len(snap.files) == 0 {
		return nil, rs, fmt.Errorf("there are no %s changes to describe", map[bool]string{true: "staged", false: "working-tree"}[useStaged])
	}

	snap.branch = rs.Status.Branch.Branch
	if snap.branch == "" {
		snap.branch = "HEAD (detached)"
	}
	for _, commit := range rs.Log.Commits {
		if len(snap.recent) >= recentCommits {
			break
		}
		snap.recent = append(snap.recent, domain.ShortHash(commit.Hash)+" "+commit.Subject)
	}
	return snap, rs, nil
}

// commitPrompt renders the snapshot and its patch as one user message. Only the
// files in the snapshot are diffed, so a lock file kept out of the list cannot
// slip into the request through the patch.
func (w *Workspace) commitPrompt(ctx context.Context, snap *commitSnapshot, previous string) (string, error) {
	patch, err := w.Git.CreatePatch(ctx, snap.repo, snap.paths, snap.staged)
	if err != nil {
		return "", err
	}
	return buildCommitPrompt(snap, string(patch), previous), nil
}

// buildCommitPrompt is the pure half of commitPrompt, so the wording can be
// tested without a repository or a network.
func buildCommitPrompt(snap *commitSnapshot, patch, previous string) string {
	var b strings.Builder
	b.WriteString("Generate a commit message for the following changes:\n\n")
	fmt.Fprintf(&b, "Branch: %s\n", snap.branch)
	b.WriteString("Recent commits:\n")
	if len(snap.recent) == 0 {
		b.WriteString("(no commits yet)\n")
	}
	for _, line := range snap.recent {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("Changed files:\n")
	for _, entry := range snap.files {
		fmt.Fprintf(&b, "%s %s\n", statusText(entry, snap.staged), entry.Path)
	}
	b.WriteString("\nDiffs:\n")
	b.WriteString(trimmedDiff(patch))
	user := b.String()
	if strings.TrimSpace(previous) != "" {
		user = "IMPORTANT: Generate a COMPLETELY DIFFERENT commit message from the previous one.\n" +
			"The previous message was: " + strconv.Quote(previous) + "\n" +
			"Use a different type, scope or description approach.\n\n" + user
	}
	return user
}

// statusText names a status entry the way a person reads it.
func statusText(entry domain.StatusEntry, staged bool) string {
	code := entry.Worktree
	if staged {
		code = entry.Index
	}
	switch code {
	case 'A':
		return "added"
	case 'M':
		return "modified"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "type changed"
	case '?':
		return "new untracked file"
	}
	if entry.Untracked() {
		return "new untracked file"
	}
	return "modified"
}

// isLockFile keeps machine-generated files out of the context: their diffs say
// nothing about what the commit does.
func isLockFile(path string) bool {
	switch filepath.Base(path) {
	case "go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"Cargo.lock", "Pipfile.lock", "poetry.lock", "composer.lock", "Gemfile.lock":
		return true
	}
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".lock") || strings.HasSuffix(lower, ".min.js") || strings.HasSuffix(lower, ".min.css")
}

// trimmedDiff keeps the first files of a patch and truncates each one, so a
// large change cannot flood the request.
func trimmedDiff(patch string) string {
	sections := strings.Split(patch, "diff --git ")
	var b strings.Builder
	used := 0
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if used >= maxDiffFiles {
			b.WriteString("... [more files omitted]\n")
			break
		}
		if strings.Contains(section, "GIT binary patch") {
			continue
		}
		b.WriteString("--- ")
		if len(section) > maxDiffChars {
			b.WriteString(section[:maxDiffChars])
			b.WriteString("\n... [truncated]\n")
		} else {
			b.WriteString(section)
			b.WriteByte('\n')
		}
		used++
	}
	if used == 0 {
		return "(no textual diff available)\n"
	}
	return b.String()
}
