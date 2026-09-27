package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/orbit"
)

// fixtureProject is the project of the fixture namespace the query asks
// about, provisioned by scripts/setup-orbit-fixtures.sh.
const fixtureProject = "kg-fixtures"

// modulePath is this repository's module, trimmed from a Go type's package
// path so the record names it the way every other audit record does.
const modulePath = "github.com/jmrplens/gitlab-mcp-server/v3/"

// recordNote is the record's own explanation of itself, for a reader who
// meets the file before the command.
const recordNote = "What GitLab.com's Orbit routes answered the requests this server's handlers build, as a key tree with no values: written by cmd/gen_orbit_record, judged offline by its -check, and compared with the Orbit output types by cmd/audit_1to1 -scope=paths. A verbatim key is recorded as its root only, because its keys are data."

// callSpec is one recorded call: which handler, with what, and which of its
// answer's subtrees are data rather than shape.
type callSpec struct {
	id orbitrecord.CallID
	// verbatim names the paths recorded as their root only.
	verbatim []string
	invoke   func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error)
}

// formatted is the response format input the handlers share.
func formatted(format string) orbit.ResponseFormatInput {
	return orbit.ResponseFormatInput{ResponseFormat: format}
}

// fixtureQuery is the traversal the query calls send: the fixture project by
// its full path, three of its columns. It is written in the DSL GitLab.com
// serves now, a nodes list and a bare equality filter.
func fixtureQuery(namespace string) map[string]any {
	return map[string]any{
		"query_type": "traversal",
		"nodes": []any{map[string]any{
			"id":      "p",
			"entity":  "Project",
			"filters": map[string]any{"full_path": namespace + "/" + fixtureProject},
			"columns": []any{"id", "full_path", "name"},
		}},
	}
}

// callSpecs is every call a recording makes, in [orbitrecord.ExpectedCalls]
// order. The verbatim paths are the answers whose keys are data: a JSON
// Schema document names its own properties, and a query row carries the
// columns the query asked for.
func callSpecs() []callSpec {
	return []callSpec{
		{id: orbitrecord.CallID{Action: "orbit.status", Variant: "raw"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Status(ctx, client, orbit.StatusInput{ResponseFormatInput: formatted("raw")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.status", Variant: "llm"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Status(ctx, client, orbit.StatusInput{ResponseFormatInput: formatted("llm")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.schema", Variant: "raw"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Schema(ctx, client, orbit.SchemaInput{Format: "raw"})
		}},
		{id: orbitrecord.CallID{Action: "orbit.schema", Variant: "llm"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Schema(ctx, client, orbit.SchemaInput{Format: "llm"})
		}},
		{id: orbitrecord.CallID{Action: "orbit.schema", Variant: "expand"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Schema(ctx, client, orbit.SchemaInput{Expand: []string{"Project"}, Format: "raw"})
		}},
		{id: orbitrecord.CallID{Action: "orbit.tools", Variant: "raw"}, verbatim: []string{"[].parameters"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Tools(ctx, client, orbit.ToolsInput{})
		}},
		{id: orbitrecord.CallID{Action: "orbit.dsl", Variant: "raw"}, verbatim: []string{orbitrecord.Root}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormatInput: formatted("raw")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.dsl", Variant: "llm"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormatInput: formatted("llm")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.query", Variant: "raw"}, verbatim: []string{"result.edges[]", "result.nodes[]"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.Query(ctx, client, orbit.QueryInput{Query: fixtureQuery(namespace), ResponseFormatInput: formatted("raw")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.query", Variant: "llm"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.Query(ctx, client, orbit.QueryInput{Query: fixtureQuery(namespace), ResponseFormatInput: formatted("llm")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.graph_status", Variant: "raw"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{FullPath: namespace, ResponseFormatInput: formatted("raw")})
		}},
		{id: orbitrecord.CallID{Action: "orbit.graph_status", Variant: "llm"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{FullPath: namespace, ResponseFormatInput: formatted("llm")})
		}},
	}
}

// Seams over what a recording needs from outside the process: a listener for
// the proxy, and the client the handlers are given. Both are variables so a
// test reaches the failure of each.
var (
	listen    = net.Listen
	newClient = func(baseURL, token string) (*gitlabclient.Client, error) {
		// Retries off: a retried request would be two exchanges for one call,
		// and a recording is of what one request was answered with.
		return gitlabclient.NewClientWithTokenRetries(baseURL, token, false, true)
	}
)

// recordCalls makes every expected call through the handlers and returns the
// record they add up to.
func recordCalls(ctx context.Context, cfg genRun) (orbitrecord.Document, error) {
	listener, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		return orbitrecord.Document{}, fmt.Errorf("start the recording proxy: %w", err)
	}
	proxy := newRecorder(cfg.upstream, cfg.client)
	// The handlers' client is on this machine, so the headers of a request
	// wait no longer than the run itself may.
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: cfg.timeout}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	client, err := newClient("http://"+listener.Addr().String(), cfg.token)
	if err != nil {
		return orbitrecord.Document{}, fmt.Errorf("build the client the handlers call through: %w", err)
	}

	doc := orbitrecord.Document{
		SchemaVersion: orbitrecord.SchemaVersion,
		Note:          recordNote,
		Source: orbitrecord.Source{
			Instance:    cfg.upstream,
			Namespace:   cfg.namespace,
			RetrievedAt: cfg.now().UTC().Format(time.DateOnly),
		},
	}
	for _, spec := range callSpecs() {
		call, output, callErr := makeCall(ctx, client, proxy, spec, cfg.namespace)
		if callErr != nil {
			return orbitrecord.Document{}, callErr
		}
		if status, isStatus := output.(orbit.StatusOutput); isStatus && spec.id.Variant == "raw" {
			doc.Source.OrbitVersion = status.Version
		}
		doc.Calls = append(doc.Calls, call)
	}
	return orbitrecord.Canonical(doc), nil
}

