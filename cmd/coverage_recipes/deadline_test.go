// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

// TestReadKnobs_AnEmptyArgument_IsItsDefault verifies that a knob the caller
// leaves out, or gives empty as an unset make variable is, is the recipe's
// own, and that a figure at the floor is not under it.
func TestReadKnobs_AnEmptyArgument_IsItsDefault(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want knobs
	}{
		{"none", nil, knobs{budget: "300", ceiling: "3600", memoryMax: 8}},
		{"empty", []string{"", "", ""}, knobs{budget: "300", ceiling: "3600", memoryMax: 8}},
		{"at the floor", []string{"20", "20", "20"}, knobs{budget: "20", ceiling: "20", memoryMax: 8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, notices, err := readKnobs(tc.args, nil)
			if err != nil || len(notices) != 0 || got != tc.want {
				t.Errorf("readKnobs(%q) = %+v, %q, %v; want %+v and nothing said", tc.args, got, notices, err, tc.want)
			}
		})
	}
}

// TestDeriveDeadline_EachBound_HoldsAtItsEdge verifies the coefficient at the
// edges of its bounds: the budget's whole multiple and one more, never under 8
// and never over 6000, the ceiling's multiple held to where it is the smaller,
// and the ceiling that holds exactly two runs, which is enough.
func TestDeriveDeadline_EachBound_HoldsAtItsEdge(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, base, budget, ceiling string
		want                        string
		says                        string
	}{
		{"a whole multiple", "0.500", "300", "3600", "601", "so -timeout-coefficient 601: about 300.5s per mutant (budget 300s, ceiling 3600s)"},
		{"eight exactly", "1.000", "7", "3600", "8", "about 8.0s per mutant"},
		{"under eight", "1.000", "6", "3600", "8", "about 8.0s per mutant"},
		{"nine", "1.000", "8", "3600", "9", "about 9.0s per mutant"},
		{"six thousand exactly", "1.000", "5999", "100000", "6000", "about 6000.0s per mutant"},
		{"over six thousand", "1.000", "6000", "100000", "6000", "about 6000.0s per mutant"},
		{"the ceiling's multiple", "1.000", "30", "20", "20", "the 20s ceiling holds the coefficient at 20 rather than 31"},
		{"the ceiling equal to it", "1.000", "19", "20", "20", "so -timeout-coefficient 20:"},
		{"the ceiling at eight", "1.000", "30", "8", "8", "the 8s ceiling holds the coefficient at 8 rather than 31"},
		{"the ceiling under eight", "1.000", "30", "7", "7", "the 7s ceiling holds the coefficient at 7, under the floor of 8"},
		{"a ceiling of two runs", "1.000", "30", "2", "2", "the 2s ceiling holds the coefficient at 2, under the floor of 8"},
		{"a ceiling past any integer of 32 bits", "0.333", "30", "1000000000", "91", "so -timeout-coefficient 91:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := deriveDeadline("./p", tc.base, nil, knobs{budget: tc.budget, ceiling: tc.ceiling})
			if err != nil || !got.derived || got.coefficient != tc.want {
				t.Fatalf("deriveDeadline() = %+v, %v; want the coefficient %s", got, err, tc.want)
			}
			if said := strings.Join(got.notices, "\n"); !strings.Contains(said, tc.says) {
				t.Errorf("said %q, want %q", said, tc.says)
			}
		})
	}
	if _, err := deriveDeadline("./p", "1.000", nil, knobs{budget: "30", ceiling: "1.9"}); err == nil || !strings.Contains(err.Error(), "a 1.9s ceiling does not hold two of them") {
		t.Errorf("deriveDeadline() under a ceiling of one run = %v, want a refusal", err)
	}
}
