// Command eval_meta_tools runs the meta-tool description evaluation fixture
// against Anthropic tool calling without executing any GitLab operation.
//
// Usage:
//
//	go run ./cmd/eval_meta_tools/
//	go run ./cmd/eval_meta_tools/ --max-tasks=5
//	go run ./cmd/eval_meta_tools/ --dry-run
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	defaultTasksPath = "docs/evaluation/meta-tool-schema-discovery-evaluation.md"
	defaultEvalDir   = "plan/metatool-token-schema-research/evals"
	defaultModel     = "claude-sonnet-4-6"
	anthropicAPI     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	toolCallLimit    = 4
)

type options struct {
	TasksPath string
	Output    string
	Model     string
	OnlyIDs   string
	MaxTasks  int
	MaxTokens int
	Retries   int
	RetryWait time.Duration
	Pause     time.Duration
	DryRun    bool
}

type evalTask struct {
	ID             string
	Prompt         string
	ExpectedTool   string
	ExpectedAction string
	RequiredParams []string
	Destructive    bool
}

type anthropicTool struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	InputSchema  any           `json:"input_schema"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature"`
	System      string             `json:"system"`
	Tools       []anthropicTool    `json:"tools"`
	ToolChoice  map[string]string  `json:"tool_choice"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

type anthropicResponse struct {
	ID      string                  `json:"id"`
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
	Error   *anthropicError         `json:"error,omitempty"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type taskResult struct {
	Task             evalTask
	SchemaLookupUsed bool
	FirstTool        string
	FirstAction      string
	FirstPass        bool
	RepairAttempted  bool
	RepairSuccess    bool
	FinalTool        string
	FinalAction      string
	FinalSuccess     bool
	DestructiveSafe  bool
	Notes            []string
}

type validationResult struct {
	Valid           bool
	ToolMatches     bool
	ActionMatches   bool
	RequiredPresent bool
	DestructiveSafe bool
	Action          string
	Message         string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "eval_meta_tools: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	opts := parseFlags()
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	if opts.Model == "" {
		opts.Model = envOrDefault("ANTHROPIC_MODEL", defaultModel)
	}
	if opts.Output == "" {
		opts.Output = defaultOutputPath(opts.Model)
	}

	tasks, err := parseTasksFile(opts.TasksPath)
	if err != nil {
		return err
	}
	tasks = filterTasks(tasks, opts.OnlyIDs)
	if opts.MaxTasks > 0 && opts.MaxTasks < len(tasks) {
		tasks = tasks[:opts.MaxTasks]
	}
	if len(tasks) == 0 {
		return errors.New("no tasks selected")
	}

	client, cleanup, err := newMockGitLabClient()
	if err != nil {
		return err
	}
	defer cleanup()

	mcpTools, routes, err := buildCatalog(client)
	if err != nil {
		return err
	}
	anthropicTools := convertTools(mcpTools)

	if opts.DryRun {
		results := runStaticValidation(tasks, routes)
		return writeReport(opts.Output, opts, results, len(anthropicTools), true)
	}

	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return errors.New("ANTHROPIC_API_KEY is required in the environment or .env")
	}

	runner := &anthropicRunner{
		apiKey:    apiKey,
		model:     opts.Model,
		maxTokens: opts.MaxTokens,
		retries:   opts.Retries,
		retryWait: opts.RetryWait,
		client:    &http.Client{Timeout: 60 * time.Second},
	}

	ctx := context.Background()
	results := make([]taskResult, 0, len(tasks))
	for _, task := range tasks {
		result := runner.evaluateTask(ctx, task, anthropicTools, routes)
		results = append(results, result)
		fmt.Printf("%s: final=%t first=%s/%s final_call=%s/%s\n", task.ID, result.FinalSuccess, result.FirstTool, result.FirstAction, result.FinalTool, result.FinalAction)
		if opts.Pause > 0 {
			time.Sleep(opts.Pause)
		}
	}

	return writeReport(opts.Output, opts, results, len(anthropicTools), false)
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.TasksPath, "tasks", defaultTasksPath, "Markdown file containing the evaluation task fixture")
	flag.StringVar(&opts.Output, "out", "", "Markdown report path")
	flag.StringVar(&opts.Model, "model", "", "Anthropic model; defaults to ANTHROPIC_MODEL or claude-sonnet-4-6")
	flag.StringVar(&opts.OnlyIDs, "task", "", "Comma-separated task IDs to run, for example MT-035,MT-040")
	flag.IntVar(&opts.MaxTasks, "max-tasks", 0, "Limit number of tasks; 0 runs all tasks")
	flag.IntVar(&opts.MaxTokens, "max-tokens", 1024, "Max output tokens per Anthropic request")
	flag.IntVar(&opts.Retries, "retries", 3, "Retries for transient Anthropic 429/5xx responses")
	flag.DurationVar(&opts.RetryWait, "retry-wait", 65*time.Second, "Fallback wait before retrying Anthropic 429 responses")
	flag.DurationVar(&opts.Pause, "pause", 0, "Optional pause between tasks")
	flag.BoolVar(&opts.DryRun, "dry-run", false, "Validate fixture routes without calling Anthropic")
	flag.Parse()
	return opts
}

