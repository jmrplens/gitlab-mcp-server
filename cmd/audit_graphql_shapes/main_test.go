package main

import (
	"bytes"
	"errors"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqldocs"
)

// backtick stands in for a backtick inside a fixture source, which is itself
// written as a raw string literal and so cannot contain one.
const backtick = "@@"

// gqlPackage stands in for client-go. The audit matches the request type by
// name, so a fixture module needs no dependency to be seen, and a throwaway
// module in a temporary directory is type-checked with nothing fetched.
const gqlPackage = `package gql

type GraphQLQuery struct {
	Query     string
	Variables map[string]any
}

type Service struct{}

func (Service) Do(query GraphQLQuery, response any, options ...func()) (int, error) { return 0, nil }
`

// testSchema is the schema the fixtures are judged by: one field per scalar
// class and per shape the judge distinguishes, and nothing else.
const testSchema = `
scalar Time
scalar BigInt
scalar JSON
scalar Duration
scalar Mystery
scalar VulnerabilityID

enum Severity { HIGH LOW }

interface Node { id: ID! }

type Query {
  project(fullPath: ID!): Project
  node: Node
  vulnerability(id: VulnerabilityID!): Vulnerability
}

type Project {
  id: ID!
  gid: VulnerabilityID!
  name: String!
  stars: Int!
  score: Float!
  archived: Boolean!
  createdAt: Time!
  size: BigInt!
  meta: JSON!
  runtime: Duration!
  severity: Severity!
  labels: [Label!]!
  mystery: Mystery
  pipeline: Pipeline
}

type Label { title: String! }
type Pipeline { id: ID! }
type Vulnerability implements Node { id: ID! title: String! }

type Mutation {
  createNote(input: NoteInput!): NotePayload
  updateNote(input: NoteInput!): NotePayload
}
input NoteInput { body: String! }
type NotePayload { note: Note errors: [String!]! }
type Note { id: ID! body: String! }

type Subscription { ping: String! }
`

// fixtureModule writes a throwaway module holding the stand-in gql package
// and one package per entry, and returns its root. A source's "@@" is a
// backtick.
func fixtureModule(t *testing.T, packages map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	sources := map[string]string{"gql": gqlPackage}
	for name, source := range packages {
		sources[name] = strings.ReplaceAll(source, backtick, "`")
	}
	for name, source := range sources {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".go"), []byte(source), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
	}
	return root
}

// schemaFile writes an SDL where -schema can read it.
func schemaFile(t *testing.T, sdl string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema.graphql")
	if err := os.WriteFile(path, []byte(sdl), 0o600); err != nil {
		t.Fatalf("write the schema: %v", err)
	}
	return path
}

// runFixture runs the audit over a fixture module against the test schema
// and returns the exit status with both streams.
func runFixture(t *testing.T, packages map[string]string, verbose bool) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:        fixtureModule(t, packages),
		patterns:   []string{"./..."},
		verbose:    verbose,
		schemaPath: schemaFile(t, testSchema),
	}, &out, &errOut)
	return status, out.String(), errOut.String()
}

