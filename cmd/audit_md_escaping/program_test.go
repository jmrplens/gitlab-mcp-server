package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// fixtureDir is the directory the in-memory fixture packages pretend to live
// in. Nothing is written there: the packages exist only in the loader overlay,
// which keeps generated Go source out of the repository while still
// type-checking it against the real toolutil.
const fixtureDir = "cmd/audit_md_escaping/fixture"

// fixturePattern matches every fixture package at once.
const fixturePattern = "./" + fixtureDir + "/..."

// fixturePatterns load the fixture together with the package that owns the
// escaping helpers.
//
// toolutil is loaded from source rather than left to export data because the
// audit's answers depend on its bodies: FormatTime returns its argument
// verbatim when neither layout parses, and that one fallback is what a
// hundred and fifty call sites in this repository hang on. A fixture that
// mocked it would test the mock.
var fixturePatterns = []string{fixturePattern, "./internal/toolutil"}

// repoRoot walks up from the test's working directory to the module root, so
// the overlay can name absolute paths inside the module and the loader
// resolves the module's own import paths.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// fixtureOverlay turns a map of repository-relative fixture path to source
// into a loader overlay rooted at the module.
func fixtureOverlay(t *testing.T, sources map[string]string) map[string][]byte {
	t.Helper()
	root := repoRoot(t)
	overlay := make(map[string][]byte, len(sources))
	for name, source := range sources {
		overlay[filepath.Join(root, filepath.FromSlash(fixtureDir), filepath.FromSlash(name))] = []byte(source)
	}
	return overlay
}

// fixtureCache memoizes one loaded program per fixture source set. Loading is
// a full type-check of the fixture against toolutil, the result is read-only
// for everything the tests ask of it, and the tests run in one goroutine, so
// paying for it once keeps the package's tests from re-parsing the same few
// packages for every case.
var fixtureCache = map[string]*program{}

// loadFixture loads the fixture packages described by sources.
func loadFixture(t *testing.T, sources map[string]string) *program {
	t.Helper()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	key := strings.Join(names, "|")
	if cached, ok := fixtureCache[key]; ok {
		return cached
	}
	prog, err := loadProgram(repoRoot(t), fixturePatterns, fixtureOverlay(t, sources))
	if err != nil {
		t.Fatalf("loadProgram: %v", err)
	}
	fixtureCache[key] = prog
	return prog
}

// fixturePackage returns one loaded fixture package by name.
func fixturePackage(t *testing.T, prog *program, name string) *packages.Package {
	t.Helper()
	for _, pkg := range prog.order {
		if strings.HasSuffix(pkg.PkgPath, "/"+name) {
			return pkg
		}
	}
	t.Fatalf("fixture package %s was not loaded", name)
	return nil
}

// caseFixture is the fixture the audit tests share. Every package in it is
// written the way a real domain is written, so the classifier is exercised on
// the shapes it has to handle rather than on a mock of them.
var caseFixture = map[string]string{
	"mdcase/mdcase.go":     mdcaseSource,
	"mdcase/extra.go":      mdcaseExtraSource,
	"mdsafe/mdsafe.go":     mdsafeSource,
	"mdexempt/mdexempt.go": mdexemptSource,
}

