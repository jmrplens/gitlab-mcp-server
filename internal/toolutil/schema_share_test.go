package toolutil

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

// TestSchemaIdentity_OnlyMapsAndCompiledSchemasHaveOne verifies which values
// can serve as memo keys: a non-nil map and a non-nil compiled schema, and
// nothing else, since a nil of either kind and any other value have no
// address worth keying on.
func TestSchemaIdentity_OnlyMapsAndCompiledSchemasHaveOne(t *testing.T) {
	t.Parallel()

	var nilMap map[string]any
	var nilSchema *jsonschema.Schema
	cases := []struct {
		name   string
		schema any
		want   bool
	}{
		{name: "map", schema: map[string]any{"type": "object"}, want: true},
		{name: "nil map", schema: nilMap},
		{name: "compiled schema", schema: &jsonschema.Schema{Type: "object"}, want: true},
		{name: "nil compiled schema", schema: nilSchema},
		{name: "nil interface", schema: nil},
		{name: "string", schema: "object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, got := schemaIdentity(tc.schema); got != tc.want {
				t.Errorf("schemaIdentity(%T) identity = %t, want %t", tc.schema, got, tc.want)
			}
			if SchemaShared(tc.schema) {
				t.Errorf("SchemaShared(%T) = true for a value nobody registered", tc.schema)
			}
		})
	}
}

// TestDeriveSchema_PrivateInputDerivesEveryTime verifies a schema nobody
// registered keeps the old behavior: each call builds its own result.
func TestDeriveSchema_PrivateInputDerivesEveryTime(t *testing.T) {
	t.Parallel()

	private := map[string]any{"type": "object"}
	calls := 0
	derive := func() any { calls++; return map[string]any{"type": "object", "call": calls} }

	first := DeriveSchema(private, "test", derive)
	second := DeriveSchema(private, "test", derive)
	if calls != 2 {
		t.Fatalf("derive ran %d times for a private schema, want 2", calls)
	}
	if sameMap(first.(map[string]any), second.(map[string]any)) {
		t.Fatal("DeriveSchema(private) returned one map twice, want a private result per call")
	}
	if SchemaShared(first) {
		t.Fatal("a private derivation was registered as shared")
	}
}

// TestDeriveSchema_SharedInputMemoizesAndIsIdempotent verifies the memo: a
// registered schema's transform is built once and served to every caller,
// the result is registered in turn, reapplying the transform to the result
// returns the result itself, and a different transform name is a different
// memo entry.
func TestDeriveSchema_SharedInputMemoizesAndIsIdempotent(t *testing.T) {
	t.Parallel()

	shared := map[string]any{"type": "object"}
	ShareSchema(shared)
	if !SchemaShared(shared) {
		t.Fatal("ShareSchema() did not register the map")
	}
	calls := 0
	derive := func() any { calls++; return map[string]any{"type": "object", "call": calls} }
	transform := "test|" + t.Name()

	first := DeriveSchema(shared, transform, derive)
	second := DeriveSchema(shared, transform, derive)
	if calls != 1 {
		t.Fatalf("derive ran %d times for a shared schema, want 1", calls)
	}
	if !sameMap(first.(map[string]any), second.(map[string]any)) {
		t.Fatal("DeriveSchema(shared) returned two maps, want one shared result")
	}
	if !SchemaShared(first) {
		t.Fatal("the shared derivation was not registered as shared")
	}
	if again := DeriveSchema(first, transform, derive); calls != 1 || !sameMap(again.(map[string]any), first.(map[string]any)) {
		t.Fatalf("reapplying the transform to its output ran derive (%d calls) or built a new map, want the output itself", calls)
	}
	other := DeriveSchema(shared, transform+"|other", derive)
	if calls != 2 || sameMap(other.(map[string]any), first.(map[string]any)) {
		t.Fatalf("a different transform name shared the first result (calls = %d)", calls)
	}
}

