package toolutil

import (
	"net/url"
	"strings"
)

// InstanceWebURL returns the web URL of the instance whose REST API is at
// apiBase: the API base with its trailing "api/v4/" removed, so an instance
// served under a relative URL root keeps the root
// (https://example.com/gitlab/api/v4/ gives https://example.com/gitlab/).
// The path always ends in "/". The userinfo, query and fragment an operator
// may have configured are left out, since the result is compared with URLs
// and never sent anywhere. apiBase itself is not changed.
func InstanceWebURL(apiBase *url.URL) *url.URL {
	web := url.URL{Scheme: apiBase.Scheme, Host: apiBase.Host}
	root := strings.TrimSuffix(apiBase.Path, "/")
	root = strings.TrimSuffix(root, "/api/v4")
	web.Path = root + "/"
	return &web
}

// PathBelowInstanceWeb reports the path of remote below the instance's web
// URL, without the leading slash, when remote is an http or https URL on
// the instance's scheme, host and port whose path lies under the instance's
// relative URL root; ok is false for anything else, the root itself
// included.
//
// It is the rule GitLab's own SubmoduleHelper applies with
// Gitlab.config.gitlab.url: a prefix match on the web URL, relative root
// included. It stops at http and https because an SSH clone URL never
// carries the relative root (GitLab builds it from
// gitlab_shell.ssh_path_prefix), so stripping the root from one would
// mangle a top-level group that happens to share the root's name. The host
// is compared without regard to case and a port left out is the scheme's
// default, which is how a browser or git reads the two.
func PathBelowInstanceWeb(apiBase *url.URL, remote string) (string, bool) {
	u, err := url.Parse(remote)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	web := InstanceWebURL(apiBase)
	if u.Scheme != web.Scheme || !strings.EqualFold(u.Hostname(), web.Hostname()) || portOrDefault(u) != portOrDefault(web) {
		return "", false
	}
	rest, found := strings.CutPrefix(u.Path, web.Path)
	if !found || rest == "" {
		return "", false
	}
	return rest, true
}

// portOrDefault returns the port of an http or https URL, the scheme's
// default when the URL names none.
func portOrDefault(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
