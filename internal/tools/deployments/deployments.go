// Package deployments implements MCP tool handlers for GitLab deployment
// operations including list, get, create, update, and delete.
// It wraps the Deployments service from client-go v2.
package deployments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const errDeploymentIDRequired = "deployment_id is required and must be > 0"

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListInput contains parameters for listing project deployments.
type ListInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy     string               `json:"order_by,omitempty"     jsonschema:"Order by id or iid or created_at or updated_at or finished_at or ref (default: id)"`
	Sort        string               `json:"sort,omitempty"         jsonschema:"Sort order: asc or desc (default: asc)"`
	Environment string               `json:"environment,omitempty"  jsonschema:"Filter by environment name"`
	Status      string               `json:"status,omitempty"       jsonschema:"Filter by status: created or running or success or failed or canceled"`
	toolutil.PaginationInput
}

// GetInput contains parameters for retrieving a single deployment.
type GetInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID int                  `json:"deployment_id"  jsonschema:"Deployment ID,required"`
}

// CreateInput contains parameters for creating a deployment.
type CreateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"        jsonschema:"Project ID or URL-encoded path,required"`
	Environment string               `json:"environment"       jsonschema:"Name of the environment to deploy to,required"`
	Ref         string               `json:"ref"               jsonschema:"Git branch or tag to deploy,required"`
	SHA         string               `json:"sha"               jsonschema:"Git SHA to deploy,required"`
	Tag         *bool                `json:"tag,omitempty"     jsonschema:"Whether the ref is a tag (default: false)"`
	Status      string               `json:"status,omitempty"  jsonschema:"Deployment status: created or running or success or failed or canceled"`
}

// UpdateInput contains parameters for updating a deployment status.
type UpdateInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID int                  `json:"deployment_id"  jsonschema:"Deployment ID,required"`
	Status       string               `json:"status"         jsonschema:"New deployment status: created or running or success or failed or canceled,required"`
}

// DeleteInput contains parameters for deleting a deployment.
type DeleteInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID int                  `json:"deployment_id"  jsonschema:"Deployment ID,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a single deployment in MCP responses.
type Output struct {
	toolutil.HintableOutput
	ID              int    `json:"id"`
	IID             int    `json:"iid"`
	Ref             string `json:"ref"`
	SHA             string `json:"sha"`
	Status          string `json:"status"`
	UserName        string `json:"user_name,omitempty"`
	EnvironmentName string `json:"environment_name,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
}

