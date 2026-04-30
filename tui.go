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

	dotIdle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render("●")
	dotThinking = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6")).Render("●")
	dotTool     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")).Render("●")
	dotError    = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("●")
	dotCompact  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A855F7")).Render("●")
	branchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	dirtyBranch = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00"))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)
)

type tickMsg time.Time

type model struct {
	// Config
	tasksDir  string
	plansDir  string
	jsonlPath string
	cwd       string
	sessionID string

	// Persistent state
	reader         *EventReader
	allEvents      []Event     // bounded ring (cap 1000)
	rateLimitsPath string      // ~/.claude/abtop-rate-limits.json or override
	pctSamples     []pctSample // 5h-window snapshots over time, for burn-rate

	// Latest snapshot
	state      State
	modelName  string
	contextPct float64
	rateLimits RateLimits
	rateOK     bool
	planTitle  string
	planStep   string
	tasks      []task
	lastPrompt string
	gitStatus  GitStatus

	// UI
	lastUpdate time.Time
	width      int
	height     int
	ready      bool
	polling    bool
	err        error
}

func newModel(tasksDir, plansDir, jsonlPath, cwd, sessionID string) model {
	r := newEventReader(jsonlPath)
	r.SeedFromEnd(500)
	seeded, _ := r.Seeded()

	m := model{
		tasksDir:       tasksDir,
		plansDir:       plansDir,
		jsonlPath:      jsonlPath,
		cwd:            cwd,
		sessionID:      sessionID,
		reader:         r,
		allEvents:      seeded,
		rateLimitsPath: defaultRateLimitsPath(),
		gitStatus:      gitInfo(cwd),
	}
	m.recomputeFromEvents(time.Now())
	return m
}

func (m *model) recomputeFromEvents(now time.Time) {
	m.state = classifyState(m.allEvents, now)
	for i := len(m.allEvents) - 1; i >= 0; i-- {
		if m.allEvents[i].Model != "" {
			m.modelName = m.allEvents[i].Model
			break
		}
	}
	if last := lastUserPrompt(m.allEvents); last != "" {
		m.lastPrompt = last
	}
	if last := lastUsage(m.allEvents); last != nil {
		m.contextPct = contextPercent(m.modelName, *last)
	}
}

func lastUserPrompt(events []Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Type != "user" {
			continue
		}
		if len(e.ToolResults) > 0 && e.UserText == "" {
			continue
		}
		if e.UserText != "" {
			return e.UserText
		}
	}
	return ""
}

func lastUsage(events []Event) *Usage {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Usage != nil {
			return events[i].Usage
		}
	}
	return nil
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
	reader := m.reader
	tasksDir := m.tasksDir
	plansDir := m.plansDir
	jsonlPath := m.jsonlPath
	cwd := m.cwd
	rlPath := m.rateLimitsPath
	return func() tea.Msg {
		newEvents, _ := reader.Tail()
		tasks, _ := readTasks(tasksDir)
		title, step := discoverPlan(plansDir, jsonlPath, cwd)
		git := gitInfo(cwd)
		rl, rlErr := readRateLimits(rlPath)
		return dataMsg{
			time:         time.Now(),
			newEvents:    newEvents,
			tasks:        tasks,
			planTitle:    title,
			planStep:     step,
			gitStatus:    git,
			rateLimits:   rl,
			rateLimitErr: rlErr,
		}
	}
}

