# hakka-code

A standalone terminal frontend for the [Hakka](https://github.com/ariloulaleelay/hakka) agent backend, inspired by the ergonomics of Claude Code.

Talks to a running Hakka instance over the WebSocket v2 protocol.

## Status

Active development, protocol v2. A full-screen Bubble Tea TUI.

Current features:

- Connects to Hakka's v2 WebSocket protocol
- Full-screen TUI: scrollable transcript viewport (PgUp/PgDown, mouse wheel) with a multi-line input box (up to 6 lines, auto-growing)
- Resumes the most recently updated non-empty session on startup and replays its history, or creates a fresh one
- Sets session cwd to the current directory
- Enables tools on startup (`#all` by default, configurable)
- Markdown-rendered assistant replies via glamour, with custom heading/table/code-block/hr handling
- Syntax-highlighted code blocks (chroma) with uniform background, adapting to terminal width
- Diff-style rendering for `edit_file`/`write_file` tool calls
- Auto-renames a session (via LLM) after its first exchange
- Per-turn statusline: model, token/context usage, cost
- Animated spinner while waiting on the LLM or a running tool
- Ctrl+C cancels the in-flight turn (not the whole program); a second Ctrl+C at the idle prompt exits
- You can keep typing while a turn is in flight — Enter is deferred until it finishes
- Prompt history browsable with Up/Down on empty input
- Tool calls collapse to a single confirmation line on success; expand on click for full detail (diff/preview, error)
- Tool result lines show rich context: line range for `read_file`, diff stats `(+N -M)` for `edit_file`, byte count for `write_file`, and wall-clock duration for all tools
- Native mouse-driven text selection with clipboard copy (OSC 52)
- Clickable rows in `/session list`, `/model list`, `/tool list` for quick session/model switching and tool toggling
- All list commands render as human-readable tables with local timestamps
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
/exit, /quit, /q         Exit
/cancel                  Cancel current turn
/clear                   Clear the screen
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

Keys:

```text
Up/Down (empty input)    Browse prompt history
PgUp/PgDown              Scroll the transcript by a page
Mouse wheel              Scroll the transcript
Mouse drag               Select text to clipboard
```

## Configuration

Optional `~/.hakka-code.json` (CLI flags always override):

```json
{
  "addr": "ws://127.0.0.1:8765/ws",
  "enable_tags": "#all"
}
```

## Architecture

```
cmd/hakka-code/main.go           — Entry point, composition root
internal/hakkacode/
├── backend/                     — WebSocket transport (interface + client)
├── protocol/                    — Wire types, constants, frame definitions
├── transcript/                  — Scrollback data model with hit-testing,
│                                  expand/collapse, and text selection
├── tui.go              (~400L)  — Bubble Tea model, Update/View, frame dispatch
├── tui_cmds.go         (~150L)  — Tea.Cmd factories (boot, wait, cancel, etc.)
├── tui_mouse.go        (~130L)  — Mouse handling and click action dispatch
├── tui_transcript.go   (~90L)   — Transcript appenders
├── turn.go             (~270L)  — In-flight turn state machine + tool info rendering
├── input.go            (~180L)  — Input widget with history and dynamic resizing
├── spinner.go          (~80L)   — Spinner widget with duration formatting
├── render.go           (~280L)  — Pure render helpers, tool event rendering
├── render_format.go    (~230L)  — Table formatters for command results
├── render_interactive.go(~170L) — Clickable list rendering
├── markdown.go         (~110L)  — Markdown orchestrator (glamour)
├── markdown_blocks.go  (~100L)  — Fenced code / table block splitter
├── markdown_code.go    (~130L)  — Code block syntax highlighting (chroma)
├── markdown_table.go   (~80L)   — Pipe table rendering (lipgloss)
├── markdown_inline.go  (~40L)   — Bold/italic/code for table cells
├── commands.go         (~160L)  — Slash command parser and registry
├── config.go           (~40L)   — ~/.hakka-code.json loader
├── styles.go           (~40L)   — ANSI SGR helpers
└── history_replay.go   (~60L)   — Event → ResponseFrame adapter
```

## Next steps

1. Incremental markdown rendering — assistant text is rendered as markdown only once the turn completes, not delta-by-delta.
2. Support `write_file` / `edit_file` diffs in the expandable tool detail view (currently only shown on error).
3. Pasting a very long single-line query into the input box can leave it scrolled to the wrong position until the next keystroke.
4. Use server-reported tool timing (if/when added to the protocol) instead of client-side wall-clock measurement.
