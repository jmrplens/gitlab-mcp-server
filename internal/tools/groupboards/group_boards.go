// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupboards

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared output types
// ---------------------------------------------------------------------------.

// GroupBoardOutput represents a GitLab group issue board. Nested objects
// (group, milestone, assignee, labels, lists) mirror the documented
// group_boards.md response sub-objects (see shapes.go). The hide_backlog_list,
// hide_closed_list, assignee and weight fields are documented by the API but
// absent from client-go's gl.GroupIssueBoard struct, so every handler that
// answers with a board reads them off the captured answer
// ([toolutil.GroupBoardExtra]) and they stay version-tolerant (absent on older
// instances → zero value, omitted from the envelope).
type GroupBoardOutput struct {
	toolutil.HintableOutput
	ID              int64                 `json:"id"`
	Name            string                `json:"name"`
	Group           *GroupRefOutput       `json:"group,omitempty"`
	Milestone       *MilestoneOutput      `json:"milestone,omitempty" tier:"premium"`
	Assignee        *BasicUserOutput      `json:"assignee,omitempty" tier:"premium"`
	Weight          *int64                `json:"weight,omitempty" tier:"premium"`
	Labels          []*LabelDetailsOutput `json:"labels,omitempty"`
	HideBacklogList bool                  `json:"hide_backlog_list"`
	HideClosedList  bool                  `json:"hide_closed_list"`
	Lists           []BoardListOutput     `json:"lists,omitempty"`
}

