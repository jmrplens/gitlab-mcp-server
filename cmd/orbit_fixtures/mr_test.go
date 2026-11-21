// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// inLineRoutes are the routes of the recorded scenario where every fixture
// is already in place, for a case to change what it is about.
func inLineRoutes(t *testing.T) map[string]goldenRoute {
	t.Helper()
	for _, golden := range readGoldens(t) {
		if golden.Scenario == "in-line" {
			return maps.Clone(golden.Routes)
		}
	}
	t.Fatal("no in-line golden under testdata/setup")
	return nil
}

// jsonRoute is a route answering status with body encoded as JSON.
func jsonRoute(t *testing.T, status int, body any) goldenRoute {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding a route: %v", err)
	}
	return goldenRoute{Status: status, Body: raw}
}

// kgMR is the path of kg-fixtures' merge requests in the recorded scenarios.
const kgMR = "/api/v4/projects/10/merge_requests"

// kgProjectID is kg-fixtures' project id in the recorded scenarios.
const kgProjectID = 10

// listedMR is a merge request as GitLab lists it from kg-fixtures into main,
// the only kind a run may take for the fixture's.
func listedMR(iid int64, state string) map[string]any {
	return map[string]any{"iid": iid, "state": state, "source_project_id": kgProjectID, "target_project_id": kgProjectID, "target_branch": "main"}
}

// fullPageFromForks is a full page of a hundred merge requests from a fork,
// opened from a branch of the fixture branch's name, the kind that can push
// the fixture's own off the first page of the list.
func fullPageFromForks() []map[string]any {
	page := make([]map[string]any, 0, 100)
	for n := range 100 {
		request := listedMR(int64(100+n), "opened")
		request["source_project_id"] = 99
		page = append(page, request)
	}
	return page
}

// setupCase runs setup against routes, with git standing in as fakeGit and
// failing the subcommand failGit names, and returns the exit code and what
// the run wrote to stderr.
func setupCase(t *testing.T, routes map[string]goldenRoute, failGit string, change func(*setupConfig)) (int, string) {
	t.Helper()
	server := httptest.NewServer(newFakeGitLab(routes))
	t.Cleanup(server.Close)
	git := &fakeGit{base: server.URL, fixture: kgFixture, fail: failGit}
	golden := goldenSetup{Flag: "--skip-mirror"}
	env := map[string]string{"GITLAB_COM_TOKEN": standInToken, "ORBIT_FIXTURES_NAMESPACE": "plens1", "ORBIT_FIXTURES_GITLAB_URL": server.URL}
	var stderr strings.Builder
	cfg, _, _ := setupFromArgs([]string{golden.Flag, "-root", filepath.Join("..", "..")}, func(key string) string { return env[key] }, &stderr)
	cfg.api.client = server.Client()
	cfg.git = git.run
	cfg.mergeInterval = 0
	cfg.sleep = func(context.Context, time.Duration) error { return nil }
	if change != nil {
		change(&cfg)
	}
	var stdout strings.Builder
	code := runSetup(t.Context(), cfg, &stdout, &stderr)
	return code, stderr.String()
}

