//go:build e2e

// board.go builds an issue board over GraphQL, because the REST create route
// is the Premium one: it refuses a first board on an unlicensed instance,
// while the mutation runs the same service the boards page runs and lets
// every group and every project have one board on every edition. A board goes
// with the group or the project it is in, so nothing is registered.

package fixture

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// GroupBoard is a group issue board a builder created.
type GroupBoard struct {
	// ID is the numeric identifier every board action takes.
	ID int64
	// GID is the GraphQL global ID it was created as.
	GID string
	// Name is what it was created with.
	Name string
}

// ProjectBoard is a project issue board a builder created.
type ProjectBoard struct {
	// ID is the numeric identifier the project board actions and the board
	// resource take.
	ID int64
	// Name is what it was created with.
	Name string
}

// The two board mutations, one per scope. They are two documents rather than
// one naming both paths because the input takes exactly one of them, and a
// document sending the other as null would leave GitLab to decide whether a
// null counts as naming it.
const (
	groupBoardMutation = `mutation($groupPath: ID!, $name: String!) {
  createBoard(input: { groupPath: $groupPath, name: $name }) {
    board { id }
    errors
  }
}`
	projectBoardMutation = `mutation($projectPath: ID!, $name: String!) {
  createBoard(input: { projectPath: $projectPath, name: $name }) {
    board { id }
    errors
  }
}`
)

// boardListLimitMutation sets the work-in-progress limit of a board column and
// the metric it is counted by. The REST update of a column takes its position
// alone, so this mutation is the only way to give a column the limit_metric
// the board answers then carry. GitLab runs it only where work-in-progress
// limits are licensed. It sends both ceilings, as GitLab's own board settings
// panel does, so the one the metric does not count is set to zero rather than
// left at whatever the column held.
const boardListLimitMutation = `mutation($listId: ListID!, $limitMetric: ListLimitMetric, $maxIssueCount: Int, $maxIssueWeight: Int) {
  boardListUpdateLimitMetrics(input: { listId: $listId, limitMetric: $limitMetric, maxIssueCount: $maxIssueCount, maxIssueWeight: $maxIssueWeight }) {
    list { id limitMetric maxIssueCount maxIssueWeight }
    errors
  }
}`

// boardListWeightMetric is the limit metric whose ceiling is the total weight
// of a column's issues, held in maxIssueWeight; issue_count holds its ceiling
// in maxIssueCount.
const boardListWeightMetric = "issue_weights"

// boardGIDPrefix is what GitLab puts in front of a board's number in its
// global ID.
const boardGIDPrefix = "gid://gitlab/Board/"

// boardListGIDPrefix is what GitLab puts in front of a board column's number
// in its global ID.
const boardListGIDPrefix = "gid://gitlab/List/"

// errBoardWithoutID is a board mutation GitLab answered with no board and no
// refusal, which leaves nothing to address the board by.
var errBoardWithoutID = errors.New("the board was created with no ID")

// errBoardListNotAnswered is a limit mutation GitLab answered with no list and
// no refusal, which leaves nothing to say the limit was applied.
var errBoardListNotAnswered = errors.New("the limit was answered with no list")

// LimitBoardList limits a board column to ceiling, counted by metric, which is
// what makes GitLab send the column's limit_metric. The ceiling goes where
// GitLab keeps it for that metric, as its board settings panel puts it: a
// total weight for issue_weights, and a number of issues for issue_count, the
// one other metric that panel offers. Work-in-progress limits are Premium, so
// only a licensed instance applies it. A column goes with its board, so
// nothing is registered.
func LimitBoardList(e *harness.Env, listID int64, metric string, ceiling int64) {
	e.T.Helper()

	if err := limitBoardList(e.Ctx, e.Client(), listID, metric, ceiling); err != nil {
		e.T.Fatalf("limiting board list %d to %d by %s: %v", listID, ceiling, metric, err)
	}
}

