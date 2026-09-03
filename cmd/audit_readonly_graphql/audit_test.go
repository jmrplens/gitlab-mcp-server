package main

import (
	"go/token"
	"sort"
	"strings"
	"testing"
)

// excusedFixture is a read-only action that really does reach a mutation and
// says so in the source, beside the action, with the directive that excuses it.
const excusedFixture = `package excused

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const writeMutation = @@
mutation($id: ID!) {
  thingTouch(input: {id: $id}) { errors }
}
@@

// Input is the fixture handler input.
type Input struct {
	ID string ` + "`json:\"id\"`" + `
}

// Output is the fixture handler output.
type Output struct {
	OK bool ` + "`json:\"ok\"`" + `
}

// Risky reads, and reaches a mutation on the way.
func Risky(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var response struct {
		Data map[string]any ` + "`json:\"data\"`" + `
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     writeMutation,
		Variables: map[string]any{"id": input.ID},
	}, &response, gl.WithContext(ctx))
	return Output{OK: err == nil}, err
}

// ActionSpecs declares the excused action.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		//gitlab:allow-readonly-graphql-mutation risky_read: the fixture that proves an exception is declared beside the action.
		toolutil.NewReadActionSpec("risky_read", toolutil.RouteAction(client, Risky), toolutil.ActionSpecOptions{}),
	}
}
`

// staleFixture declares an exception for an action that reaches no mutation,
// which is the shape an exception left behind by a later fix takes.
const staleFixture = `package stale

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input is the fixture handler input.
type Input struct {
	ID string ` + "`json:\"id\"`" + `
}

// Output is the fixture handler output.
type Output struct {
	OK bool ` + "`json:\"ok\"`" + `
}

// Quiet touches nothing.
func Quiet(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	_ = ctx
	_ = client
	return Output{OK: input.ID != ""}, nil
}

// ActionSpecs declares an action whose exception no longer excuses anything.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		//gitlab:allow-readonly-graphql-mutation quiet_read: left behind after the mutation was removed.
		toolutil.NewReadActionSpec("quiet_read", toolutil.RouteAction(client, Quiet), toolutil.ActionSpecOptions{}),
	}
}
`

// exceptionSources is the fixture set for the exception tests, kept apart from
// the detection fixtures because a directive is global to the loaded program:
// an unused one is a finding, so it would show up in every other test.
func exceptionSources() map[string]string {
	return map[string]string{"excused": excusedFixture, "stale": staleFixture}
}

// exceptionActions is the catalog for the exception fixtures.
func exceptionActions() []action {
	return []action{
		{ID: "excused.risky_read", Name: "risky_read", Owner: "excused", ReadOnly: true},
		{ID: "stale.quiet_read", Name: "quiet_read", Owner: "stale", ReadOnly: true},
	}
}

// findingActions returns the action ID of every finding, sorted.
func findingActions(result auditResult) []string {
	ids := make([]string, 0, len(result.findings))
	for _, item := range result.findings {
		ids = append(ids, item.action)
	}
	sort.Strings(ids)
	return ids
}

// messageFor returns the finding reported for one action, or "".
func messageFor(result auditResult, actionID string) string {
	for _, item := range result.findings {
		if item.action == actionID {
			return item.message
		}
	}
	return ""
}

// TestAudit_ReadOnlyActionReachingMutation_IsReported is the case this audit
// exists for: an action classified ReadOnly whose handler sends a mutation,
// whether the handler sends it directly, reaches it through a callee, writes
// the document inline, or runs it from a function-literal route. The actions
// that only query, and the action classified as mutating, must stay clean.
func TestAudit_ReadOnlyActionReachingMutation_IsReported(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := append(vulnActions(), shapesActions()...)

	result := audit(prog, actions, repoRoot(t))

	want := []string{
		"shapes.closure",
		"vuln.read_dismiss",
		"vuln.read_dismiss_indirect",
		"vuln.read_inline",
	}
	if got := findingActions(result); !equalStrings(got, want) {
		t.Errorf("findings for %v, want %v", got, want)
	}
}

