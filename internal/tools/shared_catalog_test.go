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
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
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
// build from one origin, register at once, list the same tools, leave the
// schema maps they all share exactly as they found them, and each run their
// actions under their own credential. Run under the race detector, this is
// the proof that the caches and the shared maps are read concurrently and
// written once.
//
// Building and registering are two bursts rather than one so that the schema
// digests can be taken between them: maps a burst of registrations must not
// change have to be read before it runs. Both halves stay concurrent, which
// is what the cold catalog cache and the compiled-schema cache are raced for.
//
// The listing comparison is not what says the schemas agree, and cannot be.
// In opaque mode a meta-tool's input schema is a pure function of the tool
// name and the sorted action names, which is exactly the key it is compiled
// and cached under, so every entry is served one compiled schema whatever
// maps its own catalog carries; giving each server a cache key of its own,
// the way the individual test does, would only compare two rebuilds of that
// same pure function. What the listing does carry per entry is the
// description and the annotations, both derived at registration from that
// entry's own routes. The schemas are covered by the digests instead, and the
// binding of a shared catalog to one credential by the health action.
func TestSharedMetaCatalog_ConcurrentEntriesBuildSafely(t *testing.T) {
	const entries = 6
	clients := make([]*gitlabclient.Client, entries)
	for i := range clients {
		clients[i] = testutil.NewTestClient(t, healthyGitLab())
	}
	cfg := &config.ServerConfig{Tier: edition.Premium, ExcludeTools: []string{"race-" + t.Name()}}

	catalogs := make([]*actioncatalog.Catalog, entries)
	origins := make([]*actioncatalog.Catalog, entries)
	failures := make([]error, entries)
	var building sync.WaitGroup
	for i := range entries {
		building.Go(func() {
			catalog, _, err := SharedMetaCatalog(clients[i], cfg)
			if err != nil {
				failures[i] = err
				return
			}
			catalogs[i], origins[i] = catalog, catalog.SharedOrigin()
		})
	}
	building.Wait()
	for i := range entries {
		if failures[i] != nil {
			t.Fatalf("entry %d failed to build: %v", i, failures[i])
		}
		if origins[i] == nil || origins[i] != origins[0] {
			t.Fatalf("entry %d has origin %p, want entry 0's %p", i, origins[i], origins[0])
		}
	}
	before := schemaDigests(t, catalogs[0])

	lists := make([][]byte, entries)
	var registering sync.WaitGroup
	for i := range entries {
		registering.Go(func() {
			server := newListingServer()
			RegisterMetaCatalog(server, catalogs[i])
			RegisterMetaStandaloneTools(server, clients[i])
			tools, err := toolutil.ListRegisteredTools(context.Background(), server, "race")
			if err != nil {
				failures[i] = err
				return
			}
			lists[i], failures[i] = json.Marshal(tools)
		})
	}
	registering.Wait()

	reached := make(map[string]int, entries)
	for i := range entries {
		if failures[i] != nil {
			t.Fatalf("entry %d failed to register: %v", i, failures[i])
		}
		if !bytes.Equal(lists[i], lists[0]) {
			t.Fatalf("entry %d lists different tools from entry 0", i)
		}
		if !reflect.DeepEqual(schemaDigests(t, catalogs[i]), before) {
			t.Fatalf("a burst of registrations changed a schema map entry %d shares with every other entry", i)
		}
		if url := gitlabURLReached(t, catalogs[i]); reached[url] != 0 {
			t.Fatalf("entry %d reached %q, the instance of entry %d", i, url, reached[url]-1)
		} else {
			reached[url] = i + 1
		}
	}
}

