// groupvariables_test.go contains unit tests for the group CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupvariables

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// pathGroupVars identifies the path group vars constant used by this package.
	pathGroupVars = "/api/v4/groups/10/variables"
	// pathVar1 identifies the path var 1 constant used by this package.
	pathVar1 = "/api/v4/groups/10/variables/MY_VAR"
	// varJSON identifies the var JSON constant used by this package.
	varJSON = `{"key":"MY_VAR","value":"secret","variable_type":"env_var","protected":true,"masked":false,"hidden":false,"raw":false,"environment_scope":"*","description":"Test var"}`
)

// ---------- List ----------.

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupVars {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+varJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Variables) != 1 {
		t.Fatalf("len(Variables) = %d, want 1", len(out.Variables))
	}
	if out.Variables[0].Key != "MY_VAR" {
		t.Errorf("Key = %q, want %q", out.Variables[0].Key, "MY_VAR")
	}
	if !out.Variables[0].Protected {
		t.Errorf("Protected = false, want true")
	}
}

// TestList_MissingGroupID verifies List when missing group ID.
func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id")
	}
}

// ---------- Get ----------.

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathVar1 {
			testutil.RespondJSON(w, http.StatusOK, varJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "10", Key: "MY_VAR"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Key != "MY_VAR" {
		t.Errorf("Key = %q, want %q", out.Key, "MY_VAR")
	}
	if out.Description != "Test var" {
		t.Errorf("Description = %q, want %q", out.Description, "Test var")
	}
}

// TestGet_WithEnvironmentScope verifies Get when with environment scope.
func TestGet_WithEnvironmentScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathVar1 {
			q := r.URL.Query()
			if q.Get("filter[environment_scope]") == "" {
				t.Error("expected environment_scope filter parameter")
			}
			testutil.RespondJSON(w, http.StatusOK, varJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: "10", Key: "MY_VAR", EnvironmentScope: "production"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
}

// TestGet_MissingKey verifies Get when missing key.
func TestGet_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: "10"})
	if err == nil {
		t.Fatal("Get() expected error for missing key")
	}
}

// ---------- Create ----------.

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupVars {
			testutil.RespondJSON(w, http.StatusCreated, varJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{GroupID: "10", Key: "MY_VAR", Value: "secret"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Key != "MY_VAR" {
		t.Errorf("Key = %q, want %q", out.Key, "MY_VAR")
	}
}

// TestCreate_MissingValue verifies Create when missing value.
func TestCreate_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Key: "MY_VAR"})
	if err == nil {
		t.Fatal("Create() expected error for missing value")
	}
}

// TestCreate_MissingGroupID verifies Create when missing group ID.
func TestCreate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Key: "MY_VAR", Value: "secret"})
	if err == nil {
		t.Fatal("Create() expected error for missing group_id")
	}
}

// ---------- Update ----------.

// TestUpdate_Success verifies Update when success.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathVar1 {
			testutil.RespondJSON(w, http.StatusOK, varJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(context.Background(), client, UpdateInput{GroupID: "10", Key: "MY_VAR", Value: "new-secret"})
	if err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if out.Key != "MY_VAR" {
		t.Errorf("Key = %q, want %q", out.Key, "MY_VAR")
	}
}

// TestUpdate_MissingKey verifies Update when missing key.
func TestUpdate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10"})
	if err == nil {
		t.Fatal("Update() expected error for missing key")
	}
}

// ---------- Delete ----------.

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathVar1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", Key: "MY_VAR"})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestDelete_MissingGroupID verifies Delete when missing group ID.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id")
	}
}

// TestDelete_MissingKey verifies Delete when missing key.
func TestDelete_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: "10"})
	if err == nil {
		t.Fatal("Delete() expected error for missing key")
	}
}

// ---------- Formatters ----------.

// TestFormatOutputMarkdown verifies FormatOutputMarkdown.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{Key: "MY_VAR", Value: "secret", VariableType: "env_var", Protected: true, EnvironmentScope: "*"}
	md := FormatOutputMarkdown(out)
	if md == "" {
		t.Fatal("FormatOutputMarkdown returned empty string")
	}
}

// TestFormatOutputMarkdown_Masked verifies FormatOutputMarkdown when masked.
func TestFormatOutputMarkdown_Masked(t *testing.T) {
	out := Output{Key: "MY_VAR", Value: "secret", Masked: true, VariableType: "env_var"}
	md := FormatOutputMarkdown(out)
	if md == "" {
		t.Fatal("FormatOutputMarkdown returned empty string")
	}
}

