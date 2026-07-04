package hakkacode

import (
	"context"
	"testing"
)

func TestResumeOrCreateSessionResumesMatchingCWD(t *testing.T) {
	client := &fakeBackend{
		sessions: []map[string]any{
			{"id": "sess-other", "client_cwd": "/other/project", "updated_at": "2026-01-02T00:00:00Z"},
			{"id": "sess-mine", "client_cwd": "/my/project", "updated_at": "2026-01-01T00:00:00Z"},
		},
		getSessionByID: map[string]getSessionResult{
			"sess-mine": {id: "sess-mine"},
		},
	}

	_, sessionID, _, resumed, err := resumeOrCreateSession(context.Background(), client, "/my/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resumed {
		t.Fatal("expected to resume an existing session")
	}
	if sessionID != "sess-mine" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "sess-mine")
	}
}

func TestResumeOrCreateSessionCreatesNewWhenNoCWDMatches(t *testing.T) {
	client := &fakeBackend{
		sessions: []map[string]any{
			{"id": "sess-other", "client_cwd": "/other/project", "updated_at": "2026-01-02T00:00:00Z"},
		},
		createdID: "sess-fresh",
	}

	_, sessionID, _, resumed, err := resumeOrCreateSession(context.Background(), client, "/my/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resumed {
		t.Fatal("expected a new session to be created, not resumed")
	}
	if sessionID != "sess-fresh" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "sess-fresh")
	}
}

func TestResumeOrCreateSessionPicksNewestMatchingCWD(t *testing.T) {
	client := &fakeBackend{
		sessions: []map[string]any{
			{"id": "sess-old", "client_cwd": "/my/project", "updated_at": "2026-01-01T00:00:00Z"},
			{"id": "sess-new", "client_cwd": "/my/project", "updated_at": "2026-01-03T00:00:00Z"},
			{"id": "sess-mid", "client_cwd": "/my/project", "updated_at": "2026-01-02T00:00:00Z"},
		},
		getSessionByID: map[string]getSessionResult{
			"sess-new": {id: "sess-new"},
		},
	}

	_, sessionID, _, resumed, err := resumeOrCreateSession(context.Background(), client, "/my/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resumed {
		t.Fatal("expected to resume an existing session")
	}
	if sessionID != "sess-new" {
		t.Fatalf("sessionID = %q, want %q (newest updated_at)", sessionID, "sess-new")
	}
}

func TestResumeOrCreateSessionFallsBackToMostRecentWhenCWDEmpty(t *testing.T) {
	client := &fakeBackend{
		sessions: []map[string]any{
			{"id": "sess-a", "client_cwd": "/a", "updated_at": "2026-01-01T00:00:00Z"},
		},
		getSessionByID: map[string]getSessionResult{
			"sess-a": {id: "sess-a"},
		},
	}

	_, sessionID, _, resumed, err := resumeOrCreateSession(context.Background(), client, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resumed {
		t.Fatal("expected to resume the most recent session when cwd is unknown")
	}
	if sessionID != "sess-a" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "sess-a")
	}
}
