// Package deploykeys implements GitLab Deploy Keys API operations as MCP tools.
// It supports listing, getting, creating, updating, deleting, and enabling
// project deploy keys, as well as listing and creating instance-level deploy keys
// and listing deploy keys by user.
package deploykeys

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// Output represents a project-level deploy key.
type Output struct {
	toolutil.HintableOutput
	ID                int64  `json:"id"`
	Title             string `json:"title"`
	Key               string `json:"key"`
	Fingerprint       string `json:"fingerprint,omitempty"`
	FingerprintSHA256 string `json:"fingerprint_sha256,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	CanPush           bool   `json:"can_push"`
	ExpiresAt         string `json:"expires_at,omitempty"`
}

// InstanceOutput represents an instance-level deploy key.
type InstanceOutput struct {
	toolutil.HintableOutput
	ID                         int64            `json:"id"`
	Title                      string           `json:"title"`
	Key                        string           `json:"key"`
	Fingerprint                string           `json:"fingerprint,omitempty"`
	FingerprintSHA256          string           `json:"fingerprint_sha256,omitempty"`
	CreatedAt                  string           `json:"created_at,omitempty"`
	ExpiresAt                  string           `json:"expires_at,omitempty"`
	ProjectsWithWriteAccess    []ProjectSummary `json:"projects_with_write_access,omitempty"`
	ProjectsWithReadonlyAccess []ProjectSummary `json:"projects_with_readonly_access,omitempty"`
}

// ProjectSummary holds basic project info for an InstanceDeployKey.
type ProjectSummary struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// ListOutput holds a paginated list of project deploy keys.
type ListOutput struct {
	toolutil.HintableOutput
	DeployKeys []Output                  `json:"deploy_keys"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// InstanceListOutput holds a paginated list of instance deploy keys.
type InstanceListOutput struct {
	toolutil.HintableOutput
	DeployKeys []InstanceOutput          `json:"deploy_keys"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// timeStr is an internal helper for the deploykeys package.
func timeStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// toOutput converts the GitLab API response to the tool output format.
func toOutput(k *gl.ProjectDeployKey) Output {
	return Output{
		ID:                k.ID,
		Title:             k.Title,
		Key:               k.Key,
		Fingerprint:       k.Fingerprint,
		FingerprintSHA256: k.FingerprintSHA256,
		CreatedAt:         timeStr(k.CreatedAt),
		CanPush:           k.CanPush,
		ExpiresAt:         timeStr(k.ExpiresAt),
	}
}

// toProjectSummary converts the GitLab API response to the tool output format.
func toProjectSummary(p *gl.DeployKeyProject) ProjectSummary {
	return ProjectSummary{
		ID:                p.ID,
		Name:              p.Name,
		PathWithNamespace: p.PathWithNamespace,
		CreatedAt:         timeStr(p.CreatedAt),
	}
}

// toInstanceOutput converts the GitLab API response to the tool output format.
func toInstanceOutput(k *gl.InstanceDeployKey) InstanceOutput {
	out := InstanceOutput{
		ID:                k.ID,
		Title:             k.Title,
		Key:               k.Key,
		Fingerprint:       k.Fingerprint,
		FingerprintSHA256: k.FingerprintSHA256,
		CreatedAt:         timeStr(k.CreatedAt),
		ExpiresAt:         timeStr(k.ExpiresAt),
	}
	for _, p := range k.ProjectsWithWriteAccess {
		out.ProjectsWithWriteAccess = append(out.ProjectsWithWriteAccess, toProjectSummary(p))
	}
	for _, p := range k.ProjectsWithReadonlyAccess {
		out.ProjectsWithReadonlyAccess = append(out.ProjectsWithReadonlyAccess, toProjectSummary(p))
	}
	return out
}

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// ListProjectInput represents parameters for listing project deploy keys.
type ListProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Page      int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage   int                  `json:"per_page,omitempty" jsonschema:"Results per page (max 100)"`
}

// GetInput represents parameters for getting a single deploy key.
type GetInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	DeployKeyID int64                `json:"deploy_key_id" jsonschema:"Deploy key ID,required"`
}

// AddInput represents parameters for adding a deploy key to a project.
type AddInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Title     string               `json:"title" jsonschema:"Deploy key title,required"`
	Key       string               `json:"key" jsonschema:"Public SSH key content,required"`
	CanPush   *bool                `json:"can_push,omitempty" jsonschema:"Whether the key can push to the project"`
	ExpiresAt string               `json:"expires_at,omitempty" jsonschema:"Expiry date (YYYY-MM-DD)"`
}

// UpdateInput represents parameters for updating a deploy key.
type UpdateInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	DeployKeyID int64                `json:"deploy_key_id" jsonschema:"Deploy key ID,required"`
	Title       string               `json:"title,omitempty" jsonschema:"New deploy key title"`
	CanPush     *bool                `json:"can_push,omitempty" jsonschema:"Whether the key can push to the project"`
}

// DeleteInput represents parameters for deleting a deploy key.
type DeleteInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	DeployKeyID int64                `json:"deploy_key_id" jsonschema:"Deploy key ID,required"`
}

// EnableInput represents parameters for enabling a deploy key for a project.
type EnableInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	DeployKeyID int64                `json:"deploy_key_id" jsonschema:"Deploy key ID to enable,required"`
}

// ListAllInput represents parameters for listing all instance-level deploy keys.
type ListAllInput struct {
	Public  *bool `json:"public,omitempty" jsonschema:"Filter by public keys"`
	Page    int   `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int   `json:"per_page,omitempty" jsonschema:"Results per page (max 100)"`
}

// AddInstanceInput represents parameters for creating an instance-level deploy key.
type AddInstanceInput struct {
	Title     string `json:"title" jsonschema:"Deploy key title,required"`
	Key       string `json:"key" jsonschema:"Public SSH key content,required"`
	ExpiresAt string `json:"expires_at,omitempty" jsonschema:"Expiry date (YYYY-MM-DD)"`
}

// ListUserProjectInput represents parameters for listing a user's project deploy keys.
type ListUserProjectInput struct {
	UserID  toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	Page    int                  `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int                  `json:"per_page,omitempty" jsonschema:"Results per page (max 100)"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------.

// ListProject lists all deploy keys for a project.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := &gl.ListProjectDeployKeysOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	keys, resp, err := client.GL().DeployKeys.ListProjectDeployKeys(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("deploy_key_list_project", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; deploy keys list requires Maintainer role")
	}

	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, k := range keys {
		out.DeployKeys = append(out.DeployKeys, toOutput(k))
	}
	return out, nil
}

