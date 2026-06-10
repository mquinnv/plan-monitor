package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEventReaderInitialReadFromEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	preamble := `{"type":"user","message":{"content":"old"}}` + "\n"
	if err := os.WriteFile(path, []byte(preamble), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newEventReader(path)
	r.SeedFromEnd(10)
	seeded, err := r.Seeded()
	if err != nil {
		t.Fatalf("Seeded error: %v", err)
	}
	if len(seeded) != 1 {
		t.Fatalf("expected 1 seeded event, got %d", len(seeded))
	}
	if seeded[0].Type != "user" {
		t.Errorf("seeded[0].Type = %q, want %q", seeded[0].Type, "user")
	}
}

func TestEventReaderTailReturnsOnlyNewLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user","message":{"content":"a"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newEventReader(path)
	r.SeedFromEnd(10)
	if _, err := r.Seeded(); err != nil {
		t.Fatal(err)
	}

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"assistant","message":{"content":"b"}}` + "\n")
	f.WriteString(`{"type":"user","message":{"content":"c"}}` + "\n")
	f.Close()

	newEvents, err := r.Tail()
	if err != nil {
		t.Fatalf("Tail error: %v", err)
	}
	if len(newEvents) != 2 {
		t.Fatalf("expected 2 new events, got %d", len(newEvents))
	}
	if newEvents[0].Type != "assistant" || newEvents[1].Type != "user" {
		t.Errorf("got types %q, %q", newEvents[0].Type, newEvents[1].Type)
	}

	more, err := r.Tail()
	if err != nil {
		t.Fatalf("Tail error: %v", err)
	}
	if len(more) != 0 {
		t.Errorf("expected 0 events on second Tail, got %d", len(more))
	}
}

func TestEventReaderHandlesPartialFinalLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	complete := `{"type":"user","message":{"content":"a"}}` + "\n"
	partial := `{"type":"assistant"`
	if err := os.WriteFile(path, []byte(complete+partial), 0o644); err != nil {
		t.Fatal(err)
	}

	r := newEventReader(path)
	r.SeedFromEnd(10)
	seeded, err := r.Seeded()
	if err != nil {
		t.Fatalf("Seeded error: %v", err)
	}
	if len(seeded) != 1 {
		t.Fatalf("expected 1 complete event, got %d", len(seeded))
	}

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`,"message":{"content":"b"}}` + "\n")
	f.Close()

	newEvents, err := r.Tail()
	if err != nil {
		t.Fatalf("Tail error: %v", err)
	}
	if len(newEvents) != 1 {
		t.Fatalf("expected 1 tailed event, got %d", len(newEvents))
	}
	if newEvents[0].Type != "assistant" {
		t.Errorf("Type = %q, want %q", newEvents[0].Type, "assistant")
	}
}
