// Package projectmirrors implements GitLab project remote mirror (push mirror) operations
// including list, get, get public key, add, edit, delete, and force push update.
package projectmirrors

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// hintVerifyMirrorID is the 404 hint shared by project mirror tools.
const hintVerifyMirrorID = "verify mirror_id with gitlab_list_project_mirrors"

// ListInput holds parameters for listing project mirrors.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetInput holds parameters for retrieving a single project mirror.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MirrorID  int64                `json:"mirror_id"  jsonschema:"Remote mirror ID,required"`
}

// GetPublicKeyInput holds parameters for retrieving a mirror's SSH public key.
type GetPublicKeyInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MirrorID  int64                `json:"mirror_id"  jsonschema:"Remote mirror ID,required"`
}

// AddInput holds parameters for creating a new project mirror.
type AddInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	URL                   string               `json:"url"                               jsonschema:"Remote mirror URL (e.g. https://user:token@example.com/repo.git),required"`
	Enabled               *bool                `json:"enabled,omitempty"                 jsonschema:"Whether the mirror is enabled"`
	KeepDivergentRefs     *bool                `json:"keep_divergent_refs,omitempty"     jsonschema:"Keep divergent refs on the remote"`
	OnlyProtectedBranches *bool                `json:"only_protected_branches,omitempty" jsonschema:"Mirror only protected branches"`
	MirrorBranchRegex     string               `json:"mirror_branch_regex,omitempty"     jsonschema:"Regex pattern for branches to mirror"`
	AuthMethod            string               `json:"auth_method,omitempty"             jsonschema:"Authentication method (password or ssh_public_key)"`
	HostKeys              []string             `json:"host_keys,omitempty"               jsonschema:"SSH host keys for the remote mirror"`
}

// EditInput holds parameters for updating an existing project mirror.
type EditInput struct {
	ProjectID             toolutil.StringOrInt `json:"project_id"                        jsonschema:"Project ID or URL-encoded path,required"`
	MirrorID              int64                `json:"mirror_id"                         jsonschema:"Remote mirror ID,required"`
	Enabled               *bool                `json:"enabled,omitempty"                 jsonschema:"Whether the mirror is enabled"`
	KeepDivergentRefs     *bool                `json:"keep_divergent_refs,omitempty"     jsonschema:"Keep divergent refs on the remote"`
	OnlyProtectedBranches *bool                `json:"only_protected_branches,omitempty" jsonschema:"Mirror only protected branches"`
	MirrorBranchRegex     string               `json:"mirror_branch_regex,omitempty"     jsonschema:"Regex pattern for branches to mirror"`
	AuthMethod            string               `json:"auth_method,omitempty"             jsonschema:"Authentication method (password or ssh_public_key)"`
	HostKeys              []string             `json:"host_keys,omitempty"               jsonschema:"SSH host keys for the remote mirror"`
}

// DeleteInput holds parameters for deleting a project mirror.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MirrorID  int64                `json:"mirror_id"  jsonschema:"Remote mirror ID,required"`
}

// ForcePushInput holds parameters for triggering a forced push mirror update.
type ForcePushInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	MirrorID  int64                `json:"mirror_id"  jsonschema:"Remote mirror ID,required"`
}

// HostKeyOutput represents a host key fingerprint for an SSH-based mirror.
type HostKeyOutput struct {
	FingerprintSHA256 string `json:"fingerprint_sha256"`
}

// Output represents a project mirror.
type Output struct {
	toolutil.HintableOutput
	ID                     int64           `json:"id"`
	Enabled                bool            `json:"enabled"`
	URL                    string          `json:"url"`
	UpdateStatus           string          `json:"update_status"`
	LastError              string          `json:"last_error,omitempty"`
	LastSuccessfulUpdateAt string          `json:"last_successful_update_at,omitempty"`
	LastUpdateAt           string          `json:"last_update_at,omitempty"`
	LastUpdateStartedAt    string          `json:"last_update_started_at,omitempty"`
	OnlyProtectedBranches  bool            `json:"only_protected_branches"`
	KeepDivergentRefs      bool            `json:"keep_divergent_refs"`
	MirrorBranchRegex      string          `json:"mirror_branch_regex,omitempty"`
	AuthMethod             string          `json:"auth_method,omitempty"`
	HostKeys               []HostKeyOutput `json:"host_keys,omitempty"`
}

