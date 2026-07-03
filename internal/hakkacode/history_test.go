package hakkacode

import "testing"

func TestHistoryUpDown(t *testing.T) {
	w := newInputWidget(nil)
	w.history = []string{"first", "second", "third"}
	w.historyIdx = len(w.history)

	ok := w.HistoryUp()
	if !ok {
		t.Fatal("HistoryUp #1 returned false")
	}
	if got := w.Value(); got != "third" {
		t.Fatalf("HistoryUp #1 = %q, want %q", got, "third")
	}

	ok = w.HistoryUp()
	if !ok {
		t.Fatal("HistoryUp #2 returned false")
	}
	if got := w.Value(); got != "second" {
		t.Fatalf("HistoryUp #2 = %q, want %q", got, "second")
	}

	ok = w.HistoryDown()
	if !ok {
		t.Fatal("HistoryDown #1 returned false")
	}
	if got := w.Value(); got != "third" {
		t.Fatalf("HistoryDown #1 = %q, want %q", got, "third")
	}

	ok = w.HistoryDown()
	if !ok {
		t.Fatal("HistoryDown #2 returned false")
	}
	if got := w.Value(); got != "" {
		t.Fatalf("HistoryDown past newest = %q, want empty draft", got)
	}
}

func TestHistoryUpAtStartIsNoop(t *testing.T) {
	w := newInputWidget(nil)
	w.history = []string{"only"}
	w.historyIdx = 0

	ok := w.HistoryUp()
	if ok {
		t.Fatal("expected no-op at oldest entry")
	}
	if got := w.Value(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestHistoryPreservesDraft(t *testing.T) {
	w := newInputWidget(nil)
	w.history = []string{"old"}
	w.historyIdx = len(w.history)
	w.area.SetValue("unsent draft")

	ok := w.HistoryUp()
	if !ok {
		t.Fatal("HistoryUp returned false")
	}
	if got := w.Value(); got != "old" {
		t.Fatalf("HistoryUp = %q, want %q", got, "old")
	}

	ok = w.HistoryDown()
	if !ok {
		t.Fatal("HistoryDown returned false")
	}
	if got := w.Value(); got != "unsent draft" {
		t.Fatalf("HistoryDown should restore draft, got %q", got)
	}
}
