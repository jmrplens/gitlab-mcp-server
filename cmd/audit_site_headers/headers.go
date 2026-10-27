// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// cloudflareRule is what a pending header waits for: the documentation site
// is a GitHub Pages build behind Cloudflare, and Pages lets a site set none of
// its own response headers, so only a Cloudflare rule can add them. It is one
// literal rather than a concatenation, because a constant expression lies
// outside every block the coverage profile records, and a mutation of it would
// be reported as not covered.
const cloudflareRule = "the response header rule for docs.jmrp.io in Cloudflare, which only the maintainer can set (docs/development/repository-settings.md#documentation-site-response-headers)"

// minimumMaxAge is the least Strict-Transport-Security max-age that holds: a
// year, in seconds, which is also the least the HSTS preload list accepts.
const minimumMaxAge = 31536000

// rule is one hardening header the site must send: the value to set when it
// is missing, the judge of a value sent, which returns why it is permissive or
// "" when it is not, and, for a header the site does not serve yet, what it
// waits for. A rule with no pending reason is required.
type rule struct {
	name        string
	recommended string
	judge       func(values []string) string
	pending     string
}

// siteHeaders are the four headers hardened_site names, in the order the
// criterion names them. Strict-Transport-Security and X-Content-Type-Options
// are served today (read on 2026-10-26), so they are required.
// Content-Security-Policy and X-Frame-Options wait for the Cloudflare rule;
// once the report says one is served, its pending reason is removed here, so
// that losing it fails the check from then on.
var siteHeaders = []rule{
	{name: "Content-Security-Policy", recommended: recommendedPolicy, judge: judgeContentSecurityPolicy, pending: cloudflareRule},
	{name: "Strict-Transport-Security", recommended: "max-age=31536000; includeSubDomains; preload", judge: judgeStrictTransportSecurity},
	{name: "X-Content-Type-Options", recommended: "nosniff", judge: judgeContentTypeOptions},
	{name: "X-Frame-Options", recommended: "DENY", judge: judgeFrameOptions, pending: cloudflareRule},
}

// verdict is what one page's response says of one rule: the header's fields
// as sent, none when it was not, and why the value is permissive, "" when it
// is not or was not sent.
type verdict struct {
	rule   rule
	served []string
	reason string
}

// missing reports whether the header was not sent at all.
func (v verdict) missing() bool { return len(v.served) == 0 }

// holds reports whether the header was sent with a value that is not
// permissive.
func (v verdict) holds() bool { return !v.missing() && v.reason == "" }

// fails reports whether the verdict fails the run: a required header that
// does not hold. A pending one never does.
func (v verdict) fails() bool { return v.rule.pending == "" && !v.holds() }

// judgePage gives one verdict per rule, in the order of rules, from the
// headers of one response.
func judgePage(header http.Header, rules []rule) []verdict {
	verdicts := make([]verdict, 0, len(rules))
	for _, each := range rules {
		judged := verdict{rule: each, served: header.Values(each.name)}
		if !judged.missing() {
			judged.reason = each.judge(judged.served)
		}
		verdicts = append(verdicts, judged)
	}
	return verdicts
}

// judgeStrictTransportSecurity judges the first Strict-Transport-Security
// field, the only one a browser reads (RFC 6797, section 8.1). It is
// permissive without a max-age a browser can read, with one under
// [minimumMaxAge], or when it names max-age twice, which makes a browser
// ignore the header (section 6.1).
func judgeStrictTransportSecurity(values []string) string {
	ages := []string{}
	for directive := range strings.SplitSeq(values[0], ";") {
		name, value, _ := strings.Cut(directive, "=")
		if strings.EqualFold(strings.TrimSpace(name), "max-age") {
			ages = append(ages, strings.Trim(strings.TrimSpace(value), `"`))
		}
	}
	if len(ages) == 0 {
		return "has no max-age, so a browser ignores it"
	}
	if len(ages) > 1 {
		return "names max-age twice, so a browser ignores it"
	}
	seconds, err := strconv.ParseUint(ages[0], 10, 63)
	if err != nil {
		return fmt.Sprintf("sets max-age %q, which is not a number of seconds, so a browser ignores it", ages[0])
	}
	if seconds < minimumMaxAge {
		return fmt.Sprintf("sets max-age=%d, under a year (%d seconds), the least the HSTS preload list accepts", seconds, minimumMaxAge)
	}
	return ""
}

// judgeContentTypeOptions judges X-Content-Type-Options: every field must be
// nosniff, the one value the header defines.
func judgeContentTypeOptions(values []string) string {
	for _, value := range values {
		if !strings.EqualFold(strings.TrimSpace(value), "nosniff") {
			return fmt.Sprintf("is %q, and only nosniff stops a browser guessing the type", value)
		}
	}
	return ""
}

// judgeFrameOptions judges X-Frame-Options as the HTML standard's processing
// model reads it: the fields split on commas and lower-cased into a set, which
// refuses a frame from another site when it is deny or sameorigin alone, or
// when it holds more than one value and one of them is deny, sameorigin or
// allowall. Any other set, ALLOW-FROM and ALLOWALL alone included, lets the
// page be framed anywhere.
func judgeFrameOptions(values []string) string {
	set := map[string]bool{}
	for _, value := range values {
		for each := range strings.SplitSeq(value, ",") {
			set[strings.ToLower(strings.TrimSpace(each))] = true
		}
	}
	known := set["deny"] || set["sameorigin"]
	if len(set) == 1 && known || len(set) > 1 && (known || set["allowall"]) {
		return ""
	}
	return fmt.Sprintf("is %q, and a browser refuses a frame from another site only for DENY or SAMEORIGIN", strings.Join(values, ", "))
}
