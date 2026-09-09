package paths

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// response is what a test says one endpoint answers with, written the way a
// reader of the test wants to see it rather than the way the record stores it.
//
// The record keeps the answer in an entity and the endpoint in a route, and a
// fixture that spelled both would say the same thing twice; recordIn builds the
// pair, inventing an entity name where a test does not need one.
type response struct {
	// Response is the keys the endpoint sends. An endpoint with none is one
	// the record gives no entity, which is a case in its own right.
	Response []string
	// Nested is, per key carrying an object, the keys that object sends.
	Nested map[string][]string
	// Entity names the entity, for a test whose subject is the name itself:
	// a declaration keyed on it, or two routes that must share one.
	Entity string
	// Conditions gate a field of this response, by field name. A field with
	// none is one GitLab always sends.
	Conditions map[string][]apilive.Condition
}

// recordIn writes a live GitLab record a test can join against, from a map of
// endpoints to what each answers with.
//
// The key is "METHOD /path" in either spelling a test finds natural, with or
// without the /api/v4 prefix a caller writes; the record's own mount prefix is
// what actually goes on disk.
func recordIn(t *testing.T, operations map[string]response) string {
	t.Helper()
	root := t.TempDir()
	if err := apilive.Write(filepath.Join(root, apilive.DefaultDir), fixtureDocument(operations)); err != nil {
		t.Fatalf("prepare the record: %v", err)
	}
	return root
}

// fixtureDocument builds the record a fixture describes, in memory, so a test
// that only needs the two views of it never writes a file.
func fixtureDocument(operations map[string]response) apilive.Document {
	doc := apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Source:        apilive.Source{Image: "gitlab/gitlab-ee:test", Version: "19.3.1-ee", RetrievedAt: "2026-09-09", SHA256: "abc"},
		Entities:      map[string]apilive.Entity{},
		Features:      map[string]string{"repository_mirrors": apilive.TierPremium},
	}
	for key, answer := range operations {
		method, path, found := strings.Cut(key, " ")
		if !found {
			continue
		}
		route := apilive.Route{Method: method, Path: apilive.EndpointPrefix + trimAPIPrefix(path)}
		if len(answer.Response) > 0 || answer.Entity != "" {
			route.Entity = answer.Entity
			if route.Entity == "" {
				route.Entity = "API::Entities::" + entityFixtureName(key)
			}
			doc.Entities[route.Entity] = fixtureEntity(route.Entity, answer, doc.Entities)
		}
		doc.Routes = append(doc.Routes, route)
	}
	sort.Slice(doc.Routes, func(i, j int) bool {
		if doc.Routes[i].Path != doc.Routes[j].Path {
			return doc.Routes[i].Path < doc.Routes[j].Path
		}
		return doc.Routes[i].Method < doc.Routes[j].Method
	})
	doc.Source.Entities = len(doc.Entities)
	doc.Source.Routes = len(doc.Routes)
	doc.Source.Fields = doc.FieldCount()
	doc.Source.Features = len(doc.Features)
	return doc
}

// fixtureRecord is the two views of one fixture the checks take: the endpoint
// index and the conditions, both read off the same entities, which is the
// property the port bought and a test should not be able to break.
func fixtureRecord(operations map[string]response) (*operationIndex, *conditionIndex) {
	doc := fixtureDocument(operations)
	return newOperationIndex(doc), newConditionIndex(doc)
}

// typedCheckOf runs the type grain against a fixture.
func typedCheckOf(root string, operations map[string]response, published []publishedType) TypedShapeCheck {
	index, conditions := fixtureRecord(operations)
	return typedShapeCheck(root, index, conditions, published)
}

