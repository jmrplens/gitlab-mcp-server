// Package externalstatuschecks implements MCP tool handlers for GitLab
// external status check operations. It wraps the ExternalStatusChecks
// service from client-go v2, covering both deprecated and current endpoints.
package externalstatuschecks

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

type MergeStatusCheckOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ExternalURL string `json:"external_url"`
	Status      string `json:"status"`
}

type ProjectStatusCheckOutput struct {
	toolutil.HintableOutput
	ID                int64                   `json:"id"`
	Name              string                  `json:"name"`
	ProjectID         int64                   `json:"project_id"`
	ExternalURL       string                  `json:"external_url"`
	HMAC              bool                    `json:"hmac"`
	ProtectedBranches []ProtectedBranchOutput `json:"protected_branches,omitempty"`
}

type ProtectedBranchOutput struct {
	ID                        int64  `json:"id"`
	ProjectID                 int64  `json:"project_id"`
	Name                      string `json:"name"`
	CodeOwnerApprovalRequired bool   `json:"code_owner_approval_required"`
}

type ListMergeStatusCheckOutput struct {
	toolutil.HintableOutput
	Items      []MergeStatusCheckOutput  `json:"items"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

type ListProjectStatusCheckOutput struct {
	toolutil.HintableOutput
	Items      []ProjectStatusCheckOutput `json:"items"`
	Pagination toolutil.PaginationOutput  `json:"pagination"`
}

func toMergeStatusCheckOutput(c *gl.MergeStatusCheck) MergeStatusCheckOutput {
	return MergeStatusCheckOutput{
		ID:          c.ID,
		Name:        c.Name,
		ExternalURL: c.ExternalURL,
		Status:      c.Status,
	}
}

func toProjectStatusCheckOutput(c *gl.ProjectStatusCheck) ProjectStatusCheckOutput {
	out := ProjectStatusCheckOutput{
		ID:          c.ID,
		Name:        c.Name,
		ProjectID:   c.ProjectID,
		ExternalURL: c.ExternalURL,
		HMAC:        c.HMAC,
	}
	for _, pb := range c.ProtectedBranches {
		out.ProtectedBranches = append(out.ProtectedBranches, ProtectedBranchOutput{
			ID:                        pb.ID,
			ProjectID:                 pb.ProjectID,
			Name:                      pb.Name,
			CodeOwnerApprovalRequired: pb.CodeOwnerApprovalRequired,
		})
	}
	return out
}

type ListMergeStatusChecksInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	toolutil.PaginationInput
}

// ListMergeStatusChecks lists merge status checks for a merge request (deprecated).
func ListMergeStatusChecks(ctx context.Context, client *gitlabclient.Client, input ListMergeStatusChecksInput) (ListMergeStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListMergeStatusCheckOutput{}, err
	}
	if input.ProjectID == "" {
		return ListMergeStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return ListMergeStatusCheckOutput{}, toolutil.ErrRequiredInt64("listMergeStatusChecks", "mr_iid")
	}
	opts := &gl.ListOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	checks, resp, err := client.GL().ExternalStatusChecks.ListMergeStatusChecks(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMergeStatusCheckOutput{}, toolutil.WrapErrWithMessage("listMergeStatusChecks", err)
	}
	items := make([]MergeStatusCheckOutput, len(checks))
	for i, c := range checks {
		items[i] = toMergeStatusCheckOutput(c)
	}
	return ListMergeStatusCheckOutput{Items: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

type SetStatusInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"mr_iid"                    jsonschema:"Merge request internal ID,required"`
	SHA                   string               `json:"sha"                       jsonschema:"Head SHA of the merge request source branch,required"`
	ExternalStatusCheckID int64                `json:"external_status_check_id"  jsonschema:"External status check ID to update,required"`
	Status                string               `json:"status"                    jsonschema:"Status value (e.g. passed, failed),required"`
}

// SetExternalStatusCheckStatus sets the status of an external status check (deprecated).
func SetExternalStatusCheckStatus(ctx context.Context, client *gitlabclient.Client, input SetStatusInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("setExternalStatusCheckStatus", "mr_iid")
	}
	if input.SHA == "" {
		return toolutil.ErrFieldRequired("sha")
	}
	if input.ExternalStatusCheckID <= 0 {
		return toolutil.ErrRequiredInt64("setExternalStatusCheckStatus", "external_status_check_id")
	}
	if input.Status == "" {
		return toolutil.ErrFieldRequired("status")
	}
	opts := &gl.SetExternalStatusCheckStatusOptions{
		SHA:                   new(input.SHA),
		ExternalStatusCheckID: new(input.ExternalStatusCheckID),
		Status:                new(input.Status),
	}
	_, err := client.GL().ExternalStatusChecks.SetExternalStatusCheckStatus(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("setExternalStatusCheckStatus", err)
	}
	return nil
}

type ListProjectStatusChecksInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ListProjectStatusChecks lists project-level status checks (deprecated interface, not function).
func ListProjectStatusChecks(ctx context.Context, client *gitlabclient.Client, input ListProjectStatusChecksInput) (ListProjectStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListProjectStatusCheckOutput{}, err
	}
	if input.ProjectID == "" {
		return ListProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	checks, resp, err := client.GL().ExternalStatusChecks.ListProjectStatusChecks(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectStatusCheckOutput{}, toolutil.WrapErrWithMessage("listProjectStatusChecks", err)
	}
	items := make([]ProjectStatusCheckOutput, len(checks))
	for i, c := range checks {
		items[i] = toProjectStatusCheckOutput(c)
	}
	return ListProjectStatusCheckOutput{Items: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

type CreateLegacyInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	Name               string               `json:"name"                  jsonschema:"Name of the external status check,required"`
	ExternalURL        string               `json:"external_url"          jsonschema:"External URL for the status check,required"`
	ProtectedBranchIDs []int64              `json:"protected_branch_ids,omitempty" jsonschema:"IDs of protected branches to scope the check to"`
}

// CreateExternalStatusCheck creates a project external status check (deprecated).
func CreateExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, input CreateLegacyInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.Name == "" {
		return toolutil.ErrFieldRequired("name")
	}
	if input.ExternalURL == "" {
		return toolutil.ErrFieldRequired("external_url")
	}
	opts := &gl.CreateExternalStatusCheckOptions{
		Name:        new(input.Name),
		ExternalURL: new(input.ExternalURL),
	}
	if len(input.ProtectedBranchIDs) > 0 {
		opts.ProtectedBranchIDs = &input.ProtectedBranchIDs
	}
	_, err := client.GL().ExternalStatusChecks.CreateExternalStatusCheck(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("createExternalStatusCheck", err)
	}
	return nil
}

type DeleteLegacyInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CheckID   int64                `json:"check_id"   jsonschema:"External status check ID to delete,required"`
}

// DeleteExternalStatusCheck deletes a project external status check (deprecated).
func DeleteExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, input DeleteLegacyInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.CheckID <= 0 {
		return toolutil.ErrRequiredInt64("deleteExternalStatusCheck", "check_id")
	}
	_, err := client.GL().ExternalStatusChecks.DeleteExternalStatusCheck(string(input.ProjectID), input.CheckID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("deleteExternalStatusCheck", err)
	}
	return nil
}

type UpdateLegacyInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	CheckID            int64                `json:"check_id"              jsonschema:"External status check ID to update,required"`
	Name               string               `json:"name,omitempty"        jsonschema:"Updated name"`
	ExternalURL        string               `json:"external_url,omitempty" jsonschema:"Updated external URL"`
	ProtectedBranchIDs []int64              `json:"protected_branch_ids,omitempty" jsonschema:"Updated protected branch IDs"`
}

// UpdateExternalStatusCheck updates a project external status check (deprecated).
func UpdateExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, input UpdateLegacyInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.CheckID <= 0 {
		return toolutil.ErrRequiredInt64("updateExternalStatusCheck", "check_id")
	}
	opts := &gl.UpdateExternalStatusCheckOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.ExternalURL != "" {
		opts.ExternalURL = new(input.ExternalURL)
	}
	if len(input.ProtectedBranchIDs) > 0 {
		opts.ProtectedBranchIDs = &input.ProtectedBranchIDs
	}
	_, err := client.GL().ExternalStatusChecks.UpdateExternalStatusCheck(string(input.ProjectID), input.CheckID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("updateExternalStatusCheck", err)
	}
	return nil
}

type RetryLegacyInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	CheckID   int64                `json:"check_id"   jsonschema:"External status check ID to retry,required"`
}

