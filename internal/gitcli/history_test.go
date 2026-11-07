package gitcli

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestParseCommitRecordRepairsLegacyBytes covers a commit written by a client
// in a legacy encoding: Git stores the bytes it was given and only transcodes
// when the commit carries an encoding header, so the subject can arrive as
// invalid UTF-8. Everything downstream — the log list, the diagnostics bundle,
// the event log — expects valid text.
func TestParseCommitRecordRepairsLegacyBytes(t *testing.T) {
	// "暂存格" in GBK, in place of the subject field.
	gbk := string([]byte{0xd4, 0xdd, 0xb4, 0xe6, 0xb8, 0xf1})
	rec := "abc123\x1f\x1fAda\x1fada@example.com\x1f" + itoa(testTimestamp) +
		"\x1fAda\x1f" + itoa(testTimestamp) + "\x1f" + gbk
	c, ok := parseCommitRecord([]byte(rec))
	if !ok {
		t.Fatal("parseCommitRecord rejected a well-formed record")
	}
	if !utf8.ValidString(c.Subject) {
		t.Errorf("subject is still invalid UTF-8: %q", c.Subject)
	}
	if !strings.ContainsRune(c.Subject, utf8.RuneError) {
		t.Errorf("subject = %q, want the invalid bytes replaced with U+FFFD", c.Subject)
	}
	if c.Author != "Ada" || c.Committer != "Ada" {
		t.Errorf("author/committer = %q/%q", c.Author, c.Committer)
	}
	// The rest of the record must survive: repairing text never costs the
	// timestamps or the hash.
	if !c.AuthorTime.Equal(time.Unix(testTimestamp, 0)) {
		t.Errorf("author time = %v", c.AuthorTime)
	}
	if c.Hash != "abc123" {
		t.Errorf("hash = %q", c.Hash)
	}
}

// testTimestamp is a fixed commit time; the record parser only needs to see it
// round-trip, never to read it.
const testTimestamp = 1_784_000_000

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
