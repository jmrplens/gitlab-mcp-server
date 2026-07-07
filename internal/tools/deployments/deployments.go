package deployments

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	errDeploymentIDRequired = "deployment_id is required and must be > 0"
	opCreateDeployment      = "create deployment"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListInput contains parameters for listing project deployments.
type ListInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy        string               `json:"order_by,omitempty"        jsonschema:"Order by id or iid or created_at or updated_at or finished_at or ref (default: id)"`
	Sort           string               `json:"sort,omitempty"            jsonschema:"Sort order: asc or desc (default: asc)"`
	Environment    string               `json:"environment,omitempty"     jsonschema:"Filter by environment name"`
	Status         string               `json:"status,omitempty"          jsonschema:"Filter by status: created or running or success or failed or canceled"`
	UpdatedAfter   string               `json:"updated_after,omitempty"   jsonschema:"Return deployments updated after this RFC3339 timestamp (GitLab < 14 only)"`
	UpdatedBefore  string               `json:"updated_before,omitempty"  jsonschema:"Return deployments updated before this RFC3339 timestamp (GitLab < 14 only)"`
	FinishedAfter  string               `json:"finished_after,omitempty"  jsonschema:"Return deployments finished after this RFC3339 timestamp (GitLab 14+ only)"`
	FinishedBefore string               `json:"finished_before,omitempty" jsonschema:"Return deployments finished before this RFC3339 timestamp (GitLab 14+ only)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	Tag         *bool                `json:"tag,omitempty"     jsonschema:"Whether the ref is a tag. GitLab 19 requires this explicitly: pass false for branch refs and true for tag refs (default: false)"`
	Status      string               `json:"status,omitempty"  jsonschema:"Initial deployment status: running or success or failed or canceled. GitLab 19 rejects created when creating a deployment"`
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

// Output represents a single deployment in MCP responses, reconciled 1:1 with
// the documented Deployments API response (doc/api/deployments.md). The nested
// user, environment, and deployable objects are documented reference subsets of
// the corresponding gl.Deployment sub-objects.
type Output struct {
	toolutil.HintableOutput
	ID          int                `json:"id"`
	IID         int                `json:"iid"`
	Ref         string             `json:"ref"`
	SHA         string             `json:"sha"`
	Status      string             `json:"status"`
	CreatedAt   string             `json:"created_at,omitempty"`
	UpdatedAt   string             `json:"updated_at,omitempty"`
	User        *UserOutput        `json:"user,omitempty"`
	Environment *EnvironmentOutput `json:"environment,omitempty"`
	Deployable  *DeployableOutput  `json:"deployable,omitempty"`
}

// ListOutput represents a paginated list of deployments.
type ListOutput struct {
	toolutil.HintableOutput
	Deployments []Output                  `json:"deployments"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Raw REST superset fetch
// ---------------------------------------------------------------------------.

// deploymentAPI is the raw-fetch superset of a deployment. It embeds the SDK
// gitlab.Deployment so every documented scalar and sub-object decodes through the
// SDK's own unmarshalling, and overrides the deployable key with deployableAPI to
// additionally capture the documented deployable.project object that the SDK omits.
//
// The named Deployable field shadows the embedded gitlab.Deployment.Deployable
// for the "deployable" JSON key (encoding/json prefers the shallower field), so a
// single Do(&superset) unmarshal yields both the full SDK deployment shape and the
// raw project sub-object. The embedded deployment's own Deployable stays zero and
// is never read.
type deploymentAPI struct {
	gitlab.Deployment
	Deployable deployableAPI `json:"deployable"`
}

// deployableAPI is the raw-fetch superset of gl.DeploymentDeployable. It embeds
// the SDK type for all documented deployable fields and adds the SDK-missing
// project sub-object. A nil Project (older instances omit the key) is naturally
// tolerated and yields an omitted output field.
type deployableAPI struct {
	gitlab.DeploymentDeployable
	Project *DeployableProjectOutput `json:"project,omitempty"`
}

// rawGetDeployment issues a raw REST GET against a single-deployment path,
// decoding the documented superset (including deployable.project) into a
// [deploymentAPI]. A single unmarshal is naturally tolerant of older instances
// that omit deployable.project. The single-resource path carries no pagination,
// so the gl.Response is not returned.
func rawGetDeployment(ctx context.Context, client *gitlabclient.Client, path string) (*deploymentAPI, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var d deploymentAPI
	_, err = client.GL().Do(req, &d)
	return &d, err
}

