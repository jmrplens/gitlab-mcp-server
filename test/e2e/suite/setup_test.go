//go:build e2e

// setup_test.go is the main E2E test infrastructure file. It provides
// [TestMain] for initializing the test environment, five in-process MCP
// server/client pairs (individual, meta, sampling, elicitation, safe-mode),
// snapshot guardrails for self-hosted mode, and shared helper functions used
// across all domain test files.

// Package suite contains end-to-end integration tests that exercise the
// MCP server tools against a real GitLab instance via in-process MCP
// client-server transport.
//
// Run with:
//
//	go test -v -tags e2e -timeout 300s ./test/e2e/suite/
package suite

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Format strings and test file constants used across E2E test helpers.
const (
	fmtCallErr       = "call %s: %w"
	testFileMainGo   = "main.go"
	msgCommitIDEmpty = "commit ID should not be empty"
	defaultBranch    = "main"
	testE2EBranch    = "feature/e2e-changes"
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
	safeMode    *mcp.ClientSession
	glClient    *gitlabclient.Client
	username    string
	enterprise  bool
	snapshot    *resourceSnapshot
}

// sess is the global read-only sessions instance populated by TestMain.
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

// TestMain initializes the E2E test environment by loading configuration,
// creating a GitLab client, verifying connectivity, and starting five
// in-process MCP server/client pairs: individual tools, meta-tools,
// sampling-enabled, elicitation-enabled, and safe-mode (mutating tools
// return previews). It populates the global [sess] struct and tears down
// servers after all tests complete.
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
		_ = godotenv.Load("../../../test/e2e/.env.docker")
		_ = godotenv.Load("../.env.docker")
	} else {
		_ = godotenv.Load("../../../.env")
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

	disableRateLimiting(glClient)

	// In Docker mode, wait for the API to handle concurrent requests reliably.
	// The readiness probe returns OK before nginx/puma workers are fully warmed
	// up, causing EOF/connection-reset under burst traffic from parallel tests.
	if isDockerMode() {
		log.Println("e2e: warming up GitLab API for concurrent load...")
		if stableErr := waitForAPIStable(glClient, 60*time.Second); stableErr != nil {
			log.Fatalf("e2e: %v", stableErr)
		}
		log.Println("e2e: API stable — proceeding with test setup")
	}

	// Auto-detect authenticated username from token.
	userInfo, userErr := glClient.CurrentUser(context.Background())
	if userErr != nil {
		log.Fatalf("e2e: auto-detect username: %v", userErr)
	}
	username := userInfo.Username
	log.Printf("e2e: authenticated as %s", username)

	// Create MCP server with all individual tools registered.
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e",
		Version: "test",
	}, nil)
	tools.RegisterAll(server, glClient, enterprise)
	resources.Register(server, glClient)
	resources.RegisterWorkflowGuides(server)

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

	// Create a fifth MCP server with Safe Mode enabled (mutating tools return previews).
	safeModeServer := mcp.NewServer(&mcp.Implementation{
		Name:    "gitlab-mcp-server-e2e-safemode",
		Version: "test",
	}, nil)
	tools.RegisterAll(safeModeServer, glClient, enterprise)
	tools.WrapMutatingToolsForSafeMode(safeModeServer)

	safeModeServerTransport, safeModeClientTransport := mcp.NewInMemoryTransports()

	safeModeServerCtx, safeModeServerCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := safeModeServer.Run(safeModeServerCtx, safeModeServerTransport); srvErr != nil && safeModeServerCtx.Err() == nil {
			log.Printf("e2e: safemode server stopped unexpectedly: %v", srvErr)
		}
	}()

	safeModeClient := mcp.NewClient(&mcp.Implementation{
		Name:    "e2e-test-safemode-client",
		Version: "test",
	}, nil)
	safeModeSession, err := safeModeClient.Connect(context.Background(), safeModeClientTransport, nil)
	if err != nil {
		serverCancel()
		metaServerCancel()
		samplingServerCancel()
		elicitServerCancel()
		safeModeServerCancel()
		log.Fatalf("e2e: connect safemode MCP client: %v", err)
	}

	sess = sessions{
		individual:  session,
		meta:        metaSession,
		sampling:    samplingSession,
		elicitation: elicitSession,
		safeMode:    safeModeSession,
		glClient:    glClient,
		username:    username,
		enterprise:  enterprise,
	}

	// Snapshot pre-existing resources in self-hosted mode.
	if !isDockerMode() {
		snap, snapErr := snapshotState(glClient)
		if snapErr != nil {
			log.Fatalf("e2e: snapshot pre-existing state: %v", snapErr)
		}
		sess.snapshot = snap
		log.Printf("e2e: snapshot captured — %d groups, %d projects", len(snap.groups), len(snap.projects))
	}

	code := m.Run()

	// Cleanup: delete any orphaned test projects (prefix-based).
	cleanupOrphanedProjects(glClient)

	// Verify snapshot integrity in self-hosted mode.
	if !isDockerMode() && sess.snapshot != nil {
		if intErr := verifySnapshotIntegrity(glClient, sess.snapshot); intErr != nil {
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
	_ = safeModeSession.Close()
	safeModeServerCancel()
	os.Exit(code)
}

// uniqueCounter provides a monotonic counter for guaranteed unique project names.
var uniqueCounter atomic.Int64

// uniqueName generates a timestamped name with an atomic counter to avoid collisions.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixMilli(), uniqueCounter.Add(1))
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
// API readiness helpers
// ---------------------------------------------------------------------------.

// waitForAPIStable sends concurrent API requests to verify GitLab handles
// parallel load without dropping connections. Returns nil when the API has
// responded successfully to all concurrent requests for the required number
// of consecutive rounds. This prevents the race where GitLab's readiness
// probe returns OK but nginx/puma workers haven't warmed up enough for the
// burst traffic generated by parallel E2E tests.
func waitForAPIStable(client *gitlabclient.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	const concurrency = 10
	const requiredSuccessRounds = 3

	successRounds := 0
	for time.Now().Before(deadline) {
		var wg sync.WaitGroup
		errs := make([]error, concurrency)
		for i := range concurrency {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, errs[idx] = client.Ping(ctx)
			}(i)
		}
		wg.Wait()

		allOK := true
		for _, e := range errs {
			if e != nil {
				allOK = false
				break
			}
		}

		if allOK {
			successRounds++
			if successRounds >= requiredSuccessRounds {
				return nil
			}
			time.Sleep(500 * time.Millisecond)
		} else {
			successRounds = 0
			log.Printf("e2e: API warmup: some connections dropped, retrying...")
			time.Sleep(2 * time.Second)
		}
	}
	return fmt.Errorf("GitLab API not stable after %v of concurrent probing", timeout)
}

