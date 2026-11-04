package platform

import (
	"embed"
	"os"
	"path/filepath"
)

// JetBrains Mono (SIL Open Font License 1.1, see fonts/OFL.txt) is bundled
// so code and diffs use the same face as IntelliJ on every machine.
//
//go:embed fonts/*.ttf fonts/OFL.txt
var bundledFonts embed.FS

// EditorFontFamily is the family name of the bundled editor font.
const EditorFontFamily = "JetBrains Mono"

// writeFonts copies the bundled fonts into the user cache so the system font
// manager can load them by URL. It returns their paths.
func writeFonts() ([]string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "Viff", "fonts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := bundledFonts.ReadDir("fonts")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".ttf" {
			continue
		}
		data, err := bundledFonts.ReadFile("fonts/" + e.Name())
		if err != nil {
			return nil, err
		}
		p := filepath.Join(dir, e.Name())
		if st, err := os.Stat(p); err != nil || st.Size() != int64(len(data)) {
			if err := os.WriteFile(p, data, 0o644); err != nil {
				return nil, err
			}
		}
		paths = append(paths, p)
	}
	return paths, nil
}
