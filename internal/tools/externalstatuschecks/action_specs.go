package externalstatuschecks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for external status check actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		externalStatusCheckReadSpec("list_project_checks", toolutil.RouteAction(client, ListProjectStatusChecks), "gitlab_list_project_status_checks"),
		externalStatusCheckReadSpec("list_project_mr_checks", toolutil.RouteAction(client, ListProjectMRExternalStatusChecks), "gitlab_list_project_mr_external_status_checks"),
		externalStatusCheckReadSpec("list_project", toolutil.RouteAction(client, ListProjectExternalStatusChecks), "gitlab_list_project_external_status_checks"),
		externalStatusCheckCreateSpec("create_project", toolutil.RouteAction(client, CreateProjectExternalStatusCheck), "gitlab_create_project_external_status_check"),
		externalStatusCheckDeleteSpec("delete_project", toolutil.DestructiveVoidAction(client, DeleteProjectExternalStatusCheck), "gitlab_delete_project_external_status_check"),
		externalStatusCheckUpdateSpec("update_project", toolutil.RouteAction(client, UpdateProjectExternalStatusCheck), "gitlab_update_project_external_status_check"),
		externalStatusCheckUpdateSpec("retry_project", toolutil.RouteVoidAction(client, RetryFailedExternalStatusCheckForProjectMR), "gitlab_retry_failed_external_status_check_for_project_mr"),
		externalStatusCheckUpdateSpec("set_project_mr_status", toolutil.RouteVoidAction(client, SetProjectMRExternalStatusCheckStatus), "gitlab_set_project_mr_external_status_check_status"),
	}
}

func externalStatusCheckReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := externalStatusCheckOptions(individualTool)
	decorateExternalStatusCheckMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

func externalStatusCheckCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := externalStatusCheckOptions(individualTool)
	decorateExternalStatusCheckMeta(&options, individualTool)
	return toolutil.NewCreateActionSpec(name, route, options)
}

func externalStatusCheckUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := externalStatusCheckOptions(individualTool)
	decorateExternalStatusCheckMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func externalStatusCheckDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := externalStatusCheckOptions(individualTool)
	decorateExternalStatusCheckMeta(&options, individualTool)
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func externalStatusCheckOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute externalstatuschecks domain action.", Tags: []string{"external_status_check", "status_check"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "externalstatuschecks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// decorateExternalStatusCheckMeta fills non-generic Usage, natural-language
// Aliases, RelatedActions, and the "Returns: … See also: …" individual-tool
// description for each external status check action, replacing the generic
// placeholder metadata from externalStatusCheckOptions (R-META, 1:1 audit).
func decorateExternalStatusCheckMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := externalStatusCheckActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string(nil), meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
	if meta.description != "" {
		options.IndividualTool.Description = meta.description
	}
}

// externalStatusCheckActionMetaEntry is the discovery metadata for one
// external status check action.
type externalStatusCheckActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

const (
	actionListProject   = "external_status_check.list_project"
	actionCreateProject = "external_status_check.create_project"
	actionUpdateProject = "external_status_check.update_project"
	actionDeleteProject = "external_status_check.delete_project"
	actionListProjectMR = "external_status_check.list_project_mr_checks"
)

