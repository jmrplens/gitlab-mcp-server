// instance_variables_test.go contains unit tests for the instance-level CI/CD variable MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package instancevariables

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
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
	// bodyForbidden and bodyNotFound are the refusals the mocks answer with.
	// They are real JSON documents: the bodies here used to carry an unquoted
	// identifier (`{"message":msgServerError}`), which decodes to nothing, so
	// ExtractGitLabMessage found no message and any assertion about what
	// GitLab said would have passed while saying nothing.
	bodyForbidden = `{"message":"403 Forbidden"}`
	bodyNotFound  = `{"message":"404 Variable Not Found"}`
)

// decodeVariableRequest reads the JSON object a handler sent GitLab, so a test
// can assert what the create and update handlers put on the wire rather than
// what the mock was told to answer. An empty body decodes to an empty object:
// an options struct with every field nil is exactly the "names nothing" case
// the omission tests are about, and a read of it must not be reported as a
// malformed request.
//
// It reports with t.Errorf and answers deterministically because it runs on the
// httptest server's goroutine, where FailNow would abort the wrong goroutine
// and leave the client waiting on a response nobody writes. The bool says
// whether the caller may carry on.
func decodeVariableRequest(t *testing.T, w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		t.Errorf("decode request body: %v", err)
		http.Error(w, "decode request body", http.StatusInternalServerError)
		return nil, false
	}
	return body, true
}

// wantRequestField reports when the body a handler sent GitLab does not carry
// key with the expected value, which is what a guard dropping a field the
// caller filled looks like from the far side of the wire.
func wantRequestField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()
	got, ok := body[key]
	if !ok {
		t.Errorf("request body has no %q, want %#v", key, want)
		return
	}
	if got != want {
		t.Errorf("request body %q = %#v, want %#v", key, got, want)
	}
}

// wantRequestFieldAbsent reports when the body carries a key the caller never
// filled. GitLab applies what an update names and leaves the rest alone, so a
// key sent with its zero value overwrites a setting nobody asked to change.
func wantRequestFieldAbsent(t *testing.T, body map[string]any, key string) {
	t.Helper()
	if got, ok := body[key]; ok {
		t.Errorf("request body carries %q = %#v, want it absent", key, got)
	}
}

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

// TestList_EmptyResult verifies that a GET answering with an empty array
// yields an empty variable slice rather than a nil one the caller has to
// guard.
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

// TestGet_MissingKey verifies that Get refuses a call naming no key, and
// refuses it here rather than letting GitLab answer.
//
// The mock forbids every request, so the error cannot have come from a
// response: with an ordinary mock a 404 would satisfy the same assertion and
// say nothing about which layer declined.
func TestGet_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() expected error for missing key")
	}
	if err.Error() != "key is required" {
		t.Errorf("error = %v, want it to name the missing field", err)
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

// TestCreate_MissingKey verifies that Create refuses a call naming no key
// before it reaches GitLab, and says which field is missing.
//
// The forbidding mock is what makes the second half of that claim: the two
// required-field guards are checked in order, so a test that accepted any
// error could not tell the key guard from the value guard either.
func TestCreate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Create(context.Background(), client, CreateInput{Value: "secret"})
	if err == nil {
		t.Fatal("Create() expected error for missing key")
	}
	if err.Error() != "key is required" {
		t.Errorf("error = %v, want it to name the missing field", err)
	}
}

