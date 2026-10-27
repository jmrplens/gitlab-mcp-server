// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package paths

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// searchRecord is a record holding the search routes the way GitLab mounts
// them: annotated with no entity, since the scope a caller sends selects it,
// taking that scope, the scoped one under an optional group; the blob entity
// no route names; and a route of another kind for the declarations to be held
// against.
func searchRecord() apilive.Document {
	scope := map[string]apilive.Param{"scope": {Required: true, Type: "String"}, "search": {Required: true, Type: "String"}}
	return apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Entities: map[string]apilive.Entity{
			blobEntity:              {Fields: []apilive.Field{{Name: "basename"}, {Name: "data"}, {Name: "id"}, {Name: "path"}}},
			"API::Entities::Widget": {Fields: []apilive.Field{{Name: "color"}}},
		},
		Routes: []apilive.Route{
			{Method: "GET", Path: apilive.EndpointPrefix + "/search", Params: scope},
			{Method: "GET", Path: apilive.EndpointPrefix + "/projects/:id/(-/)search", Params: scope},
			{Method: "GET", Path: apilive.EndpointPrefix + "/widgets", Entity: "API::Entities::Widget"},
			{Method: "GET", Path: apilive.EndpointPrefix + "/unscoped"},
		},
	}
}

// searchCheckOf runs the type grain against [searchRecord].
func searchCheckOf(published []publishedType) TypedShapeCheck {
	doc := searchRecord()
	return typedShapeCheck("", newOperationIndex(doc), newConditionIndex(doc), published)
}

// blobRoutes are the two endpoints a client-go search method reaches, each
// carrying the entity the scope it sends selects, or none.
func blobRoutes(entity string) []sdkRoute {
	return []sdkRoute{
		{Method: "GET", Path: "/projects/:/-/search", Many: true, Entity: entity},
		{Method: "GET", Path: "/search", Many: true, Entity: entity},
	}
}

// stubScopeDeclarations replaces the two declaration tables of this file for
// one test.
func stubScopeDeclarations(t *testing.T, scoped []scopedEntity, captured []capturedPairing) {
	t.Helper()
	previousScoped, previousCaptured := scopedEntities, capturedPairings
	scopedEntities, capturedPairings = scoped, captured
	t.Cleanup(func() { scopedEntities, capturedPairings = previousScoped, previousCaptured })
}

// TestTypedShapeCheck_ASearchScope_IsHeldToTheEntityTheScopeSelects verifies
// what the scope declarations buy the type grain. GitLab annotates its search
// routes with no entity, so a type built from a search answer used to be
// counted among the types the record gives no schema for, and nothing held
// the code search result to the blob GitLab sends. With the entity the
// method's scope selects carried on its routes, the type is compared against
// that entity on every route the method reaches, the scoped one included
// through the optional group GitLab mounts it under, and both directions are
// reported, each naming the entity it was read on.
func TestTypedShapeCheck_ASearchScope_IsHeldToTheEntityTheScopeSelects(t *testing.T) {
	pairings := structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs:     []structs.OutputPairing{{Package: "search", MCPType: "BlobOutput", SDKType: "Blob", SDKFields: []string{"basename", "data", "id"}}},
	}
	published := []publishedType{{Package: "internal/tools/search", Name: "BlobOutput", Fields: []string{"basename", "data", "id", "invented"}}}

	t.Run("without a declared entity", func(t *testing.T) {
		stubTypeGrainInputs(t, pairings, nil, map[string][]sdkRoute{"Blob": blobRoutes("")})

		check := searchCheckOf(published)

		if check.Compared != 0 || !slices.Equal(check.Skipped.NoSchema, []string{"search.BlobOutput"}) {
			t.Errorf("check = %+v, want the type skipped for a record that names no entity", check)
		}
	})

	t.Run("with the entity the scope selects", func(t *testing.T) {
		stubTypeGrainInputs(t, pairings, nil, map[string][]sdkRoute{"Blob": blobRoutes(blobEntity)})

		check := searchCheckOf(published)

		if check.Compared != 1 {
			t.Fatalf("check = %+v, want the type compared", check)
		}
		wantOperations := []string{
			"GET /projects/:/-/search (collection) as " + blobEntity,
			"GET /search (collection) as " + blobEntity,
		}
		if len(check.Unpublished) != 1 || check.Unpublished[0].Field != "invented" || !slices.Equal(check.Unpublished[0].Operations, wantOperations) {
			t.Errorf("unpublished = %+v, want the invented field, searched on both routes", check.Unpublished)
		}
		if len(check.Unsurfaced) != 1 || check.Unsurfaced[0].Field != "path" || check.Unsurfaced[0].Entity != blobEntity || check.Unsurfaced[0].SDKModels {
			t.Errorf("unsurfaced = %+v, want path, read on the blob and modeled by no SDK field", check.Unsurfaced)
		}
	})
}

