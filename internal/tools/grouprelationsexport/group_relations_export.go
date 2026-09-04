package grouprelationsexport

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
		return toolutil.WrapErrWithStatusHint("gitlab_schedule_group_relations_export", err, http.StatusNotFound, "verify group_id with gitlab_group_get")
	}
	return nil
}

// List Export Status.

// ListExportStatusInput represents input for listing group relations export status.
// It mirrors gl.ListGroupRelationsStatusOptions, including the embedded
// gl.ListOptions offset/keyset pagination and ordering controls.
type ListExportStatusInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Relation string               `json:"relation,omitempty" jsonschema:"Filter by relation type (for example labels, milestones, badges)"`
	OrderBy  string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort     string               `json:"sort,omitempty" jsonschema:"Sort order for results: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ExportBatchItem represents a single batch within a relation export.
// It mirrors gl.Batch.
type ExportBatchItem struct {
	Status       int64  `json:"status"`
	BatchNumber  int64  `json:"batch_number"`
	ObjectsCount int64  `json:"objects_count"`
	Error        string `json:"error,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// ExportStatusItem represents a single relation export status entry.
// It mirrors gl.GroupRelationStatus.
type ExportStatusItem struct {
	Relation     string            `json:"relation"`
	Status       int64             `json:"status"`
	Error        string            `json:"error,omitempty"`
	UpdatedAt    string            `json:"updated_at,omitempty"`
	Batched      bool              `json:"batched"`
	BatchesCount int64             `json:"batches_count"`
	Batches      []ExportBatchItem `json:"batches,omitempty"`
}

// ListExportStatusOutput represents the output of listing group relations export status.
type ListExportStatusOutput struct {
	Statuses   []ExportStatusItem        `json:"statuses"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListExportStatus lists the status of group relations exports.
func ListExportStatus(ctx context.Context, client *gitlabclient.Client, input ListExportStatusInput) (*ListExportStatusOutput, error) {
	opts := &gl.ListGroupRelationsStatusOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Relation != "" {
		opts.Relation = new(input.Relation)
	}
	statuses, resp, err := client.GL().GroupRelationsExport.ListExportStatus(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_list_group_relations_export_status", err, http.StatusNotFound, "verify group_id with gitlab_group_get")
	}
	items := make([]ExportStatusItem, 0, len(statuses))
	for _, s := range statuses {
		batches := make([]ExportBatchItem, 0, len(s.Batches))
		for _, b := range s.Batches {
			batches = append(batches, ExportBatchItem{
				Status:       b.Status,
				BatchNumber:  b.BatchNumber,
				ObjectsCount: b.ObjectsCount,
				Error:        b.Error,
				UpdatedAt:    b.UpdatedAt.String(),
			})
		}
		items = append(items, ExportStatusItem{
			Relation:     s.Relation,
			Status:       s.Status,
			Error:        s.Error,
			UpdatedAt:    s.UpdatedAt.String(),
			Batched:      s.Batched,
			BatchesCount: s.BatchesCount,
			Batches:      batches,
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
	toolutil.WriteHints(&sb, "Use the GitLab group relations export download endpoint (`GET /groups/:id/export_relations/download`) to download exported data")
	return sb.String()
}
