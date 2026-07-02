package transcript

import (
	"regexp"
	"strings"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

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

// Text extracts the selected text from viewport content (which may
// contain ANSI escapes). Returns clean text suitable for clipboard.
func (s *Selection) Text(content string) string {
	if !s.IsActive() || content == "" {
		return ""
	}

	// Build a parallel structure: cleanLines (ANSI-stripped) for
	// indexing by mouse-column, rawLines for the output text.
	rawLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	cleanLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		cleanLines[i] = stripANSI(l)
	}

	sl, sc, el, ec := s.Normalized()

	if sl >= len(cleanLines) || el >= len(cleanLines) {
		return ""
	}

	if sl == el {
		line := cleanLines[sl]
		if sc >= len(line) {
			return ""
		}
		end := ec
		if end > len(line) {
			end = len(line)
		}
		return line[sc:end]
	}

	var parts []string
	firstLine := cleanLines[sl]
	if sc < len(firstLine) {
		parts = append(parts, firstLine[sc:])
	} else {
		parts = append(parts, "")
	}
	for i := sl + 1; i < el; i++ {
		parts = append(parts, cleanLines[i])
	}
	lastLine := cleanLines[el]
	if ec > len(lastLine) {
		ec = len(lastLine)
	}
	parts = append(parts, lastLine[:ec])

	return strings.Join(parts, "\n")
}

// ApplyHighlight overlays the selection highlight onto content.
// Returns the content with reverse-video escapes around selected regions.
// Handles embedded ANSI reset sequences (\033[0m, \033[27m) by
// re-applying reverse video after each reset within a highlighted region.
func (s *Selection) ApplyHighlight(content string) string {
	if !s.IsActive() || content == "" {
		return content
	}
	rawLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	cleanLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		cleanLines[i] = stripANSI(l)
	}
	sl, sc, el, ec := s.Normalized()

	var result []string
	for i, raw := range rawLines {
		if i < sl || i > el {
			result = append(result, raw)
			continue
		}

		if sl == el {
			result = append(result, highlightRegion(raw, cleanLines[i], sc, ec))
			continue
		}

		if i == sl {
			result = append(result, highlightRegion(raw, cleanLines[i], sc, len(cleanLines[i])))
		} else if i == el {
			end := ec
			if end > len(cleanLines[i]) {
				end = len(cleanLines[i])
			}
			result = append(result, highlightRegion(raw, cleanLines[i], 0, end))
		} else {
			// Middle line: wrap whole line in reverse video, but
			// re-apply reverse after any embedded reset sequences
			// (\033[0m or \033[27m) within the content.
			result = append(result, wrapFullLine(raw))
		}
	}

	return strings.Join(result, "\n")
}

// wrapFullLine wraps an entire line in reverse video, handling any
// embedded ANSI reset sequences that would cancel the highlight.
func wrapFullLine(raw string) string {
	if raw == "" {
		return ansiReverse + ansiReset
	}
	var sb strings.Builder
	sb.WriteString(ansiReverse)

	for i := 0; i < len(raw); {
		if raw[i] == '\x1b' {
			// Find the end of the escape sequence.
			start := i
			for i < len(raw) && raw[i] != 'm' {
				i++
			}
			if i < len(raw) {
				i++ // include the 'm'
			}
			seq := raw[start:i]
			sb.WriteString(seq)
			// After \033[0m or \033[27m, re-apply reverse video.
			if seq == "\x1b[0m" || seq == "\x1b[27m" {
				sb.WriteString(ansiReverse)
			}
		} else {
			sb.WriteByte(raw[i])
			i++
		}
	}

	sb.WriteString(ansiReset)
	return sb.String()
}

// highlightRegion wraps a portion of raw (which may contain ANSI codes)
// in reverse-video. cleanStart and cleanEnd are column indices in the
// ANSI-stripped version of raw. Any ANSI reset sequences inside the
// highlighted region are followed by a re-application of reverse video.
func highlightRegion(raw, clean string, start, end int) string {
	if start >= len(clean) || start >= end {
		return raw
	}
	if end > len(clean) {
		end = len(clean)
	}
	// Map clean-column indices to raw byte offsets.
	rawStart := cleanColToRaw(clean, raw, start)
	rawEnd := cleanColToRaw(clean, raw, end)

	// Build: prefix + reverse + middle(re-applied) + reset + suffix.
	var sb strings.Builder
	sb.WriteString(raw[:rawStart])
	sb.WriteString(ansiReverse)

	// Write the middle region, re-applying reverse after each reset.
	middle := raw[rawStart:rawEnd]
	for i := 0; i < len(middle); {
		if middle[i] == '\x1b' {
			seqStart := i
			for i < len(middle) && middle[i] != 'm' {
				i++
			}
			if i < len(middle) {
				i++ // include 'm'
			}
			seq := middle[seqStart:i]
			sb.WriteString(seq)
			if seq == "\x1b[0m" || seq == "\x1b[27m" {
				sb.WriteString(ansiReverse)
			}
		} else {
			sb.WriteByte(middle[i])
			i++
		}
	}

	sb.WriteString(ansiReset)
	sb.WriteString(raw[rawEnd:])
	return sb.String()
}

// cleanColToRaw maps a column index in the ANSI-stripped string to a
// byte offset in the raw string (which contains ANSI escapes).
func cleanColToRaw(clean, raw string, col int) int {
	if col <= 0 {
		return 0
	}
	ci := 0 // current clean-column position
	for ri := 0; ri < len(raw); ri++ {
		if raw[ri] == '\x1b' {
			// Skip entire ANSI escape sequence.
			for ri+1 < len(raw) && raw[ri] != 'm' {
				ri++
			}
			continue
		}
		if ci == col {
			return ri
		}
		ci++
	}
	return len(raw)
}

const (
	ansiReverse = "\x1b[7m"
	ansiReset   = "\x1b[27m"
)
