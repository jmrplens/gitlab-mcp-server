package evaluator

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type options struct {
	TasksPath              string
	Output                 string
	TraceDir               string
	TerminalLog            string
	Model                  string
	Models                 string
	ToolsFile              string
	CompareReports         stringList
	CheckEfficiency        stringList
	CheckReportClean       stringList
	CompareTraces          stringList
	EfficiencyAllowTask    stringList
	PublishFrom            stringList
	PublishResults         string
	PublishReadme          string
	PublishLabel           string
	PublishMode            string
	Preset                 string
	Partition              string
	ToolSurface            string
	Edition                string
	CoverageReport         string
	Backend                string
	GitLabEnv              string
	DockerCompose          string
	DockerComposeFile      string
	DockerGitLabURL        string
	DockerAutoStart        bool
	DockerWaitTimeout      time.Duration
	MCPCommand             string
	MCPArgs                stringList
	MCPEnv                 string
	Fixtures               string
	OnlyIDs                string
	MaxTasks               int
	Repeat                 int
	MaxTokens              int
	Retries                int
	RetryWait              time.Duration
	Pause                  time.Duration
	Pricing                pricingOptions
	DryRun                 bool
	FixtureSmoke           bool
	PublishDocs            bool
	CheckDocs              bool
	PublishAllowNoise      bool
	MCPSmoke               bool
	Execute                bool
	ExposeResources        bool
	ResourceAccessActive   bool
	PromptAccessActive     bool
	CompletionAccessActive bool
	CapabilityAccessActive bool
	AllowLive              bool
	PrepareFixtures        bool
	FixturesOnly           bool
	UseFixtures            bool
	SkipDestructive        bool
	OnlyDestructive        bool
	SkipMutating           bool
	OnlyMutating           bool
	SkipUnavailable        bool
	PrintOutput            bool
	TraceProviderBodies    bool
	explicitFlags          map[string]bool
}

type stringList []string

// String returns the display label for stringList.
func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

// Set updates the resources value on stringList.
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// evalTask captures eval task data for one evaluation task.
type evalTask struct {
	ID             string
	Prompt         string
	ExpectedTool   string
	ExpectedAction string
	RequiredParams []string
	OptionalParams []string
	Destructive    bool
	Simulation     string
	Steps          []evalStep
	Case           *EvalCase
}

// evalStep keeps existing result and trace code aligned with typed expected steps.
type evalStep = ExpectedStep

// pricingOptions captures pricing options data for evaluation summaries.
type pricingOptions struct {
	InputPerMTok      float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
	CacheReadPerMTok  float64
}

// modelTool holds model tool data for the evaluator package.
type modelTool struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	InputSchema  any           `json:"input_schema"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

// snapshotTool holds snapshot tool data for the evaluator package.
type snapshotTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// cacheControl holds cache control data for the evaluator package.
type cacheControl struct {
	Type string `json:"type"`
}

// anthropicRequest models the Anthropic anthropic request payload.
type anthropicRequest struct {
	Model       string            `json:"model"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature"`
	System      string            `json:"system"`
	Tools       []modelTool       `json:"tools"`
	ToolChoice  map[string]string `json:"tool_choice"`
	Messages    []modelMessage    `json:"messages"`
}

// modelMessage holds model message data for the evaluator package.
type modelMessage struct {
	Role    string              `json:"role"`
	Content []modelContentBlock `json:"content"`
}

// modelContentBlock holds model content block data for the evaluator package.
type modelContentBlock struct {
	Type             string          `json:"type"`
	Text             string          `json:"text,omitempty"`
	ID               string          `json:"id,omitempty"`
	Name             string          `json:"name,omitempty"`
	Input            map[string]any  `json:"input,omitempty"`
	ToolUseID        string          `json:"tool_use_id,omitempty"`
	Content          string          `json:"content,omitempty"`
	IsError          bool            `json:"is_error,omitempty"`
	ProviderRawInput json.RawMessage `json:"-"`
	ThoughtSignature string          `json:"-"`
}

// MarshalJSON preserves Anthropic's required empty input object for tool_use blocks.
func (b modelContentBlock) MarshalJSON() ([]byte, error) {
	payload := map[string]any{"type": b.Type}
	if b.Text != "" {
		payload["text"] = b.Text
	}
	if b.ID != "" {
		payload["id"] = b.ID
	}
	if b.Name != "" {
		payload["name"] = b.Name
	}
	if b.Type == "tool_use" {
		input := b.Input
		if input == nil {
			// Anthropic rejects tool_use blocks with null or omitted input; an
			// empty object preserves no-argument tool calls during replay.
			input = map[string]any{}
		}
		payload["input"] = input
	} else if len(b.Input) > 0 {
		// Non-tool blocks only include input when a provider returned one.
		payload["input"] = b.Input
	}
	if b.ToolUseID != "" {
		// tool_result blocks use tool_use_id to connect results to requests.
		payload["tool_use_id"] = b.ToolUseID
	}
	if b.Content != "" {
		payload["content"] = b.Content
	}
	if b.IsError {
		// Downstream providers need is_error to treat failed tool results correctly.
		payload["is_error"] = b.IsError
	}
	return json.Marshal(payload)
}

