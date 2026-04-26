// Package badges implements MCP tool handlers for GitLab project and group
// badges. It wraps the ProjectBadgesService and GroupBadgesService from
// client-go v2.
package badges

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// hintVerifyGroupBadgeID is the 404 hint shared by group badge tools.
const hintVerifyGroupBadgeID = "verify badge_id with gitlab_list_group_badges"

// BadgeItem represents a badge in output.
type BadgeItem struct {
	ID               int64  `json:"id"`
	Name             string `json:"name,omitempty"`
	LinkURL          string `json:"link_url"`
	ImageURL         string `json:"image_url"`
	RenderedLinkURL  string `json:"rendered_link_url,omitempty"`
	RenderedImageURL string `json:"rendered_image_url,omitempty"`
	Kind             string `json:"kind,omitempty"`
}

// projectBadgeToItem is an internal helper for the badges package.
func projectBadgeToItem(b *gl.ProjectBadge) BadgeItem {
	return BadgeItem{
		ID:               b.ID,
		Name:             b.Name,
		LinkURL:          b.LinkURL,
		ImageURL:         b.ImageURL,
		RenderedLinkURL:  b.RenderedLinkURL,
		RenderedImageURL: b.RenderedImageURL,
		Kind:             b.Kind,
	}
}

// groupBadgeToItem is an internal helper for the badges package.
func groupBadgeToItem(b *gl.GroupBadge) BadgeItem {
	return BadgeItem{
		ID:               b.ID,
		Name:             b.Name,
		LinkURL:          b.LinkURL,
		ImageURL:         b.ImageURL,
		RenderedLinkURL:  b.RenderedLinkURL,
		RenderedImageURL: b.RenderedImageURL,
		Kind:             string(b.Kind),
	}
}

// Project Badges.

// ListProjectInput is the input for listing project badges.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name,omitempty" jsonschema:"Filter by badge name"`
	Page      int64                `json:"page,omitempty" jsonschema:"Page number"`
	PerPage   int64                `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// ListProjectOutput is the output for listing project badges.
type ListProjectOutput struct {
	toolutil.HintableOutput
	Badges     []BadgeItem               `json:"badges"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProject returns all badges for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListProjectOutput, error) {
	opts := &gl.ListProjectBadgesOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badges, resp, err := client.GL().ProjectBadges.ListProjectBadges(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectOutput{}, toolutil.WrapErrWithStatusHint("list_project_badges", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	items := make([]BadgeItem, 0, len(badges))
	for _, b := range badges {
		items = append(items, projectBadgeToItem(b))
	}
	return ListProjectOutput{
		Badges:     items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// GetProjectInput is the input for getting a project badge.
type GetProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	BadgeID   int64                `json:"badge_id" jsonschema:"Badge ID,required"`
}

// GetProjectOutput is the output for getting a project badge.
type GetProjectOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// GetProject gets a specific project badge.
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (GetProjectOutput, error) {
	if input.BadgeID <= 0 {
		return GetProjectOutput{}, toolutil.ErrRequiredInt64("get_project_badge", "badge_id")
	}
	badge, _, err := client.GL().ProjectBadges.GetProjectBadge(string(input.ProjectID), input.BadgeID, gl.WithContext(ctx))
	if err != nil {
		return GetProjectOutput{}, toolutil.WrapErrWithStatusHint("get_project_badge", err, http.StatusNotFound,
			"verify badge_id with gitlab_list_project_badges \u2014 inherited group badges have negative IDs and cannot be fetched at the project scope")
	}
	return GetProjectOutput{Badge: projectBadgeToItem(badge)}, nil
}

// AddProjectInput is the input for adding a project badge.
type AddProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	LinkURL   string               `json:"link_url" jsonschema:"Badge link URL (supports placeholders),required"`
	ImageURL  string               `json:"image_url" jsonschema:"Badge image URL (supports placeholders),required"`
	Name      string               `json:"name,omitempty" jsonschema:"Badge name"`
}

// AddProjectOutput is the output after adding a project badge.
type AddProjectOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// AddProject adds a badge to a project.
func AddProject(ctx context.Context, client *gitlabclient.Client, input AddProjectInput) (AddProjectOutput, error) {
	opts := &gl.AddProjectBadgeOptions{
		LinkURL:  new(input.LinkURL),
		ImageURL: new(input.ImageURL),
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badge, _, err := client.GL().ProjectBadges.AddProjectBadge(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return AddProjectOutput{}, toolutil.WrapErrWithHint("add_project_badge", err,
				"adding project badges requires Maintainer+ role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return AddProjectOutput{}, toolutil.WrapErrWithHint("add_project_badge", err,
				"link_url and image_url must be valid absolute URLs; placeholders %{project_path}, %{commit_sha}, %{default_branch} are supported")
		}
		return AddProjectOutput{}, toolutil.WrapErrWithMessage("add_project_badge", err)
	}
	return AddProjectOutput{Badge: projectBadgeToItem(badge)}, nil
}

// EditProjectInput is the input for editing a project badge.
type EditProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	BadgeID   int64                `json:"badge_id" jsonschema:"Badge ID,required"`
	LinkURL   string               `json:"link_url,omitempty" jsonschema:"New badge link URL"`
	ImageURL  string               `json:"image_url,omitempty" jsonschema:"New badge image URL"`
	Name      string               `json:"name,omitempty" jsonschema:"New badge name"`
}

// EditProjectOutput is the output after editing a project badge.
type EditProjectOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// EditProject edits a project badge.
func EditProject(ctx context.Context, client *gitlabclient.Client, input EditProjectInput) (EditProjectOutput, error) {
	if input.BadgeID <= 0 {
		return EditProjectOutput{}, toolutil.ErrRequiredInt64("edit_project_badge", "badge_id")
	}
	opts := &gl.EditProjectBadgeOptions{}
	if input.LinkURL != "" {
		opts.LinkURL = new(input.LinkURL)
	}
	if input.ImageURL != "" {
		opts.ImageURL = new(input.ImageURL)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badge, _, err := client.GL().ProjectBadges.EditProjectBadge(string(input.ProjectID), input.BadgeID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return EditProjectOutput{}, toolutil.WrapErrWithHint("edit_project_badge", err,
				"editing project badges requires Maintainer+ role; inherited group badges cannot be edited at project scope")
		}
		return EditProjectOutput{}, toolutil.WrapErrWithStatusHint("edit_project_badge", err, http.StatusNotFound,
			"verify badge_id with gitlab_list_project_badges")
	}
	return EditProjectOutput{Badge: projectBadgeToItem(badge)}, nil
}

// DeleteProjectInput is the input for deleting a project badge.
type DeleteProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	BadgeID   int64                `json:"badge_id" jsonschema:"Badge ID,required"`
}

