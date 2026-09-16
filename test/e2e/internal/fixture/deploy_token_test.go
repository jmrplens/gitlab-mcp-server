//go:build e2e

// deploy_token_test.go drives the deploy token builder's pure halves against
// the stub: the scope it asks for, the username it lets GitLab choose, and
// the two endings a deletion has.

package fixture

import (
	"net/http"
	"testing"
	"time"
)

// TestCreateProjectDeployToken_Created_AsksForReadOnlyAndTakesGitLabsUsername
// checks that the builder asks for the narrowest scope a token can have and
// reads back the username GitLab generated rather than one of its own.
func TestCreateProjectDeployToken_Created_AsksForReadOnlyAndTakesGitLabsUsername(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/11/deploy_tokens", stubCreated(map[string]any{
		"id": 4, "name": "e2e-token", "username": "gitlab+deploy-token-4", "token": "gldt-fixture",
	}))

	got, err := createProjectDeployToken(t.Context(), client, 11, "e2e-token", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("createProjectDeployToken() error = %v, want nil", err)
	}
	want := DeployToken{ID: 4, Name: "e2e-token", Username: "gitlab+deploy-token-4", Value: "gldt-fixture"}
	if got != want {
		t.Errorf("createProjectDeployToken() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectDeployToken() sent %d requests, want 1", len(requests))
	}
	scopes, _ := requests[0].Body["scopes"].([]any)
	if len(scopes) != 1 || scopes[0] != "read_repository" {
		t.Errorf("createProjectDeployToken() sent scopes %v, want [read_repository]", requests[0].Body["scopes"])
	}
	if _, sentUsername := requests[0].Body["username"]; sentUsername {
		t.Error("createProjectDeployToken() sent a username; GitLab generates one that cannot collide")
	}
}

// TestDeleteProjectDeployToken_Endings_ToleratesOneACaseDeleted checks that a
// token a case already deleted is not a cleanup failure and any other refusal
// is.
func TestDeleteProjectDeployToken_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/11/deploy_tokens/4", tc.answer)

			err := deleteProjectDeployToken(t.Context(), client, 11, 4)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteProjectDeployToken() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
