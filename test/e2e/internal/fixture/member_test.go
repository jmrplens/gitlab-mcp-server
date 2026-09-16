//go:build e2e

// member_test.go drives the project membership helpers' pure halves against
// the stub: what an add sends, and the three answers the read-back
// distinguishes, including the not-found that means "not a member" rather than
// a broken world.

package fixture

import (
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestAddProjectMember_Added_SendsTheUserAndItsAccess checks that the helper
// names both halves of a membership.
func TestAddProjectMember_Added_SendsTheUserAndItsAccess(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/4/members", stubCreated(map[string]any{
		"id": 21, "username": "member", "access_level": 30,
	}))

	if err := addProjectMember(t.Context(), client, 4, 21, gl.DeveloperPermissions); err != nil {
		t.Fatalf("addProjectMember() error = %v, want nil", err)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("addProjectMember() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["user_id"] == nil || requests[0].Body["access_level"] == nil {
		t.Errorf("addProjectMember() sent %v, want the user and its access level", requests[0].Body)
	}
}

// TestProjectHasMember_Answers covers the three answers the read-back
// distinguishes: a membership a case removed answers 404, which is a "no".
func TestProjectHasMember_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "a member", answer: stubOK(map[string]any{"id": 21, "username": "member"}), want: true},
		{name: "not a member", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/projects/4/members/21", tc.answer)

			got, err := ProjectHasMember(t.Context(), client, 4, 21)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ProjectHasMember() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("ProjectHasMember() = %t, want %t", got, tc.want)
			}
		})
	}
}
