package groupepicboards

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const groupEpicBoardHint = "verify group_id; epic boards require Premium/Ultimate and can be empty until boards are configured for the group"

// ListInput defines parameters for listing group epic boards. It mirrors
// gl.ListGroupEpicBoardsOptions (which embeds gl.ListOptions), exposing the
// offset and keyset pagination controls of that options struct.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column by which to order results (keyset pagination)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// GetInput defines parameters for getting a single group epic board.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Epic board ID,required"`
}

// Output represents a group epic board. Per the doc-grounded reconcile, the
// nested group, labels, and lists sub-objects are trimmed to the documented
// fields of doc/api/group_epic_boards.md, and the documented hide_backlog_list /
// hide_closed_list board flags (absent from client-go's gl.GroupEpicBoard) are
// surfaced via the raw REST superset.
type Output struct {
	toolutil.HintableOutput
	ID              int64                 `json:"id"`
	Name            string                `json:"name"`
	HideBacklogList bool                  `json:"hide_backlog_list"`
	HideClosedList  bool                  `json:"hide_closed_list"`
	Group           *GroupRefOutput       `json:"group,omitempty"`
	Labels          []*LabelDetailsOutput `json:"labels,omitempty"`
	Lists           []BoardListOutput     `json:"lists,omitempty"`
}

// ListOutput holds a paginated list of group epic boards.
type ListOutput struct {
	toolutil.HintableOutput
	Boards     []Output                  `json:"boards"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Raw REST superset types
// ---------------------------------------------------------------------------.

// labelDetailsAPI is a raw-fetch superset over the epic-board scope label. The
// client-go gl.LabelDetails struct only decodes id/name/color/description/
// description_html/text_color, so this type adds the documented `title`,
// group_id, project_id, template, created_at, and updated_at keys that the
// group epic board endpoint returns. Decoding is single-pass and naturally
// version-tolerant: absent keys stay at their zero value and are omitted from
// the MCP envelope.
type labelDetailsAPI struct {
	gl.LabelDetails
	Title     string `json:"title"`
	GroupID   int64  `json:"group_id"`
	ProjectID int64  `json:"project_id"`
	Template  bool   `json:"template"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// boardListAPI is a raw-fetch superset over client-go's gl.BoardList. It embeds
// the SDK struct (so id/label/position still decode through the documented json
// keys) and adds the documented `list_type` and `collapsed` keys that
// gl.BoardList omits. Collapsed is a pointer so its documented false value is
// preserved while a fully absent key stays nil and is omitted.
type boardListAPI struct {
	gl.BoardList
	ListType  string `json:"list_type"`
	Collapsed *bool  `json:"collapsed"`
}

// groupEpicBoardAPI is a raw-fetch superset over client-go's gl.GroupEpicBoard.
// It embeds the SDK struct and shadows the labels/lists keys with the superset
// element types, and adds the documented hide_backlog_list / hide_closed_list
// flags that gl.GroupEpicBoard omits. The shallower (outer) Labels/Lists fields
// win over the embedded gl.GroupEpicBoard fields during json decoding.
type groupEpicBoardAPI struct {
	gl.GroupEpicBoard
	HideBacklogList bool               `json:"hide_backlog_list"`
	HideClosedList  bool               `json:"hide_closed_list"`
	Labels          []*labelDetailsAPI `json:"labels"`
	Lists           []*boardListAPI    `json:"lists"`
}

// toOutput converts a raw-superset GroupEpicBoard to the MCP tool output format.
func toOutput(b *groupEpicBoardAPI) Output {
	if b == nil {
		return Output{}
	}
	out := Output{
		ID:              b.ID,
		Name:            b.Name,
		HideBacklogList: b.HideBacklogList,
		HideClosedList:  b.HideClosedList,
		Group:           groupRefOutput(b.Group),
		Labels:          labelDetailsOutputs(b.Labels),
	}
	for _, bl := range b.Lists {
		if bl == nil {
			continue
		}
		out.Lists = append(out.Lists, convertBoardList(bl))
	}
	return out
}

// rawListBoards issues a raw REST GET for a group's epic boards, decoding the
// full documented response (including SDK-missing hide_*_list, label, and list
// fields) into a slice of [groupEpicBoardAPI]. The supplied opts encode
// pagination via their url struct tags, and the returned gl.Response preserves
// pagination headers for [toolutil.PaginationFromResponse].
func rawListBoards(ctx context.Context, client *gitlabclient.Client, groupID string, opts *gl.ListGroupEpicBoardsOptions) ([]*groupEpicBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("groups/%s/epic_boards", gl.PathEscape(groupID))
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var boards []*groupEpicBoardAPI
	resp, err := client.GL().Do(req, &boards)
	return boards, resp, err
}

// rawGetBoard issues a raw REST GET for a single group epic board, decoding the
// full documented response into a [groupEpicBoardAPI].
func rawGetBoard(ctx context.Context, client *gitlabclient.Client, groupID string, boardID int64) (*groupEpicBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("groups/%s/epic_boards/%d", gl.PathEscape(groupID), boardID)
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var board groupEpicBoardAPI
	resp, err := client.GL().Do(req, &board)
	return &board, resp, err
}

// List retrieves epic boards for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, errors.New("groupEpicBoardList: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	opts := &gl.ListGroupEpicBoardsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	boards, resp, err := rawListBoards(ctx, client, string(input.GroupID), opts)
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("groupEpicBoardList", err, http.StatusNotFound, groupEpicBoardHint)
	}
	out := make([]Output, 0, len(boards))
	for _, b := range boards {
		out = append(out, toOutput(b))
	}
	return ListOutput{Boards: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Get retrieves a single group epic board by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, errors.New("groupEpicBoardGet: group_id is required. Use gitlab_group_list to find the group ID first")
	}
	if input.BoardID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("groupEpicBoardGet", "board_id")
	}
	b, _, err := rawGetBoard(ctx, client, string(input.GroupID), input.BoardID)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupEpicBoardGet", err, http.StatusNotFound, "verify board_id with epic_board_list on gitlab_group; if the list is empty, configure an epic board in GitLab first")
	}
	return toOutput(b), nil
}
