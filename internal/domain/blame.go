package domain

import (
	"strings"
	"time"
)

// BlameCommit is a commit that last touched some lines of a blamed file.
type BlameCommit struct {
	Hash        string
	Author      string
	AuthorEmail string
	Time        time.Time
	Summary     string
	// Path is the file's name in this commit (it differs after a rename).
	Path string
	// Previous is the parent and path the lines came from, "" for a root
	// commit or a boundary.
	Previous     string
	PreviousPath string
}

// Uncommitted reports the pseudo-commit Git uses for lines not committed yet.
func (c BlameCommit) Uncommitted() bool {
	return strings.Trim(c.Hash, "0") == ""
}

// Blame maps each line of a file to the commit that last changed it.
type Blame struct {
	Commits []BlameCommit
	// Lines holds, for every line, an index into Commits.
	Lines []int32
	// Orig holds every line's 1-based number in that commit.
	Orig []int32
}

// CommitAt returns the commit of a 0-based line.
func (b *Blame) CommitAt(line int) (BlameCommit, bool) {
	if b == nil || line < 0 || line >= len(b.Lines) {
		return BlameCommit{}, false
	}
	return b.Commits[b.Lines[line]], true
}

// AgeRank orders the committed commits from oldest (0) to newest (n-1) by
// author time; uncommitted lines get -1. Annotations shade by this rank.
func (b *Blame) AgeRank() ([]int, int) {
	rank := make([]int, len(b.Commits))
	var times []int64
	seen := map[int64]bool{}
	for _, c := range b.Commits {
		if c.Uncommitted() {
			continue
		}
		t := c.Time.Unix()
		if !seen[t] {
			seen[t] = true
			times = append(times, t)
		}
	}
	sortInt64(times)
	pos := make(map[int64]int, len(times))
	for i, t := range times {
		pos[t] = i
	}
	for i, c := range b.Commits {
		if c.Uncommitted() {
			rank[i] = -1
		} else {
			rank[i] = pos[c.Time.Unix()]
		}
	}
	return rank, len(times)
}

func sortInt64(s []int64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
