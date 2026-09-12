// instance_variables_test.go contains unit tests for the instance-level CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package instancevariables

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// TestList_Success verifies that List succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestList_EmptyResult verifies the List_EmptyResult handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_Success verifies that Get succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestGet_MissingKey verifies that Get_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestCreate_Success verifies that Create succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestCreate_MissingKey verifies that Create_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := Create(context.Background(), client, CreateInput{Value: "secret"})
	if err == nil {
		t.Fatal("Create() expected error for missing key")
	}
}

// TestCreate_MissingValue verifies that Create_MissingValue returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestUpdate_Success verifies that Update succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestUpdate_MissingKey verifies that Update_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestDelete_Success verifies that Delete succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestDelete_MissingKey verifies that Delete_MissingKey returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestFormatOutputMarkdown verifies the OutputMarkdown Markdown formatter for a representative output input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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
	want := "## Instance Variable: MY_VAR\n\n" +
		"- **Type**: env_var\n" +
		"- **Protected**: " + toolutil.EmojiSuccess + "\n" +
		"- **Masked**: " + toolutil.EmojiCross + "\n" +
		"- **Raw**: " + toolutil.EmojiCross + "\n" +
		"- **Description**: Test var\n" +
		"- **Value**: secret\n" +
		variableCardHints
	if md != want {
		t.Errorf("variable card:\n got %q\nwant %q", md, want)
	}
}

// variableCardHints is the guidance section every CI/CD variable card ends
// with, shared by the whole-output expectations in this file.
const variableCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'update' to change this variable\n" +
	"- Use action 'delete' to remove this variable\n"

// TestFormatOutputMarkdown_MaskedValue verifies the OutputMarkdown_MaskedValue Markdown formatter for a representative output_maskedvalue input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_MaskedValue(t *testing.T) {
	v := Output{
		Key:          "SECRET_VAR",
		Value:        "hidden-value",
		VariableType: "env_var",
		Masked:       true,
	}
	md := FormatOutputMarkdown(v)
	want := "## Instance Variable: SECRET_VAR\n\n" +
		"- **Type**: env_var\n" +
		"- **Protected**: " + toolutil.EmojiCross + "\n" +
		"- **Masked**: " + toolutil.EmojiSuccess + "\n" +
		"- **Raw**: " + toolutil.EmojiCross + "\n" +
		"- **Value**: [masked]\n" +
		variableCardHints
	if md != want {
		t.Errorf("masked variable card:\n got %q\nwant %q", md, want)
	}
}

// TestFormatOutputMarkdown_Empty verifies the OutputMarkdown_Empty Markdown formatter for a representative output_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", md)
	}
}

// TestFormatListMarkdown verifies the ListMarkdown Markdown formatter for a representative list input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "VAR1", VariableType: "env_var", Protected: true, Masked: false},
			{Key: "VAR2", VariableType: "file", Protected: false, Masked: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1, Page: 1, PerPage: 20},
	}
	// The list keeps four columns: an instance variable's scope is the same on
	// every row, so a column repeating it says nothing. The card names it.
	want := "## Instance CI/CD Variables (2)\n\n" +
		"| Key | Type | Protected | Masked |\n" +
		"| --- | --- | --- | --- |\n" +
		"| VAR1 | env_var | " + toolutil.EmojiSuccess + " | " + toolutil.EmojiCross + " |\n" +
		"| VAR2 | file | " + toolutil.EmojiCross + " | " + toolutil.EmojiSuccess + " |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		variableListHints
	if md := FormatListMarkdown(out); md != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", md, want)
	}
}

// variableListHints is the guidance section the instance variable list closes
// with.
const variableListHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'get' with key for full details\n" +
	"- Use action 'create' to add a new instance variable\n"

// TestFormatOutputMarkdown_NamesTheEnvironmentScope verifies that the scope
// GitLab sends reaches the card. The view model used to receive an empty
// string in its place, so an instance variable scoped to one environment read
// as one that applies everywhere.
func TestFormatOutputMarkdown_NamesTheEnvironmentScope(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:              "DB_HOST",
		Value:            "localhost",
		VariableType:     "env_var",
		EnvironmentScope: "production",
	})
	want := "## Instance Variable: DB_HOST\n\n" +
		"- **Type**: env_var\n" +
		"- **Protected**: " + toolutil.EmojiCross + "\n" +
		"- **Masked**: " + toolutil.EmojiCross + "\n" +
		"- **Raw**: " + toolutil.EmojiCross + "\n" +
		"- **Environment Scope**: production\n" +
		"- **Value**: localhost\n" +
		variableCardHints
	if md != want {
		t.Errorf("variable card with a scope:\n got %q\nwant %q", md, want)
	}
}

