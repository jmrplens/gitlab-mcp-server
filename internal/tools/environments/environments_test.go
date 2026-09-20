// environments_test.go contains unit tests for the environment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package environments

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
	errExpCancelledCtx = "expected error for canceled context"
	// errExpZeroEnvID identifies the err exp zero env ID constant used by this package.
	errExpZeroEnvID = "expected error for zero environment_id"
	// pathEnvironments identifies the path environments constant used by this package.
	pathEnvironments = "/api/v4/projects/42/environments"
	// pathEnvironment1 identifies the path environment 1 constant used by this package.
	pathEnvironment1 = "/api/v4/projects/42/environments/1"
	// msgNotFoundBody and msgForbiddenBody are the two refusals the mocks
	// answer with. They are spelled as the JSON GitLab really sends, because a
	// body that does not parse reaches a handler as a decoding failure rather
	// than as the status it was meant to carry, and an assertion about the
	// message would then be about our own fixture.
	msgNotFoundBody  = `{"message":"404 Environment Not Found"}`
	msgForbiddenBody = `{"message":"Insufficient permissions for this environment"}`
	// forbiddenMessage is what msgForbiddenBody says, the words a caller should
	// read back out of the wrapped error. It is deliberately not "403
	// Forbidden": [toolutil.ExtractGitLabMessage] drops a message that only
	// repeats the status, so a fixture spelled that way would make every
	// assertion about GitLab's own words pass vacuously.
	forbiddenMessage = "Insufficient permissions for this environment"
)

// ---------------------------------------------------------------------------
// environmentList tests
// ---------------------------------------------------------------------------.

// TestEnvironmentList_Success asserts that List publishes every environment of
// the page GitLab answered with, in order and with each row's own fields.
func TestEnvironmentList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironments && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"name":"production","slug":"production","state":"available","tier":"production","external_url":"https://prod.example.com","created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"name":"staging","slug":"staging","state":"available","tier":"staging","created_at":"2026-01-01T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Environments) != 2 {
		t.Fatalf("expected 2 environments, got %d", len(out.Environments))
	}
	if out.Environments[0].Name != "production" || out.Environments[0].Tier != "production" {
		t.Errorf("first env mismatch: %+v", out.Environments[0])
	}
}

// TestEnvironmentList_WithFilters verifies that List forwards the search and
// states filters to GitLab as query parameters. The mock asserts the query it
// received, since a filter dropped on the way out returns a plausible page of
// the wrong environments.
func TestEnvironmentList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironments {
			if r.URL.Query().Get("search") != "stag" {
				t.Errorf("expected search=stag, got %s", r.URL.Query().Get("search"))
			}
			if r.URL.Query().Get("states") != "available" {
				t.Errorf("expected states=available, got %s", r.URL.Query().Get("states"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Search:    "stag",
		States:    "available",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestEnvironmentList_MissingProjectID asserts that List refuses a call with no
// project_id itself, before any request leaves the process: the mock forbids
// every request, so a refusal that came from GitLab instead would fail here.
func TestEnvironmentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// TestEnvironmentList_CancelledContext asserts that a canceled context aborts
// the call without contacting GitLab, which the forbidding mock is what checks:
// with an answering mock the claim in this comment was nobody's assertion.
func TestEnvironmentList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentGet tests
// ---------------------------------------------------------------------------.

// TestEnvironmentGet_Success verifies that Get publishes what GitLab answered
// for one environment. The fixture is a dynamic environment, whose slug GitLab
// derives from the name rather than copying it ("review/new-ui" ->
// "review-new-ui"): every other fixture here gives the two the same value, so
// a slug read from the name would be invisible in all of them.
func TestEnvironmentGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1 && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"review/new-ui","slug":"review-new-ui","state":"available","tier":"development","external_url":"https://review.example.com","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:     "42",
		EnvironmentID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 || out.Name != "review/new-ui" || out.ExternalURL != "https://review.example.com" {
		t.Errorf("unexpected output: %+v", out)
	}
	if out.Slug != "review-new-ui" {
		t.Errorf("Slug = %q, want %q (GitLab's own slug, not the name)", out.Slug, "review-new-ui")
	}
}

// TestEnvironmentGet_ZeroID asserts that Get refuses an unset environment_id
// itself rather than asking GitLab for environment zero, which the forbidding
// mock is what checks.
func TestEnvironmentGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{
		ProjectID:     "42",
		EnvironmentID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentGet_CancelledContext asserts that a canceled context aborts
// the call without contacting GitLab, checked by the forbidding mock.
func TestEnvironmentGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentCreate tests
// ---------------------------------------------------------------------------.

// TestEnvironmentCreate_Success asserts that Create posts to the project's
// environments path and publishes the environment GitLab answered with, id and
// tier included.
func TestEnvironmentCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironments && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"qa","slug":"qa","state":"available","tier":"testing","description":"QA environment","created_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Name:        "qa",
		Description: "QA environment",
		Tier:        "testing",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 || out.Name != "qa" || out.Tier != "testing" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestEnvironmentCreate_MissingName asserts that Create refuses a call with no
// name itself rather than letting GitLab answer 400, which the forbidding mock
// is what checks.
func TestEnvironmentCreate_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Name:      "",
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// TestEnvironmentCreate_CancelledContext asserts that a canceled context aborts
// the call without creating anything on GitLab, checked by the forbidding mock.
func TestEnvironmentCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Name: "qa"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentUpdate tests
// ---------------------------------------------------------------------------.

// TestEnvironmentUpdate_Success asserts that Update sends a PUT to the
// environment's own path and publishes the environment GitLab answered with.
func TestEnvironmentUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1 && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"production-v2","slug":"production-v2","state":"available","tier":"production","external_url":"https://v2.prod.example.com"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "42",
		EnvironmentID: 1,
		Name:          "production-v2",
		ExternalURL:   "https://v2.prod.example.com",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "production-v2" {
		t.Errorf("expected name 'production-v2', got %q", out.Name)
	}
}

// TestEnvironmentUpdate_ZeroID asserts that Update refuses an unset
// environment_id itself rather than sending a PUT to environment zero, which
// the forbidding mock is what checks.
func TestEnvironmentUpdate_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "42",
		EnvironmentID: 0,
		Name:          "new-name",
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentUpdate_CancelledContext asserts that a canceled context aborts
// the call without changing anything on GitLab, checked by the forbidding mock.
func TestEnvironmentUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", EnvironmentID: 1, Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentDelete tests
// ---------------------------------------------------------------------------.

// TestEnvironmentDelete_Success asserts that Delete sends a DELETE to the
// environment's own path and reports no error for GitLab's 204, which carries
// no body to read anything else out of.
func TestEnvironmentDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1 && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "42",
		EnvironmentID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestEnvironmentDelete_ZeroID asserts that Delete refuses an unset
// environment_id itself rather than sending a DELETE to environment zero, which
// the forbidding mock is what checks. A delete is the call where that matters
// most, since nothing undoes it.
func TestEnvironmentDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:     "42",
		EnvironmentID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentDelete_CancelledContext asserts that a canceled context aborts
// the call without deleting anything on GitLab, checked by the forbidding mock.
func TestEnvironmentDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// environmentStop tests
// ---------------------------------------------------------------------------.

// TestEnvironmentStop_Success asserts that Stop posts to the environment's stop
// path and reports the state GitLab answered with, which is what tells a caller
// the environment really stopped.
func TestEnvironmentStop_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1+"/stop" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"staging","slug":"staging","state":"stopped","tier":"staging"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "42",
		EnvironmentID: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "stopped" {
		t.Errorf("expected state 'stopped', got %q", out.State)
	}
}

// TestEnvironmentStop_WithForce asserts that Stop reports the state GitLab
// answered with for a forced stop. What the flag itself does on the wire is
// asserted by [TestEnvironmentStop_ForceFlagOnTheWire].
func TestEnvironmentStop_WithForce(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathEnvironment1+"/stop" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"staging","slug":"staging","state":"stopped"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	force := true
	out, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "42",
		EnvironmentID: 1,
		Force:         &force,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "stopped" {
		t.Errorf("expected state 'stopped', got %q", out.State)
	}
}

// TestEnvironmentStop_ZeroID asserts that Stop refuses an unset environment_id
// itself rather than posting a stop to environment zero, which the forbidding
// mock is what checks.
func TestEnvironmentStop_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "42",
		EnvironmentID: 0,
	})
	if err == nil {
		t.Fatal(errExpZeroEnvID)
	}
}