// soundFixture is every shape the audit accepts, written the way the
// repository writes them: a document assembled from a shared fragment, a
// request assigned to a local first, a document chosen between two constants,
// a document written inline, a fragment spread, an inline fragment on an
// interface, an alias, a field matched case-insensitively, an embedded struct
// and a pointer to one, a json "-" and an unexported field, a type that
// unmarshals itself, one that reads text, a BigInt read into an int64 under
// ",string", an array, a type alias, a map for the data, a subscription, and
// one field the document selects that nothing reads.
const soundFixture = `package sound

import (
	"time"

	"fixture/gql"
)

const projectFields = @@
    id
    gid
    name
@@

const getProject = @@
query($path: ID!) {
  project(fullPath: $path) {@@ + projectFields + @@
    stars
    score
    archived
    createdAt
    size
    meta
    runtime
    severity
    labels { title }
    pipeline { id }
    ... on Project { pipeline { id } }
    __typename
  }
}
@@

const spread = @@
query { project(fullPath: "x") { ...Bits } }
fragment Bits on Project { id NAME: name labels { title } }
@@

const onNode = @@
query { node { id ... on Vulnerability { title } } }
@@

const aliased = @@
query { p: project(fullPath: "x") { id } }
@@

const unread = @@
query { project(fullPath: "x") { id name } }
@@

const ping = @@
subscription { ping }
@@

type tag string

func (t *tag) UnmarshalText(text []byte) error { *t = tag(text); return nil }

type text = string

type flags int

type identity struct {
	ID string @@json:"id"@@
}

type named = struct {
	Name tag @@json:"name"@@
}

type label struct {
	Title string @@json:"title"@@
}

type projectNode struct {
	identity
	*named
	flags
	ID        string          @@json:"id"@@
	GID       string          @@json:"gid"@@
	Stars     int             @@json:"stars"@@
	Score     float64         @@json:"score"@@
	Archived  bool            @@json:"archived"@@
	CreatedAt time.Time       @@json:"createdAt"@@
	Size      int64           @@json:"size,string"@@
	Meta      map[string]any  @@json:"meta"@@
	Runtime   float32         @@json:"runtime"@@
	Severity  severity        @@json:"severity"@@
	Labels    []label         @@json:"labels"@@
	Pipeline  *struct {
		ID string @@json:"id"@@
	} @@json:"pipeline"@@
	Typename text   @@json:"__typename"@@
	Ignored  string @@json:"-"@@
	hidden   string
}

type severity string

type projectResponse struct {
	Data struct {
		Project *projectNode @@json:"project"@@
	} @@json:"data"@@
	Errors []struct{ Message string } @@json:"errors"@@
}

func direct(service gql.Service) {
	var resp projectResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: getProject, Variables: map[string]any{"path": "x"}}, &resp)
}

func viaLocal(service gql.Service) {
	query := gql.GraphQLQuery{Query: getProject}
	var resp projectResponse
	_, _ = service.Do(query, &resp)
}

func viaVar(service gql.Service) {
	var query = gql.GraphQLQuery{Query: getProject}
	var resp projectResponse
	_, _ = service.Do(query, &resp)
}

func chosen(service gql.Service, second bool) {
	doc := aliased
	if second {
		doc = unread
	}
	var resp struct {
		Data map[string]*identity @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: doc}, &resp)
}

func inline(service gql.Service) {
	var resp struct {
		Data map[string]*identity @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: "query { project(fullPath: \"x\") { id } }"}, &resp)
}

func spreadAndNode(service gql.Service) {
	var withSpread struct {
		Data struct {
			Project struct {
				ID     string   @@json:"id"@@
				Name   string   @@json:"name"@@
				Labels [4]label @@json:"labels"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	var withNode struct {
		Data struct {
			Node struct {
				ID    string @@json:"id"@@
				Title string @@json:"title"@@
			} @@json:"node"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: spread}, &withSpread); _, _ = service.Do(gql.GraphQLQuery{Query: onNode}, &withNode)
}

func asMap(service gql.Service) {
	var resp struct {
		Data map[string]any @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: ping}, &resp)
}
`

// wrappedFixture is every hand-over shape: a parameter, a field of a struct
// parameter, a generic wrapper, a generic wrapper called by another generic
// wrapper with two parameters instantiated explicitly, a call through
// parentheses, a call through a function value (which is not followed), a
// call of a method with no package, and an exported wrapper another package
// calls through its selector.
const wrappedFixture = `package wrapped

import (
	"errors"

	"fixture/gql"
)

const createNote = @@
mutation { createNote(input: {body: "x"}) { note { id body } errors } }
@@

const updateNote = @@
mutation { updateNote(input: {body: "x"}) { note { id body } errors } }
@@

const getProject = @@
query { project(fullPath: "x") { id name } }
@@

type note struct {
	ID   string @@json:"id"@@
	Body string @@json:"body"@@
}

type mutation struct {
	Query string
	Key   string
}

type payload[N any] struct {
	Note   *N       @@json:"note"@@
	Errors []string @@json:"errors"@@
}

func SendExported(service gql.Service, query string) { sendDoc(service, query) }

func sendDoc(service gql.Service, query string) {
	var resp struct {
		Data map[string]payload[note] @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: query}, &resp)
}

func exec[N any](service gql.Service, m mutation) *N {
	var resp struct {
		Data map[string]payload[N] @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: m.Query}, &resp)
	return resp.Data[m.Key].Note
}

func decode[N any, E any](service gql.Service, query string) {
	var resp struct {
		Data struct {
			Project *N @@json:"project"@@
		} @@json:"data"@@
		Errors []E @@json:"errors"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: query}, &resp)
}

func listAs[N any](service gql.Service, query string) {
	decode[N, string](service, query)
}

type projectNode struct {
	ID   string @@json:"id"@@
	Name string @@json:"name"@@
}

func callers(service gql.Service) {
	sendDoc(service, createNote)
	(sendDoc)(service, updateNote)
	f := sendDoc
	f(service, createNote)
	_ = exec[note](service, mutation{Query: createNote, Key: "createNote"})
	listAs[projectNode](service, getProject)
	func() {}()
	_ = errors.New("x").Error()
}
`

