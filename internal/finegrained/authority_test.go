package finegrained

import (
	"slices"
	"testing"
)

func TestUnevaluated_CarriesWhatItWasBuiltWith(t *testing.T) {
	table := testTable()
	authority := Unevaluated(table, FallbackVersionOutside, "19.5.0-pre")
	if authority.Table() != table || authority.Phase() != PhaseUnknown ||
		authority.Fallback() != FallbackVersionOutside || authority.Reported() != "19.5.0-pre" {
		t.Errorf("Unevaluated returned %+v", authority)
	}
}

func TestAuthority_Decide_PhaseAWithholdsOnlyTheDenied(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	cases := []struct {
		name string
		id   string
		want Decision
	}{
		{
			name: "denied by an undeclared type",
			id:   "epic.list",
			want: Decision{Cause: CauseTypeUndeclared, Known: true},
		},
		{
			name: "allowed",
			id:   "merge_request.approve",
			want: Decision{Listed: true, Callable: true, Known: true},
		},
		{
			name: "allowed with the positions every token gets empty",
			id:   "vulnerability.get",
			want: Decision{Listed: true, Callable: true, Degraded: []uint32{1}, Known: true},
		},
		{
			name: "no row is unknown authority, allowed",
			id:   "issue.list",
			want: Decision{Listed: true, Callable: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authority.Decide(tc.id)
			if got.Listed != tc.want.Listed || got.Callable != tc.want.Callable || got.Cause != tc.want.Cause ||
				got.Known != tc.want.Known || !slices.Equal(got.Degraded, tc.want.Degraded) || len(got.Missing) != 0 {
				t.Errorf("Decide(%q) = %+v, want %+v", tc.id, got, tc.want)
			}
		})
	}
}
