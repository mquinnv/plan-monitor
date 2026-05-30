package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveSiblingSessions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	me := filepath.Join(dir, "me.jsonl")
	freshSibling := filepath.Join(dir, "fresh.jsonl")
	staleSibling := filepath.Join(dir, "stale.jsonl")
	notJSONL := filepath.Join(dir, "ignore.txt")
	subDir := filepath.Join(dir, "subdir")
	for _, p := range []string{me, freshSibling, staleSibling, notJSONL} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Backdate the stale sibling well outside the window.
	if err := os.Chtimes(staleSibling, now.Add(-10*time.Minute), now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	got := liveSiblingSessions(me, now, 2*time.Minute)
	if got != 1 {
		t.Errorf("liveSiblingSessions = %d, want 1 (fresh only)", got)
	}

	// Wide window picks up the stale one too.
	if got := liveSiblingSessions(me, now, time.Hour); got != 2 {
		t.Errorf("wide window = %d, want 2", got)
	}

	// Excluding self: pretending no siblings exist by giving an isolated path.
	if got := liveSiblingSessions(filepath.Join(t.TempDir(), "only.jsonl"), now, time.Hour); got != 0 {
		t.Errorf("isolated dir = %d, want 0", got)
	}
}

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

func TestResolveSessionPrefersFreshJSONLOverStaleIndex(t *testing.T) {
	// sessions-index.json can lag behind reality. The active session may
	// be writing a JSONL file whose mtime is newer than anything the
	// index references. resolveSession must pick the JSONL-mtime winner.
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "-test-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	indexData := `{
		"version": 1,
		"entries": [
			{"sessionId": "stale-session", "modified": "2026-02-03T19:41:12.821Z"}
		]
	}`
	if err := os.WriteFile(filepath.Join(projectDir, "sessions-index.json"), []byte(indexData), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "fresh-session.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveSession(tmpDir, "/test/project", "")
	if err != nil {
		t.Fatalf("resolveSession error: %v", err)
	}
	if got != "fresh-session" {
		t.Errorf("resolveSession = %q, want %q (must prefer JSONL mtime over stale index)", got, "fresh-session")
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

func TestMostRecentlyActiveSession(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jsonl")
	b := filepath.Join(dir, "b.jsonl")
	notJSONL := filepath.Join(dir, "sessions-index.json")
	for _, p := range []string{a, b, notJSONL} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Backdate a so b is the most-recently-active.
	old := mustParseTime(t, "2026-01-01T00:00:00Z")
	if err := os.Chtimes(a, old, old); err != nil {
		t.Fatal(err)
	}

	got, ok := mostRecentlyActiveSession(dir)
	if !ok {
		t.Fatal("mostRecentlyActiveSession ok=false, want true")
	}
	if got != b {
		t.Errorf("mostRecentlyActiveSession = %q, want %q", got, b)
	}

	if _, ok := mostRecentlyActiveSession(t.TempDir()); ok {
		t.Error("empty dir: ok=true, want false")
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
