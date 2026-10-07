package main

import (
	"bytes"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
)

// use is a derived REST request.
func use(path string, qualified, declaredOptional bool) derive.Use {
	return derive.Use{
		Kind: derive.KindREST, Method: http.MethodGet, Path: path, Sites: []string{"pkg.F"},
		Qualified: qualified, DeclaredOptional: declaredOptional,
	}
}

// TestGateFindings_HoldsEachRuleToItsShape verifies what each gate passes: two
// requests a directive qualifies, an action declared whole, a request an
// optional directive says may not run, a denial on an element the record
// holds, and a row the join could not place; and that the same finding is
// reported once however many paths repeat it.
func TestGateFindings_HoldsEachRuleToItsShape(t *testing.T) {
	derived := []derive.Action{
		{ID: "a.qualified", Uses: []derive.Use{use("/a", true, false), use("/b", true, false)}, Paths: [][]int{{0, 1}}},
		{ID: "a.declared", Uses: []derive.Use{use("/a", false, false), use("/b", false, false)}, Paths: [][]int{{0, 1}, {}}, Declaration: "sends-nothing"},
		{ID: "a.maybe", Uses: []derive.Use{use("/a", false, true)}, Paths: [][]int{{}}},
		{ID: "a.twice", Uses: []derive.Use{use("/a", false, false), use("/b", false, false), use("/c", true, false)}, Paths: [][]int{{0, 1}, {0, 1, 2}}},
	}
	joined := []join.Action{
		{ID: "a.unplaced"},
		{ID: "a.known", Row: &finegrained.Requirement{ID: "a.known", Denied: &finegrained.Denial{
			Cause: finegrained.CauseRESTUndeclared, Element: "GET /projects/:id",
		}}},
		{ID: "a.reachable", Row: &finegrained.Requirement{ID: "a.reachable"}},
	}
	// a.twice's two paths both carry the same two bare requests, and the
	// finding is one finding.
	got := gateFindings(derived, joined, mainRecord())
	want := "gate 1: a.twice sends GET /a (from pkg.F) and GET /b (from pkg.F) on one path and no directive or declaration says why each runs"
	if len(got) != 1 || got[0] != want {
		t.Errorf("gateFindings = %q, want %q once", got, want)
	}
}

// TestGateFindings_HoldsEveryDenialOfARowToTheRecord verifies gate 2 asks of
// every denial a row carries, the action's and each denied way's, whether the
// record holds its element as the kind its cause says: a row denied on a type
// the record lacks and a row with one held and one unheld denied way each
// report the unheld denial once, and a row with no requirement reports
// nothing.
func TestGateFindings_HoldsEveryDenialOfARowToTheRecord(t *testing.T) {
	joined := []join.Action{
		{ID: "a.unplaced"},
		{ID: "a.denied", Row: &finegrained.Requirement{ID: "a.denied", Denied: &finegrained.Denial{
			Cause: finegrained.CauseTypeUndeclared, Element: "Namespace",
		}}},
		{ID: "a.ways", Row: &finegrained.Requirement{ID: "a.ways", DeniedWays: []finegrained.Denial{
			{Cause: finegrained.CauseRESTUndeclared, Element: "DELETE /projects/:id"},
			{Cause: finegrained.CauseMutationUndeclared, Element: "thingCreate"},
		}}},
	}
	got := gateFindings(nil, joined, mainRecord())
	want := []string{
		"gate 2: a way of running a.ways is denied by thingCreate, which the live record does not hold as a graphql-mutation-undeclared",
		"gate 2: a.denied is denied by Namespace, which the live record does not hold as a graphql-type-undeclared",
	}
	if !slices.Equal(got, want) {
		t.Errorf("gateFindings = %q, want %q", got, want)
	}
}

// TestSummarize_CountsWhatAReaderChecksARunBy verifies the summary line counts
// the rows denied and the rows served with parts always empty apart.
func TestSummarize_CountsWhatAReaderChecksARunBy(t *testing.T) {
	result := outcome{
		derived: derive.Result{Actions: make([]derive.Action, 3)},
		joined: join.Result{Fallbacks: 2, Table: finegrained.Table{
			Version: "19.4.1-ee",
			Actions: []finegrained.Requirement{
				{ID: "a", Denied: &finegrained.Denial{}},
				{ID: "b", Degraded: []uint32{0}},
				{ID: "c"},
			},
		}},
	}
	var progress bytes.Buffer
	summarize(&progress, &result)
	want := "3 actions derived, 3 rows at GitLab 19.4.1-ee: 1 denied to every fine-grained token, 1 served with parts always empty; " +
		"0 operations, 0 groups, 0 GraphQL elements, 2 element signatures read from the pinned schema\n" +
		"classic scope: 0 api, 0 read_api, 0 other-credential, 0 no-request; read_api reaches 0 actions, " +
		"and these depart from their read-only classification: none\n"
	if progress.String() != want {
		t.Errorf("summary = %q, want %q", progress.String(), want)
	}
}

