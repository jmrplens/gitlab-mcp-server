package graphqlintrospect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// answering returns a target configured against a server that replies with
// body for every request.
func answering(t *testing.T, status int, body string) Target {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return Target{Endpoint: server.URL, Client: server.Client()}
}

// tinySchema is the smallest introspection payload that renders to loadable
// SDL: one object type that is also the query root.
const tinySchema = `{"data":{"__schema":{
  "queryType":{"name":"Query"},
  "types":[{"kind":"OBJECT","name":"Query","fields":[{"name":"ok","args":[],"type":{"kind":"SCALAR","name":"Boolean"}}]}]
}}}`

// TestIntrospect_WellFormedAnswer_ReturnsTheSchema verifies the happy path and
// that the token, when there is one, is sent as a bearer credential.
//
// The method and the content type are asserted beside it because they are
// straight-line assignments no gate can be wrong about: GitLab serves GraphQL
// on POST alone and reads the document out of a JSON body, so either of them
// drifting turns every fetch into a refusal the operator has to diagnose from
// an HTTP status.
func TestIntrospect_WellFormedAnswer_ReturnsTheSchema(t *testing.T) {
	var seenAuth, seenQuery, seenMethod, seenContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenMethod = r.Method
		seenContentType = r.Header.Get("Content-Type")
		var body struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		seenQuery = body.Query
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(server.Close)

	schema, err := Introspect(context.Background(), Target{Endpoint: server.URL, Token: "secret", Client: server.Client()})
	if err != nil {
		t.Fatalf("Introspect() error = %v, want nil", err)
	}

	if len(schema.Types) != 1 || schema.QueryType.Name != "Query" {
		t.Errorf("Introspect()returned %+v, want the one-type fixture", schema)
	}
	if seenAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want the bearer credential", seenAuth)
	}
	if seenMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", seenMethod, http.MethodPost)
	}
	if seenContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", seenContentType)
	}
	if !strings.Contains(seenQuery, "__schema") {
		t.Errorf("the query sent does not ask for __schema:\n%s", seenQuery)
	}
}

// TestIntrospect_NoToken_SendsNoAuthorizationHeader verifies that a run with no
// credential sends no Authorization header at all rather than an empty one.
// RFC 6750 requires a token after "Bearer", so "Bearer " alone is a malformed
// credential that an instance, or a proxy in front of it, may refuse, turning
// an introspection GitLab answers to anyone into a 401.
func TestIntrospect_NoToken_SendsNoAuthorizationHeader(t *testing.T) {
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorizations = r.Header.Values("Authorization")
		_, _ = w.Write([]byte(tinySchema))
	}))
	t.Cleanup(server.Close)

	if _, err := Introspect(context.Background(), Target{Endpoint: server.URL, Client: server.Client()}); err != nil {
		t.Fatalf("Introspect() error = %v, want nil", err)
	}

	if len(authorizations) != 0 {
		t.Errorf("an anonymous request carried Authorization %q, want no such header", authorizations)
	}
}

// gitLabShapedServer answers the two documents this package sends the way the
// package documentation says a GitLab instance does: any document asking for
// __schema gets the whole schema whatever else it selects, and the metadata
// query names a version to the bearer of token and null to anybody else.
func gitLabShapedServer(t *testing.T, token string) Target {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("the request body is not a JSON document: %v", err)
			http.Error(w, "not a JSON body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body.Query, "__schema"):
			_, _ = w.Write([]byte(tinySchema))
		case r.Header.Get("Authorization") == "Bearer "+token:
			_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.4.0-pre","revision":"e53e1e5c151"}}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"metadata":null}}`))
		}
	}))
	t.Cleanup(server.Close)
	return Target{Endpoint: server.URL, Client: server.Client()}
}

