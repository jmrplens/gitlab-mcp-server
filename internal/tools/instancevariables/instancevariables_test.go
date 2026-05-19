// instancevariables_test.go contains unit tests for the instance-level CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package instancevariables

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// pathInstanceVars identifies the path instance vars constant used by this package.
	pathInstanceVars = "/api/v4/admin/ci/variables"
	// pathVar1 identifies the path var 1 constant used by this package.
	pathVar1 = "/api/v4/admin/ci/variables/MY_VAR"
	// varJSON identifies the var JSON constant used by this package.
	varJSON = `{"key":"MY_VAR","value":"secret","variable_type":"env_var","protected":true,"masked":false,"raw":false,"description":"Test var"}`
)

// ---------- List ----------.

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathInstanceVars {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+varJSON+`]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
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

// TestList_EmptyResult verifies List when empty result.
func TestList_EmptyResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathInstanceVars {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Variables) != 0 {
		t.Errorf("len(Variables) = %d, want 0", len(out.Variables))
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

	out, err := Get(context.Background(), client, GetInput{Key: "MY_VAR"})
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

// TestGet_MissingKey verifies Get when missing key.
func TestGet_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() expected error for missing key")
	}
}

// ---------- Create ----------.

// TestCreate_Success verifies Create when success.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathInstanceVars {
			testutil.RespondJSON(w, http.StatusCreated, varJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{Key: "MY_VAR", Value: "secret"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.Key != "MY_VAR" {
		t.Errorf("Key = %q, want %q", out.Key, "MY_VAR")
	}
}

// TestCreate_MissingKey verifies Create when missing key.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Value: "secret"})
	if err == nil {
		t.Fatal("Create() expected error for missing key")
	}
}

// TestCreate_MissingValue verifies Create when missing value.
func TestCreate_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal("Create() expected error for missing value")
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

	out, err := Update(context.Background(), client, UpdateInput{Key: "MY_VAR", Value: "secret"})
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

	_, err := Update(context.Background(), client, UpdateInput{})
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

	err := Delete(context.Background(), client, DeleteInput{Key: "MY_VAR"})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestDelete_MissingKey verifies Delete when missing key.
func TestDelete_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("Delete() expected error for missing key")
	}
}

// ---------- Formatters ----------.

// TestFormatOutputMarkdown verifies FormatOutputMarkdown.
func TestFormatOutputMarkdown(t *testing.T) {
	v := Output{
		Key:          "MY_VAR",
		Value:        "secret",
		VariableType: "env_var",
		Protected:    true,
		Masked:       false,
		Raw:          false,
		Description:  "Test var",
	}
	md := FormatOutputMarkdown(v)
	if !strings.Contains(md, "MY_VAR") {
		t.Error("expected variable key in output")
	}
	if !strings.Contains(md, "secret") {
		t.Error("expected value in output when not masked")
	}
	if !strings.Contains(md, "true") {
		t.Error("expected Protected=true in output")
	}
}

// TestFormatOutputMarkdown_MaskedValue verifies FormatOutputMarkdown when masked value.
func TestFormatOutputMarkdown_MaskedValue(t *testing.T) {
	v := Output{
		Key:          "SECRET_VAR",
		Value:        "hidden-value",
		VariableType: "env_var",
		Masked:       true,
	}
	md := FormatOutputMarkdown(v)
	if strings.Contains(md, "hidden-value") {
		t.Error("masked value should not appear in output")
	}
	if !strings.Contains(md, "[masked]") {
		t.Error("expected [masked] placeholder in output")
	}
}

// TestFormatOutputMarkdown_Empty verifies FormatOutputMarkdown when empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", md)
	}
}

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "VAR1", VariableType: "env_var", Protected: true, Masked: false},
			{Key: "VAR2", VariableType: "file", Protected: false, Masked: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1, Page: 1, PerPage: 20},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "VAR1") {
		t.Error("expected VAR1 in list output")
	}
	if !strings.Contains(md, "VAR2") {
		t.Error("expected VAR2 in list output")
	}
	if !strings.Contains(md, "Instance CI/CD Variables (2)") {
		t.Error("expected header with count")
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No instance CI/CD variables found") {
		t.Errorf("FormatListMarkdown(empty) = %q, want no-results message", md)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error, with pagination parameters, canceled context
// ---------------------------------------------------------------------------.

// TestInstanceVariableList_APIError verifies InstanceVariableList when API error.
func TestInstanceVariableList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableList_WithPagination verifies InstanceVariableList when with pagination.
func TestInstanceVariableList_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/admin/ci/variables" && r.Method == http.MethodGet {
			if r.URL.Query().Get("page") != "2" {
				t.Errorf("expected page=2, got %s", r.URL.Query().Get("page"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"key":"VAR_A","value":"a","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""},
				{"key":"VAR_B","value":"b","variable_type":"file","protected":true,"masked":true,"raw":true,"description":"Secret"}
			]`, testutil.PaginationHeaders{Page: "2", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "3", PrevPage: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	out, err := List(context.Background(), client, ListInput{
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

// TestInstanceVariableList_CancelledContext verifies InstanceVariableList when cancelled context.
func TestInstanceVariableList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, canceled context
// ---------------------------------------------------------------------------.

// TestInstanceVariableGet_APIError verifies InstanceVariableGet when API error.
func TestInstanceVariableGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableGet_CancelledContext verifies InstanceVariableGet when cancelled context.
func TestInstanceVariableGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Create — API error, all optional fields, canceled context
// ---------------------------------------------------------------------------.

// TestInstanceVariableCreate_APIError verifies InstanceVariableCreate when API error.
func TestInstanceVariableCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableCreate_BadRequest verifies invalid key hints.
func TestInstanceVariableCreate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"invalid key"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "key must match") {
		t.Fatalf("error = %v, want key hint", err)
	}
}

// TestInstanceVariableCreate_AllOptionalFields verifies InstanceVariableCreate when all optional fields.
func TestInstanceVariableCreate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/admin/ci/variables" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"key":"SECRET_FILE","value":"/tmp/secret","variable_type":"file",
				"protected":true,"masked":true,"raw":true,"description":"Secret file for deploy"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	bTrue := true
	out, err := Create(context.Background(), client, CreateInput{
		Key:          "SECRET_FILE",
		Value:        "/tmp/secret",
		Description:  "Secret file for deploy",
		VariableType: "file",
		Protected:    &bTrue,
		Masked:       &bTrue,
		Raw:          &bTrue,
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
	if !out.Raw {
		t.Error("expected raw=true")
	}
	if out.Description != "Secret file for deploy" {
		t.Errorf("Description = %q, want %q", out.Description, "Secret file for deploy")
	}
}

// TestInstanceVariableCreate_CancelledContext verifies InstanceVariableCreate when cancelled context.
func TestInstanceVariableCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Update — API error, all optional fields, canceled context
// ---------------------------------------------------------------------------.

// TestInstanceVariableUpdate_APIError verifies InstanceVariableUpdate when API error.
func TestInstanceVariableUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableUpdate_NotFound verifies missing variable hints.
func TestInstanceVariableUpdate_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "gitlab_instance_variable_list") {
		t.Fatalf("error = %v, want list hint", err)
	}
}

// TestInstanceVariableUpdate_AllOptionalFields verifies InstanceVariableUpdate when all optional fields.
func TestInstanceVariableUpdate_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/admin/ci/variables/DB_HOST" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{
				"key":"DB_HOST","value":"db.prod","variable_type":"file",
				"protected":true,"masked":true,"raw":true,"description":"Updated"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":msgNotFound}`)
	}))

	bTrue := true
	out, err := Update(context.Background(), client, UpdateInput{
		Key:          "DB_HOST",
		Value:        "db.prod",
		Description:  "Updated",
		VariableType: "file",
		Protected:    &bTrue,
		Masked:       &bTrue,
		Raw:          &bTrue,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.VariableType != "file" {
		t.Errorf("VariableType = %q, want %q", out.VariableType, "file")
	}
	if out.Description != "Updated" {
		t.Errorf("Description = %q, want %q", out.Description, "Updated")
	}
}

// TestInstanceVariableUpdate_CancelledContext verifies InstanceVariableUpdate when cancelled context.
func TestInstanceVariableUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// Delete — API error, canceled context
// ---------------------------------------------------------------------------.

// TestInstanceVariableDelete_APIError verifies InstanceVariableDelete when API error.
func TestInstanceVariableDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableDelete_NotFound verifies already-deleted hints.
func TestInstanceVariableDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "may already be deleted") {
		t.Fatalf("error = %v, want deletion hint", err)
	}
}

