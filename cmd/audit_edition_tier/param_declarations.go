// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// The answers to the parameter findings: a row GitLab's API pages mark for a
// paid tier whose parameter a lower tier's schema offers, read against
// GitLab's source and answered by something other than a tier tag.

package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// paramDeclaration answers the findings of one action's parameter that a tier
// tag does not. A tagged parameter needs none: the tier filter prunes it from
// the lower tiers' schemas and its finding goes away by itself (issue 1240).
type paramDeclaration struct {
	// Documented is the tier the row names. A declaration answers a finding
	// only of that tier, since a row that starts naming another one is a lead
	// its reason was not written for.
	Documented tier
	// Category says what kind of answer this is, so a reader can tell the
	// kinds apart without reading every reason, and decides what the schema
	// is held to.
	Category string
	// Values are the values of the parameter the row's tier gates. A
	// [categoryGatedValueNotOffered] declaration holds the enum to leaving
	// them out, and a [categoryValueLevelGate] one the description to naming
	// the tier beside each of them, which it has to name whenever the
	// parameter enumerates its values. [categoryNotGated] does not read them.
	Values []int
	// Reason says why, with the GitLab source that settles it, in the words a
	// reviewer needs to judge whether it still holds.
	Reason string
}

// Parameter declaration categories.
const (
	// categoryValueLevelGate is a row that marks the whole parameter for a
	// tier that gates only some of its values, so the parameter stays on the
	// lower tier's schema and its description says which values need the
	// tier. On a parameter that enumerates its values, in an enum or in its
	// description, the declaration names the gated ones, the enum has to
	// offer each, and the description the lowest tier offering the parameter
	// is served has to name the row's tier beside each one; a gate on
	// something other than a value, such as how many assignees an issue
	// takes, is held to a description naming the tier.
	categoryValueLevelGate = "value-level-gate"
	// categoryGatedValueNotOffered is a row whose tier gates a value this
	// action never offers, because the page documents a project and a group
	// route in one table and only the group takes the value: the parameter
	// has to publish an enum, and the enum has to leave out every value the
	// declaration names.
	categoryGatedValueNotOffered = "gated-value-not-offered"
	// categoryNotGated is a row GitLab's source contradicts: the lower tier is
	// served the parameter and honors it. The reason cites the source line,
	// and the row is a documentation fix to offer GitLab.
	categoryNotGated = "not-gated"
)

