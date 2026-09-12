//go:build e2e

// sweep_test.go checks that a sweep deletes exactly what belongs to it: the
// run's own leftovers and nothing another run, or another person, owns.

package fixture

import (
	"context"
	"slices"
	"testing"
)

// TestBelongsToRun_Names_MatchOnlyTheRunID pins the scoping rule: a name or
// a path carrying the run ID belongs to the run, a prefix alone does not,
// and an empty run ID matches nothing rather than everything.
func TestBelongsToRun_Names_MatchOnlyTheRunID(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	cases := []struct {
		name  string
		obj   string
		path  string
		runID string
		want  bool
	}{
		{name: "name carries it", obj: "proj-x-" + runID + "-abc-1", path: "user/other", runID: runID, want: true},
		{name: "path carries it", obj: "Display Name", path: "user/proj-" + runID, runID: runID, want: true},
		{name: "another run", obj: "proj-x-20260911t000000z-ffffffffff-common-1", path: "user/x", runID: runID, want: false},
		{name: "same prefix only", obj: "proj-x", path: "user/proj-x", runID: runID, want: false},
		{name: "empty run ID", obj: "anything", path: "anything", runID: "", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := belongsToRun(testCase.obj, testCase.path, testCase.runID); got != testCase.want {
				t.Errorf("belongsToRun(%q, %q, %q) = %t, want %t", testCase.obj, testCase.path, testCase.runID, got, testCase.want)
			}
		})
	}
}

// TestHasPrefix_Names_MatchTheNameOrTheLastPathSegment pins the explicit
// sweep's rule, which is wider on purpose and only ever run by hand.
func TestHasPrefix_Names_MatchTheNameOrTheLastPathSegment(t *testing.T) {
	cases := []struct {
		name   string
		obj    string
		path   string
		prefix string
		want   bool
	}{
		{name: "name", obj: "e2e-proj", path: "user/renamed", prefix: "e2e-", want: true},
		{name: "last segment", obj: "Proj", path: "group/e2e-proj", prefix: "e2e-", want: true},
		{name: "namespace only", obj: "proj", path: "e2e-group/proj", prefix: "e2e-", want: false},
		{name: "empty prefix", obj: "e2e-proj", path: "e2e-proj", prefix: "", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := hasPrefix(testCase.obj, testCase.path, testCase.prefix); got != testCase.want {
				t.Errorf("hasPrefix(%q, %q, %q) = %t, want %t", testCase.obj, testCase.path, testCase.prefix, got, testCase.want)
			}
		})
	}
}

// TestSweepRun_MixedInstance_RemovesOnlyThisRunsObjects drives a sweep over
// a stub holding this run's leftovers beside another run's and a person's,
// and checks the report and what is left.
func TestSweepRun_MixedInstance_RemovesOnlyThisRunsObjects(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	const otherRun = "20260911t090000z-9999999999-common"
	stub, client := newStubGitLab(t)
	stub.addProject(1, "proj-"+runID+"-1", "user/proj-"+runID+"-1")
	stub.addProject(2, "proj-"+otherRun+"-1", "user/proj-"+otherRun+"-1")
	stub.addProject(3, "real work", "user/real-work")
	stub.addGroup(10, "grp-"+runID+"-2", "grp-"+runID+"-2")
	stub.addGroup(11, "grp-"+otherRun+"-2", "grp-"+otherRun+"-2")
	stub.addUser(20, "usr-"+runID+"-3")
	stub.addUser(21, "alice")

	report := SweepRun(context.Background(), client, runID, true)

	if err := report.Err(); err != nil {
		t.Fatalf("SweepRun() errors = %v, want none", err)
	}
	if !report.Found() {
		t.Fatal("SweepRun() found nothing, want this run's three leftovers")
	}
	if got, want := report.Projects, []string{"user/proj-" + runID + "-1"}; !slices.Equal(got, want) {
		t.Errorf("projects removed = %v, want %v", got, want)
	}
	if got, want := report.Groups, []string{"grp-" + runID + "-2"}; !slices.Equal(got, want) {
		t.Errorf("groups removed = %v, want %v", got, want)
	}
	if got, want := report.Users, []string{"usr-" + runID + "-3"}; !slices.Equal(got, want) {
		t.Errorf("users removed = %v, want %v", got, want)
	}

	projects, groups := stub.remaining()
	slices.Sort(projects)
	slices.Sort(groups)
	if want := []string{"user/proj-" + otherRun + "-1", "user/real-work"}; !slices.Equal(projects, want) {
		t.Errorf("projects left = %v, want %v", projects, want)
	}
	if want := []string{"grp-" + otherRun + "-2"}; !slices.Equal(groups, want) {
		t.Errorf("groups left = %v, want %v", groups, want)
	}
	if got, want := report.String(), "1 project(s), 1 group(s), 1 user(s) removed; 0 error(s)"; got != want {
		t.Errorf("report = %q, want %q", got, want)
	}
}

