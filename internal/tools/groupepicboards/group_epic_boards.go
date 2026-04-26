// Package groupepicboards implements GitLab group epic board operations
// including listing and getting board details.
package groupepicboards

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput defines parameters for listing group epic boards.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetInput defines parameters for getting a single group epic board.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Epic board ID,required"`
}

// BoardListEntry represents a single list (column) in an epic board.
type BoardListEntry struct {
	ID       int64  `json:"id"`
	LabelID  int64  `json:"label_id,omitempty"`
	Label    string `json:"label,omitempty"`
	Position int64  `json:"position"`
}

// Output represents a group epic board.
type Output struct {
	toolutil.HintableOutput
	ID     int64            `json:"id"`
	Name   string           `json:"name"`
	Labels []string         `json:"labels,omitempty"`
	Lists  []BoardListEntry `json:"lists,omitempty"`
}

// ListOutput holds a paginated list of group epic boards.
type ListOutput struct {
	toolutil.HintableOutput
	Boards     []Output                  `json:"boards"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// toOutput converts a GitLab GroupEpicBoard to the MCP tool output format.
func toOutput(b *gl.GroupEpicBoard) Output {
	out := Output{
		ID:   b.ID,
		Name: b.Name,
	}
	for _, l := range b.Labels {
		if l != nil {
			out.Labels = append(out.Labels, l.Name)
		}
	}
	for _, bl := range b.Lists {
		if bl == nil {
			continue
		}
		entry := BoardListEntry{
			ID:       bl.ID,
			Position: bl.Position,
		}
		if bl.Label != nil {
			entry.LabelID = bl.Label.ID
			entry.Label = bl.Label.Name
		}
		out.Lists = append(out.Lists, entry)
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
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	boards, resp, err := client.GL().GroupEpicBoards.ListGroupEpicBoards(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("groupEpicBoardList", err, http.StatusNotFound, "verify group_id \u2014 epic boards require Premium license")
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
		return Output{}, toolutil.WrapErrWithStatusHint("groupEpicBoardGet", err, http.StatusNotFound, "verify board_id with gitlab_group_epic_board_list")
	}
	return toOutput(b), nil
}
