package main

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// TestCheckRuntime_Floors_Applied verifies the five floors: a runtime with no
// test call, a package that refused, a package that ran under a -run filter,
// an asserted count below its floor, and a runtime that passes all of them.
func TestCheckRuntime_Floors_Applied(t *testing.T) {
	t.Cleanup(func() { assertedFloors = map[string]int{} })
	assertedFloors = map[string]int{"ce": 5, "community/free": 3}

	cases := []struct {
		name string
		rep  *report
		want []string
	}{
		{
			name: "no test call",
			rep:  &report{Runtime: "community/free", Summary: summary{TestCalls: 0, L1: 9}},
			want: []string{"no test call was recorded on community/free"},
		},
		{
			name: "a package refused",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1, L1: 9}, Runs: []runRow{
				{Package: "ee", Status: e2ecalls.RunRefused, Reason: "needs a license"},
			}},
			want: []string{"package ee refused to run: needs a license"},
		},
		{
			// The filtered package clears every floor on its own, which is
			// exactly the run the check must not mistake for the suite.
			name: "a package ran under a filter",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1, L1: 9}, Runs: []runRow{
				{Package: "common", Status: e2ecalls.RunStarted, Filter: "^TestIssue"},
				{Package: "ce", Status: e2ecalls.RunStarted},
			}},
			want: []string{`package common ran under the filter "^TestIssue": a partial run is not a coverage claim`},
		},
		{
			// A refused run is one finding, whatever filter it was given.
			name: "a refused package with a filter is reported once",
			rep: &report{Runtime: "community/free", Summary: summary{TestCalls: 1, L1: 9}, Runs: []runRow{
				{Package: "ee", Status: e2ecalls.RunRefused, Reason: "needs a license", Filter: "^TestEpic"},
			}},
			want: []string{"package ee refused to run: needs a license"},
		},
		{
			name: "below the floors, both selectors named",
			rep:  &report{Runtime: "community/free", Summary: summary{TestCalls: 1, L1: 2}},
			want: []string{
				"2 actions asserted on community/free, below the floor of 3 recorded for community/free",
				"2 actions asserted on community/free, below the floor of 5 recorded for ce",
			},
		},
		{
			name: "passes",
			rep:  &report{Runtime: "community/free", Summary: summary{TestCalls: 1, L1: 5}},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := checkRuntime(tc.rep, []string{"ce"})
			if !reflect.DeepEqual(result.Findings, tc.want) || result.Passed != (len(tc.want) == 0) {
				t.Errorf("checkRuntime() = passed %t, findings %q; want findings %q", result.Passed, result.Findings, tc.want)
			}
		})
	}
}

// TestMissingRuntimes_UnmatchedSelectors_Named verifies the check that an
// expected runtime left no run line.
func TestMissingRuntimes_UnmatchedSelectors_Named(t *testing.T) {
	runtimes := []*runtimeRecords{{key: "community/free"}}
	got := missingRuntimes(runtimes, []string{"ce", "ee", "enterprise/free"})
	if want := []string{"ee", "enterprise/free"}; !reflect.DeepEqual(got, want) {
		t.Errorf("missingRuntimes() = %q, want %q", got, want)
	}
}

// TestFloorSelectors_MatchingSelectors_KeyFirst verifies that a floor is read
// under the runtime's own key and under every selector it matches, once
// each.
func TestFloorSelectors_MatchingSelectors_KeyFirst(t *testing.T) {
	got := floorSelectors("enterprise/ultimate", []string{"ce", "ee", "enterprise/ultimate"})
	if want := []string{"enterprise/ultimate", "ee"}; !reflect.DeepEqual(got, want) {
		t.Errorf("floorSelectors() = %q, want %q", got, want)
	}
}
