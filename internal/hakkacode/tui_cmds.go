package hakkacode

import (
	"context"
	"fmt"

	"hakka-code/internal/hakkacode/backend"
	"hakka-code/internal/hakkacode/protocol"

	tea "github.com/charmbracelet/bubbletea"
)

func bootCmd(ctx context.Context, client backend.Backend, cfg Config) tea.Cmd {
	return func() tea.Msg {
		if _, err := client.Read(ctx); err != nil {
			return bootMsg{err: fmt.Errorf("read welcome: %w", err)}
		}

		summary, sessionID, events, resumed, err := resumeOrCreateSession(ctx, client, cfg.CWD)
		if err != nil {
			return bootMsg{err: fmt.Errorf("resume/create session: %w", err)}
		}

		msg := bootMsg{summary: summary, sessionID: sessionID, events: events, resumed: resumed}

		if cfg.CWD != "" {
			if _, err := client.ExecuteCommand(ctx, sessionID, "cwd_set", map[string]any{"cwd": cfg.CWD}); err != nil {
				msg.cwdWarn = err.Error()
			}
		}

		tags := cfg.EnableTags
		if tags == "" {
			tags = "#all"
		}
		msg.tags = tags
		if _, err := client.ExecuteCommand(ctx, sessionID, "tool_allow", map[string]any{"name": tags}); err != nil {
			msg.toolWarn = err.Error()
		}

		return msg
	}
}

// resumeOrCreateSession resumes the most recently updated session whose
// stored cwd matches the current process's cwd. Sessions from other
// directories are left alone — so switching project directories resumes
// that directory's own history instead of dumping you into an unrelated
// session — and a fresh session is created if none match.
func resumeOrCreateSession(ctx context.Context, client backend.Backend, cwd string) (*protocol.SessionSummary, string, []map[string]any, bool, error) {
	recent, err := mostRecentSessionForCWD(ctx, client, cwd)
	if err != nil {
		return nil, "", nil, false, err
	}
	if recent == nil {
		summary, sessionID, err := client.CreateSession(ctx)
		return summary, sessionID, nil, false, err
	}
	id, _ := recent["id"].(string)
	summary, sessionID, _, events, err := client.GetSession(ctx, id)
	if err != nil {
		return nil, "", nil, false, err
	}
	return summary, sessionID, events, true, nil
}

// mostRecentSessionForCWD returns the newest session (by updated_at) whose
// stored client_cwd matches cwd, or nil if none match. If cwd is unknown,
// it falls back to the newest session overall rather than discarding data.
func mostRecentSessionForCWD(ctx context.Context, client backend.Backend, cwd string) (map[string]any, error) {
	if cwd == "" {
		return client.MostRecentSession(ctx)
	}
	sessions, err := client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	var best map[string]any
	for _, s := range sessions {
		sessionCWD, _ := s["client_cwd"].(string)
		if sessionCWD != cwd {
			continue
		}
		if best == nil || updatedAt(s) > updatedAt(best) {
			best = s
		}
	}
	return best, nil
}

func updatedAt(session map[string]any) string {
	s, _ := session["updated_at"].(string)
	return s
}

func waitFrame(ctx context.Context, client backend.Backend) tea.Cmd {
	return func() tea.Msg {
		frame, err := client.Read(ctx)
		if err != nil {
			return readErrMsg{err}
		}
		return frameMsg(frame)
	}
}

func execRemoteCmd(ctx context.Context, client backend.Backend, sessionID, cmd string, params map[string]any) tea.Cmd {
	return func() tea.Msg {
		frame, err := client.ExecuteCommand(ctx, sessionID, cmd, params)
		return cmdResultMsg{cmd: cmd, frame: frame, err: err}
	}
}

func autoRenameCmd(ctx context.Context, client backend.Backend, sessionID string) tea.Cmd {
	return func() tea.Msg {
		frame, err := client.ExecuteCommand(ctx, sessionID, "session_autorename", nil)
		if err != nil || frame.Error != "" {
			return autoRenameMsg{}
		}
		session, _ := frame.Data["session"].(map[string]any)
		name, _ := session["name"].(string)
		return autoRenameMsg{name: name}
	}
}

func cancelTurnCmd(ctx context.Context, client backend.Backend, sessionID string) tea.Cmd {
	return func() tea.Msg {
		_ = client.Cancel(sessionID)
		// Drain all frames until the server confirms cancellation with a
		// "done" frame. Without this, stale frames (deltas, tool results,
		// and especially the done frame itself) from the cancelled turn
		// remain in the read channel and poison the next turn's frame
		// stream, causing it to end immediately.
		for {
			frame, err := client.Read(ctx)
			if err != nil {
				return readErrMsg{err}
			}
			if frame.Type == protocol.TypeDone {
				return frameMsg(frame)
			}
		}
	}
}
