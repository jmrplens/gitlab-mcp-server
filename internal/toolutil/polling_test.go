package toolutil

import "testing"

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

func TestIsTerminalStatus(t *testing.T) {
	terminal := []string{"success", "failed", "canceled", "skipped", "manual"}
	for _, s := range terminal {
		if !IsTerminalStatus(s) {
			t.Errorf("IsTerminalStatus(%q) = false, want true", s)
		}
	}

	nonTerminal := []string{"running", "pending", "created", "waiting_for_resource", ""}
	for _, s := range nonTerminal {
		if IsTerminalStatus(s) {
			t.Errorf("IsTerminalStatus(%q) = true, want false", s)
		}
	}
}
