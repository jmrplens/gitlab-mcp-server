package vulnerabilities

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of vulnerabilities as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Vulnerabilities\n\n")

	if len(out.Vulnerabilities) == 0 {
		sb.WriteString("No vulnerabilities found.\n")
		return sb.String()
	}

	sb.WriteString("| Severity | Title | State | Scanner | Report Type | Detected |\n")
	sb.WriteString("|----------|-------|-------|---------|-------------|----------|\n")

	for _, v := range out.Vulnerabilities {
		scanner := ""
		if v.Scanner != nil {
			scanner = v.Scanner.Name
		}
		primaryID := ""
		if v.PrimaryID != nil {
			primaryID = v.PrimaryID.Name
		}
		title := v.Title
		if primaryID != "" && primaryID != v.Title {
			title = fmt.Sprintf("%s (%s)", v.Title, primaryID)
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
			severityBadge(v.Severity),
			toolutil.EscapeMdTableCell(title),
			toolutil.EscapeMdTableCell(v.State),
			toolutil.EscapeMdTableCell(scanner),
			toolutil.EscapeMdTableCell(v.ReportType),
			formatDate(v.DetectedAt),
		)
	}

	sb.WriteString("\n")
	sb.WriteString(toolutil.FormatGraphQLPagination(out.Pagination, len(out.Vulnerabilities)))
	sb.WriteString("\n")
	return sb.String()
}

// FormatGetMarkdown renders a single vulnerability detail as Markdown.
func FormatGetMarkdown(out GetOutput) string {
	v := out.Vulnerability
	var sb strings.Builder

	fmt.Fprintf(&sb, "## Vulnerability: %s\n\n", v.Title)
	sb.WriteString("| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | %s |\n", toolutil.EscapeMdTableCell(v.ID))
	fmt.Fprintf(&sb, "| Severity | %s |\n", severityBadge(v.Severity))
	fmt.Fprintf(&sb, "| State | %s |\n", toolutil.EscapeMdTableCell(v.State))
	fmt.Fprintf(&sb, "| Report Type | %s |\n", toolutil.EscapeMdTableCell(v.ReportType))

	if v.Scanner != nil {
		scanner := v.Scanner.Name
		if v.Scanner.Vendor != "" {
			scanner += " (" + v.Scanner.Vendor + ")"
		}
		fmt.Fprintf(&sb, "| Scanner | %s |\n", toolutil.EscapeMdTableCell(scanner))
	}

	if v.PrimaryID != nil {
		id := v.PrimaryID.Name
		if v.PrimaryID.URL != "" {
			id = fmt.Sprintf("[%s](%s)", v.PrimaryID.Name, v.PrimaryID.URL)
		}
		fmt.Fprintf(&sb, "| Primary Identifier | %s |\n", id)
	}

	if v.Location != nil {
		loc := v.Location.File
		if v.Location.StartLine > 0 {
			loc += fmt.Sprintf(":%d", v.Location.StartLine)
			if v.Location.EndLine > 0 && v.Location.EndLine != v.Location.StartLine {
				loc += fmt.Sprintf("-%d", v.Location.EndLine)
			}
		}
		fmt.Fprintf(&sb, "| Location | %s |\n", toolutil.EscapeMdTableCell(loc))
	}

	fmt.Fprintf(&sb, "| Detected | %s |\n", formatDate(v.DetectedAt))
	if v.DismissedAt != "" {
		fmt.Fprintf(&sb, "| Dismissed | %s |\n", formatDate(v.DismissedAt))
	}
	if v.ConfirmedAt != "" {
		fmt.Fprintf(&sb, "| Confirmed | %s |\n", formatDate(v.ConfirmedAt))
	}
	if v.ResolvedAt != "" {
		fmt.Fprintf(&sb, "| Resolved | %s |\n", formatDate(v.ResolvedAt))
	}
	if v.DismissalReason != "" {
		fmt.Fprintf(&sb, "| Dismissal Reason | %s |\n", toolutil.EscapeMdTableCell(v.DismissalReason))
	}
	if v.Solution != "" {
		fmt.Fprintf(&sb, "| Solution | %s |\n", toolutil.EscapeMdTableCell(v.Solution))
	}
	fmt.Fprintf(&sb, "| Has Issues | %v |\n", v.HasIssues)
	fmt.Fprintf(&sb, "| Has MR | %v |\n", v.HasMR)

	if v.Project != nil {
		fmt.Fprintf(&sb, "| Project | %s |\n", toolutil.EscapeMdTableCell(v.Project.FullPath))
	}

	if len(v.Identifiers) > 0 {
		sb.WriteString("\n### Identifiers\n\n")
		sb.WriteString("| Name | Type | External ID | URL |\n")
		sb.WriteString("|------|------|-------------|-----|\n")
		for _, id := range v.Identifiers {
			name := toolutil.EscapeMdTableCell(id.Name)
			if id.URL != "" {
				name = fmt.Sprintf("[%s](%s)", toolutil.EscapeMdTableCell(id.Name), id.URL)
			}
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n",
				name,
				toolutil.EscapeMdTableCell(id.ExternalType),
				toolutil.EscapeMdTableCell(id.ExternalID),
				id.URL,
			)
		}
	}

	if v.Description != "" {
		sb.WriteString("\n### Description\n\n")
		sb.WriteString(v.Description)
		sb.WriteString("\n")
	}

	toolutil.WriteHints(&sb,
		"Use `gitlab_dismiss_vulnerability` to dismiss this finding",
		"Use `gitlab_confirm_vulnerability` to confirm this finding",
		"Use `gitlab_resolve_vulnerability` to mark as resolved",
	)
	return sb.String()
}

