// Package groups implements MCP tool handlers for GitLab group operations
// including list, get, list members, and list subgroups (descendant groups).
// It wraps the Groups service from client-go v2.
package groups

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing groups.
type ListInput struct {
	Search               string  `json:"search,omitempty"                jsonschema:"Filter groups by name or path"`
	Owned                bool    `json:"owned,omitempty"                 jsonschema:"Limit to groups explicitly owned by the authenticated user"`
	TopLevelOnly         bool    `json:"top_level_only,omitempty"        jsonschema:"Limit to top-level groups (exclude subgroups)"`
	OrderBy              string  `json:"order_by,omitempty"              jsonschema:"Order groups by field (name, path, id, similarity)"`
	Sort                 string  `json:"sort,omitempty"                  jsonschema:"Sort direction (asc, desc)"`
	Visibility           string  `json:"visibility,omitempty"            jsonschema:"Filter by visibility (public, internal, private)"`
	AllAvailable         bool    `json:"all_available,omitempty"         jsonschema:"Show all groups accessible by the authenticated user"`
	Statistics           bool    `json:"statistics,omitempty"            jsonschema:"Include group statistics (storage, counts)"`
	WithCustomAttributes bool    `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in the response"`
	SkipGroups           []int64 `json:"skip_groups,omitempty"           jsonschema:"Group IDs to exclude from results"`
	toolutil.PaginationInput
}

// Output represents a GitLab group.
type Output struct {
	toolutil.HintableOutput
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	FullPath              string `json:"full_path"`
	FullName              string `json:"full_name,omitempty"`
	Description           string `json:"description,omitempty"`
	Visibility            string `json:"visibility"`
	WebURL                string `json:"web_url"`
	ParentID              int64  `json:"parent_id,omitempty"`
	DefaultBranch         string `json:"default_branch,omitempty"`
	RequestAccessEnabled  bool   `json:"request_access_enabled"`
	CreatedAt             string `json:"created_at,omitempty"`
	MarkedForDeletion     string `json:"marked_for_deletion_on,omitempty"`
	AvatarURL             string `json:"avatar_url,omitempty"`
	ProjectCreationLevel  string `json:"project_creation_level,omitempty"`
	SubGroupCreationLevel string `json:"subgroup_creation_level,omitempty"`
	LFSEnabled            bool   `json:"lfs_enabled"`
	SharedRunnersSetting  string `json:"shared_runners_setting,omitempty"`
}

// ListOutput holds a paginated list of groups.
type ListOutput struct {
	toolutil.HintableOutput
	Groups     []Output                  `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetInput defines parameters for retrieving a single group.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// MembersListInput defines parameters for listing group members.
type MembersListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Query   string               `json:"query,omitempty" jsonschema:"Filter members by name or username"`
	toolutil.PaginationInput
}

// MemberOutput represents a GitLab group member.
type MemberOutput struct {
	ID                     int64  `json:"id"`
	Username               string `json:"username"`
	Name                   string `json:"name"`
	State                  string `json:"state"`
	AvatarURL              string `json:"avatar_url,omitempty"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	WebURL                 string `json:"web_url"`
	CreatedAt              string `json:"created_at,omitempty"`
	ExpiresAt              string `json:"expires_at,omitempty"`
	Email                  string `json:"email,omitempty"`
	GroupSAMLProvider      string `json:"group_saml_provider,omitempty"`
	MemberRoleName         string `json:"member_role_name,omitempty"`
}

