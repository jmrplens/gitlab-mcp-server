// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// goldenSetup is what scripts/setup-orbit-fixtures.sh did in one scenario,
// recorded from the script itself before the port replaced it: the routes a
// stand-in GitLab answered from, the settings and flag the run had, every
// request it sent in order, the clones and pushes it asked git for, and its
// exit status. The port is held to make the same requests, in the same order,
// with the same bodies, and to end the same way.
type goldenSetup struct {
	Scenario string                 `json:"scenario"`
	Env      map[string]string      `json:"env"`
	Flag     string                 `json:"flag"`
	Routes   map[string]goldenRoute `json:"routes"`
	Requests []goldenRequest        `json:"requests"`
	Git      []string               `json:"git"`
	Status   int                    `json:"status"`
}

// goldenRoute is one answer of the stand-in GitLab. A route is keyed by the
// method and the escaped path, with @<page> for one page of a paged read and
// #<n> for the nth request to it.
type goldenRoute struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// goldenRequest is one request a run sent.
type goldenRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Query  string `json:"query"`
	Body   string `json:"body"`
}

// fakeGitLab answers from a golden's routes as the script's stand-in curl
// did, and records every request.
type fakeGitLab struct {
	routes map[string]goldenRoute
	mu     sync.Mutex
	calls  map[string]int
	seen   []goldenRequest
}

func newFakeGitLab(routes map[string]goldenRoute) *fakeGitLab {
	return &fakeGitLab{routes: routes, calls: map[string]int{}}
}

// ServeHTTP answers one request: the route's answer for the nth call when it
// has one, else for the page asked for, else its only answer, and GitLab's
// 404 for a route it does not have.
func (f *fakeGitLab) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	key := r.Method + " " + r.URL.EscapedPath()
	f.mu.Lock()
	f.seen = append(f.seen, goldenRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Body: string(body)})
	f.calls[key]++
	n := f.calls[key]
	f.mu.Unlock()

	route, ok := f.routes[key]
	if page := r.URL.Query().Get("page"); page != "" {
		if paged, found := f.routes[key+"@"+page]; found {
			route, ok = paged, true
		}
	}
	if counted, found := f.routes[fmt.Sprintf("%s#%d", key, n)]; found {
		route, ok = counted, true
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"404 Not Found"}`)
		return
	}
	w.WriteHeader(route.Status)
	// A body recorded as a JSON string was served as its text, for an
	// answer that is not JSON. null decodes into a string too, and is
	// served as itself.
	var text string
	if bytes.HasPrefix(route.Body, []byte(`"`)) && json.Unmarshal(route.Body, &text) == nil {
		_, _ = io.WriteString(w, text)
		return
	}
	_, _ = w.Write(route.Body)
}

// requests is every request recorded so far.
func (f *fakeGitLab) requests() []goldenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.seen)
}

// standInToken is the GitLab.com token of the runs a test makes.
const standInToken = "stand-in-token"

// fakeGit stands in for git where a run reaches GitLab through it, logging
// each clone and push as the script's stand-in did: a clone copies the
// kg-fixtures fixture into the directory it names, and a push is logged with
// its arguments but the flags. The steps between them do nothing, and what a
// run reads of the fixture (ls-tree and show) goes to the real git.
//
// A clone from the instance and every push is refused, as GitLab.com refuses
// one that carries no token, unless it carries the token as an
// http.extraHeader: given to that command with -c, or configured in the
// directory it runs in with config --local. A -c value given to a clone is
// not kept in the clone, as git keeps none, so a push from the clone carries
// the token only if the clone was configured with it. The token is
// [standInToken] unless token names another.
type fakeGit struct {
	base, fixture string
	token         string
	mu            sync.Mutex
	log           []string
	fail          string
	// headers is the http.extraHeader configured in each directory.
	headers map[string]string
}

func (g *fakeGit) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	verb := gitVerb(args)
	if verb == g.fail {
		return nil, fmt.Errorf("git %s: exit status 128: fatal: stand-in refusal", verb)
	}
	switch verb {
	case "ls-tree", "show":
		return runGit(ctx, dir, args...)
	case "config":
		g.configure(dir, args)
	case "clone":
		return nil, g.clone(args)
	case "push":
		return nil, g.push(dir, args)
	}
	return nil, nil
}

