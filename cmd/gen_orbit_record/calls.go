// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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

// callSpec is one recorded call: which handler, with what, where in its answer
// the body the handler reads is, and which of that body's subtrees are data
// rather than shape.
type callSpec struct {
	id orbitrecord.CallID
	// root is where the answer is recorded from (see [rootedAnswer]), empty
	// for a REST body, which is recorded whole.
	root string
	// verbatim names the paths recorded as their root only.
	verbatim []string
	invoke   func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error)
}

// formatted is the response format input orbit.dsl takes.
func formatted(format string) orbit.ResponseFormatInput {
	return orbit.ResponseFormatInput{ResponseFormat: format}
}

// fixtureQuery is the traversal the query calls send: the fixture project by
// its full path, three of its properties. It is written in GQL, the language
// GitLab.com reads every query in since 2026-10-15 (issue 1414); the full
// path is a namespace this repository's own scripts provision, so it needs no
// quoting beyond the single quotes around it.
func fixtureQuery(namespace string) string {
	return "MATCH (p:Project {full_path: '" + namespace + "/" + fixtureProject + "'}) RETURN p.id, p.full_path, p.name"
}

// The roots of the GraphQL calls: the field below data each handler reads.
const (
	rootServiceStatus = "data.orbit.serviceStatus"
	rootSchema        = "data.orbit.schema"
	rootGroupOrbit    = "data.group.orbit"
)

// callSpecs is every call a recording makes, in [orbitrecord.ExpectedCalls]
// order. The verbatim paths are the answers whose keys are data: a JSON
// Schema document names its own properties (a tool's input schema, and the
// query command's input schema orbit.dsl reads), and a query row carries the
// columns the query asked for.
func callSpecs() []callSpec {
	return []callSpec{
		{id: orbitrecord.CallID{Action: orbitrecord.StatusAction, Variant: "default"}, root: rootServiceStatus, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Status(ctx, client, orbit.StatusInput{})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.SchemaAction, Variant: "raw"}, root: rootSchema, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Schema(ctx, client, orbit.SchemaInput{Format: "raw"})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.SchemaAction, Variant: "llm"}, root: rootMCPText, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Schema(ctx, client, orbit.SchemaInput{Format: "llm"})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.SchemaAction, Variant: "expand"}, root: rootSchema, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Schema(ctx, client, orbit.SchemaInput{Expand: []string{"Project"}, Format: "raw"})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.ToolsAction, Variant: "default"}, root: rootMCPResult, verbatim: []string{"tools[].inputSchema"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.Tools(ctx, client, orbit.ToolsInput{})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.DSLAction, Variant: "raw"}, root: rootMCPText, verbatim: []string{"[].parameters"}, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormatInput: formatted("raw")})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.DSLAction, Variant: "llm"}, root: rootMCPText, invoke: func(ctx context.Context, client *gitlabclient.Client, _ string) (any, error) {
			return orbit.DSL(ctx, client, orbit.DSLInput{ResponseFormatInput: formatted("llm")})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.QueryAction, Variant: "raw"}, verbatim: []string{"result.edges[]", "result.nodes[]"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.Query(ctx, client, orbit.QueryInput{Query: fixtureQuery(namespace), ResponseFormat: "raw"})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.QueryAction, Variant: "llm"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.Query(ctx, client, orbit.QueryInput{Query: fixtureQuery(namespace), ResponseFormat: "llm"})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.QueryAction, Variant: "gql"}, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.Query(ctx, client, orbit.QueryInput{Query: fixtureQuery(namespace), ResponseFormat: "gql"})
		}},
		{id: orbitrecord.CallID{Action: orbitrecord.GraphStatusAction, Variant: "full_path"}, root: rootGroupOrbit, invoke: func(ctx context.Context, client *gitlabclient.Client, namespace string) (any, error) {
			return orbit.GraphStatus(ctx, client, orbit.GraphStatusInput{FullPath: namespace})
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
// record they add up to. previous is the record committed at HEAD, or nil,
// which a call GitLab refused the account is carried from when the run allows
// unanswered calls.
func recordCalls(ctx context.Context, cfg genRun, previous *orbitrecord.Document) (orbitrecord.Document, error) {
	listener, err := listen("tcp", "127.0.0.1:0")
	if err != nil {
		return orbitrecord.Document{}, fmt.Errorf("start the recording proxy: %w", err)
	}
	proxy := newRecorder(cfg.upstream, cfg.client)
	// The handlers' client is on this machine, so the headers of a request
	// wait no longer than the run itself may.
	server := &http.Server{Handler: proxy, ReadHeaderTimeout: cfg.timeout}
	// One exchange per connection. The handlers' client speaks through the
	// process-wide pool of internal/gitlab, which keeps an idle connection to
	// this address once the run is over; a later proxy listening on the same
	// freed port would be handed it, and since the handlers read GraphQL and
	// the Orbit MCP endpoint, the first request is a POST, which net/http does
	// not retry when the dead connection answers "server closed idle
	// connection". No connection outlives its exchange, so none is left over.
	server.SetKeepAlivesEnabled(false)
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	// Plain HTTP on purpose: the proxy listens on 127.0.0.1 inside this
	// process, so the hop never leaves the machine, and the leg to GitLab.com
	// the proxy makes is the one TLS protects.
	proxyURL := url.URL{Scheme: "http", Host: listener.Addr().String()}
	client, err := newClient(proxyURL.String(), cfg.token)
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
			var refused *refusal
			if !cfg.allowUnanswered || !errors.As(callErr, &refused) {
				return orbitrecord.Document{}, callErr
			}
			carryOrSkip(&doc, previous, refused, cfg.now())
			continue
		}
		if indexErr := indexedAnswer(spec.id, output); indexErr != nil {
			return orbitrecord.Document{}, indexErr
		}
		if status, isStatus := output.(orbit.StatusOutput); isStatus {
			doc.Source.OrbitVersion = status.Version
		}
		if dsl, isDSL := output.(orbit.DSLOutput); isDSL && spec.id.Variant == "raw" {
			doc.Source.QueryGrammar = grammarOf(dsl)
		}
		doc.Calls = append(doc.Calls, call)
	}
	return orbitrecord.Canonical(doc), nil
}