// TestInstanceVersion_AskedOfAGitLabShapedServer_SendsTheMetadataQuery verifies
// that each entry point sends its own document. Every other test here answers
// the same body whatever is asked, so the two constants could trade places
// between [Introspect] and [InstanceVersion] and nothing would notice; against
// a real instance the version would then come back as a schema, which decodes
// to no metadata and is recorded as unknown on every run, token or not.
func TestInstanceVersion_AskedOfAGitLabShapedServer_SendsTheMetadataQuery(t *testing.T) {
	const token = "glpat-secret"
	target := gitLabShapedServer(t, token)
	target.Token = token

	schema, err := Introspect(context.Background(), target)
	if err != nil {
		t.Fatalf("Introspect() error = %v, want the schema", err)
	}
	version, revision := InstanceVersion(context.Background(), target)

	if len(schema.Types) != 1 || schema.Types[0].Name != "Query" {
		t.Errorf("Introspect() returned %+v, want the one-type fixture", schema)
	}
	if version != "19.4.0-pre" || revision != "e53e1e5c151" {
		t.Errorf("InstanceVersion() = (%q, %q), want (\"19.4.0-pre\", \"e53e1e5c151\")", version, revision)
	}
}

// selections walks a validated document and returns, for every field it
// selects, the arguments written on each occurrence of that field, rendered as
// "name: value" and joined. Operations and fragments are walked each on their
// own and spreads are not followed, so a field inside a fragment is counted
// once however many spreads reach it.
func selections(document *ast.QueryDocument) map[string][]string {
	found := map[string][]string{}
	var walk func(ast.SelectionSet)
	walk = func(set ast.SelectionSet) {
		for _, selection := range set {
			switch node := selection.(type) {
			case *ast.Field:
				arguments := make([]string, 0, len(node.Arguments))
				for _, argument := range node.Arguments {
					arguments = append(arguments, argument.Name+": "+argument.Value.String())
				}
				found[node.Name] = append(found[node.Name], strings.Join(arguments, ", "))
				walk(node.SelectionSet)
			case *ast.InlineFragment:
				walk(node.SelectionSet)
			}
		}
	}
	for _, operation := range document.Operations {
		walk(operation.SelectionSet)
	}
	for _, fragment := range document.Fragments {
		walk(fragment.SelectionSet)
	}
	return found
}

// elementOf is the type a decoder member holds one of, looking through the
// pointers and slices encoding/json looks through.
func elementOf(decoded reflect.Type) reflect.Type {
	for decoded.Kind() == reflect.Pointer || decoded.Kind() == reflect.Slice {
		decoded = decoded.Elem()
	}
	return decoded
}

// selectedByKey gathers the fields set selects, keyed by the JSON member each
// is answered under: its alias, which gqlparser sets to the field's name when
// none is written. Inline fragments and fragment spreads are followed into the
// set they contribute to, and a key selected more than once keeps every
// occurrence, since GraphQL merges their sub-selections into one member.
func selectedByKey(set ast.SelectionSet, into map[string][]*ast.Field) {
	for _, selection := range set {
		switch node := selection.(type) {
		case *ast.Field:
			into[node.Alias] = append(into[node.Alias], node)
		case *ast.InlineFragment:
			selectedByKey(node.SelectionSet, into)
		case *ast.FragmentSpread:
			selectedByKey(node.Definition.SelectionSet, into)
		}
	}
}

// unselectedMembers walks the decoder rooted at decoded and the selection set
// of operation side by side, and returns the path of every member the decoder
// reads that the document does not select at that position.
//
// A decoder that nests itself, as TypeRef does through ofType, reads deeper
// than any document can select, so the recursive member is excused once the
// chain already holds floors[type] nodes of that type. A recursive type with
// no floor is held like any other member.
func unselectedMembers(operation *ast.OperationDefinition, decoded reflect.Type, floors map[reflect.Type]int) []string {
	var missing []string
	onPath := map[reflect.Type]int{}
	var walk func(current reflect.Type, sets []ast.SelectionSet, path string)
	walk = func(current reflect.Type, sets []ast.SelectionSet, path string) {
		current = elementOf(current)
		if current.Kind() != reflect.Struct {
			return
		}
		onPath[current]++
		defer func() { onPath[current]-- }()

		selected := map[string][]*ast.Field{}
		for _, set := range sets {
			selectedByKey(set, selected)
		}
		for member := range current.Fields() {
			key, _, _ := strings.Cut(member.Tag.Get("json"), ",")
			at := strings.TrimPrefix(path+"."+key, ".")
			fields, ok := selected[key]
			if !ok {
				nested := elementOf(member.Type)
				if floor, recursive := floors[nested]; recursive && onPath[nested] >= floor {
					continue
				}
				missing = append(missing, at)
				continue
			}
			nestedSets := make([]ast.SelectionSet, 0, len(fields))
			for _, field := range fields {
				nestedSets = append(nestedSets, field.SelectionSet)
			}
			walk(member.Type, nestedSets, at)
		}
	}
	walk(decoded, []ast.SelectionSet{operation.SelectionSet}, "")
	return missing
}

