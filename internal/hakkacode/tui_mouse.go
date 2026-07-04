package hakkacode

import (
	"context"

	"hakka-code/internal/hakkacode/backend"
	"hakka-code/internal/hakkacode/protocol"
	"hakka-code/internal/hakkacode/transcript"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.viewport.LineUp(3)
		m.selection.Clear()
		return m, nil

	case msg.Button == tea.MouseButtonWheelDown:
		m.viewport.LineDown(3)
		m.selection.Clear()
		return m, nil

	case msg.Action == tea.MouseActionRelease:
		return m.handleMouseRelease(msg)

	case msg.Action == tea.MouseActionMotion:
		return m.handleMouseDrag(msg)

	case msg.Action == tea.MouseActionPress:
		return m.handleMousePress(msg)
	}
	return m, nil
}

func (m model) handleMousePress(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	vpLine := m.viewport.YOffset + msg.Y

	entry, _ := m.transcriptEntries.EntryAtLine(vpLine)
	if entry != nil && entry.IsExpandable() {
		m.selection.Start(vpLine, msg.X)
		return m, nil
	}

	if entry != nil {
		if region := m.transcriptEntries.ClickRegionAt(vpLine, msg.X); region != nil {
			return m.dispatchClickAction(region.Action)
		}
	}

	m.selection.Start(vpLine, msg.X)
	return m, nil
}

func (m model) handleMouseDrag(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	vpLine := m.viewport.YOffset + msg.Y
	m.selection.Extend(vpLine, msg.X)
	return m, nil
}

func (m model) handleMouseRelease(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.selection.State != transcript.SelDragging {
		return m, nil
	}

	vpLine := m.viewport.YOffset + msg.Y
	m.selection.Extend(vpLine, msg.X)

	entry, _ := m.transcriptEntries.EntryAtLine(vpLine)
	sl, sc, el, ec := m.selection.Normalized()
	isClick := sl == el && sc == ec

	if entry != nil && entry.IsExpandable() && isClick {
		m.selection.Clear()
		if m.transcriptEntries.ToggleEntryAt(vpLine) {
			m.rebuildViewport()
		}
		return m, nil
	}

	if !isClick {
		content := m.viewportContent()
		text := m.selection.Text(content)
		if text != "" {
			m.selection.Finish()
			return m, func() tea.Msg { return copyToClipboardMsg{text: text} }
		}
	}

	m.selection.Clear()
	return m, nil
}

func (m model) dispatchClickAction(action transcript.ClickAction) (tea.Model, tea.Cmd) {
	switch action.Action {
	case protocol.ActionSessionSwitch:
		return m, execRemoteCmd(m.ctx, m.client, m.session.id, protocol.CmdGetSession, map[string]any{"id": action.Payload})
	case protocol.ActionModelSwitch:
		return m, execRemoteCmd(m.ctx, m.client, m.session.id, protocol.CmdModelSwitch, map[string]any{"name": action.Payload})
	case protocol.ActionToolAllow:
		return m, toolToggleThenRefresh(m.ctx, m.client, m.session.id, protocol.CmdToolAllow, action.Payload)
	case protocol.ActionToolDeny:
		return m, toolToggleThenRefresh(m.ctx, m.client, m.session.id, protocol.CmdToolDeny, action.Payload)
	case protocol.ActionCopy:
		return m, func() tea.Msg { return copyToClipboardMsg{text: action.Payload} }
	}
	return m, nil
}

func toolToggleThenRefresh(ctx context.Context, client backend.Backend, sessionID, toggleCmd, toolName string) tea.Cmd {
	return tea.Sequence(
		execRemoteCmd(ctx, client, sessionID, toggleCmd, map[string]any{"name": toolName}),
		execRemoteCmd(ctx, client, sessionID, protocol.CmdToolList, nil),
	)
}
