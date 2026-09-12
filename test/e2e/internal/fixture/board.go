//go:build e2e

// board.go builds a group issue board over GraphQL, because the REST create
// route is the Premium one: it refuses a first board on an unlicensed
// instance, while the mutation runs the same service the boards page runs
// and lets every group have one board on every edition. The board goes with
// the group, so nothing is registered.

package fixture

import (
	"fmt"
	"strconv"
	"strings"

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

// boardMutation creates one group issue board.
const boardMutation = `mutation($groupPath: ID!, $name: String!) {
  createBoard(input: { groupPath: $groupPath, name: $name }) {
    board { id }
    errors
  }
}`

// boardGIDPrefix is what GitLab puts in front of a board's number in its
// global ID.
const boardGIDPrefix = "gid://gitlab/Board/"

// NewGroupBoard creates an issue board in the group, named with the run's
// own scoping.
func NewGroupBoard(e *harness.Env, group Group, prefix string) GroupBoard {
	e.T.Helper()

	name := e.Name(prefix)
	var created struct {
		CreateBoard struct {
			Board *struct {
				ID string `json:"id"`
			} `json:"board"`
			Errors []string `json:"errors"`
		} `json:"createBoard"`
	}
	err := mutate(e, boardMutation, map[string]any{"groupPath": group.Path, "name": name}, &created)
	if err != nil {
		e.T.Fatalf("creating a board in group %s: %v", group.Path, err)
	}
	if errs := created.CreateBoard.Errors; len(errs) > 0 {
		e.T.Fatalf("GitLab refused a board in group %s: %s", group.Path, strings.Join(errs, "; "))
	}
	if created.CreateBoard.Board == nil || created.CreateBoard.Board.ID == "" {
		e.T.Fatalf("the board in group %s was created with no ID", group.Path)
	}
	id, err := boardIDFromGID(created.CreateBoard.Board.ID)
	if err != nil {
		e.T.Fatal(err)
	}
	return GroupBoard{ID: id, GID: created.CreateBoard.Board.ID, Name: name}
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