// typeReferenceNodes is how many TypeRef nodes an introspection answer takes
// to spell reference: one per NON_NULL and LIST wrapper, and one for the named
// type inside them.
func typeReferenceNodes(reference *ast.Type) int {
	nodes := 1
	if reference.NonNull {
		nodes++
	}
	if reference.Elem != nil {
		return nodes + typeReferenceNodes(reference.Elem)
	}
	return nodes
}

// deepestTypeReference is the most TypeRef nodes any field, argument or input
// field type in schema takes to spell, which is how deep the ofType chain has
// to reach for the pin to carry every one of them whole.
func deepestTypeReference(schema *ast.Schema) int {
	deepest := 0
	for _, definition := range schema.Types {
		for _, field := range definition.Fields {
			deepest = max(deepest, typeReferenceNodes(field.Type))
			for _, argument := range field.Arguments {
				deepest = max(deepest, typeReferenceNodes(argument.Type))
			}
		}
	}
	return deepest
}

// TestQueries_EachDocument_IsAcceptedAndSelectsWhatItsDecoderReads holds the
// two documents this package sends to the only schema anything here can judge
// them by. They live under cmd/, which `make check-graphql-documents` does not
// read, and every mock in this file answers whatever it is asked, so a
// misspelled member or a dropped selection would otherwise reach GitLab first.
//
// Accepted is half of it. A member the decoder reads that the document never
// selects is empty on every answer from an instance that honors the selection
// set; GitLab does not today, which is exactly why no run would notice. The
// question is asked where the decoder reads the member, walking the decoder and
// the selection set together, because the introspection query selects kind,
// name and type at several depths: one of them dropped is still selected
// somewhere else, and a comparison of names across the whole document cannot
// see that it went.
//
// The ofType chain is the one place a decoder reads deeper than a document can
// select, since TypeRef nests itself and the document has to stop. It is held
// to the deepest type reference the pinned schema spells instead: a chain
// shorter than that loses the innermost type of the deepest references GitLab
// serves.
func TestQueries_EachDocument_IsAcceptedAndSelectsWhatItsDecoderReads(t *testing.T) {
	schema, err := graphqlschema.Schema()
	if err != nil {
		t.Fatalf("load the pinned schema: %v", err)
	}
	floors := map[reflect.Type]int{reflect.TypeFor[TypeRef](): deepestTypeReference(schema)}
	cases := []struct {
		name     string
		document string
		decoder  reflect.Type
	}{
		{name: "the introspection query", document: introspectionQuery, decoder: reflect.TypeFor[introspectData]()},
		{name: "the metadata query", document: metadataQuery, decoder: reflect.TypeFor[metadataData]()},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			parsed, parseErr := graphqlschema.ParseAgainst(schema, testCase.document)
			if parseErr != nil {
				t.Fatalf("the pinned schema refuses %s: %v", testCase.name, parseErr)
			}
			if len(parsed.Operations) != 1 {
				t.Fatalf("%s holds %d operations, want the one its decoder reads", testCase.name, len(parsed.Operations))
			}
			for _, path := range unselectedMembers(parsed.Operations[0], testCase.decoder, floors) {
				t.Errorf("the decoder reads %s and %s does not select it there", path, testCase.name)
			}
		})
	}
}

