// md_registry_test.go contains unit tests for the type-based Markdown formatter
// registry: RegisterMarkdown, RegisterMarkdownResult, MarkdownForResult dispatch,
// result formatter priority over string formatters, concurrent-safety of
// RegisterMarkdown, and stripTrailingLineWhitespace.
package toolutil

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// snapshotMarkdownRegistries saves both formatter registries and restores them
// when the test ends.
//
// Several tests here reset the registries wholesale to get a clean slate. That
// leaks: whatever the package init registered is gone for every test that runs
// afterwards, so assertions about init-registered formatters used to pass or
// fail purely on file-sort order. A test that calls this can reset freely and
// still leave the package as it found it.
func snapshotMarkdownRegistries(t *testing.T) {
	t.Helper()

	saved := map[reflect.Type]any{}
	stringFormatters.Range(func(k, v any) bool {
		saved[k.(reflect.Type)] = v
		return true
	})
	savedResults := map[reflect.Type]any{}
	resultFormatters.Range(func(k, v any) bool {
		savedResults[k.(reflect.Type)] = v
		return true
	})

	t.Cleanup(func() {
		stringFormatters = sync.Map{}
		resultFormatters = sync.Map{}
		for k, v := range saved {
			stringFormatters.Store(k, v)
		}
		for k, v := range savedResults {
			resultFormatters.Store(k, v)
		}
	})
}

// mdTestOutput is a test-only type registered with RegisterMarkdown
// to verify string formatter dispatch.
type mdTestOutput struct{ Name string }

// mdTestListOutput is a test-only type used to verify concurrent-safe
// registration and lookup of string formatters.
type mdTestListOutput struct{ Count int }

// mdTestResultOutput is a test-only type registered with RegisterMarkdownResult
// to verify result formatter dispatch.
type mdTestResultOutput struct{ URL string }

// mdUnregisteredOutput is a test-only type that is intentionally never
// registered, used to verify that MarkdownForResult returns nil for
// unknown types.
type mdUnregisteredOutput struct{}

// TestRegisterMarkdown_StringFormatter verifies that RegisterMarkdown stores
// a string formatter and that MarkdownForResult correctly invokes it and wraps
// the returned string in a TextContent [mcp.CallToolResult]. The test resets
// the registry to a clean state before registering, then asserts the output
// text matches the expected "## hello" value.
func TestRegisterMarkdown_StringFormatter(t *testing.T) {
	// Clean state: register formatters locally by resetting the map.
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(v mdTestOutput) string {
		return "## " + v.Name
	})

	got := MarkdownForResult(mdTestOutput{Name: "hello"})
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := got.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", got.Content[0])
	}
	if tc.Text != "## hello" {
		t.Errorf("text = %q, want %q", tc.Text, "## hello")
	}
}

// TestRegisterMarkdownResult_ResultFormatter verifies that RegisterMarkdownResult
// stores a custom result formatter and that MarkdownForResult returns the
// formatter's output unchanged. The test resets the registry, registers a
// formatter that prepends "custom: " to the URL, and asserts the content text
// matches the expected value.
func TestRegisterMarkdownResult_ResultFormatter(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdownResult(func(v mdTestResultOutput) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "custom: " + v.URL},
			},
		}
	})

	got := MarkdownForResult(mdTestResultOutput{URL: "https://example.com"})
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	tc := got.Content[0].(*mcp.TextContent)
	if tc.Text != "custom: https://example.com" {
		t.Errorf("text = %q, want %q", tc.Text, "custom: https://example.com")
	}
}

// TestMarkdownForResult_NilReturnsSuccess verifies that MarkdownForResult
// returns a non-nil success result when called with a nil input, so callers
// do not need to nil-guard the return value for the nil case.
func TestMarkdownForResult_NilReturnsSuccess(t *testing.T) {
	got := MarkdownForResult(nil)
	if got == nil {
		t.Fatal("nil input should return success result")
	}
}

// TestMarkdownForResult_UnknownTypeReturnsNil verifies that MarkdownForResult
// returns nil when no formatter is registered for the concrete type of the
// input value. The test resets the registry before the assertion to guarantee
// a clean state.
func TestMarkdownForResult_UnknownTypeReturnsNil(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	got := MarkdownForResult(mdUnregisteredOutput{})
	if got != nil {
		t.Errorf("expected nil for unregistered type, got %v", got)
	}
}

