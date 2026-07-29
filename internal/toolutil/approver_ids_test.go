// approver_ids_test.go contains unit tests for the approver filter that
// mirrors GitLab's approver_ids / approved_by_ids parameters, covering the
// numeric-ID form, the "Any"/"None" literals, and the invalid combinations
// that must be rejected rather than silently altering the filter.
package toolutil

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// encodeApproverFilter renders a filter the way the SDK would encode it onto a
// request, so tests assert on the wire form rather than on unexported state.
func encodeApproverFilter(t *testing.T, filter ApproverIDsFilter) url.Values {
	t.Helper()
	value, err := filter.ApproverIDsValue()
	if err != nil {
		t.Fatalf("ApproverIDsValue() unexpected error: %v", err)
	}
	values := url.Values{}
	if value == nil {
		return values
	}
	if encodeErr := value.EncodeValues("approver_ids", &values); encodeErr != nil {
		t.Fatalf("EncodeValues() error: %v", encodeErr)
	}
	return values
}

// TestApproverIDsFilter_Encoding verifies the wire form produced for each
// accepted input shape.
func TestApproverIDsFilter_Encoding(t *testing.T) {
	tests := []struct {
		name   string
		filter ApproverIDsFilter
		want   url.Values
	}{
		{
			name:   "empty filter encodes nothing",
			filter: nil,
			want:   url.Values{},
		},
		{
			name:   "numeric IDs encode as an indexed list",
			filter: ApproverIDsFilter{"11", "12"},
			want:   url.Values{"approver_ids[]": {"11", "12"}},
		},
		{
			name:   "Any encodes as a bare literal",
			filter: ApproverIDsFilter{"Any"},
			want:   url.Values{"approver_ids": {"Any"}},
		},
		{
			name:   "None encodes as a bare literal",
			filter: ApproverIDsFilter{"None"},
			want:   url.Values{"approver_ids": {"None"}},
		},
		{
			name:   "literals are normalized to GitLab's capitalization",
			filter: ApproverIDsFilter{"none"},
			want:   url.Values{"approver_ids": {"None"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeApproverFilter(t, tt.filter)
			if len(got) != len(tt.want) {
				t.Fatalf("encoded = %v, want %v", got, tt.want)
			}
			for key, want := range tt.want {
				if gotValues, ok := got[key]; !ok || strings.Join(gotValues, ",") != strings.Join(want, ",") {
					t.Errorf("encoded[%q] = %v, want %v", key, got[key], want)
				}
			}
		})
	}
}

// TestApproverIDsFilter_Rejections verifies that ambiguous or unparseable
// filters fail loudly. Silently dropping an entry would return a different
// result set than the caller asked for.
func TestApproverIDsFilter_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		filter  ApproverIDsFilter
		wantErr string
	}{
		{
			name:    "literal first cannot be combined with IDs",
			filter:  ApproverIDsFilter{"None", "7"},
			wantErr: "must be the only value",
		},
		{
			name:    "literal after IDs cannot be combined either",
			filter:  ApproverIDsFilter{"7", "Any"},
			wantErr: "must be the only value",
		},
		{
			name:    "non-numeric non-literal is rejected",
			filter:  ApproverIDsFilter{"alice"},
			wantErr: "is not a user ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.filter.ApproverIDsValue()
			if err == nil {
				t.Fatalf("ApproverIDsValue() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("ApproverIDsValue() error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestApproverIDsFilter_UnmarshalsNumbersAndStrings verifies that the filter
// accepts both JSON forms, which is what lets the widened input schema stay
// backward compatible with callers sending numeric IDs.
func TestApproverIDsFilter_UnmarshalsNumbersAndStrings(t *testing.T) {
	var numeric ApproverIDsFilter
	if err := json.Unmarshal([]byte(`[11, 12]`), &numeric); err != nil {
		t.Fatalf("unmarshal numeric: %v", err)
	}
	if got := encodeApproverFilter(t, numeric); strings.Join(got["approver_ids[]"], ",") != "11,12" {
		t.Errorf("numeric filter encoded = %v, want ids 11,12", got)
	}

	var literal ApproverIDsFilter
	if err := json.Unmarshal([]byte(`["None"]`), &literal); err != nil {
		t.Fatalf("unmarshal literal: %v", err)
	}
	if got := encodeApproverFilter(t, literal); strings.Join(got["approver_ids"], ",") != "None" {
		t.Errorf("literal filter encoded = %v, want None", got)
	}
}
