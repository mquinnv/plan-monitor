package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// findPlanFileFromJSONL scans the JSONL for Write tool calls that target
// plan .md files. It checks both ~/.claude/plans/ and project-local
// docs/superpowers/plans/ directories. Returns the full path to the most
// recently written plan file.
func findPlanFileFromJSONL(jsonlPath string, plansDir string, cwd string) string {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	markers := []string{
		`"file_path":"` + plansDir + "/",
	}
	// Also look for project-local plan directories
	if cwd != "" {
		localPlansDir := filepath.Join(cwd, "docs", "superpowers", "plans")
		markers = append(markers, `"file_path":"`+localPlansDir+"/")
	}

	var lastPlanPath string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		for _, marker := range markers {
			idx := strings.Index(line, marker)
			if idx < 0 {
				continue
			}
			// Extract full path after "file_path":"
			fpMarker := `"file_path":"`
			fpIdx := strings.LastIndex(line[:idx+len(marker)], fpMarker)
			if fpIdx < 0 {
				continue
			}
			rest := line[fpIdx+len(fpMarker):]
			if end := strings.Index(rest, `"`); end > 0 && strings.HasSuffix(rest[:end], ".md") {
				lastPlanPath = rest[:end]
			}
		}
	}

	return lastPlanPath
}

// discoverPlan finds the active plan for this session by scanning the JSONL
// for Write tool calls that created/modified plan files.
func discoverPlan(plansDir string, jsonlPath string, cwd string) (title string, content string) {
	planPath := findPlanFileFromJSONL(jsonlPath, plansDir, cwd)
	if planPath == "" {
		return "", ""
	}

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
		title = strings.TrimSuffix(filepath.Base(planPath), ".md")
	}

	return title, planContent
}
