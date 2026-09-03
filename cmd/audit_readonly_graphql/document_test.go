package main

import "testing"

// TestClassifyDocument_OperationTypes verifies the whole of the distinction
// this audit rests on: what a string asks GitLab to do is decided by the
// operation type written in it, and nothing else. The cases cover the shapes
// this repository actually writes (raw literals opening with a newline,
// indented documents, named and anonymous operations, documents assembled from
// a fragment constant) and the prose that must not be mistaken for one.
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
			name: "json object is a selection set as far as this audit cares",
			doc:  "{\"a\": 1}",
			want: readDocument,
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
