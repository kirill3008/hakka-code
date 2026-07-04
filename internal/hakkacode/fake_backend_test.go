package hakkacode

import (
	"context"

	"hakka-code/internal/hakkacode/protocol"
)

// fakeBackend is a minimal in-memory backend.Backend for tests that don't
// need a real WebSocket connection — just enough plumbing for handleFrame
// and boot/resume logic to exercise their own decisions.
type fakeBackend struct {
	sessions        []map[string]any
	getSessionByID  map[string]getSessionResult
	createdSummary  *protocol.SessionSummary
	createdID       string
	createErr       error
	sentUnsupported []protocol.ResponseFrame
}

type getSessionResult struct {
	summary  *protocol.SessionSummary
	id       string
	messages []map[string]any
	events   []map[string]any
	err      error
}

func (f *fakeBackend) Send(v any) error                                  { return nil }
func (f *fakeBackend) Read(ctx context.Context) (protocol.ResponseFrame, error) {
	return protocol.ResponseFrame{}, context.Canceled
}
func (f *fakeBackend) Close() error { return nil }

func (f *fakeBackend) ListSessions(ctx context.Context) ([]map[string]any, error) {
	return f.sessions, nil
}

func (f *fakeBackend) MostRecentSession(ctx context.Context) (map[string]any, error) {
	if len(f.sessions) == 0 {
		return nil, nil
	}
	return f.sessions[0], nil
}

func (f *fakeBackend) GetSession(ctx context.Context, id string) (*protocol.SessionSummary, string, []map[string]any, []map[string]any, error) {
	r, ok := f.getSessionByID[id]
	if !ok {
		return nil, "", nil, nil, nil
	}
	return r.summary, r.id, r.messages, r.events, r.err
}

func (f *fakeBackend) CreateSession(ctx context.Context) (*protocol.SessionSummary, string, error) {
	if f.createErr != nil {
		return nil, "", f.createErr
	}
	return f.createdSummary, f.createdID, nil
}

func (f *fakeBackend) SendInput(sessionID, input string) error { return nil }
func (f *fakeBackend) Cancel(sessionID string) error            { return nil }

func (f *fakeBackend) ExecuteCommand(ctx context.Context, sessionID, cmd string, params map[string]any) (protocol.ResponseFrame, error) {
	return protocol.ResponseFrame{}, nil
}

func (f *fakeBackend) ReplyUnsupportedClientRequest(frame protocol.ResponseFrame) error {
	f.sentUnsupported = append(f.sentUnsupported, frame)
	return nil
}

func newTestModel(client *fakeBackend) model {
	return newModel(context.Background(), Config{}, client)
}
