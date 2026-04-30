package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func encodeProjectPath(absPath string) string {
	return strings.ReplaceAll(absPath, "/", "-")
}

type sessionEntry struct {
	SessionID    string `json:"sessionId"`
	ProjectPath  string `json:"projectPath"`
	Modified     string `json:"modified"`
	FirstPrompt  string `json:"firstPrompt"`
	MessageCount int    `json:"messageCount"`
	GitBranch    string `json:"gitBranch"`
}

type sessionsIndex struct {
	Version int            `json:"version"`
	Entries []sessionEntry `json:"entries"`
}

func parseSessionsIndex(data []byte) ([]sessionEntry, error) {
	var idx sessionsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing sessions index: %w", err)
	}
	return idx.Entries, nil
}

// findMostRecentSession returns the session ID with the latest Modified timestamp.
// The entries slice is sorted in place. ISO 8601 UTC timestamps sort lexicographically.
func findMostRecentSession(entries []sessionEntry) string {
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Modified > entries[j].Modified
	})
	return entries[0].SessionID
}

// discoverSessionsFromJSONL finds sessions by listing .jsonl files in the project
// directory and returning the most recently modified one. This is the fallback
// when sessions-index.json doesn't exist.
func discoverSessionsFromJSONL(projectDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		return "", fmt.Errorf("listing session files: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no session files found in %s", projectDir)
	}

	// Sort by modification time, most recent first
	sort.Slice(matches, func(i, j int) bool {
		infoI, errI := os.Stat(matches[i])
		infoJ, errJ := os.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// Extract session ID from filename (strip directory and .jsonl extension)
	base := filepath.Base(matches[0])
	return strings.TrimSuffix(base, ".jsonl"), nil
}

func resolveSession(claudeProjectsDir string, cwd string, explicitSession string) (string, error) {
	if explicitSession != "" {
		return explicitSession, nil
	}

	encoded := encodeProjectPath(cwd)
	projectDir := filepath.Join(claudeProjectsDir, encoded)

	// Always discover from JSONL file mtimes — sessions-index.json can lag
	// arbitrarily behind reality (its `modified` timestamps are not refreshed
	// per turn), and we want the truly-active session, not the most-recently-
	// indexed one.
	return discoverSessionsFromJSONL(projectDir)
}
