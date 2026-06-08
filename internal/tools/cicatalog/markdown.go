package cicatalog

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of catalog resources as Markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## CI/CD Catalog Resources\n\n")

	if len(out.Resources) == 0 {
		sb.WriteString("No catalog resources found.\n")
		return sb.String()
	}

	sb.WriteString("| Name | Description | Stars | Forks | Issues | MRs | Latest Version | Released |\n")
	sb.WriteString("|------|-------------|-------|-------|--------|-----|----------------|----------|\n")

	for _, r := range out.Resources {
		desc := toolutil.EscapeMdTableCell(truncate(r.Description, 60))
		name := fmt.Sprintf("[%s](%s)", toolutil.EscapeMdTableCell(r.Name), r.WebURL)
		fmt.Fprintf(
			&sb, "| %s | %s | %d | %d | %d | %d | %s | %s |\n",
			name,
			desc,
			r.StarCount,
			r.ForksCount,
			r.OpenIssuesCount,
			r.OpenMRsCount,
			toolutil.EscapeMdTableCell(r.LatestVersionName),
			formatDate(r.LatestReleasedAt),
		)
	}

	sb.WriteString("\n")
	sb.WriteString(toolutil.FormatGraphQLPagination(out.Pagination, len(out.Resources)))
	sb.WriteString("\n")
	return sb.String()
}

// FormatGetMarkdown renders a single catalog resource detail as Markdown.
func FormatGetMarkdown(out GetOutput) string {
	r := out.Resource
	var sb strings.Builder

	fmt.Fprintf(&sb, "## Catalog Resource: %s\n\n", r.Name)
	writeCatalogResourceSummary(&sb, r)
	writeCatalogResourceComponents(&sb, r.Components)
	writeCatalogResourceVersions(&sb, r.Versions)
	return sb.String()
}

func writeCatalogResourceSummary(sb *strings.Builder, r ResourceDetail) {
	sb.WriteString("| Field | Value |\n|-------|-------|\n")
	fmt.Fprintf(sb, "| ID | %s |\n", toolutil.EscapeMdTableCell(r.ID))
	fmt.Fprintf(sb, "| Full Path | %s |\n", toolutil.EscapeMdTableCell(r.FullPath))
	fmt.Fprintf(sb, "| URL | [%s](%s) |\n", toolutil.EscapeMdTableCell(r.FullPath), r.WebURL)
	fmt.Fprintf(sb, "| Stars | %d |\n", r.StarCount)
	fmt.Fprintf(sb, "| Forks | %d |\n", r.ForksCount)
	fmt.Fprintf(sb, "| Open Issues | %d |\n", r.OpenIssuesCount)
	fmt.Fprintf(sb, "| Open MRs | %d |\n", r.OpenMRsCount)
	if r.LatestReleasedAt != "" {
		fmt.Fprintf(sb, "| Latest Release | %s |\n", formatDate(r.LatestReleasedAt))
	}
	if r.LatestVersionName != "" {
		fmt.Fprintf(sb, "| Latest Version | %s |\n", toolutil.EscapeMdTableCell(r.LatestVersionName))
	}
	if r.Description != "" {
		fmt.Fprintf(sb, "\n### Description\n\n%s\n", r.Description)
	}
}

func writeCatalogResourceComponents(sb *strings.Builder, components []ComponentItem) {
	if len(components) == 0 {
		return
	}
	sb.WriteString("\n### Components (Latest Version)\n\n")
	for _, component := range components {
		writeCatalogResourceComponent(sb, component)
	}
}

func writeCatalogResourceComponent(sb *strings.Builder, component ComponentItem) {
	fmt.Fprintf(sb, "#### `%s`\n\n", component.Name)
	if component.Description != "" {
		fmt.Fprintf(sb, "%s\n\n", component.Description)
	}
	fmt.Fprintf(sb, "**Include:** `%s`\n\n", component.IncludePath)
	if len(component.Inputs) == 0 {
		return
	}
	sb.WriteString("| Input | Type | Required | Default | Description |\n")
	sb.WriteString("|-------|------|----------|---------|-------------|\n")
	for _, input := range component.Inputs {
		writeCatalogComponentInput(sb, input)
	}
	sb.WriteString("\n")
}

func writeCatalogComponentInput(sb *strings.Builder, input InputItem) {
	required := "no"
	if input.Required {
		required = "**yes**"
	}
	fmt.Fprintf(
		sb, "| `%s` | %s | %s | %s | %s |\n",
		input.Name,
		toolutil.EscapeMdTableCell(input.Type),
		required,
		toolutil.EscapeMdTableCell(input.Default),
		toolutil.EscapeMdTableCell(input.Description),
	)
}

func writeCatalogResourceVersions(sb *strings.Builder, versions []VersionItem) {
	if len(versions) == 0 {
		return
	}
	sb.WriteString("\n### Released Versions\n\n")
	sb.WriteString("| Version | Released | Components |\n")
	sb.WriteString("|---------|----------|------------|\n")
	for _, version := range versions {
		fmt.Fprintf(
			sb, "| %s | %s | %s |\n",
			toolutil.EscapeMdTableCell(version.Name),
			formatDate(version.ReleasedAt),
			toolutil.EscapeMdTableCell(strings.Join(catalogVersionComponentNames(version), ", ")),
		)
	}
	sb.WriteString("\n")
}

func catalogVersionComponentNames(version VersionItem) []string {
	names := make([]string, 0, len(version.Components))
	for _, component := range version.Components {
		names = append(names, component.Name)
	}
	return names
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatDate extracts the YYYY-MM-DD date portion from an ISO 8601 timestamp.
// Returns an empty string if iso is empty.
func formatDate(iso string) string {
	if iso == "" {
		return ""
	}
	if len(iso) >= 10 {
		return iso[:10]
	}
	return iso
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
}
