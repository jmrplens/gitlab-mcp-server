//go:build e2e

// sweep_test.go checks that a sweep deletes exactly what belongs to it: the
// run's own leftovers and nothing another run, or another person, owns, and
// for the on-demand sweep only what runs started before its floor left.

package fixture

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
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
	if got, want := report.String(), "1 project(s), 1 group(s), 0 snippet(s), 1 user(s) removed; 0 error(s)"; got != want {
		t.Errorf("report = %q, want %q", got, want)
	}
}

// TestSweepRun_PersonalSnippets_RemovesOnlyThisRunsOwn checks the sweep
// reaches the one kind of object a run makes that nothing else it tears down
// takes along: this run's personal snippets, a builder's and the World's, are
// deleted, and another run's, a person's and a project snippet carrying this
// run's ID are left. A deletion of any of those three would reach the stub
// unscripted and fail the test on its own.
func TestSweepRun_PersonalSnippets_RemovesOnlyThisRunsOwn(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	const otherRun = "20260911t090000z-9999999999-common"
	stub, client := newStubGitLab(t)
	stub.addSnippet(30, "snippet-testx-"+runID+"-abc-4", 0)
	stub.addSnippet(31, "world-snippet-"+runID, 0)
	stub.addSnippet(32, "snippet-testx-"+otherRun+"-abc-4", 0)
	stub.addSnippet(33, "my notes", 0)
	stub.addSnippet(34, "world-project-snippet-"+runID, 2)
	stub.answers(http.MethodDelete, "/api/v4/snippets/30", stubNoContent())
	stub.answers(http.MethodDelete, "/api/v4/snippets/31", stubNoContent())

	report := SweepRun(context.Background(), client, runID, false)

	if err := report.Err(); err != nil {
		t.Fatalf("SweepRun() errors = %v, want none", err)
	}
	if !report.Found() {
		t.Error("SweepRun() found nothing, want this run's two snippets")
	}
	if got, want := report.Snippets, []string{"snippet-testx-" + runID + "-abc-4", "world-snippet-" + runID}; !slices.Equal(got, want) {
		t.Errorf("snippets removed = %v, want %v", got, want)
	}
	if got, want := sentPaths(stub), []string{"DELETE /api/v4/snippets/30", "DELETE /api/v4/snippets/31"}; !slices.Equal(got, want) {
		t.Errorf("deletions sent = %v, want %v", got, want)
	}
}

// TestSweepPrefix_PersonalSnippets_MatchTheTitleAlone checks the prefix sweep
// reaches every run's personal snippets under the prefix, and judges a title
// as a name and never as a path: a person's "notes/snippet-x" holds a slash,
// and the last-segment rule meant for a project's path would otherwise sweep
// it. A title that only contains the prefix, and a project snippet, are left.
func TestSweepPrefix_PersonalSnippets_MatchTheTitleAlone(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addSnippet(40, "snippet-a-run1", 0)
	stub.addSnippet(41, "snippet-b-run2", 0)
	stub.addSnippet(42, "notes/snippet-x", 0)
	stub.addSnippet(43, "world-snippet-run1", 0)
	stub.addSnippet(44, "snippet-in-a-project", 7)
	stub.answers(http.MethodDelete, "/api/v4/snippets/40", stubNoContent())
	stub.answers(http.MethodDelete, "/api/v4/snippets/41", stubNoContent())

	report := SweepPrefix(context.Background(), client, "snippet-", false)

	if err := report.Err(); err != nil {
		t.Fatalf("SweepPrefix() errors = %v, want none", err)
	}
	if want := []string{"snippet-a-run1", "snippet-b-run2"}; !slices.Equal(report.Snippets, want) {
		t.Errorf("snippets removed = %v, want %v", report.Snippets, want)
	}
	if got, want := sentPaths(stub), []string{"DELETE /api/v4/snippets/40", "DELETE /api/v4/snippets/41"}; !slices.Equal(got, want) {
		t.Errorf("deletions sent = %v, want %v", got, want)
	}
}

// TestSweepRun_SnippetDeleteRefused_ReportsItAndKeepsGoing checks a snippet
// GitLab will not delete is an error of the sweep naming the snippet by its
// ID and its title, carrying GitLab's status, and does not keep the next
// snippet from going.
func TestSweepRun_SnippetDeleteRefused_ReportsItAndKeepsGoing(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	stub, client := newStubGitLab(t)
	stub.addSnippet(50, "snippet-"+runID+"-1", 0)
	stub.addSnippet(51, "snippet-"+runID+"-2", 0)
	stub.answers(http.MethodDelete, "/api/v4/snippets/50", stubRefusal(http.StatusForbidden, "403 Forbidden"))
	stub.answers(http.MethodDelete, "/api/v4/snippets/51", stubNoContent())

	report := SweepRun(context.Background(), client, runID, false)

	err := report.Err()
	if err == nil || !IsStatus(err, http.StatusForbidden) || !strings.Contains(err.Error(), "deleting snippet 50 (snippet-"+runID+"-1): ") {
		t.Errorf("SweepRun() error = %v, want GitLab's 403 under the refused snippet's ID and title", err)
	}
	if want := []string{"snippet-" + runID + "-2"}; !slices.Equal(report.Snippets, want) {
		t.Errorf("snippets removed = %v, want only %v", report.Snippets, want)
	}
}

