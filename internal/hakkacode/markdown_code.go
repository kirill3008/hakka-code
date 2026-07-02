package hakkacode

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
)

// codeBlockBackground is a dark-gray ANSI-256 background, subtly distinct
// from a typical terminal's own background — enough to make a code block
// scannable while scrolling without clashing with either light or dark
// terminal themes.
const codeBlockBackground = "\x1b[48;5;236m"

// userPromptBackground highlights the user's echoed prompt in the
// transcript. The query is highlighted rather than the reply because
// it's typically much shorter — a small, easy-to-spot marker instead of
// a large colored block around a potentially long response.
const userPromptBackground = "\x1b[48;5;24m"

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

	width := boxWidth(code)
	var out strings.Builder
	for _, line := range strings.Split(buf.String(), "\n") {
		out.WriteString(paintLine(line, width, codeBlockBackground))
		out.WriteByte('\n')
	}
	return out.String()
}

// renderPlainCodeBlock is the no-syntax-highlighting fallback — still
// gets the background treatment so a lexer/formatter failure doesn't
// silently drop the visual distinction.
func renderPlainCodeBlock(code string) string {
	width := boxWidth(code)
	var out strings.Builder
	for _, line := range strings.Split(code, "\n") {
		out.WriteString(paintLine(line, width, codeBlockBackground))
		out.WriteByte('\n')
	}
	return out.String()
}

// boxMargin is the leading+trailing padding paintLine adds around each
// line's content (2 leading spaces, 1 trailing).
const boxMargin = 3

// boxWidth spans the full terminal width so the box reads as a
// continuous strip while scrolling, growing past that for lines already
// wider than the terminal.
func boxWidth(text string) int {
	width := terminalWidth() - 4
	if longest := visibleWidth(text) + boxMargin; longest > width {
		width = longest
	}
	if width < boxMargin+1 {
		width = visibleWidth(text) + boxMargin
	}
	return width
}

// paintLine backgrounds a line, re-asserting the color after every reset
// (chroma/glamour reset per token) and padding to totalWidth.
func paintLine(line string, totalWidth int, bg string) string {
	painted := strings.ReplaceAll(line, ansiReset, ansiReset+bg)
	pad := totalWidth - boxMargin - visibleLen(line)
	if pad < 0 {
		pad = 0
	}
	return bg + "  " + painted + strings.Repeat(" ", pad+1) + ansiReset
}

// renderUserPrompt wraps the user's echoed input line(s) in a background
// box distinct from the terminal's own background, so it stands out as
// a clear marker of "here's what was asked" in the scrollback.
func renderUserPrompt(text string) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return ""
	}
	// A single long query is one raw (potentially very wide) line — word
	// wrap it first so it renders as a proper multi-line block instead
	// of overflowing/getting clipped at the terminal edge.
	wrapWidth := terminalWidth() - boxMargin - 1
	if wrapWidth < 10 {
		wrapWidth = 10
	}
	wrapped := ansi.Wordwrap(text, wrapWidth, "")
	wrapped = ansi.Hardwrap(wrapped, wrapWidth, true)

	width := boxWidth(wrapped)
	var out strings.Builder
	for _, line := range strings.Split(wrapped, "\n") {
		out.WriteString(paintLine(line, width, userPromptBackground))
		out.WriteByte('\n')
	}
	return out.String()
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
