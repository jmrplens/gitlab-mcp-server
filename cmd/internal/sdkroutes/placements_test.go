// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdkroutes

import (
	"reflect"
	"slices"
	"testing"
)

// TestSDK_Placements_PlacesEachParameterInTheSegmentItFills holds the
// placement reading to the stand-in's methods: a parameter wrapped in a
// struct the reading cannot name, one passed in place, one handed to a
// helper held in a field, to a helper of the receiver and to a generic
// package function, one turned into a local by a call the reading cannot
// name and formatted into a legacy path, one carried into the path a helper
// returns, one formatted into the query string, which fills no segment, and
// two built into one piece, in place or handed to a helper, which both fill.
// A local assigned both a piece
// the reading cannot name and one built out of a parameter is read as that
// parameter in both. A parameter beside literal text in a segment fills
// none of it.
func TestSDK_Placements_PlacesEachParameterInTheSegmentItFills(t *testing.T) {
	sdk := fixture(t)
	place := func(param string, index int, kind, method, path string, segment int) Placement {
		return Placement{Param: param, Index: index, Type: kind, Route: Route{Method: method, Path: path}, Segment: segment}
	}
	const placed = "/placed/:/users/:/keys/:/v/:/:"
	cases := map[string][]Placement{
		"Issues.GetIssue": {
			place("pid", 0, "any", "GET", "/projects/:/issues/:", 1),
			place("issue", 1, "int64", "GET", "/projects/:/issues/:", 3),
		},
		"Issues.AddSpentTime": {
			place("pid", 0, "any", "POST", "/projects/:/issues/:/add_spent_time", 1),
			place("issue", 1, "int64", "POST", "/projects/:/issues/:/add_spent_time", 3),
		},
		"AwardEmoji.GetSnippetAwardEmoji": {
			place("pid", 0, "any", "GET", "/projects/:/snippets/:/award_emoji/:", 1),
			place("snippet", 1, "int64", "GET", "/projects/:/snippets/:/award_emoji/:", 3),
			place("award", 2, "int64", "GET", "/projects/:/snippets/:/award_emoji/:", 5),
		},
		"ProjectUploads.ListUploads": {place("pid", 0, "any", "GET", "/projects/:/uploads", 1)},
		"Issues.ListNotes": {
			place("pid", 0, "any", "GET", "/projects/:/issues/notes", 1),
			place("pid", 0, "any", "GET", "/projects/:/merge_requests/notes", 1),
		},
		"Repositories.StreamArchive": {place("pid", 0, "any", "GET", "/projects/:/repository/archive", 1)},
		"GenericPackages.PublishPackageFile": {
			place("pid", 0, "any", "PUT", "/projects/:/packages/generic/:/:/:", 1),
			place("name", 1, "string", "PUT", "/projects/:/packages/generic/:/:/:", 4),
			place("version", 2, "string", "PUT", "/projects/:/packages/generic/:/:/:", 5),
			place("fileName", 3, "string", "PUT", "/projects/:/packages/generic/:/:/:", 6),
		},
		"Repositories.Stats": {
			place("pid", 0, "any", "GET", "/projects/:/issues/time_stats", 1),
			place("pid", 0, "any", "GET", "/projects/:/merge_requests/time_stats", 1),
		},
		"Repositories.Folds": {
			place("id", 0, "string", "GET", "/prefixed/:", 1),
			place("id", 0, "string", "GET", "/projects/:", 1),
			place("id", 0, "string", "GET", "/relative/:", 1),
		},
		"Issues.Search": {place("scope", 0, "string", "GET", "/literal/:/path", 1)},
		"Issues.Mixed":  {place("key", 0, "string", "GET", "/mixed/:", 1)},
		"Issues.Combined": {
			place("group", 0, "string", "GET", "/combined/:", 1),
			place("name", 1, "string", "GET", "/combined/:", 1),
		},
		"Issues.Placed": {
			place("pid", 0, "any", "GET", placed, 1),
			place("pid", 0, "any", "GET", placed, 7),
			place("user", 1, "any", "GET", placed, 3),
			place("key", 2, "int64", "GET", placed, 5),
			place("key", 2, "int64", "GET", placed, 7),
		},
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			if got := sdk.Placements(key); !reflect.DeepEqual(got, want) {
				t.Errorf("Placements(%q) =\n%+v\nwant\n%+v", key, got, want)
			}
		})
	}
}

// TestSDK_Placements_ReadsNothingItCannotPlace verifies what places
// nothing: a key client-go does not declare, a method with no parameter
// whose template multiplies past the bound, one whose only argument is nil,
// and one whose paths are a parameter whole and whose verb is no net/http
// constant. The placed method's route list is what Read resolved without
// the marks.
func TestSDK_Placements_ReadsNothingItCannotPlace(t *testing.T) {
	sdk := fixture(t)
	for _, key := range []string{"Issues.Absent", "Issues.ManyChoices", "Issues.Unnamed", "Repositories.Fetch"} {
		t.Run(key, func(t *testing.T) {
			if got := sdk.Placements(key); got != nil {
				t.Errorf("Placements(%q) = %+v, want none", key, got)
			}
		})
	}
	method, _ := sdk.Method("Issues.Placed")
	if want := []Route{{Method: "GET", Path: "/placed/:/users/:/keys/:/v/:/:"}}; !reflect.DeepEqual(method.Routes, want) {
		t.Errorf("Issues.Placed routes = %v, want %v", method.Routes, want)
	}
}

// TestSDK_Placements_NameOnlyRoutesTheMethodSends holds every placement of
// every method of the stand-in to a route the first reading resolved for
// it, past the bound, through a legacy path and a helper included: a mark
// stands where that reading has an unknown piece, so taking the marks out
// spells a path it spelled.
func TestSDK_Placements_NameOnlyRoutesTheMethodSends(t *testing.T) {
	sdk := fixture(t)
	for _, method := range sdk.Methods() {
		t.Run(method.Key(), func(t *testing.T) {
			for _, placed := range sdk.Placements(method.Key()) {
				if !slices.Contains(method.Routes, placed.Route) {
					t.Errorf("%+v names a route outside %v", placed, method.Routes)
				}
			}
		})
	}
}
