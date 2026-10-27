// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// timeFieldLayouts are the spellings a timestamp input accepts, tried in
// order.
//
// RFC 3339 is what GitLab's timestamp parameters take and what the published
// schemas advertise. The two shorter forms are there because a model told an
// input is a date sends exactly "2025-01-01", GitLab reads it as midnight UTC
// itself, and refusing it would cost a round trip to learn nothing. Neither
// short form carries a zone, so time.Parse reads both as UTC, which is what
// GitLab does with them.
var timeFieldLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", DateFormatISO}

// dateFieldLayouts are the spellings a date-only input accepts. A timestamp is
// accepted for the day it names, since GitLab coerces one into its Date
// parameters too and a caller holding a timestamp means that day.
var dateFieldLayouts = []string{DateFormatISO, time.RFC3339, "2006-01-02T15:04:05"}

// ParseTimeField reads one timestamp input, naming the field it came from
// when the value does not parse.
//
// It replaces a parser that answered nil for a value it could not read, which
// every one of its 84 callers took for an absent input: a list went out with
// no date filter and answered more than was asked, and a note asked to be
// backdated was dated now, each reported as a success (issue 1308). A value
// that is not a timestamp is the caller's mistake, and only the caller can
// correct it, so it is refused.
func ParseTimeField(field, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil //nolint:nilnil // an empty value is the input left out, which the caller sends as nothing
	}
	for _, layout := range timeFieldLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("%s must be an ISO 8601 timestamp such as 2025-01-01T00:00:00Z, or a date such as 2025-01-01 (read as midnight UTC), got %q", field, value)
}

// ParseDateField reads one date-only input, naming the field it came from
// when the value does not parse.
//
// An RFC 3339 value is read for the calendar date it is written in, not the
// date it falls on in UTC: an expiry of 2026-12-31T23:00:00-05:00 names the
// last day of 2026, and moving it to the first of 2027 would grant a day the
// caller did not ask for. That is the case issue 1308 found dropped outright,
// for an SSH key and an invitation that then never expired.
func ParseDateField(field, value string) (*gl.ISOTime, error) {
	if value == "" {
		return nil, nil //nolint:nilnil // an empty value is the input left out, which the caller sends as nothing
	}
	for _, layout := range dateFieldLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			day := gl.ISOTime(time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC))
			return &day, nil
		}
	}
	return nil, fmt.Errorf("%s must be a date in YYYY-MM-DD form such as 2025-01-31, got %q", field, value)
}

// ClearableDate reads one optional date input of an update, where a field
// left out and a field set to an empty string mean different things: nil
// leaves the stored date alone, and an empty string removes it. An empty
// string returns a zero [gl.ISOTime], which client-go encodes as JSON null,
// the value GitLab clears a date with. Anything else is read on the terms of
// [ParseDateField].
func ClearableDate(field string, value *string) (*gl.ISOTime, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // a field left out is sent as nothing
	}
	if *value == "" {
		return new(gl.ISOTime{}), nil
	}
	return ParseDateField(field, *value)
}

// ParseDateFieldString reads one date-only input on the terms of
// [ParseDateField] and returns it as YYYY-MM-DD, for the client-go options
// that carry a date as a plain string, the member expiries among them. Such a
// field used to forward whatever the caller wrote, so a value GitLab could
// not read was sent and refused only after the request, if at all.
func ParseDateFieldString(field, value string) (*string, error) {
	date, err := ParseDateField(field, value)
	if err != nil || date == nil {
		return nil, err
	}
	return new(date.String()), nil
}

// TimeInput names one timestamp input and where its parsed value goes.
type TimeInput struct {
	Field string
	Value string
	Dest  **time.Time
}

// ParseTimeInputs parses each input into its destination, leaving the
// destination of an empty one alone, and refuses the call on the first value
// that does not parse, before any later one is read.
func ParseTimeInputs(inputs ...TimeInput) error {
	for _, input := range inputs {
		parsed, err := ParseTimeField(input.Field, input.Value)
		if err != nil {
			return err
		}
		if parsed != nil {
			*input.Dest = parsed
		}
	}
	return nil
}

// DateInput names one date-only input and where its parsed value goes.
type DateInput struct {
	Field string
	Value string
	Dest  **gl.ISOTime
}

// ParseDateInputs parses each date-only input into its destination, on the
// terms of [ParseTimeInputs].
func ParseDateInputs(inputs ...DateInput) error {
	for _, input := range inputs {
		parsed, err := ParseDateField(input.Field, input.Value)
		if err != nil {
			return err
		}
		if parsed != nil {
			*input.Dest = parsed
		}
	}
	return nil
}

// ClearableDateInput names one optional date input of an update and where
// its value goes.
type ClearableDateInput struct {
	Field string
	Value *string
	Dest  **gl.ISOTime
}

// ClearableDateInputs reads each update date into its destination on the
// terms of [ClearableDate], refusing the call on the first value that does
// not parse, before any later one is read.
func ClearableDateInputs(inputs ...ClearableDateInput) error {
	for _, input := range inputs {
		parsed, err := ClearableDate(input.Field, input.Value)
		if err != nil {
			return err
		}
		*input.Dest = parsed
	}
	return nil
}

// TimestampParam returns the request option that sends a timestamp parameter
// as the instant the caller gave, for a parameter GitLab declares DateTime
// that a client-go option struct carries as a date (upstream register row
// 126). Such a field encodes as YYYY-MM-DD, which GitLab reads as midnight
// UTC: an SSH key asked to expire at 23:00 expired 23 hours early, and a list
// bounded at 18:00 was bounded at the start of its day.
//
// client-go writes its options into the JSON body of a POST, PUT or PATCH and
// into the query of any other method, and the option writes the timestamp
// under name in the same place, in RFC 3339 with the offset it was given in,
// replacing whatever was there. The caller leaves the option struct's own
// field unset, so this is the one writer of the parameter. A nil timestamp
// returns no option, and the request goes out as client-go built it.
func TimestampParam(name string, at *time.Time) gl.RequestOptionFunc {
	if at == nil {
		return nil
	}
	value := at.Format(time.RFC3339Nano)
	return func(req *retryablehttp.Request) error {
		if req.Method != http.MethodPost && req.Method != http.MethodPut && req.Method != http.MethodPatch {
			query := req.URL.Query()
			query.Set(name, value)
			req.URL.RawQuery = query.Encode()
			return nil
		}
		raw, err := req.BodyBytes()
		if err != nil {
			return fmt.Errorf("read the request to send %s: %w", name, err)
		}
		var body map[string]any
		if len(raw) > 0 {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			// Numbers stay as written, so a value beside the timestamp is
			// sent as client-go encoded it rather than through a float64.
			decoder.UseNumber()
			if err = decoder.Decode(&body); err != nil {
				return fmt.Errorf("decode the request to send %s: %w", name, err)
			}
		}
		if body == nil {
			body = map[string]any{}
		}
		body[name] = value
		// A map of what encoding/json decoded marshals without error, and
		// SetBody takes a byte slice without one; both are joined rather than
		// branched on so that neither impossible failure is left untested.
		rewritten, marshalErr := json.Marshal(body)
		return errors.Join(marshalErr, req.SetBody(rewritten))
	}
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