// RetryFailedStatusCheckForMR retries a failed external status check for a merge request (deprecated).
func RetryFailedStatusCheckForMR(ctx context.Context, client *gitlabclient.Client, input RetryLegacyInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("retryFailedStatusCheckForMR", "mr_iid")
	}
	if input.CheckID <= 0 {
		return toolutil.ErrRequiredInt64("retryFailedStatusCheckForMR", "check_id")
	}
	_, err := client.GL().ExternalStatusChecks.RetryFailedStatusCheckForAMergeRequest(string(input.ProjectID), input.MRIID, input.CheckID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("retryFailedStatusCheckForMR", err)
	}
	return nil
}

type ListProjectMRInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	toolutil.PaginationInput
}

// ListProjectMRExternalStatusChecks lists external status checks for a project merge request.
func ListProjectMRExternalStatusChecks(ctx context.Context, client *gitlabclient.Client, input ListProjectMRInput) (ListMergeStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListMergeStatusCheckOutput{}, err
	}
	if input.ProjectID == "" {
		return ListMergeStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return ListMergeStatusCheckOutput{}, toolutil.ErrRequiredInt64("listProjectMRExternalStatusChecks", "mr_iid")
	}
	opts := &gl.ListProjectMergeRequestExternalStatusChecksOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	checks, resp, err := client.GL().ExternalStatusChecks.ListProjectMergeRequestExternalStatusChecks(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListMergeStatusCheckOutput{}, toolutil.WrapErrWithMessage("listProjectMRExternalStatusChecks", err)
	}
	items := make([]MergeStatusCheckOutput, len(checks))
	for i, c := range checks {
		items[i] = toMergeStatusCheckOutput(c)
	}
	return ListMergeStatusCheckOutput{Items: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ListProjectExternalStatusChecks lists external status checks for a project.
func ListProjectExternalStatusChecks(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListProjectStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListProjectStatusCheckOutput{}, err
	}
	if input.ProjectID == "" {
		return ListProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListProjectExternalStatusChecksOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	checks, resp, err := client.GL().ExternalStatusChecks.ListProjectExternalStatusChecks(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectStatusCheckOutput{}, toolutil.WrapErrWithMessage("listProjectExternalStatusChecks", err)
	}
	items := make([]ProjectStatusCheckOutput, len(checks))
	for i, c := range checks {
		items[i] = toProjectStatusCheckOutput(c)
	}
	return ListProjectStatusCheckOutput{Items: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

type CreateProjectInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	Name               string               `json:"name"                  jsonschema:"Name of the external status check,required"`
	ExternalURL        string               `json:"external_url"          jsonschema:"External URL for the status check,required"`
	SharedSecret       string               `json:"shared_secret,omitempty" jsonschema:"Shared secret for HMAC verification"`
	ProtectedBranchIDs []int64              `json:"protected_branch_ids,omitempty" jsonschema:"IDs of protected branches to scope the check to"`
}

// CreateProjectExternalStatusCheck creates an external status check for a project.
func CreateProjectExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, input CreateProjectInput) (ProjectStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProjectStatusCheckOutput{}, err
	}
	if input.ProjectID == "" {
		return ProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Name == "" {
		return ProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("name")
	}
	if input.ExternalURL == "" {
		return ProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("external_url")
	}
	opts := &gl.CreateProjectExternalStatusCheckOptions{
		Name:        new(input.Name),
		ExternalURL: new(input.ExternalURL),
	}
	if input.SharedSecret != "" {
		opts.SharedSecret = new(input.SharedSecret)
	}
	if len(input.ProtectedBranchIDs) > 0 {
		opts.ProtectedBranchIDs = &input.ProtectedBranchIDs
	}
	check, _, err := client.GL().ExternalStatusChecks.CreateProjectExternalStatusCheck(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ProjectStatusCheckOutput{}, toolutil.WrapErrWithMessage("createProjectExternalStatusCheck", err)
	}
	return toProjectStatusCheckOutput(check), nil
}

type DeleteProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	CheckID   int64                `json:"check_id"   jsonschema:"External status check ID to delete,required"`
}

// DeleteProjectExternalStatusCheck deletes an external status check from a project.
func DeleteProjectExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, input DeleteProjectInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.CheckID <= 0 {
		return toolutil.ErrRequiredInt64("deleteProjectExternalStatusCheck", "check_id")
	}
	_, err := client.GL().ExternalStatusChecks.DeleteProjectExternalStatusCheck(string(input.ProjectID), input.CheckID, &gl.DeleteProjectExternalStatusCheckOptions{}, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("deleteProjectExternalStatusCheck", err)
	}
	return nil
}

