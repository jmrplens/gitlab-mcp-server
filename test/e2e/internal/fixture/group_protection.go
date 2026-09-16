//go:build e2e

// group_protection.go builds the two group-scoped protections a licensed
// instance offers a case to read and to take off: a protected branch rule and
// an LDAP directory link.
//
// Both are rules about a name rather than about an object: a group protects
// a branch pattern no repository need carry, and an LDAP link records a
// provider GitLab will consult later. That is what makes them buildable on an
// instance with neither a repository nor a directory, and it is why each
// builder takes a group and nothing else.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The access a group protection and a directory link grant: Maintainer for
// the branch, Developer for the link.
const (
	groupProtectedBranchAccess = gl.MaintainerPermissions
	ldapLinkAccess             = gl.DeveloperPermissions
)

// LDAPProvider is the directory provider the Docker stack configures. A link
// records the provider whether or not a directory answers, which is what lets
// this fixture exist on an instance with no LDAP server.
const LDAPProvider = "ldapmain"

// GroupProtectedBranch is a branch rule a builder put on a group.
type GroupProtectedBranch struct {
	// ID is the rule's identifier.
	ID int64
	// Name is the branch pattern, which is what the group actions take.
	Name string
}

// LDAPLink is a directory link a builder put on a group.
type LDAPLink struct {
	// CN is the common name the link was created under.
	CN string
	// Provider is the directory it names.
	Provider string
}

// NewGroupProtectedBranch protects a branch pattern at group scope and
// registers its unprotection.
func NewGroupProtectedBranch(e *harness.Env, group Group) GroupProtectedBranch {
	e.T.Helper()

	name := e.Name("protected") + "/*"
	rule, err := retryTransient(e, "protect group branch "+name, createRetries, func() (GroupProtectedBranch, error) {
		return protectGroupBranch(e.Ctx, e.Client(), group.ID, name)
	})
	if err != nil {
		e.T.Fatalf("protecting branch %q of group %d: %v", name, group.ID, err)
	}

	e.Defer("group protected branch "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return unprotectGroupBranch(ctx, e.Client(), group.ID, rule.Name)
	})
	return rule
}

// NewGroupLDAPLink adds a directory link to the group and registers its
// removal.
func NewGroupLDAPLink(e *harness.Env, group Group) LDAPLink {
	e.T.Helper()

	cn := e.Name("cn")
	link, err := retryTransient(e, "add ldap link "+cn, createRetries, func() (LDAPLink, error) {
		return addGroupLDAPLink(e.Ctx, e.Client(), group.ID, cn)
	})
	if err != nil {
		e.T.Fatalf("adding LDAP link %q to group %d: %v", cn, group.ID, err)
	}

	e.Defer("ldap link "+cn, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteGroupLDAPLink(ctx, e.Client(), group.ID, link.CN, link.Provider)
	})
	return link
}

// protectGroupBranch asks GitLab for the rule.
func protectGroupBranch(ctx context.Context, client *gitlabclient.Client, groupID int64, name string) (GroupProtectedBranch, error) {
	created, _, err := client.GL().GroupProtectedBranches.ProtectRepositoryBranches(groupID,
		&gl.ProtectGroupRepositoryBranchesOptions{
			Name:             new(name),
			PushAccessLevel:  new(groupProtectedBranchAccess),
			MergeAccessLevel: new(groupProtectedBranchAccess),
		}, gl.WithContext(ctx))
	if err != nil {
		return GroupProtectedBranch{}, err
	}
	return GroupProtectedBranch{ID: created.ID, Name: created.Name}, nil
}

// unprotectGroupBranch lifts the rule and tolerates one a case lifted.
func unprotectGroupBranch(ctx context.Context, client *gitlabclient.Client, groupID int64, name string) error {
	_, err := client.GL().GroupProtectedBranches.UnprotectRepositoryBranches(groupID, name, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("unprotecting branch %q of group %d: %w", name, groupID, err)
	}
	return nil
}

// GroupBranchIsProtected reports whether the group still protects the
// pattern, which is what a case that unprotects one is verified against.
func GroupBranchIsProtected(ctx context.Context, client *gitlabclient.Client, groupID int64, name string) (bool, error) {
	_, _, err := client.GL().GroupProtectedBranches.GetProtectedBranch(groupID, name, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the protection of branch %q in group %d: %w", name, groupID, err)
	}
	return true, nil
}

// addGroupLDAPLink asks GitLab for the link.
func addGroupLDAPLink(ctx context.Context, client *gitlabclient.Client, groupID int64, cn string) (LDAPLink, error) {
	created, _, err := client.GL().Groups.AddGroupLDAPLink(groupID, &gl.AddGroupLDAPLinkOptions{
		CN:          new(cn),
		GroupAccess: new(ldapLinkAccess),
		Provider:    new(LDAPProvider),
	}, gl.WithContext(ctx))
	if err != nil {
		return LDAPLink{}, err
	}
	return LDAPLink{CN: created.CN, Provider: created.Provider}, nil
}

// deleteGroupLDAPLink removes the link and tolerates one a case deleted.
func deleteGroupLDAPLink(ctx context.Context, client *gitlabclient.Client, groupID int64, cn, provider string) error {
	_, err := client.GL().Groups.DeleteGroupLDAPLinkWithCNOrFilter(groupID,
		&gl.DeleteGroupLDAPLinkWithCNOrFilterOptions{CN: new(cn), Provider: new(provider)}, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting LDAP link %q of group %d: %w", cn, groupID, err)
	}
	return nil
}

// GroupHasLDAPLink reports whether the group still holds a link for the
// provider, which is what a case that deletes every link of one is verified
// against.
func GroupHasLDAPLink(ctx context.Context, client *gitlabclient.Client, groupID int64, provider string) (bool, error) {
	links, _, err := client.GL().Groups.ListGroupLDAPLinks(groupID, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		// A group with no link at all answers 404 rather than an empty page.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("listing the LDAP links of group %d: %w", groupID, err)
	}
	for _, link := range links {
		if link.Provider == provider {
			return true, nil
		}
	}
	return false, nil
}