// TestFixtureMergeRequest_WhatGitLabRefuses_StopsTheRun checks each answer
// that leaves the fixture merge request unmerged stops the run with GitLab's
// answer: an unreadable list, later page of it, merge request or branch, a
// list that never ends, a list answered null, a read of the merge request
// answered null or with no JSON, which leaves nothing to tell a check that
// has ended from one that has not,
// a refused reopen or one answered with the merge request still closed or with
// no JSON, a merge answered with the merge request still open, a new merge
// request answered without an iid or with no JSON, a git that cannot push the
// branch, and a wait the run is cancelled during. A merge request refused
// over a branch a failed run left stops the run without anything cloned.
func TestFixtureMergeRequest_WhatGitLabRefuses_StopsTheRun(t *testing.T) {
	opened := []map[string]any{listedMR(1, "opened")}
	closed := []map[string]any{listedMR(1, "closed")}
	mergeable := map[string]any{"iid": 1, "state": "opened", "detailed_merge_status": "mergeable"}
	branch := "GET /api/v4/projects/10/repository/branches/feature%2Frestock-helper"
	// notJSON is an answer GitLab gives with a 200 and a body no JSON reader
	// takes, an error page from a proxy in front of it.
	notJSON := jsonRoute(t, 200, "<html>")
	cases := []struct {
		name    string
		routes  map[string]any
		failGit string
		cancel  bool
		want    string
	}{
		{name: "an unreadable list", routes: map[string]any{"GET " + kgMR: "500"}, want: "cannot list the merge requests from feature/restock-helper (HTTP 500)"},
		{name: "a list that is not one", routes: map[string]any{"GET " + kgMR: map[string]any{"message": "x"}}, want: "cannot list the merge requests"},
		{name: "an unreadable merge request", routes: map[string]any{"GET " + kgMR: opened, "GET " + kgMR + "/1": "500"}, want: "cannot read MR !1 (HTTP 500)"},
		{name: "a merge answered unmerged", routes: map[string]any{"GET " + kgMR: opened, "GET " + kgMR + "/1": mergeable, "PUT " + kgMR + "/1/merge": map[string]any{"iid": 1, "state": "opened"}}, want: "MR !1 is opened after the merge, not merged"},
		{name: "a merge answered with no state", routes: map[string]any{"GET " + kgMR: opened, "GET " + kgMR + "/1": mergeable, "PUT " + kgMR + "/1/merge": map[string]any{}}, want: "MR !1 is in no state after the merge"},
		{name: "an unreadable branch", routes: map[string]any{"GET " + kgMR: closed, "GET /api/v4/projects/10/repository/branches/feature%2Frestock-helper": "500"}, want: "cannot tell whether branch feature/restock-helper exists (HTTP 500)"},
		{name: "a refused reopen", routes: map[string]any{"GET " + kgMR: closed, "GET /api/v4/projects/10/repository/branches/feature%2Frestock-helper": map[string]any{"name": "x"}, "PUT " + kgMR + "/1": "403"}, want: "could not reopen MR !1 (HTTP 403)"},
		{name: "a new merge request without an iid", routes: map[string]any{"GET " + kgMR: []any{}, "POST " + kgMR: map[string]any{"message": "conflict"}}, want: "could not open the fixture MR"},
		{name: "a closed one replaced and refused", routes: map[string]any{"GET " + kgMR: closed, "POST " + kgMR: "409"}, want: "could not open the fixture MR (HTTP 409)"},
		{name: "a push git refuses", routes: map[string]any{"GET " + kgMR: []any{}}, failGit: "push", want: "git push: exit status 128"},
		{name: "a clone git refuses", routes: map[string]any{"GET " + kgMR: []any{}}, failGit: "clone", want: "git clone: exit status 128"},
		{name: "a checkout git refuses", routes: map[string]any{"GET " + kgMR: []any{}}, failGit: "checkout", want: "git checkout: exit status 128"},
		{name: "a wait cancelled", routes: map[string]any{"GET " + kgMR: opened, "GET " + kgMR + "/1": map[string]any{"detailed_merge_status": "checking"}}, cancel: true, want: "context canceled"},
		{name: "a list answered null", routes: map[string]any{"GET " + kgMR: nil}, want: "cannot list the merge requests from feature/restock-helper (HTTP 200): null"},
		{name: "a merge request read answered null", routes: map[string]any{"GET " + kgMR: opened, "GET " + kgMR + "/1": nil}, want: "cannot read MR !1 (HTTP 200): null"},
		{name: "a merge request read answered with no JSON", routes: map[string]any{"GET " + kgMR: opened, "GET " + kgMR + "/1": notJSON}, want: "cannot read MR !1 (HTTP 200): <html>"},
		{name: "a reopen answered closed", routes: map[string]any{"GET " + kgMR: closed, branch: map[string]any{"name": "x"}, "PUT " + kgMR + "/1": map[string]any{"iid": 1, "state": "closed"}}, want: "could not reopen MR !1 (HTTP 200)"},
		{name: "a reopen answered with no JSON", routes: map[string]any{"GET " + kgMR: closed, branch: map[string]any{"name": "x"}, "PUT " + kgMR + "/1": notJSON}, want: "could not reopen MR !1 (HTTP 200): <html>"},
		{name: "a new merge request answered with no JSON", routes: map[string]any{"GET " + kgMR: []any{}, "POST " + kgMR: notJSON}, want: "could not open the fixture MR (HTTP 200): <html>"},
		{name: "a branch left behind whose merge request is refused", routes: map[string]any{"GET " + kgMR: []any{}, branch: map[string]any{"name": "x"}, "POST " + kgMR: "409"}, failGit: "clone", want: "could not open the fixture MR (HTTP 409)"},
		{name: "an unreadable later page of the list", routes: map[string]any{"GET " + kgMR: fullPageFromForks(), "GET " + kgMR + "@2": "500"}, want: "cannot list the merge requests from feature/restock-helper (HTTP 500)"},
		{name: "a list that never ends", routes: map[string]any{"GET " + kgMR: fullPageFromForks()}, want: "the merge requests from feature/restock-helper run past 100 pages"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routes := inLineRoutes(t)
			for key, body := range tc.routes {
				if route, isRoute := body.(goldenRoute); isRoute {
					routes[key] = route
					continue
				}
				if status, isStatus := body.(string); isStatus {
					routes[key] = jsonRoute(t, map[string]int{"403": 403, "409": 409, "500": 500}[status], map[string]string{"message": status})
					continue
				}
				routes[key] = jsonRoute(t, 200, body)
			}
			code, stderr := setupCase(t, routes, tc.failGit, func(cfg *setupConfig) {
				if tc.cancel {
					cfg.sleep = func(context.Context, time.Duration) error { return context.Canceled }
				}
			})
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit code %d, stderr:\n%s\nwant 1 and %q", code, stderr, tc.want)
			}
		})
	}
}

