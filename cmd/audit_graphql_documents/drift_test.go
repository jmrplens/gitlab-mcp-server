package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// pinnedFixture is the schema standing in for the pin in these tests. It is
// written the way GitLab's own is: an enum, an input object that refers to
// itself, an interface with an implementation, and a field taking arguments.
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
  findings(filter: Filter, first: Int): [Finding!]
  node(id: ID!): Node
}

schema {
  query: Query
}
`

// probedFixture is the same schema as a later release might serve it: an enum
// value withdrawn and another added, an argument narrowed, a field gone, and a
// type that changed kind.
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
}

interface Node {
  id: ID!
}

type Finding implements Node {
  id: ID!
  severity: Severity
  found: Time
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
func documentsOf(texts ...string) []document {
	found := make([]document, 0, len(texts))
	for _, text := range texts {
		found = append(found, document{pkg: "fixture", name: "queryFixture", text: text})
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
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if !recorded[testCase.want] {
				t.Errorf("the walk did not record %q; it recorded %v", testCase.want, recorded)
			}
		})
	}
	for _, absent := range []string{"Query.__typename", "Finding.__typename", "String", "ID"} {
		t.Run("does not record "+absent, func(t *testing.T) {
			if recorded[absent] {
				t.Errorf("the walk recorded %q, which no GitLab release can narrow", absent)
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
func TestDriftReport_TwoSchemasAndTheDocumentsBetweenThem_ReportsBothOutcomes(t *testing.T) {
	pinned, probed := loadSchemaFixture(t, pinnedFixture), loadSchemaFixture(t, probedFixture)
	documents := documentsOf(`
query($filter: Filter) {
  findings(filter: $filter) {
    id
    severity
  }
}
`)

	t.Run("schemas that disagree", func(t *testing.T) {
		report := driftReport(pinned, probed, documents, fixturePin, fixtureNow())

		for _, want := range []string{
			"disagree on 3 of 12 coordinate(s)",
			"    Filter.severity: the pin says [Severity!], the live schema says [String!]\n",
			"    Severity: the live schema drops CRITICAL and adds UNKNOWN\n",
			"    Time: the pin says SCALAR, the live schema says ENUM\n",
			"    the pin: 4331 types from https://gitlab.com/api/graphql (GitLab 19.4.0), retrieved 2026-03-01, 10 day(s) ago\n",
		} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(report, want) {
					t.Errorf("the report does not contain %q:\n%s", want, report)
				}
			})
		}
	})

	t.Run("one schema compared with itself", func(t *testing.T) {
		report := driftReport(pinned, pinned, documents, fixturePin, fixtureNow())

		if !strings.Contains(report, "agree on all 12 coordinate(s)") {
			t.Errorf("the report does not say the two agree:\n%s", report)
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
// somebody is in the middle of writing.
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
	})

	if len(walker.found) != 0 {
		t.Errorf("the walk recorded %v from nodes nothing resolved", walker.found)
	}
}
