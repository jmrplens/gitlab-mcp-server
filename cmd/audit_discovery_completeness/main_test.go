// main_test.go contains unit tests for the discovery-completeness auditor.
// The gold-standard fixtures pin the link_create_batch BEFORE/AFTER signature
// so future regressions in the check logic (or in the underlying specs) are
// caught by CI without running the full eval surface.
package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releaselinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
		// A name that is nothing but a suffix strips to the empty string: with
		// an owner the stem falls back to it, and without one the action is
		// kept as it stands so the cluster stays its own.
		"release._all": "release",
		"_all":         "_all",
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
	client, cleanup := auditshared.NewStubGitLabClient(auditshared.StubToken)
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
	client2, cleanup2 := auditshared.NewStubGitLabClient(auditshared.StubToken)
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

// specWithSchema builds a spec whose input schema declares the given
// properties, all of them required, so a case table can state the parameter
// names it cares about and nothing else.
func specWithSchema(required ...string) toolutil.ActionSpec {
	props := make(map[string]any, len(required))
	for _, name := range required {
		props[name] = map[string]any{"type": "string"}
	}
	return toolutil.ActionSpec{Route: toolutil.ActionRoute{InputSchema: map[string]any{
		"properties": props,
		"required":   required,
	}}}
}

// TestUnguidedRequiredIdentifiers pins the rewritten check. The old question
// ("any guidance at all, given a scope-suggestive parameter") could not fire,
// because internal/tools fills the canonical scope defaults into every spec
// before this auditor reads one; these cases hold the two halves of the
// replacement: guidance no richer than that central fill, and a required
// identifier the fill does not cover.
func TestUnguidedRequiredIdentifiers(t *testing.T) {
	withGuidance := func(spec toolutil.ActionSpec, guidance map[string]toolutil.ParameterGuidance) toolutil.ActionSpec {
		spec.ParameterGuidance = guidance
		return spec
	}
	// The central fill's own entry for project_id, read from the fill rather
	// than spelled here, so the two cases below can differ from it in exactly
	// one field and the comparison is held field by field: whichever field is
	// dropped from it, or crossed with its neighbor, one of them notices.
	central := toolutil.FillScopeParameterGuidanceSingle(specWithSchema("project_id")).ParameterGuidance["project_id"]
	otherBinding, otherConfusions := central, central
	otherBinding.ExampleBinding = `params.project_id:42`
	otherConfusions.CommonConfusions = []string{"not the group that owns it"}
	cases := []struct {
		name string
		spec toolutil.ActionSpec
		want []string
	}{
		{
			name: "required identifier beside a centrally filled scope",
			spec: toolutil.FillScopeParameterGuidanceSingle(specWithSchema("project_id", "hook_id")),
			want: []string{"hook_id"},
		},
		{
			name: "several identifiers are all named, sorted",
			spec: specWithSchema("token_id", "agent_id"),
			want: []string{"agent_id", "token_id"},
		},
		{
			name: "authored guidance anywhere clears the action",
			spec: withGuidance(specWithSchema("project_id", "hook_id"), map[string]toolutil.ParameterGuidance{
				"hook_id": {SemanticRole: "system_hook_id"},
			}),
			want: nil,
		},
		{
			name: "guidance richer than the central default is authored",
			spec: withGuidance(specWithSchema("project_id", "hook_id"), map[string]toolutil.ParameterGuidance{
				"project_id": {SemanticRole: "scope_project", CommonConfusions: []string{"not the group"}},
			}),
			want: nil,
		},
		{
			name: "an example binding of the author's own is authored guidance",
			spec: withGuidance(specWithSchema("project_id", "hook_id"), map[string]toolutil.ParameterGuidance{
				"project_id": otherBinding,
			}),
			want: nil,
		},
		{
			name: "a confusion added to the central entry is authored guidance",
			spec: withGuidance(specWithSchema("project_id", "hook_id"), map[string]toolutil.ParameterGuidance{
				"project_id": otherConfusions,
			}),
			want: nil,
		},
		{
			name: "only scope-suggestive parameters, all covered by the fill",
			spec: toolutil.FillScopeParameterGuidanceSingle(specWithSchema("project_id", "ref", "iid")),
			want: nil,
		},
		{
			name: "an identifier the action does not require is not a finding",
			spec: toolutil.ActionSpec{Route: toolutil.ActionRoute{InputSchema: map[string]any{
				"properties": map[string]any{"hook_id": map[string]any{"type": "string"}},
			}}},
			want: nil,
		},
		{
			name: "required list naming an undeclared property is ignored",
			spec: toolutil.ActionSpec{Route: toolutil.ActionRoute{InputSchema: map[string]any{
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
				"required":   []any{"hook_id", 7},
			}}},
			want: nil,
		},
		{
			name: "name, key and slug are content rather than identifiers",
			spec: specWithSchema("name", "key", "slug"),
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unguidedRequiredIdentifiers(tc.spec); !slices.Equal(got, tc.want) {
				t.Errorf("unguidedRequiredIdentifiers = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRequiredParameterNames_JSONSpelling verifies the required list is read
// whether a schema built in Go spells it []string or one decoded from JSON
// spells it []any, and that a schema without the key names nothing.
func TestRequiredParameterNames_JSONSpelling(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
		want   []string
	}{
		{name: "go slice", schema: map[string]any{"required": []string{"hook_id"}}, want: []string{"hook_id"}},
		{name: "decoded slice", schema: map[string]any{"required": []any{"hook_id", 7}}, want: []string{"hook_id"}},
		{name: "absent", schema: map[string]any{}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiredParameterNames(tc.schema); !slices.Equal(got, tc.want) {
				t.Errorf("requiredParameterNames = %v, want %v", got, tc.want)
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
	if cand("approval_rules", map[string]any{"type": "string", "description": "Approval rule kind: any, code_owner, or report_approver"}) {
		t.Error("normalized approval_rules must NOT be a candidate")
	}
	if cand("deploy_access_levels", map[string]any{"type": "string", "description": "Access kind: developer or maintainer"}) {
		t.Error("normalized deploy_access_levels must NOT be a candidate (it carries access_level)")
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
// gaps, the three info-level backlog signals, and an unknown flag, which
// falls back to info.
func TestSeverityFor_NonEscalatingFlags_KeepFixedSeverity(t *testing.T) {
	tests := []struct {
		name string
		flag string
		want string
	}{
		{name: "missing next steps", flag: "missing_next_steps", want: "warning"},
		{name: "empty param description", flag: "empty_param_description", want: "warning"},
		{name: "aliases only toolname", flag: "aliases_only_toolname", want: "warning"},
		{name: "missing parameter guidance", flag: "missing_parameter_guidance", want: "info"},
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
		// The three counts differ so that the total names which three were
		// added: with one of each, a sum that read one counter twice reached
		// the same 3 and passed.
		{name: "info threshold counts everything", summary: reportSummary{Errors: 1, Warnings: 2, Infos: 4}, threshold: severityInfo, wantErr: "7 info-or-worse finding(s)"},
		{name: "clean report passes", summary: reportSummary{}, threshold: severityInfo},
		// -severity is parsed before the report is built, so a threshold
		// outside the three levels cannot reach here from the command line;
		// the gate still has to let it through rather than fail on findings
		// it was not asked about.
		{name: "a threshold outside the three levels gates nothing", summary: reportSummary{Errors: 9, Warnings: 9, Infos: 9}, threshold: severityInfo + 1},
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
	// Package and Stem are both strings and both derived from the same name,
	// so the record says which is which only where the two differ.
	if got.Stem != "release.link" {
		t.Errorf("cluster stem = %q, want release.link", got.Stem)
	}
	if !reflect.DeepEqual(got.Members, []string{"release.link_create", "release.link_create_batch"}) {
		t.Errorf("cluster members = %v, want the two link actions sorted", got.Members)
	}
}

// TestSiblingMatches_EmbeddedSiblingName_IsNotAMatch verifies the tolerance
// that is gone. A related action that is neither the sibling, nor the dot-tail
// spelling of it, nor the underscore one, used to count whenever a sibling
// name appeared anywhere inside it, so a cluster with a sibling called
// link_create counted legacy_link_create_v2 as a cross-link to it.
//
// The tolerance was written when nothing held a RelatedActions entry to the
// catalog. `make check-action-ids` does now, so every entry reaching here is a
// canonical ID in one of the three accepted forms, and what a substring match
// can still do is report a package complete on the strength of one.
func TestSiblingMatches_EmbeddedSiblingName_IsNotAMatch(t *testing.T) {
	siblings := map[string]struct{}{"link_create": {}}
	tests := []struct {
		name    string
		related string
		want    bool
	}{
		{name: "embedded sibling", related: "legacy_link_create_v2", want: false},
		{name: "sibling as a substring of a longer action", related: "release.link_create_batch", want: false},
		{name: "unrelated", related: "tag_delete", want: false},
		{name: "the sibling itself", related: "link_create", want: true},
		{name: "the canonical ID of the sibling", related: "release.link_create", want: true},
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
			name: "a parameter confusion about something else is not a sibling reference",
			spec: toolutil.ActionSpec{
				Name:              "link_create_batch",
				Usage:             "Creates multiple links in one call.",
				ParameterGuidance: map[string]toolutil.ParameterGuidance{"links": {CommonConfusions: []string{"not to be confused with a tag"}}},
			},
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
		// Both length tests are pinned on the character they turn on, so a
		// boundary moved by one changes an answer here rather than landing
		// between two fixtures that read the same either way.
		{name: "the shortest description that says something", prop: map[string]any{"description": "Name"}, want: false},
		{name: "bare article phrase", prop: map[string]any{"description": "The id"}, want: true},
		{name: "the longest article phrase", prop: map[string]any{"description": "The abc"}, want: true},
		{name: "one character past the article phrase", prop: map[string]any{"description": "The abcd"}, want: false},
		{name: "a literal id is caught by its length", prop: map[string]any{"description": "id"}, want: true},
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
		Usage:          "  Reads one widget by id.  ",
		Aliases:        []string{"fetch widget", "show widget", "read widget"},
		RelatedActions: []string{"widget.list"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_widget_get"},
		ContentKind:    toolutil.ActionSpecContentDetail,
		Route: toolutil.ActionRoute{
			OutputType: reflect.TypeFor[unformattedOutput](),
			InputSchema: map[string]any{
				"properties": map[string]any{
					"project_id": map[string]any{"type": "string", "description": "The numeric project identifier or full path."},
					"widget_id":  map[string]any{"type": "string"},
				},
				"required": []string{"project_id", "widget_id"},
			},
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
	t.Run("field breakdown names the parameter behind each flag", func(t *testing.T) {
		want := []fieldFinding{
			{Param: "widget_id", Flag: "empty_param_description"},
			{Param: "widget_id", Flag: "missing_parameter_guidance"},
		}
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
	// The three identity fields are all strings copied off the same spec, so
	// nothing but a fixture in which no two of them agree can tell the
	// canonical ID from the tool name a reader would look the action up by.
	// The usage is padded in the fixture so the trim is pinned here too.
	t.Run("the finding names the action, its tool and its trimmed usage", func(t *testing.T) {
		if finding.Action != "widget_get" || finding.Tool != "gitlab_widget_get" || finding.Usage != "Reads one widget by id." {
			t.Errorf("finding identity = (%q, %q, %q), want (widget_get, gitlab_widget_get, the trimmed usage)",
				finding.Action, finding.Tool, finding.Usage)
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
// tallies every flag into its own counter and partitions the flags into the
// error, warning and info buckets they map to outside a cluster, and that a
// flag the tally does not know counts towards no per-flag total while still
// ranking as info.
//
// Each flag is carried by a different number of findings on purpose. With one
// finding per flag every counter read one, so a counter wired to its
// neighbor's case, the one defect this tally can have, was invisible: the
// eleven increments are the same statement eleven times over and only the
// number each lands on tells them apart.
func TestSummarize_EveryFlag_CountsPerFlagAndSeverity(t *testing.T) {
	flags := []string{
		"weak_aliases", "generic_usage", "empty_related", "missing_next_steps",
		"empty_param_description", "empty_output_description", "param_enum_candidate",
		"missing_disambiguation", "weak_individual_description",
		"missing_parameter_guidance", "aliases_only_toolname",
	}
	var findings []actionFinding
	for i, flag := range flags {
		for range i + 1 {
			findings = append(findings, actionFinding{Action: flag, Severity: "error", Flags: []string{flag}})
		}
	}
	findings = append(findings, actionFinding{Action: "widget_get", Severity: "info", Flags: []string{"not_a_flag"}})

	summary := summarize([]packageReport{{Package: "widgets", Actions: 2, Findings: findings}})

	want := reportSummary{
		Packages:                  1,
		Actions:                   2,
		WeakAliases:               1,
		GenericUsage:              2,
		EmptyRelated:              3,
		MissingNextSteps:          4,
		EmptyParamDescription:     5,
		EmptyOutputDescription:    6,
		ParamEnumCandidate:        7,
		MissingDisambiguation:     8,
		WeakIndividualDescription: 9,
		MissingParameterGuidance:  10,
		AliasesOnlyToolname:       11,
		Errors:                    10,
		Warnings:                  33,
		Infos:                     24,
	}
	if summary != want {
		t.Errorf("summary = %+v, want %+v", summary, want)
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

// TestWeakAliases_BlankAlias_CarriesNoSignal verifies a blank entry in the
// alias list counts for nothing. An alias that is empty or whitespace reaches
// no model, so counting it would clear the flag with a list that says as
// little as an empty one.
func TestWeakAliases_BlankAlias_CarriesNoSignal(t *testing.T) {
	spec := toolutil.ActionSpec{
		Name:           "release.link_create_batch",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
		Aliases:        []string{"", "   ", "create multiple release links"},
	}
	if !weakAliases(spec, 2) {
		t.Error("two blank aliases beside one real one should still be weak at minAliases=2")
	}
	if weakAliases(spec, 1) {
		t.Error("the one real alias should clear the flag at minAliases=1")
	}
}

// TestSiblingSet_BlankMembersAndSelf_AreDropped verifies the sibling set a
// disambiguation question is asked against: the action itself is never its own
// sibling, whatever case or padding the cluster spells it with, and a blank
// member contributes nothing rather than an empty name every usage string
// would trivially contain.
func TestSiblingSet_BlankMembersAndSelf_AreDropped(t *testing.T) {
	got := siblingSet([]string{"", "   ", "  Link_Create_Batch ", "link_create"}, "link_create_batch")
	want := map[string]struct{}{"link_create": {}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("siblingSet() = %v, want only link_create", got)
	}
}

// TestAnalyzeSpec_VariantAmongCRUDSiblings_IsNotAskedToDisambiguate verifies
// the third condition of the disambiguation gate: a variant action is only
// asked to distinguish itself where the cluster holds a variant to be confused
// with. A cluster of plain CRUD verbs is not that, so the action passes even
// though its own name carries a variant suffix and it says nothing about its
// siblings.
func TestAnalyzeSpec_VariantAmongCRUDSiblings_IsNotAskedToDisambiguate(t *testing.T) {
	variant := toolutil.ActionSpec{
		Name:           "widget_publish_directory",
		OwnerPackage:   "widgets",
		Usage:          "Publishes a widget.",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_widget_publish_directory"},
	}
	crudOnly := []string{"widget_publish_get", "widget_publish_list"}

	finding := analyzeSpec(variant, nil, crudOnly, 3)
	if containsStr(finding.Flags, "missing_disambiguation") {
		t.Errorf("flags = %v, want no missing_disambiguation for a cluster with no other variant", finding.Flags)
	}
}

// TestBuildPackageReports_CleanAction_IsCountedAndNeverAFinding verifies the
// two filters between a spec and the report: an action that raises no flag is
// counted in its package's action total and contributes no finding, and a
// package whose actions are all clean is dropped from a gaps-only report while
// a package with a finding survives both.
//
// No action of the live catalog is clean today, so nothing that reads it can
// reach either filter; the groups are planted here instead.
func TestBuildPackageReports_CleanAction_IsCountedAndNeverAFinding(t *testing.T) {
	clean := toolutil.ActionSpec{
		Name:           "widget_get",
		OwnerPackage:   "widgets",
		Usage:          "Reads one widget by id.",
		Aliases:        []string{"fetch widget", "show widget", "read widget"},
		RelatedActions: []string{"widget.list"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_widget_get"},
	}
	flagged := toolutil.ActionSpec{
		Name:           "gadget_get",
		OwnerPackage:   "gadgets",
		Usage:          "Use to execute gadgets domain action.",
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_gadget_get"},
	}
	groups := []tools.ActionSpecGroup{
		{ToolName: "gitlab_widget", OwnerPackage: "widgets", Actions: []toolutil.ActionSpec{clean}},
		{ToolName: "gitlab_gadget", OwnerPackage: "gadgets", Actions: []toolutil.ActionSpec{flagged}},
	}
	projected := map[string]string{
		"gitlab_widget_get": "Reads one widget. Returns: widget. See also: gitlab_widget_list.",
	}

	t.Run("the full report keeps the clean package with no finding", func(t *testing.T) {
		got := buildPackageReports(groups, nil, projected, 3, false)
		if len(got) != 2 {
			t.Fatalf("packages = %+v, want both the clean and the flagged one", got)
		}
		if !reflect.DeepEqual(got[1], packageReport{Package: "widgets", Actions: 1}) {
			t.Errorf("clean package = %+v, want one action and no finding", got[1])
		}
		if got[0].Package != "gadgets" || len(got[0].Findings) != 1 {
			t.Errorf("flagged package = %+v, want one finding under gadgets", got[0])
		}
	})

	t.Run("gaps-only drops the clean package and keeps the flagged one", func(t *testing.T) {
		got := buildPackageReports(groups, nil, projected, 3, true)
		if len(got) != 1 || got[0].Package != "gadgets" {
			t.Fatalf("gaps-only packages = %+v, want only gadgets", got)
		}
	})
}

// TestBuildPackageReports_ClusteredAction_CarriesItsSiblings verifies the join
// between a spec and the cluster it belongs to: the finding names the whole
// membership, and the variant that says nothing about its sibling is raised to
// error severity by it.
//
// The cluster is looked up by owner package and action name, both strings and
// both off the same spec, so a lookup given them the other way round finds
// nothing and every finding in the report comes back with no cluster and a
// severity the escalation never reached. Nothing else here reads a cluster off
// a built report, so that crossing used to pass the whole suite.
func TestBuildPackageReports_ClusteredAction_CarriesItsSiblings(t *testing.T) {
	single := toolutil.ActionSpec{
		Name:           "link_create",
		OwnerPackage:   "releaselinks",
		Usage:          "Creates one release asset link.",
		Aliases:        []string{"add release link", "attach asset", "create asset link"},
		RelatedActions: []string{"release.link_list"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create"},
	}
	batch := toolutil.ActionSpec{
		Name:           "link_create_batch",
		OwnerPackage:   "releaselinks",
		Usage:          "Creates release asset links.",
		Aliases:        []string{"add release links", "attach assets", "create asset links"},
		RelatedActions: []string{"release.link_list"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_release_link_create_batch"},
	}
	groups := []tools.ActionSpecGroup{{
		ToolName:     "gitlab_release_link",
		OwnerPackage: "releaselinks",
		Actions:      []toolutil.ActionSpec{single, batch},
	}}

	got := buildPackageReports(groups, collectAllClusters(groups), nil, 3, true)

	if len(got) != 1 || len(got[0].Findings) != 1 {
		t.Fatalf("packages = %+v, want one finding: the variant that says nothing about its sibling", got)
	}
	finding := got[0].Findings[0]
	if !reflect.DeepEqual(finding.Cluster, []string{"link_create", "link_create_batch"}) {
		t.Errorf("cluster = %v, want both members of the releaselinks link cluster", finding.Cluster)
	}
	if finding.Action != "link_create_batch" || !containsStr(finding.Flags, "missing_disambiguation") || finding.Severity != "error" {
		t.Errorf("finding = %+v, want link_create_batch flagged missing_disambiguation at error severity", finding)
	}
}

// TestMain_Scenarios_WritesTheReportOrGatesOnIt drives the command line main
// assembles. The default run writes the JSON document where -output names it
// and asks for no exit; -check writes no report at all and asks the process to
// exit 1 only when the threshold it was given is one findings reach, naming
// the count on stderr. The exit code is the whole of what a caller reads from
// this command, and nothing below main observes it.
//
// The threshold is what separates the two -check answers rather than a
// contrived catalog: the tree carries no error-severity finding today and
// plenty of info-level ones, so the same report passes one gate and fails the
// other. The clean half says so out loud and skips rather than fails if that
// stops being true, because turning it into a demand would make this test a
// gate on the whole catalog that nothing asked it to be.
func TestMain_Scenarios_WritesTheReportOrGatesOnIt(t *testing.T) {
	tests := []struct {
		name              string
		extraArgs         []string
		wantExit          int
		wantStderr        string
		wantReport        bool
		needsCleanCatalog bool
	}{
		{name: "the default run writes the report", wantExit: notExited, wantReport: true},
		{name: "the gate passes when nothing reaches the threshold", extraArgs: []string{"-check"}, wantExit: notExited, needsCleanCatalog: true},
		{
			name:       "the gate exits 1 and says how many",
			extraArgs:  []string{"-check", "-severity", "info"},
			wantExit:   1,
			wantStderr: "info-or-worse finding(s) present",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.needsCleanCatalog && cachedFullReport(t).Summary.Errors > 0 {
				t.Skip("the catalog carries error-severity findings, so the passing branch cannot be driven from it")
			}
			dir := t.TempDir()
			reportPath := filepath.Join(dir, "discovery-backlog.json")
			args := append([]string{"audit_discovery_completeness", "-gaps-only", "-output", reportPath}, tt.extraArgs...)

			exited, logged := runMain(t, dir, args)

			if exited != tt.wantExit {
				t.Errorf("main() asked to exit %d, want %d", exited, tt.wantExit)
			}
			assertReportWritten(t, reportPath, tt.wantReport)
			switch {
			case tt.wantStderr == "" && logged != "":
				t.Errorf("stderr = %q, want nothing on the writing path", logged)
			case tt.wantStderr != "" && !strings.Contains(logged, tt.wantStderr):
				t.Errorf("stderr = %q, want it to contain %q", logged, tt.wantStderr)
			}
		})
	}
}

// notExited is the code runMain reports when main returned without asking the
// process to exit at all, which no real exit status can be.
const notExited = -1

// runMain drives main with args, os.Exit behind a stub and os.Stderr pointed
// at a file under dir, and returns the code main asked for and everything it
// wrote to stderr. The flag set is replaced too: main declares its flags on
// the default one, which would panic on the second call of a case table.
func runMain(t *testing.T, dir string, args []string) (exited int, logged string) {
	t.Helper()
	stderrPath := filepath.Join(dir, "stderr.txt")
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create the stderr capture: %v", err)
	}

	oldArgs, oldStderr, oldFlags := os.Args, os.Stderr, flag.CommandLine
	exited = notExited
	osExit = func(code int) { exited = code }
	os.Args, os.Stderr = args, stderrFile
	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(stderrFile)
	t.Cleanup(func() {
		os.Args, os.Stderr, flag.CommandLine = oldArgs, oldStderr, oldFlags
		osExit = os.Exit
		stderrFile.Close()
	})

	main()

	written, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read the stderr capture: %v", err)
	}
	return exited, string(written)
}

// assertReportWritten checks the document at path against what the run was
// supposed to leave there: a parsable report over a non-empty backlog, or no
// file at all where the gate answers instead of reporting.
func assertReportWritten(t *testing.T, path string, want bool) {
	t.Helper()
	written, err := os.ReadFile(path)
	if !want {
		if err == nil {
			t.Errorf("the gate wrote %d bytes to %s, want no report", len(written), path)
		}
		return
	}
	if err != nil {
		t.Fatalf("read the written report: %v", err)
	}
	var got report
	if unmarshalErr := json.Unmarshal(written, &got); unmarshalErr != nil {
		t.Fatalf("the written report does not parse: %v", unmarshalErr)
	}
	if got.SchemaVersion != schemaVersion || len(got.Packages) == 0 {
		t.Errorf("written report = schema %d over %d packages, want schema %d and a non-empty backlog",
			got.SchemaVersion, len(got.Packages), schemaVersion)
	}
}
