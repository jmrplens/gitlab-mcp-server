package actionids

import (
	"slices"
	"testing"
)

// buildTestIDs builds the real catalog once for the assertions below.
func buildTestIDs(t *testing.T) *IDs {
	t.Helper()
	ids, err := Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return ids
}

// TestBuild_TheWholeCatalog_IsInTheSet holds that the set is the catalog
// rather than a sample of it, since every judgement a gate makes rests on a
// missing ID meaning the catalog has none.
func TestBuild_TheWholeCatalog_IsInTheSet(t *testing.T) {
	ids := buildTestIDs(t)

	if ids.Count() < 1000 {
		t.Fatalf("catalog IDs = %d, want the whole Ultimate catalog", ids.Count())
	}
	for _, id := range []string{"project.get", "issue.list", "group.get", "snippet.get", "snippet.project_get"} {
		t.Run(id, func(t *testing.T) {
			if !ids.IsID(id) {
				t.Errorf("IsID(%q) = false, want the catalog's own ID", id)
			}
		})
	}
	if ids.IsID("project.no_such_action") {
		t.Error("IsID reported an action the catalog does not have")
	}
	if !slices.IsSorted(ids.Sorted()) {
		t.Error("the suggestion list is unsorted, so a suggestion would depend on map order")
	}
	if len(ids.Sorted()) != ids.Count() {
		t.Errorf("suggestion list holds %d of %d IDs", len(ids.Sorted()), ids.Count())
	}
}

// TestBuild_GitLabComOnlyFamily_Resolves is the reason the catalog is built
// twice. Orbit's group is contributed only when the client is GitLab.com, so a
// set built against a self-managed instance alone reports its six IDs as
// phantoms and a gate would refuse six working cross-links.
func TestBuild_GitLabComOnlyFamily_Resolves(t *testing.T) {
	ids := buildTestIDs(t)

	for _, id := range []string{
		"orbit.status", "orbit.schema", "orbit.tools",
		"orbit.dsl", "orbit.query", "orbit.graph_status",
	} {
		t.Run(id, func(t *testing.T) {
			if !ids.IsID(id) {
				t.Errorf("IsID(%q) = false, want the GitLab.com catalog's ID", id)
			}
		})
	}
}

// TestBuild_RegisteredAlias_IsAnAliasAndNotAnID holds the split both gates
// rest on: an alias resolves at execution and is not a catalog ID, so a rule
// that demands a canonical ID has to be able to tell them apart.
func TestBuild_RegisteredAlias_IsAnAliasAndNotAnID(t *testing.T) {
	ids := buildTestIDs(t)

	const alias = "kg.status"
	if ids.IsID(alias) {
		t.Errorf("IsID(%q) = true, want an alias rather than a catalog ID", alias)
	}
	canonical, isAlias := ids.Alias(alias)
	if !isAlias {
		t.Fatalf("Alias(%q) = false, want Orbit's declared alias", alias)
	}
	if canonical != "orbit.status" {
		t.Errorf("Alias(%q) resolved to %q, want orbit.status", alias, canonical)
	}
}

// TestBuild_EveryAlias_IsNotAlsoAnID holds what finish is for. An alias that
// is a canonical ID in its own right has to stay an ID, or a correct
// cross-link would be refused as an alias reference.
func TestBuild_EveryAlias_IsNotAlsoAnID(t *testing.T) {
	ids := buildTestIDs(t)

	for alias := range ids.Aliases() {
		if ids.IsID(alias) {
			t.Errorf("%q is recorded as both a catalog ID and an alias", alias)
		}
	}
}

// TestIDs_Domains_AreTheLeftHalfOfEveryID holds the set the prose rule filters
// on: without it a sentence's github.com reads as a cross-link.
func TestIDs_Domains_AreTheLeftHalfOfEveryID(t *testing.T) {
	ids := buildTestIDs(t)

	for _, domain := range []string{"project", "issue", "group", "orbit"} {
		t.Run(domain, func(t *testing.T) {
			if !ids.HasDomain(domain) {
				t.Errorf("HasDomain(%q) = false, want a domain the catalog names", domain)
			}
		})
	}
	for _, domain := range []string{"github", "gitlab", "params", "filters"} {
		t.Run(domain, func(t *testing.T) {
			if ids.HasDomain(domain) {
				t.Errorf("HasDomain(%q) = true, want a token the prose rule turns away", domain)
			}
		})
	}
}

// TestIDs_Members_AreTheRightHalfOfEveryID holds the other half of the prose
// rule, which is the half that sees the commonest defect: an action the
// catalog really has, spelled under a domain it does not.
func TestIDs_Members_AreTheRightHalfOfEveryID(t *testing.T) {
	ids := buildTestIDs(t)

	if !ids.HasMember("get") || !ids.HasMember("list") {
		t.Error("HasMember does not know the verbs every domain spells")
	}
	if ids.HasMember("no_such_action_anywhere") {
		t.Error("HasMember reported an action name the catalog never spells")
	}
}

