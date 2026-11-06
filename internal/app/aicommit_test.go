package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"viff/internal/ai"
	"viff/internal/domain"
)

// aiEndpoint answers one chat completions request and records the user message
// it was sent, so a test can assert what left the machine.
func aiEndpoint(t *testing.T, answer string) (*httptest.Server, *string) {
	t.Helper()
	var user string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		if len(req.Messages) > 0 {
			user = req.Messages[len(req.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":`+strconv.Quote(answer)+`}}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &user
}

// aiRuleEndpoint answers one chat completions request and records both messages
// it was sent, so a test can assert which rules reached the model.
func aiRuleEndpoint(t *testing.T, answer string) (srv *httptest.Server, system, user *string) {
	t.Helper()
	var sys, usr string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		for _, m := range req.Messages {
			switch m.Role {
			case "system":
				sys = m.Content
			case "user":
				usr = m.Content
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":`+strconv.Quote(answer)+`}}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &sys, &usr
}

// TestGenerateCommitMessageSendsTheSelectedRules proves the rules the user picked
// reach the request: the selected rule's text becomes the system message, and the
// selection is read before the request starts.
func TestGenerateCommitMessageSendsTheSelectedRules(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("base")
	f.open()
	f.write("a.txt", "two\n")
	f.sync()

	const rules = "Write one lowercase sentence with no type prefix."
	id, err := f.w.SavePromptRule(PromptRule{Name: "Terse", Text: rules})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.w.SelectPromptRule(id); err != nil {
		t.Fatal(err)
	}

	srv, system, user := aiRuleEndpoint(t, "add second line")
	done := false
	f.w.GenerateCommitMessage(context.Background(), ai.Config{
		Protocol: ai.ProtocolOpenAI, Endpoint: srv.URL, Model: "test",
	}, "", func(_ string, err error) {
		if err != nil {
			t.Errorf("generate: %v", err)
		}
		done = true
	})
	f.settle(func() bool { return done })

	if !strings.Contains(*system, rules) {
		t.Fatalf("system message does not carry the selected rules:\n%s", *system)
	}
	if strings.Contains(*system, ai.DefaultRules) {
		t.Fatalf("system message still carries the built-in rules:\n%s", *system)
	}
	// The user message is unchanged by this feature: it is still the Git context.
	if !strings.Contains(*user, "modified a.txt") || !strings.Contains(*user, "+two") {
		t.Fatalf("the user message changed shape:\n%s", *user)
	}
}

// TestGenerateCommitMessageBuiltinRuleKeepsTheDefaultPrompt pins the built-in
// path: with no custom rule the request is exactly what it was before rules
// existed, because an empty rules string makes package ai use its own default.
func TestGenerateCommitMessageBuiltinRuleKeepsTheDefaultPrompt(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("base")
	f.open()
	f.write("a.txt", "two\n")
	f.sync()

	srv, system, _ := aiRuleEndpoint(t, "feat: change a")
	done := false
	f.w.GenerateCommitMessage(context.Background(), ai.Config{
		Protocol: ai.ProtocolOpenAI, Endpoint: srv.URL, Model: "test",
	}, "", func(string, error) { done = true })
	f.settle(func() bool { return done })

	if *system != ai.SystemPrompt("", "") {
		t.Fatalf("built-in system message changed:\n%s", *system)
	}
}

// TestGenerateCommitMessageSendsOnlyTheListedFiles pins the documented privacy
// boundary: a lock file stays out of the changed-file list and out of the patch.
func TestGenerateCommitMessageSendsOnlyTheListedFiles(t *testing.T) {
	f := newFixture(t)
	f.write("main.go", "package main\n")
	f.write("go.sum", "example.com/mod v1.0.0 h1:aaa\n")
	f.commitAll("base")
	f.open()

	f.write("main.go", "package main\n\nfunc main() {}\n")
	f.write("go.sum", "example.com/mod v1.0.1 h1:bbb\n")
	f.sync()

	srv, user := aiEndpoint(t, "feat: add main")
	var got string
	var gotErr error
	done := false
	f.w.GenerateCommitMessage(context.Background(), ai.Config{
		Protocol: ai.ProtocolOpenAI, Endpoint: srv.URL, Model: "test",
	}, "", func(message string, err error) { got, gotErr, done = message, err, true })
	f.settle(func() bool { return done })

	if gotErr != nil {
		t.Fatal(gotErr)
	}
	if got != "feat: add main" {
		t.Fatalf("message = %q", got)
	}
	if !strings.Contains(*user, "modified main.go") {
		t.Fatalf("prompt has no changed-file line:\n%s", *user)
	}
	if !strings.Contains(*user, "+func main() {}") {
		t.Fatalf("prompt has no diff:\n%s", *user)
	}
	if strings.Contains(*user, "go.sum") || strings.Contains(*user, "h1:bbb") {
		t.Fatalf("lock file leaked into the request:\n%s", *user)
	}
}

// TestGenerateCommitMessagePrefersStaged keeps an unstaged edit out of the
// prompt when something is staged.
func TestGenerateCommitMessagePrefersStaged(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.write("b.txt", "one\n")
	f.commitAll("base")
	f.open()

	f.write("a.txt", "staged change\n")
	f.write("b.txt", "unstaged change\n")
	f.git("add", "a.txt")
	f.sync()

	srv, user := aiEndpoint(t, "feat: stage a")
	done := false
	f.w.GenerateCommitMessage(context.Background(), ai.Config{
		Protocol: ai.ProtocolOpenAI, Endpoint: srv.URL, Model: "test",
	}, "", func(string, error) { done = true })
	f.settle(func() bool { return done })

	if !strings.Contains(*user, "staged change") {
		t.Fatalf("prompt has no staged diff:\n%s", *user)
	}
	if strings.Contains(*user, "unstaged change") {
		t.Fatalf("prompt has the unstaged edit:\n%s", *user)
	}
}

// TestGenerateCommitMessageReportsNoChanges keeps a clean tree from opening a
// request.
func TestGenerateCommitMessageReportsNoChanges(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "one\n")
	f.commitAll("base")
	f.open()

	srv, _ := aiEndpoint(t, "unused")
	var gotErr error
	done := false
	f.w.GenerateCommitMessage(context.Background(), ai.Config{
		Protocol: ai.ProtocolOpenAI, Endpoint: srv.URL, Model: "test",
	}, "", func(_ string, err error) { gotErr, done = err, true })
	f.settle(func() bool { return done })

	if gotErr == nil || !strings.Contains(gotErr.Error(), "no working-tree changes") {
		t.Fatalf("err = %v, want a no-changes error", gotErr)
	}
}

// TestBuildCommitPrompt pins the prompt's sections and the rerun instruction.
func TestBuildCommitPrompt(t *testing.T) {
	snap := &commitSnapshot{
		staged: true,
		branch: "feature/x",
		files: []domain.StatusEntry{
			{Path: "a.txt", Index: 'M', Worktree: '.'},
		},
		recent: []string{"abc12345 base commit"},
	}
	prompt := buildCommitPrompt(snap, "diff --git a/a.txt b/a.txt\n+two\n", "", false)
	for _, want := range []string{
		"Branch: feature/x",
		"abc12345 base commit",
		"modified a.txt",
		"--- a/a.txt b/a.txt",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "COMPLETELY DIFFERENT") {
		t.Fatalf("first run asked for different wording:\n%s", prompt)
	}

	again := buildCommitPrompt(snap, "diff", "feat: first", false)
	if !strings.Contains(again, "COMPLETELY DIFFERENT") || !strings.Contains(again, `"feat: first"`) {
		t.Fatalf("rerun prompt does not carry the previous message:\n%s", again)
	}
}

// TestBuildCommitPromptNeutralForCustomRules keeps the user's own rules from
// being contradicted by the surrounding text: the built-in format is named in
// two places, and both would otherwise tell the model to write Conventional
// Commits even when the rules forbid it.
func TestBuildCommitPromptNeutralForCustomRules(t *testing.T) {
	snap := &commitSnapshot{
		staged: true,
		branch: "feature/x",
		files:  []domain.StatusEntry{{Path: "a.txt", Index: 'M', Worktree: '.'}},
		recent: []string{"abc12345 feat(ai): a conventional subject"},
	}
	prompt := buildCommitPrompt(snap, "diff --git a/a.txt b/a.txt\n+two\n", "", true)
	if strings.Contains(prompt, "Use a different type, scope or description approach") {
		t.Fatalf("custom rules still get the Conventional Commits rerun wording:\n%s", prompt)
	}
	if !strings.Contains(prompt, "for naming reference only") {
		t.Fatalf("recent commits are not marked as reference only:\n%s", prompt)
	}
	// The context itself is not a setting: it must still be there in full.
	for _, want := range []string{"Branch: feature/x", "abc12345 feat(ai):", "modified a.txt", "+two"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("custom-rules prompt dropped %q:\n%s", want, prompt)
		}
	}

	again := buildCommitPrompt(snap, "diff", "one lowercase sentence", true)
	if !strings.Contains(again, "COMPLETELY DIFFERENT") || !strings.Contains(again, "different wording and structure") {
		t.Fatalf("neutral rerun instruction is missing:\n%s", again)
	}
	if strings.Contains(again, "type, scope") {
		t.Fatalf("neutral rerun instruction still names the built-in format:\n%s", again)
	}
}

func TestIsLockFile(t *testing.T) {
	locked := []string{"go.sum", "package-lock.json", "sub/dir/yarn.lock", "pnpm-lock.yaml", "Cargo.lock", "app.min.js", "app.min.css"}
	for _, name := range locked {
		if !isLockFile(name) {
			t.Errorf("isLockFile(%q) = false", name)
		}
	}
	for _, name := range []string{"main.go", "README.md", "go.mod", "src/lock.go"} {
		if isLockFile(name) {
			t.Errorf("isLockFile(%q) = true", name)
		}
	}
}

func TestTrimmedDiff(t *testing.T) {
	if got := trimmedDiff(""); !strings.Contains(got, "no textual diff") {
		t.Fatalf("empty patch = %q", got)
	}
	if got := trimmedDiff("diff --git a/x.png b/x.png\nGIT binary patch\nabc\n"); strings.Contains(got, "GIT binary patch") {
		t.Fatalf("binary patch was kept: %q", got)
	}

	big := strings.Repeat("x", maxDiffChars+100)
	got := trimmedDiff("diff --git a/a.txt b/a.txt\n" + big)
	if !strings.Contains(got, "... [truncated]") {
		t.Fatal("oversized diff was not truncated")
	}
	if len(got) > maxDiffChars+200 {
		t.Fatalf("truncated diff is still %d bytes", len(got))
	}

	var many strings.Builder
	for i := 0; i < maxDiffFiles+2; i++ {
		name := "f" + strconv.Itoa(i)
		many.WriteString("diff --git a/" + name + " b/" + name + "\n+x\n")
	}
	if got := trimmedDiff(many.String()); !strings.Contains(got, "more files omitted") {
		t.Fatalf("extra files were not omitted:\n%s", got)
	}
}
