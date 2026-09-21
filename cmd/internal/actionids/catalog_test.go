package actionids

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncompat"
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

// TestBuild_EachClientCatalog_IsFoldedIntoTheUnion holds why Build runs twice.
// Nothing observes the loss of one build while the GitLab.com catalog covers
// the self-managed one, so the union is stated here as a property rather than
// left to a count: the day an action is gated the other way, this is what
// fails.
func TestBuild_EachClientCatalog_IsFoldedIntoTheUnion(t *testing.T) {
	union := buildTestIDs(t)

	selfManaged, cleanup := mcpsurface.NewStubClient()
	defer cleanup()
	for name, client := range map[string]*gitlabclient.Client{
		"self-managed": selfManaged,
		"gitlab.com":   mcpsurface.NewGitLabComClient(),
	} {
		t.Run(name, func(t *testing.T) {
			catalog, err := catalogFor(client)
			if err != nil {
				t.Fatalf("catalogFor: %v", err)
			}
			actions := catalog.Actions()
			if len(actions) == 0 {
				t.Fatal("this build contributed no actions, so the union would be built from nothing")
			}
			for _, action := range actions {
				if !union.IsID(string(action.ID)) {
					t.Errorf("IsID(%q) = false, want every action of this build in the union", action.ID)
				}
			}
		})
	}
}

// TestBuild_EveryCatalogActionID_IsNonEmpty holds the property that makes
// addCatalog's empty-ID guard unreachable: the catalog composes an ID from a
// domain and a required action name, so nothing it hands over normalizes away
// and no alias is ever recorded against nothing.
func TestBuild_EveryCatalogActionID_IsNonEmpty(t *testing.T) {
	selfManaged, cleanup := mcpsurface.NewStubClient()
	defer cleanup()

	catalog, err := catalogFor(selfManaged)
	if err != nil {
		t.Fatalf("catalogFor: %v", err)
	}
	for _, action := range catalog.Actions() {
		if Normalize(string(action.ID)) == "" {
			t.Errorf("action %q of tool %q has an ID that normalizes to nothing", action.Name, action.ToolName)
		}
	}
}