// limitBoardList sends the limit mutation for one column and holds GitLab's
// answer to the metric and the ceilings it was asked for.
//
// A refusal GitLab reports inside the payload is an error here, because it
// arrives as a 200, and so is a list counted by another metric or limited by
// other ceilings, since a scenario reading the column afterwards would assert
// a limit GitLab was never told to apply.
func limitBoardList(ctx context.Context, client *gitlabclient.Client, listID int64, metric string, ceiling int64) error {
	maxIssueCount, maxIssueWeight := ceiling, int64(0)
	if metric == boardListWeightMetric {
		maxIssueCount, maxIssueWeight = 0, ceiling
	}
	var limited struct {
		BoardListUpdateLimitMetrics struct {
			List *struct {
				LimitMetric    string `json:"limitMetric"`
				MaxIssueCount  int64  `json:"maxIssueCount"`
				MaxIssueWeight int64  `json:"maxIssueWeight"`
			} `json:"list"`
			Errors []string `json:"errors"`
		} `json:"boardListUpdateLimitMetrics"`
	}
	variables := map[string]any{
		"listId":         boardListGIDPrefix + strconv.FormatInt(listID, 10),
		"limitMetric":    metric,
		"maxIssueCount":  maxIssueCount,
		"maxIssueWeight": maxIssueWeight,
	}
	if err := runGraphQL(ctx, client, boardListLimitMutation, variables, &limited); err != nil {
		return err
	}
	answer := limited.BoardListUpdateLimitMetrics
	if len(answer.Errors) > 0 {
		return fmt.Errorf("GitLab refused the limit: %s", strings.Join(answer.Errors, "; "))
	}
	if answer.List == nil {
		return errBoardListNotAnswered
	}
	if answer.List.LimitMetric != metric {
		return fmt.Errorf("GitLab answered the list counted by %q, want %q", answer.List.LimitMetric, metric)
	}
	if answer.List.MaxIssueCount != maxIssueCount || answer.List.MaxIssueWeight != maxIssueWeight {
		return fmt.Errorf("GitLab answered the list limited to %d issues and a weight of %d, want %d and %d",
			answer.List.MaxIssueCount, answer.List.MaxIssueWeight, maxIssueCount, maxIssueWeight)
	}
	return nil
}

// NewGroupBoard creates an issue board in the group, named with the run's
// own scoping.
func NewGroupBoard(e *harness.Env, group Group, prefix string) GroupBoard {
	e.T.Helper()

	name := e.Name(prefix)
	id, gid, err := createBoard(e.Ctx, e.Client(), groupBoardMutation, map[string]any{"groupPath": group.Path, "name": name})
	if err != nil {
		e.T.Fatalf("creating a board in group %s: %v", group.Path, err)
	}
	return GroupBoard{ID: id, GID: gid, Name: name}
}

// createProjectBoard creates an issue board in the project the path names.
func createProjectBoard(ctx context.Context, client *gitlabclient.Client, projectPath, name string) (ProjectBoard, error) {
	id, _, err := createBoard(ctx, client, projectBoardMutation, map[string]any{"projectPath": projectPath, "name": name})
	if err != nil {
		return ProjectBoard{}, err
	}
	return ProjectBoard{ID: id, Name: name}, nil
}

// createBoard sends one board mutation and reads the board it made, as the
// number the REST actions take and the global ID it was created as.
//
// A refusal GitLab reports inside the payload is an error here, because it
// arrives as a 200: a caller reading only the transport error would take it
// for a board with no ID.
func createBoard(ctx context.Context, client *gitlabclient.Client, mutation string, variables map[string]any) (id int64, gid string, err error) {
	var created struct {
		CreateBoard struct {
			Board *struct {
				ID string `json:"id"`
			} `json:"board"`
			Errors []string `json:"errors"`
		} `json:"createBoard"`
	}
	if sendErr := runGraphQL(ctx, client, mutation, variables, &created); sendErr != nil {
		return 0, "", sendErr
	}
	if errs := created.CreateBoard.Errors; len(errs) > 0 {
		return 0, "", fmt.Errorf("GitLab refused the board: %s", strings.Join(errs, "; "))
	}
	if created.CreateBoard.Board == nil || created.CreateBoard.Board.ID == "" {
		return 0, "", errBoardWithoutID
	}
	gid = created.CreateBoard.Board.ID
	id, err = boardIDFromGID(gid)
	if err != nil {
		return 0, "", err
	}
	return id, gid, nil
}

// boardIDFromGID reads the number out of a board's global ID, which is what
// the REST actions name a board by.
func boardIDFromGID(gid string) (int64, error) {
	number, found := strings.CutPrefix(gid, boardGIDPrefix)
	if !found {
		return 0, fmt.Errorf("the board's global ID %q does not start with %s", gid, boardGIDPrefix)
	}
	id, err := strconv.ParseInt(number, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("the board's global ID %q carries no positive number", gid)
	}
	return id, nil
}
