package hakkacode

import (
	"context"
	"fmt"
	"net/http"
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
			if (cmd == "session_create" || cmd == "get_session") && frame.Event == cmd {
				return frame, nil
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

func (c *Client) CreateSession(ctx context.Context) (*SessionSummary, string, error) {
	frame, err := c.ExecuteCommand(ctx, "", "session_create", nil)
	if err != nil {
		return nil, "", err
	}
	if frame.Error != "" {
		return nil, "", fmt.Errorf(frame.Error)
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
