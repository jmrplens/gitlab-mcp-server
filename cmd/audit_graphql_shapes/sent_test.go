package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// sentSchema is the schema the sent fixtures are judged by: one field per
// exclusion the walk makes, a connection with its plumbing, an interface with
// two implementations, a union, a connection that pages through edges alone, a
// payload that is not a mutation payload, and a mutation payload.
const sentSchema = `
scalar Time

type Query {
  project(fullPath: ID!): Project
  node: Thing
  holder: Holder
  twin: Holder
  defaulted: Defaulted
}

type Holder {
  id: ID!
  name: String!
  shape: Shape
  thing: Thing
  gizmos: GizmoConnection
  tags: [Label!]
  pipeline: Pipeline
  stamp: Time
  token: String
  rank: Int!
}

union Shape = Widget | Gadget

type GizmoConnection { total: Int! edges: [GizmoEdge!] }
type GizmoEdge { node: Gizmo! }
type Gizmo { id: ID! label: String! weight: Int! }

type Defaulted { id: ID! scaled(scale: Int! = 1): String keyed(key: String!): String }

type Project {
  id: ID!
  name: String!
  webUrl: String
  archived: Boolean!
  secret(token: String!): String
  optional(first: Int): String
  pipeline: Pipeline
  labels: LabelConnection
}

type Pipeline {
  id: ID!
  status: String!
  startedAt: Time
}

type LabelConnection {
  hint: String!
  count: Int!
  cursor: String
  edges: [LabelEdge!]
  nodes: [Label!]
  pageInfo: PageInfo!
}

type LabelEdge { cursor: String! node: Label! }
type PageInfo { hasNextPage: Boolean! endCursor: String }
type Label { title: String! color: String! }

interface Thing { id: ID! kind: String! }
type Widget implements Thing { id: ID! kind: String! size: Int! color: String! owner: Person }
type Gadget implements Thing { id: ID! kind: String! weight: Int! }
type Person { name: String! email: String! }

type Mutation { createNote(body: String!): NotePayload touchNote(body: String!): TouchPayload }
type NotePayload { clientMutationId: String errors: [String!]! note: Note }
type TouchPayload { clientMutationId: String note: Note }
type Note { id: ID! body: String! internal: Boolean }
`

// sentFixture is every position the walk decides about: an operation root, an
// object read through a reference the decoder only traverses, an object read
// for its scalars, a connection with its plumbing, an interface with one
// implementation named and one not, a field with a required argument beside
// one with an optional argument, and a field this package publishes under
// another spelling.
const sentFixture = `package sent

import "fixture/gql"

const getProject = @@
query {
  project(fullPath: "x") {
    pipeline { id }
    labels {
      hint
      nodes { title }
      pageInfo { hasNextPage }
    }
  }
}
@@

const getNode = @@
query { node { id ... on Widget { size } } }
@@

type Output struct {
	WebURL string @@json:"web_url"@@
}

type projectResponse struct {
	Data struct {
		Project *struct {
			Pipeline *struct {
				ID string @@json:"id"@@
			} @@json:"pipeline"@@
			Labels *struct {
				Hint  string @@json:"hint"@@
				Nodes []struct {
					Title string @@json:"title"@@
				} @@json:"nodes"@@
				PageInfo struct {
					HasNextPage bool @@json:"hasNextPage"@@
				} @@json:"pageInfo"@@
			} @@json:"labels"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

type nodeResponse struct {
	Data struct {
		Node struct {
			ID   string @@json:"id"@@
			Size int    @@json:"size"@@
		} @@json:"node"@@
	} @@json:"data"@@
}

func send(service gql.Service) {
	var project projectResponse
	var node nodeResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: getProject}, &project)
	_, _ = service.Do(gql.GraphQLQuery{Query: getNode}, &node)
}
`

// runSent runs the audit over a fixture module against the sent schema and
// returns the report it wrote, with the exit status and both streams.
func runSent(t *testing.T, packages map[string]string, declarations []sentDeclaration) (sentReport, int, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sent.json")
	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:          fixtureModule(t, packages),
		patterns:     []string{"./..."},
		schemaPath:   schemaFile(t, sentSchema),
		reportPath:   path,
		declarations: declarations,
	}, &out, &errOut)

	content, err := os.ReadFile(path) //#nosec G304 -- a path this test wrote
	if err != nil {
		t.Fatalf("read the report: %v; stdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
	var report sentReport
	if decoded := json.Unmarshal(content, &report); decoded != nil {
		t.Fatalf("decode the report: %v", decoded)
	}
	return report, status, out.String(), errOut.String()
}

// offered returns the fields the report holds for one schema type.
func offered(report sentReport, schemaType string) map[string]sentField {
	fields := map[string]sentField{}
	for _, field := range report.Sent {
		if field.SchemaType == schemaType {
			fields[field.Field] = field
		}
	}
	return fields
}

// TestRun_SentDimension_ReportsWhatTheSchemaOffersAtEveryObjectItReads
// verifies the whole depth rule and every exclusion in one walk of one
// fixture: the operation root is not a response, a traversed object is not
// asked about, a read object is, a connection's plumbing is transport rather
// than payload, a field you must supply an argument to fetch is a second
// request, an unnamed implementation of an interface is not offered, and a
// field the decoder reads is never offered back.
func TestRun_SentDimension_ReportsWhatTheSchemaOffersAtEveryObjectItReads(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": sentFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}

	cases := []struct {
		name       string
		schemaType string
		want       []string
		absent     []string
	}{
		{
			name:       "the operation root is not a response",
			schemaType: "Query",
			absent:     []string{"project", "node"},
		},
		{
			name:       "an object the decoder only traverses is not asked about",
			schemaType: "Project",
			absent:     []string{"name", "archived", "webUrl", "optional", "secret"},
		},
		{
			name:       "an object read for its scalars is asked about",
			schemaType: "Pipeline",
			want:       []string{"status", "startedAt"},
			absent:     []string{"id"},
		},
		{
			name:       "connection plumbing is transport rather than payload",
			schemaType: "LabelConnection",
			absent:     []string{"edges", "nodes", "pageInfo", "count", "cursor", "hint"},
		},
		{
			name:       "the cursor object itself is never asked about",
			schemaType: "PageInfo",
			absent:     []string{"endCursor"},
		},
		{
			name:       "an object under a connection is asked about",
			schemaType: "Label",
			want:       []string{"color"},
			absent:     []string{"title"},
		},
		{
			name:       "an interface offers its own fields and each implementation the document names",
			schemaType: "Thing",
			want:       []string{"kind", "color"},
			absent:     []string{"id", "size", "weight"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fields := offered(report, testCase.schemaType)
			for _, field := range testCase.want {
				if _, ok := fields[field]; !ok {
					t.Errorf("%s.%s is offered and not selected, and the report does not hold it: %v", testCase.schemaType, field, keys(fields))
				}
			}
			for _, field := range testCase.absent {
				if _, ok := fields[field]; ok {
					t.Errorf("%s.%s is reported and should not be: %v", testCase.schemaType, field, keys(fields))
				}
			}
		})
	}
}

// TestRun_SentDimension_ExcludesAFieldWithARequiredArgumentAndKeepsAnOptionalOne
// verifies the one exclusion that turns on a field's arguments rather than its
// name: a field you must supply an identifier to fetch is a request of its
// own, and one whose arguments all have an answer without you is not.
func TestRun_SentDimension_ExcludesAFieldWithARequiredArgumentAndKeepsAnOptionalOne(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": argumentFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	fields := offered(report, "Project")
	for _, testCase := range []struct {
		name    string
		field   string
		wantAny bool
	}{
		{name: "a required argument is a second request", field: "secret", wantAny: false},
		{name: "an optional argument is part of this response", field: "optional", wantAny: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, ok := fields[testCase.field]; ok != testCase.wantAny {
				t.Errorf("Project.%s reported = %t, want %t: %v", testCase.field, ok, testCase.wantAny, keys(fields))
			}
		})
	}
}

