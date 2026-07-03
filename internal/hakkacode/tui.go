package hakkacode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/atotto/clipboard"

	"hakka-code/internal/hakkacode/backend"
	"hakka-code/internal/hakkacode/protocol"
	"hakka-code/internal/hakkacode/transcript"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

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
	events    []map[string]any // history replay as wire-format events
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
	input    inputWidget
	spinner  spinnerWidget
	ready    bool
	width    int
	height   int

	// turn is non-nil only while a turn is in flight.
	turn *turnState

	transcriptEntries *transcript.Transcript
	selection         *transcript.Selection

	fatalErr error
}

func newModel(ctx context.Context, cfg Config, client backend.Backend) model {
	// We need a mutable reference to model to wire the viewport-height
	// callback. Use a closure capture pattern.
	m := model{
		ctx:               ctx,
		cfg:               cfg,
		client:            client,
		transcriptEntries: transcript.New(),
		selection:         transcript.NewSelection(),
	}
	m.input = newInputWidget(func() { m.recomputeViewportHeight() })
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(bootCmd(m.ctx, m.client, m.cfg), m.input.Blink())
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
		return m.handleBoot(msg), m.input.Blink()

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
		if !m.turn.active() {
			return m, nil
		}
		m.spinner.tick()
		return m, spinTick()
	}
	return m, nil
}

func (m model) handleResize(msg tea.WindowSizeMsg) model {
	m.width = msg.Width
	m.height = msg.Height
	m.input.SetWidth(msg.Width)
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
	vpHeight := m.height - m.input.Height() - 4
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
		if m.turn.active() {
			m.spinner.setLabel("Cancelling")
			return m, cancelTurnCmd(m.ctx, m.client, m.sessionID)
		}
		return m, tea.Quit

	case tea.KeyPgUp:
		m.viewport.PageUp()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.PageDown()
		return m, nil

	case tea.KeyEnter:
		if m.turn.active() {
			return m, nil
		}
		return m.submit()

	case tea.KeyUp:
		if m.input.Value() == "" {
			m.input.HistoryUp()
			return m, nil
		}
	case tea.KeyDown:
		if m.input.Value() == "" {
			m.input.HistoryDown()
			return m, nil
		}
	}

	return m, m.input.Update(msg)
}

func (m model) submit() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	m.recomputeViewportHeight()
	if line == "" {
		return m, nil
	}
	m.input.PushHistory(line)

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
	m.turn = newTurnState()
	m.spinner.start("Thinking")
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
		if m.turn.active() {
			m.spinner.setLabel("Cancelling")
		}
		return m, cancelTurnCmd(m.ctx, m.client, m.sessionID)
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

	// Replay stored history events through the same code path as live
	// turns — each event is converted to a ResponseFrame and processed
	// by handleFrame. This ensures tool rendering is identical to live
	// chat, with args/snippet from the server driving the display.
	//
	// A temporary turnState is injected so handleFrame's tool/delta/done
	// handling has a place to accumulate state — just like a live turn.
	if len(msg.events) > 0 {
		m.turn = newTurnState()
		for _, evt := range msg.events {
			frame := eventToResponseFrame(evt)
			mdl, _ := m.handleFrame(frame)
			m = mdl.(model)
		}
		m.turn = nil
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
		last := m.transcriptEntries.LastEntry()
		if last != nil && last.Type == transcript.EntryCommandResult {
			m.transcriptEntries.Pop()
			spacer := m.transcriptEntries.LastEntry()
			if spacer != nil && spacer.Type == transcript.EntrySpacer {
				m.transcriptEntries.Pop()
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
	case protocol.TypeChat:
		// History replay only — live chat frames aren't sent to
		// handleFrame. Echo the user prompt just like submit() does.
		if frame.Text != "" {
			m.appendUserPrompt(frame.Text)
		}
	case protocol.TypeDelta:
		m.spinner.setLabel("Writing response")
		if frame.Text != "" && m.turn.active() {
			m.turn.addDelta(frame.Text)
		}
	case protocol.TypeTool:
		if !m.turn.active() {
			break
		}
		m.turn.sawTool = true
		if frame.Status == protocol.StatusStart {
			m.turn.recordToolStart(frame)
			m.spinner.setLabel(m.turn.toolsLabel())
		} else {
			// Flush any assistant text that arrived before this tool result.
			m.turn.flushAssistant(m.appendEntry)

			startFrame := m.turn.finishTool(frame.ID)

			if out := renderToolEvent(startFrame, frame); out != "" {
				m.turn.appendToolCall(m.appendEntry, frame, startFrame)
			}
			m.spinner.setLabel(m.turn.toolsLabel())
		}
	case protocol.TypeUsage:
	case protocol.TypeDone:
		m.turn.flushAssistant(m.appendEntry)
		if frame.Text != "" {
			m.turn.appendRemainingText(frame.Text, m.appendEntry)
		}
		if frame.Error != "" {
			m.appendError("error: " + frame.Error)
		} else if frame.Cancelled {
			m.appendLine("[cancelled]")
		}
		if s := renderStatusline(frame.Stats); s != "" {
			m.appendStatusLine(s)
		}
		m.turn = nil
		var cmd tea.Cmd
		if m.sessionName == "" {
			cmd = autoRenameCmd(m.ctx, m.client, m.sessionID)
		}
		return m, cmd
	}

	return m, waitFrame(m.ctx, m.client)
}

func (m model) handleCopyToClipboard(msg copyToClipboardMsg) (tea.Model, tea.Cmd) {
	_ = clipboard.WriteAll(msg.text)
	return m, nil
}

// --- View ---

func (m model) View() string {
	if !m.ready {
		return "connecting…\n"
	}

	vpContent := m.viewport.View()
	if m.selection.IsActive() {
		vpContent = applySelectionToViewport(vpContent, m.selection, m.viewport.YOffset)
	}

	var status string
	if m.turn.active() {
		status = m.spinner.view()
	} else {
		status = dim("ready")
	}

	return vpContent + "\n\n" + status + "\n" + m.input.View()
}

// applySelectionToViewport applies the selection highlight to the
// viewport's visible portion. It maps the selection's absolute
// transcript coordinates into the viewport's visible window, then
// applies reverse-video highlighting only to lines that fall within
// the visible range.
func applySelectionToViewport(vpContent string, sel *transcript.Selection, yOffset int) string {
	rawLines := strings.Split(vpContent, "\n")
	hasTrailingNewline := vpContent != "" && vpContent[len(vpContent)-1] == '\n'

	sl, sc, el, ec := sel.Normalized()

	relStart := sl - yOffset
	relEnd := el - yOffset

	var result []string
	for i, line := range rawLines {
		if i == len(rawLines)-1 && hasTrailingNewline && line == "" {
			result = append(result, "")
			continue
		}

		if i < relStart || i > relEnd {
			result = append(result, line)
		} else if relStart == relEnd {
			result = append(result, transcript.HighlightRegion(line, sc, ec))
		} else if i == relStart {
			result = append(result, transcript.HighlightRegion(line, sc, -1))
		} else if i == relEnd {
			result = append(result, transcript.HighlightRegion(line, 0, ec))
		} else {
			result = append(result, transcript.WrapFullLine(line))
		}
	}

	return strings.Join(result, "\n")
}
