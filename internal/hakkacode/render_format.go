package hakkacode

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Human-readable formatters for command results that were previously
// dumped as raw JSON.

func strField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func numField(m map[string]any, key string) float64 {
	switch n := m[key].(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

func boolField(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

// toolArgsSnippet extracts a human-readable snippet from stored tool args.
// name is the tool name; args is the raw args map from the stored message.
func toolArgsSnippet(name string, args any) string {
	m, ok := args.(map[string]any)
	if !ok {
		return ""
	}
	switch name {
	case "shell":
		if c := strField(m, "cmd"); c != "" {
			return c
		}
	case "http_get":
		if u := strField(m, "url"); u != "" {
			return u
		}
	}
	// file tools: show path
	for _, key := range []string{"path", "pattern", "url"} {
		if v := strField(m, key); v != "" {
			return v
		}
	}
	return ""
}

func stringItems(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// localTime converts a backend timestamp (RFC3339, usually UTC) to the
// local timezone. Falls back to the raw string if it doesn't parse.
func localTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// renderPlainTable is a borderless, column-aligned table (like `column
// -t`) — used for command-result listings, as opposed to the bordered
// renderTable used for markdown tables in chat replies.
func renderPlainTable(headers []string, rows [][]string) string {
	t := table.New().
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(_, _ int) lipgloss.Style {
			return lipgloss.NewStyle().Padding(0, 2, 0, 0)
		})
	return t.String() + "\n"
}

// formatSessionDetail formats a session metadata map (session_info,
// session_rename, session_create, get_session).
func formatSessionDetail(s map[string]any) string {
	name := strField(s, "name")
	if name == "" {
		name = "(unnamed)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "session %s · %s\n", strField(s, "short_id"), name)
	if model := strField(s, "model"); model != "" {
		fmt.Fprintf(&b, "  model: %s\n", model)
	}
	fmt.Fprintf(&b, "  messages: %d\n", int(numField(s, "message_count")))
	if tokens := numField(s, "total_tokens"); tokens > 0 {
		fmt.Fprintf(&b, "  tokens: %d\n", int(tokens))
	}
	if cost := numField(s, "total_cost"); cost > 0 {
		fmt.Fprintf(&b, "  cost: $%.4f\n", cost)
	}
	if cwd := strField(s, "client_cwd"); cwd != "" {
		fmt.Fprintf(&b, "  cwd: %s\n", cwd)
	}
	if limit := numField(s, "compact_soft_limit"); limit > 0 {
		fmt.Fprintf(&b, "  compact limit: %d\n", int(limit))
	}
	if updated := strField(s, "updated_at"); updated != "" {
		fmt.Fprintf(&b, "  updated: %s\n", localTime(updated))
	}
	fmt.Fprintf(&b, "  id: %s\n", strField(s, "id"))
	return b.String()
}

func formatSessionList(data map[string]any) string {
	sessions, _ := data["sessions"].([]any)
	if len(sessions) == 0 {
		return "no sessions\n"
	}
	headers := []string{"", "id", "name", "msgs", "updated", ""}
	rows := make([][]string, 0, len(sessions))
	for _, item := range sessions {
		s, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mark := ""
		if boolField(s, "current") {
			mark = "*"
		}
		name := strField(s, "name")
		if name == "" {
			name = "(unnamed)"
		}
		status := ""
		if boolField(s, "in_flight") {
			status = "[running]"
		}
		rows = append(rows, []string{
			mark, strField(s, "id"), name,
			fmt.Sprintf("%d", int(numField(s, "message_count"))),
			localTime(strField(s, "updated_at")), status,
		})
	}
	return renderPlainTable(headers, rows)
}

func formatModelList(data map[string]any) string {
	models, _ := data["models"].([]any)
	headers := []string{"", "name"}
	rows := make([][]string, 0, len(models))
	for _, item := range models {
		mm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mark := ""
		if boolField(mm, "current") {
			mark = "*"
		}
		rows = append(rows, []string{mark, strField(mm, "name")})
	}
	return renderPlainTable(headers, rows)
}

func formatToolList(data map[string]any) string {
	tools, _ := data["tools"].([]any)
	headers := []string{"", "name", "description"}
	rows := make([][]string, 0, len(tools))
	for _, item := range tools {
		tm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mark := ""
		if boolField(tm, "enabled") {
			mark = "x"
		}
		desc := strField(tm, "description")
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		rows = append(rows, []string{mark, strField(tm, "name"), desc})
	}
	return renderPlainTable(headers, rows)
}

// formatMessageHistory replays stored messages mirroring live turn output.
func formatMessageHistory(messages []map[string]any) string {
	var b strings.Builder
	for _, m := range messages {
		role := strField(m, "role")
		content := strField(m, "content")
		switch role {
		case "user":
			b.WriteString(renderUserPrompt("❯ " + content))
		case "assistant":
			b.WriteString(renderMarkdown(content))
			b.WriteString("\n")
		case "tool":
			b.WriteString(formatHistoryToolLine(m))
		}
	}
	return b.String()
}

// formatHistoryToolLine renders a single stored tool message using the
// same visual style as live tool events.
func formatHistoryToolLine(m map[string]any) string {
	name := strField(m, "name")
	if name == "" {
		name = strField(m, "tool_name")
	}
	if name == "" {
		name = strField(m, "tool")
	}
	if name == "" {
		name = "tool"
	}

	status := strField(m, "status")
	errText := strField(m, "error")
	content := strField(m, "content")

	// Extract args — try multiple possible locations/formats.
	var args map[string]any
	switch v := m["args"].(type) {
	case map[string]any:
		args = v
	case string:
		_ = json.Unmarshal([]byte(v), &args)
	}
	// Some servers nest tool params under "content" as JSON.
	if args == nil {
		if s, ok := m["content"].(string); ok {
			_ = json.Unmarshal([]byte(s), &args)
		}
	}

	// Prefer our compact snippet built from args, then fall back to the
	// server-provided snippet/result fields (which may contain full output).
	snippet := buildHistorySnippet(name, args)
	if snippet == "" {
		snippet = strField(m, "snippet")
	}
	if snippet == "" {
		snippet = strField(m, "result")
	}
	if snippet == "" {
		snippet = compactJSONStr(content)
	}

	isErr := status == "err" || status == "error" || errText != ""

	if isErr {
		if errText == "" {
			errText = content
		}
		var sb strings.Builder
		if snippet != "" {
			fmt.Fprintf(&sb, "✗ %s · %s: %s\n", name, snippet, errText)
		} else {
			fmt.Fprintf(&sb, "✗ %s: %s\n", name, errText)
		}
		if name == "edit_file" && args != nil {
			old, _ := args["old"].(string)
			neww, _ := args["new"].(string)
			if d := diffIfSmall(old, neww, 15); d != "" {
				sb.WriteString(d)
			}
		}
		return sb.String()
	}

	if name == "edit_file" && args != nil {
		// For successful edit_file, show diff if small enough.
		old, _ := args["old"].(string)
		neww, _ := args["new"].(string)
		if d := diffIfSmall(old, neww, 15); d != "" {
			if snippet == "" {
				snippet = name
			}
			return fmt.Sprintf("✓ %s · %s\n%s", name, snippet, d)
		}
	}

	if snippet != "" {
		return fmt.Sprintf("✓ %s · %s\n", name, snippet)
	}
	return fmt.Sprintf("✓ %s\n", name)
}

// buildHistorySnippet builds a tool-specific human-readable snippet for
// a stored tool message, matching the server's live snippet style.
func buildHistorySnippet(name string, args map[string]any) string {
	if args == nil {
		return ""
	}

	switch name {
	case "shell":
		if cmd := strField(args, "cmd"); cmd != "" {
			return cmd
		}

	case "read_file":
		path := strField(args, "path")
		if path == "" {
			return ""
		}
		offset, hasOffset := args["offset"].(float64)
		limit, hasLimit := args["limit"].(float64)
		if hasOffset && hasLimit {
			start := int(offset) + 1 // 1-based line numbering
			end := start + int(limit) - 1
			if end > start {
				return fmt.Sprintf("%s %d-%d", path, start, end)
			}
			return fmt.Sprintf("%s @%d+%d", path, int(offset), int(limit))
		}
		if hasOffset {
			return fmt.Sprintf("%s @%d", path, int(offset))
		}
		if hasLimit {
			return fmt.Sprintf("%s (%d lines)", path, int(limit))
		}
		return path

	case "write_file":
		path := strField(args, "path")
		if path == "" {
			return ""
		}
		return path

	case "edit_file":
		path := strField(args, "path")
		if path == "" {
			return ""
		}
		old := strField(args, "old")
		neww := strField(args, "new")
		oldLines := strings.Count(old, "\n") + 1
		newLines := strings.Count(neww, "\n") + 1
		if old == "" {
			return fmt.Sprintf("%s +%d", path, newLines)
		}
		if neww == "" {
			return fmt.Sprintf("%s -%d", path, oldLines)
		}
		return fmt.Sprintf("%s -%d+%d", path, oldLines, newLines)

	case "http_get":
		if u := strField(args, "url"); u != "" {
			return u
		}

	case "search":
		pattern := strField(args, "pattern")
		path := strField(args, "path")
		if pattern != "" && path != "" {
			return fmt.Sprintf("\"%s\" in %s", pattern, path)
		}
		if pattern != "" {
			return fmt.Sprintf("\"%s\"", pattern)
		}
		if path != "" {
			return path
		}
	}

	// Fallback: show path-like fields.
	for _, key := range []string{"path", "pattern", "url"} {
		if v := strField(args, key); v != "" {
			return v
		}
	}
	return ""
}

// diffIfSmall returns a compact +/- diff when the total diff is ≤maxLines.
// Returns "" otherwise (or if both strings are empty).
func diffIfSmall(old, neww string, maxLines int) string {
	if old == "" && neww == "" {
		return ""
	}
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(neww, "\n")
	if len(oldLines)+len(newLines) > maxLines {
		return ""
	}
	var b strings.Builder
	for _, l := range oldLines {
		fmt.Fprintf(&b, "  %s- %s%s\n", sgrRed, l, sgrReset)
	}
	for _, l := range newLines {
		fmt.Fprintf(&b, "  %s+ %s%s\n", sgrGreen, l, sgrReset)
	}
	return b.String()
}
