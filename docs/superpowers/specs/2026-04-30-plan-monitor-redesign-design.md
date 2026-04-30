# plan-monitor redesign: ambient meta-info dashboard

**Date:** 2026-04-30
**Status:** Approved by user, ready for implementation planning

## Problem

The current plan-monitor TUI shows a project header, a fully-rendered markdown plan, a task list, and a footer. The plan markdown render dominates the panel and duplicates information already accessible by opening the plan file. What's missing is real-time meta-information about the Claude Code session running in the working directory: liveness, model, context fullness, rate-limit budgets, and recent activity.

User quote framing the goal:

> "the intent is to have a panel with meta info about the claude that is running in the working directory we are in"

## Use case

The panel runs in a top-right tmux pane. Claude Code runs in the left pane; a working terminal sits below the monitor. The panel is **ambient**, not interactive — the user glances at it but does not navigate or scroll it. This rules out tabbed UIs, drill-down inspectors, and scrollable content as primary interactions. The job of the panel is to answer **"what is Claude doing right now and how is it going?"** at a glance.

## Architecture: Glance Dashboard

Single-column, top-down layout. Every line earns its keep. No plan markdown rendering. Plan markdown is deliberately removed because the file is one keystroke away in the editor and the panel's job is current state, not reference material.

### Section order (top to bottom)

1. Header
2. Status block (state line + budget line)
3. Last prompt
4. Plan summary
5. Tasks
6. Activity feed
7. Footer

### Full-panel mockup

```
▸ plan-monitor · plan-monitor (main)
● Thinking 0:42 · opus · ctx 67%
5h 38% → 4:32p · wk 12% → Mon · empty in 3h12m

You: i'd like to always have the estimated time the tokens will…

Plan: Add real-time monitor sections
  ⟳ Wire up activity feed

Tasks ▰▰▰▰▱▱▱ 4/7
  ✓ Refactor JSONL reader to incremental
  ✓ Implement status block
  ✓ Add rate-limit detection
  ✓ Wire activity feed parser
  ⟳ Render activity feed
  ○ Add error counter
  ○ Polish footer

Activity
  ⟳  3s   Agent Explore "find session detection bug"
  ✓  8s   Bash $ go test ./...
  ✓ 12s   Read tui.go:1-275
  ✓ 24s   Edit plan.go +12 -3
  ✗ 38s   Bash $ npm test (exit 1)
  ⊘ 2m    Bash $ rm -rf /tmp/x  (denied)

sess a3f2c1b8 · upd 2s · 1 error
```

## Section specs

### Header

Single line. Format: `▸ plan-monitor · <project-basename> (<git-branch>)`.

- `<project-basename>` is the last path component of cwd (not full path — saves width).
- Branch in dim color. Turns yellow if there are uncommitted changes (anything in `git status --porcelain`).

### Status block

Two lines. The most-changed and most-watched part of the panel.

**Line 1 — current state:**
```
● Thinking 0:42 · opus · ctx 67%
```

- **Dot color encodes state:**
  - green ● `Idle` — last event is assistant text; no pending tool call
  - blue ● `Thinking` — assistant turn started, no tool call yet
  - yellow ● `Tool: <name>` — tool call in flight
  - red ● `Awaiting` — tool call needs user permission (heuristic: `tool_use` event with no matching `tool_result` for >N seconds while no streaming activity)
  - red ● `Error` — last turn ended in error / hook denial
  - magenta ● `Compacting` — compaction in progress
- **Time:** duration in current state (`0:42`, `2:13`, `12m`).
- **Model:** short name from JSONL — `opus`, `sonnet`, `haiku`. If a subagent (Task) is currently active, append agent type: `opus › Explore`.
- **ctx %:** input + cached + output tokens vs. model context budget. Yellow >70%, red >85%.

**Line 2 — budgets and projection:**
```
5h 38% → 4:32p · wk 12% → Mon · empty in 3h12m
```

- **Per window:** `<window> <%> → <reset-time>` for both 5-hour rolling and weekly caps.
- **ETA-to-empty:** `empty in <duration>` — the binding window's projected exhaustion at current burn rate. Burn rate is a rolling average over the last ~15 min of activity (single-tool-call rates whipsaw and are useless). When ETA exceeds both reset times, replace with `safe until reset`. When ETA < next reset, render in red.
- **Bootstrap:** when window has < 2 min of usage data, show `measuring…` instead of bogus number.
- **Degraded mode:** if rate-limit data is unavailable for the user's plan/auth, omit line 2 entirely. ctx % on line 1 always works.

**Plan/billing detection:**
- API billing — show `· $X.XX` appended after ETA on line 2.
- Max/Pro — windows are the meaningful budget; no `$` shown.
- Detection mechanism is an investigation item; reference implementations like `claude-monitor` and `ccusage` exist and should be studied during implementation.

### Last prompt

One or two lines. The most recent `user` event in the JSONL, truncated with ellipsis. Dim/italic style.

```
You: i'd like to always have the estimated time the tokens will…
```

If the most recent user message is a tool result or system event (not a real user prompt), walk back to the last actual user prompt.

### Plan summary

Compact replacement for the current full-markdown plan render.

```
Plan: Add real-time monitor sections
  ⟳ Wire up activity feed
```

- Line 1: plan title (extracted from first markdown heading, as today).
- Line 2: the single in-progress step from the plan, if recoverable. Determined by whatever signal is currently used by superpowers plan-execution skills (TBD during implementation — fall back to omitting line 2 if no signal exists).
- Section omitted entirely if no plan is discoverable for the session.

