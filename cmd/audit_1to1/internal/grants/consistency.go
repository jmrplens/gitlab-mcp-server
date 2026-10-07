package grants

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// regenerate is the command every inconsistency is answered by: the three
// artifacts are written together from the record, so one that disagrees with
// it was left behind by a change to it.
const regenerate = "run `make gen-action-grants`"

// inconsistencies holds the committed table and request record to the live
// record they were joined from, and to each other. Each finding is a sentence
// naming what disagrees; none is a judgement of GitLab or of a handler.
//
// The checks are the ones a stale table would fail, and each one guards an
// answer read from the table:
//
//   - the release, which every withheld message names;
//   - the permission vocabulary, the words the requirement is written in;
//   - the public sets, which the call guard reads;
//   - every denial's element, held by the record as the kind its cause says,
//     which is the derivation's gate 2 read back from what it wrote;
//   - what every REST operation demands, which is the join's REST half read
//     back: a re-recording at the same release that changes a route's
//     permissions moves no other check;
//   - the actions the two artifacts cover, written together and so equal, and
//     the catalog this tree builds, which one row per action is joined for;
//   - the classic scope of every operation, held to GitLab's general rule
//     and to the route's skip reason where the record carries one, and of
//     every row, held to the request record and to the operations of its
//     ways, which is what a read_api token is narrowed by.
//
// What a GraphQL operation or position demands is not read back: answering
// it means walking each document against the pinned schema, which is the
// derivation's work, so it is held only by make check-action-grants. Its
// classic scope is, since the operation's kind decides it, with the one
// exception a query selecting a field GitLab answers only to api, which the
// request record names.
func inconsistencies(table *finegrained.Table, record actionrequests.Record, live *apilive.Document, actions []actionrequests.Action) []string {
	var found []string
	if table.Version != live.Source.Version {
		found = append(found, fmt.Sprintf("the table was joined at GitLab %s and the live record is %s; %s",
			table.Version, live.Source.Version, regenerate))
	}
	found = append(found, vocabulary(table, live.Granular)...)
	found = append(found, publicSets(table, live.Granular)...)
	found = append(found, deniedElements(table, live)...)
	found = append(found, restOperations(table, live)...)
	found = append(found, coverage(table, record)...)
	found = append(found, catalogCoverage(table, actions)...)
	found = append(found, classicOperations(table, record, live)...)
	found = append(found, classicRows(table, record)...)
	sort.Strings(found)
	return found
}

// otherCredentialSkips are the skip reasons of the record that mean GitLab
// authenticates a route by a credential the caller passes, a runner's token or
// a pipeline trigger's, the same set the derivation reads.
var otherCredentialSkips = map[string]bool{"runner_token_auth": true, "trigger_token_auth": true}

// classicOperations reads the classic half of the join back from the table:
// a REST GET or HEAD needs read_api, a route GitLab authenticates by another
// credential needs no scope of the token and no other route is said to, a
// mutation needs api, and a query read_api unless the request record names
// it as one selecting a field GitLab answers only to api. A route that is not
// a GET may need read_api, which is the derivation's declaration and nothing
// the record carries; it is listed in the report rather than held here.
func classicOperations(table *finegrained.Table, record actionrequests.Record, live *apilive.Document) []string {
	routes := make(map[string]*apilive.Route, len(live.Routes))
	for i := range live.Routes {
		routes[apilive.RouteName(&live.Routes[i])] = &live.Routes[i]
	}
	apiOnly := map[string]bool{}
	for _, action := range record.Actions {
		for _, request := range action.Requests {
			if strings.HasPrefix(request.ClassicDeclaration, actionrequests.ClassicAPIOnlyField) {
				apiOnly[request.Operation] = true
			}
		}
	}
	var found []string
	for i := range table.Operations {
		op := &table.Operations[i]
		if want := classicWant(op, routes[op.Name], apiOnly[op.Name]); want != "" {
			found = append(found, fmt.Sprintf("the table says %s needs %s and %s; %s", op.Name, op.Classic, want, regenerate))
		}
	}
	return found
}

// classicWant says how an operation's classic scope departs from what its kind
// and its route allow, "" when it does not. A route the record does not hold
// is restOperations' to report.
func classicWant(op *finegrained.Operation, route *apilive.Route, apiOnly bool) string {
	kind, _, _ := strings.Cut(op.Name, " ")
	if kind == "mutation" {
		return mustNeed(op.Classic, finegrained.ClassicAPI, "a mutation needs api")
	}
	if kind == "query" {
		if apiOnly {
			return mustNeed(op.Classic, finegrained.ClassicAPI, "the request record says the query selects a field GitLab answers only to api")
		}
		return mustNeed(op.Classic, finegrained.ClassicReadAPI, "a query needs read_api")
	}
	if route == nil {
		return ""
	}
	if route.Method == http.MethodGet || route.Method == http.MethodHead {
		return mustNeed(op.Classic, finegrained.ClassicReadAPI, "GitLab accepts read_api for a "+route.Method)
	}
	skip := ""
	if route.Authorization != nil {
		skip = route.Authorization.Skip
	}
	if otherCredentialSkips[skip] {
		return mustNeed(op.Classic, finegrained.ClassicOtherCredential, "the live record's skip reason "+skip+" says GitLab authenticates it by another credential")
	}
	if op.Classic != finegrained.ClassicReadAPI && op.Classic != finegrained.ClassicAPI {
		return fmt.Sprintf("a %s with the skip reason %q needs read_api or api", route.Method, skip)
	}
	return ""
}