// TestMarkdownForResult_EmptyStringReturnsNil verifies that MarkdownForResult
// returns nil when a registered string formatter returns an empty string,
// signaling that the caller should fall back to a default representation.
func TestMarkdownForResult_EmptyStringReturnsNil(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(_ mdTestOutput) string { return "" })

	got := MarkdownForResult(mdTestOutput{Name: "empty"})
	if got != nil {
		t.Errorf("expected nil for empty markdown, got %v", got)
	}
}

// TestMarkdownForResult_ResultFormatterTakesPriority verifies that when both a
// string formatter and a result formatter are registered for the same type,
// MarkdownForResult uses the result formatter and ignores the string formatter.
// The test asserts that the content text is "result" (from the result formatter)
// rather than "string" (from the string formatter).
func TestMarkdownForResult_ResultFormatterTakesPriority(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(_ mdTestOutput) string { return "string" })
	RegisterMarkdownResult(func(_ mdTestOutput) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "result"}},
		}
	})

	got := MarkdownForResult(mdTestOutput{Name: "both"})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	tc := got.Content[0].(*mcp.TextContent)
	if tc.Text != "result" {
		t.Errorf("result formatter should take priority, got %q", tc.Text)
	}
}

// TestRegisterMarkdown_ConcurrentSafety verifies that concurrent calls to
// RegisterMarkdown and MarkdownForResult on the same type do not cause data
// races or panics. The test launches 100 goroutines that each register a
// formatter and immediately invoke MarkdownForResult, relying on the Go
// race detector to surface any unsafe concurrent access.
func TestRegisterMarkdown_ConcurrentSafety(t *testing.T) {
	snapshotMarkdownRegistries(t)
	snapshotRegistrationProblems(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			RegisterMarkdown(func(v mdTestListOutput) string {
				return "list"
			})
			_ = MarkdownForResult(mdTestListOutput{Count: n})
		}(i)
	}
	wg.Wait()
}

// TestStripTrailingLineWhitespace verifies that stripTrailingLineWhitespace
// removes trailing spaces and tabs from each line without affecting the line
// content itself. The test asserts that trailing whitespace is stripped from
// lines with mixed whitespace characters while the non-whitespace content
// and newline structure are preserved.
func TestStripTrailingLineWhitespace(t *testing.T) {
	input := "hello   \nworld\t\t\nok"
	want := "hello\nworld\nok"
	if got := stripTrailingLineWhitespace(input); got != want {
		t.Errorf("stripTrailingLineWhitespace = %q, want %q", got, want)
	}
}

// TestRegisteredMarkdownTypeNames_ReturnsRegisteredTypes verifies that
// RegisteredMarkdownTypeNames returns names for both string and result
// formatters that have been registered.
func TestRegisteredMarkdownTypeNames_ReturnsRegisteredTypes(t *testing.T) {
	RegisterMarkdown(func(_ mdTestOutput) string { return "s" })
	RegisterMarkdownResult(func(_ mdTestResultOutput) *mcp.CallToolResult { return nil })

	names := RegisteredMarkdownTypeNames()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["toolutil.mdTestOutput"] {
		t.Errorf("expected mdTestOutput in registered types, got %v", names)
	}
	if !found["toolutil.mdTestResultOutput"] {
		t.Errorf("expected mdTestResultOutput in registered types, got %v", names)
	}
}