// configure keeps the http.extraHeader a config command sets in dir.
func (g *fakeGit) configure(dir string, args []string) {
	i := slices.Index(args, "http.extraHeader")
	if i < 0 || i+1 >= len(args) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.headers == nil {
		g.headers = map[string]string{}
	}
	g.headers[dir] = args[i+1]
}

// clone copies the fixture into the directory args name last, or makes it
// empty for a mirror, refusing a clone from the instance without the token.
func (g *fakeGit) clone(args []string) error {
	target := args[len(args)-1]
	var source string
	for _, arg := range args {
		if strings.HasPrefix(arg, "http") {
			source = strings.ReplaceAll(arg, g.base, "{base}")
		}
	}
	if strings.HasPrefix(source, "{base}") && !g.carriesToken("", args) {
		return g.refuse("clone " + source)
	}
	if slices.Contains(args, "--mirror") {
		g.record("clone --mirror " + source)
		return os.MkdirAll(target, 0o750)
	}
	g.record("clone " + source)
	return os.CopyFS(target, os.DirFS(g.fixture))
}

// push logs a push run in dir with its arguments but the flags, refusing one
// without the token.
func (g *fakeGit) push(dir string, args []string) error {
	var kept []string
	for _, arg := range args[slices.Index(args, "push")+1:] {
		if arg != "--quiet" && arg != "-u" && arg != "--force" {
			kept = append(kept, strings.ReplaceAll(arg, g.base, "{base}"))
		}
	}
	line := "push " + strings.Join(kept, " ")
	if !g.carriesToken(dir, args) {
		return g.refuse(line)
	}
	g.record(line)
	return nil
}

// carriesToken reports whether a command run in dir with args sends the
// token: as a -c value of its own, or as the header dir was configured with.
// An empty dir stands for a command nothing was configured for.
func (g *fakeGit) carriesToken(dir string, args []string) bool {
	token := cmp.Or(g.token, standInToken)
	want := "PRIVATE-TOKEN: " + token
	for i, arg := range args[:len(args)-1] {
		if arg == "-c" && args[i+1] == "http.extraHeader="+want {
			return true
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return dir != "" && g.headers[dir] == want
}

// refuse logs line, a clone or a push, as refused and answers as git does
// when GitLab.com refuses a request that carries no token.
func (g *fakeGit) refuse(line string) error {
	g.record(line + " (refused: no token)")
	verb, _, _ := strings.Cut(line, " ")
	return fmt.Errorf("git %s: exit status 128: remote: HTTP Basic: Access denied", verb)
}

func (g *fakeGit) record(line string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.log = append(g.log, line)
}

// readGoldens reads every recorded scenario.
func readGoldens(t *testing.T) []goldenSetup {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "setup", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no setup goldens under testdata/setup: %v", err)
	}
	var goldens []goldenSetup
	for _, path := range paths {
		raw, readErr := os.ReadFile(path) //#nosec G304 -- a golden of this package's testdata
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		var golden goldenSetup
		if jsonErr := json.Unmarshal(raw, &golden); jsonErr != nil {
			t.Fatalf("decoding %s: %v", path, jsonErr)
		}
		goldens = append(goldens, golden)
	}
	return goldens
}

// setupAgainst runs setup with the golden's settings against server and git,
// and returns its exit code and what it wrote.
func setupAgainst(t *testing.T, golden goldenSetup, server *httptest.Server, git gitFunc) (int, string, string) {
	t.Helper()
	env := map[string]string{
		"GITLAB_COM_TOKEN":          standInToken,
		"ORBIT_FIXTURES_NAMESPACE":  "plens1",
		"ORBIT_FIXTURES_GITLAB_URL": server.URL,
	}
	maps.Copy(env, golden.Env)
	var stdout, stderr bytes.Buffer
	cfg, code, ok := setupFromArgs([]string{golden.Flag, "-root", filepath.Join("..", "..")}, func(key string) string { return env[key] }, &stderr)
	if !ok {
		t.Fatalf("setup flags refused (%d): %s", code, stderr.String())
	}
	cfg.api.client = server.Client()
	cfg.git = git
	cfg.mergeInterval = 0
	cfg.sleep = func(context.Context, time.Duration) error { return nil }
	return runSetup(t.Context(), cfg, &stdout, &stderr), stdout.String(), stderr.String()
}

// TestSetup_RecordedScenarios_SendWhatTheScriptSent holds the port to what
// scripts/setup-orbit-fixtures.sh did, scenario by scenario: the same
// requests in the same order, each to the same path with the same query and
// body (compared as values, so the order of query parameters and JSON keys
// does not count), the same clones and pushes, and the same exit status.
func TestSetup_RecordedScenarios_SendWhatTheScriptSent(t *testing.T) {
	for _, golden := range readGoldens(t) {
		t.Run(golden.Scenario, func(t *testing.T) {
			fake := newFakeGitLab(golden.Routes)
			server := httptest.NewServer(fake)
			t.Cleanup(server.Close)
			git := &fakeGit{base: server.URL, fixture: filepath.Join("..", "..", "test", "fixtures", "orbit", "kg-fixtures")}

			code, stdout, stderr := setupAgainst(t, golden, server, git.run)

			if code != golden.Status {
				t.Errorf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, golden.Status, stdout, stderr)
			}
			got := fake.requests()
			for i := range max(len(got), len(golden.Requests)) {
				switch {
				case i >= len(got):
					t.Errorf("request %d: none sent, want %s", i+1, describe(golden.Requests[i]))
				case i >= len(golden.Requests):
					t.Errorf("request %d: %s sent, want none", i+1, describe(got[i]))
				case !sameRequest(got[i], golden.Requests[i]):
					t.Errorf("request %d: %s sent, want %s", i+1, describe(got[i]), describe(golden.Requests[i]))
				}
			}
			if !slices.Equal(git.log, golden.Git) && (len(git.log) > 0 || len(golden.Git) > 0) {
				t.Errorf("git was asked for %q, want %q", git.log, golden.Git)
			}
		})
	}
}