// offsetTarget is a sweep target paging the way GitLab does, by offset, over
// rows that its removal takes out of the list, so a page asked for after a
// deletion begins further along than it did before it.
func offsetTarget(rows *[]sweepItem, pageSize int, failPage int64) sweepTarget {
	return sweepTarget{
		kind: "thing",
		list: func(_ context.Context, page int64) ([]sweepItem, int64, error) {
			if page == failPage {
				return nil, 0, errors.New("the listing is down")
			}
			start := int(page-1) * pageSize
			if start >= len(*rows) {
				return nil, 0, nil
			}
			end := min(start+pageSize, len(*rows))
			next := page + 1
			if end == len(*rows) {
				next = 0
			}
			return slices.Clone((*rows)[start:end]), next, nil
		},
		remove: func(_ context.Context, item sweepItem) error {
			*rows = slices.DeleteFunc(*rows, func(row sweepItem) bool { return row.id == item.id })
			return nil
		},
	}
}

// matchesThing accepts the offset target's rows a test means to delete.
func matchesThing(name, _ string) bool { return strings.HasPrefix(name, "match-") }

// TestSweepKind_DeletionShiftsTheOffsetPages_RemovesEveryMatch checks that
// one sweep takes every match however densely the leftovers sit. Deleting a
// page's matches before reading the next shifts the offset list under the
// sweep, so with four matches on two-row pages the old loop read the first
// page, deleted both, then read the second page at an offset the list no
// longer had matches at, and left two behind for every sweep after it.
func TestSweepKind_DeletionShiftsTheOffsetPages_RemovesEveryMatch(t *testing.T) {
	rows := []sweepItem{{id: 1, name: "match-1"}, {id: 2, name: "match-2"}, {id: 3, name: "match-3"}, {id: 4, name: "match-4"}, {id: 5, name: "keep"}}
	earlier := errors.New("an earlier kind's failure")

	removed, failures := sweepKind(context.Background(), offsetTarget(&rows, 2, 0), matchesThing, []error{earlier})

	if want := []string{"match-1", "match-2", "match-3", "match-4"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
	if want := []sweepItem{{id: 5, name: "keep"}}; !slices.Equal(rows, want) {
		t.Errorf("left = %v, want %v", rows, want)
	}
	if len(failures) != 1 || !errors.Is(failures[0], earlier) {
		t.Errorf("failures = %v, want only the earlier one handed in", failures)
	}
}

// TestSweepKind_ALaterPageFails_RemovesWhatEarlierPagesListed checks a page
// that cannot be read ends the listing without throwing away what the pages
// before it matched: those are still deleted, and the failure names the kind
// and the page.
func TestSweepKind_ALaterPageFails_RemovesWhatEarlierPagesListed(t *testing.T) {
	rows := []sweepItem{{id: 1, name: "match-1"}, {id: 2, name: "keep"}, {id: 3, name: "match-3"}}

	removed, failures := sweepKind(context.Background(), offsetTarget(&rows, 2, 2), matchesThing, nil)

	if want := []string{"match-1"}; !slices.Equal(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
	if len(failures) != 1 || !strings.Contains(failures[0].Error(), "listing things (page 2): the listing is down") {
		t.Errorf("failures = %v, want the second page's failure named by kind and page", failures)
	}
}

// TestSweepRunsStartedBefore_MixedInstance_RemovesOnlyRunsPastTheCutoff
// drives the on-demand sweep's default over an instance holding every kind
// of object from a run that started before the cutoff, beside the same kinds
// from a run started after it, which may still be going, a run whose
// identifier E2E_RUN_ID replaced, and a person's. Only the first run's go,
// a World project included, whose name carries no run and whose path does.
//
// It also checks the listings went out unsearched, since no search can name
// a shape: a sweep that sent one would find nothing on a real instance while
// the stub, which filters by substring, still answered.
func TestSweepRunsStartedBefore_MixedInstance_RemovesOnlyRunsPastTheCutoff(t *testing.T) {
	const oldRun = "20260912t101500z-0123456789-common"
	const liveRun = "20260912t130000z-abcdef0123-ee"
	const overridden = "nightly-common"
	cutoff := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	stub, client := newStubGitLab(t)
	stub.addProject(1, "proj-x-"+oldRun+"-abc-1", "user/proj-x-"+oldRun+"-abc-1")
	stub.addProject(2, "proj-x-"+liveRun+"-abc-1", "user/proj-x-"+liveRun+"-abc-1")
	stub.addProject(3, "proj-x-"+overridden+"-abc-1", "user/proj-x-"+overridden+"-abc-1")
	stub.addProject(4, "real work", "user/real-work")
	stub.addProject(5, "World project", "e2e-world-group-"+oldRun+"/e2e-world-project-"+oldRun)
	stub.addGroup(10, "e2e-world-group-"+oldRun, "e2e-world-group-"+oldRun)
	stub.addGroup(11, "grp-"+liveRun+"-2", "grp-"+liveRun+"-2")
	stub.addSnippet(30, "snippet-x-"+oldRun+"-abc-2", 0)
	stub.addSnippet(31, "snippet-x-"+liveRun+"-abc-2", 0)
	stub.addSnippet(32, "notes", 0)
	stub.addUser(20, "member-x-"+oldRun+"-abc-3")
	stub.addUser(21, "member-x-"+liveRun+"-abc-3")
	stub.addUser(22, "alice")
	stub.answers(http.MethodDelete, "/api/v4/snippets/30", stubNoContent())

	report := SweepRunsStartedBefore(context.Background(), client, cutoff, true)

	if err := report.Err(); err != nil {
		t.Fatalf("SweepRunsStartedBefore() errors = %v, want none", err)
	}
	slices.Sort(report.Projects)
	if want := []string{"e2e-world-group-" + oldRun + "/e2e-world-project-" + oldRun, "user/proj-x-" + oldRun + "-abc-1"}; !slices.Equal(report.Projects, want) {
		t.Errorf("projects removed = %v, want %v", report.Projects, want)
	}
	if want := []string{"e2e-world-group-" + oldRun}; !slices.Equal(report.Groups, want) {
		t.Errorf("groups removed = %v, want %v", report.Groups, want)
	}
	if want := []string{"snippet-x-" + oldRun + "-abc-2"}; !slices.Equal(report.Snippets, want) {
		t.Errorf("snippets removed = %v, want %v", report.Snippets, want)
	}
	if want := []string{"member-x-" + oldRun + "-abc-3"}; !slices.Equal(report.Users, want) {
		t.Errorf("users removed = %v, want %v", report.Users, want)
	}
	projects, groups := stub.remaining()
	slices.Sort(projects)
	if want := []string{"user/proj-x-" + liveRun + "-abc-1", "user/proj-x-" + overridden + "-abc-1", "user/real-work"}; !slices.Equal(projects, want) {
		t.Errorf("projects left = %v, want %v", projects, want)
	}
	if want := []string{"grp-" + liveRun + "-2"}; !slices.Equal(groups, want) {
		t.Errorf("groups left = %v, want %v", groups, want)
	}
	listings := stub.recordedListings()
	if len(listings) != 4 {
		t.Errorf("listings = %d, want one each of projects, groups, snippets and users", len(listings))
	}
	for _, listing := range listings {
		if listing.Query.Has("search") {
			t.Errorf("%s %s searched by %q, want it read whole", listing.Method, listing.Path, listing.Query.Get("search"))
		}
	}
}

// TestSweepPrefix_Listings_SearchByThePrefix checks the prefix sweep asks
// GitLab for the prefix where a listing can search, so a shared instance is
// not read whole to find a handful of names, and leaves the snippet listing,
// which cannot search, unsearched.
func TestSweepPrefix_Listings_SearchByThePrefix(t *testing.T) {
	stub, client := newStubGitLab(t)

	report := SweepPrefix(context.Background(), client, "e2e-", true)

	if err := report.Err(); err != nil || report.Found() {
		t.Fatalf("SweepPrefix() over an empty instance = %s, %v; want nothing found and no error", report, err)
	}
	want := map[string]string{"/api/v4/projects": "e2e-", "/api/v4/groups": "e2e-", "/api/v4/users": "e2e-", "/api/v4/snippets": ""}
	listings := stub.recordedListings()
	if len(listings) != len(want) {
		t.Errorf("listings = %v, want one of each of %v", listings, want)
	}
	for _, listing := range listings {
		t.Run(listing.Path, func(t *testing.T) {
			wantSearch, known := want[listing.Path]
			if !known {
				t.Fatalf("listed %s, which is no kind the sweep reaches", listing.Path)
			}
			if got := listing.Query.Get("search"); got != wantSearch || listing.Query.Has("search") != (wantSearch != "") {
				t.Errorf("searched by %q (sent: %t), want %q", got, listing.Query.Has("search"), wantSearch)
			}
		})
	}
}

// TestSweep_ListingsRefused_ReportEachKindAndGoOn checks a sweep that cannot
// read a kind at all says which kind and which page, carries GitLab's
// refusal, and still asks for every other kind: one listing an instance
// refuses must not hide the leftovers of the other three.
func TestSweep_ListingsRefused_ReportEachKindAndGoOn(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.refusals = map[string]int{
			"/api/v4/projects": http.StatusForbidden, "/api/v4/groups": http.StatusForbidden,
			"/api/v4/snippets": http.StatusForbidden, "/api/v4/users": http.StatusForbidden,
		}
	})

	report := SweepPrefix(context.Background(), client, "e2e-", true)

	if report.Found() {
		t.Errorf("SweepPrefix() over unreadable listings found %v, want nothing", report.Removed())
	}
	if len(report.Errors) != 4 {
		t.Fatalf("errors = %v, want one per kind", report.Errors)
	}
	for i, kind := range []string{"projects", "groups", "snippets", "users"} {
		t.Run(kind, func(t *testing.T) {
			err := report.Errors[i]
			if !strings.HasPrefix(err.Error(), "listing "+kind+" (page 1): ") || !IsStatus(err, http.StatusForbidden) {
				t.Errorf("error = %v, want the %s listing named with GitLab's 403", err, kind)
			}
		})
	}
}

