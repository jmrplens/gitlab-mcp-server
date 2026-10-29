// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build orbitlive

// Package orbit contains live integration tests against the GitLab Orbit
// Knowledge Graph on GitLab.com. The tests reach the real sources each handler
// in internal/tools/orbit reads (no httptest mocks): GitLab's GraphQL API for
// the status, the schema and a namespace's indexing status, the Orbit MCP
// endpoint at https://gitlab.com/api/v4/orbit/mcp for the tool list, the query
// language and the schema's compact text, and POST /api/v4/orbit/query, the one
// Orbit REST route of the six these handlers used that GitLab.com still serves
// after removing the others on 2026-10-27 and -28.
//
// # Access
//
// Since Orbit's general availability on 2026-10-29, GitLab.com answers a
// namespace's indexing status and a query only for a top-level group with the
// orbit_promo add-on or a running Orbit trial; the plan no longer counts. The
// fixture namespace lost it at 09:22 UTC that day. The service status, the
// schema, the tool list and the query language answered the account without
// it until about 09:32 UTC, and by 09:51 UTC GitLab.com answered the service
// status null and the Orbit MCP endpoint 403 as well. So every subtest skips
// with GitLab's answer when GitLab.com does not serve Orbit to the token, and
// fails on any other error, which says something about the handler: one that
// needs the fixture namespace runs under [requireOrbitServed], and one that
// needs only the account (the status, the schema, the tool list and the query
// language) under [skipUnlessServed], which skips on a null the handler
// reports only when GitLab, asked for the same field again outside the
// handler, answers it null too. The top-level group's Orbit access and
// trial answer either way, since they are what says which, so
// [testOrbitLiveAccess] always runs.
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
//   - GITLAB_COM_READ_API_TOKEN: optional. A GitLab.com personal access
//     token carrying read_api and not api; with it, one more test measures
//     that the query route accepts that scope.
//   - ORBIT_FIXTURES_NAMESPACE: optional. Overrides the default
//     `jmrp-oss/orbit-fixtures` namespace against which the
//     fixture-driven subtests run. Set this when developing against your
//     own GitLab.com namespace.
//
// # The query language
//
// Every query here is written in GQL, the read-only openCypher 9 based
// language GitLab.com compiles every query in since it turned
// orbit_gql_queries on for everyone at 01:06 UTC on 2026-10-15, and which
// orbit.dsl describes from the query_graph command GitLab.com publishes: one
// MATCH ... RETURN statement, every node labeled and at least one bounded by
// its id or a literal property, a relationship type after a colon, WHERE with
// AND and NOT, aggregates aliased to be sorted by, SKIP and LIMIT for a page,
// and ANY SHORTEST over named relationship types for a path. Each was answered
// 200 by GitLab.com against jmrp-oss/orbit-fixtures on 2026-10-26, or on
// 2026-10-29 for the two GitLab.com changed in between: PAGE gave way to SKIP
// and LIMIT (issue 1527), and a shortest path must name its relationship types
// (issue 1528). A query GitLab cannot compile fails its subtest by name
// ([liveQuery]), and TestOrbitLiveGitLabCom_DSL fails when GitLab.com stops
// describing the query command as GQL (issue 1414). A query written in the
// JSON DSL this suite used until then, which GitLab.com removed on
// 2026-10-27, is refused, and a subtest holds what a caller is told about it.
//
// No project, user or namespace id is written down: each is found by its
// path in the fixture namespace, since re-provisioning the fixtures gives
// them new ids. A path is written into a query between single quotes as it
// is: a GitLab path holds letters, digits, dots, dashes, underscores and
// slashes, none of which a GQL string literal has to escape.
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
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2erelease"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
	orbit "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/orbit" // the package under test, imported from outside it (see Layout above)
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// liveGitLabComURL is the base URL for the real GitLab.com REST API
// exercised by every handler test in this file. Pinned to gitlab.com
// (no override) because the Orbit Knowledge Graph API is currently a
// GitLab.com-only experimental surface.
const liveGitLabComURL = "https://gitlab.com"

// queryCommand is the Orbit command a graph query runs as, whose description
// orbit.dsl publishes as the query language. TestOrbitLiveGitLabCom_DSL holds
// that description to GQL, so GitLab.com describing another language fails
// the suite rather than the guidance going stale in silence.
const queryCommand = "query_graph"

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
// The fn is expected to return a JSON-marshalable value or an error. An
// error wrapping [errNotServed], which only [skipUnlessServed] returns,
// skips the subtest with GitLab's answer instead of failing it.
//
// The 60s budget is wide enough to absorb a slow /api/v4/orbit/query
// response (a shortest path can scan a non-trivial slice of the graph
// even bounded at three hops); the Orbit knowledge graph service is the
// slowest of the six endpoints. Tighter budgets (e.g. 30s) produced flaky
// CI on a re-run right after setup, when the indexer had only just
// finished catching up.
func summarize(t *testing.T, name string, fn func(context.Context) (any, error)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		out, err := fn(ctx)
		if errors.Is(err, errNotServed) {
			t.Skipf("SKIP %s: %v", name, err)
		}
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
var errQueryRefused = errors.New("GitLab refused to compile a query this suite asks, so it is written in a shape the GQL GitLab.com compiles does not accept: read orbit.dsl and correct it")

// liveQuery runs one GQL statement through the handler. Every query here is
// one GitLab.com ran, so a refusal GitLab writes for a fault in the query is
// not an answer to log: it means the language moved or a query here is
// wrong, and the subtest fails saying which (issue 1031, where the suite's
// queries had been refused this way while the handler's own checks passed
// them, and issue 1414, where GitLab.com stopped reading the language they
// were written in).
func liveQuery(ctx context.Context, client *gitlabclient.Client, query string) (orbit.QueryOutput, error) {
	return liveQueryAs(ctx, client, orbit.QueryInput{Query: query})
}

// liveQueryAs is [liveQuery] with the whole input, for a query that asks for
// a response format of its own.
func liveQueryAs(ctx context.Context, client *gitlabclient.Client, input orbit.QueryInput) (orbit.QueryOutput, error) {
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

// queryRun returns the subtest body that runs one GQL statement and logs
// what came back.
func queryRun(client *gitlabclient.Client, query string) func(context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, query)
		if err != nil {
			return nil, err
		}
		return rowSummary(out), nil
	}
}

// rowSummary is the one-line summary a query subtest logs.
func rowSummary(out orbit.QueryOutput) string {
	return fmt.Sprintf("query_type=%s row_count=%d", out.QueryType, out.RowCount)
}

// projectPath is the full path of a fixture project in namespace.
func projectPath(namespace, project string) string {
	return namespace + "/" + project
}

// orbitFixturesNamespace returns the namespace under which
// scripts/setup-orbit-fixtures.sh provisioned the fixtures. Defaults
// to "jmrp-oss/orbit-fixtures", the subgroup that holds only the two
// fixture projects, so a query bounded by the namespace's path counts no
// other project of jmrp-oss. It can be overridden by the developer running
// the test against their own GitLab.com namespace via the
// ORBIT_FIXTURES_NAMESPACE environment variable.
func orbitFixturesNamespace() string {
	ns := os.Getenv("ORBIT_FIXTURES_NAMESPACE")
	if ns == "" {
		ns = "jmrp-oss/orbit-fixtures"
	}
	return ns
}

// projectByPath is the statement that matches one fixture project by its
// full path and returns the properties named.
func projectByPath(namespace, project, returns string) string {
	return "MATCH (p:Project {full_path: '" + projectPath(namespace, project) + "'}) RETURN " + returns
}

// underNamespace is the WHERE predicate that keeps the projects of the
// fixture namespace.
func underNamespace(namespace string) string {
	return "p.full_path STARTS WITH '" + namespace + "/'"
}

