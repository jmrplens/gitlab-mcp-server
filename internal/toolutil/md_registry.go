package toolutil

import (
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	stringFormatters sync.Map // reflect.Type → stringFormatter
	resultFormatters sync.Map // reflect.Type → func(any) *mcp.CallToolResult
	// formatterNames records, per registered type, the name of the function
	// registered first, so a runtime finding can name the formatter and not
	// only the type it renders.
	formatterNames sync.Map // reflect.Type → string

	// registrationProblems records every registration the registry refused
	// or could only half honor, so a test can assert there are none: a
	// second formatter for a type that silently won, and an interface-typed
	// registration that landed under a key nothing could look up.
	registrationProblems   []string
	registrationProblemsMu sync.Mutex
)

// stringFormatter is a registered Markdown formatter with the content
// annotation its results carry, nil for the assistant default.
type stringFormatter struct {
	fn  func(any) string
	ann *mcp.Annotations
}

func init() {
	RegisterMarkdown(formatDeleteOutput)
	RegisterMarkdown(formatVoidOutput)
}

// formatDeleteOutput renders a DeleteOutput confirmation as a success string.
// The message is the handler's own sentence built from the identifier the
// caller passed, so it is written as prose with only the control bytes
// dropped; it is one line and can add no structure.
func formatDeleteOutput(v DeleteOutput) string {
	return EmojiSuccess + " " + StripControlBytes(v.Message)
}

// formatVoidOutput renders a VoidOutput confirmation as a success string, on
// the same terms as [formatDeleteOutput].
func formatVoidOutput(v VoidOutput) string {
	return EmojiSuccess + " " + StripControlBytes(v.Message)
}

// RegisterMarkdown registers a Markdown string formatter for type T.
// Subsequent calls to [MarkdownForResult] with a value of type T, or a
// pointer to one, will invoke fn and wrap the returned string in a
// [mcp.CallToolResult] annotated for the assistant.
func RegisterMarkdown[T any](fn func(T) string) {
	RegisterMarkdownAnnotated(fn, nil)
}

// RegisterMarkdownAnnotated registers a Markdown string formatter for type T
// whose results carry ann, one of the [ContentList], [ContentDetail] and
// [ContentMutate] presets; nil means [ContentAssistant]. The dispatchers
// override it with the annotation the action's declared content kind
// resolves to, so a formatter reached through the catalog is annotated by
// the spec and a formatter reached any other way by this.
//
// A second registration for a type is refused and recorded: the first one
// stays, since a registration that silently replaced another is how one
// output type came to render another action's heading. An interface type is
// refused and recorded too, because its registration lands under a key no
// concrete value ever looks up.
func RegisterMarkdownAnnotated[T any](fn func(T) string, ann *mcp.Annotations) {
	t := reflect.TypeFor[T]()
	if !registrable(t) {
		return
	}
	entry := stringFormatter{
		fn: func(v any) string {
			val, ok := v.(T)
			if !ok {
				return ""
			}
			return fn(val)
		},
		ann: ann,
	}
	if _, loaded := stringFormatters.LoadOrStore(t, entry); loaded {
		recordRegistrationProblem(fmt.Sprintf("duplicate Markdown formatter for %s: the first registration is kept", t))
	} else {
		formatterNames.LoadOrStore(t, functionName(fn))
	}
	if _, both := resultFormatters.Load(t); both {
		recordRegistrationProblem(fmt.Sprintf("%s has a string and a result formatter: the result formatter is served", t))
	}
}

// functionName names a registered function the way a stack trace does, so a
// finding can say groupcredentials.FormatPATListMarkdown rather than only the
// type it renders. A closure is named by the function that made it.
func functionName(fn any) string {
	if f := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()); f != nil {
		return f.Name()
	}
	return ""
}

// RegisteredMarkdownFormatterName returns the name of the formatter
// registered for t, the served one where two were registered, or "" when
// none is.
func RegisteredMarkdownFormatterName(t reflect.Type) string {
	if name, ok := formatterNames.Load(t); ok {
		if s, isString := name.(string); isString {
			return s
		}
	}
	return ""
}

