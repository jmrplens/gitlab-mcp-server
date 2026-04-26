// Package groupboards implements MCP tools for GitLab group issue boards and board lists.
//
// It wraps the GroupIssueBoardsService from the GitLab client-go library, exposing
// 10 operations: 5 for group board CRUD and 5 for group board list CRUD.
package groupboards

import (
	"context"
	"errors"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// GroupBoardOutput represents a GitLab group issue board.
type GroupBoardOutput struct {
	toolutil.HintableOutput
	ID             int64             `json:"id"`
	Name           string            `json:"name"`
	GroupID        int64             `json:"group_id,omitempty"`
	GroupName      string            `json:"group_name,omitempty"`
	MilestoneID    int64             `json:"milestone_id,omitempty"`
	MilestoneTitle string            `json:"milestone_title,omitempty"`
	Labels         []string          `json:"labels,omitempty"`
	Lists          []BoardListOutput `json:"lists,omitempty"`
}

// BoardListOutput represents a single list within a group board.
type BoardListOutput struct {
	toolutil.HintableOutput
	ID             int64  `json:"id"`
	LabelID        int64  `json:"label_id,omitempty"`
	LabelName      string `json:"label_name,omitempty"`
	Position       int64  `json:"position"`
	MaxIssueCount  int64  `json:"max_issue_count,omitempty"`
	MaxIssueWeight int64  `json:"max_issue_weight,omitempty"`
	AssigneeID     int64  `json:"assignee_id,omitempty"`
	AssigneeUser   string `json:"assignee_username,omitempty"`
	MilestoneID    int64  `json:"milestone_id,omitempty"`
	MilestoneTitle string `json:"milestone_title,omitempty"`
}

// ListGroupBoardsOutput represents a paginated list of group boards.
type ListGroupBoardsOutput struct {
	toolutil.HintableOutput
	Boards     []GroupBoardOutput        `json:"boards"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListBoardListsOutput represents a paginated list of board lists.
type ListBoardListsOutput struct {
	toolutil.HintableOutput
	Lists      []BoardListOutput         `json:"lists"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// convertGroupBoard is an internal helper for the groupboards package.
func convertGroupBoard(b *gl.GroupIssueBoard) GroupBoardOutput {
	out := GroupBoardOutput{
		ID:   b.ID,
		Name: b.Name,
	}
	if b.Group != nil {
		out.GroupID = b.Group.ID
		out.GroupName = b.Group.Name
	}
	if b.Milestone != nil {
		out.MilestoneID = b.Milestone.ID
		out.MilestoneTitle = b.Milestone.Title
	}
	for _, lbl := range b.Labels {
		if lbl != nil {
			out.Labels = append(out.Labels, lbl.Name)
		}
	}
	for _, l := range b.Lists {
		out.Lists = append(out.Lists, convertBoardList(l))
	}
	return out
}

// convertBoardList is an internal helper for the groupboards package.
func convertBoardList(l *gl.BoardList) BoardListOutput {
	out := BoardListOutput{
		ID:             l.ID,
		Position:       l.Position,
		MaxIssueCount:  l.MaxIssueCount,
		MaxIssueWeight: l.MaxIssueWeight,
	}
	if l.Label != nil {
		out.LabelID = l.Label.ID
		out.LabelName = l.Label.Name
	}
	if l.Assignee != nil {
		out.AssigneeID = l.Assignee.ID
		out.AssigneeUser = l.Assignee.Username
	}
	if l.Milestone != nil {
		out.MilestoneID = l.Milestone.ID
		out.MilestoneTitle = l.Milestone.Title
	}
	return out
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Group Board CRUD handlers
// ---------------------------------------------------------------------------.

// ListGroupBoardsInput represents input for listing group issue boards.
type ListGroupBoardsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	toolutil.PaginationInput
}

// ListGroupBoards lists all issue boards for a group.
func ListGroupBoards(ctx context.Context, client *gitlabclient.Client, input ListGroupBoardsInput) (ListGroupBoardsOutput, error) {
	if input.GroupID == "" {
		return ListGroupBoardsOutput{}, toolutil.WrapErrWithMessage("group_board_list", toolutil.ErrFieldRequired("group_id"))
	}
	opts := &gl.ListGroupIssueBoardsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	boards, resp, err := client.GL().GroupIssueBoards.ListGroupIssueBoards(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return ListGroupBoardsOutput{}, toolutil.WrapErrWithHint("group_board_list", err,
				"group issue boards require GitLab Premium or Ultimate (multiple boards per group); on Free tier groups have a single implicit board only")
		}
		return ListGroupBoardsOutput{}, toolutil.WrapErrWithStatusHint("group_board_list", err, http.StatusNotFound,
			"verify the group exists with gitlab_group_get")
	}
	out := ListGroupBoardsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, b := range boards {
		out.Boards = append(out.Boards, convertGroupBoard(b))
	}
	return out, nil
}

// GetGroupBoardInput represents input for getting a single group board.
type GetGroupBoardInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Board ID,required"`
}