// mdcaseSource holds the shapes a formatter gets wrong, one of each: a value
// in a heading, in a cell, in a list item, in both halves of a hand-built
// link, in a cell builder with no template, and the shapes the audit answers
// with something other than a verdict.
const mdcaseSource = `package mdcase

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_md_escaping/fixture/mdsafe"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Item is the shape a GitLab response fills.
type Item struct {
	Title  string
	URL    string
	State  string
	Count  int
	Labels []string
	Author *string
	Extra  any
}

// Pair is a struct callers build positionally.
type Pair struct {
	Left  string
	Right string
}

const headingTemplate = "## %s\n"

// packageRow is written outside any function, so a finding about it names no
// enclosing formatter.
var packageRow = fmt.Sprintf("| %s |\n", mutableTitle)

var mutableTitle string

// FormatOutputMarkdown is registered by value, the way every Markdown
// formatter in this repository is, so nothing calls it.
func FormatOutputMarkdown(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, headingTemplate, item.Title)
	fmt.Fprintf(&b, "| %s | %d |\n", item.State, item.Count)
	fmt.Fprintf(&b, "- %s\n", item.Labels[0])
	fmt.Fprintf(&b, "| [%[1]s](%[2]s) |\n", item.Title, item.URL)
	fmt.Fprintf(&b, "| %q |\n", item.Title)
	b.WriteString(toolutil.MarkdownTableRow(item.Title, toolutil.EscapeMdTableCell(item.State)))
	b.WriteString(prose(item))
	b.WriteString(packageRow)
	return b.String()
}

// prose renders a paragraph, where a pipe and an angle bracket change nothing.
func prose(item Item) string {
	return fmt.Sprintf("%s wrote %s\n", item.Title, item.State)
}

// FormatLinked escapes a title into a local and then builds a link out of the
// local and a raw URL, which is how a cell ends up holding an unescaped
// destination.
func FormatLinked(item Item) string {
	name := toolutil.EscapeMdTableCell(item.Title)
	if item.URL != "" {
		name = fmt.Sprintf("[%s](%s)", name, item.URL)
	}
	return fmt.Sprintf("| %s |\n", name)
}

// FormatRow is the shared row helper: one caller passes an escaped title and
// another passes a raw one, so the reason names the call site.
func FormatRow(title string) string {
	return fmt.Sprintf("| %s |\n", title)
}

// RenderRows calls it both ways.
func RenderRows(item Item) string {
	return FormatRow(toolutil.EscapeMdTableCell(item.Title)) + FormatRow(item.State)
}

// FormatRecursive reaches itself, which the walk has to cut without calling
// the value safe.
func FormatRecursive(item Item, depth int) string {
	return fmt.Sprintf("| %s |\n", repeat(item.Title, depth))
}

func repeat(s string, n int) string {
	if n <= 0 {
		return s
	}
	return repeat(s, n-1)
}

// FormatSpread spreads a slice of cells into the row builder.
func FormatSpread(item Item) string {
	cells := []string{toolutil.EscapeMdTableCell(item.Title), item.State}
	return toolutil.MarkdownTableRow(cells...)
}

// FormatDynamic builds its template at run time, so it is not a sink at all.
func FormatDynamic(item Item, width int) string {
	template := "| %-" + strconv.Itoa(width) + "s |\n"
	return fmt.Sprintf(template, item.Title)
}

// FormatShapes writes the value shapes the walk has a rule for, one each.
func FormatShapes(item Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s |\n", "opened by "+item.Title)
	fmt.Fprintf(&b, "| %s |\n", item.Title+" (open)")
	fmt.Fprintf(&b, "| %s |\n", *item.Author)
	fmt.Fprintf(&b, "| %s |\n", item.Extra.(string))
	fmt.Fprintf(&b, "| %s |\n", item.Title[:8])
	fmt.Fprintf(&b, "| %v |\n", []string{item.Title})
	fmt.Fprintf(&b, "| %v |\n", map[string]string{"title": item.Title})
	fmt.Fprintf(&b, "| %s |\n", strings.Join([]string{item.State, "open"}, ", "))
	fmt.Fprintf(&b, "| %s |\n", fmt.Sprintf(dynamicTemplate(), item.Title))
	fmt.Fprintf(&b, "| %s |\n", (*(&item)).Title)
	fmt.Fprintf(&b, "| %s |\n", mdsafe.DefaultLabel)
	fmt.Fprintf(&b, "| %s |\n", emptyTemplate)
	return b.String()
}

// dynamicTemplate is a template the audit cannot read, even inside a call it
// otherwise follows.
func dynamicTemplate() string {
	return "| %s |"
}

// FormatThroughClosure formats inside a function literal, whose parameters no
// call site of a declared function binds.
func FormatThroughClosure() func(string) string {
	return func(title string) string {
		return fmt.Sprintf("| %s |\n", title)
	}
}

const emptyTemplate = ""

// FormatEmptyTemplate writes a constant template with nothing in it.
func FormatEmptyTemplate(b *strings.Builder) {
	fmt.Fprintf(b, emptyTemplate)
}

// FormatUnnamed takes a value it does not name, which the index of parameters
// has to step over.
func FormatUnnamed(Item) string {
	return "| unnamed |\n"
}

// FormatPair reads two results into two names, which the audit follows to
// neither.
func FormatPair() string {
	var left, right = split("a|b")
	return fmt.Sprintf("| %s | %s |\n", left, right)
}
`

