package main

import (
	"strings"
	"testing"
)

// TestParseScope covers the happy and error paths of -scope parsing in one
// table: explicit/keyword/empty expansion to all three, single scope,
// deduplication, whitespace trimming, and rejection of unknown/garbage values.
func TestParseScope(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantCount int    // expected scope count on success
		wantFirst string // expected sole scope (single-scope cases); "" to skip
		wantErr   bool
		errSubstr string // required error substring when wantErr; "" to skip
	}{
		{name: "explicit all three", input: "structs,actions,metadata", wantCount: 3},
		{name: "keyword all expands", input: "all", wantCount: 3},
		{name: "empty expands to all", input: "", wantCount: 3},
		{name: "single scope", input: "structs", wantCount: 1, wantFirst: "structs"},
		{name: "deduplicates repeats", input: "structs,structs,actions,actions", wantCount: 2},
		{name: "trims whitespace", input: " structs , actions , metadata ", wantCount: 3},
		{name: "rejects unknown token", input: "structs,unknown", wantErr: true, errSubstr: "unknown"},
		{name: "rejects garbage word", input: "foo", wantErr: true},
		{name: "rejects mixed garbage", input: "structs,bar", wantErr: true},
		{name: "rejects symbols", input: "???", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseScope(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseScope(%q) expected error, got nil", c.input)
				}
				if c.errSubstr != "" && !strings.Contains(err.Error(), c.errSubstr) {
					t.Errorf("parseScope(%q) error = %q, want substring %q", c.input, err, c.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseScope(%q) unexpected error: %v", c.input, err)
			}
			if len(got) != c.wantCount {
				t.Fatalf("parseScope(%q) = %v (%d scopes), want %d", c.input, got, len(got), c.wantCount)
			}
			if c.wantFirst != "" && (len(got) != 1 || got[0] != c.wantFirst) {
				t.Errorf("parseScope(%q) = %v, want [%s]", c.input, got, c.wantFirst)
			}
		})
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
