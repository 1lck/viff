// Package viffui is Viff's Viva interface: the IntelliJ-style window, its
// tool windows, the diff viewer and the dialogs. It reads state from
// internal/app and asks it to act; it never builds Git commands or reads the
// Git directory itself.
package viffui

import (
	"github.com/vivaui/viva/ui"
)

// Palette is the set of colour tokens of one theme, named after the IntelliJ
// Islands theme keys they come from (platform/platform-resources/src/themes/
// islands/ManyIslands*.theme.json and the matching editor schemes). Views use
// tokens only, never literal colours.
type Palette struct {
	Dark bool

	Frame       ui.Color // main-window-bg: toolbar, stripes, status bar, gaps
	Island      ui.Color // tool-window-bg / editor-bg
	IslandAlt   ui.Color // tool-window-bg-inline
	Border      ui.Color // tool-window-border: header separators
	Separator   ui.Color // popup-border / Separator.separatorColor
	Text        ui.Color // text-default
	TextMuted   ui.Color // text-muted
	TextSecond  ui.Color // text-secondary
	TextDisable ui.Color // text-disabled
	Link        ui.Color // text-link
	Error       ui.Color
	Warning     ui.Color
	Success     ui.Color
	EditorText  ui.Color

	Hover         ui.Color // selection-bg-hovered (lists)
	ToolHover     ui.Color // toolbar-bg-hovered
	ToolPressed   ui.Color // toolbar-bg-pressed
	Selection     ui.Color // selection-bg-active
	SelectionInac ui.Color // selection-bg-inactive
	SelectedTool  ui.Color // toolbar-selected-bg-active (stripe button)
	SelectedSoft  ui.Color // toolbar-selected-bg

	Popup       ui.Color
	PopupBorder ui.Color
	Dialog      ui.Color
	Tooltip     ui.Color
	TooltipText ui.Color

	ControlBg     ui.Color
	ControlBorder ui.Color
	ControlRaised ui.Color
	Focus         ui.Color // control-brand-border
	FocusRing     ui.Color // focus outline glow
	Brand         ui.Color // control-brand-bg (default button)
	BrandText     ui.Color

	TabSelected       ui.Color
	TabSelectedBorder ui.Color
	TabSelectedInac   ui.Color

	Banner            ui.Color
	BannerBorder      ui.Color
	BannerWarn        ui.Color
	BannerWarnBorder  ui.Color
	BannerError       ui.Color
	BannerErrorBorder ui.Color

	Notification ui.Color
	Counter      ui.Color
	CounterText  ui.Color

	ScrollThumb      ui.Color
	ScrollThumbHover ui.Color

	// Editor scheme.
	LineNumber      ui.Color
	LineNumberCaret ui.Color
	CaretRow        ui.Color
	IndentGuide     ui.Color
	DiffInserted    ui.Color
	DiffDeleted     ui.Color
	DiffModified    ui.Color
	DiffConflict    ui.Color
	DiffSeparator   ui.Color
	SearchMatch     ui.Color
	TextSelection   ui.Color

	// Syntax.
	SynKeyword  ui.Color
	SynString   ui.Color
	SynNumber   ui.Color
	SynComment  ui.Color
	SynFunction ui.Color
	SynMeta     ui.Color
	SynConstant ui.Color
	SynTag      ui.Color

	// File status (FILESTATUS_*).
	StatusAdded    ui.Color
	StatusModified ui.Color
	StatusDeleted  ui.Color
	StatusUnknown  ui.Color
	StatusConflict ui.Color
	StatusIgnored  ui.Color
	StatusRenamed  ui.Color

	// VCS log.
	RefHead          ui.Color
	RefLocal         ui.Color
	RefRemote        ui.Color
	RefTag           ui.Color
	RefLabelBg       ui.Color
	CurrentBranchRow ui.Color
	Graph            [8]ui.Color
}