type UpdateProjectInput struct {
	ProjectID          toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	CheckID            int64                `json:"check_id"              jsonschema:"External status check ID to update,required"`
	Name               string               `json:"name,omitempty"        jsonschema:"Updated name"`
	ExternalURL        string               `json:"external_url,omitempty" jsonschema:"Updated external URL"`
	SharedSecret       string               `json:"shared_secret,omitempty" jsonschema:"Updated shared secret for HMAC verification"`
	ProtectedBranchIDs []int64              `json:"protected_branch_ids,omitempty" jsonschema:"Updated protected branch IDs"`
}

// UpdateProjectExternalStatusCheck updates an external status check for a project.
func UpdateProjectExternalStatusCheck(ctx context.Context, client *gitlabclient.Client, input UpdateProjectInput) (ProjectStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProjectStatusCheckOutput{}, err
	}
	if input.ProjectID == "" {
		return ProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.CheckID <= 0 {
		return ProjectStatusCheckOutput{}, toolutil.ErrRequiredInt64("updateProjectExternalStatusCheck", "check_id")
	}
	opts := &gl.UpdateProjectExternalStatusCheckOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.ExternalURL != "" {
		opts.ExternalURL = new(input.ExternalURL)
	}
	if input.SharedSecret != "" {
		opts.SharedSecret = new(input.SharedSecret)
	}
	if len(input.ProtectedBranchIDs) > 0 {
		opts.ProtectedBranchIDs = &input.ProtectedBranchIDs
	}
	check, _, err := client.GL().ExternalStatusChecks.UpdateProjectExternalStatusCheck(string(input.ProjectID), input.CheckID, opts, gl.WithContext(ctx))
	if err != nil {
		return ProjectStatusCheckOutput{}, toolutil.WrapErrWithMessage("updateProjectExternalStatusCheck", err)
	}
	return toProjectStatusCheckOutput(check), nil
}

type RetryProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"mr_iid"     jsonschema:"Merge request internal ID,required"`
	CheckID   int64                `json:"check_id"   jsonschema:"External status check ID to retry,required"`
}

// RetryFailedExternalStatusCheckForProjectMR retries a failed external status check for a project merge request.
func RetryFailedExternalStatusCheckForProjectMR(ctx context.Context, client *gitlabclient.Client, input RetryProjectInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("retryFailedExternalStatusCheckForProjectMR", "mr_iid")
	}
	if input.CheckID <= 0 {
		return toolutil.ErrRequiredInt64("retryFailedExternalStatusCheckForProjectMR", "check_id")
	}
	_, err := client.GL().ExternalStatusChecks.RetryFailedExternalStatusCheckForProjectMergeRequest(string(input.ProjectID), input.MRIID, input.CheckID, &gl.RetryFailedExternalStatusCheckForProjectMergeRequestOptions{}, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("retryFailedExternalStatusCheckForProjectMR", err)
	}
	return nil
}

type SetProjectStatusInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"mr_iid"                    jsonschema:"Merge request internal ID,required"`
	SHA                   string               `json:"sha"                       jsonschema:"Head SHA of the merge request source branch,required"`
	ExternalStatusCheckID int64                `json:"external_status_check_id"  jsonschema:"External status check ID to update,required"`
	Status                string               `json:"status"                    jsonschema:"Status value (e.g. passed, failed),required"`
}

// SetProjectMRExternalStatusCheckStatus sets the status of an external status check for a project merge request.
func SetProjectMRExternalStatusCheckStatus(ctx context.Context, client *gitlabclient.Client, input SetProjectStatusInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.MRIID <= 0 {
		return toolutil.ErrRequiredInt64("setProjectMRExternalStatusCheckStatus", "mr_iid")
	}
	if input.SHA == "" {
		return toolutil.ErrFieldRequired("sha")
	}
	if input.ExternalStatusCheckID <= 0 {
		return toolutil.ErrRequiredInt64("setProjectMRExternalStatusCheckStatus", "external_status_check_id")
	}
	if input.Status == "" {
		return toolutil.ErrFieldRequired("status")
	}
	opts := &gl.SetProjectMergeRequestExternalStatusCheckStatusOptions{
		SHA:                   new(input.SHA),
		ExternalStatusCheckID: new(input.ExternalStatusCheckID),
		Status:                new(input.Status),
	}
	_, err := client.GL().ExternalStatusChecks.SetProjectMergeRequestExternalStatusCheckStatus(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("setProjectMRExternalStatusCheckStatus", err)
	}
	return nil
}
