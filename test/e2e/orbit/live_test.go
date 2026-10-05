//go:build orbitlive

// Package orbit contains live integration tests against the experimental
// GitLab Orbit Knowledge Graph API on GitLab.com. The tests hit the real
// https://gitlab.com/api/v4/orbit/* endpoints (no httptest mocks) and
// exercise every public orbit handler in internal/tools/orbit plus the
// full query DSL surface end-to-end.
//
// # Build tag
//
// All tests are gated behind the `orbitlive` build tag so they never run
// in the default `go test ./...` sweep. Run them explicitly with:
//
//	set -a && . ./.env && set +a && \
//	  go test -tags orbitlive -count=1 -v -timeout 240s ./test/e2e/orbit/
//
// Or end-to-end (provisions fixtures, waits for the indexer, runs the
// tests) via the project Make target:
//
//	set -a && . ./.env && set +a && \
//	  make test-e2e-gitlab-com
//
// # Required environment
//
//   - GITLAB_COM_TOKEN — a GitLab.com personal access token with api scope,
//     used to authenticate every request.
//   - ORBIT_FIXTURES_NAMESPACE — optional. Overrides the default `plens1`
//     namespace against which the fixture-driven subtests run. Set this
//     when developing against your own GitLab.com namespace.
//
// # The query language
//
// Every query here is written in version 12 of the Orbit query DSL, the one
// GitLab.com serves (graph_query/v12, read through orbit.dsl): a nodes list
// even for one node, filters as a bare value or an object keyed by operators,
// a neighbors query whose center is its one node, a path query that names its
// relationship types, function-keyed aggregations and string orderings. A
// query GitLab cannot compile fails its subtest by name ([liveQuery]), and
// TestOrbitLiveGitLabCom_DSL fails when GitLab.com serves another major
// version of the language than the one orbit.query teaches (issue 1031).
//
// No project, user or namespace id is written down: each is found by its
// path in the fixture namespace, since re-provisioning the fixtures gives
// them new ids.
//
// # Layout
//
// The test file uses the external `orbit_test` package so it is
// co-located with the rest of the e2e surface and can be run or evolved
// independently of the GitLab suite in test/e2e/gitlab/.
// See docs/development/orbit-fixtures.md for the fixture layout, the
// data the fixture-driven subtests expect, and the Orbit indexer
// eventually-consistent caveat.
package orbit_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	// Import the orbit package under test. The test lives in
	// test/e2e/orbit/ (external test package) so it can also be
	// runnable on its own against any GitLab instance with the
	// `orbitlive` build tag, without pulling in the full e2e
	// suite from test/e2e/gitlab/.
	orbit "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/orbit"
)

// liveGitLabComURL is the base URL for the real GitLab.com REST API
// exercised by every handler test in this file. Pinned to gitlab.com
// (no override) because the Orbit Knowledge Graph API is currently a
// GitLab.com-only experimental surface.
const liveGitLabComURL = "https://gitlab.com"

// taughtDSLSchema is the $id of the query DSL orbit.query's guidance teaches
// (internal/tools/orbit, QueryInput). TestOrbitLiveGitLabCom_DSL holds what
// GitLab.com serves to it, so a new major version of the language fails the
// suite rather than the guidance going stale in silence.
const taughtDSLSchema = "https://gitlab.com/gitlab-org/orbit/knowledge-graph/schemas/graph_query/v12"

// Fixture project names created under <namespace>/ by
// scripts/setup-orbit-fixtures.sh. kg-fixtures populates the Project,
// MergeRequest, File, Milestone and Pipeline entities; security-fixtures
// populates the Vulnerability and Finding entities. See
// docs/development/orbit-fixtures.md for the full layout.
const (
	kgFixturesProjectPath       = "kg-fixtures"
	securityFixturesProjectPath = "security-fixtures"
	// fixtureBranch is the source branch of the squash-merged merge
	// request the setup script creates in kg-fixtures.
	fixtureBranch = "feature/restock-helper"
)

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

