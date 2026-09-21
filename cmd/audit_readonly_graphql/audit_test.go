package main

import (
	"fmt"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// excusedFixture is a read-only action that really does reach a mutation and
// says so in the source, beside the action, with the directive that excuses it.
const excusedFixture = `package excused

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const writeMutation = @@
mutation($id: ID!) {
  thingTouch(input: {id: $id}) { errors }
}
@@

// undoMutation is the second mutation this handler can reach, through a callee
// rather than in its own body, so the sites a finding would list come from two
// functions of the reachable set and their order has to be settled.
const undoMutation = @@
mutation($id: ID!) {
  thingUntouch(input: {id: $id}) { errors }
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
	if err == nil {
		return Output{OK: true}, nil
	}
	return undo(ctx, client, input)
}

// undo sends the second mutation, from a body of its own.
func undo(ctx context.Context, client *gitlabclient.Client, input Input) (Output, error) {
	var response struct {
		Data map[string]any ` + "`json:\"data\"`" + `
	}
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     undoMutation,
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

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// lineOf returns the 1-based line of the first line of a fixture source that
// contains needle, which is how a test states the line a finding has to point
// at without writing a number that moves whenever the fixture is edited.
func lineOf(t *testing.T, source, needle string) int {
	t.Helper()
	for i, line := range strings.Split(source, "\n") {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	t.Fatalf("no line of the fixture contains %q", needle)
	return 0
}

// TestAudit_FindingNamesTheActionAndTheFile verifies a failure says which
// action, which handler, which document and which line, which is what the
// issue asks a failure to name.
//
// The two location lines are held to the exact text rather than to their
// parts. The site line has to point at the spec element, the send line at the
// statement inside the handler that names the document, and the handler's
// name has to come before the document's: a finding that pointed at the
// constant's declaration, or read "dismissMutation sends Dismiss", still
// contains every word a looser assertion would look for.
func TestAudit_FindingNamesTheActionAndTheFile(t *testing.T) {
	prog := loadFixture(t, mainSources())
	result := audit(prog, vulnActions(), repoRoot(t))

	message := messageFor(result, "vuln.read_dismiss")
	if message == "" {
		t.Fatal("the constructed violation produced no finding")
	}
	file := fixtureDir + "/vuln/vuln.go:"
	want := []string{
		"vuln.read_dismiss is classified ReadOnly but its handler sends a GraphQL mutation.",
		fmt.Sprintf("    action declared at %s%d\n", file, lineOf(t, vulnFixture, `readSpec("read_dismiss",`)),
		fmt.Sprintf("    Dismiss sends dismissMutation at %s%d\n", file, lineOf(t, vulnFixture, "return send(ctx, client, dismissMutation, input)")),
	}
	for _, line := range want {
		t.Run(strings.TrimSpace(line), func(t *testing.T) {
			if !strings.Contains(message, line) {
				t.Errorf("finding does not carry the line %q:\n%s", line, message)
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
	// The whole text, because the ID and the owner are both quoted and both
	// are names: a finding that opened with the package and told the reader to
	// declare the action in a package called "vuln.never_declared" would still
	// contain each of them somewhere.
	want := "vuln.never_declared: no ActionSpec construction resolves to this action, so its handler cannot be classified.\n" +
		"    Declare the action through a toolutil action-spec constructor in package \"vuln\", or the gate cannot vouch for it."
	if result.findings[0].message != want {
		t.Errorf("finding = %q\nwant %q", result.findings[0].message, want)
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

// TestAudit_TwoFindingsForOneAction_AreOrderedByMessage verifies the report is
// ordered all the way down. Findings sort by action, and an action can carry
// two of them at once: here the catalog names an action the source does not
// declare, and the stale directive left in that package is filed under the
// same identifier. Ordering them by message is what keeps the output stable,
// so a CI log diff shows a changed finding rather than a reshuffled pair.
func TestAudit_TwoFindingsForOneAction_AreOrderedByMessage(t *testing.T) {
	prog := loadFixture(t, exceptionSources())
	actions := []action{{ID: "stale.quiet_read", Name: "renamed_away", Owner: "stale", ReadOnly: true}}

	result := audit(prog, actions, repoRoot(t))

	var messages []string
	for _, item := range result.findings {
		if item.action == "stale.quiet_read" {
			messages = append(messages, item.message)
		}
	}
	if len(messages) != 2 {
		t.Fatalf("%d finding(s) for stale.quiet_read, want the unresolved action and its stale directive", len(messages))
	}
	if messages[0] >= messages[1] {
		t.Errorf("findings for one action are not ordered by message:\n%s\n%s", messages[0], messages[1])
	}
}

// TestAudit_DocumentsNothingCanBeHeldTo_AreReported verifies the tripwire is
// part of the report rather than a note somewhere.
//
// A document in a file of its own and one assembled where it is used are both
// read by the shared inventory and placed by nothing this audit walks. Reported
// as findings, they fail the run and name the file; skipped, the run would end
// with the same sentence it prints when every document really was classified.
func TestAudit_DocumentsNothingCanBeHeldTo_AreReported(t *testing.T) {
	prog := &program{unattributed: []graphqldocs.Document{
		{
			Package:  "internal/tools/customemoji",
			Name:     "create.graphql",
			Position: token.Position{Filename: "/repo/internal/tools/customemoji/create.graphql", Line: 1},
			Text:     "mutation { createCustomEmoji { errors } }",
		},
		{
			Package:  "internal/tools/widgets",
			Position: token.Position{Filename: "/repo/internal/tools/widgets/widgets.go", Line: 42},
			Text:     "mutation { widgetDelete { errors } }",
		},
	}}

	findings := unattributedFindings(prog, "/repo")

	if len(findings) != 2 {
		t.Fatalf("unattributedFindings() returned %d finding(s), want one per document", len(findings))
	}
	cases := []struct {
		name  string
		index int
		want  []string
	}{
		{
			name:  "a document in a file of its own",
			index: 0,
			want:  []string{"create.graphql", "internal/tools/customemoji/create.graphql:1", "no handler can be held responsible"},
		},
		{
			name:  "a document assembled where it is used",
			index: 1,
			want:  []string{"an inline document", "internal/tools/widgets/widgets.go:42", "Declare it as a constant"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			message := findings[testCase.index].message
			for _, want := range testCase.want {
				if !strings.Contains(message, want) {
					t.Errorf("finding does not mention %q:\n%s", want, message)
				}
			}
		})
	}
}

// TestAudit_OwnerPackageDecidesAmongSameNamedSites verifies that when two
// packages declare an action of the same name, the one the catalog names as the
// owner is the one answered for.
//
// Both fixtures declare "quiet": the shapes one touches no GraphQL, the other
// one sends a mutation. An owner filter that selected the wrong site, or that
// selected both, would report a mutation against an action whose handler makes
// no request at all, and the fallback for an owner no site matches would hide
// that behind the same answer.
func TestAudit_OwnerPackageDecidesAmongSameNamedSites(t *testing.T) {
	prog := loadFixture(t, mainSources())

	cases := []struct {
		name         string
		act          action
		wantFindings int
	}{
		{
			name: "the owner whose handler is quiet",
			act:  action{ID: "shapes.quiet", Name: "quiet", Owner: "shapes", ReadOnly: true},
		},
		{
			name:         "the owner whose handler writes",
			act:          action{ID: "other.quiet", Name: "quiet", Owner: "other", ReadOnly: true},
			wantFindings: 1,
		},
		{
			name:         "an owner neither of them is",
			act:          action{ID: "elsewhere.quiet", Name: "quiet", Owner: "elsewhere", ReadOnly: true},
			wantFindings: 1,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := audit(prog, []action{testCase.act}, repoRoot(t))

			if len(result.findings) != testCase.wantFindings {
				t.Fatalf("audit() reported %d finding(s), want %d: %v", len(result.findings), testCase.wantFindings, findingActions(result))
			}
			if testCase.wantFindings == 0 {
				return
			}
			if !strings.Contains(result.findings[0].message, "quietMutation") {
				t.Errorf("the finding does not name the mutation the other package sends:\n%s", result.findings[0].message)
			}
		})
	}
}

// TestClassifyReached_MutationSites_AreOrderedBySourcePosition verifies the
// sites a reachable set names come back ordered by where the source names
// them, whatever order they were collected in, and that only the documents
// carrying a mutation are among them.
//
// The reachable set is a map, so a run over a real program hands the sort its
// input in an order no test can choose; a hand-built function whose documents
// are listed back to front is what puts the comparator to both answers on
// every run rather than on the runs the map happens to favor.
func TestClassifyReached_MutationSites_AreOrderedBySourcePosition(t *testing.T) {
	handler := types.NewFunc(token.NoPos, types.NewPackage("example.com/domain", "domain"), "Handle", nil)
	prog := &program{funcs: map[*types.Func]*function{handler: {
		sendsGraphQL: true,
		docs: []docRef{
			{kind: writeDocument, name: "lastMutation", pos: 300},
			{kind: readDocument, name: "middleQuery", pos: 200},
			{kind: writeDocument, name: "firstMutation", pos: 100},
		},
	}}}

	sends, mutations := classifyReached(prog, map[*types.Func]bool{handler: true})

	if !sends {
		t.Error("classifyReached() did not report the transport the function reaches")
	}
	var names []string
	for _, site := range mutations {
		names = append(names, site.doc.name)
	}
	if !equalOrdered(names, []string{"firstMutation", "lastMutation"}) {
		t.Errorf("classifyReached() listed %v, want the two mutations in position order and no query", names)
	}
}

// equalOrdered compares two string slices element by element, in order.
func equalOrdered(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestAudit_MutationSites_AreListedInSourceOrder verifies a finding lists the
// documents a handler reaches in the order the source writes them.
//
// The excused fixture's handler names one mutation in its own body and reaches
// the second through a callee declared below it, so the two sites come out of
// the reachable set's map in whichever order that run's iteration gives; without
// the ordering the two lines would swap between runs and a CI log diff would
// show a reshuffled finding rather than a changed one. The action is attributed
// to a package that declared no exception, so the finding is made rather than
// excused.
func TestAudit_MutationSites_AreListedInSourceOrder(t *testing.T) {
	prog := loadFixture(t, exceptionSources())
	actions := []action{{ID: "stale.risky_read", Name: "risky_read", Owner: "stale", ReadOnly: true}}

	message := messageFor(audit(prog, actions, repoRoot(t)), "stale.risky_read")

	first, second := strings.Index(message, "writeMutation"), strings.Index(message, "undoMutation")
	if first < 0 || second < 0 {
		t.Fatalf("the finding does not name both documents:\n%s", message)
	}
	if first > second {
		t.Errorf("the finding lists undoMutation before writeMutation, which the source writes second:\n%s", message)
	}
}

// TestAudit_Findings_AreOrderedByActionThenMessage verifies the whole report is
// ordered: by the action a finding is about, and by the message within one
// action. The order has to hold whatever order the findings were made in, which
// is why the same documents are put to it twice, reversed the second time: the
// set they come from is a map for the mutation findings and the inventory's own
// order for these, and a report that only happens to come out sorted would pass
// one of the two.
func TestAudit_Findings_AreOrderedByActionThenMessage(t *testing.T) {
	sorted := []graphqldocs.Document{
		{Package: "internal/tools/alpha", Name: "a.graphql", Position: token.Position{Filename: "/repo/a.graphql", Line: 1}},
		{Package: "internal/tools/alpha", Name: "z.graphql", Position: token.Position{Filename: "/repo/z.graphql", Line: 2}},
		{Package: "internal/tools/beta", Name: "b.graphql", Position: token.Position{Filename: "/repo/b.graphql", Line: 3}},
	}
	reversed := []graphqldocs.Document{sorted[2], sorted[1], sorted[0]}
	want := []string{"a.graphql", "z.graphql", "b.graphql"}

	cases := []struct {
		name      string
		inventory []graphqldocs.Document
	}{
		{name: "made in order", inventory: sorted},
		{name: "made in reverse", inventory: reversed},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := audit(&program{unattributed: testCase.inventory}, nil, "/repo")

			if len(result.findings) != len(want) {
				t.Fatalf("audit() reported %d finding(s), want %d", len(result.findings), len(want))
			}
			for i, document := range want {
				if !strings.Contains(result.findings[i].message, document) {
					t.Errorf("finding %d is not the one about %s:\n%s", i, document, result.findings[i].message)
				}
			}
		})
	}
}

// TestIndexLiteral_Roots_AreOrderedBySourcePosition verifies the functions a
// function-literal route stands in for come back in the order the source
// declares them, on every call. They are collected from a map, so leaving them
// in its order would make the roots of one handler differ between runs, and
// the reachable set is where every classification below starts.
//
// The literal is indexed a dozen times because the map's order is the one
// input the test cannot choose: three roots have six orders, only one of
// which is already the declared one, and asking repeatedly is what puts the
// sort in front of the orders that need work rather than the one that does
// not.
func TestIndexLiteral_Roots_AreOrderedBySourcePosition(t *testing.T) {
	prog := loadFixture(t, mainSources())
	var literal handlerRef
	for _, resolved := range (&resolver{prog: prog}).collectSites()["closure"] {
		for _, handler := range resolved.handlers {
			if handler.lit != nil {
				literal = handler
			}
		}
	}
	if literal.lit == nil {
		t.Fatal("the shapes fixture no longer routes an action through a function literal")
	}
	want := []string{"closureBody", "closureCleanup", "closureAudit"}

	for attempt := range 12 {
		var names []string
		for _, root := range prog.indexLiteral(literal.pkg, literal.lit) {
			names = append(names, root.Name())
		}
		if !equalOrdered(names, want) {
			t.Fatalf("attempt %d: indexLiteral() = %v, want %v, the order the fixture declares them", attempt, names, want)
		}
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
//
// The last case is the one the others cannot reach: a path that is not rooted
// where the root is cannot be expressed relative to it at all, so the answer
// comes back as an error rather than as a path full of "..", and a finding
// still has to name a file.
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
		{name: "not relatable to the root", root: "/repo", file: "internal/x.go", want: "internal/x.go:7"},
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
