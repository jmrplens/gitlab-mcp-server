//go:build e2e

// pipeline_test.go drives the pipeline wait and the runner lookup against
// the stub.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"strings"
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

// TestManualJobCIYAML_Shape_DeclaresAPlayableJobInTheFirstStage pins the two
// properties the world whose case plays a job rests on: the job is manual,
// and it is in the same stage as the fast one. A manual job in a later stage
// is "created" until GitLab reaches that stage, and playing one in that state
// is the refusal this configuration exists to end.
func TestManualJobCIYAML_Shape_DeclaresAPlayableJobInTheFirstStage(t *testing.T) {
	if !strings.Contains(ManualJobCIYAML, ManualJobName+":\n  stage: test\n  when: manual\n") {
		t.Errorf("ManualJobCIYAML declares no manual job named %q in the first stage:\n%s", ManualJobName, ManualJobCIYAML)
	}
	if strings.Count(ManualJobCIYAML, "stage: test") != 2 {
		t.Errorf("ManualJobCIYAML does not keep both jobs in one stage:\n%s", ManualJobCIYAML)
	}
	if !strings.Contains(ManualJobCIYAML, "tags: []") {
		t.Error("ManualJobCIYAML declares runner tags, so the instance's runner would not pick its jobs up")
	}
	if strings.Contains(CIYAML, "when: manual") {
		t.Error("CIYAML declares a manual job, which would leave every pipeline world blocked")
	}
}

// TestFindPipelineJob_Answers covers what the job wait distinguishes: the job
// it was looking for, a listing that does not hold it yet, and a refusal.
func TestFindPipelineJob_Answers(t *testing.T) {
	manual := func(job *gl.Job) bool { return job.Name == ManualJobName }
	anyJob := func(*gl.Job) bool { return true }

	cases := []struct {
		name      string
		answer    scriptedAnswer
		accept    func(*gl.Job) bool
		want      int64
		wantState string
		wantErr   bool
	}{
		{
			name: "the manual job",
			answer: stubOK([]map[string]any{
				{"id": 10, "name": "fast-pass", "status": "success"},
				{"id": 11, "name": ManualJobName, "status": "manual"},
			}),
			accept: manual, want: 11,
		},
		{
			name:      "no manual job yet",
			answer:    stubOK([]map[string]any{{"id": 10, "name": "fast-pass", "status": "running"}}),
			accept:    manual,
			wantState: "fast-pass=running",
		},
		{
			name:      "no job at all",
			answer:    stubOK([]map[string]any{}),
			accept:    anyJob,
			wantState: "holds 0 job(s)",
		},
		{
			name:   "the first job",
			answer: stubOK([]map[string]any{{"id": 10, "name": "fast-pass", "status": "running"}}),
			accept: anyJob, want: 10,
		},
		{
			name:    "refused",
			answer:  stubRefusal(http.StatusForbidden, "403 Forbidden"),
			accept:  anyJob,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/projects/2/pipelines/5/jobs", tc.answer)

			got, state, err := findPipelineJob(t.Context(), client, 2, 5, tc.accept)
			if (err != nil) != tc.wantErr {
				t.Fatalf("findPipelineJob() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("findPipelineJob() = %d, want %d", got, tc.want)
			}
			if tc.wantState != "" && !strings.Contains(state, tc.wantState) {
				t.Errorf("findPipelineJob() state = %q, want it to mention %q", state, tc.wantState)
			}
		})
	}
}

// TestCreatePipeline_Answers_ReadsThePipelineAsCreated checks the creator
// asks for a pipeline on the ref it was given and hands it back in the status
// GitLab created it in, without waiting; a refusal comes back as it came.
func TestCreatePipeline_Answers_ReadsThePipelineAsCreated(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/pipeline",
		stubCreated(map[string]any{"id": 77, "ref": "main", "sha": "abc", "status": "created"}),
		stubRefusal(http.StatusBadRequest, "Pipeline filtered out by workflow rules."))

	got, err := createPipeline(t.Context(), client, 2, "main")
	if err != nil {
		t.Fatalf("createPipeline() error = %v, want nil", err)
	}
	if want := (Pipeline{ID: 77, Ref: "main", SHA: "abc", Status: "created"}); got != want {
		t.Errorf("createPipeline() = %+v, want %+v", got, want)
	}
	if sent := stub.recordedRequests()[0].Body["ref"]; sent != "main" {
		t.Errorf("createPipeline() sent ref %v, want main", sent)
	}

	got, err = createPipeline(t.Context(), client, 2, "main")
	if !IsStatus(err, http.StatusBadRequest) || got != (Pipeline{}) {
		t.Errorf("createPipeline() on a refusal = %+v, %v; want nothing and GitLab's 400", got, err)
	}
}

