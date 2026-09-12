// system_hooks_test.go contains unit tests for the system hook MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package systemhooks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testHookURL identifies the test hook URL constant used by this package.
const testHookURL = "https://example.com/hook"

// errExpectedErrZeroID identifies the err expected err zero ID constant used by this package.
const errExpectedErrZeroID = "expected error for zero ID, got nil"

// hookJSON identifies the hook JSON constant used by this package.
const hookJSON = `{"id":1,"url":"https://example.com/hook","name":"My Hook","description":"Test hook","created_at":"2026-01-01T00:00:00Z","push_events":true,"tag_push_events":false,"merge_requests_events":true,"repository_update_events":false,"enable_ssl_verification":true,"url_variables":[{"key":"env","value":"prod"}],"token_present":true,"signing_token_present":true}`

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/hooks" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+hookJSON+`]`)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(out.Hooks))
	}
	if out.Hooks[0].URL != testHookURL {
		t.Errorf("expected %s, got %s", testHookURL, out.Hooks[0].URL)
	}
	if out.Hooks[0].Name != "My Hook" {
		t.Errorf("expected name 'My Hook', got %s", out.Hooks[0].Name)
	}
	if out.Hooks[0].Description != "Test hook" {
		t.Errorf("expected description 'Test hook', got %s", out.Hooks[0].Description)
	}
	if !out.Hooks[0].TokenPresent || !out.Hooks[0].SigningTokenPresent {
		t.Error("expected token presence flags to be true")
	}
	if len(out.Hooks[0].URLVariables) != 1 || out.Hooks[0].URLVariables[0].Key != "env" {
		t.Fatalf("unexpected URL variables: %+v", out.Hooks[0].URLVariables)
	}
	encodedHook, err := json.Marshal(out.Hooks[0])
	if err != nil {
		t.Fatalf("marshal hook output: %v", err)
	}
	if strings.Contains(string(encodedHook), `"value"`) || strings.Contains(string(encodedHook), "prod") {
		t.Fatalf("hook output exposed secret-bearing values: %s", encodedHook)
	}
}

// TestList_Error verifies List when error.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/hooks/1" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	}))

	out, err := Get(t.Context(), client, GetInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Hook.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Hook.ID)
	}
	if !out.Hook.PushEvents {
		t.Error("expected push_events true")
	}
	if out.Hook.Name != "My Hook" {
		t.Errorf("expected name 'My Hook', got %s", out.Hook.Name)
	}
	if !out.Hook.TokenPresent || !out.Hook.SigningTokenPresent {
		t.Error("expected token presence flags to be true")
	}
}

// TestAdd_Success verifies Add when success.
func TestAdd_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusCreated, hookJSON)
	}))

	tr := true
	out, err := Add(t.Context(), client, AddInput{URL: testHookURL, SigningToken: "signing-secret", PushEvents: &tr})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Hook.URL != testHookURL {
		t.Errorf("expected %s, got %s", testHookURL, out.Hook.URL)
	}
	if !strings.Contains(capturedBody, "signing_token") {
		t.Errorf("request body missing signing_token: %s", capturedBody)
	}
}

// TestAdd_Validation verifies Add validates required fields before calling the API.
func TestAdd_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	if _, err := Add(t.Context(), client, AddInput{}); err == nil {
		t.Fatal("expected error for empty URL, got nil")
	} else if !strings.Contains(err.Error(), "system_hook_add: url is required") {
		t.Fatalf("unexpected URL validation error: %v", err)
	}
}

// TestEdit_Success verifies Edit when success.
func TestEdit_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/hooks/1" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	}))

	tr := true
	out, err := Edit(t.Context(), client, EditInput{ID: 1, URL: testHookURL, SigningToken: "new-signing-secret", PushEvents: &tr})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Hook.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.Hook.ID)
	}
	for _, want := range []string{"url", "signing_token", "push_events"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %q: %s", want, capturedBody)
			}
		})
	}
}

