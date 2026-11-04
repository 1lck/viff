package viffui

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vivaui/viva/ui"
)

// A small lexer for the diff viewer: comments, strings, numbers, keywords,
// calls and annotations in the colours of IntelliJ's Darcula/Islands
// scheme. It is line-based with the state (block comment, raw string)
// carried across lines; it never builds a tree.

type tokKind uint8

const (
	tkPlain tokKind = iota
	tkKeyword
	tkString
	tkNumber
	tkComment
	tkFunction
	tkMeta
	tkConstant
	tkTag
)

type span struct {
	start, end int
	kind       tokKind
}

type lang struct {
	line     []string // line comment openers
	blockA   string
	blockB   string
	quotes   string // string delimiters
	raw      byte   // multi-line raw string delimiter (` in Go/JS)
	keywords map[string]bool
	caseFold bool
	markup   bool // XML/HTML
	json     bool
	markdown bool
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

var (
	kwC     = words("auto break case char const continue default do double else enum extern float for goto if inline int long register restrict return short signed sizeof static struct switch typedef union unsigned void volatile while bool true false NULL nullptr class namespace template typename public private protected virtual override final new delete this using try catch throw operator friend explicit constexpr noexcept include define ifdef ifndef endif pragma")
	kwGo    = words("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil iota any error string int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 uintptr byte rune float32 float64 bool complex64 complex128 make new len cap append copy delete panic recover close min max clear")
	kwJava  = words("abstract assert boolean break byte case catch char class const continue default do double else enum extends final finally float for goto if implements import instanceof int interface long native new package private protected public return short static strictfp super switch synchronized this throw throws transient try void volatile while true false null var record sealed permits yield fun val when object companion data inner open lateinit by in is as typealias suspend override internal")
	kwJS    = words("break case catch class const continue debugger default delete do else export extends finally for function if import in instanceof new return super switch this throw try typeof var void while with yield let static async await of true false null undefined interface type enum implements private public protected readonly declare namespace abstract as from keyof never unknown any number string boolean")
	kwPy    = words("False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield self match case")
	kwRust  = words("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize f32 f64 bool char str String Vec Option Some None Result Ok Err")
	kwSwift = words("associatedtype class deinit enum extension fileprivate func import init inout internal let open operator private protocol public rethrows static struct subscript typealias var break case continue default defer do else fallthrough for guard if in repeat return switch where while as catch false is nil super self Self throw throws true try async await some any")
	kwRuby  = words("BEGIN END alias and begin break case class def defined? do else elsif end ensure false for if in module next nil not or redo rescue retry return self super then true undef unless until when while yield require attr_accessor")
	kwShell = words("if then else elif fi case esac for while until do done in function return local export readonly declare set unset shift exit echo source alias")
	kwSQL   = words("select from where and or not insert into values update set delete create table drop alter add index primary key foreign references join left right inner outer on group by order having limit offset as distinct union all null is in like between exists case when then else end view default constraint unique")
	kwYAML  = words("true false null yes no on off")
	kwJSON  = words("true false null")
	kwCSS   = words("important inherit initial none auto")
)

func langFor(p string) *lang {
	base := strings.ToLower(path.Base(p))
	ext := strings.ToLower(path.Ext(p))
	switch base {
	case "makefile", "dockerfile", ".gitignore", ".gitattributes", ".env", ".editorconfig":
		return &lang{line: []string{"#"}, quotes: `"'`, keywords: kwShell}
	}
	switch ext {
	case ".go":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"'`, raw: '`', keywords: kwGo}
	case ".java", ".kt", ".kts", ".scala", ".groovy", ".gradle", ".dart":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"'`, keywords: kwJava}
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".vue", ".svelte":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"'`, raw: '`', keywords: kwJS}
	case ".c", ".h", ".cc", ".cpp", ".hpp", ".cxx", ".m", ".mm", ".cs", ".php":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"'`, keywords: kwC}
	case ".rs":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"`, keywords: kwRust}
	case ".swift":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"`, keywords: kwSwift}
	case ".py", ".pyi":
		return &lang{line: []string{"#"}, quotes: `"'`, keywords: kwPy}
	case ".rb":
		return &lang{line: []string{"#"}, quotes: `"'`, keywords: kwRuby}
	case ".sh", ".bash", ".zsh", ".fish":
		return &lang{line: []string{"#"}, quotes: `"'`, keywords: kwShell}
	case ".yml", ".yaml", ".toml", ".ini", ".cfg", ".conf", ".properties":
		return &lang{line: []string{"#", ";"}, quotes: `"'`, keywords: kwYAML}
	case ".sql":
		return &lang{line: []string{"--"}, blockA: "/*", blockB: "*/", quotes: `'"`, keywords: kwSQL, caseFold: true}
	case ".json", ".jsonc", ".json5":
		return &lang{line: []string{"//"}, quotes: `"`, keywords: kwJSON, json: true}
	case ".css", ".scss", ".less":
		return &lang{line: []string{"//"}, blockA: "/*", blockB: "*/", quotes: `"'`, keywords: kwCSS}
	case ".xml", ".html", ".htm", ".svg", ".plist", ".xhtml", ".iml":
		return &lang{blockA: "<!--", blockB: "-->", quotes: `"'`, markup: true}
	case ".md", ".markdown":
		return &lang{markdown: true}
	}
	return nil
}

// highlight lexes lines (already tab-expanded) into spans per line.
func highlight(p string, lines []string) [][]span {
	l := langFor(p)
	if l == nil || len(lines) > 30000 {
		return nil
	}
	out := make([][]span, len(lines))
	inBlock, inRaw := false, false
	for i, s := range lines {
		if len(s) > 2000 {
			continue
		}
		out[i], inBlock, inRaw = l.lexLine(s, inBlock, inRaw)
	}
	return out
}

func (l *lang) lexLine(s string, inBlock, inRaw bool) ([]span, bool, bool) {
	var out []span
	add := func(a, b int, k tokKind) {
		if b > a {
			out = append(out, span{a, b, k})
		}
	}
	if l.markdown {
		t := strings.TrimLeft(s, " ")
		switch {
		case strings.HasPrefix(t, "#"):
			add(0, len(s), tkKeyword)
		case strings.HasPrefix(t, "```"):
			add(0, len(s), tkMeta)
		case strings.HasPrefix(t, "> "):
			add(0, len(s), tkComment)
		}
		return out, false, false
	}
	i := 0
	if inBlock {
		end := strings.Index(s, l.blockB)
		if end < 0 {
			add(0, len(s), tkComment)
			return out, true, false
		}
		add(0, end+len(l.blockB), tkComment)
		i = end + len(l.blockB)
		inBlock = false
	}
	if inRaw {
		end := strings.IndexByte(s, l.raw)
		if end < 0 {
			add(0, len(s), tkString)
			return out, false, true
		}
		add(0, end+1, tkString)
		i = end + 1
		inRaw = false
	}
	for i < len(s) {
		c := s[i]
		rest := s[i:]
		if l.blockA != "" && strings.HasPrefix(rest, l.blockA) {
			end := strings.Index(s[i+len(l.blockA):], l.blockB)
			if end < 0 {
				add(i, len(s), tkComment)
				return out, true, false
			}
			e := i + len(l.blockA) + end + len(l.blockB)
			add(i, e, tkComment)
			i = e
			continue
		}
		lineComment := false
		for _, lc := range l.line {
			if strings.HasPrefix(rest, lc) && (lc != "#" || i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
				lineComment = true
				break
			}
		}
		if lineComment {
			add(i, len(s), tkComment)
			return out, false, false
		}
		if l.markup && c == '<' {
			j := i + 1
			for j < len(s) && (s[j] == '/' || s[j] == '?' || s[j] == '!') {
				j++
			}
			k := j
			for k < len(s) && (isIdent(s[k]) || s[k] == ':' || s[k] == '-' || s[k] == '.') {
				k++
			}
			add(i, k, tkTag)
			i = k
			continue
		}
		if l.markup && (c == '>' || (c == '/' && i+1 < len(s) && s[i+1] == '>')) {
			n := 1
			if c == '/' {
				n = 2
			}
			add(i, i+n, tkTag)
			i += n
			continue
		}
		if l.raw != 0 && c == l.raw {
			end := strings.IndexByte(s[i+1:], l.raw)
			if end < 0 {
				add(i, len(s), tkString)
				return out, false, true
			}
			add(i, i+end+2, tkString)
			i += end + 2
			continue
		}
		if strings.IndexByte(l.quotes, c) >= 0 {
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(s))
			kind := tkString
			if l.json {
				k := j
				for k < len(s) && s[k] == ' ' {
					k++
				}
				if k < len(s) && s[k] == ':' {
					kind = tkConstant
				}
			}
			add(i, j, kind)
			i = j
			continue
		}
		if c >= '0' && c <= '9' && (i == 0 || !isIdent(s[i-1])) {
			j := i + 1
			for j < len(s) && (isIdent(s[j]) || s[j] == '.') {
				j++
			}
			add(i, j, tkNumber)
			i = j
			continue
		}
		if c == '@' && !l.markup {
			j := i + 1
			for j < len(s) && (isIdent(s[j]) || s[j] == '.') {
				j++
			}
			add(i, j, tkMeta)
			i = j
			continue
		}
		if isIdentStart(c) {
			j := i + 1
			for j < len(s) && isIdent(s[j]) {
				j++
			}
			word := s[i:j]
			key := word
			if l.caseFold {
				key = strings.ToLower(word)
			}
			switch {
			case l.markup:
				k := j
				if k < len(s) && s[k] == '=' {
					add(i, j, tkConstant)
				}
			case l.keywords[key]:
				add(i, j, tkKeyword)
			default:
				k := j
				for k < len(s) && s[k] == ' ' {
					k++
				}
				if k < len(s) && s[k] == '(' && !l.json {
					add(i, j, tkFunction)
				} else if len(word) > 1 && isUpperWord(word) {
					add(i, j, tkConstant)
				}
			}
			i = j
			continue
		}
		if c >= utf8.RuneSelf {
			_, n := utf8.DecodeRuneInString(rest)
			i += n
			continue
		}
		i++
	}
	return out, inBlock, inRaw
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdent(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

func isUpperWord(s string) bool {
	for _, r := range s {
		if !(unicode.IsUpper(r) || r == '_' || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func (w *Window) tokColor(k tokKind) ui.Color {
	p := w.pal
	switch k {
	case tkKeyword:
		return p.SynKeyword
	case tkString:
		return p.SynString
	case tkNumber:
		return p.SynNumber
	case tkComment:
		return p.SynComment
	case tkFunction:
		return p.SynFunction
	case tkMeta:
		return p.SynMeta
	case tkConstant:
		return p.SynConstant
	case tkTag:
		return p.SynTag
	}
	return p.EditorText
}

// runsFor converts spans of an expanded line into grid runs covering it.
func (w *Window) runsFor(s string, spans []span, dst []ui.TextRun, base ui.Color) []ui.TextRun {
	dst = dst[:0]
	col := int32(0)
	emit := func(a, b int, c ui.Color) {
		if b <= a {
			return
		}
		cells := int32(0)
		for _, r := range s[a:b] {
			cells += int32(cellWidthOf(r))
		}
		dst = append(dst, ui.TextRun{Column: col, Bytes: int32(b - a), Cells: cells, Color: c})
		col += cells
	}
	pos := 0
	for _, sp := range spans {
		if sp.start < pos || sp.end > len(s) {
			continue
		}
		emit(pos, sp.start, base)
		c := w.tokColor(sp.kind)
		emit(sp.start, sp.end, c)
		if sp.kind == tkComment {
			dst[len(dst)-1].Italic = true
		}
		pos = sp.end
	}
	emit(pos, len(s), base)
	return dst
}

// cellWidthOf is how many grid cells a rune takes (East Asian wide = 2).
func cellWidthOf(r rune) int {
	if r >= 0x1100 && (r <= 0x115F || r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) || (r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) || (r >= 0xFE30 && r <= 0xFE4F) || (r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) || (r >= 0x1F300 && r <= 0x1F64F) || (r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x20000 && r <= 0x3FFFD)) {
		return 2
	}
	return 1
}

// columnOf maps a byte offset of an original (unexpanded) line to its grid
// column after tab expansion.
func columnOf(s string, b int, tab int) int {
	col := 0
	for i, r := range s {
		if i >= b {
			return col
		}
		if r == '\t' {
			col += tab - col%tab
		} else {
			col += cellWidthOf(r)
		}
	}
	return col
}

// expandLine expands tabs and replaces control characters for display.
func expandLine(s string) string {
	s = expandTabs(s, 4)
	if strings.IndexFunc(s, func(r rune) bool { return r < 0x20 }) >= 0 {
		s = strings.Map(func(r rune) rune {
			if r < 0x20 {
				return '·'
			}
			return r
		}, s)
	}
	return s
}

func displayCells(s string) int {
	n := 0
	for _, r := range s {
		n += cellWidthOf(r)
	}
	return n
}
