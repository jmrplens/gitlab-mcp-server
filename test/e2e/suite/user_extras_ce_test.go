//go:build e2e && !enterprise

// user_extras_ce_test.go covers previously unexercised user-domain actions:
// current-user secondary emails (user.add_email / user.delete_email),
// current-user GPG keys (user.add_gpg_key / user.get_gpg_key /
// user.delete_gpg_key), SSH key lookup by fingerprint
// (user.key_get_by_fingerprint), avatar upload (user.upload_avatar), and
// marking a single todo done (user.todo_mark_done).
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
	"golang.org/x/crypto/ssh"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/users"
)

// userExtrasGPGKeyOne is a fixed, pre-generated ASCII-armored OpenPGP public
// key used as the current-user GPG fixture. GPG fingerprints are unique per
// GitLab instance, so tests embedding this key run in Docker mode only where
// the instance is disposable.
const userExtrasGPGKeyOne = `-----BEGIN PGP PUBLIC KEY BLOCK-----

xsBNBGpLgFoBCACmeARDhHTJr3d+SJfHaXI+EZ/osozzFdEn322pTqP9ela66Xny
v/z5BqH1AGKD1oxneBz2UiiBcd/Gh+dLvurfh9eM/9E8EdA8S/syoHy/8z/O1tZL
NiElEY7WVq7JVHhT9soAtvMmVR84bA4Xx9RCoPVFZOcxx/1p+K/Jk0ArfHe2v0tb
yFSg3L2OIhiBcGtY4Vl2HnaLaNKPTKivOfJ3Amzs0/szwgUArXsO3M4n4AvJFN5u
ss5wfFp7n48oIYR0ykriGD3hw3ClZ+u1ccjT2liazTn9XX/lLxvt6EOjr5JQyyci
4dClx0CCVN5yqQaf+SJvcg+gQBNRgvOzk5RJABEBAAHNR0dpdExhYiBNQ1AgRTJF
IEZpeHR1cmUgT25lIChlMmUgdGVzdCBmaXh0dXJlKSA8ZTJlLWdwZy1vbmVAZXhh
bXBsZS5jb20+wsBiBBMBCAAWBQJqS4BaCRCKNx5mjYPgZAIbAwIZAQAAw40IAD06
TTUNESH5GJaBODrnVCS+uBDx0EZEgtnTFYcVNdiyYn6JHVbiNHJCLCl00WCZtKsJ
MaADY4MVSELyAGW0+U7My4BezcOneZhm208lBoo0fvzTABljCsrBij+yRBFiPIb5
OfTkEmvyxa2BLdj9+VcWM1Ep1C0ec2C6Bm3f8BWvVW2fiGmfnEbQzd9zmsutM9GH
+T4zYaCmYFO+Rqi+klaHA777iMISuNp3sIL5UE/ojz7q6WRaepOHFTGsWGIwg4Ig
YYC93wK4LpDBpq0vnHCVyM1OJ6RS6fGHQphZze9yCXDvt2D1mA3ZJwVrZTtvbsb8
GPIPuKM5SDEMgTcfG2nOwE0EakuAWgEIALyb+t5ops/YWlB4EGmyMYgV0dpGpxN/
tdqKwLKLxCXb8xUjo+6RPNWxzwIK8c0rWO0RhSI3T3/7Fcam67SavLCQsxTGLz5S
Xi+re7msZ/v2UV9+XrVaZCXLavDtPShHn4yvBbu4IQuiXgHTDUI/QuYXH+/2ASCS
FiJRJOrLa2QF9s/VUwSfawc2bF+A5rLdO+hXLIHbkbwcET0FKXZrBINzf6WRXKgN
Y/vUBnWR0WwoGv9vTKo80HefR+oLo409W2HE/npscfWMAUestCVyCWdl+d4CEqKk
bI6NjkgqWG59RLHHbKpyPNY+g0d1zIEdekIz21HMybt8NUNPhxc9BPEAEQEAAcLA
XwQYAQgAEwUCakuAWgkQijceZo2D4GQCGwwAACMFCAA/GvCVhztR1oQKtMAxMrSt
dLfa3Kcf+B+60lNRuzF2S/GcHhKgtzdnrYZfCZNh8iSxmf+Qp6eVYYitPdXgC94K
zMWwA8YEqNQ4d9z2XC5FbeBvHzS+skv9n9GGrqvG91rZes/N6VT70c74kRqjpoAo
QjDOjEPDjB22vC+853PMR4dEk+nJuukh8gVBq8Z+Y+Jj3gBl179PIEa3L2kYEqy5
AMNRhGnhH1lZLBmtLWOZjl4SHX4AtYy7GrIvUn+ZndDducrm6dzHjW3sbeY3Xo+/
G9+hr9+0WmlgxIcgyqO0j7FV3nIhqNt5joHnAUk07JYoSBIY1iZXJqxMKOX0dw0K
=FEgx
-----END PGP PUBLIC KEY BLOCK-----`

