package tenancy

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// TestDecision_Disagrees_IsExactlyTheClassDSet holds the two declared facts a
// row carries, the unit of what its reason cites and its key, to reproducing
// the specification's class D set exactly: the tool-call bucket on HTTP, the
// catalog listing bucket, the watcher pause on a GitLab 429 and the watcher
// cap. HLD-001 is class R, with HLD-002 as its partner: its stated unit is the
// entry's own, and its word "fairness" is a vocabulary departure the gate
// holds, not a unit mismatch.
func TestDecision_Disagrees_IsExactlyTheClassDSet(t *testing.T) {
	var disagree []string
	for _, d := range Decisions() {
		if d.Disagrees() {
			disagree = append(disagree, d.ID)
		}
	}
	slices.Sort(disagree)
	if got, want := strings.Join(disagree, ","), "HLD-003,RTC-001,RTC-003,RTC-005"; got != want {
		t.Errorf("rows that disagree = %s, want %s", got, want)
	}
	hld001, _ := Lookup("HLD-001")
	if hld001.Class != ClassR || hld001.Partner != "HLD-002" || hld001.Disagrees() {
		t.Errorf("HLD-001: class %d, partner %q, disagrees %v; want class R, partner HLD-002, agreeing",
			hld001.Class, hld001.Partner, hld001.Disagrees())
	}
}

