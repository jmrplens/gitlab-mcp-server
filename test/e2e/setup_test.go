//go:build e2e

// Package e2e contains end-to-end tests that exercise the MCP server tools
// against a real GitLab instance via the in-process MCP client-server loop.
// Run with: go test -v -tags e2e -timeout 120s ./test/e2e/.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Format strings and test file constants used across E2E test helpers.
const (
	fmtCallErr       = "call %s: %w"
	testFileMainGo   = "main.go"
	msgCommitIDEmpty = "commit ID should not be empty"
)

// e2eProjectPrefix is the required prefix for all projects created by E2E
// tests. Configurable via E2E_PROJECT_PREFIX env var.
var e2eProjectPrefix = "e2e-"

// sessions holds read-only MCP sessions and infrastructure created once in
// TestMain. Domain test files access these but never mutate them — all
// mutable test state is kept in local variables within each test function.
type sessions struct {
	individual  *mcp.ClientSession
	meta        *mcp.ClientSession
	sampling    *mcp.ClientSession
	elicitation *mcp.ClientSession
	glClient    *gitlabclient.Client
	enterprise  bool
	snapshot    *resourceSnapshot
}

// sess is the global read-only sessions instance populated by TestMain.
// New domain test files should use sess instead of the legacy state global.
var sess sessions

// isDockerMode returns true when running against an ephemeral Docker GitLab
// instance (E2E_MODE=docker). In Docker mode, snapshot guardrails are skipped
// because the entire instance is disposable.
func isDockerMode() bool {
	return strings.EqualFold(os.Getenv("E2E_MODE"), "docker")
}

// resourceSnapshot stores the state of pre-existing resources captured at
// startup in self-hosted mode. Used to verify E2E tests don't modify or
// delete resources they don't own.
type resourceSnapshot struct {
	groups   map[int64]string // ID → full_path
	projects map[int64]string // ID → path_with_namespace
}

// testState holds shared state across sequential test steps.
type testState struct {
	glClient              *gitlabclient.Client
	enterprise            bool // true when GITLAB_ENTERPRISE=true
	session               *mcp.ClientSession
	metaSession           *mcp.ClientSession // meta-tools session
	samplingSession       *mcp.ClientSession // sampling-enabled session (mock LLM)
	elicitSession         *mcp.ClientSession // elicitation-enabled session (auto-accept mock)
	snapshot              *resourceSnapshot  // pre-existing resources (self-hosted mode only)
	projectID             int64
	projectPath           string
	mrIID                 int64
	noteID                int64
	discussionID          string
	releaseLinkID         int64
	lastCommitSHA         string // SHA from most recent commit (for commit get/diff tests)
	issueIID              int64  // issue IID for issue lifecycle tests
	issueNoteID           int64  // issue note ID for issue note tests
	groupID               int64  // group ID discovered via group_list (0 if none)
	groupPath             string // group full path discovered via group_list
	packageID             int64  // package ID for package lifecycle tests
	packageFileID         int64  // package file ID for package file tests
	wikiSlug              string // wiki page slug for wiki lifecycle tests
	envID                 int64  // environment ID for environment lifecycle tests
	labelID               int64  // label ID for label lifecycle tests
	milestoneIID          int64  // milestone IID for milestone lifecycle tests
	draftNoteID           int64  // draft note ID for MR draft note tests
	issue2IID             int64  // second issue IID for issue link tests
	deployKeyID           int64  // deploy key ID for deploy key lifecycle tests
	snippetID             int64  // project snippet ID for snippet lifecycle tests
	issueDiscussionID     string // issue discussion ID for issue discussion tests
	issueDiscussionNoteID int64  // note ID within issue discussion
	pipelineScheduleID    int    // pipeline schedule ID for schedule lifecycle tests
	badgeID               int64  // project badge ID for badge lifecycle tests
	accessTokenID         int64  // project access token ID for token lifecycle tests
	awardEmojiID          int64  // award emoji ID for emoji lifecycle tests
	pipelineID            int64  // pipeline ID for pipeline lifecycle tests (Docker mode)
	jobID                 int64  // job ID for job lifecycle tests (Docker mode)
}

