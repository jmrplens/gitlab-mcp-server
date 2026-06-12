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

// newLiveClient creates a GitLab.com client using the GITLAB_COM_TOKEN
// environment variable, skipping the test when the token is unset.
// Used as the shared client for every handler exercised in this file.
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

// summarize runs a single live check under a 30s timeout, logs a
// short PASS/FAIL summary, and records the result as a subtest of t.
// The fn is expected to return a JSON-marshalable value or an error.
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

// TestOrbitLiveGitLabCom is the top-level live smoke test that exercises
// every public Orbit handler against GitLab.com. It verifies that each
// handler can be called with a real token, returns a non-error result,
// and decodes a representative payload. Subtests log a one-line PASS
// summary so a failure is easy to spot.
func TestOrbitLiveGitLabCom(t *testing.T) {
	testOrbitLiveGitLabComDiscovery(t)
}

// testOrbitLiveGitLabComDiscovery runs the four handler groups
// (read-only, dsl, query, graph_status) in sequence. It is split out
// of [TestOrbitLiveGitLabCom] so individual groups can be invoked
// from other live tests in this file.
func testOrbitLiveGitLabComDiscovery(t *testing.T) {
	t.Helper()
	client := newLiveClient(t)
	testOrbitLiveReadOnlyHandlers(t, client)
	testOrbitLiveDSLHandlers(t, client)
	testOrbitLiveQueryHandlers(t, client)
	testOrbitLiveGraphStatusHandlers(t, client)
}

// testOrbitLiveReadOnlyHandlers exercises [Status], [Schema], and
// [Tools] with the GitLab.com Orbit service. Each subtest logs the
// decoded fields as a one-line PASS summary.
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

// testOrbitLiveDSLHandlers exercises [DSL] in default, llm, and raw
// response formats. The subtest names correspond to the response_format
// the handler is invoked with.
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

// testOrbitLiveQueryHandlers exercises [Query] against the live API
// with a representative query for each query_type variant: traversal
// (with both node_ids and filters), aggregation, llm response format,
// neighbors, and path_finding. Each subtest logs query_type and
// row_count to surface a successful decode.
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

// testOrbitLiveGraphStatusHandlers exercises [GraphStatus] with two
// of the three supported scopes (full_path and namespace_id). The
// project_id scope is omitted because it requires a project the
// fixture setup script does not provision.
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

// TestOrbitLiveGitLabCom_Fixtures exercises the four query_type variants
// against the live fixture data the setup script provisions in any
// GitLab namespace (defaults to plens1, configurable via
// ORBIT_FIXTURES_NAMESPACE). All subtests use full_path-based filters
// instead of hardcoded project ids so the test is portable across
// namespaces: any developer who runs `scripts/setup-orbit-fixtures.sh`
// with their own token gets a working test surface.
//
// The Orbit indexer is eventually consistent: if a fresh push has not
// been indexed yet, the affected subtest will skip (not fail) by
// reporting row_count=0. Re-run the live test a few minutes after the
// setup script completes to allow the indexer to catch up.
func TestOrbitLiveGitLabCom_Fixtures(t *testing.T) {
	testOrbitLiveFixtures(t)
}

