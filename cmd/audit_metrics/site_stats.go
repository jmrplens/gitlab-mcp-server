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
// GitLab.com-only Orbit tools are included where relevant. The first
// derivation that fails ends the payload: half a stats file is not worth
// publishing, and the caller is where that is reported.
func generateSiteStats(client, gitLabComClient *gitlabclient.Client) (siteStats, error) {
	version, err := readVersionFile()
	if err != nil {
		return siteStats{}, err
	}
	toolCounts, err := siteToolCountsFor(client, gitLabComClient)
	if err != nil {
		return siteStats{}, err
	}
	metaCounts, err := siteMetaCountsFor(client, gitLabComClient)
	if err != nil {
		return siteStats{}, err
	}
	dynamicCatalog, err := dynamicActionCatalog(client, false)
	if err != nil {
		return siteStats{}, err
	}
	dynamicTools, err := listDynamicTools(dynamicCatalog)
	if err != nil {
		return siteStats{}, err
	}
	catalogActions, err := siteCatalogActionsFor(client, gitLabComClient)
	if err != nil {
		return siteStats{}, err
	}
	catalogGroups, err := siteCatalogGroupsFor(client)
	if err != nil {
		return siteStats{}, err
	}
	resourceCount, err := sumResources(client)
	if err != nil {
		return siteStats{}, err
	}
	promptCount, err := countPrompts(client)
	if err != nil {
		return siteStats{}, err
	}
	return siteStats{
		Version:        version,
		Tools:          toolCounts,
		Meta:           metaCounts,
		Dynamic:        len(dynamicTools),
		CatalogActions: catalogActions,
		CatalogGroups:  catalogGroups,
		Resources:      resourceCount,
		Prompts:        promptCount,
		Completions:    siteCompletionArgTypes,
		Capabilities:   siteCapabilities,
		ToolPackages:   countToolPackages(),
	}, nil
}

// siteToolCountsFor sizes the individual tool surface for every published
// tier.
func siteToolCountsFor(client, gitLabComClient *gitlabclient.Client) (siteToolCounts, error) {
	free, err := countIndividualTools(client, edition.Free)
	if err != nil {
		return siteToolCounts{}, err
	}
	premium, err := countIndividualTools(client, edition.Premium)
	if err != nil {
		return siteToolCounts{}, err
	}
	ultimate, err := countIndividualTools(client, edition.Ultimate)
	if err != nil {
		return siteToolCounts{}, err
	}
	gitLabCom, err := countIndividualTools(gitLabComClient, edition.Ultimate)
	if err != nil {
		return siteToolCounts{}, err
	}
	return siteToolCounts{Free: free, Premium: premium, UltimateSelfManaged: ultimate, GitLabCom: gitLabCom}, nil
}

// siteMetaCountsFor sizes the meta-tool surface for every published
// deployment.
func siteMetaCountsFor(client, gitLabComClient *gitlabclient.Client) (siteMetaCounts, error) {
	base, err := listServerTools(client, true, false)
	if err != nil {
		return siteMetaCounts{}, err
	}
	selfManaged, err := listServerTools(client, true, true)
	if err != nil {
		return siteMetaCounts{}, err
	}
	gitLabCom, err := listServerTools(gitLabComClient, true, true)
	if err != nil {
		return siteMetaCounts{}, err
	}
	return siteMetaCounts{Base: len(base), SelfManagedEnterprise: len(selfManaged), GitLabCom: len(gitLabCom)}, nil
}

// siteCatalogActionsFor counts the dynamic catalog action routes each
// published tier exposes.
func siteCatalogActionsFor(client, gitLabComClient *gitlabclient.Client) (siteCatalogActions, error) {
	free, err := dynamicActionCatalog(client, false)
	if err != nil {
		return siteCatalogActions{}, err
	}
	selfManaged, err := dynamicActionCatalog(client, true)
	if err != nil {
		return siteCatalogActions{}, err
	}
	gitLabCom, err := dynamicActionCatalog(gitLabComClient, true)
	if err != nil {
		return siteCatalogActions{}, err
	}
	return siteCatalogActions{
		Free:                  countActionRoutes(free.ActionMaps()),
		SelfManagedEnterprise: countActionRoutes(selfManaged.ActionMaps()),
		GitLabCom:             countActionRoutes(gitLabCom.ActionMaps()),
	}, nil
}

// siteCatalogGroupsFor counts the catalog groups each licensing tier
// exposes.
func siteCatalogGroupsFor(client *gitlabclient.Client) (siteCatalogGroups, error) {
	free, err := countCatalogGroupsForTier(client, edition.Free)
	if err != nil {
		return siteCatalogGroups{}, err
	}
	premium, err := countCatalogGroupsForTier(client, edition.Premium)
	if err != nil {
		return siteCatalogGroups{}, err
	}
	ultimate, err := countCatalogGroupsForTier(client, edition.Ultimate)
	if err != nil {
		return siteCatalogGroups{}, err
	}
	return siteCatalogGroups{Free: free, Premium: premium, Ultimate: ultimate}, nil
}

// countIndividualTools registers the individual tool surface for tier and
// returns the number of advertised tools, mirroring how the text report
// derives its per-surface individual-tool numbers.
func countIndividualTools(client *gitlabclient.Client, tier edition.Tier) (int, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion}, &mcp.ServerOptions{PageSize: 4000, Capabilities: &mcp.ServerCapabilities{}})
	tools.RegisterAll(server, client, tier)
	listed, err := listToolsFromServer(server)
	if err != nil {
		return 0, err
	}
	return len(listed), nil
}

// countCatalogGroupsForTier builds the canonical action catalog for tier
// (including the gitlab_server MCP group) and returns the number of catalog
// groups, matching the documented per-tier catalog-group counts.
func countCatalogGroupsForTier(client *gitlabclient.Client, tier edition.Tier) (int, error) {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		return 0, fmt.Errorf("build catalog for tier %s: %w", tier, err)
	}
	return catalog.CountGroups(), nil
}

// sumResources returns the total MCP resource count (static + templates).
func sumResources(client *gitlabclient.Client) (int, error) {
	static, templates, err := countResources(client)
	if err != nil {
		return 0, err
	}
	return static + templates, nil
}

// readVersionFile reads the repository VERSION file and returns the trimmed
// semantic version string.
func readVersionFile() (string, error) {
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
