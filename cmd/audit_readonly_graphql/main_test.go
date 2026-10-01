package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests/actionfixture"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// errFixture is returned by the failing action source in the run tests.
var errFixture = errors.New("fixture catalog failure")

// repoRoot is the module root the fixture overlay is written under.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	root, err := actionfixture.Root(dir)
	if err != nil {
		t.Fatalf("module root: %v", err)
	}
	return root
}

// fixtureCache memoizes one loaded program per fixture source set. Loading is
// a full type-check of the fixture against toolutil and takes seconds, the
// result is read-only for everything the tests ask of it but the stand-ins
// the walk adds, which no other test reads, and the tests run in one
// goroutine.
var fixtureCache = map[string]*actionrequests.Program{}

// loadFixture loads the fixture packages described by sources.
func loadFixture(t *testing.T, sources map[string]string) *actionrequests.Program {
	t.Helper()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	key := strings.Join(names, "|")
	if cached, ok := fixtureCache[key]; ok {
		return cached
	}
	root := repoRoot(t)
	prog, err := actionrequests.Load(root, []string{actionfixture.Pattern}, actionfixture.Overlay(root, sources))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	fixtureCache[key] = prog
	return prog
}

// mainSources is the fixture set the detection tests load.
func mainSources() map[string]string {
	return actionfixture.Main()
}

// vulnSources is the fixture set holding only the vulnerability handlers.
func vulnSources() map[string]string {
	return map[string]string{"vuln": actionfixture.Vuln}
}

// vulnActions is the catalog the audit tests hand to [audit] for the vuln
// fixture: the same names the fixture declares, classified the same way.
func vulnActions() []action {
	return []action{
		{ID: "vuln.list", Name: "list", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.dismiss", Name: "dismiss", Owner: "vuln", ReadOnly: false},
		{ID: "vuln.read_dismiss", Name: "read_dismiss", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.read_dismiss_indirect", Name: "read_dismiss_indirect", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.read_inline", Name: "read_inline", Owner: "vuln", ReadOnly: true},
	}
}

// runFixture runs the audit over a fixture package set and returns the exit
// status with both streams.
func runFixture(t *testing.T, sources map[string]string, actions []action, verbose bool) (int, string, string) {
	t.Helper()
	root := repoRoot(t)
	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:      root,
		verbose:  verbose,
		patterns: []string{actionfixture.Pattern},
		overlay:  actionfixture.Overlay(root, sources),
		actions:  func() ([]action, error) { return actions, nil },
	}, &out, &errOut)
	return status, out.String(), errOut.String()
}

// TestRun_CleanCatalog_Succeeds verifies the passing path: read-only actions
// that reach no mutation exit zero and say what was checked, with the two
// counts in the sentence each from its own tally. The catalog is chosen so the
// tallies differ: two read-only actions are classified and one of them sends
// GraphQL, so a sentence that reported the GraphQL senders as the checked
// count, or the other way round, does not read the same.
func TestRun_CleanCatalog_Succeeds(t *testing.T) {
	actions := []action{
		{ID: "vuln.list", Name: "list", Owner: "vuln", ReadOnly: true},
		{ID: "shapes.quiet", Name: "quiet", Owner: "shapes", ReadOnly: true},
		{ID: "vuln.dismiss", Name: "dismiss", Owner: "vuln", ReadOnly: false},
	}

	status, out, errOut := runFixture(t, mainSources(), actions, false)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	if want := "audit_readonly_graphql: 2 read-only actions reach no GraphQL mutation (1 of them send GraphQL)\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errOut != "" {
		t.Errorf("a clean run wrote to stderr:\n%s", errOut)
	}
}

// TestRun_VerboseCleanCatalog_ListsTheGraphQLActions verifies the verbose
// report names the read-only actions that touch GraphQL, so the set a reviewer
// has to care about is visible rather than a count, and that its two counts
// are each the tally they claim to be. The excused fixture is loaded beside the
// vuln one so that one exception is in use while two actions send GraphQL: a
// report that printed the one figure under the other's label would otherwise
// read the same as a correct one.
func TestRun_VerboseCleanCatalog_ListsTheGraphQLActions(t *testing.T) {
	sources := map[string]string{"vuln": actionfixture.Vuln, "excused": excusedFixture}
	actions := []action{
		{ID: "vuln.list", Name: "list", Owner: "vuln", ReadOnly: true},
		{ID: "excused.risky_read", Name: "risky_read", Owner: "excused", ReadOnly: true},
	}

	status, out, errOut := runFixture(t, sources, actions, true)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	want := "audit_readonly_graphql: 1 declared exception(s) in use\n" +
		"audit_readonly_graphql: 2 read-only action(s) send GraphQL:\n" +
		"    vuln.list\n" +
		"    excused.risky_read\n" +
		"audit_readonly_graphql: 2 read-only actions reach no GraphQL mutation (2 of them send GraphQL)\n"
	if out != want {
		t.Errorf("stdout = %q\nwant %q", out, want)
	}
}

// TestRun_ReadOnlyActionReachingMutation_Fails verifies the failing path: a
// non-zero exit, the finding on stderr, and a count of problems beside a count
// of actions checked. A clean action is audited beside the violation so the two
// counts differ, since a summary that reported the problems as the actions
// checked would otherwise read the same.
func TestRun_ReadOnlyActionReachingMutation_Fails(t *testing.T) {
	actions := []action{
		{ID: "vuln.read_dismiss", Name: "read_dismiss", Owner: "vuln", ReadOnly: true},
		{ID: "vuln.list", Name: "list", Owner: "vuln", ReadOnly: true},
	}

	status, out, errOut := runFixture(t, vulnSources(), actions, false)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	for _, want := range []string{"vuln.read_dismiss is classified ReadOnly but its handler sends a GraphQL mutation.", "\naudit_readonly_graphql: 1 problem(s) across 2 read-only action(s)\n"} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not contain %q:\n%s", want, errOut)
			}
		})
	}
	if out != "" {
		t.Errorf("a failing run wrote the clean sentence to stdout:\n%s", out)
	}
}

