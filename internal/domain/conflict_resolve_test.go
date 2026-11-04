package domain

import (
	"errors"
	"strings"
	"testing"
)

// blockCount parses text the way the viewer does.
func blockCount(text string) int {
	lines, _ := SplitLines(text)
	return len(ParseConflictMarkers(lines).Blocks)
}

func TestResolveConflictTextTwoWay(t *testing.T) {
	text := strings.Join([]string{
		"header",
		"<<<<<<< HEAD",
		"ours line",
		"=======",
		"theirs line",
		">>>>>>> feature",
		"footer",
		"",
	}, "\n")
	cases := []struct {
		choice ResolveChoice
		want   string
	}{
		{ResolveOurs, "header\nours line\nfooter\n"},
		{ResolveTheirs, "header\ntheirs line\nfooter\n"},
		{ResolveBoth, "header\nours line\ntheirs line\nfooter\n"},
	}
	for _, c := range cases {
		got, err := ResolveConflictText(text, 0, c.choice)
		if err != nil {
			t.Fatalf("%v: %v", c.choice, err)
		}
		if got != c.want {
			t.Fatalf("%v = %q, want %q", c.choice, got, c.want)
		}
		if blockCount(got) != 0 {
			t.Fatalf("%v left markers: %q", c.choice, got)
		}
	}
}

func TestResolveConflictTextDiff3DropsBase(t *testing.T) {
	text := strings.Join([]string{
		"top",
		"<<<<<<< ours",
		"ours",
		"||||||| base",
		"base",
		"=======",
		"theirs",
		">>>>>>> theirs",
		"bottom",
		"",
	}, "\n")
	for _, c := range []struct {
		choice ResolveChoice
		want   string
	}{
		{ResolveOurs, "top\nours\nbottom\n"},
		{ResolveTheirs, "top\ntheirs\nbottom\n"},
		{ResolveBoth, "top\nours\ntheirs\nbottom\n"},
	} {
		got, err := ResolveConflictText(text, 0, c.choice)
		if err != nil {
			t.Fatalf("%v: %v", c.choice, err)
		}
		if got != c.want {
			t.Fatalf("%v = %q, want %q", c.choice, got, c.want)
		}
	}
}

func TestResolveConflictTextOnlyTouchesOneBlock(t *testing.T) {
	first := []string{
		"one",
		"<<<<<<< ours",
		"one ours",
		"=======",
		"one theirs",
		">>>>>>> theirs",
	}
	second := []string{
		"two",
		"<<<<<<< ours",
		"two ours",
		"=======",
		"two theirs",
		">>>>>>> theirs",
	}
	text := strings.Join(append(append(append([]string{}, first...), second...), "end", ""), "\n")
	got, err := ResolveConflictText(text, 1, ResolveTheirs)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join(append(append([]string{}, first...), "two", "two theirs", "end", ""), "\n")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if blockCount(got) != 1 {
		t.Fatalf("blocks left = %d, want 1", blockCount(got))
	}
	// The untouched block is byte-identical, labels included.
	if !strings.Contains(got, "<<<<<<< ours\none ours\n=======\none theirs\n>>>>>>> theirs\n") {
		t.Fatalf("first block changed: %q", got)
	}
	// The other choice then resolves the remaining block too.
	got, err = ResolveConflictText(got, 0, ResolveBoth)
	if err != nil {
		t.Fatal(err)
	}
	want = strings.Join([]string{"one", "one ours", "one theirs", "two", "two theirs", "end", ""}, "\n")
	if got != want {
		t.Fatalf("both = %q, want %q", got, want)
	}
}

func TestResolveConflictTextPreservesCRLFAndFinalNewline(t *testing.T) {
	text := "one\r\n<<<<<<< ours\r\nours\r\n=======\r\ntheirs\r\n>>>>>>> theirs\r\ntwo\r\n"
	got, err := ResolveConflictText(text, 0, ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	want := "one\r\nours\r\ntwo\r\n"
	if got != want {
		t.Fatalf("CRLF = %q, want %q", got, want)
	}
	if strings.Contains(got, "\n") && strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("bare LF left in %q", got)
	}
}