// serveRoutes starts a stand-in GitLab answering from routes.
func serveRoutes(t *testing.T, routes map[string]goldenRoute) (*httptest.Server, *fakeGitLab) {
	t.Helper()
	fake := newFakeGitLab(routes)
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return server, fake
}

// TestSetup_WhatStopsAProject_StopsTheRun checks each way a fixture project
// cannot be put in place stops the run with the reason: a tree answered
// without a list, with null or with something that is not JSON, a tree that
// never ends, a group without an id or answered with no JSON, a creation GitLab
// answers without an id or with no JSON, a project lookup answered with no
// JSON (read as no project, so a creation is asked for), a clone or push of a
// new project or of the mirror that git refuses, a fixture git cannot list,
// and a visibility GitLab refuses to set.
func TestSetup_WhatStopsAProject_StopsTheRun(t *testing.T) {
	const kgProject = "GET /api/v4/projects/plens1%2Fkg-fixtures"
	cases := []struct {
		name    string
		change  func(routes map[string]goldenRoute)
		failGit string
		mirror  bool
		want    string
	}{
		{
			name: "a tree that is not a list",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, map[string]string{"message": "x"})
			},
			want: "the project's file list is unreadable",
		},
		{
			name: "a tree answered null",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, nil)
			},
			want: "the project's file list is unreadable",
		},
		{
			name: "a tree that never ends",
			change: func(routes map[string]goldenRoute) {
				var page []map[string]string
				for i := range 100 {
					page = append(page, map[string]string{"path": fmt.Sprintf("filler/%d", i)})
				}
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, page)
			},
			want: "the project's file list runs past 100 pages",
		},
		{
			name: "a group without an id",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/groups/plens1"] = jsonRoute(t, 200, map[string]any{"id": 0, "full_path": "plens1"})
			},
			want: "the namespace did not resolve to a group with an id and a full_path",
		},
		{
			name: "a group answered with no JSON",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/groups/plens1"] = jsonRoute(t, 200, "<html>")
			},
			want: "the namespace did not resolve to a group with an id and a full_path: <html>",
		},
		{
			name: "a project answered with no JSON is created, and the creation refused",
			change: func(routes map[string]goldenRoute) {
				routes[kgProject] = jsonRoute(t, 200, "<html>")
				routes["POST /api/v4/projects"] = jsonRoute(t, 400, map[string]string{"message": "name has already been taken"})
			},
			want: "failed to create project",
		},
		{
			name: "a creation answered with no JSON",
			change: func(routes map[string]goldenRoute) {
				delete(routes, kgProject)
				routes["POST /api/v4/projects"] = jsonRoute(t, 201, "<html>")
			},
			want: "failed to create project: <html>",
		},
		{
			name: "a tree that is not JSON",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, "<html>")
			},
			want: "the project's file list is unreadable",
		},
		{
			name: "an unreadable tree",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 500, map[string]string{"message": "boom"})
			},
			want: "cannot list the project's files (HTTP 500)",
		},
		{
			name: "a project without a readable visibility",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10"] = jsonRoute(t, 200, map[string]any{"id": 10})
			},
			want: "project 10 has no readable visibility",
		},
		{
			name: "a refused visibility",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10"] = jsonRoute(t, 200, map[string]any{"id": 10, "visibility": "private"})
				routes["PUT /api/v4/projects/10"] = jsonRoute(t, 403, map[string]string{"message": "forbidden"})
			},
			want: "could not set visibility (HTTP 403)",
		},
		{
			name: "a refused commit of the missing files",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, []map[string]string{{"path": "README.md"}})
				routes["POST /api/v4/projects/10/repository/commits"] = jsonRoute(t, 400, map[string]string{"message": "bad request"})
			},
			want: "could not add the missing fixture files (HTTP 400)",
		},
		{
			name:    "a fixture git cannot list",
			failGit: "ls-tree",
			want:    "cannot compare the project's files with the fixture",
		},
		{
			name: "a file git cannot show",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, []map[string]string{{"path": "README.md"}})
			},
			failGit: "show",
			want:    "cannot compare the project's files with the fixture",
		},
		{
			name: "a creation without an id",
			change: func(routes map[string]goldenRoute) {
				delete(routes, kgProject)
				routes["POST /api/v4/projects"] = jsonRoute(t, 400, map[string]string{"message": "name has already been taken"})
			},
			want: "failed to create project: {\"message\":\"name has already been taken\"}",
		},
		{
			name: "a new project git cannot clone",
			change: func(routes map[string]goldenRoute) {
				delete(routes, kgProject)
				routes["POST /api/v4/projects"] = jsonRoute(t, 201, map[string]any{"id": 10})
			},
			failGit: "clone",
			want:    "git clone: exit status 128",
		},
		{
			name: "a new project git cannot push",
			change: func(routes map[string]goldenRoute) {
				delete(routes, kgProject)
				routes["POST /api/v4/projects"] = jsonRoute(t, 201, map[string]any{"id": 10})
			},
			failGit: "push",
			want:    "git push: exit status 128",
		},
		{
			name: "a new project whose fixture git cannot list",
			change: func(routes map[string]goldenRoute) {
				delete(routes, kgProject)
				routes["POST /api/v4/projects"] = jsonRoute(t, 201, map[string]any{"id": 10})
			},
			failGit: "ls-tree",
			want:    "git ls-tree: exit status 128",
		},
		{
			name: "a new project whose fixture git cannot show",
			change: func(routes map[string]goldenRoute) {
				delete(routes, kgProject)
				routes["POST /api/v4/projects"] = jsonRoute(t, 201, map[string]any{"id": 10})
			},
			failGit: "show",
			want:    "git show: exit status 128",
		},
		{
			name: "a mirror git cannot clone",
			change: func(routes map[string]goldenRoute) {
				routes["POST /api/v4/projects"] = jsonRoute(t, 201, map[string]any{"id": 30})
			},
			mirror:  true,
			failGit: "clone",
			want:    "git clone: exit status 128",
		},
		{
			name: "the security fixture refused",
			change: func(routes map[string]goldenRoute) {
				routes["GET /api/v4/projects/20"] = jsonRoute(t, 500, map[string]string{"message": "boom"})
			},
			want: "cannot read project 20 (HTTP 500)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routes := inLineRoutes(t)
			if tc.change != nil {
				tc.change(routes)
			}
			code, stderr := setupCase(t, routes, tc.failGit, func(cfg *setupConfig) { cfg.mirror = tc.mirror })
			if code != 1 || !strings.Contains(stderr, tc.want) {
				t.Errorf("exit code %d, stderr:\n%s\nwant 1 and %q", code, stderr, tc.want)
			}
		})
	}
}

