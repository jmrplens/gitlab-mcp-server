package paths

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apishapes"
)

// indexOf builds the operation index a type-grain comparison looks up in.
func indexOf(operations map[string]apishapes.Operation) *operationIndex {
	return newOperationIndex(apishapes.Document{Operations: operations})
}

// stubTypeGrainInputs replaces the two loaders the real tree resolves: the
// typed package load that costs twenty seconds and the client-go source in a
// module cache.
//
// It empties the declaration table with them. The real entries are about the
// real tree, so against a synthetic one every last one of them is unused, and a
// test of what the join counts would be asserting on this repository's
// adjudications instead. [TestClassifyShapeFindings_ADeclaration_AnswersItsOwnFinding]
// is where the table itself is exercised.
func stubTypeGrainInputs(t *testing.T, pairings structs.Pairings, loadErr error, routes map[string][]sdkRoute) {
	t.Helper()
	previousPairings, previousRoutes, previousDeclarations, previousSent := collectPairings, readRoutes, declaredShapeFields, declaredUnsurfaced
	collectPairings = func(string) (structs.Pairings, error) { return pairings, loadErr }
	readRoutes = func(string) map[string][]sdkRoute { return routes }
	declaredShapeFields, declaredUnsurfaced = nil, nil
	t.Cleanup(func() {
		collectPairings, readRoutes, declaredShapeFields, declaredUnsurfaced = previousPairings, previousRoutes, previousDeclarations, previousSent
	})
}

// approvalOperations is what GitLab's document says about the two endpoints
// gl.MergeRequestApprovals is answered from. Both send four names; the twenty
// the type used to publish beside them are the response of the POST at
// /approvals, which was deprecated in GitLab 16.0 and which client-go no longer
// has a method for.
var approvalOperations = map[string]apishapes.Operation{
	"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {
		Response: []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
	},
	"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve": {
		Response: []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
	},
}

// approvalRoutes are the two endpoints client-go answers gl.MergeRequestApprovals from.
var approvalRoutes = map[string][]sdkRoute{"MergeRequestApprovals": {
	{Method: "GET", Path: "/projects/:/merge_requests/:/approvals"},
	{Method: "POST", Path: "/projects/:/merge_requests/:/approve"},
}}

// approvalPairing is the converter pairing the field diff records for ConfigOutput.
var approvalPairing = structs.Pairings{
	ClientGoDir: "/client-go",
	Outputs: []structs.OutputPairing{
		{Package: "mrapprovals", MCPType: "ConfigOutput", SDKType: "MergeRequestApprovals"},
	},
}

// TestTypedShapeCheck_TheShapeIssue580Fixed_IsTheOneItWasBuiltFor verifies the
// join against the one confirmed phantom this repository has found. Before the
// fix, gitlab_mr_approval_config published every field of the SDK struct and
// GitLab answered its GET with four of them; the package-grain join reported
// the difference among six hundred others, and this grain has to report those
// twenty and nothing else. The fixed shape must come back clean, or the check
// would be reporting the fix rather than the defect.
func TestTypedShapeCheck_TheShapeIssue580Fixed_IsTheOneItWasBuiltFor(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)
	published := []string{"approved", "approved_by", "user_can_approve", "user_has_approved"}
	// The shape as it stood before the fix, in declaration order: what
	// publishedTypes hands over is sorted, and the comparison does not care.
	deprecated := append([]string{
		"id", "iid", "project_id", "title", "description", "state", "created_at",
		"updated_at", "merge_status", "approvals_required", "approvals_left",
		"approvals_before_merge", "require_password_to_approve", "has_approval_rules",
		"merge_request_approvers_available", "multiple_approval_rules_available",
		"suggested_approvers", "approvers", "approver_groups", "approval_rules_left",
	}, published...)

	cases := []struct {
		name   string
		fields []string
		want   int
	}{
		{name: "the shape the deprecated POST answers with", fields: deprecated, want: 20},
		{name: "the shape GitLab answers the GET with", fields: published, want: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			check := typedShapeCheck("", indexOf(approvalOperations), []publishedType{{
				Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: testCase.fields,
			}})

			if !check.Ran || check.Compared != 1 {
				t.Fatalf("check = %+v, want one comparison", check)
			}
			if len(check.Unpublished) != testCase.want {
				t.Fatalf("reported %d field(s), want %d: %+v", len(check.Unpublished), testCase.want, check.Unpublished)
			}
			for _, finding := range check.Unpublished {
				if slices.Contains(published, finding.Field) {
					t.Errorf("reported %q, which GitLab's document lists for both operations", finding.Field)
				}
			}
		})
	}
}