// TestAudit_ReadOnlyActionSendingOnlyQueries_IsClean verifies a read-only
// action whose handler sends GraphQL queries is not a finding, which is the
// whole reason the HTTP method cannot be the test: these actions POST.
func TestAudit_ReadOnlyActionSendingOnlyQueries_IsClean(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := []action{
		{ID: "vuln.list", Name: "list", Owner: "vuln", ReadOnly: true},
		{ID: "shapes.direct", Name: "direct", Owner: "shapes", ReadOnly: true},
	}

	result := audit(prog, actions, repoRoot(t))

	if len(result.findings) != 0 {
		t.Errorf("query-only read actions produced findings: %v", findingActions(result))
	}
	if result.checked != 2 {
		t.Errorf("checked %d actions, want 2", result.checked)
	}
	if !equalStrings(result.graphQL, []string{"vuln.list", "shapes.direct"}) {
		t.Errorf("actions reported as sending GraphQL = %v, want both fixtures", result.graphQL)
	}
}

// TestAudit_MutatingActionReachingMutation_IsNotAFinding verifies the rule the
// issue states directly: a mutation reached from an action already classified
// as mutating is not a finding, because nothing claims that action is safe.
func TestAudit_MutatingActionReachingMutation_IsNotAFinding(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := []action{{ID: "vuln.dismiss", Name: "dismiss", Owner: "vuln", ReadOnly: false}}

	result := audit(prog, actions, repoRoot(t))

	if len(result.findings) != 0 {
		t.Errorf("a mutating action was reported: %v", findingActions(result))
	}
	if result.checked != 0 {
		t.Errorf("checked %d actions, want 0: a mutating action is not classified at all", result.checked)
	}
}

// TestAudit_ActionSendingNoGraphQL_IsNotAFinding verifies the other rule the
// issue states: an action that sends no GraphQL is not a finding, and is not
// counted among the actions that touch GraphQL either.
func TestAudit_ActionSendingNoGraphQL_IsNotAFinding(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := []action{{ID: "shapes.quiet", Name: "quiet", Owner: "shapes", ReadOnly: true}}

	result := audit(prog, actions, repoRoot(t))

	if len(result.findings) != 0 {
		t.Errorf("a REST-only action was reported: %v", findingActions(result))
	}
	if len(result.graphQL) != 0 {
		t.Errorf("a REST-only action was counted as sending GraphQL: %v", result.graphQL)
	}
}

// TestAudit_FindingNamesTheActionAndTheFile verifies a failure says which
// action and which file, which is what the issue asks a failure to name.
func TestAudit_FindingNamesTheActionAndTheFile(t *testing.T) {
	prog := loadFixture(t, mainSources())
	result := audit(prog, vulnActions(), repoRoot(t))

	message := messageFor(result, "vuln.read_dismiss")
	if message == "" {
		t.Fatal("the constructed violation produced no finding")
	}
	for _, want := range []string{"vuln.read_dismiss", "dismissMutation", fixtureDir + "/vuln/vuln.go:"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(message, want) {
				t.Errorf("finding does not mention %q:\n%s", want, message)
			}
		})
	}
}

// TestAudit_InlineDocument_IsNamedAsInline verifies a mutation written at the
// point of use is reported as such, since there is no constant name to print.
func TestAudit_InlineDocument_IsNamedAsInline(t *testing.T) {
	prog := loadFixture(t, mainSources())
	result := audit(prog, vulnActions(), repoRoot(t))

	message := messageFor(result, "vuln.read_inline")
	if !strings.Contains(message, "an inline mutation document") {
		t.Errorf("finding does not describe the inline document:\n%s", message)
	}
}

