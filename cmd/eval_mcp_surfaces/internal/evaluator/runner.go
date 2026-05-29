package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	mcpToolTransientAttempts  = 5
	mcpToolCallTimeout        = 3 * time.Minute
	mcpToolTransientRetryWait = 750 * time.Millisecond
)

type modelRunner struct {
	apiKey      string
	provider    string
	model       string
	modelLabel  string
	toolSurface string
	maxTokens   int
	retries     int
	retryWait   time.Duration
	client      *http.Client
	mcpSession  *mcp.ClientSession
	mcpBridge   mcpBridgeSupport
	traceBodies bool
}

type modelEvaluationState struct {
	firstFinalAttempt      bool
	lastInvalidFingerprint string
	repairCount            int
	stepIndex              int
	simulationAttempts     map[int]int
	simulatedErrorSeen     bool
	messages               []modelMessage
}

type capabilityBridgeStepContext struct {
	task              evalTask
	steps             []evalStep
	toolUse           modelContentBlock
	routes            map[string]toolutil.ActionMap
	result            *taskResult
	repairAlreadySent bool
	state             *modelEvaluationState
	followups         *[]modelContentBlock
}

type lookupFollowupContext struct {
	task      evalTask
	steps     []evalStep
	stepIndex int
	toolUse   modelContentBlock
	budget    taskCallBudget
	routes    map[string]toolutil.ActionMap
	result    *taskResult
	followups *[]modelContentBlock
	dynamic   bool
}

type validToolStepContext struct {
	steps     []evalStep
	toolUse   modelContentBlock
	result    *taskResult
	followups *[]modelContentBlock
	state     *modelEvaluationState
}

type auxiliaryToolUseContext struct {
	task              evalTask
	steps             []evalStep
	toolUse           modelContentBlock
	callBudget        taskCallBudget
	routes            map[string]toolutil.ActionMap
	result            *taskResult
	repairAlreadySent bool
	state             *modelEvaluationState
	followups         *[]modelContentBlock
}

type modelTurnContext struct {
	task        evalTask
	steps       []evalStep
	callBudget  taskCallBudget
	routes      map[string]toolutil.ActionMap
	result      *taskResult
	repairLimit int
	state       *modelEvaluationState
}

type stepToolUseContext struct {
	turn              modelTurnContext
	toolUse           modelContentBlock
	repairAlreadySent bool
	followups         *[]modelContentBlock
}

// evaluateTask handles evaluate task for modelRunner.
func (r *modelRunner) evaluateTask(ctx context.Context, task evalTask, catalog []modelTool, routes map[string]toolutil.ActionMap) taskResult {
	return r.evaluatePreparedCase(ctx, preparedCaseFromTask(task), catalog, routes)
}

func (r *modelRunner) evaluatePreparedCase(ctx context.Context, prepared PreparedCase, catalog []modelTool, routes map[string]toolutil.ActionMap) taskResult {
	task := taskForSurface(taskFromPreparedCase(prepared), r.toolSurface)
	steps := prepared.Steps
	userPrompt := taskPromptForSurface(task, r.toolSurface)
	systemPrompt := systemPromptForTask(task, r.toolSurface)
	callBudget := callBudgetForTask(task, r.toolSurface)
	result := taskResult{Task: task, Model: r.modelLabel, ToolSurface: r.toolSurface, DestructiveSafe: true, Trace: newTaskTrace(task, systemPrompt, userPrompt)}
	result.Trace.Model = r.modelLabel
	repairLimit := repairAttemptLimitForTask(r.toolSurface, len(steps))
	state := &modelEvaluationState{
		firstFinalAttempt:  true,
		simulationAttempts: map[int]int{},
		messages:           []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: userPrompt}}}},
	}

	for range taskToolCallLimitForSurface(len(steps), r.toolSurface) {
		response, err := r.call(ctx, systemPrompt, catalog, state.messages)
		result.ModelCalls++
		result.Usage.add(response.Usage)
		if err != nil {
			recordModelCallError(&result, err)
			return result
		}
		turnCtx := modelTurnContext{task: task, steps: steps, callBudget: callBudget, routes: routes, result: &result, repairLimit: repairLimit, state: state}
		if r.handleModelTurn(ctx, response, turnCtx) {
			return result
		}
	}

	result.Notes = append(result.Notes, fmt.Sprintf("tool-call step limit reached after %d/%d scenario steps", result.CompletedSteps, len(steps)))
	return result
}

func preparedCaseFromTask(task evalTask) PreparedCase {
	evalCase := EvalCase{ID: EvalCaseID(task.ID), Prompt: task.Prompt, Steps: stepsFromTask(task)}
	if task.Case != nil {
		evalCase = *task.Case
	}
	if task.Prompt != "" {
		evalCase.Prompt = task.Prompt
	}
	steps := stepsFromTask(task)
	if len(steps) == 0 {
		steps = cloneExpectedSteps(evalCase.Steps)
	}
	return PreparedCase{Case: evalCase, Prompt: firstNonEmpty(task.Prompt, casePrompt(evalCase)), Steps: steps}
}

func taskFromPreparedCase(prepared PreparedCase) evalTask {
	evalCase := prepared.Case
	if prepared.Prompt != "" {
		evalCase.Prompt = prepared.Prompt
	}
	if len(prepared.Steps) > 0 {
		evalCase.Steps = cloneExpectedSteps(prepared.Steps)
	}
	task := taskFromCase(evalCase)
	if prepared.Prompt != "" {
		task.Prompt = prepared.Prompt
	}
	if len(prepared.Steps) > 0 {
		task.Steps = cloneExpectedSteps(prepared.Steps)
	}
	return task
}

func recordModelCallError(result *taskResult, err error) {
	result.Notes = append(result.Notes, err.Error())
	event := traceEvent{Turn: result.ModelCalls, Kind: "model_error", Content: err.Error(), IsError: true}
	if providerErr, ok := errors.AsType[*modelProviderCallError](err); ok {
		event.Provider = providerErr.Trace
	}
	result.Trace.Events = append(result.Trace.Events, event)
}

func (r *modelRunner) handleModelTurn(ctx context.Context, response modelResponse, turnCtx modelTurnContext) bool {
	toolUses := toolUseBlocks(response.Content)
	turnCtx.result.ToolCalls += len(toolUses)
	turnCtx.state.messages = append(turnCtx.state.messages, modelMessage{Role: "assistant", Content: response.Content})
	usage := response.Usage
	turnCtx.result.Trace.Events = append(turnCtx.result.Trace.Events, traceEvent{Turn: turnCtx.result.ModelCalls, Kind: "assistant_message", Role: "assistant", Blocks: response.Content, Usage: &usage, Provider: response.ProviderTrace})
	if len(toolUses) == 0 {
		return handleNoToolUseResult(turnCtx.result, turnCtx.state, turnCtx.repairLimit, turnCtx.steps)
	}

	followups := make([]modelContentBlock, 0, len(toolUses))
	repairAlreadySent := turnCtx.state.repairCount >= turnCtx.repairLimit
	for _, toolUse := range toolUses {
		if r.handleStepToolUse(ctx, stepToolUseContext{turn: turnCtx, toolUse: toolUse, repairAlreadySent: repairAlreadySent, followups: &followups}) {
			return true
		}
	}
	if len(followups) == 0 {
		return false
	}
	turnCtx.state.messages = append(turnCtx.state.messages, modelMessage{Role: "user", Content: followups})
	return false
}

func (r *modelRunner) handleStepToolUse(ctx context.Context, stepCtx stepToolUseContext) bool {
	turnCtx := stepCtx.turn
	toolUse := stepCtx.toolUse
	turnCtx.result.Trace.Events = append(turnCtx.result.Trace.Events, traceToolUseEvent(turnCtx.result.ModelCalls, toolUse))
	auxCtx := auxiliaryToolUseContext{task: turnCtx.task, steps: turnCtx.steps, toolUse: toolUse, callBudget: turnCtx.callBudget, routes: turnCtx.routes, result: turnCtx.result, repairAlreadySent: stepCtx.repairAlreadySent, state: turnCtx.state, followups: stepCtx.followups}
	handled, stop := r.handleAuxiliaryToolUse(ctx, auxCtx)
	if handled || stop {
		return stop
	}

	stepIndex := turnCtx.state.stepIndex
	step := turnCtx.steps[stepIndex]
	validation := validateStepCallWithRoutes(step, toolUse.Name, toolUse.Input, turnCtx.routes)
	if !validation.Valid {
		var accepted bool
		step, validation, accepted = acceptDirectDynamicExecuteStep(turnCtx.steps, stepIndex, toolUse, validation, turnCtx.routes, turnCtx.state)
		if accepted {
			stepIndex = turnCtx.state.stepIndex
			turnCtx.result.Notes = append(turnCtx.result.Notes, fmt.Sprintf("accepted direct %s call without prior %s for action %s", dynamicExecuteActionTool, dynamicFindTool, step.ExpectedAction))
		}
	}
	recordStepAssertionResults(turnCtx.result, step, validation, stepIndex+1)
	turnCtx.result.Trace.Events = append(turnCtx.result.Trace.Events, traceValidationEvent(turnCtx.result.ModelCalls, validation))
	if acceptsDynamicPreludeCall(r.toolSurface, step, validation) {
		appendDynamicPreludeFollowup(turnCtx.steps, stepIndex, toolUse, validation, turnCtx.result, turnCtx.state, stepCtx.followups)
		return false
	}
	recordValidationAttempt(turnCtx.result, toolUse, validation, turnCtx.state)
	if validation.Valid {
		turnCtx.state.lastInvalidFingerprint = ""
		validCtx := validToolStepContext{steps: turnCtx.steps, toolUse: toolUse, result: turnCtx.result, followups: stepCtx.followups, state: turnCtx.state}
		return r.handleValidToolStep(ctx, validCtx)
	}
	return r.handleInvalidStepToolUse(ctx, stepCtx, validation)
}