// mdcaseExtraSource is a second file of the same package, so the walk that
// names the enclosing formatter has more than one file to choose from.
const mdcaseExtraSource = `package mdcase

import "fmt"

// FormatThroughValue renders through a function it was handed, which the audit
// cannot name.
func FormatThroughValue(render func(Item) string, item Item) string {
	return fmt.Sprintf("| %s |\n", render(item))
}

// FormatDelegated hands its whole value to a helper that reads a field of it.
// The helper's parameter is bound to this call's argument, so the field is
// answered by where that argument came from rather than by every caller of the
// helper.
func FormatDelegated(item Item) string {
	return fmt.Sprintf("| %s |\n", delegated(item))
}

func delegated(it Item) string {
	return it.Title
}

// FormatPositional reads a field of a struct built positionally.
func FormatPositional() string {
	pair := Pair{"a|b", "c"}
	return fmt.Sprintf("| %s |\n", pair.Left)
}

// FormatFieldAssign fills a field after building the struct, which must not
// read as a literal that left every field empty.
func FormatFieldAssign(item Item) string {
	pair := Pair{}
	pair.Right = item.Title
	return fmt.Sprintf("| %s |\n", pair.Right)
}

// FormatMulti takes one of two results, which the audit does not follow.
func FormatMulti(item Item) string {
	value, _ := split(item.Title)
	return fmt.Sprintf("| %s |\n", value)
}

func split(s string) (string, string) {
	return s, s
}

// FormatNamedResult returns through a named result.
func FormatNamedResult(item Item) string {
	return fmt.Sprintf("| %s |\n", namedResult(item))
}

func namedResult(item Item) (out string) {
	out = item.Title
	return
}

// FormatVariadic interpolates a variadic parameter its caller never fills.
func FormatVariadic(prefix string, rest ...string) string {
	return fmt.Sprintf("| %s |\n", rest[0])
}

// CallVariadic is that caller.
func CallVariadic() string {
	return FormatVariadic("x")
}
`

// mdsafeSource holds the shapes that must not be reported: every escaper, the
// non-textual types, a helper whose returns are all safe, a nested Sprintf, a
// standard-library transform, an options struct built at the call site, and a
// lookup in a table the server wrote.
const mdsafeSource = `package mdsafe

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Item is the shape a GitLab response fills.
type Item struct {
	Title string
	URL   string
	Count int
	When  time.Time
}

// Options is the shape a caller builds at the call site.
type Options struct {
	Title  string
	Column string
}

const rowTemplate = "| %s | %s |\n"

var icons = map[string]string{"open": "ok"}

// DefaultLabel is a value this package wrote, which another package reads
// through its name rather than out of a struct.
var DefaultLabel = "none"

// FormatItem renders an item with every value already made safe.
func FormatItem(item Item, opts Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n", toolutil.EscapeMdHeading(item.Title))
	fmt.Fprintf(&b, rowTemplate, toolutil.EscapeMdTableCell(item.Title), toolutil.MdTitleLink(item.Title, item.URL))
	fmt.Fprintf(&b, "| %v | %v |\n", item.Count, item.When)
	fmt.Fprintf(&b, "| %s |\n", strconv.Itoa(item.Count))
	fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(toolutil.EscapeMdTableCell(item.Title)))
	fmt.Fprintf(&b, "| %s | %s |\n", opts.Title, opts.Column)
	fmt.Fprintf(&b, "| %s |\n", DefaultLabel)
	fmt.Fprintf(&b, "| %s |\n", label(item))
	fmt.Fprintf(&b, "| %s |\n", fmt.Sprintf("%s (%s)", toolutil.EscapeMdTableCell(item.Title), "server text"))
	fmt.Fprintf(&b, "| %s |\n", toolutil.FormatTime("2024-01-01"))
	fmt.Fprintf(&b, "| %s |\n", string(rune(item.Count)))
	b.WriteString(toolutil.MarkdownTableHeader("Name", "State"))
	b.WriteString(toolutil.MarkdownTableRow(toolutil.EscapeMdTableCell(item.Title), statusIcon(item)))
	return b.String()
}

// label returns an escaped cell whichever branch it takes.
func label(item Item) string {
	if item.Title == "" {
		return "-"
	}
	return toolutil.EscapeMdTableCell(item.Title)
}

// statusIcon looks the icon up in a table the server wrote.
func statusIcon(item Item) string {
	if icon, ok := icons[item.Title]; ok {
		return icon
	}
	return "-"
}

// Render builds the options at the call site, which is what answers for their
// fields.
func Render(item Item) string {
	return FormatItem(item, Options{Title: "Items"})
}
`

