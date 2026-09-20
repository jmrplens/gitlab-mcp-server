// dependency_proxy_test.go contains unit tests for the dependencyproxy MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dependencyproxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// purgeCachePath is the endpoint GitLab serves the group cache purge on, for
// the group the tests below use. The handlers answer it and nothing else, so a
// request built for another group, or without one, is a failure rather than a
// pass on a handler that accepts everything.
const purgeCachePath = "/api/v4/groups/5/dependency_proxy/cache"

// suggestionMarker is what toolutil.WrapErrWithHint writes in front of the
// corrective advice, and so the one thing in a wrapped error that separates
// "here is what to do about it" from "here is what happened".
const suggestionMarker = "Suggestion: "

// suggestionIn returns the corrective advice a wrapped error carries, and
// whether it carries any at all.
//
// The hint sits between the marker and the cause the wrapper appends, so
// cutting twice yields the advice on its own: a hint that went missing is
// reported as absent, and one that was emptied comes back as "". Both matter,
// because the reason Purge reaches for WrapErrWithStatusHint rather than
// WrapErrWithMessage is precisely to attach that advice.
func suggestionIn(msg string) (advice string, found bool) {
	_, rest, found := strings.Cut(msg, suggestionMarker)
	if !found {
		return "", false
	}
	advice, _, _ = strings.Cut(rest, ": ")
	return advice, true
}

// TestPurge verifies the Purge handler.
// The mock GitLab API at /api/v4/groups/5/dependency_proxy/cache (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestPurge(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != purgeCachePath || r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPurge_Error verifies that Purge returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPurge_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestPurge_Forbidden_NamesTheToolAndSuggestsTheRoleTheCallerNeeds asserts what
// a caller refused by GitLab is actually told: the error names the tool they
// invoked, and it carries corrective advice.
//
// Both halves were asserted by nothing. TestPurge_Error checks only that some
// error came back, and every branch of WrapErrWithStatusHint returns one, so
// the operation string and the whole hint could go missing or drift from the
// tool name in ActionSpecs without a test noticing — verified by hand, by
// renaming the operation and changing the status code, which the suite passed.
// The operation is checked against IndividualTool.Name rather than against a
// literal here, because the property is that the two agree: an error naming a
// tool the caller cannot find sends them looking for something that is not
// there.
func TestPurge_Forbidden_NamesTheToolAndSuggestsTheRoleTheCallerNeeds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))

	err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	if !strings.Contains(msg, purgeTool) {
		t.Errorf("error does not name the tool %q: %s", purgeTool, msg)
	}
	advice, found := suggestionIn(msg)
	if !found {
		t.Fatalf("a refused purge carries no suggestion: %s", msg)
	}
	if advice == "" {
		t.Errorf("the suggestion is empty: %s", msg)
	}
}

// TestPurge_FailureThatIsNotForbidden_ReportsItWithoutTheRoleSuggestion asserts
// the other side of the status gate: a failure that is not a 403 is reported
// without the permission advice.
//
// This is what makes the status code in Purge load-bearing rather than
// decorative. The hint says the caller needs group Owner, which is a true and
// useful thing to say about a refusal and a misleading one about a group that
// does not exist or an instance that fell over: it sends someone to ask for a
// role they may already hold. Without this test the code could hand that advice
// to every failure, or gate on the wrong status, and nothing would fail.
func TestPurge_FailureThatIsNotForbidden_ReportsItWithoutTheRoleSuggestion(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "group not found", status: http.StatusNotFound, body: `{"message":"404 Group Not Found"}`},
		{name: "instance error", status: http.StatusInternalServerError, body: `{"message":"something went wrong"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tc.status, tc.body)
			}))

			err := Purge(t.Context(), client, PurgeInput{GroupID: "5"})
			if err == nil {
				t.Fatal("expected error")
			}

			msg := err.Error()
			if !strings.Contains(msg, purgeTool) {
				t.Errorf("error does not name the tool %q: %s", purgeTool, msg)
			}
			if advice, found := suggestionIn(msg); found {
				t.Errorf("a %d carries the permission suggestion %q: %s", tc.status, advice, msg)
			}
		})
	}
}