// TestTypedShapeCheck_ARouteTheRecordAnnotates_AnswersWithItsOwnEntity
// verifies that a declared entity fills only the gap it is declared for. A
// route the record annotates answers with the entity it names, whatever a
// declaration carries, since the record is GitLab's own word for that route
// and a declaration still pointing there is stale.
func TestTypedShapeCheck_ARouteTheRecordAnnotates_AnswersWithItsOwnEntity(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs:     []structs.OutputPairing{{Package: "widgets", MCPType: "Output", SDKType: "Widget"}},
	}, nil, map[string][]sdkRoute{"Widget": {{Method: "GET", Path: "/widgets", Many: true, Entity: blobEntity}}})

	check := searchCheckOf([]publishedType{{Package: "internal/tools/widgets", Name: "Output", Fields: []string{"color"}}})

	if check.Compared != 1 || len(check.Unsurfaced) != 0 || len(check.Unpublished) != 0 {
		t.Errorf("check = %+v, want the widget held to the entity its route names", check)
	}
}

// TestTypedShapeCheck_AKeyAnAnnotatedRouteSends_IsCreditedToTheEntityItNames
// verifies the order a key is credited in when a struct is answered by search
// and by a route the record annotates. Routes are read in the order their
// names sort, and a search route sorts ahead of most others, so crediting the
// first response to carry a key let a scope declaration take over every key
// of a user from the entity the user's own routes name, and every declaration
// answering such a key went stale. The record is GitLab's word for its route
// and a declaration is ours, so a declared entity is credited only with what
// no annotated route sends.
func TestTypedShapeCheck_AKeyAnAnnotatedRouteSends_IsCreditedToTheEntityItNames(t *testing.T) {
	doc := searchRecord()
	doc.Entities["API::Entities::Gadget"] = apilive.Entity{Fields: []apilive.Field{{Name: "color"}, {Name: "shine"}, {Name: "spin"}}}
	stubTypeGrainInputs(t, structs.Pairings{
		ClientGoDir: "/client-go",
		Outputs:     []structs.OutputPairing{{Package: "widgets", MCPType: "Output", SDKType: "Widget"}},
	}, nil, map[string][]sdkRoute{"Widget": {
		{Method: "GET", Path: "/search", Many: true, Entity: "API::Entities::Gadget"},
		{Method: "GET", Path: "/widgets", Many: true},
	}})

	check := typedShapeCheck("", newOperationIndex(doc), newConditionIndex(doc), []publishedType{{Package: "internal/tools/widgets", Name: "Output", Fields: []string{"spin"}}})

	credited := map[string]string{}
	for _, finding := range check.Unsurfaced {
		credited[finding.Field] = finding.Entity
	}
	want := map[string]string{"color": "API::Entities::Widget", "shine": "API::Entities::Gadget"}
	if !maps.Equal(credited, want) {
		t.Errorf("credited = %v, want %v", credited, want)
	}
}

