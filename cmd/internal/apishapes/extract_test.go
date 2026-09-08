package apishapes_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apishapes"
)

// sampleSpec is GitLab's document in miniature, carrying every shape the
// extraction has to understand: a response named through a $ref, a collection
// whose schema is an array of a $ref, a request body under multipart rather
// than JSON, a schema that refers to itself, and an operation documenting no
// schema at all.
const sampleSpec = `
openapi: 3.0.0
info:
  title: GitLab REST API
  version: v4
paths:
  /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals:
    get:
      parameters:
      - name: id
        in: path
      - name: merge_request_iid
        in: path
      - name: private_token
        in: header
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Approvals'
  /api/v4/projects:
    get:
      parameters:
      - name: search
        in: query
      - name: search
        in: query
      responses:
        '200':
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Project'
    post:
      requestBody:
        content:
          multipart/form-data:
            schema:
              $ref: '#/components/schemas/CreateProject'
      responses:
        '201':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Project'
  /api/v4/nothing:
    delete:
      responses:
        '204':
          description: no content
    summary: not an operation
  /api/v4/loop:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SelfReferring'
  /api/v4/dangling:
    get:
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/NotDeclared'
  /api/v4/plain:
    post:
      responses:
        '200':
          content:
            text/plain:
              schema:
                type: string
        '201':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Project'
  /api/v4/scalar_body:
    put:
      requestBody:
        content:
          application/json:
            schema:
              type: string
      responses:
        '204':
          description: none
components:
  schemas:
    Approvals:
      type: object
      properties:
        approved: {type: boolean}
        approved_by:
          type: array
          items:
            $ref: '#/components/schemas/Approver'
        inline:
          type: object
          properties:
            reason: {type: string}
        user_can_approve: {type: boolean}
        user_has_approved: {type: boolean}
    Approver:
      type: object
      properties:
        approved_at: {type: string}
        user: {type: object}
    Project:
      type: object
      properties:
        id: {type: integer}
        name: {type: string}
    CreateProject:
      type: object
      properties:
        name: {type: string}
        path: {type: string}
    SelfReferring:
      $ref: '#/components/schemas/SelfReferring'
`

// assertEntities checks the components an operation's response and nested
// properties were resolved through.
func assertEntities(t *testing.T, op apishapes.Operation, entity, nestedEntity string) {
	t.Helper()
	if op.Entity != entity {
		t.Errorf("entity = %q, want %q", op.Entity, entity)
	}
	if got := renderNestedEntities(op.NestedEntity); got != nestedEntity {
		t.Errorf("nested entity = %q, want %q", got, nestedEntity)
	}
}

// renderNestedEntities spells a nested-entity map as
// "property=component;property=component", sorted.
func renderNestedEntities(entities map[string]string) string {
	properties := make([]string, 0, len(entities))
	for property := range entities {
		properties = append(properties, property)
	}
	sort.Strings(properties)
	rendered := make([]string, 0, len(properties))
	for _, property := range properties {
		rendered = append(rendered, property+"="+entities[property])
	}
	return strings.Join(rendered, ";")
}

// renderNested spells a nested map as "property=names;property=names", sorted,
// so a case table can state one in a line.
func renderNested(nested map[string][]string) string {
	properties := make([]string, 0, len(nested))
	for property := range nested {
		properties = append(properties, property)
	}
	sort.Strings(properties)
	rendered := make([]string, 0, len(properties))
	for _, property := range properties {
		rendered = append(rendered, property+"="+strings.Join(nested[property], ","))
	}
	return strings.Join(rendered, ";")
}

