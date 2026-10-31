// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// hooks_test.go contains unit tests for the group MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package groups

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestEditHook_EmptyValues_ReachTheBody holds a group hook's description,
// branch filter and custom template to an empty value removing each. The
// handler used to drop all three and answer with the old values (issue 1306).
func TestEditHook_EmptyValues_ReachTheBody(t *testing.T) {
	input, err := toolutil.UnmarshalParams[EditHookInput](map[string]any{"group_id": "5", "hook_id": 1, "description": "", "push_events_branch_filter": "", "custom_webhook_template": ""})
	if err != nil {
		t.Fatalf("UnmarshalParams() error: %v", err)
	}
	client, recorder := testutil.RecordWrites(t, `{}`, `{"id":1,"url":"https://example.com"}`)
	if _, err = EditHook(t.Context(), client, input); err != nil {
		t.Fatalf("EditHook() error: %v", err)
	}
	recorder.AssertMembers(t, map[string]string{"description": `""`, "push_events_branch_filter": `""`, "custom_webhook_template": `""`})
}

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
		PushEventsBranchFilter:    new("main"),
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %q: %s", want, capturedBody)
			}
		})
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %q: %s", want, capturedBody)
			}
		})
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

// TestFormatHookMarkdown_URLVariablesRedacted verifies the whole card of a
// hook carrying both secrets and both optional status fields: the URL variable
// is named and its value redacted, and nothing else about the hook is lost.
//
// The expectation is the whole response rather than a set of substrings: the
// secret sections are tables, and a table only renders while nothing but rows
// has been written since its header.
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

	want := "## Group Hook: Deploy hook\n\n" +
		"- **ID**: 10\n" +
		"- **URL**: [" + testHookURL + "](" + testHookURL + ")\n" +
		"- **Name**: Deploy hook\n" +
		"- **Description**: Deploy events\n" +
		"- **Group ID**: 99\n" +
		"- **SSL Verification**: ❌\n" +
		"- **Token Present**: ✅\n" +
		"- **Signing Token Present**: ✅\n" +
		"- **Events**: none\n" +
		"- **Alert Status**: executable\n" +
		"- **Disabled Until**: 16 Jan 2026 10:00 UTC\n" +
		"- **Created**: 15 Jan 2026 10:00 UTC\n\n" +
		"### URL Variables\n\n| Key | Value |\n| --- | --- |\n| token | REDACTED |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group.hook_edit' to modify this hook\n" +
		"- Use action 'group.hook_delete' to remove it\n"
	if text != want {
		t.Errorf("hook card:\n got %q\nwant %q", text, want)
	}
}