// TestSweepRun_NotAdmin_LeavesUsersAlone checks that a token that cannot
// list users is not asked to, since the listing would be refused and would
// count as an error of the sweep.
func TestSweepRun_NotAdmin_LeavesUsersAlone(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	stub, client := newStubGitLab(t)
	stub.addUser(20, "usr-"+runID+"-3")

	report := SweepRun(context.Background(), client, runID, false)
	if err := report.Err(); err != nil {
		t.Fatalf("SweepRun() errors = %v, want none", err)
	}
	if len(report.Users) != 0 || report.Found() {
		t.Errorf("report = %s, want nothing touched without admin", report)
	}
	if got := stub.recordedDeletes(); len(got) != 0 {
		t.Errorf("deletes = %q, want none", got)
	}
}

// TestSweep_EmptyScope_IsRefused checks the two guards: a sweep with no run
// ID and a prefix sweep with no prefix would each delete everything the
// token owns, and answer with an error instead.
func TestSweep_EmptyScope_IsRefused(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(1, "anything", "user/anything")

	cases := []struct {
		name   string
		report SweepReport
	}{
		{name: "run without an ID", report: SweepRun(context.Background(), client, "", true)},
		{name: "prefix without a prefix", report: SweepPrefix(context.Background(), client, "", true)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.report.Err() == nil || testCase.report.Found() {
				t.Errorf("report = %s, want a refusal that touched nothing", testCase.report)
			}
		})
	}
	if got := stub.recordedDeletes(); len(got) != 0 {
		t.Errorf("deletes = %q, want none", got)
	}
}

// TestSweepPrefix_Objects_RemovesEveryRunsLeftoversUnderThePrefix checks the
// explicit sweep reaches across runs and still leaves what does not carry
// the prefix.
func TestSweepPrefix_Objects_RemovesEveryRunsLeftoversUnderThePrefix(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addProject(1, "e2e-proj-run1", "user/e2e-proj-run1")
	stub.addProject(2, "e2e-proj-run2", "user/e2e-proj-run2")
	stub.addProject(3, "keep", "user/keep")
	stub.addGroup(10, "e2e-grp", "e2e-grp")

	report := SweepPrefix(context.Background(), client, "e2e-", false)
	if err := report.Err(); err != nil {
		t.Fatalf("SweepPrefix() errors = %v, want none", err)
	}
	slices.Sort(report.Projects)
	if want := []string{"user/e2e-proj-run1", "user/e2e-proj-run2"}; !slices.Equal(report.Projects, want) {
		t.Errorf("projects removed = %v, want %v", report.Projects, want)
	}
	if want := []string{"e2e-grp"}; !slices.Equal(report.Groups, want) {
		t.Errorf("groups removed = %v, want %v", report.Groups, want)
	}
	if projects, _ := stub.remaining(); !slices.Equal(projects, []string{"user/keep"}) {
		t.Errorf("projects left = %v, want only user/keep", projects)
	}
}