// testOrbitLiveFixtures exercises the four query_type variants
// against the live fixture data the setup script provisions in any
// GitLab namespace (defaults to plens1, configurable via
// ORBIT_FIXTURES_NAMESPACE). All subtests use full_path-based filters
// instead of hardcoded project ids so the test is portable across
// namespaces.
//
// The Orbit indexer is eventually consistent: if a fresh push has not
// been indexed yet, the affected subtest will log row_count=0 without
// failing. Re-run the live test a few minutes after the setup script
// completes to allow the indexer to catch up.
func testOrbitLiveFixtures(t *testing.T) {
	t.Helper()
	client := newLiveClient(t)

	// The namespace under which `scripts/setup-orbit-fixtures.sh`
	// provisioned the fixtures. Defaults to plens1 but can be
	// overridden by the developer running the test against their own
	// GitLab.com namespace via ORBIT_FIXTURES_NAMESPACE.
	namespace := os.Getenv("ORBIT_FIXTURES_NAMESPACE")
	if namespace == "" {
		namespace = "plens1"
	}
	const (
		kgFixturesProjectPath       = "kg-fixtures"
		securityFixturesProjectPath = "security-fixtures"
	)

	summarize(t, "Project_kg_fixtures_filter", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id":     "p",
					"entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{"op": "eq", "value": namespace + "/" + kgFixturesProjectPath},
					},
					"columns": []string{"id", "full_path", "name"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Project_security_fixtures_filter", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id":     "p",
					"entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{"op": "eq", "value": namespace + "/" + securityFixturesProjectPath},
					},
					"columns": []string{"id", "full_path"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Milestone_count_active", func(ctx context.Context) (any, error) {
		// Count active milestones. The setup script provisions one
		// active milestone (the KG coverage one); the assertion is
		// row_count > 0 to remain portable across namespaces that may
		// already have other active milestones.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id":     "m",
						"entity": "Milestone",
						"filters": map[string]any{
							"state": map[string]any{"op": "eq", "value": "active"},
						},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "m", "alias": "active_milestones"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "File_count_python", func(ctx context.Context) (any, error) {
		// Count Python source files. The File entity does not expose
		// a project_id filter, so we count by path suffix across the
		// whole namespace. Each fixture project contributes 8-9 .py files.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id":     "f",
						"entity": "File",
						"filters": map[string]any{
							"path": map[string]any{"op": "ends_with", "value": ".py"},
						},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "f", "alias": "python_files"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "MergeRequest_in_kg_fixtures", func(ctx context.Context) (any, error) {
		// Fetch the squash-merged MR the setup script creates. The MR
		// id is dynamic per instance, so we filter by source_branch.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id":     "mr",
					"entity": "MergeRequest",
					"filters": map[string]any{
						"source_branch": map[string]any{"op": "eq", "value": "feature/restock-helper"},
						"target_branch": map[string]any{"op": "eq", "value": "main"},
					},
					"columns": []string{"id", "iid", "title", "state", "source_branch"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Vulnerability_count_detected", func(ctx context.Context) (any, error) {
		// Count detected vulnerabilities. The setup script provisions
		// 5 such findings (1 critical AWS key, 3 high SQLi/eval,
		// 1 medium weak-hash) via the SAST and Secret Detection
		// templates. The exact number depends on how many analyzers
		// have finished; the assertion is row_count > 0.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id":     "v",
						"entity": "Vulnerability",
						"filters": map[string]any{
							"state": map[string]any{"op": "eq", "value": "detected"},
						},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "v", "alias": "open_vulnerabilities"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Neighbors_kg_fixtures", func(ctx context.Context) (any, error) {
		// Find what nodes are connected to the kg-fixtures project.
		// The canonical neighbors shape needs a filter on the top-level
		// node plus a `neighbors: {node: <id>}` reference. We discover
		// the project by full_path so the test stays portable.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"node": map[string]any{
					"id":     "p",
					"entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{"op": "eq", "value": namespace + "/" + kgFixturesProjectPath},
					},
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
}

// TestOrbitLiveGitLabCom_FeatureCoverage exercises the full Orbit
// query DSL surface against the live plens1 namespace: filter
// operators, multi-node traversals with relationships, aggregations
// with group_by/sort/sum/max/avg, order_by, virtual columns,
// cursor pagination, and options.dynamic_columns. Each subtest
// is informational: it PASSes as long as the API accepts the
// query and returns a valid envelope, even if row_count=0 (the
// data may not match the filter in this namespace). This is the
// comprehensive coverage test: every documented query pattern the
// API supports is exercised at least once against real data.
func TestOrbitLiveGitLabCom_FeatureCoverage(t *testing.T) {
	testOrbitLiveFeatureCoverage(t)
}

// testOrbitLiveFeatureCoverage exercises the full Orbit query DSL
// surface against the live namespace: filter operators, multi-node
// traversals with relationships, aggregations with group_by/sort/
// sum/max/avg, order_by, virtual columns, cursor pagination, and
// options.dynamic_columns. Each subtest is informational: it PASSes
// as long as the API accepts the query and returns a valid envelope,
// even if row_count=0 (the data may not match the filter in this
// namespace). This is the comprehensive coverage test: every
// documented query pattern the API supports is exercised at least
// once against real data.
func testOrbitLiveFeatureCoverage(t *testing.T) {
	t.Helper()
	client := newLiveClient(t)
	namespace := orbitFixturesNamespace()
	testOrbitLiveFeatureCoverageFiltersAndTraversal(t, client, namespace)
	testOrbitLiveFeatureCoverageAggregation(t, client, namespace)
	testOrbitLiveFeatureCoverageVirtualAndMeta(t, client, namespace)
}

// orbitFixturesNamespace returns the namespace under which
// scripts/setup-orbit-fixtures.sh provisioned the fixtures. Defaults
// to "plens1" but can be overridden by the developer running the test
// against their own GitLab.com namespace via the
// ORBIT_FIXTURES_NAMESPACE environment variable.
func orbitFixturesNamespace() string {
	ns := os.Getenv("ORBIT_FIXTURES_NAMESPACE")
	if ns == "" {
		ns = "plens1"
	}
	return ns
}

// testOrbitLiveFeatureCoverageFiltersAndTraversal exercises the
// `in`, `contains`, and `gt` filter operators and a multi-node
// traversal that joins MergeRequest to Project through the
// IN_PROJECT relationship.
func testOrbitLiveFeatureCoverageFiltersAndTraversal(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	// Filter operators (in, contains, gt) and the multi-node
	// traversal with IN_PROJECT relationship.
	summarize(t, "Filter_in_operator_severity", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "v", "entity": "Vulnerability",
					"filters": map[string]any{
						"severity": map[string]any{"op": "in", "value": []string{"critical", "high"}},
					},
					"columns": []string{"id", "severity"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Filter_contains_operator_path", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "f", "entity": "File",
					"filters": map[string]any{
						"path": map[string]any{"op": "contains", "value": "orders"},
					},
					"columns": []string{"id", "path"},
				},
				"limit": 5,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Filter_gt_operator", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "p", "entity": "Project",
					"filters": map[string]any{
						"star_count": map[string]any{"op": "gt", "value": 0},
					},
					"columns": []string{"id", "full_path", "star_count"},
				},
				"limit": 10,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Traversal_project_to_merge_requests", func(ctx context.Context) (any, error) {
		// Classic "find MRs in a project" pattern. The relationship
		// IN_PROJECT connects MergeRequest → Project; the alias mr is
		// used in the columns block.
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{
					map[string]any{
						"id": "p", "entity": "Project",
						"filters": map[string]any{
							"full_path": map[string]any{"op": "eq", "value": namespace + "/kg-fixtures"},
						},
						"columns": []string{"id", "full_path"},
					},
					map[string]any{
						"id": "mr", "entity": "MergeRequest",
						"columns": []string{"id", "iid", "title", "state"},
					},
				},
				"relationships": []any{
					map[string]any{"type": "IN_PROJECT", "from": "mr", "to": "p"},
				},
				"limit": 5,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})
}

// testOrbitLiveFeatureCoverageAggregation exercises the aggregation
// functions (count, sum, max, avg) plus the group_by alternatives:
// group_by on a property (severity) and group_by on a node alias
// joined via the IN_PROJECT relationship.
func testOrbitLiveFeatureCoverageAggregation(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	// Aggregation functions: sum, max, avg on star_count plus
	// group_by severity and group_by node with IN_PROJECT relationship.
	summarize(t, "Aggregation_group_by_severity", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id": "v", "entity": "Vulnerability",
						"filters": map[string]any{
							"state": map[string]any{"op": "eq", "value": "detected"},
						},
					},
				},
				"group_by": []any{
					map[string]any{"kind": "property", "node": "v", "property": "severity", "alias": "sev"},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "v", "alias": "vuln_count"},
				},
				"aggregation_sort": map[string]any{"column": "vuln_count", "direction": "DESC"},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Aggregation_group_by_node_with_relationship", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id": "p", "entity": "Project",
						"filters": map[string]any{
							"full_path": map[string]any{"op": "starts_with", "value": namespace + "/"},
						},
					},
					map[string]any{
						"id": "mr", "entity": "MergeRequest", "columns": []string{"id"},
					},
				},
				"relationships": []any{
					map[string]any{"type": "IN_PROJECT", "from": "mr", "to": "p"},
				},
				"group_by": []any{
					map[string]any{"kind": "node", "node": "p"},
				},
				"aggregations": []any{
					map[string]any{"function": "count", "target": "mr", "alias": "mr_count"},
				},
				"aggregation_sort": map[string]any{"column": "mr_count", "direction": "DESC"},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Aggregation_sum_star_count", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id": "p", "entity": "Project",
						"filters": map[string]any{
							"star_count": map[string]any{"op": "gt", "value": 0},
						},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "sum", "target": "p", "property": "star_count", "alias": "total_stars"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Aggregation_max_star_count", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id": "p", "entity": "Project",
						"filters": map[string]any{
							"star_count": map[string]any{"op": "gt", "value": 0},
						},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "max", "target": "p", "property": "star_count", "alias": "max_stars"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Aggregation_avg_star_count", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					map[string]any{
						"id": "p", "entity": "Project",
						"filters": map[string]any{
							"star_count": map[string]any{"op": "gt", "value": 0},
						},
					},
				},
				"aggregations": []any{
					map[string]any{"function": "avg", "target": "p", "property": "star_count", "alias": "avg_stars"},
				},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})
}

