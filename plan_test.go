package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindPlanNameFromJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	lines := []string{
		`{"type":"user","message":"hello"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"planning"},{"type":"tool_use","name":"EnterPlanMode","input":{}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"here is the plan"}]}}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	found := sessionUsedPlan(jsonlPath)
	if !found {
		t.Error("expected sessionUsedPlan to return true")
	}
}

func TestFindPlanNameFromJSONLNoPlan(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "session.jsonl")

	lines := []string{
		`{"type":"user","message":"hello"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"no plan here"}]}}`,
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	os.WriteFile(jsonlPath, []byte(content), 0o644)

	found := sessionUsedPlan(jsonlPath)
	if found {
		t.Error("expected sessionUsedPlan to return false")
	}
}

func TestFindMostRecentPlan(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "old-plan.md"), []byte("# Old Plan\nstuff"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "new-plan.md"), []byte("# New Plan\nbetter stuff"), 0o644)

	name, content, err := findMostRecentPlan(tmpDir)
	if err != nil {
		t.Fatalf("findMostRecentPlan error: %v", err)
	}
	if name == "" {
		t.Fatal("expected a plan name, got empty")
	}
	if content == "" {
		t.Fatal("expected plan content, got empty")
	}
}

func TestFindMostRecentPlanEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	name, content, err := findMostRecentPlan(tmpDir)
	if err != nil {
		t.Fatalf("findMostRecentPlan error: %v", err)
	}
	if name != "" || content != "" {
		t.Errorf("expected empty results, got name=%q content=%q", name, content)
	}
}
