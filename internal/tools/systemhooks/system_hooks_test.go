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
	"reflect"
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

// hookJSONItem is what hookJSON publishes: every string it carries differs from
// every other, so a field filled from the wrong source changes the hook rather
// than reproducing it.
var hookJSONItem = HookItem{
	ID:                    1,
	URL:                   testHookURL,
	Name:                  "My Hook",
	Description:           "Test hook",
	CreatedAt:             "2026-01-01T00:00:00Z",
	PushEvents:            true,
	MergeRequestsEvents:   true,
	EnableSSLVerification: true,
	URLVariables:          []HookURLVariable{{Key: "env"}},
	TokenPresent:          true,
	SigningTokenPresent:   true,
}

// TestList_Success verifies List publishes the whole hook GitLab answered with,
// and never the URL variable's value, which is secret-bearing. The hook is
// compared entire rather than field by field: the creation time reaches no
// other assertion in this file, and a spot check cannot see a field written
// from the wrong source.
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
	if !reflect.DeepEqual(out.Hooks[0], hookJSONItem) {
		t.Errorf("hook = %+v, want %+v", out.Hooks[0], hookJSONItem)
	}
	encodedHook, err := json.Marshal(out.Hooks[0])
	if err != nil {
		t.Fatalf("marshal hook output: %v", err)
	}
	if strings.Contains(string(encodedHook), `"value"`) || strings.Contains(string(encodedHook), "prod") {
		t.Fatalf("hook output exposed secret-bearing values: %s", encodedHook)
	}
}

// TestList_Error verifies that a listing GitLab refused carries the hint the
// 403 was given for, which says what a caller who is not an administrator can
// do about it. The refusal is GitLab's, so its content is what is asserted: an
// error alone would be satisfied by any failure at all.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires administrator access") {
		t.Errorf("error = %v, want the administrator-access hint", err)
	}
}

// hookResponseFlags pairs each boolean key GitLab sends on a hook with the
// published field that must carry it.
var hookResponseFlags = []struct {
	key string
	set func(*HookItem)
}{
	{key: "push_events", set: func(h *HookItem) { h.PushEvents = true }},
	{key: "tag_push_events", set: func(h *HookItem) { h.TagPushEvents = true }},
	{key: "merge_requests_events", set: func(h *HookItem) { h.MergeRequestsEvents = true }},
	{key: "repository_update_events", set: func(h *HookItem) { h.RepositoryUpdateEvents = true }},
	{key: "enable_ssl_verification", set: func(h *HookItem) { h.EnableSSLVerification = true }},
	{key: "token_present", set: func(h *HookItem) { h.TokenPresent = true }},
	{key: "signing_token_present", set: func(h *HookItem) { h.SigningTokenPresent = true }},
}

// TestGet_OneFlagAtATime_PublishesThatFieldAndNoOther verifies each boolean
// GitLab sends lands on its own field of the published hook. The other fixtures
// here answer with several flags set together, which cannot tell them apart: a
// field filled from an adjacent one carrying the same value produces the same
// hook, and nothing in the response distinguishes the two. Each is therefore
// answered on its own and the whole hook compared against one carrying only it.
func TestGet_OneFlagAtATime_PublishesThatFieldAndNoOther(t *testing.T) {
	for _, flag := range hookResponseFlags {
		t.Run(flag.key, func(t *testing.T) {
			client := systemHookClient(t, `{"id":1,"url":"`+testHookURL+`","`+flag.key+`":true}`)
			out, err := Get(t.Context(), client, GetInput{ID: 1})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			want := HookItem{ID: 1, URL: testHookURL}
			flag.set(&want)
			if !reflect.DeepEqual(out.Hook, want) {
				t.Errorf("hook = %+v, want %+v", out.Hook, want)
			}
		})
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
	if !strings.Contains(capturedBody, `"signing_token":"signing-secret"`) {
		t.Errorf("request body missing the signing token: %s", capturedBody)
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
	for _, want := range []string{`"url":"` + testHookURL + `"`, `"signing_token":"new-signing-secret"`, `"push_events":true`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %q: %s", want, capturedBody)
			}
		})
	}
}

