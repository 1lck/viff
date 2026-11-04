package diagnostics

import (
	"strings"
	"testing"
	"time"

	"viff/internal/observability"
)

// summaryEnv is a fully populated Env for summary rendering tests.
func summaryEnv() Env {
	return testEnv()
}

// errorRecord builds one error event for summary tests.
func errorRecord(base time.Time, seq uint64, msg string) observability.Record {
	return observability.Record{
		Time:  base.Add(time.Duration(seq) * time.Second),
		Seq:   seq,
		Level: "error",
		Event: "git.fail",
		Op:    "push",
		Code:  "push.rejected",
		Msg:   msg,
		Data:  map[string]any{"exit": 128, "kind": "rejected"},
	}
}

// TestBuildSummaryStructure covers the Chinese summary layout required by
// docs/observability.md §8.4.
func TestBuildSummaryStructure(t *testing.T) {
	base := testBase()
	events := []observability.Record{
		{
			Time: base.Add(1 * time.Minute), Seq: 1, Level: "info", Event: "app.start",
		},
		errorRecord(base, 60, "failed to push some refs"),
		{
			Time: base.Add(2 * time.Minute), Seq: 61, Level: "info", Event: "git.end",
			Op: "commit", DurMS: 120, Data: map[string]any{"write": true},
		},
	}
	in := summaryInput{
		env: summaryEnv(),
		settings: SettingsSummary{
			"theme":    "dark",
			"fontSize": 13,
			"nested":   map[string]any{"k": "v"},
		},
		repos: []RepoInfo{{
			Hash: "9f3a1c2b", Name: "viff", Branch: "main",
			Operation: "rebase", Dirty: true, RefreshedAt: base,
		}},
		events:      events,
		opts:        Options{Description: "推送时一直失败", IncidentID: "inc-8f21"},
		incident:    &observability.Incident{ID: "inc-8f21", Kind: "problem"},
		generatedAt: base.Add(5 * time.Minute),
		windowStart: base,
		windowEnd:   base.Add(5 * time.Minute),

		redactionReviewed: true,
	}
	summary := string(buildSummary(in))

	for _, want := range []string{
		"# Viff 诊断摘要",
		"- 版本：0.1.0-alpha.3（abc1234，2026-11-03 构建）",
		"- 平台：darwin/arm64 · 缩放 1.00 · 主题 dark · locale zh-CN",
		"- Git：git version 2.53.0（来自 settings） · Viva：v0.2.3 · Go：go1.27.3 · 设置 schema：1",
		"窗口：2026-11-04T13:00:00Z–2026-11-04T13:05:00Z（诊断会话 inc-8f21）",
		"用户描述：推送时一直失败",
		"## 最近错误",
		"**git.fail**",
		"op=push",
		"code=push.rejected",
		"kind=rejected",
		"exit=128",
		"failed to push some refs",
		"## 时间线（错误与写操作）",
		"git.end",
		"## 环境与设置摘要",
		"### 设置摘要",
		"| 设置项 | 值 |",
		"### 仓库",
		"9f3a1c2b",
		"## 脱敏说明",
		"standard",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary does not contain %q:\n%s", want, summary)
		}
	}
	// The informational app.start event must not appear in the timeline.
	timeline := summarySection(t, summary, "## 时间线（错误与写操作）", "## 环境与设置摘要")
	if strings.Contains(timeline, "app.start") {
		t.Errorf("timeline contains the informational app.start event:\n%s", timeline)
	}
}

// summarySection returns the text between two Markdown headings.
func summarySection(t *testing.T, summary, start, end string) string {
	t.Helper()
	from := strings.Index(summary, start)
	if from < 0 {
		t.Fatalf("summary is missing heading %q", start)
	}
	to := strings.Index(summary[from:], end)
	if to < 0 {
		t.Fatalf("summary is missing heading %q after %q", end, start)
	}
	return summary[from : from+to]
}

// TestBuildSummaryRecentErrorsNewestFirst checks the 10-error cap and ordering.
func TestBuildSummaryRecentErrorsNewestFirst(t *testing.T) {
	base := testBase()
	events := make([]observability.Record, 15)
	for i := range events {
		events[i] = errorRecord(base, uint64(i+1), "err-"+twoDigits(i+1))
	}
	summary := string(buildSummary(summaryInput{
		env:               summaryEnv(),
		events:            events,
		generatedAt:       base.Add(15 * time.Minute),
		windowStart:       base,
		windowEnd:         base.Add(15 * time.Minute),
		redactionReviewed: true,
	}))

	if got := strings.Count(summary, "**git.fail**"); got != summaryRecentErrors {
		t.Errorf("summary lists %d errors, want %d", got, summaryRecentErrors)
	}
	recent := summarySection(t, summary, "## 最近错误", "## 时间线（错误与写操作）")
	if !strings.Contains(recent, "err-15") || !strings.Contains(recent, "err-06") {
		t.Errorf("summary is missing the newest errors:\n%s", recent)
	}
	if strings.Contains(recent, "err-05") {
		t.Errorf("summary lists an error older than the newest %d:\n%s", summaryRecentErrors, recent)
	}
	if strings.Index(recent, "err-15") > strings.Index(recent, "err-14") {
		t.Errorf("recent errors are not newest-first:\n%s", recent)
	}
}

