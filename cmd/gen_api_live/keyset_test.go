// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// keysetSite is the keyset site route i of a fixture carries: asked by the
// cursor the probe of CursorPager names, and located on a line of its own.
func keysetSite(i int) apilive.KeysetSite {
	return apilive.KeysetSite{
		Paginator: apilive.PaginatorStrategies, File: "lib/api/fixture.rb", Line: 100 + i,
		Text: "present paginate_with_strategies(records)", Cursor: []string{"cursor"},
	}
}

// wholePaginators is the probe of GitLab's keyset paginators as 19.5.0-ee
// answered it.
func wholePaginators() map[string][]string {
	return map[string][]string{
		apilive.ProbedCursorPager:       {"cursor"},
		apilive.ProbedPager:             {"id_after", "id_before"},
		apilive.ProbedGitalyKeysetPager: {"page_token"},
		apilive.ProbedCursorHeaders:     {"cursor"},
	}
}

// The fixtures below are edited as JSON, the form the container leaves an
// introspection in and the form the gate reads a record in, rather than as
// the types this package decodes them into. That keeps them meaningful on a
// build that has no keyset field at all: there the generator drops the field
// and the gate reads the record as a version it was not written for, and the
// assertions below say so, rather than the fixtures failing to compile.

// keysetFixtureRoutes is how many routes markKeyset marks, above the floor so
// the floor is not what a case about another rule trips on.
const keysetFixtureRoutes = 40

// asJSON is v as the generic value encoding/json decodes it into.
func asJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode the fixture: %v", err)
	}
	var generic map[string]any
	if decodeErr := json.Unmarshal(encoded, &generic); decodeErr != nil {
		t.Fatalf("decode the fixture: %v", decodeErr)
	}
	return generic
}

// writeJSONFile writes a generic document to path.
func writeJSONFile(t *testing.T, path string, doc map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode the fixture: %v", err)
	}
	if writeErr := os.WriteFile(path, encoded, 0o600); writeErr != nil {
		t.Fatalf("write the fixture: %v", writeErr)
	}
}

// readJSONFile reads a generic document from path.
func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc map[string]any
	if decodeErr := json.Unmarshal(raw, &doc); decodeErr != nil {
		t.Fatalf("decode %s: %v", path, decodeErr)
	}
	return doc
}

// jsonRoute is route i of a generic document.
func jsonRoute(doc map[string]any, i int) map[string]any {
	return doc["routes"].([]any)[i].(map[string]any)
}

// markKeyset gives a generic introspection or record the probe of the
// paginators and marks its first keysetFixtureRoutes routes as paging by
// keyset, then counts them into the source block when there is one.
func markKeyset(t *testing.T, doc map[string]any) {
	t.Helper()
	for i := range keysetFixtureRoutes {
		jsonRoute(doc, i)["keyset"] = []any{map[string]any{
			"paginator": "paginate_with_strategies", "file": "lib/api/fixture.rb", "line": float64(100 + i),
			"cursor": []any{"cursor"},
		}}
	}
	doc["keyset_paginators"] = map[string]any{
		"cursor_pager": []any{"cursor"}, "pager": []any{"id_after", "id_before"},
		"gitaly_keyset_pager": []any{"page_token"}, "cursor_headers": []any{"cursor"},
	}
	recountKeyset(doc)
}

// recountKeyset sets a generic record's keyset_routes to what it holds.
func recountKeyset(doc map[string]any) {
	source, ok := doc["source"].(map[string]any)
	if !ok {
		return
	}
	marked := 0
	for _, route := range doc["routes"].([]any) {
		if _, has := route.(map[string]any)["keyset"]; has {
			marked++
		}
	}
	source["keyset_routes"] = float64(marked)
}