// userExtrasAvatarPNGBase64 is a 1x1 PNG image, small enough to stay far
// below GitLab's 200 KB avatar limit while remaining a valid image payload.
const userExtrasAvatarPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// userExtrasSSHKey generates a one-off ED25519 keypair and returns the
// authorized-key line plus its SHA256 fingerprint. A dedicated helper (rather
// than generateTestSSHKey) is needed because the fingerprint lookup test must
// know the fingerprint of the key it registered.
func userExtrasSSHKey(t *testing.T) (string, string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	requireNoError(t, err, "generate ed25519 key")
	sshPub, err := ssh.NewPublicKey(pub)
	requireNoError(t, err, "convert to ssh public key")
	return string(ssh.MarshalAuthorizedKey(sshPub)), ssh.FingerprintSHA256(sshPub)
}

// TestIndividual_UserEmails exercises user.add_email and user.delete_email
// on the current authenticated user through individual MCP tools.
//
// The test adds a unique secondary email to the token's own account, asserts
// the returned email ID and address round-trip, and deletes it again. A
// deferred best-effort delete guards against leftovers if the delete subtest
// never runs.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_UserEmails(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityCurrentUserState}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		email := uniqueName("umail") + "@e2e-test.local"
		var emailID int64

		t.Run("Add", func(t *testing.T) {
			out, err := callToolOn[useremails.Output](ctx, sess.individual, "gitlab_add_email", useremails.AddInput{
				Email: email,
			})
			requireNoError(t, err, "add email")
			requireTruef(t, out.ID > 0, "expected email ID > 0")
			requireTruef(t, out.Email == email, "expected email %q, got %q", email, out.Email)
			emailID = out.ID
			t.Logf("Added secondary email %d: %s", emailID, email)
		})
		defer func() {
			if emailID > 0 {
				// Best-effort: the Delete subtest normally removes it already.
				_ = callToolVoidOn(ctx, sess.individual, "gitlab_delete_email", useremails.DeleteInput{EmailID: emailID})
			}
		}()

		t.Run("Delete", func(t *testing.T) {
			requireTruef(t, emailID > 0, "emailID not set")
			err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_email", useremails.DeleteInput{EmailID: emailID})
			requireNoError(t, err, "delete email")
			requireNotListedOn(ctx, t, sess.individual, "account emails after delete", "gitlab_list_emails",
				users.ListEmailsInput{},
				func(out users.EmailListOutput) []int64 {
					ids := make([]int64, 0, len(out.Emails))
					for _, e := range out.Emails {
						ids = append(ids, e.ID)
					}
					return ids
				}, emailID)
			t.Logf("Deleted secondary email %d", emailID)
			emailID = 0
		})
	})
}

// TestIndividual_UserGPGKeys exercises user.add_gpg_key, user.get_gpg_key,
// and user.delete_gpg_key on the current authenticated user.
//
// The test registers the embedded fixed GPG public key fixture, fetches it by
// ID, and deletes it. GPG key fingerprints are unique per instance, so this
// runs in Docker mode only where the whole instance is disposable and a
// crashed previous run cannot leave a colliding fingerprint behind.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual.
func TestIndividual_UserGPGKeys(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("fixed GPG key fixture requires a disposable Docker instance (fingerprints are instance-unique)")
	}
	RunWithCapabilities(t, []Capability{CapabilityCurrentUserState}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var keyID int64

		t.Run("Add", func(t *testing.T) {
			out, err := callToolOn[usergpgkeys.Output](ctx, sess.individual, "gitlab_add_gpg_key", usergpgkeys.AddInput{
				Key: userExtrasGPGKeyOne,
			})
			requireNoError(t, err, "add gpg key")
			requireTruef(t, out.ID > 0, "expected GPG key ID > 0")
			keyID = out.ID
			t.Logf("Added GPG key %d", keyID)
		})
		defer func() {
			if keyID > 0 {
				_ = callToolVoidOn(ctx, sess.individual, "gitlab_delete_gpg_key", usergpgkeys.DeleteInput{KeyID: keyID})
			}
		}()

		t.Run("Get", func(t *testing.T) {
			requireTruef(t, keyID > 0, "keyID not set")
			out, err := callToolOn[usergpgkeys.Output](ctx, sess.individual, "gitlab_get_gpg_key", usergpgkeys.GetInput{
				KeyID: keyID,
			})
			requireNoError(t, err, "get gpg key")
			requireTruef(t, out.ID == keyID, "expected key ID %d, got %d", keyID, out.ID)
			t.Logf("Got GPG key %d", out.ID)
		})

		t.Run("Delete", func(t *testing.T) {
			requireTruef(t, keyID > 0, "keyID not set")
			err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_gpg_key", usergpgkeys.DeleteInput{KeyID: keyID})
			requireNoError(t, err, "delete gpg key")
			requireNotListedOn(ctx, t, sess.individual, "account GPG keys after delete", "gitlab_list_gpg_keys",
				usergpgkeys.ListInput{}, gpgKeyIDs, keyID)
			t.Logf("Deleted GPG key %d", keyID)
			keyID = 0
		})
	})
}

