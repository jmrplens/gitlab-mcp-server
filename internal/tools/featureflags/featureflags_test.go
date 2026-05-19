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

// -- List --.

// TestListFeatureFlags_Success verifies ListFeatureFlags when success.
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

// TestListFeatureFlags_MissingProjectID verifies ListFeatureFlags when missing project ID.
func TestListFeatureFlags_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := ListFeatureFlags(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

// -- Get --.

// TestGetFeatureFlag_Success verifies GetFeatureFlag when success.
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

// TestGetFeatureFlag_MissingParams verifies GetFeatureFlag when missing params.
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

// TestCreateFeatureFlag_Success verifies CreateFeatureFlag when success.
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

// TestCreateFeatureFlag_WithStrategies verifies CreateFeatureFlag when with strategies.
func TestCreateFeatureFlag_WithStrategies(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v4/projects/1/feature_flags", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, featureFlagJSON)
	})
	client := testutil.NewTestClient(t, mux)

	strategies := `[{"name":"gradualRolloutUserId","parameters":{"percentage":"50"},"scopes":[{"environment_scope":"production"}]}]`
	out, err := CreateFeatureFlag(context.Background(), client, CreateInput{
		ProjectID:  "1",
		Name:       "my-flag",
		Strategies: strategies,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "my-flag" {
		t.Errorf("expected name 'my-flag', got %q", out.Name)
	}
}

// TestCreateFeatureFlag_InvalidStrategies verifies CreateFeatureFlag when invalid strategies.
func TestCreateFeatureFlag_InvalidStrategies(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := CreateFeatureFlag(context.Background(), client, CreateInput{
		ProjectID:  "1",
		Name:       "my-flag",
		Strategies: "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid strategies JSON")
	}
}

// TestCreateFeatureFlag_MissingParams verifies CreateFeatureFlag when missing params.
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

// TestUpdateFeatureFlag_Success verifies UpdateFeatureFlag when success.
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

// TestUpdateFeatureFlag_MissingParams verifies UpdateFeatureFlag when missing params.
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

// TestDeleteFeatureFlag_Success verifies DeleteFeatureFlag when success.
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

// TestDeleteFeatureFlag_MissingParams verifies DeleteFeatureFlag when missing params.
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

// TestFormatFeatureFlagMarkdown verifies FormatFeatureFlagMarkdown.
func TestFormatFeatureFlagMarkdown(t *testing.T) {
	out := Output{
		Name:        "my-flag",
		Description: "Test feature flag",
		Active:      true,
		Version:     "new_version_flag",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-02T00:00:00Z",
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
}

// TestFormatListFeatureFlagsMarkdown verifies FormatListFeatureFlagsMarkdown.
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

// TestFormatListFeatureFlagsMarkdown_Empty verifies FormatListFeatureFlagsMarkdown when empty.
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

// TestListFeatureFlags_APIError verifies ListFeatureFlags when API error.
func TestListFeatureFlags_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := ListFeatureFlags(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListFeatureFlags_Forbidden verifies Premium/role hints.
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

// TestListFeatureFlags_WithScope verifies ListFeatureFlags when with scope.
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

// TestGetFeatureFlag_APIError verifies GetFeatureFlag when API error.
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

// TestCreateFeatureFlag_APIError verifies CreateFeatureFlag when API error.
func TestCreateFeatureFlag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := CreateFeatureFlag(context.Background(), client, CreateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreateFeatureFlag_ErrorBranches verifies forbidden and generic error paths.
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

// TestCreateFeatureFlag_WithActive verifies CreateFeatureFlag when with active.
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

// TestUpdateFeatureFlag_APIError verifies UpdateFeatureFlag when API error.
func TestUpdateFeatureFlag_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateFeatureFlag_Forbidden verifies role hints.
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

// TestUpdateFeatureFlag_AllOptionalFields verifies UpdateFeatureFlag when all optional fields.
func TestUpdateFeatureFlag_AllOptionalFields(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("PUT /api/v4/projects/1/feature_flags/cov-flag", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covFeatureFlagJSON)
	})
	client := testutil.NewTestClient(t, handler)

	active := false
	strategies := `[{"id":1,"name":"default","parameters":{"percentage":"100"},"scopes":[{"environment_scope":"staging"}]}]`
	out, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{
		ProjectID:   "1",
		Name:        "cov-flag",
		NewName:     "cov-flag-renamed",
		Description: "updated desc",
		Active:      &active,
		Strategies:  strategies,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Name != "cov-flag" {
		t.Errorf("expected name 'cov-flag', got %q", out.Name)
	}
}

// TestUpdateFeatureFlag_InvalidStrategies verifies UpdateFeatureFlag when invalid strategies.
func TestUpdateFeatureFlag_InvalidStrategies(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	_, err := UpdateFeatureFlag(context.Background(), client, UpdateInput{
		ProjectID:  "1",
		Name:       "x",
		Strategies: "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid strategies JSON")
	}
}

// ---------------------------------------------------------------------------
// DeleteFeatureFlag — API error
// ---------------------------------------------------------------------------.

// TestDeleteFeatureFlag_APIError verifies DeleteFeatureFlag when API error.
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

// TestFormatParameters_AllFields verifies FormatParameters when all fields.
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

// TestFormatParameters_Nil verifies FormatParameters when nil.
func TestFormatParameters_Nil(t *testing.T) {
	if got := formatParameters(nil); got != "-" {
		t.Errorf("expected '-', got %q", got)
	}
}

// TestFormatParameters_Empty verifies FormatParameters when empty.
func TestFormatParameters_Empty(t *testing.T) {
	if got := formatParameters(&StrategyParameterOutput{}); got != "-" {
		t.Errorf("expected '-' for empty params, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// formatScopes — empty and multiple scopes
// ---------------------------------------------------------------------------.

// TestFormatScopes_Empty verifies FormatScopes when empty.
func TestFormatScopes_Empty(t *testing.T) {
	if got := formatScopes(nil); got != "-" {
		t.Errorf("expected '-', got %q", got)
	}
}

// TestFormatScopes_Multiple verifies FormatScopes when multiple.
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

// TestFormatFeatureFlagMarkdown_Minimal verifies FormatFeatureFlagMarkdown when minimal.
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

// TestFormatListFeatureFlagsMarkdown_WithPagination verifies FormatListFeatureFlagsMarkdown when with pagination.
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

// TestActionSpecs_Metadata verifies canonical metadata for feature flag actions.
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

// TestActionSpecs_CallAllRoutes validates feature flag routes across multiple scenarios.
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

// TestActionSpecs_FeatureFlagGetRoute verifies the canonical feature flag get route output.
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

// featureFlagSpecsByTool supports feature flag specs by tool assertions in featureflags tests.
func featureFlagSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
