package viffui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
	"viff/internal/storage"
)

// Git command and signing settings. Kept in their own file so the dialog
// stays readable and the Git fields do not crowd the appearance page.

// signFormatKeys maps the radio buttons of the Git page to the values Git
// understands; the first entry is "use Git's own setting".
var signFormatKeys = []string{"", "openpgp", "ssh"}

// gitSettingsDraft holds the editors of the Git page while the dialog is
// open. Values move into storage.Settings only on OK.
type gitSettingsDraft struct {
	ssh, gpg, key, helper *text.Editor
	format                int // index into signFormatKeys
	sign                  bool
}

func newGitSettingsDraft(s *storage.Settings) *gitSettingsDraft {
	return &gitSettingsDraft{
		ssh:    text.NewEditor(s.SSHCommand),
		gpg:    text.NewEditor(s.GPGProgram),
		key:    text.NewEditor(s.SigningKey),
		helper: text.NewEditor(s.CredentialHelper),
		format: signFormatIndex(s.SignFormat),
		sign:   s.CommitSigning,
	}
}

// form paints the fields. It runs on every paint, so validation hints follow
// what the user has typed.
func (d *gitSettingsDraft) form(f *form) {
	f.field(f.w.t(i18n.GitSSHCommand), d.ssh, f.w.t(i18n.GitSSHPlaceholder), false)
	f.commandHint(d.ssh.String())
	f.field(f.w.t(i18n.GitGPGProgram), d.gpg, f.w.t(i18n.GitGPGPlaceholder), false)
	f.commandHint(d.gpg.String())
	f.text(f.w.t(i18n.GitSignFormat))
	f.radio([]string{f.w.t(i18n.GitSignDefault), "OpenPGP", "SSH"}, &d.format)
	f.field(f.w.t(i18n.GitSigningKey), d.key, f.w.t(i18n.GitSigningKeyHint), false)
	f.check(f.w.t(i18n.GitSignByDefault), &d.sign)
	f.field(f.w.t(i18n.GitCredentialHelper), d.helper, f.w.t(i18n.GitCredentialHint), false)
}

// commandHint warns, without blocking, when the command's program is not on
// disk: a value may be a name that only exists in the user's shell.
func (f *form) commandHint(line string) {
	name := missingCommand(line)
	if name == "" {
		return
	}
	indent := f.labelW
	f.custom(18, func(r ui.Rect) {
		f.w.p.text(f.w.t(i18n.GitCommandNotFound, name), rect(r.X+indent, r.Y, r.Width-indent, 16), fontSmall, regular, f.w.pal.Warning)
	})
}

// commit writes the draft back into the settings.
func (d *gitSettingsDraft) commit(s *storage.Settings) {
	s.SSHCommand = strings.TrimSpace(d.ssh.String())
	s.GPGProgram = strings.TrimSpace(d.gpg.String())
	s.SigningKey = strings.TrimSpace(d.key.String())
	s.CredentialHelper = strings.TrimSpace(d.helper.String())
	s.SignFormat = signFormatKeys[d.format]
	s.CommitSigning = d.sign
}

func signFormatIndex(format string) int {
	for i, k := range signFormatKeys {
		if k == format {
			return i
		}
	}
	return 0
}

// missingCommand returns the program of a command line when it cannot be
// found, or "" when it exists. Only the first word is checked: the rest are
// arguments that Git or the shell expands.
func missingCommand(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return ""
	}
	name := fields[0]
	if strings.HasPrefix(name, `"`) || strings.HasPrefix(name, "'") {
		return "" // quoted paths may contain spaces; do not guess
	}
	if strings.ContainsAny(name, `/\`) {
		if st, err := os.Stat(name); err == nil && !st.IsDir() {
			return ""
		}
		return name
	}
	if _, err := exec.LookPath(name); err == nil {
		return ""
	}
	// GUI processes on macOS start without the user's shell PATH.
	for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"} {
		if st, err := os.Stat(filepath.Join(dir, name)); err == nil && !st.IsDir() {
			return ""
		}
	}
	return name
}

// settingsTabs draws the page buttons of the settings dialog. The dialog
// body is a function of the page, so it can switch without changing height
// bookkeeping.
func settingsTabs(f *form, page *int, labels []string) {
	f.custom(30, func(r ui.Rect) {
		x := r.X
		for i, label := range labels {
			bw := f.w.buttonWidth(label)
			kind := btnBorderless
			if i == *page {
				kind = btnDefault
			}
			if f.w.button(scDialogLinks.At(700+i), rect(x, r.Y, bw, 26), label, kind, false) {
				if *page != i {
					*page = i
					// A new page starts at its top: keeping the offset would
					// land the reader past the end of a shorter page.
					if d := f.w.dialog; d != nil {
						d.scroll = scroller{}
					}
					f.w.closePopups()
					f.w.Invalidate()
				}
			}
			x += bw + 8
		}
	})
	f.gap(6)
}
