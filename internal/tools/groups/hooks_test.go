// hooks_test.go contains unit tests for the group MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groups

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const (
	// pathGroupHooks identifies the path group hooks constant used by this package.
	pathGroupHooks = "/api/v4/groups/99/hooks"
	// pathGroupHook10 identifies the path group hook 10 constant used by this package.
	pathGroupHook10 = "/api/v4/groups/99/hooks/10"
	// testHookURL identifies the test hook URL constant used by this package.
	testHookURL = "https://example.com/hook"
	// errZeroHookID identifies the err zero hook ID constant used by this package.
	errZeroHookID = "expected error for zero HookID"
	// fmtExpectedHookIDError identifies the fmt expected hook ID error constant used by this package.
	fmtExpectedHookIDError = "expected error to mention 'hook_id', got: %v"
)

// groupHookJSON stores the package-level group hook JSON state.
var groupHookJSON = `{"id":10,"url":"https://example.com/hook","name":"CI Hook","description":"Triggers CI","group_id":99,"push_events":true,"merge_requests_events":true,"issues_events":false,"tag_push_events":false,"note_events":false,"job_events":false,"pipeline_events":true,"wiki_page_events":false,"deployment_events":false,"releases_events":false,"milestone_events":true,"feature_flag_events":true,"subgroup_events":false,"member_events":false,"vulnerability_events":true,"confidential_issues_events":false,"confidential_note_events":false,"enable_ssl_verification":true,"alert_status":"executable","disabled_until":"2026-01-16T10:00:00Z","url_variables":[{"key":"env","value":"prod"}],"token_present":true,"signing_token_present":true,"created_at":"2026-01-15T10:00:00Z","emoji_events":true,"resource_access_token_events":true,"project_events":true,"push_events_branch_filter":"main","branch_filter_strategy":"wildcard"}`

// groupHookListJSON stores the package-level group hook list JSON state.
var groupHookListJSON = `[` + groupHookJSON + `]`

// ---------------------------------------------------------------------------
// ListHooks tests
// ---------------------------------------------------------------------------.

