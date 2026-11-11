// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/routes"
)

// The reports a planted tree answers with: a routes report whose tracked row
// rests on issue 1614 beside a client-protocol row that names none, and an sdk
// report whose tracked identifier rests on issue 1602.
const (
	plantedRoutesReport = `{"summary": {"declared": 2}, "answered": [
		{"route": "DELETE /admin/queue", "class": "declared", "category": "tracked", "issue": 1614},
		{"route": "GET /-/cable", "class": "declared", "category": "client-protocol"}
	]}`
	plantedSDKReport = `{"summary": {"identifiers_declared": 1}, "identifiers": {"arguments": [
		{"input": "users.GetStatusInput.UserID", "status": "declared", "category": "tracked", "issue": 1602}
	]}}`
)

// issuesPath is where GitHub's REST API serves an issue of this repository.
const issuesPath = "/repos/" + issues.DefaultRepository + "/issues/"

// standInGitHub answers each issue of this repository with the status and
// body answer gives it, and records the issue numbers and the Authorization
// headers it was sent.
type standInGitHub struct {
	mu             sync.Mutex
	asked          []int
	authorizations []string
}

// serve starts the stand-in, points githubAPI at it, and returns it.
func serveStandInGitHub(t *testing.T, answer func(number int) (int, string)) *standInGitHub {
	t.Helper()
	github := &standInGitHub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, issuesPath))
		if err != nil || !strings.HasPrefix(r.URL.Path, issuesPath) {
			t.Errorf("the stand-in GitHub was asked %s, which is no issue of %s", r.URL.Path, issues.DefaultRepository)
			http.Error(w, "unexpected request", http.StatusTeapot)
			return
		}
		github.mu.Lock()
		github.asked = append(github.asked, number)
		github.authorizations = append(github.authorizations, r.Header.Get("Authorization"))
		github.mu.Unlock()
		status, body := answer(number)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	original := githubAPI
	t.Cleanup(func() { githubAPI = original })
	githubAPI = server.URL
	return github
}

// requests returns what the stand-in was sent, read under its lock.
func (g *standInGitHub) requests() (asked []int, authorizations []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.asked), slices.Clone(g.authorizations)
}

// open answers every issue open.
func open(int) (int, string) {
	return http.StatusOK, `{"state": "open"}`
}

// issuesCalls records what the routes and sdk seams were asked for.
type issuesCalls struct {
	roots         []string
	routesOptions []routes.Options
	sdkGapsOnly   []bool
}

// plantIssuesTree points the root at a fresh directory, the routes and sdk
// seams at the planted reports, and getenv at env. It returns the root and
// what the seams were asked.
func plantIssuesTree(t *testing.T, env map[string]string) (string, *issuesCalls) {
	t.Helper()
	originalRoot, originalRoutes, originalSDK, originalGetenv := repositoryRoot, routesRun, sdkRun, getenv
	t.Cleanup(func() {
		repositoryRoot, routesRun, sdkRun, getenv = originalRoot, originalRoutes, originalSDK, originalGetenv
	})
	root := t.TempDir()
	calls := &issuesCalls{}
	repositoryRoot = func(string) (string, error) { return root, nil }
	routesRun = func(scopeRoot string, opts routes.Options) ([]byte, bool, error) {
		calls.roots = append(calls.roots, scopeRoot)
		calls.routesOptions = append(calls.routesOptions, opts)
		return []byte(plantedRoutesReport), false, nil
	}
	sdkRun = func(scopeRoot string, gapsOnly bool) ([]byte, bool, error) {
		calls.roots = append(calls.roots, scopeRoot)
		calls.sdkGapsOnly = append(calls.sdkGapsOnly, gapsOnly)
		return []byte(plantedSDKReport), false, nil
	}
	getenv = func(name string) string { return env[name] }
	return root, calls
}

