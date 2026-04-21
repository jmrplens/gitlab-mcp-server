// Package snippetstoragemoves implements MCP tools for GitLab snippet
// repository storage move operations (admin only).
package snippetstoragemoves

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds pagination parameters for listing all snippet storage moves.
type ListInput struct {
	toolutil.PaginationInput
}

// ListForSnippetInput holds parameters for listing storage moves for a specific snippet.
type ListForSnippetInput struct {
	SnippetID int64 `json:"snippet_id" jsonschema:"Numeric ID of the snippet,required"`
	toolutil.PaginationInput
}

// IDInput holds parameters for getting a single storage move by ID.
type IDInput struct {
	ID int64 `json:"id" jsonschema:"Numeric ID of the storage move,required"`
}

// SnippetMoveInput holds parameters for getting a storage move for a specific snippet.
type SnippetMoveInput struct {
	SnippetID int64 `json:"snippet_id" jsonschema:"Numeric ID of the snippet,required"`
	ID        int64 `json:"id"         jsonschema:"Numeric ID of the storage move,required"`
}

// ScheduleInput holds parameters for scheduling a storage move for a snippet.
type ScheduleInput struct {
	SnippetID              int64   `json:"snippet_id"                jsonschema:"Numeric ID of the snippet,required"`
	DestinationStorageName *string `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// ScheduleAllInput holds parameters for scheduling storage moves for all snippets.
type ScheduleAllInput struct {
	SourceStorageName      *string `json:"source_storage_name,omitempty"      jsonschema:"Name of the source storage shard"`
	DestinationStorageName *string `json:"destination_storage_name,omitempty" jsonschema:"Name of the destination storage shard"`
}

// Output represents a single snippet repository storage move.
type Output struct {
	toolutil.HintableOutput
	ID                     int64          `json:"id"`
	CreatedAt              time.Time      `json:"created_at"`
	State                  string         `json:"state"`
	SourceStorageName      string         `json:"source_storage_name"`
	DestinationStorageName string         `json:"destination_storage_name"`
	Snippet                *SnippetOutput `json:"snippet,omitempty"`
}

// SnippetOutput represents the snippet associated with a storage move.
type SnippetOutput struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	WebURL string `json:"web_url,omitempty"`
}

// ListOutput represents a paginated list of snippet storage moves.
type ListOutput struct {
	toolutil.HintableOutput
	Moves      []Output                  `json:"moves"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ScheduleAllOutput represents the result of scheduling all snippet storage moves.
type ScheduleAllOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// RetrieveAll retrieves all snippet repository storage moves.
func RetrieveAll(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}

	opts := gl.RetrieveAllSnippetStorageMovesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	moves, resp, err := client.GL().SnippetRepositoryStorageMove.RetrieveAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("retrieve all snippet storage moves", err)
	}

	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for _, m := range moves {
		out.Moves = append(out.Moves, toOutput(m))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// RetrieveForSnippet retrieves all storage moves for a specific snippet.
func RetrieveForSnippet(ctx context.Context, client *gitlabclient.Client, in ListForSnippetInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.SnippetID == 0 {
		return ListOutput{}, toolutil.ErrFieldRequired("snippet_id")
	}

	opts := gl.RetrieveAllSnippetStorageMovesOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(in.Page),
			PerPage: int64(in.PerPage),
		},
	}
	moves, resp, err := client.GL().SnippetRepositoryStorageMove.RetrieveAllStorageMovesForSnippet(in.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("retrieve snippet storage moves", err)
	}

	out := ListOutput{Moves: make([]Output, 0, len(moves))}
	for _, m := range moves {
		out.Moves = append(out.Moves, toOutput(m))
	}
	out.Pagination = toolutil.PaginationFromResponse(resp)
	return out, nil
}

// Get retrieves a single snippet repository storage move by ID.
func Get(ctx context.Context, client *gitlabclient.Client, in IDInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	move, _, err := client.GL().SnippetRepositoryStorageMove.GetStorageMove(in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get snippet storage move", err)
	}
	return toOutput(move), nil
}

// GetForSnippet retrieves a single storage move for a specific snippet.
func GetForSnippet(ctx context.Context, client *gitlabclient.Client, in SnippetMoveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.SnippetID == 0 {
		return Output{}, toolutil.ErrFieldRequired("snippet_id")
	}
	if in.ID == 0 {
		return Output{}, toolutil.ErrFieldRequired("id")
	}

	move, _, err := client.GL().SnippetRepositoryStorageMove.GetStorageMoveForSnippet(in.SnippetID, in.ID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get snippet storage move for snippet", err)
	}
	return toOutput(move), nil
}

// Schedule schedules a repository storage move for a snippet.
func Schedule(ctx context.Context, client *gitlabclient.Client, in ScheduleInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.SnippetID == 0 {
		return Output{}, toolutil.ErrFieldRequired("snippet_id")
	}

	opts := gl.ScheduleStorageMoveForSnippetOptions{
		DestinationStorageName: in.DestinationStorageName,
	}
	move, _, err := client.GL().SnippetRepositoryStorageMove.ScheduleStorageMoveForSnippet(in.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("schedule snippet storage move", err)
	}
	return toOutput(move), nil
}

// ScheduleAll schedules storage moves for all snippets on a storage shard.
func ScheduleAll(ctx context.Context, client *gitlabclient.Client, in ScheduleAllInput) (ScheduleAllOutput, error) {
	if err := ctx.Err(); err != nil {
		return ScheduleAllOutput{}, err
	}

	opts := gl.ScheduleAllSnippetStorageMovesOptions{
		SourceStorageName:      in.SourceStorageName,
		DestinationStorageName: in.DestinationStorageName,
	}
	_, err := client.GL().SnippetRepositoryStorageMove.ScheduleAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ScheduleAllOutput{}, toolutil.WrapErrWithMessage("schedule all snippet storage moves", err)
	}
	return ScheduleAllOutput{Message: "All snippet repository storage moves have been scheduled"}, nil
}

func toOutput(m *gl.SnippetRepositoryStorageMove) Output {
	o := Output{
		ID:                     m.ID,
		State:                  m.State,
		SourceStorageName:      m.SourceStorageName,
		DestinationStorageName: m.DestinationStorageName,
	}
	if m.CreatedAt != nil {
		o.CreatedAt = *m.CreatedAt
	}
	if m.Snippet != nil {
		o.Snippet = &SnippetOutput{
			ID:     m.Snippet.ID,
			Title:  m.Snippet.Title,
			WebURL: m.Snippet.WebURL,
		}
	}
	return o
}
