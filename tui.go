package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	dotIdle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render("●")
	dotThinking = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6")).Render("●")
	dotTool     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFCC00")).Render("●")
	dotError    = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("●")
	dotCompact  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A855F7")).Render("●")

	statusbarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a1a")).
			Foreground(lipgloss.Color("#cccccc"))

	promptStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a1a")).
			Foreground(lipgloss.Color("#7a7a7a"))
)

type tickMsg time.Time

type model struct {
	// Config
	jsonlPath string
	sessionID string

	// followActive makes the monitor re-bind to the most-recently-active
	// .jsonl in the project dir each poll, so it follows a session that
	// rotates underneath it (new session, /clear, resume, compaction).
	// Disabled when an explicit --session was given (the user pinned it).
	followActive bool

	// Persistent state
	reader         *EventReader
	allEvents      []Event     // bounded ring (cap 1000)
	rateLimitsPath string      // ~/.claude/abtop-rate-limits.json or override
	pctSamples     []pctSample // 5h-window snapshots over time, for burn-rate

	// Latest snapshot
	state      State
	modelName  string
	contextPct float64
	lastPrompt string
	rateLimits RateLimits
	rateOK     bool

	// UI
	lastUpdate time.Time
	width      int
	height     int
	ready      bool
	polling    bool
	err        error
}

func newModel(jsonlPath, sessionID string, followActive bool) model {
	r := newEventReader(jsonlPath)
	r.SeedFromEnd(500)
	seeded, _ := r.Seeded()

	m := model{
		jsonlPath:      jsonlPath,
		sessionID:      sessionID,
		followActive:   followActive,
		reader:         r,
		allEvents:      seeded,
		rateLimitsPath: defaultRateLimitsPath(),
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
	if last := lastUsage(m.allEvents); last != nil {
		m.contextPct = contextPercent(m.modelName, *last)
	}
	m.lastPrompt = lastUserPrompt(m.allEvents)
}

// switchSession re-binds the monitor to a different session .jsonl: it opens a
// fresh reader, re-seeds from the end, and recomputes all derived state.
// Called when the active session rotates and the monitor is in follow-active
// mode.
func (m *model) switchSession(jsonlPath string, now time.Time) {
	sessionID := strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
	r := newEventReader(jsonlPath)
	r.SeedFromEnd(500)
	seeded, _ := r.Seeded()

	m.jsonlPath = jsonlPath
	m.sessionID = sessionID
	m.reader = r
	m.allEvents = seeded
	m.recomputeFromEvents(now)
}

func lastUsage(events []Event) *Usage {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Usage != nil {
			return events[i].Usage
		}
	}
	return nil
}

// lastUserPrompt returns the text of the most recent thing the user sent.
// Claude Code emits an explicit `last-prompt` event holding the clean prompt
// text; real `user` turns also carry it as plain-string content (tool_result
// turns leave UserText empty, so they're skipped). Whichever is newest in the
// event stream wins.
func lastUserPrompt(events []Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if (e.Type == "last-prompt" || e.Type == "user") && e.UserText != "" {
			return e.UserText
		}
	}
	return ""
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
	jsonlPath := m.jsonlPath
	rlPath := m.rateLimitsPath
	follow := m.followActive
	return func() tea.Msg {
		// Follow session rotation: if a newer .jsonl has appeared in the
		// project dir, surface it so Update can re-bind. Empty when not
		// following or nothing newer exists.
		activeJSONL := ""
		if follow {
			if mra, ok := mostRecentlyActiveSession(filepath.Dir(jsonlPath)); ok && mra != jsonlPath {
				activeJSONL = mra
			}
		}
		newEvents, _ := reader.Tail()
		rl, rlErr := readRateLimits(rlPath)
		return dataMsg{
			time:         time.Now(),
			activeJSONL:  activeJSONL,
			newEvents:    newEvents,
			rateLimits:   rl,
			rateLimitErr: rlErr,
		}
	}
}

type dataMsg struct {
	time         time.Time
	activeJSONL  string // non-empty when a newer session file should be adopted
	newEvents    []Event
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
		// Session rotated: re-bind to the newer file and discard this batch's
		// events (they were tailed from the old reader). They refresh on the
		// next poll.
		if msg.activeJSONL != "" && msg.activeJSONL != m.jsonlPath {
			m.switchSession(msg.activeJSONL, msg.time)
			m.lastUpdate = msg.time
			return m, nil
		}
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

	statusbar := renderStatusbar(m, time.Now())

	// Pin the status block to the bottom of the pane. When there's room for a
	// second line, show the last thing the user sent above the statusbar.
	lines := []string{statusbar}
	if m.height >= 2 {
		lines = []string{renderPromptLine(m.lastPrompt, m.width), statusbar}
	}
	gap := m.height - len(lines)
	if gap < 0 {
		gap = 0
	}
	return strings.Repeat("\n", gap) + strings.Join(lines, "\n")
}

// renderPromptLine renders the user's most recent prompt as a single
// background-filled line, collapsing whitespace and truncating to width.
func renderPromptLine(prompt string, width int) string {
	if width < 1 {
		width = 1
	}
	text := strings.Join(strings.Fields(prompt), " ")
	if text == "" {
		text = "—"
	}
	content := truncateRunes("❯ "+text, width-2)
	return promptStyle.Width(width).Render(" " + content + " ")
}

// truncateRunes clips s to at most max runes, marking elision with an ellipsis.
func truncateRunes(s string, max int) string {
	if max < 1 {
		max = 1
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
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
	right := strings.Join(rightParts, " · ")
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)

	var line string
	switch {
	case right == "":
		line = " " + left + " "
	case leftW+rightW+4 <= m.width:
		// Plenty of room — right-align the right group with a stretched gap.
		pad := m.width - leftW - rightW - 2
		if pad < 1 {
			pad = 1
		}
		line = " " + left + strings.Repeat(" ", pad) + right + " "
	case leftW+rightW+5 <= m.width:
		// Tight but everything fits inline with a single ` · ` joiner.
		line = " " + left + " · " + right + " "
	default:
		// Too narrow even inline. Drop right-group items from the end
		// (eta → wk → 5h) until packing left + " · " + right fits.
		for len(rightParts) > 0 {
			right = strings.Join(rightParts, " · ")
			if leftW+lipgloss.Width(right)+5 <= m.width {
				break
			}
			rightParts = rightParts[:len(rightParts)-1]
		}
		if len(rightParts) == 0 {
			line = " " + left + " "
		} else {
			line = " " + left + " · " + right + " "
		}
	}
	return statusbarStyle.Width(m.width).Render(line)
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
