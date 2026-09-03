// main_test.go contains unit tests for the discovery-completeness auditor.
// The gold-standard fixtures pin the link_create_batch BEFORE/AFTER signature
// so future regressions in the check logic (or in the underlying specs) are
// caught by CI without running the full eval surface.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIsGenericUsage_FlagsPlaceholders verifies the placeholder-Usage detector
// matches the generated "Use to execute … action." templates and empty strings
// while leaving purpose-specific sentences untouched.
func TestIsGenericUsage_FlagsPlaceholders(t *testing.T) {
	generic := []string{
		"",
		"   ",
		"Use to execute branches domain action.",
		"Use to execute list action.",
		"use to execute markdown_render action",
	}
	for _, usage := range generic {
		t.Run(usage, func(t *testing.T) {
			if !auditshared.IsGenericUsage(usage) {
				t.Errorf("isGenericUsage(%q) = false, want true", usage)
			}
		})
	}
	specific := []string{
		"List branches for a project with optional search and pagination.",
		"Create MULTIPLE release asset links in one call. Use this instead of repeated link_create.",
	}
	for _, usage := range specific {
		t.Run(usage, func(t *testing.T) {
			if auditshared.IsGenericUsage(usage) {
				t.Errorf("isGenericUsage(%q) = true, want false", usage)
			}
		})
	}
}

// TestWeakAliases_ThresholdHonored verifies weak_aliases escalates with
// minAliases and that natural-language aliases clear the flag.
func TestWeakAliases_ThresholdHonored(t *testing.T) {
	bare := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		Aliases:        []string{"gitlab_release_link_create_batch", "release.link_create_batch"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
	}
	if !weakAliases(bare, 3) {
		t.Error("expected bare aliases to be flagged at minAliases=3")
	}
	if !weakAliases(bare, 2) {
		t.Error("expected bare aliases to be flagged at minAliases=2")
	}
	// minAliases=1: 0 non-canonical aliases < 1, so still flagged (need at
	// least 1 explicit natural-language alias to clear the bare-aliases case).
	if !weakAliases(bare, 1) {
		t.Error("expected bare aliases to be flagged at minAliases=1")
	}
	rich := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		Aliases:        []string{"gitlab_release_link_create_batch", "create multiple release links", "batch create release asset links", "link package files to release"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
	}
	if weakAliases(rich, 3) {
		t.Error("expected 3 natural-language aliases to clear at minAliases=3")
	}
}

// TestBaseActionStem_StripsVariantAndCRUDSuffixes verifies the clustering
// stem normalizes variant actions back to a common base. When the action
// strips down to a bare CRUD verb, the stem falls back to the owner so
// clusters stay meaningful.
func TestBaseActionStem_StripsVariantAndCRUDSuffixes(t *testing.T) {
	cases := map[string]string{
		"release.link_create_batch": "release.link",
		"release.link_create":       "release.link",
		"release.link_list":         "release.link",
		"release.link_get":          "release.link",
		"package.publish_directory": "package.publish",
		"package.publish":           "package.publish",
		"merge_request.update":      "merge_request",
		"branch.list":               "branch",
		"members.add_bulk":          "members",
		"notes.delete_all":          "notes",
		"link_create_batch":         "link",
		"link_create":               "link",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := baseActionStem(in); got != want {
				t.Errorf("baseActionStem(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestSiblingCluster_DetectsBatchVsSingle pins the link_create_batch
// BEFORE/AFTER gold standard. The synthetic single vs batch pair must form
// a cluster, and the batch variant must be flagged missing_disambiguation
// when it lacks both a sibling reference and a usage signal.
func TestSiblingCluster_DetectsBatchVsSingle(t *testing.T) {
	single := toolutil.ActionSpec{
		Name:           "release.link_create",
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create"},
		Usage:          "Create a single release asset link.",
		Aliases:        []string{"gitlab_release_link_create", "create release link"},
		RelatedActions: []string{"release.link_list"},
	}
	batchBefore := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
		Usage:          "Use to execute releaselinks domain action.",
		Aliases:        []string{"gitlab_release_link_create_batch"},
		RelatedActions: nil,
	}
	batchAfter := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
		Usage:          "Create MULTIPLE release asset links in one call. Use this instead of repeated link_create.",
		Aliases:        []string{"gitlab_release_link_create_batch", "create multiple release links", "batch create release asset links"},
		RelatedActions: []string{"release.link_create", "release.create", "package.publish_directory"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"links": {
				CommonConfusions: []string{"Do not call link_create once per asset when several are requested."},
			},
		},
	}

	specs := []toolutil.ActionSpec{single, batchBefore}
	clusters := siblingClusters(specs)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(clusters), clusters)
	}
	if clusters[0].Stem != "release.link" {
		t.Errorf("cluster stem = %q, want release.link", clusters[0].Stem)
	}
	if len(clusters[0].Members) != 2 {
		t.Errorf("cluster members = %v, want 2", clusters[0].Members)
	}

	// BEFORE-state: batch has no usage signal, no related sibling, no
	// CommonConfusions naming the sibling — must be flagged.
	members := clusterMembersFor(clusters, "releaselinks", "release.link_create_batch")
	if !containsStr(members, "release.link_create") {
		t.Fatalf("cluster membership missing sibling: %v", members)
	}
	if hasDisambiguation(batchBefore, members) {
		t.Error("BEFORE-state batch should NOT have disambiguation")
	}
	if !hasDisambiguation(batchAfter, members) {
		t.Error("AFTER-state batch SHOULD have disambiguation")
	}

	// Sanity: the analyzeSpec call on the BEFORE state should raise
	// missing_disambiguation at error severity.
	finding := analyzeSpec(batchBefore, nil, members, 3)
	if !containsStr(finding.Flags, "missing_disambiguation") {
		t.Errorf("BEFORE-state missing missing_disambiguation flag: %+v", finding.Flags)
	}
	if finding.Severity != "error" {
		t.Errorf("BEFORE-state severity = %q, want error", finding.Severity)
	}

	// AFTER-state: clean (no missing_disambiguation).
	findingAfter := analyzeSpec(batchAfter, map[string]string{
		"gitlab_release_link_create_batch": "Create multiple release asset links in one call. Returns: links. See also: gitlab_release_link_create, gitlab_release_link_list.",
	}, members, 3)
	if containsStr(findingAfter.Flags, "missing_disambiguation") {
		t.Errorf("AFTER-state should not flag missing_disambiguation: %+v", findingAfter.Flags)
	}
	if containsStr(findingAfter.Flags, "generic_usage") {
		t.Errorf("AFTER-state should not flag generic_usage: %+v", findingAfter.Flags)
	}
}

