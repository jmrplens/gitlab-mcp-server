//go:build e2e && !enterprise

// search_type_ce_test.go validates the search_type parameter added to GitLab
// search tools. It uses only basic search for live GitLab calls because
// advanced and Zoekt availability depends on the target instance configuration.
package suite

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/search"
)

// TestSearchType_BasicSearchWorks verifies search_type=basic is accepted by
// both individual and meta search tools against a real GitLab project.
func TestSearchType_BasicSearchWorks(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)
	searchToken := "unique-e2e-search-type-basic-token"
	commitFile(ctx, t, sess.individual, proj, defaultBranch, "search-type-basic.txt", searchToken, "add basic search_type content")

	drainSidekiq(ctx, t, sess.glClient)

	t.Run("IndividualCodeBasic", func(t *testing.T) {
		out, err := callToolOn[search.CodeOutput](ctx, sess.individual, "gitlab_search_code", search.CodeInput{
			ProjectID: proj.pidOf(),
			Query:     searchToken,
			TypeInput: search.TypeInput{SearchType: "basic"},
		})
		requireNoError(t, err, "search code with basic search_type")
		requireTruef(t, len(out.Blobs) >= 1, "expected >=1 code result with basic search_type, got %d", len(out.Blobs))
	})

	t.Run("MetaCodeBasic", func(t *testing.T) {
		out, err := callToolOn[search.CodeOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "code",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"query":       searchToken,
				"search_type": "basic",
			},
		})
		requireNoError(t, err, "meta search code with basic search_type")
		requireTruef(t, len(out.Blobs) >= 1, "expected >=1 meta code result with basic search_type, got %d", len(out.Blobs))
	})
}

// TestSearchType_InvalidValueFails verifies invalid search_type values return
// actionable errors before depending on any GitLab backend behavior.
func TestSearchType_InvalidValueFails(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("IndividualInvalidSearchType", func(t *testing.T) {
		_, err := callToolOn[search.CodeOutput](ctx, sess.individual, "gitlab_search_code", search.CodeInput{
			Query:     "anything",
			TypeInput: search.TypeInput{SearchType: "semantic"},
		})
		requireSearchTypeError(t, err)
	})

	t.Run("MetaInvalidSearchType", func(t *testing.T) {
		_, err := callToolOn[search.CodeOutput](ctx, sess.meta, "gitlab_search", map[string]any{
			"action": "code",
			"params": map[string]any{
				"query":       "anything",
				"search_type": "semantic",
			},
		})
		requireSearchTypeError(t, err)
	})
}

// TestSearchType_SchemasExposeEnum verifies search_type is constrained in the
// live individual tools/list schema and the meta-tool detail resource.
func TestSearchType_SchemasExposeEnum(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.glClient == nil {
		t.Skip("tool manifest session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	toolsResult, err := sess.individual.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	individualTool := findE2ETool(t, toolsResult.Tools, "gitlab_search_code")
	requireE2ESearchTypeEnum(t, schemaMapFromAny(t, individualTool.InputSchema))

	metaSession := toolManifestResourceSession(t, sess.glClient, sess.enterprise)
	resource, err := metaSession.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "gitlab://tools/gitlab_search.code",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(resource.Contents) != 1 {
		t.Fatalf("contents = %d, want 1", len(resource.Contents))
	}

	var detail map[string]any
	if unmarshalErr := json.Unmarshal([]byte(resource.Contents[0].Text), &detail); unmarshalErr != nil {
		t.Fatalf("tool detail is not valid JSON: %v", unmarshalErr)
	}
	requireE2ESearchTypeEnum(t, schemaMapFromAny(t, detail["input_schema"]))
}

func requireSearchTypeError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected search_type error, got nil")
	}
	message := err.Error()
	if !strings.Contains(message, "search_type") {
		t.Fatalf("expected search_type in error, got: %v", err)
	}
	for _, want := range []string{"basic", "advanced", "zoekt"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected search_type choice %q in error, got: %v", want, err)
		}
	}
}

func findE2ETool(t *testing.T, tools []*mcp.Tool, name string) *mcp.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func schemaMapFromAny(t *testing.T, raw any) map[string]any {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema map[string]any
	if unmarshalErr := json.Unmarshal(data, &schema); unmarshalErr != nil {
		t.Fatalf("unmarshal schema: %v", unmarshalErr)
	}
	return schema
}

func requireE2ESearchTypeEnum(t *testing.T, schema map[string]any) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing or invalid: %#v", schema["properties"])
	}
	searchType, ok := properties["search_type"].(map[string]any)
	if !ok {
		t.Fatalf("search_type property missing or invalid: %#v", properties["search_type"])
	}
	got, ok := searchType["enum"].([]any)
	if !ok {
		t.Fatalf("search_type enum missing or invalid: %#v", searchType["enum"])
	}
	want := []any{"basic", "advanced", "zoekt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search_type enum = %#v, want %#v", got, want)
	}
}