// TestSummarize_CountsEachClassicScopeAndTheDepartures verifies the classic
// line counts the rows per scope, the ones read_api reaches being every row
// but those needing api or nothing known, and names each action whose
// classification departs from that reach, served or withheld.
func TestSummarize_CountsEachClassicScopeAndTheDepartures(t *testing.T) {
	result := outcome{
		catalog: []actionrequests.Action{{ID: "read", ReadOnly: true}, {ID: "lint", ReadOnly: true}, {ID: "trigger"}, {ID: "write"}},
		joined: join.Result{Table: finegrained.Table{Actions: []finegrained.Requirement{
			{ID: "lint", Classic: finegrained.ClassicAPI},
			{ID: "nothing", Classic: finegrained.ClassicNoRequest},
			{ID: "read", Classic: finegrained.ClassicReadAPI},
			{ID: "trigger", Classic: finegrained.ClassicOtherCredential},
			{ID: "unknown"},
			{ID: "write", Classic: finegrained.ClassicAPI},
		}}},
	}
	var progress bytes.Buffer
	summarize(&progress, &result)
	_, line, _ := strings.Cut(progress.String(), "\n")
	want := "classic scope: 2 api, 1 read_api, 1 other-credential, 1 no-request; read_api reaches 3 actions, " +
		"and these depart from their read-only classification: lint withheld, nothing served, trigger served\n"
	if line != want {
		t.Errorf("classic line = %q, want %q", line, want)
	}
}

// classicJoined is a joined action whose requests need the given scopes, one
// way per scope, its row needing the least of them.
func classicJoined(id string, scopes ...finegrained.ClassicScope) join.Action {
	act := join.Action{ID: id, Row: &finegrained.Requirement{ID: id, Classic: finegrained.ClassicAPI}}
	for i, scope := range scopes {
		act.Requests = append(act.Requests, join.Request{
			Kind: derive.KindREST, Method: http.MethodGet, Path: "/a", Class: derive.ClassAlternative,
			Classic: scope,
		})
		act.Paths = append(act.Paths, []int{i})
		act.Row.Classic = min(act.Row.Classic, scope)
	}
	return act
}

// TestClassicFindings_HoldsGatesFourAndFive verifies gate 4 refuses an action
// whose ways need different scopes, and one that may send an optional request
// needing more than it, and passes one whose ways agree and ones whose
// optional request needs less or as much; gate 5 refuses a read
// a read_api token does not reach and a write it does, and passes a read it
// reaches, a write it does not and an action with no row; a declaration
// answers its action's finding, and one that answers nothing is reported.
func TestClassicFindings_HoldsGatesFourAndFive(t *testing.T) {
	optional := classicJoined("a.optional", finegrained.ClassicReadAPI)
	optional.Requests = append(optional.Requests, join.Request{
		Kind: derive.KindREST, Method: http.MethodPost, Path: "/b", Class: derive.ClassOptional,
		Classic: finegrained.ClassicAPI,
	})
	cheapOptional := classicJoined("a.cheap_optional", finegrained.ClassicAPI)
	cheapOptional.Requests = append(cheapOptional.Requests, join.Request{
		Kind: derive.KindREST, Method: http.MethodGet, Path: "/c", Class: derive.ClassOptional,
		Classic: finegrained.ClassicReadAPI,
	})
	equalOptional := classicJoined("a.equal_optional", finegrained.ClassicReadAPI)
	equalOptional.Requests = append(equalOptional.Requests, join.Request{
		Kind: derive.KindREST, Method: http.MethodGet, Path: "/d", Class: derive.ClassOptional,
		Classic: finegrained.ClassicReadAPI,
	})
	joined := []join.Action{
		classicJoined("a.varies", finegrained.ClassicReadAPI, finegrained.ClassicAPI, finegrained.ClassicReadAPI),
		classicJoined("a.declared_variation", finegrained.ClassicReadAPI, finegrained.ClassicAPI),
		classicJoined("a.agrees", finegrained.ClassicAPI, finegrained.ClassicAPI),
		optional,
		cheapOptional,
		equalOptional,
		classicJoined("a.lint", finegrained.ClassicAPI),
		classicJoined("a.trigger", finegrained.ClassicOtherCredential),
		classicJoined("a.declared_departure", finegrained.ClassicAPI),
		classicJoined("a.read", finegrained.ClassicReadAPI),
		{ID: "a.unplaced"},
	}
	catalog := []actionrequests.Action{
		{ID: "a.varies", ReadOnly: true},
		{ID: "a.declared_variation", ReadOnly: true},
		{ID: "a.optional", ReadOnly: true},
		{ID: "a.equal_optional", ReadOnly: true},
		{ID: "a.lint", ReadOnly: true},
		{ID: "a.declared_departure", ReadOnly: true},
		{ID: "a.read", ReadOnly: true},
		{ID: "a.unplaced", ReadOnly: true},
	}
	variations := []classicDeclaration{{Action: "a.declared_variation"}, {Action: "a.agrees"}}
	disagreements := []classicDeclaration{{Action: "a.declared_departure"}, {Action: "a.read"}}
	got := classicFindings(joined, catalog, variations, disagreements)
	want := []string{
		"gate 4: a.optional may send POST /b, which needs api, more than the read_api the action needs",
		"gate 4: a.varies runs ways needing read_api and api; a read_api token would be served or withheld it whole",
		"gate 4: the declaration of a.agrees answers nothing; remove it",
		"gate 5: a.lint is classified read-only=true and a read_api token reaching it is false; declare why the two depart",
		"gate 5: a.trigger is classified read-only=false and a read_api token reaching it is true; declare why the two depart",
		"gate 5: the declaration of a.read answers nothing; remove it",
	}
	if !slices.Equal(got, want) {
		t.Errorf("classicFindings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
