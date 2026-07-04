package hakkacode

import (
	"testing"

	"hakka-code/internal/hakkacode/protocol"
)

func TestHandleFrameIgnoresFrameForOtherSession(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.session.id = "session-a"
	m.session.turn = newTurnState()

	mdl, _ := m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeDelta,
		SessionID: "session-b",
		Text:      "hello from another session",
	})
	m = mdl.(model)

	if m.transcriptEntries.Len() != 0 {
		t.Fatalf("expected frame for a different session to be dropped, got %d entries", m.transcriptEntries.Len())
	}
	if m.session.turn.stream != nil {
		t.Fatal("expected turn state untouched by a foreign-session frame")
	}
}

func TestHandleFrameDoneWithNoActiveTurnDoesNotPanic(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.session.id = "session-a"
	m.session.turn = nil // no turn in flight — a stray/duplicate "done" should be a no-op

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleFrame panicked on done with nil turn: %v", r)
		}
	}()

	mdl, _ := m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeDone,
		SessionID: "session-a",
	})
	m = mdl.(model)

	if m.transcriptEntries.Len() != 0 {
		t.Fatalf("expected no transcript entries from a stray done frame, got %d", m.transcriptEntries.Len())
	}
}

func TestHandleFrameDeltaStartsStreamingEntry(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.session.id = "session-a"
	m.session.turn = newTurnState()

	mdl, _ := m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeDelta,
		SessionID: "session-a",
		Text:      "hello",
	})
	m = mdl.(model)

	if m.transcriptEntries.Len() != 1 {
		t.Fatalf("expected 1 transcript entry after first delta, got %d", m.transcriptEntries.Len())
	}
	if m.session.turn.stream == nil {
		t.Fatal("expected turn.stream to be set after a delta")
	}
}

func TestHandleFrameToolStartThenFinishAppendsToolCall(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.session.id = "session-a"
	m.session.turn = newTurnState()

	mdl, _ := m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeTool,
		SessionID: "session-a",
		ID:        "tool-1",
		Tool:      "read_file",
		Status:    protocol.StatusStart,
		Args:      []byte(`{"path":"foo.go"}`),
	})
	m = mdl.(model)

	if m.session.turn.runningCount() != 1 {
		t.Fatalf("expected 1 running tool after start, got %d", m.session.turn.runningCount())
	}

	mdl, _ = m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeTool,
		SessionID: "session-a",
		ID:        "tool-1",
		Tool:      "read_file",
		Status:    protocol.StatusOK,
	})
	m = mdl.(model)

	if m.session.turn.runningCount() != 0 {
		t.Fatalf("expected 0 running tools after finish, got %d", m.session.turn.runningCount())
	}
	if m.transcriptEntries.Len() != 1 {
		t.Fatalf("expected 1 transcript entry for the completed tool call, got %d", m.transcriptEntries.Len())
	}
}

func TestHandleFrameDoneFinalizesAndClearsTurn(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.session.id = "session-a"
	m.session.turn = newTurnState()

	mdl, _ := m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeDelta,
		SessionID: "session-a",
		Text:      "hello",
	})
	m = mdl.(model)

	mdl, _ = m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeDone,
		SessionID: "session-a",
	})
	m = mdl.(model)

	if m.session.turn != nil {
		t.Fatal("expected turn to be cleared after done")
	}
}

func TestHandleFrameAdoptsSessionIDWhenUnset(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.session.id = ""

	mdl, _ := m.handleFrame(protocol.ResponseFrame{
		Type:      protocol.TypeChat,
		SessionID: "session-new",
		Text:      "hi",
	})
	m = mdl.(model)

	if m.session.id != "session-new" {
		t.Fatalf("expected sessionID to be adopted from the frame, got %q", m.session.id)
	}
}
