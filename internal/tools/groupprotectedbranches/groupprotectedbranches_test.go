// Package groupprotectedbranches tests validate the MCP tool handlers for
// GitLab group-level protected branch operations. Tests cover List, Get,
// Protect, Update, and Unprotect handlers including success paths, input
// validation, API errors, context cancellation, access level options,
// branch permissions, pagination, and Markdown formatters.
package groupprotectedbranches

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	pathGroupProtBranches = "/api/v4/groups/mygroup/protected_branches"
	pathGroupProtBranch   = "/api/v4/groups/mygroup/protected_branches/main"
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

// TestList_ContextCancelled verifies List returns an error when the
// context is cancelled before the API call.
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
			check: func(t *testing.T, out Output) {
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
				if len(out.PushAccessLevels) != 1 {
					t.Fatalf("len(PushAccessLevels) = %d, want 1", len(out.PushAccessLevels))
				}
				if out.PushAccessLevels[0].AccessLevel != 40 {
					t.Errorf("PushAccessLevels[0].AccessLevel = %d, want 40", out.PushAccessLevels[0].AccessLevel)
				}
				if out.PushAccessLevels[0].AccessLevelDescription != "Maintainers" {
					t.Errorf("PushAccessLevels[0].Description = %q, want %q", out.PushAccessLevels[0].AccessLevelDescription, "Maintainers")
				}
				if len(out.MergeAccessLevels) != 1 {
					t.Fatalf("len(MergeAccessLevels) = %d, want 1", len(out.MergeAccessLevels))
				}
				if len(out.UnprotectAccessLevels) != 0 {
					t.Errorf("len(UnprotectAccessLevels) = %d, want 0", len(out.UnprotectAccessLevels))
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

// TestGet_ContextCancelled verifies Get returns an error when the context
// is cancelled before the API call.
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

// TestProtect_ContextCancelled verifies Protect returns an error when the
// context is cancelled before the API call.
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

// TestUpdate_ContextCancelled verifies Update returns an error when the
// context is cancelled before the API call.
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

// TestUnprotect_ContextCancelled verifies Unprotect returns an error when
// the context is cancelled before the API call.
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
		deployKeyID := int64(99)
		permID := int64(7)
		destroy := true

		got := toBranchPermissions([]BranchPermissionInput{
			{
				ID:          &permID,
				AccessLevel: &lvl,
				UserID:      &userID,
				GroupID:     &groupID,
				DeployKeyID: &deployKeyID,
				Destroy:     &destroy,
			},
		})
		if got == nil {
			t.Fatal("toBranchPermissions returned nil for non-empty input")
		}
		perms := *got
		if len(perms) != 1 {
			t.Fatalf("len(perms) = %d, want 1", len(perms))
		}
		p := perms[0]
		if p.ID == nil || *p.ID != 7 {
			t.Errorf("ID = %v, want 7", p.ID)
		}
		if p.UserID == nil || *p.UserID != 5 {
			t.Errorf("UserID = %v, want 5", p.UserID)
		}
		if p.GroupID == nil || *p.GroupID != 10 {
			t.Errorf("GroupID = %v, want 10", p.GroupID)
		}
		if p.DeployKeyID == nil || *p.DeployKeyID != 99 {
			t.Errorf("DeployKeyID = %v, want 99", p.DeployKeyID)
		}
		if p.Destroy == nil || !*p.Destroy {
			t.Errorf("Destroy = %v, want true", p.Destroy)
		}
		if p.AccessLevel == nil {
			t.Fatal("AccessLevel = nil, want non-nil")
		}
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

// TestFormatOutputMarkdown validates the Markdown formatter for a single
// group protected branch including access level tables and hint sections.
func TestFormatOutputMarkdown(t *testing.T) {
	t.Run("renders branch with access levels", func(t *testing.T) {
		out := Output{
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
		}
		md := FormatOutputMarkdown(out)

		checks := []string{
			"## Protected Branch: main",
			"**Allow Force Push**: false",
			"**Code Owner Approval Required**: true",
			"### Push Access Levels",
			"| 10 | 40 | Maintainers |",
			"### Merge Access Levels",
			"| 11 | 30 | Developers + Maintainers |",
			"gitlab_group_protected_branch_update",
			"gitlab_group_protected_branch_unprotect",
		}
		for _, want := range checks {
			if !strings.Contains(md, want) {
				t.Errorf("FormatOutputMarkdown missing %q", want)
			}
		}
		if strings.Contains(md, "### Unprotect Access Levels") {
			t.Error("FormatOutputMarkdown should not render empty Unprotect Access Levels section")
		}
	})

	t.Run("renders branch without access levels", func(t *testing.T) {
		out := Output{
			ID:                        2,
			Name:                      "release/*",
			PushAccessLevels:          []AccessLevelOutput{},
			MergeAccessLevels:         []AccessLevelOutput{},
			UnprotectAccessLevels:     []AccessLevelOutput{},
			AllowForcePush:            true,
			CodeOwnerApprovalRequired: false,
		}
		md := FormatOutputMarkdown(out)

		if !strings.Contains(md, "## Protected Branch: release/*") {
			t.Error("missing heading")
		}
		if !strings.Contains(md, "**Allow Force Push**: true") {
			t.Error("missing force push")
		}
		for _, heading := range []string{"### Push Access Levels", "### Merge Access Levels", "### Unprotect Access Levels"} {
			if strings.Contains(md, heading) {
				t.Errorf("should not render empty section %q", heading)
			}
		}
	})
}

// TestFormatListMarkdown validates the Markdown formatter for a paginated
// list of group protected branches including the empty-result case.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("renders table with branches", func(t *testing.T) {
		out := ListOutput{
			Branches: []Output{
				{ID: 1, Name: "main", AllowForcePush: false, CodeOwnerApprovalRequired: true},
				{ID: 2, Name: "release/*", AllowForcePush: true, CodeOwnerApprovalRequired: false},
			},
			Pagination: toolutil.PaginationOutput{Page: 1, PerPage: 20, TotalItems: 2, TotalPages: 1},
		}
		md := FormatListMarkdown(out)

		checks := []string{
			"| ID | Name | Force Push | Code Owner |",
			"| 1 | main | false | true |",
			"| 2 | release/* | true | false |",
			"gitlab_group_protected_branch_get",
			"gitlab_group_protected_branch_protect",
		}
		for _, want := range checks {
			if !strings.Contains(md, want) {
				t.Errorf("FormatListMarkdown missing %q", want)
			}
		}
	})

	t.Run("renders empty message when no branches", func(t *testing.T) {
		out := ListOutput{Branches: []Output{}}
		md := FormatListMarkdown(out)
		want := "No group protected branches found."
		if !strings.Contains(md, want) {
			t.Errorf("FormatListMarkdown = %q, want to contain %q", md, want)
		}
	})
}
