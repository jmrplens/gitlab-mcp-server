package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
)

// TestParseTierBadge_Values_MapToMinimumTier verifies that a doc `- Tier:`
// badge value is reduced to the minimum required tier (the lowest listed tier),
// because a badge lists every tier the endpoint is available in.
func TestParseTierBadge_Values_MapToMinimumTier(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  tier
		ok    bool
	}{
		{"all_tiers_is_free", "Free, Premium, Ultimate", tierFree, true},
		{"premium_and_ultimate_is_premium", "Premium, Ultimate", tierPremium, true},
		{"ultimate_only", "Ultimate", tierUltimate, true},
		{"lowercase_names", "premium, ultimate", tierPremium, true},
		{"empty_value", "", tierFree, false},
		{"unknown_value", "Something else", tierFree, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseTierBadge(c.value)
			if got != c.want || ok != c.ok {
				t.Errorf("parseTierBadge(%q) = (%v, %v); want (%v, %v)", c.value, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestParseDocTiers_PageDefaultAndOverrides verifies that the first details
// block before any heading is the page default and later badges attached to
// sections are collected as distinct overrides.
func TestParseDocTiers_PageDefaultAndOverrides(t *testing.T) {
	doc := `---
title: Projects API
---

{{< details >}}

- Tier: Free, Premium, Ultimate

{{< /details >}}

## Retrieve a project

GET /projects/:id

## Real-time security scan

{{< details >}}

- Tier: Ultimate

{{< /details >}}

POST /projects/:id/security_scan
`
	page, overrides := parseDocTiers(doc)
	if page != tierFree {
		t.Fatalf("page tier = %v; want free", page)
	}
	if len(overrides) != 1 || overrides[0] != tierUltimate {
		t.Fatalf("overrides = %v; want [ultimate]", overrides)
	}
}

// TestParseDocTiers_UniformPremiumPage verifies a page whose only badge is a
// non-Free page default yields that tier with no overrides.
func TestParseDocTiers_UniformPremiumPage(t *testing.T) {
	doc := `---
title: Epics API
---

{{< details >}}

- Tier: Premium, Ultimate

{{< /details >}}

## List all group epics

GET /groups/:id/epics
`
	page, overrides := parseDocTiers(doc)
	if page != tierPremium {
		t.Fatalf("page tier = %v; want premium", page)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides = %v; want none", overrides)
	}
}

// TestClassifyDomain_Buckets verifies the wave classification for the canonical
// domain shapes: green (free, no work), uniform-ee, and mixed.
func TestClassifyDomain_Buckets(t *testing.T) {
	cases := []struct {
		name      string
		dr        domainReport
		overrides []tier
		page      tier
		fetched   bool
		wantClass string
		wantWork  bool
	}{
		{"unmapped", domainReport{}, nil, tierFree, false, "unmapped", true},
		{"green-clean", domainReport{CurrentEnterprise: 0}, nil, tierFree, true, "green", false},
		{"green-needs-ungate", domainReport{CurrentEnterprise: 3}, nil, tierFree, true, "green", true},
		{"uniform-premium", domainReport{}, nil, tierPremium, true, "uniform-ee", true},
		{"mixed", domainReport{}, []tier{tierUltimate}, tierFree, true, "mixed", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotClass, gotWork := classifyDomain(c.dr, c.overrides, c.page, c.fetched)
			if gotClass != c.wantClass || gotWork != c.wantWork {
				t.Errorf("%s: classifyDomain = (%q, %v); want (%q, %v)", c.name, gotClass, gotWork, c.wantClass, c.wantWork)
			}
		})
	}
}

// TestDocOverrideForAction_PrefixMatch verifies that the action-prefix doc-page
// override table redirects the endpoint families whose tier badge does not live
// on the owner package's page, and leaves every other action ungoverned.
func TestDocOverrideForAction_PrefixMatch(t *testing.T) {
	cases := []struct {
		id       string
		wantArea string
		wantUser bool
		wantOK   bool
	}{
		{"group.hook_list", "group_webhooks", false, true},
		{"group.hook_set_custom_header", "group_webhooks", false, true},
		{"merge_request.dependencies_list", "user/project/merge_requests/dependencies", true, true},
		{"merge_request.dependency_create", "user/project/merge_requests/dependencies", true, true},
		{"merge_request.dependency_delete", "user/project/merge_requests/dependencies", true, true},
		{"group.group_board_list", "", false, false},
		{"merge_request.list", "", false, false},
		{"project.hook_list", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			ref, ok := docOverrideForAction(c.id)
			if ok != c.wantOK || ref.area != c.wantArea || ref.userDoc != c.wantUser {
				t.Errorf("docOverrideForAction(%q) = (%+v, %v); want (area %q, userDoc %v, %v)",
					c.id, ref, ok, c.wantArea, c.wantUser, c.wantOK)
			}
		})
	}
}

// TestDocRef_DocPath verifies the repo-relative path rendering used in report
// notes: doc/api/ for API-reference areas and doc/ for user-doc areas.
func TestDocRef_DocPath(t *testing.T) {
	if got := (docRef{area: "group_webhooks"}).docPath(); got != "doc/api/group_webhooks.md" {
		t.Errorf("api docPath = %q; want doc/api/group_webhooks.md", got)
	}
	if got := (docRef{area: "user/project/merge_requests/dependencies", userDoc: true}).docPath(); got != "doc/user/project/merge_requests/dependencies.md" {
		t.Errorf("user docPath = %q; want doc/user/project/merge_requests/dependencies.md", got)
	}
}

// seedDoc writes one cached doc file under dir so an Offline fetcher can serve
// it without network access.
func seedDoc(t *testing.T, dir, area, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(area)+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newOfflineResolver builds a docResolver whose fetchers serve only the docs
// seeded in dir, never touching the network.
func newOfflineResolver(t *testing.T, dir string) *docResolver {
	t.Helper()
	opts := apidocs.Options{Offline: true, CacheDir: dir}
	return newDocResolver(apidocs.New(dir, opts), apidocs.New(dir, opts))
}

const premiumBadgeDoc = `---
title: Some API
---

{{< details >}}

- Tier: Premium, Ultimate

{{< /details >}}

## List things
`

// TestExpectedTierForAction_DocOverride_GradesAgainstReferencedPage verifies
// that an action governed by a doc-page override is graded against that page's
// own badge (doc-grounded) instead of the owner domain's page tier, for both an
// API-reference page (group webhooks) and a user-doc page (MR dependencies).
func TestExpectedTierForAction_DocOverride_GradesAgainstReferencedPage(t *testing.T) {
	dir := t.TempDir()
	seedDoc(t, dir, "group_webhooks", premiumBadgeDoc)
	seedDoc(t, dir, "user/project/merge_requests/dependencies", premiumBadgeDoc)
	res := newOfflineResolver(t, dir)

	cases := []struct {
		id       string
		wantTier tier
		wantNote string
	}{
		{"group.hook_add", tierPremium, "doc/api/group_webhooks.md"},
		{"merge_request.dependencies_list", tierPremium, "doc/user/project/merge_requests/dependencies.md"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			got, note := res.expectedTierForAction(context.Background(), c.id, tierFree)
			if got != c.wantTier {
				t.Errorf("expectedTierForAction(%q) tier = %v; want %v", c.id, got, c.wantTier)
			}
			if !strings.Contains(note, c.wantNote) {
				t.Errorf("expectedTierForAction(%q) note = %q; want it to cite %q", c.id, note, c.wantNote)
			}
		})
	}
}

// TestExpectedTierForAction_DocOverrideFetchFailure_KeepsPageTier verifies that
// when the override page cannot be fetched, the domain page tier is kept and
// the failure is surfaced in the note instead of silently grading the action.
func TestExpectedTierForAction_DocOverrideFetchFailure_KeepsPageTier(t *testing.T) {
	res := newOfflineResolver(t, t.TempDir()) // nothing seeded: every fetch fails

	got, note := res.expectedTierForAction(context.Background(), "group.hook_add", tierFree)
	if got != tierFree {
		t.Errorf("tier on fetch failure = %v; want free (domain page tier)", got)
	}
	if !strings.Contains(note, "doc override fetch failed") {
		t.Errorf("note = %q; want a fetch-failure marker", note)
	}
}

// TestExpectedTierForAction_ExceptionWinsOverPageTier verifies that an audited
// per-action exception still takes precedence over the domain page tier,
// covering a cross-page exception (project push rules) and a live-verified
// correction (merge_request.approval_state: the doc lists it as Free but
// GitLab 19.0.1 CE answers 404, so the audited tier is Premium).
func TestExpectedTierForAction_ExceptionWinsOverPageTier(t *testing.T) {
	res := newOfflineResolver(t, t.TempDir())

	cases := []struct {
		id       string
		wantNote string
	}{
		{"project.push_rule_get", ""},
		{"merge_request.approval_state", "live-verified Premium"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			got, note := res.expectedTierForAction(context.Background(), c.id, tierFree)
			if got != tierPremium {
				t.Errorf("expectedTierForAction(%q) tier = %v; want premium", c.id, got)
			}
			if note == "" {
				t.Errorf("expectedTierForAction(%q) note is empty; want the audited rationale", c.id)
			}
			if c.wantNote != "" && !strings.Contains(note, c.wantNote) {
				t.Errorf("expectedTierForAction(%q) note = %q; want it to contain %q", c.id, note, c.wantNote)
			}
		})
	}
}

// TestTier_String_NamesEveryLevel verifies the JSON names of the three tiers
// and that an out-of-range value falls back to free rather than panicking.
func TestTier_String_NamesEveryLevel(t *testing.T) {
	cases := []struct {
		name string
		in   tier
		want string
	}{
		{name: "free", in: tierFree, want: "free"},
		{name: "premium", in: tierPremium, want: "premium"},
		{name: "ultimate", in: tierUltimate, want: "ultimate"},
		{name: "out_of_range_is_free", in: tier(99), want: "free"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Errorf("tier(%d).String() = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseEditionTier_EditionStrings_MapToTiers verifies the action Edition
// metadata mapping: premium and ultimate in any case or padding map to their
// tiers, and empty, core or unknown values are Free.
func TestParseEditionTier_EditionStrings_MapToTiers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want tier
	}{
		{name: "empty", in: "", want: tierFree},
		{name: "core", in: "core", want: tierFree},
		{name: "premium", in: "premium", want: tierPremium},
		{name: "premium_mixed_case_padded", in: "  Premium ", want: tierPremium},
		{name: "ultimate", in: "ULTIMATE", want: tierUltimate},
		{name: "unknown_is_free", in: "enterprise", want: tierFree},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseEditionTier(tc.in); got != tc.want {
				t.Errorf("parseEditionTier(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestDocAreaForPackage_Packages_ResolveOrReportUnmapped verifies the
// explicit owner-package map: known packages resolve to their doc area and a
// package the map does not know is reported as unmapped rather than guessed.
func TestDocAreaForPackage_Packages_ResolveOrReportUnmapped(t *testing.T) {
	cases := []struct {
		name     string
		pkg      string
		wantArea string
		wantOK   bool
	}{
		{name: "branches", pkg: "branches", wantArea: "branches", wantOK: true},
		{name: "epics", pkg: "epics", wantArea: "epics", wantOK: true},
		{name: "merge_requests", pkg: "mergerequests", wantArea: "merge_requests", wantOK: true},
		{name: "unknown_package", pkg: "no-such-package", wantArea: "", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			area, ok := docAreaForPackage(tc.pkg)
			if area != tc.wantArea || ok != tc.wantOK {
				t.Errorf("docAreaForPackage(%q) = (%q, %v), want (%q, %v)", tc.pkg, area, ok, tc.wantArea, tc.wantOK)
			}
		})
	}
}

// TestParseDocTiers_UnknownBadge_Ignored verifies a badge value naming no
// known tier is skipped: it neither sets the page default nor becomes an
// override, so a later valid badge still grades the page.
func TestParseDocTiers_UnknownBadge_Ignored(t *testing.T) {
	doc := "{{< details >}}\n\n- Tier: Enterprise\n\n{{< /details >}}\n\n## Section\n\n- Tier: Ultimate\n"
	page, overrides := parseDocTiers(doc)
	if page != tierFree {
		t.Errorf("page tier = %v, want free (the unknown badge must not set it)", page)
	}
	if len(overrides) != 1 || overrides[0] != tierUltimate {
		t.Errorf("overrides = %v, want [ultimate]", overrides)
	}
}

// TestPageTier_RepeatedRef_ServedFromMemo verifies the resolver parses each
// override page once: after the first fetch the page tier is served from the
// memo even when the cached file disappears, and a failed fetch is memoized
// as a failure even when the file appears afterwards.
func TestPageTier_RepeatedRef_ServedFromMemo(t *testing.T) {
	dir := t.TempDir()
	res := newOfflineResolver(t, dir)
	ctx := context.Background()

	hooks := docRef{area: "group_webhooks"}
	seedDoc(t, dir, hooks.area, premiumBadgeDoc)
	if got, err := res.pageTier(ctx, hooks); err != nil || got != tierPremium {
		t.Fatalf("first pageTier = (%v, %v), want (premium, nil)", got, err)
	}
	if err := os.Remove(filepath.Join(dir, hooks.area+".md")); err != nil {
		t.Fatalf("remove seeded doc: %v", err)
	}
	if got, err := res.pageTier(ctx, hooks); err != nil || got != tierPremium {
		t.Errorf("memoized pageTier = (%v, %v), want (premium, nil) without re-reading the file", got, err)
	}

	deps := docRef{area: "user/project/merge_requests/dependencies", userDoc: true}
	if _, err := res.pageTier(ctx, deps); err == nil {
		t.Fatal("pageTier of an unseeded user doc returned nil error")
	}
	seedDoc(t, dir, deps.area, premiumBadgeDoc)
	if _, err := res.pageTier(ctx, deps); err == nil {
		t.Error("memoized failure was re-fetched after the doc appeared; want the memoized error")
	}
}

// TestExpectedTierForAction_PlainAction_UsesPageTier verifies an action with
// neither an audited exception nor a doc-page override is graded by its
// owner page tier with no rationale note.
func TestExpectedTierForAction_PlainAction_UsesPageTier(t *testing.T) {
	res := newOfflineResolver(t, t.TempDir())
	cases := []struct {
		id   string
		page tier
	}{
		{id: "branch.list", page: tierFree},
		{id: "epic.list", page: tierPremium},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got, note := res.expectedTierForAction(context.Background(), tc.id, tc.page)
			if got != tc.page || note != "" {
				t.Errorf("expectedTierForAction(%q, %v) = (%v, %q), want (%v, \"\")", tc.id, tc.page, got, note, tc.page)
			}
		})
	}
}

// seededResolver seeds the doc pages the report tests grade against and
// returns an offline resolver over them: branches and groups are page-Free,
// epics is page-Premium, group_webhooks (the group.hook_* override page) is
// Premium, and tags is Ultimate so every Free-gated tag action mismatches.
func seededResolver(t *testing.T) *docResolver {
	t.Helper()
	dir := t.TempDir()
	const freeDoc = "{{< details >}}\n\n- Tier: Free, Premium, Ultimate\n\n{{< /details >}}\n\n## List\n"
	const ultimateDoc = "{{< details >}}\n\n- Tier: Ultimate\n\n{{< /details >}}\n\n## List\n"
	seedDoc(t, dir, "branches", freeDoc)
	seedDoc(t, dir, "groups", freeDoc)
	seedDoc(t, dir, "epics", premiumBadgeDoc)
	seedDoc(t, dir, "group_webhooks", premiumBadgeDoc)
	seedDoc(t, dir, "tags", ultimateDoc)
	return newOfflineResolver(t, dir)
}

// findDomain returns the named domain report or fails the test.
func findDomain(t *testing.T, rep *report, name string) domainReport {
	t.Helper()
	for _, d := range rep.Domains {
		if d.Domain == name {
			return d
		}
	}
	t.Fatalf("domain %q missing from the report (%d domains)", name, len(rep.Domains))
	return domainReport{}
}

// TestBuildReport_SeededDocs_GradesEveryDomain builds the report offline
// against seeded pages and checks the real gradings: a Free page over
// CE-gated actions is green, a Premium page is uniform-ee, a Free page whose
// webhook family is documented Premium elsewhere is mixed with the override
// recorded, an Ultimate page over Free-gated actions mismatches every action,
// an unseeded page is reported as a fetch failure, and a package the doc map
// does not know is listed as unmapped. The summary must add up over the
// domains it aggregates.
func TestBuildReport_SeededDocs_GradesEveryDomain(t *testing.T) {
	rep, err := buildReport(context.Background(), seededResolver(t))
	if err != nil {
		t.Fatalf("buildReport: %v", err)
	}
	if rep.SchemaVersion != schemaVersion || rep.GeneratedAt == "" {
		t.Errorf("report header = version %d at %q, want version %d and a timestamp", rep.SchemaVersion, rep.GeneratedAt, schemaVersion)
	}

	cases := []struct {
		name   string
		assert func(t *testing.T, rep *report)
	}{
		{name: "branches_is_green", assert: assertBranchesGreen},
		{name: "epics_is_uniform_ee", assert: assertEpicsUniformEE},
		{name: "groups_is_mixed_by_webhook_override", assert: assertGroupsMixed},
		{name: "tags_mismatch_against_ultimate_page", assert: assertTagsMismatch},
		{name: "unseeded_page_is_a_fetch_failure", assert: assertIssuesFetchFailure},
		{name: "unmapped_packages_are_listed", assert: assertUnmappedListed},
		{name: "summary_adds_up", assert: assertSummaryAddsUp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.assert(t, rep)
		})
	}
}

// assertBranchesGreen checks the Free-page domain whose actions are all in the
// CE catalog: it is graded free, classified green, and needs no work.
func assertBranchesGreen(t *testing.T, rep *report) {
	t.Helper()
	d := findDomain(t, rep, "branches")
	if !d.DocFetched || d.PageTier != "free" || d.Classification != "green" || d.NeedsWork {
		t.Errorf("branches = %+v, want fetched, page free, green, no work", d)
	}
	if d.Actions == 0 || d.CurrentEnterprise != 0 || d.Mismatches != 0 {
		t.Errorf("branches counts = actions %d, enterprise %d, mismatches %d; want actions, none enterprise, none mismatched",
			d.Actions, d.CurrentEnterprise, d.Mismatches)
	}
	for _, a := range d.ActionDetails {
		if a.Expected != "free" || a.CurrentGate != "free" || a.Mismatch {
			t.Errorf("%s = %+v, want expected free, gate free, no mismatch", a.ID, a)
		}
	}
}

// assertEpicsUniformEE checks the Premium-page domain: every action is graded
// premium, every action is enterprise-gated, and the domain needs tier work.
func assertEpicsUniformEE(t *testing.T, rep *report) {
	t.Helper()
	d := findDomain(t, rep, "epics")
	if !d.DocFetched || d.PageTier != "premium" || d.Classification != "uniform-ee" || !d.NeedsWork {
		t.Errorf("epics = %+v, want fetched, page premium, uniform-ee, needs work", d)
	}
	if d.CurrentEnterprise != d.Actions || d.CurrentFree != 0 {
		t.Errorf("epics gating = %d enterprise of %d, want every action enterprise-gated", d.CurrentEnterprise, d.Actions)
	}
	for _, a := range d.ActionDetails {
		if a.Expected != "premium" {
			t.Errorf("%s expected = %q, want premium", a.ID, a.Expected)
		}
	}
}

// assertGroupsMixed checks the Free-page domain carrying higher-tier actions
// from two different sources: a doc-page override (group webhooks) and an
// audited exception (group push rules). Both grade premium and make the domain
// mixed, with the override tier recorded on the domain.
func assertGroupsMixed(t *testing.T, rep *report) {
	t.Helper()
	d := findDomain(t, rep, "groups")
	if d.PageTier != "free" || d.Classification != "mixed" || !d.NeedsWork {
		t.Errorf("groups = page %q, class %q, work %v; want free, mixed, work", d.PageTier, d.Classification, d.NeedsWork)
	}
	if len(d.OverrideTiers) != 1 || d.OverrideTiers[0] != "premium" {
		t.Errorf("groups override tiers = %v, want [premium]", d.OverrideTiers)
	}
	hook := findAction(t, d, "group.hook_list")
	if hook.Expected != "premium" || !strings.Contains(hook.Note, "doc/api/group_webhooks.md") {
		t.Errorf("group.hook_list = %+v, want premium graded against doc/api/group_webhooks.md", hook)
	}
	pushRule := findAction(t, d, "group.push_rule_get")
	if pushRule.Expected != "premium" || pushRule.Note != docGrpPushRulesPremium {
		t.Errorf("group.push_rule_get = %+v, want the audited premium exception", pushRule)
	}
}

// assertTagsMismatch checks the Ultimate-page domain whose actions are all
// gated Free: every action is graded ultimate and every one is a mismatch.
func assertTagsMismatch(t *testing.T, rep *report) {
	t.Helper()
	d := findDomain(t, rep, "tags")
	if d.PageTier != "ultimate" || d.Classification != "uniform-ee" {
		t.Errorf("tags = page %q, class %q; want ultimate, uniform-ee", d.PageTier, d.Classification)
	}
	if d.Mismatches != d.Actions || d.Actions == 0 {
		t.Errorf("tags mismatches = %d of %d actions, want every Free-gated action mismatched", d.Mismatches, d.Actions)
	}
	for _, a := range d.ActionDetails {
		if a.Expected != "ultimate" || !a.Mismatch {
			t.Errorf("%s = %+v, want expected ultimate and mismatch", a.ID, a)
		}
	}
}

// assertIssuesFetchFailure checks a mapped domain whose page is not cached:
// the failure is surfaced in the note and no action is graded.
func assertIssuesFetchFailure(t *testing.T, rep *report) {
	t.Helper()
	d := findDomain(t, rep, "issues")
	if d.DocFetched || d.Classification != "unmapped" || !d.NeedsWork || !strings.HasPrefix(d.Note, "doc fetch failed: ") {
		t.Errorf("issues = %+v, want unfetched, unmapped, needs work, fetch-failure note", d)
	}
	for _, a := range d.ActionDetails {
		if a.Expected != "" || a.Mismatch {
			t.Errorf("%s = %+v, want ungraded when the page is missing", a.ID, a)
		}
	}
}

// assertUnmappedListed checks that the unmapped-domain list is exactly the set
// of report domains the doc-area map does not know, and that each carries the
// no-mapping note rather than being graded against a guessed page.
func assertUnmappedListed(t *testing.T, rep *report) {
	t.Helper()
	if len(rep.UnmappedDomains) == 0 {
		t.Fatal("no unmapped domains reported; the doc map is documented as deliberately incomplete")
	}
	listed := map[string]bool{}
	for _, name := range rep.UnmappedDomains {
		listed[name] = true
	}
	for _, d := range rep.Domains {
		_, mapped := docAreaForPackage(d.Domain)
		if mapped == listed[d.Domain] {
			t.Errorf("%s: mapped %v but listed as unmapped %v", d.Domain, mapped, listed[d.Domain])
		}
		if !mapped && (d.Note != "no doc-area mapping" || d.Classification != "unmapped" || d.DocArea != "") {
			t.Errorf("%s = %+v, want the no-mapping note and unmapped classification", d.Domain, d)
		}
	}
}

// assertSummaryAddsUp checks the report summary against the domains it
// aggregates: the per-class totals, the classification tally, and the
// domain ordering.
func assertSummaryAddsUp(t *testing.T, rep *report) {
	t.Helper()
	var actions, free, enterprise, mismatches, needWork, classified int
	var prev string
	for _, d := range rep.Domains {
		if d.Domain < prev {
			t.Errorf("domains not sorted: %q after %q", d.Domain, prev)
		}
		prev = d.Domain
		actions += d.Actions
		free += d.CurrentFree
		enterprise += d.CurrentEnterprise
		mismatches += d.Mismatches
		if d.NeedsWork {
			needWork++
		}
	}
	for _, n := range rep.Summary.Classification {
		classified += n
	}
	s := rep.Summary
	if s.Actions != actions || s.CurrentFree != free || s.CurrentEnterprise != enterprise || s.TierMismatches != mismatches ||
		s.DomainsNeedWork != needWork || s.Domains != len(rep.Domains) || classified != len(rep.Domains) {
		t.Errorf("summary %+v does not add up over %d domains (actions %d, free %d, enterprise %d, mismatches %d, work %d)",
			s, len(rep.Domains), actions, free, enterprise, mismatches, needWork)
	}
	if s.CurrentFree+s.CurrentEnterprise != s.Actions || s.Actions == 0 {
		t.Errorf("summary gates %d + %d != actions %d", s.CurrentFree, s.CurrentEnterprise, s.Actions)
	}
}

// findAction returns the named action detail of a domain report or fails.
func findAction(t *testing.T, d domainReport, id string) actionDetail {
	t.Helper()
	for _, a := range d.ActionDetails {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("action %q missing from domain %q (%d actions)", id, d.Domain, len(d.ActionDetails))
	return actionDetail{}
}

// TestRun_OutputModes_WriteFilteredReport verifies the run seam: the report
// goes to stdout as JSON followed by a newline, or to the output file with a
// trailing newline; -gaps-only drops the green domains and keeps the ones
// needing work; and an unwritable output path is reported as a write error.
func TestRun_OutputModes_WriteFilteredReport(t *testing.T) {
	ctx := context.Background()

	t.Run("stdout_full_report", func(t *testing.T) {
		assertStdoutFullReport(t, ctx)
	})

	t.Run("file_gaps_only", func(t *testing.T) {
		assertFileGapsOnly(t, ctx)
	})

	t.Run("unwritable_output_path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing", "edition-tier.json")
		err := run(ctx, seededResolver(t), false, path, &bytes.Buffer{})
		if err == nil || !strings.HasPrefix(err.Error(), "write report: ") {
			t.Fatalf("run to an unwritable path = %v, want a write report error", err)
		}
	})
}

// assertStdoutFullReport runs the seam with the stdout sentinel and checks the
// whole report arrives as JSON terminated by a newline, green domains included.
func assertStdoutFullReport(t *testing.T, ctx context.Context) {
	t.Helper()
	var out bytes.Buffer
	if err := run(ctx, seededResolver(t), false, "-", &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !bytes.HasSuffix(out.Bytes(), []byte("}\n")) {
		t.Errorf("stdout should end with the JSON object and a newline, got %q", out.String()[max(out.Len()-5, 0):])
	}
	var rep report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	findDomain(t, &rep, "branches")
	findDomain(t, &rep, "tags")
}

// assertFileGapsOnly runs the seam with -gaps-only to a file and checks the
// green domains are dropped from the listing while the summary still counts
// every domain, and that stdout stays empty.
func assertFileGapsOnly(t *testing.T, ctx context.Context) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edition-tier.json")
	var out bytes.Buffer
	if err := run(ctx, seededResolver(t), true, path, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should stay empty when writing a file, got %q", out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Error("report file lacks the trailing newline")
	}
	var rep report
	if unmarshalErr := json.Unmarshal(data, &rep); unmarshalErr != nil {
		t.Fatalf("report file is not JSON: %v", unmarshalErr)
	}
	for _, d := range rep.Domains {
		if !d.NeedsWork {
			t.Errorf("gaps-only report kept %s, which needs no work", d.Domain)
		}
		if d.Domain == "branches" {
			t.Error("gaps-only report kept the green branches domain")
		}
	}
	findDomain(t, &rep, "tags")
	if rep.Summary.Domains == len(rep.Domains) {
		t.Errorf("summary domains = %d should still count every domain, not the %d kept", rep.Summary.Domains, len(rep.Domains))
	}
}
