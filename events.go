package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
)

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type ToolUse struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	IsError   bool   `json:"is_error"`
	Content   string `json:"-"`
}

type Event struct {
	Type        string
	Timestamp   string
	Cwd         string
	Model       string
	UserText    string
	ToolUses    []ToolUse
	ToolResults []ToolResult
	Usage       *Usage
	RawLine     string
}

type EventReader struct {
	path    string
	offset  int64
	seeded  []Event
	seedErr error
}

func newEventReader(path string) *EventReader {
	return &EventReader{path: path}
}

func (r *EventReader) SeedFromEnd(maxEvents int) {
	f, err := os.Open(r.path)
	if err != nil {
		r.seedErr = err
		return
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		r.seedErr = err
		return
	}

	// Set offset to position after the last complete newline so any
	// trailing partial line is re-read on the next Tail() once completed.
	lastNL := -1
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '\n' {
			lastNL = i
			break
		}
	}
	if lastNL == -1 {
		r.offset = 0
	} else {
		r.offset = int64(lastNL + 1)
	}

	events := parseLines(data, true)
	if len(events) > maxEvents {
		events = events[len(events)-maxEvents:]
	}
	r.seeded = events
}

func (r *EventReader) Seeded() ([]Event, error) {
	return r.seeded, r.seedErr
}

func (r *EventReader) Tail() ([]Event, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	lastNL := -1
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '\n' {
			lastNL = i
			break
		}
	}
	var consume []byte
	if lastNL == -1 {
		consume = nil
	} else {
		consume = data[:lastNL+1]
		r.offset += int64(lastNL + 1)
	}

	if len(consume) == 0 {
		return nil, nil
	}
	return parseLines(consume, false), nil
}

func parseLines(data []byte, dropPartial bool) []Event {
	var events []Event
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	hasTrailingNL := len(data) > 0 && data[len(data)-1] == '\n'

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		// Oversized lines (>4 MiB) are dropped here; not fatal — we
		// return whatever we managed to parse so the panel keeps
		// rendering rather than crashing the TUI.
		_ = err
	}
	if dropPartial && !hasTrailingNL && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}

	for _, line := range lines {
		if e, ok := parseEvent(line); ok {
			events = append(events, e)
		}
	}
	return events
}

func parseEvent(line string) (Event, bool) {
	if line == "" {
		return Event{}, false
	}
	var raw struct {
		Type       string          `json:"type"`
		Timestamp  string          `json:"timestamp"`
		Cwd        string          `json:"cwd"`
		Message    json.RawMessage `json:"message"`
		LastPrompt string          `json:"lastPrompt"` // present on type=last-prompt events
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return Event{}, false
	}

	ev := Event{Type: raw.Type, Timestamp: raw.Timestamp, Cwd: raw.Cwd, RawLine: line}

	if raw.Type == "last-prompt" && raw.LastPrompt != "" {
		ev.UserText = raw.LastPrompt
	}

	if len(raw.Message) > 0 {
		var msg struct {
			Model   string          `json:"model"`
			Content json.RawMessage `json:"content"`
			Usage   *Usage          `json:"usage"`
		}
		if err := json.Unmarshal(raw.Message, &msg); err == nil {
			ev.Model = msg.Model
			ev.Usage = msg.Usage
			extractContent(&ev, msg.Content)
		}
	}
	return ev, true
}

func extractContent(ev *Event, content json.RawMessage) {
	if len(content) == 0 {
		return
	}
	// Plain string content (e.g. simple user message).
	var asString string
	if err := json.Unmarshal(content, &asString); err == nil {
		ev.UserText = asString
		return
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return
	}
	for _, raw := range blocks {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			continue
		}
		switch head.Type {
		case "text":
			var t struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &t); err == nil && ev.UserText == "" {
				ev.UserText = t.Text
			}
		case "tool_use":
			var tu ToolUse
			if err := json.Unmarshal(raw, &tu); err == nil {
				ev.ToolUses = append(ev.ToolUses, tu)
			}
		case "tool_result":
			var tr ToolResult
			if err := json.Unmarshal(raw, &tr); err == nil {
				ev.ToolResults = append(ev.ToolResults, tr)
			}
		}
	}
}
