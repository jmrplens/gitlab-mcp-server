//go:build e2e

// project_test.go drives the project's pure halves against the stub: the
// two-step permanent delete, the branch readiness wait and the Sidekiq
// drain.

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

// TestDeleteProject_Unmarked_MarksThenRemovesUnderTheRenamedPath checks the
// dance a delayed-deletion instance demands: mark, re-read the renamed path,
// remove permanently under it.
func TestDeleteProject_Unmarked_MarksThenRemovesUnderTheRenamedPath(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(7, "e2e-proj", "user/e2e-proj")

	if err := DeleteProject(context.Background(), client, 7, "user/e2e-proj"); err != nil {
		t.Fatalf("DeleteProject() error = %v, want nil", err)
	}

	want := []string{
		"project 7 ",
		"project 7 full_path=user%2Fe2e-proj-deletion_scheduled-7&permanently_remove=true",
	}
	if got := stub.recordedDeletes(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("deletes = %q, want %q", got, want)
	}
	if projects, _ := stub.remaining(); len(projects) != 0 {
		t.Errorf("projects left = %v, want none", projects)
	}
}

// TestDeleteProject_AlreadyMarked_StillRemovesIt checks that a project a
// test already marked (its own delete scenario, say) is removed rather than
// refused on the first step's "already marked" answer.
func TestDeleteProject_AlreadyMarked_StillRemovesIt(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(8, "e2e-marked", "user/e2e-marked")
	if _, err := client.GL().Projects.DeleteProject(int64(8), nil); err != nil {
		t.Fatalf("marking through the client: %v", err)
	}

	if err := DeleteProject(context.Background(), client, 8, "user/e2e-marked"); err != nil {
		t.Fatalf("DeleteProject() error = %v, want nil", err)
	}
	if projects, _ := stub.remaining(); len(projects) != 0 {
		t.Errorf("projects left = %v, want none", projects)
	}
}

// TestDeleteProject_Gone_IsNotAnError checks that a project nothing can find
// counts as deleted, which is what a test that deleted its own fixture leaves
// for the ledger.
func TestDeleteProject_Gone_IsNotAnError(t *testing.T) {
	stub, client := newStubGitLab(t)

	if err := DeleteProject(context.Background(), client, 9, "user/never"); err != nil {
		t.Fatalf("DeleteProject() error = %v, want nil for a project that is gone", err)
	}
	if got := stub.recordedDeletes(); len(got) != 1 {
		t.Errorf("deletes = %q, want only the first step", got)
	}
}

// TestWaitForBranch_NotVisibleYet_KeepsPollingUntilItIs checks that the
// 404 a branch answers while GitLab writes it is waited through.
func TestWaitForBranch_NotVisibleYet_KeepsPollingUntilItIs(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(1, "p", "user/p")
	stub.configure(func() { stub.branchMisses = 2 })

	if err := waitForBranch(context.Background(), client, 1, "main", 5*time.Second); err != nil {
		t.Fatalf("waitForBranch() error = %v, want nil once the branch appears", err)
	}
}

// TestWaitForBranch_NeverVisible_ReportsTheTimeoutWithTheLastState checks
// the failure names what was last seen, since "timed out" alone is nothing a
// reader can act on.
func TestWaitForBranch_NeverVisible_ReportsTheTimeoutWithTheLastState(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(1, "p", "user/p")
	stub.configure(func() { stub.branchMisses = 1 << 20 })

	err := waitForBranch(context.Background(), client, 1, "main", 300*time.Millisecond)
	if !errors.Is(err, harness.ErrPollTimeout) {
		t.Fatalf("waitForBranch() error = %v, want a poll timeout", err)
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("timeout error = %q, want it to carry the last HTTP status seen", err)
	}
}

// TestConvergingStatus_Codes_NamesWhatIsWaitedThrough pins which answers a
// readiness wait polls through and which end it.
func TestConvergingStatus_Codes_NamesWhatIsWaitedThrough(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{name: "not found", status: http.StatusNotFound, want: true},
		{name: "rate limited", status: http.StatusTooManyRequests, want: true},
		{name: "server error", status: http.StatusBadGateway, want: true},
		{name: "forbidden", status: http.StatusForbidden, want: false},
		{name: "ok", status: http.StatusOK, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := convergingStatus(testCase.status); got != testCase.want {
				t.Errorf("convergingStatus(%d) = %t, want %t", testCase.status, got, testCase.want)
			}
		})
	}
}

// TestDrainSidekiq_Queue_ReturnsOnceEmpty checks that the drain reads the
// stats until nothing is enqueued and then stops asking.
func TestDrainSidekiq_Queue_ReturnsOnceEmpty(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.enqueued = 3 })

	start := time.Now()
	DrainSidekiq(context.Background(), client)

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("DrainSidekiq took %s, want a few polls", elapsed)
	}
	var left int64
	stub.configure(func() { left = stub.enqueued })
	if left != 0 {
		t.Errorf("enqueued after the drain = %d, want 0", left)
	}
}

// TestDrainSidekiq_ContextEnded_ReturnsAtOnce checks that a drain does not
// outlive the test that asked for it.
func TestDrainSidekiq_ContextEnded_ReturnsAtOnce(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.enqueued = 1 << 20 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	DrainSidekiq(ctx, client)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("DrainSidekiq took %s with a cancelled context, want an immediate return", elapsed)
	}
}

// TestProjectOf_Namespace_ReadsTheNamespaceID checks the one field that is
// read through a pointer and would be silently zero if it were not.
func TestProjectOf_Namespace_ReadsTheNamespaceID(t *testing.T) {
	got := projectOf(&gl.Project{ID: 5, Name: "n", PathWithNamespace: "g/n", DefaultBranch: "main", HTTPURLToRepo: "http://x/g/n.git", Namespace: &gl.ProjectNamespace{ID: 42}})
	want := Project{ID: 5, Path: "g/n", Name: "n", DefaultBranch: "main", HTTPURLToRepo: "http://x/g/n.git", NamespaceID: 42}
	if got != want {
		t.Errorf("projectOf() = %+v, want %+v", got, want)
	}
	if bare := projectOf(&gl.Project{ID: 6}); bare.NamespaceID != 0 {
		t.Errorf("projectOf(no namespace).NamespaceID = %d, want 0", bare.NamespaceID)
	}
}

// TestProjectIDParam_SpellsTheIDAsTheSchemaDeclaresIt pins the one spelling
// of a project ID that every surface accepts: the decimal string project_id
// is declared as, which the individual surface refuses a number in place of.
func TestProjectIDParam_SpellsTheIDAsTheSchemaDeclaresIt(t *testing.T) {
	if got, want := (Project{ID: 405}).IDParam(), "405"; got != want {
		t.Errorf("IDParam() = %q, want %q", got, want)
	}
}