// fixtureProjectIDs returns the ids of the fixture namespace's projects, as
// the Knowledge Graph reports them, by a traversal of the projects under the
// namespace's path. The subtests that bound a node by its id ask for these
// rather than for ids written down here, which re-provisioning the fixtures
// replaces. Each is checked to be a decimal integer before a query carries
// it, since it is written into the statement as a number.
func fixtureProjectIDs(ctx context.Context, client *gitlabclient.Client, namespace string) ([]string, error) {
	out, err := liveQueryAs(ctx, client, orbit.QueryInput{
		ResponseFormat: "raw",
		Query:          "MATCH (p:Project) WHERE " + underNamespace(namespace) + " RETURN p.id, p.full_path",
	})
	if err != nil {
		return nil, err
	}
	result, _ := out.Result.(map[string]any)
	nodes, _ := result["nodes"].([]any)
	ids := make([]string, 0, len(nodes))
	for _, entry := range nodes {
		row, ok := entry.(map[string]any)
		if !ok || row["id"] == nil {
			continue
		}
		id := fmt.Sprint(row["id"])
		if _, parseErr := strconv.ParseInt(id, 10, 64); parseErr != nil {
			return nil, fmt.Errorf("the Knowledge Graph reported the project id %q, which is not a decimal integer a query can carry: %w", id, parseErr)
		}
		ids = append(ids, id)
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
	testOrbitLiveAccess(t, client, namespace)
	t.Run("FixtureNamespace", func(t *testing.T) {
		requireOrbitServed(t, client, namespace)
		testOrbitLiveQueryHandlers(t, client, namespace)
		testOrbitLiveGraphStatusHandlers(t, client, namespace)
	})
}

// requireOrbitServed skips the test unless GitLab.com serves Orbit for the
// fixture namespace to this token, which since Orbit's general availability
// takes the orbit_promo add-on or a running trial on its top-level group. It
// asks the indexing status, the cheapest answer that needs the namespace, and
// skips on the answers GitLab gives an account Orbit is not served to: null
// where the status is read ([orbit.ErrOrbitUnavailable]), 403, and, for a
// fixture namespace that is itself a top-level group, its access with no
// index beside it. Any other failure fails the test, since it says something
// about the handler.
func requireOrbitServed(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{FullPath: namespace})
	switch {
	case err == nil && !out.HasIndex():
		t.Skipf("GitLab.com answers no Orbit index for %s to this token (Orbit access %s); since 2026-10-29 that takes the orbit_promo add-on or an Orbit trial on its top-level group", namespace, accessState(out.Access))
	case err == nil:
		return
	case errors.Is(err, orbit.ErrOrbitUnavailable), toolutil.IsHTTPStatus(err, http.StatusForbidden):
		t.Skipf("GitLab.com does not serve Orbit for %s to this token (%v); since 2026-10-29 that takes the orbit_promo add-on or an Orbit trial on its top-level group", namespace, err)
	default:
		t.Fatalf("orbit.GraphStatus(%s): %v", namespace, err)
	}
}

// errNotServed is what [skipUnlessServed] wraps an answer in that says
// GitLab.com does not serve Orbit to the token, which [summarize] skips on.
var errNotServed = errors.New("GitLab.com does not serve Orbit to this token; since 2026-10-29 that takes the orbit_promo add-on or an Orbit trial on a top-level group the token can read")

// skipUnlessServed wraps the body of a subtest that needs only the account,
// not the fixture namespace, so that it skips with GitLab's answer when
// GitLab.com does not serve Orbit to the token ([notServed]) and fails on any
// other error, as [requireOrbitServed] does for the subtests that need the
// fixture namespace. field is the field of GitLab's GraphQL API the subtest's
// handler reads, or [readsMCPOnly] for one that reads only the Orbit MCP
// endpoint: a null the handler reports is skipped on only when GitLab, asked
// for field again outside the handler's types, answers it null too
// ([answeredNull]), so a handler that reads null where GitLab sent a value
// fails its subtest. It is per subtest rather than one probe before them,
// since the subtests read two sources, GitLab's GraphQL API and the Orbit MCP
// endpoint, and nothing measured that the two decline an account together: by
// 09:51 UTC on 2026-10-29 both had, and what GraphQL answered for the schema
// then was not asked.
func skipUnlessServed(client *gitlabclient.Client, field *orbitField, fn func(context.Context) (any, error)) func(context.Context) (any, error) {
	return func(ctx context.Context) (any, error) {
		out, err := fn(ctx)
		if !notServed(err) {
			return out, err
		}
		if errors.Is(err, orbit.ErrOrbitUnavailable) {
			if askedErr := answeredNull(ctx, client, field); askedErr != nil {
				return nil, fmt.Errorf("%w; the handler's error: %w", askedErr, err)
			}
		}
		return nil, fmt.Errorf("%w: %w", errNotServed, err)
	}
}

// notServed reports whether err is GitLab.com declining to serve Orbit to the
// token rather than a fault of the handler: null where the handler reads
// ([orbit.ErrOrbitUnavailable]), which is how GitLab's GraphQL API answered
// the service status from 09:51 UTC on 2026-10-29 (what it answers for the
// schema then has not been measured, and [skipUnlessServed] asks GitLab again
// before it skips on a null), or the Orbit MCP endpoint's permission refusal,
// a 403 that carries no RFC 6750 code and does not refuse the account itself
// ([toolutil.IsPermissionRefusal]), which is how the endpoint that answers
// the tool list, the query language and the schema's text answered from then
// on. A 401, a scope or fine-grained grant refusal and a refusal of the
// account say the token is wrong rather than that Orbit is withheld, so they
// are not read as one, and neither is a 404, which a handler sending to a
// path GitLab does not serve gets, nor any other failure.
// TestNotServed_ThroughTheAccountLevelHandlers_ReadsGitLabsAnswer holds it
// able to fail.
func notServed(err error) bool {
	return errors.Is(err, orbit.ErrOrbitUnavailable) ||
		(toolutil.IsHTTPStatus(err, http.StatusForbidden) && toolutil.IsPermissionRefusal(err))
}

// orbitField is a field of Query.orbit an account-level handler reads, with
// the least document that asks GitLab for it again.
type orbitField struct {
	name     string
	document string
}

// The fields of Query.orbit the account-level subtests read, and
// readsMCPOnly, the field of a subtest whose handler reads only the Orbit MCP
// endpoint and so has no null to report.
var (
	readsServiceStatus = &orbitField{name: "serviceStatus", document: "query OrbitLiveServiceStatus { orbit { serviceStatus { version } } }"}
	readsSchema        = &orbitField{name: "schema", document: "query OrbitLiveSchema { orbit { schema { nodes(first: 1) { nodes { name } } } } }"}
	readsMCPOnly       *orbitField
)

