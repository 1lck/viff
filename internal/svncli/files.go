package svncli

import (
	"bytes"
	"errors"
	"io"
	"os"

	"viff/internal/vcs"
)

// errNotExist reports a path that is not on disk. It is compared with
// errors.Is so a caller can tell "absent side" from "unreadable file".
var errNotExist = errors.New("file does not exist")

// readFileLimited reads at most limit+1 bytes so the caller can detect an
// oversized file without loading it whole. A file at exactly limit is not
// truncated; anything longer comes back one byte over the limit.
func readFileLimited(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errNotExist
		}
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, errNotExist
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// isBinary uses the same heuristic as the Git adapter: a NUL byte in the first
// 8000 bytes. It keeps binary detection identical across backends so a preview
// decision does not depend on which one opened the repository.
func isBinary(data []byte) bool {
	if len(data) > 8000 {
		data = data[:8000]
	}
	return bytes.IndexByte(data, 0) >= 0
}

// contentOrAbsent turns an unreadable working file into an absent side.
func contentOrAbsent(data []byte, err error) (vcs.Content, error) {
	if err != nil {
		if errors.Is(err, errNotExist) {
			return vcs.Content{}, nil
		}
		return vcs.Content{}, err
	}
	c := vcs.Content{Exists: true, Size: int64(len(data)), Mode: "100644"}
	if len(data) > vcs.ContentLimit {
		c.TooLarge = true
		return c, nil
	}
	c.Data = data
	c.Binary = isBinary(data)
	return c, nil
}
