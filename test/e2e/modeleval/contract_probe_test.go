//go:build e2e

// contract_probe_test.go asks each configured provider whether it accepts the
// request this repository builds, and writes down what it learned.
//
// It is the one paid thing in this package that is not a run, and it exists
// because every adapter here is otherwise tested against an httptest server
// answering what the test wrote: a green suite then proves the adapter agrees
// with our own fixture and nothing about the provider. Four of the old
// evaluator's model identifiers were unusable for exactly that reason, and the
// way it was found out was by hand.
//
// It is two turns and not one, in its full mode, because every adapter's
// failure surface is the second turn: the shape of a tool result fed back is a
// tool_result block on one API, a message with the tool role on another and a
// functionResponse part on the third, and one request exercises none of it.
//
// What it sends is the tool list the server publishes, listed from a server
// this file starts in its own process. See [probeTools]: a probe carrying a
// simplified copy of the two schemas would answer a question about a request
// nothing sends.
//
// It needs no GitLab. It never calls harness.New, so the bootstrap that probes
// an instance never runs, and it reads its credentials through harness.Setting
// like everything else here.

package modeleval

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// settingProbe is what turns the probe on, and how far.
const settingProbe = "MODELEVAL_PROBE"

// The values settingProbe takes.
//
// There are two on purpose, and the cheaper one is not a convenience: this is
// the only test in the repository that spends money, so a maintainer who wants
// only the first question answered should be able to ask only that and pay for
// one request per model rather than three.
const (
	// probeOff runs nothing, which is what an unset value means.
	probeOff = ""
	// probeFirstTurn sends one request per model: the tools, a one-line
	// prompt, and the question of whether a tool call comes back.
	probeFirstTurn = "first-turn"
	// probeFull sends that, then the tool result fed back, then the
	// 128-tool slice.
	probeFull = "yes"
)

// sliceSize is how many tools the slice request carries.
//
// It is the individual surface's measurement slice, and the question is whether
// a request that size is accepted at all: the whole individual tools/list is
// 682,878 tokens at Ultimate, over at least one provider's context window
// outright, so what can be measured there is a slice and this says whether that
// slice is servable.
const sliceSize = 128

// probeResult is what one model's probe learned.
type probeResult struct {
	// Spec is the model as configured.
	Spec string `json:"spec"`
	// Provider is the adapter it was called through.
	Provider string `json:"provider"`
	// Model is the identifier sent.
	Model string `json:"model"`
	// OptionsSent are the request options as they went on the wire.
	OptionsSent map[string]string `json:"options_sent"`
	// TemperatureSent is whether the temperature field was in the request,
	// which the reasoning families refuse.
	TemperatureSent bool `json:"temperature_sent"`
	// OptionsRefused are the option names the provider's own refusal
	// mentions, which is a lead rather than a verdict: it is read out of the
	// message, and a provider that refuses a field without naming it leaves
	// this empty.
	OptionsRefused []string `json:"options_refused,omitempty"`
	// FirstTurn is whether a well-formed tool call came back.
	FirstTurn string `json:"first_turn"`
	// SecondTurn is whether the tool result fed back was accepted and
	// answered, or why it was not asked.
	SecondTurn string `json:"second_turn"`
	// Slice is whether a request carrying the slice was accepted.
	Slice string `json:"slice"`
	// Usage is the token counts the provider reported for the first turn,
	// which says which of the four numbers this provider fills in at all.
	Usage modelrecord.Usage `json:"usage"`
	// Detail carries the message of whatever failed.
	Detail string `json:"detail,omitempty"`
}

// probeReport is the file one probe writes.
type probeReport struct {
	// RunOn is the day the probe ran.
	RunOn string `json:"run_on"`
	// Mode is how far it went.
	Mode string `json:"mode"`
	// Results are what each model answered.
	Results []probeResult `json:"results"`
}

// The verdicts a probe records. They are words rather than booleans because
// "not asked" is a third answer and the two-value spelling of it is a lie in
// one direction or the other.
const (
	probeAccepted = "accepted"
	probeRefused  = "refused"
	probeNotAsked = "not asked"
)