// TestSharedMetaCatalog_SafeModePreviewsOnEveryServerOfOneKey drives safe mode
// to the wire on two servers built from one shared safe-mode catalog: each
// calls a mutating action through its own MCP session, each receives a preview
// naming the action, and neither instance is asked for anything.
//
// Safe mode is a rewrite of the catalog rather than a wrapper on the server,
// so a catalog shared between pool entries is exactly where it could go wrong:
// one entry previewing and another executing would be invisible to a test that
// builds one server.
func TestSharedMetaCatalog_SafeModePreviewsOnEveryServerOfOneKey(t *testing.T) {
	cfg := &config.ServerConfig{Tier: edition.Free, SafeMode: true, ExcludeTools: []string{"safe-" + t.Name()}}
	var requests [2]atomic.Int32
	catalogs := make([]*actioncatalog.Catalog, len(requests))
	clients := make([]*gitlabclient.Client, len(requests))
	for i := range requests {
		counting := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests[i].Add(1)
			respondJSON(w, http.StatusOK, `{"id":1}`)
		})
		clients[i] = testutil.NewTestClient(t, counting)
		catalog, _, err := SharedMetaCatalog(clients[i], cfg)
		if err != nil {
			t.Fatalf("SharedMetaCatalog(%d) error = %v", i, err)
		}
		catalogs[i] = catalog
	}
	if catalogs[0].SharedOrigin() == nil || catalogs[0].SharedOrigin() != catalogs[1].SharedOrigin() {
		t.Fatal("the two safe-mode servers did not get catalogs bound from one shared origin")
	}

	for i := range catalogs {
		t.Run("server "+strconv.Itoa(i), func(t *testing.T) {
			server := newListingServer()
			RegisterMetaCatalog(server, catalogs[i])
			RegisterMetaStandaloneTools(server, clients[i])
			result := callTool(t, server, "gitlab_issue", json.RawMessage(`{"action":"create","params":{"project_id":"1","title":"t"}}`))
			if result.IsError {
				t.Fatalf("gitlab_issue create returned an error result: %q", extractText(t, result))
			}
			var preview struct {
				Status string `json:"status"`
				Mode   string `json:"mode"`
				Tool   string `json:"tool"`
			}
			text := extractText(t, result)
			if err := json.Unmarshal([]byte(text), &preview); err != nil {
				t.Fatalf("gitlab_issue create returned %q, which is not a preview: %v", text, err)
			}
			if preview.Status != "blocked" || preview.Mode != "safe" || preview.Tool != "issue.create" {
				t.Errorf("gitlab_issue create returned %+v, want a blocked safe-mode preview naming issue.create", preview)
			}
			if reached := requests[i].Load(); reached != 0 {
				t.Errorf("server %d made %d requests to GitLab, want none in safe mode", i, reached)
			}
		})
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
// process. No package under internal/tools or internal/toolutil may:
//
//   - assign Route.Handler directly, because a handler replaced that way is
//     dropped the moment a shared catalog rebinds the route (the helpers that
//     change a handler and its binder together live in toolutil), or
//   - write into a route's ParameterGuidance map or either of its schema maps,
//     because the catalog those belong to is built once per configuration and
//     handed to every credential's server: one write reaches all of them, and
//     no test of the writing package can see it.
//
// Both trees are walked, because the route type, the guidance helpers and the
// schema derivations live in toolutil: a guard that stopped at the domain
// packages left the package most able to make the mistake outside it. What
// toolutil is allowed to do, it is allowed to do by name, in rule.exempt.
//
// Test files are walked too, for the two map rules. A test is where the
// mistake is most invisible: a route from [toolutil.RouteFunc] carries the
// schema memoized for its input type, which lives for the process and is read
// by every other test that reflects the same type, so a write into it there
// reaches them all and nothing reports it. The guard used to skip every
// _test.go, and one such write was sitting in
// TestIndividualToolFromActionSpec_RemovesStaleRequired. The Handler rule
// stays on non-test files: its hazard is a handler dropped when a shared
// catalog rebinds the route, which a test that builds and calls its own route
// never reaches, and a test may legitimately install one.
//
// A whole-field assignment is not an offense and is not matched: a route is a
// value, so replacing a field replaces it on a copy. Only a write that reaches
// through to the map itself is: an indexed write, a delete, a clear, and a
// bulk copy into it. A test that owns the route it writes into is therefore
// still refused by the text; the ones that do say so in rule.exempt.
//
// It is a text guard, so it reads the names a route is usually held under
// rather than resolving types. That is enough for what it is for: keeping the
// habit visible at review time on the roughly 800 files where the mistake
// would be silent. What each pattern does and does not match is pinned by
// [TestSharedRouteStateGuard_MatchesTheShapesItMustRefuse].
func TestDomainPackages_LeaveSharedRouteStateAlone(t *testing.T) {
	t.Parallel()

	rules := sharedRouteStateRules()
	offenders := make(map[string][]string)
	// sequential: both trees accumulate into one set of offenders, asserted per rule below.
	for _, root := range []string{".", "../toolutil"} {
		if err := collectSharedRouteStateOffenders(root, rules, offenders); err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	for _, rule := range rules {
		t.Run(rule.what, func(t *testing.T) {
			t.Parallel()
			if files := offenders[rule.what]; len(files) > 0 {
				t.Errorf("%s in %v; %s", rule.what, files, rule.remedy)
			}
		})
	}
}

// collectSharedRouteStateOffenders walks one tree and records, per rule, every
// Go file that makes the write the rule refuses. A rule that does not cover
// tests skips the _test.go files; an exempt file is skipped by name.
func collectSharedRouteStateOffenders(root string, rules []sharedRouteStateRule, offenders map[string][]string) error {
	tree := os.DirFS(root)
	return fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		source, readErr := fs.ReadFile(tree, path)
		if readErr != nil {
			return readErr
		}
		named := filepath.ToSlash(filepath.Join(root, path))
		isTest := strings.HasSuffix(path, "_test.go")
		for _, rule := range rules {
			if (isTest && !rule.coversTests) || slices.Contains(rule.exempt, named) {
				continue
			}
			if rule.pattern.Match(source) {
				offenders[rule.what] = append(offenders[rule.what], named)
			}
		}
		return nil
	})
}

