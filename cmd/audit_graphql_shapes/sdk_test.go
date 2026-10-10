package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sdkSchema is the schema the client-go fixtures are judged by. Features is
// an object carrying only other objects, the shape a work item's features
// have, so a run that reads through it is told from one that does not. A
// namespace selected for its achievements alone, which are selected for their
// nodes alone, is two such objects under the operation root rather than under
// an object that is read, so a run reading through them is told from one that
// reads through only what hangs from a read object.
const sdkSchema = `
type Query {
  project(fullPath: ID!): Project
  namespace(fullPath: ID!): Namespace
}

type Project {
  id: ID!
  name: String!
  stars: Int!
  labels: [Label!]!
  features: Features!
}

type Features {
  board: Board
  wiki: Wiki
}

type Board {
  id: ID!
  title: String!
}

type Wiki { title: String! }

type Label { title: String! }

type Mutation {
  createNote(input: NoteInput!): NotePayload
}

input NoteInput { body: String! }

type NotePayload {
  note: Note
  errors: [String!]!
  clientMutationId: String
}

type Note {
  id: ID!
  body: String!
}

type Namespace {
  id: ID!
  name: String!
  achievements: AchievementConnection!
}

type AchievementConnection {
  count: Int!
  nodes: [Achievement!]!
}

type Achievement {
  id: ID!
  name: String!
}
`

// sdkFixture stands in for client-go: a GraphQL transport behind an interface,
// a service reached through its interface and one reached concretely, a
// generic type and an alias that are not candidate implementations, a send in
// a helper only a service method reaches, one whose shell cannot be rendered,
// one whose document the schema refuses, a send in a function that is not a
// method, and a service nothing calls. ORPHAN is replaced by a document no
// send carries, or by nothing.
const sdkFixture = `package sdk

import "fmt"

type GraphQLQuery struct {
	Query     string
	Variables map[string]any
}

type GraphQLInterface interface {
	Do(query GraphQLQuery, response any) error
}

type GraphQL struct{}

func (GraphQL) Do(query GraphQLQuery, response any) error { return nil }

type Client struct {
	GraphQL  GraphQLInterface
	Projects ProjectsServiceInterface
}

type ProjectsServiceInterface interface {
	GetProject(path string) error
	ListLabels(path string) error
}

type ProjectsService struct{ client *Client }

type Page[T any] struct{ Nodes []T }

type Alias = ProjectsService

type point struct{ X int }

func (s *ProjectsService) Ping() {}

const getProject = @@query($path: ID!) { project(fullPath: $path) { id name features { board { id } } } }@@

const listLabels = @@query($path: ID!) { project(fullPath: $path) { labels { title } } }@@

const getStars = @@query { project(fullPath: "x") { stars name } }@@

const shell = @@query { project(fullPath: "%v") { name } }@@

const refused = @@query { project(fullPath: "x") { nonexistent } }@@

const createNote = @@mutation { createNote(input: {body: "x"}) { note { id } errors } }@@
ORPHAN
func (s *ProjectsService) GetProject(path string) error {
	var resp struct {
		Data struct {
			Project *struct {
				ID       string @@json:"id"@@
				Name     string @@json:"name"@@
				Features struct {
					Board *struct {
						ID string @@json:"id"@@
					} @@json:"board"@@
				} @@json:"features"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: getProject}, &resp)
}

func (s *ProjectsService) ListLabels(path string) error {
	var pinged interface{ Ping() } = s
	pinged.Ping()
	if path == "" {
		return s.labels("default")
	}
	return s.labels(path)
}

func (s *ProjectsService) labels(path string) error {
	var resp struct {
		Data struct {
			Project *struct {
				Labels []struct {
					Title string @@json:"title"@@
					Color string @@json:"color"@@
				} @@json:"labels"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: listLabels}, &resp)
}

func (s *ProjectsService) Stars(path string) error {
	var resp struct {
		Data struct {
			Project *struct {
				Name string @@json:"name"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: fmt.Sprintf(shell, point{X: 1})}, &resp)
}

func (s *ProjectsService) Broken() error {
	var resp struct {
		Data struct{} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: refused}, &resp)
}

func Standalone(c *Client) error {
	var resp struct {
		Data struct {
			Project *struct {
				Stars int @@json:"stars"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	if err := c.GraphQL.Do(GraphQLQuery{Query: getStars}, &resp); err != nil {
		return err
	}
	return c.GraphQL.Do(GraphQLQuery{Query: getStars}, &resp)
}

type NotesService struct{ client *Client }

func (s *NotesService) Edit() error {
	var resp struct {
		Data struct{} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: fmt.Sprintf(shell, point{X: 2})}, &resp)
}

func (s *NotesService) Create() error {
	var resp struct {
		Data struct {
			CreateNote *struct {
				Note *struct {
					ID   string @@json:"id"@@
					Body string @@json:"body"@@
				} @@json:"note"@@
				Errors []string @@json:"errors"@@
			} @@json:"createNote"@@
		} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: createNote}, &resp)
}
`

