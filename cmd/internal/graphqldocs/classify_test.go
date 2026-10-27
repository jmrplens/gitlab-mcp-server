// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package graphqldocs

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
//
// The two shapes this rule used to lose are here as well, because losing them
// was invisible: cmd/audit_readonly_graphql kept a looser rule of its own and
// went on reporting a clean run over a set this filter had already narrowed. A
// document does not have to open the string it is written in, and a one-field
// selection of an introspection field is not a unit annotation.
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
		// A fragment with no operation beside it is a document too: this
		// repository declares fragments as constants of their own and splices
		// them into the operations that use them.
		{name: "a fragment on its own", value: "fragment Bits on Thing {\n  id\n  name\n}", want: true},
		{name: "a named operation", value: "query Everything {\n  things { id }\n}", want: true},
		// Four shapes GraphQL accepts that a rule reading only whitespace
		// between the tokens would refuse. Each parses; none is written in
		// this repository today, which is what would have made the loss
		// silent rather than loud.
		{name: "an operation whose keyword is followed by a directive", value: "mutation @skip(if: true) { thing { errors } }", want: true},
		{name: "a comment between the keyword and the selection set", value: "mutation # sends the thing\n{ thing { errors } }", want: true},
		{name: "a comment between the operation name and its variables", value: "mutation Touch # sends the thing\n($id: ID!) { thing(id: $id) { errors } }", want: true},
		{name: "a comment between a fragment's name and its on", value: "fragment Bits # the bits we read\non Thing { id }", want: true},
		{name: "a bare selection set", value: "{ currentUser { id } }", want: true},
		{name: "a selection set spreading a fragment", value: "{ ...Bits }", want: true},
		{name: "a selection set on an underscored field", value: "{ __typename }", want: true},
		{name: "a selection set opening on the line below its keyword", value: "query\n{ thing { id } }", want: true},

		// The keyword does not have to open the string. A document assembled
		// around a template action or a format verb carries the hole in front
		// of it, and one written under a header line carries the header, and
		// neither is a reason to leave the inventory unjudged.
		{name: "a mutation under a template hole", value: "{{/* header */}}\nmutation Touch($id: ID!) {\n  touch(id: $id) { errors }\n}", want: true},
		{name: "a mutation under a format verb", value: "%s\nmutation($id: ID!) {\n  touch(id: $id) { errors }\n}", want: true},
		{name: "a mutation under a header line", value: "Sent to GitLab:\nmutation { thing { errors } }", want: true},
		{name: "a query under a header line", value: "Sent to GitLab:\nquery { thing { id } }", want: true},
		// A UCUM annotation is one word in braces and an introspection field is
		// a name GraphQL reserves, which is what tells the two apart when the
		// document is written without spaces.
		{name: "a spaceless selection set on an introspection field", value: "{__typename}", want: true},

		// The parts of each rule no row above leans on. An operation name may
		// open with an underscore and carry digits and more underscores, and
		// may be followed by a directive; a fragment may sit under a header
		// line or a template hole like an operation may, indented or not.
		{name: "an operation name with underscores and digits", value: "query _list_v2 { things { id } }", want: true},
		{name: "a named operation followed by a directive", value: "query Things @cached { things { id } }", want: true},
		{name: "a fragment under a header line", value: "Sent to GitLab:\nfragment Bits on Thing { id }", want: true},
		{name: "an indented fragment under a template hole", value: "{{/* shared */}}\n\tfragment Bits on Thing {\n\t\tid\n\t}", want: true},

		{name: "an empty string", value: "", want: false},
		{name: "prose with no braces", value: "mutation errors are reported by the caller", want: false},
		{name: "a format string mentioning a mutation", value: "mutation %s failed", want: false},
		{name: "an empty JSON object", value: "{}", want: false},
		{name: "a selection set that is never closed", value: "{ currentUser", want: false},
		{name: "a JSON object", value: `{"id":1,"name":"x"}`, want: false},
		{name: "a JSON object with whitespace", value: "{\n  \"id\": 1\n}", want: false},
		{name: "an example binding written as a map literal", value: `{paramStateEvent:"close"}`, want: false},
		{name: "the same with single quotes", value: `{paramStateEvent:'close'}`, want: false},
		{name: "an identifier that merely starts with a keyword", value: "queryBuilder{}", want: false},
		{name: "a keyword with nothing after it", value: "query", want: false},
		{name: "a Go format verb", value: "%s{%d}", want: false},
		{name: "an OpenTelemetry unit annotation", value: "{entry}", want: false},
		{name: "another one", value: "{eviction}", want: false},
		{name: "one with an underscore", value: "{listen_stream}", want: false},
		// The unit guard keeps one shape it would rather have: a spaceless
		// selection of an ordinary field reads exactly like a unit, so it stays
		// out. It costs no gate anything, since a brace form declares no
		// operation and a mutation cannot be written without its keyword.
		{name: "a spaceless selection set on an ordinary field", value: "{id}", want: false},
		{name: "prose mentioning a mutation above a JSON object", value: "mutation errors: %s\n{\"a\": 1}", want: false},
		{name: "prose beginning with the word fragment", value: "fragment of the response body: { id }", want: false},
		// "on" has to be a word of its own, or the name of any prose word that
		// happens to start with it completes a fragment definition.
		{name: "prose whose word after the name merely starts with on", value: "fragment Bits onward: { id }", want: false},
		// A map literal written with a space on every side of its colon is
		// still a map literal, and reads as a bare selection set if the rule
		// only knows the tight spelling.
		{name: "an example binding written with spaces", value: `{ state : "closed" }`, want: false},

		// A "#" comment is legal at the top of a document and gqlparser accepts
		// one, so a maintainer who writes an ordinary explanatory line above an
		// operation must not thereby drop it out of the inventory: the audit
		// would report one fewer document and still exit 0.
		{name: "a comment above a mutation", value: "# createCustomEmoji is group-scoped.\nmutation($p: ID!) {\n  create(p: $p) { id }\n}", want: true},
		{name: "several comments above a query", value: "# one\n#   two\n\nquery { currentUser { id } }", want: true},
		{name: "a comment above a bare selection set", value: "# anonymous\n{ currentUser { id } }", want: true},
		{name: "a comment above a JSON object is still not a document", value: "# not graphql\n{\"id\": 1}", want: false},
		{name: "nothing but a comment", value: "# just a note", want: false},
		{name: "comments and nothing else", value: "# one\n# two\n", want: false},

		// The keyword does not have to open its line either (issue 1321).
		// GraphQL reads a comma and a byte order mark as Ignored, a second
		// definition may follow the brace that closes the first on the same
		// line, and a shell carries its hole in front of the keyword. A
		// mutation written any of these ways used to leave the inventory, and
		// every gate that reads it, in silence.
		{name: "a mutation behind a leading comma", value: ",mutation Probe { thing(input: {id: \"1\"}) { errors } }", want: true},
		{name: "a mutation behind a byte order mark", value: "\xef\xbb\xbfmutation Probe { thing(input: {id: \"1\"}) { errors } }", want: true},
		{name: "a mutation behind a format verb on its line", value: "%s mutation Probe($id: ID!) { thing(input: {id: $id}) { errors } }", want: true},
		{name: "a mutation behind a template action on its line", value: "{{template \"head\"}} mutation Probe { thing { errors } }", want: true},
		{name: "a query behind a format verb on its line", value: "%[1]s query Things { things { id } }", want: true},
		{name: "a fragment behind a template action on its line", value: "{{/* shared */}} fragment Bits on Thing { id }", want: true},
		{name: "a mutation behind the brace and comma of a query", value: "{ a } , mutation Probe { thing { errors } }", want: true},
		// gqlparser reads a comma as Ignored inside a definition too, so a
		// document the line rule cannot read is admitted when it parses and
		// opens with a definition's keyword.
		{name: "a mutation whose name is followed by a comma", value: "mutation Probe, { thing { errors } }", want: true},
		{name: "a fragment whose name is followed by a comma", value: "fragment Bits, on Thing { id }", want: true},
		{name: "a parsed document behind a comment", value: "# probe\nmutation Probe, { thing { errors } }", want: true},
		// What the wider rules still refuse: prose whose comma stands before
		// the keyword in the middle of a line, a keyword glued to a word, and
		// text that parses only as a bare selection set, which the unit rule
		// above decides.
		{name: "prose with a comma before a keyword mid-line", value: "see the docs, query parameters {page} apply", want: false},
		{name: "a hole glued to a longer word", value: "%subquery Things { things { id } }", want: false},
		{name: "a spaceless selection set that also parses", value: "{entry}", want: false},
		{name: "a parsed document opening with a selection set and nothing else", value: "{id}", want: false},
		{name: "prose that does not parse", value: "mutation Probe, then { the rest }", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := LooksLikeDocument(testCase.value); got != testCase.want {
				t.Errorf("LooksLikeDocument(%q) = %v, want %v", testCase.value, got, testCase.want)
			}
		})
	}
}