// argumentFixture reads Project for its own scalars, so the object is asked
// about rather than traversed.
const argumentFixture = `package sent

import "fixture/gql"

const getProject = @@
query { project(fullPath: "x") { id name } }
@@

type Output struct {
	WebURL string @@json:"web_url"@@
}

func send(service gql.Service) {
	var resp struct {
		Data struct {
			Project *struct {
				ID   string @@json:"id"@@
				Name string @@json:"name"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getProject}, &resp)
}
`

// TestRun_SentDimension_AFieldThePackagePublishesElsewhere_IsAnnotatedNotSilenced
// verifies the annotation and its limit. A field this package publishes under
// another spelling carries that spelling, because the value does reach a
// caller; it is not dropped from the report, because the match is by name and
// the same name may be fed from somewhere else entirely. A field the document
// selects and the decoder reads is a different thing, and is never offered
// back at all.
func TestRun_SentDimension_AFieldThePackagePublishesElsewhere_IsAnnotatedNotSilenced(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": argumentFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	fields := offered(report, "Project")
	for _, testCase := range []struct {
		name        string
		field       string
		wantPresent bool
		publishedAs string
	}{
		{name: "a field published under another spelling is annotated", field: "webUrl", wantPresent: true, publishedAs: "web_url"},
		{name: "a field the decoder reads is never offered back", field: "name", wantPresent: false},
		{name: "a field nothing publishes carries no spelling", field: "archived", wantPresent: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			field, ok := fields[testCase.field]
			if ok != testCase.wantPresent {
				t.Fatalf("Project.%s reported = %t, want %t: %v", testCase.field, ok, testCase.wantPresent, keys(fields))
			}
			if ok && field.SameNameInPackage != testCase.publishedAs {
				t.Errorf("Project.%s same_name_in_package = %q, want %q", testCase.field, field.SameNameInPackage, testCase.publishedAs)
			}
		})
	}
}

// TestRun_SentDimension_ClassesAndNullability_SayWhatActingOnAFieldCosts
// verifies the two annotations a reader triages by: what acting on the field
// would cost, and whether the schema promises it with every answer.
func TestRun_SentDimension_ClassesAndNullability_SayWhatActingOnAFieldCosts(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": sentFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, testCase := range []struct {
		name       string
		schemaType string
		field      string
		class      string
		sent       string
	}{
		{name: "a scalar is a leaf the schema always sends", schemaType: "Pipeline", field: "status", class: sentLeaf, sent: sentAlways},
		{name: "a nullable scalar says so", schemaType: "Pipeline", field: "startedAt", class: sentLeaf, sent: sentNullable},
		{name: "an object is a struct to decode", schemaType: "Thing", field: "color", class: sentLeaf, sent: sentAlways},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			field, ok := offered(report, testCase.schemaType)[testCase.field]
			if !ok {
				t.Fatalf("%s.%s is not in the report", testCase.schemaType, testCase.field)
			}
			if field.Class != testCase.class {
				t.Errorf("%s.%s class = %q, want %q", testCase.schemaType, testCase.field, field.Class, testCase.class)
			}
			if field.Sent != testCase.sent {
				t.Errorf("%s.%s sent = %q, want %q", testCase.schemaType, testCase.field, field.Sent, testCase.sent)
			}
		})
	}
}

// TestRun_SentDimension_OneFieldReachedTwice_IsOneFindingWithItsOccurrences
// verifies the reporting grain: a package, a schema type and a field, whatever
// document or position reached it, with the positions counted rather than
// listed. Two documents of one package leaving out the same field is one thing
// to act on.
func TestRun_SentDimension_OneFieldReachedTwice_IsOneFindingWithItsOccurrences(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": twiceFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	field, ok := offered(report, "Pipeline")["startedAt"]
	if !ok {
		t.Fatalf("Pipeline.startedAt is not in the report: %v", keys(offered(report, "Pipeline")))
	}
	if field.Occurrences != 2 {
		t.Errorf("Pipeline.startedAt occurrences = %d, want 2", field.Occurrences)
	}
	if report.Summary.Findings != len(report.Sent) {
		t.Errorf("summary counts %d finding(s) and the report holds %d", report.Summary.Findings, len(report.Sent))
	}
	// The two counts are what a reader compares to see how much of the report
	// is one thing said twice, so they are asserted together and the fixture
	// makes them differ.
	if report.Summary.Occurrences <= report.Summary.Findings {
		t.Errorf("summary counts %d occurrence(s) of %d finding(s), and this fixture reaches one of them twice",
			report.Summary.Occurrences, report.Summary.Findings)
	}
}

// twiceFixture sends two documents that both read a pipeline and both leave
// out the same field.
const twiceFixture = `package sent

import "fixture/gql"

const first = @@
query { project(fullPath: "x") { pipeline { id status } } }
@@

const second = @@
query { project(fullPath: "y") { pipeline { id status } } }
@@

type response struct {
	Data struct {
		Project *struct {
			Pipeline *struct {
				ID     string @@json:"id"@@
				Status string @@json:"status"@@
			} @@json:"pipeline"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

func send(service gql.Service) {
	var one, two response
	_, _ = service.Do(gql.GraphQLQuery{Query: first}, &one)
	_, _ = service.Do(gql.GraphQLQuery{Query: second}, &two)
}
`

