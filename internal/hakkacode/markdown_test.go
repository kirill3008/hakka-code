package hakkacode

import (
	"strings"
	"testing"
)

func TestSplitMarkdownBlocksBasic(t *testing.T) {
	text := "Hello\n\n```go\nfunc main() {}\n```\n\nSome text\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\nBye"
	blocks := splitMarkdownBlocks(text)

	var kinds []blockKind
	for _, b := range blocks {
		kinds = append(kinds, b.kind)
	}
	want := []blockKind{blockText, blockCode, blockText, blockTable, blockText}
	if len(kinds) != len(want) {
		t.Fatalf("got %d blocks %v, want %d blocks %v", len(kinds), kinds, len(want), want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("block %d kind = %v, want %v", i, kinds[i], want[i])
		}
	}
	if blocks[1].lang != "go" {
		t.Fatalf("code block lang = %q, want go", blocks[1].lang)
	}
	if !strings.Contains(blocks[1].content, "func main") {
		t.Fatalf("code block content missing source: %q", blocks[1].content)
	}
}

func TestRenderProseHeadingsHaveNoHashPrefix(t *testing.T) {
	for _, level := range []string{"##", "###", "####", "#####", "######"} {
		out := renderProse(level + " Section Title")
		if strings.Contains(out, level+" ") {
			t.Fatalf("rendered heading for %q retained literal hash prefix: %q", level, out)
		}
		if !strings.Contains(out, "Section Title") {
			t.Fatalf("rendered heading missing title text: %q", out)
		}
	}
}

func TestRenderTableFitsContentWidth(t *testing.T) {
	raw := "| a | bb |\n|---|---|\n| 1 | 2 |"
	out := renderTable(raw)

	if !strings.Contains(out, "┌") || !strings.Contains(out, "┘") {
		t.Fatalf("expected a full box border, got: %q", out)
	}

	maxLineLen := 0
	for _, line := range strings.Split(out, "\n") {
		if l := visibleLen(line); l > maxLineLen {
			maxLineLen = l
		}
	}
	// A tiny 2-column table should never approach the 100-col terminal
	// fallback width — this is the "don't stretch to full width" check.
	if maxLineLen > 20 {
		t.Fatalf("table rendered too wide (%d cols) for its content: %q", maxLineLen, out)
	}
}

func TestRenderCodeBlockHasUniformBackground(t *testing.T) {
	out := renderCodeBlock("go", "func main() {\n\tfmt.Println(\"hi\")\n}")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty output")
	}
	width := visibleLen(lines[0])
	for i, l := range lines {
		if !strings.Contains(l, codeBlockBackground) {
			t.Fatalf("line %d missing background escape: %q", i, l)
		}
		if got := visibleLen(l); got != width {
			t.Fatalf("line %d width = %d, want %d (all lines should be padded uniformly): %q", i, got, width, l)
		}
	}
}

func TestRenderCodeBlockBackgroundSpansTerminalWidth(t *testing.T) {
	// A short one-line snippet should still get a background as wide as
	// the terminal (minus margin), not just as wide as the code itself —
	// otherwise the box looks like it's clipped mid-line while scrolling.
	out := renderCodeBlock("go", "x := 1")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one line, got %d: %q", len(lines), out)
	}
	got := visibleLen(lines[0])
	want := terminalWidth() - 4
	if got != want {
		t.Fatalf("background width = %d, want %d (terminal width based, not content based)", got, want)
	}
}

func TestRenderCodeBlockDoesNotClipLongLines(t *testing.T) {
	long := strings.Repeat("a", terminalWidth()+50)
	out := renderCodeBlock("text", long)
	if !strings.Contains(out, long) {
		t.Fatal("expected long code line to be preserved in full, not clipped to terminal width")
	}
}

func TestRenderProseHorizontalRuleSpansWidth(t *testing.T) {
	out := renderProse("above\n\n---\n\nbelow")
	if strings.Contains(out, "--------\n") {
		t.Fatalf("expected hr to be stretched past glamour's fixed 8-dash default, got: %q", out)
	}
	longestRun := 0
	for _, line := range strings.Split(out, "\n") {
		run := strings.Count(line, "─")
		if run > longestRun {
			longestRun = run
		}
	}
	if longestRun < 20 {
		t.Fatalf("expected a wide horizontal rule, longest run of ─ was %d: %q", longestRun, out)
	}
}
