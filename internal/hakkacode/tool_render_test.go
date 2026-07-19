package hakkacode

import (
	"strings"
	"testing"

	"hakka-code/internal/hakkacode/protocol"
)

func TestRenderToolEventStartIsSilent(t *testing.T) {
	frame := protocol.ResponseFrame{
		Type:   "tool",
		Tool:   "edit_file",
		ID:     "call_1",
		Status: "start",
		Args:   []byte(`{"path":"foo.go","old":"bar","new":"baz"}`),
	}
	out := renderToolEvent(nil, frame)
	if out != "" {
		t.Fatalf("expected no output on a bare start event, got: %q", out)
	}
}

func TestRenderToolEventOkCollapsesToOneLine(t *testing.T) {
	frame := protocol.ResponseFrame{
		Type:       "tool",
		Tool:       "write_file",
		ID:         "call_1",
		Status:     "ok",
		Snippet:    "foo.go",
		ToolResult: "Written 12 bytes to foo.go",
	}
	out := renderToolEvent(nil, frame)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one line for a successful call, got %d: %q", len(lines), out)
	}
	if !strings.Contains(out, "✓") || !strings.Contains(out, "write_file") {
		t.Fatalf("expected a one-line ok confirmation, got: %q", out)
	}
}

func TestRenderToolEventErrShowsFullDetail(t *testing.T) {
	startFrame := &protocol.ResponseFrame{
		Tool:   "edit_file",
		ID:     "call_1",
		Status: "start",
		Args:   []byte(`{"path":"foo.go","old":"bar","new":"baz"}`),
	}
	frame := protocol.ResponseFrame{
		Type:   "tool",
		Tool:   "edit_file",
		ID:     "call_1",
		Status: "err",
		Error:  "pattern not found in foo.go",
	}
	out := renderToolEvent(startFrame, frame)
	if !strings.Contains(out, "-bar") || !strings.Contains(out, "+baz") {
		t.Fatalf("expected diff lines from the buffered start frame, got: %q", out)
	}
	if !strings.Contains(out, "✗ err") {
		t.Fatalf("expected an err status marker, got: %q", out)
	}
	if !strings.Contains(out, "pattern not found in foo.go") {
		t.Fatalf("expected the error message, got: %q", out)
	}
}

func TestRenderToolEventErrWithoutStartFrame(t *testing.T) {
	frame := protocol.ResponseFrame{
		Type:   "tool",
		Tool:   "some_tool",
		ID:     "call_1",
		Status: "err",
		Error:  "something went wrong",
	}
	out := renderToolEvent(nil, frame)
	if !strings.Contains(out, "✗ err") {
		t.Fatalf("expected err marker: %q", out)
	}
	if !strings.Contains(out, "something went wrong") {
		t.Fatalf("expected error message: %q", out)
	}
	// Diff should not appear without a start frame.
	if strings.Contains(out, "\n  -") || strings.Contains(out, "\n  +") {
		t.Fatalf("expected no diff lines without start frame: %q", out)
	}
}
