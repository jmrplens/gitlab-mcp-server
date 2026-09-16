//go:build e2e

// service_account.go builds the service accounts a licensed instance offers
// at the two scopes that have a create, project and group, each with one
// personal access token of its own.
//
// A service account is a user, so its username is unique instance-wide and
// has to be generated per fixture rather than taken from a constant: two
// attempts offering one name would have the second refused with "has already
// been taken", which reads as a defect in the case rather than as a
// collision between two runs. The same rule is why the instance-level create
// a case makes is given a reserved name here instead of a literal.
//
// The token is created beside the account because the cases that read a
// service account's tokens need one to find, and because an account with no
// token lists an empty page that a case cannot tell from a broken listing.

package fixture

import (
	"context"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// serviceAccountTokenScopes is what a fixture token is minted with. It is the
// narrowest scope that lets a listing show something: nothing signs in as
// this account.
var serviceAccountTokenScopes = []string{"read_api"}

// ServiceAccount is a service account a builder created, with the token it
// was given.
type ServiceAccount struct {
	// ID is the account's user ID, which every service account action takes.
	ID int64
	// Username is what it was created as.
	Username string
	// TokenID is the personal access token created for it.
	TokenID int64
}

// NewProjectServiceAccount creates a service account in the project, mints a
// token for it and registers both removals.
func NewProjectServiceAccount(e *harness.Env, project Project) ServiceAccount {
	e.T.Helper()

	username := ReserveServiceAccountUsername(e)
	account, err := retryTransient(e, "create project service account "+username, createRetries, func() (ServiceAccount, error) {
		return createProjectServiceAccount(e.Ctx, e.Client(), project.ID, username)
	})
	if err != nil {
		e.T.Fatalf("creating service account %q in project %d: %v", username, project.ID, err)
	}

	e.Defer("project service account "+username, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteProjectServiceAccount(ctx, e.Client(), project.ID, account.ID)
	})
	return account
}

// NewGroupServiceAccount does the same in a group. GitLab serves the endpoint
// on a top-level group only, which is what NewGroup builds.
func NewGroupServiceAccount(e *harness.Env, group Group) ServiceAccount {
	e.T.Helper()

	username := ReserveServiceAccountUsername(e)
	account, err := retryTransient(e, "create group service account "+username, createRetries, func() (ServiceAccount, error) {
		return createGroupServiceAccount(e.Ctx, e.Client(), group.ID, username)
	})
	if err != nil {
		e.T.Fatalf("creating service account %q in group %d: %v", username, group.ID, err)
	}

	e.Defer("group service account "+username, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteGroupServiceAccount(ctx, e.Client(), group.ID, account.ID)
	})
	return account
}

// ReserveServiceAccountUsername returns a username nobody on the instance
// signs in as, for the case that creates an instance service account itself.
//
// It registers no undo: nothing has been created, and what the case creates
// under this name is swept by the run's own prefix sweep like every other
// object a case leaves behind.
func ReserveServiceAccountUsername(e *harness.Env) string {
	e.T.Helper()
	return e.Name("svcacct")
}

// createProjectServiceAccount asks GitLab for the account and its token.
//
//nolint:dupl // the project and group service accounts are distinct SDK services with no shared interface, each taking its own option type and spelling the username under its own field; what the two would share is the two calls.
func createProjectServiceAccount(ctx context.Context, client *gitlabclient.Client, projectID int64, username string) (ServiceAccount, error) {
	created, _, err := client.GL().Projects.CreateProjectServiceAccount(projectID, &gl.CreateProjectServiceAccountOptions{
		Name:     new(username),
		Username: new(username),
	}, gl.WithContext(ctx))
	if err != nil {
		return ServiceAccount{}, err
	}

	expiry := gl.ISOTime(time.Now().Add(tokenLifetime))
	token, _, tokenErr := client.GL().Projects.CreateProjectServiceAccountPersonalAccessToken(
		projectID, created.ID, &gl.CreateProjectServiceAccountPersonalAccessTokenOptions{
			Name:      new(username),
			Scopes:    &serviceAccountTokenScopes,
			ExpiresAt: &expiry,
		}, gl.WithContext(ctx),
	)
	if tokenErr != nil {
		return ServiceAccount{}, fmt.Errorf("minting a token for project service account %d: %w", created.ID, tokenErr)
	}
	return ServiceAccount{ID: created.ID, Username: created.Username, TokenID: token.ID}, nil
}

// createGroupServiceAccount is the group half of the same pair.
//
//nolint:dupl // see createProjectServiceAccount.
func createGroupServiceAccount(ctx context.Context, client *gitlabclient.Client, groupID int64, username string) (ServiceAccount, error) {
	created, _, err := client.GL().Groups.CreateServiceAccount(groupID, &gl.CreateServiceAccountOptions{
		Name:     new(username),
		Username: new(username),
	}, gl.WithContext(ctx))
	if err != nil {
		return ServiceAccount{}, err
	}

	expiry := gl.ISOTime(time.Now().Add(tokenLifetime))
	token, _, tokenErr := client.GL().Groups.CreateServiceAccountPersonalAccessToken(
		groupID, created.ID, &gl.CreateServiceAccountPersonalAccessTokenOptions{
			Name:      new(username),
			Scopes:    &serviceAccountTokenScopes,
			ExpiresAt: &expiry,
		}, gl.WithContext(ctx),
	)
	if tokenErr != nil {
		return ServiceAccount{}, fmt.Errorf("minting a token for group service account %d: %w", created.ID, tokenErr)
	}
	return ServiceAccount{ID: created.ID, Username: created.UserName, TokenID: token.ID}, nil
}

// deleteProjectServiceAccount removes the account and tolerates one a case
// deleted. The token goes with the user, so it needs no revocation of its
// own.
func deleteProjectServiceAccount(ctx context.Context, client *gitlabclient.Client, projectID, accountID int64) error {
	_, err := client.GL().Projects.DeleteProjectServiceAccount(projectID, accountID,
		&gl.DeleteProjectServiceAccountOptions{HardDelete: new(true)}, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting service account %d of project %d: %w", accountID, projectID, err)
	}
	return nil
}

// deleteGroupServiceAccount is the group half of the same removal.
func deleteGroupServiceAccount(ctx context.Context, client *gitlabclient.Client, groupID, accountID int64) error {
	_, err := client.GL().Groups.DeleteServiceAccount(groupID, accountID,
		&gl.DeleteServiceAccountOptions{HardDelete: new(true)}, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting service account %d of group %d: %w", accountID, groupID, err)
	}
	return nil
}
