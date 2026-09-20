// member_roles_test.go contains unit tests for GitLab member role operations.
// Tests use httptest to mock the GitLab Member Roles API.
package memberroles

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const memberRoleJSON = `{"id":1,"name":"custom-dev","description":"Custom developer","group_id":100,"base_access_level":30,"read_code":true,"admin_merge_request":false}`

// TestListInstance_Success verifies ListInstance returns the role list when
// GET /member_roles responds 200 with a single custom role.
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

// TestMemberRoleHandlers_PublishTheCapturedPermissions verifies that the
// twenty-five permissions API::Entities::MemberRole sends and client-go's
// MemberRole does not model reach the caller through each of the four routes
// that answer with a role, off the same bytes the SDK decoded.
//
// All four are driven from one table because the reader is the same on each
// and the interesting failure is a handler that forgot to wrap its context: a
// call made without gitlabclient.WithResponseCapture reaches no capture and
// publishes nothing, which no test of the reader alone can see.
func TestMemberRoleHandlers_PublishTheCapturedPermissions(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		method string
		list   bool
		call   func(context.Context, *gitlabclient.Client) (Output, error)
	}{
		{
			name: "list instance", path: "/api/v4/member_roles", method: http.MethodGet, list: true,
			call: func(ctx context.Context, c *gitlabclient.Client) (Output, error) {
				out, err := ListInstance(ctx, c, ListInstanceInput{})
				return firstRole(out), err
			},
		},
		{
			name: "list group", path: "/api/v4/groups/100/member_roles", method: http.MethodGet, list: true,
			call: func(ctx context.Context, c *gitlabclient.Client) (Output, error) {
				out, err := ListGroup(ctx, c, ListGroupInput{GroupID: "100"})
				return firstRole(out), err
			},
		},
		{
			name: "create instance", path: "/api/v4/member_roles", method: http.MethodPost,
			call: func(ctx context.Context, c *gitlabclient.Client) (Output, error) {
				return CreateInstance(ctx, c, CreateInstanceInput{Name: "custom-dev", BaseAccessLevel: 30})
			},
		},
		{
			name: "create group", path: "/api/v4/groups/100/member_roles", method: http.MethodPost,
			call: func(ctx context.Context, c *gitlabclient.Client) (Output, error) {
				return CreateGroup(ctx, c, CreateGroupInput{GroupID: "100", Name: "custom-dev", BaseAccessLevel: 30})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := roleCaptureBody
			if tc.list {
				body = "[" + roleCaptureBody + "]"
			}
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == tc.method && r.URL.Path == tc.path {
					testutil.RespondJSON(w, http.StatusOK, body)
					return
				}
				http.NotFound(w, r)
			}))

			got, err := tc.call(context.Background(), client)
			if err != nil {
				t.Fatalf("%s error: %v", tc.name, err)
			}
			published := capturedOutputPermissions(got)
			for key, want := range wantRolePermissions {
				t.Run(key, func(t *testing.T) {
					value := published[key]
					if value == nil {
						t.Fatalf("%s reached no published field", key)
					}
					if *value != want {
						t.Errorf("%s = %v, want it published as %v", key, *value, want)
					}
				})
			}
		})
	}
}

// firstRole takes the one role a list answer carries, or a zero role so the
// caller's own assertions report the miss.
func firstRole(out ListOutput) Output {
	if len(out.Roles) == 0 {
		return Output{}
	}
	return out.Roles[0]
}

// The two halves of a member role's permissions, counted so a field added to
// one of them and left out of the tables below is a failure rather than a
// permission nothing drives: twenty client-go's MemberRole models and decodes,
// and twenty-five read off the captured response beside it.
const (
	sdkRolePermissionCount      = 20
	capturedRolePermissionCount = 25
)

