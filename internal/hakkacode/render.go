package hakkacode

import (
	"encoding/json"
	"fmt"
	"strings"

	"hakka-code/internal/hakkacode/protocol"
)

// This file holds pure, string-returning render helpers shared by the
// TUI. Nothing here touches stdout directly — the TUI owns all terminal
// output via its Bubble Tea View(), so these just build the text that
// gets appended to the transcript.

// renderStatusline formats a dim one-line summary after each turn:
// model, context/token usage, and running cost.
func renderStatusline(stats *protocol.TurnStats) string {
	if stats == nil {
		return ""
	}
	return dimf("%s · %d tokens (ctx ~%dk) · $%.4f · %d msgs\n",
		stats.Model, stats.TotalTokens, (stats.EstimatedContextTokens+999)/1000, stats.TotalCost, stats.MessageCount)
}

// toolsLabel summarizes the currently in-flight tool calls for the
// spinner label. Multiple calls can be running concurrently (the engine
// fans out tool execution), so this collapses to a count once there's
// more than one.
func toolsLabel(running map[string]protocol.ResponseFrame) string {
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
func trackedSessionName(cmd string, frame protocol.ResponseFrame) (name string, ok bool) {
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
// "cmd" request as a plain string. Interactive regions (for clickable
// lists) are discarded; use appendCommandResult instead if you need them.
func renderCommandResult(cmd string, frame protocol.ResponseFrame) string {
	return renderCommandResultInteractive(cmd, frame).text
}

// replayEvents renders a sequence of stored history events (from the
// Events replay) into a human-readable string. Tool events use the same
// render path as live tool frames (renderToolEvent).
func replayEvents(events []map[string]any) string {
	var b strings.Builder
	// Track tool starts so we can show diffs for edit_file errors.
	toolStarts := map[string]protocol.ResponseFrame{}

	for _, evt := range events {
		frame := eventToResponseFrame(evt)
		switch frame.Type {
		case protocol.TypeChat:
			if frame.Text != "" {
				b.WriteString(renderUserPrompt("❯ " + frame.Text))
			}
		case protocol.TypeDelta:
			if frame.Text != "" {
				b.WriteString(renderMarkdown(frame.Text))
				b.WriteByte('\n')
			}
		case protocol.TypeTool:
			switch frame.Status {
			case protocol.StatusStart:
				if frame.ID != "" {
					toolStarts[frame.ID] = frame
				}
			default:
				var startFrame *protocol.ResponseFrame
				if s, ok := toolStarts[frame.ID]; ok {
					startFrame = &s
				}
				delete(toolStarts, frame.ID)
				out := renderToolEvent(startFrame, frame)
				if out != "" {
					b.WriteString(out)
				}
			}
		case protocol.TypeDone:
			// Done marker in history is a terminal — no text/stats.
		}
	}
	return b.String()
}

func renderSessionFrame(frame protocol.ResponseFrame) string {
	var b strings.Builder
	if frame.Session != nil {
		b.WriteString(formatSessionDetail(frame.Session))
	}
	// Prefer Events replay over Messages for rendering history, since
	// events carry args/snippet in the same wire format as live turns.
	if len(frame.Events) > 0 {
		b.WriteString(replayEvents(frame.Events))
	} else if len(frame.Messages) > 0 {
		b.WriteString(formatMessageHistory(frame.Messages))
	}
	return b.String()
}

// renderToolEvent renders a tool lifecycle frame.
//
// For "start", it returns nothing (the caller records the frame in
// toolStarts).  For "ok" it returns a one-line confirmation.  For "err"
// it returns the full picture (header, diff/preview if startFrame is
// provided, error detail).
//
// Snippet and args are taken from the startFrame (paired by id).
// Completion frames (ok/err) do not carry this data.
func renderToolEvent(startFrame *protocol.ResponseFrame, frame protocol.ResponseFrame) string {
	name := toolNameFromFrame(frame)
	if name == "" {
		name = "tool"
	}

	// Always pull snippet from the start frame.
	var snippet string
	if startFrame != nil {
		snippet = toolSnippet(*startFrame)
	}

	var b strings.Builder
	switch frame.Status {
	case "ok":
		if snippet != "" {
			fmt.Fprintf(&b, "✓ %s · %s\n", name, snippet)
		} else {
			fmt.Fprintf(&b, "✓ %s\n", name)
		}
		// For successful edit_file, include the diff.
		if name == "edit_file" && startFrame != nil {
			b.WriteString(renderEditFileDiff(startFrame.Args))
		}
		// For successful write_file, include a preview.
		if name == "write_file" && startFrame != nil {
			b.WriteString(renderWriteFilePreview(startFrame.Args))
		}
	case "err":
		if snippet != "" {
			fmt.Fprintf(&b, "\n⏺ %s · %s\n", name, snippet)
		} else {
			fmt.Fprintf(&b, "\n⏺ %s\n", name)
		}
		if startFrame != nil {
			switch name {
			case "edit_file":
				b.WriteString(renderEditFileDiff(startFrame.Args))
			case "write_file":
				b.WriteString(renderWriteFilePreview(startFrame.Args))
			}
		}
		b.WriteString("  ✗ err\n")
		if frame.Error != "" {
			fmt.Fprintf(&b, "    %s\n", frame.Error)
		}
	}
	return b.String()
}

// toolNameFromFrame extracts the tool name from a response frame,
// trying the known fields.
func toolNameFromFrame(frame protocol.ResponseFrame) string {
	if frame.Tool != "" {
		return frame.Tool
	}
	if frame.Command != "" {
		return frame.Command
	}
	return ""
}

// toolSnippet prefers the server-provided human-readable snippet, falling
// back to a compacted JSON dump of the args.
func toolSnippet(frame protocol.ResponseFrame) string {
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
	diffContext  = 2 // lines of surrounding context in unified diff
)

// renderEditFileDiff shows a compact unified-diff preview of an edit_file
// call's old/new arguments, so the user can eyeball the change without
// staring at raw JSON. Uses git-style unified diff with minimal context.
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
	return unifiedDiff(p.Old, p.New, diffContext)
}

// Diff types for unified diff generation.
type diffOp int

const (
	diffSame diffOp = iota
	diffDel
	diffIns
)

type diffEdit struct {
	op    diffOp
	line  string
	oldN  int // 1-based line number in old (0 = none)
	newN  int // 1-based line number in new (0 = none)
}

// unifiedDiff produces a git-style unified diff between oldText and newText
// with the given number of context lines around each change.
func unifiedDiff(oldText, newText string, context int) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	edits := diffLCS(oldLines, newLines)

	// Find change boundaries: indices in edits where changes start/end.
	type changeRange struct {
		start, end int // indices into edits (inclusive start, exclusive end)
	}
	var changes []changeRange
	for i := 0; i < len(edits); i++ {
		if edits[i].op != diffSame {
			start := i
			for i < len(edits) && edits[i].op != diffSame {
				i++
			}
			changes = append(changes, changeRange{start, i})
			i-- // loop will increment
		}
	}

	if len(changes) == 0 {
		return ""
	}

	// Merge changes that are close enough (within 2*context of each other).
	var merged []changeRange
	current := changes[0]
	for i := 1; i < len(changes); i++ {
		gap := changes[i].start - current.end
		if gap <= 2*context {
			// Merge: extend current to cover both.
			current.end = changes[i].end
		} else {
			merged = append(merged, current)
			current = changes[i]
		}
	}
	merged = append(merged, current)

	// Render each merged hunk.
	var b strings.Builder
	for _, m := range merged {
		// Expand with context.
		hunkStart := m.start - context
		if hunkStart < 0 {
			hunkStart = 0
		}
		hunkEnd := m.end + context
		if hunkEnd > len(edits) {
			hunkEnd = len(edits)
		}

		hunkEdits := edits[hunkStart:hunkEnd]

		// Compute hunk header line numbers.
		first := hunkEdits[0]
		last := hunkEdits[len(hunkEdits)-1]
		oldStart := first.oldN
		newStart := first.newN
		if oldStart == 0 {
			oldStart = 1
		}
		if newStart == 0 {
			newStart = 1
		}
		oldCount := last.oldN - first.oldN + 1
		newCount := last.newN - first.newN + 1
		if oldCount < 1 {
			oldCount = 1
		}
		if newCount < 1 {
			newCount = 1
		}

		fmt.Fprintf(&b, "  %s@@ -%d,%d +%d,%d @@%s\n",
			sgrCyan, oldStart, oldCount, newStart, newCount, sgrReset)

		for _, e := range hunkEdits {
			line := truncateLine(e.line, 120)
			switch e.op {
			case diffSame:
				fmt.Fprintf(&b, "  %s %s%s\n", sgrDim, line, sgrReset)
			case diffDel:
				fmt.Fprintf(&b, "  %s-%s%s\n", sgrRed, line, sgrReset)
			case diffIns:
				fmt.Fprintf(&b, "  %s+%s%s\n", sgrGreen, line, sgrReset)
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// diffLCS computes a simple LCS-based diff between two string slices.
// Returns a sequence of edit operations.
func diffLCS(a, b []string) []diffEdit {
	// Build LCS table.
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce edit sequence.
	var edits []diffEdit
	i, j := m, n
	var backtrack func(i, j int)
	backtrack = func(i, j int) {
		if i == 0 && j == 0 {
			return
		}
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			backtrack(i-1, j-1)
			edits = append(edits, diffEdit{op: diffSame, line: a[i-1], oldN: i, newN: j})
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			backtrack(i, j-1)
			edits = append(edits, diffEdit{op: diffIns, line: b[j-1], oldN: 0, newN: j})
		} else if i > 0 {
			backtrack(i-1, j)
			edits = append(edits, diffEdit{op: diffDel, line: a[i-1], oldN: i, newN: 0})
		}
	}
	backtrack(i, j)
	return edits
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
	return compactJSONStr(string(raw))
}

func compactJSONStr(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		if len(s) > 160 {
			return s[:157] + "..."
		}
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		if len(s) > 160 {
			return s[:157] + "..."
		}
		return s
	}
	if len(b) > 160 {
		return string(b[:157]) + "..."
	}
	return string(b)
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
	return previewLines(p.Content, " ", sgrDim)
}

// previewLines shows the first few lines of text with a prefix and color.
func previewLines(text, prefix, color string) string {
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
		fmt.Fprintf(&b, "  %s%s %s%s\n", color, prefix, l, sgrReset)
	}
	if extra > 0 {
		fmt.Fprintf(&b, "  %s... (%d more lines)%s\n", sgrDim, extra, sgrReset)
	}
	return b.String()
}

// truncateLine truncates a line to maxCols visible characters (excluding
// ANSI escape codes) to prevent viewport overflow. Appends "…" if truncated.
func truncateLine(line string, maxCols int) string {
	// Strip ANSI codes for length calculation.
	clean := stripANSI(line)
	if len(clean) <= maxCols {
		return line
	}
	// Find the truncation point in the original string.
	// We need to count visible characters, skipping ANSI codes.
	var visible int
	truncAt := len(line)
	for i := 0; i < len(line); {
		if line[i] == '\033' {
			// Skip ANSI escape sequence.
			end := strings.IndexByte(line[i:], 'm')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}
		visible++
		if visible > maxCols-1 { // -1 for "…"
			truncAt = i
			break
		}
		i++
	}
	return line[:truncAt] + "…" + sgrReset
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\033' {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				b.WriteByte(s[i])
				i++
				continue
			}
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
