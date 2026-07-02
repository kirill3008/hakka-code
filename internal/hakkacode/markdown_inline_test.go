package hakkacode

import (
	"strings"
	"testing"
)

func TestRenderInline(t *testing.T) {
	cases := []struct {
		in   string
		want string // substring that must appear
		bare string // literal markers that must NOT survive
	}{
		{"**bold**", "\033[1mbold\033[0m", "**bold**"},
		{"*italic*", "\033[3mitalic\033[0m", "*italic*"},
		{"`code`", "\033[36mcode\033[0m", "`code`"},
		{"__also bold__", "\033[1malso bold\033[0m", "__also bold__"},
	}
	for _, c := range cases {
		got := renderInline(c.in)
		if !strings.Contains(got, c.want) {
			t.Errorf("renderInline(%q) = %q, want substring %q", c.in, got, c.want)
		}
		if strings.Contains(got, c.bare) {
			t.Errorf("renderInline(%q) = %q, still contains literal markers %q", c.in, got, c.bare)
		}
	}
}

func TestRenderTableAppliesInlineMarkdown(t *testing.T) {
	raw := "| Name | Note |\n|---|---|\n| Alice | **important** |"
	out := renderTable(raw)
	if strings.Contains(out, "**important**") {
		t.Fatalf("expected bold markers to be rendered, not left literal: %q", out)
	}
	if !strings.Contains(out, "\033[1mimportant\033[0m") {
		t.Fatalf("expected bold ANSI codes around 'important', got: %q", out)
	}
}
