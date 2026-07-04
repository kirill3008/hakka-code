package hakkacode

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// tableBorderColor is a bright, slightly desaturated green — visible
// against both light and dark terminal backgrounds without being as
// harsh as a pure ANSI green.
const tableBorderColor = "78"

// renderTable renders a raw GFM pipe-table block (header + separator +
// rows) with a full border, sized to the content's own longest cell up to
// the terminal width — past that, cells wrap onto multiple lines instead
// of overflowing off the right edge.
func renderTable(raw string) string {
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) < 2 {
		return raw
	}
	header := splitTableRow(lines[0])
	aligns := parseTableAlignments(lines[1], len(header))
	var rows [][]string
	for _, l := range lines[2:] {
		rows = append(rows, splitTableRow(l))
	}

	build := func() *table.Table {
		return table.New().
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(tableBorderColor))).
			BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true).
			BorderHeader(true).BorderColumn(true).BorderRow(false).
			Headers(header...).
			Rows(rows...).
			StyleFunc(func(_, col int) lipgloss.Style {
				st := lipgloss.NewStyle().Padding(0, 1)
				if col >= 0 && col < len(aligns) {
					st = st.Align(aligns[col])
				}
				return st
			})
	}

	// Render at natural content width first. Only clamp to the terminal
	// width (wrapping cells onto multiple lines) if that natural width
	// would overflow — a small table shouldn't get stretched.
	out := build().String()
	if widestLine(out) > terminalWidth() {
		out = build().Width(terminalWidth()).Wrap(true).String()
	}

	return out + "\n"
}

// widestLine returns the visible-column width of the widest line in s,
// ignoring ANSI escape sequences.
func widestLine(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > max {
			max = w
		}
	}
	return max
}

func splitTableRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = renderInline(strings.TrimSpace(p))
	}
	return cells
}

func parseTableAlignments(sepLine string, n int) []lipgloss.Position {
	cells := splitTableRow(sepLine)
	aligns := make([]lipgloss.Position, n)
	for i := range aligns {
		aligns[i] = lipgloss.Left
	}
	for i, c := range cells {
		if i >= n {
			break
		}
		left := strings.HasPrefix(c, ":")
		right := strings.HasSuffix(c, ":")
		switch {
		case left && right:
			aligns[i] = lipgloss.Center
		case right:
			aligns[i] = lipgloss.Right
		default:
			aligns[i] = lipgloss.Left
		}
	}
	return aligns
}
