package memberroles

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const memberRoleJSON = `{"id":1,"name":"custom-dev","description":"Custom developer","group_id":100,"base_access_level":30,"read_code":true,"admin_merge_request":false}`

func TestListInstance_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/member_roles" {
			testutil.RespondJSON(w, http.StatusOK, `[`+memberRoleJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf("ListInstance() error: %v", err)
	}
	if len(out.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(out.Roles))
	}
	if out.Roles[0].Name != "custom-dev" {
		t.Errorf("expected name custom-dev, got %s", out.Roles[0].Name)
	}
	if out.Roles[0].BaseAccessLevel != 30 {
		t.Errorf("expected base_access_level 30, got %d", out.Roles[0].BaseAccessLevel)
	}
}

func TestListInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListInstance(ctx, client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestListInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/member_roles" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestListGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusOK, `[`+memberRoleJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("ListGroup() error: %v", err)
	}
	if len(out.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(out.Roles))
	}
}

func TestListGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestListGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestCreateInstance_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/member_roles" {
			testutil.RespondJSON(w, http.StatusCreated, memberRoleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateInstance(context.Background(), client, CreateInstanceInput{
		Name:            "custom-dev",
		BaseAccessLevel: 30,
		Description:     "Custom developer",
	})
	if err != nil {
		t.Fatalf("CreateInstance() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected id 1, got %d", out.ID)
	}
	if out.Name != "custom-dev" {
		t.Errorf("expected name custom-dev, got %s", out.Name)
	}
}

func TestCreateInstance_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateInstance(context.Background(), client, CreateInstanceInput{
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestCreateInstance_MissingBaseAccessLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateInstance(context.Background(), client, CreateInstanceInput{
		Name: "custom-dev",
	})
	if err == nil {
		t.Fatal("expected error for zero base_access_level, got nil")
	}
}

func TestCreateInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CreateInstance(ctx, client, CreateInstanceInput{
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestCreateInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/member_roles" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := CreateInstance(context.Background(), client, CreateInstanceInput{
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestCreateGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusCreated, memberRoleJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err != nil {
		t.Fatalf("CreateGroup() error: %v", err)
	}
	if out.ID != 1 {
		t.Errorf("expected id 1, got %d", out.ID)
	}
}

func TestCreateGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestCreateGroup_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestCreateGroup_MissingBaseAccessLevel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		Name:    "custom-dev",
	})
	if err == nil {
		t.Fatal("expected error for zero base_access_level, got nil")
	}
}

func TestCreateGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CreateGroup(ctx, client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestCreateGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestDeleteInstance_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/member_roles/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteInstance(context.Background(), client, DeleteInstanceInput{MemberRoleID: 1})
	if err != nil {
		t.Fatalf("DeleteInstance() error: %v", err)
	}
}

func TestDeleteInstance_MissingMemberRoleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteInstance(context.Background(), client, DeleteInstanceInput{})
	if err == nil {
		t.Fatal("expected error for zero member_role_id, got nil")
	}
}

func TestDeleteInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DeleteInstance(ctx, client, DeleteInstanceInput{MemberRoleID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDeleteInstance_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/member_roles/1" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteInstance(context.Background(), client, DeleteInstanceInput{MemberRoleID: 1})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestDeleteGroup_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/mygroup/member_roles/1" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{
		GroupID:      toolutil.StringOrInt("mygroup"),
		MemberRoleID: 1,
	})
	if err != nil {
		t.Fatalf("DeleteGroup() error: %v", err)
	}
}

func TestDeleteGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{MemberRoleID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestDeleteGroup_MissingMemberRoleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for zero member_role_id, got nil")
	}
}

func TestDeleteGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := DeleteGroup(ctx, client, DeleteGroupInput{
		GroupID:      toolutil.StringOrInt("mygroup"),
		MemberRoleID: 1,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDeleteGroup_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles/1" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{
		GroupID:      toolutil.StringOrInt("mygroup"),
		MemberRoleID: 1,
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// TestToOutput_Nil verifies that toOutput returns a zero-value Output when
// given a nil MemberRole pointer, preventing nil-pointer dereferences.
func TestToOutput_Nil(t *testing.T) {
	out := toOutput(nil)
	if out.ID != 0 {
		t.Errorf("expected ID 0 for nil input, got %d", out.ID)
	}
	if out.Name != "" {
		t.Errorf("expected empty name for nil input, got %q", out.Name)
	}
}

// TestCreateInstance_WithAllPermissions verifies that CreateInstance correctly
// forwards all permission flags to the GitLab API. This exercises every branch
// in buildCreateOpts that handles optional permission fields.
func TestCreateInstance_WithAllPermissions(t *testing.T) {
	trueVal := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/member_roles")
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":10,"name":"full-perms","description":"All permissions",
			"base_access_level":30,
			"admin_cicd_variables":true,"admin_compliance_framework":true,
			"admin_group_member":true,"admin_merge_request":true,
			"admin_push_rules":true,"admin_terraform_state":true,
			"admin_vulnerability":true,"admin_web_hook":true,
			"archive_project":true,"manage_deploy_tokens":true,
			"manage_group_access_tokens":true,"manage_merge_request_settings":true,
			"manage_project_access_tokens":true,"manage_security_policy_link":true,
			"read_code":true,"read_runners":true,"read_dependency":true,
			"read_vulnerability":true,"remove_group":true,"remove_project":true
		}`)
	}))

	out, err := CreateInstance(context.Background(), client, CreateInstanceInput{
		Name:            "full-perms",
		BaseAccessLevel: 30,
		Description:     "All permissions",
		Permissions: Permissions{
			AdminCICDVariables:         &trueVal,
			AdminComplianceFramework:   &trueVal,
			AdminGroupMembers:          &trueVal,
			AdminMergeRequests:         &trueVal,
			AdminPushRules:             &trueVal,
			AdminTerraformState:        &trueVal,
			AdminVulnerability:         &trueVal,
			AdminWebHook:               &trueVal,
			ArchiveProject:             &trueVal,
			ManageDeployTokens:         &trueVal,
			ManageGroupAccessTokens:    &trueVal,
			ManageMergeRequestSettings: &trueVal,
			ManageProjectAccessTokens:  &trueVal,
			ManageSecurityPolicyLink:   &trueVal,
			ReadCode:                   &trueVal,
			ReadRunners:                &trueVal,
			ReadDependency:             &trueVal,
			ReadVulnerability:          &trueVal,
			RemoveGroup:                &trueVal,
			RemoveProject:              &trueVal,
		},
	})
	if err != nil {
		t.Fatalf("CreateInstance() error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("expected ID 10, got %d", out.ID)
	}
	if out.Name != "full-perms" {
		t.Errorf("expected name full-perms, got %s", out.Name)
	}
	if out.ReadCode == nil || !*out.ReadCode {
		t.Error("expected ReadCode to be true")
	}
	if out.RemoveProject == nil || !*out.RemoveProject {
		t.Error("expected RemoveProject to be true")
	}
}

// TestCreateGroup_WithDescriptionAndPermissions verifies that CreateGroup
// forwards description and permission flags to the GitLab API, exercising
// permission branch coverage in buildCreateOpts via the group path.
func TestCreateGroup_WithDescriptionAndPermissions(t *testing.T) {
	trueVal := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/groups/mygroup/member_roles")
		testutil.RespondJSON(w, http.StatusCreated, `{
			"id":20,"name":"group-role","description":"Group role with perms",
			"group_id":100,"base_access_level":20,
			"read_code":true,"admin_merge_request":true
		}`)
	}))

	out, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		Name:            "group-role",
		BaseAccessLevel: 20,
		Description:     "Group role with perms",
		Permissions: Permissions{
			ReadCode:           &trueVal,
			AdminMergeRequests: &trueVal,
		},
	})
	if err != nil {
		t.Fatalf("CreateGroup() error: %v", err)
	}
	if out.ID != 20 {
		t.Errorf("expected ID 20, got %d", out.ID)
	}
	if out.GroupID != 100 {
		t.Errorf("expected GroupID 100, got %d", out.GroupID)
	}
	if out.Description != "Group role with perms" {
		t.Errorf("expected description 'Group role with perms', got %q", out.Description)
	}
}

// TestListInstance_EmptyList verifies that ListInstance handles an empty JSON
// array response and returns an empty roles slice (not nil).
func TestListInstance_EmptyList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/member_roles")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf("ListInstance() error: %v", err)
	}
	if len(out.Roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(out.Roles))
	}
}

// TestListGroup_EmptyList verifies that ListGroup handles an empty JSON
// array response for a group and returns an empty roles slice.
func TestListGroup_EmptyList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/groups/42/member_roles")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	out, err := ListGroup(context.Background(), client, ListGroupInput{
		GroupID: toolutil.StringOrInt("42"),
	})
	if err != nil {
		t.Fatalf("ListGroup() error: %v", err)
	}
	if len(out.Roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(out.Roles))
	}
}

// TestListInstance_MultipleRoles verifies that ListInstance correctly parses
// multiple member roles from the API response array.
func TestListInstance_MultipleRoles(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id":1,"name":"role-a","base_access_level":10},
			{"id":2,"name":"role-b","base_access_level":30}
		]`)
	}))

	out, err := ListInstance(context.Background(), client, ListInstanceInput{})
	if err != nil {
		t.Fatalf("ListInstance() error: %v", err)
	}
	if len(out.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(out.Roles))
	}
	if out.Roles[0].Name != "role-a" {
		t.Errorf("first role name = %q, want %q", out.Roles[0].Name, "role-a")
	}
	if out.Roles[1].Name != "role-b" {
		t.Errorf("second role name = %q, want %q", out.Roles[1].Name, "role-b")
	}
}
