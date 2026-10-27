// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// describedAction is one hand-built action whose input schema carries the
// given properties as they are written, description and enum included.
func describedAction(domain, name, owner string, properties map[string]any) actioncatalog.Action {
	return actioncatalog.Action{
		Name: name, Domain: domain, OwnerPackage: owner,
		Route: toolutil.ActionRoute{InputSchema: map[string]any{"type": "object", "properties": properties}},
	}
}

// approvalLevelBefore1280 is the description the group approval's access_level
// carried below the access level work of issue 1280: it names Premium beside
// Planner and Security Manager, which that work found false, and nothing
// beside the 5 the row's tier gates.
const approvalLevelBefore1280 = "Access level to grant (0=No access, 5=Minimal access, 10=Guest, 15=Planner (Premium), 20=Reporter, " +
	"25=Security Manager (Premium), 30=Developer, 40=Maintainer, 50=Owner). Default 30"

// memberProperties is the input schema of the hand-built member add the
// declaration rules are read against, with whatever extra properties a tier
// adds. Its access_level is a value-level gate whose description names
// Premium beside the value it gates, free_level is one that publishes no enum,
// later_level writes the gated 5 after another value and names Premium beside
// it, planner_first names Premium only beside the 15 written before the 5,
// old_level carries [approvalLevelBefore1280], assignees gates how many
// rather than which values and says so, level_plain offers every level and
// says nothing about a tier, project_level publishes an enum without the
// gated value, loose_level publishes no enum, and weight is a plain
// parameter.
func memberProperties(extra map[string]any) map[string]any {
	properties := map[string]any{
		"access_level":  map[string]any{"type": "integer", "description": "Level (5=Minimal access, with a Premium license; 10=Guest)", "enum": []any{5, 10}},
		"free_level":    map[string]any{"type": "integer", "description": "Level (5=Minimal access, Premium only; 10=Guest)"},
		"later_level":   map[string]any{"type": "integer", "description": "Level (10=Guest, 5=Minimal access, Premium only)"},
		"planner_first": map[string]any{"type": "integer", "description": "Level (15=Planner (Premium), 5=Minimal access, 10=Guest)"},
		"old_level":     map[string]any{"type": "integer", "description": approvalLevelBefore1280},
		"assignees":     map[string]any{"type": "array", "description": "Assignee IDs; more than one needs Premium"},
		"level_plain":   map[string]any{"type": "integer", "description": "Level, any of them"},
		"project_level": map[string]any{"type": "integer", "description": "Level", "enum": []any{float64(10), float64(20)}},
		"loose_level":   map[string]any{"type": "integer", "description": "Level"},
		"weight":        map[string]any{"type": "integer"},
	}
	maps.Copy(properties, extra)
	return properties
}

// memberCatalogs is one catalog per tier over the hand-built member add: the
// Premium schema adds health and the Ultimate one iteration, and the
// Ultimate schema describes level_plain as Premium, so a rule reading any
// schema but the lowest one offering the parameter would pass a description
// the lowest tier is never served.
func memberCatalogs(t *testing.T) (tierCatalogs, map[string]map[string]tier) {
	t.Helper()
	health := map[string]any{"health": map[string]any{"type": "string"}}
	free := schemaCatalog(t, "group", describedAction("group", "member_add", "groupmembers", memberProperties(nil)))
	premium := schemaCatalog(t, "group", describedAction("group", "member_add", "groupmembers", memberProperties(health)))
	ultimate := schemaCatalog(t, "group", describedAction("group", "member_add", "groupmembers", memberProperties(map[string]any{
		"health":      map[string]any{"type": "string"},
		"iteration":   map[string]any{"type": "string"},
		"level_plain": map[string]any{"type": "integer", "description": "Level; 5 needs a Premium license"},
	})))
	return tierCatalogs{free, premium, ultimate}, offeredTiers(free, premium, ultimate)
}