// DarkPalette is Islands Dark.
func DarkPalette() *Palette {
	c := ui.RGB24
	a := func(rgb uint32, alpha uint8) ui.Color { return ui.RGB24(rgb).WithAlpha(float32(alpha) / 255) }
	return &Palette{
		Dark:  true,
		Frame: c(0x26282C), Island: c(0x191A1C), IslandAlt: c(0x212326), Border: c(0x26282C), Separator: c(0x33353B),
		Text: c(0xD1D3D9), TextMuted: c(0x9FA2A8), TextSecond: c(0x73767C), TextDisable: c(0x4C4F56),
		Link: c(0x71A1FE), Error: c(0xF57E84), Warning: c(0xD59637), Success: c(0x6DB083), EditorText: c(0xBCBEC4),
		Hover: a(0xFFFFFF, 0x10), ToolHover: a(0xFFFFFF, 0x17), ToolPressed: a(0xFFFFFF, 0x29),
		Selection: c(0x2A4371), SelectionInac: c(0x33353B), SelectedTool: c(0x3871E1), SelectedSoft: c(0x2E4D89),
		Popup: c(0x26282C), PopupBorder: c(0x33353B), Dialog: c(0x191A1C), Tooltip: c(0x33353B), TooltipText: c(0xD1D3D9),
		ControlBg: c(0x191A1C), ControlBorder: c(0x40434A), ControlRaised: c(0x26282C),
		Focus: c(0x3871E1), FocusRing: a(0x3871E1, 0x80), Brand: c(0x3871E1), BrandText: c(0xFFFFFF),
		TabSelected: c(0x233558), TabSelectedBorder: c(0x2E4D89), TabSelectedInac: c(0x26282C),
		Banner: c(0x233558), BannerBorder: c(0x2E4D89), BannerWarn: c(0x44321D), BannerWarnBorder: c(0x694820),
		BannerError: c(0x56272B), BannerErrorBorder: c(0x80383E),
		Notification: c(0x33353B), Counter: c(0x73767C), CounterText: c(0x000000),
		ScrollThumb: a(0xFFFFFF, 0x26), ScrollThumbHover: a(0xFFFFFF, 0x4D),
		LineNumber: c(0x4B5059), LineNumberCaret: c(0xA1A3AB), CaretRow: c(0x1F2024), IndentGuide: c(0x323438),
		DiffInserted: c(0x294436), DiffDeleted: c(0x484A4A), DiffModified: c(0x385570), DiffConflict: c(0x45302B),
		DiffSeparator: c(0x2B2D30), SearchMatch: c(0x114957), TextSelection: c(0x214283),
		SynKeyword: c(0xCF8E6D), SynString: c(0x6AAB73), SynNumber: c(0x2AACB8), SynComment: c(0x7A7E85),
		SynFunction: c(0x56A8F5), SynMeta: c(0xB3AE60), SynConstant: c(0xC77DBB), SynTag: c(0xD5B778),
		StatusAdded: c(0x73BD79), StatusModified: c(0x70AEFF), StatusDeleted: c(0x6F737A), StatusUnknown: c(0xE88F89),
		StatusConflict: c(0xDE6A66), StatusIgnored: c(0xD69A6B), StatusRenamed: c(0x70AEFF),
		RefHead: c(0xF5D273), RefLocal: c(0x5FAD65), RefRemote: c(0xB589EC), RefTag: c(0x868A91),
		RefLabelBg: c(0x2B2D30), CurrentBranchRow: c(0x1D2336),
		Graph: [8]ui.Color{c(0x3D7DEB), c(0x5FAD65), c(0xC77DBB), c(0xE5A54B), c(0x2AACB8), c(0xE36D74), c(0xA2D06C), c(0x9C7BDB)},
	}
}

