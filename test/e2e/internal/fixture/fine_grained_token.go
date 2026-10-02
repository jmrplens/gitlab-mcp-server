//go:build e2e

// fine_grained_token.go mints a fine-grained personal access token for a
// fixture user, which is the one credential the server reads as a grant of
// named permissions rather than as a list of scopes (issue 952).
//
// GitLab creates one only for the token's own user: the administrator route
// that mints a token for somebody else takes no grant
// (lib/api/users.rb:1291-1322 at 19.4.1), and the route that does,
// POST /user/personal_access_tokens, answers as whoever holds the credential
// it was sent with (lib/api/users.rb:1841-1883). So the builder mints the user
// a classic token through the administrator first, the way NewToken does, and
// sends the creation with it. A classic creating token passes GitLab's
// privilege escalation check whatever the grant asks for
// (app/services/authz/tokens/privilege_escalation_check.rb), so the grant is
// exactly the scopes a test wrote.
//
// Every scope travels in the one creation request. GitLab validates every
// scope of a request before it builds any, so a user scope and an instance
// scope, which carry no namespace, sit together on one token, and nothing
// edits a grant once the token exists: a scenario that needs another grant
// mints another token.

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

// The access levels a fine-grained scope is created at, spelled as GitLab's
// creation route takes them (lib/api/helpers/personal_access_tokens_helpers.rb).
const (
	// AccessPersonalProjects covers the projects in the token user's own
	// namespace, which GitLab attaches to the scope itself.
	AccessPersonalProjects = "personal_projects"
	// AccessAllMemberships covers every project and group the user is a
	// member of.
	AccessAllMemberships = "all_memberships"
	// AccessSelectedMemberships covers the projects and groups the scope
	// names, and everything below a named group.
	AccessSelectedMemberships = "selected_memberships"
	// AccessUser covers what GitLab holds at the user boundary: the user's
	// own profile, tokens and namespaces.
	AccessUser = "user"
	// AccessInstance covers what GitLab holds at the instance boundary.
	AccessInstance = "instance"
)

// GranularScope is one scope of a fine-grained grant, in the shape the
// creation route takes: an access level, the assignable permissions it grants
// by GitLab's name for each (create_work_item, read_project), and, for
// AccessSelectedMemberships, the projects or groups it names.
type GranularScope struct {
	Access      string   `json:"access"`
	Permissions []string `json:"permissions"`
	ProjectIDs  []int64  `json:"project_ids,omitempty"`
	GroupIDs    []int64  `json:"group_ids,omitempty"`
}

// StartupScopes are the scopes a fine-grained token needs for the server to
// start on it and learn what it is: User: Read and Namespace: Read and
// Personal Access Token: Read at the user boundary (the identity, the
// namespace plans the tier is read from, and the token's own grant), and
// Metadata: Read at the instance (the version the grant is judged at).
//
// A scenario adds the scopes it is about to these, so a session on the token
// starts in phase B; one that leaves them out is asking for phase A.
func StartupScopes() []GranularScope {
	return []GranularScope{
		{Access: AccessUser, Permissions: []string{"read_user", "read_namespace", "read_personal_access_token"}},
		{Access: AccessInstance, Permissions: []string{"read_metadata"}},
	}
}

// fineGrainedTokenRequest is the body POST /user/personal_access_tokens takes
// with a grant. The route takes a grant or classic scopes, never both, so the
// body carries no scopes at all.
type fineGrainedTokenRequest struct {
	Name           string          `json:"name"`
	ExpiresAt      string          `json:"expires_at"`
	GranularScopes []GranularScope `json:"granular_scopes"`
}

// fineGrainedTokenAnswer is the part of the creation answer the builder
// reads: the token's id and value, and GitLab's own word that it is
// fine-grained.
type fineGrainedTokenAnswer struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Token    string `json:"token"`
	Granular bool   `json:"granular"`
}

// NewFineGrainedToken mints a fine-grained personal access token for user
// carrying exactly the given scopes, and registers its revocation on the Env.
//
// The classic token it is created with is minted through NewToken and is
// revoked with the rest of the test's state. A run whose token is not an
// administrator's cannot mint either, and the test skips with that reason.
func NewFineGrainedToken(e *harness.Env, user User, scopes ...GranularScope) Token {
	e.T.Helper()
	creator := NewToken(e, user)
	client, err := e.ClientFor(creator.Value)
	if err != nil {
		e.T.Fatalf("building a client for user %d's creating token: %v", user.ID, err)
	}

	// GitLab refreshes a member's project authorizations in a background job,
	// and a creation naming a project or group the user cannot see yet is
	// answered 404, which a backlog under a whole suite makes outlast every
	// retry. So the creation waits, with the user's own credential, until it
	// sees every namespace the scopes name.
	awaitAccess(e, user, client, scopes)

	name := e.Name("fgtok")
	expiry := time.Now().Add(tokenLifetime + 24*time.Hour).UTC().Format(time.DateOnly)
	token, err := retryTransient(e, "create fine-grained token "+name, createRetries, func() (Token, error) {
		return createFineGrainedToken(e.Ctx, client, fineGrainedTokenRequest{Name: name, ExpiresAt: expiry, GranularScopes: scopes})
	})
	if err != nil {
		e.T.Fatalf("minting a fine-grained token for user %d: %v", user.ID, err)
	}
	token.UserID = user.ID

	e.Defer("fine-grained token "+token.Name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		_, revokeErr := e.Client().GL().PersonalAccessTokens.RevokePersonalAccessTokenByID(token.ID, gl.WithContext(ctx))
		if revokeErr != nil && !IsStatus(revokeErr, http.StatusNotFound) {
			return fmt.Errorf("revoking fine-grained token %d: %w", token.ID, revokeErr)
		}
		return nil
	})
	return token
}