// TestRun_MutationPayloadWhoseErrorsNobodyReads_FailsTheGate verifies the one
// sub-class of this dimension that gates, at both shapes that lose GitLab's
// account of a refused mutation and at the one that does not.
//
// The condition is the decoder and not the document, because that is where the
// values are thrown away: a payload that asks for its errors and decodes none
// fails exactly as one that asks for nothing, which is the half the gate used
// to let through. A payload that decodes its errors and does not select them
// is a different defect, an always-empty field, which the other leg of this
// audit names better and this gate leaves to it.
func TestRun_MutationPayloadWhoseErrorsNobodyReads_FailsTheGate(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		fixture string
		want    string
		absent  string
	}{
		{
			name:    "neither selected nor decoded",
			fixture: payloadFixture,
			want:    "is a mutation payload whose errors no field of the decoder reads",
		},
		{
			name:    "selected and not decoded loses them just the same",
			fixture: selectedErrorsFixture,
			want:    "is a mutation payload whose errors no field of the decoder reads",
		},
		{
			name:    "decoded and not selected is the other leg's finding",
			fixture: decodedErrorsFixture,
			want:    "decoded from a field the document never selects",
			absent:  "mutation payload whose errors",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, status, out, errOut := runSent(t, map[string]string{"sent": testCase.fixture}, nil)

			if status != 1 {
				t.Fatalf("run() = %d, want 1; stdout:\n%s\nstderr:\n%s", status, out, errOut)
			}
			if !strings.Contains(errOut, testCase.want) {
				t.Errorf("run() stderr lacks %q:\n%s", testCase.want, errOut)
			}
			if testCase.absent != "" && strings.Contains(errOut, testCase.absent) {
				t.Errorf("run() stderr holds %q and should not:\n%s", testCase.absent, errOut)
			}
		})
	}
}

// payloadFixture sends a mutation whose payload carries user-facing errors and
// asks for none of them.
const payloadFixture = `package sent

import "fixture/gql"

const createNote = @@
mutation { createNote(body: "x") { note { id body } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			CreateNote struct {
				Note struct {
					ID   string @@json:"id"@@
					Body string @@json:"body"@@
				} @@json:"note"@@
			} @@json:"createNote"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: createNote}, &resp)
}
`

// selectedErrorsFixture asks GitLab for the payload's errors and decodes them
// into nothing, so the values arrive on the wire and are dropped: the half the
// gate missed while it turned on the document.
const selectedErrorsFixture = `package sent

import "fixture/gql"

const createNote = @@
mutation { createNote(body: "x") { errors note { id body } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			CreateNote struct {
				Note struct {
					ID   string @@json:"id"@@
					Body string @@json:"body"@@
				} @@json:"note"@@
			} @@json:"createNote"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: createNote}, &resp)
}
`

// decodedErrorsFixture declares the errors field and never selects it, which
// is the defect the existing leg of this audit already names.
const decodedErrorsFixture = `package sent

import "fixture/gql"

const createNote = @@
mutation { createNote(body: "x") { note { id body } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			CreateNote struct {
				Errors []string @@json:"errors"@@
				Note   struct {
					ID   string @@json:"id"@@
					Body string @@json:"body"@@
				} @@json:"note"@@
			} @@json:"createNote"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: createNote}, &resp)
}
`

// TestRun_SentDeclarations_AnswerAFindingAndAreReportedWhenTheyAnswerNothing
// verifies both ends of the adjudication table: a declaration annotates the
// findings it covers and takes them out of the count a reader must act on, and
// one that covers nothing fails the run, because an excuse that outlives what
// it excused is a claim about the tree that is no longer true.
func TestRun_SentDeclarations_AnswerAFindingAndAreReportedWhenTheyAnswerNothing(t *testing.T) {
	answers := sentDeclaration{
		Package:    "fixture/sent",
		SchemaType: "Pipeline",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     "the pipelines domain publishes a pipeline, and this document names one to say which",
	}
	stale := sentDeclaration{
		Package:    "fixture/sent",
		SchemaType: "Label",
		Field:      "color",
		Category:   categoryDeprecated,
		Reason:     "no document of this fixture reaches a label",
	}

	t.Run("a declaration answers the findings it covers", func(t *testing.T) {
		report, status, out, errOut := runSent(t, map[string]string{"sent": twiceFixture}, []sentDeclaration{answers})

		if status != 0 {
			t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
		}
		field, ok := offered(report, "Pipeline")["startedAt"]
		if !ok {
			t.Fatalf("Pipeline.startedAt is not in the report")
		}
		if field.Category != categoryNotThisResponse || field.Reason != answers.Reason {
			t.Errorf("Pipeline.startedAt category = %q, reason = %q; want the declaration's", field.Category, field.Reason)
		}
		if report.Summary.Undeclared != 0 || report.Summary.Findings == 0 {
			t.Errorf("summary counts %d undeclared of %d finding(s), want none undeclared and some found",
				report.Summary.Undeclared, report.Summary.Findings)
		}
	})

	t.Run("a declaration that answers nothing fails the run", func(t *testing.T) {
		report, status, _, errOut := runSent(t, map[string]string{"sent": twiceFixture}, []sentDeclaration{answers, stale})

		if status != 1 {
			t.Fatalf("run() = %d, want 1; stderr:\n%s", status, errOut)
		}
		if want := "fixture/sent.Label.color is declared as a field the schema offers"; !strings.Contains(errOut, want) {
			t.Errorf("run() stderr lacks %q:\n%s", want, errOut)
		}
		if len(report.UnusedDeclarations) != 1 || report.UnusedDeclarations[0] != stale.key() {
			t.Errorf("report unused declarations = %v, want [%s]", report.UnusedDeclarations, stale.key())
		}
	})
}

// TestRun_SentReport_SaysWhatItCouldNotConsult verifies that the two oracles
// this dimension does not have are stated once on the check, so that a reader
// used to the tier annotation of the REST list knows it is absent by fact
// rather than by omission.
func TestRun_SentReport_SaysWhatItCouldNotConsult(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": sentFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, testCase := range []struct {
		name string
		got  string
		want string
	}{
		{name: "the grain a finding is read at", got: report.Check.Grain, want: sentGrain},
		{name: "no tier oracle", got: report.Check.TierOracle, want: oracleNone},
		{name: "no deprecation oracle", got: report.Check.DeprecationOracle, want: oracleNone},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("report check = %q, want %q", testCase.got, testCase.want)
			}
		})
	}
	for _, testCase := range []struct {
		name   string
		reason string
	}{
		{name: "the tier oracle is explained", reason: report.Check.TierOracleReason},
		{name: "the deprecation oracle is explained", reason: report.Check.DeprecationOracleReason},
		{name: "the gating rule is stated", reason: report.Check.Gating},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.reason == "" {
				t.Errorf("the report leaves %q empty, which reads as a condition nobody thought about", testCase.name)
			}
		})
	}
	t.Run("the run names the report it wrote", func(t *testing.T) {
		if !strings.Contains(out, "the schema offers that no document of their package selects") {
			t.Errorf("run() stdout does not name the report:\n%s", out)
		}
	})
	t.Run("the run says how much GraphQL it never saw", func(t *testing.T) {
		if !strings.Contains(out, uncoveredUnavailable) {
			t.Errorf("run() stdout does not say the uncovered set is unknown:\n%s", out)
		}
		if report.Check.Uncovered.Unavailable != uncoveredUnavailable {
			t.Errorf("report uncovered unavailable = %q, want %q", report.Check.Uncovered.Unavailable, uncoveredUnavailable)
		}
	})
}

