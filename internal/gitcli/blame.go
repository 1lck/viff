package gitcli

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"viff/internal/domain"
)

// Blame annotates path at rev; an empty rev blames the working-tree file, where
// lines that are not committed yet belong to the all-zero pseudo-commit.
func (r *Runner) Blame(ctx context.Context, repo domain.Repo, rev, path string) (*domain.Blame, error) {
	args := []string{"blame", "--porcelain"}
	if rev != "" {
		args = append(args, rev)
	}
	args = append(args, "--", path)
	out, err := r.Run(ctx, repo.Root, args...)
	if err != nil {
		return nil, err
	}
	return ParseBlame(out), nil
}

// ParseBlame reads `git blame --porcelain` output. Commit details come once,
// after the first line a commit owns; later lines repeat only the header.
func ParseBlame(out []byte) *domain.Blame {
	b := &domain.Blame{}
	index := map[string]int32{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	cur := int32(-1)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\t") {
			// The content line ends the entry.
			continue
		}
		if cur < 0 || isBlameHeader(line) {
			f := strings.Fields(line)
			if len(f) < 3 {
				continue
			}
			i, ok := index[f[0]]
			if !ok {
				i = int32(len(b.Commits))
				index[f[0]] = i
				b.Commits = append(b.Commits, domain.BlameCommit{Hash: f[0]})
			}
			cur = i
			orig, _ := strconv.Atoi(f[1])
			b.Lines = append(b.Lines, i)
			b.Orig = append(b.Orig, int32(orig))
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		c := &b.Commits[cur]
		switch key {
		case "author":
			c.Author = val
		case "author-mail":
			c.AuthorEmail = strings.Trim(val, "<>")
		case "author-time":
			if s, err := strconv.ParseInt(val, 10, 64); err == nil {
				c.Time = time.Unix(s, 0)
			}
		case "summary":
			c.Summary = val
		case "filename":
			c.Path = val
		case "previous":
			c.Previous, c.PreviousPath, _ = strings.Cut(val, " ")
		}
	}
	return b
}

// isBlameHeader recognizes "<hash> <orig> <final> [<count>]".
func isBlameHeader(line string) bool {
	f := strings.Fields(line)
	if len(f) < 3 || len(f) > 4 || len(f[0]) < 40 {
		return false
	}
	for _, c := range f[0] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	for _, n := range f[1:] {
		if _, err := strconv.Atoi(n); err != nil {
			return false
		}
	}
	return true
}