// sdkMockFixture implements the service interface in another package of the
// stand-in, the way client-go's testing package does.
const sdkMockFixture = `package mock

type Projects struct{}

func (Projects) GetProject(path string) error { return nil }

func (Projects) ListLabels(path string) error { return nil }
`

// sdkCallerFixture is this repository's side: a package calling the stand-in
// through the service interface twice, concretely, and through a function.
const sdkCallerFixture = `package app

import "fixture/sdk"

func Get(c *sdk.Client) error { return c.Projects.GetProject("x") }

func Again(c *sdk.Client) error { return c.Projects.GetProject("y") }

func Labels(c *sdk.Client) error { return c.Projects.ListLabels("x") }

func Stars(s *sdk.ProjectsService) error { return s.Stars("x") }

func Broken(s *sdk.ProjectsService) error { return s.Broken() }

func Standalone(c *sdk.Client) error { return sdk.Standalone(c) }

func Describe(err error) string { return err.Error() }
`

// sdkOrphan is two documents the stand-in declares and no send carries: one
// written whole, and one a format rendered at a call that sends nothing.
const sdkOrphan = "\nconst orphan = @@query { project(fullPath: \"x\") { id } }@@\n" +
	"\nconst renderedOrphan = @@query { project(fullPath: %q) { name } }@@\n" +
	"\nfunc formatsOrphan() string { return fmt.Sprintf(renderedOrphan, \"x\") }\n"

// sdkDeclared answers every finding the stand-in produces, which is what a
// committed table does for the real client-go.
var sdkDeclared = sdkTables{
	shapes: []sdkShapeDeclaration{{
		Method: "ProjectsService.labels", Path: "data.project.labels[].color",
		Category: categoryDecodedNeverRead, Reason: "the label's color is decoded and nothing reads it",
	}},
	sends: []sdkSendDeclaration{
		{Method: "ProjectsService.Stars", Category: categoryBuiltPerCall, Reason: "the shell is filled with a point"},
		{Method: "ProjectsService.Broken", Category: categoryRecordedUpstream, Reason: "the document names a field GitLab removed"},
	},
	sent: []sentDeclaration{
		{Package: "fixture/sdk.ProjectsService", SchemaType: "Project", Field: "stars", Category: categorySeparateAction, Reason: "stars are read by Standalone"},
		{Package: "fixture/sdk.ProjectsService", SchemaType: "Features", Field: "wiki", Category: categoryRecordedUpstream, Reason: "row 0 records the wiki"},
		{Package: "fixture/sdk.ProjectsService", SchemaType: "Board", Field: declaredSegment, Category: categoryNotThisResponse, Reason: "the board is named by id"},
		{Package: "fixture/sdk", SchemaType: "Project", Field: declaredSegment, Category: categoryLookup, Reason: "Standalone reads the stars and nothing else"},
	},
}

// sdkFixtureRoot writes the stand-in client-go and the calling package into
// one module, with the calling package under prefix, and points the client-go
// lookup at the stand-in for the rest of the test.
func sdkFixtureRoot(t *testing.T, orphan, prefix string) string {
	t.Helper()
	root := fixtureModule(t, map[string]string{
		"sdk":           strings.Replace(sdkFixture, "ORPHAN", orphan, 1),
		"sdk/mock":      sdkMockFixture,
		prefix + "/app": sdkCallerFixture,
	})
	original := sdkDirectory
	sdkDirectory = func(string) (string, error) { return filepath.Join(root, "sdk"), nil }
	t.Cleanup(func() { sdkDirectory = original })
	return root
}

