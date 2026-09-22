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

// boardGIDPrefix is what GitLab puts in front of a board's number in its
// global ID.
const boardGIDPrefix = "gid://gitlab/Board/"

// errBoardWithoutID is a board mutation GitLab answered with no board and no
// refusal, which leaves nothing to address the board by.
var errBoardWithoutID = errors.New("the board was created with no ID")

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