// TestDefinesMutation_SeparatesWritesFromReads verifies the half of the rule
// cmd/audit_readonly_graphql rests on, in the shapes [LooksLikeDocument] admits.
//
// The two have to answer from the same reading: a document this filter lets
// into the inventory whose mutation this does not see is a mutation an audit
// classifies as a read, which is the silence that audit exists to remove. So
// every shape admitted above is asked here too, the ones that open with a
// header line and the one that puts its selection set on the next line
// included.
func TestDefinesMutation_SeparatesWritesFromReads(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "an anonymous mutation", value: "\nmutation($id: ID!) {\n  touch(id: $id) { errors }\n}\n", want: true},
		{name: "a named mutation", value: "mutation Touch($id: ID!) {\n  touch(id: $id) { errors }\n}", want: true},
		{name: "a mutation with a space before the brace", value: "mutation {\n  touch { errors }\n}", want: true},
		{name: "a mutation opening on the line below its keyword", value: "mutation\n{ touch { errors } }", want: true},
		{name: "a mutation under a template hole", value: "{{/* header */}}\nmutation Touch($id: ID!) {\n  touch(id: $id) { errors }\n}", want: true},
		{name: "a mutation under a header line", value: "Sent to GitLab:\nmutation { thing { errors } }", want: true},
		{name: "an indented mutation inside a raw literal", value: "\n\t\tmutation Touch($input: In!) {\n\t\t\ttouch(input: $input) { errors }\n\t\t}\n", want: true},
		// The same four shapes as the document test, here because this is the
		// rule that decides whether a document may reach a mutation: a write
		// that slipped past it would be read as a read.
		{name: "a mutation whose keyword is followed by a directive", value: "mutation @skip(if: true) { thing { errors } }", want: true},
		{name: "a mutation with a comment before its selection set", value: "mutation # sends the thing\n{ thing { errors } }", want: true},
		{name: "a mutation with a comment after its name", value: "mutation Touch # sends the thing\n($id: ID!) { thing(id: $id) { errors } }", want: true},
		{name: "a mutation with a comment between two of its lines", value: "mutation Touch\n# sends the thing\n($id: ID!) { thing(id: $id) { errors } }", want: true},
		{name: "a named mutation followed by a directive", value: "mutation Touch @skip(if: false) { thing { errors } }", want: true},
		{name: "a mutation whose name carries underscores and digits", value: "mutation _touch_v2($id: ID!) { touch(id: $id) { errors } }", want: true},

		// The shapes issue 1321 found the line rule blind to. The first two
		// are the issue's own reproduction and its fragment twin: one line,
		// two definitions, the mutation second. A document that parses is
		// read by gqlparser, so where on its line the keyword sits no longer
		// matters, and one that does not parse, a shell, is read by the line
		// rule widened to the keyword after a brace, a comma, a byte order
		// mark or a hole.
		{name: "a query and a mutation on one line", value: "query Q { currentUser { id } } mutation Probe { vulnerabilityDismiss(input: {id: \"gid://gitlab/Vulnerability/1\"}) { errors } }", want: true},
		{name: "a fragment and a mutation on one line", value: "fragment F on Vulnerability { id } mutation Probe { vulnerabilityDismiss(input: {id: \"gid://gitlab/Vulnerability/1\"}) { errors } }", want: true},
		{name: "a selection set and a mutation on one line", value: "{ a } , mutation Probe { thing { errors } }", want: true},
		{name: "a mutation behind a leading comma", value: ",mutation Probe { thing(input: {id: \"1\"}) { errors } }", want: true},
		{name: "a mutation behind a byte order mark", value: "\xef\xbb\xbfmutation Probe { thing(input: {id: \"1\"}) { errors } }", want: true},
		{name: "a mutation whose name is followed by a comma", value: "mutation Probe, { thing { errors } }", want: true},
		{name: "a shell holding a mutation behind a format verb", value: "%s mutation Probe($id: ID!) { thing(input: {id: $id}) { errors } }", want: true},
		{name: "a shell holding a mutation behind a template action", value: "{{template \"head\"}} mutation Probe { thing { errors } }", want: true},
		{name: "a shell holding a mutation behind a comma after a hole", value: "%v, mutation Probe { thing { errors } }", want: true},
		{name: "a shell holding a mutation after a query on its line", value: "query { a } mutation Probe { thing(id: %d) { errors } }", want: true},

		{name: "a query", value: "query($id: ID!) {\n  node(id: $id) { id }\n}", want: false},
		// A document that parses is answered by what it holds, so a line that
		// opens with the keyword inside a block string is not a mutation, and
		// neither is a query and a fragment written on one line.
		{name: "a mutation written inside a block string argument", value: "query {\n  echo(text: \"\"\"\nmutation Touch { touch { errors } }\n\"\"\") { id }\n}", want: false},
		{name: "a query and a fragment on one line", value: "query { a { ...F } } fragment F on Thing { id }", want: false},
		{name: "a shell holding a query behind a hole", value: "%s query Things { things(id: %d) { id } }", want: false},
		{name: "a subscription", value: "subscription {\n  tick\n}", want: false},
		{name: "a fragment", value: "fragment Bits on Thing { id }", want: false},
		{name: "a bare selection set", value: "{ currentUser { id } }", want: false},
		{name: "a mutation named inside a comment line", value: "query {\n  # mutation Touch($id: ID!) { touch }\n  thing { id }\n}", want: false},
		{name: "prose about a mutation", value: "mutation errors: %s", want: false},
		{name: "an identifier that merely starts with the keyword", value: "mutationBuilder{}", want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := DefinesMutation(testCase.value); got != testCase.want {
				t.Errorf("DefinesMutation(%q) = %v, want %v", testCase.value, got, testCase.want)
			}
			// A mutation the inventory does not admit is one no gate reads,
			// so every document answered as a mutation is a document.
			if testCase.want && !LooksLikeDocument(testCase.value) {
				t.Errorf("LooksLikeDocument(%q) = false for a document DefinesMutation answers as a mutation", testCase.value)
			}
		})
	}
}
