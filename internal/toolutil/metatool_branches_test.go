// metatool_branches_test.go contains unit tests for rarely exercised
// branches in metatool.go: FieldTiers guards, schema-driven alias
// explanations (iid/environment_id), encoded-path decoding failures,
// structured value coercion fallbacks, UnmarshalParams retry paths,
// schema-based coercion no-ops, destructive-action confirmation in
// MakeMetaHandler, and enrichWithHints marshal failures.
package toolutil

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FieldTiers guards.

// TestFieldTiers_NilType_ReturnsNil verifies that [FieldTiers] returns nil
// for a nil reflect.Type instead of panicking.
func TestFieldTiers_NilType_ReturnsNil(t *testing.T) {
	if got := FieldTiers(nil); got != nil {
		t.Errorf("FieldTiers(nil) = %v, want nil", got)
	}
}

// TestFieldTiers_PointerType_Dereferences verifies that [FieldTiers]
// dereferences pointer types to their struct element before collecting
// tier tags.
func TestFieldTiers_PointerType_Dereferences(t *testing.T) {
	type tiered struct {
		Plan string `json:"plan" tier:"premium"`
	}
	got := FieldTiers(reflect.TypeFor[*tiered]())
	if got["plan"] != "premium" {
		t.Errorf("FieldTiers(*tiered) = %v, want plan:premium", got)
	}
}

// TestFieldTiers_NonStructType_ReturnsNil verifies that [FieldTiers] returns
// nil for non-struct kinds, which carry no field tier metadata.
func TestFieldTiers_NonStructType_ReturnsNil(t *testing.T) {
	if got := FieldTiers(reflect.TypeFor[string]()); got != nil {
		t.Errorf("FieldTiers(string) = %v, want nil", got)
	}
}

// TestFieldTiers_UnexportedField_Skipped verifies that unexported,
// non-anonymous struct fields are skipped while exported tier-tagged fields
// are still collected.
func TestFieldTiers_UnexportedField_Skipped(t *testing.T) {
	got := FieldTiers(reflect.TypeOf(struct {
		hidden string // present only to exercise the unexported-field skip branch via reflection.
		Public string `json:"public" tier:"ultimate"`
	}{}))
	if len(got) != 1 || got["public"] != "ultimate" {
		t.Errorf("FieldTiers(unexported+exported) = %v, want only public:ultimate", got)
	}
}

// Schema-driven alias explanations.

// TestNormalizeParamAliasesForSchemaWithExplanation_IIDAlias verifies that a
// bare "iid" parameter is explained as an alias of the schema's single
// *_iid property when the schema does not accept "iid" directly.
func TestNormalizeParamAliasesForSchemaWithExplanation_IIDAlias(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mr_iid": map[string]any{"type": "integer"},
		},
	}
	params := map[string]any{"iid": 5}

	_, explanations := NormalizeParamAliasesForSchemaWithExplanation(params, schema)

	found := false
	for _, e := range explanations {
		if e.Alias == "iid" && e.Canonical == "mr_iid" {
			found = true
		}
	}
	if !found {
		t.Errorf("explanations = %+v, want iid → mr_iid entry", explanations)
	}
}

// TestNormalizeParamAliasesForSchemaWithExplanation_EnvironmentIDAlias
// verifies that "environment_id" is explained as an alias of the schema's
// "environment" property when the schema accepts "environment" but not
// "environment_id".
func TestNormalizeParamAliasesForSchemaWithExplanation_EnvironmentIDAlias(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"environment": map[string]any{"type": "string"},
		},
	}
	params := map[string]any{"environment_id": 7}

	_, explanations := NormalizeParamAliasesForSchemaWithExplanation(params, schema)

	found := false
	for _, e := range explanations {
		if e.Alias == "environment_id" && e.Canonical == "environment" {
			found = true
		}
	}
	if !found {
		t.Errorf("explanations = %+v, want environment_id → environment entry", explanations)
	}
}

// Encoded path identifiers.

// TestDecodeEncodedPathIdentifier_InvalidEscape_ReturnsFalse verifies that a
// value containing "%2f" but with a malformed trailing escape sequence is
// rejected (url.PathUnescape error) instead of being partially decoded.
func TestDecodeEncodedPathIdentifier_InvalidEscape_ReturnsFalse(t *testing.T) {
	decoded, ok := decodeEncodedPathIdentifier("group%2fproject%")
	if ok || decoded != "" {
		t.Errorf("decodeEncodedPathIdentifier(invalid escape) = (%q, %v), want (\"\", false)", decoded, ok)
	}
}

// File path alias normalization.

