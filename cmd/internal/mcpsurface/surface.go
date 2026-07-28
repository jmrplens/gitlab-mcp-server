// Package mcpsurface introspects the registered MCP surface for the generator
// commands.
//
// Several commands under cmd/ need the same thing: the tools, prompts and
// resources the server actually registers, read over a real MCP round-trip
// rather than described by hand, and against a surface that does not depend on
// the ambient environment. The server chooses its catalog from TOOL_SURFACE and
// its client from GITLAB_URL/GITLAB_TOKEN, so a generator that read either would
// emit different files on a developer machine than in CI. Every constructor here
// pins the surface explicitly and talks to an in-process stub instead, which is
// what makes the committed artifacts reproducible.
package mcpsurface

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
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
// response, so callers never have to paginate.
const listPageSize = 2000

// StubToken is the dummy credential the generators authenticate their stub
// client with. It is never sent to a real GitLab instance: NewStubClient points
// the client at an in-process HTTP server. A generator must not fall back to
// GITLAB_TOKEN, or a machine that exports one would produce different output
// from a machine that does not.
const StubToken = "gen-surface-token" //#nosec G101 -- not a real credential, in-process stub only

// NewStubClient returns a GitLab client backed by an in-process stub that
// answers every request with a fixed version payload, plus the cleanup func that
// shuts the stub down. Catalog construction needs a client but performs no real
// request, so this keeps generation offline and identical on every machine.
func NewStubClient() (*gitlabclient.Client, func(), error) {
	return auditshared.NewStubGitLabClient(StubToken)
}

// NewGitLabComClient returns a client pinned to the public GitLab.com URL. The
// catalog registers the GitLab.com-only tools (Orbit) against it, so generated
// documentation can describe the full capability set rather than whatever the
// ambient GITLAB_URL points at.
func NewGitLabComClient() (*gitlabclient.Client, error) {
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:   config.DefaultGitLabURL,
		GitLabToken: StubToken,
	})
	if err != nil {
		return nil, fmt.Errorf("create gitlab.com client: %w", err)
	}
	return client, nil
}

// Session creates an in-memory MCP server+client pair, applies setup to the
// server, and returns the connected client session together with a cleanup
// function the caller must invoke.
func Session(setup func(*mcp.Server) error) (session *mcp.ClientSession, cleanup func(), err error) {
	opts := &mcp.ServerOptions{PageSize: listPageSize}
	server := mcp.NewServer(&mcp.Implementation{Name: "mcpsurface", Version: "0.0.1"}, opts)
	if setupErr := setup(server); setupErr != nil {
		return nil, nil, setupErr
	}
	toolutil.LockdownInputSchemas(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("server connect: %w", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "mcpsurface-client", Version: "0.0.1"}, nil)
	session, err = mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		_ = serverSession.Wait()
		return nil, nil, fmt.Errorf("client connect: %w", err)
	}

	return session, func() {
		_ = session.Close()
		_ = serverSession.Wait()
	}, nil
}

// DynamicCatalog builds the canonical action catalog behind the dynamic
// find/execute surface, including the standalone actions that are not part of
// any domain meta-tool. enterprise selects the Premium/Ultimate catalog.
func DynamicCatalog(client *gitlabclient.Client, enterprise bool) (*actioncatalog.Catalog, error) {
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build dynamic action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add dynamic standalone catalog: %w", err)
	}
	return catalog, nil
}

// DynamicTools returns the visible two-tool dynamic catalog from a real MCP
// tools/list session, in find-then-execute order.
func DynamicTools(client *gitlabclient.Client) ([]*mcp.Tool, error) {
	catalog, err := DynamicCatalog(client, true)
	if err != nil {
		return nil, err
	}
	session, cleanup, err := Session(func(server *mcp.Server) error {
		dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("list dynamic tools: %w", err)
	}
	SortDynamicTools(result.Tools)
	if contractErr := ValidateDynamicToolContract(result.Tools); contractErr != nil {
		return nil, contractErr
	}
	return result.Tools, nil
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
func Resources(client *gitlabclient.Client) ([]*mcp.Resource, []*mcp.ResourceTemplate, error) {
	dynamicCatalog, err := DynamicCatalog(client, false)
	if err != nil {
		return nil, nil, err
	}
	dynamicTools, err := DynamicTools(client)
	if err != nil {
		return nil, nil, err
	}
	session, cleanup, err := Session(func(server *mcp.Server) error {
		resources.Register(server, client)
		resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
			Surface: config.ToolSurfaceDynamic,
			Tools:   dynamicTools,
			Catalog: dynamicCatalog,
		})
		resources.RegisterWorkflowGuides(server)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()

	ctx := context.Background()
	res, err := session.ListResources(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list resources: %w", err)
	}
	tpl, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list resource templates: %w", err)
	}
	return res.Resources, tpl.ResourceTemplates, nil
}

// Prompts returns every registered MCP prompt definition over a real
// prompts/list round-trip.
func Prompts(client *gitlabclient.Client) ([]*mcp.Prompt, error) {
	session, cleanup, err := Session(func(server *mcp.Server) error {
		prompts.Register(server, client)
		return nil
	})
	if err != nil {
		return nil, err
	}
	defer cleanup()

	result, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("list prompts: %w", err)
	}
	return result.Prompts, nil
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