// TestRun_CatalogSourceFails_Reports verifies a catalog that cannot be built
// is reported rather than treated as a catalog with nothing in it.
func TestRun_CatalogSourceFails_Reports(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:      repoRoot(t),
		patterns: []string{actionfixture.Pattern},
		actions:  func() ([]action, error) { return nil, errFixture },
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), errFixture.Error()) {
		t.Errorf("stderr does not carry the catalog error:\n%s", errOut.String())
	}
}

// TestRun_EmptyCatalog_Fails verifies an empty catalog fails rather than
// passing, since an audit with nothing to audit is not a passing audit.
func TestRun_EmptyCatalog_Fails(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:      repoRoot(t),
		patterns: []string{actionfixture.Pattern},
		actions:  func() ([]action, error) { return nil, nil },
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "the catalog is empty") {
		t.Errorf("stderr does not say the catalog was empty:\n%s", errOut.String())
	}
}

// TestRun_UnloadablePackages_Fails verifies a source tree that will not load
// fails the run instead of yielding an audit with no handlers to classify.
func TestRun_UnloadablePackages_Fails(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:      repoRoot(t),
		patterns: []string{"./internal/tools/testdata/..."},
		actions:  func() ([]action, error) { return vulnActions(), nil },
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "no packages matched") {
		t.Errorf("stderr does not report the load failure:\n%s", errOut.String())
	}
}

// TestAuditPatterns_CoverTheHandlers verifies the production patterns still
// name the tree the catalog handlers live in. A pattern that stopped matching
// them would leave every action unresolvable, which the audit reports, but
// naming the expectation here says why the pattern is what it is.
func TestAuditPatterns_CoverTheHandlers(t *testing.T) {
	if len(auditPatterns) != 1 || auditPatterns[0] != "./internal/..." {
		t.Errorf("auditPatterns = %v, want [./internal/...]", auditPatterns)
	}
}

// TestRunMain_TheCommandLine verifies each way a command line ends the run: -h
// asks for help and exits clean, a flag the command does not define exits 2
// as the flag package would, and a run over a tree with no source to load
// reaches the audit and fails there, naming what it could not load.
func TestRunMain_TheCommandLine(t *testing.T) {
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "go.mod"), []byte("module example.com/empty\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("prepare the module: %v", err)
	}
	previous := catalogActions
	t.Cleanup(func() { catalogActions = previous })
	catalogActions = func() ([]action, error) { return vulnActions(), nil }

	cases := []struct {
		name       string
		args       []string
		wantStatus int
		wantErr    string
	}{
		{name: "help", args: []string{"-h"}, wantStatus: 0, wantErr: "-dir string"},
		{name: "an unknown flag", args: []string{"-nope"}, wantStatus: 2, wantErr: "flag provided but not defined: -nope"},
		{name: "a run over nothing to load", args: []string{"-dir", empty, "-v"}, wantStatus: 1, wantErr: "audit_readonly_graphql: "},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			status := runMain(testCase.args, &out, &errOut)

			if status != testCase.wantStatus {
				t.Errorf("runMain(%q) = %d, want %d", testCase.args, status, testCase.wantStatus)
			}
			if !strings.Contains(errOut.String(), testCase.wantErr) {
				t.Errorf("runMain(%q) wrote to stderr %q, want %q among it", testCase.args, errOut.String(), testCase.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("runMain(%q) wrote to stdout:\n%s", testCase.args, out.String())
			}
		})
	}
}

// TestMain_HandsRunMainTheProcessAndExitsWithItsStatus verifies the one thing
// main does: it hands runMain the process's arguments after the program name
// and its standard error, and exits with what runMain returns.
func TestMain_HandsRunMainTheProcessAndExitsWithItsStatus(t *testing.T) {
	previousExit, previousArgs, previousStderr := exitProcess, os.Args, os.Stderr
	t.Cleanup(func() { exitProcess, os.Args, os.Stderr = previousExit, previousArgs, previousStderr })
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatalf("create the stream: %v", err)
	}
	var statuses []int
	exitProcess = func(status int) { statuses = append(statuses, status) }
	os.Args = []string{"audit_readonly_graphql", "-nope"}
	os.Stderr = stderr

	main()

	os.Stderr = previousStderr
	if len(statuses) != 1 || statuses[0] != 2 {
		t.Errorf("main() exited with %v, want [2]", statuses)
	}
	written, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatalf("read the stream: %v", err)
	}
	if !strings.Contains(string(written), "-nope") {
		t.Errorf("main() wrote %q to stderr, want the refused flag", written)
	}
}

// TestCatalogActions_IsTheSharedCatalog verifies the command reads the catalog
// the shared package builds, the two Ultimate builds with the standalone
// actions, rather than a narrower one of its own: Orbit's read is in it, and
// so is the project discovery.
func TestCatalogActions_IsTheSharedCatalog(t *testing.T) {
	actions, err := catalogActions()
	if err != nil {
		t.Fatalf("catalogActions: %v", err)
	}
	found := map[string]bool{}
	for _, item := range actions {
		found[item.ID] = true
	}
	for _, id := range []string{"orbit.status", "discover_project.resolve", "issue.list"} {
		t.Run(id, func(t *testing.T) {
			if !found[id] {
				t.Errorf("%s is not among the actions the audit answers for", id)
			}
		})
	}
}
