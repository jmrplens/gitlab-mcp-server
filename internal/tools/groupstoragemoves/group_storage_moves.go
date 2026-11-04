// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupstoragemoves

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The operations the group-scoped handlers name in what they report: the
// resolution of the group, a refusal and a body that does not decode.
const (
	opRetrieveForGroup = "retrieve group storage moves"
	opGetForGroup      = "get group storage move for group"
	opSchedule         = "schedule group storage move"
)

// ListInput holds pagination parameters for listing all group storage moves.
type ListInput struct {
	toolutil.PaginationInput
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. id)."`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc."`
}

// ListForGroupInput holds parameters for listing storage moves for a specific group.
type ListForGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. id)."`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc."`
}

// IDInput holds parameters for getting a single storage move by ID.
type IDInput struct {
	ID int64 `json:"id" jsonschema:"Numeric ID of the storage move,required"`
}

// GroupMoveInput holds parameters for getting a storage move for a specific group.
type GroupMoveInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	ID      int64                `json:"id"       jsonschema:"Numeric ID of the storage move,required"`
}

// ScheduleInput holds parameters for scheduling a storage move for a group.
type ScheduleInput struct {
	GroupID                toolutil.StringOrInt `json:"group_id"                  jsonschema:"Group ID or URL-encoded path,required"`
	DestinationStorageName *string              `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// ScheduleAllInput holds parameters for scheduling storage moves for all groups.
type ScheduleAllInput struct {
	SourceStorageName      *string `json:"source_storage_name,omitempty"      jsonschema:"Name of the source storage shard,required"`
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
	ErrorMessage           string       `json:"error_message,omitempty"`
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

// unresolvedError says a group's path names nothing GitLab could find, on the
// read that looks it up. It is a type of its own because the same 404 from the
// storage move route after it means something else: the group is there, and the
// move or the permission is not.
type unresolvedError struct {
	path string
	// cause is GitLab's 404.
	cause error
}

// Error names what was looked up.
func (e *unresolvedError) Error() string {
	return fmt.Sprintf("no group with the path %q was found", e.path)
}

// Unwrap hands back GitLab's answer.
func (e *unresolvedError) Unwrap() error { return e.cause }

// resolveGroupID answers the integer client-go's storage move methods take.
// GitLab's routes take the ID of the group or its URL-encoded path, and client-go
// types the parameter as an int64 (docs/development/upstream-bugs.md, row 158),
// so a value that is not a number is read into its ID first. A number sends
// nothing, and one no group can have is refused here as a missing one.
func resolveGroupID(ctx context.Context, client *gitlabclient.Client, operation string, value toolutil.StringOrInt) (int64, error) {
	id, err := value.Int64()
	if err == nil && id > 0 {
		return id, nil
	}
	if err == nil || value == "" {
		return 0, toolutil.ErrFieldRequired("group_id")
	}
	//gitlab:request optional: a path is read into its ID only when the caller names the group by it, since client-go's storage move methods take an integer
	group, _, err := client.GL().Groups.GetGroup(string(value), &gl.GetGroupOptions{
		WithProjects: new(false), //nolint:staticcheck // SA1019: the ID is all this read needs, and without it GitLab answers with up to 100 projects as well
	}, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return 0, &unresolvedError{path: string(value), cause: err}
		}
		return 0, toolutil.WrapErrWithMessage(operation, err)
	}
	return group.ID, nil
}

// RetrieveAll retrieves all group repository storage moves.
func RetrieveAll(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllGroupStorageMovesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, toolutil.KeysetPaginationInput{})
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	moves, resp, err := client.GL().GroupRepositoryStorageMove.RetrieveAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve all group storage moves", err, http.StatusForbidden,
			"requires administrator access + Premium/Ultimate; self-managed only; group wiki storage moves between Gitaly nodes")
	}

	extras, err := toolutil.CapturedStorageMoves(captured, len(moves))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("retrieve all group storage moves", err)
	}
	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for i, m := range moves {
		out.Moves = append(out.Moves, toOutput(m, extras[i]))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// RetrieveForGroup retrieves all storage moves for a specific group.
func RetrieveForGroup(ctx context.Context, client *gitlabclient.Client, in ListForGroupInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	groupID, err := resolveGroupID(ctx, client, opRetrieveForGroup, in.GroupID)
	if err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllGroupStorageMovesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, toolutil.KeysetPaginationInput{})
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	moves, resp, err := client.GL().GroupRepositoryStorageMove.RetrieveAllStorageMovesForGroup(groupID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(opRetrieveForGroup, err, http.StatusNotFound,
			"requires admin + Premium/Ultimate; verify group_id exists; only storage moves for the given group are returned")
	}

	extras, err := toolutil.CapturedStorageMoves(captured, len(moves))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr(opRetrieveForGroup, err)
	}
	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for i, m := range moves {
		out.Moves = append(out.Moves, toOutput(m, extras[i]))
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

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	move, _, err := client.GL().GroupRepositoryStorageMove.GetStorageMove(in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get group storage move", err, http.StatusNotFound,
			"requires admin + Premium/Ultimate; verify id with storage_move.retrieve_all_group")
	}
	extra, err := toolutil.CapturedStorageMove(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("get group storage move", err)
	}
	return toOutput(move, extra), nil
}

// GetForGroup retrieves a single storage move for a specific group.
func GetForGroup(ctx context.Context, client *gitlabclient.Client, in GroupMoveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}
	groupID, err := resolveGroupID(ctx, client, opGetForGroup, in.GroupID)
	if err != nil {
		return Output{}, err
	}

	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	move, _, err := client.GL().GroupRepositoryStorageMove.GetStorageMoveForGroup(groupID, in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(opGetForGroup, err, http.StatusNotFound,
			"requires admin + Premium/Ultimate; verify the group_id + id pair with storage_move.retrieve_group, which lists this group's own moves; a move id belonging to another group answers 404 here, and the record may have been pruned after completion")
	}
	extra, err := toolutil.CapturedStorageMove(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr(opGetForGroup, err)
	}
	return toOutput(move, extra), nil
}

// Schedule schedules a repository storage move for a group.
func Schedule(ctx context.Context, client *gitlabclient.Client, in ScheduleInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	groupID, err := resolveGroupID(ctx, client, opSchedule, in.GroupID)
	if err != nil {
		return Output{}, err
	}

	opts := gl.ScheduleStorageMoveForGroupOptions{
		DestinationStorageName: in.DestinationStorageName,
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	move, _, err := client.GL().GroupRepositoryStorageMove.ScheduleStorageMoveForGroup(groupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(opSchedule, err, http.StatusBadRequest,
			"requires admin + Premium/Ultimate; destination_storage_name must reference an existing Gitaly shard; cannot move to the same shard")
	}
	extra, err := toolutil.CapturedStorageMove(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr(opSchedule, err)
	}
	return toOutput(move, extra), nil
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
			"requires admin + Premium/Ultimate; source_storage_name and destination_storage_name must reference configured Gitaly shards; bulk operation. May schedule many concurrent moves")
	}
	return ScheduleAllOutput{Message: "All group repository storage moves have been scheduled"}, nil
}

func toOutput(m *gl.GroupRepositoryStorageMove, extra toolutil.StorageMoveExtra) Output {
	o := Output{
		ID:                     m.ID,
		State:                  m.State,
		SourceStorageName:      m.SourceStorageName,
		DestinationStorageName: m.DestinationStorageName,
		ErrorMessage:           extra.ErrorMessage,
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
