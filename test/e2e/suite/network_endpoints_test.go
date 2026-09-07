//go:build e2e

// network_endpoints_test.go centralizes deterministic URLs used by Docker E2E
// tests for webhook, custom emoji, and remote mirror operations. Shared by
// the CE and EE builds: the EE group-webhook coverage drives deliveries
// through the same fixture service.
package suite

import (
	"os"
	"strings"
)

// Default Docker network endpoints used when setup-gitlab.sh has not written
// explicit E2E fixture settings into the environment.
const (
	defaultE2EFixtureURL        = "http://e2e-fixture:8080"
	defaultE2EGitLabInternalURL = "http://gitlab-e2e"
)

// hasE2EFixtureService reports whether the Docker-local fixture service should
// be reachable from GitLab during this E2E run.
func hasE2EFixtureService() bool {
	if os.Getenv("E2E_FIXTURE_URL") != "" {
		return true
	}
	return strings.EqualFold(os.Getenv("E2E_MODE"), "docker")
}

// e2eFixtureServiceURL returns a URL on the Docker-local fixture service for
// resourcePath, falling back to the default service name when unset.
func e2eFixtureServiceURL(resourcePath string) string {
	baseURL := os.Getenv("E2E_FIXTURE_URL")
	if baseURL == "" {
		baseURL = defaultE2EFixtureURL
	}
	return joinE2EURL(baseURL, resourcePath)
}

// remoteMirrorTargetURL lives in network_endpoints_ce_test.go: only the CE
// mirror suites call it, and a helper both halves compile but one half uses
// is unused under the other's tag.

// joinE2EURL joins baseURL and resourcePath without preserving duplicate slash
// boundaries from either side.
func joinE2EURL(baseURL, resourcePath string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(resourcePath, "/")
}
