package vulnerabilities

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for vulnerability list, triage, and summary actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_vulnerabilities — list project vulnerabilities with optional filters.
		vulnerabilityReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_vulnerabilities"),
		// gitlab_get_vulnerability — fetch a single vulnerability by ID.
		vulnerabilityReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_vulnerability"),
		// gitlab_dismiss_vulnerability — mark a vulnerability as dismissed with a reason.
		vulnerabilityUpdateSpec("dismiss", toolutil.RouteAction(client, Dismiss), "gitlab_dismiss_vulnerability"),
		// gitlab_confirm_vulnerability — mark a vulnerability as confirmed (needs triage).
		vulnerabilityUpdateSpec("confirm", toolutil.RouteAction(client, Confirm), "gitlab_confirm_vulnerability"),
		// gitlab_resolve_vulnerability — mark a vulnerability as resolved.
		vulnerabilityUpdateSpec("resolve", toolutil.RouteAction(client, Resolve), "gitlab_resolve_vulnerability"),
		// gitlab_revert_vulnerability — revert a vulnerability to its prior state (for example from resolved back to detected).
		vulnerabilityUpdateSpec("revert", toolutil.RouteAction(client, Revert), "gitlab_revert_vulnerability"),
		// gitlab_vulnerability_severity_count — return vulnerability counts grouped by severity.
		vulnerabilityReadSpec("severity_count", toolutil.RouteAction(client, SeverityCount), "gitlab_vulnerability_severity_count"),
		// gitlab_pipeline_security_summary — return the security finding summary for a pipeline run.
		vulnerabilityReadSpec("pipeline_security_summary", toolutil.RouteAction(client, PipelineSecuritySummary), "gitlab_pipeline_security_summary"),
	}
}

// vulnerabilityReadSpec builds the canonical read-only spec for a vulnerability tool.
func vulnerabilityReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, vulnerabilityOptions(individualTool))
}

// vulnerabilityUpdateSpec builds the canonical update spec for a vulnerability tool.
func vulnerabilityUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, vulnerabilityOptions(individualTool))
}

func vulnerabilityOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute vulnerabilities domain action.", Tags: []string{"vulnerability", "security"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "vulnerabilities",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
