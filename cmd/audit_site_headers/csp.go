// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"
)

// recommendedPolicy is the Content-Security-Policy the site is to be served
// with, written for what the built site loads (read on 2026-10-26 from its
// 216 pages): its own scripts, styles, fonts, search index and worker, the
// inline scripts Starlight writes for the theme, the sidebar and the tabs,
// Pagefind's WebAssembly, data: images, and the four hosts of the install
// badges on the getting started page. Loaded with this policy in Chromium,
// every page rendered with no violation, and the theme switch and the search
// worked. It is one literal rather than a concatenation, because a constant
// expression lies outside every block the coverage profile records, and a
// mutation of it would be reported as not covered.
const recommendedPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://img.shields.io https://cursor.com https://kiro.dev https://files.lmstudio.ai; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// policy is one serialized policy: each directive, its name lower-cased, with
// the source expressions after it. A directive named twice keeps its first
// occurrence, as a browser does.
type policy map[string][]string

// judgeContentSecurityPolicy judges the Content-Security-Policy fields of a
// response. A browser enforces every policy they carry, so a response is held
// to its strictest: one policy that is not permissive is enough, and when none
// is, the reason given is the first policy's.
func judgeContentSecurityPolicy(values []string) string {
	first := ""
	for _, value := range values {
		for _, each := range parsePolicies(value) {
			reason := judgePolicy(each)
			if reason == "" {
				return ""
			}
			if first == "" {
				first = reason
			}
		}
	}
	return first
}

// parsePolicies splits one field into its policies, one per comma, and each
// policy into its directives, one per semicolon.
func parsePolicies(field string) []policy {
	var policies []policy
	for serialized := range strings.SplitSeq(field, ",") {
		parsed := policy{}
		for directive := range strings.SplitSeq(serialized, ";") {
			tokens := strings.Fields(directive)
			if len(tokens) == 0 {
				continue
			}
			name := strings.ToLower(tokens[0])
			if _, seen := parsed[name]; !seen {
				parsed[name] = tokens[1:]
			}
		}
		policies = append(policies, parsed)
	}
	return policies
}

// judgePolicy says why one policy is permissive, or "" when it is not. The
// sources of scripts and of plugins are read from script-src and object-src,
// or from default-src where either is absent, which is the fallback a browser
// applies. A script element is governed by script-src-elem where a policy has
// one, in a browser that implements CSP Level 3, and by script-src or
// default-src in one that does not, so the hosts of script-src-elem are judged
// as well; 'unsafe-eval' means nothing there, since script-src alone governs
// evaluation.
func judgePolicy(p policy) string {
	directive, scripts := p.sources("script-src")
	if directive == "" {
		return "governs no script: it has neither script-src nor default-src"
	}
	for _, source := range scripts {
		lower := strings.ToLower(source)
		if lower == "'unsafe-eval'" {
			return fmt.Sprintf("lets a script run text as code: %s allows %s", directive, source)
		}
		if admitsAnyHost(lower) {
			return fmt.Sprintf("lets a script load from any host: %s allows %s", directive, source)
		}
	}
	for _, source := range p["script-src-elem"] {
		if admitsAnyHost(strings.ToLower(source)) {
			return "lets a script load from any host: script-src-elem allows " + source
		}
	}
	if _, plugins := p.sources("object-src"); len(plugins) != 1 || !strings.EqualFold(plugins[0], "'none'") {
		return "lets a plugin load: neither object-src nor default-src is 'none'"
	}
	return ""
}

// sources returns the source list that governs what name names, and the
// directive it came from: name itself, default-src in its absence, or "" when
// neither is present.
func (p policy) sources(name string) (directive string, list []string) {
	if own, found := p[name]; found {
		return name, own
	}
	if fallback, found := p["default-src"]; found {
		return "default-src", fallback
	}
	return "", nil
}

// admitsAnyHost reports whether a lower-cased source expression admits a
// resource from any host: the bare wildcard, a scheme alone (https:, data:),
// or a host-source whose host is the wildcard, with or without a scheme, a
// port or a path. A keyword, a hash or a nonce is quoted and admits no host,
// and a subdomain wildcard names the domain it admits.
func admitsAnyHost(source string) bool {
	if strings.HasPrefix(source, "'") {
		return false
	}
	scheme, rest, hasScheme := strings.Cut(source, ":")
	if hasScheme && scheme != "" && rest == "" {
		return true
	}
	host := source
	if after, found := strings.CutPrefix(rest, "//"); found {
		host = after
	}
	host, _, _ = strings.Cut(host, "/")
	host, _, _ = strings.Cut(host, ":")
	return host == "*"
}