// TestRunGenerate_ARouteCarryingAKeysetPaginator_ComesOutMarked verifies the
// generator half of schema version 5: an introspection whose route carries a
// keyset site writes a record in which that route carries it, every field of
// it, beside the probe of the paginators and the count.
//
// The site is 19.5.0's commit list behind its flag, so its cursor and the
// flag's default travel, and a route the introspection did not mark comes out
// unmarked rather than as anything a reader would take for keyset. Its via is
// added: the real site sits in the route's own block and names no helper, and
// without one the case could not show that field traveling too.
func TestRunGenerate_ARouteCarryingAKeysetPaginator_ComesOutMarked(t *testing.T) {
	dump := asJSON(t, wholeEnough())
	markKeyset(t, dump)
	site := asJSON(t, map[string]any{
		"paginator": "gitaly_keyset_pager", "file": "lib/api/commits.rb", "line": 268,
		"text":   "commits = Gitlab::Pagination::GitalyKeysetPager.new(self, user_project).paginate(finder)",
		"via":    []string{"present_commits"},
		"cursor": []string{"page_token"},
		"flags":  []any{map[string]any{"name": "commits_keyset_pagination", "default_enabled": false}},
	})
	jsonRoute(dump, 0)["keyset"] = []any{site}
	dumpPath := filepath.Join(t.TempDir(), "introspect.json")
	writeJSONFile(t, dumpPath, dump)
	dir := t.TempDir()

	if err := runGenerate(dir, dumpFrom{path: dumpPath}, "gitlab/gitlab-ee:19.5.0-ee.0", false); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}

	record := readJSONFile(t, apilive.Path(dir))
	t.Run("the route keeps its keyset site, every field of it", func(t *testing.T) {
		if got, want := jsonRoute(record, 0)["keyset"], []any{site}; !reflect.DeepEqual(got, want) {
			t.Errorf("keyset = %v, want %v", got, want)
		}
	})
	t.Run("a route the introspection did not mark comes out unmarked", func(t *testing.T) {
		if got, has := jsonRoute(record, keysetFixtureRoutes)["keyset"]; has {
			t.Errorf("keyset = %v, want none", got)
		}
	})
	t.Run("the probe of the paginators is carried over", func(t *testing.T) {
		if got, want := record["keyset_paginators"], dump["keyset_paginators"]; !reflect.DeepEqual(got, want) {
			t.Errorf("keyset_paginators = %v, want %v", got, want)
		}
	})
	t.Run("the source block counts the routes reaching a keyset paginator", func(t *testing.T) {
		source, _ := record["source"].(map[string]any)
		if got := source["keyset_routes"]; got != float64(keysetFixtureRoutes) {
			t.Errorf("keyset_routes = %v, want %d", got, keysetFixtureRoutes)
		}
	})
}

// TestRunCheck_ARouteDeclaringACursorTheRecordDoesNotFollow_IsRefused verifies
// the rule that holds the walk to GitLab's own words: a version 5 record in
// which a route declares a cursor parameter and records no keyset site, or no
// site asked by that parameter, is refused with the route named.
//
// The walk never follows a call on another object, so a route a finder pages
// by keyset would otherwise read, silently, as one GitLab pages by offset. The
// whole refusal is the assertion, so a record refused for anything else, or a
// build that cannot read version 5 at all, reads differently from it.
func TestRunCheck_ARouteDeclaringACursorTheRecordDoesNotFollow_IsRefused(t *testing.T) {
	for _, testCase := range []struct {
		name string
		edit func(record map[string]any)
		want string
	}{
		{
			name: "a route declaring a cursor that records no keyset pagination",
			edit: func(record map[string]any) {
				jsonRoute(record, keysetFixtureRoutes)["params"] = map[string]any{
					"cursor": map[string]any{"type": "String"}, "per_page": map[string]any{"type": "Integer"},
				}
			},
			want: "1 routes declare a cursor parameter and record no keyset pagination " +
				"(GET /api/:version/fixture/40 (cursor)): the walk missed the code that reads it, " +
				"and each would read as a route GitLab pages by offset",
		},
		{
			name: "a route declaring a cursor none of its keyset sites is asked by",
			edit: func(record map[string]any) {
				jsonRoute(record, 3)["params"] = map[string]any{"page_token": map[string]any{"type": "String"}}
			},
			want: "1 routes declare a cursor parameter none of their keyset sites is asked by " +
				"(GET /api/:version/fixture/3 page_token): the record and the route's own declaration " +
				"disagree about what its next page is asked by",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			writeRecordAt(t, dir, wholeEnough(), today())
			record := readJSONFile(t, apilive.Path(dir))
			record["schema_version"] = float64(5)
			markKeyset(t, record)
			testCase.edit(record)
			writeJSONFile(t, apilive.Path(dir), record)

			err := runCheck(dir, time.Now())

			want := "the live API record cannot be rested on:\n  " + testCase.want
			if err == nil || err.Error() != want {
				t.Errorf("runCheck = %v, want %q", err, want)
			}
		})
	}
}

