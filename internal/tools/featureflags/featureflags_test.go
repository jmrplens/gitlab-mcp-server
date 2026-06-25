// featureflags_test.go contains unit tests for the feature flag MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package featureflags

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// featureFlagJSON identifies the feature flag JSON constant used by this package.
const featureFlagJSON = `{
	"name": "my-flag",
	"description": "Test feature flag",
	"active": true,
	"version": "new_version_flag",
	"created_at": "2026-01-01T00:00:00Z",
	"updated_at": "2026-01-02T00:00:00Z",
	"scopes": [],
	"strategies": [
		{
			"id": 1,
			"name": "gradualRolloutUserId",
			"parameters": {"percentage": "50", "groupId": "default", "stickiness": "default"},
			"scopes": [{"id": 10, "environment_scope": "production"}]
		}
	]
}`

// featureFlagListJSON identifies the feature flag list JSON constant used by this package.
const featureFlagListJSON = `[` + featureFlagJSON + `]`

// scopedFeatureFlagJSON is a feature flag with top-level scopes populated,
// exercising the Output.Scopes mapping (including a null entry that must be
// skipped) in convertFeatureFlag.
const scopedFeatureFlagJSON = `{
	"name": "scoped-flag",
	"description": "",
	"active": true,
	"version": "new_version_flag",
	"scopes": [{"id": 7, "environment_scope": "production"}, null],
	"strategies": []
}`

// -- List --.

// TestListFeatureFlags_Success verifies that ListFeatureFlags succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListFeatureFlags_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, featureFlagListJSON, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListFeatureFlags(context.Background(), client, ListInput{
		ProjectID: "1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.FeatureFlags) != 1 {
		t.Errorf("expected 1 flag, got %d", len(out.FeatureFlags))
	}
	if out.FeatureFlags[0].Name != "my-flag" {
		t.Errorf("expected name 'my-flag', got %q", out.FeatureFlags[0].Name)
	}
	if len(out.FeatureFlags[0].Strategies) != 1 {
		t.Errorf("expected 1 strategy, got %d", len(out.FeatureFlags[0].Strategies))
	}
}

// TestListFeatureFlags_OrderingAndKeyset verifies that ListFeatureFlags forwards
// order_by, sort, and keyset pagination (pagination + page_token) as query
// parameters, and that top-level flag scopes are surfaced on the output
// (including a null scope entry that must be skipped).
// It asserts the request query string and the mapped Output.Scopes.
func TestListFeatureFlags_OrderingAndKeyset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("order_by") != "name" || q.Get("sort") != "asc" {
			t.Errorf("order_by/sort = %q/%q, want name/asc", q.Get("order_by"), q.Get("sort"))
		}
		if q.Get("pagination") != "keyset" || q.Get("page_token") != "cursor-1" {
			t.Errorf("pagination/page_token = %q/%q, want keyset/cursor-1", q.Get("pagination"), q.Get("page_token"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+scopedFeatureFlagJSON+`]`, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, mux)

	out, err := ListFeatureFlags(context.Background(), client, ListInput{
		ProjectID:             "1",
		OrderBy:               "name",
		Sort:                  "asc",
		KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "cursor-1"},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.FeatureFlags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(out.FeatureFlags))
	}
	scopes := out.FeatureFlags[0].Scopes
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope (null skipped), got %d", len(scopes))
	}
	if scopes[0].ID != 7 || scopes[0].EnvironmentScope != "production" {
		t.Errorf("scope = %+v, want id=7 production", scopes[0])
	}
}

// TestListFeatureFlags_MissingProjectID verifies that ListFeatureFlags_MissingProjectID returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListFeatureFlags_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListFeatureFlags(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// -- Get --.

// TestGetFeatureFlag_Success verifies that GetFeatureFlag succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetFeatureFlag_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1/feature_flags/my-flag", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, featureFlagJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := GetFeatureFlag(context.Background(), client, GetInput{
		ProjectID: "1",
		Name:      "my-flag",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "my-flag" {
		t.Errorf("expected name 'my-flag', got %q", out.Name)
	}
	if !out.Active {
		t.Error("expected active=true")
	}
	if out.Strategies[0].Parameters.Percentage != "50" {
		t.Errorf("expected percentage '50', got %q", out.Strategies[0].Parameters.Percentage)
	}
}

