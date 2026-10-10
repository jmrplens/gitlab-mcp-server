package main

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assembledFixture sends documents the way client-go writes six of its own:
// the format of a fmt.Sprintf call, named and written inline, and the text a
// text/template set renders into a strings.Builder or a bytes.Buffer, one of
// them parsed into a set cloned from another. Every decoder agrees with the
// rendering, so a run that pairs each one passes.
const assembledFixture = `package assembled

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"fixture/gql"
	"fixture/shared"
)

const nameFormat = @@query { project(fullPath: %q) { name } }@@

var nameTemplate = template.Must(template.New("name").Parse(@@query { project(fullPath: "x") { name } }@@))

var starsTemplate = template.Must(template.Must(nameTemplate.Clone()).New("stars").Parse((@@query { project(fullPath: "x") { stars } }@@)))

type nameResponse struct {
	Data struct {
		Project *struct {
			Name string @@json:"name"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

func formatted(service gql.Service, path string) {
	var resp nameResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: fmt.Sprintf(nameFormat, path)}, &resp)
}

func formattedInline(service gql.Service, path string) {
	var resp struct {
		Data struct {
			Project *struct {
				ID string @@json:"id"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	query := gql.GraphQLQuery{Query: fmt.Sprintf(@@query { project(fullPath: %q) { id } }@@, path)}
	_, _ = service.Do(query, &resp)
}

func executed(service gql.Service) {
	var builder strings.Builder
	if err := nameTemplate.Execute(&builder, nil); err != nil {
		return
	}
	var resp nameResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func buffered(service gql.Service) {
	var buffer bytes.Buffer
	_ = starsTemplate.Execute(&buffer, nil)
	var resp struct {
		Data struct {
			Project *struct {
				Stars int @@json:"stars"@@
			} @@json:"project"@@
		} @@json:"data"@@
	}
	_, _ = service.Do(gql.GraphQLQuery{Query: buffer.String()}, &resp)
}

func sharedFormat(service gql.Service, path string) {
	var resp nameResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: fmt.Sprintf(shared.Format, path)}, &resp)
}

func sharedTemplate(service gql.Service) {
	var builder strings.Builder
	_ = len(builder.String())
	_ = shared.Template.Execute(&builder, nil)
	var resp nameResponse
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}
`

// sharedFixture declares a format and a template set another package fills
// and executes, which that package names through a qualified identifier.
const sharedFixture = `package shared

import "text/template"

const Format = @@query { project(fullPath: %q) { name } }@@

var Template = template.Must(template.New("shared").Parse(@@query { project(fullPath: "y") { name } }@@))
`

// TestRun_FixtureWhoseDocumentsAreAssembled_JudgesWhatTheCallSends verifies
// that a document written as a fmt.Sprintf format or as a text/template the
// method executes is paired with its decoder through the rendering the
// document inventory makes of it, under the name the call reaches it by: the
// format's constant, a format written inline, or the package variable the
// template set is held in, whether the package that sends it declares it or
// names it in another.
//
// Before this, each of these was a document built at run time, which is what
// every client-go method holding one of those documents reported, and a
// decoder nobody pairs with its document is a decoder judged by nobody.
func TestRun_FixtureWhoseDocumentsAreAssembled_JudgesWhatTheCallSends(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"assembled": assembledFixture, "shared": sharedFixture}, true)

	if status != 0 {
		t.Fatalf("run() = %d, want 0; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, want := range []string{
		"    ok  fixture/assembled nameFormat (assembled/assembled.go:29)\n",
		"    ok  fixture/assembled an inline document (assembled/assembled.go:41)\n",
		"    ok  fixture/assembled nameTemplate (assembled/assembled.go:50)\n",
		"    ok  fixture/assembled starsTemplate (assembled/assembled.go:63)\n",
		"    ok  fixture/assembled Format (assembled/assembled.go:68)\n",
		"    ok  fixture/assembled Template (assembled/assembled.go:76)\n",
		"audit_graphql_shapes: 6 pairing(s) agree with their documents, 0 selection(s) nothing reads (",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("run() stdout lacks %q:\n%s", want, out)
			}
		})
	}
}