// TestSweepRun_UserDeletions_ToleratesOneGoneAndReportsARefusal checks the
// two endings of a user's deletion besides success: a user already gone is
// as gone as the sweep asked for and is reported removed, and a user GitLab
// will not delete is an error naming the user by ID and username, with
// GitLab's status, that does not stop the next user from going.
func TestSweepRun_UserDeletions_ToleratesOneGoneAndReportsARefusal(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	stub, client := newStubGitLab(t)
	stub.addUser(20, "usr-"+runID+"-refused")
	stub.addUser(21, "usr-"+runID+"-gone")
	stub.addUser(22, "usr-"+runID+"-deleted")
	stub.configure(func() {
		stub.refusals = map[string]int{"/api/v4/users/20": http.StatusForbidden, "/api/v4/users/21": http.StatusNotFound}
	})

	report := SweepRun(context.Background(), client, runID, true)

	slices.Sort(report.Users)
	if want := []string{"usr-" + runID + "-deleted", "usr-" + runID + "-gone"}; !slices.Equal(report.Users, want) {
		t.Errorf("users removed = %v, want %v", report.Users, want)
	}
	err := report.Err()
	if len(report.Errors) != 1 || !IsStatus(err, http.StatusForbidden) || !strings.HasPrefix(err.Error(), "deleting user 20 (usr-"+runID+"-refused): ") {
		t.Errorf("errors = %v, want only the refused user's, named by ID and username with GitLab's 403", report.Errors)
	}
}

