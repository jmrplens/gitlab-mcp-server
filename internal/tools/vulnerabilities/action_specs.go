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
	options := vulnerabilityOptions(individualTool)
	decorateVulnerabilityMeta(&options, individualTool)
	return toolutil.NewReadActionSpec(name, route, options)
}

// vulnerabilityUpdateSpec builds the canonical update spec for a vulnerability tool.
func vulnerabilityUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := vulnerabilityOptions(individualTool)
	decorateVulnerabilityMeta(&options, individualTool)
	return toolutil.NewUpdateActionSpec(name, route, options)
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

// Canonical action IDs for the vulnerability domain (catalog group
// gitlab_vulnerability → base domain "vulnerability") plus the sibling
// security-finding and project domains used for cross-links.
const (
	actionVulnList            = "vulnerability.list"
	actionVulnGet             = "vulnerability.get"
	actionVulnDismiss         = "vulnerability.dismiss"
	actionVulnConfirm         = "vulnerability.confirm"
	actionVulnResolve         = "vulnerability.resolve"
	actionVulnRevert          = "vulnerability.revert"
	actionVulnSeverityCount   = "vulnerability.severity_count"
	actionVulnPipelineSummary = "vulnerability.pipeline_security_summary"
	actionSecurityFindingList = "security_finding.list"
	actionProjectGet          = "project.get"
)

// decorateVulnerabilityMeta fills non-generic Usage, natural-language
// Aliases, RelatedActions, and the "Returns: … See also: …" individual-tool
// description for the vulnerability actions that would otherwise inherit the
// generic placeholder metadata from vulnerabilityOptions. It is a no-op for
// any tool with no entry in vulnerabilityActionMeta.
func decorateVulnerabilityMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := vulnerabilityActionMeta[individualTool]
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

// vulnerabilityActionMetaEntry is the discovery metadata for one
// vulnerability action.
type vulnerabilityActionMetaEntry struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// vulnerabilityActionMeta maps each individual vulnerability tool to its
// discovery metadata. Aliases are natural-language phrases beyond the tool
// name; related actions are canonical {domain}.{action} IDs that cross-link
// sibling vulnerability actions and, where relevant, security_finding.list
// and project.get.
var vulnerabilityActionMeta = map[string]vulnerabilityActionMetaEntry{
	"gitlab_list_vulnerabilities": {
		usage:       "List a project's detected vulnerabilities. Filter by severity, state, scanner, report_type, has_issues, or has_resolution, and page through results when the prompt asks for a security overview or open findings in a known project.",
		aliases:     []string{"list project vulnerabilities", "show security vulnerabilities", "find open vulnerabilities", "list security findings for project"},
		related:     []string{actionVulnGet, actionVulnSeverityCount, actionSecurityFindingList},
		description: "List a project's vulnerabilities with severity, state, scanner, and report-type filters plus keyset pagination. Returns: matching vulnerabilities with title, severity, state, report type, scanner, identifiers, detection time, and web URL. See also: gitlab_get_vulnerability, gitlab_vulnerability_severity_count, gitlab_list_security_findings.",
	},
	"gitlab_get_vulnerability": {
		usage:       "Fetch one vulnerability by its global ID. Use after a list result or when the prompt names a concrete vulnerability ID to read its full detail, identifiers, location, linked issues, and merge request.",
		aliases:     []string{"get vulnerability", "show vulnerability details", "fetch vulnerability by id"},
		related:     []string{actionVulnList, actionVulnDismiss, actionVulnConfirm, actionVulnResolve},
		description: "Get a single vulnerability by global ID. Returns: title, description, severity, state, report type, scanner, identifiers, location, solution, linked issues, and merge request. See also: gitlab_list_vulnerabilities, gitlab_dismiss_vulnerability, gitlab_confirm_vulnerability.",
	},
	"gitlab_dismiss_vulnerability": {
		usage:       "Dismiss a vulnerability with a reason (for example a false positive or accepted risk). Use during triage to mark a finding as not actionable.",
		aliases:     []string{"dismiss vulnerability", "mark vulnerability false positive", "ignore vulnerability"},
		related:     []string{actionVulnGet, actionVulnConfirm, actionVulnResolve, actionVulnRevert},
		description: "Dismiss a vulnerability with a dismissal reason. Returns: the updated vulnerability with its new dismissed state and timestamp. See also: gitlab_get_vulnerability, gitlab_confirm_vulnerability, gitlab_revert_vulnerability.",
	},
	"gitlab_confirm_vulnerability": {
		usage:       "Confirm a vulnerability as a genuine finding that needs remediation. Use during triage to move a detected vulnerability to the confirmed state.",
		aliases:     []string{"confirm vulnerability", "mark vulnerability confirmed", "accept vulnerability as real"},
		related:     []string{actionVulnGet, actionVulnDismiss, actionVulnResolve, actionVulnRevert},
		description: "Confirm a vulnerability as a real finding. Returns: the updated vulnerability with its new confirmed state and timestamp. See also: gitlab_get_vulnerability, gitlab_dismiss_vulnerability, gitlab_resolve_vulnerability.",
	},
	"gitlab_resolve_vulnerability": {
		usage:       "Resolve a vulnerability once it has been fixed. Use after remediation to mark the finding as resolved.",
		aliases:     []string{"resolve vulnerability", "mark vulnerability fixed", "close vulnerability"},
		related:     []string{actionVulnGet, actionVulnConfirm, actionVulnRevert, actionVulnDismiss},
		description: "Resolve a vulnerability after it is fixed. Returns: the updated vulnerability with its new resolved state and timestamp. See also: gitlab_get_vulnerability, gitlab_confirm_vulnerability, gitlab_revert_vulnerability.",
	},
	"gitlab_revert_vulnerability": {
		usage:       "Revert a vulnerability to its prior detected state, undoing a previous dismiss, confirm, or resolve. Use to re-open a finding for triage.",
		aliases:     []string{"revert vulnerability", "reopen vulnerability", "undo vulnerability triage"},
		related:     []string{actionVulnGet, actionVulnDismiss, actionVulnConfirm, actionVulnResolve},
		description: "Revert a vulnerability back to the detected state. Returns: the updated vulnerability with its restored detected state. See also: gitlab_get_vulnerability, gitlab_dismiss_vulnerability, gitlab_resolve_vulnerability.",
	},
	"gitlab_vulnerability_severity_count": {
		usage:       "Return a project's vulnerability counts grouped by severity. Use for a quick risk summary before listing individual findings.",
		aliases:     []string{"vulnerability counts by severity", "security severity breakdown", "count vulnerabilities by severity"},
		related:     []string{actionVulnList, actionVulnPipelineSummary, actionProjectGet},
		description: "Count a project's vulnerabilities grouped by severity. Returns: critical, high, medium, low, info, and unknown counts. See also: gitlab_list_vulnerabilities, gitlab_pipeline_security_summary.",
	},
	"gitlab_pipeline_security_summary": {
		usage:       "Return the security report summary for one pipeline run, broken down by scan type (SAST, DAST, and others). Use to review the security results produced by a specific pipeline.",
		aliases:     []string{"pipeline security summary", "security report for pipeline", "pipeline scan results"},
		related:     []string{actionVulnSeverityCount, actionVulnList, actionProjectGet},
		description: "Summarize a pipeline's security scan results. Returns: per-scanner vulnerability and scanned-resource counts for the pipeline (SAST, DAST, and other report types). See also: gitlab_vulnerability_severity_count, gitlab_list_vulnerabilities.",
	},
}