// grammarOf is what the record names the query grammar by: the stamp of the
// query command orbit.dsl answers with raw, its description and the JSON
// Schema of its input. An answer that described no query command stamps
// nothing, and [orbitrecord.Problems] then refuses the record rather than
// writing one that cannot report the grammar changing.
func grammarOf(dsl orbit.DSLOutput) string {
	return orbitrecord.GrammarStamp(dsl.Description, dsl.Parameters)
}

// refusal is a call GitLab did not answer for the account: refused 401 or
// 403, answered with null where the handler reads, which is how GraphQL
// answers a field Orbit does not serve the user, or answered with a top-level
// group's Orbit access and no index ([declinedByAccess]). It is the one
// failure a recording told to allow it (-allow-unanswered) records rather than
// stops on, and it carries the request so an earlier answer to the same one
// can be kept.
type refusal struct {
	id      orbitrecord.CallID
	request orbitrecord.Call
	reason  string
	err     error
}

// Error renders the refusal with the handler's own error.
func (r *refusal) Error() string {
	return fmt.Sprintf("%s was not answered for this account (%s), so its answer cannot be recorded: %v", r.id, r.reason, r.err)
}

// Unwrap returns the handler's error, or [errAccessDeclined] for an answer the
// handler published.
func (r *refusal) Unwrap() error { return r.err }

// refusedBy reports whether a failed call's one exchange was GitLab declining
// the account rather than a shape the handler could not read, and why: a 401
// or 403, or a 200 whose answer is null where the handler reads, which the
// handler reports as [orbit.ErrOrbitUnavailable].
func refusedBy(ex exchange, handlerErr error) (reason string, refused bool) {
	if ex.status == http.StatusUnauthorized || ex.status == http.StatusForbidden {
		return fmt.Sprintf("%s %s answered %d %s", ex.method, ex.path, ex.status, strings.TrimSpace(string(ex.payload))), true
	}
	if ex.status == http.StatusOK && errors.Is(handlerErr, orbit.ErrOrbitUnavailable) {
		return fmt.Sprintf("%s %s (%s) answered 200 with null where the handler reads, which is GitLab's answer to an account Orbit is not available to", ex.method, ex.path, ex.operation), true
	}
	return "", false
}

// errAccessDeclined is the error a [refusal] carries for an indexing status
// the handler answered without failing, which [declinedByAccess] reads as
// GitLab declining the account.
var errAccessDeclined = errors.New("GitLab.com serves no Orbit index under the top-level group's access")

// declinedByAccess reports whether a call that succeeded is GitLab declining
// the account all the same, and why: the indexing status of a top-level group
// whose Orbit access GitLab.com serves no index under, which GitLab answers
// with the access and trial and the index null, and the handler publishes as
// they came (orbit.GraphStatus). Recorded, it would be a key tree with no
// index in it, which the next recording made with access would report as keys
// GitLab added; it is a refusal instead, which -allow-unanswered carries or
// lists. The fixture namespace is a subgroup by default, for which GitLab
// answers no access and the handler a null index, so this is the answer of a
// fixture namespace that is itself a top-level group.
func declinedByAccess(ex exchange, output any) (string, bool) {
	status, isStatus := output.(orbit.GraphStatusOutput)
	if !isStatus || status.Access == nil || status.Access.Serves() {
		return "", false
	}
	return fmt.Sprintf("%s %s (%s) answered 200 with the top-level group's Orbit access %s, under which GitLab.com serves no index", ex.method, ex.path, ex.operation, status.Access.State), true
}