// summarize runs a single live check under a 60s timeout, logs a
// short PASS/FAIL summary, and records the result as a subtest of t.
// The fn is expected to return a JSON-marshalable value or an error.
//
// The 60s budget is wide enough to absorb a slow /api/v4/orbit/query
// response (the path_finding shape can scan a non-trivial slice of
// the graph even with a small `max_depth`); the Orbit knowledge
// graph service is the slowest of the six endpoints. Tighter
// budgets (e.g. 30s) produced flaky CI on a re-run right after
// setup, when the indexer had only just finished catching up.
func summarize(t *testing.T, name string, fn func(context.Context) (any, error)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

// refusalCodes are the codes Workhorse answers a query it would not run with
// for a fault in the query itself, which orbit.query passes on as GitLab's
// own words (internal/toolutil, the Workhorse query refusal).
var refusalCodes = []string{"{code: compile_error}", "{code: validation_error}"}

// errQueryRefused is what [liveQuery] wraps a refused query in.
var errQueryRefused = errors.New("GitLab refused to compile a query this suite asks, so it is written in a shape the DSL GitLab.com serves does not accept: read orbit.dsl and correct it")

// liveQuery runs one query through the handler. Every query here is in the
// DSL GitLab.com serves, so a refusal GitLab writes for a fault in the query
// is not an answer to log: it means the language moved or a query here is
// wrong, and the subtest fails saying which (issue 1031, where the suite's
// queries had been refused this way while the handler's own checks passed
// them).
func liveQuery(ctx context.Context, client *gitlabclient.Client, input orbit.QueryInput) (orbit.QueryOutput, error) {
	out, err := orbit.Query(ctx, client, input)
	if err != nil {
		for _, code := range refusalCodes {
			if strings.Contains(err.Error(), code) {
				return out, fmt.Errorf("%w: %w", errQueryRefused, err)
			}
		}
	}
	return out, err
}

// rowSummary is the one-line summary a query subtest logs.
func rowSummary(out orbit.QueryOutput) string {
	return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount)
}

// node is a node selector of the version 12 DSL: an alias, an entity, and
// whichever of filters, node_ids and columns the caller sets.
func node(id, entity string, fields map[string]any) map[string]any {
	selector := map[string]any{"id": id, "entity": entity}
	maps.Copy(selector, fields)
	return selector
}

// projectPath is the full path of a fixture project in namespace.
func projectPath(namespace, project string) string {
	return namespace + "/" + project
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

// fixtureProjectIDs returns the ids of the fixture namespace's projects, as
// the Knowledge Graph reports them, by a traversal of the projects under the
// namespace's path. The node_ids subtests ask for these rather than for ids
// written down here, which re-provisioning the fixtures replaces.
func fixtureProjectIDs(ctx context.Context, client *gitlabclient.Client, namespace string) ([]any, error) {
	out, err := liveQuery(ctx, client, orbit.QueryInput{
		ResponseFormat: "raw",
		Query: map[string]any{
			"query_type": "traversal",
			"nodes": []any{node("p", "Project", map[string]any{
				"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
				"columns": []any{"id", "full_path"},
			})},
		},
	})
	if err != nil {
		return nil, err
	}
	result, _ := out.Result.(map[string]any)
	nodes, _ := result["nodes"].([]any)
	ids := make([]any, 0, len(nodes))
	for _, entry := range nodes {
		if row, ok := entry.(map[string]any); ok && row["id"] != nil {
			ids = append(ids, row["id"])
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no project of %s is indexed yet: run make orbit-wait-indexer and try again", namespace)
	}
	return ids, nil
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
	namespace := orbitFixturesNamespace()
	testOrbitLiveReadOnlyHandlers(t, client)
	testOrbitLiveDSLHandlers(t, client)
	testOrbitLiveQueryHandlers(t, client, namespace)
	testOrbitLiveGraphStatusHandlers(t, client, namespace)
}

// testOrbitLiveReadOnlyHandlers exercises [Status], [Schema], and
// [Tools] with the GitLab.com Orbit service. Each subtest logs the
// decoded fields as a one-line PASS summary.
func testOrbitLiveReadOnlyHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	summarize(t, "Status", func(ctx context.Context) (any, error) {
		out, err := orbit.Status(ctx, client, orbit.StatusInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("status=%s version=%s components=%d",
			out.Status, out.Version, len(out.Components)), nil
	})

	summarize(t, "Schema", func(ctx context.Context) (any, error) {
		out, err := orbit.Schema(ctx, client, orbit.SchemaInput{})
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
		out, err := orbit.Tools(ctx, client, orbit.ToolsInput{})
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
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.Content)), nil
	})

	// GitLab sends the llm grammar as a JSON string; the handler publishes
	// the text inside it, so a leading quote means the decode was lost.
	summarize(t, "DSL_llm", func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormat: "llm"})
		if err != nil {
			return nil, err
		}
		if out.Content == "" || strings.HasPrefix(out.Content, `"`) {
			return nil, fmt.Errorf("llm grammar = %.60q, want the grammar's text rather than the JSON string it arrives in", out.Content)
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.Content)), nil
	})

	summarize(t, "DSL_raw", func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormat: "raw"})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.Content)), nil
	})
}

