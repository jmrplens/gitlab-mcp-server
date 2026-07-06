// mdregistry_query_test.go contains unit tests for the registry query helpers
// [HasRegisteredMarkdownFormatter] and [MarkdownFormatterCount], covering the
// nil/interface guards, the reflect.Type and plain-value lookup paths, pointer
// dereferencing, and both the string-formatter and result-formatter registries.
package toolutil

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

// TestHasRegisteredMarkdownFormatter_ReflectType_ReturnsTrue verifies the
// canonical reflect.Type lookup path: DeleteOutput is registered by the
// package init, so its reflect.Type must resolve to a formatter.
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