// TestSharedRouteStateGuard_MatchesTheShapesItMustRefuse holds every pattern
// of the source guard to the writes it must catch and the reads it must leave
// alone. A text guard is worth exactly what it matches, and a guard that
// matched nothing would pass [TestDomainPackages_LeaveSharedRouteStateAlone]
// on every tree.
func TestSharedRouteStateGuard_MatchesTheShapesItMustRefuse(t *testing.T) {
	t.Parallel()

	for _, rule := range sharedRouteStateRules() {
		t.Run(rule.what, func(t *testing.T) {
			t.Parallel()
			for _, offense := range rule.offenses {
				if !rule.pattern.MatchString(offense) {
					t.Errorf("the guard does not match %q", offense)
				}
			}
			for _, allowed := range rule.allowed {
				if rule.pattern.MatchString(allowed) {
					t.Errorf("the guard wrongly matches %q", allowed)
				}
			}
			assertExemptionsStillApply(t, rule)
		})
	}
}

// assertExemptionsStillApply fails when a file exempt from a rule no longer
// makes the write it was exempted for. An exemption that covers nothing is an
// exemption that would silently cover the next write made in that file.
func assertExemptionsStillApply(t *testing.T, rule sharedRouteStateRule) {
	t.Helper()
	for _, path := range rule.exempt {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("exempt file %s cannot be read: %v", path, err)
			continue
		}
		if !rule.pattern.Match(source) {
			t.Errorf("%s is exempt from %q but no longer makes that write; drop the exemption", path, rule.what)
		}
	}
}

// sharedRouteStateRule is one forbidden write: what it is, the pattern that
// finds it, whether test files are walked for it, the remedy the failure
// names, the files allowed to make it, and the samples that pin the pattern's
// reach in both directions.
type sharedRouteStateRule struct {
	what        string
	pattern     *regexp.Regexp
	coversTests bool
	remedy      string
	exempt      []string
	offenses    []string
	allowed     []string
}

// mutatingMapCalls are the calls that change the map passed as their first
// argument. Each rule below refuses one of them over a route's map, and none
// of them refuses the same call over a copy, since the route's map is then
// the second argument and the pattern requires it first.
const mutatingMapCalls = `\b(?:delete|clear|maps\.Copy|maps\.Insert|maps\.DeleteFunc)\(\s*`

