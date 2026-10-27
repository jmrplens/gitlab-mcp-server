// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package e2erelease

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// recordedTable is the table every case is measured against unless it says
// otherwise: the shape internal/tools/actiongrants records, a released
// Enterprise version and its major.minor.
func recordedTable() *finegrained.Table {
	return &finegrained.Table{Version: "19.4.1-ee", Bucket: "19.4"}
}

// lastMinorTable is a table recorded on the last minor of a major, where the
// release right after it is the next major's first.
func lastMinorTable() *finegrained.Table {
	return &finegrained.Table{Version: "19.11.2-ee", Bucket: "19.11"}
}

// TestDecide_EveryEdge_TakesTheVerdictTheIssueNames walks every edge issue
// 1155 lists: the bucket itself, a patch release and a release candidate in
// it, the prerelease right after it, a later prerelease, the next minor, the
// next major after a major's last minor, an older release, and a version that
// does not parse, which stops the run rather than passing it. A number written
// with a leading zero is one of those: GitLab never writes one, and the
// server's own reading compares the bucket as text, so "19.04" would be the
// recorded minor here and another release there. The ordering cases on both
// sides of each comparison are there so that a comparison written the wrong
// way round fails.
func TestDecide_EveryEdge_TakesTheVerdictTheIssueNames(t *testing.T) {
	cases := []struct {
		name    string
		version string
		table   *finegrained.Table
		want    Verdict
	}{
		{name: "the recorded release itself", version: "19.4.1-ee", table: recordedTable(), want: Recorded},
		{name: "a patch release in the bucket", version: "19.4.3", table: recordedTable(), want: Recorded},
		{name: "the recorded minor's own prerelease", version: "19.4.0-pre", table: recordedTable(), want: Recorded},
		{name: "a release candidate of the recorded minor", version: "19.4.0-rc42-ee", table: recordedTable(), want: Recorded},
		{name: "the prerelease right after", version: "19.5.0-pre", table: recordedTable(), want: ListingOnly},
		{name: "a later prerelease", version: "19.6.0-pre", table: recordedTable(), want: Newer},
		{name: "the next minor", version: "19.5.0-ee", table: recordedTable(), want: Newer},
		{name: "the next minor's release candidate", version: "19.5.0-rc12-ee", table: recordedTable(), want: Newer},
		{name: "the next major", version: "20.0.0", table: recordedTable(), want: Newer},
		{name: "a later major with a lower minor", version: "20.1.0-ee", table: recordedTable(), want: Newer},
		{name: "the next major's prerelease after the last minor", version: "20.0.0-pre", table: lastMinorTable(), want: ListingOnly},
		{name: "the next major after the last minor", version: "20.0.0-ee", table: lastMinorTable(), want: Newer},
		{name: "a minor past the last one", version: "19.12.0-pre", table: lastMinorTable(), want: Newer},
		{name: "the previous minor", version: "19.3.2-ee", table: recordedTable(), want: Older},
		{name: "the previous minor's prerelease", version: "19.3.0-pre", table: recordedTable(), want: Older},
		{name: "an older major with a higher minor", version: "18.11.5", table: recordedTable(), want: Older},
		{name: "no version", version: "", table: recordedTable(), want: Unreadable},
		{name: "the script's unknown", version: "unknown", table: recordedTable(), want: Unreadable},
		{name: "a major alone", version: "19", table: recordedTable(), want: Unreadable},
		{name: "a leading v", version: "v19.4.1", table: recordedTable(), want: Unreadable},
		{name: "a minor that is no number", version: "19.x.1", table: recordedTable(), want: Unreadable},
		{name: "a major no int holds", version: "99999999999999999999.4.1", table: recordedTable(), want: Unreadable},
		{name: "a minor no int holds", version: "19.99999999999999999999.1", table: recordedTable(), want: Unreadable},
		{name: "a minor with a leading zero", version: "19.04.1", table: recordedTable(), want: Unreadable},
		{name: "a major with a leading zero", version: "019.4.1", table: recordedTable(), want: Unreadable},
		{name: "a prerelease minor with a leading zero", version: "19.05.0-pre", table: recordedTable(), want: Unreadable},
		{name: "a minor that is zero", version: "20.0.3-ee", table: &finegrained.Table{Version: "20.0.1-ee", Bucket: "20.0"}, want: Recorded},
		{name: "a table bucket that is empty", version: "19.4.1-ee", table: &finegrained.Table{Version: "19.4.1-ee"}, want: Unreadable},
		{name: "a table bucket that is no major.minor", version: "19.4.1-ee", table: &finegrained.Table{Version: "19.4.1-ee", Bucket: "x.4"}, want: Unreadable},
		{name: "a table bucket with a leading zero", version: "19.4.1-ee", table: &finegrained.Table{Version: "19.04.1-ee", Bucket: "19.04"}, want: Unreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.version, tc.table)
			if got.Verdict != tc.want {
				t.Errorf("Decide(%q) against %s = %s, want %s", tc.version, tc.table.Bucket, got.Verdict, tc.want)
			}
			if got.Version != tc.version || got.Table != tc.table.Version || got.Bucket != tc.table.Bucket {
				t.Errorf("Decide(%q) carried version %q, table %q and bucket %q, want what it was handed: %q, %q, %q",
					tc.version, got.Version, got.Table, got.Bucket, tc.version, tc.table.Version, tc.table.Bucket)
			}
		})
	}
}

