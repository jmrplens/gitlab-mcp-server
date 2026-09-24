package externalstatuschecks

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// MergeStatusCheckOutput represents a single external status check attached to a merge request.
type MergeStatusCheckOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ExternalURL string `json:"external_url"`
	Status      string `json:"status"`
}

// ProjectStatusCheckOutput represents a project-level external status check including its HMAC and protected branch scope.
type ProjectStatusCheckOutput struct {
	toolutil.HintableOutput
	ID                int64                   `json:"id"`
	Name              string                  `json:"name"`
	ProjectID         int64                   `json:"project_id"`
	ExternalURL       string                  `json:"external_url"`
	HMAC              bool                    `json:"hmac"`
	ProtectedBranches []ProtectedBranchOutput `json:"protected_branches,omitempty"`
}

// ProtectedBranchOutput represents a protected branch entry associated with a
// project external status check. It mirrors every field of the client-go
// gl.StatusCheckProtectedBranch type (C-IMPORTS, 1:1 audit).
type ProtectedBranchOutput struct {
	ID                        int64      `json:"id"`
	ProjectID                 int64      `json:"project_id"`
	Name                      string     `json:"name"`
	CreatedAt                 *time.Time `json:"created_at,omitempty"`
	UpdatedAt                 *time.Time `json:"updated_at,omitempty"`
	CodeOwnerApprovalRequired bool       `json:"code_owner_approval_required"`
}