// state is the shared [testState] instance populated by [TestMain] and
// used by all sequential test steps across both workflow files.
var state testState

// TestMain initializes the E2E test environment by loading configuration,
// creating a GitLab client, verifying connectivity, and starting two
// in-process MCP server/client pairs: one for individual tools and one
// for meta-tools. It populates the global [state] and tears down servers
// after all tests complete.
//
// In self-hosted mode, it snapshots all pre-existing groups and projects
// before running tests, and verifies they remain unchanged after tests
// complete. In Docker mode (E2E_MODE=docker), snapshots are skipped.
func TestMain(m *testing.M) {
	// Allow overriding the project prefix.
	if p := os.Getenv("E2E_PROJECT_PREFIX"); p != "" {
		e2eProjectPrefix = p
	}

	// Load .env — Docker mode uses a different file.
	if isDockerMode() {
		_ = godotenv.Load("../../test/e2e/.env.docker")
		_ = godotenv.Load(".env.docker")
	} else {
		_ = godotenv.Load("../../.env")
	}

	enterprise := strings.EqualFold(os.Getenv("GITLAB_ENTERPRISE"), "true")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("e2e: load config: %v", err)
	}

	glClient, err := gitlabclient.NewClient(cfg)
	if err != nil {
		log.Fatalf("e2e: create GitLab client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = glClient.Ping(ctx); err != nil {
		log.Fatalf("e2e: gitlab ping failed: %v", err)
	}

	// Create MCP server with all individual tools registered.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e",
		Version: "test",
	}, nil)
	tools.RegisterAll(server, glClient, enterprise)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverCtx, serverCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := server.Run(serverCtx, serverTransport); srvErr != nil && serverCtx.Err() == nil {
			log.Printf("e2e: server stopped unexpectedly: %v", srvErr)
		}
	}()

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-client",
		Version: "test",
	}, nil)
	session, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		serverCancel()
		log.Fatalf("e2e: connect MCP client: %v", err)
	}

	// Create a second MCP server with meta-tools for meta-tool E2E tests.
	metaServer := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-meta",
		Version: "test",
	}, nil)
	tools.RegisterAllMeta(metaServer, glClient, enterprise)

	metaServerTransport, metaClientTransport := mcp.NewInMemoryTransports()

	metaServerCtx, metaServerCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := metaServer.Run(metaServerCtx, metaServerTransport); srvErr != nil && metaServerCtx.Err() == nil {
			log.Printf("e2e: meta server stopped unexpectedly: %v", srvErr)
		}
	}()

	metaClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-meta-client",
		Version: "test",
	}, nil)
	metaSession, err := metaClient.Connect(context.Background(), metaClientTransport, nil)
	if err != nil {
		serverCancel()
		metaServerCancel()
		log.Fatalf("e2e: connect meta MCP client: %v", err)
	}

	// Create a third MCP server/client pair with sampling capability (mock LLM).
	samplingServer := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-sampling",
		Version: "test",
	}, nil)
	tools.RegisterAll(samplingServer, glClient, enterprise)

	samplingServerTransport, samplingClientTransport := mcp.NewInMemoryTransports()

	samplingServerCtx, samplingServerCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := samplingServer.Run(samplingServerCtx, samplingServerTransport); srvErr != nil && samplingServerCtx.Err() == nil {
			log.Printf("e2e: sampling server stopped unexpectedly: %v", srvErr)
		}
	}()

	samplingClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-sampling-client",
		Version: "test",
	}, &mcp.ClientOptions{
		CreateMessageHandler: mockCreateMessageHandler,
	})
	samplingSession, err := samplingClient.Connect(context.Background(), samplingClientTransport, nil)
	if err != nil {
		serverCancel()
		metaServerCancel()
		samplingServerCancel()
		log.Fatalf("e2e: connect sampling MCP client: %v", err)
	}

	// Create a fourth MCP server/client pair with elicitation capability (auto-accept mock).
	elicitServer := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-elicit",
		Version: "test",
	}, nil)
	tools.RegisterAll(elicitServer, glClient, enterprise)

	elicitServerTransport, elicitClientTransport := mcp.NewInMemoryTransports()

	elicitServerCtx, elicitServerCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := elicitServer.Run(elicitServerCtx, elicitServerTransport); srvErr != nil && elicitServerCtx.Err() == nil {
			log.Printf("e2e: elicit server stopped unexpectedly: %v", srvErr)
		}
	}()

	elicitClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-elicit-client",
		Version: "test",
	}, &mcp.ClientOptions{
		ElicitationHandler: mockElicitHandler,
	})
	elicitSession, err := elicitClient.Connect(context.Background(), elicitClientTransport, nil)
	if err != nil {
		serverCancel()
		metaServerCancel()
		samplingServerCancel()
		elicitServerCancel()
		log.Fatalf("e2e: connect elicit MCP client: %v", err)
	}

	state = testState{
		glClient:        glClient,
		enterprise:      enterprise,
		session:         session,
		metaSession:     metaSession,
		samplingSession: samplingSession,
		elicitSession:   elicitSession,
	}

	// Populate the new read-only sessions struct for domain test files.
	sess = sessions{
		individual:  session,
		meta:        metaSession,
		sampling:    samplingSession,
		elicitation: elicitSession,
		glClient:    glClient,
		enterprise:  enterprise,
	}

	// Snapshot pre-existing resources in self-hosted mode.
	if !isDockerMode() {
		snap, snapErr := snapshotState(glClient)
		if snapErr != nil {
			log.Fatalf("e2e: snapshot pre-existing state: %v", snapErr)
		}
		state.snapshot = snap
		sess.snapshot = snap
		log.Printf("e2e: snapshot captured — %d groups, %d projects", len(snap.groups), len(snap.projects))
	}

	code := m.Run()

	// Cleanup: delete any orphaned test projects (prefix-based).
	cleanupOrphanedProjects(glClient)

	// Verify snapshot integrity in self-hosted mode.
	if !isDockerMode() && state.snapshot != nil {
		if intErr := verifySnapshotIntegrity(glClient, state.snapshot); intErr != nil {
			log.Printf("e2e: SNAPSHOT INTEGRITY FAILURE: %v", intErr)
			if code == 0 {
				code = 1
			}
		} else {
			log.Println("e2e: snapshot integrity verified — all pre-existing resources unchanged")
		}
	}

	_ = session.Close()
	serverCancel()
	_ = metaSession.Close()
	metaServerCancel()
	_ = samplingSession.Close()
	samplingServerCancel()
	_ = elicitSession.Close()
	elicitServerCancel()
	os.Exit(code)
}