// TestDecide_AgreesWithTheServersJudgement holds the second opinion to the
// first on every edge a server judges: the recorded bucket is phase B, the
// prerelease right after it is phase B for the listing alone, every other
// release, older or newer, is phase A for want of a recorded release, and a
// version the rule cannot read is one the server judges no grant on either, a
// number written with a leading zero among them. The two are written apart on
// purpose; this is what says when one of them moved.
func TestDecide_AgreesWithTheServersJudgement(t *testing.T) {
	table := recordedTable()
	cases := []struct {
		name    string
		version string
	}{
		{name: "the recorded release", version: "19.4.1-ee"},
		{name: "a patch release", version: "19.4.7"},
		{name: "the prerelease right after", version: "19.5.0-pre"},
		{name: "a later prerelease", version: "19.6.0-pre"},
		{name: "the next minor", version: "19.5.0-ee"},
		{name: "an older release", version: "19.3.2-ee"},
		{name: "an older major", version: "18.11.0"},
		{name: "a minor with a leading zero", version: "19.04.1"},
		{name: "a major with a leading zero", version: "019.4.1"},
		{name: "a prerelease minor with a leading zero", version: "19.05.0-pre"},
		{name: "the script's unknown", version: "unknown"},
		{name: "no version", version: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authority := finegrained.Judge(table, finegrained.Reading{Grant: finegrained.Grant{Scopes: []finegrained.Scope{}}, Version: tc.version})
			verdict := Decide(tc.version, table).Verdict
			granted := authority.Phase() == finegrained.PhaseGranted
			switch verdict {
			case Recorded:
				if !granted || authority.ListingOnly() {
					t.Errorf("%s is the recorded release here and the server judged phase %d, listing only %t", tc.version, authority.Phase(), authority.ListingOnly())
				}
			case ListingOnly:
				if !granted || !authority.ListingOnly() {
					t.Errorf("%s is the listing-only prerelease here and the server judged phase %d, listing only %t", tc.version, authority.Phase(), authority.ListingOnly())
				}
			case Older, Newer:
				if granted || authority.Fallback() != finegrained.FallbackVersionOutside {
					t.Errorf("%s is %s here and the server judged phase %d with fallback %q", tc.version, verdict, authority.Phase(), authority.Fallback())
				}
			case Unreadable:
				if granted {
					t.Errorf("%q is unreadable here, which stops the run, and the server judged a grant on it (phase %d, listing only %t)",
						tc.version, authority.Phase(), authority.ListingOnly())
				}
			default:
				t.Errorf("%s is %s here, which none of these cases should be", tc.version, verdict)
			}
		})
	}
}

