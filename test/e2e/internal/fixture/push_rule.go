//go:build e2e

// push_rule.go builds the one push rule a project may have.
//
// A project holds at most one push rule, so this is a create where every
// other builder here is an add: asking twice is refused rather than answered
// with a second object. The rule is created with a branch-name pattern and
// nothing else, because a rule that rejected unsigned commits would also
// reject every commit a later fixture makes in the same project.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// pushRuleBranchPattern is what the fixture rule matches. It accepts every
// branch name, so the rule is real and stops nothing.
const pushRuleBranchPattern = ".*"

// PushRule is the push rule a builder put on a project.
type PushRule struct {
	// ID is the rule's own identifier, which the read-back compares.
	ID int64
	// BranchNameRegex is the pattern it was created with.
	BranchNameRegex string
}

// NewProjectPushRule puts a push rule on the project and registers its
// removal.
func NewProjectPushRule(e *harness.Env, project Project) PushRule {
	e.T.Helper()

	rule, err := retryTransient(e, "create push rule", createRetries, func() (PushRule, error) {
		return createProjectPushRule(e.Ctx, e.Client(), project.ID)
	})
	if err != nil {
		e.T.Fatalf("creating the push rule of project %d: %v", project.ID, err)
	}

	e.Defer("push rule of project "+project.Path, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectPushRule(ctx, e.Client(), project.ID)
	})
	return rule
}

// createProjectPushRule asks GitLab for the rule.
func createProjectPushRule(ctx context.Context, client *gitlabclient.Client, projectID int64) (PushRule, error) {
	created, _, err := client.GL().Projects.AddProjectPushRule(projectID, &gl.AddProjectPushRuleOptions{
		BranchNameRegex: new(pushRuleBranchPattern),
	}, gl.WithContext(ctx))
	if err != nil {
		return PushRule{}, err
	}
	return PushRule{ID: created.ID, BranchNameRegex: created.BranchNameRegex}, nil
}

// deleteProjectPushRule removes the rule and tolerates one a case deleted.
func deleteProjectPushRule(ctx context.Context, client *gitlabclient.Client, projectID int64) error {
	_, err := client.GL().Projects.DeleteProjectPushRule(projectID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting the push rule of project %d: %w", projectID, err)
	}
	return nil
}
