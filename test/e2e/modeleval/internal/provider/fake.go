//go:build e2e

// fake.go is the provider that talks to nobody.
//
// It exists to prove the pipe: a case is selected, a world is built, a call is
// sent through the real binary to a real GitLab, the answer comes back, the
// record is written and the scorer reads it. Everything on that path is under
// test except the model, and paying a provider to find out whether a fixture
// builds is a poor way to spend a budget.
//
// It is the one thing on the run path allowed to read the corpus key, and it is
// named as that exemption in the corpus's own boundary test. What keeps that
// safe is where it sits: a provider cannot write a prompt, cannot build a
// world and cannot change what a case is, so a key read here reaches nothing a
// stimulus is made of. Every row it produces is refused by the publisher, so no
// number it computes can be published as a model's.
//
// Four variants, because the pipe has four things worth proving and only one of
// them is that a correct model completes. The scorer is a machine for telling a
// value miss from a name miss, an omitted confirmation from a refused one, and
// a correct decline from a failure to act, and each of those readings needs a
// run that actually produces it.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// The variants, which are what the model half of a fake spec names.
const (
	// FakePerfect replays the key: every step, in order, with the arguments
	// the key says are right and a confirmation where one is needed.
	FakePerfect = "perfect"
	// FakeWrongArgs replays the key with one compared value replaced, so a
	// run proves the scorer reports a value miss on a call that dispatched
	// correctly. That distinction is the whole of finding F2: the evaluator
	// this replaces compared parameter names and read exactly one value.
	FakeWrongArgs = "wrong-args"
	// FakeNoConfirm replays the key and never approves a destructive action,
	// so a run proves the confirmation column measures something.
	FakeNoConfirm = "no-confirm"
	// FakeDeclines answers in text and calls nothing, which is the correct
	// behavior on a read-only surface and a failure everywhere else. A
	// read-only verification run by FakePerfect alone would prove the
	// opposite of what it set out to: that provider always calls.
	FakeDeclines = "declines"
)

// fakeVariants are the variants, in the order a refusal lists them.
func fakeVariants() []string {
	return []string{FakePerfect, FakeWrongArgs, FakeNoConfirm, FakeDeclines}
}

// plantedValue is what [FakeWrongArgs] writes instead of the right value.
//
// It is a word rather than a mutation of the right value because a scorer
// reports the value it found, and a reader of that report should be able to see
// at once that a fake put it there.
const plantedValue = "planted-by-the-fake-provider"

// authoredValue is what the fake writes where a case says the model authors the
// value. It is never compared, by [modelcorpus.Truth.Authored].
const authoredValue = "written by the fake provider"

// fakeAdapter replays a corpus key.
type fakeAdapter struct {
	spec    Spec
	variant string
}

// newFake builds one variant.
func newFake(spec Spec) (Provider, error) {
	if !slices.Contains(fakeVariants(), spec.Model) {
		return nil, fmt.Errorf("unknown fake variant %q, want one of %s",
			spec.Model, strings.Join(fakeVariants(), ", "))
	}
	return fakeAdapter{spec: spec, variant: spec.Model}, nil
}

// Spec returns what this adapter was configured from.
func (a fakeAdapter) Spec() Spec { return a.spec }

// ToolDigest hashes the tool list as it arrived.
//
// The fake sends nothing, so what it hashes is the list itself. That is the
// right answer rather than an empty string: a fake run's session line then
// carries the digest the real providers would carry, which is what makes the
// digest comparison across a fake run and a paid one mean anything.
func (a fakeAdapter) ToolDigest(tools []Tool) string {
	entries := make([]toolEntry, 0, len(tools))
	for _, tool := range tools {
		entries = append(entries, toolEntry(tool))
	}
	return digestTools(entries)
}

