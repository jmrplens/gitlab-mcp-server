// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import (
	"slices"
	"strings"
	"testing"
)

// TestRouteMatches_ADeclaredRoute_NamesTheRoutesItSpells verifies the route
// a declaration names: the same verb, as many segments, and each segment the
// same text or a wildcard standing for any one.
func TestRouteMatches_ADeclaredRoute_NamesTheRoutesItSpells(t *testing.T) {
	cases := []struct {
		declared, route string
		want            bool
	}{
		{declared: "PUT /projects/:id/integrations/*", route: "PUT /projects/:id/integrations/slack", want: true},
		{declared: "GET /groups/:id/(-/)search", route: "GET /groups/:id/(-/)search", want: true},
		{declared: "PUT /projects/:id/integrations/*", route: "POST /projects/:id/integrations/slack"},
		{declared: "PUT /projects/:id/integrations/*", route: "PUT /projects/:id/integrations/slack/extra"},
		{declared: "PUT /projects/:id/integrations/*", route: "PUT /groups/:id/integrations/slack"},
	}
	for _, tc := range cases {
		t.Run(tc.declared+" "+tc.route, func(t *testing.T) {
			if got := routeMatches(tc.declared, tc.route); got != tc.want {
				t.Errorf("routeMatches = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestAnswers_ACategory_AnswersOnlyTheClassesItMay verifies which findings a
// declaration answers: on its route and its param, or every param of the
// route; an upstream one an unmodeled param alone, so it goes stale when
// client-go models the param; a map input one a param an option struct
// models or a free map carries, and never one nothing can send; the others
// any class.
func TestAnswers_ACategory_AnswersOnlyTheClassesItMay(t *testing.T) {
	finding := func(param, class string) Finding {
		return Finding{Route: "GET /things", Param: param, Class: class}
	}
	cases := []struct {
		name        string
		declaration paramDeclaration
		finding     Finding
		want        bool
	}{
		{name: "upstream on an unmodeled param", declaration: paramDeclaration{Route: "GET /things", Param: "p", Category: categoryUpstream}, finding: finding("p", ClassUnmodeled), want: true},
		{name: "upstream on a modeled param", declaration: paramDeclaration{Route: "GET /things", Param: "p", Category: categoryUpstream}, finding: finding("p", ClassNotOffered)},
		{name: "map input on a modeled param", declaration: paramDeclaration{Route: "GET /things", Param: anyParam, Category: categoryMapInput}, finding: finding("p", ClassNotOffered), want: true},
		{name: "map input on a free map", declaration: paramDeclaration{Route: "GET /things", Param: anyParam, Category: categoryMapInput}, finding: finding("p", ClassUnjudged), want: true},
		{name: "map input on an unmodeled param", declaration: paramDeclaration{Route: "GET /things", Param: anyParam, Category: categoryMapInput}, finding: finding("p", ClassUnmodeled)},
		{name: "any class otherwise", declaration: paramDeclaration{Route: "GET /things", Param: "p", Category: categoryDeclaredNotApplied}, finding: finding("p", ClassOfferedUnrecorded), want: true},
		{name: "another param", declaration: paramDeclaration{Route: "GET /things", Param: "q", Category: categoryDeclaredNotApplied}, finding: finding("p", ClassUnmodeled)},
		{name: "another route", declaration: paramDeclaration{Route: "GET /other", Param: "p", Category: categoryDeclaredNotApplied}, finding: finding("p", ClassUnmodeled)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.declaration.answers(tc.finding); got != tc.want {
				t.Errorf("answers = %t, want %t", got, tc.want)
			}
		})
	}
	declared := []paramDeclaration{{Route: "GET /things", Param: "q", Category: categoryUpstream}, cases[0].declaration}
	if index, answered := answer(declared, finding("p", ClassUnmodeled)); !answered || index != 1 {
		t.Errorf("answer = %d, %t; want the first declaration that answers, at 1", index, answered)
	}
	if _, answered := answer(declared, finding("z", ClassUnmodeled)); answered {
		t.Error("answer found a declaration for a param none names")
	}
}

// TestStaleDeclarations_EveryWayADeclarationFails_IsNamed verifies the gate:
// a declaration that answers nothing, names a category the table does not
// define, gives no reason, or, for an upstream one, names a row the register
// does not hold, each named once and in order, while a used one of another
// category needs no row.
func TestStaleDeclarations_EveryWayADeclarationFails_IsNamed(t *testing.T) {
	declared := []paramDeclaration{
		{Route: "GET /a", Param: "used", Category: categoryUpstream, Reason: "r", Row: 7},
		{Route: "GET /a", Param: "unused", Category: categoryWebhookPayload, Reason: "r"},
		{Route: "GET /a", Param: "unknown", Category: "invented", Reason: "r"},
		{Route: "GET /a", Param: "silent", Category: categoryBranchNotTaken, Reason: " "},
		{Route: "GET /a", Param: "norow", Category: categoryUpstream, Reason: "r", Row: 8},
		{Route: "GET /a", Param: "rowless", Category: categoryMapInput, Reason: "r"},
	}
	used := []bool{true, false, true, true, true, true}
	stale := staleDeclarations(declared, used, map[int]string{7: "row seven"})
	want := []string{
		"GET /a norow (upstream): names row 8, which the upstream register does not hold",
		"GET /a silent (branch-not-taken): gives no reason",
		"GET /a unknown (invented): names no category this table defines",
		"GET /a unused (webhook-payload): answers no finding",
	}
	if !slices.Equal(stale, want) {
		t.Errorf("stale =\n%s\nwant\n%s", strings.Join(stale, "\n"), strings.Join(want, "\n"))
	}
}

// TestParamDeclarations_EveryEntryNamesACategoryAReasonAndOneParam verifies
// the table's own shape: each entry a known category with a reason, an
// upstream one a row, no two entries naming one param of one route, which
// would leave the second answering nothing, and every category the table
// defines named by some entry, so the vocabulary holds no reason nothing
// is declared for.
func TestParamDeclarations_EveryEntryNamesACategoryAReasonAndOneParam(t *testing.T) {
	seen := map[string]bool{}
	named := map[string]bool{}
	declared := paramDeclarations()
	for _, d := range declared {
		named[d.Category] = true
	}
	for _, category := range knownCategories {
		t.Run("category "+category, func(t *testing.T) {
			if !named[category] {
				t.Errorf("category %q is named by no declaration", category)
			}
		})
	}
	for _, d := range declared {
		key := d.Route + " " + d.Param
		t.Run(key, func(t *testing.T) {
			if !slices.Contains(knownCategories, d.Category) || strings.TrimSpace(d.Reason) == "" {
				t.Errorf("declaration = %+v, want a known category and a reason", d)
			}
			if (d.Category == categoryUpstream) != (d.Row > 0) {
				t.Errorf("declaration = %+v, want a row on an upstream declaration and on no other", d)
			}
			if seen[key] {
				t.Errorf("%s is declared twice", key)
			}
			seen[key] = true
		})
	}
}