// TestIndividual_UserKeyByFingerprint exercises user.key_get_by_fingerprint
// through the gitlab_get_key_by_fingerprint individual tool.
//
// The test registers a fresh SSH key for the current user (covered action),
// computes its SHA256 fingerprint locally, and resolves the key back through
// the admin-only fingerprint lookup endpoint, asserting the IDs match. The
// key is removed via a deferred delete.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual. Admin token required.
func TestIndividual_UserKeyByFingerprint(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		sshKey, fingerprint := userExtrasSSHKey(t)
		addOut, err := callToolOn[users.SSHKeyOutput](ctx, sess.individual, "gitlab_add_ssh_key", users.AddSSHKeyInput{
			Title: uniqueName("fp-key"),
			Key:   sshKey,
		})
		requireNoError(t, err, "add ssh key for fingerprint lookup")
		requireTruef(t, addOut.ID > 0, "expected SSH key ID > 0")
		defer func() {
			_ = callToolVoidOn(ctx, sess.individual, "gitlab_delete_ssh_key", users.DeleteSSHKeyInput{KeyID: addOut.ID})
		}()

		out, err := callToolOn[keys.Output](ctx, sess.individual, "gitlab_get_key_by_fingerprint", keys.GetByFingerprintInput{
			Fingerprint: fingerprint,
		})
		requireNoError(t, err, "get key by fingerprint")
		requireTruef(t, out.ID == addOut.ID, "expected key ID %d, got %d", addOut.ID, out.ID)
		t.Logf("Resolved key %d by fingerprint %s", out.ID, fingerprint)
	})
}

// TestIndividual_UserAvatarUpload exercises user.upload_avatar through the
// gitlab_upload_user_avatar individual tool.
//
// The test uploads an in-test 1x1 PNG as the current user's avatar via the
// content_base64 path and asserts the returned profile carries a non-empty
// avatar URL. GitLab keeps exactly one avatar per user, so no cleanup is
// needed beyond the upload replacing whatever was there.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_UserAvatarUpload(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityCurrentUserState}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		out, err := callToolOn[users.Output](ctx, sess.individual, "gitlab_upload_user_avatar", users.UploadCurrentUserAvatarInput{
			Filename:      "e2e-avatar.png",
			ContentBase64: userExtrasAvatarPNGBase64,
		})
		requireNoError(t, err, "upload user avatar")
		// GitLab 19 responds with only {avatar_url}, so the user ID is
		// legitimately absent from the upload response.
		requireTruef(t, out.AvatarURL != "", "expected non-empty avatar URL after upload")
		t.Logf("Uploaded avatar (user %d): %s", out.ID, out.AvatarURL)
	})
}

// TestIndividual_UserTodoMarkDone exercises user.todo_mark_done through the
// gitlab_todo_mark_done individual tool.
//
// The test creates a project and an issue, generates a deterministic pending
// todo for the current user via the raw issue-todo API (self-assignment does
// not create todos, so the explicit todo endpoint is the only reliable
// setup), then marks that todo done via MCP and asserts the confirmation
// echoes the todo ID.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_UserTodoMarkDone(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	issue := createIssue(ctx, t, sess.individual, proj, "E2E todo target")

	todo, _, err := sess.glClient.GL().Issues.CreateTodo(proj.ID, issue.IID, gl.WithContext(ctx))
	requireNoError(t, err, "create todo fixture via raw API")
	requireTruef(t, todo.ID > 0, "expected todo ID > 0")

	out, err := callToolOn[todos.MarkDoneOutput](ctx, sess.individual, "gitlab_todo_mark_done", todos.MarkDoneInput{
		ID: todo.ID,
	})
	requireNoError(t, err, "mark todo done")
	requireTruef(t, out.ID == todo.ID, "expected todo ID %d, got %d", todo.ID, out.ID)
	t.Logf("Marked todo %d done: %s", out.ID, out.Message)
}