### Tasks

Mostly as today, with two changes:

```
Tasks ▰▰▰▰▱▱▱ 4/7
  ✓ Refactor JSONL reader to incremental
  ⟳ Render activity feed
  ○ Add error counter
```

- Header replaces `Tasks (4/7 complete)` with a unicode block progress bar plus count.
- Cap visible tasks at ~10. If the list exceeds the cap, render `…and N more` instead of allowing scrolling. Always include all in-progress tasks regardless of cap.

### Activity feed

New section. Last 5–8 tool calls, **newest at top**. Each line: `<status> <age> <tool> <arg>`.

```
Activity
  ⟳  3s   Agent Explore "find session detection bug"
  ✓  8s   Bash $ go test ./...
  ✓ 12s   Read tui.go:1-275
  ✓ 24s   Edit plan.go +12 -3
  ✗ 38s   Bash $ npm test (exit 1)
  ⊘ 2m    Bash $ rm -rf /tmp/x  (denied)
```

**Status glyphs:**
- `⟳` in flight (yellow)
- `✓` succeeded (dim green)
- `✗` failed / non-zero exit (red)
- `⊘` denied by hook or user (red)

**Per-tool argument formatting:**
- `Read foo.go:10-50` — file + line range when specified
- `Edit foo.go +N -M` — diff line counts when computable from tool args
- `Bash $ <cmd>` — command, ellipsis-truncated to fit
- `Grep "<pat>" <glob>` — pattern + glob filter
- `Glob <pattern>`
- `Agent <type> "<desc>"` — agent type + description
- `Web fetch <host>` — host only
- `Skill <name>`
- `mcp <server>/<tool>` — collapsed namespace (`mcp linear/get_issue`, not `mcp__linear__get_issue`)

**Rules:**
- Truncation only ever applies to the argument column. Status, age, and tool name never truncated.
- Age column ticks live every second so the feed feels alive even when nothing new happens. Format: `<N>s`, `<N>m`, capped at `5m+` for older entries.
- Denied/errored items are kept in the feed (often the most interesting line); not filtered out.
- Subagent calls show as `Agent <type> (running)` while in flight; v1 does not recurse into the subagent's own JSONL. Could be added later if monitor evolves into an inspector.

### Footer

Single line. Drop the `q to quit` hint (panel is ambient; no need to advertise keys, though `q` still works).

```
sess a3f2c1b8 · upd 2s · 1 error
```

- Short session id (first 8 chars).
- Seconds since last data poll.
- Error/denial count for this session, shown only when > 0, in red.

## Data sourcing

### Incremental JSONL tail

The current implementation re-reads the full session JSONL on every poll. With the activity feed added, this becomes infeasible for long sessions. Replace with an incremental reader:

- Maintain a file offset between polls, per session.
- On each poll, seek to last offset, read new bytes, parse new events, update state.
- Keep a ring buffer of the last ~20 tool-call events (so adaptive height can show more than the default 5–8 without re-reading).
- On panel start, scan from EOF backwards far enough to fill the ring buffer and compute current state.

### Rate-limit data

Three possible sources, in preferred order:

1. **API response headers** logged in JSONL — if Claude Code records `anthropic-ratelimit-*` headers, read them directly.
2. **Self-tracked usage** against a hardcoded plan-cap table — count tokens since window start.
3. **External tool data** — `claude-monitor` / `ccusage` likely write state somewhere; could read theirs.

Investigate during implementation; choose the simplest source that works. The fallback for any failure is the degraded-mode display (omit line 2, keep ctx %).

### Plan / subscription detection

Probably lives in `~/.claude/.credentials.json` or similar. Investigation item. If detection fails, default to "Max-style" display (no `$`).

## Error handling

- JSONL missing or unreadable: show panel with status `● Error · session unavailable`, omit feed/tasks/plan.
- Plan file missing: omit plan section entirely.
- Tasks dir missing: omit tasks section.
- Rate-limit data missing: degraded mode (line 2 omitted).
- Git not initialized: omit branch suffix in header.
- Any single-section failure must not blank the panel — each section renders independently from its own data source.

## Testing

- Unit tests for each formatter: status line, budget line, activity-feed line, tasks header.
- Unit tests for the incremental JSONL reader: append-only growth, file rotation, partial-line at offset boundary.
- Unit tests for burn-rate calculation: empty window, single sample, rolling-average correctness.
- Unit tests for state classifier: each of the six states produces correct dot+label from a JSONL fixture.
- Snapshot test for the full panel render at a fixed terminal size with a fixture session.

## Out of scope (deliberately)

- No tabbed views, scrolling lists, or interactive navigation.
- No drill-down into subagent JSONLs.
- No plan markdown rendering.
- No keyboard shortcuts beyond the existing `q`-to-quit.
- No alerts / notifications outside the panel.

## Open questions / investigation items

These are flagged for the implementation plan rather than the spec:

1. Where does Claude Code store subscription type? (`~/.claude/.credentials.json`? Auth response? Env var?)
2. Does Claude Code log rate-limit response headers in the JSONL? If yes, in what shape?
3. Does the superpowers plan execution skill write any state file we can read for "current in-progress plan step"?
4. How do `claude-monitor` and `ccusage` source their rate-limit data? Crib their approach.
5. Best heuristic for detecting "Awaiting permission" — currently a guess based on stuck `tool_use` events.