// TestMarkdownForResult_DeleteOutputViaInit verifies that the init()
// function in md_registry.go registers the DeleteOutput formatter correctly
// and that it produces the expected success emoji + message output.
func TestMarkdownForResult_DeleteOutputViaInit(t *testing.T) {
	// Re-register: earlier tests in this file reset global maps, wiping init() state.
	RegisterMarkdown(func(v DeleteOutput) string {
		return EmojiSuccess + " " + v.Message
	})

	result := MarkdownForResult(DeleteOutput{Message: "Project deleted"})
	if result == nil {
		t.Fatal("expected non-nil result for DeleteOutput")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	want := EmojiSuccess + " Project deleted"
	if tc.Text != want {
		t.Errorf("text = %q, want %q", tc.Text, want)
	}
}

// TestMarkdownForResult_VoidOutputViaInit verifies that the built-in VoidOutput
// formatter produces a success emoji followed by the configured message.
func TestMarkdownForResult_VoidOutputViaInit(t *testing.T) {
	// Re-register: earlier tests in this file reset global maps, wiping init() state.
	RegisterMarkdown(func(v VoidOutput) string {
		return EmojiSuccess + " " + v.Message
	})

	result := MarkdownForResult(VoidOutput{Message: "Action completed"})
	if result == nil {
		t.Fatal("expected non-nil result for VoidOutput")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	want := EmojiSuccess + " Action completed"
	if tc.Text != want {
		t.Errorf("text = %q, want %q", tc.Text, want)
	}
}

// TestFormatDeleteOutput_ReturnsEmojiPlusMessage verifies that delete output
// formatting combines the success emoji and message text.
func TestFormatDeleteOutput_ReturnsEmojiPlusMessage(t *testing.T) {
	t.Parallel()
	got := formatDeleteOutput(DeleteOutput{Status: "success", Message: "branch deleted"})
	want := EmojiSuccess + " branch deleted"
	if got != want {
		t.Fatalf("formatDeleteOutput = %q, want %q", got, want)
	}
}

// TestFormatVoidOutput_ReturnsEmojiPlusMessage verifies that void output
// formatting combines the success emoji and message text.
func TestFormatVoidOutput_ReturnsEmojiPlusMessage(t *testing.T) {
	t.Parallel()
	got := formatVoidOutput(VoidOutput{Status: "success", Message: "action completed"})
	want := EmojiSuccess + " action completed"
	if got != want {
		t.Fatalf("formatVoidOutput = %q, want %q", got, want)
	}
}

// TestRegisterMarkdownPair_RegistersBothTypes verifies that RegisterMarkdownPair
// registers two distinct string formatters and MarkdownForResult dispatches
// each to the correct renderer.
func TestRegisterMarkdownPair_RegistersBothTypes(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	type pairA struct{ A string }
	type pairB struct{ B string }

	RegisterMarkdownPair(
		func(v pairA) string { return "A:" + v.A },
		func(v pairB) string { return "B:" + v.B },
	)

	if got := MarkdownForResult(pairA{A: "x"}); got == nil || !extractText(got).startsWith("A:") {
		t.Errorf("pairA dispatch = %+v, want A:x", got)
	}
	if got := MarkdownForResult(pairB{B: "y"}); got == nil || !extractText(got).startsWith("B:") {
		t.Errorf("pairB dispatch = %+v, want B:y", got)
	}
}

// TestRegisterMarkdownTriple_RegistersAllTypes verifies that RegisterMarkdownTriple
// registers three distinct string formatters and MarkdownForResult dispatches
// each correctly.
func TestRegisterMarkdownTriple_RegistersAllTypes(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	type tripleA struct{ A string }
	type tripleB struct{ B string }
	type tripleC struct{ C string }

	RegisterMarkdownTriple(
		func(v tripleA) string { return "A:" + v.A },
		func(v tripleB) string { return "B:" + v.B },
		func(v tripleC) string { return "C:" + v.C },
	)

	if got := MarkdownForResult(tripleA{A: "1"}); got == nil || !extractText(got).startsWith("A:") {
		t.Errorf("tripleA dispatch = %+v, want A:1", got)
	}
	if got := MarkdownForResult(tripleB{B: "2"}); got == nil || !extractText(got).startsWith("B:") {
		t.Errorf("tripleB dispatch = %+v, want B:2", got)
	}
	if got := MarkdownForResult(tripleC{C: "3"}); got == nil || !extractText(got).startsWith("C:") {
		t.Errorf("tripleC dispatch = %+v, want C:3", got)
	}
}

// TestRegisterMarkdown_TypeMismatchReturnsEmpty exercises the defensive
// type-assertion branch in RegisterMarkdown when the stored closure receives
// a non-matching concrete type.
func TestRegisterMarkdown_TypeMismatchReturnsEmpty(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(v mdTestOutput) string { return "## " + v.Name })

	// Inject a wrong-type value via the registry directly to exercise the
	// defensive `if !ok { return "" }` branch.
	loaded, loadOK := stringFormatters.Load(reflect.TypeFor[mdTestOutput]())
	if !loadOK {
		t.Fatal("formatter not registered for mdTestOutput")
	}
	if entry, assertOK := loaded.(stringFormatter); assertOK {
		if got := entry.fn(mdTestListOutput{}); got != "" {
			t.Errorf("type-mismatch dispatch = %q, want empty string", got)
		}
	} else {
		t.Fatalf("registered string formatter has unexpected type: %T", loaded)
	}
}

// snapshotRegistrationProblems saves the registration record and restores it
// when the test ends, so a test that registers a duplicate on purpose leaves
// nothing behind for a test that asserts the record is clean.
func snapshotRegistrationProblems(t *testing.T) {
	t.Helper()
	registrationProblemsMu.Lock()
	saved := slices.Clone(registrationProblems)
	registrationProblems = nil
	registrationProblemsMu.Unlock()
	t.Cleanup(func() {
		registrationProblemsMu.Lock()
		registrationProblems = saved
		registrationProblemsMu.Unlock()
	})
}

// mdPointerOutput is a test-only type registered by value and looked up
// through a pointer, the shape of a handler that returns *ListOutput.
type mdPointerOutput struct{ Name string }

// mdHintedOutput is a test-only output type that declares next_steps.
type mdHintedOutput struct {
	HintableOutput
	Name string `json:"name"`
}

// mdInterfaceOutput is a test-only interface a registration must refuse.
type mdInterfaceOutput interface{ Render() string }

// TestMarkdownForResult_PointerToRegisteredType_IsDereferenced verifies the
// runtime lookup answers the same question the coverage predicate answers: a
// value of a registered type, a pointer to one and a pointer to a pointer all
// render, and a typed nil pointer renders nil rather than dereferencing.
// A handler returning a pointer used to have a formatter the coverage test
// could see and the runtime never called.
func TestMarkdownForResult_PointerToRegisteredType_IsDereferenced(t *testing.T) {
	snapshotMarkdownRegistries(t)
	snapshotRegistrationProblems(t)
	RegisterMarkdown(func(v mdPointerOutput) string { return "## " + v.Name })

	value := mdPointerOutput{Name: "p"}
	pointer := &value
	cases := []struct {
		name   string
		result any
		want   string
	}{
		{name: "value", result: value, want: "## p"},
		{name: "pointer", result: pointer, want: "## p"},
		{name: "pointer to pointer", result: &pointer, want: "## p"},
		{name: "typed nil pointer", result: (*mdPointerOutput)(nil), want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MarkdownForResult(tc.result)
			if tc.want == "" {
				if got != nil {
					t.Errorf("MarkdownForResult(%T) = %+v, want nil", tc.result, got)
				}
				return
			}
			if got == nil || string(extractText(got)) != tc.want {
				t.Errorf("MarkdownForResult(%T) = %+v, want text %q", tc.result, got, tc.want)
			}
			if !HasRegisteredMarkdownFormatter(tc.result) {
				t.Errorf("HasRegisteredMarkdownFormatter(%T) = false, disagreeing with the lookup that just rendered it", tc.result)
			}
		})
	}
}