// Reasons. Each is one literal, as every declaration table here writes its
// reasons, since a constant expression is code no test runs.
const (
	// reasonMinimalAccessOnAGroup answers a group membership row whose tier is
	// the one of Minimal access.
	reasonMinimalAccessOnAGroup = "The row's tier is the one of 5 (Minimal access), which GitLab grants a member only on a top-level group whose license includes the role (EE::GroupMember#access_level_inclusion, ee/app/models/ee/group_member.rb:85-96 at v19.5.0-ee), and every other level on every tier. The description says so beside the 5 it offers (issue 1280)."
	// reasonMinimalAccessOnAShare answers the group share's row, which a
	// group link rather than a member validates.
	reasonMinimalAccessOnAShare = "The row's tier is the one of 5 (Minimal access), which a group share takes only from a top-level group whose license includes the role (EE::GroupGroupLink#minimal_access_role_allowed, ee/app/models/ee/group_group_link.rb:20 and 32-36 at v19.5.0-ee), and every other level on every tier. The description says so beside the 5 it offers (issue 1280)."
	// reasonMinimalAccessNeverOnAProject answers the project half of a page
	// that documents a project and a group membership route in one table.
	reasonMinimalAccessNeverOnAProject = "The page documents the project and the group route in one table, and the value the row's tier gates, 5 (Minimal access), is one a project grants on no tier: ProjectMember#access_level_inclusion takes Gitlab::Access.all_values, Guest to Owner (app/models/members/project_member.rb:65-70 and lib/gitlab/access.rb:54-56 at v19.5.0-ee). The schema offers no 5 (issue 1280)."
	// reasonMembershipLockOnEveryTier answers the membership lock of a group
	// create and update.
	reasonMembershipLockOnEveryTier = "Enterprise Edition declares membership_lock on every tier (ee/lib/ee/api/helpers/groups_helpers.rb:25-26 at v19.5.0-ee), assigns it without a license check and enforces it on every tier (EE::Project#membership_locked?, ee/app/models/ee/project.rb:1168-1173, and EE::Members::CreateService#check_membership_lock!, ee/app/services/ee/members/create_service.rb:58-63). Only the settings page checks the member_lock license (ee/app/views/groups/_member_lock_setting.html.haml:1). Community Edition does not declare it (lib/api/helpers/groups_helpers.rb:54-55) and drops it, as it drops every Enterprise Edition parameter, which the tier model cannot tell from Free (issue 1240)."
	// reasonComputeMinutesOnEveryTier answers the two compute minute limits of
	// a group create and update.
	reasonComputeMinutesOnEveryTier = "An administrator sets it on every tier: Enterprise Edition declares it (ee/lib/ee/api/helpers/groups_helpers.rb:29-30 at v19.5.0-ee), the services drop it only for a caller who is not an administrator (ee/app/services/ee/groups/create_service.rb:64-68 and ee/app/services/ee/groups/update_service.rb:80-86), and the quota applies to a top-level group on every tier (Ci::Minutes::Quota#enabled?, ee/app/models/ci/minutes/quota.rb:14-16). Community Edition does not declare it (lib/api/helpers/groups_helpers.rb:54-55), as it declares no Enterprise Edition parameter (issue 1240)."
	// reasonDeletionDateFilterInCE answers the deletion date filter of the
	// group list.
	reasonDeletionDateFilterInCE = "Community Edition declares the filter (lib/api/groups.rb:37 at v19.5.0-ee, whose own description still says Premium and Ultimate), hands it to the finder (lib/api/groups.rb:70-76) and GroupsFinder applies it on every tier (app/finders/groups_finder.rb:123 and 158-162, Group.marked_for_deletion_on in app/models/group.rb:161-164) (issue 1240)."
	// reasonCustomRoleRemovedOnEveryTier answers the custom role of a project
	// and a group member edit.
	reasonCustomRoleRemovedOnEveryTier = "A role ID needs Ultimate: GitLab deletes a member_role_id that is not null where the top-level group's custom_roles feature is not available (EE::Members::UpdateService#handle_member_role_assignment and #member_role_param_allowed?, ee/app/services/ee/members/update_service.rb:88-115 at v19.5.0-ee, and EE::Namespace#custom_roles_enabled?, ee/app/models/ee/namespace.rb:839-841). Null passes on every tier and is assigned with the rest of the params (app/services/members/update_service.rb:51), removing a custom role a member kept from a licensed time, and this server sends 0 as that null. The description says a role ID needs Ultimate and 0 removes one on every tier (issue 1240)."
	// reasonDuoNeedsAPaidTierOrAnAddOn answers the two Duo settings of a group
	// update.
	reasonDuoNeedsAPaidTierOrAnAddOn = "GitLab stores the setting on every Enterprise Edition tier (ee/lib/ee/api/helpers/groups_helpers.rb:102-107 at v19.5.0-ee), and GitLab Duo, which it switches, answers where the group's licensed AI features are available: Premium carries ai_chat, Ultimate ai_features, and a Free namespace has them only with a GitLab Credits add-on (EE::Group#licensed_ai_features_available?, ee/app/models/ee/group.rb:368-372, and EE::Namespace#has_active_gitlab_credits_add_on? on the root ancestor, ee/app/models/ee/namespace.rb:336-348). The response carries the setting only there (ee/lib/ee/api/entities/group.rb:47-56). A tag would take it from a Free namespace with credits, so the description says so instead (issue 1240)."
)