// runSDKFixture runs the client-go half over the stand-in with the tables
// given, and returns the exit status and both streams.
func runSDKFixture(t *testing.T, orphan string, tables sdkTables, verbose bool, report string) (int, string, string) {
	t.Helper()
	root := sdkFixtureRoot(t, orphan, "callers")
	var out, errOut bytes.Buffer
	status := runSDK(auditRun{
		dir:             root,
		patterns:        []string{"./callers/..."},
		verbose:         verbose,
		schemaPath:      schemaFile(t, sdkSchema),
		reportPath:      report,
		sdk:             true,
		sdkDeclarations: tables,
	}, &out, &errOut)
	return status, out.String(), errOut.String()
}

// TestRunSDK_EveryFindingDeclared_PassesAndListsTheAnswersUnderVerbose
// verifies a client-go run whose table answers everything: it exits 0, says
// how many sends this server reaches and how many it never calls, and under
// -v lists each answer with its reason and each send nothing reaches.
//
// Every finding the stand-in produces is in the list, which is what holds the
// run to have asked each question: the field a document's work item features
// would hide (wiki, offered under an object that carries only other objects,
// reached only because the run reads through it), the field a sibling
// document of the same service selects and so is not reported (the project's
// labels), the unit a function that is not a method files under (the
// package), and the send reached only through a helper.
func TestRunSDK_EveryFindingDeclared_PassesAndListsTheAnswersUnderVerbose(t *testing.T) {
	status, out, errOut := runSDKFixture(t, "", sdkDeclared, true, "")

	if status != 0 {
		t.Fatalf("runSDK() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	if errOut != "" {
		t.Errorf("runSDK() wrote to stderr:\n%s", errOut)
	}
	for _, want := range []string{
		"    declared  ProjectsService.labels (sdk.go:87): data.project.labels[].color: decoded from a field the document never selects, so it is always empty\n              the label's color is decoded and nothing reads it\n",
		"    declared  ProjectsService.Stars (sdk.go:98): the document is a shell fmt.Sprintf fills, and fmt.Sprintf formats point{…}, a fixture/sdk.point, which this walk has no stand-in for\n",
		"    declared  ProjectsService.Broken (sdk.go:105): refused is a document the schema refuses, so it has no selection set to judge its decoder by\n",
		"    declared  sdk.ProjectsService Project.stars (Int!): offered at data.project of getProject (sdk.go:64) and selected by no document of the service\n",
		"    declared  sdk.ProjectsService Features.wiki (Wiki): offered at data.project.features of getProject (sdk.go:64) and selected by no document of the service\n",
		"    declared  sdk.ProjectsService Board.title (String!): offered at data.project.features.board of getProject (sdk.go:64) and selected by no document of the service\n",
		"    declared  sdk Project.id (ID!): offered at data.project of getStars (sdk.go:116) and selected by no document of the service\n",
		"    not reached  NotesService.Create\n    not reached  NotesService.Edit\n",
		", 4 pairing(s) judged at the sends this server reaches, 2 function(s) holding a send it never calls: 1 disagreement(s), 2 unjudged send(s) and 6 sent field(s), every one declared (",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("runSDK() stdout lacks %q:\n%s", want, out)
			}
		})
	}
	t.Run("a field a sibling document of the service selects is not reported", func(t *testing.T) {
		if strings.Contains(out, "sdk.ProjectsService Project.labels") {
			t.Errorf("runSDK() reports the project's labels, which listLabels selects:\n%s", out)
		}
	})
}

