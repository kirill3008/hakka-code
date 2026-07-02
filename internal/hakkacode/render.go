package hakkacode

import (
	"encoding/json"
	"fmt"
	"strings"
)

// This file holds pure, string-returning render helpers shared by the
// TUI. Nothing here touches stdout directly — the TUI owns all terminal
// output via its Bubble Tea View(), so these just build the text that
// gets appended to the transcript.

// renderStatusline formats a dim one-line summary after each turn:
// model, context/token usage, and running cost.
func renderStatusline(stats *TurnStats) string {
	if stats == nil {
		return ""
	}
	line := fmt.Sprintf("%s · %d tokens (ctx ~%dk) · $%.4f · %d msgs",
		stats.Model, stats.TotalTokens, (stats.EstimatedContextTokens+999)/1000, stats.TotalCost, stats.MessageCount)
	return fmt.Sprintf("\033[2m%s\033[0m\n", line)
}

// toolsLabel summarizes the currently in-flight tool calls for the
// spinner label. Multiple calls can be running concurrently (the engine
// fans out tool execution), so this collapses to a count once there's
// more than one.
func toolsLabel(running map[string]ResponseFrame) string {
	switch len(running) {
	case 0:
		return "Thinking"
	case 1:
		for _, f := range running {
			if f.Tool != "" {
				return "Running " + f.Tool
			}
		}
		return "Running tool"
	default:
		return fmt.Sprintf("Running %d tools", len(running))
	}
}

// trackedSessionName extracts the session name from a slash-command
// response, for commands that create/switch/rename a session — so the
// caller can keep its "does this session already have a name" state in
// sync and avoid clobbering it with an unwanted auto-rename later.
//
// ok is false when cmd doesn't carry session-name information at all
// (the caller should leave its existing state untouched).
func trackedSessionName(cmd string, frame ResponseFrame) (name string, ok bool) {
	var session map[string]any
	switch cmd {
	case "session_create", "get_session":
		session = frame.Session
	case "session_rename":
		session, _ = frame.Data["session"].(map[string]any)
	default:
		return "", false
	}
	if session == nil {
		return "", false
	}
	name, _ = session["name"].(string)
	return name, true
}

// resultHeaders labels a command's output so it's clear what's being
// shown, especially right after other output with no blank line yet.
var resultHeaders = map[string]string{
	"session_list":       "sessions",
	"session_info":       "session",
	"session_create":     "session",
	"get_session":        "session",
	"session_rename":     "session",
	"session_autorename": "session",
	"model_list":         "models",
	"tool_list":          "tools",
}

// renderCommandResult renders the response to a slash-command-triggered
// "cmd" request.
func renderCommandResult(cmd string, frame ResponseFrame) string {
	if frame.Error != "" {
		return fmt.Sprintf("error: %s\n", frame.Error)
	}

	var body string
	switch frame.Type {
	case "session":
		body = renderSessionFrame(frame)
	case "result":
		body = renderData(cmd, frame.Data)
	case "done":
		if frame.Text != "" {
			body = frame.Text + "\n"
		} else {
			body = fmt.Sprintf("%s: ok\n", cmd)
		}
	default:
		body = fmt.Sprintf("%s: ok\n", cmd)
	}

	if header, ok := resultHeaders[cmd]; ok {
		return fmt.Sprintf("\033[1m%s\033[0m\n%s", header, body)
	}
	return body
}

func renderSessionFrame(frame ResponseFrame) string {
	var b strings.Builder
	if frame.Session != nil {
		b.WriteString(formatSessionDetail(frame.Session))
	}
	if len(frame.Messages) > 0 {
		b.WriteString(formatMessageHistory(frame.Messages))
	}
	return b.String()
}

