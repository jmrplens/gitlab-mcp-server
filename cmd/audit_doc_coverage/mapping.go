// Package main: README "Domains" table parser + group→doc mapping.
//
// The mapping is derived from docs/reference/tools/README.md. Most rows list
// one or more gitlab_* meta-tool names that map directly to catalog
// groups; a handful of rows use "various" or include routed tools
// that must be split off their owning group. Both cases are
// enumerated explicitly here so the auditor's main logic stays
// table-driven and the special cases are reviewable in one place.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// docsToolsPrefix is the repo-relative directory containing every
// per-domain tool reference doc. README "Document" links resolve
// against it (e.g. "access.md" -> "docs/reference/tools/access.md").
const docsToolsPrefix = "docs/reference/tools/"

// docMappingRow captures one row of the README "Domains" table.
type docMappingRow struct {
	Domain        string
	ExpectedCount int
	MetaToolsCSV  string
	DocLink       string
}

// docMapping is the full README "Domains" table plus the
// hardcoded group→doc overrides needed to handle routed tools and
// "various" rows.
type docMapping struct {
	Rows      []docMappingRow
	Override  docOverrideMap
	Ownership map[string]docOwnershipRule
}

// docOverrideMap is the set of catalog tools that should be
// attributed to a doc file even though their owning group maps
// elsewhere. Keys are doc files (relative to docs/reference/tools); values
// are the IndividualTool.Name strings expected in that doc.
//
// Examples of why this exists:
//   - branch-rules.md owns gitlab_list_branch_rules, whose owning
//     group is gitlab_branch (not its own meta-tool).
//   - project-discovery.md owns gitlab_discover_project, whose
//     owning "group" is the standalone surface utility group
//     gitlab_discover_project.
//   - capabilities.md owns the four gitlab_interactive_* elicitation
//     tools plus gitlab_server_status, whose owning groups are
//     gitlab_server (MCP maintenance) and the standalone surface
//     utility.
type docOverrideMap map[string][]string

// loadDocMapping parses docs/reference/tools/README.md and merges in the
// hardcoded overrides that handle routed tools and "various" rows.
func loadDocMapping(readmePath string) (*docMapping, error) {
	rows, err := parseDomainsTable(readmePath)
	if err != nil {
		return nil, err
	}
	ownershipPath := filepath.Join(filepath.Dir(readmePath), "doc-ownership.json")
	ownership, err := loadOwnershipRules(ownershipPath)
	if err != nil {
		return nil, fmt.Errorf("load ownership rules: %w", err)
	}
	return &docMapping{
		Rows:      rows,
		Override:  hardcodedDocOverrides(),
		Ownership: ownership,
	}, nil
}

// loadOwnershipRules reads the doc-ownership.json data file that supplements
// the README table with group extensions and prefix routing for docs whose
// README rows use "various" or "etc.". A missing file is treated as "no
// extra rules" so the auditor still runs against a bare checkout.
//
// Each entry may carry a "note" field documenting why the doc claims those
// groups/prefixes; it is human rationale only and intentionally ignored by
// the parser below (the anonymous struct reads "groups" and "prefixes" and
// silently drops any other key).
func loadOwnershipRules(path string) (map[string]docOwnershipRule, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- path derived from readmePath, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]docOwnershipRule{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var raw map[string]struct {
		Groups   []string `json:"groups"`
		Prefixes []string `json:"prefixes"`
	}
	if uerr := json.Unmarshal(data, &raw); uerr != nil {
		return nil, fmt.Errorf("parse %s: %w", path, uerr)
	}
	result := make(map[string]docOwnershipRule, len(raw))
	for doc, r := range raw {
		result[doc] = docOwnershipRule{Groups: r.Groups, Prefixes: r.Prefixes}
	}
	return result, nil
}

// parseDomainsTable reads the Domains section of docs/reference/tools/README.md
// and returns one row per table entry. Only the | Domain | Tools |
// Meta-tool | Document | columns are parsed; other columns (when
// added) are ignored.
func parseDomainsTable(readmePath string) ([]docMappingRow, error) {
	f, err := os.Open(readmePath)
	if err != nil {
		return nil, fmt.Errorf("open readme: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	inDomains := false
	var rows []docMappingRow

	// Markdown table row pattern. Captures the four data columns and
	// tolerates varying whitespace around the pipes.
	rowRE := regexp.MustCompile(`^\|\s*(.+?)\s*\|\s*(\d+)\s*\|\s*(.+?)\s*\|\s*\[(.+?)\]\((.+?)\)\s*\|`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Domains" {
			inDomains = true
			continue
		}
		if !inDomains {
			continue
		}
		// End of the Domains table when we hit a non-table line that
		// isn't the separator row.
		if trimmed != "" && !strings.HasPrefix(trimmed, "|") {
			break
		}
		matches := rowRE.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		count, convErr := strconv.Atoi(matches[2])
		if convErr != nil {
			return nil, fmt.Errorf("invalid tool count %q in row %q: %w", matches[2], line, convErr)
		}
		rows = append(rows, docMappingRow{
			Domain:        matches[1],
			ExpectedCount: count,
			MetaToolsCSV:  matches[3],
			DocLink:       matches[5],
		})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan readme: %w", scanErr)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no Domains rows parsed from %s", readmePath)
	}
	return rows, nil
}

