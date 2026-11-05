package viffui

import "testing"

// The status item's menu offers the way back and the way out.
func TestTrayMenu(t *testing.T) {
	w := &Window{}
	quit := false
	w.quit = func() { quit = true }

	items := w.TrayMenu()
	if len(items) != 3 {
		t.Fatalf("menu has %d entries, want 3", len(items))
	}
	if items[0].Title != "Show Viff" || items[0].OnSelect == nil {
		t.Fatalf("first entry = %+v, want Show Viff", items[0])
	}
	if !items[1].Separator {
		t.Fatalf("second entry is not a separator: %+v", items[1])
	}
	if items[2].Title != "Quit Viff" || items[2].OnSelect == nil {
		t.Fatalf("last entry = %+v, want Quit Viff", items[2])
	}
	items[2].OnSelect()
	if !quit {
		t.Fatal("Quit Viff did not shut the application down")
	}
}

// A headless run has no native window, so the status-item callback must stay
// harmless instead of dereferencing it.
func TestTrayCallbacksWithoutNative(t *testing.T) {
	w := &Window{}
	w.ShowFromTray()
}
