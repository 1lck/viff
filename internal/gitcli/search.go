package gitcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"viff/internal/domain"
	"viff/internal/process"
)

// Content search lives here because it is the same job as the rest of this
// package: build an argument array, run an external tool, and parse its
// machine format. It is deliberately not a Git feature: the working tree is
// read on every query and no index is kept, so the results follow Git's
// tracked files and .gitignore but never claim to be a second source of truth.

// Search backends accepted by SearchOptions.Tool.
const (
	SearchAuto    = "auto"
	SearchGit     = "git"
	SearchRipgrep = "rg"
)

const (
	// DefaultSearchMax bounds one query, so a broad pattern in a monorepo
	// cannot fill memory or the dialog.
	DefaultSearchMax = 300
	// searchTextMax is the number of runes kept from one matching line. The
	// dialog paints a clipped row anyway; this only bounds what is retained.
	searchTextMax = 300
	// searchFileMax skips generated and vendored blobs for ripgrep. Git grep
	// gets the same effect from the binary check (-I) and the index.
	searchFileMax = "2M"
)

// ErrRipgrepMissing reports that ripgrep was chosen explicitly but is not
// installed. Auto falls back to Git grep instead of returning this.
var ErrRipgrepMissing = errors.New("ripgrep not found")

// SearchOptions selects the backend and its bounds.
type SearchOptions struct {
	// Tool is SearchAuto, SearchGit or SearchRipgrep. Empty means auto.
	Tool string
	// RipgrepPath is the ripgrep executable; empty looks it up on PATH.
	RipgrepPath string
	// IncludeUntracked searches files Git does not track yet, still honoring
	// .gitignore. ripgrep always includes them, so this keeps both backends
	// returning the same set.
	IncludeUntracked bool
	// Max is the greatest number of hits returned; zero means the default.
	Max int
}

// SearchHit is one matching line.
type SearchHit struct {
	Path string
	Line int
	Text string
}

// SearchResult is one query's answer.
type SearchResult struct {
	Hits      []SearchHit
	Truncated bool
	// Tool is the backend that actually ran: SearchGit or SearchRipgrep.
	Tool string
}

// SearchContent finds query in the working tree of repo. The query is a
// literal string, not a regular expression. The call is cancellable and keeps
// no state: the next query, a cancelled context, or a changed repository
// simply starts over.
func (r *Runner) SearchContent(ctx context.Context, repo domain.Repo, query string, opts SearchOptions) (SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return SearchResult{Tool: SearchGit}, nil
	}
	if opts.Max <= 0 {
		opts.Max = DefaultSearchMax
	}
	tool := strings.TrimSpace(opts.Tool)
	if tool == "" || tool == SearchAuto {
		tool = SearchGit
		if _, err := FindRipgrep(opts.RipgrepPath); err == nil {
			tool = SearchRipgrep
		}
	}
	switch tool {
	case SearchRipgrep:
		bin, err := FindRipgrep(opts.RipgrepPath)
		if err != nil {
			return SearchResult{}, fmt.Errorf("search contents: %w", ErrRipgrepMissing)
		}
		return r.searchRipgrep(ctx, repo.Root, bin, q, opts)
	default:
		return r.searchGit(ctx, repo.Root, q, opts)
	}
}

// FindRipgrep resolves the executable. The fallback directories mirror git():
// a GUI launched from Finder starts without the user's shell PATH.
//
// It is exported so the Settings dialog can report the executable a search
// would actually run instead of resolving one of its own.
func FindRipgrep(path string) (string, error) {
	if p := strings.TrimSpace(path); p != "" {
		return exec.LookPath(p)
	}
	if p, err := exec.LookPath("rg"); err == nil {
		return p, nil
	}
	for _, c := range []string{"/opt/homebrew/bin/rg", "/usr/local/bin/rg", "/usr/bin/rg"} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", exec.ErrNotFound
}

// searchGit runs `git grep`. The -z layout is path\0line\0text\n per match, so
// paths keep spaces, colons and Unicode without a quoting format to parse.
// Git exits 1 when nothing matched; that is an empty answer, not a failure.
func (r *Runner) searchGit(ctx context.Context, root, query string, opts SearchOptions) (SearchResult, error) {
	args := []string{"grep", "-n", "-z", "-I", "-F", "--no-color"}
	if opts.IncludeUntracked {
		args = append(args, "--untracked")
	}
	args = append(args, "-e", query, "--")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd, err := r.command(ctx, root, args)
	if err != nil {
		return SearchResult{}, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return SearchResult{}, fmt.Errorf("git grep: %w", err)
	}
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return SearchResult{}, fmt.Errorf("git grep: %w", err)
	}
	// The query is user text; it is never written to the event log, which is
	// why this does not go through Runner.Log.
	res := SearchResult{Tool: SearchGit}
	res.Hits, res.Truncated = readGitGrep(bufio.NewReaderSize(out, 64<<10), opts.Max)
	if res.Truncated {
		cancel() // the rest cannot be shown; stop reading the repository
	}
	waitErr := cmd.Wait()
	if waitErr != nil && !res.Truncated && ctx.Err() == nil {
		if code, ok := exitStatus(waitErr); ok && code == 1 {
			return res, nil
		}
		return SearchResult{}, r.wrap(ctx, args, waitErr, stderr.buf.String(), "")
	}
	return res, nil
}