// TestFormatListMarkdown_Empty_NilVariables verifies FormatListMarkdown with nil Variables slice.
func TestFormatListMarkdown_Empty_NilVariables(t *testing.T) {
	out := ListOutput{
		Variables:  nil,
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 0, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if md == "" {
		t.Fatal("FormatListMarkdown returned empty string")
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Variables:  []Output{{Key: "MY_VAR", VariableType: "env_var", Protected: true, EnvironmentScope: "*"}},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 1, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if md == "" {
		t.Fatal("FormatListMarkdown returned empty string")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// errExpectedCtxCancelled identifies the err expected ctx cancelled constant used by this package.
const errExpectedCtxCancelled = "expected canceled context error, got nil"

// ---------------------------------------------------------------------------
// List — API error, canceled context, pagination parameters, empty result
// ---------------------------------------------------------------------------.

// TestList_APIError verifies List when API error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedCtxCancelled)
	}
}

// TestList_WithPagination verifies List when with pagination.
func TestList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/42/variables" && r.Method == http.MethodGet {
			if r.URL.Query().Get("page") != "2" {
				t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"key":"VAR_A","value":"a","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"},
				{"key":"VAR_B","value":"b","variable_type":"file","protected":true,"masked":true,"hidden":false,"raw":true,"environment_scope":"staging"}
			]`, testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID:         "42",
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 2},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Variables) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(out.Variables))
	}
	if out.Variables[0].Key != "VAR_A" {
		t.Errorf("first key = %q, want %q", out.Variables[0].Key, "VAR_A")
	}
	if out.Variables[1].Protected != true {
		t.Error("expected second variable protected=true")
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
}

// TestList_EmptyResult verifies List when empty result.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/variables" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Variables) != 0 {
		t.Fatalf("expected 0 variables, got %d", len(out.Variables))
	}
}

// ---------------------------------------------------------------------------
// Get — API error, canceled context, missing group_id
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_CancelledContext verifies Get when cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedCtxCancelled)
	}
}

// TestGet_MissingGroupID verifies Get when missing group ID.
func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "K"})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Create — API error, canceled context, missing key, all optional fields
// ---------------------------------------------------------------------------.

// TestCreate_APIError verifies Create when API error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_BadRequest verifies invalid key/environment-scope hints.
func TestCreate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"invalid key"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "key must match") {
		t.Fatalf("error = %v, want key hint", err)
	}
}

// TestCreate_CancelledContext verifies Create when cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{GroupID: "10", Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedCtxCancelled)
	}
}

// TestCreate_MissingKey verifies Create when missing key.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Value: "V"})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

// TestCreate_AllOptionalFields verifies Create when all optional fields.
func TestCreate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/variables" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"key":"SECRET_FILE","value":"/tmp/secret","variable_type":"file",
				"protected":true,"masked":true,"hidden":true,"raw":true,
				"environment_scope":"production","description":"Secret file for deploy"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	bTrue := true
	out, err := Create(context.Background(), client, CreateInput{
		GroupID:          "10",
		Key:              "SECRET_FILE",
		Value:            "/tmp/secret",
		Description:      "Secret file for deploy",
		VariableType:     "file",
		Protected:        &bTrue,
		Masked:           &bTrue,
		MaskedAndHidden:  &bTrue,
		Raw:              &bTrue,
		EnvironmentScope: "production",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
	if !out.Protected {
		t.Error("expected protected=true")
	}
	if !out.Hidden {
		t.Error("expected hidden=true")
	}
	if !out.Raw {
		t.Error("expected raw=true")
	}
	if out.EnvironmentScope != "production" {
		t.Errorf("EnvironmentScope = %q, want %q", out.EnvironmentScope, "production")
	}
}

// ---------------------------------------------------------------------------
// Update — API error, canceled context, missing group_id, all optional fields
// ---------------------------------------------------------------------------.

// TestUpdate_APIError verifies Update when API error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_NotFound verifies missing variable hints.
func TestUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "gitlab_group_variable_list") {
		t.Fatalf("error = %v, want list hint", err)
	}
}

// TestUpdate_CancelledContext verifies Update when cancelled context.
func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedCtxCancelled)
	}
}

// TestUpdate_MissingGroupID verifies Update when missing group ID.
func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestUpdate_AllOptionalFields verifies Update when all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/variables/DB_HOST" && r.Method == http.MethodPut {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			filter, hasFilter := body["filter"].(map[string]any)
			if !hasFilter || filter["environment_scope"] != "staging" {
				t.Fatalf("filter.environment_scope = %#v, want staging", body["filter"])
			}
			if _, hasEnvironmentScope := body["environment_scope"]; hasEnvironmentScope {
				t.Fatalf("request body contains environment_scope update field: %#v", body)
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"key":"DB_HOST","value":"db.prod","variable_type":"file",
				"protected":true,"masked":true,"hidden":false,"raw":true,
				"environment_scope":"staging","description":"Updated"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	bTrue := true
	out, err := Update(context.Background(), client, UpdateInput{
		GroupID:          "10",
		Key:              "DB_HOST",
		Value:            "db.prod",
		Description:      "Updated",
		VariableType:     "file",
		Protected:        &bTrue,
		Masked:           &bTrue,
		Raw:              &bTrue,
		EnvironmentScope: "staging",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
	if out.EnvironmentScope != "staging" {
		t.Errorf("EnvironmentScope = %q, want %q", out.EnvironmentScope, "staging")
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, canceled context, with environment_scope
// ---------------------------------------------------------------------------.

// TestDelete_APIError verifies Delete when API error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_NotFound verifies already-deleted variable hints.
func TestDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "may already be deleted") {
		t.Fatalf("error = %v, want deletion hint", err)
	}
}

// TestDelete_CancelledContext verifies Delete when cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedCtxCancelled)
	}
}

// TestDelete_WithEnvironmentScope verifies Delete when with environment scope.
func TestDelete_WithEnvironmentScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/variables/DB_HOST" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID:          "10",
		Key:              "DB_HOST",
		EnvironmentScope: "staging",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — empty key, full unmasked, masked, hidden, no desc
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_EmptyKey verifies FormatOutputMarkdown when empty key.
func TestFormatOutputMarkdown_EmptyKey(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty key, got %q", md)
	}
}

// TestFormatOutputMarkdown_FullUnmasked verifies FormatOutputMarkdown when full unmasked.
func TestFormatOutputMarkdown_FullUnmasked(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "DB_HOST",
		Value:            "localhost",
		VariableType:     "env_var",
		Protected:        true,
		Masked:           false,
		Hidden:           false,
		Raw:              true,
		EnvironmentScope: "production",
		Description:      "Database host",
	})

	for _, want := range []string{
		"## Group Variable: DB_HOST",
		"| Type | env_var |",
		"| Protected | ✅ |",
		"| Masked | ❌ |",
		"| Raw | ✅ |",
		"| Environment Scope | production |",
		"| Description | Database host |",
		"| Value | localhost |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "| Hidden | ✅ |") {
		t.Error("should not contain Hidden line when hidden=false")
	}
}

// TestFormatOutputMarkdown_MaskedValue verifies FormatOutputMarkdown when masked value.
func TestFormatOutputMarkdown_MaskedValue(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "SECRET",
		Value:            "super-secret",
		VariableType:     "env_var",
		Masked:           true,
		EnvironmentScope: "*",
	})

	if !strings.Contains(md, "| Value | [masked] |") {
		t.Errorf("expected masked value placeholder:\n%s", md)
	}
	if strings.Contains(md, "super-secret") {
		t.Error("masked value should not appear in markdown")
	}
}

// TestFormatOutputMarkdown_HiddenValue verifies FormatOutputMarkdown when hidden value.
func TestFormatOutputMarkdown_HiddenValue(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "TOKEN",
		Value:            "",
		VariableType:     "env_var",
		Hidden:           true,
		EnvironmentScope: "*",
	})

	if !strings.Contains(md, "| Hidden | ✅ |") {
		t.Errorf("expected Hidden line:\n%s", md)
	}
	if !strings.Contains(md, "| Value | [masked] |") {
		t.Errorf("hidden variable should show [masked]:\n%s", md)
	}
}

// TestFormatOutputMarkdown_NoDescription verifies FormatOutputMarkdown when no description.
func TestFormatOutputMarkdown_NoDescription(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "SIMPLE",
		Value:            "val",
		VariableType:     "env_var",
		EnvironmentScope: "*",
	})

	if strings.Contains(md, "| Description |") {
		t.Error("should not contain Description when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with variables, empty, escape table cells
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithVariables verifies FormatListMarkdown when with variables.
func TestFormatListMarkdown_WithVariables(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "DB_HOST", VariableType: "env_var", Protected: false, Masked: false, EnvironmentScope: "*"},
			{Key: "API_KEY", VariableType: "env_var", Protected: true, Masked: true, EnvironmentScope: "production"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Group CI/CD Variables (2)",
		"| Key |",
		"| --- |",
		"| DB_HOST |",
		"| API_KEY |",
		"env_var",
		"production",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No group CI/CD variables found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| Key |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_EscapesTableCells verifies FormatListMarkdown when escapes table cells.
func TestFormatListMarkdown_EscapesTableCells(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "MY|VAR", VariableType: "env_var", EnvironmentScope: "scope|test"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if strings.Contains(md, "| MY|VAR |") {
		t.Errorf("pipe in key should be escaped:\n%s", md)
	}
}
