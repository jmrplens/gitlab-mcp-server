package grants

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// dispatch is one dispatch line of a recorded run.
func dispatch(action string, requests int, refusal string) e2ecalls.Record {
	return e2ecalls.Record{Type: e2ecalls.TypeDispatch, Dispatch: &e2ecalls.Dispatch{Action: action, Requests: requests, RefusalReason: refusal}}
}

// routed is one dispatch line naming the routes its trace reached.
func routed(action string, routes ...string) e2ecalls.Record {
	return e2ecalls.Record{Type: e2ecalls.TypeDispatch, Dispatch: &e2ecalls.Dispatch{Action: action, Requests: len(routes), Routes: routes}}
}

// useE2ECalls points the shard reader at records or a failure for one test.
func useE2ECalls(t *testing.T, records []e2ecalls.Record, err error) {
	t.Helper()
	original := readE2ECalls
	t.Cleanup(func() { readE2ECalls = original })
	readE2ECalls = func(string) ([]e2ecalls.Record, error) { return records, err }
}

// TestE2ECheck_NoDirectory_RunsNothing verifies a run that named no shard
// directory reports the check as not run rather than as nothing compared.
func TestE2ECheck_NoDirectory_RunsNothing(t *testing.T) {
	if got := e2eCheck("", fixtureRecord()); !reflect.DeepEqual(got, E2ECheck{}) {
		t.Errorf("e2eCheck(\"\") = %+v, want nothing", got)
	}
}

// TestE2ECheck_AnUnreadableRecord_IsANoteNotAFailure verifies a directory that
// cannot be read is reported with why, and nothing is compared.
func TestE2ECheck_AnUnreadableRecord_IsANoteNotAFailure(t *testing.T) {
	useE2ECalls(t, nil, errors.New("no such directory"))
	got := e2eCheck("shards", fixtureRecord())
	want := E2ECheck{Ran: true, Directory: "shards", Grain: e2eGrain, Error: "read the end-to-end call record: no such directory"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("e2eCheck = %+v, want %+v", got, want)
	}
}

