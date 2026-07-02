package hakkacode

import (
	"context"
	"fmt"

	"hakka-code/internal/hakkacode/backend"

	tea "github.com/charmbracelet/bubbletea"
)

func bootCmd(ctx context.Context, client backend.Backend, cfg Config) tea.Cmd {
	return func() tea.Msg {
		if _, err := client.Read(ctx); err != nil {
			return bootMsg{err: fmt.Errorf("read welcome: %w", err)}
		}

		summary, sessionID, messages, resumed, err := resumeOrCreateSession(ctx, client)
		if err != nil {
			return bootMsg{err: fmt.Errorf("resume/create session: %w", err)}
		}

		msg := bootMsg{summary: summary, sessionID: sessionID, messages: messages, resumed: resumed}

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

func resumeOrCreateSession(ctx context.Context, client backend.Backend) (*SessionSummary, string, []map[string]any, bool, error) {
	recent, err := client.MostRecentSession(ctx)
	if err != nil {
		return nil, "", nil, false, err
	}
	if recent == nil {
		summary, sessionID, err := client.CreateSession(ctx)
		return summary, sessionID, nil, false, err
	}
	id, _ := recent["id"].(string)
	summary, sessionID, messages, err := client.GetSession(ctx, id)
	if err != nil {
		return nil, "", nil, false, err
	}
	return summary, sessionID, messages, true, nil
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

func cancelTurnCmd(client backend.Backend, sessionID string) tea.Cmd {
	return func() tea.Msg {
		_ = client.Cancel(sessionID)
		return nil
	}
}
