package main

import (
	"sort"
	"strings"
)

// categoryOnlyAsText is the one reason a function may close a server and go
// on reading its address: afterwards the address is only text, compared or
// printed, and nothing sends a request to it.
const categoryOnlyAsText = "only-as-text"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryOnlyAsText: "after the close the address is only read as text and nothing dials it, so no listener given the port can answer in its place",
}

// declaration is one excused function: why it reads a closed server's address,
// in a category and in words.
type declaration struct {
	category string
	reason   string
}

// usedAfterTheCloseOnPurpose are the functions this repository lets close an
// httptest server and go on using its address, each with the reason that is
// right.
//
// The table is meant to stay empty. A server closed in the middle of a test
// gives its port back at once, and a test that needs an address nothing
// answers on has netfixture.DeadEndpoint, which keeps the port, and one that
// needs an instance to answer and then go away has
// netfixture.NewStoppableServer. An entry is for the day reading the address
// after the close is a choice rather than an omission, and it names the
// function rather than the line so it survives an edit above it.
//
// The key is the package the repository names, a colon, and the function as
// its file spells it (`internal/x:TestY`, `internal/x:Type.Method`), which is
// also how a finding names where it sits. An entry that excuses nothing is
// reported, on the terms every declaration table here is held to: a
// declaration that has stopped describing the tree is itself a finding.
var usedAfterTheCloseOnPurpose = map[string]declaration{}

// declarationKey is how a finding is named in the declaration table.
func declarationKey(finding Finding) string {
	return finding.Package + ":" + finding.Func
}

// unknownCategories names the declarations whose category is not one of
// [categories], in a stable order.
func unknownCategories(declared map[string]declaration) []string {
	var unknown []string
	for key, entry := range declared {
		if _, known := categories[entry.category]; !known {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// staleDeclarations names the entries that excused nothing this run, out of
// the ones this run was in a position to judge.
//
// An entry naming a package the run never loaded is passed over rather than
// reported: a run narrowed to one package would otherwise condemn every
// declaration outside it, which would teach a reader to ignore the line.
func staleDeclarations(declared map[string]declaration, excused, scanned map[string]struct{}) []string {
	var stale []string
	for key := range declared {
		if _, used := excused[key]; used {
			continue
		}
		pkg, _, _ := strings.Cut(key, ":")
		if _, looked := scanned[pkg]; !looked {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	return stale
}
