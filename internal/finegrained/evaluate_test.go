package finegrained

import (
	"bytes"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// phaseBTable is a hand-built table with one action for each way a grant can
// reach, or fail to reach, what it runs:
//
//   - project.get needs Project: Read at a project or a group;
//   - merge_request.approve needs Merge Request: Approve at a project;
//   - merge_request.approve_or_read runs either way, an approval or a read;
//   - issue_link.list needs Issue Link: Read at a project, which a public
//     project serves to anyone;
//   - project.mixed needs Project: Read and Issue Link: Read together at a
//     project, one granted and one public;
//   - skip.action opts out of the check;
//   - user.get needs User: Read at the user;
//   - vulnerability.get reads Vulnerability on its spine and the UserCore
//     author off it;
//   - z.denied is denied whatever the grant.
func phaseBTable() *Table {
	return &Table{
		Version:        "19.4.1-ee",
		Bucket:         "19.4",
		Permissions:    []string{"read_project", "approve_merge_request", "read_issue_link", "read_user", "read_vulnerability"},
		Display:        []string{"Project: Read", "Merge Request: Approve", "Issue Link: Read", "User: Read", "Vulnerability: Read"},
		RefusalDisplay: []string{"Project: Read", "Merge Request: Approve", "Issue Link: Read", "User: Read", "Vulnerability: Read"},
		Assignables: []Assignable{
			{Name: "project_read", Permissions: []uint16{0}, Boundaries: BoundaryProject | BoundaryGroup, Grantable: true},
			{Name: "mr_approve", Permissions: []uint16{1}, Boundaries: BoundaryProject, Grantable: true},
			{Name: "issue_link_read", Permissions: []uint16{2}, Boundaries: BoundaryProject, Grantable: true},
			{Name: "user_read", Permissions: []uint16{3}, Boundaries: BoundaryUser, Grantable: true},
			{Name: "vulnerability_read", Permissions: []uint16{4}, Boundaries: BoundaryProject, Grantable: true},
		},
		PublicAnonymous: [2][]uint64{{1 << 2}, {0}},
		PublicKnown:     true,
		Groups: []Group{
			{Perms: []uint16{0}, Any: BoundaryProject | BoundaryGroup},
			{Perms: []uint16{1}, Any: BoundaryProject},
			{Perms: []uint16{2}, Any: BoundaryProject},
			{Perms: []uint16{3}, Any: BoundaryUser},
			{Perms: []uint16{0, 2}, Any: BoundaryProject},
			{Perms: []uint16{4}, Any: BoundaryProject},
		},
		Operations: []Operation{
			{Name: "GET /projects/:id", Groups: []uint32{0}},
			{Name: "POST /projects/:id/merge_requests/:merge_request_iid/approve", Groups: []uint32{1}},
			{Name: "GET /projects/:id/issues/:issue_iid/links", Groups: []uint32{2}},
			{Name: "GET /user", Groups: []uint32{3}},
			{Name: "GET /projects/:id/mixed", Groups: []uint32{4}},
			{Name: "query vulnerability (vulnerabilityQuery)", Spine: []uint32{0}, OffSpine: []uint32{1, 2}},
			{Name: "POST /skip", Groups: []uint32{1}, Skip: true},
		},
		Elements: []Element{
			{Path: "vulnerability", Type: "Vulnerability", Groups: []uint32{5}, Effect: EffectNull},
			{Path: "vulnerability.author", Type: "UserCore", Groups: []uint32{3}, Effect: EffectNull},
			{Path: "vulnerability.findingTokenStatus", Type: "VulnerabilityFindingTokenStatus", Undeclared: true, Effect: EffectNull},
		},
		Actions: []Requirement{
			{ID: "issue_link.list", Paths: [][]uint32{{2}}},
			{ID: "merge_request.approve", Paths: [][]uint32{{1}}},
			{ID: "merge_request.approve_or_read", Paths: [][]uint32{{1, 2}, {0}}},
			{ID: "project.get", Paths: [][]uint32{{0}}},
			{ID: "project.mixed", Paths: [][]uint32{{4}}},
			{ID: "skip.action", Paths: [][]uint32{{6}}},
			{ID: "user.get", Paths: [][]uint32{{3}}},
			{ID: "vulnerability.get", Paths: [][]uint32{{5}}, Degraded: []uint32{2}, GraphQL: true},
			{ID: "z.denied", Denied: &Denial{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull}},
		},
	}
}

// scope is a scope of the given access, namespace and assignable names.
func scope(access GrantAccess, namespace NamespaceKind, names ...string) Scope {
	return Scope{Access: access, Namespace: namespace, Permissions: names}
}

// TestEvaluate_ListsWhatTheGrantCoversAndCallsWhatGitLabWouldServe verifies,
// action by action, what a grant is listed and what the call guard passes: a
// group covered at one of its boundaries is listed, a permission public to
// anonymous callers on a public project passes the call guard without being
// listed, a group needing one granted and one public permission is callable,
// an opted-out operation passes whatever the grant, a spine position must be
// covered, and a denied action is neither.
func TestEvaluate_ListsWhatTheGrantCoversAndCallsWhatGitLabWouldServe(t *testing.T) {
	type verdict struct{ listed, callable bool }
	cases := []struct {
		name  string
		grant Grant
		want  map[string]verdict
	}{
		{
			name: "project reads on one project",
			grant: Grant{Scopes: []Scope{
				scope(AccessSelectedMemberships, NamespaceProject, "project_read", "vulnerability_read"),
			}},
			want: map[string]verdict{
				"project.get":                   {true, true},
				"merge_request.approve":         {false, false},
				"merge_request.approve_or_read": {true, true},
				"issue_link.list":               {false, true},
				"project.mixed":                 {false, true},
				"skip.action":                   {true, true},
				"user.get":                      {false, false},
				"vulnerability.get":             {true, true},
				"z.denied":                      {false, false},
			},
		},
		{
			name:  "a group scope reaches its group and its projects",
			grant: Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceGroup, "mr_approve", "issue_link_read")}},
			want: map[string]verdict{
				"merge_request.approve":         {true, true},
				"merge_request.approve_or_read": {true, true},
				"issue_link.list":               {true, true},
				"project.get":                   {false, false},
			},
		},
		{
			name: "one request granted and the other public",
			grant: Grant{Scopes: []Scope{
				scope(AccessSelectedMemberships, NamespaceProject, "mr_approve"),
			}},
			want: map[string]verdict{
				"merge_request.approve":         {true, true},
				"merge_request.approve_or_read": {false, true},
			},
		},
		{
			name:  "the user's own scope reaches the user only",
			grant: Grant{Scopes: []Scope{scope(AccessUser, NamespaceNone, "user_read", "project_read")}},
			want:  map[string]verdict{"user.get": {true, true}, "project.get": {false, false}},
		},
		{
			name:  "a name the table does not define grants nothing",
			grant: Grant{Scopes: []Scope{scope(AccessAllMemberships, NamespaceNone, "renamed_since")}},
			want:  map[string]verdict{"project.get": {false, false}, "skip.action": {true, true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authority := Evaluate(phaseBTable(), tc.grant)
			if authority.Phase() != PhaseGranted {
				t.Fatalf("phase = %v, want phase B", authority.Phase())
			}
			for id, want := range tc.want {
				got := authority.Decide(id)
				if got.Listed != want.listed || got.Callable != want.callable {
					t.Errorf("%s: listed %v callable %v, want %v %v", id, got.Listed, got.Callable, want.listed, want.callable)
				}
			}
		})
	}
}

