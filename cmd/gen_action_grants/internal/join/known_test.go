package join

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestKnown_HoldsADenialToWhatTheRecordDeclares verifies gate 2: a denial is
// known only when the element it names is one the record holds, of the kind
// its cause names. A route for a REST cause, a mutation for an undeclared
// mutation, an object, union or interface type for a position, and a type or
// a mutation for a boundary a declaration says never resolves.
func TestKnown_HoldsADenialToWhatTheRecordDeclares(t *testing.T) {
	record := fixtureRecord()
	cases := []struct {
		name   string
		denial finegrained.Denial
		want   bool
	}{
		{name: "a recorded route", denial: finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /projects/:id/nothing"}, want: true},
		{name: "a route the record lacks", denial: finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "GET /nowhere"}},
		{name: "a recorded mutation", denial: finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: "issueCreate"}, want: true},
		{name: "a mutation the record lacks", denial: finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: "undeclaredThing"}},
		{name: "an object type", denial: finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "BranchRule"}, want: true},
		{name: "a union", denial: finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "VulnerabilityDetail"}, want: true},
		{name: "a type the record lacks", denial: finegrained.Denial{Cause: finegrained.CausePayloadUndeclared, Element: "Nowhere"}},
		{name: "a mutation for a type cause", denial: finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "issueCreate"}},
		{name: "an unresolvable type", denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "WorkItem"}, want: true},
		{name: "an unresolvable mutation", denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "workItemUpdate"}, want: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Known(record, &testCase.denial); got != testCase.want {
				t.Errorf("Known(%+v) = %t, want %t", testCase.denial, got, testCase.want)
			}
		})
	}
	t.Run("no GraphQL record", func(t *testing.T) {
		bare := fixtureRecord()
		bare.GraphQLAuthz = nil
		if Known(bare, &finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "BranchRule"}) {
			t.Error("a GraphQL denial is known with no GraphQL record")
		}
	})
}
