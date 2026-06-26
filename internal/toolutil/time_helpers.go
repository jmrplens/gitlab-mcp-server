package toolutil

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

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

// FormatTimePtr renders an optional *time.Time as RFC 3339, or "" when nil.
func FormatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// FormatISOTimePtr renders an optional *gl.ISOTime as YYYY-MM-DD, or "" when nil.
func FormatISOTimePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}