// TestNormalize_CaseAndSpacing_MatchTheRuntimeResolver holds that a judgement
// in a gate and a lookup at run time cannot disagree: the dynamic registry
// lowercases and trims before it resolves an action.
func TestNormalize_CaseAndSpacing_MatchTheRuntimeResolver(t *testing.T) {
	cases := map[string]string{
		"Project.Get":   "project.get",
		"  issue.list ": "issue.list",
		"":              "",
		"group.get":     "group.get",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := Normalize(input); got != want {
				t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestNew_ExplicitSet_AppliesTheSameTwoRules holds that a set built from a
// literal is the same kind of thing as one read from the catalog: the halves
// are recorded and an alias that is an ID stays an ID.
func TestNew_ExplicitSet_AppliesTheSameTwoRules(t *testing.T) {
	ids := New(
		[]string{"demo.get", "demo.list", "  ", "Demo.Update"},
		map[string]string{"demo.fetch": "demo.get", "demo.list": "demo.list", "": "demo.get"},
	)

	if !ids.IsID("demo.update") {
		t.Error("New did not normalize the ID it was given")
	}
	if ids.Count() != 3 {
		t.Errorf("Count = %d, want the blank entry dropped", ids.Count())
	}
	if canonical, isAlias := ids.Alias("demo.fetch"); !isAlias || canonical != "demo.get" {
		t.Errorf("Alias(demo.fetch) = %q (found %t), want demo.get", canonical, isAlias)
	}
	if _, isAlias := ids.Alias("demo.list"); isAlias {
		t.Error("an alias that is also a canonical ID was kept as an alias")
	}
	if !ids.HasDomain("demo") || !ids.HasMember("get") {
		t.Error("New recorded no halves, so the prose rule would turn every token away")
	}
}

// TestIDs_AddAlias_KeepsTheFirstTarget holds that a second catalog build
// recording the same alias does not move where it points, which is what makes
// the two builds a union rather than a race.
func TestIDs_AddAlias_KeepsTheFirstTarget(t *testing.T) {
	ids := New(nil, nil)
	ids.addAlias("demo.alias", "demo.first")
	ids.addAlias("demo.alias", "demo.second")
	ids.addAlias("   ", "demo.blank")
	ids.finish()

	canonical, isAlias := ids.Alias("demo.alias")
	if !isAlias || canonical != "demo.first" {
		t.Errorf("Alias resolved to %q (found %t), want demo.first", canonical, isAlias)
	}
	if ids.AliasCount() != 1 {
		t.Errorf("AliasCount = %d, want the blank one dropped", ids.AliasCount())
	}
}

// TestIDs_AddCatalog_NilCatalog_IsNoBuild holds that a build that produced
// nothing contributes nothing rather than panicking, since the union takes two
// builds and either may be the one that failed to carry a group.
func TestIDs_AddCatalog_NilCatalog_IsNoBuild(t *testing.T) {
	ids := New([]string{"demo.get"}, nil)
	ids.addCatalog(nil)

	if ids.Count() != 1 {
		t.Errorf("Count = %d, want the one ID the set was built with", ids.Count())
	}
}

// TestIDs_Candidates_EitherHalfKnown_Qualifies holds the rule that decides
// what a sentence offers as an action ID, on the four shapes that matter: a
// real ID, a real action under an invented domain (the commonest defect), a
// token neither half of which the catalog uses, and a repeat.
func TestIDs_Candidates_EitherHalfKnown_Qualifies(t *testing.T) {
	ids := New([]string{"issue.list", "issue.get", "repository.commit_list"}, nil)

	cases := []struct {
		name  string
		prose string
		want  []string
	}{
		{"a canonical ID", "Call issue.list first.", []string{"issue.list"}},
		{"a real action under an invented domain", "Resolve SHAs with commit.list.", []string{"commit.list"}},
		{"neither half known", "See github.com and go.mod.", nil},
		{"one token twice", "issue.get, then issue.get again.", []string{"issue.get"}},
		{"order is the order of the sentence", "issue.get after issue.list", []string{"issue.get", "issue.list"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ids.Candidates(testCase.prose); !slices.Equal(got, testCase.want) {
				t.Errorf("Candidates(%q) = %v, want %v", testCase.prose, got, testCase.want)
			}
		})
	}
}

// TestIDs_Closest_NearMissAndDistantMiss holds what a suggestion is for: a
// lead when the ID is one letter off, and silence when the nearest is an
// accident of the alphabet.
func TestIDs_Closest_NearMissAndDistantMiss(t *testing.T) {
	ids := New([]string{"demo.get", "demo.list"}, nil)

	if got := ids.Closest("demo.gt"); got != "demo.get" {
		t.Errorf("Closest for a near miss = %q, want demo.get", got)
	}
	if got := ids.Closest("unrelated.something_entirely_else"); got != "" {
		t.Errorf("Closest for a distant ID = %q, want nothing", got)
	}
}
