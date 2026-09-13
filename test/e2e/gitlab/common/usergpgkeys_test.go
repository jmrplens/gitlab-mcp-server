//go:build e2e

// usergpgkeys_test.go covers GPG keys: the run user's own, from add to
// delete with the read by ID and the listing between; and another user's,
// which an administrator adds, reads, lists and removes. Every key is
// generated fresh by the fixture library, which is what lets the scenarios
// run on every surface and on any instance: GitLab refuses a fingerprint
// any account already holds, and the suite this replaces embedded two fixed
// keys and could add each once.

package common

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUserGPGKeys_OwnAccount_AddGetListDelete adds a key to the run user on
// every surface, reads it by ID, finds it in the listing, deletes it and
// checks the listing no longer holds it.
//
// Replaces: TestIndividual_UserGPGKeys
func TestUserGPGKeys_OwnAccount_AddGetListDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		added := harness.Do[usergpgkeys.Output](s, actionUserAddGPGKey, map[string]any{"key": newGPGPublicKey(e, "own-"+string(surface))})
		if added.ID == 0 || added.Key == "" {
			e.T.Fatalf("add_gpg_key answered %+v, want a key with an ID", added)
		}
		e.Defer("gpg key", func(ctx context.Context) error {
			_, err := e.Client().GL().Users.DeleteGPGKey(added.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		got := harness.Do[usergpgkeys.Output](s, actionUserGetGPGKey, map[string]any{"key_id": added.ID})
		if got.ID != added.ID {
			e.T.Errorf("get_gpg_key answered key %d, want %d", got.ID, added.ID)
		}

		listed := harness.Do[usergpgkeys.ListOutput](s, actionUserGPGKeys, nil)
		if !containsID(gpgKeyIDs(listed.Keys), added.ID) {
			e.T.Errorf("the run user's GPG keys do not hold the added key %d: %+v", added.ID, listed.Keys)
		}

		deleted := harness.Do[usergpgkeys.DeleteOutput](s, actionUserDeleteGPGKey, map[string]any{"key_id": added.ID})
		if !deleted.Deleted || deleted.KeyID != added.ID {
			e.T.Errorf("delete_gpg_key answered %+v, want deleted=true for key %d", deleted, added.ID)
		}
		after := harness.Do[usergpgkeys.ListOutput](s, actionUserGPGKeys, nil)
		if containsID(gpgKeyIDs(after.Keys), added.ID) {
			e.T.Errorf("GPG key %d is still listed after its delete", added.ID)
		}
	})
}

// TestUserGPGKeys_ForUser_AddGetListDelete gives one fixture user a key per
// surface, as an administrator: adds it, reads it by ID, finds it in the
// user's listing, deletes it and checks the listing no longer holds it.
//
// Replaces: TestIndividual_UserGPGKeysForUser, TestMeta_UserAdmin
func TestUserGPGKeys_ForUser_AddGetListDelete(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "gpguser")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		owner := map[string]any{"user_id": user.ID}

		added := harness.Do[usergpgkeys.Output](s, actionUserAddGPGKeyForUser, withParams(owner, map[string]any{"key": newGPGPublicKey(e, user.Username)}))
		if added.ID == 0 || added.Key == "" {
			e.T.Fatalf("add_gpg_key_for_user answered %+v, want a key with an ID", added)
		}
		key := withParams(owner, map[string]any{"key_id": added.ID})

		got := harness.Do[usergpgkeys.Output](s, actionUserGetGPGKeyForUser, key)
		if got.ID != added.ID {
			e.T.Errorf("get_gpg_key_for_user answered key %d, want %d", got.ID, added.ID)
		}

		listed := harness.Do[usergpgkeys.ListOutput](s, actionUserGPGKeysForUser, owner)
		if !containsID(gpgKeyIDs(listed.Keys), added.ID) {
			e.T.Errorf("the GPG keys of user %d do not hold the added key %d: %+v", user.ID, added.ID, listed.Keys)
		}

		deleted := harness.Do[usergpgkeys.DeleteOutput](s, actionUserDeleteGPGKeyForUser, key)
		if !deleted.Deleted || deleted.KeyID != added.ID {
			e.T.Errorf("delete_gpg_key_for_user answered %+v, want deleted=true for key %d", deleted, added.ID)
		}
		after := harness.Do[usergpgkeys.ListOutput](s, actionUserGPGKeysForUser, owner)
		if containsID(gpgKeyIDs(after.Keys), added.ID) {
			e.T.Errorf("GPG key %d of user %d is still listed after its delete", added.ID, user.ID)
		}
	})
}

// newGPGPublicKey generates an armored public key naming its owner, failing
// the test if the generator did.
func newGPGPublicKey(e *harness.Env, owner string) string {
	e.T.Helper()

	key, err := fixture.GPGPublicKey(owner)
	if err != nil {
		e.T.Fatal(err)
	}
	return key
}

// gpgKeyIDs collects the IDs of listed GPG keys.
func gpgKeyIDs(listed []usergpgkeys.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, key := range listed {
		ids = append(ids, key.ID)
	}
	return ids
}
