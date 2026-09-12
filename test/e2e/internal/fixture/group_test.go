//go:build e2e

// group_test.go drives the group's permanent deletion against the stub.

package fixture

import (
	"context"
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
