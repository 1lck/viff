//go:build vivahead

package viffui

import (
	"strings"
	"testing"
	"time"

	"github.com/vivaui/viva/app"

	"viff/internal/ai"
	"viff/internal/storage"
)

// The commit message rules live behind the AI button's right click, and the
// confirmation bubble behind its left click. These tests drive both the way the
// window does and assert stored state rather than pixels: which rule a
// repository resolves to, what reached prompts.json, and whether a keyboard can
// answer a question the window asked.

// aiButtonCenter is where a click on the AI button lands. The button is painted
// every frame, so its bounds are the ones the user actually sees.
func aiButtonCenter(t *testing.T, f *uiFixture) (float32, float32) {
	t.Helper()
	r := f.win.cm.aiRect
	if r.Width <= 0 || r.Height <= 0 {
		t.Fatalf("the AI button has no painted bounds: %+v", r)
	}
	return r.X + r.Width/2, r.Y + r.Height/2
}

func openPromptRulesMenu(t *testing.T, f *uiFixture) {
	t.Helper()
	x, y := aiButtonCenter(t, f)
	if err := f.s.RightClick(x, y); err != nil {
		t.Fatalf("right click the AI button: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.popups) == 0 {
		t.Fatalf("the rules menu did not open\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
}

func pendingMenuClick(t *testing.T, f *uiFixture, label string) {
	t.Helper()
	if err := f.s.ClickLabel(label); err != nil {
		t.Fatalf("click %q: %v\ntexts: %s", label, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

func waitPromptRuleDialog(t *testing.T, f *uiFixture, title string) {
	t.Helper()
	err := f.s.WaitFor(func(app.Snapshot) bool {
		return f.win.dialog != nil && f.win.dialog.title == title
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("dialog %q did not open: %v\ntexts: %s", title, err, strings.Join(f.s.Texts(), " | "))
	}
	f.s.Settle(40*time.Millisecond, time.Second)
}

// TestPromptRulesMenuCreatesAndSelectsRule walks the whole path a user takes to
// give a repository rules of its own: the right-click menu, the refused empty
// name, the created rule started from the rule in use, and the selection that
// reaches both the workspace and prompts.json.
func TestPromptRulesMenuCreatesAndSelectsRule(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	if got := f.ws.SelectedPromptRule(); !got.Builtin {
		t.Fatalf("a repository without a choice starts on the built-in rules, got %+v", got)
	}
	openPromptRulesMenu(t, f)
	if rules := f.ws.PromptRules(); len(rules) == 0 || !rules[0].Builtin {
		t.Fatalf("the built-in rule is missing from the picker: %+v", rules)
	}

	pendingMenuClick(t, f, "New Rules…")
	waitPromptRuleDialog(t, f, "New Commit Message Rules")

	// An unnamed rule is refused in the dialog, which stays open.
	pendingMenuClick(t, f, "Create")
	if f.win.dialog == nil {
		t.Fatalf("the editor closed without a name\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if !f.s.HasText("A name is required.") {
		t.Fatalf("no name problem was shown\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}

	// The name field holds the focus of a new rule, so the name can be typed
	// without walking to it: the rules text is prefilled and needs no detour.
	if err := f.s.Type("Tickets"); err != nil {
		t.Fatalf("type the rule name: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	pendingMenuClick(t, f, "Create")
	if err := f.s.WaitFor(func(app.Snapshot) bool { return f.win.dialog == nil }, 5*time.Second); err != nil {
		t.Fatalf("the editor did not close: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}

	var created string
	for _, rule := range f.ws.PromptRules() {
		if rule.Name == "Tickets" {
			created = rule.ID
			// A new rule starts from the text of the rule in use, which is the
			// built-in text while the built-in rules are selected.
			if rule.Text != ai.DefaultRules {
				t.Fatalf("new rule text = %q, want the built-in text", rule.Text)
			}
		}
	}
	if created == "" {
		t.Fatalf("the rule was not created: %+v", f.ws.PromptRules())
	}
	if got := f.ws.SelectedPromptRule(); got.ID != created {
		t.Fatalf("selected rule = %+v, want the rule just written", got)
	}

	// The selection is persisted for this repository and as the default for
	// repositories the user has not chosen for yet, so opening them later does
	// not look like an ignored choice.
	path := storage.PromptRulesPath(f.ws.Settings.Path())
	doc, err := storage.LoadPromptRules(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if doc.LastRule != created {
		t.Fatalf("lastRule = %q, want %q", doc.LastRule, created)
	}
	found := false
	for _, rule := range doc.Rules {
		if rule.ID == created {
			found = true
		}
	}
	if !found {
		t.Fatalf("the rule is missing from %s: %+v", path, doc.Rules)
	}

	f.win.Invalidate()
	f.s.Settle(40*time.Millisecond, time.Second)
	if tip := f.win.promptRuleTip(); !strings.Contains(tip, "Tickets") {
		t.Fatalf("the AI button tooltip = %q, want the rule in use", tip)
	}
	f.assertIdle()
}

// TestAIConfirmSwitchStaysOpenAndPersists keeps the switch behaving like a
// setting: clicking or pressing Enter flips it, the menu stays open so it reads
// as a control rather than a command, and the value reaches settings.json.
func TestAIConfirmSwitchStaysOpenAndPersists(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	f.win.openRepo(f.dir)
	f.settle()

	if !f.ws.Settings.AIConfirmGenerate {
		t.Fatalf("the confirmation must default on")
	}
	openPromptRulesMenu(t, f)
	pendingMenuClick(t, f, "Ask before generating")
	if len(f.win.popups) == 0 {
		t.Fatalf("the switch closed the menu; it is a setting, not a command")
	}
	if f.ws.Settings.AIConfirmGenerate {
		t.Fatalf("the switch did not turn the confirmation off")
	}
	path := f.ws.Settings.Path()
	if err := f.s.WaitFor(func(app.Snapshot) bool {
		return !storage.Load(path).AIConfirmGenerate
	}, 5*time.Second); err != nil {
		t.Fatalf("aiConfirmGenerate was not saved: %v", err)
	}

	// Up from the first row reaches the last row, the switch, and Enter flips it
	// without closing the menu.
	if err := f.s.Keys("up"); err != nil {
		t.Fatalf("select the switch row: %v", err)
	}
	if err := f.s.Keys("enter"); err != nil {
		t.Fatalf("press the switch row: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if !f.ws.Settings.AIConfirmGenerate {
		t.Fatalf("a switch row cannot be flipped from the keyboard")
	}
	if len(f.win.popups) == 0 {
		t.Fatalf("Enter on the switch closed the menu")
	}
	if err := f.s.Keys("escape"); err != nil {
		t.Fatalf("close the menu: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.popups) != 0 {
		t.Fatalf("escape did not close the menu")
	}
}

// TestAIConfirmBubbleAnswersWithKeyboard covers the question the button asks by
// default: Escape sends nothing, and Enter answers it. Before the bubble could
// be answered from the keyboard, the only way to generate was the pointer.
func TestAIConfirmBubbleAnswersWithKeyboard(t *testing.T) {
	f := newUIFixture(t)
	f.start(1440, 900)
	// A closed local port: the confirmed request fails at once instead of
	// reaching a network or waiting for a timeout.
	f.ws.Settings.AIEndpoint = "http://127.0.0.1:1/v1"
	f.ws.Settings.AIModel = "test-model"
	f.ws.Settings.AIKey = "test-key"
	f.win.openRepo(f.dir)
	f.settle()

	clickAI := func() {
		t.Helper()
		x, y := aiButtonCenter(t, f)
		if err := f.s.Click(x, y); err != nil {
			t.Fatalf("click the AI button: %v", err)
		}
		f.s.Settle(40*time.Millisecond, time.Second)
	}

	clickAI()
	if len(f.win.popups) == 0 {
		t.Fatalf("the confirmation bubble did not open\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	if f.win.cm.aiBusy {
		t.Fatalf("the request started without a confirmation")
	}
	if !f.s.HasText("Generate a commit message with AI?") {
		t.Fatalf("the bubble does not ask its question\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}

	// Escape is the cancel answer: nothing is sent.
	if err := f.s.Keys("escape"); err != nil {
		t.Fatalf("escape the bubble: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.popups) != 0 {
		t.Fatalf("escape did not close the bubble")
	}
	if f.win.cm.aiBusy {
		t.Fatalf("escape started a request")
	}

	// Enter is the default answer: the bubble closes and the request runs to a
	// failure notice, because the endpoint is a closed port.
	clickAI()
	if err := f.s.Keys("enter"); err != nil {
		t.Fatalf("answer the bubble: %v", err)
	}
	f.s.Settle(40*time.Millisecond, time.Second)
	if len(f.win.popups) != 0 {
		t.Fatalf("enter did not answer the bubble\ntexts: %s", strings.Join(f.s.Texts(), " | "))
	}
	err := f.s.WaitFor(func(app.Snapshot) bool {
		if f.win.cm.aiBusy {
			return false
		}
		for _, notice := range f.ws.Notifications {
			if notice.Title == "The AI could not write the message" {
				return true
			}
		}
		return false
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("the confirmed request never reported a result: %v\ntexts: %s", err, strings.Join(f.s.Texts(), " | "))
	}
}