func acceptDirectDynamicExecuteStep(steps []evalStep, stepIndex int, toolUse modelContentBlock, validation validationResult, routes map[string]toolutil.ActionMap, state *modelEvaluationState) (evalStep, validationResult, bool) {
	if validation.Valid {
		return steps[stepIndex], validation, false
	}
	if stepIndex < 0 || stepIndex >= len(steps) {
		return evalStep{}, validation, false
	}
	current := steps[stepIndex]
	if !expectedDynamicFindStep(current) {
		return current, validation, false
	}
	nextIndex := stepIndex + 1
	if nextIndex >= len(steps) {
		return current, validation, false
	}
	next := steps[nextIndex]
	if next.ExpectedTool != dynamicExecuteActionTool || next.ExpectedAction == "" {
		return current, validation, false
	}
	nextValidation := validateStepCallWithRoutes(next, toolUse.Name, toolUse.Input, routes)
	if !nextValidation.Valid {
		return current, validation, false
	}
	state.stepIndex = nextIndex
	return next, nextValidation, true
}

func (r *modelRunner) handleInvalidStepToolUse(ctx context.Context, stepCtx stepToolUseContext, validation validationResult) bool {
	turnCtx := stepCtx.turn
	stepIndex := turnCtx.state.stepIndex
	toolUse := stepCtx.toolUse
	if recordInvalidToolUse(turnCtx.result, stepIndex, validation, toolUse, turnCtx.state) {
		return true
	}
	if stepCtx.repairAlreadySent {
		return true
	}
	turnCtx.result.RepairAttempted = true
	turnCtx.state.repairCount++
	if r.canExecuteInvalidToolCall(turnCtx.steps[stepIndex], validation, toolUse, turnCtx.routes) {
		turnCtx.state.simulationAttempts[stepIndex]++
		simulation := r.mcpToolResult(ctx, toolUse)
		if simulation.Err != nil {
			turnCtx.result.Notes = append(turnCtx.result.Notes, toolExecutionNote(stepIndex+1, turnCtx.steps[stepIndex], simulation.Err))
		}
		block := toolResultBlock(toolUse.ID, simulation.Content, simulation.Err)
		*stepCtx.followups = append(*stepCtx.followups, block)
		turnCtx.result.Trace.Events = append(turnCtx.result.Trace.Events, traceToolResultEventWithMCP(turnCtx.result.ModelCalls, block, simulation.MCP))
		return false
	}
	repairMessage := validationRepairMessage(turnCtx.task, turnCtx.steps[stepIndex], validation, toolUse.Input)
	block := toolResultBlock(toolUse.ID, repairMessage, errors.New(repairMessage))
	*stepCtx.followups = append(*stepCtx.followups, block)
	turnCtx.result.Trace.Events = append(turnCtx.result.Trace.Events, traceToolResultEvent(turnCtx.result.ModelCalls, block))
	return false
}

func (r *modelRunner) handleValidToolStep(ctx context.Context, validCtx validToolStepContext) bool {
	stepIndex := validCtx.state.stepIndex
	completedStep := validCtx.steps[stepIndex]
	simulation := r.validatedToolResult(ctx, completedStep, validCtx.toolUse, validCtx.state.simulationAttempts[stepIndex], stepIndex+1, len(validCtx.steps))
	if simulation.Injected {
		hadPreviousAttempt := validCtx.state.simulationAttempts[stepIndex] > 0
		validCtx.state.simulationAttempts[stepIndex]++
		if simulation.Err != nil {
			validCtx.result.RepairAttempted = true
			validCtx.state.simulatedErrorSeen = true
			validCtx.result.Notes = append(validCtx.result.Notes, toolExecutionNote(stepIndex+1, completedStep, simulation.Err))
		} else if hadPreviousAttempt {
			validCtx.result.RepairSuccess = true
		}
		block := toolResultBlock(validCtx.toolUse.ID, simulation.Content, simulation.Err)
		*validCtx.followups = append(*validCtx.followups, block)
		validCtx.result.Trace.Events = append(validCtx.result.Trace.Events, traceToolResultEventWithMCP(validCtx.result.ModelCalls, block, simulation.MCP))
		return advanceInjectedSimulation(validCtx, simulation.Err, simulation.Advance)
	}
	if validCtx.state.simulationAttempts[stepIndex] > 0 {
		validCtx.result.RepairSuccess = true
	}
	validCtx.state.stepIndex++
	validCtx.result.CompletedSteps = validCtx.state.stepIndex
	if validCtx.state.repairCount > 0 {
		validCtx.result.RepairSuccess = true
	}
	block := toolResultBlock(validCtx.toolUse.ID, successfulSimulatedToolContent(completedStep, validCtx.toolUse, validCtx.state.stepIndex+1, len(validCtx.steps)), nil)
	*validCtx.followups = append(*validCtx.followups, block)
	validCtx.result.Trace.Events = append(validCtx.result.Trace.Events, traceToolResultEvent(validCtx.result.ModelCalls, block))
	if validCtx.state.stepIndex == len(validCtx.steps) {
		validCtx.result.FinalSuccess = true
		if validCtx.state.simulatedErrorSeen {
			validCtx.result.RepairSuccess = true
		}
		return true
	}
	return false
}

func advanceInjectedSimulation(validCtx validToolStepContext, simulationErr error, advance bool) bool {
	if !advance {
		return false
	}
	validCtx.state.stepIndex++
	validCtx.result.CompletedSteps = validCtx.state.stepIndex
	if validCtx.state.stepIndex != len(validCtx.steps) {
		return false
	}
	validCtx.result.FinalSuccess = simulationErr == nil
	if validCtx.result.FinalSuccess && (validCtx.state.repairCount > 0 || validCtx.state.simulatedErrorSeen) {
		validCtx.result.RepairSuccess = true
	}
	return true
}

func (r *modelRunner) handleAuxiliaryToolUse(ctx context.Context, auxCtx auxiliaryToolUseContext) (handled, stop bool) {
	stepIndex := auxCtx.state.stepIndex
	if stepIndex < len(auxCtx.steps) && expectedCapabilityBridgeStep(auxCtx.steps[stepIndex]) && isCapabilityBridge(auxCtx.toolUse) {
		bridgeCtx := capabilityBridgeStepContext{task: auxCtx.task, steps: auxCtx.steps, toolUse: auxCtx.toolUse, routes: auxCtx.routes, result: auxCtx.result, repairAlreadySent: auxCtx.repairAlreadySent, state: auxCtx.state, followups: auxCtx.followups}
		return true, r.handleExpectedCapabilityBridgeStep(ctx, bridgeCtx)
	}
	if isCapabilityBridge(auxCtx.toolUse) {
		r.appendCapabilityBridgeFollowup(ctx, auxCtx.toolUse, auxCtx.result, auxCtx.followups)
		return true, false
	}
	if isSchemaLookup(auxCtx.toolUse) {
		r.appendLookupFollowup(ctx, lookupFollowupContext{task: auxCtx.task, steps: auxCtx.steps, stepIndex: stepIndex, toolUse: auxCtx.toolUse, budget: auxCtx.callBudget, routes: auxCtx.routes, result: auxCtx.result, followups: auxCtx.followups})
		return true, false
	}
	if stepIndex < len(auxCtx.steps) && expectedDynamicFindStep(auxCtx.steps[stepIndex]) && isDynamicDiscovery(auxCtx.toolUse) {
		return true, r.handleExpectedDynamicFindStep(ctx, auxCtx)
	}
	if isDynamicDiscovery(auxCtx.toolUse) {
		r.appendLookupFollowup(ctx, lookupFollowupContext{task: auxCtx.task, steps: auxCtx.steps, stepIndex: stepIndex, toolUse: auxCtx.toolUse, budget: auxCtx.callBudget, routes: auxCtx.routes, result: auxCtx.result, followups: auxCtx.followups, dynamic: true})
		return true, false
	}
	if stepIndex >= len(auxCtx.steps) {
		block := toolResultBlock(auxCtx.toolUse.ID, "scenario already completed", errors.New("scenario already completed"))
		*auxCtx.followups = append(*auxCtx.followups, block)
		auxCtx.result.Trace.Events = append(auxCtx.result.Trace.Events, traceToolResultEvent(auxCtx.result.ModelCalls, block))
		return true, false
	}
	return false, false
}

