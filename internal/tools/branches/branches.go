// Package branches implements MCP tool handlers for GitLab branch operations
// including create, list, get, delete, and branch protection management.
// It wraps the Branches and ProtectedBranches services from client-go v2.
package branches

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for creating a new branch.
type CreateInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	BranchName string               `json:"branch_name" jsonschema:"New branch name (param 'branch_name' not 'branch' or 'name'),required"`
	Ref        string               `json:"ref"         jsonschema:"Branch name, tag, or commit SHA to create from,required"`
}

// Output represents a Git branch.
type Output struct {
	toolutil.HintableOutput
	Name               string `json:"name"`
	Merged             bool   `json:"merged"`
	Protected          bool   `json:"protected"`
	Default            bool   `json:"default"`
	WebURL             string `json:"web_url"`
	CommitID           string `json:"commit_id"`
	CanPush            bool   `json:"can_push"`
	DevelopersCanPush  bool   `json:"developers_can_push"`
	DevelopersCanMerge bool   `json:"developers_can_merge"`
}

// ListInput defines parameters for listing branches.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search    string               `json:"search,omitempty" jsonschema:"Filter branches by name (substring match)"`
	toolutil.PaginationInput
}

// ListOutput holds a paginated list of branches.
type ListOutput struct {
	toolutil.HintableOutput
	Branches   []Output                  `json:"branches"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ProtectInput defines parameters for protecting a branch.
type ProtectInput struct {
	ProjectID                 toolutil.StringOrInt `json:"project_id"                          jsonschema:"Project ID or URL-encoded path,required"`
	BranchName                string               `json:"branch_name"                         jsonschema:"Branch name or wildcard (e.g. 'main' or 'release/*'),required"`
	PushAccessLevel           int                  `json:"push_access_level,omitempty"         jsonschema:"Access level for push (0=No access 30=Developer 40=Maintainer)"`
	MergeAccessLevel          int                  `json:"merge_access_level,omitempty"        jsonschema:"Access level for merge (0=No access 30=Developer 40=Maintainer)"`
	AllowForcePush            *bool                `json:"allow_force_push,omitempty"          jsonschema:"Allow force push to this branch"`
	CodeOwnerApprovalRequired *bool                `json:"code_owner_approval_required,omitempty" jsonschema:"Require CODEOWNERS approval for changes to matching files"`
}

// ProtectedOutput represents a protected branch.
type ProtectedOutput struct {
	toolutil.HintableOutput
	ID                        int64  `json:"id"`
	Name                      string `json:"name"`
	PushAccessLevel           int    `json:"push_access_level"`
	MergeAccessLevel          int    `json:"merge_access_level"`
	AllowForcePush            bool   `json:"allow_force_push"`
	CodeOwnerApprovalRequired bool   `json:"code_owner_approval_required"`
	AlreadyProtected          bool   `json:"already_protected,omitempty"`
}

// UnprotectInput defines parameters for unprotecting a branch.
type UnprotectInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	BranchName string               `json:"branch_name" jsonschema:"Name of the protected branch to remove,required"`
}

// ProtectedListInput defines parameters for listing protected branches.
type ProtectedListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ProtectedListOutput holds the list of protected branches.
type ProtectedListOutput struct {
	toolutil.HintableOutput
	Branches   []ProtectedOutput         `json:"branches"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ProtectedToOutput converts a GitLab API [gl.ProtectedBranch] to the
// MCP tool output format, extracting push and merge access levels from the
// first entry of each access level slice.
func ProtectedToOutput(b *gl.ProtectedBranch) ProtectedOutput {
	out := ProtectedOutput{
		ID:                        b.ID,
		Name:                      b.Name,
		AllowForcePush:            b.AllowForcePush,
		CodeOwnerApprovalRequired: b.CodeOwnerApprovalRequired,
	}
	if len(b.PushAccessLevels) > 0 {
		out.PushAccessLevel = int(b.PushAccessLevels[0].AccessLevel)
	}
	if len(b.MergeAccessLevels) > 0 {
		out.MergeAccessLevel = int(b.MergeAccessLevels[0].AccessLevel)
	}
	return out
}

// Protect protects a branch in the specified GitLab project by calling
// the Protected Branches API. It configures push and merge access levels and
// optionally enables force push. Returns an error if the API call fails.
func Protect(ctx context.Context, client *gitlabclient.Client, input ProtectInput) (ProtectedOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedOutput{}, err
	}
	if input.ProjectID == "" {
		return ProtectedOutput{}, errors.New("branchProtect: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.BranchName == "" {
		return ProtectedOutput{}, toolutil.ErrRequiredString("branchProtect", "branch_name")
	}
	opts := &gl.ProtectRepositoryBranchesOptions{
		Name:             new(input.BranchName),
		PushAccessLevel:  new(gl.AccessLevelValue(input.PushAccessLevel)),
		MergeAccessLevel: new(gl.AccessLevelValue(input.MergeAccessLevel)),
	}
	if input.AllowForcePush != nil {
		opts.AllowForcePush = input.AllowForcePush
	}
	if input.CodeOwnerApprovalRequired != nil {
		opts.CodeOwnerApprovalRequired = input.CodeOwnerApprovalRequired
	}
	b, _, err := client.GL().ProtectedBranches.ProtectRepositoryBranches(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		// 409 Conflict means branch is already protected — idempotent success
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			existing, _, getErr := client.GL().ProtectedBranches.GetProtectedBranch(string(input.ProjectID), input.BranchName, gl.WithContext(ctx))
			if getErr != nil {
				return ProtectedOutput{}, toolutil.WrapErrWithHint("branchProtect", err,
					"protected branch rule already exists but could not retrieve current settings. Use gitlab_protected_branch_get to view current rules, or gitlab_protected_branch_update to modify them")
			}
			out := ProtectedToOutput(existing)
			out.AlreadyProtected = true
			return out, nil
		}
		return ProtectedOutput{}, toolutil.WrapErrWithMessage("branchProtect", err)
	}
	return ProtectedToOutput(b), nil
}