// uniqueName generates a timestamped name to avoid collisions.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixMilli())
}

// mockCreateMessageHandler returns a deterministic mock LLM response for
// sampling E2E tests. It validates that the tool gathered data correctly
// and produces a recognizable output without requiring an actual LLM.
func mockCreateMessageHandler(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	return &mcp.CreateMessageResult{
		Content: &mcp.TextContent{Text: "## Mock Analysis\n\nThis is a mock analysis generated by the E2E test sampling handler."},
		Model:   "e2e-mock-model",
		Role:    "assistant",
	}, nil
}

// mockElicitHandler auto-accepts every elicitation request with plausible
// values derived from the requested JSON schema. It handles "confirmed"
// (bool), "selection" (enum), and text fields (string) by inspecting the
// schema properties.
func mockElicitHandler(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	content := make(map[string]any)

	schema, ok := req.Params.RequestedSchema.(map[string]any)
	if ok {
		if props, pOk := schema["properties"].(map[string]any); pOk {
			for key, val := range props {
				prop, propOk := val.(map[string]any)
				if !propOk {
					continue
				}
				switch key {
				case "confirmed":
					content[key] = true
				case "selection":
					if enumVals, eOk := prop["enum"].([]any); eOk && len(enumVals) > 0 {
						content[key] = enumVals[0]
					} else {
						content[key] = "default"
					}
				default:
					content[key] = elicitTextValue(key)
				}
			}
		}
	}

	return &mcp.ElicitResult{Action: "accept", Content: content}, nil
}