// TestExtract_EveryShapeTheDocumentUses_BecomesThreeListsOfNames verifies the
// extraction against each shape GitLab's document actually writes, because a
// list this misreads becomes a finding about a field GitLab never mentioned.
func TestExtract_EveryShapeTheDocumentUses_BecomesThreeListsOfNames(t *testing.T) {
	operations, openAPI, apiVersion, err := apishapes.Extract([]byte(sampleSpec))
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil", err)
	}

	if openAPI != "3.0.0" || apiVersion != "v4" {
		t.Errorf("Extract() reported openapi %q and api %q, want 3.0.0 and v4", openAPI, apiVersion)
	}

	cases := []struct {
		name     string
		key      string
		response string
		params   string
		body     string
		// nested is the one property carrying an object, spelled
		// "property=names", and empty when the response carries none.
		nested string
		// entity is the component the response resolves to, and
		// nestedEntity the one the nested property resolves to, spelled
		// "property=component".
		entity       string
		nestedEntity string
	}{
		{
			name:     "a response named through a ref",
			key:      "GET /api/v4/projects/{id}/merge_requests/{merge_request_iid}/approvals",
			response: "approved,approved_by,inline,user_can_approve,user_has_approved",
			// The header parameter is deliberately absent: it is not something
			// an action's input struct carries.
			params: "id,merge_request_iid",
			// approved_by is an array of a ref, so the object under it is the
			// element, and it alone names a component: inline is described
			// where it sits. The other three carry scalars and are left out,
			// since a property with no object is not a property with an empty
			// one.
			nested:       "approved_by=approved_at,user;inline=reason",
			entity:       "Approvals",
			nestedEntity: "approved_by=Approver",
		},
		{
			name:     "a collection is described by its element",
			key:      "GET /api/v4/projects",
			response: "id,name",
			params:   "search",
			entity:   "Project",
		},
		{
			name:     "a body under multipart rather than json",
			key:      "POST /api/v4/projects",
			response: "id,name",
			body:     "name,path",
			entity:   "Project",
		},
		{
			name: "an operation documenting no schema",
			key:  "DELETE /api/v4/nothing",
		},
		{
			// A body that is one scalar names no property, and is recorded
			// the way a body the document does not describe is.
			name: "a body that is not an object",
			key:  "PUT /api/v4/scalar_body",
		},
		{
			name: "a schema that refers to itself",
			key:  "GET /api/v4/loop",
		},
		{
			name: "a reference to a schema nothing declares",
			key:  "GET /api/v4/dangling",
		},
		{
			// The 200 carries text/plain, which describes no properties. The
			// next success code is what the operation is read from, since a
			// response this does not understand is not a response GitLab does
			// not send.
			name:     "a success code whose only content is not json",
			key:      "POST /api/v4/plain",
			response: "id,name",
			entity:   "Project",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			op, ok := operations[testCase.key]
			if !ok {
				t.Fatalf("Extract() did not read %q; it read %d operations", testCase.key, len(operations))
			}
			if got := strings.Join(op.Response, ","); got != testCase.response {
				t.Errorf("response = %q, want %q", got, testCase.response)
			}
			if got := strings.Join(op.Params, ","); got != testCase.params {
				t.Errorf("params = %q, want %q", got, testCase.params)
			}
			if got := strings.Join(op.Body, ","); got != testCase.body {
				t.Errorf("body = %q, want %q", got, testCase.body)
			}
			if got := renderNested(op.Nested); got != testCase.nested {
				t.Errorf("nested = %q, want %q", got, testCase.nested)
			}
			assertEntities(t, op, testCase.entity, testCase.nestedEntity)
		})
	}

	t.Run("a path key that is not a method is not an operation", func(t *testing.T) {
		if _, ok := operations["SUMMARY /api/v4/nothing"]; ok {
			t.Error("Extract() read the path item's summary as an operation")
		}
	})
}

// TestExtract_SomethingThatIsNotTheDocument_IsRefused verifies the two ways the
// input can be wrong, kept apart because their causes are: a truncated or
// corrupted download, and the wrong file entirely.
func TestExtract_SomethingThatIsNotTheDocument_IsRefused(t *testing.T) {
	t.Run("not yaml at all", func(t *testing.T) {
		if _, _, _, err := apishapes.Extract([]byte("\tnot: [yaml")); err == nil {
			t.Error("Extract() error = nil for something that does not parse")
		}
	})

	t.Run("yaml that is not the document", func(t *testing.T) {
		_, _, _, err := apishapes.Extract([]byte("name: something else\n"))

		if !errors.Is(err, apishapes.ErrNotAnAPIDocument) {
			t.Errorf("Extract() error = %v, want ErrNotAnAPIDocument", err)
		}
	})

	t.Run("a method key holding something that is not an operation", func(t *testing.T) {
		_, _, _, err := apishapes.Extract([]byte("openapi: 3.0.0\npaths:\n  /api/v4/x:\n    get: not an operation\n"))

		if err == nil {
			t.Fatal("Extract() error = nil for a method whose value is a scalar")
		}
		if !strings.Contains(err.Error(), "GET /api/v4/x") {
			t.Errorf("Extract() error = %v, want it to name the operation it could not read", err)
		}
	})
}

// TestExtract_ASchemaDescribingNothing_ContributesNoNames verifies the shape a
// comparison must not mistake for an answer: a schema that names neither a
// reference, a list, nor properties. An empty list here means GitLab does not
// say, and reading it as "GitLab sends nothing" would report every field of
// ours as one it does not send.
func TestExtract_ASchemaDescribingNothing_ContributesNoNames(t *testing.T) {
	const spec = "openapi: 3.0.0\npaths:\n  /api/v4/x:\n    get:\n      responses:\n        '200':\n          content:\n            application/json:\n              schema:\n                type: string\n"

	operations, _, _, err := apishapes.Extract([]byte(spec))
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil", err)
	}
	if got := operations["GET /api/v4/x"].Response; len(got) != 0 {
		t.Errorf("response = %v, want nothing for a schema that describes no properties", got)
	}
}
