package gitcli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"viff/internal/domain"
)

// This file builds a synthetic large repository with git fast-import and
// measures the Git operations behind the log, commit detail and diff views.
// The repository lives in a temporary directory and is never committed.
//
// Benchmarks run only with -bench, so a normal `go test` pays nothing:
//
//	go test -run '^$' -bench BenchmarkLargeRepo -benchmem ./internal/gitcli
//
// The default fixture is 20k commits and 3000 refs. Both can be raised for a
// one-off measurement of a real large repository:
//
//	VIFF_PERF_COMMITS=100000 VIFF_PERF_REFS=3000 \
//	  go test -run '^$' -bench 'BenchmarkLargeRepo/LogFirstPage' -benchtime=5x ./internal/gitcli
const (
	perfFirstPage = 400 // mirrors app.LogPage; gitcli cannot import app
	perfMergeEach = 100 // every Nth commit is a merge, so the graph has lanes
	perfFiles     = 8
)

func perfCount(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

type perfRepo struct {
	t       testing.TB
	dir     string
	r       *Runner
	repo    domain.Repo
	commits int // commits on main, merges included
	refs    int // local branches
}

// newPerfRepo builds the fixture with fast-import. The import stream is
// written through a pipe, so a 100k-commit repository never sits in memory
// as one buffer.
func newPerfRepo(tb testing.TB, commits, refs int) *perfRepo {
	tb.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		tb.Skip("git is not installed")
	}
	dir := tb.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	p := &perfRepo{t: tb, dir: dir, r: &Runner{Env: isolatedEnv(tb)}, commits: commits, refs: refs}
	p.git("init", "-q", "-b", "main")
	p.git("config", "user.name", "Viff Perf")
	p.git("config", "user.email", "perf@example.com")
	p.git("config", "commit.gpgsign", "false")
	p.importHistory()
	// A clone has packed refs; packing keeps the fixture close to one and
	// makes for-each-ref read packed-refs instead of thousands of files.
	p.git("pack-refs", "--all")
	repo, err := p.r.Discover(context.Background(), dir)
	if err != nil {
		tb.Fatalf("discover fixture: %v", err)
	}
	p.repo = repo
	return p
}

