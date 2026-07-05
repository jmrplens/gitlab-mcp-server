// site_stats_test.go verifies the single-sourced documentation stats payload.
//
// These tests ensure the committed site/src/data/stats.json stays in lock-step
// with the live MCP surface (acting as an in-repo equivalent of the -check
// gate), that per-tier counts are internally consistent, and that the
// completion argument-type count matches the canonical documentation.
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// newSiteStats builds the stats payload from a mock self-managed client and a
// GitLab.com client, matching how main wires generateSiteStats. The payload is
// generated once and shared (generateSiteStats builds the full catalog for
// every tier, ~12s); tests must treat it as read-only.
var (
	siteStatsOnce   sync.Once
	siteStatsShared siteStats
	siteStatsErr    error
)

func newSiteStats(t *testing.T) siteStats {
	t.Helper()
	siteStatsOnce.Do(func() {
		client := newAuditMetricsClient(t)
		gitLabComClient, err := gitlabclient.NewClient(&config.Config{
			GitLabURL:   config.DefaultGitLabURL,
			GitLabToken: "audit-token", //#nosec G101 -- audit-only dummy token, not a real credential
		})
		if err != nil {
			siteStatsErr = err
			return
		}
		siteStatsShared = generateSiteStats(client, gitLabComClient)
	})
	if siteStatsErr != nil {
		t.Fatalf("create gitlab.com client: %v", siteStatsErr)
	}
	return siteStatsShared
}

// TestSiteStatsTierOrdering verifies the derived counts respect the nested
// tier model (Free ⊂ Premium ⊂ Ultimate ⊂ GitLab.com) and that the fixed
// surfaces (dynamic tools, version) carry their expected shape.
func TestSiteStatsTierOrdering(t *testing.T) {
	stats := newSiteStats(t)

	if stats.Version == "" {
		t.Error("version must be read from the VERSION file, got empty string")
	}
	if stats.Dynamic != 2 {
		t.Errorf("dynamic tools = %d, want 2 (find + execute)", stats.Dynamic)
	}
	if stats.Tools.Free > stats.Tools.Premium ||
		stats.Tools.Premium > stats.Tools.UltimateSelfManaged ||
		stats.Tools.UltimateSelfManaged > stats.Tools.GitLabCom {
		t.Errorf("tool counts not monotonic across tiers: %+v", stats.Tools)
	}
	if stats.CatalogGroups.Free > stats.CatalogGroups.Premium ||
		stats.CatalogGroups.Premium > stats.CatalogGroups.Ultimate {
		t.Errorf("catalog group counts not monotonic across tiers: %+v", stats.CatalogGroups)
	}
	if stats.Meta.Base > stats.Meta.SelfManagedEnterprise ||
		stats.Meta.SelfManagedEnterprise > stats.Meta.GitLabCom {
		t.Errorf("meta counts not monotonic: %+v", stats.Meta)
	}
	if stats.Resources <= 0 || stats.Prompts <= 0 || stats.ToolPackages <= 0 {
		t.Errorf("resources/prompts/tool_packages must be positive: %+v", stats)
	}
}

// TestSiteStatsMatchesCommittedFile verifies the committed
// site/src/data/stats.json equals the freshly generated payload. This is the
// in-repo guard mirroring `audit_metrics -site-stats ... -check`.
func TestSiteStatsMatchesCommittedFile(t *testing.T) {
	stats := newSiteStats(t)
	want, err := renderSiteStatsJSON(stats)
	if err != nil {
		t.Fatalf("render site stats: %v", err)
	}
	path := filepath.Join(repositoryRoot(), "site", "src", "data", "stats.json")
	got, err := os.ReadFile(path) //#nosec G304 -- fixed in-repo path
	if err != nil {
		t.Fatalf("read committed stats.json: %v", err)
	}
	if string(normalizeNewlines(got)) != string(normalizeNewlines(want)) {
		t.Errorf("%s is stale; regenerate with: go run ./cmd/audit_metrics/ -site-stats site/src/data/stats.json", path)
	}
}

// TestSiteStatsCompletionsMatchesDocs pins siteCompletionArgTypes to the count
// documented in docs/reference/capabilities/completions.md so the published
// number cannot silently drift from the canonical capability reference.
func TestSiteStatsCompletionsMatchesDocs(t *testing.T) {
	path := filepath.Join(repositoryRoot(), "docs", "reference", "capabilities", "completions.md")
	data, err := os.ReadFile(path) //#nosec G304 -- fixed in-repo path
	if err != nil {
		t.Fatalf("read completions doc: %v", err)
	}
	want := "supports **" + strconv.Itoa(siteCompletionArgTypes) + " argument types**"
	if !strings.Contains(string(data), want) {
		t.Errorf("docs do not contain %q; update siteCompletionArgTypes or the docs", want)
	}
}
