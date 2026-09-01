// schema_test.go verifies the shared schema prose walk directly: which keys
// count as prose, which as data, and how the walk reads a keyword position
// differently from a property-name position.
package gatewaycompat_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
)

// rewriteUpper is a visible, non-idempotent stand-in for a substitution.
func rewriteUpper(text string) string {
	return text + "!"
}

// decode round-trips a schema literal through JSON so the walk sees the
// map[string]any shape the middleware feeds it.
func decode(t *testing.T, schema string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(schema), &v); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return v
}

// TestRewriteSchemaProse_KeywordPositions_RewritesOnlyProse verifies that a
// description or title keyword is rewritten wherever it nests (properties,
// items, anyOf, $defs) while pattern, const, enum, examples, default and
// required survive verbatim, subtrees included.
func TestRewriteSchemaProse_KeywordPositions_RewritesOnlyProse(t *testing.T) {
	v := decode(t, `{
		"title": "Root",
		"description": "root desc",
		"pattern": "p;q",
		"const": "c;c",
		"required": ["description"],
		"enum": ["e;1", {"description": "not prose, data"}],
		"examples": [{"description": "not prose, data"}],
		"default": {"description": "not prose, data"},
		"properties": {
			"a": {"description": "a desc", "items": {"title": "item title"}}
		},
		"anyOf": [{"description": "variant desc"}],
		"$defs": {"shared": {"description": "def desc"}}
	}`)

	if !gatewaycompat.RewriteSchemaProse(v, rewriteUpper) {
		t.Fatal("RewriteSchemaProse reported no change on a schema full of prose")
	}

	want := decode(t, `{
		"title": "Root!",
		"description": "root desc!",
		"pattern": "p;q",
		"const": "c;c",
		"required": ["description"],
		"enum": ["e;1", {"description": "not prose, data"}],
		"examples": [{"description": "not prose, data"}],
		"default": {"description": "not prose, data"},
		"properties": {
			"a": {"description": "a desc!", "items": {"title": "item title!"}}
		},
		"anyOf": [{"description": "variant desc!"}],
		"$defs": {"shared": {"description": "def desc!"}}
	}`)
	if !reflect.DeepEqual(v, want) {
		t.Errorf("walk result:\n%#v\nwant:\n%#v", v, want)
	}
}

// TestRewriteSchemaProse_PropertyNamedLikeKeyword_TreatedAsSchema verifies
// the position rule: below properties, a key is a user-chosen name, so a
// property named "default" is descended (its prose rewritten) and a property
// named "description" holds a schema, not prose — while the same words in
// keyword position keep their keyword meaning.
func TestRewriteSchemaProse_PropertyNamedLikeKeyword_TreatedAsSchema(t *testing.T) {
	v := decode(t, `{
		"properties": {
			"default": {"description": "prose in a property named default", "default": "d;d"},
			"description": {"description": "prose about the description param"}
		}
	}`)

	if !gatewaycompat.RewriteSchemaProse(v, rewriteUpper) {
		t.Fatal("RewriteSchemaProse reported no change")
	}

	want := decode(t, `{
		"properties": {
			"default": {"description": "prose in a property named default!", "default": "d;d"},
			"description": {"description": "prose about the description param!"}
		}
	}`)
	if !reflect.DeepEqual(v, want) {
		t.Errorf("walk result:\n%#v\nwant:\n%#v", v, want)
	}
}

// TestRewriteSchemaProse_NoProse_ReportsUnchanged verifies the changed flag:
// a schema whose only strings are contract reports false, which is what lets
// the middleware keep the original typed schema value untouched.
func TestRewriteSchemaProse_NoProse_ReportsUnchanged(t *testing.T) {
	v := decode(t, `{"type": "object", "pattern": "a;b", "enum": ["x;y"]}`)
	if gatewaycompat.RewriteSchemaProse(v, rewriteUpper) {
		t.Error("RewriteSchemaProse reported a change on a schema with no prose")
	}
}
