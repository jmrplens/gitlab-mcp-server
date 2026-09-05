package tools

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// healthyGitLab answers every request the way a reachable instance would,
// enough for the health action to report the URL it reached.
func healthyGitLab() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0","revision":"abc123","id":1,"username":"tester"}`)
	})
}

// gitlabURLReached runs the health action of a bound catalog and returns the
// GitLab URL the action reported, which is the URL of the client it ran
// under.
func gitlabURLReached(t *testing.T, catalog *actioncatalog.Catalog) string {
	t.Helper()
	action, ok := catalog.Action("server.status")
	if !ok {
		t.Fatal("server.status is missing from the catalog")
	}
	result, err := action.Route.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("server.status error = %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encoding the health output: %v", err)
	}
	var output struct {
		GitLabURL string `json:"gitlab_url"`
	}
	if decodeErr := json.Unmarshal(encoded, &output); decodeErr != nil {
		t.Fatalf("decoding the health output: %v", decodeErr)
	}
	return output.GitLabURL
}

// schemaDigests hashes every route schema in a catalog, so a test can tell
// whether registering and listing a server changed any of them.
func schemaDigests(t *testing.T, catalog *actioncatalog.Catalog) map[actioncatalog.ActionID][2][32]byte {
	t.Helper()
	digests := make(map[actioncatalog.ActionID][2][32]byte)
	for _, action := range catalog.Actions() {
		input, err := json.Marshal(action.Route.InputSchema)
		if err != nil {
			t.Fatalf("encoding %s input schema: %v", action.ID, err)
		}
		output, err := json.Marshal(action.Route.OutputSchema)
		if err != nil {
			t.Fatalf("encoding %s output schema: %v", action.ID, err)
		}
		digests[action.ID] = [2][32]byte{sha256.Sum256(input), sha256.Sum256(output)}
	}
	return digests
}

// newListingServer builds a server with the two tools/list middlewares the
// real server installs, which is where the per-server schema rewrites used
// to happen.
func newListingServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "shared-catalog", Version: "0"}, &mcp.ServerOptions{PageSize: 2000})
	toolutil.LockdownInputSchemas(server)
	toolutil.EnrichPaginationConstraints(server)
	return server
}

// listedTools returns the JSON of a server's tools/list, the bytes a client
// would receive apart from the JSON-RPC envelope.
func listedTools(t *testing.T, server *mcp.Server) []byte {
	t.Helper()
	tools, err := toolutil.ListRegisteredTools(context.Background(), server, "shared-catalog")
	if err != nil {
		t.Fatalf("ListRegisteredTools() error = %v", err)
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("encoding tools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("the server lists no tools")
	}
	return encoded
}

// TestShareCatalog_BuildsOncePerKeyAndRetriesAfterFailure verifies the cache
// every shared catalog goes through: concurrent callers for one key wait for
// one build and receive one catalog, marked shared; a build that fails is
// reported and not cached, so the next caller builds again.
func TestShareCatalog_BuildsOncePerKeyAndRetriesAfterFailure(t *testing.T) {
	t.Parallel()

	var builds atomic.Int32
	build := func() (*actioncatalog.Catalog, WithheldActions, error) {
		builds.Add(1)
		time.Sleep(20 * time.Millisecond)
		catalog := actioncatalog.NewCatalog()
		group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_test"})
		group.SetAction(actioncatalog.Action{Name: "get", Route: toolutil.Route(func(context.Context, map[string]any) (any, error) { return map[string]any{}, nil })})
		if err := catalog.AddGroup(group); err != nil {
			return nil, WithheldActions{}, err
		}
		return catalog, WithheldActions{ByOperator: []string{"test.delete"}}, nil
	}
	key := "test|" + t.Name()
	const callers = 12
	catalogs := make([]*actioncatalog.Catalog, callers)
	withhelds := make([]WithheldActions, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			catalogs[i], withhelds[i], errs[i] = ShareCatalog(key, build)
		})
	}
	wg.Wait()
	if builds.Load() != 1 {
		t.Fatalf("build ran %d times for one key, want 1", builds.Load())
	}
	for i := range callers {
		if errs[i] != nil || catalogs[i] != catalogs[0] || len(withhelds[i].ByOperator) != 1 {
			t.Fatalf("caller %d got %p, %+v, %v; want caller 0's catalog %p", i, catalogs[i], withhelds[i], errs[i], catalogs[0])
		}
	}
	if catalogs[0].SharedOrigin() != catalogs[0] {
		t.Fatal("the cached catalog was not marked shared")
	}

	forced := errors.New("forced build failure")
	var attempts atomic.Int32
	flaky := func() (*actioncatalog.Catalog, WithheldActions, error) {
		if attempts.Add(1) == 1 {
			return nil, WithheldActions{}, forced
		}
		return build()
	}
	failingKey := key + "|flaky"
	if _, _, err := ShareCatalog(failingKey, flaky); !errors.Is(err, forced) {
		t.Fatalf("ShareCatalog(first) error = %v, want the forced failure", err)
	}
	catalog, _, err := ShareCatalog(failingKey, flaky)
	if err != nil || catalog == nil || catalog.SharedOrigin() != catalog {
		t.Fatalf("ShareCatalog(second) = %p, %v; want a fresh shared build after the failure", catalog, err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("the flaky build ran %d times, want 2: the failure must not be cached", attempts.Load())
	}
}

