// edition_tier_test.go covers the edition-tier rule: the tier phrases a
// served description states, the sentence they have to be stated in, and the
// comparison with the tier the surface actually serves each tool at.
package main

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TestTierClaim_Descriptions_ReadTheMinimumTier covers each spelling the
// surface uses, and the three shapes that state no tier: a parenthetical
// about a parameter, a list of every tier, and a claim outside the first
// sentence.
func TestTierClaim_Descriptions_ReadTheMinimumTier(t *testing.T) {
	cases := []struct {
		name        string
		description string
		want        edition.Tier
		stated      bool
	}{
		{
			name:        "ultimate",
			description: "Read a project's security settings (Ultimate). Returns: the settings.",
			want:        edition.Ultimate, stated: true,
		},
		{
			name:        "ultimate only",
			description: "Read the instance license (Ultimate only). Returns: the license.",
			want:        edition.Ultimate, stated: true,
		},
		{
			name:        "premium",
			description: "Evaluate a package (Premium, experimental). Returns: the verdict.",
			want:        edition.Premium, stated: true,
		},
		{
			name:        "premium and up",
			description: "List merge trains (Premium+). Returns: the trains.",
			want:        edition.Premium, stated: true,
		},
		{
			name:        "premium or ultimate",
			description: "Delete a group issue board (Premium/Ultimate, destructive). Returns: a summary.",
			want:        edition.Premium, stated: true,
		},
		{
			name:        "a parenthetical about a parameter states nothing",
			description: "List iterations (Premium/Ultimate iteration list type). Returns: the iterations.",
		},
		{
			name:        "every tier listed states nothing",
			description: "Read the topics, available on all tiers (Free, Premium, Ultimate). Returns: the topics.",
		},
		{
			name:        "a claim after the first sentence states nothing",
			description: "List a project's variables. Group variables need a license (Premium/Ultimate).",
		},
		{
			name:        "no parenthetical at all",
			description: "List a project's branches. Returns: the branches.",
		},
		{
			// Every parenthetical of the sentence is read, not the first alone.
			name:        "a claim after another parenthetical",
			description: "Add a note (or a reply) to an epic (Premium). Returns: the note.",
			want:        edition.Premium, stated: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, phrase, stated := tierClaim(tc.description)
			if stated != tc.stated {
				t.Fatalf("tierClaim() stated = %v (%q), want %v", stated, phrase, tc.stated)
			}
			if stated && got != tc.want {
				t.Errorf("tierClaim() = %s (%q), want %s", got, phrase, tc.want)
			}
		})
	}
}

// TestFirstSentence_DescriptionWithNoSentenceEnd_IsReadWhole checks the one
// shape the sentence split has to leave alone.
func TestFirstSentence_DescriptionWithNoSentenceEnd_IsReadWhole(t *testing.T) {
	const description = "List merge trains (Premium)"
	if got := firstSentence(description); got != description {
		t.Errorf("firstSentence() = %q, want the whole description", got)
	}
}

