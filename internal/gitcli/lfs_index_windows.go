package gitcli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows, even a metadata-only handle to the index can block Git's
// rename-over-existing operation. Query the one named entry from the parent
// directory instead. This also captures the file ID and last change time.
type lfsLocalIndexRevision struct {
	found                          bool
	id, size, write, change, birth int64
}

func (a lfsLocalIndexRevision) equal(b lfsLocalIndexRevision) bool { return a == b }

var ntQueryIndexEntry = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryDirectoryFile")

// FILE_ID_FULL_DIR_INFORMATION and FILE_DIRECTORY_INFORMATION share the
// prefix up to NameLength. The latter is a fallback for network redirectors
// that return directory timestamps but do not expose a file ID.
type lfsIndexDirEntry struct {
	NextOffset, Index                   uint32
	Birth, Access, Write, Change, Size  int64
	Allocation                          int64
	Attributes, NameLength, EASize, Pad uint32
	ID                                  int64
	Name                                [256]uint16
}

func localLFSIndexRevision(index string) (lfsLocalIndexRevision, error) {
	stamp, attrs, err := queryLFSIndexEntry(index)
	if err == nil && attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		target, resolveErr := filepath.EvalSymlinks(index)
		if resolveErr != nil {
			return lfsLocalIndexRevision{}, resolveErr
		}
		if target != index {
			stamp, _, err = queryLFSIndexEntry(target)
		}
	}
	return stamp, err
}

func queryLFSIndexEntry(index string) (lfsLocalIndexRevision, uint32, error) {
	fail := func(err error) (lfsLocalIndexRevision, uint32, error) {
		return lfsLocalIndexRevision{}, 0, &os.PathError{Op: "query index entry", Path: index, Err: err}
	}
	const fileDirectoryInformation = 1
	const fileIDFullDirectoryInformation = 38
	parent := filepath.Clean(filepath.Dir(index))
	if len(parent) >= 248 && !strings.HasPrefix(parent, `\\?\`) {
		if strings.HasPrefix(parent, `\\`) {
			parent = `\\?\UNC\` + parent[2:]
		} else {
			parent = `\\?\` + parent
		}
	}
	p, err := windows.UTF16PtrFromString(parent)
	if err != nil {
		return fail(err)
	}
	h, err := windows.CreateFile(p, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fail(err)
	}
	defer windows.CloseHandle(h)
	name, err := windows.NewNTUnicodeString(filepath.Base(index))
	if err != nil {
		return fail(err)
	}
	var entry lfsIndexDirEntry
	var iosb windows.IO_STATUS_BLOCK
	query := func(class uintptr) windows.NTStatus {
		ret, _, _ := ntQueryIndexEntry.Call(uintptr(h), 0, 0, 0, uintptr(unsafe.Pointer(&iosb)),
			uintptr(unsafe.Pointer(&entry)), unsafe.Sizeof(entry), class, 1, uintptr(unsafe.Pointer(name)), 1)
		return windows.NTStatus(ret)
	}
	status := query(fileIDFullDirectoryInformation)
	switch status {
	case windows.STATUS_INVALID_INFO_CLASS, windows.STATUS_NOT_SUPPORTED, windows.STATUS_INVALID_PARAMETER:
		status = query(fileDirectoryInformation)
		entry.ID = 0
	}
	switch status {
	case 0:
		return lfsLocalIndexRevision{true, entry.ID, entry.Size, entry.Write, entry.Change, entry.Birth}, entry.Attributes, nil
	case windows.STATUS_NO_SUCH_FILE, windows.STATUS_NO_MORE_FILES:
		return fail(fs.ErrNotExist)
	default:
		return fail(status)
	}
}