// ListOutput represents a paginated list of deployments.
type ListOutput struct {
	toolutil.HintableOutput
	Deployments []Output                  `json:"deployments"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(d *gitlab.Deployment) Output {
	out := Output{
		ID:     int(d.ID),
		IID:    int(d.IID),
		Ref:    d.Ref,
		SHA:    d.SHA,
		Status: d.Status,
	}
	if d.User != nil {
		out.UserName = d.User.Username
	}
	if d.Environment != nil {
		out.EnvironmentName = d.Environment.Name
	}
	if d.CreatedAt != nil {
		out.CreatedAt = d.CreatedAt.Format(time.RFC3339)
	}
	if d.UpdatedAt != nil {
		out.UpdatedAt = d.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// List lists resources for the deployments package.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gitlab.ListProjectDeploymentsOptions{
		ListOptions: gitlab.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.OrderBy != "" {
		opts.OrderBy = &input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = &input.Sort
	}
	if input.Environment != "" {
		opts.Environment = &input.Environment
	}
	if input.Status != "" {
		opts.Status = &input.Status
	}

	deployments, resp, err := client.GL().Deployments.ListProjectDeployments(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list deployments", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; deployments are populated by CI/CD jobs that run in environments")
	}

	items := make([]Output, 0, len(deployments))
	for _, d := range deployments {
		items = append(items, toOutput(d))
	}

	return ListOutput{
		Deployments: items,
		Pagination:  toolutil.PaginationFromResponse(resp),
	}, nil
}

// Get retrieves resources for the deployments package.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.DeploymentID == 0 {
		return Output{}, errors.New(errDeploymentIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	d, _, err := client.GL().Deployments.GetProjectDeployment(string(input.ProjectID), int64(input.DeploymentID), gitlab.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get deployment", err, http.StatusNotFound,
			"verify deployment_id with gitlab_list_deployments \u2014 deployment IDs are project-scoped")
	}

	return toOutput(d), nil
}

// Create creates resources for the deployments package.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Environment == "" {
		return Output{}, toolutil.ErrFieldRequired("environment")
	}
	if input.Ref == "" {
		return Output{}, toolutil.ErrFieldRequired("ref")
	}
	if input.SHA == "" {
		return Output{}, toolutil.ErrFieldRequired("sha")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gitlab.CreateProjectDeploymentOptions{
		Environment: &input.Environment,
		Ref:         &input.Ref,
		SHA:         &input.SHA,
	}
	if input.Tag != nil {
		opts.Tag = input.Tag
	}
	if input.Status != "" {
		status := gitlab.DeploymentStatusValue(input.Status)
		opts.Status = &status
	}

	d, _, err := client.GL().Deployments.CreateProjectDeployment(string(input.ProjectID), opts, gitlab.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("create deployment", err,
				"creating deployments requires Developer+ role; protected environments may require additional approver permissions")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("create deployment", err,
				"verify environment exists with gitlab_environment_list, sha is a valid commit, and ref is an existing branch/tag")
		}
		return Output{}, toolutil.WrapErrWithMessage("create deployment", err)
	}

	return toOutput(d), nil
}

// Update updates resources for the deployments package.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.DeploymentID == 0 {
		return Output{}, errors.New(errDeploymentIDRequired)
	}
	if input.Status == "" {
		return Output{}, toolutil.ErrFieldRequired("status")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	status := gitlab.DeploymentStatusValue(input.Status)
	opts := &gitlab.UpdateProjectDeploymentOptions{
		Status: &status,
	}

	d, _, err := client.GL().Deployments.UpdateProjectDeployment(string(input.ProjectID), int64(input.DeploymentID), opts, gitlab.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("update deployment", err,
				"status must be one of: created, running, success, failed, canceled, blocked \u2014 transitions out of terminal states are not allowed")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("update deployment", err, http.StatusNotFound,
			"verify deployment_id with gitlab_list_deployments")
	}

	return toOutput(d), nil
}

// Delete deletes resources for the deployments package.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.DeploymentID == 0 {
		return errors.New(errDeploymentIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().Deployments.DeleteProjectDeployment(string(input.ProjectID), int64(input.DeploymentID), gitlab.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("delete deployment", err,
				"deleting deployments requires Maintainer+ role and the deployment must be in a final state (success, failed, canceled)")
		}
		return toolutil.WrapErrWithStatusHint("delete deployment", err, http.StatusNotFound,
			"verify deployment_id with gitlab_list_deployments")
	}
	return nil
}

// Approve or Reject Deployment.

// ApproveOrRejectInput defines parameters for approving or rejecting a blocked deployment.
type ApproveOrRejectInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID int                  `json:"deployment_id" jsonschema:"Deployment ID,required"`
	Status       string               `json:"status" jsonschema:"Approval status: approved or rejected,required"`
	Comment      string               `json:"comment,omitempty" jsonschema:"Optional comment for the approval or rejection"`
}

// ApproveOrRejectOutput represents the result of approving or rejecting a deployment.
type ApproveOrRejectOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// ApproveOrReject approves or rejects a blocked deployment.
func ApproveOrReject(ctx context.Context, client *gitlabclient.Client, input ApproveOrRejectInput) (ApproveOrRejectOutput, error) {
	if input.ProjectID == "" {
		return ApproveOrRejectOutput{}, errors.New("approve_or_reject_deployment: project_id is required")
	}
	if input.DeploymentID == 0 {
		return ApproveOrRejectOutput{}, errors.New("approve_or_reject_deployment: deployment_id is required")
	}
	if input.Status != "approved" && input.Status != "rejected" {
		return ApproveOrRejectOutput{}, toolutil.ErrInvalidEnum("status", input.Status, []string{"approved", "rejected"})
	}

	opts := &gitlab.ApproveOrRejectProjectDeploymentOptions{
		Status: new(gitlab.DeploymentApprovalStatus(input.Status)),
	}
	if input.Comment != "" {
		opts.Comment = new(input.Comment)
	}

	_, err := client.GL().Deployments.ApproveOrRejectProjectDeployment(
		string(input.ProjectID), int64(input.DeploymentID), opts, gitlab.WithContext(ctx),
	)
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ApproveOrRejectOutput{}, toolutil.WrapErrWithHint("approve_or_reject_deployment", err,
				"approving/rejecting deployments requires being a designated approver on the protected environment; status must be 'approved' or 'rejected'")
		}
		return ApproveOrRejectOutput{}, toolutil.WrapErrWithStatusHint("approve_or_reject_deployment", err, http.StatusNotFound,
			"verify deployment_id with gitlab_list_deployments \u2014 only deployments awaiting approval can be acted on")
	}

	return ApproveOrRejectOutput{
		Message: fmt.Sprintf("Deployment #%d %s successfully", input.DeploymentID, input.Status),
	}, nil
}
