package projectmirrors

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintVerifyMirrorID is the 404 hint shared by project mirror tools.
const hintVerifyMirrorID = "verify mirror_id with project.mirror_list"

// hintMirrorPermission is the hint every remote mirror route gives a refused
// caller. GitLab guards all seven with one check, the Maintainer role on the
// project while an administrator has not turned mirroring off for everyone
// else, and answers it with 401 rather than 403 (lib/api/remote_mirrors.rb:12).
// It names the tier because the hints it replaces said push mirrors needed
// Premium, which they do not.
const hintMirrorPermission = "managing push mirrors needs the Maintainer role on the project, and an instance administrator can turn mirroring off for everyone else, which GitLab refuses the same way; push mirrors are available on every tier, Free included"

// hintForcePushDisabled is the hint for the one 400 the sync route answers a
// caller who passed its check: GitLab considers the mirror disabled
// (RemoteMirrors::SyncService). That is not the enabled flag alone.
// RemoteMirror#enabled is also false when mirroring is unavailable for the
// project (turned off on the instance with no override for it), when the
// project has no repository or is pending deletion, and while the instance is
// in Silent Mode, and an administrator passes the route's check in the first
// case as a Maintainer does in the last, so the hint names every cause rather
// than telling a caller whose flag is already on to turn it on.
const hintForcePushDisabled = "GitLab considers the mirror disabled: either its enabled flag is off (turn it on with project.mirror_edit, enabled=true), or mirroring is unavailable for this project (turned off on the instance, the instance in Silent Mode, or the project without a repository or pending deletion); read its last_error with project.mirror_get"

var credentialedURLPattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)([^\s/@]+@)`)

// ListInput holds parameters for listing project mirrors.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by (e.g. id). Only applies when pagination='keyset'."`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction for keyset-paginated results: asc or desc. Only applies when pagination='keyset'."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	URL                   string               `json:"url"                               jsonschema:"Remote push mirror URL (e.g. https://user:token@example.com/repo.git). URL-embedded credentials are supported only for push mirrors. Treat them as secrets, do not log or store them, and redact tokens or passwords in telemetry/errors,required"`
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
		URL:                   redactMirrorURL(m.URL),
		UpdateStatus:          m.UpdateStatus,
		LastError:             redactCredentialsInText(m.LastError),
		OnlyProtectedBranches: m.OnlyProtectedBranches,
		KeepDivergentRefs:     m.KeepDivergentRefs,
		MirrorBranchRegex:     m.MirrorBranchRegex,
		AuthMethod:            m.AuthMethod,
	}
	o.LastSuccessfulUpdateAt = toolutil.RFC3339Ptr(m.LastSuccessfulUpdateAt)
	o.LastUpdateAt = toolutil.RFC3339Ptr(m.LastUpdateAt)
	o.LastUpdateStartedAt = toolutil.RFC3339Ptr(m.LastUpdateStartedAt)
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
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput)
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	mirrors, resp, err := client.GL().ProjectMirrors.ListProjectMirror(string(in.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsPermissionRefusal(err) {
			return ListOutput{}, toolutil.WrapErrWithHint("projectMirrorList", err, hintMirrorPermission)
		}
		return ListOutput{}, toolutil.WrapErrWithStatusHint("projectMirrorList", err, http.StatusNotFound,
			"verify the project exists with project.get")
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
		if toolutil.IsPermissionRefusal(err) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorGet", err, hintMirrorPermission)
		}
		return Output{}, toolutil.WrapErrWithStatusHint("projectMirrorGet", err, http.StatusNotFound, hintVerifyMirrorID)
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
		if toolutil.IsPermissionRefusal(err) {
			return PublicKeyOutput{}, toolutil.WrapErrWithHint("projectMirrorGetPublicKey", err, hintMirrorPermission)
		}
		return PublicKeyOutput{}, toolutil.WrapErrWithStatusHint("projectMirrorGetPublicKey", err, http.StatusNotFound,
			"verify mirror_id with project.mirror_list. SSH public keys are only available for mirrors using SSH authentication")
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
		// Read on err, not on its redaction: redactMirrorError rebuilds the
		// error from its text when it strips a credential, which drops the
		// response the refusal is read from.
		if toolutil.IsPermissionRefusal(err) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorAdd", redactMirrorError(err), hintMirrorPermission)
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorAdd", redactMirrorError(err),
				"check the mirror URL is well-formed (https:// or ssh://), includes credentials inline if required, and does not mirror to the same project")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectMirrorAdd", redactMirrorError(err))
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
		if toolutil.IsPermissionRefusal(err) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorEdit", redactMirrorError(err), hintMirrorPermission)
		}
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("projectMirrorEdit", redactMirrorError(err), hintVerifyMirrorID)
		}
		return Output{}, toolutil.WrapErrWithMessage("projectMirrorEdit", redactMirrorError(err))
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
		if toolutil.IsPermissionRefusal(err) {
			return toolutil.WrapErrWithHint("projectMirrorDelete", err, hintMirrorPermission)
		}
		return toolutil.WrapErrWithStatusHint("projectMirrorDelete", err, http.StatusNotFound,
			hintVerifyMirrorID)
	}
	return nil
}

func redactMirrorURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		if credentialedURLPattern.MatchString(rawURL) {
			return redactCredentialsInText(rawURL)
		}
		return rawURL
	}
	if parsed.User == nil {
		return rawURL
	}
	parsed.User = url.User("redacted")
	return parsed.String()
}

func redactMirrorError(err error) error {
	if err == nil {
		return nil
	}
	message := redactCredentialsInText(err.Error())
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}

func redactCredentialsInText(text string) string {
	return credentialedURLPattern.ReplaceAllString(text, `${1}[redacted]@`)
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
		if toolutil.IsPermissionRefusal(err) {
			return toolutil.WrapErrWithHint("projectMirrorForcePush", err, hintMirrorPermission)
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return toolutil.WrapErrWithHint("projectMirrorForcePush", err, hintForcePushDisabled)
		}
		return toolutil.WrapErrWithStatusHint("projectMirrorForcePush", err, http.StatusNotFound,
			hintVerifyMirrorID)
	}
	return nil
}