// TestUnboundClient_AnswersTheInstanceClassAndRefusesRequests verifies the
// two clients shared catalogs are built with: one per instance class, each
// answering the class its URL names and refusing every request.
func TestUnboundClient_AnswersTheInstanceClassAndRefusesRequests(t *testing.T) {
	t.Parallel()

	dotcom, selfManaged := UnboundClient(true), UnboundClient(false)
	if !dotcom.IsGitLabDotCom() || selfManaged.IsGitLabDotCom() {
		t.Fatalf("instance classes = dotcom %t, self-managed %t", dotcom.IsGitLabDotCom(), selfManaged.IsGitLabDotCom())
	}
	if UnboundClient(true) != dotcom || UnboundClient(false) != selfManaged {
		t.Fatal("UnboundClient() built a second client, want one per class for the process")
	}
	if _, err := dotcom.Ping(context.Background()); !errors.Is(err, gitlabclient.ErrUnboundClient) {
		t.Fatalf("Ping() through the unbound client = %v, want ErrUnboundClient", err)
	}
}

// TestCatalogKeys_NameEverythingThatShapesTheCatalog verifies the two key
// builders: the base key carries tier, instance class and the maintenance
// group; the filter key carries the exclusions, the scopes that can change the
// catalog (sorted and deduplicated), whether the scopes were detected at all,
// and the three mode switches.
func TestCatalogKeys_NameEverythingThatShapesTheCatalog(t *testing.T) {
	t.Parallel()

	if got := BaseCatalogKey(edition.Premium, true, false); got != "tier=premium|dotcom=true|mcp=false" {
		t.Errorf("BaseCatalogKey() = %q", got)
	}
	cases := []struct {
		name string
		cfg  config.ServerConfig
		want string
	}{
		{
			name: "defaults",
			want: "exclude=|scopes=|scopesKnown=false|readonly=false|readonlyFromScope=false|safe=false",
		},
		{
			name: "detected empty scopes are told apart from unknown",
			cfg:  config.ServerConfig{TokenScopes: []string{}},
			want: "exclude=|scopes=|scopesKnown=true|readonly=false|readonlyFromScope=false|safe=false",
		},
		{
			name: "scopes the filter cannot act on leave no trace",
			cfg:  config.ServerConfig{TokenScopes: []string{"read_api", "api", "k8s_proxy", "ai_features"}},
			want: "exclude=|scopes=|scopesKnown=true|readonly=false|readonlyFromScope=false|safe=false",
		},
		{
			name: "the scopes that shape the catalog are kept, sorted and deduplicated, and the switches named",
			cfg: config.ServerConfig{
				ExcludeTools:           []string{"gitlab_admin", "issue.delete"},
				TokenScopes:            []string{"sudo", "api", "admin_mode", "read_api", "admin_mode"},
				ReadOnly:               true,
				ReadOnlyFromTokenScope: true,
				SafeMode:               true,
			},
			want: "exclude=gitlab_admin,issue.delete|scopes=admin_mode|scopesKnown=true|readonly=true|readonlyFromScope=true|safe=true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CatalogFilterKey(&tc.cfg); got != tc.want {
				t.Errorf("CatalogFilterKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCatalogFilterKey_ScopeComponentIsBoundedByTheFilter is the bound on a
// cache that never evicts. The scopes of a token are chosen by whoever minted
// it, and a user can mint personal access tokens with arbitrary scope subsets,
// so the key may carry only the scopes the filter can act on: every scope
// [MetaToolScopes] requires reaches it, and no other scope does.
//
// Driven from MetaToolScopes rather than from a written-out list, so an entry
// added there is covered here without an edit.
func TestCatalogFilterKey_ScopeComponentIsBoundedByTheFilter(t *testing.T) {
	t.Parallel()

	required := map[string]struct{}{}
	for _, scopes := range MetaToolScopes {
		for _, scope := range scopes {
			required[scope] = struct{}{}
		}
	}
	if len(required) == 0 {
		t.Fatal("MetaToolScopes requires no scope at all, so this test proves nothing")
	}
	for scope := range required {
		t.Run("kept: "+scope, func(t *testing.T) {
			t.Parallel()
			cfg := config.ServerConfig{TokenScopes: []string{scope}}
			if got := CatalogFilterKey(&cfg); !strings.Contains(got, "|scopes="+scope+"|") {
				t.Errorf("CatalogFilterKey() = %q, want the required scope %q in it", got, scope)
			}
		})
	}
	// The scopes GitLab can issue that MetaToolScopes never asks about. Each
	// must leave the key exactly as an empty scope list leaves it.
	for _, scope := range []string{"api", "read_api", "read_user", "read_repository", "write_repository", "read_registry", "write_registry", "create_runner", "manage_runner", "ai_features", "k8s_proxy", "sudo"} {
		if _, isRequired := required[scope]; isRequired {
			continue
		}
		t.Run("dropped: "+scope, func(t *testing.T) {
			t.Parallel()
			carried := config.ServerConfig{TokenScopes: []string{scope}}
			empty := config.ServerConfig{TokenScopes: []string{}}
			if got, want := CatalogFilterKey(&carried), CatalogFilterKey(&empty); got != want {
				t.Errorf("CatalogFilterKey() with %q = %q, want the key of a token carrying no scope the filter reads, %q", scope, got, want)
			}
		})
	}
}

// TestSharedMetaCatalog_ScopeSubsetsDoNotEachPinACatalog is the same bound
// through the cache itself: two credentials whose scope lists differ only in
// scopes the filter never reads get one catalog, while a credential missing an
// admin_mode-gated scope gets its own.
func TestSharedMetaCatalog_ScopeSubsetsDoNotEachPinACatalog(t *testing.T) {
	client := testutil.NewTestClient(t, healthyGitLab())
	admin := &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"scopes-" + t.Name()}, TokenScopes: []string{"api", "admin_mode"}}
	adminPlusNoise := &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"scopes-" + t.Name()}, TokenScopes: []string{"read_api", "k8s_proxy", "admin_mode", "ai_features"}}
	plain := &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"scopes-" + t.Name()}, TokenScopes: []string{"api"}}

	first, _, err := SharedMetaCatalog(client, admin)
	if err != nil {
		t.Fatalf("SharedMetaCatalog(admin) error = %v", err)
	}
	second, _, err := SharedMetaCatalog(client, adminPlusNoise)
	if err != nil {
		t.Fatalf("SharedMetaCatalog(admin plus unread scopes) error = %v", err)
	}
	if first.SharedOrigin() != second.SharedOrigin() {
		t.Error("two scope lists differing only in scopes the filter never reads pinned two catalogs")
	}
	third, _, err := SharedMetaCatalog(client, plain)
	if err != nil {
		t.Fatalf("SharedMetaCatalog(no admin_mode) error = %v", err)
	}
	if third.SharedOrigin() == first.SharedOrigin() {
		t.Error("a credential without admin_mode shared the catalog of one that has it")
	}
	if _, kept := first.Action("admin.settings_get"); !kept {
		t.Fatal("admin.settings_get is absent from the admin_mode catalog, so the removal below proves nothing")
	}
	if _, kept := third.Action("admin.settings_get"); kept {
		t.Error("the admin group survived a token with no admin_mode")
	}
}

