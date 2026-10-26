package main

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// TestJudgeStrictTransportSecurity_Values_HoldOrSayWhy verifies the judge of
// Strict-Transport-Security: a max-age of a year or more holds whatever the
// case of the directive or the quotes around its value, the first field is the
// one a browser reads, and a header without max-age, with a max-age that is
// not a number, under a year, or named twice is permissive with the reason.
func TestJudgeStrictTransportSecurity_Values_HoldOrSayWhy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "the site's value", values: []string{"max-age=31536000; includeSubDomains; preload"}},
		{name: "two years", values: []string{"max-age=63072000"}},
		{name: "a quoted value", values: []string{`max-age="31536000"`}},
		{name: "an upper-case directive", values: []string{"MAX-AGE=31536000"}},
		{name: "spaces around the directives", values: []string{" includeSubDomains ;  max-age = 31536000 "}},
		{name: "the first field holds", values: []string{"max-age=31536000", "max-age=0"}},
		{name: "the first field is read", values: []string{"max-age=0", "max-age=31536000"}, want: "sets max-age=0, under a year (31536000 seconds), the least the HSTS preload list accepts"},
		{name: "a day", values: []string{"max-age=86400"}, want: "sets max-age=86400, under a year (31536000 seconds), the least the HSTS preload list accepts"},
		{name: "a second short of a year", values: []string{"max-age=31535999"}, want: "sets max-age=31535999, under a year (31536000 seconds), the least the HSTS preload list accepts"},
		{name: "no max-age", values: []string{"includeSubDomains; preload"}, want: "has no max-age, so a browser ignores it"},
		{name: "an empty value", values: []string{""}, want: "has no max-age, so a browser ignores it"},
		{name: "not a number", values: []string{"max-age=forever"}, want: `sets max-age "forever", which is not a number of seconds, so a browser ignores it`},
		{name: "a negative number", values: []string{"max-age=-1"}, want: `sets max-age "-1", which is not a number of seconds, so a browser ignores it`},
		{name: "max-age twice", values: []string{"max-age=31536000; max-age=31536000"}, want: "names max-age twice, so a browser ignores it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := judgeStrictTransportSecurity(tc.values); got != tc.want {
				t.Errorf("judgeStrictTransportSecurity(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// TestJudgeContentTypeOptions_Values_HoldOnlyAsNosniff verifies that
// X-Content-Type-Options holds when every field is nosniff, in any case and
// with surrounding space, and that any other field is named in the reason.
func TestJudgeContentTypeOptions_Values_HoldOnlyAsNosniff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "nosniff", values: []string{"nosniff"}},
		{name: "another case and spaces", values: []string{" NoSniff "}},
		{name: "twice", values: []string{"nosniff", "nosniff"}},
		{name: "another value", values: []string{"sniff"}, want: `is "sniff", and only nosniff stops a browser guessing the type`},
		{name: "a second field", values: []string{"nosniff", "none"}, want: `is "none", and only nosniff stops a browser guessing the type`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := judgeContentTypeOptions(tc.values); got != tc.want {
				t.Errorf("judgeContentTypeOptions(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// TestJudgeFrameOptions_Values_HoldWhenABrowserRefusesTheFrame verifies the
// judge of X-Frame-Options against the HTML standard's processing model: DENY
// and SAMEORIGIN hold in any case, a header naming more than one value holds
// when one of them is deny, sameorigin or allowall (the standard then refuses
// every frame), and a lone value the standard does not act on, ALLOW-FROM
// included, or a lone ALLOWALL is permissive with the reason.
func TestJudgeFrameOptions_Values_HoldWhenABrowserRefusesTheFrame(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "DENY", values: []string{"DENY"}},
		{name: "sameorigin", values: []string{" sameorigin "}},
		{name: "the same value twice", values: []string{"DENY", "deny"}},
		{name: "two values in one field", values: []string{"DENY, SAMEORIGIN"}},
		{name: "a conflict with allowall", values: []string{"ALLOWALL", "DENY"}},
		{name: "a conflict with an unknown value", values: []string{"ALLOW-FROM https://example.com", "sameorigin"}},
		{name: "allowall beside an unknown value", values: []string{"ALLOWALL, ALLOW-FROM https://example.com"}},
		{name: "ALLOW-FROM alone", values: []string{"ALLOW-FROM https://example.com"}, want: `is "ALLOW-FROM https://example.com", and a browser refuses a frame from another site only for DENY or SAMEORIGIN`},
		{name: "ALLOWALL alone", values: []string{"ALLOWALL"}, want: `is "ALLOWALL", and a browser refuses a frame from another site only for DENY or SAMEORIGIN`},
		{name: "two values the standard does not act on", values: []string{"ALLOW-FROM a", "ALLOW-FROM b"}, want: `is "ALLOW-FROM a, ALLOW-FROM b", and a browser refuses a frame from another site only for DENY or SAMEORIGIN`},
		{name: "an empty value", values: []string{""}, want: `is "", and a browser refuses a frame from another site only for DENY or SAMEORIGIN`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := judgeFrameOptions(tc.values); got != tc.want {
				t.Errorf("judgeFrameOptions(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// TestSiteHeaders_Declarations_NameTheFourHeadersAndHoldTheirOwnValues holds
// the declarations to the criterion: the four headers it names, in its order,
// each recommended value judged nonpermissive by its own judge, the two the
// site serves today required, and the two only the Cloudflare rule can add
// declared pending with the reason that names it.
func TestSiteHeaders_Declarations_NameTheFourHeadersAndHoldTheirOwnValues(t *testing.T) {
	t.Parallel()
	names := make([]string, 0, len(siteHeaders))
	pending := map[string]string{}
	for _, each := range siteHeaders {
		names = append(names, each.name)
		if each.pending != "" {
			pending[each.name] = each.pending
		}
		t.Run(each.name, func(t *testing.T) {
			t.Parallel()
			if each.name != http.CanonicalHeaderKey(each.name) {
				t.Errorf("%s is not written canonically", each.name)
			}
			if reason := each.judge([]string{each.recommended}); reason != "" {
				t.Errorf("the recommended %s %q is judged permissive: %s", each.name, each.recommended, reason)
			}
		})
	}
	want := []string{"Content-Security-Policy", "Strict-Transport-Security", "X-Content-Type-Options", "X-Frame-Options"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("the declared headers are %v, want %v", names, want)
	}
	wantPending := map[string]string{"Content-Security-Policy": cloudflareRule, "X-Frame-Options": cloudflareRule}
	if !reflect.DeepEqual(pending, wantPending) {
		t.Errorf("the pending headers are %v, want %v", pending, wantPending)
	}
	if !strings.Contains(cloudflareRule, "docs/development/repository-settings.md#documentation-site-response-headers") {
		t.Errorf("the pending reason %q does not name the page that says what to set", cloudflareRule)
	}
}

// TestJudgePage_AResponse_GivesOneVerdictPerRule verifies that a page is
// judged rule by rule in the order declared: a header sent is read with every
// field it was sent in, whatever the case of its name on the wire, and a
// header not sent has no served value and no reason.
func TestJudgePage_AResponse_GivesOneVerdictPerRule(t *testing.T) {
	t.Parallel()
	header := http.Header{}
	header.Add("x-content-type-options", "nosniff")
	header.Add("X-Frame-Options", "ALLOWALL")
	header.Add("X-Frame-Options", "ALLOWALL")
	rules := []rule{
		{name: "X-Content-Type-Options", judge: judgeContentTypeOptions},
		{name: "X-Frame-Options", judge: judgeFrameOptions, pending: "later"},
		{name: "Strict-Transport-Security", judge: judgeStrictTransportSecurity},
	}
	got := judgePage(header, rules)
	if len(got) != len(rules) {
		t.Fatalf("judgePage gave %d verdicts, want %d", len(got), len(rules))
	}
	wants := []struct {
		served []string
		reason string
	}{
		{served: []string{"nosniff"}},
		{served: []string{"ALLOWALL", "ALLOWALL"}, reason: `is "ALLOWALL, ALLOWALL", and a browser refuses a frame from another site only for DENY or SAMEORIGIN`},
		{},
	}
	for i, want := range wants {
		t.Run(rules[i].name, func(t *testing.T) {
			t.Parallel()
			if got[i].rule.name != rules[i].name || !reflect.DeepEqual(got[i].served, want.served) || got[i].reason != want.reason {
				t.Errorf("verdict %d = {%s %q %q}, want {%s %q %q}", i, got[i].rule.name, got[i].served, got[i].reason, rules[i].name, want.served, want.reason)
			}
		})
	}
}

// TestVerdict_States_DecideWhatFails verifies the three questions a report
// asks of a verdict: a header sent with a nonpermissive value holds, and one
// missing or permissive fails only when it is required, never while it is
// pending the rule only the maintainer can set.
func TestVerdict_States_DecideWhatFails(t *testing.T) {
	t.Parallel()
	required := rule{name: "X-Content-Type-Options"}
	pending := rule{name: "X-Frame-Options", pending: "later"}
	cases := []struct {
		name                   string
		verdict                verdict
		missing, holds, failed bool
	}{
		{name: "required and served", verdict: verdict{rule: required, served: []string{"nosniff"}}, holds: true},
		{name: "required and missing", verdict: verdict{rule: required}, missing: true, failed: true},
		{name: "required and permissive", verdict: verdict{rule: required, served: []string{"x"}, reason: "why"}, failed: true},
		{name: "pending and served", verdict: verdict{rule: pending, served: []string{"DENY"}}, holds: true},
		{name: "pending and missing", verdict: verdict{rule: pending}, missing: true},
		{name: "pending and permissive", verdict: verdict{rule: pending, served: []string{"x"}, reason: "why"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.verdict.missing(); got != tc.missing {
				t.Errorf("missing() = %t, want %t", got, tc.missing)
			}
			if got := tc.verdict.holds(); got != tc.holds {
				t.Errorf("holds() = %t, want %t", got, tc.holds)
			}
			if got := tc.verdict.fails(); got != tc.failed {
				t.Errorf("fails() = %t, want %t", got, tc.failed)
			}
		})
	}
}