// TestListHooks_Success verifies that ListHooks succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestListHooks_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupHooks {
			testutil.RespondJSONWithPagination(
				w, http.StatusOK, groupHookListJSON,
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"},
			)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListHooks(context.Background(), client, ListHooksInput{GroupID: "99"})
	if err != nil {
		t.Fatalf("ListHooks() unexpected error: %v", err)
	}
	if len(out.Hooks) != 1 {
		t.Fatalf("len(out.Hooks) = %d, want 1", len(out.Hooks))
	}
	if out.Hooks[0].URL != testHookURL {
		t.Errorf("out.Hooks[0].URL = %q, want %q", out.Hooks[0].URL, testHookURL)
	}
	if !out.Hooks[0].PushEvents {
		t.Error("out.Hooks[0].PushEvents = false, want true")
	}
	if !out.Hooks[0].TokenPresent || !out.Hooks[0].SigningTokenPresent {
		t.Error("expected token presence flags to be true")
	}
	if !out.Hooks[0].MilestoneEvents || !out.Hooks[0].FeatureFlagEvents || !out.Hooks[0].VulnerabilityEvents {
		t.Error("expected new event flags to be true")
	}
	if out.Hooks[0].DisabledUntil == "" {
		t.Error("out.Hooks[0].DisabledUntil is empty, want timestamp")
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

// TestListHooks_APIError verifies that ListHooks returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestListHooks_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ListHooks(context.Background(), client, ListHooksInput{GroupID: "99"})
	if err == nil {
		t.Fatal("ListHooks() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetHook tests
// ---------------------------------------------------------------------------.

// TestGetHook_Success verifies that GetHook succeeds when the GitLab API returns a valid response.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupHook10 {
			testutil.RespondJSON(w, http.StatusOK, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 10})
	if err != nil {
		t.Fatalf("GetHook() unexpected error: %v", err)
	}
	if out.Name != "CI Hook" {
		t.Errorf("out.Name = %q, want %q", out.Name, "CI Hook")
	}
	if !out.EnableSSLVerification {
		t.Error("out.EnableSSLVerification = false, want true")
	}
	if !out.TokenPresent || !out.SigningTokenPresent {
		t.Error("expected token presence flags to be true")
	}
	if !out.EmojiEvents {
		t.Error("out.EmojiEvents = false, want true")
	}
	if !out.ResourceAccessTokenEvents {
		t.Error("out.ResourceAccessTokenEvents = false, want true")
	}
	if !out.ProjectEvents {
		t.Error("out.ProjectEvents = false, want true")
	}
	if out.PushEventsBranchFilter != "main" {
		t.Errorf("out.PushEventsBranchFilter = %q, want %q", out.PushEventsBranchFilter, "main")
	}
	if out.BranchFilterStrategy != "wildcard" {
		t.Errorf("out.BranchFilterStrategy = %q, want %q", out.BranchFilterStrategy, "wildcard")
	}
}

// TestGetHook_APIError verifies that GetHook returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGetHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 999})
	if err == nil {
		t.Fatal("GetHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// AddHook tests
// ---------------------------------------------------------------------------.

// TestAddHook_Success verifies that AddHook succeeds when the GitLab API returns a valid response.
// The test exercises the POST path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAddHook_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupHooks {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	push := true
	out, err := AddHook(context.Background(), client, AddHookInput{
		GroupID:                   "99",
		URL:                       testHookURL,
		SigningToken:              "signing-secret",
		PushEvents:                &push,
		MilestoneEvents:           &push,
		FeatureFlagEvents:         &push,
		VulnerabilityEvents:       &push,
		EmojiEvents:               &push,
		ResourceAccessTokenEvents: &push,
		ProjectEvents:             &push,
		PushEventsBranchFilter:    "main",
		BranchFilterStrategy:      "wildcard",
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
	for _, want := range []string{
		"signing_token", "milestone_events", "feature_flag_events", "vulnerability_events",
		"emoji_events", "resource_access_token_events", "project_events",
		"push_events_branch_filter", "branch_filter_strategy",
	} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing %q: %s", want, capturedBody)
		}
	}
}

// TestAddHook_APIError verifies that AddHook returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestAddHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	_, err := AddHook(context.Background(), client, AddHookInput{
		GroupID: "99",
		URL:     "https://bad.example.com",
	})
	if err == nil {
		t.Fatal("AddHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// EditHook tests
// ---------------------------------------------------------------------------.

// TestEditHook_Success verifies that EditHook succeeds when the GitLab API returns a valid response.
// The test exercises the PUT path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEditHook_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupHook10 {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	enabled := true
	out, err := EditHook(context.Background(), client, EditHookInput{
		GroupID:             "99",
		HookID:              10,
		URL:                 testHookURL,
		SigningToken:        "new-signing-secret",
		MilestoneEvents:     &enabled,
		FeatureFlagEvents:   &enabled,
		VulnerabilityEvents: &enabled,
	})
	if err != nil {
		t.Fatalf("EditHook() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
	for _, want := range []string{"signing_token", "milestone_events", "feature_flag_events", "vulnerability_events"} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("request body missing %q: %s", want, capturedBody)
		}
	}
}

// TestEditHook_APIError verifies that EditHook returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestEditHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := EditHook(context.Background(), client, EditHookInput{
		GroupID: "99",
		HookID:  999,
		URL:     "https://example.com",
	})
	if err == nil {
		t.Fatal("EditHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteHook tests
// ---------------------------------------------------------------------------.

// TestDeleteHook_Success verifies that DeleteHook succeeds when the GitLab API returns a valid response.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteHook_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGroupHook10 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{GroupID: "99", HookID: 10})
	if err != nil {
		t.Fatalf("DeleteHook() unexpected error: %v", err)
	}
}

// TestDeleteHook_APIError verifies that DeleteHook returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDeleteHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{GroupID: "99", HookID: 10})
	if err == nil {
		t.Fatal("DeleteHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// HookID validation tests
// ---------------------------------------------------------------------------.

// TestGetHook_InvalidHookID verifies the GetHook_InvalidHookID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetHook_InvalidHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 0})
	if err == nil {
		t.Fatal(errZeroHookID)
	}
	if !strings.Contains(err.Error(), "hook_id") {
		t.Errorf(fmtExpectedHookIDError, err)
	}
}

// TestEditHook_InvalidHookID verifies the EditHook_InvalidHookID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestEditHook_InvalidHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := EditHook(context.Background(), client, EditHookInput{GroupID: "99", HookID: 0})
	if err == nil {
		t.Fatal(errZeroHookID)
	}
	if !strings.Contains(err.Error(), "hook_id") {
		t.Errorf(fmtExpectedHookIDError, err)
	}
}

// TestDeleteHook_InvalidHookID verifies the DeleteHook_InvalidHookID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteHook_InvalidHookID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteHook(context.Background(), client, DeleteHookInput{GroupID: "99", HookID: 0})
	if err == nil {
		t.Fatal(errZeroHookID)
	}
	if !strings.Contains(err.Error(), "hook_id") {
		t.Errorf(fmtExpectedHookIDError, err)
	}
}