// TestDeriveSchema_OutputWithoutIdentityIsStillMemoized verifies a transform
// whose result has no identity of its own (a value that is neither a map
// nor a compiled schema) is memoized under its input and served again, with
// nothing recorded as its origin.
func TestDeriveSchema_OutputWithoutIdentityIsStillMemoized(t *testing.T) {
	t.Parallel()

	shared := map[string]any{"type": "object"}
	ShareSchema(shared)
	calls := 0
	derive := func() any { calls++; return fmt.Sprintf("rendered %d", calls) }
	transform := "render|" + t.Name()

	first := DeriveSchema(shared, transform, derive)
	second := DeriveSchema(shared, transform, derive)
	if calls != 1 || first != second || first != "rendered 1" {
		t.Fatalf("DeriveSchema() = %v then %v after %d derive calls, want one memoized rendering", first, second, calls)
	}
}

// TestDeriveSchema_TransformReturningItsInput verifies a transform that has
// nothing to change may return its input: the memo then serves the input,
// and an origin already recorded for it is kept.
func TestDeriveSchema_TransformReturningItsInput(t *testing.T) {
	t.Parallel()

	shared := map[string]any{"type": "object"}
	ShareSchema(shared)
	derived := DeriveSchema(shared, "first|"+t.Name(), func() any { return map[string]any{"type": "object"} })
	same := DeriveSchema(derived, "identity|"+t.Name(), func() any { return derived })
	if !sameMap(same.(map[string]any), derived.(map[string]any)) {
		t.Fatal("DeriveSchema() did not return the input the transform handed back")
	}
	if origin, ok := derivedOrigin.Load(reflect.ValueOf(derived).UnsafePointer()); !ok || origin != "first|"+t.Name() {
		t.Fatalf("origin of the derived map = %v, %t, want the first transform kept", origin, ok)
	}
}

// TestDeriveSchema_ConcurrentCallersShareOneResult verifies callers racing
// for one transform of one shared schema all receive the same map: the memo
// is the point of contention when a pool builds many servers at once.
//
// The derivation waits on an [arrivalGate], so every caller is inside it
// before any of them stores. Nothing here is exclusive — a memo miss lets all
// of them derive — so the gate makes the store the only thing that decides
// which map they share: replacing its LoadOrStore with a plain store, which
// hands each caller the map it built itself, was caught 23 runs in 100
// without it.
func TestDeriveSchema_ConcurrentCallersShareOneResult(t *testing.T) {
	t.Parallel()

	shared := map[string]any{"type": "object"}
	ShareSchema(shared)
	transform := "race|" + t.Name()
	const callers = 16
	results := make([]map[string]any, callers)
	allArrived, arrive := arrivalGate(callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			arrive()
			results[i], _ = DeriveSchema(shared, transform, func() any {
				<-allArrived
				return map[string]any{"type": "object", "properties": map[string]any{}}
			}).(map[string]any)
		})
	}
	wg.Wait()
	for i := 1; i < callers; i++ {
		if !sameMap(results[i], results[0]) {
			t.Fatalf("caller %d received a different map from caller 0", i)
		}
	}
}

// TestSharedSchemaIdentity_NamesRegisteredSchemasOnly verifies the composite
// key helper: a registered schema is named by its address, an unregistered
// one is not named at all.
func TestSharedSchemaIdentity_NamesRegisteredSchemasOnly(t *testing.T) {
	t.Parallel()

	private := map[string]any{"type": "object"}
	if name, ok := SharedSchemaIdentity(private); ok || name != "" {
		t.Fatalf("SharedSchemaIdentity(private) = %q, %t, want unnamed", name, ok)
	}
	compiled := &jsonschema.Schema{Type: "object"}
	ShareSchema(compiled)
	name, ok := SharedSchemaIdentity(compiled)
	if !ok || name != fmt.Sprintf("%p", compiled) {
		t.Fatalf("SharedSchemaIdentity(compiled) = %q, %t, want the pointer", name, ok)
	}
}

