package hakkacode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr       string
	CWD        string
	EnableTags string // e.g. "#all"
}

type App struct {
	cfg         Config
	client      *Client
	sessionID   string
	sessionName string
}

func Run(ctx context.Context, cfg Config) error {
	client, err := Dial(ctx, cfg.Addr)
	if err != nil {
		return err
	}
	defer client.Close()

	app := &App{cfg: cfg, client: client}
	if err := app.bootstrap(ctx); err != nil {
		return err
	}
	if err := app.repl(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (a *App) bootstrap(ctx context.Context) error {
	fmt.Printf("hakka-code · connecting to %s\n", a.cfg.Addr)

	// The server always sends a "welcome" frame immediately on connect
	// (sessions list + auto-subscribe to the best session). We don't need
	// its contents here — a.client.ExecuteCommand skips unrelated frames
	// while waiting for our own commands — but draining it up front keeps
	// the very first read fast and predictable.
	if _, err := a.client.Read(ctx); err != nil {
		return fmt.Errorf("read welcome: %w", err)
	}

	summary, sessionID, resumed, err := a.resumeOrCreateSession(ctx)
	if err != nil {
		return fmt.Errorf("resume/create session: %w", err)
	}
	a.sessionID = sessionID

	name := ""
	shortID := sessionID
	model := ""
	if summary != nil {
		name = summary.Name
		if summary.ShortID != "" {
			shortID = summary.ShortID
		}
		model = summary.Model
	}
	a.sessionName = name

	verb := "session"
	if resumed {
		verb = "resumed session"
	}
	fmt.Printf("%s · %s", verb, shortID)
	if name != "" {
		fmt.Printf(" · %s", name)
	}
	if model != "" {
		fmt.Printf(" · model %s", model)
	}
	fmt.Println()

	if a.cfg.CWD != "" {
		if _, err := a.client.ExecuteCommand(ctx, a.sessionID, "cwd_set", map[string]any{"cwd": a.cfg.CWD}); err != nil {
			fmt.Printf("warning: set cwd: %v\n", err)
		} else {
			fmt.Printf("cwd · %s\n", a.cfg.CWD)
		}
	}

	tags := a.cfg.EnableTags
	if tags == "" {
		tags = "#all"
	}
	if _, err := a.client.ExecuteCommand(ctx, a.sessionID, "tool_allow", map[string]any{"name": tags}); err != nil {
		fmt.Printf("warning: enable tools: %v\n", err)
	} else {
		fmt.Printf("tools · enabled %s\n", tags)
	}

	fmt.Println("type /help for commands, /exit to quit")
	return nil
}

// resumeOrCreateSession resumes the most recently updated non-empty
// session in this namespace, or creates a fresh one if none exist.
func (a *App) resumeOrCreateSession(ctx context.Context) (*SessionSummary, string, bool, error) {
	recent, err := a.client.MostRecentSession(ctx)
	if err != nil {
		return nil, "", false, err
	}
	if recent == nil {
		summary, sessionID, err := a.client.CreateSession(ctx)
		return summary, sessionID, false, err
	}
	id, _ := recent["id"].(string)
	summary, sessionID, err := a.client.GetSession(ctx, id)
	if err != nil {
		return nil, "", false, err
	}
	return summary, sessionID, true, nil
}

func (a *App) repl(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	// Let users paste larger prompts than Scanner's tiny default token limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			return nil
		default:
		}

		fmt.Print("\n> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			fmt.Println()
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		sc, isSlash, err := ParseSlashCommand(line)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		if isSlash {
			if err := a.handleSlash(ctx, sc); err != nil {
				return err
			}
			continue
		}

		if err := a.sendUserInput(ctx, line); err != nil {
			return err
		}
	}
}

func (a *App) handleSlash(ctx context.Context, sc *SlashCommand) error {
	switch sc.Local {
	case "help":
		fmt.Print(HelpText())
		return nil
	case "exit":
		return context.Canceled
	case "cancel":
		if err := a.client.Cancel(a.sessionID); err != nil {
			fmt.Printf("error: cancel: %v\n", err)
		}
		return nil
	case "clear":
		fmt.Print("\033[H\033[2J")
		return nil
	}

	if sc.Remote != nil {
		frame, err := a.client.ExecuteCommand(ctx, a.sessionID, sc.Remote.Cmd, sc.Remote.Params)
		if err != nil {
			return err
		}
		if frame.SessionID != "" {
			a.sessionID = frame.SessionID
		}
		a.trackSessionName(sc.Remote.Cmd, frame)
		a.renderCommandResult(sc.Remote.Cmd, frame)
	}
	return nil
}

func (a *App) sendUserInput(ctx context.Context, input string) error {
	if err := a.client.SendInput(a.sessionID, input); err != nil {
		return err
	}
	fmt.Println()
	if err := a.readUntilDone(ctx); err != nil {
		return err
	}
	a.maybeAutoRename(ctx)
	return nil
}

// maybeAutoRename triggers session_autorename once, after the first turn
// on a session that still has no name.
func (a *App) maybeAutoRename(ctx context.Context) {
	if a.sessionName != "" {
		return
	}
	frame, err := a.client.ExecuteCommand(ctx, a.sessionID, "session_autorename", nil)
	if err != nil || frame.Error != "" {
		return
	}
	session, _ := frame.Data["session"].(map[string]any)
	if name, _ := session["name"].(string); name != "" {
		a.sessionName = name
		fmt.Printf("\033[2msession renamed · %s\033[0m\n", name)
	}
}

// readUntilDone streams a chat turn's frames to the terminal until the
// terminal "done" frame arrives.
//
// Text deltas are not printed live — the final "done" frame always
// carries the complete reply text, which is rendered as markdown in one
// pass. Tool events still render live in between, so a long tool-using
// turn isn't silent. True incremental markdown rendering needs a real
// TUI (redrawing previously-printed lines) and is tracked for Phase 2.
func (a *App) readUntilDone(ctx context.Context) error {
	sawTool := false
	toolStarts := map[string]ResponseFrame{}
	for {
		frame, err := a.client.Read(ctx)
		if err != nil {
			return err
		}

		if frame.SessionID != "" {
			a.sessionID = frame.SessionID
		}

		if frame.Type == "req" {
			_ = a.client.ReplyUnsupportedClientRequest(frame)
			continue
		}

		switch frame.Type {
		case "delta":
			// Buffered — see final "done" text below.
		case "tool":
			sawTool = true
			renderToolEvent(toolStarts, frame)
		case "usage":
			// Keep MVP quiet. Later this can feed a statusline.
		case "done":
			if frame.Text != "" {
				if sawTool {
					fmt.Println()
				}
				fmt.Print(renderMarkdown(frame.Text))
			}
			if frame.Error != "" {
				fmt.Printf("\nerror: %s\n", frame.Error)
			} else if frame.Cancelled {
				fmt.Print("\n[cancelled]\n")
			}
			renderStatusline(frame.Stats)
			return nil
		}
	}
}

// trackSessionName keeps a.sessionName in sync with any command that
// creates, switches to, or renames a session, so maybeAutoRename doesn't
// clobber a name the user (or the switch target) already has.
func (a *App) trackSessionName(cmd string, frame ResponseFrame) {
	var session map[string]any
	switch cmd {
	case "session_create", "get_session":
		session = frame.Session
	case "session_rename":
		session, _ = frame.Data["session"].(map[string]any)
	default:
		return
	}
	if session == nil {
		return
	}
	name, _ := session["name"].(string)
	a.sessionName = name
}

// renderCommandResult prints the response to a slash-command-triggered
// "cmd" request.
func (a *App) renderCommandResult(cmd string, frame ResponseFrame) {
	if frame.Error != "" {
		fmt.Printf("error: %s\n", frame.Error)
		return
	}

	switch frame.Type {
	case "session":
		renderSessionFrame(frame)
	case "result":
		renderData(cmd, frame.Data)
	case "done":
		if frame.Text != "" {
			fmt.Println(frame.Text)
			return
		}
		fmt.Printf("%s: ok\n", cmd)
	default:
		fmt.Printf("%s: ok\n", cmd)
	}
}

func renderSessionFrame(frame ResponseFrame) {
	if frame.Session != nil {
		b, err := json.MarshalIndent(frame.Session, "", "  ")
		if err == nil {
			fmt.Printf("session:\n%s\n", string(b))
		}
	}
	if len(frame.Messages) > 0 {
		fmt.Printf("messages: %d\n", len(frame.Messages))
	}
}

// renderStatusline prints a dim one-line summary after each turn: model,
// context/token usage, and running cost.
func renderStatusline(stats *TurnStats) {
	if stats == nil {
		return
	}
	line := fmt.Sprintf("%s · %d tokens (ctx ~%dk) · $%.4f · %d msgs",
		stats.Model, stats.TotalTokens, (stats.EstimatedContextTokens+999)/1000, stats.TotalCost, stats.MessageCount)
	fmt.Printf("\033[2m%s\033[0m\n", line)
}

// renderToolEvent renders a tool lifecycle frame.
//
// A successful call collapses to a single confirmation line — no
// separate "start" header, no diff preview — since there's nothing to
// act on. A failed call gets the full picture (header, diff/preview,
// error) so there's enough context to debug it; toolStarts (keyed by
// call ID) buffers "start" frames so that context is available once the
// matching "err" frame arrives.
func renderToolEvent(toolStarts map[string]ResponseFrame, frame ResponseFrame) {
	name := frame.Tool
	if name == "" {
		name = "tool"
	}

	switch frame.Status {
	case "start":
		if frame.ID != "" {
			toolStarts[frame.ID] = frame
		}
		return
	case "ok":
		delete(toolStarts, frame.ID)
		snippet := toolSnippet(frame)
		if snippet != "" {
			fmt.Printf("✓ %s · %s\n", name, snippet)
		} else {
			fmt.Printf("✓ %s\n", name)
		}
	case "err":
		start, hadStart := toolStarts[frame.ID]
		delete(toolStarts, frame.ID)
		snippet := toolSnippet(frame)
		if snippet != "" {
			fmt.Printf("\n⏺ %s · %s\n", name, snippet)
		} else {
			fmt.Printf("\n⏺ %s\n", name)
		}
		if hadStart {
			switch frame.Tool {
			case "edit_file":
				renderEditFileDiff(start.Args)
			case "write_file":
				renderWriteFilePreview(start.Args)
			}
		}
		fmt.Printf("  ✗ err\n")
		if frame.Error != "" {
			fmt.Printf("    %s\n", frame.Error)
		}
	}
}

// toolSnippet prefers the server-provided human-readable snippet, falling
// back to a compacted JSON dump of the args.
func toolSnippet(frame ResponseFrame) string {
	if frame.Snippet != "" {
		return frame.Snippet
	}
	if len(frame.Args) > 0 {
		return compactJSON(frame.Args)
	}
	return ""
}

const (
	diffMaxLines = 20
	diffMaxChars = 4000
)

// renderEditFileDiff shows a compact +/- preview of an edit_file call's
// old/new arguments, so the user can eyeball the change without staring
// at raw JSON.
func renderEditFileDiff(args json.RawMessage) {
	if len(args) == 0 {
		return
	}
	var p struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return
	}
	printDiffLines("-", p.Old, "\033[31m")
	printDiffLines("+", p.New, "\033[32m")
}