// TestPickMergeRequest_States_MergedThenOpenThenClosedNewestFirst checks the
// merge request that decides a run: a merged one before an open one before a
// closed one, the newest of each, and none for states that are none of them.
// Only one from kg-fixtures itself into main counts, whatever its state: one
// from a fork, into another project or into another branch is passed over.
func TestPickMergeRequest_States_MergedThenOpenThenClosedNewestFirst(t *testing.T) {
	own := func(iid int64, state string) mergeRequest {
		return mergeRequest{IID: iid, State: state, SourceProjectID: kgProjectID, TargetProjectID: kgProjectID, TargetBranch: "main"}
	}
	fromFork, intoAnotherProject, intoAnotherBranch := own(9, "merged"), own(10, "opened"), own(11, "closed")
	fromFork.SourceProjectID = 99
	intoAnotherProject.TargetProjectID = 99
	intoAnotherBranch.TargetBranch = "release"
	cases := []struct {
		name      string
		requests  []mergeRequest
		wantState string
		wantIID   int64
	}{
		{name: "a merged one among others", requests: []mergeRequest{own(3, "opened"), own(1, "merged"), own(4, "closed")}, wantState: "merged", wantIID: 1},
		{name: "the newest open one", requests: []mergeRequest{own(2, "opened"), own(5, "opened"), own(7, "closed")}, wantState: "opened", wantIID: 5},
		{name: "the newest closed one", requests: []mergeRequest{own(2, "closed"), own(6, "closed")}, wantState: "closed", wantIID: 6},
		{name: "the newest listed first", requests: []mergeRequest{own(8, "opened"), own(3, "opened")}, wantState: "opened", wantIID: 8},
		{name: "only a locked one", requests: []mergeRequest{own(2, "locked")}},
		{name: "none", requests: nil},
		{name: "only others", requests: []mergeRequest{fromFork, intoAnotherProject, intoAnotherBranch}},
		{name: "the fixture's beside a merged one from a fork", requests: []mergeRequest{fromFork, own(2, "opened")}, wantState: "opened", wantIID: 2},
		{name: "the fixture's beside newer others", requests: []mergeRequest{intoAnotherProject, own(2, "opened"), intoAnotherBranch}, wantState: "opened", wantIID: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, iid := pickMergeRequest(tc.requests, kgProjectID)
			if state != tc.wantState || iid != tc.wantIID {
				t.Errorf("picked %q !%d, want %q !%d", state, iid, tc.wantState, tc.wantIID)
			}
		})
	}
}

