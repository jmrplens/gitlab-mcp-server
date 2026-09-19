package mrapprovalsettings

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const (
	pathGroupSettings   = "/api/v4/groups/mygroup/merge_request_approval_setting"
	pathProjectSettings = "/api/v4/projects/42/merge_request_approval_setting"
)

const settingsJSON = `{
	"allow_author_approval":{"value":false,"locked":false,"inherited_from":""},
	"allow_committer_approval":{"value":true,"locked":true,"inherited_from":"group"},
	"allow_overrides_to_approver_list_per_merge_request":{"value":false,"locked":false,"inherited_from":""},
	"retain_approvals_on_push":{"value":true,"locked":false,"inherited_from":""},
	"selective_code_owner_removals":{"value":false,"locked":false,"inherited_from":""},
	"require_password_to_approve":{"value":false,"locked":false,"inherited_from":""},
	"require_reauthentication_to_approve":{"value":false,"locked":false,"inherited_from":""}
}`

// ---------------------------------------------------------------------------
// GetGroupSettings
// ---------------------------------------------------------------------------

// TestGetGroupSettings validates the GetGroupSettings handler covering
// success, missing group_id, API errors, and cancelled context.
func TestGetGroupSettings(t *testing.T) {
	tests := []struct {
		name       string
		input      GroupGetInput
		handler    http.HandlerFunc
		cancelCtx  bool
		wantErr    bool
		errContain string
		validate   func(t *testing.T, out Output)
	}{
		{
			name:  "returns settings for valid group",
			input: GroupGetInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathGroupSettings)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.AllowCommitterApproval.Value != true {
					t.Error("expected AllowCommitterApproval.Value true")
				}
				if out.AllowCommitterApproval.Locked != true {
					t.Error("expected AllowCommitterApproval.Locked true")
				}
				if out.AllowCommitterApproval.InheritedFrom != "group" {
					t.Errorf("InheritedFrom = %q, want %q", out.AllowCommitterApproval.InheritedFrom, "group")
				}
				if out.RetainApprovalsOnPush.Value != true {
					t.Error("expected RetainApprovalsOnPush.Value true")
				}
				if out.AllowAuthorApproval.Value != false {
					t.Error("expected AllowAuthorApproval.Value false")
				}
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      GroupGetInput{},
			handler:    http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:  "returns error on 404 API response",
			input: GroupGetInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 500 API response",
			input: GroupGetInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal server error"}`)
			}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     GroupGetInput{GroupID: "mygroup"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { testutil.RespondJSON(w, http.StatusOK, settingsJSON) }),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := GetGroupSettings(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateGroupSettings
// ---------------------------------------------------------------------------

// TestUpdateGroupSettings validates the UpdateGroupSettings handler covering
// success with multiple fields, missing group_id, API errors, and cancelled context.
func TestUpdateGroupSettings(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name       string
		input      GroupUpdateInput
		handler    http.HandlerFunc
		cancelCtx  bool
		wantErr    bool
		errContain string
		validate   func(t *testing.T, out Output)
	}{
		{
			name: "updates settings with all fields",
			input: GroupUpdateInput{
				GroupID:                "mygroup",
				AllowAuthorApproval:    &boolTrue,
				AllowCommitterApproval: &boolFalse,
				AllowOverridesToApproverListPerMergeRequest: &boolTrue,
				RetainApprovalsOnPush:                       &boolFalse,
				RequireReauthenticationToApprove:            &boolTrue,
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.AssertRequestPath(t, r, pathGroupSettings)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.RetainApprovalsOnPush.Value != true {
					t.Error("expected RetainApprovalsOnPush.Value true")
				}
			},
		},
		{
			name:       "returns error when group_id is empty",
			input:      GroupUpdateInput{},
			handler:    http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr:    true,
			errContain: "group_id",
		},
		{
			name:  "returns error on 403 API response",
			input: GroupUpdateInput{GroupID: "mygroup", AllowAuthorApproval: &boolTrue},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     GroupUpdateInput{GroupID: "mygroup"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { testutil.RespondJSON(w, http.StatusOK, settingsJSON) }),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := UpdateGroupSettings(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetProjectSettings
// ---------------------------------------------------------------------------

// TestGetProjectSettings validates the GetProjectSettings handler covering
// success, missing project_id, API errors, and cancelled context.
func TestGetProjectSettings(t *testing.T) {
	tests := []struct {
		name       string
		input      ProjectGetInput
		handler    http.HandlerFunc
		cancelCtx  bool
		wantErr    bool
		errContain string
		validate   func(t *testing.T, out Output)
	}{
		{
			name:  "returns settings for valid project",
			input: ProjectGetInput{ProjectID: "42"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathProjectSettings)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.AllowAuthorApproval.Value != false {
					t.Error("expected AllowAuthorApproval.Value false")
				}
				if out.SelectiveCodeOwnerRemovals.Value != false {
					t.Error("expected SelectiveCodeOwnerRemovals.Value false")
				}
				if out.RequirePasswordToApprove.Value != false {
					t.Error("expected RequirePasswordToApprove.Value false")
				}
			},
		},
		{
			name:       "returns error when project_id is empty",
			input:      ProjectGetInput{},
			handler:    http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr:    true,
			errContain: "project_id",
		},
		{
			name:  "returns error on 404 API response",
			input: ProjectGetInput{ProjectID: "42"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     ProjectGetInput{ProjectID: "42"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { testutil.RespondJSON(w, http.StatusOK, settingsJSON) }),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := GetProjectSettings(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateProjectSettings
// ---------------------------------------------------------------------------

// TestUpdateProjectSettings validates the UpdateProjectSettings handler covering
// success with all fields, missing project_id, API errors, and cancelled context.
func TestUpdateProjectSettings(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name       string
		input      ProjectUpdateInput
		handler    http.HandlerFunc
		cancelCtx  bool
		wantErr    bool
		errContain string
		validate   func(t *testing.T, out Output)
	}{
		{
			name: "updates settings with all fields",
			input: ProjectUpdateInput{
				ProjectID:              "42",
				AllowAuthorApproval:    &boolTrue,
				AllowCommitterApproval: &boolFalse,
				AllowOverridesToApproverListPerMergeRequest: &boolTrue,
				RetainApprovalsOnPush:                       &boolFalse,
				RequireReauthenticationToApprove:            &boolTrue,
				SelectiveCodeOwnerRemovals:                  &boolFalse,
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.AssertRequestPath(t, r, pathProjectSettings)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}),
			validate: func(t *testing.T, out Output) {
				t.Helper()
				if out.RequirePasswordToApprove.Value != false {
					t.Error("expected RequirePasswordToApprove.Value false")
				}
			},
		},
		{
			name:       "returns error when project_id is empty",
			input:      ProjectUpdateInput{},
			handler:    http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr:    true,
			errContain: "project_id",
		},
		{
			name:  "returns error on 422 API response",
			input: ProjectUpdateInput{ProjectID: "42", AllowAuthorApproval: &boolTrue},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
			}),
			wantErr: true,
		},
		{
			name:      "returns error when context is cancelled",
			input:     ProjectUpdateInput{ProjectID: "42"},
			handler:   http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { testutil.RespondJSON(w, http.StatusOK, settingsJSON) }),
			cancelCtx: true,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			ctx := context.Background()
			if tt.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			out, err := UpdateProjectSettings(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errContain != "" && err != nil && !strings.Contains(err.Error(), tt.errContain) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
			}
			if tt.validate != nil {
				tt.validate(t, out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// What reaches GitLab, and where the answer lands
// ---------------------------------------------------------------------------

// decodeSettingsRequestBody reads the JSON body of an update request. It is
// called from the mock's own goroutine, so it reports and returns rather than
// aborting the test.
func decodeSettingsRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request body: %v", err)
		return nil
	}
	return body
}

// assertOnlyFieldSent holds an update body to the one field the caller named:
// present and true, with nothing beside it. The absence half carries as much as
// the presence half, because the SDK's options are pointers with omitempty, so
// "the caller said nothing about this setting" and "the caller asked for it to
// be off" are the same request unless the key stays off the wire.
func assertOnlyFieldSent(t *testing.T, body map[string]any, want string) {
	t.Helper()
	if body[want] != true {
		t.Errorf("%s sent = %v, want true", want, body[want])
	}
	for key, value := range body {
		if key != want {
			t.Errorf("body also carries %s = %v, want %s alone", key, value, want)
		}
	}
}

// TestUpdateGroupSettings_OneFieldSet_SendsThatFieldAlone holds each optional
// input of the group update to the GitLab name it travels under, one field per
// subtest.
//
// Nothing else here reads the request body, so a setting wired to the wrong
// option or left out of the options struct is invisible: the handler still
// returns whatever the mock answers. Dropping RequireReauthenticationToApprove
// passed the whole suite before this existed, which means a caller asking to
// require reauthentication would have been silently ignored.
func TestUpdateGroupSettings_OneFieldSet_SendsThatFieldAlone(t *testing.T) {
	yes := true
	tests := []struct {
		name  string
		input GroupUpdateInput
	}{
		{"allow_author_approval", GroupUpdateInput{GroupID: "mygroup", AllowAuthorApproval: &yes}},
		{"allow_committer_approval", GroupUpdateInput{GroupID: "mygroup", AllowCommitterApproval: &yes}},
		{"allow_overrides_to_approver_list_per_merge_request", GroupUpdateInput{GroupID: "mygroup", AllowOverridesToApproverListPerMergeRequest: &yes}},
		{"retain_approvals_on_push", GroupUpdateInput{GroupID: "mygroup", RetainApprovalsOnPush: &yes}},
		{"require_reauthentication_to_approve", GroupUpdateInput{GroupID: "mygroup", RequireReauthenticationToApprove: &yes}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assertOnlyFieldSent(t, decodeSettingsRequestBody(t, r), tt.name)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}))
			if _, err := UpdateGroupSettings(context.Background(), client, tt.input); err != nil {
				t.Fatalf("UpdateGroupSettings: %v", err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("requests = %d, want 1", got)
			}
		})
	}
}

// TestUpdateProjectSettings_OneFieldSet_SendsThatFieldAlone is the project half
// of the same property, and carries the one setting only projects have:
// selective_code_owner_removals is absent from the group options struct, so it
// is the field a copy-paste between the two handlers would lose.
func TestUpdateProjectSettings_OneFieldSet_SendsThatFieldAlone(t *testing.T) {
	yes := true
	tests := []struct {
		name  string
		input ProjectUpdateInput
	}{
		{"allow_author_approval", ProjectUpdateInput{ProjectID: "42", AllowAuthorApproval: &yes}},
		{"allow_committer_approval", ProjectUpdateInput{ProjectID: "42", AllowCommitterApproval: &yes}},
		{"allow_overrides_to_approver_list_per_merge_request", ProjectUpdateInput{ProjectID: "42", AllowOverridesToApproverListPerMergeRequest: &yes}},
		{"retain_approvals_on_push", ProjectUpdateInput{ProjectID: "42", RetainApprovalsOnPush: &yes}},
		{"require_reauthentication_to_approve", ProjectUpdateInput{ProjectID: "42", RequireReauthenticationToApprove: &yes}},
		{"selective_code_owner_removals", ProjectUpdateInput{ProjectID: "42", SelectiveCodeOwnerRemovals: &yes}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int64
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assertOnlyFieldSent(t, decodeSettingsRequestBody(t, r), tt.name)
				testutil.RespondJSON(w, http.StatusOK, settingsJSON)
			}))
			if _, err := UpdateProjectSettings(context.Background(), client, tt.input); err != nil {
				t.Fatalf("UpdateProjectSettings: %v", err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("requests = %d, want 1", got)
			}
		})
	}
}

// distinctSettingsJSON names each setting's inherited_from after the setting
// itself, so no two of the seven are interchangeable.
const distinctSettingsJSON = `{
	"allow_author_approval":{"value":true,"locked":false,"inherited_from":"allow_author_approval"},
	"allow_committer_approval":{"value":true,"locked":false,"inherited_from":"allow_committer_approval"},
	"allow_overrides_to_approver_list_per_merge_request":{"value":true,"locked":false,"inherited_from":"allow_overrides_to_approver_list_per_merge_request"},
	"retain_approvals_on_push":{"value":true,"locked":false,"inherited_from":"retain_approvals_on_push"},
	"selective_code_owner_removals":{"value":true,"locked":false,"inherited_from":"selective_code_owner_removals"},
	"require_password_to_approve":{"value":true,"locked":false,"inherited_from":"require_password_to_approve"},
	"require_reauthentication_to_approve":{"value":true,"locked":false,"inherited_from":"require_reauthentication_to_approve"}
}`

// TestGetProjectSettings_EachSetting_LandsInTheFieldNamedAfterIt holds the
// converter every one of the four handlers shares to reading each setting from
// the source that carries its name.
//
// The ordinary fixture leaves five of the seven at the same (false, false, "")
// triple, so a converter reading require_password_to_approve into
// RequireReauthenticationToApprove and back answers exactly what every other
// assertion in this file expects. inherited_from is the discriminator because
// it is the one cell of a setting that is not a boolean.
func TestGetProjectSettings_EachSetting_LandsInTheFieldNamedAfterIt(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, distinctSettingsJSON)
	}))
	out, err := GetProjectSettings(context.Background(), client, ProjectGetInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}

	tests := []struct {
		name string
		got  SettingOutput
	}{
		{"allow_author_approval", out.AllowAuthorApproval},
		{"allow_committer_approval", out.AllowCommitterApproval},
		{"allow_overrides_to_approver_list_per_merge_request", out.AllowOverridesToApproverListPerMergeRequest},
		{"retain_approvals_on_push", out.RetainApprovalsOnPush},
		{"selective_code_owner_removals", out.SelectiveCodeOwnerRemovals},
		{"require_password_to_approve", out.RequirePasswordToApprove},
		{"require_reauthentication_to_approve", out.RequireReauthenticationToApprove},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.InheritedFrom != tt.name {
				t.Errorf("read from %q, want %q", tt.got.InheritedFrom, tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------

// settingsTableHead is the header and delimiter of the approval settings table.
const settingsTableHead = "| Setting | Value | Locked | Inherited From |\n| --- | --- | --- | --- |\n"

// settingsRows renders the seven rows of a settings table with the value,
// locked and inherited-from cells each case expects, so the whole-output
// expectations below name only what differs.
func settingsRows(rows ...string) string {
	return strings.Join(rows, "")
}

// TestFormatOutputMarkdown validates the whole rendering of the approval
// settings for each scope: the heading the scope names, the seven settings as
// one table, and the update action the hint points at.
//
// The empty scope is its own case because it is the one the registry reaches:
// both the group and the project route answer with [Output], so the registered
// formatter cannot know which one it is rendering. It used to interpolate the
// empty string into both the heading and a tool name, giving
// "##  MR Approval Settings" and gitlab_update__mr_approval_settings.
func TestFormatOutputMarkdown(t *testing.T) {
	groupHint := "\n---\n💡 **Next steps:**\n- Use action 'merge_request.approval_settings_group_update' to change these group settings\n"
	projectHint := "\n---\n💡 **Next steps:**\n- Use action 'merge_request.approval_settings_project_update' to change these project settings\n"
	bothHints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'merge_request.approval_settings_group_update' to change a group's approval settings\n" +
		"- Use action 'merge_request.approval_settings_project_update' to change a project's approval settings\n"
	allOff := settingsRows(
		"| Allow approver list overrides | ❌ | ❌ | - |\n",
		"| Retain approvals on push | ❌ | ❌ | - |\n",
		"| Selective code owner removals | ❌ | ❌ | - |\n",
		"| Require password to approve | ❌ | ❌ | - |\n",
		"| Require reauthentication | ❌ | ❌ | - |\n",
	)
	tests := []struct {
		name   string
		output Output
		scope  string
		want   string
	}{
		{
			name:  "renders project scope with all fields",
			scope: "Project",
			output: Output{
				AllowAuthorApproval:    SettingOutput{Value: true, Locked: false},
				AllowCommitterApproval: SettingOutput{Value: false, Locked: true, InheritedFrom: "group"},
			},
			want: "## Project MR Approval Settings\n\n" + settingsTableHead +
				"| Allow author approval | ✅ | ❌ | - |\n" +
				"| Allow committer approval | ❌ | ✅ | group |\n" + allOff + projectHint,
		},
		{
			name:   "renders group scope with hint",
			scope:  "Group",
			output: Output{RetainApprovalsOnPush: SettingOutput{Value: true, Locked: false}},
			want: "## Group MR Approval Settings\n\n" + settingsTableHead +
				"| Allow author approval | ❌ | ❌ | - |\n" +
				"| Allow committer approval | ❌ | ❌ | - |\n" +
				"| Allow approver list overrides | ❌ | ❌ | - |\n" +
				"| Retain approvals on push | ✅ | ❌ | - |\n" +
				"| Selective code owner removals | ❌ | ❌ | - |\n" +
				"| Require password to approve | ❌ | ❌ | - |\n" +
				"| Require reauthentication | ❌ | ❌ | - |\n" + groupHint,
		},
		{
			name:  "renders no scope with a neutral heading and both update actions",
			scope: "",
			output: Output{
				AllowAuthorApproval: SettingOutput{Value: true},
			},
			want: "## MR Approval Settings\n\n" + settingsTableHead +
				"| Allow author approval | ✅ | ❌ | - |\n" +
				"| Allow committer approval | ❌ | ❌ | - |\n" + allOff + bothHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.output, tt.scope); got != tt.want {
				t.Errorf("rendered =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