func (r *modelRunner) handleExpectedDynamicFindStep(ctx context.Context, auxCtx auxiliaryToolUseContext) bool {
	stepIndex := auxCtx.state.stepIndex
	step := auxCtx.steps[stepIndex]
	validation := validateStepCallWithRoutes(step, auxCtx.toolUse.Name, auxCtx.toolUse.Input, auxCtx.routes)
	recordStepAssertionResults(auxCtx.result, step, validation, stepIndex+1)
	auxCtx.result.Trace.Events = append(auxCtx.result.Trace.Events, traceValidationEvent(auxCtx.result.ModelCalls, validation))
	recordValidationAttempt(auxCtx.result, auxCtx.toolUse, validation, auxCtx.state)
	if !validation.Valid {
		return handleInvalidExpectedDynamicFindCall(auxCtx, step, validation)
	}
	auxCtx.state.lastInvalidFingerprint = ""
	auxCtx.result.SchemaLookupUsed = true
	payload, exchange, lookupErr := r.lookupToolResult(ctx, auxCtx.routes, auxCtx.toolUse, true)
	if lookupErr == nil {
		lookupErr = validateDynamicFindResult(auxCtx.steps, stepIndex, payload, exchange)
	}
	block := toolResultBlock(auxCtx.toolUse.ID, payload, lookupErr)
	*auxCtx.followups = append(*auxCtx.followups, block)
	if exchange != nil {
		auxCtx.result.Trace.Events = append(auxCtx.result.Trace.Events, traceToolResultEventWithMCP(auxCtx.result.ModelCalls, block, exchange))
	} else {
		auxCtx.result.Trace.Events = append(auxCtx.result.Trace.Events, traceToolResultEvent(auxCtx.result.ModelCalls, block))
	}
	if lookupErr != nil {
		auxCtx.result.Notes = append(auxCtx.result.Notes, lookupErr.Error())
		return false
	}
	auxCtx.state.stepIndex++
	auxCtx.result.CompletedSteps = auxCtx.state.stepIndex
	if auxCtx.state.repairCount > 0 {
		auxCtx.result.RepairSuccess = true
	}
	if auxCtx.state.stepIndex == len(auxCtx.steps) {
		auxCtx.result.FinalSuccess = true
		return true
	}
	return false
}

func handleInvalidExpectedDynamicFindCall(auxCtx auxiliaryToolUseContext, step evalStep, validation validationResult) bool {
	if recordInvalidToolUse(auxCtx.result, auxCtx.state.stepIndex, validation, auxCtx.toolUse, auxCtx.state) {
		return true
	}
	if auxCtx.repairAlreadySent {
		return true
	}
	auxCtx.result.RepairAttempted = true
	auxCtx.state.repairCount++
	repairMessage := validationRepairMessage(auxCtx.task, step, validation, auxCtx.toolUse.Input)
	block := toolResultBlock(auxCtx.toolUse.ID, repairMessage, errors.New(repairMessage))
	*auxCtx.followups = append(*auxCtx.followups, block)
	auxCtx.result.Trace.Events = append(auxCtx.result.Trace.Events, traceToolResultEvent(auxCtx.result.ModelCalls, block))
	return false
}

func expectedDynamicFindStep(step evalStep) bool {
	return step.ExpectedTool == dynamicFindTool && step.ExpectedAction == ""
}

func validateDynamicFindResult(steps []evalStep, stepIndex int, payload string, exchange *traceMCPExchange) error {
	expectedAction := nextDynamicExecuteAction(steps, stepIndex)
	if expectedAction == "" || dynamicFindPayloadIncludesAction(payload, expectedAction) || dynamicFindExchangeIncludesAction(exchange, expectedAction) {
		return nil
	}
	return fmt.Errorf("gitlab_find_action results did not include expected action %s; retry with a query that describes the requested GitLab operation", expectedAction)
}

func nextDynamicExecuteAction(steps []evalStep, stepIndex int) string {
	if stepIndex+1 >= len(steps) {
		return ""
	}
	next := steps[stepIndex+1]
	if dynamicExecuteStep(next) {
		return next.ExpectedAction
	}
	return ""
}

func dynamicFindPayloadIncludesAction(payload, expectedAction string) bool {
	type dynamicFindResult struct {
		ID string `json:"id"`
	}
	var output struct {
		Results []dynamicFindResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(payload), &output); err == nil && len(output.Results) > 0 {
		return slices.ContainsFunc(output.Results, func(result dynamicFindResult) bool {
			return result.ID == expectedAction
		})
	}
	return strings.Contains(payload, "`"+expectedAction+"`") || strings.Contains(payload, `"id":"`+expectedAction+`"`)
}

