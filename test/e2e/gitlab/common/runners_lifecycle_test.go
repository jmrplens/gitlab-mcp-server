//go:build e2e

// runners_lifecycle_test.go ports the throwaway-runner management the old
// suite drove through the individual runner tools: the group and project
// registration-token resets, and the register-to-remove lifecycle of runners
// that are paused and tagged so they never pick a suite job up. The shared
// Docker runner is never touched.
//
// The instance runner reads and the project claim/release refusals that the
// old TestMeta_Runner covered live in TestRunners_DockerRunner beside this
// file, which the EE port wrote; this test adds the management writes that
// only the throwaway runners exercise, so the two together carry the runner
// family the old suite spread across TestIndividual_RunnerExtras and
// TestMeta_Runner.

package common

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestRunners_ThrowawayLifecycle resets the group and project registration
// tokens and drives three throwaway runners through the uncovered mutations:
// runner A through verify, jobs, update, reset_token and delete_by_token,
// runner B through delete_registered, and runner C through remove.
//
// Replaces: TestIndividual_RunnerExtras, TestMeta_Runner
func TestRunners_ThrowawayLifecycle(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	s := e.On(harness.SurfaceIndividual)
	project := fixture.NewProject(e, fixture.WithNamePrefix("runner-xtra"))
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("runner-xtra"))

	owned := harness.Do[runners.ListOutput](s, actionRunnerList, nil)
	e.T.Logf("%d owned runner(s)", len(owned.Runners))

	resetRegistrationToken(e, s, actionRunnerResetGroupRegTok, map[string]any{"group_id": group.IDParam()}, "reset group registration token")
	regToken := resetRegistrationToken(e, s, actionRunnerResetProjectRegTok, map[string]any{"project_id": project.IDParam()}, "reset project registration token")

	runnerA := registerThrowawayRunner(e, s, regToken, "e2e-runner-extras-a")
	authToken := runnerA.Token

	harness.DoVoid(s, actionRunnerVerify, map[string]any{"token": authToken})

	jobs := harness.Do[runners.JobListOutput](s, actionRunnerJobs, map[string]any{"runner_id": runnerA.ID})
	if len(jobs.Jobs) != 0 {
		e.T.Errorf("a paused throwaway runner reports %d job(s), want none", len(jobs.Jobs))
	}

	const updatedDescription = "e2e-runner-extras-a-updated"
	updated := harness.Do[runners.DetailsOutput](s, actionRunnerUpdate, map[string]any{"runner_id": runnerA.ID, "description": updatedDescription})
	if updated.Description != updatedDescription {
		e.T.Errorf("runner update answered description %q, want %q", updated.Description, updatedDescription)
	}

	reset := harness.Do[runners.AuthTokenOutput](s, actionRunnerResetToken, map[string]any{"runner_id": runnerA.ID})
	if reset.Token == "" {
		e.T.Fatalf("runner reset_token answered an empty token")
	}
	// The reset invalidates the previous auth token, so the delete-by-token
	// below names the rotated one.
	harness.DoVoid(s, actionRunnerDeleteByToken, map[string]any{"token": reset.Token})

	runnerB := registerThrowawayRunner(e, s, regToken, "e2e-runner-extras-b")
	harness.DoVoid(s, actionRunnerDeleteRegistered, map[string]any{"runner_id": runnerB.ID})

	runnerC := registerThrowawayRunner(e, s, regToken, "e2e-runner-extras-c")
	harness.DoVoid(s, actionRunnerRemove, map[string]any{"runner_id": runnerC.ID})
}

// TestRunners_InstanceRegistrationToken resets the instance-global runner
// registration token, which rotates a secret every session shares, so it
// takes the instance-global lock.
//
// Replaces: TestIndividual_RunnerExtrasInstanceRegToken
func TestRunners_InstanceRegistrationToken(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceIndividual)

	token := resetRegistrationToken(e, s, actionRunnerResetInstanceRegTok, nil, "reset instance registration token")
	if token == "" {
		e.T.Errorf("reset instance registration token answered an empty token")
	}
}

// resetRegistrationToken resets one registration token and returns it,
// skipping the test when the instance has turned the deprecated
// registration-token flow off. Registration tokens were still functional on
// the GitLab the baseline ran on; an instance that has removed them answers a
// 403, 404 or 410, which is a skip rather than a failure.
func resetRegistrationToken(e *harness.Env, s *harness.Session, action harness.ActionID, params map[string]any, what string) string {
	e.T.Helper()

	out, err := harness.Try[runners.AuthTokenOutput](s, action, params)
	if err != nil {
		if runnerLegacyDisabled(err) {
			e.Skipf("%s: the legacy runner registration-token flow is unavailable on this instance: %v", what, err)
		}
		e.T.Fatalf("%s: %v", what, err)
	}
	if out.Token == "" {
		e.Skipf("%s answered no token: the legacy registration-token flow is unavailable on this instance", what)
	}
	return out.Token
}

// registerThrowawayRunner registers a paused, tagged runner against the
// registration token and schedules a best-effort raw removal, so a failed
// step cannot leak a runner. Paused registration guarantees the runner never
// dequeues a suite job.
func registerThrowawayRunner(e *harness.Env, s *harness.Session, regToken, description string) runners.Output {
	e.T.Helper()

	out, err := harness.Try[runners.Output](s, actionRunnerRegister, map[string]any{
		"token": regToken, "description": description, "paused": true, "tag_list": []string{"e2e-runner-extras"},
	})
	if err != nil {
		if runnerLegacyDisabled(err) {
			e.Skipf("register throwaway runner %q: the legacy registration-token flow is unavailable: %v", description, err)
		}
		e.T.Fatalf("register throwaway runner %q: %v", description, err)
	}
	if out.ID == 0 || out.Token == "" {
		e.T.Fatalf("runner register answered %+v, want a runner with an ID and an auth token", out)
	}
	runnerID := out.ID
	e.T.Cleanup(func() {
		// The lifecycle normally deletes the runner; a 404 here is expected
		// and ignored.
		if _, removeErr := e.Client().GL().Runners.RemoveRunner(int(runnerID)); removeErr != nil && !fixture.IsStatus(removeErr, http.StatusNotFound) {
			e.T.Logf("best-effort removal of throwaway runner %d answered: %v", runnerID, removeErr)
		}
	})
	return out
}

// runnerLegacyDisabled reports whether an answer is the instance refusing the
// deprecated registration-token flow, which is a skip rather than a failure.
// The harness hands back the server's own words rather than a typed status, so
// the statuses are matched in the text.
func runnerLegacyDisabled(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"403", "404", "410", "forbidden", "not found", "registration"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