// TestSweepReport_Removed_NamesEveryKindInOrder checks the one list both
// logs read: every kind, each entry after its kind, in the order the sweep
// deletes them, and a report that removed nothing lists nothing and says it
// found nothing.
func TestSweepReport_Removed_NamesEveryKindInOrder(t *testing.T) {
	report := SweepReport{Projects: []string{"p"}, Groups: []string{"g"}, Snippets: []string{"s"}, Users: []string{"u"}}

	if want := []string{"project p", "group g", "snippet s", "user u"}; !slices.Equal(report.Removed(), want) {
		t.Errorf("Removed() = %v, want %v", report.Removed(), want)
	}
	if !report.Found() {
		t.Error("Found() = false for a report that removed four objects")
	}
	if got, want := report.String(), "1 project(s), 1 group(s), 1 snippet(s), 1 user(s) removed; 0 error(s)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if snippetsOnly := (SweepReport{Snippets: []string{"s"}}); !snippetsOnly.Found() {
		t.Error("Found() = false for a report that removed only a snippet")
	}
	if empty := (SweepReport{}); empty.Found() || empty.Removed() != nil {
		t.Errorf("an empty report: Found() = %t, Removed() = %v; want false and nothing", empty.Found(), empty.Removed())
	}
}

// captureExitHooks arms the exit sweep afresh for one test and collects
// what it registers instead of handing it to the harness, whose hooks never
// run in a test process. It restores both afterwards, so a later builder
// arms the sweep as it would have.
func captureExitHooks(t *testing.T) *[]func() error {
	t.Helper()
	var hooks []func() error
	previousOnce, previousRegister := sweepOnce, registerExitHook
	sweepOnce = new(sync.Once)
	registerExitHook = func(hook func() error) { hooks = append(hooks, hook) }
	t.Cleanup(func() { sweepOnce, registerExitHook = previousOnce, previousRegister })
	return &hooks
}

