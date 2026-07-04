package hakkacode

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"hakka-code/internal/hakkacode/protocol"
	"hakka-code/internal/hakkacode/transcript"
)

// turnState tracks the state of an in-flight assistant turn — accumulated
// delta text, pending tool starts, and whether we need a spacer before the
// next prose block.
//
// It lives on model and is only non-nil while a turn is active. When the
// turn ends (done/cancelled/error) the state is consumed and set back to
// nil.
type turnState struct {
	toolStarts     map[string]protocol.ResponseFrame
	toolStartTimes map[string]time.Time
	sawTool        bool
	stream         *streamingText // nil when no deltas have arrived yet

	// lazy, when true, skips markdown rendering during addDelta and
	// defers it to streamFinalize. Used for history replay during boot
	// where per-delta rendering is wasted (viewport isn't shown yet).
	lazy bool
}

// streamingText tracks the in-progress assistant prose as deltas arrive.
// Deltas are shown immediately as plain text (no markdown rendering until
// the stream is finalised) by mutating the last transcript entry in-place.
type streamingText struct {
	rawBuf strings.Builder
	entry  *transcript.TranscriptEntry // pointer into transcript.entries slice
}

// newTurnState initialises an empty turn.
func newTurnState() *turnState {
	return &turnState{
		toolStarts:     make(map[string]protocol.ResponseFrame),
		toolStartTimes: make(map[string]time.Time),
	}
}

// newLazyTurnState initialises a turn that defers markdown rendering to
// the end — used for history replay during boot.
func newLazyTurnState() *turnState {
	ts := newTurnState()
	ts.lazy = true
	return ts
}

// active reports whether a turn is in flight.
func (ts *turnState) active() bool { return ts != nil }

// streaming reports whether deltas have started arriving.
func (ts *turnState) streaming() bool { return ts.stream != nil }

