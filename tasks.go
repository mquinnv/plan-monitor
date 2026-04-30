package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type task struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	ActiveForm  string   `json:"activeForm,omitempty"`
	Status      string   `json:"status"`
	Blocks      []string `json:"blocks"`
	BlockedBy   []string `json:"blockedBy"`
}

func readTasks(dir string) ([]task, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading tasks dir: %w", err)
	}

	var tasks []task
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue // file may have been removed between readdir and read
		}

		var t task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	sort.Slice(tasks, func(i, j int) bool {
		a, _ := strconv.Atoi(tasks[i].ID)
		b, _ := strconv.Atoi(tasks[j].ID)
		return a < b
	})

	return tasks, nil
}

func taskCounts(tasks []task) (completed, total int) {
	total = len(tasks)
	for _, t := range tasks {
		if t.Status == "completed" {
			completed++
		}
	}
	return
}

// progressBar renders done/total filled out of `width` cells using
// unicode block characters. Empty total is treated as zero progress.
func progressBar(done, total, width int) string {
	filled := 0
	if total > 0 {
		filled = done * width / total
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteString("▰")
		} else {
			b.WriteString("▱")
		}
	}
	return b.String()
}

// capTasks returns up to `cap` tasks plus the count of hidden tasks.
// Always includes any in-progress task even if it falls outside the cap.
func capTasks(tasks []task, cap int) (visible []task, hidden int) {
	if len(tasks) <= cap {
		return tasks, 0
	}
	var inProgress []task
	for _, t := range tasks {
		if t.Status == "in_progress" {
			inProgress = append(inProgress, t)
		}
	}
	rest := make([]task, 0, cap)
	for _, t := range tasks {
		if t.Status == "in_progress" {
			continue
		}
		if len(rest)+len(inProgress) >= cap {
			break
		}
		rest = append(rest, t)
	}
	visible = append(rest, inProgress...)
	hidden = len(tasks) - len(visible)
	return visible, hidden
}
