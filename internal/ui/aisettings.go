package viffui

import (
	"context"
	"strings"

	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"viff/internal/ai"
	"viff/internal/i18n"
	"viff/internal/storage"
)

// aiProviderPreset is one built-in provider. Like a provider in pi, it declares
// the wire formats it actually serves and the base URL they share: the protocol
// decides the path, the provider decides the host, so a provider that has no
// messages endpoint simply never offers that protocol.
type aiProviderPreset struct {
	ID        string
	Name      string
	BaseURL   string
	Protocols []string // the formats this host serves; the first is the default
}

// supports reports whether this provider speaks a wire format.
func (p aiProviderPreset) supports(protocol string) bool {
	for _, q := range p.Protocols {
		if q == protocol {
			return true
		}
	}
	return false
}

// aiProviderPresets lists the endpoints people actually use. The base URL is
// the prefix each protocol's path hangs off, so it is stored without the path
// the transport appends.
var aiProviderPresets = []aiProviderPreset{
	{"openai", "OpenAI", "https://api.openai.com/v1", []string{"openai-completions", "openai-responses"}},
	{"anthropic", "Anthropic", "https://api.anthropic.com", []string{"anthropic-messages"}},
	{"deepseek", "DeepSeek", "https://api.deepseek.com", []string{"openai-completions"}},
	{"openrouter", "OpenRouter", "https://openrouter.ai/api/v1", []string{"openai-completions", "anthropic-messages"}},
	{"google", "Google AI", "https://generativelanguage.googleapis.com/v1beta/openai", []string{"openai-completions"}},
	{"mistral", "Mistral", "https://api.mistral.ai/v1", []string{"openai-completions"}},
	{"groq", "Groq", "https://api.groq.com/openai/v1", []string{"openai-completions"}},
	{"xai", "xAI", "https://api.x.ai/v1", []string{"openai-completions"}},
	{"cerebras", "Cerebras", "https://api.cerebras.ai/v1", []string{"openai-completions"}},
	{"together", "Together AI", "https://api.together.xyz/v1", []string{"openai-completions"}},
	{"fireworks", "Fireworks", "https://api.fireworks.ai/inference/v1", []string{"openai-completions"}},
	{"moonshot", "Moonshot", "https://api.moonshot.cn/v1", []string{"openai-completions"}},
	{"zai", "Z.ai", "https://api.z.ai/api/paas/v4", []string{"openai-completions"}},
	{"minimax", "MiniMax", "https://api.minimax.chat/v1", []string{"openai-completions"}},
	{"nvidia", "NVIDIA", "https://integrate.api.nvidia.com/v1", []string{"openai-completions"}},
	{"huggingface", "Hugging Face", "https://router.huggingface.co/v1", []string{"openai-completions"}},
	{"baseten", "Baseten", "https://inference.baseten.co/v1", []string{"openai-completions"}},
	{"ollama", "Ollama", "http://localhost:11434/v1", []string{"openai-completions"}},
	{"lmstudio", "LM Studio", "http://localhost:1234/v1", []string{"openai-completions"}},
}

func presetByID(id string) (aiProviderPreset, bool) {
	for _, p := range aiProviderPresets {
		if p.ID == id {
			return p, true
		}
	}
	return aiProviderPreset{}, false
}

// aiProtocols maps the dropdown entries to the wire formats.
var aiProtocols = []string{"openai-completions", "openai-responses", "anthropic-messages"}

func aiProtocolIndex(name string) int {
	for i, p := range aiProtocols {
		if p == name {
			return i
		}
	}
	switch {
	case strings.HasPrefix(name, "anthropic"):
		return 2
	case strings.HasPrefix(name, "openai-responses"):
		return 1
	}
	return 0
}

// protocolTitle names a wire format for the dropdown.
func protocolTitle(w *Window, index int) string {
	switch index {
	case 1:
		return w.t(i18n.GitAIProtoResponses)
	case 2:
		return w.t(i18n.GitAIProtoAnthropic)
	}
	return w.t(i18n.GitAIProtoChat)
}

// aiSettingsDraft holds the AI page while the dialog is open. Values move into
// storage.Settings only on OK, the same way the Git page works.
type aiSettingsDraft struct {
	provider string       // preset id, custom name, or "" for an older file
	custom   bool         // the endpoint is typed rather than built in
	name     *text.Editor // name of the custom provider
	endpoint *text.Editor
	model    *text.Editor
	key      *text.Editor
	protocol int
	showKey  bool // the key is masked until the eye is pressed
	models   []string
	fetching bool
	fetchErr string
	anchor   ui.Rect
}

func newAISettingsDraft(s *storage.Settings) *aiSettingsDraft {
	d := &aiSettingsDraft{
		provider: s.AIProvider,
		name:     text.NewEditor(s.AIProvider),
		endpoint: text.NewEditor(s.AIEndpoint),
		model:    text.NewEditor(s.AIModel),
		key:      text.NewEditor(s.AIKey),
		protocol: aiProtocolIndex(s.AIProtocol),
	}
	p, ok := presetByID(d.provider)
	if !ok {
		// An older settings file has no provider: treat its endpoint as custom.
		d.custom = true
		return d
	}
	// A stored protocol the provider does not serve would silently send the
	// request to a path that host does not have, so fall back to its default.
	if !p.supports(aiProtocols[d.protocol]) {
		d.protocol = aiProtocolIndex(p.Protocols[0])
	}
	return d
}

