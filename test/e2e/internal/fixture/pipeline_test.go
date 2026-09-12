//go:build e2e

// pipeline_test.go drives the pipeline wait and the runner lookup against
// the stub.

package fixture

import (
	"context"
	"errors"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// shortPipelinePolls makes the pipeline wait poll quickly for one test; the
// production interval is sized for a real runner.
func shortPipelinePolls(t *testing.T) {
	t.Helper()
	saved := pipelinePollInterval
	pipelinePollInterval = 10 * time.Millisecond
	t.Cleanup(func() { pipelinePollInterval = saved })
}

// TestWaitForPipelineStatus_Running_ReturnsTheTerminalStatus checks that the
// wait polls through a running pipeline to its terminal status and tolerates
// a burst of API errors on the way.
func TestWaitForPipelineStatus_Running_ReturnsTheTerminalStatus(t *testing.T) {
	shortPipelinePolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.pipelineFailures = 2
		stub.pipelineStatuses = []string{"pending", "running", "success"}
	})

	status, err := waitForPipelineStatus(context.Background(), client, 1, 10, 30*time.Second)
	if err != nil {
		t.Fatalf("waitForPipelineStatus() error = %v, want nil", err)
	}
	if status != "success" {
		t.Errorf("status = %q, want success", status)
	}
}

// TestWaitForPipelineStatus_NeverTerminal_FailsWithTheLastStatus checks the
// budget is enforced and the last status is named, which is the property the
// EE wait it replaces lacked.
func TestWaitForPipelineStatus_NeverTerminal_FailsWithTheLastStatus(t *testing.T) {
	shortPipelinePolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.pipelineStatuses = []string{"pending"} })

	status, err := waitForPipelineStatus(context.Background(), client, 1, 10, 200*time.Millisecond)
	if !errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForPipelineStatus() error = %v, want a poll timeout", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want the last one seen", status)
	}
}

// TestWaitForPipelineStatus_ErrorsInARow_GivesUpAfterTen checks that a
// GitLab that stops answering ends the wait with the error rather than with
// the budget, naming how many polls failed.
func TestWaitForPipelineStatus_ErrorsInARow_GivesUpAfterTen(t *testing.T) {
	shortPipelinePolls(t)
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.pipelineFailures = pipelineMaxConsecutiveErrors + 5 })

	status, err := waitForPipelineStatus(context.Background(), client, 1, 10, 30*time.Second)
	if err == nil || errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForPipelineStatus() error = %v, want the API error after ten failures", err)
	}
	if status != "unknown" {
		t.Errorf("status = %q, want unknown when no poll ever answered", status)
	}
	var left int
	stub.configure(func() { left = stub.pipelineFailures })
	if left != 5 {
		t.Errorf("polls made = %d, want exactly %d", pipelineMaxConsecutiveErrors+5-left, pipelineMaxConsecutiveErrors)
	}
}

// TestIsTerminalPipelineStatus_Values_NamesTheFour pins the set a pipeline
// never leaves.
func TestIsTerminalPipelineStatus_Values_NamesTheFour(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "success", want: true},
		{status: "failed", want: true},
		{status: "canceled", want: true},
		{status: "skipped", want: true},
		{status: "pending", want: false},
		{status: "running", want: false},
		{status: "created", want: false},
		{status: "", want: false},
	}
	for _, testCase := range cases {
		t.Run("status "+testCase.status, func(t *testing.T) {
			if got := IsTerminalPipelineStatus(testCase.status); got != testCase.want {
				t.Errorf("IsTerminalPipelineStatus(%q) = %t, want %t", testCase.status, got, testCase.want)
			}
		})
	}
}

// TestFindRunner_Description_ReturnsTheDockerRunner checks the lookup that
// replaces the two copies of findDockerRunnerID: the runner is found by the
// description register-runner.sh gives it, and its absence is reported with
// the count that was looked at.
func TestFindRunner_Description_ReturnsTheDockerRunner(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.runners = []*gl.Runner{
			{ID: 11, Description: "somebody-else"},
			{ID: 12, Description: DockerRunnerDescription},
		}
	})

	id, seen, err := findRunner(context.Background(), client, DockerRunnerDescription)
	if err != nil {
		t.Fatalf("findRunner() error = %v, want nil", err)
	}
	if id != 12 || seen != 2 {
		t.Errorf("findRunner() = (%d, %d), want (12, 2)", id, seen)
	}

	id, seen, err = findRunner(context.Background(), client, "not-registered")
	if err != nil || id != 0 || seen != 2 {
		t.Errorf("findRunner(absent) = (%d, %d, %v), want (0, 2, nil)", id, seen, err)
	}
}

// TestCIYAML_Shape_RunsOnAnyRunner pins the two properties the pipeline
// fixture relies on: one stage with one job, and no runner tags, so it is
// picked up by whatever runner the instance has.
func TestCIYAML_Shape_RunsOnAnyRunner(t *testing.T) {
	want := "stages:\n  - test\n\nfast-pass:\n  stage: test\n  script:\n    - echo \"e2e pipeline job\"\n  tags: []\n"
	if CIYAML != want {
		t.Errorf("CIYAML = %q, want %q", CIYAML, want)
	}
	if CIFilePath != ".gitlab-ci.yml" {
		t.Errorf("CIFilePath = %q, want .gitlab-ci.yml", CIFilePath)
	}
}