func filterTasks(tasks []evalTask, onlyIDs string) []evalTask {
	if strings.TrimSpace(onlyIDs) == "" {
		return tasks
	}
	selected := make(map[string]struct{})
	for id := range strings.SplitSeq(onlyIDs, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	filtered := make([]evalTask, 0, len(selected))
	for _, task := range tasks {
		if _, ok := selected[task.ID]; ok {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func defaultOutputPath(model string) string {
	stamp := time.Now().UTC().Format("20060102-150405")
	model = strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(model)
	return filepath.Join(defaultEvalDir, fmt.Sprintf("anthropic-%s-%s.md", stamp, model))
}

func parseTasksFile(path string) ([]evalTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tasks: %w", err)
	}
	return parseTasksMarkdown(string(data))
}

func parseTasksMarkdown(markdown string) ([]evalTask, error) {
	var tasks []evalTask
	for line := range strings.SplitSeq(markdown, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "| MT-") {
			continue
		}
		cols := splitMarkdownRow(line)
		if len(cols) < 7 {
			return nil, fmt.Errorf("task row has %d columns, want at least 7: %s", len(cols), line)
		}
		tool, action, err := parseExpectedToolAction(cols[2])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", cols[0], err)
		}
		tasks = append(tasks, evalTask{
			ID:             cols[0],
			Prompt:         cols[1],
			ExpectedTool:   tool,
			ExpectedAction: action,
			RequiredParams: parseParamList(cols[3]),
			Destructive:    strings.EqualFold(cols[5], "yes"),
		})
	}
	if len(tasks) == 0 {
		return nil, errors.New("no MT-* task rows found")
	}
	return tasks, nil
}

func splitMarkdownRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func parseExpectedToolAction(value string) (tool, action string, err error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected tool/action pair, got %q", value)
	}
	tool = strings.Trim(strings.TrimSpace(parts[0]), "`")
	action = strings.Trim(strings.TrimSpace(parts[1]), "`")
	if tool == "" || action == "" {
		return "", "", fmt.Errorf("empty tool/action in %q", value)
	}
	return tool, action, nil
}

func parseParamList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return nil
	}
	parts := strings.Split(value, ",")
	params := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.Trim(strings.TrimSpace(part), "`")
		if name != "" {
			params = append(params, name)
		}
	}
	return params
}