// Get retrieves a single deploy key by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.DeployKeyID == 0 {
		return Output{}, toolutil.ErrFieldRequired("deploy_key_id")
	}

	key, _, err := client.GL().DeployKeys.GetDeployKey(string(input.ProjectID), input.DeployKeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("deploy_key_get", err, http.StatusNotFound,
			"verify deploy_key_id with gitlab_deploy_key_list_project; the key must currently be enabled on this project")
	}

	return toOutput(key), nil
}

// Add adds a deploy key to a project.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Title == "" {
		return Output{}, toolutil.ErrFieldRequired("title")
	}
	if input.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}

	opts := &gl.AddDeployKeyOptions{
		Title: new(input.Title),
		Key:   new(input.Key),
	}

	if input.CanPush != nil {
		opts.CanPush = input.CanPush
	}

	if input.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", input.ExpiresAt)
		if err != nil {
			return Output{}, fmt.Errorf("invalid expires_at format, use YYYY-MM-DD: %w", err)
		}
		opts.ExpiresAt = &t
	}

	key, _, err := client.GL().DeployKeys.AddDeployKey(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("deploy_key_add", err, http.StatusBadRequest,
			"key must be a valid SSH public key (ssh-rsa/ed25519/ecdsa) and unique within the instance; title must be unique within the project; requires Maintainer role")
	}

	return toOutput(key), nil
}

