package main

import (
	"testing"
)

// TestRunMetaSchemaSizing_Completes verifies the meta-schema sizing can build
// the full base-plus-enterprise meta-tool registry and measure schema sizes.
// Migrated from the former cmd/audit_meta_schema/main_test.go.
func TestRunMetaSchemaSizing_Completes(t *testing.T) {
	if err := runMetaSchemaSizing(); err != nil {
		t.Fatalf("runMetaSchemaSizing() error: %v", err)
	}
}

// TestHumanBytes_AllMagnitudes verifies the humanBytes byte formatter emits
// expected B/KB/MB suffixes for the three supported magnitude ranges.
func TestHumanBytes_AllMagnitudes(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "sub-kilobyte renders as B", n: 512, want: "512 B"},
		{name: "kilobyte threshold renders as KB", n: 1024, want: "1.0 KB"},
		{name: "megabyte threshold renders as MB", n: 1024 * 1024, want: "1.0 MB"},
		{name: "large value renders as MB", n: 3 * 1024 * 1024, want: "3.0 MB"},
		{name: "zero renders as B", n: 0, want: "0 B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := humanBytes(tt.n); got != tt.want {
				t.Fatalf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
