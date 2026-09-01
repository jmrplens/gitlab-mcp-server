package groups

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// ShareGroupWithGroup
// ---------------------------------------------------------------------------.

// ShareGroupInput defines parameters for sharing a group with another group.
type ShareGroupInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"      jsonschema:"Group ID or URL-encoded path of the group being shared,required"`
	SharedGroupID int64                `json:"shared_group_id" jsonschema:"ID of the group to share with,required"`
	GroupAccess   int                  `json:"group_access"  jsonschema:"Access level granted to the shared group (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner),required"`
	ExpiresAt     string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the share (YYYY-MM-DD)"`
	MemberRoleID  *int64               `json:"member_role_id,omitempty" jsonschema:"Custom member role ID to grant (Ultimate)"`
}

// ShareGroupOutput holds the result of sharing a group with another group.
type ShareGroupOutput struct {
	toolutil.HintableOutput
	Message       string `json:"message"`
	SharedGroupID int64  `json:"shared_group_id,omitempty"`
	GroupAccess   int    `json:"group_access,omitempty"`
	AccessRole    string `json:"access_role,omitempty"`
}

// accessLevelName returns the human-readable name for a GitLab access level.
func accessLevelName(level int) string {
	names := map[int]string{
		10: "Guest",
		20: "Reporter",
		25: "Security Manager",
		30: "Developer",
		40: "Maintainer",
		50: "Owner",
	}
	if name, ok := names[level]; ok {
		return name
	}
	return fmt.Sprintf("Level %d", level)
}

// ShareGroupWithGroup shares a group with another group.
func ShareGroupWithGroup(ctx context.Context, client *gitlabclient.Client, input ShareGroupInput) (ShareGroupOutput, error) {
	if err := ctx.Err(); err != nil {
		return ShareGroupOutput{}, err
	}
	if input.GroupID == "" {
		return ShareGroupOutput{}, errors.New("groupShareWithGroup: group_id is required. Use gitlab_group_list to find the ID, then pass it as group_id")
	}
	if input.SharedGroupID == 0 {
		return ShareGroupOutput{}, errors.New("groupShareWithGroup: shared_group_id is required. Use gitlab_group_list to find the group ID to share with")
	}
	if input.GroupAccess == 0 {
		return ShareGroupOutput{}, errors.New("groupShareWithGroup: group_access is required. Valid levels: 10 (Guest), 20 (Reporter), 30 (Developer), 40 (Maintainer), 50 (Owner)")
	}
	opts := &gl.ShareGroupWithGroupOptions{
		GroupID:     new(input.SharedGroupID),
		GroupAccess: new(gl.AccessLevelValue(input.GroupAccess)),
	}
	if input.ExpiresAt != "" {
		isoTime, perr := gl.ParseISOTime(input.ExpiresAt)
		if perr != nil {
			return ShareGroupOutput{}, fmt.Errorf("groupShareWithGroup: expires_at must be YYYY-MM-DD: %w", perr)
		}
		opts.ExpiresAt = &isoTime
	}
	if input.MemberRoleID != nil {
		opts.MemberRoleID = input.MemberRoleID
	}
	_, _, err := client.GL().Groups.ShareGroupWithGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return ShareGroupOutput{}, toolutil.WrapErrWithHint("groupShareWithGroup", err,
				"group_access must be 10/20/30/40/50 (Guest/Reporter/Developer/Maintainer/Owner); expires_at must be YYYY-MM-DD; the group may already be shared with this group — use gitlab_group_shared_with_list to verify")
		}
		return ShareGroupOutput{}, toolutil.WrapErrWithStatusHint("groupShareWithGroup", err, http.StatusNotFound,
			"verify group_id and shared_group_id with gitlab_group_get")
	}
	roleName := accessLevelName(input.GroupAccess)
	return ShareGroupOutput{
		Message:       fmt.Sprintf("Group %s shared with group %d as %s", input.GroupID, input.SharedGroupID, roleName),
		SharedGroupID: input.SharedGroupID,
		GroupAccess:   input.GroupAccess,
		AccessRole:    roleName,
	}, nil
}

// ---------------------------------------------------------------------------
// UnshareGroupFromGroup
// ---------------------------------------------------------------------------.

// UnshareGroupInput defines parameters for revoking a group-to-group share.
type UnshareGroupInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"        jsonschema:"Group ID or URL-encoded path of the group being unshared,required"`
	SharedGroupID int64                `json:"shared_group_id" jsonschema:"ID of the group whose share is removed,required"`
}

// UnshareGroupFromGroup removes a group-to-group share.
func UnshareGroupFromGroup(ctx context.Context, client *gitlabclient.Client, input UnshareGroupInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupUnshareFromGroup: group_id is required")
	}
	if input.SharedGroupID == 0 {
		return errors.New("groupUnshareFromGroup: shared_group_id is required")
	}
	_, err := client.GL().Groups.UnshareGroupFromGroup(string(input.GroupID), input.SharedGroupID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupUnshareFromGroup", err, http.StatusNotFound,
			"the group is not shared with this group, or the IDs are wrong — use gitlab_group_shared_with_list to verify; requires Owner role")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ListGroupSharedProjects
// ---------------------------------------------------------------------------.