func dynamicFindExchangeIncludesAction(exchange *traceMCPExchange, expectedAction string) bool {
	if exchange == nil || len(exchange.Response) == 0 {
		return false
	}
	type dynamicFindResult struct {
		ID string `json:"id"`
	}
	var output struct {
		StructuredContent struct {
			Results []dynamicFindResult `json:"results"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(exchange.Response, &output); err != nil {
		return false
	}
	return slices.ContainsFunc(output.StructuredContent.Results, func(result dynamicFindResult) bool {
		return result.ID == expectedAction
	})
}

func handleNoToolUseResult(result *taskResult, state *modelEvaluationState, repairLimit int, steps []evalStep) bool {
	result.Notes = append(result.Notes, "model returned no tool_use block")
	if state.firstFinalAttempt {
		result.FirstPass = false
		state.firstFinalAttempt = false
	}
	if state.repairCount >= repairLimit {
		return true
	}
	result.RepairAttempted = true
	state.repairCount++
	repairMessage := noToolUseRepairMessage(state.stepIndex, steps)
	state.messages = append(state.messages, modelMessage{Role: "user", Content: []modelContentBlock{{Type: "text", Text: repairMessage}}})
	result.Trace.Events = append(result.Trace.Events, traceEvent{Turn: result.ModelCalls, Kind: "repair_prompt", Role: "user", Content: repairMessage})
	return false
}

func (r *modelRunner) handleExpectedCapabilityBridgeStep(ctx context.Context, bridgeCtx capabilityBridgeStepContext) bool {
	step := bridgeCtx.steps[bridgeCtx.state.stepIndex]
	validation := validateStepCallWithRoutes(step, bridgeCtx.toolUse.Name, bridgeCtx.toolUse.Input, bridgeCtx.routes)
	if !validation.Valid {
		var ok bool
		step, validation, ok = bridgeCtx.acceptOptionalBridgeStep(step, validation)
		if ok {
			bridgeCtx.result.Notes = append(bridgeCtx.result.Notes, fmt.Sprintf("accepted optional %s skip before %s", bridgeCtx.steps[bridgeCtx.state.stepIndex-1].ExpectedTool, bridgeCtx.toolUse.Name))
		}
	}
	recordStepAssertionResults(bridgeCtx.result, step, validation, bridgeCtx.state.stepIndex+1)
	bridgeCtx.result.Trace.Events = append(bridgeCtx.result.Trace.Events, traceValidationEvent(bridgeCtx.result.ModelCalls, validation))
	recordValidationAttempt(bridgeCtx.result, bridgeCtx.toolUse, validation, bridgeCtx.state)
	if !validation.Valid {
		return handleInvalidCapabilityBridgeCall(bridgeCtx, step, validation)
	}
	bridgeCtx.state.lastInvalidFingerprint = ""
	recordCapabilityBridgeMetrics(bridgeCtx.result, bridgeCtx.toolUse)
	resourceResult := r.capabilityBridgeResult(ctx, bridgeCtx.toolUse)
	block := toolResultBlock(bridgeCtx.toolUse.ID, resourceResult.Content, resourceResult.Err)
	*bridgeCtx.followups = append(*bridgeCtx.followups, block)
	bridgeCtx.result.Trace.Events = append(bridgeCtx.result.Trace.Events, traceToolResultEventWithMCP(bridgeCtx.result.ModelCalls, block, resourceResult.MCP))
	if resourceResult.Err != nil {
		bridgeCtx.result.Notes = append(bridgeCtx.result.Notes, resourceResult.Err.Error())
		return false
	}
	bridgeCtx.state.stepIndex++
	bridgeCtx.result.CompletedSteps = bridgeCtx.state.stepIndex
	if bridgeCtx.state.repairCount > 0 {
		bridgeCtx.result.RepairSuccess = true
	}
	if bridgeCtx.state.stepIndex == len(bridgeCtx.steps) {
		bridgeCtx.result.FinalSuccess = true
		return true
	}
	return false
}

func (bridgeCtx capabilityBridgeStepContext) acceptOptionalBridgeStep(step evalStep, validation validationResult) (evalStep, validationResult, bool) {
	if !step.OptionalStep || validation.Valid {
		return step, validation, false
	}
	nextIndex := bridgeCtx.state.stepIndex + 1
	if nextIndex >= len(bridgeCtx.steps) || !expectedCapabilityBridgeStep(bridgeCtx.steps[nextIndex]) {
		return step, validation, false
	}
	nextStep := bridgeCtx.steps[nextIndex]
	nextValidation := validateStepCallWithRoutes(nextStep, bridgeCtx.toolUse.Name, bridgeCtx.toolUse.Input, bridgeCtx.routes)
	if !nextValidation.Valid {
		return step, validation, false
	}
	bridgeCtx.state.stepIndex = nextIndex
	bridgeCtx.result.CompletedSteps = bridgeCtx.state.stepIndex
	return nextStep, nextValidation, true
}

func handleInvalidCapabilityBridgeCall(bridgeCtx capabilityBridgeStepContext, step evalStep, validation validationResult) bool {
	if recordInvalidToolUse(bridgeCtx.result, bridgeCtx.state.stepIndex, validation, bridgeCtx.toolUse, bridgeCtx.state) {
		return true
	}
	if bridgeCtx.repairAlreadySent {
		return true
	}
	bridgeCtx.result.RepairAttempted = true
	bridgeCtx.state.repairCount++
	repairMessage := validationRepairMessage(bridgeCtx.task, step, validation, bridgeCtx.toolUse.Input)
	block := toolResultBlock(bridgeCtx.toolUse.ID, repairMessage, errors.New(repairMessage))
	*bridgeCtx.followups = append(*bridgeCtx.followups, block)
	bridgeCtx.result.Trace.Events = append(bridgeCtx.result.Trace.Events, traceToolResultEvent(bridgeCtx.result.ModelCalls, block))
	return false
}

func recordValidationAttempt(result *taskResult, toolUse modelContentBlock, validation validationResult, state *modelEvaluationState) {
	if state.firstFinalAttempt {
		result.FirstTool = toolUse.Name
		result.FirstAction = validation.Action
		result.FirstPass = validation.Valid
		state.firstFinalAttempt = false
	}
	result.FinalTool = toolUse.Name
	result.FinalAction = validation.Action
	result.DestructiveSafe = result.DestructiveSafe && validation.DestructiveSafe
}

func appendDynamicPreludeFollowup(steps []evalStep, stepIndex int, toolUse modelContentBlock, validation validationResult, result *taskResult, state *modelEvaluationState, followups *[]modelContentBlock) {
	if state.firstFinalAttempt {
		result.FirstTool = toolUse.Name
		result.FirstAction = validation.Action
		result.FirstPass = true
		state.firstFinalAttempt = false
	}
	result.FinalTool = toolUse.Name
	result.FinalAction = validation.Action
	block := toolResultBlock(toolUse.ID, successfulSimulatedToolContent(steps[stepIndex], toolUse, stepIndex+1, len(steps)), nil)
	*followups = append(*followups, block)
	result.Trace.Events = append(result.Trace.Events, traceToolResultEvent(result.ModelCalls, block))
}

func (r *modelRunner) appendCapabilityBridgeFollowup(ctx context.Context, toolUse modelContentBlock, result *taskResult, followups *[]modelContentBlock) {
	recordCapabilityBridgeMetrics(result, toolUse)
	resourceResult := r.capabilityBridgeResult(ctx, toolUse)
	block := toolResultBlock(toolUse.ID, resourceResult.Content, resourceResult.Err)
	*followups = append(*followups, block)
	result.Trace.Events = append(result.Trace.Events, traceToolResultEventWithMCP(result.ModelCalls, block, resourceResult.MCP))
	if resourceResult.Err != nil {
		result.Notes = append(result.Notes, resourceResult.Err.Error())
	}
}

func (r *modelRunner) appendLookupFollowup(ctx context.Context, lookupCtx lookupFollowupContext) {
	lookupCtx.result.SchemaLookupUsed = true
	if lookupCtx.stepIndex < len(lookupCtx.steps) {
		if message, blocked := discoveryBudgetFeedback(lookupCtx.task, lookupCtx.steps[lookupCtx.stepIndex], lookupCtx.toolUse, lookupCtx.budget); blocked {
			block := toolResultBlock(lookupCtx.toolUse.ID, message, errors.New(message))
			*lookupCtx.followups = append(*lookupCtx.followups, block)
			lookupCtx.result.Trace.Events = append(lookupCtx.result.Trace.Events, traceToolResultEvent(lookupCtx.result.ModelCalls, block))
			return
		}
	}
	payload, exchange, lookupErr := r.lookupToolResult(ctx, lookupCtx.routes, lookupCtx.toolUse, lookupCtx.dynamic)
	block := toolResultBlock(lookupCtx.toolUse.ID, payload, lookupErr)
	*lookupCtx.followups = append(*lookupCtx.followups, block)
	if exchange != nil {
		lookupCtx.result.Trace.Events = append(lookupCtx.result.Trace.Events, traceToolResultEventWithMCP(lookupCtx.result.ModelCalls, block, exchange))
	} else {
		lookupCtx.result.Trace.Events = append(lookupCtx.result.Trace.Events, traceToolResultEvent(lookupCtx.result.ModelCalls, block))
	}
	if lookupErr != nil {
		lookupCtx.result.Notes = append(lookupCtx.result.Notes, lookupErr.Error())
	}
}

func (r *modelRunner) lookupToolResult(ctx context.Context, routes map[string]toolutil.ActionMap, toolUse modelContentBlock, dynamic bool) (string, *traceMCPExchange, error) {
	if dynamic && toolUse.Name == dynamicFindTool && r.mcpSession != nil {
		result := r.mcpToolResult(ctx, toolUse)
		return result.Content, result.MCP, result.Err
	}
	payload, err := lookupToolResult(ctx, routes, toolUse, dynamic)
	return payload, nil, err
}

func lookupToolResult(ctx context.Context, routes map[string]toolutil.ActionMap, toolUse modelContentBlock, dynamic bool) (string, error) {
	if dynamic {
		return dynamicDiscoveryResult(ctx, routes, toolUse)
	}
	return schemaLookupResult(routes, toolUse.Input)
}

func recordInvalidToolUse(result *taskResult, stepIndex int, validation validationResult, toolUse modelContentBlock, state *modelEvaluationState) bool {
	invalidFingerprint := invalidToolUseFingerprint(toolUse)
	if invalidFingerprint != "" && invalidFingerprint == state.lastInvalidFingerprint {
		result.Notes = append(result.Notes, fmt.Sprintf("step %d repeated invalid retry: %s", stepIndex+1, validation.Message))
		return true
	}
	state.lastInvalidFingerprint = invalidFingerprint
	result.Notes = append(result.Notes, fmt.Sprintf("step %d: %s", stepIndex+1, validation.Message))
	return false
}

// noToolUseRepairMessage builds no tool use repair message for retry and repair feedback.
func noToolUseRepairMessage(stepIndex int, steps []evalStep) string {
	if stepIndex < 0 || stepIndex >= len(steps) {
		return "The previous response did not call an MCP tool. Continue by calling the next required tool now; do not answer in prose."
	}
	step := steps[stepIndex]
	if step.ExpectedAction == "" {
		return fmt.Sprintf("The previous response did not call an MCP tool. Continue by calling %s now; do not answer in prose.", step.ExpectedTool)
	}
	return fmt.Sprintf("The previous response did not call an MCP tool. Continue by calling %s with action %s now; do not answer in prose.", step.ExpectedTool, step.ExpectedAction)
}

// canExecuteInvalidToolCall reports whether the *modelRunner satisfies the can execute invalid tool call condition.
func (r *modelRunner) canExecuteInvalidToolCall(step evalStep, validation validationResult, toolUse modelContentBlock, routes map[string]toolutil.ActionMap) bool {
	if r.mcpSession == nil || step.Simulation != "" {
		return false
	}
	route, ok := routes[toolUse.Name][validation.Action]
	if !ok || route.Destructive {
		return false
	}
	if strings.Contains(validation.Message, diagnosticUnknownParams) {
		return false
	}
	if !validation.ToolMatches && validation.ActionMatches && strings.Contains(validation.Message, diagnosticMissingRequiredParams) {
		return false
	}
	if toolUse.Name == dynamicExecuteActionTool {
		if _, hasParams := toolUse.Input["params"]; !hasParams {
			return false
		}
		if !validation.ActionMatches {
			return false
		}
		if strings.Contains(validation.Message, diagnosticMissingRequiredParams) {
			return false
		}
	}
	if (!validation.ToolMatches || !validation.ActionMatches) && !isReadOnlyUnexpectedAction(validation.Action) {
		return false
	}
	return validation.DestructiveSafe
}

// isReadOnlyUnexpectedAction reports whether an unexpected action is harmlessly read-only.
func isReadOnlyUnexpectedAction(action string) bool {
	leaf := action
	if dot := strings.LastIndex(action, "."); dot >= 0 {
		leaf = action[dot+1:]
	}
	switch leaf {
	case "current", "health_check", "schema_get", "schema_index", "trace", "content", "raw", "projects", "groups", "users", "issues", "merge_requests", "commits":
		return true
	}
	return leaf == "get" || leaf == "list" || strings.HasPrefix(leaf, "get_") || strings.HasPrefix(leaf, "list_") || strings.HasSuffix(leaf, "_get") || strings.HasSuffix(leaf, "_list")
}

// invalidToolUseFingerprint resolves invalid tool use fingerprint for evaluator execution.
func invalidToolUseFingerprint(toolUse modelContentBlock) string {
	data, err := json.Marshal(struct {
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	}{Name: toolUse.Name, Input: toolUse.Input})
	if err != nil {
		return toolUse.Name
	}
	return string(data)
}

// taskToolCallLimit resolves task tool call limit for evaluator execution.
func taskToolCallLimit(stepCount int) int {
	limit := stepCount*3 + 4
	if limit < toolCallLimit {
		return toolCallLimit
	}
	return limit
}

// taskToolCallLimitForSurface resolves task tool call limit for surface for evaluator execution.
func taskToolCallLimitForSurface(stepCount int, _ string) int {
	return taskToolCallLimit(stepCount)
}

// taskCallBudget captures task call budget data for one evaluation task.
type taskCallBudget struct {
	ExpectedSteps         int
	AllowedDiscoveryCalls int
	AllowedRepairCalls    int
	MaxCalls              int
	SuppressDiscovery     bool
}

// callBudgetForTask calculates the maximum discovery, repair, and execution calls for a task.
func callBudgetForTask(task evalTask, toolSurface string) taskCallBudget {
	steps := taskSteps(task)
	budget := taskCallBudget{
		ExpectedSteps:         len(steps),
		AllowedDiscoveryCalls: 0,
		AllowedRepairCalls:    repairAttemptLimitForTask(toolSurface, len(steps)),
	}
	budget.MaxCalls = max(budget.ExpectedSteps, budget.ExpectedSteps+budget.AllowedDiscoveryCalls+budget.AllowedRepairCalls)
	return budget
}

// discoveryBudgetFeedback handles discovery budget feedback and returns [string].
func discoveryBudgetFeedback(task evalTask, step evalStep, toolUse modelContentBlock, budget taskCallBudget) (string, bool) {
	if !budget.SuppressDiscovery || !isRedundantDiscoveryTool(toolUse.Name) {
		return "", false
	}
	if !exactDynamicCallAvailable(task, []evalStep{step}) {
		return "", false
	}
	return fmt.Sprintf("The exact gitlab_execute_action call is already complete: action %s has high-confidence values for all required params. Execute it directly now; no discovery or schema lookup is needed.", step.ExpectedAction), true
}

// isRedundantDiscoveryTool reports whether a tool call repeats avoidable dynamic discovery.
func isRedundantDiscoveryTool(toolName string) bool {
	switch toolName {
	case dynamicFindTool, "gitlab", "gitlab_server":
		return true
	default:
		return false
	}
}

// exactDynamicCallAvailable reports whether the prompt already proves one safe dynamic call.
func exactDynamicCallAvailable(task evalTask, steps []evalStep) bool {
	if len(steps) != 1 {
		return false
	}
	step := steps[0]
	if step.ExpectedTool != dynamicExecuteActionTool || step.ExpectedAction == "" {
		return false
	}
	_, provenances := exactCallParams(step, task.Prompt, false)
	return exactCallParamsAreSafe(provenances)
}

// repairAttemptLimitForSurface returns the default repair budget for one tool surface.
func repairAttemptLimitForSurface(_ string) int {
	return 1
}

// repairAttemptLimitForTask scales the repair budget for multi-step dynamic tasks.
func repairAttemptLimitForTask(toolSurface string, _ int) int {
	return repairAttemptLimitForSurface(toolSurface)
}

// acceptsDynamicPreludeCall accepts discovery calls that correctly precede a dynamic execute call.
func acceptsDynamicPreludeCall(_ string, _ evalStep, _ validationResult) bool {
	return false
}

// successfulSimulatedToolContent resolves successful simulated tool content for evaluator execution.
func successfulSimulatedToolContent(step evalStep, toolUse modelContentBlock, nextStep, totalSteps int) string {
	result := map[string]any{"ok": true, "next_step": nextStep, "total_steps": totalSteps}
	action, _ := toolUse.Input["action"].(string)
	if step.ExpectedAction != "" {
		action = toolutil.NormalizeActionAlias(action, toolutil.ActionMap{step.ExpectedAction: {}})
	}
	params, _ := toolUse.Input["params"].(map[string]any)
	addSimulatedResourceIDs(result, action, params)
	populateSimulatedToolResult(result, step, toolUse, action, params)
	addProducedValues(result, step, params)
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("ok; continue with step %d of %d", nextStep, totalSteps)
	}
	return string(data)
}

func addProducedValues(result map[string]any, step evalStep, params map[string]any) {
	if len(step.ProducedValues) == 0 {
		return
	}
	produced := map[string]any{}
	for _, key := range step.ProducedValues {
		if value, ok := result[key]; ok {
			produced[key] = value
			continue
		}
		if value, ok := params[key]; ok {
			produced[key] = value
		}
	}
	if len(produced) > 0 {
		result["produced_values"] = produced
	}
}

func populateSimulatedToolResult(result map[string]any, step evalStep, toolUse modelContentBlock, action string, params map[string]any) {
	switch {
	case toolUse.Name == "gitlab_discover_project" || action == actionDiscoverProjectResolve:
		remoteURL, _ := toolUse.Input["remote_url"].(string)
		if remoteURL == "" {
			remoteURL, _ = params["remote_url"].(string)
		}
		projectPath := projectPathFromRemoteURL(remoteURL)
		result["project"] = map[string]any{
			"id":                  42,
			"path_with_namespace": projectPath,
			"project_id":          projectPath,
			"name":                projectNameFromPath(projectPath),
			"default_branch":      "main",
			"web_url":             strings.TrimSuffix(remoteURL, ".git"),
		}
	case step.ExpectedAction == actionDiscoverProjectResolve && (action == actionSearchProjects || action == actionProjectList || action == actionProjectGet):
		project := simulatedProjectFromLookup(toolUse.Input, params)
		result["project"] = project
		result["projects"] = []map[string]any{project}
		result["environments"] = []map[string]any{{
			"id":             122,
			"name":           "production",
			"environment":    "production",
			"project_id":     project["project_id"],
			"project_path":   project["path_with_namespace"],
			"default_branch": project["default_branch"],
		}}
	default:
		if applyActionSimulation(result, toolUse, action, params) {
			return
		}
		result["expected_tool"] = step.ExpectedTool
		result["expected_action"] = step.ExpectedAction
	}
}

func applyActionSimulation(result map[string]any, toolUse modelContentBlock, action string, params map[string]any) bool {
	switch action {
	case actionProjectGet:
		projectID, _ := params["project_id"].(string)
		result["project"] = map[string]any{
			"id":                  42,
			"path_with_namespace": projectID,
			"project_id":          projectID,
			"name":                projectNameFromPath(projectID),
			"default_branch":      "main",
		}
		return true
	case actionProjectList:
		result["projects"] = []map[string]any{simulatedProjectFromLookup(toolUse.Input, params)}
		return true
	case "pipeline.trigger_list":
		result["triggers"] = []map[string]any{{
			"id":          119,
			"trigger_id":  119,
			"project_id":  params["project_id"],
			"description": "eval-crud-trigger",
		}}
		return true
	case "package.publish_directory", "publish_directory":
		result["published"] = simulatedPackageDirectoryItems(params)
		result["total_files"] = len(packageReleaseFixtureFiles)
		return true
	case "group.group_label_list":
		result["labels"] = []map[string]any{{
			"id":       120,
			"label_id": 120,
			"group_id": params["group_id"],
			"name":     "eval-group-label",
		}}
		return true
	case "wiki.list":
		result["pages"] = []map[string]any{{
			"slug":       "eval-wiki-page",
			"title":      "Evaluation wiki page",
			"project_id": params["project_id"],
		}}
		return true
	case "release.get":
		result["release"] = map[string]any{
			"project_id": params["project_id"],
			"tag_name":   params["tag_name"],
			"assets": map[string]any{
				"links": []map[string]any{{
					"id":      121,
					"link_id": 121,
				}},
			},
		}
		return true
	case "environment.list":
		result["environments"] = []map[string]any{{
			"id":          122,
			"name":        "production",
			"environment": "production",
			"project_id":  params["project_id"],
		}}
		return true
	case actionEnvironmentProtectedList:
		result["protected_environments"] = []map[string]any{{
			"name":        "production",
			"environment": "production",
			"project_id":  params["project_id"],
		}}
		return true
	case "repository.file_get":
		result["file"] = map[string]any{
			"project_id": params["project_id"],
			"file_path":  params["file_path"],
			"ref":        params["ref"],
			"encoding":   "base64",
		}
		return true
	case actionPipelineGet:
		result["pipeline"] = map[string]any{
			"project_id":  params["project_id"],
			"pipeline_id": params["pipeline_id"],
			"status":      "success",
		}
		return true
	default:
		return false
	}
}

// simulatedPackageDirectoryItems returns package file URLs for simulated package-directory publishes.
func simulatedPackageDirectoryItems(params map[string]any) []map[string]any {
	projectID, _ := params["project_id"].(string)
	if projectID == "" {
		projectID = liveFixtureProjectPath
	}
	packageName, _ := params["package_name"].(string)
	if packageName == "" {
		packageName = liveFixturePackageReleaseName
	}
	packageVersion, _ := params["package_version"].(string)
	if packageVersion == "" {
		packageVersion = liveFixturePackageReleaseVersion
	}
	baseURL := fmt.Sprintf("https://gitlab.example.com/api/v4/projects/%s/packages/generic/%s/%s",
		url.PathEscape(projectID), url.PathEscape(packageName), url.PathEscape(packageVersion))
	items := make([]map[string]any, 0, len(packageReleaseFixtureFiles))
	for index, file := range packageReleaseFixtureFiles {
		items = append(items, map[string]any{
			"file_name":       file.name,
			"package_file_id": 200 + index,
			"url":             baseURL + "/" + url.PathEscape(file.name),
		})
	}
	return items
}

// simulatedProjectFromLookup maps simulated project from lookup between API and evaluator models.
func simulatedProjectFromLookup(input, params map[string]any) map[string]any {
	projectPath := simulatedProjectPath(input, params)
	return map[string]any{
		"id":                  42,
		"name":                projectNameFromPath(projectPath),
		"path":                projectNameFromPath(projectPath),
		"path_with_namespace": projectPath,
		"project_id":          projectPath,
		"default_branch":      "main",
		"web_url":             "https://gitlab.example.com/" + projectPath,
	}
}

// simulatedProjectPath returns the simulated project path used by evaluator requests.
func simulatedProjectPath(input, params map[string]any) string {
	for _, source := range []map[string]any{params, input} {
		for _, key := range []string{"project_id", "path_with_namespace", "full_path", "remote_url", "search", "query"} {
			if value, ok := source[key].(string); ok && strings.TrimSpace(value) != "" {
				candidate := strings.TrimSpace(value)
				if key == "remote_url" || strings.Contains(candidate, "://") || strings.HasSuffix(candidate, ".git") {
					return projectPathFromRemoteURL(candidate)
				}
				if strings.Contains(candidate, "/") {
					return candidate
				}
			}
		}
	}
	return liveFixtureProjectPath
}

// projectNameFromPath returns the project name from path used by evaluator requests.
func projectNameFromPath(projectPath string) string {
	projectPath = strings.Trim(projectPath, "/")
	if projectPath == "" {
		return "gitlab-mcp-server"
	}
	if slash := strings.LastIndex(projectPath, "/"); slash >= 0 {
		return projectPath[slash+1:]
	}
	return projectPath
}

// addSimulatedResourceIDs injects stable IDs into simulated tool results for downstream steps.
func addSimulatedResourceIDs(result map[string]any, action string, params map[string]any) {
	switch action {
	case actionIssueCreate:
		addTopLevelID(result, "issue_iid", 123)
		result["issue"] = map[string]any{"id": 123, "iid": 123, "issue_iid": 123, "project_id": params["project_id"]}
	case actionIssueLinkCreate:
		addTopLevelID(result, "issue_link_id", 124)
		result["issue_link"] = map[string]any{"id": 124, "issue_link_id": 124, "project_id": params["project_id"], "issue_iid": params["issue_iid"]}
	case "pipeline.trigger_create":
		addTopLevelID(result, "trigger_id", 119)
		result["trigger"] = map[string]any{"id": 119, "trigger_id": 119, "project_id": params["project_id"]}
	case "release.link_create":
		addTopLevelID(result, "link_id", 121)
		result["link"] = map[string]any{"id": 121, "link_id": 121, "project_id": params["project_id"], "tag_name": params["tag_name"]}
	case "group.group_label_create":
		addTopLevelID(result, "label_id", 120)
		result["label"] = map[string]any{"id": 120, "label_id": 120, "group_id": params["group_id"]}
	case "wiki.create":
		slug, _ := params["slug"].(string)
		if slug == "" {
			slug = "eval-wiki-page"
		}
		result["wiki"] = map[string]any{"slug": slug, "project_id": params["project_id"]}
	case "admin.broadcast_message_create":
		addTopLevelID(result, "id", 125)
		result["broadcast_message"] = map[string]any{"id": 125}
	case "project.hook_add":
		addTopLevelID(result, "hook_id", 101)
		result["hook"] = map[string]any{"id": 101, "hook_id": 101, "project_id": params["project_id"]}
	case "project.badge_add":
		addTopLevelID(result, "badge_id", 102)
		result["badge"] = map[string]any{"id": 102, "badge_id": 102, "project_id": params["project_id"]}
	case "snippet.project_create":
		addTopLevelID(result, "snippet_id", 103)
		filePath := snippetFilePathFromParams(params)
		result["snippet"] = map[string]any{"id": 103, "snippet_id": 103, "project_id": params["project_id"], "file_path": filePath, "file_name": filePath}
	case "mr_review.note_create", "mr_review.draft_note_create":
		addTopLevelID(result, "note_id", 104)
		result["note"] = map[string]any{"id": 104, "note_id": 104, "project_id": params["project_id"], "merge_request_iid": params["merge_request_iid"]}
	case "access.deploy_token_create_project":
		addTopLevelID(result, "deploy_token_id", 105)
		result["deploy_token"] = map[string]any{"id": 105, "deploy_token_id": 105, "project_id": params["project_id"]}
	case "access.deploy_key_add":
		addTopLevelID(result, "deploy_key_id", 106)
		result["deploy_key"] = map[string]any{"id": 106, "deploy_key_id": 106, "project_id": params["project_id"]}
	case "project.member_add":
		addTopLevelID(result, "user_id", 107)
		result["member"] = map[string]any{"id": 107, "user_id": 107, "project_id": params["project_id"]}
	case "group.group_milestone_create":
		addTopLevelID(result, "milestone_iid", 108)
		result["milestone"] = map[string]any{"id": 108, "milestone_iid": 108, "group_id": params["group_id"]}
	case "pipeline.schedule_create":
		addTopLevelID(result, "schedule_id", 109)
		result["schedule"] = map[string]any{"id": 109, "schedule_id": 109, "project_id": params["project_id"]}
	case "merge_request.emoji_mr_create":
		addTopLevelID(result, "award_id", 110)
		result["award"] = map[string]any{"id": 110, "award_id": 110, "project_id": params["project_id"], "merge_request_iid": params["merge_request_iid"]}
	}
}

// addTopLevelID adds top level ID for the evaluator package.
func addTopLevelID(result map[string]any, name string, id int) {
	result["id"] = id
	result[name] = id
}

// snippetFilePathFromParams returns the snippet file path from params used by evaluator requests.
func snippetFilePathFromParams(params map[string]any) string {
	if fileName, ok := params["file_name"].(string); ok && fileName != "" {
		return fileName
	}
	files, _ := params["files"].([]any)
	for _, file := range files {
		object, _ := file.(map[string]any)
		if filePath, ok := object["file_path"].(string); ok && filePath != "" {
			return filePath
		}
	}
	return "snippet.txt"
}

// projectPathFromRemoteURL returns the project path from remote URL used by evaluator requests.
func projectPathFromRemoteURL(remoteURL string) string {
	withoutSuffix := strings.TrimSuffix(remoteURL, ".git")
	if _, withoutScheme, found := strings.Cut(withoutSuffix, "://"); found {
		if _, path, hasSlash := strings.Cut(withoutScheme, "/"); hasSlash {
			return path
		}
	}
	if colon := strings.LastIndex(withoutSuffix, ":"); colon >= 0 {
		return withoutSuffix[colon+1:]
	}
	return withoutSuffix
}

// validatedToolResult handles validated tool result for modelRunner.
func (r *modelRunner) validatedToolResult(ctx context.Context, step evalStep, toolUse modelContentBlock, attempt, stepNumber, totalSteps int) simulationResult {
	if step.Simulation != "" || r.mcpSession == nil {
		return simulatedToolResult(step, attempt, stepNumber, totalSteps)
	}
	return r.mcpToolResult(ctx, toolUse)
}

// mcpToolResult handles MCP tool result for modelRunner.
func (r *modelRunner) mcpToolResult(ctx context.Context, toolUse modelContentBlock) simulationResult {
	var last simulationResult
	for attempt := range mcpToolTransientAttempts {
		last = r.mcpToolResultOnce(ctx, toolUse)
		if !isTransientMCPToolResult(last) {
			return last
		}
		if attempt == mcpToolTransientAttempts-1 {
			return last
		}
		if err := waitForContext(ctx, mcpToolTransientRetryWait); err != nil {
			return simulationResult{Content: fmt.Sprintf("MCP tool call retry interrupted: %s", err), Injected: true, Err: err}
		}
	}
	return last
}

func (r *modelRunner) mcpToolResultOnce(ctx context.Context, toolUse modelContentBlock) simulationResult {
	callCtx, cancel := context.WithTimeout(ctx, mcpToolCallTimeout)
	defer cancel()
	exchange := &traceMCPExchange{Request: traceMCPRequest{Name: toolUse.Name, Arguments: toolUse.Input}}
	started := time.Now()
	result, err := r.mcpSession.CallTool(callCtx, &mcp.CallToolParams{
		Name:      toolUse.Name,
		Arguments: toolUse.Input,
	})
	exchange.DurationMillis = time.Since(started).Milliseconds()
	if err != nil {
		exchange.ProtocolError = err.Error()
		return simulationResult{Content: fmt.Sprintf("MCP tool call failed: %s", err), Injected: true, Err: err, MCP: exchange}
	}
	content := toolResultContentForTool(toolUse.Name, result)
	exchange.setResponse(result)
	if result == nil {
		emptyResultErr := errors.New("MCP tool call returned an empty result")
		return simulationResult{Content: content, Injected: true, Err: emptyResultErr, MCP: exchange}
	}
	if result.IsError {
		return simulationResult{Content: content, Injected: true, Err: errors.New(content), MCP: exchange}
	}
	return simulationResult{Content: content, Advance: true, Injected: true, MCP: exchange}
}

func isTransientMCPToolResult(result simulationResult) bool {
	if result.Err == nil {
		return false
	}
	text := strings.ToLower(result.Content + " " + result.Err.Error())
	return strings.Contains(text, "packed-refs locked") || strings.Contains(text, "reference update failed") || strings.Contains(text, "failed to remove tag")
}

// capabilityBridgeResult handles evaluator MCP client capability bridge calls.

func toolExecutionNote(stepNumber int, step evalStep, err error) string {
	if step.Simulation != "" {
		return fmt.Sprintf("step %d simulation %s: %s", stepNumber, step.Simulation, err.Error())
	}
	message := fmt.Sprintf("step %d MCP execution: %s", stepNumber, err.Error())
	payload := repairPayloadForExecutionError(step, err, message)
	data, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return message
	}
	return string(data)
}

// repairPayloadForExecutionError builds repair payload for execution error for retry and repair feedback.
func repairPayloadForExecutionError(step evalStep, err error, message string) repairPayload {
	return repairPayload{
		ErrorKind:    executionErrorKind(step, err),
		FailedAction: step.ExpectedAction,
		BadParam:     executionErrorBadParam(step, err),
		ExpectedType: "GitLab API request accepted by the selected action",
		LikelyFix:    strings.TrimSpace(message + roleSensitiveRepairHint(step)),
		RetryAllowed: true,
		Message:      message,
	}
}

// executionErrorKind classifies a failed live execution for repair feedback.
func executionErrorKind(step evalStep, err error) string {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "400") && roleSensitiveRepairHint(step) != "":
		return "gitlab_bad_request_role_confusion"
	case strings.Contains(text, "400") || strings.Contains(text, "bad request"):
		return "gitlab_bad_request"
	case strings.Contains(text, "404") || strings.Contains(text, "not found"):
		return "gitlab_not_found"
	case strings.Contains(text, "403") || strings.Contains(text, "forbidden"):
		return "gitlab_forbidden"
	default:
		return "mcp_execution_error"
	}
}

// executionErrorBadParam derives execution error bad param from task and schema inputs.
func executionErrorBadParam(step evalStep, err error) string {
	if executionErrorKind(step, err) != "gitlab_bad_request_role_confusion" {
		return ""
	}
	switch step.ExpectedAction {
	case "job.token_scope_remove_project":
		return "project_id,target_project_id"
	case actionIssueLinkCreate:
		return "project_id,issue_iid,target_project_id,target_issue_iid"
	case "merge_request.create":
		return "source_branch,target_branch"
	default:
		return ""
	}
}

// newTaskTrace constructs task trace.
func newTaskTrace(task evalTask, systemPrompt, userPrompt string) taskTrace {
	steps := taskSteps(task)
	expected := make([]traceExpectedStep, 0, len(steps))
	for i, step := range steps {
		expected = append(expected, traceExpectedStep{
			Step:           i + 1,
			Tool:           step.ExpectedTool,
			Action:         step.ExpectedAction,
			RequiredParams: slices.Clone(step.RequiredParams),
			OptionalParams: slices.Clone(step.OptionalParams),
			OptionalStep:   step.OptionalStep,
			Destructive:    step.Destructive,
			Simulation:     step.Simulation,
		})
	}
	return taskTrace{
		TaskID:       task.ID,
		Prompt:       task.Prompt,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Expected:     expected,
		Events: []traceEvent{{
			Turn:    0,
			Kind:    "user_prompt",
			Role:    "user",
			Content: userPrompt,
		}},
	}
}

// traceToolUseEvent retrieves the trace of tool use event for the evaluator package.
func traceToolUseEvent(turn int, toolUse modelContentBlock) traceEvent {
	action, _ := toolUse.Input["action"].(string)
	return traceEvent{
		Turn:      turn,
		Kind:      "tool_use",
		Role:      "assistant",
		ToolUseID: toolUse.ID,
		Tool:      toolUse.Name,
		Action:    action,
		Input:     toolUse.Input,
		RawInput:  toolUse.ProviderRawInput,
	}
}

// traceValidationEvent retrieves the trace of validation event for the evaluator package.
func traceValidationEvent(turn int, validation validationResult) traceEvent {
	return traceEvent{
		Turn: turn,
		Kind: "validation",
		Validation: &traceValidation{
			Valid:           validation.Valid,
			ToolMatches:     validation.ToolMatches,
			ActionMatches:   validation.ActionMatches,
			RequiredPresent: validation.RequiredPresent,
			DestructiveSafe: validation.DestructiveSafe,
			Message:         validation.Message,
		},
	}
}

// traceToolResultEvent retrieves the trace of tool result event for the evaluator package.
func traceToolResultEvent(turn int, block modelContentBlock) traceEvent {
	return traceEvent{
		Turn:      turn,
		Kind:      "tool_result",
		Role:      "user",
		ToolUseID: block.ToolUseID,
		Content:   block.Content,
		IsError:   block.IsError,
	}
}

// traceToolResultEventWithMCP records a tool result plus the underlying MCP exchange.
func traceToolResultEventWithMCP(turn int, block modelContentBlock, exchange *traceMCPExchange) traceEvent {
	event := traceToolResultEvent(turn, block)
	event.MCP = exchange
	return event
}

// setResponse records a complete MCP result when it can be represented as JSON.
func (e *traceMCPExchange) setResponse(result *mcp.CallToolResult) {
	if e == nil || result == nil {
		return
	}
	e.IsError = result.IsError
	data, err := json.Marshal(result)
	if err != nil {
		e.ResponseText = fmt.Sprintf("marshal MCP result: %s", err)
		return
	}
	e.Response = append(json.RawMessage(nil), data...)
}

// traceSummaryFromResult retrieves the trace of summary from result for the evaluator package.
func traceSummaryFromResult(result taskResult) traceSummary {
	return traceSummary{
		FirstTool:            result.FirstTool,
		FirstAction:          result.FirstAction,
		FinalTool:            result.FinalTool,
		FinalAction:          result.FinalAction,
		SchemaLookupUsed:     result.SchemaLookupUsed,
		ResourceLookupUsed:   result.ResourceLookupUsed,
		CapabilityLookupUsed: result.CapabilityLookupUsed,
		FirstPass:            result.FirstPass,
		RepairAttempted:      result.RepairAttempted,
		RepairSuccess:        result.RepairSuccess,
		FinalSuccess:         result.FinalSuccess,
		DestructiveSafe:      result.DestructiveSafe,
		CompletedSteps:       result.CompletedSteps,
		ExpectedSteps:        len(taskSteps(result.Task)),
		ModelCalls:           result.ModelCalls,
		ToolCalls:            result.ToolCalls,
		ResourceCalls:        result.ResourceCalls,
		CapabilityCalls:      result.CapabilityCalls,
		Notes:                strings.Join(result.Notes, "; "),
	}
}

// call handles call for modelRunner.
func (r *modelRunner) call(ctx context.Context, systemPrompt string, catalog []modelTool, messages []modelMessage) (modelResponse, error) {
	provider := modelProviderFor(r.provider)
	request := modelProviderRequest{
		Model:       r.model,
		MaxTokens:   r.maxTokens,
		Temperature: 0,
		System:      systemPrompt,
		Tools:       catalog,
		Messages:    messages,
		TraceBodies: r.traceBodies,
	}
	var lastErr error
	for attempt := 0; attempt <= r.retries; attempt++ {
		if attempt > 0 {
			if err := waitForContext(ctx, r.retryWait); err != nil {
				return modelResponse{}, err
			}
		}
		out, retry, callErr := provider.callOnce(ctx, r.client, r.apiKey, request)
		if callErr == nil {
			return out, nil
		}
		lastErr = callErr
		if !retry {
			break
		}
	}
	return modelResponse{}, lastErr
}

// redactResponse formats redact response for evaluator output.
func redactResponse(body []byte) string {
	text := string(body)
	if len(text) > 1000 {
		text = text[:1000] + "..."
	}
	return text
}

// toolUseBlocks converts the GitLab API response to the tool output format.
func toolUseBlocks(blocks []modelContentBlock) []modelContentBlock {
	out := make([]modelContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "tool_use" {
			out = append(out, block)
		}
	}
	return out
}

// isSchemaLookup reports whether a tool call asks the server for catalog schemas.
func isSchemaLookup(toolUse modelContentBlock) bool {
	if toolUse.Name != "gitlab_server" {
		return false
	}
	action, _ := toolUse.Input["action"].(string)
	return action == "schema_get" || action == "schema_index"
}

// isCapabilityBridge reports whether a tool call uses an MCP capability bridge.
func isCapabilityBridge(toolUse modelContentBlock) bool {
	return isCapabilityBridgeName(toolUse.Name)
}

func expectedCapabilityBridgeStep(step evalStep) bool {
	return step.ExpectedAction == "" && isCapabilityBridgeName(step.ExpectedTool)
}

func recordCapabilityBridgeMetrics(result *taskResult, toolUse modelContentBlock) {
	result.CapabilityLookupUsed = true
	result.CapabilityCalls++
	if isResourceLookup(toolUse) {
		result.ResourceLookupUsed = true
		result.ResourceCalls++
	}
}

func isCapabilityBridgeName(name string) bool {
	switch name {
	case capabilityListTool, resourceListTool, resourceReadTool, promptListTool, promptGetTool, completionTool:
		return true
	default:
		return false
	}
}

func isResourceLookup(toolUse modelContentBlock) bool {
	return toolUse.Name == resourceListTool || toolUse.Name == resourceReadTool
}

// isDynamicDiscovery reports whether a dynamic catalog lookup tool was called.
func isDynamicDiscovery(toolUse modelContentBlock) bool {
	return toolUse.Name == dynamicFindTool
}

// dynamicDiscoveryResult returns simulated discovery output for dynamic find
// calls, keeping evaluation independent from live GitLab state.
func dynamicDiscoveryResult(ctx context.Context, routes map[string]toolutil.ActionMap, toolUse modelContentBlock) (string, error) {
	switch toolUse.Name {
	case dynamicFindTool:
		query, _ := toolUse.Input["query"].(string)
		limit := intFromAny(toolUse.Input["limit"], 20)
		return marshalToolResult(dynamicFindResult(ctx, routes, query, limit))
	default:
		return "", fmt.Errorf("unsupported dynamic discovery tool %q", toolUse.Name)
	}
}

// dynamicFindResult searches and describes matches using the same runtime
// registry as the dynamic toolset.
func dynamicFindResult(ctx context.Context, routes map[string]toolutil.ActionMap, query string, limit int) any {
	registry := dynamictools.NewRegistry(dynamicCatalogRoutesFromValidationRoutes(routes))
	return dynamicFindResultWithRegistry(ctx, registry, query, limit)
}

func dynamicFindResultWithRegistry(ctx context.Context, registry *dynamictools.Registry, query string, limit int) any {
	_, output, err := registry.Find(ctx, nil, dynamictools.FindInput{Query: query, Limit: limit})
	if err != nil {
		return map[string]any{"query": query, "count": 0, "results": []any{}, "error": err.Error()}
	}
	return output
}

// dynamicCatalogRoutesFromValidationRoutes derives dynamic catalog routes from validation routes from catalog metadata.
func dynamicCatalogRoutesFromValidationRoutes(routes map[string]toolutil.ActionMap) map[string]toolutil.ActionMap {
	catalogRoutes := make(map[string]toolutil.ActionMap)
	for actionID, route := range routes[dynamicExecuteActionTool] {
		domain, action, ok := strings.Cut(actionID, ".")
		if !ok || domain == "" || action == "" {
			continue
		}
		toolName := "gitlab_" + domain
		if catalogRoutes[toolName] == nil {
			catalogRoutes[toolName] = make(toolutil.ActionMap)
		}
		catalogRoutes[toolName][action] = route
	}
	return catalogRoutes
}

// intFromAny converts JSON numeric values to int with a fallback default.
func intFromAny(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	}
	return fallback
}

// schemaLookupResult handles schema lookup result and returns [string].
func schemaLookupResult(routes map[string]toolutil.ActionMap, input map[string]any) (string, error) {
	action, _ := input["action"].(string)
	params, _ := input["params"].(map[string]any)
	switch action {
	case "schema_index":
		if tool, _ := params["tool"].(string); tool != "" {
			lookupRoutes, lookupTool, _ := schemaLookupAlias(routes, tool, "")
			index, ok := toolutil.BuildMetaSchemaDiscoveryIndexForTool(lookupRoutes, lookupTool)
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
			return marshalToolResult(schemaGetUsage())
		}
		if selectedAction == "" {
			lookupRoutes, lookupTool, _ := schemaLookupAlias(routes, tool, "")
			index, ok := toolutil.BuildMetaSchemaDiscoveryIndexForTool(lookupRoutes, lookupTool)
			if !ok {
				return "", fmt.Errorf("schema_get: unknown tool %q", tool)
			}
			return marshalToolResult(index)
		}
		lookupRoutes, lookupTool, lookupAction := schemaLookupAlias(routes, tool, selectedAction)
		schema, ok := toolutil.LookupMetaActionSchema(lookupRoutes, lookupTool, lookupAction)
		if !ok {
			return "", fmt.Errorf("schema_get: unknown action %q for tool %q", selectedAction, tool)
		}
		return marshalToolResult(schema)
	default:
		return "", fmt.Errorf("unsupported schema action %q", action)
	}
}

// schemaGetUsage returns examples for the evaluator's schema lookup tool.
func schemaGetUsage() map[string]any {
	return map[string]any{
		"message": "schema_get needs params.tool to return an exact action schema",
		"examples": []map[string]any{
			{
				"purpose": "unified dispatcher project lookup schema",
				"call":    map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab", "action": actionProjectGet}},
			},
			{
				"purpose": "unified dispatcher pipeline lookup schema",
				"call":    map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab", "action": actionPipelineGet}},
			},
			{
				"purpose": "legacy domain meta-tool schema",
				"call":    map[string]any{"action": "schema_get", "params": map[string]any{"tool": "gitlab_project", "action": "get"}},
			},
		},
		"index_call": map[string]any{"action": "schema_index", "params": map[string]any{"tool": "gitlab"}},
	}
}

// schemaLookupAlias maps legacy meta-tool schema requests onto unified catalog actions.
func schemaLookupAlias(routes map[string]toolutil.ActionMap, tool, action string) (lookupRoutes map[string]toolutil.ActionMap, lookupTool, lookupAction string) {
	superRoutes, hasSuperDispatcher := routes["gitlab"]
	if !hasSuperDispatcher || tool == "gitlab" || tool == "gitlab_server" || !strings.HasPrefix(tool, "gitlab_") {
		return routes, tool, action
	}

	domain := strings.TrimPrefix(tool, "gitlab_")
	if action != "" {
		superAction := domain + "." + action
		if _, ok := superRoutes[superAction]; ok {
			return routes, "gitlab", superAction
		}
		return routes, tool, action
	}

	prefix := domain + "."
	filtered := make(toolutil.ActionMap)
	for superAction, route := range superRoutes {
		if suffix, found := strings.CutPrefix(superAction, prefix); found {
			filtered[suffix] = route
		}
	}
	if len(filtered) == 0 {
		return routes, tool, action
	}
	return map[string]toolutil.ActionMap{tool: filtered}, tool, action
}

// marshalToolResult serializes simulated tool output as a JSON content string.
func marshalToolResult(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal tool result: %w", err)
	}
	return string(data), nil
}

// toolResultBlock converts the GitLab API response to the tool output format.
func toolResultBlock(toolUseID, content string, err error) modelContentBlock {
	block := modelContentBlock{Type: "tool_result", ToolUseID: toolUseID, Content: content}
	if err != nil {
		block.IsError = true
		if content == "" {
			block.Content = err.Error()
		}
	}
	return block
}

// systemPrompt builds system prompt for evaluator prompts.