// sharedRouteStateRules returns the writes no package under internal/tools or
// internal/toolutil may make into the state a shared catalog gives every
// server in the process.
func sharedRouteStateRules() []sharedRouteStateRule {
	return []sharedRouteStateRule{
		{
			what:    "Route.Handler is assigned directly",
			pattern: regexp.MustCompile(`\.Handler\s*=[^=]`),
			remedy:  "use ActionRoute.WrapHandler or ActionRoute.WithBoundHandler so the binder changes with it",
			// The file those two helpers live in, which is where the
			// assignment is the implementation rather than the mistake. It
			// installs the handler and the binder together, and
			// ValidateRouteBinding is the check that the two agree.
			exempt:   []string{"../toolutil/meta_tool.go"},
			offenses: []string{"route.Handler = wrapped", "action.Route.Handler=wrapped"},
			allowed:  []string{"if route.Handler == nil {", "Handler: wrapped,"},
		},
		{
			what:        "a route's ParameterGuidance is written into",
			pattern:     regexp.MustCompile(`(?i)\broute\.ParameterGuidance\s*\[[^\]]*\]\s*=[^=]|(?i)` + mutatingMapCalls + `[\w.]*route\.ParameterGuidance\b`),
			coversTests: true,
			remedy:      "build the guidance the spec is given instead; the route's map is shared with every server in the process",
			// This file spells every offense as a string literal, and the one
			// test below writes into the guidance of a route it built itself,
			// from a map literal, which is the point of that test.
			exempt:   []string{"shared_catalog_test.go", "../toolutil/action_spec_test.go"},
			offenses: []string{`route.ParameterGuidance["project_id"] = guidance`, `action.Route.ParameterGuidance["ref"]=g`, "delete(spec.Route.ParameterGuidance, key)", "clear(route.ParameterGuidance)", "maps.Copy(route.ParameterGuidance, extra)", "maps.Insert(route.ParameterGuidance, seq)", "maps.DeleteFunc(route.ParameterGuidance, drop)"},
			allowed:  []string{`options.ParameterGuidance["project_id"] = guidance`, `if g, ok := route.ParameterGuidance["project_id"]; ok {`, "route.ParameterGuidance = merged", "maps.Copy(mine, route.ParameterGuidance)", "maps.Clone(route.ParameterGuidance)"},
		},
		{
			what:        "a route's schema map is written into",
			pattern:     regexp.MustCompile(`(?i)\broute\.(Input|Output)Schema\s*\[[^\]]*\]\s*=[^=]|(?i)` + mutatingMapCalls + `[\w.]*route\.(Input|Output)Schema\b`),
			coversTests: true,
			remedy:      "copy it with toolutil.CloneSchemaMap first; the route's schemas are frozen and shared with every server in the process",
			// This file spells every offense as a string literal.
			exempt:   []string{"shared_catalog_test.go"},
			offenses: []string{`route.InputSchema["required"] = names`, `entry.Route.OutputSchema["type"]="object"`, "delete(route.InputSchema, \"required\")", "clear(route.InputSchema)", "maps.Copy(route.OutputSchema, extra)", "maps.Insert(route.InputSchema, seq)", "maps.DeleteFunc(route.InputSchema, drop)"},
			allowed:  []string{`schema["required"] = names`, `if props, ok := route.InputSchema["properties"]; ok {`, "route.OutputSchema = cloned", "maps.Copy(mine, route.InputSchema)", "maps.Clone(route.InputSchema)"},
		},
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

// sharingTiers are the three instance tiers a catalog is built for. Tier
// pruning rewrites the schema maps, so a hazard that only exists in a pruned
// schema is only visible when all three are walked.
func sharingTiers() []edition.Tier {
	return []edition.Tier{edition.Free, edition.Premium, edition.Ultimate}
}

// writableObjectsIn walks value and returns, keyed by address, the path to
// every object reachable from it that a goroutine could write into: maps,
// slices, and pointers. Strings and numbers are left out because nothing can
// write through them, and an empty map or slice is left out because it holds
// nothing a write could reach without first growing it, which allocates a new
// one.
//
// It is deliberately written against reflect rather than against the three
// types [toolutil.CloneSchemaMap] copies, so that a container the clone does
// not know about is found rather than assumed absent.
func writableObjectsIn(value any) map[uintptr]string {
	found := make(map[uintptr]string)
	walkWritable(reflect.ValueOf(value), "", found)
	return found
}

// walkWritable is the dispatch half of [writableObjectsIn]: it records the
// value in front of it when that value is an object a write can reach, and
// hands the containers to the walkers below.
func walkWritable(v reflect.Value, path string, found map[uintptr]string) {
	switch v.Kind() {
	case reflect.Interface:
		if !v.IsNil() {
			walkWritable(v.Elem(), path, found)
		}
	case reflect.Pointer:
		if !v.IsNil() && recordWritable(found, v.Pointer(), path) {
			walkWritable(v.Elem(), path, found)
		}
	case reflect.Map:
		walkWritableMap(v, path, found)
	case reflect.Slice:
		walkWritableSlice(v, path, found)
	case reflect.Array, reflect.Struct:
		walkWritableFixed(v, path, found)
	default:
	}
}

func walkWritableMap(v reflect.Value, path string, found map[uintptr]string) {
	if v.IsNil() || v.Len() == 0 || !recordWritable(found, v.Pointer(), path) {
		return
	}
	for _, key := range v.MapKeys() {
		walkWritable(v.MapIndex(key), path+"."+writableKeyName(key), found)
	}
}

func walkWritableSlice(v reflect.Value, path string, found map[uintptr]string) {
	if v.IsNil() || v.Len() == 0 || !recordWritable(found, v.Pointer(), path) {
		return
	}
	for i := range v.Len() {
		walkWritable(v.Index(i), path+"["+strconv.Itoa(i)+"]", found)
	}
}

// walkWritableFixed descends an array or a struct. Neither is an object of its
// own: both live inside whatever holds them, so there is nothing to record and
// only their elements or fields are walked.
func walkWritableFixed(v reflect.Value, path string, found map[uintptr]string) {
	if v.Kind() == reflect.Struct {
		for i := range v.NumField() {
			walkWritable(v.Field(i), path+"."+v.Type().Field(i).Name, found)
		}
		return
	}
	for i := range v.Len() {
		walkWritable(v.Index(i), path+"["+strconv.Itoa(i)+"]", found)
	}
}

// recordWritable records address under path and reports whether it was new,
// which is what stops the walk from following a cycle or repeating a shared
// subtree.
func recordWritable(found map[uintptr]string, address uintptr, path string) bool {
	if _, seen := found[address]; seen {
		return false
	}
	found[address] = path
	return true
}

// writableKeyName renders a map key for a path. Schema maps are keyed by
// string; anything else is named by its kind rather than by a value that
// cannot be read out of an unexported field.
func writableKeyName(key reflect.Value) string {
	if key.Kind() == reflect.String {
		return key.String()
	}
	return "<" + key.Kind().String() + ">"
}

// cloneAliasReport returns one line per writable object a deep copy of schema
// still shares with the original, and how many objects the copy owns, so a
// caller can tell an empty report from an empty walk.
//
// Object identity is the question rather than a list of types, on purpose.
// [toolutil.CloneSchemaMap] copies map[string]any, []any and []string and
// returns everything else by reference, so a []map[string]any, a
// map[string]string or a []int anywhere in a schema would survive the copy
// shared, and a later write through the "copy" would land in the map every
// server in the process is reading. Asking what the two trees have in common
// finds that without having to predict which container someone adds next.
func cloneAliasReport(schema map[string]any) (aliased []string, copied int) {
	if len(schema) == 0 {
		return nil, 0
	}
	clone := toolutil.CloneSchemaMap(schema)
	original := writableObjectsIn(schema)
	inClone := writableObjectsIn(clone)
	// Keep both alive until the comparison is over, so no address in either
	// map can name an object the collector has already reclaimed.
	defer runtime.KeepAlive(schema)
	defer runtime.KeepAlive(clone)
	for address, path := range inClone {
		if shared, ok := original[address]; ok {
			aliased = append(aliased, "the copy at "+path+" is the original at "+shared)
		}
	}
	sort.Strings(aliased)
	return aliased, len(inClone)
}

// reportCloneAliases fails t for every writable object a deep copy of schema
// still shares with it, and returns how many objects that copy owns, so a
// caller can tell an empty report from an empty walk.
func reportCloneAliases(t *testing.T, name string, schema map[string]any) int {
	t.Helper()
	aliased, copied := cloneAliasReport(schema)
	for _, line := range aliased {
		t.Errorf("%s: %s", name, line)
	}
	return copied
}

// TestSharedCatalogSchemas_CloneOwnsEverythingItCopies walks every schema the
// real catalogs carry, at all three tiers, and fails on any value the deep
// copy would hand back shared with the original.
//
// This is the first half of the answer to the concurrent map write seen once
// on Windows during individual-surface registration: the writing goroutine
// could have been one that believed it held a private copy. If any schema in
// the catalog held a container the copy does not descend into, every write
// through that copy would land in the process-lived original while another
// goroutine was cloning it, which is exactly the crash.
func TestSharedCatalogSchemas_CloneOwnsEverythingItCopies(t *testing.T) {
	t.Parallel()

	for _, tier := range sharingTiers() {
		t.Run(tier.String(), func(t *testing.T) {
			t.Parallel()
			client := testutil.NewTestClient(t, healthyGitLab())
			catalog, _, err := SharedIndividualCatalog(client, &config.ServerConfig{Tier: tier})
			if err != nil {
				t.Fatalf("SharedIndividualCatalog() error = %v", err)
			}
			actions := catalog.Actions()
			if len(actions) == 0 {
				t.Fatal("the catalog carries no actions, so nothing below is checked")
			}
			walked := 0
			for _, action := range actions {
				walked += reportCloneAliases(t, string(action.ID)+" input", action.Route.InputSchema)
				walked += reportCloneAliases(t, string(action.ID)+" output", action.Route.OutputSchema)
			}
			// A schema tree is tens of maps deep in properties and $defs, so
			// a walk that found only a handful of objects walked nothing.
			if walked < len(actions) {
				t.Errorf("the walk owned %d objects across %d actions, want at least one each; the comparison above proved nothing", walked, len(actions))
			}
		})
	}
}

// sharedCatalogReader is one consumer of a shared catalog, named by what it
// does so a failure says which reader was running. meta selects the catalog it
// is given: the meta surface needs the catalog its own filter produced, and
// every other reader takes the individual one. Both carry the same schema
// maps, which is what these readers are racing over.
type sharedCatalogReader struct {
	name string
	meta bool
	read func(t *testing.T, catalog *actioncatalog.Catalog, client *gitlabclient.Client, label string)
}

// sharedCatalogReaders returns every consumer that reads one shared catalog in
// a running server, in the shapes the server uses them.
//
// The list is the point. A shared catalog's schemas are read by the individual
// projection, the meta projection, the dynamic registry, the gitlab://tools
// manifest and the two tools/list middlewares, each on its own goroutine in a
// pool that builds a server per configuration shape. A test that drives one of
// them proves nothing about a write made by another.
func sharedCatalogReaders() []sharedCatalogReader {
	return []sharedCatalogReader{
		{name: "individual registration and listing", read: readByRegisteringIndividual},
		{name: "meta registration and listing", meta: true, read: readByRegisteringMeta},
		{name: "dynamic registry", read: readByDynamicRegistry},
		{name: "tool manifest snapshot", read: readByToolManifest},
		{name: "individual tool projection", read: readByProjectingIndividualTools},
		{name: "meta action schemas", meta: true, read: readByMetaActionSchemas},
		{name: "deep copies of every route schema", read: readByCloningEverySchema},
	}
}

// readByRegisteringIndividual registers the individual surface on a server of
// its own and lists it through both tools/list middlewares. This is the path
// the reported crash was on: registration re-derives each route's input schema
// through NewActionSpec, which deep-copies it.
func readByRegisteringIndividual(t *testing.T, catalog *actioncatalog.Catalog, _ *gitlabclient.Client, label string) {
	t.Helper()
	server := newListingServer()
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
		IncludeStandaloneUtilities: true,
		SchemaCacheKey:             "individual|" + label,
	})
	listSharedCatalogTools(t, server, label)
}