// RegisterMarkdownPair registers two Markdown string formatters.
func RegisterMarkdownPair[A, B any](first func(A) string, second func(B) string) {
	RegisterMarkdown(first)
	RegisterMarkdown(second)
}

// RegisterMarkdownTriple registers three Markdown string formatters.
func RegisterMarkdownTriple[A, B, C any](first func(A) string, second func(B) string, third func(C) string) {
	RegisterMarkdown(first)
	RegisterMarkdown(second)
	RegisterMarkdown(third)
}

// RegisterMarkdownResult registers a result formatter for type T.
// Use this for types that need custom [mcp.CallToolResult] construction
// (e.g. uploads with image content). A second registration for a type, or a
// registration for an interface type, is refused and recorded on the same
// terms as [RegisterMarkdownAnnotated].
func RegisterMarkdownResult[T any](fn func(T) *mcp.CallToolResult) {
	t := reflect.TypeFor[T]()
	if !registrable(t) {
		return
	}
	entry := func(v any) *mcp.CallToolResult {
		val, ok := v.(T)
		if !ok {
			return nil
		}
		return fn(val)
	}
	if _, loaded := resultFormatters.LoadOrStore(t, entry); loaded {
		recordRegistrationProblem(fmt.Sprintf("duplicate Markdown result formatter for %s: the first registration is kept", t))
	} else {
		// A result formatter is the one served, so its name replaces a string
		// formatter's for the same type.
		formatterNames.Store(t, functionName(fn))
	}
	if _, both := stringFormatters.Load(t); both {
		recordRegistrationProblem(fmt.Sprintf("%s has a string and a result formatter: the result formatter is served", t))
	}
}

// registrable reports whether a formatter may be registered for t, recording
// why not when it may not: an interface type is refused, since nothing looks
// a formatter up by an interface and the registration would sit under a key
// no concrete value reaches.
func registrable(t reflect.Type) bool {
	if t == nil || t.Kind() == reflect.Interface {
		recordRegistrationProblem(fmt.Sprintf("Markdown formatter registered for the interface type %v: nothing looks a formatter up by an interface", t))
		return false
	}
	return true
}

// recordRegistrationProblem appends one problem to the record.
func recordRegistrationProblem(problem string) {
	registrationProblemsMu.Lock()
	defer registrationProblemsMu.Unlock()
	registrationProblems = append(registrationProblems, problem)
}

// MarkdownRegistrationProblems returns every registration the registry
// refused or could only half honor, sorted, for a test to assert there are
// none.
func MarkdownRegistrationProblems() []string {
	registrationProblemsMu.Lock()
	defer registrationProblemsMu.Unlock()
	out := slices.Clone(registrationProblems)
	sort.Strings(out)
	return out
}

// MarkdownForResult resolves a tool output to its Markdown [mcp.CallToolResult].
// A nil input renders the success confirmation; a value of an unregistered
// type, or a typed nil pointer, renders nil for the dispatcher to fall back
// on. A pointer to a registered type is dereferenced first, which is the
// same question [HasRegisteredMarkdownFormatter] answers: the two used to
// disagree, so a handler returning a pointer had a formatter the coverage
// test could see and the runtime never called.
func MarkdownForResult(result any) *mcp.CallToolResult {
	if result == nil {
		return SuccessResult("ok")
	}
	value := reflect.ValueOf(result)
	for value.Kind() == reflect.Pointer && !hasFormatter(value.Type()) {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
		result = value.Interface()
	}

	// Result formatters take priority (e.g. uploads with image content).
	if fn, ok := resultFormatters.Load(value.Type()); ok {
		if f, fOK := fn.(func(any) *mcp.CallToolResult); fOK {
			return f(result)
		}
	}

	if fn, ok := stringFormatters.Load(value.Type()); ok {
		if f, fOK := fn.(stringFormatter); fOK {
			return wrapMarkdown(f.fn(result), f.ann)
		}
	}

	return nil
}