// TestParameterGuidanceIdentity_NamesContentNotAddress verifies the property
// the memo key depends on: nil and empty guidance name the same thing, two
// maps with the same content name the same thing however they were built, and
// any change to a field, to a confusion or to the parameter a field belongs to
// changes the name.
//
// Content and not address is what makes the name safe to put in a memo entry
// that outlives the map: an address can be handed out again to a different
// map, and the entry would then serve the wrong guidance.
func TestParameterGuidanceIdentity_NamesContentNotAddress(t *testing.T) {
	t.Parallel()

	if got := ParameterGuidanceIdentity(nil); got != "" {
		t.Errorf("ParameterGuidanceIdentity(nil) = %q, want empty", got)
	}
	if got := ParameterGuidanceIdentity(map[string]ParameterGuidance{}); got != "" {
		t.Errorf("ParameterGuidanceIdentity(empty) = %q, want empty", got)
	}

	guidance := func() map[string]ParameterGuidance {
		return map[string]ParameterGuidance{
			"project_id": {SemanticRole: "scope_project", ValueSource: "gitlab_project", ExampleBinding: "42", CommonConfusions: []string{"group_id", "name"}},
			"branch":     {SemanticRole: "ref"},
		}
	}
	name := ParameterGuidanceIdentity(guidance())
	if name == "" || name != ParameterGuidanceIdentity(guidance()) {
		t.Fatalf("ParameterGuidanceIdentity() = %q, want one stable non-empty name for equal content", name)
	}

	cases := map[string]func(map[string]ParameterGuidance){
		"a changed semantic role": func(g map[string]ParameterGuidance) { g["branch"] = ParameterGuidance{SemanticRole: "sha"} },
		"a changed value source":  func(g map[string]ParameterGuidance) { g["branch"] = ParameterGuidance{ValueSource: "ref"} },
		"a changed example":       func(g map[string]ParameterGuidance) { g["branch"] = ParameterGuidance{ExampleBinding: "main"} },
		"a changed confusion": func(g map[string]ParameterGuidance) {
			g["branch"] = ParameterGuidance{CommonConfusions: []string{"tag"}}
		},
		"a renamed parameter":      func(g map[string]ParameterGuidance) { g["ref"] = g["branch"]; delete(g, "branch") },
		"an extra parameter":       func(g map[string]ParameterGuidance) { g["state"] = ParameterGuidance{} },
		"a parameter that is gone": func(g map[string]ParameterGuidance) { delete(g, "branch") },
	}
	for label, edit := range cases {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			edited := guidance()
			edit(edited)
			if ParameterGuidanceIdentity(edited) == name {
				t.Errorf("ParameterGuidanceIdentity() named %s the same as the original", label)
			}
		})
	}
}

// TestCloneSchemaMap_DeepCopiesAndKeepsNil verifies the exported copy for
// callers with a reason to mutate: nested maps and slices are copied, and a
// nil map stays nil.
func TestCloneSchemaMap_DeepCopiesAndKeepsNil(t *testing.T) {
	t.Parallel()

	if got := CloneSchemaMap(nil); got != nil {
		t.Fatalf("CloneSchemaMap(nil) = %#v, want nil", got)
	}
	original := map[string]any{
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
		"required":   []string{"name"},
		"anyOf":      []any{map[string]any{"type": "object"}},
	}
	cloned := CloneSchemaMap(original)
	cloned["properties"].(map[string]any)["name"].(map[string]any)["type"] = "integer"
	cloned["required"].([]string)[0] = "changed"
	cloned["anyOf"].([]any)[0].(map[string]any)["type"] = "array"
	if original["properties"].(map[string]any)["name"].(map[string]any)["type"] != "string" ||
		original["required"].([]string)[0] != "name" ||
		original["anyOf"].([]any)[0].(map[string]any)["type"] != "object" {
		t.Fatalf("CloneSchemaMap() shares storage with its input: %#v", original)
	}
}