// ---------------------------------------------------------------------------
// MCP call helpers
// ---------------------------------------------------------------------------.

// isTransientNetworkError returns true if the error is a transient network
// issue that can be retried (EOF, connection reset, broken pipe). These occur
// when GitLab CE is under heavy parallel load and nginx/puma drops connections.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection refused")
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

// toolCallMaxAttempts is the maximum number of attempts for a single MCP
// tool call. Transient network errors (EOF, connection reset, broken pipe,
// connection refused) trigger a retry with progressive backoff. These are
// common when GitLab CE Docker is under load right after startup.
const toolCallMaxAttempts = 4

// callToolWithRetry invokes the named MCP tool on the given session and
// retries up to [toolCallMaxAttempts] times when the error matches
// [isTransientNetworkError]. Returns the final [mcp.CallToolResult] (or nil)
// and the last error encountered.
func callToolWithRetry(ctx context.Context, session *mcp.ClientSession, name string, input any) (*mcp.CallToolResult, error) {
	var result *mcp.CallToolResult
	var err error
	for attempt := range toolCallMaxAttempts {
		result, err = session.CallTool(ctx, &mcp.CallToolParams{
			Name:      name,
			Arguments: input,
		})
		if err == nil && result != nil && result.IsError {
			err = extractToolError(name, result)
		} else if err != nil {
			err = fmt.Errorf(fmtCallErr, name, err)
		}
		if err == nil || !isTransientNetworkError(err) || attempt >= toolCallMaxAttempts-1 {
			return result, err
		}
		backoff := time.Duration(attempt+1) * 500 * time.Millisecond
		select {
		case <-ctx.Done():
			return result, err
		case <-time.After(backoff):
		}
	}
	return result, err
}

