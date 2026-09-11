package toolutil

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
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

// A timestamp has two spellings here and each helper is named for the one it
// writes. The wire form is RFC 3339 in UTC, what GitLab sends and what an
// output struct carries so a client can parse it: [RFC3339] and [RFC3339Ptr]
// write it from a time.Time. The display form is "2 Jan 2006 15:04 UTC", what
// a Markdown card or table shows a reader: [FormatTime] writes it from the
// wire string and [FormatTimeValue] from a time.Time. The names used to say
// nothing about which was which, and a table came to show the wire form
// because a helper called FormatTimePtr rendered it.
//
// Both forms are in UTC. A layout ending in a literal Z stamps whatever wall
// clock the value carries with a zone it may not be in, so the conversion
// happens here, once, rather than at each caller.
const (
	displayTimeLayout = "2 Jan 2006 15:04 UTC"
	displayDateLayout = "2 Jan 2006"
)

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
		return t.UTC().Format(displayTimeLayout)
	}
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t.Format(displayDateLayout)
	}
	return EscapeMdTableCell(s)
}

// FormatTimeValue renders t in the display form, or "" for a zero time, which
// is what a field GitLab did not send decodes to and would otherwise read as
// the year one.
func FormatTimeValue(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(displayTimeLayout)
}

// RFC3339 renders t in the wire form, RFC 3339 in UTC, or "" for a zero time.
func RFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// RFC3339Ptr renders an optional time in the wire form, or "" when nil.
func RFC3339Ptr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return RFC3339(*t)
}

// FormatTimePtr renders an optional *time.Time as RFC 3339, or "" when nil.
//
// It is superseded by [RFC3339Ptr], which it is an alias of: the name says
// display and the output is the wire form, which is how a table came to show
// RFC 3339 where every other one shows [FormatTime]'s layout. New code calls
// RFC3339Ptr, or [FormatTimeValue] for a value a reader sees, and the
// staticcheck deprecation marker goes on here once the forty-odd callers have
// moved, since the marker fails the lint gate on every one of them until then.
func FormatTimePtr(t *time.Time) string {
	return RFC3339Ptr(t)
}

// FormatISOTimePtr renders an optional *gl.ISOTime as YYYY-MM-DD, or "" when nil.
func FormatISOTimePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}