// TestIntrospectionQuery_DeprecatedMembers_AreAskedForWhereEveryInstanceAccepts
// holds both halves of the trade the query's comment describes. Fields and
// enum values ask for their deprecated members, because a deprecated field is
// one GitLab still serves and our documents select; arguments and input fields
// do not, because an older self-managed instance refuses the argument there and
// with it the whole introspection. Either half reversed still validates, so
// only the arguments themselves can say which way it was written.
func TestIntrospectionQuery_DeprecatedMembers_AreAskedForWhereEveryInstanceAccepts(t *testing.T) {
	schema, err := graphqlschema.Schema()
	if err != nil {
		t.Fatalf("load the pinned schema: %v", err)
	}
	parsed, err := graphqlschema.ParseAgainst(schema, introspectionQuery)
	if err != nil {
		t.Fatalf("the pinned schema refuses the introspection query: %v", err)
	}
	selected := selections(parsed)

	cases := []struct {
		name  string
		field string
		want  []string
	}{
		{name: "fields include the deprecated ones", field: "fields", want: []string{"includeDeprecated: true"}},
		{name: "enum values include the deprecated ones", field: "enumValues", want: []string{"includeDeprecated: true"}},
		{name: "arguments ask for nothing an old instance refuses", field: "args", want: []string{""}},
		{name: "input fields ask for nothing an old instance refuses", field: "inputFields", want: []string{""}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := selected[testCase.field]; !slices.Equal(got, testCase.want) {
				t.Errorf("%s is selected with arguments %q, want %q", testCase.field, got, testCase.want)
			}
		})
	}
}

// gitLabShapedIntrospection is one introspection answer written the way an
// instance writes one: every member spelled as GraphQL's own __schema spells
// it, every kind in the specification's upper snake case, and every list in an
// order the renderer is supposed to discard. The five scalars the prelude
// already defines and one introspection type are in it because an instance
// sends those too.
const gitLabShapedIntrospection = `{"data":{"__schema":{
  "queryType":{"name":"Query"},
  "mutationType":{"name":"Mutation"},
  "subscriptionType":{"name":"Subscription"},
  "types":[
    {"kind":"SCALAR","name":"Time"},
    {"kind":"SCALAR","name":"ID"},
    {"kind":"SCALAR","name":"Boolean"},
    {"kind":"SCALAR","name":"String"},
    {"kind":"SCALAR","name":"Int"},
    {"kind":"SCALAR","name":"Float"},
    {"kind":"OBJECT","name":"__Type","fields":[
      {"name":"name","type":{"kind":"SCALAR","name":"String"}}]},
    {"kind":"UNION","name":"Anything","possibleTypes":[{"name":"Thing"},{"name":"Query"}]},
    {"kind":"ENUM","name":"Mood","enumValues":[{"name":"GOOD"},{"name":"BAD"}]},
    {"kind":"INPUT_OBJECT","name":"TouchInput","inputFields":[
      {"name":"note","type":{"kind":"SCALAR","name":"String"},"defaultValue":"\"none\""},
      {"name":"id","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}},"defaultValue":null}]},
    {"kind":"INPUT_OBJECT","name":"Window","inputFields":[
      {"name":"after","type":{"kind":"SCALAR","name":"Time"}},
      {"name":"before","type":{"kind":"SCALAR","name":"Time"}}]},
    {"kind":"OBJECT","name":"Subscription","fields":[
      {"name":"tick","args":[],"type":{"kind":"SCALAR","name":"Time"}}]},
    {"kind":"OBJECT","name":"Mutation","fields":[
      {"name":"touch","args":[
        {"name":"after","type":{"kind":"SCALAR","name":"Time"}},
        {"name":"input","type":{"kind":"NON_NULL","ofType":{"kind":"INPUT_OBJECT","name":"TouchInput"}}}],
       "type":{"kind":"SCALAR","name":"Boolean"}}]},
    {"kind":"OBJECT","name":"Thing","interfaces":[{"name":"Node"}],"fields":[
      {"name":"id","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}},
      {"name":"name","type":{"kind":"SCALAR","name":"String"}}]},
    {"kind":"INTERFACE","name":"Node","possibleTypes":[{"name":"Thing"},{"name":"Query"}],"fields":[
      {"name":"id","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}}]},
    {"kind":"OBJECT","name":"Query","interfaces":[{"name":"Node"}],"fields":[
      {"name":"things","args":[
        {"name":"mood","type":{"kind":"ENUM","name":"Mood"},"defaultValue":"GOOD"},
        {"name":"during","type":{"kind":"INPUT_OBJECT","name":"Window"}}],
       "type":{"kind":"LIST","ofType":{"kind":"NON_NULL","ofType":{"kind":"OBJECT","name":"Thing"}}}},
      {"name":"id","type":{"kind":"NON_NULL","ofType":{"kind":"SCALAR","name":"ID"}}}]}
  ]}}}`