// renderToolEvent renders a tool lifecycle frame.
//
// A successful call collapses to a single confirmation line — no
// separate "start" header, no diff preview — since there's nothing to
// act on. A failed call gets the full picture (header, diff/preview,
// error) so there's enough context to debug it; toolStarts (keyed by
// call ID) buffers "start" frames so that context is available once the
// matching "err" frame arrives.
func renderToolEvent(toolStarts map[string]ResponseFrame, frame ResponseFrame) string {
	name := frame.Tool
	if name == "" {
		name = "tool"
	}

	var b strings.Builder
	switch frame.Status {
	case "start":
		if frame.ID != "" {
			toolStarts[frame.ID] = frame
		}
		return ""
	case "ok":
		delete(toolStarts, frame.ID)
		snippet := toolSnippet(frame)
		if snippet != "" {
			fmt.Fprintf(&b, "✓ %s · %s\n", name, snippet)
		} else {
			fmt.Fprintf(&b, "✓ %s\n", name)
		}
	case "err":
		start, hadStart := toolStarts[frame.ID]
		delete(toolStarts, frame.ID)
		snippet := toolSnippet(frame)
		if snippet != "" {
			fmt.Fprintf(&b, "\n⏺ %s · %s\n", name, snippet)
		} else {
			fmt.Fprintf(&b, "\n⏺ %s\n", name)
		}
		if hadStart {
			switch frame.Tool {
			case "edit_file":
				b.WriteString(renderEditFileDiff(start.Args))
			case "write_file":
				b.WriteString(renderWriteFilePreview(start.Args))
			}
		}
		b.WriteString("  ✗ err\n")
		if frame.Error != "" {
			fmt.Fprintf(&b, "    %s\n", frame.Error)
		}
	}
	return b.String()
}

// toolSnippet prefers the server-provided human-readable snippet, falling
// back to a compacted JSON dump of the args.
func toolSnippet(frame ResponseFrame) string {
	if frame.Snippet != "" {
		return frame.Snippet
	}
	if len(frame.Args) > 0 {
		return compactJSON(frame.Args)
	}
	return ""
}

const (
	diffMaxLines = 20
	diffMaxChars = 4000
)

// renderEditFileDiff shows a compact +/- preview of an edit_file call's
// old/new arguments, so the user can eyeball the change without staring
// at raw JSON.
func renderEditFileDiff(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var p struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(diffLines("-", p.Old, "\033[31m"))
	b.WriteString(diffLines("+", p.New, "\033[32m"))
	return b.String()
}

// renderWriteFilePreview shows the first few lines of a write_file call's
// content argument.
func renderWriteFilePreview(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var p struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return ""
	}
	return diffLines(" ", p.Content, "\033[2m")
}

func diffLines(prefix, text, color string) string {
	if text == "" {
		return ""
	}
	if len(text) > diffMaxChars {
		text = text[:diffMaxChars] + "\n[TRUNCATED]"
	}
	lines := strings.Split(text, "\n")
	shown := lines
	extra := 0
	if len(lines) > diffMaxLines {
		shown = lines[:diffMaxLines]
		extra = len(lines) - diffMaxLines
	}
	var b strings.Builder
	for _, l := range shown {
		fmt.Fprintf(&b, "  %s%s %s\033[0m\n", color, prefix, l)
	}
	if extra > 0 {
		fmt.Fprintf(&b, "  \033[2m... (%d more lines)\033[0m\n", extra)
	}
	return b.String()
}

func renderData(cmd string, data map[string]any) string {
	if len(data) == 0 {
		return fmt.Sprintf("%s: ok\n", cmd)
	}
	switch cmd {
	case "session_list":
		return formatSessionList(data)
	case "model_list":
		return formatModelList(data)
	case "tool_list":
		return formatToolList(data)
	case "session_info", "session_rename", "session_autorename":
		if s, ok := data["session"].(map[string]any); ok {
			return formatSessionDetail(s)
		}
	case "cwd_set":
		return fmt.Sprintf("cwd set to %s\n", strField(data, "cwd"))
	case "compact":
		if n := numField(data, "compact_soft_limit"); n > 0 {
			return fmt.Sprintf("compact soft limit: %d tokens\n", int(n))
		}
		return "compact soft limit: unset\n"
	case "session_delete":
		if boolField(data, "active_cleared") {
			return fmt.Sprintf("deleted session %s (was active)\n", strField(data, "deleted"))
		}
		return fmt.Sprintf("deleted session %s\n", strField(data, "deleted"))
	case "tool_allow":
		return fmt.Sprintf("allowed: %s\n", strings.Join(stringItems(data["allowed"]), ", "))
	case "tool_deny":
		return fmt.Sprintf("denied: %s\n", strings.Join(stringItems(data["denied"]), ", "))
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("%s: ok\n", cmd)
	}
	return fmt.Sprintf("%s:\n%s\n", cmd, string(b))
}

func compactJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	if len(b) > 160 {
		return string(b[:157]) + "..."
	}
	return string(b)
}