// TestRunCheck_AVersion4RecordLabeled5_IsRefused verifies the inversion
// schema version 5 exists to stop: a record with no keyset half at all, as a
// version 4 introspection writes it, labeled 5. Read as version 5 it says
// that GitLab pages every route by offset, so the gate refuses it, by the
// missing probe and by the floor, and says which of the two readings it is.
func TestRunCheck_AVersion4RecordLabeled5_IsRefused(t *testing.T) {
	dir := t.TempDir()
	writeRecordAt(t, dir, wholeEnough(), today())
	record := readJSONFile(t, apilive.Path(dir))
	record["schema_version"] = float64(5)
	delete(record, "keyset_paginators")
	delete(record["source"].(map[string]any), "keyset_routes")
	for _, route := range record["routes"].([]any) {
		delete(route.(map[string]any), "keyset")
	}
	writeJSONFile(t, apilive.Path(dir), record)

	err := runCheck(dir, time.Now())

	want := "the live API record cannot be rested on:\n" +
		"  it carries no keyset_paginators block, which every version 5 introspection writes: it reads like a " +
		"version 4 record labeled 5, and every route in it would read as one GitLab pages by offset\n" +
		"  it holds 0 routes reaching a keyset paginator and a GitLab has at least 35: the keyset walk did not finish, " +
		"or an introspection that does not walk it wrote the record"
	if err == nil || err.Error() != want {
		t.Errorf("runCheck = %v, want %q", err, want)
	}
}

// TestKeysetFixtures_ClearTheFloor holds the literal the JSON fixtures above
// mark to the floor they have to clear, which they cannot name themselves.
func TestKeysetFixtures_ClearTheFloor(t *testing.T) {
	t.Parallel()
	if keysetFixtureRoutes <= minKeysetRoutes {
		t.Errorf("markKeyset marks %d routes and the floor is %d: every case would trip the floor", keysetFixtureRoutes, minKeysetRoutes)
	}
	if got := documentOf(wholeEnough()).KeysetRoutes(); got != minKeysetRoutes+1 {
		t.Errorf("wholeEnough marks %d routes, want one above the floor of %d", got, minKeysetRoutes)
	}
}

// keysetDocument is a document holding count routes that reach a keyset
// paginator and the probe, which every keyset rule passes: the base each case
// below edits.
func keysetDocument(count int) apilive.Document {
	doc := apilive.Document{KeysetPaginators: wholePaginators()}
	for i := range count {
		doc.Routes = append(doc.Routes, apilive.Route{
			Method: "GET", Path: routePath(i), Keyset: []apilive.KeysetSite{keysetSite(i)},
		})
	}
	return doc
}

