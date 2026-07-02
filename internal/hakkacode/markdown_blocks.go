package hakkacode

import "strings"

type blockKind int

const (
	blockText blockKind = iota
	blockCode
	blockTable
)

type mdBlock struct {
	kind    blockKind
	lang    string // for blockCode
	content string
}

// splitMarkdownBlocks scans markdown text line-by-line and separates it
// into fenced-code blocks, pipe-table blocks, and everything else
// ("prose"), preserving order. Code and table blocks get dedicated
// renderers instead of glamour's document-oriented ones.
func splitMarkdownBlocks(text string) []mdBlock {
	lines := strings.Split(text, "\n")
	var blocks []mdBlock
	var textBuf []string

	flush := func() {
		if len(textBuf) == 0 {
			return
		}
		blocks = append(blocks, mdBlock{kind: blockText, content: strings.Join(textBuf, "\n")})
		textBuf = nil
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if fence, ok := fenceMarker(trimmed); ok {
			lang := strings.TrimSpace(trimmed[len(fence):])
			flush()
			j := i + 1
			var code []string
			for j < len(lines) && strings.TrimSpace(lines[j]) != fence {
				code = append(code, lines[j])
				j++
			}
			blocks = append(blocks, mdBlock{kind: blockCode, lang: lang, content: strings.Join(code, "\n")})
			i = j + 1 // skip closing fence line (or EOF if unterminated)
			continue
		}

		if isTableRowLine(line) && i+1 < len(lines) && isTableSeparatorLine(lines[i+1]) {
			flush()
			rows := []string{line, lines[i+1]}
			j := i + 2
			for j < len(lines) && isTableRowLine(lines[j]) {
				rows = append(rows, lines[j])
				j++
			}
			blocks = append(blocks, mdBlock{kind: blockTable, content: strings.Join(rows, "\n")})
			i = j
			continue
		}

		textBuf = append(textBuf, line)
		i++
	}
	flush()
	return blocks
}

func fenceMarker(trimmedLine string) (string, bool) {
	for _, fence := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmedLine, fence) {
			return fence, true
		}
	}
	return "", false
}

// isTableSeparatorLine reports whether line looks like a GFM table's
// header separator, e.g. "|---|:--:|--:|".
func isTableSeparatorLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || !strings.Contains(t, "|") || !strings.Contains(t, "-") {
		return false
	}
	for _, r := range t {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

// isTableRowLine reports whether line looks like a pipe-delimited table
// row (header or data).
func isTableRowLine(line string) bool {
	t := strings.TrimSpace(line)
	return t != "" && strings.Contains(t, "|")
}
