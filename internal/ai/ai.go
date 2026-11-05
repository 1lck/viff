// Package ai turns a Git context into a commit message with a chat endpoint.
// The protocol is chosen by name, the way a provider describes its API: an
// OpenAI-compatible server speaks chat completions, an Anthropic-compatible one
// speaks messages. It has no internal dependencies, so it stays testable and
// cannot reach the rest of the program.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Protocol names the wire format of the endpoint.
type Protocol string

const (
	// ProtocolOpenAI is POST {base}/chat/completions with a Bearer token. It is
	// the default, so an older settings file with no protocol keeps working.
	ProtocolOpenAI Protocol = "openai-completions"
	// ProtocolAnthropic is POST {base}/v1/messages with an x-api-key header and
	// the system prompt as its own field.
	ProtocolAnthropic Protocol = "anthropic-messages"
	// ProtocolResponses is POST {base}/responses, the newer OpenAI surface: the
	// system prompt is "instructions" and the answer is a list of output items.
	ProtocolResponses Protocol = "openai-responses"
)

// ParseProtocol maps a stored protocol name to one, falling back to OpenAI
// because that is what the first version of this feature spoke.
func ParseProtocol(name string) Protocol {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "anthropic", "anthropic-messages", "messages":
		return ProtocolAnthropic
	case "openai-responses", "responses":
		return ProtocolResponses
	}
	return ProtocolOpenAI
}

// Config names where to send the request and how to talk to it. Endpoint is a
// base URL such as https://api.openai.com/v1 or https://api.anthropic.com;
// the protocol appends its own path.
type Config struct {
	Protocol Protocol
	Endpoint string
	APIKey   string
	Model    string
	Language string
}

// Ready reports whether the config has what a request needs.
func (c Config) Ready() bool {
	return strings.TrimSpace(c.Endpoint) != "" && strings.TrimSpace(c.Model) != ""
}

// Client sends the request. A nil HTTP keeps a default client with a timeout.
type Client struct {
	HTTP *http.Client
}

const requestTimeout = 45 * time.Second

const systemPrompt = `You write Git commit messages that follow the Conventional Commits format.

Format: <type>[optional scope]: <description>
An optional body follows one blank line after the description; footers such as
BREAKING CHANGE: follow one blank line after the body.

Rules:
- type is one of feat, fix, docs, style, refactor, perf, test, build, ci, chore,
  revert, chosen by what the change actually does.
- scope is optional, lowercase, and names the area touched (a package, module or
  directory). Omit it when the change is broad.
- description is imperative mood ("add", not "added"), lowercase, without a
  trailing period, and at most 72 characters.
- Add a body only when the diff needs explaining beyond the description.
- Mark incompatible changes with "BREAKING CHANGE:" in a footer.
- Read the diff: ignore formatting noise, machine-generated files and lock
  files when choosing the type.

Return only the commit message, with no quotes, no code fences and no
explanation.`

// CommitMessage asks the endpoint for a message for user, the assembled Git
// context, and returns it cleaned for pasting into the commit box.
func (c *Client) CommitMessage(ctx context.Context, cfg Config, user string) (string, error) {
	if !cfg.Ready() {
		return "", fmt.Errorf("ai: endpoint and model are required")
	}
	system := systemPrompt
	if lang := strings.TrimSpace(cfg.Language); lang != "" && !strings.HasPrefix(lang, "en") {
		system += "\n\nWrite the whole message in the language named by the BCP 47 tag " + lang +
			"; keep the Conventional Commits tokens (type, scope, BREAKING CHANGE) in English."
	}
	switch cfg.Protocol {
	case ProtocolAnthropic:
		return c.anthropicMessage(ctx, cfg, system, user)
	case ProtocolResponses:
		return c.responsesMessage(ctx, cfg, system, user)
	}
	return c.openAIMessage(ctx, cfg, system, user)
}

// openAIMessage posts to an OpenAI-compatible chat completions endpoint.
func (c *Client) openAIMessage(ctx context.Context, cfg Config, system, user string) (string, error) {
	body := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.3,
		"stream":      false,
	}
	headers := map[string]string{}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	url := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/") + "/chat/completions"
	data, err := c.post(ctx, url, headers, body)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("ai: cannot read the answer: %w", err)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", fmt.Errorf("ai: %s", decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("ai: the endpoint returned no choices")
	}
	return finish(decoded.Choices[0].Message.Content)
}

