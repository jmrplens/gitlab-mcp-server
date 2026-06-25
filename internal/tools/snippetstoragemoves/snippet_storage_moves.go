package snippetstoragemoves

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ListInput holds pagination and ordering parameters for listing all snippet
// storage moves. Offset and keyset pagination are both supported, mirroring
// gl.RetrieveAllSnippetStorageMovesOptions (embedded gl.ListOptions).
type ListInput struct {
	OrderBy string `json:"order_by,omitempty" jsonschema:"Field to order results by (for example 'id'). Maps to the GitLab order_by query parameter."`
	Sort    string `json:"sort,omitempty"     jsonschema:"Sort direction: 'asc' or 'desc'. Maps to the GitLab sort query parameter."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListForSnippetInput holds parameters for listing storage moves for a specific
// snippet. Offset and keyset pagination are both supported, mirroring
// gl.RetrieveAllSnippetStorageMovesOptions (embedded gl.ListOptions).
type ListForSnippetInput struct {
	SnippetID int64  `json:"snippet_id"         jsonschema:"Numeric ID of the snippet,required"`
	OrderBy   string `json:"order_by,omitempty" jsonschema:"Field to order results by (for example 'id'). Maps to the GitLab order_by query parameter."`
	Sort      string `json:"sort,omitempty"     jsonschema:"Sort direction: 'asc' or 'desc'. Maps to the GitLab sort query parameter."`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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

// SnippetOutput represents the snippet associated with a storage move. It
// mirrors gl.RepositorySnippet exactly so the 1:1 audit surfaces every field
// the GitLab API returns for the nested snippet reference.
type SnippetOutput struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description,omitempty"`
	Visibility    string     `json:"visibility,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	ProjectID     int64      `json:"project_id,omitempty"`
	WebURL        string     `json:"web_url,omitempty"`
	RawURL        string     `json:"raw_url,omitempty"`
	SSHURLToRepo  string     `json:"ssh_url_to_repo,omitempty"`
	HTTPURLToRepo string     `json:"http_url_to_repo,omitempty"`
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
		ListOptions: gl.ListOptions{OrderBy: in.OrderBy, Sort: in.Sort},
	}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput)
	moves, resp, err := client.GL().SnippetRepositoryStorageMove.RetrieveAllStorageMoves(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve all snippet storage moves", err, http.StatusForbidden,
			"requires administrator access; self-managed only; storage moves are repository shard migrations between Gitaly nodes")
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
		ListOptions: gl.ListOptions{OrderBy: in.OrderBy, Sort: in.Sort},
	}
	toolutil.ApplyListOptions(&opts.ListOptions, in.PaginationInput, in.KeysetPaginationInput)
	moves, resp, err := client.GL().SnippetRepositoryStorageMove.RetrieveAllStorageMovesForSnippet(in.SnippetID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("retrieve snippet storage moves", err, http.StatusNotFound,
			"requires admin; verify snippet_id exists; only storage moves for the given snippet are returned")
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
		return Output{}, toolutil.WrapErrWithStatusHint("get snippet storage move", err, http.StatusNotFound,
			"requires admin; verify id with gitlab_retrieve_all_snippet_storage_moves; the move record may have been pruned after completion")
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
		return Output{}, toolutil.WrapErrWithStatusHint("get snippet storage move for snippet", err, http.StatusNotFound,
			"requires admin; verify snippet_id + id combination with gitlab_get_snippet_storage_move_for_snippet")
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
		return Output{}, toolutil.WrapErrWithStatusHint("schedule snippet storage move", err, http.StatusBadRequest,
			"requires admin; destination_storage_name must reference an existing Gitaly storage shard configured on the instance; cannot move to the same shard the snippet is already on")
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
		return ScheduleAllOutput{}, toolutil.WrapErrWithStatusHint("schedule all snippet storage moves", err, http.StatusBadRequest,
			"requires admin; source_storage_name and destination_storage_name must reference configured Gitaly shards; bulk operation \u2014 may schedule many concurrent moves")
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
			ID:            m.Snippet.ID,
			Title:         m.Snippet.Title,
			Description:   m.Snippet.Description,
			Visibility:    string(m.Snippet.Visibility),
			UpdatedAt:     m.Snippet.UpdatedAt,
			CreatedAt:     m.Snippet.CreatedAt,
			ProjectID:     m.Snippet.ProjectID,
			WebURL:        m.Snippet.WebURL,
			RawURL:        m.Snippet.RawURL,
			SSHURLToRepo:  m.Snippet.SSHURLToRepo,
			HTTPURLToRepo: m.Snippet.HTTPURLToRepo,
		}
	}
	return o
}
