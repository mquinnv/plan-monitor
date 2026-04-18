package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// findPlanFileFromJSONL scans the JSONL for Write tool calls that target
// ~/.claude/plans/*.md. This avoids false positives from ls output or other
// mentions of the plans directory. The pattern we look for is:
//   "file_path":"/Users/.../.claude/plans/<slug>.md"
func findPlanFileFromJSONL(jsonlPath string, plansDir string) string {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	// The pattern that appears in Write tool_use input
	marker := `"file_path":"` + plansDir + "/"

	var lastPlanFile string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		// Extract filename after the marker
		rest := line[idx+len(marker):]
		if end := strings.Index(rest, `"`); end > 0 && strings.HasSuffix(rest[:end], ".md") {
			lastPlanFile = rest[:end]
		}
	}

	return lastPlanFile
}

// discoverPlan finds the active plan for this session by scanning the JSONL
// for Write tool calls that created/modified plan files in ~/.claude/plans/.
func discoverPlan(plansDir string, jsonlPath string) (title string, content string) {
	planFile := findPlanFileFromJSONL(jsonlPath, plansDir)
	if planFile == "" {
		return "", ""
	}

	planPath := filepath.Join(plansDir, planFile)
	data, err := os.ReadFile(planPath)
	if err != nil {
		return "", ""
	}
	planContent := string(data)

	// Extract title from first markdown heading
	for _, line := range strings.Split(planContent, "\n") {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
			break
		}
	}
	if title == "" {
		title = strings.TrimSuffix(planFile, ".md")
	}

	return title, planContent
}