// GetGroupBoard retrieves a single group issue board.
func GetGroupBoard(ctx context.Context, client *gitlabclient.Client, input GetGroupBoardInput) (GroupBoardOutput, error) {
	if input.GroupID == "" {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_get", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_get", toolutil.ErrFieldRequired("board_id"))
	}
	board, _, err := client.GL().GroupIssueBoards.GetGroupIssueBoard(string(input.GroupID), input.BoardID, gl.WithContext(ctx))
	if err != nil {
		return GroupBoardOutput{}, toolutil.WrapErrWithStatusHint("group_board_get", err, http.StatusNotFound,
			"board_id not found on this group \u2014 use gitlab_group_board_list to discover current board IDs")
	}
	return convertGroupBoard(board), nil
}

// CreateGroupBoardInput represents input for creating a group board.
type CreateGroupBoardInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	Name    string               `json:"name" jsonschema:"Board name,required"`
}

// CreateGroupBoard creates a new group issue board.
func CreateGroupBoard(ctx context.Context, client *gitlabclient.Client, input CreateGroupBoardInput) (GroupBoardOutput, error) {
	if input.GroupID == "" {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_create", toolutil.ErrFieldRequired("group_id"))
	}
	if input.Name == "" {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_create", toolutil.ErrFieldRequired("name"))
	}
	opts := &gl.CreateGroupIssueBoardOptions{
		Name: new(input.Name),
	}
	board, _, err := client.GL().GroupIssueBoards.CreateGroupIssueBoard(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return GroupBoardOutput{}, toolutil.WrapErrWithHint("group_board_create", err,
				"creating multiple group issue boards requires GitLab Premium or Ultimate, plus Reporter role on the group")
		}
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return GroupBoardOutput{}, toolutil.WrapErrWithHint("group_board_create", err,
				"name is required and must be unique within the group; verify all referenced milestone/iteration/label IDs exist via gitlab_milestone_list / gitlab_group_label_list")
		}
		return GroupBoardOutput{}, toolutil.WrapErrWithStatusHint("group_board_create", err, http.StatusNotFound,
			"verify the group exists with gitlab_group_get")
	}
	return convertGroupBoard(board), nil
}

// UpdateGroupBoardInput represents input for updating a group board.
type UpdateGroupBoardInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID     int64                `json:"board_id" jsonschema:"Board ID,required"`
	Name        string               `json:"name,omitempty" jsonschema:"Board name"`
	AssigneeID  int64                `json:"assignee_id,omitempty" jsonschema:"Assignee user ID"`
	MilestoneID int64                `json:"milestone_id,omitempty" jsonschema:"Milestone ID"`
	Labels      string               `json:"labels,omitempty" jsonschema:"Comma-separated board scope labels"`
	Weight      int64                `json:"weight,omitempty" jsonschema:"Board scope weight"`
}