// TestSharedMetaCatalog_TwoClientsShareOneCatalogBoundToEach verifies the
// contract every pool entry relies on: two clients with one configuration
// receive catalogs with one shared origin and one set of schema maps, and
// each runs its actions under its own client.
func TestSharedMetaCatalog_TwoClientsShareOneCatalogBoundToEach(t *testing.T) {
	clientA := testutil.NewTestClient(t, healthyGitLab())
	clientB := testutil.NewTestClient(t, healthyGitLab())
	cfg := &config.ServerConfig{Tier: edition.Free, ReadOnly: true}

	catalogA, withheldA, err := SharedMetaCatalog(clientA, cfg)
	if err != nil {
		t.Fatalf("SharedMetaCatalog(A) error = %v", err)
	}
	catalogB, withheldB, err := SharedMetaCatalog(clientB, cfg)
	if err != nil {
		t.Fatalf("SharedMetaCatalog(B) error = %v", err)
	}
	if catalogA == catalogB || catalogA.SharedOrigin() == nil || catalogA.SharedOrigin() != catalogB.SharedOrigin() {
		t.Fatal("the two clients did not get distinct catalogs bound from one shared origin")
	}
	if !reflect.DeepEqual(withheldA, withheldB) || len(withheldA.ByOperator) == 0 {
		t.Fatalf("withheld = %+v and %+v, want the same non-empty read-only narrowing", withheldA, withheldB)
	}
	for _, action := range catalogA.Actions() {
		other, ok := catalogB.Action(action.ID)
		if !ok {
			t.Fatalf("%s is in catalog A but not in catalog B", action.ID)
		}
		if reflect.ValueOf(action.Route.InputSchema).UnsafePointer() != reflect.ValueOf(other.Route.InputSchema).UnsafePointer() {
			t.Fatalf("%s carries two input schema maps, want one shared", action.ID)
		}
		if !toolutil.SchemaShared(action.Route.InputSchema) {
			t.Fatalf("%s input schema is not registered as shared", action.ID)
		}
	}
	if urlA, urlB := gitlabURLReached(t, catalogA), gitlabURLReached(t, catalogB); urlA == urlB || !strings.HasPrefix(urlA, "http://127.0.0.1") {
		t.Fatalf("server.status reached %q and %q, want each client's own instance", urlA, urlB)
	}
}

