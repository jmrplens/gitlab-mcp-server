package toolutil

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// MemberUserOutput mirrors gl.MemberCreatedBy (the created_by object).
type MemberUserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// NewMemberUserOutput mirrors a gl.MemberCreatedBy into the shared output
// shape, returning nil when the SDK value is nil.
func NewMemberUserOutput(u *gl.MemberCreatedBy) *MemberUserOutput {
	if u == nil {
		return nil
	}
	return &MemberUserOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// SAMLIdentityOutput mirrors gl.GroupMemberSAMLIdentity (the
// group_saml_identity object).
type SAMLIdentityOutput struct {
	ExternUID      string `json:"extern_uid"`
	Provider       string `json:"provider"`
	SAMLProviderID int64  `json:"saml_provider_id"`
}

// NewSAMLIdentityOutput mirrors a gl.GroupMemberSAMLIdentity into the shared
// output shape, returning nil when the SDK value is nil.
func NewSAMLIdentityOutput(s *gl.GroupMemberSAMLIdentity) *SAMLIdentityOutput {
	if s == nil {
		return nil
	}
	return &SAMLIdentityOutput{
		ExternUID:      s.ExternUID,
		Provider:       s.Provider,
		SAMLProviderID: s.SAMLProviderID,
	}
}

// MemberRoleOutput mirrors gl.MemberRole (the member_role object). Custom
// member roles are an Enterprise (Premium/Ultimate) feature; the object is nil
// on instances or members without a custom role. All permission flags are
// surfaced for 1:1 SDK fidelity.
type MemberRoleOutput struct {
	ID                         int64  `json:"id"`
	Name                       string `json:"name"`
	Description                string `json:"description,omitempty"`
	GroupID                    int64  `json:"group_id"`
	BaseAccessLevel            int    `json:"base_access_level"`
	AdminCICDVariables         bool   `json:"admin_cicd_variables,omitempty"`
	AdminComplianceFramework   bool   `json:"admin_compliance_framework,omitempty"`
	AdminGroupMembers          bool   `json:"admin_group_member,omitempty"`
	AdminMergeRequests         bool   `json:"admin_merge_request,omitempty"`
	AdminPushRules             bool   `json:"admin_push_rules,omitempty"`
	AdminTerraformState        bool   `json:"admin_terraform_state,omitempty"`
	AdminVulnerability         bool   `json:"admin_vulnerability,omitempty"`
	AdminWebHook               bool   `json:"admin_web_hook,omitempty"`
	ArchiveProject             bool   `json:"archive_project,omitempty"`
	ManageDeployTokens         bool   `json:"manage_deploy_tokens,omitempty"`
	ManageGroupAccessTokens    bool   `json:"manage_group_access_tokens,omitempty"`
	ManageMergeRequestSettings bool   `json:"manage_merge_request_settings,omitempty"`
	ManageProjectAccessTokens  bool   `json:"manage_project_access_tokens,omitempty"`
	ManageSecurityPolicyLink   bool   `json:"manage_security_policy_link,omitempty"`
	ReadCode                   bool   `json:"read_code,omitempty"`
	ReadRunners                bool   `json:"read_runners,omitempty"`
	ReadDependency             bool   `json:"read_dependency,omitempty"`
	ReadVulnerability          bool   `json:"read_vulnerability,omitempty"`
	RemoveGroup                bool   `json:"remove_group,omitempty"`
	RemoveProject              bool   `json:"remove_project,omitempty"`
}

// NewMemberRoleOutput mirrors a gl.MemberRole into the shared output shape,
// returning nil when the SDK value is nil.
func NewMemberRoleOutput(r *gl.MemberRole) *MemberRoleOutput {
	if r == nil {
		return nil
	}
	return &MemberRoleOutput{
		ID:                         r.ID,
		Name:                       r.Name,
		Description:                r.Description,
		GroupID:                    r.GroupID,
		BaseAccessLevel:            int(r.BaseAccessLevel),
		AdminCICDVariables:         r.AdminCICDVariables,
		AdminComplianceFramework:   r.AdminComplianceFramework,
		AdminGroupMembers:          r.AdminGroupMembers,
		AdminMergeRequests:         r.AdminMergeRequests,
		AdminPushRules:             r.AdminPushRules,
		AdminTerraformState:        r.AdminTerraformState,
		AdminVulnerability:         r.AdminVulnerability,
		AdminWebHook:               r.AdminWebHook,
		ArchiveProject:             r.ArchiveProject,
		ManageDeployTokens:         r.ManageDeployTokens,
		ManageGroupAccessTokens:    r.ManageGroupAccessTokens,
		ManageMergeRequestSettings: r.ManageMergeRequestSettings,
		ManageProjectAccessTokens:  r.ManageProjectAccessTokens,
		ManageSecurityPolicyLink:   r.ManageSecurityPolicyLink,
		ReadCode:                   r.ReadCode,
		ReadRunners:                r.ReadRunners,
		ReadDependency:             r.ReadDependency,
		ReadVulnerability:          r.ReadVulnerability,
		RemoveGroup:                r.RemoveGroup,
		RemoveProject:              r.RemoveProject,
	}
}