// TestRunSDK_NothingDeclared_NamesEveryFindingAndFails verifies the gate: a
// disagreement, an unjudged send and a sent field nothing answers each fail
// the run on a line of their own, as do a document no send carries and every
// declaration that answers nothing, of each of the three tables, and the
// summary counts them.
func TestRunSDK_NothingDeclared_NamesEveryFindingAndFails(t *testing.T) {
	stale := sdkTables{
		shapes: []sdkShapeDeclaration{{Method: "ProjectsService.gone", Path: "data.gone", Category: categoryDecodedNeverRead, Reason: "gone"}},
		sends:  []sdkSendDeclaration{{Method: "ProjectsService.Gone", Category: categoryBuiltPerCall, Reason: "gone"}},
		sent:   []sentDeclaration{{Package: "fixture/sdk.ProjectsService", SchemaType: "Gone", Field: declaredSegment, Category: categoryLookup, Reason: "gone"}},
	}
	status, out, errOut := runSDKFixture(t, sdkOrphan, stale, false, "")

	if status != 1 {
		t.Fatalf("runSDK() = %d, want 1; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, want := range []string{
		"ProjectsService.labels (sdk.go:93): data.project.labels[].color: decoded from a field the document never selects, so it is always empty\n",
		"ProjectsService.Stars (sdk.go:104): the document is a shell fmt.Sprintf fills,",
		"ProjectsService.Broken (sdk.go:111): refused is a document the schema refuses,",
		"sdk.ProjectsService Project.stars (Int!): offered at data.project of getProject (sdk.go:70) and selected by no document of the service\n",
		"sdk Project.id (ID!): offered at data.project of getStars (sdk.go:122)",
		"fixture/sdk (sdk.go:50): orphan is a document no send this audit can see carries, so its decoder is judged by nobody\n",
		"fixture/sdk (sdk.go:52): renderedOrphan is a document no send this audit can see carries, so its decoder is judged by nobody\n",
		"sdk_declarations.go: ProjectsService.gone data.gone is declared and answered nothing:",
		"sdk_declarations.go: ProjectsService.Gone is declared and answered nothing:",
		"sdk_declarations_sent.go: fixture/sdk.ProjectsService.Gone.* is declared and answered nothing:",
		", 4 pairing(s) judged at the sends this server reaches, 2 function(s) holding a send it never calls: 9 finding(s) nothing declares, 2 document(s) no send carries, 3 stale declaration(s) (",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("runSDK() stderr lacks %q:\n%s", want, errOut)
			}
		})
	}
	t.Run("a send nothing reaches is not judged", func(t *testing.T) {
		if strings.Contains(errOut, "NotesService") || strings.Contains(errOut, "data.createNote") {
			t.Errorf("runSDK() judged a send this server never calls:\n%s", errOut)
		}
	})
	t.Run("without -v nothing is listed on stdout", func(t *testing.T) {
		if out != "" {
			t.Errorf("runSDK() wrote to stdout without -v:\n%s", out)
		}
	})
}

// TestRunSDK_Report_RecordsWhatTheRunDecided verifies the record -report
// writes: what was read and reached, from where, what was not, and every
// finding with its answer.
func TestRunSDK_Report_RecordsWhatTheRunDecided(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sdk-sent.json")
	status, out, errOut := runSDKFixture(t, sdkOrphan, sdkDeclared, false, path)

	if status != 1 {
		t.Fatalf("runSDK() = %d, want 1 for the orphan; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	if want := "audit_graphql_shapes: client-go: 6 field(s) the schema offers that no document of their service selects -> " + path + "\n"; !strings.Contains(out, want) {
		t.Errorf("runSDK() stdout lacks %q:\n%s", want, out)
	}
	raw, err := os.ReadFile(path) //#nosec G304 -- a file under the test's own temporary directory
	if err != nil {
		t.Fatalf("read the report: %v", err)
	}
	var written sdkReport
	if decoded := json.Unmarshal(raw, &written); decoded != nil {
		t.Fatalf("decode the report: %v", decoded)
	}
	for _, testCase := range []struct {
		name      string
		got, want any
	}{
		{name: "the grain", got: written.Check.Grain, want: sdkGrain},
		{name: "the module is the stand-in's directory", got: strings.HasSuffix(written.Check.Module, "/sdk"), want: true},
		{name: "the judged sends", got: written.Check.Pairings, want: 4},
		{name: "the first reached function", got: written.Check.Reached[0].Method, want: "ProjectsService.GetProject"},
		{name: "where it is reached from", got: strings.Join(written.Check.Reached[0].From, ","), want: "fixture/callers/app"},
		{name: "a function reached through a helper", got: written.Check.Reached[1].Method, want: "ProjectsService.labels"},
		{name: "a function that is no method", got: written.Check.Reached[2].Method, want: "Standalone"},
		{name: "the reached functions", got: len(written.Check.Reached), want: 3},
		{name: "what is not reached", got: strings.Join(written.Check.NotReached, ","), want: "NotesService.Create,NotesService.Edit"},
		{name: "the sent fields", got: len(written.Sent), want: 6},
		{name: "every one answered", got: written.Summary.Undeclared, want: 0},
		{name: "the disagreement's answer", got: written.Disagreements[0].Category, want: categoryDecodedNeverRead},
		{name: "the unjudged sends", got: len(written.Unjudged), want: 2},
		{
			name: "the orphans", got: strings.Join(written.UnpairedDocuments, ","),
			want: "sdk.go:50: orphan is a document no send this audit can see carries, so its decoder is judged by nobody," +
				"sdk.go:52: renderedOrphan is a document no send this audit can see carries, so its decoder is judged by nobody",
		},
		{name: "the gate", got: written.Check.Gating, want: sdkGatingReason()},
		{name: "what reached means", got: written.Check.ReachReason, want: sdkReachReason()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("the report holds %v, want %v", testCase.got, testCase.want)
			}
		})
	}
}