// TestTypedShapeCheck_AFinding_NamesWhatWasSearched verifies that a finding
// carries its own grounds. A reader arriving at one has to be able to tell it
// from a package-grain finding, see which client-go struct named the operations
// and see the operations themselves, or the only way to judge it is to rebuild
// the join by hand.
func TestTypedShapeCheck_AFinding_NamesWhatWasSearched(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)

	check := typedShapeCheck("", indexOf(approvalOperations), []publishedType{{
		Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"approved", "title"},
	}})

	want := []UnpublishedField{{
		Grain:     grainType,
		Package:   "internal/tools/mrapprovals",
		Type:      "ConfigOutput",
		Field:     "title",
		SDKType:   "MergeRequestApprovals",
		Endpoints: 2,
		Operations: []string{
			"GET /projects/:/merge_requests/:/approvals",
			"POST /projects/:/merge_requests/:/approve",
		},
	}}
	if !reflect.DeepEqual(check.Unpublished, want) {
		t.Errorf("unpublished = %+v, want %+v", check.Unpublished, want)
	}
}

// TestTypedShapeCheck_WhatItRefusesToJudge_IsCountedAndNotReported verifies the
// three silences this grain keeps, which are the whole reason it is sharper
// than the package one. A type no converter pairs is a wrapper or a synthetic
// result and belongs to no endpoint; a type nothing routes to is only ever
// nested inside another response; and a route the document is silent about says
// nothing rather than that GitLab sends nothing. Judging any of them would
// invent findings, so each is counted where a reader can see how much of the
// tree went uncompared.
func TestTypedShapeCheck_WhatItRefusesToJudge_IsCountedAndNotReported(t *testing.T) {
	cases := []struct {
		name       string
		pairings   structs.Pairings
		routes     map[string][]sdkRoute
		operations map[string]apishapes.Operation
		want       TypedShapeCheck
	}{
		{
			name:       "no converter pairs the type with an SDK struct",
			pairings:   structs.Pairings{ClientGoDir: "/client-go"},
			routes:     approvalRoutes,
			operations: approvalOperations,
			want:       TypedShapeCheck{Ran: true, SkippedNoPairing: 1},
		},
		{
			name:       "no method answers with the SDK struct",
			pairings:   approvalPairing,
			routes:     nil,
			operations: approvalOperations,
			want:       TypedShapeCheck{Ran: true, SkippedNoRoute: 1},
		},
		{
			name:       "the document carries none of the routes",
			pairings:   approvalPairing,
			routes:     approvalRoutes,
			operations: map[string]apishapes.Operation{"GET /api/v4/version": {Response: []string{"version"}}},
			want:       TypedShapeCheck{Ran: true, SkippedNoSchema: 1},
		},
		{
			name:     "the document names the routes and no response for them",
			pairings: approvalPairing,
			routes:   approvalRoutes,
			operations: map[string]apishapes.Operation{
				"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {},
				"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve":  {},
			},
			want: TypedShapeCheck{Ran: true, SkippedNoSchema: 1},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stubTypeGrainInputs(t, testCase.pairings, nil, testCase.routes)

			check := typedShapeCheck("", indexOf(testCase.operations), []publishedType{{
				Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"invented"},
			}})

			if !reflect.DeepEqual(check, testCase.want) {
				t.Errorf("check = %+v, want %+v", check, testCase.want)
			}
		})
	}
}

// TestTypedShapeCheck_ALooseMatch_IsNotAccepted verifies that the loose lookup
// the package grain leans on is refused here. Accepting a literal segment of
// ours where GitLab has a placeholder is evidence about a fixture value in the
// recorded inventory; a route template has no fixture values in it, so the same
// acceptance here would only be a guess at which operation a path meant.
func TestTypedShapeCheck_ALooseMatch_IsNotAccepted(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, map[string][]sdkRoute{
		"MergeRequestApprovals": {{Method: "GET", Path: "/projects/:/merge_requests/:/approvals/extra"}},
	})

	check := typedShapeCheck("", indexOf(map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals/{approval_id}": {
			Response: []string{"approved"},
		},
	}), []publishedType{{
		Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"approved"},
	}})

	if check.SkippedNoSchema != 1 || len(check.Unpublished) != 0 {
		t.Errorf("check = %+v, want the type left uncompared rather than matched loosely", check)
	}
}

