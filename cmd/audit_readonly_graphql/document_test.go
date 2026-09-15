package main

import "testing"

// TestClassifyDocument_OperationTypes verifies the whole of the distinction
// this audit rests on: what a string asks GitLab to do is decided by the
// operation type written in it, and nothing else. The cases cover the shapes
// this repository actually writes (raw literals opening with a newline,
// indented documents, named and anonymous operations, documents assembled from
// a fragment constant) and the prose that must not be mistaken for one.
//
// Whether a string is a document at all is the inventory's answer now, so the
// shapes it refuses are notADocument here rather than a read nobody sends: a
// JSON object and a metric's unit annotation were classified by this audit
// alone and by nothing else, and the shapes it admits that this audit used to
// be asked about separately are classified the same way from one rule.
func TestClassifyDocument_OperationTypes(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want documentKind
	}{
		{
			name: "anonymous mutation",
			doc:  "\nmutation($id: VulnerabilityID!) {\n  vulnerabilityDismiss(input: {id: $id}) {\n    errors\n  }\n}\n",
			want: writeDocument,
		},
		{
			name: "named mutation",
			doc:  "mutation CreateSecurityCategory($input: SecurityCategoryCreateInput!) {\n  securityCategoryCreate(input: $input) { errors }\n}",
			want: writeDocument,
		},
		{
			name: "mutation with a space before the brace",
			doc:  "mutation {\n  destroyNote(input: {id: \"x\"}) { errors }\n}",
			want: writeDocument,
		},
		{
			name: "indented mutation inside a raw literal",
			doc:  "\n\t\tmutation UpdateThing($input: ThingInput!) {\n\t\t\tthingUpdate(input: $input) { errors }\n\t\t}\n",
			want: writeDocument,
		},
		{
			name: "anonymous query",
			doc:  "\nquery($fullPath: ID!) {\n  group(fullPath: $fullPath) { id }\n}\n",
			want: readDocument,
		},
		{
			name: "named query",
			doc:  "query GetEpic($fullPath: ID!, $iid: String!) {\n  group(fullPath: $fullPath) { epic(iid: $iid) { id } }\n}",
			want: readDocument,
		},
		{
			name: "bare selection set is an anonymous query",
			doc:  "{\n  currentUser { id username }\n}",
			want: readDocument,
		},
		{
			name: "subscription is a read",
			doc:  "subscription OnThing($id: ID!) {\n  thing(id: $id) { state }\n}",
			want: readDocument,
		},
		{
			name: "fragment alone declares no operation",
			doc:  "fragment vulnFields on Vulnerability {\n  id\n  title\n}",
			want: readDocument,
		},
		{
			name: "prose containing the word mutation",
			doc:  "%s mutation errors: %s",
			want: notADocument,
		},
		{
			name: "hint mentioning a mutation by name",
			doc:  "body is rendered as Markdown; the createNote mutation may fail if the work item is locked",
			want: notADocument,
		},
		{
			name: "error format string that starts with the word",
			doc:  "mutation failed",
			want: notADocument,
		},
		{
			name: "empty string",
			doc:  "",
			want: notADocument,
		},
		{
			name: "whitespace only",
			doc:  "   \n\t ",
			want: notADocument,
		},
		{
			name: "braces without an operation",
			doc:  "{\"error\": \"not graphql\"",
			want: notADocument,
		},
		{
			name: "json object is not a document",
			doc:  "{\"a\": 1}",
			want: notADocument,
		},
		{
			name: "mutation under a template hole",
			doc:  "{{/* header */}}\nmutation Touch($id: ID!) {\n  touch(id: $id) { errors }\n}",
			want: writeDocument,
		},
		{
			name: "mutation under a header line",
			doc:  "\nSent to GitLab:\nmutation { thing { errors } }\n",
			want: writeDocument,
		},
		{
			name: "mutation opening on the line below its keyword",
			doc:  "mutation\n{ thing { errors } }",
			want: writeDocument,
		},
		{
			name: "spaceless selection set on an introspection field",
			doc:  "{__typename}",
			want: readDocument,
		},
		{
			name: "an OpenTelemetry unit annotation is not a document",
			doc:  "{entry}",
			want: notADocument,
		},
		{
			name: "a fragment spliced into a mutation is still a mutation",
			doc:  "\nmutation($id: VulnerabilityID!) {\n  vulnerabilityConfirm(input: {id: $id}) {\n    vulnerability {\n      id\n      title\n    }\n    errors\n  }\n}\n",
			want: writeDocument,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := classifyDocument(testCase.doc); got != testCase.want {
				t.Errorf("classifyDocument(%q) = %v, want %v", testCase.doc, got, testCase.want)
			}
		})
	}
}

// TestDocumentKind_String_NamesEveryKind verifies the report wording for each
// classification, including the zero value a caller sees for a string that
// carries no GraphQL at all.
func TestDocumentKind_String_NamesEveryKind(t *testing.T) {
	cases := []struct {
		name string
		kind documentKind
		want string
	}{
		{name: "write", kind: writeDocument, want: "mutation"},
		{name: "read", kind: readDocument, want: "query"},
		{name: "neither", kind: notADocument, want: "not-a-document"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.kind.String(); got != testCase.want {
				t.Errorf("documentKind(%d).String() = %q, want %q", testCase.kind, got, testCase.want)
			}
		})
	}
}