// TestParamDeclarationProblem_EachRule_IsReadFromTheSchemasAlone holds the
// offline half of the rule a declaration answers to, case by case, which is
// the one hard check the answers table has (the maintainer's answer to the
// question of a networked gate, issue 1240): a declaration gives a reason, its
// action offers the parameter below the tier the row names, and its category
// is one of the known ones and holds against the schema the lowest tier is
// served. A value-level gate on the values of a parameter that enumerates
// them is held to a description naming the tier beside each gated value, one
// on anything else to a description naming the tier, and a gated value the
// action does not offer to an enum that leaves it out.
func TestParamDeclarationProblem_EachRule_IsReadFromTheSchemasAlone(t *testing.T) {
	catalogs, offered := memberCatalogs(t)
	const reason = "the source line that settles it"
	cases := []struct {
		name string
		key  string
		decl paramDeclaration
		want string
	}{
		{
			name: "value_level_gate_holds", key: "group.member_add access_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
		},
		{
			name: "value_level_gate_holds_without_an_enum", key: "group.member_add free_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
		},
		{
			name: "value_level_gate_holds_for_a_value_written_later", key: "group.member_add later_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
		},
		{
			name: "value_level_gate_of_a_quantity_holds", key: "group.member_add assignees",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Reason: reason},
		},
		{
			name: "value_level_gate_silent_description", key: "group.member_add level_plain",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Reason: reason},
			want: `its description at free, "Level, any of them", does not name premium, so it does not say which values need it`,
		},
		{
			name: "value_level_gate_names_another_tier", key: "group.member_add access_level",
			decl: paramDeclaration{Documented: tierUltimate, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
			want: `its description at free, "Level (5=Minimal access, with a Premium license; 10=Guest)", does not name ultimate beside 5=, so it does not say that 5 needs it`,
		},
		{
			name: "value_level_gate_tier_beside_other_values", key: "group.member_add old_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
			want: "its description at free, " + strconv.Quote(approvalLevelBefore1280) + ", does not name premium beside 5=, so it does not say that 5 needs it",
		},
		{
			name: "value_level_gate_value_inside_another", key: "group.member_add planner_first",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
			want: `its description at free, "Level (15=Planner (Premium), 5=Minimal access, 10=Guest)", does not name premium beside 5=, so it does not say that 5 needs it`,
		},
		{
			name: "value_level_gate_second_value_silent", key: "group.member_add access_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5, 10}, Reason: reason},
			want: `its description at free, "Level (5=Minimal access, with a Premium license; 10=Guest)", does not name premium beside 10=, so it does not say that 10 needs it`,
		},
		{
			name: "value_level_gate_value_not_written", key: "group.member_add free_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{20}, Reason: reason},
			want: `its description at free, "Level (5=Minimal access, Premium only; 10=Guest)", does not name premium beside 20=, so it does not say that 20 needs it`,
		},
		{
			name: "value_level_gate_value_not_offered", key: "group.member_add project_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Values: []int{5}, Reason: reason},
			want: "its enum at free does not offer 5, so the row's tier gates a value the action never offers",
		},
		{
			name: "value_level_gate_enum_names_no_value", key: "group.member_add project_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Reason: reason},
			want: "its parameter enumerates its values at free, and it names none of them as the ones the row's tier gates",
		},
		{
			name: "value_level_gate_description_enumerates_no_value_named", key: "group.member_add old_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryValueLevelGate, Reason: reason},
			want: "its parameter enumerates its values at free, and it names none of them as the ones the row's tier gates",
		},
		{
			name: "gated_value_left_out", key: "group.member_add project_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryGatedValueNotOffered, Values: []int{5}, Reason: reason},
		},
		{
			name: "gated_value_offered", key: "group.member_add access_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryGatedValueNotOffered, Values: []int{5}, Reason: reason},
			want: "its enum at free offers 5, the value the declaration says it leaves out",
		},
		{
			name: "second_gated_value_offered", key: "group.member_add project_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryGatedValueNotOffered, Values: []int{5, 20}, Reason: reason},
			want: "its enum at free offers 20, the value the declaration says it leaves out",
		},
		{
			name: "gated_value_without_an_enum", key: "group.member_add loose_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryGatedValueNotOffered, Values: []int{5}, Reason: reason},
			want: "it publishes no enum at free, so nothing holds the gated values out of it",
		},
		{
			name: "gated_value_unnamed", key: "group.member_add project_level",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryGatedValueNotOffered, Reason: reason},
			want: "it names no gated value for the enum to leave out",
		},
		{
			name: "not_gated_holds", key: "group.member_add weight",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: reason},
		},
		{
			name: "unknown_category", key: "group.member_add weight",
			decl: paramDeclaration{Documented: tierPremium, Category: "tagged", Reason: reason},
			want: `its category "tagged" is none of value-level-gate, gated-value-not-offered or not-gated`,
		},
		{
			name: "no_reason", key: "group.member_add weight",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: " \t"},
			want: "it gives no reason",
		},
		{
			name: "unknown_action", key: "group.ghost weight",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: reason},
			want: "no tier's schema of group.ghost offers weight",
		},
		{
			name: "unknown_parameter", key: "group.member_add ghost",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: reason},
			want: "no tier's schema of group.member_add offers ghost",
		},
		{
			name: "key_without_a_parameter", key: "group.member_add",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: reason},
			want: "no tier's schema of group.member_add offers ",
		},
		{
			name: "tagged_at_the_row_tier", key: "group.member_add health",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: reason},
			want: "premium is the lowest tier offering it, which the row's premium does not exceed, so the schema answers the row",
		},
		{
			name: "tagged_above_the_row_tier", key: "group.member_add iteration",
			decl: paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: reason},
			want: "ultimate is the lowest tier offering it, which the row's premium does not exceed, so the schema answers the row",
		},
		{
			name: "offered_below_an_ultimate_row", key: "group.member_add health",
			decl: paramDeclaration{Documented: tierUltimate, Category: categoryNotGated, Reason: reason},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.decl.problem(tc.key, offered, catalogs); got != tc.want {
				t.Errorf("problem(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// TestCheckParamDeclarations_Table_SplitsTheOnesThatHoldFromTheStale holds the
// split the report is built on: a declaration that holds answers findings,
// and one that does not is reported stale with its problem and answers
// nothing, so a reason that stopped describing the schema never hides a
// finding. Nothing declared is neither.
func TestCheckParamDeclarations_Table_SplitsTheOnesThatHoldFromTheStale(t *testing.T) {
	catalogs, offered := memberCatalogs(t)
	holds := paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: "it holds"}
	answers, stale := checkParamDeclarations(map[string]paramDeclaration{
		"group.member_add weight": holds,
		"group.member_add health": holds,
	}, offered, catalogs)
	if want := map[string]paramDeclaration{"group.member_add weight": holds}; !reflect.DeepEqual(answers, want) {
		t.Errorf("answers = %+v, want %+v", answers, want)
	}
	wantStale := []staleParamDeclaration{{
		Declaration: "group.member_add health",
		Reason:      "premium is the lowest tier offering it, which the row's premium does not exceed, so the schema answers the row",
	}}
	if !reflect.DeepEqual(stale, wantStale) {
		t.Errorf("stale = %+v, want %+v", stale, wantStale)
	}

	answers, stale = checkParamDeclarations(nil, offered, catalogs)
	if len(answers) != 0 || stale != nil {
		t.Errorf("checkParamDeclarations(nil) = %+v, %+v; want nothing answered and nothing stale", answers, stale)
	}
}

// TestAnswerParams_Findings_AreAnsweredOnlyByADeclarationOfTheirTier holds the
// join of a finding to its answer: the declaration of its action and
// parameter answers it when it names the tier the row does, carrying its
// category and reason, and a finding whose row names another tier, or that
// nothing declares, stays open, in the order the findings came.
func TestAnswerParams_Findings_AreAnsweredOnlyByADeclarationOfTheirTier(t *testing.T) {
	const route, doc = "PUT /projects/:id/boards/:board_id", "doc/api/boards.md"
	labels := paramFinding{Action: "project.board_update", Route: route, Param: "labels", Documented: "premium", OfferedAt: "free", Doc: doc}
	weight := paramFinding{Action: "project.board_update", Route: route, Param: "weight", Documented: "ultimate", OfferedAt: "free", Doc: doc}
	name := paramFinding{Action: "project.board_update", Route: route, Param: "name", Documented: "premium", OfferedAt: "free", Doc: doc}
	again := labels
	again.Route = "POST /projects/:id/boards"
	answers := map[string]paramDeclaration{
		"project.board_update labels": {Documented: tierPremium, Category: categoryNotGated, Reason: "labels reason"},
		"project.board_update weight": {Documented: tierPremium, Category: categoryNotGated, Reason: "weight reason"},
	}
	open, answered := answerParams([]paramFinding{labels, weight, name, again}, answers)
	if want := []paramFinding{weight, name}; !reflect.DeepEqual(open, want) {
		t.Errorf("open = %+v, want %+v", open, want)
	}
	want := []answeredParamFinding{
		{paramFinding: labels, Category: categoryNotGated, Reason: "labels reason"},
		{paramFinding: again, Category: categoryNotGated, Reason: "labels reason"},
	}
	if !reflect.DeepEqual(answered, want) {
		t.Errorf("answered = %+v, want %+v", answered, want)
	}
}

// declaredBoardsFixture is the boards fixture with a third domain whose page
// is never read: an issue update owned by the issues package, which no page
// in the cache documents. The boards domain also holds a group hook add,
// which is graded against the group webhooks page as well as the boards one,
// and that page is not in the cache either. Its catalogs describe the
// update's labels as a Premium value-level gate and leave its weight
// undescribed.
func declaredBoardsFixture(t *testing.T) *docResolver {
	t.Helper()
	update := func(props ...string) actioncatalog.Action {
		properties := map[string]any{"labels": map[string]any{"type": "string", "description": "Labels; more than one needs Premium"}}
		for _, prop := range append([]string{"project_id", "board_id", "name", "weight"}, props...) {
			properties[prop] = map[string]any{"type": "string"}
		}
		return describedAction("project", "board_update", "boards", properties)
	}
	listCreate := func(props ...string) actioncatalog.Action {
		return schemaAction("project", "board_list_create", "boards", append([]string{"label_id"}, props...)...)
	}
	groupUpdate := schemaAction("project", "group_board_update", "groupboards", "name", "weight")
	issueUpdate := schemaAction("project", "issue_update", "issues", "title")
	hookAdd := schemaAction("group", "hook_add", "boards", "url")
	stubTierCatalogs(t,
		schemaCatalog(t, "project", update(), listCreate(), groupUpdate, issueUpdate, hookAdd),
		schemaCatalog(t, "project", update("assignee_id"), listCreate("iteration_id"), groupUpdate, issueUpdate, hookAdd),
		schemaCatalog(t, "project", update("assignee_id"), listCreate("iteration_id"), groupUpdate, issueUpdate, hookAdd),
	)
	dir := t.TempDir()
	seedDoc(t, dir, "boards", boardsPage)
	seedDoc(t, dir, "group_boards", freeBadgeDoc)
	res := newOfflineResolver(t, dir)
	res.routes = map[string][]string{
		"project.board_update":       {"PUT /projects/:id/boards/:board_id"},
		"project.board_list_create":  {"POST /projects/:id/boards/:board_id/lists"},
		"project.group_board_update": {"PUT /groups/:id/boards/:board_id"},
		"project.issue_update":       {"PUT /projects/:id/issues/:issue_iid"},
		"group.hook_add":             {"POST /groups/:id/hooks"},
	}
	return res
}

// TestBuildReport_DeclaredParams_AreAnsweredAndTheStaleOnesListed is the
// answers table end to end (issue 1240): a declaration that holds moves its
// finding out of param_findings into param_findings_answered with its
// category and reason; a declaration naming another tier than the row leaves
// the finding open and is stale for answering nothing, as is one whose action's
// page was read and marks nothing, and one whose description says nothing of
// the tier is stale offline and answers nothing either; a declaration whose
// action's pages could not all be read is not judged for answering nothing,
// since a row it answers may be on the one that was not, whether that is the
// owner page or the override page its action is graded against beside it. The
// summary counts each list and the report lists the stale ones in key order.
func TestBuildReport_DeclaredParams_AreAnsweredAndTheStaleOnesListed(t *testing.T) {
	res := declaredBoardsFixture(t)
	notGated := paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: "GitLab serves it on Free"}
	res.declared = map[string]paramDeclaration{
		"project.board_update labels":            {Documented: tierPremium, Category: categoryValueLevelGate, Reason: "only more than one label needs Premium"},
		"project.board_update weight":            {Documented: tierUltimate, Category: categoryNotGated, Reason: "written for an Ultimate row"},
		"project.board_list_create iteration_id": {Documented: tierUltimate, Category: categoryValueLevelGate, Reason: "undescribed"},
		"project.group_board_update weight":      notGated,
		"project.issue_update title":             notGated,
		"group.hook_add url":                     notGated,
	}
	rep, err := buildReport(context.Background(), res)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	boards := findDomain(t, rep, "boards")
	const update, doc = "PUT /projects/:id/boards/:board_id", "doc/api/boards.md"
	wantOpen := []paramFinding{
		{
			Action: "project.board_list_create", Route: "POST /projects/:id/boards/:board_id/lists", Param: "iteration_id",
			Documented: "ultimate", OfferedAt: "premium", Doc: doc,
		},
		{Action: "project.board_update", Route: update, Param: "weight", Documented: "premium", OfferedAt: "free", Doc: doc},
	}
	if !reflect.DeepEqual(boards.ParamFindings, wantOpen) {
		t.Errorf("boards open findings =\n %+v\nwant\n %+v", boards.ParamFindings, wantOpen)
	}
	wantAnswered := []answeredParamFinding{{
		Action: "project.board_update", Route: update, Param: "labels", Documented: "premium", OfferedAt: "free", Doc: doc,
		Category: categoryValueLevelGate, Reason: "only more than one label needs Premium",
	}}
	if !reflect.DeepEqual(boards.ParamFindingsAnswered, wantAnswered) {
		t.Errorf("boards answered =\n %+v\nwant\n %+v", boards.ParamFindingsAnswered, wantAnswered)
	}
	if !boards.NeedsWork {
		t.Error("boards needs no work, want it to for its two open findings")
	}
	const answersNothing = "it answers no finding: no page read marks it premium on a route the action sends"
	wantStale := []staleParamDeclaration{
		{
			Declaration: "project.board_list_create iteration_id",
			Reason:      `its description at premium, "", does not name ultimate, so it does not say which values need it`,
		},
		{Declaration: "project.board_update weight", Reason: "it answers no finding: no page read marks it ultimate on a route the action sends"},
		{Declaration: "project.group_board_update weight", Reason: answersNothing},
	}
	if !reflect.DeepEqual(rep.StaleParamDeclarations, wantStale) {
		t.Errorf("stale =\n %+v\nwant\n %+v", rep.StaleParamDeclarations, wantStale)
	}
	s := rep.Summary
	if s.ParamFindings != 2 || s.ParamFindingsAnswered != 1 || s.StaleParamDeclarations != 3 || s.ParamRowsJoined != 4 {
		t.Errorf("summary = %d open, %d answered, %d stale over %d rows; want 2, 1 and 3 over 4",
			s.ParamFindings, s.ParamFindingsAnswered, s.StaleParamDeclarations, s.ParamRowsJoined)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	for _, tc := range []struct{ name, fragment string }{
		{
			name: "answered_finding_with_its_answer",
			fragment: `"param_findings_answered":[{"action":"project.board_update","route":"PUT /projects/:id/boards/:board_id","param":"labels",` +
				`"documented":"premium","offered_at":"free","doc":"doc/api/boards.md","category":"value-level-gate","reason":"only more than one label needs Premium"}]`,
		},
		{name: "stale_declarations_at_the_top", fragment: `{"declaration":"project.group_board_update weight","reason":"` + answersNothing + `"}]`},
		{name: "summary_counts", fragment: `"param_findings":2,"param_findings_answered":1,"stale_param_declarations":3}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(string(data), tc.fragment) {
				t.Errorf("report JSON lacks %s\nin %s", tc.fragment, data)
			}
		})
	}
}

// TestBuildReport_EveryFindingAnswered_TheDomainNeedsNoWork holds that an
// answered finding is no gap: a green page whose every finding a declaration
// answers needs no work, so -gaps-only drops it, and the report lists nothing
// stale and no open finding.
func TestBuildReport_EveryFindingAnswered_TheDomainNeedsNoWork(t *testing.T) {
	res := boardsFixture(t)
	notGated := paramDeclaration{Documented: tierPremium, Category: categoryNotGated, Reason: "GitLab serves it on Free"}
	res.declared = map[string]paramDeclaration{
		"project.board_update labels":            notGated,
		"project.board_update weight":            notGated,
		"project.board_list_create iteration_id": {Documented: tierUltimate, Category: categoryNotGated, Reason: "Premium is served it"},
	}
	rep, err := buildReport(context.Background(), res)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	boards := findDomain(t, rep, "boards")
	if boards.NeedsWork || boards.ParamFindings != nil || len(boards.ParamFindingsAnswered) != 3 {
		t.Errorf("boards = needs work %v, open %+v, %d answered; want no work, nothing open and three answered",
			boards.NeedsWork, boards.ParamFindings, len(boards.ParamFindingsAnswered))
	}
	if rep.StaleParamDeclarations != nil || rep.Summary.StaleParamDeclarations != 0 || rep.Summary.ParamFindings != 0 {
		t.Errorf("report = stale %+v (%d), %d open; want neither", rep.StaleParamDeclarations,
			rep.Summary.StaleParamDeclarations, rep.Summary.ParamFindings)
	}
}

// realTierCatalogs builds the three catalogs the report reads, the way
// buildReport builds them.
func realTierCatalogs(t *testing.T) tierCatalogs {
	t.Helper()
	var catalogs tierCatalogs
	for level, opts := range []tools.ActionCatalogOptions{
		{Enterprise: false, IncludeMCP: true},
		{Tier: edition.Premium, IncludeMCP: true},
		{Enterprise: true, IncludeMCP: true},
	} {
		catalog, err := tools.BuildActionCatalog(nil, opts)
		if err != nil {
			t.Fatalf("BuildActionCatalog(%+v): %v", opts, err)
		}
		catalogs[level] = catalog
	}
	return catalogs
}

// TestDeclaredParamAnswers_TheRealTable_HoldsAgainstTheServedSchemas is the
// gate the answers table has, and it needs no network: every declaration in
// it names an action and a parameter the schemas offer below the tier its row
// names, gives a reason, and holds for its category against the schema the
// lowest tier is served. A declaration a tier tag or a schema change has
// overtaken fails here rather than answering a finding it no longer
// describes.
func TestDeclaredParamAnswers_TheRealTable_HoldsAgainstTheServedSchemas(t *testing.T) {
	catalogs := realTierCatalogs(t)
	answers, stale := checkParamDeclarations(declaredParamAnswers, offeredTiers(catalogs[tierFree], catalogs[tierPremium], catalogs[tierUltimate]), catalogs)
	for _, s := range stale {
		t.Errorf("declaredParamAnswers[%q] is stale: %s", s.Declaration, s.Reason)
	}
	if len(answers) != len(declaredParamAnswers) {
		t.Errorf("%d of %d declarations hold", len(answers), len(declaredParamAnswers))
	}
}

// approvalPage is the section of doc/api/access_requests.md that documents
// both approvals in one table, with the row GitLab's page carries on master
// since the 19.5 release: the value 5 is valid on a top-level group with a
// Premium or Ultimate license.
const approvalPage = "## Approve an access request\n\n" +
	"```plaintext\nPUT /groups/:id/access_requests/:user_id/approve\nPUT /projects/:id/access_requests/:user_id/approve\n```\n\n" +
	"| Attribute | Type | Required | Description |\n| --- | --- | --- | --- |\n" +
	"| `access_level` | integer | no | Possible values: `10` (Guest) to `50` (Owner). On Premium and Ultimate, `5` (Minimal access) " +
	"is also valid when `id` is a top-level group. Default: `30`. |\n"

// TestRunMain_RealTable_AnswersTheApprovalsMinimalAccessRow drives the entry
// point over a module root whose record names both approvals and whose cache
// holds their section, so the table runMain hands the report is the real one:
// the group approval's row is answered as a value-level gate its description
// states, the project approval's as a gated value its schema leaves out, and
// no declaration of the real table is stale.
func TestRunMain_RealTable_AnswersTheApprovalsMinimalAccessRow(t *testing.T) {
	root := fakeModuleRoot(t)
	seedRecord(t, root,
		actionrequests.RecordAction{ID: "access.approve_group", Requests: []actionrequests.RecordRequest{
			{Kind: actionrequests.KindREST, Route: "PUT /groups/:id/access_requests/:user_id/approve"},
		}},
		actionrequests.RecordAction{ID: "access.approve_project", Requests: []actionrequests.RecordRequest{
			{Kind: actionrequests.KindREST, Route: "PUT /projects/:id/access_requests/:user_id/approve"},
		}},
	)
	seedDoc(t, apidocs.CacheDir(root), "access_requests", approvalPage)
	t.Chdir(root)

	rep := runReport(t, "-offline")
	access := findDomain(t, rep, "accessrequests")
	if access.ParamFindings != nil || access.NeedsWork {
		t.Errorf("accessrequests = open %+v, needs work %v; want both rows answered", access.ParamFindings, access.NeedsWork)
	}
	got := map[string]string{}
	for _, answered := range access.ParamFindingsAnswered {
		got[answered.Action+" "+answered.Param] = answered.Category
	}
	want := map[string]string{
		"access.approve_group access_level":   categoryValueLevelGate,
		"access.approve_project access_level": categoryGatedValueNotOffered,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("answered = %v, want %v", got, want)
	}
	if rep.StaleParamDeclarations != nil {
		t.Errorf("stale = %+v, want none of the real table", rep.StaleParamDeclarations)
	}
}

// requestTable opens a request table the way GitLab's API pages write one,
// with the Required column that tells it from a response table.
const requestTable = "| Attribute | Type | Required | Description |\n| --- | --- | --- | --- |\n"

// endpointSection is one endpoint section of an API page: its heading, the
// code block spelling its endpoint and a request table holding rows.
func endpointSection(heading, endpoint string, rows ...string) string {
	return "## " + heading + "\n\n```plaintext\n" + endpoint + "\n```\n\n" + requestTable + strings.Join(rows, "\n") + "\n\n"
}

// restRecord is the record of an action that sends the REST routes given.
func restRecord(id string, routes ...string) actionrequests.RecordAction {
	requests := make([]actionrequests.RecordRequest, len(routes))
	for i, route := range routes {
		requests[i] = actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: route}
	}
	return actionrequests.RecordAction{ID: id, Requests: requests}
}

// The rows of doc/api/groups.md that issue 1240 lists, as GitLab's page
// carries them on master on 2026-10-27.
const (
	rowMembershipLock      = "| `membership_lock` | boolean | no | Users cannot be added to projects in this group. Premium and Ultimate only. |"
	rowExtraMinutesLimit   = "| `extra_shared_runners_minutes_limit` | integer | no | Can be set by administrators only. Additional compute minutes for this group. GitLab Self-Managed, Premium and Ultimate only. |"
	rowMinutesLimit        = "| `shared_runners_minutes_limit` | integer | no | Can be set by administrators only. Maximum number of monthly compute minutes for this group. Can be `nil` (default; inherit system default), `0` (unlimited), or `> 0`. GitLab Self-Managed, Premium and Ultimate only. |"
	rowWikiAccessLevel     = "| `wiki_access_level` | string | no | The wiki access level. Can be `disabled`, `private`, or `enabled`. Premium and Ultimate only. |"
	rowFileTemplateProject = "| `file_template_project_id` | integer | no | The ID of a project to load custom file templates from. Premium and Ultimate only. |"
	rowPreventForking      = "| `prevent_forking_outside_group` | boolean | no | When enabled, users can not fork projects from this group to external namespaces. Premium and Ultimate only. |"
	rowIPRestriction       = "| `ip_restriction_ranges` | string | no | Comma-separated list of IP addresses or subnet masks to restrict group access. Premium and Ultimate only. |"
	rowEmailDomains        = "| `allowed_email_domains_list` | string | no | Comma-separated list of email address domains to allow group access. GitLab Premium and Ultimate only. |"
	rowDuoFeatures         = "| `duo_features_enabled` | boolean | no | Indicates whether GitLab Duo features are enabled for this group. GitLab Self-Managed, Premium and Ultimate only. |"
	rowLockDuoFeatures     = "| `lock_duo_features_enabled` | boolean | no | Indicates whether the GitLab Duo features enabled setting is enforced for all subgroups. GitLab Self-Managed, Premium and Ultimate only. |"
	rowPipelineSucceeds    = "| `only_allow_merge_if_pipeline_succeeds` | boolean | no | Only allow merging merge requests if the pipeline succeeds. When enabled for a group, applies to all projects in the group. Premium and Ultimate only. |"
	rowSkippedPipeline     = "| `allow_merge_on_skipped_pipeline` | boolean | no | Allow merging merge requests when the pipeline is skipped. Only applies when `only_allow_merge_if_pipeline_succeeds` is `true`. Premium and Ultimate only. |"
	rowDiscussionsResolved = "| `only_allow_merge_if_all_discussions_are_resolved` | boolean | no | Only allow merging merge requests when all discussions are resolved. When enabled for a group, applies to all projects in the group. Premium and Ultimate only. |"
	rowRepositoryStorage   = "| `repository_storage` | string | no | Filter by repository storage used by the group (administrators only). Premium and Ultimate only. |"
	rowMarkedForDeletionOn = "| `marked_for_deletion_on` | date | no | Filter by date when group was marked for deletion. Introduced in GitLab 17.1. Premium and Ultimate only. |"
	rowSecurityReports     = "| `with_security_reports` | boolean | no | Return only projects that have security reports artifacts present in any of their builds. Default is `false`. Ultimate only. |"
	rowShareGroupAccess    = "| `group_access` | integer | yes | The default `access_level` to assign to the invited group. Possible values: `10` (Guest) to `50` (Owner). On Premium and Ultimate, `5` (Minimal access) is also valid when `id` is a top-level group. |"
)

// The custom role rows of doc/api/group_members.md, project_members.md and
// group_ldap_links.md, and the Minimal access row the group member routes
// carry beside them, as GitLab's pages carry them on master on 2026-10-27.
const (
	rowMemberAccessLevel   = "| `access_level` | integer | yes | A valid access level. Possible values: `10` (Guest) to `50` (Owner). On Premium and Ultimate, `5` (Minimal access) is also valid when `id` is a top-level group. |"
	rowMemberRoleAdd       = "| `member_role_id` | integer | no | Ultimate only. The ID of a custom member role. |"
	rowMemberRoleEdit      = "| `member_role_id` | integer | no | Ultimate only. The ID of a custom member role. If no value is specified, removes all roles. |"
	rowLDAPLinkMemberRole  = "| `member_role_id` | integer | no | The ID of the member role. Ultimate only. |"
	routeGroupMemberAdd    = "POST /groups/:id/members"
	routeGroupMemberEdit   = "PUT /groups/:id/members/:user_id"
	routeProjectMemberAdd  = "POST /projects/:id/members"
	routeProjectMemberEdit = "PUT /projects/:id/members/:user_id"
)

// The rows of doc/api/group_level_variables.md, as GitLab's page carries them
// on master on 2026-10-27, and the routes they document.
const (
	rowVariableFilter        = "| `filter` | hash | No | Filters results when multiple variables share the same key. Possible values: `[environment_scope]`. Premium and Ultimate only. |"
	rowVariableScope         = "| `environment_scope` | string | No | The environment scope of a variable. Premium and Ultimate only. |"
	routeGroupVariables      = "POST /groups/:id/variables"
	routeGroupVariable       = "/groups/:id/variables/:key"
	routeGroupVariableGet    = "GET " + routeGroupVariable
	routeGroupVariableUpdate = "PUT " + routeGroupVariable
	routeGroupVariableDelete = "DELETE " + routeGroupVariable
)

// The assignee row of doc/api/issues.md, as GitLab's page carries it on master
// on 2026-10-27, and the route it documents.
const (
	rowIssueAssigneeIDs = "| `assignee_ids` | integer array | No | The IDs of the users to assign the issue to. Premium and Ultimate only.|"
	routeIssueCreate    = "POST /projects/:id/issues"
)

// The approver row of doc/api/merge_requests.md, which the page writes the
// same way for its three lists, as GitLab's page carries it on master on
// 2026-10-27, and the routes it documents.
const (
	rowMergeRequestApproverIDs = "| `approver_ids[]` | integer array | No | Returns merge requests which have all users with the specified `id` as eligible approvers according to the approval rules. `None` returns merge requests with no eligible approvers. `Any` returns merge requests with at least one eligible approver. Premium and Ultimate only. |"
	routeMergeRequestsGlobal   = "GET /merge_requests"
	routeMergeRequestsProject  = "GET /projects/:id/merge_requests"
	routeMergeRequestsGroup    = "GET /groups/:id/merge_requests"
)

// TestRunMain_RealTable_AnswersTheRowsGitLabsSourceSettles drives the entry
// point over a module root whose record names the actions of one page and
// whose cache holds that page's sections, for each page whose rows issue 1240
// read against GitLab's source. No finding of the page stays open: a row a
// tier tag answered is not a finding at all, since the schema below its tier
// no longer offers the parameter, and every other row is answered by the real
// table with the category it gives, and no declaration of the table is stale.
func TestRunMain_RealTable_AnswersTheRowsGitLabsSourceSettles(t *testing.T) {
	cases := []struct {
		name   string
		area   string
		page   string
		domain string
		record []actionrequests.RecordAction
		want   map[string]string
	}{
		{
			name:   "groups",
			area:   "groups",
			domain: "groups",
			page: endpointSection("List groups", "GET /groups", rowRepositoryStorage, rowMarkedForDeletionOn) +
				endpointSection("List projects", "GET /groups/:id/projects", rowSecurityReports) +
				endpointSection("Create a group", "POST /groups", rowMembershipLock, rowExtraMinutesLimit, rowMinutesLimit, rowWikiAccessLevel) +
				endpointSection("Update group attributes", "PUT /groups/:id",
					rowExtraMinutesLimit, rowFileTemplateProject, rowMembershipLock, rowPreventForking, rowMinutesLimit,
					rowIPRestriction, rowEmailDomains, rowWikiAccessLevel, rowDuoFeatures, rowLockDuoFeatures,
					rowPipelineSucceeds, rowSkippedPipeline, rowDiscussionsResolved) +
				// The share's Minimal access row is the page's other answer,
				// and without it the read page would make that one stale.
				endpointSection("Invite groups", "POST /groups/:id/share", rowShareGroupAccess),
			record: []actionrequests.RecordAction{
				restRecord("group.list", "GET /groups"),
				restRecord("group.projects", "GET /groups/:id/projects"),
				restRecord("group.create", "POST /groups"),
				restRecord("group.update", "PUT /groups/:id"),
				restRecord("group.share_with_group", "POST /groups/:id/share"),
			},
			want: map[string]string{
				"group.share_with_group group_access":             categoryValueLevelGate,
				"group.create membership_lock":                    categoryNotGated,
				"group.create shared_runners_minutes_limit":       categoryNotGated,
				"group.create extra_shared_runners_minutes_limit": categoryNotGated,
				"group.update membership_lock":                    categoryNotGated,
				"group.update shared_runners_minutes_limit":       categoryNotGated,
				"group.update extra_shared_runners_minutes_limit": categoryNotGated,
				"group.update duo_features_enabled":               categoryValueLevelGate,
				"group.update lock_duo_features_enabled":          categoryValueLevelGate,
				"group.list marked_for_deletion_on":               categoryNotGated,
			},
		},
		{
			name:   "group_members",
			area:   "group_members",
			domain: "groupmembers",
			page: endpointSection("Add a group member", routeGroupMemberAdd, rowMemberAccessLevel, rowMemberRoleAdd) +
				endpointSection("Update a group member", routeGroupMemberEdit, rowMemberAccessLevel, rowMemberRoleEdit),
			record: []actionrequests.RecordAction{
				restRecord("group.group_member_add", routeGroupMemberAdd),
				restRecord("group.group_member_edit", routeGroupMemberEdit),
			},
			want: map[string]string{
				"group.group_member_add access_level":    categoryValueLevelGate,
				"group.group_member_edit access_level":   categoryValueLevelGate,
				"group.group_member_edit member_role_id": categoryValueLevelGate,
			},
		},
		{
			name:   "project_members",
			area:   "project_members",
			domain: "members",
			page: endpointSection("Add a member to a project", routeProjectMemberAdd, rowMemberRoleAdd) +
				endpointSection("Update a member of a project", routeProjectMemberEdit, rowMemberRoleEdit),
			record: []actionrequests.RecordAction{
				restRecord("project.member_add", routeProjectMemberAdd),
				restRecord("project.member_edit", routeProjectMemberEdit),
			},
			want: map[string]string{
				"project.member_edit member_role_id": categoryValueLevelGate,
			},
		},
		{
			name:   "group_ldap_links",
			area:   "group_ldap_links",
			domain: "groupldap",
			page:   endpointSection("Add an LDAP group link with CN or filter", "POST /groups/:id/ldap_group_links", rowLDAPLinkMemberRole),
			record: []actionrequests.RecordAction{
				restRecord("group.ldap_link_add", "POST /groups/:id/ldap_group_links"),
			},
			want: map[string]string{},
		},
		{
			name:   "group_level_variables",
			area:   "group_level_variables",
			domain: "groupvariables",
			page: endpointSection("Retrieve details of a group variable", routeGroupVariableGet, rowVariableFilter) +
				endpointSection("Create a group variable", routeGroupVariables, rowVariableScope) +
				endpointSection("Update a group variable", routeGroupVariableUpdate, rowVariableScope, rowVariableFilter) +
				endpointSection("Delete a group variable", routeGroupVariableDelete, rowVariableFilter),
			record: []actionrequests.RecordAction{
				restRecord("ci_variable.group_get", routeGroupVariableGet),
				restRecord("ci_variable.group_create", routeGroupVariables),
				restRecord("ci_variable.group_update", routeGroupVariableUpdate),
				restRecord("ci_variable.group_delete", routeGroupVariableDelete),
			},
			want: map[string]string{
				"ci_variable.group_get filter":               categoryNotGated,
				"ci_variable.group_create environment_scope": categoryValueLevelGate,
				"ci_variable.group_update environment_scope": categoryNotGated,
				"ci_variable.group_update filter":            categoryNotGated,
				"ci_variable.group_delete filter":            categoryNotGated,
			},
		},
		{
			name:   "issues",
			area:   "issues",
			domain: "issues",
			page:   endpointSection("New issue", routeIssueCreate, rowIssueAssigneeIDs),
			record: []actionrequests.RecordAction{restRecord("issue.create", routeIssueCreate)},
			want: map[string]string{
				"issue.create assignee_ids": categoryValueLevelGate,
			},
		},
		{
			name:   "merge_requests",
			area:   "merge_requests",
			domain: "mergerequests",
			page: endpointSection("List merge requests", routeMergeRequestsGlobal, rowMergeRequestApproverIDs) +
				endpointSection("List project merge requests", routeMergeRequestsProject, rowMergeRequestApproverIDs) +
				endpointSection("List group merge requests", routeMergeRequestsGroup, rowMergeRequestApproverIDs),
			record: []actionrequests.RecordAction{
				restRecord("merge_request.list_global", routeMergeRequestsGlobal),
				restRecord("merge_request.list", routeMergeRequestsProject),
				restRecord("merge_request.list_group", routeMergeRequestsGroup),
			},
			want: map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fakeModuleRoot(t)
			seedRecord(t, root, tc.record...)
			seedDoc(t, apidocs.CacheDir(root), tc.area, tc.page)
			t.Chdir(root)

			rep := runReport(t, "-offline")
			domain := findDomain(t, rep, tc.domain)
			if domain.ParamFindings != nil {
				t.Errorf("%s keeps open findings %+v, want every row answered", tc.domain, domain.ParamFindings)
			}
			got := map[string]string{}
			for _, answered := range domain.ParamFindingsAnswered {
				got[answered.Action+" "+answered.Param] = answered.Category
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("answered = %v, want %v", got, tc.want)
			}
			if rep.StaleParamDeclarations != nil {
				t.Errorf("stale = %+v, want none of the real table", rep.StaleParamDeclarations)
			}
		})
	}
}