// TestOrbitLiveGitLabCom_DSL holds the query DSL GitLab.com serves to the one
// orbit.query teaches, by its $id, and logs its version. The guidance names
// the shape of version 12 (nodes, operator-keyed filters, a neighbors query
// with no node of its own, rel_types on every path), and the Orbit response
// record names the DSL it was taken against, so a new major version is a
// change somebody has to read the guidance against: this is where it fails
// first, on the next live run, instead of a model being taught a language
// GitLab no longer compiles, which is what issue 1031 was.
func TestOrbitLiveGitLabCom_DSL(t *testing.T) {
	client := newLiveClient(t)
	summarize(t, "DSL_is_the_version_orbit_query_teaches", func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormat: "raw"})
		if err != nil {
			return nil, err
		}
		var stamp struct {
			ID       string   `json:"$id"`
			Version  string   `json:"version"`
			Required []string `json:"required"`
		}
		if err = json.Unmarshal([]byte(out.Content), &stamp); err != nil {
			return nil, fmt.Errorf("the raw DSL is not a JSON Schema document: %w", err)
		}
		if stamp.ID != taughtDSLSchema {
			return nil, fmt.Errorf("GitLab.com serves the query DSL %q at version %s and orbit.query teaches %q: read the new language and move the guidance, the examples and this constant to it", stamp.ID, stamp.Version, taughtDSLSchema)
		}
		return fmt.Sprintf("dsl=%s version=%s required=%v", stamp.ID, stamp.Version, stamp.Required), nil
	})
}