// TestNormalizeFieldName_TwoSpellingsOfOneValue_Compare verifies the
// comparison that lets a GraphQL field find the value this repository
// publishes under its own convention.
func TestNormalizeFieldName_TwoSpellingsOfOneValue_Compare(t *testing.T) {
	for _, testCase := range []struct {
		name string
		left string
		want string
	}{
		{name: "camel case", left: "startLine", want: "startline"},
		{name: "snake case", left: "start_line", want: "startline"},
		{name: "an initialism", left: "webURL", want: "weburl"},
		{name: "digits are kept", left: "last30DayUsageCount", want: "last30dayusagecount"},
		{name: "a separator below the digits is dropped", left: "web-url", want: "weburl"},
		{name: "a letter above the ASCII alphabet is dropped", left: "naïve", want: "nave"},
		// Each of the three ranges is spelled with its two ends and the two
		// runes just outside them, because every comparison here has a
		// boundary a name made of ordinary words never reaches: the pinned
		// schema has no field whose name carries an A, so a rule that dropped
		// one would compare equal to this one on every real input.
		{name: "each range keeps its ends and drops what sits just outside", left: "@AZ[`az{/09:", want: "azaz09"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizeFieldName(testCase.left); got != testCase.want {
				t.Errorf("normalizeFieldName(%q) = %q, want %q", testCase.left, got, testCase.want)
			}
		})
	}
}

// TestRun_SentDimension_AFieldASiblingDocumentSelects_IsNotAGapInThePackage
// verifies the claim the report makes.
//
// A package sends several documents at one object and they differ on purpose:
// this repository expresses a licensing tier as a CE document that omits a
// field and an EE document that selects it, and a mutation payload can echo
// little more than an id where a query reads the whole object. Judging each
// document alone reports the sibling's selections as gaps, which made
// nineteen percent of the two largest packages false. So a field is a gap only
// where no document of the package selects it.
func TestRun_SentDimension_AFieldASiblingDocumentSelects_IsNotAGapInThePackage(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		fixture  string
		reported bool
	}{
		{name: "no document of the package selects it", fixture: twiceFixture, reported: true},
		{name: "a sibling document selects it", fixture: siblingFixture, reported: false},
		// The cancellation is evidence that the package surfaces the value,
		// so a sibling that asks GitLab for the field and decodes none of it
		// cancels nothing: it surfaces exactly as little as the document that
		// never asked. Letting a dead selection speak here would repeat, one
		// level down, the union-standing-for-an-intersection error this whole
		// subtraction exists to fix.
		{name: "a sibling document selects it and decodes nothing", fixture: siblingDropsItFixture, reported: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			report, status, out, errOut := runSent(t, map[string]string{"sent": testCase.fixture}, nil)

			if status != 0 {
				t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
			}
			fields := offered(report, "Pipeline")
			if _, ok := fields["startedAt"]; ok != testCase.reported {
				t.Errorf("Pipeline.startedAt reported = %t, want %t: %v", ok, testCase.reported, keys(fields))
			}
		})
	}
}

// siblingFixture is twiceFixture with the second document selecting the field
// the first leaves out, which is the shape of a CE and an EE document sent by
// one package.
const siblingFixture = `package sent

import "fixture/gql"

const first = @@
query { project(fullPath: "x") { pipeline { id status } } }
@@

const second = @@
query { project(fullPath: "y") { pipeline { id status startedAt } } }
@@

type firstResponse struct {
	Data struct {
		Project *struct {
			Pipeline *struct {
				ID     string @@json:"id"@@
				Status string @@json:"status"@@
			} @@json:"pipeline"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

type secondResponse struct {
	Data struct {
		Project *struct {
			Pipeline *struct {
				ID        string @@json:"id"@@
				Status    string @@json:"status"@@
				StartedAt string @@json:"startedAt"@@
			} @@json:"pipeline"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

func send(service gql.Service) {
	var one firstResponse
	var two secondResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: first}, &one)
	_, _ = service.Do(gql.GraphQLQuery{Query: second}, &two)
}
`

// siblingDropsItFixture is siblingFixture with the second document selecting
// the field and its decoder reading none of it, which is the shape the other
// leg of this walk already reports as a selection nothing reads. The package
// surfaces the value nowhere, so the finding has to survive.
const siblingDropsItFixture = `package sent

import "fixture/gql"

const first = @@
query { project(fullPath: "x") { pipeline { id status } } }
@@

const second = @@
query { project(fullPath: "y") { pipeline { id status startedAt } } }
@@

type firstResponse struct {
	Data struct {
		Project *struct {
			Pipeline *struct {
				ID     string @@json:"id"@@
				Status string @@json:"status"@@
			} @@json:"pipeline"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

type secondResponse struct {
	Data struct {
		Project *struct {
			Pipeline *struct {
				ID     string @@json:"id"@@
				Status string @@json:"status"@@
			} @@json:"pipeline"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

func send(service gql.Service) {
	var one firstResponse
	var two secondResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: first}, &one)
	_, _ = service.Do(gql.GraphQLQuery{Query: second}, &two)
}
`

// TestRun_SentDimension_ADocumentSentThroughAWrapper_IsFiledAgainstTheDecoder
// verifies who a finding is filed against when the send and the decoder are in
// different packages.
//
// Every epic note and discussion mutation is sent by a shared wrapper in
// toolutil and decoded into the domain's own struct. Filing the finding, and
// the answer to whether the value is published already, against the send names
// a package that publishes nothing and cannot act on the finding.
func TestRun_SentDimension_ADocumentSentThroughAWrapper_IsFiledAgainstTheDecoder(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"wrap": wrapperFixture, "domain": handedOverFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	field, ok := offered(report, "Pipeline")["startedAt"]
	if !ok {
		t.Fatalf("Pipeline.startedAt is not in the report: %v", keys(offered(report, "Pipeline")))
	}
	t.Run("the finding names the package that decodes the object", func(t *testing.T) {
		if field.Package != "fixture/domain" {
			t.Errorf("Pipeline.startedAt package = %q, want fixture/domain", field.Package)
		}
	})
	t.Run("what the package publishes is read from that package", func(t *testing.T) {
		if field.SameNameInPackage != "started_at" {
			t.Errorf("Pipeline.startedAt same_name_in_package = %q, want started_at", field.SameNameInPackage)
		}
	})
	// The two positions are separate because a reader needs both and they are
	// in different packages: the send is one line in a wrapper that knows
	// nothing about pipelines, and the call that named this document is what a
	// reader opens to act on the finding.
	t.Run("the finding names the send and where the document was handed over", func(t *testing.T) {
		if !strings.HasPrefix(field.Position, "wrap/wrap.go:") {
			t.Errorf("Pipeline.startedAt position = %q, want the send in the wrapper", field.Position)
		}
		if !strings.HasPrefix(field.HandedOverAt, "domain/domain.go:") {
			t.Errorf("Pipeline.startedAt handed_over_at = %q, want the call that named the document", field.HandedOverAt)
		}
	})
}

// wrapperFixture sends a document it receives through a parameter and decodes
// it into a type its caller binds, which is the shape of the shared note
// mutation executor.
const wrapperFixture = `package wrap

import "fixture/gql"

type Payload[N any] struct {
	Data struct {
		Project *N @@json:"project"@@
	} @@json:"data"@@
}

func Send[N any](service gql.Service, query string, out *Payload[N]) {
	_, _ = service.Do(gql.GraphQLQuery{Query: query}, out)
}
`