// DeleteProject deletes a project badge.
func DeleteProject(ctx context.Context, client *gitlabclient.Client, input DeleteProjectInput) error {
	if input.BadgeID <= 0 {
		return toolutil.ErrRequiredInt64("delete_project_badge", "badge_id")
	}
	_, err := client.GL().ProjectBadges.DeleteProjectBadge(string(input.ProjectID), input.BadgeID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("delete_project_badge", err,
				"deleting project badges requires Maintainer+ role; inherited group badges must be deleted at the group level")
		}
		return toolutil.WrapErrWithStatusHint("delete_project_badge", err, http.StatusNotFound,
			"verify badge_id with gitlab_list_project_badges")
	}
	return nil
}

// PreviewProjectInput is the input for previewing a project badge.
type PreviewProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	LinkURL   string               `json:"link_url" jsonschema:"Badge link URL with placeholders,required"`
	ImageURL  string               `json:"image_url" jsonschema:"Badge image URL with placeholders,required"`
}

// PreviewProjectOutput is the output for badge preview.
type PreviewProjectOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// PreviewProject previews how badge URLs render after placeholder resolution.
func PreviewProject(ctx context.Context, client *gitlabclient.Client, input PreviewProjectInput) (PreviewProjectOutput, error) {
	opts := &gl.ProjectBadgePreviewOptions{
		LinkURL:  new(input.LinkURL),
		ImageURL: new(input.ImageURL),
	}
	badge, _, err := client.GL().ProjectBadges.PreviewProjectBadge(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return PreviewProjectOutput{}, toolutil.WrapErrWithStatusHint("preview_project_badge", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get; preview substitutes placeholders without persisting the badge")
	}
	return PreviewProjectOutput{Badge: projectBadgeToItem(badge)}, nil
}

// Group Badges.

// ListGroupInput is the input for listing group badges.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Name    string               `json:"name,omitempty" jsonschema:"Filter by badge name"`
	Page    int64                `json:"page,omitempty" jsonschema:"Page number"`
	PerPage int64                `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// ListGroupOutput is the output for listing group badges.
type ListGroupOutput struct {
	toolutil.HintableOutput
	Badges     []BadgeItem               `json:"badges"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListGroup returns all badges for a group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListGroupOutput, error) {
	opts := &gl.ListGroupBadgesOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badges, resp, err := client.GL().GroupBadges.ListGroupBadges(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListGroupOutput{}, toolutil.WrapErrWithStatusHint("list_group_badges", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get")
	}
	items := make([]BadgeItem, 0, len(badges))
	for _, b := range badges {
		items = append(items, groupBadgeToItem(b))
	}
	return ListGroupOutput{
		Badges:     items,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// GetGroupInput is the input for getting a group badge.
type GetGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	BadgeID int64                `json:"badge_id" jsonschema:"Badge ID,required"`
}

// GetGroupOutput is the output for getting a group badge.
type GetGroupOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// GetGroup gets a specific group badge.
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (GetGroupOutput, error) {
	if input.BadgeID <= 0 {
		return GetGroupOutput{}, toolutil.ErrRequiredInt64("get_group_badge", "badge_id")
	}
	badge, _, err := client.GL().GroupBadges.GetGroupBadge(string(input.GroupID), input.BadgeID, gl.WithContext(ctx))
	if err != nil {
		return GetGroupOutput{}, toolutil.WrapErrWithStatusHint("get_group_badge", err, http.StatusNotFound,
			hintVerifyGroupBadgeID)
	}
	return GetGroupOutput{Badge: groupBadgeToItem(badge)}, nil
}

// AddGroupInput is the input for adding a group badge.
type AddGroupInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	LinkURL  string               `json:"link_url" jsonschema:"Badge link URL (supports placeholders),required"`
	ImageURL string               `json:"image_url" jsonschema:"Badge image URL (supports placeholders),required"`
	Name     string               `json:"name,omitempty" jsonschema:"Badge name"`
}

// AddGroupOutput is the output after adding a group badge.
type AddGroupOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// AddGroup adds a badge to a group.
func AddGroup(ctx context.Context, client *gitlabclient.Client, input AddGroupInput) (AddGroupOutput, error) {
	opts := &gl.AddGroupBadgeOptions{
		LinkURL:  new(input.LinkURL),
		ImageURL: new(input.ImageURL),
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badge, _, err := client.GL().GroupBadges.AddGroupBadge(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return AddGroupOutput{}, toolutil.WrapErrWithHint("add_group_badge", err,
				"adding group badges requires Owner role on the group")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return AddGroupOutput{}, toolutil.WrapErrWithHint("add_group_badge", err,
				"link_url and image_url must be valid absolute URLs; group badges are inherited by all projects in the group")
		}
		return AddGroupOutput{}, toolutil.WrapErrWithMessage("add_group_badge", err)
	}
	return AddGroupOutput{Badge: groupBadgeToItem(badge)}, nil
}

// EditGroupInput is the input for editing a group badge.
type EditGroupInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	BadgeID  int64                `json:"badge_id" jsonschema:"Badge ID,required"`
	LinkURL  string               `json:"link_url,omitempty" jsonschema:"New badge link URL"`
	ImageURL string               `json:"image_url,omitempty" jsonschema:"New badge image URL"`
	Name     string               `json:"name,omitempty" jsonschema:"New badge name"`
}

