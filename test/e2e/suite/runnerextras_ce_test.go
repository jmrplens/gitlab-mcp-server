//go:build e2e && !enterprise

// runnerextras_ce_test.go tests the runner MCP tools that manage throwaway
// runners and registration tokens against a live GitLab instance. Covers
// runner.list, runner.jobs, runner.verify, runner.register, runner.update,
// runner.reset_token, runner.reset_project_reg_token,
// runner.reset_group_reg_token, runner.reset_instance_reg_token,
// runner.delete_by_token, runner.delete_registered, and runner.remove.
// Throwaway runners are registered paused and tagged so they can never pick
// up suite jobs, and the shared compose runner is never touched.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runners"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// runnerExtrasToolRegister is the individual tool used to register the three
// throwaway runners in the lifecycle subtests.
const runnerExtrasToolRegister = "gitlab_runner_register"

// TestIndividual_RunnerExtras exercises runner.list, the group and project
// registration-token resets, and the throwaway-runner lifecycle (register,
// verify, jobs, update, reset_token, delete_by_token, delete_registered,
// remove) using individual MCP tools.
//
// Registration tokens are deprecated on newer GitLab but still functional on
// 18.x self-managed unless explicitly disabled; when the instance rejects the
// legacy flow the affected subtests skip with a documented reason instead of
// failing.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_RunnerExtras(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured (group fixture uses the meta surface)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	proj := createProject(ctx, t, sess.individual)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "runner-xtra")

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[runners.ListOutput](ctx, sess.individual, "gitlab_runner_list", runners.ListInput{})
		requireNoError(t, err, "runner list")
		t.Logf("Listed %d owned runners", len(out.Runners))
	})

	t.Run("ResetGroupRegToken", func(t *testing.T) {
		out, err := callToolOn[runners.AuthTokenOutput](ctx, sess.individual, "gitlab_runner_reset_group_reg_token", runners.ResetGroupRegTokenInput{
			GroupID: toolutil.StringOrInt(grp.gidStr()),
		})
		runnerExtrasSkipIfRegistrationDisabled(t, err, "reset group runner registration token")
		requireTruef(t, out.Token != "", "expected non-empty group registration token")
	})

	t.Run("RegistrationLifecycle", func(t *testing.T) {
		regToken := runnerExtrasProjectRegToken(ctx, t, proj)
		runRunnerExtrasLifecycle(ctx, t, regToken)
	})
}

// TestIndividual_RunnerExtrasInstanceRegToken exercises
// runner.reset_instance_reg_token, which rotates an instance-global secret
// and therefore only runs against the disposable Docker GitLab with an admin
// token. Rotating the token is safe there because already-registered runners
// authenticate with their own auth tokens, not the registration token.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_RunnerExtrasInstanceRegToken(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("instance registration token reset mutates instance-global state; Docker mode only")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		out, err := callToolOn[runners.AuthTokenOutput](ctx, sess.individual, "gitlab_runner_reset_instance_reg_token", runners.ResetInstanceRegTokenInput{})
		runnerExtrasSkipIfRegistrationDisabled(t, err, "reset instance runner registration token")
		requireTruef(t, out.Token != "", "expected non-empty instance registration token")
	})
}

// runnerExtrasProjectRegToken resets and returns the throwaway project's
// runner registration token, covering runner.reset_project_reg_token. Skips
// the calling test when the legacy registration-token flow is unavailable.
func runnerExtrasProjectRegToken(ctx context.Context, t *testing.T, proj ProjectFixture) string {
	t.Helper()
	out, err := callToolOn[runners.AuthTokenOutput](ctx, sess.individual, "gitlab_runner_reset_project_reg_token", runners.ResetProjectRegTokenInput{
		ProjectID: proj.pidOf(),
	})
	runnerExtrasSkipIfRegistrationDisabled(t, err, "reset project runner registration token")
	requireTruef(t, out.Token != "", "expected non-empty project registration token")
	return out.Token
}

