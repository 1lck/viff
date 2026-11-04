package diagnostics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"viff/internal/observability"
)

const (
	// summaryRecentErrors is how many error-level records the summary lists.
	summaryRecentErrors = 10
	// summaryTimeline is how many error/warn/write records the timeline keeps.
	summaryTimeline = 50
	// summaryMsgRunes bounds one message shown in the summary.
	summaryMsgRunes = 400
	// summaryTimelineMsgRunes bounds the one-line message of a timeline entry.
	summaryTimelineMsgRunes = 120
	// summaryCellRunes bounds one table cell.
	summaryCellRunes = 300
)

// summaryInput is the sanitized material summary.md is rendered from. The
// caller (WriteBundle) has already dropped or replaced anything suspicious, so
// rendering is pure formatting.
type summaryInput struct {
	env               Env
	settings          SettingsSummary
	repos             []RepoInfo
	events            []observability.Record
	opts              Options
	incident          *observability.Incident
	generatedAt       time.Time
	windowStart       time.Time
	windowEnd         time.Time
	redactionReviewed bool
}

// buildSummary renders the human-readable Chinese summary.md. The structure
// follows docs/observability.md §8.4: an author should be able to find the
// failing operation from this file alone.
func buildSummary(in summaryInput) []byte {
	var b strings.Builder
	b.WriteString("# Viff 诊断摘要\n\n")
	b.WriteString("- 版本：" + versionSummary(in.env) + "\n")
	b.WriteString("- 平台：" + platformSummary(in.env) + "\n")
	b.WriteString("- " + gitSummary(in.env) + "\n")
	b.WriteString("- " + windowSummary(in) + "\n")
	if desc := strings.TrimSpace(in.opts.Description); desc != "" {
		b.WriteString("- 用户描述：" + indentContinuation(desc, "  ") + "\n")
	}

	b.WriteString("\n## 最近错误\n\n")
	writeRecentErrors(&b, in.events)

	b.WriteString("\n## 时间线（错误与写操作）\n\n")
	writeTimeline(&b, in.events)

	b.WriteString("\n## 环境与设置摘要\n\n")
	writeEnvironmentTable(&b, in.env)
	writeSettingsTable(&b, in.settings)
	writeReposTable(&b, in.repos)

	b.WriteString("\n## 脱敏说明\n\n")
	b.WriteString("本包按 standard 档脱敏：仓库路径已哈希、remote URL 仅保留 host、" +
		"Git 输出截断且已脱敏；不包含 token、密钥、提交内容或文件内容。\n")
	if !in.redactionReviewed {
		b.WriteString("\n导出时检测到疑似敏感内容，相关行已被跳过；" +
			"manifest.json 中 `redaction_reviewed` 为 false，请人工确认后再分享本包。\n")
	}
	return []byte(b.String())
}

// writeRecentErrors lists the newest error records, newest first.
func writeRecentErrors(b *strings.Builder, events []observability.Record) {
	errors := make([]observability.Record, 0, summaryRecentErrors)
	for _, r := range events {
		if isErrorRecord(r) {
			errors = append(errors, r)
		}
	}
	if len(errors) > summaryRecentErrors {
		errors = errors[len(errors)-summaryRecentErrors:]
	}
	if len(errors) == 0 {
		b.WriteString("（无）\n")
		return
	}
	number := 0
	for i := len(errors) - 1; i >= 0; i-- {
		r := errors[i]
		number++
		fmt.Fprintf(b, "%d. `%s` **%s**", number, clockTime(r.Time), orNone(r.Event))
		writeRecordDetails(b, r)
		if msg := truncateRunes(strings.TrimSpace(r.Msg), summaryMsgRunes); msg != "" {
			b.WriteString("\n   " + indentContinuation(msg, "   ") + "\n")
		} else {
			b.WriteString("\n")
		}
	}
}

// writeTimeline lists the newest error/warn/write records in chronological
// order (oldest first) so the sequence of actions stays readable.
func writeTimeline(b *strings.Builder, events []observability.Record) {
	selected := make([]observability.Record, 0, summaryTimeline)
	for _, r := range events {
		if isTimelineRecord(r) {
			selected = append(selected, r)
		}
	}
	if len(selected) > summaryTimeline {
		selected = selected[len(selected)-summaryTimeline:]
	}
	if len(selected) == 0 {
		b.WriteString("（无）\n")
		return
	}
	for _, r := range selected {
		fmt.Fprintf(b, "- `%s` %s %s", clockTime(r.Time), orNone(r.Level), orNone(r.Event))
		if r.Op != "" {
			b.WriteString(" " + r.Op)
		}
		if r.Code != "" {
			b.WriteString(" code=" + r.Code)
		}
		if dur := fmtDurationMS(r.DurMS); dur != "" {
			b.WriteString(" dur=" + dur)
		}
		if msg := firstLine(r.Msg); msg != "" {
			b.WriteString(" — " + truncateRunes(msg, summaryTimelineMsgRunes))
		}
		b.WriteString("\n")
	}
}