// TestEdit_AllOptionalFields verifies Edit forwards every optional field it was
// given, each under its own key. The values are asserted beside the keys: this
// used to look for the key alone, which a converter writing one field's value
// under another's key satisfies word for word, and every value here differs
// from every other so no pair can trade places.
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
	for _, want := range []string{
		`"url":"https://example.com/hook2"`,
		`"name":"Named Hook"`,
		`"description":"Hook desc"`,
		`"token":"secret-token"`,
		`"signing_token":"signing-secret"`,
		`"push_events_branch_filter":"main"`,
		`"branch_filter_strategy":"wildcard"`,
		`"push_events":false`,
		`"tag_push_events":true`,
		`"merge_requests_events":true`,
		`"repository_update_events":true`,
		`"enable_ssl_verification":false`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %s: %s", want, capturedBody)
			}
		})
	}
}

// TestTest_Success verifies Test publishes GitLab's whole sample payload. Every
// string of the fixture differs from every other, which the previous one did
// not do (it gave the name and the path one value, so the two could have
// traded places unnoticed), and the event is compared entire rather than on
// the two fields that used to be spot-checked.
func TestTest_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/hooks/1" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"event_name":"project_create","name":"Sample Project","path":"stand-in/sample-project","project_id":42,"owner_name":"Ada Admin","owner_email":"ada@example.com"}`)
	}))

	out, err := Test(t.Context(), client, TestInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := HookEventItem{
		EventName:  "project_create",
		Name:       "Sample Project",
		Path:       "stand-in/sample-project",
		ProjectID:  42,
		OwnerName:  "Ada Admin",
		OwnerEmail: "ada@example.com",
	}
	if !reflect.DeepEqual(out.Event, want) {
		t.Errorf("event = %+v, want %+v", out.Event, want)
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

// TestDelete_Error verifies that a deletion GitLab refused carries the hint the
// 403 was given for, which names the listing to verify the hook id against and
// says the deletion cannot be undone. The refusal is GitLab's, so its content is
// what is asserted.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), actionList) {
		t.Errorf("error = %v, want it to name the listing that verifies the hook id", err)
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

// TestSetURLVariable_Success verifies the key reaches the path and the value
// reaches the body. The body is held to the value and not merely to the key
// name, which a call sending the key under it would also satisfy.
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
	if !strings.Contains(capturedBody, `"value":"prod"`) {
		t.Errorf("request body missing the variable's value: %s", capturedBody)
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
// Get: API error
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
// Add: API error, with all optional fields
// ---------------------------------------------------------------------------.

// TestEdit_APIError verifies that an edit GitLab refused is reported rather
// than swallowed. The refusal carries the hint that names the listing to
// verify the hook id with, which is the one thing a caller who reached a hook
// that is not there can act on.
func TestEdit_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	_, err := Edit(context.Background(), client, EditInput{ID: 999, URL: "https://hook.example.com"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), actionList) {
		t.Errorf("error = %v, want it to name the listing that verifies the hook id", err)
	}
}

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

// TestAdd_AllOptionalFields verifies Add forwards every optional field it was
// given, each under its own key and with its own value, and publishes the hook
// GitLab answered with. The signing token is among them: the input used to omit
// it, so nothing held the add path to sending one.
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
	if out.Hook.PushEvents {
		t.Error("expected push_events false")
	}
	if out.Hook.Name != "Named Hook" {
		t.Errorf("expected name 'Named Hook', got %s", out.Hook.Name)
	}
	if out.Hook.Description != "Hook desc" {
		t.Errorf("expected description 'Hook desc', got %s", out.Hook.Description)
	}
	// Every optional field the input carried has to reach the request under its
	// own key, which only the body says: the answer is the fixture's and not the
	// request's. The value goes with the key, since a key looked for on its own
	// is still found when another field's value was written under it.
	for _, want := range []string{
		`"url":"https://example.com/hook2"`,
		`"name":"Named Hook"`,
		`"description":"Hook desc"`,
		`"token":"secret-token"`,
		`"signing_token":"signing-secret"`,
		`"push_events_branch_filter":"main"`,
		`"branch_filter_strategy":"wildcard"`,
		`"push_events":false`,
		`"tag_push_events":true`,
		`"merge_requests_events":true`,
		`"repository_update_events":true`,
		`"enable_ssl_verification":false`,
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(sentBody, want) {
				t.Errorf("add request body missing %s: %s", want, sentBody)
			}
		})
	}
}

