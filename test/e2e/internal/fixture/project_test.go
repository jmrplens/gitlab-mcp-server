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

// TestDeleteProject_TransferUnderWay_WaitsThroughTheRefusedMark checks the
// cleanup of a project GitLab 19.4 is still moving: the mark is refused with
// 400 "State cannot transition via ..." until the transfer lets go of it, and
// the deletion marks it once it does rather than failing on the first refusal.
func TestDeleteProject_TransferUnderWay_WaitsThroughTheRefusedMark(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(10, "e2e-moving", "user/e2e-moving")
	stub.configure(func() { stub.projects[10].transferMarkRefusals = 1 })

	if err := DeleteProject(context.Background(), client, 10, "user/e2e-moving"); err != nil {
		t.Fatalf("DeleteProject() error = %v, want nil once the transfer lets go", err)
	}
	want := []string{
		"project 10 ",
		"project 10 ",
		"project 10 full_path=user%2Fe2e-moving-deletion_scheduled-10&permanently_remove=true",
	}
	if got := stub.recordedDeletes(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("deletes = %q, want the refused mark, the mark and the removal %q", got, want)
	}
	if projects, _ := stub.remaining(); len(projects) != 0 {
		t.Errorf("projects left = %v, want none", projects)
	}
}

// TestDeleteProject_TransferNeverLetsGo_ReportsTheLastState checks that the
// wait through a transfer is bounded by the cleanup context, and that running
// out of it names the transfer the deletion was waiting on.
func TestDeleteProject_TransferNeverLetsGo_ReportsTheLastState(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(11, "e2e-stuck", "user/e2e-stuck")
	stub.configure(func() { stub.projects[11].transferMarkRefusals = 1 << 20 })
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	err := DeleteProject(ctx, client, 11, "user/e2e-stuck")
	if err == nil {
		t.Fatal("DeleteProject() error = nil, want the transfer the mark waited on reported")
	}
	if !strings.Contains(err.Error(), "marking project 11") || !strings.Contains(err.Error(), "is in a transfer") {
		t.Errorf("DeleteProject() error = %v, want the mark named with the transfer it waited on", err)
	}
	if projects, _ := stub.remaining(); len(projects) != 1 {
		t.Errorf("projects left = %v, want the one that never let go", projects)
	}
}

// TestDeleteProject_TransferRefusalUnderAnotherStatus_IsAnError checks the
// wait reads the status and not only the words: GitLab refuses a mark during
// a transfer with 400, and the same words under a 503 are not waited through.
func TestDeleteProject_TransferRefusalUnderAnotherStatus_IsAnError(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(12, "e2e-odd", "user/e2e-odd")
	stub.configure(func() {
		stub.projects[12].transferMarkRefusals = 1
		stub.projects[12].transferMarkStatus = http.StatusServiceUnavailable
	})

	err := DeleteProject(context.Background(), client, 12, "user/e2e-odd")
	if err == nil {
		t.Fatal("DeleteProject() error = nil, want the 503 reported")
	}
	if !IsStatus(err, http.StatusServiceUnavailable) || !strings.Contains(err.Error(), "marking project 12") {
		t.Errorf("DeleteProject() error = %v, want the mark's 503", err)
	}
	if got := stub.recordedDeletes(); len(got) != 1 {
		t.Errorf("deletes = %q, want the one refused mark and no wait", got)
	}
}

// scriptedDeletion is a deletion's three steps answering from lists, the last
// answer repeating, with what each was asked recorded, for the orderings a
// stub GitLab cannot be told to produce.
type scriptedDeletion struct {
	marks   []error
	reads   []error
	removes []error
	paths   []string

	marked, read, removed int
	removedUnder          []string
}

// deletionAnswer answers call n from answers, repeating the last.
func deletionAnswer(answers []error, n int) error {
	return answers[min(n, len(answers)-1)]
}

// steps is the deletionSteps the script answers through.
func (d *scriptedDeletion) steps() deletionSteps {
	return deletionSteps{
		mark: func(context.Context) error {
			d.marked++
			return deletionAnswer(d.marks, d.marked-1)
		},
		currentPath: func(context.Context) (string, error) {
			d.read++
			return d.paths[min(d.read, len(d.paths))-1], deletionAnswer(d.reads, d.read-1)
		},
		remove: func(_ context.Context, path string) error {
			d.removed++
			d.removedUnder = append(d.removedUnder, path)
			return deletionAnswer(d.removes, d.removed-1)
		},
	}
}

