//go:build e2e

// deployment_test.go pins the two facts a GitLab 19 deployment request has
// to state, so a change to either is a deliberate one.

package fixture

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestDeploymentFacts_Constants_StateWhatGitLab19Demands checks that a
// fixture deployment says its ref is a branch and is created running, the
// two things GitLab 19 refuses a request without.
func TestDeploymentFacts_Constants_StateWhatGitLab19Demands(t *testing.T) {
	if DeploymentRefIsBranch {
		t.Errorf("DeploymentRefIsBranch = true, want false: the fixture deploys a branch")
	}
	if DeploymentStatus != gl.DeploymentStatusRunning {
		t.Errorf("DeploymentStatus = %q, want %q", DeploymentStatus, gl.DeploymentStatusRunning)
	}
}