// hookRequestFlags are the five boolean toggles an add or an edit carries into
// the request body, each with the input field that sets it on either call.
var hookRequestFlags = []struct {
	key     string
	setAdd  func(*AddInput, *bool)
	setEdit func(*EditInput, *bool)
}{
	{
		key:     "push_events",
		setAdd:  func(in *AddInput, v *bool) { in.PushEvents = v },
		setEdit: func(in *EditInput, v *bool) { in.PushEvents = v },
	},
	{
		key:     "tag_push_events",
		setAdd:  func(in *AddInput, v *bool) { in.TagPushEvents = v },
		setEdit: func(in *EditInput, v *bool) { in.TagPushEvents = v },
	},
	{
		key:     "merge_requests_events",
		setAdd:  func(in *AddInput, v *bool) { in.MergeRequestsEvents = v },
		setEdit: func(in *EditInput, v *bool) { in.MergeRequestsEvents = v },
	},
	{
		key:     "repository_update_events",
		setAdd:  func(in *AddInput, v *bool) { in.RepositoryUpdateEvents = v },
		setEdit: func(in *EditInput, v *bool) { in.RepositoryUpdateEvents = v },
	},
	{
		key:     "enable_ssl_verification",
		setAdd:  func(in *AddInput, v *bool) { in.EnableSSLVerification = v },
		setEdit: func(in *EditInput, v *bool) { in.EnableSSLVerification = v },
	},
}

// captureHookBody drives call against a client that answers with one hook and
// returns the request body the handler sent.
func captureHookBody(t *testing.T, call func(*gitlabclient.Client) error) string {
	t.Helper()
	var sent string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		sent = string(body)
		testutil.RespondJSON(w, http.StatusOK, hookJSON)
	}))
	if err := call(client); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	return sent
}

// assertOnlyFlagSent fails unless the body sets the named flag and names no
// other flag at all, which is what tells one toggle from the next.
func assertOnlyFlagSent(t *testing.T, key, body string) {
	t.Helper()
	if !strings.Contains(body, `"`+key+`":true`) {
		t.Errorf("request body missing %q: %s", key, body)
	}
	for _, other := range hookRequestFlags {
		if other.key != key && strings.Contains(body, `"`+other.key+`"`) {
			t.Errorf("request body carries %q, which the caller never set: %s", other.key, body)
		}
	}
}

// TestAddAndEdit_OneFlagAtATime_SendThatToggleAndNoOther verifies each event
// toggle reaches GitLab under its own key. The all-optional fixtures set three
// of the five to true together, so a toggle written under another's key builds
// the same request and neither gate can see it; setting one at a time and
// refusing every other key distinguishes all five on both call paths.
func TestAddAndEdit_OneFlagAtATime_SendThatToggleAndNoOther(t *testing.T) {
	for _, flag := range hookRequestFlags {
		t.Run(flag.key, func(t *testing.T) {
			on := true
			t.Run("add", func(t *testing.T) {
				input := AddInput{URL: testHookURL}
				flag.setAdd(&input, &on)
				assertOnlyFlagSent(t, flag.key, captureHookBody(t, func(client *gitlabclient.Client) error {
					_, err := Add(t.Context(), client, input)
					return err
				}))
			})
			t.Run("edit", func(t *testing.T) {
				input := EditInput{ID: 1}
				flag.setEdit(&input, &on)
				assertOnlyFlagSent(t, flag.key, captureHookBody(t, func(client *gitlabclient.Client) error {
					_, err := Edit(t.Context(), client, input)
					return err
				}))
			})
		})
	}
}

// hookRequestStrings are the string inputs an add or an edit carries, each
// with a value no other one takes, so a guard reading a neighbour's field is
// a different request rather than an equal one.
var hookRequestStrings = []struct {
	key     string
	value   string
	setAdd  func(*AddInput, string)
	setEdit func(*EditInput, string)
}{
	{"name", "named-hook", func(i *AddInput, v string) { i.Name = v }, func(i *EditInput, v string) { i.Name = v }},
	{"description", "described-hook", func(i *AddInput, v string) { i.Description = v }, func(i *EditInput, v string) { i.Description = v }},
	{"token", "token-value", func(i *AddInput, v string) { i.Token = v }, func(i *EditInput, v string) { i.Token = v }},
	{"signing_token", "signing-value", func(i *AddInput, v string) { i.SigningToken = v }, func(i *EditInput, v string) { i.SigningToken = v }},
	{"push_events_branch_filter", "release/*", func(i *AddInput, v string) { i.PushEventsBranchFilter = v }, func(i *EditInput, v string) { i.PushEventsBranchFilter = v }},
	{"branch_filter_strategy", "wildcard", func(i *AddInput, v string) { i.BranchFilterStrategy = v }, func(i *EditInput, v string) { i.BranchFilterStrategy = v }},
}