// answeredNull asks GitLab's GraphQL API for field again, with a document of
// its own decoded into no handler's types, and returns nil only when GitLab
// answered field null, or Query.orbit itself null. Any other answer, a value
// there or a failure to ask, is the error [skipUnlessServed] fails the subtest
// with, since the null the handler reported is then its own reading rather
// than GitLab's.
func answeredNull(ctx context.Context, client *gitlabclient.Client, field *orbitField) error {
	if field == nil {
		return errors.New("the handler reported null, and it reads no field of GitLab's GraphQL API")
	}
	var answer struct {
		Data   map[string]any          `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: field.document}, &answer, gl.WithContext(ctx)); err != nil {
		return fmt.Errorf("asking GitLab again for Query.orbit.%s: %w", field.name, err)
	}
	if err := toolutil.GraphQLTopLevelError("asking GitLab again for Query.orbit."+field.name, answer.Errors); err != nil {
		return err
	}
	orbitAnswer, sent := answer.Data["orbit"]
	if sent && orbitAnswer == nil {
		return nil
	}
	if fields, isObject := orbitAnswer.(map[string]any); isObject {
		if value, fieldSent := fields[field.name]; fieldSent && value == nil {
			return nil
		}
	}
	return fmt.Errorf("the handler reported null, and GitLab, asked again for Query.orbit.%s, did not answer it null: the null is the handler's reading", field.name)
}

// accessState names an Orbit access state for a skip or a summary, and what its
// absence means: GitLab reports access for a top-level group alone, to a token
// with the Owner role there.
func accessState(access *orbit.GraphStatusAccess) string {
	if access == nil {
		return "not reported, which GitLab does for a subgroup, a project, and a token below the Owner role"
	}
	if access.Reason == "" {
		return access.State
	}
	return access.State + " (" + access.Reason + ")"
}

// testOrbitLiveAccess exercises [GraphStatus] on the fixture namespace's
// top-level group, the one namespace GitLab answers Orbit access and trial for.
// It runs whether or not GitLab.com serves Orbit there, since the access is
// the answer that says which: a group without access is answered with its
// access and no index rather than refused. It needs the fixture account to
// own the group, since GitLab answers the access to the Owner role alone. It
// does not hold the index to the access state: GitLab decides the index from
// the add-on and the trial it recorded, and the state partly from
// CustomersDot, so an index can come under TRIAL_STATUS_UNAVAILABLE or a
// trial that has just ended, and the summary reports both.
func testOrbitLiveAccess(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	topLevel, _, _ := strings.Cut(namespace, "/")
	summarize(t, "GraphStatus_top_level_access", func(ctx context.Context) (any, error) {
		out, err := orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{FullPath: topLevel})
		if err != nil {
			return nil, err
		}
		if out.Access == nil {
			return nil, fmt.Errorf("GitLab.com answered no Orbit access for the top-level group %s, which it reports to a token that can read the group's Orbit setting, the Owner role", topLevel)
		}
		indexed := out.HasIndex()
		trial := "<nil>"
		if out.Trial != nil {
			trial = out.Trial.State
		}
		return fmt.Sprintf("group=%s access=%s turn_on_denial_reason=%s trial=%s indexed=%t",
			topLevel, accessState(out.Access), out.Access.TurnOnDenialReason, trial, indexed), nil
	})
}

// testOrbitLiveReadOnlyHandlers exercises [Status], [Schema], and
// [Tools] with the GitLab.com Orbit service. Each subtest logs the
// decoded fields as a one-line PASS summary, and skips when GitLab.com
// does not serve Orbit to the token ([skipUnlessServed]).
func testOrbitLiveReadOnlyHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	summarize(t, "Status", skipUnlessServed(client, readsServiceStatus, func(ctx context.Context) (any, error) {
		out, err := orbit.Status(ctx, client, orbit.StatusInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("status=%s version=%s components=%d",
			out.Status, out.Version, len(out.Components)), nil
	}))

	summarize(t, "Schema", skipUnlessServed(client, readsSchema, func(ctx context.Context) (any, error) {
		out, err := orbit.Schema(ctx, client, orbit.SchemaInput{})
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("domains=%d nodes=%d edges=%d", len(out.Domains), len(out.Nodes), len(out.Edges)), nil
	}))

	summarize(t, "Tools", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		out, err := orbit.Tools(ctx, client, orbit.ToolsInput{})
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(out.Tools))
		for _, tool := range out.Tools {
			names = append(names, tool.Name)
		}
		return fmt.Sprintf("tools=%d names=[%s]", len(out.Tools), strings.Join(names, ",")), nil
	}))
}

// testOrbitLiveDSLHandlers exercises [DSL] in default, llm, and raw
// response formats. The subtest names correspond to the response_format
// the handler is invoked with. The raw answer, asked for or left to
// GitLab's default, is the query command with its grammar and input schema,
// and the llm answer the same as compact text. Each subtest skips when
// GitLab.com does not serve Orbit to the token ([skipUnlessServed]).
func testOrbitLiveDSLHandlers(t *testing.T, client *gitlabclient.Client) {
	t.Helper()
	described := func(out orbit.DSLOutput) (any, error) {
		if out.Name != queryCommand || out.Description == "" || out.Parameters == nil {
			return nil, fmt.Errorf("query language = %+v, want the %s command with its grammar and input schema", out, queryCommand)
		}
		return fmt.Sprintf("format=%s command=%s grammar_bytes=%d", out.ResponseFormat, out.Name, len(out.Description)), nil
	}
	summarize(t, "DSL_default", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{})
		if err != nil {
			return nil, err
		}
		return described(out)
	}))

	summarize(t, "DSL_llm", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormat: "llm"})
		if err != nil {
			return nil, err
		}
		if !strings.Contains(out.FormattedText, queryCommand) {
			return nil, fmt.Errorf("llm query language = %.80q, want the compact text of the %s command", out.FormattedText, queryCommand)
		}
		return fmt.Sprintf("format=%s bytes=%d", out.ResponseFormat, len(out.FormattedText)), nil
	}))

	summarize(t, "DSL_raw", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormat: "raw"})
		if err != nil {
			return nil, err
		}
		return described(out)
	}))
}

// TestOrbitLiveGitLabCom_DSL holds the query language GitLab.com describes to
// the one orbit.query teaches: GQL, a MATCH ... RETURN statement sent as the
// query command's one string input. The guidance names GQL's rules, and the
// Orbit response record stamps the grammar it was taken against, so GitLab.com
// describing another language is a change somebody has to read the guidance
// against: this is where it fails first, on the next live run, instead of a
// model being taught a language GitLab no longer compiles, which is what
// issues 1031 and 1414 were. It skips when GitLab.com does not serve Orbit to
// the token ([skipUnlessServed]), since the command catalog is then refused.
func TestOrbitLiveGitLabCom_DSL(t *testing.T) {
	client := newLiveClient(t)
	summarize(t, "DSL_is_the_GQL_orbit_query_teaches", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		out, err := orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormat: "raw"})
		if err != nil {
			return nil, err
		}
		if out.Name != queryCommand {
			return nil, fmt.Errorf("GitLab.com describes no %s command (got %q): orbit.dsl describes nothing, read what list_commands of the Orbit MCP endpoint, POST /api/v4/orbit/mcp, answers", queryCommand, out.Name)
		}
		if !strings.Contains(out.Description, "MATCH ... RETURN") {
			return nil, fmt.Errorf("GitLab.com describes the query command as %.120q, which is not the MATCH ... RETURN statement orbit.query teaches: read the new language and move the guidance, the examples and this test to it", out.Description)
		}
		var input struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
		}
		raw, err := json.Marshal(out.Parameters)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &input); err != nil {
			return nil, fmt.Errorf("the query command's input schema is not an object schema: %w", err)
		}
		if input.Properties["query"].Type != "string" {
			return nil, fmt.Errorf("the query command takes its query as %q, and orbit.query sends GQL text: read the input schema and move the handler to it", input.Properties["query"].Type)
		}
		return fmt.Sprintf("command=%s required=%v grammar_bytes=%d", out.Name, input.Required, len(out.Description)), nil
	}))
}

// testOrbitLiveQueryHandlers exercises [Query] against the live API with a
// representative statement for each query type GitLab infers from GQL:
// traversal (bounded by an id and by a literal property), aggregation, the
// llm response format, neighbors and a shortest path. Each subtest logs
// query_type and row_count to surface a successful decode. The last holds
// what a caller who sends the retired JSON DSL is told.
func testOrbitLiveQueryHandlers(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Query_traversal_by_id", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		return queryRun(client, "MATCH (p:Project {id: "+ids[0]+"}) RETURN p.id")(ctx)
	})

	summarize(t, "Query_traversal_with_filter", queryRun(client,
		"MATCH (p:Project) WHERE "+underNamespace(namespace)+" RETURN p.id"))

	summarize(t, "Query_aggregation", queryRun(client,
		"MATCH (p:Project) WHERE "+underNamespace(namespace)+" RETURN count(p) AS project_count"))

	summarize(t, "Query_llm_format", func(ctx context.Context) (any, error) {
		out, err := liveQueryAs(ctx, client, orbit.QueryInput{
			ResponseFormat: "llm",
			Query:          "MATCH (p:Project) WHERE " + underNamespace(namespace) + " RETURN p.id",
		})
		if err != nil {
			return nil, err
		}
		preview := out.FormattedText
		if len(preview) > 200 {
			preview = preview[:200] + "...(truncated)"
		}
		return fmt.Sprintf("formatted_text=%q", preview), nil
	})

	// The gql format answers with the result as a graph pattern table, text/plain
	// like llm, so it arrives as formatted_text too.
	summarize(t, "Query_gql_format", func(ctx context.Context) (any, error) {
		out, err := liveQueryAs(ctx, client, orbit.QueryInput{
			ResponseFormat: "gql",
			Query:          "MATCH (p:Project) WHERE " + underNamespace(namespace) + " RETURN p.id",
		})
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(out.FormattedText) == "" {
			return nil, errors.New("the gql format answered no text")
		}
		preview := out.FormattedText
		if len(preview) > 200 {
			preview = preview[:200] + "...(truncated)"
		}
		return fmt.Sprintf("formatted_text=%q", preview), nil
	})

	// The neighbors of one node: the far end of the undirected pattern has a
	// variable and nothing else, so every relationship of the project, in
	// either direction, is reached.
	summarize(t, "Query_neighbors", queryRun(client,
		"MATCH (p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'})--(n) RETURN n"))

	// A shortest path between two bounded ends, along the one relationship
	// type that joins them, followed in its defined direction: a merge
	// request is IN_PROJECT its project, not the other way round.
	summarize(t, "Query_path_finding", queryRun(client,
		"MATCH path = ANY SHORTEST (mr:MergeRequest {source_branch: '"+fixtureBranch+"'})-[:IN_PROJECT*1..2]->"+
			"(p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'}) RETURN path"))

	summarize(t, "Query_as_JSON_DSL_text_is_refused_with_the_GQL_hint", func(ctx context.Context) (any, error) {
		// The JSON DSL this suite and orbit.query taught until issue 1414,
		// which gitlab-org/gitlab!261947 removed on 2026-10-27, written as the
		// text a client that serializes a nested object sends. orbit.query
		// takes text alone since then, so it reaches GitLab as written, and
		// GitLab.com parses it as GQL and answers compile_error; the caller is
		// told GitLab.com reads a query as GQL text, with an example. The
		// account is held by its code and by this server's hint, never by
		// GitLab's prose, which is the GQL parser's wording rather than
		// Workhorse's contract; the summary carries the prose to the run's log
		// instead.
		dsl, err := json.Marshal(map[string]any{
			"query_type": "traversal",
			"nodes": []any{map[string]any{
				"id": "p", "entity": "Project",
				"filters": map[string]any{"full_path": projectPath(namespace, kgFixturesProjectPath)},
			}},
		})
		if err != nil {
			return nil, err
		}
		_, err = orbit.Query(ctx, client, orbit.QueryInput{Query: string(dsl)})
		if err == nil {
			return nil, errors.New("GitLab.com ran a query written in the JSON DSL, which it removed on 2026-10-27: read orbit.dsl")
		}
		for _, want := range []string{"{code: compile_error}", "GitLab.com reads a query as GQL text, not as a JSON object"} {
			if !strings.Contains(err.Error(), want) {
				return nil, fmt.Errorf("refusal = %q, want it to carry %q", err.Error(), want)
			}
		}
		return "refused with GitLab's compile error and the GQL hint: " + err.Error(), nil
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
		return fmt.Sprintf("projects=%s indexing.state=%s access=%s", projects, state, accessState(out.Access)), nil
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
// smallest statement of each kind GitLab.com runs in GQL. If GitLab ever
// tightens the language, these subtests fail first, and the QueryInput
// documentation, the parameter guidance and the site's Orbit page should be
// moved to the new contract with them.
func TestOrbitLiveGitLabCom_ShapeDiscovery(t *testing.T) {
	client := newLiveClient(t)
	namespace := orbitFixturesNamespace()
	requireOrbitServed(t, client, namespace)

	summarize(t, "Aggregation_with_filter", queryRun(client,
		"MATCH (p:Project) WHERE "+underNamespace(namespace)+" RETURN count(p) AS count_projects"))

	summarize(t, "Aggregation_with_ids", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		return queryRun(client, "MATCH (p:Project) WHERE p.id IN ["+strings.Join(ids, ", ")+"] RETURN count(p) AS count_projects")(ctx)
	})

	summarize(t, "Neighbors_of_one_node_by_id", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		return queryRun(client, "MATCH (p:Project {id: "+ids[0]+"})--(n) RETURN n")(ctx)
	})

	// A shortest path names its relationship types since GitLab.com began
	// refusing a wildcard (issue 1528): "shortest paths require named
	// relationship types", measured 2026-10-29.
	summarize(t, "PathFinding_named_relationship", queryRun(client,
		"MATCH path = ANY SHORTEST (b:Branch {name: 'main'})-[:IN_PROJECT*1..2]->"+
			"(p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'}) RETURN path"))
}

// TestOrbitLiveGitLabCom_SchemaFormats holds the schema to both of its
// sources: GitLab's GraphQL API for the structured answer, under either input
// name of the format and with none, and the get_graph_schema command of the
// Orbit MCP endpoint for the compact text. Neither needs the fixture
// namespace, so it does not wait on [requireOrbitServed]; each subtest skips
// instead when GitLab.com does not serve Orbit to the token at all
// ([skipUnlessServed]).
func TestOrbitLiveGitLabCom_SchemaFormats(t *testing.T) {
	client := newLiveClient(t)

	summarize(t, "Schema_default_format", skipUnlessServed(client, readsSchema, func(ctx context.Context) (any, error) {
		return liveRawSchema(ctx, client, orbit.SchemaInput{})
	}))
	summarize(t, "Schema_expand", skipUnlessServed(client, readsSchema, func(ctx context.Context) (any, error) {
		out, err := orbit.Schema(ctx, client, orbit.SchemaInput{Expand: []string{"Project"}})
		if err != nil {
			return nil, err
		}
		for _, node := range out.Nodes {
			if node.Name == "Project" && len(node.Properties) > 0 {
				return fmt.Sprintf("Project properties=%d", len(node.Properties)), nil
			}
		}
		return nil, fmt.Errorf("schema expanded on Project carried no Project properties: %+v", out.Nodes)
	}))
	summarize(t, "Schema_raw_via_format", skipUnlessServed(client, readsSchema, func(ctx context.Context) (any, error) {
		return liveRawSchema(ctx, client, orbit.SchemaInput{Format: "raw"})
	}))
	summarize(t, "Schema_raw_via_response_format", skipUnlessServed(client, readsSchema, func(ctx context.Context) (any, error) {
		return liveRawSchema(ctx, client, orbit.SchemaInput{ResponseFormat: "raw"})
	}))
	summarize(t, "Schema_llm_via_format", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		return liveLLMSchema(ctx, client, orbit.SchemaInput{Format: "llm"})
	}))
	summarize(t, "Schema_llm_via_response_format", skipUnlessServed(client, readsMCPOnly, func(ctx context.Context) (any, error) {
		return liveLLMSchema(ctx, client, orbit.SchemaInput{ResponseFormat: "llm"})
	}))
}

// liveRawSchema asks for the structured ontology and fails unless it came
// back structured.
func liveRawSchema(ctx context.Context, client *gitlabclient.Client, input orbit.SchemaInput) (any, error) {
	out, err := orbit.Schema(ctx, client, input)
	if err != nil {
		return nil, err
	}
	if len(out.Domains) == 0 || len(out.Nodes) == 0 || len(out.Edges) == 0 {
		return nil, fmt.Errorf("raw schema = %+v, want the structured schema", out)
	}
	return fmt.Sprintf("domains=%d nodes=%d edges=%d", len(out.Domains), len(out.Nodes), len(out.Edges)), nil
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

// TestOrbitLiveGitLabCom_DetectTier_IsPaidWhereOrbitIsServed holds the tier
// detection to GitLab.com's own answer (issue 1224). GitLab.com serves Orbit
// only to a member, at Reporter or higher, of a group whose top-level group's
// plan carries the orbit feature, which GitlabSubscriptions::Features files
// under Premium, so a token GitLab.com serves Orbit to belongs to a paid group
// and the server must resolve it to Premium or higher whether or not it
// administers that group. Before issue 1224 the maintainer's token, a Developer
// of a subgroup of a licensed group, was served Orbit and resolved Free. A
// token GitLab.com does not serve Orbit to proves nothing about the tier, and
// the test says so and skips. It needs no fixture namespace, and reads only:
// the tier probe sends GET /license, GET /namespaces and one GraphQL query per
// page of top-level groups.
func TestOrbitLiveGitLabCom_DetectTier_IsPaidWhereOrbitIsServed(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	requireOrbitServed(t, client, orbitFixturesNamespace())
	if tier := client.DetectTier(ctx); tier < edition.Premium {
		t.Errorf("DetectTier() = %v for a token GitLab.com serves Orbit to, want Premium or higher: "+
			"Orbit is served only to a member of a group whose top-level group's plan is paid", tier)
	}
}

// TestOrbitLiveGitLabCom_Version_IsOneTheActionTableJudgesAGrantAt holds the
// release GitLab.com reports to the fine-grained action table this tree
// compiles in (issue 1023). A fine-grained session's grant is judged only at
// the table's own release, or for the listing alone at the prerelease of the
// release right after it, which is what GitLab.com reports between releases;
// anywhere else the session falls back to phase A, listed everything but what
// no fine-grained token reaches, with nothing in a run saying so. That is how
// GitLab.com moving from 19.5.0-pre to 19.6.0-pre put every fine-grained
// session there in phase A against the 19.4 table, found by a reading of the
// upstream register rather than by a test. The version is read the way the
// server reads the one it judges a grant at ([gitlabclient.Client.ReadVersion]),
// and the rule is internal/testutil/e2erelease's, the one -check-release and
// the e2e harness stop a run with ([gitLabComReleaseProblem]). It needs no
// fixture namespace and reads only GET /api/v4/version.
func TestOrbitLiveGitLabCom_Version_IsOneTheActionTableJudgesAGrantAt(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	version, answered := client.ReadVersion(ctx)
	if !answered {
		t.Fatal("GitLab.com did not answer GET /api/v4/version, so nothing can say which phase its fine-grained sessions are in")
	}
	table := actiongrants.Table()
	if problem := gitLabComReleaseProblem(version, table); problem != "" {
		t.Fatal(problem)
	}
	t.Logf("GitLab.com reports %s: %s", version, e2erelease.Decide(version, table).Text())
}

// gitLabComReleaseProblem returns why a fine-grained session on an instance
// reporting version would not have its grant judged by table, or "" when it
// would: at the table's own release (phase B) or at the prerelease right after
// it (phase B for the listing). It is internal/testutil/e2erelease's rule held
// tighter in one place: an older release is a pin a Docker run may choose and
// so goes ahead there, while GitLab.com reporting one would put every
// fine-grained session on it in phase A just as a newer one does.
func gitLabComReleaseProblem(version string, table *finegrained.Table) string {
	decision := e2erelease.Decide(version, table)
	switch decision.Verdict {
	case e2erelease.Recorded, e2erelease.ListingOnly:
		return ""
	case e2erelease.Older:
		return fmt.Sprintf("GitLab.com reported %s, which is older than the release the fine-grained action table was recorded from (%s): "+
			"no fine-grained grant is judged on it, so every fine-grained session on GitLab.com runs in phase A. "+
			"The table was recorded from a release GitLab.com has not reached; record it from the newest release GitLab.com has passed "+
			"(go run ./cmd/gen_api_live/ -image gitlab/gitlab-ee:<release>-ee.0, then make gen-action-grants).",
			version, decision.Table)
	default:
		return "GitLab.com: " + decision.Text()
	}
}

// TestGitLabComReleaseProblem_AgainstARecordedTable_PassesItsReleaseAndTheNextPrerelease
// holds the rule the live version test applies able to fail, against a
// stand-in table recorded from 19.5.0: the table's own releases and the 19.6
// prerelease pass, and an older release, a newer one, a prerelease two minors
// ahead and a version that does not parse are each refused with the reason a
// maintainer acts on. It needs no network and no token.
func TestGitLabComReleaseProblem_AgainstARecordedTable_PassesItsReleaseAndTheNextPrerelease(t *testing.T) {
	table := &finegrained.Table{Version: "19.5.0-ee", Bucket: "19.5"}
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "the release the table was recorded from", version: "19.5.0-ee"},
		{name: "a patch of that release", version: "19.5.2-ee"},
		{name: "the prerelease of that release", version: "19.5.0-pre"},
		{name: "the prerelease of the release right after it", version: "19.6.0-pre"},
		{name: "a prerelease two minors ahead", version: "19.7.0-pre", want: "make gen-action-grants"},
		{name: "the release right after it", version: "19.6.0-ee", want: "make gen-api-live"},
		{name: "an older release", version: "19.4.1-ee", want: "older than the release"},
		{name: "a version that does not parse", version: "", want: "does not start with a major.minor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gitLabComReleaseProblem(tt.version, table)
			if tt.want == "" {
				if got != "" {
					t.Errorf("gitLabComReleaseProblem(%q) = %q, want none", tt.version, got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("gitLabComReleaseProblem(%q) = %q, want it to contain %q", tt.version, got, tt.want)
			}
		})
	}
}

// TestOrbitLiveGitLabCom_Fixtures exercises the query types GitLab infers
// from GQL against the live fixture data the setup script provisions in any
// GitLab namespace (defaults to jmrp-oss/orbit-fixtures, configurable
// via ORBIT_FIXTURES_NAMESPACE). All subtests bound their nodes by path or by a
// literal property instead of hardcoded project ids so the test is portable
// across namespaces: any developer who runs `scripts/setup-orbit-fixtures.sh`
// with their own token gets a working test surface.
//
// The Orbit indexer is eventually consistent: if a fresh push has not
// been indexed yet, the affected subtest logs row_count=0 without failing.
// Re-run the live test a few minutes after the setup script completes to
// allow the indexer to catch up.
func TestOrbitLiveGitLabCom_Fixtures(t *testing.T) {
	testOrbitLiveFixtures(t)
}

// TestOrbitLiveGitLabCom_ReadAPIToken_PassesTheQueryScopeCheck measures what
// the server's classic scope derivation declares without a GitLab.com
// measurement: that the POST /orbit/query route accepts a token carrying
// read_api alone, as ee/lib/api/orbit/data.rb grants it at v19.4.1-ee
// (ADR-0026). It runs only with GITLAB_COM_READ_API_TOKEN, a developer's
// GitLab.com token carrying read_api and not api, since nothing mints one
// and GITLAB_COM_TOKEN carries api. GitLab's scope check runs before the
// route does anything, so an answer that is not insufficient_scope, an
// answer or a refusal for any other reason, is the scope check passed.
// [refusedForScope] tells the two apart, and
// TestRefusedForScope_ThroughTheQueryHandler_ReadsGitLabsAnswer holds it able
// to fail.
func TestOrbitLiveGitLabCom_ReadAPIToken_PassesTheQueryScopeCheck(t *testing.T) {
	token := os.Getenv("GITLAB_COM_READ_API_TOKEN")
	if token == "" {
		t.Skip("GITLAB_COM_READ_API_TOKEN not set; skipping the read_api measurement of the query route")
	}
	client, err := gitlabclient.NewClientWithToken(liveGitLabComURL, token, false)
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}
	requireOrbitServed(t, client, orbitFixturesNamespace())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := orbit.Query(ctx, client, readAPIQuery())
	if refusedForScope(err) {
		t.Fatalf("GitLab.com refused POST /orbit/query to a read_api token for its scope, which the derivation declares it accepts: %v", err)
	}
	if err != nil {
		t.Logf("the scope check passed and the route answered otherwise: %v", err)
		return
	}
	t.Logf("a read_api token queried the graph: query_type=%s row_count=%d", out.QueryType, out.RowCount)
}

// readAPIQuery is the query the read_api measurement sends: the id of the
// fixture project, found by its path as every query here finds one, which is
// about the least the route can be asked, in GQL.
func readAPIQuery() orbit.QueryInput {
	return orbit.QueryInput{Query: projectByPath(orbitFixturesNamespace(), kgFixturesProjectPath, "p.id")}
}

// refusedForScope reports whether err carries GitLab's refusal of the calling
// token for its scopes: a 403 whose body carries the RFC 6750 code
// insufficient_scope, or whose challenge does. The rendered error cannot be
// read for it, because the handler's error bounds and sanitizes GitLab's body
// and leaves out one that names the scope it wanted, which is exactly the
// body GitLab answers this refusal with; the answer is read from the response
// the error wraps instead.
func refusedForScope(err error) bool {
	var response *gl.ErrorResponse
	if !errors.As(err, &response) || response.Response == nil || response.Response.StatusCode != http.StatusForbidden {
		return false
	}
	if strings.Contains(response.Response.Header.Get("WWW-Authenticate"), `error="insufficient_scope"`) {
		return true
	}
	var body struct {
		Error string `json:"error"`
	}
	return json.Unmarshal(response.Body, &body) == nil && body.Error == "insufficient_scope"
}

// TestRefusedForScope_ThroughTheQueryHandler_ReadsGitLabsAnswer holds the
// discriminator of the read_api measurement able to fail: the query handler
// is run against a stand-in answering POST /orbit/query the way GitLab
// answers a token whose scopes do not reach a route, with the body this
// repository records for that refusal and with the challenge alone, and with
// the answers that are not that refusal. It needs no network and no token.
func TestRefusedForScope_ThroughTheQueryHandler_ReadsGitLabsAnswer(t *testing.T) {
	const scopeBody = `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"api"}`
	tests := []struct {
		name      string
		status    int
		challenge string
		body      string
		want      bool
	}{
		{name: "the body GitLab refuses a scope with", status: http.StatusForbidden, body: scopeBody, want: true},
		{name: "the challenge alone", status: http.StatusForbidden, challenge: `Bearer realm="", error="insufficient_scope"`, body: `{"message":"403 Forbidden"}`, want: true},
		{name: "a permission refusal", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`},
		{name: "a fine-grained refusal", status: http.StatusForbidden, body: `{"error":"insufficient_granular_scope","error_description":"Access denied"}`},
		{name: "the code on another status", status: http.StatusBadRequest, body: scopeBody},
		{name: "a route GitLab does not serve", status: http.StatusNotFound, body: `{"message":"404 Not Found"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/orbit/query") {
					t.Errorf("request = %s %s, want POST /orbit/query", r.Method, r.URL.Path)
				}
				if tt.challenge != "" {
					w.Header().Set("WWW-Authenticate", tt.challenge)
				}
				testutil.RespondJSON(w, tt.status, tt.body)
			}))
			_, err := orbit.Query(context.Background(), client, readAPIQuery())
			if err == nil {
				t.Fatal("orbit.Query() error = nil, want the stand-in's refusal")
			}
			if got := refusedForScope(err); got != tt.want {
				t.Errorf("refusedForScope(%v) = %t, want %t", err, got, tt.want)
			}
		})
	}
	t.Run("an error that wraps no response", func(t *testing.T) {
		if refusedForScope(errors.New("dial tcp: connection refused")) || refusedForScope(nil) {
			t.Error("refusedForScope() = true for an error that carries no answer from GitLab")
		}
	})
}

// notServedBody is the body GitLab.com refuses an Orbit route with to an
// account it does not serve Orbit to, as this repository records it.
const notServedBody = `{"message":"403 Forbidden - No Orbit enabled namespaces available"}`

// TestNotServed_ThroughTheAccountLevelHandlers_ReadsGitLabsAnswer holds the
// gate of the subtests that need only the account able to fail: the status
// and the tool list are run against a stand-in answering the two ways
// GitLab.com declined Orbit to the fixture account on 2026-10-29 (the service
// status null, the Orbit MCP endpoint's 403) and ways that say something else
// is wrong (the token refused, its scope or its fine-grained grant, the
// account blocked, a path GitLab does not serve, a server fault, a GraphQL
// error), and only the first two are read as Orbit not being served, so a
// fault of the handler still fails its subtest. It needs no network and no
// token.
func TestNotServed_ThroughTheAccountLevelHandlers_ReadsGitLabsAnswer(t *testing.T) {
	status := func(ctx context.Context, client *gitlabclient.Client) error {
		_, err := orbit.Status(ctx, client, orbit.StatusInput{})
		return err
	}
	tools := func(ctx context.Context, client *gitlabclient.Client) error {
		_, err := orbit.Tools(ctx, client, orbit.ToolsInput{})
		return err
	}
	tests := []struct {
		name   string
		call   func(context.Context, *gitlabclient.Client) error
		status int
		body   string
		want   bool
	}{
		{name: "the service status answered null", call: status, status: http.StatusOK, body: `{"data":{"orbit":{"serviceStatus":null}}}`, want: true},
		{name: "the Orbit MCP endpoint's permission refusal", call: tools, status: http.StatusForbidden, body: notServedBody, want: true},
		{name: "the token refused", call: tools, status: http.StatusUnauthorized, body: `{"message":"401 Unauthorized"}`},
		{name: "a scope refusal", call: tools, status: http.StatusForbidden, body: `{"error":"insufficient_scope","error_description":"The request requires higher privileges than provided by the access token.","scope":"api"}`},
		{name: "a fine-grained grant refusal", call: tools, status: http.StatusForbidden, body: `{"error":"insufficient_granular_scope","error_description":"Access denied"}`},
		{name: "the account blocked", call: tools, status: http.StatusForbidden, body: `{"message":"403 Forbidden - Your account has been blocked."}`},
		{name: "a path GitLab does not serve", call: tools, status: http.StatusNotFound, body: `{"message":"404 Not Found"}`},
		{name: "a server fault", call: tools, status: http.StatusInternalServerError, body: `{"message":"500 Internal Server Error"}`},
		{name: "a GraphQL error", call: status, status: http.StatusOK, body: `{"data":null,"errors":[{"message":"Internal server error"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, tt.body)
			}))
			err := tt.call(context.Background(), client)
			if err == nil {
				t.Fatal("handler error = nil, want the stand-in's answer")
			}
			if got := notServed(err); got != tt.want {
				t.Errorf("notServed(%v) = %t, want %t", err, got, tt.want)
			}
		})
	}
	t.Run("an error that wraps no response", func(t *testing.T) {
		if notServed(errors.New("dial tcp: connection refused")) || notServed(nil) {
			t.Error("notServed() = true for an error that carries no answer from GitLab")
		}
	})
}

