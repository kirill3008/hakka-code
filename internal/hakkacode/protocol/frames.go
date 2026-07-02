// Package protocol defines the Hakka v2 wire protocol types and constants.
//
// Every frame (inbound and outbound) has a mandatory "type" field that acts
// as the sole discriminant.
//
// LLM content is always carried in a field called "text".
package protocol

import "encoding/json"

// Command is a structured command sent by a JSON-capable client.
type Command struct {
	Cmd    string         `json:"cmd"`
	Params map[string]any `json:"params,omitempty"`
}

// RequestFrame is the outbound (client -> server) envelope.
type RequestFrame struct {
	Type      string   `json:"type"`
	SessionID string   `json:"session_id,omitempty"`
	Input     string   `json:"input,omitempty"`  // for "chat"
	Stream    bool     `json:"stream,omitempty"` // for "chat"
	Command   *Command `json:"command,omitempty"` // for "cmd"
	RequestID string   `json:"request_id,omitempty"` // for "resp"
	Result    any      `json:"result,omitempty"`     // for "resp"
	Error     string   `json:"error,omitempty"`      // for "resp"
}

// TurnStats is the end-of-turn statistics embedded in a "done" frame.
type TurnStats struct {
	TotalTokens            int     `json:"total_tokens"`
	TotalCost              float64 `json:"total_cost"`
	MessageCount           int     `json:"message_count"`
	EstimatedContextTokens int     `json:"estimated_context_tokens"`
	Model                  string  `json:"model"`
}

// ResponseFrame is the inbound (server -> client) envelope.
type ResponseFrame struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`

	// --- "delta" / "done" fields ---
	Text string `json:"text,omitempty"`

	// --- "done" fields ---
	Error     string     `json:"error,omitempty"`
	Cancelled bool       `json:"cancelled,omitempty"`
	Stats     *TurnStats `json:"stats,omitempty"`

	// --- "tool" fields ---
	Tool       string          `json:"tool,omitempty"`
	ID         string          `json:"id,omitempty"`
	Status     string          `json:"status,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Snippet    string          `json:"snippet,omitempty"`
	ToolResult string          `json:"result,omitempty"`

	// --- "usage" fields ---
	PromptTokens     *int     `json:"prompt_tokens,omitempty"`
	CompletionTokens *int     `json:"completion_tokens,omitempty"`
	TotalTokens      *int     `json:"total_tokens,omitempty"`
	DurationNS       *int64   `json:"duration_ns,omitempty"`
	Cost             *float64 `json:"cost,omitempty"`
	TotalCost        *float64 `json:"total_cost,omitempty"`
	EstimatedTokens  *int     `json:"estimated_context_tokens,omitempty"`

	// --- "req" fields (flat) ---
	RequestID string `json:"request_id,omitempty"`
	Command   string `json:"command,omitempty"`

	// --- "result" fields ---
	Cmd  string         `json:"cmd,omitempty"`
	Data map[string]any `json:"data,omitempty"`

	// --- "welcome" / "session" fields ---
	Sessions []map[string]any `json:"sessions,omitempty"`
	Session  map[string]any   `json:"session,omitempty"`
	Messages []map[string]any `json:"messages,omitempty"`
	Events   []map[string]any `json:"events,omitempty"`

	// --- "session" lifecycle fields ---
	Event   string `json:"event,omitempty"` // "session_create", "get_session", "renamed", "deleted"
	OldName string `json:"old_name,omitempty"`
	Name    string `json:"name,omitempty"`
}

// SessionSummary is a convenience view over the loosely-typed session map
// returned by the server (agent.Session.Metadata()).
type SessionSummary struct {
	ID                     string
	ShortID                string
	Name                   string
	MessageCount           int
	Model                  string
	TotalTokens            int
	EstimatedContextTokens int
}

// SessionSummaryFromMap extracts a SessionSummary from a loosely-typed map.
func SessionSummaryFromMap(m map[string]any) *SessionSummary {
	if m == nil {
		return nil
	}
	s := &SessionSummary{}
	s.ID, _ = m["id"].(string)
	s.ShortID, _ = m["short_id"].(string)
	s.Name, _ = m["name"].(string)
	s.Model, _ = m["model"].(string)
	s.MessageCount = intFromAny(m["message_count"])
	s.TotalTokens = intFromAny(m["total_tokens"])
	s.EstimatedContextTokens = intFromAny(m["estimated_context_tokens"])
	return s
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// Frame type constants.
const (
	TypeChat    = "chat"
	TypeCmd     = "cmd"
	TypeResp    = "resp"
	TypeCancel  = "cancel"
	TypeWelcome = "welcome"
	TypeDelta   = "delta"
	TypeDone    = "done"
	TypeTool    = "tool"
	TypeUsage   = "usage"
	TypeReq     = "req"
	TypeResult  = "result"
	TypeSession = "session"
	TypeError   = "error"
)

// Tool status constants.
const (
	StatusStart = "start"
	StatusOK    = "ok"
	StatusErr   = "err"
)

// Command name constants.
const (
	CmdSessionList       = "session_list"
	CmdSessionInfo       = "session_info"
	CmdSessionCreate     = "session_create"
	CmdSessionRename     = "session_rename"
	CmdSessionAutoRename = "session_autorename"
	CmdSessionDelete     = "session_delete"
	CmdGetSession        = "get_session"
	CmdModelList         = "model_list"
	CmdModelSwitch       = "model_switch"
	CmdToolList          = "tool_list"
	CmdToolAllow         = "tool_allow"
	CmdToolDeny          = "tool_deny"
	CmdCWDSet            = "cwd_set"
	CmdCompact           = "compact"
)

// Click action constants.
const (
	ActionSessionSwitch = "session-switch"
	ActionModelSwitch   = "model-switch"
	ActionToolAllow     = "tool-allow"
	ActionToolDeny      = "tool-deny"
	ActionCopy          = "copy"
)