// TestAddMissingFiles_WhatTheProjectHolds_DecidesEachFile checks which files
// the commit of a reconciliation carries and how, and what the run says of
// each: a directory in the tree does not stand in for the fixture file of its
// name, which is added; a project holding GitLab's README alone has it
// replaced and every other file added; and a project holding one file that is
// not the README has every fixture file added, the README among them.
func TestAddMissingFiles_WhatTheProjectHolds_DecidesEachFile(t *testing.T) {
	tracked, err := trackedFiles(t.Context(), runGit, kgFixture)
	if err != nil {
		t.Skipf("git cannot list the fixture here: %v", err)
	}
	var withoutLicense []map[string]string
	for _, file := range tracked {
		if file != "LICENSE" {
			withoutLicense = append(withoutLicense, map[string]string{"path": file})
		}
	}
	allCreated := map[string]string{}
	for _, file := range tracked {
		allCreated[file] = "create"
	}
	readmeReplaced := maps.Clone(allCreated)
	readmeReplaced["README.md"] = "update"
	cases := []struct {
		name       string
		tree       any
		want       map[string]string
		wantStderr []string
	}{
		{
			name:       "a directory named like a fixture file",
			tree:       append(slices.Clone(withoutLicense), map[string]string{"path": "LICENSE", "type": "tree"}),
			want:       map[string]string{"LICENSE": "create"},
			wantStderr: []string{"→ added LICENSE"},
		},
		{
			name:       "GitLab's README alone",
			tree:       []map[string]string{{"path": "README.md"}},
			want:       readmeReplaced,
			wantStderr: []string{"→ replaced README.md", "→ added LICENSE"},
		},
		{
			name:       "one file that is not the README",
			tree:       []map[string]string{{"path": "NOTES.md"}},
			want:       allCreated,
			wantStderr: []string{"→ added README.md", "→ added LICENSE"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			routes := inLineRoutes(t)
			routes["GET /api/v4/projects/10/repository/tree"] = jsonRoute(t, 200, tc.tree)
			routes["POST /api/v4/projects/10/repository/commits"] = jsonRoute(t, 201, map[string]string{"id": "c0"})
			server, fake := serveRoutes(t, routes)
			git := &fakeGit{base: server.URL, fixture: kgFixture}
			code, _, stderr := setupAgainst(t, goldenSetup{Flag: "--skip-mirror"}, server, git.run)
			if code != 0 {
				t.Fatalf("exit code %d, stderr:\n%s", code, stderr)
			}
			if got := committedActions(t, fake.requests()); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("the commit carries %v, want %v", got, tc.want)
			}
			for _, line := range tc.wantStderr {
				if !strings.Contains(stderr, line) {
					t.Errorf("stderr does not say %q:\n%s", line, stderr)
				}
			}
		})
	}
}