// captureLog sends the standard logger to a buffer for one test, which is
// where the exit sweep reports what it removed.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buf
}

// TestExitSweep_Leftovers_LogsWhatItRemovedAndFailsOnTheRest checks the
// hook a run's exit runs: silent when the tests cleaned up after themselves,
// a leak logged object by object when they did not and the sweep removed it,
// and a failure of the run naming the run when the sweep could not.
func TestExitSweep_Leftovers_LogsWhatItRemovedAndFailsOnTheRest(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	snippet := "snippet-" + runID + "-1"

	t.Run("nothing left", func(t *testing.T) {
		logged := captureLog(t)
		_, client := newStubGitLab(t)

		if err := exitSweep(client, runID, true)(); err != nil {
			t.Errorf("exitSweep() = %v, want nil", err)
		}
		if logged.Len() != 0 {
			t.Errorf("exitSweep() logged %q over a clean run, want nothing", logged.String())
		}
	})

	t.Run("leftovers removed", func(t *testing.T) {
		logged := captureLog(t)
		stub, client := newStubGitLab(t)
		stub.addProject(1, "proj-"+runID+"-1", "user/proj-"+runID+"-1")
		stub.addGroup(10, "grp-"+runID+"-2", "grp-"+runID+"-2")
		stub.addSnippet(30, snippet, 0)
		stub.addUser(20, "usr-"+runID+"-3")
		stub.answers(http.MethodDelete, "/api/v4/snippets/30", stubNoContent())

		if err := exitSweep(client, runID, true)(); err != nil {
			t.Errorf("exitSweep() = %v, want nil", err)
		}
		for _, want := range []string{
			"e2e: sweep for run " + runID + " removed leftovers a cleanup did not: 1 project(s), 1 group(s), 1 snippet(s), 1 user(s) removed; 0 error(s)",
			"e2e: sweep removed project user/proj-" + runID + "-1",
			"e2e: sweep removed group grp-" + runID + "-2",
			"e2e: sweep removed snippet " + snippet,
			"e2e: sweep removed user usr-" + runID + "-3",
		} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(logged.String(), want) {
					t.Errorf("exitSweep() logged %q, want it to say %q", logged.String(), want)
				}
			})
		}
	})

	t.Run("a leftover it could not remove", func(t *testing.T) {
		stub, client := newStubGitLab(t)
		stub.addSnippet(30, snippet, 0)
		stub.answers(http.MethodDelete, "/api/v4/snippets/30", stubRefusal(http.StatusForbidden, "403 Forbidden"))

		err := exitSweep(client, runID, false)()
		if err == nil || !IsStatus(err, http.StatusForbidden) || !strings.HasPrefix(err.Error(), "sweep for run "+runID+": deleting snippet 30 ("+snippet+"): ") {
			t.Errorf("exitSweep() = %v, want the run failed naming the run and the snippet, with GitLab's 403", err)
		}
	})
}

// TestArmSweep_Builders_RegisterOneExitSweepPerProcess checks the arming
// itself: the first builder registers the exit sweep, and a second one in the
// same process registers nothing more, since one sweep per run is what the
// hook is scoped to.
func TestArmSweep_Builders_RegisterOneExitSweepPerProcess(t *testing.T) {
	hooks := captureExitHooks(t)
	_, client := newStubGitLab(t)
	e := harness.NewDetached(t, client)

	armSweep(e)
	armSweep(e)

	if len(*hooks) != 1 {
		t.Errorf("armSweep() twice registered %d hooks, want 1", len(*hooks))
	}
}

// TestArmRunSweep_ServerMadeSnippet_ArmsTheSweepThatRemovesIt checks the
// arming a scenario asks for when it makes a personal snippet through the
// server rather than through NewSnippet: one exit sweep however often it is
// asked, and that sweep deletes a snippet named after the run and leaves one
// named after another run.
func TestArmRunSweep_ServerMadeSnippet_ArmsTheSweepThatRemovesIt(t *testing.T) {
	hooks := captureExitHooks(t)
	stub, client := newStubGitLab(t)
	var runID string

	t.Run("asked on every surface", func(t *testing.T) {
		e := harness.NewDetached(t, client)
		runID = e.RunID()
		ArmRunSweep(e)
		ArmRunSweep(e)
	})

	if len(*hooks) != 1 {
		t.Fatalf("ArmRunSweep() twice registered %d exit hooks, want the one exit sweep", len(*hooks))
	}
	stub.addSnippet(20, "snippet-"+runID+"-abc-1", 0)
	stub.addSnippet(21, "snippet-20260911t090000z-9999999999-common-abc-1", 0)
	stub.answers(http.MethodDelete, "/api/v4/snippets/20", stubNoContent())

	if err := (*hooks)[0](); err != nil {
		t.Fatalf("the exit sweep ArmRunSweep armed failed: %v", err)
	}
	deletions := slices.DeleteFunc(sentPaths(stub), func(sent string) bool { return !strings.HasPrefix(sent, http.MethodDelete) })
	if want := []string{"DELETE /api/v4/snippets/20"}; !slices.Equal(deletions, want) {
		t.Errorf("the exit sweep sent the deletions %v, want %v", deletions, want)
	}
}

