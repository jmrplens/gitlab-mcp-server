package main

import (
	"slices"
	"testing"
)

// buildTestOracle builds the real oracle once for the assertions below.
func buildTestOracle(t *testing.T) *oracle {
	t.Helper()
	ids, err := buildOracle()
	if err != nil {
		t.Fatalf("build oracle: %v", err)
	}
	return ids
}

// TestBuildOracle_TheWholeCatalog_IsInTheSet holds that the oracle is the
// catalog rather than a sample of it, since every judgement below rests on a
// missing ID meaning the catalog has none.
func TestBuildOracle_TheWholeCatalog_IsInTheSet(t *testing.T) {
	ids := buildTestOracle(t)

	if len(ids.ids) < 1000 {
		t.Fatalf("catalog IDs = %d, want the whole Ultimate catalog", len(ids.ids))
	}
	for _, id := range []string{"project.get", "issue.list", "group.get", "snippet.get", "snippet.project_get"} {
		t.Run(id, func(t *testing.T) {
			if !ids.isID(id) {
				t.Errorf("isID(%q) = false, want the catalog's own ID", id)
			}
		})
	}
	if ids.isID("project.no_such_action") {
		t.Error("isID reported an action the catalog does not have")
	}
	if !slices.IsSorted(ids.sorted) {
		t.Error("the suggestion list is unsorted, so a suggestion would depend on map order")
	}
	if len(ids.sorted) != len(ids.ids) {
		t.Errorf("suggestion list holds %d of %d IDs", len(ids.sorted), len(ids.ids))
	}
}

// TestBuildOracle_GitLabComOnlyFamily_Resolves is the reason the catalog is
// built twice. Orbit's group is contributed only when the client is
// GitLab.com, so an oracle built against a self-managed instance alone reports
// its six IDs as phantoms and a fixer deletes six working cross-links.
func TestBuildOracle_GitLabComOnlyFamily_Resolves(t *testing.T) {
	ids := buildTestOracle(t)

	for _, id := range []string{
		"orbit.status", "orbit.schema", "orbit.tools",
		"orbit.dsl", "orbit.query", "orbit.graph_status",
	} {
		t.Run(id, func(t *testing.T) {
			if !ids.isID(id) {
				t.Errorf("isID(%q) = false, want the GitLab.com catalog's ID", id)
			}
		})
	}
}

// TestBuildOracle_RegisteredAlias_IsAnAliasAndNotAnID holds the split the
// report rests on: an alias resolves at execution and is not a catalog ID, so
// it belongs in neither the ID set nor the findings.
func TestBuildOracle_RegisteredAlias_IsAnAliasAndNotAnID(t *testing.T) {
	ids := buildTestOracle(t)

	const alias = "kg.status"
	if ids.isID(alias) {
		t.Errorf("isID(%q) = true, want an alias rather than a catalog ID", alias)
	}
	canonical, isAlias := ids.alias(alias)
	if !isAlias {
		t.Fatalf("alias(%q) = false, want Orbit's declared alias", alias)
	}
	if canonical != "orbit.status" {
		t.Errorf("alias(%q) resolved to %q, want orbit.status", alias, canonical)
	}
}

// TestBuildOracle_EveryAlias_IsNotAlsoAnID holds what finish is for. An alias
// that is a canonical ID in its own right has to stay an ID, or a correct
// cross-link would be reported as an alias reference.
func TestBuildOracle_EveryAlias_IsNotAlsoAnID(t *testing.T) {
	ids := buildTestOracle(t)

	for alias := range ids.aliases {
		if _, isID := ids.ids[alias]; isID {
			t.Errorf("%q is recorded as both a catalog ID and an alias", alias)
		}
	}
}

// TestOracle_Domains_AreTheLeftHalfOfEveryID holds the set the prose rule
// filters on: without it a Usage line's github.com reads as a cross-link.
func TestOracle_Domains_AreTheLeftHalfOfEveryID(t *testing.T) {
	ids := buildTestOracle(t)

	for _, domain := range []string{"project", "issue", "group", "orbit"} {
		t.Run(domain, func(t *testing.T) {
			if !ids.hasDomain(domain) {
				t.Errorf("hasDomain(%q) = false, want a domain the catalog names", domain)
			}
		})
	}
	for _, domain := range []string{"github", "gitlab", "params", "filters"} {
		t.Run(domain, func(t *testing.T) {
			if ids.hasDomain(domain) {
				t.Errorf("hasDomain(%q) = true, want a token the prose rule turns away", domain)
			}
		})
	}
}

// TestNormalizeID_CaseAndSpacing_MatchTheRuntimeResolver holds that a
// judgement here and a lookup at run time cannot disagree: the dynamic
// registry lowercases and trims before it resolves an action.
func TestNormalizeID_CaseAndSpacing_MatchTheRuntimeResolver(t *testing.T) {
	cases := map[string]string{
		"Project.Get":   "project.get",
		"  issue.list ": "issue.list",
		"":              "",
		"group.get":     "group.get",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := normalizeID(input); got != want {
				t.Errorf("normalizeID(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestOracle_AddAlias_KeepsTheFirstTarget holds that a second catalog build
// recording the same alias does not move where it points, which is what makes
// the two builds a union rather than a race.
func TestOracle_AddAlias_KeepsTheFirstTarget(t *testing.T) {
	ids := &oracle{ids: map[string]struct{}{}, aliases: map[string]string{}, domains: map[string]struct{}{}}
	ids.addAlias("demo.alias", "demo.first")
	ids.addAlias("demo.alias", "demo.second")
	ids.addAlias("   ", "demo.blank")
	ids.finish()

	canonical, isAlias := ids.alias("demo.alias")
	if !isAlias || canonical != "demo.first" {
		t.Errorf("alias resolved to %q (found %t), want demo.first", canonical, isAlias)
	}
	if len(ids.aliases) != 1 {
		t.Errorf("aliases = %v, want the blank one dropped", ids.aliases)
	}
}