// TestTypedShapeCheck_ARouteNothingWasSearchedIn_IsNotNamed verifies that a
// finding names the responses it was really held against. A route the document
// does not carry, and one it carries without a response, contribute no names to
// the union, so naming them would tell a reader the field was looked for in
// three endpoints when it was looked for in one, and the count would mean
// something different here than it does at package grain.
func TestTypedShapeCheck_ARouteNothingWasSearchedIn_IsNotNamed(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, map[string][]sdkRoute{"MergeRequestApprovals": {
		{Method: "GET", Path: "/projects/:/merge_requests/:/approvals"},
		{Method: "POST", Path: "/projects/:/merge_requests/:/approve"},
		{Method: "GET", Path: "/projects/:/merge_requests/:/approval_state"},
	}})

	check := typedShapeCheck("", indexOf(map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {Response: []string{"approved"}},
		"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve":  {},
	}), []publishedType{{
		Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"title"},
	}})

	want := []UnpublishedField{{
		Grain:      grainType,
		Package:    "internal/tools/mrapprovals",
		Type:       "ConfigOutput",
		Field:      "title",
		SDKType:    "MergeRequestApprovals",
		Endpoints:  1,
		Operations: []string{"GET /projects/:/merge_requests/:/approvals"},
	}}
	if check.Compared != 1 || !reflect.DeepEqual(check.Unpublished, want) {
		t.Errorf("check = %+v, want the one operation whose response was searched", check)
	}
}

// TestTypedShapeCheck_OneTypeFromSeveralSDKStructs_UnionsTheirOperations
// verifies the shape a domain that serves a group and a project scope from one
// output type takes: labels are filled from both gl.Label and gl.GroupLabel, so
// a field either endpoint sends is a field GitLab sends. Diffing against one of
// them would report the other's as unpublished, which is the same defect the
// package grain's union avoids one level up.
func TestTypedShapeCheck_OneTypeFromSeveralSDKStructs_UnionsTheirOperations(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs: []structs.OutputPairing{
			{Package: "labeldata", MCPType: "Output", SDKType: "Label"},
			{Package: "labeldata", MCPType: "Output", SDKType: "GroupLabel"},
			// The same pairing twice: a type built by two converters from one
			// struct must not have its operations searched, or counted, twice.
			{Package: "labeldata", MCPType: "Output", SDKType: "Label"},
		},
	}, nil, map[string][]sdkRoute{
		"Label":      {{Method: "GET", Path: "/projects/:/labels", Many: true}},
		"GroupLabel": {{Method: "GET", Path: "/groups/:/labels", Many: true}},
	})

	check := typedShapeCheck("", indexOf(map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/labels": {Response: []string{"id", "name"}},
		"GET /api/v4/groups/{id}/labels":   {Response: []string{"id", "subscribed"}},
	}), []publishedType{{
		Package: "internal/tools/labeldata", Name: "Output",
		Fields: []string{"id", "invented", "name", "subscribed"},
	}})

	if check.Compared != 1 || len(check.Unpublished) != 1 || check.Unpublished[0].Field != "invented" {
		t.Fatalf("check = %+v, want only the field neither endpoint sends", check)
	}
	finding := check.Unpublished[0]
	if finding.SDKType != "GroupLabel, Label" || finding.Endpoints != 2 {
		t.Errorf("finding = %+v, want both structs named and both endpoints counted", finding)
	}
	if finding.Operations[0] != "GET /groups/:/labels (collection)" {
		t.Errorf("operations = %v, want a collection endpoint said to be one", finding.Operations)
	}
}

// TestTypedShapeCheck_WithoutItsInputs_DoesNotRun verifies the two ways this
// grain is skipped whole. Both are a tree it cannot read rather than a finding
// about the tree, and reporting Ran false is what keeps a reader from taking
// zero comparisons for zero phantoms.
func TestTypedShapeCheck_WithoutItsInputs_DoesNotRun(t *testing.T) {
	cases := []struct {
		name     string
		pairings structs.Pairings
		err      error
	}{
		{name: "the tool packages do not load", pairings: approvalPairing, err: errors.New("load packages")},
		{name: "the import graph names no client-go", pairings: structs.Pairings{Outputs: approvalPairing.Outputs}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stubTypeGrainInputs(t, testCase.pairings, testCase.err, approvalRoutes)

			check := typedShapeCheck("", indexOf(approvalOperations), []publishedType{{
				Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"title"},
			}})

			if !reflect.DeepEqual(check, TypedShapeCheck{}) {
				t.Errorf("check = %+v, want nothing run", check)
			}
		})
	}
}