// committedActions is what the commits of kg-fixtures among requests carry:
// each file path with its action.
func committedActions(t *testing.T, requests []goldenRequest) map[string]string {
	t.Helper()
	actions := map[string]string{}
	for _, request := range requests {
		if request.Method != http.MethodPost || request.Path != "/api/v4/projects/10/repository/commits" {
			continue
		}
		var commit struct{ Actions []fileAction }
		if err := json.Unmarshal([]byte(request.Body), &commit); err != nil {
			t.Fatalf("commit body %q: %v", request.Body, err)
		}
		for _, action := range commit.Actions {
			actions[action.FilePath] = action.Action
		}
	}
	return actions
}

// TestPushFixture_NewProject_HoldsOnlyWhatHEADTracks checks a new project is
// pushed with the files git tracks in the fixture, as HEAD holds them, and
// without GitLab's README or a file the working tree holds untracked: these
// projects are public, and the script it replaces copied the working tree.
func TestPushFixture_NewProject_HoldsOnlyWhatHEADTracks(t *testing.T) {
	fixture := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@example.test", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "root"},
	} {
		if _, err := runGit(t.Context(), fixture, args...); err != nil {
			t.Skipf("git cannot make a repository here: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(fixture, "LICENSE"), []byte("MIT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fixture, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "src", "a.py"), []byte("committed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// sequential: the commit records what the add staged.
	for _, args := range [][]string{{"add", "LICENSE", "src/a.py"}, {"-c", "user.email=t@example.test", "-c", "user.name=t", "commit", "-q", "-m", "fixture"}} {
		if _, err := runGit(t.Context(), fixture, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(fixture, "src", "a.py"), []byte("an uncommitted edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, ".env"), []byte("SECRET=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var pushed map[string]string
	git := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		switch gitVerb(args) {
		case "clone":
			target := args[len(args)-1]
			if err := os.MkdirAll(target, 0o750); err != nil {
				return nil, err
			}
			return nil, os.WriteFile(filepath.Join(target, "README.md"), []byte("GitLab's\n"), 0o600)
		case "push":
			pushed = map[string]string{}
			tree := os.DirFS(dir)
			err := fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				content, readErr := fs.ReadFile(tree, path)
				pushed[path] = string(content)
				return readErr
			})
			return nil, err
		case "ls-tree", "show":
			return runGit(ctx, dir, args...)
		}
		return nil, nil
	}
	run := &provisioner{token: "x", gitlabURL: "https://gitlab.example.test", git: git, out: io.Discard, errOut: io.Discard}

	if err := run.pushFixture(t.Context(), fixture, "https://gitlab.example.test/acme/kg-fixtures.git"); err != nil {
		t.Fatalf("pushFixture: %v", err)
	}
	want := map[string]string{"LICENSE": "MIT\n", "src/a.py": "committed\n"}
	if !reflect.DeepEqual(pushed, want) {
		t.Errorf("pushed %q, want %q", pushed, want)
	}
}

// TestPushes_WhatTheWorkDirectoryRefuses_StopsThePush checks every push stops
// when its work directory cannot be made, when GitLab's README cannot be
// removed, and when a fixture file cannot be written where it goes, and that
// a mirror stops when git refuses its push.
func TestPushes_WhatTheWorkDirectoryRefuses_StopsThePush(t *testing.T) {
	fixture := func(run *provisioner) error { return run.pushFixture(t.Context(), kgFixture, "r") }
	cases := []struct {
		name string
		// clone are the directories the stand-in clone leaves under the
		// directory it is given.
		clone  []string
		listed string
		push   func(*provisioner) error
		want   error
	}{
		{name: "a README that cannot be removed", clone: []string{"README.md/inside/"}, push: fixture},
		{name: "a file where a directory goes", listed: "a\x00a/b\x00", push: fixture},
		{name: "a directory where a file goes", clone: []string{"d/x/y/"}, listed: "d/x\x00", push: fixture},
		{name: "a branch whose clone has no fulfillment module", push: func(run *provisioner) error { return run.pushMergeRequestBranch(t.Context()) }, want: os.ErrNotExist},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			git := func(_ context.Context, _ string, args ...string) ([]byte, error) {
				switch gitVerb(args) {
				case "clone":
					return nil, makeTree(args[len(args)-1], tc.clone)
				case "ls-tree":
					return []byte(tc.listed), nil
				case "show":
					return []byte("x"), nil
				}
				return nil, nil
			}
			err := tc.push(pushRun(git))
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Errorf("the push = %v, want it stopped", err)
			}
		})
	}
}

