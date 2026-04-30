package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginTop(1)

	completedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))

	inProgressStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFCC00"))

	pendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			MarginTop(1)
)

type tickMsg time.Time

type model struct {
	// Config
	tasksDir  string
	plansDir  string
	jsonlPath string
	cwd       string
	sessionID string

	// State
	tasks      []task
	planTitle  string
	planStep   string
	lastUpdate time.Time
	width      int
	height     int
	scroll     int
	ready      bool
	polling    bool
	err        error
}

func newModel(tasksDir, plansDir, jsonlPath, cwd, sessionID string) model {
	return model{
		tasksDir:  tasksDir,
		plansDir:  plansDir,
		jsonlPath: jsonlPath,
		cwd:       cwd,
		sessionID: sessionID,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.pollData(),
		m.tick(),
	)
}

func (m model) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) pollData() tea.Cmd {
	tasksDir := m.tasksDir
	plansDir := m.plansDir
	jsonlPath := m.jsonlPath
	cwd := m.cwd
	return func() tea.Msg {
		tasks, _ := readTasks(tasksDir)
		title, step := discoverPlan(plansDir, jsonlPath, cwd)
		return dataMsg{
			time:      time.Now(),
			tasks:     tasks,
			planTitle: title,
			planStep:  step,
		}
	}
}

type dataMsg struct {
	time      time.Time
	tasks     []task
	planTitle string
	planStep  string
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.scroll > 0 {
				m.scroll--
			}
		case "down", "j":
			m.scroll++
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	case tickMsg:
		if m.polling {
			return m, m.tick()
		}
		m.polling = true
		return m, tea.Batch(m.pollData(), m.tick())

	case dataMsg:
		m.polling = false
		m.tasks = msg.tasks
		m.planTitle = msg.planTitle
		m.planStep = msg.planStep
		m.lastUpdate = msg.time
	}

	return m, nil
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	// Build tasks section first so we can measure it
	var taskLines []string
	completed, total := taskCounts(m.tasks)
	if total > 0 {
		taskLines = append(taskLines, headerStyle.Render(fmt.Sprintf("Tasks (%d/%d complete)", completed, total)))
		for _, t := range m.tasks {
			var icon, label string
			switch t.Status {
			case "completed":
				icon = completedStyle.Render("  ✓")
				label = completedStyle.Render(" " + t.Subject)
			case "in_progress":
				icon = inProgressStyle.Render("  ⟳")
				subj := t.Subject
				if t.ActiveForm != "" {
					subj = t.ActiveForm
				}
				label = inProgressStyle.Render(" " + subj)
			default:
				icon = pendingStyle.Render("  ○")
				label = pendingStyle.Render(" " + t.Subject)
			}
			taskLines = append(taskLines, icon+label)
		}
	} else {
		taskLines = append(taskLines, headerStyle.Render("Tasks"))
		taskLines = append(taskLines, pendingStyle.Render("  No active tasks"))
	}

	// Build footer
	elapsed := time.Since(m.lastUpdate).Truncate(time.Second)
	shortID := m.sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	footer := fmt.Sprintf("Session: %s · Updated %s ago · q to quit", shortID, elapsed)

	// Build output
	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("plan-monitor") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(m.cwd))
	b.WriteString("\n")
	usedLines := 1

	// Plan section
	if m.planTitle != "" {
		b.WriteString(headerStyle.Render("Plan: " + m.planTitle))
		b.WriteString("\n")
		usedLines++

		if m.planStep != "" {
			b.WriteString(inProgressStyle.Render("  ⟳ " + m.planStep))
			b.WriteString("\n")
			usedLines++
		}
	}

	// Tasks section
	for _, line := range taskLines {
		b.WriteString(line)
		b.WriteString("\n")
		usedLines++
	}

	// Pad to push footer to bottom
	for usedLines < m.height-1 {
		b.WriteString("\n")
		usedLines++
	}

	// Footer
	b.WriteString(footerStyle.Render(footer))

	return b.String()
}

