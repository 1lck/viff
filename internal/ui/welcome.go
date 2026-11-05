package viffui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/assets/branding"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// welcomeView is the welcome screen's state.
type welcomeView struct {
	search *text.Editor
	nav    int
	sel    int
	scroll scroller
}

func (v *welcomeView) init() {
	v.search = text.NewEditor("")
	v.sel = -1
}

func (w *Window) recentShown() []string {
	q := strings.ToLower(strings.TrimSpace(w.wel.search.String()))
	var out []string
	for _, r := range w.ws.Settings.Recent {
		if q != "" && !fuzzy(strings.ToLower(r.Path), q) {
			continue
		}
		out = append(out, r.Path)
	}
	return out
}

// paintWelcome draws IntelliJ's Welcome screen.
func (w *Window) paintWelcome(full ui.Rect) {
	v := &w.wel
	r := full
	bar := splitTop(&r, toolbarH)
	w.c.MoveRegion(bar)
	w.p.textCenter("Welcome to Viff", bar, fontUI, semibold, w.pal.TextMuted)

	// Left navigation.
	nav := splitLeft(&r, 230)
	nav = inset(nav, 0, 0)
	w.c.DrawBitmap(branding.Bitmap, rect(nav.X+20, nav.Y+16, 36, 36))
	w.p.text("Viff", rect(nav.X+66, nav.Y+14, 150, 22), 15, semibold, w.pal.Text)
	w.p.text(w.t(i18n.WelcomeGitClient), rect(nav.X+66, nav.Y+34, 150, 18), fontSmall, regular, w.pal.TextSecond)
	items := []struct {
		title string
		icon  icons.Pair
	}{{"Projects", icons.Project}, {"Customize", icons.Settings}}
	ny := nav.Y + 74
	for i, it := range items {
		id := scWelNav.At(i)
		rr := rect(nav.X+12, ny, nav.Width-24, 30)
		switch {
		case v.nav == i:
			w.p.round(rr, 6, w.pal.Selection)
		case w.hovered(id):
			w.p.round(rr, 6, w.pal.Hover)
		}
		w.p.iconIn(it.icon, rect(rr.X+8, rr.Y, 16, rr.Height))
		w.p.text(it.title, rect(rr.X+32, rr.Y, rr.Width-40, rr.Height), fontUI, regular, w.pal.Text)
		w.c.HitWithOptions(id, rr, it.title, ui.HitOptions{Role: ui.RoleTab, Selected: v.nav == i, SkipFocus: true})
		if w.c.Clicked(id) {
			i := i
			w.after(func() { v.nav = i })
		}
		ny += 34
	}

	main := inset(rect(r.X, r.Y, r.Width, r.Height), 0, 0)
	main.Width -= 10
	main.Height -= 10
	w.p.round(main, islandRadius, w.pal.Island)
	if v.nav == 1 {
		w.paintCustomize(inset(main, 24, 20))
		return
	}
	recent := w.recentShown()
	if len(w.ws.Settings.Recent) == 0 {
		w.paintWelcomeEmpty(main)
		return
	}
	// Top row: search and buttons.
	top := rect(main.X+20, main.Y+16, main.Width-40, 30)
	bx := right(top)
	for i, b := range []struct {
		label string
		id    ui.ID
		fn    func()
	}{{"Clone", idWelClone, w.openClone}, {"Open", idWelOpen, w.chooseRepoNow}, {"New Repository", idWelInit, w.openInit}, {"Repositories…", idWelcomeRepositories, w.openRepositories}} {
		bw := w.buttonWidth(b.label)
		kind := btnNormal
		if i == 1 {
			kind = btnDefault
		}
		if w.button(b.id, rect(bx-bw, top.Y+1, bw, buttonH), b.label, kind, false) {
			fn := b.fn
			w.after(fn)
		}
		bx -= bw + 10
	}
	w.field(idWelSearch, v.search, rect(top.X, top.Y, min(bx-top.X-10, 420), 30), fieldOpts{icon: icons.Search, placeholder: "Search projects", nav: true})
	w.onKeys(idWelSearch, func(e ui.KeyEvent) bool {
		switch e.Key {
		case ui.KeyDown:
			v.sel = min(len(recent)-1, v.sel+1)
		case ui.KeyUp:
			v.sel = max(0, v.sel-1)
		case ui.KeyEnter:
			if v.sel >= 0 && v.sel < len(recent) {
				w.openRepo(recent[v.sel])
			} else if len(recent) == 1 {
				w.openRepo(recent[0])
			}
		default:
			return false
		}
		return true
	})

	list := rect(main.X+10, top.Y+46, main.Width-20, bottom(main)-top.Y-56)
	w.list(idWelList, scWelRecent, list, "Recent projects")
	rowHt := float32(52)
	content := float32(len(recent)) * rowHt
	v.scroll.clamp(content, list.Height)
	w.onScrollIn(list, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(content, list.Height) })
	if len(recent) == 0 {
		w.p.textCenter(w.t(i18n.WelcomeNoProjects), rect(list.X, list.Y+40, list.Width, 22), fontUI, regular, w.pal.TextSecond)
	}
	w.c.Clip(list)
	for i, p := range recent {
		y := list.Y + float32(i)*rowHt - v.scroll.Y
		rr := rect(list.X, y, list.Width, rowHt-4)
		id := scWelRecent.At(i)
		rid := scWelRemove.At(i)
		hot := w.hovered(id) || w.hovered(rid)
		if v.sel == i {
			w.p.round(rr, 8, w.pal.Selection)
		} else if hot {
			w.p.round(rr, 8, w.pal.Hover)
		}
		name := filepath.Base(p)
		w.projectBadge(rect(rr.X+12, rr.Y+(rr.Height-30)/2, 30, 30), name, 7)
		w.p.text(name, rect(rr.X+54, rr.Y+6, rr.Width-110, 20), fontUI, semibold, w.pal.Text)
		missing := false
		if _, err := os.Stat(p); err != nil {
			missing = true
		}
		sub := shortenHome(p)
		subC := w.pal.TextSecond
		if missing {
			sub += " (not found)"
			subC = w.pal.Error
		}
		w.p.text(sub, rect(rr.X+54, rr.Y+25, rr.Width-110, 18), fontSmall, regular, subC)
		w.c.HitWithOptions(id, rr, name, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: v.sel == i})
		if hot {
			if w.action(rid, right(rr)-36, rr.Y+(rr.Height-actionSize)/2, icons.Close, "Remove from Recent Projects", actOpts{}) {
				path := p
				w.after(func() { w.ws.Settings.RemoveRecent(path); w.saveSettings() })
			}
		}
		if w.c.Clicked(id) {
			path := p
			w.after(func() { w.openRepo(path) })
		}
		path := p
		w.onContext(id, func(x, y float32) {
			w.openMenu(x, y, []item{
				{title: w.t(i18n.WelcomeOpen), run: func() { w.openRepo(path) }},
				{title: w.t(i18n.ShowInFileManager), icon: icons.Folder, run: func() { revealPath(path) }},
				{title: w.t(i18n.WelcomeCopyPath), icon: icons.Copy, run: func() { w.copyText(path) }},
				sep(),
				{title: w.t(i18n.WelcomeRemoveRecent), icon: icons.Close, run: func() { w.ws.Settings.RemoveRecent(path); w.saveSettings() }},
			})
		})
	}
	w.c.Unclip()
	w.vbar(scLogScrollbars.At(30), list, &v.scroll, content)
}