// handedOverFixture names the document, owns the struct the answer lands in,
// and publishes the field under its own spelling.
const handedOverFixture = `package domain

import (
	"fixture/gql"
	"fixture/wrap"
)

const getProject = @@
query { project(fullPath: "x") { pipeline { id status } } }
@@

type pipeline struct {
	ID     string @@json:"id"@@
	Status string @@json:"status"@@
}

type node struct {
	Pipeline *pipeline @@json:"pipeline"@@
}

type Output struct {
	StartedAt string @@json:"started_at"@@
}

func send(service gql.Service) {
	var resp wrap.Payload[node]
	wrap.Send(service, getProject, &resp)
}
`

// TestRun_SentCoverage_TellsPositionsAskedFromPositionsReachedAndSkipped
// verifies the coverage figure the report publishes.
//
// The count of positions the question was put at is smaller than the surface a
// reader would assume from it: a schema type is asked about once per pairing
// however often it is reached, an object that decodes no leaf is traversed
// rather than read, and an object selection no Go field decodes stops the walk
// before anything under it is reached. A figure that does not distinguish
// those overstates what was covered.
func TestRun_SentCoverage_TellsPositionsAskedFromPositionsReachedAndSkipped(t *testing.T) {
	t.Run("every position reached is asked about or counted as skipped", func(t *testing.T) {
		report, status, out, errOut := runSent(t, map[string]string{"sent": sentFixture}, nil)

		if status != 0 {
			t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
		}
		coverage := report.Check.Coverage
		if got := coverage.Asked + coverage.Traversed + coverage.RepeatedType; got != coverage.Reached {
			t.Errorf("positions asked + traversed + repeated = %d, want the %d reached: %+v", got, coverage.Reached, coverage)
		}
		if coverage.Asked == 0 || coverage.Traversed == 0 {
			t.Errorf("the fixture reads one object and traverses another, and the coverage says %+v", coverage)
		}
	})

	t.Run("an object selection no field decodes is counted, not asked", func(t *testing.T) {
		report, status, out, errOut := runSent(t, map[string]string{"sent": undecodedFixture}, nil)

		if status != 0 {
			t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
		}
		if got := report.Check.Coverage.Undecoded; got != 1 {
			t.Errorf("positions skipped undecoded = %d, want 1: %+v", got, report.Check.Coverage)
		}
		if fields := offered(report, "Pipeline"); len(fields) != 0 {
			t.Errorf("a pipeline nothing decodes was asked about anyway: %v", keys(fields))
		}
	})
}

// undecodedFixture selects a pipeline and decodes none of it, so the walk
// stops at the selection with a note and never reaches the object under it.
const undecodedFixture = `package sent

import "fixture/gql"

const getProject = @@
query { project(fullPath: "x") { id name pipeline { id status } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			Project *struct {
				ID   string @@json:"id"@@
				Name string @@json:"name"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getProject}, &resp)
}
`

// TestUncoveredGraphQL_APackageThisWalkNeverPairs_IsNamedInTheReport verifies
// the limit the report states about itself.
//
// The walk pairs a document with a decoder and needs both in the source it
// loads, so a package whose operations client-go builds is outside it
// entirely, and R-PATH cannot see those either. A report that counted only
// what it found would read as the whole GraphQL surface.
func TestUncoveredGraphQL_APackageThisWalkNeverPairs_IsNamedInTheReport(t *testing.T) {
	inventory := requestinventory.Inventory{Requests: []requestinventory.Row{
		{Package: "internal/tools/branchrules", Kind: requestinventory.KindGraphQL, Operation: "query BranchRules"},
		{Package: "internal/tools/epicnotes", Kind: requestinventory.KindGraphQL, Operation: "mutation CreateNote"},
		{Package: "internal/tools/achievements", Kind: requestinventory.KindGraphQL, Operation: "query Achievements"},
		{Package: "internal/tools/achievements", Kind: requestinventory.KindGraphQL, Operation: "mutation AwardAchievement"},
		{Package: "internal/tools/achievements", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects"},
	}}
	pairings := []pairing{
		{Package: "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branchrules"},
		{
			Package:       "github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil",
			OriginPackage: "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicnotes",
		},
	}

	uncovered := uncoveredGraphQL(inventory, pairings)

	for _, testCase := range []struct {
		name string
		got  int
		want int
	}{
		{name: "one package is left", got: len(uncovered.Packages), want: 1},
		{name: "its two GraphQL operations are counted", got: uncovered.Operations, want: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("uncoveredGraphQL() %s = %d, want %d: %+v", testCase.name, testCase.got, testCase.want, uncovered)
			}
		})
	}
	t.Run("a package reached through a wrapper is covered", func(t *testing.T) {
		if len(uncovered.Packages) == 0 || uncovered.Packages[0].Package != "internal/tools/achievements" {
			t.Errorf("uncoveredGraphQL() names %+v, want only internal/tools/achievements", uncovered.Packages)
		}
	})
}

// holderFixture reads one object twice in one document, leaving unselected a
// union, an interface, a connection that pages through edges alone, a list, an
// object and a scalar, so that one finding of every class comes back. Its
// third selection reads an object whose one field takes an argument that
// answers itself beside one that does not.
const holderFixture = `package sent

import "fixture/gql"

const getHolder = @@
query Holding {
  holder { id name }
  twin { id name }
  defaulted { id }
}
@@

type leaves struct {
	ID   string @@json:"id"@@
	Name string @@json:"name"@@
}

func send(service gql.Service) {
	var holding struct {
		Data struct {
			Holder    *leaves @@json:"holder"@@
			Twin      *leaves @@json:"twin"@@
			Defaulted *struct {
				ID string @@json:"id"@@
			} @@json:"defaulted"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getHolder}, &holding)
}
`

// shapeFixture names the members of a union and of an interface in every way
// a document can: an inline fragment, one on the position's own type, one on a
// type the position cannot be, a named fragment spread, and a fragment with no
// type condition at all.
const shapeFixture = `package sent

import "fixture/gql"

const getShape = @@
query Shaping {
  holder {
    id
    name
    shape { ... on Widget { size ... on Thing { kind } } }
    thing { id kind ... on Thing { kind } ...Widgetish ... { id } }
  }
}
fragment Widgetish on Widget { size }
@@

func send(service gql.Service) {
	var shaping struct {
		Data struct {
			Holder *struct {
				ID    string @@json:"id"@@
				Name  string @@json:"name"@@
				Shape *struct {
					Size int    @@json:"size"@@
					Kind string @@json:"kind"@@
				} @@json:"shape"@@
				Thing *struct {
					ID   string @@json:"id"@@
					Kind string @@json:"kind"@@
					Size int    @@json:"size"@@
				} @@json:"thing"@@
			} @@json:"holder"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getShape}, &shaping)
}
`

