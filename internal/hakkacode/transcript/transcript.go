package transcript

import (
	"sort"
	"strings"
)

// Renderer is the callback that converts a TranscriptEntry into rendered
// lines given the current terminal width. The Transcript calls this during
// Rebuild() for every entry that doesn't yet have cached Rendered lines.
type Renderer func(entry *TranscriptEntry, width int) []string

// Transcript is the scrollback buffer: an ordered list of typed entries
// that can be hit-tested, expanded/collapsed, and rendered to a viewport
// string.
type Transcript struct {
	entries   []*TranscriptEntry
	lineCount int  // total rendered lines across all entries
	dirty     bool // true when entries changed and Rebuild needs calling
}

// New creates an empty Transcript.
func New() *Transcript {
	return &Transcript{}
}

// Len returns the number of entries.
func (t *Transcript) Len() int {
	return len(t.entries)
}

// EntryAt returns the entry at index i, or nil.
func (t *Transcript) EntryAt(i int) *TranscriptEntry {
	if i < 0 || i >= len(t.entries) {
		return nil
	}
	return t.entries[i]
}

// LastEntry returns the last entry, or nil if the transcript is empty.
func (t *Transcript) LastEntry() *TranscriptEntry {
	return t.EntryAt(t.Len() - 1)
}

// Pop removes the last entry and adjusts lineCount. Returns nil if empty.
func (t *Transcript) Pop() *TranscriptEntry {
	if len(t.entries) == 0 {
		return nil
	}
	e := t.entries[len(t.entries)-1]
	t.entries = t.entries[:len(t.entries)-1]
	n := len(e.Rendered)
	if n == 0 {
		n = 1
	}
	t.lineCount -= n
	if t.lineCount < 0 {
		t.lineCount = 0
	}
	return e
}

// Append adds an entry to the end of the transcript, assigns its LineOff
// incrementally, and updates lineCount — so hit-testing works immediately
// without requiring a Rebuild. Rebuild is still needed after a toggle or
// resize to re-render and re-anchor all entries.
func (t *Transcript) Append(entry *TranscriptEntry) {
	entry.LineOff = t.lineCount
	t.entries = append(t.entries, entry)
	n := len(entry.Rendered)
	if n == 0 {
		// An entry with no rendered lines still consumes one empty line
		// in the viewport (the trailing newline appended by Rebuild).
		n = 1
	}
	t.lineCount += n
}

// ToggleEntry toggles the collapsed state of the entry at the given index.
// Returns true if the entry was toggled and a rebuild is required.
func (t *Transcript) ToggleEntry(idx int) bool {
	if idx < 0 || idx >= len(t.entries) {
		return false
	}
	e := t.entries[idx]
	if !e.IsExpandable() {
		return false
	}
	e.Toggle()
	t.dirty = true
	return true
}

// EntryAtLine returns the entry containing the given absolute viewport
// line number, and the line offset within that entry's Rendered slice.
// Returns nil if line is out of bounds.
func (t *Transcript) EntryAtLine(line int) (*TranscriptEntry, int) {
	if line < 0 || len(t.entries) == 0 {
		return nil, -1
	}
	// If lineCount hasn't been computed yet (entries added before the
	// Append fix), recompute offsets lazily.
	if t.lineCount == 0 {
		t.recomputeOffsets()
	}
	if line >= t.lineCount {
		return nil, -1
	}
	idx := sort.Search(len(t.entries), func(i int) bool {
		return t.entries[i].LineOff > line
	})
	if idx == 0 {
		return nil, -1
	}
	e := t.entries[idx-1]
	rel := line - e.LineOff
	if rel < 0 || rel >= len(e.Rendered) {
		return nil, -1
	}
	return e, rel
}

// recomputeOffsets walks all entries and sets LineOff and lineCount from
// Rendered slices. Used lazily when LineOff was never set.
func (t *Transcript) recomputeOffsets() {
	off := 0
	for _, e := range t.entries {
		e.LineOff = off
		n := len(e.Rendered)
		if n == 0 {
			n = 1
		}
		off += n
	}
	t.lineCount = off
}

// ClickRegionAt returns the ClickRegion at the given absolute viewport
// (line, col), or nil if there's nothing clickable there.
func (t *Transcript) ClickRegionAt(line, col int) *ClickRegion {
	e, relLine := t.EntryAtLine(line)
	if e == nil || len(e.ClickRegions) == 0 {
		return nil
	}
	for i := range e.ClickRegions {
		r := &e.ClickRegions[i]
		if r.Line == relLine && col >= r.Col && col < r.Col+r.Width {
			return r
		}
	}
	return nil
}

// Rebuild re-renders all entries using the given renderer and width,
// recomputing LineOff and lineCount. Returns the full viewport content
// as a single string.
func (t *Transcript) Rebuild(renderer Renderer, width int) string {
	var sb strings.Builder
	lineOff := 0

	for _, e := range t.entries {
		// Re-render if dirty, resized, or never rendered.
		if t.dirty || e.Rendered == nil {
			e.Rendered = renderer(e, width)
		}
		e.LineOff = lineOff

		for _, l := range e.Rendered {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}
		lineOff += len(e.Rendered)
	}

	t.lineCount = lineOff
	t.dirty = false
	return sb.String()
}

// Content returns the full viewport string. Caller must have called
// Rebuild first if the transcript is dirty.
func (t *Transcript) Content() string {
	if t.dirty {
		return ""
	}
	return t.String()
}

// LineCount returns the total number of rendered lines.
func (t *Transcript) LineCount() int {
	return t.lineCount
}

// IsDirty reports whether Rebuild needs calling before Content or
// hit-testing.
func (t *Transcript) IsDirty() bool {
	return t.dirty
}

// ToggleEntryAt toggles the collapsed state of the entry containing the
// given viewport line. Returns true if toggled.
func (t *Transcript) ToggleEntryAt(line int) bool {
	e, _ := t.EntryAtLine(line)
	if e == nil || !e.IsExpandable() {
		return false
	}
	e.Toggle()
	t.dirty = true
	return true
}

// String returns the full viewport content string — identical to what
// Rebuild produces but using the current Rendered slices without
// re-rendering.
func (t *Transcript) String() string {
	var sb strings.Builder
	for _, e := range t.entries {
		for _, line := range e.Rendered {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
