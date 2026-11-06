//go:build vivahead

package viffui

import (
	"slices"
	"testing"
	"time"

	"viff/internal/i18n"
)

// Exercise the Log chip itself: the toolbar branch switcher already searched,
// but the log filter opened a menu with no focused text field.
func TestLogBranchFilterSearch(t *testing.T) {
	for _, tc := range []struct{ theme, language string }{
		{"dark", "en"}, {"light", "zh-CN"},
	} {
		t.Run(tc.theme, func(t *testing.T) {
			f := newUIFixture(t)
			f.ws.Settings.Theme, f.ws.Settings.Language = tc.theme, tc.language
			f.win.applyTheme()
			f.win.applyLanguage()
			f.start(1440, 900)
			f.win.openRepo(f.dir)
			f.settle()
			branch := f.win.t(i18n.ChromeLogFilterBranch)
			typeQuery := func(query string) {
				t.Helper()
				if err := f.s.Type(query); err != nil {
					t.Fatal(err)
				}
				f.s.Settle(40*time.Millisecond, time.Second)
			}
			keys := func(chords ...string) {
				t.Helper()
				for _, chord := range chords {
					if err := f.s.Keys(chord); err != nil {
						t.Fatal(err)
					}
				}
				f.s.Settle(40*time.Millisecond, time.Second)
			}

			f.click(branch)
			f.snap("log-branch-menu")
			typeQuery("SEARCH")
			p := f.win.popups[0]
			var matches []string
			for _, i := range p.visible() {
				if it := p.items[i]; it.selectable() {
					matches = append(matches, it.title)
				}
			}
			if !slices.Equal(matches, []string{"feature/search", "origin/feature/search"}) {
				t.Fatalf("search matches = %v", matches)
			}
			if got := f.ws.Repo.Log.Filter.Branch; got != "" {
				t.Fatalf("typing applied branch filter %q before selection", got)
			}
			f.snap("log-branch-matches")
			// Navigate in both directions, then select the remote, not the local.
			keys("down", "up", "down", "enter")
			f.settle()
			const remote = "origin/feature/search"
			if got := f.ws.Repo.Log.Filter.Branch; got != remote {
				t.Fatalf("branch after selecting the second match = %q, want %q", got, remote)
			}
			if got := f.win.lg.brSel; got != "refs/remotes/"+remote {
				t.Fatalf("selected ref = %q", got)
			}

			f.click(branch + ": " + remote)
			if it, ok := menuItem(f.win.popups[0].items, remote); !ok || !it.checked {
				t.Fatal("the current branch filter is not checked on reopen")
			}
			typeQuery("unmatched")
			if !f.s.HasText(f.win.t(i18n.ChromePopupEmpty)) {
				t.Fatal("missing empty search result")
			}
			f.snap("log-branch-empty")
			keys("enter")
			if len(f.win.popups) != 1 || f.ws.Repo.Log.Filter.Branch != remote {
				t.Fatal("Enter on an empty result changed the branch filter or closed the popup")
			}
			keys("escape")
			if len(f.win.popups) != 0 || f.ws.Repo.Log.Filter.Branch != remote {
				t.Fatal("dismissing a search changed the branch filter or left the popup open")
			}
			f.click(branch + ": " + remote)
			if f.win.popups[0].search.String() != "" {
				t.Fatal("reopening retained the previous search")
			}
			typeQuery("v0.1")
			f.click("v0.1.0")
			f.settle()
			if got := f.ws.Repo.Log.Filter.Branch; got != "v0.1.0" {
				t.Fatalf("clicking a matching tag selected %q", got)
			}
			f.click(branch + ": v0.1.0")
			typeQuery("HEAD")
			keys("enter")
			f.settle()
			if got := f.ws.Repo.Log.Filter.Branch; got != "HEAD" {
				t.Fatalf("selecting HEAD applied %q", got)
			}
			f.click(branch + ": HEAD")
			f.click(f.win.t(i18n.ChromeLogFilterAll))
			f.settle()
			if got := f.ws.Repo.Log.Filter.Branch; got != "" {
				t.Fatalf("All left branch filter %q active", got)
			}
			f.assertIdle()
		})
	}
}
