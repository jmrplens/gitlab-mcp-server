package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// sentSchema is the schema the sent fixtures are judged by: one field per
// exclusion the walk makes, a connection with its plumbing, an interface with
// two implementations, and a mutation payload.
const sentSchema = `
scalar Time

type Query {
  project(fullPath: ID!): Project
  node: Thing
}

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
type Widget implements Thing { id: ID! kind: String! size: Int! color: String! }
type Gadget implements Thing { id: ID! kind: String! weight: Int! }

type Mutation { createNote(body: String!): NotePayload }
type NotePayload { clientMutationId: String errors: [String!]! note: Note }
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
		if report.Summary.Undeclared != 0 {
			t.Errorf("summary undeclared = %d, want 0", report.Summary.Undeclared)
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

// keys names the fields a report holds for one type, for a failure message.
func keys(fields map[string]sentField) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	return names
}