// TestRun_SentDimension_EveryKindOfFieldSaysWhatActingOnItWouldCost verifies
// the class annotation over the whole schema vocabulary rather than over the
// scalars alone, because the class is what a reader triages by and the two
// that are easy to get wrong are the ones with no fields of their own: a union
// carries none, so the connection test that decides a collection reads nothing
// on it, and a connection that pages through edges alone answers nothing to
// the first half of that test.
func TestRun_SentDimension_EveryKindOfFieldSaysWhatActingOnItWouldCost(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": holderFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, testCase := range []struct {
		name  string
		field string
		class string
	}{
		{name: "a union is a struct to decode", field: "shape", class: sentObject},
		{name: "an interface is a struct to decode", field: "thing", class: sentObject},
		{name: "a connection that pages through edges alone is a collection", field: "gizmos", class: sentCollection},
		{name: "a plain list is a collection", field: "tags", class: sentCollection},
		{name: "an object is a struct to decode", field: "pipeline", class: sentObject},
		{name: "a scalar is one more field", field: "stamp", class: sentLeaf},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			field, ok := offered(report, "Holder")[testCase.field]
			if !ok {
				t.Fatalf("Holder.%s is not in the report: %v", testCase.field, keys(offered(report, "Holder")))
			}
			if field.Class != testCase.class {
				t.Errorf("Holder.%s class = %q, want %q", testCase.field, field.Class, testCase.class)
			}
			if field.Sent != sentNullable {
				t.Errorf("Holder.%s sent = %q, want %q", testCase.field, field.Sent, sentNullable)
			}
		})
	}
	t.Run("a field whose only argument answers itself is part of this response", func(t *testing.T) {
		fields := offered(report, "Defaulted")
		if _, ok := fields["scaled"]; !ok {
			t.Errorf("Defaulted.scaled takes an argument with a default and is not in the report: %v", keys(fields))
		}
		if _, ok := fields["keyed"]; ok {
			t.Errorf("Defaulted.keyed must be supplied a key and is reported anyway: %v", keys(fields))
		}
	})
	t.Run("a finding names the operation it was reached through", func(t *testing.T) {
		field, ok := offered(report, "Holder")["stamp"]
		if !ok {
			t.Fatalf("Holder.stamp is not in the report")
		}
		if field.Operation != "query Holding" {
			t.Errorf("Holder.stamp operation = %q, want the name the document wrote", field.Operation)
		}
	})
}

// TestRun_SentSummary_CountsEachFigureInItsOwnPlace verifies the eight numbers
// the report leads with, over the fixture whose findings span every class.
//
// They are asserted together and the fixture is arranged so no two of them
// agree. Each pair is a straight line of the summary that no branch decides,
// so a count written into the field beside it is invisible to a test that
// checks one at a time, and invisible to one that checks a total.
func TestRun_SentSummary_CountsEachFigureInItsOwnPlace(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": holderFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	summary := report.Summary
	if summary.Leaf != 4 || summary.Object != 3 || summary.Collection != 2 {
		t.Errorf("summary leaf/object/collection = %d/%d/%d, want 4/3/2", summary.Leaf, summary.Object, summary.Collection)
	}
	if summary.Always != 1 || summary.Nullable != 8 {
		t.Errorf("summary always/nullable = %d/%d, want 1/8", summary.Always, summary.Nullable)
	}
	if summary.Findings != len(report.Sent) {
		t.Errorf("summary counts %d finding(s) and the report holds %d", summary.Findings, len(report.Sent))
	}
	// The coverage block is the same shape of claim about the walk: one
	// object is read, one is read a second time and skipped, and nothing is
	// merely traversed, so the four figures differ.
	coverage := report.Check.Coverage
	if coverage.Reached != 3 || coverage.Asked != 2 || coverage.Traversed != 0 || coverage.RepeatedType != 1 {
		t.Errorf("coverage reached/asked/traversed/repeated = %d/%d/%d/%d, want 3/2/0/1",
			coverage.Reached, coverage.Asked, coverage.Traversed, coverage.RepeatedType)
	}
}

// TestRun_SentDimension_AUnionOrInterfaceIsAskedAboutTheMembersTheDocumentNames
// verifies the rule that keeps this dimension from becoming a schema dump at
// exactly the position where it would explode.
//
// A union carries no fields of its own, so asking it alone would report
// nothing and lose a whole family; asking every member would report every
// variant on every finding. So the members are the ones the document names,
// the position's own type is not a member of itself, and a type named deeper
// in the selection that the position cannot be is not one either.
func TestRun_SentDimension_AUnionOrInterfaceIsAskedAboutTheMembersTheDocumentNames(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": shapeFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, testCase := range []struct {
		name       string
		schemaType string
		want       []string
		absent     []string
	}{
		{
			name:       "a union offers the member the document names and nothing of the member it does not",
			schemaType: "Shape",
			want:       []string{"id", "color", "owner"},
			absent:     []string{"size", "kind", "weight"},
		},
		{
			name:       "an interface offers its own fields and the implementation a spread names",
			schemaType: "Thing",
			want:       []string{"color", "owner"},
			absent:     []string{"id", "kind", "size", "weight"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fields := offered(report, testCase.schemaType)
			for _, field := range testCase.want {
				if _, ok := fields[field]; !ok {
					t.Errorf("%s.%s is offered and not selected, and the report does not hold it: %v", testCase.schemaType, field, keys(fields))
				}
			}
			for _, field := range testCase.absent {
				if _, ok := fields[field]; ok {
					t.Errorf("%s.%s is reported and should not be: %v", testCase.schemaType, field, keys(fields))
				}
			}
		})
	}
}

// positionsFixture stops the walk in each of the three ways it can be stopped
// and selects, without decoding, an interface, a union, a list and the cursor
// object, so that the rule deciding which of those the question could have
// been put at is exercised on every kind of type.
const positionsFixture = `package sent

import "fixture/gql"

const getPositions = @@
query {
  holder {
    id
    name
    thing { id kind }
    shape { ... on Widget { size } }
    tags { title }
    pipeline { id status }
  }
  twin { id name }
  defaulted { id }
  project(fullPath: "x") {
    labels { hint pageInfo { hasNextPage } }
  }
}
@@

type raw struct{}

func (r *raw) UnmarshalJSON(body []byte) error { return nil }

func send(service gql.Service) {
	var resp struct {
		Data struct {
			Holder *struct {
				ID       string @@json:"id"@@
				Name     string @@json:"name"@@
				Pipeline raw    @@json:"pipeline"@@
			} @@json:"holder"@@
			Twin      map[string]string @@json:"twin"@@
			Defaulted map[string]string @@json:"defaulted"@@
			Project   *struct {
				Labels *struct {
					Hint string @@json:"hint"@@
				} @@json:"labels"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getPositions}, &resp)
}
`

