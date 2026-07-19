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

// sessionState groups everything tied to the session currently shown —
// its identity and the turn in flight against it — as distinct from the
// TUI's widget/rendering state.
type sessionState struct {
	id   string
	name string

	// turn is non-nil only while a turn is in flight.
	turn *turnState
}

type model struct {
	ctx    context.Context
	cfg    Config
	client backend.Backend

	session sessionState

	viewport viewport.Model
	input    inputWidget
	spinner  spinnerWidget
	ready    bool
	width    int
	height   int

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
			m.session.name = msg.name
			m.appendLine(dimf("session renamed · %s", msg.name))
		}
		return m, nil

	case copyToClipboardMsg:
		return m.handleCopyToClipboard(msg)

	case spinTickMsg:
		if !m.session.turn.active() {
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
		if m.session.turn.active() {
			m.spinner.setLabel("Cancelling")
			return m, cancelTurnCmd(m.ctx, m.client, m.session.id)
		}
		return m, tea.Quit

	case tea.KeyPgUp:
		m.viewport.PageUp()
		return m, nil
	case tea.KeyPgDown:
		m.viewport.PageDown()
		return m, nil

	case tea.KeyEnter:
		if m.session.turn.active() {
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

	if err := m.client.SendInput(m.session.id, line); err != nil {
		m.fatalErr = err
		return m, tea.Quit
	}
	m.appendUserPrompt(line)
	m.session.turn = newTurnState()
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
		if m.session.turn.active() {
			m.spinner.setLabel("Cancelling")
		}
		return m, cancelTurnCmd(m.ctx, m.client, m.session.id)
	case "clear":
		m.transcriptEntries = transcript.New()
		m.viewport.SetContent("")
		return m, nil
	}
	if sc.Remote != nil {
		return m, execRemoteCmd(m.ctx, m.client, m.session.id, sc.Remote.Cmd, sc.Remote.Params)
	}
	return m, nil
}

func (m model) handleBoot(msg bootMsg) model {
	if msg.err != nil {
		m.fatalErr = msg.err
		m.appendLine(fmt.Sprintf("error: %v", msg.err))
		return m
	}
	m.session.id = msg.sessionID

	name, shortID, mdl := "", msg.sessionID, ""
	if msg.summary != nil {
		name = msg.summary.Name
		if msg.summary.ShortID != "" {
			shortID = msg.summary.ShortID
		}
		mdl = msg.summary.Model
	}
	m.session.name = name

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
		m.session.turn = newLazyTurnState()
		for _, evt := range msg.events {
			frame := eventToResponseFrame(evt)
			mdl, _ := m.handleFrame(frame)
			m = mdl.(model)
		}
		m.session.turn = nil
		// Live turns get a trailing statusline that visually separates them
		// from the next prompt; replayed history has no stats to produce
		// one, so add an explicit spacer instead.
		m.appendEntry(&transcript.TranscriptEntry{Type: transcript.EntrySpacer, Raw: ""})
	}
	return m
}

func (m model) handleCmdResult(msg cmdResultMsg) model {
	if msg.err != nil {
		m.appendLine(fmt.Sprintf("error: %v", msg.err))
		return m
	}
	if msg.frame.SessionID != "" {
		m.session.id = msg.frame.SessionID
	}
	if name, ok := trackedSessionName(msg.cmd, msg.frame); ok {
		m.session.name = name
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

// handleFrame dispatches an inbound frame to the handler for its type. It
// owns only cross-cutting concerns — replying to proxied client requests
// and dropping frames for sessions other than the one currently shown —
// leaving each frame type's own state changes to its handle*Frame method.
func (m model) handleFrame(frame protocol.ResponseFrame) (tea.Model, tea.Cmd) {
	if frame.Type == protocol.TypeReq {
		_ = m.client.ReplyUnsupportedClientRequest(frame)
		return m, waitFrame(m.ctx, m.client)
	}

	// The server fans frames for other sessions across this same socket
	// (background activity, welcome auto-subscribe). Only frames for the
	// session currently shown should drive turn state — anything else is
	// dropped here instead of corrupting or panicking the active turn.
	if frame.SessionID != "" && m.session.id != "" && frame.SessionID != m.session.id {
		return m, waitFrame(m.ctx, m.client)
	}

	if frame.SessionID != "" {
		m.session.id = frame.SessionID
	}

	switch frame.Type {
	case protocol.TypeChat:
		m = m.handleChatFrame(frame)
	case protocol.TypeDelta:
		m = m.handleDeltaFrame(frame)
	case protocol.TypeTool:
		m = m.handleToolFrame(frame)
	case protocol.TypeDone:
		var cmd tea.Cmd
		m, cmd = m.handleDoneFrame(frame)
		return m, cmd
	}

	return m, waitFrame(m.ctx, m.client)
}

// handleChatFrame echoes a replayed user prompt. Live chat frames aren't
// sent to handleFrame — this only fires during history replay.
func (m model) handleChatFrame(frame protocol.ResponseFrame) model {
	if frame.Text != "" {
		m.appendUserPrompt(frame.Text)
	}
	return m
}

// handleDeltaFrame appends or extends the in-flight assistant text as
// streamed prose arrives.
func (m model) handleDeltaFrame(frame protocol.ResponseFrame) model {
	m.spinner.setLabel("Writing response")
	if frame.Text != "" && m.session.turn.active() {
		m.session.turn.addDelta(frame.Text, m.appendEntry, m.updateStreamingEntry)
	}
	return m
}

// handleToolFrame records a tool call's start, or renders its completion
// once the matching start/finish pair is available.
func (m model) handleToolFrame(frame protocol.ResponseFrame) model {
	if !m.session.turn.active() {
		return m
	}
	m.session.turn.sawTool = true
	if frame.Status == protocol.StatusStart {
		m.session.turn.recordToolStart(frame)
		m.spinner.setLabel(m.session.turn.toolsLabel())
		return m
	}

	// Finalise any streaming prose as markdown before the tool call.
	m.session.turn.streamFinalize(m.updateStreamingEntry)

	startFrame := m.session.turn.finishTool(frame)
	if out := renderToolEvent(startFrame, frame); out != "" {
		m.session.turn.appendToolCall(m.appendEntry, frame, startFrame)
	}
	m.spinner.setLabel(m.session.turn.toolsLabel())
	return m
}

// handleDoneFrame finalises the in-flight turn: renders any trailing text,
// surfaces errors/cancellation, appends the turn's statusline, and clears
// turn state. It kicks off auto-rename for still-unnamed sessions.
func (m model) handleDoneFrame(frame protocol.ResponseFrame) (model, tea.Cmd) {
	if !m.session.turn.active() {
		return m, nil
	}
	m.session.turn.streamFinalizeOrAppend(frame.Text, m.appendEntry, m.updateStreamingEntry)
	if frame.Error != "" {
		m.appendError("error: " + frame.Error)
	} else if frame.Cancelled {
		m.appendLine("[cancelled]")
	}
	if s := renderStatusline(frame.Stats); s != "" {
		m.appendStatusLine(s)
	}
	m.session.turn = nil
	var cmd tea.Cmd
	if m.session.name == "" {
		cmd = autoRenameCmd(m.ctx, m.client, m.session.id)
	}
	return m, cmd
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
	if m.session.turn.active() {
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