// makeCall invokes one handler and reads the one exchange it made.
func makeCall(ctx context.Context, client *gitlabclient.Client, proxy *recorder, spec callSpec, namespace string) (orbitrecord.Call, any, error) {
	output, err := spec.invoke(ctx, client, namespace)
	exchanges := proxy.take()
	if err != nil {
		if len(exchanges) == 1 {
			if reason, refused := refusedBy(exchanges[0], err); refused {
				return orbitrecord.Call{}, nil, &refusal{id: spec.id, request: requestOf(spec, exchanges[0]), reason: reason, err: err}
			}
		}
		return orbitrecord.Call{}, nil, fmt.Errorf("%s failed, so its answer cannot be recorded: %w", spec.id, err)
	}
	if len(exchanges) != 1 {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s made %d Orbit requests and a call is recorded from exactly one", spec.id, len(exchanges))
	}
	ex := exchanges[0]
	if reason, declined := declinedByAccess(ex, output); declined {
		return orbitrecord.Call{}, nil, &refusal{id: spec.id, request: requestOf(spec, ex), reason: reason, err: errAccessDeclined}
	}
	rootType, rooted, err := rootedAnswer(spec.root, ex.contentType, ex.payload)
	if err != nil {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s: %s: %w", spec.id, ex.describe(), err)
	}
	keys, err := keyTree(rootType, rooted, spec.verbatim)
	if err != nil {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s: %s: %w", spec.id, ex.describe(), err)
	}
	rootContentType := ""
	if spec.root != "" {
		rootContentType = rootType
	}
	outputName, err := outputType(output)
	if err != nil {
		return orbitrecord.Call{}, nil, fmt.Errorf("%s: %w", spec.id, err)
	}
	call := requestOf(spec, ex)
	call.Output = outputName
	call.Response = orbitrecord.Response{
		Status:          ex.status,
		ContentType:     ex.contentType,
		Root:            spec.root,
		RootContentType: rootContentType,
		Keys:            keys,
	}
	return call, output, nil
}

// requestOf is the call one exchange made, before its answer is read.
func requestOf(spec callSpec, ex exchange) orbitrecord.Call {
	return orbitrecord.Call{
		Action:  spec.id.Action,
		Variant: spec.id.Variant,
		Request: orbitrecord.Request{
			Method: ex.method, Path: ex.path, Query: ex.query, Body: ex.body,
			Operation: ex.operation, Arguments: ex.arguments,
		},
	}
}

// carryOrSkip is what a recording allowed to go without an answer does with a
// refused call: it keeps the answer an earlier recording got to the same
// request, dated by that recording, or else lists the call as unrecorded with
// the reason and the time GitLab gave it.
func carryOrSkip(doc, previous *orbitrecord.Document, refused *refusal, now time.Time) {
	if previous != nil {
		for _, call := range previous.Calls {
			if call.ID() != refused.id || !call.SameRequest(refused.request) {
				continue
			}
			if call.RetrievedAt == "" {
				call.RetrievedAt = previous.Source.RetrievedAt
			}
			doc.Calls = append(doc.Calls, call)
			return
		}
	}
	doc.Unrecorded = append(doc.Unrecorded, orbitrecord.Unrecorded{
		Action:  refused.id.Action,
		Variant: refused.id.Variant,
		Reason:  refused.reason + " at " + now.UTC().Format(time.RFC3339) + ", and no earlier recording holds an answer to this request",
	})
}

// errUnindexed is what a recording over a fixture namespace the Knowledge
// Graph has not indexed ends with.
var errUnindexed = errors.New("the fixture namespace is not indexed, so the answers lack the rows and counts whose shape the record holds; wait for the indexer (make orbit-wait-indexer) and record again")

// indexedAnswer refuses the two answers a fixture namespace the indexer has not
// reached gives: a traversal of the fixture project that finds no row, and an
// indexing status that counts no indexed project. Either is a success, so the
// handler does not fail, and either records a key tree without the paths an
// indexed namespace answers with (the rows of result.nodes, the per-domain
// counts), which the next recording would then report as keys GitLab added.
// make orbit-wait-indexer proceeds when it times out, so this is where a
// recording over an unindexed namespace is stopped.
func indexedAnswer(id orbitrecord.CallID, output any) error {
	if id.Variant != "raw" && id.Variant != "full_path" {
		return nil
	}
	switch typed := output.(type) {
	case orbit.QueryOutput:
		if typed.RowCount == 0 {
			return fmt.Errorf("%s found no row for the fixture project: %w", id, errUnindexed)
		}
	case orbit.GraphStatusOutput:
		if typed.Projects == nil || typed.Projects.Indexed == 0 {
			return fmt.Errorf("%s counts no indexed project: %w", id, errUnindexed)
		}
	}
	return nil
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
