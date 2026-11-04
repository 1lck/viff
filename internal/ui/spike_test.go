//go:build vivahead

package viffui

import (
	"os"
	"testing"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"

	"viff/internal/platform"
	"viff/internal/ui/icons"
)

func TestSpike(t *testing.T) {
	out := os.Getenv("VIFF_SHOT")
	if out == "" {
		t.Skip("set VIFF_SHOT")
	}
	platform.RegisterFonts()
	pal := DarkPalette()
	mono := ui.MonoFamily("JetBrains Mono")
	cfg := app.Config{Title: "spike", Width: 600, Height: 260, Paint: func(c *ui.Canvas, size ui.Size) {
		p := &pen{c: c, pal: pal, mono: mono}
		p.fill(rect(0, 0, size.Width, size.Height), pal.Frame)
		isl := rect(46, 46, 540, 200)
		p.round(isl, islandRadius, pal.Island)
		row := rect(isl.X, isl.Y+10, isl.Width, rowH)
		p.fill(row, pal.Selection)
		p.icon(icons.ChevronDown, row.X+8, row.Y+4, 16)
		p.icon(icons.Folder, row.X+28, row.Y+4, 16)
		p.text("Changes  agpqy 中文", rect(row.X+50, row.Y, 300, row.Height), fontUI, regular, pal.Text)
		p.text("3 files", rect(row.X+200, row.Y, 100, row.Height), fontUI, regular, pal.TextSecond)
		p.fill(rect(row.X+300, row.Y, 1, row.Height), pal.Error)
		p.fill(rect(row.X+300, row.Y+12, 200, 1), pal.Error)
		code := rect(isl.X+10, isl.Y+50, 500, 20)
		p.fill(code, pal.DiffInserted)
		p.monoText("func main() { fmt.Println(\"héllo\") } // 中文", code, 13, pal.EditorText)
		p.text("Commit", rect(isl.X+10, isl.Y+90, 100, 40), fontUI, semibold, pal.Text)
		for i, ic := range []icons.Pair{icons.Commit, icons.Vcs, icons.Refresh, icons.Settings, icons.Search, icons.Push, icons.Fetch, icons.Java, icons.Json, icons.BranchLabel, icons.CurrentBranchLabel, icons.Tag} {
			p.icon(ic, isl.X+10+float32(i)*26, isl.Y+140, 16)
		}
		p.icon(icons.Commit, isl.X+10, isl.Y+165, 20)
		for i, ic := range []icons.Pair{icons.Folder, icons.Text, icons.Markdown, icons.Image, icons.Add, icons.Close, icons.Delete, icons.Abort, icons.Undo, icons.Diff, icons.CherryPick, icons.Incoming, icons.Outgoing, icons.Shelve, icons.Star, icons.Error, icons.Warning, icons.Success, icons.Info} {
			p.icon(ic, isl.X+40+float32(i)*24, isl.Y+169, 16)
		}
	}}
	s, err := headless.Open(cfg, nil, headless.Options{Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Screenshot(out); err != nil {
		t.Fatal(err)
	}
	t.Log(app.MonospaceFamilies())
}