// ListOutput contains a paginated list of project mirrors.
type ListOutput struct {
	toolutil.HintableOutput
	Mirrors    []Output                  `json:"mirrors"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// PublicKeyOutput represents a mirror's SSH public key.
type PublicKeyOutput struct {
	toolutil.HintableOutput
	PublicKey string `json:"public_key"`
}

func toOutput(m *gl.ProjectMirror) Output {
	o := Output{
		ID:                    m.ID,
		Enabled:               m.Enabled,
		URL:                   m.URL,
		UpdateStatus:          m.UpdateStatus,
		LastError:             m.LastError,
		OnlyProtectedBranches: m.OnlyProtectedBranches,
		KeepDivergentRefs:     m.KeepDivergentRefs,
		MirrorBranchRegex:     m.MirrorBranchRegex,
		AuthMethod:            m.AuthMethod,
	}
	if m.LastSuccessfulUpdateAt != nil {
		o.LastSuccessfulUpdateAt = m.LastSuccessfulUpdateAt.Format(toolutil.DateTimeFormat)
	}
	if m.LastUpdateAt != nil {
		o.LastUpdateAt = m.LastUpdateAt.Format(toolutil.DateTimeFormat)
	}
	if m.LastUpdateStartedAt != nil {
		o.LastUpdateStartedAt = m.LastUpdateStartedAt.Format(toolutil.DateTimeFormat)
	}
	if m.HostKeys != nil {
		for _, hk := range *m.HostKeys {
			o.HostKeys = append(o.HostKeys, HostKeyOutput{FingerprintSHA256: hk.FingerprintSHA256})
		}
	}
	return o
}

// List returns all remote mirrors for a project.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("projectMirrorList", err)
	}
	if in.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := &gl.ListProjectMirrorOptions{}
	if in.Page > 0 {
		opts.Page = int64(in.Page)
	}
	if in.PerPage > 0 {
		opts.PerPage = int64(in.PerPage)
	}
	mirrors, resp, err := client.GL().ProjectMirrors.ListProjectMirror(string(in.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ListOutput{}, toolutil.WrapErrWithHint("projectMirrorList", err,
				"push mirroring requires GitLab Premium/Ultimate \u2014 verify the project tier and that you have Maintainer+ role")
		}
		return ListOutput{}, toolutil.WrapErrWithStatusHint("projectMirrorList", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get")
	}
	out := ListOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, m := range mirrors {
		out.Mirrors = append(out.Mirrors, toOutput(m))
	}
	return out, nil
}

// Get returns a single remote mirror for a project.
func Get(ctx context.Context, client *gitlabclient.Client, in GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectMirrorGet", err)
	}
	if in.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.MirrorID == 0 {
		return Output{}, toolutil.ErrFieldRequired("mirror_id")
	}
	m, _, err := client.GL().ProjectMirrors.GetProjectMirror(string(in.ProjectID), in.MirrorID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("projectMirrorGet", err, http.StatusNotFound,
			"verify mirror_id with gitlab_list_project_mirrors \u2014 push mirrors require GitLab Premium/Ultimate")
	}
	return toOutput(m), nil
}

// GetPublicKey returns the SSH public key for a remote mirror.
func GetPublicKey(ctx context.Context, client *gitlabclient.Client, in GetPublicKeyInput) (PublicKeyOutput, error) {
	if err := ctx.Err(); err != nil {
		return PublicKeyOutput{}, toolutil.WrapErrWithMessage("projectMirrorGetPublicKey", err)
	}
	if in.ProjectID == "" {
		return PublicKeyOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.MirrorID == 0 {
		return PublicKeyOutput{}, toolutil.ErrFieldRequired("mirror_id")
	}
	pk, _, err := client.GL().ProjectMirrors.GetProjectMirrorPublicKey(string(in.ProjectID), in.MirrorID, gl.WithContext(ctx))
	if err != nil {
		return PublicKeyOutput{}, toolutil.WrapErrWithStatusHint("projectMirrorGetPublicKey", err, http.StatusNotFound,
			"verify mirror_id with gitlab_list_project_mirrors \u2014 SSH public keys are only available for mirrors using SSH authentication")
	}
	return PublicKeyOutput{PublicKey: pk.PublicKey}, nil
}

// Add creates a new remote mirror for a project.
func Add(ctx context.Context, client *gitlabclient.Client, in AddInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectMirrorAdd", err)
	}
	if in.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.URL == "" {
		return Output{}, errors.New("projectMirrorAdd: url is required")
	}
	opts := &gl.AddProjectMirrorOptions{
		URL:                   new(in.URL),
		Enabled:               in.Enabled,
		KeepDivergentRefs:     in.KeepDivergentRefs,
		OnlyProtectedBranches: in.OnlyProtectedBranches,
	}
	if in.MirrorBranchRegex != "" {
		opts.MirrorBranchRegex = new(in.MirrorBranchRegex)
	}
	if in.AuthMethod != "" {
		opts.AuthMethod = new(in.AuthMethod)
	}
	if len(in.HostKeys) > 0 {
		opts.HostKeys = &in.HostKeys
	}
	m, _, err := client.GL().ProjectMirrors.AddProjectMirror(string(in.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorAdd", err,
				"creating push mirrors requires GitLab Premium/Ultimate and Maintainer+ role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorAdd", err,
				"check the mirror URL is well-formed (https:// or ssh://) and includes credentials inline if required \u2014 mirror to the same project is forbidden")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectMirrorAdd", err)
	}
	return toOutput(m), nil
}

// Edit updates an existing remote mirror for a project.
func Edit(ctx context.Context, client *gitlabclient.Client, in EditInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectMirrorEdit", err)
	}
	if in.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.MirrorID == 0 {
		return Output{}, toolutil.ErrFieldRequired("mirror_id")
	}
	opts := &gl.EditProjectMirrorOptions{
		Enabled:               in.Enabled,
		KeepDivergentRefs:     in.KeepDivergentRefs,
		OnlyProtectedBranches: in.OnlyProtectedBranches,
	}
	if in.MirrorBranchRegex != "" {
		opts.MirrorBranchRegex = new(in.MirrorBranchRegex)
	}
	if in.AuthMethod != "" {
		opts.AuthMethod = new(in.AuthMethod)
	}
	if len(in.HostKeys) > 0 {
		opts.HostKeys = &in.HostKeys
	}
	m, _, err := client.GL().ProjectMirrors.EditProjectMirror(string(in.ProjectID), in.MirrorID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorEdit", err,
				"editing push mirrors requires Maintainer+ role on a Premium/Ultimate project")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("projectMirrorEdit", err, http.StatusNotFound,
			hintVerifyMirrorID)
	}
	return toOutput(m), nil
}

// Delete removes a remote mirror from a project.
func Delete(ctx context.Context, client *gitlabclient.Client, in DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage("projectMirrorDelete", err)
	}
	if in.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if in.MirrorID == 0 {
		return toolutil.ErrFieldRequired("mirror_id")
	}
	_, err := client.GL().ProjectMirrors.DeleteProjectMirror(string(in.ProjectID), in.MirrorID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("projectMirrorDelete", err,
				"deleting push mirrors requires Maintainer+ role")
		}
		return toolutil.WrapErrWithStatusHint("projectMirrorDelete", err, http.StatusNotFound,
			hintVerifyMirrorID)
	}
	return nil
}

// ForcePushUpdate triggers an immediate push mirror update.
func ForcePushUpdate(ctx context.Context, client *gitlabclient.Client, in ForcePushInput) error {
	if err := ctx.Err(); err != nil {
		return toolutil.WrapErrWithMessage("projectMirrorForcePush", err)
	}
	if in.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if in.MirrorID == 0 {
		return toolutil.ErrFieldRequired("mirror_id")
	}
	_, err := client.GL().ProjectMirrors.ForcePushMirrorUpdate(string(in.ProjectID), in.MirrorID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("projectMirrorForcePush", err,
				"force-pushing mirrors requires Maintainer+ role; the mirror must be enabled and not in a failed-auth state")
		}
		return toolutil.WrapErrWithStatusHint("projectMirrorForcePush", err, http.StatusNotFound,
			hintVerifyMirrorID)
	}
	return nil
}