// TestPushes_NoTemporaryDirectory_StopsEveryPush checks the fixture, mirror
// and branch pushes each stop when their work directory cannot be made.
func TestPushes_NoTemporaryDirectory_StopsEveryPush(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}
	run := pushRun(func(context.Context, string, ...string) ([]byte, error) { return nil, nil })
	for name, push := range map[string]func() error{
		"fixture": func() error { return run.pushFixture(t.Context(), kgFixture, "r") },
		"mirror":  func() error { return run.pushMirror(t.Context(), "u", "r") },
		"branch":  func() error { return run.pushMergeRequestBranch(t.Context()) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := push(); err == nil {
				t.Errorf("the %s push went on without a work directory", name)
			}
		})
	}
}

// TestPushMirror_APushGitRefuses_StopsTheMirror checks a mirror whose push
// git refuses is reported with git's subcommand.
func TestPushMirror_APushGitRefuses_StopsTheMirror(t *testing.T) {
	git := &fakeGit{base: "https://gitlab.example.test", fixture: kgFixture, fail: "push"}
	if err := pushRun(git.run).pushMirror(t.Context(), "u", "r"); err == nil || !strings.Contains(err.Error(), "git push") {
		t.Errorf("pushMirror = %v", err)
	}
}

// TestPushes_ToGitLab_CarryTheToken checks the clone from GitLab.com and the
// push to it of the fixture, mirror and branch pushes each carry the run's
// token, the push included: a -c value given to the clone is not kept in it,
// so a push that relied on it would be refused by GitLab.com, as the stand-in
// git refuses it. A git expecting another token refuses the first request of
// each, so what is sent is the run's token and not a constant.
func TestPushes_ToGitLab_CarryTheToken(t *testing.T) {
	const base = "https://gitlab.example.test"
	cases := []struct {
		name string
		push func(*provisioner) error
		want []string
	}{
		{
			name: "fixture",
			push: func(run *provisioner) error {
				return run.pushFixture(t.Context(), kgFixture, base+"/acme/kg-fixtures.git")
			},
			want: []string{"clone {base}/acme/kg-fixtures.git", "push origin main"},
		},
		{
			name: "mirror",
			push: func(run *provisioner) error { return run.pushMirror(t.Context(), mirrorUpstream, base+"/acme/cli.git") },
			want: []string{"clone --mirror " + mirrorUpstream, "push {base}/acme/cli.git refs/heads/*:refs/heads/* refs/tags/*:refs/tags/*"},
		},
		{
			name: "branch",
			push: func(run *provisioner) error { return run.pushMergeRequestBranch(t.Context()) },
			want: []string{"clone {base}/acme/kg-fixtures.git", "push origin " + mrBranch},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			git := &fakeGit{base: base, fixture: kgFixture}
			run := pushRun(git.run)
			run.token = standInToken
			if err := tc.push(run); err != nil {
				t.Fatalf("the %s push = %v, git was asked for %q", tc.name, err, git.log)
			}
			if !slices.Equal(git.log, tc.want) {
				t.Errorf("git was asked for %q, want %q", git.log, tc.want)
			}
		})
		t.Run(tc.name+" with another token", func(t *testing.T) {
			git := &fakeGit{base: base, fixture: kgFixture, token: "another-token"}
			run := pushRun(git.run)
			run.token = standInToken
			if err := tc.push(run); err == nil || !strings.Contains(err.Error(), "Access denied") {
				t.Errorf("the %s push = %v, want it refused; git was asked for %q", tc.name, err, git.log)
			}
		})
	}
}

