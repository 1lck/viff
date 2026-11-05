package viffui

import (
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/i18n"
	"viff/internal/storage"
)

// aiSettingsDraft holds the AI page while the dialog is open. Values move into
// storage.Settings only on OK, the same way the Git page works.
type aiSettingsDraft struct {
	endpoint *text.Editor
	model    *text.Editor
	key      *text.Editor
	protocol int // index into aiProtocols
}

// aiProtocols maps the radio buttons to the wire formats the generator speaks.
var aiProtocols = []string{"openai-completions", "anthropic-messages"}

func aiProtocolIndex(name string) int {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "anthropic") {
		return 1
	}
	return 0
}

func newAISettingsDraft(s *storage.Settings) *aiSettingsDraft {
	return &aiSettingsDraft{
		endpoint: text.NewEditor(s.AIEndpoint),
		model:    text.NewEditor(s.AIModel),
		key:      text.NewEditor(s.AIKey),
		protocol: aiProtocolIndex(s.AIProtocol),
	}
}

// form paints the page. It runs on every paint, so the hints stay current.
func (d *aiSettingsDraft) form(f *form) {
	f.field(f.w.t(i18n.GitAIEndpoint), d.endpoint, f.w.t(i18n.GitAIEndpointHint), false)
	f.field(f.w.t(i18n.GitAIModel), d.model, f.w.t(i18n.GitAIModelHint), false)
	f.field(f.w.t(i18n.GitAIKey), d.key, f.w.t(i18n.GitAIKeyHint), false)
	f.text(f.w.t(i18n.GitAIProtocol))
	f.radio([]string{f.w.t(i18n.GitAIProtoOpenAI), f.w.t(i18n.GitAIProtoAnthropic)}, &d.protocol)
	f.gap(6)
	f.custom(22, func(r ui.Rect) {
		f.w.p.text(f.w.t(i18n.GitAIHint), rect(r.X, r.Y, r.Width, 20), fontSmall, regular, f.w.pal.TextSecond)
	})
}

// commit writes the draft back into the settings.
func (d *aiSettingsDraft) commit(s *storage.Settings) {
	s.AIEndpoint = strings.TrimSpace(d.endpoint.String())
	s.AIModel = strings.TrimSpace(d.model.String())
	s.AIKey = strings.TrimSpace(d.key.String())
	s.AIProtocol = aiProtocols[d.protocol]
}
