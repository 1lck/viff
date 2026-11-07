package viffui

import (
	"time"

	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// noticeView tracks which notifications show as balloons.
type noticeView struct {
	shown  map[uint64]time.Duration // first paint of each balloon
	hidden map[uint64]bool          // balloons closed or timed out
	read   uint64                   // highest id seen in the panel
	scroll scroller
}

const balloonLife = 10 * time.Second

func (v *noticeView) unread(ws *vapp.Workspace) int {
	n := 0
	for _, x := range ws.Notifications {
		if x.ID > v.read {
			n++
		}
	}
	return n
}

func (v *noticeView) markRead(ws *vapp.Workspace) {
	for _, x := range ws.Notifications {
		v.read = max(v.read, x.ID)
	}
}

func (w *Window) noticeIcon(k vapp.NoticeKind) icons.Pair {
	switch k {
	case vapp.NoticeSuccess:
		return icons.Success
	case vapp.NoticeWarning:
		return icons.Warning
	case vapp.NoticeError:
		return icons.Error
	}
	return icons.Info
}

// paintNotices shows recent notifications as balloons at the bottom right.
func (w *Window) paintNotices(full ui.Rect) {
	v := &w.nt
	// A dialog covers the window and owns the keyboard and the pointer, so a
	// balloon must not be drawn over it: the balloon would paint above the
	// dialog and swallow a click meant for its buttons. The notice waits in the
	// workspace list and appears once the dialog is gone, with its full life.
	if w.dialog != nil {
		clear(v.shown)
		return
	}
	if v.shown == nil {
		v.shown = map[uint64]time.Duration{}
		v.hidden = map[uint64]bool{}
	}
	if w.notifyOpen {
		v.markRead(w.ws)
	}
	now := w.c.Now()
	var live []vapp.Notification
	for i := len(w.ws.Notifications) - 1; i >= 0 && len(live) < 3; i-- {
		n := w.ws.Notifications[i]
		if v.hidden[n.ID] || n.ID <= v.read {
			continue
		}
		live = append(live, n)
	}
	width := float32(380)
	x := full.Width - stripeW - width - 10
	y := full.Height - statusH - 10
	w.c.PushLayer(5)
	defer w.c.PopLayer()
	for k, n := range live {
		first, ok := v.shown[n.ID]
		if !ok {
			first = now
			v.shown[n.ID] = now
		}
		bid := scNoticeBalloon.At(k)
		sticky := n.Sticky || n.Kind == vapp.NoticeError || len(n.Actions) > 0
		if !sticky && !w.hovered(bid) {
			if left := balloonLife - (now - first); left <= 0 {
				id := n.ID
				w.after(func() { v.hidden[id] = true })
				continue
			} else {
				w.c.RequestFrameIn(left)
			}
		}
		h := w.noticeHeight(n, width)
		r := rect(x, y-h, width, h)
		y -= h + 8
		w.paintNoticeCard(n, r, k, true)
	}
}

func (w *Window) noticeBodyLines(n vapp.Notification, width float32) []string {
	if n.Body == "" {
		return nil
	}
	var out []string
	for _, para := range splitLines(n.Body) {
		if para == "" {
			out = append(out, "")
			continue
		}
		out = append(out, w.c.WrapLines(nil, para, fontUI, regular, width-56, 6)...)
		if len(out) >= 6 {
			return out[:6]
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func (w *Window) noticeHeight(n vapp.Notification, width float32) float32 {
	h := float32(14 + 20 + 12)
	h += float32(len(w.noticeBodyLines(n, width))) * 19
	if len(n.Actions) > 0 {
		h += 26
	}
	return h
}

// paintNoticeCard draws one notification (balloon or panel entry).
func (w *Window) paintNoticeCard(n vapp.Notification, r ui.Rect, k int, balloon bool) {
	bid := scNoticeBalloon.At(k)
	if !balloon {
		bid = scNoticeBalloon.At(100 + k)
	}
	if balloon {
		w.c.Shadow(r, 10, ui.ShadowStyle{OffsetY: 4, Blur: 18}, ui.Color{Alpha: 0.35})
		w.p.outline(r, 10, 1, w.pal.PopupBorder, w.pal.Notification)
	} else {
		w.p.outline(r, 8, 1, w.pal.Border, w.pal.IslandAlt)
	}
	w.c.HitWithOptions(bid, r, n.Title, ui.HitOptions{SkipFocus: true, Role: ui.RoleStatus})
	w.p.iconIn(w.noticeIcon(n.Kind), rect(r.X+12, r.Y+12, 16, 20))
	tx := r.X + 38
	title := n.Title
	if title == "" {
		title = "Viff"
	}
	w.p.text(title, rect(tx, r.Y+12, r.Width-tx+r.X-40, 20), fontUI, semibold, w.pal.Text)
	if !balloon {
		ts := n.Time.Format(w.clockLayout())
		tw := w.p.width(ts, fontSmall, regular)
		w.p.text(ts, rect(right(r)-tw-34, r.Y+12, tw+2, 20), fontSmall, regular, w.pal.TextSecond)
	}
	y := r.Y + 34
	for _, l := range w.noticeBodyLines(n, r.Width) {
		w.p.text(l, rect(tx, y, r.Width-56, 19), fontUI, regular, w.pal.TextMuted)
		y += 19
	}
	if len(n.Actions) > 0 {
		ax := tx
		for i, a := range n.Actions {
			id := scNoticeAction.At(k*8 + i)
			if !balloon {
				id = scNoticeAction.At(800 + k*8 + i)
			}
			clicked, aw := w.link(id, ax, y+4, 20, a.Title)
			if clicked {
				run, nid := a.Run, n.ID
				w.after(func() {
					w.nt.hidden[nid] = true
					w.ws.Dismiss(nid)
					if run != nil {
						run()
					}
				})
			}
			ax += aw + 18
		}
	}
	cid := scNoticeClose.At(k)
	if !balloon {
		cid = scNoticeClose.At(100 + k)
	}
	if w.hovered(bid) || w.hovered(cid) || !balloon {
		if w.action(cid, right(r)-30, r.Y+8, icons.CloseSmall, w.t(i18n.CommonClose), actOpts{size: 22}) {
			nid := n.ID
			w.after(func() {
				w.nt.hidden[nid] = true
				if !balloon {
					w.ws.Dismiss(nid)
				}
			})
		}
	}
}

// paintNotificationsPanel is the Notifications tool window.
func (w *Window) paintNotificationsPanel(r ui.Rect) {
	w.island(r)
	content, ax := w.toolHeader(r, scNoticeExpand, []string{w.t(i18n.NoticesTitle)}, 0, func(int) {})
	if w.action(scNoticeExpand.At(50), ax-actionSize, r.Y+(headerH-actionSize)/2, icons.Hide, w.t(i18n.NoticesHide), actOpts{}) {
		w.after(func() { w.notifyOpen = false })
	}
	ns := w.ws.Notifications
	if len(ns) > 0 {
		if w.action(scNoticeExpand.At(51), ax-2*actionSize-4, r.Y+(headerH-actionSize)/2, icons.Delete, w.t(i18n.NoticesClearAll), actOpts{}) {
			w.after(func() {
				for _, n := range w.ws.Notifications {
					w.nt.hidden[n.ID] = true
				}
				w.ws.Notifications = nil
			})
		}
	}
	if len(ns) == 0 {
		w.p.textCenter(w.t(i18n.NoticesEmpty), rect(content.X, content.Y+content.Height*0.35, content.Width, 22), fontUI, regular, w.pal.TextSecond)
		return
	}
	v := &w.nt
	width := content.Width - 20
	total := float32(0)
	for _, n := range ns {
		total += w.noticeHeight(n, width) + 8
	}
	v.scroll.clamp(total+10, content.Height)
	w.onScrollIn(content, func(dx, dy float32) { v.scroll.Y += dy; v.scroll.clamp(total+10, content.Height) })
	w.c.Clip(content)
	y := content.Y + 4 - v.scroll.Y
	for i := len(ns) - 1; i >= 0; i-- {
		n := ns[i]
		h := w.noticeHeight(n, width)
		if y+h > content.Y && y < bottom(content) {
			w.paintNoticeCard(n, rect(content.X+10, y, width, h), len(ns)-1-i, false)
		}
		y += h + 8
	}
	w.c.Unclip()
}