// TestDeletePermanently_GoneWhenMarkedAgain_IsNotAnError checks the second
// mark a landed transfer calls for: an object gone by then is as deleted as
// the cleanup asked, and nothing is removed.
func TestDeletePermanently_GoneWhenMarkedAgain_IsNotAnError(t *testing.T) {
	d := &scriptedDeletion{
		marks:   []error{nil, statusError(http.StatusNotFound, "404 Project Not Found")},
		reads:   []error{nil},
		removes: []error{statusError(http.StatusBadRequest, "Project must be marked for deletion first.")},
		paths:   []string{"user/p-deletion_scheduled-1"},
	}

	if err := deletePermanently(context.Background(), "project", 1, "user/p", d.steps()); err != nil {
		t.Fatalf("deletePermanently() error = %v, want nil for an object gone at the second mark", err)
	}
	if d.marked != 2 || d.removed != 1 {
		t.Errorf("marks = %d, removals = %d, want 2 and 1", d.marked, d.removed)
	}
}

// TestDeletePermanently_ReadBackGone_IsNotAnError checks that an object the
// read after the mark no longer finds is not removed: it is gone, which is
// what the cleanup asked for.
func TestDeletePermanently_ReadBackGone_IsNotAnError(t *testing.T) {
	d := &scriptedDeletion{
		marks: []error{nil},
		reads: []error{statusError(http.StatusNotFound, "404 Project Not Found")},
		paths: []string{""},
	}

	if err := deletePermanently(context.Background(), "project", 1, "user/p", d.steps()); err != nil {
		t.Fatalf("deletePermanently() error = %v, want nil", err)
	}
	if d.removed != 0 {
		t.Errorf("removals = %d, want none for an object that is gone", d.removed)
	}
}

// TestDeletePermanently_ReadBackFails_RemovesUnderThePathItKnew checks that a
// read failing for another reason than the object being gone does not stop
// the deletion: it removes the object under the path it was given.
func TestDeletePermanently_ReadBackFails_RemovesUnderThePathItKnew(t *testing.T) {
	d := &scriptedDeletion{
		marks:   []error{nil},
		reads:   []error{statusError(http.StatusForbidden, "403 Forbidden")},
		removes: []error{nil},
		paths:   []string{"ignored"},
	}

	if err := deletePermanently(context.Background(), "project", 1, "user/p", d.steps()); err != nil {
		t.Fatalf("deletePermanently() error = %v, want nil", err)
	}
	if strings.Join(d.removedUnder, ",") != "user/p" {
		t.Errorf("removed under %q, want the path it was given", d.removedUnder)
	}
}

// TestDeletePermanently_RemovalRefusedTwice_IsAnError checks that the second
// mark a landed transfer calls for is made once: a removal refused again for
// the same reason is reported rather than retried without end.
func TestDeletePermanently_RemovalRefusedTwice_IsAnError(t *testing.T) {
	d := &scriptedDeletion{
		marks:   []error{nil},
		reads:   []error{nil},
		removes: []error{statusError(http.StatusBadRequest, "Project must be marked for deletion first.")},
		paths:   []string{"user/p-deletion_scheduled-1"},
	}

	err := deletePermanently(context.Background(), "project", 1, "user/p", d.steps())
	if err == nil || !strings.Contains(err.Error(), "permanently deleting project 1") {
		t.Fatalf("deletePermanently() error = %v, want the second refusal reported", err)
	}
	if d.marked != 2 || d.removed != 2 {
		t.Errorf("marks = %d, removals = %d, want 2 and 2", d.marked, d.removed)
	}
}

// TestDeletePermanently_UnmarkedRefusalUnderAnotherStatus_IsNotRetried checks
// the status is part of the match for the removal's refusal too: the words
// under a 409 are reported, not answered with a second mark.
func TestDeletePermanently_UnmarkedRefusalUnderAnotherStatus_IsNotRetried(t *testing.T) {
	d := &scriptedDeletion{
		marks:   []error{nil},
		reads:   []error{nil},
		removes: []error{statusError(http.StatusConflict, "Project must be marked for deletion first.")},
		paths:   []string{"user/p-deletion_scheduled-1"},
	}

	if err := deletePermanently(context.Background(), "project", 1, "user/p", d.steps()); err == nil {
		t.Fatal("deletePermanently() error = nil, want the 409 reported")
	}
	if d.marked != 1 || d.removed != 1 {
		t.Errorf("marks = %d, removals = %d, want 1 and 1", d.marked, d.removed)
	}
}