type dataMsg struct {
	time         time.Time
	newEvents    []Event
	tasks        []task
	planTitle    string
	planStep     string
	gitStatus    GitStatus
	rateLimits   RateLimits
	rateLimitErr error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
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
		m.gitStatus = msg.gitStatus
		if len(msg.newEvents) > 0 {
			m.allEvents = append(m.allEvents, msg.newEvents...)
			if len(m.allEvents) > 1000 {
				m.allEvents = m.allEvents[len(m.allEvents)-1000:]
			}
		}
		if msg.rateLimitErr == nil {
			m.rateLimits = msg.rateLimits
			m.rateOK = true
			if len(m.pctSamples) == 0 || m.pctSamples[len(m.pctSamples)-1].pct != msg.rateLimits.FiveHour.UsedPercent {
				m.pctSamples = append(m.pctSamples, pctSample{at: msg.time, pct: msg.rateLimits.FiveHour.UsedPercent})
			}
			cutoff := msg.time.Add(-1 * time.Hour)
			trimmed := m.pctSamples[:0]
			for _, s := range m.pctSamples {
				if s.at.After(cutoff) {
					trimmed = append(trimmed, s)
				}
			}
			m.pctSamples = trimmed
		} else {
			m.rateOK = false
		}
		m.recomputeFromEvents(msg.time)
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
	b.WriteString(renderHeader(m.cwd, m.gitStatus))
	b.WriteString("\n")

	// Status line 1
	b.WriteString(renderStatusLine1(m.state, m.modelName, m.contextPct, time.Now()))
	b.WriteString("\n")

	// Status line 2 — only when rateOK
	if m.rateOK {
		line2 := renderStatusLine2(m.rateLimits, m.pctSamples, time.Now())
		if line2 != "" {
			b.WriteString(line2)
			b.WriteString("\n")
		}
	}

	// Last prompt
	if m.lastPrompt != "" {
		b.WriteString("\n")
		b.WriteString(renderLastPrompt(m.lastPrompt, m.width))
		b.WriteString("\n")
	}

	// Plan summary
	if m.planTitle != "" {
		b.WriteString("\n")
		b.WriteString(headerStyle.Render("Plan: " + m.planTitle))
		b.WriteString("\n")
		if m.planStep != "" {
			b.WriteString(inProgressStyle.Render("  ⟳ " + m.planStep))
			b.WriteString("\n")
		}
	}

	// Tasks
	completed, total := taskCounts(m.tasks)
	b.WriteString("\n")
	bar := progressBar(completed, total, 7)
	b.WriteString(headerStyle.Render(fmt.Sprintf("Tasks %s %d/%d", bar, completed, total)))
	b.WriteString("\n")
	visible, hidden := capTasks(m.tasks, 10)
	for _, t := range visible {
		b.WriteString(renderTaskLine(t))
		b.WriteString("\n")
	}
	if hidden > 0 {
		b.WriteString(pendingStyle.Render(fmt.Sprintf("  …and %d more", hidden)))
		b.WriteString("\n")
	}

	// Activity feed
	feed := buildActivityFeed(m.allEvents, 7)
	if rendered := renderActivityFeed(feed, time.Now(), m.width); rendered != "" {
		b.WriteString("\n")
		b.WriteString(rendered)
	}

	// Footer (placeholder — Task 11 finalizes)
	b.WriteString("\n")
	elapsed := time.Since(m.lastUpdate).Truncate(time.Second)
	shortID := m.sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	b.WriteString(footerStyle.Render(fmt.Sprintf("sess %s · upd %s", shortID, elapsed)))

	return b.String()
}

type feedEntry struct {
	tu      ToolUse
	at      time.Time
	status  feedStatus
	summary string // formatTool result; caller may append " (denied)" / " (exit N)"
}

type feedStatus int

const (
	feedRunning feedStatus = iota
	feedSuccess
	feedFailed
	feedDenied
)

// buildActivityFeed walks events newest-first, pairs tool_use with its
// matching tool_result, and returns up to `cap` entries.
func buildActivityFeed(events []Event, cap int) []feedEntry {
	results := map[string]ToolResult{}
	for _, e := range events {
		for _, tr := range e.ToolResults {
			results[tr.ToolUseID] = tr
		}
	}
	var feed []feedEntry
	for i := len(events) - 1; i >= 0 && len(feed) < cap; i-- {
		e := events[i]
		for _, tu := range e.ToolUses {
			at := parseTimestamp(e.Timestamp)
			st := feedSuccess
			if tr, ok := results[tu.ID]; ok {
				if tr.IsError {
					st = feedFailed
				}
			} else {
				st = feedRunning
			}
			feed = append(feed, feedEntry{
				tu: tu, at: at, status: st, summary: formatTool(tu),
			})
			if len(feed) >= cap {
				break
			}
		}
	}
	return feed
}