// callToolOn invokes the named MCP tool on the given session and
// unmarshals the response into output type O. It first tries
// [mcp.CallToolResult.StructuredContent], falling back to JSON-parsing
// the first [mcp.TextContent] block. Returns a zero value of O and an
// error if the call fails, the tool reports an error, or no extractable
// output is found. Transient network errors are retried transparently via
// [callToolWithRetry].
func callToolOn[O any](ctx context.Context, session *mcp.ClientSession, name string, input any) (O, error) {
	var zero O
	result, err := callToolWithRetry(ctx, session, name, input)
	if err != nil {
		return zero, err
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

// callToolVoidOn invokes the named MCP tool on the given session and
// discards the response body. Returns an error if the call fails or the
// tool reports an error via [mcp.CallToolResult.IsError]. Transient
// network errors are retried transparently via [callToolWithRetry].
func callToolVoidOn(ctx context.Context, session *mcp.ClientSession, name string, input any) error {
	_, err := callToolWithRetry(ctx, session, name, input)
	return err
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

// disableRateLimiting turns off all GitLab rate limiting via the application
// settings API. This prevents 429 errors when many parallel E2E tests hit
// the API simultaneously. Requires admin permissions; failures are non-fatal.
func disableRateLimiting(client *gitlabclient.Client) {
	falseVal := false
	_, _, err := client.GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{
		ThrottleAuthenticatedAPIEnabled:             &falseVal,
		ThrottleAuthenticatedWebEnabled:             &falseVal,
		ThrottleUnauthenticatedAPIEnabled:           &falseVal,
		ThrottleUnauthenticatedWebEnabled:           &falseVal,
		ThrottleAuthenticatedPackagesAPIEnabled:     &falseVal,
		ThrottleAuthenticatedGitLFSEnabled:          &falseVal,
		ThrottleAuthenticatedFilesAPIEnabled:        &falseVal,
		ThrottleUnauthenticatedFilesAPIEnabled:      &falseVal,
		ThrottleAuthenticatedDeprecatedAPIEnabled:   &falseVal,
		ThrottleUnauthenticatedDeprecatedAPIEnabled: &falseVal,
	})
	if err != nil {
		log.Printf("e2e: warning: could not disable rate limiting (requires admin): %v", err)
	} else {
		log.Println("e2e: rate limiting disabled for E2E test run")
	}
}

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

// cleanupOrphanedProjects permanently deletes any projects whose name starts
// with the E2E prefix. This catches orphans from previous failed runs,
// including projects already marked for delayed deletion.
func cleanupOrphanedProjects(client *gitlabclient.Client) {
	opts := &gl.ListProjectsOptions{
		Owned:                new(true),
		IncludePendingDelete: new(true),
	}
	opts.PerPage = 100
	projects, _, err := client.GL().Projects.ListProjects(opts)
	if err != nil {
		log.Printf("e2e: cleanup: failed to list projects: %v", err)
		return
	}
	for _, p := range projects {
		if strings.HasPrefix(p.Name, e2eProjectPrefix) {
			permanentlyDeleteProject(client, p)
		}
	}
}

// permanentlyDeleteProject performs a two-step permanent deletion for a single
// project. Step 1 marks the project for deletion (no-op if already marked);
// step 2 calls DeleteProject with PermanentlyRemove=true. This mirrors the
// logic in internal/tools/projects for GitLab CE delayed-deletion instances.
func permanentlyDeleteProject(client *gitlabclient.Client, p *gl.Project) {
	// Step 1: mark for deletion (may already be marked → ignore 400 errors).
	_, err := client.GL().Projects.DeleteProject(p.ID, nil)
	if err != nil {
		errMsg := err.Error()
		if !strings.Contains(errMsg, "already being deleted") && !strings.Contains(errMsg, "marked for deletion") {
			log.Printf("e2e: cleanup: failed to mark orphan %q (ID=%d) for deletion: %v", p.PathWithNamespace, p.ID, err)
		}
	}

	// Step 2: permanent removal. Re-fetch the project to get the
	// potentially updated path (GitLab appends "-deletion_scheduled-<ID>").
	updated, _, getErr := client.GL().Projects.GetProject(p.ID, nil)
	path := p.PathWithNamespace
	if getErr == nil && updated != nil {
		path = updated.PathWithNamespace
	}

	_, err = client.GL().Projects.DeleteProject(p.ID, &gl.DeleteProjectOptions{
		PermanentlyRemove: new(true),
		FullPath:          &path,
	})
	if err != nil {
		log.Printf("e2e: cleanup: failed to permanently delete orphan %q (ID=%d): %v", p.PathWithNamespace, p.ID, err)
	} else {
		log.Printf("e2e: cleanup: permanently deleted orphan project %q (ID=%d)", p.PathWithNamespace, p.ID)
	}
}

// drainSidekiq polls the GitLab Sidekiq metrics API until all background job
// queues are idle (enqueued == 0). Accelerates E2E tests by allowing async
// operations (MR merge checks, pipeline creation, commit indexing) to complete
// before assertions. No-op if the API is unavailable or context is done.
func drainSidekiq(ctx context.Context, t *testing.T) {
	t.Helper()
	if sess.glClient == nil {
		return
	}
	const maxWait = 15 * time.Second
	const pollInterval = 250 * time.Millisecond

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		stats, _, err := sess.glClient.GL().Sidekiq.GetJobStats()
		if err != nil {
			return
		}
		if stats.Jobs.Enqueued == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

// waitForPipeline polls the GitLab API until the pipeline reaches a terminal
// state (success, failed, canceled, skipped) or the timeout expires. To be
// resilient against slow CI runners in ephemeral Docker environments, this
// helper:
//   - Uses a generous default timeout (15 min).
//   - Polls every 5 s.
//   - Tolerates transient API errors (logs and retries up to 10 consecutive
//     errors before giving up).
//   - Logs the final status before failing so test output makes the runner
//     state easy to diagnose.
func waitForPipeline(t *testing.T, projectID int64, pipelineID int64, timeout time.Duration) string {
	t.Helper()
	drainSidekiq(context.Background(), t)
	if timeout == 0 {
		timeout = 900 * time.Second
	}
	const pollInterval = 5 * time.Second
	const maxConsecutiveErrors = 10
	deadline := time.Now().Add(timeout)
	lastStatus := "unknown"
	consecutiveErrors := 0
	for time.Now().Before(deadline) {
		p, _, err := sess.glClient.GL().Pipelines.GetPipeline(projectID, pipelineID)
		if err != nil {
			consecutiveErrors++
			t.Logf("waitForPipeline: error polling pipeline %d (%d/%d): %v", pipelineID, consecutiveErrors, maxConsecutiveErrors, err)
			if consecutiveErrors >= maxConsecutiveErrors {
				t.Fatalf("waitForPipeline: pipeline %d aborted after %d consecutive API errors: %v", pipelineID, consecutiveErrors, err)
			}
			time.Sleep(pollInterval)
			continue
		}
		consecutiveErrors = 0
		lastStatus = p.Status
		switch p.Status {
		case "success", "failed", "canceled", "skipped":
			t.Logf("waitForPipeline: pipeline %d reached terminal status: %s", pipelineID, p.Status)
			return p.Status
		}
		t.Logf("waitForPipeline: pipeline %d status=%s, waiting...", pipelineID, p.Status)
		time.Sleep(pollInterval)
	}
	t.Fatalf("waitForPipeline: pipeline %d did not reach terminal status within %s (last status: %s)", pipelineID, timeout, lastStatus)
	return ""
}

// hasRunner returns true if a CI runner is available for pipeline tests.
// In Docker mode it always returns true; in self-hosted mode it checks the
// Runners API for registered instance runners.
func hasRunner() bool {
	if isDockerMode() {
		return true
	}
	runnerType := "instance_type"
	runners, _, err := sess.glClient.GL().Runners.ListRunners(&gl.ListRunnersOptions{
		Type: &runnerType,
	})
	return err == nil && len(runners) > 0
}

// requirePremiumFeature fails the test if the error indicates the feature
// requires a premium/ultimate license or admin permissions. Enterprise tests
// are gated at skip level so they only run when the GitLab instance supports them.
func requirePremiumFeature(t *testing.T, err error, feature string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s failed: %v", feature, err)
	}
}

// int64Ptr returns a pointer to v. Used for optional int64 fields in tool inputs.
//
//go:fix inline
func int64Ptr(v int64) *int64 { return new(v) }
