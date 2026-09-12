//go:build e2e

// group_test.go drives the group's permanent deletion against the stub.

package fixture

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestDeleteGroup_Unmarked_MarksThenRemovesUnderTheRenamedPath checks that a
// group goes through the same two steps a project does: mark, re-read the
// renamed path, remove permanently under it.
func TestDeleteGroup_Unmarked_MarksThenRemovesUnderTheRenamedPath(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addGroup(3, "e2e-grp", "e2e-grp")

	if err := DeleteGroup(context.Background(), client, 3, "e2e-grp"); err != nil {
		t.Fatalf("DeleteGroup() error = %v, want nil", err)
	}
	want := []string{
		"group 3 ",
		"group 3 full_path=e2e-grp-deletion_scheduled-3&permanently_remove=true",
	}
	if got := stub.recordedDeletes(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("deletes = %q, want %q", got, want)
	}
	if _, groups := stub.remaining(); len(groups) != 0 {
		t.Errorf("groups left = %v, want none", groups)
	}
}

// TestDeleteGroup_AlreadyMarked_StillRemovesIt checks that a group a test
// already marked is removed rather than refused.
func TestDeleteGroup_AlreadyMarked_StillRemovesIt(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addGroup(5, "e2e-marked", "e2e-marked")
	if _, err := client.GL().Groups.DeleteGroup(int64(5), nil); err != nil {
		t.Fatalf("marking through the client: %v", err)
	}

	if err := DeleteGroup(context.Background(), client, 5, "e2e-marked"); err != nil {
		t.Fatalf("DeleteGroup() error = %v, want nil", err)
	}
	if _, groups := stub.remaining(); len(groups) != 0 {
		t.Errorf("groups left = %v, want none", groups)
	}
}

// TestDeleteGroup_TopLevelOnDelayedDeletion_MarksAndToleratesTheRefusal checks
// the teardown of the World's own top-level group on an instance with delayed
// deletion: the mark step schedules it, the permanent-remove step is refused
// because that option is subgroups-only, and DeleteGroup treats the refusal as
// success because the group is as gone as the API allows.
//
// It pins the fix for the failure the first World teardown hit on GitLab CE 18:
// DELETE /groups/<id>?permanently_remove=true answering 400 "`permanently_remove`
// option is only available for subgroups."
func TestDeleteGroup_TopLevelOnDelayedDeletion_MarksAndToleratesTheRefusal(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addTopLevelGroup(6, "e2e-world-group", "e2e-world-group")

	if err := DeleteGroup(context.Background(), client, 6, "e2e-world-group"); err != nil {
		t.Fatalf("DeleteGroup() error = %v, want nil for a marked top-level group the API cannot purge", err)
	}
	want := []string{
		"group 6 ",
		"group 6 full_path=e2e-world-group-deletion_scheduled-6&permanently_remove=true",
	}
	if got := stub.recordedDeletes(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("deletes = %q, want the mark then the refused permanent-remove %q", got, want)
	}
}

// TestDeleteGroup_RefusalUnderAnotherStatus_IsAnError checks the tolerance
// above reads the status and not only the words: the same message under a
// 503 is an answer GitLab does not give for a top-level group, and swallowing
// it would report a teardown that left the group behind.
func TestDeleteGroup_RefusalUnderAnotherStatus_IsAnError(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addTopLevelGroupRefusing(7, "e2e-world-group", "e2e-world-group", http.StatusServiceUnavailable)

	err := DeleteGroup(context.Background(), client, 7, "e2e-world-group")
	if err == nil {
		t.Fatal("DeleteGroup() error = nil, want the refusal reported when it is not the 400 GitLab answers with")
	}
	if !IsStatus(err, http.StatusServiceUnavailable) {
		t.Errorf("DeleteGroup() error = %v, want the 503 the stub answered", err)
	}
}

// TestDeleteGroup_Gone_IsNotAnError checks that a group nothing can find
// counts as deleted.
func TestDeleteGroup_Gone_IsNotAnError(t *testing.T) {
	_, client := newStubGitLab(t)
	if err := DeleteGroup(context.Background(), client, 4, "never"); err != nil {
		t.Fatalf("DeleteGroup() error = %v, want nil for a group that is gone", err)
	}
}

// TestGroupOf_Fields_ReadsWhatATestNeeds checks the projection of what
// GitLab returned into what a test refers to.
func TestGroupOf_Fields_ReadsWhatATestNeeds(t *testing.T) {
	got := groupOf(&gl.Group{ID: 9, Name: "Sub", FullPath: "parent/sub", ParentID: 2})
	want := Group{ID: 9, Path: "parent/sub", Name: "Sub", ParentID: 2}
	if got != want {
		t.Errorf("groupOf() = %+v, want %+v", got, want)
	}
}