// callerFixture calls the wrapped package's wrapper through a selector, from
// another package, so the walk over callers crosses a package boundary.
const callerFixture = `package caller

import (
	"fixture/gql"
	"fixture/wrapped"
)

const updateNote = @@
mutation { updateNote(input: {body: "y"}) { note { id body } errors } }
@@

func call(service gql.Service) {
	wrapped.SendExported(service, updateNote)
}
`

// TestRun_FixtureWhereEveryDecoderAgrees_PassesAndListsUnderVerbose verifies
// the passing shape end to end: every accepted way of handing a document to a
// call is paired and judged, the summary names the schema, and -v lists the
// pairings with the one selection nothing reads marked rather than failed.
func TestRun_FixtureWhereEveryDecoderAgrees_PassesAndListsUnderVerbose(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"sound": soundFixture, "wrapped": wrappedFixture, "caller": callerFixture}, true)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s\nstdout:\n%s", status, errOut, out)
	}
	if errOut != "" {
		t.Errorf("run() wrote to stderr on a clean tree:\n%s", errOut)
	}
	for _, want := range []string{
		"    ok  fixture/sound getProject (sound/sound.go:",
		"    ok  fixture/sound an inline document (sound/sound.go:",
		"    ok  fixture/wrapped createNote (wrapped/wrapped.go:",
		", handed over at wrapped/wrapped.go:",
		", handed over at caller/caller.go:",
		"fixture/sound unread (sound/sound.go:",
		"    ~ data.project.name: selected and never decoded",
		"pairing(s) agree with their documents, 1 selection(s) nothing reads (",
		"types from ",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("run() stdout lacks %q:\n%s", want, out)
			}
		})
	}
}

// brokenFixture is one of every disagreement the judge reports, plus one
// selection nothing reads so a failing block shows its notes under -v.
const brokenFixture = `package broken

import (
	"fmt"

	"fixture/gql"
)

const listBroken = @@
query($path: ID!) {
  project(fullPath: $path) {
    name
    stars
    score
    archived
    severity
    labels { title }
    pipeline { id }
    size
    mystery
    meta
  }
}
@@

type shape struct {
	Name     fmt.Stringer   @@json:"name"@@
	Stars    string         @@json:"stars"@@
	Score    int            @@json:"score"@@
	Archived string         @@json:"archived"@@
	Severity int            @@json:"severity"@@
	Labels   struct{}       @@json:"labels"@@
	Pipeline map[int]string @@json:"pipeline"@@
	Size     int64          @@json:"size"@@
	Mystery  string         @@json:"mystery"@@
	Extra    string         @@json:"extra"@@
}

func kinds(service gql.Service) {
	var resp struct {
		Data struct {
			Project string @@json:"project"@@
		} @@json:"data"@@
	}
	var shaped struct {
		Data struct {
			Project *shape @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: listBroken}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: listBroken}, &shaped)
}
`

