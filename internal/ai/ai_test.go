package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseProtocol(t *testing.T) {
	cases := map[string]Protocol{
		"":                     ProtocolOpenAI,
		"openai-completions":   ProtocolOpenAI,
		"chat":                 ProtocolOpenAI,
		"anthropic":            ProtocolAnthropic,
		" Anthropic-Messages ": ProtocolAnthropic,
		"messages":             ProtocolAnthropic,
		"openai-responses":     ProtocolResponses,
		"responses":            ProtocolResponses,
	}
	for name, want := range cases {
		if got := ParseProtocol(name); got != want {
			t.Errorf("ParseProtocol(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"  feat: add a thing  ":            "feat: add a thing",
		"```\nfeat: add a thing\n```":      "feat: add a thing",
		"```text\nfix: repair\n```":        "fix: repair",
		`"feat: quoted"`:                   "feat: quoted",
		"'fix: single'":                    "fix: single",
		"feat: a\n\nbody\n":                "feat: a\n\nbody",
		"```\nfeat: fenced\n\nbody\n```\n": "feat: fenced\n\nbody",
		"no fences":                        "no fences",
	}
	for raw, want := range cases {
		if got := Clean(raw); got != want {
			t.Errorf("Clean(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestMessagesURL(t *testing.T) {
	cases := map[string]string{
		"https://api.anthropic.com":   "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/":  "https://api.anthropic.com/v1/messages",
		"https://gateway.example/v1":  "https://gateway.example/v1/messages",
		"https://gateway.example/v1/": "https://gateway.example/v1/messages",
	}
	for base, want := range cases {
		if got := messagesURL(base); got != want {
			t.Errorf("messagesURL(%q) = %q, want %q", base, got, want)
		}
	}
}

// serve starts one endpoint and hands each request to handle.
func serve(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(handle))
	t.Cleanup(srv.Close)
	return srv
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	return body
}

// TestCommitMessageOpenAI pins the chat completions shape: the path, the bearer
// header and the system/user messages.
func TestCommitMessageOpenAI(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("%s %s, want POST /v1/chat/completions", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		body := decodeBody(t, r)
		if body["model"] != "gpt-test" {
			t.Errorf("model = %v", body["model"])
		}
		messages, _ := body["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("messages = %v, want system and user", body["messages"])
		}
		if role := messages[0].(map[string]any)["role"]; role != "system" {
			t.Errorf("first role = %v", role)
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"feat: add a thing"}}]}`)
	})

	client := &Client{}
	got, err := client.CommitMessage(context.Background(), Config{
		Protocol: ProtocolOpenAI, Endpoint: srv.URL + "/v1", APIKey: "sk-test", Model: "gpt-test",
	}, "diff")
	if err != nil {
		t.Fatal(err)
	}
	if got != "feat: add a thing" {
		t.Fatalf("message = %q", got)
	}
}

// TestCommitMessageAnthropic pins the messages shape: the key travels in
// x-api-key, the system prompt is a field and the answer is a content list.
func TestCommitMessageAnthropic(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-ant" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Error("anthropic-version header is missing")
		}
		body := decodeBody(t, r)
		if body["system"] == "" {
			t.Error("system prompt is missing")
		}
		if body["max_tokens"] == nil {
			t.Error("max_tokens is missing")
		}
		io.WriteString(w, `{"content":[{"type":"text","text":"fix: repair it"}]}`)
	})

	client := &Client{}
	got, err := client.CommitMessage(context.Background(), Config{
		Protocol: ProtocolAnthropic, Endpoint: srv.URL, APIKey: "sk-ant", Model: "claude-test",
	}, "diff")
	if err != nil {
		t.Fatal(err)
	}
	if got != "fix: repair it" {
		t.Fatalf("message = %q", got)
	}
}

// TestCommitMessageResponses pins the responses shape: the system prompt is
// instructions and the answer is a list of output items.
func TestCommitMessageResponses(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		body := decodeBody(t, r)
		if body["instructions"] == "" {
			t.Error("instructions are missing")
		}
		io.WriteString(w, `{"output":[{"content":[{"type":"output_text","text":"docs: explain it"}]}]}`)
	})

	client := &Client{}
	got, err := client.CommitMessage(context.Background(), Config{
		Protocol: ProtocolResponses, Endpoint: srv.URL + "/v1", APIKey: "sk-test", Model: "gpt-test",
	}, "diff")
	if err != nil {
		t.Fatal(err)
	}
	if got != "docs: explain it" {
		t.Fatalf("message = %q", got)
	}
}

// TestCommitMessageRejectsFailures covers the paths that must not look like an
// empty answer: a bad status, an endpoint error field and no choices.
func TestCommitMessageRejectsFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"status", http.StatusUnauthorized, `{"error":{"message":"bad key"}}`, "401"},
		{"endpoint error", http.StatusOK, `{"error":{"message":"model overloaded"}}`, "model overloaded"},
		{"no choices", http.StatusOK, `{"choices":[]}`, "no choices"},
		{"empty content", http.StatusOK, `{"choices":[{"message":{"content":"   "}}]}`, "empty message"},
		{"not json", http.StatusOK, `not json`, "cannot read the answer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				io.WriteString(w, c.body)
			})
			client := &Client{}
			_, err := client.CommitMessage(context.Background(), Config{
				Protocol: ProtocolOpenAI, Endpoint: srv.URL, Model: "m",
			}, "diff")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

// TestCommitMessageRequiresConfig keeps an unconfigured request from reaching
// the network at all.
func TestCommitMessageRequiresConfig(t *testing.T) {
	client := &Client{}
	if _, err := client.CommitMessage(context.Background(), Config{}, "diff"); err == nil {
		t.Fatal("an empty config must be refused")
	}
}

// TestListModels reads both shapes endpoints use: OpenAI's data[].id and
// Ollama's models[].name.
func TestListModels(t *testing.T) {
	cases := map[string]string{
		"data":   `{"data":[{"id":"z-model"},{"id":"a-model"},{"id":"a-model"}]}`,
		"ollama": `{"models":[{"name":"llama3"},{"name":"qwen2"}]}`,
		"both":   `{"data":[{"id":"b"},{"id":"a"}],"models":[{"name":"c"},{"name":"a"}]}`,
	}
	want := map[string][]string{
		"data":   {"a-model", "z-model"},
		"ollama": {"llama3", "qwen2"},
		"both":   {"a", "b", "c"},
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					t.Errorf("path = %q, want /v1/models", r.URL.Path)
				}
				io.WriteString(w, payload)
			})
			names, err := (&Client{}).ListModels(context.Background(), Config{
				Endpoint: srv.URL + "/v1", APIKey: "sk-test",
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(names, ",") != strings.Join(want[name], ",") {
				t.Fatalf("names = %v, want %v", names, want[name])
			}
		})
	}
}

// TestListModelsRejectsEmpty keeps a listing endpoint that answers with nothing
// from opening an empty menu.
func TestListModelsRejectsEmpty(t *testing.T) {
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[]}`)
	})
	_, err := (&Client{}).ListModels(context.Background(), Config{Endpoint: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "no models") {
		t.Fatalf("err = %v, want a no-models error", err)
	}
}

// TestLanguageIsPassedToTheModel keeps a Chinese interface from producing an
// English commit message.
func TestLanguageIsPassedToTheModel(t *testing.T) {
	var system string
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(t, r)
		messages, _ := body["messages"].([]any)
		system = messages[0].(map[string]any)["content"].(string)
		io.WriteString(w, `{"choices":[{"message":{"content":"feat: 增加功能"}}]}`)
	})
	_, err := (&Client{}).CommitMessage(context.Background(), Config{
		Protocol: ProtocolOpenAI, Endpoint: srv.URL, Model: "m", Language: "zh-CN",
	}, "diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(system, "zh-CN") {
		t.Fatalf("system prompt does not name the language: %q", system)
	}
}