// TestDecisionRuns_OnlyTheStoppingVerdictsStop checks which verdicts let a run
// go ahead: the two judged states and an older release run, a newer release
// and an unreadable version stop, and so does a verdict nobody defined, since
// a run that cannot say why it may go ahead should not.
func TestDecisionRuns_OnlyTheStoppingVerdictsStop(t *testing.T) {
	cases := []struct {
		verdict Verdict
		want    bool
	}{
		{verdict: Recorded, want: true},
		{verdict: ListingOnly, want: true},
		{verdict: Older, want: true},
		{verdict: Newer, want: false},
		{verdict: Unreadable, want: false},
		{verdict: Verdict(99), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.verdict.String(), func(t *testing.T) {
			if got := (Decision{Verdict: tc.verdict}).Runs(); got != tc.want {
				t.Errorf("Runs() for %s = %t, want %t", tc.verdict, got, tc.want)
			}
		})
	}
}

// TestVerdictString_EveryVerdict_NamesItself checks that a verdict prints as a
// word a log line can carry, and that one nobody defined says so with its
// number rather than passing for one that exists.
func TestVerdictString_EveryVerdict_NamesItself(t *testing.T) {
	cases := map[Verdict]string{
		Unreadable:  "unreadable",
		Recorded:    "recorded",
		ListingOnly: "listing-only",
		Older:       "older",
		Newer:       "newer",
		Verdict(99): "verdict(99)",
	}
	for verdict, want := range cases {
		t.Run(want, func(t *testing.T) {
			if got := verdict.String(); got != want {
				t.Errorf("String() = %q, want %q", got, want)
			}
		})
	}
}

// TestDecisionText_EachVerdict_SaysWhatTheRunDoes holds the words of every
// verdict, since the text is the whole value of a stop: a refusal that named
// neither release nor what to do would be no better than the silent phase A
// it replaces. The two running states and an older release are one line each;
// a stop names both releases and, for a newer release, the three steps and
// the pin that runs the table's own release meanwhile.
func TestDecisionText_EachVerdict_SaysWhatTheRunDoes(t *testing.T) {
	cases := []struct {
		name      string
		decision  Decision
		oneLine   bool
		mentions  []string
		forbidden []string
	}{
		{
			name:     "the recorded release",
			decision: Decide("19.4.1-ee", recordedTable()),
			oneLine:  true,
			mentions: []string{"GitLab 19.4.1-ee", "19.4.1-ee", "the listing and the calls"},
		},
		{
			name:     "the prerelease right after",
			decision: Decide("19.5.0-pre", recordedTable()),
			oneLine:  true,
			mentions: []string{"GitLab 19.5.0-pre", "19.4.1-ee", "the listing alone"},
		},
		{
			name:     "an older release",
			decision: Decide("19.3.2-ee", recordedTable()),
			oneLine:  true,
			mentions: []string{"GitLab 19.3.2-ee", "older", "19.4.1-ee", "phase A"},
		},
		{
			name:     "a newer release",
			decision: Decide("19.5.0-ee", recordedTable()),
			mentions: []string{
				"GitLab 19.5.0-ee", "newer", "19.4.1-ee", "19.5.0-pre", "phase A",
				"1. make gen-api-live", "-image", "docs/development/gitlab-api-live.json",
				"2. make gen-action-grants", "internal/tools/actiongrants/table_gen.go",
				"docs/development/action-requests.json", "site/src/content/docs/reference/fine-grained-permissions.mdx",
				"3. commit", "start the run again",
				"GITLAB_IMAGE=gitlab/gitlab-ce:19.4.1-ce.0", "gitlab/gitlab-ee:19.4.1-ee.0",
			},
		},
		{
			name:     "a newer release after a major's last minor",
			decision: Decide("20.0.0-ee", lastMinorTable()),
			mentions: []string{"GitLab 20.0.0-ee", "19.11.2-ee", "20.0.0-pre", "GITLAB_IMAGE=gitlab/gitlab-ce:19.11.2-ce.0"},
		},
		{
			name:      "an instance version that does not parse",
			decision:  Decide("unknown", recordedTable()),
			mentions:  []string{`"unknown"`, "major.minor as GitLab writes one", "19.4.1-ee", "stops", "/api/v4/version"},
			forbidden: []string{"gen-action-grants"},
		},
		{
			name:      "an instance version with a leading zero",
			decision:  Decide("19.04.1", recordedTable()),
			mentions:  []string{`"19.04.1"`, "major.minor as GitLab writes one", "19.4.1-ee", "stops"},
			forbidden: []string{"gen-action-grants"},
		},
		{
			name:     "a table bucket that does not parse",
			decision: Decide("19.4.1-ee", &finegrained.Table{Version: "19.4.1-ee", Bucket: "x.4"}),
			mentions: []string{`"x.4"`, "major.minor as GitLab writes one", "make gen-action-grants", "stops"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.decision.Text()
			if text == "" {
				t.Fatalf("%s says nothing", tc.decision.Verdict)
			}
			if tc.oneLine && strings.Contains(text, "\n") {
				t.Errorf("%s should be one line, and is:\n%s", tc.decision.Verdict, text)
			}
			for _, want := range tc.mentions {
				if !strings.Contains(text, want) {
					t.Errorf("the text does not mention %q:\n%s", want, text)
				}
			}
			for _, unwanted := range tc.forbidden {
				if strings.Contains(text, unwanted) {
					t.Errorf("the text mentions %q, which is not this verdict's remedy:\n%s", unwanted, text)
				}
			}
		})
	}
}

