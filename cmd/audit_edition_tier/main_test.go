package main

import (
	"context"
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
		value string
		want  tier
		ok    bool
	}{
		{"Free, Premium, Ultimate", tierFree, true},
		{"Premium, Ultimate", tierPremium, true},
		{"Ultimate", tierUltimate, true},
		{"premium, ultimate", tierPremium, true},
		{"", tierFree, false},
		{"Something else", tierFree, false},
	}
	for _, c := range cases {
		got, ok := parseTierBadge(c.value)
		if got != c.want || ok != c.ok {
			t.Errorf("parseTierBadge(%q) = (%v, %v); want (%v, %v)", c.value, got, ok, c.want, c.ok)
		}
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