// AwaitProjectAccess holds until user can read every project named, asked
// with a classic token of the user's own, the wait NewFineGrainedToken makes
// for the projects a grant names.
//
// A scenario that sends GitLab a fine-grained token's request about a project
// its grant does not cover needs it for that project: until the background job
// that refreshes the user's project authorizations has run, GitLab answers the
// request 404, for a boundary the user cannot see, and only afterwards the 403
// that names the permission the grant lacks, which is the answer the scenario
// is about.
func AwaitProjectAccess(e *harness.Env, user User, projects ...Project) {
	e.T.Helper()
	reader := NewToken(e, user)
	client, err := e.ClientFor(reader.Value)
	if err != nil {
		e.T.Fatalf("building a client for user %d's reading token: %v", user.ID, err)
	}
	ids := make([]int64, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	awaitAccess(e, user, client, []GranularScope{{ProjectIDs: ids}})
}

// awaitAccess holds until client, which acts as user, reads every project and
// group the scopes name, or ends the test.
func awaitAccess(e *harness.Env, user User, client *gitlabclient.Client, scopes []GranularScope) {
	e.T.Helper()
	DrainSidekiq(e.Ctx, e.Client())
	if err := waitForGrantedAccess(e.Ctx, client, scopes, budget(e, accessWait, enterpriseAccessWait)); err != nil {
		e.T.Fatalf("waiting for user %d to see the projects and groups it is a member of: %v", user.ID, err)
	}
}

// The wait for a user's membership to reach its own credential, larger on a
// licensed instance like every wait here, and how often it is asked.
const (
	accessWait           = 2 * time.Minute
	enterpriseAccessWait = 4 * time.Minute
	accessPollInterval   = time.Second
)

// waitForGrantedAccess holds until client's user can read every project and
// group the scopes name, the way GitLab's creation route checks it.
func waitForGrantedAccess(ctx context.Context, client *gitlabclient.Client, scopes []GranularScope, timeout time.Duration) error {
	for _, scope := range scopes {
		for _, id := range scope.ProjectIDs {
			if err := harness.Poll(ctx, accessPollInterval, timeout, func() (bool, string, error) {
				if _, _, err := client.GL().Projects.GetProject(id, nil, gl.WithContext(ctx)); err != nil {
					return harness.WaitThrough(fmt.Sprintf("project %d", id), err)
				}
				return true, "", nil
			}); err != nil {
				return err
			}
		}
		for _, id := range scope.GroupIDs {
			if err := harness.Poll(ctx, accessPollInterval, timeout, func() (bool, string, error) {
				if _, _, err := client.GL().Groups.GetGroup(id, nil, gl.WithContext(ctx)); err != nil {
					return harness.WaitThrough(fmt.Sprintf("group %d", id), err)
				}
				return true, "", nil
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// createFineGrainedToken sends one creation request with the creating user's
// own client and reads back the token it made.
//
// client-go has no method for the route with a grant (upstream-bugs row 82),
// so the request is built through its NewRequest, which still carries the
// retries, the context and the base URL every other call of the client does.
// An answer GitLab did not mark granular is refused: a token created without
// its grant would make every scenario on it test a classic token instead,
// and pass.
func createFineGrainedToken(ctx context.Context, client *gitlabclient.Client, body fineGrainedTokenRequest) (Token, error) {
	req, err := client.GL().NewRequest(http.MethodPost, "user/personal_access_tokens", body, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return Token{}, fmt.Errorf("building the creation request: %w", err)
	}
	var created fineGrainedTokenAnswer
	if _, err = client.GL().Do(req, &created); err != nil {
		return Token{}, err
	}
	if created.Token == "" {
		return Token{}, fmt.Errorf("fine-grained token %q was created without a value", body.Name)
	}
	if !created.Granular {
		return Token{}, fmt.Errorf("token %q was created, and GitLab does not report it as fine-grained", body.Name)
	}
	return Token{ID: created.ID, Value: created.Token, Name: created.Name}, nil
}
