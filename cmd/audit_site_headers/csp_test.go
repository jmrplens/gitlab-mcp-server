package main

import (
	"reflect"
	"testing"
)

// TestJudgeContentSecurityPolicy_Policies_HoldOrSayWhy verifies the judge of
// Content-Security-Policy: a policy holds when it governs scripts, its script
// sources admit no host at large and no evaluation of text as code, and
// plugins are refused. The sources come from script-src or, without it, from
// default-src, and the hosts of a script-src-elem, which a CSP3 browser reads
// for a script element in place of them, are judged as well. The first
// occurrence of a directive wins, names are read in any case, and the policies
// a response carries are all enforced, so one that holds is enough.
// 'unsafe-inline' and 'wasm-unsafe-eval' hold, because the site's own inline
// scripts and its search index need them.
func TestJudgeContentSecurityPolicy_Policies_HoldOrSayWhy(t *testing.T) {
	t.Parallel()
	const noPlugins = "; object-src 'none'"
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "the recommended policy", values: []string{recommendedPolicy}},
		{name: "default-src alone", values: []string{"default-src 'self'" + noPlugins}},
		{name: "default-src 'none' covers plugins too", values: []string{"default-src 'none'"}},
		{name: "upper-case names and keywords", values: []string{"SCRIPT-SRC 'self'; OBJECT-SRC 'NONE'"}},
		{name: "hosts, hashes and nonces", values: []string{"script-src https://cdn.example.com *.example.com example.com:443 'sha256-abc' 'nonce-x' 'strict-dynamic'" + noPlugins}},
		{name: "the first directive wins", values: []string{"script-src 'self'; script-src *" + noPlugins}},
		{name: "a second policy in the same field holds", values: []string{"script-src *, default-src 'self'" + noPlugins}},
		{name: "a second field holds", values: []string{"script-src *", "default-src 'none'"}},
		{name: "empty directives between semicolons", values: []string{";; script-src 'self' ;" + noPlugins + ";"}},
		{name: "no script directive", values: []string{"object-src 'none'"}, want: "governs no script: it has neither script-src nor default-src"},
		{name: "an empty value", values: []string{""}, want: "governs no script: it has neither script-src nor default-src"},
		{name: "a wildcard", values: []string{"script-src 'self' *" + noPlugins}, want: "lets a script load from any host: script-src allows *"},
		{name: "a scheme", values: []string{"script-src 'self' https:" + noPlugins}, want: "lets a script load from any host: script-src allows https:"},
		{name: "data through default-src", values: []string{"default-src 'self' data:"}, want: "lets a script load from any host: default-src allows data:"},
		{name: "a wildcard host behind a scheme", values: []string{"script-src https://*" + noPlugins}, want: "lets a script load from any host: script-src allows https://*"},
		{name: "a wildcard host with a port", values: []string{"script-src *:443" + noPlugins}, want: "lets a script load from any host: script-src allows *:443"},
		{name: "script-src-elem naming hosts", values: []string{"script-src 'self'; script-src-elem 'self' https://cdn.example.com" + noPlugins}},
		{name: "script-src-elem beside default-src alone", values: []string{"default-src 'self'; script-src-elem 'self'" + noPlugins}},
		{name: "a scheme in script-src-elem", values: []string{"default-src 'self'; script-src 'self'; script-src-elem 'self' https:" + noPlugins}, want: "lets a script load from any host: script-src-elem allows https:"},
		{name: "a wildcard in script-src-elem", values: []string{"script-src 'self'; Script-Src-Elem *" + noPlugins}, want: "lets a script load from any host: script-src-elem allows *"},
		{name: "a wildcard in script-src beside script-src-elem", values: []string{"script-src *; script-src-elem 'self'" + noPlugins}, want: "lets a script load from any host: script-src allows *"},
		{name: "evaluation", values: []string{"script-src 'self' 'unsafe-eval'" + noPlugins}, want: "lets a script run text as code: script-src allows 'unsafe-eval'"},
		{name: "plugins without default-src", values: []string{"script-src 'self'"}, want: "lets a plugin load: neither object-src nor default-src is 'none'"},
		{name: "plugins from the site", values: []string{"script-src 'self'; object-src 'self'"}, want: "lets a plugin load: neither object-src nor default-src is 'none'"},
		{name: "plugins with more than none", values: []string{"script-src 'self'; object-src 'none' 'self'"}, want: "lets a plugin load: neither object-src nor default-src is 'none'"},
		{name: "every policy is permissive", values: []string{"script-src *", "object-src 'none'"}, want: "lets a script load from any host: script-src allows *"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := judgeContentSecurityPolicy(tc.values); got != tc.want {
				t.Errorf("judgeContentSecurityPolicy(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

// TestParsePolicies_AField_SplitsPoliciesAndDirectives verifies the parse the
// judge stands on: a field carries one policy per comma, each directive is a
// lower-cased name and the sources after it, the first occurrence of a name is
// kept, and an empty policy is still a policy.
func TestParsePolicies_AField_SplitsPoliciesAndDirectives(t *testing.T) {
	t.Parallel()
	got := parsePolicies("Script-Src 'self'  https://a.example ; script-src *; upgrade-insecure-requests, ")
	want := []policy{
		{"script-src": {"'self'", "https://a.example"}, "upgrade-insecure-requests": {}},
		{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePolicies = %#v, want %#v", got, want)
	}
}

// TestAdmitsAnyHost_Sources_RecognizeEveryFormThatNamesNoHost verifies which
// source expressions admit any host: the bare wildcard, a scheme alone, and a
// host that is itself the wildcard, with or without a scheme or a port. A
// keyword, a hash, a named host and a subdomain wildcard name what they admit.
func TestAdmitsAnyHost_Sources_RecognizeEveryFormThatNamesNoHost(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"*":                     true,
		"https:":                true,
		"data:":                 true,
		"blob:":                 true,
		"https://*":             true,
		"https://*:443":         true,
		"*:443":                 true,
		"https://*/path":        true,
		"'self'":                false,
		"'unsafe-inline'":       false,
		"'sha256-abc'":          false,
		"https://cdn.example":   false,
		"cdn.example":           false,
		"cdn.example:443":       false,
		"*.example.com":         false,
		"https://*.example.com": false,
		":":                     false,
	}
	for source, want := range cases {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if got := admitsAnyHost(source); got != want {
				t.Errorf("admitsAnyHost(%q) = %t, want %t", source, got, want)
			}
		})
	}
}