// TestKeysetProblems_EachRule_NamesWhatItRefuses walks every keyset rule from
// a document that passes them all, one edit at a time, and holds the whole
// sentence each produces, so a rule that fires on the wrong site, counts the
// wrong things or fires for a shape it should pass reads differently.
func TestKeysetProblems_EachRule_NamesWhatItRefuses(t *testing.T) {
	for _, testCase := range []struct {
		name string
		edit func(*apilive.Document)
		want []string
	}{
		{name: "a record at the floor with every site readable", edit: func(*apilive.Document) {}},
		{
			name: "one route short of the floor",
			edit: func(d *apilive.Document) { d.Routes = d.Routes[1:] },
			want: []string{"it holds 34 routes reaching a keyset paginator and a GitLab has at least 35: the keyset walk did not " +
				"finish, or an introspection that does not walk it wrote the record"},
		},
		{
			name: "no probe of the paginators",
			edit: func(d *apilive.Document) { d.KeysetPaginators = nil },
			want: []string{"it carries no keyset_paginators block, which every version 5 introspection writes: it " +
				"reads like a version 4 record labeled 5, and every route in it would read as one GitLab pages by offset"},
		},
		{
			name: "a probe that names nothing, and one nothing reads",
			edit: func(d *apilive.Document) {
				d.KeysetPaginators[apilive.ProbedPager] = nil
				delete(d.KeysetPaginators, apilive.ProbedCursorHeaders)
				d.KeysetPaginators["offset_pager"] = []string{"page"}
			},
			want: []string{
				"2 keyset paginators name no parameter (cursor_headers, pager): the probe did not run, and a route each " +
					"serves would read as followed by nothing",
				"1 keyset paginators are ones nothing here reads (offset_pager): introspect.rb probes a paginator this " +
					"build does not know",
			},
		},
		{
			name: "sites naming a paginator nothing reads, no place and no cursor",
			edit: func(d *apilive.Document) {
				d.Routes[1].Keyset[0].Paginator = "offset_paginate"
				d.Routes[2].Keyset[0].Paginator = ""
				d.Routes[3].Keyset[0].File = ""
				d.Routes[4].Keyset[0].Line = 0
				d.Routes[5].Keyset[0].Cursor = nil
			},
			want: []string{
				"2 keyset sites name a paginator nothing here reads (GET /api/:version/fixture/1 offset_paginate, " +
					`GET /api/:version/fixture/2 ""): introspect.rb met a paginator this build does not know`,
				"2 keyset sites carry no file and line (GET /api/:version/fixture/3, GET /api/:version/fixture/4): " +
					"nobody could read the code that serves the page",
				"1 keyset sites name no parameter their next page is asked by (GET /api/:version/fixture/5 " +
					"lib/api/fixture.rb:105): a reader could not say what to send back",
			},
		},
		{
			name: "a site on the first line of its file is located",
			edit: func(d *apilive.Document) { d.Routes[4].Keyset[0].Line = 1 },
		},
		{
			name: "routes declaring a cursor the record does not follow",
			edit: func(d *apilive.Document) {
				d.Routes = append(d.Routes, apilive.Route{Method: "GET", Path: "/api/:version/offset", Params: map[string]apilive.Param{
					"id_before": {}, "id_after": {}, "per_page": {},
				}})
				d.Routes[6].Params = map[string]apilive.Param{"cursor": {}, "page_token": {}}
				d.Routes[7].Params = map[string]apilive.Param{"cursor": {}}
				d.Routes[8].Params = map[string]apilive.Param{"page": {}}
			},
			want: []string{
				"1 routes declare a cursor parameter and record no keyset pagination (GET /api/:version/offset " +
					"(id_after, id_before)): the walk missed the code that reads it, and each would read as a route " +
					"GitLab pages by offset",
				"1 routes declare a cursor parameter none of their keyset sites is asked by (GET /api/:version/fixture/6 " +
					"page_token): the record and the route's own declaration disagree about what its next page is asked by",
			},
		},
		{
			name: "a cursor only the second of two sites is asked by is followed",
			edit: func(d *apilive.Document) {
				d.Routes[9].Keyset = append(d.Routes[9].Keyset, apilive.KeysetSite{
					Paginator: apilive.PaginatorGitalyKeysetPager, File: "lib/api/fixture.rb", Line: 200,
					Cursor: []string{"page_token"},
				})
				d.Routes[9].Params = map[string]apilive.Param{"page_token": {}}
			},
		},
		{
			name: "a parameter only a site names is a cursor too",
			edit: func(d *apilive.Document) {
				d.Routes[1].Keyset[0].Cursor = []string{"last"}
				d.Routes[2].Params = map[string]apilive.Param{"last": {}}
			},
			want: []string{
				"1 routes declare a cursor parameter none of their keyset sites is asked by (GET /api/:version/fixture/2 " +
					"last): the record and the route's own declaration disagree about what its next page is asked by",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			doc := keysetDocument(minKeysetRoutes)
			testCase.edit(&doc)

			assertProblems(t, keysetProblems(doc), testCase.want)
		})
	}
}

// TestRunGenerate_AKeysetSiteNothingReads_IsRefusedAfterTheFineGrainedHalf
// verifies the generator refuses to write what the gate would refuse, and
// writes nothing: a site with no cursor is fixed by teaching the script, and
// written down it would read as a page nobody can follow.
//
// It is refused after the fine-grained half, and the second case is what says
// so: a dump carrying both an unreadable authorization and an unreadable
// keyset site is reported as the first, since the order is the order a
// maintainer fixes them in, and the two refusals swapped would pass the first
// case alone.
func TestRunGenerate_AKeysetSiteNothingReads_IsRefusedAfterTheFineGrainedHalf(t *testing.T) {
	uncursored := func(d *dumped) { d.Routes[0].Keyset[0].Cursor = nil }
	for _, testCase := range []struct {
		name   string
		mutate func(*dumped)
		want   string
	}{
		{
			name:   "a whole dump with one keyset site naming no cursor",
			mutate: uncursored,
			want: "refusing to write a record whose keyset pagination cannot be read:\n  1 keyset sites name no parameter " +
				"their next page is asked by (GET /api/:version/fixture/0 lib/api/fixture.rb:100): a reader could not say what to send back",
		},
		{
			name: "an unreadable authorization is reported before it",
			mutate: func(d *dumped) {
				uncursored(d)
				d.Routes[2].Authorization.UnknownKeys = []string{"future_option"}
			},
			want: "refusing to write a record whose fine-grained authorization cannot be read:\n  " +
				"1 route authorizations carry keys nothing here reads (GET /api/:version/fixture/2 future_option): " +
				"introspect.rb met an option it does not know, and a reader would judge the route without it",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			payload := wholeEnough()
			testCase.mutate(&payload)
			dir := t.TempDir()

			err := runGenerate(dir, dumpFrom{path: writeDump(t, payload)}, "gitlab/gitlab-ee:latest", false)

			if err == nil || err.Error() != testCase.want {
				t.Errorf("runGenerate = %v, want %q", err, testCase.want)
			}
			if _, statErr := os.Stat(apilive.Path(dir)); statErr == nil {
				t.Error("a refused record was written anyway")
			}
		})
	}
}