// addDelta appends a text delta chunk. The first delta creates a new
// transcript entry; subsequent deltas mutate it in-place so the viewport
// updates immediately without creating duplicate entries.
//
// appendFn is called only for the first delta (to insert a new entry).
// updateFn is called for every delta after the first to refresh the
// viewport content.
//
// In lazy mode (history replay), markdown rendering is skipped entirely
// during addDelta and deferred to streamFinalize so the O(n²) cost of
// re-rendering the full accumulated text on every delta is avoided.
func (ts *turnState) addDelta(
	text string,
	appendFn func(e *transcript.TranscriptEntry),
	updateFn func(entry *transcript.TranscriptEntry, lines []string),
) {
	if ts.stream == nil {
		if ts.sawTool {
			appendFn(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
		}

		ts.stream = &streamingText{}
		ts.stream.rawBuf.WriteString(text)

		var rendered []string
		if ts.lazy {
			rendered = strings.Split(text, "\n")
		} else {
			rendered = renderStreaming(text)
		}

		entry := &transcript.TranscriptEntry{
			Type:     transcript.EntryAssistantText,
			Raw:      text,
			Rendered: rendered,
		}
		ts.stream.entry = entry
		appendFn(entry)
	} else {
		ts.stream.rawBuf.WriteString(text)
		full := ts.stream.rawBuf.String()
		entry := ts.stream.entry
		entry.Raw = full

		if ts.lazy {
			entry.Rendered = strings.Split(full, "\n")
		} else {
			entry.Rendered = renderStreaming(full)
		}
		updateFn(entry, entry.Rendered)
	}
}

// streamFinalize renders the accumulated text as markdown if lazy mode
// was used, then clears the streaming state.
func (ts *turnState) streamFinalize(
	updateFn func(entry *transcript.TranscriptEntry, lines []string),
) {
	if ts.stream == nil {
		return
	}
	if ts.lazy {
		raw := strings.TrimRight(ts.stream.rawBuf.String(), "\n") + "\n"
		entry := ts.stream.entry
		entry.Raw = strings.TrimRight(raw, "\n")
		entry.Rendered = renderStreaming(raw)
		updateFn(entry, entry.Rendered)
	}
	ts.stream = nil
}

// streamFinalizeOrAppend handles the done frame. If deltas were streamed,
// it finalises the streaming entry as markdown. If no deltas arrived but
// the done frame carries text (e.g. cached/historical turn), it appends
// the text as a new entry.
func (ts *turnState) streamFinalizeOrAppend(
	doneText string,
	appendFn func(e *transcript.TranscriptEntry),
	updateFn func(entry *transcript.TranscriptEntry, lines []string),
) {
	if ts.stream != nil {
		// Streamed path: polish the in-progress entry.
		ts.streamFinalize(updateFn)
	} else if doneText != "" {
		// Non-streamed path: full text arrived in the done frame.
		if ts.sawTool {
			appendFn(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
		}
		appendFn(&transcript.TranscriptEntry{
			Type: transcript.EntryAssistantText,
			Raw:  strings.TrimRight(renderMarkdown(doneText), "\n") + "\n",
		})
	}
}

// recordToolStart remembers the start frame and wall-clock time for a
// tool invocation so it can be paired with the completion frame later.
func (ts *turnState) recordToolStart(frame protocol.ResponseFrame) {
	if frame.ID != "" {
		ts.toolStarts[frame.ID] = frame
		ts.toolStartTimes[frame.ID] = time.Now()
	}
}

// finishTool drains the pending tool start for the given id and returns
// it (so the caller can extract args/snippet), then removes it from the
// map. Returns nil if there was no matching start.
func (ts *turnState) finishTool(id string) *protocol.ResponseFrame {
	start, ok := ts.toolStarts[id]
	delete(ts.toolStarts, id)
	delete(ts.toolStartTimes, id)
	if ok {
		return &start
	}
	return nil
}

// toolDuration returns the elapsed wall-clock time since the tool start
// frame was recorded. Returns zero if the id was never recorded.
func (ts *turnState) toolDuration(id string) time.Duration {
	t, ok := ts.toolStartTimes[id]
	if !ok {
		return 0
	}
	return time.Since(t)
}

// runningCount returns the number of tools still awaiting completion.
func (ts *turnState) runningCount() int { return len(ts.toolStarts) }

// toolsLabel returns a spinner label describing the currently running tools.
func (ts *turnState) toolsLabel() string {
	return toolsLabel(ts.toolStarts)
}

// appendToolCall creates and appends a tool call transcript entry.
func (ts *turnState) appendToolCall(
	appendFn func(e *transcript.TranscriptEntry),
	frame protocol.ResponseFrame,
	startFrame *protocol.ResponseFrame,
) {
	name := toolNameFromFrame(frame)
	if name == "" {
		name = "tool"
	}

	status := transcript.ToolOK
	if frame.Status == protocol.StatusErr {
		status = transcript.ToolErr
	}

	var snippet string
	var args json.RawMessage
	if startFrame != nil {
		snippet = toolSnippet(*startFrame)
		args = startFrame.Args
	}

	snippet = sanitizeSnippet(snippet)

	// Compute extra info: tool-type-specific details + wall-clock duration
	// (zero for history replays since there's no timing data stored).
	extra := toolInfoSuffix(name, args, frame.ToolResult)
	if d := ts.toolDuration(frame.ID); d > 0 {
		extra += " " + dimf("(%s)", formatDuration(d))
	}

	var raw string
	if status == transcript.ToolOK {
		if snippet != "" {
			raw = "✓ " + name + " · " + snippet
		} else {
			raw = "✓ " + name
		}
		raw += extra
	} else {
		if snippet != "" {
			raw = "✗ " + name + " · " + snippet + ": " + frame.Error
		} else {
			raw = "✗ " + name + ": " + frame.Error
		}
	}

	appendFn(&transcript.TranscriptEntry{
		Type:       transcript.EntryToolCall,
		ToolName:   name,
		ToolID:     frame.ID,
		ToolStatus: status,
		ToolArgs:   args,
		ToolError:  frame.Error,
		Collapsed:  status == transcript.ToolOK,
		Raw:        raw,
	})
}

const (
	bytesPerKB = 1024
	bytesPerMB = 1024 * 1024
)

// toolInfoSuffix returns a compact suffix with tool-type-specific details,
// e.g. " lines 10-25 · 12ms" for read_file, " (+3 -1) · 5ms" for
// edit_file, " (4.2KB) · 3ms" for write_file. The duration is always
// appended last and dimmed, except when it's zero (history replay).
func toolInfoSuffix(name string, args json.RawMessage, result string) string {
	var parts []string

	switch name {
	case "read_file":
		if s := readFileInfo(args, result); s != "" {
			parts = append(parts, s)
		}
	case "edit_file":
		if s := editFileInfo(args); s != "" {
			parts = append(parts, s)
		}
	case "write_file":
		if s := writeFileInfo(args); s != "" {
			parts = append(parts, s)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// readFileInfo extracts the line range from a read_file call. The path
// itself is already shown by the snippet, so this only adds line info.
// Args: {"path": "...", "range_start": N, "range_end": M}
// Result: the file content as a string.
// Returns e.g. "lines 10-25".
func readFileInfo(args json.RawMessage, result string) string {
	if len(args) == 0 || result == "" {
		return ""
	}
	var p struct {
		Path       string `json:"path"`
		RangeStart int    `json:"range_start"`
		RangeEnd   int    `json:"range_end"`
		MaxBytes   int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Path == "" {
		return ""
	}

	// Count lines from the actual result.
	nl := strings.Count(result, "\n")
	totalLines := nl + 1

	if p.RangeEnd > 0 {
		start := p.RangeStart
		if start == 0 {
			// Infer start from range_end and total lines.
			start = p.RangeEnd - totalLines + 1
		}
		if start < 1 {
			start = 1
		}
		return dimf("lines %d-%d", start, p.RangeEnd)
	}

	if p.MaxBytes > 0 && len(result) >= p.MaxBytes {
		return dimf("%d lines (truncated)", totalLines)
	}

	return dimf("%d lines", totalLines)
}

// editFileInfo computes diff line stats from an edit_file call's args.
// Args: {"old": "...", "new": "..."}
// Returns e.g. "(+5 -2)".
func editFileInfo(args json.RawMessage) string {
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
	oldLines := strings.Count(p.Old, "\n")
	if p.Old != "" {
		oldLines++
	}
	newLines := strings.Count(p.New, "\n")
	if p.New != "" {
		newLines++
	}
	if oldLines == newLines && oldLines == 0 {
		return ""
	}
	return dimf("(+%d -%d)", newLines, oldLines)
}

// writeFileInfo computes the byte length of a write_file call's content.
// Args: {"content": "...", "path": "..."}
// Returns e.g. "(4.2KB)" or "(840B)".
func writeFileInfo(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var p struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return ""
	}
	n := utf8.RuneCountInString(p.Content)
	switch {
	case n >= bytesPerMB:
		return dimf("(%.1fMB)", float64(n)/float64(bytesPerMB))
	case n >= bytesPerKB:
		return dimf("(%.1fKB)", float64(n)/float64(bytesPerKB))
	case n > 0:
		return dimf("(%dB)", n)
	default:
		return dimf("(0B)")
	}
}