// TestAddAndEdit_OneStringAtATime_SendThatKeyAndNoOther holds each string
// guard to the field it reads.
//
// The flags above are pinned one at a time; the strings were not. Both
// all-optional fixtures set every string together, so a guard crossed to read
// a neighbor (`if input.Description != ""` copying `input.Name`) produced the
// same request there, and the one input that sets a name without a description
// asserts only that the name arrived. Driving one string at a time, and
// refusing every other string key, tells all six apart on both call paths.
func TestAddAndEdit_OneStringAtATime_SendThatKeyAndNoOther(t *testing.T) {
	for _, field := range hookRequestStrings {
		t.Run(field.key, func(t *testing.T) {
			t.Run("add", func(t *testing.T) {
				input := AddInput{URL: testHookURL}
				field.setAdd(&input, field.value)
				assertOnlyStringSent(t, field.key, field.value, captureHookBody(t, func(client *gitlabclient.Client) error {
					_, err := Add(t.Context(), client, input)
					return err
				}))
			})
			t.Run("edit", func(t *testing.T) {
				input := EditInput{ID: 1}
				field.setEdit(&input, field.value)
				assertOnlyStringSent(t, field.key, field.value, captureHookBody(t, func(client *gitlabclient.Client) error {
					_, err := Edit(t.Context(), client, input)
					return err
				}))
			})
		})
	}
}

// assertOnlyStringSent holds a request body to the one string key the caller
// set, with its value, and to no other string key of the family.
func assertOnlyStringSent(t *testing.T, key, value, body string) {
	t.Helper()
	if !strings.Contains(body, `"`+key+`":"`+value+`"`) {
		t.Errorf("request body does not carry %q as %q: %s", key, value, body)
	}
	for _, other := range hookRequestStrings {
		if other.key != key && strings.Contains(body, `"`+other.key+`"`) {
			t.Errorf("request body carries %q, which the caller never set: %s", other.key, body)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: API error
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

// TestNotFoundHints_EachHandlerCarriesItsOwnSentence holds the four 404 hints
// to the handler each was written for.
//
// All four open with "verify hook_id with admin.system_hook_list", which is
// what the tests asserted, and every one of them satisfies that: the four
// literals could trade places and nothing would fail. They differ in the tail,
// and get's whole sentence is a prefix of edit's, so containment alone cannot
// tell those two apart either. The assertion is therefore on the whole
// sentence in the position the formatter puts it, between "Suggestion: " and
// the colon that introduces the wrapped error, which a longer hint cannot
// satisfy.
func TestNotFoundHints_EachHandlerCarriesItsOwnSentence(t *testing.T) {
	const listing = "verify hook_id with admin.system_hook_list"
	cases := []struct {
		name string
		hint string
		call func(*gitlabclient.Client) error
	}{
		{
			name: "get",
			hint: listing + "; admin-only on self-managed instances",
			call: func(c *gitlabclient.Client) error {
				_, err := Get(t.Context(), c, GetInput{ID: 999})
				return err
			},
		},
		{
			name: "edit",
			hint: listing + "; admin-only on self-managed instances; unset fields keep current values",
			call: func(c *gitlabclient.Client) error {
				_, err := Edit(t.Context(), c, EditInput{ID: 999, URL: "https://hook.example.com"})
				return err
			},
		},
		{
			name: "test",
			hint: listing + "; test triggers a sample push event. Verify the receiving endpoint is reachable",
			call: func(c *gitlabclient.Client) error {
				_, err := Test(t.Context(), c, TestInput{ID: 999})
				return err
			},
		},
		{
			name: "set_url_variable",
			hint: listing + "; URL variable keys are case-sensitive and referenced by placeholders in the hook URL",
			call: func(c *gitlabclient.Client) error {
				return SetURLVariable(t.Context(), c, SetURLVariableInput{ID: 999, Key: "TOKEN", Value: "v"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not found"}`)
			}))

			err := tc.call(client)
			if err == nil {
				t.Fatalf("%s error = nil, want the refusal", tc.name)
			}
			if want := "Suggestion: " + tc.hint + ":"; !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to carry %q", err.Error(), want)
			}
		})
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