// TestSkipUnlessServed_ANullTheHandlerReports_IsAskedOfGitLabAgain holds the
// gate able to tell GitLab's null from a handler's: the status and the schema
// handlers are run against a stand-in that answers their own documents with
// null, and the gate's document for the same field with null, with
// Query.orbit null, with a value, with a server fault or with a GraphQL error.
// Only the two nulls skip the subtest, so a handler that reads null where
// GitLab sent a value fails it. The Orbit MCP endpoint's permission refusal is
// skipped on without asking GitLab again, and a null reported by a subtest
// that reads only that endpoint fails. It needs no network and no token.
func TestSkipUnlessServed_ANullTheHandlerReports_IsAskedOfGitLabAgain(t *testing.T) {
	const (
		statusNull = `{"data":{"orbit":{"serviceStatus":null}}}`
		schemaNull = `{"data":{"orbit":{"schema":null}}}`
	)
	status := func(client *gitlabclient.Client) func(context.Context) (any, error) {
		return func(ctx context.Context) (any, error) { return orbit.Status(ctx, client, orbit.StatusInput{}) }
	}
	schema := func(client *gitlabclient.Client) func(context.Context) (any, error) {
		return func(ctx context.Context) (any, error) { return orbit.Schema(ctx, client, orbit.SchemaInput{}) }
	}
	tools := func(client *gitlabclient.Client) func(context.Context) (any, error) {
		return func(ctx context.Context) (any, error) { return orbit.Tools(ctx, client, orbit.ToolsInput{}) }
	}
	tests := []struct {
		name        string
		run         func(*gitlabclient.Client) func(context.Context) (any, error)
		field       *orbitField
		handler     string
		againStatus int
		again       string
		wantSkip    bool
		wantAsked   bool
	}{
		{name: "GitLab answers the status null again", run: status, field: readsServiceStatus, handler: statusNull, again: statusNull, wantSkip: true, wantAsked: true},
		{name: "GitLab answers Query.orbit null", run: status, field: readsServiceStatus, handler: statusNull, again: `{"data":{"orbit":null}}`, wantSkip: true, wantAsked: true},
		{name: "GitLab answers the status with a value", run: status, field: readsServiceStatus, handler: statusNull, again: `{"data":{"orbit":{"serviceStatus":{"version":"0.163.0"}}}}`, wantAsked: true},
		{name: "GitLab answers the schema null again", run: schema, field: readsSchema, handler: schemaNull, again: schemaNull, wantSkip: true, wantAsked: true},
		{name: "GitLab answers the schema with a value", run: schema, field: readsSchema, handler: schemaNull, again: `{"data":{"orbit":{"schema":{"nodes":{"nodes":[{"name":"Project"}]}}}}}`, wantAsked: true},
		{name: "asking GitLab again meets a server fault", run: status, field: readsServiceStatus, handler: statusNull, againStatus: http.StatusInternalServerError, again: `{"message":"500 Internal Server Error"}`, wantAsked: true},
		{name: "asking GitLab again meets a GraphQL error", run: status, field: readsServiceStatus, handler: statusNull, again: `{"data":null,"errors":[{"message":"Internal server error"}]}`, wantAsked: true},
		{name: "the Orbit MCP endpoint's permission refusal is not asked again", run: tools, field: readsMCPOnly, wantSkip: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			againStatus := http.StatusOK
			if tt.againStatus != 0 {
				againStatus = tt.againStatus
			}
			var asked atomic.Int32
			client := testutil.NewTestClient(t, askedAgainStandIn(t, tt.handler, againStatus, tt.again, &asked))
			_, err := skipUnlessServed(client, tt.field, tt.run(client))(context.Background())
			if err == nil {
				t.Fatal("skipUnlessServed() error = nil, want the stand-in's answer")
			}
			if got := errors.Is(err, errNotServed); got != tt.wantSkip {
				t.Errorf("skipUnlessServed() error = %v, wraps errNotServed = %t, want %t", err, got, tt.wantSkip)
			}
			if got := asked.Load() > 0; got != tt.wantAsked {
				t.Errorf("GitLab asked again = %t, want %t", got, tt.wantAsked)
			}
		})
	}
	t.Run("a null from a subtest that reads only the Orbit MCP endpoint", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("request = %s %s, want none", r.Method, r.URL.Path)
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
		}))
		reportsNull := func(context.Context) (any, error) {
			return nil, fmt.Errorf("orbit_tools: %w", orbit.ErrOrbitUnavailable)
		}
		_, err := skipUnlessServed(client, readsMCPOnly, reportsNull)(context.Background())
		if err == nil || errors.Is(err, errNotServed) {
			t.Errorf("skipUnlessServed() error = %v, want a failure that does not wrap errNotServed", err)
		}
	})
}

