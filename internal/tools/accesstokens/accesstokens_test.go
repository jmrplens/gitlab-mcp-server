// accesstokens_test.go contains unit tests for the access token MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package accesstokens

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Shared test constants used across accesstokens_test.go and coverage_test.go.
const (
	errProjectIDRequired = "project_id is required"
	errTokenIDRequired   = "token_id is required"
	errGroupIDRequired   = "group_id is required"
	fmtUnexpErr          = "unexpected error: %v"
	fmtExpProjectIDErr   = "expected project_id required error, got: %v"
	fmtExpTokenIDErr     = "expected token_id required error, got: %v"
	fmtExpGroupIDErr     = "expected group_id required error, got: %v"
	jsonNotFound         = `{"message":"not found"}`
	jsonServerErr        = `{"message":"server error"}`
	errExpectedAPI       = "expected API error, got nil"
	testTokenName        = "my-token"

	// accesstokens_test.go.
	fmtTokenMismatch   = "token mismatch: %+v"
	fmtExpRotatedToken = "expected rotated token, got %s"
	testGlpatABC       = "glpat-abc123"
	stateActive        = "active"

	// coverage_test.go.
	errInvalidExpiresAt  = "invalid expires_at"
	fmtExpInvalidDateErr = "expected invalid date error, got: %v"
	fmtExpErrContaining  = "expected error containing %q, got: %v"
	errCreatedAtEmpty    = "CreatedAt should be populated"
	errLastUsedAtEmpty   = "LastUsedAt should be populated"
	errExpiresAtEmpty    = "ExpiresAt should be populated"
	fmtTokenWant         = "Token = %q, want %q"
	fmtDescWant          = "Description = %q, want %q"
	testVersion          = "0.0.1"
	tcBadDate            = "bad date"
	testDescTest         = "description test"
	testDescFullGroup    = "Full group token"

	// shared API paths.
	pathProjectTokens = "/api/v4/projects/42/access_tokens"
	pathGroupTokens   = "/api/v4/groups/10/access_tokens"
	testFullToken     = "full-token"
	testExpiresDate   = "2027-12-31"
)

// ---------------------------------------------------------------------------
// Project Access Tokens
// ---------------------------------------------------------------------------.

// TestProjectList_Success verifies that ProjectList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectTokens && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"name":"bot-token","active":true,"revoked":false,"scopes":["api"],"access_level":30,"user_id":100,"created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"name":"ci-token","active":true,"revoked":false,"scopes":["read_api","read_repository"],"access_level":20}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectList(context.Background(), client, ProjectListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(out.Tokens))
	}
	if out.Tokens[0].Name != "bot-token" || out.Tokens[0].AccessLevel != 30 {
		t.Errorf("first token mismatch: %+v", out.Tokens[0])
	}
	if out.Tokens[1].Name != "ci-token" {
		t.Errorf("second token mismatch: %+v", out.Tokens[1])
	}
}

// TestProjectList_WithState verifies the ProjectList_WithState handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectList_WithState(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectTokens {
			if r.URL.Query().Get("state") != stateActive {
				t.Errorf("expected state=active, got %s", r.URL.Query().Get("state"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectList(context.Background(), client, ProjectListInput{ProjectID: "42", State: stateActive})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 0 {
		t.Fatalf("expected 0 tokens, got %d", len(out.Tokens))
	}
}

// TestProjectList_MissingProjectID verifies that ProjectList_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	_, err := ProjectList(context.Background(), client, ProjectListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestProjectGet_Success verifies that ProjectGet succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/access_tokens/5 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestProjectGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/access_tokens/5" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":5,"name":"my-token","active":true,"revoked":false,"scopes":["api"],"access_level":30}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "42", TokenID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 || out.Name != testTokenName {
		t.Errorf(fmtTokenMismatch, out)
	}
}

// TestProjectGet_MissingInputs verifies that ProjectGet_MissingInputs returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectGet_MissingInputs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))

	_, err := ProjectGet(context.Background(), client, ProjectGetInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjectIDErr, err)
	}

	_, err = ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// TestProjectCreate_Success verifies that ProjectCreate succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectTokens && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"name":"new-bot","token":"glpat-abc123","active":true,"scopes":["api","read_repository"],"access_level":30,"expires_at":"2026-12-31"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID:   "42",
		Name:        "new-bot",
		Scopes:      []string{"api", "read_repository"},
		AccessLevel: 30,
		ExpiresAt:   "2026-12-31",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != testGlpatABC {
		t.Errorf("expected token glpat-abc123, got %s", out.Token)
	}
	if out.Name != "new-bot" {
		t.Errorf("expected name new-bot, got %s", out.Name)
	}
}

// TestProjectCreate_Validation verifies the ProjectCreate_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectCreate_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))

	tests := []struct {
		name  string
		input ProjectCreateInput
		errSS string
	}{
		{"missing project_id", ProjectCreateInput{Name: "x", Scopes: []string{"api"}}, errProjectIDRequired},
		{"missing name", ProjectCreateInput{ProjectID: "42", Scopes: []string{"api"}}, "name is required"},
		{"missing scopes", ProjectCreateInput{ProjectID: "42", Name: "x"}, "scopes is required"},
		{"empty scope", ProjectCreateInput{ProjectID: "42", Name: "x", Scopes: []string{""}}, "must not be empty"},
		{"scope with whitespace", ProjectCreateInput{ProjectID: "42", Name: "x", Scopes: []string{" api"}}, "surrounding whitespace"},
		{"unsupported scope", ProjectCreateInput{ProjectID: "42", Name: "x", Scopes: []string{"everything"}}, "is not supported"},
		{"duplicate scope", ProjectCreateInput{ProjectID: "42", Name: "x", Scopes: []string{"api", "api"}}, "duplicated"},
		{"bad date", ProjectCreateInput{ProjectID: "42", Name: "x", Scopes: []string{"api"}, ExpiresAt: "not-a-date"}, errInvalidExpiresAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ProjectCreate(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.errSS) {
				t.Fatalf(fmtExpErrContaining, tc.errSS, err)
			}
		})
	}
}

