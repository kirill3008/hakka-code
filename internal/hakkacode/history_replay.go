package hakkacode

import (
	"encoding/json"

	"hakka-code/internal/hakkacode/protocol"
)

// eventToResponseFrame converts a stored history event (from the
// server's Events replay) into a protocol.ResponseFrame so it can be
// processed through handleFrame — the same code path as live streaming.
//
// Event types:
//   "chat"    → user prompt (handled separately by the caller)
//   "delta"   → assistant text delta
//   "tool"    → tool start/ok/err (includes args, snippet, result/error)
//   "usage"   → token usage
//   "done"    → turn completion
func eventToResponseFrame(evt map[string]any) protocol.ResponseFrame {
	typ, _ := evt["type"].(string)

	frame := protocol.ResponseFrame{Type: typ}

	// Extract server-provided timestamp (milliseconds since epoch).
	if ts, ok := evt["ts"].(float64); ok {
		frame.Timestamp = int64(ts)
	}

	switch typ {
	case "chat":
		frame.Type = protocol.TypeChat
		if s, ok := evt["text"].(string); ok {
			frame.Text = s
		}

	case "delta":
		frame.Type = protocol.TypeDelta
		if s, ok := evt["text"].(string); ok {
			frame.Text = s
		}

	case "tool":
		frame.Type = protocol.TypeTool
		if s, ok := evt["id"].(string); ok {
			frame.ID = s
		}
		if s, ok := evt["tool"].(string); ok {
			frame.Tool = s
		}
		if s, ok := evt["status"].(string); ok {
			frame.Status = s
		}
		if s, ok := evt["snippet"].(string); ok {
			frame.Snippet = s
		}
		if args, ok := evt["args"]; ok {
			b, err := json.Marshal(args)
			if err == nil {
				frame.Args = b
			}
		}
		// Tool result (ok) or error (err).
		if s, ok := evt["result"].(string); ok {
			frame.ToolResult = s
		}
		if s, ok := evt["error"].(string); ok {
			frame.Error = s
		}

	case "usage":
		frame.Type = protocol.TypeUsage

	case "done":
		frame.Type = protocol.TypeDone
	}

	return frame
}
