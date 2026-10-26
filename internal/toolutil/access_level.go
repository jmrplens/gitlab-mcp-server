// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// accessLevelNames maps GitLab access level values to human-readable labels.
var accessLevelNames = map[gl.AccessLevelValue]string{
	gl.NoPermissions:              "No access",
	gl.MinimalAccessPermissions:   "Minimal access",
	gl.GuestPermissions:           "Guest",
	gl.PlannerPermissions:         "Planner",
	gl.ReporterPermissions:        "Reporter",
	gl.SecurityManagerPermissions: "Security Manager",
	gl.DeveloperPermissions:       "Developer",
	gl.MaintainerPermissions:      "Maintainer",
	gl.OwnerPermissions:           "Owner",
	gl.AdminPermissions:           "Admin",
}

// AccessLevelDescription maps a GitLab access level to its human-readable
// label. A level the table does not name is rendered with its number, "Level
// 35", because the number is what GitLab sent and the one thing a reader can
// act on; "Unknown" told them nothing, which is why two packages kept a copy
// of this table with the numeric fallback of their own.
func AccessLevelDescription(level gl.AccessLevelValue) string {
	if name, ok := accessLevelNames[level]; ok {
		return name
	}
	return fmt.Sprintf("Level %d", level)
}

// AccessLevelSet is the membership access levels one family of GitLab routes
// accepts, as GitLab declares them at the release this server is pinned to,
// and the one place a schema, a description or a refusal reads them from
// (issue 1280). The schemas used to be written by hand per field, and offered
// 0, which no route accepts, 5 on a project, where no edition accepts it, and
// 60 on filters, which refuse it.
//
// Planner (15) and Security Manager (25) are in every set that takes the
// whole range, on every tier: Gitlab::Access.options lists both on Community
// Edition, so the "(Premium/Ultimate)" the descriptions gave them was wrong.
type AccessLevelSet struct {
	levels []gl.AccessLevelValue
	// minimalOnTopLevelGroupOnly says that the set's Minimal access is
	// granted only on a top-level group whose license includes the role,
	// which EE's GroupMember#access_level_inclusion holds every group write
	// to; anywhere else GitLab refuses it or, on an approval, leaves the
	// request pending.
	minimalOnTopLevelGroupOnly bool
}

// everyRole is every membership role from Guest to Owner,
// Gitlab::Access.all_values, in GitLab's order.
var everyRole = []gl.AccessLevelValue{
	gl.GuestPermissions, gl.PlannerPermissions, gl.ReporterPermissions, gl.SecurityManagerPermissions,
	gl.DeveloperPermissions, gl.MaintainerPermissions, gl.OwnerPermissions,
}

// withMinimalAccess returns Minimal access followed by levels.
func withMinimalAccess(levels []gl.AccessLevelValue) []gl.AccessLevelValue {
	return slices.Concat([]gl.AccessLevelValue{gl.MinimalAccessPermissions}, levels)
}

// The sets, each named for the routes that accept it.
var (
	// FilterAccessLevels is what min_access_level and shared_min_access_level
	// take on every list that has them: each declares values:
	// Gitlab::Access.all_values, Guest to Owner in every edition.
	FilterAccessLevels = AccessLevelSet{levels: everyRole}
	// ProjectAccessLevels is what a project grants a member, an invitee, a
	// group it is shared with and an approved access request: Member's
	// access_level_inclusion and the share's values: both hold it to
	// Gitlab::Access.all_values, so Minimal access is refused on a project
	// in every edition.
	ProjectAccessLevels = AccessLevelSet{levels: everyRole}
	// GroupAccessLevels is what a group grants a member, an invitee, a group
	// it is shared with, a SAML group link and an approved access request:
	// Guest to Owner, and Minimal access on a top-level group whose license
	// includes it. 60 (Admin) is not a membership level, and the group member
	// update refuses it.
	GroupAccessLevels = AccessLevelSet{levels: withMinimalAccess(everyRole), minimalOnTopLevelGroupOnly: true}
	// LDAPGroupLinkAccessLevels is what an LDAP group link grants: the route
	// declares values_with_minimal_access and the link checks nothing
	// beside it, so Minimal access is taken on a subgroup as well.
	LDAPGroupLinkAccessLevels = AccessLevelSet{levels: withMinimalAccess(everyRole)}
	// AccessTokenAccessLevels is what a project or group access token can be
	// created with: values: options_with_owner. Owner is accepted on a
	// project token as on a group one, from a creator who holds it, which
	// GitLab's own service spec asserts; the description that called it
	// invalid on a project was wrong.
	AccessTokenAccessLevels = AccessLevelSet{levels: everyRole}
	// BroadcastTargetAccessLevels is what a broadcast message can target:
	// System::BroadcastMessage::ALLOWED_TARGET_ACCESS_LEVELS, which leaves
	// Security Manager out.
	BroadcastTargetAccessLevels = AccessLevelSet{levels: []gl.AccessLevelValue{
		gl.GuestPermissions, gl.PlannerPermissions, gl.ReporterPermissions,
		gl.DeveloperPermissions, gl.MaintainerPermissions, gl.OwnerPermissions,
	}}
	// CustomRoleBaseAccessLevels is what a custom role can extend: the route
	// declares Gitlab::Access.all_values and MemberRole validates the level
	// against options_for_custom_roles, which drops Owner, so Guest to
	// Maintainer is what both accept. doc/api/member_roles.md lists Owner as
	// well, and GitLab refuses it.
	CustomRoleBaseAccessLevels = AccessLevelSet{levels: []gl.AccessLevelValue{
		gl.GuestPermissions, gl.PlannerPermissions, gl.ReporterPermissions, gl.SecurityManagerPermissions,
		gl.DeveloperPermissions, gl.MaintainerPermissions,
	}}
)

