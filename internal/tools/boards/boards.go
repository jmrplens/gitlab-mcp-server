// Package boards implements MCP tools for GitLab project issue boards and board lists.
//
// It wraps the IssueBoardsService from the GitLab client-go library, exposing
// 10 operations: 5 for board CRUD and 5 for board list CRUD.
package boards

import (
	"context"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// BoardOutput represents a GitLab issue board.
type BoardOutput struct {
	toolutil.HintableOutput
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	ProjectID       int64             `json:"project_id,omitempty"`
	ProjectName     string            `json:"project_name,omitempty"`
	ProjectPath     string            `json:"project_path,omitempty"`
	MilestoneID     int64             `json:"milestone_id,omitempty"`
	MilestoneTitle  string            `json:"milestone_title,omitempty"`
	AssigneeID      int64             `json:"assignee_id,omitempty"`
	AssigneeUser    string            `json:"assignee_username,omitempty"`
	Weight          int64             `json:"weight,omitempty"`
	Labels          []string          `json:"labels,omitempty"`
	HideBacklogList bool              `json:"hide_backlog_list"`
	HideClosedList  bool              `json:"hide_closed_list"`
	Lists           []BoardListOutput `json:"lists,omitempty"`
}

// BoardListOutput represents a single list within a board.
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

// ListBoardsOutput represents a paginated list of boards.
type ListBoardsOutput struct {
	toolutil.HintableOutput
	Boards     []BoardOutput             `json:"boards"`
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

// convertBoard is an internal helper for the boards package.
func convertBoard(b *gl.IssueBoard) BoardOutput {
	out := BoardOutput{
		ID:              b.ID,
		Name:            b.Name,
		Weight:          b.Weight,
		HideBacklogList: b.HideBacklogList,
		HideClosedList:  b.HideClosedList,
	}
	if b.Project != nil {
		out.ProjectID = b.Project.ID
		out.ProjectName = b.Project.Name
		out.ProjectPath = b.Project.PathWithNamespace
	}
	if b.Milestone != nil {
		out.MilestoneID = b.Milestone.ID
		out.MilestoneTitle = b.Milestone.Title
	}
	if b.Assignee != nil {
		out.AssigneeID = b.Assignee.ID
		out.AssigneeUser = b.Assignee.Username
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

// convertBoardList is an internal helper for the boards package.
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
// Board CRUD handlers
// ---------------------------------------------------------------------------.

// ListBoardsInput represents input for listing project issue boards.
type ListBoardsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	toolutil.PaginationInput
}

// ListBoards lists all issue boards for a project.
func ListBoards(ctx context.Context, client *gitlabclient.Client, input ListBoardsInput) (ListBoardsOutput, error) {
	if input.ProjectID == "" {
		return ListBoardsOutput{}, toolutil.WrapErrWithMessage("board_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListIssueBoardsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	boards, resp, err := client.GL().Boards.ListIssueBoards(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListBoardsOutput{}, toolutil.WrapErrWithStatusHint("board_list", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get \u2014 issue boards must be enabled in project settings")
	}
	out := ListBoardsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, b := range boards {
		out.Boards = append(out.Boards, convertBoard(b))
	}
	return out, nil
}

// GetBoardInput represents input for getting a single board.
type GetBoardInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id" jsonschema:"Board ID,required"`
}

// GetBoard retrieves a single issue board.
func GetBoard(ctx context.Context, client *gitlabclient.Client, input GetBoardInput) (BoardOutput, error) {
	if input.ProjectID == "" {
		return BoardOutput{}, toolutil.WrapErrWithMessage("board_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return BoardOutput{}, toolutil.WrapErrWithMessage("board_get", toolutil.ErrFieldRequired("board_id"))
	}
	board, _, err := client.GL().Boards.GetIssueBoard(string(input.ProjectID), input.BoardID, gl.WithContext(ctx))
	if err != nil {
		return BoardOutput{}, toolutil.WrapErrWithStatusHint("board_get", err, http.StatusNotFound,
			"verify board_id with gitlab_board_list \u2014 board_id is the global board ID, not an IID")
	}
	return convertBoard(board), nil
}

// CreateBoardInput represents input for creating a board.
type CreateBoardInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	Name      string               `json:"name" jsonschema:"Board name,required"`
}

// CreateBoard creates a new issue board.
func CreateBoard(ctx context.Context, client *gitlabclient.Client, input CreateBoardInput) (BoardOutput, error) {
	if input.ProjectID == "" {
		return BoardOutput{}, toolutil.WrapErrWithMessage("board_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.Name == "" {
		return BoardOutput{}, toolutil.WrapErrWithMessage("board_create", toolutil.ErrFieldRequired("name"))
	}
	opts := &gl.CreateIssueBoardOptions{
		Name: new(input.Name),
	}
	board, _, err := client.GL().Boards.CreateIssueBoard(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return BoardOutput{}, toolutil.WrapErrWithHint("board_create", err,
				"creating multiple boards per project requires GitLab Premium or Ultimate; on Free tier each project supports a single board")
		}
		return BoardOutput{}, toolutil.WrapErrWithStatusHint("board_create", err, http.StatusNotFound,
			"verify the project exists with gitlab_project_get and that you have Reporter+ role")
	}
	return convertBoard(board), nil
}

// UpdateBoardInput represents input for updating a board.
type UpdateBoardInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID         int64                `json:"board_id" jsonschema:"Board ID,required"`
	Name            string               `json:"name,omitempty" jsonschema:"Board name"`
	AssigneeID      int64                `json:"assignee_id,omitempty" jsonschema:"Assignee user ID"`
	MilestoneID     int64                `json:"milestone_id,omitempty" jsonschema:"Milestone ID"`
	Labels          string               `json:"labels,omitempty" jsonschema:"Comma-separated board scope labels"`
	Weight          int64                `json:"weight,omitempty" jsonschema:"Board scope weight"`
	HideBacklogList *bool                `json:"hide_backlog_list,omitempty" jsonschema:"Hide the Open list"`
	HideClosedList  *bool                `json:"hide_closed_list,omitempty" jsonschema:"Hide the Closed list"`
}

// UpdateBoard updates an existing issue board.
func UpdateBoard(ctx context.Context, client *gitlabclient.Client, input UpdateBoardInput) (BoardOutput, error) {
	if input.ProjectID == "" {
		return BoardOutput{}, toolutil.WrapErrWithMessage("board_update", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return BoardOutput{}, toolutil.WrapErrWithMessage("board_update", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.UpdateIssueBoardOptions{}
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
	if input.HideBacklogList != nil {
		opts.HideBacklogList = input.HideBacklogList
	}
	if input.HideClosedList != nil {
		opts.HideClosedList = input.HideClosedList
	}
	board, _, err := client.GL().Boards.UpdateIssueBoard(string(input.ProjectID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return BoardOutput{}, toolutil.WrapErrWithHint("board_update", err,
				"board scope (assignee/milestone/labels/weight) requires GitLab Premium or Ultimate; on Free tier only name and hide_*_list are mutable")
		}
		return BoardOutput{}, toolutil.WrapErrWithStatusHint("board_update", err, http.StatusNotFound,
			"verify board_id with gitlab_board_list")
	}
	return convertBoard(board), nil
}

// DeleteBoardInput represents input for deleting a board.
type DeleteBoardInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id" jsonschema:"Board ID,required"`
}

// DeleteBoard deletes an issue board.
func DeleteBoard(ctx context.Context, client *gitlabclient.Client, input DeleteBoardInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("board_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return toolutil.WrapErrWithMessage("board_delete", toolutil.ErrFieldRequired("board_id"))
	}
	_, err := client.GL().Boards.DeleteIssueBoard(string(input.ProjectID), input.BoardID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return toolutil.WrapErrWithHint("board_delete", err,
				"deleting boards requires Maintainer+ role; the default board cannot be deleted on Free tier")
		}
		return toolutil.WrapErrWithStatusHint("board_delete", err, http.StatusNotFound,
			"verify board_id with gitlab_board_list")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Board List CRUD handlers
// ---------------------------------------------------------------------------.

// ListBoardListsInput represents input for listing board lists.
type ListBoardListsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id" jsonschema:"Board ID,required"`
	toolutil.PaginationInput
}

// ListBoardLists lists all lists in a board.
func ListBoardLists(ctx context.Context, client *gitlabclient.Client, input ListBoardListsInput) (ListBoardListsOutput, error) {
	if input.ProjectID == "" {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("board_list_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("board_list_list", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.GetIssueBoardListsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	lists, resp, err := client.GL().Boards.GetIssueBoardLists(string(input.ProjectID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListBoardListsOutput{}, toolutil.WrapErrWithStatusHint("board_list_list", err, http.StatusNotFound,
			"verify project_id and board_id with gitlab_board_list")
	}
	out := ListBoardListsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, l := range lists {
		out.Lists = append(out.Lists, convertBoardList(l))
	}
	return out, nil
}

// GetBoardListInput represents input for getting a single board list.
type GetBoardListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id" jsonschema:"Board ID,required"`
	ListID    int64                `json:"list_id" jsonschema:"Board list ID,required"`
}

// GetBoardList retrieves a single board list.
func GetBoardList(ctx context.Context, client *gitlabclient.Client, input GetBoardListInput) (BoardListOutput, error) {
	if input.ProjectID == "" {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_get", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_get", toolutil.ErrFieldRequired("board_id"))
	}
	if input.ListID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_get", toolutil.ErrFieldRequired("list_id"))
	}
	list, _, err := client.GL().Boards.GetIssueBoardList(string(input.ProjectID), input.BoardID, input.ListID, gl.WithContext(ctx))
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("board_list_get", err, http.StatusNotFound,
			"verify board_id and list_id with gitlab_board_list_list")
	}
	return convertBoardList(list), nil
}

