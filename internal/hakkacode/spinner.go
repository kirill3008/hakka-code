package hakkacode

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// spinnerWidget is a tiny state machine for the animated spinner shown
// during an active turn. It tracks which frame to display, the label
// text (e.g. "Thinking", "Running shell"), and the elapsed time.
type spinnerWidget struct {
	idx       int
	label     string
	startTime time.Time
}

// start resets the spinner for a new turn with the given label.
func (s *spinnerWidget) start(label string) {
	s.idx = 0
	s.label = label
	s.startTime = time.Now()
}

// tick advances the animation frame by one.
func (s *spinnerWidget) tick() {
	s.idx++
}

// setLabel changes the label text without affecting the frame counter.
func (s *spinnerWidget) setLabel(label string) {
	s.label = label
}

// view returns the spinner's display string, e.g. "⠋ Thinking (3s)".
func (s *spinnerWidget) view() string {
	frame := spinnerFrames[s.idx%len(spinnerFrames)]
	elapsed := formatDuration(time.Since(s.startTime))
	return dimf("%s %s (%s)", frame, s.label, elapsed)
}

// active returns true if the spinner has been started (non-zero startTime).
func (s *spinnerWidget) active() bool {
	return !s.startTime.IsZero()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// formatDuration renders a duration as a compact human-readable string,
// e.g. "3s", "2m15s", "1h3m".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// spinTick returns a tea.Cmd that fires a spinTickMsg every 100ms.
func spinTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return spinTickMsg{} })
}
