// Package memberroles implements GitLab member role operations at instance and
// group level including list, create, and delete.
package memberroles

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInstanceInput holds parameters for listing instance member roles.
type ListInstanceInput struct{}

// ListGroupInput holds parameters for listing group member roles.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// CreateInstanceInput holds parameters for creating an instance member role.
type CreateInstanceInput struct {
	Name            string `json:"name"              jsonschema:"Name of the custom role,required"`
	BaseAccessLevel int    `json:"base_access_level" jsonschema:"Base access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer),required"`
	Description     string `json:"description,omitempty" jsonschema:"Description of the custom role"`
	Permissions
}

// CreateGroupInput holds parameters for creating a group member role.
type CreateGroupInput struct {
	GroupID         toolutil.StringOrInt `json:"group_id"          jsonschema:"Group ID or URL-encoded path,required"`
	Name            string               `json:"name"              jsonschema:"Name of the custom role,required"`
	BaseAccessLevel int                  `json:"base_access_level" jsonschema:"Base access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer),required"`
	Description     string               `json:"description,omitempty" jsonschema:"Description of the custom role"`
	Permissions
}

// DeleteInstanceInput holds parameters for deleting an instance member role.
type DeleteInstanceInput struct {
	MemberRoleID int64 `json:"member_role_id" jsonschema:"Member role ID to delete,required"`
}

// DeleteGroupInput holds parameters for deleting a group member role.
type DeleteGroupInput struct {
	GroupID      toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	MemberRoleID int64                `json:"member_role_id" jsonschema:"Member role ID to delete,required"`
}

// Permissions represents the optional permission flags for a custom member role.
type Permissions struct {
	AdminCICDVariables         *bool `json:"admin_cicd_variables,omitempty"          jsonschema:"Allow admin CI/CD variables"`
	AdminComplianceFramework   *bool `json:"admin_compliance_framework,omitempty"    jsonschema:"Allow admin compliance framework"`
	AdminGroupMembers          *bool `json:"admin_group_member,omitempty"            jsonschema:"Allow admin group members"`
	AdminMergeRequests         *bool `json:"admin_merge_request,omitempty"           jsonschema:"Allow admin merge requests"`
	AdminPushRules             *bool `json:"admin_push_rules,omitempty"              jsonschema:"Allow admin push rules"`
	AdminTerraformState        *bool `json:"admin_terraform_state,omitempty"         jsonschema:"Allow admin Terraform state"`
	AdminVulnerability         *bool `json:"admin_vulnerability,omitempty"           jsonschema:"Allow admin vulnerability"`
	AdminWebHook               *bool `json:"admin_web_hook,omitempty"                jsonschema:"Allow admin webhooks"`
	ArchiveProject             *bool `json:"archive_project,omitempty"               jsonschema:"Allow archive project"`
	ManageDeployTokens         *bool `json:"manage_deploy_tokens,omitempty"          jsonschema:"Allow manage deploy tokens"`
	ManageGroupAccessTokens    *bool `json:"manage_group_access_tokens,omitempty"    jsonschema:"Allow manage group access tokens"`
	ManageMergeRequestSettings *bool `json:"manage_merge_request_settings,omitempty" jsonschema:"Allow manage MR settings"`
	ManageProjectAccessTokens  *bool `json:"manage_project_access_tokens,omitempty"  jsonschema:"Allow manage project access tokens"`
	ManageSecurityPolicyLink   *bool `json:"manage_security_policy_link,omitempty"   jsonschema:"Allow manage security policy link"`
	ReadCode                   *bool `json:"read_code,omitempty"                     jsonschema:"Allow read code"`
	ReadRunners                *bool `json:"read_runners,omitempty"                  jsonschema:"Allow read runners"`
	ReadDependency             *bool `json:"read_dependency,omitempty"               jsonschema:"Allow read dependency"`
	ReadVulnerability          *bool `json:"read_vulnerability,omitempty"            jsonschema:"Allow read vulnerability"`
	RemoveGroup                *bool `json:"remove_group,omitempty"                  jsonschema:"Allow remove group"`
	RemoveProject              *bool `json:"remove_project,omitempty"                jsonschema:"Allow remove project"`
}