// sdkSharedFixture stands in for a client-go whose services send through one
// shared generic helper, the shape a client-go refactor could take, beside a
// service whose document reaches a namespace's achievements through two
// objects that only carry the next one. NotesService sends through the helper
// and nothing on this repository's side calls it.
const sdkSharedFixture = `package sdk

type GraphQLQuery struct {
	Query     string
	Variables map[string]any
}

type GraphQLInterface interface {
	Do(query GraphQLQuery, response any) error
}

type Client struct{ GraphQL GraphQLInterface }

func query[T any](c *Client, document string) (*T, error) {
	var resp struct {
		Data T @@json:"data"@@
	}
	err := c.GraphQL.Do(GraphQLQuery{Query: document}, &resp)
	return &resp.Data, err
}

const getProject = @@query { project(fullPath: "x") { id name } }@@

const createNote = @@mutation { createNote(input: {body: "x"}) { note { id } errors } }@@

const getAchievements = @@query { namespace(fullPath: "x") { achievements { nodes { id } } } }@@

type projectData struct {
	Project *struct {
		ID    string @@json:"id"@@
		Name  string @@json:"name"@@
		Stars int    @@json:"stars"@@
	} @@json:"project"@@
}

type noteData struct {
	CreateNote *struct {
		Note *struct {
			ID   string @@json:"id"@@
			Body string @@json:"body"@@
		} @@json:"note"@@
		Errors []string @@json:"errors"@@
	} @@json:"createNote"@@
}

type ProjectsService struct{ client *Client }

func (s *ProjectsService) GetProject() error {
	_, err := query[projectData](s.client, getProject)
	return err
}

type NotesService struct{ client *Client }

func (s *NotesService) Create() error {
	_, err := query[noteData](s.client, createNote)
	return err
}

type NamespacesService struct{ client *Client }

func (s *NamespacesService) Achievements() error {
	var resp struct {
		Data struct {
			Namespace *struct {
				Achievements struct {
					Nodes []struct {
						ID string @@json:"id"@@
					} @@json:"nodes"@@
				} @@json:"achievements"@@
			} @@json:"namespace"@@
		} @@json:"data"@@
	}
	return s.client.GraphQL.Do(GraphQLQuery{Query: getAchievements}, &resp)
}
`

// sdkSharedCallerFixture is this repository's side of sdkSharedFixture: it
// calls the project and namespace services and never the notes service.
const sdkSharedCallerFixture = `package app

import "fixture/sdk"

func Get(s *sdk.ProjectsService) error { return s.GetProject() }

func Achievements(s *sdk.NamespacesService) error { return s.Achievements() }
`

