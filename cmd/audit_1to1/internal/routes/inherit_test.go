// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
)

// decisionInput is a tree whose client-go methods are decided every way the
// tables decide one: called by an action, in acceptedMissingMethods, replaced
// by a waits entry or by one that builds the URL, by the declaration of their
// service or by a waits entry replacing the whole service, and by nothing. Its
// COVERED_GRAPHQL decisions name a GraphQL action, none, a REST action, and
// a GraphQL action beside one the catalog does not hold.
func decisionInput() judgeInput {
	return judgeInput{
		record: actionrequests.Record{Actions: []actionrequests.RecordAction{
			{ID: "widget.list", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindREST, Route: "GET /widgets", SDKMethods: []string{"Widgets.ListWidgets"}}}},
			{ID: "widget.graph", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindGraphQL, Operation: "query widget"}}},
			{ID: "widget.rest", Requests: []actionrequests.RecordRequest{{Kind: actionrequests.KindREST, Route: "GET /widgets/rest"}}},
		}},
		catalog: map[string]bool{"widget.list": true, "widget.graph": true, "widget.rest": true},
		accepted: map[string]string{
			"Widgets.GetWidgetGraph":  "COVERED_GRAPHQL. Widgets over GraphQL",
			"Widgets.GetWidgetBare":   "COVERED_GRAPHQL. No action named",
			"Widgets.GetWidgetREST":   "COVERED_GRAPHQL. A REST action named",
			"Widgets.GetWidgetMixed":  "COVERED_GRAPHQL. One action of two missing",
			"Widgets.GetWidgetOther":  "COVERED_GENERIC. Another route",
			"Widgets.GetWidgetBinary": "INTENTIONAL_SKIP_BINARY",
		},
		graphql: map[string][]string{
			"Widgets.GetWidgetGraph": {"widget.graph"},
			"Widgets.GetWidgetREST":  {"widget.rest"},
			"Widgets.GetWidgetMixed": {"widget.graph", "widget.gone"},
			"Cranks.ListCranks":      {"widget.graph"},
		},
		waits: []waits.Entry{
			{Row: 6, Category: waits.CategoryUpstream, Replaces: []string{"Widgets.SetWidget", "Cogs"}, Reason: "the body by hand"},
			{Category: waits.CategoryURLBuilt, Replaces: []string{"Widgets.WidgetArchive"}, Reason: "the URL"},
			{Category: waits.CategoryProbe, Replaces: []string{"Widgets.SetWidget"}, Reason: "a second entry, which loses"},
		},
		services: map[string]sdk.ServiceDeclaration{
			"Gadgets": {Category: adjudicationSuperseded, Reason: "GitLab replaced it"},
			"Cranks":  {Category: adjudicationGraphQL, Reason: "Cranks over GraphQL"},
		},
	}
}

// TestAdjudicate_EachTable_DecidesAMethodInItsOrder verifies the decision
// found for a method, and whether it carries onto a route no request sends: a
// COVERED_GRAPHQL decision, of the method or of its service, only with the
// GraphQL actions that stand for it, each in the catalog and sending GraphQL.
func TestAdjudicate_EachTable_DecidesAMethodInItsOrder(t *testing.T) {
	j := newJudge(decisionInput())
	cases := []Adjudication{
		{Method: "Widgets.ListWidgets", Category: adjudicationCalled},
		{Method: "Widgets.GetWidgetGraph", Category: adjudicationGraphQL, Reason: "Widgets over GraphQL", Actions: []string{"widget.graph"}, Carries: true},
		{Method: "Widgets.GetWidgetBare", Category: adjudicationGraphQL, Reason: "No action named"},
		{Method: "Widgets.GetWidgetREST", Category: adjudicationGraphQL, Reason: "A REST action named", Actions: []string{"widget.rest"}},
		{Method: "Widgets.GetWidgetMixed", Category: adjudicationGraphQL, Reason: "One action of two missing", Actions: []string{"widget.graph", "widget.gone"}},
		{Method: "Widgets.GetWidgetOther", Category: "COVERED_GENERIC", Reason: "Another route"},
		{Method: "Widgets.GetWidgetBinary", Category: "INTENTIONAL_SKIP_BINARY"},
		{Method: "Widgets.SetWidget", Category: adjudicationRaw, Reason: "row 6: the body by hand"},
		{Method: "Widgets.WidgetArchive", Category: adjudicationURLBuilt, Reason: "url-built: the URL", Carries: true},
		{Method: "Gadgets.ListGadgets", Category: adjudicationSuperseded, Reason: "GitLab replaced it", Carries: true},
		{Method: "Cranks.ListCranks", Category: adjudicationGraphQL, Reason: "Cranks over GraphQL", Actions: []string{"widget.graph"}, Carries: true},
		{Method: "Cogs.ListCogs", Category: adjudicationRaw, Reason: "row 6: the body by hand"},
		{Method: "Sprockets.ListSprockets", Category: adjudicationNone},
	}
	for _, want := range cases {
		t.Run(want.Method, func(t *testing.T) {
			if got := j.adjudicate(want.Method); !reflect.DeepEqual(got, want) {
				t.Errorf("adjudicate(%s) = %+v, want %+v", want.Method, got, want)
			}
		})
	}
	asked := sortedKeys(j.graphqlAsked)
	want := []string{"Cranks.ListCranks", "Widgets.GetWidgetBare", "Widgets.GetWidgetGraph", "Widgets.GetWidgetMixed", "Widgets.GetWidgetREST"}
	if !slices.Equal(asked, want) {
		t.Errorf("GraphQL actions asked of %v, want the COVERED_GRAPHQL methods %v", asked, want)
	}
}

