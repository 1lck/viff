package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"viff/internal/domain"
	"viff/internal/gitcli"
)

// Op is a write command's body. progress reports Git's progress lines.
type Op func(ctx context.Context, repo domain.Repo, progress func(string)) error

// RunOptions tunes how a command reports.
type RunOptions struct {
	// Success is the notification title on success; "" shows none.
	Success string
	// SuccessBody is its body.
	SuccessBody string
	// OnError replaces the default error notification. It runs on the UI
	// thread and returns true when it handled the error.
	OnError func(error) bool
	// After runs on the UI thread after success.
	After func()
	// Always runs on the UI thread after the command, success or not.
	Always func()
	// Read marks a command that does not need the write lock.
	Read bool
}

// Run executes a write command as a task. Writes to one repository run one at
// a time; after any write, finished or failed or cancelled, the state is
// queried again from Git, because a cancelled write may have done part of
// its work.
func (w *Workspace) Run(title string, op Op, o RunOptions) {
	rs := w.Repo
	if rs == nil {
		return
	}
	task, ctx, finish := w.startTask(title, !o.Read)
	repo := rs.Repo
	go func() {
		var lock interface {
			Lock()
			Unlock()
		}
		if !o.Read {
			lock = writeLock(repo)
			lock.Lock()
		}
		err := op(ctx, repo, func(line string) { w.setProgress(task, line) })
		if lock != nil {
			lock.Unlock()
		}
		w.post(func() {
			finish()
			if w.Repo == rs {
				w.Refresh(true)
			}
			if err != nil {
				if o.OnError == nil || !o.OnError(err) {
					w.NotifyError(title+" failed", err)
				}
			} else {
				if o.Success != "" {
					w.Notify(NoticeSuccess, o.Success, o.SuccessBody)
				}
				if o.After != nil {
					o.After()
				}
			}
			if o.Always != nil {
				o.Always()
			}
		})
	}()
}

// StagePaths stages whole files.
func (w *Workspace) StagePaths(paths []string) {
	if len(paths) == 0 {
		return
	}
	w.Run("Stage", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.Stage(ctx, repo, paths)
	}, RunOptions{})
}

// UnstagePaths unstages whole files.
func (w *Workspace) UnstagePaths(paths []string) {
	if len(paths) == 0 || w.Repo == nil {
		return
	}
	hasHead := w.Repo.HasHead()
	w.Run("Unstage", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.Unstage(ctx, repo, paths, hasHead)
	}, RunOptions{})
}

// Rollback discards local changes of tracked files (from HEAD for staged
// ones) and deletes the given untracked files. The UI confirms first.
func (w *Workspace) Rollback(tracked, untracked []string, fromHead bool) {
	w.Run("Rollback", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		if len(tracked) > 0 {
			var err error
			if fromHead {
				err = w.Git.Revert(ctx, repo, tracked)
			} else {
				err = w.Git.Discard(ctx, repo, tracked)
			}
			if err != nil {
				return err
			}
		}
		for _, p := range untracked {
			full := filepath.Join(repo.Root, filepath.FromSlash(p))
			if !strings.HasPrefix(full, repo.Root+string(filepath.Separator)) {
				return fmt.Errorf("refusing to delete %s outside the repository", p)
			}
			if err := os.RemoveAll(full); err != nil {
				return err
			}
		}
		return nil
	}, RunOptions{})
}

// BlockAction is what to do with one changed block of a working diff.
type BlockAction uint8

const (
	BlockStage BlockAction = iota
	BlockUnstage
	BlockRevert // discard from the working tree
)

