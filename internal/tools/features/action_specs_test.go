// action_specs_test.go covers the route this package contributes to the
// canonical action catalog. The specs themselves live in
// internal/tools/adminspecs and are tested there.
package features

import (
	"net/http"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const actionSpecFeatureJSON = `{"name":"flag1","state":"on","gates":[{"key":"boolean","value":true}]}`

// TestSetRoute_InputSchema_AcceptsTheThreeShapesAGateTakes verifies that the
// route widens the reflected schema for `value`.
//
// It is the reason this route exists at all: a feature gate is set to a
// boolean, a percentage or a string, and the schema reflection over SetInput
// produces one type for the field. A model reading a schema that names a single
// type sends only that shape, so the widening is what makes two thirds of the
// endpoint reachable.
func TestSetRoute_InputSchema_AcceptsTheThreeShapesAGateTakes(t *testing.T) {
	route := SetRoute(testutil.NewTestClient(t, featureActionHandler()))

	properties, ok := route.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties schema has type %T, want map[string]any", route.InputSchema["properties"])
	}
	value, ok := properties["value"].(map[string]any)
	if !ok {
		t.Fatalf("value schema has type %T, want map[string]any", properties["value"])
	}
	if _, typed := value["type"]; typed {
		t.Errorf("value schema still names one type: %v", value)
	}
	oneOf, ok := value["oneOf"].([]any)
	if !ok {
		t.Fatalf("value oneOf has type %T, want []any", value["oneOf"])
	}
	// The set and not the count: three alternatives that repeat a type, or
	// name one a gate cannot take, would still be three.
	var got []string
	for _, alternative := range oneOf {
		shape, isMap := alternative.(map[string]any)
		if !isMap {
			t.Fatalf("value oneOf alternative has type %T, want map[string]any", alternative)
		}
		kind, isString := shape["type"].(string)
		if !isString {
			t.Fatalf("value oneOf alternative %v names no type", shape)
		}
		got = append(got, kind)
	}
	slices.Sort(got)
	if want := []string{"boolean", "integer", "string"}; !slices.Equal(got, want) {
		t.Fatalf("value oneOf types = %v, want %v", got, want)
	}
}

// TestSetRoute_Handler_SetsTheFlag verifies that the widened route still
// reaches the handler it wraps, so the schema rewrite is not paid for with a
// route that cannot run.
func TestSetRoute_Handler_SetsTheFlag(t *testing.T) {
	route := SetRoute(testutil.NewTestClient(t, featureActionHandler()))

	result, err := route.Handler(t.Context(), map[string]any{"name": "flag1", "value": true})
	if err != nil {
		t.Fatalf("Handler() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("Handler() returned nil")
	}
}

func featureActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /api/v4/features/flag1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecFeatureJSON)
	})
	return handler
}