// mdexemptSource holds a value declared safe in the source, one declared safe
// that no longer reaches a Markdown construct, and two malformed directives
// that are not read as exemptions at all.
const mdexemptSource = `package mdexempt

import (
	"fmt"
	"strings"
)

// Result is the shape the action catalog fills, which is not GitLab-authored
// text at all.
type Result struct {
	ID    string
	Title string
}

//gitlab:allow-unescaped result.ID: a canonical catalog ID, compiled in from an ActionSpec rather than read from GitLab.
//gitlab:allow-unescaped result.Retired: nothing interpolates this any more.
//gitlab:allow-unescaped result.Title
//gitlab:allow-unescaped : a reason with nothing to excuse
func FormatResult(result Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | %s |\n", result.ID, result.Title)
	return b.String()
}
`

// brokenFixture is a package that does not type-check, which the loader has to
// refuse rather than audit half of.
var brokenFixture = map[string]string{
	"mdbroken/mdbroken.go": `package mdbroken

func Broken() string {
	return undefinedHelper()
}
`,
}

// TestLoadProgram_Fixture_IndexesDeclarationsAndCalls checks that a loaded
// program can answer both questions the classifier asks of it: what a declared
// function's body is, and where that function is called.
func TestLoadProgram_Fixture_IndexesDeclarationsAndCalls(t *testing.T) {
	prog := loadFixture(t, caseFixture)

	if len(prog.order) != 4 {
		t.Fatalf("loaded %d packages, want the 3 fixture packages and toolutil", len(prog.order))
	}
	var found, called bool
	for fn, decl := range prog.decls {
		if fn.Name() != "FormatRow" {
			continue
		}
		found = true
		if decl.decl.Body == nil {
			t.Errorf("FormatRow indexed without a body")
		}
		if len(prog.callers[fn]) != 2 {
			t.Errorf("FormatRow has %d recorded call sites, want 2", len(prog.callers[fn]))
		}
		called = true
	}
	if !found || !called {
		t.Errorf("FormatRow not indexed: found=%v called=%v", found, called)
	}
}

// TestLoadProgram_MissingDirectory_Fails checks that a root the loader cannot
// enter is reported rather than read as an empty tree.
func TestLoadProgram_MissingDirectory_Fails(t *testing.T) {
	_, err := loadProgram(filepath.Join(t.TempDir(), "absent"), []string{"./..."}, nil)
	if err == nil {
		t.Fatal("loadProgram accepted a directory that does not exist")
	}
}

// TestLoadProgram_NoMatch_Fails checks that a pattern matching nothing fails,
// since a gate that audited no package would otherwise pass.
func TestLoadProgram_NoMatch_Fails(t *testing.T) {
	_, err := loadProgram(repoRoot(t), []string{modulePath + "/internal/nothing-here/..."}, nil)
	if err == nil {
		t.Fatal("loadProgram accepted a pattern that matched nothing")
	}
	if !strings.Contains(err.Error(), "no packages matched") {
		t.Errorf("error %q does not say the pattern matched nothing", err)
	}
}

// TestLoadProgram_BrokenPackage_Fails checks that a package that does not
// type-check stops the run. Auditing it would classify every value as one the
// audit cannot follow, which is the one failure mode a gate must not have.
func TestLoadProgram_BrokenPackage_Fails(t *testing.T) {
	_, err := loadProgram(repoRoot(t), fixturePatterns, fixtureOverlay(t, brokenFixture))
	if err == nil {
		t.Fatal("loadProgram accepted a package that does not type-check")
	}
	if !strings.Contains(err.Error(), "mdbroken") {
		t.Errorf("error %q does not name the package that failed to load", err)
	}
}

// TestProgramPosition_Fixture_ResolvesAFile checks that a position renders
// with the file the expression was written in.
func TestProgramPosition_Fixture_ResolvesAFile(t *testing.T) {
	prog := loadFixture(t, caseFixture)
	sinks := collectSinks(prog)
	if len(sinks) == 0 {
		t.Fatal("no sinks collected from the fixture")
	}
	pos := prog.position(sinks[0].call.Pos())
	if !strings.HasSuffix(filepath.ToSlash(pos.Filename), ".go") || pos.Line == 0 {
		t.Errorf("position %s is not a source line", pos)
	}
}
