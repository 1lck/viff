package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"viff/internal/ai"
	"viff/internal/domain"
)

const (
	maxDiffChars  = 4000
	maxDiffFiles  = 8
	recentCommits = 5
)

// GenerateCommitMessage collects the Git context, asks the configured endpoint
// for a message and reports it on the workspace goroutine. previous carries the
// last message so a second run asks for a different wording.
func (w *Workspace) GenerateCommitMessage(ctx context.Context, cfg ai.Config, previous string, done func(string, error)) {
	go func() {
		user, err := w.commitContext(ctx)
		if err == nil && strings.TrimSpace(previous) != "" {
			user = "IMPORTANT: Generate a COMPLETELY DIFFERENT commit message from the previous one.\n" +
				"The previous message was: " + strconv.Quote(previous) + "\n" +
				"Use a different type, scope or description approach.\n\n" + user
		}
		var message string
		if err == nil {
			client := &ai.Client{}
			message, err = client.CommitMessage(ctx, cfg, user)
		}
		w.post(func() { done(message, err) })
	}()
}

// ListAIModels asks the configured endpoint for the model names it serves and
// reports them on the workspace goroutine.
func (w *Workspace) ListAIModels(ctx context.Context, cfg ai.Config, done func([]string, error)) {
	go func() {
		names, err := (&ai.Client{}).ListModels(ctx, cfg)
		w.post(func() { done(names, err) })
	}()
}

// commitContext renders the branch, the recent commits, the changed files and
// their diffs as one user message. Staged changes are preferred; the working
// tree is used when nothing is staged.
func (w *Workspace) commitContext(ctx context.Context) (string, error) {
	rs := w.Repo
	if rs == nil {
		return "", fmt.Errorf("no repository is open")
	}
	repo := rs.Repo
	staged, _, _, conflicts := rs.Status.Counts()
	if conflicts > 0 {
		return "", fmt.Errorf("resolve the conflicting files first")
	}
	useStaged := staged > 0

	var files []domain.StatusEntry
	for _, entry := range rs.Status.Entries {
		if entry.Ignored() {
			continue
		}
		if useStaged && !entry.Staged() {
			continue
		}
		if !useStaged && entry.Staged() {
			continue
		}
		if isLockFile(entry.Path) {
			continue
		}
		files = append(files, entry)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("there are no %s changes to describe", map[bool]string{true: "staged", false: "working-tree"}[useStaged])
	}

	branch := rs.Status.Branch.Branch
	if branch == "" {
		branch = "HEAD (detached)"
	}
	var b strings.Builder
	b.WriteString("Generate a commit message for the following changes:\n\n")
	fmt.Fprintf(&b, "Branch: %s\n", branch)
	b.WriteString("Recent commits:\n")
	recent := 0
	for _, commit := range rs.Log.Commits {
		if recent >= recentCommits {
			break
		}
		fmt.Fprintf(&b, "%s %s\n", domain.ShortHash(commit.Hash), commit.Subject)
		recent++
	}
	if recent == 0 {
		b.WriteString("(no commits yet)\n")
	}
	b.WriteString("Changed files:\n")
	for _, entry := range files {
		fmt.Fprintf(&b, "%s %s\n", statusText(entry, useStaged), entry.Path)
	}

	patch, err := w.Git.CreatePatch(ctx, repo, nil, useStaged)
	if err != nil {
		return "", err
	}
	b.WriteString("\nDiffs:\n")
	b.WriteString(trimmedDiff(string(patch)))
	return b.String(), nil
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
