// Package backend defines the transport abstraction for communicating with
// the Hakka agent over its WebSocket protocol.
package backend

import (
	"context"

	"hakka-code/internal/hakkacode/protocol"
)

// Backend is the interface through which the TUI talks to the Hakka
// server. It's separated from the WebSocket implementation so the UI
// can be tested with a mock and swapped to a different transport later.
type Backend interface {
	// Send writes an arbitrary frame to the server.
	Send(v any) error

	// Read returns the next frame from the connection, blocking until one
	// arrives or ctx is done.
	Read(ctx context.Context) (protocol.ResponseFrame, error)

	// ExecuteCommand sends a structured "cmd" request and waits for its
	// matching response frame.
	ExecuteCommand(ctx context.Context, sessionID, cmd string, params map[string]any) (protocol.ResponseFrame, error)

	// SendInput sends a chat message to the given session.
	SendInput(sessionID, input string) error

	// Cancel requests cancellation of the in-flight turn for the given session.
	Cancel(sessionID string) error

	// ListSessions returns metadata for all sessions in the current namespace.
	ListSessions(ctx context.Context) ([]map[string]any, error)

	// MostRecentSession returns the most recently updated non-empty session,
	// or nil if there is none.
	MostRecentSession(ctx context.Context) (map[string]any, error)

	// GetSession fetches a session by id or unique prefix, along with its
	// stored message history.
	GetSession(ctx context.Context, id string) (*protocol.SessionSummary, string, []map[string]any, error)

	// CreateSession creates a new session.
	CreateSession(ctx context.Context) (*protocol.SessionSummary, string, error)

	// ReplyUnsupportedClientRequest tells the server we don't handle a
	// client-request frame (e.g., tool calls proxied from the server).
	ReplyUnsupportedClientRequest(frame protocol.ResponseFrame) error

	// Close shuts down the connection.
	Close() error
}