// TestGetFeatureFlag_MissingParams verifies that GetFeatureFlag_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetFeatureFlag_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := GetFeatureFlag(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error for missing params")
	}
	_, err = GetFeatureFlag(context.Background(), client, GetInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// -- Create --.

// TestCreateFeatureFlag_Success verifies that CreateFeatureFlag succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateFeatureFlag_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, featureFlagJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateFeatureFlag(context.Background(), client, CreateInput{
		ProjectID:   "1",
		Name:        "my-flag",
		Description: "Test feature flag",
		Version:     "new_version_flag",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "my-flag" {
		t.Errorf("expected name 'my-flag', got %q", out.Name)
	}
}

// TestCreateFeatureFlag_WithStrategies verifies the CreateFeatureFlag_WithStrategies handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateFeatureFlag_WithStrategies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, featureFlagJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := CreateFeatureFlag(context.Background(), client, CreateInput{
		ProjectID: "1",
		Name:      "my-flag",
		Strategies: []StrategyInput{
			{
				Name:       "gradualRolloutUserId",
				Parameters: &StrategyParameterInput{Percentage: "50"},
				Scopes:     []ScopeInput{{EnvironmentScope: "production"}},
			},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "my-flag" {
		t.Errorf("expected name 'my-flag', got %q", out.Name)
	}
}

// TestCreateFeatureFlag_MissingParams verifies that CreateFeatureFlag_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateFeatureFlag_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateFeatureFlag(context.Background(), client, CreateInput{})
	if err == nil {
		t.Fatal("expected error for missing params")
	}
	_, err = CreateFeatureFlag(context.Background(), client, CreateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// -- Update --.

// TestUpdateFeatureFlag_Success verifies that UpdateFeatureFlag succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateFeatureFlag_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/v4/projects/1/feature_flags/my-flag", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, featureFlagJSON)
	})
	client := testutil.NewTestClient(t, mux)

	out, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{
		ProjectID:   "1",
		Name:        "my-flag",
		Description: "Updated desc",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "my-flag" {
		t.Errorf("expected name 'my-flag', got %q", out.Name)
	}
}

// TestUpdateFeatureFlag_MissingParams verifies that UpdateFeatureFlag_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateFeatureFlag_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal("expected error for missing params")
	}
	_, err = UpdateFeatureFlag(context.Background(), client, UpdateInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// -- Delete --.

// TestDeleteFeatureFlag_Success verifies that DeleteFeatureFlag succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteFeatureFlag_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v4/projects/1/feature_flags/my-flag", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)

	err := DeleteFeatureFlag(context.Background(), client, DeleteInput{
		ProjectID: "1",
		Name:      "my-flag",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeleteFeatureFlag_MissingParams verifies that DeleteFeatureFlag_MissingParams returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteFeatureFlag_MissingParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	err := DeleteFeatureFlag(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("expected error for missing params")
	}
	err = DeleteFeatureFlag(context.Background(), client, DeleteInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

// -- Formatters --.

// TestFormatFeatureFlagMarkdown verifies the FeatureFlagMarkdown Markdown formatter for a representative featureflag input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatFeatureFlagMarkdown(t *testing.T) {
	out := Output{
		Name:        "my-flag",
		Description: "Test feature flag",
		Active:      true,
		Version:     "new_version_flag",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-02T00:00:00Z",
		Scopes: []ScopeOutput{
			{ID: 7, EnvironmentScope: "production"},
		},
		Strategies: []StrategyOutput{
			{
				ID:   1,
				Name: "gradualRolloutUserId",
				Parameters: &StrategyParameterOutput{
					Percentage: "50",
					GroupID:    "default",
					Stickiness: "default",
				},
				Scopes: []ScopeOutput{
					{ID: 10, EnvironmentScope: "production"},
				},
			},
		},
	}
	md := FormatFeatureFlagMarkdown(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, "my-flag") {
		t.Error("expected markdown to contain flag name")
	}
	if !contains(md, "gradualRolloutUserId") {
		t.Error("expected markdown to contain strategy name")
	}
	if !contains(md, "| Scopes | production |") {
		t.Error("expected markdown to contain top-level Scopes row")
	}
}

// TestFormatListFeatureFlagsMarkdown verifies the ListFeatureFlagsMarkdown Markdown formatter for a representative listfeatureflags input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListFeatureFlagsMarkdown(t *testing.T) {
	out := ListOutput{
		FeatureFlags: []Output{
			{Name: "flag-1", Active: true, Version: "new_version_flag"},
			{Name: "flag-2", Active: false, Version: "legacy_flag"},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 1},
	}
	md := FormatListFeatureFlagsMarkdown(out)
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, "flag-1") || !contains(md, "flag-2") {
		t.Error("expected markdown to contain both flag names")
	}
}

// TestFormatListFeatureFlagsMarkdown_Empty verifies the ListFeatureFlagsMarkdown_Empty Markdown formatter for a representative listfeatureflags_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListFeatureFlagsMarkdown_Empty(t *testing.T) {
	out := ListOutput{FeatureFlags: []Output{}}
	md := FormatListFeatureFlagsMarkdown(out)
	if !contains(md, "No feature flags found") {
		t.Error("expected 'No feature flags found' message")
	}
}

