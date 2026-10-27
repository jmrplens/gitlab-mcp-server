// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// environment_access_test.go tests the rules a protected environment's deploy
// access levels and approval rules are held to before a protect or an update
// is sent, and the input schema overrides that state the same rules.
package toolutil

import (
	"maps"
	"reflect"
	"testing"
)

// entry is an entry type the checks read, as the four entry shapes of
// internal/tools/protectedenvdata are.
type entry EnvironmentAccess

// EnvironmentAccess answers the entry as it is.
func (e entry) EnvironmentAccess() EnvironmentAccess {
	return EnvironmentAccess(e)
}

// The operation the refusals below are reported under.
const opEnvironment = "protect environment"

// TestCheckEnvironmentProtect_RefusesWhatGitLabRefuses verifies the check a
// protect is put to, case by case: what each entry has to name, the values
// GitLab takes and the deprecated count, with the refusal worded as served.
func TestCheckEnvironmentProtect_RefusesWhatGitLabRefuses(t *testing.T) {
	developers := entry{AccessLevel: new(30)}
	cases := []struct {
		name     string
		deploy   []entry
		approval []entry
		count    *int64
		want     string
	}{
		{name: "a role", deploy: []entry{developers}},
		{name: "a user", deploy: []entry{{UserID: new(int64(5))}}},
		{name: "a group with its inheritance", deploy: []entry{{GroupID: new(int64(7)), GroupInheritanceType: new(int64(1))}}},
		{name: "every level GitLab takes", deploy: []entry{{AccessLevel: new(20)}, {AccessLevel: new(30)}, {AccessLevel: new(40)}, {AccessLevel: new(60)}}},
		{name: "a count of 0", deploy: []entry{developers}, count: new(int64(0))},
		{
			name:     "approval rules naming one grantee or more, with 1 and 5 approvals",
			deploy:   []entry{developers},
			approval: []entry{{UserID: new(int64(5)), RequiredApprovals: new(int64(1))}, {AccessLevel: new(40), GroupID: new(int64(7)), RequiredApprovals: new(int64(5))}},
		},
		{name: "no deploy entry", want: "protect environment: deploy_access_levels needs at least one entry, each naming exactly one of access_level, user_id or group_id"},
		{name: "an empty deploy list", deploy: []entry{}, want: "protect environment: deploy_access_levels needs at least one entry, each naming exactly one of access_level, user_id or group_id"},
		{
			name:   "a deploy entry naming nothing",
			deploy: []entry{developers, {}},
			want:   "protect environment: deploy_access_levels[1] names none of access_level, user_id or group_id, and an entry names exactly one",
		},
		{
			name:   "a deploy entry naming a role and a user",
			deploy: []entry{{AccessLevel: new(40), UserID: new(int64(5))}},
			want:   "protect environment: deploy_access_levels[0] names access_level and user_id, and an entry names exactly one of access_level, user_id or group_id",
		},
		{
			name:   "a deploy entry naming a user and a group",
			deploy: []entry{{UserID: new(int64(5)), GroupID: new(int64(7))}},
			want:   "protect environment: deploy_access_levels[0] names user_id and group_id, and an entry names exactly one of access_level, user_id or group_id",
		},
		{
			name:   "a deploy entry naming all three",
			deploy: []entry{{AccessLevel: new(40), UserID: new(int64(5)), GroupID: new(int64(7))}},
			want:   "protect environment: deploy_access_levels[0] names access_level, user_id and group_id, and an entry names exactly one of access_level, user_id or group_id",
		},
		{
			name:   "a deploy level GitLab does not take",
			deploy: []entry{{AccessLevel: new(0)}},
			want:   "protect environment: deploy_access_levels[0] grants access_level 0, and GitLab takes 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin)",
		},
		{
			name:   "a deploy inheritance GitLab does not take",
			deploy: []entry{{GroupID: new(int64(7)), GroupInheritanceType: new(int64(2))}},
			want:   "protect environment: deploy_access_levels[0] sets group_inheritance_type 2, and GitLab takes 0 (direct members) or 1 (inherited members too)",
		},
		{
			name:     "an approval rule naming nothing",
			deploy:   []entry{developers},
			approval: []entry{{AccessLevel: new(40)}, {RequiredApprovals: new(int64(2))}},
			want:     "protect environment: approval_rules[1] names none of access_level, user_id or group_id, and a rule names at least one",
		},
		{
			name:     "an approval rule asking for no approval",
			deploy:   []entry{developers},
			approval: []entry{{AccessLevel: new(40), RequiredApprovals: new(int64(0))}},
			want:     "protect environment: approval_rules[0] asks for 0 required_approvals, and GitLab takes 1 to 5",
		},
		{
			name:     "an approval rule asking for six approvals",
			deploy:   []entry{developers},
			approval: []entry{{AccessLevel: new(40), RequiredApprovals: new(int64(6))}},
			want:     "protect environment: approval_rules[0] asks for 6 required_approvals, and GitLab takes 1 to 5",
		},
		{
			name:     "an approval level GitLab does not take",
			deploy:   []entry{developers},
			approval: []entry{{AccessLevel: new(50)}},
			want:     "protect environment: approval_rules[0] grants access_level 50, and GitLab takes 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin)",
		},
		{
			name:     "an approval inheritance GitLab does not take",
			deploy:   []entry{developers},
			approval: []entry{{GroupID: new(int64(7)), GroupInheritanceType: new(int64(-1))}},
			want:     "protect environment: approval_rules[0] sets group_inheritance_type -1, and GitLab takes 0 (direct members) or 1 (inherited members too)",
		},
		{
			name:   "a count above 0",
			deploy: []entry{developers},
			count:  new(int64(1)),
			want:   "protect environment: required_approval_count is deprecated and GitLab takes only 0, so require approvals with required_approvals on approval_rules",
		},
		{
			name:   "a count below 0",
			deploy: []entry{developers},
			count:  new(int64(-1)),
			want:   "protect environment: required_approval_count is deprecated and GitLab takes only 0, so require approvals with required_approvals on approval_rules",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefusal(t, CheckEnvironmentProtect(opEnvironment, tc.deploy, tc.approval, tc.count), tc.want)
		})
	}
}

