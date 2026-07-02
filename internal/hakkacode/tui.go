package hakkacode

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"hakka-code/internal/hakkacode/backend"
	"hakka-code/internal/hakkacode/protocol"
	"hakka-code/internal/hakkacode/transcript"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// inputMaxLines caps how tall the input box is allowed to grow before it
// starts scrolling internally instead.
const inputMaxLines = 6

// inputBorderColor matches the table border's green so bordered UI chrome
// reads as one consistent accent color.
const inputBorderColor = tableBorderColor

type Config struct {
	Addr       string
	CWD        string
	EnableTags string // e.g. "#all"
}

// Run connects to the backend and drives the interactive TUI until the
// user quits or ctx is cancelled (e.g. SIGTERM).
func Run(ctx context.Context, cfg Config) error {
	client, err := backend.Dial(ctx, cfg.Addr)
	if err != nil {
		return err
	}
	defer client.Close()

	// Must happen before the terminal enters Bubble Tea's raw mode — see
	// detectTerminalTheme's doc comment.
	detectTerminalTheme()

	m := newModel(ctx, cfg, client)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(model); ok && fm.fatalErr != nil && !errors.Is(fm.fatalErr, context.Canceled) {
		return fm.fatalErr
	}
	return nil
}

// --- messages ---

type bootMsg struct {
	summary   *protocol.SessionSummary
	sessionID string
	messages  []map[string]any
	resumed   bool
	cwdWarn   string
	toolWarn  string
	tags      string
	err       error
}

type frameMsg protocol.ResponseFrame
type readErrMsg struct{ err error }
type spinTickMsg struct{}
type cmdResultMsg struct {
	cmd   string
	frame protocol.ResponseFrame
	err   error
}
type autoRenameMsg struct {
	name string
}
type copyToClipboardMsg struct {
	text string
}

// --- model ---

type model struct {
	ctx    context.Context
	cfg    Config
	client backend.Backend

	sessionID   string
	sessionName string

	viewport viewport.Model
	input    textarea.Model
	ready    bool
	width    int
	height   int

	turnActive bool
	toolStarts map[string]protocol.ResponseFrame
	sawTool    bool
	spinIdx    int
	spinLabel  string
	spinStart  time.Time

	transcriptEntries *transcript.Transcript
	selection         *transcript.Selection

	fatalErr error

	history      []string
	historyIdx   int // == len(history) means "on the live draft, not browsing"
	historyDraft string
}

func newModel(ctx context.Context, cfg Config, client backend.Backend) model {
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

	return model{
		ctx:               ctx,
		cfg:               cfg,
		client:            client,
		input:             ta,
		toolStarts:        map[string]protocol.ResponseFrame{},
		transcriptEntries: transcript.New(),
		selection:         transcript.NewSelection(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(bootCmd(m.ctx, m.client, m.cfg), textarea.Blink)
}

func spinTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// --- Update ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleResize(msg), nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case bootMsg:
		return m.handleBoot(msg), textarea.Blink

	case frameMsg:
		return m.handleFrame(protocol.ResponseFrame(msg))

	case readErrMsg:
		m.fatalErr = msg.err
		return m, tea.Quit

	case cmdResultMsg:
		return m.handleCmdResult(msg), nil

	case autoRenameMsg:
		if msg.name != "" {
			m.sessionName = msg.name
			m.appendLine(dimf("session renamed · %s", msg.name))
		}
		return m, nil

	case copyToClipboardMsg:
		return m.handleCopyToClipboard(msg)

	case spinTickMsg:
		if !m.turnActive {
			return m, nil
		}
		m.spinIdx++
		return m, spinTick()
	}
	return m, nil
}

func (m model) handleResize(msg tea.WindowSizeMsg) model {
	m.width = msg.Width
	m.height = msg.Height
	m.input.SetWidth(msg.Width - 3)
	if !m.ready {
		m.viewport = viewport.New(msg.Width, 1)
		m.viewport.SetContent(m.transcriptEntries.String())
		m.input.Focus()
		m.ready = true
	} else {
		m.viewport.Width = msg.Width
	}
	m.recomputeViewportHeight()
	m.viewport.GotoBottom()
	return m
}

