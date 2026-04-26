// mirroring.go implements GitLab project pull mirror operations: get mirror
// configuration, configure pull mirroring, and trigger immediate mirror updates.

package projects

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetPullMirrorInput defines parameters for getting pull mirror details.
type GetPullMirrorInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// PullMirrorOutput holds pull mirror configuration details.
type PullMirrorOutput struct {
	toolutil.HintableOutput
	ID                               int64  `json:"id"`
	Enabled                          bool   `json:"enabled"`
	URL                              string `json:"url,omitempty"`
	UpdateStatus                     string `json:"update_status,omitempty"`
	LastError                        string `json:"last_error,omitempty"`
	LastSuccessfulUpdateAt           string `json:"last_successful_update_at,omitempty"`
	LastUpdateAt                     string `json:"last_update_at,omitempty"`
	LastUpdateStartedAt              string `json:"last_update_started_at,omitempty"`
	MirrorTriggerBuilds              bool   `json:"mirror_trigger_builds"`
	OnlyMirrorProtectedBranches      bool   `json:"only_mirror_protected_branches"`
	MirrorOverwritesDivergedBranches bool   `json:"mirror_overwrites_diverged_branches"`
	MirrorBranchRegex                string `json:"mirror_branch_regex,omitempty"`
}

func pullMirrorToOutput(m *gl.ProjectPullMirrorDetails) PullMirrorOutput {
	out := PullMirrorOutput{
		ID:                               m.ID,
		Enabled:                          m.Enabled,
		URL:                              m.URL,
		UpdateStatus:                     m.UpdateStatus,
		LastError:                        m.LastError,
		MirrorTriggerBuilds:              m.MirrorTriggerBuilds,
		OnlyMirrorProtectedBranches:      m.OnlyMirrorProtectedBranches,
		MirrorOverwritesDivergedBranches: m.MirrorOverwritesDivergedBranches,
		MirrorBranchRegex:                m.MirrorBranchRegex,
	}
	if m.LastSuccessfulUpdateAt != nil {
		out.LastSuccessfulUpdateAt = m.LastSuccessfulUpdateAt.Format(time.RFC3339)
	}
	if m.LastUpdateAt != nil {
		out.LastUpdateAt = m.LastUpdateAt.Format(time.RFC3339)
	}
	if m.LastUpdateStartedAt != nil {
		out.LastUpdateStartedAt = m.LastUpdateStartedAt.Format(time.RFC3339)
	}
	return out
}

// GetPullMirror retrieves pull mirror configuration for a project.
func GetPullMirror(ctx context.Context, client *gitlabclient.Client, input GetPullMirrorInput) (PullMirrorOutput, error) {
	if err := ctx.Err(); err != nil {
		return PullMirrorOutput{}, err
	}
	if input.ProjectID == "" {
		return PullMirrorOutput{}, errors.New("projectGetPullMirror: project_id is required")
	}
	details, _, err := client.GL().Projects.GetProjectPullMirrorDetails(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return PullMirrorOutput{}, toolutil.WrapErrWithStatusHint("projectGetPullMirror", err, http.StatusNotFound, "verify project_id with gitlab_project_get \u2014 pull mirroring requires Premium license")
	}
	return pullMirrorToOutput(details), nil
}

// ConfigurePullMirrorInput defines parameters for configuring pull mirroring.
type ConfigurePullMirrorInput struct {
	ProjectID                        toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Enabled                          *bool                `json:"enabled,omitempty" jsonschema:"Enable or disable pull mirroring"`
	URL                              string               `json:"url,omitempty" jsonschema:"Mirror source URL"`
	AuthUser                         string               `json:"auth_user,omitempty" jsonschema:"Authentication username for the mirror URL"`
	AuthPassword                     string               `json:"auth_password,omitempty" jsonschema:"Authentication password for the mirror URL"`
	MirrorBranchRegex                string               `json:"mirror_branch_regex,omitempty" jsonschema:"Regex to filter branches to mirror"`
	MirrorTriggerBuilds              *bool                `json:"mirror_trigger_builds,omitempty" jsonschema:"Trigger CI builds when mirror updates"`
	OnlyMirrorProtectedBranches      *bool                `json:"only_mirror_protected_branches,omitempty" jsonschema:"Only mirror protected branches"`
	MirrorOverwritesDivergedBranches *bool                `json:"mirror_overwrites_diverged_branches,omitempty" jsonschema:"Overwrite diverged branches on mirror update"`
}

// ConfigurePullMirror sets up or updates pull mirroring for a project.
func ConfigurePullMirror(ctx context.Context, client *gitlabclient.Client, input ConfigurePullMirrorInput) (PullMirrorOutput, error) {
	if err := ctx.Err(); err != nil {
		return PullMirrorOutput{}, err
	}
	if input.ProjectID == "" {
		return PullMirrorOutput{}, errors.New("projectConfigurePullMirror: project_id is required")
	}
	opts := &gl.ConfigureProjectPullMirrorOptions{}
	if input.Enabled != nil {
		opts.Enabled = input.Enabled
	}
	if input.URL != "" {
		opts.URL = &input.URL
	}
	if input.AuthUser != "" {
		opts.AuthUser = &input.AuthUser
	}
	if input.AuthPassword != "" {
		opts.AuthPassword = &input.AuthPassword
	}
	if input.MirrorBranchRegex != "" {
		opts.MirrorBranchRegex = &input.MirrorBranchRegex
	}
	if input.MirrorTriggerBuilds != nil {
		opts.MirrorTriggerBuilds = input.MirrorTriggerBuilds
	}
	if input.OnlyMirrorProtectedBranches != nil {
		opts.OnlyMirrorProtectedBranches = input.OnlyMirrorProtectedBranches
	}
	if input.MirrorOverwritesDivergedBranches != nil {
		opts.MirrorOverwritesDivergedBranches = input.MirrorOverwritesDivergedBranches
	}
	details, _, err := client.GL().Projects.ConfigureProjectPullMirror(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return PullMirrorOutput{}, toolutil.WrapErrWithStatusHint("projectConfigurePullMirror", err, http.StatusBadRequest, "verify project_id and mirror_url are correct \u2014 pull mirroring requires Premium license")
	}
	return pullMirrorToOutput(details), nil
}

// StartMirroringInput defines parameters for triggering a mirror pull.
type StartMirroringInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// StartMirroring triggers an immediate pull mirror update for a project.
func StartMirroring(ctx context.Context, client *gitlabclient.Client, input StartMirroringInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectStartMirroring: project_id is required")
	}
	_, err := client.GL().Projects.StartMirroringProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("projectStartMirroring", err, http.StatusNotFound, "verify project_id \u2014 pull mirroring must be configured first")
	}
	return nil
}
