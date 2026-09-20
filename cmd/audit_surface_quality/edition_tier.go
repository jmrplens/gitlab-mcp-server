// The edition-tier rule: the licensing tier a tool's own description states,
// against the tier the surface registers that tool at.

package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// editionTierCategory is the rule family an edition-tier violation is
// grouped under in the report.
const editionTierCategory = "edition-tier"

// editionTierParenthetical matches the parenthetical a served description
// states its tier in. Only the first sentence is read, since a tier named
// later in a description belongs to a field or to a related tool.
var editionTierParenthetical = regexp.MustCompile(`\(([^)]*)\)`)

// tierClaims maps the phrases a description opens a tier claim with to the
// minimum tier each one names. The spellings are the ones the surface uses;
// a phrase outside this table is not read as a claim, which is why
// "available on all tiers (Free, Premium, Ultimate)" states nothing here.
var tierClaims = map[string]edition.Tier{
	"premium":          edition.Premium,
	"premium+":         edition.Premium,
	"premium/ultimate": edition.Premium,
	"ultimate":         edition.Ultimate,
	"ultimate only":    edition.Ultimate,
}

// auditEditionTier compares the tier each individual tool's description
// states with the lowest tier the surface serves that tool at.
//
// It reads the registered surface three times rather than an ActionSpec's
// Edition field, because the field is not what a client is gated by: the
// catalog aggregation assigns a tier over whole domains and overwrites
// whatever a spec declared, so a spec can disagree with its own description
// and with the license table and nothing observes it. What this reads is the
// answer a Free, a Premium and an Ultimate instance actually get.
//
// A disagreement is a defect either way round. A tool served below the tier
// its description names is one whose every call GitLab refuses with a license
// error, and a tool served above it is one a licensed instance is told it
// cannot have.
//
// The prose form ("Requires an Ultimate license") is deliberately not read
// here. Reading it today reports the three group Datadog tools, which are
// served at Free and state they need Premium: that is a question about where
// group-level integrations are gated rather than a defect in a description,
// and a gate introduced red is a gate that gets switched off.
func auditEditionTier(client *gitlabclient.Client) []violation {
	return editionTierViolations(listIndividualTools(client, edition.Ultimate), servedTiers(client))
}

// servedTiers maps each individual tool to the lowest tier the surface
// serves it at. The tiers nest, so listing from the widest down and letting
// each listing overwrite the last leaves the lowest.
func servedTiers(client *gitlabclient.Client) map[string]edition.Tier {
	servedAt := map[string]edition.Tier{}
	for _, tier := range []edition.Tier{edition.Ultimate, edition.Premium, edition.Free} {
		for _, t := range listIndividualTools(client, tier) {
			servedAt[t.Name] = tier
		}
	}
	return servedAt
}

// editionTierViolations is the comparison itself, over a listing and the
// tier each of its tools is served from.
func editionTierViolations(tools []*mcp.Tool, servedAt map[string]edition.Tier) []violation {
	var vs []violation
	for _, t := range tools {
		claimed, phrase, stated := tierClaim(t.Description)
		if !stated {
			continue
		}
		served := servedAt[t.Name]
		if served == claimed {
			continue
		}
		vs = append(vs, violation{
			t.Name, editionTierCategory,
			fmt.Sprintf("the description states %q and the surface serves the tool from %s", phrase, served),
		})
	}
	return vs
}

// tierClaim reads the minimum tier a served description states, with the
// phrase it was read from.
//
// Only a parenthetical of the first sentence counts, and only one whose
// leading clause is a tier phrase: "(Premium/Ultimate, destructive)" states
// Premium, and "(Premium/Ultimate iteration list type)" states nothing,
// being about the shape of a parameter rather than about the tool.
func tierClaim(description string) (tier edition.Tier, phrase string, stated bool) {
	for _, match := range editionTierParenthetical.FindAllStringSubmatch(firstSentence(description), -1) {
		clause, _, _ := strings.Cut(match[1], ",")
		clause = strings.TrimSpace(clause)
		if claimed, ok := tierClaims[strings.ToLower(clause)]; ok {
			return claimed, clause, true
		}
	}
	return edition.Free, "", false
}

// firstSentence returns the description up to its first sentence end, which
// is where the convention puts the tier claim.
func firstSentence(description string) string {
	first, _, found := strings.Cut(description, ". ")
	if found {
		return first
	}
	return description
}
