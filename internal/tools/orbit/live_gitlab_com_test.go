//go:build orbitlive

// live_gitlab_com_test.go exercises every public orbit handler against
// https://gitlab.com using the GITLAB_COM_TOKEN from the environment.
// It is gated behind the `orbitlive` build tag so it never runs in the
// normal `go test ./...` sweep. Run it explicitly with:
//
//	GITLAB_COM_TOKEN=... go test -tags orbitlive -count=1 -v ./internal/tools/orbit/
//
// The test reports each handler as PASS / FAIL with a one-line summary so
// we can see at a glance which tools are wired correctly to the live API.
package orbit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

const liveGitLabComURL = "https://gitlab.com"

func newLiveClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	token := os.Getenv("GITLAB_COM_TOKEN")
	if token == "" {
		t.Skip("GITLAB_COM_TOKEN not set; skipping live test against gitlab.com")
	}
	client, err := gitlabclient.NewClientWithToken(liveGitLabComURL, token, false)
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}
	return client
}

func summarize(t *testing.T, name string, fn func(context.Context) (any, error)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, err := fn(ctx)
		if err != nil {
			t.Errorf("FAIL %s: %v", name, err)
			return
		}
		b, mErr := json.Marshal(out)
		if mErr != nil {
			t.Logf("OK %s (non-JSON output): %T", name, out)
			return
		}
		preview := string(b)
		if len(preview) > 400 {
			preview = preview[:400] + "...(truncated)"
		}
		t.Logf("OK %s: %s", name, preview)
	})
}

func TestOrbitLiveGitLabCom(t *testing.T) {
	testOrbitLiveGitLabComDiscovery(t)
}

func testOrbitLiveGitLabComDiscovery(t *testing.T) {
	t.Helper()
	client := newLiveClient(t)
	testOrbitLiveReadOnlyHandlers(t, client)
	testOrbitLiveDSLHandlers(t, client)
	testOrbitLiveQueryHandlers(t, client)
	testOrbitLiveGraphStatusHandlers(t, client)
}

func testOrbitLiveReadOnlyHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	summarize(t, "Status", func(ctx context.Context) (any, error) {
		out, err := Status(ctx, client, StatusInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("status=%s version=%s components=%d",
			out.Status, out.Version, len(out.Components)), nil
	})

	summarize(t, "Schema", func(ctx context.Context) (any, error) {
		out, err := Schema(ctx, client, SchemaInput{})
		if err != nil {
			return nil, err
		}
		domainCount := len(out.Domains)
		nodeCount := len(out.Nodes)
		edgeCount := len(out.Edges)
		return fmt.Sprintf("schema_version=%s domains=%d nodes=%d edges=%d",
			out.SchemaVersion, domainCount, nodeCount, edgeCount), nil
	})

	summarize(t, "Tools", func(ctx context.Context) (any, error) {
		out, err := Tools(ctx, client, ToolsInput{})
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(out.Tools))
		for _, tool := range out.Tools {
			names = append(names, tool.Name)
		}
		return fmt.Sprintf("tools=%d names=[%s]", len(out.Tools), strings.Join(names, ",")), nil
	})
}

func testOrbitLiveDSLHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	summarize(t, "DSL_default", func(ctx context.Context) (any, error) {
		out, err := DSL(ctx, client, DSLInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.Content)), nil
	})

	summarize(t, "DSL_llm", func(ctx context.Context) (any, error) {
		out, err := DSL(ctx, client, DSLInput{ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"}})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.Content)), nil
	})

	summarize(t, "DSL_raw", func(ctx context.Context) (any, error) {
		out, err := DSL(ctx, client, DSLInput{ResponseFormatInput: ResponseFormatInput{ResponseFormat: "raw"}})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.Content)), nil
	})
}

func testOrbitLiveQueryHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	summarize(t, "Query_traversal_minimal", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id":       "proj",
					"entity":   "Project",
					"node_ids": []int{83009763},
					"columns":  []string{"id"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Query_traversal_with_filter", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id":     "p",
					"entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{
							"op":    "starts_with",
							"value": "plens1/",
						},
					},
					"columns": []string{"id"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Query_aggregation", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id":      "p",
						"entity":  "Project",
						"columns": []string{"id"},
						"filters": map[string]any{
							"full_path": map[string]any{"op": "starts_with", "value": "plens1/"},
						},
					},
				},
				"aggregations": []any{
					map[string]any{
						"function": "count",
						"target":   "p",
						"alias":    "project_count",
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Query_llm_format", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"},
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id":      "proj",
					"entity":  "Project",
					"columns": []string{"id"},
					"filters": map[string]any{
						"full_path": map[string]any{
							"op":    "starts_with",
							"value": "plens1/",
						},
					},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		preview := out.FormattedText
		if len(preview) > 200 {
			preview = preview[:200] + "...(truncated)"
		}
		return fmt.Sprintf("query_type=%s formatted_text=%q", out.QueryType, preview), nil
	})

	summarize(t, "Query_neighbors", func(ctx context.Context) (any, error) {
		// Canonical neighbors shape: a top-level `node` (bounded by
		// node_ids/filters) plus a `neighbors: {node: <id>}` reference.
		// `neighbors.node` is a string that references the top-level
		// node's `id` (not the entity name).
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"node": map[string]any{
					"id":       "p",
					"entity":   "Project",
					"node_ids": []int{83009763},
				},
				"neighbors": map[string]any{
					"node": "p",
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Query_path_finding", func(ctx context.Context) (any, error) {
		// Canonical path_finding shape: two top-level `nodes` (from/to)
		// with node_ids, plus `path: {type: algorithm, from: <id>, to: <id>,
		// max_depth: 1..3}`. `path.type` is the algorithm enum
		// (shortest | all_shortest | any); `from`/`to` are string id
		// references to the top-level nodes.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "path_finding",
				"nodes": []any{
					map[string]any{
						"id":       "u",
						"entity":   "User",
						"node_ids": []int{15767218},
					},
					map[string]any{
						"id":       "p",
						"entity":   "Project",
						"node_ids": []int{83009763},
					},
				},
				"path": map[string]any{
					"type":      "shortest",
					"from":      "u",
					"to":        "p",
					"max_depth": 3,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})
}

func testOrbitLiveGraphStatusHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	summarize(t, "GraphStatus_full_path", func(ctx context.Context) (any, error) {
		out, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "plens1"})
		if err != nil {
			return nil, err
		}
		projects := "<nil>"
		if out.Projects != nil {
			projects = fmt.Sprintf("indexed=%d total=%d", out.Projects.Indexed, out.Projects.TotalKnown)
		}
		state := "<nil>"
		if out.Indexing != nil {
			state = out.Indexing.State
		}
		return fmt.Sprintf("projects=%s indexing.state=%s", projects, state), nil
	})

	summarize(t, "GraphStatus_namespace_id", func(ctx context.Context) (any, error) {
		out, err := GraphStatus(ctx, client, GraphStatusInput{NamespaceID: 134059988})
		if err != nil {
			return nil, err
		}
		projects := "<nil>"
		if out.Projects != nil {
			projects = fmt.Sprintf("indexed=%d total=%d", out.Projects.Indexed, out.Projects.TotalKnown)
		}
		return "projects=" + projects, nil
	})
}

// TestOrbitLiveGitLabCom_ShapeDiscovery keeps regression coverage of the
// canonical Query DSL shapes for each of the four query_type variants.
// These shapes were discovered by probing the live GitLab.com API; they
// are the smallest inputs that succeed for each variant. If GitLab ever
// tightens validation, these subtests will fail first and the docstring
// on [QueryInput] should be updated to match the new contract.
func TestOrbitLiveGitLabCom_ShapeDiscovery(t *testing.T) {
	client := newLiveClient(t)

	// Aggregation with a filter scope: canonical shape is `nodes: [{...,
	// filters: {...}}]` plus `aggregations: [{function, target, alias}]`.
	summarize(t, "Aggregation_with_filter", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id":     "p",
						"entity": "Project",
						"filters": map[string]any{
							"full_path": map[string]any{"op": "starts_with", "value": "plens1/"},
						},
						"columns": []string{"id"},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "p", "alias": "count_projects"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	// Aggregation with explicit node_ids: alternative to filters.
	summarize(t, "Aggregation_with_node_ids", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id":       "p",
						"entity":   "Project",
						"node_ids": []int{83009763, 83009767, 83009772},
						"columns":  []string{"id"},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "p", "alias": "count_projects"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	// Neighbors: a top-level `node` (bounded) + `neighbors: {node: <id>}`.
	// The `neighbors.node` value is the top-level node's `id` (not the
	// entity name).
	summarize(t, "Neighbors_id_reference", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"node": map[string]any{
					"id":       "p",
					"entity":   "Project",
					"node_ids": []int{83009763},
				},
				"neighbors": map[string]any{
					"node": "p",
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	// Path_finding: two top-level `nodes` (from/to) + `path: {type, from, to,
	// max_depth}`. `path.type` is the algorithm (shortest | all_shortest |
	// any); from/to are id references to the top-level nodes.
	summarize(t, "PathFinding_shortest", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "path_finding",
				"nodes": []any{
					map[string]any{
						"id":       "u",
						"entity":   "User",
						"node_ids": []int{15767218},
					},
					map[string]any{
						"id":       "p",
						"entity":   "Project",
						"node_ids": []int{83009763},
					},
				},
				"path": map[string]any{
					"type":      "shortest",
					"from":      "u",
					"to":        "p",
					"max_depth": 3,
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	// Schema with the default response format (handler omits the
	// response_format parameter so the API applies its own default of "json").
	summarize(t, "Schema_default_format", func(ctx context.Context) (any, error) {
		out, err := Schema(ctx, client, SchemaInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("schema_version=%s domains=%d nodes=%d edges=%d",
			out.SchemaVersion, len(out.Domains), len(out.Nodes), len(out.Edges)), nil
	})
}