// declaredParamAnswers holds the answer to every parameter finding that is
// not a tier tag, keyed by "action param".
//
// Each entry is held twice. [TestDeclaredParamAnswers_TheRealTable_HoldsAgainstTheServedSchemas]
// holds it to the schemas offline, so an entry a tag or a schema change has
// overtaken fails the unit suite; and a report that read all of the action's
// pages lists an entry that answered no finding as stale, which is the half that
// needs GitLab's pages and therefore gates nothing.
//
// The first seven entries are the Minimal access rows GitLab's pages gained
// after the 19.5 release ("On Premium and Ultimate, 5 (Minimal access) is also
// valid when id is a top-level group"), which the access level work of issue
// 1280 settled in the schemas: a group's description states the condition
// beside the 5 it offers, and a project's enum leaves the 5 out. The rest are
// the rows of issue 1240 that GitLab's source at v19.5.0-ee answers with
// something other than a tier tag, one block per page.
var declaredParamAnswers = map[string]paramDeclaration{
	"access.approve_group access_level":    {Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reasonMinimalAccessOnAGroup},
	"access.invite_group access_level":     {Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reasonMinimalAccessOnAGroup},
	"group.group_member_add access_level":  {Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reasonMinimalAccessOnAGroup},
	"group.group_member_edit access_level": {Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reasonMinimalAccessOnAGroup},
	"group.share_with_group group_access":  {Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reasonMinimalAccessOnAShare},
	"access.approve_project access_level":  {Documented: tierPremium, Category: categoryGatedValueNotOffered, Values: []int{5}, Reason: reasonMinimalAccessNeverOnAProject},
	"access.invite_project access_level":   {Documented: tierPremium, Category: categoryGatedValueNotOffered, Values: []int{5}, Reason: reasonMinimalAccessNeverOnAProject},

	"group.create membership_lock":                    {Documented: tierPremium, Category: categoryNotGated, Reason: reasonMembershipLockOnEveryTier},
	"group.update membership_lock":                    {Documented: tierPremium, Category: categoryNotGated, Reason: reasonMembershipLockOnEveryTier},
	"group.create shared_runners_minutes_limit":       {Documented: tierPremium, Category: categoryNotGated, Reason: reasonComputeMinutesOnEveryTier},
	"group.create extra_shared_runners_minutes_limit": {Documented: tierPremium, Category: categoryNotGated, Reason: reasonComputeMinutesOnEveryTier},
	"group.update shared_runners_minutes_limit":       {Documented: tierPremium, Category: categoryNotGated, Reason: reasonComputeMinutesOnEveryTier},
	"group.update extra_shared_runners_minutes_limit": {Documented: tierPremium, Category: categoryNotGated, Reason: reasonComputeMinutesOnEveryTier},
	"group.list marked_for_deletion_on":               {Documented: tierPremium, Category: categoryNotGated, Reason: reasonDeletionDateFilterInCE},
	"group.update duo_features_enabled":               {Documented: tierPremium, Category: categoryValueLevelGate, Reason: reasonDuoNeedsAPaidTierOrAnAddOn},
	"group.update lock_duo_features_enabled":          {Documented: tierPremium, Category: categoryValueLevelGate, Reason: reasonDuoNeedsAPaidTierOrAnAddOn},

	"group.group_member_edit member_role_id": {Documented: tierUltimate, Category: categoryValueLevelGate, Reason: reasonCustomRoleRemovedOnEveryTier},
	"project.member_edit member_role_id":     {Documented: tierUltimate, Category: categoryValueLevelGate, Reason: reasonCustomRoleRemovedOnEveryTier},
}