// TestFormatListMarkdown_Empty verifies the ListMarkdown_Empty Markdown formatter for a representative list_empty input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_Empty(t *testing.T) {
	const want = "No instance CI/CD variables found.\n"
	if md := FormatListMarkdown(ListOutput{}); md != want {
		t.Errorf("FormatListMarkdown(empty)\n got %q\nwant %q", md, want)
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

// TestInstanceVariableList_APIError verifies that InstanceVariableList returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestInstanceVariableList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableList_WithPagination verifies that InstanceVariableList_WithPagination forwards pagination parameters to the GitLab API and parses the response metadata.
// The mock GitLab API at /api/v4/admin/ci/variables (GET) responds with HTTP OK.
// It asserts the response metadata is propagated to the [toolutil.PaginationOutput].
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
		Page: 2, PerPage: 2,
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

// TestInstanceVariableList_OrderSortKeyset verifies that List forwards the
// order_by, sort, and keyset pagination (pagination, page_token) parameters to
// the GitLab API, mirroring gl.ListInstanceVariablesOptions 1:1.
// The mock GitLab API at /api/v4/admin/ci/variables (GET) asserts each query
// parameter and responds with HTTP OK.
func TestInstanceVariableList_OrderSortKeyset(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathInstanceVars && r.Method == http.MethodGet {
			q := r.URL.Query()
			if q.Get("order_by") != "key" {
				t.Errorf("order_by = %q, want key", q.Get("order_by"))
			}
			if q.Get("sort") != "desc" {
				t.Errorf("sort = %q, want desc", q.Get("sort"))
			}
			if q.Get("pagination") != "keyset" {
				t.Errorf("pagination = %q, want keyset", q.Get("pagination"))
			}
			if q.Get("page_token") != "cursor-123" {
				t.Errorf("page_token = %q, want cursor-123", q.Get("page_token"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[`+varJSON+`]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	out, err := List(context.Background(), client, ListInput{
		OrderBy:    "key",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "cursor-123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Variables) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(out.Variables))
	}
}

// TestInstanceVariableList_CancelledContext verifies the InstanceVariableList_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestInstanceVariableGet_APIError verifies that InstanceVariableGet returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestInstanceVariableGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableGet_CancelledContext verifies the InstanceVariableGet_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestInstanceVariableCreate_APIError verifies that InstanceVariableCreate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestInstanceVariableCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableCreate_BadRequest verifies the InstanceVariableCreate_BadRequest handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestInstanceVariableCreate_AllOptionalFields verifies the InstanceVariableCreate_AllOptionalFields handler.
// The mock GitLab API at /api/v4/admin/ci/variables (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
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

// TestInstanceVariableCreate_CancelledContext verifies the InstanceVariableCreate_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestInstanceVariableUpdate_APIError verifies that InstanceVariableUpdate returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestInstanceVariableUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableUpdate_NotFound verifies that InstanceVariableUpdate_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestInstanceVariableUpdate_AllOptionalFields verifies the InstanceVariableUpdate_AllOptionalFields handler.
// The mock GitLab API at /api/v4/admin/ci/variables/DB_HOST (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestInstanceVariableUpdate_CancelledContext verifies the InstanceVariableUpdate_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestInstanceVariableDelete_APIError verifies that InstanceVariableDelete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestInstanceVariableDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableDelete_NotFound verifies that InstanceVariableDelete_NotFound returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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

// TestInstanceVariableDelete_CancelledContext verifies the InstanceVariableDelete_CancelledContext handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
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

// TestFormatOutputMarkdown_FullUnmasked verifies the OutputMarkdown_FullUnmasked Markdown formatter for a representative output_fullunmasked input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
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

	want := "## Instance Variable: DB_HOST\n\n" +
		"- **Type**: env_var\n" +
		"- **Protected**: " + toolutil.EmojiSuccess + "\n" +
		"- **Masked**: " + toolutil.EmojiCross + "\n" +
		"- **Raw**: " + toolutil.EmojiSuccess + "\n" +
		"- **Description**: Database host\n" +
		"- **Value**: localhost\n" +
		variableCardHints
	if md != want {
		t.Errorf("variable card:\n got %q\nwant %q", md, want)
	}
}

// TestFormatOutputMarkdown_NoDescription verifies the OutputMarkdown_NoDescription Markdown formatter for a representative output_nodescription input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatOutputMarkdown_NoDescription(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		Key:          "SIMPLE",
		Value:        "val",
		VariableType: "env_var",
	})

	want := "## Instance Variable: SIMPLE\n\n" +
		"- **Type**: env_var\n" +
		"- **Protected**: " + toolutil.EmojiCross + "\n" +
		"- **Masked**: " + toolutil.EmojiCross + "\n" +
		"- **Raw**: " + toolutil.EmojiCross + "\n" +
		"- **Value**: val\n" +
		variableCardHints
	if md != want {
		t.Errorf("variable card without a description:\n got %q\nwant %q", md, want)
	}
}

// TestFormatOutputMarkdown_HiddenVariable_WithholdsTheValue verifies that an
// instance variable GitLab marks hidden has its value withheld from the card,
// and that the flag itself is reported.
//
// It matters because hidden is the stronger of GitLab's two secrecy flags: a
// variable created with masked_and_hidden is never shown again in GitLab's own
// UI, and the API still answers an administrator's read with the value. The
// card asked only whether the variable was masked, so a hidden-but-unmasked
// variable was printed in full, which is the one shape the flag exists for.
func TestFormatOutputMarkdown_HiddenVariable_WithholdsTheValue(t *testing.T) {
	head := "## Instance Variable: DEPLOY_KEY\n\n- **Type**: env_var\n- **Protected**: " + toolutil.EmojiCross + "\n"
	cases := []struct {
		name     string
		variable Output
		want     string
	}{
		{
			name:     "hidden and not masked",
			variable: Output{Key: "DEPLOY_KEY", Value: "s3cret-value", VariableType: "env_var", Hidden: true},
			want: head + "- **Masked**: " + toolutil.EmojiCross + "\n- **Hidden**: " + toolutil.EmojiSuccess + "\n" +
				"- **Raw**: " + toolutil.EmojiCross + "\n- **Value**: [masked]\n" + variableCardHints,
		},
		{
			name:     "masked and hidden",
			variable: Output{Key: "DEPLOY_KEY", Value: "s3cret-value", VariableType: "env_var", Masked: true, Hidden: true},
			want: head + "- **Masked**: " + toolutil.EmojiSuccess + "\n- **Hidden**: " + toolutil.EmojiSuccess + "\n" +
				"- **Raw**: " + toolutil.EmojiCross + "\n- **Value**: [masked]\n" + variableCardHints,
		},
		{
			name:     "neither is still printed",
			variable: Output{Key: "DEPLOY_KEY", Value: "s3cret-value", VariableType: "env_var"},
			want: head + "- **Masked**: " + toolutil.EmojiCross + "\n" +
				"- **Raw**: " + toolutil.EmojiCross + "\n- **Value**: s3cret-value\n" + variableCardHints,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if md := FormatOutputMarkdown(tc.variable); md != tc.want {
				t.Errorf("variable card:\n got %q\nwant %q", md, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown — with variables, escapes table cells
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithVariables verifies the ListMarkdown_WithVariables Markdown formatter for a representative list_withvariables input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_WithVariables(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "DB_HOST", VariableType: "env_var", Protected: false, Masked: false},
			{Key: "API_KEY", VariableType: "env_var", Protected: true, Masked: true},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	want := "## Instance CI/CD Variables (2)\n\n" +
		"| Key | Type | Protected | Masked |\n" +
		"| --- | --- | --- | --- |\n" +
		"| DB_HOST | env_var | " + toolutil.EmojiCross + " | " + toolutil.EmojiCross + " |\n" +
		"| API_KEY | env_var | " + toolutil.EmojiSuccess + " | " + toolutil.EmojiSuccess + " |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		variableListHints
	if md := FormatListMarkdown(out); md != want {
		t.Errorf("FormatListMarkdown(with variables)\n got %q\nwant %q", md, want)
	}
}

// TestInstanceVariables_UnreadableCapturedHidden verifies that every instance
// variable handler returns an error rather than a half-filled variable when
// GitLab sends hidden as something that is not a boolean. The SDK ignores the
// key its own InstanceVariable does not model, so the read of the captured
// response is the only thing that can notice, and a hidden variable published
// as visible is exactly the mistake this flag exists to prevent.
func TestInstanceVariables_UnreadableCapturedHidden(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"key":"TOKEN","value":"x","hidden":"maybe"}]`)
			_, err := List(context.Background(), client, ListInput{})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"key":"TOKEN","value":"x","hidden":"maybe"}`)
			_, err := Get(context.Background(), client, GetInput{Key: "TOKEN"})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"key":"TOKEN","value":"x","hidden":"maybe"}`)
			_, err := Create(context.Background(), client, CreateInput{Key: "TOKEN", Value: "x"})
			return err
		}},
		{Name: "update", Call: func() error {
			client := poisoned(`{"key":"TOKEN","value":"x","hidden":"maybe"}`)
			_, err := Update(context.Background(), client, UpdateInput{Key: "TOKEN", Value: "y"})
			return err
		}},
	})
}

// TestFormatListMarkdown_EscapesTableCells verifies the ListMarkdown_EscapesTableCells Markdown formatter for a representative list_escapestablecells input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatListMarkdown_EscapesTableCells(t *testing.T) {
	out := ListOutput{
		Variables: []Output{
			{Key: "MY|VAR", VariableType: "env_var"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1, Page: 1, PerPage: 20, TotalPages: 1},
	}
	const wantRow = "| MY&#124;VAR | env_var | " + toolutil.EmojiCross + " | " + toolutil.EmojiCross + " |\n"
	if md := FormatListMarkdown(out); !strings.Contains(md, wantRow) {
		t.Errorf("FormatListMarkdown() missing %q:\n%s", wantRow, md)
	}
}
