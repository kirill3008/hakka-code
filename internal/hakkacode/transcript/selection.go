package transcript

import "strings"

// SelectionState is the phase of a text selection drag.
type SelectionState int

const (
	// SelNone means no selection is active.
	SelNone SelectionState = iota

	// SelDragging means the mouse is held and dragging.
	SelDragging

	// SelDone means the user released the mouse; the selection is final
	// and should be copied. The highlight stays until next interaction.
	SelDone
)

// Selection tracks a mouse-driven text selection across the viewport.
// All coordinates are absolute viewport positions (not entry-relative).
type Selection struct {
	State     SelectionState
	StartLine int // viewport-absolute
	StartCol  int
	EndLine   int
	EndCol    int
}

// NewSelection creates an inactive selection.
func NewSelection() *Selection {
	return &Selection{State: SelNone}
}

// Start begins a new selection at (line, col).
func (s *Selection) Start(line, col int) {
	s.State = SelDragging
	s.StartLine = line
	s.StartCol = col
	s.EndLine = line
	s.EndCol = col
}

// Extend updates the selection endpoint during a drag.
func (s *Selection) Extend(line, col int) {
	if s.State != SelDragging {
		return
	}
	s.EndLine = line
	s.EndCol = col
}

// Finish completes the selection.
func (s *Selection) Finish() {
	if s.State == SelDragging {
		s.State = SelDone
	}
}

// Clear resets the selection.
func (s *Selection) Clear() {
	s.State = SelNone
	s.StartLine = 0
	s.StartCol = 0
	s.EndLine = 0
	s.EndCol = 0
}

// IsActive reports whether a selection is being dragged or was just completed.
func (s *Selection) IsActive() bool {
	return s.State == SelDragging || s.State == SelDone
}

// Normalized returns the selection's start and end points ordered such
// that (startLine, startCol) ≤ (endLine, endCol).
func (s *Selection) Normalized() (sLine, sCol, eLine, eCol int) {
	if s.StartLine < s.EndLine || (s.StartLine == s.EndLine && s.StartCol <= s.EndCol) {
		return s.StartLine, s.StartCol, s.EndLine, s.EndCol
	}
	return s.EndLine, s.EndCol, s.StartLine, s.StartCol
}

// Contains reports whether (line, col) falls within the selection.
func (s *Selection) Contains(line, col int) bool {
	if !s.IsActive() {
		return false
	}
	sl, sc, el, ec := s.Normalized()
	if line < sl || line > el {
		return false
	}
	if line == sl && col < sc {
		return false
	}
	if line == el && col >= ec {
		return false
	}
	return true
}

// Text extracts the selected text from viewport content. The content
// must be the same string that was passed to the viewport (i.e. with
// newline-separated lines matching the line numbering used by Selection).
func (s *Selection) Text(content string) string {
	if !s.IsActive() || content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	sl, sc, el, ec := s.Normalized()

	if sl >= len(lines) || el >= len(lines) {
		return ""
	}

	if sl == el {
		// Single-line selection.
		line := lines[sl]
		if sc >= len(line) {
			return ""
		}
		end := ec
		if end > len(line) {
			end = len(line)
		}
		return line[sc:end]
	}

	// Multi-line selection.
	var parts []string
	// First line: from sc to end.
	firstLine := lines[sl]
	if sc < len(firstLine) {
		parts = append(parts, firstLine[sc:])
	} else {
		parts = append(parts, "")
	}
	// Middle lines: whole lines.
	for i := sl + 1; i < el; i++ {
		parts = append(parts, lines[i])
	}
	// Last line: from start to ec.
	lastLine := lines[el]
	if ec > len(lastLine) {
		ec = len(lastLine)
	}
	parts = append(parts, lastLine[:ec])

	return strings.Join(parts, "\n")
}

// ApplyHighlight overlays the selection highlight onto content.
// Returns the content with reverse-video escapes around selected regions.
func (s *Selection) ApplyHighlight(content string) string {
	if !s.IsActive() || content == "" {
		return content
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	sl, sc, el, ec := s.Normalized()

	var result []string
	for i, line := range lines {
		if i < sl || i > el {
			result = append(result, line)
			continue
		}

		if sl == el {
			// Single-line: wrap from sc to ec.
			result = append(result, highlightRegion(line, sc, ec))
			continue
		}

		if i == sl {
			// First line: from sc to end.
			result = append(result, highlightRegion(line, sc, len(line)))
		} else if i == el {
			// Last line: from start to ec.
			if ec > len(line) {
				ec = len(line)
			}
			result = append(result, highlightRegion(line, 0, ec))
		} else {
			// Middle line: full line.
			result = append(result, "\x1b[7m"+line+"\x1b[27m")
		}
	}

	return strings.Join(result, "\n")
}

const (
	ansiReverse = "\x1b[7m"
	ansiReset   = "\x1b[27m"
)

func highlightRegion(line string, start, end int) string {
	if start >= len(line) || start >= end {
		return line
	}
	if end > len(line) {
		end = len(line)
	}
	return line[:start] + ansiReverse + line[start:end] + ansiReset + line[end:]
}
