package mcpsurface

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamiccatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// The dynamic surface is the default a user gets with no configuration, and it
// is a fixed two-tool contract. Generators name the pair so a rename shows up as
// a failed generation rather than as silently different output.
const (
	DynamicFindToolName          = "gitlab_find_action"
	DynamicExecuteActionToolName = "gitlab_execute_action"
)

// listPageSize is high enough that the whole surface arrives in a single list
// response, so callers never have to paginate. It has to be stated: a server
// that leaves ServerOptions.PageSize at zero gets the SDK's DefaultPageSize of
// 1000, which is fewer entries than the individual surface carries at Ultimate.
//
// That was not a hypothetical. Two of the readers this package replaced set no
// page size and listed once, so they measured and audited the first 1000 of
// 1085 individual tools and said nothing about the 85 they never saw.
// [requireCompleteListing] is what keeps the next such regression loud.
const listPageSize = 2000

// StubToken is the dummy credential the generators authenticate their stub
// client with. It is never sent to a real GitLab instance: NewStubClient points
// the client at an in-process HTTP server. A generator must not fall back to
// GITLAB_TOKEN, or a machine that exports one would produce different output
// from a machine that does not.
const StubToken = "gen-surface-token" //#nosec G101 -- not a real credential, in-process stub only

// newGitLabClient is the client constructor, as a variable so a test can drive
// the one failure [NewStubClientWithToken] can take. Every input it is given
// here is fixed by this package, so nothing a caller passes can provoke it.
var newGitLabClient = gitlabclient.NewClient //nolint:gochecknoglobals // test seam

// NewStubClient returns a GitLab client backed by an in-process stub that
// answers every request with a fixed version payload, plus the cleanup func that
// shuts the stub down. Catalog construction needs a client but performs no real
// request, so this keeps generation offline and identical on every machine.
func NewStubClient() (client *gitlabclient.Client, cleanup func()) {
	return NewStubClientWithToken(StubToken)
}

// NewStubClientWithToken is [NewStubClient] with the caller's dummy token, for
// the commands that name their own. Retries are disabled: the stub answers
// every request in one round trip, and a command that somehow reached a
// failing one should say so rather than spend a backoff schedule on it.
//
// It panics rather than returning an error. The only input client construction
// validates is a URL httptest allocated a line earlier, so a failure here is a
// programming error in this package rather than a condition a caller could
// handle: every caller could only print it and stop, which is what a panic
// already does, and nine of them were carrying an unreachable branch to say so.
func NewStubClientWithToken(token string) (client *gitlabclient.Client, cleanup func()) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))

	built, err := newGitLabClient(&config.Config{
		GitLabURL:      srv.URL,
		GitLabToken:    token,
		DisableRetries: true,
	})
	if err != nil {
		srv.Close()
		panic(fmt.Sprintf("mcpsurface: create stub GitLab client: %v", err))
	}
	return built, srv.Close
}

// NewGitLabComClient returns a client pinned to the public GitLab.com URL. The
// catalog registers the GitLab.com-only tools (Orbit) against it, so generated
// documentation can describe the full capability set rather than whatever the
// ambient GITLAB_URL points at.
//
// The URL is the compiled-in default and the token is a constant, so the only
// thing client construction validates is already fixed at build time.
func NewGitLabComClient() *gitlabclient.Client {
	return cmdutil.Must(gitlabclient.NewClient(&config.Config{
		GitLabURL:   config.DefaultGitLabURL,
		GitLabToken: StubToken,
	}))
}

// Session creates an in-memory MCP server+client pair, applies setup to the
// server, and returns the connected client session together with a cleanup
// function the caller must invoke.
//
// The two schema middlewares are the chain cmd/server installs at
// cmd/server/main.go:1802-1808, in that order: the lockdown first, then the
// pagination bounds, which the comment there records must sit inside it so it
// sees the same finalized schema set. A listing that applies neither, or only
// the first, measures and documents a schema no client ever receives — without
// `additionalProperties: false`, with the jsonschema `,required` tag suffixes
// still in the descriptions, and without the page/per_page bounds.
//
// Both ends of an in-memory transport are this process, so neither connect can
// fail; setup registers the catalog compiled into this binary. Reporting either
// as an error would add a return path to every generator that lists a surface
// and none of them could ever take it.
func Session(setup func(*mcp.Server)) (session *mcp.ClientSession, cleanup func()) {
	opts := &mcp.ServerOptions{PageSize: listPageSize, Capabilities: &mcp.ServerCapabilities{}}
	server := mcp.NewServer(&mcp.Implementation{Name: "mcpsurface", Version: "0.0.1"}, opts)
	setup(server)
	toolutil.LockdownInputSchemas(server)
	toolutil.EnrichPaginationConstraints(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession := cmdutil.Must(server.Connect(ctx, serverTransport, nil))

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "mcpsurface-client", Version: "0.0.1"}, nil)
	session = cmdutil.Must(mcpClient.Connect(ctx, clientTransport, nil))

	return session, func() {
		_ = session.Close()
		_ = serverSession.Wait()
	}
}