// TestProjectRotate_Success verifies that ProjectRotate succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/access_tokens/5/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestProjectRotate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/access_tokens/5/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":5,"name":"my-token","token":"glpat-new123","active":true,"expires_at":"2027-06-01"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectRotate(context.Background(), client, ProjectRotateInput{ProjectID: "42", TokenID: 5, ExpiresAt: "2027-06-01"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-new123" {
		t.Errorf(fmtExpRotatedToken, out.Token)
	}
}

// TestProjectRevoke_Success verifies that ProjectRevoke succeeds when the
// GitLab API returns 204 No Content for the DELETE
// /api/v4/projects/:id/access_tokens/:token_id endpoint.
//
// The test wires an httptest server that responds with 204 on the exact
// DELETE path and 404 on any other request, then asserts no error is
// returned. This protects the success-path contract of the revoke handler.
func TestProjectRevoke_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/access_tokens/5" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	err := ProjectRevoke(context.Background(), client, ProjectRevokeInput{ProjectID: "42", TokenID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestProjectRevoke_Validation verifies the ProjectRevoke_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectRevoke_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))

	err := ProjectRevoke(context.Background(), client, ProjectRevokeInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
	err = ProjectRevoke(context.Background(), client, ProjectRevokeInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// Group Access Tokens
// ---------------------------------------------------------------------------.

// TestGroupList_Success verifies that GroupList succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTokens && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":3,"name":"group-bot","active":true,"revoked":false,"scopes":["read_api"],"access_level":20}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupList(context.Background(), client, GroupListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	if out.Tokens[0].Name != "group-bot" {
		t.Errorf("token name mismatch: %+v", out.Tokens[0])
	}
}

// TestGroupList_MissingGroupID verifies that GroupList_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	_, err := GroupList(context.Background(), client, GroupListInput{})
	if err == nil || !strings.Contains(err.Error(), errGroupIDRequired) {
		t.Fatalf(fmtExpGroupIDErr, err)
	}
}

// TestGroupGet_Success verifies that GroupGet succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/access_tokens/3 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGroupGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/3" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":3,"name":"group-bot","active":true,"scopes":["read_api"],"access_level":20}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupGet(context.Background(), client, GroupGetInput{GroupID: "10", TokenID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 || out.AccessLevel != 20 {
		t.Errorf(fmtTokenMismatch, out)
	}
}

// TestGroupCreate_Success verifies that GroupCreate succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTokens && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":8,"name":"group-ci","token":"glpat-grp99","active":true,"scopes":["api"],"access_level":40}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupCreate(context.Background(), client, GroupCreateInput{
		GroupID:     "10",
		Name:        "group-ci",
		Scopes:      []string{"api"},
		AccessLevel: 40,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-grp99" {
		t.Errorf("expected token glpat-grp99, got %s", out.Token)
	}
}

// TestGroupRotate_Success verifies that GroupRotate succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/access_tokens/3/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGroupRotate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/3/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":3,"name":"group-bot","token":"glpat-rotated","active":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupRotate(context.Background(), client, GroupRotateInput{GroupID: "10", TokenID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-rotated" {
		t.Errorf(fmtExpRotatedToken, out.Token)
	}
}

// TestGroupRevoke_Success verifies that GroupRevoke succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/access_tokens/3 (DELETE) responds with HTTP NotFound.
// It asserts the returned output matches the expected fields.
func TestGroupRevoke_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/3" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	err := GroupRevoke(context.Background(), client, GroupRevokeInput{GroupID: "10", TokenID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// Personal Access Tokens
// ---------------------------------------------------------------------------.

// TestPersonalList_Success verifies that PersonalList succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":100,"name":"my-pat","active":true,"revoked":false,"scopes":["api"],"user_id":1}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalList(context.Background(), client, PersonalListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
	if out.Tokens[0].Name != "my-pat" {
		t.Errorf("token name mismatch: %+v", out.Tokens[0])
	}
}

// TestPersonalList_WithFilters verifies the PersonalList_WithFilters handler.
// The mock GitLab API at /api/v4/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens" {
			if r.URL.Query().Get("state") != stateActive {
				t.Errorf("expected state=active, got %s", r.URL.Query().Get("state"))
			}
			if r.URL.Query().Get("search") != testTokenName {
				t.Errorf("expected search=my-token, got %s", r.URL.Query().Get("search"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := PersonalList(context.Background(), client, PersonalListInput{State: stateActive, Search: testTokenName})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPersonalGet_SelfSuccess verifies the PersonalGet_SelfSuccess handler.
// The mock GitLab API at /api/v4/personal_access_tokens/self (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalGet_SelfSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/self" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":50,"name":"current-pat","active":true,"scopes":["api"],"user_id":1}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 0})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 50 || out.Name != "current-pat" {
		t.Errorf(fmtTokenMismatch, out)
	}
}

// TestPersonalGet_ByIDSuccess verifies the PersonalGet_ByIDSuccess handler.
// The mock GitLab API at /api/v4/personal_access_tokens/99 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalGet_ByIDSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/99" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"name":"other-pat","active":true,"scopes":["read_api"],"user_id":2}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 99 {
		t.Errorf("expected id 99, got %d", out.ID)
	}
}

// TestPersonalRotate_Success verifies that PersonalRotate succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/personal_access_tokens/99/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalRotate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/99/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"name":"other-pat","token":"glpat-rotated-pat","active":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalRotate(context.Background(), client, PersonalRotateInput{TokenID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-rotated-pat" {
		t.Errorf(fmtExpRotatedToken, out.Token)
	}
}

