package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
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
		"0 operations, 0 groups, 0 GraphQL elements, 2 element signatures read from the pinned schema\n"
	if progress.String() != want {
		t.Errorf("summary = %q, want %q", progress.String(), want)
	}
}
