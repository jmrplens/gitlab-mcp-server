package grouprelationsexport

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Schedule Export.

// ScheduleExportInput represents input for scheduling a group relations export.
type ScheduleExportInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"The ID or URL-encoded path of the group,required"`
	Batched *bool                `json:"batched,omitempty" jsonschema:"Whether to batch the export"`
}

// ScheduleExport schedules a new group relations export.
func ScheduleExport(ctx context.Context, client *gitlabclient.Client, input ScheduleExportInput) error {
	if string(input.GroupID) == "" {
		return toolutil.ErrRequiredString("gitlab_schedule_group_relations_export", "group_id")
	}
	opts := &gl.GroupRelationsScheduleExportOptions{}
	if input.Batched != nil {
		opts.Batched = input.Batched
	}
	_, err := client.GL().GroupRelationsExport.ScheduleExport(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_schedule_group_relations_export", err, http.StatusNotFound, "verify group_id with group.get")
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
	Relation          string            `json:"relation"`
	Status            int64             `json:"status"`
	Error             string            `json:"error,omitempty"`
	UpdatedAt         string            `json:"updated_at,omitempty"`
	Batched           bool              `json:"batched"`
	BatchesCount      int64             `json:"batches_count"`
	TotalObjectsCount int64             `json:"total_objects_count"`
	Batches           []ExportBatchItem `json:"batches,omitempty"`
}

// exportStatusExtra is the key of lib/api/entities/bulk_imports/export_status.rb
// that client-go's GroupRelationStatus does not model: how many objects the
// relation's export holds, across every batch. It is exposed with no
// condition, and read from the captured response (ADR-0021). The gap is
// recorded in docs/development/upstream-bugs.md.
type exportStatusExtra struct {
	TotalObjectsCount int64 `json:"total_objects_count"`
}

// capturedExportStatuses reads the extras off the answer to a request for a
// group's export statuses, one per relation in order, the count held to what
// the SDK decoded.
func capturedExportStatuses(capture *gitlabclient.ResponseCapture, decoded int) ([]exportStatusExtra, error) {
	var extras []exportStatusExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d statuses and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}

// ListExportStatusOutput represents the output of listing group relations export status.
type ListExportStatusOutput struct {
	Statuses   []ExportStatusItem        `json:"statuses"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListExportStatus lists the status of group relations exports.
func ListExportStatus(ctx context.Context, client *gitlabclient.Client, input ListExportStatusInput) (*ListExportStatusOutput, error) {
	if string(input.GroupID) == "" {
		return nil, toolutil.ErrRequiredString("gitlab_list_group_relations_export_status", "group_id")
	}
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
		return exportStatusOfRelation(ctx, client, input.GroupID, opts)
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	statuses, resp, err := client.GL().GroupRelationsExport.ListExportStatus(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_list_group_relations_export_status", err, http.StatusNotFound, "verify group_id with group.get")
	}
	extras, err := capturedExportStatuses(captured, len(statuses))
	if err != nil {
		return nil, toolutil.WrapErr("gitlab_list_group_relations_export_status", err)
	}
	items := make([]ExportStatusItem, 0, len(statuses))
	for i, s := range statuses {
		items = append(items, toStatusItem(s, extras[i]))
	}
	pag := toolutil.PaginationFromResponse(resp)
	return &ListExportStatusOutput{
		Statuses:   items,
		Pagination: pag,
	}, nil
}

// relationStatus is one relation's export status as GitLab presents it when
// the request names the relation: the keys client-go models and the one it
// does not, decoded from the one object together.
type relationStatus struct {
	gl.GroupRelationStatus
	exportStatusExtra
}

// exportStatusOfRelation asks for the status of one relation's export. With
// relation set, lib/api/group_export.rb presents that one export as an object
// rather than the array it answers with otherwise, and client-go's
// ListExportStatus decodes every answer into a slice, which refuses an
// object, so the filter failed on every instance. The request is issued here
// with the same client-go options the list sends and decoded into one status.
// The gap is recorded in docs/development/upstream-bugs.md.
func exportStatusOfRelation(ctx context.Context, client *gitlabclient.Client, groupID toolutil.StringOrInt, opts *gl.ListGroupRelationsStatusOptions) (*ListExportStatusOutput, error) {
	status, err := getRelationStatus(ctx, client, "groups/"+gl.PathEscape(string(groupID))+"/export_relations/status", opts)
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_list_group_relations_export_status", err, http.StatusNotFound,
			"GitLab answers 404 when the group has no export of this relation: verify group_id with group.get, "+
				"list every relation's status by leaving relation out, and schedule an export with group.group_relations_schedule")
	}
	return &ListExportStatusOutput{Statuses: []ExportStatusItem{toStatusItem(&status.GroupRelationStatus, status.exportStatusExtra)}}, nil
}

// getRelationStatus issues the GET for one relation's status at path and
// decodes the one object GitLab answers with.
func getRelationStatus(ctx context.Context, client *gitlabclient.Client, path string, opts *gl.ListGroupRelationsStatusOptions) (*relationStatus, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var status relationStatus
	if _, err = client.GL().Do(req, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// toStatusItem converts one relation's export status, with the count the
// capture read beside it.
func toStatusItem(s *gl.GroupRelationStatus, extra exportStatusExtra) ExportStatusItem {
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
	return ExportStatusItem{
		Relation:          s.Relation,
		Status:            s.Status,
		Error:             s.Error,
		UpdatedAt:         s.UpdatedAt.String(),
		Batched:           s.Batched,
		BatchesCount:      s.BatchesCount,
		TotalObjectsCount: extra.TotalObjectsCount,
		Batches:           batches,
	}
}

// The rendering of an export status list lives in markdown.go, which is the
// formatter the registry serves. The second copy that used to sit here was
// registered for no type at all: the handler answers with a pointer and the
// registration named the value, so nothing ever called it, and it printed the
// status code as a number and the batched flag as "true".