// Output represents a member role.
type Output struct {
	toolutil.HintableOutput
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	GroupID         int64  `json:"group_id,omitempty"`
	BaseAccessLevel int    `json:"base_access_level"`
	Permissions
}

// ListOutput holds the list response.
type ListOutput struct {
	toolutil.HintableOutput
	Roles []Output `json:"roles"`
}

func toOutput(r *gl.MemberRole) Output {
	if r == nil {
		return Output{}
	}
	return Output{
		ID:              r.ID,
		Name:            r.Name,
		Description:     r.Description,
		GroupID:         r.GroupID,
		BaseAccessLevel: int(r.BaseAccessLevel),
		Permissions: Permissions{
			AdminCICDVariables:         new(r.AdminCICDVariables),
			AdminComplianceFramework:   new(r.AdminComplianceFramework),
			AdminGroupMembers:          new(r.AdminGroupMembers),
			AdminMergeRequests:         new(r.AdminMergeRequests),
			AdminPushRules:             new(r.AdminPushRules),
			AdminTerraformState:        new(r.AdminTerraformState),
			AdminVulnerability:         new(r.AdminVulnerability),
			AdminWebHook:               new(r.AdminWebHook),
			ArchiveProject:             new(r.ArchiveProject),
			ManageDeployTokens:         new(r.ManageDeployTokens),
			ManageGroupAccessTokens:    new(r.ManageGroupAccessTokens),
			ManageMergeRequestSettings: new(r.ManageMergeRequestSettings),
			ManageProjectAccessTokens:  new(r.ManageProjectAccessTokens),
			ManageSecurityPolicyLink:   new(r.ManageSecurityPolicyLink),
			ReadCode:                   new(r.ReadCode),
			ReadRunners:                new(r.ReadRunners),
			ReadDependency:             new(r.ReadDependency),
			ReadVulnerability:          new(r.ReadVulnerability),
			RemoveGroup:                new(r.RemoveGroup),
			RemoveProject:              new(r.RemoveProject),
		},
	}
}

func buildCreateOpts(name string, baseLevel int, desc string, p Permissions) *gl.CreateMemberRoleOptions {
	opts := &gl.CreateMemberRoleOptions{
		Name:            new(name),
		BaseAccessLevel: new(gl.AccessLevelValue(baseLevel)),
	}
	if desc != "" {
		opts.Description = new(desc)
	}
	if p.AdminCICDVariables != nil {
		opts.AdminCICDVariables = p.AdminCICDVariables
	}
	if p.AdminComplianceFramework != nil {
		opts.AdminComplianceFramework = p.AdminComplianceFramework
	}
	if p.AdminGroupMembers != nil {
		opts.AdminGroupMembers = p.AdminGroupMembers
	}
	if p.AdminMergeRequests != nil {
		opts.AdminMergeRequest = p.AdminMergeRequests
	}
	if p.AdminPushRules != nil {
		opts.AdminPushRules = p.AdminPushRules
	}
	if p.AdminTerraformState != nil {
		opts.AdminTerraformState = p.AdminTerraformState
	}
	if p.AdminVulnerability != nil {
		opts.AdminVulnerability = p.AdminVulnerability
	}
	if p.AdminWebHook != nil {
		opts.AdminWebHook = p.AdminWebHook
	}
	if p.ArchiveProject != nil {
		opts.ArchiveProject = p.ArchiveProject
	}
	if p.ManageDeployTokens != nil {
		opts.ManageDeployTokens = p.ManageDeployTokens
	}
	if p.ManageGroupAccessTokens != nil {
		opts.ManageGroupAccessTokens = p.ManageGroupAccessTokens
	}
	if p.ManageMergeRequestSettings != nil {
		opts.ManageMergeRequestSettings = p.ManageMergeRequestSettings
	}
	if p.ManageProjectAccessTokens != nil {
		opts.ManageProjectAccessTokens = p.ManageProjectAccessTokens
	}
	if p.ManageSecurityPolicyLink != nil {
		opts.ManageSecurityPolicyLink = p.ManageSecurityPolicyLink
	}
	if p.ReadCode != nil {
		opts.ReadCode = p.ReadCode
	}
	if p.ReadRunners != nil {
		opts.ReadRunners = p.ReadRunners
	}
	if p.ReadDependency != nil {
		opts.ReadDependency = p.ReadDependency
	}
	if p.ReadVulnerability != nil {
		opts.ReadVulnerability = p.ReadVulnerability
	}
	if p.RemoveGroup != nil {
		opts.RemoveGroup = p.RemoveGroup
	}
	if p.RemoveProject != nil {
		opts.RemoveProject = p.RemoveProject
	}
	return opts
}

