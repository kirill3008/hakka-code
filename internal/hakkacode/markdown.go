package hakkacode

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// isDarkBackground is memoized: termenv queries it via a raw stdin read,
// which races Bubble Tea's own input reader if run after startup (leaks
// garbage like "]11;rgb:..." into whatever's focused).
var isDarkBackground = sync.OnceValue(termenv.HasDarkBackground)

// detectTerminalTheme must run before tea.Program starts — see
// isDarkBackground.
func detectTerminalTheme() {
	isDarkBackground()
}

// renderMarkdown hand-splits code/table/prose blocks and renders each
// separately — glamour's own table/heading defaults don't fit a chat
// turn well (full-width tables, literal "#" markers for h2-h6).
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
	case isDarkBackground():
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
