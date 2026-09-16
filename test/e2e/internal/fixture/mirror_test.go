//go:build e2e

// mirror_test.go drives the mirror builder's pure halves against the stub:
// that a mirror is created switched off, and the two endings its removal has.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateProjectMirror_Created_IsDisabled pins the one decision the builder
// makes: an enabled mirror starts pushing as soon as GitLab notices it, and
// the timing of that background job would decide what a case reads back.
func TestCreateProjectMirror_Created_IsDisabled(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/13/remote_mirrors", stubCreated(map[string]any{
		"id": 4, "enabled": false,
	}))

	got, err := createProjectMirror(t.Context(), client, 13, "https://gitlab.invalid/target.git")
	if err != nil {
		t.Fatalf("createProjectMirror() error = %v, want nil", err)
	}
	want := Mirror{ID: 4, Enabled: false}
	if got != want {
		t.Errorf("createProjectMirror() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectMirror() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["enabled"] != false {
		t.Errorf("createProjectMirror() sent %v, want a mirror that is switched off", requests[0].Body)
	}
}

// TestDeleteProjectMirror_Endings_ToleratesOneACaseDeleted checks that a mirror
// a case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteProjectMirror_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/13/remote_mirrors/4", tc.answer)

			err := deleteProjectMirror(t.Context(), client, 13, 4)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectMirror() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
