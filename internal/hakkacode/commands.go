package hakkacode

import (
	"fmt"
	"strconv"
	"strings"

	"hakka-code/internal/hakkacode/protocol"
)

type SlashCommand struct {
	Local  string
	Remote *protocol.Command
}

type commandParser func(args []string) (*SlashCommand, error)

var commandRegistry = map[string]commandParser{}

func init() {
	// local commands
	reg("help", local("help"))
	reg("?", local("help"))
	reg("exit", local("exit"))
	reg("quit", local("exit"))
	reg("q", local("exit"))
	reg("cancel", local("cancel"))
	reg("clear", local("clear"))

	// remote commands
	reg("session", parseSession)
	reg("model", parseModel)
	reg("models", parseModelList)
	reg("tool", parseTool)
	reg("tools", parseToolList)
	reg("cwd", parseCWD)
	reg("compact", parseCompact)
}

func reg(name string, p commandParser) { commandRegistry[name] = p }

func local(name string) commandParser {
	return func(_ []string) (*SlashCommand, error) {
		return &SlashCommand{Local: name}, nil
	}
}

func ParseSlashCommand(line string) (*SlashCommand, bool, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return nil, false, nil
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, true, fmt.Errorf("empty command")
	}

	cmd := strings.TrimPrefix(fields[0], "/")
	args := fields[1:]

	p, ok := commandRegistry[cmd]
	if !ok {
		return nil, true, fmt.Errorf("unknown command %q; try /help", fields[0])
	}

	sc, err := p(args)
	return sc, true, err
}

func parseSession(args []string) (*SlashCommand, error) {
	if len(args) == 0 || args[0] == "info" {
		return &SlashCommand{Remote: &protocol.Command{Cmd: "session_info"}}, nil
	}
	switch args[0] {
	case "list", "ls":
		return &SlashCommand{Remote: &protocol.Command{Cmd: "session_list"}}, nil
	case "create", "new":
		return &SlashCommand{Remote: &protocol.Command{Cmd: "session_create"}}, nil
	case "get", "switch", "use":
		if len(args) != 2 {
			return nil, fmt.Errorf("usage: /session switch <id>")
		}
		return &SlashCommand{Remote: &protocol.Command{Cmd: "get_session", Params: map[string]any{"id": args[1]}}}, nil
	case "rename":
		if len(args) < 2 {
			return nil, fmt.Errorf("usage: /session rename <name>")
		}
		return &SlashCommand{Remote: &protocol.Command{Cmd: "session_rename", Params: map[string]any{"name": strings.Join(args[1:], " ")}}}, nil
	case "delete", "rm":
		if len(args) != 2 {
			return nil, fmt.Errorf("usage: /session delete <id|this>")
		}
		return &SlashCommand{Remote: &protocol.Command{Cmd: "session_delete", Params: map[string]any{"id": args[1]}}}, nil
	default:
		return nil, fmt.Errorf("unknown /session command; try /help")
	}
}

func parseModel(args []string) (*SlashCommand, error) {
	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		return &SlashCommand{Remote: &protocol.Command{Cmd: "model_list"}}, nil
	}
	if len(args) == 1 {
		return &SlashCommand{Remote: &protocol.Command{Cmd: "model_switch", Params: map[string]any{"name": args[0]}}}, nil
	}
	return nil, fmt.Errorf("usage: /model [name] or /model list")
}

func parseModelList(args []string) (*SlashCommand, error) {
	return &SlashCommand{Remote: &protocol.Command{Cmd: "model_list"}}, nil
}

func parseTool(args []string) (*SlashCommand, error) {
	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		return &SlashCommand{Remote: &protocol.Command{Cmd: "tool_list"}}, nil
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("usage: /tool allow <name|#tag> | /tool deny <name|#tag> | /tool list")
	}
	switch args[0] {
	case "allow", "enable":
		return &SlashCommand{Remote: &protocol.Command{Cmd: "tool_allow", Params: map[string]any{"name": args[1]}}}, nil
	case "deny", "disable":
		return &SlashCommand{Remote: &protocol.Command{Cmd: "tool_deny", Params: map[string]any{"name": args[1]}}}, nil
	default:
		return nil, fmt.Errorf("unknown /tool command; try /help")
	}
}

func parseToolList(args []string) (*SlashCommand, error) {
	return &SlashCommand{Remote: &protocol.Command{Cmd: "tool_list"}}, nil
}

func parseCWD(args []string) (*SlashCommand, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("usage: /cwd <path>")
	}
	return &SlashCommand{Remote: &protocol.Command{Cmd: "cwd_set", Params: map[string]any{"cwd": args[0]}}}, nil
}

func parseCompact(args []string) (*SlashCommand, error) {
	if len(args) == 0 {
		return &SlashCommand{Remote: &protocol.Command{Cmd: "compact"}}, nil
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("usage: /compact [n]")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("compact n must be an integer")
	}
	return &SlashCommand{Remote: &protocol.Command{Cmd: "compact", Params: map[string]any{"n": n}}}, nil
}

func HelpText() string {
	return `Commands:
  /help, /?                 Show this help
  /exit, /quit, /q           Exit hakka-code
  /cancel                   Cancel current turn
  /clear                    Clear terminal screen
  /session                  Show current session info
  /session list             List sessions
  /session create           Create/switch to fresh session
  /session switch <id>      Fetch/switch to session by id or prefix
  /session rename <name>    Rename current session
  /session delete <id|this> Delete a session
  /model list               List models
  /model <name>             Switch model
  /tool list                List tools
  /tool allow <name|#tag>   Enable a tool or tag, e.g. #all
  /tool deny <name|#tag>    Disable a tool or tag
  /cwd <path>               Set session working directory
  /compact [n]              Show or set context soft limit

Keys:
  Ctrl+P / Ctrl+N           Previous/next prompt from history
  PgUp / PgDown             Scroll the transcript by a page
  Up / Down / mouse wheel   Scroll the transcript by a line
`
}