// TestNormalizeFilePathAlias_ExistingFilename_NoRewrite verifies that
// normalizeFilePathAlias leaves params untouched when the output already
// carries a "filename" value, so a caller-provided filename is never
// overwritten by the file_path split.
func TestNormalizeFilePathAlias_ExistingFilename_NoRewrite(t *testing.T) {
	out := map[string]any{"file_path": "docs/readme.md", "filename": "custom.md"}
	accepts := func(name string) bool { return name == "path" || name == "filename" }
	cloned := false
	clone := func() map[string]any {
		cloned = true
		return maps.Clone(out)
	}

	normalizeFilePathAlias(out, accepts, clone)

	if cloned {
		t.Error("normalizeFilePathAlias cloned params despite existing filename")
	}
	if _, hasPath := out["path"]; hasPath {
		t.Errorf("normalizeFilePathAlias added path key: %v", out)
	}
}

// JSON field reflection fallbacks.

// EmbeddedID is a named non-struct type embedded anonymously in test structs
// to exercise the empty-JSON-name fallback: anonymous fields yield an empty
// jsonFieldName, and non-struct kinds cannot be flattened, so the collectors
// must fall back to the Go field name. Exported so the embedded field is not
// skipped by the unexported-field guard.
type EmbeddedID int

// TestCollectJSONFieldNames_AnonymousNonStruct_UsesGoFieldName verifies that
// an anonymous embedded non-struct field (which has no JSON tag name and
// cannot be flattened) falls back to its Go field name when collecting
// accepted parameter names.
func TestCollectJSONFieldNames_AnonymousNonStruct_UsesGoFieldName(t *testing.T) {
	fields := map[string]struct{}{}
	collectJSONFieldNames(reflect.TypeOf(struct{ EmbeddedID }{}), fields)
	if _, ok := fields["EmbeddedID"]; !ok {
		t.Errorf("collectJSONFieldNames(embedded non-struct) = %v, want EmbeddedID key", fields)
	}
}

// TestCollectJSONFieldTypes_AnonymousNonStruct_UsesGoFieldName verifies that
// jsonFieldTypes falls back to the Go field name for an anonymous embedded
// non-struct field, mirroring encoding/json field promotion.
func TestCollectJSONFieldTypes_AnonymousNonStruct_UsesGoFieldName(t *testing.T) {
	fields := jsonFieldTypes(reflect.TypeOf(struct{ EmbeddedID }{}))
	if got, ok := fields["EmbeddedID"]; !ok || got.Kind() != reflect.Int {
		t.Errorf("jsonFieldTypes(embedded non-struct) = %v, want EmbeddedID:int", fields)
	}
}

// UnmarshalParams edge cases.

// TestUnmarshalParams_UnmarshalableValue_ReturnsValidationError verifies
// that a parameter value that cannot be serialized to JSON (a function)
// produces a ParamValidationError from the initial json.Marshal step.
func TestUnmarshalParams_UnmarshalableValue_ReturnsValidationError(t *testing.T) {
	type input struct {
		X any `json:"x"`
	}
	_, err := UnmarshalParams[input](map[string]any{"x": func() {}})
	if err == nil {
		t.Fatal("UnmarshalParams(func value) error = nil, want marshal error")
	}
	var validationErr *ParamValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("UnmarshalParams(func value) error = %T, want *ParamValidationError", err)
	}
}

// TestUnmarshalParams_MapTarget_RetriesWithBroadCoercion verifies the legacy
// retry path: for a map target (no reflectable JSON fields, so the typed
// coercion pipeline is skipped) a numeric string fails strict unmarshalling
// first and then succeeds after coerceNumericStrings converts it.
func TestUnmarshalParams_MapTarget_RetriesWithBroadCoercion(t *testing.T) {
	got, err := UnmarshalParams[map[string]int](map[string]any{"n": "5"})
	if err != nil {
		t.Fatalf("UnmarshalParams(map target, numeric string) error = %v", err)
	}
	if got["n"] != 5 {
		t.Errorf("UnmarshalParams(map target) = %v, want n:5", got)
	}
}

// TestUnmarshalParams_MapTarget_NaNRetryMarshalFails verifies that when the
// broad numeric coercion retry produces a value JSON cannot encode (the
// string "NaN" parses to a float64 NaN), the retry marshal fails and the
// original strict unmarshal error is preserved.
func TestUnmarshalParams_MapTarget_NaNRetryMarshalFails(t *testing.T) {
	_, err := UnmarshalParams[map[string]int](map[string]any{"n": "NaN"})
	if err == nil {
		t.Fatal("UnmarshalParams(map target, NaN string) error = nil, want error")
	}
	var validationErr *ParamValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("UnmarshalParams(NaN string) error = %T, want *ParamValidationError", err)
	}
}

// Structured value coercion.

