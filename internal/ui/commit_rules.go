package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	vapp "viff/internal/app"
	"viff/internal/i18n"
	"viff/internal/ui/icons"
)

// The commit message prompt rules are the named texts the model receives as its
// system prompt. Viff never parses the answer, so a rule is the only place the
// shape of a message is described. The rules are persisted by the app layer,
// one set per name, and each repository remembers the one it uses.
//
// The built-in rule is supplied by the app layer and can never be edited,
// renamed or deleted: the rows that would change it stay disabled here, and the
// backend refuses such a change as well.
//
// The picker hangs off the AI button's right click, so the Amend row keeps its
// three buttons and its layout.

func (w *Window) promptRules() []vapp.PromptRule {
	if w.ws == nil {
		return nil
	}
	return w.ws.PromptRules()
}

func (w *Window) selectedPromptRule() vapp.PromptRule {
	if w.ws == nil {
		return vapp.PromptRule{}
	}
	return w.ws.SelectedPromptRule()
}

// promptRuleTip names the rule the next generated message will follow. It is the
// button's tooltip, so a repository that quietly resolves to the built-in rules
// -- because it has no selection of its own and nothing was used last -- says so
// before the user clicks, instead of leaving them to infer it from the output.
func (w *Window) promptRuleTip() string {
	rule := w.selectedPromptRule()
	if rule.Name == "" {
		return w.t(i18n.WorkspaceAIRulesRightClickHint)
	}
	return w.t(i18n.WorkspaceAITipWithRule, rule.Name)
}

// builtinPromptRule finds the rule the program ships with. It is first in the
// list the app returns, and the only one whose Builtin is set.
func (w *Window) builtinPromptRule() (vapp.PromptRule, bool) {
	for _, r := range w.promptRules() {
		if r.Builtin {
			return r, true
		}
	}
	return vapp.PromptRule{}, false
}

// openPromptRulesMenu opens the rule picker at the pointer.
func (w *Window) openPromptRulesMenu(x, y float32) {
	w.openMenu(x, y, w.promptRulesItems())
}

// promptRulesItems is the AI button's context menu: pick the rule the next
// generated message follows, or manage the rules themselves.
func (w *Window) promptRulesItems() []item {
	sel := w.selectedPromptRule()
	items := []item{header(w.t(i18n.WorkspaceAIRulesTitle))}
	for _, rule := range w.promptRules() {
		rule := rule
		// No icon here on purpose: the column is the check column, so a rule
		// shows ☑️ when it is the active one and stays blank otherwise. Text
		// still starts at the same x because the popup reserves the column.
		items = append(items, item{
			title: rule.Name, checked: rule.ID == sel.ID,
			run: func() { w.selectPromptRule(rule.ID) },
		})
	}
	builtin, hasBuiltin := w.builtinPromptRule()
	items = append(items,
		sep(),
		item{title: w.t(i18n.WorkspaceAIRulesNew), icon: icons.Add, run: func() { w.openPromptRuleEditor(vapp.PromptRule{}) }},
		item{title: w.t(i18n.WorkspaceAIRulesEdit), icon: icons.Edit, disabled: sel.Builtin, run: func() { w.openPromptRuleEditor(sel) }},
		item{title: w.t(i18n.WorkspaceAIRulesRename), icon: icons.Edit, disabled: sel.Builtin, run: func() { w.openRenamePromptRule(sel) }},
		item{title: w.t(i18n.WorkspaceAIRulesDelete), icon: icons.Delete, disabled: sel.Builtin, run: func() { w.openDeletePromptRule(sel) }},
		sep(),
		// Restoring means selecting the built-in rule again; the user's own
		// rules stay in the list, so the way back is not lost.
		item{title: w.t(i18n.WorkspaceAIRulesRestore), icon: icons.Revert, disabled: !hasBuiltin || sel.Builtin, run: func() { w.selectPromptRule(builtin.ID) }},
		sep(),
		// A real switch, not a checked row: it is a setting of this button, and
		// the user who wants it off is already looking at this menu. Flipping it
		// keeps the menu open so it reads as a control rather than a command.
		item{
			title:       w.t(i18n.WorkspaceAIConfirmToggle),
			switchOn:    w.confirmAIGenerate(),
			switchOnSet: func(on bool) { w.setAIConfirm(on) },
		},
	)
	return items
}

// confirmAIGenerate reports whether the AI button asks before it starts a
// request. An unset field keeps the documented default, true.
func (w *Window) confirmAIGenerate() bool {
	if w.ws == nil || w.ws.Settings == nil {
		return true
	}
	return w.ws.Settings.AIConfirmGenerate
}

// setAIConfirm writes the switch value the control reported and saves it with
// the other settings. It takes the target value instead of flipping the field:
// the menu already flipped what it paints from, so flipping here as well would
// put the two out of step and silently invert the setting.
func (w *Window) setAIConfirm(on bool) {
	if w.ws == nil || w.ws.Settings == nil {
		return
	}
	w.ws.Settings.AIConfirmGenerate = on
	w.saveSettings()
}

// selectPromptRule makes id the rules this repository's generated messages
// follow. The failure is reported as a notification, like every other workspace
// mutation called from a menu.
func (w *Window) selectPromptRule(id string) {
	if w.ws == nil {
		return
	}
	if err := w.ws.SelectPromptRule(id); err != nil {
		w.ws.NotifyError(w.t(i18n.WorkspaceAIRulesCannotSelect), err)
		return
	}
	w.Invalidate()
}

