// Package mrapprovalsettings tests validate the MCP tool handlers for GitLab
// merge request approval settings at project and group level.
// Covers success, input validation (missing IDs), API errors, and context
// cancellation for all four handlers, plus markdown formatting.
package mrapprovalsettings

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"internal server error"}`)
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
// FormatOutputMarkdown
// ---------------------------------------------------------------------------

// TestFormatOutputMarkdown validates the markdown table rendering for
// different scopes and setting combinations.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name        string
		output      Output
		scope       string
		wantContain []string
	}{
		{
			name:  "renders project scope with all fields",
			scope: "Project",
			output: Output{
				AllowAuthorApproval:    SettingOutput{Value: true, Locked: false},
				AllowCommitterApproval: SettingOutput{Value: false, Locked: true, InheritedFrom: "group"},
			},
			wantContain: []string{
				"## Project MR Approval Settings",
				"Allow author approval",
				"Allow committer approval",
				"group",
				"gitlab_update_project_mr_approval_settings",
			},
		},
		{
			name:  "renders group scope with hint",
			scope: "Group",
			output: Output{
				RetainApprovalsOnPush: SettingOutput{Value: true, Locked: false},
			},
			wantContain: []string{
				"## Group MR Approval Settings",
				"Retain approvals on push",
				"gitlab_update_group_mr_approval_settings",
			},
		},
		{
			name:   "renders inherited_from dash when empty",
			scope:  "Project",
			output: Output{AllowAuthorApproval: SettingOutput{Value: false, Locked: false, InheritedFrom: ""}},
			wantContain: []string{
				"—",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatOutputMarkdown(tt.output, tt.scope)
			if md == "" {
				t.Fatal("expected non-empty markdown")
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q", want)
				}
			}
		})
	}
}
