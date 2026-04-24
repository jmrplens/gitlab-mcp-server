// Package groupprotectedbranches implements MCP tool handlers for GitLab
// group-level protected branch operations.
package groupprotectedbranches

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// AccessLevelOutput represents an access description for a group protected branch.
type AccessLevelOutput struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	DeployKeyID            int64  `json:"deploy_key_id,omitempty"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
}

// Output represents a single group-level protected branch.
type Output struct {
	toolutil.HintableOutput
	ID                        int64               `json:"id"`
	Name                      string              `json:"name"`
	PushAccessLevels          []AccessLevelOutput `json:"push_access_levels"`
	MergeAccessLevels         []AccessLevelOutput `json:"merge_access_levels"`
	UnprotectAccessLevels     []AccessLevelOutput `json:"unprotect_access_levels"`
	AllowForcePush            bool                `json:"allow_force_push"`
	CodeOwnerApprovalRequired bool                `json:"code_owner_approval_required"`
}

// ListOutput holds a paginated list of group protected branches.
type ListOutput struct {
	toolutil.HintableOutput
	Branches   []Output                  `json:"branches"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

func toAccessLevels(src []*gl.GroupBranchAccessDescription) []AccessLevelOutput {
	out := make([]AccessLevelOutput, len(src))
	for i, a := range src {
		out[i] = AccessLevelOutput{
			ID:                     a.ID,
			AccessLevel:            int(a.AccessLevel),
			AccessLevelDescription: a.AccessLevelDescription,
			DeployKeyID:            a.DeployKeyID,
			UserID:                 a.UserID,
			GroupID:                a.GroupID,
		}
	}
	return out
}

func toOutput(b *gl.GroupProtectedBranch) Output {
	return Output{
		ID:                        b.ID,
		Name:                      b.Name,
		PushAccessLevels:          toAccessLevels(b.PushAccessLevels),
		MergeAccessLevels:         toAccessLevels(b.MergeAccessLevels),
		UnprotectAccessLevels:     toAccessLevels(b.UnprotectAccessLevels),
		AllowForcePush:            b.AllowForcePush,
		CodeOwnerApprovalRequired: b.CodeOwnerApprovalRequired,
	}
}

// BranchPermissionInput represents a permission entry for protect/update operations.
type BranchPermissionInput struct {
	ID          *int64 `json:"id,omitempty"           jsonschema:"Existing permission ID to update"`
	UserID      *int64 `json:"user_id,omitempty"      jsonschema:"User ID"`
	GroupID     *int64 `json:"group_id,omitempty"     jsonschema:"Group ID"`
	DeployKeyID *int64 `json:"deploy_key_id,omitempty" jsonschema:"Deploy key ID"`
	AccessLevel *int   `json:"access_level,omitempty" jsonschema:"Access level (0=No access, 30=Developer, 40=Maintainer, 60=Admin)"`
	Destroy     *bool  `json:"_destroy,omitempty"     jsonschema:"Set true to remove this permission"`
}

// ListInput defines parameters for the List action which retrieves group-level protected branches.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Search  string               `json:"search,omitempty" jsonschema:"Search by branch name"`
	toolutil.PaginationInput
}

// GetInput defines parameters for the Get action which retrieves a single group-level protected branch.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Branch  string               `json:"branch"   jsonschema:"Branch name or wildcard,required"`
}

// ProtectInput defines parameters for the Protect action which creates a group-level protected branch rule.
type ProtectInput struct {
	GroupID                   toolutil.StringOrInt    `json:"group_id"                            jsonschema:"Group ID or URL-encoded path,required"`
	Name                      string                  `json:"name"                                jsonschema:"Branch name or wildcard to protect,required"`
	PushAccessLevel           *int                    `json:"push_access_level,omitempty"         jsonschema:"Push access level (0=No access, 30=Developer, 40=Maintainer, 60=Admin)"`
	MergeAccessLevel          *int                    `json:"merge_access_level,omitempty"        jsonschema:"Merge access level"`
	UnprotectAccessLevel      *int                    `json:"unprotect_access_level,omitempty"    jsonschema:"Unprotect access level"`
	AllowForcePush            *bool                   `json:"allow_force_push,omitempty"          jsonschema:"Allow force push"`
	CodeOwnerApprovalRequired *bool                   `json:"code_owner_approval_required,omitempty" jsonschema:"Require code owner approval"`
	AllowedToPush             []BranchPermissionInput `json:"allowed_to_push,omitempty"           jsonschema:"Users/groups allowed to push"`
	AllowedToMerge            []BranchPermissionInput `json:"allowed_to_merge,omitempty"          jsonschema:"Users/groups allowed to merge"`
	AllowedToUnprotect        []BranchPermissionInput `json:"allowed_to_unprotect,omitempty"      jsonschema:"Users/groups allowed to unprotect"`
}

