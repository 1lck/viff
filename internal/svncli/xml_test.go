package svncli

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// The samples below are verbatim output of svn 1.15.0 on Windows x64, captured
// on the reference machine. They are embedded byte for byte, indentation
// included, so the parser is tested against what svn really prints rather than a
// tidied version of it.
const statusSample = `<?xml version="1.0" encoding="UTF-8"?>
<status>
<target
   path=".">
<entry
   path="added.txt">
<wc-status
   props="none"
   item="added"
   revision="-1">
</wc-status>
</entry>
<entry
   path="docs\a.txt">
<wc-status
   props="none"
   item="missing"
   revision="2">
<commit
   revision="2">
<author>momo</author>
<date>2026-11-05T07:20:51.371585Z</date>
</commit>
</wc-status>
</entry>
<entry
   path="newdir">
<wc-status
   item="unversioned"
   props="none">
</wc-status>
</entry>
</target>
</status>
`

const infoSample = `<?xml version="1.0" encoding="UTF-8"?>
<info>
<entry
   kind="dir"
   path="."
   revision="1">
<url>file:///C:/repo/trunk/src</url>
<relative-url>^/trunk/src</relative-url>
<repository>
<root>file:///C:/repo</root>
<uuid>a3a0b7ef-e7b1-e348-8697-531da99edfa1</uuid>
</repository>
<wc-info>
<wcroot-abspath>C:/repo/wc</wcroot-abspath>
<wc-compatible-version>1.8</wc-compatible-version>
<wc-format>31</wc-format>
<store-pristine>yes</store-pristine>
<schedule>normal</schedule>
<depth>infinity</depth>
</wc-info>
<commit
   revision="1">
<author>momo</author>
<date>2026-11-05T07:20:45.749368Z</date>
</commit>
</entry>
</info>
`

const logSample = `<?xml version="1.0" encoding="UTF-8"?>
<log>
<logentry
   revision="1">
<author>momo</author>
<date>2026-11-05T07:20:45.749368Z</date>
<paths>
<path
   text-mods="false"
   kind="dir"
   action="A"
   prop-mods="false">/branches</path>
</paths>
<msg>init layout</msg>
</logentry>
</log>
`

// blameSample is svn's real multi-target output: one closed target per file,
// including a target with no entries. Two targets in one run is the normal case
// for a conflicted file, which also gets a ".mine" target.
const blameSample = `<?xml version="1.0" encoding="UTF-8"?>
<blame>
<target
   path="src\main.c">
<entry
   line-number="1">
<commit
   revision="2">
<author>momo</author>
<date>2026-11-05T07:20:51.371585Z</date>
</commit>
</entry>
<entry
   line-number="2">
</entry>
</target>
<target
   path="src\main.c.mine">
</target>
</blame>
`

// blameTruncatedSample is the same capture with the first </target> dropped. It
// stands for output cut short by an interrupted process, which has to surface as
// an error rather than a panic or a half-filled result.
const blameTruncatedSample = `<?xml version="1.0" encoding="UTF-8"?>
<blame>
<target
   path="src\main.c">
<entry
   line-number="1">
<commit
   revision="2">
<author>momo</author>
<date>2026-11-05T07:20:51.371585Z</date>
</commit>
</entry>
<entry
   line-number="2">
</entry>
<target
   path="src\main.c.mine">
</target>
</blame>
`

