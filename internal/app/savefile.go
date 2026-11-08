package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"viff/internal/domain"
	"viff/internal/i18n"
)

// In-app editing writes one working-tree file. The read-only viewer and every
// Git view stay the source of truth: a save re-reads the file immediately
// before the atomic rename and refuses to overwrite a file somebody else
// changed since it was opened.
//
// The editor is deliberately not a general file writer. It accepts one
// repository-relative path, an expected hash and text the editor holds; it
// refuses symlinks, submodules, binary or oversized files, and a read-only
// file. Encoding is limited to UTF-8. The editor normalizes line endings to
// LF internally; SaveFile restores the ending the file had when it was opened.

var (
	// ErrEditPath means the path is not a repository-relative regular file.
	ErrEditPath = errors.New("the path is not a file inside this repository")
	// ErrEditMissing means the file disappeared after it was opened.
	ErrEditMissing = errors.New("the file no longer exists")
	// ErrEditSymlink means the path is a symbolic link; Viff does not follow
	// one when saving, because the link target may be outside the repository.
	ErrEditSymlink = errors.New("symbolic links cannot be edited in Viff")
	// ErrEditSubmodule means the path is a submodule or directory.
	ErrEditSubmodule = errors.New("submodules cannot be edited in Viff")
	// ErrEditReadOnly means the file has no write permission bit.
	ErrEditReadOnly = errors.New("the file is read-only")
	// ErrEditTooLarge means the file is above the editing limit.
	ErrEditTooLarge = errors.New("the file is too large to edit in Viff")
	// ErrEditBinary means the bytes are not valid UTF-8 text.
	ErrEditBinary = errors.New("the file is not UTF-8 text")
	// ErrEditChanged means the file changed on disk after it was opened.
	ErrEditChanged = errors.New("the file changed on disk")
)

// EditLimit is the largest file the in-app editor opens. The renderer rebuilds
// display lines and syntax spans after an edit, so the limit is lower than the
// diff viewer's ContentLimit; larger files stay read-only and open externally.
const EditLimit = 1 << 20

// SaveFileRequest is one editor save. Text is the editor's LF-normalized text;
// SaveFile writes it back with the file's original line ending and BOM.
type SaveFileRequest struct {
	// Path is repository-relative and slash-separated.
	Path string
	// Text is the complete new contents, with LF line endings.
	Text string
	// LineEnding is "CRLF", "LF" or "" for a file without a newline.
	LineEnding string
	// BOM restores a UTF-8 byte-order mark that the editor stripped.
	BOM bool
	// Expected is the SHA-256 of the file when the editor loaded it.
	Expected [sha256.Size]byte
}

// SaveFileResult describes what SaveFile wrote.
type SaveFileResult struct {
	// Hash is the SHA-256 of the bytes now on disk, so the next save can
	// detect an external change against this one.
	Hash [sha256.Size]byte
	// Size is the number of bytes written.
	Size int64
}

// SaveFile writes req.Text to req.Path on the UI thread's repository. It runs
// as a write task: saves to one repository are serialized with Git writes, and
// the status is queried again afterwards. done runs on the UI thread; a failed
// save keeps the editor's text and returns the reason.
func (w *Workspace) SaveFile(req SaveFileRequest, done func(SaveFileResult, error)) {
	if done == nil {
		done = func(SaveFileResult, error) {}
	}
	rs := w.Repo
	if rs == nil {
		done(SaveFileResult{}, errors.New(w.tr(i18n.AppErrorOpenRepositoryFirst)))
		return
	}
	var result SaveFileResult
	w.Run(w.tr(i18n.AppNoticeSavingFile, domain.FileName(req.Path)), func(ctx context.Context, repo domain.Repo, _ func(string)) error {
		var err error
		result, err = writeWorkingFile(repo, req)
		return err
	}, RunOptions{
		// The editor has better messages than the generic Git one: changed on
		// disk, read-only, too large. It also owns the Reload action.
		OnError: func(err error) bool {
			if w.Repo == rs {
				done(SaveFileResult{}, err)
			}
			return true
		},
		After: func() {
			if w.Repo == rs {
				done(result, nil)
			}
		},
	})
}

