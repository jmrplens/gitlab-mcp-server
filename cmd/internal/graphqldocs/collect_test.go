package graphqldocs

import (
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// fixtureDir is the directory the in-memory fixture packages pretend to live
// in. Nothing is written there: the packages exist only in the loader overlay,
// which keeps generated Go source out of the repository while still
// type-checking it for real, which is what folds a document assembled from a
// shared fragment into the one string GitLab would receive.
const fixtureDir = "cmd/internal/graphqldocs/fixture"

// fixturePattern matches every fixture package at once.
const fixturePattern = "./" + fixtureDir + "/..."

// backtickPlaceholder stands in for a backtick inside a fixture source, which
// is itself written as a raw string literal and so cannot contain one.
const backtickPlaceholder = "@@"

// repoRoot walks up from the test's working directory to the module root, so
// the fixture overlay can name absolute paths inside the module.
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

// fixtureOverlay turns a map of package name to source into a loader overlay
// rooted at the module. Each entry becomes one file in its own package
// directory under [fixtureDir].
func fixtureOverlay(t *testing.T, sources map[string]string) map[string][]byte {
	t.Helper()
	root := repoRoot(t)
	overlay := make(map[string][]byte, len(sources))
	for name, source := range sources {
		path := filepath.Join(root, filepath.FromSlash(fixtureDir), name, name+".go")
		overlay[path] = []byte(strings.ReplaceAll(source, backtickPlaceholder, "`"))
	}
	return overlay
}

// docsFixture is the fixture the collector tests share. It is written the way
// a real domain is written, including the document assembled by concatenating
// a shared fragment, which is the shape a regular expression over the source
// cannot resolve and four of this repository's documents actually use.
const docsFixture = `package docs

import "strings"

// declaredWithoutValue is the shape a walk over declarations has to survive:
// names and values do not line up, because there are no values.
var declaredWithoutValue string

// assembled is not a constant, so nothing can be folded out of it: a document
// built at run time is the transport's business, not this audit's.
var assembled = strings.TrimSpace("prefix-") + declaredWithoutValue

// sharedFields is a fragment of a selection set, not a document: it has braces
// but no operation, so the filter must leave it alone.
const sharedFields = @@
  id
  title
  author {
    name
  }
@@

const listQuery = @@
query($path: ID!) {
  project(fullPath: $path) {
    issues {
      nodes {@@ + sharedFields + @@
      }
    }
  }
}
@@

// notADocument is prose that mentions a mutation and carries braces.
const notADocument = "the mutation failed: %s {see the log}"

// payload is JSON, which opens with the same character a bare selection set
// does.
const payload = @@{"id":1,"name":"x"}@@

// inlineWriter names a document where it is used rather than declaring it.
func inlineWriter() (string, string) {
	return @@
mutation($id: ID!) {
  issueSetLocked(input: {id: $id}) {
    errors
  }
}
@@, @@
query {
  currentUser {
    id
  }
}
@@
}

// closureWriter hides a document inside a value a declaration is initialized
// with, so the walk has to descend into a declaration rather than skip it.
var closureWriter = func() string {
	return @@
query {
  metadata {
    version
  }
}
@@
}
`

// otherFixture is a second package, so the ordering of findings across
// packages is exercised rather than assumed.
const otherFixture = `package other

const alsoAQuery = @@
query {
  currentUser {
    name
  }
}
@@
`

// loadFixture collects the documents in a fixture package set.
func loadFixture(t *testing.T, sources map[string]string) []Document {
	t.Helper()
	found, err := Collect(repoRoot(t), []string{fixturePattern}, fixtureOverlay(t, sources))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return found
}

// names returns each collected document's label, sorted, for comparison.
func names(documents []Document) []string {
	labels := make([]string, 0, len(documents))
	for _, found := range documents {
		labels = append(labels, found.Label())
	}
	sort.Strings(labels)
	return labels
}

// TestCollect_EveryShapeADocumentIsWrittenIn_IsFoundExactlyOnce verifies the
// collector against the four shapes this repository uses, and against the
// strings that look like documents and are not.
func TestCollect_EveryShapeADocumentIsWrittenIn_IsFoundExactlyOnce(t *testing.T) {
	found := loadFixture(t, map[string]string{"docs": docsFixture, "other": otherFixture})

	got := names(found)
	want := []string{"alsoAQuery", "an inline document", "an inline document", "an inline document", "listQuery"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("collected %v, want %v", got, want)
	}
	t.Run("findings are ordered by package", func(t *testing.T) {
		if !strings.HasSuffix(found[0].Package, "/docs") || !strings.HasSuffix(found[len(found)-1].Package, "/other") {
			t.Errorf("collected in package order %q .. %q, want docs before other", found[0].Package, found[len(found)-1].Package)
		}
	})

	byLabel := map[string]Document{}
	for _, one := range found {
		byLabel[one.Label()] = one
	}
	t.Run("the fragment is spliced into the named document", func(t *testing.T) {
		if !strings.Contains(byLabel["listQuery"].Text, "author {") {
			t.Errorf("listQuery was collected without its fragment:\n%s", byLabel["listQuery"].Text)
		}
	})
	t.Run("the package is recorded", func(t *testing.T) {
		if !strings.HasSuffix(byLabel["listQuery"].Package, "/docs") {
			t.Errorf("package = %q, want the fixture package", byLabel["listQuery"].Package)
		}
	})
	t.Run("the position points at the declaration", func(t *testing.T) {
		if byLabel["listQuery"].Position.Line == 0 {
			t.Error("the collected document has no position")
		}
	})
}

// objectsFixture declares one document as a variable and one under the blank
// identifier, which is still an object and still nothing any body can name.
const objectsFixture = `package objects

var declaredMutation = @@
mutation($id: ID!) {
  thing(input: {id: $id}) { errors }
}
@@

const _ = @@
query {
  currentUser {
    id
  }
}
@@
`

// TestCollect_DocumentsWithAndWithoutADefiningObject_CarryTheRightOne verifies
// the field a caller resolving documents through the type checker joins on.
//
// cmd/audit_readonly_graphql reaches a document through the object a handler's
// call graph names, so a document declared under a name has to carry that
// object and a document nothing names has to carry none. Guessing from the
// label instead would put a .graphql file, whose label is a file name, in the
// same bucket as a constant.
func TestCollect_DocumentsWithAndWithoutADefiningObject_CarryTheRightOne(t *testing.T) {
	found := loadFixture(t, map[string]string{"objects": objectsFixture, "docs": docsFixture})

	byLabel := map[string]Document{}
	for _, one := range found {
		byLabel[one.Label()] = one
	}
	cases := []struct {
		name       string
		label      string
		wantObject string
	}{
		{name: "a document declared as a variable", label: "declaredMutation", wantObject: "declaredMutation"},
		{name: "a document declared as a constant", label: "listQuery", wantObject: "listQuery"},
		// The type checker gives even a blank declaration an object, so this is
		// a named document like any other. Nothing can name it back, which is
		// a fact about reachability and not about the inventory.
		{name: "a document bound to the blank identifier", label: "_", wantObject: "_"},
		{name: "a document written where it is used", label: "an inline document"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document, ok := byLabel[testCase.label]
			if !ok {
				t.Fatalf("the fixture no longer collects a document labeled %q", testCase.label)
			}
			if testCase.wantObject == "" {
				if document.Object != nil {
					t.Errorf("Object = %v, want none for %s", document.Object, testCase.label)
				}
				return
			}
			if document.Object == nil {
				t.Fatalf("Object = nil, want the object %q is declared as", testCase.label)
			}
			if got := document.Object.Name(); got != testCase.wantObject {
				t.Errorf("Object.Name() = %q, want %q", got, testCase.wantObject)
			}
		})
	}
}

