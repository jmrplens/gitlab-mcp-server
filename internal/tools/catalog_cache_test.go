// catalog_cache_test.go memoizes nil-client action-catalog builds for the
// package's tests. Building the full catalog resolves and validates ~850
// action schemas (~2s); the result depends only on the projection options
// when no GitLab client, updater, or spec-group overrides are involved, so
// tests that merely inspect the projection share one build per option set.
// Callers receive a Clone so no test can leak mutations into another.
package tools

import (
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// sharedCatalogKey identifies one cacheable nil-client catalog projection.
type sharedCatalogKey struct {
	tier       edition.Tier
	enterprise bool
	includeMCP bool
}

var (
	sharedCatalogsMu sync.Mutex
	sharedCatalogs   = map[sharedCatalogKey]*actioncatalog.Catalog{}
)

// cacheableCatalogOptions reports whether opts only carries the fields the
// shared cache keys on: a client or spec-group override makes the
// build unique to its caller.
func cacheableCatalogOptions(opts ActionCatalogOptions) bool {
	return opts.SpecGroups == nil
}

// sharedActionCatalog returns a clone of the memoized nil-client catalog for
// opts, building it on first use.
func sharedActionCatalog(opts ActionCatalogOptions) (*actioncatalog.Catalog, error) {
	key := sharedCatalogKey{tier: opts.Tier, enterprise: opts.Enterprise, includeMCP: opts.IncludeMCP}
	sharedCatalogsMu.Lock()
	defer sharedCatalogsMu.Unlock()
	if cached, ok := sharedCatalogs[key]; ok {
		return cached.Clone(), nil
	}
	catalog, err := BuildActionCatalog(nil, opts)
	if err != nil {
		return nil, err
	}
	sharedCatalogs[key] = catalog
	return catalog.Clone(), nil
}
