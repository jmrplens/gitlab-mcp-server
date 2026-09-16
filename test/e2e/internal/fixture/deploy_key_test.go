//go:build e2e

// deploy_key_test.go drives the deploy key builder's pure halves against the
// stub, and checks the one thing the fixture promises beyond a create: that
// the key it offers is a freshly generated one rather than a constant two
// attempts would collide on.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateDeployKey_Created_SendsTheGeneratedKeyReadOnly checks that the
// builder offers the public key it was handed, asks for no push access, and
// reads back what a case addresses the key by.
func TestCreateDeployKey_Created_SendsTheGeneratedKeyReadOnly(t *testing.T) {
	publicKey, err := SSHPublicKey()
	if err != nil {
		t.Fatalf("SSHPublicKey() error = %v, want nil", err)
	}
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/5/deploy_keys", stubCreated(map[string]any{
		"id": 9, "title": "e2e-key", "key": publicKey,
	}))

	got, err := createDeployKey(t.Context(), client, 5, "e2e-key", publicKey)
	if err != nil {
		t.Fatalf("createDeployKey() error = %v, want nil", err)
	}
	if got.ID != 9 || got.Title != "e2e-key" || got.Key != publicKey {
		t.Errorf("createDeployKey() = %+v, want the key GitLab answered with", got)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createDeployKey() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["key"] != publicKey {
		t.Errorf("createDeployKey() sent key %v, want the generated public key", requests[0].Body["key"])
	}
	if requests[0].Body["can_push"] != false {
		t.Errorf("createDeployKey() sent can_push %v, want false", requests[0].Body["can_push"])
	}
}

// TestSSHPublicKey_TwoCalls_DifferSoTwoFixturesNeverCollide checks the reason
// the key is generated rather than written down: GitLab holds a deploy key's
// fingerprint unique across the instance.
func TestSSHPublicKey_TwoCalls_DifferSoTwoFixturesNeverCollide(t *testing.T) {
	first, err := SSHPublicKey()
	if err != nil {
		t.Fatalf("SSHPublicKey() error = %v, want nil", err)
	}
	second, err := SSHPublicKey()
	if err != nil {
		t.Fatalf("SSHPublicKey() error = %v, want nil", err)
	}
	if first == second {
		t.Error("SSHPublicKey() returned the same key twice, which two deploy key fixtures would collide on")
	}
}

// TestDeleteDeployKey_Endings_ToleratesOneACaseDeleted checks that a key a
// case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteDeployKey_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/projects/5/deploy_keys/9", tc.answer)

			err := deleteDeployKey(t.Context(), client, 5, 9)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteDeployKey() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