// ApplyBlock stages, unstages or reverts lines of a working-tree diff. sel
// selects the old-side and new-side lines of the block to apply (nil means
// the whole block). Before applying, both sides are read again: if either
// changed since the diff was shown, nothing is applied and ErrStale is
// returned through done, so an external tool's newer edit is never
// overwritten with an old patch.
func (w *Workspace) ApplyBlock(d *DiffResult, block int, action BlockAction, selOld, selNew func(int) bool, done func(error)) {
	if d == nil || !d.Source.Working || block < 0 || block >= len(d.Blocks) {
		return
	}
	b := d.Blocks[block]
	if selOld == nil {
		selOld = domain.SelectRange(b.OldStart, b.OldEnd)
	}
	if selNew == nil {
		selNew = domain.SelectRange(b.NewStart, b.NewEnd)
	}
	src := d.Source
	oldWant, newWant := d.Old.Content, d.New.Content
	oldSide := domain.PatchSide{Lines: d.Old.Lines, NoFinalNewline: d.Old.NoEOL}
	newSide := domain.PatchSide{Lines: d.New.Lines, NoFinalNewline: d.New.NoEOL}
	blocks := d.Blocks
	title := map[BlockAction]string{BlockStage: "Stage lines", BlockUnstage: "Unstage lines", BlockRevert: "Revert lines"}[action]
	w.Run(title, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		oldRev, newRev, _, _ := revisions(src)
		oldNow, err := w.Git.Read(ctx, repo, oldRev, src.Path)
		if err != nil {
			return err
		}
		newNow, err := w.Git.Read(ctx, repo, newRev, src.Path)
		if err != nil {
			return err
		}
		if !bytes.Equal(oldNow.Data, oldWant.Data) || !bytes.Equal(newNow.Data, newWant.Data) || oldNow.Exists != oldWant.Exists || newNow.Exists != newWant.Exists {
			return ErrStale
		}
		var patch []byte
		cached := true
		switch {
		case action == BlockStage && src.Area == domain.AreaUnstaged:
			// index → worktree, applied forward to the index.
			patch = domain.BuildPatch(src.Path, oldSide, newSide, blocks, selOld, selNew)
		case action == BlockUnstage && src.Area == domain.AreaStaged:
			// HEAD → index: build index → HEAD for the selection and apply it
			// to the index.
			patch = domain.BuildPatch(src.Path, newSide, oldSide, swapBlocks(blocks), selNew, selOld)
		case action == BlockRevert && src.Area == domain.AreaUnstaged:
			// worktree → index for the selection, applied to the worktree.
			patch = domain.BuildPatch(src.Path, newSide, oldSide, swapBlocks(blocks), selNew, selOld)
			cached = false
		default:
			return fmt.Errorf("this action does not apply here")
		}
		if patch == nil {
			return nil
		}
		return w.Git.ApplyPatch(ctx, repo, patch, cached, false)
	}, RunOptions{OnError: func(err error) bool {
		if done != nil {
			done(err)
		}
		if err == ErrStale {
			w.Notify(NoticeWarning, "File changed", ErrStale.Error())
			return true
		}
		return false
	}, After: func() {
		if done != nil {
			done(nil)
		}
	}})
}

func swapBlocks(blocks []domain.Block) []domain.Block {
	out := make([]domain.Block, len(blocks))
	for i, b := range blocks {
		out[i] = domain.Block{OldStart: b.NewStart, OldEnd: b.NewEnd, NewStart: b.OldStart, NewEnd: b.OldEnd}
	}
	return out
}

// CommitRequest is the commit tool window's input.
type CommitRequest struct {
	Message  string
	Amend    bool
	SignOff  bool
	NoVerify bool
	// Push pushes after a successful commit.
	Push bool
}

// Commit records the staged changes. Hooks run with the user's environment;
// a failing hook fails the commit and its output is shown.
func (w *Workspace) Commit(req CommitRequest, after func()) {
	staged, _, _, _ := w.Repo.Status.Counts()
	subject := strings.SplitN(strings.TrimSpace(req.Message), "\n", 2)[0]
	body := fmt.Sprintf("%d file%s committed: %s", staged, plural(staged), subject)
	if req.Amend {
		body = "Amended: " + subject
	}
	w.Run("Commit", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.Commit(ctx, repo, gitcli.CommitOptions{Message: req.Message, Amend: req.Amend, SignOff: req.SignOff, NoVerify: req.NoVerify})
		return err
	}, RunOptions{Success: "Committed", SuccessBody: body, After: after})
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// LastMessage loads HEAD's message for amend.
func (w *Workspace) LastMessage(done func(string)) {
	rs := w.Repo
	if rs == nil || !rs.HasHead() {
		return
	}
	repo := rs.Repo
	go func() {
		msg, err := w.Git.LastCommitMessage(context.Background(), repo)
		w.post(func() {
			if err == nil && w.Repo == rs {
				done(msg)
			}
		})
	}()
}