// TestRegisterMarkdown_Collisions_AreRecordedAndTheFirstIsKept verifies the
// registration record: a second formatter for one type is refused and the
// first keeps rendering, a string and a result formatter for one type are
// recorded, and an interface-typed registration is refused, since it would
// land under a key no concrete value looks up.
func TestRegisterMarkdown_Collisions_AreRecordedAndTheFirstIsKept(t *testing.T) {
	snapshotMarkdownRegistries(t)
	snapshotRegistrationProblems(t)

	RegisterMarkdown(func(mdPointerOutput) string { return "first" })
	RegisterMarkdown(func(mdPointerOutput) string { return "second" })
	RegisterMarkdownResult(func(mdPointerOutput) *mcp.CallToolResult { return nil })
	RegisterMarkdown(func(mdInterfaceOutput) string { return "interface" })

	want := []string{
		"Markdown formatter registered for the interface type toolutil.mdInterfaceOutput: nothing looks a formatter up by an interface",
		"duplicate Markdown formatter for toolutil.mdPointerOutput: the first registration is kept",
		"toolutil.mdPointerOutput has a string and a result formatter: the result formatter is served",
	}
	if got := MarkdownRegistrationProblems(); !slices.Equal(got, want) {
		t.Errorf("MarkdownRegistrationProblems() = %q, want %q", got, want)
	}
	if entry, ok := stringFormatters.Load(reflect.TypeFor[mdPointerOutput]()); !ok || entry.(stringFormatter).fn(mdPointerOutput{}) != "first" {
		t.Errorf("the first registration did not survive the second")
	}
	if _, ok := stringFormatters.Load(reflect.TypeFor[mdInterfaceOutput]()); ok {
		t.Error("the interface-typed registration was stored")
	}
}

