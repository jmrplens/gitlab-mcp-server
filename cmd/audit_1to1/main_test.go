package main

import (
	"strings"
	"testing"
)

func TestParseScope_DefaultAll(t *testing.T) {
	got, err := parseScope("structs,actions,metadata")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d scopes, want 3: %v", len(got), got)
	}
}

func TestParseScope_KeywordAll(t *testing.T) {
	got, err := parseScope("all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("'all' should expand to 3 scopes, got %d: %v", len(got), got)
	}
}

func TestParseScope_EmptyExpandsToAll(t *testing.T) {
	got, err := parseScope("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("empty should expand to 3 scopes, got %d: %v", len(got), got)
	}
}

func TestParseScope_SingleScope(t *testing.T) {
	got, err := parseScope("structs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "structs" {
		t.Fatalf("got %v, want [structs]", got)
	}
}

func TestParseScope_Deduplicates(t *testing.T) {
	got, err := parseScope("structs,structs,actions,actions")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unique scopes, got %d: %v", len(got), got)
	}
}

func TestParseScope_TrimsWhitespace(t *testing.T) {
	got, err := parseScope(" structs , actions , metadata ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 scopes after trim, got %d: %v", len(got), got)
	}
}

func TestParseScope_RejectsUnknown(t *testing.T) {
	_, err := parseScope("structs,unknown")
	if err == nil {
		t.Fatal("expected error for unknown scope, got nil")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error should mention 'unknown', got: %v", err)
	}
}

func TestParseScope_RejectsGarbage(t *testing.T) {
	cases := []string{"foo", "structs,bar", "???"}
	for _, c := range cases {
		if _, err := parseScope(c); err == nil {
			t.Errorf("parseScope(%q) expected error, got nil", c)
		}
	}
}

func TestRunSingle_UnknownScope(t *testing.T) {
	_, err := runSingle("bogus", false)
	if err == nil {
		t.Fatal("expected error for unknown scope")
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("error should mention 'unknown scope', got: %v", err)
	}
}
