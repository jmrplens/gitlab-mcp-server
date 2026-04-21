// Package jobtokenscope implements MCP tool handlers for managing GitLab
// project CI/CD job token scope settings. It wraps the JobTokenScopeService
// from client-go v2.
package jobtokenscope

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Access Settings.

// GetAccessSettingsInput is the input for getting job token access settings.
type GetAccessSettingsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// AccessSettingsOutput is the output for job token access settings.
type AccessSettingsOutput struct {
	toolutil.HintableOutput
	InboundEnabled bool `json:"inbound_enabled"`
}

// GetAccessSettings returns the CI/CD job token access settings for a project.
func GetAccessSettings(ctx context.Context, client *gitlabclient.Client, input GetAccessSettingsInput) (AccessSettingsOutput, error) {
	settings, _, err := client.GL().JobTokenScope.GetProjectJobTokenAccessSettings(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return AccessSettingsOutput{}, toolutil.WrapErrWithMessage("get_job_token_access_settings", err)
	}
	return AccessSettingsOutput{
		InboundEnabled: settings.InboundEnabled,
	}, nil
}

// PatchAccessSettingsInput is the input for patching job token access settings.
type PatchAccessSettingsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Enabled   bool                 `json:"enabled" jsonschema:"Enable or disable the CI/CD job token scope,required"`
}

// PatchAccessSettings updates the CI/CD job token access settings for a project.
func PatchAccessSettings(ctx context.Context, client *gitlabclient.Client, input PatchAccessSettingsInput) (toolutil.DeleteOutput, error) {
	opts := &gl.PatchProjectJobTokenAccessSettingsOptions{
		Enabled: input.Enabled,
	}
	_, err := client.GL().JobTokenScope.PatchProjectJobTokenAccessSettings(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.DeleteOutput{}, toolutil.WrapErrWithMessage("patch_job_token_access_settings", err)
	}
	return toolutil.DeleteOutput{Status: "updated"}, nil
}

// Project Inbound Allowlist.

// ListInboundAllowlistInput is the input for listing job token inbound allowlist projects.
type ListInboundAllowlistInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
}

// AllowlistProjectItem is a project on the inbound allowlist.
type AllowlistProjectItem struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

// ListInboundAllowlistOutput is the output for listing inbound allowlist projects.
type ListInboundAllowlistOutput struct {
	toolutil.HintableOutput
	Projects   []AllowlistProjectItem    `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListInboundAllowlist returns the projects on the job token inbound allowlist.
func ListInboundAllowlist(ctx context.Context, client *gitlabclient.Client, input ListInboundAllowlistInput) (ListInboundAllowlistOutput, error) {
	opts := &gl.GetJobTokenInboundAllowListOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	projects, resp, err := client.GL().JobTokenScope.GetProjectJobTokenInboundAllowList(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListInboundAllowlistOutput{}, toolutil.WrapErrWithMessage("list_job_token_inbound_allowlist", err)
	}
	items := make([]AllowlistProjectItem, 0, len(projects))
	for _, p := range projects {
		items = append(items, AllowlistProjectItem{
			ID:                p.ID,
			Name:              p.Name,
			PathWithNamespace: p.PathWithNamespace,
			WebURL:            p.WebURL,
		})
	}
	return ListInboundAllowlistOutput{
		Projects:   items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// AddProjectAllowlistInput is the input for adding a project to the inbound allowlist.
type AddProjectAllowlistInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TargetProjectID int64                `json:"target_project_id" jsonschema:"ID of the project to add to the allowlist,required"`
}

// InboundAllowItemOutput is the output for an inbound allowlist item.
type InboundAllowItemOutput struct {
	toolutil.HintableOutput
	SourceProjectID int64 `json:"source_project_id"`
	TargetProjectID int64 `json:"target_project_id"`
}

// AddProjectAllowlist adds a project to the CI/CD job token inbound allowlist.
func AddProjectAllowlist(ctx context.Context, client *gitlabclient.Client, input AddProjectAllowlistInput) (InboundAllowItemOutput, error) {
	if input.TargetProjectID <= 0 {
		return InboundAllowItemOutput{}, toolutil.ErrRequiredInt64("add_project_job_token_allowlist", "target_project_id")
	}
	opts := &gl.JobTokenInboundAllowOptions{
		TargetProjectID: new(input.TargetProjectID),
	}
	item, _, err := client.GL().JobTokenScope.AddProjectToJobScopeAllowList(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return InboundAllowItemOutput{}, toolutil.WrapErrWithMessage("add_project_job_token_allowlist", err)
	}
	return InboundAllowItemOutput{
		SourceProjectID: item.SourceProjectID,
		TargetProjectID: item.TargetProjectID,
	}, nil
}

// RemoveProjectAllowlistInput is the input for removing a project from the inbound allowlist.
type RemoveProjectAllowlistInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TargetProjectID int64                `json:"target_project_id" jsonschema:"ID of the project to remove from the allowlist,required"`
}

// RemoveProjectAllowlist removes a project from the CI/CD job token inbound allowlist.
func RemoveProjectAllowlist(ctx context.Context, client *gitlabclient.Client, input RemoveProjectAllowlistInput) error {
	if input.TargetProjectID <= 0 {
		return toolutil.ErrRequiredInt64("remove_project_job_token_allowlist", "target_project_id")
	}
	_, err := client.GL().JobTokenScope.RemoveProjectFromJobScopeAllowList(string(input.ProjectID), input.TargetProjectID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("remove_project_job_token_allowlist", err)
	}
	return nil
}

// Group Allowlist.

// ListGroupAllowlistInput is the input for listing job token allowlist groups.
type ListGroupAllowlistInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (max 100)"`
}