// TestRunSDK_SharedHelperAndTraversedObjects_JudgesWhatEachReachedServiceSends
// verifies two things a client-go shaped otherwise than today's would expose.
//
// A document handed to a shared helper belongs to the method that hands it
// over: the notes service's document is not judged because nothing here calls
// the notes service, although the helper it sends through is reached through
// the project service, and what the project document leaves out is filed under
// the project service rather than under the helper's package. A disagreement
// is named by that method, at the line that hands the document over.
//
// And reading through an object that carries only the next one is limited to
// an object that hangs from one that is read: the namespace hangs from the
// operation root and its achievements from the namespace, so both are
// traversed and asked nothing, while the achievement nodes, which decode a
// leaf, are asked.
func TestRunSDK_SharedHelperAndTraversedObjects_JudgesWhatEachReachedServiceSends(t *testing.T) {
	root := fixtureModule(t, map[string]string{
		"sdk":         sdkSharedFixture,
		"callers/app": sdkSharedCallerFixture,
	})
	original := sdkDirectory
	sdkDirectory = func(string) (string, error) { return filepath.Join(root, "sdk"), nil }
	t.Cleanup(func() { sdkDirectory = original })
	path := filepath.Join(t.TempDir(), "sdk-sent.json")
	var out, errOut bytes.Buffer

	status := runSDK(auditRun{
		dir:        root,
		patterns:   []string{"./callers/..."},
		verbose:    true,
		schemaPath: schemaFile(t, sdkSchema),
		reportPath: path,
		sdk:        true,
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("runSDK() = %d, want 1 for the findings nothing declares; stdout:\n%s\nstderr:\n%s", status, out.String(), errOut.String())
	}
	for _, want := range []string{
		"ProjectsService.GetProject (sdk.go:18, handed over at sdk.go:49): data.project.stars: decoded from a field the document never selects, so it is always empty\n",
		"sdk.ProjectsService Project.labels ([Label!]!): offered at data.project of getProject (sdk.go:18, handed over at sdk.go:49) and selected by no document of the service\n",
		"sdk.NamespacesService Achievement.name (String!): offered at data.namespace.achievements.nodes[] of getAchievements (sdk.go:74) and selected by no document of the service\n",
		", 2 pairing(s) judged at the sends this server reaches, 1 function(s) holding a send it never calls: ",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(errOut.String(), want) {
				t.Errorf("runSDK() stderr lacks %q:\n%s", want, errOut.String())
			}
		})
	}
	for _, absent := range []string{"createNote", "Note.", "NotePayload", "sdk Project.", "Namespace.", "AchievementConnection."} {
		t.Run("nothing names "+absent, func(t *testing.T) {
			if strings.Contains(errOut.String(), absent) {
				t.Errorf("runSDK() stderr names %q:\n%s", absent, errOut.String())
			}
		})
	}
	t.Run("the notes service is listed as not reached", func(t *testing.T) {
		if !strings.Contains(out.String(), "    not reached  NotesService.Create\n") {
			t.Errorf("runSDK() stdout lacks the notes service:\n%s", out.String())
		}
	})
	raw, err := os.ReadFile(path) //#nosec G304 -- a file under the test's own temporary directory
	if err != nil {
		t.Fatalf("read the report: %v", err)
	}
	var written sdkReport
	if decoded := json.Unmarshal(raw, &written); decoded != nil {
		t.Fatalf("decode the report: %v", decoded)
	}
	for _, testCase := range []struct {
		name      string
		got, want any
	}{
		{name: "the positions reached", got: written.Check.Coverage.Reached, want: 4},
		{name: "the positions asked", got: written.Check.Coverage.Asked, want: 2},
		{name: "the positions traversed", got: written.Check.Coverage.Traversed, want: 2},
		{name: "the reached functions", got: len(written.Check.Reached), want: 2},
		{name: "the project's send is the project service's", got: written.Check.Reached[1].Method, want: "ProjectsService.GetProject"},
		{name: "the tier oracle's reason is this run's", got: written.Check.TierOracleReason, want: sdkTierOracleReason()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("the report holds %v, want %v", testCase.got, testCase.want)
			}
		})
	}
	for _, field := range written.Sent {
		t.Run("the grain of "+field.SchemaType+"."+field.Field, func(t *testing.T) {
			if field.Grain != sdkGrain {
				t.Errorf("the sent row is read at %q, want the run's grain %q", field.Grain, sdkGrain)
			}
		})
	}
}