// TestRegisterMarkdownAnnotated_Preset_IsCarriedByTheResult verifies that a
// formatter registered with a content preset renders results carrying it,
// and that the plain registration renders the assistant default, so the
// presets are reachable through the registry at all.
func TestRegisterMarkdownAnnotated_Preset_IsCarriedByTheResult(t *testing.T) {
	snapshotMarkdownRegistries(t)
	snapshotRegistrationProblems(t)
	RegisterMarkdownAnnotated(func(mdPointerOutput) string { return "list" }, ContentList)
	RegisterMarkdown(func(mdTestOutput) string { return "plain" })

	if got := MarkdownForResult(mdPointerOutput{}).Content[0].(*mcp.TextContent).Annotations; got != ContentList {
		t.Errorf("annotated registration rendered %+v, want the list preset", got)
	}
	if got := MarkdownForResult(mdTestOutput{}).Content[0].(*mcp.TextContent).Annotations; got != ContentAssistant {
		t.Errorf("plain registration rendered %+v, want the assistant default", got)
	}
}

// TestRegisteredMarkdownTypes_Registrations_AreListedSorted verifies the seam
// the runtime gate drives: every registered type, string and result variants
// alike, in name order, with the count agreeing.
func TestRegisteredMarkdownTypes_Registrations_AreListedSorted(t *testing.T) {
	snapshotMarkdownRegistries(t)
	snapshotRegistrationProblems(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}
	RegisterMarkdown(func(mdTestOutput) string { return "" })
	RegisterMarkdownResult(func(mdTestResultOutput) *mcp.CallToolResult { return nil })
	RegisterMarkdown(func(mdPointerOutput) string { return "" })

	got := RegisteredMarkdownTypes()
	want := []reflect.Type{reflect.TypeFor[mdPointerOutput](), reflect.TypeFor[mdTestOutput](), reflect.TypeFor[mdTestResultOutput]()}
	if !slices.Equal(got, want) {
		t.Errorf("RegisteredMarkdownTypes() = %v, want %v", got, want)
	}
	if MarkdownFormatterCount() != 3 {
		t.Errorf("MarkdownFormatterCount() = %d, want 3", MarkdownFormatterCount())
	}
}

// TestAnnotationsForContentKind_Kinds_MapToOnePresetEach verifies the one
// mapping from a declared content kind to the annotation a result carries,
// with the assistant default for an image action's text, an empty kind and a
// kind nothing declares.
func TestAnnotationsForContentKind_Kinds_MapToOnePresetEach(t *testing.T) {
	cases := []struct {
		name string
		kind string
		want *mcp.Annotations
	}{
		{name: "list", kind: ActionSpecContentList, want: ContentList},
		{name: "detail", kind: ActionSpecContentDetail, want: ContentDetail},
		{name: "mutate", kind: ActionSpecContentMutate, want: ContentMutate},
		{name: "assistant", kind: ActionSpecContentAssistant, want: ContentAssistant},
		{name: "image text", kind: ActionSpecContentImage, want: ContentAssistant},
		{name: "empty", kind: "", want: ContentAssistant},
		{name: "unknown", kind: "poster", want: ContentAssistant},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AnnotationsForContentKind(tc.kind); got != tc.want {
				t.Errorf("AnnotationsForContentKind(%q) = %+v, want %+v", tc.kind, got, tc.want)
			}
		})
	}
}

// hintedTextResult is a formatted result whose Markdown ends with one hint.
func hintedTextResult() *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "## X\n\n---\n💡 **Next steps:**\n- do this\n"}}}
}