// fixtureEntity turns one fixture answer into the entity that renders it,
// writing a child entity for each key carrying an object.
func fixtureEntity(parent string, answer response, entities map[string]apilive.Entity) apilive.Entity {
	entity := apilive.Entity{}
	for _, name := range answer.Response {
		field := apilive.Field{Name: name, Conditions: answer.Conditions[name]}
		if under, nested := answer.Nested[name]; nested {
			// Named after the parent as well as the property: two endpoints
			// that share a shape each write their own child, and a name from
			// the property alone would have the second overwrite the first,
			// which is the very union the shape index is being tested for.
			child := parent + "::" + strings.ToUpper(name[:1]) + name[1:] + "Fixture"
			fields := make([]apilive.Field, 0, len(under))
			for _, key := range under {
				fields = append(fields, apilive.Field{Name: key})
			}
			entities[child] = apilive.Entity{Fields: fields}
			field.Using = child
		}
		entity.Fields = append(entity.Fields, field)
	}
	return entity
}

// entityFixtureName invents a stable entity name from an endpoint key, so a
// fixture that does not care what the entity is called does not have to say.
func entityFixtureName(key string) string {
	var out strings.Builder
	upper := true
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			if upper {
				out.WriteString(strings.ToUpper(string(r)))
				upper = false
				continue
			}
			out.WriteRune(r)
		default:
			upper = true
		}
	}
	return out.String()
}

// trimAPIPrefix accepts a fixture path written the way GitLab's documentation
// spells it, with the mount prefix and braced placeholders, and returns the
// path Grape holds.
//
// The conversion is a fixture convenience and lives only here: the record's own
// routes already spell a placeholder `:id`, which is why the production reader
// has no such translation to do.
func trimAPIPrefix(path string) string {
	for _, prefix := range []string{"/api/v4", "/api"} {
		if trimmed, found := strings.CutPrefix(path, prefix); found {
			path = trimmed
			break
		}
	}
	if path == "" {
		return "/"
	}
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			out.WriteByte(path[i])
			continue
		}
		end := strings.IndexByte(path[i:], '}')
		if end < 0 {
			out.WriteString(path[i:])
			break
		}
		out.WriteByte(':')
		out.WriteString(path[i+1 : i+end])
		i += end
	}
	return out.String()
}

// TestShapeCheck_JoinsOurSpellingToGitLabs verifies the three ways an endpoint
// can be looked up, because the join decides everything downstream: a row that
// does not match contributes no names, and a package whose rows all miss would
// have every field of its output reported as one GitLab does not send.
func TestShapeCheck_JoinsOurSpellingToGitLabs(t *testing.T) {
	root := recordIn(t, map[string]response{
		"GET /api/v4/projects/{id}/issues/{issue_iid}": {Response: []string{"iid", "state", "title"}},
		"GET /api/v4/version":                          {Response: []string{"revision", "version"}},
	})

	cases := []struct {
		name    string
		row     requestinventory.Row
		quality string
		literal string
	}{
		{
			name:    "the same path with the same placeholder names",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/version"},
			quality: "exact",
		},
		{
			name:    "our placeholder names differ from GitLab's",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/:project_id/issues/:issue_id"},
			quality: "exact",
		},
		{
			name:    "a fixture value stands where an identifier belongs",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/myproject/issues/:issue_id"},
			quality: "loose",
			literal: "myproject",
		},
		{
			name:    "an endpoint the document does not carry",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/:project_id/nowhere"},
			quality: "unmatched",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			check := shapeCheck(root, []requestinventory.Row{testCase.row}, nil)

			if !check.Ran {
				t.Fatal("shapeCheck() did not run against a record it should have read")
			}
			got := map[string]int{"exact": check.Join.Exact, "loose": check.Join.Loose, "unmatched": check.Join.Unmatched}
			if got[testCase.quality] != 1 {
				t.Errorf("join = %+v, want one %s match", check.Join, testCase.quality)
			}
			switch {
			case testCase.literal == "" && len(check.Untemplated) > 0:
				t.Errorf("reported %q as untemplated for a match that needed no literal", check.Untemplated[0].Segment)
			case testCase.literal != "" && (len(check.Untemplated) != 1 || check.Untemplated[0].Segment != testCase.literal):
				t.Errorf("untemplated = %+v, want the one segment %q", check.Untemplated, testCase.literal)
			}
		})
	}
}

