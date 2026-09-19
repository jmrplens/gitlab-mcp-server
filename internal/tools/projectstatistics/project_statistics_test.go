// project_statistics_test.go contains unit tests for the project statistics MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package projectstatistics

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestGet reads the whole answer back: the total, and every day with its own
// date and count. No two numbers in the fixture agree, so a converter reading
// a neighbour's key changes what is asserted here. The earlier version checked
// the total and the length alone, and emptying both DayStat fields left the
// suite green.
func TestGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/1/statistics")
		testutil.RespondJSON(w, http.StatusOK, `{"fetches":{"total":42,"days":[{"count":5,"date":"2026-01-01"},{"count":7,"date":"2026-02-03"}]}}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TotalFetches != 42 {
		t.Errorf("TotalFetches = %d, want 42", out.TotalFetches)
	}
	want := []DayStat{{Date: "2026-01-01", Count: 5}, {Date: "2026-02-03", Count: 7}}
	if !reflect.DeepEqual(out.Days, want) {
		t.Errorf("Days = %+v, want %+v", out.Days, want)
	}
}

// TestGet_Error checks that a project GitLab will not serve is refused with
// the hint that routes the caller to the tool which verifies the id.
//
// The hint is keyed on one status, so a wrong code drops the only actionable
// part of the refusal while leaving an error that still looks correct.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Suggestion: verify project_id with gitlab_project_get") {
		t.Errorf("404 refusal %q carries no project lookup hint", err)
	}
}

// TestGet_Forbidden_CarriesNoProjectLookupHint is the other side of that
// status: a 403 means the caller lacks the Reporter access this endpoint
// needs, not that the id is wrong, so sending them to look the project up
// would point them at the wrong thing. GitLab's own message is what says
// which it was, so the refusal has to keep it.
func TestGet_Forbidden_CarriesNoProjectLookupHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"insufficient access to project statistics"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "Suggestion:") {
		t.Errorf("403 refusal %q carries a hint keyed to another status", err)
	}
	if !strings.Contains(err.Error(), "({message: insufficient access to project statistics})") {
		t.Errorf("403 refusal %q does not lift GitLab's own message out of the cause", err)
	}
}

// TestFormatMarkdown verifies the whole card: the total as the object's field
// and the days as the nested collection under their own heading.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown(GetOutput{TotalFetches: 42, Days: []DayStat{{Date: "2026-01-01", Count: 5}}})
	want := "## Project Statistics (Last 30 Days)\n\n" +
		"- **Total Fetches**: 42\n\n" +
		"### Daily Fetches\n\n" +
		"| Date | Count |\n| --- | --- |\n" +
		"| 1 Jan 2026 | 5 |\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use fetcher counts to track project activity trends\n"
	if md != want {
		t.Errorf("FormatMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestFormatMarkdown_Empty verifies that a statistics answer with no days
// renders the total alone: zero is an answer GitLab gave, and the collection
// heading is not written for a collection with nothing in it.
func TestFormatMarkdown_Empty(t *testing.T) {
	md := FormatMarkdown(GetOutput{TotalFetches: 0})
	want := "## Project Statistics (Last 30 Days)\n\n" +
		"- **Total Fetches**: 0\n\n" +
		"---\n💡 **Next steps:**\n" +
		"- Use fetcher counts to track project activity trends\n"
	if md != want {
		t.Errorf("FormatMarkdown()\n got: %q\nwant: %q", md, want)
	}
}

// TestGet_MissingProjectID refuses an empty project_id before any request is
// made, naming both the tool and the parameter. Both halves matter: the
// message is what a model reads to correct its call, and an empty id would
// otherwise be sent as /projects//statistics, which GitLab answers with
// something about a route rather than about the argument.
func TestGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gitlab_get_project_statistics") || !strings.Contains(msg, "project_id") {
		t.Errorf("refusal %q does not name both the tool and the parameter", msg)
	}
}

// TestActionSpecs_Metadata verifies project statistics action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "projectstatistics" || specs[0].IndividualTool.Name != "gitlab_get_project_statistics" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatal("project statistics ActionSpec should define usage")
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatal("project statistics ActionSpec should define aliases")
	}
	if specs[0].ParameterGuidance["project_id"].SemanticRole == "" {
		t.Fatal("project statistics ActionSpec should define project_id parameter guidance")
	}
}

// TestActionSpecs_CallRoute verifies the project statistics canonical route.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"fetches":{"total":42,"days":[{"count":5,"date":"2026-01-01"}]}}`)
	})
	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	result, err := spec.Route.Handler(t.Context(), map[string]any{"project_id": "1"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}
