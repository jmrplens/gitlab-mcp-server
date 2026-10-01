package finegrained

import (
	"encoding/json"
	"slices"
)

// GrantAccess is the access level of one granular scope, which says what
// namespaces it reaches (app/models/authz/granular_scope.rb).
type GrantAccess uint8

// The access levels GitLab 19.4 defines.
const (
	AccessPersonalProjects GrantAccess = iota + 1
	AccessSelectedMemberships
	AccessAllMemberships
	AccessUser
	AccessInstance
)

// accessNames are the spellings GitLab's API entity exposes, by level.
var accessNames = map[string]GrantAccess{
	"personal_projects":    AccessPersonalProjects,
	"selected_memberships": AccessSelectedMemberships,
	"all_memberships":      AccessAllMemberships,
	"user":                 AccessUser,
	"instance":             AccessInstance,
}

// NamespaceKind is the kind of namespace a scope is attached to, read from
// what the entity exposes: a project id only for a project namespace, a group
// id only for a group (lib/api/entities/personal_access_token_granular_scope.rb).
type NamespaceKind uint8

// The namespace kinds a scope can carry.
const (
	// NamespaceNone is a scope with no namespace: all memberships, the user
	// and the instance.
	NamespaceNone NamespaceKind = iota
	// NamespaceUser is the creating user's own namespace, which a personal
	// projects scope carries and the entity exposes as neither id.
	NamespaceUser
	// NamespaceGroup is a group, which covers the group and every project
	// under it.
	NamespaceGroup
	// NamespaceProject is one project's namespace.
	NamespaceProject
)

// Scope is one granular scope of a grant.
type Scope struct {
	Access    GrantAccess
	Namespace NamespaceKind
	// NamespaceID is the project or group id the entity exposes, 0 for the
	// kinds it exposes none for.
	NamespaceID int64
	// Permissions are assignable names, as a user grants them.
	Permissions []string
}

// Grant is what a fine-grained token was granted when it was created; nothing
// edits it afterwards.
type Grant struct {
	Scopes []Scope
}

// rawToken is the part of GET /personal_access_tokens/:id this reads, which
// client-go does not model.
type rawToken struct {
	Granular       *bool       `json:"granular"`
	GranularScopes *[]rawScope `json:"granular_scopes"`
}

// rawScope is one granular scope as the entity exposes it.
type rawScope struct {
	Access      string   `json:"access"`
	Permissions []string `json:"permissions"`
	ProjectID   *int64   `json:"project_id"`
	GroupID     *int64   `json:"group_id"`
}

// DecodeGrant reads a token's grant out of the body GitLab answers GET
// /personal_access_tokens/:id with, holding it to at most maxScopes scopes.
//
// It falls back rather than guesses. A body that does not decode, a token
// that says it is not fine-grained, an access level GitLab 19.4 does not
// define, a scope naming both a project and a group, and a selected
// memberships scope naming neither are the grant's shape being unknown; an
// absent or null scope list is a grant that cannot be read; more scopes than
// maxScopes is a grant too large. A present, empty list is a grant of nothing,
// and is returned as one.
//
// Scopes on the same namespace are unioned: GitLab validates every input of
// one creation request before it builds any, so one request can carry the
// same namespace twice, and several scopes with no namespace at once (a user
// and an instance scope together is what the startup permissions need).
func DecodeGrant(body []byte, maxScopes int) (Grant, FallbackReason) {
	var token rawToken
	if err := json.Unmarshal(body, &token); err != nil {
		return Grant{}, FallbackGrantShape
	}
	if token.Granular != nil && !*token.Granular {
		return Grant{}, FallbackGrantShape
	}
	if token.GranularScopes == nil || *token.GranularScopes == nil {
		return Grant{}, FallbackGrantUnreadable
	}
	raw := *token.GranularScopes
	if len(raw) > maxScopes {
		return Grant{}, FallbackGrantTooLarge
	}
	grant := Grant{Scopes: []Scope{}}
	for _, entry := range raw {
		scope, ok := decodeScope(entry)
		if !ok {
			return Grant{}, FallbackGrantShape
		}
		grant.Scopes = union(grant.Scopes, scope)
	}
	return grant, FallbackNone
}

// decodeScope reads one scope, and reports false for one it would have to
// guess at.
func decodeScope(entry rawScope) (Scope, bool) {
	access, known := accessNames[entry.Access]
	if !known || entry.ProjectID != nil && entry.GroupID != nil {
		return Scope{}, false
	}
	scope := Scope{Access: access, Permissions: slices.Clone(entry.Permissions)}
	switch {
	case entry.ProjectID != nil:
		scope.Namespace, scope.NamespaceID = NamespaceProject, *entry.ProjectID
	case entry.GroupID != nil:
		scope.Namespace, scope.NamespaceID = NamespaceGroup, *entry.GroupID
	case access == AccessPersonalProjects:
		// Both creation paths attach a personal projects scope to the creating
		// user's namespace, which the entity exposes as neither id.
		scope.Namespace = NamespaceUser
	case access == AccessSelectedMemberships:
		return Scope{}, false
	}
	return scope, true
}

// union adds a scope to a grant, merging its permissions into a scope already
// held on the same access level and namespace.
func union(scopes []Scope, scope Scope) []Scope {
	for i := range scopes {
		held := &scopes[i]
		if held.Access == scope.Access && held.Namespace == scope.Namespace && held.NamespaceID == scope.NamespaceID {
			for _, permission := range scope.Permissions {
				if !slices.Contains(held.Permissions, permission) {
					held.Permissions = append(held.Permissions, permission)
				}
			}
			return scopes
		}
	}
	return append(scopes, scope)
}