// TestCancelPipeline_Answers_NamesThePipelineItCouldNotCancel checks the
// cancel reaches the pipeline's cancel endpoint, and that a refusal names the
// pipeline and keeps GitLab's status.
func TestCancelPipeline_Answers_NamesThePipelineItCouldNotCancel(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/pipelines/77/cancel",
		stubOK(map[string]any{"id": 77, "status": "canceling"}),
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	if err := cancelPipeline(t.Context(), client, 2, 77); err != nil {
		t.Fatalf("cancelPipeline() error = %v, want nil", err)
	}
	err := cancelPipeline(t.Context(), client, 2, 77)
	if !IsStatus(err, http.StatusForbidden) || !strings.Contains(err.Error(), "canceling pipeline 77 of project 2") {
		t.Errorf("cancelPipeline() on a refusal = %v, want GitLab's 403 naming the pipeline", err)
	}
}

// TestAwaitPipelineJob_Answers_WaitsForTheJobItWants covers the three endings
// of the job wait: the job appears after a listing that did not hold it yet,
// the budget runs out, and GitLab refuses the listing, which ends the wait at
// once rather than at the budget.
func TestAwaitPipelineJob_Answers_WaitsForTheJobItWants(t *testing.T) {
	manual := func(job *gl.Job) bool { return job.Name == ManualJobName }
	notYet := stubOK([]map[string]any{{"id": 10, "name": "fast-pass", "status": "running"}})
	cases := []struct {
		name    string
		answers []scriptedAnswer
		wait    time.Duration
		want    int64
		wantErr error
		refused bool
	}{
		{
			name:    "appears on the second listing",
			answers: []scriptedAnswer{notYet, stubOK([]map[string]any{{"id": 11, "name": ManualJobName, "status": "manual"}})},
			wait:    30 * time.Second, want: 11,
		},
		{name: "never appears", answers: []scriptedAnswer{notYet}, wait: 100 * time.Millisecond, wantErr: harness.ErrPollTimeout},
		{name: "listing refused", answers: []scriptedAnswer{stubRefusal(http.StatusForbidden, "403 Forbidden")}, wait: 30 * time.Second, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/projects/2/pipelines/5/jobs", tc.answers...)

			started := time.Now()
			got, err := awaitPipelineJob(t.Context(), client, 2, 5, tc.wait, manual)
			if got != tc.want {
				t.Errorf("awaitPipelineJob() = %d, want %d", got, tc.want)
			}
			switch {
			case tc.refused:
				if !IsStatus(err, http.StatusForbidden) || time.Since(started) > 10*time.Second {
					t.Errorf("awaitPipelineJob() on a refused listing = %v after %s, want GitLab's 403 at once", err, time.Since(started))
				}
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("awaitPipelineJob() error = %v, want %v", err, tc.wantErr)
				}
			case err != nil:
				t.Errorf("awaitPipelineJob() error = %v, want nil", err)
			}
		})
	}
}

// TestJobDescription_NamesWhatTheWaitWantedOrSaysAnyJob covers both halves of
// the message a job wait fails with.
func TestJobDescription_NamesWhatTheWaitWantedOrSaysAnyJob(t *testing.T) {
	if got := jobDescription(""); got != "at all" {
		t.Errorf("jobDescription(%q) = %q, want %q", "", got, "at all")
	}
	if got := jobDescription("named deploy"); got != "named deploy" {
		t.Errorf("jobDescription() = %q, want the description it was given", got)
	}
}
