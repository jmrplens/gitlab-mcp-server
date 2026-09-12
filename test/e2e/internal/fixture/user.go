//go:build e2e

// user.go builds the objects that need an administrator: a disposable user,
// a personal access token for one, and an SSH key. A test that drives an
// action against "another user" cannot use the run's own account for it, and
// a test of token operations must never run them against the credential the
// whole run depends on.

package fixture

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
	"golang.org/x/crypto/ssh"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// User is a disposable user a builder created through the admin API.
type User struct {
	// ID is the numeric identifier every user-scoped action takes.
	ID int64
	// Username is the login name, scoped to the run.
	Username string
	// Email is the address it was registered with, on a domain that delivers
	// nowhere.
	Email string
	// Password is the random password it was created with, for a test that
	// signs in as it. Nothing else ever uses it.
	Password string
}

// Token is a personal access token a builder minted for a user.
type Token struct {
	// ID is what the revocation and rotation actions take.
	ID int64
	// Value is the secret itself, shown once at creation.
	Value string
	// UserID is the user it belongs to.
	UserID int64
	// Name is what it was created as.
	Name string
}

// SSHKey is an SSH key a builder added to a user.
type SSHKey struct {
	// ID is what the key actions take.
	ID int64
	// Title is what it was added as.
	Title string
	// PublicKey is the authorized_keys form of it.
	PublicKey string
}

// UserOption adjusts one user builder.
type UserOption func(*gl.CreateUserOptions)

// AsAdmin creates the user as an instance administrator.
func AsAdmin() UserOption {
	return func(opts *gl.CreateUserOptions) { opts.Admin = new(true) }
}

// AsExternal creates the user as an external user, which cannot see internal
// projects.
func AsExternal() UserOption {
	return func(opts *gl.CreateUserOptions) { opts.External = new(true) }
}

// userEmailDomain is a domain that resolves nowhere, so a confirmation mail
// GitLab might still try to send has nowhere to go.
const userEmailDomain = "@e2e-test.invalid"

// NewUser creates a confirmed user with a random password through the admin
// API and registers its hard deletion on the Env. A run whose token is not an
// administrator's cannot create one, and the test is skipped with that
// reason; declare harness.Needs(harness.NeedAdmin) to say so up front.
func NewUser(e *harness.Env, prefix string, opts ...UserOption) User {
	e.T.Helper()
	requireAdmin(e, "creating a user")
	armSweep(e)

	username := e.Name(prefix)
	password := "Pw-" + rand.Text()
	createOpts := &gl.CreateUserOptions{
		Email:            new(username + userEmailDomain),
		Name:             new("E2E " + username),
		Username:         new(username),
		Password:         new(password),
		SkipConfirmation: new(true),
	}
	for _, opt := range opts {
		opt(createOpts)
	}

	user, err := retryTransient(e, "create user "+username, createRetries, func() (User, error) {
		created, _, err := e.Client().GL().Users.CreateUser(createOpts, gl.WithContext(e.Ctx))
		if err != nil {
			return User{}, err
		}
		return User{ID: created.ID, Username: created.Username, Email: created.Email, Password: password}, nil
	})
	if err != nil {
		e.T.Fatalf("creating user %q: %v", username, err)
	}

	e.Defer("user "+user.Username, func(ctx context.Context) error {
		return DeleteUser(ctx, e.Client(), user.ID)
	})
	return user
}

// DeleteUser hard-deletes a user. One that is already gone, which a test that
// rejected or deleted it leaves behind, is not an error.
func DeleteUser(ctx context.Context, client *gitlabclient.Client, userID int64) error {
	ctx, cancel := withCleanupTimeout(ctx)
	defer cancel()

	_, err := client.GL().Users.DeleteUser(userID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting user %d: %w", userID, err)
	}
	return nil
}

// tokenLifetime is how long a fixture token stays valid. A day is longer
// than any run and shorter than any policy an instance enforces.
const tokenLifetime = 24 * time.Hour