// TestEvaluate_NoPublicSetRecorded_LetsGitLabJudgeARESTCall verifies that,
// while the table carries no evaluated public set, a REST group held at a
// project or a group passes the call guard whatever the grant, while one held
// at the user does not, and neither a GraphQL spine position nor a GraphQL
// query's or mutation's own group is ever let through on the public question.
func TestEvaluate_NoPublicSetRecorded_LetsGitLabJudgeARESTCall(t *testing.T) {
	table := phaseBTable()
	table.PublicKnown = false
	table.PublicAnonymous = [2][]uint64{}
	table.Operations = append(table.Operations,
		Operation{Name: "query approvals (approvalsQuery)", Groups: []uint32{1}},
		Operation{Name: "mutation approve (mergeRequestApprove)", Groups: []uint32{1}})
	// Appended after z.denied, in the order the lookup's binary search needs.
	table.Actions = append(table.Actions,
		Requirement{ID: "zz.mutation_group", Paths: [][]uint32{{8}}, GraphQL: true},
		Requirement{ID: "zz.query_group", Paths: [][]uint32{{7}}, GraphQL: true})
	authority := Evaluate(table, Grant{})
	for id, callable := range map[string]bool{
		"merge_request.approve": true,
		"issue_link.list":       true,
		"user.get":              false,
		"vulnerability.get":     false,
		"zz.query_group":        false,
		"zz.mutation_group":     false,
	} {
		t.Run(id, func(t *testing.T) {
			got := authority.Decide(id)
			if got.Listed || got.Callable != callable {
				t.Errorf("listed %v callable %v, want unlisted and callable %v", got.Listed, got.Callable, callable)
			}
		})
	}
}

// TestAuthority_Decide_PhaseB_NamesWhatIsMissingAndWhatIsServedEmpty verifies
// a decision on an action the grant does not reach names the groups it fails
// on the path it comes closest to passing, and a decision on a served action
// lists the positions served empty: the row's undeclared ones, then each
// declared one off the spine the grant does not cover, each once.
func TestAuthority_Decide_PhaseB_NamesWhatIsMissingAndWhatIsServedEmpty(t *testing.T) {
	table := phaseBTable()
	authority := Evaluate(table, Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceProject, "vulnerability_read")}})

	approve := authority.Decide("merge_request.approve")
	if approve.Cause != CauseNotGranted || !slices.Equal(approve.Missing, []uint32{1}) || len(approve.Degraded) != 0 {
		t.Errorf("Decide(merge_request.approve) = %+v, want not granted, missing group 1, nothing degraded", approve)
	}
	either := authority.Decide("merge_request.approve_or_read")
	if !slices.Equal(either.Missing, []uint32{0}) {
		t.Errorf("Decide(approve_or_read).Missing = %v, want the read's one group, the closer of the two ways", either.Missing)
	}
	vulnerability := authority.Decide("vulnerability.get")
	if !vulnerability.Listed || !slices.Equal(vulnerability.Degraded, []uint32{2, 1}) || vulnerability.Missing != nil {
		t.Errorf("Decide(vulnerability.get) = %+v, want listed with positions 2 then 1 served empty", vulnerability)
	}
	if table.Actions[7].Degraded[0] != 2 || len(table.Actions[7].Degraded) != 1 {
		t.Errorf("the row's own degraded list was written through: %v", table.Actions[7].Degraded)
	}
	granted := Evaluate(table, Grant{Scopes: []Scope{
		scope(AccessSelectedMemberships, NamespaceProject, "vulnerability_read"),
		scope(AccessUser, NamespaceNone, "user_read"),
	}})
	if got := granted.Decide("vulnerability.get").Degraded; !slices.Equal(got, []uint32{2}) {
		t.Errorf("with User: Read granted, degraded = %v, want the undeclared position only", got)
	}
	reader := Evaluate(table, Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceProject, "project_read")}})
	mixed := reader.Decide("project.mixed")
	if mixed.Listed || !mixed.Callable || mixed.Cause != CauseNotGranted || !slices.Equal(mixed.Missing, []uint32{4}) || mixed.Degraded != nil {
		t.Errorf("Decide(project.mixed) = %+v, want unlisted, callable, not granted, missing group 4, nothing degraded", mixed)
	}
	skipped := reader.Decide("skip.action")
	if !skipped.Listed || skipped.Missing != nil {
		t.Errorf("Decide(skip.action) = %+v, want listed with nothing missing", skipped)
	}
}

