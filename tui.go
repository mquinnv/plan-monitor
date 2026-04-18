package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
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
	tasks       []task
	planTitle   string
	planContent string
	lastUpdate  time.Time
	width       int
	height      int
	scroll      int
	ready       bool
	err         error
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
	return func() tea.Msg {
		return dataMsg{time: time.Now()}
	}
}

type dataMsg struct {
	time time.Time
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
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
		return m, tea.Batch(m.pollData(), m.tick())

	case dataMsg:
		tasks, err := readTasks(m.tasksDir)
		if err != nil {
			m.err = err
		} else {
			m.tasks = tasks
		}

		title, content := discoverPlan(m.plansDir, m.jsonlPath)
		m.planTitle = title
		m.planContent = content
		m.lastUpdate = msg.time
	}

	return m, nil
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder

	// Header
	header := titleStyle.Render("plan-monitor") + " " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(m.cwd)
	b.WriteString(header)
	b.WriteString("\n")

	// Plan section
	if m.planTitle != "" {
		b.WriteString(headerStyle.Render("Plan: " + m.planTitle))
		b.WriteString("\n")

		rendered, err := renderMarkdown(m.planContent, m.width-4)
		if err == nil && rendered != "" {
			// Limit plan display height
			lines := strings.Split(rendered, "\n")
			maxPlanLines := m.height / 3
			if maxPlanLines < 5 {
				maxPlanLines = 5
			}
			if len(lines) > maxPlanLines {
				start := m.scroll
				if start > len(lines)-maxPlanLines {
					start = len(lines) - maxPlanLines
				}
				if start < 0 {
					start = 0
				}
				lines = lines[start : start+maxPlanLines]
				lines = append(lines, pendingStyle.Render("  ... (scroll with j/k)"))
			}
			b.WriteString(strings.Join(lines, "\n"))
			b.WriteString("\n")
		}
	}

	// Tasks section
	completed, total := taskCounts(m.tasks)
	if total > 0 {
		taskHeader := fmt.Sprintf("Tasks (%d/%d complete)", completed, total)
		b.WriteString(headerStyle.Render(taskHeader))
		b.WriteString("\n")

		for _, t := range m.tasks {
			var icon, line string
			switch t.Status {
			case "completed":
				icon = completedStyle.Render("  ✓")
				line = completedStyle.Render(" " + t.Subject)
			case "in_progress":
				icon = inProgressStyle.Render("  ⟳")
				label := t.Subject
				if t.ActiveForm != "" {
					label = t.ActiveForm
				}
				line = inProgressStyle.Render(" " + label)
			default:
				icon = pendingStyle.Render("  ○")
				line = pendingStyle.Render(" " + t.Subject)
			}
			b.WriteString(icon + line + "\n")
		}
	} else {
		b.WriteString(headerStyle.Render("Tasks"))
		b.WriteString("\n")
		b.WriteString(pendingStyle.Render("  No active tasks"))
		b.WriteString("\n")
	}

	// Footer
	elapsed := time.Since(m.lastUpdate).Truncate(time.Second)
	shortID := m.sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	footer := fmt.Sprintf("Session: %s · Updated %s ago · q to quit", shortID, elapsed)
	b.WriteString(footerStyle.Render(footer))

	return b.String()
}

func renderMarkdown(md string, width int) (string, error) {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}
	return r.Render(md)
}