// wantGitLabShapedSDL is the whole of what gitLabShapedIntrospection has to
// render to: every nameable thing sorted, the prelude's scalars and the
// introspection type left out, and one blank line between blocks.
const wantGitLabShapedSDL = `union Anything = Query | Thing

enum Mood {
  BAD
  GOOD
}

type Mutation {
  touch(after: Time, input: TouchInput!): Boolean
}

interface Node {
  id: ID!
}

type Query implements Node {
  id: ID!
  things(during: Window, mood: Mood = GOOD): [Thing!]
}

type Subscription {
  tick: Time
}

type Thing implements Node {
  id: ID!
  name: String
}

scalar Time

input TouchInput {
  id: ID!
  note: String = "none"
}

input Window {
  after: Time
  before: Time
}

schema {
  query: Query
  mutation: Mutation
  subscription: Subscription
}
`

// TestIntrospect_AGitLabShapedAnswer_RendersTheWholeSchema holds the part of
// this package neither coverage nor a flipped operator can reach: the JSON
// tags binding these structs to GraphQL's own __schema spelling, and the kind
// constants the renderer switches on. Both are data rather than branches, so a
// tag that drifts to snake case, a pair of tags that trade places, or a kind
// spelled wrong drops a whole dimension of the pin without failing anything:
// defaults, type wrappers, implemented interfaces and union members all arrive
// empty, and every gate reading that pin keeps passing on a schema that
// promises less than GitLab serves.
//
// The fixture therefore travels the real route, from an instance's bytes
// through [Introspect] to the SDL, and the rendering is compared whole: the
// sort order is the other thing a fragment cannot hold, and the round trip
// through gqlparser is what says the result is still a schema.
func TestIntrospect_AGitLabShapedAnswer_RendersTheWholeSchema(t *testing.T) {
	schema, err := Introspect(context.Background(), answering(t, http.StatusOK, gitLabShapedIntrospection))
	if err != nil {
		t.Fatalf("Introspect() error = %v, want nil", err)
	}

	sdl := RenderSDL(schema)

	if sdl != wantGitLabShapedSDL {
		t.Errorf("RenderSDL() rendered\n%s\nwant\n%s", sdl, wantGitLabShapedSDL)
	}
	if _, loadErr := graphqlschema.Load([]byte(sdl)); loadErr != nil {
		t.Fatalf("the rendered SDL does not load:\n%v\n\n%s", loadErr, sdl)
	}
}

// TestFetchTimeout_BoundsAWholeFetchWithRoomToSpare pins the one figure both
// commands hand to an http.Client and to context.WithTimeout. It is held to a
// range rather than to its literal because what has to be true of it is that
// the bound is generous enough for an instance that takes minutes to produce
// tens of megabytes of JSON, and still short enough to end a fetch nobody is
// going to answer. Nothing inside this package reads the constant, so a value
// that collapsed towards zero, canceling every introspection before it began,
// would otherwise fail no test here and only surface as an empty pin.
func TestFetchTimeout_BoundsAWholeFetchWithRoomToSpare(t *testing.T) {
	if FetchTimeout < time.Minute {
		t.Errorf("FetchTimeout = %v, too short for an instance that answers in minutes", FetchTimeout)
	}
	if FetchTimeout > time.Hour {
		t.Errorf("FetchTimeout = %v, too long to end a fetch that will never be answered", FetchTimeout)
	}
}

