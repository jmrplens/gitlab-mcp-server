package securitysettings

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	// The wire form is written by the one helper that converts to UTC first: a
	// layout ending in a literal Z stamps whatever wall clock the value carries
	// with a zone it may not be in, and the card parses what this writes.
	o.CreatedAt = toolutil.RFC3339Ptr(s.CreatedAt)
	o.UpdatedAt = toolutil.RFC3339Ptr(s.UpdatedAt)
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
		return ProjectOutput{}, wrapSecuritySettingsErr("get project security settings", err, securitySettingsHints{
			forbiddenHint: hintSecuritySettingsLicense,
			refusedHint: "reading a project's security settings needs the Developer, Maintainer, Owner or Security Manager role, " +
				"and the project's Security and compliance feature must be enabled",
			notFoundHint: hintVerifyProject,
		})
	}
	return toProjectOutput(settings), nil
}

// hintVerifyProject is the one thing a 404 from a project's security settings
// can mean: GitLab answers the license and the role with 403 and 401, so what
// is left is a project it cannot find.
const hintVerifyProject = "verify project_id with project.get"

// hintSecuritySettingsLicense is the 403 every security settings route
// answers first: the instance license lacks the feature
// (ee/lib/api/project_security_settings.rb:10, group_security_settings.rb:14).
const hintSecuritySettingsLicense = "the GitLab instance's license does not include this security feature (secret push protection needs Ultimate)"

// securitySettingsHints is what one security settings route says for each of
// the three ways GitLab refuses it.
type securitySettingsHints struct {
	// forbiddenHint is for the 403 GitLab answers without an error code: the
	// license, and on an update an archived project or an instance-enforced
	// setting.
	forbiddenHint string
	// refusedHint is for the role refusal, which GitLab answers with 401
	// (entry 55 of docs/development/upstream-bugs.md).
	refusedHint string
	// notFoundHint is for a project or group GitLab cannot find.
	notFoundHint string
}

// wrapSecuritySettingsErr reports a refused security settings call with the
// hint its status earns. The 403 is read first because on these routes it is
// never a role: the role is the 401 (project_security_settings.rb:30 and 53,
// group_security_settings.rb:36), and a 403 is the license or a state of the
// project, which a role hint would send the caller to fix in the wrong place.
// A 403 carrying an error code is the API guard's scope refusal, and a plain
// one naming a blocked, deactivated or otherwise refused account is the guard
// refusing the account itself; neither gets a hint, since no license or role
// is why.
func wrapSecuritySettingsErr(operation string, err error, hints securitySettingsHints) error {
	if toolutil.IsHTTPStatus(err, http.StatusForbidden) && toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint(operation, err, hints.forbiddenHint)
	}
	if toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint(operation, err, hints.refusedHint)
	}
	return toolutil.WrapErrWithStatusHint(operation, err, http.StatusNotFound, hints.notFoundHint)
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
		return ProjectOutput{}, wrapSecuritySettingsErr("update project security settings", err, securitySettingsHints{
			forbiddenHint: hintSecuritySettingsLicense + ", or the project or one of its parent groups is archived, " +
				"or the instance enforces secret push protection so it cannot be turned off",
			refusedHint: "changing a project's security settings needs the Maintainer, Owner or Security Manager role, " +
				"and the project's Security and compliance feature must be enabled",
			notFoundHint: hintVerifyProject,
		})
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
		return GroupOutput{}, wrapSecuritySettingsErr("update group security settings", err, securitySettingsHints{
			forbiddenHint: hintSecuritySettingsLicense,
			refusedHint: "changing a group's security settings needs the Maintainer, Owner or Security Manager role on the group, " +
				"and on GitLab.com the group's plan must include Ultimate",
			notFoundHint: "verify group_id with group.get",
		})
	}
	return toGroupOutput(settings), nil
}
