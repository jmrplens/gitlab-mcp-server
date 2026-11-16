// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package finegrained

import (
	"slices"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// GrantAccess is the access level of one granular scope, which says what
// namespaces it reaches (app/models/authz/granular_scope.rb). It is the
// register's type, the input [tenancy.CoverableAt] reads.
type GrantAccess = tenancy.GrantAccess

// The access levels GitLab 19.4 defines.
const (
	AccessPersonalProjects    = tenancy.AccessPersonalProjects
	AccessSelectedMemberships = tenancy.AccessSelectedMemberships
	AccessAllMemberships      = tenancy.AccessAllMemberships
	AccessUser                = tenancy.AccessUser
	AccessInstance            = tenancy.AccessInstance
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
// It is the register's type, the input [tenancy.CoverableAt] reads.
type NamespaceKind = tenancy.NamespaceKind

// The namespace kinds a scope can carry.
const (
	// NamespaceNone is a scope with no namespace: all memberships, the user
	// and the instance.
	NamespaceNone = tenancy.NamespaceNone
	// NamespaceUser is the creating user's own namespace, which a personal
	// projects scope carries and the entity exposes as neither id.
	NamespaceUser = tenancy.NamespaceUser
	// NamespaceGroup is a group, which covers the group and every project
	// under it.
	NamespaceGroup = tenancy.NamespaceGroup
	// NamespaceProject is one project's namespace.
	NamespaceProject = tenancy.NamespaceProject
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

// GrantScope is one granular scope as GitLab's entity exposes it
// (lib/api/entities/personal_access_token_granular_scope.rb), which the
// caller reads from what client-go decodes of GET
// /personal_access_tokens/:id. The package takes it in its own type so that
// what it decides stays independent of the SDK.
type GrantScope struct {
	Access      string
	Permissions []string
	ProjectID   *int64
	GroupID     *int64
}

// GrantFromScopes reads a token's grant out of what GitLab answered GET
// /personal_access_tokens/:id with, holding it to at most maxScopes scopes:
// granular is the token's granular flag, nil when the answer did not carry
// it, and scopes its granular scopes, nil when the answer carried none or
// null and empty, not nil, when it carried an empty list.
//
// It falls back rather than guesses. A token that says it is not
// fine-grained, an access level GitLab 19.4 does not define, a scope naming
// both a project and a group, and a selected memberships scope naming neither
// are the grant's shape being unknown; an absent or null scope list is a grant
// that cannot be read; more scopes than maxScopes is a grant too large. A
// present, empty list is a grant of nothing, and is returned as one.
//
// Scopes on the same namespace are unioned: GitLab validates every input of
// one creation request before it builds any, so one request can carry the
// same namespace twice, and several scopes with no namespace at once (a user
// and an instance scope together is what the startup permissions need).
func GrantFromScopes(granular *bool, scopes []GrantScope, maxScopes int) (Grant, FallbackReason) {
	if granular != nil && !*granular {
		return Grant{}, FallbackGrantShape
	}
	// encoding/json decodes an absent key and a null alike into a nil slice,
	// so this one test reads both as unreadable.
	if scopes == nil {
		return Grant{}, FallbackGrantUnreadable
	}
	if len(scopes) > maxScopes {
		return Grant{}, FallbackGrantTooLarge
	}
	grant := Grant{Scopes: []Scope{}}
	for _, entry := range scopes {
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
func decodeScope(entry GrantScope) (Scope, bool) {
	access, known := accessNames[entry.Access]
	if !known || entry.ProjectID != nil && entry.GroupID != nil {
		return Scope{}, false
	}
	scope := Scope{Access: access, Permissions: slices.Clone(entry.Permissions)}
	if entry.ProjectID != nil {
		scope.Namespace, scope.NamespaceID = NamespaceProject, *entry.ProjectID
		return scope, true
	}
	if entry.GroupID != nil {
		scope.Namespace, scope.NamespaceID = NamespaceGroup, *entry.GroupID
		return scope, true
	}
	if access == AccessPersonalProjects {
		// Both creation paths attach a personal projects scope to the creating
		// user's namespace, which the entity exposes as neither id.
		scope.Namespace = NamespaceUser
		return scope, true
	}
	if access == AccessSelectedMemberships {
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
