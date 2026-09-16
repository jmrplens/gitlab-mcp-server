//go:build e2e

// protected_env.go protects an environment at the two scopes a licensed
// instance offers, project and group, and builds the deployment a protected
// environment holds back for approval.
//
// The approval world deliberately names the run's own user as the approver.
// GitLab refuses a self-approval, so the case that approves the deployment is
// refused by GitLab rather than by the surface, which is a deterministic
// answer that needs no second user and still goes through the whole approval
// path. It is the same arrangement the end-to-end suite's own protected
// environment scenario settled on.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The access a protection grants and the approvals it demands: Maintainer
// deploy access, one approval.
const (
	protectedEnvironmentAccess    = gl.MaintainerPermissions
	protectedEnvironmentApprovals = int64(1)
)

// ProtectedEnvironment is an environment a builder protected.
type ProtectedEnvironment struct {
	// Name is the environment the protection covers, which is what every
	// protected environment action takes.
	Name string
}

// DeploymentApproval is a protected environment with a deployment waiting on
// it.
type DeploymentApproval struct {
	// Environment is the environment the deployment went to.
	Environment Environment
	// Protected is the protection that holds the deployment back.
	Protected ProtectedEnvironment
	// Deployment is the deployment waiting for approval.
	Deployment Deployment
}

// NewProtectedEnvironment protects the project's environment with a
// Maintainer deploy access level and registers its unprotection.
//
// When approver is not zero the protection also carries an approval rule
// naming that user, which is what makes a deployment into the environment
// wait rather than proceed.
func NewProtectedEnvironment(e *harness.Env, project Project, environment Environment, approver int64) ProtectedEnvironment {
	e.T.Helper()

	protection, err := retryTransient(e, "protect environment "+environment.Name, createRetries, func() (ProtectedEnvironment, error) {
		return protectEnvironment(e.Ctx, e.Client(), project.ID, environment.Name, approver)
	})
	if err != nil {
		e.T.Fatalf("protecting environment %q of project %d: %v", environment.Name, project.ID, err)
	}

	e.Defer("protected environment "+environment.Name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return unprotectEnvironment(ctx, e.Client(), project.ID, protection.Name)
	})
	return protection
}

// NewGroupProtectedEnvironment protects an environment name at group scope
// and registers its unprotection.
//
// The name need not be an environment of any project: a group protection is a
// rule about a name, which is why it takes one rather than an [Environment].
func NewGroupProtectedEnvironment(e *harness.Env, group Group) ProtectedEnvironment {
	e.T.Helper()

	name := e.Name("groupenv")
	protection, err := retryTransient(e, "protect group environment "+name, createRetries, func() (ProtectedEnvironment, error) {
		return protectGroupEnvironment(e.Ctx, e.Client(), group.ID, name)
	})
	if err != nil {
		e.T.Fatalf("protecting environment %q of group %d: %v", name, group.ID, err)
	}

	e.Defer("group protected environment "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return unprotectGroupEnvironment(ctx, e.Client(), group.ID, protection.Name)
	})
	return protection
}

// NewDeploymentApproval builds the whole approval world: an environment,
// a protection whose approval rule names the run's own user, a commit, and a
// deployment of that commit waiting on the rule.
func NewDeploymentApproval(e *harness.Env, project Project) DeploymentApproval {
	e.T.Helper()

	environment := NewEnvironment(e, project, "approval")
	protection := NewProtectedEnvironment(e, project, environment, e.Runtime().UserID)
	commit := CommitFile(e, project, project.DefaultBranch,
		e.Name("deploy")+".txt", "deployment approval fixture\n", "add the deployment approval fixture")
	deployment := NewDeployment(e, project, environment, commit.SHA)

	return DeploymentApproval{Environment: environment, Protected: protection, Deployment: deployment}
}

// protectEnvironment asks GitLab for the project-scoped protection.
func protectEnvironment(ctx context.Context, client *gitlabclient.Client, projectID int64, name string, approver int64) (ProtectedEnvironment, error) {
	opts := &gl.ProtectRepositoryEnvironmentsOptions{
		Name: new(name),
		DeployAccessLevels: &[]*gl.EnvironmentAccessOptions{
			{AccessLevel: new(protectedEnvironmentAccess)},
		},
	}
	if approver != 0 {
		opts.ApprovalRules = &[]*gl.EnvironmentApprovalRuleOptions{
			{UserID: new(approver), RequiredApprovalCount: new(protectedEnvironmentApprovals)},
		}
	}

	created, _, err := client.GL().ProtectedEnvironments.ProtectRepositoryEnvironments(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return ProtectedEnvironment{}, err
	}
	return ProtectedEnvironment{Name: created.Name}, nil
}

// unprotectEnvironment lifts the protection and tolerates one a case lifted.
func unprotectEnvironment(ctx context.Context, client *gitlabclient.Client, projectID int64, name string) error {
	_, err := client.GL().ProtectedEnvironments.UnprotectEnvironment(projectID, name, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("unprotecting environment %q of project %d: %w", name, projectID, err)
	}
	return nil
}

// protectGroupEnvironment asks GitLab for the group-scoped protection.
func protectGroupEnvironment(ctx context.Context, client *gitlabclient.Client, groupID int64, name string) (ProtectedEnvironment, error) {
	created, _, err := client.GL().GroupProtectedEnvironments.ProtectGroupEnvironment(groupID,
		&gl.ProtectGroupEnvironmentOptions{
			Name: new(name),
			DeployAccessLevels: &[]*gl.GroupEnvironmentAccessOptions{
				{AccessLevel: new(protectedEnvironmentAccess)},
			},
		}, gl.WithContext(ctx))
	if err != nil {
		return ProtectedEnvironment{}, err
	}
	return ProtectedEnvironment{Name: created.Name}, nil
}

// unprotectGroupEnvironment lifts the group protection and tolerates one a
// case lifted.
func unprotectGroupEnvironment(ctx context.Context, client *gitlabclient.Client, groupID int64, name string) error {
	_, err := client.GL().GroupProtectedEnvironments.UnprotectGroupEnvironment(groupID, name, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("unprotecting environment %q of group %d: %w", name, groupID, err)
	}
	return nil
}

// GroupEnvironmentIsProtected reports whether the group still protects the
// name, which is what a case that unprotects one is verified against.
func GroupEnvironmentIsProtected(ctx context.Context, client *gitlabclient.Client, groupID int64, name string) (bool, error) {
	_, _, err := client.GL().GroupProtectedEnvironments.GetGroupProtectedEnvironment(groupID, name, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the protection of environment %q in group %d: %w", name, groupID, err)
	}
	return true, nil
}
