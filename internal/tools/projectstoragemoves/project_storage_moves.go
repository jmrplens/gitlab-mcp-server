// Package projectstoragemoves implements MCP tools for GitLab project
// repository storage move operations (admin only).
package projectstoragemoves

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds pagination parameters for listing all project storage moves.
type ListInput struct {
	toolutil.PaginationInput
}

// ListForProjectInput holds parameters for listing storage moves for a specific project.
type ListForProjectInput struct {
	ProjectID int64 `json:"project_id" jsonschema:"Numeric ID of the project,required"`
	toolutil.PaginationInput
}

// IDInput holds parameters for getting a single storage move by ID.
type IDInput struct {
	ID int64 `json:"id" jsonschema:"Numeric ID of the storage move,required"`
}

// ProjectMoveInput holds parameters for getting a storage move for a specific project.
type ProjectMoveInput struct {
	ProjectID int64 `json:"project_id" jsonschema:"Numeric ID of the project,required"`
	ID        int64 `json:"id"         jsonschema:"Numeric ID of the storage move,required"`
}

// ScheduleInput holds parameters for scheduling a storage move for a project.
type ScheduleInput struct {
	ProjectID              int64   `json:"project_id"                jsonschema:"Numeric ID of the project,required"`
	DestinationStorageName *string `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// ScheduleAllInput holds parameters for scheduling storage moves for all projects.
type ScheduleAllInput struct {
	SourceStorageName      *string `json:"source_storage_name,omitempty"      jsonschema:"Name of the source storage shard"`
	DestinationStorageName *string `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// Output represents a single project repository storage move.
type Output struct {
	toolutil.HintableOutput
	ID                     int64          `json:"id"`
	CreatedAt              time.Time      `json:"created_at"`
	State                  string         `json:"state"`
	SourceStorageName      string         `json:"source_storage_name"`
	DestinationStorageName string         `json:"destination_storage_name"`
	Project                *ProjectOutput `json:"project,omitempty"`
}

// ProjectOutput represents the project associated with a storage move.
type ProjectOutput struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
}

// ListOutput represents a paginated list of project storage moves.
type ListOutput struct {
	toolutil.HintableOutput
	Moves      []Output                  `json:"moves"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ScheduleAllOutput represents the result of scheduling all project storage moves.
type ScheduleAllOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// RetrieveAll retrieves all project repository storage moves.
func RetrieveAll(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllProjectStorageMovesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	moves, resp, err := client.GL().ProjectRepositoryStorageMove.RetrieveAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve all project storage moves", err, http.StatusForbidden,
			"requires administrator access; self-managed only; storage moves are repository shard migrations between Gitaly nodes")
	}

	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for _, m := range moves {
		out.Moves = append(out.Moves, toOutput(m))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// RetrieveForProject retrieves all storage moves for a specific project.
func RetrieveForProject(ctx context.Context, client *gitlabclient.Client, in ListForProjectInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.ProjectID == 0 {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := gl.RetrieveAllProjectStorageMovesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	moves, resp, err := client.GL().ProjectRepositoryStorageMove.RetrieveAllStorageMovesForProject(in.ProjectID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve project storage moves", err, http.StatusNotFound,
			"requires admin; verify project_id (numeric) exists; only storage moves for the given project are returned")
	}

	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for _, m := range moves {
		out.Moves = append(out.Moves, toOutput(m))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a single project repository storage move by ID.
func Get(ctx context.Context, client *gitlabclient.Client, in IDInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	move, _, err := client.GL().ProjectRepositoryStorageMove.GetStorageMove(in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get project storage move", err, http.StatusNotFound,
			"requires admin; verify id with gitlab_project_storage_move_list; the move record may have been pruned after completion")
	}
	return toOutput(move), nil
}

// GetForProject retrieves a single storage move for a specific project.
func GetForProject(ctx context.Context, client *gitlabclient.Client, in ProjectMoveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ProjectID == 0 {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	move, _, err := client.GL().ProjectRepositoryStorageMove.GetStorageMoveForProject(in.ProjectID, in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get project storage move for project", err, http.StatusNotFound,
			"requires admin; verify project_id + id combination with gitlab_project_storage_move_list_for_project")
	}
	return toOutput(move), nil
}

// Schedule schedules a repository storage move for a project.
func Schedule(ctx context.Context, client *gitlabclient.Client, in ScheduleInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ProjectID == 0 {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := gl.ScheduleStorageMoveForProjectOptions{
		DestinationStorageName: in.DestinationStorageName,
	}
	move, _, err := client.GL().ProjectRepositoryStorageMove.ScheduleStorageMoveForProject(in.ProjectID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("schedule project storage move", err, http.StatusBadRequest,
			"requires admin; destination_storage_name must reference an existing Gitaly storage shard configured on the instance; cannot move to the same shard the project is already on")
	}
	return toOutput(move), nil
}

// ScheduleAll schedules storage moves for all projects on a storage shard.
func ScheduleAll(ctx context.Context, client *gitlabclient.Client, in ScheduleAllInput) (ScheduleAllOutput, error) {
	if err := ctx.Err(); err != nil {
		return ScheduleAllOutput{}, err
	}

	opts := gl.ScheduleAllProjectStorageMovesOptions{
		SourceStorageName:      in.SourceStorageName,
		DestinationStorageName: in.DestinationStorageName,
	}
	_, err := client.GL().ProjectRepositoryStorageMove.ScheduleAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ScheduleAllOutput{}, toolutil.WrapErrWithStatusHint("schedule all project storage moves", err, http.StatusBadRequest,
			"requires admin; source_storage_name and destination_storage_name must reference configured Gitaly shards; bulk operation \u2014 may schedule many concurrent moves")
	}
	return ScheduleAllOutput{Message: "All project repository storage moves have been scheduled"}, nil
}

func toOutput(m *gl.ProjectRepositoryStorageMove) Output {
	o := Output{
		ID:                     m.ID,
		State:                  m.State,
		SourceStorageName:      m.SourceStorageName,
		DestinationStorageName: m.DestinationStorageName,
	}
	if m.CreatedAt != nil {
		o.CreatedAt = *m.CreatedAt
	}
	if m.Project != nil {
		o.Project = &ProjectOutput{
			ID:                m.Project.ID,
			Name:              m.Project.Name,
			PathWithNamespace: m.Project.PathWithNamespace,
		}
	}
	return o
}
