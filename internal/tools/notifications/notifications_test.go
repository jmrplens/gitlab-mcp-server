// notifications_test.go contains unit tests for the notification settings MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package notifications

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// fmtUnexpPath identifies the fmt unexp path constant used by this package.
const fmtUnexpPath = "unexpected path: %s"

// errNoReachAPI identifies the err no reach API constant used by this package.
const errNoReachAPI = "should not reach API"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// TestGetGlobalSettings_Success verifies GetGlobalSettings when success.
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

// TestGetSettingsForProject_Success verifies GetSettingsForProject when success.
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

// TestGetSettingsForProject_ValidationError verifies GetSettingsForProject when validation error.
func TestGetSettingsForProject_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetSettingsForProject(t.Context(), client, GetProjectInput{ProjectID: ""})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestGetSettingsForGroup_Success verifies GetSettingsForGroup when success.
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

// TestGetSettingsForGroup_ValidationError verifies GetSettingsForGroup when validation error.
func TestGetSettingsForGroup_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := GetSettingsForGroup(t.Context(), client, GetGroupInput{GroupID: ""})
	if err == nil {
		t.Fatal("expected error for empty group_id")
	}
}

// TestUpdateGlobalSettings_Success verifies UpdateGlobalSettings when success.
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

// TestUpdateSettingsForProject_ValidationError verifies UpdateSettingsForProject when validation error.
func TestUpdateSettingsForProject_ValidationError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal(errNoReachAPI)
	}))

	_, err := UpdateSettingsForProject(t.Context(), client, UpdateProjectInput{ProjectID: ""})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestUpdateSettingsForGroup_APIError verifies UpdateSettingsForGroup when API error.
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

// TestFormatMarkdownString_WithEvents verifies FormatMarkdownString when with events.
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

// TestFormatMarkdownString_NoEvents verifies FormatMarkdownString when no events.
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

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// covSettingsJSON identifies the cov settings JSON constant used by this package.
const covSettingsJSON = `{"level":"participating","notification_email":"test@example.com","events":{"close_issue":true,"new_issue":false,"close_merge_request":false,"failed_pipeline":false,"fixed_pipeline":false,"issue_due":false,"merge_merge_request":false,"merge_when_pipeline_succeeds":false,"moved_project":false,"new_epic":false,"new_merge_request":false,"new_note":false,"push_to_merge_request":false,"reassign_issue":false,"reassign_merge_request":false,"reopen_issue":false,"reopen_merge_request":false,"success_pipeline":false}}`

// API error tests.

// TestGetGlobalSettings_APIError verifies GetGlobalSettings when API error.
func TestGetGlobalSettings_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := GetGlobalSettings(t.Context(), client, GetGlobalInput{})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetSettingsForProject_APIError verifies GetSettingsForProject when API error.
func TestGetSettingsForProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := GetSettingsForProject(t.Context(), client, GetProjectInput{ProjectID: "proj"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestGetSettingsForGroup_APIError verifies GetSettingsForGroup when API error.
func TestGetSettingsForGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := GetSettingsForGroup(t.Context(), client, GetGroupInput{GroupID: "grp"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateGlobalSettings_APIError verifies UpdateGlobalSettings when API error.
func TestUpdateGlobalSettings_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := UpdateGlobalSettings(t.Context(), client, UpdateGlobalInput{eventFields: eventFields{Level: "watch"}})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateSettingsForProject_Success verifies UpdateSettingsForProject when success.
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

// TestUpdateSettingsForProject_APIError verifies UpdateSettingsForProject when API error.
func TestUpdateSettingsForProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := UpdateSettingsForProject(t.Context(), client, UpdateProjectInput{ProjectID: "proj"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestUpdateSettingsForGroup_Success verifies UpdateSettingsForGroup when success.
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

// TestUpdateSettingsForGroup_ValidationError verifies UpdateSettingsForGroup when validation error.
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

// TestBuildUpdateOpts_AllBooleans verifies BuildUpdateOpts when all booleans.
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

// TestBuildUpdateOpts_UnknownLevel verifies BuildUpdateOpts when unknown level.
func TestBuildUpdateOpts_UnknownLevel(t *testing.T) {
	opts := buildUpdateOpts(eventFields{Level: "unknown_level"})
	if opts.Level != nil {
		t.Error("unknown level should not set Level")
	}
}

// TestBuildUpdateOpts_EmptyLevel verifies BuildUpdateOpts when empty level.
func TestBuildUpdateOpts_EmptyLevel(t *testing.T) {
	opts := buildUpdateOpts(eventFields{})
	if opts.Level != nil {
		t.Error("empty level should not set Level")
	}
}

// TestBuildUpdateOpts_ValidLevels verifies BuildUpdateOpts when valid levels.
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

// TestFormatMarkdown_Wrapper verifies FormatMarkdown when wrapper.
func TestFormatMarkdown_Wrapper(t *testing.T) {
	result := FormatMarkdown(Output{Level: "watch"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// eventLine.

// TestEventLine_Enabled verifies EventLine when enabled.
func TestEventLine_Enabled(t *testing.T) {
	line := eventLine("Test Event", true)
	if !strings.Contains(line, "✅") {
		t.Error("expected checkmark for enabled")
	}
	if !strings.Contains(line, "Test Event") {
		t.Error("expected event name")
	}
}

// TestEventLine_Disabled verifies EventLine when disabled.
func TestEventLine_Disabled(t *testing.T) {
	line := eventLine("Test Event", false)
	if !strings.Contains(line, "❌") {
		t.Error("expected cross for disabled")
	}
}

// FormatMarkdownString edge cases.

// TestFormatMarkdownString_NoEmail verifies FormatMarkdownString when no email.
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

// TestFormatMarkdownString_AllEvents verifies FormatMarkdownString when all events.
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

// TestActionSpecs_Metadata verifies notification settings action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "notifications" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// ActionSpec route execution for all 6 tools.

// TestActionSpecs_CallRoutes covers ActionSpecs with table-driven subtests for call routes.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, covSettingsJSON)
	}))
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
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
			spec, ok := specByTool[tc.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.name)
			}
			res, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler %s: %v", tc.name, err)
			}
			if res == nil {
				t.Fatalf("nil result for %s", tc.name)
			}
		})
	}
}