// TestCheckEnvironmentUpdate_RefusesWhatGitLabRefuses verifies the check an
// update is put to. An entry may name an existing one by its id, so a deploy
// entry with an id needs no grantee and never takes two, and an approval rule
// needs a grantee and 1 to 5 approvals unless _destroy is true, an existing
// rule included: Rails validates no record it destroys, so the approvals a
// removed rule asks for are never read, while its values still meet Grape's.
// The list may be empty or absent, since an update need not touch it.
func TestCheckEnvironmentUpdate_RefusesWhatGitLabRefuses(t *testing.T) {
	cases := []struct {
		name     string
		deploy   []entry
		approval []entry
		count    *int64
		want     string
	}{
		{name: "nothing to change"},
		{name: "an empty deploy list", deploy: []entry{}},
		{name: "an existing deploy entry by its id", deploy: []entry{{ID: new(int64(11)), Destroy: new(true)}}},
		{name: "an existing deploy entry given a role", deploy: []entry{{ID: new(int64(11)), AccessLevel: new(30)}}},
		{name: "a new deploy entry naming a group", deploy: []entry{{GroupID: new(int64(7))}}},
		{name: "an approval rule removed", approval: []entry{{ID: new(int64(21)), Destroy: new(true)}}},
		{name: "an approval rule naming a role", approval: []entry{{ID: new(int64(21)), AccessLevel: new(30), RequiredApprovals: new(int64(3))}}},
		{name: "a removed approval rule asking for no approval", approval: []entry{{ID: new(int64(21)), Destroy: new(true), AccessLevel: new(30), RequiredApprovals: new(int64(0))}}},
		{name: "a removed approval rule asking for six approvals", approval: []entry{{ID: new(int64(21)), Destroy: new(true), RequiredApprovals: new(int64(6))}}},
		{
			name:     "an existing approval rule asking for no approval",
			approval: []entry{{ID: new(int64(21)), AccessLevel: new(30), RequiredApprovals: new(int64(0))}},
			want:     "protect environment: approval_rules[0] asks for 0 required_approvals, and GitLab takes 1 to 5",
		},
		{
			name:     "an approval rule kept with _destroy false asking for six approvals",
			approval: []entry{{ID: new(int64(21)), Destroy: new(false), UserID: new(int64(5)), RequiredApprovals: new(int64(6))}},
			want:     "protect environment: approval_rules[0] asks for 6 required_approvals, and GitLab takes 1 to 5",
		},
		{
			name:   "a new deploy entry naming nothing",
			deploy: []entry{{Destroy: new(true)}},
			want:   "protect environment: deploy_access_levels[0] names neither an existing entry by id nor one of access_level, user_id or group_id",
		},
		{
			name:   "an existing deploy entry naming a role and a group",
			deploy: []entry{{ID: new(int64(11)), AccessLevel: new(30), GroupID: new(int64(7))}},
			want:   "protect environment: deploy_access_levels[0] names access_level and group_id, and an entry names exactly one of access_level, user_id or group_id",
		},
		{
			name:   "an existing deploy entry given a level GitLab does not take",
			deploy: []entry{{ID: new(int64(11)), AccessLevel: new(10)}},
			want:   "protect environment: deploy_access_levels[0] grants access_level 10, and GitLab takes 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin)",
		},
		{
			name:     "an existing approval rule naming nothing",
			approval: []entry{{ID: new(int64(21)), RequiredApprovals: new(int64(2))}},
			want:     "protect environment: approval_rules[0] names none of access_level, user_id or group_id, and a rule names at least one unless _destroy is true",
		},
		{
			name:     "an approval rule kept with _destroy false",
			approval: []entry{{ID: new(int64(21)), Destroy: new(false)}},
			want:     "protect environment: approval_rules[0] names none of access_level, user_id or group_id, and a rule names at least one unless _destroy is true",
		},
		{
			name:     "a removed approval rule with a level GitLab does not take",
			approval: []entry{{ID: new(int64(21)), Destroy: new(true), AccessLevel: new(0)}},
			want:     "protect environment: approval_rules[0] grants access_level 0, and GitLab takes 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin)",
		},
		{
			name:  "a count above 0",
			count: new(int64(3)),
			want:  "protect environment: required_approval_count is deprecated and GitLab takes only 0, so require approvals with required_approvals on approval_rules",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefusal(t, CheckEnvironmentUpdate(opEnvironment, tc.deploy, tc.approval, tc.count), tc.want)
		})
	}
}

