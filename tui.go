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
	tasks        []task
	planTitle    string
	planRendered string // pre-rendered markdown
	lastUpdate   time.Time
	width        int
	height       int
	scroll       int
	ready        bool
	polling      bool
	err          error
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
	width := m.width
	return func() tea.Msg {
		tasks, _ := readTasks(tasksDir)
		title, content := discoverPlan(plansDir, jsonlPath)
		var rendered string
		if content != "" {
			rendered, _ = renderMarkdown(content, width-4)
		}
		return dataMsg{
			time:         time.Now(),
			tasks:        tasks,
			planTitle:    title,
			planRendered: rendered,
		}
	}
}

type dataMsg struct {
	time         time.Time
	tasks        []task
	planTitle    string
	planRendered string
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
		m.planRendered = msg.planRendered
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

	// Calculate space budget
	// 1 header + 1 plan title + len(taskLines) + 1 footer + margins
	fixedLines := 1 + len(taskLines) + 2 // header + tasks + footer + blank
	if m.planTitle != "" {
		fixedLines++ // plan title line
	}
	planBudget := m.height - fixedLines
	if planBudget < 5 {
		planBudget = 5
	}

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

		if m.planRendered != "" {
			lines := strings.Split(strings.TrimRight(m.planRendered, "\n"), "\n")
			scrollable := len(lines) > planBudget
			if scrollable {
				start := m.scroll
				maxStart := len(lines) - planBudget
				if start > maxStart {
					start = maxStart
				}
				if start < 0 {
					start = 0
				}
				lines = lines[start : start+planBudget]
				lines = append(lines, pendingStyle.Render("  ... (scroll with j/k)"))
			}
			for _, line := range lines {
				b.WriteString(line)
				b.WriteString("\n")
				usedLines++
			}
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