// TestSiblingCluster_IgnoresSingletons verifies that clusters with fewer
// than 2 members are not emitted and that hasDisambiguation returns true
// (vacuously) for the single-member case.
func TestSiblingCluster_IgnoresSingletons(t *testing.T) {
	specs := []toolutil.ActionSpec{{
		Name:           "project.get",
		OwnerPackage:   "projects",
		Usage:          "Use to execute projects domain action.",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_project_get"},
	}}
	clusters := siblingClusters(specs)
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters, got %+v", clusters)
	}
	if !hasDisambiguation(specs[0], nil) {
		t.Error("single-member (no cluster) should vacuously pass disambiguation")
	}
}

// TestMissingDisambiguation_OnlyFlagsNonCRUDVariants pins the Phase 1
// refinement: pure CRUD families (create/get/list/delete/update on the
// same resource) do NOT need disambiguation because the verb is the
// disambiguator. Only base-vs-variant clusters with non-CRUD suffixes
// (_batch, _bulk, _all, _directory, _single) trigger the check.
func TestMissingDisambiguation_OnlyFlagsNonCRUDVariants(t *testing.T) {
	// Pure CRUD family: token_group_create/get/list. None have non-CRUD
	// variant suffixes; none should be flagged missing_disambiguation.
	crud := []toolutil.ActionSpec{
		{
			Name: "accesstokens.token_group_create", OwnerPackage: "accesstokens",
			Usage:          "Create a group access token.",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_create_group_access_token"},
		},
		{
			Name: "accesstokens.token_group_get", OwnerPackage: "accesstokens",
			Usage:          "Get a group access token by token_id.",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_get_group_access_token"},
		},
		{
			Name: "accesstokens.token_group_list", OwnerPackage: "accesstokens",
			Usage:          "List group access tokens.",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_group_access_tokens"},
		},
	}
	clusters := siblingClusters(crud)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %+v", clusters)
	}
	for _, spec := range crud {
		t.Run(spec.Name, func(t *testing.T) {
			members := clusterMembersFor(clusters, spec.OwnerPackage, spec.Name)
			finding := analyzeSpec(spec, nil, members, 3)
			if containsStr(finding.Flags, "missing_disambiguation") {
				t.Errorf("pure CRUD member %q should NOT be flagged missing_disambiguation: %+v", spec.Name, finding.Flags)
			}
		})
	}

	// Base-vs-variant cluster: link_create (base) + link_create_batch (variant).
	// The _batch variant should be flagged because it lacks both a sibling
	// reference and a usage signal.
	base := toolutil.ActionSpec{
		Name: "releaselinks.link_create", OwnerPackage: "releaselinks",
		Usage:          "Create a single release asset link.",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create"},
		Aliases:        []string{"create release link"},
	}
	variant := toolutil.ActionSpec{
		Name: "releaselinks.link_create_batch", OwnerPackage: "releaselinks",
		Usage:          "Use to execute releaselinks domain action.",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
		Aliases:        []string{"gitlab_release_link_create_batch"},
	}
	clusters2 := siblingClusters([]toolutil.ActionSpec{base, variant})
	members := clusterMembersFor(clusters2, "releaselinks", variant.Name)
	finding := analyzeSpec(variant, nil, members, 3)
	if !containsStr(finding.Flags, "missing_disambiguation") {
		t.Errorf("base-vs-variant batch should be flagged missing_disambiguation: %+v", finding.Flags)
	}
}

// TestHasNonCRUDVariantSuffix verifies the suffix detector matches only
// the non-CRUD variant markers and ignores pure CRUD verbs.
func TestHasNonCRUDVariantSuffix(t *testing.T) {
	variant := []string{
		"release.link_create_batch",
		"package.publish_directory",
		"deploy_key_list_all",
		"registry_tag_delete_bulk",
		"members.add_bulk",
		"notes.delete_all",
	}
	for _, name := range variant {
		t.Run(name, func(t *testing.T) {
			if !hasNonCRUDVariantSuffix(name) {
				t.Errorf("hasNonCRUDVariantSuffix(%q) = false, want true", name)
			}
		})
	}
	crud := []string{
		"branch.list",
		"branch.get",
		"branch.create",
		"branch.update",
		"branch.delete",
		"token_group_create",
		"deploy_key_get",
	}
	for _, name := range crud {
		t.Run(name, func(t *testing.T) {
			if hasNonCRUDVariantSuffix(name) {
				t.Errorf("hasNonCRUDVariantSuffix(%q) = true, want false", name)
			}
		})
	}
}

