package hakkacode

import "testing"

func TestToolArgsSnippetShell(t *testing.T) {
	args := map[string]any{"cmd": "rm /tmp/test.txt"}
	got := toolArgsSnippet("shell", args)
	if got != "rm /tmp/test.txt" {
		t.Fatalf("shell snippet = %q, want %q", got, "rm /tmp/test.txt")
	}
}

func TestToolArgsSnippetEditFile(t *testing.T) {
	args := map[string]any{"path": "foo/bar.go", "old": "x", "new": "y"}
	got := toolArgsSnippet("edit_file", args)
	if got != "foo/bar.go" {
		t.Fatalf("edit_file snippet = %q, want %q", got, "foo/bar.go")
	}
}

func TestToolArgsSnippetReadFile(t *testing.T) {
	args := map[string]any{"path": "foo.txt"}
	got := toolArgsSnippet("read_file", args)
	if got != "foo.txt" {
		t.Fatalf("read_file snippet = %q, want %q", got, "foo.txt")
	}
}

func TestToolArgsSnippetHttpGet(t *testing.T) {
	args := map[string]any{"url": "https://example.com"}
	got := toolArgsSnippet("http_get", args)
	if got != "https://example.com" {
		t.Fatalf("http_get snippet = %q, want %q", got, "https://example.com")
	}
}

func TestToolArgsSnippetUnknownToolFallsBackToPath(t *testing.T) {
	args := map[string]any{"path": "bar/baz.go"}
	got := toolArgsSnippet("some_unknown_tool", args)
	if got != "bar/baz.go" {
		t.Fatalf("unknown tool snippet = %q, want %q", got, "bar/baz.go")
	}
}

func TestToolArgsSnippetEmpty(t *testing.T) {
	// Non-map arg, and empty map.
	if got := toolArgsSnippet("shell", "not a map"); got != "" {
		t.Fatalf("non-map arg = %q, want empty", got)
	}
	if got := toolArgsSnippet("shell", map[string]any{}); got != "" {
		t.Fatalf("empty map = %q, want empty", got)
	}
}