// EditGroupOutput is the output after editing a group badge.
type EditGroupOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// EditGroup edits a group badge.
func EditGroup(ctx context.Context, client *gitlabclient.Client, input EditGroupInput) (EditGroupOutput, error) {
	if input.BadgeID <= 0 {
		return EditGroupOutput{}, toolutil.ErrRequiredInt64("edit_group_badge", "badge_id")
	}
	opts := &gl.EditGroupBadgeOptions{}
	if input.LinkURL != "" {
		opts.LinkURL = new(input.LinkURL)
	}
	if input.ImageURL != "" {
		opts.ImageURL = new(input.ImageURL)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badge, _, err := client.GL().GroupBadges.EditGroupBadge(string(input.GroupID), input.BadgeID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return EditGroupOutput{}, toolutil.WrapErrWithHint("edit_group_badge", err,
				"editing group badges requires Owner role on the group")
		}
		return EditGroupOutput{}, toolutil.WrapErrWithStatusHint("edit_group_badge", err, http.StatusNotFound,
			hintVerifyGroupBadgeID)
	}
	return EditGroupOutput{Badge: groupBadgeToItem(badge)}, nil
}

// DeleteGroupInput is the input for deleting a group badge.
type DeleteGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	BadgeID int64                `json:"badge_id" jsonschema:"Badge ID,required"`
}

// DeleteGroup deletes a group badge.
func DeleteGroup(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) error {
	if input.BadgeID <= 0 {
		return toolutil.ErrRequiredInt64("delete_group_badge", "badge_id")
	}
	_, err := client.GL().GroupBadges.DeleteGroupBadge(string(input.GroupID), input.BadgeID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("delete_group_badge", err,
				"deleting group badges requires Owner role on the group")
		}
		return toolutil.WrapErrWithStatusHint("delete_group_badge", err, http.StatusNotFound,
			hintVerifyGroupBadgeID)
	}
	return nil
}

// PreviewGroupInput is the input for previewing a group badge.
type PreviewGroupInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	LinkURL  string               `json:"link_url" jsonschema:"Badge link URL with placeholders,required"`
	ImageURL string               `json:"image_url" jsonschema:"Badge image URL with placeholders,required"`
	Name     string               `json:"name,omitempty" jsonschema:"Badge name"`
}

// PreviewGroupOutput is the output for group badge preview.
type PreviewGroupOutput struct {
	toolutil.HintableOutput
	Badge BadgeItem `json:"badge"`
}

// PreviewGroup previews how group badge URLs render after placeholder resolution.
func PreviewGroup(ctx context.Context, client *gitlabclient.Client, input PreviewGroupInput) (PreviewGroupOutput, error) {
	opts := &gl.GroupBadgePreviewOptions{
		LinkURL:  new(input.LinkURL),
		ImageURL: new(input.ImageURL),
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	badge, _, err := client.GL().GroupBadges.PreviewGroupBadge(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return PreviewGroupOutput{}, toolutil.WrapErrWithStatusHint("preview_group_badge", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; preview substitutes placeholders without persisting the badge")
	}
	return PreviewGroupOutput{Badge: groupBadgeToItem(badge)}, nil
}

// Markdown Formatters.
