package hakkacode

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// renderMarkdown renders text as terminal-formatted markdown.
//
// Glamour's default document-oriented layout doesn't fit a chat turn well:
// it stretches tables to the full terminal width and (by design, per its
// style JSON) prints literal "##"/"###" markers for h2-h6 headings. So we
// hand-split the text into code/table/prose blocks and render each with
// the tool best suited to it — prose still goes through glamour (with a
// heading style fix), but code blocks and tables get their own renderers.
func renderMarkdown(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	var b strings.Builder
	for _, blk := range splitMarkdownBlocks(text) {
		switch blk.kind {
		case blockCode:
			b.WriteString(renderCodeBlock(blk.lang, blk.content))
		case blockTable:
			b.WriteString(renderTable(blk.content))
		default:
			b.WriteString(renderProse(blk.content))
		}
	}
	return b.String()
}

// renderProse renders non-code, non-table markdown text via glamour, using
// a heading style patched to drop the literal "#" prefixes that glamour's
// built-in styles print for h2-h6 (h1 already renders as a colored block
// with no "#").
func renderProse(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(headingFixedStyle()),
		glamour.WithWordWrap(terminalWidth()),
	)
	if err != nil {
		return text
	}
	out, err := r.Render(text)
	if err != nil {
		return text
	}
	return out
}

func headingFixedStyle() ansi.StyleConfig {
	var cfg ansi.StyleConfig
	switch {
	case !term.IsTerminal(int(os.Stdout.Fd())):
		cfg = styles.NoTTYStyleConfig
	case termenv.HasDarkBackground():
		cfg = styles.DarkStyleConfig
	default:
		cfg = styles.LightStyleConfig
	}
	cfg.H2.Prefix = ""
	cfg.H3.Prefix = ""
	cfg.H4.Prefix = ""
	cfg.H5.Prefix = ""
	cfg.H6.Prefix = ""

	// glamour's built-in hr style is a fixed 8-dash literal regardless of
	// terminal width, which barely reads as a horizontal rule in a wide
	// terminal. Stretch it to the render width instead.
	ruleWidth := terminalWidth() - 4
	if ruleWidth < 8 {
		ruleWidth = 8
	}
	cfg.HorizontalRule.Format = "\n" + strings.Repeat("─", ruleWidth) + "\n"

	return cfg
}

func terminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 100
}
