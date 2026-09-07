package securityfindings

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action IDs for the security-finding domain (catalog group
// gitlab_security_finding → base domain "security_finding") plus the sibling
// vulnerability and project domains used for cross-links.
const (
	actionSecurityFindingList = "security_finding.list"
	actionVulnList            = "vulnerability.list"
	actionVulnPipelineSummary = "vulnerability.pipeline_security_summary"
	actionVulnSeverityCount   = "vulnerability.severity_count"
	actionProjectGet          = "project.get"
)

// ActionSpecs returns canonical specs for security finding actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_security_findings — list a pipeline's security report findings.
		toolutil.NewReadActionSpec("list", toolutil.RouteAction(client, List), toolutil.ActionSpecOptions{
			Aliases: []string{
				"gitlab_list_security_findings",
				"list pipeline security findings",
				"show security scan findings",
				"list SAST and DAST findings",
				"find security report findings for pipeline",
			},
			Usage:          "List the security report findings produced by one pipeline run. Filter by severity, scanner, state, or report_type (SAST, DAST, dependency scanning, container scanning, secret detection), sort by severity, and page through results when the prompt asks for a pipeline's scan findings or raw security report output. Use gitlab_pipeline_security_summary first for an aggregate count, then this action for individual findings.",
			Tags:           []string{"security", "finding"},
			RelatedActions: []string{actionVulnList, actionVulnPipelineSummary, actionVulnSeverityCount, actionProjectGet},
			OpenWorld:      true,
			Edition:        "premium",
			OwnerPackage:   "securityfindings",
			// PipelineSecurityReportFindingSort has these two values. Without
			// the override the canonical REST "sort" enum (asc, desc) is
			// injected, and the server itself would then refuse both of them.
			InputSchemaOverrides: []toolutil.InputSchemaOverride{
				toolutil.SchemaEnumOverride("sort", "severity_desc", "severity_asc"),
			},
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        "gitlab_list_security_findings",
				Title:       toolutil.TitleFromName("gitlab_list_security_findings"),
				Description: "List a pipeline's security report findings with severity, scanner, report-type and state filters, severity sorting and keyset pagination. Returns: matching findings with UUID, title, severity, report type, scanner, identifiers (CVE, CWE, OWASP), code location, state, evidence, and linked vulnerability state. See also: gitlab_list_vulnerabilities, gitlab_pipeline_security_summary, gitlab_vulnerability_severity_count.",
			},
		}),
	}
}