func TestParseStatus(t *testing.T) {
	entries, err := ParseStatus([]byte(statusSample))
	if err != nil {
		t.Fatalf("ParseStatus: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	added := entries[0]
	if added.Path != "added.txt" || added.Item != "added" || added.Revision != "-1" || added.Props != "none" {
		t.Errorf("added entry = %+v", added)
	}
	// An added file has no commit element yet, so author and date stay empty
	// instead of turning into an error.
	if added.Author != "" || !added.Date.IsZero() {
		t.Errorf("added entry author/date = %q/%v, want empty", added.Author, added.Date)
	}
	if added.Conflicted() {
		t.Error("added entry reported as conflicted")
	}

	// Windows separators are kept exactly as svn printed them.
	missing := entries[1]
	if missing.Path != `docs\a.txt` {
		t.Errorf("missing entry path = %q, want %q", missing.Path, `docs\a.txt`)
	}
	if missing.Item != "missing" || missing.Revision != "2" || missing.Props != "none" {
		t.Errorf("missing entry = %+v", missing)
	}
	if missing.Author != "momo" {
		t.Errorf("missing entry author = %q, want momo", missing.Author)
	}
	wantDate := time.Date(2026, time.November, 5, 7, 20, 51, 371585000, time.UTC)
	if !missing.Date.Equal(wantDate) {
		t.Errorf("missing entry date = %v, want %v", missing.Date, wantDate)
	}

	unversioned := entries[2]
	if unversioned.Path != "newdir" || unversioned.Item != "unversioned" {
		t.Errorf("unversioned entry = %+v", unversioned)
	}
	// svn omits the revision attribute for unversioned paths.
	if unversioned.Revision != "" {
		t.Errorf("unversioned revision = %q, want empty", unversioned.Revision)
	}
}

func TestParseStatusEdges(t *testing.T) {
	t.Run("empty status element", func(t *testing.T) {
		entries, err := ParseStatus([]byte(`<?xml version="1.0" encoding="UTF-8"?><status></status>`))
		if err != nil {
			t.Fatalf("ParseStatus: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("got %d entries, want 0", len(entries))
		}
	})

	t.Run("conflicted item and props modified", func(t *testing.T) {
		data := `<status><target path=".">` +
			`<entry path="c.txt"><wc-status props="conflicted" item="conflicted" revision="3"></wc-status></entry>` +
			`<entry path="p.txt"><wc-status props="modified" item="modified" revision="4"></wc-status></entry>` +
			`</target></status>`
		entries, err := ParseStatus([]byte(data))
		if err != nil {
			t.Fatalf("ParseStatus: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if !entries[0].Conflicted() {
			t.Errorf("entry %+v not reported as conflicted", entries[0])
		}
		if entries[1].Props != "modified" || entries[1].Conflicted() {
			t.Errorf("props-only entry = %+v", entries[1])
		}
	})

	t.Run("changelist entries are kept", func(t *testing.T) {
		data := `<status><target path=".">` +
			`<entry path="plain.txt"><wc-status props="none" item="unversioned"></wc-status></entry>` +
			`<changelist name="wip">` +
			`<entry path="listed.txt"><wc-status props="none" item="modified" revision="7"></wc-status></entry>` +
			`</changelist>` +
			`</target></status>`
		entries, err := ParseStatus([]byte(data))
		if err != nil {
			t.Fatalf("ParseStatus: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if entries[0].Path != "plain.txt" || entries[1].Path != "listed.txt" {
			t.Errorf("paths = %q, %q", entries[0].Path, entries[1].Path)
		}
	})

	t.Run("unknown elements are ignored", func(t *testing.T) {
		data := `<status><target path="."><entry path="x.txt">` +
			`<wc-status props="none" item="normal" revision="9"><tree-conflicted>false</tree-conflicted></wc-status>` +
			`</entry></target></status>`
		entries, err := ParseStatus([]byte(data))
		if err != nil {
			t.Fatalf("ParseStatus: %v", err)
		}
		if len(entries) != 1 || entries[0].Path != "x.txt" || entries[0].Item != "normal" {
			t.Fatalf("entries = %+v", entries)
		}
	})

	t.Run("malformed xml returns an error", func(t *testing.T) {
		for _, input := range []string{"", `<status>`, `<status><target path="."><entry path="a.txt">`} {
			if _, err := ParseStatus([]byte(input)); err == nil {
				t.Errorf("ParseStatus(%q) returned no error", input)
			}
		}
	})

	t.Run("invalid commit date returns an error", func(t *testing.T) {
		data := `<status><target path="."><entry path="a.txt">` +
			`<wc-status props="none" item="modified" revision="2">` +
			`<commit revision="2"><author>momo</author><date>2026-13-45</date></commit>` +
			`</wc-status></entry></target></status>`
		if _, err := ParseStatus([]byte(data)); err == nil {
			t.Error("ParseStatus with invalid date returned no error")
		}
	})
}

func TestParseInfo(t *testing.T) {
	info, err := ParseInfo([]byte(infoSample))
	if err != nil {
		t.Fatalf("ParseInfo: %v", err)
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"path", info.Path, "."},
		{"kind", info.Kind, "dir"},
		// revision is the path's BASE revision, not the repository HEAD.
		{"revision", info.Revision, "1"},
		{"url", info.URL, "file:///C:/repo/trunk/src"},
		{"relative url", info.RelativeURL, "^/trunk/src"},
		{"repo root", info.RepoRoot, "file:///C:/repo"},
		{"repo uuid", info.RepoUUID, "a3a0b7ef-e7b1-e348-8697-531da99edfa1"},
		// wcroot-abspath uses forward slashes on Windows and stays untouched.
		{"wc root", info.WCRootAbsPath, "C:/repo/wc"},
		{"wc format", info.WCFormat, "31"},
		{"last changed revision", info.LastChangedRevision, "1"},
		{"last changed author", info.LastChangedAuthor, "momo"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	wantDate := time.Date(2026, time.November, 5, 7, 20, 45, 749368000, time.UTC)
	if !info.LastChangedDate.Equal(wantDate) {
		t.Errorf("last changed date = %v, want %v", info.LastChangedDate, wantDate)
	}
}

func TestParseInfoEdges(t *testing.T) {
	t.Run("missing commit element", func(t *testing.T) {
		data := `<info><entry kind="file" path="a.txt" revision="5">` +
			`<url>file:///repo/a.txt</url><relative-url>^/a.txt</relative-url>` +
			`</entry></info>`
		info, err := ParseInfo([]byte(data))
		if err != nil {
			t.Fatalf("ParseInfo: %v", err)
		}
		if info.Kind != "file" || info.Revision != "5" || info.URL != "file:///repo/a.txt" {
			t.Errorf("info = %+v", info)
		}
		if info.LastChangedRevision != "" || info.LastChangedAuthor != "" || !info.LastChangedDate.IsZero() {
			t.Errorf("last changed = %q/%q/%v, want zero values",
				info.LastChangedRevision, info.LastChangedAuthor, info.LastChangedDate)
		}
	})

	t.Run("no entry element", func(t *testing.T) {
		if _, err := ParseInfo([]byte(`<info></info>`)); err == nil {
			t.Error("ParseInfo with no entry returned no error")
		}
	})

	t.Run("malformed xml", func(t *testing.T) {
		for _, input := range []string{"", `<info><entry kind="file"`} {
			if _, err := ParseInfo([]byte(input)); err == nil {
				t.Errorf("ParseInfo(%q) returned no error", input)
			}
		}
	})

	t.Run("invalid commit date", func(t *testing.T) {
		data := `<info><entry kind="dir" path="." revision="1">` +
			`<commit revision="1"><date>2026-11-05T07:20:45</date></commit>` +
			`</entry></info>`
		if _, err := ParseInfo([]byte(data)); err == nil {
			t.Error("ParseInfo with invalid date returned no error")
		}
	})
}

func TestParseLog(t *testing.T) {
	entries, err := ParseLog([]byte(logSample))
	if err != nil {
		t.Fatalf("ParseLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Revision != 1 {
		t.Errorf("revision = %d, want 1", entry.Revision)
	}
	if entry.Author != "momo" || entry.Message != "init layout" {
		t.Errorf("author/message = %q/%q", entry.Author, entry.Message)
	}
	wantDate := time.Date(2026, time.November, 5, 7, 20, 45, 749368000, time.UTC)
	if !entry.Date.Equal(wantDate) {
		t.Errorf("date = %v, want %v", entry.Date, wantDate)
	}
	if len(entry.Paths) != 1 {
		t.Fatalf("got %d paths, want 1", len(entry.Paths))
	}
	changed := entry.Paths[0]
	if changed.Path != "/branches" || changed.Action != "A" || changed.Kind != "dir" {
		t.Errorf("path change = %+v", changed)
	}
	if changed.TextMods || changed.PropMods {
		t.Errorf("text/prop mods = %v/%v, want false", changed.TextMods, changed.PropMods)
	}
}

func TestParseLogEdges(t *testing.T) {
	t.Run("empty log element", func(t *testing.T) {
		entries, err := ParseLog([]byte(`<log></log>`))
		if err != nil {
			t.Fatalf("ParseLog: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("got %d entries, want 0", len(entries))
		}
	})

	t.Run("anonymous commit with no message and no paths", func(t *testing.T) {
		data := `<log><logentry revision="7">` +
			`<date>2026-11-05T07:20:45.749368Z</date><msg></msg>` +
			`</logentry></log>`
		entries, err := ParseLog([]byte(data))
		if err != nil {
			t.Fatalf("ParseLog: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("got %d entries, want 1", len(entries))
		}
		entry := entries[0]
		if entry.Author != "" || entry.Message != "" {
			t.Errorf("author/message = %q/%q, want empty", entry.Author, entry.Message)
		}
		if len(entry.Paths) != 0 {
			t.Errorf("paths = %+v, want none", entry.Paths)
		}
		if !entry.Date.Equal(time.Date(2026, time.November, 5, 7, 20, 45, 749368000, time.UTC)) {
			t.Errorf("date = %v", entry.Date)
		}
	})

	t.Run("prop-only change and absent mod attributes", func(t *testing.T) {
		data := `<log><logentry revision="8"><paths>` +
			`<path text-mods="false" kind="file" action="M" prop-mods="true">/trunk/a.txt</path>` +
			`<path text-mods="true" kind="file" action="M" prop-mods="true">/trunk/b.txt</path>` +
			`<path kind="file" action="D">/trunk/c.txt</path>` +
			`</paths><msg>tweak</msg></logentry></log>`
		entries, err := ParseLog([]byte(data))
		if err != nil {
			t.Fatalf("ParseLog: %v", err)
		}
		entry := entries[0]
		if entry.Author != "" {
			t.Errorf("author = %q, want empty", entry.Author)
		}
		if len(entry.Paths) != 3 {
			t.Fatalf("got %d paths, want 3", len(entry.Paths))
		}
		if entry.Paths[0].TextMods || !entry.Paths[0].PropMods {
			t.Errorf("prop-only change = %+v", entry.Paths[0])
		}
		if !entry.Paths[1].TextMods || !entry.Paths[1].PropMods {
			t.Errorf("mixed change = %+v", entry.Paths[1])
		}
		// Absent attributes mean "not reported" and must not fail the parse.
		if entry.Paths[2].TextMods || entry.Paths[2].PropMods || entry.Paths[2].Action != "D" {
			t.Errorf("delete without mods = %+v", entry.Paths[2])
		}
	})

	t.Run("malformed input returns an error", func(t *testing.T) {
		inputs := []string{
			"",
			`<log><logentry revision="1">`,
			`<log><logentry revision="abc"></logentry></log>`,
			`<log><logentry revision="1"><date>yesterday</date></logentry></log>`,
			`<log><logentry revision="1"><paths><path text-mods="maybe" prop-mods="false">/a</path></paths></logentry></log>`,
			`<log><logentry revision="1"><paths><path text-mods="true" prop-mods="maybe">/a</path></paths></logentry></log>`,
		}
		for _, input := range inputs {
			if _, err := ParseLog([]byte(input)); err == nil {
				t.Errorf("ParseLog(%q) returned no error", input)
			}
		}
	})
}

func TestParseBlame(t *testing.T) {
	files, err := ParseBlame([]byte(blameSample))
	if err != nil {
		t.Fatalf("ParseBlame: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d targets, want 2", len(files))
	}

	main := files[0]
	// Windows separators from the target path are kept as printed.
	if main.TargetPath != `src\main.c` {
		t.Errorf("target path = %q, want %q", main.TargetPath, `src\main.c`)
	}
	if len(main.Lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(main.Lines))
	}
	first := main.Lines[0]
	if first.LineNumber != 1 || first.Revision != "2" || first.Author != "momo" {
		t.Errorf("first line = %+v", first)
	}
	wantDate := time.Date(2026, time.November, 5, 7, 20, 51, 371585000, time.UTC)
	if !first.Date.Equal(wantDate) {
		t.Errorf("first line date = %v, want %v", first.Date, wantDate)
	}
	// A line without a commit element carries no attribution. That is normal
	// output for untouched lines and must not be an error.
	second := main.Lines[1]
	if second.LineNumber != 2 || second.Revision != "" || second.Author != "" || !second.Date.IsZero() {
		t.Errorf("second line = %+v", second)
	}

	// The second target has no entries but still has to be reported.
	mine := files[1]
	if mine.TargetPath != `src\main.c.mine` {
		t.Errorf("second target path = %q", mine.TargetPath)
	}
	if len(mine.Lines) != 0 {
		t.Errorf("second target lines = %+v, want none", mine.Lines)
	}
}

func TestParseBlameMalformed(t *testing.T) {
	// Truncated output has to surface as an error, never a panic or a half result.
	if _, err := ParseBlame([]byte(blameTruncatedSample)); err == nil {
		t.Fatal("ParseBlame on truncated output returned no error")
	}
	inputs := []string{
		"",
		`<blame><target path="a.c">`,
		`<blame><target path="a.c"><entry line-number="x"></entry></target></blame>`,
		`<blame><target path="a.c"><entry line-number="1"><commit revision="2"><date>nope</date></commit></entry></target></blame>`,
	}
	for _, input := range inputs {
		if _, err := ParseBlame([]byte(input)); err == nil {
			t.Errorf("ParseBlame(%q) returned no error", input)
		}
	}
}

func TestParseTime(t *testing.T) {
	// svn prints microsecond precision with a trailing Z. The instant has to be
	// kept exactly; the zone becomes the user's, because every view of Viff
	// speaks local time and svn reports UTC.
	got, err := parseTime("2026-11-05T07:20:51.371585Z")
	if err != nil {
		t.Fatalf("parseTime: %v", err)
	}
	want := time.Date(2026, time.November, 5, 7, 20, 51, 371585000, time.UTC)
	if !got.Equal(want) {
		t.Errorf("instant = %v, want %v", got, want)
	}
	if got.Location() != time.Local {
		t.Errorf("location = %v, want the local zone", got.Location())
	}
	if got.Nanosecond() != 371585000 {
		t.Errorf("nanoseconds = %d", got.Nanosecond())
	}

	// Missing values are the common case for unversioned entries and untouched
	// blame lines, so they must be the zero time and not an error.
	zero, err := parseTime("")
	if err != nil {
		t.Fatalf("parseTime(empty): %v", err)
	}
	if !zero.IsZero() {
		t.Errorf("parseTime(empty) = %v, want zero time", zero)
	}

	if _, err := parseTime("not a timestamp"); err == nil {
		t.Error("parseTime with garbage returned no error")
	}
}

func TestParseBoolAttr(t *testing.T) {
	// svn always writes "true"/"false"; an absent attribute reads as false.
	trueValue, err := parseBoolAttr("true")
	if err != nil || !trueValue {
		t.Errorf("parseBoolAttr(true) = %v, %v", trueValue, err)
	}
	falseValue, err := parseBoolAttr("false")
	if err != nil || falseValue {
		t.Errorf("parseBoolAttr(false) = %v, %v", falseValue, err)
	}
	absent, err := parseBoolAttr("")
	if err != nil || absent {
		t.Errorf("parseBoolAttr(empty) = %v, %v", absent, err)
	}
	if _, err := parseBoolAttr("yes"); err == nil {
		t.Error("parseBoolAttr(yes) returned no error")
	}
}

func TestStatusEntryConflicted(t *testing.T) {
	if !(StatusEntry{Item: "conflicted"}).Conflicted() {
		t.Error("conflicted item not detected")
	}
	for _, item := range []string{"normal", "modified", "added", "deleted", "missing", "unversioned"} {
		if (StatusEntry{Item: item}).Conflicted() {
			t.Errorf("item %q reported as conflicted", item)
		}
	}
}

// TestParseTimeReturnsLocalZone pins the zone svn timestamps leave the parser
// in. svn reports UTC and every view of Viff speaks the user's zone, so a
// timestamp that keeps "UTC" here shows a noon commit as "Today 04:51" and
// files it under the wrong day.
func TestParseTimeReturnsLocalZone(t *testing.T) {
	got, err := parseTime("2026-11-06T04:51:00.000000Z")
	if err != nil {
		t.Fatalf("parseTime: %v", err)
	}
	want := time.Date(2026, time.November, 6, 4, 51, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("instant = %v, want %v", got, want)
	}
	if got.Location() != time.Local {
		t.Errorf("location = %v, want the local zone", got.Location())
	}
}

// TestParseLogToleratesInvalidUTF8 covers a repository written by a client in a
// legacy encoding: svn declares UTF-8 in the XML header no matter what the
// stored bytes are, and encoding/xml used to reject the whole document, which
// failed the entire log view over one damaged revision.
func TestParseLogToleratesInvalidUTF8(t *testing.T) {
	// "暂存格" in GBK, as a Chinese Windows client would have committed it.
	gbk := []byte{0xd4, 0xdd, 0xb4, 0xe6, 0xb8, 0xf1}
	data := append([]byte(`<?xml version="1.0" encoding="UTF-8"?><log><logentry revision="12">`+
		`<author>momo</author><date>2026-11-06T04:51:00.000000Z</date><msg>`), gbk...)
	data = append(data, []byte(`</msg></logentry></log>`)...)
	entries, err := ParseLog(data)
	if err != nil {
		t.Fatalf("ParseLog with legacy bytes: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Revision != 12 || entry.Author != "momo" {
		t.Errorf("revision/author = %d/%q", entry.Revision, entry.Author)
	}
	if entry.Message == "" {
		t.Error("message was dropped instead of repaired")
	}
	if !utf8.ValidString(entry.Message) {
		t.Errorf("message is still invalid UTF-8: %q", entry.Message)
	}
	if !strings.ContainsRune(entry.Message, utf8.RuneError) {
		t.Errorf("message = %q, want the invalid bytes replaced with U+FFFD", entry.Message)
	}
}

// TestParseStatusToleratesInvalidUTF8 keeps the other parsers honest: paths come
// from the same working copy, so they carry the same bytes.
func TestParseStatusToleratesInvalidUTF8(t *testing.T) {
	path := append([]byte(`<?xml version="1.0" encoding="UTF-8"?><status><target path="`), 0xd4, 0xdd)
	path = append(path, []byte(`"><entry path="src\`+"\xd4\xdd"+`.go"><wc-status item="modified" props="none" revision="3"/></entry></target></status>`)...)
	entries, err := ParseStatus(path)
	if err != nil {
		t.Fatalf("ParseStatus with legacy bytes: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if !utf8.ValidString(entries[0].Path) {
		t.Errorf("path is still invalid UTF-8: %q", entries[0].Path)
	}
}
