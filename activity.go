package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// formatTool renders a tool_use into a single short feed line.
// The age, status glyph, and styling are applied by the caller.
func formatTool(tu ToolUse) string {
	switch {
	case tu.Name == "Bash":
		cmd, _ := tu.Input["command"].(string)
		return "Bash $ " + cmd
	case tu.Name == "Read":
		fp, _ := tu.Input["file_path"].(string)
		base := filepath.Base(fp)
		off, hasOff := numericInput(tu.Input, "offset")
		lim, hasLim := numericInput(tu.Input, "limit")
		if hasOff && hasLim {
			return fmt.Sprintf("Read %s:%d-%d", base, off, off+lim)
		}
		return "Read " + base
	case tu.Name == "Edit" || tu.Name == "Write":
		fp, _ := tu.Input["file_path"].(string)
		return tu.Name + " " + filepath.Base(fp)
	case tu.Name == "Grep":
		pat, _ := tu.Input["pattern"].(string)
		glob, _ := tu.Input["glob"].(string)
		if glob != "" {
			return fmt.Sprintf("Grep %q %s", pat, glob)
		}
		return fmt.Sprintf("Grep %q", pat)
	case tu.Name == "Glob":
		pat, _ := tu.Input["pattern"].(string)
		return "Glob " + pat
	case tu.Name == "Task" || tu.Name == "Agent":
		st, _ := tu.Input["subagent_type"].(string)
		desc, _ := tu.Input["description"].(string)
		if st == "" {
			st = "Agent"
		}
		return fmt.Sprintf("Agent %s %q", st, desc)
	case tu.Name == "WebFetch":
		raw, _ := tu.Input["url"].(string)
		host := raw
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			host = u.Host
		}
		return "Web fetch " + host
	case tu.Name == "WebSearch":
		q, _ := tu.Input["query"].(string)
		return fmt.Sprintf("Web search %q", q)
	case tu.Name == "Skill":
		name, _ := tu.Input["skill"].(string)
		return "Skill " + name
	case strings.HasPrefix(tu.Name, "mcp__"):
		// mcp__server__tool → mcp server/tool
		rest := strings.TrimPrefix(tu.Name, "mcp__")
		parts := strings.SplitN(rest, "__", 2)
		if len(parts) == 2 {
			return "mcp " + parts[0] + "/" + parts[1]
		}
		return "mcp " + rest
	}
	return tu.Name
}

func numericInput(m map[string]interface{}, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

// truncateArg cuts s to maxRunes runes, adding "…" when truncated.
// The ellipsis counts as one rune within maxRunes.
func truncateArg(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(r[:maxRunes-1]) + "…"
}