// assertRefusal holds err to want, the whole refusal, or to no error when
// want is empty.
func assertRefusal(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Errorf("check = %v, want nil", err)
		}
		return
	}
	if err == nil || err.Error() != want {
		t.Errorf("check = %v, want %q", err, want)
	}
}

// TestEnvironmentSchemaOverrides_StateTheRulesTheChecksHold verifies the
// overrides the protect and the update actions of both packages carry, one by
// one: the list a protect needs, what each entry names, and the values GitLab
// takes, each nullable as the field it constrains is, since a null is a field
// the request leaves out. A protect's approval rules require 1 to 5
// approvals, and an update's only when _destroy does not remove the rule.
func TestEnvironmentSchemaOverrides_StateTheRulesTheChecksHold(t *testing.T) {
	values := []InputSchemaOverride{
		SchemaPropertyOverride("deploy_access_levels.access_level", map[string]any{"enum": []any{20, 30, 40, 60, nil}}),
		SchemaPropertyOverride("deploy_access_levels.group_inheritance_type", map[string]any{"enum": []any{int64(0), int64(1), nil}}),
		SchemaPropertyOverride("approval_rules.access_level", map[string]any{"enum": []any{20, 30, 40, 60, nil}}),
		SchemaPropertyOverride("approval_rules.group_inheritance_type", map[string]any{"enum": []any{int64(0), int64(1), nil}}),
		SchemaPropertyOverride("required_approval_count", map[string]any{"minimum": 0, "maximum": 0}),
	}
	grantees := []string{"access_level", "user_id", "group_id"}
	approvals := map[string]any{"minimum": 1, "maximum": 5}

	t.Run("protect", func(t *testing.T) {
		want := append([]InputSchemaOverride{
			SchemaPropertyOverride("deploy_access_levels", map[string]any{"minItems": 1}),
			SchemaItemsOneOfRequired("deploy_access_levels", grantees...),
			SchemaItemsAnyOfRequired("approval_rules", grantees...),
			SchemaPropertyOverride("approval_rules.required_approvals", approvals),
		}, values...)
		if got := EnvironmentProtectSchemaOverrides(); !reflect.DeepEqual(got, want) {
			t.Errorf("EnvironmentProtectSchemaOverrides() = %#v, want %#v", got, want)
		}
	})

	t.Run("update", func(t *testing.T) {
		want := append([]InputSchemaOverride{
			SchemaItemsAnyOfRequired("deploy_access_levels", "id", "access_level", "user_id", "group_id"),
			SchemaItemsAtMostOneOf("deploy_access_levels", grantees...),
			SchemaItemsAnyOfRequiredUnlessTrue("approval_rules", "_destroy", grantees...),
			SchemaItemsOverride("approval_rules", map[string]any{
				"if": map[string]any{
					"required":             []string{"_destroy"},
					"properties":           map[string]any{"_destroy": map[string]any{"const": true}},
					"additionalProperties": true,
				},
				"else": map[string]any{"properties": map[string]any{"required_approvals": approvals}},
			}),
		}, values...)
		if got := EnvironmentUpdateSchemaOverrides(); !reflect.DeepEqual(got, want) {
			t.Errorf("EnvironmentUpdateSchemaOverrides() = %#v, want %#v", got, want)
		}
	})
}