// TestAudit_UnresolvedAction_IsReported verifies an action the audit cannot
// place is a failure rather than a silent pass. A gate that skips what it
// cannot resolve is a gate that stops holding the moment a domain is written
// in a shape the resolver does not follow.
func TestAudit_UnresolvedAction_IsReported(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := []action{{ID: "vuln.never_declared", Name: "never_declared", Owner: "vuln", ReadOnly: true}}

	result := audit(prog, actions, repoRoot(t))

	if len(result.findings) != 1 {
		t.Fatalf("an unresolvable action produced %d findings, want 1", len(result.findings))
	}
	if !strings.Contains(result.findings[0].message, "no ActionSpec construction resolves") {
		t.Errorf("finding does not say the action could not be resolved:\n%s", result.findings[0].message)
	}
	if result.checked != 0 {
		t.Errorf("checked %d actions, want 0: an unresolved action was not classified", result.checked)
	}
}

// TestAudit_RouteWithNoResolvableHandler_IsReported verifies the quiet failure
// mode is loud: an action whose route the resolver cannot follow to a handler
// has an empty reachable set, so every classification would come back clean
// whatever the handler does. It is reported instead.
func TestAudit_RouteWithNoResolvableHandler_IsReported(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := []action{{ID: "shapes.shared_route", Name: "shared_route", Owner: "shapes", ReadOnly: true}}

	result := audit(prog, actions, repoRoot(t))

	if len(result.findings) != 1 {
		t.Fatalf("an unfollowable route produced %d findings, want 1", len(result.findings))
	}
	if !strings.Contains(result.findings[0].message, "resolves to no handler") {
		t.Errorf("finding does not say the route had no handler:\n%s", result.findings[0].message)
	}
}

// TestAudit_OwnerPackageMismatch_FallsBackToEverySiteWithThatName verifies the
// resolution fallback errs towards reporting: when no site in the owning
// package declares the action, every site with that name is taken, so a
// mismatch cannot be the quiet outcome.
func TestAudit_OwnerPackageMismatch_FallsBackToEverySiteWithThatName(t *testing.T) {
	prog := loadFixture(t, mainSources())
	actions := []action{{ID: "elsewhere.read_dismiss", Name: "read_dismiss", Owner: "elsewhere", ReadOnly: true}}

	result := audit(prog, actions, repoRoot(t))

	if len(result.findings) != 1 {
		t.Fatalf("the fallback produced %d findings, want 1", len(result.findings))
	}
	if !strings.Contains(result.findings[0].message, "GraphQL mutation") {
		t.Errorf("finding is not the mutation report:\n%s", result.findings[0].message)
	}
}

// TestAudit_ExceptionBesideTheAction_SuppressesTheFinding verifies a deliberate
// exception declared in the source next to the action excuses it, and that an
// exception nothing uses is reported so it cannot outlive its reason.
func TestAudit_ExceptionBesideTheAction_SuppressesTheFinding(t *testing.T) {
	prog := loadFixture(t, exceptionSources())

	result := audit(prog, exceptionActions(), repoRoot(t))

	if messageFor(result, "excused.risky_read") != "" {
		t.Error("the excused action was reported despite its directive")
	}
	if result.exceptions != 1 {
		t.Errorf("%d exceptions used, want 1", result.exceptions)
	}
	stale := messageFor(result, "stale.quiet_read")
	if stale == "" {
		t.Fatal("the unused directive was not reported")
	}
	if !strings.Contains(stale, "no longer sends a mutation") {
		t.Errorf("stale finding does not explain itself:\n%s", stale)
	}
}

// TestAudit_ExceptionInAnotherPackage_DoesNotApply verifies an exception only
// excuses the action it was declared beside: the same action name attributed to
// a different owning package is reported, and the now-unused directive is
// reported too.
func TestAudit_ExceptionInAnotherPackage_DoesNotApply(t *testing.T) {
	prog := loadFixture(t, exceptionSources())
	actions := []action{{ID: "stale.risky_read", Name: "risky_read", Owner: "stale", ReadOnly: true}}

	result := audit(prog, actions, repoRoot(t))

	if messageFor(result, "stale.risky_read") == "" {
		t.Error("an action excused only in another package was not reported")
	}
	if result.exceptions != 0 {
		t.Errorf("%d exceptions used, want 0", result.exceptions)
	}
}