// pushRun is a run of setup over git, for a push called on its own.
func pushRun(git gitFunc) *provisioner {
	return &provisioner{token: "x", gitlabURL: "https://gitlab.example.test", namespace: "acme", git: git, out: io.Discard, errOut: io.Discard}
}

// makeTree makes root and, under it, every directory of dirs.
func makeTree(root string, dirs []string) error {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o750); err != nil {
			return err
		}
	}
	return nil
}

// TestRefuseNonPublic_Namespaces_OnlyJmrpOssIsHeldToPublic checks the guard
// is about jmrp-oss and its subgroups alone, whatever their case, and only
// for a visibility other than public.
func TestRefuseNonPublic_Namespaces_OnlyJmrpOssIsHeldToPublic(t *testing.T) {
	cases := []struct {
		path, visibility string
		refused          bool
	}{
		{path: "jmrp-oss", visibility: "private", refused: true},
		{path: "JMRP-OSS/Orbit-Fixtures", visibility: "internal", refused: true},
		{path: "jmrp-oss/orbit-fixtures", visibility: "public"},
		{path: "jmrp-ossified", visibility: "private"},
		{path: "acme", visibility: "private"},
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.visibility, func(t *testing.T) {
			run := &provisioner{visibility: tc.visibility}
			err := run.refuseNonPublic(tc.path)
			if (err != nil) != tc.refused || (err != nil && !errors.Is(err, errRefused)) {
				t.Errorf("refuseNonPublic = %v, want refused %t", err, tc.refused)
			}
		})
	}
}