// twoDigits formats small indices for stable test message names.
func twoDigits(value int) string {
	if value < 10 {
		return "0" + string(rune('0'+value))
	}
	return string(rune('0'+value/10)) + string(rune('0'+value%10))
}

// TestBuildSummaryTimeline checks that the timeline keeps the newest 50
// error/warn/write entries and ignores plain info events.
func TestBuildSummaryTimeline(t *testing.T) {
	base := testBase()
	var events []observability.Record
	for i := 1; i <= 60; i++ {
		events = append(events, errorRecord(base, uint64(i), "err-"+twoDigits(i)))
	}
	for i := 0; i < 10; i++ {
		events = append(events, observability.Record{
			Time: base.Add(time.Duration(100+i) * time.Second), Seq: uint64(100 + i),
			Level: "info", Event: "repo.refresh", Op: "refresh",
		})
	}
	for i := 0; i < 5; i++ {
		events = append(events, observability.Record{
			Time: base.Add(time.Duration(200+i) * time.Second), Seq: uint64(200 + i),
			Level: "info", Event: "git.end", Op: "commit",
			Data: map[string]any{"write": true},
		})
	}
	summary := string(buildSummary(summaryInput{
		env:               summaryEnv(),
		events:            events,
		generatedAt:       base.Add(10 * time.Minute),
		windowStart:       base,
		windowEnd:         base.Add(10 * time.Minute),
		redactionReviewed: true,
	}))
	timeline := summarySection(t, summary, "## 时间线（错误与写操作）", "## 环境与设置摘要")
	if got := strings.Count(timeline, "- `"); got != summaryTimeline {
		t.Errorf("timeline has %d entries, want %d:\n%s", got, summaryTimeline, timeline)
	}
	if strings.Contains(timeline, "repo.refresh") {
		t.Errorf("timeline contains a plain info refresh event:\n%s", timeline)
	}
	if !strings.Contains(timeline, "git.end") {
		t.Errorf("timeline is missing write events:\n%s", timeline)
	}
	if strings.Contains(timeline, "err-15") {
		t.Errorf("timeline kept an entry older than the newest %d:\n%s", summaryTimeline, timeline)
	}
	if !strings.Contains(timeline, "err-16") {
		t.Errorf("timeline is missing the oldest kept entry err-16:\n%s", timeline)
	}
}

// TestBuildSummaryEmptyData checks that empty input renders explicit "无"
// markers instead of blank sections.
func TestBuildSummaryEmptyData(t *testing.T) {
	summary := string(buildSummary(summaryInput{
		env:         Env{},
		generatedAt: testBase(),
		windowStart: testBase(),
		windowEnd:   testBase(),

		redactionReviewed: true,
	}))
	for _, want := range []string{
		"# Viff 诊断摘要",
		"## 最近错误\n\n（无）",
		"## 时间线（错误与写操作）\n\n（无）",
		"### 设置摘要\n\n（无）",
		"### 仓库\n\n（无）",
		"## 脱敏说明",
		"无",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("empty summary does not contain %q:\n%s", want, summary)
		}
	}
}

// TestBuildSummaryRecordDetails checks op/code/kind/exit/dur/msg rendering.
func TestBuildSummaryRecordDetails(t *testing.T) {
	base := testBase()
	record := observability.Record{
		Time: base.Add(time.Minute), Seq: 1, Level: "error", Event: "git.fail",
		Op: "fetch", Code: "auth.failed", DurMS: 1200,
		Msg:  "authentication failed",
		Data: map[string]any{"exit": 128, "kind": "auth"},
	}
	summary := string(buildSummary(summaryInput{
		env:               summaryEnv(),
		events:            []observability.Record{record},
		generatedAt:       base.Add(2 * time.Minute),
		windowStart:       base,
		windowEnd:         base.Add(2 * time.Minute),
		redactionReviewed: false,
	}))
	for _, want := range []string{
		"code=auth.failed",
		"kind=auth",
		"exit=128",
		"（1.2s）",
		"authentication failed",
		"redaction_reviewed` 为 false",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary does not contain %q:\n%s", want, summary)
		}
	}
}

// TestBuildSummaryMultilineDescription checks comment continuation lines.
func TestBuildSummaryMultilineDescription(t *testing.T) {
	base := testBase()
	summary := string(buildSummary(summaryInput{
		env:               summaryEnv(),
		opts:              Options{Description: "第一行\n第二行"},
		generatedAt:       base,
		windowStart:       base,
		windowEnd:         base,
		redactionReviewed: true,
	}))
	if !strings.Contains(summary, "第一行\n  第二行") {
		t.Errorf("summary lost the multi-line description:\n%s", summary)
	}
}

// TestBuildSummaryTableEscaping checks that table cells stay one row.
func TestBuildSummaryTableEscaping(t *testing.T) {
	base := testBase()
	summary := string(buildSummary(summaryInput{
		env: summaryEnv(),
		settings: SettingsSummary{
			"weird|key": "a\nb",
		},
		generatedAt:       base,
		windowStart:       base,
		windowEnd:         base,
		redactionReviewed: true,
	}))
	if strings.Contains(summary, "a\nb") {
		t.Errorf("summary did not flatten a newline inside a table cell:\n%s", summary)
	}
	if !strings.Contains(summary, `weird\|key`) {
		t.Errorf("summary did not escape a pipe in a table cell:\n%s", summary)
	}
}