// elicitTextValue returns a plausible mock value for a text field based on
// its name. Elicitation tools use field names like "title", "description",
// "source_branch", "target_branch", "tag_name", "name", "default_branch".
func elicitTextValue(fieldName string) string {
	defaults := map[string]string{
		"title":          "E2E elicitation test",
		"description":    "Created by E2E elicitation mock handler",
		"name":           "e2e-elicit-resource",
		"source_branch":  testE2EBranch,
		"target_branch":  "main",
		"tag_name":       "v99.0.0-elicit",
		"labels":         "e2e-test",
		"default_branch": "main",
	}
	if v, ok := defaults[fieldName]; ok {
		return v
	}
	return "e2e-mock-" + fieldName
}

// ---------------------------------------------------------------------------
// MCP call helpers
// ---------------------------------------------------------------------------.

// callSamplingTool invokes an MCP tool via the sampling-enabled session.
func callSamplingTool[O any](ctx context.Context, name string, input any) (O, error) {
	var zero O
	result, err := state.samplingSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return zero, fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return zero, extractToolError(name, result)
	}
	if result.StructuredContent != nil {
		var data []byte
		data, err = json.Marshal(result.StructuredContent)
		if err != nil {
			return zero, fmt.Errorf("unmarshal structured content for %s: %w", name, err)
		}
		var out O
		if umErr := json.Unmarshal(data, &out); umErr != nil {
			return zero, fmt.Errorf("decode structured content for %s: %w", name, umErr)
		}
		return out, nil
	}
	return zero, fmt.Errorf("no structured content in %s response", name)
}

// callElicitTool invokes an MCP tool via the elicitation-enabled session.
func callElicitTool[O any](ctx context.Context, name string, input any) (O, error) {
	var zero O
	result, err := state.elicitSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return zero, fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return zero, extractToolError(name, result)
	}
	if result.StructuredContent != nil {
		var data []byte
		data, err = json.Marshal(result.StructuredContent)
		if err != nil {
			return zero, fmt.Errorf("unmarshal structured content for %s: %w", name, err)
		}
		var out O
		if umErr := json.Unmarshal(data, &out); umErr != nil {
			return zero, fmt.Errorf("decode structured content for %s: %w", name, umErr)
		}
		return out, nil
	}
	return zero, fmt.Errorf("no structured content in %s response", name)
}

// callTool invokes an MCP tool via the client session and unmarshals the
// structured result into the output type O.
func callTool[O any](ctx context.Context, name string, input any) (O, error) {
	var zero O
	result, err := state.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return zero, fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return zero, extractToolError(name, result)
	}

	// Prefer structured content (typed ToolHandlerFor output).
	if result.StructuredContent != nil {
		var data []byte
		data, err = json.Marshal(result.StructuredContent)
		if err != nil {
			return zero, fmt.Errorf("marshal structured content: %w", err)
		}
		var out O
		err = json.Unmarshal(data, &out)
		if err != nil {
			return zero, fmt.Errorf("unmarshal %s result to %T: %w", name, out, err)
		}
		return out, nil
	}

	// Fallback: extract JSON from the first text content block.
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			var out O
			err = json.Unmarshal([]byte(tc.Text), &out)
			if err != nil {
				return zero, fmt.Errorf("unmarshal %s text to %T: %w", name, out, err)
			}
			return out, nil
		}
	}

	return zero, fmt.Errorf("tool %s: no extractable output", name)
}

// callToolVoid invokes an MCP tool that returns no structured output (e.g. delete, unapprove).
func callToolVoid(ctx context.Context, name string, input any) error {
	result, err := state.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return extractToolError(name, result)
	}
	return nil
}

