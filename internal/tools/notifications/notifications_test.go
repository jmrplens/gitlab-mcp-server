// notifications_test.go contains unit tests for the notification settings MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package notifications

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const fmtUnexpPath = "unexpected path: %s"

const errNoReachAPI = "should not reach API"

const fmtUnexpErr = "unexpected error: %v"

// TestGetGlobalSettings_Success verifies that GetGlobalSettings handles the success scenario correctly.
func TestGetGlobalSettings_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/notification_settings" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"level":"participating","notification_email":"test@example.com","events":{"close_issue":true,"new_issue":false}}`)
	}))

	out, err := GetGlobalSettings(t.Context(), client, GetGlobalInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Level != "participating" {
		t.Errorf("level = %q, want participating", out.Level)
	}
	if out.NotificationEmail != "test@example.com" {
		t.Errorf("email = %q, want test@example.com", out.NotificationEmail)
	}
	if out.Events == nil {
		t.Fatal("expected events to be non-nil")
	}
	if !out.Events.CloseIssue {
		t.Error("expected close_issue to be true")
	}
}

// TestGetSettingsForProject_Success verifies that GetSettingsForProject handles the success scenario correctly.
func TestGetSettingsForProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/my-project/notification_settings" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"level":"watch","notification_email":"","events":null}`)
	}))

	out, err := GetSettingsForProject(t.Context(), client, GetProjectInput{ProjectID: "my-project"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Level != "watch" {
		t.Errorf("level = %q, want watch", out.Level)
	}
}

// TestGetSettingsForProject_ValidationError verifies that GetSettingsForProject handles the validation error scenario correctly.
func TestGetSettingsForProject_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetSettingsForProject(t.Context(), client, GetProjectInput{ProjectID: ""})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestGetSettingsForGroup_Success verifies that GetSettingsForGroup handles the success scenario correctly.
func TestGetSettingsForGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/my-group/notification_settings" {
			t.Errorf(fmtUnexpPath, r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"level":"global","notification_email":""}`)
	}))

	out, err := GetSettingsForGroup(t.Context(), client, GetGroupInput{GroupID: "my-group"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Level != "global" {
		t.Errorf("level = %q, want global", out.Level)
	}
}

// TestGetSettingsForGroup_ValidationError verifies that GetSettingsForGroup handles the validation error scenario correctly.
func TestGetSettingsForGroup_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetSettingsForGroup(t.Context(), client, GetGroupInput{GroupID: ""})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestUpdateGlobalSettings_Success verifies that UpdateGlobalSettings handles the success scenario correctly.
func TestUpdateGlobalSettings_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		testutil.RespondJSON(w, http.StatusOK, `{"level":"custom","notification_email":"new@example.com","events":{"new_issue":true}}`)
	}))

	tr := true
	out, err := UpdateGlobalSettings(t.Context(), client, UpdateGlobalInput{
		eventFields: eventFields{Level: "custom", NewIssue: &tr},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Level != "custom" {
		t.Errorf("level = %q, want custom", out.Level)
	}
}

// TestUpdateSettingsForProject_ValidationError verifies that UpdateSettingsForProject handles the validation error scenario correctly.
func TestUpdateSettingsForProject_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := UpdateSettingsForProject(t.Context(), client, UpdateProjectInput{ProjectID: ""})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestUpdateSettingsForGroup_APIError verifies that UpdateSettingsForGroup handles the a p i error scenario correctly.
func TestUpdateSettingsForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))

	_, err := UpdateSettingsForGroup(t.Context(), client, UpdateGroupInput{GroupID: "my-group"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

// Formatter tests.

// TestFormatMarkdownString_WithEvents verifies that FormatMarkdownString handles the with events scenario correctly.
func TestFormatMarkdownString_WithEvents(t *testing.T) {
	out := Output{
		Level:             "custom",
		NotificationEmail: "test@example.com",
		Events: &EventOutput{
			CloseIssue: true,
			NewIssue:   true,
		},
	}
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "custom") {
		t.Error("expected level in markdown")
	}
	if !strings.Contains(md, "test@example.com") {
		t.Error("expected email in markdown")
	}
	if !strings.Contains(md, "Custom Events") {
		t.Error("expected custom events section")
	}
	if !strings.Contains(md, "✅ Close Issue") {
		t.Error("expected close_issue enabled")
	}
}

// TestFormatMarkdownString_NoEvents verifies that FormatMarkdownString handles the no events scenario correctly.
func TestFormatMarkdownString_NoEvents(t *testing.T) {
	out := Output{Level: "watch"}
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "watch") {
		t.Error("expected level in markdown")
	}
	if strings.Contains(md, "Custom Events") {
		t.Error("should not have custom events section")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

const errExpectedErr = "expected error"

const covSettingsJSON = `{"level":"participating","notification_email":"test@example.com","events":{"close_issue":true,"new_issue":false,"close_merge_request":false,"failed_pipeline":false,"fixed_pipeline":false,"issue_due":false,"merge_merge_request":false,"merge_when_pipeline_succeeds":false,"moved_project":false,"new_epic":false,"new_merge_request":false,"new_note":false,"push_to_merge_request":false,"reassign_issue":false,"reassign_merge_request":false,"reopen_issue":false,"reopen_merge_request":false,"success_pipeline":false}}`

// API error tests.

// TestGetGlobalSettings_APIError verifies the behavior of cov get global settings a p i error.
func TestGetGlobalSettings_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := GetGlobalSettings(t.Context(), client, GetGlobalInput{})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetSettingsForProject_APIError verifies the behavior of cov get settings for project a p i error.
func TestGetSettingsForProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := GetSettingsForProject(t.Context(), client, GetProjectInput{ProjectID: "proj"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetSettingsForGroup_APIError verifies the behavior of cov get settings for group a p i error.
func TestGetSettingsForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := GetSettingsForGroup(t.Context(), client, GetGroupInput{GroupID: "grp"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateGlobalSettings_APIError verifies the behavior of cov update global settings a p i error.
func TestUpdateGlobalSettings_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := UpdateGlobalSettings(t.Context(), client, UpdateGlobalInput{eventFields: eventFields{Level: "watch"}})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateSettingsForProject_Success verifies the behavior of cov update settings for project success.
func TestUpdateSettingsForProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	tr := true
	out, err := UpdateSettingsForProject(t.Context(), client, UpdateProjectInput{
		ProjectID: "proj", eventFields: eventFields{Level: "custom", NewIssue: &tr},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Level != "participating" {
		t.Errorf("level = %q", out.Level)
	}
}

// TestUpdateSettingsForProject_APIError verifies the behavior of cov update settings for project a p i error.
func TestUpdateSettingsForProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := UpdateSettingsForProject(t.Context(), client, UpdateProjectInput{ProjectID: "proj"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateSettingsForGroup_Success verifies the behavior of cov update settings for group success.
func TestUpdateSettingsForGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	out, err := UpdateSettingsForGroup(t.Context(), client, UpdateGroupInput{GroupID: "grp", eventFields: eventFields{Level: "watch"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Level != "participating" {
		t.Errorf("level = %q", out.Level)
	}
}

// TestUpdateSettingsForGroup_ValidationError verifies the behavior of cov update settings for group validation error.
func TestUpdateSettingsForGroup_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API")
	}))
	_, err := UpdateSettingsForGroup(t.Context(), client, UpdateGroupInput{GroupID: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// buildUpdateOpts coverage.

// TestBuildUpdateOpts_AllBooleans verifies the behavior of cov build update opts all booleans.
func TestBuildUpdateOpts_AllBooleans(t *testing.T) {
	tr := true
	fa := false
	opts := buildUpdateOpts(eventFields{
		Level: "custom", NotificationEmail: "email@test.com",
		CloseIssue: &tr, CloseMergeRequest: &fa, FailedPipeline: &tr, FixedPipeline: &fa,
		IssueDue: &tr, MergeMergeRequest: &fa, MergeWhenPipelineSucceeds: &tr, MovedProject: &fa,
		NewEpic: &tr, NewIssue: &fa, NewMergeRequest: &tr, NewNote: &fa,
		PushToMergeRequest: &tr, ReassignIssue: &fa, ReassignMergeRequest: &tr, ReopenIssue: &fa,
		ReopenMergeRequest: &tr, SuccessPipeline: &fa,
	})
	if opts.CloseIssue == nil || *opts.CloseIssue != true {
		t.Error("CloseIssue should be true")
	}
	if opts.CloseMergeRequest == nil || *opts.CloseMergeRequest != false {
		t.Error("CloseMergeRequest should be false")
	}
	if opts.NotificationEmail == nil || *opts.NotificationEmail != "email@test.com" {
		t.Error("email should be set")
	}
}

// TestBuildUpdateOpts_UnknownLevel verifies the behavior of cov build update opts unknown level.
func TestBuildUpdateOpts_UnknownLevel(t *testing.T) {
	opts := buildUpdateOpts(eventFields{Level: "unknown_level"})
	if opts.Level != nil {
		t.Error("unknown level should not set Level")
	}
}

// TestBuildUpdateOpts_EmptyLevel verifies the behavior of cov build update opts empty level.
func TestBuildUpdateOpts_EmptyLevel(t *testing.T) {
	opts := buildUpdateOpts(eventFields{})
	if opts.Level != nil {
		t.Error("empty level should not set Level")
	}
}

// TestBuildUpdateOpts_ValidLevels verifies the behavior of cov build update opts valid levels.
func TestBuildUpdateOpts_ValidLevels(t *testing.T) {
	for _, lv := range []string{"disabled", "participating", "watch", "global", "mention", "custom"} {
		t.Run(lv, func(t *testing.T) {
			opts := buildUpdateOpts(eventFields{Level: lv})
			if opts.Level == nil {
				t.Errorf("level %q should set Level", lv)
			}
		})
	}
}

// FormatMarkdown wrapper.

// TestFormatMarkdown_Wrapper verifies the behavior of cov format markdown wrapper.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	result := FormatMarkdown(Output{Level: "watch"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// eventLine.

// TestEventLine_Enabled verifies the behavior of cov event line enabled.
func TestEventLine_Enabled(t *testing.T) {
	line := eventLine("Test Event", true)
	if !strings.Contains(line, "✅") {
		t.Error("expected checkmark for enabled")
	}
	if !strings.Contains(line, "Test Event") {
		t.Error("expected event name")
	}
}

// TestEventLine_Disabled verifies the behavior of cov event line disabled.
func TestEventLine_Disabled(t *testing.T) {
	line := eventLine("Test Event", false)
	if !strings.Contains(line, "❌") {
		t.Error("expected cross for disabled")
	}
}

// FormatMarkdownString edge cases.

// TestFormatMarkdownString_NoEmail verifies the behavior of cov format markdown string no email.
func TestFormatMarkdownString_NoEmail(t *testing.T) {
	out := Output{Level: "watch"}
	md := FormatMarkdownString(out)
	if strings.Contains(md, "Email") {
		t.Error("should not show Email for empty notification_email")
	}
	if !strings.Contains(md, "watch") {
		t.Error("expected level in markdown")
	}
}

// TestFormatMarkdownString_AllEvents verifies the behavior of cov format markdown string all events.
func TestFormatMarkdownString_AllEvents(t *testing.T) {
	out := Output{
		Level:             "custom",
		NotificationEmail: "a@b.com",
		Events: &EventOutput{
			CloseIssue:                true,
			CloseMergeRequest:         true,
			FailedPipeline:            true,
			FixedPipeline:             false,
			IssueDue:                  true,
			MergeMergeRequest:         true,
			MergeWhenPipelineSucceeds: false,
			MovedProject:              true,
			NewIssue:                  true,
			NewMergeRequest:           false,
			NewEpic:                   true,
			NewNote:                   true,
			PushToMergeRequest:        false,
			ReassignIssue:             true,
			ReassignMergeRequest:      false,
			ReopenIssue:               true,
			ReopenMergeRequest:        true,
			SuccessPipeline:           false,
		},
	}
	md := FormatMarkdownString(out)
	if !strings.Contains(md, "Custom Events") {
		t.Error("expected Custom Events section")
	}
	if !strings.Contains(md, "a@b.com") {
		t.Error("expected email")
	}
}

// RegisterTools / RegisterMeta no-panic.

// TestRegisterTools_NoPanic verifies the behavior of cov register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	RegisterTools(server, client)
}

// TestRegisterMeta_NoPanic verifies the behavior of cov register meta no panic.
func TestRegisterMeta_NoPanic(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	RegisterMeta(server, client)
}

// MCP round-trip for all 6 tools.

// TestMCPRound_Trip validates cov m c p round trip across multiple scenarios using table-driven subtests.
func TestMCPRound_Trip(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	RegisterTools(server, client)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	go server.Connect(ctx, st, nil)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	tests := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_notification_global_get", map[string]any{}},
		{"gitlab_notification_project_get", map[string]any{"project_id": "proj"}},
		{"gitlab_notification_group_get", map[string]any{"group_id": "grp"}},
		{"gitlab_notification_global_update", map[string]any{"level": "watch"}},
		{"gitlab_notification_project_update", map[string]any{"project_id": "proj", "level": "watch"}},
		{"gitlab_notification_group_update", map[string]any{"group_id": "grp", "level": "watch"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var res *mcp.CallToolResult
			res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
		})
	}
}
