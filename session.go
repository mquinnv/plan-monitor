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

func resolveSession(claudeProjectsDir string, cwd string, explicitSession string) (string, error) {
	if explicitSession != "" {
		return explicitSession, nil
	}

	encoded := encodeProjectPath(cwd)
	indexPath := filepath.Join(claudeProjectsDir, encoded, "sessions-index.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return "", fmt.Errorf("reading sessions index at %s: %w", indexPath, err)
	}

	entries, err := parseSessionsIndex(data)
	if err != nil {
		return "", err
	}

	sessionID := findMostRecentSession(entries)
	if sessionID == "" {
		return "", fmt.Errorf("no sessions found for project %s", cwd)
	}

	return sessionID, nil
}