// Update updates an existing deploy key.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.DeployKeyID == 0 {
		return Output{}, toolutil.ErrFieldRequired("deploy_key_id")
	}

	opts := &gl.UpdateDeployKeyOptions{}
	if input.Title != "" {
		opts.Title = new(input.Title)
	}
	if input.CanPush != nil {
		opts.CanPush = input.CanPush
	}

	key, _, err := client.GL().DeployKeys.UpdateDeployKey(string(input.ProjectID), input.DeployKeyID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("deploy_key_update", err, http.StatusForbidden,
			"updating deploy keys requires Maintainer role; only keys originally created in this project can be edited (not keys enabled from other projects)")
	}

	return toOutput(key), nil
}

// Delete removes a deploy key from a project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.DeployKeyID == 0 {
		return toolutil.ErrFieldRequired("deploy_key_id")
	}

	_, err := client.GL().DeployKeys.DeleteDeployKey(string(input.ProjectID), input.DeployKeyID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("deploy_key_delete", err, http.StatusForbidden,
			"deleting deploy keys requires Maintainer role; deleting a key removes it from all projects where it is enabled")
	}

	return nil
}

// Enable enables a deploy key for a project.
func Enable(ctx context.Context, client *gitlabclient.Client, input EnableInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.DeployKeyID == 0 {
		return Output{}, toolutil.ErrFieldRequired("deploy_key_id")
	}

	key, _, err := client.GL().DeployKeys.EnableDeployKey(string(input.ProjectID), input.DeployKeyID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("deploy_key_enable", err, http.StatusNotFound,
			"verify deploy_key_id exists at instance level via gitlab_deploy_key_list_all; the key may already be enabled")
	}

	return toOutput(key), nil
}

// ListAll lists all instance-level deploy keys.
func ListAll(ctx context.Context, client *gitlabclient.Client, input ListAllInput) (InstanceListOutput, error) {
	opts := &gl.ListInstanceDeployKeysOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Public != nil {
		opts.Public = input.Public
	}

	keys, resp, err := client.GL().DeployKeys.ListAllDeployKeys(opts, gl.WithContext(ctx))
	if err != nil {
		return InstanceListOutput{}, toolutil.WrapErrWithStatusHint("deploy_key_list_all", err, http.StatusForbidden,
			"listing all instance deploy keys requires admin token")
	}

	out := InstanceListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, k := range keys {
		out.DeployKeys = append(out.DeployKeys, toInstanceOutput(k))
	}
	return out, nil
}

// AddInstance creates an instance-level deploy key.
func AddInstance(ctx context.Context, client *gitlabclient.Client, input AddInstanceInput) (InstanceOutput, error) {
	if input.Title == "" {
		return InstanceOutput{}, toolutil.ErrFieldRequired("title")
	}
	if input.Key == "" {
		return InstanceOutput{}, toolutil.ErrFieldRequired("key")
	}

	opts := &gl.AddInstanceDeployKeyOptions{
		Title: new(input.Title),
		Key:   new(input.Key),
	}

	if input.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", input.ExpiresAt)
		if err != nil {
			return InstanceOutput{}, fmt.Errorf("invalid expires_at format, use YYYY-MM-DD: %w", err)
		}
		opts.ExpiresAt = &t
	}

	key, _, err := client.GL().DeployKeys.AddInstanceDeployKey(opts, gl.WithContext(ctx))
	if err != nil {
		return InstanceOutput{}, toolutil.WrapErrWithStatusHint("deploy_key_add_instance", err, http.StatusForbidden,
			"creating instance-level deploy keys requires admin token; key must be unique")
	}

	return toInstanceOutput(key), nil
}

// ListUserProject lists deploy keys for a specific user's projects.
func ListUserProject(ctx context.Context, client *gitlabclient.Client, input ListUserProjectInput) (ListOutput, error) {
	if input.UserID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("user_id")
	}

	opts := &gl.ListUserProjectDeployKeysOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	keys, resp, err := client.GL().DeployKeys.ListUserProjectDeployKeys(string(input.UserID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("deploy_key_list_user_project", err, http.StatusNotFound,
			"verify user_id with gitlab_user_get; admin token required to query other users' deploy keys")
	}

	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, k := range keys {
		out.DeployKeys = append(out.DeployKeys, toOutput(k))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
