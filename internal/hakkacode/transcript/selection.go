package transcript

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripANSI removes ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return StripANSI(s)
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
// contain ANSI escapes and multi-byte UTF-8). Returns clean text
// suitable for clipboard.
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
		return sliceByColumns(cleanLines[sl], sc, ec)
	}

	var parts []string
	first := sliceByColumns(cleanLines[sl], sc, -1)
	parts = append(parts, first)
	for i := sl + 1; i < el; i++ {
		parts = append(parts, cleanLines[i])
	}
	last := sliceByColumns(cleanLines[el], 0, ec)
	parts = append(parts, last)

	return strings.Join(parts, "\n")
}

// sliceByColumns returns a substring of s from display-column startCol
// to endCol. If endCol is negative, returns from startCol to end of s.
// Columns count runes, not bytes.
func sliceByColumns(s string, startCol, endCol int) string {
	bi := colToByteOffset(s, startCol)
	if endCol < 0 {
		return s[bi:]
	}
	ei := colToByteOffset(s, endCol)
	if ei > len(s) {
		ei = len(s)
	}
	if bi >= ei {
		return ""
	}
	return s[bi:ei]
}

// colToByteOffset converts a display-column (rune) index into the byte
// offset within s. Returns len(s) when col is beyond the string.
func colToByteOffset(s string, col int) int {
	ri := 0 // current rune index
	for bi := 0; bi < len(s); {
		if ri == col {
			return bi
		}
		_, size := utf8.DecodeRuneInString(s[bi:])
		bi += size
		ri++
	}
	return len(s)
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
			result = append(result, highlightRegion(raw, cleanLines[i], sc, utf8.RuneCountInString(cleanLines[i])))
		} else if i == el {
			end := ec
			cleanRuneCount := utf8.RuneCountInString(cleanLines[i])
			if end > cleanRuneCount {
				end = cleanRuneCount
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
// in reverse-video. cleanStart and cleanEnd are rune (display-column)
// indices in the ANSI-stripped version of raw. Any ANSI reset sequences
// inside the highlighted region are followed by a re-application of
// reverse video.
func highlightRegion(raw, clean string, start, end int) string {
	cleanRunes := utf8.RuneCountInString(clean)
	if start >= cleanRunes || start >= end {
		return raw
	}
	if end > cleanRunes {
		end = cleanRunes
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

// CleanColToRaw maps a column index in the ANSI-stripped string to a
// byte offset in the raw string (which contains ANSI escapes and
// multi-byte UTF-8 characters). Each rune — regardless of its byte
// length — occupies exactly one display column.
func CleanColToRaw(clean, raw string, col int) int {
	return cleanColToRaw(clean, raw, col)
}

// cleanColToRaw maps a column index in the ANSI-stripped string to a
// byte offset in the raw string (which contains ANSI escapes and
// multi-byte UTF-8 characters). Each rune — regardless of its byte
// length — occupies exactly one display column.
func cleanColToRaw(clean, raw string, col int) int {
	if col <= 0 {
		return 0
	}
	ci := 0 // current clean-column position
	for ri := 0; ri < len(raw); ri++ {
		b := raw[ri]
		if b == '\x1b' {
			// Skip entire ANSI escape sequence.
			for ri+1 < len(raw) && raw[ri] != 'm' {
				ri++
			}
			continue
		}
		// Multi-byte UTF-8 continuation bytes (0x80–0xBF) don't start a
		// new rune — skip them without incrementing the column counter.
		if b&0xC0 == 0x80 {
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