// CreateBoardListInput represents input for creating a board list.
type CreateBoardListInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID     int64                `json:"board_id" jsonschema:"Board ID,required"`
	LabelID     int64                `json:"label_id,omitempty" jsonschema:"Label ID to create a label list"`
	AssigneeID  int64                `json:"assignee_id,omitempty" jsonschema:"Assignee ID to create an assignee list"`
	MilestoneID int64                `json:"milestone_id,omitempty" jsonschema:"Milestone ID to create a milestone list"`
	IterationID int64                `json:"iteration_id,omitempty" jsonschema:"Iteration ID to create an iteration list"`
}

// CreateBoardList creates a new board list.
func CreateBoardList(ctx context.Context, client *gitlabclient.Client, input CreateBoardListInput) (BoardListOutput, error) {
	if input.ProjectID == "" {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_create", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_create", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.CreateIssueBoardListOptions{}
	if input.LabelID != 0 {
		opts.LabelID = new(input.LabelID)
	}
	if input.AssigneeID != 0 {
		opts.AssigneeID = new(input.AssigneeID)
	}
	if input.MilestoneID != 0 {
		opts.MilestoneID = new(input.MilestoneID)
	}
	if input.IterationID != 0 {
		opts.IterationID = new(input.IterationID)
	}
	list, _, err := client.GL().Boards.CreateIssueBoardList(string(input.ProjectID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return BoardListOutput{}, toolutil.WrapErrWithHint("board_list_create", err,
				"assignee_id, milestone_id, and iteration_id lists require GitLab Premium or Ultimate; on Free tier only label_id lists are supported")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return BoardListOutput{}, toolutil.WrapErrWithHint("board_list_create", err,
				"exactly one of label_id, assignee_id, milestone_id, or iteration_id must be set; verify the referenced ID exists in this project's scope")
		}
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("board_list_create", err, http.StatusNotFound,
			"verify project_id and board_id with gitlab_board_list")
	}
	return convertBoardList(list), nil
}

// UpdateBoardListInput represents input for updating (reordering) a board list.
type UpdateBoardListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id" jsonschema:"Board ID,required"`
	ListID    int64                `json:"list_id" jsonschema:"Board list ID,required"`
	Position  int64                `json:"position" jsonschema:"New position of the list,required"`
}

