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
// ID of its own, the entry excuses nothing and is reported stale. Whether one
// is used is decided by the Usage lines and by the schema descriptions of
// internal/tools/dynamic, the two places the reasons below name; a schema
// description anywhere else may not name one at all. The suite's quotations
// consult the table without keeping an entry alive, since what they quote is
// the served source the entries are written about.
var declaredAliasMentions = map[string]string{
	"issue.close":  "the gitlab_issue_update Usage line, which exists to tell a model that dynamic execute accepts this spelling and fills state_event from it; dynamic execute's own action description names it as its example of an alias for the same reason",
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

// surfaceMention is one tool name one package's served prose may spell.
type surfaceMention struct {
	pkg  string
	tool string
}

// declaredSurfaceToolMentions are the tool names a package's served prose may
// spell, because every sentence that package writes is served by the one
// surface that registers the tool, each with the reason.
//
// The served-prose rule refuses a tool name because it is right for one
// surface of three, and that premise fails for exactly one package:
// internal/tools/dynamic is the dynamic surface. Its two tools are registered
// only there, and everything it writes (a refusal of an over-long query, the
// unknown-action answer, the description of a result field) is text one of
// those two tools returns, so telling a model to call the other is the one
// portable instruction it has. The canonical ID is no substitute: it names an
// action, and these sentences name the tool an action is passed to.
//
// It is keyed by package rather than added to [hintToolExemptions] because
// the same token anywhere else is the defect the rule exists for: a meta or
// individual sentence naming gitlab_execute_action names a tool that surface
// does not register. A run over the whole tree reports an entry that excused
// nothing, and the gate fails on it, as on every declaration table here.
var declaredSurfaceToolMentions = map[surfaceMention]string{
	{pkg: dynamicPackage, tool: "gitlab_find_action"}:    "the dynamic surface's search tool, named in the text that surface's two tools return",
	{pkg: dynamicPackage, tool: "gitlab_execute_action"}: "the dynamic surface's execute tool, named in the text that surface's two tools return",
}

// dynamicPackage is the one package that is a surface of its own, so the
// served-prose declarations scoped to a package are scoped to it.
const dynamicPackage = "internal/tools/dynamic"

// exemptSurfaceMention reports whether a tool name is declared correct in the
// served prose of the package that spells it.
func exemptSurfaceMention(mention surfaceMention) bool {
	_, declared := declaredSurfaceToolMentions[mention]
	return declared
}

// staleHintDeclarations names the entries of [hintToolExemptions] that excused
// nothing this run.
//
// It fails the gate, on the terms every declaration table here is held to: a
// declaration that has stopped describing the tree is one a reader would
// otherwise trust. It was reported and not gated while the rule it belongs to
// was staged, and kept that way when the rule began gating, which left it the
// one table here whose staleness passed.
func staleHintDeclarations(used map[string]struct{}) []string {
	var stale []string
	for token := range hintToolExemptions {
		if _, wasUsed := used[token]; !wasUsed {
			stale = append(stale, token+" is no longer spelled in any served prose (hintToolExemptions)")
		}
	}
	sort.Strings(stale)
	return stale
}

// staleSurfaceMentions names the entries of [declaredSurfaceToolMentions]
// that excused nothing this run, on the terms of [staleHintDeclarations].
func staleSurfaceMentions(used map[surfaceMention]struct{}) []string {
	var stale []string
	for mention := range declaredSurfaceToolMentions {
		if _, wasUsed := used[mention]; !wasUsed {
			stale = append(stale, mention.tool+" is no longer spelled in the served prose of "+mention.pkg+" (declaredSurfaceToolMentions)")
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
