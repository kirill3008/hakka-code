package hakkacode

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// codeBlockBackground is a dark-gray ANSI-256 background, subtly distinct
// from a typical terminal's own background — enough to make a code block
// scannable while scrolling without clashing with either light or dark
// terminal themes.
const codeBlockBackground = "\x1b[48;5;236m"

const ansiReset = "\x1b[0m"

var ansiEscapeRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// renderCodeBlock syntax-highlights a fenced code block via chroma and
// paints a uniform background behind every line, so the block reads as a
// distinct box rather than blending into the terminal's own background.
func renderCodeBlock(lang, code string) string {
	code = strings.TrimRight(code, "\n")
	if code == "" {
		return ""
	}

	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return renderPlainCodeBlock(code)
	}

	var buf bytes.Buffer
	if err := formatters.TTY256.Format(&buf, chromastyles.Get("monokai"), iterator); err != nil {
		return renderPlainCodeBlock(code)
	}

	width := codeBlockWidth(code)
	var out strings.Builder
	for _, line := range strings.Split(buf.String(), "\n") {
		out.WriteString(padWithBackground(line, width))
		out.WriteByte('\n')
	}
	return out.String()
}

// renderPlainCodeBlock is the no-syntax-highlighting fallback — still
// gets the background treatment so a lexer/formatter failure doesn't
// silently drop the visual distinction.
func renderPlainCodeBlock(code string) string {
	width := codeBlockWidth(code)
	var out strings.Builder
	for _, line := range strings.Split(code, "\n") {
		out.WriteString(padWithBackground(line, width))
		out.WriteByte('\n')
	}
	return out.String()
}

// codeBlockMargin is the leading+trailing padding padWithBackground adds
// around each line's content (2 leading spaces, 1 trailing).
const codeBlockMargin = 3

// codeBlockWidth returns the total visible width the background box
// should span. It spans the full terminal width (minus a small margin,
// matching glamour's own prose indent) rather than just the longest code
// line, so the box reads as a continuous strip while scrolling. A code
// line longer than the terminal still gets to keep its own width —
// nothing is clipped.
func codeBlockWidth(code string) int {
	width := terminalWidth() - 4
	if longest := visibleWidth(code) + codeBlockMargin; longest > width {
		width = longest
	}
	if width < codeBlockMargin+1 {
		width = visibleWidth(code) + codeBlockMargin
	}
	return width
}

// padWithBackground wraps a single (possibly already ANSI-colored) line
// with a background color, re-asserting it after every reset the line
// contains (chroma resets attributes per-token, which would otherwise
// clear our background partway through the line), and pads with spaces
// so the background fills to totalWidth (which includes codeBlockMargin).
func padWithBackground(line string, totalWidth int) string {
	painted := strings.ReplaceAll(line, ansiReset, ansiReset+codeBlockBackground)
	pad := totalWidth - codeBlockMargin - visibleLen(line)
	if pad < 0 {
		pad = 0
	}
	return codeBlockBackground + "  " + painted + strings.Repeat(" ", pad+1) + ansiReset
}

func visibleWidth(text string) int {
	max := 0
	for _, line := range strings.Split(text, "\n") {
		if l := visibleLen(line); l > max {
			max = l
		}
	}
	return max
}

func visibleLen(s string) int {
	return len([]rune(ansiEscapeRE.ReplaceAllString(s, "")))
}