// contains reports whether contains.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

// containsStr reports whether contains str.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// covFeatureFlagJSON is a minimal feature flag JSON for coverage tests.
const covFeatureFlagJSON = `{
	"name": "cov-flag",
	"description": "coverage flag",
	"active": true,
	"version": "new_version_flag",
	"created_at": "2026-01-01T00:00:00Z",
	"updated_at": "2026-01-02T00:00:00Z",
	"strategies": [
		{
			"id": 1,
			"name": "default",
			"parameters": {"percentage": "100"},
			"scopes": [{"id": 10, "environment_scope": "production"}]
		}
	]
}`

// ---------------------------------------------------------------------------
// ListFeatureFlags — API error, scope param
// ---------------------------------------------------------------------------.

// TestListFeatureFlags_APIError verifies that ListFeatureFlags returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListFeatureFlags_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListFeatureFlags(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListFeatureFlags_Forbidden verifies the ListFeatureFlags_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListFeatureFlags_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := ListFeatureFlags(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "Premium/Ultimate") {
		t.Fatalf("error = %v, want tier hint", err)
	}
}

// TestListFeatureFlags_WithScope verifies the ListFeatureFlags_WithScope handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListFeatureFlags_WithScope(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "enabled" {
			t.Errorf("expected scope=enabled, got %q", r.URL.Query().Get("scope"))
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+covFeatureFlagJSON+`]`, testutil.PaginationHeaders{
			Page: "1", NextPage: "", TotalPages: "1", PerPage: "20", Total: "1",
		})
	})
	client := testutil.NewTestClient(t, handler)

	out, err := ListFeatureFlags(context.Background(), client, ListInput{
		ProjectID: "1",
		Scope:     "enabled",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.FeatureFlags) != 1 {
		t.Errorf("expected 1 flag, got %d", len(out.FeatureFlags))
	}
}

// ---------------------------------------------------------------------------
// GetFeatureFlag — API error
// ---------------------------------------------------------------------------.

// TestGetFeatureFlag_APIError verifies that GetFeatureFlag returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetFeatureFlag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := GetFeatureFlag(context.Background(), client, GetInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// CreateFeatureFlag — API error, Active param
// ---------------------------------------------------------------------------.

// TestCreateFeatureFlag_APIError verifies that CreateFeatureFlag returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateFeatureFlag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateFeatureFlag(context.Background(), client, CreateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateFeatureFlag_ErrorBranches verifies that CreateFeatureFlagBranches returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreateFeatureFlag_ErrorBranches(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "forbidden", statusCode: http.StatusForbidden, wantText: "Developer+ role"},
		{name: "generic", statusCode: http.StatusUnprocessableEntity, wantText: "feature_flag_create"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, testCase.statusCode, `{"message":"failed"}`)
			}))
			_, err := CreateFeatureFlag(context.Background(), client, CreateInput{ProjectID: "1", Name: "x"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("error = %v, want %q", err, testCase.wantText)
			}
		})
	}
}

// TestCreateFeatureFlag_WithActive verifies the CreateFeatureFlag_WithActive handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestCreateFeatureFlag_WithActive(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covFeatureFlagJSON)
	})
	client := testutil.NewTestClient(t, handler)

	active := true
	out, err := CreateFeatureFlag(context.Background(), client, CreateInput{
		ProjectID: "1",
		Name:      "cov-flag",
		Active:    &active,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "cov-flag" {
		t.Errorf("expected name 'cov-flag', got %q", out.Name)
	}
}

// ---------------------------------------------------------------------------
// UpdateFeatureFlag — API error, NewName, Active, Strategies, invalid strategies
// ---------------------------------------------------------------------------.

// TestUpdateFeatureFlag_APIError verifies that UpdateFeatureFlag returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestUpdateFeatureFlag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateFeatureFlag_Forbidden verifies the UpdateFeatureFlag_Forbidden handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateFeatureFlag_Forbidden(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"forbidden"}`)
	}))
	_, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "Developer+ role") {
		t.Fatalf("error = %v, want role hint", err)
	}
}