// TestEnvironmentStop_CancelledContext asserts that a canceled context aborts
// the call without stopping anything on GitLab, checked by the forbidding mock.
func TestEnvironmentStop_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Stop(ctx, client, StopInput{ProjectID: "42", EnvironmentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error, name filter, pagination
// ---------------------------------------------------------------------------.

// assertGitLabError checks what a caller can act on in an error GitLab
// produced, rather than only that one came back: the operation that failed,
// GitLab's own message carried through, and the suggestion the handler attaches
// only at the status its hint was written for. wantHint is empty for a response
// whose status the handler's gate does not name, and asserting its absence is
// half the point: a hint offered for every status is advice about the wrong
// failure.
func assertGitLabError(t *testing.T, err error, operation, wantHint string) {
	t.Helper()
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.HasPrefix(err.Error(), operation+": ") {
		t.Errorf("error %q does not open by naming the operation %q", err, operation)
	}
	if !strings.Contains(err.Error(), forbiddenMessage) {
		t.Errorf("error %q does not carry GitLab's own message %q", err, forbiddenMessage)
	}
	if wantHint == "" {
		if strings.Contains(err.Error(), "Suggestion:") {
			t.Errorf("error %q carries a suggestion written for another status", err)
		}
		return
	}
	if !strings.Contains(err.Error(), wantHint) {
		t.Errorf("error %q does not carry the hint %q", err, wantHint)
	}
}

// TestEnvironmentList_APIError asserts that a refusal from GitLab reaches the
// caller naming the operation and repeating GitLab's message, and without the
// 404 hint this handler reserves for a missing project.
func TestEnvironmentList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	assertGitLabError(t, err, "environmentList", "")
}

// TestEnvironmentList_WithNameFilter asserts that the exact-name filter travels
// as the name query parameter, which the mock checks, and that the page GitLab
// answered with is published back.
func TestEnvironmentList_WithNameFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/environments" {
			if got := r.URL.Query().Get("name"); got != "production" {
				t.Errorf("expected name=production, got %s", got)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":1,"name":"production","slug":"production","state":"available","tier":"production"}]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "1", Name: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Environments) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(out.Environments))
	}
	if out.Environments[0].Name != "production" {
		t.Errorf("expected name=production, got %q", out.Environments[0].Name)
	}
}

// TestEnvironmentList_Pagination asserts that List forwards the page a caller
// asked for and publishes GitLab's own page headers back in
// [toolutil.PaginationOutput], which is the only way a caller learns there is
// more to ask for.
func TestEnvironmentList_Pagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/environments" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id":3,"name":"dev","slug":"dev","state":"available"}]`,
				testutil.PaginationHeaders{Page: "2", PerPage: "1", Total: "3", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "1",
		Page:      2, PerPage: 1,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 3 {
		t.Errorf("NextPage = %d, want 3", out.Pagination.NextPage)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestEnvironmentGet_APIError asserts that a refusal from GitLab reaches the
// caller naming the operation and repeating GitLab's message, and without the
// 404 hint this handler reserves for an environment that is not there.
func TestEnvironmentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", EnvironmentID: 1})
	assertGitLabError(t, err, "environmentGet", "")
}

// TestEnvironmentGet_MissingProjectID asserts that Get refuses a call with no
// project_id before any request leaves the process, which the forbidding mock
// is what checks.
func TestEnvironmentGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, missing project_id, all optional fields
// ---------------------------------------------------------------------------.

// TestEnvironmentCreate_APIError asserts that a refusal from GitLab reaches the
// caller naming the operation and repeating GitLab's message, and without the
// 400 hint this handler reserves for a rejected name, tier or URL.
func TestEnvironmentCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Name: "staging"})
	assertGitLabError(t, err, "environmentCreate", "")
}

// TestEnvironmentCreate_MissingProjectID asserts that Create refuses a call
// with no project_id before any request leaves the process, which the
// forbidding mock is what checks.
func TestEnvironmentCreate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Create(context.Background(), client, CreateInput{Name: "staging"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// recordingHandler answers one path with a fixed response and keeps the request
// body it was sent, so a test can assert what this server sent GitLab and not
// only what GitLab sent back. Nothing in a response can show that a caller's
// optional field was dropped on the way out, which is how four of this
// package's send guards could be inverted with the suite still green.
func recordingHandler(t *testing.T, method, path string, status int, response string) (http.Handler, *string) {
	t.Helper()
	var body string
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the request body: %v", err)
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"unreadable request body"}`)
			return
		}
		body = string(raw)
		testutil.RespondJSON(w, status, response)
	}), &body
}

// TestEnvironmentCreate_AllOptionalFields asserts that the description, the
// external URL and the tier a caller gave reach GitLab in the request body, and
// that the environment GitLab answers with is published back. Each value is
// distinct, so a guard sending the wrong one is a failure and not a coincidence.
func TestEnvironmentCreate_AllOptionalFields(t *testing.T) {
	handler, body := recordingHandler(t, http.MethodPost, "/api/v4/projects/1/environments", http.StatusCreated, `{
		"id":10,"name":"review","slug":"review","state":"available",
		"tier":"development","description":"Review env","external_url":"https://review.example.com",
		"created_at":"2026-06-01T00:00:00Z"
	}`)
	client := testutil.NewTestClient(t, handler)

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "1",
		Name:        "review",
		Description: "Review env",
		ExternalURL: "https://review.example.com",
		Tier:        "development",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{
		`"name":"review"`,
		`"description":"Review env"`,
		`"external_url":"https://review.example.com"`,
		`"tier":"development"`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(*body, want) {
				t.Errorf("request body %q missing %q", *body, want)
			}
		})
	}
	if out.Tier != "development" {
		t.Errorf("Tier = %q, want %q", out.Tier, "development")
	}
	if out.ExternalURL != "https://review.example.com" {
		t.Errorf("ExternalURL = %q, want %q", out.ExternalURL, "https://review.example.com")
	}
	if out.Description != "Review env" {
		t.Errorf("Description = %q, want %q", out.Description, "Review env")
	}
}

