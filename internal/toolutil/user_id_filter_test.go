package toolutil

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

// queryEncoder is the part of client-go's filter values that writes them into
// a query.
type queryEncoder interface {
	EncodeValues(key string, values *url.Values) error
}

// encodedUserIDFilter renders a filter value the way client-go puts it in a
// query, which is what GitLab reads.
func encodedUserIDFilter(t *testing.T, value queryEncoder) string {
	t.Helper()
	values := url.Values{}
	if err := value.EncodeValues("assignee_id", &values); err != nil {
		t.Fatalf("EncodeValues() error: %v", err)
	}
	return values.Get("assignee_id")
}

// TestAssigneeAndReviewerFilter_ReadAUserIDNoneOrAny holds both filters to
// what GitLab's assignee_id and reviewer_id take: a numeric user ID, and None
// and Any in any case, sent in the spelling GitLab compares, and to no value
// at all for a filter left out.
func TestAssigneeAndReviewerFilter_ReadAUserIDNoneOrAny(t *testing.T) {
	tests := []struct {
		name  string
		value StringOrInt
		want  string
	}{
		{name: "a user ID", value: "42", want: "42"},
		{name: "None", value: "None", want: "None"},
		{name: "any in lower case", value: " any ", want: "Any"},
		{name: "NONE in upper case", value: "NONE", want: "None"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignee, err := AssigneeFilter("assignee_id", tt.value)
			if err != nil {
				t.Fatalf("AssigneeFilter() error: %v", err)
			}
			reviewer, err := ReviewerFilter("assignee_id", tt.value)
			if err != nil {
				t.Fatalf("ReviewerFilter() error: %v", err)
			}
			if got := encodedUserIDFilter(t, assignee); got != tt.want {
				t.Errorf("AssigneeFilter() encodes as %q, want %q", got, tt.want)
			}
			if got := encodedUserIDFilter(t, reviewer); got != tt.want {
				t.Errorf("ReviewerFilter() encodes as %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("left out", func(t *testing.T) {
		assignee, err := AssigneeFilter("assignee_id", "")
		reviewer, reviewerErr := ReviewerFilter("reviewer_id", "")
		if assignee != nil || reviewer != nil || err != nil || reviewerErr != nil {
			t.Errorf("filters = %v, %v, %v, %v, want no value and no error", assignee, reviewer, err, reviewerErr)
		}
	})
}

// TestAssigneeAndReviewerFilter_RefuseWhatNoUserIs holds both filters to
// refusing, naming the field, 0, which no user has and GitLab answers with
// nothing or with everything depending on the route, with a pointer to None,
// and a negative ID or any other text with the values they take.
func TestAssigneeAndReviewerFilter_RefuseWhatNoUserIs(t *testing.T) {
	tests := []struct {
		name    string
		value   StringOrInt
		refusal string
	}{
		{name: "zero", value: "0", refusal: `reviewer_id 0 matches no user: pass "None"`},
		{name: "negative", value: "-3", refusal: `reviewer_id must be a user ID, "None" or "Any", got "-3"`},
		{name: "a name", value: "alice", refusal: `reviewer_id must be a user ID, "None" or "Any", got "alice"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assignee, err := AssigneeFilter("reviewer_id", tt.value)
			if assignee != nil || err == nil || !strings.Contains(err.Error(), tt.refusal) {
				t.Errorf("AssigneeFilter() = %v, %v, want a refusal containing %q", assignee, err, tt.refusal)
			}
			reviewer, err := ReviewerFilter("reviewer_id", tt.value)
			if reviewer != nil || err == nil || !strings.Contains(err.Error(), tt.refusal) {
				t.Errorf("ReviewerFilter() = %v, %v, want a refusal containing %q", reviewer, err, tt.refusal)
			}
		})
	}
}

// TestInt64UserIDFilter_SendsAnIDThroughTheOptionAndALiteralThroughTheQuery
// holds the form for an option struct that types the filter *int64: an ID goes
// into the option and no request option is needed, None and Any are written
// into the query by the request option, and a refusal and an empty value
// return neither.
func TestInt64UserIDFilter_SendsAnIDThroughTheOptionAndALiteralThroughTheQuery(t *testing.T) {
	id, option, err := Int64UserIDFilter("assignee_id", "42")
	if err != nil || id == nil || *id != 42 || option != nil {
		t.Errorf("an ID: %v, option set %v, %v, want 42 and no option", id, option != nil, err)
	}
	if id, option, err = Int64UserIDFilter("assignee_id", ""); id != nil || option != nil || err != nil {
		t.Errorf("left out: %v, option set %v, %v, want nothing", id, option != nil, err)
	}
	if id, option, err = Int64UserIDFilter("assignee_id", "0"); err == nil || id != nil || option != nil {
		t.Errorf("zero: %v, option set %v, %v, want a refusal and nothing else", id, option != nil, err)
	}

	id, option, err = Int64UserIDFilter("assignee_id", "any")
	if err != nil || id != nil || option == nil {
		t.Fatalf("Any: %v, option set %v, %v, want no ID and an option", id, option != nil, err)
	}
	req, err := retryablehttp.NewRequestWithContext(context.Background(), http.MethodGet, "https://gitlab.example.com/api/v4/issues_statistics?labels=bug", nil)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	if err = option(req); err != nil {
		t.Fatalf("option error: %v", err)
	}
	query := req.URL.Query()
	if query.Get("assignee_id") != "Any" || query.Get("labels") != "bug" {
		t.Errorf("query = %q, want assignee_id Any beside the labels already there", req.URL.RawQuery)
	}
}
