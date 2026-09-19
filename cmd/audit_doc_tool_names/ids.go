package main

import (
	"sort"
	"strings"
)

// fileNameTails are the right halves that name a file rather than an action:
// an extension, or a segment of a file name the documentation writes out.
//
// This is a class and not a list of instances, which is the whole reason it is
// short. The candidate test is [actionids.IDs.Candidates] and a token
// qualifies when either half is one the catalog uses, so every file whose stem
// happens to be a catalog domain arrives here: server.env, issue.rb,
// pipeline.svg, project.git, repository.md. Enumerating those one by one would
// have filled the table below with fifty entries a reader learns to skip,
// while saying nothing a reader could check. Enumerating the tails says the
// rule: a documentation page writes file names, and a file name's last segment
// is not an action.
//
// No catalog action is named for any of these, so the rule can only excuse a
// mention that was never an action ID.
var fileNameTails = map[string]struct{}{
	"env": {}, "exe": {}, "git": {}, "go": {}, "html": {}, "instructions": {},
	"json": {}, "log": {}, "mcpb": {}, "md": {}, "mdx": {}, "plist": {},
	"properties": {}, "rb": {}, "service": {}, "sh": {}, "sock": {}, "svg": {},
	"target": {}, "txt": {}, "yaml": {}, "yml": {},
}

// metaEntryPrefix is the shape of a meta-surface manifest entry ID.
//
// `gitlab://tools/{id}` publishes one ID per surface, and on the meta surface
// that ID is the tool name and the action joined by a dot:
// gitlab_merge_request.create, gitlab_project.get, gitlab_orbit.query. Those
// are documented call shapes rather than catalog IDs, and their left half is a
// tool name the other half of this command already checks, so nothing is left
// unjudged by passing them over here: a page naming gitlab_no_such_tool.create
// is still reported, as an unregistered tool name.
const metaEntryPrefix = "gitlab_"

// allowedIDs lists what is left: the dotted tokens the documentation spells
// that pass the candidate test, are not file names, and are not action IDs.
// Each entry says what it is instead.
//
// They are three shapes. A telemetry or protocol attribute whose leaf happens
// to be an action name (user.id, resources.subscribe). A path into a document
// or a data file (stats.tools, result.content). And a name that belongs to
// somebody else's software, quoted because this documentation is about it
// (gotest.tools, project.security_setting).
//
// An entry that excuses nothing is reported, on the terms every declaration
// table in this repository is held to.
var allowedIDs = map[string]string{
	"achievement.namespace":    "a GraphQL field path, quoted in upstream-bugs.md as the selection client-go sends",
	"gotest.tools":             "the Go module gotest.tools, named in the static analysis page",
	"mcp.schema":               "the middle of the Agent Plugins schema file name, mcp.schema.json",
	"project.security_setting": "GitLab's own entity name, quoted in upstream-bugs.md as the thing the endpoint answers with",
	"resources.subscribe":      "the MCP method, spelled with a dot by the gateway configuration the enterprise guide quotes",
	"result.content":           "a jq path into a JSON-RPC response, in the CI/CD examples",
	"server.name":              "the io.modelcontextprotocol.server.name image label the registry validates ownership through",
	"server.type":              "a key of the .mcpb manifest, whose value says the bundle carries a binary",
	"stats.tools":              "an Astro binding into site/src/data/stats.json, interpolated into a sentence",
	"user.hash":                "the OpenTelemetry attribute the pseudonymous identity policy emits",
	"user.id":                  "the OpenTelemetry attribute the full identity policy emits",
	"user.name":                "the other attribute of that same policy",
	"issue.close":              "a registered alias, named as an alias by the dynamic tools page that explains what execute accepts",
	"issue.reopen":             "the other half of that same sentence",
}

// exemptID reports whether a dotted documentation token is one of the three
// things that are not an action ID: a file name, a meta-surface entry ID, or a
// declared exception. Only the last is tracked, since only the last is a table
// an entry can go stale in.
func exemptID(token string) (declared, tracked bool) {
	if _, ok := allowedIDs[token]; ok {
		return true, true
	}
	domain, tail, found := strings.Cut(token, ".")
	if !found {
		return false, false
	}
	if _, isFile := fileNameTails[tail]; isFile {
		return true, false
	}
	return strings.HasPrefix(domain, metaEntryPrefix), false
}

// staleAllowedIDs names the entries that excused nothing this run.
//
// The tool-name table beside it deliberately has no such rule: it is older
// than this one, and its entries name shapes a documentation edit may stop
// spelling for a release and spell again later. This table's entries are about
// tokens a page writes today, so an unused one is a lead.
func staleAllowedIDs(used map[string]struct{}) []string {
	var stale []string
	for token := range allowedIDs {
		if _, wasUsed := used[token]; !wasUsed {
			stale = append(stale, token)
		}
	}
	sort.Strings(stale)
	return stale
}

// idFinding is one dotted token a page teaches that the catalog does not hold,
// with the files that spell it.
type idFinding struct {
	// Canonical is what the token resolves to when it is a registered alias
	// rather than a catalog ID, and empty when it resolves to nothing at all.
	// The two are one finding and two fixes: an alias works when a model
	// follows it and is in no listing, and a dead ID works nowhere.
	Canonical string
	Files     []string
}

// describe is how one finding reads in the report.
func (f idFinding) describe(token string) string {
	if f.Canonical != "" {
		return token + " is a registered alias of " + f.Canonical + ", not an action ID a listing publishes"
	}
	return token + " names no action"
}

// sortedTokens orders the findings by how many files spell each one, biggest
// first, so a report opens with the mistake that spread furthest.
func sortedTokens(findings map[string]idFinding) []string {
	tokens := make([]string, 0, len(findings))
	for token := range findings {
		tokens = append(tokens, token)
	}
	sort.Slice(tokens, func(i, j int) bool {
		left, right := findings[tokens[i]], findings[tokens[j]]
		if len(left.Files) != len(right.Files) {
			return len(left.Files) > len(right.Files)
		}
		return tokens[i] < tokens[j]
	})
	return tokens
}

// isIDFamilyPrefix reports whether a dotted token is a family written in
// prose rather than a name, such as `issue.work_item_*`: the star is not a
// name character, so what the token rule matches is everything up to it and
// the trailing underscore is the truncation. It is the same rule
// [wildcardSuffix] already applies to a tool name.
func isIDFamilyPrefix(token string) bool {
	return strings.HasSuffix(token, wildcardSuffix)
}