// TestDecision_Disagrees_OnlyAnAllowanceOnARequesterCan walks each condition
// of the rule: a rule, a key before admission, a reason with no unit, a reason
// in the key's own unit and a process reason met by a partner all agree.
func TestDecision_Disagrees_OnlyAnAllowanceOnARequesterCan(t *testing.T) {
	base := Decision{Kind: Ceiling, Key: KeyEntry, ReasonUnit: KeyTenant}
	for _, tc := range []struct {
		name   string
		change func(d *Decision)
		want   bool
	}{
		{"a ceiling on the entry for a per-user reason", func(*Decision) {}, true},
		{"a rule never disagrees", func(d *Decision) { d.Kind = Rule }, false},
		{"a lifetime never disagrees", func(d *Decision) { d.Kind = Lifetime }, false},
		{"a key before admission", func(d *Decision) { d.Key = KeyAddress }, false},
		{"a key on the process", func(d *Decision) { d.Key = KeyProcess }, false},
		{"a reason that names no unit", func(d *Decision) { d.ReasonUnit = KeyNone }, false},
		{"a reason about a credential on the entry", func(d *Decision) { d.ReasonUnit = KeyCredential }, false},
		{"a process reason with no partner", func(d *Decision) { d.ReasonUnit = KeyProcess }, true},
		{"a process reason with a partner", func(d *Decision) { d.ReasonUnit, d.Partner = KeyProcess, "HLD-002" }, false},
		{"a per-user reason with a partner", func(d *Decision) { d.Partner = "HLD-004" }, true},
		{"a share", func(d *Decision) { d.Kind = Share }, true},
		{"a rate", func(d *Decision) { d.Kind = Rate }, true},
		{"a budget", func(d *Decision) { d.Kind = Budget }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.change(&d)
			if got := d.Disagrees(); got != tc.want {
				t.Errorf("Disagrees() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestKind_IsAllowance holds the four kinds that grant a key something to hold
// or spend apart from the three that do not.
func TestKind_IsAllowance(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind Kind
		want bool
	}{
		{"rule", Rule, false},
		{"ceiling", Ceiling, true},
		{"rate", Rate, true},
		{"budget", Budget, true},
		{"lifetime", Lifetime, false},
		{"bound", Bound, false},
		{"share", Share, true},
		{"unset", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.kind.IsAllowance(); got != tc.want {
				t.Errorf("Kind(%d).IsAllowance() = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

// TestChannel_String names every channel in kebab case (spec: Refusal
// channels), and an unknown one by its number.
func TestChannel_String(t *testing.T) {
	for _, tc := range []struct {
		channel Channel
		want    string
	}{
		{Gate, "gate"},
		{RPC, "rpc"},
		{ToolError, "tool-error"},
		{EmptyCompletion, "empty-completion"},
		{Withheld, "withheld"},
		{Absent, "absent"},
		{Unknown, "unknown"},
		{ListenEnd, "listen-end"},
		{SessionClose, "session-close"},
		{Silent, "silent"},
		{Startup, "startup"},
		{0, "Channel(0)"},
		{99, "Channel(99)"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.channel.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAnswer_String names every answer class in lower case (spec: Where a
// refused caller learns what to do), and an unknown one by its number.
func TestAnswer_String(t *testing.T) {
	for _, tc := range []struct {
		answer Answer
		want   string
	}{
		{RetryLater, "retry later"},
		{Reauthorize, "reauthorize"},
		{WidenScope, "widen the scope"},
		{AskOperator, "ask the operator"},
		{FixRequest, "fix the request"},
		{StartOver, "start over"},
		{NoAnswer, "none given"},
		{0, "Answer(0)"},
		{42, "Answer(42)"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.answer.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDecision_Carries reads a row's findings.
func TestDecision_Carries(t *testing.T) {
	d := Decision{Findings: []string{"F-04", "F-13"}}
	if !d.Carries("F-13") || d.Carries("F-07") || (Decision{}).Carries("F-04") {
		t.Errorf("Carries misreads %v", d.Findings)
	}
}

// TestDecision_RecordsDeparture accepts a departure only through a finding
// listed for that invariant, and ignores an id that names no finding.
func TestDecision_RecordsDeparture(t *testing.T) {
	d := Decision{Findings: []string{"F-99", "F-04"}}
	if !d.RecordsDeparture("INV-003") {
		t.Error("F-04 records INV-003 and was not read")
	}
	if d.RecordsDeparture("INV-004") {
		t.Error("no finding of the row records INV-004")
	}
	if (Decision{Findings: []string{"F-99"}}).RecordsDeparture("INV-003") {
		t.Error("F-99 names no finding and recorded a departure")
	}
}

// TestAllFindings_AreTheSpecificationsThirtyFiveWithTheirIssues holds the
// findings register to the specification's thirty-five, in order, each filed
// as the issue the tracker holds it under.
func TestAllFindings_AreTheSpecificationsThirtyFiveWithTheirIssues(t *testing.T) {
	issues := map[int][]string{
		950: {"F-29", "F-30"},
		951: {"F-03", "F-31"},
		952: {"F-08", "F-09", "F-17"},
		953: {"F-12", "F-15", "F-28"},
		954: {"F-14", "F-25"},
		955: {"F-01", "F-02", "F-05", "F-06", "F-18", "F-32"},
		956: {"F-07", "F-10"},
		957: {"F-13"},
		958: {"F-11", "F-16", "F-34"},
		959: {"F-19", "F-33"},
		960: {"F-04", "F-23", "F-26", "F-27"},
		961: {"F-20", "F-21", "F-22", "F-24"},
		982: {"F-35"},
	}
	want := map[string]int{}
	for issue, ids := range issues {
		for _, id := range ids {
			want[id] = issue
		}
	}
	findings := AllFindings()
	if len(findings) != 35 || len(want) != 35 {
		t.Fatalf("%d findings and %d in the issue map, want 35 of each", len(findings), len(want))
	}
	for i, f := range findings {
		t.Run(f.ID, func(t *testing.T) {
			if wantID := fmt.Sprintf("F-%02d", i+1); f.ID != wantID {
				t.Errorf("finding %d is %s, want %s", i, f.ID, wantID)
			}
			if f.Issue != want[f.ID] || FindingIssue(f.ID) != want[f.ID] {
				t.Errorf("%s is filed as issue %d, want %d", f.ID, f.Issue, want[f.ID])
			}
		})
	}
	if FindingIssue("F-36") != 0 {
		t.Error("F-36 names no finding and was given an issue")
	}
}

// TestFinding_Records names the invariants each rule of Validate accepts a
// departure from, and the findings that record each.
func TestFinding_Records(t *testing.T) {
	for _, tc := range []struct {
		invariant string
		findings  string
	}{
		{"INV-003", "F-04"},
		{"INV-004", "F-03"},
		{"INV-008", "F-08,F-09"},
		{"INV-010", "F-29,F-31,F-35"},
		{"INV-011", "F-07"},
		{"INV-012", "F-07,F-10"},
		{"INV-015", "F-34"},
		{"INV-016", "F-02,F-03,F-05,F-06"},
		{"INV-017", "F-13,F-16"},
		{"INV-018", "F-18,F-30,F-31"},
	} {
		t.Run(tc.invariant, func(t *testing.T) {
			var got []string
			for _, f := range AllFindings() {
				if f.Records(tc.invariant) {
					got = append(got, f.ID)
				}
			}
			if strings.Join(got, ",") != tc.findings {
				t.Errorf("findings recording %s = %v, want %s", tc.invariant, got, tc.findings)
			}
		})
	}
}