// TestRunSDK_WhenTheReportCannotBeWritten_SaysWhyAndFails verifies that a
// -report path the run cannot write ends the run with the reason, and names
// no report it did not write.
func TestRunSDK_WhenTheReportCannotBeWritten_SaysWhyAndFails(t *testing.T) {
	status, out, errOut := runSDKFixture(t, "", sdkDeclared, false, filepath.Join(t.TempDir(), "no-such-directory", "sdk.json"))

	if status != 1 {
		t.Fatalf("runSDK() = %d, want 1; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	if !strings.Contains(errOut, "write the client-go sent report") {
		t.Errorf("runSDK() stderr = %q, want it to name the write it could not do", errOut)
	}
	if strings.Contains(out, "sdk.json") {
		t.Errorf("runSDK() stdout names a report it did not write:\n%s", out)
	}
}

// TestRunSDK_WhenTheRunCannotStart_SaysWhyAndFails verifies each way a
// client-go run ends before judging anything: a schema it cannot read, a
// client-go it cannot find, a client-go source that does not type-check, and
// a tree of ours that cannot be loaded.
func TestRunSDK_WhenTheRunCannotStart_SaysWhyAndFails(t *testing.T) {
	root := sdkFixtureRoot(t, "", "callers")
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	broken := fixtureModule(t, map[string]string{"bad": "package bad\n\nfunc use() { undefinedHelper() }\n"})
	for _, testCase := range []struct {
		name      string
		schema    string
		patterns  []string
		directory func(string) (string, error)
		want      string
	}{
		{
			name: "a schema file that does not exist", schema: filepath.Join(t.TempDir(), "missing.graphql"),
			want: "read the schema to judge against",
		},
		{
			name: "no client-go to read", directory: func(string) (string, error) { return "", errors.New("no client-go here") },
			want: "no client-go here",
		},
		{
			name: "a client-go that does not type-check", directory: func(string) (string, error) { return filepath.Join(broken, "bad"), nil },
			want: "load the client-go source in " + filepath.Join(broken, "bad"),
		},
		{
			name: "a tree of ours that cannot be loaded", patterns: []string{"./empty/..."},
			want: "no packages matched ./empty/...",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.directory != nil {
				original := sdkDirectory
				sdkDirectory = testCase.directory
				t.Cleanup(func() { sdkDirectory = original })
			}
			cfg := auditRun{dir: root, patterns: []string{"./callers/..."}, schemaPath: schemaFile(t, sdkSchema), sdk: true}
			if testCase.schema != "" {
				cfg.schemaPath = testCase.schema
			}
			if testCase.patterns != nil {
				cfg.patterns = testCase.patterns
			}
			var out, errOut bytes.Buffer

			status := runSDK(cfg, &out, &errOut)

			if status != 1 {
				t.Errorf("runSDK() = %d, want 1", status)
			}
			if !strings.Contains(errOut.String(), testCase.want) {
				t.Errorf("runSDK() stderr = %q, want it to name %q", errOut.String(), testCase.want)
			}
		})
	}
}

// TestRunMain_SDK_HoldsTheRunToTheCommittedTables verifies what -sdk hands
// the run: the client-go half, this repository's patterns, and the committed
// tables, every entry of which is stale against a stand-in that answers none
// of the real client-go's findings, which is exactly what a run handed the
// real tables and not a fixture's must report.
func TestRunMain_SDK_HoldsTheRunToTheCommittedTables(t *testing.T) {
	want := len(declaredSDK.shapes) + len(declaredSDK.sends) + len(declaredSDK.sent)
	if want == 0 {
		t.Fatal("the committed client-go tables are empty, so a run handed them cannot be told from one handed nothing")
	}
	root := sdkFixtureRoot(t, "", "internal")
	var out, errOut bytes.Buffer

	status := runMain([]string{"-sdk", "-dir", root, "-schema", schemaFile(t, sdkSchema)}, &out, &errOut)

	if status != 1 {
		t.Fatalf("runMain() = %d, want 1; stdout:\n%s\nstderr:\n%s", status, out.String(), errOut.String())
	}
	if got := strings.Count(errOut.String(), "\nsdk_declarations.go: ") + strings.Count(errOut.String(), "\nsdk_declarations_sent.go: "); got != want {
		t.Errorf("runMain() reported %d stale declaration(s), want the %d the committed tables hold:\n%s", got, want, errOut.String())
	}
	t.Run("a sent declaration is named in the file that keeps it", func(t *testing.T) {
		if got := strings.Count(errOut.String(), "\nsdk_declarations_sent.go: "); got != len(declaredSDK.sent) {
			t.Errorf("runMain() named %d stale sent declaration(s) in sdk_declarations_sent.go, want %d", got, len(declaredSDK.sent))
		}
	})
	if !strings.Contains(errOut.String(), ", 4 pairing(s) judged at the sends this server reaches, ") {
		t.Errorf("runMain() did not read the callers under ./internal/...:\n%s", errOut.String())
	}
}