// Call answers one turn.
//
// Which step it is on is read from the conversation rather than from a counter
// of its own, because two attempts may share one adapter and a counter would
// make them interfere. The conversation is the state.
func (a fakeAdapter) Call(_ context.Context, request Request) (Response, error) {
	answer := Response{Status: modelrecord.TurnOK, Usage: modelrecord.Usage{}}
	if a.variant == FakeDeclines {
		answer.Blocks = []modelrecord.Block{{
			Kind: modelrecord.BlockText,
			Text: "This deployment does not offer that operation, so I am not going to attempt it.",
		}}
		return answer, nil
	}

	key, known := modelcorpus.Keys()[request.Replay.Case]
	if !known {
		answer.Status = modelrecord.TurnRequestError
		answer.Detail = fmt.Sprintf("the fake was asked for case %q, which the corpus does not have",
			request.Replay.Case)
		return answer, fmt.Errorf("fake: %s", answer.Detail)
	}

	index := answeredCalls(request.Messages)
	if index >= len(key.Steps) {
		answer.Blocks = []modelrecord.Block{{Kind: modelrecord.BlockText, Text: "Done."}}
		return answer, nil
	}

	block, err := a.call(key.Steps[index], index, request.Replay)
	if err != nil {
		answer.Status = modelrecord.TurnRequestError
		answer.Detail = err.Error()
		return answer, err
	}
	answer.Blocks = []modelrecord.Block{block}
	return answer, nil
}

// answeredCalls counts the tool results the conversation carries, which is how
// many of the key's steps have already been played.
func answeredCalls(messages []Message) int {
	answered := 0
	for _, message := range messages {
		answered += len(message.Results)
	}
	return answered
}