// TestProviderContract asks each configured provider whether it accepts the
// request shape this repository builds.
func TestProviderContract(t *testing.T) {
	mode := strings.ToLower(strings.TrimSpace(harness.Setting(settingProbe)))
	switch mode {
	case probeOff:
		t.Skipf("%s is not set: this is the one test here that spends money, so it runs only when "+
			"asked. %s=%s sends one request per model; %s=%s sends the two turns and the slice.",
			settingProbe, settingProbe, probeFirstTurn, settingProbe, probeFull)
	case probeFirstTurn, probeFull:
	default:
		t.Fatalf("%s=%q is not one of %q, %q", settingProbe, mode, probeFirstTurn, probeFull)
	}

	specs, err := configuredModels()
	if err != nil {
		t.Fatalf("reading %s: %v", settingModels, err)
	}

	report := probeReport{RunOn: time.Now().UTC().Format(time.DateOnly), Mode: mode}
	for _, spec := range specs {
		t.Run(spec.Raw, func(t *testing.T) {
			result := probeOne(t, spec, mode)
			report.Results = append(report.Results, result)
			t.Logf("%s: first turn %s, second turn %s, slice %s, usage %+v %s",
				spec.Raw, result.FirstTurn, result.SecondTurn, result.Slice, result.Usage, result.Detail)
		})
	}
	writeProbeReport(t, report)
}

// probeOne probes one model.
func probeOne(t *testing.T, spec provider.Spec, mode string) probeResult {
	t.Helper()
	result := probeResult{
		Spec:            spec.Raw,
		Provider:        spec.Provider,
		Model:           spec.Model,
		OptionsSent:     spec.Wire(),
		TemperatureSent: spec.Temperature != nil,
		FirstTurn:       probeNotAsked,
		SecondTurn:      probeNotAsked,
		Slice:           probeNotAsked,
	}

	key := credentialFor(spec)
	if key == "" && spec.Provider != provider.Fake {
		name, _ := provider.KeyName(spec.Provider)
		t.Skipf("%s is not configured, so %s cannot be probed", name, spec.Raw)
	}
	adapter, err := provider.New(provider.Config{Spec: spec, APIKey: key})
	if err != nil {
		t.Fatalf("building %s: %v", spec.Raw, err)
	}

	tools := probeTools(t, 2)
	first, err := adapter.Call(t.Context(), provider.Request{
		Tools:    tools,
		System:   probeContract,
		Messages: []provider.Message{{Role: provider.RoleUser, Text: probePrompt}},
	})
	result.Usage = first.Usage
	if err != nil {
		result.FirstTurn = probeRefused
		result.Detail = first.Detail
		result.OptionsRefused = optionsNamedIn(first.Detail, spec)
		t.Errorf("the first turn was refused: %v", err)
		return result
	}

	calls := first.ToolCalls()
	switch {
	case len(calls) == 0:
		result.FirstTurn = "answered in text"
		result.Detail = firstText(first)
	case first.Malformed():
		result.FirstTurn = "malformed tool call"
		result.Detail = calls[0].Raw
	default:
		result.FirstTurn = probeAccepted
	}
	if mode == probeFirstTurn {
		return result
	}

	result.SecondTurn = probeSecondTurn(t, adapter, tools, first)
	result.Slice = probeSlice(t, adapter)
	return result
}

// probeSecondTurn feeds a fixed tool result back and asks for the follow-up.
//
// The result is fixed rather than real because what is being probed is the
// shape: whether this provider accepts its own tool call handed back beside an
// answer to it, which is the exchange every attempt past the first depends on.
func probeSecondTurn(t *testing.T, adapter provider.Provider, tools []provider.Tool,
	first provider.Response,
) string {
	t.Helper()
	calls := first.ToolCalls()
	if len(calls) == 0 {
		return probeNotAsked
	}

	second, err := adapter.Call(t.Context(), provider.Request{
		Tools:  tools,
		System: probeContract,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Text: probePrompt},
			{Role: provider.RoleAssistant, Blocks: first.Blocks, Echo: first.Echo},
			{Role: provider.RoleTool, Results: []provider.ToolResult{{
				CallID: calls[0].CallID,
				Tool:   calls[0].Tool,
				Content: "| IID | Title | State |\n| --- | --- | --- |\n" +
					"| 1 | First issue | opened |\n| 2 | Second issue | opened |",
			}}},
		},
	})
	if err != nil {
		t.Errorf("the second turn was refused, which is the turn every attempt past the first "+
			"depends on: %v", err)
		return probeRefused
	}
	if len(second.Blocks) == 0 {
		return "answered with nothing"
	}
	return probeAccepted
}

