// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// access_level_test.go verifies the human-readable label mapping for GitLab
// access level constants used in project/group member outputs, and the sets
// of levels each family of GitLab routes accepts.
package toolutil

import (
	"reflect"
	"slices"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestAccessLevelDescription verifies human-readable labels for every known
// GitLab access level, plus the fallback for a value the table does not name,
// which keeps the number: "Unknown" told the reader nothing about what GitLab
// sent, and a level this server has no word for is still a level.
func TestAccessLevelDescription(t *testing.T) {
	tests := []struct {
		name  string
		level gl.AccessLevelValue
		want  string
	}{
		{"no access", gl.NoPermissions, "No access"},
		{"guest", gl.GuestPermissions, "Guest"},
		{"planner", gl.PlannerPermissions, "Planner"},
		{"reporter", gl.ReporterPermissions, "Reporter"},
		{"security manager", gl.SecurityManagerPermissions, "Security Manager"},
		{"developer", gl.DeveloperPermissions, "Developer"},
		{"maintainer", gl.MaintainerPermissions, "Maintainer"},
		{"owner", gl.OwnerPermissions, "Owner"},
		{"admin", gl.AdminPermissions, "Admin"},
		{"minimal", gl.MinimalAccessPermissions, "Minimal access"},
		{"unknown value keeps the number", gl.AccessLevelValue(99), "Level 99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccessLevelDescription(tt.level)
			if got != tt.want {
				t.Errorf("AccessLevelDescription(%d) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

// TestAccessLevelSets_HoldWhatGitLabAccepts verifies each set against the
// levels GitLab accepts at v19.5.0-ee on the routes it is for (issue 1280),
// written here as numbers so the test does not read the table it checks.
func TestAccessLevelSets_HoldWhatGitLabAccepts(t *testing.T) {
	tests := []struct {
		name string
		set  AccessLevelSet
		want []int
	}{
		{name: "filters", set: FilterAccessLevels, want: []int{10, 15, 20, 25, 30, 40, 50}},
		{name: "project", set: ProjectAccessLevels, want: []int{10, 15, 20, 25, 30, 40, 50}},
		{name: "group", set: GroupAccessLevels, want: []int{5, 10, 15, 20, 25, 30, 40, 50}},
		{name: "ldap", set: LDAPGroupLinkAccessLevels, want: []int{5, 10, 15, 20, 25, 30, 40, 50}},
		{name: "access tokens", set: AccessTokenAccessLevels, want: []int{10, 15, 20, 25, 30, 40, 50}},
		{name: "broadcast targets", set: BroadcastTargetAccessLevels, want: []int{10, 15, 20, 30, 40, 50}},
		{name: "custom role bases", set: CustomRoleBaseAccessLevels, want: []int{10, 15, 20, 25, 30, 40}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, level := range tt.set.Levels() {
				got = append(got, int(level))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Levels() = %v, want %v", got, tt.want)
			}
			for _, level := range []int{0, 1, 5, 10, 25, 50, 60} {
				if accepted := tt.set.Accepts(level); accepted != slices.Contains(tt.want, level) {
					t.Errorf("Accepts(%d) = %v, want %v", level, accepted, !accepted)
				}
			}
		})
	}
}

// TestAccessLevelSet_Levels_IsTheCallersCopy verifies that changing the
// slice Levels returns leaves the set as it was: the sets are shared by every
// action that reads them.
func TestAccessLevelSet_Levels_IsTheCallersCopy(t *testing.T) {
	levels := ProjectAccessLevels.Levels()
	levels[0] = gl.AdminPermissions
	if ProjectAccessLevels.Accepts(int(gl.AdminPermissions)) || !ProjectAccessLevels.Accepts(int(gl.GuestPermissions)) {
		t.Errorf("changing the returned slice changed the set: %v", ProjectAccessLevels.Levels())
	}
}

// TestAccessLevelSet_Choices_NamesEachLevelOnceAndWhereMinimalAccessIsGranted
// verifies the text the descriptions and refusals offer: each level as
// N=Role, in GitLab's order, and Minimal access with the place GitLab grants
// it when that is only a top-level group.
func TestAccessLevelSet_Choices_NamesEachLevelOnceAndWhereMinimalAccessIsGranted(t *testing.T) {
	tests := []struct {
		name string
		set  AccessLevelSet
		want string
	}{
		{name: "every role", set: ProjectAccessLevels, want: "10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner"},
		{name: "group", set: GroupAccessLevels, want: "5=Minimal access (only on a top-level group with a Premium or Ultimate license), 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner"},
		{name: "ldap", set: LDAPGroupLinkAccessLevels, want: "5=Minimal access, 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner"},
		{name: "broadcast targets", set: BroadcastTargetAccessLevels, want: "10=Guest, 15=Planner, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner"},
		{name: "custom role bases", set: CustomRoleBaseAccessLevels, want: "10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.set.Choices(); got != tt.want {
				t.Errorf("Choices() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAccessLevelSet_SchemaOverride_PublishesTheSetAsAnIntegerEnum verifies
// the override an action applies to a level parameter: an integer enum of
// the set, which also drops the null a pointer field is reflected with, since
// leaving the parameter out is how a caller asks for GitLab's default.
func TestAccessLevelSet_SchemaOverride_PublishesTheSetAsAnIntegerEnum(t *testing.T) {
	override := ProjectAccessLevels.SchemaOverride(" access_level ")
	if override.PropertyPath != "access_level" || override.Items {
		t.Fatalf("override = %+v, want one for the property access_level itself", override)
	}
	want := map[string]any{"type": "integer", "enum": []any{10, 15, 20, 25, 30, 40, 50}}
	if !reflect.DeepEqual(override.Values, want) {
		t.Errorf("Values = %#v, want %#v", override.Values, want)
	}
	items := BroadcastTargetAccessLevels.SchemaItemsOverride("target_access_levels")
	if items.PropertyPath != "target_access_levels" || !items.Items {
		t.Fatalf("items override = %+v, want one for the items of target_access_levels", items)
	}
	wantItems := map[string]any{"type": "integer", "enum": []any{10, 15, 20, 30, 40, 50}}
	if !reflect.DeepEqual(items.Values, wantItems) {
		t.Errorf("items Values = %#v, want %#v", items.Values, wantItems)
	}
}

// TestApplyCanonicalAccessLevelEnums_ConstrainsEveryMinimumAccessFilter
// verifies the central pass over the two filter names GitLab declares with
// one value set on every route that takes them: an integer property gains the
// filter levels as its enum, while an enum an action set itself, a property
// of another type and a property of another name are left alone.
func TestApplyCanonicalAccessLevelEnums_ConstrainsEveryMinimumAccessFilter(t *testing.T) {
	filters := []any{10, 15, 20, 25, 30, 40, 50}
	schema := map[string]any{"properties": map[string]any{
		"min_access_level":        map[string]any{"type": "integer"},
		"shared_min_access_level": map[string]any{"type": "integer", "enum": []any{30}},
		"access_level":            map[string]any{"type": "integer"},
	}}
	applyCanonicalAccessLevelEnums(schema)
	props := schema["properties"].(map[string]any)
	if got := props["min_access_level"].(map[string]any)["enum"]; !reflect.DeepEqual(got, filters) {
		t.Errorf("min_access_level enum = %#v, want %#v", got, filters)
	}
	if got := props["shared_min_access_level"].(map[string]any)["enum"]; !reflect.DeepEqual(got, []any{30}) {
		t.Errorf("shared_min_access_level enum = %#v, want the action's own [30]", got)
	}
	if _, has := props["access_level"].(map[string]any)["enum"]; has {
		t.Error("access_level gained an enum, though no central set is GitLab's for it")
	}

	stringFilter := map[string]any{"properties": map[string]any{"min_access_level": map[string]any{"type": "string"}}}
	applyCanonicalAccessLevelEnums(stringFilter)
	if _, has := stringFilter["properties"].(map[string]any)["min_access_level"].(map[string]any)["enum"]; has {
		t.Error("a string min_access_level gained the integer enum")
	}
	for _, shape := range []map[string]any{nil, {}, {"properties": "not a map"}, {"properties": map[string]any{"min_access_level": "not a map"}}} {
		applyCanonicalAccessLevelEnums(shape)
	}
}

// TestNewActionSpec_MinimumAccessFilter_PublishesTheFilterLevels verifies
// the central pass is part of every action's derived input schema, so a list
// action publishing min_access_level offers only what GitLab's filter
// accepts without declaring it.
func TestNewActionSpec_MinimumAccessFilter_PublishesTheFilterLevels(t *testing.T) {
	route := ActionRoute{InputSchema: map[string]any{"type": "object", "properties": map[string]any{
		"min_access_level": map[string]any{"type": "integer", "description": "Minimum access level"},
	}}}
	spec := NewActionSpec("list", route, ActionSpecOptions{})
	prop := spec.Route.InputSchema["properties"].(map[string]any)["min_access_level"].(map[string]any)
	if got := prop["enum"]; !reflect.DeepEqual(got, []any{10, 15, 20, 25, 30, 40, 50}) {
		t.Errorf("min_access_level enum = %#v, want the filter levels", got)
	}
}