// externalStatusCheckActionMeta maps each individual external status check tool
// to its discovery metadata.
var externalStatusCheckActionMeta = map[string]externalStatusCheckActionMetaEntry{
	"gitlab_list_project_status_checks": {
		usage:       "List a project's external status check services (deprecated path). Prefer gitlab_list_project_external_status_checks. Use this only when matching the legacy endpoint. Requires Maintainer role and a Premium/Ultimate license.",
		aliases:     []string{"list project status checks", "show project external status checks", "external status check services"},
		related:     []string{actionListProject, actionCreateProject, actionListProjectMR},
		description: "List a project's external status check services (deprecated endpoint). Returns: each check with id, name, external_url, hmac, protected branches, and pagination metadata. See also: gitlab_list_project_external_status_checks, gitlab_create_project_external_status_check, gitlab_list_project_mr_external_status_checks.",
	},
	"gitlab_list_project_external_status_checks": {
		usage:       "List the external status check services configured for a project, including their HMAC setting and protected-branch scope. Requires Maintainer role and a Premium/Ultimate license. Supports order_by, sort, and keyset pagination.",
		aliases:     []string{"list project external status checks", "show project status check services", "project status checks"},
		related:     []string{actionCreateProject, actionUpdateProject, actionDeleteProject, actionListProjectMR},
		description: "List a project's external status check services. Returns: each check with id, name, external_url, hmac, protected branches, and pagination metadata. See also: gitlab_create_project_external_status_check, gitlab_update_project_external_status_check, gitlab_delete_project_external_status_check, gitlab_list_project_mr_external_status_checks.",
	},
	"gitlab_list_project_mr_external_status_checks": {
		usage:       "List the external status checks that apply to a specific merge request along with each check's current status. Use merge_request_iid (project-scoped). Requires Maintainer role and a Premium/Ultimate license. Supports order_by, sort, and keyset pagination.",
		aliases:     []string{"list merge request status checks", "show mr external status checks", "status checks for a merge request"},
		related:     []string{actionListProject, "external_status_check.set_project_mr_status", "external_status_check.retry_project"},
		description: "List the external status checks for a merge request and their status. Returns: each check with id, name, external_url, status, and pagination metadata. See also: gitlab_list_project_external_status_checks, gitlab_set_project_mr_external_status_check_status, gitlab_retry_failed_external_status_check_for_project_mr.",
	},
	"gitlab_create_project_external_status_check": {
		usage:       "Create an external status check service for a project, optionally scoping it to protected branches and supplying a shared secret for HMAC verification. Requires Maintainer role and a Premium/Ultimate license.",
		aliases:     []string{"create external status check", "add project status check", "register external status check service"},
		related:     []string{actionListProject, actionUpdateProject, actionDeleteProject},
		description: "Create an external status check service for a project. Returns: the created check with id, name, external_url, hmac, and protected branches. See also: gitlab_list_project_external_status_checks, gitlab_update_project_external_status_check, gitlab_delete_project_external_status_check.",
	},
	"gitlab_update_project_external_status_check": {
		usage:       "Update a project external status check service: change its name, external_url, shared secret, or protected-branch scope. Requires Maintainer role and a Premium/Ultimate license.",
		aliases:     []string{"update external status check", "edit project status check", "modify external status check service"},
		related:     []string{actionListProject, actionCreateProject, actionDeleteProject},
		description: "Update a project external status check service. Returns: the updated check with id, name, external_url, hmac, and protected branches. See also: gitlab_list_project_external_status_checks, gitlab_create_project_external_status_check, gitlab_delete_project_external_status_check.",
	},
	"gitlab_delete_project_external_status_check": {
		usage:       "Permanently delete a project external status check service. Destructive and irreversible. Confirm project_id and check_id before calling. Requires Maintainer role and a Premium/Ultimate license.",
		aliases:     []string{"delete external status check", "remove project status check", "remove external status check service"},
		related:     []string{actionListProject, actionCreateProject, actionUpdateProject},
		description: "Delete a project external status check service permanently. Returns: a success confirmation. See also: gitlab_list_project_external_status_checks, gitlab_create_project_external_status_check, gitlab_update_project_external_status_check.",
	},
	"gitlab_retry_failed_external_status_check_for_project_mr": {
		usage:       "Retry a failed external status check for a merge request so the external service is re-invoked. The check must currently be in the failed state. Requires Maintainer role and a Premium/Ultimate license.",
		aliases:     []string{"retry failed status check", "re-run merge request status check", "retry external status check"},
		related:     []string{actionListProjectMR, "external_status_check.set_project_mr_status"},
		description: "Retry a failed external status check for a merge request. Returns: a success confirmation. See also: gitlab_list_project_mr_external_status_checks, gitlab_set_project_mr_external_status_check_status.",
	},
	"gitlab_set_project_mr_external_status_check_status": {
		usage:       "Set the status (passed or failed) of an external status check for a merge request, identified by sha and external_status_check_id. Typically called by the HMAC-authenticated external service that owns the check.",
		aliases:     []string{"set merge request status check status", "report external status check result", "pass or fail status check"},
		related:     []string{actionListProjectMR, "external_status_check.retry_project"},
		description: "Set the status of an external status check for a merge request. Returns: a success confirmation. See also: gitlab_list_project_mr_external_status_checks, gitlab_retry_failed_external_status_check_for_project_mr.",
	},
}