// TestFixtureMergeRequest_OthersFromTheBranch_CountAsNone checks a merge
// request from a fork, into another project or into another branch is never
// merged, reopened or taken for the fixture's. The project is public, so
// anyone can open one from a branch of the fixture branch's name, the list
// filters by that name alone, and the run merges what it picks with the
// operator's token. With only those listed, the run sends what it sends with
// none listed: the recorded scenario of a branch a failed run left behind,
// whose merge request is opened from it and merged.
func TestFixtureMergeRequest_OthersFromTheBranch_CountAsNone(t *testing.T) {
	var golden goldenSetup
	for _, recorded := range readGoldens(t) {
		if recorded.Scenario == "opens-the-mr-from-a-branch-left-behind" {
			golden = recorded
		}
	}
	if golden.Scenario == "" {
		t.Fatal("no opens-the-mr-from-a-branch-left-behind golden under testdata/setup")
	}
	fromFork, intoAnotherProject, intoAnotherBranch := listedMR(5, "opened"), listedMR(4, "closed"), listedMR(3, "merged")
	fromFork["source_project_id"] = 99
	intoAnotherProject["target_project_id"] = 99
	intoAnotherBranch["target_branch"] = "release"
	golden.Routes = maps.Clone(golden.Routes)
	golden.Routes["GET "+kgMR] = jsonRoute(t, 200, []map[string]any{fromFork, intoAnotherProject, intoAnotherBranch})
	fake := newFakeGitLab(golden.Routes)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	git := &fakeGit{base: server.URL, fixture: kgFixture}

	code, stdout, stderr := setupAgainst(t, golden, server, git.run)

	if code != 0 {
		t.Fatalf("exit code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	got := fake.requests()
	if len(got) != len(golden.Requests) {
		t.Fatalf("%d requests sent, want the %d of a run with none listed", len(got), len(golden.Requests))
	}
	for i, request := range got {
		if !sameRequest(request, golden.Requests[i]) {
			t.Errorf("request %d: %s sent, want %s", i+1, describe(request), describe(golden.Requests[i]))
		}
	}
	if len(git.log) > 0 {
		t.Errorf("git was asked for %q, want nothing: the fixture branch is there", git.log)
	}
}

// TestFixtureMergeRequest_OnALaterPage_IsFound checks the list of merge
// requests from the fixture branch is read a hundred at a time over every
// page before one is chosen. GitLab answers twenty unless asked for more, and
// merge requests from forks can push the fixture's own off the first page, so
// a choice made from that page alone would open and merge another one.
func TestFixtureMergeRequest_OnALaterPage_IsFound(t *testing.T) {
	routes := inLineRoutes(t)
	routes["GET "+kgMR] = jsonRoute(t, 200, fullPageFromForks())
	routes["GET "+kgMR+"@2"] = jsonRoute(t, 200, []map[string]any{listedMR(2, "merged")})
	server, fake := serveRoutes(t, routes)
	git := &fakeGit{base: server.URL, fixture: kgFixture}

	code, stdout, stderr := setupAgainst(t, goldenSetup{Flag: "--skip-mirror"}, server, git.run)

	if code != 0 || !strings.Contains(stdout, "MR !2 is merged") {
		t.Fatalf("exit code %d, want 0 and MR !2 merged\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var listed []string
	for _, request := range fake.requests() {
		switch {
		case request.Method == http.MethodGet && request.Path == kgMR:
			listed = append(listed, request.Query)
		case request.Method != http.MethodGet && strings.HasPrefix(request.Path, kgMR):
			t.Errorf("%s sent, want nothing opened, reopened or merged", describe(request))
		}
	}
	want := []string{
		"source_branch=feature/restock-helper&state=all&per_page=100&page=1",
		"source_branch=feature/restock-helper&state=all&per_page=100&page=2",
	}
	if !slices.Equal(listed, want) {
		t.Errorf("the list was read as %q, want %q", listed, want)
	}
	if len(git.log) > 0 {
		t.Errorf("git was asked for %q, want nothing", git.log)
	}
}

// TestInsertSeedStock_TheModule_GoesInFrontOfReserveStock checks the change
// the fixture merge request makes, and that a module with no reserve_stock to
// put it in front of, or none at all, fails rather than pushing a branch
// without the change.
func TestInsertSeedStock_TheModule_GoesInFrontOfReserveStock(t *testing.T) {
	dir := t.TempDir()
	module := filepath.Join(dir, "fulfillment.py")
	if err := os.WriteFile(module, []byte("x = 1\ndef reserve_stock(order):\n    pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := insertSeedStock(module); err != nil {
		t.Fatalf("insertSeedStock: %v", err)
	}
	got, _ := os.ReadFile(module) //#nosec G304 -- a file of the test's own temporary directory
	if want := "x = 1\n" + seedStock + "def reserve_stock(order):\n    pass\n"; string(got) != want {
		t.Errorf("module = %q, want %q", got, want)
	}

	if err := os.WriteFile(module, []byte("def reserve_stock(order):\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := insertSeedStock(module); err == nil || !strings.Contains(err.Error(), "no reserve_stock") {
		t.Errorf("a module opening with reserve_stock: %v", err)
	}
	if err := insertSeedStock(filepath.Join(dir, "missing.py")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing module: %v", err)
	}
}
