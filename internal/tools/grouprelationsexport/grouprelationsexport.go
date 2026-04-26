// Package grouprelationsexport implements MCP tools for GitLab group relations export operations.
package grouprelationsexport

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Schedule Export.

// ScheduleExportInput represents input for scheduling a group relations export.
type ScheduleExportInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Batched *bool                `json:"batched,omitempty" jsonschema:"Whether to batch the export"`
}

// ScheduleExport schedules a new group relations export.
func ScheduleExport(ctx context.Context, client *gitlabclient.Client, input ScheduleExportInput) error {
	opts := &gl.GroupRelationsScheduleExportOptions{}
	if input.Batched != nil {
		opts.Batched = input.Batched
	}
	_, err := client.GL().GroupRelationsExport.ScheduleExport(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_schedule_group_relations_export", err, http.StatusNotFound, "verify group_id with gitlab_get_group")
	}
	return nil
}

// List Export Status.

// ListExportStatusInput represents input for listing group relations export status.
type ListExportStatusInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Relation string               `json:"relation,omitempty" jsonschema:"Filter by relation type"`
	Page     int64                `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage  int64                `json:"per_page,omitempty" jsonschema:"Number of items per page"`
}

// ExportStatusItem represents a single relation export status entry.
type ExportStatusItem struct {
	Relation     string `json:"relation"`
	Status       int64  `json:"status"`
	Error        string `json:"error,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	Batched      bool   `json:"batched"`
	BatchesCount int64  `json:"batches_count"`
}

// ListExportStatusOutput represents the output of listing group relations export status.
type ListExportStatusOutput struct {
	Statuses   []ExportStatusItem        `json:"statuses"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListExportStatus lists the status of group relations exports.
func ListExportStatus(ctx context.Context, client *gitlabclient.Client, input ListExportStatusInput) (*ListExportStatusOutput, error) {
	opts := &gl.ListGroupRelationsStatusOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	if input.Relation != "" {
		opts.Relation = new(input.Relation)
	}
	statuses, resp, err := client.GL().GroupRelationsExport.ListExportStatus(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_list_group_relations_export_status", err, http.StatusNotFound, "verify group_id with gitlab_get_group")
	}
	items := make([]ExportStatusItem, 0, len(statuses))
	for _, s := range statuses {
		items = append(items, ExportStatusItem{
			Relation:     s.Relation,
			Status:       s.Status,
			Error:        s.Error,
			UpdatedAt:    s.UpdatedAt.String(),
			Batched:      s.Batched,
			BatchesCount: s.BatchesCount,
		})
	}
	pag := toolutil.PaginationFromResponse(resp)
	return &ListExportStatusOutput{
		Statuses:   items,
		Pagination: pag,
	}, nil
}

// Markdown Formatters.

// FormatScheduleExport formats the schedule export result as markdown.
func FormatScheduleExport() string {
	return "Group relations export scheduled successfully."
}

// FormatListExportStatus formats the export status list as markdown.
func FormatListExportStatus(out *ListExportStatusOutput) string {
	if len(out.Statuses) == 0 {
		return "No export statuses found.\n"
	}
	var sb strings.Builder
	sb.WriteString("| Relation | Status | Error | Batched | Batches Count | Updated At |\n")
	sb.WriteString("|---|---|---|---|---|---|\n")
	for _, s := range out.Statuses {
		fmt.Fprintf(&sb, "| %s | %d | %s | %t | %d | %s |\n",
			toolutil.EscapeMdTableCell(s.Relation),
			s.Status,
			toolutil.EscapeMdTableCell(s.Error),
			s.Batched,
			s.BatchesCount,
			toolutil.EscapeMdTableCell(s.UpdatedAt))
	}
	toolutil.WritePagination(&sb, out.Pagination)
	toolutil.WriteHints(&sb, "Use `gitlab_download_group_relations_export` to download exported data")
	return sb.String()
}
