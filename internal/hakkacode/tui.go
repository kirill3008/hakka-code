package hakkacode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
//
// Alt screen + a managed viewport, not inline scrollback: the input box
// stays reliably pinned right below the transcript this way. The
// tradeoff is losing the terminal's native mouse-wheel scrollback — PgUp
// /PgDown cover that instead. No mouse mode, so native text
// selection/copy still works.
func Run(ctx context.Context, cfg Config) error {
	client, err := Dial(ctx, cfg.Addr)
	if err != nil {
		return err
	}
	defer client.Close()

	// Must happen before the terminal enters Bubble Tea's raw mode — see
	// detectTerminalTheme's doc comment.
	detectTerminalTheme()

	m := newModel(ctx, cfg, client)
	p := tea.NewProgram(m, tea.WithAltScreen())

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
	summary *SessionSummary
	sessionID string
	messages []map[string]any
	resumed bool
	cwdWarn string
	toolWarn string
	tags string
	err error
}

type frameMsg ResponseFrame
type readErrMsg struct{ err error }
type spinTickMsg struct{}
type cmdResultMsg struct {
	cmd   string
	frame ResponseFrame
	err   error
}
type autoRenameMsg struct {
	name string
}

// --- model ---

type model struct {
	ctx    context.Context
	cfg    Config
	client *Client

	sessionID   string
	sessionName string

	viewport viewport.Model
	input    textarea.Model
	ready    bool
	width    int
	height   int

	turnActive bool
	toolStarts map[string]ResponseFrame
	sawTool    bool
	spinIdx    int
	spinLabel  string
	spinStart  time.Time

	transcript string
	fatalErr   error

	history      []string
	historyIdx   int // == len(history) means "on the live draft, not browsing"
	historyDraft string
}

func newModel(ctx context.Context, cfg Config, client *Client) model {
	ta := textarea.New()
	ta.Prompt = "❯ " // used for the placeholder line; SetPromptFunc governs real content
	ta.Placeholder = "Type a message, or /help for commands"
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.MaxHeight = inputMaxLines
	ta.SetHeight(1)
	// Only the first wrapped/logical line gets the prompt glyph;
	// continuation lines (wrapped or pasted) get blank padding instead
	// of repeating "❯ " on every row.
	ta.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "❯ "
		}
		return "  "
	})
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // no distinct current-line highlight
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.KeyMap.InsertNewline.SetEnabled(false) // Enter is handled entirely by handleKey

	return model{
		ctx:        ctx,
		cfg:        cfg,
		client:     client,
		input:      ta,
		toolStarts: map[string]ResponseFrame{},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(bootCmd(m.ctx, m.client, m.cfg), textarea.Blink)
}

// --- boot sequence ---

func bootCmd(ctx context.Context, client *Client, cfg Config) tea.Cmd {
	return func() tea.Msg {
		// The server always sends a "welcome" frame immediately on
		// connect. We don't need its contents — ExecuteCommand skips
		// unrelated frames while waiting for our own commands — but
		// draining it up front keeps behavior predictable.
		if _, err := client.Read(ctx); err != nil {
			return bootMsg{err: fmt.Errorf("read welcome: %w", err)}
		}

		summary, sessionID, messages, resumed, err := resumeOrCreateSession(ctx, client)
		if err != nil {
			return bootMsg{err: fmt.Errorf("resume/create session: %w", err)}
		}

		msg := bootMsg{summary: summary, sessionID: sessionID, messages: messages, resumed: resumed}

		if cfg.CWD != "" {
			if _, err := client.ExecuteCommand(ctx, sessionID, "cwd_set", map[string]any{"cwd": cfg.CWD}); err != nil {
				msg.cwdWarn = err.Error()
			}
		}

		tags := cfg.EnableTags
		if tags == "" {
			tags = "#all"
		}
		msg.tags = tags
		if _, err := client.ExecuteCommand(ctx, sessionID, "tool_allow", map[string]any{"name": tags}); err != nil {
			msg.toolWarn = err.Error()
		}

		return msg
	}
}

// resumeOrCreateSession resumes the most recently updated non-empty
// session in this namespace, or creates a fresh one if none exist.
func resumeOrCreateSession(ctx context.Context, client *Client) (*SessionSummary, string, []map[string]any, bool, error) {
	recent, err := client.MostRecentSession(ctx)
	if err != nil {
		return nil, "", nil, false, err
	}
	if recent == nil {
		summary, sessionID, err := client.CreateSession(ctx)
		return summary, sessionID, nil, false, err
	}
	id, _ := recent["id"].(string)
	summary, sessionID, messages, err := client.GetSession(ctx, id)
	if err != nil {
		return nil, "", nil, false, err
	}
	return summary, sessionID, messages, true, nil
}

