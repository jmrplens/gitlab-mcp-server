package main

import (
	"sort"
	"strings"
)

// proseExemptions are the dotted tokens a Usage line or a description may
// spell that are not action IDs, each with the reason it is not one.
//
// The table is meant to stay short. What keeps it short is the test in
// [proseCandidates]: a token is a candidate when either half is one the
// catalog uses, which turns away github.com, gitlab.com and e.g, since neither
// half of those is a domain or an action name. What is left are the tokens
// that pass that test and are still not IDs, and this repository has written
// down what a long list of permanent exemptions costs: a reader learns to skip
// it. If this grows past a handful, the rule that produced the entries is the
// thing to fix.
//
// [proseNonDomains] carries the other half, and exists because one shape
// recurs. The rule used to demand a known domain, which turned away every
// params.note_id an example binding writes; widening it to accept a known
// action name on either side is what let this command see "commit.list", the
// commonest wrong spelling in prose, and it let params.status in with it.
// A binding prefix is a class rather than an instance, so it is declared as
// one: a table of every params.x that happens to collide with an action name
// would grow without ever saying anything.
//
// An entry that exempts nothing is reported, on the terms every declaration
// table here is held to: a declaration that has stopped describing the tree is
// itself a finding.
var proseExemptions = map[string]string{
	"project.git": "the suffix of a git remote URL, quoted by projectdiscovery when it explains what it parses",
}

// proseNonDomains are left halves that never name a catalog domain, whatever
// their right half spells, each with what they do name instead.
var proseNonDomains = map[string]string{
	"params": "the binding an example writes, as in params.status: the right half is a parameter name and collides with an action name by accident",
}

// exemptProse reports whether a prose token is declared not to be an action ID.
func exemptProse(token string) bool {
	if _, declared := proseExemptions[token]; declared {
		return true
	}
	domain, _, found := strings.Cut(token, ".")
	if !found {
		return false
	}
	_, isBinding := proseNonDomains[domain]
	return isBinding
}

// staleProseExemptions names the entries that excused nothing this run.
func staleProseExemptions(used map[string]struct{}) []string {
	var stale []string
	for token := range proseExemptions {
		if _, wasUsed := used[token]; !wasUsed {
			stale = append(stale, token)
		}
	}
	sort.Strings(stale)
	return stale
}