// openPromptRuleEditor creates a rule when rule.ID is empty and edits it
// otherwise: the same dialog, because both ask for a label and a text.
func (w *Window) openPromptRuleEditor(rule vapp.PromptRule) {
	if rule.Builtin {
		return
	}
	creating := rule.ID == ""
	start := rule.Text
	if creating {
		// A new rule starts from the text of the rule in use rather than a blank
		// box: the user edits a working starting point instead of writing a whole
		// prompt from nothing.
		start = w.selectedPromptRule().Text
	}
	name := text.NewEditor(rule.Name)
	rules := &textareaField{
		ed:          text.NewEditor(""),
		label:       w.t(i18n.WorkspaceAIRulesRules),
		placeholder: w.t(i18n.WorkspaceAIRulesPlaceholder),
	}
	setText(rules.ed, start)

	title, ok := w.t(i18n.WorkspaceAIRulesEditTitle), w.t(i18n.WorkspaceAIRulesSave)
	if creating {
		title, ok = w.t(i18n.WorkspaceAIRulesNewTitle), w.t(i18n.WorkspaceAIRulesCreate)
	}
	problem, last, focusedName := "", "", false
	w.openDialog(&dialog{
		title: title, ok: ok, width: 620,
		noClose:  true,
		textarea: idPromptRuleText,
		focus:    idPromptRuleText,
		body: func(f *form) {
			// Editing either input clears the error the last attempt left behind.
			if cur := name.String() + "\x00" + rules.ed.String(); cur != last {
				last, problem = cur, ""
			}
			nameless := strings.TrimSpace(name.String()) == ""
			nameID := f.field(w.t(i18n.WorkspaceAIRulesName), name, w.t(i18n.WorkspaceAIRulesNamePlaceholder), problem != "" && nameless)
			switch {
			case creating && !focusedName:
				// A new rule starts with an empty name and a text that is already
				// filled in, so the name is the field a keyboard user must reach
				// first. Editing an existing rule keeps the focus on its text.
				focusedName = true
				w.focusNext = nameID
			case problem != "" && nameless:
				// The refused attempt is about the empty name, so the focus goes
				// back to it instead of staying on the button that refused it and
				// leaving the answer nowhere to be typed.
				w.focusNext = nameID
			}
			f.text(w.t(i18n.WorkspaceAIRulesRules))
			f.custom(200, func(r ui.Rect) { rules.paint(w, idPromptRuleText, r) })
			if problem != "" {
				f.gap(4)
				f.message(problem, w.pal.Error)
			}
		},
		onOK: func() bool {
			n := strings.TrimSpace(name.String())
			txt := strings.TrimSpace(rules.ed.String())
			switch {
			case n == "":
				problem = w.t(i18n.WorkspaceAIRulesNameRequired)
				return false
			case txt == "":
				problem = w.t(i18n.WorkspaceAIRulesTextRequired)
				return false
			}
			saved := rule
			saved.Name, saved.Text = n, txt
			id, err := w.ws.SavePromptRule(saved)
			if err != nil {
				problem = vapp.ErrorText(err)
				return false
			}
			if creating {
				// The rule the user just wrote is the one they meant to use.
				w.selectPromptRule(id)
			}
			return true
		},
	})
}

// openRenamePromptRule changes a rule's label; its text stays as it is.
func (w *Window) openRenamePromptRule(rule vapp.PromptRule) {
	if rule.Builtin {
		return
	}
	cur := rule.Name
	name := text.NewEditor(cur)
	problem, last := "", cur
	w.openDialog(&dialog{
		title: w.t(i18n.WorkspaceAIRulesRenameTitle), ok: w.t(i18n.WorkspaceAIRulesRename), width: 420,
		body: func(f *form) {
			n := strings.TrimSpace(name.String())
			if n != last {
				last, problem = n, ""
			}
			id := f.field(w.t(i18n.WorkspaceAIRulesName), name, w.t(i18n.WorkspaceAIRulesNamePlaceholder), problem != "")
			f.w.dialog.focus = id
			if problem != "" {
				f.message(problem, w.pal.Error)
			}
		},
		onOK: func() bool {
			n := strings.TrimSpace(name.String())
			if n == "" {
				problem = w.t(i18n.WorkspaceAIRulesNameRequired)
				return false
			}
			if n == cur {
				return true
			}
			renamed := rule
			renamed.Name = n
			if _, err := w.ws.SavePromptRule(renamed); err != nil {
				problem = vapp.ErrorText(err)
				return false
			}
			return true
		},
	})
}

// openDeletePromptRule removes a rule after confirmation. The repository that
// used it falls back to the built-in rules.
func (w *Window) openDeletePromptRule(rule vapp.PromptRule) {
	if rule.Builtin {
		return
	}
	w.confirm(w.t(i18n.WorkspaceAIRulesDeleteTitle), w.t(i18n.WorkspaceAIRulesDeleteBody, rule.Name), w.t(i18n.DialogDelete), func() {
		if err := w.ws.DeletePromptRule(rule.ID); err != nil {
			w.ws.NotifyError(w.t(i18n.WorkspaceAIRulesCannotDelete), err)
			return
		}
		w.Invalidate()
	})
}