// TestEnvironmentCreate_OnlyTheNameIsSent asserts the other half of every
// optional-field guard: what the caller left unset is absent from the request
// rather than sent as an empty string, which GitLab would store over whatever
// its default is.
func TestEnvironmentCreate_OnlyTheNameIsSent(t *testing.T) {
	handler, body := recordingHandler(t, http.MethodPost, "/api/v4/projects/1/environments", http.StatusCreated,
		`{"id":11,"name":"minimal","slug":"minimal","state":"available"}`)
	client := testutil.NewTestClient(t, handler)

	if _, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Name: "minimal"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, unwanted := range []string{
		"description", "external_url", "tier",
		"cluster_agent_id", "kubernetes_namespace", "flux_resource_path", "auto_stop_setting",
	} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(*body, `"`+unwanted+`"`) {
				t.Errorf("request body %q carries %q, which the caller never set", *body, unwanted)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update — API error, missing project_id, all optional fields
// ---------------------------------------------------------------------------.

// TestEnvironmentUpdate_APIError asserts that a refusal from GitLab reaches the
// caller naming the operation and repeating GitLab's message, and without the
// 404 hint this handler reserves for an environment that is not there.
func TestEnvironmentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", EnvironmentID: 1})
	assertGitLabError(t, err, "environmentUpdate", "")
}

// TestEnvironmentUpdate_MissingProjectID asserts that Update refuses a call
// with no project_id before any request leaves the process, which the
// forbidding mock is what checks.
func TestEnvironmentUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Update(context.Background(), client, UpdateInput{EnvironmentID: 1, Name: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentUpdate_AllOptionalFields asserts that the new name,
// description, external URL and tier reach GitLab in the request body: an
// update whose field never left the process answers with the environment
// unchanged, which reads like success.
func TestEnvironmentUpdate_AllOptionalFields(t *testing.T) {
	handler, body := recordingHandler(t, http.MethodPut, "/api/v4/projects/1/environments/5", http.StatusOK, `{
		"id":5,"name":"staging-v2","slug":"staging-v2","state":"available",
		"tier":"staging","description":"Updated staging","external_url":"https://staging-v2.example.com"
	}`)
	client := testutil.NewTestClient(t, handler)

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "1",
		EnvironmentID: 5,
		Name:          "staging-v2",
		Description:   "Updated staging",
		ExternalURL:   "https://staging-v2.example.com",
		Tier:          "staging",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{
		`"name":"staging-v2"`,
		`"description":"Updated staging"`,
		`"external_url":"https://staging-v2.example.com"`,
		`"tier":"staging"`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(*body, want) {
				t.Errorf("request body %q missing %q", *body, want)
			}
		})
	}
	if out.Tier != "staging" {
		t.Errorf("Tier = %q, want %q", out.Tier, "staging")
	}
	if out.Description != "Updated staging" {
		t.Errorf("Description = %q, want %q", out.Description, "Updated staging")
	}
}

// TestEnvironmentUpdate_SendsOnlyWhatTheCallerSet asserts that an update
// naming one field sends that field alone: an empty string for each of the
// others would blank them on GitLab, which is the one way an update can do
// damage the caller never asked for.
func TestEnvironmentUpdate_SendsOnlyWhatTheCallerSet(t *testing.T) {
	handler, body := recordingHandler(t, http.MethodPut, "/api/v4/projects/1/environments/5", http.StatusOK,
		`{"id":5,"name":"staging","slug":"staging","state":"available"}`)
	client := testutil.NewTestClient(t, handler)

	if _, err := Update(context.Background(), client, UpdateInput{
		ProjectID:     "1",
		EnvironmentID: 5,
		Description:   "Only this one",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(*body, `"description":"Only this one"`) {
		t.Errorf("request body %q missing the description the caller set", *body)
	}
	for _, unwanted := range []string{"name", "external_url", "tier", "auto_stop_setting"} {
		t.Run(unwanted, func(t *testing.T) {
			if strings.Contains(*body, `"`+unwanted+`"`) {
				t.Errorf("request body %q carries %q, which the caller never set", *body, unwanted)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, missing project_id
// ---------------------------------------------------------------------------.

// TestEnvironmentDelete_APIError asserts that GitLab's 403 reaches the caller
// with the one piece of advice that answers it: an environment has to be
// stopped before it can be deleted. This is the status this handler's hint is
// written for, so here the suggestion must be present.
func TestEnvironmentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", EnvironmentID: 1})
	assertGitLabError(t, err, "environmentDelete", "environment must be stopped before deletion")
}

// TestEnvironmentDelete_MissingProjectID asserts that Delete refuses a call
// with no project_id before any request leaves the process, which the
// forbidding mock is what checks.
func TestEnvironmentDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Stop — API error, missing project_id, force=false
// ---------------------------------------------------------------------------.

// TestEnvironmentStop_APIError asserts that GitLab's 403 reaches the caller
// with the advice this handler reserves for that status: the role a stop needs,
// and force=true for an environment with deployments still running.
func TestEnvironmentStop_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	_, err := Stop(context.Background(), client, StopInput{ProjectID: "1", EnvironmentID: 1})
	assertGitLabError(t, err, "environmentStop", "use force=true to stop environments with active deployments")
}

// TestEnvironmentStop_MissingProjectID asserts that Stop refuses a call with no
// project_id before any request leaves the process, which the forbidding mock
// is what checks.
func TestEnvironmentStop_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Stop(context.Background(), client, StopInput{EnvironmentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestEnvironmentStop_ForceFalse asserts that an explicit force=false is an
// ordinary stop as far as the output is concerned. That it travels as false
// rather than being dropped is asserted by
// [TestEnvironmentStop_ForceFlagOnTheWire].
func TestEnvironmentStop_ForceFalse(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/environments/2/stop" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":2,"name":"staging","slug":"staging","state":"stopped","tier":"staging"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
	}))

	force := false
	out, err := Stop(context.Background(), client, StopInput{
		ProjectID:     "1",
		EnvironmentID: 2,
		Force:         &force,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.State != "stopped" {
		t.Errorf("State = %q, want %q", out.State, "stopped")
	}
}

// TestEnvironmentStop_ForceFlagOnTheWire asserts what the three states of the
// force flag put in the request body. The two set states are asserted apart and
// with the value each carries, because a pointer read for its presence alone
// would let true and false swap unnoticed, and the unset one must send no force
// key at all rather than a false GitLab would read as a decision.
func TestEnvironmentStop_ForceFlagOnTheWire(t *testing.T) {
	forced, unforced := true, false
	for _, tt := range []struct {
		name   string
		force  *bool
		want   string
		absent bool
	}{
		{name: "unset sends no force", force: nil, want: `"force"`, absent: true},
		{name: "true sends force true", force: &forced, want: `"force":true`},
		{name: "false sends force false", force: &unforced, want: `"force":false`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler, body := recordingHandler(t, http.MethodPost, "/api/v4/projects/1/environments/2/stop", http.StatusOK,
				`{"id":2,"name":"staging","slug":"staging","state":"stopped"}`)
			client := testutil.NewTestClient(t, handler)

			if _, err := Stop(context.Background(), client, StopInput{ProjectID: "1", EnvironmentID: 2, Force: tt.force}); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if got := strings.Contains(*body, tt.want); got == tt.absent {
				t.Errorf("request body %q: contains %q = %v, want %v", *body, tt.want, got, !tt.absent)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// toOutput — all optional timestamp fields
// ---------------------------------------------------------------------------.

// TestToOutput_AllTimestampFields pins the whole card of an environment whose
// three timestamps GitLab all sent: each is rendered in the display form and
// none of them as the wire string.
func TestToOutput_AllTimestampFields(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:          1,
		Name:        "production",
		Slug:        "production",
		State:       "available",
		Tier:        "production",
		Description: "Main prod environment",
		ExternalURL: "https://prod.example.com",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-06-15T12:00:00Z",
		AutoStopAt:  "2026-12-31T23:59:59Z",
	})

	want := "## Environment: production\n\n" +
		"- **ID**: 1\n" +
		"- **Slug**: production\n" +
		"- **State**: available\n" +
		"- **Tier**: production\n" +
		"- **Description**: Main prod environment\n" +
		"- **External URL**: [https://prod.example.com](https://prod.example.com)\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Updated**: 15 Jun 2026 12:00 UTC\n" +
		"- **Auto-Stop At**: 31 Dec 2026 23:59 UTC\n" +
		availableHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_EmptyName asserts that an output carrying no name is
// rendered as nothing at all rather than as a card with an empty heading, which
// is what a zero value reaching the registry would produce.
func TestFormatOutputMarkdown_EmptyName(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty name, got %q", md)
	}
}

// TestFormatEnvironmentNotFound asserts that the not-found card names the
// environment the caller asked for and offers the two ways out of it, and that
// it is marked an error: a model reading "not found" with no identifier cannot
// tell which of its calls missed.
func TestFormatEnvironmentNotFound(t *testing.T) {
	result := formatEnvironmentNotFound(environmentNotFoundOutput{Identifier: "ID 99 in project 42"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("not-found result is not marked an error")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content in not-found result")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want *mcp.TextContent", result.Content[0])
	}
	for _, want := range []string{
		"ID 99 in project 42",
		"gitlab_environment_list",
		"Verify the environment_id is correct for this project",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(text.Text, want) {
				t.Errorf("not-found card %q missing %q", text.Text, want)
			}
		})
	}
}

// stoppedHints and availableHints are the two guidance sections an environment
// card closes with, chosen by the state the environment is in.
const (
	stoppedHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'environment.delete' to delete this stopped environment\n" +
		"- Use action 'environment.deployment_list' to see the deployments to this environment\n"
	availableHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'environment.stop' to stop this environment\n" +
		"- Use action 'environment.deployment_list' to see the deployments to this environment\n"
)

// listHints is the guidance section the listing closes with, the preserve-links
// instruction first because the External URL column carries links.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'environment.get' to see one environment and what is deployed to it\n" +
	"- Use action 'environment.create' to add a new environment\n" +
	"- Use action 'environment.list' to page through the rest of the project's environments\n"

// TestFormatOutputMarkdown_MinimalFields pins the whole card of an environment
// GitLab answered with nothing optional: four rows, no label with an empty
// value under it, and the hint the state allows.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:    7,
		Name:  "dev",
		Slug:  "dev",
		State: "stopped",
	})

	want := "## Environment: dev\n\n" +
		"- **ID**: 7\n" +
		"- **Slug**: dev\n" +
		"- **State**: stopped\n" +
		stoppedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_FullCard pins the whole card of an environment with
// everything GitLab sends: the external URL as a link, the project and the
// cluster agent as nested objects, and the deployment running on it as a
// section, which is the question an environment is looked up to answer.
func TestFormatOutputMarkdown_FullCard(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                  12,
		Name:                "production",
		Slug:                "production",
		State:               "available",
		Tier:                "production",
		Description:         "Customer-facing",
		ExternalURL:         "https://prod.example.com",
		AutoStopSetting:     "with_action",
		KubernetesNamespace: "prod",
		FluxResourcePath:    "kustomization/prod",
		CreatedAt:           "2026-01-02T03:04:00Z",
		UpdatedAt:           "2026-02-03T04:05:00Z",
		AutoStopAt:          "2026-03-04T05:06:00Z",
		Project: &ProjectOutput{
			ID:                3,
			PathWithNamespace: "acme/web",
			WebURL:            "https://gitlab.example.com/acme/web",
		},
		ClusterAgent: &ClusterAgentOutput{ID: 4, Name: "prod-agent"},
		LastDeployment: &DeploymentOutput{
			ID:        90,
			IID:       12,
			Ref:       "main",
			SHA:       "0123456789abcdef",
			Status:    "success",
			CreatedAt: "2026-03-01T10:00:00Z",
			User:      &DeploymentUserOutput{Username: "dana", WebURL: "https://gitlab.example.com/dana"},
			Deployable: &DeployableOutput{
				Pipeline: &DeployablePipelineOutput{ID: 77, WebURL: "https://gitlab.example.com/acme/web/-/pipelines/77"},
			},
		},
	})

	want := "## Environment: production\n\n" +
		"- **ID**: 12\n" +
		"- **Slug**: production\n" +
		"- **State**: available\n" +
		"- **Tier**: production\n" +
		"- **Description**: Customer-facing\n" +
		"- **External URL**: [https://prod.example.com](https://prod.example.com)\n" +
		"- **Auto-Stop Setting**: with_action\n" +
		"- **Kubernetes Namespace**: `prod`\n" +
		"- **Flux Resource Path**: `kustomization/prod`\n" +
		"- **Created**: 2 Jan 2026 03:04 UTC\n" +
		"- **Updated**: 3 Feb 2026 04:05 UTC\n" +
		"- **Auto-Stop At**: 4 Mar 2026 05:06 UTC\n" +
		"- **Project**:\n" +
		"  - **ID**: 3\n" +
		"  - **Path**: acme/web\n" +
		"  - **URL**: [https://gitlab.example.com/acme/web](https://gitlab.example.com/acme/web)\n" +
		"- **Cluster Agent**:\n" +
		"  - **ID**: 4\n" +
		"  - **Name**: prod-agent\n" +
		"\n### Last Deployment\n\n" +
		"- **ID**: 90\n" +
		"- **IID**: 12\n" +
		"- **Status**: ✅ success\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `01234567`\n" +
		"- **Created**: 1 Mar 2026 10:00 UTC\n" +
		"- **Deployed By**: [@dana](https://gitlab.example.com/dana)\n" +
		"- **Pipeline**: [#77](https://gitlab.example.com/acme/web/-/pipelines/77)\n" +
		availableHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_StoppingOffersNeitherStopNorDelete pins the third
// state: an environment GitLab is still stopping can be neither stopped again
// nor deleted, so the card offers only the deployments.
func TestFormatOutputMarkdown_StoppingOffersNeitherStopNorDelete(t *testing.T) {
	got := FormatOutputMarkdown(Output{ID: 5, Name: "dev", State: "stopping"})

	want := "## Environment: dev\n\n" +
		"- **ID**: 5\n" +
		"- **State**: stopping\n" +
		"\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'environment.deployment_list' to see the deployments to this environment\n"

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithEnvironments pins the whole listing: the heading
// counting what GitLab reported, the external URL as a link, the pagination
// line and one guidance section.
func TestFormatListMarkdown_WithEnvironments(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Environments: []Output{
			{ID: 1, Name: "production", State: "available", Tier: "production", ExternalURL: "https://prod.example.com"},
			{ID: 2, Name: "staging", State: "available", Tier: "staging", ExternalURL: "https://staging.example.com"},
			{ID: 3, Name: "dev", State: "stopped", Tier: "development", ExternalURL: ""},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 3, Page: 1, PerPage: 20, TotalPages: 1},
	})

	want := "## Environments (3)\n\n" +
		"| ID | Name | State | Tier | External URL |\n| --- | --- | --- | --- | --- |\n" +
		"| 1 | production | available | production | [https://prod.example.com](https://prod.example.com) |\n" +
		"| 2 | staging | available | staging | [https://staging.example.com](https://staging.example.com) |\n" +
		"| 3 | dev | stopped | development |  |\n" +
		"\nPage 1 of 1 | 3 items total | 20 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_KeysetPage pins the heading of a page GitLab sent no
// total for: it counts the rows shown rather than claiming a total of zero
// above them.
func TestFormatListMarkdown_KeysetPage(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Environments: []Output{{ID: 1, Name: "production", State: "available"}},
		Pagination:   toolutil.PaginationOutput{HasMore: true},
	})

	want := "## Environments (1 shown, more available)\n\n" +
		"| ID | Name | State | Tier | External URL |\n| --- | --- | --- | --- | --- |\n" +
		"| 1 | production | available |  |  |\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins that a project with no environments
// renders the one sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No environments found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestGet_WithAutoStopAt asserts that the auto-stop instant GitLab sent is
// published: it is the one timestamp that says when an environment will go
// away by itself, and it is optional, so it has its own case.
func TestGet_WithAutoStopAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":1,"name":"review","slug":"review","state":"available",
			"created_at":"2026-01-01T00:00:00Z",
			"updated_at":"2026-01-02T00:00:00Z",
			"auto_stop_at":"2026-02-01T00:00:00Z"
		}`)
	}))
	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AutoStopAt == "" {
		t.Error("expected AutoStopAt to be set")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata asserts that the package publishes six actions, each
// under an individual tool name of its own and owned by this package, and that
// the three a model reaches for first carry the usage, aliases and parameter
// guidance discovery is built out of.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := environmentSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "environments" {
			t.Fatalf("OwnerPackage for %s = %q, want environments", spec.Name, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_environment_list"]
	if list.Usage == "" || len(list.Aliases) == 0 {
		t.Fatalf("gitlab_environment_list metadata incomplete: usage=%q aliases=%d", list.Usage, len(list.Aliases))
	}

	get := byTool["gitlab_environment_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["environment_id"].SemanticRole == "" {
		t.Fatalf("gitlab_environment_get metadata incomplete: usage=%q aliases=%d guidance(environment_id)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["environment_id"].SemanticRole)
	}

	stop := byTool["gitlab_environment_stop"]
	if stop.Usage == "" || len(stop.Aliases) == 0 {
		t.Fatalf("gitlab_environment_stop metadata incomplete: usage=%q aliases=%d", stop.Usage, len(stop.Aliases))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 6 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes asserts that every one of the six routes runs
// end to end from the arguments a catalog surface hands it: each returns a
// result and no error against a mock serving that action's own endpoint.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newEnvironmentSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_environment_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_environment_get", map[string]any{"project_id": "1", "environment_id": 1}},
		{"create", "gitlab_environment_create", map[string]any{"project_id": "1", "name": "review"}},
		{"update", "gitlab_environment_update", map[string]any{"project_id": "1", "environment_id": 1, "name": "updated"}},
		{"delete", "gitlab_environment_delete", map[string]any{"project_id": "1", "environment_id": 1}},
		{"stop", "gitlab_environment_stop", map[string]any{"project_id": "1", "environment_id": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: ActionSpec route factory
// ---------------------------------------------------------------------------.

// newEnvironmentSpecsByTool constructs environment specs by tool test fixtures.
func newEnvironmentSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	envJSON := `{"id":1,"name":"production","slug":"production","state":"available","tier":"production","external_url":"https://prod.example.com","created_at":"2026-01-01T00:00:00Z"}`

	handler := http.NewServeMux()

	// List environments
	handler.HandleFunc("GET /api/v4/projects/1/environments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+envJSON+`]`)
	})

	// Get environment
	handler.HandleFunc("GET /api/v4/projects/1/environments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	})

	// Create environment
	handler.HandleFunc("POST /api/v4/projects/1/environments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, envJSON)
	})

	// Update environment
	handler.HandleFunc("PUT /api/v4/projects/1/environments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, envJSON)
	})

	// Delete environment
	handler.HandleFunc("DELETE /api/v4/projects/1/environments/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Stop environment
	handler.HandleFunc("POST /api/v4/projects/1/environments/1/stop", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"production","slug":"production","state":"stopped","tier":"production"}`)
	})

	client := testutil.NewTestClient(t, handler)
	return environmentSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_EnvironmentGetRoute asserts that the get route's wrapper
