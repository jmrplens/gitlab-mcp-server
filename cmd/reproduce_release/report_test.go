// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// report_test.go covers what a run prints, in both of its modes, and the
// verdict the exit code is taken from.
package main

import (
	"bytes"
	"testing"
)

// TestReport_Write covers the report line by line: the header, a line per
// binary that agrees, a line per binary that differs with the notes on why
// indented under it, the run's own notes, and the verdict, in each mode's
// words.
func TestReport_Write(t *testing.T) {
	t.Parallel()

	same := outcome{asset: "fixture-linux-amd64", target: "linux/amd64 with go1.27.3", before: "aa", after: "aa"}
	differs := outcome{asset: "fixture-darwin-all", target: "darwin/all", before: "bb", after: "cc", notes: []string{"one", "two"}}
	for _, tc := range []struct {
		name string
		rep  report
		ok   bool
		want string
	}{
		{
			name: "a release reproduced",
			rep:  report{sides: againstRelease, header: "rebuilt v1.2.3", outcomes: []outcome{same}, notes: []string{"not rebuilt: THIRD_PARTY_NOTICES"}},
			ok:   true,
			want: "reproduce_release: rebuilt v1.2.3\n" +
				"reproduced fixture-linux-amd64 (linux/amd64 with go1.27.3) aa\n" +
				"not rebuilt: THIRD_PARTY_NOTICES\n" +
				"1 of 1 binaries rebuilt bit for bit\n",
		},
		{
			name: "a release that differs",
			rep:  report{sides: againstRelease, header: "rebuilt v1.2.3", outcomes: []outcome{same, differs}},
			want: "reproduce_release: rebuilt v1.2.3\n" +
				"reproduced fixture-linux-amd64 (linux/amd64 with go1.27.3) aa\n" +
				"DIFFERS fixture-darwin-all (darwin/all): published bb, rebuilt cc\n" +
				"    one\n" +
				"    two\n" +
				"1 of 2 binaries differ from what the release published\n",
		},
		{
			name: "two builds that agree",
			rep:  report{sides: againstItself, header: "built v1.2.3 twice", outcomes: []outcome{same}},
			ok:   true,
			want: "reproduce_release: built v1.2.3 twice\n" +
				"repeated fixture-linux-amd64 (linux/amd64 with go1.27.3) aa\n" +
				"1 of 1 binaries built twice to the same bytes\n",
		},
		{
			name: "two builds that differ",
			rep:  report{sides: againstItself, header: "built v1.2.3 twice", outcomes: []outcome{differs}},
			want: "reproduce_release: built v1.2.3 twice\n" +
				"DIFFERS fixture-darwin-all (darwin/all): first build bb, second build cc\n" +
				"    one\n" +
				"    two\n" +
				"1 of 1 binaries differ between the two builds\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			tc.rep.write(&out)
			if out.String() != tc.want {
				t.Errorf("write =\n%s\nwant\n%s", out.String(), tc.want)
			}
			if got := tc.rep.ok(); got != tc.ok {
				t.Errorf("ok() = %v, want %v", got, tc.ok)
			}
		})
	}
}

// TestJudge_ComparesTwoCopies covers the comparison every line reports: two
// copies that agree carry no notes, two that differ carry the explanation.
func TestJudge_ComparesTwoCopies(t *testing.T) {
	t.Parallel()

	if got := judge("a", "linux/amd64", []byte("abc"), []byte("abc"), againstRelease); got.before != digestOfABC || got.after != digestOfABC || got.notes != nil {
		t.Errorf("judge of two equal copies = %#v", got)
	}
	got := judge("a", "linux/amd64", []byte("abc"), []byte("abd"), againstRelease)
	if got.before != digestOfABC || got.after == digestOfABC || len(got.notes) != 1 {
		t.Errorf("judge of two copies that differ = %#v", got)
	}
}