// repeatedFixture declares a document in a grouped const block and repeats the
// declaration implicitly on the line below it, which is the one shape where
// the text a name holds is on the object and nowhere in the declaration. The
// integer pair below it is the same shape carrying something that is not a
// document, so the fold is asked the question rather than the shape answering
// it.
const repeatedFixture = `package repeated

const (
	dismissMutation = @@
mutation($id: ID!) {
  vulnerabilityDismiss(input: {id: $id}) { errors }
}
@@
	resolveMutation
)

const (
	first = 1
	second
)
`

// TestCollect_AConstantAGroupedDeclarationRepeats_IsInTheInventory verifies the
// declaration shape whose text exists only on the type-checked object.
//
// A grouped const block repeats the previous line's expression implicitly, so
// the repeating line's ValueSpec carries a name and no value at all. Reading
// only the expressions written in the declaration drops such a document out of
// the inventory, and it leaves no trace anywhere else either: the object it is
// declared as is then an object no document in the inventory carries, so
// cmd/audit_readonly_graphql, which joins on exactly that, cannot report it as
// unattributed. A mutation could ship classified by nobody.
func TestCollect_AConstantAGroupedDeclarationRepeats_IsInTheInventory(t *testing.T) {
	found := loadFixture(t, map[string]string{"repeated": repeatedFixture})

	byLabel := map[string]Document{}
	for _, one := range found {
		byLabel[one.Label()] = one
	}
	cases := []struct {
		name  string
		label string
	}{
		{name: "the line that writes the document", label: "dismissMutation"},
		{name: "the line that repeats it implicitly", label: "resolveMutation"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document, ok := byLabel[testCase.label]
			if !ok {
				t.Fatalf("%s is not in the inventory; collected %v", testCase.label, names(found))
			}
			if !strings.Contains(document.Text, "vulnerabilityDismiss") {
				t.Errorf("Text = %q, want the mutation the group declares", document.Text)
			}
			if document.Object == nil {
				t.Fatalf("Object = nil, want the constant %q is declared as", testCase.label)
			}
			if got := document.Object.Name(); got != testCase.label {
				t.Errorf("Object.Name() = %q, want %q", got, testCase.label)
			}
		})
	}
	t.Run("a repeated constant that is not a document is left alone", func(t *testing.T) {
		if _, ok := byLabel["second"]; ok {
			t.Error("a repeated integer constant was collected as a GraphQL document")
		}
	})
}

