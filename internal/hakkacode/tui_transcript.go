package hakkacode

import (
	"encoding/json"
	"fmt"
	"strings"

	"hakka-code/internal/hakkacode/transcript"
)

func (m *model) appendLine(text string) {
	m.appendEntry(&transcript.TranscriptEntry{
		Type: transcript.EntrySystem,
		Raw:  text,
	})
}

func (m *model) appendEntry(entry *transcript.TranscriptEntry) {
	stick := !m.ready || m.viewport.AtBottom()

	if entry.Rendered == nil {
		entry.Rendered = strings.Split(strings.TrimRight(entry.Raw, "\n"), "\n")
		if len(entry.Rendered) == 1 && entry.Rendered[0] == "" {
			entry.Rendered = []string{""}
		}
	}

	m.transcriptEntries.Append(entry)

	if m.ready {
		m.viewport.SetContent(m.transcriptEntries.String())
		if stick {
			m.viewport.GotoBottom()
		}
	}
}

func (m *model) appendUserPrompt(rawLine string) {
	m.appendEntry(&transcript.TranscriptEntry{
		Type: transcript.EntryUserPrompt,
		Raw:  strings.TrimRight(renderUserPrompt("❯ "+rawLine), "\n"),
	})
}

func (m *model) appendAssistantText(raw string) {
	m.appendEntry(&transcript.TranscriptEntry{
		Type: transcript.EntryAssistantText,
		Raw:  strings.TrimRight(renderMarkdown(raw), "\n") + "\n",
	})
}

func (m *model) appendToolCall(name string, id string, status transcript.ToolStatus, args json.RawMessage, snippet, errText string) {
	snippet = sanitizeSnippet(snippet)
	entry := &transcript.TranscriptEntry{
		Type:       transcript.EntryToolCall,
		ToolName:   name,
		ToolID:     id,
		ToolStatus: status,
		ToolArgs:   args,
		ToolError:  errText,
		Collapsed:  status == transcript.ToolOK,
	}
	if status == transcript.ToolOK {
		if snippet != "" {
			entry.Raw = fmt.Sprintf("✓ %s · %s", name, snippet)
		} else {
			entry.Raw = fmt.Sprintf("✓ %s", name)
		}
	} else {
		entry.Raw = fmt.Sprintf("✗ %s: %s", name, errText)
	}
	m.appendEntry(entry)
}

func sanitizeSnippet(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	return s
}

func (m *model) appendStatusLine(raw string) {
	m.appendEntry(&transcript.TranscriptEntry{
		Type: transcript.EntryStatusLine,
		Raw:  strings.TrimRight(raw, "\n"),
	})
}

func (m *model) appendError(raw string) {
	m.appendEntry(&transcript.TranscriptEntry{
		Type: transcript.EntryError,
		Raw:  raw,
	})
}

func (m *model) appendCommandResult(res renderedResult) {
	entry := &transcript.TranscriptEntry{
		Type:         transcript.EntryCommandResult,
		Raw:          strings.TrimRight(res.text, "\n"),
		ClickRegions: res.regions,
	}
	m.appendEntry(entry)
}

func (m *model) rebuildViewport() {
	content := m.transcriptEntries.Rebuild(m.entryRenderer, m.width)
	if m.ready {
		m.viewport.SetContent(content)
	}
}

func (m *model) entryRenderer(entry *transcript.TranscriptEntry, width int) []string {
	if entry.Rendered != nil && !m.transcriptEntries.IsDirty() {
		return entry.Rendered
	}
	return strings.Split(strings.TrimRight(entry.Raw, "\n"), "\n")
}

func (m *model) viewportContent() string {
	return m.transcriptEntries.String()
}