// TestResolveOrphanScope_Settings_PickTheSweep checks which on-demand sweep
// each setting asks for: none when neither is set, which is what keeps a bare
// go test of this package from deleting anything; the prefix when it is set,
// whatever the age says; otherwise the age, read as a floor before now; and a
// refusal for an age that is not one, or that would reach runs still going.
func TestResolveOrphanScope_Settings_PickTheSweep(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		env       map[string]string
		wantAsked bool
		want      orphanScope
		wantErr   string
	}{
		{name: "nothing set", env: map[string]string{}},
		{name: "a blank age", env: map[string]string{sweepMinAgeEnv: "   "}},
		{name: "a prefix", env: map[string]string{sweepPrefixEnv: " e2e- ", sweepMinAgeEnv: "2h"}, wantAsked: true, want: orphanScope{prefix: "e2e-"}},
		{name: "a blank prefix", env: map[string]string{sweepPrefixEnv: "  ", sweepMinAgeEnv: "2h"}, wantAsked: true, want: orphanScope{cutoff: now.Add(-2 * time.Hour), minAge: 2 * time.Hour}},
		{name: "an age", env: map[string]string{sweepMinAgeEnv: " 90m "}, wantAsked: true, want: orphanScope{cutoff: now.Add(-90 * time.Minute), minAge: 90 * time.Minute}},
		{name: "no age at all", env: map[string]string{sweepMinAgeEnv: "0s"}, wantAsked: true, want: orphanScope{cutoff: now}},
		{name: "an age that is not one", env: map[string]string{sweepMinAgeEnv: "soon"}, wantAsked: true, wantErr: `E2E_SWEEP_MIN_AGE="soon" is not a duration such as 2h: `},
		{name: "a negative age", env: map[string]string{sweepMinAgeEnv: "-1s"}, wantAsked: true, wantErr: `E2E_SWEEP_MIN_AGE="-1s" is negative, which would reach runs still going`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, asked, err := resolveOrphanScope(func(key string) string { return testCase.env[key] }, now)

			if asked != testCase.wantAsked {
				t.Errorf("asked = %t, want %t", asked, testCase.wantAsked)
			}
			if testCase.wantErr == "" && err != nil {
				t.Errorf("error = %v, want none", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.HasPrefix(err.Error(), testCase.wantErr)) {
				t.Errorf("error = %v, want one opening %q", err, testCase.wantErr)
			}
			if got != testCase.want {
				t.Errorf("scope = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestSweepMinAge_MakefileDefaults_OutlastEveryPackage holds the floor make
// e2e-clean-orphans passes by default above the longest a package's fixtures
// can be in use under the default timeouts the same Makefile hands go test.
//
// The floor is the only thing keeping the default sweep off a run still
// going, and nothing else ties it to the two timeouts, which are defined
// hundreds of lines away from it. E2E_GITLAB_TIMEOUT has been raised once
// already; raised again until it plus the exit hooks' budgets reached the
// floor, it would let the sweep delete a live package's fixtures with every
// test green. This test fails from that point on, which at the 2h default is
// a timeout of 112 minutes or more. The timeout's alarm is a fatal
// panic, so the exit hooks run only after tests that ended in time, and then
// under their own budgets: the floor must clear the longer timeout by those
// two budgets. go test's backstop, a SIGQUIT a tenth of the timeout past it
// and never less than a minute, only shortens that. What this reads is the
// Makefile's defaults, so a timeout raised for one run on the command line,
// or passed to a go test of its own, is outside it; the Makefile says what to
// set the floor to then.
func TestSweepMinAge_MakefileDefaults_OutlastEveryPackage(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the repository Makefile: %v", err)
	}

	floor := makefileDefault(t, makefile, "E2E_SWEEP_MIN_AGE")
	longest := max(makefileDefault(t, makefile, "E2E_GITLAB_TIMEOUT"), makefileDefault(t, makefile, "E2E_DOCKER_ENTERPRISE_TIMEOUT"))
	inUse := longest + sweepBudget + worldTeardownBudget

	if floor <= inUse {
		t.Errorf("E2E_SWEEP_MIN_AGE defaults to %s, want more than %s: the longer suite timeout, %s, plus the exit sweep's %s and the World teardown's %s",
			floor, inUse, longest, sweepBudget, worldTeardownBudget)
	}
}

// TestCleanOrphansRecipe_SweepMinAge_CommandLineThenEnvironmentThenDotEnv
// renders the make e2e-clean-orphans recipe and runs it against a stand-in
// go, holding the floor it hands the sweep to the order the Makefile
// documents: the make command line, then the environment, then .env, and only
// then the 2h default.
//
// The floor is the one thing keeping the default sweep off a run still going,
// and the recipe used to prefix go test with make's own value, which is the 2h
// default whenever the variable is not on the command line or in the
// environment, because the Makefile does not include .env: a floor raised
// there was dropped without a word. The recipe is rendered with make -n
// rather than read as text, so what is judged is the command make would run,
// and it runs in a directory of its own, so the .env it sources is the one the
// case writes and never the repository's.
//
// The .env a case writes sets a sweep prefix as well, and every case holds
// the sweep to none: the prefix is read from the command line or the
// environment alone, on purpose, since one left in .env would turn every
// later clean into a sweep by name that reaches live runs.
func TestCleanOrphansRecipe_SweepMinAge_CommandLineThenEnvironmentThenDotEnv(t *testing.T) {
	recipe := newCleanOrphansRecipe(t)

	cases := []struct {
		name string
		// environment is the floor in make's environment, "" for none.
		environment string
		// commandLine is the floor on make's command line, "" for none.
		commandLine string
		// dotEnv is whether the directory the recipe runs in holds a .env
		// setting the floor to 5h.
		dotEnv bool
		want   string
	}{
		{name: "set nowhere", want: "2h"},
		{name: "set in .env alone", dotEnv: true, want: "5h"},
		{name: "set in the environment and in .env", environment: "4h", dotEnv: true, want: "4h"},
		{name: "set on the command line, in the environment and in .env", environment: "4h", commandLine: "3h", dotEnv: true, want: "3h"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			environ := recipeEnviron(testCase.environment)
			out := recipe.run(t, recipe.render(t, environ, testCase.commandLine), environ, testCase.dotEnv)

			if !strings.Contains(out, "floor="+testCase.want+"\n") {
				t.Errorf("the sweep was handed %q, want the floor %s", out, testCase.want)
			}
			if !strings.Contains(out, "prefix=\n") {
				t.Errorf("the sweep was handed %q, want no prefix: one set in .env is not read", out)
			}
			if !strings.Contains(out, "-run ^TestSweepOrphans_Leftovers_OnDemand$ ./test/e2e/internal/fixture/") {
				t.Errorf("the recipe ran %q, want the on-demand sweep of this package", out)
			}
		})
	}
}

// cleanOrphansRecipe is what rendering and running the e2e-clean-orphans
// recipe needs: make, the shell make runs a recipe line with, the repository
// the Makefile sits in, and a directory holding the stand-in go.
type cleanOrphansRecipe struct {
	makeBin, shell, root, stubs string
}

// newCleanOrphansRecipe finds the tools the recipe needs, skipping the test
// where one is missing, and writes the stand-in go. It says which floor it was
// handed and what it was asked to run, and does nothing else: no GitLab is
// reached and nothing is swept.
func newCleanOrphansRecipe(t *testing.T) cleanOrphansRecipe {
	t.Helper()
	var recipe cleanOrphansRecipe
	var err error
	if recipe.makeBin, err = exec.LookPath("make"); err != nil {
		t.Skipf("make is not installed here, so the recipe cannot be rendered: %v", err)
	}
	// make runs a recipe line with sh, and this recipe hands its body to bash.
	if recipe.shell, err = exec.LookPath("sh"); err != nil {
		t.Skipf("sh is not installed here, so the rendered recipe cannot be run: %v", err)
	}
	if _, err = exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not installed here, so the rendered recipe cannot be run: %v", err)
	}
	if recipe.root, err = filepath.Abs(filepath.Join("..", "..", "..", "..")); err != nil {
		t.Fatalf("resolving the repository root: %v", err)
	}
	recipe.stubs = t.TempDir()
	stub := "#!/bin/sh\nprintf 'floor=%s\\n' \"$E2E_SWEEP_MIN_AGE\"\nprintf 'prefix=%s\\n' \"$E2E_SWEEP_PREFIX\"\nprintf 'args=%s\\n' \"$*\"\n"
	//#nosec G306 -- the stand-in go is a script the rendered recipe must execute
	if err = os.WriteFile(filepath.Join(recipe.stubs, "go"), []byte(stub), 0o700); err != nil {
		t.Fatalf("writing the stand-in go: %v", err)
	}
	return recipe
}

// render asks make for the recipe it would run under environ, with the floor
// on its command line when commandLine is not empty.
func (r cleanOrphansRecipe) render(t *testing.T, environ []string, commandLine string) string {
	t.Helper()
	args := []string{"-n", "-s", "--no-print-directory", "e2e-clean-orphans"}
	if commandLine != "" {
		args = append(args, sweepMinAgeEnv+"="+commandLine)
	}
	//#nosec G204 -- make from the PATH, with this file's own arguments and a case's duration
	render := exec.CommandContext(t.Context(), r.makeBin, args...)
	render.Dir = r.root
	render.Env = environ
	recipe, err := render.Output()
	if err != nil {
		t.Fatalf("make -n e2e-clean-orphans: %v", err)
	}
	return string(recipe)
}

// run runs a rendered recipe the way make would, in a directory of its own
// holding a .env that sets the floor to 5h and a sweep prefix when dotEnv is
// true, with the stand-in go first on the PATH, and returns what the stand-in
// printed.
func (r cleanOrphansRecipe) run(t *testing.T, recipe string, environ []string, dotEnv bool) string {
	t.Helper()
	work := t.TempDir()
	if dotEnv {
		if err := os.WriteFile(filepath.Join(work, ".env"), []byte(sweepMinAgeEnv+"=5h\n"+sweepPrefixEnv+"=adm-alert-\n"), 0o600); err != nil {
			t.Fatalf("writing .env: %v", err)
		}
	}
	//#nosec G204 -- the recipe is what make rendered from this repository's own Makefile
	run := exec.CommandContext(t.Context(), r.shell, "-c", recipe)
	run.Dir = work
	// The last PATH wins, which puts the stand-in first.
	run.Env = append(slices.Clone(environ), "PATH="+r.stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("running the rendered recipe: %v\n%s", err, out)
	}
	return string(out)
}

// recipeEnviron is this process's environment with nothing in it that would
// decide the floor or the make invocation on the test's behalf: neither sweep
// variable, and none of the variables a parent make hands a child, since a
// variable given on a parent's command line reaches a child make through
// MAKEFLAGS as a command-line variable of its own. The floor is added back
// when the case puts one in the environment.
func recipeEnviron(floor string) []string {
	dropped := []string{sweepMinAgeEnv, sweepPrefixEnv, "MAKEFLAGS", "MFLAGS", "GNUMAKEFLAGS", "MAKELEVEL", "MAKEOVERRIDES"}
	environ := slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return slices.Contains(dropped, name)
	})
	if floor != "" {
		environ = append(environ, sweepMinAgeEnv+"="+floor)
	}
	return environ
}

