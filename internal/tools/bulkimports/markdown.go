package bulkimports

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name. Bulk imports are routes on the
// admin catalog group, so their domain is "admin".
const (
	actionGet             = "admin.bulk_import_get"
	actionCancel          = "admin.bulk_import_cancel"
	actionEntityList      = "admin.bulk_import_entity_list"
	actionEntityGet       = "admin.bulk_import_entity_get"
	actionEntityFailures  = "admin.bulk_import_entity_failures"
	labelSourceType       = "Source Type"
	labelSourceURL        = "Source URL"
	labelHasFailures      = "Has Failures"
	hintFailuresDiagnosed = "read the failure diagnostics"
)

// FormatStartMigrationMarkdown renders the migration a start request created,
// as a card.
func FormatStartMigrationMarkdown(out MigrationOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Bulk Import Migration Started")
	c.Int("ID", out.ID)
	c.Field("Status", out.Status)
	c.Field(labelSourceType, out.SourceType)
	c.Link(labelSourceURL, "", out.SourceURL)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	c.Bool(labelHasFailures, out.HasFailures)
	c.End(toolutil.HintAction(actionGet, "watch the migration's progress"))
	return b.String()
}

// FormatListMarkdown renders a page of bulk import migrations as a Markdown
// table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Migrations) == 0 {
		return toolutil.EmptyMessage("bulk import migrations")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Bulk Import Migrations", len(out.Migrations), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Status", labelSourceType, labelSourceURL, labelHasFailures, "Created"))
	for _, m := range out.Migrations {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(m.ID, 10),
			toolutil.EscapeMdTableCell(m.Status),
			toolutil.EscapeMdTableCell(m.SourceType),
			toolutil.MdTitleLink(m.SourceURL, m.SourceURL),
			toolutil.BoolEmoji(m.HasFailures),
			toolutil.FormatTime(m.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionGet, "read one migration in full"),
		toolutil.HintAction(actionEntityList, "inspect the entities of a migration"),
	)
	return b.String()
}

// FormatGetMarkdown renders one bulk import migration as a card.
func FormatGetMarkdown(out MigrationSummary) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Bulk Import Migration #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Status", out.Status)
	c.Field(labelSourceType, out.SourceType)
	c.Link(labelSourceURL, "", out.SourceURL)
	c.Bool(labelHasFailures, out.HasFailures)
	c.Time("Created", out.CreatedAt)
	c.Time("Updated", out.UpdatedAt)
	hints := []string{toolutil.HintAction(actionEntityList, "inspect the entities this migration moved")}
	if out.HasFailures {
		hints = append(hints, toolutil.HintAction(actionEntityFailures, hintFailuresDiagnosed))
	}
	if out.Status == "started" || out.Status == "created" {
		hints = append(hints, toolutil.HintAction(actionCancel, "abort this migration while it runs"))
	}
	c.End(hints...)
	return b.String()
}

// FormatListEntitiesMarkdown renders a page of bulk import entities as a
// Markdown table.
func FormatListEntitiesMarkdown(out ListEntitiesOutput) string {
	if len(out.Entities) == 0 {
		return toolutil.EmptyMessage("bulk import entities")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Bulk Import Entities", len(out.Entities), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Bulk Import", "Type", "Status", "Source", "Destination", "Failures"))
	for _, e := range out.Entities {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			strconv.FormatInt(e.BulkImportID, 10),
			toolutil.EscapeMdTableCell(e.EntityType),
			toolutil.EscapeMdTableCell(e.Status),
			toolutil.EscapeMdTableCell(e.SourceFullPath),
			toolutil.EscapeMdTableCell(e.DestinationFullPath),
			toolutil.BoolEmoji(e.HasFailures),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionEntityGet, "read one entity in full"),
		toolutil.HintAction(actionEntityFailures, hintFailuresDiagnosed),
	)
	return b.String()
}

// FormatGetEntityMarkdown renders one bulk import entity as a card, with the
// per-relation counts as a nested collection.
func FormatGetEntityMarkdown(e EntitySummary) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Bulk Import Entity #%d", e.ID))
	c.Int("ID", e.ID)
	c.Int("Bulk Import ID", e.BulkImportID)
	c.Field("Status", e.Status)
	c.Field("Entity Type", e.EntityType)
	c.Field("Source", e.SourceFullPath)
	c.Field("Destination", e.DestinationFullPath)
	c.Bool("Migrate Projects", e.MigrateProjects)
	c.Bool("Migrate Memberships", e.MigrateMemberships)
	c.Bool(labelHasFailures, e.HasFailures)
	c.Time("Created", e.CreatedAt)
	c.Time("Updated", e.UpdatedAt)
	writeEntityStats(c, e.Stats)
	if e.HasFailures {
		c.End(toolutil.HintAction(actionEntityFailures, hintFailuresDiagnosed))
		return b.String()
	}
	c.End(toolutil.HintAction(actionEntityList, "see the migration's other entities"))
	return b.String()
}

// writeEntityStats writes the per-relation counts, and only for a relation the
// migration reported. GitLab sends the stats object with a key per relation it
// processed, so a relation it left out used to render as a row of three zeros:
// a claim that nothing was imported where the truth is that nothing was said.
// A relation whose three counts are all zero is therefore not written, and a
// stats object with no such relation opens no section at all.
func writeEntityStats(c *toolutil.Card, stats EntityStats) {
	rows := []struct {
		relation string
		item     EntityStatItem
	}{
		{"Labels", stats.Labels},
		{"Milestones", stats.Milestones},
	}
	reported := make([]int, 0, len(rows))
	for i, row := range rows {
		if row.item != (EntityStatItem{}) {
			reported = append(reported, i)
		}
	}
	if len(reported) == 0 {
		return
	}
	t := c.Table("Stats", "Relation", "Source", "Fetched", "Imported")
	for _, i := range reported {
		//gitlab:allow-unescaped rows[i].relation: the relation's name as this formatter spells it, Labels or Milestones, never a value GitLab sent.
		t.Row(
			rows[i].relation,
			strconv.Itoa(rows[i].item.Source),
			strconv.Itoa(rows[i].item.Fetched),
			strconv.Itoa(rows[i].item.Imported),
		)
	}
}

// FormatEntityFailuresMarkdown renders one entity's import failures as a
// Markdown table.
func FormatEntityFailuresMarkdown(out ListEntityFailuresOutput) string {
	if len(out.Failures) == 0 {
		return toolutil.EmptyMessage("bulk import failures")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, fmt.Sprintf("Bulk Import Failures (import #%d, entity #%d)", out.BulkImportID, out.EntityID), len(out.Failures), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("Relation", "Step", "Pipeline", "Class", "Message", "Source", "Created"))
	for _, f := range out.Failures {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(f.Relation),
			toolutil.EscapeMdTableCell(f.Step),
			toolutil.EscapeMdTableCell(f.PipelineClass),
			toolutil.EscapeMdTableCell(f.ExceptionClass),
			toolutil.EscapeMdTableCell(f.ExceptionMessage),
			toolutil.MdTitleLink(f.SourceURL, f.SourceURL),
			toolutil.FormatTime(f.CreatedAt),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, true,
		toolutil.HintAction(actionEntityGet, "read the entity these failures belong to"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatStartMigrationMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatListEntitiesMarkdown)
	toolutil.RegisterMarkdown(FormatGetEntityMarkdown)
	toolutil.RegisterMarkdown(FormatEntityFailuresMarkdown)
}
