package dynamic

import (
	"fmt"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actionregistry"
)

// AliasAuditFinding describes one dynamic alias governance finding.
type AliasAuditFinding struct {
	Severity  string
	Problem   string
	Alias     string
	Canonical string
	Source    string
	Message   string
}

// AuditDefaultActionAliases checks the built-in compatibility alias metadata.
func AuditDefaultActionAliases(catalog *actionregistry.Catalog) []AliasAuditFinding {
	return auditActionAliases(catalog, actionAliases())
}

func auditActionAliases(catalog *actionregistry.Catalog, aliases []actionAlias) []AliasAuditFinding {
	canonicalIDs := make(map[string]struct{})
	if catalog != nil {
		for _, group := range catalog.Groups() {
			for _, action := range group.ActionsInOrder() {
				canonicalIDs[string(action.ID)] = struct{}{}
			}
		}
	}

	findings := make([]AliasAuditFinding, 0)
	seenPairs := make(map[string]struct{}, len(aliases))
	aliasTargets := make(map[string][]string)
	for _, alias := range aliases {
		pairKey := alias.Alias + "\x00" + alias.Canonical
		if _, ok := seenPairs[pairKey]; ok {
			findings = append(findings, aliasFinding("error", "duplicate_alias", alias, "duplicate alias/canonical pair"))
		}
		seenPairs[pairKey] = struct{}{}
		aliasTargets[alias.Alias] = append(aliasTargets[alias.Alias], alias.Canonical)

		if alias.Alias == alias.Canonical {
			findings = append(findings, aliasFinding("error", "alias_equals_canonical", alias, "alias must not equal its canonical action ID"))
		}
		if _, ok := canonicalIDs[alias.Canonical]; catalog != nil && !ok {
			findings = append(findings, aliasFinding("error", "non_canonical_target", alias, "alias target is not present in the canonical action catalog"))
		}
		if !alias.searchable() {
			findings = append(findings, aliasFinding("info", "unsearchable_alias", alias, "alias canonicalizes but is intentionally excluded from search ranking"))
		}
	}

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
