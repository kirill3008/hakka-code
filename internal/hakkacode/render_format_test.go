package hakkacode

import (
	"strings"
	"testing"
)

func TestFormatSessionList(t *testing.T) {
	data := map[string]any{
		"sessions": []any{
			map[string]any{"id": "abc-full-id", "short_id": "abc", "name": "test", "current": true, "message_count": 3.0, "updated_at": "2026-01-01T00:00:00Z"},
		},
	}
	out := formatSessionList(data)
	if strings.Contains(out, "{") {
		t.Fatalf("expected human-readable output, not JSON: %q", out)
	}
	if !strings.Contains(out, "abc-full-id") {
		t.Fatalf("expected full session id for copy/switch, got: %q", out)
	}
	if !strings.Contains(out, "test") {
		t.Fatalf("missing expected fields: %q", out)
	}
}

func TestFormatSessionListAlignsDespiteLongNames(t *testing.T) {
	data := map[string]any{
		"sessions": []any{
			map[string]any{"id": "1", "name": "short", "message_count": 1.0, "updated_at": "2026-01-01T00:00:00Z"},
			map[string]any{"id": "2", "name": "a much much longer session name", "message_count": 2.0, "updated_at": "2026-01-01T00:00:00Z"},
		},
	}
	out := formatSessionList(data)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 3 { // header + 2 rows
		t.Fatalf("expected header + 2 rows, got %d lines: %q", len(lines), out)
	}
	// A real table keeps every row the same rendered width; naive
	// %-Ns padding breaks once a value exceeds N.
	width := visibleLen(lines[0])
	for i, l := range lines {
		if got := visibleLen(l); got != width {
			t.Fatalf("line %d width = %d, want %d (table misaligned): %q", i, got, width, l)
		}
	}
}

func TestFormatModelList(t *testing.T) {
	data := map[string]any{"models": []any{
		map[string]any{"name": "deepseek", "current": true},
		map[string]any{"name": "claude", "current": false},
	}}
	out := formatModelList(data)
	if !strings.Contains(out, "*") || !strings.Contains(out, "deepseek") {
		t.Fatalf("expected current model marked, got: %q", out)
	}
}

func TestLocalTimeConvertsFromUTC(t *testing.T) {
	out := localTime("2026-01-01T00:00:00Z")
	if out == "2026-01-01T00:00:00Z" {
		t.Fatal("expected timestamp to be reformatted, not left as raw UTC RFC3339")
	}
}

func TestFormatMessageHistory(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": "hello there"},
	}
	out := formatMessageHistory(msgs)
	if !strings.Contains(out, "hi") || !strings.Contains(out, "hello there") {
		t.Fatalf("expected both messages rendered: %q", out)
	}
}