// askedAgainStandIn is the GitLab of
// TestSkipUnlessServed_ANullTheHandlerReports_IsAskedOfGitLabAgain: it refuses
// the Orbit MCP endpoint as GitLab.com refuses an account it does not serve,
// answers a handler's GraphQL document with handlerAnswer, and answers the
// gate's own document ([orbitField]), counted in asked, with againStatus and
// again.
func askedAgainStandIn(t *testing.T, handlerAnswer string, againStatus int, again string, asked *atomic.Int32) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/orbit/mcp") {
			testutil.RespondJSON(w, http.StatusForbidden, notServedBody)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read the request body: %v", err)
			return
		}
		if !strings.Contains(string(body), "OrbitLive") {
			testutil.RespondJSON(w, http.StatusOK, handlerAnswer)
			return
		}
		asked.Add(1)
		testutil.RespondJSON(w, againStatus, again)
	})
}

// testOrbitLiveFixtures exercises the query types GitLab infers from GQL
// against the live fixture data the setup script provisions in any GitLab
// namespace (defaults to jmrp-oss/orbit-fixtures, configurable via
// ORBIT_FIXTURES_NAMESPACE). All subtests bound their nodes by path or by a
// literal property instead of hardcoded project ids so the test is portable
// across namespaces.
//
// The Orbit indexer is eventually consistent: if a fresh push has not
// been indexed yet, the affected subtest will log row_count=0 without
// failing. Re-run the live test a few minutes after the setup script
// completes to allow the indexer to catch up.
func testOrbitLiveFixtures(t *testing.T) {
	t.Helper()
	client := newLiveClient(t)
	namespace := orbitFixturesNamespace()
	requireOrbitServed(t, client, namespace)

	summarize(t, "Project_kg_fixtures_by_path", queryRun(client,
		projectByPath(namespace, kgFixturesProjectPath, "p.id, p.full_path, p.name")))

	summarize(t, "Project_security_fixtures_by_equality", queryRun(client,
		"MATCH (p:Project) WHERE p.full_path = '"+projectPath(namespace, securityFixturesProjectPath)+"' RETURN p.id, p.full_path"))

	// Count active milestones. The setup script provisions one active
	// milestone (the KG coverage one); other namespaces may hold more.
	summarize(t, "Milestone_count_active", queryRun(client,
		"MATCH (m:Milestone {state: 'active'}) RETURN count(m) AS active_milestones"))

	// Count Python source files. The File entity does not expose a project
	// property to bound by, so the count is by path suffix across every file
	// the caller may read. Each fixture project contributes 8-9 .py files.
	summarize(t, "File_count_python", queryRun(client,
		"MATCH (f:File) WHERE f.path ENDS WITH '.py' RETURN count(f) AS python_files"))

	// Fetch the squash-merged MR the setup script creates. The MR id is
	// dynamic per instance, so it is matched by its branches.
	summarize(t, "MergeRequest_in_kg_fixtures", queryRun(client,
		"MATCH (mr:MergeRequest {source_branch: '"+fixtureBranch+"', target_branch: 'main'}) "+
			"RETURN mr.id, mr.iid, mr.title, mr.state, mr.source_branch"))

	// Count detected vulnerabilities. The setup script provisions 5 such
	// findings (1 critical AWS key, 3 high SQLi/eval, 1 medium weak-hash) via
	// the SAST and Secret Detection templates. The exact number depends on how
	// many analyzers have finished.
	summarize(t, "Vulnerability_count_detected", queryRun(client,
		"MATCH (v:Vulnerability {state: 'detected'}) RETURN count(v) AS open_vulnerabilities"))

	// What the kg-fixtures project is connected to, in both directions: its
	// group, creator and labels are reached by incoming relationships, which
	// the undirected pattern takes with the outgoing ones.
	summarize(t, "Neighbors_kg_fixtures", queryRun(client,
		"MATCH (p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'})--(n) RETURN n"))
}