// TestShapeCheck_BothGrains_AreReportedTogether verifies that the two joins are
// published beside each other from one call. Keeping the package grain is what
// lets a reader see which of its findings the sharper join keeps, and a report
// carrying only one of them cannot be read that way.
func TestShapeCheck_BothGrains_AreReportedTogether(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)
	root := recordIn(t, approvalOperations)

	check := shapeCheck(root, nil, []publishedType{{
		Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"approved", "title"},
	}})

	// No request row was recorded, so the package grain has no endpoints to
	// union and judges nothing; the type grain reaches the same operations
	// through the SDK and reports the one field.
	if len(check.Unpublished) != 0 {
		t.Errorf("package grain reported %+v without an endpoint to search", check.Unpublished)
	}
	if len(check.Typed.Unpublished) != 1 || check.Typed.Unpublished[0].Grain != grainType {
		t.Errorf("typed = %+v, want the one type-grain finding", check.Typed)
	}
}

// approvedByOperations is the same GET, with the properties GitLab's record
// gives the object under approved_by. It is the shape the nested level was
// added for: approved_by carries a user and a timestamp, and holding the type
// that models it against the top-level names would condemn both.
var approvedByOperations = map[string]apishapes.Operation{
	"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {
		Response: []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
		Nested:   map[string][]string{"approved_by": {"approved_at", "user"}},
	},
}

// approvalConfigWithApprovers is ConfigOutput as it stands, with the nested
// type its approved_by field carries.
var approvalConfigWithApprovers = publishedType{
	Package: "internal/tools/mrapprovals",
	Name:    "ConfigOutput",
	Fields:  []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
	Nested: map[string]nestedType{
		"approved_by": {Name: "ApproverOutput", Fields: []string{"approved_at", "user"}},
	},
}

// TestTypedShapeCheck_ANestedType_IsJudgedUnderItsOwnProperty verifies the
// level the record's schema version 2 made possible. The fields of a nested
// type are the properties of the object it sits under, never of the response,
// so the same list that is right under approved_by is wrong at the top: a
// nested type judged against the response names is the 1418-finding run that
// made this level uncomparable before the record carried it.
func TestTypedShapeCheck_ANestedType_IsJudgedUnderItsOwnProperty(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)

	check := typedShapeCheck("", indexOf(approvedByOperations), []publishedType{approvalConfigWithApprovers})

	if check.NestedCompared != 1 {
		t.Errorf("NestedCompared = %d, want the one nested type held against approved_by", check.NestedCompared)
	}
	if len(check.Nested) != 0 {
		t.Errorf("nested findings = %+v, want none: the record gives approved_by both names", check.Nested)
	}
}

// TestTypedShapeCheck_ANestedFieldTheObjectDoesNotCarry_IsReportedUnderIt
// verifies that a nested finding says which property it belongs to. Two nested
// types can publish the same tag under different properties, so a finding that
// named only the type and the field would send a reader to the wrong object.
func TestTypedShapeCheck_ANestedFieldTheObjectDoesNotCarry_IsReportedUnderIt(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)
	candidate := approvalConfigWithApprovers
	candidate.Nested = map[string]nestedType{
		"approved_by": {Name: "ApproverOutput", Fields: []string{"approved_at", "invented", "user"}},
	}

	check := typedShapeCheck("", indexOf(approvedByOperations), []publishedType{candidate})

	want := []UnpublishedField{{
		Grain: grainType, Package: "internal/tools/mrapprovals", Type: "ApproverOutput",
		Under: "approved_by", Field: "invented", SDKType: "MergeRequestApprovals", Endpoints: 1,
		Operations: []string{"GET /projects/:/merge_requests/:/approvals"},
	}}
	if !reflect.DeepEqual(check.Nested, want) {
		t.Errorf("nested findings = %+v, want %+v", check.Nested, want)
	}
}

// TestTypedShapeCheck_ANestedPropertyTheRecordDescribesNoObjectFor_IsNotJudged
// verifies the reticence that makes the nested level usable at all. An empty
// nested list means the document does not describe an object there, which is
// the same thing an empty response list means one level up, and judging against
// one would report every field of the type.
func TestTypedShapeCheck_ANestedPropertyTheRecordDescribesNoObjectFor_IsNotJudged(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)

	check := typedShapeCheck("", indexOf(approvalOperations), []publishedType{approvalConfigWithApprovers})

	if check.Compared != 1 {
		t.Errorf("Compared = %d, want the top-level type still judged", check.Compared)
	}
	if check.NestedCompared != 0 || len(check.Nested) != 0 {
		t.Errorf("nested = %d compared, %+v reported; want the property left alone", check.NestedCompared, check.Nested)
	}
}

