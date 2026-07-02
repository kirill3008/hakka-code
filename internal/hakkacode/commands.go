package hakkacode

import (
	"fmt"
	"strconv"
	"strings"
)

type SlashCommand struct {
	Local  string
	Remote *Command
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

	sc := &SlashCommand{}
	switch cmd {
	case "help", "?":
		sc.Local = "help"
	case "exit", "quit", "q":
		sc.Local = "exit"
	case "cancel":
		sc.Local = "cancel"
	case "clear":
		sc.Local = "clear"
	case "session":
		return parseSessionCommand(args)
	case "model", "models":
		return parseModelCommand(cmd, args)
	case "tool", "tools":
		return parseToolCommand(args)
	case "cwd":
		if len(args) != 1 {
			return nil, true, fmt.Errorf("usage: /cwd <path>")
		}
		sc.Remote = &Command{Cmd: "cwd_set", Params: map[string]any{"cwd": args[0]}}
	case "compact":
		if len(args) == 0 {
			sc.Remote = &Command{Cmd: "compact"}
			break
		}
		if len(args) != 1 {
			return nil, true, fmt.Errorf("usage: /compact [n]")
		}
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return nil, true, fmt.Errorf("compact n must be an integer")
		}
		sc.Remote = &Command{Cmd: "compact", Params: map[string]any{"n": n}}
	default:
		return nil, true, fmt.Errorf("unknown command %q; try /help", fields[0])
	}
	return sc, true, nil
}

func parseSessionCommand(args []string) (*SlashCommand, bool, error) {
	if len(args) == 0 || args[0] == "info" {
		return &SlashCommand{Remote: &Command{Cmd: "session_info"}}, true, nil
	}
	switch args[0] {
	case "list", "ls":
		return &SlashCommand{Remote: &Command{Cmd: "session_list"}}, true, nil
	case "create", "new":
		return &SlashCommand{Remote: &Command{Cmd: "session_create"}}, true, nil
	case "get", "switch", "use":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("usage: /session switch <id>")
		}
		return &SlashCommand{Remote: &Command{Cmd: "get_session", Params: map[string]any{"id": args[1]}}}, true, nil
	case "rename":
		if len(args) < 2 {
			return nil, true, fmt.Errorf("usage: /session rename <name>")
		}
		return &SlashCommand{Remote: &Command{Cmd: "session_rename", Params: map[string]any{"name": strings.Join(args[1:], " ")}}}, true, nil
	case "delete", "rm":
		if len(args) != 2 {
			return nil, true, fmt.Errorf("usage: /session delete <id|this>")
		}
		return &SlashCommand{Remote: &Command{Cmd: "session_delete", Params: map[string]any{"id": args[1]}}}, true, nil
	default:
		return nil, true, fmt.Errorf("unknown /session command; try /help")
	}
}

func parseModelCommand(cmd string, args []string) (*SlashCommand, bool, error) {
	if cmd == "models" || len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		return &SlashCommand{Remote: &Command{Cmd: "model_list"}}, true, nil
	}
	if len(args) == 1 {
		return &SlashCommand{Remote: &Command{Cmd: "model_switch", Params: map[string]any{"name": args[0]}}}, true, nil
	}
	return nil, true, fmt.Errorf("usage: /model [name] or /model list")
}

func parseToolCommand(args []string) (*SlashCommand, bool, error) {
	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		return &SlashCommand{Remote: &Command{Cmd: "tool_list"}}, true, nil
	}
	if len(args) != 2 {
		return nil, true, fmt.Errorf("usage: /tool allow <name|#tag> | /tool deny <name|#tag> | /tool list")
	}
	switch args[0] {
	case "allow", "enable":
		return &SlashCommand{Remote: &Command{Cmd: "tool_allow", Params: map[string]any{"name": args[1]}}}, true, nil
	case "deny", "disable":
		return &SlashCommand{Remote: &Command{Cmd: "tool_deny", Params: map[string]any{"name": args[1]}}}, true, nil
	default:
		return nil, true, fmt.Errorf("unknown /tool command; try /help")
	}
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
`
}