// makeCall invokes one handler and reads the one exchange it made.
func makeCall(ctx context.Context, client *gitlabclient.Client, proxy *recorder, spec callSpec, namespace string) (orbitrecord.Call, any, error) {
	output, err := spec.invoke(ctx, client, namespace)
	exchanges := proxy.take()
	if err != nil {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s failed, so its answer cannot be recorded: %w", spec.id, err)
	}
	if len(exchanges) != 1 {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s made %d Orbit requests and a call is recorded from exactly one", spec.id, len(exchanges))
	}
	ex := exchanges[0]
	keys, err := keyTree(ex.contentType, ex.payload, spec.verbatim)
	if err != nil {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s: %s: %w", spec.id, ex.describe(), err)
	}
	outputName, err := outputType(output)
	if err != nil {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s: %w", spec.id, err)
	}
	return orbitrecord.Call{
		Action:  spec.id.Action,
		Variant: spec.id.Variant,
		Output:  outputName,
		Request: orbitrecord.Request{Method: ex.method, Path: ex.path, Query: ex.query, Body: ex.body},
		Response: orbitrecord.Response{
			Status:      ex.status,
			ContentType: ex.contentType,
			Keys:        keys,
		},
	}, output, nil
}

// errForeignType is what [outputType] answers for a type declared outside this
// repository, which no audit here could find.
var errForeignType = errors.New("the handler returned a type declared outside this module")

// outputType names the type a handler returned, repository relative:
// internal/tools/orbit.StatusOutput.
func outputType(output any) (string, error) {
	typ := reflect.TypeOf(output)
	pkg, inModule := strings.CutPrefix(typ.PkgPath(), modulePath)
	if !inModule {
		return "", fmt.Errorf("%w: %s", errForeignType, typ)
	}
	return pkg + "." + typ.Name(), nil
}