// brokenAssemblyFixture is every way an assembled document cannot be paired,
// each of which must be reported with its reason rather than read as a
// document built at run time for no stated cause: a format the inventory
// could not render, a format that is not a document, a builder no template
// writes and one two templates write, a template held in a local, one built by
// cloning rather than parsed, one declared with no value, one parsed from text
// that is not a document, and one whose text reads the data it is executed
// with. A builder reached through a field is not followed at all.
const brokenAssemblyFixture = `package broken

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"fixture/gql"
)

type point struct{ X int }

type holder struct{ b strings.Builder }

const pointFormat = @@query { project(fullPath: "%v") { name } }@@

var base = template.Must(template.New("base").Parse(@@query { project(fullPath: "x") { name } }@@))

var cloned = template.Must(base.Clone())

var alias = base

var unset *template.Template

var text = strings.ToLower("QUERY")

var fromText = template.Must(template.New("t").Parse(text))

var readsData = template.Must(template.New("d").Parse(@@query { project(fullPath: "{{ .Path }}") { name } }@@))

type response struct {
	Data struct {
		Project *struct {
			Name string @@json:"name"@@
		} @@json:"project"@@
	} @@json:"data"@@
}

func unrendered(service gql.Service, at point) {
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: fmt.Sprintf(pointFormat, at)}, &resp)
}

func notADocument(service gql.Service, format string) {
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: fmt.Sprintf(format, "x")}, &resp)
}

func neverExecuted(service gql.Service, writers chan io.Writer, other *holder) {
	var builder, second strings.Builder
	_ = base.Execute(os.Stdout, nil)
	_ = base.Execute(<-writers, nil)
	_ = base.Execute(&other.b, nil)
	_ = base.Execute(&second, nil)
	fmt.Println(builder.Len())
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func executedTwice(service gql.Service) {
	var builder strings.Builder
	_ = base.Execute(&builder, nil)
	_ = base.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func fromLocal(service gql.Service) {
	local := base
	var builder strings.Builder
	_ = local.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func fromClone(service gql.Service) {
	var builder strings.Builder
	_ = cloned.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func fromAlias(service gql.Service) {
	var builder strings.Builder
	_ = alias.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func fromUnset(service gql.Service) {
	var builder strings.Builder
	_ = unset.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func fromNoDocument(service gql.Service) {
	var builder strings.Builder
	_ = fromText.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func fromData(service gql.Service) {
	var builder strings.Builder
	_ = readsData.Execute(&builder, struct{ Path string }{"x"})
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func throughField(service gql.Service, h *holder) {
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: h.b.String()}, &resp)
}

func makeTemplate() *template.Template { return base }

func fromCall(service gql.Service) {
	var builder strings.Builder
	_ = makeTemplate().Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}

func sendsBase(service gql.Service) {
	var builder strings.Builder
	_ = base.Execute(&builder, nil)
	var resp response
	_, _ = service.Do(gql.GraphQLQuery{Query: builder.String()}, &resp)
}
`

// TestRun_FixtureWhoseAssembledDocumentsCannotBePaired_SaysWhyForEach
// verifies that each way an assembled document fails to resolve is a problem
// naming its own cause, at the send it is about, and that the run fails on
// them. The last send of the fixture pairs, so the run is held to report the
// others rather than to have found nothing to judge.
func TestRun_FixtureWhoseAssembledDocumentsCannotBePaired_SaysWhyForEach(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"broken": brokenAssemblyFixture}, true)

	if status != 1 {
		t.Fatalf("run() = %d, want 1; stdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, want := range []string{
		"fixture/broken (broken/broken.go:43): the document is a shell fmt.Sprintf fills, and fmt.Sprintf formats at, a fixture/broken.point, which this walk has no stand-in for\n",
		"\nfixture/broken (broken/broken.go:48): the document is built at run time, from fmt.Sprintf(format, \"x\"), so nothing can be judged before a request is made\n",
		"\nfixture/broken (broken/broken.go:59): the document is what builder holds, and 0 text/template Execute calls in this function write it, where exactly one says which template it is\n",
		"\nfixture/broken (broken/broken.go:67): the document is what builder holds, and 2 text/template Execute calls in this function write it, where exactly one says which template it is\n",
		"\nfixture/broken (broken/broken.go:75): the document is what builder holds, and the template it is written from, local, is not a package variable, so its set is built when a function runs\n",
		"\nfixture/broken (broken/broken.go:82): the document is what builder holds, and cloned is built with base.Clone() rather than parsed from one text\n",
		"\nfixture/broken (broken/broken.go:89): the document is what builder holds, and alias is built with base rather than parsed from one text\n",
		"\nfixture/broken (broken/broken.go:96): the document is what builder holds, and unset is declared with no value this audit reads\n",
		"\nfixture/broken (broken/broken.go:103): the document is what builder holds, and the text fromText parses is not a document the inventory reads\n",
		"\nfixture/broken (broken/broken.go:110): the document is a shell text/template fills, and it reads {{.Path}}, a value client-go fills when the method runs\n",
		"\nfixture/broken (broken/broken.go:115): the document is built at run time, from h.b.String(), so nothing can be judged before a request is made\n",
		"\nfixture/broken (broken/broken.go:124): the document is what builder holds, and the template it is written from, makeTemplate(), is not a package variable, so its set is built when a function runs\n",
		"    ok  fixture/broken base (broken/broken.go:131)\n",
	} {
		t.Run(strings.TrimSpace(want), func(t *testing.T) {
			if !strings.Contains(errOut+out, want) {
				t.Errorf("run() output lacks %q:\nstdout:\n%s\nstderr:\n%s", want, out, errOut)
			}
		})
	}
}

