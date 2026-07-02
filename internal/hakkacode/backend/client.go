package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"hakka-code/internal/hakkacode/protocol"

	"github.com/gorilla/websocket"
)

// Client is the WebSocket transport implementation of Backend.
type Client struct {
	addr string
	conn *websocket.Conn
	mu   sync.Mutex

	// A websocket connection supports only one concurrent reader, so a
	// single background pump goroutine owns conn.ReadJSON for the
	// connection's whole lifetime and fans frames out through this
	// channel. Read(ctx) just drains it — safe to call from anywhere
	// (a command waiter, a chat-turn loop, a TUI event loop) as long as
	// only one caller drains at a time, which matches how this client is
	// actually used (one action in flight at a time).
	frames  chan protocol.ResponseFrame
	readErr chan error
}

// Dial connects to the Hakka WebSocket server and returns a ready-to-use
// Client that satisfies Backend.
func Dial(ctx context.Context, addr string) (*Client, error) {
	dialer := websocket.DefaultDialer
	conn, resp, err := dialer.DialContext(ctx, addr, nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("connect %s: %w; http status: %s", addr, err, resp.Status)
		}
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return nil, fmt.Errorf("connect %s: unexpected http status %s", addr, resp.Status)
	}
	c := &Client{
		addr:    addr,
		conn:    conn,
		frames:  make(chan protocol.ResponseFrame, 32),
		readErr: make(chan error, 1),
	}
	go c.pump()
	return c, nil
}

// pump is the connection's sole reader for its entire lifetime.
func (c *Client) pump() {
	for {
		var frame protocol.ResponseFrame
		if err := c.conn.ReadJSON(&frame); err != nil {
			c.readErr <- err
			close(c.frames)
			return
		}
		c.frames <- frame
	}
}

// Close shuts down the WebSocket connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Send writes an arbitrary frame to the server.
func (c *Client) Send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return c.conn.WriteJSON(v)
}

// Read returns the next frame from the connection, or an error if ctx is
// done or the connection's read pump has stopped (closed/broken
// connection).
func (c *Client) Read(ctx context.Context) (protocol.ResponseFrame, error) {
	select {
	case <-ctx.Done():
		return protocol.ResponseFrame{}, ctx.Err()
	case frame, ok := <-c.frames:
		if !ok {
			select {
			case err := <-c.readErr:
				return protocol.ResponseFrame{}, err
			default:
				return protocol.ResponseFrame{}, io.EOF
			}
		}
		return frame, nil
	}
}

// ExecuteCommand sends a structured "cmd" request and waits for its
// matching response frame.
//
// session_create / get_session complete via a "type":"session" frame
// (Event == cmd); everything else completes via "type":"result"
// (Cmd == cmd) or "type":"done" (error / fallback ack). Any other frame
// received while waiting (e.g. events fanned out from a reconnected
// in-flight turn, or unrelated "session" frames from the welcome
// auto-subscribe) is skipped.
func (c *Client) ExecuteCommand(ctx context.Context, sessionID string, cmd string, params map[string]any) (protocol.ResponseFrame, error) {
	req := protocol.RequestFrame{
		Type:      "cmd",
		SessionID: sessionID,
		Command: &protocol.Command{
			Cmd:    cmd,
			Params: params,
		},
	}
	if err := c.Send(req); err != nil {
		return protocol.ResponseFrame{}, err
	}

	for {
		frame, err := c.Read(ctx)
		if err != nil {
			return protocol.ResponseFrame{}, err
		}

		if frame.Type == "req" {
			_ = c.ReplyUnsupportedClientRequest(frame)
			continue
		}

		switch frame.Type {
		case "error":
			return frame, nil
		case "session":
			if cmd == "session_create" && frame.Event == cmd {
				return frame, nil
			}
			if cmd == "get_session" && frame.Event == cmd {
				reqID, _ := params["id"].(string)
				if sessionIDMatches(reqID, frame.Session) {
					return frame, nil
				}
				// Otherwise this is an unrelated "session" frame fanned out
				// by the server's own welcome auto-subscribe — keep waiting
				// for the response to our specific request.
			}
		case "result":
			if frame.Cmd == cmd {
				return frame, nil
			}
		case "done":
			return frame, nil
		}
		// Anything else (delta/tool/usage/unrelated session/welcome) is
		// noise from a concurrently fanned-out turn — skip it.
	}
}