// TestRun_FixtureWhereDecodersDisagree_NamesEveryDisagreementAndFails
// verifies that each kind of disagreement is reported with the response path
// it is about, under the pairing it belongs to, and fails the run.
func TestRun_FixtureWhereDecodersDisagree_NamesEveryDisagreementAndFails(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"broken": brokenFixture}, true)

	if status != 1 {
		t.Fatalf("run() = %d, want 1; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	if out != "" {
		t.Errorf("run() wrote to stdout with nothing clean to list:\n%s", out)
	}
	for _, want := range []string{
		"fixture/broken listBroken (broken/broken.go:",
		"    - data.project: Project is an object and is decoded into string",
		"    - data.project.name: String! is sent as a JSON string and is decoded into fmt.Stringer",
		"    - data.project.stars: Int! is sent as a JSON integer and is decoded into string",
		"    - data.project.score: Float! is sent as a JSON number that may carry a fraction and is decoded into int",
		"    - data.project.archived: Boolean! is sent as a JSON boolean and is decoded into string",
		"    - data.project.severity: Severity! is sent as a JSON string and is decoded into int",
		"    - data.project.labels: [Label!]! is a list and is decoded into struct{}, which is not a slice",
		"    - data.project.pipeline: Pipeline is an object and is decoded into map[int]string, whose keys are not strings",
		"    - data.project.size: BigInt! is sent as a JSON string and is decoded into int64",
		"    - data.project.mystery: Mystery is a scalar this audit has no serialization for",
		"    - data.project.extra: decoded from a field the document never selects, so it is always empty",
		"    ~ data.project.meta: selected and never decoded",
		"audit_graphql_shapes: 11 disagreement(s) in 2 pairing(s), 0 unpaired or unjudged (",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("run() stderr lacks %q:\n%s", want, errOut)
			}
		})
	}
}

// unpairedFixture is every way a call can fail to be paired with a document,
// each of which is a failure rather than a silence, plus a document nothing
// sends and a decoder typed by a parameter nothing binds.
const unpairedFixture = `package unpaired

import (
	"fmt"
	"strings"

	"fixture/gql"
)

const nobody = @@
query { project(fullPath: "x") { id name } }
@@

const sent = @@
query { project(fullPath: "x") { id } }
@@

const refused = @@
query { project(fullPath: "x") { nonexistent } }
@@

const viaField = @@
query { project(fullPath: "x") { name } }
@@

var runtimeDoc = strings.Repeat("query { project(fullPath: \"x\") { id } }", 1)

type mutation struct {
	Query string
	Key   string
}

type other struct{}

func (other) Do(a, b int) {}

type response struct {
	Data struct {
		Project *struct {
			ID string @@json:"id"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

func build() gql.GraphQLQuery { return gql.GraphQLQuery{} }

func two() (string, int) { return "", 0 }

func pair() (gql.Service, string) { return gql.Service{}, "" }

func shapes(service gql.Service) {
	var resp response
	var plain string
	var noData struct {
		Errors []string @@json:"errors"@@
	}
	m := mutation{Query: viaField}
	doc, _ := two()
	fromCall := build()
	other{}.Do(1, 2)
	_, _ = service.Do(gql.GraphQLQuery{Query: refused}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: fmt.Sprintf("query { %s }", "x")}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: m.Query}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: doc}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: runtimeDoc}, &resp)
	_, _ = service.Do(build(), &resp)
	_, _ = service.Do(fromCall, &resp)
	_, _ = service.Do(gql.GraphQLQuery{sent, nil}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Variables: nil}, &resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: sent}, resp)
	_, _ = service.Do(gql.GraphQLQuery{Query: sent}, &plain)
	_, _ = service.Do(gql.GraphQLQuery{Query: sent}, &noData)
}

func wrapDoc(service gql.Service, query string) {
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: query}, &resp)
}

func wrapField(service gql.Service, m mutation) {
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: m.Query}, &resp)
}

func lonely(service gql.Service, query string) {
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: query}, &resp)
}

func loop(service gql.Service, query string) {
	loop(service, query)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: query}, &resp)
}

func unbound[N any](service gql.Service) {
	var resp struct {
		Data struct {
			Project *N @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: sent}, &resp)
}

func handovers(service gql.Service) {
	m := mutation{Query: viaField}
	wrapDoc(service, fmt.Sprint("x"))
	wrapDoc(pair())
	wrapField(service, m)
	wrapField(service, mutation{Key: "k"})
}
`