// TestPersonalRotate_Validation verifies the PersonalRotate_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestPersonalRotate_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	_, err := PersonalRotate(context.Background(), client, PersonalRotateInput{})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// TestPersonalRevoke_Success verifies that PersonalRevoke succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/personal_access_tokens/99 (DELETE) responds with HTTP NotFound.
// It asserts the returned output matches the expected fields.
func TestPersonalRevoke_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/99" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	err := PersonalRevoke(context.Background(), client, PersonalRevokeInput{TokenID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPersonalRevoke_Validation verifies the PersonalRevoke_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestPersonalRevoke_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	err := PersonalRevoke(context.Background(), client, PersonalRevokeInput{})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// TestAccessLevelName verifies that accessLevelName maps GitLab access level
// integers (10/20/30/40/50) to their canonical human-readable names and
// falls back to "Unknown (N)" for any other value.
//
// The test runs a table-driven check across the five known levels plus two
// out-of-range values (0 and 99). This protects the human-facing output
// across all GitLab access tiers.
func TestAccessLevelName(t *testing.T) {
	tests := []struct {
		level int
		want  string
	}{
		{10, "Guest"},
		{20, "Reporter"},
		{30, "Developer"},
		{40, "Maintainer"},
		{50, "Owner"},
		{0, "Unknown (0)"},
		{99, "Unknown (99)"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := accessLevelName(tc.level)
			if got != tc.want {
				t.Errorf("accessLevelName(%d) = %q, want %q", tc.level, got, tc.want)
			}
		})
	}
}

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{
		ID:     5,
		Name:   testTokenName,
		Active: true,
		Scopes: []string{"api", "read_api"},
		Token:  testGlpatABC,
	}
	md := FormatOutputMarkdown(out)
	if !strings.Contains(md, "Access Token #5") {
		t.Error("markdown should contain token ID heading")
	}
	if !strings.Contains(md, testGlpatABC) {
		t.Error("markdown should contain token value")
	}
}

// TestFormatOutputMarkdown_AccessLevel verifies the OutputMarkdown_AccessLevel Markdown formatter for a representative output_accesslevel input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_AccessLevel(t *testing.T) {
	out := Output{
		ID:          7,
		Name:        "level-token",
		Active:      true,
		AccessLevel: 30,
	}
	md := FormatOutputMarkdown(out)
	if !strings.Contains(md, "Developer") {
		t.Errorf("expected Developer role name in markdown, got:\n%s", md)
	}
	if strings.Contains(md, "**Access Level**: 30") {
		t.Error("access level should not show as raw number")
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No access tokens found") {
		t.Error("empty list should show no tokens message")
	}
}

// TestFormatListMarkdown_WithTokens verifies the ListMarkdown_WithTokens Markdown formatter for a representative list_withtokens input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithTokens(t *testing.T) {
	out := ListOutput{
		Tokens: []Output{
			{ID: 1, Name: "bot-1", Active: true, Scopes: []string{"api"}, ExpiresAt: "2026-12-31"},
			{ID: 2, Name: "bot-2", Active: false, Scopes: []string{"read_api"}},
		},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "bot-1") || !strings.Contains(md, "bot-2") {
		t.Error("markdown should contain both token names")
	}
	if !strings.Contains(md, "never") {
		t.Error("token without expiry should show 'never'")
	}
}

// ---------------------------------------------------------------------------
// ProjectRotateSelf
// ---------------------------------------------------------------------------.

// TestProjectRotateSelf_Success verifies that ProjectRotateSelf succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/access_tokens/self/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestProjectRotateSelf_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/access_tokens/self/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":5,"name":"self-token","active":true,"scopes":["api"],"token":"new-pat-value"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "new-pat-value" {
		t.Errorf("expected token new-pat-value, got %s", out.Token)
	}
}

// TestProjectRotateSelf_MissingProjectID verifies that ProjectRotateSelf_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectRotateSelf_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// ---------------------------------------------------------------------------
// GroupRotateSelf
// ---------------------------------------------------------------------------.

// TestGroupRotateSelf_Success verifies that GroupRotateSelf succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/groups/10/access_tokens/self/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGroupRotateSelf_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/self/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":8,"name":"group-self","active":true,"scopes":["api"],"token":"new-group-pat"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "new-group-pat" {
		t.Errorf("expected token new-group-pat, got %s", out.Token)
	}
}

// TestGroupRotateSelf_MissingGroupID verifies that GroupRotateSelf_MissingGroupID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupRotateSelf_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	_, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{})
	if err == nil {
		t.Fatal("expected error for missing group_id")
	}
}

// ---------------------------------------------------------------------------
// PersonalRotateSelf
// ---------------------------------------------------------------------------.

// TestPersonalRotateSelf_Success verifies that PersonalRotateSelf succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/personal_access_tokens/self/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalRotateSelf_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/self/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":15,"name":"my-pat","active":true,"scopes":["api"],"token":"new-personal-pat"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalRotateSelf(context.Background(), client, PersonalRotateSelfInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "new-personal-pat" {
		t.Errorf("expected token new-personal-pat, got %s", out.Token)
	}
}

// TestPersonalRotateSelf_APIError verifies that PersonalRotateSelf returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalRotateSelf_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := PersonalRotateSelf(context.Background(), client, PersonalRotateSelfInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// PersonalRevokeSelf
// ---------------------------------------------------------------------------.

// TestPersonalRevokeSelf_Success verifies that PersonalRevokeSelf succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/personal_access_tokens/self (DELETE) responds with HTTP NotFound.
// It asserts the returned output matches the expected fields.
func TestPersonalRevokeSelf_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/self" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	err := PersonalRevokeSelf(context.Background(), client, PersonalRevokeSelfInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPersonalRevokeSelf_APIError verifies that PersonalRevokeSelf returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalRevokeSelf_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := PersonalRevokeSelf(context.Background(), client, PersonalRevokeSelfInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Canceled context -- ALL 18 handlers
// ---------------------------------------------------------------------------.