// writeWorkingFile is the save's body, separated so tests can exercise the
// checks without the task scheduler.
func writeWorkingFile(repo domain.Repo, req SaveFileRequest) (SaveFileResult, error) {
	full, err := repositoryFilePath(repo, req.Path)
	if err != nil {
		return SaveFileResult{}, err
	}
	info, err := os.Lstat(full)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditMissing, req.Path)
	case err != nil:
		return SaveFileResult{}, fmt.Errorf("read %s: %w", req.Path, err)
	case info.Mode()&os.ModeSymlink != 0:
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditSymlink, req.Path)
	case info.IsDir():
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditSubmodule, req.Path)
	case !info.Mode().IsRegular():
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditPath, req.Path)
	case info.Mode().Perm()&0o222 == 0:
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditReadOnly, req.Path)
	case info.Size() > EditLimit:
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditTooLarge, req.Path)
	}
	if len(req.Text) > EditLimit {
		// Check before building the CRLF-expanded copy: a paste far over the
		// limit must not allocate a second large buffer first.
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditTooLarge, req.Path)
	}
	before, err := os.ReadFile(full)
	if err != nil {
		return SaveFileResult{}, fmt.Errorf("read %s: %w", req.Path, err)
	}
	if len(before) > EditLimit {
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditTooLarge, req.Path)
	}
	if isBinaryData(before) || !utf8.Valid(bytesWithoutBOM(before)) {
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditBinary, req.Path)
	}
	if sha256.Sum256(before) != req.Expected {
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditChanged, req.Path)
	}
	data, err := encodeEdit(req)
	if err != nil {
		return SaveFileResult{}, err
	}
	if len(data) > EditLimit {
		return SaveFileResult{}, fmt.Errorf("%w: %s", ErrEditTooLarge, req.Path)
	}
	if err := replaceFile(full, info.Mode().Perm(), data); err != nil {
		return SaveFileResult{}, fmt.Errorf("write %s: %w", req.Path, err)
	}
	return SaveFileResult{Hash: sha256.Sum256(data), Size: int64(len(data))}, nil
}

// repositoryFilePath turns a repository-relative path into an absolute one and
// refuses anything that could leave the repository. It is lexical: a symlinked
// parent directory is not followed here, but the file itself is rejected as a
// symlink before writing.
func repositoryFilePath(repo domain.Repo, rel string) (string, error) {
	if repo.Root == "" || rel == "" || strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("%w: %q", ErrEditPath, rel)
	}
	if runtime.GOOS == "windows" {
		// Git stores forward slashes; a backslash is a separator only on
		// Windows, where a Git path can legitimately arrive with one.
		rel = strings.ReplaceAll(rel, "\\", "/")
	}
	slash := path.Clean(rel)
	if slash == "." || slash == ".." || path.IsAbs(slash) || strings.HasPrefix(slash, "../") {
		return "", fmt.Errorf("%w: %q", ErrEditPath, rel)
	}
	for _, part := range strings.Split(slash, "/") {
		if part == "" || part == ".." {
			return "", fmt.Errorf("%w: %q", ErrEditPath, rel)
		}
		if strings.EqualFold(part, ".git") {
			return "", fmt.Errorf("%w: %q", ErrEditPath, rel)
		}
	}
	full := filepath.Join(repo.Root, filepath.FromSlash(slash))
	inside, err := filepath.Rel(repo.Root, full)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("%w: %q", ErrEditPath, rel)
	}
	return full, nil
}

// encodeEdit restores the BOM and line ending the editor normalized away.
func encodeEdit(req SaveFileRequest) ([]byte, error) {
	// The editor strips the BOM; a caller that passes one back must not end up
	// with two.
	text := strings.TrimPrefix(req.Text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("%w: %s", ErrEditBinary, req.Path)
	}
	switch req.LineEnding {
	case "":
		// A file without a newline stays without one unless the user added
		// one; text is written exactly as the editor holds it.
	case "CRLF":
		text = strings.ReplaceAll(text, "\n", "\r\n")
	case "LF":
	default:
		return nil, fmt.Errorf("%w: unknown line ending %q", ErrEditPath, req.LineEnding)
	}
	size := len(text)
	if req.BOM {
		size += len(utf8BOM)
	}
	out := make([]byte, 0, size)
	if req.BOM {
		out = append(out, utf8BOM...)
	}
	out = append(out, text...)
	return out, nil
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func bytesWithoutBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

// replaceFile writes data beside the target and renames it over the target, so
// a failed write leaves the old file intact. The replacement keeps the
// original permission bits; ownership is left to the operating system.
func replaceFile(name string, perm os.FileMode, data []byte) error {
	dir := filepath.Dir(name)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+".viff-save-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, name)
}

// isBinaryData is the same first-bytes heuristic the content reader uses: a
// NUL before the text limit means binary. UTF-8 validity is checked separately
// when encoding.
func isBinaryData(data []byte) bool {
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	for _, b := range head {
		if b == 0 {
			return true
		}
	}
	return false
}
