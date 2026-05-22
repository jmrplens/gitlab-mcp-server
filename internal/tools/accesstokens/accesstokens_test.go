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

// TestProjectList_Success verifies ProjectList when success.
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

// TestProjectList_WithState verifies ProjectList when with state.
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

// TestProjectList_MissingProjectID verifies ProjectList when missing project ID.
func TestProjectList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	_, err := ProjectList(context.Background(), client, ProjectListInput{})
	if err == nil || !strings.Contains(err.Error(), errProjectIDRequired) {
		t.Fatalf(fmtExpProjectIDErr, err)
	}
}

// TestProjectGet_Success verifies ProjectGet when success.
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

// TestProjectGet_MissingInputs verifies ProjectGet when missing inputs.
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

// TestProjectCreate_Success verifies ProjectCreate when success.
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

// TestProjectCreate_Validation covers ProjectCreate with table-driven subtests for validation.
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

// TestProjectRotate_Success verifies ProjectRotate when success.
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

// TestProjectRevoke_Success verifies ProjectRevoke when success.
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

// TestProjectRevoke_Validation verifies ProjectRevoke when validation.
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

// TestGroupList_Success verifies GroupList when success.
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

// TestGroupList_MissingGroupID verifies GroupList when missing group ID.
func TestGroupList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	_, err := GroupList(context.Background(), client, GroupListInput{})
	if err == nil || !strings.Contains(err.Error(), errGroupIDRequired) {
		t.Fatalf(fmtExpGroupIDErr, err)
	}
}

// TestGroupGet_Success verifies GroupGet when success.
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

// TestGroupCreate_Success verifies GroupCreate when success.
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

// TestGroupRotate_Success verifies GroupRotate when success.
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

// TestGroupRevoke_Success verifies GroupRevoke when success.
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

// TestPersonalList_Success verifies PersonalList when success.
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

// TestPersonalList_WithFilters verifies PersonalList when with filters.
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

// TestPersonalGet_SelfSuccess verifies PersonalGet when self success.
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

// TestPersonalGet_ByIDSuccess verifies PersonalGet when by ID success.
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

// TestPersonalRotate_Success verifies PersonalRotate when success.
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

// TestPersonalRotate_Validation verifies PersonalRotate when validation.
func TestPersonalRotate_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Helper() }))
	_, err := PersonalRotate(context.Background(), client, PersonalRotateInput{})
	if err == nil || !strings.Contains(err.Error(), errTokenIDRequired) {
		t.Fatalf(fmtExpTokenIDErr, err)
	}
}

// TestPersonalRevoke_Success verifies PersonalRevoke when success.
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

// TestPersonalRevoke_Validation verifies PersonalRevoke when validation.
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

// TestAccessLevelName covers AccessLevelName with table-driven subtests.
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

// TestFormatOutputMarkdown verifies FormatOutputMarkdown.
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

// TestFormatOutputMarkdown_AccessLevel verifies FormatOutputMarkdown when access level.
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

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No access tokens found") {
		t.Error("empty list should show no tokens message")
	}
}

// TestFormatListMarkdown_WithTokens verifies FormatListMarkdown when with tokens.
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

// TestProjectRotateSelf_Success verifies ProjectRotateSelf when success.
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

// TestProjectRotateSelf_MissingProjectID verifies ProjectRotateSelf when missing project ID.
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

// TestGroupRotateSelf_Success verifies GroupRotateSelf when success.
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

// TestGroupRotateSelf_MissingGroupID verifies GroupRotateSelf when missing group ID.
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

// TestPersonalRotateSelf_Success verifies PersonalRotateSelf when success.
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

// TestPersonalRotateSelf_APIError verifies PersonalRotateSelf when API error.
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

// TestPersonalRevokeSelf_Success verifies PersonalRevokeSelf when success.
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

// TestPersonalRevokeSelf_APIError verifies PersonalRevokeSelf when API error.
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

// TestCancelled_Context covers Cancelled with table-driven subtests for context.
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

