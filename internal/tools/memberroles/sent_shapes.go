package memberroles

import (
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// roleExtra is what ee/lib/api/entities/member_role.rb sends on a member role
// that client-go's MemberRole does not carry, read from the captured response
// beside the SDK's own decode (ADR-0021). The gap is recorded in
// docs/development/upstream-bugs.md.
//
// These are not a subset anybody chose. The entity exposes its permissions by
// looping over `::MemberRole.all_customizable_permissions`, so the names exist
// only in a constant the running application assembles and no scan of the Ruby
// can read them; the committed live record, taken from a booted GitLab, is the
// only oracle that has them. At v19.3.1-ee it lists 45 permission flags where
// the SDK models 20, and every one of the 45 is exposed with `default: false`
// and no condition, so a member role response carries all of them.
//
// Each is a pointer rather than a bool because absent and false are different
// answers here: GitLab adds customizable permissions most releases, and an
// instance older than one of these sends no key for it. A value would tell the
// caller the permission was denied when the instance never had it.
//
// These stay off [Permissions], which is the create-input fragment as well as
// part of the output: `gl.CreateMemberRoleOptions` accepts the same 20 the
// struct models, so putting them there would advertise request parameters the
// SDK cannot send.
type roleExtra struct {
	AdminAICatalogItem          *bool `json:"admin_ai_catalog_item"`
	AdminAICatalogItemConsumer  *bool `json:"admin_ai_catalog_item_consumer"`
	AdminIntegrations           *bool `json:"admin_integrations"`
	AdminProtectedBranch        *bool `json:"admin_protected_branch"`
	AdminProtectedEnvironments  *bool `json:"admin_protected_environments"`
	AdminRunners                *bool `json:"admin_runners"`
	AdminSecurityAttributes     *bool `json:"admin_security_attributes"`
	ApplySecurityScanProfiles   *bool `json:"apply_security_scan_profiles"`
	CreateSecurityScanProfiles  *bool `json:"create_security_scan_profiles"`
	DeleteSecurityScanProfiles  *bool `json:"delete_security_scan_profiles"`
	DestroyPackage              *bool `json:"destroy_package"`
	ReadAdminCICD               *bool `json:"read_admin_cicd"`
	ReadAdminGroups             *bool `json:"read_admin_groups"`
	ReadAdminMonitoring         *bool `json:"read_admin_monitoring"`
	ReadAdminProjects           *bool `json:"read_admin_projects"`
	ReadAdminSubscription       *bool `json:"read_admin_subscription"`
	ReadAdminUsers              *bool `json:"read_admin_users"`
	ReadAgentArtifacts          *bool `json:"read_agent_artifacts"`
	ReadComplianceDashboard     *bool `json:"read_compliance_dashboard"`
	ReadCRMContact              *bool `json:"read_crm_contact"`
	ReadSecurityAttribute       *bool `json:"read_security_attribute"`
	ReadSecurityScanProfiles    *bool `json:"read_security_scan_profiles"`
	ReadVirtualRegistry         *bool `json:"read_virtual_registry"`
	UpdateSecAIWorkflowSettings *bool `json:"update_sec_ai_workflow_settings"`
	UpdateSecurityScanProfiles  *bool `json:"update_security_scan_profiles"`
}

// capturedRole reads the permissions client-go's MemberRole does not model off
// the answer to a request that returned one role.
func capturedRole(capture *gitlabclient.ResponseCapture) (roleExtra, error) {
	var extra roleExtra
	if err := capture.Decode(&extra); err != nil {
		return roleExtra{}, err
	}
	return extra, nil
}

// capturedRoles reads the same off a list answer, one extra per role in the
// list's order.
//
// The count is held to what the SDK decoded: the two read the same bytes, so a
// difference is a fault in this reader rather than in the answer.
func capturedRoles(capture *gitlabclient.ResponseCapture, decoded int) ([]roleExtra, error) {
	var extras []roleExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d member roles and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}
