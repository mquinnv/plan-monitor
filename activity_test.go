package main

import "testing"

func TestFormatToolBash(t *testing.T) {
	tu := ToolUse{Name: "Bash", Input: map[string]interface{}{"command": "go test ./..."}}
	got := formatTool(tu)
	want := "Bash $ go test ./..."
	if got != want {
		t.Errorf("formatTool(Bash) = %q, want %q", got, want)
	}
}

func TestFormatToolReadWithLines(t *testing.T) {
	tu := ToolUse{Name: "Read", Input: map[string]interface{}{
		"file_path": "/some/path/tui.go", "offset": float64(10), "limit": float64(50),
	}}
	got := formatTool(tu)
	want := "Read tui.go:10-60"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatToolReadNoLines(t *testing.T) {
	tu := ToolUse{Name: "Read", Input: map[string]interface{}{"file_path": "/x/foo.go"}}
	got := formatTool(tu)
	if got != "Read foo.go" {
		t.Errorf("got %q, want %q", got, "Read foo.go")
	}
}

func TestFormatToolGrep(t *testing.T) {
	tu := ToolUse{Name: "Grep", Input: map[string]interface{}{"pattern": "tickMsg", "glob": "*.go"}}
	got := formatTool(tu)
	if got != `Grep "tickMsg" *.go` {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolAgent(t *testing.T) {
	tu := ToolUse{Name: "Task", Input: map[string]interface{}{
		"subagent_type": "Explore", "description": "find session bug",
	}}
	got := formatTool(tu)
	if got != `Agent Explore "find session bug"` {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolMCP(t *testing.T) {
	tu := ToolUse{Name: "mcp__linear__get_issue", Input: map[string]interface{}{}}
	got := formatTool(tu)
	if got != "mcp linear/get_issue" {
		t.Errorf("got %q", got)
	}
}

func TestFormatToolWebFetch(t *testing.T) {
	tu := ToolUse{Name: "WebFetch", Input: map[string]interface{}{"url": "https://docs.anthropic.com/api/foo"}}
	got := formatTool(tu)
	if got != "Web fetch docs.anthropic.com" {
		t.Errorf("got %q", got)
	}
}

func TestTruncateArg(t *testing.T) {
	got := truncateArg("a very long bash command here", 15)
	if got != "a very long ba…" {
		t.Errorf("got %q", got)
	}
}