// testOrbitLiveQueryHandlers exercises [Query] against the live API
// with a representative query for each query_type variant: traversal
// (with both node_ids and filters), aggregation, llm response format,
// neighbors, and path_finding. Each subtest logs query_type and
// row_count to surface a successful decode.
func testOrbitLiveQueryHandlers(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Query_traversal_by_node_ids", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes":      []any{node("proj", "Project", map[string]any{"node_ids": ids[:1], "columns": []any{"id"}})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Query_traversal_with_filter", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
					"columns": []any{"id"},
				})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Query_aggregation", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
				})},
				"aggregations": []any{map[string]any{"count": "p", "as": "project_count"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Query_llm_format", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			ResponseFormat: "llm",
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("proj", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
					"columns": []any{"id"},
				})},
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
		// The center of a neighbors query is its one node; neighbors holds
		// only direction and rel_types. The direction defaults to outgoing,
		// so both is what reaches every relationship of the project.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
				})},
				"neighbors": map[string]any{"direction": "both"},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Query_path_finding", func(ctx context.Context) (any, error) {
		// A path query names its relationship types, and each is followed
		// only in its defined direction: a merge request is IN_PROJECT its
		// project, not the other way round.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "path_finding",
				"nodes": []any{
					node("mr", "MergeRequest", map[string]any{"filters": map[string]any{"source_branch": fixtureBranch}}),
					node("p", "Project", map[string]any{"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)}}),
				},
				"path": map[string]any{"type": "shortest", "from": "mr", "to": "p", "max_depth": 2, "rel_types": []any{"IN_PROJECT"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Query_version_11_shape_is_refused_in_GitLabs_words", func(ctx context.Context) (any, error) {
		// The shape this server taught until issue 1031: a top-level node.
		// It reaches GitLab as written, and what comes back is GitLab's own
		// account of the fault and the hint naming orbit.dsl, never a
		// refusal of this server's making.
		_, err := orbit.Query(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"node":       node("p", "Project", map[string]any{"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)}}),
			},
		})
		if err == nil {
			return nil, errors.New("GitLab.com ran a query with a top-level node, which version 12 of the DSL refuses: read orbit.dsl, the language may have moved")
		}
		for _, want := range []string{"{code: compile_error}", `"nodes" is a required property`, "orbit.dsl serves the query language"} {
			if !strings.Contains(err.Error(), want) {
				return nil, fmt.Errorf("refusal = %q, want it to carry %q", err.Error(), want)
			}
		}
		return "refused with GitLab's compile error", nil
	})
}

// testOrbitLiveGraphStatusHandlers exercises [GraphStatus] with two
// of the three supported scopes (full_path and namespace_id). The
// project_id scope is omitted because it requires a project the
// fixture setup script does not provision.
func testOrbitLiveGraphStatusHandlers(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "GraphStatus_full_path", func(ctx context.Context) (any, error) {
		out, err := orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{FullPath: namespace})
		if err != nil {
			return nil, err
		}
		projects := "<nil>"
		if out.Projects != nil {
			projects = fmt.Sprintf("indexed=%d total=%d gaps=%d", out.Projects.Indexed, out.Projects.TotalKnown, out.Projects.Gaps)
		}
		state := "<nil>"
		if out.Indexing != nil {
			state = out.Indexing.State
		}
		return fmt.Sprintf("projects=%s indexing.state=%s", projects, state), nil
	})

	summarize(t, "GraphStatus_namespace_id", func(ctx context.Context) (any, error) {
		group, _, err := client.GL().Namespaces.GetNamespace(namespace, gl.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("look up the fixture namespace %s: %w", namespace, err)
		}
		out, err := orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{NamespaceID: group.ID})
		if err != nil {
			return nil, err
		}
		projects := "<nil>"
		if out.Projects != nil {
			projects = fmt.Sprintf("indexed=%d total=%d gaps=%d", out.Projects.Indexed, out.Projects.TotalKnown, out.Projects.Gaps)
		}
		return "projects=" + projects, nil
	})
}

