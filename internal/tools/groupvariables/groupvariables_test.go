// groupvariables_test.go contains unit tests for the group CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groupvariables

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pathGroupVars = "/api/v4/groups/10/variables"
	pathVar1      = "/api/v4/groups/10/variables/MY_VAR"
	varJSON       = `{"key":"MY_VAR","value":"secret","variable_type":"env_var","protected":true,"masked":false,"hidden":false,"raw":false,"environment_scope":"*","description":"Test var"}`
)

// ---------- List ----------.

// TestList_Success verifies the behavior of list success.
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

// TestList_MissingGroupID verifies the behavior of list missing group i d.
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

// TestGet_Success verifies the behavior of get success.
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

// TestGet_WithEnvironmentScope verifies the behavior of get with environment scope.
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

// TestGet_MissingKey verifies the behavior of get missing key.
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

// TestCreate_Success verifies the behavior of create success.
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

// TestCreate_MissingValue verifies the behavior of create missing value.
func TestCreate_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Key: "MY_VAR"})
	if err == nil {
		t.Fatal("Create() expected error for missing value")
	}
}

// TestCreate_MissingGroupID verifies the behavior of create missing group i d.
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

// TestUpdate_Success verifies the behavior of update success.
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

// TestUpdate_MissingKey verifies the behavior of update missing key.
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

// TestDelete_Success verifies the behavior of delete success.
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

// TestDelete_MissingGroupID verifies the behavior of delete missing group i d.
func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal("Delete() expected error for missing group_id")
	}
}

// TestDelete_MissingKey verifies the behavior of delete missing key.
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

// TestFormatOutputMarkdown verifies the behavior of format output markdown.
func TestFormatOutputMarkdown(t *testing.T) {
	out := Output{Key: "MY_VAR", Value: "secret", VariableType: "env_var", Protected: true, EnvironmentScope: "*"}
	md := FormatOutputMarkdown(out)
	if md == "" {
		t.Fatal("FormatOutputMarkdown returned empty string")
	}
}

// TestFormatOutputMarkdown_Masked verifies the behavior of format output markdown masked.
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

// TestFormatListMarkdown verifies the behavior of format list markdown.
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

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

const errExpectedCtxCancelled = "expected canceled context error, got nil"

// ---------------------------------------------------------------------------
// List — API error, canceled context, pagination parameters, empty result
// ---------------------------------------------------------------------------.

// TestList_APIError verifies the behavior of list a p i error.
func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{GroupID: "10"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestList_CancelledContext verifies the behavior of list cancelled context.
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

// TestList_WithPagination verifies the behavior of list with pagination.
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

// TestList_EmptyResult verifies the behavior of list empty result.
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

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGet_CancelledContext verifies the behavior of get cancelled context.
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

// TestGet_MissingGroupID verifies the behavior of get missing group i d.
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

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
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

// TestCreate_MissingKey verifies the behavior of create missing key.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{GroupID: "10", Value: "V"})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

// TestCreate_AllOptionalFields verifies the behavior of create all optional fields.
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

// TestUpdate_APIError verifies the behavior of update a p i error.
func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdate_CancelledContext verifies the behavior of update cancelled context.
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

// TestUpdate_MissingGroupID verifies the behavior of update missing group i d.
func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal("expected error for missing group_id, got nil")
	}
}

// TestUpdate_AllOptionalFields verifies the behavior of update all optional fields.
func TestUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/10/variables/DB_HOST" && r.Method == http.MethodPut {
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

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "10", Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
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

// TestDelete_WithEnvironmentScope verifies the behavior of delete with environment scope.
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

// TestFormatOutputMarkdown_EmptyKey verifies the behavior of format output markdown empty key.
func TestFormatOutputMarkdown_EmptyKey(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for empty key, got %q", md)
	}
}

// TestFormatOutputMarkdown_FullUnmasked verifies the behavior of format output markdown full unmasked.
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

// TestFormatOutputMarkdown_MaskedValue verifies the behavior of format output markdown masked value.
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

// TestFormatOutputMarkdown_HiddenValue verifies the behavior of format output markdown hidden value.
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

// TestFormatOutputMarkdown_NoDescription verifies the behavior of format output markdown no description.
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

// TestFormatListMarkdown_WithVariables verifies the behavior of format list markdown with variables.
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

// TestFormatListMarkdown_Empty verifies the behavior of cov format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No group CI/CD variables found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| Key |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_EscapesTableCells verifies the behavior of format list markdown escapes table cells.
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

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterMeta — no panic
// ---------------------------------------------------------------------------.

// TestRegisterMeta_NoPanic verifies the behavior of register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 5 tools
// ---------------------------------------------------------------------------.

// requireToolCallSuccess is an internal helper for the groupvariables package.
func requireToolCallSuccess(t *testing.T, session *mcp.ClientSession, ctx context.Context, toolName string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s) error: %v", toolName, err)
	}
	if result.IsError {
		for _, c := range result.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				t.Fatalf("CallTool(%s) returned error: %s", toolName, tc.Text)
			}
		}
		t.Fatalf("CallTool(%s) returned IsError=true", toolName)
	}
}

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newGroupVariablesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_group_variable_list", map[string]any{"group_id": "10"}},
		{"get", "gitlab_group_variable_get", map[string]any{"group_id": "10", "key": "MY_VAR", "environment_scope": ""}},
		{"create", "gitlab_group_variable_create", map[string]any{
			"group_id": "10", "key": "NEW_VAR", "value": "new-val",
			"description": "", "variable_type": "", "protected": false,
			"masked": false, "masked_and_hidden": false, "raw": false,
			"environment_scope": "",
		}},
		{"update", "gitlab_group_variable_update", map[string]any{
			"group_id": "10", "key": "MY_VAR", "value": "new-host",
			"description": "", "variable_type": "", "protected": false,
			"masked": false, "raw": false, "environment_scope": "",
		}},
		{"delete", "gitlab_group_variable_delete", map[string]any{"group_id": "10", "key": "MY_VAR", "environment_scope": ""}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			requireToolCallSuccess(t, session, ctx, tt.tool, tt.args)
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newGroupVariablesMCPSession is an internal helper for the groupvariables package.
func newGroupVariablesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	variableJSON := `{"key":"MY_VAR","value":"secret","variable_type":"env_var","protected":true,"masked":false,"hidden":false,"raw":false,"environment_scope":"*","description":"Test var"}`

	handler := http.NewServeMux()

	// List variables
	handler.HandleFunc("GET /api/v4/groups/10/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+variableJSON+`]`)
	})

	// Get variable
	handler.HandleFunc("GET /api/v4/groups/10/variables/MY_VAR", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, variableJSON)
	})

	// Create variable
	handler.HandleFunc("POST /api/v4/groups/10/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"key":"NEW_VAR","value":"new-val","variable_type":"env_var","protected":false,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"}`)
	})

	// Update variable
	handler.HandleFunc("PUT /api/v4/groups/10/variables/MY_VAR", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"MY_VAR","value":"new-host","variable_type":"env_var","protected":true,"masked":false,"hidden":false,"raw":false,"environment_scope":"*"}`)
	})

	// Delete variable
	handler.HandleFunc("DELETE /api/v4/groups/10/variables/MY_VAR", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
