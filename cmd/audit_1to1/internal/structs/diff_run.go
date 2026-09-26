package structs

import (
	"maps"
	"slices"
)

// declarationTable is one of the six tables the diff consults before it
// reports a candidate: each entry excuses the findings its key names, with the
// reason it does.
//
// The name travels with the entries because it is how a stale entry is
// reported, and holding both in one value is what keeps a lookup and the
// staleness verdict from reading one table under two names.
type declarationTable struct {
	name    string
	entries map[string]string
}

// declarationTables is every table the diff consults, in the order the report
// lists their stale entries.
var declarationTables = []*declarationTable{
	acceptedOutputRenames,
	curatedRefSubsets,
	docOmittedFields,
	docAddedFields,
	acceptedExtraOutputs,
	acceptedMissingInputs,
}

// staleDeclaration is a declaration that answered no candidate finding of a
// run over the whole tree.
//
// It is a finding on the same terms as the candidates the tables excuse. A key
// nothing reaches is a claim about a field or a type the tree no longer has in
// that shape, and left in place it answers the next finding that happens to
// carry its name, with a reason written for something else.
type staleDeclaration struct {
	// Table names the declaration table the key sits in.
	Table string `json:"table"`
	// Key is the declaration as the table spells it.
	Key string `json:"key"`
	// Reason is the rationale the declaration carries, so a reader deciding
	// whether to delete it or to fix the tree it no longer describes has the
	// original argument in front of them.
	Reason string `json:"reason"`
}

// diffRun is one pass of the field diff over the tree, and the record of which
// declaration keys answered a candidate finding during it.
//
// The record is what the tables were never held to. Each table was read in the
// answering direction only, so a key that stopped matching anything stayed and
// excused nothing until a later finding happened to carry its name. A key
// counts as used when its lookup returns true for a candidate the diff was
// about to report, which is the only thing a declaration is for; a key the diff
// never reaches with a candidate, or reaches only after another declaration
// has already answered it, is stale.
type diffRun struct {
	answered map[*declarationTable]map[string]struct{}
}

// newDiffRun starts a run with nothing answered yet.
func newDiffRun() *diffRun {
	return &diffRun{answered: map[*declarationTable]map[string]struct{}{}}
}

// answer reports whether table declares key, recording the key as used when it
// does.
func (r *diffRun) answer(table *declarationTable, key string) bool {
	if _, declared := table.entries[key]; !declared {
		return false
	}
	used := r.answered[table]
	if used == nil {
		used = map[string]struct{}{}
		r.answered[table] = used
	}
	used[key] = struct{}{}
	return true
}

// staleDeclarations lists every key of tables that answered nothing during the
// run, table by table in the order given and each table's keys sorted.
//
// It is meaningful only after a run over the whole tree, which is the only run
// buildReport makes: the -gaps-only filter drops clean packages from the
// report and never from the diff, so every package has been asked by then.
func (r *diffRun) staleDeclarations(tables []*declarationTable) []staleDeclaration {
	var stale []staleDeclaration
	for _, table := range tables {
		used := r.answered[table]
		for _, key := range slices.Sorted(maps.Keys(table.entries)) {
			if _, ok := used[key]; ok {
				continue
			}
			stale = append(stale, staleDeclaration{Table: table.name, Key: key, Reason: table.entries[key]})
		}
	}
	return stale
}
