//go:build e2e

package suite

// setup_test.go contains the main E2E test infrastructure: [TestMain], six
// in-process MCP server/client pairs, snapshot guardrails for self-hosted mode,
// and shared helpers used across all domain test files.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	dynamic     *mcp.ClientSession
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
// creating a GitLab client, verifying connectivity, and starting six
// in-process MCP server/client pairs: individual tools, meta-tools,
// dynamic tools, sampling-enabled, elicitation-enabled, and safe-mode
// (mutating tools return previews). It populates the global [sess] struct and
// tears down servers after all tests complete.
//
// In self-hosted mode, it snapshots all pre-existing groups and projects
// before running tests, and verifies they remain unchanged after tests
// complete. In Docker mode (E2E_MODE=docker), snapshots are skipped.
func TestMain(m *testing.M) {
	configureE2EStartupEnvironment()

	enterprise := strings.EqualFold(os.Getenv("GITLAB_ENTERPRISE"), "true")
	glClient := mustE2EGitLabClient()
	username := mustE2EUsername(glClient)
	runtime := startE2ERuntime(glClient, enterprise)
	sess = runtime.sessions
	sess.username = username
	sess.glClient = glClient
	sess.enterprise = enterprise
	captureE2ESnapshot(glClient)

	code := m.Run()
	code = verifyE2ESnapshotAfterRun(glClient, code)
	cleanupOrphanedProjects(glClient)
	closeE2ESessions(runtime.running)
	os.Exit(code)
}

type runningE2ESession struct {
	session *mcp.ClientSession
	cancel  context.CancelFunc
}

type e2eRuntime struct {
	sessions sessions
	running  []runningE2ESession
}

func startE2ERuntime(glClient *gitlabclient.Client, enterprise bool) e2eRuntime {
	runtime := e2eRuntime{}
	runtime.sessions.individual = mustStartE2ESession(&runtime, "individual", "gitlab-mcp-server-e2e", "e2e-test-client", nil, configureIndividualE2EServer(glClient, enterprise))
	runtime.sessions.meta = mustStartE2ESession(&runtime, "meta", "gitlab-mcp-server-e2e-meta", "e2e-test-meta-client", nil, configureMetaE2EServer(glClient, enterprise))
	runtime.sessions.dynamic = mustStartE2ESession(&runtime, "dynamic", "gitlab-mcp-server-e2e-dynamic", "e2e-test-dynamic-client", nil, configureDynamicE2EServer(glClient, enterprise))
	runtime.sessions.sampling = mustStartE2ESession(&runtime, "sampling", "gitlab-mcp-server-e2e-sampling", "e2e-test-sampling-client", samplingClientOptions(), configureToolOnlyE2EServer(glClient, enterprise))
	runtime.sessions.elicitation = mustStartE2ESession(&runtime, "elicitation", "gitlab-mcp-server-e2e-elicit", "e2e-test-elicit-client", elicitationClientOptions(), configureToolOnlyE2EServer(glClient, enterprise))
	runtime.sessions.safeMode = mustStartE2ESession(&runtime, "safemode", "gitlab-mcp-server-e2e-safemode", "e2e-test-safemode-client", nil, configureSafeModeE2EServer(glClient, enterprise))
	return runtime
}

func mustStartE2ESession(runtime *e2eRuntime, label, serverName, clientName string, clientOptions *mcp.ClientOptions, configure func(*mcp.Server) error) *mcp.ClientSession {
	running, err := startE2ESession(serverName, clientName, clientOptions, configure)
	if err != nil {
		closeE2ESessions(runtime.running)
		log.Fatalf("e2e: %s session: %v", label, err)
	}
	runtime.running = append(runtime.running, running)
	return running.session
}

func startE2ESession(serverName, clientName string, clientOptions *mcp.ClientOptions, configure func(*mcp.Server) error) (runningE2ESession, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "test"}, nil)
	if err := configure(server); err != nil {
		return runningE2ESession{}, err
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverCtx, serverCancel := context.WithCancel(context.Background())
	go func() {
		if srvErr := server.Run(serverCtx, serverTransport); srvErr != nil && serverCtx.Err() == nil {
			log.Printf("e2e: %s server stopped unexpectedly: %v", serverName, srvErr)
		}
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "test"}, clientOptions)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		serverCancel()
		return runningE2ESession{}, fmt.Errorf("connect %s MCP client: %w", clientName, err)
	}
	return runningE2ESession{session: session, cancel: serverCancel}, nil
}

func configureIndividualE2EServer(glClient *gitlabclient.Client, enterprise bool) func(*mcp.Server) error {
	return func(server *mcp.Server) error {
		tools.RegisterAll(server, glClient, enterprise)
		resources.Register(server, glClient)
		resources.RegisterWorkflowGuides(server)
		return nil
	}
}

func configureMetaE2EServer(glClient *gitlabclient.Client, enterprise bool) func(*mcp.Server) error {
	return func(server *mcp.Server) error {
		return tools.RegisterAllMeta(server, glClient, enterprise)
	}
}