// TestRun_SentCoverage_CountsOnlyThePositionsTheQuestionCouldHaveBeenPutAt
// verifies the three silences the coverage figure publishes and the rule that
// decides which positions they are counted at.
//
// Each counter stands for a place the walk stopped without asking the schema
// anything, and a figure that counted every stop would overstate what was
// declined: the cursor object is not part of any response, so a selection of
// it that nothing decodes is not a question that went unasked. An interface, a
// union and a list all are, which is what makes the rule worth a test of its
// own rather than a scalar-shaped assumption.
func TestRun_SentCoverage_CountsOnlyThePositionsTheQuestionCouldHaveBeenPutAt(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": positionsFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	coverage := report.Check.Coverage
	for _, testCase := range []struct {
		name string
		got  int
		want int
	}{
		// The three wanted figures differ on purpose: with the same number
		// under each, a walk that counted one stop as another would read
		// exactly like this one.
		{name: "a type that unmarshals itself is trusted with its own decoding", got: coverage.SelfDecoding, want: 1},
		{name: "an object decoded into a map has no fields to judge", got: coverage.Map, want: 2},
		{name: "an interface, a union and a list nothing decodes are three questions unasked", got: coverage.Undecoded, want: 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("coverage %s = %d, want %d: %+v", testCase.name, testCase.got, testCase.want, coverage)
			}
		})
	}
}

// askedPayloadFixture selects a payload's errors and decodes them, so the
// payload is an object this walk reads rather than one it traverses, and the
// fields it offers are asked about.
const askedPayloadFixture = `package sent

import "fixture/gql"

const createNote = @@
mutation { createNote(body: "x") { errors note { id body } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			CreateNote struct {
				Errors []string @@json:"errors"@@
				Note   struct {
					ID   string @@json:"id"@@
					Body string @@json:"body"@@
				} @@json:"note"@@
			} @@json:"createNote"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: createNote}, &resp)
}
`

// TestRun_SentDimension_TheClientMutationIdIsNeverOfferedBack verifies the one
// exclusion that is about the protocol rather than about the response.
//
// Every GitLab mutation payload carries a client mutation id, which is the
// caller's own correlation value echoed back. Offering it would put one
// finding on every mutation this server sends, each of them advice to publish
// a value the caller already has, which is how a backlog stops being read.
func TestRun_SentDimension_TheClientMutationIdIsNeverOfferedBack(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": askedPayloadFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	if fields := offered(report, "NotePayload"); len(fields) != 0 {
		t.Errorf("NotePayload offers %v, and the client mutation id is the only field left", keys(fields))
	}
	if _, ok := offered(report, "Note")["internal"]; !ok {
		t.Errorf("Note.internal is not in the report, so the payload was traversed rather than read")
	}
}

// touchPayloadFixture sends a mutation whose payload carries no errors field
// at all, so the gate's first condition decides it rather than its second.
const touchPayloadFixture = `package sent

import "fixture/gql"

const touch = @@
mutation { touchNote(body: "x") { note { id body } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			TouchNote struct {
				Note struct {
					ID   string @@json:"id"@@
					Body string @@json:"body"@@
				} @@json:"note"@@
			} @@json:"touchNote"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: touch}, &resp)
}
`

