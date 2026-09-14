//go:build e2e

// usersshkeys_test.go covers SSH keys: the run user's own, from add to
// delete with the listing read between; the two administrator lookups that
// resolve a key to its owner, by ID and by fingerprint; and another user's
// keys, which an administrator adds, reads, lists and removes. Every key is
// generated fresh, since GitLab refuses a fingerprint any account already
// holds.

package common

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
	"golang.org/x/crypto/ssh"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUserSSHKeys_OwnAccount_AddGetListDelete adds a key to the run user on
// every surface, reads it by ID, finds it in the listing, deletes it and
// checks the listing no longer holds it.
//
// Replaces: TestMeta_UserSSHKeyLifecycle
func TestUserSSHKeys_OwnAccount_AddGetListDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		added, _ := addOwnSSHKey(e, s)

		got := harness.Do[users.SSHKeyOutput](s, actionUserGetSSHKey, map[string]any{"key_id": added.ID})
		if got.ID != added.ID || got.Title != added.Title {
			e.T.Errorf("get_ssh_key answered %d %q, want %d %q", got.ID, got.Title, added.ID, added.Title)
		}

		listed := harness.Do[users.SSHKeyListOutput](s, actionUserSSHKeys, map[string]any{"per_page": 100})
		if !containsID(userSSHKeyIDs(listed.Keys), added.ID) {
			e.T.Errorf("the run user's keys do not hold the added key %d: %+v", added.ID, listed.Keys)
		}

		deleted := harness.Do[users.DeleteSSHKeyOutput](s, actionUserDeleteSSHKey, map[string]any{"key_id": added.ID})
		if !deleted.Deleted || deleted.KeyID != added.ID {
			e.T.Errorf("delete_ssh_key answered %+v, want deleted=true for key %d", deleted, added.ID)
		}
		after := harness.Do[users.SSHKeyListOutput](s, actionUserSSHKeys, map[string]any{"per_page": 100})
		if containsID(userSSHKeyIDs(after.Keys), added.ID) {
			e.T.Errorf("key %d is still listed after its delete", added.ID)
		}
	})
}

// TestUserSSHKeys_Lookups_ByIDAndByFingerprint adds a key to the run user
// and resolves it back to the account twice, by its global ID and by the
// SHA256 fingerprint computed locally from the public key, on every surface.
// Both lookups answer administrators only.
//
// Replaces: TestIndividual_UserKeyByFingerprint, TestMeta_UserNamespacesNotifications
func TestUserSSHKeys_Lookups_ByIDAndByFingerprint(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	rt := e.Runtime()

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		added, publicKey := addOwnSSHKey(e, s)

		withUser := harness.Do[keys.Output](s, actionUserKeyGetWithUser, map[string]any{"key_id": added.ID})
		if withUser.ID != added.ID || withUser.User.ID != rt.UserID {
			e.T.Errorf("key_get_with_user answered key %d of user %d, want key %d of the run user %d", withUser.ID, withUser.User.ID, added.ID, rt.UserID)
		}

		// The fingerprint is computed from the key the test generated, not
		// from the one GitLab echoes, so the lookup is held to what was sent.
		fingerprint := sshFingerprint(e, publicKey)
		byFingerprint := harness.Do[keys.Output](s, actionUserKeyGetByFingerprint, map[string]any{"fingerprint": fingerprint})
		if byFingerprint.ID != added.ID || byFingerprint.User.Username != rt.Username {
			e.T.Errorf("key_get_by_fingerprint(%s) answered key %d of %q, want key %d of %q", fingerprint, byFingerprint.ID, byFingerprint.User.Username, added.ID, rt.Username)
		}
	})
}

// addOwnSSHKey adds a fresh key to the run user through the session and
// registers its removal through client-go, the safety net behind a scenario
// that deletes it through the server or does not delete it at all. It
// returns the key as GitLab answered and the public key that was sent.
func addOwnSSHKey(e *harness.Env, s *harness.Session) (users.SSHKeyOutput, string) {
	e.T.Helper()

	title := e.Name("key")
	publicKey := mintSSHPublicKey(e)
	added := harness.Do[users.SSHKeyOutput](s, actionUserAddSSHKey, map[string]any{"title": title, "key": publicKey})
	if added.ID == 0 || added.Title != title {
		e.T.Fatalf("add_ssh_key answered %+v, want a key titled %q with an ID", added, title)
	}
	e.Defer("ssh key "+title, func(ctx context.Context) error {
		_, err := e.Client().GL().Users.DeleteSSHKey(added.ID, gl.WithContext(ctx))
		if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
			return err
		}
		return nil
	})
	return added, publicKey
}

// TestUserSSHKeys_ForUser_AddGetListDelete gives one fixture user a key per
// surface, as an administrator: adds it, reads it by ID, finds it in the
// user's listing, deletes it and checks the listing no longer holds it.
//
// Replaces: TestMeta_UserAdmin
func TestUserSSHKeys_ForUser_AddGetListDelete(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "sshuser")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		owner := map[string]any{"user_id": user.ID}
		title := e.Name("key")

		added := harness.Do[users.SSHKeyOutput](s, actionUserAddSSHKeyForUser, withParams(owner, map[string]any{"title": title, "key": mintSSHPublicKey(e)}))
		if added.ID == 0 || added.Title != title {
			e.T.Fatalf("add_ssh_key_for_user answered %+v, want a key titled %q with an ID", added, title)
		}
		key := withParams(owner, map[string]any{"key_id": added.ID})

		got := harness.Do[users.SSHKeyOutput](s, actionUserGetSSHKeyForUser, key)
		if got.ID != added.ID || got.Title != title {
			e.T.Errorf("get_ssh_key_for_user answered %d %q, want %d %q", got.ID, got.Title, added.ID, title)
		}

		listed := harness.Do[users.SSHKeyListOutput](s, actionUserSSHKeysForUser, owner)
		if !containsID(userSSHKeyIDs(listed.Keys), added.ID) {
			e.T.Errorf("the keys of user %d do not hold the added key %d: %+v", user.ID, added.ID, listed.Keys)
		}

		deleted := harness.Do[users.DeleteSSHKeyOutput](s, actionUserDeleteSSHKeyForUser, key)
		if !deleted.Deleted || deleted.KeyID != added.ID {
			e.T.Errorf("delete_ssh_key_for_user answered %+v, want deleted=true for key %d", deleted, added.ID)
		}
		after := harness.Do[users.SSHKeyListOutput](s, actionUserSSHKeysForUser, owner)
		if containsID(userSSHKeyIDs(after.Keys), added.ID) {
			e.T.Errorf("key %d of user %d is still listed after its delete", added.ID, user.ID)
		}
	})
}

// mintSSHPublicKey generates a public key in authorized_keys form, failing
// the test if the generator did.
func mintSSHPublicKey(e *harness.Env) string {
	e.T.Helper()

	key, err := fixture.SSHPublicKey()
	if err != nil {
		e.T.Fatal(err)
	}
	return key
}

// sshFingerprint computes the SHA256 fingerprint of a public key in
// authorized_keys form, in the "SHA256:..." spelling GitLab indexes keys by.
func sshFingerprint(e *harness.Env, authorizedKey string) string {
	e.T.Helper()

	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		e.T.Fatalf("the key GitLab answered with does not parse as an authorized key: %v", err)
	}
	return ssh.FingerprintSHA256(parsed)
}

// userSSHKeyIDs collects the IDs of listed SSH keys.
func userSSHKeyIDs(listed []users.SSHKeyOutput) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, key := range listed {
		ids = append(ids, key.ID)
	}
	return ids
}