// extractToolError reads the first text content block from a failed
// [mcp.CallToolResult] and returns it as a formatted error.
func extractToolError(name string, result *mcp.CallToolResult) error {
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			return fmt.Errorf("tool %s: %s", name, tc.Text)
		}
	}
	return fmt.Errorf("tool %s returned error", name)
}

// ---------------------------------------------------------------------------
// Meta-tool call helpers (use metaSession)
// ---------------------------------------------------------------------------.

// callMeta invokes a meta-tool via the meta session and unmarshals the output.
func callMeta[O any](ctx context.Context, metaTool, action string, params map[string]any) (O, error) {
	return callToolOn[O](ctx, state.metaSession, metaTool, map[string]any{
		"action": action,
		"params": params,
	})
}

// callMetaVoid invokes a meta-tool action that returns no structured output.
func callMetaVoid(ctx context.Context, metaTool, action string, params map[string]any) error {
	return callToolVoidOn(ctx, state.metaSession, metaTool, map[string]any{
		"action": action,
		"params": params,
	})
}

// callToolOn is a session-parameterized version of callTool.
func callToolOn[O any](ctx context.Context, session *mcp.ClientSession, name string, input any) (O, error) {
	var zero O
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return zero, fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return zero, extractToolError(name, result)
	}
	if result.StructuredContent != nil {
		var data []byte
		data, err = json.Marshal(result.StructuredContent)
		if err != nil {
			return zero, fmt.Errorf("marshal structured content: %w", err)
		}
		var out O
		err = json.Unmarshal(data, &out)
		if err != nil {
			return zero, fmt.Errorf("unmarshal %s result to %T: %w", name, out, err)
		}
		return out, nil
	}
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(*mcp.TextContent); ok {
			var out O
			err = json.Unmarshal([]byte(tc.Text), &out)
			if err != nil {
				return zero, fmt.Errorf("unmarshal %s text to %T: %w", name, out, err)
			}
			return out, nil
		}
	}
	return zero, fmt.Errorf("tool %s: no extractable output", name)
}

// callToolVoidOn is a session-parameterized version of callToolVoid.
func callToolVoidOn(ctx context.Context, session *mcp.ClientSession, name string, input any) error {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: input,
	})
	if err != nil {
		return fmt.Errorf(fmtCallErr, name, err)
	}
	if result.IsError {
		return extractToolError(name, result)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Wait helpers
// ---------------------------------------------------------------------------.

// ---------------------------------------------------------------------------
// Test assertion helpers
// ---------------------------------------------------------------------------.

// requireNoError calls t.Fatalf if err is non-nil, including the action
// label in the failure message.
func requireNoError(t *testing.T, err error, action string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed: %v", action, err)
	}
}

// requireTrue calls t.Fatalf with the given format string if condition
// is false.
func requireTrue(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()
	if !condition {
		t.Fatalf(format, args...)
	}
}

// ---------------------------------------------------------------------------
// Snapshot guardrails (self-hosted mode only)
// ---------------------------------------------------------------------------.

// snapshotState queries GitLab for all groups and projects visible to the
// authenticated user and returns a resourceSnapshot. Used in self-hosted mode
// to detect if E2E tests accidentally modify resources they don't own.
func snapshotState(client *gitlabclient.Client) (*resourceSnapshot, error) {
	snap := &resourceSnapshot{
		groups:   make(map[int64]string),
		projects: make(map[int64]string),
	}

	// Fetch all groups (paginated).
	var groupPage int64 = 1
	for {
		opts := &gl.ListGroupsOptions{}
		opts.Page = groupPage
		opts.PerPage = 100
		groups, resp, err := client.GL().Groups.ListGroups(opts)
		if err != nil {
			return nil, fmt.Errorf("list groups (page %d): %w", groupPage, err)
		}
		for _, g := range groups {
			snap.groups[g.ID] = g.FullPath
		}
		if resp.NextPage == 0 {
			break
		}
		groupPage = resp.NextPage
	}

	// Fetch all projects (paginated).
	var projectPage int64 = 1
	for {
		opts := &gl.ListProjectsOptions{}
		opts.Page = projectPage
		opts.PerPage = 100
		projs, resp, err := client.GL().Projects.ListProjects(opts)
		if err != nil {
			return nil, fmt.Errorf("list projects (page %d): %w", projectPage, err)
		}
		for _, p := range projs {
			snap.projects[p.ID] = p.PathWithNamespace
		}
		if resp.NextPage == 0 {
			break
		}
		projectPage = resp.NextPage
	}

	return snap, nil
}

