// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"fmt"
	"slices"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// EnvironmentAccess is one entry of a protected environment's
// deploy_access_levels or approval_rules, reduced to the fields GitLab's
// rules about an entry read. The project and the group protected environment
// packages send the same four entry shapes, a deploy access level and an
// approval rule to protect with and to update with, which
// internal/tools/protectedenvdata declares once, since GitLab serves the
// project and the group routes from the same param blocks
// (ee/lib/api/protected_environments.rb). Every shape answers itself through
// [EnvironmentAccesser], which is how the rules are written once here without
// importing that package, which imports this one. A field a shape does not
// carry stays nil.
type EnvironmentAccess struct {
	ID                   *int64
	AccessLevel          *int
	UserID               *int64
	GroupID              *int64
	GroupInheritanceType *int64
	RequiredApprovals    *int64
	Destroy              *bool
}

// EnvironmentAccesser is an entry shape the protected environment checks
// read: one that answers its fields as an [EnvironmentAccess].
type EnvironmentAccesser interface {
	// EnvironmentAccess answers the entry with the fields it carries.
	EnvironmentAccess() EnvironmentAccess
}

// environmentGrantees are the three fields an entry names who it grants by:
// a role, a user or a group.
var environmentGrantees = []string{"access_level", "user_id", "group_id"}

// environmentAccessLevels are the roles an entry may grant, Reporter,
// Developer, Maintainer and Admin: ALLOWED_ACCESS_LEVELS in
// ee/app/models/protected_environments/authorizable.rb. Grape refuses any
// other with 400, 0 (No access) included, which the deploy access level's
// description used to offer.
var environmentAccessLevels = []int{
	int(gl.ReporterPermissions),
	int(gl.DeveloperPermissions),
	int(gl.MaintainerPermissions),
	int(gl.AdminPermissions),
}

// environmentInheritanceTypes are the values of group_inheritance_type,
// GROUP_INHERITANCE_TYPE in the same file: 0 for a group's direct members and
// 1 for its inherited members too.
var environmentInheritanceTypes = []int64{0, 1}

// The approvals an approval rule may require, as the approval rule model
// validates them (ee/app/models/protected_environments/approval_rule.rb).
// GitLab's default, when the rule names none, is 1.
const (
	minimumRequiredApprovals = 1
	maximumRequiredApprovals = 5
)

// CheckEnvironmentProtect refuses a protect GitLab would refuse, before it is
// sent: no deploy access level, a deploy access level naming none or more
// than one of access_level, user_id and group_id, an approval rule naming
// none of them, a role, an inheritance or a number of approvals GitLab does
// not take, and a required_approval_count other than 0.
//
// GitLab refuses each of these itself, a deploy access level naming two with
// 422 (the ExactlyOnePresentValidator of
// ee/app/models/protected_environments/deploy_access_level.rb) and an
// approval rule naming none with 400 (the at_least_one_of of
// ee/lib/api/protected_environments.rb), but only a schema validator holds an
// entry to its item schema: the SDK's, on the individual tool and on the meta
// tool when its parameter schema is full. The meta dispatcher checks the root
// required list alone and the dynamic one reads no item schema, so on a meta
// tool whose parameter schema is opaque or compact, and on dynamic, the call
// reaches the handler as it came, and find's example used to send a deploy
// access level naming nothing (issue 1271). Refusing here words the refusal
// the same wherever no schema refused it first, and saves the round trip.
func CheckEnvironmentProtect[D, A EnvironmentAccesser](operation string, deploy []D, approval []A, requiredApprovalCount *int64) error {
	if len(deploy) == 0 {
		return fmt.Errorf("%s: deploy_access_levels needs at least one entry, each naming exactly one of access_level, user_id or group_id", operation)
	}
	return checkEnvironmentAccess(operation, deploy, approval, requiredApprovalCount, false)
}

// CheckEnvironmentUpdate refuses an update GitLab would refuse, before it is
// sent. An update's entry may name an existing one by its id, so the rules
// are those GitLab's own give for one: a deploy access level names an
// existing entry by id or exactly one of access_level, user_id and group_id,
// and never two of them, and an approval rule names at least one of the three
// unless _destroy is true, even when it carries an id, which is what Grape's
// given block on _destroy demands. The values are held as on a protect but
// for the approvals of a rule _destroy removes, which Rails never validates,
// and either list may be absent, since an update need not touch it.
func CheckEnvironmentUpdate[D, A EnvironmentAccesser](operation string, deploy []D, approval []A, requiredApprovalCount *int64) error {
	return checkEnvironmentAccess(operation, deploy, approval, requiredApprovalCount, true)
}