// TestEdit_AllOptionalFields verifies Edit forwards all optional fields.
func TestEdit_AllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v4/hooks/2" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, `{"id":2,"url":"https://example.com/hook2","name":"Named Hook","description":"Hook desc","created_at":"2026-01-01T00:00:00Z","push_events":false,"tag_push_events":true,"merge_requests_events":true,"repository_update_events":true,"enable_ssl_verification":false}`)
	}))

	f, tr := false, true
	out, err := Edit(context.Background(), client, EditInput{
		ID:                     2,
		URL:                    "https://example.com/hook2",
		Name:                   "Named Hook",
		Description:            "Hook desc",
		Token:                  "secret-token",
		SigningToken:           "signing-secret",
		PushEvents:             &f,
		PushEventsBranchFilter: "main",
		BranchFilterStrategy:   "wildcard",
		TagPushEvents:          &tr,
		MergeRequestsEvents:    &tr,
		RepositoryUpdateEvents: &tr,
		EnableSSLVerification:  &f,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Hook.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.Hook.ID)
	}
	for _, want := range []string{"url", "name", "description", "token", "signing_token", "push_events_branch_filter", "branch_filter_strategy", "tag_push_events", "merge_requests_events", "repository_update_events", "enable_ssl_verification"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %q: %s", want, capturedBody)
			}
		})
	}
}

