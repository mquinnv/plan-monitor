package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	sessionFlag := flag.String("session", "", "Use a specific session ID instead of auto-detecting")
	flag.Parse()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
		os.Exit(1)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects")
	plansDir := filepath.Join(homeDir, ".claude", "plans")

	sessionID, err := resolveSession(claudeProjectsDir, cwd, *sessionFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding session: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure you're in a directory with an active Claude Code session.\n")
		os.Exit(1)
	}

	tasksDir := filepath.Join(homeDir, ".claude", "tasks", sessionID)
	encodedPath := encodeProjectPath(cwd)
	jsonlPath := filepath.Join(claudeProjectsDir, encodedPath, sessionID+".jsonl")

	// Follow the most-recently-active session unless the user pinned one with
	// --session. Without this, a long-lived monitor stays frozen on whatever
	// file was newest at launch and goes stale when the session rotates.
	followActive := *sessionFlag == ""

	m := newModel(tasksDir, plansDir, jsonlPath, cwd, sessionID, followActive)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