// MemberListOutput holds a paginated list of group members.
type MemberListOutput struct {
	toolutil.HintableOutput
	Members    []MemberOutput            `json:"members"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// SubgroupsListInput defines parameters for listing subgroups.
type SubgroupsListInput struct {
	GroupID        toolutil.StringOrInt `json:"group_id"                jsonschema:"Group ID or URL-encoded path,required"`
	Search         string               `json:"search,omitempty"        jsonschema:"Filter subgroups by name or path"`
	AllAvailable   bool                 `json:"all_available,omitempty" jsonschema:"Show all subgroups accessible by the authenticated user"`
	Owned          bool                 `json:"owned,omitempty"         jsonschema:"Limit to subgroups explicitly owned by the authenticated user"`
	MinAccessLevel int                  `json:"min_access_level,omitempty" jsonschema:"Minimum access level (10=Guest,20=Reporter,30=Developer,40=Maintainer,50=Owner)"`
	OrderBy        string               `json:"order_by,omitempty"      jsonschema:"Order subgroups by field (name, path, id, similarity)"`
	Sort           string               `json:"sort,omitempty"          jsonschema:"Sort direction (asc, desc)"`
	Statistics     bool                 `json:"statistics,omitempty"    jsonschema:"Include group statistics (storage, counts)"`
	toolutil.PaginationInput
}

// ToOutput converts a GitLab API [gl.Group] to the MCP tool output
// format, extracting identifier, path, visibility, and parent information.
func ToOutput(g *gl.Group) Output {
	out := Output{
		ID:                    g.ID,
		Name:                  g.Name,
		Path:                  g.Path,
		FullPath:              g.FullPath,
		FullName:              g.FullName,
		Description:           g.Description,
		Visibility:            string(g.Visibility),
		WebURL:                g.WebURL,
		ParentID:              g.ParentID,
		DefaultBranch:         g.DefaultBranch,
		RequestAccessEnabled:  g.RequestAccessEnabled,
		AvatarURL:             g.AvatarURL,
		ProjectCreationLevel:  string(g.ProjectCreationLevel),
		SubGroupCreationLevel: string(g.SubGroupCreationLevel),
	}
	if g.CreatedAt != nil {
		out.CreatedAt = g.CreatedAt.Format(time.RFC3339)
	}
	if g.MarkedForDeletionOn != nil {
		out.MarkedForDeletion = g.MarkedForDeletionOn.String()
	}
	out.LFSEnabled = g.LFSEnabled
	out.SharedRunnersSetting = string(g.SharedRunnersSetting)
	return out
}

// MemberToOutput converts a GitLab API [gl.GroupMember] to the MCP
// tool output format, including a human-readable access level description.
func MemberToOutput(m *gl.GroupMember) MemberOutput {
	out := MemberOutput{
		ID:                     m.ID,
		Username:               m.Username,
		Name:                   m.Name,
		State:                  m.State,
		AvatarURL:              m.AvatarURL,
		AccessLevel:            int(m.AccessLevel),
		AccessLevelDescription: toolutil.AccessLevelDescription(m.AccessLevel),
		WebURL:                 m.WebURL,
		Email:                  m.Email,
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.ExpiresAt != nil {
		out.ExpiresAt = m.ExpiresAt.String()
	}
	if m.GroupSAMLIdentity != nil {
		out.GroupSAMLProvider = m.GroupSAMLIdentity.Provider
	}
	if m.MemberRole != nil {
		out.MemberRoleName = m.MemberRole.Name
	}
	return out
}

// List retrieves a paginated list of GitLab groups visible to the
// authenticated user. Supports filtering by search term, ownership, and
// top-level-only restriction. Returns the groups with pagination metadata.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := &gl.ListGroupsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.TopLevelOnly {
		opts.TopLevelOnly = new(true)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.AllAvailable {
		opts.AllAvailable = new(true)
	}
	if input.Statistics {
		opts.Statistics = new(true)
	}
	if input.WithCustomAttributes {
		opts.WithCustomAttributes = new(true)
	}
	if len(input.SkipGroups) > 0 {
		opts.SkipGroups = &input.SkipGroups
	}

	groups, resp, err := client.GL().Groups.ListGroups(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("List", err)
	}

	out := ListOutput{
		Groups:     make([]Output, len(groups)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// Get retrieves a single GitLab group by its ID or URL-encoded path.
// Returns the group details or an error if the group is not found.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("Get: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	g, _, err := client.GL().Groups.GetGroup(string(input.GroupID), &gl.GetGroupOptions{}, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("Get", err)
	}
	return ToOutput(g), nil
}

// MembersList retrieves all members of a GitLab group, including
// inherited members from parent groups. Supports filtering by name or
// username and pagination. Returns the member list with pagination metadata.
func MembersList(ctx context.Context, client *gitlabclient.Client, input MembersListInput) (MemberListOutput, error) {
	if err := ctx.Err(); err != nil {
		return MemberListOutput{}, err
	}
	if input.GroupID == "" {
		return MemberListOutput{}, errors.New("MembersList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListGroupMembersOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	if input.Query != "" {
		opts.Query = new(input.Query)
	}

	memberList, resp, err := client.GL().Groups.ListAllGroupMembers(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return MemberListOutput{}, toolutil.WrapErrWithMessage("MembersList", err)
	}

	out := MemberListOutput{
		Members:    make([]MemberOutput, len(memberList)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, m := range memberList {
		out.Members[i] = MemberToOutput(m)
	}
	return out, nil
}

// SubgroupsList retrieves a paginated list of descendant groups (subgroups)
// for a given parent group. Supports filtering by search term and pagination.
// Returns the subgroups with pagination metadata.
func SubgroupsList(ctx context.Context, client *gitlabclient.Client, input SubgroupsListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("SubgroupsList: group_id is required. Use gitlab_group_list to find the ID first, then pass it as group_id")
	}

	opts := &gl.ListDescendantGroupsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.AllAvailable {
		opts.AllAvailable = new(true)
	}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Statistics {
		opts.Statistics = new(true)
	}

	groups, resp, err := client.GL().Groups.ListDescendantGroups(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("SubgroupsList", err)
	}

	out := ListOutput{
		Groups:     make([]Output, len(groups)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Input types for new group operations
// ---------------------------------------------------------------------------.

// CreateInput defines parameters for creating a group.
type CreateInput struct {
	Name                 string `json:"name"                          jsonschema:"Group name,required"`
	Path                 string `json:"path,omitempty"                jsonschema:"Group URL path (defaults to kebab-case of name)"`
	Description          string `json:"description,omitempty"         jsonschema:"Group description"`
	Visibility           string `json:"visibility,omitempty"          jsonschema:"Visibility level (private, internal, public)"`
	ParentID             int64  `json:"parent_id,omitempty"           jsonschema:"Parent group ID (creates a subgroup)"`
	RequestAccessEnabled *bool  `json:"request_access_enabled,omitempty" jsonschema:"Allow users to request access"`
	LFSEnabled           *bool  `json:"lfs_enabled,omitempty"         jsonschema:"Enable Git LFS"`
	DefaultBranch        string `json:"default_branch,omitempty"      jsonschema:"Default branch name"`
}

// UpdateInput defines parameters for updating a group.
type UpdateInput struct {
	GroupID              toolutil.StringOrInt `json:"group_id"                jsonschema:"Group ID or URL-encoded path,required"`
	Name                 string               `json:"name,omitempty"          jsonschema:"Group name"`
	Path                 string               `json:"path,omitempty"          jsonschema:"Group URL path"`
	Description          string               `json:"description,omitempty"   jsonschema:"Group description"`
	Visibility           string               `json:"visibility,omitempty"    jsonschema:"Visibility level (private, internal, public)"`
	RequestAccessEnabled *bool                `json:"request_access_enabled,omitempty" jsonschema:"Allow users to request access"`
	LFSEnabled           *bool                `json:"lfs_enabled,omitempty"   jsonschema:"Enable Git LFS"`
	DefaultBranch        string               `json:"default_branch,omitempty" jsonschema:"Default branch name"`
}

// DeleteInput defines parameters for deleting a group.
type DeleteInput struct {
	GroupID           toolutil.StringOrInt `json:"group_id"                    jsonschema:"Group ID or URL-encoded path,required"`
	PermanentlyRemove bool                 `json:"permanently_remove,omitempty" jsonschema:"Permanently remove instead of marking for deletion"`
	FullPath          string               `json:"full_path,omitempty"          jsonschema:"Full path (required when permanently_remove=true)"`
}

// RestoreInput defines parameters for restoring a group marked for deletion.
type RestoreInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// ArchiveInput defines parameters for archiving or unarchiving a group.
type ArchiveInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// SearchInput defines parameters for searching groups.
type SearchInput struct {
	Query string `json:"query" jsonschema:"Search query string,required"`
}

// TransferInput defines parameters for transferring a project to a group.
type TransferInput struct {
	GroupID   toolutil.StringOrInt `json:"group_id"    jsonschema:"Group ID or URL-encoded path,required"`
	ProjectID toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path to transfer,required"`
}

