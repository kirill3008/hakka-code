package hakkacode

import "testing"

func TestHistoryUpDown(t *testing.T) {
	m := newModel(nil, Config{}, nil)
	m.history = []string{"first", "second", "third"}
	m.historyIdx = len(m.history)

	m = m.historyUp()
	if got := m.input.Value(); got != "third" {
		t.Fatalf("historyUp #1 = %q, want %q", got, "third")
	}
	m = m.historyUp()
	if got := m.input.Value(); got != "second" {
		t.Fatalf("historyUp #2 = %q, want %q", got, "second")
	}
	m = m.historyDown()
	if got := m.input.Value(); got != "third" {
		t.Fatalf("historyDown #1 = %q, want %q", got, "third")
	}
	m = m.historyDown()
	if got := m.input.Value(); got != "" {
		t.Fatalf("historyDown past newest = %q, want empty draft", got)
	}
}

func TestHistoryUpAtStartIsNoop(t *testing.T) {
	m := newModel(nil, Config{}, nil)
	m.history = []string{"only"}
	m.historyIdx = 0
	m = m.historyUp()
	if got := m.input.Value(); got != "" {
		t.Fatalf("expected no-op at oldest entry, got %q", got)
	}
}

func TestHistoryPreservesDraft(t *testing.T) {
	m := newModel(nil, Config{}, nil)
	m.history = []string{"old"}
	m.historyIdx = len(m.history)
	m.input.SetValue("unsent draft")

	m = m.historyUp()
	if got := m.input.Value(); got != "old" {
		t.Fatalf("historyUp = %q, want %q", got, "old")
	}
	m = m.historyDown()
	if got := m.input.Value(); got != "unsent draft" {
		t.Fatalf("historyDown should restore draft, got %q", got)
	}
}