// AllowlistGroupItem is a group on the job token allowlist.
type AllowlistGroupItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"full_path"`
	WebURL   string `json:"web_url"`
}

// ListGroupAllowlistOutput is the output for listing allowlist groups.
type ListGroupAllowlistOutput struct {
	toolutil.HintableOutput
	Groups     []AllowlistGroupItem      `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListGroupAllowlist returns the groups on the job token allowlist.
func ListGroupAllowlist(ctx context.Context, client *gitlabclient.Client, input ListGroupAllowlistInput) (ListGroupAllowlistOutput, error) {
	opts := &gl.GetJobTokenAllowlistGroupsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	groups, resp, err := client.GL().JobTokenScope.GetJobTokenAllowlistGroups(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListGroupAllowlistOutput{}, toolutil.WrapErrWithMessage("list_job_token_group_allowlist", err)
	}
	items := make([]AllowlistGroupItem, 0, len(groups))
	for _, g := range groups {
		items = append(items, AllowlistGroupItem{
			ID:       g.ID,
			Name:     g.Name,
			FullPath: g.FullPath,
			WebURL:   g.WebURL,
		})
	}
	return ListGroupAllowlistOutput{
		Groups:     items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// AddGroupAllowlistInput is the input for adding a group to the allowlist.
type AddGroupAllowlistInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TargetGroupID int64                `json:"target_group_id" jsonschema:"ID of the group to add to the allowlist,required"`
}

// GroupAllowlistItemOutput is the output for a group allowlist item.
type GroupAllowlistItemOutput struct {
	toolutil.HintableOutput
	SourceProjectID int64 `json:"source_project_id"`
	TargetGroupID   int64 `json:"target_group_id"`
}

// AddGroupAllowlist adds a group to the CI/CD job token allowlist.
func AddGroupAllowlist(ctx context.Context, client *gitlabclient.Client, input AddGroupAllowlistInput) (GroupAllowlistItemOutput, error) {
	if input.TargetGroupID <= 0 {
		return GroupAllowlistItemOutput{}, toolutil.ErrRequiredInt64("add_group_job_token_allowlist", "target_group_id")
	}
	opts := &gl.AddGroupToJobTokenAllowlistOptions{
		TargetGroupID: new(input.TargetGroupID),
	}
	item, _, err := client.GL().JobTokenScope.AddGroupToJobTokenAllowlist(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return GroupAllowlistItemOutput{}, toolutil.WrapErrWithMessage("add_group_job_token_allowlist", err)
	}
	return GroupAllowlistItemOutput{
		SourceProjectID: item.SourceProjectID,
		TargetGroupID:   item.TargetGroupID,
	}, nil
}

// RemoveGroupAllowlistInput is the input for removing a group from the allowlist.
type RemoveGroupAllowlistInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TargetGroupID int64                `json:"target_group_id" jsonschema:"ID of the group to remove from the allowlist,required"`
}

// RemoveGroupAllowlist removes a group from the CI/CD job token allowlist.
func RemoveGroupAllowlist(ctx context.Context, client *gitlabclient.Client, input RemoveGroupAllowlistInput) error {
	if input.TargetGroupID <= 0 {
		return toolutil.ErrRequiredInt64("remove_group_job_token_allowlist", "target_group_id")
	}
	_, err := client.GL().JobTokenScope.RemoveGroupFromJobTokenAllowlist(string(input.ProjectID), input.TargetGroupID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("remove_group_job_token_allowlist", err)
	}
	return nil
}

// Markdown Formatters.
