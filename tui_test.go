package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// When the active Claude session in a directory rotates (a newer .jsonl
// appears), an MRA-following monitor must rebind to it: swapping the reader,
// session ID, and reseeding/recomputing derived state. This is the fix for
// the "goes stale on long-running sessions" bug — the monitor was frozen to
// whatever file was newest at launch.
func TestSwitchSessionRebindsToNewerFile(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old-sess.jsonl")
	if err := os.WriteFile(old, []byte(`{"type":"assistant","timestamp":"2026-05-15T10:00:00Z","message":{"model":"claude-old","content":[{"type":"text","text":"hi"}]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := model{
		jsonlPath:    old,
		sessionID:    "old-sess",
		followActive: true,
		reader:       newEventReader(old),
	}
	m.reader.SeedFromEnd(500)
	m.allEvents, _ = m.reader.Seeded()
	m.recomputeFromEvents(time.Now())
	if m.modelName != "claude-old" {
		t.Fatalf("precondition: modelName = %q, want %q", m.modelName, "claude-old")
	}

	// A newer session file appears in the same directory.
	newp := filepath.Join(dir, "new-sess.jsonl")
	if err := os.WriteFile(newp, []byte(`{"type":"assistant","timestamp":"2026-05-29T10:00:00Z","message":{"model":"claude-new","content":[{"type":"text","text":"hi"}]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m.switchSession(newp, time.Now())

	if m.jsonlPath != newp {
		t.Errorf("jsonlPath = %q, want %q", m.jsonlPath, newp)
	}
	if m.sessionID != "new-sess" {
		t.Errorf("sessionID = %q, want %q", m.sessionID, "new-sess")
	}
	if m.modelName != "claude-new" {
		t.Errorf("modelName = %q, want %q (reseed+recompute from new file)", m.modelName, "claude-new")
	}
}

func TestShortModel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"claude-opus-4-7", "opus 4.7"},
		{"claude-opus-4-7[1m]", "opus 4.7 1M"},
		{"claude-sonnet-4-6", "sonnet 4.6"},
		{"claude-haiku-4-5-20251001", "haiku 4.5"},
		{"", "—"},
		{"unknown-model", "unknown-model"},
	}
	for _, c := range cases {
		got := shortModel(c.in)
		if got != c.want {
			t.Errorf("shortModel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatBudget(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{200_000, "200k"},
		{1_000_000, "1M"},
		{1_500_000, "1.5M"},
		{500, "500"},
		{0, "0"},
	}
	for _, c := range cases {
		got := formatBudget(c.in)
		if got != c.want {
			t.Errorf("formatBudget(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestContextBudget(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-7", 200_000},
		{"claude-opus-4-7[1m]", 1_000_000},
		{"unknown-model", defaultContextBudget},
	}
	for _, c := range cases {
		got := contextBudget(c.model)
		if got != c.want {
			t.Errorf("contextBudget(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}