// probeSlice asks whether a request carrying the individual surface's slice is
// accepted at all.
func probeSlice(t *testing.T, adapter provider.Provider) string {
	t.Helper()
	answer, err := adapter.Call(t.Context(), provider.Request{
		Tools:    probeTools(t, sliceSize),
		System:   probeContract,
		Messages: []provider.Message{{Role: provider.RoleUser, Text: probePrompt}},
	})
	if err != nil {
		// This one is reported and does not fail the test: a provider that
		// refuses the slice is a fact about what the individual surface can
		// be measured on, not a defect in the adapter.
		t.Logf("the %d-tool slice was refused: %v", sliceSize, err)
		return probeRefused
	}
	if len(answer.Blocks) == 0 {
		return "accepted, answered with nothing"
	}
	return probeAccepted
}

// probeContract is the one-line system message the probe sends. It is not the
// surface contract the corpus carries: what is being probed is the request
// shape, and a probe that drifted with the contract would stop being
// comparable with the one before it.
const probeContract = "You call GitLab tools. Use a tool when one fits the request."

// probePrompt is the one-line prompt.
const probePrompt = "List the open issues of project 7."

// probeTools builds a tool list of the given size.
//
// The first two are the dynamic surface's own pair, read from a tools/list of a
// server that registers them through the server's own registration. They were
// written out by hand here until that was compared with what is served, and the
// two differ in the way that matters to this test: the action property carries
// the SEP-2243 x-mcp-header annotation, an unknown keyword inside the schema,
// and both tools carry the descriptions the server publishes rather than a
// sentence written beside the test. A probe asking whether a provider accepts
// the request this repository builds has to send that request; Gemini refusing
// an unknown keyword inside parameters is the class this is about, and is why
// the adapter this package replaces carried a sanitizer of its own.
//
// The rest, when a slice is asked for, are copies of the execute tool under
// other names: the question a slice answers is whether a request carrying that
// many declarations is served, and a provider counts declarations rather than
// reading them.
func probeTools(t *testing.T, count int) []provider.Tool {
	t.Helper()
	tools := servedDynamicTools(t)
	execute := toolNamed(t, tools, dynamictools.ExecuteActionToolName)
	for index := len(tools); index < count; index++ {
		filler := execute
		filler.Name = "gitlab_slice_tool_" + strconv.Itoa(index)
		tools = append(tools, filler)
	}
	return tools
}

// servedDynamicTools lists the dynamic surface's two tools as a client reads
// them.
//
// It needs no GitLab, no credential and no child process: the catalog is the
// shared unbound one, the registration is the server's own, and the listing
// crosses an in-memory transport. What comes back is therefore the schema
// tools/list serialized, which is the one a model is shown; deriving it from
// the structs here instead would measure this test's reading of the surface
// rather than the surface.
//
// The tier is Ultimate because it is the widest catalog, and the catalog is the
// base one rather than the assembled dynamic catalog cmd/server hands the
// surface: which actions exist decides what these two tools can reach and
// nothing about what either of them is called, says or takes, which is all this
// sends.
func servedDynamicTools(t *testing.T) []provider.Tool {
	t.Helper()
	catalog, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("building the action catalog: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "gitlab-mcp-server", Version: "probe"}, nil)
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	serverSide, clientSide := mcp.NewInMemoryTransports()
	if _, connectErr := server.Connect(t.Context(), serverSide, nil); connectErr != nil {
		t.Fatalf("connecting the server: %v", connectErr)
	}
	session, connectErr := mcp.NewClient(&mcp.Implementation{Name: "probe", Version: "probe"}, nil).
		Connect(t.Context(), clientSide, nil)
	if connectErr != nil {
		t.Fatalf("connecting the client: %v", connectErr)
	}
	t.Cleanup(func() { _ = session.Close() })

	listed, listErr := session.ListTools(t.Context(), nil)
	if listErr != nil {
		t.Fatalf("listing the served tools: %v", listErr)
	}
	tools := make([]provider.Tool, 0, len(listed.Tools))
	for _, served := range listed.Tools {
		if served.InputSchema == nil {
			t.Fatalf("%s is served with no input schema", served.Name)
		}
		schema, marshalErr := json.Marshal(served.InputSchema)
		if marshalErr != nil {
			t.Fatalf("re-encoding the schema of %s: %v", served.Name, marshalErr)
		}
		tool, toolErr := provider.NewTool(served.Name, served.Description, schema)
		if toolErr != nil {
			t.Fatalf("building %s: %v", served.Name, toolErr)
		}
		tools = append(tools, tool)
	}
	return tools
}

// toolNamed returns one tool of a list by name.
func toolNamed(t *testing.T, tools []provider.Tool, name string) provider.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("the served list carries no %s: %v", name, toolNames(tools))
	return provider.Tool{}
}