// UpdateBoardList reorders a board list.
func UpdateBoardList(ctx context.Context, client *gitlabclient.Client, input UpdateBoardListInput) (BoardListOutput, error) {
	if input.ProjectID == "" {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_update", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_update", toolutil.ErrFieldRequired("board_id"))
	}
	if input.ListID == 0 {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("board_list_update", toolutil.ErrFieldRequired("list_id"))
	}
	opts := &gl.UpdateIssueBoardListOptions{
		Position: new(input.Position),
	}
	list, _, err := client.GL().Boards.UpdateIssueBoardList(string(input.ProjectID), input.BoardID, input.ListID, opts, gl.WithContext(ctx))
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("board_list_update", err, http.StatusNotFound,
			"verify board_id and list_id with gitlab_board_list_list \u2014 position is 0-based and must be within the current list count")
	}
	return convertBoardList(list), nil
}

// DeleteBoardListInput represents input for deleting a board list.
type DeleteBoardListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id" jsonschema:"Board ID,required"`
	ListID    int64                `json:"list_id" jsonschema:"Board list ID,required"`
}

// DeleteBoardList deletes a board list.
func DeleteBoardList(ctx context.Context, client *gitlabclient.Client, input DeleteBoardListInput) error {
	if input.ProjectID == "" {
		return toolutil.WrapErrWithMessage("board_list_delete", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return toolutil.WrapErrWithMessage("board_list_delete", toolutil.ErrFieldRequired("board_id"))
	}
	if input.ListID == 0 {
		return toolutil.WrapErrWithMessage("board_list_delete", toolutil.ErrFieldRequired("list_id"))
	}
	_, err := client.GL().Boards.DeleteIssueBoardList(string(input.ProjectID), input.BoardID, input.ListID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("board_list_delete", err, http.StatusNotFound,
			"verify board_id and list_id with gitlab_board_list_list")
	}
	return nil
}