// Checkout switches to a ref.
func (w *Workspace) Checkout(ref domain.Ref) {
	w.Run("Checkout "+ref.Short, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.Checkout(ctx, repo, ref)
	}, RunOptions{OnError: func(err error) bool {
		switch gitcli.KindOf(err) {
		case gitcli.ErrLocalChangesOverwrite, gitcli.ErrUntrackedOverwrite:
			w.Notify(NoticeError, "Checkout failed", ErrorText(err)+"\n\nCommit, stash or roll back your changes first.")
			return true
		}
		return false
	}, Success: "", After: func() {
		w.Notify(NoticeInfo, "Checked out "+ref.Short, "")
	}})
}

// CheckoutRevision detaches HEAD.
func (w *Workspace) CheckoutRevision(rev string) {
	w.Run("Checkout "+domain.ShortHash(rev), func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.CheckoutRevision(ctx, repo, rev)
	}, RunOptions{Success: "Checked out " + domain.ShortHash(rev), SuccessBody: "HEAD is detached. Create a branch to keep new commits."})
}

// NewBranch creates a branch.
func (w *Workspace) NewBranch(name, start string, checkout, overwrite bool) {
	w.Run("Create branch "+name, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.CreateBranch(ctx, repo, name, start, checkout, overwrite)
	}, RunOptions{Success: "Created branch " + name})
}

// RenameBranch renames a local branch.
func (w *Workspace) RenameBranch(old, name string) {
	w.Run("Rename branch", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.RenameBranch(ctx, repo, old, name)
	}, RunOptions{Success: "Renamed " + old + " to " + name})
}

// DeleteBranch deletes a branch. An unmerged branch is not deleted; the
// notification offers to force it, naming the consequence.
func (w *Workspace) DeleteBranch(ref domain.Ref, force bool) {
	if ref.Kind == domain.RefRemote {
		w.Run("Delete "+ref.Short, func(ctx context.Context, repo domain.Repo, progress func(string)) error {
			return w.Git.DeleteRemoteBranch(ctx, repo, ref.Remote(), ref.BranchName(), progress)
		}, RunOptions{Success: "Deleted remote branch " + ref.Short})
		return
	}
	target := ref.Target
	w.Run("Delete "+ref.Short, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.DeleteBranch(ctx, repo, ref.Short, force)
	}, RunOptions{
		OnError: func(err error) bool {
			if gitcli.KindOf(err) == gitcli.ErrNotMerged {
				w.Notify(NoticeWarning, "Branch "+ref.Short+" is not fully merged",
					"Its commits are not on the current branch. Deleting it anyway leaves them reachable only through the reflog.",
					NoticeAction{Title: "Delete anyway", Run: func() { w.DeleteBranch(ref, true) }})
				return true
			}
			return false
		},
		After: func() {
			w.Notify(NoticeSuccess, "Deleted branch "+ref.Short, "It pointed at "+domain.ShortHash(target)+".",
				NoticeAction{Title: "Restore", Run: func() { w.NewBranch(ref.Short, target, false, false) }})
		},
	})
}

// CreateTag creates a tag.
func (w *Workspace) CreateTag(name, rev, message string) {
	w.Run("Create tag "+name, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.CreateTag(ctx, repo, name, rev, message)
	}, RunOptions{Success: "Created tag " + name})
}

// DeleteTag deletes a local tag.
func (w *Workspace) DeleteTag(name string) {
	w.Run("Delete tag "+name, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.DeleteTag(ctx, repo, name)
	}, RunOptions{Success: "Deleted tag " + name})
}

