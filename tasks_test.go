package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTasks(t *testing.T) {
	tmpDir := t.TempDir()

	// Write task files
	task1 := `{"id":"1","subject":"Do thing","description":"desc","status":"completed","blocks":[],"blockedBy":[]}`
	task2 := `{"id":"2","subject":"Other thing","description":"desc2","activeForm":"Doing other thing","status":"in_progress","blocks":[],"blockedBy":[]}`
	task3 := `{"id":"3","subject":"Future thing","description":"desc3","status":"pending","blocks":[],"blockedBy":[]}`

	os.WriteFile(filepath.Join(tmpDir, "1.json"), []byte(task1), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "2.json"), []byte(task2), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "3.json"), []byte(task3), 0o644)
	// Also write a .lock file that should be ignored
	os.WriteFile(filepath.Join(tmpDir, ".lock"), []byte(""), 0o644)

	tasks, err := readTasks(tmpDir)
	if err != nil {
		t.Fatalf("readTasks error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Should be sorted by ID
	if tasks[0].ID != "1" || tasks[1].ID != "2" || tasks[2].ID != "3" {
		t.Errorf("tasks not sorted by ID: got %s, %s, %s", tasks[0].ID, tasks[1].ID, tasks[2].ID)
	}
	if tasks[1].ActiveForm != "Doing other thing" {
		t.Errorf("task 2 activeForm = %q, want %q", tasks[1].ActiveForm, "Doing other thing")
	}
}

func TestReadTasksEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	tasks, err := readTasks(tmpDir)
	if err != nil {
		t.Fatalf("readTasks error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestReadTasksMissingDir(t *testing.T) {
	tasks, err := readTasks("/nonexistent/path")
	if err != nil {
		t.Fatalf("readTasks should not error on missing dir, got: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestTaskCounts(t *testing.T) {
	tasks := []task{
		{ID: "1", Status: "completed"},
		{ID: "2", Status: "in_progress"},
		{ID: "3", Status: "pending"},
		{ID: "4", Status: "completed"},
	}
	completed, total := taskCounts(tasks)
	if completed != 2 || total != 4 {
		t.Errorf("taskCounts = (%d, %d), want (2, 4)", completed, total)
	}
}