// renderWriteFilePreview shows the first few lines of a write_file call's
// content argument.
func renderWriteFilePreview(args json.RawMessage) {
	if len(args) == 0 {
		return
	}
	var p struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return
	}
	printDiffLines(" ", p.Content, "\033[2m")
}

func printDiffLines(prefix, text, color string) {
	if text == "" {
		return
	}
	if len(text) > diffMaxChars {
		text = text[:diffMaxChars] + "\n[TRUNCATED]"
	}
	lines := strings.Split(text, "\n")
	shown := lines
	extra := 0
	if len(lines) > diffMaxLines {
		shown = lines[:diffMaxLines]
		extra = len(lines) - diffMaxLines
	}
	for _, l := range shown {
		fmt.Printf("  %s%s %s\033[0m\n", color, prefix, l)
	}
	if extra > 0 {
		fmt.Printf("  \033[2m... (%d more lines)\033[0m\n", extra)
	}
}

func renderData(label string, data map[string]any) {
	if len(data) == 0 {
		fmt.Printf("%s: ok\n", label)
		return
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("%s: ok\n", label)
		return
	}
	fmt.Printf("%s:\n%s\n", label, string(b))
}

func compactJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	if len(b) > 160 {
		return string(b[:157]) + "..."
	}
	return string(b)
}