// TestProjectList_APIError verifies ProjectList when API error.
func TestProjectList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectList(context.Background(), client, ProjectListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectGet_APIError verifies ProjectGet when API error.
func TestProjectGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectGet(context.Background(), client, ProjectGetInput{ProjectID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectCreate_APIError verifies ProjectCreate when API error.
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

// TestProjectRotate_APIError verifies ProjectRotate when API error.
func TestProjectRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectRotate(context.Background(), client, ProjectRotateInput{ProjectID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectRevoke_APIError verifies ProjectRevoke when API error.
func TestProjectRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	err := ProjectRevoke(context.Background(), client, ProjectRevokeInput{ProjectID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestProjectRotateSelf_APIError verifies ProjectRotateSelf when API error.
func TestProjectRotateSelf_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupList_APIError verifies GroupList when API error.
func TestGroupList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupList(context.Background(), client, GroupListInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupGet_APIError verifies GroupGet when API error.
func TestGroupGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupGet(context.Background(), client, GroupGetInput{GroupID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupCreate_APIError verifies GroupCreate when API error.
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

// TestGroupRotate_APIError verifies GroupRotate when API error.
func TestGroupRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupRotate(context.Background(), client, GroupRotateInput{GroupID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupRevoke_APIError verifies GroupRevoke when API error.
func TestGroupRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	err := GroupRevoke(context.Background(), client, GroupRevokeInput{GroupID: "1", TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGroupRotateSelf_APIError verifies GroupRotateSelf when API error.
func TestGroupRotateSelf_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{GroupID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalList_APIError verifies PersonalList when API error.
func TestPersonalList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalList(context.Background(), client, PersonalListInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalGet_SelfAPIError verifies PersonalGet when self API error.
func TestPersonalGet_SelfAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 0})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalGet_ByIDAPIError verifies PersonalGet when by idapi error.
func TestPersonalGet_ByIDAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalGet(context.Background(), client, PersonalGetInput{TokenID: 99})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalRotate_APIError verifies PersonalRotate when API error.
func TestPersonalRotate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	_, err := PersonalRotate(context.Background(), client, PersonalRotateInput{TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPersonalRevoke_APIError verifies PersonalRevoke when API error.
func TestPersonalRevoke_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, jsonServerErr)
	}))
	err := PersonalRevoke(context.Background(), client, PersonalRevokeInput{TokenID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAccessTokenInputValidationAPIErrors covers GitLab 400 validation hints for token mutations.
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

// TestGroupGet_MissingInputs verifies GroupGet when missing inputs.
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

// TestGroupCreate_Validation covers GroupCreate with table-driven subtests for validation.
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

// TestGroupRotate_Validation covers GroupRotate with table-driven subtests for validation.
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

// TestGroupRevoke_Validation verifies GroupRevoke when validation.
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

// TestProjectRotate_Validation covers ProjectRotate with table-driven subtests for validation.
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

// TestProjectRotateSelf_BadDate verifies ProjectRotateSelf when bad date.
func TestProjectRotateSelf_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := ProjectRotateSelf(context.Background(), client, ProjectRotateSelfInput{ProjectID: "42", ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// TestGroupRotateSelf_BadDate verifies GroupRotateSelf when bad date.
func TestGroupRotateSelf_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := GroupRotateSelf(context.Background(), client, GroupRotateSelfInput{GroupID: "10", ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// TestPersonalRotate_BadDate verifies PersonalRotate when bad date.
func TestPersonalRotate_BadDate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { /* validation test: handler not called */ }))
	_, err := PersonalRotate(context.Background(), client, PersonalRotateInput{TokenID: 1, ExpiresAt: "bad"})
	if err == nil || !strings.Contains(err.Error(), errInvalidExpiresAt) {
		t.Fatalf(fmtExpInvalidDateErr, err)
	}
}

// TestPersonalRotateSelf_BadDate verifies PersonalRotateSelf when bad date.
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

// TestFromProjectToken_AllDates verifies FromProjectToken when all dates.
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

// TestFromGroupToken_AllDates verifies FromGroupToken when all dates.
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

// TestFromPersonalToken_AllDates verifies FromPersonalToken when all dates.
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

// TestProjectList_WithPagination verifies ProjectList when with pagination.
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

	input := ProjectListInput{ProjectID: "42"}
	input.Page = 2
	input.PerPage = 5
	out, err := ProjectList(context.Background(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("expected TotalPages=2, got %d", out.Pagination.TotalPages)
	}
}

// TestPersonalList_WithUserID verifies PersonalList when with user ID.
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

// TestGroupList_WithPagination verifies GroupList when with pagination.
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

	input := GroupListInput{GroupID: "10"}
	input.Page = 1
	input.PerPage = 10
	out, err := GroupList(context.Background(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Pagination.PerPage != 10 {
		t.Errorf("expected PerPage=10, got %d", out.Pagination.PerPage)
	}
}

// TestGroupList_WithState verifies GroupList when with state.
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

// TestPersonalList_WithPagination verifies PersonalList when with pagination.
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

	input := PersonalListInput{}
	input.Page = 3
	input.PerPage = 5
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

// TestGroupCreate_WithOptionalFields verifies GroupCreate when with optional fields.
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

// TestProjectCreate_WithDescription verifies ProjectCreate when with description.
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

// TestGroupRotate_WithExpiresAt verifies GroupRotate when with expires at.
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

// TestGroupRotateSelf_WithExpiresAt verifies GroupRotateSelf when with expires at.
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

// TestProjectRotateSelf_WithExpiresAt verifies ProjectRotateSelf when with expires at.
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

// TestPersonalRotate_WithExpiresAt verifies PersonalRotate when with expires at.
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

// TestPersonalRotateSelf_WithExpiresAt verifies PersonalRotateSelf when with expires at.
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

// TestFormatOutputMarkdown_AllFields verifies FormatOutputMarkdown when all fields.
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
		if !strings.Contains(md, s) {
			t.Errorf("FormatOutputMarkdown missing %q in:\n%s", s, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown -- with pagination data
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithPagination verifies FormatListMarkdown when with pagination.
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

// TestActionSpecs_Metadata verifies canonical metadata for access token actions.
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

// TestActionSpecs_CallAllRoutes validates access token routes through canonical specs.
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
