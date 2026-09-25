package main

import (
	"sort"
	"strings"
)

// categoryOutlivesTheCall is the one reason a call may be excused: a request
// that is meant to finish whatever becomes of the call that started it, and is
// bounded by something other than the caller's context.
const categoryOutlivesTheCall = "outlives-the-call"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryOutlivesTheCall: "the request must finish even if the call that started it is abandoned, and something other than the caller's context bounds it",
}

// declaration is one excused function: why it passes no context, in a
// category and in words.
type declaration struct {
	category string
	reason   string
}

// withoutContextOnPurpose are the functions this repository lets reach
// client-go without the caller's context, each with the reason that is right.
//
// The table is meant to stay empty. A request built without the context is
// one the action deadline cannot end and an abandoned HTTP POST cannot stop,
// and every call that has done it so far was an omission rather than a
// choice. An entry is for the day one is a choice, and it names the function
// rather than the line so it survives an edit above it.
//
// The key is the package the repository names, a colon, and the function as
// its file spells it (`internal/tools/x:Func`, `internal/tools/x:Type.Method`),
// which is also how a finding names where it sits. An entry that excuses
// nothing is reported, on the terms every declaration table here is held to:
// a declaration that has stopped describing the tree is itself a finding.
var withoutContextOnPurpose = map[string]declaration{}

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