// TestMain_CheckIssues_HoldsTheTrackedRowsToOpenIssues runs -check-issues
// through main against a stand-in GitHub. Every issue open passes, each issue
// asked once, with the token. A closed issue fails naming the issue and the
// row that rests on it, which is the detection the check exists for; an
// answer that refuses the question fails as unable to judge, never as a pass.
// Either way the report is written first, and both reports were read whole,
// since -gaps-only drops the rows that name an issue.
func TestMain_CheckIssues_HoldsTheTrackedRowsToOpenIssues(t *testing.T) {
	cases := []struct {
		name      string
		answer    func(number int) (int, string)
		wantFatal string
		wantState map[int]string
	}{
		{
			name:      "every_issue_open",
			answer:    open,
			wantState: map[int]string{1602: issues.StateOpen, 1614: issues.StateOpen},
		},
		{
			name: "a_closed_issue",
			answer: func(number int) (int, string) {
				if number == 1614 {
					return http.StatusOK, `{"state": "closed", "state_reason": "completed"}`
				}
				return open(number)
			},
			wantFatal: "audit_1to1: tracked rows rest on issues that are not open or could not be asked about: #1614 is closed, named by 1 rows: routes DELETE /admin/queue;",
			wantState: map[int]string{1602: issues.StateOpen, 1614: issues.StateClosed},
		},
		{
			name: "a_refused_question",
			answer: func(number int) (int, string) {
				if number == 1602 {
					return http.StatusForbidden, `{"message": "API rate limit exceeded"}`
				}
				return open(number)
			},
			wantFatal: `#1602 is unjudged (GitHub answered 403 Forbidden: {"message": "API rate limit exceeded"}), named by 1 rows: sdk users.GetStatusInput.UserID;`,
			wantState: map[int]string{1602: issues.StateUnjudged, 1614: issues.StateOpen},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, calls := plantIssuesTree(t, map[string]string{"GITHUB_TOKEN": "workflow-token"})
			github := serveStandInGitHub(t, tc.answer)
			messages := captureFatal(t)
			output := filepath.Join(t.TempDir(), "issues.json")
			resetFlags(t, "-check-issues", "-output", output)

			main()

			switch {
			case tc.wantFatal == "" && len(*messages) != 0:
				t.Errorf("main reported %v, want the check to pass", *messages)
			case tc.wantFatal != "" && (len(*messages) != 1 || !strings.Contains((*messages)[0], tc.wantFatal)):
				t.Errorf("main reported %v, want one fatal message containing %q", *messages, tc.wantFatal)
			}
			asked, authorizations := github.requests()
			slices.Sort(asked)
			if !reflect.DeepEqual(asked, []int{1602, 1614}) {
				t.Errorf("GitHub was asked about %v, want each of 1602 and 1614 once", asked)
			}
			for _, authorization := range authorizations {
				if authorization != "Bearer workflow-token" {
					t.Errorf("GitHub was asked with Authorization %q, want the workflow's token", authorization)
				}
			}
			assertIssuesReport(t, output, tc.wantState)
			if !reflect.DeepEqual(calls.roots, []string{root, root}) || !reflect.DeepEqual(calls.routesOptions, []routes.Options{{}}) ||
				!reflect.DeepEqual(calls.sdkGapsOnly, []bool{false}) {
				t.Errorf("the scopes were asked at %v with routes %+v and sdk -gaps-only %v, want each once at %s with its whole report",
					calls.roots, calls.routesOptions, calls.sdkGapsOnly, root)
			}
		})
	}
}

// assertIssuesReport holds the report at path to the state each issue is in.
func assertIssuesReport(t *testing.T, path string, want map[int]string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the report: %v", err)
	}
	var report issues.Report
	if err = json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode the report %s: %v", content, err)
	}
	got := map[int]string{}
	for _, issue := range report.Issues {
		got[issue.Number] = issue.State
	}
	if !reflect.DeepEqual(got, want) || report.Repository != issues.DefaultRepository {
		t.Errorf("the report holds %v of %s, want %v of %s", got, report.Repository, want, issues.DefaultRepository)
	}
}