// hasFormatter reports whether either registry holds a formatter for exactly
// t.
func hasFormatter(t reflect.Type) bool {
	if _, ok := stringFormatters.Load(t); ok {
		return true
	}
	_, ok := resultFormatters.Load(t)
	return ok
}

// wrapMarkdown converts a Markdown string into a CallToolResult normalized
// through [NormalizeResultMarkdown] and annotated with ann, or for the
// assistant when ann is nil.
func wrapMarkdown(md string, ann *mcp.Annotations) *mcp.CallToolResult {
	if md == "" {
		return nil
	}
	if ann == nil {
		ann = ContentAssistant
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: NormalizeResultMarkdown(md), Annotations: ann},
		},
	}
}

// NormalizeResultMarkdown is the one normalization every rendered response
// passes through on its way into a content block: trailing spaces and tabs
// dropped from each line, and control bytes dropped everywhere, since a
// formatter that writes a GitLab field straight into its builder would
// otherwise deliver an escape sequence to whatever prints the text. It is in
// one place so the constructor a formatter happens to be registered through
// cannot change the rendering. See [StripControlBytes].
func NormalizeResultMarkdown(md string) string {
	return StripControlBytes(stripTrailingLineWhitespace(md))
}

// stripTrailingLineWhitespace removes trailing spaces and tabs from each line.
func stripTrailingLineWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// RegisteredMarkdownTypes returns every type a Markdown formatter is
// registered for, string and result variants alike, sorted by name so a scan
// over them is deterministic. It is the seam the runtime structural gate
// drives: a fixture can be built for a type where only a name could not.
func RegisteredMarkdownTypes() []reflect.Type {
	var types []reflect.Type
	stringFormatters.Range(func(key, _ any) bool {
		if t, ok := key.(reflect.Type); ok {
			types = append(types, t)
		}
		return true
	})
	resultFormatters.Range(func(key, _ any) bool {
		if t, ok := key.(reflect.Type); ok {
			types = append(types, t)
		}
		return true
	})
	sort.Slice(types, func(i, j int) bool { return types[i].String() < types[j].String() })
	return types
}

// RegisteredMarkdownTypeNames returns the type names of all registered
// Markdown formatters (both string and result variants). Used by validation
// tests to verify sub-packages self-register their formatters.
func RegisteredMarkdownTypeNames() []string {
	types := RegisteredMarkdownTypes()
	names := make([]string, 0, len(types))
	for _, t := range types {
		names = append(names, t.String())
	}
	return names
}

// HasRegisteredMarkdownFormatter reports whether a Markdown formatter
// has been registered for the given Go type. Accepts either a reflect.Type
// (the canonical lookup path, matching spec.Route.OutputType) or a value
// of any kind (used by tests). Pointer types are dereferenced to their
// element type for the registry lookup, exactly as [MarkdownForResult]
// dereferences a pointer value. Returns false for nil, for the
// special "interface" reflect.Type returned by reflect.TypeOf on a
// nil/untyped interface, and for unregistered types.
//
// This is the authoritative source for "is there a Markdown formatter
// for this output type?" queries from external tools (e.g.
// cmd/audit_discovery_completeness uses it to gate the
// `missing_next_steps` check).
func HasRegisteredMarkdownFormatter(v any) bool {
	if v == nil {
		return false
	}
	var t reflect.Type
	switch x := v.(type) {
	case reflect.Type:
		t = x
	default:
		t = reflect.TypeOf(v)
	}
	// No separate nil check on t: a nil reflect.Type converted to any is a nil
	// interface, so every way one can arrive is already answered by the v == nil
	// guard above, and reflect.TypeOf never returns nil for a non-nil value.
	if t == reflect.TypeFor[any]() {
		return false
	}
	for {
		if hasFormatter(t) {
			return true
		}
		if t.Kind() != reflect.Pointer {
			return false
		}
		t = t.Elem()
	}
}

