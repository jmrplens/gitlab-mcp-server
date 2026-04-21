// string_or_int.go defines the StringOrInt type for flexible JSON
// unmarshalling. GitLab API parameters like project_id and group_id accept
// both numeric IDs and URL-encoded paths, but LLMs often send numeric IDs as
// JSON numbers instead of strings. This type transparently handles both.

package toolutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// StringOrInt is a string type that accepts both JSON strings and JSON numbers
// during unmarshalling. It always stores the value as a string internally.
// This is needed because LLMs frequently send numeric IDs (e.g. 405) as JSON
// numbers rather than strings, even when the schema declares "type": "string".
type StringOrInt string //nolint:recvcheck // UnmarshalJSON requires pointer receiver, others are value receivers by design

// String returns the underlying string value.
func (s StringOrInt) String() string {
	return string(s)
}

// UnmarshalJSON implements [json.Unmarshaler] to accept both JSON strings
// (e.g. "405", "group/project") and JSON numbers (e.g. 405, 42.0).
func (s *StringOrInt) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	// Try string first (most common expected case).
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = StringOrInt(str)
		return nil
	}

	// Try number (LLMs often send numeric IDs as JSON numbers).
	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		// If it's an integer, format without decimals.
		if _, err = strconv.ParseInt(num.String(), 10, 64); err == nil {
			*s = StringOrInt(num.String())
			return nil
		}
		// If it's a float, convert to int (e.g. 405.0 -> "405").
		var f float64
		if f, err = strconv.ParseFloat(num.String(), 64); err == nil {
			*s = StringOrInt(strconv.FormatInt(int64(f), 10))
			return nil
		}
	}

	return fmt.Errorf("StringOrInt: cannot unmarshal %s into string or number", string(data))
}

// MarshalJSON implements [json.Marshaler] to always output a JSON string.
func (s StringOrInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// Int64 parses the stored string as a base-10 integer and returns it.
// Returns 0 and an error if the value is empty or not a valid integer.
func (s StringOrInt) Int64() (int64, error) {
	if s == "" {
		return 0, errors.New("StringOrInt: empty value")
	}
	return strconv.ParseInt(string(s), 10, 64)
}