// TestIntrospect_AnswersThatCarryNoSchema_AreRefused verifies that a reply
// which decodes but says nothing is reported rather than written out. An empty
// artifact would parse, load, and then accept every broken document silently,
// which is the one failure this whole gate must not have.
//
// A refusal that says the instance answered also names which instance, and
// what it answered when that was not a GraphQL reply, because the operator
// chose the endpoint and has to tell a deploy page from a wrong URL.
func TestIntrospect_AnswersThatCarryNoSchema_AreRefused(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		body         string
		want         string
		wantEndpoint bool
	}{
		{name: "no __schema member", status: http.StatusOK, body: `{"data":{}}`, want: "answered introspection with no types", wantEndpoint: true},
		{name: "a schema with no types", status: http.StatusOK, body: `{"data":{"__schema":{"types":[]}}}`, want: "answered introspection with no types", wantEndpoint: true},
		{name: "a data member that is not an object", status: http.StatusOK, body: `{"data":[1,2,3]}`, want: "decode the introspection payload"},
		{name: "an instance that refuses the round trip", status: http.StatusServiceUnavailable, body: "deploying", want: "answered 503 Service Unavailable: deploying", wantEndpoint: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			target := answering(t, testCase.status, testCase.body)
			schema, err := Introspect(context.Background(), target)

			if err == nil {
				t.Fatalf("Introspect() error = nil, want one naming %q", testCase.want)
			}
			if schema != nil {
				t.Errorf("Introspect() schema = %+v, want nil on failure", schema)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Introspect() error = %q, want it to name %q", err, testCase.want)
			}
			if testCase.wantEndpoint && !strings.Contains(err.Error(), target.Endpoint) {
				t.Errorf("Introspect() error = %q, want it to name the endpoint %s", err, target.Endpoint)
			}
		})
	}
}

// TestTruncatedAnswer_CountsAroundTheFloor_AreJudgedTheSameWay verifies the one
// judgement the pin check and the live re-probe share. Both refuse to work from
// an answer too short to be a GitLab schema, and they must refuse at the same
// count: a floor either of them could lower on its own would let that side keep
// promising something the other had already stopped promising.
//
// The half-size case holds the floor to its purpose rather than to itself: a
// floor moved towards zero keeps every case relative to [MinimumTypes] green
// while accepting an answer that lost half of GitLab.
func TestTruncatedAnswer_CountsAroundTheFloor_AreJudgedTheSameWay(t *testing.T) {
	cases := []struct {
		name  string
		types int
		want  bool
	}{
		{name: "an empty answer", types: 0, want: true},
		{name: "half of the smallest GitLab schema seen", types: 4233 / 2, want: true},
		{name: "a Community Edition-sized answer", types: MinimumTypes - 1, want: true},
		{name: "exactly the floor", types: MinimumTypes, want: false},
		{name: "an unlicensed Enterprise Edition instance", types: 4233, want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := TruncatedAnswer(testCase.types); got != testCase.want {
				t.Errorf("TruncatedAnswer(%d) = %v, want %v", testCase.types, got, testCase.want)
			}
		})
	}
}

// refusingTransport fails every round trip the way a port nobody listens on
// does, without depending on a port staying free.
type refusingTransport struct{}

// RoundTrip refuses the request before anything is sent.
func (refusingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connect: connection refused")
}

