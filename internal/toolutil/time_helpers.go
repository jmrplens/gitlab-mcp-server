// time_helpers.go provides time-parsing utilities shared across tool handlers.

package toolutil

import "time"

// ParseOptionalTime parses an RFC3339 string and returns a *time.Time.
// Returns nil if the string is empty or unparseable.
func ParseOptionalTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// FormatTime converts an RFC3339 timestamp string to a human-readable format
// ("2 Jan 2006 15:04 UTC"). Returns the original string unchanged if parsing
// fails, so existing callers remain safe.
func FormatTime(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t.UTC().Format("2 Jan 2006 15:04 UTC")
	}
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t.Format("2 Jan 2006")
	}
	return s
}
