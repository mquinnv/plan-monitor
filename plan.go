package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sessionUsedPlan scans the JSONL file line-by-line for EnterPlanMode tool calls.
// Does not parse full JSON — just looks for the string to avoid loading large files.
func sessionUsedPlan(jsonlPath string) bool {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "EnterPlanMode") {
			return true
		}
	}
	return false
}

// findMostRecentPlan returns the name and content of the most recently modified
// .md file in the plans directory.
func findMostRecentPlan(plansDir string) (name string, content string, err error) {
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("reading plans dir: %w", err)
	}

	type planFile struct {
		name    string
		modTime int64
	}

	var plans []planFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		plans = append(plans, planFile{name: entry.Name(), modTime: info.ModTime().UnixNano()})
	}

	if len(plans) == 0 {
		return "", "", nil
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].modTime > plans[j].modTime
	})

	mostRecent := plans[0]
	data, err := os.ReadFile(filepath.Join(plansDir, mostRecent.name))
	if err != nil {
		return "", "", fmt.Errorf("reading plan file: %w", err)
	}

	return mostRecent.name, string(data), nil
}

// discoverPlan finds the active plan content. It checks if the session used a plan,
// and returns the most recently modified plan file.
func discoverPlan(plansDir string, jsonlPath string) (title string, content string) {
	if !sessionUsedPlan(jsonlPath) {
		return "", ""
	}

	name, planContent, err := findMostRecentPlan(plansDir)
	if err != nil || name == "" {
		return "", ""
	}

	// Extract title from first markdown heading
	for _, line := range strings.Split(planContent, "\n") {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
			break
		}
	}
	if title == "" {
		title = strings.TrimSuffix(name, ".md")
	}

	return title, planContent
}
