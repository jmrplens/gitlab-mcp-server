// referenced_test.go covers the row a merge request's closes-issues and
// related-issues listings answer with, read off a captured answer the way the
// two merge request handlers read it.
package issues

import (
	"encoding/json"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// referencedRowsBody is a page carrying an issue of this instance and an
// issue of an external tracker, as lib/api/merge_requests.rb renders them in
// one array: IssueBasic for the first, API::Entities::ExternalIssue for the
// second.
const referencedRowsBody = `[` +
	`{"id":10,"iid":5,"title":"Bug fix","state":"opened","type":"ISSUE","start_date":"2026-01-02"},` +
	`{"title":"External Issue PROJ-7","id":"PROJ-7"}` +
	`]`

// decodeIssues decodes body the way client-go decodes a page of issues, so
// the external row's string id lands in ExternalID through Issue.UnmarshalJSON.
func decodeIssues(t *testing.T, body string) []*gl.Issue {
	t.Helper()
	var list []*gl.Issue
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode the page: %v", err)
	}
	return list
}

// TestToReferencedOutputs_CarriesEachRowsOwnIdentifier checks that each row
// keeps what its own entity sent: the issue of this instance its IID and the
// IssueBasic keys the capture read, the external one the tracker's identifier,
// and neither the other's.
func TestToReferencedOutputs_CarriesEachRowsOwnIdentifier(t *testing.T) {
	rows, err := ToReferencedOutputs(decodeIssues(t, referencedRowsBody), gitlabclient.CapturedBody([]byte(referencedRowsBody)))
	if err != nil {
		t.Fatalf("ToReferencedOutputs() unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].IID != 5 || rows[0].ExternalID != "" || rows[0].Type != "ISSUE" || rows[0].StartDate != "2026-01-02" {
		t.Errorf("internal row = %+v, want IID 5, no external_id, type ISSUE, start_date 2026-01-02", rows[0])
	}
	if rows[1].ExternalID != "PROJ-7" || rows[1].IID != 0 || rows[1].Title != "External Issue PROJ-7" {
		t.Errorf("external row = %+v, want external_id PROJ-7, IID 0 and the tracker's title", rows[1])
	}
}

// TestToReferencedOutputs_RefusesAnAnswerItCannotHold checks that a captured
// answer the IssueBasic reader cannot decode is the conversion's error rather
// than rows with the captured keys silently missing.
func TestToReferencedOutputs_RefusesAnAnswerItCannotHold(t *testing.T) {
	body := `[{"id":10,"iid":5,"start_date":7}]`
	if _, err := ToReferencedOutputs(decodeIssues(t, body), gitlabclient.CapturedBody([]byte(body))); err == nil {
		t.Error("ToReferencedOutputs() succeeded on a captured answer its reader cannot hold")
	}
}

// TestReferencedOutput_PublishesExternalIDOnlyWhenSent checks the key's
// omission: an issue of this instance carries no external_id at all rather
// than an empty one, and an external tracker's issue carries its identifier.
func TestReferencedOutput_PublishesExternalIDOnlyWhenSent(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  ReferencedOutput
		want bool
	}{
		{"instance issue", ReferencedOutput{IID: 5}, false},
		{"external issue", ReferencedOutput{ExternalID: "PROJ-7"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.row)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var published map[string]any
			if err = json.Unmarshal(raw, &published); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if _, ok := published["external_id"]; ok != tc.want {
				t.Errorf("external_id present = %t, want %t (%s)", ok, tc.want, raw)
			}
		})
	}
}