// BoardListOutput represents a single list within a group board. Nested objects
// (assignee, label, iteration, milestone) mirror the full client-go gl.BoardList
// sub-objects per the 1:1 audit policy.
//
// LimitMetric, MaxIssueCount and MaxIssueWeight are the list's
// work-in-progress limits, which ee/lib/ee/api/entities/list.rb exposes only
// where they are available (the group's wip_limits licensed feature,
// Premium), so a Free schema does not advertise them.
type BoardListOutput struct {
	toolutil.HintableOutput
	ID             int64                    `json:"id"`
	Assignee       *BoardListAssigneeOutput `json:"assignee,omitempty" tier:"premium"`
	Iteration      *IterationOutput         `json:"iteration,omitempty" tier:"premium"`
	Label          *LabelOutput             `json:"label,omitempty"`
	LimitMetric    string                   `json:"limit_metric,omitempty" tier:"premium"`
	MaxIssueCount  int64                    `json:"max_issue_count,omitempty" tier:"premium"`
	MaxIssueWeight int64                    `json:"max_issue_weight,omitempty" tier:"premium"`
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
// The update's request of its own
// ---------------------------------------------------------------------------.

// newRawRequest builds the request of the one handler that bypasses its
// client-go wrapper, the board update. It is a package variable so tests can
// exercise the request-construction failure branch, which no valid input
// reaches at runtime (PathEscape sanitizes everything interpolated into the
// path).
var newRawRequest = func(ctx context.Context, client *gitlabclient.Client, method, path string, opts any) (*retryablehttp.Request, error) {
	return client.GL().NewRequest(method, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
}

// updateGroupBoardBody is the body of a group board update: every field
// client-go's UpdateGroupIssueBoardOptions models, and the two list switches
// GitLab's route takes on every tier and that struct does not (register row 98
// of docs/development/upstream-bugs.md). A switch left nil is left out, so
// GitLab keeps that list as it is. The type retires when a client-go release
// models the switches.
type updateGroupBoardBody struct {
	gl.UpdateGroupIssueBoardOptions
	HideBacklogList *bool `url:"hide_backlog_list,omitempty" json:"hide_backlog_list,omitempty"`
	HideClosedList  *bool `url:"hide_closed_list,omitempty" json:"hide_closed_list,omitempty"`
}

// rawUpdateGroupBoard issues a raw REST PUT updating a group issue board with
// the body [updateGroupBoardBody] carries, decoding the answer into
// client-go's gl.GroupIssueBoard as its UpdateIssueBoard would. What that
// struct does not model is read by the caller off the captured answer, like
// every other group board answer, so ctx is the context the capture was taken
// under.
func rawUpdateGroupBoard(ctx context.Context, client *gitlabclient.Client, groupID string, boardID int64, opts *updateGroupBoardBody) (*gl.GroupIssueBoard, error) {
	path := fmt.Sprintf("groups/%s/boards/%d", gl.PathEscape(groupID), boardID)
	req, err := newRawRequest(ctx, client, http.MethodPut, path, opts)
	if err != nil {
		return nil, err
	}
	board := new(gl.GroupIssueBoard)
	_, err = client.GL().Do(req, board)
	return board, err
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// convertGroupBoard maps a GitLab group issue board into MCP output. The
// group, milestone, labels and columns come from client-go's decode; the
// documented hide_backlog_list, hide_closed_list, assignee and weight fields
// gl.GroupIssueBoard omits, and each column's limit_metric, come from what was
// read off the captured answer beside it. The columns are joined to what was
// read beside them by position, since both decode the same bytes; a column
// the captured board holds nothing for is published without a metric rather
// than with another column's.
func convertGroupBoard(b *gl.GroupIssueBoard, extra toolutil.GroupBoardExtra) GroupBoardOutput {
	out := GroupBoardOutput{
		ID:              b.ID,
		Name:            b.Name,
		Group:           groupRefOutput(b.Group),
		Milestone:       milestoneOutput(b.Milestone),
		Assignee:        extra.Assignee,
		Weight:          extra.Weight,
		Labels:          groupLabelOutputs(b.Labels),
		HideBacklogList: extra.HideBacklogList,
		HideClosedList:  extra.HideClosedList,
	}
	for i, l := range b.Lists {
		var listExtra toolutil.BoardListExtra
		if i < len(extra.Lists) {
			listExtra = extra.Lists[i]
		}
		out.Lists = append(out.Lists, convertBoardList(l, listExtra))
	}
	return out
}

// convertBoardList maps a GitLab board list into group board MCP output,
// filling from the decoded list and from what was read beside it.
func convertBoardList(l *gl.BoardList, extra toolutil.BoardListExtra) BoardListOutput {
	return BoardListOutput{
		ID:             l.ID,
		Assignee:       boardListAssigneeOutput(l.Assignee),
		Iteration:      iterationOutput(l.Iteration),
		Label:          labelOutput(l.Label),
		LimitMetric:    extra.LimitMetric,
		MaxIssueCount:  l.MaxIssueCount,
		MaxIssueWeight: l.MaxIssueWeight,
		Milestone:      milestoneOutput(l.Milestone),
		Position:       l.Position,
	}
}

// ---------------------------------------------------------------------------
// Refusals
// ---------------------------------------------------------------------------.

// The hints a permission refusal of a group board write carries, written from
// what GitLab answers at v19.4.1-ee (lib/api/group_boards.rb,
// ee/lib/ee/api/group_boards.rb, lib/api/boards_responses.rb and the roles
// under config/authz/roles). Every write is authorized on admin_issue_board or
// admin_issue_board_list, which config/authz/roles/planner.yml grants on a
// group and GroupPolicy withholds from every role while the group or an
// ancestor is archived. Creating and deleting a board then call forbidden!
// unless the group has multiple group issue boards, a Premium feature, with
// the same plain 403, so those two name the license beside the role. Nothing
// else checks one, and GitLab deletes a group's last board like any other.
const (
	groupBoardCreateHint = "creating a group issue board through the API needs GitLab Premium or Ultimate as well as at least the Planner role on the group, and GitLab refuses it to every role while the group or one of its parent groups is archived"
	groupBoardUpdateHint = "updating a group issue board needs at least the Planner role on the group, and GitLab refuses it to every role while the group or one of its parent groups is archived"
	groupBoardDeleteHint = "deleting a group issue board through the API needs GitLab Premium or Ultimate as well as at least the Planner role on the group, and GitLab refuses it to every role while the group or one of its parent groups is archived"
	groupBoardListsHint  = "managing the lists of a group issue board needs at least the Planner role on the group, and GitLab refuses it to every role while the group or one of its parent groups is archived"
)

// groupBoardRefusal wraps GitLab's refusal of a group board write: a
// permission refusal, the plain 401 or 403 [toolutil.IsPermissionRefusal]
// recognizes, carries permissionHint, a 404 carries notFoundHint, and
// anything else GitLab's own message. The permission hint is keyed on the
// refusal rather than on a status, since a 403 naming an RFC 6750 code
// refuses the token's scope rather than the caller's role or license, and
// some routes answer a missing permission with 401.
func groupBoardRefusal(operation string, err error, permissionHint, notFoundHint string) error {
	if toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint(operation, err, permissionHint)
	}
	return toolutil.WrapErrWithStatusHint(operation, err, http.StatusNotFound, notFoundHint)
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	boards, resp, err := client.GL().GroupIssueBoards.ListGroupIssueBoards(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		// No license or role hint: GET /groups/:id/boards checks no license,
		// and read_issue_board, which it is authorized on, is granted to
		// everyone who can read the group, so a group the caller cannot read
		// is the 404 below.
		return ListGroupBoardsOutput{}, toolutil.WrapErrWithStatusHint("group_board_list", err, http.StatusNotFound,
			"verify the group exists with group.get")
	}
	extras, err := toolutil.CapturedGroupBoards(captured, len(boards))
	if err != nil {
		return ListGroupBoardsOutput{}, toolutil.WrapErr("group_board_list", err)
	}
	out := ListGroupBoardsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for i, b := range boards {
		out.Boards = append(out.Boards, convertGroupBoard(b, extras[i]))
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	board, _, err := client.GL().GroupIssueBoards.GetGroupIssueBoard(string(input.GroupID), input.BoardID, gl.WithContext(ctx))
	if err != nil {
		return GroupBoardOutput{}, toolutil.WrapErrWithStatusHint("group_board_get", err, http.StatusNotFound,
			"board_id not found on this group. Use group.group_board_list to discover current board IDs")
	}
	extra, err := toolutil.CapturedGroupBoard(captured)
	if err != nil {
		return GroupBoardOutput{}, toolutil.WrapErr("group_board_get", err)
	}
	return convertGroupBoard(board, extra), nil
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	board, _, err := client.GL().GroupIssueBoards.CreateGroupIssueBoard(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		// A 400 is Grape's for a name left out, which the check above already
		// refuses, so it is reported in GitLab's words: board names need not be
		// unique, and this input names no milestone, iteration or label.
		return GroupBoardOutput{}, groupBoardRefusal("group_board_create", err, groupBoardCreateHint,
			"verify the group exists with group.get")
	}
	extra, err := toolutil.CapturedGroupBoard(captured)
	if err != nil {
		return GroupBoardOutput{}, toolutil.WrapErr("group_board_create", err)
	}
	return convertGroupBoard(board, extra), nil
}

// UpdateGroupBoardInput represents input for updating a group board. The four
// scope fields are tagged Premium because doc/api/group_boards.md marks them
// "Premium and Ultimate only": a Free instance updates the board and leaves
// the scope out, so the Free schema does not offer them. Weight is a pointer
// because 0 is a scope of its own: GitLab reads only nil and -1 as no weight
// scope (EMPTY_SCOPE_STATE in ee/app/models/ee/board.rb), and an issue created
// on a board scoped to 0 gets weight 0. MilestoneID is a pointer for the same
// reason: 0 is the predefined No Milestone scope and -1 the empty one
// (Timebox::None and Timebox::Any). AssigneeID is a pointer because GitLab
// looks the assignee up by its ID and clears the scope when no user has it,
// which 0 never matches (issue 1306).
//
// HideBacklogList and HideClosedList carry no tier tag, since GitLab takes
// both on every tier (update_params_ce in lib/api/boards_responses.rb), and are
// pointers because false shows a list again and has to reach GitLab rather
// than be read as unset. client-go's UpdateGroupIssueBoardOptions models
// neither (register row 98 of docs/development/upstream-bugs.md), so they go
// out in [updateGroupBoardBody] (issue 1241).
type UpdateGroupBoardInput struct {
	GroupID         toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or path,required"`
	BoardID         int64                `json:"board_id" jsonschema:"Board ID,required"`
	Name            string               `json:"name,omitempty" jsonschema:"Board name"`
	AssigneeID      *int64               `json:"assignee_id,omitempty" tier:"premium" jsonschema:"User ID of the assignee the board is scoped to. Pass 0 to remove the assignee from the scope. Omit the field to leave it untouched"`
	MilestoneID     *int64               `json:"milestone_id,omitempty" tier:"premium" jsonschema:"Milestone ID the board is scoped to: 0 for issues with no milestone, -1 to remove the milestone from the scope, -2 for upcoming and -3 for started milestones. Omit the field to leave it untouched"`
	Labels          []string             `json:"labels,omitempty" tier:"premium" jsonschema:"Label names the board is scoped to. Pass [] to remove every label from the scope. Omit the field to leave it untouched"`
	Weight          *int64               `json:"weight,omitempty" tier:"premium" jsonschema:"Weight from 0 to 9 the board is scoped to"`
	HideBacklogList *bool                `json:"hide_backlog_list,omitempty" jsonschema:"Hide the board's Open list (true) or show it (false). Omit the field to leave it untouched"`
	HideClosedList  *bool                `json:"hide_closed_list,omitempty" jsonschema:"Hide the board's Closed list (true) or show it (false). Omit the field to leave it untouched"`
}

// UpdateGroupBoard updates a group issue board.
func UpdateGroupBoard(ctx context.Context, client *gitlabclient.Client, input UpdateGroupBoardInput) (GroupBoardOutput, error) {
	if input.GroupID == "" {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_update", toolutil.ErrFieldRequired("group_id"))
	}
	if input.BoardID == 0 {
		return GroupBoardOutput{}, toolutil.WrapErrWithMessage("group_board_update", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &updateGroupBoardBody{HideBacklogList: input.HideBacklogList, HideClosedList: input.HideClosedList}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	opts.AssigneeID = input.AssigneeID
	opts.MilestoneID = input.MilestoneID
	// Presence decides, never emptiness: [] removes every label from the scope
	// (issue 1306).
	if input.Labels != nil {
		lbls := gl.LabelOptions(input.Labels)
		opts.Labels = &lbls
	}
	if input.Weight != nil {
		opts.Weight = input.Weight
	}
	// A request of its own so the two list switches reach GitLab, read like
	// every other group board answer: what gl.GroupIssueBoard does not model
	// comes off the captured answer.
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	board, err := rawUpdateGroupBoard(ctx, client, string(input.GroupID), input.BoardID, opts)
	if err != nil {
		// Nothing this input sends is refused, so no hint is written for one
		// and anything but a permission refusal or a 404 is reported in
		// GitLab's words. update_board in API::BoardsResponses answers 400
		// only when the board it checks does not validate, and the board it
		// checks is not the one it updated: its board helper runs
		// board_parent.boards.find on every call, so the check and the answer
		// each load the stored board again. A name the update cannot save
		// (over 255 characters) is therefore answered 200 with the board
		// GitLab kept and the rename dropped (register row 94 of
		// docs/development/upstream-bugs.md). The scope is never refused
		// either: Boards::UpdateService drops a milestone the group cannot
		// use, clears the assignee for an id no user has, creates a label for
		// a title the group does not have, and drops all of them where scoped
		// boards are not licensed.
		return GroupBoardOutput{}, groupBoardRefusal("group_board_update", err, groupBoardUpdateHint,
			"board_id not found. Use group.group_board_list to verify")
	}
	extra, err := toolutil.CapturedGroupBoard(captured)
	if err != nil {
		return GroupBoardOutput{}, toolutil.WrapErr("group_board_update", err)
	}
	return convertGroupBoard(board, extra), nil
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
		return groupBoardRefusal("group_board_delete", err, groupBoardDeleteHint,
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	lists, resp, err := client.GL().GroupIssueBoards.ListGroupIssueBoardLists(string(input.GroupID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListBoardListsOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_list", err, http.StatusNotFound,
			"board_id not found on this group. Use group.group_board_list to discover board IDs")
	}
	extras, err := toolutil.CapturedBoardLists(captured, len(lists))
	if err != nil {
		return ListBoardListsOutput{}, toolutil.WrapErr("group_board_list_list", err)
	}
	out := ListBoardListsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for i, l := range lists {
		out.Lists = append(out.Lists, convertBoardList(l, extras[i]))
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	list, _, err := client.GL().GroupIssueBoards.GetGroupIssueBoardList(string(input.GroupID), input.BoardID, input.ListID, gl.WithContext(ctx))
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErrWithStatusHint("group_board_list_get", err, http.StatusNotFound,
			"list_id not found on this board. Use group.group_board_list_lists to discover list IDs (each list represents a label column)")
	}
	extra, err := toolutil.CapturedBoardList(captured)
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErr("group_board_list_get", err)
	}
	return convertBoardList(list, extra), nil
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	list, _, err := client.GL().GroupIssueBoards.CreateGroupIssueBoardList(string(input.GroupID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		return BoardListOutput{}, createGroupBoardListError(err)
	}
	extra, err := toolutil.CapturedBoardList(captured)
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErr("group_board_list_create", err)
	}
	return convertBoardList(list, extra), nil
}

// createGroupBoardListError wraps GitLab's refusal of a column creation with
// the hint its cause calls for, in terms of the one parameter the input
// offers. GitLab's route also takes assignee_id, milestone_id and iteration_id
// on a licensed instance, but client-go's CreateGroupIssueBoardListOptions
// carries label_id alone, so neither hint may send a model after the others.
//
// What GitLab answers, read at v19.4.1-ee: a label this group cannot use and a
// second list for a label the board already has a list for are both a 400
// (API::BoardsResponses#create_list renders the service's first error,
// "Label not found" or "Label has already been taken", with that status), and
// a license the instance lacks is a 400 too, for the other list types only. A
// caller without admin_issue_board_list on the group is refused by authorize!
// with a 403: every role from Planner up holds it, and an archived group, or
// one under an archived parent, withholds it from all of them. That refusal is
// keyed on [toolutil.IsPermissionRefusal] rather than on the status, since a
// 403 naming an RFC 6750 code refuses the token's scope rather than the
// caller's role, and some routes answer a missing permission with 401.
func createGroupBoardListError(err error) error {
	if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return toolutil.WrapErrWithHint("group_board_list_create", err,
			"label_id must name a label of this group or of one of its parent groups (group.group_label_list with include_ancestor_groups lists them, and a project label is answered as not found), and a board holds one list per label, so GitLab answers that the label has already been taken when this board already has a list for that label (group.group_board_list_lists shows them)")
	}
	if toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint("group_board_list_create", err, groupBoardListsHint)
	}
	return toolutil.WrapErrWithMessage("group_board_list_create", err)
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	list, _, err := client.GL().GroupIssueBoards.UpdateIssueBoardList(
		string(input.GroupID), input.BoardID, input.ListID, opts, gl.WithContext(ctx),
	)
	if err != nil {
		return BoardListOutput{}, groupBoardRefusal("group_board_list_update", err, groupBoardListsHint,
			"list_id not found on this board (only the position can be updated; recreate the list to change its scope)")
	}
	extra, err := toolutil.CapturedBoardList(captured)
	if err != nil {
		return BoardListOutput{}, toolutil.WrapErr("group_board_list_update", err)
	}
	return convertBoardList(list, extra), nil
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
		return groupBoardRefusal("group_board_list_delete", err, groupBoardListsHint,
			"list_id already deleted or never existed on this board")
	}
	return nil
}