// TestAuthority_Decide_PhaseB_MissingIsTheFirstClosestPathsUncoveredGroups
// verifies the groups a refusal names: of two ways failing as many groups
// each, the first; and of one operation needing a covered group, an uncovered
// one and that uncovered one again, the uncovered group once, the covered one
// never.
func TestAuthority_Decide_PhaseB_MissingIsTheFirstClosestPathsUncoveredGroups(t *testing.T) {
	table := phaseBTable()
	table.Operations = append(table.Operations, Operation{Name: "GET /projects/:id/two", Groups: []uint32{0, 1, 1}})
	table.Actions = []Requirement{
		{ID: "x.tie", Paths: [][]uint32{{1}, {2}}},
		{ID: "x.two", Paths: [][]uint32{{7}}},
	}
	authority := Evaluate(table, Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceProject, "project_read")}})
	if got := authority.Decide("x.tie").Missing; !slices.Equal(got, []uint32{1}) {
		t.Errorf("Decide(x.tie).Missing = %v, want the first way's group 1 on a tie", got)
	}
	if got := authority.Decide("x.two").Missing; !slices.Equal(got, []uint32{1}) {
		t.Errorf("Decide(x.two).Missing = %v, want the uncovered group 1 once, and not the covered group 0", got)
	}
}

// TestAuthority_FailedGroups_SkipAnOptedOutOperation verifies the groups a
// refusal names leave out an operation that opts out of the check, since the
// grant does not decide it, while naming the rest of the path's.
func TestAuthority_FailedGroups_SkipAnOptedOutOperation(t *testing.T) {
	authority := Evaluate(phaseBTable(), Grant{})
	if got := authority.failedGroups([]uint32{6, 3}); !slices.Equal(got, []uint32{3}) {
		t.Errorf("failedGroups = %v, want only the user read's group", got)
	}
}

// TestEvaluate_AnUndeclaredSpinePosition_IsReachedByNoGrant verifies a way of
// running whose spine holds a position that declares nothing passes for no
// grant, even one covering every group around it: the row would be denied by
// the generator, and the evaluation agrees on its own.
func TestEvaluate_AnUndeclaredSpinePosition_IsReachedByNoGrant(t *testing.T) {
	table := phaseBTable()
	table.Operations = append(table.Operations, Operation{Name: "query undeclared (q)", Spine: []uint32{2}})
	table.Actions = []Requirement{{ID: "x.undeclared", Paths: [][]uint32{{7}}}}
	authority := Evaluate(table, Grant{Scopes: []Scope{scope(AccessAllMemberships, NamespaceNone, "project_read", "vulnerability_read")}})
	if got := authority.Decide("x.undeclared"); got.Listed || got.Callable {
		t.Errorf("Decide = %+v, want neither listed nor callable", got)
	}
}

// TestEvaluate_APublicGroupBoundary_IsJudgedOnTheGroupsOwnSet verifies the call
// guard reads a group held at a group boundary against the public group set,
// not the project one: a permission public on a project is not public on a
// group, so a group-only requirement on it is not callable.
func TestEvaluate_APublicGroupBoundary_IsJudgedOnTheGroupsOwnSet(t *testing.T) {
	table := phaseBTable()
	table.Groups = append(table.Groups, Group{Perms: []uint16{2}, Any: BoundaryGroup})
	table.Operations = append(table.Operations, Operation{Name: "GET /groups/:id/links", Groups: []uint32{6}})
	table.Actions = []Requirement{{ID: "group.links", Paths: [][]uint32{{7}}}}
	if got := Evaluate(table, Grant{}).Decide("group.links"); got.Callable {
		t.Errorf("Decide = %+v, want not callable: the permission is public on a project only", got)
	}
	table.PublicAnonymous[PublicGroup] = []uint64{1 << 2}
	if got := Evaluate(table, Grant{}).Decide("group.links"); !got.Callable || got.Listed {
		t.Errorf("Decide = %+v, want callable and unlisted once the group set holds it", got)
	}
}

