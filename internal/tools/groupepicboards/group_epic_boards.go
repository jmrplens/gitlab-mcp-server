package groupepicboards

import (
	"context"
	"errors"
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

// Output represents a group epic board. Nested objects (group, labels, lists)
// mirror the full client-go gl.GroupEpicBoard sub-objects per the 1:1 audit
// policy: group is the full gl.Group identifying subset, labels are full
// gl.LabelDetails mirrors, and lists are full gl.BoardList mirrors.
type Output struct {
	toolutil.HintableOutput
	ID     int64                 `json:"id"`
	Name   string                `json:"name"`
	Group  *GroupRefOutput       `json:"group,omitempty"`
	Labels []*LabelDetailsOutput `json:"labels,omitempty"`
	Lists  []BoardListOutput     `json:"lists,omitempty"`
}

// ListOutput holds a paginated list of group epic boards.
type ListOutput struct {
	toolutil.HintableOutput
	Boards     []Output                  `json:"boards"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// toOutput converts a GitLab GroupEpicBoard to the MCP tool output format.
func toOutput(b *gl.GroupEpicBoard) Output {
	if b == nil {
		return Output{}
	}
	out := Output{
		ID:     b.ID,
		Name:   b.Name,
		Group:  groupRefOutput(b.Group),
		Labels: labelDetailsOutputs(b.Labels),
	}
	for _, bl := range b.Lists {
		if bl == nil {
			continue
		}
		out.Lists = append(out.Lists, convertBoardList(bl))
	}
	return out
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
	boards, resp, err := client.GL().GroupEpicBoards.ListGroupEpicBoards(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("groupEpicBoardList", err, http.StatusNotFound, groupEpicBoardHint)
	}
	out := make([]Output, len(boards))
	for i, b := range boards {
		out[i] = toOutput(b)
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
	b, _, err := client.GL().GroupEpicBoards.GetGroupEpicBoard(string(input.GroupID), input.BoardID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("groupEpicBoardGet", err, http.StatusNotFound, "verify board_id with epic_board_list on gitlab_group; if the list is empty, configure an epic board in GitLab first")
	}
	return toOutput(b), nil
}
