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

// declaredAliasMentions are the prose tokens that name a **registered alias**
// on purpose, because the sentence is about the alias.
//
// The gate demands a canonical catalog ID everywhere else, and this table is
// the one place that demand would be wrong rather than strict. A cross-link
// field publishes IDs a model calls, so an alias there is a defect whatever it
// resolves to; a Usage line is prose, and a line whose subject is "execute also
// accepts these two spellings" cannot state that in canonical IDs without
// saying something false. The distinction is enforced structurally rather than
// by trust: only a prose site consults this table, so a related entry or a hint
// naming one of these is still a finding.
//
// It stays a table of two because a run also refuses an entry that no longer
// resolves as an alias: if the spelling is retired, or promoted to a canonical
// ID of its own, the entry excuses nothing and is reported stale.
var declaredAliasMentions = map[string]string{
	"issue.close":  "the gitlab_issue_update Usage line, which exists to tell a model that dynamic execute accepts this spelling and fills state_event from it",
	"issue.reopen": "the other half of that same sentence",
}

// hintToolExemptions are the gitlab_-shaped tokens a hint may spell that name
// no tool, each with what they name instead.
//
// The hint rule refuses every gitlab_* name, registered or not, because the
// canonical action ID is the one spelling every surface resolves. That makes
// the table's job narrow: a token this shape that is not a tool name at all.
// GitLab's own template families are the class, and the documentation gate
// declares this same token for this same reason, so a spelling one of them
// passes over cannot be one the other reports.
var hintToolExemptions = map[string]string{
	"gitlab_ci_ymls": "a GitLab template family and API path segment (templates/gitlab_ci_ymls), named by the project-template hints beside dockerfiles and gitignores",
}

// exemptHintTool reports whether a gitlab_-shaped token in a hint is declared
// not to be a tool name.
func exemptHintTool(token string) bool {
	_, declared := hintToolExemptions[token]
	return declared
}

// staleHintDeclarations names the entries of [hintToolExemptions] that excused
// nothing this run.
//
// It is reported rather than gated, like everything else the hint rule says:
// the rule it belongs to reports, and a stale entry there cannot be worth more
// than the findings around it.
func staleHintDeclarations(used map[string]struct{}) []string {
	var stale []string
	for token := range hintToolExemptions {
		if _, wasUsed := used[token]; !wasUsed {
			stale = append(stale, token+" is no longer spelled in any hint (hintToolExemptions)")
		}
	}
	sort.Strings(stale)
	return stale
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

// exemptAliasMention reports whether a prose token is declared to name a
// registered alias on purpose.
func exemptAliasMention(token string) bool {
	_, declared := declaredAliasMentions[token]
	return declared
}

// staleDeclarations names the entries of both tables that excused nothing this
// run, each with the table it sits in, so a reader is sent to one file and one
// map rather than to a token they then have to find.
func staleDeclarations(usedExemptions, usedAliasMentions map[string]struct{}) []string {
	var stale []string
	for token := range proseExemptions {
		if _, wasUsed := usedExemptions[token]; !wasUsed {
			stale = append(stale, token+" is no longer spelled in any Usage line or description (proseExemptions)")
		}
	}
	for token := range declaredAliasMentions {
		if _, wasUsed := usedAliasMentions[token]; !wasUsed {
			stale = append(stale, token+" is no longer a registered alias named in prose (declaredAliasMentions)")
		}
	}
	sort.Strings(stale)
	return stale
}