// UnprotectOutput holds the result of an unprotect operation.
type UnprotectOutput struct {
	toolutil.HintableOutput
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Unprotect removes protection from a branch in the specified GitLab project.
// The operation is idempotent: if the branch is not protected, it returns
// success with an informational message instead of an error.
func Unprotect(ctx context.Context, client *gitlabclient.Client, input UnprotectInput) (UnprotectOutput, error) {
	if err := ctx.Err(); err != nil {
		return UnprotectOutput{}, err
	}
	if input.ProjectID == "" {
		return UnprotectOutput{}, errors.New("branchUnprotect: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.BranchName == "" {
		return UnprotectOutput{}, toolutil.ErrRequiredString("branchUnprotect", "branch_name")
	}
	_, err := client.GL().ProtectedBranches.UnprotectRepositoryBranches(string(input.ProjectID), input.BranchName, gl.WithContext(ctx))
	if err != nil {
		// 404 means the branch is not protected — idempotent success.
		// client-go may return *ErrorResponse (with status code) or plain error "404 Not Found".
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) || toolutil.ContainsAny(err, "404") {
			return UnprotectOutput{
				Status:  "already_unprotected",
				Message: fmt.Sprintf("Branch %q in project %s is not protected — no action needed.", input.BranchName, input.ProjectID),
			}, nil
		}
		return UnprotectOutput{}, toolutil.WrapErrWithMessage("branchUnprotect", err)
	}
	return UnprotectOutput{
		Status:  "success",
		Message: fmt.Sprintf("Protection removed from branch %q in project %s.", input.BranchName, input.ProjectID),
	}, nil
}

// ProtectedList retrieves a paginated list of protected branches for
// the specified GitLab project. Pagination parameters are forwarded to the API.
func ProtectedList(ctx context.Context, client *gitlabclient.Client, input ProtectedListInput) (ProtectedListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedListOutput{}, err
	}
	if input.ProjectID == "" {
		return ProtectedListOutput{}, errors.New("protectedBranchesList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.ListProtectedBranchesOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	branches, resp, err := client.GL().ProtectedBranches.ListProtectedBranches(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ProtectedListOutput{}, toolutil.WrapErrWithMessage("protectedBranchesList", err)
	}
	out := make([]ProtectedOutput, len(branches))
	for i, b := range branches {
		out[i] = ProtectedToOutput(b)
	}
	return ProtectedListOutput{Branches: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ToOutput converts a GitLab API [gl.Branch] to the MCP tool output
// format, extracting the commit ID from the embedded commit if present.
func ToOutput(b *gl.Branch) Output {
	out := Output{
		Name:               b.Name,
		Merged:             b.Merged,
		Protected:          b.Protected,
		Default:            b.Default,
		WebURL:             b.WebURL,
		CanPush:            b.CanPush,
		DevelopersCanPush:  b.DevelopersCanPush,
		DevelopersCanMerge: b.DevelopersCanMerge,
	}
	if b.Commit != nil {
		out.CommitID = b.Commit.ID
	}
	return out
}

// Create creates a new branch in the specified GitLab project from the
// given ref (branch, tag, or commit SHA). Returns the created branch details
// or an error if the ref does not exist or the branch already exists.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("branchCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.BranchName == "" {
		return Output{}, toolutil.ErrRequiredString("branchCreate", "branch_name")
	}
	b, _, err := client.GL().Branches.CreateBranch(string(input.ProjectID), &gl.CreateBranchOptions{
		Branch: new(input.BranchName),
		Ref:    new(input.Ref),
	}, gl.WithContext(ctx))
	if err != nil {
		if toolutil.ContainsAny(err, "invalid reference", "not found", "does not exist") {
			return Output{}, fmt.Errorf("branchCreate: ref '%s' not found. Use gitlab_branch_list to see available branches or check the project's default branch: %w", input.Ref, err)
		}
		if toolutil.ContainsAny(err, "already exists") {
			return Output{}, toolutil.WrapErrWithHint("branchCreate", err,
				"a branch with this name already exists. Use gitlab_branch_get to check it, or choose a different name")
		}
		return Output{}, toolutil.WrapErrWithMessage("branchCreate", err)
	}
	return ToOutput(b), nil
}

// List retrieves a paginated list of branches for the specified GitLab
// project. An optional search filter restricts results by branch name substring.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("branchList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.ListBranchesOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	branches, resp, err := client.GL().Branches.ListBranches(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("branchList", err)
	}
	out := make([]Output, len(branches))
	for i, b := range branches {
		out[i] = ToOutput(b)
	}
	return ListOutput{Branches: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInput defines parameters for retrieving a single branch.
type GetInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	BranchName string               `json:"branch_name" jsonschema:"Branch name to retrieve (param 'branch_name' not 'branch'),required"`
}