// TestTypedShapeCheck_ACapturedType_IsHeldToTheMethodsItReads verifies the
// pairing a type read off the captured response (ADR-0021) is given. No
// converter fills it, so nothing paired it and it was counted among the
// types with no pairing; declared, it is judged against the routes of the
// methods whose answer it reads, listed apart from the projections, and every
// key it leaves out is one no client-go struct models, which is what reading
// the capture was for.
func TestTypedShapeCheck_ACapturedType_IsHeldToTheMethodsItReads(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{ClientGoDir: "/client-go"}, nil, nil)
	stubMethodRoutes(t, map[string][]sdkRoute{"Search.WikiBlobsByProject": blobRoutes(blobEntity)[:1]})
	stubScopeDeclarations(t, nil, []capturedPairing{{Package: "search", Type: "WikiBlobOutput", Methods: []string{"Search.WikiBlobsByProject"}}})

	check := searchCheckOf([]publishedType{
		{Package: "internal/tools/search", Name: "WikiBlobOutput", Fields: []string{"basename", "data", "id"}, Inner: true, Payload: true},
		{Package: "internal/tools/search", Name: "WikiOutput", Fields: []string{"pagination", "wiki_blobs"}, Wraps: []string{"WikiBlobOutput"}},
	})

	if check.Compared != 1 || check.ComparedInner != 1 || !slices.Equal(check.Captured, []string{"search.WikiBlobOutput"}) || len(check.Projections) != 0 {
		t.Fatalf("check = %+v, want the captured type compared and listed apart", check)
	}
	if !slices.Equal(check.Envelopes, []string{"search.WikiOutput"}) || check.SkippedNoPairing != 0 {
		t.Errorf("envelopes = %v, no-pairing = %d; want the list around the row judged through it", check.Envelopes, check.SkippedNoPairing)
	}
	if len(check.Unsurfaced) != 1 || check.Unsurfaced[0].Field != "path" || check.Unsurfaced[0].SDKType != capturedSDKType || check.Unsurfaced[0].SDKModels {
		t.Errorf("unsurfaced = %+v, want path, against the captured response and modeled by no SDK struct", check.Unsurfaced)
	}
}

// TestWithCaptured_ATypeAConverterPairs_KeepsItsConverter verifies that a
// captured pairing fills only the gap it is declared for, as a projection
// does: a type a converter pairs is judged through its converter, and only a
// type nothing pairs is given the captured methods.
func TestWithCaptured_ATypeAConverterPairs_KeepsItsConverter(t *testing.T) {
	stubScopeDeclarations(t, nil, []capturedPairing{
		{Package: "search", Type: "PairedOutput", Methods: []string{"Search.Blobs"}},
		{Package: "search", Type: "CapturedOutput", Methods: []string{"Search.Blobs"}},
	})

	projected := withCaptured(map[[2]string][]structs.ProjectionPairing{}, map[[2]string][]string{{"search", "PairedOutput"}: {"Blob"}})

	if got, ok := projected[[2]string{"search", "PairedOutput"}]; ok {
		t.Errorf("paired type given %+v, want it left to its converter", got)
	}
	if got := projected[[2]string{"search", "CapturedOutput"}]; !isCaptured(got) || !slices.Equal(got[0].Methods, []string{"Search.Blobs"}) {
		t.Errorf("captured type given %+v, want the captured methods", got)
	}
	if isCaptured(nil) || isCaptured([]structs.ProjectionPairing{{SDKType: "Blob"}}) {
		t.Error("isCaptured reports a projection or no pairing as captured")
	}
}

