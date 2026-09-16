//go:build e2e

// access_token_test.go drives the project access token builder's pure halves
// against the stub: what a create sends and reads back, and the two endings a
// revocation has.

package fixture

import (
	"net/http"
	"testing"
	"time"
)

// TestCreateProjectAccessToken_Created_ReadsTheSecretAndTheID checks that the
// builder sends the scope and access level a fixture token needs and reads
// back the three values a case addresses it by.
func TestCreateProjectAccessToken_Created_ReadsTheSecretAndTheID(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/7/access_tokens", stubCreated(map[string]any{
		"id": 41, "name": "e2e-pat", "token": "glpat-fixture",
	}))

	got, err := createProjectAccessToken(t.Context(), client, 7, "e2e-pat", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("createProjectAccessToken() error = %v, want nil", err)
	}
	want := ProjectAccessToken{ID: 41, Name: "e2e-pat", Value: "glpat-fixture", ProjectID: 7}
	if got != want {
		t.Errorf("createProjectAccessToken() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createProjectAccessToken() sent %d requests, want 1", len(requests))
	}
	body := requests[0].Body
	if body["access_level"] != float64(40) {
		t.Errorf("createProjectAccessToken() sent access_level %v, want the maintainer level 40", body["access_level"])
	}
	scopes, _ := body["scopes"].([]any)
	if len(scopes) != 1 || scopes[0] != "api" {
		t.Errorf("createProjectAccessToken() sent scopes %v, want [api]", body["scopes"])
	}
}

// TestRevokeProjectAccessToken_Endings_ToleratesOneACaseRevoked checks that a
// token that is no longer there is not a cleanup failure, and that any other
// refusal is.
func TestRevokeProjectAccessToken_Endings_ToleratesOneACaseRevoked(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "revoked", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/7/access_tokens/41", tc.answer)

			err := revokeProjectAccessToken(t.Context(), client, 7, 41)
			if (err != nil) != tc.wantErr {
				t.Errorf("revokeProjectAccessToken() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
