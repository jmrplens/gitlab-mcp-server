package dynamic

import (
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

const aliasPairSeparator = "\x00"

// AliasAuditFinding describes one dynamic alias governance finding.
type AliasAuditFinding struct {
	Severity  string
	Problem   string
	Alias     string
	Canonical string
	Source    string
	Message   string
}

// CatalogDiscoveryFinding describes one catalog action whose dynamic search
// metadata is weak enough to deserve review.
type CatalogDiscoveryFinding struct {
	Severity string
	Problem  string
	Tool     string
	Action   string
	ID       string
	Message  string
}

// AuditDefaultActionAliases returns governance findings for catalog-projected
// compatibility aliases. It reports duplicate alias/canonical pairs, aliases
// that map to missing canonical actions when a catalog is provided, and
// ambiguous aliases that resolve to multiple canonical IDs.
//
// Severity levels in the returned AliasAuditFinding values are interpreted as
// follows: "error" for definite violations, "warning" for ambiguous alias
// mappings that require explicit canonical IDs, and "info" for expected
// informational states such as intentionally unsearchable aliases.
func AuditDefaultActionAliases(catalog *actioncatalog.Catalog) []AliasAuditFinding {
	return auditActionAliases(catalog, actionAliases())
}

// AuditCatalogDiscoveryTerms reports actions in dense catalog groups that have
// no discovery signal beyond their canonical ID/domain/action words. It is a
// deterministic audit helper for targeted metadata work, not a hard production
// validation gate.
func AuditCatalogDiscoveryTerms(catalog *actioncatalog.Catalog) []CatalogDiscoveryFinding {
	if catalog == nil {
		return nil
	}
	return AuditRegistryDiscoveryTerms(NewRegistryFromCatalog(catalog))
}

// AuditRegistryDiscoveryTerms reports actions in dense dynamic registry groups
// that have no discovery signal beyond their canonical ID/domain/action words.
// Use this variant when a caller already has a dynamic registry available.
func AuditRegistryDiscoveryTerms(registry *Registry) []CatalogDiscoveryFinding {
	if registry == nil {
		return nil
	}
	denseGroups := denseRegistryGroups(registry)
	if len(denseGroups) == 0 {
		return nil
	}

	findings := make([]CatalogDiscoveryFinding, 0)
	for _, entry := range registry.entries {
		if !denseGroups[entry.Tool] || hasActionDiscoverySignal(entry) {
			continue
		}
		findings = append(findings, CatalogDiscoveryFinding{
			Severity: "warning",
			Problem:  "weak_discovery_terms",
			Tool:     entry.Tool,
			Action:   entry.Action,
			ID:       entry.ID,
			Message:  "dense catalog action has no aliases, tags, usage guidance, related actions, parameter names, schema descriptions, or enum values beyond its canonical ID",
		})
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		if findings[i].Tool != findings[j].Tool {
			return findings[i].Tool < findings[j].Tool
		}
		return findings[i].ID < findings[j].ID
	})
	return findings
}

// denseRegistryGroups returns tool groups large enough that weak per-action
// discovery metadata can make adjacent actions hard to distinguish.
func denseRegistryGroups(registry *Registry) map[string]bool {
	const minDenseGroupActions = 8
	groupCounts := make(map[string]int)
	for _, entry := range registry.entries {
		groupCounts[entry.Tool]++
	}

	dense := make(map[string]bool)
	for toolName, count := range groupCounts {
		if count >= minDenseGroupActions {
			dense[toolName] = true
		}
	}
	return dense
}

// hasActionDiscoverySignal reports whether an action has any searchable signal
// beyond its derived canonical ID, domain, and action words.
func hasActionDiscoverySignal(entry actionEntry) bool {
	if len(entry.Aliases) > 0 || len(entry.Tags) > 0 || usageHintForEntry(entry) != "" || len(relatedActionsForEntry(entry)) > 0 {
		return true
	}
	document := entry.Document
	return len(entry.RequiredParams) > 0 ||
		len(document.OptionalParams) > 0 ||
		len(document.SchemaProperties) > 0 ||
		len(document.SchemaEnums) > 0 ||
		len(document.SchemaDescTerms) > 0
}

func auditActionAliases(catalog *actioncatalog.Catalog, aliases []actionAlias) []AliasAuditFinding {
	canonicalIDs := collectCanonicalIDs(catalog)
	findings, aliasTargets := detectAliasErrors(aliases, canonicalIDs, catalog != nil)
	findings = append(findings, detectAmbiguousAliases(aliasTargets)...)

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		if findings[i].Problem != findings[j].Problem {
			return findings[i].Problem < findings[j].Problem
		}
		return findings[i].Alias < findings[j].Alias
	})
	return findings
}

func collectCanonicalIDs(catalog *actioncatalog.Catalog) map[string]struct{} {
	canonicalIDs := make(map[string]struct{})
	if catalog == nil {
		return canonicalIDs
	}
	for _, group := range catalog.Groups() {
		for _, action := range group.ActionsInOrder() {
			canonicalIDs[string(action.ID)] = struct{}{}
		}
	}
	return canonicalIDs
}

func detectAliasErrors(aliases []actionAlias, canonicalIDs map[string]struct{}, validateCanonicalTarget bool) (findings []AliasAuditFinding, aliasTargets map[string][]string) {
	findings = make([]AliasAuditFinding, 0)
	seenPairs := make(map[string]struct{}, len(aliases))
	aliasTargets = make(map[string][]string)

	for _, alias := range aliases {
		pairKey := alias.Alias + aliasPairSeparator + alias.Canonical
		if _, ok := seenPairs[pairKey]; ok {
			findings = append(findings, aliasFinding("error", "duplicate_alias", alias, "duplicate alias/canonical pair"))
		}
		seenPairs[pairKey] = struct{}{}
		aliasTargets[alias.Alias] = append(aliasTargets[alias.Alias], alias.Canonical)

		if alias.Alias == alias.Canonical {
			findings = append(findings, aliasFinding("error", "alias_equals_canonical", alias, "alias must not equal its canonical action ID"))
		}
		if validateCanonicalTarget {
			if _, ok := canonicalIDs[alias.Canonical]; !ok {
				findings = append(findings, aliasFinding("error", "non_canonical_target", alias, "alias target is not present in the canonical action catalog"))
			}
		}
		if !alias.searchable() {
			findings = append(findings, aliasFinding("info", "unsearchable_alias", alias, "alias canonicalizes but is intentionally excluded from search ranking"))
		}
	}

	return findings, aliasTargets
}

func detectAmbiguousAliases(aliasTargets map[string][]string) []AliasAuditFinding {
	findings := make([]AliasAuditFinding, 0)
	for aliasName, targets := range aliasTargets {
		targets = dedupeSortedStrings(targets)
		if len(targets) > 1 {
			findings = append(findings, AliasAuditFinding{
				Severity: "warning",
				Problem:  "ambiguous_compatibility_alias",
				Alias:    aliasName,
				Message:  fmt.Sprintf("alias maps to multiple canonical actions: %v", targets),
			})
		}
	}
	return findings
}

func aliasFinding(severity, problem string, alias actionAlias, message string) AliasAuditFinding {
	return AliasAuditFinding{
		Severity:  severity,
		Problem:   problem,
		Alias:     alias.Alias,
		Canonical: alias.Canonical,
		Source:    string(alias.Source),
		Message:   message,
	}
}
