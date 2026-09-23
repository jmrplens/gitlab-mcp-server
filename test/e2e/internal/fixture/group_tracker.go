//go:build e2e

// group_tracker.go builds the objects that hang off a group's issue tracker
// rather than a project's: a group label and a group milestone. Both go
// with the group, so neither registers a deletion of its own.

package fixture

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// GroupLabel is a group label a builder created.
type GroupLabel struct {
	// ID is what a label-backed board list and an epic's label ids take.
	ID int64
	// Name is what a label filter takes.
	Name string
	// Color is the hex color it was created with.
	Color string
}

// GroupMilestone is a group milestone a builder created.
type GroupMilestone struct {
	// ID is the instance-wide identifier.
	ID int64
	// IID is the group-scoped identifier every group milestone action takes.
	IID int64
	// Title is what it was created with.
	Title string
}

// groupMilestoneLength is how long a fixture group milestone runs. It starts
// today, so a burndown chart has a window to report on.
const groupMilestoneLength = 14 * 24 * time.Hour

// NewGroupLabel creates a group label named with the run's own scoping.
func NewGroupLabel(e *harness.Env, group Group, prefix string) GroupLabel {
	e.T.Helper()

	name := e.Name(prefix)
	label, err := retryTransient(e, "create group label "+name, createRetries, func() (GroupLabel, error) {
		return createGroupLabel(e.Ctx, e.Client(), group.ID, name, "e2e: "+e.T.Name())
	})
	if err != nil {
		e.T.Fatalf("creating group label %q in group %d: %v", name, group.ID, err)
	}
	return label
}

// createGroupLabel asks GitLab for the group label, in the one color every
// fixture label carries.
func createGroupLabel(ctx context.Context, client *gitlabclient.Client, groupID int64, name, description string) (GroupLabel, error) {
	created, _, err := client.GL().GroupLabels.CreateGroupLabel(groupID, &gl.CreateGroupLabelOptions{
		Name:        new(name),
		Color:       new(labelColor),
		Description: new(description),
	}, gl.WithContext(ctx))
	if err != nil {
		return GroupLabel{}, err
	}
	return GroupLabel{ID: created.ID, Name: created.Name, Color: created.Color}, nil
}

// NewGroupMilestone creates a group milestone titled with the run's own
// scoping, running from today for two weeks.
func NewGroupMilestone(e *harness.Env, group Group, prefix string) GroupMilestone {
	e.T.Helper()

	title := e.Name(prefix)
	start := time.Now().UTC()
	milestone, err := retryTransient(e, "create group milestone "+title, createRetries, func() (GroupMilestone, error) {
		return createGroupMilestone(e.Ctx, e.Client(), group.ID, title, "e2e: "+e.T.Name(), start)
	})
	if err != nil {
		e.T.Fatalf("creating group milestone %q in group %d: %v", title, group.ID, err)
	}
	return milestone
}

// createGroupMilestone asks GitLab for a group milestone running from start
// for groupMilestoneLength. The start is the caller's, so the dates it sends
// are the caller's to state rather than whatever the clock read inside.
func createGroupMilestone(ctx context.Context, client *gitlabclient.Client, groupID int64, title, description string, start time.Time) (GroupMilestone, error) {
	startDate := gl.ISOTime(start)
	dueDate := gl.ISOTime(start.Add(groupMilestoneLength))
	created, _, err := client.GL().GroupMilestones.CreateGroupMilestone(groupID, &gl.CreateGroupMilestoneOptions{
		Title:       new(title),
		Description: new(description),
		StartDate:   &startDate,
		DueDate:     &dueDate,
	}, gl.WithContext(ctx))
	if err != nil {
		return GroupMilestone{}, err
	}
	return GroupMilestone{ID: created.ID, IID: created.IID, Title: created.Title}, nil
}
