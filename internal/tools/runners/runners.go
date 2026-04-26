// Package runners implements GitLab Runners API operations as MCP tools.
// It supports listing, getting, updating, removing runners, managing project/group
// runner assignments, listing runner jobs, registering/verifying runners,
// and resetting runner authentication tokens.
package runners

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const errRunnerIDRequired = "runner_id is required and must be > 0"

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a GitLab CI Runner in responses.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Name        string `json:"name"`
	Paused      bool   `json:"paused"`
	IsShared    bool   `json:"is_shared"`
	RunnerType  string `json:"runner_type"`
	Online      bool   `json:"online"`
	Status      string `json:"status"`
}

// DetailsOutput represents detailed runner information.
type DetailsOutput struct {
	toolutil.HintableOutput
	ID              int64    `json:"id"`
	Description     string   `json:"description"`
	Name            string   `json:"name"`
	Paused          bool     `json:"paused"`
	IsShared        bool     `json:"is_shared"`
	RunnerType      string   `json:"runner_type"`
	Online          bool     `json:"online"`
	Status          string   `json:"status"`
	ContactedAt     string   `json:"contacted_at,omitempty"`
	MaintenanceNote string   `json:"maintenance_note,omitempty"`
	TagList         []string `json:"tag_list,omitempty"`
	RunUntagged     bool     `json:"run_untagged"`
	Locked          bool     `json:"locked"`
	AccessLevel     string   `json:"access_level"`
	MaximumTimeout  int64    `json:"maximum_timeout,omitempty"`
	ProjectCount    int      `json:"project_count,omitempty"`
	GroupCount      int      `json:"group_count,omitempty"`
}