// passes a successful read through untouched: the environment reaches the
// caller as an [Output] rather than as the not-found card the wrapper reserves
// for a 404.
func TestActionSpecs_EnvironmentGetRoute(t *testing.T) {
	const respJSON = `{"id":7,"name":"prod","slug":"prod","state":"available","tier":"production"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/environments/7") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := environmentSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_environment_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "environment_id": 7})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 7 || out.Name != "prod" {
		t.Fatalf("environment output = %#v, want ID 7 name prod", out)
	}
}

// environmentSpecsByTool supports environment specs by tool assertions in environments tests.
func environmentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// ---------------------------------------------------------------------------
// 1:1 audit — nested output sub-objects and additive input fields
// ---------------------------------------------------------------------------.

// envFullJSON is a full single-environment API response exercising every
// documented 1:1 field: scalar (auto_stop_setting, kubernetes_namespace,
// flux_resource_path) and nested objects (cluster_agent, last_deployment with
// its deployable+pipeline+user+commit+runner) as documented in
// doc/api/environments.md "Retrieve an environment".
const envFullJSON = `{
	"id":7,"name":"production","slug":"production","state":"available","tier":"production",
	"description":"Prod","external_url":"https://prod.example.com",
	"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-06-15T12:00:00Z",
	"auto_stop_at":"2026-12-31T23:59:59Z","auto_stop_setting":"with_action",
	"kubernetes_namespace":"prod-ns","flux_resource_path":"flux/prod",
	"cluster_agent":{
		"id":11,"name":"prod-agent","created_at":"2025-12-01T00:00:00Z","created_by_user_id":3,
		"config_project":{"id":99,"description":"Agent cfg","name":"cfg","name_with_namespace":"grp / cfg","path":"cfg","path_with_namespace":"grp/cfg","created_at":"2025-11-01T00:00:00Z"}
	},
	"last_deployment":{
		"id":501,"iid":12,"ref":"main","sha":"abc123","status":"success",
		"created_at":"2026-06-15T11:00:00Z",
		"user":{"id":4,"name":"Deployer","username":"deployer","state":"active","avatar_url":"https://av","web_url":"https://u"},
		"deployable":{
			"id":900,"status":"success","stage":"deploy","name":"deploy-prod","ref":"main","tag":false,
			"coverage":88.5,"created_at":"2026-06-15T10:55:00Z","started_at":"2026-06-15T10:56:00Z",
			"finished_at":"2026-06-15T11:00:00Z","duration":240,
			"user":{"id":4,"name":"Deployer","username":"deployer","state":"active","web_url":"https://u","created_at":"2025-01-01T00:00:00Z","bio":"bio text","location":"Earth","public_email":"d@x","organization":"Acme"},
			"commit":{"id":"abc123def","short_id":"abc123","title":"Deploy fix","message":"Deploy fix\n","author_name":"Dev","author_email":"dev@x","authored_date":"2026-06-15T10:00:00Z","committer_name":"Dev","committer_email":"dev@x","committed_date":"2026-06-15T10:00:00Z","created_at":"2026-06-15T10:00:00Z","parent_ids":["p1"]},
			"pipeline":{"id":700,"sha":"abc123","ref":"main","status":"success","web_url":"https://pipe"},
			"runner":{"id":55,"description":"shared-runner","name":"runner-1","is_shared":true,"runner_type":"instance_type","online":true,"status":"online"}
		}
	}
}`

// TestEnvironmentGet_FullNestedObjects verifies that Get surfaces every additive
// 1:1 field and nested sub-object faithfully from the API response.
func TestEnvironmentGet_FullNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/7" {
			testutil.RespondJSON(w, http.StatusOK, envFullJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	if out.AutoStopSetting != "with_action" {
		t.Errorf("AutoStopSetting = %q, want with_action", out.AutoStopSetting)
	}
	if out.KubernetesNamespace != "prod-ns" {
		t.Errorf("KubernetesNamespace = %q, want prod-ns", out.KubernetesNamespace)
	}
	if out.FluxResourcePath != "flux/prod" {
		t.Errorf("FluxResourcePath = %q, want flux/prod", out.FluxResourcePath)
	}

	assertClusterAgent(t, out.ClusterAgent)
	assertLastDeployment(t, out.LastDeployment)
}

// assertClusterAgent validates the fully populated cluster_agent sub-object.
func assertClusterAgent(t *testing.T, ca *ClusterAgentOutput) {
	t.Helper()
	if ca == nil {
		t.Fatal("ClusterAgent is nil")
	}
	if ca.ID != 11 || ca.Name != "prod-agent" || ca.CreatedByUserID != 3 || ca.CreatedAt == "" {
		t.Errorf("ClusterAgent = %#v", ca)
	}
	if ca.ConfigProject == nil || ca.ConfigProject.ID != 99 ||
		ca.ConfigProject.PathWithNamespace != "grp/cfg" || ca.ConfigProject.CreatedAt == "" {
		t.Errorf("ConfigProject = %#v", ca.ConfigProject)
	}
}

// assertLastDeployment validates the fully populated last_deployment sub-object,
// including its user, deployable, and pipeline references.
func assertLastDeployment(t *testing.T, ld *DeploymentOutput) {
	t.Helper()
	if ld == nil {
		t.Fatal("LastDeployment is nil")
	}
	wantLD := DeploymentOutput{ID: 501, IID: 12, Ref: "main", SHA: "abc123", Status: "success"}
	if ld.ID != wantLD.ID || ld.IID != wantLD.IID || ld.Ref != wantLD.Ref || ld.SHA != wantLD.SHA || ld.Status != wantLD.Status {
		t.Errorf("LastDeployment = %#v", ld)
	}
	if ld.CreatedAt == "" {
		t.Error("LastDeployment created_at empty")
	}
	// Every field of the deploying user is asserted, and each carries a value
	// no other field of the object has: the display name and the avatar were
	// the two nothing here read, so either could have been filled from the
	// field beside it with no test noticing.
	if u := ld.User; u == nil || u.ID != 4 || u.Name != "Deployer" || u.Username != "deployer" ||
		u.State != "active" || u.AvatarURL != "https://av" || u.WebURL != "https://u" {
		t.Errorf("LastDeployment.User = %#v", ld.User)
	}
	assertDeployable(t, ld.Deployable)
}

// assertDeployable validates the deployable job sub-object and its documented
// user, commit, pipeline, and runner references.
func assertDeployable(t *testing.T, dep *DeployableOutput) {
	t.Helper()
	if dep == nil {
		t.Fatal("Deployable is nil")
	}
	if dep.ID != 900 || dep.Status != "success" || dep.Stage != "deploy" || dep.Name != "deploy-prod" {
		t.Errorf("Deployable identity = %#v", dep)
	}
	// The ref and the tag flag say which commit the job ran for and whether it
	// was a tag: nothing read either, so the ref could have been filled from
	// the status beside it and the flag inverted unnoticed.
	if dep.Ref != "main" || dep.Tag {
		t.Errorf("Deployable ref/tag = %q/%v, want main/false", dep.Ref, dep.Tag)
	}
	if dep.Coverage != 88.5 || dep.Duration != 240 {
		t.Errorf("Deployable metrics = %#v", dep)
	}
	if dep.CreatedAt == "" || dep.StartedAt == "" || dep.FinishedAt == "" {
		t.Error("Deployable timestamps empty")
	}
	assertDeployableUser(t, dep.User)
	assertDeployableCommit(t, dep.Commit)
	assertDeployablePipeline(t, dep.Pipeline)
	assertDeployableRunner(t, dep.Runner)
}

// assertDeployableUser validates the documented deployable.user subset.
func assertDeployableUser(t *testing.T, u *DeployableUserOutput) {
	t.Helper()
	if u == nil || u.ID != 4 || u.Username != "deployer" || u.Bio != "bio text" ||
		u.Location != "Earth" || u.PublicEmail != "d@x" || u.Organization != "Acme" || u.CreatedAt == "" {
		t.Errorf("Deployable.User = %#v", u)
	}
}

// assertDeployableCommit validates the documented deployable.commit subset.
func assertDeployableCommit(t *testing.T, c *DeployableCommitOutput) {
	t.Helper()
	if c == nil || c.ID != "abc123def" || c.ShortID != "abc123" || c.Title != "Deploy fix" ||
		c.AuthorEmail != "dev@x" || c.CommittedDate == "" || len(c.ParentIDs) != 1 || c.ParentIDs[0] != "p1" {
		t.Errorf("Deployable.Commit = %#v", c)
	}
}

// assertDeployablePipeline validates the documented deployable.pipeline subset.
func assertDeployablePipeline(t *testing.T, p *DeployablePipelineOutput) {
	t.Helper()
	if p == nil || p.ID != 700 || p.WebURL != "https://pipe" || p.SHA != "abc123" || p.Ref != "main" || p.Status != "success" {
		t.Errorf("Deployable.Pipeline = %#v", p)
	}
}

// assertDeployableRunner validates the documented deployable.runner subset.
func assertDeployableRunner(t *testing.T, r *DeployableRunnerOutput) {
	t.Helper()
	if r == nil || r.ID != 55 || r.Name != "runner-1" || !r.IsShared ||
		r.RunnerType != "instance_type" || !r.Online || r.Status != "online" {
		t.Errorf("Deployable.Runner = %#v", r)
	}
}

// TestEnvironmentGet_NilNestedObjects verifies that absent nested objects map to
// nil pointers (no panic, no empty structs) when the API omits them.
func TestEnvironmentGet_NilNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/8" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":8,"name":"review","slug":"review","state":"available"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 8})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ClusterAgent != nil {
		t.Errorf("ClusterAgent = %#v, want nil", out.ClusterAgent)
	}
	if out.LastDeployment != nil {
		t.Errorf("LastDeployment = %#v, want nil", out.LastDeployment)
	}
}

// TestEnvironmentGet_DeploymentWithoutDeployable verifies a deployment whose
// deployable carries no identity (zero id, empty name) maps to a nil Deployable.
func TestEnvironmentGet_DeploymentWithoutDeployable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/9" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":9,"name":"staging","slug":"staging","state":"available",
				"last_deployment":{"id":1,"ref":"main","status":"running"}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 9})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.LastDeployment == nil || out.LastDeployment.ID != 1 {
		t.Fatalf("LastDeployment = %#v", out.LastDeployment)
	}
	if out.LastDeployment.Deployable != nil {
		t.Errorf("Deployable = %#v, want nil", out.LastDeployment.Deployable)
	}
	if out.LastDeployment.User != nil {
		t.Errorf("User = %#v, want nil", out.LastDeployment.User)
	}
}

// TestEnvironmentGet_DeployableWithoutNestedRefs verifies a deployable that has
// identity but omits its user, commit, and runner references maps each to a nil
// pointer (covering the nil-guard branches of the deployable converters).
func TestEnvironmentGet_DeployableWithoutNestedRefs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/10" {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"name":"qa","slug":"qa","state":"available",
				"last_deployment":{"id":2,"ref":"main","status":"success",
					"deployable":{"id":3,"name":"deploy-qa","status":"success"}}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	dep := out.LastDeployment.Deployable
	if dep == nil || dep.ID != 3 {
		t.Fatalf("Deployable = %#v", dep)
	}
	if dep.User != nil {
		t.Errorf("Deployable.User = %#v, want nil", dep.User)
	}
	if dep.Commit != nil {
		t.Errorf("Deployable.Commit = %#v, want nil", dep.Commit)
	}
	if dep.Runner != nil {
		t.Errorf("Deployable.Runner = %#v, want nil", dep.Runner)
	}
}

// TestEnvironmentList_OrderBySortKeyset verifies that List forwards order_by,
// sort, pagination=keyset, and page_token to the GitLab API query string.
func TestEnvironmentList_OrderBySortKeyset(t *testing.T) {
	var gotQuery string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"production","slug":"production","state":"available"}]`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		OrderBy:    "name",
		Sort:       "desc",
		Pagination: "keyset",
		PageToken:  "tok42",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{"order_by=name", "sort=desc", "pagination=keyset", "page_token=tok42"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(gotQuery, want) {
				t.Errorf("query %q missing %q", gotQuery, want)
			}
		})
	}
}

