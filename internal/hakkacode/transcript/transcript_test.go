package transcript

import (
	"strings"
	"testing"
)

func TestAppendAndEntryAtLine(t *testing.T) {
	tr := New()

	// Append 3 entries: 2 lines, 1 line, 3 lines.
	tr.Append(&TranscriptEntry{Type: EntrySystem, Rendered: []string{"a", "b"}})
	tr.Append(&TranscriptEntry{Type: EntrySystem, Rendered: []string{"c"}})
	tr.Append(&TranscriptEntry{Type: EntrySystem, Rendered: []string{"d", "e", "f"}})

	// Rebuild to compute LineOff.
	content := tr.Rebuild(noopRenderer, 80)
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")

	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d: %q", len(lines), lines)
	}

	tests := []struct {
		line     int
		wantEntry int  // index into entries
		wantRel  int   // relative line within entry
		wantNil  bool
	}{
		{0, 0, 0, false},
		{1, 0, 1, false},
		{2, 1, 0, false},
		{3, 2, 0, false},
		{4, 2, 1, false},
		{5, 2, 2, false},
		{6, -1, -1, true},
		{-1, -1, -1, true},
	}

	for _, tc := range tests {
		e, rel := tr.EntryAtLine(tc.line)
		if tc.wantNil {
			if e != nil {
				t.Errorf("line %d: expected nil, got entry with type %v", tc.line, e.Type)
			}
			continue
		}
		if e == nil {
			t.Errorf("line %d: expected entry %d, got nil", tc.line, tc.wantEntry)
			continue
		}
		if e != tr.entries[tc.wantEntry] {
			t.Errorf("line %d: got entry %p, want entries[%d] (%p)", tc.line, e, tc.wantEntry, tr.entries[tc.wantEntry])
		}
		if rel != tc.wantRel {
			t.Errorf("line %d: rel = %d, want %d", tc.line, rel, tc.wantRel)
		}
	}
}

func TestToggleEntry(t *testing.T) {
	tr := New()

	tr.Append(&TranscriptEntry{Type: EntrySystem, Rendered: []string{"sys"}})
	tool := &TranscriptEntry{
		Type:      EntryToolCall,
		ToolName:  "write_file",
		ToolStatus: ToolOK,
		Collapsed:  true,
		Rendered:   []string{"✓ write_file · foo.go"},
	}
	tr.Append(tool)

	if !tr.ToggleEntry(1) {
		t.Fatal("expected ToggleEntry to return true")
	}
	if tool.Collapsed {
		t.Fatal("expected tool entry to be expanded after toggle")
	}
	if !tr.IsDirty() {
		t.Fatal("expected transcript to be dirty after toggle")
	}

	// Toggle a non-expandable entry should return false.
	if tr.ToggleEntry(0) {
		t.Fatal("expected ToggleEntry on system entry to return false")
	}
}

func TestClickRegionAt(t *testing.T) {
	tr := New()

	e := &TranscriptEntry{
		Type:     EntryCommandResult,
		Rendered: []string{"* abc-full-id  test-session  2026-01-01 12:00:00", "  def-full-id  another       2026-01-02 13:00:00"},
		ClickRegions: []ClickRegion{
			{Line: 0, Col: 2, Width: 11, Action: ClickAction{Action: "session-switch", Payload: "abc-full-id"}},
			{Line: 1, Col: 2, Width: 11, Action: ClickAction{Action: "session-switch", Payload: "def-full-id"}},
		},
	}
	tr.Append(e)
	tr.Rebuild(noopRenderer, 80)

	// Click on the first session id.
	r := tr.ClickRegionAt(0, 5)
	if r == nil {
		t.Fatal("expected click region at (0,5)")
	}
	if r.Action.Action != "session-switch" || r.Action.Payload != "abc-full-id" {
		t.Fatalf("unexpected region: %+v", r)
	}

	// Click on empty area.
	r = tr.ClickRegionAt(0, 20)
	if r != nil {
		t.Fatal("expected nil for click in empty area")
	}

	// Click on second row.
	r = tr.ClickRegionAt(1, 2)
	if r == nil || r.Action.Payload != "def-full-id" {
		t.Fatalf("unexpected region for row 1: %+v", r)
	}
}

func TestIsExpandable(t *testing.T) {
	tool := &TranscriptEntry{Type: EntryToolCall}
	sys := &TranscriptEntry{Type: EntrySystem}

	if !tool.IsExpandable() {
		t.Fatal("tool entry should be expandable")
	}
	if sys.IsExpandable() {
		t.Fatal("system entry should not be expandable")
	}
}

func TestEmptyTranscript(t *testing.T) {
	tr := New()
	if e, _ := tr.EntryAtLine(0); e != nil {
		t.Fatal("expected nil for empty transcript")
	}
	if tr.Content() != "" {
		t.Fatal("expected empty content")
	}
	if tr.LineCount() != 0 {
		t.Fatal("expected 0 line count")
	}
}

func noopRenderer(entry *TranscriptEntry, width int) []string {
	return entry.Rendered
}
