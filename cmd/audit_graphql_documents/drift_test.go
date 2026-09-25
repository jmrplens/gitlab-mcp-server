package main

import (
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// pinnedFixture is the schema standing in for the pin in these tests. It is
// written the way GitLab's own is: an enum, an input object that refers to
// itself, an interface with an implementation, and a field taking arguments.
//
// `after` is here for one comparison and is passed by no document: it is the
// argument the pin has and the later release does not, which is the direction
// of the presence report that a schema agreeing on every argument cannot
// exercise.
const pinnedFixture = `
scalar Time

enum Severity {
  LOW
  HIGH
  CRITICAL
}

input Filter {
  severity: [Severity!]
  since: Time
  also: Filter
}

interface Node {
  id: ID!
}

type Finding implements Node {
  id: ID!
  severity: Severity
  title: String
  found: Time
}

type Query {
  findings(filter: Filter, first: Int, after: String): [Finding!]
  node(id: ID!): Node
}

schema {
  query: Query
}
`

// probedFixture is the same schema as a later release might serve it: an enum
// value withdrawn and another added, an argument narrowed, a field gone, a type
// that changed kind, an input object that grew a field, and a type that did not
// exist before.
//
// The last two are the ways a later release adds rather than removes. `cursor`
// is what a document handing `Filter` a value silently starts being able to
// send, so it is the coordinate that says whether the walk followed the schema
// this run judged by; `Introduced` is a type the pin has never heard of.
const probedFixture = `
enum Time {
  NOW
}

enum Severity {
  LOW
  HIGH
  UNKNOWN
}

input Filter {
  severity: [String!]
  since: Time
  also: Filter
  cursor: String
}

interface Node {
  id: ID!
}

type Finding implements Node {
  id: ID!
  severity: Severity
  found: Time
}

type Introduced {
  id: ID!
}

type Query {
  findings(filter: Filter, first: Int!): [Finding!]
  node(id: ID!): Node
}

schema {
  query: Query
}
`

// loadSchemaFixture parses one of the fixture schemas.
func loadSchemaFixture(t *testing.T, sdl string) *ast.Schema {
	t.Helper()
	schema, err := graphqlschema.Load([]byte(sdl))
	if err != nil {
		t.Fatalf("load the fixture schema: %v", err)
	}
	return schema
}

// documentsOf wraps raw document text the way the collector hands it over.
func documentsOf(texts ...string) []graphqldocs.Document {
	found := make([]graphqldocs.Document, 0, len(texts))
	for _, text := range texts {
		found = append(found, graphqldocs.Document{Package: "fixture", Name: "queryFixture", Text: text})
	}
	return found
}

// fixturePin is a provenance record with a date the age line can subtract.
var fixturePin = graphqlschema.Source{
	Instance:      "https://gitlab.com/api/graphql",
	GitLabVersion: "19.4.0",
	RetrievedAt:   "2026-03-01",
	Types:         4331,
}

// fixtureNow is the day the fixture pin's age is measured from.
func fixtureNow() time.Time { return time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC) }

