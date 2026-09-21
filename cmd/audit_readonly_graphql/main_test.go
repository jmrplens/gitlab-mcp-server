package main

import (
	"bytes"
	"strings"
	"testing"
)

// runFixture runs the audit over a fixture package set and returns the exit
// status with both streams.
func runFixture(t *testing.T, sources map[string]string, actions []action, verbose bool) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:      repoRoot(t),
		verbose:  verbose,
		patterns: []string{fixturePattern},
		overlay:  fixtureOverlay(t, sources),
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
	sources := map[string]string{"vuln": vulnFixture, "excused": excusedFixture}
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
		patterns: []string{fixturePattern},
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
		patterns: []string{fixturePattern},
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