// TestCreate_MissingValue verifies that Create refuses a call naming a key and
// no value before it reaches GitLab, naming value rather than key.
//
// A variable with no value is the one shape GitLab's own endpoint would take
// happily, storing the empty string, so this guard is ours and the test has to
// prove it fired here.
func TestCreate_MissingValue(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Create(context.Background(), client, CreateInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal("Create() expected error for missing value")
	}
	if err.Error() != "value is required" {
		t.Errorf("error = %v, want it to name the missing field", err)
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

// TestUpdate_MissingKey verifies that Update refuses a call naming no key
// before it reaches GitLab, and says which field is missing.
//
// The key is the path segment rather than a body field, so without the guard
// the request would go to the collection itself; the forbidding mock is what
// proves none was sent.
func TestUpdate_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Update(context.Background(), client, UpdateInput{})
	if err == nil {
		t.Fatal("Update() expected error for missing key")
	}
	if err.Error() != "key is required" {
		t.Errorf("error = %v, want it to name the missing field", err)
	}
}

// ---------- Delete ----------.

// TestDelete_Success verifies that Delete reports no error when GitLab answers
// its DELETE with 204. The handler returns only an error, so that is the whole
// of what there is to assert.
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

// TestDelete_MissingKey verifies that Delete refuses a call naming no key
// before it reaches GitLab, and says which field is missing.
//
// This is the guard worth forbidding a request over: the key is the path
// segment, so a delete that let an empty one through would address the
// collection rather than a variable.
func TestDelete_MissingKey(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := Delete(context.Background(), client, DeleteInput{})
	if err == nil {
		t.Fatal("Delete() expected error for missing key")
	}
	if err.Error() != "key is required" {
		t.Errorf("error = %v, want it to name the missing field", err)
	}
}

// ---------- Formatters ----------.

// TestFormatOutputMarkdown renders a representative variable and compares the
// whole card, headings, flag emoji and hints included. The formatter reads a
// value and contacts no API.
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

// TestFormatOutputMarkdown_MaskedValue verifies that a masked variable has its
// value withheld from the card and the flag reported, comparing the whole
// rendering rather than looking for the placeholder.
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

// TestFormatOutputMarkdown_Empty verifies that a zero variable renders nothing
// at all, so a nil result is never dressed up as a card describing a variable
// that does not exist.
func TestFormatOutputMarkdown_Empty(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("FormatOutputMarkdown(empty) = %q, want empty", md)
	}
}

// TestFormatListMarkdown compares the whole table a two-variable page renders,
// its columns, its rows and its pagination line, against the expected text.
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

// TestFormatListMarkdown_Empty verifies that a page with no variables renders
// the empty-state sentence alone, with no table header and no hints.
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

// TestInstanceVariableList_APIError verifies that List reports an error when
// GitLab refuses its GET. The hint that refusal carries is asserted by the
// refusal table further down.
func TestInstanceVariableList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, bodyForbidden)
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
		testutil.RespondJSON(w, http.StatusNotFound, bodyNotFound)
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

// TestInstanceVariableList_CancelledContext verifies that a canceled context
// aborts List before it builds a request, so nothing reaches GitLab.
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

