package groups

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// ShareGroupWithGroup
// ---------------------------------------------------------------------------.

// ShareGroupInput defines parameters for sharing a group with another group.
//
// group.group_member_share reaches the same route with an input of its own
// (groupmembers.ShareInput), which names the group shared with share_group_id.
// The two describe group_access and member_role_id alike, and member_role_id
// carries the same tier, so neither schema tells a caller something the other
// contradicts.
type ShareGroupInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"      jsonschema:"Group ID or URL-encoded path of the group being shared,required"`
	SharedGroupID int64                `json:"shared_group_id" jsonschema:"ID of the group to share with,required"`
	GroupAccess   int                  `json:"group_access"  jsonschema:"Access level the members of the group shared with gain (5=Minimal access (Premium/Ultimate), 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner). 60=Admin is not valid for group shares,required"`
	ExpiresAt     string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the share (YYYY-MM-DD)"`
	MemberRoleID  int64                `json:"member_role_id,omitempty" jsonschema:"Custom member role the share grants (Ultimate only). Its base access level must equal group_access" tier:"ultimate"`
}

// ShareGroupOutput holds the result of sharing a group with another group.
type ShareGroupOutput struct {
	toolutil.HintableOutput
	Message       string `json:"message"`
	SharedGroupID int64  `json:"shared_group_id,omitempty"`
	GroupAccess   int    `json:"group_access,omitempty"`
	AccessRole    string `json:"access_role,omitempty"`
}

// ShareGroupWithGroup shares a group with another group.
func ShareGroupWithGroup(ctx context.Context, client *gitlabclient.Client, input ShareGroupInput) (ShareGroupOutput, error) {
	if err := ctx.Err(); err != nil {
		return ShareGroupOutput{}, err
	}
	if input.GroupID == "" {
		return ShareGroupOutput{}, errors.New("groupShareWithGroup: group_id is required. Use group.list to find the ID, then pass it as group_id")
	}
	if input.SharedGroupID == 0 {
		return ShareGroupOutput{}, errors.New("groupShareWithGroup: shared_group_id is required. Use group.list to find the group ID to share with")
	}
	if input.GroupAccess == 0 {
		return ShareGroupOutput{}, errors.New("groupShareWithGroup: group_access is required. Valid levels: 10 (Guest), 15 (Planner), 20 (Reporter), 25 (Security Manager), 30 (Developer), 40 (Maintainer), 50 (Owner), and 5 (Minimal access) on Premium or Ultimate")
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
	if input.MemberRoleID != 0 {
		opts.MemberRoleID = new(input.MemberRoleID)
	}
	_, _, err := client.GL().Groups.ShareGroupWithGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ShareGroupOutput{}, shareGroupError(err)
	}
	roleName := toolutil.AccessLevelDescription(gl.AccessLevelValue(input.GroupAccess))
	return ShareGroupOutput{
		Message:       fmt.Sprintf("Group %s shared with group %d as %s", input.GroupID, input.SharedGroupID, roleName),
		SharedGroupID: input.SharedGroupID,
		GroupAccess:   input.GroupAccess,
		AccessRole:    roleName,
	}, nil
}

