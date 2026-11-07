package app

import (
	"context"
	"sync"
	"testing"

	"viff/internal/domain"
	"viff/internal/vcs"
)

// logReaderRecorder wraps the fixture's backend to see whether the log closes
// the reader it opened. It embeds the interface, so every other call still runs
// the real backend.
type logReaderRecorder struct {
	vcs.Backend
	mu     sync.Mutex
	opened int
	closed int
}

func (r *logReaderRecorder) OpenLog(ctx context.Context, repo domain.Repo, f vcs.LogFilter) (vcs.LogReader, error) {
	reader, err := r.Backend.OpenLog(ctx, repo, f)
	if err != nil {
		return reader, err
	}
	r.mu.Lock()
	r.opened++
	r.mu.Unlock()
	return &recordingReader{LogReader: reader, rec: r}, nil
}

func (r *logReaderRecorder) counts() (opened, closed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.opened, r.closed
}

type recordingReader struct {
	vcs.LogReader
	rec *logReaderRecorder
}

func (r *recordingReader) Close() {
	r.LogReader.Close()
	r.rec.mu.Lock()
	r.rec.closed++
	r.rec.mu.Unlock()
}

// TestLogStopsItsReaderWhenTheLastPageArrives pins the log's reader lifetime.
// The reader owns a running `git log`, and the old code dropped it once the last
// page arrived without closing it: the child stayed alive holding the
// repository's pack files open. On Windows that made TempDir cleanup fail with
// "Access is denied" long after the log was done, and in the application it left
// a git process running for nothing.
func TestLogStopsItsReaderWhenTheLastPageArrives(t *testing.T) {
	f := newFixture(t)
	rec := &logReaderRecorder{Backend: f.w.Git}
	f.w.Git = rec
	f.fastImport(fastImportStream(6, 0, 0, 0))
	f.open()
	f.settle(func() bool { return f.idle() && f.w.Repo.Log.Done })

	opened, closed := rec.counts()
	if opened == 0 {
		t.Fatal("the log never opened a reader")
	}
	if closed != opened {
		t.Fatalf("log kept %d of %d readers open: the git child process still runs and holds the repository", opened-closed, opened)
	}

	// A reload after the last page opens a fresh reader, which must be closed in
	// turn: paging twice is what a user does when switching filters.
	f.w.SetLogFilter(LogFilter{Text: "commit"})
	f.settle(func() bool { return f.idle() && f.w.Repo.Log.Done })
	opened, closed = rec.counts()
	if opened < 2 {
		t.Fatalf("the reload reused a closed reader (%d opened)", opened)
	}
	if closed != opened {
		t.Fatalf("after a reload %d of %d readers are still open", opened-closed, opened)
	}
}