// TestDecisionText_UndefinedVerdict_SaysItIsNotOne checks that a verdict
// nobody defined is described as such rather than with another verdict's
// words, since Runs refuses it and the refusal has to say something.
func TestDecisionText_UndefinedVerdict_SaysItIsNotOne(t *testing.T) {
	text := Decision{Verdict: Verdict(99), Version: "19.4.1-ee", Table: "19.4.1-ee", Bucket: "19.4"}.Text()
	if !strings.Contains(text, "verdict(99)") || !strings.Contains(text, "19.4.1-ee") {
		t.Errorf("the text of an undefined verdict is %q, want one naming the verdict and the release", text)
	}
}

// TestNextPrerelease_SpellsTheOneReleaseTheListingFollows checks the
// prerelease spelled after a bucket: the next minor of the same major, the
// next major's first after a major's last minor (and after anything past it),
// and nothing for a bucket that is no major.minor as GitLab writes one.
func TestNextPrerelease_SpellsTheOneReleaseTheListingFollows(t *testing.T) {
	cases := []struct {
		bucket, want string
	}{
		{bucket: "19.4", want: "19.5.0-pre"},
		{bucket: "20.0", want: "20.1.0-pre"},
		{bucket: "19.04", want: ""},
		{bucket: "019.4", want: ""},
		{bucket: "19.10", want: "19.11.0-pre"},
		{bucket: "19.11", want: "20.0.0-pre"},
		{bucket: "19.12", want: "20.0.0-pre"},
		{bucket: "19", want: ""},
		{bucket: "x.4", want: ""},
		{bucket: "19.x", want: ""},
		{bucket: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.bucket, func(t *testing.T) {
			if got := NextPrerelease(tc.bucket); got != tc.want {
				t.Errorf("NextPrerelease(%q) = %q, want %q", tc.bucket, got, tc.want)
			}
		})
	}
}

// TestPrereleaseAfter_NamesTheOneReleaseTheListingFollows walks the rule the
// listing-only state is held to: the next minor's prerelease, the next major's
// first after a major's last minor, and nothing else, a release of that minor,
// a later minor and a bucket that is no major.minor included.
func TestPrereleaseAfter_NamesTheOneReleaseTheListingFollows(t *testing.T) {
	cases := []struct {
		name, version, bucket string
		want                  bool
	}{
		{name: "the next minor's prerelease", version: "19.5.0-pre", bucket: "19.4", want: true},
		{name: "the next major's after the last minor", version: "20.0.0-pre", bucket: "19.11", want: true},
		{name: "the next minor's release", version: "19.5.0", bucket: "19.4"},
		{name: "the recorded minor's prerelease", version: "19.4.0-pre", bucket: "19.4"},
		{name: "a later minor's prerelease", version: "19.6.0-pre", bucket: "19.4"},
		{name: "the same major after the last minor", version: "19.12.0-pre", bucket: "19.11"},
		{name: "a bucket with no minor", version: "19.5.0-pre", bucket: "19"},
		{name: "a bucket with no major", version: "19.5.0-pre", bucket: "x.4"},
		{name: "a bucket with a leading zero", version: "19.5.0-pre", bucket: "19.04"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrereleaseAfter(tc.version, tc.bucket); got != tc.want {
				t.Errorf("PrereleaseAfter(%q, %q) = %t, want %t", tc.version, tc.bucket, got, tc.want)
			}
		})
	}
}