// TestFinishToolResult_Envelope_NilErrorAndImageResults verifies the envelope
// half of the tail every dispatcher shares: a nil result becomes the JSON
// rendering, an error result passes through with no structured output, a
// nil output stays nil, and an image action's text block is for the
// assistant while its image block is for the user.
func TestFinishToolResult_Envelope_NilErrorAndImageResults(t *testing.T) {
	route := ActionRoute{ContentKind: ActionSpecContentDetail}

	t.Run("nil result renders JSON", func(t *testing.T) {
		got, structured := FinishToolResult(nil, mdHintedOutput{Name: "n"}, route, nil)
		if got == nil || string(extractText(got)) != `{"name":"n"}` {
			t.Errorf("FinishToolResult(nil) = %+v, want the JSON rendering", got)
		}
		if _, ok := structured.(mdHintedOutput); !ok {
			t.Errorf("structured output is %T, want the typed value", structured)
		}
	})
	t.Run("error result passes through", func(t *testing.T) {
		errResult := &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "no"}}}
		got, structured := FinishToolResult(errResult, mdHintedOutput{}, route, nil)
		if got != errResult || structured != nil {
			t.Errorf("FinishToolResult(error) = %+v, %v; want the error result and no structured output", got, structured)
		}
	})
	t.Run("nil output stays nil", func(t *testing.T) {
		got, structured := FinishToolResult(hintedTextResult(), nil, ActionRoute{}, nil)
		if structured != nil || got == nil {
			t.Errorf("FinishToolResult(nil output) = %+v, %v", got, structured)
		}
	})
	t.Run("image block is for the user", func(t *testing.T) {
		withImage := &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: "meta"},
			&mcp.ImageContent{Data: []byte{1}, MIMEType: "image/png"},
		}}
		got, _ := FinishToolResult(withImage, mdTestOutput{}, ActionRoute{ContentKind: ActionSpecContentImage}, nil)
		if ann := got.Content[0].(*mcp.TextContent).Annotations; ann != ContentAssistant {
			t.Errorf("image action's text annotated %+v, want the assistant default", ann)
		}
		if ann := got.Content[1].(*mcp.ImageContent).Annotations; ann != ContentUser {
			t.Errorf("image block annotated %+v, want the user preset", ann)
		}
	})
}

// TestFinishToolResult_Hints_SetOnTheTypedOutput verifies the hints half of
// the tail: the hints are set on a value type through a copy that leaves the
// caller's value alone and on a pointer type in place, a type declaring no
// next_steps is returned as it came, the text block carries the route's
// content kind, and the embedded resource written last carries the hints.
func TestFinishToolResult_Hints_SetOnTheTypedOutput(t *testing.T) {
	route := ActionRoute{ContentKind: ActionSpecContentDetail, EmbeddedResource: "gitlab://things/{id}"}

	t.Run("value type gets its hints through a copy", func(t *testing.T) {
		in := mdHintedOutput{Name: "v"}
		got, structured := FinishToolResult(hintedTextResult(), in, route, map[string]any{"id": 7})
		out, ok := structured.(mdHintedOutput)
		if !ok || len(out.NextSteps) != 1 || out.NextSteps[0] != "do this" || out.Name != "v" {
			t.Errorf("structured output = %+v, want the value with its hint set", structured)
		}
		if in.NextSteps != nil {
			t.Error("the caller's value was mutated")
		}
		if ann := got.Content[0].(*mcp.TextContent).Annotations; ann != ContentDetail {
			t.Errorf("text annotated %+v, want the detail preset", ann)
		}
		embedded, ok := got.Content[1].(*mcp.EmbeddedResource)
		if !ok || embedded.Resource.URI != "gitlab://things/7" || !strings.Contains(embedded.Resource.Text, `"next_steps":["do this"]`) {
			t.Errorf("embedded resource = %+v, want the URI and a body carrying the hints", got.Content[1])
		}
	})
	t.Run("pointer type gets its hints in place", func(t *testing.T) {
		in := &mdHintedOutput{Name: "p"}
		_, structured := FinishToolResult(hintedTextResult(), in, ActionRoute{}, nil)
		if structured != any(in) || len(in.NextSteps) != 1 {
			t.Errorf("structured output = %+v, want the same pointer with its hint set", structured)
		}
	})
	t.Run("type without next_steps is returned as it came", func(t *testing.T) {
		in := mdTestOutput{Name: "plain"}
		_, structured := FinishToolResult(hintedTextResult(), in, ActionRoute{}, nil)
		if structured != any(in) {
			t.Errorf("structured output = %#v, want the value unchanged", structured)
		}
	})
}

// TestNormalizeResultMarkdown_Text_DropsTrailingWhitespaceAndControlBytes
// verifies the one normalization every content block passes through.
func TestNormalizeResultMarkdown_Text_DropsTrailingWhitespaceAndControlBytes(t *testing.T) {
	if got, want := NormalizeResultMarkdown("a \t\nb\x1b[2J\t\nc"), "a\nb[2J\nc"; got != want {
		t.Errorf("NormalizeResultMarkdown() = %q, want %q", got, want)
	}
}