// TestBuild_EveryCompatibilityAlias_ResolvesToItsCanonical holds the oracle's
// claim about the historical aliases: each one resolves, and what it resolves
// to is an action the catalog really holds. The catalog and Build record these
// twice over, so the property is asserted rather than either path.
func TestBuild_EveryCompatibilityAlias_ResolvesToItsCanonical(t *testing.T) {
	ids := buildTestIDs(t)

	aliases := actioncompat.ActionAliases()
	if len(aliases) == 0 {
		t.Fatal("the compatibility table is empty, so this test asserts nothing")
	}
	for _, alias := range aliases {
		t.Run(alias.Alias, func(t *testing.T) {
			if !ids.IsID(alias.Canonical) {
				t.Errorf("%q names canonical %q, which the catalog does not hold", alias.Alias, alias.Canonical)
			}
			if ids.IsID(alias.Alias) {
				return
			}
			canonical, isAlias := ids.Alias(alias.Alias)
			if !isAlias {
				t.Fatalf("Alias(%q) = false, want the compatibility table's alias", alias.Alias)
			}
			if canonical != Normalize(alias.Canonical) {
				t.Errorf("Alias(%q) = %q, want %q", alias.Alias, canonical, Normalize(alias.Canonical))
			}
		})
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

// TestIDs_AddID_MalformedID_RecordsNoHalves holds what the prose rule is
// allowed to filter on. A value with no separator, or with an empty half, is
// still kept as an ID because the catalog gave it, and contributes nothing to
// the domain and member sets: admitting "nodot" as a domain would make every
// bare word in a sentence read as a cross-link.
func TestIDs_AddID_MalformedID_RecordsNoHalves(t *testing.T) {
	cases := []struct {
		name      string
		id        string
		domain    string
		member    string
		hasDomain bool
		hasMember bool
	}{
		{"no separator at all", "nodot", "nodot", "", false, false},
		{"an empty domain half", ".get", "", "get", false, false},
		{"an empty member half", "demo.", "demo", "", true, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ids := New([]string{testCase.id}, nil)

			if !ids.IsID(testCase.id) {
				t.Fatalf("IsID(%q) = false, want the value the catalog gave", testCase.id)
			}
			if got := ids.HasDomain(testCase.domain); got != testCase.hasDomain {
				t.Errorf("HasDomain(%q) = %t, want %t", testCase.domain, got, testCase.hasDomain)
			}
			if got := ids.HasMember(testCase.member); got != testCase.hasMember {
				t.Errorf("HasMember(%q) = %t, want %t", testCase.member, got, testCase.hasMember)
			}
		})
	}
}

// TestIDs_AddID_ReturnsTheNormalizedSpelling holds the contract addCatalog
// reads: the returned spelling is what an action's aliases are recorded
// against, so a value that arrives with capitals or spacing comes back the way
// the dynamic registry resolves one, and a value that is no ID comes back
// empty.
func TestIDs_AddID_ReturnsTheNormalizedSpelling(t *testing.T) {
	ids := New(nil, nil)

	if got := ids.addID("  Demo.Update  "); got != "demo.update" {
		t.Errorf("addID = %q, want demo.update", got)
	}
	if got := ids.addID("   "); got != "" {
		t.Errorf("addID of a blank value = %q, want the empty string", got)
	}
}

// TestIDs_AddAlias_TargetIsNormalized holds that what an alias resolves to is
// spelled the way a canonical ID is. A caller puts the answer back through
// IsID, so a target kept as it was written would miss on case or spacing and
// read as a cross-link to nothing.
func TestIDs_AddAlias_TargetIsNormalized(t *testing.T) {
	ids := New([]string{"demo.first"}, map[string]string{"  Demo.Alias ": "  Demo.First  "})

	canonical, isAlias := ids.Alias("demo.alias")
	if !isAlias {
		t.Fatal("Alias(demo.alias) = false, want the alias the set was built with")
	}
	if canonical != "demo.first" {
		t.Errorf("Alias(demo.alias) = %q, want demo.first", canonical)
	}
	if !ids.IsID(canonical) {
		t.Errorf("the resolved target %q is no canonical ID, so a caller could not follow it", canonical)
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

// TestIDs_Closest_ADistanceEqualToTheBound_IsStillALead holds that the bound is
// inclusive. A candidate exactly at it is the furthest one worth naming, and
// dropping it would silence the lead a reader gets from a two-character typo,
// which is the commonest one there is.
func TestIDs_Closest_ADistanceEqualToTheBound_IsStillALead(t *testing.T) {
	ids := New([]string{"demo.get"}, nil)

	if got := ids.Closest("demo.g"); got != "demo.get" {
		t.Errorf("Closest(demo.g) = %q, want demo.get at exactly the bound", got)
	}
}

// TestIDs_Closest_TwoCandidatesWithinTheBound_PicksTheNearest holds both halves
// of the comparison a second candidate goes through: a strictly nearer one
// replaces the leader, and a tie keeps whichever the sorted list reached first,
// which is what makes a suggestion the same on every run.
func TestIDs_Closest_TwoCandidatesWithinTheBound_PicksTheNearest(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		id   string
		want string
	}{
		{"a later candidate that is strictly nearer wins", []string{"demo.aet", "demo.get"}, "demo.get", "demo.get"},
		{"a tie keeps the first in sorted order", []string{"demo.aget", "demo.bget"}, "demo.xget", "demo.aget"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := New(testCase.ids, nil).Closest(testCase.id); got != testCase.want {
				t.Errorf("Closest(%q) = %q, want %q", testCase.id, got, testCase.want)
			}
		})
	}
}

// TestEditDistance_KnownDistances_AreLevenshtein holds the measure Closest
// ranks by against distances that are the same in any textbook. Every
// suggestion the two gates print rests on it, and a wrong distance is silent:
// nothing downstream can tell one from a catalog that really lacks the ID.
func TestEditDistance_KnownDistances_AreLevenshtein(t *testing.T) {
	cases := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{"both empty", "", "", 0},
		{"everything inserted", "", "abc", 3},
		{"everything deleted", "abc", "", 3},
		{"identical", "abc", "abc", 0},
		{"one substitution", "a", "b", 1},
		{"kitten and sitting", "kitten", "sitting", 3},
		{"flaw and lawn", "flaw", "lawn", 2},
		{"an action ID missing one letter", "issue.list", "issue.lst", 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := editDistance(testCase.left, testCase.right); got != testCase.want {
				t.Errorf("editDistance(%q, %q) = %d, want %d", testCase.left, testCase.right, got, testCase.want)
			}
		})
	}
}

