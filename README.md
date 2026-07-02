# hakka-code

A standalone terminal frontend for the [Hakka](https://github.com/ariloulaleelay/hakka) agent backend, inspired by the ergonomics of Claude Code.

This repository is intentionally separate from the upstream Hakka backend for now. It talks to a running Hakka instance over the WebSocket protocol.

## Status

Early MVP.

Current features:

- Connects to Hakka WebSocket gateway
- Creates a fresh session on startup
- Sets session cwd to the current directory
- Enables all tools with `#all`
- Simple REPL chat loop
- Streams assistant output
- Renders basic tool lifecycle events
- Client-side slash command parsing

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

## Next steps

Likely next iterations:

1. Better command-result rendering instead of raw JSON.
2. Session switch should reload/display history nicely.
3. Ctrl+C/Esc cancellation while a turn is streaming.
4. Replace the REPL with a Bubble Tea TUI.
5. Add markdown/code-block rendering.
6. Add collapsible tool output.
