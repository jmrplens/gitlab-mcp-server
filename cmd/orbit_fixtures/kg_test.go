// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// TestFirstID_Answers_NameTheirIDOrNone checks the id an existing resource
// is found by: an object's, the first element's of a list, and none for an
// empty list, an object without one, a body that is not JSON or no answer.
func TestFirstID_Answers_NameTheirIDOrNone(t *testing.T) {
	cases := []struct {
		name string
		got  answer
		want int64
	}{
		{name: "an object", got: answer{status: 200, body: []byte(`{"id":5}`)}, want: 5},
		{name: "a list", got: answer{status: 200, body: []byte(`[{"id":6},{"id":7}]`)}, want: 6},
		{name: "an empty list", got: answer{status: 200, body: []byte(`[]`)}},
		{name: "an object without an id", got: answer{status: 404, body: []byte(`{"message":"404 Not Found"}`)}},
		{name: "a list of something else", got: answer{status: 200, body: []byte(`["x"]`)}},
		{name: "not JSON", got: answer{status: 200, body: []byte(`<html>`)}},
		{name: "no answer", got: answer{err: errors.New("reset")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstID(tc.got); got != tc.want {
				t.Errorf("firstID = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestKGResources_ACreationWithoutAnID_StopsTheRun checks each resource that
// cannot be created stops the run where it is refused: the milestone, an
// issue and an environment, each answered without an id.
func TestKGResources_ACreationWithoutAnID_StopsTheRun(t *testing.T) {
	for _, collection := range []string{"milestones", "issues", "environments"} {
		t.Run(collection, func(t *testing.T) {
			routes := inLineRoutes(t)
			routes["GET /api/v4/projects/10/"+collection] = jsonRoute(t, 200, []any{})
			routes["POST /api/v4/projects/10/"+collection] = jsonRoute(t, 403, map[string]string{"message": "403 Forbidden"})
			code, stderr := setupCase(t, routes, "", nil)
			if want := "POST /projects/10/" + collection + " did not create anything"; code != 1 || !strings.Contains(stderr, want) {
				t.Errorf("exit code %d, stderr:\n%s\nwant 1 and %q", code, stderr, want)
			}
		})
	}
}

// TestEnsureLabel_ALabelOfAnotherName_DoesNotCount checks a label search,
// which matches by substring, finds a label only by its exact name, so the
// fixture's own is still created.
func TestEnsureLabel_ALabelOfAnotherName_DoesNotCount(t *testing.T) {
	routes := inLineRoutes(t)
	routes["GET /api/v4/projects/10/labels"] = jsonRoute(t, 200, []map[string]any{{"id": 9, "name": "bugfix"}})
	routes["POST /api/v4/projects/10/labels"] = jsonRoute(t, 201, map[string]any{"id": 10})
	server, fake := serveRoutes(t, routes)
	git := &fakeGit{base: server.URL, fixture: kgFixture}
	code, _, stderr := setupAgainst(t, goldenSetup{Flag: "--skip-mirror"}, server, git.run)
	if code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", code, stderr)
	}
	var posted int
	for _, request := range fake.requests() {
		if request.Method == http.MethodPost && request.Path == "/api/v4/projects/10/labels" {
			posted++
		}
	}
	if posted != len(fixtureLabels) {
		t.Errorf("%d labels created, want all %d", posted, len(fixtureLabels))
	}
}
