package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
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

func TestLastUserPrompt(t *testing.T) {
	events := []Event{
		{Type: "last-prompt", UserText: "first thing"},
		{Type: "assistant", UserText: "i replied"},
		{Type: "user", UserText: ""}, // tool_result turn — no text
		{Type: "last-prompt", UserText: "second thing"},
		{Type: "user", UserText: ""}, // another tool_result
	}
	if got := lastUserPrompt(events); got != "second thing" {
		t.Errorf("lastUserPrompt = %q, want %q", got, "second thing")
	}

	// Falls back to a real user turn when no last-prompt event is present.
	userOnly := []Event{
		{Type: "user", UserText: "typed it"},
		{Type: "assistant", UserText: "ok"},
	}
	if got := lastUserPrompt(userOnly); got != "typed it" {
		t.Errorf("lastUserPrompt = %q, want %q", got, "typed it")
	}

	if got := lastUserPrompt(nil); got != "" {
		t.Errorf("lastUserPrompt(nil) = %q, want empty", got)
	}
}

func TestFirstUserPrompt(t *testing.T) {
	events := []Event{
		{Type: "assistant", UserText: "intro"}, // not a user turn
		{Type: "last-prompt", UserText: "first thing"},
		{Type: "user", UserText: ""}, // tool_result turn — no text
		{Type: "last-prompt", UserText: "second thing"},
	}
	if got := firstUserPrompt(events); got != "first thing" {
		t.Errorf("firstUserPrompt = %q, want %q", got, "first thing")
	}

	// Falls back to a real user turn when no last-prompt event is present.
	userOnly := []Event{
		{Type: "user", UserText: "typed it"},
		{Type: "user", UserText: "typed again"},
	}
	if got := firstUserPrompt(userOnly); got != "typed it" {
		t.Errorf("firstUserPrompt = %q, want %q", got, "typed it")
	}

	if got := firstUserPrompt(nil); got != "" {
		t.Errorf("firstUserPrompt(nil) = %q, want empty", got)
	}
}

func TestRenderPromptLine(t *testing.T) {
	// Newlines and runs of whitespace collapse to single spaces.
	got := renderPromptLine("", "hello\n\n  world", 40)
	if !strings.Contains(got, "❯ hello world") {
		t.Errorf("renderPromptLine = %q, want it to contain %q", got, "❯ hello world")
	}

	// A label is shown before the prompt marker.
	labeled := renderPromptLine("first", "the goal", 40)
	if !strings.Contains(labeled, "first") || !strings.Contains(labeled, "❯ the goal") {
		t.Errorf("labeled renderPromptLine = %q, want it to contain label and prompt", labeled)
	}

	// Long prompts truncate with an ellipsis and never exceed the width.
	wide := renderPromptLine("", "this is a very long prompt that will not fit", 20)
	if w := lipgloss.Width(wide); w != 20 {
		t.Errorf("rendered width = %d, want 20", w)
	}
	if !strings.Contains(wide, "…") {
		t.Errorf("expected truncated line to contain ellipsis, got %q", wide)
	}

	// Empty prompt renders the em-dash placeholder.
	if got := renderPromptLine("", "", 20); !strings.Contains(got, "—") {
		t.Errorf("empty prompt = %q, want placeholder %q", got, "—")
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
