package viffui

import (
	"sort"
	"strings"
	"time"

	"github.com/vivaui/viva/text"

	vapp "viff/internal/app"
	"viff/internal/domain"
)

// The Project tool window lists the working tree: tracked files plus untracked files
// that are not ignored. It is a read-only browser, not a second Git layer:
// the list comes from one `ls-files` query owned by app, and every open goes
// through the existing single-version viewer (DiffSource.View).
//
// Like the changes tree, it keeps every path in memory and paints only the
// rows inside the viewport. Directories are folded into the row list on each
// rebuild instead of becoming widgets, so a monorepo does not create a node
// per file.

type filesView struct {
	entries []string
	gen     uint64
	at      time.Time
	loaded  bool
	loading bool
	err     error
	seq     uint64

	filter *text.Editor
	rows   []fileRow
	// rootName and rootHint label the repository row at the top of the
	// Project panel; rootOpen folds the whole tree.
	rootName string
	rootHint string
	rootOpen bool
	sel      string
	scroll   scroller
	// query is the filter text the current rows were built for.
	query string

	// collapsed remembers folded directories by path; absent means expanded.
	collapsed map[string]bool
	// status decorates names in the tree; it follows the status snapshot.
	status map[string]domain.ChangeKind
}

type fileRow struct {
	key   string
	name  string
	hint  string
	path  string
	dir   bool
	root  bool
	depth int
	kind  domain.ChangeKind
	known bool
}

// projectRootKey is the row key of the repository root. It is not a path, so
// no file can collide with it.
const projectRootKey = "\x00project-root"

func (v *filesView) init() {
	v.filter = text.NewEditor("")
	v.collapsed = map[string]bool{}
	v.rootOpen = true
}

// reset drops the list and filter when the repository changes. The sequence
// number invalidates answers for the previous repository that are still in
// flight.
func (v *filesView) reset() {
	v.seq++
	v.entries = nil
	v.rows = v.rows[:0]
	v.gen = 0
	v.at = time.Time{}
	v.loaded = false
	v.loading = false
	v.err = nil
	v.sel = ""
	v.scroll = scroller{}
	v.query = ""
	v.status = nil
	v.rootName, v.rootHint = "", ""
	v.init()
}

// dirNode is one directory of the derived tree. It exists only while rows are
// rebuilt, so its cost is bounded by the number of directories, not files.
type dirNode struct {
	name  string
	path  string
	dirs  map[string]*dirNode
	files []string
}

// rebuild turns the filtered paths into tree rows. With a filter active every
// directory is expanded so matches stay visible; without one, folded
// directories are skipped with their whole subtree.
func (v *filesView) rebuild() {
	q := strings.ToLower(strings.TrimSpace(v.filter.String()))
	v.query = q
	root := &dirNode{dirs: map[string]*dirNode{}}
	for _, p := range v.entries {
		if q != "" && !strings.Contains(strings.ToLower(p), q) {
			continue
		}
		parts := strings.Split(p, "/")
		n := root
		for _, part := range parts[:len(parts)-1] {
			child := n.dirs[part]
			if child == nil {
				child = &dirNode{name: part, dirs: map[string]*dirNode{}}
				if n.path == "" {
					child.path = part
				} else {
					child.path = n.path + "/" + part
				}
				n.dirs[part] = child
			}
			n = child
		}
		n.files = append(n.files, p)
	}
	v.rows = v.rows[:0]
	depth := 0
	if v.rootName != "" {
		v.rows = append(v.rows, fileRow{key: projectRootKey, name: v.rootName, hint: v.rootHint, dir: true, root: true, depth: 0})
		if v.rootOpen || q != "" {
			depth = 1
		} else {
			v.sel = projectRootKey
			return
		}
	}
	v.appendRows(root, depth, q == "")
	if v.sel != "" && v.rowIndex(v.sel) < 0 {
		v.sel = projectRootKey
	}
}

func (v *filesView) appendRows(n *dirNode, depth int, respectFolds bool) {
	dirs := make([]*dirNode, 0, len(n.dirs))
	for _, d := range n.dirs {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	for _, d := range dirs {
		v.rows = append(v.rows, fileRow{key: d.path + "/", name: d.name, path: d.path, dir: true, depth: depth})
		if respectFolds && v.collapsed[d.path] {
			continue
		}
		v.appendRows(d, depth+1, respectFolds)
	}
	// entries is already sorted, so a directory's files stay in order.
	for _, p := range n.files {
		kind, known := v.status[p]
		v.rows = append(v.rows, fileRow{key: p, name: domain.FileName(p), path: p, depth: depth, kind: kind, known: known})
	}
}

// rowIndex finds a row by key, or -1.
func (v *filesView) rowIndex(key string) int {
	for i, row := range v.rows {
		if row.key == key {
			return i
		}
	}
	return -1
}

// fileStatuses maps a path to what the status snapshot says about it. A path
// that is both staged and modified keeps the working-tree kind, which is the
// one the next commit will not necessarily contain.
func fileStatuses(rs *vapp.RepoState) map[string]domain.ChangeKind {
	if rs == nil {
		return nil
	}
	m := make(map[string]domain.ChangeKind, len(rs.Status.Entries))
	for _, e := range rs.Status.Entries {
		if e.Ignored() {
			continue
		}
		var k domain.ChangeKind
		switch {
		case e.Conflicted():
			k = domain.ChangeConflicted
		case e.Untracked():
			k = domain.ChangeUntracked
		case e.Staged() && !e.Unstaged():
			k = e.KindIn(domain.AreaStaged)
		default:
			k = e.KindIn(domain.AreaUnstaged)
		}
		m[e.Path] = k
	}
	return m
}
