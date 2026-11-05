package app

import (
	"context"
	"strings"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
	"viff/internal/i18n"
)

// LogPage is how many commits are read per page.
const LogPage = 400

// LogState is the loaded part of the commit log with its graph.
type LogState struct {
	Filter  LogFilter
	Commits []domain.Commit
	Rows    []domain.GraphRow
	// MaxWidth is the widest graph row so far, for the column width.
	MaxWidth int
	Done     bool
	Loading  bool
	Err      error
	// Gen increases with each reload so the UI can reset its selection.
	Gen uint64

	reader *gitcli.LogReader
	layout domain.GraphLayout
	seq    uint64
	loaded bool
	cancel context.CancelFunc
	index  map[string]int
}

// LogFilter is the log toolbar's filter.
type LogFilter struct {
	Text   string
	Branch string // "" = all
	Author string
	Since  time.Duration // 0 = any time
	Path   string
}

// Active reports a filter that narrows the log.
func (f LogFilter) Active() bool { return f != LogFilter{} }

// Loaded reports a log that was read at least once.
func (l *LogState) Loaded() bool { return l.loaded }

// IndexOf returns the row of a commit hash.
func (l *LogState) IndexOf(hash string) (int, bool) {
	i, ok := l.index[hash]
	return i, ok
}

func (l *LogState) close() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	if r := l.reader; r != nil {
		l.reader = nil
		go r.Close()
	}
}

// SetLogFilter changes the filter and reloads.
func (w *Workspace) SetLogFilter(f LogFilter) {
	if w.Repo == nil || w.Repo.Log.Filter == f {
		return
	}
	w.Repo.Log.Filter = f
	w.ReloadLog()
}

// ReloadLog restarts the log from the top.
func (w *Workspace) ReloadLog() {
	rs := w.Repo
	if rs == nil {
		return
	}
	l := &rs.Log
	l.close()
	l.seq++
	l.Gen++
	// The canceled load may still have been in flight; clear its flag so the
	// new reader starts. Its stale result is dropped by the seq check.
	l.Loading = false
	l.Commits, l.Rows, l.MaxWidth, l.Done, l.Err = nil, nil, 0, false, nil
	l.layout = domain.GraphLayout{}
	l.index = map[string]int{}
	l.loaded = true
	w.LoadMoreLog()
}

// LoadMoreLog reads the next page if the log is not complete.
func (w *Workspace) LoadMoreLog() {
	rs := w.Repo
	if rs == nil {
		return
	}
	l := &rs.Log
	if l.Loading || l.Done {
		return
	}
	l.Loading = true
	seq := l.seq
	reader := l.reader
	filter := l.Filter
	repo := rs.Repo
	firstPage := reader == nil
	var ctx context.Context
	if reader == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		l.cancel = cancel
	}
	go func() {
		var err error
		if reader == nil {
			gf := gitcli.LogFilter{Text: filter.Text, Author: filter.Author}
			if filter.Branch != "" {
				gf.Revisions = []string{filter.Branch}
			}
			if filter.Since > 0 {
				gf.Since = time.Now().Add(-filter.Since)
			}
			if filter.Path != "" {
				gf.Paths = []string{filter.Path}
			}
			reader, err = w.Git.OpenLog(ctx, repo, gf)
		}
		var commits []domain.Commit
		done := true
		if err == nil {
			commits, done, err = reader.Next(LogPage)
		}
		// A hash typed in the text filter matches the commit itself as well.
		var hashHit *domain.Commit
		if filter.Text != "" && len(filter.Text) >= 4 && isHex(filter.Text) && firstPage {
			if h, e := w.Git.ResolveRef(context.Background(), repo, filter.Text); e == nil && h != "" {
				if d, e := w.Git.CommitDetail(context.Background(), repo, h); e == nil {
					c := d.Commit
					hashHit = &c
				}
			}
		}
		w.post(func() {
			if w.Repo != rs || l.seq != seq {
				if reader != nil {
					go reader.Close()
				}
				return
			}
			l.reader = reader
			l.Loading = false
			if err != nil {
				l.Err = err
				l.Done = true
				return
			}
			if hashHit != nil {
				if _, dup := l.index[hashHit.Hash]; !dup {
					commits = append([]domain.Commit{*hashHit}, commits...)
				}
			}
			for _, c := range commits {
				if _, dup := l.index[c.Hash]; dup {
					continue
				}
				row := l.layout.Append(c)
				l.index[c.Hash] = len(l.Commits)
				l.Commits = append(l.Commits, c)
				l.Rows = append(l.Rows, row)
				if row.Width > l.MaxWidth {
					l.MaxWidth = row.Width
				}
			}
			l.Done = done
			if done && l.reader != nil {
				l.reader = nil
			}
		})
	}()
}

func isHex(s string) bool {
	for _, c := range strings.ToLower(s) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// SelectCommit loads the detail of a commit for the log's detail panel.
//
// The detail is published as soon as Git has it. Listing the branches that
// contain the commit walks the repository's history and takes seconds on a
// large repository, so it runs afterwards and fills DetailBranches when it
// finishes instead of holding the panel on its loading state.
func (w *Workspace) SelectCommit(hash string) {
	rs := w.Repo
	if rs == nil || rs.DetailHash == hash {
		return
	}
	rs.cancelDetail()
	rs.detailSeq++
	seq := rs.detailSeq
	rs.DetailHash = hash
	rs.DetailBranches = nil
	if hash == "" {
		rs.Detail = nil
		rs.DetailLoading = false
		return
	}
	rs.DetailLoading = true
	repo := rs.Repo
	// The same context covers both queries, so selecting another commit (or
	// closing the repository) stops the slow branch walk too.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	rs.detailCancel = cancel
	go func() {
		defer cancel()
		d, err := w.Git.CommitDetail(ctx, repo, hash)
		w.post(func() {
			if w.Repo != rs || rs.detailSeq != seq {
				return
			}
			rs.DetailLoading = false
			if err != nil {
				rs.Detail = nil
				w.NotifyError(w.tr(i18n.AppNoticeCannotLoadCommit), err)
				return
			}
			rs.Detail = &d
		})
		if err != nil {
			return
		}
		branches, _ := w.Git.ContainingBranches(ctx, repo, hash, 20)
		w.post(func() {
			if w.Repo != rs || rs.detailSeq != seq {
				return
			}
			rs.DetailBranches = branches
		})
	}()
}

func (rs *RepoState) cancelDetail() {
	if rs.detailCancel != nil {
		rs.detailCancel()
		rs.detailCancel = nil
	}
}