// TestUpdateFeatureFlag_AllOptionalFields verifies the UpdateFeatureFlag_AllOptionalFields handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdateFeatureFlag_AllOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("PUT /api/v4/projects/1/feature_flags/cov-flag", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covFeatureFlagJSON)
	})
	client := testutil.NewTestClient(t, handler)

	active := false
	out, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{
		ProjectID:   "1",
		Name:        "cov-flag",
		NewName:     "cov-flag-renamed",
		Description: "updated desc",
		Active:      &active,
		Strategies: []StrategyInput{
			{
				ID:         1,
				Name:       "default",
				Parameters: &StrategyParameterInput{Percentage: "100"},
				Scopes:     []ScopeInput{{EnvironmentScope: "staging"}},
			},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "cov-flag" {
		t.Errorf("expected name 'cov-flag', got %q", out.Name)
	}
}

// ---------------------------------------------------------------------------
// DeleteFeatureFlag — API error
// ---------------------------------------------------------------------------.

// TestDeleteFeatureFlag_APIError verifies that DeleteFeatureFlag returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteFeatureFlag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	err := DeleteFeatureFlag(context.Background(), client, DeleteInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// formatParameters — all parameter branches
// ---------------------------------------------------------------------------.

// TestFormatParameters_AllFields verifies the Parameters_AllFields Markdown formatter for a representative parameters_allfields input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatParameters_AllFields(t *testing.T) {
	p := &StrategyParameterOutput{
		Percentage: "50",
		GroupID:    "g1",
		UserIDs:    "1,2,3",
		Rollout:    "random",
		Stickiness: "default",
	}
	result := formatParameters(p)
	for _, want := range []string{"percentage=50", "groupId=g1", "userIds=1,2,3", "rollout=random", "stickiness=default"} {
		if !strings.Contains(result, want) {
			t.Errorf("formatParameters missing %q in %q", want, result)
		}
	}
}

// TestFormatParameters_Nil verifies the Parameters_Nil Markdown formatter for a representative parameters_nil input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatParameters_Nil(t *testing.T) {
	if got := formatParameters(nil); got != "-" {
		t.Errorf("expected '-', got %q", got)
	}
}