// TestEnvironmentCreate_NewOptionFields verifies that Create forwards the
// additive cluster_agent_id, kubernetes_namespace, flux_resource_path, and
// auto_stop_setting options to the GitLab API.
func TestEnvironmentCreate_NewOptionFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			body = string(buf)
			testutil.RespondJSON(w, http.StatusCreated, envFullJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	agentID := int64(11)
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:           "42",
		Name:                "production",
		ClusterAgentID:      &agentID,
		KubernetesNamespace: "prod-ns",
		FluxResourcePath:    "flux/prod",
		AutoStopSetting:     "with_action",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{`"cluster_agent_id":11`, `"kubernetes_namespace":"prod-ns"`, `"flux_resource_path":"flux/prod"`, `"auto_stop_setting":"with_action"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("request body %q missing %q", body, want)
			}
		})
	}
	if out.ClusterAgent == nil || out.ClusterAgent.ID != 11 {
		t.Errorf("ClusterAgent = %#v", out.ClusterAgent)
	}
}

// TestEnvironmentUpdate_NewOptionFields verifies that Update forwards the
// additive cluster_agent_id, kubernetes_namespace, flux_resource_path, and
// auto_stop_setting options to the GitLab API.
func TestEnvironmentUpdate_NewOptionFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			body = string(buf)
			testutil.RespondJSON(w, http.StatusOK, envFullJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	agentID := int64(11)
	_, err := Update(context.Background(), client, UpdateInput{
		ProjectID:           "42",
		EnvironmentID:       7,
		ClusterAgentID:      &agentID,
		KubernetesNamespace: "prod-ns",
		FluxResourcePath:    "flux/prod",
		AutoStopSetting:     "with_action",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	for _, want := range []string{`"cluster_agent_id":11`, `"kubernetes_namespace":"prod-ns"`, `"flux_resource_path":"flux/prod"`, `"auto_stop_setting":"with_action"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("request body %q missing %q", body, want)
			}
		})
	}
}

