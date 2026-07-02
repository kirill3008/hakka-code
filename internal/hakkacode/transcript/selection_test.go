package transcript

import (
	"strings"
	"testing"
)

func TestSelectionSingleLine(t *testing.T) {
	sel := NewSelection()
	sel.Start(0, 5)
	sel.Extend(0, 10)
	sel.Finish()

	content := "hello world here"
	text := sel.Text(content)
	if text != " worl" {
		t.Fatalf("expected ' worl', got %q", text)
	}
}

func TestSelectionMultiLine(t *testing.T) {
	sel := NewSelection()
	sel.Start(0, 2)
	sel.Extend(2, 4)
	sel.Finish()

	content := "0123456789\nabcdefghij\nABCDEFGHIJ"
	text := sel.Text(content)
	expected := "23456789\nabcdefghij\nABCD"
	if text != expected {
		t.Fatalf("expected %q, got %q", expected, text)
	}
}

func TestSelectionNormalizeReverseDrag(t *testing.T) {
	sel := NewSelection()
	sel.Start(2, 8)
	sel.Extend(0, 2)
	sel.Finish()

	sl, sc, el, ec := sel.Normalized()
	if sl != 0 || sc != 2 || el != 2 || ec != 8 {
		t.Fatalf("normalized: (%d,%d)-(%d,%d), want (0,2)-(2,8)", sl, sc, el, ec)
	}
}

func TestSelectionHighlight(t *testing.T) {
	sel := NewSelection()
	sel.Start(0, 2)
	sel.Extend(0, 6)
	sel.Finish()

	content := "0123456789"
	out := sel.ApplyHighlight(content)
	if !strings.Contains(out, "\x1b[7m2345\x1b[27m") {
		t.Fatalf("expected reverse-video highlight, got: %q", out)
	}
	// Parts before and after should be preserved.
	if !strings.Contains(out, "01"+ansiReverse) {
		t.Fatalf("expected '01' before highlight, got: %q", out)
	}
	if !strings.Contains(out, ansiReset+"6789") {
		t.Fatalf("expected '6789' after highlight, got: %q", out)
	}
}

func TestSelectionClear(t *testing.T) {
	sel := NewSelection()
	sel.Start(0, 0)
	sel.Extend(5, 5)
	sel.Clear()

	if sel.IsActive() {
		t.Fatal("expected inactive after clear")
	}
}

func TestSelectionEmpty(t *testing.T) {
	sel := NewSelection()
	if sel.Text("") != "" {
		t.Fatal("expected empty text for inactive selection")
	}
	if sel.ApplyHighlight("") != "" {
		t.Fatal("expected empty highlight for empty content")
	}
}

func TestSelectionAnsiContent(t *testing.T) {
	// Content with ANSI escapes: visually "hello world" at columns 0-10,
	// but the raw string has extra escape bytes.
	content := "\x1b[2mhello\x1b[0m world"
	sel := NewSelection()
	sel.Start(0, 0)
	sel.Extend(0, 5)
	sel.Finish()

	text := sel.Text(content)
	if text != "hello" {
		t.Fatalf("expected 'hello', got %q", text)
	}

	// Multi-line with ANSI.
	content = "\x1b[2mhello\x1b[0m\n\x1b[1mworld\x1b[0m"
	sel = NewSelection()
	sel.Start(0, 0)
	sel.Extend(1, 5)
	sel.Finish()

	text = sel.Text(content)
	if text != "hello\nworld" {
		t.Fatalf("expected 'hello\\nworld', got %q", text)
	}
}