// readByRegisteringMeta registers the meta surface and lists it, which builds
// the per-tool envelopes over the same route schemas.
func readByRegisteringMeta(t *testing.T, catalog *actioncatalog.Catalog, client *gitlabclient.Client, label string) {
	t.Helper()
	server := newListingServer()
	RegisterMetaCatalog(server, catalog)
	RegisterMetaStandaloneTools(server, client)
	listSharedCatalogTools(t, server, label)
}

// readByDynamicRegistry builds the dynamic registry over the catalog and asks
// it the two questions that carry schemas back: a find and a describe.
func readByDynamicRegistry(t *testing.T, catalog *actioncatalog.Catalog, _ *gitlabclient.Client, _ string) {
	t.Helper()
	registry := dynamic.NewRegistryFromCatalog(catalog)
	if _, found, err := registry.Find(context.Background(), nil, dynamic.FindInput{Query: "create merge request"}); err != nil {
		t.Errorf("dynamic Find() error = %v", err)
	} else if len(found.Results) == 0 {
		t.Error("dynamic Find() returned no results, so it carried no schema")
	}
	if _, described, err := registry.Describe(context.Background(), nil, dynamic.DescribeInput{Action: "project.get"}); err != nil {
		t.Errorf("dynamic Describe() error = %v", err)
	} else if len(described.Actions) == 0 {
		t.Error("dynamic Describe() described nothing")
	}
}

