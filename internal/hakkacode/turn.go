package hakkacode

import (
	"encoding/json"
	"strings"

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
	toolStarts   map[string]protocol.ResponseFrame
	sawTool      bool
	assistantBuf strings.Builder
	flushedText  bool
}

// newTurnState initialises an empty turn.
func newTurnState() *turnState {
	return &turnState{
		toolStarts: make(map[string]protocol.ResponseFrame),
	}
}

// active reports whether a turn is in flight.
func (ts *turnState) active() bool { return ts != nil }

// addDelta appends a text delta chunk to the assistant buffer.
func (ts *turnState) addDelta(text string) {
	ts.assistantBuf.WriteString(text)
}

// recordToolStart remembers the start frame for a tool invocation so it
// can be paired with the completion frame later.
func (ts *turnState) recordToolStart(frame protocol.ResponseFrame) {
	if frame.ID != "" {
		ts.toolStarts[frame.ID] = frame
	}
}

// finishTool drains the pending tool start for the given id and returns
// it (so the caller can extract args/snippet), then removes it from the
// map. Returns nil if there was no matching start.
func (ts *turnState) finishTool(id string) *protocol.ResponseFrame {
	start, ok := ts.toolStarts[id]
	delete(ts.toolStarts, id)
	if ok {
		return &start
	}
	return nil
}

// runningCount returns the number of tools still awaiting completion.
func (ts *turnState) runningCount() int { return len(ts.toolStarts) }

// toolsLabel returns a spinner label describing the currently running tools.
func (ts *turnState) toolsLabel() string {
	return toolsLabel(ts.toolStarts)
}

// flushAssistant returns any accumulated assistant delta text as a
// transcript entry, adding a spacer if we previously output tool calls.
// The internal buffer is cleared.
func (ts *turnState) flushAssistant(appendFn func(e *transcript.TranscriptEntry)) {
	if ts.assistantBuf.Len() == 0 {
		return
	}
	if ts.sawTool {
		appendFn(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
	}
	appendFn(&transcript.TranscriptEntry{
		Type: transcript.EntryAssistantText,
		Raw:  strings.TrimRight(renderMarkdown(ts.assistantBuf.String()), "\n") + "\n",
	})
	ts.assistantBuf.Reset()
	ts.flushedText = true
}

// appendRemainingText flushes any remaining delta buffer if no deltas
// were rendered (e.g. a cached/historical turn where full text arrived
// in the done frame).
func (ts *turnState) appendRemainingText(text string, appendFn func(e *transcript.TranscriptEntry)) {
	if ts.flushedText {
		return
	}
	if text == "" {
		return
	}
	if ts.sawTool {
		appendFn(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
	}
	appendFn(&transcript.TranscriptEntry{
		Type: transcript.EntryAssistantText,
		Raw:  strings.TrimRight(renderMarkdown(text), "\n") + "\n",
	})
	ts.assistantBuf.Reset()
	ts.flushedText = true
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

	var raw string
	if status == transcript.ToolOK {
		if snippet != "" {
			raw = "✓ " + name + " · " + snippet
		} else {
			raw = "✓ " + name
		}
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