// TestActionSpecs_UpdateDeleteMetadata verifies the 1:1 R-META metadata for the
// previously generic-flagged update and delete environment tools: specific
// usage, non-toolname aliases, related actions, and a "Returns:/See also:"
// individual-tool description.
func TestActionSpecs_UpdateDeleteMetadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := environmentSpecsByTool(t, ActionSpecs(client))

	for _, tt := range []struct {
		name        string
		tool        string
		wantInUsage string
		wantInDesc  []string
	}{
		{"update", "gitlab_environment_update", "Update an existing environment", []string{"Returns:", "See also:", "gitlab_environment_get"}},
		{"delete", "gitlab_environment_delete", "Delete an environment", []string{"Returns:", "See also:", "gitlab_environment_stop"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := byTool[tt.tool]
			if !strings.Contains(spec.Usage, tt.wantInUsage) {
				t.Errorf("%s usage = %q, want substring %q", tt.tool, spec.Usage, tt.wantInUsage)
			}
			if strings.Contains(spec.Usage, "Use to execute") {
				t.Errorf("%s usage still generic: %q", tt.tool, spec.Usage)
			}
			for _, a := range spec.Aliases {
				if a == tt.tool {
					t.Errorf("%s aliases still contain only the tool name: %v", tt.tool, spec.Aliases)
				}
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s has empty RelatedActions", tt.tool)
			}
			for _, w := range tt.wantInDesc {
				if !strings.Contains(spec.IndividualTool.Description, w) {
					t.Errorf("%s description = %q, want substring %q", tt.tool, spec.IndividualTool.Description, w)
				}
			}
		})
	}
}

