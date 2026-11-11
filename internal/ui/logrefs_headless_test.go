//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

func TestLogRefTooltip(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			f := newUIFixture(t)
			branch := "feature/" + strings.Repeat("long-name-", 8) + "提交提示"
			remote := "origin/" + branch
			const tag = "release/包含完整名称的标签"
			f.git("branch", "-m", branch)
			f.git("update-ref", "refs/remotes/"+remote, "HEAD")
			f.git("tag", tag)
			f.ws.Settings.Theme = theme
			f.win.applyTheme()
			f.start(1280, 800)
			f.win.openRepo(f.dir)
			f.settle()
			row, ok := f.s.Hit("Log the listen address")
			if !ok {
				t.Fatal("missing commit row")
			}
			x, y := right(row.Bounds)-authorW-dateW-20, row.Bounds.Y+logRowH/2
			if err := f.s.Move(x, y); err != nil {
				t.Fatal(err)
			}
			if err := f.s.WaitFor(func(s app.Snapshot) bool {
				return f.s.HasText(tag)
			}, 2*time.Second); err != nil {
				t.Fatalf("hovering the reference labels did not show the hidden tag: %v", err)
			}
			want := []string{branch, remote, tag}
			// Wrapped names must remain complete and inside the window.
			var painted strings.Builder
			for _, text := range f.s.Snapshot().Texts {
				if text.Layer != 40 {
					continue
				}
				painted.WriteString(text.Value)
				if text.Bounds.X < 0 || right(text.Bounds) > 1280 || text.Bounds.Y < 0 || bottom(text.Bounds) > 800 {
					t.Fatalf("tooltip text outside the window: %+v", text)
				}
			}
			for _, name := range want {
				if !strings.Contains(painted.String(), name) {
					t.Fatalf("painted tooltip %q is missing %q", painted.String(), name)
				}
			}
			f.snap("log-ref-tooltip")
			f.assertIdle()

			// The labels still select the commit and give its list keyboard focus.
			if err := f.s.Click(x, y); err != nil {
				t.Fatal(err)
			}
			f.settle()
			head := f.ws.Repo.Status.Branch.Head
			if !f.win.lg.sel[head] || !f.win.focused(idLogTable) {
				t.Fatal("clicking reference labels did not select and focus the commit")
			}
			if err := f.s.Keys("down"); err != nil {
				t.Fatal(err)
			}
			f.settle()
			if f.win.lg.sel[head] {
				t.Fatal("Down did not move selection away from the reference row")
			}
			if err := f.s.RightClick(x, y); err != nil {
				t.Fatal(err)
			}
			f.settle()
			if len(f.win.popups) == 0 || !f.win.lg.sel[head] || f.win.tipID != (ui.ID{}) {
				t.Fatal("right-clicking references did not select the commit, open its menu and dismiss the tooltip")
			}
			if err := f.s.Keys("escape"); err != nil {
				t.Fatal(err)
			}
			if err := f.s.Move(row.Bounds.X+80, y); err != nil {
				t.Fatal(err)
			}
			f.s.Settle(40*time.Millisecond, time.Second)
			if f.win.tipID != (ui.ID{}) {
				t.Fatal("reference tooltip remained after leaving the labels")
			}
			// Re-enter from the subject without crossing a different commit hit.
			if err := f.s.Move(x, y); err != nil {
				t.Fatal(err)
			}
			if err := f.s.WaitFor(func(app.Snapshot) bool { return f.s.HasText(tag) }, 2*time.Second); err != nil {
				t.Fatalf("moving within the commit row did not restore the tooltip: %v", err)
			}
			// A single short reference also has a tip; a row with no refs has none.
			for _, tc := range []struct{ subject, tip string }{
				{"Guard the store with a mutex", "fix/store-locking"},
				{"Initial commit", ""},
			} {
				hit, ok := f.s.Hit(tc.subject)
				if !ok {
					t.Fatalf("missing commit row %q", tc.subject)
				}
				if err := f.s.Move(right(hit.Bounds)-authorW-dateW-20, hit.Bounds.Y+logRowH/2); err != nil {
					t.Fatal(err)
				}
				f.s.Settle(40*time.Millisecond, time.Second)
				if got := f.win.tips[f.win.tipID]; got != tc.tip {
					t.Fatalf("tooltip on %q = %q, want %q", tc.subject, got, tc.tip)
				}
			}
			// Ordinary action tips keep their shortcut alongside the title.
			for _, hit := range f.s.Snapshot().Hits {
				if hit.ID != idCmExpand {
					continue
				}
				main, shortcut := splitShortcut(f.win.tips[idCmExpand])
				if main == "" || shortcut == "" {
					t.Fatal("expected an action tooltip with a title and shortcut")
				}
				if err := f.s.Move(hit.Bounds.X+hit.Bounds.Width/2, hit.Bounds.Y+hit.Bounds.Height/2); err != nil {
					t.Fatal(err)
				}
				if err := f.s.WaitFor(func(s app.Snapshot) bool {
					var painted string
					for _, text := range s.Texts {
						if text.Layer == 40 {
							painted += text.Value
						}
					}
					return painted == main+kbdHint(shortcut)
				}, 2*time.Second); err != nil {
					t.Fatalf("ordinary tooltip lost its title or shortcut: %v", err)
				}
				return
			}
			t.Fatal("missing Expand All action")
		})
	}
}
