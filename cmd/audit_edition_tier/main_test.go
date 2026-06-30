package main

import (
	"net/http"
	"testing"
	"time"
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
		gotClass, gotWork := classifyDomain(c.dr, c.overrides, c.page, c.fetched)
		if gotClass != c.wantClass || gotWork != c.wantWork {
			t.Errorf("%s: classifyDomain = (%q, %v); want (%q, %v)", c.name, gotClass, gotWork, c.wantClass, c.wantWork)
		}
	}
}

// TestParseRetryAfter covers the Retry-After header forms: delta-seconds,
// HTTP-date in the future, and the unparsable/empty cases that fall back to 0
// (exponential backoff).
func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("5"); got != 5*time.Second {
		t.Errorf("delta-seconds: got %v, want 5s", got)
	}
	if got := parseRetryAfter("  10 "); got != 10*time.Second {
		t.Errorf("padded delta-seconds: got %v, want 10s", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Errorf("empty: got %v, want 0", got)
	}
	if got := parseRetryAfter("0"); got != 0 {
		t.Errorf("zero: got %v, want 0", got)
	}
	if got := parseRetryAfter("garbage"); got != 0 {
		t.Errorf("garbage: got %v, want 0", got)
	}
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(future); got <= 0 || got > 31*time.Second {
		t.Errorf("http-date: got %v, want ~30s", got)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Errorf("past http-date: got %v, want 0", got)
	}
}

// TestBackoffDelay verifies Retry-After is honored (and capped) when present,
// and that the exponential fallback grows with the attempt and stays within
// [base, base*1.5) given full jitter on half the window.
func TestBackoffDelay(t *testing.T) {
	// Retry-After honored and capped at fetchMaxBackoff.
	if got := backoffDelay(1, 3*time.Second); got < 3*time.Second || got > 3*time.Second+fetchBaseSpacing {
		t.Errorf("retry-after honored: got %v, want ~3s", got)
	}
	if got := backoffDelay(1, 10*time.Minute); got > fetchMaxBackoff+fetchBaseSpacing {
		t.Errorf("retry-after cap: got %v, want <= %v", got, fetchMaxBackoff+fetchBaseSpacing)
	}
	// Exponential fallback: attempt 1 -> base 1s, attempt 3 -> base 4s.
	d1 := backoffDelay(1, 0)
	if d1 < time.Second || d1 >= time.Second+time.Second/2 {
		t.Errorf("attempt 1 backoff out of range: %v", d1)
	}
	d3 := backoffDelay(3, 0)
	if d3 < 4*time.Second || d3 >= 6*time.Second {
		t.Errorf("attempt 3 backoff out of range: %v", d3)
	}
}