// envProjectJSON is a single-environment API response whose project object
// carries every one of the twenty-four basic_project_details keys GitLab
// renders under an environment, each with a value distinctive enough that a
// converter reading the wrong source field cannot produce it.
const envProjectJSON = `{
	"id":7,"name":"production","slug":"production","state":"available",
	"project":{
		"id":42,
		"description":"The project this environment belongs to",
		"name":"api",
		"name_with_namespace":"Acme / api",
		"path":"api",
		"path_with_namespace":"acme/api",
		"created_at":"2026-01-02T03:04:05Z",
		"default_branch":"main",
		"tag_list":["legacy-tag"],
		"topics":["go","mcp"],
		"ssh_url_to_repo":"git@example.com:acme/api.git",
		"http_url_to_repo":"https://example.com/acme/api.git",
		"web_url":"https://example.com/acme/api",
		"readme_url":"https://example.com/acme/api/-/blob/main/README.md",
		"forks_count":9,
		"license_url":"https://example.com/acme/api/-/blob/main/LICENSE",
		"license":{"key":"mit","name":"MIT License","nickname":"MIT","html_url":"https://licenses.example/mit","source_url":"https://licenses.example/mit.txt"},
		"avatar_url":"https://example.com/uploads/project.png",
		"star_count":13,
		"last_activity_at":"2026-02-03T04:05:06Z",
		"visibility":"internal",
		"namespace":{"id":5,"name":"Acme","path":"acme","kind":"group","full_path":"acme","parent_id":2,"avatar_url":"https://example.com/uploads/group.png","web_url":"https://example.com/groups/acme"},
		"custom_attributes":[{"key":"cost_center","value":"platform"}],
		"repository_storage":"nfs-01"
	}
}`