// runRunnerExtrasLifecycle registers three throwaway runners against the
// project registration token and drives each uncovered mutation: runner A
// through verify → jobs → update → reset_token → delete_by_token, runner B
// through delete_registered, and runner C through remove.
func runRunnerExtrasLifecycle(ctx context.Context, t *testing.T, regToken string) {
	t.Helper()

	runnerA := runnerExtrasRegister(ctx, t, regToken, "e2e-runner-extras-a")
	authToken := runnerA.Token

	t.Run("Verify", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_runner_verify", runners.VerifyInput{Token: authToken})
		requireNoError(t, err, "runner verify")
	})

	t.Run("Jobs", func(t *testing.T) {
		out, err := callToolOn[runners.JobListOutput](ctx, sess.individual, "gitlab_runner_jobs", runners.ListJobsInput{RunnerID: runnerA.ID})
		requireNoError(t, err, "runner jobs")
		// A paused throwaway runner can never pick up jobs, so an empty list
		// is the deterministic expected outcome.
		requireTruef(t, len(out.Jobs) == 0, "expected no jobs for paused throwaway runner, got %d", len(out.Jobs))
	})

	t.Run("Update", func(t *testing.T) {
		const updatedDescription = "e2e-runner-extras-a-updated"
		out, err := callToolOn[runners.DetailsOutput](ctx, sess.individual, "gitlab_runner_update", runners.UpdateInput{
			RunnerID:    runnerA.ID,
			Description: updatedDescription,
		})
		requireNoError(t, err, "runner update")
		requireTruef(t, out.Description == updatedDescription, "expected description %q, got %q", updatedDescription, out.Description)
	})

	t.Run("ResetToken", func(t *testing.T) {
		out, err := callToolOn[runners.AuthTokenOutput](ctx, sess.individual, "gitlab_runner_reset_token", runners.ResetAuthTokenInput{RunnerID: runnerA.ID})
		requireNoError(t, err, "runner reset auth token")
		requireTruef(t, out.Token != "", "expected non-empty auth token after reset")
		// The reset invalidates the previous auth token; the delete-by-token
		// subtest must use the rotated one.
		authToken = out.Token
	})

	t.Run("DeleteByToken", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_runner_delete_by_token", runners.DeleteByTokenInput{Token: authToken})
		requireNoError(t, err, "runner delete by token")
	})

	runnerB := runnerExtrasRegister(ctx, t, regToken, "e2e-runner-extras-b")
	t.Run("DeleteRegistered", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_runner_delete_registered", runners.DeleteByIDInput{RunnerID: runnerB.ID})
		requireNoError(t, err, "runner delete registered")
	})

	runnerC := runnerExtrasRegister(ctx, t, regToken, "e2e-runner-extras-c")
	t.Run("Remove", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_runner_remove", runners.RemoveInput{RunnerID: runnerC.ID})
		requireNoError(t, err, "runner remove")
	})
}

// runnerExtrasRegister registers a throwaway paused, tagged runner with the
// given registration token and schedules a best-effort raw-API removal so a
// failed subtest cannot leak runners. Paused registration guarantees the
// runner never dequeues suite jobs. Skips the calling test when the instance
// rejects the legacy registration-token flow.
func runnerExtrasRegister(ctx context.Context, t *testing.T, regToken, description string) runners.Output {
	t.Helper()
	out, err := callToolOn[runners.Output](ctx, sess.individual, runnerExtrasToolRegister, runners.RegisterInput{
		Token:       regToken,
		Description: description,
		Paused:      new(true),
		TagList:     []string{"e2e-runner-extras"},
	})
	runnerExtrasSkipIfRegistrationDisabled(t, err, "register throwaway runner")
	requireTruef(t, out.ID > 0, "expected positive runner ID for %s", description)
	requireTruef(t, out.Token != "", "expected non-empty auth token for %s", description)
	runnerID := out.ID
	t.Cleanup(func() {
		// Best-effort: the lifecycle subtests normally delete the runner; a
		// 404 here is expected and ignored.
		_, _ = sess.glClient.GL().Runners.RemoveRunner(int(runnerID))
	})
	return out
}

// runnerExtrasSkipIfRegistrationDisabled skips the test when the instance
// has disabled or removed the legacy registration-token flow (deprecated
// since GitLab 15.6, scheduled for removal in 20.0, still functional on 18.x
// self-managed unless turned off). Any other error fails the test.
func runnerExtrasSkipIfRegistrationDisabled(t *testing.T, err error, action string) {
	t.Helper()
	if err == nil {
		return
	}
	msg := strings.ToLower(err.Error())
	if isHTTPStatus(err, http.StatusForbidden) || isHTTPStatus(err, http.StatusNotFound) ||
		strings.Contains(msg, "410") || strings.Contains(msg, "registration") {
		t.Skipf("%s: legacy runner registration-token flow unavailable on this instance: %v", action, err)
	}
	requireNoError(t, err, action)
}