// TestLoadProgram_TreeThatCannotBeWalked_IsRefused verifies the two refusals
// the loader has beside a directory that is not a module: a package that does
// not type-check, which would fold no constant and type no call, and a pattern
// that matches nothing, which is the shape a mistyped pattern produces and
// must not become a clean run over nothing.
func TestLoadProgram_TreeThatCannotBeWalked_IsRefused(t *testing.T) {
	root := fixtureModule(t, map[string]string{"bad": "package bad\n\nfunc use() { undefinedHelper() }\n"})
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	cases := []struct {
		name     string
		patterns []string
		want     string
	}{
		{name: "a package that does not type-check", patterns: []string{"./bad/..."}, want: "load fixture/bad: "},
		{name: "a pattern matching nothing", patterns: []string{"./empty/..."}, want: "no packages matched ./empty/..."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loaded, err := loadProgram(root, testCase.patterns)

			if err == nil {
				t.Fatalf("loadProgram() error = nil, want one naming %q", testCase.want)
			}
			if loaded != nil {
				t.Errorf("loadProgram() = %v, want nil on failure", loaded)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("loadProgram() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestComparePairings_OrdersByTheCallThenByWhereTheDocumentCameFrom verifies
// the order the report is read in, at each of the four levels that decide it
// and in both directions. A pairing comes first when the comparison is
// negative.
//
// The order matters because a pairing is collected in whatever order the walk
// reaches it (a document handed over through a wrapper is completed after
// every direct send, so the slice arrives unsorted) and two runs over one
// tree have to print the same list. Each level is checked both ways round,
// since a comparator that answers one direction correctly and the other by
// accident sorts correctly only on input that was already sorted.
func TestComparePairings_OrdersByTheCallThenByWhereTheDocumentCameFrom(t *testing.T) {
	at := func(file string, line, column int) token.Position {
		return token.Position{Filename: file, Line: line, Column: column}
	}
	first := at("a.go", 10, 4)
	for _, testCase := range []struct {
		name  string
		left  pairing
		right pairing
		want  bool
	}{
		{
			name:  "an earlier file comes first",
			left:  pairing{Position: at("a.go", 99, 9)},
			right: pairing{Position: at("b.go", 1, 1)},
			want:  true,
		},
		{
			name:  "a later file does not",
			left:  pairing{Position: at("b.go", 1, 1)},
			right: pairing{Position: at("a.go", 99, 9)},
			want:  false,
		},
		{
			name:  "within one file an earlier line comes first",
			left:  pairing{Position: at("a.go", 3, 40)},
			right: pairing{Position: at("a.go", 9, 1)},
			want:  true,
		},
		{
			name:  "within one file a later line does not",
			left:  pairing{Position: at("a.go", 9, 1)},
			right: pairing{Position: at("a.go", 3, 40)},
			want:  false,
		},
		{
			name:  "two sends on one line are ordered by column",
			left:  pairing{Position: at("a.go", 9, 6)},
			right: pairing{Position: at("a.go", 9, 40)},
			want:  true,
		},
		{
			name:  "and the later column does not come first",
			left:  pairing{Position: at("a.go", 9, 40)},
			right: pairing{Position: at("a.go", 9, 6)},
			want:  false,
		},
		{
			name:  "one call reached from two hand-overs is ordered by where the document came from",
			left:  pairing{Position: first, Origin: at("caller.go", 2, 2)},
			right: pairing{Position: first, Origin: at("caller.go", 7, 2)},
			want:  true,
		},
		{
			name:  "and the later hand-over does not come first",
			left:  pairing{Position: first, Origin: at("caller.go", 7, 2)},
			right: pairing{Position: first, Origin: at("caller.go", 2, 2)},
			want:  false,
		},
		{
			name:  "a pairing is not before itself",
			left:  pairing{Position: first},
			right: pairing{Position: first},
			want:  false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := comparePairings(testCase.left, testCase.right) < 0; got != testCase.want {
				t.Errorf("comparePairings(%v@%v, %v@%v) < 0 = %t, want %t",
					testCase.left.Position, testCase.left.Origin,
					testCase.right.Position, testCase.right.Origin, got, testCase.want)
			}
		})
	}
}
