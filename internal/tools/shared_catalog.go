// shared_catalog.go builds each catalog once per configuration and hands every
// server a copy bound to its own client.
//
// The HTTP pool builds one MCP server per credential, and until this file
// existed each of them built its own catalog: the same thousand actions, the
// same schemas, the same search documents, once per token. Measured at a
// hundred pooled credentials, that was 130 MiB per credential on the dynamic
// surface, of which every byte but the handler closures was the same as the
// next credential's. Nothing in a catalog depends on the credential except the
// handlers, and those are rebuilt for each client in a fraction of the time
// the catalog takes.
//
// The key is everything that shapes the surface other than the credential:
// the tier, the instance class (GitLab.com carries the Orbit actions), whether
// the maintenance group is included, and, for the filtered surfaces, the
// operator's exclusions, the token's scopes, read-only mode with its cause,
// and safe mode. The key space is bounded by configuration: every one of those
// is either a deployment setting or one of a handful of values GitLab can
// answer, so the cache holds a few catalogs for the life of the process and is
// never evicted.

package tools

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// sharedCatalogEntry is one cached catalog, built by the first caller for
// its key while every concurrent caller for the same key waits.
type sharedCatalogEntry struct {
	once     sync.Once
	catalog  *actioncatalog.Catalog
	withheld WithheldActions
	err      error
}

// sharedCatalogs holds the catalogs built so far, by key.
var sharedCatalogs sync.Map // string -> *sharedCatalogEntry

// Test seams. Neither call can fail from any input the server takes: the
// action specs are compiled in and validated by their own tests, and the
// filter adds groups the base catalog already validated. The branches that
// report their failure exist for the day one of those facts changes, and
// would otherwise never run.
var (
	sharedBaseCatalog   = SharedBaseCatalog   //nolint:gochecknoglobals // test seam
	filterSharedCatalog = FilterActionCatalog //nolint:gochecknoglobals // test seam
)

// ShareCatalog returns the catalog cached under key, building it with build
// on the first call and marking the result shared (see
// [actioncatalog.Catalog.MarkShared]). Concurrent callers for one key wait
// for the one build; callers for different keys build in parallel. A failed
// build is not cached, so the next caller tries again and reports the same
// error rather than a stale one.
//
// The returned catalog is the shared, unbound copy: its handlers refuse every
// call. Bind it with [actioncatalog.Catalog.BindTo] before registering
// anything from it.
func ShareCatalog(key string, build func() (*actioncatalog.Catalog, WithheldActions, error)) (*actioncatalog.Catalog, WithheldActions, error) {
	loaded, _ := sharedCatalogs.LoadOrStore(key, &sharedCatalogEntry{})
	entry, _ := loaded.(*sharedCatalogEntry)
	entry.once.Do(func() {
		entry.catalog, entry.withheld, entry.err = build()
		if entry.err != nil {
			sharedCatalogs.CompareAndDelete(key, loaded)
			return
		}
		entry.catalog.MarkShared()
	})
	return entry.catalog, entry.withheld, entry.err
}

// unboundClients are the two clients the shared catalogs are built with: one
// per instance class, since that is the only thing an ActionSpecs function
// reads off the client while building. Neither carries a credential.
var (
	unboundClientsOnce sync.Once
	unboundDotCom      *gitlabclient.Client
	unboundSelfManaged *gitlabclient.Client
)

// UnboundClient returns the credential-less client a shared catalog is built
// with, for the instance class dotcom selects. See
// [gitlabclient.NewUnboundClient] for why the shared copy must not be built
// with a real one.
func UnboundClient(dotcom bool) *gitlabclient.Client {
	unboundClientsOnce.Do(func() {
		unboundDotCom = gitlabclient.NewUnboundClient("https://" + gitlabclient.GitLabDotComHost)
		unboundSelfManaged = gitlabclient.NewUnboundClient("https://gitlab.invalid")
	})
	if dotcom {
		return unboundDotCom
	}
	return unboundSelfManaged
}

// BaseCatalogKey names the unfiltered catalog a configuration starts from:
// the tier, the instance class and whether the maintenance group is in.
func BaseCatalogKey(tier edition.Tier, dotcom, includeMCP bool) string {
	return "tier=" + tier.String() + "|dotcom=" + strconv.FormatBool(dotcom) + "|mcp=" + strconv.FormatBool(includeMCP)
}