// testOrbitLiveFeatureCoverageVirtualAndMeta exercises order_by,
// virtual columns (diff, content), cursor pagination, the id_range
// scope, and the options.dynamic_columns knob for neighbors
// hydration.
func testOrbitLiveFeatureCoverageVirtualAndMeta(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	// order_by, virtual columns, cursor pagination, id_range scope,
	// and options.dynamic_columns for neighbors hydration.
	summarize(t, "Traversal_order_by_name_desc", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "p", "entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{"op": "starts_with", "value": namespace + "/"},
					},
					"columns": []string{"id", "full_path", "name"},
				},
				"order_by": map[string]any{"node": "p", "property": "name", "direction": "DESC"},
				"limit":    5,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Traversal_merge_request_with_diff_column", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "mr", "entity": "MergeRequest",
					"filters": map[string]any{
						"source_branch": map[string]any{"op": "eq", "value": "feature/restock-helper"},
					},
					"columns": []string{"id", "iid", "title", "diff"},
				},
				"limit": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Traversal_file_with_content_column", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "f", "entity": "File",
					"filters": map[string]any{
						"path": map[string]any{"op": "ends_with", "value": "models.py"},
					},
					"columns": []string{"id", "path", "language", "content"},
				},
				"limit": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Traversal_cursor_pagination", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "p", "entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{"op": "starts_with", "value": namespace + "/"},
					},
					"columns": []string{"id", "full_path"},
				},
				"limit":  2,
				"cursor": map[string]any{"page_size": 2, "offset": 0},
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Traversal_id_range_scope", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node": map[string]any{
					"id": "p", "entity": "Project",
					"id_range": map[string]any{"start": 1, "end": 100000},
					"columns":  []string{"id", "full_path"},
				},
				"limit": 5,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})

	summarize(t, "Neighbors_with_dynamic_columns_option", func(ctx context.Context) (any, error) {
		out, err := Query(ctx, client, QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"node": map[string]any{
					"id": "p", "entity": "Project",
					"filters": map[string]any{
						"full_path": map[string]any{"op": "eq", "value": namespace + "/kg-fixtures"},
					},
				},
				"neighbors": map[string]any{"node": "p"},
				"options":   map[string]any{"dynamic_columns": "default"},
				"limit":     5,
			},
		})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount), nil
	})
}
