package graphqldocs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// formatsFixture writes every shape of argument a format string's holes are
// filled from: a value of each basic kind, a field of a struct, constants of
// each kind, and a format declared under a name rather than written inline.
const formatsFixture = `package formats

import "fmt"

type options struct{ Path string }

// byArgument takes each hole's value from an argument the caller holds.
func byArgument(opt options, iid int64, size uint, archived bool, weight float64) string {
	return fmt.Sprintf(@@query ByArgument { project(fullPath: %q) { issue(iid: "%d") { id size(min: %d) archived(is: %t) weight(over: %g) } } }@@,
		opt.Path, iid, size, archived, weight)
}

// byConstant takes each hole's value from a constant written at the call.
func byConstant() string {
	return fmt.Sprintf(@@query ByConstant { project(fullPath: %q) { issue(iid: "%d") { id done(is: %t) weight(over: %g) } } }@@,
		"group/project", 7, false, 2.5)
}

// namedFormat is a format declared under a name and handed to the call by it.
const namedFormat = @@query ByName { project(fullPath: %q) { id } }@@

func byName(projectFullPath string) string { return fmt.Sprintf(namedFormat, projectFullPath) }

// conversion is a call the walk passes over: it fills no document.
func conversion(text string) []byte { return []byte(text) }
`

// templatesFixture writes a text/template set the way client-go writes its
// work item documents: a fragment cloned into another, cloned into the
// operation, each a package variable built from the one before it.
const templatesFixture = `package templates

import "text/template"

const userFields = @@id
username@@

var userTemplate = template.Must(template.New("User").Parse(userFields))

var itemTemplate = template.Must(template.Must(userTemplate.Clone()).New("Item").Parse(@@id
author { {{ template "User" }} }@@))

var getItemTemplate = template.Must(template.Must(itemTemplate.Clone()).New("GetItem").Parse(@@query GetItem($path: ID!) { namespace(fullPath: $path) { workItem(iid: "1") { {{ template "Item" }} } } }@@))

var byUserTemplate = template.Must(template.Must((userTemplate).Clone()).New("ByUser").Parse(@@query ByUser { currentUser { {{- template "User" -}} } }@@))

// wholeTemplate is parsed as a template and has no hole, so it is a document
// written whole and nothing renders it.
var wholeTemplate = template.Must(template.New("Whole").Parse(@@query Whole { currentUser { id } }@@))
`

// unrenderedFixture writes every shape a shell is left unrendered in, each
// under an operation name the test finds it by.
const unrenderedFixture = `package unrendered

import (
	"fmt"
	"text/template"
)

type options struct{ Path string }

func spread(values []any) string {
	return fmt.Sprintf(@@query Spread { project(fullPath: %q) { id } }@@, values...)
}

func structValue(opt options) string {
	return fmt.Sprintf(@@query Struct { project(fullPath: %q) { id } }@@, opt)
}

func complexValue() string {
	return fmt.Sprintf(@@query Complex { project(weight: %v) { id } }@@, 2i)
}

func complexVariable(weight complex128) string {
	return fmt.Sprintf(@@query ComplexVariable { project(weight: %v) { id } }@@, weight)
}

type holder struct{ set *template.Template }

var held holder

func viaField() *template.Template {
	return template.Must(held.set.New("ViaField").Parse(@@query ViaField { currentUser { {{ template "User" }} } }@@))
}

func mismatch(path string) string {
	return fmt.Sprintf(@@query Mismatch { issue(iid: %d) { id } }@@, path)
}

const twoSites = @@query TwoSites { project(fullPath: %q) { id } }@@

func first(a string) string  { return fmt.Sprintf(twoSites, a) }
func second(b string) string { return fmt.Sprintf(twoSites, b) }

const unused = @@query Unused { project(fullPath: %q) { id } }@@

var userTemplate = template.Must(template.New("User").Parse("id"))

var readsData = template.Must(template.New("ReadsData").Parse(@@query ReadsData($path: ID!{{ if .Decls }}, {{ .Decls }}{{ end }}) { id }@@))

var handsValue = template.Must(template.Must(userTemplate.Clone()).New("HandsValue").Parse(@@query HandsValue { currentUser { {{ template "User" . }} } }@@))

var missing = template.Must(template.New("Missing").Parse(@@query Missing { currentUser { {{ template "Nowhere" }} } }@@))

var broken = template.Must(template.New("Broken").Parse(@@query Broken { currentUser { {{ end }} } }@@))

func local() *template.Template {
	var t = template.Must(userTemplate.Clone())
	return template.Must(t.New("Local").Parse(@@query Local { currentUser { {{ template "User" }} } }@@))
}

var recursive = template.Must(template.New("Recursive").Parse(@@query Recursive { currentUser { {{ template "Recursive" }} } }@@))

var withFuncs = template.Must(template.New("Funcs").Funcs(template.FuncMap{}).Parse(@@query Funcs { currentUser { {{ template "User" }} } }@@))

func namedAtRunTime(name string) *template.Template {
	return template.Must(template.New(name).Parse(@@query Dynamic { currentUser { {{ template "User" }} } }@@))
}

var declaredOnly *template.Template

func fromDeclaredOnly() *template.Template {
	return template.Must(declaredOnly.New("DeclaredOnly").Parse(@@query DeclaredOnly { currentUser { {{ template "User" }} } }@@))
}

func fragmentText() string { return "id" }

var fromValue = template.Must(template.New("Value").Parse(fragmentText()))

var nonConstant = template.Must(template.Must(fromValue.Clone()).New("NonConstant").Parse(@@query NonConstant { currentUser { {{ template "Value" }} } }@@))

var mixed = template.Must(template.Must(userTemplate.Clone()).New("Mixed").Parse(@@query Mixed { project(fullPath: %q) { {{ template "User" }} } }@@))
`

