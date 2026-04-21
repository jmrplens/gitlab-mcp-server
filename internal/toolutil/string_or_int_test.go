// string_or_int_test.go contains unit tests for the StringOrInt JSON
// unmarshaling type.
package toolutil

import (
	"encoding/json"
	"testing"
)

const (
	testGroupProject = "group/project"
	fmtUnexpectedErr = "unexpected error: %v"
)

// TestStringOrInt_UnmarshalJSON verifies that [StringOrInt] correctly unmarshals
// strings, integers, floats, null, and rejects unsupported JSON types.
func TestStringOrInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    StringOrInt
		wantErr bool
	}{
		{
			name:  "string value",
			input: `"405"`,
			want:  StringOrInt("405"),
		},
		{
			name:  "string path",
			input: `"group/project"`,
			want:  StringOrInt(testGroupProject),
		},
		{
			name:  "string url-encoded path",
			input: `"group%2Fproject"`,
			want:  StringOrInt("group%2Fproject"),
		},
		{
			name:  "integer value",
			input: `405`,
			want:  StringOrInt("405"),
		},
		{
			name:  "zero integer",
			input: `0`,
			want:  StringOrInt("0"),
		},
		{
			name:  "large integer",
			input: `123456789`,
			want:  StringOrInt("123456789"),
		},
		{
			name:  "float with zero decimal",
			input: `405.0`,
			want:  StringOrInt("405"),
		},
		{
			name:  "null value",
			input: `null`,
			want:  StringOrInt(""),
		},
		{
			name:  "empty string",
			input: `""`,
			want:  StringOrInt(""),
		},
		{
			name:    "boolean value",
			input:   `true`,
			wantErr: true,
		},
		{
			name:    "array value",
			input:   `[1,2,3]`,
			wantErr: true,
		},
		{
			name:    "object value",
			input:   `{"key":"val"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got StringOrInt
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %s, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStringOrInt_MarshalJSON verifies that [StringOrInt] marshals back to a
// JSON string value regardless of the original input type.
func TestStringOrInt_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input StringOrInt
		want  string
	}{
		{
			name:  "numeric string",
			input: StringOrInt("405"),
			want:  `"405"`,
		},
		{
			name:  "path string",
			input: StringOrInt(testGroupProject),
			want:  `"group/project"`,
		},
		{
			name:  "empty string",
			input: StringOrInt(""),
			want:  `""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if string(got) != tt.want {
				t.Errorf("got %s, want %s", string(got), tt.want)
			}
		})
	}
}

// TestStringOrInt_String verifies that the [StringOrInt.String] method
// returns the stored string representation.
func TestStringOrInt_String(t *testing.T) {
	s := StringOrInt("405")
	if s.String() != "405" {
		t.Errorf("got %q, want %q", s.String(), "405")
	}
}

// TestStringOrInt_InStruct verifies that StringOrInt works correctly when
// embedded inside a struct, which is the real-world usage pattern.
func TestStringOrInt_InStruct(t *testing.T) {
	type input struct {
		ProjectID StringOrInt `json:"project_id"`
		Name      string      `json:"name"`
	}

	tests := []struct {
		name    string
		json    string
		wantID  StringOrInt
		wantErr bool
	}{
		{
			name:   "project_id as string",
			json:   `{"project_id": "405", "name": "test"}`,
			wantID: StringOrInt("405"),
		},
		{
			name:   "project_id as number",
			json:   `{"project_id": 405, "name": "test"}`,
			wantID: StringOrInt("405"),
		},
		{
			name:   "project_id as path",
			json:   `{"project_id": "group/project", "name": "test"}`,
			wantID: StringOrInt("group/project"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got input
			err := json.Unmarshal([]byte(tt.json), &got)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got.ProjectID != tt.wantID {
				t.Errorf("ProjectID = %q, want %q", got.ProjectID, tt.wantID)
			}
		})
	}
}

// TestStringOrInt_Int64 verifies that [StringOrInt.Int64] parses numeric
// string values to int64 and returns an error for non-numeric strings.
func TestStringOrInt_Int64(t *testing.T) {
	tests := []struct {
		name    string
		input   StringOrInt
		want    int64
		wantErr bool
	}{
		{"valid integer", StringOrInt("405"), 405, false},
		{"valid zero", StringOrInt("0"), 0, false},
		{"negative", StringOrInt("-1"), -1, false},
		{"empty", StringOrInt(""), 0, true},
		{"non-numeric", StringOrInt("abc"), 0, true},
		{"path", StringOrInt("group/project"), 0, true},
		{"float string", StringOrInt("3.14"), 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Int64()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf(fmtUnexpectedErr, err)
			}
			if got != tt.want {
				t.Errorf("Int64() = %d, want %d", got, tt.want)
			}
		})
	}
}