// environmentSchema is the input schema of a protect or an update as the SDK
// generates it from the packages' input types: two nullable lists of objects
// whose fields are all nullable, an approval rule's approvals among them, and
// the deprecated count.
func environmentSchema() map[string]any {
	nullable := func(kind string) map[string]any { return map[string]any{"type": []any{"null", kind}} }
	entries := func(names ...string) map[string]any {
		properties := map[string]any{
			"id":                     nullable("integer"),
			"access_level":           nullable("integer"),
			"user_id":                nullable("integer"),
			"group_id":               nullable("integer"),
			"group_inheritance_type": nullable("integer"),
			"_destroy":               nullable("boolean"),
		}
		for _, name := range names {
			properties[name] = nullable("integer")
		}
		return map[string]any{"type": []any{"null", "array"}, "items": map[string]any{"type": "object", "properties": properties}}
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"deploy_access_levels":    entries(),
			"approval_rules":          entries("required_approvals"),
			"required_approval_count": nullable("integer"),
		},
	}
}

// TestEnvironmentSchemaOverrides_LockedDownSchema_BoundTheApprovalsOfAKeptRule
// verifies the approvals an approval rule may ask for as a client receives
// the schema: the overrides applied to the input schema and locked down the
// way tools/list serves every tool. A protect's rule asks for 1 to 5, and so
// does an update's unless _destroy is true, a false or a null _destroy
// keeping the rule: Rails validates no record it marks for destruction, so
// GitLab takes any count on a rule _destroy removes. The update bounds the
// count in an else the lockdown leaves open, so a rule that stays still
// carries its grantee and its id beside the count, and nothing else.
func TestEnvironmentSchemaOverrides_LockedDownSchema_BoundTheApprovalsOfAKeptRule(t *testing.T) {
	type ruleCall struct {
		name  string
		rule  map[string]any
		valid bool
	}
	developers := []any{map[string]any{"access_level": 30}}
	actions := []struct {
		name      string
		overrides []InputSchemaOverride
		base      map[string]any
		rules     []ruleCall
	}{
		{
			name:      "protect",
			overrides: EnvironmentProtectSchemaOverrides(),
			base:      map[string]any{"deploy_access_levels": developers},
			rules: []ruleCall{
				{name: "one approval", rule: map[string]any{"user_id": 5, "required_approvals": 1}, valid: true},
				{name: "five approvals", rule: map[string]any{"access_level": 40, "required_approvals": 5}, valid: true},
				{name: "the approvals left out", rule: map[string]any{"group_id": 7}, valid: true},
				{name: "null approvals", rule: map[string]any{"group_id": 7, "required_approvals": nil}, valid: true},
				{name: "no approval", rule: map[string]any{"user_id": 5, "required_approvals": 0}},
				{name: "six approvals", rule: map[string]any{"user_id": 5, "required_approvals": 6}},
			},
		},
		{
			name:      "update",
			overrides: EnvironmentUpdateSchemaOverrides(),
			base:      map[string]any{},
			rules: []ruleCall{
				{name: "a kept rule asking for one approval", rule: map[string]any{"id": 21, "user_id": 5, "required_approvals": 1}, valid: true},
				{name: "a new rule asking for five approvals", rule: map[string]any{"access_level": 40, "required_approvals": 5}, valid: true},
				{name: "a kept rule leaving the approvals out", rule: map[string]any{"id": 21, "group_id": 7, "group_inheritance_type": 1}, valid: true},
				{name: "a kept rule with null approvals", rule: map[string]any{"id": 21, "group_id": 7, "required_approvals": nil}, valid: true},
				{name: "a rule _destroy false keeps asking for five approvals", rule: map[string]any{"id": 21, "_destroy": false, "user_id": 5, "required_approvals": 5}, valid: true},
				{name: "a removed rule asking for no approval", rule: map[string]any{"id": 21, "_destroy": true, "required_approvals": 0}, valid: true},
				{name: "a removed rule asking for six approvals", rule: map[string]any{"id": 21, "_destroy": true, "user_id": 5, "required_approvals": 6}, valid: true},
				{name: "a kept rule asking for no approval", rule: map[string]any{"id": 21, "user_id": 5, "required_approvals": 0}},
				{name: "a kept rule asking for six approvals", rule: map[string]any{"id": 21, "user_id": 5, "required_approvals": 6}},
				{name: "a rule _destroy false keeps asking for six approvals", rule: map[string]any{"id": 21, "_destroy": false, "user_id": 5, "required_approvals": 6}},
				{name: "a rule with a null _destroy asking for six approvals", rule: map[string]any{"id": 21, "_destroy": nil, "user_id": 5, "required_approvals": 6}},
				{name: "a kept rule carrying a property it does not declare", rule: map[string]any{"id": 21, "user_id": 5, "required_approvals": 2, "access_level_description": "Release managers"}},
			},
		},
	}
	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			schema := environmentSchema()
			applyInputSchemaOverrides(schema, action.overrides)
			locked, isMap := lockedDownSchema(schema).(map[string]any)
			if !isMap {
				t.Fatalf("lockedDownSchema() = %T, want a map", lockedDownSchema(schema))
			}
			resolved := resolveSchemaForTest(t, locked)
			for _, rule := range action.rules {
				t.Run(rule.name, func(t *testing.T) {
					arguments := maps.Clone(action.base)
					arguments["approval_rules"] = []any{rule.rule}
					if err := resolved.Validate(arguments); (err == nil) != rule.valid {
						t.Errorf("Validate(%v) error = %v, want valid %t", arguments, err, rule.valid)
					}
				})
			}
		})
	}
}

// TestEnvironmentSchemaOverrides_ReturnFreshLists verifies that each call
// answers lists of its own, since every action spec keeps what it is given
// and an edit to one action's overrides must not reach another's.
func TestEnvironmentSchemaOverrides_ReturnFreshLists(t *testing.T) {
	for name, build := range map[string]func() []InputSchemaOverride{
		"protect": EnvironmentProtectSchemaOverrides,
		"update":  EnvironmentUpdateSchemaOverrides,
	} {
		t.Run(name, func(t *testing.T) {
			first := build()
			first[0].PropertyPath = "edited"
			first[len(first)-1].Values["maximum"] = 9
			if second := build(); second[0].PropertyPath == "edited" || second[len(second)-1].Values["maximum"] != 0 {
				t.Errorf("a second call answers %#v after the first was edited", second)
			}
		})
	}
}
