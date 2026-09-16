//go:build e2e

// job_token_scope_test.go drives the allowlist builder against the stub, and
// pins the three properties the file's comment rests on: the scope is
// switched on before the entry is added, a refused add is judged by the
// read-back rather than by its status, and an allowlist that does not hold
// the target afterwards is reported rather than handed back as a fixture.

package fixture

import (
	"net/http"
	"strings"
	"testing"
)

// scriptAllowlist points the stub at the three endpoints the builder uses.
func scriptAllowlist(stub *stubGitLab, patch, add, list scriptedAnswer) {
	stub.answers(http.MethodPatch, "/api/v4/projects/1/job_token_scope", patch)
	stub.answers(http.MethodPost, "/api/v4/projects/1/job_token_scope/allowlist", add)
	stub.answers(http.MethodGet, "/api/v4/projects/1/job_token_scope/allowlist", list)
}

// TestAllowJobTokenProject_Added_SwitchesTheScopeOnFirst checks the order the
// two writes go in, which is what decides whether the entry is read at all.
func TestAllowJobTokenProject_Added_SwitchesTheScopeOnFirst(t *testing.T) {
	stub, client := newStubGitLab(t)
	scriptAllowlist(stub,
		stubNoContent(),
		stubCreated(map[string]any{"source_project_id": 1, "target_project_id": 2}),
		stubOK([]any{map[string]any{"id": 2, "path_with_namespace": "e2e/target"}}),
	)

	got, err := allowJobTokenProject(t.Context(), client, 1, 2)
	if err != nil {
		t.Fatalf("allowJobTokenProject() error = %v, want nil", err)
	}
	if got != (JobTokenScope{SourceProjectID: 1, TargetProjectID: 2}) {
		t.Errorf("allowJobTokenProject() = %+v, want the pair it was asked for", got)
	}

	requests := stub.recordedRequests()
	if len(requests) != 3 {
		t.Fatalf("allowJobTokenProject() sent %d requests, want the patch, the add and the read-back", len(requests))
	}
	if requests[0].Method != http.MethodPatch {
		t.Errorf("allowJobTokenProject() sent %s first, want the scope switched on before the entry is added", requests[0].Method)
	}
	if requests[0].Body["enabled"] != true {
		t.Errorf("allowJobTokenProject() sent enabled %v, want true", requests[0].Body["enabled"])
	}
}

// alreadyOnTheList is the refusal GitLab gives an entry a previous attempt
// already added: a 400 carrying the service's own message, not a conflict.
// Scripting the status GitLab does not send is what would make this test pass
// over a builder no retried attempt can get through.
const alreadyOnTheList = "This project is already in the job token allowlist."

// TestAllowJobTokenProject_AlreadyOnTheList_CarriesOnToTheReadBack checks the
// retried-attempt ending: GitLab refuses the second add with a 400, and the
// entry the first one made is what the caller asked for.
func TestAllowJobTokenProject_AlreadyOnTheList_CarriesOnToTheReadBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	scriptAllowlist(stub,
		stubNoContent(),
		stubRefusal(http.StatusBadRequest, alreadyOnTheList),
		stubOK([]any{map[string]any{"id": 2, "path_with_namespace": "e2e/target"}}),
	)

	got, err := allowJobTokenProject(t.Context(), client, 1, 2)
	if err != nil {
		t.Fatalf("allowJobTokenProject() error = %v, want the refusal tolerated", err)
	}
	if got != (JobTokenScope{SourceProjectID: 1, TargetProjectID: 2}) {
		t.Errorf("allowJobTokenProject() = %+v, want the pair it was asked for", got)
	}
	if requests := stub.recordedRequests(); len(requests) != 3 {
		t.Errorf("allowJobTokenProject() sent %d requests, want the refused add followed by the read-back", len(requests))
	}
}

// TestAllowJobTokenProject_RefusedAndNotOnTheList_ReportsTheAdd checks the
// other side of tolerating a refused add: an allowlist that does not hold the
// target afterwards is a failure, and the error names what the add said
// rather than only that the entry is missing.
func TestAllowJobTokenProject_RefusedAndNotOnTheList_ReportsTheAdd(t *testing.T) {
	stub, client := newStubGitLab(t)
	scriptAllowlist(stub,
		stubNoContent(),
		stubRefusal(http.StatusForbidden, "403 Forbidden"),
		stubOK([]any{map[string]any{"id": 99, "path_with_namespace": "someone/else"}}),
	)

	_, err := allowJobTokenProject(t.Context(), client, 1, 2)
	if err == nil {
		t.Fatal("allowJobTokenProject() error = nil, want the refused add reported")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("allowJobTokenProject() error = %q, want it to carry what the add was refused with", err)
	}
}

// TestAllowJobTokenProject_NotOnTheListAfterwards_IsReported checks the read
// back that turns a silent no-op into a failure here rather than in the case
// that depended on it.
func TestAllowJobTokenProject_NotOnTheListAfterwards_IsReported(t *testing.T) {
	stub, client := newStubGitLab(t)
	scriptAllowlist(stub,
		stubNoContent(),
		stubCreated(map[string]any{"source_project_id": 1, "target_project_id": 2}),
		stubOK([]any{map[string]any{"id": 99, "path_with_namespace": "someone/else"}}),
	)

	if _, err := allowJobTokenProject(t.Context(), client, 1, 2); err == nil {
		t.Error("allowJobTokenProject() error = nil, want an allowlist without the target reported")
	}
}

// TestAllowJobTokenProject_ScopeRefused_StopsBeforeTheEntry checks that a
// refused patch ends the builder, since an entry added to a project whose
// scope is off is read by nothing.
func TestAllowJobTokenProject_ScopeRefused_StopsBeforeTheEntry(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPatch, "/api/v4/projects/1/job_token_scope",
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	if _, err := allowJobTokenProject(t.Context(), client, 1, 2); err == nil {
		t.Fatal("allowJobTokenProject() error = nil, want the refused patch reported")
	}
	if got := len(stub.recordedRequests()); got != 1 {
		t.Errorf("allowJobTokenProject() sent %d requests after a refused patch, want 1", got)
	}
}
