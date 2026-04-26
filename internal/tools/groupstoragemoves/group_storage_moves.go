// Package groupstoragemoves implements MCP tools for GitLab group
// repository storage move operations (admin only).
package groupstoragemoves

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds pagination parameters for listing all group storage moves.
type ListInput struct {
	toolutil.PaginationInput
}

// ListForGroupInput holds parameters for listing storage moves for a specific group.
type ListForGroupInput struct {
	GroupID int64 `json:"group_id" jsonschema:"Numeric ID of the group,required"`
	toolutil.PaginationInput
}

// IDInput holds parameters for getting a single storage move by ID.
type IDInput struct {
	ID int64 `json:"id" jsonschema:"Numeric ID of the storage move,required"`
}

// GroupMoveInput holds parameters for getting a storage move for a specific group.
type GroupMoveInput struct {
	GroupID int64 `json:"group_id" jsonschema:"Numeric ID of the group,required"`
	ID      int64 `json:"id"       jsonschema:"Numeric ID of the storage move,required"`
}

// ScheduleInput holds parameters for scheduling a storage move for a group.
type ScheduleInput struct {
	GroupID                int64   `json:"group_id"                  jsonschema:"Numeric ID of the group,required"`
	DestinationStorageName *string `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// ScheduleAllInput holds parameters for scheduling storage moves for all groups.
type ScheduleAllInput struct {
	SourceStorageName      *string `json:"source_storage_name,omitempty"      jsonschema:"Name of the source storage shard"`
	DestinationStorageName *string `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// Output represents a single group repository storage move.
type Output struct {
	toolutil.HintableOutput
	ID                     int64        `json:"id"`
	CreatedAt              time.Time    `json:"created_at"`
	State                  string       `json:"state"`
	SourceStorageName      string       `json:"source_storage_name"`
	DestinationStorageName string       `json:"destination_storage_name"`
	Group                  *GroupOutput `json:"group,omitempty"`
}

// GroupOutput represents the group associated with a storage move.
type GroupOutput struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"web_url,omitempty"`
}

// ListOutput represents a paginated list of group storage moves.
type ListOutput struct {
	toolutil.HintableOutput
	Moves      []Output                  `json:"moves"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ScheduleAllOutput represents the result of scheduling all group storage moves.
type ScheduleAllOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// RetrieveAll retrieves all group repository storage moves.
func RetrieveAll(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllGroupStorageMovesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	moves, resp, err := client.GL().GroupRepositoryStorageMove.RetrieveAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve all group storage moves", err, http.StatusForbidden,
			"requires administrator access + Premium/Ultimate; self-managed only; group wiki storage moves between Gitaly nodes")
	}

	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for _, m := range moves {
		out.Moves = append(out.Moves, toOutput(m))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// RetrieveForGroup retrieves all storage moves for a specific group.
func RetrieveForGroup(ctx context.Context, client *gitlabclient.Client, in ListForGroupInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID == 0 {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}

	opts := gl.RetrieveAllGroupStorageMovesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	moves, resp, err := client.GL().GroupRepositoryStorageMove.RetrieveAllStorageMovesForGroup(in.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve group storage moves", err, http.StatusNotFound,
			"requires admin + Premium/Ultimate; verify group_id (numeric) exists; only storage moves for the given group are returned")
	}

	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for _, m := range moves {
		out.Moves = append(out.Moves, toOutput(m))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a single group repository storage move by ID.
func Get(ctx context.Context, client *gitlabclient.Client, in IDInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	move, _, err := client.GL().GroupRepositoryStorageMove.GetStorageMove(in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get group storage move", err, http.StatusNotFound,
			"requires admin + Premium/Ultimate; verify id with gitlab_retrieve_all_group_storage_moves")
	}
	return toOutput(move), nil
}

// GetForGroup retrieves a single storage move for a specific group.
func GetForGroup(ctx context.Context, client *gitlabclient.Client, in GroupMoveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.GroupID == 0 {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	move, _, err := client.GL().GroupRepositoryStorageMove.GetStorageMoveForGroup(in.GroupID, in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get group storage move for group", err, http.StatusNotFound,
			"requires admin + Premium/Ultimate; verify group_id + id combination with gitlab_group_storage_move_list_for_group")
	}
	return toOutput(move), nil
}

// Schedule schedules a repository storage move for a group.
func Schedule(ctx context.Context, client *gitlabclient.Client, in ScheduleInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.GroupID == 0 {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}

	opts := gl.ScheduleStorageMoveForGroupOptions{
		DestinationStorageName: in.DestinationStorageName,
	}
	move, _, err := client.GL().GroupRepositoryStorageMove.ScheduleStorageMoveForGroup(in.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("schedule group storage move", err, http.StatusBadRequest,
			"requires admin + Premium/Ultimate; destination_storage_name must reference an existing Gitaly shard; cannot move to the same shard")
	}
	return toOutput(move), nil
}

// ScheduleAll schedules storage moves for all groups on a storage shard.
func ScheduleAll(ctx context.Context, client *gitlabclient.Client, in ScheduleAllInput) (ScheduleAllOutput, error) {
	if err := ctx.Err(); err != nil {
		return ScheduleAllOutput{}, err
	}

	opts := gl.ScheduleAllGroupStorageMovesOptions{
		SourceStorageName:      in.SourceStorageName,
		DestinationStorageName: in.DestinationStorageName,
	}
	_, err := client.GL().GroupRepositoryStorageMove.ScheduleAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ScheduleAllOutput{}, toolutil.WrapErrWithStatusHint("schedule all group storage moves", err, http.StatusBadRequest,
			"requires admin + Premium/Ultimate; source_storage_name and destination_storage_name must reference configured Gitaly shards; bulk operation \u2014 may schedule many concurrent moves")
	}
	return ScheduleAllOutput{Message: "All group repository storage moves have been scheduled"}, nil
}

func toOutput(m *gl.GroupRepositoryStorageMove) Output {
	o := Output{
		ID:                     m.ID,
		State:                  m.State,
		SourceStorageName:      m.SourceStorageName,
		DestinationStorageName: m.DestinationStorageName,
	}
	if m.CreatedAt != nil {
		o.CreatedAt = *m.CreatedAt
	}
	if m.Group != nil {
		o.Group = &GroupOutput{
			ID:     m.Group.ID,
			Name:   m.Group.Name,
			WebURL: m.Group.WebURL,
		}
	}
	return o
}