// TestRegisterMarkdownResult_TypeMismatchReturnsNil exercises the defensive
// type-assertion branch in RegisterMarkdownResult when the stored closure
// receives a non-matching concrete type.
func TestRegisterMarkdownResult_TypeMismatchReturnsNil(t *testing.T) {
	snapshotMarkdownRegistries(t)
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdownResult(func(v mdTestResultOutput) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: v.URL}}}
	})

	loaded, loadOK := resultFormatters.Load(reflect.TypeFor[mdTestResultOutput]())
	if !loadOK {
		t.Fatal("result formatter not registered for mdTestResultOutput")
	}
	if fn, assertOK := loaded.(func(any) *mcp.CallToolResult); assertOK {
		if got := fn(mdTestListOutput{}); got != nil {
			t.Errorf("type-mismatch dispatch = %+v, want nil", got)
		}
	} else {
		t.Fatalf("registered result formatter has unexpected type: %T", loaded)
	}
}

// extractText returns the first text-content entry of a CallToolResult or "".
func extractText(result *mcp.CallToolResult) textPrefix {
	if result == nil {
		return textPrefix("")
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return textPrefix(tc.Text)
		}
	}
	return textPrefix("")
}

// textPrefix is a tiny string type with a startsWith helper to keep
// the assertions above readable.
type textPrefix string

func (s textPrefix) startsWith(prefix string) bool {
	return len(s) >= len(prefix) && string(s[:len(prefix)]) == prefix
}

// mdQueryUnregistered is a local type that never gets a Markdown formatter,
// used to exercise the negative lookup path.
type mdQueryUnregistered struct{ X int }

// mdQueryResultOnly is a local type registered exclusively through
// [RegisterMarkdownResult], used to exercise the result-formatter branch of
// [HasRegisteredMarkdownFormatter].
type mdQueryResultOnly struct{ Y int }

// TestHasRegisteredMarkdownFormatter_NilValue_ReturnsFalse verifies that
// [HasRegisteredMarkdownFormatter] returns false for a nil input, which has
// no concrete type to look up in the registries.
func TestHasRegisteredMarkdownFormatter_NilValue_ReturnsFalse(t *testing.T) {
	if HasRegisteredMarkdownFormatter(nil) {
		t.Error("HasRegisteredMarkdownFormatter(nil) = true, want false")
	}
}

// TestHasRegisteredMarkdownFormatter_NilReflectType_ReturnsFalse verifies that
// a nil reflect.Type is answered false rather than panicking on the Kind() call
// that follows.
//
// This is how a spec with no output type arrives: Route.OutputType is a
// reflect.Type field, and an unset one converted to any is a nil interface, not
// an interface holding a nil type. The nil-value guard at the top of the
// function is therefore the one that answers it, and there is no second nil
// check downstream for it to reach.
func TestHasRegisteredMarkdownFormatter_NilReflectType_ReturnsFalse(t *testing.T) {
	var unset reflect.Type
	if HasRegisteredMarkdownFormatter(unset) {
		t.Error("HasRegisteredMarkdownFormatter(reflect.Type(nil)) = true, want false")
	}
}

// TestHasRegisteredMarkdownFormatter_ReflectType_ReturnsTrue verifies the
// canonical reflect.Type lookup path: a registered type's reflect.Type must
// resolve to a formatter. The formatter comes from the package init, which
// every registry-resetting test in this file now restores via
// snapshotMarkdownRegistries.
func TestHasRegisteredMarkdownFormatter_ReflectType_ReturnsTrue(t *testing.T) {
	if !HasRegisteredMarkdownFormatter(reflect.TypeFor[DeleteOutput]()) {
		t.Error("HasRegisteredMarkdownFormatter(reflect.TypeFor[DeleteOutput]()) = false, want true")
	}
}

// TestHasRegisteredMarkdownFormatter_Value_ReturnsTrue verifies the
// plain-value lookup path (the default switch arm using reflect.TypeOf)
// for a type with a registered string formatter.
func TestHasRegisteredMarkdownFormatter_Value_ReturnsTrue(t *testing.T) {
	if !HasRegisteredMarkdownFormatter(DeleteOutput{Message: "gone"}) {
		t.Error("HasRegisteredMarkdownFormatter(DeleteOutput{}) = false, want true")
	}
}