// TestShapeCheck_APublishedFieldNoEndpointSends_IsReported verifies the finding
// this whole comparison exists for, which is
// https://github.com/jmrplens/gitlab-mcp-server/issues/580 in miniature: a type
// publishing fields GitLab's own document does not list for any endpoint the
// package calls.
func TestShapeCheck_APublishedFieldNoEndpointSends_IsReported(t *testing.T) {
	root := recordIn(t, map[string]response{
		"GET /api/v4/projects/{id}/merge_requests/{iid}/approvals": {
			Response: []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
		},
	})
	rows := []requestinventory.Row{{
		Package: "internal/tools/mrapprovals", Kind: "rest", Method: "GET",
		Path: "/projects/:project_id/merge_requests/:merge_request_id/approvals",
	}}
	published := []publishedType{{
		Package: "internal/tools/mrapprovals",
		Name:    "ConfigOutput",
		Fields:  []string{"approved", "approved_by", "id", "state", "title"},
	}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 3 {
		t.Fatalf("reported %d field(s), want the three GitLab does not send: %+v", len(check.Unpublished), check.Unpublished)
	}
	var names []string
	for _, field := range check.Unpublished {
		names = append(names, field.Field)
		if field.Endpoints != 1 {
			t.Errorf("%s says %d endpoints were searched, want 1", field.Field, field.Endpoints)
		}
	}
	if strings.Join(names, ",") != "id,state,title" {
		t.Errorf("unpublished = %v, want id, state and title", names)
	}
}

// TestShapeCheck_APackageWhoseEndpointsDeclareNothing_IsNotJudged verifies the
// silence that keeps the check honest. GitLab's document names no response
// schema for 553 of its operations, and an empty union means it does not say
// rather than that it sends nothing: reporting against one would call every
// field of that package's output a field GitLab never sends.
func TestShapeCheck_APackageWhoseEndpointsDeclareNothing_IsNotJudged(t *testing.T) {
	root := recordIn(t, map[string]response{
		"DELETE /api/v4/projects/{id}/silent": {},
	})
	rows := []requestinventory.Row{{Package: "internal/tools/quiet", Kind: "rest", Method: "DELETE", Path: "/projects/:project_id/silent"}}
	published := []publishedType{{Package: "internal/tools/quiet", Name: "Output", Fields: []string{"anything"}}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 0 {
		t.Errorf("reported %+v against an endpoint whose response the document does not describe", check.Unpublished)
	}
}

// TestShapeCheck_AnEndpointWithNoDescribedResponse_IsNotCountedAsSearched
// verifies that the count a finding carries means one thing at both grains. An
// operation the document leaves without a response contributes no names to the
// union, so counting it would say the field was looked for in two responses
// where the record holds one, and a reader weighing the finding would credit it
// with evidence that was never there.
func TestShapeCheck_AnEndpointWithNoDescribedResponse_IsNotCountedAsSearched(t *testing.T) {
	root := recordIn(t, map[string]response{
		"GET /api/v4/projects/{id}/thing":    {Response: []string{"described"}},
		"DELETE /api/v4/projects/{id}/thing": {},
	})
	rows := []requestinventory.Row{
		{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/:project_id/thing"},
		{Package: "p", Kind: "rest", Method: "DELETE", Path: "/projects/:project_id/thing"},
	}
	published := []publishedType{
		{Package: "p", Name: "Output", Fields: []string{"described", "invented"}},
		// An inner type is nobody's response and is held to none.
		{Package: "p", Name: "RowOutput", Fields: []string{"invented_too"}, Inner: true},
	}

	check := shapeCheck(root, rows, published)

	if check.Join.Exact != 2 {
		t.Fatalf("join = %+v, want both rows matched", check.Join)
	}
	if len(check.Unpublished) != 1 || check.Unpublished[0].Endpoints != 1 || check.Unpublished[0].Field != "invented" {
		t.Errorf("unpublished = %+v, want the one field of the top-level type held against the one described response", check.Unpublished)
	}
}

// TestShapeCheck_GraphQLRows_AreNotJoined verifies that a GraphQL row is left
// alone. The record describes the REST API, so a GraphQL request has no
// operation there and counting it as unmatched would report a miss that is only
// about looking in the wrong document.
func TestShapeCheck_GraphQLRows_AreNotJoined(t *testing.T) {
	root := recordIn(t, map[string]response{"GET /api/v4/version": {Response: []string{"version"}}})
	rows := []requestinventory.Row{{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "query"}}

	check := shapeCheck(root, rows, nil)

	if check.Join.RESTRows != 0 || check.Join.Unmatched != 0 {
		t.Errorf("join = %+v, want a GraphQL row counted nowhere", check.Join)
	}
}

// TestShapeCheck_NoRecord_DoesNotRun verifies the one way this check is
// skipped. A missing record is a repository that has not generated one yet, and
// the scope around this must keep working rather than fail for a reason
// unrelated to what it audits.
func TestShapeCheck_NoRecord_DoesNotRun(t *testing.T) {
	check := shapeCheck(t.TempDir(), []requestinventory.Row{{Kind: "rest", Method: "GET", Path: "/version"}}, nil)

	if check.Ran {
		t.Error("shapeCheck() ran without a record to read")
	}
	if len(check.Unpublished) != 0 || check.Join.RESTRows != 0 {
		t.Errorf("check = %+v, want nothing reported", check)
	}
}

// TestShapeCheck_TwoOperationsShareAShape_UnionTheirResponses verifies that a
// shape carrying two operations contributes both their fields. Taking one
// arbitrarily would report the other's fields as unpublished, which is a
// finding about the lookup rather than about GitLab.
func TestShapeCheck_TwoOperationsShareAShape_UnionTheirResponses(t *testing.T) {
	root := recordIn(t, map[string]response{
		"GET /api/v4/groups/{id}/thing":    {Response: []string{"from_groups"}},
		"GET /api/v4/groups/{name}/thing":  {Response: []string{"from_name"}},
		"GET /api/v4/projects/{id}/absent": {},
	})
	rows := []requestinventory.Row{{Package: "p", Kind: "rest", Method: "GET", Path: "/groups/:group_id/thing"}}
	published := []publishedType{{Package: "p", Name: "Output", Fields: []string{"from_groups", "from_name", "invented"}}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 1 || check.Unpublished[0].Field != "invented" {
		t.Errorf("unpublished = %+v, want only the field neither operation declares", check.Unpublished)
	}
}

// TestShapeCheck_ARecordKeyThatNamesNoMethod_IsSkipped verifies that a key the
// index cannot split is passed over rather than indexed under a method of "".
// Every key the generator writes is "METHOD /path", so this is a guard against
// a hand-edited or future-schema record, and skipping is the honest response:
// an entry nothing can address contributes no response names, and inventing a
// method for it would answer a lookup with somebody else's fields.
func TestShapeCheck_ARecordKeyThatNamesNoMethod_IsSkipped(t *testing.T) {
	root := recordIn(t, map[string]response{
		"malformed-key-with-no-method": {Response: []string{"never_seen"}},
		"GET /api/v4/version":          {Response: []string{"version"}},
	})
	rows := []requestinventory.Row{{Package: "p", Kind: "rest", Method: "GET", Path: "/version"}}
	published := []publishedType{{Package: "p", Name: "Output", Fields: []string{"never_seen", "version"}}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 1 || check.Unpublished[0].Field != "never_seen" {
		t.Errorf("unpublished = %+v, want the field only the unaddressable entry declares", check.Unpublished)
	}
}
