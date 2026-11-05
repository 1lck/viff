package app

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func benchCommitCount() int {
	if v := os.Getenv("VIFF_PERF_COMMITS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 20000
}

func benchRefCount() int {
	if v := os.Getenv("VIFF_PERF_REFS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3000
}

// BenchmarkLargeRepo measures the two user-visible waits of the log on a
// synthetic large repository: opening the first page, and seeing a commit's
// detail after selecting it.
//
// "DetailAndBranches" is the wait the panel used to have (the detail plus the
// containing-branch walk). "DetailOnly" stops when the detail is visible; the
// gap between the two is the time the split moved off the critical path.
//
//	VIFF_PERF_COMMITS=100000 VIFF_PERF_REFS=3000 go test -run '^$' \
//	  -bench 'BenchmarkLargeRepo' -benchtime=3x ./internal/app
func BenchmarkLargeRepo(b *testing.B) {
	if testing.Short() {
		b.Skip("large-repository benchmark skipped in -short mode")
	}
	f := newFixture(b)
	f.fastImport(fastImportStream(benchCommitCount(), benchRefCount(), 0, 0))

	b.Run("LogFirstScreen", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var gen uint64
			if rs := f.w.Repo; rs != nil {
				gen = rs.Gen
			}
			f.w.Open(f.dir, func(error) {})
			waitFor(b, f.w, "log first page", func() bool {
				rs := f.w.Repo
				return rs != nil && rs.Gen > gen && rs.Log.Loaded() && !rs.Log.Loading && len(rs.Log.Commits) > 0 && !f.w.Pending()
			})
		}
	})

	// The detail benchmarks need the repository open and its first page read.
	var gen uint64
	if rs := f.w.Repo; rs != nil {
		gen = rs.Gen
	}
	f.w.Open(f.dir, func(error) {})
	waitFor(b, f.w, "repository open", func() bool {
		rs := f.w.Repo
		return rs != nil && rs.Gen > gen && rs.StatusLoaded && rs.Log.Loaded() && !rs.Log.Loading && !f.w.Pending()
	})
	rs := f.w.Repo
	if len(rs.Log.Commits) < 2 {
		b.Fatalf("fixture log has %d commits", len(rs.Log.Commits))
	}
	var hashes []string
	for i, c := range rs.Log.Commits {
		if i >= 100 {
			break
		}
		hashes = append(hashes, c.Hash)
	}

	b.Run("DetailOnly", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			hash := hashes[i%len(hashes)]
			f.w.SelectCommit(hash)
			waitFor(b, f.w, "commit detail", func() bool {
				rs := f.w.Repo
				return rs.DetailHash == hash && !rs.DetailLoading && rs.Detail != nil
			})
			// Stop the branch walk so the next iteration is not measured
			// against a busy machine.
			f.w.SelectCommit("")
			f.w.Drain()
		}
	})

	b.Run("DetailAndBranches", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			hash := hashes[i%len(hashes)]
			f.w.SelectCommit(hash)
			waitFor(b, f.w, "commit detail and branches", func() bool {
				rs := f.w.Repo
				return rs.DetailHash == hash && !rs.DetailLoading && rs.Detail != nil && len(rs.DetailBranches) > 0
			})
			f.w.SelectCommit("")
			f.w.Drain()
		}
	})
}

// waitFor drains the workspace's background results until cond holds.
func waitFor(b *testing.B, w *Workspace, what string, cond func() bool) {
	b.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		w.Drain()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			b.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}
