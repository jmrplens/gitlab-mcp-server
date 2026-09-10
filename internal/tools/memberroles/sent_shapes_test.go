// sent_shapes_test.go covers the reader that takes off a captured response the
// customizable permissions API::Entities::MemberRole sends and client-go's
// MemberRole does not model, and the ways that reader refuses an answer it
// cannot hold.
package memberroles

import (
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// roleCaptureBody is one member role carrying all twenty-five keys, with
// alternating values so a reader that mapped two fields to one key would show
// the wrong answer rather than the same one twice.
const roleCaptureBody = `{
	"id": 3,
	"admin_ai_catalog_item": true,
	"admin_ai_catalog_item_consumer": false,
	"admin_integrations": true,
	"admin_protected_branch": false,
	"admin_protected_environments": true,
	"admin_runners": false,
	"admin_security_attributes": true,
	"apply_security_scan_profiles": false,
	"create_security_scan_profiles": true,
	"delete_security_scan_profiles": false,
	"destroy_package": true,
	"read_admin_cicd": false,
	"read_admin_groups": true,
	"read_admin_monitoring": false,
	"read_admin_projects": true,
	"read_admin_subscription": false,
	"read_admin_users": true,
	"read_agent_artifacts": false,
	"read_compliance_dashboard": true,
	"read_crm_contact": false,
	"read_security_attribute": true,
	"read_security_scan_profiles": false,
	"read_virtual_registry": true,
	"update_sec_ai_workflow_settings": false,
	"update_security_scan_profiles": true
}`

// capturedRolePermissions pairs each key with the field it must have landed
// on, so one table drives both the reader test and the handler test.
func capturedRolePermissions(extra roleExtra) map[string]*bool {
	return map[string]*bool{
		"admin_ai_catalog_item":           extra.AdminAICatalogItem,
		"admin_ai_catalog_item_consumer":  extra.AdminAICatalogItemConsumer,
		"admin_integrations":              extra.AdminIntegrations,
		"admin_protected_branch":          extra.AdminProtectedBranch,
		"admin_protected_environments":    extra.AdminProtectedEnvironments,
		"admin_runners":                   extra.AdminRunners,
		"admin_security_attributes":       extra.AdminSecurityAttributes,
		"apply_security_scan_profiles":    extra.ApplySecurityScanProfiles,
		"create_security_scan_profiles":   extra.CreateSecurityScanProfiles,
		"delete_security_scan_profiles":   extra.DeleteSecurityScanProfiles,
		"destroy_package":                 extra.DestroyPackage,
		"read_admin_cicd":                 extra.ReadAdminCICD,
		"read_admin_groups":               extra.ReadAdminGroups,
		"read_admin_monitoring":           extra.ReadAdminMonitoring,
		"read_admin_projects":             extra.ReadAdminProjects,
		"read_admin_subscription":         extra.ReadAdminSubscription,
		"read_admin_users":                extra.ReadAdminUsers,
		"read_agent_artifacts":            extra.ReadAgentArtifacts,
		"read_compliance_dashboard":       extra.ReadComplianceDashboard,
		"read_crm_contact":                extra.ReadCRMContact,
		"read_security_attribute":         extra.ReadSecurityAttribute,
		"read_security_scan_profiles":     extra.ReadSecurityScanProfiles,
		"read_virtual_registry":           extra.ReadVirtualRegistry,
		"update_sec_ai_workflow_settings": extra.UpdateSecAIWorkflowSettings,
		"update_security_scan_profiles":   extra.UpdateSecurityScanProfiles,
	}
}

// wantRolePermissions is what roleCaptureBody says each key is, in the same
// alternating order the body writes them.
var wantRolePermissions = map[string]bool{ //nolint:gochecknoglobals // the expectation table two tests share
	"admin_ai_catalog_item":           true,
	"admin_ai_catalog_item_consumer":  false,
	"admin_integrations":              true,
	"admin_protected_branch":          false,
	"admin_protected_environments":    true,
	"admin_runners":                   false,
	"admin_security_attributes":       true,
	"apply_security_scan_profiles":    false,
	"create_security_scan_profiles":   true,
	"delete_security_scan_profiles":   false,
	"destroy_package":                 true,
	"read_admin_cicd":                 false,
	"read_admin_groups":               true,
	"read_admin_monitoring":           false,
	"read_admin_projects":             true,
	"read_admin_subscription":         false,
	"read_admin_users":                true,
	"read_agent_artifacts":            false,
	"read_compliance_dashboard":       true,
	"read_crm_contact":                false,
	"read_security_attribute":         true,
	"read_security_scan_profiles":     false,
	"read_virtual_registry":           true,
	"update_sec_ai_workflow_settings": false,
	"update_security_scan_profiles":   true,
}

// TestCapturedRole_ReadsEveryPermissionTheEntitySends drives the reader over a
// body carrying all twenty-five keys and checks each lands on its own field
// with the value the body gave it.
func TestCapturedRole_ReadsEveryPermissionTheEntitySends(t *testing.T) {
	extra, err := capturedRole(gitlabclient.CapturedBody([]byte(roleCaptureBody)))
	if err != nil {
		t.Fatalf("capturedRole() error: %v", err)
	}
	got := capturedRolePermissions(extra)
	if len(got) != len(wantRolePermissions) {
		t.Fatalf("the reader exposes %d permissions, the expectation table names %d", len(got), len(wantRolePermissions))
	}
	for key, want := range wantRolePermissions {
		t.Run(key, func(t *testing.T) {
			value := got[key]
			if value == nil {
				t.Fatalf("%s was not read off the capture", key)
			}
			if *value != want {
				t.Errorf("%s = %v, want %v", key, *value, want)
			}
		})
	}
}

// TestCapturedRole_AbsentPermissionStaysNil covers the other side of the
// pointer: an instance older than one of these permissions sends no key for
// it, and the field must stay nil rather than say the permission was denied.
func TestCapturedRole_AbsentPermissionStaysNil(t *testing.T) {
	extra, err := capturedRole(gitlabclient.CapturedBody([]byte(`{"id": 3}`)))
	if err != nil {
		t.Fatalf("capturedRole() error: %v", err)
	}
	for key, value := range capturedRolePermissions(extra) {
		t.Run(key, func(t *testing.T) {
			if value != nil {
				t.Errorf("%s = %v, want nil for a key the instance did not send", key, *value)
			}
		})
	}
}

// TestCapturedRole_RefusesAnAnswerItCannotHold covers the single-role reader's
// refusals: a body that is not an object, and a permission typed as something
// a boolean field cannot hold.
func TestCapturedRole_RefusesAnAnswerItCannotHold(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"a list where GitLab sends an object", `[{"id": 3}]`},
		{"a permission that is not a boolean", `{"read_admin_users": "yes"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := capturedRole(gitlabclient.CapturedBody([]byte(tc.body))); err == nil {
				t.Errorf("capturedRole() accepted %s", tc.name)
			}
		})
	}
	if _, err := capturedRole(&gitlabclient.ResponseCapture{}); err == nil {
		t.Error("capturedRole() accepted a capture that saw no response")
	}
}

// TestCapturedRoles_ListReaderHoldsTheCountAndTheOrder covers the list reader:
// one extra per role in the list's order, a length that must equal the SDK's,
// and the two bodies it cannot hold.
func TestCapturedRoles_ListReaderHoldsTheCountAndTheOrder(t *testing.T) {
	body := []byte(`[{"read_admin_users": true}, {"read_admin_users": false}]`)
	extras, err := capturedRoles(gitlabclient.CapturedBody(body), 2)
	if err != nil {
		t.Fatalf("capturedRoles() error: %v", err)
	}
	if len(extras) != 2 || extras[0].ReadAdminUsers == nil || !*extras[0].ReadAdminUsers ||
		extras[1].ReadAdminUsers == nil || *extras[1].ReadAdminUsers {
		t.Fatalf("capturedRoles() = %+v, want the two roles in order", extras)
	}

	cases := []struct {
		name    string
		body    string
		decoded int
		want    string
	}{
		{"count disagrees with the SDK", `[{"id": 1}, {"id": 2}]`, 1, "holds 2 member roles and the SDK decoded 1"},
		{"object where GitLab sends a list", `{"id": 1}`, 1, ""},
		{"a permission that is not a boolean", `[{"read_admin_users": 3}]`, 1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, refused := capturedRoles(gitlabclient.CapturedBody([]byte(tc.body)), tc.decoded)
			if refused == nil {
				t.Fatalf("capturedRoles() accepted %s", tc.name)
			}
			if tc.want != "" && !strings.Contains(refused.Error(), tc.want) {
				t.Errorf("capturedRoles() error = %v, want it to name both counts", refused)
			}
		})
	}

	if _, unseen := capturedRoles(&gitlabclient.ResponseCapture{}, 0); unseen == nil {
		t.Error("capturedRoles() accepted a capture that saw no response")
	}
}
