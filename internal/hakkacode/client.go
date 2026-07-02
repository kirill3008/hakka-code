package hakkacode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	addr string
	conn *websocket.Conn
	mu   sync.Mutex
}

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
	return &Client{addr: addr, conn: conn}, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	return c.conn.WriteJSON(v)
}

func (c *Client) Read(ctx context.Context) (ResponseFrame, error) {
	type result struct {
		frame ResponseFrame
		err   error
	}

	ch := make(chan result, 1)
	go func() {
		var frame ResponseFrame
		err := c.conn.ReadJSON(&frame)
		ch <- result{frame: frame, err: err}
	}()

	select {
	case <-ctx.Done():
		return ResponseFrame{}, ctx.Err()
	case res := <-ch:
		return res.frame, res.err
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
func (c *Client) ExecuteCommand(ctx context.Context, sessionID string, cmd string, params map[string]any) (ResponseFrame, error) {
	req := RequestFrame{
		Type:      "cmd",
		SessionID: sessionID,
		Command: &Command{
			Cmd:    cmd,
			Params: params,
		},
	}
	if err := c.Send(req); err != nil {
		return ResponseFrame{}, err
	}

	for {
		frame, err := c.Read(ctx)
		if err != nil {
			return ResponseFrame{}, err
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

// GetSession fetches a session by id or unique prefix.
func (c *Client) GetSession(ctx context.Context, id string) (*SessionSummary, string, error) {
	frame, err := c.ExecuteCommand(ctx, "", "get_session", map[string]any{"id": id})
	if err != nil {
		return nil, "", err
	}
	if frame.Error != "" {
		return nil, "", errors.New(frame.Error)
	}
	sessionID := frame.SessionID
	var summary *SessionSummary
	if frame.Session != nil {
		summary = sessionSummaryFromMap(frame.Session)
		if summary.ID != "" {
			sessionID = summary.ID
		}
	}
	if sessionID == "" {
		return nil, "", fmt.Errorf("get_session returned no session id")
	}
	return summary, sessionID, nil
}

func (c *Client) CreateSession(ctx context.Context) (*SessionSummary, string, error) {
	frame, err := c.ExecuteCommand(ctx, "", "session_create", nil)
	if err != nil {
		return nil, "", err
	}
	if frame.Error != "" {
		return nil, "", errors.New(frame.Error)
	}

	sessionID := frame.SessionID
	var summary *SessionSummary
	if frame.Session != nil {
		summary = sessionSummaryFromMap(frame.Session)
		if summary.ID != "" {
			sessionID = summary.ID
		}
	}
	if sessionID == "" {
		return nil, "", fmt.Errorf("session_create returned no session id")
	}
	return summary, sessionID, nil
}

func (c *Client) SendInput(sessionID string, input string) error {
	return c.Send(RequestFrame{
		Type:      "chat",
		SessionID: sessionID,
		Input:     input,
		Stream:    true,
	})
}

func (c *Client) Cancel(sessionID string) error {
	return c.Send(RequestFrame{
		Type:      "cancel",
		SessionID: sessionID,
	})
}

func (c *Client) ReplyUnsupportedClientRequest(frame ResponseFrame) error {
	if frame.RequestID == "" {
		return nil
	}
	return c.Send(RequestFrame{
		Type:      "resp",
		RequestID: frame.RequestID,
		Error:     "unsupported by hakka-code MVP",
	})
}