// TestFromPackages_NoPackages_CollectsNothing verifies the in-source half
// survives being handed nothing. Every load this repository does refuses an
// empty result before it gets here, and an exported function that indexes the
// first package must not depend on that promise being kept by its callers.
func TestFromPackages_NoPackages_CollectsNothing(t *testing.T) {
	if found := FromPackages(nil); found != nil {
		t.Errorf("FromPackages(nil) = %v, want nothing", found)
	}
}

// TestCollect_LoadFailures_AreReportedRatherThanSilentlyEmpty verifies that a
// run which could not read the source says so. A partially typed package folds
// no constants, so every document in it would go unseen, which is exactly the
// failure this audit must not have.
func TestCollect_LoadFailures_AreReportedRatherThanSilentlyEmpty(t *testing.T) {
	const broken = `package broken

const q = "query { x }"

func use() { undefinedHelper() }
`
	cases := []struct {
		name     string
		dir      string
		patterns []string
		overlay  map[string][]byte
		want     string
	}{
		{
			name:     "a directory that is not a module",
			dir:      filepath.Join(t.TempDir(), "nowhere"),
			patterns: []string{"./..."},
			want:     "load packages",
		},
		{
			name:     "a package that does not type-check",
			dir:      repoRoot(t),
			patterns: []string{fixturePattern},
			overlay:  fixtureOverlay(t, map[string]string{"broken": broken}),
			want:     "load ",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			found, err := Collect(testCase.dir, testCase.patterns, testCase.overlay)

			if err == nil {
				t.Fatalf("Collect() error = nil, want one naming %q", testCase.want)
			}
			if found != nil {
				t.Errorf("Collect() documents = %v, want nil on failure", found)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Collect() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestCollect_PatternMatchingNothing_IsReported verifies the guard against an
// audit that looks at the wrong place and reports a clean run. docs/ is a real
// directory of this repository that holds no Go at all, which is exactly the
// shape a mistyped pattern produces.
func TestCollect_PatternMatchingNothing_IsReported(t *testing.T) {
	found, err := Collect(repoRoot(t), []string{"./docs/..."}, nil)

	if err == nil {
		t.Fatalf("Collect() error = nil and found %d document(s), want the empty-match refusal", len(found))
	}
	if !strings.Contains(err.Error(), "no packages matched") {
		t.Errorf("Collect() error = %q, want it to say nothing matched", err)
	}
}

// TestDocumentLabel_UnnamedDocument_SaysSo verifies the label a report files an
// inline document under, since there is no constant name to print.
func TestDocumentLabel_UnnamedDocument_SaysSo(t *testing.T) {
	cases := []struct {
		name string
		doc  Document
		want string
	}{
		{name: "declared under a name", doc: Document{Name: "queryListThings"}, want: "queryListThings"},
		{name: "written where it is used", doc: Document{}, want: "an inline document"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.doc.Label(); got != testCase.want {
				t.Errorf("Label() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestStandalone_DocumentsInFilesOfTheirOwn_AreFoundAndThePinIsNot verifies the half
// of the inventory that does not go through the type checker.
//
// A go:embed variable is not a constant, so a document moved into its own file
// folds to nothing and would leave the inventory in silence: the audit would
// report one fewer document and still exit 0. Reading the files directly closes
// that. The pinned schema is the one .graphql file that must be skipped, since
// it is an SDL and not a document anybody sends.
func TestStandalone_DocumentsInFilesOfTheirOwn_AreFoundAndThePinIsNot(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "internal", "tools", "customemoji")
	if err := os.MkdirAll(tree, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	write := func(dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
	}
	write(tree, "create.graphql", "mutation($p: ID!) {\n  createCustomEmoji(input: {groupPath: $p}) { errors }\n}\n")
	write(tree, "helper.go", "package customemoji\n")
	write(tree, "notes.txt", "mutation { nothing }\n")
	write(filepath.Join(root, "internal"), graphqlschema.SDLFileName, "type Query {\n  ok: Boolean\n}\n")

	found, err := Standalone(root, []string{"./internal/..."})
	if err != nil {
		t.Fatalf("Standalone() error = %v, want nil", err)
	}
	if len(found) != 1 {
		t.Fatalf("Standalone() found %d document(s), want 1: %+v", len(found), found)
	}
	if found[0].Name != "create.graphql" {
		t.Errorf("Standalone() named the document %q, want %q", found[0].Name, "create.graphql")
	}
	if !strings.Contains(found[0].Text, "createCustomEmoji") {
		t.Errorf("Standalone() read %q, want the document's text", found[0].Text)
	}
	if found[0].Position.Filename == "" || found[0].Position.Line != 1 {
		t.Errorf("Standalone() positioned the document at %+v, want its file at line 1", found[0].Position)
	}
	if found[0].Object != nil {
		t.Errorf("Standalone() carried object %v, want none: a file of its own is declared by nothing, "+
			"which is what cmd/audit_readonly_graphql reports rather than resolves", found[0].Object)
	}
}

// TestStandalone_ATreeThatIsNotThere_IsNotAnError verifies that a pattern
// naming a directory which does not exist is left to the package loader, which
// has already answered it. Complaining twice about one mistyped pattern helps
// nobody.
func TestStandalone_ATreeThatIsNotThere_IsNotAnError(t *testing.T) {
	found, err := Standalone(t.TempDir(), []string{"./nowhere/..."})
	if err != nil {
		t.Fatalf("Standalone() error = %v, want nil", err)
	}
	if len(found) != 0 {
		t.Errorf("Standalone() found %+v, want nothing", found)
	}
}

// TestStandalone_ARootThatIsNotADirectory_Fails verifies that the two ways a
// walk root can be unusable are told apart. A root that is simply absent is a
// question about the patterns, which the package loader answers; anything else
// is a tree the audit was asked to read and could not, and skipping that would
// be the silence this pass exists to remove.
func TestStandalone_ARootThatIsNotADirectory_Fails(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "internal"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	found, err := Standalone(root, []string{"./internal/..."})

	if err == nil {
		t.Fatal("Standalone() error = nil, want the unusable root")
	}
	if found != nil {
		t.Errorf("Standalone() returned %+v, want nothing on failure", found)
	}
	if !strings.Contains(err.Error(), "open ") {
		t.Errorf("Standalone() error = %q, want it to name the root it could not open", err)
	}
}

// TestStandalone_AFileItCannotRead_Fails verifies that a document the audit
// could not read stops the run. Skipping it would be the exact silence the
// standalone-file pass was added to remove.
func TestStandalone_AFileItCannotRead_Fails(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "internal")
	if err := os.MkdirAll(tree, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	// A dangling symlink is the portable way to make the read fail: WalkDir
	// reports it as an ordinary entry without following it, and the read then
	// finds nothing there. A mode-0 file would not do, since the suite runs as
	// root in CI and root reads it anyway.
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(tree, "dangling.graphql")); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	_, err := Standalone(root, []string{"./internal/..."})

	if err == nil {
		t.Fatal("Standalone() error = nil, want the read failure")
	}
	if !strings.Contains(err.Error(), "read ") {
		t.Errorf("Standalone() error = %q, want it to name the file it could not read", err)
	}
}

// TestWalkRoots_TurnsLoadPatternsIntoDirectories verifies that both halves of
// the inventory search the same tree, since a mismatch would leave standalone
// documents unjudged wherever the two disagreed.
func TestWalkRoots_TurnsLoadPatternsIntoDirectories(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    string
	}{
		{name: "a recursive pattern", pattern: "./internal/...", want: filepath.Join("/repo", "internal")},
		{name: "one package", pattern: "./internal/tools", want: filepath.Join("/repo", "internal", "tools")},
		// The expectation goes through filepath.Join like its neighbors, because
		// walkRoots joins the directory with the pattern and Windows spells the
		// result "\repo".
		{name: "the whole module", pattern: "./...", want: filepath.Join("/repo")},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			roots := walkRoots("/repo", []string{testCase.pattern})

			if len(roots) != 1 || roots[0] != testCase.want {
				t.Errorf("walkRoots() = %v, want [%s]", roots, testCase.want)
			}
		})
	}
}

// TestCollect_AStandaloneDocumentItCannotRead_StopsBeforeTypeChecking verifies
// that a file the audit cannot read ends the run, and ends it before the
// expensive half. Skipping it would be the silence this pass exists to remove.
func TestCollect_AStandaloneDocumentItCannotRead_StopsBeforeTypeChecking(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "dangling.graphql")); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	found, err := Collect(root, []string{"./..."}, nil)

	if err == nil {
		t.Fatal("Collect() error = nil, want the read failure")
	}
	if found != nil {
		t.Errorf("Collect() returned %+v, want nothing on failure", found)
	}
	if !strings.Contains(err.Error(), "read ") {
		t.Errorf("Collect() error = %q, want it to name the file it could not read", err)
	}
}

// TestSortDocuments_OrdersByPackageThenFileThenPosition verifies the order
// findings are reported in.
//
// The filename is part of the key because a package holds documents from
// several files, and for a standalone .graphql document every offset is the
// same: without it, two such documents in one package would be ordered by
// nothing at all and a re-run could report them either way round.
func TestSortDocuments_OrdersByPackageThenFileThenPosition(t *testing.T) {
	documents := []Document{
		{Package: "b/pkg", Name: "second package", Position: token.Position{Filename: "b.go"}},
		{Package: "a/pkg", Name: "later in the same file", Position: token.Position{Filename: "a.go", Offset: 90}},
		{Package: "a/pkg", Name: "second file", Position: token.Position{Filename: "z.graphql"}},
		{Package: "a/pkg", Name: "first file", Position: token.Position{Filename: "a.go", Offset: 10}},
	}

	sortDocuments(documents)

	got := make([]string, 0, len(documents))
	for _, found := range documents {
		got = append(got, found.Name)
	}
	want := []string{"first file", "later in the same file", "second file", "second package"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sortDocuments() = %v, want %v", got, want)
	}
}