// --- async command helpers ---

func waitFrame(ctx context.Context, client *Client) tea.Cmd {
	return func() tea.Msg {
		frame, err := client.Read(ctx)
		if err != nil {
			return readErrMsg{err}
		}
		return frameMsg(frame)
	}
}

func execRemoteCmd(ctx context.Context, client *Client, sessionID, cmd string, params map[string]any) tea.Cmd {
	return func() tea.Msg {
		frame, err := client.ExecuteCommand(ctx, sessionID, cmd, params)
		return cmdResultMsg{cmd: cmd, frame: frame, err: err}
	}
}

func autoRenameCmd(ctx context.Context, client *Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		frame, err := client.ExecuteCommand(ctx, sessionID, "session_autorename", nil)
		if err != nil || frame.Error != "" {
			return autoRenameMsg{}
		}
		session, _ := frame.Data["session"].(map[string]any)
		name, _ := session["name"].(string)
		return autoRenameMsg{name: name}
	}
}

func cancelTurnCmd(client *Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		_ = client.Cancel(sessionID)
		return nil
	}
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

	case bootMsg:
		return m.handleBoot(msg), textarea.Blink

	case frameMsg:
		return m.handleFrame(ResponseFrame(msg))

	case readErrMsg:
		m.fatalErr = msg.err
		return m, tea.Quit

	case cmdResultMsg:
		return m.handleCmdResult(msg), nil

	case autoRenameMsg:
		if msg.name != "" {
			m.sessionName = msg.name
			m.appendLine(fmt.Sprintf("\033[2msession renamed · %s\033[0m", msg.name))
		}
		return m, nil

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
	// Border consumes 2 columns; leave 1 column of breathing room inside.
	m.input.SetWidth(msg.Width - 3)
	if !m.ready {
		m.viewport = viewport.New(msg.Width, 1)
		m.viewport.SetContent(m.transcript)
		m.input.Focus()
		m.ready = true
	} else {
		m.viewport.Width = msg.Width
	}
	m.recomputeViewportHeight()
	m.viewport.GotoBottom()
	return m
}

// recomputeViewportHeight gives the viewport whatever vertical space is
// left after the status line (with a blank separator above it) and the
// bordered, possibly multi-line input box.
func (m *model) recomputeViewportHeight() {
	const statusArea = 2 // blank separator + status line
	inputBoxHeight := m.input.Height() + 2 // + top/bottom border
	vpHeight := m.height - statusArea - inputBoxHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.Height = vpHeight
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			// Type-ahead: keep whatever's typed, submit once the
			// current turn finishes and Enter is pressed again.
			return m, nil
		}
		return m.submit()

	case tea.KeyCtrlP:
		return m.historyUp(), nil
	case tea.KeyCtrlN:
		return m.historyDown(), nil

	// History moved to Ctrl+P/Ctrl+N, so plain Up/Down are free to
	// scroll — but only at the input box's top/bottom edge, so arrow
	// keys still navigate normally within a multi-line draft otherwise.
	// This is also what makes mouse-wheel scroll work again: without
	// mouse mode, this terminal sends wheel scroll as synthesized
	// Up/Down key presses.
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

	// Pin tall during Update() so its internal auto-scroll never fires
	// early (its scroll offset is sticky and doesn't reset on its own).
	m.input.SetHeight(inputMaxLines)
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if msg.Paste {
		// Shrinking after the fact doesn't move the scroll offset, so a
		// paste that scrolled while pinned tall would stay scrolled
		// wrong even at the right height. Leave it tall (correctly
		// positioned already) — it'll shrink on the next keystroke.
		m.recomputeViewportHeight()
	} else {
		m.shrinkInputToFit()
	}
	return m, cmd
}

