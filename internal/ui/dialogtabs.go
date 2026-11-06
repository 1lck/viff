package viffui

import "github.com/vivaui/viva/ui"

// dialogTabs paints the page buttons of a multi-page dialog. The caller's
// body switches on the page, so the dialog keeps one height and one focus
// scope while its content changes. onSwitch runs in the click's event pass,
// after the page changed; callers use it to move the text focus.
func dialogTabs(f *form, page *int, labels []string, onSwitch func(int)) {
	f.custom(30, func(r ui.Rect) {
		x := r.X
		for i, label := range labels {
			bw := f.w.buttonWidth(label)
			kind := btnBorderless
			if i == *page {
				kind = btnDefault
			}
			if f.w.button(scDialogTabs.At(i), rect(x, r.Y, bw, 26), label, kind, false) {
				if *page != i {
					*page = i
					f.w.closePopups()
					if onSwitch != nil {
						onSwitch(i)
					}
					f.w.Invalidate()
				}
			}
			x += bw + 8
		}
	})
	f.gap(6)
}
