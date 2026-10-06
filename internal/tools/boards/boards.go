package boards

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

// BoardOutput represents a GitLab issue board. Sub-objects (project, milestone,
// assignee, labels, lists) mirror the client-go gl.IssueBoard struct field for
// field on their canonical json keys (1:1 audit policy: full nested objects).
type BoardOutput struct {
	toolutil.HintableOutput
	ID              int64                 `json:"id"`
	Name            string                `json:"name"`
	Project         *ProjectOutput        `json:"project,omitempty"`
	Milestone       *MilestoneOutput      `json:"milestone,omitempty" tier:"premium"`
	Assignee        *BasicUserOutput      `json:"assignee,omitempty" tier:"premium"`
	Weight          int64                 `json:"weight,omitempty" tier:"premium"`
	Labels          []*LabelDetailsOutput `json:"labels,omitempty"`
	HideBacklogList bool                  `json:"hide_backlog_list"`
	HideClosedList  bool                  `json:"hide_closed_list"`
	Lists           []BoardListOutput     `json:"lists,omitempty"`
	// Group is the group reference GitLab renders on a board, null on one that
	// belongs to a project.
	Group *toolutil.BasicGroupDetailsOutput `json:"group,omitempty"`
}

// BoardListOutput represents a single list within a board. Sub-objects (label,
// assignee, milestone, iteration) mirror the client-go gl.BoardList struct on
// their canonical json keys (1:1 audit policy: full nested objects).
type BoardListOutput struct {
	toolutil.HintableOutput
	ID             int64                    `json:"id"`
	Label          *LabelOutput             `json:"label,omitempty"`
	Assignee       *BoardListAssigneeOutput `json:"assignee,omitempty" tier:"premium"`
	Milestone      *MilestoneOutput         `json:"milestone,omitempty" tier:"premium"`
	Iteration      *IterationOutput         `json:"iteration,omitempty" tier:"premium"`
	Position       int64                    `json:"position"`
	MaxIssueCount  int64                    `json:"max_issue_count,omitempty"`
	MaxIssueWeight int64                    `json:"max_issue_weight,omitempty"`
	// LimitMetric is the documented REST `limit_metric` field on each board list
	// (all_metrics / issue_count / issue_weights, or null). The client-go
	// gl.BoardList struct omits it, so it is decoded via the raw-superset fetch
	// path (boardListAPI) used by the board get / list-board-lists read handlers.
	LimitMetric string `json:"limit_metric,omitempty"`
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
// Raw REST superset types
// ---------------------------------------------------------------------------.

// boardListAPI is a raw-fetch superset over client-go's gl.BoardList. It embeds
// the SDK struct (so every SDK-known field — id/label/assignee/milestone/
// iteration/position/max_issue_*) still decodes through the documented json
// keys) and adds the documented `limit_metric` field that gl.BoardList omits.
// Decoding is single-pass and naturally version-tolerant: when limit_metric is
// absent from the response, LimitMetric stays the zero value and is omitted from
// the MCP envelope.
type boardListAPI struct {
	gl.BoardList
	LimitMetric string `json:"limit_metric"`
}

// issueBoardAPI is a raw-fetch superset over client-go's gl.IssueBoard. It
// embeds the SDK struct and shadows the `lists` key with the boardListAPI
// superset so each list entry's documented limit_metric is captured; the
// shallower (outer) Lists field wins over the embedded gl.IssueBoard.Lists
// during json decoding.
type issueBoardAPI struct {
	gl.IssueBoard
	Lists []*boardListAPI                   `json:"lists"`
	Group *toolutil.BasicGroupDetailsOutput `json:"group"`
}

// newRawRequest builds a raw API request for the two reads that decode the
// documented limit_metric client-go's gl.BoardList omits. It is a package
// variable so tests can exercise the request-construction failure branches,
// which no valid input reaches at runtime (PathEscape sanitizes everything
// interpolated into the path).
var newRawRequest = func(ctx context.Context, client *gitlabclient.Client, method, path string, opts any) (*retryablehttp.Request, error) {
	return client.GL().NewRequest(method, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
}

// rawGetBoard issues a raw REST GET for a single issue board, decoding the full
// documented response (including each list's SDK-missing limit_metric) into an
// [issueBoardAPI].
func rawGetBoard(ctx context.Context, client *gitlabclient.Client, projectID string, boardID int64) (*issueBoardAPI, *gl.Response, error) {
	path := fmt.Sprintf("projects/%s/boards/%d", gl.PathEscape(projectID), boardID)
	req, err := newRawRequest(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var board issueBoardAPI
	resp, err := client.GL().Do(req, &board)
	return &board, resp, err
}

// rawListBoardLists issues a raw REST GET for a board's lists, decoding the full
// documented response (including each list's SDK-missing limit_metric) into a
// slice of [boardListAPI]. The supplied opts encode pagination via their url
// struct tags, and the returned gl.Response preserves pagination headers for
// [toolutil.PaginationFromResponse].
func rawListBoardLists(ctx context.Context, client *gitlabclient.Client, projectID string, boardID int64, opts *gl.GetIssueBoardListsOptions) ([]*boardListAPI, *gl.Response, error) {
	path := fmt.Sprintf("projects/%s/boards/%d/lists", gl.PathEscape(projectID), boardID)
	req, err := newRawRequest(ctx, client, http.MethodGet, path, opts)
	if err != nil {
		return nil, nil, err
	}
	var lists []*boardListAPI
	resp, err := client.GL().Do(req, &lists)
	return lists, resp, err
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// convertBoard maps a GitLab project issue board into the MCP output shape,
// surfacing the full project/milestone/assignee/label sub-objects.
func convertBoard(b *gl.IssueBoard, extra toolutil.BoardExtra) BoardOutput {
	out := BoardOutput{
		ID:              b.ID,
		Name:            b.Name,
		Project:         projectOutput(b.Project),
		Milestone:       milestoneOutput(b.Milestone),
		Assignee:        basicUserOutput(b.Assignee),
		Weight:          b.Weight,
		Labels:          labelDetailsOutputs(b.Labels),
		HideBacklogList: b.HideBacklogList,
		HideClosedList:  b.HideClosedList,
		Group:           extra.Group,
	}
	for _, l := range b.Lists {
		out.Lists = append(out.Lists, convertBoardList(l))
	}
	return out
}

// convertBoardAPI maps a raw-fetch issue board (issueBoardAPI superset) into the
// MCP output shape, preserving each list's documented limit_metric.
func convertBoardAPI(b *issueBoardAPI) BoardOutput {
	out := BoardOutput{
		ID:              b.ID,
		Name:            b.Name,
		Project:         projectOutput(b.Project),
		Milestone:       milestoneOutput(b.Milestone),
		Assignee:        basicUserOutput(b.Assignee),
		Weight:          b.Weight,
		Labels:          labelDetailsOutputs(b.Labels),
		HideBacklogList: b.HideBacklogList,
		HideClosedList:  b.HideClosedList,
		Group:           b.Group,
	}
	for _, l := range b.Lists {
		out.Lists = append(out.Lists, convertBoardListAPI(l))
	}
	return out
}

// convertBoardList maps a GitLab board list into the MCP output shape,
// surfacing the full label/assignee/milestone/iteration sub-objects.
func convertBoardList(l *gl.BoardList) BoardListOutput {
	return BoardListOutput{
		ID:             l.ID,
		Label:          labelOutput(l.Label),
		Assignee:       boardListAssigneeOutput(l.Assignee),
		Milestone:      milestoneOutput(l.Milestone),
		Iteration:      iterationOutput(l.Iteration),
		Position:       l.Position,
		MaxIssueCount:  l.MaxIssueCount,
		MaxIssueWeight: l.MaxIssueWeight,
	}
}

// convertBoardListAPI maps a raw-fetch board list (boardListAPI superset) into
// the MCP output shape, additionally surfacing the documented limit_metric the
// client-go gl.BoardList struct omits.
func convertBoardListAPI(l *boardListAPI) BoardListOutput {
	out := convertBoardList(&l.BoardList)
	out.LimitMetric = l.LimitMetric
	return out
}

// applyOrderSort copies the keyset order_by/sort parameters onto a
// gl.ListOptions, setting only the values the caller supplied.
func applyOrderSort(opts *gl.ListOptions, orderBy, sort string) {
	if orderBy != "" {
		opts.OrderBy = orderBy
	}
	if sort != "" {
		opts.Sort = sort
	}
}

// ---------------------------------------------------------------------------
// Refusals
// ---------------------------------------------------------------------------.

// The hints a refusal of a project board route carries, written from what
// GitLab answers at v19.4.1-ee (lib/api/boards.rb, lib/api/boards_responses.rb,
// the EE list create service and the roles under config/authz/roles).
const (
	// boardsAccessHint follows a permission refusal of a read. The four reads
	// are authorized on read_issue_board, which every role from Guest holds and
	// which ProjectPolicy withholds while the project's issues feature is
	// disabled, or limited to project members and the caller is not one.
	boardsAccessHint = "a project's issue boards follow its issues feature, which GitLab refuses while issues are disabled in the project, or limited to project members and the caller is not one"

	// boardsManageHint follows a permission refusal of a write. Board writes
	// are authorized on admin_issue_board and list writes on
	// admin_issue_board_list, which config/authz/roles/planner.yml grants, and
	// which ProjectPolicy withholds from every role while the issues feature
	// is disabled or the project or an ancestor group is archived. No license
	// is checked on any of them: a project may have several boards on every
	// tier (Project#multiple_issue_boards_available? is true), and GitLab
	// deletes a project's last board like any other.
	boardsManageHint = "managing a project's issue boards and their lists needs at least the Planner role on the project, and GitLab refuses it to every role while the project's issues feature is disabled or the project or one of its parent groups is archived"

	// listTypeLicenseHint follows the 400 an Enterprise instance answers a
	// list type its license or the project's plan lacks with
	// (EE::Boards::Lists::CreateService#license_validation_error).
	listTypeLicenseHint = "assignee, milestone and iteration lists need GitLab Premium or Ultimate, which the license or plan this project is on does not include, so only a label list (label_id) can be created on its boards"

	// listScopeHint follows every other 400 of a list creation: Grape's own
	// parameter rules (a Community Edition route requires label_id, an
	// Enterprise one takes exactly one of the four), a target the project
	// cannot use ("Label not found") and a second list for one the board
	// already has ("Label has already been taken").
	listScopeHint = "exactly one of label_id, assignee_id, milestone_id or iteration_id must be set, and a Community Edition instance takes label_id alone. The id must name a label, milestone or iteration this project can use (its own or a parent group's, see project.label_list and project.milestone_list) or an existing user, and a board holds one list per label, assignee, milestone or iteration, so GitLab answers that it has already been taken when this board has that list already (project.board_list_list shows them)"

	// listTypeLicenseClause follows listScopeHint on a 400 refusing an
	// assignee, milestone or iteration list in words other than the English
	// license refusal. GitLab writes that refusal in the caller's preferred
	// language (API::Helpers#current_user sets Gitlab::I18n.locale from it,
	// and the message is built with _()), so listTypeLicenseRefusal matches
	// it only for a caller whose GitLab speaks English, and a licensed list
	// type refused in any other language is told about the license here.
	listTypeLicenseClause = "On an instance whose license or plan does not include them, assignee, milestone and iteration lists are refused with a 400 as well, in the language set in the caller's GitLab preferences, and only a label list (label_id) can be created there"
)

// listTypeLicenseRefusal is the fixed part of the message GitLab refuses a
// list type the license lacks with ("Assignee lists not available with your
// current license", and the same for milestone, iteration and status lists)
// when the caller's preferred language is English.
const listTypeLicenseRefusal = "lists not available with your current license"

// boardRefusal wraps GitLab's refusal of a project board route: a permission
// refusal, the plain 401 or 403 [toolutil.IsPermissionRefusal] recognizes,
// carries permissionHint, a 404 carries notFoundHint, and anything else
// GitLab's own message. The permission hint is keyed on the refusal rather
// than on a status, since a 403 naming an RFC 6750 code refuses the token's
// scope rather than the caller's role, and some routes answer a missing
// permission with 401.
func boardRefusal(operation string, err error, permissionHint, notFoundHint string) error {
	if toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint(operation, err, permissionHint)
	}
	return toolutil.WrapErrWithStatusHint(operation, err, http.StatusNotFound, notFoundHint)
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Board CRUD handlers
// ---------------------------------------------------------------------------.

// ListBoardsInput represents input for listing project issue boards.
// OrderBy/Sort/pagination/page_token map onto the embedded gl.ListOptions to
// mirror the SDK's keyset-capable list options.
type ListBoardsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset pagination (e.g. id, created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListBoards lists all issue boards for a project.
func ListBoards(ctx context.Context, client *gitlabclient.Client, input ListBoardsInput) (ListBoardsOutput, error) {
	if input.ProjectID == "" {
		return ListBoardsOutput{}, toolutil.WrapErrWithMessage("board_list", toolutil.ErrFieldRequired("project_id"))
	}
	opts := &gl.ListIssueBoardsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	boards, resp, err := client.GL().Boards.ListIssueBoards(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListBoardsOutput{}, boardRefusal("board_list", err, boardsAccessHint,
			"verify the project exists with project.get")
	}
	extras, err := toolutil.CapturedBoards(captured, len(boards))
	if err != nil {
		return ListBoardsOutput{}, toolutil.WrapErr("board_list", err)
	}
	out := ListBoardsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for i, b := range boards {
		out.Boards = append(out.Boards, convertBoard(b, extras[i]))
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
	// Raw REST fetch so the documented limit_metric on each list (absent from
	// client-go's gl.BoardList) is surfaced.
	board, _, err := rawGetBoard(ctx, client, string(input.ProjectID), input.BoardID)
	if err != nil {
		return BoardOutput{}, boardRefusal("board_get", err, boardsAccessHint,
			"verify board_id with project.board_list. board_id is the global board ID, not an IID")
	}
	return convertBoardAPI(board), nil
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	board, _, err := client.GL().Boards.CreateIssueBoard(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return BoardOutput{}, boardRefusal("board_create", err, boardsManageHint,
			"verify the project exists with project.get")
	}
	extra, err := toolutil.CapturedBoard(captured)
	if err != nil {
		return BoardOutput{}, toolutil.WrapErr("board_create", err)
	}
	return convertBoard(board, extra), nil
}

// UpdateBoardInput represents input for updating a board.
type UpdateBoardInput struct {
	ProjectID       toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or path,required"`
	BoardID         int64                `json:"board_id" jsonschema:"Board ID,required"`
	Name            string               `json:"name,omitempty" jsonschema:"Board name"`
	AssigneeID      int64                `json:"assignee_id,omitempty" jsonschema:"Assignee user ID"`
	MilestoneID     int64                `json:"milestone_id,omitempty" jsonschema:"Milestone ID"`
	Labels          []string             `json:"labels,omitempty" jsonschema:"Board scope label names"`
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
	if len(input.Labels) > 0 {
		lbls := gl.LabelOptions(input.Labels)
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	board, _, err := client.GL().Boards.UpdateIssueBoard(string(input.ProjectID), input.BoardID, opts, gl.WithContext(ctx))
	if err != nil {
		return BoardOutput{}, boardRefusal("board_update", err, boardsManageHint,
			"verify board_id with project.board_list")
	}
	extra, err := toolutil.CapturedBoard(captured)
	if err != nil {
		return BoardOutput{}, toolutil.WrapErr("board_update", err)
	}
	return convertBoard(board, extra), nil
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
		return boardRefusal("board_delete", err, boardsManageHint,
			"verify board_id with project.board_list")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Board List CRUD handlers
// ---------------------------------------------------------------------------.

// ListBoardListsInput represents input for listing board lists.
// OrderBy/Sort/pagination/page_token map onto the embedded gl.ListOptions to
// mirror the SDK's keyset-capable list options.
type ListBoardListsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id"         jsonschema:"Project ID or path,required"`
	BoardID   int64                `json:"board_id"           jsonschema:"Board ID,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order results by for keyset pagination (e.g. id, created_at, updated_at)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListBoardLists lists all lists in a board.
func ListBoardLists(ctx context.Context, client *gitlabclient.Client, input ListBoardListsInput) (ListBoardListsOutput, error) {
	if input.ProjectID == "" {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("board_list_list", toolutil.ErrFieldRequired("project_id"))
	}
	if input.BoardID == 0 {
		return ListBoardListsOutput{}, toolutil.WrapErrWithMessage("board_list_list", toolutil.ErrFieldRequired("board_id"))
	}
	opts := &gl.GetIssueBoardListsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	applyOrderSort(&opts.ListOptions, input.OrderBy, input.Sort)
	// Raw REST fetch so the documented limit_metric on each list (absent from
	// client-go's gl.BoardList) is surfaced.
	lists, resp, err := rawListBoardLists(ctx, client, string(input.ProjectID), input.BoardID, opts)
	if err != nil {
		return ListBoardListsOutput{}, boardRefusal("board_list_list", err, boardsAccessHint,
			"verify project_id and board_id with project.board_list")
	}
	out := ListBoardListsOutput{Pagination: toolutil.PaginationFromResponse(resp)}
	for _, l := range lists {
		out.Lists = append(out.Lists, convertBoardListAPI(l))
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
		return BoardListOutput{}, boardRefusal("board_list_get", err, boardsAccessHint,
			"verify board_id and list_id with project.board_list_list")
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
		licensedType := input.AssigneeID != 0 || input.MilestoneID != 0 || input.IterationID != 0
		return BoardListOutput{}, createBoardListError(err, licensedType)
	}
	return convertBoardList(list), nil
}

// createBoardListError wraps GitLab's refusal of a column creation with the
// hint its cause calls for. Every refusal of what was sent is a 400:
// API::BoardsResponses#create_list renders the service's first error with that
// status, and Grape answers its own parameter rules with it. A list type the
// license or the project's plan lacks is one of those 400s, so the license is
// named on a 400 and never on a 403, which this route answers only for a
// caller without admin_issue_board_list. GitLab's English message alone says
// for certain that the license refused it; GitLab translates that message,
// so a 400 refusing an assignee, milestone or iteration list (licensedType)
// in any other words names the license beside the scope, and a 400 refusing
// a label list, which no license gates, names none.
func createBoardListError(err error, licensedType bool) error {
	if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		if toolutil.ContainsAny(err, listTypeLicenseRefusal) {
			return toolutil.WrapErrWithHint("board_list_create", err, listTypeLicenseHint)
		}
		if licensedType {
			return toolutil.WrapErrWithHint("board_list_create", err, listScopeHint+". "+listTypeLicenseClause)
		}
		return toolutil.WrapErrWithHint("board_list_create", err, listScopeHint)
	}
	return boardRefusal("board_list_create", err, boardsManageHint,
		"verify project_id and board_id with project.board_list")
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
		return BoardListOutput{}, boardRefusal("board_list_update", err, boardsManageHint,
			"verify board_id and list_id with project.board_list_list. Position is 0-based and must be within the current list count")
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
		return boardRefusal("board_list_delete", err, boardsManageHint,
			"verify board_id and list_id with project.board_list_list")
	}
	return nil
}