// makefileDefault reads the duration a `NAME ?= value` line of the Makefile
// gives name, and fails the test when there is none or it is not a duration.
func makefileDefault(t *testing.T, makefile []byte, name string) time.Duration {
	t.Helper()
	match := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + ` \?= (\S+)$`).FindSubmatch(makefile)
	if match == nil {
		t.Fatalf("the Makefile sets no default for %s", name)
	}
	value, err := time.ParseDuration(string(match[1]))
	if err != nil {
		t.Fatalf("the Makefile's default for %s is not a duration: %v", name, err)
	}
	return value
}

// TestOrphanScope_Sweep_RunsTheSweepItNames checks each scope runs its own
// sweep over one instance holding an object of each: the prefix scope takes
// the prefixed object and leaves the old run's, and the age scope the other
// way round, and each says what it reached in the words the log line uses.
func TestOrphanScope_Sweep_RunsTheSweepItNames(t *testing.T) {
	const oldRun = "20260912t101500z-0123456789-common"
	cutoff := time.Date(2026, 9, 12, 14, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	cases := []struct {
		name       string
		scope      orphanScope
		wantPath   string
		wantString string
	}{
		{name: "by prefix", scope: orphanScope{prefix: "e2e-"}, wantPath: "user/e2e-notes", wantString: `names opening with "e2e-"`},
		{
			name: "by run age", scope: orphanScope{cutoff: cutoff, minAge: 2 * time.Hour}, wantPath: "user/proj-" + oldRun + "-1",
			wantString: "names of runs started before 2026-09-12T12:00:00Z (2h0m0s ago)",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.addProject(1, "e2e-notes", "user/e2e-notes")
			stub.addProject(2, "proj-"+oldRun+"-1", "user/proj-"+oldRun+"-1")

			report := testCase.scope.run(context.Background(), client, false)

			if err := report.Err(); err != nil {
				t.Fatalf("run() errors = %v, want none", err)
			}
			if want := []string{testCase.wantPath}; !slices.Equal(report.Projects, want) {
				t.Errorf("projects removed = %v, want %v", report.Projects, want)
			}
			if got := testCase.scope.String(); got != testCase.wantString {
				t.Errorf("String() = %q, want %q", got, testCase.wantString)
			}
		})
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