// UpdateInput defines parameters for the Update action which modifies a group-level protected branch rule.
type UpdateInput struct {
	GroupID                   toolutil.StringOrInt    `json:"group_id"                            jsonschema:"Group ID or URL-encoded path,required"`
	Branch                    string                  `json:"branch"                              jsonschema:"Branch name or wildcard,required"`
	Name                      string                  `json:"name,omitempty"                      jsonschema:"New branch name or wildcard"`
	AllowForcePush            *bool                   `json:"allow_force_push,omitempty"          jsonschema:"Allow force push"`
	CodeOwnerApprovalRequired *bool                   `json:"code_owner_approval_required,omitempty" jsonschema:"Require code owner approval"`
	AllowedToPush             []BranchPermissionInput `json:"allowed_to_push,omitempty"           jsonschema:"Users/groups allowed to push"`
	AllowedToMerge            []BranchPermissionInput `json:"allowed_to_merge,omitempty"          jsonschema:"Users/groups allowed to merge"`
	AllowedToUnprotect        []BranchPermissionInput `json:"allowed_to_unprotect,omitempty"      jsonschema:"Users/groups allowed to unprotect"`
}

// UnprotectInput defines parameters for the Unprotect action which removes a group-level protected branch rule.
type UnprotectInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Branch  string               `json:"branch"   jsonschema:"Branch name or wildcard to unprotect,required"`
}

func toBranchPermissions(input []BranchPermissionInput) *[]*gl.GroupBranchPermissionOptions {
	if len(input) == 0 {
		return nil
	}
	perms := make([]*gl.GroupBranchPermissionOptions, len(input))
	for i, p := range input {
		o := &gl.GroupBranchPermissionOptions{
			ID:          p.ID,
			UserID:      p.UserID,
			GroupID:     p.GroupID,
			DeployKeyID: p.DeployKeyID,
			Destroy:     p.Destroy,
		}
		if p.AccessLevel != nil {
			v := gl.AccessLevelValue(*p.AccessLevel)
			o.AccessLevel = &v
		}
		perms[i] = o
	}
	return &perms
}

// List retrieves all group-level protected branches.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.ListGroupProtectedBranchesOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	branches, resp, err := client.GL().GroupProtectedBranches.ListProtectedBranches(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("listGroupProtectedBranches", err)
	}
	out := make([]Output, len(branches))
	for i, b := range branches {
		out[i] = toOutput(b)
	}
	return ListOutput{
		Branches:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get retrieves a single group-level protected branch.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Branch == "" {
		return Output{}, toolutil.ErrFieldRequired("branch")
	}
	b, _, err := client.GL().GroupProtectedBranches.GetProtectedBranch(string(input.GroupID), input.Branch, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("getGroupProtectedBranch", err)
	}
	return toOutput(b), nil
}

// Protect creates a new group-level protected branch rule.
func Protect(ctx context.Context, client *gitlabclient.Client, input ProtectInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}
	opts := &gl.ProtectGroupRepositoryBranchesOptions{
		Name:                      new(input.Name),
		AllowForcePush:            input.AllowForcePush,
		CodeOwnerApprovalRequired: input.CodeOwnerApprovalRequired,
		AllowedToPush:             toBranchPermissions(input.AllowedToPush),
		AllowedToMerge:            toBranchPermissions(input.AllowedToMerge),
		AllowedToUnprotect:        toBranchPermissions(input.AllowedToUnprotect),
	}
	if input.PushAccessLevel != nil {
		v := gl.AccessLevelValue(*input.PushAccessLevel)
		opts.PushAccessLevel = &v
	}
	if input.MergeAccessLevel != nil {
		v := gl.AccessLevelValue(*input.MergeAccessLevel)
		opts.MergeAccessLevel = &v
	}
	if input.UnprotectAccessLevel != nil {
		v := gl.AccessLevelValue(*input.UnprotectAccessLevel)
		opts.UnprotectAccessLevel = &v
	}
	b, _, err := client.GL().GroupProtectedBranches.ProtectRepositoryBranches(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("protectGroupBranch", err)
	}
	return toOutput(b), nil
}

// Update modifies a group-level protected branch rule.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Branch == "" {
		return Output{}, toolutil.ErrFieldRequired("branch")
	}
	opts := &gl.UpdateGroupProtectedBranchOptions{
		AllowForcePush:            input.AllowForcePush,
		CodeOwnerApprovalRequired: input.CodeOwnerApprovalRequired,
		AllowedToPush:             toBranchPermissions(input.AllowedToPush),
		AllowedToMerge:            toBranchPermissions(input.AllowedToMerge),
		AllowedToUnprotect:        toBranchPermissions(input.AllowedToUnprotect),
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	b, _, err := client.GL().GroupProtectedBranches.UpdateProtectedBranch(string(input.GroupID), input.Branch, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("updateGroupProtectedBranch", err)
	}
	return toOutput(b), nil
}

// Unprotect removes a group-level protected branch rule.
func Unprotect(ctx context.Context, client *gitlabclient.Client, input UnprotectInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.Branch == "" {
		return toolutil.ErrFieldRequired("branch")
	}
	_, err := client.GL().GroupProtectedBranches.UnprotectRepositoryBranches(string(input.GroupID), input.Branch, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("unprotectGroupBranch", err)
	}
	return nil
}
