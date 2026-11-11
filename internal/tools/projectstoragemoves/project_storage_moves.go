// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package projectstoragemoves

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The operations the project-scoped handlers name in what they report: the
// resolution of the project, a refusal and a body that does not decode.
const (
	opRetrieveForProject = "retrieve project storage moves"
	opGetForProject      = "get project storage move for project"
	opSchedule           = "schedule project storage move"
)

// ListInput holds pagination parameters for listing all project storage moves.
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. id)."`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc."`
	toolutil.PaginationInput
}

// ListForProjectInput holds parameters for listing storage moves for a specific project.
type ListForProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by (e.g. id)."`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction: asc or desc."`
	toolutil.PaginationInput
}

// IDInput holds parameters for getting a single storage move by ID.
type IDInput struct {
	ID int64 `json:"id" jsonschema:"Numeric ID of the storage move,required"`
}

// ProjectMoveInput holds parameters for getting a storage move for a specific project.
type ProjectMoveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	ID        int64                `json:"id"         jsonschema:"Numeric ID of the storage move,required"`
}

// ScheduleInput holds parameters for scheduling a storage move for a project.
type ScheduleInput struct {
	ProjectID              toolutil.StringOrInt `json:"project_id"                jsonschema:"Project ID or URL-encoded path,required"`
	DestinationStorageName *string              `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// ScheduleAllInput holds parameters for scheduling storage moves for all projects.
type ScheduleAllInput struct {
	SourceStorageName      *string `json:"source_storage_name,omitempty"      jsonschema:"Name of the source storage shard,required"`
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
	ErrorMessage           string         `json:"error_message,omitempty"`
	Project                *ProjectOutput `json:"project,omitempty"`
}

// ProjectOutput represents the project associated with a storage move.
// It mirrors gitlab.RepositoryProject 1:1.
type ProjectOutput struct {
	ID                int64      `json:"id"`
	Description       string     `json:"description,omitempty"`
	Name              string     `json:"name"`
	NameWithNamespace string     `json:"name_with_namespace,omitempty"`
	Path              string     `json:"path,omitempty"`
	PathWithNamespace string     `json:"path_with_namespace"`
	CreatedAt         *time.Time `json:"created_at,omitempty"`
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

// unresolvedError says a project's path names nothing GitLab could find, on the
// read that looks it up. It is a type of its own because the same 404 from the
// storage move route after it means something else: the project is there, and
// the move or the permission is not.
type unresolvedError struct {
	path string
	// cause is GitLab's 404.
	cause error
}

// Error names what was looked up.
func (e *unresolvedError) Error() string {
	return fmt.Sprintf("no project with the path %q was found", e.path)
}

// Unwrap hands back GitLab's answer.
func (e *unresolvedError) Unwrap() error { return e.cause }

// resolveProjectID answers the integer client-go's storage move methods take.
// GitLab's routes take the ID of the project or its URL-encoded path, and
// client-go types the parameter as an int64 (docs/development/upstream-bugs.md,
// row 158), so a value that is not a number is read into its ID first. A number
// sends nothing, and one no project can have is refused here as a missing one.
func resolveProjectID(ctx context.Context, client *gitlabclient.Client, operation string, value toolutil.StringOrInt) (int64, error) {
	id, err := value.Int64()
	if err == nil && id > 0 {
		return id, nil
	}
	if err == nil || value == "" {
		return 0, toolutil.ErrFieldRequired("project_id")
	}
	//gitlab:request optional: a path is read into its ID only when the caller names the project by it, since client-go's storage move methods take an integer
	project, _, err := client.GL().Projects.GetProject(string(value), nil, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return 0, &unresolvedError{path: string(value), cause: err}
		}
		return 0, toolutil.WrapErrWithMessage(operation, err)
	}
	return project.ID, nil
}

// RetrieveAll retrieves all project repository storage moves.
func RetrieveAll(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllProjectStorageMovesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, toolutil.KeysetPaginationInput{})
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
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
	projectID, err := resolveProjectID(ctx, client, opRetrieveForProject, in.ProjectID)
	if err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllProjectStorageMovesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, toolutil.KeysetPaginationInput{})
	if in.OrderBy != "" {
		opts.OrderBy = in.OrderBy
	}
	if in.Sort != "" {
		opts.Sort = in.Sort
	}
	moves, resp, err := client.GL().ProjectRepositoryStorageMove.RetrieveAllStorageMovesForProject(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint(opRetrieveForProject, err, http.StatusNotFound,
			"requires admin; verify project_id exists; only storage moves for the given project are returned")
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
			"requires admin; verify id with storage_move.retrieve_all_project; the move record may have been pruned after completion")
	}
	return toOutput(move), nil
}

// GetForProject retrieves a single storage move for a specific project.
func GetForProject(ctx context.Context, client *gitlabclient.Client, in ProjectMoveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}
	projectID, err := resolveProjectID(ctx, client, opGetForProject, in.ProjectID)
	if err != nil {
		return Output{}, err
	}

	move, _, err := client.GL().ProjectRepositoryStorageMove.GetStorageMoveForProject(projectID, in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(opGetForProject, err, http.StatusNotFound,
			"requires admin; verify the project_id + id pair with storage_move.retrieve_project, which lists this project's own moves; a move id belonging to another project answers 404 here, and the record may have been pruned after completion")
	}
	return toOutput(move), nil
}

// Schedule schedules a repository storage move for a project.
func Schedule(ctx context.Context, client *gitlabclient.Client, in ScheduleInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	projectID, err := resolveProjectID(ctx, client, opSchedule, in.ProjectID)
	if err != nil {
		return Output{}, err
	}

	opts := gl.ScheduleStorageMoveForProjectOptions{
		DestinationStorageName: in.DestinationStorageName,
	}
	move, _, err := client.GL().ProjectRepositoryStorageMove.ScheduleStorageMoveForProject(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint(opSchedule, err, http.StatusBadRequest,
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
			"requires admin; source_storage_name and destination_storage_name must reference configured Gitaly shards; bulk operation. May schedule many concurrent moves")
	}
	return ScheduleAllOutput{Message: "All project repository storage moves have been scheduled"}, nil
}

func toOutput(m *gl.ProjectRepositoryStorageMove) Output {
	o := Output{
		ID:                     m.ID,
		State:                  m.State,
		SourceStorageName:      m.SourceStorageName,
		DestinationStorageName: m.DestinationStorageName,
		ErrorMessage:           m.ErrorMessage,
	}
	if m.CreatedAt != nil {
		o.CreatedAt = *m.CreatedAt
	}
	if m.Project != nil {
		o.Project = &ProjectOutput{
			ID:                m.Project.ID,
			Description:       m.Project.Description,
			Name:              m.Project.Name,
			NameWithNamespace: m.Project.NameWithNamespace,
			Path:              m.Project.Path,
			PathWithNamespace: m.Project.PathWithNamespace,
			CreatedAt:         m.Project.CreatedAt,
		}
	}
	return o
}