// readGitGrep parses until EOF, the hit limit, or the first malformed entry.
func readGitGrep(r *bufio.Reader, max int) ([]SearchHit, bool) {
	var hits []SearchHit
	for {
		path, err := readSearchField(r, 0)
		if path == "" || err != nil {
			return hits, false
		}
		lineText, err := readSearchField(r, 0)
		if err != nil {
			return hits, false
		}
		text, err := readSearchField(r, '\n')
		line, convErr := strconv.Atoi(lineText)
		if convErr != nil || line <= 0 {
			if err != nil {
				return hits, false
			}
			continue
		}
		hits = append(hits, SearchHit{Path: path, Line: line, Text: clipSearchText(text)})
		if len(hits) >= max {
			return hits, true
		}
		if err != nil {
			return hits, false
		}
	}
}

// readSearchField reads one delimited field and drops the delimiter.
func readSearchField(r *bufio.Reader, delim byte) (string, error) {
	s, err := r.ReadString(delim)
	if len(s) > 0 && s[len(s)-1] == delim {
		s = s[:len(s)-1]
	}
	return s, err
}

// searchRipgrep runs rg with its JSON stream, which keeps paths and columns
// unambiguous even when a file name contains a newline. --hidden includes
// tracked dot files; -g !.git keeps the repository database out.
func (r *Runner) searchRipgrep(ctx context.Context, root, bin, query string, opts SearchOptions) (SearchResult, error) {
	args := []string{
		"--json", "-F", "--hidden",
		"-g", "!.git", "-g", "!.git/**",
		"--max-filesize", searchFileMax,
		"-e", query, ".",
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	process.HideConsoleWindow(cmd)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), r.Env...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		return SearchResult{}, fmt.Errorf("ripgrep: %w", err)
	}
	stderr := &limitedBuffer{limit: stderrLimit}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return SearchResult{}, fmt.Errorf("ripgrep: %w", err)
	}
	res := SearchResult{Tool: SearchRipgrep}
	res.Hits, res.Truncated = readRipgrep(out, opts.Max)
	if res.Truncated {
		cancel()
	}
	waitErr := cmd.Wait()
	if waitErr != nil && !res.Truncated && ctx.Err() == nil {
		if code, ok := exitStatus(waitErr); ok && code == 1 {
			return res, nil
		}
		return SearchResult{}, fmt.Errorf("ripgrep: %w: %s", waitErr, strings.TrimSpace(stderr.buf.String()))
	}
	return res, nil
}

// rgEvent is the part of ripgrep's JSON protocol this feature reads.
type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text  string `json:"text"`
			Bytes string `json:"bytes"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func readRipgrep(r io.Reader, max int) ([]SearchHit, bool) {
	var hits []SearchHit
	dec := json.NewDecoder(r)
	for {
		var ev rgEvent
		if err := dec.Decode(&ev); err != nil {
			return hits, false
		}
		if ev.Type != "match" || ev.Data.Path.Text == "" || ev.Data.LineNumber <= 0 {
			continue
		}
		if ev.Data.Lines.Text == "" && ev.Data.Lines.Bytes != "" {
			continue // a binary match has no text to show
		}
		text := ev.Data.Lines.Text
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[:i]
		}
		hits = append(hits, SearchHit{
			Path: strings.TrimPrefix(filepath.ToSlash(ev.Data.Path.Text), "./"),
			Line: ev.Data.LineNumber,
			Text: clipSearchText(text),
		})
		if len(hits) >= max {
			return hits, true
		}
	}
}

// clipSearchText trims the line ending and bounds one row. Tabs stay: the
// dialog expands them the way the diff view does.
func clipSearchText(s string) string {
	s = strings.TrimRight(s, "\r\n")
	if utf8.RuneCountInString(s) <= searchTextMax {
		return s
	}
	return string([]rune(s)[:searchTextMax])
}

// exitStatus reports a process exit code, if err is one.
func exitStatus(err error) (int, bool) {
	var e *exec.ExitError
	if errors.As(err, &e) {
		return e.ExitCode(), true
	}
	return 0, false
}