// writeRecordDetails appends op/code/kind/exit/dur facts to a record line.
func writeRecordDetails(b *strings.Builder, r observability.Record) {
	if r.Op != "" {
		b.WriteString(" op=" + r.Op)
	}
	if r.Code != "" {
		b.WriteString(" code=" + r.Code)
	}
	if kind := recordDataString(r, "kind"); kind != "" {
		b.WriteString(" kind=" + kind)
	}
	if exit := recordDataString(r, "exit"); exit != "" {
		b.WriteString(" exit=" + exit)
	}
	if dur := fmtDurationMS(r.DurMS); dur != "" {
		b.WriteString("（" + dur + "）")
	}
}

// writeEnvironmentTable renders the environment facts collected at export time.
func writeEnvironmentTable(b *strings.Builder, env Env) {
	b.WriteString("| 项 | 值 |\n|---|---|\n")
	rows := [][2]string{
		{"应用版本", versionSummary(env)},
		{"Commit", orNone(env.Commit)},
		{"构建时间", orNone(env.BuildDate)},
		{"Go", orNone(env.GoVersion)},
		{"Viva", orNone(env.VivaVersion)},
		{"平台", platformLabel(env)},
		{"缩放", scaleLabel(env.UIScale)},
		{"主题", orNone(env.Theme)},
		{"Locale", orNone(env.Locale)},
		{"编辑器字体", orNone(env.EditorFont)},
		{"Git", gitVersionLabel(env)},
		{"配置目录", orNone(env.ConfigDir)},
		{"日志级别", orNone(env.LogLevel)},
		{"录制中", boolLabel(env.Recording)},
		{"设置 schema", schemaLabel(env.SettingsSchema)},
	}
	for _, row := range rows {
		fmt.Fprintf(b, "| %s | %s |\n", mdCell(row[0], summaryCellRunes), mdCell(row[1], summaryCellRunes))
	}
}

// writeSettingsTable renders the non-secret settings summary.
func writeSettingsTable(b *strings.Builder, settings SettingsSummary) {
	b.WriteString("\n### 设置摘要\n\n")
	if len(settings) == 0 {
		b.WriteString("（无）\n")
		return
	}
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("| 设置项 | 值 |\n|---|---|\n")
	for _, k := range keys {
		fmt.Fprintf(b, "| %s | %s |\n",
			mdCell(k, summaryCellRunes), mdCell(valueString(settings[k]), summaryCellRunes))
	}
}

// writeReposTable renders the redacted repository identities.
func writeReposTable(b *strings.Builder, repos []RepoInfo) {
	b.WriteString("\n### 仓库\n\n")
	if len(repos) == 0 {
		b.WriteString("（无）\n")
		return
	}
	b.WriteString("| 哈希 | 名称 | 分支 | 操作 | 未提交 | 最近刷新 |\n|---|---|---|---|---|---|\n")
	for _, r := range repos {
		name := "未包含"
		if r.Name != "" {
			name = r.Name
		}
		dirty := boolLabel(r.Dirty)
		refreshed := "无"
		if !r.RefreshedAt.IsZero() {
			refreshed = formatStamp(r.RefreshedAt)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n",
			mdCell(orNone(r.Hash), summaryCellRunes),
			mdCell(name, summaryCellRunes),
			mdCell(orNone(r.Branch), summaryCellRunes),
			mdCell(orNone(r.Operation), summaryCellRunes),
			dirty,
			mdCell(refreshed, summaryCellRunes))
	}
}

// isErrorRecord reports whether r is an error-level event.
func isErrorRecord(r observability.Record) bool {
	return r.Level == observability.LevelError.String()
}

// isTimelineRecord reports whether r belongs in the error/write timeline.
func isTimelineRecord(r observability.Record) bool {
	if isErrorRecord(r) || r.Level == observability.LevelWarn.String() {
		return true
	}
	if write, ok := r.Data["write"].(bool); ok && write {
		return true
	}
	if r.Event != observability.EventGitEnd {
		return false
	}
	return writeOps[r.Op]
}

// writeOps is the fallback write-operation set for records that predate the
// structured "write" flag (kept in sync with internal/app's git logging).
var writeOps = map[string]bool{
	"add": true, "am": true, "apply": true, "branch": true,
	"checkout": true, "cherry-pick": true, "clone": true, "commit": true,
	"fetch": true, "filter-branch": true, "gc": true, "merge": true,
	"mv": true, "pull": true, "push": true, "rebase": true,
	"replace": true, "reset": true, "restore": true, "revert": true,
	"rm": true, "stash": true, "submodule": true, "switch": true,
	"tag": true, "worktree": true,
}