// ListProjectsInput defines parameters for listing group projects.
type ListProjectsInput struct {
	GroupID          toolutil.StringOrInt `json:"group_id"                  jsonschema:"Group ID or URL-encoded path,required"`
	Search           string               `json:"search,omitempty"          jsonschema:"Filter projects by name"`
	Archived         *bool                `json:"archived,omitempty"        jsonschema:"Filter archived projects"`
	Visibility       string               `json:"visibility,omitempty"      jsonschema:"Filter by visibility (public, internal, private)"`
	OrderBy          string               `json:"order_by,omitempty"        jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at, similarity)"`
	Sort             string               `json:"sort,omitempty"            jsonschema:"Sort direction (asc, desc)"`
	Simple           bool                 `json:"simple,omitempty"          jsonschema:"Return limited fields"`
	Owned            bool                 `json:"owned,omitempty"           jsonschema:"Limit to projects owned by current user"`
	Starred          bool                 `json:"starred,omitempty"         jsonschema:"Limit to starred projects"`
	IncludeSubGroups bool                 `json:"include_subgroups,omitempty" jsonschema:"Include projects in subgroups"`
	WithShared       *bool                `json:"with_shared,omitempty"     jsonschema:"Include shared projects"`
	toolutil.PaginationInput
}

// ProjectItem is a simplified project representation for group context.
type ProjectItem struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Description       string `json:"description,omitempty"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch,omitempty"`
	Archived          bool   `json:"archived"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// ListProjectsOutput holds a paginated list of group projects.
type ListProjectsOutput struct {
	toolutil.HintableOutput
	Projects   []ProjectItem             `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// Create creates a new GitLab group.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.Name == "" {
		return Output{}, errors.New("groupCreate: name is required")
	}

	opts := &gl.CreateGroupOptions{
		Name: new(input.Name),
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.ParentID != 0 {
		opts.ParentID = new(input.ParentID)
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}

	g, _, err := client.GL().Groups.CreateGroup(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupCreate", err, "creating groups requires Owner role on the parent namespace")
		}
		return Output{}, toolutil.WrapErrWithMessage("groupCreate", err)
	}
	return ToOutput(g), nil
}

