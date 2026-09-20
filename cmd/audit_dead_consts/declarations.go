package main

import (
	"sort"
	"strings"
)

// unreadOnPurpose are the unexported constants this repository keeps although
// nothing reads them, each with the reason keeping it is right.
//
// The table is meant to stay empty, and an entry is a claim rather than a
// silence: a constant nothing reads is almost always the residue of a call
// site that was removed or never written, and deleting it is the fix. What an
// entry is for is the case where the declaration itself is the product, such
// as a value a comment above it is about, and where a reader would put it back
// the moment it went.
//
// The key is the package the repository names, a colon, and the constant, as
// [declarationKey] spells it; a constant declared inside a function carries
// that function before its name (`internal/tools/x:Type.Method.name`), so an
// entry for a package-level constant excuses no local one sharing its name
// and the reverse. An entry that excuses nothing is reported, on the terms
// every declaration table here is held to: a declaration that has stopped
// describing the tree is itself a finding.
var unreadOnPurpose = map[string]string{
	"internal/tools/dynamic:aliasSourceCatalog": "the aliasSource block is the vocabulary of a field the alias audit publishes and " +
		"docs/development/dynamic-search-ranker.md lists, and sourceForCompatibilityAlias converts whatever internal/tools/actioncompat " +
		"spells rather than naming a member, so two of the five values reach the surface without any code naming them",
	"internal/tools/dynamic:aliasSourceStandalone": "the same block: actioncompat.SourceStandalone puts \"standalone\" on the wire through " +
		"that conversion, so the name here documents a value the surface really carries",
}

// declarationKey is how a constant is named in the declaration table: its
// package, then its enclosing function when it has one, then its name.
func declarationKey(constant Constant) string {
	if constant.Func == "" {
		return constant.Package + ":" + constant.Name
	}
	return constant.Package + ":" + constant.Func + "." + constant.Name
}

// staleDeclarations names the entries that excused nothing this run, out of
// the ones this run was in a position to judge.
//
// An entry naming a package the run never loaded is passed over rather than
// reported: a run narrowed to one package would otherwise condemn every
// declaration outside it, which would teach a reader to ignore the line.
func staleDeclarations(excused, scanned map[string]struct{}) []string {
	var stale []string
	for key := range unreadOnPurpose {
		if _, used := excused[key]; used {
			continue
		}
		pkg, _, found := strings.Cut(key, ":")
		if !found {
			continue
		}
		if _, looked := scanned[pkg]; !looked {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	return stale
}
