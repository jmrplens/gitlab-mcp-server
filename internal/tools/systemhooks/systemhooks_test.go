// systemhooks_test.go contains unit tests for the system hook MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package systemhooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// testHookURL identifies the test hook URL constant used by this package.
const testHookURL = "https://example.com/hook"

// errExpectedErrZeroID identifies the err expected err zero ID constant used by this package.
const errExpectedErrZeroID = "expected error for zero ID, got nil"

// errAPINotCalledZeroID identifies the err API not called zero ID constant used by this package.
const errAPINotCalledZeroID = "API should not be called when ID is 0"

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
			t.Fatalf("read request body: %v", err)
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))

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
			t.Fatalf("read request body: %v", err)
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
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing %q: %s", want, capturedBody)
		}
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
			t.Fatalf("read request body: %v", err)
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
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing %q: %s", want, capturedBody)
		}
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	_, err := Get(t.Context(), client, GetInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestTest_ZeroID verifies Test when zero ID.
func TestTest_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	_, err := Test(t.Context(), client, TestInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestDelete_ZeroID verifies Delete when zero ID.
func TestDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	err := Delete(t.Context(), client, DeleteInput{ID: 0})
	if err == nil {
		t.Fatal(errExpectedErrZeroID)
	}
}

// TestEdit_ZeroID verifies Edit when zero ID.
func TestEdit_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
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
			t.Fatalf("read request body: %v", err)
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	if err := SetURLVariable(t.Context(), client, SetURLVariableInput{}); err == nil {
		t.Fatal(errExpectedErrZeroID)
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal(errAPINotCalledZeroID)
	}))
	if err := DeleteURLVariable(t.Context(), client, DeleteURLVariableInput{}); err == nil {
		t.Fatal(errExpectedErrZeroID)
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

// TestFormatListMarkdown verifies FormatListMarkdown.
func TestFormatListMarkdown(t *testing.T) {
	result := FormatListMarkdown(ListOutput{
		Hooks: []HookItem{
			{ID: 1, URL: testHookURL, Name: "My Hook", PushEvents: true, EnableSSLVerification: true},
		},
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "example.com") {
		t.Errorf("expected URL in output, got: %s", text)
	}
	if !strings.Contains(text, "My Hook") {
		t.Errorf("expected name in output, got: %s", text)
	}
}

// TestFormatHookMarkdown verifies FormatHookMarkdown.
func TestFormatHookMarkdown(t *testing.T) {
	result := FormatHookMarkdown(HookItem{ID: 1, URL: testHookURL, Name: "My Hook", Description: "A test hook", PushEvents: true})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "System Hook #1") {
		t.Errorf("expected hook header, got: %s", text)
	}
	if !strings.Contains(text, "My Hook") {
		t.Errorf("expected name in output, got: %s", text)
	}
	if !strings.Contains(text, "A test hook") {
		t.Errorf("expected description in output, got: %s", text)
	}
}

// TestFormatHookMarkdown_URLVariablesRedacted verifies system hook markdown
// includes URL variable names while redacting their values.
//
// The formatter receives a hook with token metadata and one URL variable. The
// expected output includes hook details plus REDACTED variable text, preserving
// operational context without leaking webhook secrets.
func TestFormatHookMarkdown_URLVariablesRedacted(t *testing.T) {
	result := FormatHookMarkdown(HookItem{
		ID:           1,
		URL:          testHookURL,
		URLVariables: []HookURLVariable{{Key: "token"}},
	})
	text := result.Content[0].(*mcp.TextContent).Text

	for _, want := range []string{"URL Variables", "token", "REDACTED"} {
		if !strings.Contains(text, want) {
			t.Errorf("FormatHookMarkdown missing %q: %s", want, text)
		}
	}
}

// TestFormatTestMarkdown verifies FormatTestMarkdown.
func TestFormatTestMarkdown(t *testing.T) {
	result := FormatTestMarkdown(TestOutput{Event: HookEventItem{EventName: "project_create", Name: "test", ProjectID: 42}})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "project_create") {
		t.Errorf("expected event name, got: %s", text)
	}
}

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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
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

// ---------------------------------------------------------------------------
// Formatters — empty list
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	result := FormatListMarkdown(ListOutput{})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "No system hooks found") {
		t.Errorf("expected empty message, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Formatters — hook with created_at
// ---------------------------------------------------------------------------.

// TestFormatHookMarkdown_WithCreatedAt verifies FormatHookMarkdown when with created at.
func TestFormatHookMarkdown_WithCreatedAt(t *testing.T) {
	result := FormatHookMarkdown(HookItem{
		ID:        1,
		URL:       "https://example.com/hook",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "1 Jan 2026 00:00 UTC") {
		t.Errorf("expected created_at in output, got: %s", text)
	}
}
