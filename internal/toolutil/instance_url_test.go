// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// instance_url_test.go holds InstanceWebURL and PathBelowInstanceWeb to the
// rule GitLab's own SubmoduleHelper applies to a URL on the instance: a
// prefix match on the web URL, relative root included, for http and https
// alone, on the instance's scheme, host and port.
package toolutil

import (
	"net/url"
	"testing"
)

// mustParseURL parses a fixture URL or stops the test.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return u
}

// TestInstanceWebURL_DropsTheAPIPathAndKeepsTheRoot verifies that the web
// URL is the API base less its api/v4 suffix, with the relative root kept,
// a trailing slash always present, and the userinfo, query and fragment an
// operator may have configured left out.
func TestInstanceWebURL_DropsTheAPIPathAndKeepsTheRoot(t *testing.T) {
	cases := []struct{ base, want string }{
		{"https://gitlab.example.com/api/v4/", "https://gitlab.example.com/"},
		{"https://gitlab.example.com/api/v4", "https://gitlab.example.com/"},
		{"https://example.com/gitlab/api/v4/", "https://example.com/gitlab/"},
		{"http://example.com:8080/a/b/api/v4/", "http://example.com:8080/a/b/"},
		{"https://user:secret@example.com/gitlab/api/v4/?x=1#f", "https://example.com/gitlab/"},
		{"https://example.com/", "https://example.com/"},
	}
	for _, c := range cases {
		t.Run(c.base, func(t *testing.T) {
			base := mustParseURL(t, c.base)
			if got := InstanceWebURL(base).String(); got != c.want {
				t.Errorf("InstanceWebURL(%q) = %q, want %q", c.base, got, c.want)
			}
			if base.String() != c.base {
				t.Errorf("InstanceWebURL changed its argument to %q", base.String())
			}
		})
	}
}

// TestPathBelowInstanceWeb_MatchesOnlyTheInstanceWebPrefix verifies the
// path below the web URL for an http or https remote on the instance's
// scheme, host and port, and no answer for an SSH remote, another host,
// scheme or port, a path outside the relative root, or the root itself.
func TestPathBelowInstanceWeb_MatchesOnlyTheInstanceWebPrefix(t *testing.T) {
	const rooted = "https://example.com/gitlab/api/v4/"
	cases := []struct {
		name, base, remote, want string
		ok                       bool
	}{
		{"root with .git", rooted, "https://example.com/gitlab/group/project.git", "group/project.git", true},
		{"root without .git", rooted, "https://example.com/gitlab/group/sub/project", "group/sub/project", true},
		{"host in another case", rooted, "https://EXAMPLE.com/gitlab/group/project", "group/project", true},
		{"default port written out", rooted, "https://example.com:443/gitlab/group/project", "group/project", true},
		{"no root", "https://gitlab.example.com/api/v4/", "https://gitlab.example.com/group/project.git", "group/project.git", true},
		{"http default port", "http://gitlab.example.com/api/v4/", "http://gitlab.example.com:80/group/project", "group/project", true},
		{"credentials in the remote", rooted, "https://user:pw@example.com/gitlab/group/project.git", "group/project.git", true},
		{"outside the root", rooted, "https://example.com/other/group/project.git", "", false},
		{"a sibling of the root", rooted, "https://example.com/gitlabx/group/project.git", "", false},
		{"the root itself", rooted, "https://example.com/gitlab/", "", false},
		{"another host", rooted, "https://github.com/gitlab/group/project.git", "", false},
		{"another scheme", rooted, "http://example.com/gitlab/group/project.git", "", false},
		{"another port", rooted, "https://example.com:8443/gitlab/group/project.git", "", false},
		{"ssh scheme", rooted, "ssh://git@example.com/gitlab/group/project.git", "", false},
		{"scp form", rooted, "git@example.com:gitlab/group/project.git", "", false},
		{"relative", rooted, "../group/project.git", "", false},
		{"unparsable", rooted, "https://example.com/%zz", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := PathBelowInstanceWeb(mustParseURL(t, c.base), c.remote)
			if got != c.want || ok != c.ok {
				t.Errorf("PathBelowInstanceWeb(%q, %q) = (%q, %v), want (%q, %v)", c.base, c.remote, got, ok, c.want, c.ok)
			}
		})
	}
}