// readByToolManifest builds the gitlab://tools snapshot in both surfaces that
// project catalog actions into it, each of which reads a route's schema for
// every entry it publishes. No share key is given, so a cached snapshot never
// stands in for the projection this is exercising.
func readByToolManifest(t *testing.T, catalog *actioncatalog.Catalog, _ *gitlabclient.Client, _ string) {
	t.Helper()
	routes := catalog.ActionMaps()
	for _, surface := range []string{"dynamic", "meta"} {
		resources.RegisterToolSurfaceResources(newListingServer(), resources.ToolSurfaceResourceOptions{
			Surface:    surface,
			Catalog:    catalog,
			MetaRoutes: routes,
		})
	}
}

// readByProjectingIndividualTools runs the projection alone, without a server,
// so the deep copy inside NewActionSpec runs for every action rather than only
// for the ones a registration reaches.
func readByProjectingIndividualTools(t *testing.T, catalog *actioncatalog.Catalog, _ *gitlabclient.Client, _ string) {
	t.Helper()
	for _, action := range catalog.Actions() {
		if strings.TrimSpace(action.IndividualTool.Name) == "" {
			continue
		}
		if _, err := toolutil.IndividualToolFromActionSpec(actionSpecFromCatalogAction(action), toolutil.IndividualToolProjectionOptions{
			Description: "Projected for the concurrency guard.",
		}); err != nil {
			t.Errorf("IndividualToolFromActionSpec(%s) error = %v", action.ID, err)
		}
	}
}