// PushTag pushes a tag.
func (w *Workspace) PushTag(remote, name string) {
	w.Run("Push tag "+name, func(ctx context.Context, repo domain.Repo, p func(string)) error {
		return w.Git.PushTag(ctx, repo, remote, name, p)
	}, RunOptions{Success: "Pushed tag " + name + " to " + remote})
}

// SetUpstream sets a branch's upstream.
func (w *Workspace) SetUpstream(branch, upstream string) {
	w.Run("Set upstream", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.SetUpstream(ctx, repo, branch, upstream)
	}, RunOptions{Success: "Tracking " + upstream})
}

// Fetch fetches all remotes with prune.
func (w *Workspace) Fetch(quiet bool) {
	if w.Repo == nil || len(w.Repo.Remotes) == 0 {
		if !quiet {
			w.Notify(NoticeInfo, "Nothing to fetch", "This repository has no remotes.")
		}
		return
	}
	o := RunOptions{}
	if !quiet {
		o.Success = "Fetch successful"
	} else {
		o.OnError = func(error) bool { return true } // background fetch stays silent
	}
	w.Run("Fetching", func(ctx context.Context, repo domain.Repo, p func(string)) error {
		return w.Git.Fetch(ctx, repo, "", w.Settings == nil || w.Settings.FetchPrune, p)
	}, o)
}

// Pull updates the current branch from its upstream.
func (w *Workspace) Pull(mode gitcli.PullMode) {
	rs := w.Repo
	if rs == nil {
		return
	}
	if rs.Status.Branch.Upstream == "" {
		w.Notify(NoticeWarning, "Cannot update", "The current branch has no upstream branch. Push it with an upstream or set one in the branches popup.")
		return
	}
	before := rs.Status.Branch.Head
	w.Run("Updating", func(ctx context.Context, repo domain.Repo, p func(string)) error {
		_, err := w.Git.Pull(ctx, repo, mode, p)
		return err
	}, RunOptions{
		OnError: func(err error) bool {
			if gitcli.KindOf(err) == gitcli.ErrConflict {
				w.Notify(NoticeWarning, "Update has conflicts", "Resolve them with your tool; Viff shows the conflicts and follows the repository state.")
				return true
			}
			return false
		},
		After: func() {
			if w.Repo == nil {
				return
			}
			if w.Repo.Status.Branch.Head == before {
				w.Notify(NoticeSuccess, "All files are up-to-date", "")
			} else {
				w.Notify(NoticeSuccess, "Project updated", "Updated to "+domain.ShortHash(w.Repo.Status.Branch.Head))
			}
		},
	})
}

// Push pushes a branch.
func (w *Workspace) Push(o gitcli.PushOptions, after func()) {
	title := "Pushing"
	w.Run(title, func(ctx context.Context, repo domain.Repo, p func(string)) error {
		_, err := w.Git.Push(ctx, repo, o, p)
		return err
	}, RunOptions{
		OnError: func(err error) bool {
			if gitcli.KindOf(err) == gitcli.ErrRejected {
				msg := "The remote has commits you do not have. Update first, then push again."
				if o.ForceWithLease {
					msg = "The remote branch changed since your last fetch, so the force push was refused to protect that work. Fetch and review before trying again."
				}
				w.Notify(NoticeError, "Push rejected", msg+"\n\n"+ErrorText(err),
					NoticeAction{Title: "Update", Run: func() { w.Pull(gitcli.PullMerge) }})
				return true
			}
			return false
		},
		After: func() {
			dst := o.RemoteName
			if dst == "" {
				dst = o.Branch
			}
			w.Notify(NoticeSuccess, "Pushed "+o.Branch+" to "+o.Remote+"/"+dst, "")
			if after != nil {
				after()
			}
		},
	})
}

// Outgoing lists the commits a push would send.
func (w *Workspace) Outgoing(branch, upstream string, done func([]domain.Commit, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		filter := gitcli.LogFilter{Revisions: []string{"refs/heads/" + branch}}
		if upstream != "" {
			filter.Revisions = append(filter.Revisions, "^refs/remotes/"+upstream)
		} else {
			// A new branch: what is not on any remote branch yet.
			filter.Revisions = append(filter.Revisions, "--not", "--remotes")
		}
		var commits []domain.Commit
		lr, err := w.Git.OpenLog(ctx, repo, filter)
		if err == nil {
			commits, _, err = lr.Next(500)
			lr.Close()
		}
		w.post(func() {
			if w.Repo == rs {
				done(commits, err)
			}
		})
	}()
}