// TestScopeDeclarationProblems_WhatNoLongerDescribesTheTree_IsReported
// verifies that the two tables are held to the record and to client-go like
// every declaration table here. A declaration that no longer describes the
// tree would carry an entity onto a route that names its own, or pair a type
// that is gone, and either would be a claim nothing checks.
func TestScopeDeclarationProblems_WhatNoLongerDescribesTheTree_IsReported(t *testing.T) {
	methodRoutes := map[string][]sdkRoute{
		"Search.Blobs":   blobRoutes("")[1:],
		"Search.Widgets": {{Method: "GET", Path: "/widgets", Many: true}},
		"Search.Plain":   {{Method: "GET", Path: "/unscoped", Many: true}},
		"Search.Lost":    {{Method: "GET", Path: "/nowhere", Many: true}},
	}
	published := []publishedType{{Package: "internal/tools/search", Name: "WikiBlobOutput"}}
	scoped := []scopedEntity{
		{Method: "Search.Blobs", Scope: "blobs", Entity: blobEntity},
		{Method: "Search.Gone", Scope: "blobs", Entity: blobEntity},
		{Method: "Search.Blobs", Scope: "blobs", Entity: "API::Entities::Missing"},
		{Method: "Search.Widgets", Scope: "widgets", Entity: blobEntity},
		{Method: "Search.Plain", Scope: "plain", Entity: blobEntity},
		{Method: "Search.Lost", Scope: "lost", Entity: blobEntity},
	}
	captured := []capturedPairing{
		{Package: "search", Type: "WikiBlobOutput", Methods: []string{"Search.Blobs"}},
		{Package: "search", Type: "GoneOutput", Methods: []string{"Search.Blobs"}},
		{Package: "search", Type: "WikiBlobOutput", Methods: []string{"Search.Gone"}},
	}

	got := scopeDeclarationProblems(scoped, captured, methodRoutes, newOperationIndex(searchRecord()), published)

	want := []string{
		"captured pairing search.GoneOutput names a type the package no longer publishes",
		"captured pairing search.WikiBlobOutput names Search.Gone, which client-go has no route for",
		"scope declaration Search.Blobs names API::Entities::Missing, which the record holds no entity for",
		"scope declaration Search.Gone names a method client-go has no route for",
		"scope declaration Search.Lost reaches GET /nowhere, which the record does not mount",
		"scope declaration Search.Plain reaches GET /unscoped, which takes no scope",
		"scope declaration Search.Widgets reaches GET /widgets, which the record now annotates with API::Entities::Widget",
	}
	if !slices.Equal(got, want) {
		t.Errorf("problems =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestTypedShapeCheck_StaleScopeDeclarations_AreReportedOnlyOnceItRuns
// verifies that the problems reach the stale list the gate reads, and only
// from a run that compared something, for the reason
// [TypedShapeCheck.staleDeclarations] records.
func TestTypedShapeCheck_StaleScopeDeclarations_AreReportedOnlyOnceItRuns(t *testing.T) {
	stubTypeGrainInputs(t, structs.Pairings{ClientGoDir: "/client-go"}, nil, nil)
	stubScopeDeclarations(t, []scopedEntity{{Method: "Search.Gone", Scope: "blobs", Entity: blobEntity}}, nil)

	check := searchCheckOf(nil)

	if want := []string{"scope declaration Search.Gone names a method client-go has no route for"}; !slices.Equal(check.UnusedScopeDeclarations, want) || !slices.Equal(check.staleDeclarations(), want) {
		t.Errorf("unused = %v, stale = %v; want the one problem in both", check.UnusedScopeDeclarations, check.staleDeclarations())
	}
	if stale := (TypedShapeCheck{UnusedScopeDeclarations: check.UnusedScopeDeclarations}).staleDeclarations(); stale != nil {
		t.Errorf("stale = %v from a check that did not run, want nothing", stale)
	}
}

// TestScopeTables_EveryEntry_IsWellFormed holds the real tables to what a
// reader of one needs: every scope declaration names a method, the scope it
// sends and an entity GitLab presents, once; every captured pairing names a
// type and the methods it reads, every one of them declared with the entity
// its scope selects, so a captured type is never paired with a route the
// type grain has no entity for.
func TestScopeTables_EveryEntry_IsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, declaration := range scopedEntities {
		if declaration.Method == "" || declaration.Scope == "" || !strings.HasPrefix(declaration.Entity, "API::Entities::") {
			t.Errorf("scope declaration %+v is missing its method, scope or entity", declaration)
		}
		if seen[declaration.Method] {
			t.Errorf("scope declaration for %s is written twice", declaration.Method)
		}
		seen[declaration.Method] = true
	}
	for _, pairing := range capturedPairings {
		if pairing.Package == "" || pairing.Type == "" || len(pairing.Methods) == 0 {
			t.Errorf("captured pairing %+v is missing its type or methods", pairing)
		}
		for _, method := range pairing.Methods {
			if scopedEntityOf(method) == "" {
				t.Errorf("captured pairing %s.%s reads %s, which no scope declaration names an entity for", pairing.Package, pairing.Type, method)
			}
		}
	}
}