// TestTest_Success verifies Test when success.
func TestTest_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/hooks/1" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"event_name":"project_create","name":"test-proj","path":"test-proj","project_id":42,"owner_name":"admin","owner_email":"admin@example.com"}`)
	}))

	out, err := Test(t.Context(), client, TestInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Event.EventName != "project_create" {
		t.Errorf("expected project_create, got %s", out.Event.EventName)
	}
	if out.Event.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %d", out.Event.ProjectID)
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_ZeroID verifies Get when zero ID.
func TestGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(t.Context(), client, GetInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestTest_ZeroID verifies Test when zero ID.
func TestTest_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Test(t.Context(), client, TestInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestDelete_ZeroID verifies Delete when zero ID.
func TestDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(t.Context(), client, DeleteInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestEdit_ZeroID verifies Edit when zero ID.
func TestEdit_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Edit(t.Context(), client, EditInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestSetURLVariable_Success verifies SetURLVariable when success.
func TestSetURLVariable_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/hooks/1/url_variables/env" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		capturedBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, `{"key":"env","value":"prod"}`)
	}))

	if err := SetURLVariable(t.Context(), client, SetURLVariableInput{ID: 1, Key: "env", Value: "prod"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(capturedBody, "value") {
		t.Errorf("request body missing value: %s", capturedBody)
	}
}

// TestSetURLVariable_IllegalKey422_HintsKeyFormat verifies that a GitLab 19
// 422 "Illegal key or value" response is wrapped with a hint about the accepted
// key characters and the non-empty value requirement.
func TestSetURLVariable_IllegalKey422_HintsKeyFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"error":"Illegal key or value"}`)
	}))

	err := SetURLVariable(t.Context(), client, SetURLVariableInput{ID: 1, Key: "env1", Value: "prod"})
	if err == nil {
		t.Fatal("expected error for 422 Illegal key or value")
	}
	if !strings.Contains(err.Error(), "letters and underscores") {
		t.Errorf("error = %q, want key-format hint", err.Error())
	}
}

// TestDeleteURLVariable_Success verifies DeleteURLVariable when success.
func TestDeleteURLVariable_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/hooks/1/url_variables/env" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	if err := DeleteURLVariable(t.Context(), client, DeleteURLVariableInput{ID: 1, Key: "env"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestSetURLVariable_Validation verifies SetURLVariable validation branches.
func TestSetURLVariable_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	if err := SetURLVariable(t.Context(), client, SetURLVariableInput{}); err == nil {
		t.Fatal(errExpectedErrZeroID)
	} else if !strings.Contains(err.Error(), "system_hook_set_url_variable: id is required") {
		t.Fatalf("unexpected zero-id validation error: %v", err)
	}
	if err := SetURLVariable(t.Context(), client, SetURLVariableInput{ID: 1}); err == nil {
		t.Fatal("expected error for empty key, got nil")
	} else if !strings.Contains(err.Error(), "system_hook_set_url_variable: key is required") {
		t.Fatalf("unexpected key validation error: %v", err)
	}
	if err := SetURLVariable(t.Context(), client, SetURLVariableInput{ID: 1, Key: "env"}); err == nil {
		t.Fatal("expected error for empty value, got nil")
	} else if !strings.Contains(err.Error(), "system_hook_set_url_variable: value is required") {
		t.Fatalf("unexpected value validation error: %v", err)
	}
}

// TestDeleteURLVariable_Validation verifies DeleteURLVariable validation branches.
func TestDeleteURLVariable_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	if err := DeleteURLVariable(t.Context(), client, DeleteURLVariableInput{}); err == nil {
		t.Fatal(errExpectedErrZeroID)
	} else if !strings.Contains(err.Error(), "system_hook_delete_url_variable: id is required") {
		t.Fatalf("unexpected zero-id validation error: %v", err)
	}
	if err := DeleteURLVariable(t.Context(), client, DeleteURLVariableInput{ID: 1}); err == nil {
		t.Fatal("expected error for empty key, got nil")
	} else if !strings.Contains(err.Error(), "system_hook_delete_url_variable: key is required") {
		t.Fatalf("unexpected key validation error: %v", err)
	}
}

// TestURLVariable_APIErrors verifies URL variable backend errors.
func TestURLVariable_APIErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	if err := SetURLVariable(t.Context(), client, SetURLVariableInput{ID: 1, Key: "env", Value: "prod"}); err == nil {
		t.Fatal(errExpectedAPI)
	}
	if err := DeleteURLVariable(t.Context(), client, DeleteURLVariableInput{ID: 1, Key: "env"}); err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// The Markdown formatters are asserted whole in markdown_test.go.

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// Get — API error
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies Get when API error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Add — API error, with all optional fields
// ---------------------------------------------------------------------------.

// TestAdd_APIError verifies Add when API error.
func TestAdd_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Add(context.Background(), client, AddInput{URL: "https://bad.example.com"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestAdd_AllOptionalFields verifies Add when all optional fields.
func TestAdd_AllOptionalFields(t *testing.T) {
	var sentBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read request body: %v", readErr)
			}
			sentBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"url":"https://example.com/hook2","name":"Named Hook","description":"Hook desc","created_at":"2026-01-01T00:00:00Z","push_events":false,"tag_push_events":true,"merge_requests_events":true,"repository_update_events":true,"enable_ssl_verification":false}`)
			return
		}
		http.NotFound(w, r)
	}))

	f, tr := false, true
	out, err := Add(context.Background(), client, AddInput{
		URL:                    "https://example.com/hook2",
		Name:                   "Named Hook",
		Description:            "Hook desc",
		Token:                  "secret-token",
		PushEvents:             &f,
		PushEventsBranchFilter: "main",
		BranchFilterStrategy:   "wildcard",
		TagPushEvents:          &tr,
		MergeRequestsEvents:    &tr,
		RepositoryUpdateEvents: &tr,
		EnableSSLVerification:  &f,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Hook.ID != 2 {
		t.Errorf("expected ID 2, got %d", out.Hook.ID)
	}
	if out.Hook.PushEvents {
		t.Error("expected push_events false")
	}
	if out.Hook.Name != "Named Hook" {
		t.Errorf("expected name 'Named Hook', got %s", out.Hook.Name)
	}
	if out.Hook.Description != "Hook desc" {
		t.Errorf("expected description 'Hook desc', got %s", out.Hook.Description)
	}
	// Every optional field the input carried has to reach the request, which
	// only the body says: the answer is the fixture's and not the request's.
	for _, want := range []string{`"description":"Hook desc"`, `"name":"Named Hook"`, `"token":"secret-token"`, `"push_events_branch_filter":"main"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(sentBody, want) {
				t.Errorf("add request body missing %s: %s", want, sentBody)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test — API error
// ---------------------------------------------------------------------------.

// TestTest_APIError verifies Test when API error.
func TestTest_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":msgBadRequest}`)
	}))
	_, err := Test(context.Background(), client, TestInput{ID: 999})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestEdit_WithoutURL_LeavesTheURLOutOfTheRequest verifies that editing a hook
