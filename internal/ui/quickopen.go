package viffui

import (
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

const (
	searchPageFiles = iota
	searchPageContents

	// searchDebounce keeps a fast typist from starting one Git process per
	// keystroke. The timer only sets a flag; the query itself starts on the UI
	// thread after the next paint.
	searchDebounce = 180 * time.Millisecond
)

// contentSearch is the Contents page's view state. The hits come from the
// workspace's read query and are never written back to the repository.
type contentSearch struct {
	query          string
	hits           []vapp.SearchHit
	truncated      bool
	loading        bool
	err            error
	ripgrepMissing bool
	sel            int
	scroll         scroller
	viewH          float32

	// gen invalidates the pending debounce timer when the text changes again.
	// due is written by the timer goroutine and read on the UI thread. token
	// drops callbacks of a query the user has already replaced.
	gen   atomic.Uint64
	due   atomic.Bool
	token uint64
}

// Open File is the keyboard-first way into the working tree. Its two pages
// share one dialog: Files filters every path, Contents searches the text of
// the working tree. Both open matches read-only in the same viewer, so the
// dialog adds no second source of truth.
func (w *Window) openQuickOpen() { w.openSearchDialog(searchPageFiles) }

// openFindInFiles opens the same dialog on its Contents page.
func (w *Window) openFindInFiles() { w.openSearchDialog(searchPageContents) }

func (w *Window) openSearchDialog(page int) {
	if w.repo() == nil {
		return
	}
	w.ensureFiles()
	v := &w.fm
	filter := text.NewEditor("")
	content := text.NewEditor("")
	st := &contentSearch{query: "\x00", sel: -1}

	// ---- Files page ----
	query := "\x00"
	built := false
	builtSeq := uint64(0)
	var shown []int
	sel := -1
	scroll := &scroller{}
	viewH := float32(0)

	apply := func() {
		q := strings.ToLower(strings.TrimSpace(filter.String()))
		query = q
		shown = shown[:0]
		for i, p := range v.entries {
			if q == "" || strings.Contains(strings.ToLower(p), q) {
				shown = append(shown, i)
			}
		}
		if sel >= len(shown) {
			sel = len(shown) - 1
		}
		if sel >= len(shown) {
			sel = -1
		}
		if sel < 0 && len(shown) > 0 {
			sel = 0
		}
		scroll.Y = 0
	}
	openFile := func() {
		if sel < 0 || sel >= len(shown) || shown[sel] >= len(v.entries) {
			return
		}
		p := v.entries[shown[sel]]
		w.dialog = nil
		w.openRepoFile(p, true)
	}
	reveal := func() {
		if sel >= 0 {
			scroll.reveal(float32(sel)*rowH, rowH, viewH)
		}
	}

	// ---- Contents page ----
	openHit := func() {
		if st.sel < 0 || st.sel >= len(st.hits) {
			return
		}
		hit := st.hits[st.sel]
		w.dialog = nil
		w.openRepoFileAt(hit.Path, hit.Line, true)
	}
	moveHit := func(to int) {
		n := len(st.hits)
		if n == 0 {
			return
		}
		st.sel = max(0, min(n-1, to))
		st.scroll.reveal(float32(st.sel)*rowH, rowH, st.viewH)
		w.c.RequestFrame()
	}

	initialFocus := idQuickInput
	if page == searchPageContents {
		initialFocus = idSearchInput
	}
	d := &dialog{
		title: w.t(i18n.SearchTitle),
		ok:    w.t(i18n.SearchOpenButton),
		width: 680,
		focus: initialFocus,
		body: func(f *form) {
			dialogTabs(f, &page,
				[]string{w.t(i18n.SearchTabFiles), w.t(i18n.SearchTabContents)},
				func(i int) {
					if i == searchPageContents {
						w.focusDialogField(idSearchInput)
					} else {
						w.focusDialogField(idQuickInput)
					}
				})

			if page == searchPageFiles {
				f.custom(30, func(r ui.Rect) {
					if w.field(idQuickInput, filter, r, fieldOpts{icon: icons.Search, placeholder: w.t(i18n.SearchPlaceholderFiles), nav: true}) {
						w.submitDialog()
					}
					w.onKeys(idQuickInput, func(e ui.KeyEvent) bool {
						switch e.Key {
						case ui.KeyDown:
							if len(shown) > 0 {
								sel = min(sel+1, len(shown)-1)
								reveal()
								w.c.RequestFrame()
							}
							return true
						case ui.KeyUp:
							if len(shown) > 0 {
								sel = max(sel-1, 0)
								reveal()
								w.c.RequestFrame()
							}
							return true
						case ui.KeyHome:
							if len(shown) > 0 {
								sel = 0
								reveal()
								w.c.RequestFrame()
							}
							return true
						case ui.KeyEnd:
							if len(shown) > 0 {
								sel = len(shown) - 1
								reveal()
								w.c.RequestFrame()
							}
							return true
						}
						return false
					})
				})
				f.gap(10)
				f.custom(340, func(lr ui.Rect) {
					w.p.outline(lr, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
					inner := inset(lr, 1, 1)
					viewH = inner.Height
					q := strings.ToLower(strings.TrimSpace(filter.String()))
					if v.loaded && (!built || q != query || builtSeq != v.seq) {
						apply()
						built = true
						builtSeq = v.seq
					}
					switch {
					case !v.loaded:
						w.p.textCenter(w.t(i18n.WorkspaceStateLoading), inner, fontUI, regular, w.pal.TextSecond)
						return
					case v.err != nil:
						w.p.textCenter(vapp.ErrorText(v.err), inner, fontUI, regular, w.pal.Error)
						return
					case len(v.entries) == 0:
						w.p.textCenter("The working tree has no files", inner, fontUI, regular, w.pal.TextSecond)
						return
					case len(shown) == 0:
						w.p.textCenter("No files match the search", inner, fontUI, regular, w.pal.TextSecond)
						return
					}
					n := len(shown)
					content := float32(n)*rowH + 8
					scroll.clamp(content, inner.Height)
					w.onScrollIn(inner, func(dx, dy float32) {
						scroll.Y += dy
						scroll.clamp(content, inner.Height)
					})
					w.c.Clip(inner)
					first := max(0, int(scroll.Y/rowH)-1)
					last := min(n, first+int(inner.Height/rowH)+3)
					for k := first; k < last; k++ {
						p := v.entries[shown[k]]
						y := inner.Y + 4 + float32(k)*rowH - scroll.Y
						row := rect(inner.X+4, y, inner.Width-8, rowH)
						id := scQuickRows.At(k)
						if k == sel {
							w.p.round(row, 4, w.pal.Selection)
						} else if w.hovered(id) {
							w.p.round(row, 4, w.pal.Hover)
						}
						x := row.X + 8
						w.p.iconIn(fileIcon(p), rect(x, y, 16, rowH))
						x += 22
						tw := max(0, right(row)-x-8)
						w.p.text(w.p.truncateMiddle(p, fontUI, regular, tw), rect(x, y, tw, rowH), fontUI, regular, w.pal.Text)
						w.c.HitWithOptions(id, row, p, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: k == sel})
						if w.c.Clicked(id) {
							k := k
							w.after(func() {
								sel = k
								reveal()
							})
						}
						w.onDouble(id, func() {
							sel = k
							openFile()
						})
					}
					w.c.Unclip()
					w.vbar(idQuickBar, inner, scroll, content)
				})
			} else {
				f.custom(30, func(r ui.Rect) {
					if w.field(idSearchInput, content, r, fieldOpts{icon: icons.Search, placeholder: w.t(i18n.SearchPlaceholderContents), nav: true}) {
						w.submitDialog()
					}
					w.onKeys(idSearchInput, func(e ui.KeyEvent) bool {
						switch e.Key {
						case ui.KeyDown:
							moveHit(st.sel + 1)
						case ui.KeyUp:
							moveHit(st.sel - 1)
						case ui.KeyHome:
							moveHit(0)
						case ui.KeyEnd:
							moveHit(len(st.hits) - 1)
						case ui.KeyPageDown:
							moveHit(st.sel + max(1, int(st.viewH/rowH)-1))
						case ui.KeyPageUp:
							moveHit(st.sel - max(1, int(st.viewH/rowH)-1))
						default:
							return false
						}
						return true
					})
				})
				f.gap(10)
				f.custom(340, func(lr ui.Rect) {
					w.p.outline(lr, 6, 1, w.pal.ControlBorder, w.pal.ControlBg)
					inner := inset(lr, 1, 1)
					st.viewH = inner.Height
					q := strings.TrimSpace(content.String())
					if q != st.query {
						w.restartContentSearch(st, q)
					}
					if st.due.Swap(false) {
						w.after(func() { w.startContentSearch(st) })
					}
					switch {
					case q == "":
						w.p.textCenter(w.t(i18n.SearchEmptyContents), inner, fontUI, regular, w.pal.TextSecond)
						return
					case st.ripgrepMissing:
						w.p.textCenter(w.t(i18n.SearchRipgrepMissing), inner, fontUI, regular, w.pal.TextSecond)
						return
					case st.err != nil:
						w.p.textCenter(vapp.ErrorText(st.err), inner, fontUI, regular, w.pal.Error)
						return
					case st.loading:
						// The status line below carries the progress text; the
						// result area stays quiet until hits arrive.
					case len(st.hits) == 0:
						w.p.textCenter(w.t(i18n.SearchNoMatches), inner, fontUI, regular, w.pal.TextSecond)
						return
					}
					if len(st.hits) == 0 {
						return
					}
					n := len(st.hits)
					listContent := float32(n)*rowH + 8
					st.scroll.clamp(listContent, inner.Height)
					w.onScrollIn(inner, func(dx, dy float32) {
						st.scroll.Y += dy
						st.scroll.clamp(listContent, inner.Height)
					})
					w.c.Clip(inner)
					first := max(0, int(st.scroll.Y/rowH)-1)
					last := min(n, first+int(inner.Height/rowH)+3)
					for k := first; k < last; k++ {
						hit := st.hits[k]
						y := inner.Y + 4 + float32(k)*rowH - st.scroll.Y
						row := rect(inner.X+4, y, inner.Width-8, rowH)
						id := scQuickRows.At(k)
						if k == st.sel {
							w.p.round(row, 4, w.pal.Selection)
						} else if w.hovered(id) {
							w.p.round(row, 4, w.pal.Hover)
						}
						x := row.X + 8
						pw := clampF(row.Width*0.4, 100, 260)
						label := hit.Path + ":" + strconv.Itoa(hit.Line)
						w.p.text(w.p.truncateMiddle(label, fontUI, regular, pw), rect(x, y, pw, rowH), fontUI, regular, w.pal.TextSecond)
						tx := x + pw + 10
						tw := max(0, right(row)-tx-8)
						w.paintSearchSnippet(expandLine(hit.Text), content.String(), rect(tx, y, tw, rowH))
						w.c.HitWithOptions(id, row, label, ui.HitOptions{Role: ui.RoleOption, SkipFocus: true, Selected: k == st.sel})
						if w.c.Clicked(id) {
							k := k
							w.after(func() {
								st.sel = k
								st.scroll.reveal(float32(k)*rowH, rowH, st.viewH)
							})
						}
						w.onDouble(id, func() {
							st.sel = k
							openHit()
						})
					}
					w.c.Unclip()
					w.vbar(idQuickBar, inner, &st.scroll, listContent)
				})
				f.gap(4)
				f.custom(20, func(r ui.Rect) {
					if s := w.contentSearchStatus(st); s != "" {
						w.p.text(s, r, fontSmall, regular, w.pal.TextSecond)
					}
				})
			}
			f.gap(4)
			f.custom(26, func(r ui.Rect) {
				hint := i18n.SearchHintFiles
				if page == searchPageContents {
					hint = i18n.SearchHintContents
				}
				label := w.t(i18n.SearchSettingsButton)
				bw := w.buttonWidth(label)
				w.p.text(w.t(hint), rect(r.X, r.Y+4, max(0, r.Width-bw-12), 20), fontSmall, regular, w.pal.TextSecond)
				if w.button(idSearchSettings, rect(right(r)-bw, r.Y, bw, 24), label, btnNormal, false) {
					w.after(func() {
						w.dialog = nil
						w.openSettingsSearch()
					})
				}
			})
		},
		okDisabled: func() bool {
			if page == searchPageContents {
				return st.sel < 0 || st.sel >= len(st.hits)
			}
			return sel < 0 || sel >= len(shown)
		},
		onOK: func() bool {
			if page == searchPageContents {
				openHit()
			} else {
				openFile()
			}
			return true
		},
	}

	w.openDialog(d)
	if page == searchPageContents {
		w.focusDialogField(idSearchInput)
	} else {
		w.focusDialogField(idQuickInput)
	}
}

// focusDialogField moves the native text input to a field after the page that
// contains it is painted. Requesting focus immediately would target the hit
// tree of the frame that is still painting, where a just-switched page has no
// field yet; the deferred call sets focusNext for the next frame instead.
func (w *Window) focusDialogField(id ui.ID) {
	w.after(func() {
		if w.dialog == nil {
			return
		}
		w.focusNext = id
		w.Invalidate()
	})
}

// restartContentSearch clears the previous answer and debounces the new query.
// A stale timer simply loses its generation check.
func (w *Window) restartContentSearch(st *contentSearch, q string) {
	st.query = q
	st.hits = nil
	st.truncated = false
	st.err = nil
	st.ripgrepMissing = false
	st.sel = -1
	st.scroll.Y = 0
	st.due.Store(false)
	if q == "" {
		st.loading = false
		st.gen.Add(1)
		return
	}
	st.loading = true
	gen := st.gen.Add(1)
	time.AfterFunc(searchDebounce, func() {
		if st.gen.Load() != gen {
			return
		}
		st.due.Store(true)
		w.Invalidate()
	})
}

// startContentSearch runs the query on the UI thread; the workspace cancels
// the previous one and posts its answer back here.
func (w *Window) startContentSearch(st *contentSearch) {
	q := st.query
	if q == "" || w.dialog == nil {
		return
	}
	st.token++
	token := st.token
	st.loading = true
	w.ws.SearchContent(q, vapp.SearchContentOptions{
		Tool:        w.ws.Settings.SearchTool,
		RipgrepPath: w.ws.Settings.RipgrepPath,
	}, func(res vapp.SearchResult, err error) {
		if token != st.token || st.query != q {
			return // a newer query owns the dialog
		}
		st.loading = false
		st.err = err
		st.ripgrepMissing = errors.Is(err, vapp.ErrRipgrepMissing)
		st.hits = res.Hits
		st.truncated = res.Truncated
		st.sel = -1
		if len(res.Hits) > 0 {
			st.sel = 0
		}
		st.scroll.Y = 0
	})
}

// contentSearchStatus is the line under the result list.
func (w *Window) contentSearchStatus(st *contentSearch) string {
	switch {
	case st.truncated:
		return w.t(i18n.SearchTruncated, len(st.hits))
	case st.loading:
		return w.t(i18n.SearchLoading)
	case len(st.hits) > 0:
		return w.pluralT(len(st.hits), i18n.SearchResultsOne, i18n.SearchResultsMany)
	}
	return ""
}

// paintSearchSnippet draws one matching line and marks the query when both
// strings are plain ASCII; non-ASCII text keeps the match unmarked rather than
// risking a wrong byte offset.
func (w *Window) paintSearchSnippet(line, query string, r ui.Rect) {
	if query != "" && asciiOnly(line) && asciiOnly(query) {
		if i := strings.Index(strings.ToLower(line), strings.ToLower(query)); i >= 0 {
			x0 := r.X + w.p.width(line[:i], fontUI, regular)
			x1 := x0 + w.p.width(line[i:i+len(query)], fontUI, regular)
			x0, x1 = max(x0, r.X), min(x1, right(r))
			if x1 > x0 {
				w.p.round(rect(x0, r.Y+2, x1-x0, r.Height-4), 3, w.pal.SearchMatch.WithAlpha(0.6))
			}
		}
	}
	w.p.text(line, r, fontUI, regular, w.pal.Text)
}