// TestCoordinateString_EachDepth_ReadsAsAReaderWouldWriteIt verifies the three
// shapes a finding is named by, since the name is the whole value of a drift
// line: it has to be something a person can look up in the schema.
func TestCoordinateString_EachDepth_ReadsAsAReaderWouldWriteIt(t *testing.T) {
	cases := []struct {
		name string
		at   coordinate
		want string
	}{
		{name: "a type", at: coordinate{typeName: "Finding"}, want: "Finding"},
		{name: "a field", at: coordinate{typeName: "Finding", fieldName: "severity"}, want: "Finding.severity"},
		{
			name: "an argument",
			at:   coordinate{typeName: "Query", fieldName: "findings", argName: "filter"},
			want: "Query.findings(filter)",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.at.String(); got != testCase.want {
				t.Errorf("coordinate.String() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestTouchedCoordinates_WhatTheDocumentsSelect_IsWhatIsCompared verifies the
// walk that makes this report worth reading.
//
// Whole-schema drift between two GitLab releases is thousands of lines and
// tells nobody anything. What is compared has to be exactly what our own
// documents depend on: the fields they select, the arguments they pass, the
// types those name, and every field of an input object, since a document hands
// one a value whose fields it never names and any of them may be sent.
//
// One field is aliased, because a field selected under its own name carries
// that name as its alias too, and a walk that recorded the alias would then
// pass every other case here while naming coordinates no schema holds.
func TestTouchedCoordinates_WhatTheDocumentsSelect_IsWhatIsCompared(t *testing.T) {
	pinned := loadSchemaFixture(t, pinnedFixture)

	found := touchedCoordinates(pinned, pinned, documentsOf(`
query($filter: Filter) {
  __typename
  findings(filter: $filter) {
    __typename
    id
    severity
    ...timestamps
  }
  recent: findings(first: 2) {
    id
  }
  node(id: "gid://x/1") {
    id
    ... on Finding {
      title
    }
  }
}

fragment timestamps on Finding {
  found
}
`))

	recorded := make(map[string]bool, len(found))
	for _, at := range found {
		recorded[at.String()] = true
	}
	cases := []struct {
		name string
		want string
	}{
		{name: "a selected field", want: "Finding.severity"},
		{name: "an argument the document passes", want: "Query.findings(filter)"},
		{name: "the type an argument declares", want: "Filter"},
		{name: "a field of that input object", want: "Filter.severity"},
		{name: "an enum a selected field returns", want: "Severity"},
		{name: "a field reached only through an inline fragment", want: "Finding.title"},
		{name: "a field reached only through a named fragment", want: "Finding.found"},
		{name: "a custom scalar reached through the input object", want: "Time"},
		{name: "an argument passed to an aliased field, under the field's name", want: "Query.findings(first)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !recorded[testCase.want] {
				t.Errorf("the walk did not record %q; it recorded %v", testCase.want, recorded)
			}
		})
	}
	const specification = "which no GitLab release can narrow"
	const alias = "which is the document's alias, not a field any schema holds"
	for _, absent := range []struct{ coordinate, why string }{
		{"Query.__typename", specification},
		{"Finding.__typename", specification},
		{"String", specification},
		{"ID", specification},
		{"Query.recent", alias},
		{"Query.recent(first)", alias},
	} {
		t.Run("does not record "+absent.coordinate, func(t *testing.T) {
			if recorded[absent.coordinate] {
				t.Errorf("the walk recorded %q, %s", absent.coordinate, absent.why)
			}
		})
	}
}

// TestTouchedCoordinates_ADocumentTheProbedSchemaRefuses_IsWalkedUnderThePin
// verifies the fallback, which matters on exactly the run that matters. A
// document the pin accepts and the live schema refuses must still be walked, or
// the report would fall silent about the very document whose refusal prompted
// the question.
func TestTouchedCoordinates_ADocumentTheProbedSchemaRefuses_IsWalkedUnderThePin(t *testing.T) {
	pinned, probed := loadSchemaFixture(t, pinnedFixture), loadSchemaFixture(t, probedFixture)

	found := touchedCoordinates(probed, pinned, documentsOf(
		"query { findings { title } }",
		"query { thisFieldExistsNowhere }",
	))

	var sawTitle bool
	for _, at := range found {
		if at.String() == "Finding.title" {
			sawTitle = true
		}
	}
	if !sawTitle {
		t.Errorf("the walk dropped the document the probed schema refuses; it recorded %v", found)
	}
}

// TestTouchedCoordinates_ADocumentNeitherSchemaAccepts_ContributesNothing
// verifies that a document nothing can resolve is skipped rather than guessed
// at. It is already reported as a refusal, and a walk of it would name fields
// against a type nobody agreed on.
func TestTouchedCoordinates_ADocumentNeitherSchemaAccepts_ContributesNothing(t *testing.T) {
	pinned := loadSchemaFixture(t, pinnedFixture)

	found := touchedCoordinates(pinned, pinned, documentsOf("query { nothingHere }"))

	if len(found) != 0 {
		t.Errorf("touchedCoordinates() = %v, want nothing from a document neither schema accepts", found)
	}
}

// TestDifference_EveryWayTwoSchemasDisagree_IsNamedAtItsCoordinate verifies the
// comparison itself. Each case is a shape GitLab has actually shipped between
// two releases, and the one that broke this repository is the argument that
// narrowed from a string to an enum.
func TestDifference_EveryWayTwoSchemasDisagree_IsNamedAtItsCoordinate(t *testing.T) {
	pinned, probed := loadSchemaFixture(t, pinnedFixture), loadSchemaFixture(t, probedFixture)

	cases := []struct {
		name string
		at   coordinate
		want string
	}{
		{
			name: "an argument that narrowed",
			at:   coordinate{typeName: "Filter", fieldName: "severity"},
			want: "the pin says [Severity!], the live schema says [String!]",
		},
		{
			name: "an argument that became required",
			at:   coordinate{typeName: "Query", fieldName: "findings", argName: "first"},
			want: "the pin says Int, the live schema says Int!",
		},
		{
			name: "a field that is gone",
			at:   coordinate{typeName: "Finding", fieldName: "title"},
			want: "the pin has it, the live schema does not",
		},
		{
			name: "a type that changed kind",
			at:   coordinate{typeName: "Time"},
			want: "the pin says SCALAR, the live schema says ENUM",
		},
		{
			name: "an enum whose values moved",
			at:   coordinate{typeName: "Severity"},
			want: "the live schema drops CRITICAL and adds UNKNOWN",
		},
		{
			name: "a type only one of them has",
			at:   coordinate{typeName: "NotInEither"},
			want: "",
		},
		{name: "a field both agree on", at: coordinate{typeName: "Finding", fieldName: "id"}, want: ""},
		{name: "a type both agree on", at: coordinate{typeName: "Node"}, want: ""},
		{
			name: "an argument both agree on",
			at:   coordinate{typeName: "Query", fieldName: "node", argName: "id"},
			want: "",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := difference(pinned, probed, testCase.at); got != testCase.want {
				t.Errorf("difference(%s) = %q, want %q", testCase.at, got, testCase.want)
			}
		})
	}
}

// TestDifference_AnArgumentOrFieldOnlyOneSideHas_NamesWhichSide verifies both
// directions of the presence report, since a coordinate reached under one
// schema may be absent from the other in either direction, and a reader has to
// know which of the two to go and look at.
func TestDifference_AnArgumentOrFieldOnlyOneSideHas_NamesWhichSide(t *testing.T) {
	pinned, probed := loadSchemaFixture(t, pinnedFixture), loadSchemaFixture(t, probedFixture)

	cases := []struct {
		name         string
		left, right  *ast.Schema
		at           coordinate
		want         string
		wantContains bool
	}{
		{
			name: "a field the pin does not have",
			left: probed, right: pinned,
			at:   coordinate{typeName: "Finding", fieldName: "title"},
			want: "the live schema has it, the pin does not",
		},
		{
			name: "a type the pin does not have",
			left: probed, right: pinned,
			at:   coordinate{typeName: "NotInThePin"},
			want: "",
		},
		{
			name: "a type only the live schema has",
			left: pinned, right: probed,
			at:   coordinate{typeName: "Introduced"},
			want: "the live schema has it, the pin does not",
		},
		{
			name: "a field only the live schema has",
			left: pinned, right: probed,
			at:   coordinate{typeName: "Filter", fieldName: "cursor"},
			want: "the live schema has it, the pin does not",
		},
		{
			name: "an argument only the pin has",
			left: pinned, right: probed,
			at:   coordinate{typeName: "Query", fieldName: "findings", argName: "after"},
			want: "the pin has it, the live schema does not",
		},
		{
			name: "that same argument read the other way round",
			left: probed, right: pinned,
			at:   coordinate{typeName: "Query", fieldName: "findings", argName: "after"},
			want: "the live schema has it, the pin does not",
		},
		{
			name: "an argument the live schema does not have",
			left: pinned, right: probed,
			at:   coordinate{typeName: "Query", fieldName: "findings", argName: "notAnArgument"},
			want: "",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := difference(testCase.left, testCase.right, testCase.at); got != testCase.want {
				t.Errorf("difference(%s) = %q, want %q", testCase.at, got, testCase.want)
			}
		})
	}
}

// TestPresence_ACoordinateNeitherSideHas_IsAgreement verifies the one case that
// is not a finding: a coordinate reached under one document's schema that
// neither schema resolves is not a disagreement between them.
func TestPresence_ACoordinateNeitherSideHas_IsAgreement(t *testing.T) {
	cases := []struct {
		name         string
		inPin, inNew bool
		want         string
	}{
		{name: "neither", want: ""},
		{name: "both", inPin: true, inNew: true, want: ""},
		{name: "only the pin", inPin: true, want: "the pin has it, the live schema does not"},
		{name: "only the live schema", inNew: true, want: "the live schema has it, the pin does not"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := presence(testCase.inPin, testCase.inNew); got != testCase.want {
				t.Errorf("presence(%v, %v) = %q, want %q", testCase.inPin, testCase.inNew, got, testCase.want)
			}
		})
	}
}