// TestAuthority_Lists_AnswersWithoutTheWords verifies the listing's shortcut
// in both phases: an action with no row is listed, a denied one is not, every
// other one is in phase A, and in phase B what the grant reaches is.
func TestAuthority_Lists_AnswersWithoutTheWords(t *testing.T) {
	phaseA := Unevaluated(phaseBTable(), FallbackNone, "")
	phaseB := Evaluate(phaseBTable(), Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceProject, "project_read")}})
	for _, tc := range []struct {
		name      string
		authority *Authority
		id        string
		want      bool
	}{
		{"no row", phaseB, "not.recorded_lists", true},
		{"denied", phaseB, "z.denied", false},
		{"phase A serves the rest", phaseA, "merge_request.approve", true},
		{"phase B lists what the grant reaches", phaseB, "project.get", true},
		{"phase B leaves out the rest", phaseB, "merge_request.approve", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.authority.Lists(tc.id); got != tc.want || got != tc.authority.Decide(tc.id).Listed {
				t.Errorf("Lists(%q) = %v, want %v and Decide's Listed", tc.id, got, tc.want)
			}
		})
	}
}

// TestAuthority_Decide_AnActionWithNoRow_IsServedAndLoggedOnce verifies an
// action the table has no row for is unknown authority, served in either
// phase, and that it is logged once per process at debug, however often it is
// asked about.
func TestAuthority_Decide_AnActionWithNoRow_IsServedAndLoggedOnce(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	// Unique per run, since the set of IDs already logged lives as long as the
	// process and a test run twice in one would find it logged.
	id := "never.recorded_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	// sequential: the second authority must find the ID the first one logged, which is the once this test counts
	for _, authority := range []*Authority{Evaluate(phaseBTable(), Grant{}), Unevaluated(phaseBTable(), FallbackNone, "")} {
		if got := authority.Decide(id); !got.Listed || !got.Callable || got.Known {
			t.Errorf("Decide(%q) = %+v, want unknown authority, served", id, got)
		}
	}
	if count := strings.Count(logged.String(), id); count != 1 {
		t.Errorf("the action was logged %d times, want once:\n%s", count, logged.String())
	}
}

// TestJudge_EachGuardFallsBackWithItsReason verifies the order the guards are
// applied in and the authority each one returns: the grant's own reason first,
// then a version not read, then a version outside the record, then a grant
// naming permissions the table does not define, with how many and the first
// three, each cut, and phase B when every guard holds.
func TestJudge_EachGuardFallsBackWithItsReason(t *testing.T) {
	long := strings.Repeat("n", 80)
	unknown := Grant{Scopes: []Scope{
		scope(AccessAllMemberships, NamespaceNone, "project_read", "gone_a", long),
		scope(AccessUser, NamespaceNone, "gone_a", "gone_c", "gone_d"),
	}}
	cases := []struct {
		name     string
		reading  Reading
		phase    Phase
		fallback FallbackReason
		reported string
	}{
		{"the grant's own reason", Reading{Fallback: FallbackGrantTooLarge, Version: "19.4.1-ee"}, PhaseUnknown, FallbackGrantTooLarge, "19.4.1-ee"},
		{"the version read's reason", Reading{Fallback: FallbackVersionUnanswered}, PhaseUnknown, FallbackVersionUnanswered, ""},
		{"no version", Reading{}, PhaseUnknown, FallbackVersionUnreadable, ""},
		{"another release", Reading{Version: "19.3.2-ee"}, PhaseUnknown, FallbackVersionOutside, "19.3.2-ee"},
		{"a release two minors on", Reading{Version: "19.6.0-pre"}, PhaseUnknown, FallbackVersionOutside, "19.6.0-pre"},
		{"an unknown permission", Reading{Grant: unknown, Version: "19.4.0"}, PhaseUnknown, FallbackUnknownPermission, "19.4.0"},
		{"every guard holds", Reading{Grant: Grant{}, Version: "19.4.3-ee"}, PhaseGranted, FallbackNone, "19.4.3-ee"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Judge(phaseBTable(), tc.reading)
			if got.Phase() != tc.phase || got.Fallback() != tc.fallback || got.Reported() != tc.reported || got.ListingOnly() {
				t.Errorf("Judge = phase %v, fallback %q, reported %q, listing only %v; want %v, %q, %q, false",
					got.Phase(), got.Fallback(), got.Reported(), got.ListingOnly(), tc.phase, tc.fallback, tc.reported)
			}
		})
	}
	count, sample := Judge(phaseBTable(), Reading{Grant: unknown, Version: "19.4.0"}).UnknownPermissions()
	if count != 4 || !slices.Equal(sample, []string{"gone_a", long[:64], "gone_c"}) {
		t.Errorf("UnknownPermissions() = %d, %q; want 4 and the first three, each cut to 64 bytes", count, sample)
	}
}

// TestJudge_ThePrereleasePastTheRecord_ListsByTheGrantAndCallsByPhaseA verifies
// the one release past the table it lets in: the listing follows the grant as
// the table declares it, every action phase A allows is callable, a denied one
// stays withheld, and an action left out of the listing is worded as one a
// call still reaches.
func TestJudge_ThePrereleasePastTheRecord_ListsByTheGrantAndCallsByPhaseA(t *testing.T) {
	authority := Judge(phaseBTable(), Reading{
		Grant:   Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceProject, "project_read")}},
		Version: "19.5.0-pre",
	})
	if !authority.ListingOnly() || authority.Phase() != PhaseGranted || authority.Reported() != "19.5.0-pre" {
		t.Fatalf("Judge = phase %v, listing only %v, reported %q", authority.Phase(), authority.ListingOnly(), authority.Reported())
	}
	project, approve, denied := authority.Decide("project.get"), authority.Decide("merge_request.approve"), authority.Decide("z.denied")
	if !project.Listed || !project.Callable || approve.Listed || !approve.Callable || denied.Callable {
		t.Errorf("project %+v, approve %+v, denied %+v", project, approve, denied)
	}
	text := authority.WithheldText("merge_request.approve", approve)
	if !strings.Contains(text, " The instance reports GitLab 19.5.0-pre, a prerelease past the release the permissions are recorded for, "+
		"so the listing follows 19.4.1 and a call is still passed to GitLab, which judges it. Do not") {
		t.Errorf("WithheldText =\n%s", text)
	}
}