func newMockGitLabClient() (*gitlabclient.Client, func(), error) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"17.0.0"}`)
	}))
	cfg := &config.Config{GitLabURL: srv.URL, GitLabToken: "eval-token"}
	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		srv.Close()
		return nil, nil, fmt.Errorf("client: %w", err)
	}
	return client, srv.Close, nil
}

func buildCatalog(client *gitlabclient.Client) ([]*mcp.Tool, map[string]toolutil.ActionMap, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: "eval-meta-tools", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
	routes := toolutil.CaptureMetaRoutes(func() {
		tools.RegisterAllMeta(server, client, true)
		tools.RegisterMCPMeta(server, client, nil)
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		return nil, nil, fmt.Errorf("server connect: %w", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "eval-meta-tools-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("list tools: %w", err)
	}
	return result.Tools, routes, nil
}

func convertTools(toolList []*mcp.Tool) []anthropicTool {
	out := make([]anthropicTool, 0, len(toolList))
	for _, tool := range toolList {
		if tool == nil {
			continue
		}
		var inputSchema any = map[string]any{"type": "object"}
		if tool.InputSchema != nil {
			inputSchema = tool.InputSchema
		}
		out = append(out, anthropicTool{Name: tool.Name, Description: tool.Description, InputSchema: inputSchema})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 0 {
		out[len(out)-1].CacheControl = &cacheControl{Type: "ephemeral"}
	}
	return out
}

type anthropicRunner struct {
	apiKey    string
	model     string
	maxTokens int
	retries   int
	retryWait time.Duration
	client    *http.Client
}

func (r *anthropicRunner) evaluateTask(ctx context.Context, task evalTask, catalog []anthropicTool, routes map[string]toolutil.ActionMap) taskResult {
	result := taskResult{Task: task}
	messages := []anthropicMessage{{Role: "user", Content: []anthropicContentBlock{{Type: "text", Text: taskPrompt(task)}}}}
	firstFinalAttempt := true
	repairSent := false

	for range toolCallLimit {
		response, err := r.call(ctx, catalog, messages)
		if err != nil {
			result.Notes = append(result.Notes, err.Error())
			return result
		}
		toolUses := toolUseBlocks(response.Content)
		messages = append(messages, anthropicMessage{Role: "assistant", Content: response.Content})
		if len(toolUses) == 0 {
			result.Notes = append(result.Notes, "model returned no tool_use block")
			return result
		}

		var followups []anthropicContentBlock
		finalProcessed := false
		for _, toolUse := range toolUses {
			if isSchemaLookup(toolUse) {
				result.SchemaLookupUsed = true
				payload, lookupErr := schemaLookupResult(routes, toolUse.Input)
				followups = append(followups, toolResultBlock(toolUse.ID, payload, lookupErr))
				if lookupErr != nil {
					result.Notes = append(result.Notes, lookupErr.Error())
				}
				continue
			}
			if finalProcessed {
				followups = append(followups, toolResultBlock(toolUse.ID, "extra final tool call ignored", errors.New("extra final tool call ignored")))
				continue
			}

			finalProcessed = true
			validation := validateToolCall(task, toolUse.Name, toolUse.Input)
			if firstFinalAttempt {
				result.FirstTool = toolUse.Name
				result.FirstAction = validation.Action
				result.FirstPass = validation.Valid
				firstFinalAttempt = false
			}
			result.FinalTool = toolUse.Name
			result.FinalAction = validation.Action
			result.DestructiveSafe = validation.DestructiveSafe
			if validation.Valid {
				result.FinalSuccess = true
				if repairSent {
					result.RepairSuccess = true
				}
				return result
			}

			result.Notes = append(result.Notes, validation.Message)
			if repairSent {
				return result
			}
			result.RepairAttempted = true
			repairSent = true
			followups = append(followups, toolResultBlock(toolUse.ID, validation.Message, errors.New(validation.Message)))
		}
		if len(followups) > 0 {
			messages = append(messages, anthropicMessage{Role: "user", Content: followups})
			continue
		}
	}

	result.Notes = append(result.Notes, "tool-call step limit reached")
	return result
}

func (r *anthropicRunner) call(ctx context.Context, catalog []anthropicTool, messages []anthropicMessage) (anthropicResponse, error) {
	payload := anthropicRequest{
		Model:       r.model,
		MaxTokens:   r.maxTokens,
		Temperature: 0,
		System:      systemPrompt(),
		Tools:       catalog,
		ToolChoice:  map[string]string{"type": "any"},
		Messages:    messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return anthropicResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	var lastErr error
	for attempt := 0; attempt <= r.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return anthropicResponse{}, ctx.Err()
			case <-time.After(r.retryWait):
			}
		}
		out, retry, callErr := r.callOnce(ctx, body)
		if callErr == nil {
			return out, nil
		}
		lastErr = callErr
		if !retry {
			break
		}
	}
	return anthropicResponse{}, lastErr
}

func (r *anthropicRunner) callOnce(ctx context.Context, body []byte) (anthropicResponse, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return anthropicResponse{}, false, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", r.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	resp, err := r.client.Do(req)
	if err != nil {
		return anthropicResponse{}, true, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return anthropicResponse{}, true, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return anthropicResponse{}, retry, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, redactResponse(respBody))
	}
	var out anthropicResponse
	if decodeErr := json.Unmarshal(respBody, &out); decodeErr != nil {
		return anthropicResponse{}, false, fmt.Errorf("decode response: %w", decodeErr)
	}
	if out.Error != nil {
		return anthropicResponse{}, false, fmt.Errorf("anthropic error %s: %s", out.Error.Type, out.Error.Message)
	}
	return out, false, nil
}

func redactResponse(body []byte) string {
	text := string(body)
	if len(text) > 1000 {
		text = text[:1000] + "..."
	}
	return text
}

func toolUseBlocks(blocks []anthropicContentBlock) []anthropicContentBlock {
	out := make([]anthropicContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "tool_use" {
			out = append(out, block)
		}
	}
	return out
}

func isSchemaLookup(toolUse anthropicContentBlock) bool {
	if toolUse.Name != "gitlab_server" {
		return false
	}
	action, _ := toolUse.Input["action"].(string)
	return action == "schema_get" || action == "schema_index"
}

func schemaLookupResult(routes map[string]toolutil.ActionMap, input map[string]any) (string, error) {
	action, _ := input["action"].(string)
	params, _ := input["params"].(map[string]any)
	switch action {
	case "schema_index":
		if tool, _ := params["tool"].(string); tool != "" {
			index, ok := toolutil.BuildMetaSchemaDiscoveryIndexForTool(routes, tool)
			if !ok {
				return "", fmt.Errorf("schema_index: unknown tool %q", tool)
			}
			return marshalToolResult(index)
		}
		return marshalToolResult(toolutil.BuildMetaSchemaDiscoveryIndex(routes))
	case "schema_get":
		tool, _ := params["tool"].(string)
		selectedAction, _ := params["action"].(string)
		if tool == "" {
			return "", errors.New("schema_get: tool is required")
		}
		if selectedAction == "" {
			index, ok := toolutil.BuildMetaSchemaDiscoveryIndexForTool(routes, tool)
			if !ok {
				return "", fmt.Errorf("schema_get: unknown tool %q", tool)
			}
			return marshalToolResult(index)
		}
		schema, ok := toolutil.LookupMetaActionSchema(routes, tool, selectedAction)
		if !ok {
			return "", fmt.Errorf("schema_get: unknown action %q for tool %q", selectedAction, tool)
		}
		return marshalToolResult(schema)
	default:
		return "", fmt.Errorf("unsupported schema action %q", action)
	}
}

func marshalToolResult(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal tool result: %w", err)
	}
	return string(data), nil
}

func toolResultBlock(toolUseID, content string, err error) anthropicContentBlock {
	block := anthropicContentBlock{Type: "tool_result", ToolUseID: toolUseID, Content: content}
	if err != nil {
		block.IsError = true
		if content == "" {
			block.Content = err.Error()
		}
	}
	return block
}

func systemPrompt() string {
	return `You are evaluating GitLab MCP meta-tool descriptions. Use only the provided tools. Every final task call must use the meta-tool envelope {"action":"...","params":{...}}. You may call gitlab_server schema_index or schema_get first when you need exact params. Do not invent tools, actions, or parameter names. For destructive tasks, include confirm:true inside params when making the final task call. Return tool calls only; do not answer with explanatory text.`
}

func taskPrompt(task evalTask) string {
	destructive := "No"
	if task.Destructive {
		destructive = "Yes; include confirm:true in params for the final task call."
	}
	return fmt.Sprintf("Task %s: %s\nDestructive: %s\nChoose the next MCP tool call needed to perform this task. You may look up schemas first, but the final task call should perform the requested GitLab operation.", task.ID, task.Prompt, destructive)
}

func validateToolCall(task evalTask, toolName string, input map[string]any) validationResult {
	action, _ := input["action"].(string)
	params, _ := input["params"].(map[string]any)
	if params == nil {
		params = map[string]any{}
	}
	result := validationResult{
		ToolMatches:     toolName == task.ExpectedTool,
		ActionMatches:   action == task.ExpectedAction,
		RequiredPresent: true,
		Action:          action,
	}

	var problems []string
	if !result.ToolMatches {
		problems = append(problems, fmt.Sprintf("expected tool %s, got %s", task.ExpectedTool, toolName))
	}
	if !result.ActionMatches {
		problems = append(problems, fmt.Sprintf("expected action %s, got %s", task.ExpectedAction, action))
	}
	for key := range input {
		if key != "action" && key != "params" {
			problems = append(problems, fmt.Sprintf("unexpected top-level parameter %s; put action-specific fields under params", key))
		}
	}
	for _, required := range task.RequiredParams {
		if _, ok := params[required]; !ok {
			result.RequiredPresent = false
			problems = append(problems, fmt.Sprintf("missing required params.%s", required))
		}
	}
	result.DestructiveSafe = true
	if task.Destructive {
		result.DestructiveSafe = isTruthy(params["confirm"])
		if !result.DestructiveSafe {
			problems = append(problems, "destructive task requires params.confirm=true")
		}
	}
	result.Valid = len(problems) == 0
	if result.Valid {
		result.Message = "ok"
	} else {
		result.Message = strings.Join(problems, "; ")
	}
	return result
}

func isTruthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(v)
		return err == nil && parsed
	default:
		return false
	}
}

func runStaticValidation(tasks []evalTask, routes map[string]toolutil.ActionMap) []taskResult {
	results := make([]taskResult, 0, len(tasks))
	for _, task := range tasks {
		_, ok := routes[task.ExpectedTool][task.ExpectedAction]
		result := taskResult{Task: task, FirstTool: task.ExpectedTool, FirstAction: task.ExpectedAction, FinalTool: task.ExpectedTool, FinalAction: task.ExpectedAction, DestructiveSafe: true}
		if ok {
			result.FirstPass = true
			result.FinalSuccess = true
		} else {
			result.Notes = append(result.Notes, "expected route missing from catalog")
		}
		results = append(results, result)
	}
	return results
}

func writeReport(path string, opts options, results []taskResult, toolCount int, dryRun bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	var b strings.Builder
	metrics := calculateMetrics(results)
	mode := "Anthropic tool-calling"
	if dryRun {
		mode = "static route/schema validation"
	}
	fmt.Fprintf(&b, "# Meta-Tool Anthropic Evaluation\n\n")
	fmt.Fprintf(&b, "Date: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "Mode: %s\n", mode)
	fmt.Fprintf(&b, "Model: `%s`\n", opts.Model)
	fmt.Fprintf(&b, "Catalog tools: %d\n", toolCount)
	fmt.Fprintf(&b, "Tasks: %d\n\n", len(results))
	fmt.Fprintf(&b, "## Metrics\n\n")
	fmt.Fprintf(&b, "| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Tool-selection accuracy | %.1f%% |\n", metrics.ToolSelection)
	fmt.Fprintf(&b, "| Action-selection accuracy | %.1f%% |\n", metrics.ActionSelection)
	fmt.Fprintf(&b, "| First-call validation pass rate | %.1f%% |\n", metrics.FirstPass)
	fmt.Fprintf(&b, "| Schema lookup use rate | %.1f%% |\n", metrics.SchemaLookup)
	fmt.Fprintf(&b, "| Repair success rate | %.1f%% |\n", metrics.RepairSuccess)
	fmt.Fprintf(&b, "| Destructive safety | %.1f%% |\n", metrics.DestructiveSafety)
	fmt.Fprintf(&b, "| Final task success proxy | %.1f%% |\n", metrics.FinalSuccess)
	fmt.Fprintf(&b, "\n## Task Results\n\n")
	fmt.Fprintf(&b, "| Task | Expected | First final call | Schema lookup | First pass | Repair | Final success | Notes |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, result := range results {
		notes := strings.Join(result.Notes, "; ")
		if notes == "" {
			notes = "-"
		}
		repair := "-"
		if result.RepairAttempted {
			repair = boolText(result.RepairSuccess)
		}
		fmt.Fprintf(&b, "| %s | `%s` / `%s` | `%s` / `%s` | %s | %s | %s | %s | %s |\n",
			result.Task.ID,
			result.Task.ExpectedTool,
			result.Task.ExpectedAction,
			emptyDash(result.FirstTool),
			emptyDash(result.FirstAction),
			boolText(result.SchemaLookupUsed),
			boolText(result.FirstPass),
			repair,
			boolText(result.FinalSuccess),
			escapeTable(notes),
		)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Printf("wrote evaluation report: %s\n", path)
	return nil
}

type metrics struct {
	ToolSelection     float64
	ActionSelection   float64
	FirstPass         float64
	SchemaLookup      float64
	RepairSuccess     float64
	DestructiveSafety float64
	FinalSuccess      float64
}

func calculateMetrics(results []taskResult) metrics {
	if len(results) == 0 {
		return metrics{}
	}
	var toolOK, actionOK, firstOK, lookupOK, destructiveTotal, destructiveOK, finalOK int
	var repairTotal, repairOK int
	for _, result := range results {
		if result.FirstTool == result.Task.ExpectedTool {
			toolOK++
		}
		if result.FirstAction == result.Task.ExpectedAction {
			actionOK++
		}
		if result.FirstPass {
			firstOK++
		}
		if result.SchemaLookupUsed {
			lookupOK++
		}
		if result.RepairAttempted {
			repairTotal++
			if result.RepairSuccess {
				repairOK++
			}
		}
		if result.Task.Destructive {
			destructiveTotal++
			if result.DestructiveSafe {
				destructiveOK++
			}
		}
		if result.FinalSuccess {
			finalOK++
		}
	}
	return metrics{
		ToolSelection:     percent(toolOK, len(results)),
		ActionSelection:   percent(actionOK, len(results)),
		FirstPass:         percent(firstOK, len(results)),
		SchemaLookup:      percent(lookupOK, len(results)),
		RepairSuccess:     percent(repairOK, repairTotal),
		DestructiveSafety: percent(destructiveOK, destructiveTotal),
		FinalSuccess:      percent(finalOK, len(results)),
	}
}

func percent(value, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(value) * 100 / float64(total)
}

func boolText(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func escapeTable(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}