// requireCompleteListing panics when a list response carries a next cursor,
// which means the server answered with one page of a longer listing and every
// caller is about to describe a surface with entries missing from it.
//
// It panics rather than paginating on purpose. A cursor here is a statement
// about this package's own [listPageSize], not about anything the run
// encountered, and the readers that quietly truncated their listings did so
// for years precisely because nothing said a word. Silence is the failure mode
// this guards; a generator that stops is one a maintainer can fix in a line.
func requireCompleteListing(listing, nextCursor string, count int) {
	if nextCursor != "" {
		panic(fmt.Sprintf("mcpsurface: the %s listing stopped after %d entries with more to come; listPageSize (%d) no longer covers the surface", listing, count, listPageSize))
	}
}

// DynamicCatalog builds the canonical action catalog behind the dynamic
// find/execute surface, including the standalone actions that are not part of
// any domain meta-tool. enterprise selects the Premium/Ultimate catalog.
//
// It delegates to [dynamiccatalog.Build] with a configuration that narrows
// nothing, so the generators describe the catalog the server assembles rather
// than a second assembly of the same parts: an unconfigured deployment is what
// a generated artifact must describe, and this is the function cmd/server
// calls to assemble it. Several audit commands and the e2e suite still put
// their own copy together; each one that moves onto this package is one fewer
// surface that can drift from the served one without a test noticing.
//
// Assembly reads only the ActionSpecs compiled into this binary, so a failure
// means the committed catalog is malformed, which no generator run can fix and
// every caller would only print.
func DynamicCatalog(client *gitlabclient.Client, enterprise bool) *actioncatalog.Catalog {
	catalog, _, err := dynamiccatalog.Build(client, &config.ServerConfig{Tier: edition.TierForEnterprise(enterprise)})
	cmdutil.MustDo(err)
	return catalog
}

// DynamicTools returns the visible two-tool dynamic catalog from a real MCP
// tools/list session, in find-then-execute order.
//
// The contract check is an assertion about compiled-in registration, not about
// anything this run encountered: only an edit to the dynamic surface can break
// it, and it must abort generation rather than rewrite every artifact.
// [ValidateDynamicToolContract] stays exported so the rule itself is tested
// directly.
func DynamicTools(client *gitlabclient.Client) []*mcp.Tool {
	dynamicTools := DynamicToolsFromCatalog(DynamicCatalog(client, true))
	SortDynamicTools(dynamicTools)
	cmdutil.MustDo(ValidateDynamicToolContract(dynamicTools))
	return dynamicTools
}

// DynamicToolsFromCatalog lists the two-tool dynamic surface projected from
// catalog, for the callers that assemble one per tier rather than taking the
// enterprise one [DynamicTools] builds. The contract check is not applied
// here: it is an assertion about the surface a user gets, and a caller
// measuring one tier of it has already chosen the catalog.
func DynamicToolsFromCatalog(catalog *actioncatalog.Catalog) []*mcp.Tool {
	session, cleanup := Session(func(server *mcp.Server) {
		dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	})
	defer cleanup()

	listed := cmdutil.Must(session.ListTools(context.Background(), nil))
	requireCompleteListing(config.ToolSurfaceDynamic+" tools", listed.NextCursor, len(listed.Tools))
	return listed.Tools
}

// listedTools memoizes [IndividualTools] and [MetaTools] per
// (client, surface, tier, meta parameter-schema mode). Registering a full
// surface costs seconds, the result depends only on the compiled-in catalog
// and those inputs, and every caller only reads it.
//
// The schema mode is part of the key because the footprint measurement
// re-lists the meta surface under each mode to size its input schemas; a key
// without it would hand every mode the first listing and report three
// identical sizes. The client is keyed by pointer, which is sound only while
// a client is treated as immutable once built: the tools a client's surface
// carries depend on the instance it names (GitLab.com registers Orbit), and
// nothing here would notice a caller mutating one between calls.
//
// The returned slice and the tools it holds are shared: read-only.
var listedTools sync.Map // listKey -> []*mcp.Tool

// listKey names one memoized listing. Surface is a config.ToolSurface*
// constant.
type listKey struct {
	client     *gitlabclient.Client
	surface    string
	tier       edition.Tier
	schemaMode string
}

// IndividualTools returns the individual surface at tier as a client receives
// it: what cmd/server registers for config.ToolSurfaceIndividual, listed over
// a real tools/list round-trip through [Session]'s served-schema chain.
//
// [tools.RegisterAll] is the server's own pair for this surface — the catalog
// built with IncludeMCP, projected with the standalone utilities — so the
// gitlab_server_* tools are in the result.
func IndividualTools(client *gitlabclient.Client, tier edition.Tier) []*mcp.Tool {
	return listSurface(listKey{client: client, surface: config.ToolSurfaceIndividual, tier: tier, schemaMode: tools.MetaParamSchema()},
		func(server *mcp.Server) {
			tools.RegisterAll(server, client, tier)
		})
}