func TestResolveConflictTextPreservesNoFinalNewline(t *testing.T) {
	text := "one\n<<<<<<< ours\nours\n=======\ntheirs\n>>>>>>> theirs\ntwo"
	got, err := ResolveConflictText(text, 0, ResolveTheirs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "one\ntheirs\ntwo" {
		t.Fatalf("no-final-newline = %q", got)
	}
	// A closing marker that itself ends the file without a newline: the
	// chosen CRLF line must lose the now-unused carriage return.
	text = "top\r\n<<<<<<< ours\r\nours\r\n=======\r\ntheirs\r\n>>>>>>> theirs"
	got, err = ResolveConflictText(text, 0, ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "top\r\nours" {
		t.Fatalf("CRLF no-final-newline = %q", got)
	}
}

func TestResolveConflictTextKeepsUTF8BOM(t *testing.T) {
	text := conflictBOM + "<<<<<<< ours\nours\n=======\ntheirs\n>>>>>>> theirs\ntail\n"
	got, err := ResolveConflictText(text, 0, ResolveBoth)
	if err != nil {
		t.Fatal(err)
	}
	want := conflictBOM + "ours\ntheirs\ntail\n"
	if got != want {
		t.Fatalf("BOM = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, conflictBOM) {
		t.Fatal("BOM was dropped")
	}
}

func TestResolveConflictTextEmptySides(t *testing.T) {
	// Ours deletes the line, theirs keeps one: accepting either must give a
	// sane file, not a stray blank line.
	text := "a\n<<<<<<< ours\n=======\ntheirs\n>>>>>>> theirs\nb\n"
	got, err := ResolveConflictText(text, 0, ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\nb\n" {
		t.Fatalf("empty ours = %q", got)
	}
	got, err = ResolveConflictText(text, 0, ResolveBoth)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\ntheirs\nb\n" {
		t.Fatalf("both with empty ours = %q", got)
	}
	text = "a\n<<<<<<< ours\nours\n=======\n>>>>>>> theirs\nb\n"
	got, err = ResolveConflictText(text, 0, ResolveTheirs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\nb\n" {
		t.Fatalf("empty theirs = %q", got)
	}
}

func TestResolveConflictTextCustomMarkerSize(t *testing.T) {
	text := "a\n<<<<<<<<< ours\nours\n=========\ntheirs\n>>>>>>>>> theirs\nb\n"
	got, err := ResolveConflictText(text, 0, ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\nours\nb\n" {
		t.Fatalf("marker size 9 = %q", got)
	}
}

func TestResolveConflictTextRefusesExposedLiteralMarkers(t *testing.T) {
	// A nine-character block nested in a seven-character one is literal
	// content of the outer block. Accepting the outer side would expose it at
	// file level, where it reads as a block of its own and would leave the
	// viewer asking the user to resolve their own text. Refuse instead: the
	// file is left for the user to edit, with nothing lost.
	text := strings.Join([]string{
		"<<<<<<< ours",
		"<<<<<<<<< literal",
		"literal",
		"=========",
		"literal theirs",
		">>>>>>>>> literal",
		"=======",
		"theirs",
		">>>>>>> theirs",
		"",
	}, "\n")
	got, err := ResolveConflictText(text, 0, ResolveOurs)
	if !errors.Is(err, ErrConflictAmbiguous) {
		t.Fatalf("err = %v, text = %q", err, got)
	}
	if got != "" {
		t.Fatalf("returned text %q on error", got)
	}
}

func TestResolveConflictTextKeepsNonUTF8Bytes(t *testing.T) {
	// Latin-1 bytes outside and inside the block must survive unchanged.
	text := "caf\xe9\n<<<<<<< ours\n\xe9ours\n=======\ntheirs\n>>>>>>> theirs\n\xff\n"
	got, err := ResolveConflictText(text, 0, ResolveOurs)
	if err != nil {
		t.Fatal(err)
	}
	want := "caf\xe9\n\xe9ours\n\xff\n"
	if got != want {
		t.Fatalf("bytes = %q, want %q", got, want)
	}
}

func TestResolveConflictTextRefusesUnsafeText(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		block int
		want  error
	}{
		{"no blocks", "one\ntwo\n", 0, ErrConflictAbsent},
		{"unterminated", "one\n<<<<<<< ours\nours\n=======\ntheirs\n", 0, ErrConflictMarkers},
		{"stray closer", ">>>>>>> theirs\n", 0, ErrConflictMarkers},
		{"out of range", "<<<<<<< ours\nours\n=======\ntheirs\n>>>>>>> theirs\n", 3, ErrConflictBlock},
		{"negative", "<<<<<<< ours\nours\n=======\ntheirs\n>>>>>>> theirs\n", -1, ErrConflictBlock},
	}
	for _, c := range cases {
		got, err := ResolveConflictText(c.text, c.block, ResolveOurs)
		if !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if got != "" {
			t.Fatalf("%s: returned text %q on error", c.name, got)
		}
	}
	// A content line that looks like a closer becomes an ambiguous marker
	// once the enclosing block is gone: refuse instead of writing it.
	text := "before\n<<<<<<< ours\n>>>>>>> ours\n=======\ntheirs\n>>>>>>> theirs\nafter\n"
	if got, err := ResolveConflictText(text, 0, ResolveOurs); !errors.Is(err, ErrConflictAmbiguous) {
		t.Fatalf("ambiguous result: err = %v text = %q", err, got)
	}
	// An unknown choice is refused.
	if _, err := ResolveConflictText("a\n", 0, ResolveChoice(9)); err == nil {
		t.Fatal("unknown choice accepted")
	}
}

func TestResolveChoiceStrings(t *testing.T) {
	if ResolveOurs.String() != "Ours" || ResolveTheirs.String() != "Theirs" || ResolveBoth.String() != "Both" {
		t.Fatalf("choices = %q, %q, %q", ResolveOurs, ResolveTheirs, ResolveBoth)
	}
	if !ResolveOurs.Valid() || !ResolveBoth.Valid() || ResolveChoice(3).Valid() {
		t.Fatal("Valid is wrong")
	}
}

func TestResolveConflictTextRealCRLFMarkers(t *testing.T) {
	// Git writes its marker lines with LF even in a CRLF working file; the
	// content lines keep their CRLF. Removing the markers must leave the file
	// with CRLF endings throughout.
	text := "one\r\n<<<<<<< HEAD\nours\r\n=======\ntheirs\r\n>>>>>>> other\r\n"
	got, err := ResolveConflictText(text, 0, ResolveTheirs)
	if err != nil {
		t.Fatal(err)
	}
	if got != "one\r\ntheirs\r\n" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("bare LF left in %q", got)
	}
}