// The hints below say what POST /groups/:id/share answers each status for,
// read off lib/api/groups.rb and Groups::GroupLinks::CreateService (with
// GroupLinkable) at GitLab 19.4.1. Grape answers 400 for a parameter it
// refuses. The service answers 404 when the caller may not create a group
// link on this group (create_group_link, which the Owner role grants), may
// not read the other group, or the top-level group keeps shares inside its
// hierarchy, and 409 for every link the model refuses to save. Nothing on the
// route answers 422. A 403 comes from the credential itself: a fine-grained
// token without the share_group permission.
const (
	shareGroupBadRequestHint = "group_access must be 10/15/20/25/30/40/50 (Guest/Planner/Reporter/Security Manager/Developer/Maintainer/Owner), or 5 (Minimal access) on a Premium or Ultimate top-level group; expires_at must be YYYY-MM-DD"
	shareGroupConflictHint   = "GitLab refused to record the share. The group may already be shared with this group (use group.shared_with to verify, and group.unshare_from_group first to change it); with member_role_id, the custom role must belong to this group's top-level group and its base access level must equal group_access; 5 (Minimal access) needs a Premium or Ultimate license and a top-level group; and where the top-level group restricts membership by email domain, the other group's allowed domains must be a subset of it"
	shareGroupRefusedHint    = "the credential may not share groups: a fine-grained personal access token needs the share_group permission. A caller without the Owner role on this group, or without read access to the other group, is answered 404 rather than 403"
	shareGroupNotFoundHint   = "verify group_id and shared_group_id with group.get. GitLab also answers 404 when the caller may not link this group (the Owner role grants that) or read the other group, and when the top-level group prevents sharing outside its hierarchy"
)

// shareGroupError wraps a failed POST /groups/:id/share with the hint for the
// status GitLab answered it with.
func shareGroupError(err error) error {
	switch {
	case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
		return toolutil.WrapErrWithHint("groupShareWithGroup", err, shareGroupBadRequestHint)
	case toolutil.IsHTTPStatus(err, http.StatusConflict):
		return toolutil.WrapErrWithHint("groupShareWithGroup", err, shareGroupConflictHint)
	case toolutil.IsPermissionRefusal(err):
		return toolutil.WrapErrWithHint("groupShareWithGroup", err, shareGroupRefusedHint)
	default:
		return toolutil.WrapErrWithStatusHint("groupShareWithGroup", err, http.StatusNotFound, shareGroupNotFoundHint)
	}
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
			"the group is not shared with this group, or the IDs are wrong. Use group.shared_with to verify; requires Owner role")
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
	MinAccessLevel           int                  `json:"min_access_level,omitempty" jsonschema:"Limit to projects where the caller has at least this access level (5=Minimal access,10=Guest,15=Planner,20=Reporter,25=Security Manager,30=Developer,40=Maintainer,50=Owner)"`
	OrderBy                  string               `json:"order_by,omitempty"        jsonschema:"Order by field: id, name, path, created_at, updated_at, star_count, or last_activity_at. Default is created_at"`
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
			"verify group_id with group.get. Shared projects are projects shared *into* this group from elsewhere, not the group's own projects")
	}
	return SharedProjectsListOutput{Projects: projectItemsFromGroup(projects, input.Simple != nil && *input.Simple), Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.

// FormatShareGroupMarkdown renders the result of a group-to-group share as the
// card of what was created: the confirmation is the heading, and the share's
// own fields are the rows.
func FormatShareGroupMarkdown(out ShareGroupOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, toolutil.EmojiSuccess+" "+out.Message)
	c.Count("Shared Group ID", out.SharedGroupID)
	c.Field("Access", out.AccessRole)
	c.Count("Access Level", int64(out.GroupAccess))
	c.End(
		toolutil.HintAction(actionGroupSharedWith, "confirm the share"),
		toolutil.HintAction(actionGroupUnshare, "revoke it"),
	)
	return b.String()
}

// FormatSharedProjectsListMarkdown renders the projects shared with a group.
func FormatSharedProjectsListMarkdown(out SharedProjectsListOutput) string {
	if len(out.Projects) == 0 {
		return toolutil.EmptyMessage("shared projects")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Shared Projects", len(out.Projects), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Path", "Visibility", "Archived"))
	for _, p := range out.Projects {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(p.ID, 10),
			toolutil.MdTitleLink(p.Name, p.WebURL),
			toolutil.EscapeMdTableCell(p.PathWithNamespace),
			toolutil.EscapeMdTableCell(p.Visibility),
			archivedCell(p),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionProjectGet, "view a shared project's details"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatShareGroupMarkdown)
	toolutil.RegisterMarkdown(FormatSharedProjectsListMarkdown)
}