// TestInstanceVariableDelete_CancelledContext verifies InstanceVariableDelete when cancelled context.
func TestInstanceVariableDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown — full unmasked, no description
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_FullUnmasked verifies FormatOutputMarkdown when full unmasked.
func TestFormatOutputMarkdown_FullUnmasked(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:          "DB_HOST",
		Value:        "localhost",
		VariableType: "env_var",
		Protected:    true,
		Masked:       false,
		Raw:          true,
		Description:  "Database host",
	})

	for _, want := range []string{
		"## Instance Variable: DB_HOST",
		"**Type**: env_var",
		"**Protected**: true",
		"**Masked**: false",
		"**Raw**: true",
		"**Description**: Database host",
		"**Value**: localhost",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatOutputMarkdown_NoDescription verifies FormatOutputMarkdown when no description.
func TestFormatOutputMarkdown_NoDescription(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:          "SIMPLE",
		Value:        "val",
		VariableType: "env_var",
	})

	if strings.Contains(md, "**Description**") {
		t.Error("should not contain Description when empty")
	}
	if !strings.Contains(md, "**Value**: val") {
		t.Errorf("expected value in output:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with variables, escapes table cells
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithVariables verifies FormatListMarkdown when with variables.
func TestFormatListMarkdown_WithVariables(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "DB_HOST", VariableType: "env_var", Protected: false, Masked: false},
			{Key: "API_KEY", VariableType: "env_var", Protected: true, Masked: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Instance CI/CD Variables (2)",
		"| Key |",
		"| --- |",
		"| DB_HOST |",
		"| API_KEY |",
		"env_var",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_EscapesTableCells verifies FormatListMarkdown when escapes table cells.
func TestFormatListMarkdown_EscapesTableCells(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "MY|VAR", VariableType: "env_var"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)
	if strings.Contains(md, "| MY|VAR |") {
		t.Errorf("pipe in key should be escaped:\n%s", md)
	}
}