// UpdateGroupBoard updates a group issue board.
func UpdateGroupBoard(ctx context.Context, client *gitlabclient.Client, input UpdateGroupBoardInput) (GroupBoardOutput, error) {
	if input.GroupID == "" {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_update", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_update", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.UpdateGroupIssueBoardOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = new(input.AssigneeID)
	}
	if input.MilestoneID != 0 {
		opts.MilestoneID = new(input.MilestoneID)
	}
	if input.Labels != "" {
		lbls := gl.LabelOptions(strings.Split(input.Labels, ","))
		opts.Labels = &lbls
	}
	if input.Weight != 0 {
		opts.Weight = new(input.Weight)
	}
	board, _, err := client.GL().GroupIssueBoards.UpdateIssueBoard(string(input.GroupID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return GroupBoardOutput{}, toolutil.WrapErrWithHint("group_board_update", err,
				"updating board scope (assignee, milestone, iteration, labels, weight) requires GitLab Premium or Ultimate; basic name updates require Reporter role")
		}
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return GroupBoardOutput{}, toolutil.WrapErrWithHint("group_board_update", err,
				"verify referenced assignee_id (gitlab_get_user), milestone_id (gitlab_milestone_list), and label IDs exist; weight is 0\u20139")
		}
		return GroupBoardOutput{}, toolutil.WrapErrWithStatusHint("group_board_update", err, http.StatusNotFound,
			"board_id not found \u2014 use gitlab_group_board_list to verify")
	}
	return convertGroupBoard(board), nil
}

// DeleteGroupBoardInput represents input for deleting a group board.
type DeleteGroupBoardInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Board ID,required"`
}

// DeleteGroupBoard deletes a group issue board.
func DeleteGroupBoard(ctx context.Context, client *gitlabclient.Client, input DeleteGroupBoardInput) error {
	if input.GroupID == "" {
		return toolutil.WrapErrWithMessage("group_board_delete", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return toolutil.WrapErrWithMessage("group_board_delete", toolutil.ErrFieldRequired("board_id"))
	}
	_, err := client.GL().GroupIssueBoards.DeleteIssueBoard(string(input.GroupID), input.BoardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("group_board_delete", err,
				"deleting boards requires GitLab Premium/Ultimate plus Reporter role; the group's last/default board cannot be deleted")
		}
		return toolutil.WrapErrWithStatusHint("group_board_delete", err, http.StatusNotFound,
			"board_id already deleted or never existed")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Group Board List CRUD handlers
// ---------------------------------------------------------------------------.

// ListGroupBoardListsInput represents input for listing group board lists.
type ListGroupBoardListsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Board ID,required"`
	toolutil.PaginationInput
}

// ListGroupBoardLists lists all lists in a group board.
func ListGroupBoardLists(ctx context.Context, client *gitlabclient.Client, input ListGroupBoardListsInput) (ListBoardListsOutput, error) {
	if input.GroupID == "" {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("group_board_list_list", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("group_board_list_list", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.ListGroupIssueBoardListsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	lists, resp, err := client.GL().GroupIssueBoards.ListGroupIssueBoardLists(string(input.GroupID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListBoardListsOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_list", err, http.StatusNotFound,
			"board_id not found on this group \u2014 use gitlab_group_board_list to discover board IDs")
	}
	out := ListBoardListsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, l := range lists {
		out.Lists = append(out.Lists, convertBoardList(l))
	}
	return out, nil
}

// GetGroupBoardListInput represents input for getting a single group board list.
type GetGroupBoardListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Board ID,required"`
	ListID  int64                `json:"list_id" jsonschema:"Board list ID,required"`
}

// GetGroupBoardList retrieves a single group board list.
func GetGroupBoardList(ctx context.Context, client *gitlabclient.Client, input GetGroupBoardListInput) (BoardListOutput, error) {
	if input.GroupID == "" {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_get", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_get", toolutil.ErrFieldRequired("board_id"))
	}
	if input.ListID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_get", toolutil.ErrFieldRequired("list_id"))
	}
	list, _, err := client.GL().GroupIssueBoards.GetGroupIssueBoardList(string(input.GroupID), input.BoardID, input.ListID, gl.WithContext(ctx))
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_get", err, http.StatusNotFound,
			"list_id not found on this board \u2014 use gitlab_group_board_list_lists to discover list IDs (each list represents a label column)")
	}
	return convertBoardList(list), nil
}