// TestOrbitLiveGitLabCom_FeatureCoverage exercises the GQL surface against
// the live fixture namespace: comparison and string predicates, a list
// membership, multi-node patterns with a relationship type, aggregations
// grouped by a property and by a node with their ORDER BY, sum, max and avg,
// ORDER BY on a property, the virtual diff and content properties, SKIP and
// LIMIT, an id range, the incoming neighbors of one relationship type, the
// properties of every neighbor, and the ontology procedure. Each subtest is
// informational: it PASSes as long as the API accepts the statement and
// returns a valid envelope, even if row_count=0 (the data may not match the
// predicate in this namespace). This is the comprehensive coverage test:
// every documented statement pattern the API supports is exercised at least
// once against real data.
func TestOrbitLiveGitLabCom_FeatureCoverage(t *testing.T) {
	testOrbitLiveFeatureCoverage(t)
}

// testOrbitLiveFeatureCoverage runs the three groups of
// [TestOrbitLiveGitLabCom_FeatureCoverage].
func testOrbitLiveFeatureCoverage(t *testing.T) {
	t.Helper()
	client := newLiveClient(t)
	namespace := orbitFixturesNamespace()
	requireOrbitServed(t, client, namespace)
	testOrbitLiveFeatureCoveragePredicatesAndTraversal(t, client, namespace)
	testOrbitLiveFeatureCoverageAggregation(t, client, namespace)
	testOrbitLiveFeatureCoverageVirtualAndMeta(t, client, namespace)
}