// shrinkInputToFit sizes the box down to what its content needs (up to
// inputMaxLines) after handleKey pinned it tall for Update().
//
// LineInfo().Height only covers the cursor's current logical line, with
// no public way to size earlier ones — a multi-line paste could need
// more than that per line, so multi-line content just keeps max height
// rather than risk shrinking below what's showing.
func (m *model) shrinkInputToFit() {
	var lines int
	if m.input.LineCount() > 1 {
		lines = inputMaxLines
	} else {
		lines = m.input.LineInfo().Height
	}
	if lines < 1 {
		lines = 1
	}
	if lines > inputMaxLines {
		lines = inputMaxLines
	}
	if lines != m.input.Height() {
		m.input.SetHeight(lines)
		m.recomputeViewportHeight()
	}
}

// historyUp/historyDown browse previously submitted lines, like shell
// history. historyDraft preserves whatever was being typed before
// browsing started, restored on historyDown past the newest entry.
func (m model) historyUp() model {
	if len(m.history) == 0 || m.historyIdx == 0 {
		return m
	}
	if m.historyIdx == len(m.history) {
		m.historyDraft = m.input.Value()
	}
	m.historyIdx--
	m.input.SetValue(m.history[m.historyIdx])
	m.shrinkInputToFit()
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
	m.shrinkInputToFit()
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
	m.appendLine(strings.TrimRight(renderUserPrompt("❯ "+line), "\n"))
	m.turnActive = true
	m.toolStarts = map[string]ResponseFrame{}
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
		m.transcript = ""
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
	// Leading blank line: command output otherwise butts right up
	// against whatever was printed before it with no separation.
	m.appendLine("\n" + strings.TrimRight(renderCommandResult(msg.cmd, msg.frame), "\n"))
	return m
}

func (m model) handleFrame(frame ResponseFrame) (tea.Model, tea.Cmd) {
	if frame.SessionID != "" {
		m.sessionID = frame.SessionID
	}

	if frame.Type == "req" {
		_ = m.client.ReplyUnsupportedClientRequest(frame)
		return m, waitFrame(m.ctx, m.client)
	}

	switch frame.Type {
	case "delta":
		m.spinLabel = "Writing response"
	case "tool":
		m.sawTool = true
		if frame.Status == "start" {
			renderToolEvent(m.toolStarts, frame) // buffers only, no text
			m.spinLabel = toolsLabel(m.toolStarts)
		} else {
			if out := renderToolEvent(m.toolStarts, frame); out != "" {
				m.appendLine(strings.TrimRight(out, "\n"))
			}
			m.spinLabel = toolsLabel(m.toolStarts)
		}
	case "usage":
		// Not surfaced live; final totals land in the "done" statusline.
	case "done":
		m.turnActive = false
		if frame.Text != "" {
			if m.sawTool {
				m.transcript += "\n"
			}
			// Trailing "\n" left in on purpose: appendLine adds its own
			// separator before the next line, so this leaves one full
			// blank line after the response for readability before
			// whatever comes next (error/statusline/next prompt).
			m.appendLine(strings.TrimRight(renderMarkdown(frame.Text), "\n") + "\n")
		}
		if frame.Error != "" {
			m.appendLine("error: " + frame.Error)
		} else if frame.Cancelled {
			m.appendLine("[cancelled]")
		}
		if s := renderStatusline(frame.Stats); s != "" {
			m.appendLine(strings.TrimRight(s, "\n"))
		}
		var cmd tea.Cmd
		if m.sessionName == "" {
			cmd = autoRenameCmd(m.ctx, m.client, m.sessionID)
		}
		return m, cmd
	}

	return m, waitFrame(m.ctx, m.client)
}

// appendLine adds text to the transcript and, if the viewport is already
// scrolled to the bottom, keeps it pinned there. A user who's scrolled up
// to read history isn't yanked back down by new output.
func (m *model) appendLine(text string) {
	stick := !m.ready || m.viewport.AtBottom()
	if len(m.transcript) > 0 {
		m.transcript += "\n"
	}
	m.transcript += text
	if m.ready {
		m.viewport.SetContent(m.transcript)
		if stick {
			m.viewport.GotoBottom()
		}
	}
}

// --- View ---

func (m model) View() string {
	if !m.ready {
		return "connecting…\n"
	}

	var status string
	if m.turnActive {
		frame := spinnerFrames[m.spinIdx%len(spinnerFrames)]
		elapsed := int(time.Since(m.spinStart).Seconds())
		status = fmt.Sprintf("\033[2m%s %s (%ds)\033[0m", frame, m.spinLabel, elapsed)
	} else {
		status = "\033[2mready\033[0m"
	}

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(inputBorderColor)).
		Render(m.input.View())

	return m.viewport.View() + "\n\n" + status + "\n" + inputBox
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