// Levels returns the set's levels in GitLab's order, as a slice of the
// caller's own.
func (s AccessLevelSet) Levels() []gl.AccessLevelValue {
	return slices.Clone(s.levels)
}

// Accepts reports whether level is one of the set's.
func (s AccessLevelSet) Accepts(level int) bool {
	return slices.Contains(s.levels, gl.AccessLevelValue(level))
}

// Choices renders the set as the descriptions and refusals offer it, each
// level as N=Role in GitLab's order, with where Minimal access is granted
// when that is only a top-level group.
func (s AccessLevelSet) Choices() string {
	choices := make([]string, 0, len(s.levels))
	for _, level := range s.levels {
		choice := strconv.Itoa(int(level)) + "=" + AccessLevelDescription(level)
		if level == gl.MinimalAccessPermissions && s.minimalOnTopLevelGroupOnly {
			choice += " (only on a top-level group with a Premium or Ultimate license)"
		}
		choices = append(choices, choice)
	}
	return strings.Join(choices, ", ")
}

// enum returns the set as the values of a JSON Schema enum.
func (s AccessLevelSet) enum() []any {
	values := make([]any, 0, len(s.levels))
	for _, level := range s.levels {
		values = append(values, int(level))
	}
	return values
}

// SchemaOverride returns the input-schema override that publishes the
// parameter at propertyPath as an integer enum of the set. The type is set as
// well because a level a caller may leave out, to be given GitLab's default,
// is a pointer field, which reflects as an integer or null, and null is no
// level.
func (s AccessLevelSet) SchemaOverride(propertyPath string) InputSchemaOverride {
	return SchemaPropertyOverride(propertyPath, map[string]any{"type": "integer", "enum": s.enum()})
}

// SchemaItemsOverride returns the same override for the items of the list
// parameter at propertyPath, for a parameter that takes several levels.
func (s AccessLevelSet) SchemaItemsOverride(propertyPath string) InputSchemaOverride {
	return SchemaItemsOverride(propertyPath, map[string]any{"type": "integer", "enum": s.enum()})
}

// canonicalAccessLevelParams maps the access level parameters GitLab declares
// with one value set on every route that takes them to that set, which
// [applyCanonicalAccessLevelEnums] publishes centrally, as
// [canonicalParamEnums] does for strings.
var canonicalAccessLevelParams = map[string]AccessLevelSet{
	"min_access_level":        FilterAccessLevels,
	"shared_min_access_level": FilterAccessLevels,
}

// applyCanonicalAccessLevelEnums injects the [canonicalAccessLevelParams] sets
// into the top-level integer properties of schema that do not already declare
// an enum. Per-action InputSchemaOverrides are applied first (see
// [NewActionSpec]) and therefore win.
func applyCanonicalAccessLevelEnums(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, set := range canonicalAccessLevelParams {
		prop, isMap := props[name].(map[string]any)
		if !isMap || prop["type"] != "integer" {
			continue
		}
		if _, has := prop["enum"]; has {
			continue
		}
		prop["enum"] = set.enum()
	}
}
