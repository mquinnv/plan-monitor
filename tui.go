package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
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
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	branchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	statusbarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a1a")).
			Foreground(lipgloss.Color("#cccccc"))
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
		// last-prompt events expose the live user input even before it
		// becomes a persisted user turn — prefer them when present.
		if e.Type == "last-prompt" && e.UserText != "" {
			return e.UserText
		}
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

	now := time.Now()
	var top strings.Builder

	// Header
	top.WriteString(renderHeader(m.cwd, m.gitStatus, m.sessionID, m.width))
	top.WriteString("\n")

	// Last prompt — anchors "what was asked"
	if m.lastPrompt != "" {
		top.WriteString("\n")
		top.WriteString(renderLastPrompt(m.lastPrompt, m.width))
		top.WriteString("\n")
	}

	// Plan summary
	if m.planTitle != "" {
		top.WriteString("\n")
		top.WriteString(headerStyle.Render("Plan: " + m.planTitle))
		top.WriteString("\n")
		if m.planStep != "" {
			top.WriteString(inProgressStyle.Render("  ⟳ " + m.planStep))
			top.WriteString("\n")
		}
	}

	// Tasks
	completed, total := taskCounts(m.tasks)
	top.WriteString("\n")
	taskPct := 0.0
	if total > 0 {
		taskPct = 100.0 * float64(completed) / float64(total)
	}
	taskBar := renderBar(10, taskPct, "#7D56F4")
	top.WriteString(headerStyle.Render(fmt.Sprintf("Tasks %s %d/%d", taskBar, completed, total)))
	top.WriteString("\n")
	visible, hidden := capTasks(m.tasks, 10)
	for _, t := range visible {
		top.WriteString(renderTaskLine(t))
		top.WriteString("\n")
	}
	if hidden > 0 {
		top.WriteString(pendingStyle.Render(fmt.Sprintf("  …and %d more", hidden)))
		top.WriteString("\n")
	}

	// Activity feed
	feed := buildActivityFeed(m.allEvents, 7)
	if rendered := renderActivityFeed(feed, now, m.width); rendered != "" {
		top.WriteString("\n")
		top.WriteString(rendered)
	}

	topStr := top.String()
	statusbar := renderStatusbar(m, now)

	// Pin statusbar to bottom of pane.
	topLines := strings.Count(topStr, "\n")
	gap := m.height - topLines - 1
	if gap < 1 {
		gap = 1
	}
	return topStr + strings.Repeat("\n", gap) + statusbar
}