// TestCoerceStructuredValue_PointerSliceElem_WrapsObject verifies that a
// single JSON object destined for a slice of struct pointers is wrapped in
// a one-element list after the pointer element type is dereferenced.
func TestCoerceStructuredValue_PointerSliceElem_WrapsObject(t *testing.T) {
	type rule struct {
		A int `json:"a"`
	}
	coerced, changed := coerceStructuredValue("rules", map[string]any{"a": 1}, reflect.TypeFor[[]*rule]())
	if !changed {
		t.Fatal("coerceStructuredValue(map for []*struct) changed = false, want true")
	}
	items, ok := coerced.([]any)
	if !ok || len(items) != 1 {
		t.Errorf("coerceStructuredValue(map for []*struct) = %v, want single-item list", coerced)
	}
}

// TestCoerceStructuredValue_StringForStructSlice_Unchanged verifies that a
// scalar string that is neither an object, a role name, nor a list is left
// untouched for a slice-of-struct field.
func TestCoerceStructuredValue_StringForStructSlice_Unchanged(t *testing.T) {
	type rule struct {
		A int `json:"a"`
	}
	coerced, changed := coerceStructuredValue("rules", "hello", reflect.TypeFor[[]rule]())
	if changed || coerced != "hello" {
		t.Errorf("coerceStructuredValue(string for []struct) = (%v, %v), want (hello, false)", coerced, changed)
	}
}

// TestCoerceStructuredValue_NonRoleScalarItems_Unchanged verifies that list
// items that are neither objects nor GitLab role names are kept as-is and
// the overall value is reported unchanged.
func TestCoerceStructuredValue_NonRoleScalarItems_Unchanged(t *testing.T) {
	type rule struct {
		A int `json:"a"`
	}
	value := []any{"not-a-role"}
	coerced, changed := coerceStructuredValue("rules", value, reflect.TypeFor[[]rule]())
	if changed {
		t.Errorf("coerceStructuredValue(scalar items) changed = true, want false")
	}
	if !reflect.DeepEqual(coerced, value) {
		t.Errorf("coerceStructuredValue(scalar items) = %v, want %v", coerced, value)
	}
}

// TestNormalizeStructuredObjectFields_NoReflectableFields_ReturnsValue
// verifies that objects targeted at a struct without JSON fields are
// returned unchanged.
func TestNormalizeStructuredObjectFields_NoReflectableFields_ReturnsValue(t *testing.T) {
	value := map[string]any{"x": 1}
	got := normalizeStructuredObjectFields(value, reflect.TypeFor[struct{}]())
	if !reflect.DeepEqual(got, value) {
		t.Errorf("normalizeStructuredObjectFields(empty struct) = %v, want %v", got, value)
	}
}

// TestNormalizeStructuredObjectFields_ApprovalCountWithoutPrincipal_DefaultsAccessLevel
// verifies that an approval-rule object carrying only a required approval
// count (no access_level/user_id/group_id principal) receives the
// Maintainer access level 40 default.
func TestNormalizeStructuredObjectFields_ApprovalCountWithoutPrincipal_DefaultsAccessLevel(t *testing.T) {
	type approvalRule struct {
		RequiredApprovals int `json:"required_approvals"`
		AccessLevel       int `json:"access_level"`
	}
	got := normalizeStructuredObjectFields(map[string]any{"required_approvals": 2}, reflect.TypeFor[approvalRule]())
	if got["access_level"] != 40 {
		t.Errorf("normalizeStructuredObjectFields(count only)[access_level] = %v, want 40", got["access_level"])
	}
}

// Numeric slice coercion.

// TestCoerceSliceValueForTargetType_PointerElem_CoercesStrings verifies that
// pointer element types are dereferenced before the numeric-kind check so
// numeric strings inside a []*int value are still coerced.
func TestCoerceSliceValueForTargetType_PointerElem_CoercesStrings(t *testing.T) {
	coerced, changed, err := coerceSliceValueForTargetType("ids", []any{"5"}, reflect.TypeFor[*int]())
	if err != nil {
		t.Fatalf("coerceSliceValueForTargetType([]*int) error = %v", err)
	}
	if !changed {
		t.Fatal("coerceSliceValueForTargetType([]*int) changed = false, want true")
	}
	items, ok := coerced.([]any)
	if !ok || len(items) != 1 || items[0] != int64(5) {
		t.Errorf("coerceSliceValueForTargetType([]*int) = %v, want [5]", coerced)
	}
}

// TestCoerceSliceValueForTargetType_NonSliceValue_Unchanged verifies that a
// non-list value for a numeric slice element type is returned unchanged
// without error.
func TestCoerceSliceValueForTargetType_NonSliceValue_Unchanged(t *testing.T) {
	coerced, changed, err := coerceSliceValueForTargetType("ids", 42, reflect.TypeFor[int]())
	if err != nil || changed || coerced != 42 {
		t.Errorf("coerceSliceValueForTargetType(non-slice) = (%v, %v, %v), want (42, false, nil)", coerced, changed, err)
	}
}

