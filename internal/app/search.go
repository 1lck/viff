package app

import (
	"context"
	"strings"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// SearchHit and SearchResult are the content-search answer. They are aliases
// so the dialog reads them from this package instead of importing the Git
// adapter directly.
type (
	SearchHit    = gitcli.SearchHit
	SearchResult = gitcli.SearchResult
)

// ErrRipgrepMissing reports that ripgrep was chosen but is not installed. The
// dialog turns it into a settings hint instead of an error.
var ErrRipgrepMissing = gitcli.ErrRipgrepMissing

// RipgrepPath resolves the ripgrep executable a search would run, so the
// Settings dialog reports that one instead of resolving a path of its own.
// An empty configured path means "rg" from PATH; "" means it is missing.
func RipgrepPath(configured string) string {
	bin, err := gitcli.FindRipgrep(configured)
	if err != nil {
		return ""
	}
	return bin
}

// contentSearcher is the optional backend capability behind content search.
// It is not part of vcs.Backend because Subversion has no equivalent of
// "grep the working copy" that Viff can run: the entry is hidden instead of
// the adapter stubbing a method it cannot implement (ADR 0025).
type contentSearcher interface {
	SearchContent(ctx context.Context, repo domain.Repo, query string, opts gitcli.SearchOptions) (gitcli.SearchResult, error)
}

// SupportsContentSearch reports whether the active backend can search file
// contents.
func (w *Workspace) SupportsContentSearch() bool {
	_, ok := w.Git.(contentSearcher)
	return ok
}

// SearchContentOptions selects the backend for one query.
type SearchContentOptions struct {
	Tool        string
	RipgrepPath string
}

// SearchContent searches the working tree for query. It is a read query: it
// takes no write lock and does not trigger a refresh, and it is cancelled by
// the next query, a repository change, or Close.
//
// done runs on the UI thread and only for the newest query. A result that
// arrives after another query started, or after the repository changed, is
// dropped instead of painting the wrong answer.
func (w *Workspace) SearchContent(query string, opts SearchContentOptions, done func(gitcli.SearchResult, error)) {
	rs := w.Repo
	query = strings.TrimSpace(query)
	if rs == nil || query == "" {
		return
	}
	searcher, ok := w.Git.(contentSearcher)
	if !ok {
		done(SearchResult{}, ErrUnsupported)
		return
	}
	w.searchSeq++
	seq := w.searchSeq
	w.cancelSearch()
	ctx, cancel := context.WithCancel(context.Background())
	w.searchCancel = cancel
	go func() {
		defer cancel()
		res, err := searcher.SearchContent(ctx, rs.Repo, query, gitcli.SearchOptions{
			Tool:             opts.Tool,
			RipgrepPath:      opts.RipgrepPath,
			IncludeUntracked: true,
		})
		w.post(func() {
			if seq != w.searchSeq || w.Repo != rs {
				return
			}
			w.searchCancel = nil
			done(res, err)
		})
	}()
}

// cancelSearch stops the query in flight, if any.
func (w *Workspace) cancelSearch() {
	if w.searchCancel != nil {
		w.searchCancel()
		w.searchCancel = nil
	}
}