// TestDeleteProject_TransferLandsAfterTheMark_MarksAgainAndRemoves checks the
// other half of a transfer racing a cleanup: the move lands between the mark
// and the removal, which leaves the project unmarked under its new path, and
// the removal refused with 400 "... must be marked for deletion first" is
// answered by marking it again and removing it under the path read back.
func TestDeleteProject_TransferLandsAfterTheMark_MarksAgainAndRemoves(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(13, "e2e-late", "user/e2e-late")
	stub.configure(func() {
		stub.projects[13].transferLandsAfterMark = true
		stub.projects[13].transferredPath = "group/e2e-late"
	})

	if err := DeleteProject(context.Background(), client, 13, "user/e2e-late"); err != nil {
		t.Fatalf("DeleteProject() error = %v, want nil", err)
	}
	want := []string{
		"project 13 ",
		"project 13 full_path=user%2Fe2e-late-deletion_scheduled-13&permanently_remove=true",
		"project 13 ",
		"project 13 full_path=group%2Fe2e-late-deletion_scheduled-13&permanently_remove=true",
	}
	if got := stub.recordedDeletes(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("deletes = %q, want mark, refused removal, mark, removal %q", got, want)
	}
	if projects, _ := stub.remaining(); len(projects) != 0 {
		t.Errorf("projects left = %v, want none", projects)
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

// TestDrainSidekiqWithin_QueueEmpties_ReportsDrained checks that a drain
// which saw the queues empty says so, which is what lets a transfer scenario
// hold the action to a move landing inside its wait.
func TestDrainSidekiqWithin_QueueEmpties_ReportsDrained(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.enqueued = 3 })

	if !DrainSidekiqWithin(context.Background(), client, 5*time.Second) {
		t.Error("DrainSidekiqWithin() = false for queues that emptied, want true")
	}
}

// TestDrainSidekiqWithin_QueueNeverEmpties_ReportsNotDrainedAtTheBound checks
// that a queue that stays busy ends the drain at the caller's bound, and is
// reported as not drained rather than passed off as empty.
func TestDrainSidekiqWithin_QueueNeverEmpties_ReportsNotDrainedAtTheBound(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.enqueued = 1 << 20 })

	const bound = 600 * time.Millisecond
	start := time.Now()
	drained := DrainSidekiqWithin(context.Background(), client, bound)
	elapsed := time.Since(start)
	if drained {
		t.Error("DrainSidekiqWithin() = true for queues that never emptied, want false")
	}
	if elapsed < bound || elapsed > bound+5*time.Second {
		t.Errorf("DrainSidekiqWithin() returned after %s, want the %s bound", elapsed, bound)
	}
}

// TestDrainSidekiqWithin_ContextEnded_ReportsNotDrained checks that a read
// refused because the context ended is not mistaken for a token that cannot
// read the metrics: the queues were never seen, so they are not drained.
func TestDrainSidekiqWithin_ContextEnded_ReportsNotDrained(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.enqueued = 1 << 20 })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if DrainSidekiqWithin(ctx, client, 5*time.Second) {
		t.Error("DrainSidekiqWithin() = true with a cancelled context, want false")
	}
}

// TestDrainSidekiqWithin_MetricsRefused_ReportsDrained pins the behavior a
// token that is not an administrator's has always had: nothing is known to be
// waiting, so the drain returns at once and reports nothing to wait for.
func TestDrainSidekiqWithin_MetricsRefused_ReportsDrained(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() { stub.sidekiqRefused = true })

	start := time.Now()
	if !DrainSidekiqWithin(context.Background(), client, 5*time.Second) {
		t.Error("DrainSidekiqWithin() = false when the metrics are refused, want true")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("DrainSidekiqWithin() took %s when the metrics are refused, want an immediate return", elapsed)
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

// TestCreateProject_OwnedBy_CreatesInTheUsersNamespace checks that a project
// asked for in another user's namespace is created through the administrator
// route that takes the user, with the options any project is created with,
// and that one asked for with no owner goes to the ordinary route.
func TestCreateProject_OwnedBy_CreatesInTheUsersNamespace(t *testing.T) {
	cases := []struct {
		name  string
		opts  []ProjectOption
		route string
	}{
		{name: "owned by a user", opts: []ProjectOption{OwnedBy(User{ID: 9})}, route: "/api/v4/projects/user/9"},
		{name: "the run user's", route: "/api/v4/projects"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, e := detachedStub(t)
			stub.answers(http.MethodPost, tc.route, stubCreated(map[string]any{
				"id": 77, "name": "proj", "path_with_namespace": "someone/proj", "default_branch": DefaultBranch,
				"namespace": map[string]any{"id": 33},
			}))
			spec := projectSpec{name: "proj", visibility: gl.PrivateVisibility, readme: true}
			for _, opt := range tc.opts {
				opt(&spec)
			}

			project, err := createProject(e, spec, func() string { return "proj-name" })
			if err != nil {
				t.Fatalf("createProject() error = %v", err)
			}
			if project.ID != 77 || project.NamespaceID != 33 || project.Path != "someone/proj" {
				t.Errorf("createProject() = %+v, want the project the route answered with", project)
			}
			requests := stub.recordedRequests()
			if len(requests) != 1 || requests[0].Body["name"] != "proj-name" || requests[0].Body["initialize_with_readme"] != true {
				t.Errorf("the creation sent %+v, want one request carrying the name and the README", requests)
			}
		})
	}
}
