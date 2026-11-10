package viffui

import (
	"slices"
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
// When the workspace holds several repositories, a workspace row groups them
// and every repository is listed under it. Only the active repository shows a
// file tree: the other rows are navigation summaries, and choosing one switches
// the active repository (ADR 0046). A single-repository workspace keeps the
// original row with the repository as the root.
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
	// rootName labels the active repository row; rootOpen folds the file tree
	// under it.
	rootName string
	rootOpen bool
	// repoRoots is the workspace list in display order, and activeRoot marks
	// the row whose file tree is shown. The rows are rebuilt only when this
	// list changes, not on every paint.
	repoRoots  []string
	activeRoot string
	// workspaceOpen folds the workspace row in a multi-repository workspace.
	workspaceOpen bool
	sel           string
	scroll        scroller
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
	path  string
	dir   bool
	root  bool
	depth int
	kind  domain.ChangeKind
	known bool
	// repo marks a repository row; repoRoot is its workspace root. The active
	// repository is both root and repo and owns the file rows below it.
	repo     bool
	repoRoot string
	// workspace marks the row that groups every repository.
	workspace bool
}

// projectRootKey is the row key of the active repository. It is not a path, so
// no file can collide with it.
const projectRootKey = "\x00project-root"

// projectWorkspaceKey is the row key of the workspace row shown above several
// repositories.
const projectWorkspaceKey = "\x00project-workspace"

// repoRowKey is the row key of a repository that is not active. The NUL prefix
// keeps it apart from every file path and from the two fixed keys above.
func repoRowKey(root string) string { return "\x00project-repo:" + root }

func (v *filesView) init() {
	v.filter = text.NewEditor("")
	v.collapsed = map[string]bool{}
	v.rootOpen = true
	v.workspaceOpen = true
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
	v.rootName = ""
	v.repoRoots = nil
	v.activeRoot = ""
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

// syncRepositories records the workspace list the rows are built from. It
// rebuilds only when the roots or the active one changed, so painting the
// panel does not rebuild a large tree on every frame. Status text is not part
// of the key: it is read when the row is painted, and the active repository
// reloads the list when its status changes (ensureFiles).
func (v *filesView) syncRepositories(roots []string, active string) bool {
	if slices.Equal(v.repoRoots, roots) && v.activeRoot == active {
		return false
	}
	v.repoRoots = append(v.repoRoots[:0], roots...)
	v.activeRoot = active
	v.rebuild()
	return true
}

// grouped reports that the workspace row is shown. With a single repository
// the repository itself stays the root, which is the layout Viff had before
// multi-repository projects (ADR 0046).
func (v *filesView) grouped() bool { return len(v.repoRoots) > 1 }

// activeRepoName is the active row's label: the repository name from the
// loaded state when available, otherwise derived from the root.
func (v *filesView) activeRepoName() string {
	if v.rootName != "" {
		return v.rootName
	}
	if v.activeRoot != "" {
		return repoRootName(v.activeRoot)
	}
	return ""
}

// expandAll opens the workspace, the active repository and every directory.
func (v *filesView) expandAll() {
	clear(v.collapsed)
	v.rootOpen = true
	v.workspaceOpen = true
	v.rebuild()
}

// collapseAll folds the directories of the active file tree. The workspace and
// repository rows stay open so their summaries remain visible.
func (v *filesView) collapseAll() {
	for _, row := range v.rows {
		if row.dir && !row.root && !row.repo && !row.workspace && row.path != "" {
			v.collapsed[row.path] = true
		}
	}
	v.rebuild()
}

// hasTreeContent reports that at least one directory or file row is visible,
// so a filter with no match can say so even though the workspace and
// repository rows are always present.
func (v *filesView) hasTreeContent() bool {
	for _, row := range v.rows {
		if !row.repo && !row.workspace {
			return true
		}
	}
	return false
}

// ensureSelection keeps the selection on a row that still exists. Folding the
// workspace can hide the active repository, so the fallback walks outward from
// the active row to the workspace row.
func (v *filesView) ensureSelection() {
	if v.sel != "" && v.rowIndex(v.sel) >= 0 {
		return
	}
	switch {
	case v.rowIndex(projectRootKey) >= 0:
		v.sel = projectRootKey
	case v.rowIndex(projectWorkspaceKey) >= 0:
		v.sel = projectWorkspaceKey
	case len(v.rows) > 0:
		v.sel = v.rows[0].key
	default:
		v.sel = ""
	}
}

// rebuild turns the filtered paths into tree rows. A changed filter opens the
// whole tree once, so its matches are visible instead of staying hidden inside
// a folded directory; folds the user makes after that are kept, and folded
// directories are skipped with their whole subtree.
func (v *filesView) rebuild() {
	q := strings.ToLower(strings.TrimSpace(v.filter.String()))
	if q != v.query {
		clear(v.collapsed)
		v.rootOpen = true
	}
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
	grouped := v.grouped()
	if grouped {
		v.rows = append(v.rows, fileRow{key: projectWorkspaceKey, dir: true, workspace: true})
		if !v.workspaceOpen {
			v.ensureSelection()
			return
		}
	}
	repoDepth := 0
	if grouped {
		repoDepth = 1
	}
	for _, repoRoot := range v.repoRoots {
		active := repoRoot == v.activeRoot
		name := repoRootName(repoRoot)
		row := fileRow{key: repoRowKey(repoRoot), name: name, dir: true, repo: true, repoRoot: repoRoot, depth: repoDepth}
		if active {
			row.key = projectRootKey
			row.root = true
			row.name = v.activeRepoName()
			if row.name == "" {
				row.name = name
			}
		}
		v.rows = append(v.rows, row)
		if active && v.rootOpen {
			v.appendRows(root, repoDepth+1)
		}
	}
	if len(v.repoRoots) == 0 {
		// The list is filled on the first paint; this keeps a workspace built
		// without it (older tests, a literal) showing its repository row.
		if name := v.activeRepoName(); name != "" {
			v.rows = append(v.rows, fileRow{key: projectRootKey, name: name, dir: true, root: true, repo: true, repoRoot: v.activeRoot, depth: 0})
			if v.rootOpen {
				v.appendRows(root, 1)
			}
		}
	}
	v.ensureSelection()
}

func (v *filesView) appendRows(n *dirNode, depth int) {
	dirs := make([]*dirNode, 0, len(n.dirs))
	for _, d := range n.dirs {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	for _, d := range dirs {
		v.rows = append(v.rows, fileRow{key: d.path + "/", name: d.name, path: d.path, dir: true, depth: depth})
		if v.collapsed[d.path] {
			continue
		}
		v.appendRows(d, depth+1)
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