// MetaTools returns the meta surface at tier as a client receives it: what
// cmd/server registers for config.ToolSurfaceMeta, listed over a real
// tools/list round-trip through [Session]'s served-schema chain.
//
// The catalog is built with IncludeMCP, which is how cmd/server builds the one
// it registers (tools.SharedMetaCatalog, keyed with includeMCP true), so
// gitlab_server is present. [tools.RegisterAllMeta] builds without it and is
// therefore one tool short of the served surface — the difference that had the
// published meta counts saying 33 where the binary serves 34.
func MetaTools(client *gitlabclient.Client, tier edition.Tier) []*mcp.Tool {
	return listSurface(listKey{client: client, surface: config.ToolSurfaceMeta, tier: tier, schemaMode: tools.MetaParamSchema()},
		func(server *mcp.Server) {
			catalog := cmdutil.Must(tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Tier: tier, IncludeMCP: true}))
			tools.RegisterMetaCatalog(server, catalog)
			tools.RegisterMetaStandaloneTools(server, client)
		})
}

// listSurface returns the memoized listing for key, registering it with setup
// on the first call for that key.
func listSurface(key listKey, setup func(*mcp.Server)) []*mcp.Tool {
	if cached, ok := listedTools.Load(key); ok {
		listed, _ := cached.([]*mcp.Tool)
		return listed
	}

	session, cleanup := Session(setup)
	defer cleanup()

	result := cmdutil.Must(session.ListTools(context.Background(), nil))
	requireCompleteListing(key.surface+" tools", result.NextCursor, len(result.Tools))
	listedTools.Store(key, result.Tools)
	return result.Tools
}

// SortDynamicTools orders the dynamic surface find-then-execute, which is the
// order a model should use them in, and sorts anything unexpected by name so the
// output stays deterministic.
func SortDynamicTools(dynamicTools []*mcp.Tool) {
	order := map[string]int{
		DynamicFindToolName:          0,
		DynamicExecuteActionToolName: 1,
	}
	sort.SliceStable(dynamicTools, func(i, j int) bool {
		left, leftOK := order[dynamicTools[i].Name]
		right, rightOK := order[dynamicTools[j].Name]
		if leftOK && rightOK {
			return left < right
		}
		if leftOK != rightOK {
			return leftOK
		}
		return dynamicTools[i].Name < dynamicTools[j].Name
	})
}

// ValidateDynamicToolContract fails when the dynamic surface is no longer
// exactly find plus execute, so a rename or an extra tool aborts generation
// instead of quietly rewriting every generated artifact.
func ValidateDynamicToolContract(dynamicTools []*mcp.Tool) error {
	expected := []string{DynamicFindToolName, DynamicExecuteActionToolName}
	if len(dynamicTools) != len(expected) {
		return fmt.Errorf("expected %d dynamic tools, got %d", len(expected), len(dynamicTools))
	}
	for i, name := range expected {
		if dynamicTools[i].Name != name {
			return fmt.Errorf("unexpected dynamic tool %q at position %d", dynamicTools[i].Name, i)
		}
	}
	return nil
}

// Resources returns the static resources and resource templates advertised by
// the MCP server, including the surface-aware tool manifest template. The
// manifest is rendered for the dynamic surface because that is what a user gets
// by default.
//
// The manifest's catalog is the enterprise one, which is the catalog behind the
// tools listed beside it. It used to be the Free one while the tools came from
// [DynamicTools]' enterprise catalog, which described neither deployment; the
// listing is unaffected either way, since the manifest is two registrations
// whatever it holds, but a mismatched pair is a trap for the next caller that
// reads the manifest's content rather than counting it.
func Resources(client *gitlabclient.Client) ([]*mcp.Resource, []*mcp.ResourceTemplate) {
	dynamicCatalog := DynamicCatalog(client, true)
	dynamicTools := DynamicTools(client)
	session, cleanup := Session(func(server *mcp.Server) {
		resources.Register(server, client)
		resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
			Surface: config.ToolSurfaceDynamic,
			Tools:   dynamicTools,
			Catalog: dynamicCatalog,
		})
		resources.RegisterWorkflowGuides(server)
	})
	defer cleanup()

	ctx := context.Background()
	res := cmdutil.Must(session.ListResources(ctx, nil))
	requireCompleteListing("resources", res.NextCursor, len(res.Resources))
	tpl := cmdutil.Must(session.ListResourceTemplates(ctx, nil))
	requireCompleteListing("resource templates", tpl.NextCursor, len(tpl.ResourceTemplates))
	return res.Resources, tpl.ResourceTemplates
}

// Prompts returns every registered MCP prompt definition over a real
// prompts/list round-trip.
func Prompts(client *gitlabclient.Client) []*mcp.Prompt {
	session, cleanup := Session(func(server *mcp.Server) {
		prompts.Register(server, client)
	})
	defer cleanup()

	listed := cmdutil.Must(session.ListPrompts(context.Background(), nil))
	requireCompleteListing("prompts", listed.NextCursor, len(listed.Prompts))
	return listed.Prompts
}

// ProjectRoot walks up from the working directory to the directory holding
// go.mod, so a generator works from anywhere in the repository.
func ProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}