// describe is a request as a failure names it.
func describe(request goldenRequest) string {
	text := request.Method + " " + request.Path
	if request.Query != "" {
		text += "?" + request.Query
	}
	if request.Body != "" {
		text += " " + request.Body
	}
	return text
}

// sameRequest reports whether two requests are the same request: method and
// escaped path as sent, the query and the JSON body as values.
func sameRequest(got, want goldenRequest) bool {
	if got.Method != want.Method || got.Path != want.Path {
		return false
	}
	gotQuery, gotErr := url.ParseQuery(got.Query)
	wantQuery, wantErr := url.ParseQuery(want.Query)
	if gotErr != nil || wantErr != nil || !reflect.DeepEqual(gotQuery, wantQuery) {
		return false
	}
	if got.Body == "" || want.Body == "" {
		return got.Body == want.Body
	}
	var gotBody, wantBody any
	if json.Unmarshal([]byte(got.Body), &gotBody) != nil || json.Unmarshal([]byte(want.Body), &wantBody) != nil {
		return got.Body == want.Body
	}
	return reflect.DeepEqual(withoutContent(gotBody), withoutContent(wantBody))
}

// withoutContent drops the content of every file action of a commit body,
// which is the fixture file as HEAD held it when the golden was recorded:
// a golden is about which files a commit carries and how, and
// TestSetup_CommittedFiles_CarryWhatHEADHolds holds the content to HEAD as it
// is now, so a fixture edited later does not stale every golden.
func withoutContent(body any) any {
	object, isObject := body.(map[string]any)
	if !isObject {
		return body
	}
	actions, hasActions := object["actions"].([]any)
	if !hasActions {
		return body
	}
	for _, action := range actions {
		if fields, ok := action.(map[string]any); ok {
			delete(fields, "content")
		}
	}
	return object
}

// TestSetup_CommittedFiles_CarryWhatHEADHolds checks every file a recorded
// scenario commits carries the bytes HEAD holds for it in the fixture, base64
// encoded, never the working tree's.
func TestSetup_CommittedFiles_CarryWhatHEADHolds(t *testing.T) {
	var checked int
	for _, golden := range readGoldens(t) {
		t.Run(golden.Scenario, func(t *testing.T) {
			fake := newFakeGitLab(golden.Routes)
			server := httptest.NewServer(fake)
			t.Cleanup(server.Close)
			git := &fakeGit{base: server.URL, fixture: kgFixture}
			setupAgainst(t, golden, server, git.run)
			for _, request := range fake.requests() {
				if request.Method == http.MethodPost && strings.HasSuffix(request.Path, "/repository/commits") {
					checked += checkCommittedFiles(t, golden.Scenario, request)
				}
			}
		})
	}
	if checked == 0 {
		t.Error("no recorded scenario commits a file, so nothing was checked")
	}
}

// checkCommittedFiles holds every file action of one commit request to the
// bytes HEAD holds for it in the fixture of the project committed to, and
// returns how many it checked.
func checkCommittedFiles(t *testing.T, scenario string, request goldenRequest) int {
	t.Helper()
	fixture := kgFixture
	if strings.Contains(request.Path, "/projects/20/") {
		fixture = filepath.Join("..", "..", "test", "fixtures", "orbit", "security-fixtures")
	}
	var commit struct{ Actions []fileAction }
	if err := json.Unmarshal([]byte(request.Body), &commit); err != nil {
		t.Fatalf("%s: commit body %q: %v", scenario, request.Body, err)
	}
	for _, action := range commit.Actions {
		committed, err := committedBytes(t.Context(), runGit, fixture, action.FilePath)
		if err != nil {
			t.Fatalf("%s: %s at HEAD: %v", scenario, action.FilePath, err)
		}
		if action.Encoding != "base64" || action.Content != base64.StdEncoding.EncodeToString(committed) {
			t.Errorf("%s: %s carries other bytes than HEAD holds", scenario, action.FilePath)
		}
	}
	return len(commit.Actions)
}