// TestInstanceVariableGet_APIError verifies that Get reports an error when
// GitLab answers its GET 403, which is the status the handler carries no hint
// for, so what is asserted is the refusal itself.
func TestInstanceVariableGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, bodyForbidden)
	}))
	_, err := Get(context.Background(), client, GetInput{Key: "MY_VAR"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableGet_CancelledContext verifies that a canceled context
// aborts Get before it builds a request, so nothing reaches GitLab.
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

// TestInstanceVariableCreate_APIError verifies that Create reports an error
// when GitLab refuses its POST. The hint that refusal carries is asserted by
// the refusal table further down, which reads it per handler.
func TestInstanceVariableCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, bodyForbidden)
	}))
	_, err := Create(context.Background(), client, CreateInput{Key: "K", Value: "V"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableCreate_BadRequest verifies that a POST GitLab answers
// 400 carries the key-syntax hint rather than the admin-privilege one, which
// is the other branch of the same handler.
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

// TestInstanceVariableCreate_AllOptionalFields_ReachTheRequestBody verifies that
// every optional field a caller filled is in the body Create sends GitLab.
//
// Asserting only the response proves nothing about the request: the mock writes
// the canned JSON whatever arrives, so any of the five guards that copy an
// optional field onto the options struct could be inverted and drop the field
// on the floor with every assertion still passing. A create that silently
// discards variable_type stores an env_var where the caller asked for a file,
// and one that discards masked writes the secret into every job log.
func TestInstanceVariableCreate_AllOptionalFields_ReachTheRequestBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/admin/ci/variables" && r.Method == http.MethodPost {
			body, ok := decodeVariableRequest(t, w, r)
			if !ok {
				return
			}
			wantRequestField(t, body, "key", "SECRET_FILE")
			wantRequestField(t, body, "value", "/tmp/secret")
			wantRequestField(t, body, "description", "Secret file for deploy")
			wantRequestField(t, body, "variable_type", "file")
			wantRequestField(t, body, "protected", true)
			wantRequestField(t, body, "masked", true)
			wantRequestField(t, body, "raw", true)
			testutil.RespondJSON(w, http.StatusCreated, `{
				"key":"SECRET_FILE","value":"/tmp/secret","variable_type":"file",
				"protected":true,"masked":true,"raw":true,"description":"Secret file for deploy"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, bodyNotFound)
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

// TestInstanceVariableCreate_UnsetOptionalFields_AreAbsentFromTheRequestBody
// verifies that a create naming only key and value sends GitLab only those two.
//
// This is the other half of the same guards, and the half that decides what
// GitLab stores. The SDK's options carry each field as a pointer, and
// `omitempty` drops only a nil one, so a pointer to the empty string still
// reaches the wire: a guard inverted here would send `"variable_type": ""` and
// `"description": ""` for fields the caller never named, in place of leaving
// the keys out. The defaults the caller is relying on are GitLab's, and GitLab
// applies them to a key the body does not carry.
func TestInstanceVariableCreate_UnsetOptionalFields_AreAbsentFromTheRequestBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/admin/ci/variables" && r.Method == http.MethodPost {
			body, ok := decodeVariableRequest(t, w, r)
			if !ok {
				return
			}
			wantRequestField(t, body, "key", "PLAIN")
			wantRequestField(t, body, "value", "plain-value")
			wantRequestFieldAbsent(t, body, "description")
			wantRequestFieldAbsent(t, body, "variable_type")
			wantRequestFieldAbsent(t, body, "protected")
			wantRequestFieldAbsent(t, body, "masked")
			wantRequestFieldAbsent(t, body, "raw")
			testutil.RespondJSON(w, http.StatusCreated, `{
				"key":"PLAIN","value":"plain-value","variable_type":"env_var",
				"protected":false,"masked":false,"raw":false,"description":""
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, bodyNotFound)
	}))

	out, err := Create(context.Background(), client, CreateInput{Key: "PLAIN", Value: "plain-value"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != "PLAIN" {
		t.Errorf("Key = %q, want %q", out.Key, "PLAIN")
	}
}

// TestInstanceVariableCreate_CancelledContext verifies that a canceled context
// aborts Create before it builds a request, so nothing reaches GitLab.
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

// TestInstanceVariableUpdate_APIError verifies that Update reports an error
// when GitLab refuses its PUT. The hint that refusal carries is asserted by
// the refusal table further down.
func TestInstanceVariableUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, bodyForbidden)
	}))
	_, err := Update(context.Background(), client, UpdateInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableUpdate_NotFound verifies that a PUT GitLab answers 404
// sends the caller to the list action rather than to the admin-privilege
// sentence the 403 branch carries.
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

// TestInstanceVariableUpdate_AllOptionalFields_ReachTheRequestBody verifies that
// every field a caller filled is in the body Update sends GitLab.
//
// GitLab's update endpoint applies what the body names and leaves the rest of
// the variable alone, so a guard that drops a filled field turns an update into
// a no-op that still answers 200 and still renders as a success. The response
// the mock writes cannot show that, since it is the same JSON either way.
func TestInstanceVariableUpdate_AllOptionalFields_ReachTheRequestBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/admin/ci/variables/DB_HOST" && r.Method == http.MethodPut {
			body, ok := decodeVariableRequest(t, w, r)
			if !ok {
				return
			}
			wantRequestField(t, body, "value", "db.prod")
			wantRequestField(t, body, "description", "Updated")
			wantRequestField(t, body, "variable_type", "file")
			wantRequestField(t, body, "protected", true)
			wantRequestField(t, body, "masked", true)
			wantRequestField(t, body, "raw", true)
			testutil.RespondJSON(w, http.StatusOK, `{
				"key":"DB_HOST","value":"db.prod","variable_type":"file",
				"protected":true,"masked":true,"raw":true,"description":"Updated"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, bodyNotFound)
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

// TestInstanceVariableUpdate_UnsetOptionalFields_AreAbsentFromTheRequestBody
// verifies that an update naming only the key sends GitLab an empty body.
//
// This is the costliest of the eleven guards to get wrong. An update applies
// what the body names, so `if input.Value != ""` inverted sends
// `"value": ""` for a caller who named no value, and a caller updating only
// the description would blank the secret the variable holds while GitLab
// answers 200 and the card renders a success. Nothing about the response says
// so, which is why the assertion has to be on the request.
func TestInstanceVariableUpdate_UnsetOptionalFields_AreAbsentFromTheRequestBody(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathVar1 && r.Method == http.MethodPut {
			body, ok := decodeVariableRequest(t, w, r)
			if !ok {
				return
			}
			wantRequestFieldAbsent(t, body, "value")
			wantRequestFieldAbsent(t, body, "description")
			wantRequestFieldAbsent(t, body, "variable_type")
			wantRequestFieldAbsent(t, body, "protected")
			wantRequestFieldAbsent(t, body, "masked")
			wantRequestFieldAbsent(t, body, "raw")
			testutil.RespondJSON(w, http.StatusOK, varJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, bodyNotFound)
	}))

	out, err := Update(context.Background(), client, UpdateInput{Key: "MY_VAR"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != "MY_VAR" {
		t.Errorf("Key = %q, want %q", out.Key, "MY_VAR")
	}
}

// TestInstanceVariableUpdate_CancelledContext verifies that a canceled context
// aborts Update before it builds a request, so nothing reaches GitLab.
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

// TestInstanceVariableDelete_APIError verifies that Delete reports an error
// when GitLab refuses its DELETE. The hint that refusal carries is asserted by
// the refusal table further down.
func TestInstanceVariableDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, bodyForbidden)
	}))
	err := Delete(context.Background(), client, DeleteInput{Key: "K"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestInstanceVariableDelete_NotFound verifies that a DELETE GitLab answers
// 404 says the variable may already be gone, which is the one reading of that
// status a delete has, rather than repeating the privilege sentence.
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

// TestInstanceVariableDelete_CancelledContext verifies that a canceled context
// aborts Delete before it builds a request, so nothing reaches GitLab.
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

// TestFormatOutputMarkdown_FullUnmasked verifies that a variable neither
// masked nor hidden has its value printed, with protected and raw reported
// apart from one another.
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

// TestFormatOutputMarkdown_NoDescription verifies that a variable carrying no
// description leaves the row out rather than printing an empty one.
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

// TestFormatListMarkdown_WithVariables compares the whole table for a page
// whose two rows disagree on both flag columns, so neither column can be
// rendering the other's value.
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

// TestInstanceVariableGet_OneFlagAtATime_ReachesItsOwnOutputField verifies that
// each boolean GitLab sends lands in the Output field named after it, and that
// the two fields the SDK does not model reach the output from the captured
// response.
//
// A block of flags has no fixture in which no two values agree, and every
// response this package drove carried masked and raw with the same value, so
// the two assignments in toOutput could trade places with both gates green and
// every assertion here passing. One flag per case, compared against a whole
// Output carrying only that field, distinguishes all four. The scope and the
// hidden cases ride along because nothing asserted that the captured read
// reaches the output at all: replacing both with their zero values used to
// pass this file entire.
func TestInstanceVariableGet_OneFlagAtATime_ReachesItsOwnOutputField(t *testing.T) {
	const plain = `{"key":"SOLO","value":"v","variable_type":"env_var"`
	base := Output{Key: "SOLO", Value: "v", VariableType: "env_var"}
	cases := []struct {
		name string
		body string
		want Output
	}{
		{name: "no flag", body: plain + `}`, want: base},
		{
			name: "protected alone",
			body: plain + `,"protected":true}`,
			want: Output{Key: "SOLO", Value: "v", VariableType: "env_var", Protected: true},
		},
		{
			name: "masked alone",
			body: plain + `,"masked":true}`,
			want: Output{Key: "SOLO", Value: "v", VariableType: "env_var", Masked: true},
		},
		{
			name: "raw alone",
			body: plain + `,"raw":true}`,
			want: Output{Key: "SOLO", Value: "v", VariableType: "env_var", Raw: true},
		},
		{
			name: "hidden alone",
			body: plain + `,"hidden":true}`,
			want: Output{Key: "SOLO", Value: "v", VariableType: "env_var", Hidden: true},
		},
		{
			name: "environment scope alone",
			body: plain + `,"environment_scope":"production"}`,
			want: Output{Key: "SOLO", Value: "v", VariableType: "env_var", EnvironmentScope: "production"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, tc.body)
			}))
			got, err := Get(context.Background(), client, GetInput{Key: "SOLO"})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Get() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestInstanceVariableList_PairsEachVariableWithItsOwnCapturedFields verifies
// that the captured environment scope and hidden flag reach the variable they
// were sent for, not the first one in the page.
//
// The SDK decodes the array and the capture is read beside it, so the two
// sequences are joined by position alone: an index that stopped moving would
// stamp the head variable's scope and hidden flag onto every row, and a page
// whose variables agree on both cannot show it. These two disagree on both,
// and the whole slice is compared rather than one row.
func TestInstanceVariableList_PairsEachVariableWithItsOwnCapturedFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"key":"OPEN","value":"open-value","variable_type":"env_var","environment_scope":"production"},
			{"key":"SEALED","value":"sealed-value","variable_type":"file","hidden":true,"environment_scope":"staging"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := []Output{
		{Key: "OPEN", Value: "open-value", VariableType: "env_var", EnvironmentScope: "production"},
		{Key: "SEALED", Value: "sealed-value", VariableType: "file", Hidden: true, EnvironmentScope: "staging"},
	}
	if !reflect.DeepEqual(out.Variables, want) {
		t.Errorf("List() variables = %+v, want %+v", out.Variables, want)
	}
}

// TestInstanceVariables_Refusals_NameTheirOwnOperationAndHint verifies that a
// refused call is reported under the operation that was refused and carries
// the corrective sentence written for it.
//
// Both halves are plain strings a handler passes to a wrapper, so no gate can
// be wrong about either: the operation constants of two handlers could be
// exchanged, and the three admin-privilege sentences could be dealt out to the
// wrong verbs, with the suite green throughout. What a reader is told then is
// which call failed, and it is the wrong one.
func TestInstanceVariables_Refusals_NameTheirOwnOperationAndHint(t *testing.T) {
	refuse := func(status int, body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, status, body)
		}))
	}
	cases := []struct {
		name     string
		call     func() error
		wantOp   string
		wantHint string
	}{
		{
			name: "list forbidden",
			call: func() error {
				_, err := List(context.Background(), refuse(http.StatusForbidden, bodyForbidden), ListInput{})
				return err
			},
			wantOp:   "list instance variables",
			wantHint: "instance-level CI/CD variables are admin-only. Verify your token has admin scope",
		},
		{
			name: "get missing key",
			call: func() error {
				_, err := Get(context.Background(), refuse(http.StatusNotFound, bodyNotFound), GetInput{Key: "GONE"})
				return err
			},
			wantOp:   "get instance variable",
			wantHint: "verify the variable key exists with gitlab_instance_variable_list; admin-only API",
		},
		{
			name: "create forbidden",
			call: func() error {
				_, err := Create(context.Background(), refuse(http.StatusForbidden, bodyForbidden), CreateInput{Key: "K", Value: "V"})
				return err
			},
			wantOp:   "create instance variable",
			wantHint: "creating instance variables requires admin privileges",
		},
		{
			name: "update forbidden",
			call: func() error {
				_, err := Update(context.Background(), refuse(http.StatusForbidden, bodyForbidden), UpdateInput{Key: "K", Value: "V"})
				return err
			},
			wantOp:   "update instance variable",
			wantHint: "updating instance variables requires admin privileges",
		},
		{
			name: "delete forbidden",
			call: func() error {
				return Delete(context.Background(), refuse(http.StatusForbidden, bodyForbidden), DeleteInput{Key: "K"})
			},
			wantOp:   "delete instance variable",
			wantHint: "deleting instance variables requires admin privileges",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.HasPrefix(err.Error(), tc.wantOp+": ") {
				t.Errorf("error = %v, want it reported under %q", err, tc.wantOp)
			}
			if !strings.Contains(err.Error(), "Suggestion: "+tc.wantHint) {
				t.Errorf("error = %v, want the suggestion %q", err, tc.wantHint)
			}
		})
	}
}

// TestFormatListMarkdown_EscapesTableCells verifies that a key carrying a pipe
// is escaped rather than splitting the row into an extra column.
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