// TestRun_FixtureWithUnpairableCalls_ReportsEachAndFails verifies that every
// call the audit cannot pair, every document it cannot find a call for, and
// every wrapper nothing completes is a line on stderr with its reason, that a
// decoder typed by a parameter nothing binds is noted rather than judged, and
// that the run fails on the unpaired alone.
func TestRun_FixtureWithUnpairableCalls_ReportsEachAndFails(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"unpaired": unpairedFixture}, true)

	if status != 1 {
		t.Fatalf("run() = %d, want 1; stderr:\n%s", status, errOut)
	}
	for _, want := range []string{
		"refused is a document the schema refuses, which make check-graphql-documents reports",
		"the document is built at run time, from fmt.Sprintf(",
		"the document is built at run time, from m.Query,",
		"the document is built at run time, from two(),",
		"the document is built at run time, from runtimeDoc,",
		"the request is not a GraphQLQuery literal written at the call or assigned to the variable the call names",
		"the request literal sets no Query field by name",
		"the decode target is not a pointer, so nothing GitLab answers can be written into it",
		"    - data: the decode target is string, not a struct with a data field",
		"    - data: the decode target has no data field, so everything GitLab answers is dropped",
		"hands wrapDoc its document here, and the document is built at run time, from fmt.Sprint(",
		"calls wrapDoc with fewer arguments than the parameter its document arrives through",
		"hands wrapField a m that is not a literal written at the call, so its Query field cannot be read",
		"hands wrapField a literal that sets no Query field by name",
		"lonely receives its document through a parameter and nothing calls it, so no document reaches this decoder",
		"hands loop a document it received through a parameter of its own, 9 hand-overs deep, which is further than this audit follows",
		"nobody is a document no send this audit can see carries, so its decoder is judged by nobody",
		"viaField is a document no send this audit can see carries",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("run() stderr lacks %q:\n%s", want, errOut)
			}
		})
	}
	if !strings.Contains(out, "    ~ data.project: typed by a parameter no caller binds, so it is left unjudged") {
		t.Errorf("run() stdout lacks the unbound-parameter note:\n%s", out)
	}
}

// TestRun_FixtureWithNoSend_FailsRatherThanPassingOnNothing verifies that an
// audit that found nothing to judge is reported as broken, not satisfied.
func TestRun_FixtureWithNoSend_FailsRatherThanPassingOnNothing(t *testing.T) {
	status, _, errOut := runFixture(t, map[string]string{"quiet": "package quiet\n\nfunc nothing() {}\n"}, false)

	if status != 1 {
		t.Fatalf("run() = %d, want 1; stderr:\n%s", status, errOut)
	}
	if !strings.Contains(errOut, "found no send to judge under ./...") {
		t.Errorf("run() stderr = %q, want the empty-run refusal", errOut)
	}
}

// pinnedFixture selects real GitLab types, so the default run, the one CI
// makes, is exercised against the pinned schema. Its title field carries no
// tag, so it is matched by name the way encoding/json matches it.
const pinnedFixture = `package pinned

import "fixture/gql"

const getVulnerability = @@
query($id: VulnerabilityID!) {
  vulnerability(id: $id) {
    id
    title
  }
}
@@

func get(service gql.Service) {
	var resp struct {
		Data struct {
			Vulnerability struct {
				ID    string @@json:"id"@@
				Title string
			} @@json:"vulnerability"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getVulnerability}, &resp)
}
`

// TestRun_WithoutASchemaFlag_JudgesByThePin verifies the default: the pinned
// schema judges, and the summary names its provenance.
func TestRun_WithoutASchemaFlag_JudgesByThePin(t *testing.T) {
	var out, errOut bytes.Buffer
	status := run(auditRun{dir: fixtureModule(t, map[string]string{"pinned": pinnedFixture}), patterns: []string{"./..."}}, &out, &errOut)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stderr:\n%s", status, errOut.String())
	}
	if !strings.Contains(out.String(), "1 pairing(s) agree with their documents, 0 selection(s) nothing reads (") ||
		!strings.Contains(out.String(), "retrieved ") {
		t.Errorf("run() stdout = %q, want the summary naming the pin", out.String())
	}
}