// form paints the page. It runs on every paint, so the fields follow the choice
// of provider.
func (d *aiSettingsDraft) form(f *form) {
	w := f.w
	current := w.t(i18n.GitAIProviderCustom)
	if p, ok := presetByID(d.provider); ok && !d.custom {
		current = p.Name
	} else if name := strings.TrimSpace(d.name.String()); name != "" {
		current = name
	}
	f.combo(w.t(i18n.GitAIProvider), current, func() []item {
		items := make([]item, 0, len(aiProviderPresets)+1)
		for _, p := range aiProviderPresets {
			p := p
			items = append(items, item{title: p.Name, checked: !d.custom && d.provider == p.ID, run: func() { d.pick(p) }})
		}
		items = append(items, item{title: w.t(i18n.GitAIProviderCustom), checked: d.custom, run: func() {
			d.custom = true
			// A preset's id is not a name the user typed. Keeping it here
			// would store a custom endpoint under a preset name, and the next
			// open would treat it as that preset and drop the URL.
			if _, ok := presetByID(d.provider); ok {
				setText(d.name, "")
			}
		}})
		return items
	})
	if d.custom {
		f.field(w.t(i18n.GitAIName), d.name, "My gateway", false)
		f.field(w.t(i18n.GitAIBaseURL), d.endpoint, "https://example.com/v1", false)
	}
	// The protocol list is the provider's own capability list, so a host with
	// no messages endpoint (DeepSeek, Groq, …) cannot be asked for one.
	names := aiProtocols
	if p, ok := presetByID(d.provider); ok && !d.custom {
		names = p.Protocols
	}
	f.combo(w.t(i18n.GitAIProtocol), protocolTitle(w, d.protocol), func() []item {
		items := make([]item, 0, len(names))
		for _, name := range names {
			index := aiProtocolIndex(name)
			items = append(items, item{title: protocolTitle(w, index), checked: d.protocol == index, run: func() { d.protocol = index }})
		}
		return items
	})
	if !d.custom {
		f.custom(22, func(r ui.Rect) {
			text := w.t(i18n.GitAIBaseURL) + " " + d.endpoint.String()
			lw := w.p.width(text, fontSmall, regular)
			w.p.text(text, rect(right(r)-lw, r.Y, lw+2, 20), fontSmall, regular, w.pal.TextSecond)
		})
	}
	f.secretField(w.t(i18n.GitAIKey), d.key, &d.showKey, "sk-…")
	f.field(w.t(i18n.GitAIModel), d.model, "gpt-4o-mini", false)
	f.custom(26, func(r ui.Rect) {
		d.anchor = rect(r.X+f.labelW, r.Y, 120, 26)
		if w.button(idAIFetchModels, rect(r.X+f.labelW, r.Y, 110, 26), w.t(i18n.GitAIFetchModels), btnNormal, d.fetching) {
			w.after(func() { d.fetchModels(w) })
		}
		if d.fetching {
			w.p.text(w.t(i18n.GitAIFetching), rect(r.X+f.labelW+118, r.Y+4, r.Width-f.labelW-118, 20), fontSmall, regular, w.pal.TextSecond)
		}
	})
	if d.fetchErr != "" {
		f.custom(20, func(r ui.Rect) {
			w.p.text(d.fetchErr, rect(r.X, r.Y, r.Width, 18), fontSmall, regular, w.pal.Error)
		})
	}
}

// pick switches to a built-in provider and fills in what it brings.
func (d *aiSettingsDraft) pick(p aiProviderPreset) {
	d.custom = false
	d.provider = p.ID
	setText(d.endpoint, p.BaseURL)
	d.protocol = aiProtocolIndex(p.Protocols[0])
	d.models = nil
	d.fetchErr = ""
}

// fetchModels asks the endpoint for its model names and opens them as a menu.
func (d *aiSettingsDraft) fetchModels(w *Window) {
	if d.fetching {
		return
	}
	cfg := ai.Config{
		Protocol: ai.ParseProtocol(aiProtocols[d.protocol]),
		Endpoint: d.endpoint.String(),
		APIKey:   d.key.String(),
	}
	d.fetching, d.fetchErr = true, ""
	w.ws.ListAIModels(context.Background(), cfg, func(names []string, err error) {
		d.fetching = false
		if err != nil {
			d.fetchErr = err.Error()
			return
		}
		d.models = names
		d.fetchErr = ""
		w.after(func() { d.openModelMenu(w) })
	})
}

// openModelMenu lists the fetched models and fills the field with the choice.
func (d *aiSettingsDraft) openModelMenu(w *Window) {
	current := strings.TrimSpace(d.model.String())
	items := make([]item, 0, len(d.models))
	for _, name := range d.models {
		name := name
		items = append(items, item{title: name, checked: name == current, run: func() { setText(d.model, name) }})
	}
	if len(items) == 0 {
		return
	}
	w.openMenuBelow(d.anchor, items, 260)
}

// commit writes the draft back into the settings. A built-in provider stores
// its base URL, so the generator does not have to look it up.
func (d *aiSettingsDraft) commit(s *storage.Settings) {
	s.AIModel = strings.TrimSpace(d.model.String())
	s.AIKey = strings.TrimSpace(d.key.String())
	s.AIProtocol = aiProtocols[d.protocol]
	if d.custom {
		s.AIProvider = strings.TrimSpace(d.name.String())
		s.AIEndpoint = strings.TrimSpace(d.endpoint.String())
		return
	}
	if p, ok := presetByID(d.provider); ok {
		s.AIProvider = p.ID
		s.AIEndpoint = strings.TrimSpace(d.endpoint.String())
		return
	}
	s.AIEndpoint = strings.TrimSpace(d.endpoint.String())
}
