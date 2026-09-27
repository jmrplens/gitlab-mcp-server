package jobtokenscope

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Access Settings.

// GetAccessSettingsInput is the input for getting job token access settings.
type GetAccessSettingsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// AccessSettingsOutput is the output for job token access settings: both keys
// lib/api/entities/project_job_token_scope.rb sends. InboundEnabled says
// whether only the projects on the allowlist may reach this project with a job
// token. OutboundEnabled says whether this project's own job token is limited
// to the projects it names, the older outbound scope GitLab deprecated and
// planned to remove in 18.0 and still sends; client-go's
// JobTokenAccessSettings has no field for it, so it is read off the captured
// response (ADR-0021).
type AccessSettingsOutput struct {
	toolutil.HintableOutput
	InboundEnabled  bool `json:"inbound_enabled"`
	OutboundEnabled bool `json:"outbound_enabled"`
}

// accessSettingsExtra is the key of the job token scope entity client-go's
// JobTokenAccessSettings does not model.
type accessSettingsExtra struct {
	OutboundEnabled bool `json:"outbound_enabled"`
}

// GetAccessSettings returns the CI/CD job token access settings for a project.
func GetAccessSettings(ctx context.Context, client *gitlabclient.Client, input GetAccessSettingsInput) (AccessSettingsOutput, error) {
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	settings, _, err := client.GL().JobTokenScope.GetProjectJobTokenAccessSettings(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return AccessSettingsOutput{}, toolutil.WrapErrWithStatusHint("get_job_token_access_settings", err, http.StatusNotFound,
			"verify project_id with project.get; CI/CD job token settings are at project level")
	}
	var extra accessSettingsExtra
	if err = captured.Decode(&extra); err != nil {
		return AccessSettingsOutput{}, toolutil.WrapErr("get_job_token_access_settings", err)
	}
	return AccessSettingsOutput{
		InboundEnabled:  settings.InboundEnabled,
		OutboundEnabled: extra.OutboundEnabled,
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
		return toolutil.DeleteOutput{}, toolutil.WrapErrWithStatusHint("patch_job_token_access_settings", err, http.StatusForbidden,
			"updating job token access settings requires Maintainer role; verify project_id")
	}
	return toolutil.DeleteOutput{Status: "updated"}, nil
}

// Project Inbound Allowlist.

// ListInboundAllowlistInput is the input for listing job token inbound allowlist projects.
type ListInboundAllowlistInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id). Only applies when pagination='keyset'."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset-paginated results: asc or desc. Only applies when pagination='keyset'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListInboundAllowlistOutput is the output for listing inbound allowlist
// projects. GitLab renders each of them as Entities::BasicProjectDetails
// (lib/api/project_job_token_scope.rb), which is internal/tools/projects'
// BasicOutput, so every row carries what that entity sends: the names and
// paths, the web and clone URLs, visibility, default branch, topics, counts
// and the namespace. The license pair and the custom attributes the same shape
// can carry wait on presenter options this route never passes, so they stay
// empty and are omitted here.
type ListInboundAllowlistOutput struct {
	toolutil.HintableOutput
	Projects   []projects.BasicOutput    `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// applyAllowlistListOptions copies the offset, keyset, and keyset-only
// order_by/sort parameters onto a [gl.ListOptions]. It is shared by the
// inbound project and group allowlist list handlers, which both embed
// [toolutil.PaginationInput] and [toolutil.KeysetPaginationInput] plus
// OrderBy/Sort fields.
func applyAllowlistListOptions(opts *gl.ListOptions, page toolutil.PaginationInput, keyset toolutil.KeysetPaginationInput, orderBy, sort string) {
	toolutil.ApplyListOptions(opts, page, keyset)
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
}

// ListInboundAllowlist returns the projects on the job token inbound allowlist.
func ListInboundAllowlist(ctx context.Context, client *gitlabclient.Client, input ListInboundAllowlistInput) (ListInboundAllowlistOutput, error) {
	opts := &gl.GetJobTokenInboundAllowListOptions{}
	applyAllowlistListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)
	allowed, resp, err := client.GL().JobTokenScope.GetProjectJobTokenInboundAllowList(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListInboundAllowlistOutput{}, toolutil.WrapErrWithStatusHint("list_job_token_inbound_allowlist", err, http.StatusNotFound,
			"verify project_id; allowlist may be empty if inbound scope is disabled")
	}
	items := make([]projects.BasicOutput, 0, len(allowed))
	for _, p := range allowed {
		items = append(items, projects.ToBasicOutput(p))
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
		return InboundAllowItemOutput{}, toolutil.WrapErrWithStatusHint("add_project_job_token_allowlist", err, http.StatusForbidden,
			"adding to inbound allowlist requires Maintainer role on source project; verify target_project_id exists and is accessible")
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
		return toolutil.WrapErrWithStatusHint("remove_project_job_token_allowlist", err, http.StatusNotFound,
			"verify target_project_id is on the allowlist with job.token_scope_list_inbound; requires Maintainer role")
	}
	return nil
}

// Group Allowlist.

// ListGroupAllowlistInput is the input for listing job token allowlist groups.
type ListGroupAllowlistInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id). Only applies when pagination='keyset'."`
	Sort      string               `json:"sort,omitempty" jsonschema:"Sort direction for keyset-paginated results: asc or desc. Only applies when pagination='keyset'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// AllowlistGroupItem is a group on the job token allowlist, as
// GET /projects/:id/job_token_scope/groups_allowlist presents it
// (Entities::BasicGroupDetails: the id, the name and the page, and no path).
type AllowlistGroupItem struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"web_url"`
}

// ListGroupAllowlistOutput is the output for listing allowlist groups.
type ListGroupAllowlistOutput struct {
	toolutil.HintableOutput
	Groups     []AllowlistGroupItem      `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListGroupAllowlist returns the groups on the job token allowlist.
func ListGroupAllowlist(ctx context.Context, client *gitlabclient.Client, input ListGroupAllowlistInput) (ListGroupAllowlistOutput, error) {
	opts := &gl.GetJobTokenAllowlistGroupsOptions{}
	applyAllowlistListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput, input.OrderBy, input.Sort)
	groups, resp, err := client.GL().JobTokenScope.GetJobTokenAllowlistGroups(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListGroupAllowlistOutput{}, toolutil.WrapErrWithStatusHint("list_job_token_group_allowlist", err, http.StatusNotFound,
			"verify project_id; group allowlist requires GitLab 17.0+")
	}
	items := make([]AllowlistGroupItem, 0, len(groups))
	for _, g := range groups {
		items = append(items, AllowlistGroupItem{
			ID:     g.ID,
			Name:   g.Name,
			WebURL: g.WebURL,
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
		return GroupAllowlistItemOutput{}, toolutil.WrapErrWithStatusHint("add_group_job_token_allowlist", err, http.StatusForbidden,
			"adding to group allowlist requires Maintainer role on project; verify target_group_id exists; requires GitLab 17.0+")
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
		return toolutil.WrapErrWithStatusHint("remove_group_job_token_allowlist", err, http.StatusNotFound,
			"verify target_group_id is on the allowlist with job.token_scope_list_groups; requires Maintainer role")
	}
	return nil
}

// Markdown Formatters.
