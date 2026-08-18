package groupboards

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// GroupBoardOutput represents a GitLab group issue board. Nested objects
// (group, milestone, assignee, labels, lists) mirror the documented
// group_boards.md response sub-objects (see shapes.go). The hide_backlog_list,
// hide_closed_list, assignee and weight fields are documented by the API but
// absent from client-go's gl.GroupIssueBoard struct, so they are decoded via the
// raw-superset fetch path (groupIssueBoardAPI) used by the read/create/update
// handlers and stay version-tolerant (absent on older instances → zero value,
// omitted from the envelope).
type GroupBoardOutput struct {
	toolutil.HintableOutput
	ID              int64                 `json:"id"`
	Name            string                `json:"name"`
	Group           *GroupRefOutput       `json:"group,omitempty"`
	Milestone       *MilestoneOutput      `json:"milestone,omitempty" tier:"premium"`
	Assignee        *BasicUserOutput      `json:"assignee,omitempty" tier:"premium"`
	Weight          int64                 `json:"weight,omitempty" tier:"premium"`
	Labels          []*LabelDetailsOutput `json:"labels,omitempty"`
	HideBacklogList bool                  `json:"hide_backlog_list"`
	HideClosedList  bool                  `json:"hide_closed_list"`
	Lists           []BoardListOutput     `json:"lists,omitempty"`
}

