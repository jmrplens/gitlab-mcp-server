// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// setupCase runs setup against routes, with git standing in as fakeGit and
// failing the subcommand failGit names, and returns the exit code and what
// the run wrote to stderr.
func setupCase(t *testing.T, routes map[string]goldenRoute, failGit string, change func(*setupConfig)) (int, string) {
	t.Helper()
	server := httptest.NewServer(newFakeGitLab(routes))
	t.Cleanup(server.Close)
	git := &fakeGit{base: server.URL, fixture: kgFixture, fail: failGit}
	golden := goldenSetup{Flag: "--skip-mirror"}
	env := map[string]string{"GITLAB_COM_TOKEN": "stand-in-token", "ORBIT_FIXTURES_NAMESPACE": "plens1", "ORBIT_FIXTURES_GITLAB_URL": server.URL}
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
// answer: an unreadable list, merge request or branch, a list answered null,
// a refused reopen or one answered with the merge request still closed or with
// no JSON, a merge answered with the merge request still open, a new merge
// request answered without an iid or with no JSON, a git that cannot push the
// branch, and a wait the run is cancelled during. A merge request refused
// over a branch a failed run left stops the run without anything cloned.
func TestFixtureMergeRequest_WhatGitLabRefuses_StopsTheRun(t *testing.T) {
	opened := []map[string]any{{"iid": 1, "state": "opened"}}
	closed := []map[string]any{{"iid": 1, "state": "closed"}}
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
		{name: "a reopen answered closed", routes: map[string]any{"GET " + kgMR: closed, branch: map[string]any{"name": "x"}, "PUT " + kgMR + "/1": map[string]any{"iid": 1, "state": "closed"}}, want: "could not reopen MR !1 (HTTP 200)"},
		{name: "a reopen answered with no JSON", routes: map[string]any{"GET " + kgMR: closed, branch: map[string]any{"name": "x"}, "PUT " + kgMR + "/1": notJSON}, want: "could not reopen MR !1 (HTTP 200): <html>"},
		{name: "a new merge request answered with no JSON", routes: map[string]any{"GET " + kgMR: []any{}, "POST " + kgMR: notJSON}, want: "could not open the fixture MR (HTTP 200): <html>"},
		{name: "a branch left behind whose merge request is refused", routes: map[string]any{"GET " + kgMR: []any{}, branch: map[string]any{"name": "x"}, "POST " + kgMR: "409"}, failGit: "clone", want: "could not open the fixture MR (HTTP 409)"},
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
func TestPickMergeRequest_States_MergedThenOpenThenClosedNewestFirst(t *testing.T) {
	cases := []struct {
		name      string
		requests  []mergeRequest
		wantState string
		wantIID   int64
	}{
		{name: "a merged one among others", requests: []mergeRequest{{IID: 3, State: "opened"}, {IID: 1, State: "merged"}, {IID: 4, State: "closed"}}, wantState: "merged", wantIID: 1},
		{name: "the newest open one", requests: []mergeRequest{{IID: 2, State: "opened"}, {IID: 5, State: "opened"}, {IID: 7, State: "closed"}}, wantState: "opened", wantIID: 5},
		{name: "the newest closed one", requests: []mergeRequest{{IID: 2, State: "closed"}, {IID: 6, State: "closed"}}, wantState: "closed", wantIID: 6},
		{name: "the newest listed first", requests: []mergeRequest{{IID: 8, State: "opened"}, {IID: 3, State: "opened"}}, wantState: "opened", wantIID: 8},
		{name: "only a locked one", requests: []mergeRequest{{IID: 2, State: "locked"}}},
		{name: "none", requests: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, iid := pickMergeRequest(tc.requests)
			if state != tc.wantState || iid != tc.wantIID {
				t.Errorf("picked %q !%d, want %q !%d", state, iid, tc.wantState, tc.wantIID)
			}
		})
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