// rawListDeployments issues a raw REST GET against the deployments list path,
// decoding the documented superset into a slice of [deploymentAPI]. The opts
// encode ordering, filters, and pagination via their url struct tags, and the
// returned gl.Response preserves the pagination headers for
// [toolutil.PaginationFromResponse].
func rawListDeployments(ctx context.Context, client *gitlabclient.Client, path string, opts *gitlab.ListProjectDeploymentsOptions) ([]*deploymentAPI, *gitlab.Response, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, []gitlab.RequestOptionFunc{gitlab.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var deployments []*deploymentAPI
	resp, err := client.GL().Do(req, &deployments)
	return deployments, resp, err
}

// ---------------------------------------------------------------------------
// Converter
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(d *gitlab.Deployment) Output {
	return Output{
		ID:          int(d.ID),
		IID:         int(d.IID),
		Ref:         d.Ref,
		SHA:         d.SHA,
		Status:      d.Status,
		CreatedAt:   toolutil.FormatTimePtr(d.CreatedAt),
		UpdatedAt:   toolutil.FormatTimePtr(d.UpdatedAt),
		User:        projectUserOutput(d.User),
		Environment: environmentOutput(d.Environment),
		Deployable:  deployableOutput(d.Deployable, nil),
	}
}

// toOutputAPI converts the raw-fetch superset to the tool output format,
// surfacing the SDK-missing deployable.project object alongside the standard
// SDK-decoded fields.
func toOutputAPI(d *deploymentAPI) Output {
	return Output{
		ID:          int(d.ID),
		IID:         int(d.IID),
		Ref:         d.Ref,
		SHA:         d.SHA,
		Status:      d.Status,
		CreatedAt:   toolutil.FormatTimePtr(d.CreatedAt),
		UpdatedAt:   toolutil.FormatTimePtr(d.UpdatedAt),
		User:        projectUserOutput(d.User),
		Environment: environmentOutput(d.Environment),
		Deployable:  deployableOutput(d.Deployable.DeploymentDeployable, d.Deployable.Project),
	}
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

	opts := &gitlab.ListProjectDeploymentsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
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
	opts.UpdatedAfter = toolutil.ParseOptionalTime(input.UpdatedAfter)
	opts.UpdatedBefore = toolutil.ParseOptionalTime(input.UpdatedBefore)
	opts.FinishedAfter = toolutil.ParseOptionalTime(input.FinishedAfter)
	opts.FinishedBefore = toolutil.ParseOptionalTime(input.FinishedBefore)

	// Raw REST fetch so the documented deployable.project object
	// ({ci_job_token_scope_enabled}), absent from gl.DeploymentDeployable, is
	// surfaced 1:1 with the Deployments API. A single unmarshal is naturally
	// tolerant of older instances that omit the field.
	path := fmt.Sprintf("projects/%s/deployments", gitlab.PathEscape(string(input.ProjectID)))
	deployments, resp, err := rawListDeployments(ctx, client, path, opts)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list deployments", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; deployments are populated by CI/CD jobs that run in environments")
	}

	items := make([]Output, 0, len(deployments))
	for _, d := range deployments {
		items = append(items, toOutputAPI(d))
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

	// Raw REST fetch so the documented deployable.project object
	// ({ci_job_token_scope_enabled}), absent from gl.DeploymentDeployable, is
	// surfaced 1:1 with the Deployments API. A single unmarshal is naturally
	// tolerant of older instances that omit the field.
	path := fmt.Sprintf("projects/%s/deployments/%d", gitlab.PathEscape(string(input.ProjectID)), input.DeploymentID)
	d, err := rawGetDeployment(ctx, client, path)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get deployment", err, http.StatusNotFound,
			"verify deployment_id with gitlab_deployment_list \u2014 deployment IDs are project-scoped")
	}

	return toOutputAPI(d), nil
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
			return Output{}, toolutil.WrapErrWithHint(opCreateDeployment, err,
				"creating deployments requires Developer+ role; protected environments may require additional approver permissions")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			// GitLab 19 rejects requests that omit tag or that pass status
			// "created"; each drift needs its own corrective field hint.
			switch {
			case toolutil.ContainsAny(err, "tag is missing"):
				return Output{}, toolutil.WrapErrWithHint(opCreateDeployment, err,
					"GitLab 19 requires the tag field explicitly — retry with tag:false for branch refs or tag:true for tag refs")
			case toolutil.ContainsAny(err, "status does not have a valid value"):
				return Output{}, toolutil.WrapErrWithHint(opCreateDeployment, err,
					"the API accepts status running, success, failed, or canceled when creating a deployment — GitLab 19 rejects 'created'; omit status or use an accepted value")
			}
			return Output{}, toolutil.WrapErrWithHint(opCreateDeployment, err,
				"verify environment exists with gitlab_environment_list, sha is a valid commit, and ref is an existing branch/tag")
		}
		return Output{}, toolutil.WrapErrWithMessage(opCreateDeployment, err)
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
			"verify deployment_id with gitlab_deployment_list")
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
			"verify deployment_id with gitlab_deployment_list")
	}
	return nil
}

// Approve or Reject Deployment.

// ApproveOrRejectInput defines parameters for approving or rejecting a blocked deployment.
type ApproveOrRejectInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	DeploymentID  int                  `json:"deployment_id" jsonschema:"Deployment ID,required"`
	Status        string               `json:"status" jsonschema:"Approval status: approved or rejected,required"`
	Comment       string               `json:"comment,omitempty" jsonschema:"Optional comment for the approval or rejection"`
	RepresentedAs string               `json:"represented_as,omitempty" jsonschema:"Name of the approval rule to act as, when the user belongs to multiple approval rules"`
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
	if input.RepresentedAs != "" {
		opts.RepresentedAs = new(input.RepresentedAs)
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
			"verify deployment_id with gitlab_deployment_list \u2014 only deployments awaiting approval can be acted on")
	}

	return ApproveOrRejectOutput{
		Message: fmt.Sprintf("Deployment #%d %s successfully", input.DeploymentID, input.Status),
	}, nil
}