// mustNeed is the reason an operation's scope is wrong, "" when it is the one
// its kind needs.
func mustNeed(got, want finegrained.ClassicScope, reason string) string {
	if got == want {
		return ""
	}
	return reason
}

// classicRows holds every row's classic scope to the request record and to
// its ways: it is known, it is what the record says the action needs, and it
// is no more than the least any of its ways needs, equal to it when no way of
// the action is one no fine-grained token passes, since those ways are kept
// out of the row's paths and may need less.
func classicRows(table *finegrained.Table, record actionrequests.Record) []string {
	recorded := make(map[string]string, len(record.Actions))
	for _, action := range record.Actions {
		recorded[action.ID] = action.Classic
	}
	var found []string
	for i := range table.Actions {
		row := &table.Actions[i]
		if row.Classic == finegrained.ClassicUnknown {
			found = append(found, fmt.Sprintf("%s needs no known classic scope in the table; %s", row.ID, regenerate))
			continue
		}
		if want, ok := recorded[row.ID]; ok && want != row.Classic.String() {
			found = append(found, fmt.Sprintf("the table says %s needs %s and the request record says %s; %s", row.ID, row.Classic, want, regenerate))
		}
		if len(row.Paths) == 0 {
			continue
		}
		least := leastWay(table, row.Paths)
		if row.Classic > least || len(row.DeniedWays) == 0 && row.Classic != least {
			found = append(found, fmt.Sprintf("the table says %s needs %s and the least of its ways needs %s; %s", row.ID, row.Classic, least, regenerate))
		}
	}
	return found
}

// leastWay is the classic scope the least demanding of a row's ways needs, a
// way needing the most any of its operations needs and one sending nothing no
// request.
func leastWay(table *finegrained.Table, paths [][]uint32) finegrained.ClassicScope {
	least := finegrained.ClassicAPI
	for _, path := range paths {
		way := finegrained.ClassicNoRequest
		for _, op := range path {
			way = max(way, table.Operations[op].Classic)
		}
		least = min(least, way)
	}
	return least
}

// vocabulary holds the table's raw permissions to the ones the record's
// assignable permissions expand to, which is the set the join reads them
// from.
func vocabulary(table *finegrained.Table, granular *apilive.Granular) []string {
	if granular == nil {
		return []string{"the live record carries no fine-grained permission vocabulary; re-record it with `make gen-api-live`"}
	}
	expandable := granular.Expandable()
	var found []string
	for _, name := range table.Permissions {
		if !expandable[name] {
			found = append(found, fmt.Sprintf("the table names the permission %s, which no assignable of the live record expands to; %s", name, regenerate))
		}
		delete(expandable, name)
	}
	for name := range expandable {
		found = append(found, fmt.Sprintf("the live record's permission %s is missing from the table; %s", name, regenerate))
	}
	return found
}

// publicSets holds the table's public project and group sets to the
// anonymous policy the record evaluated: the table knows a set exactly when
// the record carries one, and holds a permission in it exactly when the
// record does.
func publicSets(table *finegrained.Table, granular *apilive.Granular) []string {
	var public *apilive.PublicAnonymous
	if granular != nil {
		public = granular.PublicAnonymous
	}
	recordKnows := public != nil
	if table.PublicKnown != recordKnows {
		return []string{fmt.Sprintf("the table says the public sets are known (%t) and the live record says %t; %s",
			table.PublicKnown, recordKnows, regenerate)}
	}
	if !recordKnows {
		return nil
	}
	var found []string
	for _, boundary := range []struct {
		name  string
		index int
		names []string
	}{
		{name: "project", index: finegrained.PublicProject, names: public.Project},
		{name: "group", index: finegrained.PublicGroup, names: public.Group},
	} {
		for perm, name := range table.Permissions {
			if bitSet(table.PublicAnonymous[boundary.index], perm) != slices.Contains(boundary.names, name) {
				found = append(found, fmt.Sprintf("the table and the live record disagree on whether %s is public on a %s; %s",
					name, boundary.name, regenerate))
			}
		}
	}
	return found
}

// bitSet reports whether bit index is set in bits, false past its end.
func bitSet(bits []uint64, index int) bool {
	word := index / 64
	return word < len(bits) && bits[word]&(1<<(index%64)) != 0
}