// TestGraphQLProblems_EachWayARowFails_IsNamed verifies every problem of a
// graphqlActions row is reported: one that names no action, an action the
// catalog does not hold or that sends no GraphQL, and a row no decision was
// read with; a row that lacks nothing and was read reports nothing.
func TestGraphQLProblems_EachWayARowFails_IsNamed(t *testing.T) {
	in := decisionInput()
	in.graphql = map[string][]string{
		"Widgets.GetWidgetGraph": {"widget.graph"},
		"Widgets.GetWidgetBad":   {"widget.rest", "widget.gone"},
		"Widgets.GetWidgetEmpty": {},
	}
	j := newJudge(in)
	j.graphqlAsked["Widgets.GetWidgetGraph"], j.graphqlAsked["Widgets.GetWidgetBad"] = true, true
	got := j.graphqlProblems()
	slices.Sort(got)
	want := []string{
		"graphql actions of Widgets.GetWidgetBad: it names widget.gone, which sends no GraphQL request",
		"graphql actions of Widgets.GetWidgetBad: it names widget.gone, which the catalog does not hold",
		"graphql actions of Widgets.GetWidgetBad: it names widget.rest, which sends no GraphQL request",
		"graphql actions of Widgets.GetWidgetEmpty answer no route: no route no action reaches is sent by it under a COVERED_GRAPHQL decision",
		"graphql actions of Widgets.GetWidgetEmpty name none",
	}
	if !slices.Equal(got, want) {
		t.Errorf("graphqlProblems = %q, want %q", got, want)
	}
}

// TestGraphQLActions_EveryRow_NamesAnActionForAMethod holds the table as
// written to its shape, whatever the catalog says: a "Service.Method" key and
// at least one action per row.
func TestGraphQLActions_EveryRow_NamesAnActionForAMethod(t *testing.T) {
	for method, actions := range graphqlActions {
		t.Run(method, func(t *testing.T) {
			service, name, cut := strings.Cut(method, ".")
			if !cut || service == "" || name == "" || len(actions) == 0 {
				t.Errorf("graphqlActions[%q] = %v, want a Service.Method key and an action", method, actions)
			}
		})
	}
}

// TestInherits_TheDecisionsOfARoutesMethods_DecideItTogether verifies a
// route is inherited only when every method's decision carries, contradicted
// when one says the route is sent, and left to the family rows otherwise,
// the decisions kept on the verdict every way; a route no method sends is not
// asked.
func TestInherits_TheDecisionsOfARoutesMethods_DecideItTogether(t *testing.T) {
	j := newJudge(decisionInput())
	cases := []struct {
		name      string
		methods   []string
		want      bool
		wantClass string
	}{
		{name: "no method", want: false},
		{name: "every decision carries", methods: []string{"Gadgets.ListGadgets", "Widgets.GetWidgetGraph"}, want: true, wantClass: ClassInherited},
		{name: "one says the route is sent", methods: []string{"Widgets.SetWidget", "Widgets.GetWidgetGraph"}, want: true, wantClass: ClassContradicted},
		{name: "one does not carry", methods: []string{"Widgets.GetWidgetOther", "Widgets.WidgetArchive"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := &Route{SDKMethods: tc.methods}
			got := j.inherits(verdict)
			if got != tc.want || verdict.Class != tc.wantClass || len(verdict.Adjudications) != len(tc.methods) {
				t.Errorf("inherits = %t (%+v), want %t and class %q with a decision per method", got, verdict, tc.want, tc.wantClass)
			}
			for index, decision := range verdict.Adjudications {
				if decision.Method != tc.methods[index] {
					t.Errorf("decision %d is of %s, want %s", index, decision.Method, tc.methods[index])
				}
			}
		})
	}
}

// TestCarryingAdjudications_OnlyWhatHoldsWithoutARequest_Carries pins the
// two sets the inheritance rests on, so a category moved between them is a
// change someone reads.
func TestCarryingAdjudications_OnlyWhatHoldsWithoutARequest_Carries(t *testing.T) {
	carrying := sortedKeys(carryingAdjudications)
	if want := []string{adjudicationGraphQL, adjudicationSuperseded, adjudicationUnwrapped, adjudicationURLBuilt}; !slices.Equal(carrying, sortedUnique(want)) {
		t.Errorf("carrying = %v, want %v", carrying, want)
	}
	if contradicting := sortedKeys(contradictingAdjudications); !slices.Equal(contradicting, []string{adjudicationRaw, adjudicationVariant}) {
		t.Errorf("contradicting = %v, want COVERED_RAW and COVERED_VARIANT", contradicting)
	}
}