// TestOrbitLiveGitLabCom_ShapeDiscovery keeps regression coverage of the
// smallest query of each kind GitLab.com runs in version 12 of the DSL. If
// GitLab ever tightens validation, these subtests fail first, and the
// QueryInput documentation, the parameter guidance and the site's Orbit page
// should be moved to the new contract with them.
func TestOrbitLiveGitLabCom_ShapeDiscovery(t *testing.T) {
	client := newLiveClient(t)
	namespace := orbitFixturesNamespace()

	summarize(t, "Aggregation_with_filter", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
				})},
				"aggregations": []any{map[string]any{"count": "p", "as": "count_projects"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Aggregation_with_node_ids", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type":   "aggregation",
				"nodes":        []any{node("p", "Project", map[string]any{"node_ids": ids})},
				"aggregations": []any{map[string]any{"count": "p", "as": "count_projects"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Neighbors_center_is_the_one_node", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"nodes":      []any{node("p", "Project", map[string]any{"node_ids": ids[:1]})},
				"neighbors":  map[string]any{"direction": "both"},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "PathFinding_any_relationship", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "path_finding",
				"nodes": []any{
					node("mr", "MergeRequest", map[string]any{"filters": map[string]any{"source_branch": fixtureBranch}}),
					node("p", "Project", map[string]any{"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)}}),
				},
				"path": map[string]any{"type": "shortest", "from": "mr", "to": "p", "max_depth": 3, "rel_types": []any{"*"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	// Schema with the default response format (handler omits the
	// response_format parameter so the API applies its own default, raw).
	summarize(t, "Schema_default_format", func(ctx context.Context) (any, error) {
		out, err := orbit.Schema(ctx, client, orbit.SchemaInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("schema_version=%s domains=%d nodes=%d edges=%d",
			out.SchemaVersion, len(out.Domains), len(out.Nodes), len(out.Edges)), nil
	})

	// Schema with each format named explicitly, under each input name. The
	// format must reach GitLab as response_format: sent as format, which
	// Grape reserves, it was answered 406 whatever its value.
	summarize(t, "Schema_raw_via_format", func(ctx context.Context) (any, error) {
		return liveRawSchema(ctx, client, orbit.SchemaInput{Format: "raw"})
	})
	summarize(t, "Schema_raw_via_response_format", func(ctx context.Context) (any, error) {
		return liveRawSchema(ctx, client, orbit.SchemaInput{ResponseFormat: "raw"})
	})
	summarize(t, "Schema_llm_via_format", func(ctx context.Context) (any, error) {
		return liveLLMSchema(ctx, client, orbit.SchemaInput{Format: "llm"})
	})
	summarize(t, "Schema_llm_via_response_format", func(ctx context.Context) (any, error) {
		return liveLLMSchema(ctx, client, orbit.SchemaInput{ResponseFormat: "llm"})
	})
}

// liveRawSchema asks for the structured ontology and fails unless it came
// back structured.
func liveRawSchema(ctx context.Context, client *gitlabclient.Client, input orbit.SchemaInput) (any, error) {
	out, err := orbit.Schema(ctx, client, input)
	if err != nil {
		return nil, err
	}
	if out.SchemaVersion == "" || len(out.Domains) == 0 {
		return nil, fmt.Errorf("raw schema = %+v, want the structured ontology", out)
	}
	return fmt.Sprintf("schema_version=%s domains=%d", out.SchemaVersion, len(out.Domains)), nil
}

// liveLLMSchema asks for the compact text ontology and fails unless it came
// back as text.
func liveLLMSchema(ctx context.Context, client *gitlabclient.Client, input orbit.SchemaInput) (any, error) {
	out, err := orbit.Schema(ctx, client, input)
	if err != nil {
		return nil, err
	}
	if out.FormattedText == "" {
		return nil, fmt.Errorf("llm schema = %+v, want its compact text in formatted_text", out)
	}
	return fmt.Sprintf("formatted_text bytes=%d", len(out.FormattedText)), nil
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
// been indexed yet, the affected subtest logs row_count=0 without failing.
// Re-run the live test a few minutes after the setup script completes to
// allow the indexer to catch up.
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
	namespace := orbitFixturesNamespace()

	summarize(t, "Project_kg_fixtures_filter", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
					"columns": []any{"id", "full_path", "name"},
				})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Project_security_fixtures_filter", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"eq": projectPath(namespace, securityFixturesProjectPath)}},
					"columns": []any{"id", "full_path"},
				})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Milestone_count_active", func(ctx context.Context) (any, error) {
		// Count active milestones. The setup script provisions one active
		// milestone (the KG coverage one); other namespaces may hold more.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type":   "aggregation",
				"nodes":        []any{node("m", "Milestone", map[string]any{"filters": map[string]any{"state": "active"}})},
				"aggregations": []any{map[string]any{"count": "m", "as": "active_milestones"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "File_count_python", func(ctx context.Context) (any, error) {
		// Count Python source files. The File entity does not expose
		// a project_id filter, so we count by path suffix across the
		// whole namespace. Each fixture project contributes 8-9 .py files.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type":   "aggregation",
				"nodes":        []any{node("f", "File", map[string]any{"filters": map[string]any{"path": map[string]any{"ends_with": ".py"}}})},
				"aggregations": []any{map[string]any{"count": "f", "as": "python_files"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "MergeRequest_in_kg_fixtures", func(ctx context.Context) (any, error) {
		// Fetch the squash-merged MR the setup script creates. The MR
		// id is dynamic per instance, so we filter by its branches.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("mr", "MergeRequest", map[string]any{
					"filters": map[string]any{"source_branch": fixtureBranch, "target_branch": "main"},
					"columns": []any{"id", "iid", "title", "state", "source_branch"},
				})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Vulnerability_count_detected", func(ctx context.Context) (any, error) {
		// Count detected vulnerabilities. The setup script provisions
		// 5 such findings (1 critical AWS key, 3 high SQLi/eval,
		// 1 medium weak-hash) via the SAST and Secret Detection
		// templates. The exact number depends on how many analyzers
		// have finished.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type":   "aggregation",
				"nodes":        []any{node("v", "Vulnerability", map[string]any{"filters": map[string]any{"state": "detected"}})},
				"aggregations": []any{map[string]any{"count": "v", "as": "open_vulnerabilities"}},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Neighbors_kg_fixtures", func(ctx context.Context) (any, error) {
		// What the kg-fixtures project is connected to, in both
		// directions: its group, creator and labels are reached by
		// incoming relationships, which the default outgoing direction
		// leaves out.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
				})},
				"neighbors": map[string]any{"direction": "both"},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})
}

// TestOrbitLiveGitLabCom_FeatureCoverage exercises the full Orbit
// query DSL surface against the live fixture namespace: filter
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

// testOrbitLiveFeatureCoverageFiltersAndTraversal exercises the
// `in`, `contains`, `gt` and combined `gte`/`lt` filter operators and a
// multi-node traversal that joins MergeRequest to Project through the
// IN_PROJECT relationship.
func testOrbitLiveFeatureCoverageFiltersAndTraversal(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Filter_in_operator_severity", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("v", "Vulnerability", map[string]any{
					"filters": map[string]any{"severity": map[string]any{"in": []any{"critical", "high"}}},
					"columns": []any{"id", "severity"},
				})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Filter_contains_operator_path", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("f", "File", map[string]any{
					"filters": map[string]any{"path": map[string]any{"contains": "orders"}},
					"columns": []any{"id", "path"},
				})},
				"limit": 5,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Filter_gt_operator", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"star_count": map[string]any{"gt": 0}},
					"columns": []any{"id", "full_path", "star_count"},
				})},
				"limit": 10,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Filter_operators_combined_on_one_property", func(ctx context.Context) (any, error) {
		// Several operator keys on one property AND-combine.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{
						"full_path":  map[string]any{"starts_with": namespace + "/"},
						"star_count": map[string]any{"gte": 0, "lt": 1000},
					},
					"columns": []any{"id", "star_count"},
				})},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Traversal_project_to_merge_requests", func(ctx context.Context) (any, error) {
		// Classic "find MRs in a project" pattern. The relationship
		// IN_PROJECT connects MergeRequest to Project; the alias mr is
		// used in the columns block.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{
					node("p", "Project", map[string]any{
						"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
						"columns": []any{"id", "full_path"},
					}),
					node("mr", "MergeRequest", map[string]any{"columns": []any{"id", "iid", "title", "state"}}),
				},
				"relationships": []any{map[string]any{"type": "IN_PROJECT", "from": "mr", "to": "p"}},
				"limit":         5,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})
}

