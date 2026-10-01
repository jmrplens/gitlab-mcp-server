package grants

import (
	"fmt"
	"slices"
	"sort"

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
//   - the actions the two artifacts cover, written together and so equal.
func inconsistencies(table *finegrained.Table, record actionrequests.Record, live *apilive.Document) []string {
	var found []string
	if table.Version != live.Source.Version {
		found = append(found, fmt.Sprintf("the table was joined at GitLab %s and the live record is %s; %s",
			table.Version, live.Source.Version, regenerate))
	}
	found = append(found, vocabulary(table, live.Granular)...)
	found = append(found, publicSets(table, live.Granular)...)
	found = append(found, deniedElements(table, live)...)
	found = append(found, coverage(table, record)...)
	sort.Strings(found)
	return found
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
	if table.PublicKnown != (public != nil) {
		return []string{fmt.Sprintf("the table says the public sets are known (%t) and the live record says %t; %s",
			table.PublicKnown, public != nil, regenerate)}
	}
	if public == nil {
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