// TestRun_APayloadWithNoErrorsField_IsNotAMutationPayloadAndDoesNotGate
// verifies the boundary of the one sub-class that fails a build.
//
// The gate is for a payload that carries GitLab's account of a refused
// mutation and hands it to a decoder that drops it. An object with no errors
// field carries no such account, so there is nothing to drop and nothing to
// fail; treating the client mutation id alone as the mark would fail every
// object GitLab happens to give one.
func TestRun_APayloadWithNoErrorsField_IsNotAMutationPayloadAndDoesNotGate(t *testing.T) {
	report, status, out, errOut := runSent(t, map[string]string{"sent": touchPayloadFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	if strings.Contains(errOut, "mutation payload whose errors") {
		t.Errorf("run() gated a payload that carries no errors field:\n%s", errOut)
	}
	if fields := offered(report, "Note"); len(fields) == 0 {
		t.Errorf("the note under the payload was never asked about, so the walk stopped before the gate could matter")
	}
}

// publishedFixture publishes one value per way a struct can hold another
// (directly, through a pointer, through a slice and through an array) and one
// name twice under two spellings, beside a published type that is not a struct
// at all and one that names itself.
const publishedFixture = `package published

import "fixture/gql"

const getProject = @@
query { project(fullPath: "x") { id name } }
@@

type inline struct {
	Labels string @@json:"labels"@@
}

type nested struct {
	Archived string @@json:"archived"@@
}

type row struct {
	Optional string @@json:"optional"@@
}

type cell struct {
	Pipeline string @@json:"pipeline"@@
}

type DetailOutput struct {
	WebURL    string        @@json:"web_url"@@
	Weburl    string        @@json:"weburl"@@
	Direct    inline        @@json:"direct"@@
	Nested    *nested       @@json:"nested"@@
	Rows      []row         @@json:"rows"@@
	Pair      [2]cell       @@json:"pair"@@
	Tags      []string      @@json:"tags"@@
	Recursive *DetailOutput @@json:"recursive"@@
}

type AliasOutput = string

type CountItem int

func send(service gql.Service) {
	var resp struct {
		Data struct {
			Project *struct {
				ID   string @@json:"id"@@
				Name string @@json:"name"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getProject}, &resp)
}
`

// secondFixture reads two more objects in another package, so the findings the
// report holds span two packages and three schema types: the order the report
// puts them in is observable, and the two counts the summary carries cannot be
// read for one another.
const secondFixture = `package second

import "fixture/gql"

const getPipeline = @@
query { project(fullPath: "x") { pipeline { id status } labels { nodes { title } } } }
@@

func send(service gql.Service) {
	var resp struct {
		Data struct {
			Project *struct {
				Pipeline *struct {
					ID     string @@json:"id"@@
					Status string @@json:"status"@@
				} @@json:"pipeline"@@
				Labels *struct {
					Nodes []struct {
						Title string @@json:"title"@@
					} @@json:"nodes"@@
				} @@json:"labels"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: getPipeline}, &resp)
}
`

// TestRun_SentDimension_AValuePublishedAnywhereInAnOutputTypeIsFound verifies
// the reach of the annotation that tells a triager the value already gets out.
//
// The index is built from a package's output types and not from the struct the
// document decodes into, so it has to walk whatever those types nest a value
// in: a struct held directly, behind a pointer, in a slice or in an array are
// four different Go types and one publishing decision. A type that names
// itself must be walked once rather than forever, and a published name that
// two fields spell differently must resolve to one of them rather than to
// whichever the map iteration reached last.
func TestRun_SentDimension_AValuePublishedAnywhereInAnOutputTypeIsFound(t *testing.T) {
	// The declaration answers one of the three objects, so the four figures
	// the run's own line carries are four different numbers and none of them
	// can be printed in another's place.
	answered := sentDeclaration{
		Package:    "fixture/second",
		SchemaType: "Label",
		Field:      declaredSegment,
		Category:   categoryNotThisResponse,
		Reason:     "a label is named to say which one a node is, and the labels domain publishes its own",
	}
	report, status, out, errOut := runSent(t, map[string]string{"published": publishedFixture, "second": secondFixture}, []sentDeclaration{answered})

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	fields := offered(report, "Project")
	for _, testCase := range []struct {
		name        string
		field       string
		publishedAs string
	}{
		{name: "a field of the output type itself", field: "webUrl", publishedAs: "web_url"},
		{name: "a field of a struct it holds directly", field: "labels", publishedAs: "labels"},
		{name: "a field of a struct behind a pointer", field: "archived", publishedAs: "archived"},
		{name: "a field of a struct in a slice", field: "optional", publishedAs: "optional"},
		{name: "a field of a struct in an array", field: "pipeline", publishedAs: "pipeline"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			field, ok := fields[testCase.field]
			if !ok {
				t.Fatalf("Project.%s is not in the report: %v", testCase.field, keys(fields))
			}
			if field.SameNameInPackage != testCase.publishedAs {
				t.Errorf("Project.%s same_name_in_package = %q, want %q", testCase.field, field.SameNameInPackage, testCase.publishedAs)
			}
		})
	}
	t.Run("findings are ordered by package, then object, then field", func(t *testing.T) {
		order := func(field sentField) string {
			return field.Package + "\x00" + field.SchemaType + "\x00" + field.Field
		}
		for i := 1; i < len(report.Sent); i++ {
			previous, current := report.Sent[i-1], report.Sent[i]
			if order(previous) >= order(current) {
				t.Errorf("finding %d (%s.%s.%s) does not precede %d (%s.%s.%s)",
					i-1, previous.Package, previous.SchemaType, previous.Field,
					i, current.Package, current.SchemaType, current.Field)
			}
		}
		// The two counts are asserted together, and the fixture is built so
		// they differ: with one package per schema type each would read the
		// other's value and nothing here would notice.
		if report.Summary.Packages != 2 || report.Summary.SchemaTypes != 3 {
			t.Errorf("summary counts %d package(s) and %d schema type(s), want 2 and 3",
				report.Summary.Packages, report.Summary.SchemaTypes)
		}
	})
	// The line the run prints is what a reader sees without opening the
	// report, and its four figures are the only place they appear together;
	// asserted as one string so none of them can be printed in another's
	// place.
	t.Run("the run's own line carries the four figures in their own places", func(t *testing.T) {
		want := fmt.Sprintf("%d field(s) the schema offers that no document of their package selects, %d undeclared, across 2 package(s) and 3 schema type(s) ->",
			report.Summary.Findings, report.Summary.Undeclared)
		if !strings.Contains(out, want) {
			t.Errorf("run() stdout lacks %q:\n%s", want, out)
		}
		if report.Summary.Undeclared >= report.Summary.Findings || report.Summary.Undeclared == 0 {
			t.Errorf("summary counts %d undeclared of %d finding(s), and the declaration answers some but not all",
				report.Summary.Undeclared, report.Summary.Findings)
		}
	})
}

// TestRun_WhenTheInventoryIsRead_NamesEveryGraphQLPackageTheWalkNeverSaw
// verifies the other side of the seam over the request inventory.
//
// The run that cannot read the record says so; the run that can must publish
// the set rather than a count, ordered by package so two reports of one tree
// compare, and say on its second line how many operations it never asked
// about. A reader given only the findings reads them as the whole GraphQL
// surface, which is the thing this line exists to prevent.
func TestRun_WhenTheInventoryIsRead_NamesEveryGraphQLPackageTheWalkNeverSaw(t *testing.T) {
	original := readInventory
	readInventory = func(string) (requestinventory.Inventory, error) {
		return requestinventory.Inventory{Requests: []requestinventory.Row{
			{Package: "internal/tools/workitems", Kind: requestinventory.KindGraphQL, Operation: "query WorkItem"},
			{Package: "internal/tools/achievements", Kind: requestinventory.KindGraphQL, Operation: "mutation Award"},
			{Package: "internal/tools/achievements", Kind: requestinventory.KindGraphQL, Operation: "query Achievements"},
			{Package: "internal/tools/terraformstates", Kind: requestinventory.KindGraphQL, Operation: "query State"},
			{Package: "internal/tools/epics", Kind: requestinventory.KindGraphQL, Operation: "query Epics"},
			{Package: "internal/tools/achievements", Kind: requestinventory.KindREST, Method: "GET", Path: "/projects"},
		}}, nil
	}
	t.Cleanup(func() { readInventory = original })

	report, status, out, errOut := runSent(t, map[string]string{"sent": sentFixture}, nil)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	uncovered := report.Check.Uncovered
	t.Run("the set is published rather than declared unknown", func(t *testing.T) {
		if uncovered.Unavailable != "" {
			t.Errorf("report uncovered unavailable = %q, want it empty when the record was read", uncovered.Unavailable)
		}
	})
	t.Run("every recorded GraphQL package this walk never paired is named, in order", func(t *testing.T) {
		var named []string
		for _, pkg := range uncovered.Packages {
			named = append(named, pkg.Package)
		}
		want := []string{
			"internal/tools/achievements",
			"internal/tools/epics",
			"internal/tools/terraformstates",
			"internal/tools/workitems",
		}
		// Compared whole rather than element by element: the order is part of
		// the claim, and a length check followed by a per-element loop says
		// the same thing in two places while reporting a reordering as four
		// separate failures.
		if !slices.Equal(named, want) {
			t.Errorf("report uncovered packages = %v, want %v", named, want)
		}
	})
	t.Run("only the GraphQL rows are counted", func(t *testing.T) {
		if uncovered.Operations != 5 {
			t.Errorf("report uncovered operations = %d, want 5", uncovered.Operations)
		}
	})
	t.Run("the run says on its own line how much it never asked about", func(t *testing.T) {
		if want := "not asked of 5 GraphQL operation(s) in 4 package(s)"; !strings.Contains(out, want) {
			t.Errorf("run() stdout lacks %q:\n%s", want, out)
		}
		if strings.Contains(out, uncoveredUnavailable) {
			t.Errorf("run() stdout calls the set unknown although the record was read:\n%s", out)
		}
	})
}

// TestRepoRelative_APathOutsideInternal_IsLeftAlone verifies the half of the
// trim that keeps the two lists from meeting by accident: an import path with
// no internal segment has no repository-relative spelling to guess, and
// inventing a shorter one could make it match a row it is not.
func TestRepoRelative_APathOutsideInternal_IsLeftAlone(t *testing.T) {
	for _, testCase := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "a package under internal is trimmed to what the inventory records",
			path: "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/branchrules",
			want: "internal/tools/branchrules",
		},
		{name: "a package outside internal is left as it is", path: "fixture/sent", want: "fixture/sent"},
		{name: "a third-party path is left as it is", path: "gitlab.com/gitlab-org/api/client-go/v3", want: "gitlab.com/gitlab-org/api/client-go/v3"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := repoRelative(testCase.path); got != testCase.want {
				t.Errorf("repoRelative(%q) = %q, want %q", testCase.path, got, testCase.want)
			}
		})
	}
}

// keys names the fields a report holds for one type, for a failure message.
func keys(fields map[string]sentField) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names
}