// TestEnvironmentGet_ProjectObject verifies that projectOutput surfaces every
// key of the basic_project_details object GitLab renders under an environment,
// the nested license and namespace objects included. The project is the one
// nested object of an environment nothing else in this package exercises, and
// a converter that reads the wrong source field is invisible without a
// per-field assertion, so each key is checked on its own.
func TestEnvironmentGet_ProjectObject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/7" {
			testutil.RespondJSON(w, http.StatusOK, envProjectJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 7})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	p := out.Project
	if p == nil {
		t.Fatal("Project is nil, want the project object the answer carries")
	}

	for _, tt := range []struct {
		name string
		got  any
		want any
	}{
		{"id", p.ID, int64(42)},
		{"description", p.Description, "The project this environment belongs to"},
		{"name", p.Name, "api"},
		{"name_with_namespace", p.NameWithNamespace, "Acme / api"},
		{"path", p.Path, "api"},
		{"path_with_namespace", p.PathWithNamespace, "acme/api"},
		{"created_at", p.CreatedAt, "2026-01-02T03:04:05Z"},
		{"default_branch", p.DefaultBranch, "main"},
		{"tag_list", p.TagList, []string{"legacy-tag"}},
		{"topics", p.Topics, []string{"go", "mcp"}},
		{"ssh_url_to_repo", p.SSHURLToRepo, "git@example.com:acme/api.git"},
		{"http_url_to_repo", p.HTTPURLToRepo, "https://example.com/acme/api.git"},
		{"web_url", p.WebURL, "https://example.com/acme/api"},
		{"readme_url", p.ReadmeURL, "https://example.com/acme/api/-/blob/main/README.md"},
		{"forks_count", p.ForksCount, int64(9)},
		{"license_url", p.LicenseURL, "https://example.com/acme/api/-/blob/main/LICENSE"},
		{"license", p.License, &ProjectLicenseOutput{
			Key:       "mit",
			Name:      "MIT License",
			Nickname:  "MIT",
			HTMLURL:   "https://licenses.example/mit",
			SourceURL: "https://licenses.example/mit.txt",
		}},
		{"avatar_url", p.AvatarURL, "https://example.com/uploads/project.png"},
		{"star_count", p.StarCount, int64(13)},
		{"last_activity_at", p.LastActivityAt, "2026-02-03T04:05:06Z"},
		{"visibility", p.Visibility, "internal"},
		{"namespace", p.Namespace, &ProjectNamespaceOutput{
			ID:        5,
			Name:      "Acme",
			Path:      "acme",
			Kind:      "group",
			FullPath:  "acme",
			ParentID:  2,
			AvatarURL: "https://example.com/uploads/group.png",
			WebURL:    "https://example.com/groups/acme",
		}},
		{"custom_attributes", p.CustomAttributes, []toolutil.CustomAttributeOutput{{Key: "cost_center", Value: "platform"}}},
		{"repository_storage", p.RepositoryStorage, "nfs-01"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("%s = %#v, want %#v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestEnvironmentGet_ProjectWithoutNestedObjects verifies the other side of
// every guard in projectOutput: a project GitLab sends without a license,
// without a namespace and without timestamps produces nil pointers and empty
// strings rather than zero-valued objects, and a null entry in
// custom_attributes is skipped rather than dereferenced.
func TestEnvironmentGet_ProjectWithoutNestedObjects(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/9" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":9,"name":"review","slug":"review","state":"available",
				"project":{"id":43,"name":"unlicensed","custom_attributes":[null,{"key":"tier","value":"free"}]}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 9})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	p := out.Project
	if p == nil {
		t.Fatal("Project is nil, want the project object the answer carries")
	}
	if p.License != nil {
		t.Errorf("License = %#v, want nil", p.License)
	}
	if p.Namespace != nil {
		t.Errorf("Namespace = %#v, want nil", p.Namespace)
	}
	if p.CreatedAt != "" || p.LastActivityAt != "" {
		t.Errorf("timestamps = %q and %q, want both empty", p.CreatedAt, p.LastActivityAt)
	}
	want := []toolutil.CustomAttributeOutput{{Key: "tier", Value: "free"}}
	if !reflect.DeepEqual(p.CustomAttributes, want) {
		t.Errorf("CustomAttributes = %#v, want %#v", p.CustomAttributes, want)
	}
}

// TestEnvironmentGet_WithoutProject verifies that an environment GitLab sends
// with no project object surfaces a nil Project rather than an empty one, so a
// reader can tell "no project was sent" from "a project with no fields".
func TestEnvironmentGet_WithoutProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/10" {
			testutil.RespondJSON(w, http.StatusOK, `{"id":10,"name":"review","slug":"review","state":"available"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: 10})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Project != nil {
		t.Errorf("Project = %#v, want nil", out.Project)
	}
}

// ---------------------------------------------------------------------------
// The branches the card and the deployable guard take when GitLab sends less
// ---------------------------------------------------------------------------.

// TestActionSpecs_EnvironmentGetRoute_RefusalThatIsNotNotFound asserts that the
// get route answers a refusal that is not a 404 with the error itself. The
// wrapper turns a 404 into the not-found card, and it must turn nothing else
// into one: a 403 reported as "environment not found" tells a model to go
// looking for an id that is there and that its token may not read.
func TestActionSpecs_EnvironmentGetRoute_RefusalThatIsNotNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, msgForbiddenBody)
	}))
	byTool := environmentSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_environment_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "environment_id": 7})
	if err == nil {
		t.Fatalf("Route.Handler returned %#v and no error, want the refusal", result)
	}
	if _, isNotFound := result.(environmentNotFoundOutput); isNotFound {
		t.Errorf("Route.Handler answered a 403 with the not-found card: %#v", result)
	}
	if !strings.Contains(err.Error(), forbiddenMessage) {
		t.Errorf("error %q does not carry GitLab's own message %q", err, forbiddenMessage)
	}
}

// TestFormatOutputMarkdown_DeploymentGitLabSentLittleOf pins the card of an
// environment whose deployment carries no status, no deploying user and no job:
// each of those rows is left out rather than written empty, and the pipeline
// row in particular must not be reached through the absent job.
func TestFormatOutputMarkdown_DeploymentGitLabSentLittleOf(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:    3,
		Name:  "dev",
		State: "available",
		LastDeployment: &DeploymentOutput{
			ID:        90,
			IID:       2,
			Ref:       "main",
			SHA:       "0badcafe1234",
			CreatedAt: "2026-03-01T10:00:00Z",
		},
	})

	want := "## Environment: dev\n\n" +
		"- **ID**: 3\n" +
		"- **State**: available\n" +
		"\n### Last Deployment\n\n" +
		"- **ID**: 90\n" +
		"- **IID**: 2\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `0badcafe`\n" +
		"- **Created**: 1 Mar 2026 10:00 UTC\n" +
		availableHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_DeployableWithoutAPipeline pins the card of a
// deployment whose job GitLab sent without the pipeline that ran it: the job is
// there, so the section is written, and the pipeline row is the only one left
// out.
func TestFormatOutputMarkdown_DeployableWithoutAPipeline(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:    3,
		Name:  "dev",
		State: "available",
		LastDeployment: &DeploymentOutput{
			ID:         91,
			Status:     "success",
			Ref:        "main",
			Deployable: &DeployableOutput{ID: 900, Name: "deploy-dev"},
		},
	})

	want := "## Environment: dev\n\n" +
		"- **ID**: 3\n" +
		"- **State**: available\n" +
		"\n### Last Deployment\n\n" +
		"- **ID**: 91\n" +
		"- **Status**: ✅ success\n" +
		"- **Ref**: main\n" +
		availableHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_SHAShorterThanTheShortForm asserts that a SHA GitLab
// already sent short is written whole rather than sliced. Eight characters is
// the length the card shortens to, so at exactly eight the guard and the slice
// below it produce the same string: no mutation of that comparison can fail,
// and this test exists for the branch, not for the mutant.
func TestFormatOutputMarkdown_SHAShorterThanTheShortForm(t *testing.T) {
	for _, tt := range []struct {
		name string
		sha  string
	}{
		{name: "exactly the short form", sha: "0badcafe"},
		{name: "shorter than the short form", sha: "0badca"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdown(Output{
				ID:             3,
				Name:           "dev",
				State:          "available",
				LastDeployment: &DeploymentOutput{ID: 92, SHA: tt.sha},
			})

			want := "## Environment: dev\n\n" +
				"- **ID**: 3\n" +
				"- **State**: available\n" +
				"\n### Last Deployment\n\n" +
				"- **ID**: 92\n" +
				"- **SHA**: `" + tt.sha + "`\n" +
				availableHints

			if got != want {
				t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestEnvironmentGet_DeployableWithOneIdentityField asserts that a job carrying
// either half of its identity is surfaced. The converter drops a job only when
// GitLab sent neither an id nor a name, which is how an absent job arrives in a
// value-typed field; dropping one that carries either would hide the job that
// put the code there.
func TestEnvironmentGet_DeployableWithOneIdentityField(t *testing.T) {
	for _, tt := range []struct {
		name       string
		envID      int64
		deployJSON string
		wantID     int64
		wantName   string
	}{
		{name: "an id and no name", envID: 11, deployJSON: `{"id":900,"status":"running"}`, wantID: 900},
		{name: "a name and no id", envID: 12, deployJSON: `{"name":"deploy-qa","status":"running"}`, wantName: "deploy-qa"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/environments/"+strconv.FormatInt(tt.envID, 10) {
					testutil.RespondJSON(w, http.StatusOK, `{"id":`+strconv.FormatInt(tt.envID, 10)+`,"name":"qa","slug":"qa","state":"available",
						"last_deployment":{"id":2,"ref":"main","status":"running","deployable":`+tt.deployJSON+`}}`)
					return
				}
				testutil.RespondJSON(w, http.StatusNotFound, msgNotFoundBody)
			}))

			out, err := Get(context.Background(), client, GetInput{ProjectID: "42", EnvironmentID: tt.envID})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			dep := out.LastDeployment.Deployable
			if dep == nil {
				t.Fatal("Deployable is nil; a job carrying half its identity was dropped")
			}
			if dep.ID != tt.wantID || dep.Name != tt.wantName {
				t.Errorf("Deployable id/name = %d/%q, want %d/%q", dep.ID, dep.Name, tt.wantID, tt.wantName)
			}
		})
	}
}
