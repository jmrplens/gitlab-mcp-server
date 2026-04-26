// Package securitysettings implements GitLab project and group security settings
// operations including get and update for secret push protection.
package securitysettings

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GetProjectInput holds parameters for getting project security settings.
type GetProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// UpdateProjectInput holds parameters for updating project secret push protection.
type UpdateProjectInput struct {
	ProjectID                   toolutil.StringOrInt `json:"project_id"                    jsonschema:"Project ID or URL-encoded path,required"`
	SecretPushProtectionEnabled bool                 `json:"secret_push_protection_enabled" jsonschema:"Enable or disable secret push protection,required"`
}

// UpdateGroupInput holds parameters for updating group secret push protection.
type UpdateGroupInput struct {
	GroupID                     toolutil.StringOrInt `json:"group_id"                      jsonschema:"Group ID or URL-encoded path,required"`
	SecretPushProtectionEnabled bool                 `json:"secret_push_protection_enabled" jsonschema:"Enable or disable secret push protection,required"`
	ProjectsToExclude           []int64              `json:"projects_to_exclude,omitempty"  jsonschema:"Project IDs to exclude from group-level protection"`
}

// ProjectOutput represents project security settings.
type ProjectOutput struct {
	toolutil.HintableOutput
	ProjectID                           int64  `json:"project_id"`
	CreatedAt                           string `json:"created_at,omitempty"`
	UpdatedAt                           string `json:"updated_at,omitempty"`
	AutoFixContainerScanning            bool   `json:"auto_fix_container_scanning"`
	AutoFixDAST                         bool   `json:"auto_fix_dast"`
	AutoFixDependencyScanning           bool   `json:"auto_fix_dependency_scanning"`
	AutoFixSAST                         bool   `json:"auto_fix_sast"`
	ContinuousVulnerabilityScansEnabled bool   `json:"continuous_vulnerability_scans_enabled"`
	ContainerScanningForRegistryEnabled bool   `json:"container_scanning_for_registry_enabled"`
	SecretPushProtectionEnabled         bool   `json:"secret_push_protection_enabled"`
}

// GroupOutput represents group security settings.
type GroupOutput struct {
	toolutil.HintableOutput
	SecretPushProtectionEnabled bool     `json:"secret_push_protection_enabled"`
	Errors                      []string `json:"errors,omitempty"`
}

func toProjectOutput(s *gl.ProjectSecuritySettings) ProjectOutput {
	if s == nil {
		return ProjectOutput{}
	}
	o := ProjectOutput{
		ProjectID:                           s.ProjectID,
		AutoFixContainerScanning:            s.AutoFixContainerScanning,
		AutoFixDAST:                         s.AutoFixDAST,
		AutoFixDependencyScanning:           s.AutoFixDependencyScanning,
		AutoFixSAST:                         s.AutoFixSAST,
		ContinuousVulnerabilityScansEnabled: s.ContinuousVulnerabilityScansEnabled,
		ContainerScanningForRegistryEnabled: s.ContainerScanningForRegistryEnabled,
		SecretPushProtectionEnabled:         s.SecretPushProtectionEnabled,
	}
	if s.CreatedAt != nil {
		o.CreatedAt = s.CreatedAt.Format("2006-01-02T15:04:05Z")
	}
	if s.UpdatedAt != nil {
		o.UpdatedAt = s.UpdatedAt.Format("2006-01-02T15:04:05Z")
	}
	return o
}

func toGroupOutput(s *gl.GroupSecuritySettings) GroupOutput {
	if s == nil {
		return GroupOutput{}
	}
	return GroupOutput{
		SecretPushProtectionEnabled: s.SecretPushProtectionEnabled,
		Errors:                      s.Errors,
	}
}

// GetProject returns the security settings for a project.
func GetProject(ctx context.Context, client *gitlabclient.Client, in GetProjectInput) (ProjectOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProjectOutput{}, err
	}
	if in.ProjectID.String() == "" {
		return ProjectOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	settings, _, err := client.GL().ProjectSecuritySettings.ListProjectSecuritySettings(in.ProjectID.String())
	if err != nil {
		return ProjectOutput{}, toolutil.WrapErrWithStatusHint("get project security settings", err, http.StatusNotFound, "verify project_id with gitlab_project_get \u2014 requires Ultimate license")
	}
	return toProjectOutput(settings), nil
}

// UpdateProject updates the secret push protection setting for a project.
func UpdateProject(ctx context.Context, client *gitlabclient.Client, in UpdateProjectInput) (ProjectOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProjectOutput{}, err
	}
	if in.ProjectID.String() == "" {
		return ProjectOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	// SDK uses value type (not pointer) for this options struct
	opts := gl.UpdateProjectSecuritySettingsOptions{
		SecretPushProtectionEnabled: new(in.SecretPushProtectionEnabled),
	}
	settings, _, err := client.GL().ProjectSecuritySettings.UpdateSecretPushProtectionEnabledSetting(in.ProjectID.String(), opts)
	if err != nil {
		return ProjectOutput{}, toolutil.WrapErrWithStatusHint("update project security settings", err, http.StatusNotFound, "verify project_id with gitlab_project_get \u2014 requires Maintainer role and Ultimate license")
	}
	return toProjectOutput(settings), nil
}

// UpdateGroup updates the secret push protection setting for a group.
func UpdateGroup(ctx context.Context, client *gitlabclient.Client, in UpdateGroupInput) (GroupOutput, error) {
	if err := ctx.Err(); err != nil {
		return GroupOutput{}, err
	}
	if in.GroupID.String() == "" {
		return GroupOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	// SDK uses value type (not pointer) for this options struct
	opts := gl.UpdateGroupSecuritySettingsOptions{
		SecretPushProtectionEnabled: new(in.SecretPushProtectionEnabled),
	}
	if len(in.ProjectsToExclude) > 0 {
		opts.ProjectsToExclude = new(in.ProjectsToExclude)
	}
	settings, _, err := client.GL().GroupSecuritySettings.UpdateSecretPushProtectionEnabledSetting(in.GroupID.String(), opts)
	if err != nil {
		return GroupOutput{}, toolutil.WrapErrWithStatusHint("update group security settings", err, http.StatusNotFound, "verify group_id with gitlab_group_get \u2014 requires Owner role and Ultimate license")
	}
	return toGroupOutput(settings), nil
}