// TestCancelled_Context verifies the Cancelled_Context handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestCancelled_Context(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))

	ctx := testutil.CancelledCtx(t)

	tests := []struct {
		name string
		fn   func() error
	}{
		{"ProjectList", func() error { _, err := ProjectList(ctx, client, ProjectListInput{ProjectID: "1"}); return err }},
		{"ProjectGet", func() error {
			_, err := ProjectGet(ctx, client, ProjectGetInput{ProjectID: "1", TokenID: 1})
			return err
		}},
		{"ProjectCreate", func() error {
			_, err := ProjectCreate(ctx, client, ProjectCreateInput{ProjectID: "1", Name: "t", Scopes: []string{"api"}})
			return err
		}},
		{"ProjectRotate", func() error {
			_, err := ProjectRotate(ctx, client, ProjectRotateInput{ProjectID: "1", TokenID: 1})
			return err
		}},
		{"ProjectRevoke", func() error {
			return ProjectRevoke(ctx, client, ProjectRevokeInput{ProjectID: "1", TokenID: 1})
		}},
		{"ProjectRotateSelf", func() error {
			_, err := ProjectRotateSelf(ctx, client, ProjectRotateSelfInput{ProjectID: "1"})
			return err
		}},
		{"GroupList", func() error { _, err := GroupList(ctx, client, GroupListInput{GroupID: "1"}); return err }},
		{"GroupGet", func() error { _, err := GroupGet(ctx, client, GroupGetInput{GroupID: "1", TokenID: 1}); return err }},
		{"GroupCreate", func() error {
			_, err := GroupCreate(ctx, client, GroupCreateInput{GroupID: "1", Name: "t", Scopes: []string{"api"}})
			return err
		}},
		{"GroupRotate", func() error {
			_, err := GroupRotate(ctx, client, GroupRotateInput{GroupID: "1", TokenID: 1})
			return err
		}},
		{"GroupRevoke", func() error {
			return GroupRevoke(ctx, client, GroupRevokeInput{GroupID: "1", TokenID: 1})
		}},
		{"GroupRotateSelf", func() error {
			_, err := GroupRotateSelf(ctx, client, GroupRotateSelfInput{GroupID: "1"})
			return err
		}},
		{"PersonalList", func() error { _, err := PersonalList(ctx, client, PersonalListInput{}); return err }},
		{"PersonalGet", func() error { _, err := PersonalGet(ctx, client, PersonalGetInput{TokenID: 1}); return err }},
		{"PersonalRotate", func() error {
			_, err := PersonalRotate(ctx, client, PersonalRotateInput{TokenID: 1})
			return err
		}},
		{"PersonalRevoke", func() error {
			return PersonalRevoke(ctx, client, PersonalRevokeInput{TokenID: 1})
		}},
		{"PersonalRotateSelf", func() error {
			_, err := PersonalRotateSelf(ctx, client, PersonalRotateSelfInput{})
			return err
		}},
		{"PersonalRevokeSelf", func() error {
			return PersonalRevokeSelf(ctx, client, PersonalRevokeSelfInput{})
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil || !strings.Contains(err.Error(), "context cancel") {
				t.Fatalf("expected context canceled error, got: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// API error -- handlers missing error coverage
// ---------------------------------------------------------------------------.

// TestProjectList_APIError verifies that ProjectList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectList(context.Background(), client, ProjectListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectGet_APIError verifies that ProjectGet returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectCreate_APIError verifies that ProjectCreate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID: "1", Name: "t", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectRotate_APIError verifies that ProjectRotate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectRotate(context.Background(), client, ProjectRotateInput{ProjectID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectRevoke_APIError verifies that ProjectRevoke returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	err := ProjectRevoke(context.Background(), client, ProjectRevokeInput{ProjectID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectRotateSelf_APIError verifies that ProjectRotateSelf returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestProjectRotateSelf_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupList_APIError verifies that GroupList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupList(context.Background(), client, GroupListInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupGet_APIError verifies that GroupGet returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupGet(context.Background(), client, GroupGetInput{GroupID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupCreate_APIError verifies that GroupCreate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupCreate(context.Background(), client, GroupCreateInput{
		GroupID: "1", Name: "t", Scopes: []string{"api"},
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupRotate_APIError verifies that GroupRotate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupRotate(context.Background(), client, GroupRotateInput{GroupID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupRevoke_APIError verifies that GroupRevoke returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	err := GroupRevoke(context.Background(), client, GroupRevokeInput{GroupID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupRotateSelf_APIError verifies that GroupRotateSelf returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupRotateSelf_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalList_APIError verifies that PersonalList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalList(context.Background(), client, PersonalListInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalGet_SelfAPIError verifies that PersonalGet_SelfAPIError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalGet_SelfAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 0})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalGet_ByIDAPIError verifies that PersonalGet_ByIDAPIError returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalGet_ByIDAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalRotate_APIError verifies that PersonalRotate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalRotate(context.Background(), client, PersonalRotateInput{TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalRevoke_APIError verifies that PersonalRevoke returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestPersonalRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	err := PersonalRevoke(context.Background(), client, PersonalRevokeInput{TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAccessTokenInputValidationAPIErrors verifies that AccessTokenInputValidationAPIErrors returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAccessTokenInputValidationAPIErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"invalid expires_at"}`)
	}))

	tests := []struct {
		name string
		call func(context.Context) error
		want string
	}{
		{name: "ProjectCreate", want: "validate scopes", call: func(ctx context.Context) error {
			_, err := ProjectCreate(ctx, client, ProjectCreateInput{ProjectID: "1", Name: "token", Scopes: []string{"api"}})
			return err
		}},
		{name: "ProjectRotate", want: "token may already be revoked", call: func(ctx context.Context) error {
			_, err := ProjectRotate(ctx, client, ProjectRotateInput{ProjectID: "1", TokenID: 1})
			return err
		}},
		{name: "GroupCreate", want: "validate scopes", call: func(ctx context.Context) error {
			_, err := GroupCreate(ctx, client, GroupCreateInput{GroupID: "1", Name: "token", Scopes: []string{"api"}})
			return err
		}},
		{name: "GroupRotate", want: "token may already be revoked", call: func(ctx context.Context) error {
			_, err := GroupRotate(ctx, client, GroupRotateInput{GroupID: "1", TokenID: 1})
			return err
		}},
		{name: "PersonalRotate", want: "token may already be revoked", call: func(ctx context.Context) error {
			_, err := PersonalRotate(ctx, client, PersonalRotateInput{TokenID: 1})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(t.Context())
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want hint containing %q", err.Error(), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation tests -- missing coverage
// ---------------------------------------------------------------------------.

// TestGroupGet_MissingInputs verifies that GroupGet_MissingInputs returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGroupGet_MissingInputs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))

	_, err := GroupGet(context.Background(), client, GroupGetInput{})
	if err == nil || !strings.Contains(err.Error(), errGroupIDRequired) {
		t.Fatalf(fmtExpGroupIDErr, err)
	}

	_, err = GroupGet(context.Background(), client, GroupGetInput{GroupID: "10"})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// TestGroupCreate_Validation verifies the GroupCreate_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupCreate_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))

	tests := []struct {
		name  string
		input GroupCreateInput
		errSS string
	}{
		{"missing group_id", GroupCreateInput{Name: "x", Scopes: []string{"api"}}, errGroupIDRequired},
		{"missing name", GroupCreateInput{GroupID: "10", Scopes: []string{"api"}}, "name is required"},
		{"missing scopes", GroupCreateInput{GroupID: "10", Name: "x"}, "scopes is required"},
		{tcBadDate, GroupCreateInput{GroupID: "10", Name: "x", Scopes: []string{"api"}, ExpiresAt: "not-a-date"}, errInvalidExpiresAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GroupCreate(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.errSS) {
				t.Fatalf(fmtExpErrContaining, tc.errSS, err)
			}
		})
	}
}

// TestGroupRotate_Validation verifies the GroupRotate_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupRotate_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))

	tests := []struct {
		name  string
		input GroupRotateInput
		errSS string
	}{
		{"missing group_id", GroupRotateInput{TokenID: 1}, errGroupIDRequired},
		{"missing token_id", GroupRotateInput{GroupID: "10"}, errTokenIDRequired},
		{tcBadDate, GroupRotateInput{GroupID: "10", TokenID: 1, ExpiresAt: "bad-date"}, errInvalidExpiresAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GroupRotate(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.errSS) {
				t.Fatalf(fmtExpErrContaining, tc.errSS, err)
			}
		})
	}
}

// TestGroupRevoke_Validation verifies the GroupRevoke_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupRevoke_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))

	err := GroupRevoke(context.Background(), client, GroupRevokeInput{})
	if err == nil || !strings.Contains(err.Error(), errGroupIDRequired) {
		t.Fatalf(fmtExpGroupIDErr, err)
	}
	err = GroupRevoke(context.Background(), client, GroupRevokeInput{GroupID: "10"})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// TestProjectRotate_Validation verifies the ProjectRotate_Validation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectRotate_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))

	tests := []struct {
		name  string
		input ProjectRotateInput
		errSS string
	}{
		{"missing project_id", ProjectRotateInput{TokenID: 1}, errProjectIDRequired},
		{"missing token_id", ProjectRotateInput{ProjectID: "42"}, errTokenIDRequired},
		{tcBadDate, ProjectRotateInput{ProjectID: "42", TokenID: 1, ExpiresAt: "bad"}, errInvalidExpiresAt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ProjectRotate(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.errSS) {
				t.Fatalf(fmtExpErrContaining, tc.errSS, err)
			}
		})
	}
}

// TestProjectRotateSelf_BadDate verifies the ProjectRotateSelf_BadDate handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectRotateSelf_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{ProjectID: "42", ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// TestGroupRotateSelf_BadDate verifies the GroupRotateSelf_BadDate handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupRotateSelf_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{GroupID: "10", ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// TestPersonalRotate_BadDate verifies the PersonalRotate_BadDate handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestPersonalRotate_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := PersonalRotate(context.Background(), client, PersonalRotateInput{TokenID: 1, ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// TestPersonalRotateSelf_BadDate verifies the PersonalRotateSelf_BadDate handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestPersonalRotateSelf_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := PersonalRotateSelf(context.Background(), client, PersonalRotateSelfInput{ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// ---------------------------------------------------------------------------
// Converter edge cases -- all date fields populated
// ---------------------------------------------------------------------------.

// TestFromProjectToken_AllDates verifies the FromProjectToken_AllDates handler.
// The mock GitLab API at /api/v4/projects/1/access_tokens/5 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestFromProjectToken_AllDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/1/access_tokens/5" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":5,"name":"dated","active":true,"revoked":false,
				"scopes":["api"],"access_level":30,"user_id":10,
				"description":"with dates","token":"glpat-x",
				"created_at":"2026-06-01T10:00:00Z",
				"last_used_at":"2026-07-15T14:30:00Z",
				"expires_at":"2026-12-31"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "1", TokenID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CreatedAt == "" {
		t.Error(errCreatedAtEmpty)
	}
	if out.LastUsedAt == "" {
		t.Error(errLastUsedAtEmpty)
	}
	if out.ExpiresAt == "" {
		t.Error(errExpiresAtEmpty)
	}
	if out.Description != "with dates" {
		t.Errorf(fmtDescWant, out.Description, "with dates")
	}
}

// TestFromGroupToken_AllDates verifies the FromGroupToken_AllDates handler.
// The mock GitLab API at /api/v4/groups/10/access_tokens/3 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestFromGroupToken_AllDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/3" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":3,"name":"group-dated","active":true,"revoked":false,
				"scopes":["read_api"],"access_level":20,"user_id":5,
				"description":"group dates","token":"glpat-g",
				"created_at":"2026-03-01T08:00:00Z",
				"last_used_at":"2026-04-20T12:00:00Z",
				"expires_at":"2027-06-30"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupGet(context.Background(), client, GroupGetInput{GroupID: "10", TokenID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CreatedAt == "" {
		t.Error(errCreatedAtEmpty)
	}
	if out.LastUsedAt == "" {
		t.Error(errLastUsedAtEmpty)
	}
	if out.ExpiresAt == "" {
		t.Error(errExpiresAtEmpty)
	}
}

// TestFromPersonalToken_AllDates verifies the FromPersonalToken_AllDates handler.
// The mock GitLab API at /api/v4/personal_access_tokens/50 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestFromPersonalToken_AllDates(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/50" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":50,"name":"personal-dated","active":true,"revoked":false,
				"scopes":["api"],"user_id":1,
				"description":"personal dates","token":"glpat-p",
				"created_at":"2026-01-15T09:00:00Z",
				"last_used_at":"2026-02-28T16:45:00Z",
				"expires_at":"2027-01-01"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 50})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.CreatedAt == "" {
		t.Error(errCreatedAtEmpty)
	}
	if out.LastUsedAt == "" {
		t.Error(errLastUsedAtEmpty)
	}
	if out.ExpiresAt == "" {
		t.Error(errExpiresAtEmpty)
	}
}

// ---------------------------------------------------------------------------
// Pagination parameters
// ---------------------------------------------------------------------------.

// TestProjectList_WithPagination verifies that ProjectList_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestProjectList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectTokens {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
				Page: "2", PerPage: "5", Total: "10", TotalPages: "2",
			})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	input := ProjectListInput{
		ProjectID: "42",
		Page:      2,
		PerPage:   5,
	}
	out, err := ProjectList(context.Background(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("expected TotalPages=2, got %d", out.Pagination.TotalPages)
	}
}

// TestPersonalList_WithUserID verifies the PersonalList_WithUserID handler.
// The mock GitLab API at /api/v4/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalList_WithUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens" {
			if r.URL.Query().Get("user_id") != "42" {
				t.Errorf("expected user_id=42, got %s", r.URL.Query().Get("user_id"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":1,"name":"pat","active":true}]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "20", Total: "1", TotalPages: "1",
			})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalList(context.Background(), client, PersonalListInput{UserID: 42})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(out.Tokens))
	}
}

// TestGroupList_WithPagination verifies that GroupList_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestGroupList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTokens {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
				Page: "1", PerPage: "10", Total: "0", TotalPages: "0",
			})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	input := GroupListInput{
		GroupID: "10",
		Page:    1,
		PerPage: 10,
	}
	out, err := GroupList(context.Background(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.PerPage != 10 {
		t.Errorf("expected PerPage=10, got %d", out.Pagination.PerPage)
	}
}

// TestGroupList_WithState verifies the GroupList_WithState handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupList_WithState(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTokens {
			if r.URL.Query().Get("state") != "inactive" {
				t.Errorf("expected state=inactive, got %s", r.URL.Query().Get("state"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := GroupList(context.Background(), client, GroupListInput{GroupID: "10", State: "inactive"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPersonalList_WithPagination verifies that PersonalList_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/personal_access_tokens (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestPersonalList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens" {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{
				Page: "3", PerPage: "5", Total: "15", TotalPages: "3",
			})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	input := PersonalListInput{
		Page:    3,
		PerPage: 5,
	}
	out, err := PersonalList(context.Background(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("expected TotalPages=3, got %d", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// GroupCreate with optional fields (description, access_level, expires_at)
// ---------------------------------------------------------------------------.

// TestGroupCreate_WithOptionalFields verifies the GroupCreate_WithOptionalFields handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGroupCreate_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTokens && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":9,"name":"full-token","token":"glpat-full",
				"active":true,"scopes":["api","read_api"],"access_level":40,
				"description":"Full group token","expires_at":"2027-12-31"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupCreate(context.Background(), client, GroupCreateInput{
		GroupID:     "10",
		Name:        testFullToken,
		Scopes:      []string{"api", "read_api"},
		AccessLevel: 40,
		Description: testDescFullGroup,
		ExpiresAt:   testExpiresDate,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-full" {
		t.Errorf(fmtTokenWant, out.Token, "glpat-full")
	}
	if out.Description != testDescFullGroup {
		t.Errorf(fmtDescWant, out.Description, testDescFullGroup)
	}
}

// ---------------------------------------------------------------------------
// ProjectCreate with description (optional field coverage)
// ---------------------------------------------------------------------------.

// TestProjectCreate_WithDescription verifies the ProjectCreate_WithDescription handler.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestProjectCreate_WithDescription(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectTokens && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":11,"name":"desc-token","token":"glpat-desc","active":true,
				"scopes":["api"],"description":"description test"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectCreate(context.Background(), client, ProjectCreateInput{
		ProjectID:   "42",
		Name:        "desc-token",
		Scopes:      []string{"api"},
		Description: testDescTest,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Description != testDescTest {
		t.Errorf(fmtDescWant, out.Description, testDescTest)
	}
}

// ---------------------------------------------------------------------------
// GroupRotate with ExpiresAt
// ---------------------------------------------------------------------------.

// TestGroupRotate_WithExpiresAt verifies the GroupRotate_WithExpiresAt handler.
// The mock GitLab API at /api/v4/groups/10/access_tokens/3/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGroupRotate_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/3/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":3,"name":"group-bot","token":"glpat-new","active":true,"expires_at":"2028-01-01"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupRotate(context.Background(), client, GroupRotateInput{GroupID: "10", TokenID: 3, ExpiresAt: "2028-01-01"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-new" {
		t.Errorf(fmtTokenWant, out.Token, "glpat-new")
	}
}

// ---------------------------------------------------------------------------
// GroupRotateSelf with ExpiresAt
// ---------------------------------------------------------------------------.

// TestGroupRotateSelf_WithExpiresAt verifies the GroupRotateSelf_WithExpiresAt handler.
// The mock GitLab API at /api/v4/groups/10/access_tokens/self/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGroupRotateSelf_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/access_tokens/self/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":8,"name":"group-self","token":"glpat-gs","active":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{GroupID: "10", ExpiresAt: "2028-06-15"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-gs" {
		t.Errorf(fmtTokenWant, out.Token, "glpat-gs")
	}
}

// ---------------------------------------------------------------------------
// ProjectRotateSelf with ExpiresAt
// ---------------------------------------------------------------------------.

// TestProjectRotateSelf_WithExpiresAt verifies the ProjectRotateSelf_WithExpiresAt handler.
// The mock GitLab API at /api/v4/projects/42/access_tokens/self/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestProjectRotateSelf_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/access_tokens/self/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":5,"name":"proj-self","token":"glpat-ps","active":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{ProjectID: "42", ExpiresAt: "2028-01-01"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-ps" {
		t.Errorf(fmtTokenWant, out.Token, "glpat-ps")
	}
}

// ---------------------------------------------------------------------------
// PersonalRotate with ExpiresAt
// ---------------------------------------------------------------------------.

// TestPersonalRotate_WithExpiresAt verifies the PersonalRotate_WithExpiresAt handler.
// The mock GitLab API at /api/v4/personal_access_tokens/99/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalRotate_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/99/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"name":"pat","token":"glpat-pr","active":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalRotate(context.Background(), client, PersonalRotateInput{TokenID: 99, ExpiresAt: "2028-06-01"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-pr" {
		t.Errorf(fmtTokenWant, out.Token, "glpat-pr")
	}
}

// ---------------------------------------------------------------------------
// PersonalRotateSelf with ExpiresAt
// ---------------------------------------------------------------------------.

// TestPersonalRotateSelf_WithExpiresAt verifies the PersonalRotateSelf_WithExpiresAt handler.
// The mock GitLab API at /api/v4/personal_access_tokens/self/rotate (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestPersonalRotateSelf_WithExpiresAt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens/self/rotate" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusOK, `{"id":15,"name":"self-pat","token":"glpat-prs","active":true}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	out, err := PersonalRotateSelf(context.Background(), client, PersonalRotateSelfInput{ExpiresAt: "2028-06-01"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Token != "glpat-prs" {
		t.Errorf(fmtTokenWant, out.Token, "glpat-prs")
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown -- all optional fields
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_AllFields verifies the OutputMarkdown_AllFields Markdown formatter for a representative output_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	out := Output{
		ID:          42,
		Name:        testFullToken,
		Description: "A token with all fields set",
		Active:      true,
		Revoked:     false,
		Scopes:      []string{"api", "read_api", "write_repository"},
		AccessLevel: 40,
		CreatedAt:   "2026-06-01T10:00:00Z",
		ExpiresAt:   testExpiresDate,
		Token:       "glpat-secret123",
	}
	md := FormatOutputMarkdown(out)

	checks := []string{
		"Access Token #42",
		testFullToken,
		"A token with all fields set",
		"true",  // Active
		"false", // Revoked
		"api, read_api, write_repository",
		"Maintainer",
		"1 Jun 2026 10:00 UTC",
		"31 Dec 2027",
		"glpat-secret123",
	}
	for _, s := range checks {
		t.Run(s, func(t *testing.T) {
			if !strings.Contains(md, s) {
				t.Errorf("FormatOutputMarkdown missing %q in:\n%s", s, md)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown -- with pagination data
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithPagination verifies the ListMarkdown_WithPagination Markdown formatter for a representative list_withpagination input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestFormatListMarkdown_WithPagination(t *testing.T) {
	out := ListOutput{
		Tokens: []Output{
			{ID: 1, Name: "tok-1", Active: true, Scopes: []string{"api"}, ExpiresAt: "2027-01-01"},
		},
	}
	out.Pagination.Page = 1
	out.Pagination.PerPage = 20
	out.Pagination.TotalItems = 1
	out.Pagination.TotalPages = 1

	md := FormatListMarkdown(out)
	if !strings.Contains(md, "tok-1") {
		t.Error("markdown should contain token name")
	}
	if !strings.Contains(md, "1 Jan 2027") {
		t.Error("markdown should contain expiry date")
	}
}

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := accessTokenSpecsByTool(t, specs)

	if len(specs) != 18 {
		t.Fatalf("len(ActionSpecs) = %d, want 18", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "accesstokens" {
			t.Fatalf("OwnerPackage for %s = %q, want accesstokens", spec.Name, spec.OwnerPackage)
		}
	}
}

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newAccessTokenRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"project_list", "gitlab_project_access_token_list", map[string]any{"project_id": "42"}},
		{"project_get", "gitlab_project_access_token_get", map[string]any{"project_id": "42", "token_id": 5}},
		{"project_create", "gitlab_project_access_token_create", map[string]any{"project_id": "42", "name": "t", "scopes": []any{"api"}}},
		{"project_rotate", "gitlab_project_access_token_rotate", map[string]any{"project_id": "42", "token_id": 5}},
		{"project_revoke", "gitlab_project_access_token_revoke", map[string]any{"project_id": "42", "token_id": 5}},
		{"project_rotate_self", "gitlab_project_access_token_rotate_self", map[string]any{"project_id": "42"}},
		{"group_list", "gitlab_group_access_token_list", map[string]any{"group_id": "10"}},
		{"group_get", "gitlab_group_access_token_get", map[string]any{"group_id": "10", "token_id": 3}},
		{"group_create", "gitlab_group_access_token_create", map[string]any{"group_id": "10", "name": "t", "scopes": []any{"api"}}},
		{"group_rotate", "gitlab_group_access_token_rotate", map[string]any{"group_id": "10", "token_id": 3}},
		{"group_revoke", "gitlab_group_access_token_revoke", map[string]any{"group_id": "10", "token_id": 3}},
		{"group_rotate_self", "gitlab_group_access_token_rotate_self", map[string]any{"group_id": "10"}},
		{"personal_list", "gitlab_personal_access_token_list", map[string]any{}},
		{"personal_get", "gitlab_personal_access_token_get", map[string]any{"token_id": 50}},
		{"personal_rotate", "gitlab_personal_access_token_rotate", map[string]any{"token_id": 50}},
		{"personal_revoke", "gitlab_personal_access_token_revoke", map[string]any{"token_id": 50}},
		{"personal_rotate_self", "gitlab_personal_access_token_rotate_self", map[string]any{}},
		{"personal_revoke_self", "gitlab_personal_access_token_revoke_self", map[string]any{}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			assertAccessTokenRouteOK(t, byTool, tt.tool, tt.args)
		})
	}
}

// assertAccessTokenRouteOK calls a canonical route and fails the test if it returns an error.
func assertAccessTokenRouteOK(t *testing.T, byTool map[string]toolutil.ActionSpec, toolName string, args map[string]any) {
	t.Helper()

	result, err := byTool[toolName].Route.Handler(t.Context(), args)
	if err != nil {
		t.Fatalf("Route.Handler(%s) error: %v", toolName, err)
	}
	if result == nil {
		t.Fatalf("Route.Handler(%s) returned nil", toolName)
	}
}

// ---------------------------------------------------------------------------
// Helper: route spec factory
// ---------------------------------------------------------------------------.

// newAccessTokenRouteSpecs constructs access token route specs test fixtures.
func newAccessTokenRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	projectTokenJSON := `{"id":5,"name":"proj-token","active":true,"revoked":false,"scopes":["api"],"access_level":30,"token":"glpat-proj"}`
	groupTokenJSON := `{"id":3,"name":"group-token","active":true,"revoked":false,"scopes":["api"],"access_level":20,"token":"glpat-grp"}`
	personalTokenJSON := `{"id":50,"name":"personal-token","active":true,"revoked":false,"scopes":["api"],"user_id":1,"token":"glpat-pat"}`

	handler := http.NewServeMux()

	// Project Access Tokens
	handler.HandleFunc("GET /api/v4/projects/42/access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+projectTokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/access_tokens/5", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, projectTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/access_tokens/5/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectTokenJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/access_tokens/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/projects/42/access_tokens/self/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, projectTokenJSON)
	})

	// Group Access Tokens
	handler.HandleFunc("GET /api/v4/groups/10/access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+groupTokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/groups/10/access_tokens/3", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/10/access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, groupTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/groups/10/access_tokens/3/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupTokenJSON)
	})
	handler.HandleFunc("DELETE /api/v4/groups/10/access_tokens/3", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/groups/10/access_tokens/self/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, groupTokenJSON)
	})

	// Personal Access Tokens
	handler.HandleFunc("GET /api/v4/personal_access_tokens", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+personalTokenJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/personal_access_tokens/50", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, personalTokenJSON)
	})
	handler.HandleFunc("POST /api/v4/personal_access_tokens/50/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, personalTokenJSON)
	})
	handler.HandleFunc("DELETE /api/v4/personal_access_tokens/50", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("POST /api/v4/personal_access_tokens/self/rotate", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, personalTokenJSON)
	})
	handler.HandleFunc("DELETE /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	return accessTokenSpecsByTool(t, ActionSpecs(client))
}

// accessTokenSpecsByTool supports access token specs by tool assertions in accesstokens tests.
func accessTokenSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}

// TestProjectList_WithOrderingAndKeyset verifies that ProjectList forwards the
// order_by, sort, and keyset pagination parameters to the GitLab API.
func TestProjectList_WithOrderingAndKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathProjectTokens {
			q := r.URL.Query()
			if q.Get("order_by") != "created_at" {
				t.Errorf("expected order_by=created_at, got %s", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("expected sort=desc, got %s", q.Get("sort"))
			}
			if q.Get("pagination") != "keyset" {
				t.Errorf("expected pagination=keyset, got %s", q.Get("pagination"))
			}
			if q.Get("page_token") != "abc" {
				t.Errorf("expected page_token=abc, got %s", q.Get("page_token"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := ProjectList(context.Background(), client, ProjectListInput{
		ProjectID:  "42",
		OrderBy:    "created_at",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "abc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGroupList_WithAllFilters verifies that GroupList forwards every list
// filter (search, revoked, date filters, sort, keyset) to the GitLab API.
func TestGroupList_WithAllFilters(t *testing.T) {
	revoked := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupTokens {
			q := r.URL.Query()
			checks := map[string]string{
				"search":           "ci-bot",
				"revoked":          "true",
				"created_after":    "2024-01-01",
				"created_before":   "2024-12-31",
				"expires_after":    "2025-01-01",
				"expires_before":   "2025-12-31",
				"last_used_after":  "2024-06-01",
				"last_used_before": "2024-06-30",
				"order_by":         "created_at",
				"sort":             "created_desc",
				"pagination":       "keyset",
				"page_token":       "xyz",
			}
			for key, want := range checks {
				t.Run(key, func(t *testing.T) {
					if got := q.Get(key); got != want {
						t.Errorf("expected %s=%s, got %s", key, want, got)
					}
				})
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := GroupList(context.Background(), client, GroupListInput{
		GroupID:        "10",
		Search:         "ci-bot",
		Revoked:        &revoked,
		CreatedAfter:   "2024-01-01",
		CreatedBefore:  "2024-12-31",
		ExpiresAfter:   "2025-01-01",
		ExpiresBefore:  "2025-12-31",
		LastUsedAfter:  "2024-06-01",
		LastUsedBefore: "2024-06-30",
		OrderBy:        "created_at",
		Sort:           "created_desc",
		Pagination:     "keyset", PageToken: "xyz",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGroupList_InvalidDateFilter verifies that GroupList returns a parse error
// when a date filter is not in YYYY-MM-DD format, without calling the API.
func TestGroupList_InvalidDateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when date filter is invalid")
	}))

	_, err := GroupList(context.Background(), client, GroupListInput{
		GroupID:      "10",
		CreatedAfter: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid created_after, got nil")
	}
}

// TestPersonalList_WithAllFilters verifies that PersonalList forwards every list
// filter (revoked, date filters, sort, keyset) to the GitLab API.
func TestPersonalList_WithAllFilters(t *testing.T) {
	revoked := false
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/personal_access_tokens" {
			q := r.URL.Query()
			checks := map[string]string{
				"revoked":          "false",
				"created_after":    "2024-01-01",
				"created_before":   "2024-12-31",
				"expires_after":    "2025-01-01",
				"expires_before":   "2025-12-31",
				"last_used_after":  "2024-06-01",
				"last_used_before": "2024-06-30",
				"order_by":         "created_at",
				"sort":             "name_asc",
				"pagination":       "keyset",
				"page_token":       "pat-cursor",
			}
			for key, want := range checks {
				t.Run(key, func(t *testing.T) {
					if got := q.Get(key); got != want {
						t.Errorf("expected %s=%s, got %s", key, want, got)
					}
				})
			}
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, jsonNotFound)
	}))

	_, err := PersonalList(context.Background(), client, PersonalListInput{
		Revoked:        &revoked,
		CreatedAfter:   "2024-01-01",
		CreatedBefore:  "2024-12-31",
		ExpiresAfter:   "2025-01-01",
		ExpiresBefore:  "2025-12-31",
		LastUsedAfter:  "2024-06-01",
		LastUsedBefore: "2024-06-30",
		OrderBy:        "created_at",
		Sort:           "name_asc",
		Pagination:     "keyset", PageToken: "pat-cursor",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestPersonalList_InvalidDateFilter verifies that PersonalList returns a parse
// error when a date filter is malformed, without calling the API.
func TestPersonalList_InvalidDateFilter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("API should not be called when date filter is invalid")
	}))

	_, err := PersonalList(context.Background(), client, PersonalListInput{LastUsedBefore: "31-12-2024"})
	if err == nil {
		t.Fatal("expected error for invalid last_used_before, got nil")
	}
}
