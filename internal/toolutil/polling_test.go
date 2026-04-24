// polling_test.go contains unit tests for the polling helper functions
// ClampPollInterval, ClampPollTimeout, and IsTerminalStatus.
package toolutil

import "testing"

// TestClampPollInterval verifies that ClampPollInterval constrains a polling
// interval to [PollMinInterval, PollMaxInterval] and returns PollDefaultInterval
// when the input is below the minimum. Table-driven subtests cover values below,
// at, within, and above the allowed range.
func TestClampPollInterval(t *testing.T) {
	tests := []struct {
		name string
		v    int
		want int
	}{
		{"below minimum returns default", 0, PollDefaultInterval},
		{"negative returns default", -1, PollDefaultInterval},
		{"at minimum", PollMinInterval, PollMinInterval},
		{"mid range", 30, 30},
		{"at maximum", PollMaxInterval, PollMaxInterval},
		{"above maximum clamped", 120, PollMaxInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampPollInterval(tt.v); got != tt.want {
				t.Errorf("ClampPollInterval(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

// TestClampPollTimeout verifies that ClampPollTimeout constrains a polling
// timeout to [PollMinTimeout, PollMaxTimeout] and returns PollDefaultTimeout
// when the input is zero or negative. Table-driven subtests cover zero,
// negative, minimum, mid-range, maximum, and above-maximum values.
func TestClampPollTimeout(t *testing.T) {
	tests := []struct {
		name string
		v    int
		want int
	}{
		{"zero returns default", 0, PollDefaultTimeout},
		{"negative returns default", -5, PollDefaultTimeout},
		{"at minimum", PollMinTimeout, PollMinTimeout},
		{"mid range", 600, 600},
		{"at maximum", PollMaxTimeout, PollMaxTimeout},
		{"above maximum clamped", 5000, PollMaxTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampPollTimeout(tt.v); got != tt.want {
				t.Errorf("ClampPollTimeout(%d) = %d, want %d", tt.v, got, tt.want)
			}
		})
	}
}

// TestIsTerminalStatus verifies that IsTerminalStatus correctly classifies
// CI/CD pipeline statuses. Table-driven subtests assert that "success",
// "failed", "canceled", "skipped", and "manual" are terminal, while
// "running", "pending", "created", "waiting_for_resource", and empty
// string are non-terminal.
func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"terminal: success", "success", true},
		{"terminal: failed", "failed", true},
		{"terminal: canceled", "canceled", true},
		{"terminal: skipped", "skipped", true},
		{"terminal: manual", "manual", true},
		{"non-terminal: running", "running", false},
		{"non-terminal: pending", "pending", false},
		{"non-terminal: created", "created", false},
		{"non-terminal: waiting_for_resource", "waiting_for_resource", false},
		{"non-terminal: empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminalStatus(tt.status); got != tt.want {
				t.Errorf("IsTerminalStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