// Update modifies an existing GitLab group.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("groupUpdate: group_id is required")
	}

	opts := &gl.UpdateGroupOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}

	g, _, err := client.GL().Groups.UpdateGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("groupUpdate", err, "group updates require Owner role on the group")
		}
		return Output{}, toolutil.WrapErrWithMessage("groupUpdate", err)
	}
	return ToOutput(g), nil
}

// Delete removes a GitLab group.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.GroupID == "" {
		return errors.New("groupDelete: group_id is required")
	}

	opts := &gl.DeleteGroupOptions{}
	if input.PermanentlyRemove {
		opts.PermanentlyRemove = new(true)
		if input.FullPath != "" {
			opts.FullPath = new(input.FullPath)
		}
	}

	_, err := client.GL().Groups.DeleteGroup(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("groupDelete", err, "only group owners can delete groups")
		}
		return toolutil.WrapErrWithMessage("groupDelete", err)
	}
	return nil
}

// Restore restores a group that was marked for deletion.
func Restore(ctx context.Context, client *gitlabclient.Client, input RestoreInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("groupRestore: group_id is required")
	}

	g, _, err := client.GL().Groups.RestoreGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("groupRestore", err)
	}
	return ToOutput(g), nil
}

// Archive archives a GitLab group. Requires Owner role or administrator.
func Archive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) error {
	if input.GroupID == "" {
		return errors.New("groupArchive: group_id is required")
	}

	_, err := client.GL().Groups.ArchiveGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("groupArchive", err, "archiving groups requires Owner role or administrator")
		}
		return toolutil.WrapErrWithMessage("groupArchive", err)
	}
	return nil
}

// Unarchive unarchives a GitLab group. Requires Owner role or administrator.
func Unarchive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) error {
	if input.GroupID == "" {
		return errors.New("groupUnarchive: group_id is required")
	}

	_, err := client.GL().Groups.UnarchiveGroup(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("groupUnarchive", err, "unarchiving groups requires Owner role or administrator")
		}
		return toolutil.WrapErrWithMessage("groupUnarchive", err)
	}
	return nil
}

// Search searches for groups by query string.
func Search(ctx context.Context, client *gitlabclient.Client, input SearchInput) (ListOutput, error) {
	if input.Query == "" {
		return ListOutput{}, errors.New("groupSearch: query is required")
	}

	groups, _, err := client.GL().Groups.SearchGroup(input.Query, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("groupSearch", err)
	}

	out := ListOutput{
		Groups: make([]Output, len(groups)),
	}
	for i, g := range groups {
		out.Groups[i] = ToOutput(g)
	}
	return out, nil
}

// TransferProject transfers a project into the group namespace.
func TransferProject(ctx context.Context, client *gitlabclient.Client, input TransferInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, errors.New("groupTransferProject: group_id is required")
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("groupTransferProject: project_id is required")
	}

	g, _, err := client.GL().Groups.TransferGroup(string(input.GroupID), string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("groupTransferProject", err)
	}
	return ToOutput(g), nil
}

// ListProjects retrieves projects belonging to a group.
func ListProjects(ctx context.Context, client *gitlabclient.Client, input ListProjectsInput) (ListProjectsOutput, error) {
	if input.GroupID == "" {
		return ListProjectsOutput{}, errors.New("groupListProjects: group_id is required")
	}

	opts := &gl.ListGroupProjectsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Simple {
		opts.Simple = new(true)
	}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.Starred {
		opts.Starred = new(true)
	}
	if input.IncludeSubGroups {
		opts.IncludeSubGroups = new(true)
	}
	if input.WithShared != nil {
		opts.WithShared = input.WithShared
	}

	projects, resp, err := client.GL().Groups.ListGroupProjects(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectsOutput{}, toolutil.WrapErrWithMessage("groupListProjects", err)
	}

	items := make([]ProjectItem, len(projects))
	for i, p := range projects {
		items[i] = ProjectItem{
			ID:                p.ID,
			Name:              p.Name,
			PathWithNamespace: p.PathWithNamespace,
			Description:       p.Description,
			Visibility:        string(p.Visibility),
			WebURL:            p.WebURL,
			DefaultBranch:     p.DefaultBranch,
			Archived:          p.Archived,
		}
		if p.CreatedAt != nil {
			items[i].CreatedAt = p.CreatedAt.Format(time.RFC3339)
		}
	}
	return ListProjectsOutput{Projects: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