// shellFixtures are the three packages the rendering tests load together.
var shellFixtures = map[string]string{
	"formats":    formatsFixture,
	"templates":  templatesFixture,
	"unrendered": unrenderedFixture,
}

// operationName finds the name an operation is declared under in a document,
// rendered or not.
var operationName = regexp.MustCompile(`(?:query|mutation) ([A-Za-z]+)`)

// byOperation indexes documents by the name of the operation they declare.
func byOperation(t *testing.T, documents []Document) map[string]Document {
	t.Helper()
	indexed := map[string]Document{}
	for _, document := range documents {
		match := operationName.FindStringSubmatch(document.Text)
		if match == nil {
			t.Fatalf("a fixture document declares no named operation: %q", document.Text)
		}
		indexed[match[1]] = document
	}
	return indexed
}

// TestFromPackages_AFormatString_IsRenderedWithAStandInPerHole verifies a
// document client-go writes as the format of a fmt.Sprintf call comes back as
// the text the call produces, every hole filled from what the call hands it:
// a constant's own value, and for a value only a caller holds, a stand-in of
// its type, a string spelled as the expression it came from so a reader of
// the rendering can see where each value would go.
func TestFromPackages_AFormatString_IsRenderedWithAStandInPerHole(t *testing.T) {
	documents := byOperation(t, loadFixture(t, shellFixtures))

	cases := []struct {
		operation string
		want      string
	}{
		{
			operation: "ByArgument",
			want:      `query ByArgument { project(fullPath: "opt_Path") { issue(iid: "1") { id size(min: 1) archived(is: true) weight(over: 1.5) } } }`,
		},
		{
			operation: "ByConstant",
			want:      `query ByConstant { project(fullPath: "group/project") { issue(iid: "7") { id done(is: false) weight(over: 2.5) } } }`,
		},
		{operation: "ByName", want: `query ByName { project(fullPath: "projectFullPath") { id } }`},
	}
	for _, testCase := range cases {
		t.Run(testCase.operation, func(t *testing.T) {
			document := documents[testCase.operation]
			if document.Text != testCase.want {
				t.Errorf("text = %q, want %q", document.Text, testCase.want)
			}
			if document.Assembly == nil || document.Assembly.By != AssembledByFormat || document.Assembly.Unrendered != "" {
				t.Fatalf("assembly = %+v, want a rendered format string", document.Assembly)
			}
			if !strings.Contains(document.Assembly.Shell, "%q") {
				t.Errorf("shell = %q, want the format as the source writes it", document.Assembly.Shell)
			}
			if IsTemplate(document) {
				t.Errorf("IsTemplate() = true for the rendering %q", document.Text)
			}
		})
	}
	t.Run("a document declared under a name keeps it", func(t *testing.T) {
		if documents["ByName"].Name != "namedFormat" {
			t.Errorf("name = %q, want namedFormat", documents["ByName"].Name)
		}
	})
}

