//go:build e2e

// useremails_test.go covers secondary email addresses: the run user's own,
// added, listed and removed; and another user's, which an administrator
// adds confirmed, lists, cannot read through the current-user endpoint
// because it is somebody else's, and removes.

package common

import (
	"context"
	"crypto/rand"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// emailDomain is the domain every address here is registered on, one that
// resolves nowhere, so the confirmation GitLab mails has nowhere to go.
const emailDomain = "@e2e-test.invalid"

// emailTokenLength is how much of a random token an address carries. Twelve
// base32 characters are sixty bits, which is far more than enough to keep two
// runs of one instance apart, and short enough to leave room under the limit
// below.
const emailTokenLength = 12

// uniqueAddress returns a fresh address on [emailDomain], named by the caller
// and made unique by a random token rather than by the harness's own naming.
//
// The harness names a resource after the test that creates it, which is what
// makes a leftover recognizable, and that name is far longer than the sixty
// four characters an address allows before its local part: an invitation is
// validated against that limit and is refused outright, which is what this
// exists for. Nothing here needs the run scoping anyway, since an address
// hangs off a user, a project or a group the fixture library removes.
func uniqueAddress(name string) string {
	return name + "-" + strings.ToLower(rand.Text()[:emailTokenLength]) + emailDomain
}

// TestUserEmails_OwnAccount_AddListDelete adds an address to the run user
// on every surface, finds it in the account's listing, deletes it and
// checks the listing no longer holds it.
//
// Replaces: TestIndividual_UserEmails, TestMeta_UserSelf
func TestUserEmails_OwnAccount_AddListDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		address := uniqueAddress("mail")

		added := harness.Do[useremails.Output](s, actionUserAddEmail, map[string]any{"email": address})
		if added.ID == 0 || added.Email != address {
			e.T.Fatalf("add_email answered %+v, want %q with an ID", added, address)
		}
		e.Defer("email "+address, func(ctx context.Context) error {
			_, err := e.Client().GL().Users.DeleteEmail(added.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		listed := harness.Do[users.EmailListOutput](s, actionUserEmails, nil)
		if !containsID(ownEmailIDs(listed.Emails), added.ID) {
			e.T.Errorf("the run user's emails do not hold the added address %d: %+v", added.ID, listed.Emails)
		}

		deleted := harness.Do[useremails.DeleteOutput](s, actionUserDeleteEmail, map[string]any{"email_id": added.ID})
		if !deleted.Deleted || deleted.EmailID != added.ID {
			e.T.Errorf("delete_email answered %+v, want deleted=true for email %d", deleted, added.ID)
		}
		after := harness.Do[users.EmailListOutput](s, actionUserEmails, nil)
		if containsID(ownEmailIDs(after.Emails), added.ID) {
			e.T.Errorf("email %d is still listed after its delete", added.ID)
		}
	})
}

// TestUserEmails_ForUser_AddListRefuseOwnReadDelete gives one fixture user
// an address per surface, as an administrator: adds it confirmed, finds it
// in the user's listing, is told not found when asking for it through the
// current user's own endpoint since the address is not the run user's,
// deletes it and checks the listing no longer holds it.
//
// Replaces: TestMeta_UserAdmin
func TestUserEmails_ForUser_AddListRefuseOwnReadDelete(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "mailuser")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		owner := map[string]any{"user_id": user.ID}
		address := uniqueAddress("mail")

		added := harness.Do[useremails.Output](s, actionUserAddEmailForUser, withParams(owner, map[string]any{"email": address, "skip_confirmation": true}))
		if added.ID == 0 || added.Email != address {
			e.T.Fatalf("add_email_for_user answered %+v, want %q with an ID", added, address)
		}

		listed := harness.Do[useremails.ListOutput](s, actionUserEmailsForUser, owner)
		if !containsID(emailIDs(listed.Emails), added.ID) {
			e.T.Errorf("the emails of user %d do not hold the added address %d: %+v", user.ID, added.ID, listed.Emails)
		}

		refusal := harness.Refused(s, actionUserGetEmail, map[string]any{"email_id": added.ID}, harness.FailureNotFound)
		assertMentions(e, "the refusal of another user's email", refusal, "user.emails")

		deleted := harness.Do[useremails.DeleteOutput](s, actionUserDeleteEmailForUser, withParams(owner, map[string]any{"email_id": added.ID}))
		if !deleted.Deleted || deleted.EmailID != added.ID {
			e.T.Errorf("delete_email_for_user answered %+v, want deleted=true for email %d", deleted, added.ID)
		}
		after := harness.Do[useremails.ListOutput](s, actionUserEmailsForUser, owner)
		if containsID(emailIDs(after.Emails), added.ID) {
			e.T.Errorf("email %d of user %d is still listed after its delete", added.ID, user.ID)
		}
	})
}

// ownEmailIDs collects the IDs of the run user's listed addresses.
func ownEmailIDs(listed []users.EmailOutput) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, email := range listed {
		ids = append(ids, email.ID)
	}
	return ids
}

// emailIDs collects the IDs of another user's listed addresses.
func emailIDs(listed []useremails.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, email := range listed {
		ids = append(ids, email.ID)
	}
	return ids
}
