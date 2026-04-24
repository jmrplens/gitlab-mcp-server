// instancevariables_test.go contains unit tests for the instance-level CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.

package instancevariables

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
	pathInstanceVars = "/api/v4/admin/ci/variables"
	pathVar1         = "/api/v4/admin/ci/variables/MY_VAR"
	varJSON          = `{"key":"MY_VAR","value":"secret","variable_type":"env_var","protected":true,"masked":false,"raw":false,"description":"Test var"}`
)

// ---------- List ----------.

// TestList_Success verifies the behavior of list success.
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

// TestList_EmptyResult verifies the behavior of list empty result.
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

// TestGet_Success verifies the behavior of get success.
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

// TestGet_MissingKey verifies the behavior of get missing key.
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

// TestCreate_Success verifies the behavior of create success.
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

// TestCreate_MissingKey verifies the behavior of create missing key.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Value: "secret"})
	if err == nil {
		t.Fatal("Create() expected error for missing key")
	}
}

// TestCreate_MissingValue verifies the behavior of create missing value.
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

// TestUpdate_Success verifies the behavior of update success.
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

// TestUpdate_MissingKey verifies the behavior of update missing key.
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

// TestDelete_Success verifies the behavior of delete success.
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

// TestDelete_MissingKey verifies the behavior of delete missing key.
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

// TestFormatOutputMarkdown verifies the behavior of format output markdown.
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

// TestFormatOutputMarkdown_MaskedValue verifies the behavior of format output markdown masked value.
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

// TestFormatOutputMarkdown_Empty verifies the behavior of format output markdown empty.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", md)
	}
}

// TestFormatListMarkdown verifies the behavior of format list markdown.
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

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No instance CI/CD variables found") {
		t.Errorf("FormatListMarkdown(empty) = %q, want no-results message", md)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpCancelledCtx = "expected error for canceled context"

const errExpectedAPI = "expected API error, got nil"

const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error, with pagination parameters, canceled context
// ---------------------------------------------------------------------------.

// TestInstanceVariableList_APIError verifies the behavior of instance variable list a p i error.
func TestInstanceVariableList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableList_WithPagination verifies the behavior of instance variable list with pagination.
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

// TestInstanceVariableList_CancelledContext verifies the behavior of instance variable list cancelled context.
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

// TestInstanceVariableGet_APIError verifies the behavior of instance variable get a p i error.
func TestInstanceVariableGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableGet_CancelledContext verifies the behavior of instance variable get cancelled context.
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

// TestInstanceVariableCreate_APIError verifies the behavior of instance variable create a p i error.
func TestInstanceVariableCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableCreate_AllOptionalFields verifies the behavior of instance variable create all optional fields.
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

// TestInstanceVariableCreate_CancelledContext verifies the behavior of instance variable create cancelled context.
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

// TestInstanceVariableUpdate_APIError verifies the behavior of instance variable update a p i error.
func TestInstanceVariableUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableUpdate_AllOptionalFields verifies the behavior of instance variable update all optional fields.
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

// TestInstanceVariableUpdate_CancelledContext verifies the behavior of instance variable update cancelled context.
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

// TestInstanceVariableDelete_APIError verifies the behavior of instance variable delete a p i error.
func TestInstanceVariableDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableDelete_CancelledContext verifies the behavior of instance variable delete cancelled context.
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

// TestFormatOutputMarkdown_FullUnmasked verifies the behavior of format output markdown full unmasked.
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

// TestFormatOutputMarkdown_NoDescription verifies the behavior of format output markdown no description.
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

// TestFormatListMarkdown_WithVariables verifies the behavior of format list markdown with variables.
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

// TestFormatListMarkdown_EscapesTableCells verifies the behavior of format list markdown escapes table cells.
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

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newInstanceVariablesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_instance_variable_list", map[string]any{}},
		{"get", "gitlab_instance_variable_get", map[string]any{"key": "MY_VAR"}},
		{"create", "gitlab_instance_variable_create", map[string]any{
			"key": "NEW_VAR", "value": "new-val",
			"description": "", "variable_type": "",
			"protected": false, "masked": false, "raw": false,
		}},
		{"update", "gitlab_instance_variable_update", map[string]any{
			"key": "MY_VAR", "value": "updated-val",
			"description": "", "variable_type": "",
			"protected": false, "masked": false, "raw": false,
		}},
		{"delete", "gitlab_instance_variable_delete", map[string]any{"key": "MY_VAR"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: MCP session factory
// ---------------------------------------------------------------------------.

// newInstanceVariablesMCPSession is an internal helper for the instancevariables package.
func newInstanceVariablesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	variableJSON := `{"key":"MY_VAR","value":"secret","variable_type":"env_var","protected":true,"masked":false,"raw":false,"description":"Test var"}`

	handler := http.NewServeMux()

	// List instance variables
	handler.HandleFunc("GET /api/v4/admin/ci/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+variableJSON+`]`)
	})

	// Get instance variable
	handler.HandleFunc("GET /api/v4/admin/ci/variables/MY_VAR", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, variableJSON)
	})

	// Create instance variable
	handler.HandleFunc("POST /api/v4/admin/ci/variables", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{"key":"NEW_VAR","value":"new-val","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""}`)
	})

	// Update instance variable
	handler.HandleFunc("PUT /api/v4/admin/ci/variables/MY_VAR", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"key":"MY_VAR","value":"updated-val","variable_type":"env_var","protected":false,"masked":false,"raw":false,"description":""}`)
	})

	// Delete instance variable
	handler.HandleFunc("DELETE /api/v4/admin/ci/variables/MY_VAR", func(w http.ResponseWriter, _ *http.Request) {
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