// CatalogFilterKey names the narrowing [FilterActionCatalog] applies from a
// server configuration, so two servers narrowed the same way share one
// filtered catalog. The scopes are sorted because GitLab lists a token's
// scopes in no order this cares about, and a nil list is told apart from an
// empty one because the scope filter treats them differently.
func CatalogFilterKey(cfg *config.ServerConfig) string {
	scopes := slices.Clone(cfg.TokenScopes)
	sort.Strings(scopes)
	return fmt.Sprintf("exclude=%s|scopes=%s|scopesKnown=%t|readonly=%t|readonlyFromScope=%t|safe=%t",
		strings.Join(cfg.ExcludeTools, ","), strings.Join(scopes, ","), cfg.TokenScopes != nil,
		cfg.ReadOnly, cfg.ReadOnlyFromTokenScope, cfg.SafeMode)
}

// SharedBaseCatalog returns the shared, unbound catalog for a tier and
// instance class, building it on first use with [UnboundClient]. Callers
// that want a catalog to register from want [BuildActionCatalog], which binds
// this one; this is for the assemblers that filter it before binding.
func SharedBaseCatalog(dotcom bool, opts ActionCatalogOptions) (*actioncatalog.Catalog, error) {
	tier := opts.effectiveTier()
	key := "base|" + BaseCatalogKey(tier, dotcom, opts.IncludeMCP)
	catalog, _, err := ShareCatalog(key, func() (*actioncatalog.Catalog, WithheldActions, error) {
		built, buildErr := buildActionCatalog(UnboundClient(dotcom), ActionCatalogOptions{Tier: tier, IncludeMCP: opts.IncludeMCP})
		return built, WithheldActions{}, buildErr
	})
	return catalog, err
}

// SharedMetaCatalog returns the catalog the meta surface registers for cfg,
// narrowed by [FilterActionCatalog] and bound to client, together with what
// the narrowing withheld. One filtered catalog is built per distinct
// narrowing and shared by every server that needs it.
func SharedMetaCatalog(client *gitlabclient.Client, cfg *config.ServerConfig) (*actioncatalog.Catalog, WithheldActions, error) {
	dotcom := client.IsGitLabDotCom()
	key := "meta|" + BaseCatalogKey(cfg.Tier, dotcom, true) + "|" + CatalogFilterKey(cfg)
	catalog, withheld, err := ShareCatalog(key, func() (*actioncatalog.Catalog, WithheldActions, error) {
		base, baseErr := sharedBaseCatalog(dotcom, ActionCatalogOptions{Tier: cfg.Tier, IncludeMCP: true})
		if baseErr != nil {
			return nil, WithheldActions{}, fmt.Errorf("build meta action catalog: %w", baseErr)
		}
		filtered, filteredWithheld, filterErr := filterSharedCatalog(base, cfg)
		if filterErr != nil {
			return nil, WithheldActions{}, fmt.Errorf("filter meta action catalog: %w", filterErr)
		}
		return filtered, filteredWithheld, nil
	})
	if err != nil {
		return nil, WithheldActions{}, err
	}
	return catalog.BindTo(client), withheld, nil
}

// SharedIndividualCatalog returns the catalog the individual surface registers
// for cfg, with the operator's exclusions applied and bound to client, and the
// canonical IDs those exclusions removed. The exclusions are applied to the
// catalog rather than to registered names, for the reason
// [registerConfiguredToolSurface] gives: only the catalog can map all three
// spellings an operator writes.
func SharedIndividualCatalog(client *gitlabclient.Client, cfg *config.ServerConfig) (*actioncatalog.Catalog, []string, error) {
	dotcom := client.IsGitLabDotCom()
	key := "individual|" + BaseCatalogKey(cfg.Tier, dotcom, true) + "|exclude=" + strings.Join(cfg.ExcludeTools, ",")
	catalog, withheld, err := ShareCatalog(key, func() (*actioncatalog.Catalog, WithheldActions, error) {
		base, baseErr := sharedBaseCatalog(dotcom, ActionCatalogOptions{Tier: cfg.Tier, IncludeMCP: true})
		if baseErr != nil {
			return nil, WithheldActions{}, fmt.Errorf("build individual action catalog: %w", baseErr)
		}
		// Resolved before the exclusion is applied, since afterwards the
		// catalog can no longer map the operator's entries to anything.
		excluded := base.ExcludedActionIDs(cfg.ExcludeTools)
		return ExcludeFromCatalog(base, cfg.ExcludeTools), WithheldActions{ExcludedByName: excluded}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return catalog.BindTo(client), withheld.ExcludedByName, nil
}
