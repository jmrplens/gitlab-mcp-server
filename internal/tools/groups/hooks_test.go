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
var groupHookJSON = `{"id":10,"url":"https://example.com/hook","name":"CI Hook","description":"Triggers CI","group_id":99,"push_events":true,"merge_requests_events":true,"issues_events":false,"tag_push_events":false,"note_events":false,"job_events":false,"pipeline_events":true,"wiki_page_events":false,"deployment_events":false,"releases_events":false,"milestone_events":true,"feature_flag_events":true,"subgroup_events":false,"member_events":false,"vulnerability_events":true,"confidential_issues_events":false,"confidential_note_events":false,"enable_ssl_verification":true,"alert_status":"executable","disabled_until":"2026-01-16T10:00:00Z","url_variables":[{"key":"env","value":"prod"}],"token_present":true,"signing_token_present":true,"created_at":"2026-01-15T10:00:00Z"}`

// groupHookListJSON stores the package-level group hook list JSON state.
var groupHookListJSON = `[` + groupHookJSON + `]`

// ---------------------------------------------------------------------------
// ListHooks tests
// ---------------------------------------------------------------------------.

// TestListHooks_Success verifies ListHooks when success.
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

// TestListHooks_APIError verifies ListHooks when API error.
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

// TestGetHook_Success verifies GetHook when success.
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
}

// TestGetHook_APIError verifies GetHook when API error.
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

// TestAddHook_Success verifies AddHook when success.
func TestAddHook_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupHooks {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	push := true
	out, err := AddHook(context.Background(), client, AddHookInput{
		GroupID: "99",
		HookInput: HookInput{
			URL:                 testHookURL,
			SigningToken:        "signing-secret",
			PushEvents:          &push,
			MilestoneEvents:     &push,
			FeatureFlagEvents:   &push,
			VulnerabilityEvents: &push,
		},
	})
	if err != nil {
		t.Fatalf("AddHook() unexpected error: %v", err)
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

// TestAddHook_APIError verifies AddHook when API error.
func TestAddHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))

	_, err := AddHook(context.Background(), client, AddHookInput{
		GroupID:   "99",
		HookInput: HookInput{URL: "https://bad.example.com"},
	})
	if err == nil {
		t.Fatal("AddHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// EditHook tests
// ---------------------------------------------------------------------------.

// TestEditHook_Success verifies EditHook when success.
func TestEditHook_Success(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathGroupHook10 {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, groupHookJSON)
			return
		}
		http.NotFound(w, r)
	}))

	enabled := true
	out, err := EditHook(context.Background(), client, EditHookInput{
		GroupID: "99",
		HookID:  10,
		HookInput: HookInput{
			URL:                 testHookURL,
			SigningToken:        "new-signing-secret",
			MilestoneEvents:     &enabled,
			FeatureFlagEvents:   &enabled,
			VulnerabilityEvents: &enabled,
		},
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

// TestEditHook_APIError verifies EditHook when API error.
func TestEditHook_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := EditHook(context.Background(), client, EditHookInput{
		GroupID:   "99",
		HookID:    999,
		HookInput: HookInput{URL: "https://example.com"},
	})
	if err == nil {
		t.Fatal("EditHook() expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteHook tests
// ---------------------------------------------------------------------------.

// TestDeleteHook_Success verifies DeleteHook when success.
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

// TestDeleteHook_APIError verifies DeleteHook when API error.
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

// TestGetHook_InvalidHookID verifies GetHook when invalid hook ID.
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

// TestEditHook_InvalidHookID verifies EditHook when invalid hook ID.
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

// TestDeleteHook_InvalidHookID verifies DeleteHook when invalid hook ID.
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

// TestGetHook_URLVariables verifies that URL variables (added in
// gitlab.com/gitlab-org/api/client-go/v2 v2.21.0) are surfaced in the output.
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
