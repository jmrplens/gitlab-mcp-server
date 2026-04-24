// time_helpers_test.go contains unit tests for time formatting and parsing helpers.

package toolutil

import "testing"

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

// TestFormatTime_InvalidFormat verifies that FormatTime returns the original
// string verbatim when it cannot be parsed as RFC 3339.
func TestFormatTime_InvalidFormat(t *testing.T) {
	input := "not-a-date"
	got := FormatTime(input)
	if got != input {
		t.Errorf("FormatTime(%q) = %q, want original input", input, got)
	}
}

// TestParseOptionalTime_Valid verifies that ParseOptionalTime correctly
// parses a valid RFC 3339 timestamp string.
func TestParseOptionalTime_Valid(t *testing.T) {
	got := ParseOptionalTime("2026-01-01T00:00:00Z")
	if got == nil {
		t.Fatal("ParseOptionalTime() returned nil for valid input")
	}
}

// TestParseOptionalTime_Empty verifies that ParseOptionalTime returns a
// zero time when given an empty string.
func TestParseOptionalTime_Empty(t *testing.T) {
	got := ParseOptionalTime("")
	if got != nil {
		t.Errorf("ParseOptionalTime(\"\") = %v, want nil", got)
	}
}

// TestParseOptionalTime_Invalid verifies that ParseOptionalTime returns a
// zero time when given an unparseable timestamp string.
func TestParseOptionalTime_Invalid(t *testing.T) {
	got := ParseOptionalTime("invalid")
	if got != nil {
		t.Errorf("ParseOptionalTime(\"invalid\") = %v, want nil", got)
	}
}

// TestFormatTime_DateOnly verifies that FormatTime handles YYYY-MM-DD format.
func TestFormatTime_DateOnly(t *testing.T) {
	got := FormatTime("2026-03-20")
	want := "20 Mar 2026"
	if got != want {
		t.Errorf("FormatTime() = %q, want %q", got, want)
	}
}
