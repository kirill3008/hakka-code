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

	rawLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	cleanLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		cleanLines[i] = StripANSI(l)
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
	ri := 0
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

// ApplyHighlight overlays the selection highlight onto content, using
// the selection's normalized line/column ranges.
func (s *Selection) ApplyHighlight(content string) string {
	if !s.IsActive() || content == "" {
		return content
	}
	rawLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	cleanLines := make([]string, len(rawLines))
	for i, l := range rawLines {
		cleanLines[i] = StripANSI(l)
	}
	sl, sc, el, ec := s.Normalized()

	var result []string
	for i, raw := range rawLines {
		if i < sl || i > el {
			result = append(result, raw)
			continue
		}

		if sl == el {
			result = append(result, HighlightRegion(raw, sc, ec))
			continue
		}

		if i == sl {
			result = append(result, HighlightRegion(raw, sc, utf8.RuneCountInString(cleanLines[i])))
		} else if i == el {
			end := ec
			cleanRuneCount := utf8.RuneCountInString(cleanLines[i])
			if end > cleanRuneCount {
				end = cleanRuneCount
			}
			result = append(result, HighlightRegion(raw, 0, end))
		} else {
			result = append(result, WrapFullLine(raw))
		}
	}

	return strings.Join(result, "\n")
}

// ---------------------------------------------------------------------------
// Public single-line highlight helpers — used directly from the TUI (avoiding
// duplicated logic in tui.go). All coordinates are rune (display-column)
// indices. end == -1 means "to end of line".
// ---------------------------------------------------------------------------

// HighlightRegion wraps a portion of a single line in reverse-video.
// start and end are display-column (rune) indices in the ANSI-stripped
// content. Any ANSI reset sequences inside the highlighted region are
// followed by a re-application of reverse video.
func HighlightRegion(raw string, start, end int) string {
	clean := StripANSI(raw)
	cleanRunes := utf8.RuneCountInString(clean)

	if start < 0 {
		start = 0
	}
	if end < 0 || end > cleanRunes {
		end = cleanRunes
	}
	if start >= cleanRunes || start >= end {
		return raw
	}

	rawStart := cleanColToRaw(clean, raw, start)
	rawEnd := cleanColToRaw(clean, raw, end)

	var sb strings.Builder
	sb.WriteString(raw[:rawStart])
	sb.WriteString(ansiReverse)

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

// WrapFullLine wraps an entire line in reverse video, re-applying
// reverse after any embedded ANSI reset sequences.
func WrapFullLine(raw string) string {
	if raw == "" {
		return ansiReverse + ansiReset
	}
	var sb strings.Builder
	sb.WriteString(ansiReverse)

	for i := 0; i < len(raw); {
		if raw[i] == '\x1b' {
			start := i
			for i < len(raw) && raw[i] != 'm' {
				i++
			}
			if i < len(raw) {
				i++ // include the 'm'
			}
			seq := raw[start:i]
			sb.WriteString(seq)
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

// CleanColToRaw maps a column index in the ANSI-stripped string to a
// byte offset in the raw string (which contains ANSI escapes and
// multi-byte UTF-8 characters). Each rune — regardless of its byte
// length — occupies exactly one display column.
func CleanColToRaw(clean, raw string, col int) int {
	return cleanColToRaw(clean, raw, col)
}

// cleanColToRaw is the private implementation of CleanColToRaw.
func cleanColToRaw(clean, raw string, col int) int {
	if col <= 0 {
		return 0
	}
	ci := 0
	for ri := 0; ri < len(raw); ri++ {
		b := raw[ri]
		if b == '\x1b' {
			for ri+1 < len(raw) && raw[ri] != 'm' {
				ri++
			}
			continue
		}
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
