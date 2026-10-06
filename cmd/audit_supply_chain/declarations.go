package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// declarationsFile is where a declaration table finding sends its reader.
const declarationsFile = "cmd/audit_supply_chain/declarations.go"

// categoryHeldOffline is the one reason a match may be excused: the tool the
// pattern names is always run with a flag that forbids it to reach a
// registry, so what it runs is what the job built or installed from files it
// verified, and the reason names the flag and where it is passed.
const categoryHeldOffline = "held-offline"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryHeldOffline: "the matched tool is always given a flag that forbids network resolution, so it runs what the job built or verified; the reason names the flag and where it is passed",
}

// declaration is one excused match: why a file a credentialed job reaches may
// carry a pattern the run-time-code rules refuse, in a category and in words.
type declaration struct {
	category string
	reason   string
}

// declaredRunTimeCode are the matches this repository accepts in a file a
// credentialed job reaches, keyed by the file's repository-relative path (or
// Makefile:target for a recipe), one space, and the rule's name.
//
// An entry excuses that rule in that file wherever a credentialed job reaches
// it, and nothing else: the file's other rules, the same rule in another file,
// and a run block, which is fixed where it is written, are all still judged. A
// reference that resolves to no file is never declared either, since there is
// nothing behind it to excuse. An entry that excuses nothing is reported, on
// the terms every declaration table in this repository is held to.
var declaredRunTimeCode = map[string]declaration{
	"scripts/validate-npm.mjs npx": {
		category: categoryHeldOffline,
		reason: "stopUnderNpx starts the launcher the way a client configured with npx does, to prove a SIGTERM " +
			"sent to npx alone ends the server, and passes --offline in the arguments it spawns npx with, so npx " +
			"resolves the package from the project the job installed from the tarballs it just packed and cannot " +
			"fetch a published one; serverUnder then holds the process to that project's binary. The other " +
			"matches are comments and the child process variable named after the tool",
	},
}

// tableProblems holds both tables to what this audit read, once every workflow
// has been read: the secret table first, then the declarations.
func (a *supplyChainAudit) tableProblems() []string {
	return append(a.secretProblems(), a.declarationProblems()...)
}

// declarationProblems names every declaration that cannot be read as written,
// gives a category this command does not define or no reason, or excused
// nothing, in a stable order.
func (a *supplyChainAudit) declarationProblems() []string {
	keys := make([]string, 0, len(a.declarations))
	for key := range a.declarations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	for _, key := range keys {
		subject, rule, separated := strings.Cut(key, " ")
		if !separated || subject == "" || !slices.Contains(ruleNames(), rule) {
			problems = append(problems, fmt.Sprintf("%s: %q is not a path and a rule name (%s) separated by one space",
				declarationsFile, key, strings.Join(ruleNames(), ", ")))
			continue
		}
		entry := a.declarations[key]
		if categories[entry.category] == "" {
			problems = append(problems, fmt.Sprintf("%s: %q: the category %q is not one this command defines",
				declarationsFile, key, entry.category))
		}
		if strings.TrimSpace(entry.reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: %q gives no reason", declarationsFile, key))
		}
		if !a.excused[key] {
			problems = append(problems, fmt.Sprintf(
				"%s: %q excuses nothing: no credentialed job reaches that file matching that rule", declarationsFile, key,
			))
		}
	}
	return problems
}

// ruleNames are the names a declaration may give a rule, in the order the
// rules are applied.
func ruleNames() []string {
	names := make([]string, 0, len(unlockedCode))
	for _, rule := range unlockedCode {
		names = append(names, rule.name)
	}
	return names
}
