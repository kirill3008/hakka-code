package backend

import (
	"context"

	"hakka-code/internal/hakkacode/protocol"
)

// Connector is the raw WebSocket transport: send a frame, read the next
// frame, or tear down the connection.
type Connector interface {
	Send(v any) error
	Read(ctx context.Context) (protocol.ResponseFrame, error)
	Close() error
}

// SessionStore manages session lifecycle on the server.
type SessionStore interface {
	ListSessions(ctx context.Context) ([]map[string]any, error)
	MostRecentSession(ctx context.Context) (map[string]any, error)
	GetSession(ctx context.Context, id string) (*protocol.SessionSummary, string, []map[string]any, []map[string]any, error)
	CreateSession(ctx context.Context) (*protocol.SessionSummary, string, error)
}

// Chat carries chat-turn operations: send a prompt or cancel an in-flight
// turn.
type Chat interface {
	SendInput(sessionID, input string) error
	Cancel(sessionID string) error
}

// Commander sends structured commands and handles server-initiated client
// requests.
type Commander interface {
	ExecuteCommand(ctx context.Context, sessionID, cmd string, params map[string]any) (protocol.ResponseFrame, error)
	ReplyUnsupportedClientRequest(frame protocol.ResponseFrame) error
}

// Backend composes all transport interfaces into the full client surface
// that the TUI depends on.
type Backend interface {
	Connector
	SessionStore
	Chat
	Commander
}