// operationResult explains an operation that stopped with conflicts.
func (w *Workspace) operationError(title string) func(error) bool {
	return func(err error) bool {
		if gitcli.KindOf(err) == gitcli.ErrConflict {
			w.Notify(NoticeWarning, title+" stopped with conflicts",
				"Resolve the conflicts with your external tool. Viff shows them read-only, refreshes when files change, and offers Continue or Abort once Git is ready.")
			return true
		}
		return false
	}
}

// Merge merges rev into the current branch.
func (w *Workspace) Merge(rev string, noFF bool) {
	w.Run("Merge "+rev, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.Merge(ctx, repo, rev, noFF, false)
		return err
	}, RunOptions{OnError: w.operationError("Merge"), Success: "Merged " + rev})
}

// Rebase rebases the current branch onto rev.
func (w *Workspace) Rebase(onto string) {
	w.Run("Rebase onto "+onto, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.Rebase(ctx, repo, onto)
		return err
	}, RunOptions{OnError: w.operationError("Rebase"), Success: "Rebased onto " + onto})
}

// CherryPick applies commits onto HEAD.
func (w *Workspace) CherryPick(hashes []string) {
	w.Run("Cherry-pick", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.CherryPick(ctx, repo, hashes)
		return err
	}, RunOptions{OnError: w.operationError("Cherry-pick"), Success: fmt.Sprintf("Cherry-picked %d commit%s", len(hashes), plural(len(hashes)))})
}

// RevertCommits reverts commits with new commits.
func (w *Workspace) RevertCommits(hashes []string) {
	w.Run("Revert commits", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.RevertCommits(ctx, repo, hashes)
		return err
	}, RunOptions{OnError: w.operationError("Revert"), Success: fmt.Sprintf("Reverted %d commit%s", len(hashes), plural(len(hashes)))})
}

// Reset moves the current branch.
func (w *Workspace) Reset(mode gitcli.ResetMode, rev string) {
	before := ""
	if w.Repo != nil {
		before = w.Repo.Status.Branch.Head
	}
	w.Run("Reset", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.Reset(ctx, repo, mode, rev)
	}, RunOptions{After: func() {
		w.Notify(NoticeSuccess, "Reset to "+domain.ShortHash(rev), "The branch pointed at "+domain.ShortHash(before)+" before; it stays in the reflog.",
			NoticeAction{Title: "Create branch at old position", Run: func() {
				w.NewBranch("backup-"+domain.ShortHash(before), before, false, false)
			}})
	}})
}

// OperationAction continues, skips or aborts the in-progress operation.
func (w *Workspace) OperationAction(action string) {
	rs := w.Repo
	if rs == nil || !rs.Operation.Active() {
		return
	}
	kind := rs.Operation.Kind
	title := strings.ToUpper(action[:1]) + action[1:] + " " + strings.ToLower(kind.Title())
	w.Run(title, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.OperationCommand(ctx, repo, kind, action)
		return err
	}, RunOptions{OnError: w.operationError(kind.Title())})
}

// StashPush stashes changes.
func (w *Workspace) StashPush(message string, untracked, keepIndex bool) {
	w.Run("Stash", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.StashPush(ctx, repo, message, untracked, keepIndex)
	}, RunOptions{Success: "Changes stashed"})
}

// StashApply applies or pops a stash.
func (w *Workspace) StashApply(s domain.Stash, pop, index bool) {
	verb := "Apply"
	if pop {
		verb = "Pop"
	}
	w.Run(verb+" "+s.Ref, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.StashApply(ctx, repo, s.Ref, pop, index)
		return err
	}, RunOptions{OnError: w.operationError("Unstash"), Success: verb + " " + s.Ref + " succeeded"})
}

