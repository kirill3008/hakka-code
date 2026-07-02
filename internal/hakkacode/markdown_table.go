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
// rows) with a full border, sized to the content's own longest cell —
// not stretched to the terminal width like glamour's default table.
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

	t := table.New().
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

	return t.String() + "\n"
}

func splitTableRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
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