// renderStatusbar packs state and budget info onto a single
// background-filled line at the bottom of the pane.
func renderStatusbar(m model, now time.Time) string {
	var dot string
	switch m.state.Kind {
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
	if !m.state.Since.IsZero() {
		durStr = formatDuration(now.Sub(m.state.Since))
	}

	leftParts := []string{fmt.Sprintf("%s %s %s", dot, m.state.Label(), durStr)}
	if m.modelName != "" {
		leftParts = append(leftParts, shortModel(m.modelName))
	}
	leftParts = append(leftParts, fmt.Sprintf("ctx %s %d%%",
		renderBar(10, m.contextPct, thresholdColor(m.contextPct)),
		int(m.contextPct+0.5)))

	var rightParts []string
	if m.rateOK {
		fhPct := float64(m.rateLimits.FiveHour.UsedPercent)
		wkPct := float64(m.rateLimits.SevenDay.UsedPercent)
		rightParts = append(rightParts,
			fmt.Sprintf("5h %s %d%%→%s",
				renderBar(10, fhPct, thresholdColor(fhPct)),
				m.rateLimits.FiveHour.UsedPercent,
				m.rateLimits.FiveHour.ResetsAt.Local().Format("3:04p")),
			fmt.Sprintf("wk %s %d%%→%s",
				renderBar(10, wkPct, thresholdColor(wkPct)),
				m.rateLimits.SevenDay.UsedPercent,
				m.rateLimits.SevenDay.ResetsAt.Local().Format("Mon")),
		)
		rate := burnRatePctPerMin(m.pctSamples, now)
		if rate > 0 {
			eta := etaToEmptyPct(m.rateLimits.FiveHour.UsedPercent, rate)
			if eta > 0 && now.Add(eta).Before(m.rateLimits.FiveHour.ResetsAt) {
				rightParts = append(rightParts, "empty in "+formatDuration(eta))
			}
		}
	}

	left := strings.Join(leftParts, " · ")
	// Drop right-group items from the end (eta first, then wk, then 5h)
	// only if the line genuinely won't fit — leading + trailing space + at
	// least 1 char between groups.
	for len(rightParts) > 0 {
		right := strings.Join(rightParts, " · ")
		if lipgloss.Width(left)+lipgloss.Width(right)+3 <= m.width {
			break
		}
		rightParts = rightParts[:len(rightParts)-1]
	}
	right := strings.Join(rightParts, " · ")

	var line string
	if right == "" {
		line = " " + left + " "
	} else {
		pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if pad < 1 {
			pad = 1
		}
		line = " " + left + strings.Repeat(" ", pad) + right + " "
	}
	return statusbarStyle.Width(m.width).Render(line)
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

func renderHeader(cwd string, g GitStatus, sessionID string, width int) string {
	prefix := titleStyle.Render("▸ plan-monitor")
	name := projectBasename(cwd)
	left := prefix + " · " + name
	if g.Branch != "" {
		branchStr := "(" + g.Branch + ")"
		if g.Dirty {
			branchStr = dirtyBranch.Render(branchStr)
		} else {
			branchStr = branchStyle.Render(branchStr)
		}
		left = left + " " + branchStr
	}

	if sessionID == "" || width <= 0 {
		return left
	}
	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	right := dimStyle.Render("sess " + shortID)
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 2 {
		return left + "  " + right
	}
	return left + strings.Repeat(" ", pad) + right
}

// renderBar draws a styled progress bar at pct (0-100) with given width and
// solid fill color. Uses bubbles/progress for consistent character rendering.
func renderBar(width int, pct float64, color string) string {
	p := progress.New(
		progress.WithSolidFill(color),
		progress.WithoutPercentage(),
		progress.WithWidth(width),
	)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return p.ViewAs(pct / 100.0)
}

// thresholdColor returns the bar fill color for a usage percentage:
// green default, yellow >=70, red >=85.
func thresholdColor(pct float64) string {
	switch {
	case pct >= 85:
		return "#EF4444"
	case pct >= 70:
		return "#FFCC00"
	default:
		return "#04B575"
	}
}

func renderLastPrompt(text string, width int) string {
	max := width - len("You: ") - 2
	if max < 30 {
		max = 30
	}
	one := strings.ReplaceAll(text, "\n", " ")
	return promptStyle.Render("You: " + truncateArg(one, max))
}

// shortModel renders a model id as "<family> <major>.<minor>" with an
// optional " 1M" suffix for the [1m] context variant. Examples:
//
//	claude-opus-4-7        → "opus 4.7"
//	claude-opus-4-7[1m]    → "opus 4.7 1M"
//	claude-haiku-4-5-20251 → "haiku 4.5"
//	(empty)                → "—"
func shortModel(m string) string {
	if m == "" {
		return "—"
	}
	suffix := ""
	if strings.HasSuffix(m, "[1m]") {
		suffix = " 1M"
		m = strings.TrimSuffix(m, "[1m]")
	}
	m = strings.TrimPrefix(m, "claude-")
	parts := strings.Split(m, "-")
	if len(parts) >= 3 && allDigits(parts[1]) && allDigits(parts[2]) {
		return parts[0] + " " + parts[1] + "." + parts[2] + suffix
	}
	return m + suffix
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
