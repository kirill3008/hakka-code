package hakkacode

import (
	"strings"
	"testing"
)

func TestRenderToolEventStartIsSilent(t *testing.T) {
	starts := map[string]ResponseFrame{}
	frame := ResponseFrame{
		Type:   "tool",
		Tool:   "edit_file",
		ID:     "call_1",
		Status: "start",
		Args:   []byte(`{"path":"foo.go","old":"bar","new":"baz"}`),
	}
	out := renderToolEvent(starts, frame)
	if out != "" {
		t.Fatalf("expected no output on a bare start event, got: %q", out)
	}
	if _, ok := starts["call_1"]; !ok {
		t.Fatal("expected start frame to be buffered by call ID")
	}
}

func TestRenderToolEventOkCollapsesToOneLine(t *testing.T) {
	starts := map[string]ResponseFrame{"call_1": {Tool: "write_file", ID: "call_1", Status: "start"}}
	frame := ResponseFrame{
		Type:       "tool",
		Tool:       "write_file",
		ID:         "call_1",
		Status:     "ok",
		Snippet:    "foo.go",
		ToolResult: "Written 12 bytes to foo.go",
	}
	out := renderToolEvent(starts, frame)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly one line for a successful call, got %d: %q", len(lines), out)
	}
	if !strings.Contains(out, "✓") || !strings.Contains(out, "write_file") {
		t.Fatalf("expected a one-line ok confirmation, got: %q", out)
	}
	if _, ok := starts["call_1"]; ok {
		t.Fatal("expected start buffer entry to be cleared on completion")
	}
}

func TestRenderToolEventErrShowsFullDetail(t *testing.T) {
	starts := map[string]ResponseFrame{
		"call_1": {
			Tool:   "edit_file",
			ID:     "call_1",
			Status: "start",
			Args:   []byte(`{"path":"foo.go","old":"bar","new":"baz"}`),
		},
	}
	frame := ResponseFrame{
		Type:   "tool",
		Tool:   "edit_file",
		ID:     "call_1",
		Status: "err",
		Error:  "pattern not found in foo.go",
	}
	out := renderToolEvent(starts, frame)
	if !strings.Contains(out, "- bar") || !strings.Contains(out, "+ baz") {
		t.Fatalf("expected diff lines from the buffered start frame, got: %q", out)
	}
	if !strings.Contains(out, "✗ err") {
		t.Fatalf("expected an err status marker, got: %q", out)
	}
	if !strings.Contains(out, "pattern not found in foo.go") {
		t.Fatalf("expected the error message, got: %q", out)
	}
}