// ListOutput holds a paginated list of runners.
type ListOutput struct {
	toolutil.HintableOutput
	Runners    []Output                  `json:"runners"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// JobListOutput holds a paginated list of runner jobs.
type JobListOutput struct {
	toolutil.HintableOutput
	Jobs       []jobs.Output             `json:"jobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// AuthTokenOutput represents a runner authentication token.
type AuthTokenOutput struct {
	toolutil.HintableOutput
	Token     string `json:"token"`
	ExpiresAt string `json:"token_expires_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toOutput converts the GitLab API response to the tool output format.
func toOutput(r *gl.Runner) Output {
	return Output{
		ID:          r.ID,
		Description: r.Description,
		Name:        r.Name,
		Paused:      r.Paused,
		IsShared:    r.IsShared,
		RunnerType:  r.RunnerType,
		Online:      r.Online,
		Status:      r.Status,
	}
}

// toDetailsOutput converts the GitLab API response to the tool output format.
func toDetailsOutput(d *gl.RunnerDetails) DetailsOutput {
	out := DetailsOutput{
		ID:              d.ID,
		Description:     d.Description,
		Name:            d.Name,
		Paused:          d.Paused,
		IsShared:        d.IsShared,
		RunnerType:      d.RunnerType,
		Online:          d.Online,
		Status:          d.Status,
		MaintenanceNote: d.MaintenanceNote,
		TagList:         d.TagList,
		RunUntagged:     d.RunUntagged,
		Locked:          d.Locked,
		AccessLevel:     d.AccessLevel,
		MaximumTimeout:  d.MaximumTimeout,
		ProjectCount:    len(d.Projects),
		GroupCount:      len(d.Groups),
	}
	if d.ContactedAt != nil {
		out.ContactedAt = d.ContactedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// ListRunners — list owned runners
// ---------------------------------------------------------------------------.

// ListInput defines parameters for listing owned runners.
type ListInput struct {
	Type    string `json:"type,omitempty"    jsonschema:"Runner type filter: instance_type, group_type, project_type"`
	Status  string `json:"status,omitempty"  jsonschema:"Runner status filter: online, offline, stale, never_contacted"`
	Paused  *bool  `json:"paused,omitempty"  jsonschema:"Filter by paused state"`
	TagList string `json:"tag_list,omitempty" jsonschema:"Comma-separated list of tags to filter by"`
	toolutil.PaginationInput
}

// List returns owned runners with optional filters.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListRunnersOptions{}
	if input.Type != "" {
		opts.Type = new(input.Type)
	}
	if input.Status != "" {
		opts.Status = new(input.Status)
	}
	if input.Paused != nil {
		opts.Paused = input.Paused
	}
	if input.TagList != "" {
		tags := strings.Split(input.TagList, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		opts.TagList = &tags
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	runners, resp, err := client.GL().Runners.ListRunners(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list runners", err, http.StatusUnprocessableEntity,
			"status filter must be one of active|paused|online|offline|never_contacted|stale and type must be instance_type|group_type|project_type")
	}

	items := make([]Output, len(runners))
	for i, r := range runners {
		items[i] = toOutput(r)
	}
	return ListOutput{Runners: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// GetRunnerDetails
// ---------------------------------------------------------------------------.

// GetInput defines parameters for getting runner details.
type GetInput struct {
	RunnerID int64 `json:"runner_id" jsonschema:"Runner ID,required"`
}

// Get returns detailed information about a specific runner.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (DetailsOutput, error) {
	if input.RunnerID == 0 {
		return DetailsOutput{}, errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return DetailsOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	d, _, err := client.GL().Runners.GetRunnerDetails(int(input.RunnerID), gl.WithContext(ctx))
	if err != nil {
		return DetailsOutput{}, toolutil.WrapErrWithStatusHint("get runner details", err, http.StatusNotFound,
			"runner not found or already deleted \u2014 use gitlab_runner_list_all (admin) or gitlab_runner_list to discover current runner_id values")
	}
	return toDetailsOutput(d), nil
}

// ---------------------------------------------------------------------------
// UpdateRunnerDetails
// ---------------------------------------------------------------------------.

// UpdateInput defines parameters for updating a runner.
type UpdateInput struct {
	RunnerID        int64    `json:"runner_id"                    jsonschema:"Runner ID,required"`
	Description     string   `json:"description,omitempty"        jsonschema:"Runner description"`
	Paused          *bool    `json:"paused,omitempty"             jsonschema:"Pause/unpause the runner"`
	TagList         []string `json:"tag_list,omitempty"           jsonschema:"List of runner tags"`
	RunUntagged     *bool    `json:"run_untagged,omitempty"       jsonschema:"Whether to run untagged jobs"`
	Locked          *bool    `json:"locked,omitempty"             jsonschema:"Whether runner is locked to current project"`
	AccessLevel     string   `json:"access_level,omitempty"       jsonschema:"Access level: not_protected, ref_protected"`
	MaximumTimeout  *int64   `json:"maximum_timeout,omitempty"    jsonschema:"Maximum job timeout in seconds"`
	MaintenanceNote string   `json:"maintenance_note,omitempty"   jsonschema:"Maintenance note for the runner"`
}

// Update modifies a runner's configuration and returns updated details.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (DetailsOutput, error) {
	if input.RunnerID == 0 {
		return DetailsOutput{}, errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return DetailsOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.UpdateRunnerDetailsOptions{}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Paused != nil {
		opts.Paused = input.Paused
	}
	if len(input.TagList) > 0 {
		opts.TagList = &input.TagList
	}
	if input.RunUntagged != nil {
		opts.RunUntagged = input.RunUntagged
	}
	if input.Locked != nil {
		opts.Locked = input.Locked
	}
	if input.AccessLevel != "" {
		opts.AccessLevel = new(input.AccessLevel)
	}
	if input.MaximumTimeout != nil {
		opts.MaximumTimeout = input.MaximumTimeout
	}
	if input.MaintenanceNote != "" {
		opts.MaintenanceNote = new(input.MaintenanceNote)
	}

	d, _, err := client.GL().Runners.UpdateRunnerDetails(int(input.RunnerID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return DetailsOutput{}, toolutil.WrapErrWithHint("update runner details", err,
				"updating instance runners requires an admin token; group/project runners require Owner/Maintainer role on the owning scope")
		}
		return DetailsOutput{}, toolutil.WrapErrWithStatusHint("update runner details", err, http.StatusNotFound,
			"runner not found \u2014 verify runner_id with gitlab_runner_list")
	}
	return toDetailsOutput(d), nil
}

// ---------------------------------------------------------------------------
// RemoveRunner
// ---------------------------------------------------------------------------.

// RemoveInput defines parameters for removing a runner.
type RemoveInput struct {
	RunnerID int64 `json:"runner_id" jsonschema:"Runner ID to remove,required"`
}

// Remove deletes a runner by its ID.
func Remove(ctx context.Context, client *gitlabclient.Client, input RemoveInput) error {
	if input.RunnerID == 0 {
		return errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().Runners.RemoveRunner(int(input.RunnerID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("remove runner", err,
				"removing instance runners requires an admin token; for project runners use gitlab_runner_disable_project instead, for group runners require Owner role")
		}
		return toolutil.WrapErrWithStatusHint("remove runner", err, http.StatusNotFound,
			"runner already deleted or never existed \u2014 nothing to remove")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ListRunnerJobs
// ---------------------------------------------------------------------------.

// ListJobsInput defines parameters for listing jobs processed by a runner.
type ListJobsInput struct {
	RunnerID int64  `json:"runner_id"           jsonschema:"Runner ID,required"`
	Status   string `json:"status,omitempty"    jsonschema:"Job status filter: running, success, failed, canceled"`
	OrderBy  string `json:"order_by,omitempty"  jsonschema:"Order by field: id (default)"`
	Sort     string `json:"sort,omitempty"      jsonschema:"Sort direction: asc, desc"`
	toolutil.PaginationInput
}

// ListJobs returns jobs processed by a specific runner.
func ListJobs(ctx context.Context, client *gitlabclient.Client, input ListJobsInput) (JobListOutput, error) {
	if input.RunnerID == 0 {
		return JobListOutput{}, errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return JobListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListRunnerJobsOptions{}
	if input.Status != "" {
		opts.Status = new(input.Status)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	jobList, resp, err := client.GL().Runners.ListRunnerJobs(int(input.RunnerID), opts, gl.WithContext(ctx))
	if err != nil {
		return JobListOutput{}, toolutil.WrapErrWithStatusHint("list runner jobs", err, http.StatusNotFound,
			"runner not found \u2014 verify runner_id with gitlab_runner_get or gitlab_runner_list")
	}

	items := make([]jobs.Output, len(jobList))
	for i, j := range jobList {
		items[i] = jobs.ToOutput(j)
	}
	return JobListOutput{Jobs: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// ListProjectRunners
// ---------------------------------------------------------------------------.

// ListProjectInput defines parameters for listing project runners.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"           jsonschema:"Project ID or URL-encoded path,required"`
	Type      string               `json:"type,omitempty"       jsonschema:"Runner type filter: instance_type, group_type, project_type"`
	Status    string               `json:"status,omitempty"     jsonschema:"Runner status filter: online, offline, stale, never_contacted"`
	TagList   string               `json:"tag_list,omitempty"   jsonschema:"Comma-separated list of tags to filter by"`
	toolutil.PaginationInput
}

// ListProject returns runners assigned to a specific project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListProjectRunnersOptions{}
	if input.Type != "" {
		opts.Type = new(input.Type)
	}
	if input.Status != "" {
		opts.Status = new(input.Status)
	}
	if input.TagList != "" {
		tags := strings.Split(input.TagList, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		opts.TagList = &tags
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	runners, resp, err := client.GL().Runners.ListProjectRunners(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list project runners", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get \u2014 use namespace/project path or numeric ID")
	}

	items := make([]Output, len(runners))
	for i, r := range runners {
		items[i] = toOutput(r)
	}
	return ListOutput{Runners: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// EnableProjectRunner
// ---------------------------------------------------------------------------.

// EnableProjectInput defines parameters for assigning a runner to a project.
type EnableProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RunnerID  int64                `json:"runner_id"  jsonschema:"Runner ID to assign,required"`
}

// EnableProject assigns a runner to a project.
func EnableProject(ctx context.Context, client *gitlabclient.Client, input EnableProjectInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.RunnerID == 0 {
		return Output{}, errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.EnableProjectRunnerOptions{
		RunnerID: input.RunnerID,
	}

	r, _, err := client.GL().Runners.EnableProjectRunner(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("enable project runner", err,
				"runner is locked to another project (set locked=false via gitlab_runner_update first), is a group/instance runner that cannot be enabled per-project, or you need Maintainer/Owner role")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("enable project runner", err, http.StatusNotFound,
			"runner_id or project_id not found \u2014 verify with gitlab_runner_list and gitlab_project_get")
	}
	return toOutput(r), nil
}

// ---------------------------------------------------------------------------
// DisableProjectRunner
// ---------------------------------------------------------------------------.

// DisableProjectInput defines parameters for removing a runner from a project.
type DisableProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	RunnerID  int64                `json:"runner_id"  jsonschema:"Runner ID to remove from project,required"`
}

// DisableProject removes a runner from a project.
func DisableProject(ctx context.Context, client *gitlabclient.Client, input DisableProjectInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.RunnerID == 0 {
		return errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().Runners.DisableProjectRunner(string(input.ProjectID), input.RunnerID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("disable project runner", err, http.StatusNotFound,
			"runner is not currently assigned to this project \u2014 use gitlab_runner_list_project to see assigned runners")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ListGroupsRunners
// ---------------------------------------------------------------------------.

// ListGroupInput defines parameters for listing group runners.
type ListGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id"            jsonschema:"Group ID or URL-encoded path,required"`
	Type    string               `json:"type,omitempty"      jsonschema:"Runner type filter: instance_type, group_type, project_type"`
	Status  string               `json:"status,omitempty"    jsonschema:"Runner status filter: online, offline, stale, never_contacted"`
	TagList string               `json:"tag_list,omitempty"  jsonschema:"Comma-separated list of tags to filter by"`
	toolutil.PaginationInput
}

// ListGroup returns runners available in a specific group.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListGroupsRunnersOptions{}
	if input.Type != "" {
		opts.Type = new(input.Type)
	}
	if input.Status != "" {
		opts.Status = new(input.Status)
	}
	if input.TagList != "" {
		tags := strings.Split(input.TagList, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		opts.TagList = &tags
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	runners, resp, err := client.GL().Runners.ListGroupsRunners(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list group runners", err, http.StatusNotFound,
			"verify the group exists with gitlab_group_get \u2014 use group full_path or numeric ID")
	}

	items := make([]Output, len(runners))
	for i, r := range runners {
		items[i] = toOutput(r)
	}
	return ListOutput{Runners: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// RegisterNewRunner
// ---------------------------------------------------------------------------.

// RegisterInput defines parameters for registering a new runner.
type RegisterInput struct {
	Token           string   `json:"token"                        jsonschema:"Registration token,required"`
	Description     string   `json:"description,omitempty"        jsonschema:"Runner description"`
	Paused          *bool    `json:"paused,omitempty"             jsonschema:"Register in paused state"`
	Locked          *bool    `json:"locked,omitempty"             jsonschema:"Lock runner to current project"`
	RunUntagged     *bool    `json:"run_untagged,omitempty"       jsonschema:"Whether to run untagged jobs"`
	TagList         []string `json:"tag_list,omitempty"           jsonschema:"List of runner tags"`
	AccessLevel     string   `json:"access_level,omitempty"       jsonschema:"Access level: not_protected, ref_protected"`
	MaximumTimeout  *int64   `json:"maximum_timeout,omitempty"    jsonschema:"Maximum job timeout in seconds"`
	MaintenanceNote string   `json:"maintenance_note,omitempty"   jsonschema:"Maintenance note"`
}

// Register creates a new runner with the given registration token.
func Register(ctx context.Context, client *gitlabclient.Client, input RegisterInput) (Output, error) {
	if input.Token == "" {
		return Output{}, toolutil.ErrFieldRequired("token")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.RegisterNewRunnerOptions{
		Token: new(input.Token),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Paused != nil {
		opts.Paused = input.Paused
	}
	if input.Locked != nil {
		opts.Locked = input.Locked
	}
	if input.RunUntagged != nil {
		opts.RunUntagged = input.RunUntagged
	}
	if len(input.TagList) > 0 {
		opts.TagList = &input.TagList
	}
	if input.AccessLevel != "" {
		opts.AccessLevel = new(input.AccessLevel)
	}
	if input.MaximumTimeout != nil {
		opts.MaximumTimeout = input.MaximumTimeout
	}
	if input.MaintenanceNote != "" {
		opts.MaintenanceNote = new(input.MaintenanceNote)
	}

	r, _, err := client.GL().Runners.RegisterNewRunner(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("register new runner", err,
				"registration token is invalid, expired, or has been revoked \u2014 obtain a fresh token via gitlab_runner_reset_instance_reg_token (admin), gitlab_runner_reset_group_reg_token, or gitlab_runner_reset_project_reg_token")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("register new runner", err, http.StatusUnprocessableEntity,
			"validation failed \u2014 ensure token is non-empty and any tag_list entries are valid")
	}
	return toOutput(r), nil
}

// ---------------------------------------------------------------------------
// DeleteRegisteredRunnerByID
// ---------------------------------------------------------------------------.

// DeleteByIDInput defines parameters for deleting a registered runner by ID.
type DeleteByIDInput struct {
	RunnerID int64 `json:"runner_id" jsonschema:"Runner ID to delete,required"`
}

// DeleteByID deletes a registered runner by its ID.
func DeleteByID(ctx context.Context, client *gitlabclient.Client, input DeleteByIDInput) error {
	if input.RunnerID == 0 {
		return errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	_, err := client.GL().Runners.DeleteRegisteredRunnerByID(input.RunnerID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete registered runner", err, http.StatusNotFound,
			"runner already deleted or never existed \u2014 nothing to remove")
	}
	return nil
}

// ---------------------------------------------------------------------------
// VerifyRegisteredRunner
// ---------------------------------------------------------------------------.

// VerifyInput defines parameters for verifying a runner token.
type VerifyInput struct {
	Token string `json:"token" jsonschema:"Runner authentication token to verify,required"`
}

// Verify checks whether a runner authentication token is valid.
func Verify(ctx context.Context, client *gitlabclient.Client, input VerifyInput) error {
	if input.Token == "" {
		return toolutil.ErrFieldRequired("token")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.VerifyRegisteredRunnerOptions{
		Token: new(input.Token),
	}
	_, err := client.GL().Runners.VerifyRegisteredRunner(opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("verify runner", err, http.StatusForbidden,
			"runner authentication token is invalid, expired, or has been reset \u2014 obtain a fresh token via gitlab_runner_reset_auth_token or re-register the runner")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ResetRunnerAuthenticationToken
// ---------------------------------------------------------------------------.

// ResetAuthTokenInput defines parameters for resetting a runner's auth token.
type ResetAuthTokenInput struct {
	RunnerID int64 `json:"runner_id" jsonschema:"Runner ID,required"`
}

// ResetAuthToken resets the authentication token for a runner.
func ResetAuthToken(ctx context.Context, client *gitlabclient.Client, input ResetAuthTokenInput) (AuthTokenOutput, error) {
	if input.RunnerID == 0 {
		return AuthTokenOutput{}, errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return AuthTokenOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().Runners.ResetRunnerAuthenticationToken(input.RunnerID, gl.WithContext(ctx))
	if err != nil {
		return AuthTokenOutput{}, toolutil.WrapErrWithStatusHint("reset runner auth token", err, http.StatusNotFound,
			"runner not found \u2014 verify runner_id with gitlab_runner_list")
	}

	out := AuthTokenOutput{}
	if t.Token != nil {
		out.Token = *t.Token
	}
	if t.TokenExpiresAt != nil {
		out.ExpiresAt = t.TokenExpiresAt.Format(time.RFC3339)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ListAllRunners — admin list of all runners (instance-level)
// ---------------------------------------------------------------------------.

// ListAllInput defines parameters for listing all runners (admin).
type ListAllInput struct {
	Type    string `json:"type,omitempty"    jsonschema:"Runner type filter: instance_type, group_type, project_type"`
	Status  string `json:"status,omitempty"  jsonschema:"Runner status filter: online, offline, stale, never_contacted"`
	Paused  *bool  `json:"paused,omitempty"  jsonschema:"Filter by paused state"`
	TagList string `json:"tag_list,omitempty" jsonschema:"Comma-separated list of tags to filter by"`
	toolutil.PaginationInput
}

// ListAll returns all runners across the GitLab instance (admin endpoint).
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListAllInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.ListRunnersOptions{}
	if input.Type != "" {
		opts.Type = new(input.Type)
	}
	if input.Status != "" {
		opts.Status = new(input.Status)
	}
	if input.Paused != nil {
		opts.Paused = input.Paused
	}
	if input.TagList != "" {
		tags := strings.Split(input.TagList, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		opts.TagList = &tags
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}

	runners, resp, err := client.GL().Runners.ListAllRunners(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list all runners", err, http.StatusForbidden,
			"listing all instance runners requires an admin token \u2014 use gitlab_runner_list (scoped to your accessible runners) instead")
	}

	items := make([]Output, len(runners))
	for i, r := range runners {
		items[i] = toOutput(r)
	}
	return ListOutput{Runners: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// DeleteRegisteredRunner — delete runner by authentication token
// ---------------------------------------------------------------------------.

// DeleteByTokenInput defines parameters for deleting a runner by its token.
type DeleteByTokenInput struct {
	Token string `json:"token" jsonschema:"Runner authentication token,required"`
}

// DeleteByToken deletes a registered runner using its authentication token.
func DeleteByToken(ctx context.Context, client *gitlabclient.Client, input DeleteByTokenInput) error {
	if input.Token == "" {
		return toolutil.ErrFieldRequired("token")
	}
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	opts := &gl.DeleteRegisteredRunnerOptions{
		Token: new(input.Token),
	}
	_, err := client.GL().Runners.DeleteRegisteredRunner(opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete registered runner by token", err, http.StatusForbidden,
			"authentication token is invalid or already revoked \u2014 if you have the runner_id, use gitlab_runner_delete_by_id instead")
	}
	return nil
}

// ---------------------------------------------------------------------------
// ResetInstanceRunnerRegistrationToken
// ---------------------------------------------------------------------------.

// ResetInstanceRegTokenInput is an empty struct for the instance reg token reset (no parameters needed).
type ResetInstanceRegTokenInput struct{}

// ResetInstanceRegToken resets the instance-level runner registration token.
//
// Deprecated: Scheduled for removal in GitLab 20.0.
func ResetInstanceRegToken(ctx context.Context, client *gitlabclient.Client, _ ResetInstanceRegTokenInput) (AuthTokenOutput, error) {
	if err := ctx.Err(); err != nil {
		return AuthTokenOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().Runners.ResetInstanceRunnerRegistrationToken(gl.WithContext(ctx))
	if err != nil {
		return AuthTokenOutput{}, toolutil.WrapErrWithStatusHint("reset instance runner registration token", err, http.StatusForbidden,
			"resetting the instance-level registration token requires an admin token \u2014 for group/project scopes use gitlab_runner_reset_group_reg_token / gitlab_runner_reset_project_reg_token")
	}
	return toRegTokenOutput(t), nil
}

// ---------------------------------------------------------------------------
// ResetGroupRunnerRegistrationToken
// ---------------------------------------------------------------------------.

// ResetGroupRegTokenInput defines parameters for resetting group runner reg token.
type ResetGroupRegTokenInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// ResetGroupRegToken resets a group's runner registration token.
//
// Deprecated: Scheduled for removal in GitLab 20.0.
func ResetGroupRegToken(ctx context.Context, client *gitlabclient.Client, input ResetGroupRegTokenInput) (AuthTokenOutput, error) {
	if input.GroupID == "" {
		return AuthTokenOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	if err := ctx.Err(); err != nil {
		return AuthTokenOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().Runners.ResetGroupRunnerRegistrationToken(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return AuthTokenOutput{}, toolutil.WrapErrWithHint("reset group runner registration token", err,
				"resetting a group runner registration token requires Owner role on the group")
		}
		return AuthTokenOutput{}, toolutil.WrapErrWithStatusHint("reset group runner registration token", err, http.StatusNotFound,
			"verify the group exists with gitlab_group_get")
	}
	return toRegTokenOutput(t), nil
}

// ---------------------------------------------------------------------------
// ResetProjectRunnerRegistrationToken
// ---------------------------------------------------------------------------.

// ResetProjectRegTokenInput defines parameters for resetting project runner reg token.
type ResetProjectRegTokenInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ResetProjectRegToken resets a project's runner registration token.
//
// Deprecated: Scheduled for removal in GitLab 20.0.
func ResetProjectRegToken(ctx context.Context, client *gitlabclient.Client, input ResetProjectRegTokenInput) (AuthTokenOutput, error) {
	if input.ProjectID == "" {
		return AuthTokenOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if err := ctx.Err(); err != nil {
		return AuthTokenOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	t, _, err := client.GL().Runners.ResetProjectRunnerRegistrationToken(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return AuthTokenOutput{}, toolutil.WrapErrWithHint("reset project runner registration token", err,
				"resetting a project runner registration token requires Maintainer or Owner role on the project")
		}
		return AuthTokenOutput{}, toolutil.WrapErrWithStatusHint("reset project runner registration token", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	return toRegTokenOutput(t), nil
}

// toRegTokenOutput converts a RunnerRegistrationToken to AuthTokenOutput.
func toRegTokenOutput(t *gl.RunnerRegistrationToken) AuthTokenOutput {
	out := AuthTokenOutput{}
	if t.Token != nil {
		out.Token = *t.Token
	}
	if t.TokenExpiresAt != nil {
		out.ExpiresAt = t.TokenExpiresAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// ListRunnerManagers — list managers for a specific runner
// ---------------------------------------------------------------------------.

// ManagerOutput represents a runner manager in responses.
type ManagerOutput struct {
	ID           int64  `json:"id"`
	SystemID     string `json:"system_id"`
	Version      string `json:"version"`
	Revision     string `json:"revision"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	CreatedAt    string `json:"created_at,omitempty"`
	ContactedAt  string `json:"contacted_at,omitempty"`
	IPAddress    string `json:"ip_address"`
	Status       string `json:"status"`
}

// ManagerListOutput holds a list of runner managers.
type ManagerListOutput struct {
	toolutil.HintableOutput
	Managers []ManagerOutput `json:"managers"`
}

// ListManagersInput defines parameters for listing runner managers.
type ListManagersInput struct {
	RunnerID int64 `json:"runner_id" jsonschema:"Runner ID,required"`
}

// ListManagers retrieves all managers for a specific runner.
func ListManagers(ctx context.Context, client *gitlabclient.Client, input ListManagersInput) (ManagerListOutput, error) {
	if input.RunnerID == 0 {
		return ManagerListOutput{}, errors.New(errRunnerIDRequired)
	}
	if err := ctx.Err(); err != nil {
		return ManagerListOutput{}, toolutil.WrapErrWithMessage(toolutil.ErrMsgContextCanceled, err)
	}

	managers, _, err := client.GL().Runners.ListRunnerManagers(int(input.RunnerID), gl.WithContext(ctx))
	if err != nil {
		return ManagerListOutput{}, toolutil.WrapErrWithStatusHint("list runner managers", err, http.StatusNotFound,
			"runner not found \u2014 verify runner_id with gitlab_runner_list")
	}

	items := make([]ManagerOutput, len(managers))
	for i, m := range managers {
		items[i] = toManagerOutput(m)
	}
	return ManagerListOutput{Managers: items}, nil
}

// toManagerOutput converts the GitLab API response to the tool output format.
func toManagerOutput(m *gl.RunnerManager) ManagerOutput {
	out := ManagerOutput{
		ID:           m.ID,
		SystemID:     m.SystemID,
		Version:      m.Version,
		Revision:     m.Revision,
		Platform:     m.Platform,
		Architecture: m.Architecture,
		IPAddress:    m.IPAddress,
		Status:       m.Status,
	}
	if m.CreatedAt != nil {
		out.CreatedAt = m.CreatedAt.Format(time.RFC3339)
	}
	if m.ContactedAt != nil {
		out.ContactedAt = m.ContactedAt.Format(time.RFC3339)
	}
	return out
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