// ListSharedProjectsInput defines parameters for listing projects shared with a group.
type ListSharedProjectsInput struct {
	GroupID                  toolutil.StringOrInt `json:"group_id"                  jsonschema:"Group ID or URL-encoded path,required"`
	Archived                 *bool                `json:"archived,omitempty"        jsonschema:"Filter archived projects"`
	MinAccessLevel           int                  `json:"min_access_level,omitempty" jsonschema:"Limit to projects where the caller has at least this access level (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	OrderBy                  string               `json:"order_by,omitempty"        jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Search                   string               `json:"search,omitempty"          jsonschema:"Filter projects by name"`
	Simple                   *bool                `json:"simple,omitempty"          jsonschema:"Return limited fields"`
	Sort                     string               `json:"sort,omitempty"            jsonschema:"Sort direction (asc, desc)"`
	Starred                  *bool                `json:"starred,omitempty"         jsonschema:"Limit to starred projects"`
	Visibility               string               `json:"visibility,omitempty"      jsonschema:"Filter by visibility (public, internal, private)"`
	WithCustomAttributes     *bool                `json:"with_custom_attributes,omitempty"      jsonschema:"Include custom attributes in the response"`
	WithIssuesEnabled        *bool                `json:"with_issues_enabled,omitempty"         jsonschema:"Limit to projects with issues enabled"`
	WithMergeRequestsEnabled *bool                `json:"with_merge_requests_enabled,omitempty" jsonschema:"Limit to projects with merge requests enabled"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// SharedProjectsListOutput holds a paginated list of projects shared with a group.
type SharedProjectsListOutput struct {
	toolutil.HintableOutput
	Projects   []ProjectItem             `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// applyListSharedProjectsOptions copies the input filters onto the SDK options.
func applyListSharedProjectsOptions(input ListSharedProjectsInput, opts *gl.ListGroupSharedProjectsOptions) {
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Simple != nil {
		opts.Simple = input.Simple
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Starred != nil {
		opts.Starred = input.Starred
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.WithCustomAttributes != nil {
		opts.WithCustomAttributes = input.WithCustomAttributes
	}
	if input.WithIssuesEnabled != nil {
		opts.WithIssuesEnabled = input.WithIssuesEnabled
	}
	if input.WithMergeRequestsEnabled != nil {
		opts.WithMergeRequestsEnabled = input.WithMergeRequestsEnabled
	}
}

// ListSharedProjects retrieves projects shared with a group.
func ListSharedProjects(ctx context.Context, client *gitlabclient.Client, input ListSharedProjectsInput) (SharedProjectsListOutput, error) {
	if err := ctx.Err(); err != nil {
		return SharedProjectsListOutput{}, err
	}
	if input.GroupID == "" {
		return SharedProjectsListOutput{}, errors.New("groupListSharedProjects: group_id is required")
	}
	opts := &gl.ListGroupSharedProjectsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyListSharedProjectsOptions(input, opts)

	projects, resp, err := client.GL().Groups.ListGroupSharedProjects(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return SharedProjectsListOutput{}, toolutil.WrapErrWithStatusHint("groupListSharedProjects", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get — shared projects are projects shared *into* this group from elsewhere, not the group's own projects")
	}
	return SharedProjectsListOutput{Projects: projectItemsFromGroup(projects), Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// TransferSubGroup
// ---------------------------------------------------------------------------.

// TransferSubGroupInput defines parameters for transferring a group under a new
// parent group or to the top level.
type TransferSubGroupInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id"  jsonschema:"Group ID or URL-encoded path of the group to move,required"`
	ParentID *int64               `json:"parent_id,omitempty" jsonschema:"ID of the new parent group. Omit to turn the subgroup into a top-level group"`
}

// TransferSubGroup moves a group under a new parent group, or promotes a
// subgroup to a top-level group when parent_id is omitted.
func TransferSubGroup(ctx context.Context, client *gitlabclient.Client, input TransferSubGroupInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupTransferSubGroup: group_id is required")
	}
	opts := &gl.TransferSubGroupOptions{}
	if input.ParentID != nil {
		opts.GroupID = input.ParentID
	}
	g, _, err := client.GL().Groups.TransferSubGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupTransferSubGroup", err,
				"transferring a group requires Owner role on both the group and the destination parent group; use gitlab_group_transfer_locations to discover valid destinations")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("groupTransferSubGroup", err,
				"the destination parent is invalid (e.g. it would create a cycle, a path collision, or a visibility mismatch); use gitlab_group_transfer_locations to find valid parents")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("groupTransferSubGroup", err, http.StatusNotFound,
			"verify group_id (and parent_id) with gitlab_group_get")
	}
	return ToOutput(g), nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// FormatShareGroupMarkdown renders the result of a group-to-group share.
func FormatShareGroupMarkdown(out ShareGroupOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", toolutil.EmojiSuccess, out.Message)
	if out.AccessRole != "" {
		fmt.Fprintf(&b, "- **Access**: %s (%d)\n", out.AccessRole, out.GroupAccess)
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_group_shared_with_list` to confirm the share",
		"Use `gitlab_group_unshare_from_group` to revoke it",
	)
	return b.String()
}

// FormatSharedProjectsListMarkdown renders the projects shared with a group.
func FormatSharedProjectsListMarkdown(out SharedProjectsListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Shared Projects (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Projects), out.Pagination)
	if len(out.Projects) == 0 {
		b.WriteString("No shared projects found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Path | Visibility | Archived |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, p := range out.Projects {
		archived := "No"
		if p.Archived {
			archived = "Yes"
		}
		name := toolutil.EscapeMdTableCell(p.Name)
		if p.WebURL != "" {
			name = toolutil.MdTitleLink(name, p.WebURL)
		}
		fmt.Fprintf(
			&b, "| %d | %s | %s | %s | %s |\n",
			p.ID, name, toolutil.EscapeMdTableCell(p.PathWithNamespace), p.Visibility, archived,
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use `gitlab_project_get` to view a shared project's details",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatShareGroupMarkdown)
	toolutil.RegisterMarkdown(FormatSharedProjectsListMarkdown)
}