// ListMergeStatusCheckOutput is the paginated result of listing merge request external status checks.
type ListMergeStatusCheckOutput struct {
	toolutil.HintableOutput
	Items      []MergeStatusCheckOutput  `json:"items"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectStatusCheckOutput is the paginated result of listing project external status checks.
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
			CreatedAt:                 pb.CreatedAt,
			UpdatedAt:                 pb.UpdatedAt,
			CodeOwnerApprovalRequired: pb.CodeOwnerApprovalRequired,
		})
	}
	return out
}

// ListProjectStatusChecksInput defines parameters for the ListProjectStatusChecks action.
type ListProjectStatusChecksInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order by for keyset pagination (e.g. id)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// The hints a refused caller is given. Every status check route answers a
// project whose namespace lacks the Ultimate feature with 401 from one
// before-block (ee/lib/api/status_checks.rb:16), and the project routes answer
// a missing role with 401 too (:67, and
// ee/app/services/external_status_checks/update_service.rb:40), so these are
// keyed on toolutil.IsPermissionRefusal rather than on a status.
const (
	// hintStatusCheckLicense is the license every route checks first.
	hintStatusCheckLicense = "external status checks need an Ultimate license on the project's namespace (on GitLab.com, the plan of its top-level group), and GitLab answers a project without it with 401 on every status check route"
	// hintStatusCheckMaintainer is the license together with the role the
	// project list and the update check next.
	hintStatusCheckMaintainer = "reading or changing a project's external status checks needs the Maintainer role, and an Ultimate license on the project's namespace; GitLab answers the lack of either with 401. Verify project_id with project.get"
)

// refusedForLicense reports whether err is the 401 a merge request's status
// check route answers when the project's namespace lacks Ultimate. On those
// routes a missing role is a 403 of its own (the route's authorize! call), so
// the two are told apart by status, not read as one refusal.
func refusedForLicense(err error) bool {
	return toolutil.IsHTTPStatus(err, http.StatusUnauthorized) && toolutil.IsPermissionRefusal(err)
}

// refusedForRole reports whether err is the 403 a merge request's status
// check route answers a caller whose role on the merge request falls short.
func refusedForRole(err error) bool {
	return toolutil.IsHTTPStatus(err, http.StatusForbidden) && toolutil.IsPermissionRefusal(err)
}

// ListProjectStatusChecks lists project-level external status checks.
func ListProjectStatusChecks(ctx context.Context, client *gitlabclient.Client, input ListProjectStatusChecksInput) (ListProjectStatusCheckOutput, error) {
	return listProjectStatusChecks(ctx, input.ProjectID, "listProjectStatusChecks",
		"deprecated endpoint - prefer external_status_check.list_project; "+hintStatusCheckMaintainer,
		func(projectID string, opts ...gl.RequestOptionFunc) ([]*gl.ProjectStatusCheck, *gl.Response, error) {
			listOptions := &gl.ListOptions{}
			toolutil.ApplyListOptions(listOptions, input.PaginationInput, input.KeysetPaginationInput)
			listOptions.OrderBy = input.OrderBy
			listOptions.Sort = input.Sort
			return client.GL().ExternalStatusChecks.ListProjectStatusChecks(projectID, listOptions, opts...)
		})
}

func listProjectStatusChecks(ctx context.Context, projectID toolutil.StringOrInt, operation, permissionHint string, list func(string, ...gl.RequestOptionFunc) ([]*gl.ProjectStatusCheck, *gl.Response, error)) (ListProjectStatusCheckOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListProjectStatusCheckOutput{}, err
	}
	if projectID == "" {
		return ListProjectStatusCheckOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	checks, resp, err := list(string(projectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsPermissionRefusal(err) {
			return ListProjectStatusCheckOutput{}, toolutil.WrapErrWithHint(operation, err, permissionHint)
		}
		return ListProjectStatusCheckOutput{}, toolutil.WrapErrWithMessage(operation, err)
	}
	items := make([]ProjectStatusCheckOutput, len(checks))
	for i, c := range checks {
		items[i] = toProjectStatusCheckOutput(c)
	}
	return ListProjectStatusCheckOutput{Items: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ListProjectMRInput defines parameters for the ListProjectMRExternalStatusChecks action.
type ListProjectMRInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order by for keyset pagination (e.g. id)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
		return ListMergeStatusCheckOutput{}, toolutil.ErrRequiredInt64("listProjectMRExternalStatusChecks", "merge_request_iid")
	}
	opts := &gl.ListProjectMergeRequestExternalStatusChecksOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	opts.OrderBy = input.OrderBy
	opts.Sort = input.Sort
	checks, resp, err := client.GL().ExternalStatusChecks.ListProjectMergeRequestExternalStatusChecks(string(input.ProjectID), input.MRIID, opts, gl.WithContext(ctx))
	if err != nil {
		if refusedForLicense(err) {
			return ListMergeStatusCheckOutput{}, toolutil.WrapErrWithHint("listProjectMRExternalStatusChecks", err, hintStatusCheckLicense)
		}
		if refusedForRole(err) {
			return ListMergeStatusCheckOutput{}, toolutil.WrapErrWithHint("listProjectMRExternalStatusChecks", err,
				"reading a merge request's status checks needs at least the Reporter role on the project")
		}
		return ListMergeStatusCheckOutput{}, toolutil.WrapErrWithStatusHint("listProjectMRExternalStatusChecks", err, http.StatusNotFound,
			"verify merge_request_iid (project-scoped, not the global ID) with merge_request.list")
	}
	items := make([]MergeStatusCheckOutput, len(checks))
	for i, c := range checks {
		items[i] = toMergeStatusCheckOutput(c)
	}
	return ListMergeStatusCheckOutput{Items: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ListProjectInput defines parameters for the ListProjectExternalStatusChecks action.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order by for keyset pagination (e.g. id)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListProjectExternalStatusChecks lists external status checks for a project.
func ListProjectExternalStatusChecks(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListProjectStatusCheckOutput, error) {
	return listProjectStatusChecks(ctx, input.ProjectID, "listProjectExternalStatusChecks", hintStatusCheckMaintainer,
		func(projectID string, opts ...gl.RequestOptionFunc) ([]*gl.ProjectStatusCheck, *gl.Response, error) {
			listOptions := &gl.ListProjectExternalStatusChecksOptions{}
			toolutil.ApplyListOptions(&listOptions.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
			listOptions.OrderBy = input.OrderBy
			listOptions.Sort = input.Sort
			return client.GL().ExternalStatusChecks.ListProjectExternalStatusChecks(projectID, listOptions, opts...)
		})
}

// CreateProjectInput defines parameters for the CreateProjectExternalStatusCheck action.
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
		if toolutil.IsPermissionRefusal(err) {
			return ProjectStatusCheckOutput{}, toolutil.WrapErrWithHint("createProjectExternalStatusCheck", err, hintStatusCheckLicense)
		}
		return ProjectStatusCheckOutput{}, toolutil.WrapErrWithStatusHint("createProjectExternalStatusCheck", err, http.StatusBadRequest,
			"name must be unique within the project; external_url must be a valid HTTPS URL reachable from GitLab; protected_branch_ids must be IDs (not names) from branch.list_protected")
	}
	return toProjectStatusCheckOutput(check), nil
}

// DeleteProjectInput defines parameters for the DeleteProjectExternalStatusCheck action.
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
		// The license is the one refusal this route answers. A caller without
		// the Maintainer role is answered 204 and nothing is deleted, because
		// the route discards the refusal its service returns
		// (docs/development/upstream-bugs.md), so no hint here names the role.
		if toolutil.IsPermissionRefusal(err) {
			return toolutil.WrapErrWithHint("deleteProjectExternalStatusCheck", err, hintStatusCheckLicense)
		}
		return toolutil.WrapErrWithStatusHint("deleteProjectExternalStatusCheck", err, http.StatusNotFound,
			"verify check_id with external_status_check.list_project")
	}
	return nil
}

// UpdateProjectInput defines parameters for the UpdateProjectExternalStatusCheck action.
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
		if toolutil.IsPermissionRefusal(err) {
			return ProjectStatusCheckOutput{}, toolutil.WrapErrWithHint("updateProjectExternalStatusCheck", err, hintStatusCheckMaintainer)
		}
		return ProjectStatusCheckOutput{}, toolutil.WrapErrWithStatusHint("updateProjectExternalStatusCheck", err, http.StatusNotFound,
			"verify check_id with external_status_check.list_project; name must remain unique; external_url must be valid HTTPS")
	}
	return toProjectStatusCheckOutput(check), nil
}

// RetryProjectInput defines parameters for the RetryFailedExternalStatusCheckForProjectMR action.
type RetryProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MRIID     int64                `json:"merge_request_iid"     jsonschema:"Merge request internal ID,required"`
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
		return toolutil.ErrRequiredInt64("retryFailedExternalStatusCheckForProjectMR", "merge_request_iid")
	}
	if input.CheckID <= 0 {
		return toolutil.ErrRequiredInt64("retryFailedExternalStatusCheckForProjectMR", "check_id")
	}
	_, err := client.GL().ExternalStatusChecks.RetryFailedExternalStatusCheckForProjectMergeRequest(string(input.ProjectID), input.MRIID, input.CheckID, &gl.RetryFailedExternalStatusCheckForProjectMergeRequestOptions{}, gl.WithContext(ctx))
	if err != nil {
		if refusedForLicense(err) {
			return toolutil.WrapErrWithHint("retryFailedExternalStatusCheckForProjectMR", err, hintStatusCheckLicense)
		}
		if refusedForRole(err) {
			return toolutil.WrapErrWithHint("retryFailedExternalStatusCheckForProjectMR", err,
				"retrying a status check needs at least the Developer role on the project")
		}
		return toolutil.WrapErrWithStatusHint("retryFailedExternalStatusCheckForProjectMR", err, http.StatusUnprocessableEntity,
			"check must currently be in 'failed' state to retry; verify status with external_status_check.list_project_mr_checks; rate-limited per project")
	}
	return nil
}

// SetProjectStatusInput defines parameters for the SetProjectMRExternalStatusCheckStatus action.
type SetProjectStatusInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	MRIID                 int64                `json:"merge_request_iid"                    jsonschema:"Merge request internal ID,required"`
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
		return toolutil.ErrRequiredInt64("setProjectMRExternalStatusCheckStatus", "merge_request_iid")
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
		if refusedForLicense(err) {
			return toolutil.WrapErrWithHint("setProjectMRExternalStatusCheckStatus", err, hintStatusCheckLicense)
		}
		if refusedForRole(err) {
			return toolutil.WrapErrWithHint("setProjectMRExternalStatusCheckStatus", err,
				"setting a status check's status needs permission to approve the merge request")
		}
		return toolutil.WrapErrWithStatusHint("setProjectMRExternalStatusCheckStatus", err, http.StatusBadRequest,
			"sha must match the current MR head (use merge_request.get to confirm); status must be 'passed' or 'failed'; only the external service that created the check (HMAC-authenticated) can set its status")
	}
	return nil
}