// TestFromPackages_ATemplate_IsRenderedThroughTheVariablesItIsClonedFrom
// verifies a text/template document comes back as what executing it produces,
// with every template it names taken from the package variables its set was
// cloned out of, two levels deep, a parenthesized receiver included, and with
// the trim markers a template may carry applied as text/template applies them.
func TestFromPackages_ATemplate_IsRenderedThroughTheVariablesItIsClonedFrom(t *testing.T) {
	documents := byOperation(t, loadFixture(t, shellFixtures))

	cases := []struct {
		operation string
		want      string
	}{
		{
			operation: "GetItem",
			want:      "query GetItem($path: ID!) { namespace(fullPath: $path) { workItem(iid: \"1\") { id\nauthor { id\nusername } } } }",
		},
		{operation: "ByUser", want: "query ByUser { currentUser {id\nusername} }"},
	}
	for _, testCase := range cases {
		t.Run(testCase.operation, func(t *testing.T) {
			document := documents[testCase.operation]
			if document.Text != testCase.want {
				t.Errorf("text = %q, want %q", document.Text, testCase.want)
			}
			if document.Assembly == nil || document.Assembly.By != AssembledByTemplate || document.Assembly.Unrendered != "" {
				t.Fatalf("assembly = %+v, want a rendered template", document.Assembly)
			}
			if !strings.Contains(document.Assembly.Shell, "{{") {
				t.Errorf("shell = %q, want the template as the source writes it", document.Assembly.Shell)
			}
		})
	}
	t.Run("a document with no hole is left as it is", func(t *testing.T) {
		whole := documents["Whole"]
		if whole.Assembly != nil || whole.Text != "query Whole { currentUser { id } }" {
			t.Errorf("document = %+v, want the text as written and no assembly", whole)
		}
	})
}

// TestFromPackages_AShellNoCallRenders_KeepsItsHolesAndSaysWhy covers every
// way a shell stays one. Each keeps the text the source writes, so a caller
// asking IsTemplate still counts it apart, and says why in words a reader can
// act on: a rendering with a hole guessed at would be judged as text GitLab
// never receives, which is worse than a shell named as one.
func TestFromPackages_AShellNoCallRenders_KeepsItsHolesAndSaysWhy(t *testing.T) {
	documents := byOperation(t, loadFixture(t, shellFixtures))

	cases := []struct {
		operation string
		by        string
		reason    string
	}{
		{operation: "Spread", by: AssembledByFormat, reason: "spread into the call"},
		{operation: "Struct", by: AssembledByFormat, reason: "opt, a "},
		{operation: "Complex", by: AssembledByFormat, reason: "2i, a "},
		{operation: "ComplexVariable", by: AssembledByFormat, reason: "weight, a complex128"},
		{operation: "ViaField", by: AssembledByTemplate, reason: "comes from held.set, which this walk does not follow"},
		{operation: "Mismatch", by: AssembledByFormat, reason: "%!d(string=path)"},
		{operation: "TwoSites", by: AssembledByFormat, reason: "its 2 call sites render it 2 ways"},
		{operation: "Unused", reason: "no fmt.Sprintf call and no text/template Parse"},
		{operation: "ReadsData", by: AssembledByTemplate, reason: "{{if .Decls}}"},
		{operation: "HandsValue", by: AssembledByTemplate, reason: `hands the template "User" a value`},
		{operation: "Missing", by: AssembledByTemplate, reason: `the template "Nowhere"`},
		{operation: "Broken", by: AssembledByTemplate, reason: "does not parse"},
		{operation: "Local", by: AssembledByTemplate, reason: "t is not a package variable"},
		{operation: "Recursive", by: AssembledByTemplate, reason: `the template "Recursive" does not execute`},
		{operation: "Funcs", by: AssembledByTemplate, reason: "Funcs"},
		{operation: "Dynamic", by: AssembledByTemplate, reason: "name, which is not a constant"},
		{operation: "DeclaredOnly", by: AssembledByTemplate, reason: "declaredOnly is declared with no value"},
		{operation: "NonConstant", by: AssembledByTemplate, reason: "fragmentText(), which is not a constant"},
		{operation: "Mixed", by: AssembledByTemplate, reason: "still carries %q"},
	}
	for _, testCase := range cases {
		t.Run(testCase.operation, func(t *testing.T) {
			document := documents[testCase.operation]
			if document.Assembly == nil {
				t.Fatalf("document %q carries no assembly", document.Text)
			}
			if document.Text != document.Assembly.Shell || !IsTemplate(document) {
				t.Errorf("text = %q, want the shell %q kept as it is", document.Text, document.Assembly.Shell)
			}
			if document.Assembly.By != testCase.by {
				t.Errorf("by = %q, want %q", document.Assembly.By, testCase.by)
			}
			if !strings.Contains(document.Assembly.Unrendered, testCase.reason) {
				t.Errorf("unrendered = %q, want it to say %q", document.Assembly.Unrendered, testCase.reason)
			}
		})
	}
	// The six rendered or whole documents of the two other tests are the rest.
	if len(documents) != len(cases)+6 {
		t.Errorf("the fixtures hold %d documents, want %d: a case was added without a row", len(documents), len(cases)+6)
	}
}