func (m *model) recomputeViewportHeight() {
	const statusArea = 2
	inputBoxHeight := m.input.Height() + 2
	vpHeight := m.height - statusArea - inputBoxHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.Height = vpHeight
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.selection.State == transcript.SelDone {
		m.selection.Clear()
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.turnActive {
			m.spinLabel = "Cancelling"
			return m, cancelTurnCmd(m.client, m.sessionID)
		}
		return m, tea.Quit

	case tea.KeyPgUp:
		m.viewport.PageUp()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.PageDown()
		return m, nil

	case tea.KeyEnter:
		if m.turnActive {
			return m, nil
		}
		return m.submit()

	case tea.KeyCtrlP:
		return m.historyUp(), nil
	case tea.KeyCtrlN:
		return m.historyDown(), nil

	case tea.KeyUp:
		if m.input.LineInfo().RowOffset == 0 {
			m.viewport.LineUp(1)
			return m, nil
		}
	case tea.KeyDown:
		li := m.input.LineInfo()
		if li.RowOffset == li.Height-1 {
			m.viewport.LineDown(1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.resizeInputToFit()
	return m, cmd
}

// resizeInputToFit computes the visual line count of the input content
// and sets the textarea height accordingly (clamped to inputMaxLines).
// It also fixes the textarea's scroll offset which can drift on resize.
func (m *model) resizeInputToFit() {
	needed := m.inputVisualLines()
	if needed < 1 {
		needed = 1
	}
	if needed > inputMaxLines {
		needed = inputMaxLines
	}
	if needed != m.input.Height() {
		m.input.SetHeight(needed)
		m.recomputeViewportHeight()
	}
}

// inputVisualLines counts how many visual (wrapped) rows the current
// input content occupies, including prompt columns. This is more
// reliable than LineInfo().Height which only covers the cursor's line.
func (m *model) inputVisualLines() int {
	content := m.input.Value()
	// The textarea width minus 2 accounts for prompt padding columns.
	avail := m.input.Width() - 2
	if avail < 10 {
		avail = 80
	}
	lines := strings.Split(content, "\n")
	total := 0
	for _, line := range lines {
		// Empty line still occupies one row.
		runeLen := utf8.RuneCountInString(line)
		if runeLen == 0 {
			total++
			continue
		}
		// Each full width worth of runes wraps to another visual row.
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

func (m model) historyUp() model {
	if len(m.history) == 0 || m.historyIdx == 0 {
		return m
	}
	if m.historyIdx == len(m.history) {
		m.historyDraft = m.input.Value()
	}
	m.historyIdx--
	m.input.SetValue(m.history[m.historyIdx])
	m.resizeInputToFit()
	return m
}

func (m model) historyDown() model {
	if m.historyIdx >= len(m.history) {
		return m
	}
	m.historyIdx++
	if m.historyIdx == len(m.history) {
		m.input.SetValue(m.historyDraft)
	} else {
		m.input.SetValue(m.history[m.historyIdx])
	}
	m.resizeInputToFit()
	return m
}

func (m model) submit() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	m.input.SetHeight(1)
	m.recomputeViewportHeight()
	if line == "" {
		return m, nil
	}
	m.history = append(m.history, line)
	m.historyIdx = len(m.history)
	m.historyDraft = ""

	sc, isSlash, err := ParseSlashCommand(line)
	if err != nil {
		m.appendLine(fmt.Sprintf("error: %v", err))
		return m, nil
	}
	if isSlash {
		return m.handleSlash(sc)
	}

	if err := m.client.SendInput(m.sessionID, line); err != nil {
		m.fatalErr = err
		return m, tea.Quit
	}
	m.appendUserPrompt(line)
	m.turnActive = true
	m.toolStarts = map[string]protocol.ResponseFrame{}
	m.sawTool = false
	m.spinIdx = 0
	m.spinLabel = "Thinking"
	m.spinStart = time.Now()
	return m, tea.Batch(waitFrame(m.ctx, m.client), spinTick())
}

func (m model) handleSlash(sc *SlashCommand) (tea.Model, tea.Cmd) {
	switch sc.Local {
	case "help":
		m.appendLine(HelpText())
		return m, nil
	case "exit":
		return m, tea.Quit
	case "cancel":
		if m.turnActive {
			m.spinLabel = "Cancelling"
		}
		return m, cancelTurnCmd(m.client, m.sessionID)
	case "clear":
		m.transcriptEntries = transcript.New()
		m.viewport.SetContent("")
		return m, nil
	}
	if sc.Remote != nil {
		return m, execRemoteCmd(m.ctx, m.client, m.sessionID, sc.Remote.Cmd, sc.Remote.Params)
	}
	return m, nil
}

func (m model) handleBoot(msg bootMsg) model {
	if msg.err != nil {
		m.fatalErr = msg.err
		m.appendLine(fmt.Sprintf("error: %v", msg.err))
		return m
	}
	m.sessionID = msg.sessionID

	name, shortID, mdl := "", msg.sessionID, ""
	if msg.summary != nil {
		name = msg.summary.Name
		if msg.summary.ShortID != "" {
			shortID = msg.summary.ShortID
		}
		mdl = msg.summary.Model
	}
	m.sessionName = name

	verb := "session"
	if msg.resumed {
		verb = "resumed session"
	}
	line := fmt.Sprintf("hakka-code · %s\n%s · %s", m.cfg.Addr, verb, shortID)
	if name != "" {
		line += " · " + name
	}
	if mdl != "" {
		line += " · model " + mdl
	}
	m.appendLine(line)

	if m.cfg.CWD != "" {
		if msg.cwdWarn != "" {
			m.appendLine("warning: set cwd: " + msg.cwdWarn)
		} else {
			m.appendLine("cwd · " + m.cfg.CWD)
		}
	}
	if msg.toolWarn != "" {
		m.appendLine("warning: enable tools: " + msg.toolWarn)
	} else {
		m.appendLine("tools · enabled " + msg.tags)
	}
	m.appendLine("type /help for commands, /exit to quit")
	if len(msg.messages) > 0 {
		m.appendLine("\n" + strings.TrimRight(formatMessageHistory(msg.messages), "\n"))
	}
	return m
}

func (m model) handleCmdResult(msg cmdResultMsg) model {
	if msg.err != nil {
		m.appendLine(fmt.Sprintf("error: %v", msg.err))
		return m
	}
	if msg.frame.SessionID != "" {
		m.sessionID = msg.frame.SessionID
	}
	if name, ok := trackedSessionName(msg.cmd, msg.frame); ok {
		m.sessionName = name
	}

	// tool_allow/tool_deny are silenced — the refresh command that
	// follows will produce the visible output.
	if msg.cmd == "tool_allow" || msg.cmd == "tool_deny" {
		return m
	}

	// For list refreshes (tool_list, model_list), replace the previous
	// list entry if one exists, so the scrollback doesn't accumulate
	// duplicate lists.
	if msg.cmd == "tool_list" || msg.cmd == "model_list" || msg.cmd == "session_list" {
		// Pop the last command result + the spacer before it.
		last := m.transcriptEntries.LastEntry()
		if last != nil && last.Type == transcript.EntryCommandResult {
			m.transcriptEntries.Pop() // the list result
			spacer := m.transcriptEntries.LastEntry()
			if spacer != nil && spacer.Type == transcript.EntrySpacer {
				m.transcriptEntries.Pop() // the spacer before it
			}
		}
	}

	m.appendEntry(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
	res := renderCommandResultInteractive(msg.cmd, msg.frame)
	m.appendCommandResult(res)
	return m
}

func (m model) handleFrame(frame protocol.ResponseFrame) (tea.Model, tea.Cmd) {
	if frame.SessionID != "" {
		m.sessionID = frame.SessionID
	}

	if frame.Type == protocol.TypeReq {
		_ = m.client.ReplyUnsupportedClientRequest(frame)
		return m, waitFrame(m.ctx, m.client)
	}

	switch frame.Type {
	case protocol.TypeDelta:
		m.spinLabel = "Writing response"
	case protocol.TypeTool:
		m.sawTool = true
		if frame.Status == protocol.StatusStart {
			if frame.ID != "" {
				m.toolStarts[frame.ID] = frame
			}
			m.spinLabel = toolsLabel(m.toolStarts)
		} else {
			var startFrame *protocol.ResponseFrame
			if s, ok := m.toolStarts[frame.ID]; ok {
				startFrame = &s
			}
			delete(m.toolStarts, frame.ID)

			out := renderToolEvent(startFrame, frame)
			if out != "" {
				status := transcript.ToolOK
				if frame.Status == protocol.StatusErr {
					status = transcript.ToolErr
				}
				snippet := toolSnippet(frame)
				m.appendToolCall(toolNameFromFrame(frame), frame.ID, status, frame.Args, snippet, frame.Error)
			}
			m.spinLabel = toolsLabel(m.toolStarts)
		}
	case protocol.TypeUsage:
	case protocol.TypeDone:
		m.turnActive = false
		if frame.Text != "" {
			if m.sawTool {
				m.appendEntry(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
			}
			m.appendAssistantText(frame.Text)
		}
		if frame.Error != "" {
			m.appendError("error: " + frame.Error)
		} else if frame.Cancelled {
			m.appendLine("[cancelled]")
		}
		if s := renderStatusline(frame.Stats); s != "" {
			m.appendStatusLine(s)
		}
		var cmd tea.Cmd
		if m.sessionName == "" {
			cmd = autoRenameCmd(m.ctx, m.client, m.sessionID)
		}
		return m, cmd
	}

	return m, waitFrame(m.ctx, m.client)
}

func (m model) handleCopyToClipboard(msg copyToClipboardMsg) (tea.Model, tea.Cmd) {
	return m, tea.Printf("\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(msg.text)))
}

// --- View ---

func (m model) View() string {
	if !m.ready {
		return "connecting…\n"
	}

	var status string
	if m.turnActive {
		frame := spinnerFrames[m.spinIdx%len(spinnerFrames)]
		elapsed := time.Since(m.spinStart)
		status = dimf("%s %s (%s)", frame, m.spinLabel, formatDuration(elapsed))
	} else {
		status = dim("ready")
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(inputBorderColor)).
		Render(m.input.View())

	return m.viewport.View() + "\n\n" + status + "\n" + inputBox
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