// checkEnvironmentAccess holds the deprecated count and every entry of both
// lists to their rules, reporting the first entry that breaks one by its
// list and index.
func checkEnvironmentAccess[D, A EnvironmentAccesser](operation string, deploy []D, approval []A, requiredApprovalCount *int64, update bool) error {
	if requiredApprovalCount != nil && *requiredApprovalCount != 0 {
		return fmt.Errorf("%s: required_approval_count is deprecated and GitLab takes only 0, so require approvals with required_approvals on approval_rules", operation)
	}
	for i, entry := range deploy {
		if problem := deployAccessProblem(entry.EnvironmentAccess(), update); problem != "" {
			return fmt.Errorf("%s: deploy_access_levels[%d] %s", operation, i, problem)
		}
	}
	for i, entry := range approval {
		if problem := approvalRuleProblem(entry.EnvironmentAccess(), update); problem != "" {
			return fmt.Errorf("%s: approval_rules[%d] %s", operation, i, problem)
		}
	}
	return nil
}

// deployAccessProblem says what a deploy access level breaks, empty when
// nothing. On an update an entry with an id needs no grantee, since it names
// the existing entry it changes or removes.
//
// The conditions are early returns rather than the cases of a tagless switch,
// whose expressions no coverage profile places, which leaves their mutants
// unmeasured.
func deployAccessProblem(access EnvironmentAccess, update bool) string {
	grantees := access.grantees()
	if len(grantees) > 1 {
		return "names " + joinNames(grantees) + ", and an entry names exactly one of access_level, user_id or group_id"
	}
	if len(grantees) == 1 || (update && access.ID != nil) {
		return access.valueProblem()
	}
	if update {
		return "names neither an existing entry by id nor one of access_level, user_id or group_id"
	}
	return "names none of access_level, user_id or group_id, and an entry names exactly one"
}

// approvalRuleProblem says what an approval rule breaks, empty when nothing.
// A rule _destroy removes is held to its values alone: Grape's given block
// exempts it from naming a grantee, and Rails validates no record it marks
// for destruction, so the approvals it asks for are never read. Grape holds
// its access_level and group_inheritance_type to their values all the same.
func approvalRuleProblem(access EnvironmentAccess, update bool) string {
	destroyed := access.destroyed()
	if len(access.grantees()) == 0 && !destroyed {
		if update {
			return "names none of access_level, user_id or group_id, and a rule names at least one unless _destroy is true"
		}
		return "names none of access_level, user_id or group_id, and a rule names at least one"
	}
	if count := access.RequiredApprovals; count != nil && !destroyed && (*count < minimumRequiredApprovals || *count > maximumRequiredApprovals) {
		return fmt.Sprintf("asks for %d required_approvals, and GitLab takes 1 to 5", *count)
	}
	return access.valueProblem()
}

// valueProblem says which value of an entry GitLab does not take, empty when
// it takes every one.
func (access EnvironmentAccess) valueProblem() string {
	if level := access.AccessLevel; level != nil && !slices.Contains(environmentAccessLevels, *level) {
		return fmt.Sprintf("grants access_level %d, and GitLab takes 20 (Reporter), 30 (Developer), 40 (Maintainer) or 60 (Admin)", *level)
	}
	if inheritance := access.GroupInheritanceType; inheritance != nil && !slices.Contains(environmentInheritanceTypes, *inheritance) {
		return fmt.Sprintf("sets group_inheritance_type %d, and GitLab takes 0 (direct members) or 1 (inherited members too)", *inheritance)
	}
	return ""
}

// grantees names the grantee fields an entry carries, in the order
// environmentGrantees lists them. A field counts once it is not nil, which is
// when the request carries it: a null in the call decodes to nil and is left
// out, as GitLab would never see it.
func (access EnvironmentAccess) grantees() []string {
	var names []string
	if access.AccessLevel != nil {
		names = append(names, "access_level")
	}
	if access.UserID != nil {
		names = append(names, "user_id")
	}
	if access.GroupID != nil {
		names = append(names, "group_id")
	}
	return names
}

