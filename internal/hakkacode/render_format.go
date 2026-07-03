package hakkacode

import (
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

// formatMessageHistory replays stored messages when Events replay is
// not available (legacy servers). Prefer the Events field when present;
// it carries args/snippet in wire format matching live streaming.
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
			snippet := strField(m, "snippet")
			if snippet == "" {
				snippet = strField(m, "result")
			}
			if snippet == "" {
				// Try to extract a human-readable snippet from args.
				if args, ok := m["args"]; ok {
					snippet = toolArgsSnippet(name, args)
				}
			}
			if snippet == "" {
				snippet = compactJSONStr(content)
			}

			if status == "err" || status == "error" || errText != "" {
				if errText == "" {
					errText = content
				}
				if snippet != "" {
					fmt.Fprintf(&b, "✗ %s · %s: %s\n", name, snippet, errText)
				} else {
					fmt.Fprintf(&b, "✗ %s: %s\n", name, errText)
				}
			} else if snippet != "" {
				fmt.Fprintf(&b, "✓ %s · %s\n", name, snippet)
			} else {
				fmt.Fprintf(&b, "✓ %s\n", name)
			}
		}
	}
	return b.String()
}