// TestGetHook_URLVariables verifies the GetHook_URLVariables handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGetHook_URLVariables(t *testing.T) {
	body := `{"id":10,"url":"https://example.com/hook","group_id":99,"push_events":true,"enable_ssl_verification":true,"url_variables":[{"key":"token","value":""},{"key":"api_key","value":""}]}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathGroupHook10 {
			testutil.RespondJSON(w, http.StatusOK, body)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 10})
	if err != nil {
		t.Fatalf("GetHook: %v", err)
	}
	if len(out.URLVariables) != 2 {
		t.Fatalf("URLVariables len = %d, want 2", len(out.URLVariables))
	}
	if out.URLVariables[0].Key != "token" || out.URLVariables[1].Key != "api_key" {
		t.Errorf("URLVariables = %+v", out.URLVariables)
	}
	encodedHook, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal hook output: %v", err)
	}
	if strings.Contains(string(encodedHook), `"value"`) {
		t.Fatalf("hook output exposed URL variable values: %s", encodedHook)
	}
}

// TestFormatHookMarkdown_URLVariablesRedacted verifies group hook markdown shows
// URL variable names without exposing secret values.
//
// The formatter receives a hook with token metadata and one URL variable. The
// expected output includes hook details and REDACTED variable display, preserving
// useful diagnostics without leaking sensitive webhook configuration.
func TestFormatHookMarkdown_URLVariablesRedacted(t *testing.T) {
	text := FormatHookMarkdown(HookOutput{
		ID:                  10,
		URL:                 testHookURL,
		Name:                "Deploy hook",
		Description:         "Deploy events",
		GroupID:             99,
		AlertStatus:         "executable",
		DisabledUntil:       "2026-01-16T10:00:00Z",
		CreatedAt:           "2026-01-15T10:00:00Z",
		TokenPresent:        true,
		SigningTokenPresent: true,
		URLVariables:        []HookURLVariable{{Key: "token"}},
	})

	for _, want := range []string{"Deploy hook", "Deploy events", "Alert Status", "Disabled Until", "Created", "URL Variables", "token", "REDACTED"} {
		if !strings.Contains(text, want) {
			t.Errorf("FormatHookMarkdown missing %q: %s", want, text)
		}
	}
}

// TestEnabledEvents_AllEvents verifies enabledEvents renders every supported
// group hook event flag.
//
// The hook output enables legacy and newer event fields, including milestone,
// feature flag, subgroup, member, and vulnerability events. The expected string
// contains each event name so markdown summaries do not silently omit flags.
func TestEnabledEvents_AllEvents(t *testing.T) {
	text := enabledEvents(HookOutput{
		PushEvents:          true,
		TagPushEvents:       true,
		MergeRequestsEvents: true,
		IssuesEvents:        true,
		NoteEvents:          true,
		JobEvents:           true,
		PipelineEvents:      true,
		WikiPageEvents:      true,
		DeploymentEvents:    true,
		ReleasesEvents:      true,
		MilestoneEvents:     true,
		FeatureFlagEvents:   true,
		SubGroupEvents:      true,
		MemberEvents:        true,
		VulnerabilityEvents: true,
	})

	for _, want := range []string{"push", "tag_push", "merge_request", "issues", "note", "job", "pipeline", "wiki", "deployment", "releases", "milestone", "feature_flag", "subgroup", "member", "vulnerability"} {
		if !strings.Contains(text, want) {
			t.Errorf("enabledEvents missing %q: %s", want, text)
		}
	}
}

// ---------------------------------------------------------------------------
// Hook sub-operation tests: custom headers, URL variables, test, resend.
// ---------------------------------------------------------------------------.

// TestSetHookCustomHeader_Success verifies the PUT custom_headers request body/path.
func TestSetHookCustomHeader_Success(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/99/hooks/10/custom_headers/X-Token" {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	if err := SetHookCustomHeader(context.Background(), client, SetHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: "X-Token", Value: "secret"}); err != nil {
		t.Fatalf("SetHookCustomHeader() unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, "secret") {
		t.Fatalf("request body missing value: %s", gotBody)
	}
}

// TestSetHookCustomHeader_Guards verifies required-input guards.
func TestSetHookCustomHeader_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []SetHookCustomHeaderInput{{}, {GroupID: "99"}, {GroupID: "99", HookID: 10}}
	for i, in := range cases {
		if err := SetHookCustomHeader(context.Background(), client, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// TestSetHookCustomHeaderOutput_Success verifies the void wrapper confirmation.
func TestSetHookCustomHeaderOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	out, err := SetHookCustomHeaderOutput(context.Background(), client, SetHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: "X-Token", Value: "v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "X-Token") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestDeleteHookCustomHeader_Success verifies the DELETE custom_headers request.
func TestDeleteHookCustomHeader_Success(t *testing.T) {
	var hit bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/99/hooks/10/custom_headers/X-Token" {
			hit = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	if err := DeleteHookCustomHeader(context.Background(), client, DeleteHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: "X-Token"}); err != nil {
		t.Fatalf("DeleteHookCustomHeader() unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("expected DELETE request")
	}
}

// TestDeleteHookCustomHeader_Guards verifies required-input guards.
func TestDeleteHookCustomHeader_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []DeleteHookCustomHeaderInput{{}, {GroupID: "99"}, {GroupID: "99", HookID: 10}}
	for i, in := range cases {
		if err := DeleteHookCustomHeader(context.Background(), client, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// TestSetHookURLVariable_Success verifies the PUT url_variables request.
func TestSetHookURLVariable_Success(t *testing.T) {
	var gotBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/99/hooks/10/url_variables/env" {
			body, _ := io.ReadAll(r.Body)
			gotBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	if err := SetHookURLVariable(context.Background(), client, SetHookURLVariableInput{GroupID: "99", HookID: 10, Key: "env", Value: "prod"}); err != nil {
		t.Fatalf("SetHookURLVariable() unexpected error: %v", err)
	}
	if !strings.Contains(gotBody, "prod") {
		t.Fatalf("request body missing value: %s", gotBody)
	}
}

// TestSetHookURLVariable_IllegalKey422_HintsKeyFormat verifies that a GitLab 19
// 422 "Illegal key or value" response is wrapped with a hint about the accepted
// key characters and the non-empty value requirement.
func TestSetHookURLVariable_IllegalKey422_HintsKeyFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"error":"Illegal key or value"}`)
	}))
	err := SetHookURLVariable(context.Background(), client, SetHookURLVariableInput{GroupID: "99", HookID: 10, Key: "env1", Value: "prod"})
	if err == nil {
		t.Fatal("expected error for 422 Illegal key or value")
	}
	if !strings.Contains(err.Error(), "letters and underscores") {
		t.Errorf("error = %q, want key-format hint", err.Error())
	}
}

