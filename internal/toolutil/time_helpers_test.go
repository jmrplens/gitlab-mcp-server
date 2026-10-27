// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// time_helpers_test.go contains unit tests for time formatting and parsing helpers.
package toolutil

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/sourcewalk"
)

// TestFormatTime_ValidRFC3339 verifies that FormatTime formats a valid
// RFC 3339 timestamp into a human-readable date string.
func TestFormatTime_ValidRFC3339(t *testing.T) {
	got := FormatTime("2026-03-20T15:45:00Z")
	want := "20 Mar 2026 15:45 UTC"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

// TestFormatTime_WithTimezone verifies that FormatTime correctly handles
// timestamps with explicit timezone offsets.
func TestFormatTime_WithTimezone(t *testing.T) {
	got := FormatTime("2026-03-20T10:45:00-05:00")
	want := "20 Mar 2026 15:45 UTC"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

// TestFormatTime_Empty verifies that FormatTime returns an empty string
// when given an empty input.
func TestFormatTime_Empty(t *testing.T) {
	got := FormatTime("")
	if got != "" {
		t.Errorf("FormatTime(\"\") = %q, want empty", got)
	}
}

// TestFormatTime_InvalidFormat verifies that FormatTime returns the string it
// was given when it cannot be parsed as a timestamp, unchanged for a value
// that renders as itself.
func TestFormatTime_InvalidFormat(t *testing.T) {
	input := "not-a-date"
	got := FormatTime(input)
	if got != input {
		t.Errorf("FormatTime(%q) = %q, want original input", input, got)
	}
}

// TestFormatTime_InvalidFormat_ContainsTheValue verifies that a value reaching
// the fallback cannot change the shape of the Markdown it is written into.
//
// Only a value that is not a timestamp gets this far, so the fallback is the
// one branch of this function that can be handed text somebody wrote. Every
// caller writes the result into a table cell or a list item, and the pipe, the
// newline and the '<' each end or reshape one.
func TestFormatTime_InvalidFormat_ContainsTheValue(t *testing.T) {
	got := FormatTime("a | b\nc <img src=x>")
	if strings.ContainsAny(got, "|\n<") {
		t.Errorf("FormatTime() = %q, which still carries a pipe, a newline or a '<'", got)
	}
	if !strings.Contains(got, "&#124;") || !strings.Contains(got, "&lt;") {
		t.Errorf("FormatTime() = %q, want the pipe and the angle bracket as entities", got)
	}
}

// TestParseTimeField_ReadsEverySpellingAGitLabTimestampTakes verifies the
// three spellings a timestamp input accepts: RFC 3339 with any offset, a
// local date-time read as UTC, and a bare date read as midnight UTC. The
// second and third are what a model told the input is a date sends, and GitLab
// itself reads both.
func TestParseTimeField_ReadsEverySpellingAGitLabTimestampTakes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Time
	}{
		{"rfc3339 in UTC", "2026-01-02T03:04:05Z", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"rfc3339 with an offset", "2026-01-02T03:04:05-05:00", time.Date(2026, 1, 2, 8, 4, 5, 0, time.UTC)},
		{"rfc3339 with fractional seconds", "2026-01-02T03:04:05.5Z", time.Date(2026, 1, 2, 3, 4, 5, 500000000, time.UTC)},
		{"local date-time", "2026-01-02T03:04:05", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"bare date", "2026-01-02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeField("created_after", tt.value)
			if err != nil {
				t.Fatalf("ParseTimeField(%q) error = %v", tt.value, err)
			}
			if got == nil || !got.Equal(tt.want) {
				t.Errorf("ParseTimeField(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

// TestParseTimeField_EmptyIsAbsent verifies that an empty value is the input
// left out: no time and no error, so the caller sends no filter.
func TestParseTimeField_EmptyIsAbsent(t *testing.T) {
	got, err := ParseTimeField("created_after", "")
	if err != nil || got != nil {
		t.Errorf("ParseTimeField(\"\") = %v, %v, want nil, nil", got, err)
	}
}

// TestParseTimeField_RefusesWhatDoesNotParse verifies that a value that is
// not a timestamp is refused with the field it came from, the value and the
// forms that are accepted, rather than dropped. Dropping it is what issue 1308
// found at 84 sites: the request went out unfiltered, or a note was dated now
// instead of when it was asked to be.
func TestParseTimeField_RefusesWhatDoesNotParse(t *testing.T) {
	for _, value := range []string{"not-a-date", "2026-02-30", "01/04/2026", "2026-01-02 03:04:05", " 2026-01-02"} {
		t.Run(value, func(t *testing.T) {
			got, err := ParseTimeField("updated_before", value)
			if err == nil {
				t.Fatalf("ParseTimeField(%q) = %v, want an error", value, got)
			}
			for _, part := range []string{"updated_before", strconv.Quote(value), "2025-01-01T00:00:00Z", "2025-01-01"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("ParseTimeField(%q) error = %q, want it to name %s", value, err, part)
				}
			}
		})
	}
}

// TestParseDateField_ReadsADateAndTheDateOfATimestamp verifies the spellings
// a date-only input accepts: YYYY-MM-DD, and an RFC 3339 timestamp, whose
// calendar date is taken as written rather than in UTC, since the caller wrote
// that day and the offset only says where.
func TestParseDateField_ReadsADateAndTheDateOfATimestamp(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"bare date", "2026-12-31", "2026-12-31"},
		{"rfc3339 in UTC", "2026-12-31T00:00:00Z", "2026-12-31"},
		{"rfc3339 late in the day behind UTC", "2026-12-31T23:00:00-05:00", "2026-12-31"},
		{"local date-time", "2026-12-31T08:00:00", "2026-12-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDateField("expires_at", tt.value)
			if err != nil {
				t.Fatalf("ParseDateField(%q) error = %v", tt.value, err)
			}
			if got == nil || got.String() != tt.want {
				t.Errorf("ParseDateField(%q) = %v, want %s", tt.value, got, tt.want)
			}
		})
	}
}

// TestParseDateField_EmptyIsAbsent verifies that an empty value is the input
// left out.
func TestParseDateField_EmptyIsAbsent(t *testing.T) {
	got, err := ParseDateField("due_date", "")
	if err != nil || got != nil {
		t.Errorf("ParseDateField(\"\") = %v, %v, want nil, nil", got, err)
	}
}

// TestParseDateField_RefusesWhatDoesNotParse verifies that a value that is
// not a date is refused with the field, the value and the accepted form. An
// impossible day is refused too, which time.Parse does on its own and a
// hand-written split would not.
func TestParseDateField_RefusesWhatDoesNotParse(t *testing.T) {
	for _, value := range []string{"not-a-date", "2026-02-30", "01/04/2026", "2026/12/31", "20261231"} {
		t.Run(value, func(t *testing.T) {
			got, err := ParseDateField("due_date", value)
			if err == nil {
				t.Fatalf("ParseDateField(%q) = %v, want an error", value, got)
			}
			for _, part := range []string{"due_date", strconv.Quote(value), "YYYY-MM-DD"} {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("ParseDateField(%q) error = %q, want it to name %s", value, err, part)
				}
			}
		})
	}
}

