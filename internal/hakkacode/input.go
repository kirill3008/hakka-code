package hakkacode

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// inputMaxLines caps how tall the input box is allowed to grow before it
// starts scrolling internally instead.
const inputMaxLines = 6

// inputWidget wraps the textarea with history navigation and dynamic
// resizing. It encapsulates everything related to the user input box.
type inputWidget struct {
	area             textarea.Model
	history          []string
	historyIdx       int // == len(history) means "on the live draft"
	historyDraft     string
	viewportHeightFn func() // called when input height changes
}

// newInputWidget creates a ready-to-use input widget.
func newInputWidget(onHeightChange func()) inputWidget {
	ta := textarea.New()
	ta.Prompt = "❯ "
	ta.Placeholder = "Type a message, or /help for commands"
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.MaxHeight = inputMaxLines
	ta.SetHeight(1)
	ta.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "❯ "
		}
		return "  "
	})
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.KeyMap.InsertNewline.SetEnabled(false)

	return inputWidget{
		area:             ta,
		historyIdx:       0,
		viewportHeightFn: onHeightChange,
	}
}

// SetWidth sets the textarea width with an offset for border padding.
func (w *inputWidget) SetWidth(width int) {
	w.area.SetWidth(width - 3)
}

// Focus gives keyboard focus to the textarea.
func (w *inputWidget) Focus() {
	w.area.Focus()
}

// Value returns the current text.
func (w *inputWidget) Value() string {
	return w.area.Value()
}

// Reset clears the input and resizes to single-line height.
func (w *inputWidget) Reset() {
	w.area.Reset()
	w.area.SetHeight(1)
	if w.viewportHeightFn != nil {
		w.viewportHeightFn()
	}
}

// Height returns the current textarea height in rows.
func (w *inputWidget) Height() int {
	return w.area.Height()
}

// View renders the input box inside a rounded border, matching the
// table accent color.
func (w *inputWidget) View() string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(tableBorderColor)).
		Render(w.area.View())
}

// Update delegates to the textarea and resizes the input box to fit.
func (w *inputWidget) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	w.area, cmd = w.area.Update(msg)
	w.resizeToFit()
	return cmd
}

// Blink returns the cursor blink tea.Cmd, so the caller can include it
// in the initial batch (textarea.Blink is a package-level var, not a
// method).
func (w *inputWidget) Blink() tea.Cmd {
	return textarea.Blink
}

// HistoryUp navigates to the previous history entry. Returns true if
// the action was consumed (caller should skip normal processing).
func (w *inputWidget) HistoryUp() bool {
	if len(w.history) == 0 || w.historyIdx == 0 {
		return false
	}
	if w.historyIdx == len(w.history) {
		w.historyDraft = w.area.Value()
	}
	w.historyIdx--
	w.area.SetValue(w.history[w.historyIdx])
	w.resizeToFit()
	return true
}

// HistoryDown navigates to the next history entry. Returns true if
// consumed.
func (w *inputWidget) HistoryDown() bool {
	if w.historyIdx >= len(w.history) {
		return false
	}
	w.historyIdx++
	if w.historyIdx == len(w.history) {
		w.area.SetValue(w.historyDraft)
	} else {
		w.area.SetValue(w.history[w.historyIdx])
	}
	w.resizeToFit()
	return true
}

// PushHistory appends a submitted line to history and resets the
// navigation cursor to the live draft position.
func (w *inputWidget) PushHistory(line string) {
	w.history = append(w.history, line)
	w.historyIdx = len(w.history)
	w.historyDraft = ""
}

// resizeToFit computes the visual line count of the input content and
// sets the textarea height accordingly (clamped to inputMaxLines).
func (w *inputWidget) resizeToFit() {
	needed := w.visualLines()
	if needed < 1 {
		needed = 1
	}
	if needed > inputMaxLines {
		needed = inputMaxLines
	}
	if needed != w.area.Height() {
		w.area.SetHeight(needed)
		if w.viewportHeightFn != nil {
			w.viewportHeightFn()
		}
	}
}

// visualLines counts how many visual (wrapped) rows the current input
// content occupies, including prompt columns.
func (w *inputWidget) visualLines() int {
	content := w.area.Value()
	avail := w.area.Width() - 2
	if avail < 10 {
		avail = 80
	}
	lines := strings.Split(content, "\n")
	total := 0
	for _, line := range lines {
		runeLen := utf8.RuneCountInString(line)
		if runeLen == 0 {
			total++
			continue
		}
		wrapped := (runeLen + avail - 1) / avail
		if wrapped < 1 {
			wrapped = 1
		}
		total += wrapped
	}
	if total < 1 {
		total = 1
	}
	return total
}