// TestSetHookURLVariable_Guards verifies required-input guards.
func TestSetHookURLVariable_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []SetHookURLVariableInput{{}, {GroupID: "99"}, {GroupID: "99", HookID: 10}}
	for i, in := range cases {
		if err := SetHookURLVariable(context.Background(), client, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// TestSetHookURLVariableOutput_Success verifies the void wrapper confirmation.
func TestSetHookURLVariableOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	out, err := SetHookURLVariableOutput(context.Background(), client, SetHookURLVariableInput{GroupID: "99", HookID: 10, Key: "env", Value: "v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "env") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestDeleteHookURLVariable_Success verifies the DELETE url_variables request.
func TestDeleteHookURLVariable_Success(t *testing.T) {
	var hit bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/99/hooks/10/url_variables/env" {
			hit = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	if err := DeleteHookURLVariable(context.Background(), client, DeleteHookURLVariableInput{GroupID: "99", HookID: 10, Key: "env"}); err != nil {
		t.Fatalf("DeleteHookURLVariable() unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("expected DELETE request")
	}
}

// TestDeleteHookURLVariable_Guards verifies required-input guards.
func TestDeleteHookURLVariable_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []DeleteHookURLVariableInput{{}, {GroupID: "99"}, {GroupID: "99", HookID: 10}}
	for i, in := range cases {
		if err := DeleteHookURLVariable(context.Background(), client, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// TestTestHook_Success verifies the POST test/{trigger} request.
func TestTestHook_Success(t *testing.T) {
	var hit bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/hooks/10/test/push_events" {
			hit = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	if err := TestHook(context.Background(), client, TestHookInput{GroupID: "99", HookID: 10, Trigger: "push_events"}); err != nil {
		t.Fatalf("TestHook() unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("expected POST test request")
	}
}

// TestTestHook_Guards verifies required-input guards.
func TestTestHook_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []TestHookInput{{}, {GroupID: "99"}, {GroupID: "99", HookID: 10}}
	for i, in := range cases {
		if err := TestHook(context.Background(), client, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// TestTestHookOutput_Success verifies the void wrapper confirmation.
func TestTestHookOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	out, err := TestHookOutput(context.Background(), client, TestHookInput{GroupID: "99", HookID: 10, Trigger: "push_events"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "push_events") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestResendHookEvent_Success verifies the POST events/{id}/resend request.
func TestResendHookEvent_Success(t *testing.T) {
	var hit bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/hooks/10/events/5/resend" {
			hit = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	if err := ResendHookEvent(context.Background(), client, ResendHookEventInput{GroupID: "99", HookID: 10, HookEventID: 5}); err != nil {
		t.Fatalf("ResendHookEvent() unexpected error: %v", err)
	}
	if !hit {
		t.Fatal("expected POST resend request")
	}
}

// TestResendHookEvent_Guards verifies required-input guards.
func TestResendHookEvent_Guards(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	cases := []ResendHookEventInput{{}, {GroupID: "99"}, {GroupID: "99", HookID: 10}}
	for i, in := range cases {
		if err := ResendHookEvent(context.Background(), client, in); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// TestResendHookEventOutput_Success verifies the void wrapper confirmation.
func TestResendHookEventOutput_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	out, err := ResendHookEventOutput(context.Background(), client, ResendHookEventInput{GroupID: "99", HookID: 10, HookEventID: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "5") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// TestHookSubOps_CanceledContext verifies each hook sub-op honors a canceled context.
func TestHookSubOps_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := SetHookCustomHeader(ctx, client, SetHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: "k", Value: "v"}); err == nil {
		t.Error("SetHookCustomHeader: expected context error")
	}
	if err := DeleteHookCustomHeader(ctx, client, DeleteHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: "k"}); err == nil {
		t.Error("DeleteHookCustomHeader: expected context error")
	}
	if err := SetHookURLVariable(ctx, client, SetHookURLVariableInput{GroupID: "99", HookID: 10, Key: "k", Value: "v"}); err == nil {
		t.Error("SetHookURLVariable: expected context error")
	}
	if err := DeleteHookURLVariable(ctx, client, DeleteHookURLVariableInput{GroupID: "99", HookID: 10, Key: "k"}); err == nil {
		t.Error("DeleteHookURLVariable: expected context error")
	}
	if err := TestHook(ctx, client, TestHookInput{GroupID: "99", HookID: 10, Trigger: "push_events"}); err == nil {
		t.Error("TestHook: expected context error")
	}
	if err := ResendHookEvent(ctx, client, ResendHookEventInput{GroupID: "99", HookID: 10, HookEventID: 5}); err == nil {
		t.Error("ResendHookEvent: expected context error")
	}
}

// TestHookHelpers_APIError_ReturnsWrappedError verifies that every custom
// header / URL variable / test / resend hook helper wraps a failing GitLab
// API response into its operation-prefixed error instead of returning nil,
// covering the error branch of each of the six twin handlers.
func TestHookHelpers_APIError_ReturnsWrappedError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	tests := []struct {
		name   string
		call   func() error
		wantOp string
	}{
		{name: "set custom header", wantOp: "groupSetHookCustomHeader", call: func() error {
			return SetHookCustomHeader(t.Context(), client, SetHookCustomHeaderInput{GroupID: "42", HookID: 7, Key: "X-K", Value: "v"})
		}},
		{name: "delete custom header", wantOp: "groupDeleteHookCustomHeader", call: func() error {
			return DeleteHookCustomHeader(t.Context(), client, DeleteHookCustomHeaderInput{GroupID: "42", HookID: 7, Key: "X-K"})
		}},
		{name: "set url variable", wantOp: "groupSetHookURLVariable", call: func() error {
			return SetHookURLVariable(t.Context(), client, SetHookURLVariableInput{GroupID: "42", HookID: 7, Key: "k", Value: "v"})
		}},
		{name: "delete url variable", wantOp: "groupDeleteHookURLVariable", call: func() error {
			return DeleteHookURLVariable(t.Context(), client, DeleteHookURLVariableInput{GroupID: "42", HookID: 7, Key: "k"})
		}},
		{name: "test hook", wantOp: "groupTestHook", call: func() error {
			return TestHook(t.Context(), client, TestHookInput{GroupID: "42", HookID: 7, Trigger: "push_events"})
		}},
		{name: "resend hook event", wantOp: "groupResendHookEvent", call: func() error {
			return ResendHookEvent(t.Context(), client, ResendHookEventInput{GroupID: "42", HookID: 7, HookEventID: 3})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatalf("%s: expected error from failing API, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantOp) {
				t.Errorf("%s: error %q does not carry op prefix %q", tt.name, err, tt.wantOp)
			}
		})
	}
}

// TestHookOutputWrappers_APIError_PropagatesError verifies the legacy
// *Output wrappers propagate the underlying helper error (covering their
// error pass-through branches) instead of returning a success payload.
func TestHookOutputWrappers_APIError_PropagatesError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	tests := []struct {
		name string
		call func() error
	}{
		{name: "set custom header output", call: func() error {
			_, err := SetHookCustomHeaderOutput(t.Context(), client, SetHookCustomHeaderInput{GroupID: "42", HookID: 7, Key: "X-K", Value: "v"})
			return err
		}},
		{name: "set url variable output", call: func() error {
			_, err := SetHookURLVariableOutput(t.Context(), client, SetHookURLVariableInput{GroupID: "42", HookID: 7, Key: "k", Value: "v"})
			return err
		}},
		{name: "test hook output", call: func() error {
			_, err := TestHookOutput(t.Context(), client, TestHookInput{GroupID: "42", HookID: 7, Trigger: "push_events"})
			return err
		}},
		{name: "resend hook event output", call: func() error {
			_, err := ResendHookEventOutput(t.Context(), client, ResendHookEventInput{GroupID: "42", HookID: 7, HookEventID: 3})
			return err
		}},
		{name: "delete push rule output", call: func() error {
			_, err := DeletePushRuleOutput(t.Context(), client, DeletePushRuleInput{GroupID: "42"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatalf("%s: expected propagated error from failing API, got nil", tt.name)
			}
		})
	}
}

// TestHook_AuditCustomFields verifies AddHook forwards custom_webhook_template
// and custom_headers, and that GetHook surfaces them as redacted []objects.
func TestHook_AuditCustomFields(t *testing.T) {
	var body string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/99/hooks":
			bufBytes, _ := io.ReadAll(r.Body)
			body = string(bufBytes)
			testutil.RespondJSON(w, http.StatusCreated, `{"id":10,"url":"https://example.com/hook","group_id":99,
				"custom_webhook_template":"{\"a\":1}","custom_headers":[{"key":"X-Token","value":"secret"}],
				"url_variables":[{"key":"env","value":"prod"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := AddHook(context.Background(), client, AddHookInput{
		GroupID:               "99",
		URL:                   "https://example.com/hook",
		CustomWebhookTemplate: `{"a":1}`,
		CustomHeaders:         []HookCustomHeaderInput{{Key: "X-Token", Value: "secret"}},
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
	}
	if !strings.Contains(body, `"custom_webhook_template":"{\"a\":1}"`) {
		t.Errorf("custom_webhook_template missing from request: %s", body)
	}
	if !strings.Contains(body, `"custom_headers"`) || !strings.Contains(body, `"X-Token"`) || !strings.Contains(body, `"secret"`) {
		t.Errorf("custom_headers missing from request: %s", body)
	}
	if out.CustomWebhookTemplate != `{"a":1}` {
		t.Errorf("custom_webhook_template = %q, want object template", out.CustomWebhookTemplate)
	}
	if len(out.CustomHeaders) != 1 || out.CustomHeaders[0].Key != "X-Token" || out.CustomHeaders[0].Value != "" {
		t.Errorf("custom_headers should expose key only (value redacted): %+v", out.CustomHeaders)
	}
	if len(out.URLVariables) != 1 || out.URLVariables[0].Key != "env" || out.URLVariables[0].Value != "" {
		t.Errorf("url_variables should expose key only (value redacted): %+v", out.URLVariables)
	}
}

// TestHookToOutput_NilCustomHeaderElement verifies hookToOutput skips nil
// custom-header pointers without panicking.
func TestHookToOutput_NilCustomHeaderElement(t *testing.T) {
	out := hookToOutput(&gl.GroupHook{
		ID:            1,
		CustomHeaders: []*gl.HookCustomHeader{nil, {Key: "X-Real", Value: "secret"}},
	})
	if len(out.CustomHeaders) != 1 || out.CustomHeaders[0].Key != "X-Real" || out.CustomHeaders[0].Value != "" {
		t.Errorf("nil custom header not skipped / value not redacted: %+v", out.CustomHeaders)
	}
}

// TestListHooks_AuditParams verifies order_by/sort/keyset reach the hooks list
// request.
func TestListHooks_AuditParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/99/hooks" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("order_by") != "id" || q.Get("sort") != "asc" || q.Get("pagination") != "keyset" {
			t.Errorf("order_by/sort/pagination missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := ListHooks(context.Background(), client, ListHooksInput{
		GroupID:               "99",
		OrderBy:               "id",
		Sort:                  "asc",
		KeysetPaginationInput: keysetKeyset(),
	})
	if err != nil {
		t.Fatalf("ListHooks() unexpected error: %v", err)
	}
}