// testOrbitLiveFeatureCoverageAggregation exercises the aggregation
// functions (count, sum, max, avg) plus the group_by alternatives:
// group_by on a property (severity) and group_by on a node alias
// joined via the IN_PROJECT relationship.
func testOrbitLiveFeatureCoverageAggregation(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Aggregation_group_by_severity", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type":       "aggregation",
				"nodes":            []any{node("v", "Vulnerability", map[string]any{"filters": map[string]any{"state": "detected"}})},
				"group_by":         []any{"v.severity"},
				"aggregations":     []any{map[string]any{"count": "v", "as": "vuln_count"}},
				"aggregation_sort": "-vuln_count",
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Aggregation_group_by_node_with_relationship", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "aggregation",
				"nodes": []any{
					node("p", "Project", map[string]any{
						"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
					}),
					node("mr", "MergeRequest", map[string]any{"columns": []any{"id"}}),
				},
				"relationships":    []any{map[string]any{"type": "IN_PROJECT", "from": "mr", "to": "p"}},
				"group_by":         []any{"p"},
				"aggregations":     []any{map[string]any{"count": "mr", "as": "mr_count"}},
				"aggregation_sort": "-mr_count",
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	starsOf := func(function, alias string) func(context.Context) (any, error) {
		return func(ctx context.Context) (any, error) {
			out, err := liveQuery(ctx, client, orbit.QueryInput{
				Query: map[string]any{
					"query_type": "aggregation",
					"nodes": []any{node("p", "Project", map[string]any{
						"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
					})},
					"aggregations": []any{map[string]any{function: "p.star_count", "as": alias}},
				},
			})
			if err != nil {
				return nil, err
			}
			return rowSummary(out), nil
		}
	}
	summarize(t, "Aggregation_sum_star_count", starsOf("sum", "total_stars"))
	summarize(t, "Aggregation_max_star_count", starsOf("max", "max_stars"))
	summarize(t, "Aggregation_avg_star_count", starsOf("avg", "avg_stars"))
}