// sdkOutputPermissions pairs the key client-go's MemberRole reads a permission
// from with the published field [toOutput] must copy it onto. The key is the
// one GitLab sends, which the SDK struct and [Permissions] spell alike, so the
// table says what the whole chain has to do with each value and not merely
// that some field carries it.
func sdkOutputPermissions(out Output) map[string]*bool {
	return map[string]*bool{
		"admin_cicd_variables":          out.AdminCICDVariables,
		"admin_compliance_framework":    out.AdminComplianceFramework,
		"admin_group_member":            out.AdminGroupMembers,
		"admin_merge_request":           out.AdminMergeRequests,
		"admin_push_rules":              out.AdminPushRules,
		"admin_terraform_state":         out.AdminTerraformState,
		"admin_vulnerability":           out.AdminVulnerability,
		"admin_web_hook":                out.AdminWebHook,
		"archive_project":               out.ArchiveProject,
		"manage_deploy_tokens":          out.ManageDeployTokens,
		"manage_group_access_tokens":    out.ManageGroupAccessTokens,
		"manage_merge_request_settings": out.ManageMergeRequestSettings,
		"manage_project_access_tokens":  out.ManageProjectAccessTokens,
		"manage_security_policy_link":   out.ManageSecurityPolicyLink,
		"read_code":                     out.ReadCode,
		"read_runners":                  out.ReadRunners,
		"read_dependency":               out.ReadDependency,
		"read_vulnerability":            out.ReadVulnerability,
		"remove_group":                  out.RemoveGroup,
		"remove_project":                out.RemoveProject,
	}
}

// capturedOutputPermissions is the same pairing for the twenty-five the
// capture reads, taken off the published role rather than off the reader's own
// [roleExtra], so it answers what [withCapturedPermissions] placed where.
func capturedOutputPermissions(out Output) map[string]*bool {
	return map[string]*bool{
		"admin_ai_catalog_item":           out.AdminAICatalogItem,
		"admin_ai_catalog_item_consumer":  out.AdminAICatalogItemConsumer,
		"admin_integrations":              out.AdminIntegrations,
		"admin_protected_branch":          out.AdminProtectedBranch,
		"admin_protected_environments":    out.AdminProtectedEnvironments,
		"admin_runners":                   out.AdminRunners,
		"admin_security_attributes":       out.AdminSecurityAttributes,
		"apply_security_scan_profiles":    out.ApplySecurityScanProfiles,
		"create_security_scan_profiles":   out.CreateSecurityScanProfiles,
		"delete_security_scan_profiles":   out.DeleteSecurityScanProfiles,
		"destroy_package":                 out.DestroyPackage,
		"read_admin_cicd":                 out.ReadAdminCICD,
		"read_admin_groups":               out.ReadAdminGroups,
		"read_admin_monitoring":           out.ReadAdminMonitoring,
		"read_admin_projects":             out.ReadAdminProjects,
		"read_admin_subscription":         out.ReadAdminSubscription,
		"read_admin_users":                out.ReadAdminUsers,
		"read_agent_artifacts":            out.ReadAgentArtifacts,
		"read_compliance_dashboard":       out.ReadComplianceDashboard,
		"read_crm_contact":                out.ReadCRMContact,
		"read_security_attribute":         out.ReadSecurityAttribute,
		"read_security_scan_profiles":     out.ReadSecurityScanProfiles,
		"read_virtual_registry":           out.ReadVirtualRegistry,
		"update_sec_ai_workflow_settings": out.UpdateSecAIWorkflowSettings,
		"update_security_scan_profiles":   out.UpdateSecurityScanProfiles,
	}
}

// outputRolePermissions is both halves at once: every permission key a member
// role answer can carry, against the field the published role must show it on.
func outputRolePermissions(out Output) map[string]*bool {
	all := sdkOutputPermissions(out)
	maps.Copy(all, capturedOutputPermissions(out))
	return all
}

// oneHotRoleBody is a member role answer where the named permission is the
// only true one and every other of the forty-five is false.
func oneHotRoleBody(on string) string {
	var body strings.Builder
	body.WriteString(`{"id":10,"name":"one-hot","base_access_level":30`)
	for _, key := range slices.Sorted(maps.Keys(outputRolePermissions(Output{}))) {
		fmt.Fprintf(&body, `,%q:%t`, key, key == on)
	}
	body.WriteString("}")
	return body.String()
}

