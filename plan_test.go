package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindPlanFileFromJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(plansDir, 0o755)
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	// Simulate a Write tool call with "file_path" field
	content := `{"type":"user","message":"hello"}
{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"` + plansDir + `/my-cool-plan.md","content":"# Plan"}}]}
{"type":"user","message":"done"}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	got := findPlanFileFromJSONL(jsonlPath, plansDir, "")
	want := plansDir + "/my-cool-plan.md"
	if got != want {
		t.Errorf("findPlanFileFromJSONL = %q, want %q", got, want)
	}
}

func TestFindPlanFileFromJSONLIgnoresGenericMentions(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	// ls output mentions plan files but not via "file_path" — should be ignored
	content := `{"type":"user","message":"hello"}
{"type":"assistant","content":"writing plan to ` + plansDir + `/my-cool-plan.md"}
{"stdout":"` + plansDir + `/old-plan.md\n"}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	got := findPlanFileFromJSONL(jsonlPath, plansDir, "")
	if got != "" {
		t.Errorf("findPlanFileFromJSONL = %q, want empty (should ignore non-file_path mentions)", got)
	}
}

func TestFindPlanFileFromJSONLNoPlan(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	content := `{"type":"user","message":"hello"}
{"type":"assistant","content":"no plan here"}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	got := findPlanFileFromJSONL(jsonlPath, plansDir, "")
	if got != "" {
		t.Errorf("findPlanFileFromJSONL = %q, want empty", got)
	}
}

func TestFindPlanFileFromJSONLPicksLast(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(plansDir, 0o755)
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	// Two Write tool calls — should pick the last one
	content := `{"input":{"file_path":"` + plansDir + `/first-plan.md"}}
{"input":{"file_path":"` + plansDir + `/second-plan.md"}}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	got := findPlanFileFromJSONL(jsonlPath, plansDir, "")
	want := plansDir + "/second-plan.md"
	if got != want {
		t.Errorf("findPlanFileFromJSONL = %q, want %q", got, want)
	}
}

func TestFindPlanFileFromJSONLProjectLocal(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	cwd := filepath.Join(tmpDir, "project")
	localPlansDir := filepath.Join(cwd, "docs", "superpowers", "plans")
	os.MkdirAll(localPlansDir, 0o755)
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	content := `{"input":{"file_path":"` + localPlansDir + `/2026-04-20-my-plan.md"}}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	got := findPlanFileFromJSONL(jsonlPath, plansDir, cwd)
	want := localPlansDir + "/2026-04-20-my-plan.md"
	if got != want {
		t.Errorf("findPlanFileFromJSONL = %q, want %q", got, want)
	}
}

func TestDiscoverPlan(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(plansDir, 0o755)
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	os.WriteFile(filepath.Join(plansDir, "test-plan.md"), []byte("# My Test Plan\nsome content"), 0o644)

	content := `{"input":{"file_path":"` + plansDir + `/test-plan.md"}}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	title, planContent := discoverPlan(plansDir, jsonlPath, "")
	if title != "My Test Plan" {
		t.Errorf("title = %q, want %q", title, "My Test Plan")
	}
	if planContent == "" {
		t.Error("expected plan content, got empty")
	}
}

func TestDiscoverPlanNoPlanInSession(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	os.MkdirAll(plansDir, 0o755)
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	os.WriteFile(jsonlPath, []byte(`{"type":"user","message":"hello"}`+"\n"), 0o644)

	title, content := discoverPlan(plansDir, jsonlPath, "")
	if title != "" || content != "" {
		t.Errorf("expected empty, got title=%q content=%q", title, content)
	}
}

func TestDiscoverPlanProjectLocal(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	cwd := filepath.Join(tmpDir, "project")
	localPlansDir := filepath.Join(cwd, "docs", "superpowers", "plans")
	os.MkdirAll(localPlansDir, 0o755)
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	planContent := "# Local Plan\nproject-specific content"
	os.WriteFile(filepath.Join(localPlansDir, "my-plan.md"), []byte(planContent), 0o644)

	content := `{"input":{"file_path":"` + localPlansDir + `/my-plan.md"}}
`
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	title, got := discoverPlan(plansDir, jsonlPath, cwd)
	if title != "Local Plan" {
		t.Errorf("title = %q, want %q", title, "Local Plan")
	}
	if got == "" {
		t.Error("expected plan content, got empty")
	}
}