// TestE2ECheck_EachActionsBusiestTrace_IsHeldToItsShortestPath verifies the
// per-action comparison: an action whose busiest trace carried at least the
// fewest requests any of its paths makes is consistent, retries and a lower
// second trace included; one carrying fewer is a lead; one the derivation
// says sends nothing seen sending is a lead of the other kind, while one seen
// sending nothing is consistent, and so is one the record holds no path for;
// an optional request is on no path and is not counted, so an action with two
// mandatory requests and an optional one is a lead at one request and
// consistent at two; an action whose every request is an alternative is held
// to its shortest path rather than to none, so a trace that sent nothing is a
// lead and one that sent one request is not; a declined dispatch, a line of
// another type, a line naming no action and a line with no dispatch are
// passed over; and an id the record does not hold is named once.
func TestE2ECheck_EachActionsBusiestTrace_IsHeldToItsShortestPath(t *testing.T) {
	useE2ECalls(t, []e2ecalls.Record{
		dispatch("issue.update", 3, ""),
		dispatch("issue.update", 1, ""),
		dispatch("two.step", 1, ""),
		dispatch("two.exact", 2, ""),
		dispatch("either.way", 0, ""),
		dispatch("either.ok", 1, ""),
		dispatch("issue.get", 0, ""),
		dispatch("issue.get", 0, "safe_mode"),
		dispatch("topic.list", 1, ""),
		dispatch("later.get", 0, ""),
		dispatch("later.get", 1, ""),
		dispatch("namespace.list", 9, "confirmation"),
		dispatch("", 2, ""),
		dispatch("gone.action", 1, ""),
		dispatch("gone.action", 1, ""),
		dispatch("quiet.action", 0, ""),
		{Type: e2ecalls.TypeDispatch},
		{Type: e2ecalls.TypeCall},
	}, nil)
	record := fixtureRecord()
	twoMandatory := []actionrequests.RecordRequest{
		rest("GET /a", actionrequests.ClassMandatory),
		rest("GET /b", actionrequests.ClassMandatory),
		rest("GET /c", actionrequests.ClassOptional),
	}
	// Every request of the two below is an alternative, and the shorter of
	// their two ways is the second, so neither has a mandatory request and
	// both are held to one.
	alternatives := []actionrequests.RecordRequest{
		rest("GET /a", actionrequests.ClassAlternative),
		rest("GET /b", actionrequests.ClassAlternative),
		rest("GET /c", actionrequests.ClassAlternative),
	}
	record.Actions = append(record.Actions,
		actionrequests.RecordAction{ID: "quiet.action"},
		actionrequests.RecordAction{ID: "two.step", Requests: twoMandatory, Paths: [][]int{{0, 1}}},
		actionrequests.RecordAction{ID: "two.exact", Requests: twoMandatory, Paths: [][]int{{0, 1}}},
		actionrequests.RecordAction{ID: "either.way", Requests: alternatives, Paths: [][]int{{0, 1}, {2}}},
		actionrequests.RecordAction{ID: "either.ok", Requests: alternatives, Paths: [][]int{{0, 1}, {2}}},
	)
	got := e2eCheck("shards", record)
	want := E2ECheck{
		Ran: true, Directory: "shards", Grain: e2eGrain,
		Compared: 9, Consistent: 5,
		FewerThanAnyPath: []CountLead{
			{Action: "either.way", Fewest: 1, Observed: 0},
			{Action: "issue.get", Fewest: 1, Observed: 0},
			{Action: "two.step", Fewest: 2, Observed: 1},
		},
		SentWhereNoneDerived: []CountLead{{Action: "topic.list", Fewest: 0, Observed: 1}},
		NotInRecord:          []string{"gone.action"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("e2eCheck =\n%+v\nwant\n%+v", got, want)
	}
}

// TestE2ECheck_Routes_EachIsAccountedForOrALead verifies the route half: a
// route the derivation names is accounted for, and so is one a declared
// pattern covers, a slug of the family and a HEAD sent to a route Grape
// declares for GET alike; the GraphQL endpoint is accounted for by any
// GraphQL request and is a lead on an action that sends none; a route the
// derivation does not name is a lead even beside routes it does; a method
// alone, a route the table holds no template for, is a lead of its own kind
// unless a declared pattern of that method placed the action's route; an
// action with an unresolved request accounts for anything; and a line naming
// no route is held to its count alone, outside the routes compared.
func TestE2ECheck_Routes_EachIsAccountedForOrALead(t *testing.T) {
	useE2ECalls(t, []e2ecalls.Record{
		routed("issue.get", "GET /projects/:id/issues", "GET /projects/:id/integrations/slack"),
		routed("issue.get", "GET"),
		routed("issue.update", "GET /projects/:id/issues", "PUT /projects/:id/issues/:issue_iid", "DELETE /projects/:id/issues/:issue_iid"),
		routed("branch.rule_list", graphQLRoute),
		routed("issue.thing", graphQLRoute),
		routed("issue.bulk", "PATCH"),
		routed("namespace.list", "DELETE /anything", "GET"),
		routed("file.raw", "HEAD /projects/:id/repository/files/:file_path/raw"),
		dispatch("branch.protected_list", 1, ""),
	}, nil)
	record := fixtureRecord()
	head := rest("GET /projects/:id/repository/files/:file_path/raw", actionrequests.ClassMandatory)
	head.Derived = "HEAD /projects/:/repository/files/:/raw"
	record.Actions = append(record.Actions, actionrequests.RecordAction{ID: "file.raw", Requests: []actionrequests.RecordRequest{head}, Paths: [][]int{{0}}})

	got := e2eCheck("shards", record)
	want := E2ECheck{
		Ran: true, Directory: "shards", Grain: e2eGrain,
		Compared: 8, Consistent: 5, RoutesCompared: 7,
		RoutesNotDerived: []RouteLead{
			{Action: "issue.thing", Route: graphQLRoute},
			{Action: "issue.update", Route: "DELETE /projects/:id/issues/:issue_iid"},
		},
		RoutesTheTableDoesNotName: []RouteLead{{Action: "issue.bulk", Route: "PATCH"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("e2eCheck =\n%+v\nwant\n%+v", got, want)
	}
}

// TestAccountFor_EachRequestIsAskedOfARouteItDoesNotSend verifies a request
// accounts for no route it does not send: a GraphQL request for a REST route,
// a request placed by a pattern for a method alone of another verb, and for a
// route of its own verb its pattern does not cover.
func TestAccountFor_EachRequestIsAskedOfARouteItDoesNotSend(t *testing.T) {
	placed := rest("GET /projects/:id/integrations/apple-app-store", actionrequests.ClassAlternative)
	placed.Derived = "GET /projects/:/integrations/:"
	cases := []struct {
		name    string
		request actionrequests.RecordRequest
		route   string
		want    routeVerdict
	}{
		{name: "a GraphQL request and a REST route", request: graphQL("query project (x)", actionrequests.ClassMandatory, "project"), route: "GET /projects/:id", want: routeNotDerived},
		{name: "a placed request and another method alone", request: placed, route: "PUT", want: routeUnnamed},
		{name: "a placed request and a route its pattern misses", request: placed, route: "GET /groups/:id/integrations/slack", want: routeNotDerived},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action := actionrequests.RecordAction{ID: "a.b", Requests: []actionrequests.RecordRequest{tc.request}, Paths: [][]int{{0}}}
			if got := accountFor(action, tc.route); got != tc.want {
				t.Errorf("accountFor(%s) = %v, want %v", tc.route, got, tc.want)
			}
		})
	}
}

// TestPatternCovers_TheMethodAndEverySpelledSegment verifies the pattern match
// a declaration's route is read by.
func TestPatternCovers_TheMethodAndEverySpelledSegment(t *testing.T) {
	cases := []struct {
		name, pattern, route string
		want                 bool
	}{
		{name: "a slug", pattern: "PUT /projects/:/integrations/:", route: "PUT /projects/:id/integrations/slack", want: true},
		{name: "another method", pattern: "PUT /projects/:/integrations/:", route: "GET /projects/:id/integrations/slack"},
		{name: "a literal differs", pattern: "PUT /projects/:/integrations/:", route: "PUT /groups/:id/integrations/slack"},
		{name: "a segment more", pattern: "PUT /projects/:/integrations/:", route: "PUT /projects/:id/integrations/slack/test"},
		{name: "a segment fewer", pattern: "PUT /projects/:/integrations/:", route: "PUT /projects/:id/integrations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := patternCovers(tc.pattern, tc.route); got != tc.want {
				t.Errorf("patternCovers(%q, %q) = %t, want %t", tc.pattern, tc.route, got, tc.want)
			}
		})
	}
}
