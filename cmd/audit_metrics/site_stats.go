// site_stats.go generates the single-sourced statistics file consumed by the
// Astro Starlight documentation site (site/src/data/stats.json). Every value is
// derived from the canonical action catalog projection, the in-memory MCP
// surface, or the repository VERSION file — never hardcoded — so the published
// numbers cannot drift away from the real server surface.
//
// The same per-surface, per-tier derivations used by the text report drive
// these counts:
//
//   - tools.*           individual tool surface via [mcpsurface.IndividualTools] per tier
//   - meta.*            meta-tool surface via [mcpsurface.MetaTools] per tier and instance
//   - dynamic           the fixed find/execute dynamic surface (2 tools)
//   - catalog_actions.* dynamic catalog action routes per tier and instance
//   - catalog_groups.*  catalog group count per tier (IncludeMCP)
//   - resources/prompts registered MCP resource and prompt counts
//   - tool_packages     Go package directories under internal/tools

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// siteCapabilities is the number of MCP protocol capabilities this server
// implements: completions, progress, elicitation, and resource
// subscriptions. Capabilities are wired individually in cmd/server rather
// than through an enumerable registry, so this value mirrors the canonical
// count the site's capability overview states in both languages ("the 4 MCP
// capabilities", "las 4 capacidades MCP"). It is pinned by
// TestSiteStatsCapabilitiesMatchesDocs so it cannot silently drift, which is
// exactly how the site's capability count went stale when the fourth
// capability shipped.
const siteCapabilities = 4

// siteCompletionArgNames is the number of distinct completion argument types
// the server supports. The completion handler dispatches argument types through
// a switch in internal/completions rather than an enumerable registry, so this
// value mirrors the canonical count the site's completions page states in both
// languages ("completes 18 argument names", "completa 18 nombres de
// argumento"). It is pinned by TestSiteStatsCompletionsMatchesDocs so it cannot
// silently drift.
const siteCompletionArgNames = 18

// siteStats is the single-sourced statistics payload written to
// site/src/data/stats.json and imported by the documentation MDX pages.
type siteStats struct {
	Version        string             `json:"version"`
	Tools          siteToolCounts     `json:"tools"`
	Meta           siteMetaCounts     `json:"meta"`
	Dynamic        int                `json:"dynamic"`
	CatalogActions siteCatalogActions `json:"catalog_actions"`
	CatalogGroups  siteCatalogGroups  `json:"catalog_groups"`
	Resources      int                `json:"resources"`
	Prompts        int                `json:"prompts"`
	Completions    int                `json:"completions"`
	Capabilities   int                `json:"capabilities"`
	ToolPackages   int                `json:"tool_packages"`
}

// siteToolCounts holds the individual-tool surface size per licensing tier.
type siteToolCounts struct {
	Free                int `json:"free"`
	Premium             int `json:"premium"`
	UltimateSelfManaged int `json:"ultimate_self_managed"`
	GitLabCom           int `json:"gitlab_com"`
}

// siteMetaCounts holds the meta-tool surface size per tier and instance.
//
// Base is Free/CE, SelfManagedEnterprise and GitLabCom are Ultimate. Premium
// is measured on both instances because they differ there: GitLab.com serves
// the Orbit group from Premium up, and no self-managed instance serves it.
type siteMetaCounts struct {
	Base                  int `json:"base"`
	Premium               int `json:"premium"`
	SelfManagedEnterprise int `json:"self_managed_enterprise"`
	GitLabComPremium      int `json:"gitlab_com_premium"`
	GitLabCom             int `json:"gitlab_com"`
}

// siteCatalogActions holds the dynamic catalog action-route count per tier
// and instance, with the same split as [siteMetaCounts].
type siteCatalogActions struct {
	Free                  int `json:"free"`
	Premium               int `json:"premium"`
	SelfManagedEnterprise int `json:"self_managed_enterprise"`
	GitLabComPremium      int `json:"gitlab_com_premium"`
	GitLabCom             int `json:"gitlab_com"`
}

// siteCatalogGroups holds the catalog group count per licensing tier.
type siteCatalogGroups struct {
	Free     int `json:"free"`
	Premium  int `json:"premium"`
	Ultimate int `json:"ultimate"`
}

// generateSiteStats derives every published statistic from the live MCP
// surface, the canonical catalog, and the VERSION file. client is a
// self-managed instance client; gitLabComClient targets GitLab.com so the
// GitLab.com-only Orbit tools are included where relevant.
//
// Only the VERSION read can fail, and it ends the payload: half a stats file
// is not worth publishing, and the caller is where that is reported. Every
// other value is measured off surfaces this process registers in memory from
// the catalog compiled into it, which is why those builders return a count
// and not a count-or-failure.
func generateSiteStats(client, gitLabComClient *gitlabclient.Client) (siteStats, error) {
	version, err := readVersionFile()
	if err != nil {
		return siteStats{}, err
	}
	return siteStats{
		Version:        version,
		Tools:          siteToolCountsFor(client, gitLabComClient),
		Meta:           siteMetaCountsFor(client, gitLabComClient),
		Dynamic:        len(listDynamicTools(dynamicActionCatalog(client, false))),
		CatalogActions: siteCatalogActionsFor(client, gitLabComClient),
		CatalogGroups:  siteCatalogGroupsFor(client),
		Resources:      sumResources(client),
		Prompts:        countPrompts(client),
		Completions:    siteCompletionArgNames,
		Capabilities:   siteCapabilities,
		ToolPackages:   countToolPackages(),
	}, nil
}