// StashDrop drops a stash.
func (w *Workspace) StashDrop(s domain.Stash) {
	hash := s.Hash
	w.Run("Drop "+s.Ref, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.StashDrop(ctx, repo, s.Ref)
	}, RunOptions{Success: "Dropped " + s.Ref, SuccessBody: "It was commit " + hash + "; git stash store can restore it while the object exists."})
}

// StashBranch creates a branch from a stash.
func (w *Workspace) StashBranch(s domain.Stash, branch string) {
	w.Run("Branch from stash", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.StashBranch(ctx, repo, branch, s.Ref)
	}, RunOptions{Success: "Created branch " + branch + " from " + s.Ref})
}

// AddRemote adds a remote.
func (w *Workspace) AddRemote(name, url string) {
	w.Run("Add remote", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.AddRemote(ctx, repo, name, url)
	}, RunOptions{Success: "Added remote " + name})
}

// RemoveRemote removes a remote.
func (w *Workspace) RemoveRemote(name string) {
	w.Run("Remove remote", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.RemoveRemote(ctx, repo, name)
	}, RunOptions{Success: "Removed remote " + name})
}

// SetRemoteURL edits a remote.
func (w *Workspace) SetRemoteURL(name, url string) {
	w.Run("Edit remote", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.SetRemoteURL(ctx, repo, name, url)
	}, RunOptions{Success: "Updated remote " + name})
}

// Reflog loads HEAD's reflog.
func (w *Workspace) Reflog(done func([]domain.ReflogEntry, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		list, err := w.Git.Reflog(context.Background(), repo, 300)
		w.post(func() {
			if w.Repo == rs {
				done(list, err)
			}
		})
	}()
}

// Blame annotates a file at rev ("" for the working tree) in the background.
func (w *Workspace) Blame(rev, path string, done func(*domain.Blame, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		b, err := w.Git.Blame(ctx, repo, rev, path)
		w.post(func() {
			if w.Repo == rs {
				done(b, err)
			}
		})
	}()
}

// CreatePatch builds a patch for paths (all changes when empty) in the
// background. staged takes the index against HEAD; otherwise the working tree
// against the index. Paths are relative to the repository root.
func (w *Workspace) CreatePatch(paths []string, staged bool, done func([]byte, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		patch, err := w.Git.CreatePatch(ctx, repo, paths, staged)
		w.post(func() {
			if w.Repo == rs {
				done(patch, err)
			}
		})
	}()
}

// ApplyPatchFile applies a saved patch to the working tree or, with cached,
// to the index alone. Git checks the whole patch first, so a patch that does
// not apply leaves the repository untouched; the failure keeps Git's message.
func (w *Workspace) ApplyPatchFile(path string, cached bool, done func(error)) {
	body := filepath.Base(path)
	if cached {
		body += ", applied to the index only"
	}
	w.Run("Apply Patch", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		patch, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read patch: %w", err)
		}
		return w.Git.ApplyPatch(ctx, repo, patch, cached, false)
	}, RunOptions{
		Success:     "Patch applied",
		SuccessBody: body,
		OnError: func(err error) bool {
			w.Notify(NoticeError, "Cannot apply patch", ErrorText(err))
			if done != nil {
				done(err)
			}
			return true
		},
		After: func() {
			if done != nil {
				done(nil)
			}
		},
	})
}

// Worktrees loads the worktree list and remembers it in the repository state.
func (w *Workspace) Worktrees(done func([]domain.Worktree, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		list, err := w.Git.Worktrees(context.Background(), repo)
		w.post(func() {
			if w.Repo != rs {
				return
			}
			if err == nil {
				rs.Worktrees, rs.WorktreesLoaded = list, true
			}
			if done != nil {
				done(list, err)
			}
		})
	}()
}