// BoardListOutput represents a single list within a group board. Nested objects
// (assignee, label, iteration, milestone) mirror the full client-go gl.BoardList
// sub-objects per the 1:1 audit policy.
type BoardListOutput struct {
	toolutil.HintableOutput
	ID             int64                    `json:"id"`
	Assignee       *BoardListAssigneeOutput `json:"assignee,omitempty" tier:"premium"`
	Iteration      *IterationOutput         `json:"iteration,omitempty" tier:"premium"`
	Label          *LabelOutput             `json:"label,omitempty"`
	MaxIssueCount  int64                    `json:"max_issue_count,omitempty"`
	MaxIssueWeight int64                    `json:"max_issue_weight,omitempty"`
	Milestone      *MilestoneOutput         `json:"milestone,omitempty" tier:"premium"`
	Position       int64                    `json:"position"`
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
// Raw REST superset types
// ---------------------------------------------------------------------------.

// groupIssueBoardAPI is a raw-fetch superset over client-go's gl.GroupIssueBoard.
// It embeds the SDK struct (so every SDK-known field — id/name/group/milestone/
// labels/lists — still decodes through the documented json keys) and adds the
// documented hide_backlog_list, hide_closed_list, assignee and weight fields
// that gl.GroupIssueBoard omits. Decoding is single-pass and naturally
// version-tolerant: when these fields are absent from the response, they stay
// the zero value and (for assignee/weight) are omitted from the MCP envelope.
type groupIssueBoardAPI struct {
	gl.GroupIssueBoard
	HideBacklogList bool          `json:"hide_backlog_list"`
	HideClosedList  bool          `json:"hide_closed_list"`
	Assignee        *gl.BasicUser `json:"assignee"`
	Weight          int64         `json:"weight"`
}

// rawListGroupBoards issues a raw REST GET for a group's issue boards, decoding
// the full documented response (including each board's SDK-missing
// hide_backlog_list/hide_closed_list/assignee/weight) into a slice of
// [groupIssueBoardAPI]. The supplied opts encode pagination/order via their url
// struct tags, and the returned gl.Response preserves pagination headers for
// [toolutil.PaginationFromResponse].
func rawListGroupBoards(ctx context.Context, client *gitlabclient.Client, groupID string, opts *gl.ListGroupIssueBoardsOptions) ([]*groupIssueBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("groups/%s/boards", gl.PathEscape(groupID))
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var boards []*groupIssueBoardAPI
	resp, err := client.GL().Do(req, &boards)
	return boards, resp, err
}

// rawGetGroupBoard issues a raw REST GET for a single group issue board,
// decoding the full documented response (including the SDK-missing
// hide_backlog_list/hide_closed_list/assignee/weight) into a [groupIssueBoardAPI].
func rawGetGroupBoard(ctx context.Context, client *gitlabclient.Client, groupID string, boardID int64) (*groupIssueBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("groups/%s/boards/%d", gl.PathEscape(groupID), boardID)
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var board groupIssueBoardAPI
	resp, err := client.GL().Do(req, &board)
	return &board, resp, err
}

// rawCreateGroupBoard issues a raw REST POST creating a group issue board,
// decoding the full documented response into a [groupIssueBoardAPI] so the
// SDK-missing hide_*_list/assignee/weight fields are surfaced.
func rawCreateGroupBoard(ctx context.Context, client *gitlabclient.Client, groupID string, opts *gl.CreateGroupIssueBoardOptions) (*groupIssueBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("groups/%s/boards", gl.PathEscape(groupID))
	req, err := client.GL().NewRequest(http.MethodPost, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var board groupIssueBoardAPI
	resp, err := client.GL().Do(req, &board)
	return &board, resp, err
}

// rawUpdateGroupBoard issues a raw REST PUT updating a group issue board,
// decoding the full documented response into a [groupIssueBoardAPI] so the
// SDK-missing hide_*_list/assignee/weight fields are surfaced.
func rawUpdateGroupBoard(ctx context.Context, client *gitlabclient.Client, groupID string, boardID int64, opts *gl.UpdateGroupIssueBoardOptions) (*groupIssueBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("groups/%s/boards/%d", gl.PathEscape(groupID), boardID)
	req, err := client.GL().NewRequest(http.MethodPut, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var board groupIssueBoardAPI
	resp, err := client.GL().Do(req, &board)
	return &board, resp, err
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// convertGroupBoardAPI maps a raw-fetch group issue board (groupIssueBoardAPI
// superset) into MCP output, surfacing the documented hide_backlog_list,
// hide_closed_list, assignee and weight fields the client-go gl.GroupIssueBoard
// struct omits.
func convertGroupBoardAPI(b *groupIssueBoardAPI) GroupBoardOutput {
	out := GroupBoardOutput{
		ID:              b.ID,
		Name:            b.Name,
		Group:           groupRefOutput(b.Group),
		Milestone:       milestoneOutput(b.Milestone),
		Assignee:        basicUserOutput(b.Assignee),
		Weight:          b.Weight,
		Labels:          groupLabelOutputs(b.Labels),
		HideBacklogList: b.HideBacklogList,
		HideClosedList:  b.HideClosedList,
	}
	for _, l := range b.Lists {
		out.Lists = append(out.Lists, convertBoardList(l))
	}
	return out
}

// convertBoardList maps a GitLab board list into group board MCP output.
func convertBoardList(l *gl.BoardList) BoardListOutput {
	return BoardListOutput{
		ID:             l.ID,
		Assignee:       boardListAssigneeOutput(l.Assignee),
		Iteration:      iterationOutput(l.Iteration),
		Label:          labelOutput(l.Label),
		MaxIssueCount:  l.MaxIssueCount,
		MaxIssueWeight: l.MaxIssueWeight,
		Milestone:      milestoneOutput(l.Milestone),
		Position:       l.Position,
	}
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
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order results by (keyset pagination)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListGroupBoards lists all issue boards for a group.
func ListGroupBoards(ctx context.Context, client *gitlabclient.Client, input ListGroupBoardsInput) (ListGroupBoardsOutput, error) {
	if input.GroupID == "" {
		return ListGroupBoardsOutput{}, toolutil.WrapErrWithMessage("group_board_list", toolutil.ErrFieldRequired("group_id"))
	}
	opts := &gl.ListGroupIssueBoardsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	// Raw REST fetch so the documented hide_backlog_list/hide_closed_list/
	// assignee/weight fields (absent from client-go's gl.GroupIssueBoard) are
	// surfaced.
	boards, resp, err := rawListGroupBoards(ctx, client, string(input.GroupID), opts)
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
		out.Boards = append(out.Boards, convertGroupBoardAPI(b))
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
	// Raw REST fetch so the documented hide_backlog_list/hide_closed_list/
	// assignee/weight fields (absent from client-go's gl.GroupIssueBoard) are
	// surfaced.
	board, _, err := rawGetGroupBoard(ctx, client, string(input.GroupID), input.BoardID)
	if err != nil {
		return GroupBoardOutput{}, toolutil.WrapErrWithStatusHint("group_board_get", err, http.StatusNotFound,
			"board_id not found on this group \u2014 use gitlab_group_board_list to discover current board IDs")
	}
	return convertGroupBoardAPI(board), nil
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
	// Raw REST fetch so the documented hide_backlog_list/hide_closed_list/
	// assignee/weight fields (absent from client-go's gl.GroupIssueBoard) are
	// surfaced on the created board.
	board, _, err := rawCreateGroupBoard(ctx, client, string(input.GroupID), opts)
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
	return convertGroupBoardAPI(board), nil
}

// UpdateGroupBoardInput represents input for updating a group board.
type UpdateGroupBoardInput struct {
	GroupID     toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID     int64                `json:"board_id" jsonschema:"Board ID,required"`
	Name        string               `json:"name,omitempty" jsonschema:"Board name"`
	AssigneeID  int64                `json:"assignee_id,omitempty" jsonschema:"Assignee user ID"`
	MilestoneID int64                `json:"milestone_id,omitempty" jsonschema:"Milestone ID"`
	Labels      []string             `json:"labels,omitempty" jsonschema:"Board scope labels"`
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
	if len(input.Labels) > 0 {
		lbls := gl.LabelOptions(input.Labels)
		opts.Labels = &lbls
	}
	if input.Weight != 0 {
		opts.Weight = new(input.Weight)
	}
	// Raw REST fetch so the documented hide_backlog_list/hide_closed_list/
	// assignee/weight fields (absent from client-go's gl.GroupIssueBoard) are
	// surfaced on the updated board.
	board, _, err := rawUpdateGroupBoard(ctx, client, string(input.GroupID), input.BoardID, opts)
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
	return convertGroupBoardAPI(board), nil
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
	OrderBy string               `json:"order_by,omitempty" jsonschema:"Column to order results by (keyset pagination)"`
	Sort    string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListGroupBoardLists lists all lists in a group board.
func ListGroupBoardLists(ctx context.Context, client *gitlabclient.Client, input ListGroupBoardListsInput) (ListBoardListsOutput, error) {
	if input.GroupID == "" {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("group_board_list_list", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("group_board_list_list", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.ListGroupIssueBoardListsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
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
	// client-go v2.58 declares []*BoardList for the group-level list update,
	// but GitLab returns the single updated list object, so the wrapper can
	// never unmarshal a successful response. The request is issued directly
	// until the upstream signature is fixed (the project-level equivalent
	// already returns *BoardList).
	path := fmt.Sprintf("groups/%s/boards/%d/lists/%d", gl.PathEscape(string(input.GroupID)), input.BoardID, input.ListID)
	req, err := client.GL().NewRequest(http.MethodPut, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithMessage("group_board_list_update", err)
	}
	var list gl.BoardList
	if _, err = client.GL().Do(req, &list); err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_update", err, http.StatusNotFound,
			"list_id not found on this board (only the position can be updated; recreate the list to change its scope)")
	}
	return convertBoardList(&list), nil
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