// FormatMutationMarkdown renders a vulnerability state mutation result as Markdown.
func FormatMutationMarkdown(out MutationOutput, action string) string {
	v := out.Vulnerability
	var sb strings.Builder

	fmt.Fprintf(&sb, "## Vulnerability %s\n\n", action)
	sb.WriteString("| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(&sb, "| ID | %s |\n", toolutil.EscapeMdTableCell(v.ID))
	fmt.Fprintf(&sb, "| Title | %s |\n", toolutil.EscapeMdTableCell(v.Title))
	fmt.Fprintf(&sb, "| Severity | %s |\n", severityBadge(v.Severity))
	fmt.Fprintf(&sb, "| State | %s |\n", toolutil.EscapeMdTableCell(v.State))
	fmt.Fprintf(&sb, "| Report Type | %s |\n", toolutil.EscapeMdTableCell(v.ReportType))

	if v.PrimaryID != nil {
		fmt.Fprintf(&sb, "| Primary ID | %s |\n", toolutil.EscapeMdTableCell(v.PrimaryID.Name))
	}
	if v.DismissalReason != "" {
		fmt.Fprintf(&sb, "| Dismissal Reason | %s |\n", toolutil.EscapeMdTableCell(v.DismissalReason))
	}

	toolutil.WriteHints(&sb,
		"Use `gitlab_get_vulnerability` to view the full vulnerability details",
		"Use `gitlab_list_vulnerabilities` to view all project vulnerabilities",
	)
	return sb.String()
}

// severityBadge returns an emoji + text badge for vulnerability severity.
func severityBadge(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "🔴 CRITICAL"
	case "HIGH":
		return "🟠 HIGH"
	case "MEDIUM":
		return "🟡 MEDIUM"
	case "LOW":
		return "🔵 LOW"
	case "INFO":
		return "ℹ️ INFO"
	default:
		return severity
	}
}

// formatDate trims the time portion from ISO 8601 timestamps for display.
func formatDate(ts string) string {
	if len(ts) > 10 {
		return ts[:10]
	}
	return ts
}

// FormatSeverityCountMarkdown renders vulnerability severity counts as Markdown.
func FormatSeverityCountMarkdown(out SeverityCountOutput) string {
	var sb strings.Builder
	sb.WriteString("## Vulnerability Severity Counts\n\n")
	sb.WriteString("| Severity | Count |\n")
	sb.WriteString("|----------|-------|\n")
	fmt.Fprintf(&sb, "| 🔴 CRITICAL | %d |\n", out.Critical)
	fmt.Fprintf(&sb, "| 🟠 HIGH | %d |\n", out.High)
	fmt.Fprintf(&sb, "| 🟡 MEDIUM | %d |\n", out.Medium)
	fmt.Fprintf(&sb, "| 🔵 LOW | %d |\n", out.Low)
	fmt.Fprintf(&sb, "| ℹ️ INFO | %d |\n", out.Info)
	fmt.Fprintf(&sb, "| ❓ UNKNOWN | %d |\n", out.Unknown)
	fmt.Fprintf(&sb, "| **Total** | **%d** |\n", out.Total)
	toolutil.WriteHints(&sb,
		"Use `gitlab_list_vulnerabilities` to view individual findings",
		"Use `gitlab_pipeline_security_summary` for pipeline-specific scan results",
	)
	return sb.String()
}

// FormatPipelineSecuritySummaryMarkdown renders a pipeline security summary as Markdown.
func FormatPipelineSecuritySummaryMarkdown(out PipelineSecuritySummaryOutput) string {
	var sb strings.Builder
	sb.WriteString("## Pipeline Security Report Summary\n\n")

	if out.TotalVulnerabilities == 0 && out.Sast == nil && out.Dast == nil &&
		out.DependencyScanning == nil && out.ContainerScanning == nil &&
		out.SecretDetection == nil && out.CoverageFuzzing == nil &&
		out.APIFuzzing == nil && out.ClusterImageScanning == nil {
		sb.WriteString("No security scans ran in this pipeline.\n")
		return sb.String()
	}

	sb.WriteString("| Scanner | Vulnerabilities | Scanned Resources |\n")
	sb.WriteString("|---------|----------------:|-------------------:|\n")

	writeScannerRow := func(name string, s *ScannerSummaryItem) {
		if s != nil {
			fmt.Fprintf(&sb, "| %s | %d | %d |\n", name, s.VulnerabilitiesCount, s.ScannedResourcesCount)
		}
	}

	writeScannerRow("SAST", out.Sast)
	writeScannerRow("DAST", out.Dast)
	writeScannerRow("Dependency Scanning", out.DependencyScanning)
	writeScannerRow("Container Scanning", out.ContainerScanning)
	writeScannerRow("Secret Detection", out.SecretDetection)
	writeScannerRow("Coverage Fuzzing", out.CoverageFuzzing)
	writeScannerRow("API Fuzzing", out.APIFuzzing)
	writeScannerRow("Cluster Image Scanning", out.ClusterImageScanning)

	fmt.Fprintf(&sb, "\n**Total Vulnerabilities: %d**\n", out.TotalVulnerabilities)
	toolutil.WriteHints(&sb,
		"Use `gitlab_list_vulnerabilities` to view individual findings",
		"Use `gitlab_vulnerability_severity_count` for severity breakdown",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(func(v MutationOutput) string { return FormatMutationMarkdown(v, "updated") })
	toolutil.RegisterMarkdown(FormatSeverityCountMarkdown)
	toolutil.RegisterMarkdown(FormatPipelineSecuritySummaryMarkdown)
}