// TestEnabledEvents_AllEvents verifies the event list of a hook subscribed to
// the fifteen flags this formatter has always named, in the order it writes
// them.
//
// The six the markdown audit (issue 697) added are covered in groups_test.go,
// which asserts the whole list of all twenty-one.
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

	want := "push, tag_push, merge_request, issues, note, job, pipeline, wiki, deployment, releases, " +
		"milestone, feature_flag, subgroup, member, vulnerability"
	if text != want {
		t.Errorf("enabledEvents:\n got %q\nwant %q", text, want)
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
	cases := []struct {
		name string
		in   SetHookCustomHeaderInput
	}{
		{"missing_group_id", SetHookCustomHeaderInput{}},
		{"missing_hook_id", SetHookCustomHeaderInput{GroupID: "99"}},
		{"missing_key", SetHookCustomHeaderInput{GroupID: "99", HookID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := SetHookCustomHeader(context.Background(), client, tc.in); err == nil {
				t.Error("expected error")
			}
		})
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
	cases := []struct {
		name string
		in   DeleteHookCustomHeaderInput
	}{
		{"missing_group_id", DeleteHookCustomHeaderInput{}},
		{"missing_hook_id", DeleteHookCustomHeaderInput{GroupID: "99"}},
		{"missing_key", DeleteHookCustomHeaderInput{GroupID: "99", HookID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := DeleteHookCustomHeader(context.Background(), client, tc.in); err == nil {
				t.Error("expected error")
			}
		})
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
	cases := []struct {
		name string
		in   SetHookURLVariableInput
	}{
		{"missing_group_id", SetHookURLVariableInput{}},
		{"missing_hook_id", SetHookURLVariableInput{GroupID: "99"}},
		{"missing_key", SetHookURLVariableInput{GroupID: "99", HookID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := SetHookURLVariable(context.Background(), client, tc.in); err == nil {
				t.Error("expected error")
			}
		})
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
	cases := []struct {
		name string
		in   DeleteHookURLVariableInput
	}{
		{"missing_group_id", DeleteHookURLVariableInput{}},
		{"missing_hook_id", DeleteHookURLVariableInput{GroupID: "99"}},
		{"missing_key", DeleteHookURLVariableInput{GroupID: "99", HookID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := DeleteHookURLVariable(context.Background(), client, tc.in); err == nil {
				t.Error("expected error")
			}
		})
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
	cases := []struct {
		name string
		in   TestHookInput
	}{
		{"missing_group_id", TestHookInput{}},
		{"missing_hook_id", TestHookInput{GroupID: "99"}},
		{"missing_trigger", TestHookInput{GroupID: "99", HookID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := TestHook(context.Background(), client, tc.in); err == nil {
				t.Error("expected error")
			}
		})
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
	cases := []struct {
		name string
		in   ResendHookEventInput
	}{
		{"missing_group_id", ResendHookEventInput{}},
		{"missing_hook_id", ResendHookEventInput{GroupID: "99"}},
		{"missing_event_id", ResendHookEventInput{GroupID: "99", HookID: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ResendHookEvent(context.Background(), client, tc.in); err == nil {
				t.Error("expected error")
			}
		})
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
		CustomWebhookTemplate: new(`{"a":1}`),
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
	}, toolutil.GroupHookExtra{})
	if len(out.CustomHeaders) != 1 || out.CustomHeaders[0].Key != "X-Real" || out.CustomHeaders[0].Value != "" {
		t.Errorf("nil custom header not skipped / value not redacted: %+v", out.CustomHeaders)
	}
}

// TestListHooks_AuditParams verifies order_by/sort reach the hooks list
// request.
func TestListHooks_AuditParams(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/99/hooks" {
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		if q.Get("order_by") != "id" || q.Get("sort") != "asc" {
			t.Errorf("order_by/sort missing: %s", r.URL.RawQuery)
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	_, err := ListHooks(context.Background(), client, ListHooksInput{
		GroupID: "99",
		OrderBy: "id",
		Sort:    "asc",
	})
	if err != nil {
		t.Fatalf("ListHooks() unexpected error: %v", err)
	}
}

// TestGroupHooks_ZeroHookIDIsRefusedByEveryHandler verifies each handler that
// takes a hook id refuses a zero one before reaching GitLab.
//
// Zero is the id an omitted argument decodes to, and the endpoints would
// answer 404 for it, so the guard is what turns a missing argument into a
// message naming the parameter. The client is a handler that fails the test
// if it is ever called, so a guard that stopped refusing would be caught even
// if GitLab happened to answer.
func TestGroupHooks_ZeroHookIDIsRefusedByEveryHandler(t *testing.T) {
	for name, call := range map[string]func(*gitlabclient.Client) error{
		"get": func(client *gitlabclient.Client) error {
			_, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99"})
			return err
		},
		"set_custom_header": func(client *gitlabclient.Client) error {
			return SetHookCustomHeader(context.Background(), client, SetHookCustomHeaderInput{GroupID: "99", Key: "X-Env", Value: "prod"})
		},
		"set_url_variable": func(client *gitlabclient.Client) error {
			return SetHookURLVariable(context.Background(), client, SetHookURLVariableInput{GroupID: "99", Key: "env", Value: "prod"})
		},
		"delete_url_variable": func(client *gitlabclient.Client) error {
			return DeleteHookURLVariable(context.Background(), client, DeleteHookURLVariableInput{GroupID: "99", Key: "env"})
		},
		"edit": func(client *gitlabclient.Client) error {
			_, err := EditHook(context.Background(), client, EditHookInput{GroupID: "99"})
			return err
		},
		"delete_custom_header": func(client *gitlabclient.Client) error {
			return DeleteHookCustomHeader(context.Background(), client, DeleteHookCustomHeaderInput{GroupID: "99", Key: "X-Env"})
		},
		"test": func(client *gitlabclient.Client) error {
			return TestHook(context.Background(), client, TestHookInput{GroupID: "99", Trigger: "push_events"})
		},
		"resend_event": func(client *gitlabclient.Client) error {
			return ResendHookEvent(context.Background(), client, ResendHookEventInput{GroupID: "99", HookEventID: 3})
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
			if err == nil || !strings.Contains(err.Error(), "hook_id") {
				t.Errorf("%s with a zero hook_id = %v, want an error naming hook_id", name, err)
			}
		})
	}
}

// TestHookToOutput_SecretCarryingListsAreEmptyWhenGitLabSendsNone verifies the
// two lists whose values GitLab masks are surfaced as keys when the hook has
// them and left absent when it does not, rather than as an empty array.
func TestHookToOutput_SecretCarryingListsAreEmptyWhenGitLabSendsNone(t *testing.T) {
	with := hookToOutput(&gl.GroupHook{
		ID:            1,
		URLVariables:  []gl.HookURLVariable{{Key: "env", Value: "prod"}},
		CustomHeaders: []*gl.HookCustomHeader{{Key: "X-Env", Value: "prod"}},
	}, toolutil.GroupHookExtra{})
	if len(with.URLVariables) != 1 || with.URLVariables[0].Key != "env" || with.URLVariables[0].Value != "" {
		t.Errorf("url_variables = %+v, want the key alone", with.URLVariables)
	}
	if len(with.CustomHeaders) != 1 || with.CustomHeaders[0].Key != "X-Env" {
		t.Errorf("custom_headers = %+v, want the key alone", with.CustomHeaders)
	}

	without := hookToOutput(&gl.GroupHook{ID: 1}, toolutil.GroupHookExtra{})
	if without.URLVariables != nil || without.CustomHeaders != nil {
		t.Errorf("output = %+v, want neither list", without)
	}
}

// TestApplyGroupHookIdentityOptions_CarriesEachFieldOnlyWhenGiven verifies the
// identity half of the hook options: a field the input names reaches the SDK
// options, and one it does not leaves the option unset, so an edit that
// mentions nothing changes nothing.
func TestApplyGroupHookIdentityOptions_CarriesEachFieldOnlyWhenGiven(t *testing.T) {
	full := &gl.AddGroupHookOptions{}
	applyGroupHookIdentityOptions(HookInput{
		URL: "https://example.com/hook", Name: "Named", Description: new("why"),
		Token: "s3cret", EnableSSLVerification: new(true),
	}, full)
	if full.URL == nil || full.Name == nil || full.Description == nil || full.Token == nil || full.EnableSSLVerification == nil {
		t.Errorf("options = %+v, want every named field set", full)
	}

	bare := &gl.AddGroupHookOptions{}
	applyGroupHookIdentityOptions(HookInput{}, bare)
	if bare.URL != nil || bare.Name != nil || bare.Description != nil || bare.Token != nil || bare.EnableSSLVerification != nil {
		t.Errorf("options = %+v, want nothing set from an empty input", bare)
	}
}

// ---------------------------------------------------------------------------
// Every field the hook converter copies, and every flag the writes send
// ---------------------------------------------------------------------------.

// getDistinctHook answers a hook request with body and returns what GetHook
// published for it.
func getDistinctHook(t *testing.T, body string) HookOutput {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathGroupHook10 {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	out, err := GetHook(context.Background(), client, GetHookInput{GroupID: "99", HookID: 10})
	if err != nil {
		t.Fatalf("GetHook() unexpected error: %v", err)
	}
	return out
}

// TestGetHook_PublishesEachFieldGitLabSent verifies the whole hook a
// single-hook route answers with, against an answer in which no two values
// agree, the two secret-bearing lists reduced to their keys. The flags are
// left to the one-at-a-time test below.
func TestGetHook_PublishesEachFieldGitLabSent(t *testing.T) {
	out := getDistinctHook(t, `{"id":10,"url":"https://ci.example.com/hook","name":"CI Hook","description":"Triggers CI",`+
		`"group_id":99,"alert_status":"temporarily_disabled","disabled_until":"2026-01-16T10:00:00Z",`+
		`"created_at":"2026-01-15T10:00:00Z","push_events_branch_filter":"release/*","branch_filter_strategy":"wildcard",`+
		`"custom_webhook_template":"{\"a\":1}","url_variables":[{"key":"env","value":"prod"}],`+
		`"custom_headers":[{"key":"X-Env","value":"prod"}]}`)
	want := HookOutput{
		ID: 10, URL: "https://ci.example.com/hook", Name: "CI Hook", Description: "Triggers CI", GroupID: 99,
		AlertStatus: "temporarily_disabled", DisabledUntil: "2026-01-16T10:00:00Z", CreatedAt: "2026-01-15T10:00:00Z",
		PushEventsBranchFilter: "release/*", BranchFilterStrategy: "wildcard", CustomWebhookTemplate: `{"a":1}`,
		URLVariables:  []HookURLVariable{{Key: "env"}},
		CustomHeaders: []HookCustomHeaderOutput{{Key: "X-Env"}},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("GetHook() published\n%+v\nwant\n%+v", out, want)
	}
}

// TestGetHook_PublishesEachFlagOnItsOwn verifies each of the twenty-four
// booleans a hook carries lands on the field named after it when sent alone,
// the one read off the capture included.
//
// The hook fixture sets twelve flags true and the rest false, so a converter
// reading a neighbor's flag passed whenever the two happened to agree, which
// is about half of all pairs.
func TestGetHook_PublishesEachFlagOnItsOwn(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want HookOutput
	}{
		{key: "push_events", want: HookOutput{ID: 10, PushEvents: true}},
		{key: "tag_push_events", want: HookOutput{ID: 10, TagPushEvents: true}},
		{key: "merge_requests_events", want: HookOutput{ID: 10, MergeRequestsEvents: true}},
		{key: "issues_events", want: HookOutput{ID: 10, IssuesEvents: true}},
		{key: "note_events", want: HookOutput{ID: 10, NoteEvents: true}},
		{key: "job_events", want: HookOutput{ID: 10, JobEvents: true}},
		{key: "pipeline_events", want: HookOutput{ID: 10, PipelineEvents: true}},
		{key: "wiki_page_events", want: HookOutput{ID: 10, WikiPageEvents: true}},
		{key: "deployment_events", want: HookOutput{ID: 10, DeploymentEvents: true}},
		{key: "releases_events", want: HookOutput{ID: 10, ReleasesEvents: true}},
		{key: "subgroup_events", want: HookOutput{ID: 10, SubGroupEvents: true}},
		{key: "member_events", want: HookOutput{ID: 10, MemberEvents: true}},
		{key: "confidential_issues_events", want: HookOutput{ID: 10, ConfidentialIssuesEvents: true}},
		{key: "confidential_note_events", want: HookOutput{ID: 10, ConfidentialNoteEvents: true}},
		{key: "enable_ssl_verification", want: HookOutput{ID: 10, EnableSSLVerification: true}},
		{key: "feature_flag_events", want: HookOutput{ID: 10, FeatureFlagEvents: true}},
		{key: "milestone_events", want: HookOutput{ID: 10, MilestoneEvents: true}},
		{key: "vulnerability_events", want: HookOutput{ID: 10, VulnerabilityEvents: true}},
		{key: "emoji_events", want: HookOutput{ID: 10, EmojiEvents: true}},
		{key: "resource_access_token_events", want: HookOutput{ID: 10, ResourceAccessTokenEvents: true}},
		{key: "project_events", want: HookOutput{ID: 10, ProjectEvents: true}},
		{key: "token_present", want: HookOutput{ID: 10, TokenPresent: true}},
		{key: "signing_token_present", want: HookOutput{ID: 10, SigningTokenPresent: true}},
		{key: "repository_update_events", want: HookOutput{ID: 10, RepositoryUpdateEvents: true}},
		{key: "duo_flow_callback_enabled", want: HookOutput{ID: 10, DuoFlowCallbackEnabled: new(true)}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			out := getDistinctHook(t, `{"id":10,"`+tc.key+`":true}`)
			if !reflect.DeepEqual(out, tc.want) {
				t.Errorf("GetHook() published %+v, want %+v", out, tc.want)
			}
		})
	}
}

// TestGetHook_DuoFlowCallbackEnabled_TellsOffFromNotSent verifies the Duo
// flow callback setting keeps the difference between a hook with it off,
// which GitLab 19.4 and later sends as false, and an answer from an older
// instance that sends no key at all: the first is published as false and the
// second is left out.
func TestGetHook_DuoFlowCallbackEnabled_TellsOffFromNotSent(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want *bool
	}{
		{name: "sent off", body: `{"id":10,"duo_flow_callback_enabled":false}`, want: new(false)},
		{name: "not sent", body: `{"id":10}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := getDistinctHook(t, tc.body)
			if !reflect.DeepEqual(out.DuoFlowCallbackEnabled, tc.want) {
				t.Errorf("DuoFlowCallbackEnabled = %v, want %v", out.DuoFlowCallbackEnabled, tc.want)
			}
		})
	}
}

// hookEventFlags is every event flag a hook write takes, keyed by the name
// GitLab takes it under, each input naming only that flag.
func hookEventFlags() map[string]HookInput {
	return map[string]HookInput{
		"push_events":                  {PushEvents: new(true)},
		"tag_push_events":              {TagPushEvents: new(true)},
		"merge_requests_events":        {MergeRequestsEvents: new(true)},
		"issues_events":                {IssuesEvents: new(true)},
		"note_events":                  {NoteEvents: new(true)},
		"job_events":                   {JobEvents: new(true)},
		"pipeline_events":              {PipelineEvents: new(true)},
		"wiki_page_events":             {WikiPageEvents: new(true)},
		"deployment_events":            {DeploymentEvents: new(true)},
		"releases_events":              {ReleasesEvents: new(true)},
		"milestone_events":             {MilestoneEvents: new(true)},
		"feature_flag_events":          {FeatureFlagEvents: new(true)},
		"subgroup_events":              {SubGroupEvents: new(true)},
		"member_events":                {MemberEvents: new(true)},
		"vulnerability_events":         {VulnerabilityEvents: new(true)},
		"confidential_issues_events":   {ConfidentialIssuesEvents: new(true)},
		"confidential_note_events":     {ConfidentialNoteEvents: new(true)},
		"emoji_events":                 {EmojiEvents: new(true)},
		"resource_access_token_events": {ResourceAccessTokenEvents: new(true)},
		"project_events":               {ProjectEvents: new(true)},
		"enable_ssl_verification":      {EnableSSLVerification: new(true)},
	}
}

// TestHookWrites_EachFlagReachesTheRequestOnItsOwn verifies each event flag,
// named alone, produces an add body of the URL and exactly that flag, and an
// edit body of exactly that flag: one flag written under a neighbor's key
// cannot pass a body held to one key.
//
// The add options pair each input flag with its destination in a table, and
// the edit options are copied field by field off the add options, so there
// are two places a pair can cross, and the earlier tests sent seven flags true
// at once and searched the body for their names.
func TestHookWrites_EachFlagReachesTheRequestOnItsOwn(t *testing.T) {
	for key, input := range hookEventFlags() {
		t.Run("add/"+key, func(t *testing.T) {
			body := recordedJSONBody(t, http.StatusCreated, groupHookJSON, func(client *gitlabclient.Client) error {
				_, err := AddHook(context.Background(), client, AddHookInput{GroupID: "99", URL: testHookURL, HookInput: input})
				return err
			})
			want := map[string]any{"url": testHookURL, key: true}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("add body = %v, want %v", body, want)
			}
		})
		t.Run("edit/"+key, func(t *testing.T) {
			body := recordedJSONBody(t, http.StatusOK, groupHookJSON, func(client *gitlabclient.Client) error {
				_, err := EditHook(context.Background(), client, EditHookInput{GroupID: "99", HookID: 10, HookInput: input})
				return err
			})
			want := map[string]any{key: true}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("edit body = %v, want %v", body, want)
			}
		})
	}
}

// TestEditHook_CarriesEveryIdentityFieldUnderItsOwnKey verifies the whole edit
// body for an input naming every non-flag field with a value of its own, since
// the edit options are copied off the add options one field at a time and the
// two tokens, the two filters and the two texts are each of a kind a copy
// could cross.
func TestEditHook_CarriesEveryIdentityFieldUnderItsOwnKey(t *testing.T) {
	body := recordedJSONBody(t, http.StatusOK, groupHookJSON, func(client *gitlabclient.Client) error {
		_, err := EditHook(context.Background(), client, EditHookInput{
			GroupID: "99", HookID: 10,
			URL: "https://ci.example.com/hook", Name: "CI Hook", Description: new("Triggers CI"),
			Token: "shared-secret", SigningToken: "signing-secret",
			PushEventsBranchFilter: new("release/*"), BranchFilterStrategy: "wildcard",
			CustomWebhookTemplate: new(`{"a":1}`),
			CustomHeaders:         []HookCustomHeaderInput{{Key: "X-Env", Value: "prod"}},
		})
		return err
	})
	want := map[string]any{
		"url": "https://ci.example.com/hook", "name": "CI Hook", "description": "Triggers CI",
		"token": "shared-secret", "signing_token": "signing-secret",
		"push_events_branch_filter": "release/*", "branch_filter_strategy": "wildcard",
		"custom_webhook_template": `{"a":1}`,
		"custom_headers":          []any{map[string]any{"key": "X-Env", "value": "prod"}},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("edit body = %v, want %v", body, want)
	}
}

// pathArgCases are the values the hook sub-operation tests below pass as the
// argument client-go places into the request path without escaping it: the
// forms with a "." or ".." segment, which the handler refuses, each with the
// segment the refusal must report, and a value with a dot inside a segment,
// which it sends and which has none.
var pathArgCases = []struct {
	name    string
	value   string
	segment string
}{
	{"dot dot between names", "a/../b", `".."`},
	{"bare dot dot", "..", `".."`},
	{"bare dot", ".", `"."`},
	{"encoded dot dot", "%2e%2e", `".." once its percent-encoding is undone`},
	{"dot between names", "x/./y", `"."`},
	{"dot inside a segment", "v1.2", ""},
}

// assertPathArgOutcome checks what a handler did with one of [pathArgCases]:
// a value with a segment to report returns the whole refusal of field under
// operation naming that segment and sends nothing, and any other value is
// sent once.
func assertPathArgOutcome(t *testing.T, err error, operation, field, segment string, requests int) {
	t.Helper()
	if segment == "" {
		if err != nil {
			t.Fatalf("%s with a %s that has a dot inside a segment = %v, want it sent", operation, field, err)
		}
		if requests != 1 {
			t.Errorf("%s sent %d requests, want 1", operation, requests)
		}
		return
	}
	if err == nil {
		t.Fatalf("%s accepted a %s with a dot segment, want a refusal", operation, field)
	}
	want := operation + ": " + field + " has the segment " + segment +
		`; pass a value with no "." or ".." segment, written plainly or percent-encoded`
	if got := err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if requests != 0 {
		t.Errorf("%s sent %d requests for a refused %s, want none", operation, requests, field)
	}
}

// TestSetHookCustomHeader_DotSegmentKey_IsRefused asserts that a custom header
// key with a "." or ".." segment is refused before any request is sent, with
// an error that names key, and that a key with a dot inside a segment is sent.
// client-go places the key into the request path without escaping it.
func TestSetHookCustomHeader_DotSegmentKey_IsRefused(t *testing.T) {
	for _, tc := range pathArgCases {
		t.Run(tc.name, func(t *testing.T) {
			client, recorder := testutil.RecordQueries(t, `{}`)
			err := SetHookCustomHeader(t.Context(), client, SetHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: tc.value, Value: "v"})
			assertPathArgOutcome(t, err, "groupSetHookCustomHeader", "key", tc.segment, recorder.Requests())
		})
	}
}

// TestDeleteHookCustomHeader_DotSegmentKey_IsRefused asserts the same check
// on the custom header delete route, which also places the key into the
// request path without escaping it.
func TestDeleteHookCustomHeader_DotSegmentKey_IsRefused(t *testing.T) {
	for _, tc := range pathArgCases {
		t.Run(tc.name, func(t *testing.T) {
			client, recorder := testutil.RecordQueries(t, `{}`)
			err := DeleteHookCustomHeader(t.Context(), client, DeleteHookCustomHeaderInput{GroupID: "99", HookID: 10, Key: tc.value})
			assertPathArgOutcome(t, err, "groupDeleteHookCustomHeader", "key", tc.segment, recorder.Requests())
		})
	}
}

// TestSetHookURLVariable_DotSegmentKey_IsRefused asserts that a URL variable
// key with a "." or ".." segment is refused before any request is sent, with
// an error that names key, and that a key with a dot inside a segment is sent.
// client-go places the key into the request path without escaping it.
func TestSetHookURLVariable_DotSegmentKey_IsRefused(t *testing.T) {
	for _, tc := range pathArgCases {
		t.Run(tc.name, func(t *testing.T) {
			client, recorder := testutil.RecordQueries(t, `{}`)
			err := SetHookURLVariable(t.Context(), client, SetHookURLVariableInput{GroupID: "99", HookID: 10, Key: tc.value, Value: "v"})
			assertPathArgOutcome(t, err, "groupSetHookURLVariable", "key", tc.segment, recorder.Requests())
		})
	}
}

// TestDeleteHookURLVariable_DotSegmentKey_IsRefused asserts the same check on
// the URL variable delete route, which also places the key into the request
// path without escaping it.
func TestDeleteHookURLVariable_DotSegmentKey_IsRefused(t *testing.T) {
	for _, tc := range pathArgCases {
		t.Run(tc.name, func(t *testing.T) {
			client, recorder := testutil.RecordQueries(t, `{}`)
			err := DeleteHookURLVariable(t.Context(), client, DeleteHookURLVariableInput{GroupID: "99", HookID: 10, Key: tc.value})
			assertPathArgOutcome(t, err, "groupDeleteHookURLVariable", "key", tc.segment, recorder.Requests())
		})
	}
}

// TestTestHook_DotSegmentTrigger_IsRefused asserts that a test trigger with a
// "." or ".." segment is refused before any request is sent, with an error
// that names trigger, and that a trigger with a dot inside a segment is sent.
// client-go places the trigger into the request path without escaping it.
func TestTestHook_DotSegmentTrigger_IsRefused(t *testing.T) {
	for _, tc := range pathArgCases {
		t.Run(tc.name, func(t *testing.T) {
			client, recorder := testutil.RecordQueries(t, `{}`)
			err := TestHook(t.Context(), client, TestHookInput{GroupID: "99", HookID: 10, Trigger: tc.value})
			assertPathArgOutcome(t, err, "groupTestHook", "trigger", tc.segment, recorder.Requests())
		})
	}
}

// segmentEncodings are values with a byte a request path segment cannot
// carry as it is, each with the exact form it must reach the path in:
// percent-encoded as one segment, a "/" included. The last case has no such
// byte and must reach the path unchanged.
var segmentEncodings = []struct {
	name    string
	value   string
	escaped string
}{
	{"a space", "my key", "my%20key"},
	{"a non-ASCII byte", "clé", "cl%C3%A9"},
	{"a slash", "a/b", "a%2Fb"},
	{"a percent sign", "100%", "100%25"},
	{"no byte to escape", "X_Token", "X_Token"},
}

// TestHookSubOperations_PathArgument_IsEscapedAsOneSegment asserts the exact
// path each hook sub-operation is requested at: the key or trigger is
// percent-encoded as one path segment, a "/" in it included, the full-path
// group_id stays one escaped segment, and a value with no byte to escape is
// sent unchanged. client-go places the key and the trigger into the request
// path as given, so each handler escapes it first.
func TestHookSubOperations_PathArgument_IsEscapedAsOneSegment(t *testing.T) {
	const hook = "/api/v4/groups/group%2Fsub/hooks/10/"
	operations := []struct {
		name  string
		route string
		call  func(ctx context.Context, client *gitlabclient.Client, value string) error
	}{
		{"set custom header", "custom_headers/", func(ctx context.Context, client *gitlabclient.Client, value string) error {
			return SetHookCustomHeader(ctx, client, SetHookCustomHeaderInput{GroupID: "group/sub", HookID: 10, Key: value, Value: "v"})
		}},
		{"delete custom header", "custom_headers/", func(ctx context.Context, client *gitlabclient.Client, value string) error {
			return DeleteHookCustomHeader(ctx, client, DeleteHookCustomHeaderInput{GroupID: "group/sub", HookID: 10, Key: value})
		}},
		{"set URL variable", "url_variables/", func(ctx context.Context, client *gitlabclient.Client, value string) error {
			return SetHookURLVariable(ctx, client, SetHookURLVariableInput{GroupID: "group/sub", HookID: 10, Key: value, Value: "v"})
		}},
		{"delete URL variable", "url_variables/", func(ctx context.Context, client *gitlabclient.Client, value string) error {
			return DeleteHookURLVariable(ctx, client, DeleteHookURLVariableInput{GroupID: "group/sub", HookID: 10, Key: value})
		}},
		{"test trigger", "test/", func(ctx context.Context, client *gitlabclient.Client, value string) error {
			return TestHook(ctx, client, TestHookInput{GroupID: "group/sub", HookID: 10, Trigger: value})
		}},
	}
	for _, op := range operations {
		for _, tc := range segmentEncodings {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				client, recorder := testutil.RecordQueries(t, `{}`)
				if err := op.call(t.Context(), client, tc.value); err != nil {
					t.Fatalf("%s(%q) = %v, want it sent", op.name, tc.value, err)
				}
				if recorder.Requests() != 1 {
					t.Fatalf("%s sent %d requests, want 1", op.name, recorder.Requests())
				}
				if got, want := recorder.Path(), hook+op.route+tc.escaped; got != want {
					t.Errorf("request path = %q, want %q", got, want)
				}
			})
		}
	}
}