// AddWorktree creates a worktree at path with a new branch, starting at
// start ("" means HEAD). It reports the git error, then reloads the state.
func (w *Workspace) AddWorktree(path, branch, start string, done func(error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	_, ctx, finish := w.startTask("New worktree "+branch, true)
	repo := rs.Repo
	go func() {
		lock := writeLock(repo)
		lock.Lock()
		err := w.Git.AddWorktree(ctx, repo, path, branch, start)
		lock.Unlock()
		w.post(func() {
			finish()
			if w.Repo == rs {
				if err != nil {
					w.NotifyError("Cannot create worktree", err)
				} else {
					w.Notify(NoticeSuccess, "Worktree created", path)
					w.Refresh(true)
				}
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

// RemoveWorktree deletes the worktree at path. Without force Git refuses to
// delete one with modified or untracked files. The branch stays.
func (w *Workspace) RemoveWorktree(path string, force bool, done func(error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	_, ctx, finish := w.startTask("Delete worktree", true)
	repo := rs.Repo
	go func() {
		lock := writeLock(repo)
		lock.Lock()
		err := w.Git.RemoveWorktree(ctx, repo, path, force)
		lock.Unlock()
		w.post(func() {
			finish()
			if w.Repo == rs {
				if err != nil {
					w.NotifyError("Cannot delete worktree", err)
				} else {
					w.Notify(NoticeSuccess, "Worktree deleted", path)
					w.Refresh(true)
				}
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

// Submodules lists the repository's submodules in the background.
func (w *Workspace) Submodules(done func([]domain.Submodule, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		list, err := w.Git.Submodules(context.Background(), repo)
		w.post(func() {
			if w.Repo == rs {
				done(list, err)
			}
		})
	}()
}

// UpdateSubmodule checks out the commit recorded in the index, initializing
// the submodule when init is set. An empty path updates every submodule.
func (w *Workspace) UpdateSubmodule(path string, init bool, done func(error)) {
	name := "all submodules"
	if path != "" {
		name = path
	}
	var runErr error
	w.Run("Update "+name, func(ctx context.Context, repo domain.Repo, progress func(string)) error {
		return w.Git.UpdateSubmodule(ctx, repo, path, init, progress)
	}, RunOptions{
		Success: "Updated " + name,
		OnError: func(err error) bool { runErr = err; return false },
		Always: func() {
			if done != nil {
				done(runErr)
			}
		},
	})
}

// SyncSubmodules copies the URLs from .gitmodules into .git/config.
func (w *Workspace) SyncSubmodules(path string, done func(error)) {
	name := "all submodules"
	if path != "" {
		name = path
	}
	var runErr error
	w.Run("Sync "+name, func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		return w.Git.SyncSubmodules(ctx, repo, path)
	}, RunOptions{
		Success: "Synced " + name,
		OnError: func(err error) bool { runErr = err; return false },
		Always: func() {
			if done != nil {
				done(runErr)
			}
		},
	})
}

// Clone clones into parent/dir and opens the result.
func (w *Workspace) Clone(url, parent, dir string, done func(error)) {
	task, ctx, finish := w.startTask("Cloning "+url, true)
	go func() {
		err := os.MkdirAll(parent, 0o755)
		if err == nil {
			err = w.Git.Clone(ctx, parent, url, dir, func(l string) { w.setProgress(task, l) })
		}
		w.post(func() {
			finish()
			if err != nil {
				w.NotifyError("Clone failed", err)
			} else {
				w.Open(filepath.Join(parent, dir), nil)
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

// Init creates a repository and opens it.
func (w *Workspace) Init(dir string, done func(error)) {
	_, ctx, finish := w.startTask("Init", true)
	go func() {
		err := os.MkdirAll(dir, 0o755)
		if err == nil {
			err = w.Git.Init(ctx, dir)
		}
		w.post(func() {
			finish()
			if err != nil {
				w.NotifyError("Init failed", err)
			} else {
				w.Open(dir, nil)
			}
			if done != nil {
				done(err)
			}
		})
	}()
}

// CompareWith loads the changes between two revisions (rev..HEAD style).
func (w *Workspace) CompareWith(from, to string, done func([]domain.Change, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		list, err := w.Git.DiffNames(context.Background(), repo, from, to)
		w.post(func() {
			if w.Repo == rs {
				done(list, err)
			}
		})
	}()
}

// CompareCommits loads the files that differ between two commits: from is the
// old side and to the new one. The commits need not be parent and child; any
// two commit-ish revisions work, and the order decides the direction.
func (w *Workspace) CompareCommits(from, to string, done func([]domain.Change, error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		list, err := w.Git.DiffNames(ctx, repo, from, to)
		w.post(func() {
			if w.Repo == rs {
				done(list, err)
			}
		})
	}()
}

// RebaseCommits loads the commits an interactive rebase starting at from
// would replay, oldest first, and the base it rebases onto.
func (w *Workspace) RebaseCommits(from string, done func(list []gitcli.RebaseCommit, base string, err error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		list, base, err := w.Git.RebaseCommits(ctx, repo, from)
		w.post(func() {
			if w.Repo == rs {
				done(list, base, err)
			}
		})
	}()
}

// InteractiveRebase replays the commits after base following the plan.
// Conflicts stop it like any rebase; the operation banner then offers
// Continue, Skip and Abort.
func (w *Workspace) InteractiveRebase(base string, steps []gitcli.RebaseStep) {
	w.Run("Interactive rebase", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.InteractiveRebase(ctx, repo, base, steps)
		return err
	}, RunOptions{OnError: w.operationError("Rebase"), Success: "Rebase finished"})
}

// CommitMessage loads one commit's full message for the edit dialog.
func (w *Workspace) CommitMessage(hash string, done func(message string, err error)) {
	rs := w.Repo
	if rs == nil {
		return
	}
	repo := rs.Repo
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		d, err := w.Git.CommitDetail(ctx, repo, hash)
		w.post(func() {
			if w.Repo == rs {
				done(d.Body, err)
			}
		})
	}()
}

// RewordCommit replaces one commit's message. Its content and the commits
// after it stay the same.
func (w *Workspace) RewordCommit(hash, message string) {
	short := domain.ShortHash(hash)
	w.Run("Edit commit message", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.RewordCommit(ctx, repo, hash, message)
		return err
	}, RunOptions{OnError: w.rewriteError("Edit commit message"), Success: "Message of " + short + " updated",
		SuccessBody: "The commits after it were replayed on top."})
}

// SquashCommit folds one commit into its parent with a combined message.
func (w *Workspace) SquashCommit(hash, message string) {
	short := domain.ShortHash(hash)
	w.Run("Squash commit", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.SquashCommit(ctx, repo, hash, message)
		return err
	}, RunOptions{OnError: w.rewriteError("Squash commit"), Success: "Squashed " + short + " into its parent",
		SuccessBody: "The combined commit keeps both changes."})
}

// DropCommit removes one commit from the branch; its changes disappear.
func (w *Workspace) DropCommit(hash string) {
	short := domain.ShortHash(hash)
	w.Run("Drop commit", func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		_, err := w.Git.DropCommit(ctx, repo, hash)
		return err
	}, RunOptions{OnError: w.rewriteError("Drop commit"), Success: "Dropped " + short,
		SuccessBody: "The commits after it were replayed without it."})
}

// rewriteError explains a refused or aborted history edit. The Git command
// aborts its own rebase, so the branch is back where it started.
func (w *Workspace) rewriteError(title string) func(error) bool {
	return func(err error) bool {
		switch {
		case errors.Is(err, gitcli.ErrRewriteAbortLeft):
			w.Notify(NoticeError, title+" failed", "The rebase is still in progress. Run `git rebase --abort` in a terminal before doing anything else.\n\n"+ErrorText(err))
			return true
		case gitcli.KindOf(err) == gitcli.ErrConflict:
			w.Notify(NoticeWarning, title+" was aborted", "The edit hit a conflict. Viff aborted the rebase and left the branch as it was.")
			return true
		case errors.Is(err, gitcli.ErrRewriteOperation),
			errors.Is(err, gitcli.ErrRewriteRoot),
			errors.Is(err, gitcli.ErrRewriteMerge),
			errors.Is(err, gitcli.ErrRewriteUnrelated),
			errors.Is(err, gitcli.ErrRewriteMessage):
			w.Notify(NoticeWarning, title, err.Error())
			return true
		}
		return false
	}
}
