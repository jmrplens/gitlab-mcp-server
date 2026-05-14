package vulnerabilities

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for vulnerability list, triage, and summary actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		vulnerabilityReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_vulnerabilities"),
		vulnerabilityReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_vulnerability"),
		vulnerabilityUpdateSpec("dismiss", toolutil.RouteAction(client, Dismiss), "gitlab_dismiss_vulnerability"),
		vulnerabilityUpdateSpec("confirm", toolutil.RouteAction(client, Confirm), "gitlab_confirm_vulnerability"),
		vulnerabilityUpdateSpec("resolve", toolutil.RouteAction(client, Resolve), "gitlab_resolve_vulnerability"),
		vulnerabilityUpdateSpec("revert", toolutil.RouteAction(client, Revert), "gitlab_revert_vulnerability"),
		vulnerabilityReadSpec("severity_count", toolutil.RouteAction(client, SeverityCount), "gitlab_vulnerability_severity_count"),
		vulnerabilityReadSpec("pipeline_security_summary", toolutil.RouteAction(client, PipelineSecuritySummary), "gitlab_pipeline_security_summary"),
	}
}

func vulnerabilityReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := vulnerabilityOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func vulnerabilityUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := vulnerabilityOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func vulnerabilityOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"vulnerability", "security"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "vulnerabilities",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