// TestNewWithTools_ToolID_AnswersOnlyForANameThatProjectsAnID holds what the
// tool map is for and the one invariant that makes it worth having.
//
// A rule that refuses a gitlab_* name in served prose has to be able to say
// what should have been written there, and the individual surface is one tool
// per action, so the catalog can. What comes out is always a canonical ID: a
// tool naming something this set does not hold is dropped rather than
// recorded, because a caller that had to re-check the answer would be keeping
// the invariant itself.
func TestNewWithTools_ToolID_AnswersOnlyForANameThatProjectsAnID(t *testing.T) {
	ids := NewWithTools(
		[]string{"demo.get", "demo.list"},
		map[string]string{"demo.fetch": "demo.get"},
		map[string]string{
			"gitlab_fetch_demo": "demo.get",
			"gitlab_demo_list":  "demo.list",
			"gitlab_gone":       "demo.vanished",
			"  ":                "demo.get",
			"gitlab_no_id":      "",
		},
	)

	cases := []struct {
		name  string
		tool  string
		id    string
		known bool
	}{
		{name: "a declared name that is not derivable", tool: "gitlab_fetch_demo", id: "demo.get", known: true},
		{name: "a name that reads like its ID", tool: "gitlab_demo_list", id: "demo.list", known: true},
		{name: "surrounding space is not part of a name", tool: "  gitlab_fetch_demo  ", id: "demo.get", known: true},
		{name: "a tool whose ID the catalog does not hold", tool: "gitlab_gone"},
		{name: "a tool with no ID at all", tool: "gitlab_no_id"},
		{name: "a name nothing registers", tool: "gitlab_unheard_of"},
		{name: "the empty name", tool: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			id, known := ids.ToolID(testCase.tool)
			if known != testCase.known || id != testCase.id {
				t.Errorf("ToolID(%q) = %q, %v; want %q, %v", testCase.tool, id, known, testCase.id, testCase.known)
			}
		})
	}
	if ids.ToolCount() != 2 {
		t.Errorf("ToolCount() = %d, want the two names that project an ID this set holds", ids.ToolCount())
	}
}

// TestNewWithTools_OneNameRecordedTwice_KeepsTheFirst holds the rule an alias
// is held to as well, and for the same reason: the catalog is built twice,
// self-managed and GitLab.com, and the two overlap almost entirely, so a
// second pass would otherwise rewrite every entry with the same value.
func TestNewWithTools_OneNameRecordedTwice_KeepsTheFirst(t *testing.T) {
	ids := NewWithTools([]string{"demo.get", "demo.list"}, nil, nil)
	ids.addTool("gitlab_demo", "demo.get")
	ids.addTool("gitlab_demo", "demo.list")

	if id, _ := ids.ToolID("gitlab_demo"); id != "demo.get" {
		t.Errorf("ToolID() = %q, want the first recording kept", id)
	}
}

// TestNew_WithoutTools_AnswersNothingAboutThem holds that the plain
// constructor is the same set without the map, so a caller that never asked
// about tool names gets a clean "no" rather than a nil map dereference.
func TestNew_WithoutTools_AnswersNothingAboutThem(t *testing.T) {
	ids := New([]string{"demo.get"}, nil)

	if id, known := ids.ToolID("gitlab_demo_get"); known || id != "" {
		t.Errorf("ToolID() = %q, %v; want nothing known", id, known)
	}
	if ids.ToolCount() != 0 {
		t.Errorf("ToolCount() = %d, want none", ids.ToolCount())
	}
}