// TestClearableDate_TellsAbsentFromEmptyFromADate verifies the three answers
// an update date takes: nothing for a field left out, an encoding of null for
// an empty string, and the date for a date. The null is checked as client-go
// would write it, since that is the value GitLab reads.
func TestClearableDate_TellsAbsentFromEmptyFromADate(t *testing.T) {
	cases := []struct {
		name  string
		value *string
		want  string
	}{
		{name: "left out", value: nil, want: "absent"},
		{name: "empty", value: new(""), want: "null"},
		{name: "date", value: new("2026-03-01"), want: `"2026-03-01"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClearableDate("due_date", tc.value)
			if err != nil {
				t.Fatalf("ClearableDate() error: %v", err)
			}
			encoded := "absent"
			if got != nil {
				raw, marshalErr := json.Marshal(got)
				if marshalErr != nil {
					t.Fatalf("json.Marshal() error: %v", marshalErr)
				}
				encoded = string(raw)
			}
			if encoded != tc.want {
				t.Errorf("ClearableDate() encodes as %s, want %s", encoded, tc.want)
			}
		})
	}
}

// TestClearableDate_RefusesWhatDoesNotParse verifies that a value that is
// neither empty nor a date is refused naming the field.
func TestClearableDate_RefusesWhatDoesNotParse(t *testing.T) {
	got, err := ClearableDate("due_date", new("tomorrow"))
	if err == nil || !strings.Contains(err.Error(), "due_date") {
		t.Errorf("ClearableDate(tomorrow) = %v, %v, want an error naming due_date", got, err)
	}
}

// TestClearableDateInputs_FillsEachDestinationAndStopsAtTheFirstRefusal
// verifies that every input writes its destination, a field left out writing
// nil over whatever was there, and that a refusal leaves the inputs after it
// unread.
func TestClearableDateInputs_FillsEachDestinationAndStopsAtTheFirstRefusal(t *testing.T) {
	stale := gl.ISOTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	start, due := &stale, &stale
	err := ClearableDateInputs(
		ClearableDateInput{Field: "start_date", Value: nil, Dest: &start},
		ClearableDateInput{Field: "due_date", Value: new(""), Dest: &due},
	)
	if err != nil {
		t.Fatalf("ClearableDateInputs() error: %v", err)
	}
	if start != nil {
		t.Errorf("start = %v, want nil for a field left out", start)
	}
	if due == nil || !time.Time(*due).IsZero() {
		t.Errorf("due = %v, want the zero date that encodes as null", due)
	}

	untouched := &stale
	err = ClearableDateInputs(
		ClearableDateInput{Field: "start_date", Value: new("soon"), Dest: &start},
		ClearableDateInput{Field: "due_date", Value: new("2026-03-01"), Dest: &untouched},
	)
	if err == nil || !strings.Contains(err.Error(), "start_date") {
		t.Fatalf("ClearableDateInputs() error = %v, want one naming start_date", err)
	}
	if untouched != &stale {
		t.Errorf("due_date was read after the refusal, got %v", untouched)
	}
}

// TestParseTimeInputs_FillsEachDestinationAndStopsAtTheFirstRefusal verifies
// the batch form the list filters use: every non-empty input lands in its
// destination, an empty one leaves its destination alone, and the first value
// that does not parse refuses the whole call before a later one is read.
func TestParseTimeInputs_FillsEachDestinationAndStopsAtTheFirstRefusal(t *testing.T) {
	t.Run("all parse", func(t *testing.T) {
		var after, before, untouched *time.Time
		err := ParseTimeInputs(
			TimeInput{Field: "created_after", Value: "2026-01-01", Dest: &after},
			TimeInput{Field: "created_before", Value: "2026-02-01T00:00:00Z", Dest: &before},
			TimeInput{Field: "updated_after", Value: "", Dest: &untouched},
		)
		if err != nil {
			t.Fatalf("ParseTimeInputs() error = %v", err)
		}
		if after == nil || !after.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("created_after = %v, want 2026-01-01 midnight UTC", after)
		}
		if before == nil || !before.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("created_before = %v, want 2026-02-01 midnight UTC", before)
		}
		if untouched != nil {
			t.Errorf("updated_after = %v, want nil for an empty value", untouched)
		}
	})
	t.Run("first refusal wins", func(t *testing.T) {
		var first, second *time.Time
		err := ParseTimeInputs(
			TimeInput{Field: "created_after", Value: "garbage", Dest: &first},
			TimeInput{Field: "created_before", Value: "also-garbage", Dest: &second},
		)
		if err == nil || !strings.Contains(err.Error(), "created_after") || strings.Contains(err.Error(), "created_before") {
			t.Errorf("ParseTimeInputs() error = %v, want the refusal of created_after alone", err)
		}
		if first != nil || second != nil {
			t.Errorf("destinations = %v, %v, want both untouched after a refusal", first, second)
		}
	})
}

// TestParseDateInputs_FillsEachDestinationAndStopsAtTheFirstRefusal verifies
// the batch form for date-only inputs.
func TestParseDateInputs_FillsEachDestinationAndStopsAtTheFirstRefusal(t *testing.T) {
	t.Run("all parse", func(t *testing.T) {
		var start, due, untouched *gl.ISOTime
		err := ParseDateInputs(
			DateInput{Field: "start_date", Value: "2026-01-01", Dest: &start},
			DateInput{Field: "due_date", Value: "2026-02-01T10:00:00Z", Dest: &due},
			DateInput{Field: "end_date", Value: "", Dest: &untouched},
		)
		if err != nil {
			t.Fatalf("ParseDateInputs() error = %v", err)
		}
		if start == nil || start.String() != "2026-01-01" {
			t.Errorf("start_date = %v, want 2026-01-01", start)
		}
		if due == nil || due.String() != "2026-02-01" {
			t.Errorf("due_date = %v, want 2026-02-01", due)
		}
		if untouched != nil {
			t.Errorf("end_date = %v, want nil for an empty value", untouched)
		}
	})
	t.Run("first refusal wins", func(t *testing.T) {
		var first, second *gl.ISOTime
		err := ParseDateInputs(
			DateInput{Field: "start_date", Value: "2026-02-30", Dest: &first},
			DateInput{Field: "due_date", Value: "nope", Dest: &second},
		)
		if err == nil || !strings.Contains(err.Error(), "start_date") || strings.Contains(err.Error(), "due_date") {
			t.Errorf("ParseDateInputs() error = %v, want the refusal of start_date alone", err)
		}
		if first != nil || second != nil {
			t.Errorf("destinations = %v, %v, want both untouched after a refusal", first, second)
		}
	})
}

// TestParseDateFieldString_WritesTheDateAClientGoStringFieldCarries verifies
// the string form for the client-go options that carry a date as a plain
// string: a date or a timestamp becomes YYYY-MM-DD, an empty value stays
// absent, and anything else is refused in [ParseDateField]'s words.
func TestParseDateFieldString_WritesTheDateAClientGoStringFieldCarries(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
	}{
		{"date", "2026-12-31", "2026-12-31"},
		{"timestamp_read_for_the_day_it_names", "2026-12-31T23:00:00-05:00", "2026-12-31"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDateFieldString("expires_at", tc.value)
			if err != nil || got == nil || *got != tc.want {
				t.Errorf("ParseDateFieldString(%q) = %v, %v; want %q", tc.value, got, err, tc.want)
			}
		})
	}
	t.Run("empty_is_absent", func(t *testing.T) {
		got, err := ParseDateFieldString("expires_at", "")
		if got != nil || err != nil {
			t.Errorf("ParseDateFieldString(\"\") = %v, %v; want nil, nil", got, err)
		}
	})
	t.Run("refused", func(t *testing.T) {
		got, err := ParseDateFieldString("expires_at", "31/12/2026")
		if got != nil || err == nil || !strings.Contains(err.Error(), `expires_at must be a date in YYYY-MM-DD form such as 2025-01-31, got "31/12/2026"`) {
			t.Errorf("ParseDateFieldString(31/12/2026) = %v, %v; want a refusal naming the field and quoting the value", got, err)
		}
	})
}

// TestTimestampParam_NilIsNoOption verifies that a timestamp left out gives
// no request option at all, so the request goes out exactly as client-go
// encoded it.
func TestTimestampParam_NilIsNoOption(t *testing.T) {
	if TimestampParam("expires_at", nil) != nil {
		t.Error("TimestampParam(nil) returned an option, want none")
	}
}

// timestampParamInstant is the instant the TimestampParam tests send, written
// with an offset so a test can tell it was not moved to UTC or truncated to
// its day: client-go carries these parameters as dates, which reach GitLab as
// midnight UTC, so an SSH key asked to expire at 23:00 expired 23 hours early
// (register row 126).
const timestampParamInstant = "2026-12-31T23:00:00-05:00"

// applyTimestampParam builds a request the way client-go would, with the given
// method, URL and body, and applies TimestampParam for name to it.
func applyTimestampParam(t *testing.T, method, url string, body any, name string) *retryablehttp.Request {
	t.Helper()
	at, err := time.Parse(time.RFC3339, timestampParamInstant)
	if err != nil {
		t.Fatalf("time.Parse: %v", err)
	}
	req, err := retryablehttp.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err = TimestampParam(name, &at)(req); err != nil {
		t.Fatalf("option: %v", err)
	}
	return req
}

// TestTimestampParam_BodyMethods_WriteTheInstantIntoTheBody verifies the
// option writes the timestamp, with the offset it was given in, into the JSON
// body of a POST, PUT or PATCH, which is where client-go encodes their
// options, replacing the date client-go wrote under the same name and leaving
// every other key as it was, a number too large for a float64 included.
func TestTimestampParam_BodyMethods_WriteTheInstantIntoTheBody(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := applyTimestampParam(t, method, "https://gitlab.example.com/api/v4/user/keys",
				[]byte(`{"title":"k","expires_at":"2026-12-31","count":12345678901234567890}`), "expires_at")
			raw, err := req.BodyBytes()
			if err != nil {
				t.Fatalf("BodyBytes: %v", err)
			}
			var body map[string]json.RawMessage
			if err = json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode body %s: %v", raw, err)
			}
			want := map[string]string{"expires_at": `"` + timestampParamInstant + `"`, "title": `"k"`, "count": "12345678901234567890"}
			if len(body) != len(want) {
				t.Errorf("body %s carries %d keys, want %d", raw, len(body), len(want))
			}
			for key, value := range want {
				if string(body[key]) != value {
					t.Errorf("body %s = %s, want %s (body %s)", key, body[key], value, raw)
				}
			}
			if req.URL.RawQuery != "" {
				t.Errorf("query = %q, want none on a %s", req.URL.RawQuery, method)
			}
		})
	}
}

// TestTimestampParam_EmptyBody_GainsTheTimestampAlone verifies a POST whose
// body client-go left empty is sent as an object holding the timestamp.
func TestTimestampParam_EmptyBody_GainsTheTimestampAlone(t *testing.T) {
	req := applyTimestampParam(t, http.MethodPost, "https://gitlab.example.com/api/v4/user/keys", nil, "expires_at")
	raw, err := req.BodyBytes()
	if err != nil || string(raw) != `{"expires_at":"`+timestampParamInstant+`"}` {
		t.Errorf("body = %s, %v; want the timestamp alone", raw, err)
	}
}

// TestTimestampParam_OtherMethods_WriteTheInstantIntoTheQuery verifies the
// option writes the timestamp into the query of a GET or DELETE, where
// client-go encodes their options, replacing the date client-go wrote and
// keeping every other parameter.
func TestTimestampParam_OtherMethods_WriteTheInstantIntoTheQuery(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := applyTimestampParam(t, method,
				"https://gitlab.example.com/api/v4/snippets/all?created_after=2026-12-31&repository_storage=nfs-01", nil, "created_after")
			query := req.URL.Query()
			if got := query["created_after"]; len(got) != 1 || got[0] != timestampParamInstant {
				t.Errorf("created_after = %q, want only %q", got, timestampParamInstant)
			}
			if got := query.Get("repository_storage"); got != "nfs-01" {
				t.Errorf("repository_storage = %q, want it kept", got)
			}
		})
	}
}

// TestTimestampParam_UnreadableBody_IsAnError verifies a body the option
// cannot read or decode fails the request rather than going out with the
// date client-go wrote in place of the instant the caller asked for.
func TestTimestampParam_UnreadableBody_IsAnError(t *testing.T) {
	at := time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)
	t.Run("not_JSON", func(t *testing.T) {
		req, err := retryablehttp.NewRequest(http.MethodPost, "https://gitlab.example.com/api/v4/user/keys", []byte("not json"))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err = TimestampParam("expires_at", &at)(req); err == nil || !strings.Contains(err.Error(), "expires_at") {
			t.Errorf("option error = %v, want a refusal naming expires_at", err)
		}
	})
	t.Run("reader_fails", func(t *testing.T) {
		// retryablehttp calls the reader once when the request is built, to
		// size it, so the reader fails from its second call on.
		calls := 0
		req, err := retryablehttp.NewRequest(http.MethodPost, "https://gitlab.example.com/api/v4/user/keys",
			retryablehttp.ReaderFunc(func() (io.Reader, error) {
				calls++
				if calls > 1 {
					return nil, errors.New("gone")
				}
				return strings.NewReader("{}"), nil
			}))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err = TimestampParam("expires_at", &at)(req); err == nil || !strings.Contains(err.Error(), "gone") {
			t.Errorf("option error = %v, want the reader's error", err)
		}
	})
}

// TestFormatTime_DateOnly verifies that FormatTime handles YYYY-MM-DD format.
func TestFormatTime_DateOnly(t *testing.T) {
	got := FormatTime("2026-03-20")
	want := "20 Mar 2026"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}

// TestFormatTimePtr_Nil verifies that FormatTimePtr returns "" for a nil pointer.
func TestFormatTimePtr_Nil(t *testing.T) {
	got := FormatTimePtr(nil)
	if got != "" {
		t.Errorf("FormatTimePtr(nil) = %q, want \"\"", got)
	}
}

// TestFormatTimePtr_Valid verifies that FormatTimePtr formats a non-nil time as RFC 3339.
func TestFormatTimePtr_Valid(t *testing.T) {
	ts := time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC)
	got := FormatTimePtr(&ts)
	want := "2026-03-20T15:45:00Z"
	if got != want {
		t.Errorf("FormatTimePtr() = %q, want %q", got, want)
	}
}

// TestRFC3339_WireFormInUTC verifies that RFC3339 writes the wire form GitLab
// itself uses: RFC 3339 in UTC, whatever zone the value carries, and nothing
// for a zero time, which is what a field GitLab did not send decodes to and
// would otherwise read as the year one.
func TestRFC3339_WireFormInUTC(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "utc", in: time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC), want: "2026-03-20T15:45:00Z"},
		{name: "offset is converted", in: time.Date(2026, 3, 20, 10, 45, 0, 0, time.FixedZone("EST", -5*3600)), want: "2026-03-20T15:45:00Z"},
		{name: "zero time is absent", in: time.Time{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RFC3339(tt.in); got != tt.want {
				t.Errorf("RFC3339(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRFC3339Ptr_NilIsAbsent verifies that the pointer form writes nothing
// for nil and the wire form otherwise, and that the deprecated FormatTimePtr
// is the same function under its old name, so a caller not yet migrated
// renders what it did.
func TestRFC3339Ptr_NilIsAbsent(t *testing.T) {
	ts := time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC)
	tests := []struct {
		name string
		in   *time.Time
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "value", in: &ts, want: "2026-03-20T15:45:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RFC3339Ptr(tt.in); got != tt.want {
				t.Errorf("RFC3339Ptr() = %q, want %q", got, tt.want)
			}
			if got := FormatTimePtr(tt.in); got != tt.want {
				t.Errorf("FormatTimePtr() = %q, want %q (the deprecated alias must agree)", got, tt.want)
			}
		})
	}
}

// TestFormatTimeValue_DisplayForm verifies that a time.Time renders in the
// same display form FormatTime gives the wire string, in UTC, and that a zero
// time is absent rather than the year one.
func TestFormatTimeValue_DisplayForm(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "utc", in: time.Date(2026, 3, 20, 15, 45, 0, 0, time.UTC), want: "20 Mar 2026 15:45 UTC"},
		{name: "offset is converted", in: time.Date(2026, 3, 20, 10, 45, 0, 0, time.FixedZone("EST", -5*3600)), want: "20 Mar 2026 15:45 UTC"},
		{name: "zero time is absent", in: time.Time{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatTimeValue(tt.in); got != tt.want {
				t.Errorf("FormatTimeValue(%v) = %q, want %q", tt.in, got, tt.want)
			}
			if wire := RFC3339(tt.in); FormatTime(wire) != tt.want {
				t.Errorf("FormatTime(RFC3339(%v)) = %q, want %q: the two display paths disagree", tt.in, FormatTime(wire), tt.want)
			}
		})
	}
}

// TestFormatISOTimePtr_Nil verifies that FormatISOTimePtr returns "" for a nil pointer.
func TestFormatISOTimePtr_Nil(t *testing.T) {
	got := FormatISOTimePtr(nil)
	if got != "" {
		t.Errorf("FormatISOTimePtr(nil) = %q, want \"\"", got)
	}
}

// TestFormatISOTimePtr_Valid verifies that FormatISOTimePtr formats a non-nil ISOTime as YYYY-MM-DD.
func TestFormatISOTimePtr_Valid(t *testing.T) {
	iso := gl.ISOTime(time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC))
	got := FormatISOTimePtr(&iso)
	want := "2026-03-20"
	if got != want {
		t.Errorf("FormatISOTimePtr() = %q, want %q", got, want)
	}
}

// formatTimePtrFuncDef matches a top-level Go function definition of
// formatTimePtr (with any return type). Used by the uniqueness guardrail.
var formatTimePtrFuncDef = regexp.MustCompile(`^func\s+formatTimePtr\b`)

// formatISOTimePtrFuncDef matches a top-level Go function definition of
// formatISOTimePtr (with any return type). Used by the uniqueness guardrail.
var formatISOTimePtrFuncDef = regexp.MustCompile(`^func\s+formatISOTimePtr\b`)

// timeHelperScan holds the result of the single shared repository sweep the
// two DEDUP-003 guardrail tests consume: offending "path:lineno" locations
// per helper regex. The sweep is repo-wide but runs once (sync.Once) and
// prunes what is not source of ours (node_modules, dist, the Astro site, and
// through sourcewalk every dot-directory and nested checkout), which
// previously dominated the walk with tens of thousands of irrelevant entries.
type timeHelperScan struct {
	formatTimePtr    []string
	formatISOTimePtr []string
	err              error
}

var (
	timeHelperScanOnce   sync.Once
	timeHelperScanResult timeHelperScan
)

// findDuplicateTimeHelper returns the duplicate-definition hits for re,
// excluding the canonical home (this package's source directory). Both
// guardrail regexes are matched in one shared walk.
func findDuplicateTimeHelper(t *testing.T, re *regexp.Regexp, skipDir string) []string {
	t.Helper()
	timeHelperScanOnce.Do(func() { timeHelperScanResult = scanRepoForTimeHelpers(t, skipDir) })
	if timeHelperScanResult.err != nil {
		t.Fatalf("walk repo: %v", timeHelperScanResult.err)
	}
	if re == formatISOTimePtrFuncDef {
		return timeHelperScanResult.formatISOTimePtr
	}
	return timeHelperScanResult.formatTimePtr
}

// scanRepoForTimeHelpers performs the single repository sweep, collecting the
// hits for both helper regexes at once.
func scanRepoForTimeHelpers(t *testing.T, skipDir string) timeHelperScan {
	t.Helper()
	return scanTreeForTimeHelpers(findRepoRoot(t), skipDir)
}

// scanTreeForTimeHelpers sweeps the tree at repoRoot for both helper regexes.
//
// It takes its root as an argument, and reports rather than aborts, so that
// [TestScanTreeForTimeHelpers_ANestedCheckout_IsNotCounted] can drive it over
// a tree it plants. That is the only reason it is separate from the caller
// above: what this sweep must not do cannot be shown against the repository
// itself, because the thing it must not read is whatever an agent happens to
// have checked out at the time.
func scanTreeForTimeHelpers(repoRoot, skipDir string) timeHelperScan {
	// These names are this sweep's own disinterest: generated output and the
	// built site hold no Go source of ours. What is not this repository's
	// source at all is sourcewalk's rule rather than this list's: a
	// dot-directory, and any nested checkout, which on a machine running the
	// parallel-agent tooling means the hundred and more worktrees under
	// .claude. That is why .git is not named here either.
	prunedDirs := map[string]struct{}{
		"node_modules": {}, "dist": {}, "site": {},
	}
	var scan timeHelperScan
	// The sweep is scoped to the repository root: every open goes through the
	// root, so a path that resolves outside it, whether a symlink planted in
	// the tree or one swapped in between the walk seeing an entry and opening
	// it, is refused rather than read.
	root, openErr := os.OpenRoot(repoRoot)
	if openErr != nil {
		scan.err = openErr
		return scan
	}
	defer func() { _ = root.Close() }()
	rootFS := root.FS()
	scan.err = fs.WalkDir(rootFS, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// The rule is asked of rootFS rather than of the native path, so
			// the marker probe stays inside the root this sweep opened.
			if _, pruned := prunedDirs[d.Name()]; pruned || sourcewalk.SkipDirBelowRootFS(rootFS, rel) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		native := filepath.FromSlash(rel)
		if filepath.Join(repoRoot, filepath.Dir(native)) == skipDir {
			return nil
		}
		f, err := root.Open(native)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			if formatTimePtrFuncDef.MatchString(line) {
				scan.formatTimePtr = append(scan.formatTimePtr, native+":"+itoa(lineNo))
			}
			if formatISOTimePtrFuncDef.MatchString(line) {
				scan.formatISOTimePtr = append(scan.formatISOTimePtr, native+":"+itoa(lineNo))
			}
		}
		return nil
	})
	return scan
}

// findRepoRoot returns the absolute path to the repository root (the directory
// containing go.mod) by walking up from the test's working directory.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// itoa is a small allocation-free integer-to-string helper used by the
// guardrail to format line numbers without pulling strconv into the test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// TestFormatTimePtr_UniqueAcrossRepo is the DEDUP-003 guardrail: the canonical
// formatTimePtr lives in this package as FormatTimePtr (exported). Any other
// package that defines an unexported `formatTimePtr` re-introduces duplication
// and confuses readers — call sites should use toolutil.FormatTimePtr or a
// distinctively-named helper (e.g. formatAuditDate).
func TestFormatTimePtr_UniqueAcrossRepo(t *testing.T) {
	canonicalDir, _ := filepath.Abs(".")
	hits := findDuplicateTimeHelper(t, formatTimePtrFuncDef, canonicalDir)
	if len(hits) > 0 {
		t.Fatalf("found %d duplicate formatTimePtr definition(s) outside %s:\n  %s\n"+
			"Use toolutil.FormatTimePtr or rename the helper to a purpose-specific name.",
			len(hits), canonicalDir, strings.Join(hits, "\n  "))
	}
}

// TestFormatISOTimePtr_UniqueAcrossRepo is the DEDUP-003 guardrail for the
// ISO-time variant — see TestFormatTimePtr_UniqueAcrossRepo.
func TestFormatISOTimePtr_UniqueAcrossRepo(t *testing.T) {
	canonicalDir, _ := filepath.Abs(".")
	hits := findDuplicateTimeHelper(t, formatISOTimePtrFuncDef, canonicalDir)
	if len(hits) > 0 {
		t.Fatalf("found %d duplicate formatISOTimePtr definition(s) outside %s:\n  %s\n"+
			"Use toolutil.FormatISOTimePtr or rename the helper to a purpose-specific name.",
			len(hits), canonicalDir, strings.Join(hits, "\n  "))
	}
}

// TestScanTreeForTimeHelpers_ANestedCheckout_IsNotCounted holds the two
// guardrails above to this repository's own source.
//
// The sweep they share walks from the repository root, and a checkout can
// contain other checkouts: the parallel-agent tooling puts a git worktree per
// agent under .claude/worktrees, and a developer can put one anywhere with
// `git worktree add`. Each is a complete copy of this repository on some other
// branch, and folding one into the sweep goes wrong twice. The visible way is
// cost, which is what was found: at 198 worktrees these guardrails swept 198
// copies and the package stopped finishing inside the ten-minute test timeout.
// The way worth guarding is the quiet one. A guardrail that fails names the
// file holding the duplicate, so a copy carrying another branch's code can
// fail it against a path that does not exist in this tree, and the reader is
// sent to open a file that is not there.
//
// So the assertion is that a definition inside a nested checkout is not
// counted, rather than that the walk survives one. The ordinary directory is
// the control and is what makes the rest mean anything: without a hit that
// must be found, every assertion here would also pass on a sweep that pruned
// the whole tree and read nothing.
func TestScanTreeForTimeHelpers_ANestedCheckout_IsNotCounted(t *testing.T) {
	base := t.TempDir()
	const definitions = "package helpers\n\nfunc formatTimePtr() string { return \"\" }\n\nfunc formatISOTimePtr() string { return \"\" }\n"

	// sequential: setup steps building one fixture tree, asserted by the cases below
	plant := func(dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(base, dir), 0o750); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(base, dir, "helpers.go"), []byte(definitions), 0o600); err != nil {
			t.Fatalf("WriteFile(%s/helpers.go) error = %v", dir, err)
		}
	}
	// The control: ordinary source of ours, which must be found.
	plant("ours")
	// A linked worktree in the shape the agent tooling leaves behind. Either
	// rule alone excludes it: it sits under a dot-directory, and it carries a
	// .git file of its own.
	worktree := filepath.Join(".claude", "worktrees", "agent")
	plant(worktree)
	if err := os.WriteFile(filepath.Join(base, worktree, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%s/.git) error = %v", worktree, err)
	}
	// A checkout whose name gives nothing away, which `git worktree add
	// ./scratch` produces. Only the marker rule excludes this one, so it is
	// what would still be read if the fix had been a name in a skip list.
	plant("scratch")
	if err := os.MkdirAll(filepath.Join(base, "scratch", ".git"), 0o750); err != nil {
		t.Fatalf("MkdirAll(scratch/.git) error = %v", err)
	}

	scan := scanTreeForTimeHelpers(base, "")
	if scan.err != nil {
		t.Fatalf("scanTreeForTimeHelpers() error = %v, want nil", scan.err)
	}

	want := filepath.Join("ours", "helpers.go")
	cases := []struct {
		name string
		hits []string
	}{
		{name: "formatTimePtr", hits: scan.formatTimePtr},
		{name: "formatISOTimePtr", hits: scan.formatISOTimePtr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.hits) != 1 {
				t.Fatalf("%s: got %d hit(s) %v, want the one in %s alone: either a nested checkout was counted, or the walk read nothing at all",
					tc.name, len(tc.hits), tc.hits, want)
			}
			if !strings.HasPrefix(tc.hits[0], want+":") {
				t.Errorf("%s: got hit %q, want one in %s", tc.name, tc.hits[0], want)
			}
		})
	}
}
