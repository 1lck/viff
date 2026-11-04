package gitcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"viff/internal/domain"
)

// Content is one side of a diff: the bytes of a file at some revision, or a
// description of why the bytes are not shown.
type Content struct {
	Exists    bool
	Data      []byte
	Size      int64
	Binary    bool
	TooLarge  bool
	Mode      string
	Symlink   bool
	Submodule bool
}

// ContentLimit is the largest file loaded for a text diff. Bigger files get a
// restricted preview that never reads them whole.
const ContentLimit = 8 << 20

// Revision selects which version of a file to read.
type Revision struct {
	// Kind: "worktree", "index", "stage" (with Stage), or "commit" (with Rev).
	Kind  string
	Rev   string
	Stage int
}

var (
	RevWorktree = Revision{Kind: "worktree"}
	RevIndex    = Revision{Kind: "index"}
	RevHead     = Revision{Kind: "commit", Rev: "HEAD"}
)

// RevCommit names a file at a commit.
func RevCommit(rev string) Revision { return Revision{Kind: "commit", Rev: rev} }

// RevStage names one index stage of an unmerged file.
func RevStage(n int) Revision { return Revision{Kind: "stage", Stage: n} }

func (rev Revision) objectName(path string) string {
	switch rev.Kind {
	case "index":
		return ":" + path
	case "stage":
		return ":" + strconv.Itoa(rev.Stage) + ":" + path
	}
	return rev.Rev + ":" + path
}

// Read loads path at rev, bounded by ContentLimit.
func (r *Runner) Read(ctx context.Context, repo domain.Repo, rev Revision, path string) (Content, error) {
	if rev.Kind == "worktree" {
		return readWorktree(filepath.Join(repo.Root, filepath.FromSlash(path)))
	}
	name := rev.objectName(path)
	out, err := r.Run(ctx, repo.Root, "cat-file", "-t", name)
	if err != nil {
		// A path missing at a revision is an absent side, not a failure.
		return Content{}, nil
	}
	switch strings.TrimSpace(string(out)) {
	case "commit":
		return Content{Exists: true, Submodule: true}, nil
	case "blob":
	default:
		return Content{}, nil
	}
	mode := r.objectMode(ctx, repo, rev, path)
	sizeOut, err := r.Run(ctx, repo.Root, "cat-file", "-s", name)
	if err != nil {
		return Content{}, err
	}
	size, _ := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64)
	c := Content{Exists: true, Size: size, Mode: mode, Symlink: mode == "120000"}
	if size > ContentLimit {
		c.TooLarge = true
		head, err := r.headOfBlob(ctx, repo, name)
		if err == nil {
			c.Binary = isBinary(head)
		}
		return c, nil
	}
	data, err := r.Run(ctx, repo.Root, "cat-file", "blob", name)
	if err != nil {
		return Content{}, err
	}
	c.Data = data
	c.Binary = isBinary(data)
	return c, nil
}

func (r *Runner) headOfBlob(ctx context.Context, repo domain.Repo, name string) ([]byte, error) {
	s, err := r.Stream(ctx, repo.Root, "cat-file", "blob", name)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	buf := make([]byte, 8000)
	n, _ := io.ReadFull(s, buf)
	return buf[:n], nil
}

func readWorktree(path string) (Content, error) {
	st, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Content{}, nil
		}
		return Content{}, err
	}
	c := Content{Exists: true, Size: st.Size(), Mode: worktreeMode(st)}
	if st.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return Content{}, err
		}
		c.Symlink = true
		c.Data = []byte(target)
		return c, nil
	}
	if st.IsDir() {
		// A submodule or nested repository.
		c.Submodule = true
		return c, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return Content{}, err
	}
	defer f.Close()
	if st.Size() > ContentLimit {
		c.TooLarge = true
		head := make([]byte, 8000)
		n, _ := io.ReadFull(f, head)
		c.Binary = isBinary(head[:n])
		return c, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, ContentLimit+1))
	if err != nil {
		return Content{}, err
	}
	c.Data = data
	c.Binary = isBinary(data)
	return c, nil
}

// objectMode is the file mode Git records for path at rev ("100644",
// "100755", "120000"), or "" when it cannot tell.
func (r *Runner) objectMode(ctx context.Context, repo domain.Repo, rev Revision, path string) string {
	switch rev.Kind {
	case "commit":
		// ls-tree matches its arguments as patterns, so keep only the exact path.
		out, err := r.Run(ctx, repo.Root, "ls-tree", "-z", "--full-tree", rev.Rev, "--", path)
		if err != nil {
			return ""
		}
		for _, rec := range strings.Split(string(out), "\x00") {
			meta, name, ok := strings.Cut(rec, "\t")
			if ok && name == path {
				return strings.Fields(meta)[0]
			}
		}
	case "index", "stage":
		out, err := r.Run(ctx, repo.Root, "ls-files", "-s", "-z", "--", ":(literal)"+path)
		if err != nil {
			return ""
		}
		want := "0"
		if rev.Kind == "stage" {
			want = strconv.Itoa(rev.Stage)
		}
		for _, rec := range strings.Split(string(out), "\x00") {
			meta, name, ok := strings.Cut(rec, "\t")
			if f := strings.Fields(meta); ok && name == path && len(f) == 3 && f[2] == want {
				return f[0]
			}
		}
	}
	return ""
}

// worktreeMode derives Git's mode from the file system. Windows has no
// executable bit, so it reports "" there and the index mode is trusted.
func worktreeMode(st os.FileInfo) string {
	switch {
	case st.Mode()&os.ModeSymlink != 0:
		return "120000"
	case st.IsDir():
		return "160000"
	case runtime.GOOS == "windows":
		return ""
	case st.Mode()&0o111 != 0:
		return "100755"
	}
	return "100644"
}

// isBinary uses Git's heuristic: a NUL byte in the first 8000 bytes.
func isBinary(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.IndexByte(data, 0) >= 0
}
