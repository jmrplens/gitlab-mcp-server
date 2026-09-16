//go:build e2e

// group_credential.go builds the two group-scoped credentials a licensed
// instance offers, the SSH certificate authority and the custom member role,
// and the group wiki page beside them.
//
// The certificate's key is generated per fixture rather than taken from a
// constant, for the reason a deploy key's is: GitLab holds a key unique
// wherever it stores one, and two attempts offering the same bytes would have
// the second refused with "has already been taken", which reads as a defect
// in the case rather than as a collision between two runs.
//
// The member role is an instance role and not a group one, although a case
// naming it also names a group. A self-managed Ultimate instance refuses the
// group-level create as deprecated and points at the instance level, so the
// instance role is the one a Docker fixture can raise; the group is what the
// case's own prompt names, and the builder raises both.

package fixture

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// memberRoleBaseAccess is what a custom role is built on: Guest, the lowest
// level GitLab accepts for one.
const memberRoleBaseAccess = gl.GuestPermissions

// SSHCertificate is a group SSH certificate authority a builder created.
type SSHCertificate struct {
	// ID is what the certificate actions take.
	ID int64
	// Title is what it was created as.
	Title string
}

// MemberRole is a custom member role a builder created on the instance.
type MemberRole struct {
	// ID is what the member role actions take.
	ID int64
	// Name is what it was created as.
	Name string
}

// NewGroupSSHCertificate mints a key, registers it as the group's certificate
// authority and registers its removal.
func NewGroupSSHCertificate(e *harness.Env, group Group) SSHCertificate {
	e.T.Helper()

	key, keyErr := SSHPublicKey()
	if keyErr != nil {
		e.T.Fatal(keyErr)
	}
	title := e.Name("sshcert")

	certificate, err := retryTransient(e, "create group ssh certificate "+title, createRetries, func() (SSHCertificate, error) {
		return createGroupSSHCertificate(e.Ctx, e.Client(), group.ID, title, key)
	})
	if err != nil {
		e.T.Fatalf("creating SSH certificate %q in group %d: %v", title, group.ID, err)
	}

	e.Defer("group ssh certificate "+title, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteGroupSSHCertificate(ctx, e.Client(), group.ID, certificate.ID)
	})
	return certificate
}

// NewMemberRole creates a custom member role on the instance and registers
// its removal.
func NewMemberRole(e *harness.Env) MemberRole {
	e.T.Helper()

	name := e.Name("role")
	role, err := retryTransient(e, "create instance member role "+name, createRetries, func() (MemberRole, error) {
		return createInstanceMemberRole(e.Ctx, e.Client(), name)
	})
	if err != nil {
		e.T.Fatalf("creating instance member role %q: %v", name, err)
	}

	e.Defer("instance member role "+name, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteInstanceMemberRole(ctx, e.Client(), role.ID)
	})
	return role
}

// NewGroupWikiPage creates a Markdown page in the group's wiki and registers
// its deletion.
func NewGroupWikiPage(e *harness.Env, group Group) WikiPage {
	e.T.Helper()

	title := e.Name("groupwiki")
	page, err := retryTransient(e, "create group wiki page "+title, createRetries, func() (WikiPage, error) {
		return createGroupWikiPage(e.Ctx, e.Client(), group.ID, title)
	})
	if err != nil {
		e.T.Fatalf("creating wiki page %q in group %d: %v", title, group.ID, err)
	}

	e.Defer("group wiki page "+page.Slug, func(ctx context.Context) error {
		ctx, cancel := withCleanupTimeout(ctx)
		defer cancel()
		return deleteGroupWikiPage(ctx, e.Client(), group.ID, page.Slug)
	})
	return page
}

// createGroupSSHCertificate asks GitLab for the certificate.
func createGroupSSHCertificate(ctx context.Context, client *gitlabclient.Client, groupID int64, title, key string) (SSHCertificate, error) {
	created, _, err := client.GL().GroupSSHCertificates.CreateGroupSSHCertificate(groupID,
		&gl.CreateGroupSSHCertificateOptions{Key: new(key), Title: new(title)}, gl.WithContext(ctx))
	if err != nil {
		return SSHCertificate{}, err
	}
	return SSHCertificate{ID: created.ID, Title: created.Title}, nil
}

