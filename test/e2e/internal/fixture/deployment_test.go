//go:build e2e

// deployment_test.go pins the two facts a GitLab 19 deployment request has
// to state, so a change to either is a deliberate one, and drives the two
// creators against the stub.

package fixture

import (
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestCreateEnvironment_Answers_ReadsTheEnvironmentOrTheRefusal checks the
// environment creator sends the name and reads back what GitLab made, and
// hands GitLab's refusal back as it came.
func TestCreateEnvironment_Answers_ReadsTheEnvironmentOrTheRefusal(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/environments",
		stubCreated(map[string]any{"id": 5, "name": "env-run", "state": "available"}),
		stubRefusal(http.StatusBadRequest, "name has already been taken"))

	got, err := createEnvironment(t.Context(), client, 2, "env-run")
	if err != nil {
		t.Fatalf("createEnvironment() error = %v, want nil", err)
	}
	if want := (Environment{ID: 5, Name: "env-run"}); got != want {
		t.Errorf("createEnvironment() = %+v, want %+v", got, want)
	}
	if sent := stub.recordedRequests()[0].Body["name"]; sent != "env-run" {
		t.Errorf("createEnvironment() sent name %v, want env-run", sent)
	}

	got, err = createEnvironment(t.Context(), client, 2, "env-run")
	if !IsStatus(err, http.StatusBadRequest) || got != (Environment{}) {
		t.Errorf("createEnvironment() on a refusal = %+v, %v; want nothing and GitLab's 400", got, err)
	}
}

// TestCreateDeployment_Answers_StatesTheBranchAndTheStatus checks the
// deployment creator sends the two facts GitLab 19 refuses a request without,
// beside the environment, ref and commit it was given, and reads back what
// GitLab made; a refusal comes back as it came.
func TestCreateDeployment_Answers_StatesTheBranchAndTheStatus(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/deployments",
		stubCreated(map[string]any{"id": 6, "sha": "abc123", "status": "running"}),
		stubRefusal(http.StatusBadRequest, "tag is missing"))
	environment := Environment{ID: 5, Name: "env-run"}

	got, err := createDeployment(t.Context(), client, 2, environment, "feature/world", "abc123")
	if err != nil {
		t.Fatalf("createDeployment() error = %v, want nil", err)
	}
	if want := (Deployment{ID: 6, Environment: environment, SHA: "abc123", Status: "running"}); got != want {
		t.Errorf("createDeployment() = %+v, want %+v", got, want)
	}
	sent := stub.recordedRequests()[0].Body
	wantSent := map[string]any{"environment": "env-run", "ref": "feature/world", "sha": "abc123", "tag": false, "status": "running"}
	for field, want := range wantSent {
		t.Run(field, func(t *testing.T) {
			if sent[field] != want {
				t.Errorf("createDeployment() sent %s = %v, want %v", field, sent[field], want)
			}
		})
	}

	got, err = createDeployment(t.Context(), client, 2, environment, "feature/world", "abc123")
	if !IsStatus(err, http.StatusBadRequest) || got != (Deployment{}) {
		t.Errorf("createDeployment() on a refusal = %+v, %v; want nothing and GitLab's 400", got, err)
	}
}

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