// destroyed reports whether an entry asks to be removed.
func (access EnvironmentAccess) destroyed() bool {
	return access.Destroy != nil && *access.Destroy
}

// joinNames writes two or more names as a sentence does: "a and b", "a, b
// and c".
func joinNames(names []string) string {
	last := len(names) - 1
	return strings.Join(names[:last], ", ") + " and " + names[last]
}

// EnvironmentProtectSchemaOverrides returns the input schema overrides a
// protect action carries, stating the rules [CheckEnvironmentProtect] holds:
// at least one deploy access level, each naming exactly one grantee, each
// approval rule naming at least one and requiring 1 to 5 approvals, and the
// values GitLab takes.
func EnvironmentProtectSchemaOverrides() []InputSchemaOverride {
	overrides := []InputSchemaOverride{
		SchemaPropertyOverride("deploy_access_levels", map[string]any{"minItems": 1}),
		SchemaItemsOneOfRequired("deploy_access_levels", environmentGrantees...),
		SchemaItemsAnyOfRequired("approval_rules", environmentGrantees...),
		SchemaPropertyOverride("approval_rules.required_approvals", requiredApprovalsRange()),
	}
	return append(overrides, environmentValueOverrides()...)
}

// EnvironmentUpdateSchemaOverrides returns the input schema overrides an
// update action carries, stating the rules [CheckEnvironmentUpdate] holds: a
// deploy access level naming an id or a grantee and never two grantees, an
// approval rule naming a grantee and requiring 1 to 5 approvals unless
// _destroy is true, and the values GitLab takes.
//
// The approvals are bounded in an else beside an if on _destroy rather than
// on the property, since a property's schema cannot read its sibling: Rails
// validates no record _destroy removes, so GitLab takes any count on one.
func EnvironmentUpdateSchemaOverrides() []InputSchemaOverride {
	overrides := []InputSchemaOverride{
		SchemaItemsAnyOfRequired("deploy_access_levels", "id", "access_level", "user_id", "group_id"),
		SchemaItemsAtMostOneOf("deploy_access_levels", environmentGrantees...),
		SchemaItemsAnyOfRequiredUnlessTrue("approval_rules", "_destroy", environmentGrantees...),
		SchemaItemsOverride("approval_rules", map[string]any{
			"if":   trueBranch("_destroy"),
			"else": map[string]any{"properties": map[string]any{"required_approvals": requiredApprovalsRange()}},
		}),
	}
	return append(overrides, environmentValueOverrides()...)
}

// requiredApprovalsRange is the schema of the approvals an approval rule may
// require.
func requiredApprovalsRange() map[string]any {
	return map[string]any{"minimum": minimumRequiredApprovals, "maximum": maximumRequiredApprovals}
}

// environmentValueOverrides states the values GitLab takes on both lists and
// the deprecated count. Each enum holds null beside the values, because the
// fields are nullable and a null is a field the request leaves out, which
// GitLab accepts: an enum without it would refuse a call such as
// {"user_id": 5, "access_level": null}.
func environmentValueOverrides() []InputSchemaOverride {
	levels := nullableEnum(environmentAccessLevels)
	inheritance := nullableEnum(environmentInheritanceTypes)
	return []InputSchemaOverride{
		SchemaPropertyOverride("deploy_access_levels.access_level", map[string]any{"enum": levels}),
		SchemaPropertyOverride("deploy_access_levels.group_inheritance_type", map[string]any{"enum": inheritance}),
		SchemaPropertyOverride("approval_rules.access_level", map[string]any{"enum": levels}),
		SchemaPropertyOverride("approval_rules.group_inheritance_type", map[string]any{"enum": inheritance}),
		SchemaPropertyOverride("required_approval_count", map[string]any{"minimum": 0, "maximum": 0}),
	}
}

// nullableEnum returns values as the members of an enum, followed by null.
// The slice is sized by append alone: a capacity worked out beforehand would
// change nothing anyone can observe, and gremlins reported the arithmetic
// that computed it as a mutant no test could kill.
func nullableEnum[T int | int64](values []T) []any {
	var enum []any
	for _, value := range values {
		enum = append(enum, value)
	}
	return append(enum, nil)
}
