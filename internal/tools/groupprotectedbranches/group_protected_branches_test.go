// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// group_protected_branches_test.go contains unit tests for the GitLab group protected branch MCP tool handlers.
package groupprotectedbranches

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	pathGroupProtBranches = "/api/v4/groups/mygroup/protected_branches"
	pathGroupProtBranch   = "/api/v4/groups/mygroup/protected_branches/main"
	pathGroupProtRelease  = "/api/v4/groups/mygroup/protected_branches/release%2F1.0"
)

const branchJSON = `{
	"id":1,
	"name":"main",
	"push_access_levels":[{"id":10,"access_level":40,"access_level_description":"Maintainers","user_id":0,"group_id":0,"deploy_key_id":0}],
	"merge_access_levels":[{"id":11,"access_level":30,"access_level_description":"Developers + Maintainers"}],
	"unprotect_access_levels":[],
	"allow_force_push":false,
	"code_owner_approval_required":true
}`

const branchListJSON = `[
	{"id":1,"name":"main","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":false,"code_owner_approval_required":true},
	{"id":2,"name":"release/*","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":true,"code_owner_approval_required":false}
]`

// TestList validates the List handler for group protected branches.
// Covers success with and without search, pagination, empty results,
// API errors, missing group_id, and context cancellation.
func TestList(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		handler   http.HandlerFunc
		wantErr   bool
		wantCount int
		wantFirst string
	}{
		{
			name:  "returns branches for group",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathGroupProtBranches)
				testutil.RespondJSON(w, http.StatusOK, branchListJSON)
			}),
			wantCount: 2,
			wantFirst: "main",
		},
		{
			name:  "passes search parameter to API",
			input: ListInput{GroupID: "mygroup", Search: "release"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertQueryParam(t, r, "search", "release")
				testutil.RespondJSON(w, http.StatusOK, `[{"id":2,"name":"release/*","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":true,"code_owner_approval_required":false}]`)
			}),
			wantCount: 1,
			wantFirst: "release/*",
		},
		{
			name:  "returns empty list when no branches found",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			}),
			wantCount: 0,
		},
		{
			name:  "includes pagination from response headers",
			input: ListInput{GroupID: "mygroup", PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 1}},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "page", "2")
				testutil.AssertQueryParam(t, r, "per_page", "1")
				testutil.RespondJSONWithPagination(w, http.StatusOK, `[{"id":2,"name":"release/*","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[]}]`, testutil.PaginationHeaders{
					Page: "2", PerPage: "1", Total: "2", TotalPages: "2", PrevPage: "1",
				})
			}),
			wantCount: 1,
			wantFirst: "release/*",
		},
		{
			name: "passes order_by, sort and keyset parameters to API",
			input: ListInput{
				GroupID:               "mygroup",
				OrderBy:               "name",
				Sort:                  "desc",
				KeysetPaginationInput: toolutil.KeysetPaginationInput{Pagination: "keyset", PageToken: "main"},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertQueryParam(t, r, "order_by", "name")
				testutil.AssertQueryParam(t, r, "sort", "desc")
				testutil.AssertQueryParam(t, r, "pagination", "keyset")
				testutil.AssertQueryParam(t, r, "page_token", "main")
				testutil.RespondJSON(w, http.StatusOK, branchListJSON)
			}),
			wantCount: 2,
			wantFirst: "main",
		},
		{
			name:    "returns error when group_id is empty",
			input:   ListInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := List(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(out.Branches) != tt.wantCount {
				t.Fatalf("len(Branches) = %d, want %d", len(out.Branches), tt.wantCount)
			}
			if tt.wantFirst != "" && len(out.Branches) > 0 {
				if out.Branches[0].Name != tt.wantFirst {
					t.Errorf("first branch Name = %q, want %q", out.Branches[0].Name, tt.wantFirst)
				}
			}
		})
	}
}

