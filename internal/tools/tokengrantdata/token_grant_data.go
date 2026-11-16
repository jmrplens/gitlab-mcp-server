// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package tokengrantdata

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GranularScopeInput is one scope of the grant a token create action asks
// GitLab for: the granular_scope_params block of
// lib/api/helpers/personal_access_tokens_helpers.rb, which client-go sends as
// [gl.GranularScopeOptions].
type GranularScopeInput struct {
	Access      string   `json:"access" jsonschema:"Resources the scope reaches: personal_projects (the user's own projects), all_memberships (every project and group the user is a member of), selected_memberships (the projects and groups named in project_ids and group_ids), user (the user's own profile, tokens and namespaces) or instance,required"`
	Permissions []string `json:"permissions" jsonschema:"Assignable permissions the scope grants, by GitLab's assignable name for each (read_project, create_merge_request, read_user), not the raw permissions an assignable one bundles,required"`
	ProjectIDs  []int64  `json:"project_ids,omitempty" jsonschema:"IDs of the projects the scope reaches, with access selected_memberships only"`
	GroupIDs    []int64  `json:"group_ids,omitempty" jsonschema:"IDs of the groups the scope reaches, and everything below each, with access selected_memberships only"`
}

// AccessValues are the access values GitLab takes on a granular scope
// (GRANULAR_SCOPE_ACCESS_VALUES), in client-go's order.
var AccessValues = []string{
	string(gl.GranularScopeAccessPersonalProjects),
	string(gl.GranularScopeAccessAllMemberships),
	string(gl.GranularScopeAccessSelectedMemberships),
	string(gl.GranularScopeAccessUser),
	string(gl.GranularScopeAccessInstance),
}

// errScopesChoice is the refusal of a token create call that names neither
// or both of the two kinds of grant.
var errScopesChoice = errors.New("send exactly one of scopes (a classic token) or granular_scopes (a fine-grained token), not both")

// NotFoundHint explains the 404 a token create route answers a
// granular_scopes call with when the granular_personal_access_tokens feature
// flag is off for the token's user, which reads like a missing route.
const NotFoundHint = "GitLab answers 404 to granular_scopes when the granular_personal_access_tokens feature flag is off for the token's user, as it does for a user that does not exist; enable the flag or send classic scopes instead"

// CheckScopesChoice refuses a token create call that sends neither classic
// scopes nor a grant, or both: GitLab declares exactly_one_of on the
// current-user route, and mutually_exclusive on the impersonation route,
// whose token without either is refused by PersonalAccessToken's presence
// validation of scopes.
func CheckScopesChoice(scopes []string, granular []GranularScopeInput) error {
	if (len(scopes) == 0) == (len(granular) == 0) {
		return errScopesChoice
	}
	return nil
}

// ValidateScopes refuses, before anything is sent, a scope of the grant a
// caller asked for that GitLab would refuse or read otherwise than the
// caller wrote it: an access value outside [AccessValues], no permissions,
// project or group IDs beside an access other than selected_memberships,
// which GitLab drops and so widens the grant to all of that access, and
// selected_memberships with none, which GitLab turns into no scope at all.
func ValidateScopes(scopes []GranularScopeInput) error {
	for i, scope := range scopes {
		if !slices.Contains(AccessValues, scope.Access) {
			return fmt.Errorf("granular_scopes[%d].access must be one of %s", i, strings.Join(AccessValues, ", "))
		}
		if len(scope.Permissions) == 0 {
			return fmt.Errorf("granular_scopes[%d].permissions is required and must not be empty", i)
		}
		named := len(scope.ProjectIDs) > 0 || len(scope.GroupIDs) > 0
		selected := scope.Access == string(gl.GranularScopeAccessSelectedMemberships)
		if named != selected {
			return fmt.Errorf("granular_scopes[%d]: project_ids and group_ids go with access selected_memberships, which needs at least one of them", i)
		}
	}
	return nil
}

// Options spells a validated grant as client-go's options, one entry per
// scope, and nil for no grant, which client-go leaves out of the request.
func Options(scopes []GranularScopeInput) []*gl.GranularScopeOptions {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]*gl.GranularScopeOptions, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, &gl.GranularScopeOptions{
			Access:      new(gl.GranularScopeAccessValue(scope.Access)),
			Permissions: scope.Permissions,
			ProjectIDs:  scope.ProjectIDs,
			GroupIDs:    scope.GroupIDs,
		})
	}
	return out
}

// CheckGrant refuses what a token create call asks for before it is sent:
// neither or both kinds of grant ([CheckScopesChoice]), then a scope GitLab
// would refuse or read otherwise ([ValidateScopes]).
func CheckGrant(scopes []string, granular []GranularScopeInput) error {
	if err := CheckScopesChoice(scopes, granular); err != nil {
		return err
	}
	return ValidateScopes(granular)
}

// SchemaOverrides are the input-schema overrides of a token create action
// that takes classic scopes or a grant: exactly one of the two, as
// [CheckScopesChoice] enforces, and each scope's access held to
// [AccessValues].
func SchemaOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaOneOfRequired("scopes", "granular_scopes"),
		toolutil.SchemaEnumOverride("granular_scopes.access", AccessValues...),
	}
}

// WrapCreateErr wraps the error of a token create call: the 404 of a
// granular call with [NotFoundHint], and anything else as
// [toolutil.WrapErrWithStatusHint] does with the action's own hint for status.
func WrapCreateErr(op string, err error, granular bool, status int, hint string) error {
	if granular && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return toolutil.WrapErrWithHint(op, err, NotFoundHint)
	}
	return toolutil.WrapErrWithStatusHint(op, err, status, hint)
}