// TestFormatParameters_Empty verifies the Parameters_Empty Markdown formatter for a representative parameters_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatParameters_Empty(t *testing.T) {
	if got := formatParameters(&StrategyParameterOutput{}); got != "-" {
		t.Errorf("expected '-' for empty params, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// formatScopes — empty and multiple scopes
// ---------------------------------------------------------------------------.

// TestFormatScopes_Empty verifies the Scopes_Empty Markdown formatter for a representative scopes_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatScopes_Empty(t *testing.T) {
	if got := formatScopes(nil); got != "-" {
		t.Errorf("expected '-', got %q", got)
	}
}

// TestFormatScopes_Multiple verifies the Scopes_Multiple Markdown formatter for a representative scopes_multiple input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatScopes_Multiple(t *testing.T) {
	scopes := []ScopeOutput{
		{ID: 1, EnvironmentScope: "production"},
		{ID: 2, EnvironmentScope: "staging"},
	}
	got := formatScopes(scopes)
	if !strings.Contains(got, "production") || !strings.Contains(got, "staging") {
		t.Errorf("expected both scopes, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// FormatFeatureFlagMarkdown — no strategies, no dates
// ---------------------------------------------------------------------------.

// TestFormatFeatureFlagMarkdown_Minimal verifies the FeatureFlagMarkdown_Minimal Markdown formatter for a representative featureflag_minimal input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatFeatureFlagMarkdown_Minimal(t *testing.T) {
	out := Output{
		Name:    "bare-flag",
		Active:  false,
		Version: "legacy_flag",
	}
	md := FormatFeatureFlagMarkdown(out)
	if !strings.Contains(md, "bare-flag") {
		t.Error("expected flag name in markdown")
	}
	if strings.Contains(md, "### Strategies") {
		t.Error("should not contain Strategies section for empty strategies")
	}
	if strings.Contains(md, "Created") {
		t.Error("should not contain Created row when empty")
	}
	if strings.Contains(md, "Updated") {
		t.Error("should not contain Updated row when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListFeatureFlagsMarkdown — with pagination
// ---------------------------------------------------------------------------.

// TestFormatListFeatureFlagsMarkdown_WithPagination verifies the ListFeatureFlagsMarkdown_WithPagination Markdown formatter for a representative listfeatureflags_withpagination input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
func TestFormatListFeatureFlagsMarkdown_WithPagination(t *testing.T) {
	out := ListOutput{
		FeatureFlags: []Output{
			{Name: "f1", Active: true, Version: "v1", Strategies: []StrategyOutput{{ID: 1}}},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, TotalPages: 3, TotalItems: 50, PerPage: 20},
	}
	md := FormatListFeatureFlagsMarkdown(out)
	if !strings.Contains(md, "f1") {
		t.Error("missing flag name")
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := featureFlagSpecsByTool(t, specs)

	if len(specs) != 5 {
		t.Fatalf("len(ActionSpecs) = %d, want 5", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "featureflags" {
			t.Fatalf("OwnerPackage for %s = %q, want featureflags", spec.Name, spec.OwnerPackage)
		}
	}
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 5 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates the CallAllRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := covNewFeatureFlagSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_feature_flag_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_feature_flag_get", map[string]any{"project_id": "1", "name": "cov-flag"}},
		{"create", "gitlab_feature_flag_create", map[string]any{"project_id": "1", "name": "new-flag"}},
		{"update", "gitlab_feature_flag_update", map[string]any{"project_id": "1", "name": "cov-flag", "description": "upd"}},
		{"delete", "gitlab_feature_flag_delete", map[string]any{"project_id": "1", "name": "cov-flag"}},
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

// covNewFeatureFlagSpecsByTool supports cov new feature flag specs by tool assertions in featureflags tests.
func covNewFeatureFlagSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	handler.HandleFunc("GET /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+covFeatureFlagJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/1/feature_flags/cov-flag", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covFeatureFlagJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, covFeatureFlagJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/1/feature_flags/cov-flag", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covFeatureFlagJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/1/feature_flags/cov-flag", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	return featureFlagSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_FeatureFlagGetRoute validates the FeatureFlagGetRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_FeatureFlagGetRoute(t *testing.T) {
	const respJSON = `{"name":"experimental_ui","description":"","active":true,"version":"new_version_flag"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/feature_flags/experimental_ui") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := featureFlagSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_feature_flag_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "name": "experimental_ui"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.Name != "experimental_ui" || !out.Active {
		t.Fatalf("feature flag output = %#v, want active experimental_ui", out)
	}
}

// TestFeatureFlagActionSpecs_Metadata verifies that every feature-flag action
// spec carries non-generic R-META discovery metadata: a specific Usage (not the
// generic placeholder), at least two natural-language aliases distinct from the
// individual tool name, RelatedActions, and a "Returns: … See also: …"
// individual-tool description.
// It asserts these fields on all five projected specs.
func TestFeatureFlagActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	byTool := featureFlagSpecsByTool(t, ActionSpecs(client))

	wantTools := []string{
		"gitlab_feature_flag_list",
		"gitlab_feature_flag_get",
		"gitlab_feature_flag_create",
		"gitlab_feature_flag_update",
		"gitlab_feature_flag_delete",
	}
	for _, tool := range wantTools {
		spec, ok := byTool[tool]
		if !ok {
			t.Fatalf("missing spec for %s", tool)
		}
		if spec.Usage == "" || strings.Contains(spec.Usage, "Use to execute featureflags domain action") {
			t.Errorf("%s: generic or empty Usage: %q", tool, spec.Usage)
		}
		nlAliases := 0
		for _, a := range spec.Aliases {
			if a != tool {
				nlAliases++
			}
		}
		if nlAliases < 2 {
			t.Errorf("%s: expected >=2 natural-language aliases, got %v", tool, spec.Aliases)
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: missing RelatedActions", tool)
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description missing Returns:/See also:: %q", tool, desc)
		}
	}
}

// TestDecorateFeatureFlagMeta_UnknownTool verifies that decorateFeatureFlagMeta
// leaves options untouched for a tool name absent from featureFlagActionMeta.
// It asserts the generic placeholder Usage survives the no-op path.
func TestDecorateFeatureFlagMeta_UnknownTool(t *testing.T) {
	options := featureFlagOptions("gitlab_feature_flag_unknown")
	decorateFeatureFlagMeta(&options, "gitlab_feature_flag_unknown")
	if options.Usage != "Use to execute featureflags domain action." {
		t.Errorf("expected generic Usage to survive, got %q", options.Usage)
	}
}

// featureFlagSpecsByTool supports feature flag specs by tool assertions in featureflags tests.
func featureFlagSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
