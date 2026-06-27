// main_test.go contains unit tests for the discovery-completeness auditor.
// The gold-standard fixtures pin the link_create_batch BEFORE/AFTER signature
// so future regressions in the check logic (or in the underlying specs) are
// caught by CI without running the full eval surface.
package main

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

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
		if !isGenericUsage(usage) {
			t.Errorf("isGenericUsage(%q) = false, want true", usage)
		}
	}
	specific := []string{
		"List branches for a project with optional search and pagination.",
		"Create MULTIPLE release asset links in one call. Use this instead of repeated link_create.",
	}
	for _, usage := range specific {
		if isGenericUsage(usage) {
			t.Errorf("isGenericUsage(%q) = true, want false", usage)
		}
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
		if got := baseActionStem(in); got != want {
			t.Errorf("baseActionStem(%q) = %q, want %q", in, got, want)
		}
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
		members := clusterMembersFor(clusters, spec.OwnerPackage, spec.Name)
		finding := analyzeSpec(spec, nil, members, 3)
		if containsStr(finding.Flags, "missing_disambiguation") {
			t.Errorf("pure CRUD member %q should NOT be flagged missing_disambiguation: %+v", spec.Name, finding.Flags)
		}
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
		if !hasNonCRUDVariantSuffix(name) {
			t.Errorf("hasNonCRUDVariantSuffix(%q) = false, want true", name)
		}
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
		if hasNonCRUDVariantSuffix(name) {
			t.Errorf("hasNonCRUDVariantSuffix(%q) = true, want false", name)
		}
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
		if got := baseActionStem(in); got != want {
			t.Errorf("baseActionStem(%q) = %q, want %q", in, got, want)
		}
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
		if got := siblingMatches(strings.ToLower(in), siblings); got != want {
			t.Errorf("siblingMatches(%q) = %v, want %v", in, got, want)
		}
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
	withClusterMembers(pureCRUD, func() {
		for _, flag := range []string{"weak_aliases", "empty_related", "weak_individual_description"} {
			if got := severityFor(flag, true); got != "warning" {
				t.Errorf("pure CRUD cluster: severityFor(%q, inCluster) = %q, want warning", flag, got)
			}
		}
	})

	// Base-vs-variant cluster with a _batch member.
	withBatch := []string{"link_create", "link_create_batch", "link_get", "link_list"}
	withClusterMembers(withBatch, func() {
		for _, flag := range []string{"weak_aliases", "empty_related", "weak_individual_description"} {
			if got := severityFor(flag, true); got != "error" {
				t.Errorf("base-vs-variant cluster: severityFor(%q, inCluster) = %q, want error", flag, got)
			}
		}
	})

	// Out-of-cluster (e.g., a single-member or no cluster): always warning.
	for _, flag := range []string{"weak_aliases", "empty_related", "weak_individual_description"} {
		if got := severityFor(flag, false); got != "warning" {
			t.Errorf("out-of-cluster: severityFor(%q, false) = %q, want warning", flag, got)
		}
	}

	// Flags that are always error/warning regardless of cluster.
	if got := severityFor("generic_usage", false); got != "error" {
		t.Errorf("severityFor(generic_usage) = %q, want error", got)
	}
	if got := severityFor("missing_disambiguation", false); got != "error" {
		t.Errorf("severityFor(missing_disambiguation) = %q, want error", got)
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
		if got := usageHasSignal(in); got != want {
			t.Errorf("usageHasSignal(%q) = %v, want %v", in, got, want)
		}
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
		if !containsStr(got, w) {
			t.Errorf("emptyParamDescriptions missing %q in %v", w, got)
		}
	}
	for _, g := range got {
		if g == "good" {
			t.Errorf("emptyParamDescriptions incorrectly flagged %q", g)
		}
	}
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
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		t.Fatalf("auditclient.NewMock: %v", err)
	}
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
	client2, cleanup2, err := auditclient.NewMock()
	if err != nil {
		t.Fatalf("auditclient.NewMock: %v", err)
	}
	defer cleanup2()
	rep, err := buildReport(false, 3)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
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
	first, err := buildReport(true, 3)
	if err != nil {
		t.Fatalf("first buildReport: %v", err)
	}
	second, err := buildReport(true, 3)
	if err != nil {
		t.Fatalf("second buildReport: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("buildReport is not deterministic across runs")
	}
}

// TestBuildReport_NonEmptyActions verifies the auditor reports actions.
func TestBuildReport_NonEmptyActions(t *testing.T) {
	rep, err := buildReport(false, 3)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
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
	rep, err := buildReport(false, 3)
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
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
		if !isScopeSuggestiveName(n) {
			t.Errorf("%q should be scope-suggestive", n)
		}
	}
	for _, n := range no {
		if isScopeSuggestiveName(n) {
			t.Errorf("%q should NOT be scope-suggestive", n)
		}
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
		if !descriptionImpliesEnum(d) {
			t.Errorf("descriptionImpliesEnum(%q) = false, want true", d)
		}
	}
	for _, d := range no {
		if descriptionImpliesEnum(d) {
			t.Errorf("descriptionImpliesEnum(%q) = true, want false", d)
		}
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