// anthropicMessage posts to an Anthropic-compatible messages endpoint, where
// the system prompt is a field of its own and the key travels in a header.
func (c *Client) anthropicMessage(ctx context.Context, cfg Config, system, user string) (string, error) {
	body := map[string]any{
		"model":       cfg.Model,
		"max_tokens":  1024,
		"temperature": 0.3,
		"system":      system,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	}
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		headers["x-api-key"] = key
	}
	url := messagesURL(strings.TrimSpace(cfg.Endpoint))
	data, err := c.post(ctx, url, headers, body)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("ai: cannot read the answer: %w", err)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", fmt.Errorf("ai: %s", decoded.Error.Message)
	}
	var text strings.Builder
	for _, part := range decoded.Content {
		if part.Type == "" || part.Type == "text" {
			text.WriteString(part.Text)
		}
	}
	return finish(text.String())
}

// messagesURL keeps a base URL that already ends in /v1 from growing a second
// one.
func messagesURL(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(endpoint, "/v1") {
		return endpoint + "/messages"
	}
	return endpoint + "/v1/messages"
}

// responsesMessage posts to an OpenAI-compatible responses endpoint, where the
// system prompt is "instructions" and the answer is a list of output items.
func (c *Client) responsesMessage(ctx context.Context, cfg Config, system, user string) (string, error) {
	body := map[string]any{
		"model":        cfg.Model,
		"instructions": system,
		"input": []map[string]string{
			{"role": "user", "content": user},
		},
		"temperature": 0.3,
	}
	headers := map[string]string{}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	url := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/") + "/responses"
	data, err := c.post(ctx, url, headers, body)
	if err != nil {
		return "", err
	}
	var decoded struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("ai: cannot read the answer: %w", err)
	}
	if decoded.Error != nil && decoded.Error.Message != "" {
		return "", fmt.Errorf("ai: %s", decoded.Error.Message)
	}
	var text strings.Builder
	for _, item := range decoded.Output {
		for _, part := range item.Content {
			if part.Type == "" || part.Type == "output_text" || part.Type == "text" {
				text.WriteString(part.Text)
			}
		}
	}
	return finish(text.String())
}

// ListModels asks the endpoint for the model names it serves. Only
// OpenAI-compatible endpoints answer this; an Anthropic-compatible one refuses,
// and the caller keeps the field editable.
func (c *Client) ListModels(ctx context.Context, cfg Config) ([]string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("ai: endpoint is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(data))
		if len(message) > 200 {
			message = message[:200] + "…"
		}
		return nil, fmt.Errorf("ai: endpoint answered %s: %s", response.Status, message)
	}
	var decoded struct {
		// OpenAI and its clones answer with data[].id.
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		// Ollama's native endpoint answers with models[].name.
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("ai: cannot read the model list: %w", err)
	}
	seen := map[string]bool{}
	var names []string
	for _, entry := range decoded.Data {
		if entry.ID == "" || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		names = append(names, entry.ID)
	}
	for _, entry := range decoded.Models {
		if entry.Name == "" || seen[entry.Name] {
			continue
		}
		seen[entry.Name] = true
		names = append(names, entry.Name)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("ai: the endpoint listed no models")
	}
	sort.Strings(names)
	return names, nil
}

// post sends one JSON request and returns the body of a successful answer.
func (c *Client) post(ctx context.Context, url string, headers map[string]string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(data))
		if len(message) > 300 {
			message = message[:300] + "…"
		}
		return nil, fmt.Errorf("ai: endpoint answered %s: %s", response.Status, message)
	}
	return data, nil
}

// finish cleans the answer and rejects an empty one.
func finish(raw string) (string, error) {
	message := Clean(raw)
	if message == "" {
		return "", fmt.Errorf("ai: the endpoint returned an empty message")
	}
	return message, nil
}

// Clean trims a model answer down to the message itself: surrounding space,
// markdown code fences and wrapping quotes are dropped.
func Clean(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}