func renderActivityFeed(feed []feedEntry, now time.Time, width int) string {
	if len(feed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render("Activity"))
	b.WriteString("\n")
	maxArg := width - 12
	if maxArg < 20 {
		maxArg = 20
	}
	for _, fe := range feed {
		var glyph string
		switch fe.status {
		case feedRunning:
			glyph = inProgressStyle.Render("⟳")
		case feedSuccess:
			glyph = completedStyle.Render("✓")
		case feedFailed, feedDenied:
			glyph = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("✗")
		}
		age := "—"
		if !fe.at.IsZero() {
			age = formatAge(now.Sub(fe.at))
		}
		summary := truncateArg(fe.summary, maxArg)
		b.WriteString(fmt.Sprintf("  %s %3s   %s\n", glyph, age, summary))
	}
	return b.String()
}

func formatAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < 5*time.Minute {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return "5m+"
}

func renderTaskLine(t task) string {
	switch t.Status {
	case "completed":
		return completedStyle.Render("  ✓ " + t.Subject)
	case "in_progress":
		s := t.Subject
		if t.ActiveForm != "" {
			s = t.ActiveForm
		}
		return inProgressStyle.Render("  ⟳ " + s)
	default:
		return pendingStyle.Render("  ○ " + t.Subject)
	}
}

func renderHeader(cwd string, g GitStatus) string {
	prefix := titleStyle.Render("▸ plan-monitor")
	name := projectBasename(cwd)
	if g.Branch == "" {
		return prefix + " · " + name
	}
	branchStr := "(" + g.Branch + ")"
	if g.Dirty {
		branchStr = dirtyBranch.Render(branchStr)
	} else {
		branchStr = branchStyle.Render(branchStr)
	}
	return prefix + " · " + name + " " + branchStr
}

func renderStatusLine1(s State, model string, ctxPct float64, now time.Time) string {
	var dot string
	switch s.Kind {
	case StateIdle:
		dot = dotIdle
	case StateThinking:
		dot = dotThinking
	case StateTool:
		dot = dotTool
	case StateAwaiting, StateError:
		dot = dotError
	case StateCompacting:
		dot = dotCompact
	default:
		dot = dotIdle
	}

	durStr := "0:00"
	if !s.Since.IsZero() {
		durStr = formatDuration(now.Sub(s.Since))
	}
	modelShort := shortModel(model)

	ctxColor := lipgloss.NewStyle()
	if ctxPct >= 85 {
		ctxColor = ctxColor.Foreground(lipgloss.Color("#EF4444"))
	} else if ctxPct >= 70 {
		ctxColor = ctxColor.Foreground(lipgloss.Color("#FFCC00"))
	}
	ctxStr := ctxColor.Render(fmt.Sprintf("ctx %d%%", int(ctxPct+0.5)))

	return fmt.Sprintf("%s %s %s · %s · %s", dot, s.Label(), durStr, modelShort, ctxStr)
}

func renderStatusLine2(rl RateLimits, pctSamples []pctSample, now time.Time) string {
	rate := burnRatePctPerMin(pctSamples, now)
	var etaStr string
	switch {
	case rate == 0:
		etaStr = "measuring…"
	default:
		eta := etaToEmptyPct(rl.FiveHour.UsedPercent, rate)
		if eta == 0 || now.Add(eta).After(rl.FiveHour.ResetsAt) {
			etaStr = "safe until reset"
		} else {
			etaStr = "empty in " + formatDuration(eta)
		}
	}

	return fmt.Sprintf("5h %d%% → %s · wk %d%% → %s · %s",
		rl.FiveHour.UsedPercent, rl.FiveHour.ResetsAt.Local().Format("3:04p"),
		rl.SevenDay.UsedPercent, rl.SevenDay.ResetsAt.Local().Format("Mon"),
		etaStr)
}

func renderLastPrompt(text string, width int) string {
	max := width - len("You: ") - 2
	if max < 30 {
		max = 30
	}
	one := strings.ReplaceAll(text, "\n", " ")
	return promptStyle.Render("You: " + truncateArg(one, max))
}

func shortModel(m string) string {
	switch {
	case strings.Contains(m, "opus"):
		return "opus"
	case strings.Contains(m, "sonnet"):
		return "sonnet"
	case strings.Contains(m, "haiku"):
		return "haiku"
	case m == "":
		return "—"
	default:
		return m
	}
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("0:%02d", int(d.Seconds()))
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) - mins*60
		return fmt.Sprintf("%d:%02d", mins, secs)
	}
	h := int(d.Hours())
	mins := int(d.Minutes()) - h*60
	return fmt.Sprintf("%dh%dm", h, mins)
}
