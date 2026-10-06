package paths

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
)

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
	previousPairings, previousRoutes, previousMethodRoutes := collectPairings, readRoutes, readMethodRoutes
	previousDeclarations, previousSent := declaredShapeFields, declaredUnsurfaced
	collectPairings = func(string) (structs.Pairings, error) { return pairings, loadErr }
	readRoutes = func(string) map[string][]sdkRoute { return routes }
	readMethodRoutes = func(string) map[string][]sdkRoute { return nil }
	declaredShapeFields, declaredUnsurfaced = nil, nil
	t.Cleanup(func() {
		collectPairings, readRoutes, readMethodRoutes = previousPairings, previousRoutes, previousMethodRoutes
		declaredShapeFields, declaredUnsurfaced = previousDeclarations, previousSent
	})
}

// stubMethodRoutes hands the type grain the per-method routes a projection is
// judged against, on top of what [stubTypeGrainInputs] stubbed.
func stubMethodRoutes(t *testing.T, routes map[string][]sdkRoute) {
	t.Helper()
	previous := readMethodRoutes
	readMethodRoutes = func(string) map[string][]sdkRoute { return routes }
	t.Cleanup(func() { readMethodRoutes = previous })
}

// approvalOperations is what GitLab's document says about the two endpoints
// gl.MergeRequestApprovals is answered from. Both send four names; the twenty
// the type used to publish beside them are the response of the POST at
// /approvals, which was deprecated in GitLab 16.0 and which client-go no longer
// has a method for.
var approvalOperations = map[string]response{
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
			check := typedCheckOf("", approvalOperations, []publishedType{{
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

// TestTypedShapeCheck_ATypeNamedAsAField_IsJudgedWhenAConverterPairsIt
// verifies the rule that decides whether this grain sees anything at all.
//
// The convention in this repository is a one-key envelope: a get handler
// returns `{badge: BadgeItem}`, and it is `BadgeItem` that models GitLab's
// response and that a converter pairs with a client-go struct. The envelope
// carries no pairing. So a grain that passes over every type some struct names
// as a field passes over exactly the types that model the responses, and it
// saw 26 of 441 for that reason alone.
//
// What being named as a field actually says is nothing about GitLab: the
// pairing is the whole question, because that struct's service methods name
// the endpoints. A type named as a field and paired is judged; one named as a
// field and unpaired is not, and is not counted either, since the skip figures
// are about the responses this grain was meant to judge and would stop being
// comparable if a second population joined them.
func TestTypedShapeCheck_ATypeNamedAsAField_IsJudgedWhenAConverterPairsIt(t *testing.T) {
	cases := []struct {
		name          string
		pairings      structs.Pairings
		payload       bool
		wantCompared  int
		wantInner     int
		wantNoPairing int
		wantFindings  int
	}{
		{
			name:     "wrapped by an envelope and paired",
			pairings: approvalPairing, payload: true,
			wantCompared: 1, wantInner: 1, wantFindings: 1,
		},
		{
			name:     "wrapped by an envelope and unpaired",
			pairings: structs.Pairings{ClientGoDir: "/client-go"}, payload: true,
		},
		{
			// A reference to another resource sitting inside a response. Its
			// pairing names the struct of the whole entity, so judging it
			// here would hold a job's project reference to what
			// GET /projects/:id answers with.
			name:     "a field of a response that carries other content, paired",
			pairings: approvalPairing,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stubTypeGrainInputs(t, testCase.pairings, nil, approvalRoutes)

			check := typedCheckOf("", approvalOperations, []publishedType{{
				Package: "internal/tools/mrapprovals", Name: "ConfigOutput",
				Fields: []string{"approved", "title"}, Inner: true, Payload: testCase.payload,
			}})

			if check.Compared != testCase.wantCompared || check.ComparedInner != testCase.wantInner {
				t.Errorf("compared = %d (%d inner), want %d (%d inner)",
					check.Compared, check.ComparedInner, testCase.wantCompared, testCase.wantInner)
			}
			if check.SkippedNoPairing != testCase.wantNoPairing || len(check.Skipped.NoPairing) != testCase.wantNoPairing {
				t.Errorf("no-pairing skips = %d/%v, want %d", check.SkippedNoPairing, check.Skipped.NoPairing, testCase.wantNoPairing)
			}
			if len(check.Unpublished) != testCase.wantFindings {
				t.Errorf("reported %d finding(s), want %d: %+v", len(check.Unpublished), testCase.wantFindings, check.Unpublished)
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

	check := typedCheckOf("", approvalOperations, []publishedType{{
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
// tree went uncompared, and named beside the count: the figure alone says how
// much was declined and nothing about whether declining was right, which is
// the only question a reader has.
func TestTypedShapeCheck_WhatItRefusesToJudge_IsCountedAndNotReported(t *testing.T) {
	cases := []struct {
		name       string
		pairings   structs.Pairings
		routes     map[string][]sdkRoute
		operations map[string]response
		want       TypedShapeCheck
	}{
		{
			name:       "no converter pairs the type with an SDK struct",
			pairings:   structs.Pairings{ClientGoDir: "/client-go"},
			routes:     approvalRoutes,
			operations: approvalOperations,
			want: TypedShapeCheck{
				Ran: true, SkippedNoPairing: 1,
				Skipped: SkippedTypes{NoPairing: []string{"mrapprovals.ConfigOutput"}},
			},
		},
		{
			name:       "no method answers with the SDK struct",
			pairings:   approvalPairing,
			routes:     nil,
			operations: approvalOperations,
			want: TypedShapeCheck{
				Ran: true, SkippedNoRoute: 1,
				Skipped: SkippedTypes{NoRoute: []string{"mrapprovals.ConfigOutput"}},
			},
		},
		{
			name:       "the document carries none of the routes",
			pairings:   approvalPairing,
			routes:     approvalRoutes,
			operations: map[string]response{"GET /api/v4/version": {Response: []string{"version"}}},
			want: TypedShapeCheck{
				Ran: true, SkippedNoSchema: 1,
				Skipped: SkippedTypes{NoSchema: []string{"mrapprovals.ConfigOutput"}},
			},
		},
		{
			name:     "the document names the routes and no response for them",
			pairings: approvalPairing,
			routes:   approvalRoutes,
			operations: map[string]response{
				"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {},
				"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve":  {},
			},
			want: TypedShapeCheck{
				Ran: true, SkippedNoSchema: 1,
				Skipped: SkippedTypes{NoSchema: []string{"mrapprovals.ConfigOutput"}},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stubTypeGrainInputs(t, testCase.pairings, nil, testCase.routes)

			check := typedCheckOf("", testCase.operations, []publishedType{{
				Package: "internal/tools/mrapprovals", Name: "ConfigOutput", Fields: []string{"invented"},
			}})

			if !reflect.DeepEqual(check, testCase.want) {
				t.Errorf("check = %+v, want %+v", check, testCase.want)
			}
		})
	}
}

// TestTypedShapeCheck_ABorrowedType_IsPassedOver verifies that a type another
// tools package declares, returned under a package that names it in a field,
// is neither compared nor counted here even when something pairs it: its own
// package judges it, and judging it again under the package that borrows it
// would hold one domain's shape to another domain's endpoints.
func TestTypedShapeCheck_ABorrowedType_IsPassedOver(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs:     []structs.OutputPairing{{Package: "mergerequests", MCPType: "mrapprovals.ConfigOutput", SDKType: "MergeRequestApprovals"}},
	}, nil, approvalRoutes)

	check := typedCheckOf("", approvalOperations, []publishedType{{
		Package: "internal/tools/mergerequests", Name: "mrapprovals.ConfigOutput", Fields: []string{"invented"},
		Inner: true, Borrowed: true,
	}})

	if want := (TypedShapeCheck{Ran: true}); !reflect.DeepEqual(check, want) {
		t.Errorf("check = %+v, want %+v", check, want)
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

	check := typedCheckOf("", map[string]response{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals/{approval_id}": {
			Response: []string{"approved"},
		},
	}, []publishedType{{
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

	check := typedCheckOf("", map[string]response{
		"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {Response: []string{"approved"}},
		"POST /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approve":  {},
	}, []publishedType{{
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
			// Two different structs answered from one endpoint, which is what
			// client-go does wherever a scope has a struct of its own and a
			// shared route: the endpoint is one response and is counted once.
			{Package: "labeldata", MCPType: "Output", SDKType: "ProjectLabel"},
		},
	}, nil, map[string][]sdkRoute{
		"Label":        {{Method: "GET", Path: "/projects/:/labels", Many: true}},
		"GroupLabel":   {{Method: "GET", Path: "/groups/:/labels", Many: true}},
		"ProjectLabel": {{Method: "GET", Path: "/projects/:/labels", Many: true}},
	})

	check := typedCheckOf("", map[string]response{
		"GET /api/v4/projects/{id}/labels": {Response: []string{"id", "name"}},
		"GET /api/v4/groups/{id}/labels":   {Response: []string{"id", "subscribed"}},
	}, []publishedType{{
		Package: "internal/tools/labeldata", Name: "Output",
		Fields: []string{"id", "invented", "name", "subscribed"},
	}})

	if check.Compared != 1 || len(check.Unpublished) != 1 || check.Unpublished[0].Field != "invented" {
		t.Fatalf("check = %+v, want only the field neither endpoint sends", check)
	}
	finding := check.Unpublished[0]
	if finding.SDKType != "GroupLabel, Label, ProjectLabel" || finding.Endpoints != 2 {
		t.Errorf("finding = %+v, want every struct named and each endpoint counted once", finding)
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

			check := typedCheckOf("", approvalOperations, []publishedType{{
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
var approvedByOperations = map[string]response{
	"GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals": {
		Response: []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
		Nested:   map[string][]string{"approved_by": {"approved_at", "user"}},
	},
}

// approvalConfigWithApprovers is ConfigOutput as a Community Edition answer
// fills it, with the nested type its approved_by field carries.
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

	check := typedCheckOf("", approvedByOperations, []publishedType{approvalConfigWithApprovers})

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

	check := typedCheckOf("", approvedByOperations, []publishedType{candidate})

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

	check := typedCheckOf("", approvalOperations, []publishedType{approvalConfigWithApprovers})

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
	operations := map[string]response{
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

	check := typedCheckOf("", operations, []publishedType{candidate})

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

// milestoneIssueOperations is what GitLab's document says about the milestone's
// issue list and about the project issue list client-go's Issue is also
// answered from: the second sends a key the first does not.
var milestoneIssueOperations = map[string]response{
	"GET /api/v4/projects/{id}/milestones/{milestone_id}/issues": {Response: []string{"id", "iid", "title", "labels"}},
	"GET /api/v4/projects/{id}/issues":                           {Response: []string{"id", "iid", "title", "labels", "subscribed"}},
}

// milestoneIssueRoutes is every endpoint client-go answers Issue from, and
// milestoneIssueMethodRoutes the one method a milestone's issue row is read
// from.
var (
	milestoneIssueRoutes = map[string][]sdkRoute{"Issue": {
		{Method: "GET", Path: "/projects/:/milestones/:/issues", Many: true},
		{Method: "GET", Path: "/projects/:/issues", Many: true},
	}}
	milestoneIssueMethodRoutes = map[string][]sdkRoute{
		"Milestones.GetMilestoneIssues": {{Method: "GET", Path: "/projects/:/milestones/:/issues", Many: true}},
	}
)

// issueItemProjection is what the structs pass records for a row a handler
// builds field by field out of the milestone issue list's answer.
var issueItemProjection = structs.ProjectionPairing{
	Package: "milestones", MCPType: "IssueItem", SDKType: "Issue",
	SDKFields: []string{"id", "iid", "labels", "title"},
	Methods:   []string{"Milestones.GetMilestoneIssues"},
}

// TestTypedShapeCheck_AProjection_IsJudgedAgainstTheMethodItIsReadFrom
// verifies the pairing issue 971 added. A compact row is built in a handler
// out of one method's answer, and no converter names the pairing, so the type
// grain used to count it a skip and the only comparison that saw its gaps was
// the package grain. Paired now, it is held to the endpoint of that one
// method: a key only the project issue list sends is not a key this row was
// ever sent, and reporting it would repeat the package grain's noise at the
// sharper grain.
func TestTypedShapeCheck_AProjection_IsJudgedAgainstTheMethodItIsReadFrom(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Projections: []structs.ProjectionPairing{issueItemProjection},
	}, nil, milestoneIssueRoutes)
	stubMethodRoutes(t, milestoneIssueMethodRoutes)

	check := typedCheckOf("", milestoneIssueOperations, []publishedType{
		{Package: "internal/tools/milestones", Name: "IssueItem", Fields: []string{"id", "iid", "invented", "title"}, Inner: true, Payload: true},
		{Package: "internal/tools/milestones", Name: "MilestoneIssuesOutput", Fields: []string{"issues", "pagination"}, Wraps: []string{"IssueItem"}},
	})

	if check.Compared != 1 || check.ComparedInner != 1 || !slices.Equal(check.Projections, []string{"milestones.IssueItem"}) {
		t.Fatalf("check = %+v, want the row compared through its projection", check)
	}
	wantOperations := []string{"GET /projects/:/milestones/:/issues (collection)"}
	if len(check.Unpublished) != 1 || check.Unpublished[0].Field != "invented" || !slices.Equal(check.Unpublished[0].Operations, wantOperations) {
		t.Errorf("unpublished = %+v, want only the invented field, searched in the one endpoint", check.Unpublished)
	}
	if len(check.Unsurfaced) != 1 || check.Unsurfaced[0].Field != "labels" || !check.Unsurfaced[0].SDKModels {
		t.Errorf("unsurfaced = %+v, want labels alone, which the projection's own struct models", check.Unsurfaced)
	}
	if !slices.Equal(check.Envelopes, []string{"milestones.MilestoneIssuesOutput"}) || check.SkippedNoPairing != 0 {
		t.Errorf("envelopes = %v, no-pairing = %d; want the list around the row counted as its packaging", check.Envelopes, check.SkippedNoPairing)
	}
}

// TestTypedShapeCheck_AProjectionWithoutAMethod_IsReadAsItsStructWouldBe
// verifies the fallback for a row whose method the structs pass could not
// find: a literal built from a value no call in reach answers with. The
// struct's own endpoints are then searched, which is what a converter pairing
// would have searched, rather than nothing.
func TestTypedShapeCheck_AProjectionWithoutAMethod_IsReadAsItsStructWouldBe(t *testing.T) {
	unrouted := issueItemProjection
	unrouted.Methods = nil
	stubTypeGrainInputs(t, structs.Pairings{ClientGoDir: "/client-go", Projections: []structs.ProjectionPairing{unrouted}}, nil, milestoneIssueRoutes)
	stubMethodRoutes(t, milestoneIssueMethodRoutes)

	check := typedCheckOf("", milestoneIssueOperations, []publishedType{
		{Package: "internal/tools/milestones", Name: "IssueItem", Fields: []string{"id", "iid", "labels", "title"}},
	})

	if check.Compared != 1 || len(check.Unsurfaced) != 1 || check.Unsurfaced[0].Field != "subscribed" {
		t.Errorf("check = %+v, want both endpoints of the struct searched", check)
	}
}

// TestTypedShapeCheck_AProjectionWhoseMethodReachesNothing_IsUnrouted verifies
// that a method this reader could not see a route for leaves the row unrouted
// rather than borrowing its struct's endpoints: the method named it, and a
// guess at which of the struct's endpoints the method meant is exactly what the
// per-method reading exists to avoid.
func TestTypedShapeCheck_AProjectionWhoseMethodReachesNothing_IsUnrouted(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{ClientGoDir: "/client-go", Projections: []structs.ProjectionPairing{issueItemProjection}}, nil, milestoneIssueRoutes)

	check := typedCheckOf("", milestoneIssueOperations, []publishedType{
		{Package: "internal/tools/milestones", Name: "IssueItem", Fields: []string{"id"}},
	})

	if check.SkippedNoRoute != 1 || !slices.Equal(check.Skipped.NoRoute, []string{"milestones.IssueItem"}) || len(check.Projections) != 0 {
		t.Errorf("check = %+v, want the row counted unrouted and not among the compared projections", check)
	}
}

// TestTypedShapeCheck_AConverterPairing_WinsOverAProjection verifies that a
// type a converter pairs keeps every endpoint of its struct. The converter
// says the type models the struct wherever it is answered; a literal in one
// handler says less, and narrowing the converter's type to that handler's
// method would silently drop the endpoints its other callers read.
func TestTypedShapeCheck_AConverterPairing_WinsOverAProjection(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs:     []structs.OutputPairing{{Package: "milestones", MCPType: "IssueItem", SDKType: "Issue"}},
		Projections: []structs.ProjectionPairing{issueItemProjection},
	}, nil, milestoneIssueRoutes)
	stubMethodRoutes(t, milestoneIssueMethodRoutes)

	check := typedCheckOf("", milestoneIssueOperations, []publishedType{
		{Package: "internal/tools/milestones", Name: "IssueItem", Fields: []string{"id", "iid", "labels", "title"}},
	})

	if check.Compared != 1 || len(check.Projections) != 0 || len(check.Unsurfaced) != 1 || check.Unsurfaced[0].Field != "subscribed" {
		t.Errorf("check = %+v, want the converter's two endpoints searched and no projection counted", check)
	}
}

// TestTypedShapeCheck_AnEnvelopeAroundAnUnpairedPayload_StaysASkip verifies
// the other half of the envelope rule: packaging is only judged through its
// payload when the payload has a pairing. Around a payload nothing pairs, the
// envelope is the one name the unjudged response is counted under, and moving
// it out of the skips would lose it from every count.
func TestTypedShapeCheck_AnEnvelopeAroundAnUnpairedPayload_StaysASkip(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{ClientGoDir: "/client-go"}, nil, milestoneIssueRoutes)

	check := typedCheckOf("", milestoneIssueOperations, []publishedType{
		{Package: "internal/tools/milestones", Name: "MilestoneIssuesOutput", Fields: []string{"issues"}, Wraps: []string{"IssueItem", "OtherItem"}},
		{Package: "internal/tools/milestones", Name: "ListOutput", Fields: []string{"items"}},
	})

	want := []string{"milestones.ListOutput", "milestones.MilestoneIssuesOutput"}
	if check.SkippedNoPairing != 2 || !slices.Equal(check.Skipped.NoPairing, want) || len(check.Envelopes) != 0 {
		t.Errorf("check = %+v, want both counted without a pairing", check)
	}
}

// TestTypedShapeCheck_AnEnvelopeAroundASkippedPayload_TakesItsSkip verifies
// that packaging is listed among the envelopes only when the response it
// packages was compared. A payload can be paired and still not be judged,
// for want of a route or of a response schema, and the envelope around it is
// then counted in that payload's skip rather than called judged. The envelope
// is listed before its payloads in every case, since what it is counted as
// has to wait for them whatever order the types are read in.
func TestTypedShapeCheck_AnEnvelopeAroundASkippedPayload_TakesItsSkip(t *testing.T) {
	const pkg = "internal/tools/milestones"
	// Named to sort before IssueItem, so the case with one of each reads the
	// compared payload first and has to keep looking.
	converted := structs.OutputPairing{Package: "milestones", MCPType: "AnotherItem", SDKType: "Issue"}
	cases := []struct {
		name       string
		pairings   structs.Pairings
		operations map[string]response
		methods    map[string][]sdkRoute
		wraps      []string
		want       SkippedTypes
		envelopes  []string
	}{
		{
			name:       "a payload whose method reaches no route",
			pairings:   structs.Pairings{ClientGoDir: "/client-go", Projections: []structs.ProjectionPairing{issueItemProjection}},
			operations: milestoneIssueOperations,
			wraps:      []string{"IssueItem"},
			want:       SkippedTypes{NoRoute: []string{"milestones.IssueItem", "milestones.MilestoneIssuesOutput"}},
		},
		{
			name:     "a payload whose routes the record gives no response",
			pairings: structs.Pairings{ClientGoDir: "/client-go", Outputs: []structs.OutputPairing{converted}},
			wraps:    []string{"AnotherItem"},
			want:     SkippedTypes{NoSchema: []string{"milestones.AnotherItem", "milestones.MilestoneIssuesOutput"}},
		},
		{
			name:       "a compared payload beside one with no route",
			pairings:   structs.Pairings{ClientGoDir: "/client-go", Outputs: []structs.OutputPairing{converted}, Projections: []structs.ProjectionPairing{issueItemProjection}},
			operations: milestoneIssueOperations,
			wraps:      []string{"AnotherItem", "IssueItem"},
			want:       SkippedTypes{NoRoute: []string{"milestones.IssueItem", "milestones.MilestoneIssuesOutput"}},
		},
		{
			name:       "every payload compared",
			pairings:   structs.Pairings{ClientGoDir: "/client-go", Outputs: []structs.OutputPairing{converted}, Projections: []structs.ProjectionPairing{issueItemProjection}},
			operations: milestoneIssueOperations,
			methods:    milestoneIssueMethodRoutes,
			wraps:      []string{"AnotherItem", "IssueItem"},
			envelopes:  []string{"milestones.MilestoneIssuesOutput"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stubTypeGrainInputs(t, testCase.pairings, nil, milestoneIssueRoutes)
			stubMethodRoutes(t, testCase.methods)

			published := []publishedType{{Package: pkg, Name: "MilestoneIssuesOutput", Fields: []string{"issues"}, Wraps: testCase.wraps}}
			for _, payload := range testCase.wraps {
				published = append(published, publishedType{Package: pkg, Name: payload, Fields: []string{"id"}, Inner: true, Payload: true})
			}
			check := typedCheckOf("", testCase.operations, published)

			if !reflect.DeepEqual(check.Skipped, testCase.want) || !slices.Equal(check.Envelopes, testCase.envelopes) {
				t.Errorf("skipped = %+v, envelopes = %v; want %+v and %v", check.Skipped, check.Envelopes, testCase.want, testCase.envelopes)
			}
			if check.SkippedNoRoute != len(testCase.want.NoRoute) || check.SkippedNoSchema != len(testCase.want.NoSchema) || check.SkippedNoPairing != 0 {
				t.Errorf("counters = %d no route, %d no schema, %d no pairing; want them to agree with the lists", check.SkippedNoRoute, check.SkippedNoSchema, check.SkippedNoPairing)
			}
		})
	}
}

// TestTypedShapeCheck_APayloadThatIsItselfPackaging_IsCountedUnderItsEnvelope
// verifies that a payload is never counted on its own name, even when it is a
// top-level type, one embedded rather than named as a field, that packages a
// paired payload of its own. The response is counted under the outermost
// envelope, whose own payload carries no pairing, so it is the one type named
// among the skips; listing the middle one among the envelopes as well would
// count the one response twice.
func TestTypedShapeCheck_APayloadThatIsItselfPackaging_IsCountedUnderItsEnvelope(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs:     []structs.OutputPairing{{Package: "milestones", MCPType: "IssueItem", SDKType: "Issue"}},
	}, nil, milestoneIssueRoutes)

	check := typedCheckOf("", milestoneIssueOperations, []publishedType{
		{Package: "internal/tools/milestones", Name: "GetOutput", Fields: []string{"id"}, Wraps: []string{"DetailOutput"}},
		{Package: "internal/tools/milestones", Name: "DetailOutput", Fields: []string{"id"}, Payload: true, Wraps: []string{"IssueItem"}},
		{Package: "internal/tools/milestones", Name: "IssueItem", Fields: []string{"id"}, Inner: true, Payload: true},
	})

	if check.Compared != 1 || len(check.Envelopes) != 0 || !slices.Equal(check.Skipped.NoPairing, []string{"milestones.GetOutput"}) {
		t.Errorf("check = %+v, want the item compared and the response counted once, under the outer envelope", check)
	}
}

// TestWrapsOnlyPaired_EveryPayload_MustBePaired verifies that one paired
// payload among several does not make the envelope judged: the others are
// responses nothing reads, and they are only counted if the envelope is.
func TestWrapsOnlyPaired_EveryPayload_MustBePaired(t *testing.T) {
	converted := map[[2]string][]string{{"projects", "Output"}: {"Project"}}
	projected := map[[2]string][]structs.ProjectionPairing{{"projects", "BasicOutput"}: {{SDKType: "BasicProject"}}}
	cases := []struct {
		name  string
		wraps []string
		want  bool
	}{
		{name: "a converter's payload", wraps: []string{"Output"}, want: true},
		{name: "a projection's payload", wraps: []string{"BasicOutput"}, want: true},
		{name: "both, one of each", wraps: []string{"BasicOutput", "Output"}, want: true},
		{name: "one of them unpaired", wraps: []string{"Output", "StrangerOutput"}},
		{name: "no payload at all"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := publishedType{Package: "internal/tools/projects", Name: "ListOutput", Wraps: testCase.wraps}
			if got := wrapsOnlyPaired(candidate, converted, projected); got != testCase.want {
				t.Errorf("wrapsOnlyPaired(%v) = %v, want %v", testCase.wraps, got, testCase.want)
			}
		})
	}
}

// TestProjectedSDKTypes_EachStruct_IsNamedOnceInOrder verifies how a type
// built from several structs names them on a finding: once each and sorted,
// as a converter pairing's list is, so the two spellings read alike.
func TestProjectedSDKTypes_EachStruct_IsNamedOnceInOrder(t *testing.T) {
	got := projectedSDKTypes([]structs.ProjectionPairing{{SDKType: "Project"}, {SDKType: "BasicProject"}, {SDKType: "Project"}})
	if want := []string{"BasicProject", "Project"}; !slices.Equal(got, want) {
		t.Errorf("projectedSDKTypes() = %v, want %v", got, want)
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
