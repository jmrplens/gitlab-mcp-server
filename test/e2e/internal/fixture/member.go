//go:build e2e

// member.go makes a user a member of a project.
//
// It is the project half of the membership AddGroupMember does for a group,
// and it is here rather than in project.go because project.go is about
// raising a project and taking it down again, and a membership is neither.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// AddProjectMember makes user a member of the project at the given access
// level. The membership goes with the project, so nothing is registered.
func AddProjectMember(e *harness.Env, project Project, user User, accessLevel gl.AccessLevelValue) {
	e.T.Helper()

	_, err := retryTransient(e, "add project member "+user.Username, createRetries, func() (struct{}, error) {
		return struct{}{}, addProjectMember(e.Ctx, e.Client(), project.ID, user.ID, accessLevel)
	})
	if err != nil {
		e.T.Fatalf("adding user %s to project %d: %v", user.Username, project.ID, err)
	}
}

// addProjectMember asks GitLab for the membership.
func addProjectMember(ctx context.Context, client *gitlabclient.Client, projectID, userID int64, accessLevel gl.AccessLevelValue) error {
	_, _, err := client.GL().ProjectMembers.AddProjectMember(projectID, &gl.AddProjectMemberOptions{
		UserID:      userID,
		AccessLevel: new(accessLevel),
	}, gl.WithContext(ctx))
	return err
}

// ProjectHasMember reports whether the user is still a member of the project,
// which is what a case that adds or removes one is verified against. A
// membership the project never had answers 404, which is the "no" this
// reports rather than an error.
func ProjectHasMember(ctx context.Context, client *gitlabclient.Client, projectID, userID int64) (bool, error) {
	_, _, err := client.GL().ProjectMembers.GetProjectMember(projectID, userID, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the membership of user %d in project %d: %w", userID, projectID, err)
	}
	return true, nil
}