// TestEditionTierViolations_Disagreement_IsReportedEitherWayRound drives the
// comparison with a listing of its own, which is the only way to see a
// disagreement: the served surface has none, and a rule whose reporting
// branch no test reaches is a rule nobody has watched work.
func TestEditionTierViolations_Disagreement_IsReportedEitherWayRound(t *testing.T) {
	tools := []*mcp.Tool{
		{Name: "gitlab_agrees", Description: "Toggle secret push protection (Ultimate). Returns: the settings."},
		{Name: "gitlab_served_too_low", Description: "Toggle secret push protection (Ultimate). Returns: the settings."},
		{Name: "gitlab_served_too_high", Description: "List merge trains (Premium). Returns: the trains."},
		{Name: "gitlab_states_nothing", Description: "List branches. Returns: the branches."},
	}
	servedAt := map[string]edition.Tier{
		"gitlab_agrees":          edition.Ultimate,
		"gitlab_served_too_low":  edition.Premium,
		"gitlab_served_too_high": edition.Ultimate,
		"gitlab_states_nothing":  edition.Free,
	}

	got := editionTierViolations(tools, servedAt)

	// The detail names the phrase as the description spelled it and the tier
	// as the surface spells it, which is what tells the two apart in a
	// message that would still read as a disagreement the other way round.
	want := []violation{
		{"gitlab_served_too_low", editionTierCategory, `the description states "Ultimate" and the surface serves the tool from premium`},
		{"gitlab_served_too_high", editionTierCategory, `the description states "Premium" and the surface serves the tool from ultimate`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("editionTierViolations() = %+v, want %+v", got, want)
	}
}

// TestServedTiers_EveryTool_IsMappedToItsLowestTier checks the map the
// comparison reads: a Free tool is recorded at Free although the Ultimate
// listing carries it too.
func TestServedTiers_EveryTool_IsMappedToItsLowestTier(t *testing.T) {
	client := stubClient(t)
	servedAt := servedTiers(client)

	ultimate := listIndividualTools(client, edition.Ultimate)
	if len(servedAt) != len(ultimate) {
		t.Fatalf("servedTiers() mapped %d tool(s), want the %d the widest tier serves", len(servedAt), len(ultimate))
	}
	free := 0
	for _, tool := range listIndividualTools(client, edition.Free) {
		if servedAt[tool.Name] != edition.Free {
			t.Errorf("%s is served at Free and mapped to %s", tool.Name, servedAt[tool.Name])
		}
		free++
	}
	if free == 0 {
		t.Error("the Free listing is empty, so nothing was checked")
	}
}

// TestAuditEditionTier_ServedSurface_AgreesWithEveryDescription is the rule
// over the real surface: no individual tool states a tier the surface does
// not serve it from.
//
// It also asserts the rule is not vacuous. A phrase table that matched
// nothing would report nothing and read exactly like a surface with no
// disagreement, so the descriptions that do state a tier are counted here
// and the count has to be substantial.
func TestAuditEditionTier_ServedSurface_AgreesWithEveryDescription(t *testing.T) {
	client := stubClient(t)

	if got := auditEditionTier(client); len(got) != 0 {
		t.Errorf("auditEditionTier() = %+v, want no violation", got)
	}

	claims := map[edition.Tier]int{}
	for _, tool := range listIndividualTools(client, edition.Ultimate) {
		if claimed, _, stated := tierClaim(tool.Description); stated {
			claims[claimed]++
		}
	}
	t.Logf("edition-tier: %d description(s) state Premium, %d state Ultimate", claims[edition.Premium], claims[edition.Ultimate])
	if claims[edition.Premium] == 0 || claims[edition.Ultimate] == 0 {
		t.Errorf("the phrase table read %d Premium and %d Ultimate claim(s); a rule that reads none can never fire",
			claims[edition.Premium], claims[edition.Ultimate])
	}
}

// TestListIndividualTools_LowerTiers_AreSubsetsOfTheWidest checks the
// property the rule reads a served tier off: the surface nests, so the tier
// a tool first appears at is the minimum an instance needs for it.
func TestListIndividualTools_LowerTiers_AreSubsetsOfTheWidest(t *testing.T) {
	client := stubClient(t)
	names := func(tools []*mcp.Tool) map[string]bool {
		out := make(map[string]bool, len(tools))
		for _, tool := range tools {
			out[tool.Name] = true
		}
		return out
	}

	free := names(listIndividualTools(client, edition.Free))
	premium := names(listIndividualTools(client, edition.Premium))
	ultimate := names(listIndividualTools(client, edition.Ultimate))

	if len(free) == 0 || len(free) >= len(premium) || len(premium) >= len(ultimate) {
		t.Fatalf("surface sizes are %d free, %d premium, %d ultimate; want each tier to add tools", len(free), len(premium), len(ultimate))
	}
	for name := range free {
		if !premium[name] {
			t.Errorf("%s is served at Free and not at Premium", name)
		}
	}
	for name := range premium {
		if !ultimate[name] {
			t.Errorf("%s is served at Premium and not at Ultimate", name)
		}
	}
}