// CreateGroupBoardListInput represents input for creating a group board list.
type CreateGroupBoardListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Board ID,required"`
	LabelID int64                `json:"label_id" jsonschema:"Label ID to create a label list,required"`
}

// CreateGroupBoardList creates a new group board list.
func CreateGroupBoardList(ctx context.Context, client *gitlabclient.Client, input CreateGroupBoardListInput) (BoardListOutput, error) {
	if input.GroupID == "" {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_create", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_create", toolutil.ErrFieldRequired("board_id"))
	}
	if input.LabelID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_create", toolutil.ErrFieldRequired("label_id"))
	}
	opts := &gl.CreateGroupIssueBoardListOptions{
		LabelID: new(input.LabelID),
	}
	list, _, err := client.GL().GroupIssueBoards.CreateGroupIssueBoardList(string(input.GroupID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return BoardListOutput{}, toolutil.WrapErrWithHint("group_board_list_create", err,
				"exactly one of label_id (group label), assignee_id (Premium+), or milestone_id (Premium+) must be provided; verify referenced ID exists; a list with the same scope already exists on this board")
		}
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_create", err, http.StatusForbidden,
			"creating non-label lists (assignee/milestone) requires GitLab Premium or Ultimate; all list creation requires Reporter role on the group")
	}
	return convertBoardList(list), nil
}

// UpdateGroupBoardListInput represents input for updating a group board list.
type UpdateGroupBoardListInput struct {
	GroupID  toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID  int64                `json:"board_id" jsonschema:"Board ID,required"`
	ListID   int64                `json:"list_id" jsonschema:"Board list ID,required"`
	Position int64                `json:"position" jsonschema:"New position of the list,required"`
}

// UpdateGroupBoardList reorders a group board list.
// The V2 API returns a slice of board lists; we return the first match.
func UpdateGroupBoardList(ctx context.Context, client *gitlabclient.Client, input UpdateGroupBoardListInput) (BoardListOutput, error) {
	if input.GroupID == "" {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_update", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_update", toolutil.ErrFieldRequired("board_id"))
	}
	if input.ListID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_update", toolutil.ErrFieldRequired("list_id"))
	}
	opts := &gl.UpdateGroupIssueBoardListOptions{
		Position: new(input.Position),
	}
	lists, _, err := client.GL().GroupIssueBoards.UpdateIssueBoardList(string(input.GroupID), input.BoardID, input.ListID, opts, gl.WithContext(ctx))
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_update", err, http.StatusNotFound,
			"list_id not found on this board (only the position can be updated; recreate the list to change its scope)")
	}
	// V2 returns a slice; find the updated list by ID
	for _, l := range lists {
		if l.ID == input.ListID {
			return convertBoardList(l), nil
		}
	}
	// Fallback to first element if available
	if len(lists) > 0 {
		return convertBoardList(lists[0]), nil
	}
	return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_update", errors.New("no board list returned"))
}

// DeleteGroupBoardListInput represents input for deleting a group board list.
type DeleteGroupBoardListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID int64                `json:"board_id" jsonschema:"Board ID,required"`
	ListID  int64                `json:"list_id" jsonschema:"Board list ID,required"`
}

// DeleteGroupBoardList deletes a group board list.
func DeleteGroupBoardList(ctx context.Context, client *gitlabclient.Client, input DeleteGroupBoardListInput) error {
	if input.GroupID == "" {
		return toolutil.WrapErrWithMessage("group_board_list_delete", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return toolutil.WrapErrWithMessage("group_board_list_delete", toolutil.ErrFieldRequired("board_id"))
	}
	if input.ListID == 0 {
		return toolutil.WrapErrWithMessage("group_board_list_delete", toolutil.ErrFieldRequired("list_id"))
	}
	_, err := client.GL().GroupIssueBoards.DeleteGroupIssueBoardList(string(input.GroupID), input.BoardID, input.ListID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("group_board_list_delete", err, http.StatusNotFound,
			"list_id already deleted or never existed on this board")
	}
	return nil
}