// TestEnumDifference_ValuesAddedAndWithdrawn_AreBothReported verifies each half
// of the enum comparison on its own, since an enum that only grew is not a
// problem and an enum that lost a value is one our handlers will hit.
func TestEnumDifference_ValuesAddedAndWithdrawn_AreBothReported(t *testing.T) {
	enum := func(values ...string) *ast.Definition {
		definition := &ast.Definition{Kind: ast.Enum, Name: "Severity"}
		for _, value := range values {
			definition.EnumValues = append(definition.EnumValues, &ast.EnumValueDefinition{Name: value})
		}
		return definition
	}

	cases := []struct {
		name           string
		pinned, probed *ast.Definition
		want           string
	}{
		{name: "the same values", pinned: enum("LOW", "HIGH"), probed: enum("HIGH", "LOW"), want: ""},
		{name: "a value withdrawn", pinned: enum("LOW", "HIGH"), probed: enum("LOW"), want: "the live schema drops HIGH"},
		{name: "a value added", pinned: enum("LOW"), probed: enum("LOW", "HIGH"), want: "the live schema adds HIGH"},
		{
			name:   "both at once",
			pinned: enum("LOW", "HIGH"), probed: enum("LOW", "UNKNOWN"),
			want: "the live schema drops HIGH and adds UNKNOWN",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := enumDifference(testCase.pinned, testCase.probed); got != testCase.want {
				t.Errorf("enumDifference() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestDriftReport_TwoSchemasAndTheDocumentsBetweenThem_ReportsBothOutcomes
// verifies the block a reader actually sees, in both of its shapes: a count
// when the two agree, and a line per coordinate when they do not.
//
// The pin's age is in both, because the number is the point. A pin is a
// photograph of gitlab.com on one day, and without saying how old it is the
// report invites the reader to assume it is current.
// The document supplies `first` so that both fixtures accept it, which is what
// makes the walk's choice of schema observable: a document only the pin accepts
// is walked under the pin whichever schema is preferred, and the Filter.cursor
// line below would then be missing for a reason that says nothing about the
// preference.
//
// Each report is compared whole. The lines come out in the order a reader
// looks a coordinate up in, and the coordinates are gathered in a map, so a
// report that did not sort them would differ from one run to the next and two
// runs could not be compared line for line.
func TestDriftReport_TwoSchemasAndTheDocumentsBetweenThem_ReportsBothOutcomes(t *testing.T) {
	pinned, probed := loadSchemaFixture(t, pinnedFixture), loadSchemaFixture(t, probedFixture)
	documents := documentsOf(`
query($filter: Filter) {
  findings(filter: $filter, first: 1) {
    id
    severity
  }
}
`)
	const pinLine = "    the pin: 4331 types from https://gitlab.com/api/graphql (GitLab 19.4.0), retrieved 2026-03-01, 10 day(s) ago\n"

	t.Run("schemas that disagree", func(t *testing.T) {
		report := driftReport(pinned, probed, documents, fixturePin, fixtureNow())

		want := "audit_graphql_documents: the pin and the live schema disagree on 5 of 14 coordinate(s) the documents touch\n" +
			// Only reachable when the walk followed the schema this run judged
			// by: the pin's Filter has no cursor to reach it through.
			"    Filter.cursor: the live schema has it, the pin does not\n" +
			"    Filter.severity: the pin says [Severity!], the live schema says [String!]\n" +
			"    Query.findings(first): the pin says Int, the live schema says Int!\n" +
			"    Severity: the live schema drops CRITICAL and adds UNKNOWN\n" +
			"    Time: the pin says SCALAR, the live schema says ENUM\n" +
			pinLine
		if report != want {
			t.Errorf("driftReport() =\n%s\nwant\n%s", report, want)
		}
	})

	t.Run("one schema compared with itself", func(t *testing.T) {
		report := driftReport(pinned, pinned, documents, fixturePin, fixtureNow())

		want := "audit_graphql_documents: the pin and the live schema agree on all 13 coordinate(s) the documents touch\n" + pinLine
		if report != want {
			t.Errorf("driftReport() =\n%s\nwant\n%s", report, want)
		}
	})
}

// TestDriftReport_APinWithNoUsableDate_StillReports verifies that provenance
// nothing can subtract costs the age line and nothing else. The record's own
// decoding has already accepted the field, and a second complaint about it here
// would bury the comparison the reader came for.
func TestDriftReport_APinWithNoUsableDate_StillReports(t *testing.T) {
	pinned := loadSchemaFixture(t, pinnedFixture)
	undated := fixturePin
	undated.RetrievedAt = "the day before yesterday"

	report := driftReport(pinned, pinned, documentsOf("query { findings { id } }"), undated, fixtureNow())

	if !strings.Contains(report, "agree on all") {
		t.Errorf("the report does not carry its comparison:\n%s", report)
	}
	if strings.Contains(report, "day(s) ago") {
		t.Errorf("the report invented an age for a date it cannot read:\n%s", report)
	}
}

// TestCoordinateWalker_ANodeNothingResolved_IsSkipped verifies the guards on the
// walk. A field carrying neither the definition it resolved to nor the type it
// was selected on cannot be placed in any schema, and a walk that assumed
// otherwise would panic on the one input this command is pointed at: source
// somebody is in the middle of writing. An entry that holds no selection at
// all is the same case one level up, and is passed over like the rest.
func TestCoordinateWalker_ANodeNothingResolved_IsSkipped(t *testing.T) {
	walker := &coordinateWalker{
		schema:  loadSchemaFixture(t, pinnedFixture),
		found:   map[coordinate]bool{},
		visited: map[string]bool{},
	}

	walker.selections(ast.SelectionSet{
		&ast.Field{Name: "unresolved"},
		&ast.Field{Name: "selectedOnNothing", Definition: &ast.FieldDefinition{
			Name: "selectedOnNothing", Type: ast.NamedType("String", nil),
		}},
		&ast.FragmentSpread{Name: "neverDefined"},
		nil,
	})

	if len(walker.found) != 0 {
		t.Errorf("the walk recorded %v from nodes nothing resolved", walker.found)
	}
}

// TestCoordinateWalker_ANameTheSchemaDoesNotHold_RecordsOnlyWhatItResolved
// verifies the two places the walk consults a schema that may not answer, both
// of which a validated document can never reach and a half-written one can: a
// field whose return type names something the schema does not define, and an
// argument the field it is passed to does not declare. Neither may be recorded
// as a coordinate, because a coordinate the pin cannot resolve either would be
// reported as drift that no GitLab release produced.
func TestCoordinateWalker_ANameTheSchemaDoesNotHold_RecordsOnlyWhatItResolved(t *testing.T) {
	walker := &coordinateWalker{
		schema:  loadSchemaFixture(t, pinnedFixture),
		found:   map[coordinate]bool{},
		visited: map[string]bool{},
	}

	walker.selections(ast.SelectionSet{&ast.Field{
		Name:             "findings",
		ObjectDefinition: &ast.Definition{Kind: ast.Object, Name: "Query"},
		Definition: &ast.FieldDefinition{
			Name:      "findings",
			Type:      ast.NamedType("NoSuchType", nil),
			Arguments: ast.ArgumentDefinitionList{{Name: "filter", Type: ast.NamedType("Filter", nil)}},
		},
		Arguments: ast.ArgumentList{{Name: "notDeclaredHere"}},
	}})

	recorded := make([]string, 0, len(walker.found))
	for at := range walker.found {
		recorded = append(recorded, at.String())
	}
	sort.Strings(recorded)
	want := []string{"Query", "Query.findings", "Query.findings(notDeclaredHere)"}
	if !slices.Equal(recorded, want) {
		t.Errorf("the walk recorded %v, want exactly %v", recorded, want)
	}
}
