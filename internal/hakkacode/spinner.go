package hakkacode

import (
	"fmt"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinner is a single-line animated status indicator, used to show the
// REPL is waiting on the backend (LLM turnaround, tool execution) rather
// than dead. Only the spinner's own goroutine ever writes to stdout;
// everything else just updates the label under a mutex, so callers never
// race the animation frame with their own output.
type spinner struct {
	mu      sync.Mutex
	label   string
	start   time.Time
	stop    chan struct{}
	done    chan struct{}
	running bool
}

func newSpinner() *spinner {
	return &spinner{}
}

// Start begins animating with the given label, or just updates the label
// if already running.
func (s *spinner) Start(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		s.label = label
		return
	}
	s.running = true
	s.label = label
	s.start = time.Now()
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.loop(s.stop, s.done)
}

// SetLabel updates the label without affecting run state. A no-op if not
// currently running.
func (s *spinner) SetLabel(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		s.label = label
	}
}

// Stop halts the animation and clears its line. Safe to call when not
// running, and safe to call more than once.
func (s *spinner) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	stop, done := s.stop, s.done
	s.mu.Unlock()

	close(stop)
	<-done
}

func (s *spinner) loop(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-stop:
			fmt.Print("\r\033[K")
			return
		case <-ticker.C:
			s.mu.Lock()
			label := s.label
			elapsed := int(time.Since(s.start).Seconds())
			s.mu.Unlock()
			frame := spinnerFrames[i%len(spinnerFrames)]
			fmt.Printf("\r\033[K\033[2m%s %s (%ds)\033[0m", frame, label, elapsed)
			i++
		}
	}
}