// testOrbitLiveFeatureCoverageVirtualAndMeta exercises order_by,
// virtual columns (diff, content), keyset pagination, the id_range
// scope, a neighbors query narrowed to one relationship type, and the
// options.dynamic_columns knob for neighbors hydration.
func testOrbitLiveFeatureCoverageVirtualAndMeta(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Traversal_order_by_name_desc", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
					"columns": []any{"id", "full_path", "name"},
				})},
				"order_by": "-p.name",
				"limit":    5,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Traversal_merge_request_with_diff_column", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("mr", "MergeRequest", map[string]any{
					"filters": map[string]any{"source_branch": fixtureBranch},
					"columns": []any{"id", "iid", "title", "diff"},
				})},
				"limit": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Traversal_file_with_content_column", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("f", "File", map[string]any{
					"filters": map[string]any{"path": map[string]any{"ends_with": "models.py"}},
					"columns": []any{"id", "path", "language", "content"},
				})},
				"limit": 1,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Traversal_keyset_pagination", func(ctx context.Context) (any, error) {
		// The first page omits after; the next one would pass the
		// pagination.next_cursor this answer carries.
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": map[string]any{"starts_with": namespace + "/"}},
					"columns": []any{"id", "full_path"},
				})},
				"limit":  2,
				"cursor": map[string]any{"page_size": 1},
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Traversal_id_range_scope", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "traversal",
				"nodes": []any{node("p", "Project", map[string]any{
					"id_range": map[string]any{"start": ids[0], "end": ids[0]},
					"columns":  []any{"id", "full_path"},
				})},
				"limit": 5,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Neighbors_incoming_of_one_relationship_type", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
				})},
				"neighbors": map[string]any{"direction": "incoming", "rel_types": []any{"IN_PROJECT"}},
				"limit":     5,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})

	summarize(t, "Neighbors_with_dynamic_columns_option", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, orbit.QueryInput{
			Query: map[string]any{
				"query_type": "neighbors",
				"nodes": []any{node("p", "Project", map[string]any{
					"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
				})},
				"neighbors": map[string]any{"direction": "both"},
				"options":   map[string]any{"dynamic_columns": "default"},
				"limit":     5,
			},
		})
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	})
}
