package main

import "testing"

// TestLooksLikeDocument_TellsGraphQLFromEverythingElse verifies the pre-filter
// every constant string in the repository passes through.
//
// Both directions cost something real. A string wrongly taken for a document
// is reported as broken GraphQL, which is a finding a reviewer has to dismiss;
// a document wrongly skipped is a document that ships unjudged, which is the
// failure this whole gate exists to end. The JSON cases are the ones that
// actually happened: json.RawMessage constants and mocked API responses look
// exactly like a bare selection set until you read the character after the
// brace.
func TestLooksLikeDocument_TellsGraphQLFromEverythingElse(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "a query", value: "\nquery($id: ID!) {\n  node(id: $id) { id }\n}\n", want: true},
		{name: "a mutation", value: "\nmutation($id: ID!) {\n  touch(id: $id) { errors }\n}\n", want: true},
		{name: "a subscription", value: "subscription {\n  tick\n}", want: true},
		{name: "a document opening with a fragment", value: "fragment Bits on Thing { id }\nquery { thing { ...Bits } }", want: true},
		{name: "a named operation", value: "query Everything {\n  things { id }\n}", want: true},
		{name: "a bare selection set", value: "{ currentUser { id } }", want: true},
		{name: "a selection set spreading a fragment", value: "{ ...Bits }", want: true},
		{name: "a selection set on an underscored field", value: "{ __typename }", want: true},

		{name: "an empty string", value: "", want: false},
		{name: "prose with no braces", value: "mutation errors are reported by the caller", want: false},
		{name: "a format string mentioning a mutation", value: "mutation %s failed", want: false},
		{name: "an empty JSON object", value: "{}", want: false},
		{name: "a JSON object", value: `{"id":1,"name":"x"}`, want: false},
		{name: "a JSON object with whitespace", value: "{\n  \"id\": 1\n}", want: false},
		{name: "an example binding written as a map literal", value: `{paramStateEvent:"close"}`, want: false},
		{name: "the same with single quotes", value: `{paramStateEvent:'close'}`, want: false},
		{name: "an identifier that merely starts with a keyword", value: "queryBuilder{}", want: false},
		{name: "a keyword with nothing after it", value: "query", want: false},
		{name: "a Go format verb", value: "%s{%d}", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := looksLikeDocument(testCase.value); got != testCase.want {
				t.Errorf("looksLikeDocument(%q) = %v, want %v", testCase.value, got, testCase.want)
			}
		})
	}
}