// modelResponse holds model response data for the evaluator package.
type modelResponse struct {
	ID            string              `json:"id"`
	Type          string              `json:"type"`
	Role          string              `json:"role"`
	Content       []modelContentBlock `json:"content"`
	Usage         modelUsage          `json:"usage"`
	Error         *modelError         `json:"error,omitempty"`
	ProviderTrace *modelProviderTrace `json:"-"`
}

// modelProviderTrace records the provider HTTP exchange without sensitive headers.
type modelProviderTrace struct {
	Provider         string          `json:"provider"`
	Method           string          `json:"method"`
	Endpoint         string          `json:"endpoint"`
	RequestBody      json.RawMessage `json:"request_body,omitempty"`
	ResponseStatus   int             `json:"response_status,omitempty"`
	ResponseBody     json.RawMessage `json:"response_body,omitempty"`
	ResponseBodyText string          `json:"response_body_text,omitempty"`
}

// modelUsage holds model usage data for the evaluator package.
type modelUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// add handles add for modelUsage.
func (u *modelUsage) add(other modelUsage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
}

// modelError holds model error data for the evaluator package.
type modelError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// taskResult captures task result data for one evaluation task.
type taskResult struct {
	Task                 evalTask
	Run                  int
	Model                string
	ToolSurface          string
	SchemaLookupUsed     bool
	ResourceLookupUsed   bool
	CapabilityLookupUsed bool
	FirstTool            string
	FirstAction          string
	FirstPass            bool
	RepairAttempted      bool
	RepairSuccess        bool
	FinalTool            string
	FinalAction          string
	FinalSuccess         bool
	DestructiveSafe      bool
	CompletedSteps       int
	ModelCalls           int
	ToolCalls            int
	ResourceCalls        int
	CapabilityCalls      int
	Usage                modelUsage
	Notes                []string
	AssertionResults     []CaseAssertionResult
	Trace                taskTrace
}

// taskTrace records task trace data in evaluation traces.
type taskTrace struct {
	Run          int                 `json:"run"`
	Model        string              `json:"model,omitempty"`
	TaskID       string              `json:"task_id"`
	Prompt       string              `json:"prompt"`
	SystemPrompt string              `json:"system_prompt"`
	UserPrompt   string              `json:"user_prompt"`
	Expected     []traceExpectedStep `json:"expected"`
	Events       []traceEvent        `json:"events"`
	Summary      traceSummary        `json:"summary"`
}

// traceExpectedStep records trace expected step data in evaluation traces.
type traceExpectedStep struct {
	Step           int      `json:"step"`
	Tool           string   `json:"tool"`
	Action         string   `json:"action,omitempty"`
	RequiredParams []string `json:"required_params,omitempty"`
	OptionalParams []string `json:"optional_params,omitempty"`
	OptionalStep   bool     `json:"optional_step,omitempty"`
	Destructive    bool     `json:"destructive"`
	Simulation     string   `json:"simulation,omitempty"`
}

// traceEvent records trace event data in evaluation traces.
type traceEvent struct {
	Turn       int                 `json:"turn"`
	Kind       string              `json:"kind"`
	Role       string              `json:"role,omitempty"`
	ToolUseID  string              `json:"tool_use_id,omitempty"`
	Tool       string              `json:"tool,omitempty"`
	Action     string              `json:"action,omitempty"`
	Input      map[string]any      `json:"input,omitempty"`
	RawInput   json.RawMessage     `json:"provider_raw_input,omitempty"`
	Blocks     []modelContentBlock `json:"blocks,omitempty"`
	Content    string              `json:"content,omitempty"`
	IsError    bool                `json:"is_error,omitempty"`
	Usage      *modelUsage         `json:"usage,omitempty"`
	Provider   *modelProviderTrace `json:"provider,omitempty"`
	MCP        *traceMCPExchange   `json:"mcp,omitempty"`
	Validation *traceValidation    `json:"validation,omitempty"`
}

