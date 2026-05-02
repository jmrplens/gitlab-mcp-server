package toolutil

// Polling bounds and defaults (all values in seconds) for tools that wait on
// a GitLab resource to reach a terminal state (pipelines, jobs, deployments).
const (
	PollMinInterval     = 5
	PollMaxInterval     = 60
	PollDefaultInterval = 10
	PollMinTimeout      = 1
	PollMaxTimeout      = 3600
	PollDefaultTimeout  = 300
)

// pollTerminalStatuses contains statuses that indicate a CI/CD resource has finished.
var pollTerminalStatuses = map[string]bool{
	"success":  true,
	"failed":   true,
	"canceled": true,
	"skipped":  true,
	"manual":   true,
}

// IsTerminalStatus reports whether a CI/CD status represents a finished state.
func IsTerminalStatus(status string) bool {
	return pollTerminalStatuses[status]
}

// ClampPollInterval constrains a polling interval to [PollMinInterval, PollMaxInterval],
// returning PollDefaultInterval when the value is below the minimum.
func ClampPollInterval(v int) int {
	if v < PollMinInterval {
		return PollDefaultInterval
	}
	if v > PollMaxInterval {
		return PollMaxInterval
	}
	return v
}

// ClampPollTimeout constrains a polling timeout to [PollMinTimeout, PollMaxTimeout],
// returning PollDefaultTimeout when the value is below the minimum.
func ClampPollTimeout(v int) int {
	if v < PollMinTimeout {
		return PollDefaultTimeout
	}
	if v > PollMaxTimeout {
		return PollMaxTimeout
	}
	return v
}
