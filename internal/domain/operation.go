package domain

// OperationKind is a multi-step Git operation that is in progress.
type OperationKind uint8

const (
	OpNone OperationKind = iota
	OpMerge
	OpRebase
	OpCherryPick
	OpRevert
	OpApplyMailbox
	OpBisect
)

// Title is the user-facing name of the operation.
func (k OperationKind) Title() string {
	switch k {
	case OpMerge:
		return "Merge"
	case OpRebase:
		return "Rebase"
	case OpCherryPick:
		return "Cherry-pick"
	case OpRevert:
		return "Revert"
	case OpApplyMailbox:
		return "Apply patches"
	case OpBisect:
		return "Bisect"
	}
	return ""
}

// Operation describes the in-progress operation, rebuilt from the Git
// directory each time: nothing about it is remembered across restarts.
type Operation struct {
	Kind OperationKind
	// HeadName is the branch being rebased (rebase) or HEAD's branch.
	HeadName string
	// Onto is the commit a rebase replays onto.
	Onto string
	// Incoming is the commit being merged, picked, reverted or replayed.
	Incoming string
	// IncomingSubject is the first line of Incoming's message when known.
	IncomingSubject string
	// MergeMessage is the prepared message from MERGE_MSG.
	MergeMessage string
	Step, Total  int
	Interactive  bool
	// OntoName and IncomingName are branch or tag names pointing at Onto and
	// Incoming, filled in from the refs by the application when known.
	OntoName     string
	IncomingName string
}

// Active reports an operation in progress.
func (o Operation) Active() bool { return o.Kind != OpNone }

// CanContinue reports whether the operation has a --continue.
func (o Operation) CanContinue() bool {
	switch o.Kind {
	case OpMerge, OpRebase, OpCherryPick, OpRevert, OpApplyMailbox:
		return true
	}
	return false
}

// CanSkip reports whether the operation has a --skip.
func (o Operation) CanSkip() bool {
	switch o.Kind {
	case OpRebase, OpCherryPick, OpRevert, OpApplyMailbox:
		return true
	}
	return false
}

// CanAbort reports whether the operation has an --abort (or bisect reset).
func (o Operation) CanAbort() bool { return o.Kind != OpNone }

// SideLabels names the two sides of a conflict as Git defines them for the
// operation. In a rebase "ours" is the commit being built on (the upstream)
// and "theirs" is the commit being replayed, so the labels never claim a
// side is "yours".
func (o Operation) SideLabels(currentBranch string) (ours, theirs string) {
	short := func(h string) string { return ShortHash(h) }
	switch o.Kind {
	case OpRebase:
		ours = "Rebasing onto " + short(o.Onto)
		if o.OntoName != "" {
			ours = o.OntoName + " (" + short(o.Onto) + ")"
		}
		if o.Onto == "" {
			ours = "Upstream (being rebased onto)"
		}
		theirs = "Commit being replayed"
		if o.Incoming != "" {
			theirs = short(o.Incoming)
			if o.IncomingSubject != "" {
				theirs += " " + o.IncomingSubject
			}
		}
		return
	case OpRevert:
		ours = headLabel(currentBranch)
		theirs = "Revert of " + short(o.Incoming)
		return
	}
	ours = headLabel(currentBranch)
	theirs = short(o.Incoming)
	if o.IncomingName != "" && o.Kind == OpMerge {
		theirs = o.IncomingName + " (" + theirs + ")"
	}
	if o.IncomingSubject != "" {
		theirs += " " + o.IncomingSubject
	}
	if theirs == "" {
		theirs = "Incoming"
	}
	return
}

func headLabel(branch string) string {
	if branch == "" {
		return "HEAD (detached)"
	}
	return branch + " (HEAD)"
}

// ConflictKind classifies an unmerged path by its XY status.
type ConflictKind uint8

const (
	ConflictBothModified ConflictKind = iota
	ConflictBothAdded
	ConflictBothDeleted
	ConflictAddedByUs
	ConflictAddedByThem
	ConflictDeletedByUs
	ConflictDeletedByThem
)

// ConflictKindOf maps a porcelain XY code to a conflict kind.
func ConflictKindOf(x, y byte) ConflictKind {
	switch string([]byte{x, y}) {
	case "AA":
		return ConflictBothAdded
	case "DD":
		return ConflictBothDeleted
	case "AU":
		return ConflictAddedByUs
	case "UA":
		return ConflictAddedByThem
	case "DU":
		return ConflictDeletedByUs
	case "UD":
		return ConflictDeletedByThem
	}
	return ConflictBothModified
}

// Description is the short explanation shown next to a conflicted file.
func (k ConflictKind) Description() string {
	switch k {
	case ConflictBothAdded:
		return "Both added"
	case ConflictBothDeleted:
		return "Both deleted"
	case ConflictAddedByUs:
		return "Added by ours"
	case ConflictAddedByThem:
		return "Added by theirs"
	case ConflictDeletedByUs:
		return "Deleted by ours, modified by theirs"
	case ConflictDeletedByThem:
		return "Modified by ours, deleted by theirs"
	}
	return "Both modified"
}

// Textual reports a conflict whose working file may hold conflict markers.
func (k ConflictKind) Textual() bool {
	return k == ConflictBothModified || k == ConflictBothAdded
}

// Stage is one index stage of an unmerged path (1 base, 2 ours, 3 theirs).
type Stage struct {
	Number int
	Mode   string
	Hash   string
}

// Conflict is one unmerged path with the stages Git recorded for it.
type Conflict struct {
	Path   string
	Kind   ConflictKind
	Stages []Stage
}

// Stage returns the stage with number n, if present.
func (c Conflict) Stage(n int) (Stage, bool) {
	for _, s := range c.Stages {
		if s.Number == n {
			return s, true
		}
	}
	return Stage{}, false
}

// Binary file modes are detected elsewhere; Symlink reports a symlink stage.
func (s Stage) Symlink() bool { return s.Mode == "120000" }