// readByMetaActionSchemas asks for the params schema every meta-tool action
// serves, which enriches a copy of the route's schema with the destructive
// property and the parameter guidance.
func readByMetaActionSchemas(t *testing.T, catalog *actioncatalog.Catalog, _ *gitlabclient.Client, _ string) {
	t.Helper()
	routes := catalog.ActionMaps()
	for tool, actions := range routes {
		for action := range actions {
			if _, ok := toolutil.LookupMetaActionSchema(routes, tool, action); !ok {
				t.Errorf("LookupMetaActionSchema(%s, %s) reported the action missing", tool, action)
			}
		}
	}
}

// readByCloningEverySchema deep-copies every schema in the catalog, which is
// the iteration the reported crash died in. It runs unconditionally rather
// than through a memo, so the window a memoized derivation only opens once per
// process is open on every round.
func readByCloningEverySchema(t *testing.T, catalog *actioncatalog.Catalog, _ *gitlabclient.Client, _ string) {
	t.Helper()
	for _, action := range catalog.Actions() {
		if got := toolutil.CloneSchemaMap(action.Route.InputSchema); len(got) != len(action.Route.InputSchema) {
			t.Errorf("%s: the copy has %d keys, want %d", action.ID, len(got), len(action.Route.InputSchema))
		}
		toolutil.CloneSchemaMap(action.Route.OutputSchema)
	}
}

// listSharedCatalogTools drives a tools/list, which is what runs the lockdown
// and pagination middlewares over the schemas the server registered.
func listSharedCatalogTools(t *testing.T, server *mcp.Server, label string) {
	t.Helper()
	tools, err := toolutil.ListRegisteredTools(context.Background(), server, "shared-catalog-race")
	if err != nil {
		t.Errorf("%s: ListRegisteredTools() error = %v", label, err)
		return
	}
	if len(tools) == 0 {
		t.Errorf("%s: the server listed no tools", label)
	}
}