// TestList_ContextCancelled verifies the List_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestList_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestGet validates the Get handler for a single group protected branch.
// Covers success with access levels, missing fields, API errors, and
// context cancellation.
func TestGet(t *testing.T) {
	tests := []struct {
		name    string
		input   GetInput
		handler http.HandlerFunc
		wantErr bool
		check   func(t *testing.T, out Output)
	}{
		{
			name:  "returns branch with access levels",
			input: GetInput{GroupID: "mygroup", Branch: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathGroupProtBranch)
				testutil.RespondJSON(w, http.StatusOK, branchJSON)
			}),
			check: assertProtectedBranchAccessLevels,
		},
		{
			name:  "escapes branch name with slash once",
			input: GetInput{GroupID: "mygroup", Branch: "release/1.0"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				if got := r.URL.EscapedPath(); got != pathGroupProtRelease {
					t.Errorf("EscapedPath = %q, want %q", got, pathGroupProtRelease)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"id":2,"name":"release/1.0","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":false}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "release/1.0" {
					t.Errorf("Name = %q, want %q", out.Name, "release/1.0")
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   GetInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when branch is empty",
			input:   GetInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on 404 API response",
			input: GetInput{GroupID: "mygroup", Branch: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Get(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

func assertProtectedBranchAccessLevels(t *testing.T, out Output) {
	t.Helper()
	if out.ID != 1 {
		t.Errorf("ID = %d, want 1", out.ID)
	}
	if out.Name != "main" {
		t.Errorf("Name = %q, want %q", out.Name, "main")
	}
	if !out.CodeOwnerApprovalRequired {
		t.Error("CodeOwnerApprovalRequired = false, want true")
	}
	assertProtectedBranchPushAccess(t, out.PushAccessLevels)
	if len(out.MergeAccessLevels) != 1 {
		t.Fatalf("len(MergeAccessLevels) = %d, want 1", len(out.MergeAccessLevels))
	}
	if len(out.UnprotectAccessLevels) != 0 {
		t.Errorf("len(UnprotectAccessLevels) = %d, want 0", len(out.UnprotectAccessLevels))
	}
}

func assertProtectedBranchPushAccess(t *testing.T, levels []AccessLevelOutput) {
	t.Helper()
	if len(levels) != 1 {
		t.Fatalf("len(PushAccessLevels) = %d, want 1", len(levels))
	}
	if levels[0].AccessLevel != 40 {
		t.Errorf("PushAccessLevels[0].AccessLevel = %d, want 40", levels[0].AccessLevel)
	}
	if levels[0].AccessLevelDescription != "Maintainers" {
		t.Errorf("PushAccessLevels[0].Description = %q, want %q", levels[0].AccessLevelDescription, "Maintainers")
	}
}

// TestGet_ContextCancelled verifies the Get_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestGet_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, branchJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{GroupID: "mygroup", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestProtect validates the Protect handler for creating group-level
// protected branch rules. Covers basic success, access level options,
// branch permissions, missing fields, API errors, and context cancellation.
func TestProtect(t *testing.T) {
	pushLevel := 40
	mergeLevel := 30
	unprotectLevel := 60
	allowForce := true
	codeOwner := true
	accessLvl := 40
	userID := int64(5)
	groupID := int64(10)
	deployKeyID := int64(99)
	destroy := false

	tests := []struct {
		name    string
		input   ProtectInput
		handler http.HandlerFunc
		wantErr bool
		check   func(t *testing.T, out Output)
	}{
		{
			name:  "creates basic protected branch",
			input: ProtectInput{GroupID: "mygroup", Name: "release/*"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathGroupProtBranches)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":2,"name":"release/*","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":false}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "release/*" {
					t.Errorf("Name = %q, want %q", out.Name, "release/*")
				}
			},
		},
		{
			name: "creates branch with all access levels",
			input: ProtectInput{
				GroupID:                   "mygroup",
				Name:                      "main",
				PushAccessLevel:           &pushLevel,
				MergeAccessLevel:          &mergeLevel,
				UnprotectAccessLevel:      &unprotectLevel,
				AllowForcePush:            &allowForce,
				CodeOwnerApprovalRequired: &codeOwner,
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.RespondJSON(w, http.StatusCreated, branchJSON)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "main" {
					t.Errorf("Name = %q, want %q", out.Name, "main")
				}
				if !out.CodeOwnerApprovalRequired {
					t.Error("CodeOwnerApprovalRequired = false, want true")
				}
			},
		},
		{
			name: "creates branch with allowed_to permissions",
			input: ProtectInput{
				GroupID: "mygroup",
				Name:    "develop",
				AllowedToPush: []BranchPermissionInput{
					{AccessLevel: &accessLvl, UserID: &userID},
				},
				AllowedToMerge: []BranchPermissionInput{
					{GroupID: &groupID, DeployKeyID: &deployKeyID, Destroy: &destroy},
				},
				AllowedToUnprotect: []BranchPermissionInput{
					{AccessLevel: &accessLvl},
				},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"name":"develop","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":false}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "develop" {
					t.Errorf("Name = %q, want %q", out.Name, "develop")
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   ProtectInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when name is empty",
			input:   ProtectInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on 409 API conflict",
			input: ProtectInput{GroupID: "mygroup", Name: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusConflict, `{"message":"409 Conflict"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Protect(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Protect() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// TestProtect_ContextCancelled verifies the Protect_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestProtect_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Protect(ctx, client, ProtectInput{GroupID: "mygroup", Name: "main"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestUpdate validates the Update handler for modifying group-level
// protected branch rules. Covers success with name change, with permissions,
// missing fields, API errors, and context cancellation.
func TestUpdate(t *testing.T) {
	allowForce := true
	accessLvl := 30
	permID := int64(100)

	tests := []struct {
		name    string
		input   UpdateInput
		handler http.HandlerFunc
		wantErr bool
		check   func(t *testing.T, out Output)
	}{
		{
			name:  "updates basic settings",
			input: UpdateInput{GroupID: "mygroup", Branch: "main", AllowForcePush: &allowForce},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPatch)
				testutil.AssertRequestPath(t, r, pathGroupProtBranch)
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":true}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if !out.AllowForcePush {
					t.Error("AllowForcePush = false, want true")
				}
			},
		},
		{
			name:  "escapes branch name with slash once",
			input: UpdateInput{GroupID: "mygroup", Branch: "release/1.0", AllowForcePush: &allowForce},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPatch)
				if got := r.URL.EscapedPath(); got != pathGroupProtRelease {
					t.Errorf("EscapedPath = %q, want %q", got, pathGroupProtRelease)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"id":2,"name":"release/1.0","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":true}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if !out.AllowForcePush {
					t.Error("AllowForcePush = false, want true")
				}
			},
		},
		{
			name:  "updates with new branch name",
			input: UpdateInput{GroupID: "mygroup", Branch: "main", Name: "main-v2"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPatch)
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main-v2","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":false}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "main-v2" {
					t.Errorf("Name = %q, want %q", out.Name, "main-v2")
				}
			},
		},
		{
			name: "updates with allowed_to permissions",
			input: UpdateInput{
				GroupID: "mygroup",
				Branch:  "main",
				AllowedToPush: []BranchPermissionInput{
					{ID: &permID, AccessLevel: &accessLvl},
				},
				AllowedToMerge: []BranchPermissionInput{
					{AccessLevel: &accessLvl},
				},
				AllowedToUnprotect: []BranchPermissionInput{
					{AccessLevel: &accessLvl},
				},
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","push_access_levels":[],"merge_access_levels":[],"unprotect_access_levels":[],"allow_force_push":false}`)
			}),
			check: func(t *testing.T, out Output) {
				t.Helper()
				if out.Name != "main" {
					t.Errorf("Name = %q, want %q", out.Name, "main")
				}
			},
		},
		{
			name:    "returns error when group_id is empty",
			input:   UpdateInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when branch is empty",
			input:   UpdateInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: UpdateInput{GroupID: "mygroup", Branch: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Update(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, out)
			}
		})
	}
}

// TestUpdate_ContextCancelled verifies the Update_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUpdate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Update(ctx, client, UpdateInput{GroupID: "mygroup", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestUpdate_Rename_SendsNameOnlyWhenTheCallerGaveOne holds both halves of the
// optional rename: `name` reaches GitLab exactly when the caller supplied one,
// and the key is absent from the body when they did not.
//
// Why it matters: every other rename assertion in this file reads the new name
// back out of the mock's own response, which the test itself wrote, so both
// sides of that comparison move together and the guard deciding whether `name`
// is sent at all can be inverted without failing anything. Inverted, a caller
// who asked for a rename has it silently dropped and is handed back a rule
// still carrying its old name, while a caller who asked for no rename has
// GitLab told to name the rule the empty string, which it refuses. Only the
// request the handler builds can tell those apart, so this asserts on the body
// rather than on the output.
func TestUpdate_Rename_SendsNameOnlyWhenTheCallerGaveOne(t *testing.T) {
	tests := []struct {
		name     string
		input    UpdateInput
		wantSent bool
		wantName string
	}{
		{
			name:     "a supplied name reaches GitLab",
			input:    UpdateInput{GroupID: "mygroup", Branch: "main", Name: "main-v2"},
			wantSent: true,
			wantName: "main-v2",
		},
		{
			name:  "no name leaves the key out of the body",
			input: UpdateInput{GroupID: "mygroup", Branch: "main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode PATCH body: %v", err)
					http.Error(w, "decode PATCH body", http.StatusInternalServerError)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, branchJSON)
			}))

			if _, err := Update(context.Background(), client, tt.input); err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			got, sent := body["name"]
			if sent != tt.wantSent {
				t.Fatalf("name sent = %v, want %v; body = %#v", sent, tt.wantSent, body)
			}
			if tt.wantSent && got != tt.wantName {
				t.Errorf("name = %#v, want %q", got, tt.wantName)
			}
		})
	}
}

// customRoleBranchJSON is a group protected branch whose merge rule grants a
// custom member role, as ee/lib/ee/api/entities/protected_ref_access.rb
// renders it on an Ultimate instance.
const customRoleBranchJSON = `{"id":4,"name":"release/*","push_access_levels":[],` +
	`"merge_access_levels":[{"id":41,"access_level":30,"access_level_description":"Release role","member_role_id":42,"member_role_name":"Release role"}],` +
	`"unprotect_access_levels":[],"allow_force_push":false,"code_owner_approval_required":false}`

// TestCustomRolePermission_ProtectAndUpdate_SendAndPublishMemberRoleID pins
// member_role_id on both sides of the two writes: a custom role the caller
// names in allowed_to_merge reaches GitLab on that entry's own key and on no
// other, and the rule GitLab answers with publishes it on the access level,
// with the role's name read off the captured answer. The role's id is the only
// id in the request, so a handler that wrote it to the user, group or deploy
// key key, or dropped it, fails here.
func TestCustomRolePermission_ProtectAndUpdate_SendAndPublishMemberRoleID(t *testing.T) {
	allowed := []BranchPermissionInput{{MemberRoleID: new(int64(42))}}
	tests := []struct {
		name   string
		method string
		path   string
		call   func(context.Context, *gitlabclient.Client) (Output, error)
	}{
		{
			name:   "protect",
			method: http.MethodPost,
			path:   pathGroupProtBranches,
			call: func(ctx context.Context, client *gitlabclient.Client) (Output, error) {
				return Protect(ctx, client, ProtectInput{GroupID: "mygroup", Name: "release/*", AllowedToMerge: allowed})
			},
		},
		{
			name:   "update",
			method: http.MethodPatch,
			path:   pathGroupProtBranch,
			call: func(ctx context.Context, client *gitlabclient.Client) (Output, error) {
				return Update(ctx, client, UpdateInput{GroupID: "mygroup", Branch: "main", AllowedToMerge: allowed})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, out := captureAllowedToMerge(t, tt.method, tt.path, tt.call)
			assertCustomRoleEntry(t, entries)
			if len(out.MergeAccessLevels) != 1 || out.MergeAccessLevels[0].MemberRoleID != 42 ||
				out.MergeAccessLevels[0].MemberRoleName != "Release role" {
				t.Errorf("MergeAccessLevels = %+v, want one entry with member_role_id 42 named Release role", out.MergeAccessLevels)
			}
		})
	}
}

// captureAllowedToMerge runs call against a mock answering method and path
// with customRoleBranchJSON, and returns the allowed_to_merge entries of the
// body the handler sent beside the output it built. The body is decoded on the
// mock's goroutine, which reports a failure with t.Errorf only.
func captureAllowedToMerge(t *testing.T, method, path string, call func(context.Context, *gitlabclient.Client) (Output, error)) ([]map[string]any, Output) {
	t.Helper()
	var body struct {
		AllowedToMerge []map[string]any `json:"allowed_to_merge"`
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode %s body: %v", method, err)
			http.Error(w, "undecodable body", http.StatusInternalServerError)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, customRoleBranchJSON)
	}))

	out, err := call(context.Background(), client)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, path, err)
	}
	return body.AllowedToMerge, out
}

// assertCustomRoleEntry checks that the one allowed_to_merge entry carries the
// custom role on member_role_id and on no other key.
func assertCustomRoleEntry(t *testing.T, entries []map[string]any) {
	t.Helper()
	if len(entries) != 1 {
		t.Fatalf("allowed_to_merge = %#v, want one entry", entries)
	}
	entry := entries[0]
	if got := entry["member_role_id"]; got != float64(42) {
		t.Errorf("allowed_to_merge[0].member_role_id = %#v, want 42", got)
	}
	for _, key := range []string{"id", "user_id", "group_id", "deploy_key_id", "access_level", "_destroy"} {
		if got, sent := entry[key]; sent {
			t.Errorf("allowed_to_merge[0].%s = %#v, want the key left out", key, got)
		}
	}
}

// TestGet_MemberRoleName_ReadFromTheCapture pins the custom role name
// client-go does not model, which Get reads off the captured answer: published
// on the access level that carries it, and left empty where GitLab sent no
// name, by leaving the key out (an instance without custom roles licensed) or
// as null (a level that grants no custom role).
func TestGet_MemberRoleName_ReadFromTheCapture(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  string
	}{
		{name: "carries it", level: `{"id":11,"access_level":30,"member_role_id":42,"member_role_name":"Lead Developer"}`, want: "Lead Developer"},
		{name: "leaves the key out", level: `{"id":11,"access_level":30}`, want: ""},
		{name: "sends null", level: `{"id":11,"access_level":30,"member_role_id":null,"member_role_name":null}`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"main","unprotect_access_levels":[`+tt.level+`]}`)
			}))
			out, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", Branch: "main"})
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if len(out.UnprotectAccessLevels) != 1 || out.UnprotectAccessLevels[0].MemberRoleName != tt.want {
				t.Errorf("UnprotectAccessLevels = %+v, want one level named %q", out.UnprotectAccessLevels, tt.want)
			}
		})
	}
}

// TestList_MemberRoleName_FollowsItsOwnLevel holds each captured name to the
// access level it was sent on, across branches and arrays, so a converter that
// read the wrong index or the wrong branch's extra fails here.
func TestList_MemberRoleName_FollowsItsOwnLevel(t *testing.T) {
	const body = `[` +
		`{"id":1,"name":"main","push_access_levels":[{"id":11,"access_level":40},{"id":12,"access_level":30,"member_role_id":8,"member_role_name":"Release role"}]},` +
		`{"id":2,"name":"stable","merge_access_levels":[{"id":21,"access_level":40,"member_role_id":9,"member_role_name":"Auditor"}]}` +
		`]`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(out.Branches) != 2 {
		t.Fatalf("Branches = %+v, want 2", out.Branches)
	}
	push := out.Branches[0].PushAccessLevels
	if len(push) != 2 || push[0].MemberRoleName != "" || push[1].MemberRoleName != "Release role" || push[1].MemberRoleID != 8 {
		t.Errorf("main push levels = %+v, want the name on the second level only", push)
	}
	merge := out.Branches[1].MergeAccessLevels
	if len(merge) != 1 || merge[0].MemberRoleName != "Auditor" || merge[0].MemberRoleID != 9 {
		t.Errorf("stable merge levels = %+v, want the Auditor role", merge)
	}
}

// TestGroupProtectedBranches_UnreadableCapturedMemberRoleName verifies that
// every handler answering with a rule returns an error rather than a level
// without its name when GitLab sends member_role_name as something that is
// not a string. client-go does not model the key, so the captured read is the
// only thing that can notice.
func TestGroupProtectedBranches_UnreadableCapturedMemberRoleName(t *testing.T) {
	const branch = `{"id":1,"name":"main","push_access_levels":[{"id":11,"access_level":30,"member_role_id":8,"member_role_name":7}]}`
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			_, err := List(context.Background(), poisoned(`[`+branch+`]`), ListInput{GroupID: "mygroup"})
			return err
		}},
		{Name: "get", Call: func() error {
			_, err := Get(context.Background(), poisoned(branch), GetInput{GroupID: "mygroup", Branch: "main"})
			return err
		}},
		{Name: "protect", Call: func() error {
			_, err := Protect(context.Background(), poisoned(branch), ProtectInput{GroupID: "mygroup", Name: "main"})
			return err
		}},
		{Name: "update", Call: func() error {
			_, err := Update(context.Background(), poisoned(branch), UpdateInput{GroupID: "mygroup", Branch: "main"})
			return err
		}},
	})
}

// TestUnprotect validates the Unprotect handler for removing group-level
// protected branch rules. Covers success, missing fields, API errors,
// and context cancellation.
func TestUnprotect(t *testing.T) {
	tests := []struct {
		name    string
		input   UnprotectInput
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			name:  "removes protected branch rule",
			input: UnprotectInput{GroupID: "mygroup", Branch: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathGroupProtBranch)
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:  "escapes branch name with slash once",
			input: UnprotectInput{GroupID: "mygroup", Branch: "release/1.0"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				if got := r.URL.EscapedPath(); got != pathGroupProtRelease {
					t.Errorf("EscapedPath = %q, want %q", got, pathGroupProtRelease)
				}
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:    "returns error when group_id is empty",
			input:   UnprotectInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when branch is empty",
			input:   UnprotectInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on 404 API response",
			input: UnprotectInput{GroupID: "mygroup", Branch: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			err := Unprotect(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unprotect() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestUnprotect_ContextCancelled verifies the Unprotect_ContextCancelled handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that a canceled context aborts the call without contacting GitLab.
func TestUnprotect_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := testutil.CancelledCtx(t)
	err := Unprotect(ctx, client, UnprotectInput{GroupID: "mygroup", Branch: "main"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestToBranchPermissions validates the toBranchPermissions helper for
// converting input permission slices into GitLab API option structs.
// Tests empty input (nil return), entries with access levels, and entries
// without access levels.
func TestToBranchPermissions(t *testing.T) {
	t.Run("returns nil for empty input", func(t *testing.T) {
		got := toBranchPermissions(nil)
		if got != nil {
			t.Errorf("toBranchPermissions(nil) = %v, want nil", got)
		}
		got = toBranchPermissions([]BranchPermissionInput{})
		if got != nil {
			t.Errorf("toBranchPermissions([]) = %v, want nil", got)
		}
	})

	t.Run("converts entry with access level", func(t *testing.T) {
		lvl := 40
		userID := int64(5)
		groupID := int64(10)
		memberRoleID := int64(42)
		deployKeyID := int64(99)
		permID := int64(7)
		destroy := true

		got := toBranchPermissions([]BranchPermissionInput{{
			ID:           &permID,
			AccessLevel:  &lvl,
			UserID:       &userID,
			GroupID:      &groupID,
			MemberRoleID: &memberRoleID,
			DeployKeyID:  &deployKeyID,
			Destroy:      &destroy,
		}})
		assertBranchPermissionWithAccessLevel(t, got)
	})

	t.Run("converts entry without access level", func(t *testing.T) {
		userID := int64(3)
		got := toBranchPermissions([]BranchPermissionInput{
			{UserID: &userID},
		})
		if got == nil {
			t.Fatal("toBranchPermissions returned nil for non-empty input")
		}
		perms := *got
		if perms[0].AccessLevel != nil {
			t.Errorf("AccessLevel = %v, want nil", perms[0].AccessLevel)
		}
	})
}

func assertBranchPermissionWithAccessLevel(t *testing.T, got *[]*gl.GroupBranchPermissionOptions) {
	t.Helper()
	if got == nil {
		t.Fatal("toBranchPermissions returned nil for non-empty input")
	}
	perms := *got
	if len(perms) != 1 {
		t.Fatalf("len(perms) = %d, want 1", len(perms))
	}
	permission := perms[0]
	assertInt64Pointer(t, "ID", permission.ID, 7)
	assertInt64Pointer(t, "UserID", permission.UserID, 5)
	assertInt64Pointer(t, "GroupID", permission.GroupID, 10)
	assertInt64Pointer(t, "MemberRoleID", permission.MemberRoleID, 42)
	assertInt64Pointer(t, "DeployKeyID", permission.DeployKeyID, 99)
	if permission.Destroy == nil || !*permission.Destroy {
		t.Errorf("Destroy = %v, want true", permission.Destroy)
	}
	if permission.AccessLevel == nil {
		t.Fatal("AccessLevel = nil, want non-nil")
	}
}

func assertInt64Pointer(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %d", name, got, want)
	}
}

// cardHints is the guidance section every protected-branch card closes with.
const cardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'group.protected_branch_update' to add or remove an allowed user, group or role\n" +
	"- Use action 'group.protected_branch_unprotect' to remove this protection from every subgroup project\n"

// listHints is the guidance section the listing closes with. The table carries
// no link, so the preserve-links instruction is not written.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'group.protected_branch_get' to see one branch's access levels in full\n" +
	"- Use action 'group.protected_branch_protect' to protect another branch or wildcard\n" +
	"- Use action 'group.protected_branch_list' to page through the rest of the group's protected branches\n"

const accessHeader = "| ID | Level | Grantee | Description |\n| --- | --- | --- | --- |\n"

// TestFormatOutputMarkdown_WithAccessLevels pins the whole card of a protected
// branch: the flags as glyphs, each set of access levels as its own table, and
// no heading for a set GitLab sent empty.
func TestFormatOutputMarkdown_WithAccessLevels(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:   1,
		Name: "main",
		PushAccessLevels: []AccessLevelOutput{
			{ID: 10, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
		},
		MergeAccessLevels: []AccessLevelOutput{
			{ID: 11, AccessLevel: 30, AccessLevelDescription: "Developers + Maintainers"},
		},
		UnprotectAccessLevels:     []AccessLevelOutput{},
		AllowForcePush:            false,
		CodeOwnerApprovalRequired: true,
	})

	want := "## Protected Branch: main\n\n" +
		"- **ID**: 1\n" +
		"- **Allow Force Push**: ❌\n" +
		"- **Code Owner Approval Required**: ✅\n" +
		"\n### Push Access Levels\n\n" +
		accessHeader +
		"| 10 | Maintainer |  | Maintainers |\n" +
		"\n### Merge Access Levels\n\n" +
		accessHeader +
		"| 11 | Developer |  | Developers + Maintainers |\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_GranularRules pins what the grantee column was added
// for: a rule naming a user, a group, a custom member role or a deploy key says
// so, and its Level reads "-" rather than the role number GitLab echoes beside
// it, which is not what the rule grants. A custom role carries GitLab's name
// for it, escaped for the cell, when the answer held one.
func TestFormatOutputMarkdown_GranularRules(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:   7,
		Name: "release/*",
		PushAccessLevels: []AccessLevelOutput{
			{ID: 20, AccessLevel: 40, AccessLevelDescription: "Sam Bauch", UserID: 123},
			{ID: 21, AccessLevel: 40, AccessLevelDescription: "Release managers", GroupID: 55},
			{ID: 22, AccessLevel: 40, AccessLevelDescription: "deploy-bot", DeployKeyID: 9},
			{ID: 24, AccessLevel: 30, AccessLevelDescription: "Release role", MemberRoleID: 42},
			{ID: 25, AccessLevel: 30, AccessLevelDescription: "Lead developers", MemberRoleID: 43, MemberRoleName: "Lead | Dev"},
			{ID: 23, AccessLevel: 0, AccessLevelDescription: "No one"},
		},
		AllowForcePush: true,
	})

	want := "## Protected Branch: release/*\n\n" +
		"- **ID**: 7\n" +
		"- **Allow Force Push**: ✅\n" +
		"- **Code Owner Approval Required**: ❌\n" +
		"\n### Push Access Levels\n\n" +
		accessHeader +
		"| 20 | - | user #123 | Sam Bauch |\n" +
		"| 21 | - | group #55 | Release managers |\n" +
		"| 22 | - | deploy key #9 | deploy-bot |\n" +
		"| 24 | - | custom role #42 | Release role |\n" +
		"| 25 | - | custom role Lead &#124; Dev #43 | Lead developers |\n" +
		"| 23 | No access |  | No one |\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_NoAccessLevels pins the card of a branch with no
// rules at all: three fields and no empty section under them.
func TestFormatOutputMarkdown_NoAccessLevels(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                        2,
		Name:                      "release/*",
		PushAccessLevels:          []AccessLevelOutput{},
		MergeAccessLevels:         []AccessLevelOutput{},
		UnprotectAccessLevels:     []AccessLevelOutput{},
		AllowForcePush:            true,
		CodeOwnerApprovalRequired: false,
	})

	want := "## Protected Branch: release/*\n\n" +
		"- **ID**: 2\n" +
		"- **Allow Force Push**: ✅\n" +
		"- **Code Owner Approval Required**: ❌\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_WithBranches pins the whole listing: the heading
// counting what GitLab reported, the table opening a block of its own rather
// than continuing a hint bullet, and one guidance section at the end.
func TestFormatListMarkdown_WithBranches(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Branches: []Output{
			{ID: 1, Name: "main", AllowForcePush: false, CodeOwnerApprovalRequired: true},
			{ID: 2, Name: "release/*", AllowForcePush: true, CodeOwnerApprovalRequired: false},
		},
		Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1},
	})

	want := "## Group Protected Branches (2)\n\n" +
		"| ID | Name | Force Push | Code Owner |\n| --- | --- | --- | --- |\n" +
		"| 1 | main | ❌ | ✅ |\n" +
		"| 2 | release/* | ✅ | ❌ |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins that a group with no protected branches
// renders the one sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Branches: []Output{}})

	if want := "No group protected branches found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
