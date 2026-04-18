package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncodeProjectPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/michael/Projects/foo", "-Users-michael-Projects-foo"},
		{"/Users/michael/Projects/plan-monitor", "-Users-michael-Projects-plan-monitor"},
		{"/home/user/code", "-home-user-code"},
	}
	for _, tt := range tests {
		got := encodeProjectPath(tt.input)
		if got != tt.want {
			t.Errorf("encodeProjectPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseSessionsIndex(t *testing.T) {
	data := []byte(`{
		"version": 1,
		"entries": [
			{
				"sessionId": "aaa-111",
				"projectPath": "/Users/test/project",
				"modified": "2026-04-18T10:00:00.000Z",
				"firstPrompt": "hello",
				"messageCount": 5
			},
			{
				"sessionId": "bbb-222",
				"projectPath": "/Users/test/project",
				"modified": "2026-04-18T12:00:00.000Z",
				"firstPrompt": "world",
				"messageCount": 10
			}
		]
	}`)

	entries, err := parseSessionsIndex(data)
	if err != nil {
		t.Fatalf("parseSessionsIndex returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SessionID != "aaa-111" {
		t.Errorf("first entry sessionId = %q, want %q", entries[0].SessionID, "aaa-111")
	}
}

func TestFindMostRecentSession(t *testing.T) {
	entries := []sessionEntry{
		{SessionID: "older", Modified: "2026-04-18T10:00:00.000Z"},
		{SessionID: "newer", Modified: "2026-04-18T12:00:00.000Z"},
	}
	got := findMostRecentSession(entries)
	if got != "newer" {
		t.Errorf("findMostRecentSession = %q, want %q", got, "newer")
	}
}

func TestResolveSession(t *testing.T) {
	// Create temp dir structure mimicking ~/.claude/projects/<encoded>/
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "-test-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexData := `{
		"version": 1,
		"entries": [
			{"sessionId": "sess-abc", "modified": "2026-04-18T14:00:00.000Z"}
		]
	}`
	if err := os.WriteFile(filepath.Join(projectDir, "sessions-index.json"), []byte(indexData), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSession(tmpDir, "/test/project", "")
	if err != nil {
		t.Fatalf("resolveSession error: %v", err)
	}
	if got != "sess-abc" {
		t.Errorf("resolveSession = %q, want %q", got, "sess-abc")
	}
}

func TestResolveSessionExplicitOverride(t *testing.T) {
	got, err := resolveSession("", "", "explicit-id")
	if err != nil {
		t.Fatalf("resolveSession error: %v", err)
	}
	if got != "explicit-id" {
		t.Errorf("resolveSession = %q, want %q", got, "explicit-id")
	}
}

func TestResolveSessionFallbackToJSONL(t *testing.T) {
	// No sessions-index.json, but .jsonl files exist
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "-test-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create two .jsonl files with different mod times
	older := filepath.Join(projectDir, "older-session.jsonl")
	newer := filepath.Join(projectDir, "newer-session.jsonl")
	if err := os.WriteFile(older, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Touch older file to be older
	oldTime := mustParseTime(t, "2026-01-01T00:00:00Z")
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSession(tmpDir, "/test/project", "")
	if err != nil {
		t.Fatalf("resolveSession error: %v", err)
	}
	if got != "newer-session" {
		t.Errorf("resolveSession = %q, want %q", got, "newer-session")
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("failed to parse time %q: %v", s, err)
	}
	return parsed
}