// sessionIDMatches reports whether a returned session map plausibly
// corresponds to the requested id/prefix. An empty requested id always
// matches (caller has no specific target).
func sessionIDMatches(requested string, got map[string]any) bool {
	if requested == "" {
		return true
	}
	if got == nil {
		return false
	}
	id, _ := got["id"].(string)
	short, _ := got["short_id"].(string)
	return requested == id || requested == short ||
		strings.HasPrefix(id, requested) || strings.HasPrefix(requested, id)
}

// ListSessions returns the session metadata maps for the current
// namespace, as reported by "session_list".
func (c *Client) ListSessions(ctx context.Context) ([]map[string]any, error) {
	frame, err := c.ExecuteCommand(ctx, "", "session_list", nil)
	if err != nil {
		return nil, err
	}
	if frame.Error != "" {
		return nil, errors.New(frame.Error)
	}
	raw, _ := frame.Data["sessions"].([]any)
	sessions := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			sessions = append(sessions, m)
		}
	}
	return sessions, nil
}

// MostRecentSession returns the most recently updated non-empty session,
// or nil if there is none.
func (c *Client) MostRecentSession(ctx context.Context) (map[string]any, error) {
	sessions, err := c.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	sort.Slice(sessions, func(i, j int) bool {
		ui, _ := sessions[i]["updated_at"].(string)
		uj, _ := sessions[j]["updated_at"].(string)
		return ui > uj
	})
	return sessions[0], nil
}

// GetSession fetches a session by id or unique prefix, along with its
// stored message history.
func (c *Client) GetSession(ctx context.Context, id string) (*protocol.SessionSummary, string, []map[string]any, error) {
	frame, err := c.ExecuteCommand(ctx, "", "get_session", map[string]any{"id": id})
	if err != nil {
		return nil, "", nil, err
	}
	if frame.Error != "" {
		return nil, "", nil, errors.New(frame.Error)
	}
	sessionID := frame.SessionID
	var summary *protocol.SessionSummary
	if frame.Session != nil {
		summary = protocol.SessionSummaryFromMap(frame.Session)
		if summary.ID != "" {
			sessionID = summary.ID
		}
	}
	if sessionID == "" {
		return nil, "", nil, fmt.Errorf("get_session returned no session id")
	}
	return summary, sessionID, frame.Messages, nil
}

// CreateSession creates a new session on the server.
func (c *Client) CreateSession(ctx context.Context) (*protocol.SessionSummary, string, error) {
	frame, err := c.ExecuteCommand(ctx, "", "session_create", nil)
	if err != nil {
		return nil, "", err
	}
	if frame.Error != "" {
		return nil, "", errors.New(frame.Error)
	}

	sessionID := frame.SessionID
	var summary *protocol.SessionSummary
	if frame.Session != nil {
		summary = protocol.SessionSummaryFromMap(frame.Session)
		if summary.ID != "" {
			sessionID = summary.ID
		}
	}
	if sessionID == "" {
		return nil, "", fmt.Errorf("session_create returned no session id")
	}
	return summary, sessionID, nil
}

// SendInput sends a chat message to the given session.
func (c *Client) SendInput(sessionID string, input string) error {
	return c.Send(protocol.RequestFrame{
		Type:      "chat",
		SessionID: sessionID,
		Input:     input,
		Stream:    true,
	})
}

// Cancel requests cancellation of the in-flight turn for the given session.
func (c *Client) Cancel(sessionID string) error {
	return c.Send(protocol.RequestFrame{
		Type:      "cancel",
		SessionID: sessionID,
	})
}

// ReplyUnsupportedClientRequest tells the server we don't handle a
// client-request frame (e.g., tool calls proxied from the server).
func (c *Client) ReplyUnsupportedClientRequest(frame protocol.ResponseFrame) error {
	if frame.RequestID == "" {
		return nil
	}
	return c.Send(protocol.RequestFrame{
		Type:      "resp",
		RequestID: frame.RequestID,
		Error:     "unsupported by hakka-code MVP",
	})
}