// TestParseException_DirectiveForms verifies what counts as a declared
// exception: the directive, an action name, and a reason. A directive without
// a reason is not an exception, because an exception with no stated reason is
// the thing this design is trying to prevent.
func TestParseException_DirectiveForms(t *testing.T) {
	cases := []struct {
		name       string
		comment    string
		wantOK     bool
		wantAction string
		wantReason string
	}{
		{
			name:       "action and reason",
			comment:    "//gitlab:allow-readonly-graphql-mutation epic_note_list: GitLab has no read equivalent.",
			wantOK:     true,
			wantAction: "epic_note_list",
			wantReason: "GitLab has no read equivalent.",
		},
		{
			name:       "indented inside a doc comment",
			comment:    "  //gitlab:allow-readonly-graphql-mutation  spaced_action :  reason with spaces  ",
			wantOK:     true,
			wantAction: "spaced_action",
			wantReason: "reason with spaces",
		},
		{name: "no reason", comment: "//gitlab:allow-readonly-graphql-mutation epic_note_list", wantOK: false},
		{name: "empty reason", comment: "//gitlab:allow-readonly-graphql-mutation epic_note_list:", wantOK: false},
		{name: "no action", comment: "//gitlab:allow-readonly-graphql-mutation : a reason", wantOK: false},
		{name: "ordinary comment", comment: "// this is not a directive", wantOK: false},
		{name: "similar prefix", comment: "//gitlab:allow-something-else foo: bar", wantOK: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := parseException("pkg", testCase.comment, 0)
			if ok != testCase.wantOK {
				t.Fatalf("parseException(%q) ok = %t, want %t", testCase.comment, ok, testCase.wantOK)
			}
			if !ok {
				return
			}
			if got.action != testCase.wantAction {
				t.Errorf("action = %q, want %q", got.action, testCase.wantAction)
			}
			if got.reason != testCase.wantReason {
				t.Errorf("reason = %q, want %q", got.reason, testCase.wantReason)
			}
			if got.pkgName != "pkg" {
				t.Errorf("package = %q, want %q", got.pkgName, "pkg")
			}
		})
	}
}

// TestRelative_PathsOutsideTheRoot_StayAbsolute verifies a position the
// repository root does not contain is printed as it is, rather than as a
// nonsense path full of parent directories.
func TestRelative_PathsOutsideTheRoot_StayAbsolute(t *testing.T) {
	cases := []struct {
		name string
		root string
		file string
		want string
	}{
		{name: "inside the root", root: "/repo", file: "/repo/internal/tools/x.go", want: "internal/tools/x.go:7"},
		{name: "outside the root", root: "/repo", file: "/elsewhere/x.go", want: "/elsewhere/x.go:7"},
		{name: "no root given", root: "", file: "/repo/internal/x.go", want: "/repo/internal/x.go:7"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := relative(token.Position{Filename: testCase.file, Line: 7}, testCase.root)
			if got != testCase.want {
				t.Errorf("relative(%q, %q) = %q, want %q", testCase.file, testCase.root, got, testCase.want)
			}
		})
	}
}

// shapesActions is the catalog for the shapes fixture: every construction
// shape declared read-only, which is honest for all of them but the closure.
func shapesActions() []action {
	names := []string{"direct", "helper", "decorated", "literal", "variable", "constant", "appended", "closure", "quiet"}
	actions := make([]action, 0, len(names))
	for _, name := range names {
		actions = append(actions, action{ID: "shapes." + name, Name: name, Owner: "shapes", ReadOnly: true})
	}
	return actions
}

// equalStrings compares two string slices element by element.
func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	sort.Strings(wantCopy)
	for i := range gotCopy {
		if gotCopy[i] != wantCopy[i] {
			return false
		}
	}
	return true
}
