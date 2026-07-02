# hakka-code

A standalone terminal frontend for the [Hakka](https://github.com/ariloulaleelay/hakka) agent backend, inspired by the ergonomics of Claude Code.

This repository is intentionally separate from the upstream Hakka backend for now. It talks to a running Hakka instance over the WebSocket protocol.

## Status

Early MVP, protocol v2. A Bubble Tea TUI (previously a plain line-based REPL).

Current features:

- Connects to Hakka's v2 WebSocket protocol
- Full-screen TUI: scrollable transcript viewport (PgUp/PgDown, Up/Down, mouse wheel where the terminal maps wheel scroll to arrow keys) pinned above a persistent input box. No mouse mode, so the terminal's native text selection/copy still works
- Resumes the most recently updated non-empty session on startup and replays its history, or creates a fresh one
- Sets session cwd to the current directory
- Enables tools on startup (`#all` by default, configurable)
- Markdown-rendered assistant replies (via glamour, with custom heading/table/code-block/hr handling)
- Diff-style rendering for `edit_file`/`write_file` tool calls
- Auto-renames a session (via LLM) after its first exchange
- Per-turn statusline: model, token/context usage, cost
- Animated spinner while waiting on the LLM or a running tool, so a quiet turn doesn't look hung
- Ctrl+C cancels the in-flight turn (not the whole program); a second Ctrl+C at the idle prompt exits normally
- You can keep typing while a turn is in flight — Enter is deferred until it finishes
- Growing multi-line input box (up to 6 lines) instead of horizontal scrolling
- Ctrl+P/Ctrl+N browse prompt history
- Tool calls collapse to a single confirmation line on success; full detail (diff/preview, error) only shows on failure
- `/session list`, `/model list`, `/tool list`, etc. render as human-readable tables (with local timestamps) instead of raw JSON
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

Inside the TUI:

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

1. True incremental rendering — assistant text is still rendered as markdown only once the turn completes, not token-by-token.
2. Add collapsible tool output.
3. Wide tables currently overflow the terminal width rather than wrapping/scrolling — no clear best fix chosen yet.
4. Pasting a very long single-line query into the input box can leave it scrolled to the wrong position until the next keystroke.
5. Mouse wheel scroll now works on terminals that map wheel scroll to arrow keys without mouse mode enabled (Up/Down scroll the transcript by a line at the input box's edge) — but that's terminal-dependent, not a real mouse binding. A terminal that maps wheel to something else, or that doesn't do this synthesis at all, won't get wheel scroll. Properly supporting mouse wheel unconditionally, without losing native copy, means implementing our own mouse-driven text selection (own highlight rendering + clipboard write via OSC 52), like Claude Code's CLI reportedly does — a real feature, not a quick fix.
