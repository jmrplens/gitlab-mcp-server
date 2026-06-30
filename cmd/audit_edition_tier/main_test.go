package main

import "testing"

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
		gotClass, gotWork := classifyDomain(c.dr, c.overrides, c.page, c.fetched)
		if gotClass != c.wantClass || gotWork != c.wantWork {
			t.Errorf("%s: classifyDomain = (%q, %v); want (%q, %v)", c.name, gotClass, gotWork, c.wantClass, c.wantWork)
		}
	}
}
