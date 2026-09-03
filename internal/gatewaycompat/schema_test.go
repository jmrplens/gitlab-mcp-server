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

// TestRewriteSchemaProse_WrongTypesInProsePositions_AreSkipped covers what the
// walk does when a document puts an unexpected type where the schema grammar
// promises one.
//
// Neither case is valid JSON Schema, and neither is hypothetical: the walk runs
// over whatever a tool declared, and a hand-written schema map is not validated
// before it is listed. The rule is that a value the walk cannot read is left
// alone rather than rewritten, replaced or panicked on — and that it does not
// stop the sibling keys, which is what a bare return here would have done.
func TestRewriteSchemaProse_WrongTypesInProsePositions_AreSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema map[string]any
	}{
		{
			name:   "a name-to-schema keyword holding something other than a map",
			schema: map[string]any{"properties": "not a map", "description": "kept;as is"},
		},
		{
			name:   "prose holding something other than a string",
			schema: map[string]any{"description": 42, "title": []any{"also not prose"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			before, err := json.Marshal(tt.schema)
			if err != nil {
				t.Fatalf("marshal the schema under test: %v", err)
			}

			changed := gatewaycompat.RewriteSchemaProse(tt.schema, func(text string) string {
				return gatewaycompat.Apply(semicolonToPeriod, text)
			})

			after, err := json.Marshal(tt.schema)
			if err != nil {
				t.Fatalf("marshal the walked schema: %v", err)
			}
			if changed != (string(before) != string(after)) {
				t.Errorf("RewriteSchemaProse reported changed=%v while the document went from %s to %s", changed, before, after)
			}
			if _, unreadable := tt.schema["description"].(int); unreadable && changed {
				t.Error("a non-string description was reported as rewritten")
			}
		})
	}
}
