// site_stats_test.go verifies the single-sourced documentation stats payload.
//
// These tests ensure the committed site/src/data/stats.json stays in lock-step
// with the live MCP surface (acting as an in-repo equivalent of the -check
// gate), that per-tier counts are internally consistent, that the write and
// check paths of the generator behave on disk, and that the completion
// argument-type count matches the canonical documentation.
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// newSiteStats builds the stats payload from a mock self-managed client and a
// GitLab.com client, matching how main wires generateSiteStats. The payload is
// generated once and shared (generateSiteStats builds the full catalog for
// every tier, ~12s); tests must treat it as read-only.
var (
	siteStatsOnce   sync.Once
	siteStatsShared siteStats
	errSiteStats    error
)

func newSiteStats(t *testing.T) siteStats {
	t.Helper()
	siteStatsOnce.Do(func() {
		siteStatsShared, errSiteStats = generateSiteStats(newAuditMetricsClient(t), newGitLabComClient(t))
	})
	if errSiteStats != nil {
		t.Fatalf("generateSiteStats() error: %v", errSiteStats)
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
	want := renderSiteStatsJSON(newSiteStats(t))
	path := filepath.Join(repositoryRoot(), "site", "src", "data", "stats.json")
	got, err := os.ReadFile(path) //#nosec G304 -- fixed in-repo path
	if err != nil {
		t.Fatalf("read committed stats.json: %v", err)
	}
	if string(normalizeNewlines(got)) != string(normalizeNewlines(want)) {
		t.Errorf("%s is stale; regenerate with: go run ./cmd/audit_metrics/ -site-stats site/src/data/stats.json", path)
	}
}

// TestWriteOrCheckSiteStats_CommittedFile_PassesCheck verifies the -check
// mode accepts the committed stats file for the live payload, which is the
// exact call `make check-site-stats` makes.
func TestWriteOrCheckSiteStats_CommittedFile_PassesCheck(t *testing.T) {
	path := filepath.Join(repositoryRoot(), "site", "src", "data", "stats.json")
	if err := writeOrCheckSiteStats(path, newSiteStats(t), true); err != nil {
		t.Fatalf("writeOrCheckSiteStats(check) error: %v", err)
	}
}

// TestWriteOrCheckSiteStats_WriteThenCheck_RoundTrips verifies the write mode
// creates the target directory and emits the prettier-compatible payload, and
// that the check mode then accepts what was written, CRLF line endings
// included.
func TestWriteOrCheckSiteStats_WriteThenCheck_RoundTrips(t *testing.T) {
	stats := siteStats{Version: "1.2.3", Dynamic: 2, Resources: 45, Prompts: 37, Completions: 17, Capabilities: 4, ToolPackages: 175}
	path := filepath.Join(t.TempDir(), "site", "src", "data", "stats.json")

	if err := writeOrCheckSiteStats(path, stats, false); err != nil {
		t.Fatalf("writeOrCheckSiteStats(write) error: %v", err)
	}
	written, err := os.ReadFile(path) //#nosec G304 -- temp path built by the test
	if err != nil {
		t.Fatalf("read written stats: %v", err)
	}
	want := renderSiteStatsJSON(stats)
	if string(written) != string(want) {
		t.Fatalf("written stats =\n%s\nwant\n%s", written, want)
	}
	if !strings.HasPrefix(string(written), "{\n  \"version\": \"1.2.3\",\n") || !strings.HasSuffix(string(written), "}\n") {
		t.Fatalf("written stats are not 2-space indented with a trailing newline:\n%s", written)
	}

	if checkErr := writeOrCheckSiteStats(path, stats, true); checkErr != nil {
		t.Fatalf("writeOrCheckSiteStats(check) after write error: %v", checkErr)
	}
	crlf := strings.ReplaceAll(string(want), "\n", "\r\n")
	if writeErr := os.WriteFile(path, []byte(crlf), 0o600); writeErr != nil {
		t.Fatalf("write CRLF stats: %v", writeErr)
	}
	if crlfErr := writeOrCheckSiteStats(path, stats, true); crlfErr != nil {
		t.Fatalf("writeOrCheckSiteStats(check) rejected CRLF line endings: %v", crlfErr)
	}
}

// TestWriteOrCheckSiteStats_Failures_ReturnActionableErrors verifies each
// failure names what went wrong: a stale committed file, a file that cannot
// be read in check mode, a target whose directory cannot be created, and a
// target that cannot be written.
func TestWriteOrCheckSiteStats_Failures_ReturnActionableErrors(t *testing.T) {
	stats := siteStats{Version: "1.2.3", Dynamic: 2}
	root := t.TempDir()
	stale := filepath.Join(root, "stale.json")
	if err := os.WriteFile(stale, []byte("{\n  \"version\": \"0.0.0\"\n}\n"), 0o600); err != nil {
		t.Fatalf("write stale stats: %v", err)
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	directory := filepath.Join(root, "directory")
	if err := os.Mkdir(directory, 0o750); err != nil {
		t.Fatalf("mkdir target directory: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		checkOnly bool
		want      string
	}{
		{name: "stale file fails the check", path: stale, checkOnly: true, want: stale + " is out of date; run: go run ./cmd/audit_metrics/ -site-stats " + stale},
		{name: "missing file fails the check", path: filepath.Join(root, "missing.json"), checkOnly: true, want: "read " + filepath.Join(root, "missing.json")},
		{name: "parent that is a file fails the write", path: filepath.Join(blocker, "stats.json"), want: "create dir for " + filepath.Join(blocker, "stats.json")},
		{name: "target that is a directory fails the write", path: directory, want: "write " + directory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeOrCheckSiteStats(tt.path, stats, tt.checkOnly)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("writeOrCheckSiteStats() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestReadVersionFile_RepositoryVersion_MatchesVersionFile verifies the
// published version is the trimmed content of the repository VERSION file.
func TestReadVersionFile_RepositoryVersion_MatchesVersionFile(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repositoryRoot(), "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(raw))
	if want == "" {
		t.Fatal("VERSION file is empty")
	}
	got, err := readVersionFile()
	if err != nil {
		t.Fatalf("readVersionFile() error: %v", err)
	}
	if got != want {
		t.Fatalf("readVersionFile() = %q, want %q", got, want)
	}
}

// TestReadVersionFileAt_MissingFile_ReturnsReadError verifies a missing
// VERSION file travels back to the caller as an error naming the read,
// rather than publishing stats with an empty version.
func TestReadVersionFileAt_MissingFile_ReturnsReadError(t *testing.T) {
	root := t.TempDir()

	got, err := readVersionFileAt(root)

	if err == nil || !strings.Contains(err.Error(), "read VERSION: ") {
		t.Fatalf("readVersionFileAt(missing) error = %v, want the VERSION read failure", err)
	}
	if got != "" {
		t.Fatalf("readVersionFileAt(missing) = %q, want no version", got)
	}
}

// TestSiteStatsCapabilitiesMatchesDocs pins siteCapabilities to the count
// documented in docs/reference/capabilities/README.md, so the published
// number cannot silently drift from the canonical capability reference.
//
// This is the guard the fourth capability shipped without: the site's
// overview pages said "three MCP protocol capabilities" for as long as the
// count lived only in hand-written prose, and nothing failed.
func TestSiteStatsCapabilitiesMatchesDocs(t *testing.T) {
	path := filepath.Join(repositoryRoot(), "docs", "reference", "capabilities", "README.md")
	data, err := os.ReadFile(path) //#nosec G304 -- fixed in-repo path
	if err != nil {
		t.Fatalf("read capabilities README: %v", err)
	}
	want := "the **" + strconv.Itoa(siteCapabilities) + " MCP capabilities**"
	if !strings.Contains(string(data), want) {
		t.Errorf("docs do not contain %q; update siteCapabilities or the docs", want)
	}
}

// TestSiteStatsCompletionsMatchesDocs pins siteCompletionArgNames to the count
// documented in docs/reference/capabilities/completions.md so the published
// number cannot silently drift from the canonical capability reference.
func TestSiteStatsCompletionsMatchesDocs(t *testing.T) {
	path := filepath.Join(repositoryRoot(), "docs", "reference", "capabilities", "completions.md")
	data, err := os.ReadFile(path) //#nosec G304 -- fixed in-repo path
	if err != nil {
		t.Fatalf("read completions doc: %v", err)
	}
	want := "supports **" + strconv.Itoa(siteCompletionArgNames) + " argument names**"
	if !strings.Contains(string(data), want) {
		t.Errorf("docs do not contain %q; update siteCompletionArgNames or the docs", want)
	}
}