// TestRun_WhenTheRunCannotStart_SaysWhyAndFails verifies each way a run ends
// before judging anything: a schema that cannot be read or parsed, and a
// tree that cannot be loaded.
func TestRun_WhenTheRunCannotStart_SaysWhyAndFails(t *testing.T) {
	cases := []struct {
		name string
		cfg  auditRun
		want string
	}{
		{
			name: "a schema file that does not exist",
			cfg:  auditRun{dir: ".", patterns: []string{"./..."}, schemaPath: filepath.Join(t.TempDir(), "missing.graphql")},
			want: "read the schema to judge against",
		},
		{
			name: "a schema file that does not parse",
			cfg:  auditRun{dir: ".", patterns: []string{"./..."}, schemaPath: schemaFile(t, "type {")},
			want: "parse the schema",
		},
		{
			name: "a directory that is not a module",
			cfg:  auditRun{dir: filepath.Join(t.TempDir(), "nowhere"), patterns: []string{"./..."}, schemaPath: schemaFile(t, testSchema)},
			want: "load packages",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			status := run(testCase.cfg, &out, &errOut)

			if status != 1 {
				t.Errorf("run() = %d, want 1", status)
			}
			if !strings.Contains(errOut.String(), testCase.want) {
				t.Errorf("run() stderr = %q, want it to name %q", errOut.String(), testCase.want)
			}
		})
	}
}

// TestRun_WhenTheDocumentInventoryCannotBeRead_Fails verifies the seam over
// the sibling audit's reader: its one failure is a tree it cannot read, which
// this run reports rather than judging on a list it does not have.
func TestRun_WhenTheDocumentInventoryCannotBeRead_Fails(t *testing.T) {
	original := collectDocuments
	collectDocuments = func(string, []string, map[string][]byte) ([]graphqldocs.Document, error) {
		return nil, errors.New("disk on fire")
	}
	t.Cleanup(func() { collectDocuments = original })

	status, _, errOut := runFixture(t, map[string]string{"pinned": pinnedFixture}, false)

	if status != 1 {
		t.Errorf("run() = %d, want 1", status)
	}
	if !strings.Contains(errOut, "read the documents: disk on fire") {
		t.Errorf("run() stderr = %q, want the reader's failure", errOut)
	}
}

// TestRelative_PositionOutsideTheRoot_IsLeftAbsolute verifies that a position
// the root does not contain is printed as it is rather than as a path
// climbing out of the root.
func TestRelative_PositionOutsideTheRoot_IsLeftAbsolute(t *testing.T) {
	root := t.TempDir()
	inside := token.Position{Filename: filepath.Join(root, "a", "b.go"), Line: 3}
	outside := token.Position{Filename: filepath.Join(t.TempDir(), "c.go"), Line: 7}

	if got := relative(inside, root); got != "a/b.go:3" {
		t.Errorf("relative(inside) = %q, want a/b.go:3", got)
	}
	if got := relative(outside, root); got != outside.Filename+":7" {
		t.Errorf("relative(outside) = %q, want the absolute path", got)
	}
}

// TestAbsolute_WhenTheWorkingDirectoryIsGone_KeepsTheDirAsWritten verifies
// the fallback when the root cannot be made absolute, which is the one case
// filepath.Abs fails in: the working directory no longer exists.
//
// Only Linux can be made to fail that way. Windows refuses to remove a
// process's working directory, and macOS keeps answering getcwd from the
// path it remembers, so on both the premise cannot be set up and the case is
// skipped rather than reported as a defect in the operating system.
func TestAbsolute_WhenTheWorkingDirectoryIsGone_KeepsTheDirAsWritten(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	t.Chdir(gone)
	if err := os.Remove(gone); err != nil {
		t.Skipf("this platform will not remove the working directory, so filepath.Abs cannot be made to fail here: %v", err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform's getcwd still answers after the working directory is removed, so filepath.Abs cannot fail here")
	}

	if got := absolute("."); got != "." {
		t.Errorf("absolute(.) = %q, want it left as written", got)
	}
}