func (w *Window) paintWelcomeEmpty(r ui.Rect) {
	cy := r.Y + r.Height*0.3
	w.p.textCenter("Welcome to Viff", rect(r.X, cy, r.Width, 30), 20, semibold, w.pal.Text)
	w.p.textCenter("Open a Git repository, clone one, or create a new one.", rect(r.X, cy+34, r.Width, 22), fontUI, regular, w.pal.TextMuted)
	big := []struct {
		label string
		icon  icons.Pair
		id    ui.ID
		fn    func()
	}{{"New Repository", icons.Add, idWelInit, w.openInit}, {"Open", icons.Open, idWelOpen, w.chooseRepoNow}, {"Clone", icons.Download, idWelClone, w.openClone}, {"Repositories…", icons.Project, idWelcomeRepositories, w.openRepositories}}
	bw := float32(120)
	x := r.X + (r.Width-4*bw-3*24)/2
	y := cy + 84
	for _, b := range big {
		br := rect(x, y, bw, 76)
		box := rect(x+(bw-52)/2, y, 52, 52)
		bg := w.pal.ControlRaised
		if w.hovered(b.id) {
			bg = w.pal.ToolPressed
		}
		w.p.outline(box, 12, 1, w.pal.ControlBorder, bg)
		w.p.icon(b.icon, box.X+16, box.Y+16, 20)
		w.p.textCenter(b.label, rect(x, y+56, bw, 20), fontUI, regular, w.pal.TextMuted)
		w.c.HitWithOptions(b.id, br, b.label, ui.HitOptions{Role: ui.RoleButton})
		if w.c.Clicked(b.id) {
			fn := b.fn
			w.after(fn)
		}
		x += bw + 24
	}
	w.p.textCenter(w.t(i18n.WelcomeDropHint), rect(r.X, y+110, r.Width, 20), fontSmall, regular, w.pal.TextDisable)
}

func (w *Window) paintCustomize(r ui.Rect) {
	s := w.ws.Settings
	f := &form{w: w, x: r.X, y: r.Y, width: min(r.Width, 520), labelW: 140}
	f.bold(w.t(i18n.WelcomeCustomize))
	themeTitle := map[string]string{"dark": "Islands Dark", "light": "Islands Light", "system": "Sync with OS"}
	f.combo("Color theme:", themeTitle[s.Theme], func() []item {
		var items []item
		for _, k := range []string{"dark", "light", "system"} {
			k := k
			items = append(items, item{title: themeTitle[k], checked: s.Theme == k, run: func() { s.Theme = k; w.applyTheme(); w.saveSettings() }})
		}
		return items
	})
	f.custom(36, func(rr ui.Rect) {
		if w.button(idWelTheme, rect(rr.X+140, rr.Y, 140, buttonH), "All Settings…", btnNormal, false) {
			w.after(w.openSettings)
		}
	})
}

var _ = app.OpenURL