func (p *perfRepo) git(args ...string) string {
	p.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = p.dir
	cmd.Env = append(os.Environ(), p.r.Env...)
	cmd.Env = append(cmd.Env, "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		p.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (p *perfRepo) hash(rev string) string {
	p.t.Helper()
	return strings.TrimSpace(p.git("rev-parse", "--verify", rev))
}

func (p *perfRepo) importHistory() {
	p.t.Helper()
	cmd := exec.Command("git", "fast-import", "--done", "--quiet")
	cmd.Dir = p.dir
	cmd.Env = append(os.Environ(), p.r.Env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		p.t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		p.t.Fatalf("start fast-import: %v", err)
	}
	writeErr := make(chan error, 1)
	go func() {
		w := bufio.NewWriterSize(stdin, 1<<20)
		err := writeFastImport(w, p.commits, p.refs)
		if err == nil {
			err = w.Flush()
		}
		_ = stdin.Close()
		writeErr <- err
	}()
	err = cmd.Wait()
	if werr := <-writeErr; werr != nil {
		p.t.Fatalf("write fast-import stream: %v", werr)
	}
	if err != nil {
		p.t.Fatalf("fast-import: %v\n%s", err, stderr.String())
	}
}

// writeFastImport writes a main line of commits with periodic merges, then
// branches, tags and remote refs pointing into that history. Timestamps grow
// by a minute per commit so --date-order has a stable frontier.
func writeFastImport(w *bufio.Writer, commits, refs int) error {
	const (
		baseTime = 1_500_000_000
		perfSide = "refs/heads/perf-side"
	)
	authors := [][2]string{
		{"Ada Lovelace", "ada@example.com"},
		{"Grace Hopper", "grace@example.com"},
		{"Alan Turing", "alan@example.com"},
		{"Barbara Liskov", "barbara@example.com"},
		{"Edsger Dijkstra", "edsger@example.com"},
	}
	for i := 1; i <= commits; i++ {
		who := authors[i%len(authors)]
		when := baseTime + i*60
		prev := i - 1
		merge := i%perfMergeEach == 0
		ref := "refs/heads/main"
		msg := fmt.Sprintf("Commit %d: touch file %d", i, i%perfFiles)
		path := perfPath(i)
		content := fmt.Sprintf("value %d\n", i)
		if merge {
			// A side commit and a merge commit: both parents are ordinary
			// commits, so the graph gets real lanes and merge edges.
			side := commits + i/perfMergeEach
			fmt.Fprintf(w, "commit %s\nmark :%d\nauthor %s <%s> %d +0000\ncommitter %s <%s> %d +0000\ndata %d\n%s\n",
				perfSide, side, who[0], who[1], when+30, who[0], who[1], when+30,
				len(fmt.Sprintf("Side %d\n", i)), fmt.Sprintf("Side %d\n", i))
			fmt.Fprintf(w, "from :%d\n", prev)
			writeFile(w, perfSidePath(i), fmt.Sprintf("side %d\n", i))
			fmt.Fprintf(w, "commit %s\nmark :%d\nauthor %s <%s> %d +0000\ncommitter %s <%s> %d +0000\ndata %d\n%s\n",
				ref, i, who[0], who[1], when, who[0], who[1], when,
				len(msg+"\n"), msg+"\n")
			fmt.Fprintf(w, "from :%d\nmerge :%d\n", prev, side)
			writeFile(w, path, content)
			continue
		}
		if err := writeCommit(w, ref, i, prev, who, when, msg, path, content); err != nil {
			return err
		}
	}
	// Branch tips spread over the whole history so --branches has to walk
	// from thousands of starting points.
	for j := 1; j <= refs; j++ {
		if _, err := fmt.Fprintf(w, "reset refs/heads/perf-%04d\nfrom :%d\n", j, 1+(j*7919)%commits); err != nil {
			return err
		}
		if j%5 == 0 {
			if _, err := fmt.Fprintf(w, "reset refs/tags/perf-%04d\nfrom :%d\n", j, 1+(j*131)%commits); err != nil {
				return err
			}
		}
		if j%10 == 0 {
			if _, err := fmt.Fprintf(w, "reset refs/remotes/origin/perf-%04d\nfrom :%d\n", j, 1+(j*17)%commits); err != nil {
				return err
			}
		}
		if j%25 == 0 {
			mark := 1 + (j*131)%commits
			tagMsg := fmt.Sprintf("Tag %d\n", j)
			who := authors[j%len(authors)]
			if _, err := fmt.Fprintf(w, "tag perf-annotated-%04d\nfrom :%d\ntagger %s <%s> %d +0000\ndata %d\n%s\n",
				j, mark, who[0], who[1], baseTime+j*60, len(tagMsg), tagMsg); err != nil {
				return err
			}
		}
	}
	// Remove the helper ref the side commits were written to; they stay
	// reachable through the merge commits.
	if _, err := fmt.Fprintf(w, "reset %s\n", perfSide); err != nil {
		return err
	}
	_, err := fmt.Fprint(w, "done\n")
	return err
}

func writeCommit(w *bufio.Writer, ref string, mark, parent int, who [2]string, when int, msg, path, content string) error {
	parentLine := ""
	if parent > 0 {
		parentLine = fmt.Sprintf("from :%d\n", parent)
	}
	if _, err := fmt.Fprintf(w, "commit %s\nmark :%d\nauthor %s <%s> %d +0000\ncommitter %s <%s> %d +0000\ndata %d\n%s\n%s",
		ref, mark, who[0], who[1], when, who[0], who[1], when, len(msg+"\n"), msg+"\n", parentLine); err != nil {
		return err
	}
	writeFile(w, path, content)
	return nil
}

func writeFile(w *bufio.Writer, path, content string) {
	fmt.Fprintf(w, "M 100644 inline %s\ndata %d\n%s\n", path, len(content), content)
}

func perfPath(i int) string {
	return fmt.Sprintf("src/pkg%d/file%d.txt", i%3, i%perfFiles)
}

func perfSidePath(i int) string {
	return fmt.Sprintf("src/side%d/file%d.txt", i%3, i%perfFiles)
}

// BenchmarkLargeRepo measures the Git calls behind the log's first page,
// paging through the whole history, the refs query, commit detail, the
// containing-branch lookup, and reading one side of a diff.
func BenchmarkLargeRepo(b *testing.B) {
	if testing.Short() {
		b.Skip("large-repository benchmark skipped in -short mode")
	}
	repo := newPerfRepo(b, perfCount("VIFF_PERF_COMMITS", 20000), perfCount("VIFF_PERF_REFS", 3000))
	b.Logf("fixture: %d commits on main, %d branches, %d total refs",
		repo.commits, repo.refs, countRefs(b, repo))
	head := repo.hash("main")
	mid := repo.hash("main~" + strconv.Itoa(repo.commits/2))
	ctx := context.Background()

	b.Run("LogFirstPage", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			l, err := repo.r.OpenLog(ctx, repo.repo, LogFilter{})
			if err != nil {
				b.Fatal(err)
			}
			commits, _, err := l.Next(perfFirstPage)
			if err != nil {
				b.Fatal(err)
			}
			if len(commits) != perfFirstPage {
				b.Fatalf("first page = %d commits, want %d", len(commits), perfFirstPage)
			}
			l.Close()
		}
	})
	b.Run("LogAllPages", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			l, err := repo.r.OpenLog(ctx, repo.repo, LogFilter{})
			if err != nil {
				b.Fatal(err)
			}
			n := 0
			for {
				commits, done, err := l.Next(perfFirstPage)
				if err != nil {
					b.Fatal(err)
				}
				n += len(commits)
				if done {
					break
				}
			}
			l.Close()
			if n <= repo.commits {
				b.Fatalf("walked %d commits, want more than %d", n, repo.commits)
			}
		}
	})
	b.Run("Refs", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := repo.r.Refs(ctx, repo.repo); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("CommitDetail", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := repo.r.CommitDetail(ctx, repo.repo, mid); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ContainingBranches", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := repo.r.ContainingBranches(ctx, repo.repo, head, 20); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ReadFileAtCommit", func(b *testing.B) {
		b.ReportAllocs()
		rev := RevCommit(mid)
		path := perfPath(repo.commits / 2)
		for i := 0; i < b.N; i++ {
			if _, err := repo.r.Read(ctx, repo.repo, rev, path); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func countRefs(tb testing.TB, p *perfRepo) int {
	tb.Helper()
	refs, err := p.r.Refs(context.Background(), p.repo)
	if err != nil {
		tb.Fatal(err)
	}
	return len(refs)
}

// TestSyntheticPerfFixture keeps the generator honest: the benchmark numbers
// only mean something if the fixture has the promised shape.
func TestSyntheticPerfFixture(t *testing.T) {
	p := newPerfRepo(t, 500, 20)
	// 500 commits on the main line plus one side commit per merge (every
	// 100th commit), all reachable from main.
	if got := strings.TrimSpace(p.git("rev-list", "--count", "main")); got != "505" {
		t.Fatalf("main reaches %s commits, want 505", got)
	}
	if got := strings.TrimSpace(p.git("rev-list", "--count", "--all")); got != "505" {
		t.Fatalf("all refs reach %s commits, want 505", got)
	}
	refs, err := p.r.Refs(context.Background(), p.repo)
	if err != nil {
		t.Fatal(err)
	}
	// main plus 20 branches, 4 tags, 2 remote branches, no annotated tags.
	if len(refs) != 27 {
		t.Fatalf("refs = %d, want 27: %+v", len(refs), refs)
	}
	kinds := map[domain.RefKind]int{}
	for _, r := range refs {
		kinds[r.Kind]++
	}
	if kinds[domain.RefLocal] != 21 || kinds[domain.RefTag] != 4 || kinds[domain.RefRemote] != 2 {
		t.Fatalf("ref kinds = %v", kinds)
	}
}