// TestMemberRoleOutput_HoldsEachPermissionToItsOwnWireKey drives one answer per
// permission, each carrying that key as the only true one, and holds the
// published role to showing true on that permission's field and false on the
// other forty-four.
//
// Every one of the forty-five is copied by a straight-line assignment no branch
// guards, twenty in the [toOutput] literal and twenty-five in
// [withCapturedPermissions], so the only thing a test can catch there is a copy
// that takes the field beside it. An answer that gives several permissions the
// same value cannot: exchanging two assignments whose sources agree leaves both
// reads unchanged, which is as true of a fixture alternating true and false,
// where every same-parity pair agrees, as of one setting them all to true.
// Driving the keys one at a time makes every pair disagree in one of the runs,
// and reading all forty-five back on each run means a copy that dropped its
// source or took another's is seen wherever it lands.
func TestMemberRoleOutput_HoldsEachPermissionToItsOwnWireKey(t *testing.T) {
	keys := slices.Sorted(maps.Keys(outputRolePermissions(Output{})))
	if want := sdkRolePermissionCount + capturedRolePermissionCount; len(keys) != want {
		t.Fatalf("the permission table names %d keys, want the %d a member role answer carries", len(keys), want)
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			body := oneHotRoleBody(key)
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/v4/member_roles" {
					testutil.RespondJSON(w, http.StatusCreated, body)
					return
				}
				http.NotFound(w, r)
			}))

			got, err := CreateInstance(context.Background(), client, CreateInstanceInput{Name: "one-hot", BaseAccessLevel: 30})
			if err != nil {
				t.Fatalf("CreateInstance() error: %v", err)
			}
			for other, value := range outputRolePermissions(got) {
				if value == nil {
					t.Errorf("%s reached no published field when the answer set %s", other, key)
					continue
				}
				if want := other == key; *value != want {
					t.Errorf("%s = %v when the answer set %s alone, want %v", other, *value, key, want)
				}
			}
		})
	}
}

// TestRolePermissionTables_NameTheSameCapturedKeys holds the published-side
// table to the reader-side one, so a permission the capture reads and the
// published table leaves out cannot pass the one-hot drive by never being
// asked about.
func TestRolePermissionTables_NameTheSameCapturedKeys(t *testing.T) {
	published := capturedOutputPermissions(Output{})
	if len(published) != capturedRolePermissionCount {
		t.Fatalf("the published table names %d captured permissions, want %d", len(published), capturedRolePermissionCount)
	}
	for key := range capturedRolePermissions(roleExtra{}) {
		t.Run(key, func(t *testing.T) {
			if _, ok := published[key]; !ok {
				t.Errorf("%s is read off the capture and named in no published-permission table", key)
			}
		})
	}
}

// TestMemberRoleHandlers_CaptureUnreadable verifies each role-returning
// handler reports a body the permission reader cannot hold rather than
// serving a role with the permissions silently missing.
func TestMemberRoleHandlers_CaptureUnreadable(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		method string
		body   string
		call   func(context.Context, *gitlabclient.Client) error
	}{
		{
			name: "list instance", path: "/api/v4/member_roles", method: http.MethodGet,
			body: `[{"id":1,"read_admin_users":"yes"}]`,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListInstance(ctx, c, ListInstanceInput{})
				return err
			},
		},
		{
			name: "list group", path: "/api/v4/groups/100/member_roles", method: http.MethodGet,
			body: `[{"id":1,"read_admin_users":"yes"}]`,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListGroup(ctx, c, ListGroupInput{GroupID: "100"})
				return err
			},
		},
		{
			name: "create instance", path: "/api/v4/member_roles", method: http.MethodPost,
			body: `{"id":1,"read_admin_users":"yes"}`,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := CreateInstance(ctx, c, CreateInstanceInput{Name: "n", BaseAccessLevel: 30})
				return err
			},
		},
		{
			name: "create group", path: "/api/v4/groups/100/member_roles", method: http.MethodPost,
			body: `{"id":1,"read_admin_users":"yes"}`,
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := CreateGroup(ctx, c, CreateGroupInput{GroupID: "100", Name: "n", BaseAccessLevel: 30})
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == tc.method && r.URL.Path == tc.path {
					testutil.RespondJSON(w, http.StatusOK, tc.body)
					return
				}
				http.NotFound(w, r)
			}))
			if err := tc.call(context.Background(), client); err == nil {
				t.Errorf("%s accepted a captured body the permission reader cannot hold", tc.name)
			}
		})
	}
}

