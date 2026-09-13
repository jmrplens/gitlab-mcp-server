//go:build e2e

// epic.go builds an epic, which GitLab 19 offers only as a work item: the
// REST epics API is gone there, so the builder goes through client-go's work
// item service, the same route the epic tools take. An epic goes with its
// group, so nothing is registered.

package fixture

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Epic is an epic a builder created in a group.
type Epic struct {
	// IID is the group-scoped number every epic action takes.
	IID int64
	// ID is the instance-wide identifier.
	ID int64
	// Title is what it was created with.
	Title string
	// GroupPath is the full path of the group it belongs to, which is what
	// the epic actions name the group by.
	GroupPath string
}

// NewEpic creates an epic in the group. Epics are a licensed feature, and a
// package that runs on an unlicensed instance never reaches this builder:
// the harness refuses a Premium action before it is sent.
func NewEpic(e *harness.Env, group Group, title string) Epic {
	e.T.Helper()

	epic, err := retryTransient(e, "create epic "+title, createRetries, func() (Epic, error) {
		created, _, err := e.Client().GL().WorkItems.CreateWorkItem(group.Path, gl.WorkItemTypeEpic,
			&gl.CreateWorkItemOptions{Title: title}, gl.WithContext(e.Ctx))
		if err != nil {
			return Epic{}, err
		}
		return Epic{IID: created.IID, ID: created.ID, Title: created.Title, GroupPath: group.Path}, nil
	})
	if err != nil {
		e.T.Fatalf("creating epic %q in group %s: %v", title, group.Path, err)
	}
	return epic
}
