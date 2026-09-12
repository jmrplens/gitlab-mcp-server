//go:build e2e

// issue.go builds the objects that hang off a project's issue tracker: an
// issue, a label and a milestone. All three go with the project, so none
// registers a deletion of its own.

package fixture

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Issue is an issue a builder created.
type Issue struct {
	// IID is the project-scoped identifier every issue action takes.
	IID int64
	// ID is the instance-wide identifier.
	ID int64
	// Title is what it was created with.
	Title string
}

// Label is a project label a builder created.
type Label struct {
	// ID is the label's identifier.
	ID int64
	// Name is what a label filter takes.
	Name string
	// Color is the hex color it was created with.
	Color string
}

// Milestone is a project milestone a builder created.
type Milestone struct {
	// ID is the instance-wide identifier a milestone_id parameter takes.
	ID int64
	// IID is the project-scoped identifier.
	IID int64
	// Title is what it was created with.
	Title string
}

// NewIssue creates an issue in the project.
func NewIssue(e *harness.Env, project Project, title string) Issue {
	e.T.Helper()

	issue, err := retryTransient(e, "create issue", createRetries, func() (Issue, error) {
		created, _, err := e.Client().GL().Issues.CreateIssue(project.ID, &gl.CreateIssueOptions{
			Title:       new(title),
			Description: new("e2e: " + e.T.Name()),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Issue{}, err
		}
		return Issue{IID: created.IID, ID: created.ID, Title: created.Title}, nil
	})
	if err != nil {
		e.T.Fatalf("creating an issue in project %d: %v", project.ID, err)
	}
	return issue
}

// labelColor is the color every fixture label gets. GitLab requires one and
// no test cares which.
const labelColor = "#428BCA"

// NewLabel creates a project label, named with the run's own scoping so two
// tests sharing a project cannot collide on it.
func NewLabel(e *harness.Env, project Project, prefix string) Label {
	e.T.Helper()
	return createLabel(e, project, e.Name(prefix))
}

// createLabel creates a project label under exactly the given name.
func createLabel(e *harness.Env, project Project, name string) Label {
	e.T.Helper()

	label, err := retryTransient(e, "create label "+name, createRetries, func() (Label, error) {
		created, _, err := e.Client().GL().Labels.CreateLabel(project.ID, &gl.CreateLabelOptions{
			Name:        new(name),
			Color:       new(labelColor),
			Description: new("e2e: " + e.T.Name()),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Label{}, err
		}
		return Label{ID: created.ID, Name: created.Name, Color: created.Color}, nil
	})
	if err != nil {
		e.T.Fatalf("creating label %q in project %d: %v", name, project.ID, err)
	}
	return label
}

// NewMilestone creates a project milestone titled with the run's own scoping.
func NewMilestone(e *harness.Env, project Project, prefix string) Milestone {
	e.T.Helper()
	return createMilestone(e, project, e.Name(prefix))
}

// createMilestone creates a project milestone under exactly the given title.
func createMilestone(e *harness.Env, project Project, title string) Milestone {
	e.T.Helper()

	milestone, err := retryTransient(e, "create milestone "+title, createRetries, func() (Milestone, error) {
		created, _, err := e.Client().GL().Milestones.CreateMilestone(project.ID, &gl.CreateMilestoneOptions{
			Title:       new(title),
			Description: new("e2e: " + e.T.Name()),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Milestone{}, err
		}
		return Milestone{ID: created.ID, IID: created.IID, Title: created.Title}, nil
	})
	if err != nil {
		e.T.Fatalf("creating milestone %q in project %d: %v", title, project.ID, err)
	}
	return milestone
}
