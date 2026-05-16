package tools

import (
	"fmt"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/internal/tools/dynamic"
)

const (
	// expectedBaseDynamicCatalogActions identifies the expected base dynamic catalog actions constant used by this package.
	expectedBaseDynamicCatalogActions = 867
	// expectedEnterpriseDynamicCatalogActions identifies the expected enterprise dynamic catalog actions constant used by this package.
	expectedEnterpriseDynamicCatalogActions = 1010
	// expectedGitLabComEnterpriseCatalogActions identifies the expected GitLab com enterprise catalog actions constant used by this package.
	expectedGitLabComEnterpriseCatalogActions = 1015
)

// TestActionCatalog_BaselineCountsDoNotRegress covers ActionCatalog with table-driven subtests for baseline counts do not regress.
func TestActionCatalog_BaselineCountsDoNotRegress(t *testing.T) {
	testCases := []struct {
		name       string
		client     *gitlabclient.Client
		enterprise bool
		want       int
	}{
		{name: "base", want: expectedBaseDynamicCatalogActions},
		{name: "self-managed enterprise", enterprise: true, want: expectedEnterpriseDynamicCatalogActions},
		{name: "gitlab.com enterprise", client: newGitLabDotComClient(t), enterprise: true, want: expectedGitLabComEnterpriseCatalogActions},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := mustBuildDynamicActionCatalogForTest(t, tc.client, tc.enterprise)
			if got := catalog.CountActions(); got != tc.want {
				t.Fatalf("dynamic catalog action count = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestActionSpecCoverage_AllCatalogRoutesClassified verifies ActionSpecCoverage when all catalog routes classified.
func TestActionSpecCoverage_AllCatalogRoutesClassified(t *testing.T) {
	catalog := mustBuildDynamicActionCatalogForTest(t, newGitLabDotComClient(t), true)
	missing := make([]actioncatalog.ActionID, 0)
	for _, action := range catalog.Actions() {
		if action.SpecBacked {
			continue
		}
		missing = append(missing, action.ID)
	}
	if len(missing) > 0 {
		t.Fatalf("catalog actions must be spec-backed:\n%s", formatMissingActionSpecs(missing))
	}
}

// mustBuildDynamicActionCatalogForTest builds dynamic action catalog for test test fixtures and fails the test on error.
func mustBuildDynamicActionCatalogForTest(t *testing.T, client *gitlabclient.Client, enterprise bool) *actioncatalog.Catalog {
	t.Helper()
	catalog := mustBuildActionCatalog(t, client, ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	catalog, err := dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}
	return catalog
}

// formatMissingActionSpecs renders the result as a formatted string.
func formatMissingActionSpecs(ids []actioncatalog.ActionID) string {
	var builder strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&builder, "\t%s\n", id)
	}
	return builder.String()
}
