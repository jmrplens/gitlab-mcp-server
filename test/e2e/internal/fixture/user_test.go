//go:build e2e

// user_test.go pins the key generation and the user deletion's tolerance.

package fixture

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestSSHPublicKey_Generated_ParsesAsAnEd25519AuthorizedKey checks the key
// is in the form GitLab accepts, and that two calls give two keys, since an
// instance refuses a fingerprint it already holds.
func TestSSHPublicKey_Generated_ParsesAsAnEd25519AuthorizedKey(t *testing.T) {
	first, err := SSHPublicKey()
	if err != nil {
		t.Fatalf("SSHPublicKey() error = %v", err)
	}
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(first))
	if err != nil {
		t.Fatalf("the key does not parse as an authorized key: %v", err)
	}
	if got := parsed.Type(); got != ssh.KeyAlgoED25519 {
		t.Errorf("key type = %q, want %q", got, ssh.KeyAlgoED25519)
	}
	if !strings.HasSuffix(first, "\n") {
		t.Errorf("key %q does not end with the newline authorized_keys carries", first)
	}

	second, err := SSHPublicKey()
	if err != nil {
		t.Fatalf("second SSHPublicKey() error = %v", err)
	}
	if first == second {
		t.Errorf("two generated keys are identical")
	}
}

// TestDeleteUser_States_DeletesAndToleratesGone checks that a user is hard
// deleted and that one already gone is not an error, which is the state a
// rejected or deleted user is left in.
func TestDeleteUser_States_DeletesAndToleratesGone(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addUser(30, "disposable")

	if err := DeleteUser(context.Background(), client, 30); err != nil {
		t.Fatalf("DeleteUser() error = %v, want nil", err)
	}
	if err := DeleteUser(context.Background(), client, 30); err != nil {
		t.Errorf("DeleteUser(gone) error = %v, want nil", err)
	}
	if got := stub.recordedDeletes(); len(got) != 1 || got[0] != "user 30" {
		t.Errorf("deletes = %q, want one deletion of user 30", got)
	}
}

// TestUserEmailDomain_Reserved_DeliversNowhere pins the address domain to
// one RFC 2606 reserves, so a confirmation mail can never reach anyone.
func TestUserEmailDomain_Reserved_DeliversNowhere(t *testing.T) {
	if !strings.HasSuffix(userEmailDomain, ".invalid") {
		t.Errorf("userEmailDomain = %q, want a .invalid domain", userEmailDomain)
	}
}