// ListInstance returns all instance-level member roles.
func ListInstance(ctx context.Context, client *gitlabclient.Client, _ ListInstanceInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	roles, _, err := client.GL().MemberRolesService.ListInstanceMemberRoles()
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list instance member roles", err)
	}
	out := ListOutput{Roles: make([]Output, 0, len(roles))}
	for _, r := range roles {
		out.Roles = append(out.Roles, toOutput(r))
	}
	return out, nil
}

// ListGroup returns all member roles for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, in ListGroupInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	roles, _, err := client.GL().MemberRolesService.ListMemberRoles(in.GroupID.String())
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list group member roles", err)
	}
	out := ListOutput{Roles: make([]Output, 0, len(roles))}
	for _, r := range roles {
		out.Roles = append(out.Roles, toOutput(r))
	}
	return out, nil
}

// CreateInstance creates a new instance-level member role.
func CreateInstance(ctx context.Context, client *gitlabclient.Client, in CreateInstanceInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if in.BaseAccessLevel == 0 {
		return Output{}, toolutil.ErrFieldRequired("base_access_level")
	}
	opts := buildCreateOpts(in.Name, in.BaseAccessLevel, in.Description, in.Permissions)
	role, _, err := client.GL().MemberRolesService.CreateInstanceMemberRole(opts)
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create instance member role", err)
	}
	return toOutput(role), nil
}

// CreateGroup creates a new group-level member role.
func CreateGroup(ctx context.Context, client *gitlabclient.Client, in CreateGroupInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.GroupID.String() == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if in.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	if in.BaseAccessLevel == 0 {
		return Output{}, toolutil.ErrFieldRequired("base_access_level")
	}
	opts := buildCreateOpts(in.Name, in.BaseAccessLevel, in.Description, in.Permissions)
	role, _, err := client.GL().MemberRolesService.CreateMemberRole(in.GroupID.String(), opts)
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create group member role", err)
	}
	return toOutput(role), nil
}

// DeleteInstance deletes an instance-level member role.
func DeleteInstance(ctx context.Context, client *gitlabclient.Client, in DeleteInstanceInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.MemberRoleID == 0 {
		return toolutil.ErrFieldRequired("member_role_id")
	}
	_, err := client.GL().MemberRolesService.DeleteInstanceMemberRole(in.MemberRoleID)
	if err != nil {
		return toolutil.WrapErrWithMessage("delete instance member role", err)
	}
	return nil
}

// DeleteGroup deletes a group-level member role.
func DeleteGroup(ctx context.Context, client *gitlabclient.Client, in DeleteGroupInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.MemberRoleID == 0 {
		return toolutil.ErrFieldRequired("member_role_id")
	}
	_, err := client.GL().MemberRolesService.DeleteMemberRole(in.GroupID.String(), in.MemberRoleID)
	if err != nil {
		return toolutil.WrapErrWithMessage("delete group member role", err)
	}
	return nil
}