// versionSummary renders "0.1.0 (abc1234，2026-11-03 构建)".
func versionSummary(env Env) string {
	version := orNone(env.AppVersion)
	details := make([]string, 0, 2)
	if env.Commit != "" {
		details = append(details, env.Commit)
	}
	if env.BuildDate != "" {
		details = append(details, env.BuildDate+" 构建")
	}
	if len(details) > 0 {
		version += "（" + strings.Join(details, "，") + "）"
	}
	return version
}

// platformSummary renders GOOS/GOARCH, scale, theme and locale.
func platformSummary(env Env) string {
	return fmt.Sprintf("%s · 缩放 %s · 主题 %s · locale %s",
		platformLabel(env), scaleLabel(env.UIScale), orNone(env.Theme), orNone(env.Locale))
}

// gitSummary renders the Git/Viva/Go/settings-schema row.
func gitSummary(env Env) string {
	return fmt.Sprintf("Git：%s · Viva：%s · Go：%s · 设置 schema：%s",
		gitVersionLabel(env), orNone(env.VivaVersion), orNone(env.GoVersion), schemaLabel(env.SettingsSchema))
}

// windowSummary renders the export time and the incident window.
func windowSummary(in summaryInput) string {
	out := "导出时间：" + formatStamp(in.generatedAt) +
		" · 窗口：" + formatStamp(in.windowStart) + "–" + formatStamp(in.windowEnd)
	if in.opts.IncidentID != "" {
		out += "（诊断会话 " + in.opts.IncidentID + "）"
	}
	return out
}

// platformLabel renders GOOS/GOARCH, or 无 when unknown.
func platformLabel(env Env) string {
	platform := strings.Trim(strings.Join([]string{env.GOOS, env.GOARCH}, "/"), "/")
	if platform == "" {
		return "无"
	}
	return platform
}

// scaleLabel renders the UI scale, or 无 when unknown.
func scaleLabel(scale float64) string {
	if scale <= 0 {
		return "无"
	}
	return fmt.Sprintf("%.2f", scale)
}

// gitVersionLabel renders the Git version and where it came from.
func gitVersionLabel(env Env) string {
	label := gitSourceLabel(env.GitSource)
	if env.GitVersion == "" && label == "" {
		return "无"
	}
	version := orNone(env.GitVersion)
	if label != "" {
		version += "（" + label + "）"
	}
	return version
}

// gitSourceLabel translates Env.GitSource into a Chinese description.
func gitSourceLabel(source string) string {
	switch source {
	case "":
		return ""
	case "settings":
		return "来自 settings"
	case "path":
		return "来自 PATH"
	case "missing":
		return "未找到"
	default:
		return source
	}
}

// schemaLabel renders the settings schema version.
func schemaLabel(schema int) string {
	if schema <= 0 {
		return "无"
	}
	return strconv.Itoa(schema)
}

// boolLabel renders a boolean fact in Chinese.
func boolLabel(v bool) string {
	if v {
		return "是"
	}
	return "否"
}

// valueString renders one settings value for the summary table.
func valueString(v any) string {
	switch typed := v.(type) {
	case nil:
		return "无"
	case string:
		if strings.TrimSpace(typed) == "" {
			return "无"
		}
		return typed
	case bool:
		return boolLabel(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case json.Number:
		return string(typed)
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return "无"
		}
		return string(raw)
	}
}

// recordDataString returns one scalar field of Record.Data as display text.
func recordDataString(r observability.Record, key string) string {
	if r.Data == nil {
		return ""
	}
	return valueString(r.Data[key])
}

// clockTime renders HH:MM:SS in UTC (records are stored in UTC).
func clockTime(t time.Time) string {
	if t.IsZero() {
		return "--:--:--"
	}
	return t.UTC().Format("15:04:05")
}

// formatStamp renders a timestamp as UTC RFC 3339, or 无 for the zero value.
func formatStamp(t time.Time) string {
	if t.IsZero() {
		return "无"
	}
	return t.UTC().Format(time.RFC3339)
}

// fmtDurationMS renders milliseconds the way a human reads them, or "" when
// unknown. Sub-second values keep millisecond precision.
func fmtDurationMS(ms int64) string {
	if ms <= 0 {
		return ""
	}
	d := time.Duration(ms) * time.Millisecond
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(100 * time.Millisecond).String()
}

// orNone returns "无" instead of an empty string, so an empty bundle still
// reads as a statement instead of a blank line.
func orNone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "无"
	}
	return s
}

// firstLine returns the first line of s, trimmed.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// indentContinuation prefixes every line after the first with indent.
func indentContinuation(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

// mdCell makes one Markdown table cell from arbitrary text.
func mdCell(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "无"
	}
	s = strings.NewReplacer("\r", " ", "\n", " ", "|", "\\|").Replace(s)
	return truncateRunes(s, maxRunes)
}

// truncateRunes cuts s after maxRunes runes, marking the cut with an ellipsis.
func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