// TestListInstance_CancelledContext verifies ListInstance returns an error when
// invoked with an already-cancelled context, without contacting the API.
func TestListInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListInstance(ctx, client, ListInstanceInput{})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListInstance_APIError verifies ListInstance propagates an error when
// GET /member_roles responds 403 Forbidden.
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

// TestListGroup_Success verifies ListGroup returns the role list when
// GET /groups/:id/member_roles responds 200 with one role.
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

// TestListGroup_MissingGroupID verifies ListGroup returns a validation error
// when group_id is empty, without hitting the API.
func TestListGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestListGroup_CancelledContext verifies ListGroup returns an error when
// invoked with an already-cancelled context.
func TestListGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := ListGroup(ctx, client, ListGroupInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestListGroup_APIError verifies ListGroup propagates an error when
// GET /groups/:id/member_roles responds 400 Bad Request.
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
		t.Fatal("expected error for 400 response, got nil")
	}
}

// TestListGroup_SelfManagedDeprecationHint verifies 400 responses explain
// self-managed group role deprecation and point to instance-level roles.
func TestListGroup_SelfManagedDeprecationHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Group-level custom roles are deprecated on self-managed instances"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: toolutil.StringOrInt("mygroup")})
	assertGroupMemberRoleDeprecationHint(t, err)
}

// TestListGroup_ForbiddenIncludesAccessHint verifies non-400 failures use the group access guidance.
func TestListGroup_ForbiddenIncludesAccessHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := ListGroup(context.Background(), client, ListGroupInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
	if !strings.Contains(err.Error(), "requires Owner role") {
		t.Fatalf("error missing group access guidance: %v", err)
	}
}

// TestCreateInstance_Success verifies CreateInstance returns the new role when
// POST /member_roles responds 201 Created.
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

// TestCreateInstance_MissingName verifies CreateInstance returns a validation
// error when the name field is empty, without hitting the API.
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

// TestCreateInstance_MissingBaseAccessLevel verifies CreateInstance returns a
// validation error when base_access_level is zero.
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

// TestCreateInstance_CancelledContext verifies CreateInstance returns an error
// when invoked with an already-cancelled context.
func TestCreateInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := CreateInstance(ctx, client, CreateInstanceInput{
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateInstance_APIError verifies CreateInstance propagates an error when
// POST /member_roles responds 403 Forbidden.
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

// TestCreateGroup_Success verifies CreateGroup returns the new role when
// POST /groups/:id/member_roles responds 201 Created.
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

// TestCreateGroup_MissingGroupID verifies CreateGroup returns a validation
// error when group_id is empty.
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

// TestCreateGroup_MissingName verifies CreateGroup returns a validation error
// when the name field is empty.
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

// TestCreateGroup_MissingBaseAccessLevel verifies CreateGroup returns a
// validation error when base_access_level is zero.
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

// TestCreateGroup_CancelledContext verifies CreateGroup returns an error when
// invoked with an already-cancelled context.
func TestCreateGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := CreateGroup(ctx, client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestCreateGroup_APIError verifies CreateGroup propagates an error when
// POST /groups/:id/member_roles responds 400 Bad Request.
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
		t.Fatal("expected error for 400 response, got nil")
	}
}

// TestCreateGroup_SelfManagedDeprecationHint verifies create failures on
// self-managed GitLab tell callers to use instance-level member roles.
func TestCreateGroup_SelfManagedDeprecationHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Group-level custom roles are deprecated on self-managed instances"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := CreateGroup(context.Background(), client, CreateGroupInput{
		GroupID:         toolutil.StringOrInt("mygroup"),
		Name:            "custom-dev",
		BaseAccessLevel: 30,
	})
	assertGroupMemberRoleDeprecationHint(t, err)
}

