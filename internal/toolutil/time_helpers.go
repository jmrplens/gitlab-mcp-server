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
// ("2 Jan 2006 15:04 UTC"), falling back to the date-only layout and then to
// the string it was given.
//
// Every one of this function's callers writes the result straight into
// Markdown, so the fallback runs it through [EscapeMdTableCell] rather than
// returning it as it arrived. A value that reaches the fallback is by
// definition not a timestamp, which leaves only two ways to get there: a field
// this server formatted itself and a field carrying whatever GitLab put in it.
// Returning the second verbatim put a pipe, a newline and a '<' into a table
// cell from 155 call sites, and telling each of those call sites to escape a
// timestamp would have taught the next reader that a date needs escaping.
// Escaping here costs nothing that renders: neither layout's output contains
// any character the escaper touches.
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
	return EscapeMdTableCell(s)
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