// TestKeysetSummary_CountsTheRoutesOnlyAFlagGitLabShipsOffTurnsOn verifies the
// keyset line of a report. A route counts as flagged only when every site it
// has is behind a flag GitLab ships off: one site on by default is enough for
// an instance nobody configured to page it by keyset, and a route that pages
// by offset is never counted, flagged or not.
func TestKeysetSummary_CountsTheRoutesOnlyAFlagGitLabShipsOffTurnsOn(t *testing.T) {
	t.Parallel()
	off := []apilive.KeysetFlag{{Name: "commits_keyset_pagination", DefaultEnabled: new(false)}}
	doc := apilive.Document{
		Source: apilive.Source{KeysetRoutes: 7},
		Routes: []apilive.Route{
			{Keyset: []apilive.KeysetSite{{Paginator: apilive.PaginatorGitalyKeysetPager, Flags: off}}},
			{Keyset: []apilive.KeysetSite{{Flags: off}, {Paginator: apilive.PaginatorStrategies}}},
			{Keyset: []apilive.KeysetSite{{Paginator: apilive.PaginatorStrategies}}},
			{},
			{Keyset: []apilive.KeysetSite{{Flags: off}, {Flags: off}}},
		},
	}

	want := "7 routes reach a keyset paginator, 2 of them only behind a feature flag GitLab ships off"
	if got := keysetSummary(doc); got != want {
		t.Errorf("keysetSummary() = %q, want %q", got, want)
	}
}

// TestKeysetProblems_TheCommittedShapeOfARoute_PassesAsItIsRecorded holds the
// rules to the shapes 19.5.0 records, so a rule that would refuse GitLab's own
// answer fails here rather than on the next boot: the project list asked by
// both of Keyset::Pager's parameters and declaring both, and the work item
// discussion notes, a route with two sites both asked by the cursor it
// declares.
func TestKeysetProblems_TheCommittedShapeOfARoute_PassesAsItIsRecorded(t *testing.T) {
	t.Parallel()
	doc := keysetDocument(minKeysetRoutes)
	doc.Routes[0] = apilive.Route{
		Method: "GET", Path: "/api/:version/projects",
		Params: map[string]apilive.Param{"id_after": {Type: "Integer"}, "id_before": {Type: "Integer"}},
		Keyset: []apilive.KeysetSite{{
			Paginator: apilive.PaginatorStrategies, File: "lib/api/projects.rb", Line: 248,
			Via: []string{"present_projects"}, Cursor: []string{"id_after", "id_before"},
		}},
	}
	doc.Routes[1] = apilive.Route{
		Method: "GET", Path: "/api/:version/projects/:id/-/work_items/:work_item_iid/discussions/:discussion_id/notes",
		Params: map[string]apilive.Param{"cursor": {Type: "String"}},
		Keyset: []apilive.KeysetSite{
			{Paginator: apilive.PaginatorKeysetPaginate, File: "lib/api/work_items/discussion_notes.rb", Line: 53, Cursor: []string{"cursor"}},
			{Paginator: apilive.PaginatorHeaderBuilder, File: "lib/api/work_items/discussion_notes.rb", Line: 63, Cursor: []string{"cursor"}},
		},
	}

	if got := keysetProblems(doc); len(got) != 0 {
		t.Errorf("keysetProblems() = %q, want none", got)
	}
}

// TestKeysetProblems_RunByTheGate_AreReportedBesideTheOthers verifies the gate
// runs the keyset rules at all, on a record that passes every other one.
func TestKeysetProblems_RunByTheGate_AreReportedBesideTheOthers(t *testing.T) {
	payload := wholeEnough()
	payload.Routes[2].Keyset[0].Paginator = "offset_paginate"
	dir := t.TempDir()
	writeRecordAt(t, dir, payload, today())

	err := runCheck(dir, time.Now())

	want := fmt.Sprintf("1 keyset sites name a paginator nothing here reads (%s offset_paginate)", "GET "+routePath(2))
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("runCheck = %v, want it to name %q", err, want)
	}
}
