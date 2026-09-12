//go:build e2e

// deployment.go builds an environment and a deployment into it, stating the
// two things GitLab 19 insists on and older releases let a caller omit.

package fixture

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Environment is an environment a builder created.
type Environment struct {
	// ID is what the environment actions take.
	ID int64
	// Name is what a deployment names.
	Name string
}

// Deployment is a deployment a builder created.
type Deployment struct {
	// ID is what the deployment actions take.
	ID int64
	// Environment is the environment it went to.
	Environment Environment
	// SHA is the commit it deployed.
	SHA string
	// Status is the status it was created in.
	Status string
}

// NewEnvironment creates an environment in the project, named with the run's
// own scoping. It goes with the project, so nothing is registered.
func NewEnvironment(e *harness.Env, project Project, prefix string) Environment {
	e.T.Helper()

	name := e.Name(prefix)
	environment, err := retryTransient(e, "create environment "+name, createRetries, func() (Environment, error) {
		created, _, err := e.Client().GL().Environments.CreateEnvironment(project.ID, &gl.CreateEnvironmentOptions{Name: new(name)}, gl.WithContext(e.Ctx))
		if err != nil {
			return Environment{}, err
		}
		return Environment{ID: created.ID, Name: created.Name}, nil
	})
	if err != nil {
		e.T.Fatalf("creating environment %q in project %d: %v", name, project.ID, err)
	}
	return environment
}

// DeploymentRefIsBranch is the tag flag every fixture deployment states.
//
// GitLab 19 rejects a deployment creation that omits the flag, where earlier
// releases inferred it from the ref, so a request has to say that the ref is
// a branch. It is a named constant so that a test building the same request
// through the server can state the same fact by the same name.
const DeploymentRefIsBranch = false

// DeploymentStatus is the status every fixture deployment is created in.
//
// GitLab 19 rejects "created" as an API deployment status; "running" is
// accepted, and on a protected environment it is the state that yields a
// blocked, approvable deployment, which is what the approval tests need.
const DeploymentStatus = gl.DeploymentStatusRunning

// NewDeployment creates a deployment of sha from the project's default
// branch into environment, in the running status. It goes with the project,
// so nothing is registered.
func NewDeployment(e *harness.Env, project Project, environment Environment, sha string) Deployment {
	e.T.Helper()

	deployment, err := retryTransient(e, "create deployment", createRetries, func() (Deployment, error) {
		created, _, err := e.Client().GL().Deployments.CreateProjectDeployment(project.ID, &gl.CreateProjectDeploymentOptions{
			Environment: new(environment.Name),
			Ref:         new(project.DefaultBranch),
			SHA:         new(sha),
			Tag:         new(DeploymentRefIsBranch),
			Status:      new(DeploymentStatus),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Deployment{}, err
		}
		return Deployment{ID: created.ID, Environment: environment, SHA: created.SHA, Status: created.Status}, nil
	})
	if err != nil {
		e.T.Fatalf("creating a deployment of %s into %q in project %d: %v", ShortSHA(sha), environment.Name, project.ID, err)
	}
	return deployment
}