// TestSharedIndividualCatalog_TwoServersListOneCatalogUnchanged is the wire
// proof for the individual surface: two servers registered from one shared
// catalog list byte-identical tools, and neither registration nor listing
// changed any schema map the catalog carries.
func TestSharedIndividualCatalog_TwoServersListOneCatalogUnchanged(t *testing.T) {
	clientA := testutil.NewTestClient(t, healthyGitLab())
	clientB := testutil.NewTestClient(t, healthyGitLab())
	cfg := &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"gitlab_issue_delete"}}

	catalogA, excludedA, err := SharedIndividualCatalog(clientA, cfg)
	if err != nil {
		t.Fatalf("SharedIndividualCatalog(A) error = %v", err)
	}
	catalogB, excludedB, err := SharedIndividualCatalog(clientB, cfg)
	if err != nil {
		t.Fatalf("SharedIndividualCatalog(B) error = %v", err)
	}
	if catalogA.SharedOrigin() == nil || catalogA.SharedOrigin() != catalogB.SharedOrigin() {
		t.Fatal("the two clients did not get catalogs bound from one shared origin")
	}
	if !reflect.DeepEqual(excludedA, excludedB) || !contains(excludedA, "issue.delete") {
		t.Fatalf("excluded = %v and %v, want issue.delete in both", excludedA, excludedB)
	}
	if _, kept := catalogA.Action("issue.delete"); kept {
		t.Fatal("issue.delete survived the exclusion")
	}
	before := schemaDigests(t, catalogA)

	// A cache key of its own for each server, and neither is the key the
	// server uses in production. compiledSchema returns the compiled schema
	// cached under a key without ever reading the map it was handed, so two
	// registrations under one key would compare the first server's compiled
	// schema against itself: the comparison below would hold even if the
	// second caller had been handed stale or wrongly pruned maps. Distinct
	// keys make each server compile its own catalog's maps, which is what the
	// listing is supposed to be comparing.
	serverA, serverB := newListingServer(), newListingServer()
	RegisterIndividualCatalogTools(serverA, catalogA, individualOptionsWithSchemaKey(t, "A"))
	RegisterIndividualCatalogTools(serverB, catalogB, individualOptionsWithSchemaKey(t, "B"))
	listA, listB := listedTools(t, serverA), listedTools(t, serverB)
	if !bytes.Equal(listA, listB) {
		t.Fatal("the two servers list different tools from one shared catalog")
	}
	if !reflect.DeepEqual(schemaDigests(t, catalogA), before) || !reflect.DeepEqual(schemaDigests(t, catalogB), before) {
		t.Fatal("registering and listing changed a schema map the shared catalog carries")
	}
	if urlA, urlB := gitlabURLReached(t, catalogA), gitlabURLReached(t, catalogB); urlA == urlB {
		t.Fatalf("both catalogs reached %q, want each client's own instance", urlA)
	}
}

