//go:build e2e

// network_endpoints_ce_test.go holds the Docker-network URL helper only the
// Community Edition mirror suites read. It was split from
// network_endpoints_test.go when the suite was still two halves behind two
// build tags, so that it lived behind the constraint of its callers: an
// Enterprise compile dropped every *_ce_test.go and left the helper unused,
// which nothing noticed while nothing compiled that half outside a manual run
// (issue 570). Every file carries `e2e` alone now, and the file keeps its name
// until the suite goes.
//
// Build tag: e2e.
package suite

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

// remoteMirrorTargetURL returns an authenticated Git URL for target that GitLab
// can reach from inside the Docker network when configuring push mirrors.
func remoteMirrorTargetURL(t *testing.T, target ProjectFixture) string {
	t.Helper()

	token := os.Getenv("GITLAB_TOKEN")
	requireTruef(t, token != "", "GITLAB_TOKEN is required to configure a remote mirror target")

	baseURL := os.Getenv("E2E_GITLAB_INTERNAL_URL")
	if baseURL == "" {
		baseURL = defaultE2EGitLabInternalURL
	}

	parsedURL, err := url.Parse(baseURL)
	requireNoError(t, err, "parse internal GitLab URL")
	parsedURL.User = url.UserPassword("oauth2", token)
	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + "/" + strings.TrimLeft(target.Path, "/") + ".git"
	parsedURL.RawQuery = ""
	parsedURL.Fragment = ""

	return parsedURL.String()
}