// TestTypedShapeCheck_TwoOperationsSharingAShape_UnionTheirNestedProperties
// verifies the merge the shape index performs. Two operations can collapse to
// one shape, and taking either one's nested map alone would report the other's
// nested fields as unpublished, which is the same defect the response-name
// union was written to avoid.
func TestTypedShapeCheck_TwoOperationsSharingAShape_UnionTheirNestedProperties(t *testing.T) {
	stubTypeGrainInputs(t, approvalPairing, nil, approvalRoutes)
	operations := map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {
			Response: []string{"approved_by"},
			Nested:   map[string][]string{"approved_by": {"user"}},
		},
		"GET /api/v4/projects/{other}/merge_requests/{iid}/approvals": {
			Response: []string{"approved_by"},
			Nested:   map[string][]string{"approved_by": {"approved_at"}},
		},
	}
	candidate := approvalConfigWithApprovers
	candidate.Fields = []string{"approved_by"}

	check := typedShapeCheck("", indexOf(operations), []publishedType{candidate})

	if len(check.Nested) != 0 {
		t.Errorf("nested findings = %+v, want none: between them the two operations name both properties", check.Nested)
	}
}

// TestClassifyShapeFindings_ADeclaration_AnswersItsOwnFinding verifies the
// table the type grain answers a finding with, at both levels and in both
// directions: a declaration that matches annotates its finding and is not
// stale, and one that matches nothing is reported so the excuse cannot outlive
// the thing it excused.
func TestClassifyShapeFindings_ADeclaration_AnswersItsOwnFinding(t *testing.T) {
	previous := declaredShapeFields
	declaredShapeFields = []shapeDeclaration{
		{Package: "internal/tools/one", Type: "Output", Field: declaredSegment, Category: categoryRecordSilent, Reason: "whole type"},
		{Package: "internal/tools/two", Type: "NestedOutput", Field: "named", Category: categoryRecordSilent, Reason: "one field"},
		{Package: "internal/tools/gone", Type: "Output", Field: "vanished", Category: categoryRecordSilent, Reason: "matches nothing"},
	}
	t.Cleanup(func() { declaredShapeFields = previous })

	topLevel, nested, unused := classifyShapeFindings(
		[]UnpublishedField{
			{Package: "internal/tools/one", Type: "Output", Field: "anything"},
			{Package: "internal/tools/one", Type: "OtherOutput", Field: "anything"},
		},
		[]UnpublishedField{
			{Package: "internal/tools/two", Type: "NestedOutput", Under: "parent", Field: "named"},
			{Package: "internal/tools/two", Type: "NestedOutput", Under: "parent", Field: "other"},
		},
	)

	if topLevel[0].Reason != "whole type" || topLevel[1].declared() {
		t.Errorf("top-level = %+v, want only the declared type answered", topLevel)
	}
	if nested[0].Reason != "one field" || nested[1].declared() {
		t.Errorf("nested = %+v, want only the declared field answered", nested)
	}
	if !reflect.DeepEqual(unused, []string{"internal/tools/gone.Output.vanished"}) {
		t.Errorf("unused = %v, want the declaration that matched nothing", unused)
	}
}

// TestTypedShapeCheck_StaleDeclarations_AreSilentUntilTheCheckRuns verifies
// that a check that compared nothing says nothing about its declarations. Every
// one of them would look unused, and the loudest wrong answer this could give
// is that they are all stale.
func TestTypedShapeCheck_StaleDeclarations_AreSilentUntilTheCheckRuns(t *testing.T) {
	stale := TypedShapeCheck{Ran: true, UnusedDeclarations: []string{"internal/tools/one.Output.field"}}
	if lines := stale.staleDeclarations(); len(lines) != 1 || !strings.Contains(lines[0], "internal/tools/one.Output.field") {
		t.Errorf("staleDeclarations() = %v, want the one declaration named", lines)
	}
	if lines := (TypedShapeCheck{UnusedDeclarations: stale.UnusedDeclarations}).staleDeclarations(); lines != nil {
		t.Errorf("staleDeclarations() = %v on a check that did not run, want nothing", lines)
	}
}

// TestTypedShapeCheck_UndeclaredFindings_AreCountedAtBothLevels verifies the
// number the report leads with, which is what a reader is being asked to act
// on: an answered finding is context, and one at either level that nothing
// answers is work.
func TestTypedShapeCheck_UndeclaredFindings_AreCountedAtBothLevels(t *testing.T) {
	check := TypedShapeCheck{
		Unpublished: []UnpublishedField{{Field: "answered", Category: categoryRecordSilent}, {Field: "open"}},
		Nested:      []UnpublishedField{{Field: "open too"}},
	}
	if got := check.undeclared(); got != 2 {
		t.Errorf("undeclared() = %d, want the two findings nothing answers", got)
	}
}