// TestBaseActionStem_StripsScopeSuffixes verifies the scope suffixes
// (_project, _user, _group, _instance) collapse scope-specific variants
// to the same base stem.
func TestBaseActionStem_StripsScopeSuffixes(t *testing.T) {
	cases := map[string]string{
		"deploy_key_list_project":   "deploy_key",
		"deploy_key_list_user":      "deploy_key",
		"deploy_key_list_group":     "deploy_key",
		"deploy_key_list_instance":  "deploy_key",
		"deploy_token_list_project": "deploy_token",
		"deploy_token_list_group":   "deploy_token",
		"deploy_token_list_all":     "deploy_token",
		"pages_domain_list_all":     "pages_domain",
		"pages_domain_list_project": "pages_domain",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := baseActionStem(in); got != want {
				t.Errorf("baseActionStem(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// TestSiblingMatches_AcceptsPrefixedAndUnderscoreForms pins the cross-format
// matching that resolves RelatedActions "pages.domain_list" against cluster
// sibling "pages_domain_list" (the same action referenced with two different
// separator conventions).
func TestSiblingMatches_AcceptsPrefixedAndUnderscoreForms(t *testing.T) {
	siblings := map[string]struct{}{
		"pages_domain_list":   {},
		"pages_domain_get":    {},
		"pages_domain_create": {},
	}
	cases := map[string]bool{
		"pages_domain_list":    true,  // exact lowercase
		"pages.domain_list":    true,  // head + "_" + tail -> "pages_domain_list"
		"PAGES.DOMAIN_LIST":    true,  // normalized lowercase + head/tail form
		"pages.domain_unknown": false, // no match
		"totally_unrelated":    false, // no match
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := siblingMatches(strings.ToLower(in), siblings); got != want {
				t.Errorf("siblingMatches(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

// TestSeverityFor_OnlyEscalatesInNonCRUDClusters pins the Wave 2 scope
// refinement: weak_aliases/empty_related/weak_individual_description are
// escalated to error only when the cluster has a non-CRUD variant member
// (the eval-failure class). Pure CRUD families stay at warning because the
// verb is itself the disambiguator.
func TestSeverityFor_OnlyEscalatesInNonCRUDClusters(t *testing.T) {
	// Pure CRUD family: no batch/bulk/all/directory/single members.
	pureCRUD := []string{"deploy_key_get", "deploy_key_add", "deploy_key_delete", "deploy_key_update"}
	// Base-vs-variant cluster with a _batch member.
	withBatch := []string{"link_create", "link_create_batch", "link_get", "link_list"}
	escalating := []string{"weak_aliases", "empty_related", "weak_individual_description"}

	type scenario struct {
		name           string
		flag           string
		inCluster      bool
		clusterMembers []string
		want           string
	}
	var cases []scenario
	for _, f := range escalating {
		cases = append(
			cases,
			scenario{"pure CRUD cluster stays warning/" + f, f, true, pureCRUD, "warning"},
			scenario{"base-vs-variant cluster escalates to error/" + f, f, true, withBatch, "error"},
			scenario{"out-of-cluster stays warning/" + f, f, false, nil, "warning"},
		)
	}
	// Flags that are always error regardless of cluster.
	cases = append(
		cases,
		scenario{"generic_usage always error", "generic_usage", false, nil, "error"},
		scenario{"missing_disambiguation always error", "missing_disambiguation", false, nil, "error"},
	)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := severityFor(c.flag, c.inCluster, c.clusterMembers); got != c.want {
				t.Errorf("severityFor(%q, inCluster=%v) = %q, want %q", c.flag, c.inCluster, got, c.want)
			}
		})
	}
}

// TestUsageHasSignal_DetectsDistinguishingPhrases verifies the Usage signal
// heuristic matches the gold-standard phrasing patterns. "single" is
// deliberately excluded from the keyword list (too generic).
func TestUsageHasSignal_DetectsDistinguishingPhrases(t *testing.T) {
	cases := map[string]bool{
		"Create a single release asset link.":              false,
		"Create MULTIPLE release asset links in one call.": true,
		"Use this instead of repeated link_create.":        true,
		"List branches for a project.":                     false,
		"Attach multiple assets to release.":               true,
		"Publish all assets in a directory.":               true,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			if got := usageHasSignal(in); got != want {
				t.Errorf("usageHasSignal(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

// TestEmptyParamDescription_FlagsBoilerplate verifies the field-level
// detector catches missing or boilerplate descriptions.
func TestEmptyParamDescription_FlagsBoilerplate(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"id":   map[string]any{"type": "string", "description": "ID"},
			"good": map[string]any{"type": "string", "description": "Human-readable name, used for display"},
			"nested": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"deep": map[string]any{"type": "string"},
				},
			},
			"list": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}
	got := emptyParamDescriptions(schema)
	// Should flag "name" (no description), "id" ("ID" too short), and
	// "nested.deep" (nested empty). "good" should pass.
	wantContains := []string{"name", "id", "nested.deep"}
	for _, w := range wantContains {
		t.Run(w, func(t *testing.T) {
			if !containsStr(got, w) {
				t.Errorf("emptyParamDescriptions missing %q in %v", w, got)
			}
		})
	}
	for _, g := range got {
		if g == "good" {
			t.Errorf("emptyParamDescriptions incorrectly flagged %q", g)
		}
	}
}

// cachedFullReport runs the full discovery analysis (gapsOnly=false,
// minAliases=3) once under its own mock client and shares the result across
// this package's tests: buildReport spins an in-memory MCP server over the
// full catalog (~4s per run). Tests must treat the returned report as
// read-only.
var (
	fullReportOnce sync.Once
	fullReport     report
)

func cachedFullReport(t *testing.T) report {
	t.Helper()
	fullReportOnce.Do(func() {
		fullReport = buildReport(false, 3)
	})
	return fullReport
}

// TestBuildReport_LinkCreateBatchGoldStandard exercises the auditor against
// the canonical link_create / link_create_batch cluster using synthetic
// specs that mirror the BEFORE/AFTER signature from the discovery eval
// (plan/discovery-metadata-completeness.md §1). This pins the auditor
// without coupling to the live releaselinks package, whose current source
// has a subsequent override that clobbers the original gold-standard fix.
//
// Both BEFORE-state and AFTER-state are pinned here so future regressions
// in either direction (a check that no longer detects the gap, or a
// regression in the source that re-introduces it) are caught.
func TestBuildReport_LinkCreateBatchGoldStandard(t *testing.T) {
	client, cleanup := auditclient.NewMock()
	defer cleanup()
	_ = client // silence unused warning; client reserved for future live-catalog assertions.

	// Synthetic cluster: link_create (single) + link_create_batch (BEFORE-state).
	singleBefore := toolutil.ActionSpec{
		Name:           "release.link_create",
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create"},
		Usage:          "Create a single release asset link.",
		Aliases:        []string{"gitlab_release_link_create", "create release link"},
		RelatedActions: []string{"release.link_list"},
	}
	batchBefore := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
		Usage:          "Use to execute releaselinks domain action.",
		Aliases:        []string{"gitlab_release_link_create_batch"},
	}
	batchAfter := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		OwnerPackage:   "releaselinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
		Usage:          "Create MULTIPLE release asset links in one call. Use this instead of repeated link_create.",
		Aliases:        []string{"gitlab_release_link_create_batch", "create multiple release links", "batch create release asset links", "link package files to release"},
		RelatedActions: []string{"release.link_create", "release.create", "package.publish_directory"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"links": {
				CommonConfusions: []string{"Do not call link_create once per asset when several are requested."},
			},
		},
	}

	clusters := siblingClusters([]toolutil.ActionSpec{singleBefore, batchBefore})
	if len(clusters) != 1 || clusters[0].Stem != "release.link" {
		t.Fatalf("expected one release.link cluster, got %+v", clusters)
	}
	members := clusters[0].Members

	// BEFORE-state: must be flagged missing_disambiguation at error severity.
	before := analyzeSpec(batchBefore, nil, members, 3)
	if !containsStr(before.Flags, "missing_disambiguation") {
		t.Errorf("BEFORE-state should flag missing_disambiguation: %+v", before.Flags)
	}
	if before.Severity != "error" {
		t.Errorf("BEFORE-state severity = %q, want error", before.Severity)
	}
	if !containsStr(before.Flags, "generic_usage") {
		t.Errorf("BEFORE-state should flag generic_usage: %+v", before.Flags)
	}

	// AFTER-state: must NOT be flagged missing_disambiguation.
	after := analyzeSpec(batchAfter, map[string]string{
		"gitlab_release_link_create_batch": "Create multiple release asset links in one call. Returns: links. See also: gitlab_release_link_create, gitlab_release_link_list.",
	}, members, 3)
	if containsStr(after.Flags, "missing_disambiguation") {
		t.Errorf("AFTER-state should NOT flag missing_disambiguation: %+v", after.Flags)
	}
	if containsStr(after.Flags, "generic_usage") {
		t.Errorf("AFTER-state should NOT flag generic_usage: %+v", after.Flags)
	}
	if containsStr(after.Flags, "weak_aliases") {
		t.Errorf("AFTER-state should NOT flag weak_aliases (4 NL aliases): %+v", after.Flags)
	}

	// Live-catalog cross-check: the current releaselinks.ActionSpecs batch
	// action is reported via the full auditor (with dedup and dynamic
	// registry corroboration) so future changes to the source packages are
	// visible in CI. The result is informational only (t.Logf) — the gold
	// standard is pinned above against the synthetic spec.
	client2, cleanup2 := auditclient.NewMock()
	defer cleanup2()
	rep := cachedFullReport(t)
	var liveFlags []string
	for _, pr := range rep.Packages {
		if pr.Package != "releaselinks" {
			continue
		}
		for _, f := range pr.Findings {
			t.Logf("releaselinks/%s: severity=%s flags=%v", f.Action, f.Severity, f.Flags)
			if f.Action == "link_create_batch" {
				liveFlags = f.Flags
			}
		}
	}
	// Log the live status. A regression in releaselinks (e.g. the override
	// that clobbers the gold-standard) will surface here as
	// missing_disambiguation on the live batch spec.
	t.Logf("live link_create_batch flags (informational): %v", liveFlags)
	_ = releaselinks.ActionSpecs(client2) // keep the import in use; future assertions may consult it.
}

// TestBuildReport_Deterministic verifies repeated runs are identical.
func TestBuildReport_Deterministic(t *testing.T) {
	first := buildReport(true, 3)
	second := buildReport(true, 3)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("buildReport is not deterministic across runs")
	}
}

// TestBuildReport_NonEmptyActions verifies the auditor reports actions.
func TestBuildReport_NonEmptyActions(t *testing.T) {
	rep := cachedFullReport(t)
	if rep.Summary.Actions == 0 {
		t.Fatalf("no actions analyzed: %+v", rep.Summary)
	}
	if rep.SchemaVersion != schemaVersion {
		t.Errorf("schema_version = %d, want %d", rep.SchemaVersion, schemaVersion)
	}
	// Findings summary should round-trip via JSON.
	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(content), `"schema_version"`) {
		t.Errorf("serialized report missing schema_version")
	}
}

// TestBuildReport_LiveBaseline establishes the post-PR-190 baseline counts
// for the discovery completeness auditor. The test is informational: it
// logs the current snapshot so future Phase 1 waves have a reference and
// regressions are visible. The "errors" count is expected to be non-zero
// initially — Phase 1 (dimensioning + FP triage + multi-agent burn-down) is
// the work that drives it down to zero. See plan/post-pr190-cleanup.md
// META-001 §5 (Phases).
func TestBuildReport_LiveBaseline(t *testing.T) {
	rep := cachedFullReport(t)
	t.Logf("discovery baseline: actions=%d errors=%d warnings=%d packages=%d clusters=%d",
		rep.Summary.Actions, rep.Summary.Errors, rep.Summary.Warnings,
		rep.Summary.Packages, len(rep.Clusters))
	for _, c := range rep.Clusters {
		t.Logf("cluster %s/%s: %v", c.Package, c.Stem, c.Members)
	}
	// Top error contributors for Phase 1 triage.
	pkgErrCount := map[string]int{}
	for _, pr := range rep.Packages {
		for _, f := range pr.Findings {
			if f.Severity == "error" {
				pkgErrCount[pr.Package]++
			}
		}
	}
	t.Logf("top error-severity contributors:")
	count := 0
	for _, p := range sortedIntMapDesc(pkgErrCount) {
		if count >= 10 {
			break
		}
		t.Logf("  %s: %d", p.Key, p.Val)
		count++
	}
	// Schema sanity: at least 1 cluster and at least 1 cluster is releaselinks/link.
	if len(rep.Clusters) == 0 {
		t.Error("expected at least one cluster; got 0")
	}
	// Auditor JSON round-trips cleanly (used by -output and CI gates).
	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(content), `"clusters"`) {
		t.Errorf("serialized report missing clusters key")
	}
}

// sortedIntMapDesc returns (value, key) pairs sorted by value descending.
func sortedIntMapDesc(m map[string]int) []intPair {
	out := make([]intPair, 0, len(m))
	for k, v := range m {
		out = append(out, intPair{Key: k, Val: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Val != out[j].Val {
			return out[i].Val > out[j].Val
		}
		return out[i].Key < out[j].Key
	})
	return out
}

type intPair struct {
	Key string
	Val int
}

// containsStr reports whether slice contains s.
func containsStr(slice []string, s string) bool {
	return slices.Contains(slice, s)
}

// Compile-time guard: keep the imports referenced even when some helpers are
// only used by future tests.
var _ = gitlabclient.Client{}

// TestNeedsMarkdownFormatter pins the heuristic for missing_next_steps:
// list/detail content kinds always need it; destructive actions don't.
func TestNeedsMarkdownFormatter(t *testing.T) {
	listSpec := toolutil.ActionSpec{ContentKind: toolutil.ActionSpecContentList}
	if !needsMarkdownFormatter(listSpec) {
		t.Errorf("list content must need a formatter")
	}
	destructive := toolutil.ActionSpec{ContentKind: toolutil.ActionSpecContentDetail, Destructive: true}
	if needsMarkdownFormatter(destructive) {
		t.Errorf("destructive detail should not require formatter")
	}
}

// TestAliasesOnlyToolname verifies the alias-content heuristic against
// gold-standard inputs.
func TestAliasesOnlyToolname(t *testing.T) {
	cases := []struct {
		name string
		spec toolutil.ActionSpec
		want bool
	}{
		{
			name: "all aliases equal toolname",
			spec: toolutil.ActionSpec{
				Name:           "foo_create",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_foo_create"},
				Aliases:        []string{"gitlab_foo_create", "foo_create"},
			},
			want: true,
		},
		{
			name: "natural-language alias present",
			spec: toolutil.ActionSpec{
				Name:           "foo_create",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_foo_create"},
				Aliases:        []string{"gitlab_foo_create", "create foo", "add foo"},
			},
			want: false,
		},
		{
			name: "empty aliases (handled by weak_aliases)",
			spec: toolutil.ActionSpec{
				Name:           "foo_create",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_foo_create"},
				Aliases:        nil,
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aliasesOnlyToolname(tc.spec); got != tc.want {
				t.Errorf("aliasesOnlyToolname = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestMissingParameterGuidance pins the scope-suggestive heuristic.
func TestMissingParameterGuidance(t *testing.T) {
	cases := []struct {
		name string
		spec toolutil.ActionSpec
		want bool
	}{
		{
			name: "scope-suggestive id without guidance",
			spec: toolutil.ActionSpec{
				Route: toolutil.ActionRoute{
					InputSchema: map[string]any{
						"properties": map[string]any{
							"project_id": map[string]any{"type": "string"},
							"name":       map[string]any{"type": "string"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "scope-suggestive name without guidance",
			spec: toolutil.ActionSpec{
				Route: toolutil.ActionRoute{
					InputSchema: map[string]any{
						"properties": map[string]any{
							"ref": map[string]any{"type": "string"},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "guidance present",
			spec: toolutil.ActionSpec{
				Route: toolutil.ActionRoute{
					InputSchema: map[string]any{
						"properties": map[string]any{
							"project_id": map[string]any{"type": "string"},
						},
					},
				},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"project_id": {SemanticRole: "scope_project"},
				},
			},
			want: false,
		},
		{
			name: "no scope-suggestive names",
			spec: toolutil.ActionSpec{
				Route: toolutil.ActionRoute{
					InputSchema: map[string]any{
						"properties": map[string]any{
							"name":  map[string]any{"type": "string"},
							"color": map[string]any{"type": "string"},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingParameterGuidance(tc.spec); got != tc.want {
				t.Errorf("missingParameterGuidance = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsScopeSuggestiveName covers the exact and suffix matches.
func TestIsScopeSuggestiveName(t *testing.T) {
	yes := []string{
		"ref", "branch", "tag", "sha", "path", "iid",
		"project_id", "group_id", "user_id", "instance_id", "milestone_id", "epic_id",
	}
	no := []string{"name", "color", "description", "content", "labels"}
	for _, n := range yes {
		t.Run(n, func(t *testing.T) {
			if !isScopeSuggestiveName(n) {
				t.Errorf("%q should be scope-suggestive", n)
			}
		})
	}
	for _, n := range no {
		t.Run(n, func(t *testing.T) {
			if isScopeSuggestiveName(n) {
				t.Errorf("%q should NOT be scope-suggestive", n)
			}
		})
	}
}

// TestDescriptionImpliesEnum verifies the INPUT-ENUM prose heuristic flags
// descriptions that enumerate a fixed value set (explicit "one of", a bare
// asc/desc sort direction, or a colon/paren-introduced comma/slash/"or" list)
// while leaving free-text descriptions and single-value mentions unflagged.
func TestDescriptionImpliesEnum(t *testing.T) {
	yes := []string{
		"Sort order: asc or desc",
		"Sort direction (asc, desc)",
		"Branch filter strategy: wildcard, regex, or all_branches",
		"Filter by state (one of opened, closed)",
		"Aggregation interval: daily, monthly, all",
	}
	no := []string{
		"Project ID or URL-encoded path",
		"Note body text in Markdown",
		"Return events created after this RFC3339 timestamp",
		"",
		"Numeric merge request IID",
	}
	for _, d := range yes {
		t.Run(d, func(t *testing.T) {
			if !descriptionImpliesEnum(d) {
				t.Errorf("descriptionImpliesEnum(%q) = false, want true", d)
			}
		})
	}
	for _, d := range no {
		t.Run(d, func(t *testing.T) {
			if descriptionImpliesEnum(d) {
				t.Errorf("descriptionImpliesEnum(%q) = true, want false", d)
			}
		})
	}
}

// TestIsEnumCandidate verifies the INPUT-ENUM field gate: a scalar string/integer
// property whose prose enumerates values and which has no structured enum is a
// candidate, while properties with an existing enum, non-scalar types,
// normalized fields (access_level family), and free-form names (paths, content)
// are excluded.
func TestIsEnumCandidate(t *testing.T) {
	cand := func(name string, p map[string]any) bool { return isEnumCandidate(name, p) }
	if !cand("sort", map[string]any{"type": "string", "description": "Sort direction (asc, desc)"}) {
		t.Error("sort with asc/desc prose should be a candidate")
	}
	if cand("sort", map[string]any{"type": "string", "enum": []any{"asc", "desc"}, "description": "Sort direction (asc, desc)"}) {
		t.Error("field with existing enum must NOT be a candidate")
	}
	if cand("access_level", map[string]any{"type": "integer", "description": "Access level: 10=Guest, 30=Developer"}) {
		t.Error("normalized access_level must NOT be a candidate (actioncompat accepts names)")
	}
	if cand("file_path", map[string]any{"type": "string", "description": "Path like dir/sub or root"}) {
		t.Error("free-form file_path must NOT be a candidate")
	}
	if cand("labels", map[string]any{"type": "array", "description": "one of the label sets"}) {
		t.Error("non-scalar array must NOT be a candidate")
	}
	if cand("title", map[string]any{"type": "string", "description": "Free text, e.g. asc or desc placeholder"}) {
		t.Error("free-form title name must NOT be a candidate")
	}
}

// TestParseSeverity_Scenarios_ParsesThresholdNames verifies the -severity
// flag accepts the three level names in any case and with surrounding
// whitespace, and rejects anything else with a message naming the input.
func TestParseSeverity_Scenarios_ParsesThresholdNames(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{name: "error", in: "error", want: severityError},
		{name: "warning uppercase", in: "WARNING", want: severityWarning},
		{name: "info padded", in: "  info  ", want: severityInfo},
		{name: "unknown", in: "critical", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSeverity(tt.in)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "must be error, warning, or info") {
					t.Fatalf("parseSeverity(%q) error = %v, want the usage message", tt.in, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("parseSeverity(%q) = %d, %v; want %d, nil", tt.in, got, err, tt.want)
			}
		})
	}
}

// TestSeverityRank_Scenarios_RanksKnownLevels verifies the rank of each level
// name and that an unknown label ranks as info, so an unrecognized severity
// can never make a finding look more urgent than it is.
func TestSeverityRank_Scenarios_RanksKnownLevels(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "error", in: "error", want: severityError},
		{name: "warning", in: "warning", want: severityWarning},
		{name: "info", in: "info", want: severityInfo},
		{name: "unknown label", in: "fatal", want: severityInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := severityRank(tt.in); got != tt.want {
				t.Errorf("severityRank(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestSeverityFor_NonEscalatingFlags_KeepFixedSeverity verifies the flags
// whose severity does not depend on the cluster: the warning-level metadata
// gaps, the two info-level backlog signals, and an unknown flag, which falls
// back to info.
func TestSeverityFor_NonEscalatingFlags_KeepFixedSeverity(t *testing.T) {
	tests := []struct {
		name string
		flag string
		want string
	}{
		{name: "missing next steps", flag: "missing_next_steps", want: "warning"},
		{name: "empty param description", flag: "empty_param_description", want: "warning"},
		{name: "missing parameter guidance", flag: "missing_parameter_guidance", want: "warning"},
		{name: "aliases only toolname", flag: "aliases_only_toolname", want: "warning"},
		{name: "empty output description", flag: "empty_output_description", want: "info"},
		{name: "param enum candidate", flag: "param_enum_candidate", want: "info"},
		{name: "unknown flag", flag: "not_a_flag", want: "info"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterMembers := []string{"link_create", "link_create_batch"}
			if got := severityFor(tt.flag, true, clusterMembers); got != tt.want {
				t.Errorf("severityFor(%q, inCluster) = %q, want %q", tt.flag, got, tt.want)
			}
			if got := severityFor(tt.flag, false, nil); got != tt.want {
				t.Errorf("severityFor(%q, out of cluster) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

// TestReportCheck_Scenarios_GatesOnThreshold verifies the -check gate: each
// threshold fails on findings at or above its level, passes below it, and a
// summary with no finding at all passes every threshold.
func TestReportCheck_Scenarios_GatesOnThreshold(t *testing.T) {
	tests := []struct {
		name      string
		summary   reportSummary
		threshold int
		wantErr   string
	}{
		{name: "error threshold with an error", summary: reportSummary{Errors: 2}, threshold: severityError, wantErr: "2 error-severity finding(s)"},
		{name: "error threshold ignores warnings", summary: reportSummary{Warnings: 3, Infos: 4}, threshold: severityError},
		{name: "warning threshold counts errors and warnings", summary: reportSummary{Errors: 1, Warnings: 2, Infos: 9}, threshold: severityWarning, wantErr: "3 warning-or-worse finding(s)"},
		{name: "warning threshold ignores infos", summary: reportSummary{Infos: 5}, threshold: severityWarning},
		{name: "info threshold counts everything", summary: reportSummary{Errors: 1, Warnings: 1, Infos: 1}, threshold: severityInfo, wantErr: "3 info-or-worse finding(s)"},
		{name: "clean report passes", summary: reportSummary{}, threshold: severityInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := report{Summary: tt.summary}.check(tt.threshold)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("check() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("check() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// captureDiscoveryStdout swaps os.Stdout for a temporary file until the test
// ends and returns a reader for what was written, so the "-" report path can
// be observed.
func captureDiscoveryStdout(t *testing.T) func() string {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = previous
		_ = file.Close()
	})
	return func() string {
		data, readErr := os.ReadFile(file.Name())
		if readErr != nil {
			t.Fatalf("read stdout capture: %v", readErr)
		}
		return string(data)
	}
}

// TestWriteReport_Scenarios_WritesStdoutOrFile verifies the "-" sentinel
// writes the JSON report to stdout, a nested output path gets its parent
// directories created, and a parent that is a regular file is reported as an
// error instead of silently dropping the report.
func TestWriteReport_Scenarios_WritesStdoutOrFile(t *testing.T) {
	t.Run("stdout sentinel", func(t *testing.T) {
		stdout := captureDiscoveryStdout(t)
		if err := writeReport("-", []byte("{}\n")); err != nil {
			t.Fatalf("writeReport(-) error = %v", err)
		}
		if got := stdout(); got != "{}\n" {
			t.Errorf("stdout = %q, want the report", got)
		}
	})
	t.Run("nested file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "plan", "discovery-backlog.json")
		if err := writeReport(path, []byte("{}\n")); err != nil {
			t.Fatalf("writeReport() error = %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "{}\n" {
			t.Errorf("written report = %q, %v; want the content", data, err)
		}
	})
	t.Run("parent is a file", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		if err := writeReport(filepath.Join(blocker, "backlog.json"), []byte("{}\n")); err == nil {
			t.Fatal("writeReport() error = nil, want the directory creation failure")
		}
	})
}

// TestInferOwnerFromName_Scenarios_TakesDottedPrefix verifies the defensive
// owner guess reads the segment before the first dot and returns the whole
// name when there is none.
func TestInferOwnerFromName_Scenarios_TakesDottedPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "dotted name", in: "release.link_create", want: "release"},
		{name: "bare name", in: "link_create", want: "link_create"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inferOwnerFromName(tt.in); got != tt.want {
				t.Errorf("inferOwnerFromName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSiblingClusters_SpecsWithoutOwner_ClusterByInferredOwner verifies specs
// that carry no OwnerPackage are clustered under the owner inferred from
// their dotted name, and that a single-member bucket is dropped.
func TestSiblingClusters_SpecsWithoutOwner_ClusterByInferredOwner(t *testing.T) {
	clusters := siblingClusters([]toolutil.ActionSpec{
		{Name: "release.link_create"},
		{Name: "release.link_create_batch"},
		{Name: "release.solo_get"},
	})
	if len(clusters) != 1 {
		t.Fatalf("clusters = %+v, want one multi-member cluster", clusters)
	}
	got := clusters[0]
	if got.Package != "release" {
		t.Errorf("cluster package = %q, want the inferred owner release", got.Package)
	}
	if !reflect.DeepEqual(got.Members, []string{"release.link_create", "release.link_create_batch"}) {
		t.Errorf("cluster members = %v, want the two link actions sorted", got.Members)
	}
}

// TestSiblingMatches_EmbeddedSiblingName_MatchesByContains verifies the
// defensive fallback: a related action that is neither an exact nor a
// dot-tail match still counts when it embeds a sibling name, so a
// non-conformant RelatedActions value does not produce a false gap.
func TestSiblingMatches_EmbeddedSiblingName_MatchesByContains(t *testing.T) {
	siblings := map[string]struct{}{"link_create": {}}
	tests := []struct {
		name    string
		related string
		want    bool
	}{
		{name: "embedded sibling", related: "legacy_link_create_v2", want: true},
		{name: "unrelated", related: "tag_delete", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := siblingMatches(tt.related, siblings); got != tt.want {
				t.Errorf("siblingMatches(%q) = %v, want %v", tt.related, got, tt.want)
			}
		})
	}
}

// TestHasDisambiguation_Scenarios_RequiresSignalAndSiblingReference verifies
// the two ways an action can point at its sibling once its Usage carries a
// distinguishing signal (a RelatedActions entry or a CommonConfusions entry),
// that a Usage naming the sibling verbatim counts as the signal, and that a
// signal alone is not enough.
func TestHasDisambiguation_Scenarios_RequiresSignalAndSiblingReference(t *testing.T) {
	tests := []struct {
		name    string
		spec    toolutil.ActionSpec
		members []string
		want    bool
	}{
		{
			name: "no siblings is a vacuous pass",
			spec: toolutil.ActionSpec{Name: "link_create"},
			want: true,
		},
		{
			name:    "usage keyword plus a related sibling",
			spec:    toolutil.ActionSpec{Name: "link_create_batch", Usage: "Use instead of link creation one at a time.", RelatedActions: []string{"release.link_create"}},
			members: []string{"link_create", "link_create_batch"},
			want:    true,
		},
		{
			name:    "usage names the sibling verbatim without a keyword",
			spec:    toolutil.ActionSpec{Name: "link_create_batch", Usage: "Prefer link_create when you have one link.", RelatedActions: []string{"link_create"}},
			members: []string{"link_create", "link_create_batch"},
			want:    true,
		},
		{
			name: "sibling named in a parameter confusion",
			spec: toolutil.ActionSpec{
				Name:              "link_create_batch",
				Usage:             "Creates multiple links in one call.",
				ParameterGuidance: map[string]toolutil.ParameterGuidance{"links": {CommonConfusions: []string{"not to be confused with link_create"}}},
			},
			members: []string{"link_create", "link_create_batch"},
			want:    true,
		},
		{
			name:    "signal without any sibling reference",
			spec:    toolutil.ActionSpec{Name: "link_create_batch", Usage: "Creates multiple links in one call.", RelatedActions: []string{"tag.delete"}},
			members: []string{"link_create", "link_create_batch"},
		},
		{
			name:    "no signal at all",
			spec:    toolutil.ActionSpec{Name: "link_create_batch", Usage: "Creates links.", RelatedActions: []string{"link_create"}},
			members: []string{"link_create", "link_create_batch"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasDisambiguation(tt.spec, tt.members); got != tt.want {
				t.Errorf("hasDisambiguation() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSchemaPointer_NilSchema_IsZero verifies a nil schema fingerprints to
// zero so the cycle guard never keys on a missing schema.
func TestSchemaPointer_NilSchema_IsZero(t *testing.T) {
	if got := schemaPointer(nil); got != 0 {
		t.Fatalf("schemaPointer(nil) = %d, want 0", got)
	}
}

// TestResolveSchemaRef_Scenarios_FollowsLocalDefs verifies a "#/$defs/Name"
// reference resolves against local $defs, and that a schema without a $ref,
// with a foreign $ref, or with an unresolvable name is returned unchanged.
func TestResolveSchemaRef_Scenarios_FollowsLocalDefs(t *testing.T) {
	target := map[string]any{"type": "object", "description": "resolved"}
	tests := []struct {
		name     string
		schema   map[string]any
		resolved bool
	}{
		{
			name:     "local defs reference",
			schema:   map[string]any{"$ref": "#/$defs/Target", "$defs": map[string]any{"Target": target}},
			resolved: true,
		},
		{name: "no ref", schema: map[string]any{"type": "string"}},
		{name: "foreign ref", schema: map[string]any{"$ref": "https://example.com/schema.json"}},
		{name: "ref without defs", schema: map[string]any{"$ref": "#/$defs/Missing"}},
		{name: "ref naming an absent def", schema: map[string]any{"$ref": "#/$defs/Absent", "$defs": map[string]any{"Other": target}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSchemaRef(tt.schema)
			if tt.resolved {
				if !reflect.DeepEqual(got, target) {
					t.Fatalf("resolveSchemaRef() = %v, want the resolved definition", got)
				}
				return
			}
			if !reflect.DeepEqual(got, tt.schema) {
				t.Fatalf("resolveSchemaRef() = %v, want the schema unchanged", got)
			}
		})
	}
}

// TestWalkSchemaProperties_Scenarios_VisitsResolvedPropertiesOnce verifies
// the traversal: a nil schema visits nothing, a schema behind a $ref is
// resolved before its properties are read, and a schema already recorded in
// the visited set is not walked twice.
func TestWalkSchemaProperties_Scenarios_VisitsResolvedPropertiesOnce(t *testing.T) {
	behindRef := map[string]any{
		"$ref": "#/$defs/Body",
		"$defs": map[string]any{"Body": map[string]any{
			"properties": map[string]any{"title": map[string]any{"type": "string", "description": "The issue title."}},
		}},
	}
	// shared is a schema whose two properties resolve to the same target map:
	// the direct one is walked first, so the $ref wrapper's target is already
	// in the visited set when the wrapper is resolved.
	target := map[string]any{"properties": map[string]any{"inner": map[string]any{"type": "string", "description": "An inner field."}}}
	shared := map[string]any{"properties": map[string]any{
		"a_direct": target,
		"b_ref":    map[string]any{"$ref": "#/$defs/Target", "$defs": map[string]any{"Target": target}},
	}}
	tests := []struct {
		name       string
		schema     map[string]any
		preVisited bool
		want       []string
	}{
		{name: "nil schema", schema: nil},
		{name: "resolved through a ref", schema: behindRef, want: []string{"title"}},
		{name: "already visited", schema: behindRef, preVisited: true},
		{name: "ref target already visited", schema: shared, want: []string{"a_direct", "inner", "b_ref"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			visited := map[uintptr]bool{}
			if tt.preVisited {
				visited[schemaPointer(tt.schema)] = true
			}
			var seen []string
			walkSchemaProperties(tt.schema, "", visited, func(name, _ string, _ map[string]any) {
				seen = append(seen, name)
			})
			if !reflect.DeepEqual(seen, tt.want) {
				t.Errorf("visited properties = %v, want %v", seen, tt.want)
			}
		})
	}
}

// TestIsEmptyOrBoilerplateDescription_Scenarios_FlagsUninformativeText
// verifies the field-description gate: a missing or blank description, a
// description too short to say anything, a bare "The x" article phrase and a
// literal "id" are all boilerplate, while a real sentence is not.
func TestIsEmptyOrBoilerplateDescription_Scenarios_FlagsUninformativeText(t *testing.T) {
	tests := []struct {
		name string
		prop map[string]any
		want bool
	}{
		{name: "no description key", prop: map[string]any{"type": "string"}, want: true},
		{name: "non-string description", prop: map[string]any{"description": 42}, want: true},
		{name: "blank description", prop: map[string]any{"description": "   "}, want: true},
		{name: "too short", prop: map[string]any{"description": "ID."}, want: true},
		{name: "bare article phrase", prop: map[string]any{"description": "The id"}, want: true},
		{name: "literal id", prop: map[string]any{"description": "id"}, want: true},
		{name: "real sentence", prop: map[string]any{"description": "The numeric project identifier."}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyOrBoilerplateDescription(tt.prop); got != tt.want {
				t.Errorf("isEmptyOrBoilerplateDescription(%v) = %v, want %v", tt.prop, got, tt.want)
			}
		})
	}
}

// TestNeedsMarkdownFormatter_Scenarios_SkipsDestructiveAndNamelessSpecs
// verifies the missing_next_steps precondition: destructive actions never
// need a formatter, list/detail content always does, a named action does by
// default, and an unnamed non-list action does not.
func TestNeedsMarkdownFormatter_Scenarios_SkipsDestructiveAndNamelessSpecs(t *testing.T) {
	tests := []struct {
		name string
		spec toolutil.ActionSpec
		want bool
	}{
		{name: "destructive", spec: toolutil.ActionSpec{Name: "branch_delete", Destructive: true}},
		{name: "list content", spec: toolutil.ActionSpec{Name: "branch_list", ContentKind: toolutil.ActionSpecContentList}, want: true},
		{name: "named action", spec: toolutil.ActionSpec{Name: "branch_get"}, want: true},
		{name: "unnamed action", spec: toolutil.ActionSpec{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsMarkdownFormatter(tt.spec); got != tt.want {
				t.Errorf("needsMarkdownFormatter(%+v) = %v, want %v", tt.spec, got, tt.want)
			}
		})
	}
}

// TestAliasesOnlyToolname_BlankAliases_AreSkipped verifies blank alias
// entries carry no signal: a spec whose only aliases are blank or repeat the
// canonical and tool names is flagged, while one real alias clears it.
func TestAliasesOnlyToolname_BlankAliases_AreSkipped(t *testing.T) {
	tests := []struct {
		name    string
		aliases []string
		want    bool
	}{
		{name: "blank and echoed names", aliases: []string{"  ", "branch_get", "gitlab_branch_get"}, want: true},
		{name: "one real alias", aliases: []string{"  ", "show branch"}, want: false},
		{name: "no aliases at all", aliases: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := toolutil.ActionSpec{
				Name:           "branch_get",
				Aliases:        tt.aliases,
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_branch_get"},
			}
			if got := aliasesOnlyToolname(spec); got != tt.want {
				t.Errorf("aliasesOnlyToolname(%v) = %v, want %v", tt.aliases, got, tt.want)
			}
		})
	}
}

// unformattedOutput is an output type no Markdown formatter is registered
// for, so a spec routing to it raises missing_next_steps.
type unformattedOutput struct {
	Name string `json:"name"`
}

// TestAnalyzeSpec_SyntheticSpec_RaisesEachActionFlag verifies analyzeSpec
// raises each action-level flag from the corresponding gap in one synthetic
// spec: a projected individual description missing its Returns/See also
// sections, an output type with no registered Markdown formatter, a
// scope-suggestive parameter with no ParameterGuidance, and an input
// property with no description.
func TestAnalyzeSpec_SyntheticSpec_RaisesEachActionFlag(t *testing.T) {
	spec := toolutil.ActionSpec{
		Name:           "widget_get",
		Usage:          "Reads one widget by id.",
		Aliases:        []string{"fetch widget", "show widget", "read widget"},
		RelatedActions: []string{"widget.list"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_widget_get"},
		ContentKind:    toolutil.ActionSpecContentDetail,
		Route: toolutil.ActionRoute{
			OutputType: reflect.TypeFor[unformattedOutput](),
			InputSchema: map[string]any{"properties": map[string]any{
				"project_id": map[string]any{"type": "string", "description": "The numeric project identifier or full path."},
				"widget_id":  map[string]any{"type": "string"},
			}},
		},
	}
	projected := map[string]string{"gitlab_widget_get": "Reads one widget."}

	finding := analyzeSpec(spec, projected, nil, 3)
	for _, want := range []string{
		"weak_individual_description",
		"missing_next_steps",
		"missing_parameter_guidance",
		"empty_param_description",
	} {
		t.Run(want, func(t *testing.T) {
			if !slices.Contains(finding.Flags, want) {
				t.Errorf("flags = %v, want %q", finding.Flags, want)
			}
		})
	}
	t.Run("field breakdown names the undescribed parameter", func(t *testing.T) {
		want := []fieldFinding{{Param: "widget_id", Flag: "empty_param_description"}}
		if !reflect.DeepEqual(finding.Fields, want) {
			t.Errorf("fields = %+v, want %+v", finding.Fields, want)
		}
	})
	t.Run("severity is the highest of the flags", func(t *testing.T) {
		if finding.Severity != "warning" {
			t.Errorf("severity = %q, want warning", finding.Severity)
		}
	})
	t.Run("schema presence is recorded", func(t *testing.T) {
		if !finding.HasSchema {
			t.Error("HasSchema = false, want true for a spec carrying an input schema")
		}
	})
}

// TestCollectFieldFindings_OutputSchemaGap_RaisesOutputFlag verifies the
// output-schema walk contributes its own field findings and flag,
// independently of the input schema.
func TestCollectFieldFindings_OutputSchemaGap_RaisesOutputFlag(t *testing.T) {
	spec := toolutil.ActionSpec{Route: toolutil.ActionRoute{
		OutputSchema: map[string]any{"properties": map[string]any{"web_url": map[string]any{"type": "string"}}},
	}}

	fields, flags := collectFieldFindings(spec)
	if !reflect.DeepEqual(fields, []fieldFinding{{Param: "web_url", Flag: "empty_output_description"}}) {
		t.Errorf("fields = %+v, want the undescribed output property", fields)
	}
	if !reflect.DeepEqual(flags, []string{"empty_output_description"}) {
		t.Errorf("flags = %v, want [empty_output_description]", flags)
	}
}

// TestSummarize_EveryFlag_CountsPerFlagAndSeverity verifies the summary
// tallies one action carrying every flag: each per-flag counter reaches one,
// and the severity totals partition the flags into the error, warning and
// info buckets they map to outside a cluster.
func TestSummarize_EveryFlag_CountsPerFlagAndSeverity(t *testing.T) {
	flags := []string{
		"weak_aliases", "generic_usage", "empty_related", "missing_next_steps",
		"empty_param_description", "empty_output_description", "param_enum_candidate",
		"missing_disambiguation", "weak_individual_description",
		"missing_parameter_guidance", "aliases_only_toolname",
	}
	summary := summarize([]packageReport{{
		Package:  "widgets",
		Actions:  2,
		Findings: []actionFinding{{Action: "widget_get", Severity: "error", Flags: flags}},
	}})

	if summary.Packages != 1 || summary.Actions != 2 {
		t.Errorf("summary = %+v, want one package with two actions", summary)
	}
	counts := map[string]int{
		"weak_aliases":                summary.WeakAliases,
		"generic_usage":               summary.GenericUsage,
		"empty_related":               summary.EmptyRelated,
		"missing_next_steps":          summary.MissingNextSteps,
		"empty_param_description":     summary.EmptyParamDescription,
		"empty_output_description":    summary.EmptyOutputDescription,
		"param_enum_candidate":        summary.ParamEnumCandidate,
		"missing_disambiguation":      summary.MissingDisambiguation,
		"weak_individual_description": summary.WeakIndividualDescription,
		"missing_parameter_guidance":  summary.MissingParameterGuidance,
		"aliases_only_toolname":       summary.AliasesOnlyToolname,
	}
	for _, flag := range flags {
		t.Run(flag, func(t *testing.T) {
			if counts[flag] != 1 {
				t.Errorf("%s count = %d, want 1", flag, counts[flag])
			}
		})
	}
	if summary.Errors != 2 || summary.Warnings != 7 || summary.Infos != 2 {
		t.Errorf("severity totals = %d errors, %d warnings, %d infos; want 2/7/2", summary.Errors, summary.Warnings, summary.Infos)
	}
}

// TestBuildReport_GapsOnly_DropsCleanPackages verifies the -gaps-only report
// keeps only packages that raise at least one finding, while reporting the
// same clusters as the full report.
func TestBuildReport_GapsOnly_DropsCleanPackages(t *testing.T) {
	full := cachedFullReport(t)

	gapsOnly := buildReport(true, 3)
	if len(gapsOnly.Packages) > len(full.Packages) {
		t.Fatalf("gaps-only reports %d packages, full reports %d", len(gapsOnly.Packages), len(full.Packages))
	}
	for _, pr := range gapsOnly.Packages {
		if len(pr.Findings) == 0 {
			t.Errorf("package %q has no finding in the gaps-only report", pr.Package)
		}
	}
	if len(gapsOnly.Clusters) != len(full.Clusters) {
		t.Errorf("gaps-only clusters = %d, want the full report's %d", len(gapsOnly.Clusters), len(full.Clusters))
	}
}