// TestNextPrerelease_IsOnlyTheReleaseRightAfterTheBucket verifies which
// reported versions count as the prerelease right after a recorded
// major.minor: the next minor, or after the last minor of a major the next
// major's first, and nothing else.
func TestNextPrerelease_IsOnlyTheReleaseRightAfterTheBucket(t *testing.T) {
	for _, tc := range []struct {
		version, bucket string
		want            bool
	}{
		{"19.5.0-pre", "19.4", true},
		{"19.5.0", "19.4", false},
		{"19.6.0-pre", "19.4", false},
		{"20.0.0-pre", "19.4", false},
		{"19.4.0-pre", "19.4", false},
		{"19.5.0-pre", "19.x", false},
		{"19.1.0-pre", "19.x", false},
		{"20.5.0-pre", "x.4", false},
		{"0.5.0-pre", "x.4", false},
		{"19.11.0-pre", "19.10", true},
		{"20.0.0-pre", "19.10", false},
		{"20.0.0-pre", "19.11", true},
		{"19.12.0-pre", "19.11", false},
		{"18.0.0-pre", "19.11", false},
		{"14.0.0-pre", "13.12", true},
	} {
		t.Run(tc.version+" after "+tc.bucket, func(t *testing.T) {
			if got := nextPrerelease(tc.version, tc.bucket); got != tc.want {
				t.Errorf("nextPrerelease = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRejudge_ReplacesOnlyOnReadsThatAnswered verifies a re-read replaces the
// authority only with what reads that answered decide: a version not read
// keeps it, a grant read that failed at the same major.minor keeps it, a
// failed grant read at another major.minor moves it to that release's phase A
// verdict, and a grant read with a version replaces it, as it does when
// nothing was held.
func TestRejudge_ReplacesOnlyOnReadsThatAnswered(t *testing.T) {
	table := phaseBTable()
	current := Judge(table, Reading{Grant: Grant{}, Version: "19.4.1-ee"})
	read := Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceGroup, "project_read")}}
	cases := []struct {
		name     string
		current  *Authority
		reading  Reading
		replaced bool
		fallback FallbackReason
	}{
		{"no version", current, Reading{Grant: read}, false, FallbackNone},
		{"grant unanswered at the same release", current, Reading{Fallback: FallbackGrantUnanswered, Version: "19.4.2"}, false, FallbackNone},
		{"grant too large at the same release", current, Reading{Fallback: FallbackGrantTooLarge, Version: "19.4.1-ee"}, false, FallbackNone},
		{"grant unanswered after an upgrade", current, Reading{Fallback: FallbackGrantUnanswered, Version: "19.6.0"}, true, FallbackGrantUnanswered},
		{"grant read", current, Reading{Grant: read, Version: "19.4.1-ee"}, true, FallbackNone},
		{"nothing held", nil, Reading{Fallback: FallbackGrantShape, Version: "19.4.1-ee"}, true, FallbackGrantShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, replaced := Rejudge(table, tc.current, tc.reading)
			if replaced != tc.replaced {
				t.Fatalf("replaced = %v, want %v", replaced, tc.replaced)
			}
			if !replaced && got != tc.current {
				t.Errorf("Rejudge kept a different authority than it was given")
			}
			if replaced && (got == tc.current || got.Fallback() != tc.fallback) {
				t.Errorf("Rejudge = %+v, want a new authority with fallback %q", got, tc.fallback)
			}
		})
	}
}

// TestMoved_IsAChangeOfWhatALogLineStates verifies which replacements count
// as a move: one from nothing, and a change of phase, of the reason, or of
// the major.minor judged at; a replacement that states what the one before it
// did, a patch release or the same verdict read again, is not.
func TestMoved_IsAChangeOfWhatALogLineStates(t *testing.T) {
	table := phaseBTable()
	recorded := Judge(table, Reading{Grant: Grant{}, Version: "19.4.1-ee"})
	outside := Judge(table, Reading{Grant: Grant{}, Version: "19.6.0-ee"})
	cases := []struct {
		name     string
		from, to *Authority
		want     bool
	}{
		{"from nothing", nil, recorded, true},
		{"a patch release", recorded, Judge(table, Reading{Grant: Grant{}, Version: "19.4.2"}), false},
		{"the same verdict read again", outside, Judge(table, Reading{Grant: Grant{}, Version: "19.6.0-ee"}), false},
		{"an upgrade past the record", recorded, outside, true},
		{"back to the recorded release", outside, recorded, true},
		{"another upgrade outside the record", outside, Judge(table, Reading{Grant: Grant{}, Version: "19.7.0-ee"}), true},
		{"onto the listing-only prerelease", recorded, Judge(table, Reading{Grant: Grant{}, Version: "19.5.0-pre"}), true},
		{"another reason at the same release", outside, Unevaluated(table, FallbackGrantShape, "19.6.0-ee"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Moved(tc.from, tc.to); got != tc.want {
				t.Errorf("Moved = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAuthority_LogArgs_NamesThePhaseAndNothingOfTheGrant verifies the pairs a
// log line about an authority carries, and that the unknown permissions are
// named only for a grant that held some.
func TestAuthority_LogArgs_NamesThePhaseAndNothingOfTheGrant(t *testing.T) {
	plain := Judge(phaseBTable(), Reading{Grant: Grant{}, Version: "19.4.1-ee"}).LogArgs()
	if want := []any{"phase", "B", "reason", "", "bucket", "19.4", "recorded_bucket", "19.4"}; !slices.Equal(plain, want) {
		t.Errorf("LogArgs() = %v, want %v", plain, want)
	}
	unknown := Judge(phaseBTable(), Reading{
		Grant:   Grant{Scopes: []Scope{scope(AccessUser, NamespaceNone, "gone")}},
		Version: "19.4.1-ee",
	}).LogArgs()
	if len(unknown) != 12 || unknown[1] != "A" || unknown[3] != string(FallbackUnknownPermission) ||
		unknown[8] != "unknown_permissions" || unknown[9] != 1 {
		t.Errorf("LogArgs() = %v, want phase A, the reason and one unknown permission", unknown)
	}
}

// TestAuthority_WithheldText_PhaseB_ListedHasNoWordsAndAPassedCallSaysSo
// verifies the phase B words: none for a listed action, the refusal for one
// the call guard refuses, and for one left out of the listing whose call
// still passes, the refusal followed by the sentence that says a call is
// handed to GitLab.
func TestAuthority_WithheldText_PhaseB_ListedHasNoWordsAndAPassedCallSaysSo(t *testing.T) {
	authority := Evaluate(phaseBTable(), Grant{Scopes: []Scope{scope(AccessSelectedMemberships, NamespaceProject, "project_read")}})
	if got := authority.WithheldText("project.get", authority.Decide("project.get")); got != "" {
		t.Errorf("listed action told %q", got)
	}
	refused := authority.WithheldText("merge_request.approve", authority.Decide("merge_request.approve"))
	if !strings.Contains(refused, "the project permission [Merge Request: Approve], as GitLab 19.4.1 declares it.") ||
		strings.Contains(refused, "still passed") {
		t.Errorf("refused call told\n%s", refused)
	}
	passed := authority.WithheldText("issue_link.list", authority.Decide("issue_link.list"))
	if !strings.Contains(passed, "where the group does not refuse classic tokens. A call is still passed to GitLab, "+
		"which serves it on a public project or group where these permissions are public. Do not report") {
		t.Errorf("passed call told\n%s", passed)
	}
}

// TestPhase_String_NamesThePhase verifies the phases' names in a log line.
func TestPhase_String_NamesThePhase(t *testing.T) {
	if PhaseUnknown.String() != "A" || PhaseGranted.String() != "B" {
		t.Errorf("phases read %q and %q", PhaseUnknown.String(), PhaseGranted.String())
	}
}

// TestBitset_HasAnIndexPastItsEnd_IsFalse verifies a set answers no for an
// index beyond its words rather than reading past them, which is what lets a
// public set shorter than the permission list stand for its missing bits.
func TestBitset_HasAnIndexPastItsEnd_IsFalse(t *testing.T) {
	set := newBitset(3)
	set.set(2)
	if !set.has(2) || set.has(1) || set.has(64) || bitset(nil).has(0) {
		t.Errorf("set = %v", set)
	}
}

// fuzzBytes hands a fuzz input out as small choices, answering 0 once it runs
// out, so every input builds some table and some grant.
type fuzzBytes struct {
	data []byte
	at   int
}

// pick returns a choice below n.
func (f *fuzzBytes) pick(n int) int {
	if n <= 1 || f.at >= len(f.data) {
		return 0
	}
	b := f.data[f.at]
	f.at++
	return int(b) % n
}

// index picks a table index below n, the length of one of a fuzz table's
// slices.
func (f *fuzzBytes) index(n int) uint32 {
	return uint32(f.pick(n)) //#nosec G115 -- pick answers in [0, n), and n is the length of a fuzz table's slice, a handful of entries
}

// perm picks a raw permission index below n, the fuzz table's permission
// count.
func (f *fuzzBytes) perm(n int) uint16 {
	return uint16(f.pick(n)) //#nosec G115 -- pick answers in [0, n), and n is at most six permissions
}

// boundaries picks a non-empty set of the four boundary types.
func (f *fuzzBytes) boundaries() Boundary {
	return Boundary(1 + f.pick(15)) //#nosec G115 -- pick(15) answers in [0, 15), so the set is in [1, 15], the four boundary bits
}

// access picks one of the n access kinds a scope can carry, the values
// GitLab sends and one past them.
func (f *fuzzBytes) access(n int) GrantAccess {
	return GrantAccess(f.pick(n)) //#nosec G115 -- pick answers in [0, n), and n is six access kinds
}

// The namespaces the soundness reference places scopes and targets in: the
// creating user's namespace, a root group, a subgroup of it, and two projects'
// namespaces, one in the subgroup and one in the user's namespace. 0 is no
// namespace.
const (
	nsNone = iota
	nsUser
	nsRoot
	nsSub
	nsProjectInSub
	nsProjectInUser
	nsCount
)

// nsKind is the kind of namespace each of the reference's namespaces is.
var nsKind = [nsCount]NamespaceKind{
	nsNone: NamespaceNone, nsUser: NamespaceUser, nsRoot: NamespaceGroup, nsSub: NamespaceGroup,
	nsProjectInSub: NamespaceProject, nsProjectInUser: NamespaceProject,
}

// refTarget is one object a call can reach: its boundary type and, for a
// project or a group, its namespace's self and ancestors.
type refTarget struct {
	kind     Boundary
	ancestry []int
}

// refTargets are every object of the reference's namespace tree: the two
// projects, the two groups, the user and the instance.
var refTargets = []refTarget{
	{BoundaryProject, []int{nsProjectInSub, nsSub, nsRoot}},
	{BoundaryProject, []int{nsProjectInUser, nsUser}},
	{BoundaryGroup, []int{nsRoot}},
	{BoundaryGroup, []int{nsSub, nsRoot}},
	{BoundaryUser, nil},
	{BoundaryInstance, nil},
}

// refScope is one scope as the reference sees it: on a concrete namespace.
type refScope struct {
	access GrantAccess
	ns     int
	perms  []uint16
}

// fuzzTable builds a small table from a fuzz input: permissions with one
// assignable each, groups at random boundaries, GraphQL positions some of
// them undeclared, REST and GraphQL operations, and actions of one or two
// ways, some denied.
func fuzzTable(f *fuzzBytes) *Table {
	perms := 1 + f.pick(6)
	t := &Table{Version: "19.4.1-ee", Bucket: "19.4", PublicKnown: f.pick(2) == 1, PublicAnonymous: [2][]uint64{{0}, {0}}}
	for i := range perms {
		name := "p" + strconv.Itoa(i)
		t.Permissions = append(t.Permissions, name)
		t.Display = append(t.Display, name)
		t.RefusalDisplay = append(t.RefusalDisplay, name)
		t.Assignables = append(t.Assignables, Assignable{Name: name, Permissions: []uint16{uint16(i)}, Boundaries: AllBoundaries, Grantable: true})
		for p := range t.PublicAnonymous {
			if f.pick(3) == 0 {
				t.PublicAnonymous[p][0] |= 1 << i
			}
		}
	}
	for range 1 + f.pick(5) {
		group := Group{Perms: []uint16{f.perm(perms)}, Any: f.boundaries()}
		if f.pick(2) == 1 {
			group.Perms = append(group.Perms, f.perm(perms))
		}
		t.Groups = append(t.Groups, group)
	}
	for range f.pick(4) {
		t.Elements = append(t.Elements, Element{Path: "x", Type: "T", Groups: []uint32{f.index(len(t.Groups))}, Undeclared: f.pick(5) == 0})
	}
	for range 1 + f.pick(4) {
		t.Operations = append(t.Operations, fuzzOperation(f, t))
	}
	for k := range 1 + f.pick(4) {
		row := Requirement{ID: "a" + strconv.Itoa(k)}
		if f.pick(6) == 0 {
			row.Denied = &Denial{Cause: CauseTypeUndeclared, Element: "T", Effect: EffectNull}
		} else {
			for range 1 + f.pick(2) {
				path := []uint32{f.index(len(t.Operations))}
				if f.pick(2) == 1 {
					path = append(path, f.index(len(t.Operations)))
				}
				row.Paths = append(row.Paths, path)
			}
		}
		t.Actions = append(t.Actions, row)
	}
	return t
}

// fuzzOperation builds one operation: a REST route with one or two groups,
// sometimes opting out of the check, or a GraphQL one with a group of its own
// and positions on and off its spine.
func fuzzOperation(f *fuzzBytes, t *Table) Operation {
	if len(t.Elements) == 0 || f.pick(2) == 0 {
		op := Operation{Name: "GET /x", Groups: []uint32{f.index(len(t.Groups))}, Skip: f.pick(8) == 0}
		if f.pick(2) == 1 {
			op.Groups = append(op.Groups, f.index(len(t.Groups)))
		}
		return op
	}
	op := Operation{Name: "query x (q)"}
	if f.pick(2) == 1 {
		op.Groups = []uint32{f.index(len(t.Groups))}
	}
	for range f.pick(3) {
		op.Spine = append(op.Spine, f.index(len(t.Elements)))
	}
	for range f.pick(2) {
		op.OffSpine = append(op.OffSpine, f.index(len(t.Elements)))
	}
	return op
}

// fuzzGrant builds up to three scopes on concrete namespaces, the reference's
// view of them, and the grant [Evaluate] reads.
func fuzzGrant(f *fuzzBytes, t *Table) ([]refScope, Grant) {
	var scopes []refScope
	grant := Grant{Scopes: []Scope{}}
	for range f.pick(4) {
		s := refScope{access: f.access(6), ns: f.pick(nsCount)}
		var names []string
		for i := range t.Permissions {
			if f.pick(2) == 1 {
				s.perms = append(s.perms, uint16(i))
				names = append(names, t.Permissions[i])
			}
		}
		scopes = append(scopes, s)
		grant.Scopes = append(grant.Scopes, Scope{Access: s.access, Namespace: nsKind[s.ns], Permissions: names})
	}
	return scopes, grant
}

// refApplies is GitLab's applicability of one scope to one object
// (app/models/authz/granular_scope.rb, applicable_to_boundary?): a user or an
// instance object takes a scope of its own access with no namespace; a project
// or a group takes a selected memberships or personal projects scope whose
// namespace is among its namespace's self and ancestors, and an all
// memberships scope with no namespace, the token's user being taken to be a
// member.
func refApplies(s refScope, target refTarget) bool {
	switch target.kind {
	case BoundaryUser:
		return s.access == AccessUser && s.ns == nsNone
	case BoundaryInstance:
		return s.access == AccessInstance && s.ns == nsNone
	default:
		if (s.access == AccessSelectedMemberships || s.access == AccessPersonalProjects) && s.ns != nsNone &&
			slices.Contains(target.ancestry, s.ns) {
			return true
		}
		return s.access == AccessAllMemberships && s.ns == nsNone
	}
}

// refGroupPasses is GitLab's verdict on one group for some object of the
// tree: one of a type the group may be held at, at which every permission is
// granted by a scope that applies to it.
func refGroupPasses(t *Table, scopes []refScope, index uint32) bool {
	group := t.Groups[index]
	for _, target := range refTargets {
		if group.Any&target.kind == 0 {
			continue
		}
		granted := func(perm uint16) bool {
			return slices.ContainsFunc(scopes, func(s refScope) bool { return refApplies(s, target) && slices.Contains(s.perms, perm) })
		}
		if !slices.ContainsFunc(group.Perms, func(perm uint16) bool { return !granted(perm) }) {
			return true
		}
	}
	return false
}

// refAllows reports whether GitLab would serve some way of running a row for
// some object of the tree: every operation of a path opts out or has every
// group, its own and its spine positions', granted at some object.
func refAllows(t *Table, scopes []refScope, row *Requirement) bool {
	passes := func(group uint32) bool { return refGroupPasses(t, scopes, group) }
	for _, path := range row.Paths {
		ok := true
		for _, index := range path {
			op := t.Operations[index]
			if op.Skip {
				continue
			}
			for _, element := range op.Spine {
				ok = ok && !t.Elements[element].Undeclared && !slices.ContainsFunc(t.Elements[element].Groups, func(g uint32) bool { return !passes(g) })
			}
			ok = ok && !slices.ContainsFunc(op.Groups, func(g uint32) bool { return !passes(g) })
		}
		if ok {
			return true
		}
	}
	return false
}

// FuzzEvaluate_IsSoundAgainstGitLab holds the existential reading to GitLab's
// own rule: whatever a reference evaluator applying GitLab's applicability
// table object by object, over a namespace tree of two groups, two projects,
// the user and the instance, lets a grant run, [Evaluate] lists, REST routes
// and GraphQL spine positions alike, and a denied action is never listed nor
// callable. The reverse is not claimed: before a call the target is unknown,
// and listing what some target allows is the over-approximation the design
// accepts.
func FuzzEvaluate_IsSoundAgainstGitLab(f *testing.F) {
	f.Add([]byte{3, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})
	f.Add([]byte{5, 0, 14, 1, 2, 2, 1, 1, 3, 0, 1, 2, 2, 4, 1, 1, 0, 1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		input := &fuzzBytes{data: data}
		table := fuzzTable(input)
		scopes, grant := fuzzGrant(input, table)
		authority := Evaluate(table, grant)
		for i := range table.Actions {
			row := &table.Actions[i]
			decision := authority.Decide(row.ID)
			if row.Denied != nil {
				if decision.Listed || decision.Callable {
					t.Fatalf("%s is denied and decided %+v", row.ID, decision)
				}
				continue
			}
			if refAllows(table, scopes, row) && !decision.Listed {
				t.Fatalf("GitLab would serve %s to the grant %+v and it is not listed; table %+v", row.ID, scopes, table)
			}
			if decision.Listed && !decision.Callable {
				t.Fatalf("%s is listed and not callable", row.ID)
			}
		}
	})
}

// FuzzEvaluate_IsMonotone holds that granting more never takes anything away:
// adding a permission to one scope, or a scope to the grant, leaves every
// action that was listed listed and every one that was callable callable.
func FuzzEvaluate_IsMonotone(f *testing.F) {
	f.Add([]byte{2, 1, 0, 3, 5, 7, 9, 2, 4, 6, 8, 1, 3}, uint8(0), uint8(1), uint8(2))
	f.Add([]byte{6, 1, 15, 1, 2, 2, 0, 1, 1, 3, 2, 1, 0, 2}, uint8(3), uint8(4), uint8(9))
	f.Fuzz(func(t *testing.T, data []byte, which, access, ns uint8) {
		input := &fuzzBytes{data: data}
		table := fuzzTable(input)
		_, grant := fuzzGrant(input, table)
		before := Evaluate(table, grant)

		wider := Grant{Scopes: slices.Clone(grant.Scopes)}
		name := table.Permissions[int(which)%len(table.Permissions)]
		if len(wider.Scopes) > 0 && access%2 == 0 {
			at := int(ns) % len(wider.Scopes)
			wider.Scopes[at].Permissions = append(slices.Clone(wider.Scopes[at].Permissions), name)
		} else {
			wider.Scopes = append(wider.Scopes, Scope{
				Access: GrantAccess(access % 6), Namespace: nsKind[int(ns)%nsCount], Permissions: []string{name},
			})
		}
		after := Evaluate(table, wider)
		for _, row := range table.Actions {
			was, is := before.Decide(row.ID), after.Decide(row.ID)
			if (was.Listed && !is.Listed) || (was.Callable && !is.Callable) {
				t.Fatalf("%s was %+v and is %+v after granting %s more", row.ID, was, is, name)
			}
		}
	})
}