// siteToolCountsFor sizes the individual tool surface for every published
// tier.
func siteToolCountsFor(client, gitLabComClient *gitlabclient.Client) siteToolCounts {
	return siteToolCounts{
		Free:                countIndividualTools(client, edition.Free),
		Premium:             countIndividualTools(client, edition.Premium),
		UltimateSelfManaged: countIndividualTools(client, edition.Ultimate),
		GitLabCom:           countIndividualTools(gitLabComClient, edition.Ultimate),
	}
}

// siteMetaCountsFor sizes the meta-tool surface for every published tier
// and instance.
func siteMetaCountsFor(client, gitLabComClient *gitlabclient.Client) siteMetaCounts {
	return siteMetaCounts{
		Base:                  countMetaTools(client, edition.Free),
		Premium:               countMetaTools(client, edition.Premium),
		SelfManagedEnterprise: countMetaTools(client, edition.Ultimate),
		GitLabComPremium:      countMetaTools(gitLabComClient, edition.Premium),
		GitLabCom:             countMetaTools(gitLabComClient, edition.Ultimate),
	}
}

// siteCatalogActionsFor counts the dynamic catalog action routes each
// published tier and instance exposes.
func siteCatalogActionsFor(client, gitLabComClient *gitlabclient.Client) siteCatalogActions {
	return siteCatalogActions{
		Free:                  countCatalogActions(client, edition.Free),
		Premium:               countCatalogActions(client, edition.Premium),
		SelfManagedEnterprise: countCatalogActions(client, edition.Ultimate),
		GitLabComPremium:      countCatalogActions(gitLabComClient, edition.Premium),
		GitLabCom:             countCatalogActions(gitLabComClient, edition.Ultimate),
	}
}

// siteCatalogGroupsFor counts the catalog groups each licensing tier
// exposes.
func siteCatalogGroupsFor(client *gitlabclient.Client) siteCatalogGroups {
	return siteCatalogGroups{
		Free:     countCatalogGroupsForTier(client, edition.Free),
		Premium:  countCatalogGroupsForTier(client, edition.Premium),
		Ultimate: countCatalogGroupsForTier(client, edition.Ultimate),
	}
}

// countIndividualTools returns the number of tools the individual surface
// advertises at tier, from the same [mcpsurface] listing the text report
// derives its per-surface individual-tool numbers from.
func countIndividualTools(client *gitlabclient.Client, tier edition.Tier) int {
	return len(mcpsurface.IndividualTools(client, tier))
}

// countMetaTools returns the number of tools the meta surface advertises at
// tier, from the same [mcpsurface] listing [listServerTools] reads for the
// text report.
func countMetaTools(client *gitlabclient.Client, tier edition.Tier) int {
	return len(mcpsurface.MetaTools(client, tier))
}

// countCatalogActions returns the number of action routes the dynamic
// catalog holds at tier, the catalog [dynamicActionCatalog] builds for the
// text report.
func countCatalogActions(client *gitlabclient.Client, tier edition.Tier) int {
	return countActionRoutes(dynamicActionCatalogForTier(client, tier).ActionMaps())
}

// countCatalogGroupsForTier builds the canonical action catalog for tier
// (including the gitlab_server MCP group) and returns the number of catalog
// groups, matching the documented per-tier catalog-group counts. The build
// reads the compiled-in ActionSpecs, so it cannot fail here for a reason the
// caller could do anything about; the error the catalog produces already
// names the group or the check that rejected it.
func countCatalogGroupsForTier(client *gitlabclient.Client, tier edition.Tier) int {
	catalog := cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: tier, IncludeMCP: true}))
	return catalog.CountGroups()
}

// sumResources returns the total MCP resource count (static + templates).
func sumResources(client *gitlabclient.Client) int {
	static, templates := countResources(client)
	return static + templates
}

// readVersionFile reads the repository VERSION file and returns the trimmed
// semantic version string. It is a variable so a test can make that read
// fail: the file sits under the root this binary derives from its own source
// path, so no input a test controls can take it away, and without the seam
// the arms that stop the payload and exit non-zero on it are never reached.
var readVersionFile = func() (string, error) {
	return readVersionFileAt(repositoryRoot())
}

// readVersionFileAt reads the VERSION file under root and returns the trimmed
// semantic version string, or the read failure: a stats payload without a
// version is not worth publishing, so the caller stops on it.
func readVersionFileAt(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "VERSION")) //#nosec G304 -- root is the repository root, not user input
	if err != nil {
		return "", fmt.Errorf("read VERSION: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// renderSiteStatsJSON marshals stats to prettier-compatible JSON (2-space
// indent, trailing newline) so the committed file passes the site's
// `prettier --check` lint step without reformatting. siteStats is a flat
// struct of strings and ints built by this package, so the marshal has
// nothing to refuse.
func renderSiteStatsJSON(stats siteStats) []byte {
	return append(cmdutil.Must(json.MarshalIndent(stats, "", "  ")), '\n')
}

// writeOrCheckSiteStats writes the generated stats JSON to path, or — when
// checkOnly is set — verifies the committed file matches the freshly generated
// content and returns an actionable error if it is stale.
func writeOrCheckSiteStats(path string, stats siteStats, checkOnly bool) error {
	return docgen.WriteOrCheck(path, renderSiteStatsJSON(stats), checkOnly,
		"go run ./cmd/audit_metrics/ -site-stats "+path)
}