// deniedElements reads gate 2 back from the table: every denial, of an action
// or of one way of running it, names something the record holds as the kind
// its cause says. A denial that does not was decided by nothing GitLab
// declared.
func deniedElements(table *finegrained.Table, live *apilive.Document) []string {
	var found []string
	for i := range table.Actions {
		row := &table.Actions[i]
		if row.Denied != nil && !live.HoldsDenial(row.Denied) {
			found = append(found, fmt.Sprintf("%s is denied by %s, which the live record does not hold as a %s; %s",
				row.ID, row.Denied.Element, row.Denied.Cause, regenerate))
		}
		for j := range row.DeniedWays {
			way := &row.DeniedWays[j]
			if !live.HoldsDenial(way) {
				found = append(found, fmt.Sprintf("a way of running %s is denied by %s, which the live record does not hold as a %s; %s",
					row.ID, way.Element, way.Cause, regenerate))
			}
		}
	}
	return found
}

// restOperations reads the join's REST half back from the table: every
// operation the table names as a route is a route of the live record, and
// demands what that route's authorization says, read by the same
// apilive.Route.Requirements the join reads it by, group for group and in
// order, with the same skip. A route GitLab deferred or never declared demands
// no group and is no skip; the denial it decides is deniedElements' to hold.
func restOperations(table *finegrained.Table, live *apilive.Document) []string {
	routes := make(map[string]*apilive.Route, len(live.Routes))
	for i := range live.Routes {
		routes[apilive.RouteName(&live.Routes[i])] = &live.Routes[i]
	}
	var found []string
	for i := range table.Operations {
		op := &table.Operations[i]
		if graphQLOperation(op.Name) {
			continue
		}
		route := routes[op.Name]
		if route == nil {
			found = append(found, fmt.Sprintf("the table's operation %s is no route of the live record; %s", op.Name, regenerate))
			continue
		}
		read, skip, _ := route.Requirements()
		if want, got := demand(read, skip), demand(tableGroups(table, op), op.Skip); got != want {
			found = append(found, fmt.Sprintf("the table says %s demands %s and the live record says %s; %s",
				op.Name, got, want, regenerate))
		}
	}
	return found
}

// tableGroups reads an operation's groups out of the table in the form the
// record's are read in, permission names in the order the group holds them.
func tableGroups(table *finegrained.Table, op *finegrained.Operation) []apilive.Requirement {
	groups := make([]apilive.Requirement, 0, len(op.Groups))
	for _, index := range op.Groups {
		group := &table.Groups[index]
		names := make([]string, 0, len(group.Perms))
		for _, perm := range group.Perms {
			names = append(names, table.Permissions[perm])
		}
		groups = append(groups, apilive.Requirement{Permissions: names, Any: group.Any})
	}
	return groups
}

// demand spells what an operation demands, so the table's and the record's
// compare as text and a disagreement reads as one: each group's permissions
// and the boundaries it may be held at, then the skip.
func demand(groups []apilive.Requirement, skip bool) string {
	parts := make([]string, 0, len(groups)+1)
	for _, group := range groups {
		parts = append(parts, strings.Join(group.Permissions, " and ")+" at "+group.Any.String())
	}
	if skip {
		parts = append(parts, "a skip that leaves the route to GitLab")
	}
	if len(parts) == 0 {
		return "no permission"
	}
	return strings.Join(parts, "; ")
}

// catalogCoverage holds the table to the catalog this tree builds. The
// generator joins one row per catalog action, so an action the catalog builds
// that the table lacks was added after the last derivation, and a row no
// catalog builds is one an action left behind.
func catalogCoverage(table *finegrained.Table, actions []actionrequests.Action) []string {
	built := make(map[string]bool, len(actions))
	for _, action := range actions {
		built[action.ID] = true
	}
	var found []string
	for i := range table.Actions {
		id := table.Actions[i].ID
		if !built[id] {
			found = append(found, fmt.Sprintf("%s has a row in the table and is built by no catalog; %s", id, regenerate))
		}
		delete(built, id)
	}
	for id := range built {
		found = append(found, fmt.Sprintf("%s is built by the catalog and has no row in the table; %s", id, regenerate))
	}
	return found
}

// coverage holds the table and the request record to the same actions. The
// generator writes both from one join, so an action one holds and the other
// does not is an artifact left behind.
func coverage(table *finegrained.Table, record actionrequests.Record) []string {
	recorded := make(map[string]bool, len(record.Actions))
	for _, action := range record.Actions {
		recorded[action.ID] = true
	}
	var found []string
	for i := range table.Actions {
		id := table.Actions[i].ID
		if !recorded[id] {
			found = append(found, fmt.Sprintf("%s has a row in the table and no entry in the request record; %s", id, regenerate))
		}
		delete(recorded, id)
	}
	for id := range recorded {
		found = append(found, fmt.Sprintf("%s has an entry in the request record and no row in the table; %s", id, regenerate))
	}
	return found
}