// TestHasRegisteredMarkdownFormatter_PointerValue_Dereferenced verifies that
// pointer types are dereferenced to their element type before the registry
// lookup, so *DeleteOutput matches the DeleteOutput formatter.
func TestHasRegisteredMarkdownFormatter_PointerValue_Dereferenced(t *testing.T) {
	if !HasRegisteredMarkdownFormatter(&DeleteOutput{Message: "gone"}) {
		t.Error("HasRegisteredMarkdownFormatter(*DeleteOutput) = false, want true")
	}
	if !HasRegisteredMarkdownFormatter(reflect.TypeFor[**DeleteOutput]()) {
		t.Error("HasRegisteredMarkdownFormatter(**DeleteOutput type) = false, want true")
	}
}

// TestHasRegisteredMarkdownFormatter_AnyInterfaceType_ReturnsFalse verifies
// that the special "any" interface reflect.Type is rejected: it carries no
// concrete output type and must never match a formatter.
func TestHasRegisteredMarkdownFormatter_AnyInterfaceType_ReturnsFalse(t *testing.T) {
	if HasRegisteredMarkdownFormatter(reflect.TypeFor[any]()) {
		t.Error("HasRegisteredMarkdownFormatter(reflect.TypeFor[any]()) = true, want false")
	}
}

// TestHasRegisteredMarkdownFormatter_UnregisteredType_ReturnsFalse verifies
// that a type absent from both the string and result registries reports
// false (the final fall-through return).
func TestHasRegisteredMarkdownFormatter_UnregisteredType_ReturnsFalse(t *testing.T) {
	if HasRegisteredMarkdownFormatter(mdQueryUnregistered{X: 1}) {
		t.Error("HasRegisteredMarkdownFormatter(unregistered type) = true, want false")
	}
}

// TestHasRegisteredMarkdownFormatter_ResultFormatter_ReturnsTrue verifies
// that types registered only via [RegisterMarkdownResult] (custom
// CallToolResult construction, e.g. image content) are also reported as
// having a formatter.
func TestHasRegisteredMarkdownFormatter_ResultFormatter_ReturnsTrue(t *testing.T) {
	RegisterMarkdownResult(func(mdQueryResultOnly) *mcp.CallToolResult {
		return SuccessResult("ok")
	})
	if !HasRegisteredMarkdownFormatter(mdQueryResultOnly{Y: 2}) {
		t.Error("HasRegisteredMarkdownFormatter(result-only type) = false, want true")
	}
}

// TestMarkdownFormatterCount_IncludesInitRegistrations verifies that
// [MarkdownFormatterCount] counts both registries and reports at least the
// two formatters registered by this package's init (DeleteOutput and
// VoidOutput).
func TestMarkdownFormatterCount_IncludesInitRegistrations(t *testing.T) {
	if n := MarkdownFormatterCount(); n < 2 {
		t.Errorf("MarkdownFormatterCount() = %d, want >= 2", n)
	}
}

// TestWrapMarkdown_CarriesNoControlBytes verifies that the registry's own
// result builder drops terminal control sequences, like the builders in
// markdown.go. It is the path every registered formatter's output takes on its
// way to a tool result, so a formatter that writes a GitLab field straight into
// its builder is contained here even when nothing else contained it.
func TestWrapMarkdown_CarriesNoControlBytes(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want string
	}{
		{name: "empty markdown returns no result", md: "", want: ""},
		{name: "ordinary markdown is unchanged", md: "## Title\n\nbody\n", want: "## Title\n\nbody\n"},
		{name: "clear screen and window title are dropped", md: "trace\x1b[2J\x1b]0;pwned\x07\n", want: "trace[2J]0;pwned\n"},
		{name: "trailing whitespace still stripped", md: "line   \nnext\t\n", want: "line\nnext\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapMarkdown(tt.md, nil)
			if tt.want == "" {
				if result != nil {
					t.Errorf("wrapMarkdown(%q) = %v, want nil", tt.md, result)
				}
				return
			}
			if result == nil || len(result.Content) == 0 {
				t.Fatalf("wrapMarkdown(%q) returned no content", tt.md)
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("wrapMarkdown content is %T, want *mcp.TextContent", result.Content[0])
			}
			if text.Text != tt.want {
				t.Errorf("wrapMarkdown(%q) = %q, want %q", tt.md, text.Text, tt.want)
			}
		})
	}
}
