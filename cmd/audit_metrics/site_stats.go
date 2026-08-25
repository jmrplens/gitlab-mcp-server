// site_stats.go generates the single-sourced statistics file consumed by the
// Astro Starlight documentation site (site/src/data/stats.json). Every value is
// derived from the canonical action catalog projection, the in-memory MCP
// surface, or the repository VERSION file — never hardcoded — so the published
// numbers cannot drift away from the real server surface.
//
// The same per-surface, per-tier derivations used by the text report drive
// these counts:
//
//   - tools.*           individual tool surface via [tools.RegisterAll] per tier
//   - meta.*            meta-tool surface via [tools.RegisterAllMeta]
//   - dynamic           the fixed find/execute dynamic surface (2 tools)
//   - catalog_actions.* dynamic catalog action routes per tier
//   - catalog_groups.*  catalog group count per tier (IncludeMCP)
//   - resources/prompts registered MCP resource and prompt counts
//   - tool_packages     Go package directories under internal/tools
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// siteCapabilities is the number of MCP protocol capabilities this server
// implements: completions, progress, elicitation, and resource
// subscriptions. Capabilities are wired individually in cmd/server rather
// than through an enumerable registry, so this value mirrors the canonical
// count documented in docs/reference/capabilities/README.md ("the 4 MCP
// capabilities"). It is pinned by TestSiteStatsCapabilitiesMatchesDocs so
// it cannot silently drift — which is exactly how the site's capability
// count went stale when the fourth capability shipped.
const siteCapabilities = 4

// siteCompletionArgTypes is the number of distinct completion argument types
// the server supports. The completion handler dispatches argument types through
// a switch in internal/completions rather than an enumerable registry, so this
// value mirrors the canonical count documented in
// docs/reference/capabilities/completions.md ("17 argument types"). It is pinned
// by TestSiteStatsCompletionsMatchesDocs so it cannot silently drift.
const siteCompletionArgTypes = 17

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

// siteMetaCounts holds the meta-tool surface size per deployment.
type siteMetaCounts struct {
	Base                  int `json:"base"`
	SelfManagedEnterprise int `json:"self_managed_enterprise"`
	GitLabCom             int `json:"gitlab_com"`
}

// siteCatalogActions holds the dynamic catalog action-route count per tier.
type siteCatalogActions struct {
	Free                  int `json:"free"`
	SelfManagedEnterprise int `json:"self_managed_enterprise"`
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
func generateSiteStats(client, gitLabComClient *gitlabclient.Client) siteStats {
	return siteStats{
		Version: readVersionFile(),
		Tools: siteToolCounts{
			Free:                countIndividualTools(client, edition.Free),
			Premium:             countIndividualTools(client, edition.Premium),
			UltimateSelfManaged: countIndividualTools(client, edition.Ultimate),
			GitLabCom:           countIndividualTools(gitLabComClient, edition.Ultimate),
		},
		Meta: siteMetaCounts{
			Base:                  len(listServerTools(client, true, false)),
			SelfManagedEnterprise: len(listServerTools(client, true, true)),
			GitLabCom:             len(listServerTools(gitLabComClient, true, true)),
		},
		Dynamic: len(listDynamicTools(dynamicActionCatalog(client, false))),
		CatalogActions: siteCatalogActions{
			Free:                  countActionRoutes(dynamicActionCatalog(client, false).ActionMaps()),
			SelfManagedEnterprise: countActionRoutes(dynamicActionCatalog(client, true).ActionMaps()),
			GitLabCom:             countActionRoutes(dynamicActionCatalog(gitLabComClient, true).ActionMaps()),
		},
		CatalogGroups: siteCatalogGroups{
			Free:     countCatalogGroupsForTier(client, edition.Free),
			Premium:  countCatalogGroupsForTier(client, edition.Premium),
			Ultimate: countCatalogGroupsForTier(client, edition.Ultimate),
		},
		Resources:    sumResources(client),
		Prompts:      countPrompts(client),
		Completions:  siteCompletionArgTypes,
		Capabilities: siteCapabilities,
		ToolPackages: countToolPackages(),
	}
}

// countIndividualTools registers the individual tool surface for tier and
// returns the number of advertised tools, mirroring how the text report
// derives its per-surface individual-tool numbers.
func countIndividualTools(client *gitlabclient.Client, tier edition.Tier) int {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, &mcp.ServerOptions{PageSize: 4000})
	tools.RegisterAll(server, client, tier)
	return len(listToolsFromServer(server))
}

// countCatalogGroupsForTier builds the canonical action catalog for tier
// (including the gitlab_server MCP group) and returns the number of catalog
// groups, matching the documented per-tier catalog-group counts.
func countCatalogGroupsForTier(client *gitlabclient.Client, tier edition.Tier) int {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build catalog for tier %s: %v\n", tier, err)
		os.Exit(1)
	}
	return catalog.CountGroups()
}

// sumResources returns the total MCP resource count (static + templates).
func sumResources(client *gitlabclient.Client) int {
	static, templates := countResources(client)
	return static + templates
}

// readVersionFile reads the repository VERSION file and returns the trimmed
// semantic version string.
func readVersionFile() string {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(), "VERSION"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read VERSION: %v\n", err)
		os.Exit(1)
	}
	return strings.TrimSpace(string(data))
}

// renderSiteStatsJSON marshals stats to prettier-compatible JSON (2-space
// indent, trailing newline) so the committed file passes the site's
// `prettier --check` lint step without reformatting.
func renderSiteStatsJSON(stats siteStats) ([]byte, error) {
	raw, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal site stats: %w", err)
	}
	return append(raw, '\n'), nil
}

// writeOrCheckSiteStats writes the generated stats JSON to path, or — when
// checkOnly is set — verifies the committed file matches the freshly generated
// content and returns an actionable error if it is stale.
func writeOrCheckSiteStats(path string, stats siteStats, checkOnly bool) error {
	content, err := renderSiteStatsJSON(stats)
	if err != nil {
		return err
	}
	if checkOnly {
		existing, readErr := os.ReadFile(path) //#nosec G304 -- path is an operator-supplied generator target
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		if !bytes.Equal(normalizeNewlines(existing), normalizeNewlines(content)) {
			return fmt.Errorf("%s is out of date; run: go run ./cmd/audit_metrics/ -site-stats %s", path, path)
		}
		return nil
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o750); mkErr != nil {
		return fmt.Errorf("create dir for %s: %w", path, mkErr)
	}
	if writeErr := os.WriteFile(path, content, 0o600); writeErr != nil {
		return fmt.Errorf("write %s: %w", path, writeErr)
	}
	return nil
}

// normalizeNewlines strips carriage returns so the check is line-ending
// agnostic across platforms.
func normalizeNewlines(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}