// traceMCPExchange records the actual MCP tool request and response.
type traceMCPExchange struct {
	Request        traceMCPRequest `json:"request"`
	Response       json.RawMessage `json:"response,omitempty"`
	ResponseText   string          `json:"response_text,omitempty"`
	IsError        bool            `json:"is_error,omitempty"`
	DurationMillis int64           `json:"duration_ms,omitempty"`
	ProtocolError  string          `json:"protocol_error,omitempty"`
}

// traceMCPRequest records the MCP CallTool payload.
type traceMCPRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// modelProviderCallError preserves provider exchange details for failed calls.
type modelProviderCallError struct {
	err   error
	Trace *modelProviderTrace
}

// Error returns the wrapped provider call error text.
func (e *modelProviderCallError) Error() string {
	return e.err.Error()
}

// Unwrap returns the underlying provider call error.
func (e *modelProviderCallError) Unwrap() error {
	return e.err
}

// traceValidation records trace validation data in evaluation traces.
type traceValidation struct {
	Valid           bool   `json:"valid"`
	ToolMatches     bool   `json:"tool_matches"`
	ActionMatches   bool   `json:"action_matches"`
	RequiredPresent bool   `json:"required_present"`
	DestructiveSafe bool   `json:"destructive_safe"`
	Message         string `json:"message"`
}

// traceSummary records trace summary data in evaluation traces.
type traceSummary struct {
	FirstTool            string `json:"first_tool,omitempty"`
	FirstAction          string `json:"first_action,omitempty"`
	FinalTool            string `json:"final_tool,omitempty"`
	FinalAction          string `json:"final_action,omitempty"`
	SchemaLookupUsed     bool   `json:"schema_lookup_used"`
	ResourceLookupUsed   bool   `json:"resource_lookup_used"`
	CapabilityLookupUsed bool   `json:"capability_lookup_used"`
	FirstPass            bool   `json:"first_pass"`
	RepairAttempted      bool   `json:"repair_attempted"`
	RepairSuccess        bool   `json:"repair_success"`
	FinalSuccess         bool   `json:"final_success"`
	DestructiveSafe      bool   `json:"destructive_safe"`
	CompletedSteps       int    `json:"completed_steps"`
	ExpectedSteps        int    `json:"expected_steps"`
	ModelCalls           int    `json:"model_calls"`
	ToolCalls            int    `json:"tool_calls"`
	ResourceCalls        int    `json:"resource_calls"`
	CapabilityCalls      int    `json:"capability_calls"`
	Notes                string `json:"notes,omitempty"`
}

// validationResult holds validation result data for the evaluator package.
type validationResult struct {
	Valid           bool
	ToolMatches     bool
	ActionMatches   bool
	RequiredPresent bool
	DestructiveSafe bool
	Action          string
	Message         string
}

// simulationResult holds simulation result data for the evaluator package.
type simulationResult struct {
	Content  string
	Advance  bool
	Injected bool
	Err      error
	MCP      *traceMCPExchange
}

// resourceLookupResult captures one evaluator resource bridge response.
type resourceLookupResult struct {
	Content string
	Err     error
	MCP     *traceMCPExchange
}

type mcpBridgeSupport struct {
	Capabilities bool
	Resources    bool
	Prompts      bool
	Completion   bool
}

func (s mcpBridgeSupport) any() bool {
	return s.Capabilities || s.Resources || s.Prompts || s.Completion
}

type evalResourceRef struct {
	URI         string           `json:"uri"`
	Name        string           `json:"name,omitempty"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	MIMEType    string           `json:"mime_type,omitempty"`
	Annotations *mcp.Annotations `json:"annotations,omitempty"`
}

type evalResourceTemplateRef struct {
	URITemplate string           `json:"uri_template"`
	Name        string           `json:"name,omitempty"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	MIMEType    string           `json:"mime_type,omitempty"`
	Annotations *mcp.Annotations `json:"annotations,omitempty"`
}

type evalResourceListOutput struct {
	Resources         []evalResourceRef         `json:"resources"`
	ResourceTemplates []evalResourceTemplateRef `json:"resource_templates"`
}

type evalResourceContent struct {
	URI      string `json:"uri,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`
	Text     string `json:"text,omitempty"`
}

type evalResourceReadOutput struct {
	URI      string                `json:"uri"`
	Contents []evalResourceContent `json:"contents"`
}

type evalCapabilitiesOutput struct {
	ProtocolVersion string                  `json:"protocol_version,omitempty"`
	ServerInfo      *mcp.Implementation     `json:"server_info,omitempty"`
	Capabilities    *mcp.ServerCapabilities `json:"capabilities,omitempty"`
	Instructions    string                  `json:"instructions,omitempty"`
	BridgeTools     []string                `json:"bridge_tools"`
}

type evalPromptListOutput struct {
	Prompts []*mcp.Prompt `json:"prompts"`
}