// Schema-driven coercion no-ops.

// TestCoerceSchemaParamValue_NumberProperty_NonNumericString_Unchanged
// verifies that values for a number-typed schema property that cannot be
// parsed as floats are passed through unchanged.
func TestCoerceSchemaParamValue_NumberProperty_NonNumericString_Unchanged(t *testing.T) {
	property := map[string]any{"type": "number"}
	got, changed := coerceSchemaParamValue("weight", "abc", property)
	if changed || got != "abc" {
		t.Errorf("coerceSchemaParamValue(number, non-numeric) = (%v, %v), want (abc, false)", got, changed)
	}
}

// TestCoerceSchemaArrayValue_NonSliceValue_Unchanged verifies that a scalar
// value for an integer-array schema property is returned unchanged when it
// cannot be interpreted as a list.
func TestCoerceSchemaArrayValue_NonSliceValue_Unchanged(t *testing.T) {
	property := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "integer"},
	}
	got, changed := coerceSchemaArrayValue("abc", property)
	if changed || got != "abc" {
		t.Errorf("coerceSchemaArrayValue(non-slice) = (%v, %v), want (abc, false)", got, changed)
	}
}

// TestCoerceSingleStringArraysForSchema_NonStringValue_Skipped verifies that
// non-string values for a string-array schema property are not wrapped in a
// single-element list.
func TestCoerceSingleStringArraysForSchema_NonStringValue_Skipped(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"labels": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}
	params := map[string]any{"labels": 7}
	got := coerceSingleStringArraysForSchema(params, schema)
	if !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringArraysForSchema(non-string) = %v, want unchanged %v", got, params)
	}
}

// MakeMetaHandler destructive confirmation and error propagation.

// TestMakeMetaHandler_DestructiveDeclined_ReturnsCancelled verifies that a
// destructive route is intercepted by the elicitation confirmation flow and
// that a user decline cancels the action without invoking the handler.
func TestMakeMetaHandler_DestructiveDeclined_ReturnsCancelled(t *testing.T) {
	t.Setenv("YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")

	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})

	executed := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			executed = true
			return map[string]any{"ok": true}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	result, raw, err := handler(context.Background(), req, MetaToolInput{Action: "delete", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("handler(destructive declined) error = %v", err)
	}
	if executed {
		t.Error("destructive handler executed despite declined confirmation")
	}
	if raw != nil {
		t.Errorf("handler(destructive declined) raw = %v, want nil", raw)
	}
	if result == nil {
		t.Fatal("handler(destructive declined) result = nil, want cancellation result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "canceled") {
		t.Errorf("handler(destructive declined) content = %+v, want canceled message", result.Content)
	}
}

// TestMakeMetaHandler_HandlerError_Propagated verifies that a generic
// (non-validation) handler error is returned to the MCP framework unchanged
// instead of being converted into an in-band error result.
func TestMakeMetaHandler_HandlerError_Propagated(t *testing.T) {
	wantErr := errors.New("boom")
	routes := ActionMap{
		"fail": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return nil, wantErr
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "fail", Params: map[string]any{}})
	if !errors.Is(err, wantErr) {
		t.Errorf("handler(generic error) error = %v, want %v", err, wantErr)
	}
	if result != nil || raw != nil {
		t.Errorf("handler(generic error) result/raw = %v/%v, want nil/nil", result, raw)
	}
}

// TestMetaSchemaCompileKey_NonOpaqueMode_ReturnsEmpty verifies that schema
// compile caching is disabled (empty key) outside the default opaque
// META_PARAM_SCHEMA mode, where compact/full schemas have no cheap stable
// identity.
func TestMetaSchemaCompileKey_NonOpaqueMode_ReturnsEmpty(t *testing.T) {
	restore := SetMetaParamSchemaModeScoped(MetaParamSchemaCompact)
	defer restore()

	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{}, nil }),
	}
	if key := metaSchemaCompileKey("gitlab_test", routes); key != "" {
		t.Errorf("metaSchemaCompileKey(compact mode) = %q, want empty", key)
	}
}

// enrichWithHints marshal failure.

// TestEnrichWithHints_UnmarshalableResult_ReturnsOriginal verifies that a
// result value JSON cannot encode (a channel) is returned unchanged even
// when the call result carries next-step hints.
func TestEnrichWithHints_UnmarshalableResult_ReturnsOriginal(t *testing.T) {
	callResult := &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: "Done.\n\n💡 **Next steps:**\n- Use gitlab_list_projects\n"},
	}}
	result := make(chan int)

	got := enrichWithHints(result, callResult)

	gotChan, ok := got.(chan int)
	if !ok || gotChan != result {
		t.Errorf("enrichWithHints(chan) = %v, want original channel", got)
	}
}