// The two packages of acrossFixtures that hand a format from one to the
// other: declaring declares it, filling formats it through a qualified
// identifier, which is the shape a document shared between client-go's
// packages would take.
const declaringFixture = `package declaring

// Query is a format a function of another package fills.
const Query = @@query CrossPackage { project(fullPath: %q) { id } }@@
`

// fillingFixture formats declaringFixture's document.
const fillingFixture = `package filling

import (
	"fmt"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs/fixture/declaring"
)

func fill(path string) string { return fmt.Sprintf(declaring.Query, path) }
`

// shadowFixture calls a function value spelled fmt.Sprintf that is not
// fmt.Sprintf: a package variable named fmt whose Sprintf field returns the
// format untouched, so what this package sends is the shell as written.
const shadowFixture = `package shadow

type printer struct{ Sprintf func(format string, values ...any) string }

var fmt = printer{Sprintf: func(format string, _ ...any) string { return format }}

func shadowed(path string) string {
	return fmt.Sprintf(@@query Shadowed { project(fullPath: %q) { name } }@@, path)
}
`

// acrossFixtures are the packages the cross-package and spelling cases load
// together, apart from shellFixtures so that test's count of its own
// documents stays its own.
var acrossFixtures = map[string]string{
	"declaring": declaringFixture,
	"filling":   fillingFixture,
	"shadow":    shadowFixture,
}

// TestFromPackages_AFormatAnotherPackageDeclares_IsJoinedByItsObject covers
// the two ways a call could be misread as filling a document, or missed as
// one. A format declared in one package and filled through a qualified
// identifier in another is the same constant, so the call has to reach it by
// the object the identifier names rather than by where the identifier is
// written. And a call that is spelled fmt.Sprintf but reaches a function value
// is not fmt.Sprintf, so its document is a shell no call this walk reads takes
// rather than a rendering of a call that never happens.
func TestFromPackages_AFormatAnotherPackageDeclares_IsJoinedByItsObject(t *testing.T) {
	documents := byOperation(t, loadFixture(t, acrossFixtures))

	t.Run("a format filled through a qualified identifier", func(t *testing.T) {
		document := documents["CrossPackage"]
		const want = `query CrossPackage { project(fullPath: "path") { id } }`
		if document.Text != want {
			t.Errorf("text = %q, want %q", document.Text, want)
		}
		if document.Assembly == nil || document.Assembly.By != AssembledByFormat || document.Assembly.Unrendered != "" {
			t.Errorf("assembly = %+v, want a rendered format string", document.Assembly)
		}
	})
	t.Run("a function value spelled like fmt.Sprintf", func(t *testing.T) {
		document := documents["Shadowed"]
		if document.Assembly == nil || document.Assembly.By != "" {
			t.Fatalf("assembly = %+v, want no call named as the one that fills it", document.Assembly)
		}
		if document.Text != document.Assembly.Shell || !strings.Contains(document.Assembly.Unrendered, "no fmt.Sprintf call") {
			t.Errorf("document = %+v, want the shell kept and the reason saying no call takes it", document)
		}
	})
}

// TestSDKDirectory_IsTheClientGoTheModuleBuildsAgainst verifies the directory
// is the one the repository's own go.mod resolves, and that a module which
// does not require client-go is refused rather than answered with nothing.
func TestSDKDirectory_IsTheClientGoTheModuleBuildsAgainst(t *testing.T) {
	t.Run("this repository", func(t *testing.T) {
		dir, err := SDKDirectory(repoRoot(t))
		if err != nil {
			t.Fatalf("SDKDirectory() error = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "workitems.go")); statErr != nil {
			t.Errorf("SDKDirectory() = %q, which holds no workitems.go: %v", dir, statErr)
		}
	})
	t.Run("a module that does not require it", func(t *testing.T) {
		// Under -mod=mod the toolchain would add whatever client-go the module
		// cache holds rather than refuse, which is the answer this pins out.
		t.Setenv("GOFLAGS", "-mod=mod")
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		got, err := SDKDirectory(dir)
		if err == nil || got != "" {
			t.Fatalf("SDKDirectory() = %q, %v; want a refusal", got, err)
		}
		if !strings.Contains(err.Error(), SDKImportPath) {
			t.Errorf("error = %q, want it to name %s", err, SDKImportPath)
		}
	})
	t.Run("a directory the toolchain cannot run in", func(t *testing.T) {
		if _, err := SDKDirectory(filepath.Join(t.TempDir(), "nowhere")); err == nil {
			t.Error("SDKDirectory() reported no error for a directory that does not exist")
		}
	})
}

// clientGoStandIn is a GitLab that keeps every document client-go posts and
// answers each with just enough for the method to send what follows it. It
// runs on the httptest server's goroutine, so it records and never stops the
// test.
type clientGoStandIn struct {
	t         *testing.T
	mu        sync.Mutex
	documents []string
}

// ServeHTTP records the document and answers it.
func (s *clientGoStandIn) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var payload struct {
		Query string `json:"query"`
	}
	body, err := io.ReadAll(req.Body)
	if err == nil {
		err = json.Unmarshal(body, &payload)
	}
	if err != nil {
		s.t.Errorf("the stand-in could not read a request to %s: %v", req.URL.Path, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.documents = append(s.documents, payload.Query)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(payload.Query, "GetWorkItemID") {
		_, _ = io.WriteString(w, `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`)
		return
	}
	_, _ = io.WriteString(w, `{"data":{}}`)
}

// sent returns the documents posted so far.
func (s *clientGoStandIn) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.documents)
}