// NewToken mints a personal access token for user with the given scopes
// through the admin API and registers its revocation on the Env. With no
// scopes, "api" is assumed.
func NewToken(e *harness.Env, user User, scopes ...string) Token {
	e.T.Helper()
	requireAdmin(e, "minting a token for another user")

	if len(scopes) == 0 {
		scopes = []string{"api"}
	}
	name := e.Name("tok")
	expiry := gl.ISOTime(time.Now().Add(tokenLifetime))

	token, err := retryTransient(e, "create token "+name, createRetries, func() (Token, error) {
		created, _, err := e.Client().GL().Users.CreatePersonalAccessToken(user.ID, &gl.CreatePersonalAccessTokenOptions{
			Name:      new(name),
			Scopes:    &scopes,
			ExpiresAt: &expiry,
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Token{}, err
		}
		if created.Token == "" {
			return Token{}, fmt.Errorf("token %q was created without a value", name)
		}
		return Token{ID: created.ID, Value: created.Token, UserID: user.ID, Name: created.Name}, nil
	})
	if err != nil {
		e.T.Fatalf("minting a token for user %d: %v", user.ID, err)
	}

	e.Defer("token "+token.Name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, revokeErr := e.Client().GL().PersonalAccessTokens.RevokePersonalAccessTokenByID(token.ID, gl.WithContext(ctx))
		if revokeErr != nil && !IsStatus(revokeErr, http.StatusNotFound) {
			return fmt.Errorf("revoking token %d: %w", token.ID, revokeErr)
		}
		return nil
	})
	return token
}

// SSHPublicKey generates a fresh ED25519 key pair and returns the public half
// in authorized_keys form, for a deploy key or a user key. The private half
// is discarded: nothing in a test ever connects with it.
func SSHPublicKey() (string, error) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generating an ed25519 key: %w", err)
	}
	sshPublic, err := ssh.NewPublicKey(public)
	if err != nil {
		return "", fmt.Errorf("converting the key to SSH form: %w", err)
	}
	return string(ssh.MarshalAuthorizedKey(sshPublic)), nil
}

// NewSSHKey generates a key and adds it to user through the admin API,
// registering its removal on the Env.
func NewSSHKey(e *harness.Env, user User) SSHKey {
	e.T.Helper()
	requireAdmin(e, "adding a key to another user")

	publicKey, keyErr := SSHPublicKey()
	if keyErr != nil {
		e.T.Fatal(keyErr)
	}
	title := e.Name("key")

	key, addErr := retryTransient(e, "add ssh key "+title, createRetries, func() (SSHKey, error) {
		added, _, err := e.Client().GL().Users.AddSSHKeyForUser(user.ID, &gl.AddSSHKeyOptions{
			Title: new(title),
			Key:   new(publicKey),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return SSHKey{}, err
		}
		return SSHKey{ID: added.ID, Title: added.Title, PublicKey: publicKey}, nil
	})
	if addErr != nil {
		e.T.Fatalf("adding an SSH key to user %d: %v", user.ID, addErr)
	}

	e.Defer("ssh key "+key.Title, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, deleteErr := e.Client().GL().Users.DeleteSSHKeyForUser(user.ID, key.ID, gl.WithContext(ctx))
		if deleteErr != nil && !IsStatus(deleteErr, http.StatusNotFound) {
			return fmt.Errorf("deleting ssh key %d of user %d: %w", key.ID, user.ID, deleteErr)
		}
		return nil
	})
	return key
}

// requireAdmin skips a test whose fixture needs an administrator the run does
// not have, naming what it was about to do.
func requireAdmin(e *harness.Env, what string) {
	e.T.Helper()
	if !e.Runtime().Admin {
		e.Skipf("%s needs an administrator token, and this run's is not one; declare harness.Needs(harness.NeedAdmin)", what)
	}
}