// relativeDocPath converts a README "Document" link target (e.g.
// "access.md") to the repo-relative doc path used in the report
// (e.g. "docs/reference/tools/access.md").
func relativeDocPath(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimSuffix(link, ".md")
	base := filepath.Base(link)
	if base == "" || base == "." {
		return docsToolsPrefix
	}
	return docsToolsPrefix + base + ".md"
}

// expectedGroupsForRow returns the catalog group ToolNames that the
// given README row maps to. The result is the parsed "Meta-tool"
// column with backticks, parenthetical annotations, and other noise
// stripped. Rows annotated "(routed)" return nil: the listed group is
// a routing conduit, but the row owns only the tools enumerated in
// the hardcoded override (see parsePrefixAllowlists for the
// explicit per-doc naming-prefix lists that handle shared-group
// docs).
//
// First-claimer-wins in computeExpectedByDoc means the FIRST row to
// claim a given group is the canonical home for all tools in that
// group. Rows after the first that ALSO claim a group are interpreted
// via the prefix allowlist, not by re-claiming the group wholesale.
func expectedGroupsForRow(row docMappingRow) []string {
	if isRoutedRow(row) {
		return nil
	}

	// Parse the "Meta-tool" column. Tokens are comma-separated, may
	// contain backticks, and may include "etc.", "(routed)", or
	// "(enterprise routes)" annotations that we strip.
	csv := row.MetaToolsCSV
	csv = strings.ReplaceAll(csv, "`", "")
	csv = strings.ReplaceAll(csv, "etc.", "")
	csv = strings.ReplaceAll(csv, "(routed)", "")
	csv = strings.ReplaceAll(csv, "(enterprise routes)", "")
	csv = strings.ReplaceAll(csv, "(with `TOOL_SURFACE=meta`, routed as a branch action)", "")
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		// Keep only canonical gitlab_* meta-tool names; drop empty
		// tokens and parenthesised notes that may be left over.
		if trimmed == "" || !strings.HasPrefix(trimmed, "gitlab_") {
			continue
		}
		if strings.ContainsAny(trimmed, "()") {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hardcodedDocOverrides returns the explicit list of routed tools
// per doc file. Tools listed here are attributed to the doc even
// though their owning group is mapped elsewhere by the README.
//
// The keys are doc basenames (e.g. "branch-rules.md"); the values are
// the IndividualTool.Name strings. Keep this list small and well-
// commented; the auditor's job is to verify the docs match the
// catalog, so any new routing exception should be reviewed here.
func hardcodedDocOverrides() docOverrideMap {
	return docOverrideMap{
		// Branch rules routes through gitlab_branch but its docs live
		// in branch-rules.md rather than branches.md.
		"branch-rules.md": {
			"gitlab_list_branch_rules",
		},
		// Project discovery is a standalone surface utility that
		// ships in its own meta-tool but is documented separately.
		"project-discovery.md": {
			"gitlab_discover_project",
		},
		// MCP capabilities: 4 elicitation tools + gitlab_server_status
		// (the rest of gitlab_server lives under admin.md). The
		// README's "15" count is the sum across these tools; we
		// enumerate them here to avoid depending on gitlab_server's
		// full action set.
		"capabilities.md": {
			"gitlab_interactive_issue_create",
			"gitlab_interactive_mr_create",
			"gitlab_interactive_project_create",
			"gitlab_interactive_release_create",
			"gitlab_server_status",
		},
		// Orbit is a GitLab.com-only meta-tool. The auditclient
		// mock reports a non-gitlab.com URL so the catalog walk
		// skips it entirely. We hardcode the six Orbit Knowledge
		// Graph tools here so orbit.md's expected set reflects the
		// production deployment and the Phase-1 agents can write
		// prose against a known set.
		"orbit.md": {
			"gitlab_orbit_dsl",
			"gitlab_orbit_graph_status",
			"gitlab_orbit_query",
			"gitlab_orbit_schema",
			"gitlab_orbit_status",
			"gitlab_orbit_tools",
		},
	}
}

// computeExpectedByDoc walks the catalog and assigns each tool to
// the doc file that should own it, using first-claimer-wins:
//
//  1. The hardcoded routed-tool overrides take precedence
//     (branch-rules.md, capabilities.md, project-discovery.md).
//  2. Otherwise the FIRST README row in table order whose claimed
//     groups include the tool's owning group is its primary home.
//  3. Otherwise the tool's name prefix is matched against the
//     per-doc prefix allowlist (covers shared-group docs like
//     boards, mirrors, access, security, notifications,
//     integrations, identity-security, analytics-compliance).
//  4. Otherwise the tool is unassigned.
//
// First-claimer-wins keeps the README's ordering meaningful: each
// row in the Domains table is the canonical home for the groups it
// claims, and any later row that ALSO claims a group is interpreted
// as "only owns the subset of tools whose name matches the shared
// naming convention". The prefix allowlist handles that subset
// without forcing every doc to enumerate every tool by hand.
func computeExpectedByDoc(mapping *docMapping, catalog *catalogSnapshot) (out map[string][]string, unassigned []string) {
	firstClaimer := buildFirstClaimer(mapping)
	prefixRules := buildPrefixRules(mapping)
	overrideDoc := buildOverrideDoc(mapping)

	out = make(map[string][]string)
	seenInCatalog := make(map[string]bool)

	// Step 4: assign each catalog tool to its primary home. Tools
	// matching multiple prefix rules go to the doc that owns the
	// longest matching prefix.
	for toolName, info := range catalog.Tools {
		seenInCatalog[toolName] = true
		if doc, ok := assignTool(toolName, info, firstClaimer, prefixRules, overrideDoc); ok {
			out[doc] = append(out[doc], toolName)
			continue
		}
		// Events tools are joined into gitlab_user by
		// buildUserActionSpecs, so the first-claimer rule above
		// already places them in users.md. If a future events tool
		// lands outside that group it surfaces here as unassigned
		// and gets explicit routing via a follow-up allowlist entry.
		unassigned = append(unassigned, toolName)
	}

	// Step 5: the override list may include tools that the catalog
	// walk didn't surface (standalone surface utilities like
	// gitlab_interactive_* and gitlab_discover_project). Always
	// add them to the target doc so the Phase-1 agents know they're
	// in scope, even when the catalog snapshot doesn't enumerate
	// them. These tools are NOT counted as missing — the doc that
	// documents them satisfies the override.
	for tool, doc := range overrideDoc {
		if seenInCatalog[tool] {
			continue
		}
		out[doc] = append(out[doc], tool)
	}

	// Dedupe and sort each doc's expected list.
	for d, tools := range out {
		out[d] = sortedUnique(tools)
	}

	unassigned = sortedUnique(unassigned)
	return out, unassigned
}

// parsePrefixAllowlists returns the per-doc naming-prefix allowlists
// used by computeExpectedByDoc to handle docs that share groups with
// other docs. The longest matching prefix wins, so the lists are
// ordered from most specific to most generic.
//
// Each entry is a comment-and-table combo:
//   - The doc basename identifies the file under docs/reference/tools/.
//   - The list of prefixes enumerates every IndividualTool.Name
//     segment that this doc owns, regardless of owning group.
//
// The lists are derived from the docs as they exist today (PR #190
// shipped the prose), plus the README's "(routed)" annotations for
// tools that are documented in a non-primary doc.
// docOwnershipRule captures one docs/reference/tools/<doc>.md file's catalog
// ownership rules. Each doc may claim catalog groups (the README
// "Meta-tool" column omits these when truncated with "etc." /
// "various") and naming prefixes (tools belonging to this doc even
// when their owning group is shared). The single source of truth
// projects into:
//
//   - groupExtensions() for the first-claimer group lookup
//   - parsePrefixAllowlists() for the prefix-wins tool routing
//
// Keys are doc basenames ("access.md") because both functions
// re-key the result into the canonical "docs/reference/tools/<name>.md" form
// before returning.
type docOwnershipRule struct {
	Groups   []string
	Prefixes []string
}

// groupExtensions projects docOwnershipRules into the group→doc map
// used by the first-claimer lookup in computeExpectedByDoc. Keys are
// the canonical "docs/reference/tools/<name>.md" form so the per-doc lookup
// in buildReport matches the fileFinding entries directly. Docs
// with no group claims are skipped (the first-claimer rule does not
// claim them).
func groupExtensions(rules map[string]docOwnershipRule) map[string][]string {
	out := make(map[string][]string)
	for doc, rule := range rules {
		if len(rule.Groups) == 0 {
			continue
		}
		out[docsToolsPrefix+doc] = append([]string(nil), rule.Groups...)
	}
	return out
}

// parsePrefixAllowlists projects docOwnershipRules into the
// canonical doc→prefixes map used by the prefix-wins routing in
// assignTool. Keys are the canonical "docs/reference/tools/<name>.md" form so
// the longest-match-wins lookup finds the right doc; docs with no
// prefix claims are skipped.
func parsePrefixAllowlists(rules map[string]docOwnershipRule) map[string][]string {
	out := make(map[string][]string)
	for doc, rule := range rules {
		if len(rule.Prefixes) == 0 {
			continue
		}
		out[docsToolsPrefix+doc] = append([]string(nil), rule.Prefixes...)
	}
	return out
}

// buildFirstClaimer returns a map from catalog group ToolName to
// the canonical "docs/reference/tools/<name>.md" path of the first README
// row that claims it. groupExtensions patches in groups the README
// truncated with "etc." or otherwise omitted (e.g. the
// "gitlab_ci_variable" group claimed by ci-cd.md) so the
// first-claimer lookup never falls through to unassigned for those
// groups.
func buildFirstClaimer(mapping *docMapping) map[string]string {
	firstClaimer := make(map[string]string)
	for _, row := range mapping.Rows {
		canonicalDoc := relativeDocPath(row.DocLink)
		for _, g := range expectedGroupsForRow(row) {
			if _, taken := firstClaimer[g]; taken {
				continue
			}
			firstClaimer[g] = canonicalDoc
		}
	}
	for d, groups := range groupExtensions(mapping.Ownership) {
		canonicalDoc := canonicalOverridePath(d)
		for _, g := range groups {
			if _, taken := firstClaimer[g]; taken {
				continue
			}
			firstClaimer[g] = canonicalDoc
		}
	}
	return firstClaimer
}

// prefixRule is one (doc, prefix) tuple from parsePrefixAllowlists.
// Multiple rules can match a single tool; the longest prefix wins.
type prefixRule struct {
	doc    string
	prefix string
}

// buildPrefixRules flattens the per-doc prefix allowlist into a
// single slice, sorted longest-prefix-first so the best match is
// found first during assignTool.
func buildPrefixRules(mapping *docMapping) []prefixRule {
	prefixByDoc := parsePrefixAllowlists(mapping.Ownership)
	docKeys := make([]string, 0, len(prefixByDoc))
	for d := range prefixByDoc {
		docKeys = append(docKeys, d)
	}
	sort.Strings(docKeys)
	var rules []prefixRule
	for _, d := range docKeys {
		prefixes := prefixByDoc[d]
		sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
		for _, p := range prefixes {
			rules = append(rules, prefixRule{d, p})
		}
	}
	return rules
}

// buildOverrideDoc materializes the hardcoded routed-tool overrides
// as a tool→doc map. canonicalOverridePath lifts the basenames to
// the canonical form so the rest of the resolution logic can use
// the map uniformly.
func buildOverrideDoc(mapping *docMapping) map[string]string {
	out := make(map[string]string)
	for d, tools := range mapping.Override {
		canonicalDoc := canonicalOverridePath(d)
		for _, t := range tools {
			out[t] = canonicalDoc
		}
	}
	return out
}

// assignTool returns the canonical doc path for a single catalog
// tool. The lookup order is: hardcoded override (routed tools win)
// → longest-matching prefix rule (shared-group docs) → first-claimer
// of the owning group. Returns ("", false) when none of these
// rules claim the tool; the caller is then responsible for the
// events-package fallback and unassigned tracking.
func assignTool(toolName string, info catalogTool, firstClaimer map[string]string, prefixRules []prefixRule, overrideDoc map[string]string) (string, bool) {
	if doc, ok := overrideDoc[toolName]; ok {
		return doc, true
	}
	var bestDoc string
	var bestLen int
	for _, rule := range prefixRules {
		if !strings.HasPrefix(toolName, rule.prefix) {
			continue
		}
		if len(rule.prefix) > bestLen {
			bestDoc = rule.doc
			bestLen = len(rule.prefix)
		}
	}
	if bestDoc != "" {
		return bestDoc, true
	}
	if doc, ok := firstClaimer[info.Group]; ok {
		return doc, true
	}
	return "", false
}

// canonicalOverridePath converts a hardcoded override key (basename
// like "branch-rules.md") to the canonical "docs/reference/tools/<name>.md"
// form so it matches relativeDocPath's output and the per-doc
// lookup in buildReport can translate to the filesystem-discovered
// path with a single basename map.
func canonicalOverridePath(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, docsToolsPrefix) {
		return name
	}
	return docsToolsPrefix + strings.TrimPrefix(name, "./")
}

// isRoutedRow reports whether the README row's "Meta-tool" column
// carries the "(routed)" annotation. Routed rows do not claim the
// listed group wholesale — they only own the tools enumerated in the
// hardcoded override for that doc (e.g. branch-rules.md).
func isRoutedRow(row docMappingRow) bool {
	return strings.Contains(row.MetaToolsCSV, "(routed)")
}

// sortedUnique returns the sorted, deduplicated copy of values.
func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