// TestSharedMetaCatalog_ConcurrentEntriesBuildSafely verifies what the pool
// does under a startup burst: several entries for one fresh configuration
// build and register at once, from one origin, and list the same tools.
// Run under the race detector, this is the proof that the caches and the
// shared maps are read concurrently and written once.
func TestSharedMetaCatalog_ConcurrentEntriesBuildSafely(t *testing.T) {
	const entries = 6
	clients := make([]*gitlabclient.Client, entries)
	for i := range clients {
		clients[i] = testutil.NewTestClient(t, healthyGitLab())
	}
	cfg := &config.ServerConfig{Tier: edition.Premium, ExcludeTools: []string{"race-" + t.Name()}}

	origins := make([]*actioncatalog.Catalog, entries)
	lists := make([][]byte, entries)
	failures := make([]error, entries)
	var wg sync.WaitGroup
	for i := range entries {
		wg.Go(func() {
			catalog, _, err := SharedMetaCatalog(clients[i], cfg)
			if err != nil {
				failures[i] = err
				return
			}
			origins[i] = catalog.SharedOrigin()
			server := newListingServer()
			RegisterMetaCatalog(server, catalog)
			RegisterMetaStandaloneTools(server, clients[i])
			tools, err := toolutil.ListRegisteredTools(context.Background(), server, "race")
			if err != nil {
				failures[i] = err
				return
			}
			lists[i], failures[i] = json.Marshal(tools)
		})
	}
	wg.Wait()
	for i := range entries {
		if failures[i] != nil {
			t.Fatalf("entry %d failed: %v", i, failures[i])
		}
		if origins[i] == nil || origins[i] != origins[0] {
			t.Fatalf("entry %d has origin %p, want entry 0's %p", i, origins[i], origins[0])
		}
		if !bytes.Equal(lists[i], lists[0]) {
			t.Fatalf("entry %d lists different tools from entry 0", i)
		}
	}
}

// TestSharedCatalogs_BuildFailuresAreReportedWithTheirStep verifies each
// assembler names the step that failed, through the seams that stand in for
// failures no real input can cause, under keys no other test has built.
func TestSharedCatalogs_BuildFailuresAreReportedWithTheirStep(t *testing.T) {
	forced := errors.New("forced catalog failure")
	client := testutil.NewTestClient(t, healthyGitLab())
	fresh := func(suffix string) *config.ServerConfig {
		return &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"fail-" + t.Name() + "-" + suffix}}
	}
	failBase := func(t *testing.T) {
		t.Helper()
		original := sharedBaseCatalog
		t.Cleanup(func() { sharedBaseCatalog = original })
		sharedBaseCatalog = func(bool, ActionCatalogOptions) (*actioncatalog.Catalog, error) { return nil, forced }
	}
	failFilter := func(t *testing.T) {
		t.Helper()
		original := filterSharedCatalog
		t.Cleanup(func() { filterSharedCatalog = original })
		filterSharedCatalog = func(*actioncatalog.Catalog, *config.ServerConfig) (*actioncatalog.Catalog, WithheldActions, error) {
			return nil, WithheldActions{}, forced
		}
	}
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		run     func() error
		want    string
	}{
		{
			name:    "meta base",
			arrange: failBase,
			run:     func() error { _, _, err := SharedMetaCatalog(client, fresh("meta-base")); return err },
			want:    "build meta action catalog",
		},
		{
			name:    "meta filter",
			arrange: failFilter,
			run:     func() error { _, _, err := SharedMetaCatalog(client, fresh("meta-filter")); return err },
			want:    "filter meta action catalog",
		},
		{
			name:    "individual base",
			arrange: failBase,
			run:     func() error { _, _, err := SharedIndividualCatalog(client, fresh("individual-base")); return err },
			want:    "build individual action catalog",
		},
		{
			name:    "bound base catalog",
			arrange: failBase,
			run: func() error {
				_, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: edition.Ultimate})
				return err
			},
			want: "forced catalog failure",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.arrange(t)
			err := tc.run()
			if !errors.Is(err, forced) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want the forced failure carrying %q", err, tc.want)
			}
		})
	}
}