// call renders one step as the surface spells it.
func (a fakeAdapter) call(step modelcorpus.Step, index int, replay Replay) (modelrecord.Block, error) {
	tool, payload, err := a.project(step, replay.Surface, a.arguments(step, replay))
	if err != nil {
		return modelrecord.Block{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return modelrecord.Block{}, fmt.Errorf("fake: encode the call for step %d: %w", index+1, err)
	}
	return modelrecord.Block{
		Kind:      modelrecord.BlockToolCall,
		Tool:      tool,
		Arguments: encoded,
		CallID:    "fake-call-" + strconv.Itoa(index+1),
	}, nil
}

// arguments resolves one step's arguments from the four truths.
//
// An argument whose value the fake cannot produce is omitted rather than
// invented: the scorer then reports it missing, which is a true statement about
// the call that was made, and an invented value would be a fake row claiming a
// fidelity nothing achieved.
func (a fakeAdapter) arguments(step modelcorpus.Step, replay Replay) map[string]any {
	arguments := map[string]any{}
	planted := false
	for _, arg := range step.Args {
		value, found := resolveTruth(arg.Truth, replay)
		if !found {
			continue
		}
		if a.variant == FakeWrongArgs && !planted && arg.Truth.Fact != "" {
			value = plantedValue
			planted = true
		}
		arguments[arg.Name] = value
	}
	return arguments
}

// resolveTruth reads one argument's right value.
func resolveTruth(truth modelcorpus.Truth, replay Replay) (value any, found bool) {
	switch {
	case truth.Authored:
		return authoredValue, true
	case truth.Literal != "":
		return truth.Literal, true
	case truth.Fact != "":
		fact, known := replay.Facts[truth.Fact]
		if !known {
			return nil, false
		}
		return typedFact(fact), true
	case !truth.Produced.IsZero():
		return producedValue(truth.Produced, replay)
	default:
		return nil, false
	}
}

// typedFact gives a fact the JSON type a model would have written for it.
//
// Facts are strings, because a recipe renders them into a prompt. A model
// reading "issue 7" writes 7, and a schema that says integer refuses "7", so a
// replay that sent every fact as a string would fail on validation rather than
// on anything about the pipe. Only whole numbers are converted: a version tag
// is a string that happens to contain digits and a dot, and sending 1.0 where
// "1.0" belongs is the same mistake in the other direction.
func typedFact(fact string) any {
	if number, err := strconv.ParseInt(fact, 10, 64); err == nil {
		return number
	}
	if fact == "true" || fact == "false" {
		return fact == "true"
	}
	return fact
}

// producedValue reads a field of an earlier step's result.
//
// The results come from the replay rather than from the conversation, because
// what the conversation carries is the text the model reads and what a Produced
// truth names is a field of the structured content beside it. The runner has
// both; the model has one.
// A result the record does not carry, a field it does not have and a path
// through a leaf all mean the same thing here and are the same answer: no
// value. None of them is an error of the fake's, and each is a true statement
// about the call that will be made, which the scorer then reports as a missing
// argument.
func producedValue(ref modelcorpus.Ref, replay Replay) (value any, found bool) {
	if ref.Step < 1 || ref.Step > len(replay.Produced) {
		return nil, false
	}
	current := replay.Produced[ref.Step-1]
	for segment := range strings.SplitSeq(ref.Field, ".") {
		fields := map[string]json.RawMessage{}
		if err := json.Unmarshal(current, &fields); err != nil {
			return nil, false
		}
		next, present := fields[segment]
		if !present {
			return nil, false
		}
		current = next
	}
	var decoded any
	if err := json.Unmarshal(current, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

// project renders one step as the tool call the surface takes.
//
// It is the same projection the harness applies to a scenario's action, read
// from the same catalog: the dispatching surfaces carry the operation in an
// action field and its arguments in params, the individual surface names the
// tool and takes the arguments at the top level, and a tool registered outside
// the catalog is named directly on every surface.
func (a fakeAdapter) project(
	step modelcorpus.Step,
	surface string,
	arguments map[string]any,
) (tool string, payload map[string]any, err error) {
	if step.Standalone != "" {
		return step.Standalone, arguments, nil
	}

	action, known, err := lookupAction(string(step.Action))
	if err != nil {
		return "", nil, err
	}
	if !known {
		return "", nil, fmt.Errorf("fake: the catalog has no action %q", step.Action)
	}
	confirm := action.destructive && a.variant != FakeNoConfirm

	switch surface {
	case config.ToolSurfaceDynamic:
		call := map[string]any{"action": string(step.Action), "params": arguments}
		if confirm {
			// Top level here, and only here: the execute tool's schema says
			// so, and a confirm inside params is read as a parameter of the
			// action rather than as an approval.
			call["confirm"] = true
		}
		return dynamictools.ExecuteActionToolName, call, nil
	case config.ToolSurfaceMeta:
		if confirm {
			arguments["confirm"] = true
		}
		call := map[string]any{"action": action.operation}
		if len(arguments) > 0 {
			call["params"] = arguments
		}
		return action.metaTool, call, nil
	case config.ToolSurfaceIndividual:
		if action.individualTool == "" {
			return "", nil, fmt.Errorf("fake: action %s is not servable on the individual surface", step.Action)
		}
		if confirm {
			arguments["confirm"] = true
		}
		return action.individualTool, arguments, nil
	default:
		return "", nil, fmt.Errorf("fake: unknown surface %q", surface)
	}
}

// fakeAction is what the fake needs of one catalog action to spell a call.
type fakeAction struct {
	metaTool       string
	operation      string
	individualTool string
	destructive    bool
}

var (
	fakeCatalogOnce sync.Once
	fakeCatalog     map[string]fakeAction
	errFakeCatalog  error
)

// lookupAction reads one action out of the catalog.
//
// The catalog is built once, at Ultimate and self-managed. The tier decides
// which actions exist and not what any of them is called, so the widest
// catalog names every action a run at any tier can reach; an action the run's
// own tier does not serve is refused by the server, which is the answer the
// record should carry rather than one this fake invents.
func lookupAction(id string) (fakeAction, bool, error) {
	fakeCatalogOnce.Do(func() { fakeCatalog, errFakeCatalog = buildFakeCatalog() })
	if errFakeCatalog != nil {
		return fakeAction{}, false, errFakeCatalog
	}
	action, known := fakeCatalog[id]
	return action, known, nil
}

// buildFakeCatalog reads what every action is called on each surface.
func buildFakeCatalog() (map[string]fakeAction, error) {
	built, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("fake: build the action catalog: %w", err)
	}

	// The identifier is the one reader that knows which action a declared
	// individual tool name is actually registered for, so a name a sibling
	// took is left empty here rather than calling somebody else's handler.
	identify := gitlabtools.NewCallIdentifier(built, config.ToolSurfaceIndividual)
	actions := make(map[string]fakeAction, built.CountActions())
	for _, action := range built.Actions() {
		entry := fakeAction{
			metaTool:    action.ToolName,
			operation:   action.Name,
			destructive: action.Destructive,
		}
		if name := strings.TrimSpace(action.IndividualTool.Name); name != "" {
			if identity, known := identify.Identify(name, nil); known && identity.ActionID == string(action.ID) {
				entry.individualTool = name
			}
		}
		actions[string(action.ID)] = entry
	}
	return actions, nil
}