// LightPalette is Islands Light.
func LightPalette() *Palette {
	c := ui.RGB24
	a := func(rgb uint32, alpha uint8) ui.Color { return ui.RGB24(rgb).WithAlpha(float32(alpha) / 255) }
	return &Palette{
		Frame: c(0xE9EAEE), Island: c(0xFFFFFF), IslandAlt: c(0xF7F8F9), Border: c(0xE9EAEE), Separator: c(0xE9EAEE),
		Text: c(0x000000), TextMuted: c(0x5F6269), TextSecond: c(0x73767C), TextDisable: c(0x9FA2A8),
		Link: c(0x2F5EB9), Error: c(0xC54E58), Warning: c(0xA56906), Success: c(0x338555), EditorText: c(0x080808),
		Hover: a(0x000000, 0x08), ToolHover: a(0x000000, 0x12), ToolPressed: a(0x000000, 0x20),
		Selection: c(0xD0DFFE), SelectionInac: c(0xE9EAEE), SelectedTool: c(0x3871E1), SelectedSoft: c(0xD0DFFE),
		Popup: c(0xFFFFFF), PopupBorder: c(0xE9EAEE), Dialog: c(0xF7F8F9), Tooltip: c(0xFFFFFF), TooltipText: c(0x000000),
		ControlBg: c(0xFFFFFF), ControlBorder: c(0xD1D3D9), ControlRaised: c(0xFFFFFF),
		Focus: c(0x3871E1), FocusRing: a(0x3871E1, 0x60), Brand: c(0x3871E1), BrandText: c(0xFFFFFF),
		TabSelected: c(0xE3EBFE), TabSelectedBorder: c(0xA7C5FF), TabSelectedInac: c(0xE9EAEE),
		Banner: c(0xF7F8FF), BannerBorder: c(0xBDD3FF), BannerWarn: c(0xFFF6E9), BannerWarnBorder: c(0xF4CD9A),
		BannerError: c(0xFFF6F5), BannerErrorBorder: c(0xFFC4C5),
		Notification: c(0xFFFFFF), Counter: c(0x8B8E94), CounterText: c(0xFFFFFF),
		ScrollThumb: a(0x000000, 0x33), ScrollThumbHover: a(0x000000, 0x59),
		LineNumber: c(0xAEB3C2), LineNumberCaret: c(0x767A8A), CaretRow: c(0xF5F8FE), IndentGuide: c(0xEBECF0),
		DiffInserted: c(0xBEE6BE), DiffDeleted: c(0xD6D6D6), DiffModified: c(0xC2D8F2), DiffConflict: c(0xFFD5CC),
		DiffSeparator: c(0xF0F0F0), SearchMatch: c(0xFCD47E), TextSelection: c(0xA6D2FF),
		SynKeyword: c(0x0033B3), SynString: c(0x067D17), SynNumber: c(0x1750EB), SynComment: c(0x8C8C8C),
		SynFunction: c(0x00627A), SynMeta: c(0x9E880D), SynConstant: c(0x871094), SynTag: c(0x0033B3),
		StatusAdded: c(0x067D17), StatusModified: c(0x0033B3), StatusDeleted: c(0x6C707E), StatusUnknown: c(0xB23247),
		StatusConflict: c(0xDE1B2E), StatusIgnored: c(0x8C4F00), StatusRenamed: c(0x0033B3),
		RefHead: c(0xE0A200), RefLocal: c(0x3E8C49), RefRemote: c(0x8E5BD1), RefTag: c(0x868A91),
		RefLabelBg: c(0xEBECF0), CurrentBranchRow: c(0xF0F4FF),
		Graph: [8]ui.Color{c(0x3574F0), c(0x3E8C49), c(0xA651A0), c(0xD2861B), c(0x15918F), c(0xD7414C), c(0x6E9A2C), c(0x7553C9)},
	}
}

// Metrics are the IntelliJ new UI sizes in logical pixels.
const (
	fontUI       = 13
	fontSmall    = 12
	fontTiny     = 11
	toolbarH     = 40
	stripeW      = 40
	statusH      = 28
	headerH      = 40
	tabsH        = 40
	rowH         = 24
	islandGap    = 6
	islandRadius = 10
	buttonH      = 28
	fieldH       = 28
	radiusCtl    = 4
	radiusPopup  = 8
	iconSize     = 16
	actionSize   = 26
)

// viewTheme maps the palette onto Viva's theme for the few Viva components
// used (the multi-line commit message editor and spinners).
func (p *Palette) viewTheme() ui.Theme {
	t := ui.DarkTheme()
	if !p.Dark {
		t = ui.LightTheme()
	}
	t.Background = p.Island
	t.Foreground = p.Text
	t.Card = p.Island
	t.CardForeground = p.Text
	t.Popover = p.Popup
	t.PopoverForeground = p.Text
	t.Muted = p.IslandAlt
	t.MutedForeground = p.TextSecond
	t.Border = p.ControlBorder
	t.Input = p.ControlBorder
	t.Primary = p.Brand
	t.PrimaryForeground = p.BrandText
	t.Selection = p.TextSelection
	t.SelectionForeground = p.Text
	t.Ring = p.Focus
	t.Hover = p.ToolHover
	t.Accent = p.ToolHover
	t.AccentForeground = p.Text
	t.Metrics.FontSizeM = fontUI
	t.Metrics.FontSizeS = fontSmall
	t.Metrics.ControlHeight = fieldH
	t.Metrics.RadiusControl = radiusCtl
	t.Metrics.PadControlX = 9
	return t
}