// without a url sends no url at all, rather than sending an empty one that
// would clear the hook's endpoint.
func TestEdit_WithoutURL_LeavesTheURLOutOfTheRequest(t *testing.T) {
	var sentBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("read request body: %v", readErr)
		}
		sentBody = string(body)
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	}))

	if _, err := Edit(t.Context(), client, EditInput{ID: 1, Name: "Renamed"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if strings.Contains(sentBody, `"url"`) {
		t.Errorf("edit request body carries a url it was not given: %s", sentBody)
	}
	if !strings.Contains(sentBody, `"name":"Renamed"`) {
		t.Errorf("edit request body missing the new name: %s", sentBody)
	}
}

// ---------------------------------------------------------------------------
// Fields GitLab sends beside the ones the SDK's own Hook models
// ---------------------------------------------------------------------------.

// hookSentJSON is one hook carrying every key the SDK's Hook leaves out: the
// branch filter and the strategy that reads it, the alert status and how long
// the hook stays disabled, the payload template, the custom headers and the
// organization a system hook belongs to.
const hookSentJSON = `{"id":1,"url":"https://example.com/hook","name":"My Hook",` +
	`"created_at":"2026-01-01T00:00:00Z","push_events":true,` +
	`"push_events_branch_filter":"release/*","branch_filter_strategy":"wildcard",` +
	`"alert_status":"temporarily_disabled","disabled_until":"2026-02-03T04:05:06Z",` +
	`"custom_webhook_template":"{\"event\":\"push\"}",` +
	`"custom_headers":[{"key":"X-Env","value":"prod"}],"organization_id":7,` +
	`"token_present":true,"signing_token_present":true}`

// hookWithoutConditionalJSON is the same hook without the two keys GitLab
// sends only under a condition: the headers a caller can ask to be left out,
// and the organization only a system hook has.
const hookWithoutConditionalJSON = `{"id":1,"url":"https://example.com/hook","name":"My Hook",` +
	`"push_events":true,"push_events_branch_filter":"release/*","branch_filter_strategy":"wildcard",` +
	`"alert_status":"executable","disabled_until":null,"custom_webhook_template":""}`

// systemHookClient answers every request with body, which the caller writes as
// an array for the list handler and as an object for the rest.
func systemHookClient(t *testing.T, body string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
}

// hookCalls are the four handlers that answer with a hook, each taking the
// body its endpoint answers with and returning the hook it published.
var hookCalls = []struct {
	name string
	call func(client *gitlabclient.Client) (HookItem, error)
	// list says whether the endpoint answers with an array.
	list bool
}{
	{name: "list", list: true, call: func(client *gitlabclient.Client) (HookItem, error) {
		out, err := List(context.Background(), client, ListInput{})
		if err != nil {
			return HookItem{}, err
		}
		if len(out.Hooks) != 1 {
			return HookItem{}, errNoHook
		}
		return out.Hooks[0], nil
	}},
	{name: "get", call: func(client *gitlabclient.Client) (HookItem, error) {
		out, err := Get(context.Background(), client, GetInput{ID: 1})
		return out.Hook, err
	}},
	{name: "add", call: func(client *gitlabclient.Client) (HookItem, error) {
		out, err := Add(context.Background(), client, AddInput{URL: testHookURL})
		return out.Hook, err
	}},
	{name: "edit", call: func(client *gitlabclient.Client) (HookItem, error) {
		out, err := Edit(context.Background(), client, EditInput{ID: 1, URL: testHookURL})
		return out.Hook, err
	}},
}

// errNoHook reports a handler that answered without the single hook the body
// carries, which would otherwise show up as a nil-index panic.
var errNoHook = errors.New("the handler published no hook")

// hookSentValues is what each string-valued key of hookSentJSON carries, so
// one table checks them all.
var hookSentValues = map[string]string{
	"push_events_branch_filter": "release/*",
	"branch_filter_strategy":    "wildcard",
	"alert_status":              "temporarily_disabled",
	"disabled_until":            "2026-02-03T04:05:06Z",
	"custom_webhook_template":   `{"event":"push"}`,
}

// hookBodyFor wraps the object body in an array for a list endpoint.
func hookBodyFor(list bool, body string) string {
	if list {
		return "[" + body + "]"
	}
	return body
}

// TestSystemHooks_PublishTheFieldsGitLabSendsBesideTheSDKs verifies that each
// handler that answers with a hook publishes the seven keys the SDK's Hook
// does not model, read off the captured response.
func TestSystemHooks_PublishTheFieldsGitLabSendsBesideTheSDKs(t *testing.T) {
	for _, hookCall := range hookCalls {
		t.Run(hookCall.name, func(t *testing.T) {
			hook, err := hookCall.call(systemHookClient(t, hookBodyFor(hookCall.list, hookSentJSON)))
			if err != nil {
				t.Fatalf("%s: %v", hookCall.name, err)
			}
			for field, got := range map[string]string{
				"push_events_branch_filter": hook.PushEventsBranchFilter,
				"branch_filter_strategy":    hook.BranchFilterStrategy,
				"alert_status":              hook.AlertStatus,
				"disabled_until":            hook.DisabledUntil,
				"custom_webhook_template":   hook.CustomWebhookTemplate,
			} {
				t.Run(field, func(t *testing.T) {
					if want := hookSentValues[field]; got != want {
						t.Errorf("%s = %q, want %q", field, got, want)
					}
				})
			}
			if len(hook.CustomHeaders) != 1 || hook.CustomHeaders[0].Key != "X-Env" {
				t.Errorf("custom_headers = %+v, want the one header GitLab sent", hook.CustomHeaders)
			}
			if hook.OrganizationID != 7 {
				t.Errorf("organization_id = %d, want 7", hook.OrganizationID)
			}
		})
	}
}

// TestSystemHooks_OmitTheConditionalFieldsGitLabDidNotSend verifies that a
// hook answered without the headers and without the organization publishes
// neither, and that the unconditional keys beside them still arrive.
func TestSystemHooks_OmitTheConditionalFieldsGitLabDidNotSend(t *testing.T) {
	for _, hookCall := range hookCalls {
		t.Run(hookCall.name, func(t *testing.T) {
			hook, err := hookCall.call(systemHookClient(t, hookBodyFor(hookCall.list, hookWithoutConditionalJSON)))
			if err != nil {
				t.Fatalf("%s: %v", hookCall.name, err)
			}
			if hook.CustomHeaders != nil {
				t.Errorf("custom_headers = %+v, want none", hook.CustomHeaders)
			}
			if hook.OrganizationID != 0 {
				t.Errorf("organization_id = %d, want none", hook.OrganizationID)
			}
			if hook.DisabledUntil != "" {
				t.Errorf("disabled_until = %q, want none for a hook that is not disabled", hook.DisabledUntil)
			}
			if hook.AlertStatus != "executable" {
				t.Errorf("alert_status = %q, want executable", hook.AlertStatus)
			}
		})
	}
}

// TestSystemHooks_UnreadableCapturedFields verifies every handler that answers
// with a hook reports the captured response's decode failure rather than a
// half-filled hook. The SDK's own Hook has no organization_id, so only the
// read beside it can notice that GitLab sent a string there.
func TestSystemHooks_UnreadableCapturedFields(t *testing.T) {
	const poisoned = `{"id":1,"url":"https://example.com/hook","organization_id":"not-a-number"}`
	cases := make([]testutil.CapturedCase, 0, len(hookCalls))
	for _, hookCall := range hookCalls {
		cases = append(cases, testutil.CapturedCase{Name: hookCall.name, Call: func() error {
			_, err := hookCall.call(systemHookClient(t, hookBodyFor(hookCall.list, poisoned)))
			return err
		}})
	}
	testutil.AssertCapturedDecodeFailures(t, cases)
}

// TestFormatHookMarkdown_CustomHeaderValuesRedacted verifies the custom header
// table names each header and never its value, which GitLab masks and this
// server does not surface.
func TestFormatHookMarkdown_CustomHeaderValuesRedacted(t *testing.T) {
	client := systemHookClient(t, hookSentJSON)
	out, err := Get(t.Context(), client, GetInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	encoded, err := json.Marshal(out.Hook)
	if err != nil {
		t.Fatalf("marshal hook output: %v", err)
	}
	if strings.Contains(string(encoded), "prod") {
		t.Errorf("hook output exposed a custom header value: %s", encoded)
	}
}