// testOrbitLiveFeatureCoveragePredicatesAndTraversal exercises IN, CONTAINS,
// a comparison, several predicates joined by AND, and a pattern that joins
// MergeRequest to Project through the IN_PROJECT relationship.
func testOrbitLiveFeatureCoveragePredicatesAndTraversal(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Predicate_in_list_severity", queryRun(client,
		"MATCH (v:Vulnerability) WHERE v.severity IN ['critical', 'high'] RETURN v.id, v.severity"))

	summarize(t, "Predicate_contains_path", queryRun(client,
		"MATCH (f:File) WHERE f.path CONTAINS 'orders' RETURN f.id, f.path LIMIT 5"))

	summarize(t, "Predicate_greater_than", queryRun(client,
		"MATCH (p:Project) WHERE p.star_count > 0 RETURN p.id, p.full_path, p.star_count LIMIT 10"))

	// Several predicates AND-combine, two of them on one property.
	summarize(t, "Predicates_combined_with_AND", queryRun(client,
		"MATCH (p:Project) WHERE "+underNamespace(namespace)+" AND p.star_count >= 0 AND p.star_count < 1000 RETURN p.id, p.star_count"))

	// The classic "find MRs in a project" pattern: IN_PROJECT joins
	// MergeRequest to Project, and both nodes' properties are returned.
	summarize(t, "Traversal_project_to_merge_requests", queryRun(client,
		"MATCH (mr:MergeRequest)-[:IN_PROJECT]->(p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'}) "+
			"RETURN p.id, p.full_path, mr.id, mr.iid, mr.title, mr.state LIMIT 5"))
}