func configureDynamicE2EServer(glClient *gitlabclient.Client, enterprise bool) func(*mcp.Server) error {
	return func(server *mcp.Server) error {
		catalog, err := tools.BuildActionCatalog(glClient, tools.ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
		if err != nil {
			return fmt.Errorf("build dynamic action catalog: %w", err)
		}
		catalog, err = dynamictools.AddStandaloneCatalog(catalog, glClient, dynamictools.StandaloneOptions{})
		if err != nil {
			return fmt.Errorf("add standalone dynamic catalog: %w", err)
		}
		dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
		return nil
	}
}

func configureToolOnlyE2EServer(glClient *gitlabclient.Client, enterprise bool) func(*mcp.Server) error {
	return func(server *mcp.Server) error {
		tools.RegisterAll(server, glClient, enterprise)
		return nil
	}
}

func configureSafeModeE2EServer(glClient *gitlabclient.Client, enterprise bool) func(*mcp.Server) error {
	return func(server *mcp.Server) error {
		tools.RegisterAll(server, glClient, enterprise)
		tools.WrapMutatingToolsForSafeMode(server)
		return nil
	}
}

func samplingClientOptions() *mcp.ClientOptions {
	return &mcp.ClientOptions{CreateMessageHandler: mockCreateMessageHandler}
}

func elicitationClientOptions() *mcp.ClientOptions {
	return &mcp.ClientOptions{ElicitationHandler: mockElicitHandler}
}

func captureE2ESnapshot(glClient *gitlabclient.Client) {
	if isDockerMode() {
		return
	}
	snap, err := snapshotState(glClient)
	if err != nil {
		log.Fatalf("e2e: snapshot pre-existing state: %v", err)
	}
	sess.snapshot = snap
	log.Printf("e2e: snapshot captured — %d groups, %d projects", len(snap.groups), len(snap.projects))
}

func verifyE2ESnapshotAfterRun(glClient *gitlabclient.Client, code int) int {
	if isDockerMode() || sess.snapshot == nil {
		return code
	}
	if err := verifySnapshotIntegrity(glClient, sess.snapshot); err != nil {
		log.Printf("e2e: SNAPSHOT INTEGRITY FAILURE: %v", err)
		if code == 0 {
			return 1
		}
		return code
	}
	log.Println("e2e: snapshot integrity verified — all pre-existing resources unchanged")
	return code
}

func closeE2ESessions(running []runningE2ESession) {
	for _, runningSession := range slices.Backward(running) {
		_ = runningSession.session.Close()
		runningSession.cancel()
	}
}

func configureE2EStartupEnvironment() {
	if p := os.Getenv("E2E_PROJECT_PREFIX"); p != "" {
		e2eProjectPrefix = p
	}
	e2eRunID = configuredE2ERunID(time.Now())
	log.Printf("e2e: run ID %s", e2eRunID)
	if isDockerMode() {
		_ = godotenv.Load("../../../test/e2e/.env.docker")
		_ = godotenv.Load("../.env.docker")
		return
	}
	_ = godotenv.Load("../../../.env")
}

func mustE2EGitLabClient() *gitlabclient.Client {
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
	if isDockerMode() {
		log.Println("e2e: warming up GitLab API for concurrent load...")
		if stableErr := waitForAPIStable(glClient, 60*time.Second); stableErr != nil {
			log.Fatalf("e2e: %v", stableErr)
		}
		log.Println("e2e: API stable — proceeding with test setup")
	}
	return glClient
}

func mustE2EUsername(glClient *gitlabclient.Client) string {
	userInfo, userErr := currentUserWithRetry(glClient, 60*time.Second)
	if userErr != nil {
		log.Fatalf("e2e: auto-detect username: %v", userErr)
	}
	log.Printf("e2e: authenticated as %s", userInfo.Username)
	return userInfo.Username
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
	content := mockElicitContent(req)
	return &mcp.ElicitResult{Action: "accept", Content: content}, nil
}

func mockElicitContent(req *mcp.ElicitRequest) map[string]any {
	content := make(map[string]any)
	schema, ok := req.Params.RequestedSchema.(map[string]any)
	if !ok {
		return content
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return content
	}
	for key, val := range props {
		prop, propOk := val.(map[string]any)
		if !propOk {
			continue
		}
		content[key] = mockElicitPropertyValue(key, prop)
	}
	return content
}

func mockElicitPropertyValue(key string, prop map[string]any) any {
	switch key {
	case "confirmed":
		return true
	case "selection":
		if enumVals, ok := prop["enum"].([]any); ok && len(enumVals) > 0 {
			return enumVals[0]
		}
		return "default"
	default:
		return elicitTextValue(key)
	}
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

// currentUserWithRetry waits for the first authenticated GitLab API endpoint
// used by E2E setup to become stable after Docker GitLab reports readiness.
func currentUserWithRetry(client *gitlabclient.Client, timeout time.Duration) (*gitlabclient.CurrentUserInfo, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for attempt := 1; time.Now().Before(deadline); attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		userInfo, err := client.CurrentUser(ctx)
		cancel()
		if err == nil {
			return userInfo, nil
		}
		lastErr = err
		if !isTransientNetworkError(err) {
			return nil, err
		}
		log.Printf("e2e: current user lookup hit transient GitLab error (attempt %d): %v", attempt, err)
		backoff := time.Duration(attempt) * 500 * time.Millisecond
		backoff = min(backoff, 2*time.Second)
		if remaining := time.Until(deadline); backoff > remaining {
			backoff = remaining
		}
		if backoff > 0 {
			time.Sleep(backoff)
		}
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return nil, fmt.Errorf("current user endpoint not stable after %v: %w", timeout, lastErr)
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

// requireTruef calls t.Fatalf with the given format string if condition
// is false.
func requireTruef(t *testing.T, condition bool, format string, args ...any) {
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

// verifySnapshotIntegrity re-snapshots groups and projects and compares them
// against the initial snapshot. Returns an error if any pre-existing resource
// was deleted or renamed.
func verifySnapshotIntegrity(client *gitlabclient.Client, snap *resourceSnapshot) error {
	current, err := snapshotState(client)
	if err != nil {
		return fmt.Errorf("snapshot current state: %w", err)
	}

	missing := snapshotIntegrityDifferences(snap, current)
	if len(missing) > 0 {
		return fmt.Errorf("%d pre-existing resources were modified or deleted:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	return nil
}

// snapshotIntegrityDifferences returns descriptions of pre-existing groups or
// projects that disappeared or changed path during the E2E run.
func snapshotIntegrityDifferences(original, current *resourceSnapshot) []string {
	var missing []string

	// Check groups still exist with same path.
	for id, origPath := range original.groups {
		currentPath, ok := current.groups[id]
		if !ok {
			missing = append(missing, fmt.Sprintf("group %q (ID=%d): missing", origPath, id))
			continue
		}
		if currentPath != origPath {
			missing = append(missing, fmt.Sprintf("group ID=%d renamed: %q → %q", id, origPath, currentPath))
		}
	}

	// Check projects still exist with same path.
	for id, origPath := range original.projects {
		currentPath, ok := current.projects[id]
		if !ok {
			missing = append(missing, fmt.Sprintf("project %q (ID=%d): missing", origPath, id))
			continue
		}
		if currentPath != origPath {
			missing = append(missing, fmt.Sprintf("project ID=%d renamed: %q → %q", id, origPath, currentPath))
		}
	}
	return missing
}

// cleanupOrphanedProjects permanently deletes any projects whose name starts
// with the E2E prefix. This catches orphans from previous failed runs,
// including projects already marked for delayed deletion.
func cleanupOrphanedProjects(client *gitlabclient.Client) {
	log.Printf("e2e: cleanup: scanning orphaned projects with prefix %q for run ID %q", e2eProjectPrefix, e2eRunID)
	var projects []*gl.Project
	var page int64 = 1
	for {
		opts := &gl.ListProjectsOptions{
			Owned:                new(true),
			IncludePendingDelete: new(true),
		}
		opts.Page = page
		opts.PerPage = 100

		pageProjects, resp, err := client.GL().Projects.ListProjects(opts)
		if err != nil {
			log.Printf("e2e: cleanup: failed to list projects page %d: %v", page, err)
			return
		}
		projects = append(projects, pageProjects...)
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
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
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return
		}
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
	} else if toolutil.IsHTTPStatus(getErr, http.StatusNotFound) {
		return
	}

	_, err = client.GL().Projects.DeleteProject(p.ID, &gl.DeleteProjectOptions{
		PermanentlyRemove: new(true),
		FullPath:          &path,
	})
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return
		}
		log.Printf("e2e: cleanup: failed to permanently delete orphan %q (ID=%d): %v", p.PathWithNamespace, p.ID, err)
	} else {
		log.Printf("e2e: cleanup: permanently deleted orphan project %q (ID=%d)", p.PathWithNamespace, p.ID)
	}
}

// drainSidekiq polls the GitLab Sidekiq metrics API until all background job
// queues are idle (enqueued == 0). Accelerates E2E tests by allowing async
// operations (MR merge checks, pipeline creation, commit indexing) to complete
// before assertions. No-op if the API is unavailable or context is done.
func drainSidekiq(ctx context.Context, t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	if client == nil {
		return
	}
	const maxWait = 15 * time.Second
	const pollInterval = 250 * time.Millisecond

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		stats, _, err := client.GL().Sidekiq.GetJobStats()
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

// hasRunner returns true if a CI runner is available for pipeline tests.
// In Docker mode it always returns true; in self-hosted mode it checks the
// Runners API for registered instance runners.
func hasRunner(client *gitlabclient.Client) bool {
	if isDockerMode() {
		return true
	}
	if client == nil {
		return false
	}
	runnerType := "instance_type"
	runners, _, err := client.GL().Runners.ListRunners(&gl.ListRunnersOptions{
		Type: &runnerType,
	})
	return err == nil && len(runners) > 0
}