// TestDomainPackages_LeaveSharedRouteStateAlone is the source guard over
// everything a route carries that is now shared by every server in the
// process. No package under internal/tools may:
//
//   - assign Route.Handler directly, because a handler replaced that way is
//     dropped the moment a shared catalog rebinds the route (the helpers that
//     change a handler and its binder together live in toolutil), or
//   - write into a route's ParameterGuidance map or either of its schema maps,
//     because the catalog those belong to is built once per configuration and
//     handed to every credential's server: one write reaches all of them, and
//     no test of the writing package can see it.
//
// A whole-field assignment is not an offence and is not matched: a route is a
// value, so replacing a field replaces it on a copy. Only an indexed write and
// a delete reach through to the map itself.
//
// It is a text guard, so it reads the names a route is usually held under
// rather than resolving types. That is enough for what it is for: keeping the
// habit visible at review time on the roughly 770 files where the mistake
// would be silent.
func TestDomainPackages_LeaveSharedRouteStateAlone(t *testing.T) {
	t.Parallel()

	forbidden := []struct {
		what     string
		pattern  *regexp.Regexp
		remedy   string
		offences []string
		allowed  []string
	}{
		{
			what:     "Route.Handler is assigned directly",
			pattern:  regexp.MustCompile(`\.Handler\s*=[^=]`),
			remedy:   "use ActionRoute.WrapHandler or ActionRoute.WithBoundHandler so the binder changes with it",
			offences: []string{"route.Handler = wrapped", "action.Route.Handler=wrapped"},
			allowed:  []string{"if route.Handler == nil {", "Handler: wrapped,"},
		},
		{
			what:     "a route's ParameterGuidance is written into",
			pattern:  regexp.MustCompile(`(?i)\broute\.ParameterGuidance\s*\[[^\]]*\]\s*=[^=]|(?i)\bdelete\(\s*[\w.]*route\.ParameterGuidance\b`),
			remedy:   "build the guidance the spec is given instead; the route's map is shared with every server in the process",
			offences: []string{`route.ParameterGuidance["project_id"] = guidance`, `action.Route.ParameterGuidance["ref"]=g`, "delete(spec.Route.ParameterGuidance, key)"},
			allowed:  []string{`options.ParameterGuidance["project_id"] = guidance`, `if g, ok := route.ParameterGuidance["project_id"]; ok {`, "route.ParameterGuidance = merged"},
		},
		{
			what:     "a route's schema map is written into",
			pattern:  regexp.MustCompile(`(?i)\broute\.(Input|Output)Schema\s*\[[^\]]*\]\s*=[^=]|(?i)\bdelete\(\s*[\w.]*route\.(Input|Output)Schema\b`),
			remedy:   "copy it with toolutil.CloneSchemaMap first; the route's schemas are frozen and shared with every server in the process",
			offences: []string{`route.InputSchema["required"] = names`, `entry.Route.OutputSchema["type"]="object"`, "delete(route.InputSchema, \"required\")"},
			allowed:  []string{`schema["required"] = names`, `if props, ok := route.InputSchema["properties"]; ok {`, "route.OutputSchema = cloned"},
		},
	}
	// A text guard is only worth what it matches, so each pattern is held to
	// the shapes it must refuse and the shapes it must leave alone.
	for _, rule := range forbidden {
		for _, offence := range rule.offences {
			if !rule.pattern.MatchString(offence) {
				t.Errorf("the guard for %q does not match %q", rule.what, offence)
			}
		}
		for _, allowed := range rule.allowed {
			if rule.pattern.MatchString(allowed) {
				t.Errorf("the guard for %q wrongly matches %q", rule.what, allowed)
			}
		}
	}

	offenders := make(map[string][]string)
	tree := os.DirFS(".")
	err := fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := fs.ReadFile(tree, path)
		if readErr != nil {
			return readErr
		}
		for _, rule := range forbidden {
			if rule.pattern.Match(source) {
				offenders[rule.what] = append(offenders[rule.what], path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the package tree: %v", err)
	}
	for _, rule := range forbidden {
		if files := offenders[rule.what]; len(files) > 0 {
			t.Errorf("%s in %v; %s", rule.what, files, rule.remedy)
		}
	}
}

// individualOptionsWithSchemaKey returns the individual registration options
// with a compiled-schema cache key private to one test and one server, so a
// cache hit can never stand in for the schema maps a comparison is about.
func individualOptionsWithSchemaKey(t *testing.T, server string) IndividualCatalogRegisterOptions {
	t.Helper()
	return IndividualCatalogRegisterOptions{
		IncludeStandaloneUtilities: true,
		SchemaCacheKey:             "individual|" + t.Name() + "|" + server,
	}
}

// contains reports whether values holds needle.
func contains(values []string, needle string) bool {
	return slices.Contains(values, needle)
}