// testOrbitLiveFeatureCoverageAggregation exercises the aggregate functions
// (count, sum, max, avg) and both groupings: by a property (severity) and by
// a node joined through the IN_PROJECT relationship, each sorted by its
// aliased aggregate.
func testOrbitLiveFeatureCoverageAggregation(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Aggregation_grouped_by_severity", queryRun(client,
		"MATCH (v:Vulnerability {state: 'detected'}) RETURN v.severity, count(v) AS vuln_count ORDER BY vuln_count DESC"))

	summarize(t, "Aggregation_grouped_by_node_with_relationship", queryRun(client,
		"MATCH (mr:MergeRequest)-[:IN_PROJECT]->(p:Project) WHERE "+underNamespace(namespace)+
			" RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC"))

	starsOf := func(function, alias string) func(context.Context) (any, error) {
		return queryRun(client, "MATCH (p:Project) WHERE "+underNamespace(namespace)+" RETURN "+function+"(p.star_count) AS "+alias)
	}
	summarize(t, "Aggregation_sum_star_count", starsOf("sum", "total_stars"))
	summarize(t, "Aggregation_max_star_count", starsOf("max", "max_stars"))
	summarize(t, "Aggregation_avg_star_count", starsOf("avg", "avg_stars"))
}

// testOrbitLiveFeatureCoverageVirtualAndMeta exercises ORDER BY, the virtual
// diff and content properties, SKIP and LIMIT, an id range, the incoming neighbors of
// one relationship type, the properties of every neighbor, and the ontology
// procedure.
func testOrbitLiveFeatureCoverageVirtualAndMeta(t *testing.T, client *gitlabclient.Client, namespace string) {
	t.Helper()
	summarize(t, "Traversal_order_by_name_desc", queryRun(client,
		"MATCH (p:Project) WHERE "+underNamespace(namespace)+" RETURN p.id, p.full_path, p.name ORDER BY p.name DESC LIMIT 5"))

	summarize(t, "Traversal_merge_request_with_diff_property", queryRun(client,
		"MATCH (mr:MergeRequest {source_branch: '"+fixtureBranch+"'}) RETURN mr.id, mr.iid, mr.title, mr.diff LIMIT 1"))

	summarize(t, "Traversal_file_with_content_property", queryRun(client,
		"MATCH (f:File) WHERE f.path ENDS WITH 'models.py' RETURN f.id, f.path, f.language, f.content LIMIT 1"))

	// A page is SKIP and LIMIT since GitLab.com retired PAGE and AFTER
	// (issue 1527): the second project of the namespace by id.
	summarize(t, "Traversal_skip_limit", queryRun(client,
		"MATCH (p:Project) WHERE "+underNamespace(namespace)+" RETURN p.id, p.full_path ORDER BY p.id SKIP 1 LIMIT 1"))

	summarize(t, "Traversal_id_range", func(ctx context.Context) (any, error) {
		ids, err := fixtureProjectIDs(ctx, client, namespace)
		if err != nil {
			return nil, err
		}
		return queryRun(client, "MATCH (p:Project) WHERE p.id >= "+ids[0]+" AND p.id <= "+ids[0]+" RETURN p.id, p.full_path LIMIT 5")(ctx)
	})

	summarize(t, "Neighbors_incoming_of_one_relationship_type", queryRun(client,
		"MATCH (p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'})<-[:IN_PROJECT]-(n) RETURN n LIMIT 5"))

	summarize(t, "Neighbors_with_their_properties", queryRun(client,
		"MATCH (p:Project {full_path: '"+projectPath(namespace, kgFixturesProjectPath)+"'})--(n) RETURN properties(n) LIMIT 5"))

	// The ontology procedure GQL carries, answered like orbit.schema with
	// one entity named.
	summarize(t, "Schema_procedure_for_one_entity", func(ctx context.Context) (any, error) {
		out, err := liveQuery(ctx, client, "CALL db.schema('Project')")
		if err != nil {
			return nil, err
		}
		if out.Result == nil {
			return nil, errors.New("CALL db.schema('Project') answered no result")
		}
		return fmt.Sprintf("query_type=%q result=present", out.QueryType), nil
	})
}