// TestPost_TransportAndProtocolFailures_AreNamed verifies that each way one
// GraphQL round trip can fail says what happened and which endpoint it was
// asking, since the operator running this command chose that endpoint. A
// status that is not 200 also carries the start of the body beside it, which
// is the only part of the answer that tells a gateway page from a refusal.
//
// Each want is the beginning of the message as this package writes it, with %s
// where it names the endpoint, rather than a phrase plus a search for the
// endpoint anywhere in the message. Two of these refusals wrap an error from
// net/url or net/http, and both of those quote the URL on their own account
// (`parse "://not a url": ...`, `Post "http://gitlab.invalid/api/graphql":
// ...`), so a search of the whole message finds the endpoint whether or not
// this package named it.
//
// Nothing listening is a transport that refuses every round trip rather than a
// closed httptest server: the port a closed listener frees can be handed to
// the next server this test opens, or to a parallel mutation run's, and the
// case then reads that server's answer instead of a refused connection.
func TestPost_TransportAndProtocolFailures_AreNamed(t *testing.T) {
	truncating := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A body shorter than the length announced makes the client's read
		// fail after the status line has already been accepted.
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write([]byte("{"))
	}))
	t.Cleanup(truncating.Close)

	cases := []struct {
		name   string
		target Target
		// want is a prefix of the error, with %s standing for the endpoint.
		want string
	}{
		{
			name:   "an endpoint that is not a URL",
			target: Target{Endpoint: "://not a url", Client: http.DefaultClient},
			want:   "build the request for %s: ",
		},
		{
			name:   "nothing listening",
			target: Target{Endpoint: "http://gitlab.invalid/api/graphql", Client: &http.Client{Transport: refusingTransport{}}},
			want:   "ask %s: ",
		},
		{
			name:   "a body that ends early",
			target: Target{Endpoint: truncating.URL, Client: truncating.Client()},
			want:   "read the answer from %s: ",
		},
		{
			name:   "a status that is not 200",
			target: answering(t, http.StatusBadGateway, "<html>gateway</html>"),
			want:   "%s answered 502 Bad Gateway: <html>gateway</html>",
		},
		{
			name:   "a body that is not JSON",
			target: answering(t, http.StatusOK, "definitely not json"),
			want:   "decode the answer from %s: ",
		},
		{
			name:   "an errors array",
			target: answering(t, http.StatusOK, `{"errors":[{"message":"field not found"},{"message":"and another"}]}`),
			want:   "%s refused the query: field not found; and another",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			want := fmt.Sprintf(testCase.want, testCase.target.Endpoint)

			raw, err := post(context.Background(), testCase.target, "query { ok }")

			if err == nil {
				t.Fatalf("post() error = nil, want one beginning %q", want)
			}
			if raw != nil {
				t.Errorf("post() data = %s, want nil on failure", raw)
			}
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("post() error = %q, want it to begin %q", err, want)
			}
		})
	}
}

// TestInstanceVersion_WhateverTheInstanceSays_NeverStopsTheRun verifies that
// provenance is best effort. GitLab answers the metadata query with null to an
// anonymous caller, and the schema is what the command exists to fetch, so a
// version nobody would tell us is recorded as unknown rather than fatal.
func TestInstanceVersion_WhateverTheInstanceSays_NeverStopsTheRun(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		status       int
		wantVersion  string
		wantRevision string
	}{
		{
			name:         "a version and a revision",
			body:         `{"data":{"metadata":{"version":"19.4.0-pre","revision":"e53e1e5c151"}}}`,
			status:       http.StatusOK,
			wantVersion:  "19.4.0-pre",
			wantRevision: "e53e1e5c151",
		},
		{name: "null, which is what an anonymous caller gets", body: `{"data":{"metadata":null}}`, status: http.StatusOK, wantVersion: UnknownVersion},
		{name: "a data member of the wrong shape", body: `{"data":"nope"}`, status: http.StatusOK, wantVersion: UnknownVersion},
		{name: "a refusal", body: `{"errors":[{"message":"no"}]}`, status: http.StatusOK, wantVersion: UnknownVersion},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			version, revision := InstanceVersion(context.Background(), answering(t, testCase.status, testCase.body))

			if version != testCase.wantVersion {
				t.Errorf("version = %q, want %q", version, testCase.wantVersion)
			}
			if revision != testCase.wantRevision {
				t.Errorf("revision = %q, want %q", revision, testCase.wantRevision)
			}
		})
	}
}

// TestSnippet_LongBody_IsShortened verifies that an HTML error page does not
// take the whole terminal with it when an instance answers one.
//
// A body of exactly the limit is one of the cases because the two sides do not
// agree there: shortening it would append an ellipsis promising a remainder
// that does not exist, so the boundary decides whether an error line can lie
// about how much it dropped.
func TestSnippet_LongBody_IsShortened(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "short enough to show whole", payload: "  gateway timeout  ", want: "gateway timeout"},
		{name: "exactly the limit", payload: strings.Repeat("x", 200), want: strings.Repeat("x", 200)},
		{name: "longer than the limit", payload: strings.Repeat("x", 260), want: strings.Repeat("x", 200) + "..."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := snippet([]byte(testCase.payload)); got != testCase.want {
				t.Errorf("snippet() = %q, want %q", got, testCase.want)
			}
		})
	}
}