// verifySnapshotIntegrity re-fetches all groups and projects and compares
// them against the initial snapshot. Returns an error if any pre-existing
// resource was deleted or renamed.
func verifySnapshotIntegrity(client *gitlabclient.Client, snap *resourceSnapshot) error {
	var missing []string

	// Check groups still exist with same path.
	for id, origPath := range snap.groups {
		g, _, err := client.GL().Groups.GetGroup(int(id), &gl.GetGroupOptions{})
		if err != nil {
			missing = append(missing, fmt.Sprintf("group %q (ID=%d): %v", origPath, id, err))
			continue
		}
		if g.FullPath != origPath {
			missing = append(missing, fmt.Sprintf("group ID=%d renamed: %q → %q", id, origPath, g.FullPath))
		}
	}

	// Check projects still exist with same path.
	for id, origPath := range snap.projects {
		p, _, err := client.GL().Projects.GetProject(int(id), &gl.GetProjectOptions{})
		if err != nil {
			missing = append(missing, fmt.Sprintf("project %q (ID=%d): %v", origPath, id, err))
			continue
		}
		if p.PathWithNamespace != origPath {
			missing = append(missing, fmt.Sprintf("project ID=%d renamed: %q → %q", id, origPath, p.PathWithNamespace))
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("%d pre-existing resources were modified or deleted:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	return nil
}

// cleanupOrphanedProjects deletes any projects whose name starts with the
// E2E prefix. This catches orphans from previous failed runs.
func cleanupOrphanedProjects(client *gitlabclient.Client) {
	opts := &gl.ListProjectsOptions{
		Owned: new(true),
	}
	opts.PerPage = 100
	projects, _, err := client.GL().Projects.ListProjects(opts)
	if err != nil {
		log.Printf("e2e: cleanup: failed to list projects: %v", err)
		return
	}
	for _, p := range projects {
		if strings.HasPrefix(p.Name, e2eProjectPrefix) {
			_, err = client.GL().Projects.DeleteProject(p.ID, nil)
			if err != nil {
				log.Printf("e2e: cleanup: failed to delete orphan %q (ID=%d): %v", p.PathWithNamespace, p.ID, err)
			} else {
				log.Printf("e2e: cleanup: deleted orphan project %q (ID=%d)", p.PathWithNamespace, p.ID)
			}
		}
	}
}

// waitForPipeline polls the GitLab API until the pipeline reaches a terminal
// state (success, failed, canceled, skipped) or the timeout expires.
func waitForPipeline(t *testing.T, projectID int64, pipelineID int64, timeout time.Duration) string {
	t.Helper()
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p, _, err := state.glClient.GL().Pipelines.GetPipeline(projectID, pipelineID)
		if err != nil {
			t.Logf("waitForPipeline: error polling pipeline %d: %v", pipelineID, err)
			time.Sleep(5 * time.Second)
			continue
		}
		switch p.Status {
		case "success", "failed", "canceled", "skipped":
			t.Logf("waitForPipeline: pipeline %d reached terminal status: %s", pipelineID, p.Status)
			return p.Status
		}
		t.Logf("waitForPipeline: pipeline %d status=%s, waiting...", pipelineID, p.Status)
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("waitForPipeline: pipeline %d did not reach terminal status within %s", pipelineID, timeout)
	return ""
}
