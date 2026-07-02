package hakkacode

import (
	"strings"
	"testing"
)

func TestRenderUserPromptHasBackground(t *testing.T) {
	out := renderUserPrompt("❯ hello world")
	if !strings.Contains(out, userPromptBackground) {
		t.Fatalf("expected user prompt background escape, got: %q", out)
	}
}

func TestRenderUserPromptEmpty(t *testing.T) {
	if renderUserPrompt("") != "" {
		t.Fatal("expected empty passthrough")
	}
}

func TestRenderUserPromptWrapsLongSingleLine(t *testing.T) {
	long := "❯ " + strings.Repeat("word ", 60) // one raw line, no newlines
	out := renderUserPrompt(long)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a long single-line query to wrap into multiple lines, got %d line(s): %q", len(lines), out)
	}
	for i, l := range lines {
		if w := visibleLen(l); w > terminalWidth() {
			t.Fatalf("line %d is wider than the terminal (%d > %d): %q", i, w, terminalWidth(), l)
		}
	}
}

func TestRenderMarkdownProseHasNoBackground(t *testing.T) {
	// The assistant's reply itself is no longer highlighted — only the
	// user's echoed prompt is (see renderUserPrompt) — since replies are
	// typically much longer than the query and a full background box
	// around them was more clutter than signal.
	out := renderMarkdown("some text\n\n```go\nx := 1\n```\n")
	if strings.Contains(out, userPromptBackground) {
		t.Fatalf("assistant reply should not carry the user-prompt background, got: %q", out)
	}
	if !strings.Contains(out, codeBlockBackground) {
		t.Fatalf("expected code block to keep its own background, got: %q", out)
	}
}