// MarkdownFormatterCount returns the number of registered Markdown
// formatters (string + result variants). Useful for sanity checks in
// tests and tools that need to know "are formatters even loaded?".
func MarkdownFormatterCount() int {
	return len(RegisteredMarkdownTypes())
}

// AnnotationsForContentKind maps an action's declared content kind
// ([ActionSpecContentList], [ActionSpecContentDetail],
// [ActionSpecContentMutate], [ActionSpecContentAssistant],
// [ActionSpecContentImage]) to the content annotation its text block
// carries, in one place, so the kind a spec declares and the annotation the
// dispatcher serves cannot disagree. An image action's text block is for the
// assistant like any other; the image block itself carries [ContentUser]. An
// empty or unknown kind is the assistant default.
func AnnotationsForContentKind(kind string) *mcp.Annotations {
	switch strings.TrimSpace(kind) {
	case ActionSpecContentList:
		return ContentList
	case ActionSpecContentDetail:
		return ContentDetail
	case ActionSpecContentMutate:
		return ContentMutate
	default:
		return ContentAssistant
	}
}

// FinishToolResult is the one tail every dispatcher applies to a formatted
// result, so an envelope rule cannot be added to two dispatchers of three
// again. In order: a nil result, which a formatter returning "" produces,
// becomes the JSON rendering rather than reaching the client with no
// content block; an error result is returned as it is, with no structured
// output, since the action produced none of what its schema describes;
// every text block is annotated with the action's declared content kind and
// every image block for the user; the next-step hints the Markdown carries
// are set on the typed output when its type declares them; and the canonical
// resource is embedded last, so the JSON it carries agrees with the
// structured output, hints included.
//
// The hints are set through the output's own [HintSetter], never spliced
// into its JSON: the individual surface validates the structured output
// against the schema the type declares, and a next_steps key on a type that
// declares none would fail that validation, so a type without
// [HintableOutput] carries its hints in the Markdown alone.
func FinishToolResult(callResult *mcp.CallToolResult, result any, route ActionRoute, params map[string]any) (finished *mcp.CallToolResult, structured any) {
	if callResult == nil {
		callResult = defaultFormatResult(result)
	}
	if callResult.IsError {
		return callResult, nil
	}
	annotateContent(callResult, AnnotationsForContentKind(route.ContentKind))
	result = withHintsSet(callResult, result)
	EmbedCanonicalResource(callResult, route.EmbeddedResource, params, result)
	return callResult, result
}

// annotateContent gives every text block the action's annotation and every
// image block the user's, leaving an embedded resource's own annotation as
// the embed wrote it.
func annotateContent(callResult *mcp.CallToolResult, text *mcp.Annotations) {
	for _, c := range callResult.Content {
		switch block := c.(type) {
		case *mcp.TextContent:
			block.Annotations = text
		case *mcp.ImageContent:
			if block.Annotations == nil {
				block.Annotations = ContentUser
			}
		}
	}
}

// withHintsSet returns result with the hints the Markdown carries set on it
// when its type can hold them: a pointer that implements [HintSetter] is set
// in place, a value whose pointer does is copied and set, and anything else
// is returned as it came.
func withHintsSet(callResult *mcp.CallToolResult, result any) any {
	if result == nil {
		return nil
	}
	hints := resultHints(callResult)
	if len(hints) == 0 {
		return result
	}
	if setter, ok := result.(HintSetter); ok {
		setter.SetNextSteps(hints)
		return result
	}
	value := reflect.ValueOf(result)
	if value.Kind() != reflect.Struct {
		return result
	}
	copied := reflect.New(value.Type())
	copied.Elem().Set(value)
	setter, ok := reflect.TypeAssert[HintSetter](copied)
	if !ok {
		return result
	}
	setter.SetNextSteps(hints)
	return copied.Elem().Interface()
}

// resultHints reads the next-step hints from the first text block that
// carries a guidance section.
func resultHints(callResult *mcp.CallToolResult) []string {
	for _, c := range callResult.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if hints := ExtractHints(tc.Text); len(hints) > 0 {
			return hints
		}
	}
	return nil
}
