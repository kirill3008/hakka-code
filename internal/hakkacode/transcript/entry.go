// Package transcript provides the structured scrollback data model that
// enables hit-testing, expand/collapse, text selection, and interactive
// command results.
package transcript

import "encoding/json"

// EntryType discriminates transcript entries for rendering and interaction.
type EntryType int

const (
	// EntrySystem is a boot message, warning, or informational line.
	EntrySystem EntryType = iota

	// EntryUserPrompt is the user's echoed input.
	EntryUserPrompt

	// EntryAssistantText is rendered markdown from the assistant.
	EntryAssistantText

	// EntryToolCall is an expandable tool call (collapsed by default on success).
	EntryToolCall

	// EntryStatusLine is a turn summary (model, tokens, cost).
	EntryStatusLine

	// EntryError is an error message.
	EntryError

	// EntryCommandResult is output from a slash command.
	EntryCommandResult

	// EntrySpacer is a blank line.
	EntrySpacer
)

// ToolStatus captures the outcome of a tool call.
type ToolStatus string

const (
	ToolOK  ToolStatus = "ok"
	ToolErr ToolStatus = "err"
)

// ClickAction encodes an action triggered by clicking a region.
type ClickAction struct {
	Action string // "session-switch", "model-switch", "tool-toggle", "copy"
	Payload string // session id, model name, tool name, or text to copy
}

// ClickRegion is a clickable area within a rendered entry.
type ClickRegion struct {
	Line   int         // line index relative to the entry's rendered output (0-based)
	Col    int         // column offset
	Width  int         // span in columns
	Action ClickAction
}

// TranscriptEntry is a single unit in the scrollback — a user prompt, an
// assistant reply, a tool call, a status line, etc. Each entry caches its
// rendered lines and knows its position in the viewport, enabling O(log n)
// hit-testing and expand/collapse.
type TranscriptEntry struct {
	Type EntryType
	Raw  string // original content, kept for re-render on resize

	// Rendered lines, computed by the Renderer callback when the entry is
	// appended or when the terminal width changes.
	Rendered []string

	// LineOff is the first absolute line index in the full viewport
	// content. Set by Transcript.Rebuild.
	LineOff int

	// ToolCall fields (valid when Type == EntryToolCall).
	ToolName   string
	ToolID     string
	ToolStatus ToolStatus
	ToolArgs   json.RawMessage // for diff/preview on expand
	ToolError  string

	// Collapsed is true when a tool call should show only a single-line
	// summary instead of its full detail.
	Collapsed bool

	// ClickRegions are clickable areas within this entry's rendered output.
	// Line offsets are relative to the entry's own Rendered slice.
	ClickRegions []ClickRegion
}

// IsExpandable reports whether the entry can be expanded/collapsed
// (currently only tool calls).
func (e *TranscriptEntry) IsExpandable() bool {
	return e.Type == EntryToolCall
}

// IsCollapsed reports whether the entry is currently in its collapsed state.
func (e *TranscriptEntry) IsCollapsed() bool {
	return e.Collapsed
}

// Toggle flips the collapsed state. The caller must call
// Transcript.Rebuild afterwards to regenerate line offsets and viewport
// content.
func (e *TranscriptEntry) Toggle() {
	if e.IsExpandable() {
		e.Collapsed = !e.Collapsed
	}
}
