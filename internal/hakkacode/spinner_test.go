package hakkacode

import (
	"testing"
	"time"
)

func TestSpinnerStartStopLifecycle(t *testing.T) {
	s := newSpinner()
	s.Start("Thinking")
	time.Sleep(150 * time.Millisecond) // let at least one tick fire
	s.SetLabel("Writing response")
	time.Sleep(150 * time.Millisecond)
	s.Stop()
	// Stop should be idempotent and safe to call again.
	s.Stop()
}

func TestSpinnerRestartAfterStop(t *testing.T) {
	s := newSpinner()
	s.Start("Thinking")
	s.Stop()
	s.Start("Running tool")
	s.Stop()
}

func TestSpinnerSetLabelNoopWhenNotRunning(t *testing.T) {
	s := newSpinner()
	// Must not panic or block when called on an idle spinner.
	s.SetLabel("should be ignored")
	s.Stop()
}

func TestToolsLabel(t *testing.T) {
	cases := []struct {
		name    string
		running map[string]ResponseFrame
		want    string
	}{
		{"none", map[string]ResponseFrame{}, "Thinking"},
		{"one", map[string]ResponseFrame{"a": {Tool: "edit_file"}}, "Running edit_file"},
		{"many", map[string]ResponseFrame{"a": {Tool: "edit_file"}, "b": {Tool: "shell"}}, "Running 2 tools"},
	}
	for _, c := range cases {
		if got := toolsLabel(c.running); got != c.want {
			t.Errorf("%s: toolsLabel() = %q, want %q", c.name, got, c.want)
		}
	}
}
