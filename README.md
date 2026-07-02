# hakka-code

A standalone terminal frontend for the [Hakka](https://github.com/ariloulaleelay/hakka) agent backend, inspired by the ergonomics of Claude Code.

This repository is intentionally separate from the upstream Hakka backend for now. It talks to a running Hakka instance over the WebSocket protocol.

## Status

Early MVP, protocol v2.

Current features:

- Connects to Hakka's v2 WebSocket protocol
- Resumes the most recently updated non-empty session on startup, or creates a fresh one
- Sets session cwd to the current directory
- Enables tools on startup (`#all` by default, configurable)
- Simple REPL chat loop with markdown-rendered assistant replies (via glamour)
- Diff-style rendering for `edit_file`/`write_file` tool calls
- Auto-renames a session (via LLM) after its first exchange
- Per-turn statusline: model, token/context usage, cost
- Animated spinner while waiting on the LLM or a running tool, so a quiet turn doesn't look hung
- Ctrl+C cancels the in-flight turn (not the whole program); a second Ctrl+C at the idle prompt exits normally
- Tool calls collapse to a single confirmation line on success; full detail (diff/preview, error) only shows on failure
- Client-side slash command parsing
- Optional config file at `~/.hakka-code.json` (`addr`, `enable_tags`)

## Requirements

- Go 1.23+
- Running Hakka backend, usually:

```sh
hakka --config hakka.json --db ~/.hakka.db
```

The backend should expose:

```text
ws://127.0.0.1:8765/ws
```

## Build

```sh
go build -o bin/hakka-code ./cmd/hakka-code
```

## Run

```sh
./bin/hakka-code
```

Custom backend address:

```sh
./bin/hakka-code --addr ws://127.0.0.1:8765/ws
```

## Commands

Inside the REPL:

```text
/help                    Show help
/exit                    Exit
/cancel                  Cancel current turn
/session                 Show current session info
/session list            List sessions
/session create          Create/switch to fresh session
/session switch <id>     Fetch/switch to session by id or prefix
/session rename <name>   Rename current session
/session delete <id>     Delete session
/model list              List models
/model <name>            Switch model
/tool list               List tools
/tool allow <name|#tag>  Enable tool or tag
/tool deny <name|#tag>   Disable tool or tag
/cwd <path>              Set session cwd
/compact [n]             Show or set compaction limit
```

## Configuration

Optional `~/.hakka-code.json` (CLI flags always override):

```json
{
  "addr": "ws://127.0.0.1:8765/ws",
  "enable_tags": "#all"
}
```

## Next steps

Likely next iterations:

1. Replace the REPL with a Bubble Tea TUI for true incremental rendering (today, assistant text is rendered as markdown only once the turn completes) and to allow typing ahead while a turn streams.
2. Session switch should reload/display history nicely.
3. Add collapsible tool output.