// deleteGroupSSHCertificate removes the certificate and tolerates one a case
// deleted.
func deleteGroupSSHCertificate(ctx context.Context, client *gitlabclient.Client, groupID, certificateID int64) error {
	_, err := client.GL().GroupSSHCertificates.DeleteGroupSSHCertificate(groupID, certificateID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting SSH certificate %d of group %d: %w", certificateID, groupID, err)
	}
	return nil
}

// GroupHasSSHCertificate reports whether the group still holds the
// certificate, which is what a case that deletes one is verified against.
// GitLab offers no read of a single certificate, so the listing answers.
func GroupHasSSHCertificate(ctx context.Context, client *gitlabclient.Client, groupID, certificateID int64) (bool, error) {
	certificates, _, err := client.GL().GroupSSHCertificates.ListGroupSSHCertificates(groupID, gl.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("listing the SSH certificates of group %d: %w", groupID, err)
	}
	for _, certificate := range certificates {
		if certificate.ID == certificateID {
			return true, nil
		}
	}
	return false, nil
}

// createInstanceMemberRole asks GitLab for the role.
func createInstanceMemberRole(ctx context.Context, client *gitlabclient.Client, name string) (MemberRole, error) {
	created, _, err := client.GL().MemberRolesService.CreateInstanceMemberRole(&gl.CreateMemberRoleOptions{
		Name:            new(name),
		BaseAccessLevel: new(memberRoleBaseAccess),
		Description:     new("e2e model evaluation custom role"),
		ReadCode:        new(true),
	}, gl.WithContext(ctx))
	if err != nil {
		return MemberRole{}, err
	}
	return MemberRole{ID: created.ID, Name: created.Name}, nil
}

// deleteInstanceMemberRole removes the role and tolerates one a case deleted.
func deleteInstanceMemberRole(ctx context.Context, client *gitlabclient.Client, roleID int64) error {
	_, err := client.GL().MemberRolesService.DeleteInstanceMemberRole(roleID, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting instance member role %d: %w", roleID, err)
	}
	return nil
}

// InstanceHasMemberRole reports whether the instance still holds the role,
// which is what a case that deletes one is verified against.
func InstanceHasMemberRole(ctx context.Context, client *gitlabclient.Client, roleID int64) (bool, error) {
	roles, _, err := client.GL().MemberRolesService.ListInstanceMemberRoles(gl.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("listing the instance member roles: %w", err)
	}
	for _, role := range roles {
		if role.ID == roleID {
			return true, nil
		}
	}
	return false, nil
}

// createGroupWikiPage asks GitLab for the page.
func createGroupWikiPage(ctx context.Context, client *gitlabclient.Client, groupID int64, title string) (WikiPage, error) {
	created, _, err := client.GL().GroupWikis.CreateGroupWikiPage(groupID, &gl.CreateGroupWikiPageOptions{
		Title:   new(title),
		Content: new(wikiContent),
		Format:  new(gl.WikiFormatMarkdown),
	}, gl.WithContext(ctx))
	if err != nil {
		return WikiPage{}, err
	}
	return WikiPage{Slug: created.Slug, Title: created.Title, Content: created.Content}, nil
}

// deleteGroupWikiPage removes the page and tolerates one a case deleted.
func deleteGroupWikiPage(ctx context.Context, client *gitlabclient.Client, groupID int64, slug string) error {
	_, err := client.GL().GroupWikis.DeleteGroupWikiPage(groupID, slug, gl.WithContext(ctx))
	if err != nil && !IsStatus(err, http.StatusNotFound) {
		return fmt.Errorf("deleting wiki page %q of group %d: %w", slug, groupID, err)
	}
	return nil
}

// GroupWikiPageExists reports whether the group's wiki still holds the page,
// which is what a case that deletes one is verified against.
func GroupWikiPageExists(ctx context.Context, client *gitlabclient.Client, groupID int64, slug string) (bool, error) {
	_, _, err := client.GL().GroupWikis.GetGroupWikiPage(groupID, slug, &gl.GetGroupWikiPageOptions{}, gl.WithContext(ctx))
	if IsStatus(err, http.StatusNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading wiki page %q of group %d: %w", slug, groupID, err)
	}
	return true, nil
}