// TestSDKDocuments_TheShellsItRenders_AreWhatClientGoSends holds the
// rendering to the only oracle that cannot be wrong about it: client-go
// itself, at the version the module requires. Every method whose document is
// a shell is called against a stand-in, with each string argument spelled the
// way the rendering spells its stand-in, and every rendered document has to be
// byte for byte one of the documents posted. A client-go bump that adds a
// shell fails here until the method that posts it is called below, and one
// that changes a template fails here until the rendering follows it.
//
// The work item list is the shell that stays one, and it is pinned as such:
// its fragment is parsed from the fields a caller asks for, inside a function,
// and its variables are the filters a caller sets, so no rendering exists
// without a caller.
func TestSDKDocuments_TheShellsItRenders_AreWhatClientGoSends(t *testing.T) {
	dir, err := SDKDirectory(repoRoot(t))
	if err != nil {
		t.Fatalf("SDKDirectory() error = %v", err)
	}
	documents, err := SDKDocuments(dir)
	if err != nil {
		t.Fatalf("SDKDocuments() error = %v", err)
	}

	standIn := &clientGoStandIn{t: t}
	server := httptest.NewServer(standIn)
	t.Cleanup(server.Close)
	client, err := gl.NewClient("token", gl.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("build the client: %v", err)
	}
	// The methods' own answers are beside the point: what they posted is what
	// the rendering is held to.
	_, _, _ = client.WorkItems.GetWorkItem("fullPath", 1)
	_, _, _ = client.WorkItems.CreateWorkItem("fullPath", "gid://gitlab/WorkItems::Type/1", &gl.CreateWorkItemOptions{Title: "t"})
	_, _, _ = client.WorkItems.UpdateWorkItem("fullPath", 1, &gl.UpdateWorkItemOptions{})
	_, _, _ = client.TerraformStates.List("projectFullPath")
	_, _, _ = client.TerraformStates.Get("projectFullPath", "name")
	const shellsDriven = 5

	sent := standIn.sent()
	var rendered, unrendered []Document
	for _, document := range documents {
		switch {
		case document.Assembly == nil:
		case document.Assembly.Unrendered == "":
			rendered = append(rendered, document)
		default:
			unrendered = append(unrendered, document)
		}
	}
	if len(rendered) != shellsDriven {
		t.Errorf("%d documents were rendered and %d methods are driven; call the method that posts each", len(rendered), shellsDriven)
	}
	for _, document := range rendered {
		t.Run(fmt.Sprintf("%s at %s:%d", document.Label(), filepath.Base(document.Position.Filename), document.Position.Line), func(t *testing.T) {
			if !slices.Contains(sent, document.Text) {
				t.Errorf("client-go posted none of its %d documents as\n%s", len(sent), document.Text)
			}
		})
	}
	t.Run("the work item list stays a shell", func(t *testing.T) {
		if len(unrendered) != 1 || unrendered[0].Name != "listWorkItemsQueryShell" || unrendered[0].Assembly.By != AssembledByTemplate {
			t.Fatalf("unrendered = %+v, want the work item list shell alone", unrendered)
		}
		if !strings.Contains(unrendered[0].Assembly.Unrendered, "is not a package variable") {
			t.Errorf("unrendered = %q, want it to say its template set is built at run time", unrendered[0].Assembly.Unrendered)
		}
	})
}