// TestSharedCatalog_EveryReaderRunsAtOnceOverOneCatalog runs every consumer of
// a shared catalog concurrently over the same catalog, at all three tiers, so
// that a write into a schema any of them reads is a race the detector reports
// and a concurrent map access the runtime's own check can catch.
//
// It exists because of a fatal "concurrent map iteration and map write" seen
// once on the Windows leg of CI, inside the deep copy that individual-surface
// registration makes of a route's input schema. The iterating side was in the
// stack; the writing side was not, because Go's built-in check prints only the
// running goroutine. No writer has been found: nothing under internal, cmd,
// the MCP SDK or the JSON Schema library writes into a schema map it did not
// allocate, and the copy that registration makes shares no writable object
// with the original, which
// [TestSharedCatalogSchemas_CloneOwnsEverythingItCopies] checks over the real
// catalogs. What the crashing job does carry is the known Go runtime crash of
// issue 467 (golang/go#81238): its first run of this same package died inside
// the collector, dereferencing a bad type pointer while scanning an object,
// and the map fatal came from the rerun the crash-aware runner started on that
// same host. Across the Windows jobs of the last two dozen failed CI runs, the
// map fatal appears in exactly that one, the one job that also carries the
// runtime crash.
//
// So this stays as a standing guard rather than a reproduction: if a writer
// does exist, it is a process crash in production, and this is where it is
// caught. Raise -count for a soak; the readers are the same on every round,
// and a round costs seconds.
func TestSharedCatalog_EveryReaderRunsAtOnceOverOneCatalog(t *testing.T) {
	t.Parallel()

	const workersPerReader = 2
	for _, tier := range sharingTiers() {
		t.Run(tier.String(), func(t *testing.T) {
			t.Parallel()
			client := testutil.NewTestClient(t, healthyGitLab())
			cfg := &config.ServerConfig{Tier: tier}
			individual, _, err := SharedIndividualCatalog(client, cfg)
			if err != nil {
				t.Fatalf("SharedIndividualCatalog() error = %v", err)
			}
			meta, _, metaErr := SharedMetaCatalog(client, cfg)
			if metaErr != nil {
				t.Fatalf("SharedMetaCatalog() error = %v", metaErr)
			}
			var readers sync.WaitGroup
			for _, reader := range sharedCatalogReaders() {
				catalog := individual
				if reader.meta {
					catalog = meta
				}
				for worker := range workersPerReader {
					label := tier.String() + "|" + reader.name + "|" + strconv.Itoa(worker)
					readers.Go(func() { reader.read(t, catalog, client, label) })
				}
			}
			readers.Wait()
		})
	}
}

// TestCloneAliasReport_FindsTheContainersTheCopyDoesNotDescendInto holds the
// detector above to the shapes it exists to catch. A report that could not
// fail would pass [TestSharedCatalogSchemas_CloneOwnsEverythingItCopies] on
// any catalog, however the copy was written.
func TestCloneAliasReport_FindsTheContainersTheCopyDoesNotDescendInto(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		schema     map[string]any
		wantShared bool
	}{
		{
			name:   "the JSON types a schema is made of",
			schema: map[string]any{"type": "object", "properties": map[string]any{"page": map[string]any{"type": "integer", "minimum": 1}}, "required": []string{"page"}, "anyOf": []any{map[string]any{"required": []any{"page"}}}},
		},
		{
			name:       "a map the copy does not know",
			schema:     map[string]any{"properties": map[string]any{"ref": map[string]any{"x_tiers": map[string]string{"ref": "premium"}}}},
			wantShared: true,
		},
		{
			name:       "a slice of maps the copy does not know",
			schema:     map[string]any{"properties": map[string]any{"ref": map[string]any{"x_examples": []map[string]any{{"value": "main"}}}}},
			wantShared: true,
		},
		{
			name:       "a slice of a type the copy does not know",
			schema:     map[string]any{"properties": map[string]any{"ids": map[string]any{"x_defaults": []int{1, 2}}}},
			wantShared: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			aliased, copied := cloneAliasReport(tc.schema)
			if copied == 0 {
				t.Fatal("the walk found nothing to compare")
			}
			if shared := len(aliased) > 0; shared != tc.wantShared {
				t.Errorf("shared = %v (%v), want %v", shared, aliased, tc.wantShared)
			}
		})
	}
}