// TestCheckTrackedIssues_AFailure_IsNamedAndAsksNothing drives the check
// through every way it stops before asking GitHub, and holds each to
// returning what stopped it, asking nothing and writing nothing: a root that
// cannot be found, either report failing, reports that cannot be judged (one
// listing none of the routes its summary declares, as -gaps-only writes it),
// and, after asking, an output that cannot be written.
func TestCheckTrackedIssues_AFailure_IsNamedAndAsksNothing(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name      string
		arrange   func()
		output    func(t *testing.T) string
		want      string
		wantAsked bool
	}{
		{
			name:    "no_repository_root",
			arrange: func() { repositoryRoot = func(string) (string, error) { return "", boom } },
			want:    "find repository root: boom",
		},
		{
			name:    "the_routes_report_fails",
			arrange: func() { routesRun = func(string, routes.Options) ([]byte, bool, error) { return nil, false, boom } },
			want:    "routes report: boom",
		},
		{
			name:    "the_sdk_report_fails",
			arrange: func() { sdkRun = func(string, bool) ([]byte, bool, error) { return nil, false, boom } },
			want:    "sdk report: boom",
		},
		{
			name: "a_routes_report_without_its_declared_rows",
			arrange: func() {
				routesRun = func(string, routes.Options) ([]byte, bool, error) {
					return []byte(`{"summary": {"declared": 2}}`), true, nil
				}
			},
			want: "the routes report lists 0 of the 2 routes its summary declares: read it without -gaps-only",
		},
		{
			name: "an_output_it_cannot_write",
			output: func(t *testing.T) string {
				t.Helper()
				file := filepath.Join(t.TempDir(), "file")
				writeTree(t, filepath.Dir(file), map[string]string{"file": ""})
				return filepath.Join(file, "issues.json")
			},
			want:      "write output: ",
			wantAsked: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plantIssuesTree(t, nil)
			github := serveStandInGitHub(t, open)
			if tc.arrange != nil {
				tc.arrange()
			}
			output := filepath.Join(t.TempDir(), "issues.json")
			if tc.output != nil {
				output = tc.output(t)
			}
			err := checkTrackedIssues(context.Background(), output)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkTrackedIssues error = %v, want it to contain %q", err, tc.want)
			}
			if asked, _ := github.requests(); (len(asked) != 0) != tc.wantAsked {
				t.Errorf("GitHub was asked about %v, want asked = %t", asked, tc.wantAsked)
			}
			if _, statErr := os.Stat(output); statErr == nil {
				t.Errorf("the check wrote %s beside its error", output)
			}
		})
	}
}

// TestGitHubToken_TheWorkflowsTokenFirst holds the token GitHub is asked
// with to GITHUB_TOKEN when a workflow sets it, to GH_TOKEN, which the gh CLI
// reads, when it does not, and to none when neither is set, which GitHub
// answers within its anonymous rate limit.
func TestGitHubToken_TheWorkflowsTokenFirst(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "both", env: map[string]string{"GITHUB_TOKEN": "workflow", "GH_TOKEN": "cli"}, want: "workflow"},
		{name: "github_token_only", env: map[string]string{"GITHUB_TOKEN": "workflow"}, want: "workflow"},
		{name: "gh_token_only", env: map[string]string{"GH_TOKEN": "cli"}, want: "cli"},
		{name: "github_token_empty", env: map[string]string{"GITHUB_TOKEN": "", "GH_TOKEN": "cli"}, want: "cli"},
		{name: "neither", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plantIssuesTree(t, tc.env)
			if got := githubToken(); got != tc.want {
				t.Errorf("githubToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCheckTrackedIssues_TheTree_AsksAboutEveryTrackedIssue runs the check on
// the repository's own routes and sdk reports against a stand-in GitHub that
// answers every issue open, and holds it to asking about each issue the
// tracked rows name, once: the domain issues of R-ROUTE's family rows
// (#1209, and #1604 to #1634). The identifier issue of the sdk scope (#1602)
// left the list when the three user inputs it tracked were widened, which
// retired the last of its rows. A report field the check reads under another
// name would leave those rows unasked, which is what this catches. A change
// that tracks a row under a new issue, or retires the last row of one,
// changes the list here.
func TestCheckTrackedIssues_TheTree_AsksAboutEveryTrackedIssue(t *testing.T) {
	github := serveStandInGitHub(t, open)
	// No token of the environment the suite runs in goes to the stand-in.
	originalGetenv := getenv
	t.Cleanup(func() { getenv = originalGetenv })
	getenv = func(string) string { return "" }
	output := filepath.Join(t.TempDir(), "issues.json")
	if err := checkTrackedIssues(context.Background(), output); err != nil {
		t.Fatalf("checkTrackedIssues on the tree: %v", err)
	}
	want := []int{1209}
	for number := 1604; number <= 1634; number++ {
		want = append(want, number)
	}
	asked, _ := github.requests()
	slices.Sort(asked)
	if !reflect.DeepEqual(asked, want) {
		t.Errorf("GitHub was asked about %v, want %v", asked, want)
	}
}
