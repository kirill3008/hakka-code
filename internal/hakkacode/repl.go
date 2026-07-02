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
	Addr string
	CWD  string
}

type App struct {
	cfg       Config
	client    *Client
	sessionID string
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

	summary, sessionID, err := a.client.CreateSession(ctx)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
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

	fmt.Printf("session · %s", shortID)
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

	if _, err := a.client.ExecuteCommand(ctx, a.sessionID, "tool_allow", map[string]any{"name": "#all"}); err != nil {
		fmt.Printf("warning: enable tools: %v\n", err)
	} else {
		fmt.Println("tools · enabled #all")
	}

	fmt.Println("type /help for commands, /exit to quit")
	return nil
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
		a.renderCommandResult(sc.Remote.Cmd, frame)
	}
	return nil
}

func (a *App) sendUserInput(ctx context.Context, input string) error {
	if err := a.client.SendInput(a.sessionID, input); err != nil {
		return err
	}
	fmt.Println()
	return a.readUntilDone(ctx)
}

// readUntilDone streams a chat turn's frames to the terminal until the
// terminal "done" frame arrives.
func (a *App) readUntilDone(ctx context.Context) error {
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
			fmt.Print(frame.Text)
		case "tool":
			renderToolEvent(frame)
		case "usage":
			// Keep MVP quiet. Later this can feed a statusline.
		case "done":
			if frame.Error != "" {
				fmt.Printf("\nerror: %s\n", frame.Error)
			} else if frame.Cancelled {
				fmt.Print("\n[cancelled]\n")
			} else {
				fmt.Println()
			}
			return nil
		}
	}
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

func renderToolEvent(frame ResponseFrame) {
	name := frame.Tool
	if name == "" {
		name = "tool"
	}
	snippet := frame.Snippet
	if snippet == "" && len(frame.Args) > 0 {
		snippet = compactJSON(frame.Args)
	}
	if snippet != "" {
		fmt.Printf("\n⏺ %s · %s\n", name, snippet)
	} else {
		fmt.Printf("\n⏺ %s\n", name)
	}
	if frame.Status != "" {
		mark := "⎿"
		switch frame.Status {
		case "ok":
			mark = "✓"
		case "err":
			mark = "✗"
		}
		fmt.Printf("  %s %s\n", mark, frame.Status)
	}
	if frame.Status == "err" && frame.Error != "" {
		fmt.Printf("    %s\n", frame.Error)
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