// answeredParamFinding is a finding a declaration answers, with the answer.
type answeredParamFinding struct {
	paramFinding
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// staleParamDeclaration is a declaration that answers nothing, and why.
type staleParamDeclaration struct {
	Declaration string `json:"declaration"`
	Reason      string `json:"reason"`
}

// tierCatalogs holds the catalog each tier is served, indexed by tier.
type tierCatalogs [3]*actioncatalog.Catalog

// property returns the input schema property param of action id in the
// catalog of tier at, or nil when that schema does not carry it.
func (c tierCatalogs) property(at tier, id, param string) map[string]any {
	action, _ := c[at].Action(actioncatalog.ActionID(id))
	properties, _ := action.Route.InputSchema["properties"].(map[string]any)
	property, _ := properties[param].(map[string]any)
	return property
}

// checkParamDeclarations splits declared into the declarations that hold
// against the schemas, which answer findings, and the stale ones, with why,
// which answer none. The stale ones come in no particular order; the report
// sorts them with the rest.
func checkParamDeclarations(declared map[string]paramDeclaration, offered map[string]map[string]tier, catalogs tierCatalogs) (answers map[string]paramDeclaration, stale []staleParamDeclaration) {
	answers = map[string]paramDeclaration{}
	for key, declaration := range declared {
		if problem := declaration.problem(key, offered, catalogs); problem != "" {
			stale = append(stale, staleParamDeclaration{Declaration: key, Reason: problem})
			continue
		}
		answers[key] = declaration
	}
	return answers, stale
}

// problem says why the declaration keyed "action param" answers nothing, read
// from the schemas alone, or returns "" when it holds. It needs no page, which
// is what lets the unit suite hold the table: a declaration gives a reason, its
// action offers the parameter at a tier below the one its row names (a tag
// that prunes it there answers the row instead), and it holds for its category
// against the schema the lowest tier offering the parameter is served.
func (d paramDeclaration) problem(key string, offered map[string]map[string]tier, catalogs tierCatalogs) string {
	if strings.TrimSpace(d.Reason) == "" {
		return "it gives no reason"
	}
	id, param, _ := strings.Cut(key, " ")
	at, ok := offered[id][param]
	if !ok {
		return "no tier's schema of " + id + " offers " + param
	}
	if at >= d.Documented {
		return at.String() + " is the lowest tier offering it, which the row's " + d.Documented.String() +
			" does not exceed, so the schema answers the row"
	}
	property := catalogs.property(at, id, param)
	switch d.Category {
	case categoryValueLevelGate:
		return valueLevelGateProblem(d, at, property)
	case categoryGatedValueNotOffered:
		return gatedValueProblem(d, at, property)
	case categoryNotGated:
		return ""
	default:
		return fmt.Sprintf("its category %q is none of %s, %s or %s", d.Category,
			categoryValueLevelGate, categoryGatedValueNotOffered, categoryNotGated)
	}
}

// valueLevelGateProblem holds a value-level gate to the schema of the
// parameter served at tier at. A parameter that enumerates its values, in an
// enum or in a description writing them as "5=Minimal access", has the gated
// ones named by the declaration, and each has to be one the enum offers and
// have the row's tier named beside it, from its "5=" to the next value the
// description writes: a tier named beside another value says nothing of this
// one. A gate on something other than a value has nothing to be named beside,
// and its description has to name the tier.
func valueLevelGateProblem(d paramDeclaration, at tier, property map[string]any) string {
	description, _ := property["description"].(string)
	enum, isEnum := property["enum"].([]any)
	if len(d.Values) == 0 {
		if isEnum || writtenValue.MatchString(description) {
			return fmt.Sprintf("its parameter enumerates its values at %s, and it names none of them as the ones the row's tier gates", at)
		}
		if strings.Contains(strings.ToLower(description), d.Documented.String()) {
			return ""
		}
		return fmt.Sprintf("its description at %s, %q, does not name %s, so it does not say which values need it", at, description, d.Documented)
	}
	for _, value := range d.Values {
		if isEnum && !enumOffers(enum, value) {
			return fmt.Sprintf("its enum at %s does not offer %d, so the row's tier gates a value the action never offers", at, value)
		}
		if !strings.Contains(strings.ToLower(valueSegment(description, value)), d.Documented.String()) {
			return fmt.Sprintf("its description at %s, %q, does not name %s beside %d=, so it does not say that %d needs it",
				at, description, d.Documented, value, value)
		}
	}
	return ""
}

// writtenValue matches a value a description writes the way the membership
// levels are written, "5=Minimal access". The digits are read whole, so the
// 15 of "15=Planner" is never read as a 5.
var writtenValue = regexp.MustCompile(`(\d+)=`)

// valueSegment returns the part of description that writes value, from its
// "<value>=" to the next value the description writes or to its end, or ""
// when it writes no such value. A value written twice is read where it is
// written first.
func valueSegment(description string, value int) string {
	written := writtenValue.FindAllStringSubmatchIndex(description, -1)
	for i, match := range written {
		if description[match[2]:match[3]] != strconv.Itoa(value) {
			continue
		}
		end := len(description)
		if i+1 < len(written) {
			end = written[i+1][0]
		}
		return description[match[0]:end]
	}
	return ""
}

// enumOffers reports whether enum holds value. An enum built in Go holds ints
// and one decoded from JSON float64s, and both print as the integer.
func enumOffers(enum []any, value int) bool {
	return slices.ContainsFunc(enum, func(v any) bool { return fmt.Sprint(v) == strconv.Itoa(value) })
}

// gatedValueProblem holds a gated value the action does not offer to an enum,
// served at tier at, that leaves out every value the declaration names.
func gatedValueProblem(d paramDeclaration, at tier, property map[string]any) string {
	enum, isEnum := property["enum"].([]any)
	if !isEnum {
		return fmt.Sprintf("it publishes no enum at %s, so nothing holds the gated values out of it", at)
	}
	if len(d.Values) == 0 {
		return "it names no gated value for the enum to leave out"
	}
	for _, value := range d.Values {
		if enumOffers(enum, value) {
			return fmt.Sprintf("its enum at %s offers %d, the value the declaration says it leaves out", at, value)
		}
	}
	return ""
}

// answerParams splits findings into the ones no declaration answers and the
// ones the declaration of their action and parameter answers, for the tier
// the row names, keeping the order they came in.
func answerParams(findings []paramFinding, answers map[string]paramDeclaration) (open []paramFinding, answered []answeredParamFinding) {
	for _, finding := range findings {
		declaration, declared := answers[finding.Action+" "+finding.Param]
		if declared && declaration.Documented.String() == finding.Documented {
			answered = append(answered, answeredParamFinding{paramFinding: finding, Category: declaration.Category, Reason: declaration.Reason})
			continue
		}
		open = append(open, finding)
	}
	return open, answered
}

// unansweredDeclarations returns each declaration that answered no finding
// although every page its action is graded against was read, which is the
// half of the stale rule that needs GitLab's pages. An action with a page
// that could not be read, its owner page or the override page beside it, may
// have the row the declaration answers on that page, and is not judged.
func unansweredDeclarations(answers map[string]paramDeclaration, graded, answered map[string]bool) []staleParamDeclaration {
	var stale []staleParamDeclaration
	for key, declaration := range answers {
		id, _, _ := strings.Cut(key, " ")
		if graded[id] && !answered[key] {
			stale = append(stale, staleParamDeclaration{
				Declaration: key,
				Reason:      "it answers no finding: no page read marks it " + declaration.Documented.String() + " on a route the action sends",
			})
		}
	}
	return stale
}
