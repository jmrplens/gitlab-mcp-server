// markdown.go provides Markdown formatting functions for bulk import migration MCP tool output.

package bulkimports

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatStartMigrationMarkdown formats a start migration result as markdown.
func FormatStartMigrationMarkdown(out MigrationOutput) string {
	var sb strings.Builder
	sb.WriteString("## Bulk Import Migration Started\n\n")
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Status | %s |\n", out.Status)
	fmt.Fprintf(&sb, "| Source Type | %s |\n", out.SourceType)
	fmt.Fprintf(&sb, "| Source URL | %s |\n", toolutil.EscapeMdTableCell(out.SourceURL))
	fmt.Fprintf(&sb, "| Created At | %s |\n", toolutil.FormatTime(out.CreatedAt))
	fmt.Fprintf(&sb, "| Updated At | %s |\n", toolutil.FormatTime(out.UpdatedAt))
	fmt.Fprintf(&sb, "| Has Failures | %v |\n", out.HasFailures)
	toolutil.WriteHints(&sb, "Monitor migration progress with gitlab_bulk_import_get")
	return sb.String()
}

// FormatListMarkdown formats a list of bulk import migrations as markdown.
func FormatListMarkdown(out ListOutput) string {
	var sb strings.Builder
	sb.WriteString("## Bulk Import Migrations\n\n")
	toolutil.WriteListSummary(&sb, len(out.Migrations), out.Pagination)
	if len(out.Migrations) == 0 {
		sb.WriteString("_No migrations found._\n")
		return sb.String()
	}
	sb.WriteString("| ID | Status | Source Type | Source URL | Has Failures | Created |\n|---|---|---|---|---|---|\n")
	for _, m := range out.Migrations {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s | %v | %s |\n",
			m.ID, m.Status, m.SourceType,
			toolutil.EscapeMdTableCell(m.SourceURL),
			m.HasFailures, toolutil.FormatTime(m.CreatedAt))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		"Use gitlab_bulk_import_get with id for full details",
		"Use gitlab_bulk_import_entity_list to inspect entities of a migration",
	)
	return sb.String()
}

// FormatGetMarkdown formats a single bulk import migration as markdown.
func FormatGetMarkdown(out MigrationSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Bulk Import Migration #%d\n\n", out.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&sb, "| Status | %s |\n", out.Status)
	fmt.Fprintf(&sb, "| Source Type | %s |\n", out.SourceType)
	fmt.Fprintf(&sb, "| Source URL | %s |\n", toolutil.EscapeMdTableCell(out.SourceURL))
	fmt.Fprintf(&sb, "| Has Failures | %v |\n", out.HasFailures)
	fmt.Fprintf(&sb, "| Created At | %s |\n", toolutil.FormatTime(out.CreatedAt))
	fmt.Fprintf(&sb, "| Updated At | %s |\n", toolutil.FormatTime(out.UpdatedAt))
	hints := []string{"Use gitlab_bulk_import_entity_list with bulk_import_id to inspect entities"}
	if out.HasFailures {
		hints = append(hints, "Failures detected — use gitlab_bulk_import_entity_failures for diagnostics")
	}
	if out.Status == "started" || out.Status == "created" {
		hints = append(hints, "Use gitlab_bulk_import_cancel to abort an in-progress migration")
	}
	toolutil.WriteHints(&sb, hints...)
	return sb.String()
}

// FormatListEntitiesMarkdown formats a list of bulk import entities as markdown.
func FormatListEntitiesMarkdown(out ListEntitiesOutput) string {
	var sb strings.Builder
	sb.WriteString("## Bulk Import Entities\n\n")
	toolutil.WriteListSummary(&sb, len(out.Entities), out.Pagination)
	if len(out.Entities) == 0 {
		sb.WriteString("_No entities found._\n")
		return sb.String()
	}
	sb.WriteString("| ID | Bulk Import | Type | Status | Source | Destination | Failures |\n|---|---|---|---|---|---|---|\n")
	for _, e := range out.Entities {
		fmt.Fprintf(&sb, "| %d | %d | %s | %s | %s | %s | %v |\n",
			e.ID, e.BulkImportID, e.EntityType, e.Status,
			toolutil.EscapeMdTableCell(e.SourceFullPath),
			toolutil.EscapeMdTableCell(e.DestinationFullPath),
			e.HasFailures)
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		"Use gitlab_bulk_import_entity_get for full details on a single entity",
		"Use gitlab_bulk_import_entity_failures to inspect failure diagnostics",
	)
	return sb.String()
}

// FormatGetEntityMarkdown formats a single bulk import entity as markdown.
func FormatGetEntityMarkdown(e EntitySummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Bulk Import Entity #%d\n\n", e.ID)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", e.ID)
	fmt.Fprintf(&sb, "| Bulk Import ID | %d |\n", e.BulkImportID)
	fmt.Fprintf(&sb, "| Status | %s |\n", e.Status)
	fmt.Fprintf(&sb, "| Entity Type | %s |\n", e.EntityType)
	fmt.Fprintf(&sb, "| Source | %s |\n", toolutil.EscapeMdTableCell(e.SourceFullPath))
	fmt.Fprintf(&sb, "| Destination | %s |\n", toolutil.EscapeMdTableCell(e.DestinationFullPath))
	fmt.Fprintf(&sb, "| Migrate Projects | %v |\n", e.MigrateProjects)
	fmt.Fprintf(&sb, "| Migrate Memberships | %v |\n", e.MigrateMemberships)
	fmt.Fprintf(&sb, "| Has Failures | %v |\n", e.HasFailures)
	fmt.Fprintf(&sb, "| Created At | %s |\n", toolutil.FormatTime(e.CreatedAt))
	fmt.Fprintf(&sb, "| Updated At | %s |\n", toolutil.FormatTime(e.UpdatedAt))
	sb.WriteString("\n### Stats\n\n")
	sb.WriteString("| Relation | Source | Fetched | Imported |\n|---|---|---|---|\n")
	fmt.Fprintf(&sb, "| Labels | %d | %d | %d |\n", e.Stats.LabelsSource, e.Stats.LabelsFetched, e.Stats.LabelsImported)
	fmt.Fprintf(&sb, "| Milestones | %d | %d | %d |\n", e.Stats.MilestonesSource, e.Stats.MilestonesFetched, e.Stats.MilestonesImported)
	if e.HasFailures {
		toolutil.WriteHints(&sb, "Failures detected — use gitlab_bulk_import_entity_failures for diagnostics")
	}
	return sb.String()
}

// FormatEntityFailuresMarkdown formats migration entity failures as markdown.
func FormatEntityFailuresMarkdown(out ListEntityFailuresOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Bulk Import Failures (import #%d, entity #%d)\n\n", out.BulkImportID, out.EntityID)
	if len(out.Failures) == 0 {
		sb.WriteString("_No failures recorded._\n")
		return sb.String()
	}
	sb.WriteString("| Relation | Step | Pipeline | Class | Message | Source | Created |\n|---|---|---|---|---|---|---|\n")
	for _, f := range out.Failures {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s | %s |\n",
			f.Relation, f.Step, f.PipelineClass, f.ExceptionClass,
			toolutil.EscapeMdTableCell(f.ExceptionMessage),
			toolutil.EscapeMdTableCell(f.SourceURL),
			toolutil.FormatTime(f.CreatedAt))
	}
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		"Inspect exception_class and pipeline_class to triage import errors",
	)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatStartMigrationMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatListEntitiesMarkdown)
	toolutil.RegisterMarkdown(FormatGetEntityMarkdown)
	toolutil.RegisterMarkdown(FormatEntityFailuresMarkdown)
}
