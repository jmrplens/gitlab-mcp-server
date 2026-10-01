package finegrained_test

import (
	"runtime"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// These tests run the evaluation against the generated table the server
// compiles in, which an internal test of this package cannot import: the
// table's package imports this one. They measure what the design estimated
// and pin what it promised: what one authority holds, what evaluating a grant
// costs, and what deciding a whole listing costs.

// broadGrant is a grant a person might plausibly write: every assignable the
// table defines that a token can be granted, held on one group, plus the
// user's and the instance's standalone permissions. It names every grantable
// permission, which is the most a scope can carry, so the evaluation it
// measures is the largest one a single scope asks for.
func broadGrant(table *finegrained.Table) finegrained.Grant {
	var names []string
	for _, assignable := range table.Assignables {
		if assignable.Grantable && !assignable.Deprecated {
			names = append(names, assignable.Name)
		}
	}
	return finegrained.Grant{Scopes: []finegrained.Scope{
		{Access: finegrained.AccessSelectedMemberships, Namespace: finegrained.NamespaceGroup, NamespaceID: 1, Permissions: names},
		{Access: finegrained.AccessUser, Permissions: names},
		{Access: finegrained.AccessInstance, Permissions: names},
	}}
}

// retainedBytesPerAuthority measures, after a collection, how many heap bytes
// each of n authorities evaluated from grant keeps alive: the grant and the
// transient index of assignable names are garbage once Evaluate returns, so
// what is left is what an authority holds for its pool entry's life.
func retainedBytesPerAuthority(table *finegrained.Table, grant finegrained.Grant, n int) uint64 {
	held := make([]*finegrained.Authority, n)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range held {
		held[i] = finegrained.Evaluate(table, grant)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(held)
	if after.HeapAlloc < before.HeapAlloc {
		return 0
	}
	return (after.HeapAlloc - before.HeapAlloc) / uint64(n) //#nosec G115 -- n is a positive count of authorities this benchmark holds
}

// retainedCeiling is the most one authority may hold: the four sets over the
// raw permissions and the two over the actions are about a kilobyte against
// the generated table, so a field added later that grows per action, which is
// what a per-action list of degraded positions once would have cost (about
// 26 KB an entry), fails here.
const retainedCeiling = 2 << 10

// TestEvaluate_GeneratedTable_AnAuthorityHoldsUnderTwoKiB pins what one
// authority evaluated against the generated table retains.
func TestEvaluate_GeneratedTable_AnAuthorityHoldsUnderTwoKiB(t *testing.T) {
	table := actiongrants.Table()
	if got := retainedBytesPerAuthority(table, broadGrant(table), 256); got > retainedCeiling {
		t.Errorf("an authority retains %d bytes, want at most %d", got, retainedCeiling)
	}
}

// TestEvaluate_GeneratedTable_AGrantOfEverythingListsWhatPhaseAAllows verifies
// that a grant holding every assignable permission at every boundary type
// (all memberships, the user and the instance) is listed exactly the actions
// phase A allows: every requirement the generator joined is one some grant
// can meet, and only the denied ones are out of every grant's reach.
func TestEvaluate_GeneratedTable_AGrantOfEverythingListsWhatPhaseAAllows(t *testing.T) {
	table := actiongrants.Table()
	var names []string
	for _, assignable := range table.Assignables {
		names = append(names, assignable.Name)
	}
	authority := finegrained.Evaluate(table, finegrained.Grant{Scopes: []finegrained.Scope{
		{Access: finegrained.AccessAllMemberships, Permissions: names},
		{Access: finegrained.AccessUser, Permissions: names},
		{Access: finegrained.AccessInstance, Permissions: names},
	}})
	phaseA := finegrained.Unevaluated(table, finegrained.FallbackNone, "")
	for _, row := range table.Actions {
		if got, want := authority.Decide(row.ID).Listed, phaseA.Decide(row.ID).Listed; got != want {
			t.Errorf("%s: listed %v by a grant of everything, %v in phase A", row.ID, got, want)
		}
	}
}

// BenchmarkEvaluate measures evaluating a broad grant against the generated
// table, which an entry build and each accepted revalidation of a
// fine-grained entry pay once, and reports what one authority retains.
func BenchmarkEvaluate(b *testing.B) {
	table := actiongrants.Table()
	grant := broadGrant(table)
	b.ReportAllocs()
	for b.Loop() {
		finegrained.Evaluate(table, grant)
	}
	b.ReportMetric(float64(retainedBytesPerAuthority(table, grant, 256)), "retained-B/authority")
}

// TestAuthority_Lists_IsDecidesListedOnTheGeneratedTable holds the listing's
// shortcut to the decision it stands for, action by action of the generated
// table, in phase A and in phase B.
func TestAuthority_Lists_IsDecidesListedOnTheGeneratedTable(t *testing.T) {
	table := actiongrants.Table()
	for name, authority := range map[string]*finegrained.Authority{
		"phase A": finegrained.Unevaluated(table, finegrained.FallbackNone, ""),
		"phase B": finegrained.Evaluate(table, broadGrant(table)),
	} {
		t.Run(name, func(t *testing.T) {
			for _, row := range table.Actions {
				if got, want := authority.Lists(row.ID), authority.Decide(row.ID).Listed; got != want {
					t.Errorf("%s: Lists %v, Decide.Listed %v", row.ID, got, want)
				}
			}
		})
	}
}

// BenchmarkListFilter measures the listing decision a fine-grained session's
// tools/list asks for, one Lists per action of the generated table, which is
// what the listing filter pays per listing on the individual surface.
func BenchmarkListFilter(b *testing.B) {
	table := actiongrants.Table()
	authority := finegrained.Evaluate(table, broadGrant(table))
	b.ReportAllocs()
	for b.Loop() {
		for i := range table.Actions {
			authority.Lists(table.Actions[i].ID)
		}
	}
}