// Get retrieves a single branch by name from a GitLab project.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("branchGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.BranchName == "" {
		return Output{}, toolutil.ErrRequiredString("branchGet", "branch_name")
	}

	b, _, err := client.GL().Branches.GetBranch(string(input.ProjectID), input.BranchName, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("branchGet", err)
	}
	return ToOutput(b), nil
}

// DeleteInput defines parameters for deleting a branch.
type DeleteInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	BranchName string               `json:"branch_name" jsonschema:"Branch name to delete,required"`
}

// Delete deletes a branch from a GitLab project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("branchDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.BranchName == "" {
		return toolutil.ErrRequiredString("branchDelete", "branch_name")
	}

	_, err := client.GL().Branches.DeleteBranch(string(input.ProjectID), input.BranchName, gl.WithContext(ctx))
	if err != nil {
		if toolutil.ContainsAny(err, "protected branch") {
			return toolutil.WrapErrWithHint("branchDelete", err,
				"use gitlab_branch_unprotect first, then retry deletion")
		}
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return toolutil.WrapErrWithHint("branchDelete", err,
				"branch not found. Use gitlab_branch_list to verify the branch name")
		}
		return toolutil.WrapErrWithMessage("branchDelete", err)
	}
	return nil
}

// DeleteMergedInput defines parameters for deleting all merged branches in a project.
type DeleteMergedInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DeleteMerged deletes all branches that have been merged into the default branch.
// The default branch and protected branches are never deleted.
func DeleteMerged(ctx context.Context, client *gitlabclient.Client, input DeleteMergedInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("deleteMergedBranches: project_id is required")
	}
	_, err := client.GL().Branches.DeleteMergedBranches(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("deleteMergedBranches", err)
	}
	return nil
}

// ProtectedGetInput defines parameters for retrieving a single protected branch.
type ProtectedGetInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	BranchName string               `json:"branch_name" jsonschema:"Name of the protected branch,required"`
}

// ProtectedGet retrieves a single protected branch by name.
func ProtectedGet(ctx context.Context, client *gitlabclient.Client, input ProtectedGetInput) (ProtectedOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedOutput{}, err
	}
	if input.ProjectID == "" {
		return ProtectedOutput{}, errors.New("protectedBranchGet: project_id is required")
	}
	if input.BranchName == "" {
		return ProtectedOutput{}, toolutil.ErrRequiredString("protectedBranchGet", "branch_name")
	}
	b, _, err := client.GL().ProtectedBranches.GetProtectedBranch(string(input.ProjectID), input.BranchName, gl.WithContext(ctx))
	if err != nil {
		return ProtectedOutput{}, toolutil.WrapErrWithMessage("protectedBranchGet", err)
	}
	return ProtectedToOutput(b), nil
}

// ProtectedUpdateInput defines parameters for updating a protected branch's settings.
type ProtectedUpdateInput struct {
	ProjectID                 toolutil.StringOrInt `json:"project_id"                          jsonschema:"Project ID or URL-encoded path,required"`
	BranchName                string               `json:"branch_name"                         jsonschema:"Name of the protected branch,required"`
	AllowForcePush            *bool                `json:"allow_force_push,omitempty"          jsonschema:"Allow force push to this branch"`
	CodeOwnerApprovalRequired *bool                `json:"code_owner_approval_required,omitempty" jsonschema:"Require CODEOWNERS approval"`
}

// ProtectedUpdate updates settings on an existing protected branch.
func ProtectedUpdate(ctx context.Context, client *gitlabclient.Client, input ProtectedUpdateInput) (ProtectedOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedOutput{}, err
	}
	if input.ProjectID == "" {
		return ProtectedOutput{}, errors.New("protectedBranchUpdate: project_id is required")
	}
	if input.BranchName == "" {
		return ProtectedOutput{}, toolutil.ErrRequiredString("protectedBranchUpdate", "branch_name")
	}
	opts := &gl.UpdateProtectedBranchOptions{}
	if input.AllowForcePush != nil {
		opts.AllowForcePush = input.AllowForcePush
	}
	if input.CodeOwnerApprovalRequired != nil {
		opts.CodeOwnerApprovalRequired = input.CodeOwnerApprovalRequired
	}
	b, _, err := client.GL().ProtectedBranches.UpdateProtectedBranch(string(input.ProjectID), input.BranchName, opts, gl.WithContext(ctx))
	if err != nil {
		return ProtectedOutput{}, toolutil.WrapErrWithMessage("protectedBranchUpdate", err)
	}
	return ProtectedToOutput(b), nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