// TestCreateGroup_ForbiddenUsesGenericCreateGuidance verifies non-400 create errors do not use the self-managed deprecation hint.
func TestCreateGroup_ForbiddenUsesGenericCreateGuidance(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
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
		t.Fatal("expected error for 403 response, got nil")
	}
	if strings.Contains(err.Error(), groupMemberRoleSelfManagedHint) {
		t.Fatalf("unexpected self-managed hint for 403 response: %v", err)
	}
}

// TestDeleteInstance_Success verifies DeleteInstance returns no error when
// DELETE /member_roles/:id responds 204 No Content.
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

// TestDeleteInstance_MissingMemberRoleID verifies DeleteInstance returns a
// validation error when member_role_id is zero.
func TestDeleteInstance_MissingMemberRoleID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteInstance(context.Background(), client, DeleteInstanceInput{})
	if err == nil {
		t.Fatal("expected error for zero member_role_id, got nil")
	}
}

// TestDeleteInstance_CancelledContext verifies DeleteInstance returns an error
// when invoked with an already-cancelled context.
func TestDeleteInstance_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := DeleteInstance(ctx, client, DeleteInstanceInput{MemberRoleID: 1})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteInstance_APIError verifies DeleteInstance propagates an error when
// DELETE /member_roles/:id responds 403 Forbidden.
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

// TestDeleteGroup_Success verifies DeleteGroup returns no error when
// DELETE /groups/:id/member_roles/:role_id responds 204 No Content.
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

// TestDeleteGroup_MissingGroupID verifies DeleteGroup returns a validation
// error when group_id is empty.
func TestDeleteGroup_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{MemberRoleID: 1})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

// TestDeleteGroup_MissingMemberRoleID verifies DeleteGroup returns a
// validation error when member_role_id is zero.
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

// TestDeleteGroup_CancelledContext verifies DeleteGroup returns an error when
// invoked with an already-cancelled context.
func TestDeleteGroup_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx := testutil.CancelledCtx(t)

	err := DeleteGroup(ctx, client, DeleteGroupInput{
		GroupID:      toolutil.StringOrInt("mygroup"),
		MemberRoleID: 1,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// TestDeleteGroup_APIError verifies DeleteGroup propagates an error when
// DELETE /groups/:id/member_roles/:role_id responds 400 Bad Request.
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
		t.Fatal("expected error for 400 response, got nil")
	}
}

// TestDeleteGroup_SelfManagedDeprecationHint verifies delete failures on
// self-managed GitLab include the same actionable deprecation guidance.
func TestDeleteGroup_SelfManagedDeprecationHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/member_roles/1" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Group-level custom roles are deprecated on self-managed instances"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteGroup(context.Background(), client, DeleteGroupInput{
		GroupID:      toolutil.StringOrInt("mygroup"),
		MemberRoleID: 1,
	})
	assertGroupMemberRoleDeprecationHint(t, err)
}

func assertGroupMemberRoleDeprecationHint(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"deprecated", "self-managed", "instance-level", "GitLab.com Ultimate"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
}

// TestToOutput_Nil verifies that toOutput returns a zero-value Output when
// given a nil MemberRole pointer, preventing nil-pointer dereferences.
func TestToOutput_Nil(t *testing.T) {
	out := toOutput(nil, roleExtra{})
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
	var capturedBody string
	trueVal := true
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/member_roles")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		capturedBody = string(body)
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
		Name:                       "full-perms",
		BaseAccessLevel:            30,
		Description:                "All permissions",
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
	for _, want := range []string{"name", "base_access_level", "description", "admin_cicd_variables", "admin_merge_request", "read_code", "remove_project", "manage_deploy_tokens", "admin_vulnerability"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing field %q", want)
			}
		})
	}
	for _, want := range []string{`"name":"full-perms"`, `"base_access_level":30`, `"description":"All permissions"`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(capturedBody, want) {
				t.Errorf("request body missing %s; got %s", want, capturedBody)
			}
		})
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
		GroupID:            toolutil.StringOrInt("mygroup"),
		Name:               "group-role",
		BaseAccessLevel:    20,
		Description:        "Group role with perms",
		ReadCode:           &trueVal,
		AdminMergeRequests: &trueVal,
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