// toolNames spells a tool list for a refusal message.
func toolNames(tools []provider.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

// TestProbeTools_AreTheServedPairAndCarryWhatTheServerPublishes is what keeps
// the probe honest, and it costs nothing: no provider is called and no GitLab
// is contacted, so it runs on every push while the probe itself does not.
//
// It asserts the two things a hand-written copy of the pair got wrong. The list
// is the surface's own two tools under the names the server registers, and the
// execute tool's schema still carries the x-mcp-header annotation after the
// round trip through tools/list and the adapters' normalization, which is the
// keyword a strict provider may refuse and so the one a probe must actually
// send.
func TestProbeTools_AreTheServedPairAndCarryWhatTheServerPublishes(t *testing.T) {
	tools := probeTools(t, 2)
	if len(tools) != 2 {
		t.Fatalf("the dynamic surface served %d tools, want its own pair: %v", len(tools), toolNames(tools))
	}
	for _, name := range []string{dynamictools.FindActionToolName, dynamictools.ExecuteActionToolName} {
		t.Run(name, func(t *testing.T) {
			tool := toolNamed(t, tools, name)
			if strings.TrimSpace(tool.Description) == "" {
				t.Errorf("%s is sent with no description, so the model is shown less than a client is", name)
			}
		})
	}

	execute := toolNamed(t, tools, dynamictools.ExecuteActionToolName)
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(execute.Schema, &schema); err != nil {
		t.Fatalf("the execute schema does not decode: %v", err)
	}
	if _, annotated := schema.Properties["action"]["x-mcp-header"]; !annotated {
		t.Errorf("the execute schema's action property is %v, want the x-mcp-header annotation the "+
			"server publishes: a probe that drops it asks about a request nothing sends",
			schema.Properties["action"])
	}

	// The slice is filled with copies of the execute tool, so a provider
	// counting declarations sees the number the individual surface would send.
	slice := probeTools(t, sliceSize)
	if len(slice) != sliceSize {
		t.Fatalf("the slice carries %d tools, want %d", len(slice), sliceSize)
	}
	if slice[sliceSize-1].Name == execute.Name {
		t.Error("the filler tools share the execute tool's name, so the provider sees one tool declared twice")
	}
}

// firstText returns the prose of an answer, bounded.
func firstText(answer provider.Response) string {
	for _, block := range answer.Blocks {
		if block.Kind == modelrecord.BlockText && block.Text != "" {
			if len(block.Text) > 400 {
				return block.Text[:400]
			}
			return block.Text
		}
	}
	return ""
}

// optionsNamedIn returns the option names a refusal mentions.
//
// It is a lead and says so on the report: providers name the field they refused
// often enough to be worth reading, and a provider that refuses one silently
// leaves this empty rather than being guessed at.
func optionsNamedIn(detail string, spec provider.Spec) []string {
	var named []string
	for option := range spec.Wire() {
		if strings.Contains(strings.ToLower(detail), option) {
			named = append(named, option)
		}
	}
	return named
}

// writeProbeReport writes what the probe learned under dist/.
func writeProbeReport(t *testing.T, report probeReport) {
	t.Helper()
	if len(report.Results) == 0 {
		t.Log("no model answered, so there is nothing to write down")
		return
	}

	root, rootErr := repositoryRoot()
	if rootErr != nil {
		t.Fatalf("finding the repository root: %v", rootErr)
	}
	directory := filepath.Join(root, "dist", "modeleval")
	if mkdirErr := os.MkdirAll(directory, 0o750); mkdirErr != nil {
		t.Fatalf("making %s: %v", directory, mkdirErr)
	}
	encoded, encodeErr := json.MarshalIndent(report, "", "  ")
	if encodeErr != nil {
		t.Fatalf("encoding the probe report: %v", encodeErr)
	}
	path := filepath.Join(directory, "probe-"+report.RunOn+".json")
	if writeErr := os.WriteFile(path, append(encoded, '\n'), 0o600); writeErr != nil {
		t.Fatalf("writing %s: %v", path, writeErr)
	}
	t.Logf("the probe wrote %s", path)
}

// repositoryRoot walks up from the working directory to the module root.
func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(directory, "go.mod")); statErr == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("no go.mod above the working directory")
		}
		directory = parent
	}
}
