package main

import (
	"go/token"
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
// type-checking it for real, which is what folds a document assembled from a
// shared fragment into the one string GitLab would receive.
const fixtureDir = "cmd/audit_graphql_documents/fixture"

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
func loadFixture(t *testing.T, sources map[string]string) []document {
	t.Helper()
	found, err := collect(repoRoot(t), []string{fixturePattern}, fixtureOverlay(t, sources))
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return found
}

// names returns each collected document's label, sorted, for comparison.
func names(documents []document) []string {
	labels := make([]string, 0, len(documents))
	for _, found := range documents {
		labels = append(labels, found.label())
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
		if !strings.HasSuffix(found[0].pkg, "/docs") || !strings.HasSuffix(found[len(found)-1].pkg, "/other") {
			t.Errorf("collected in package order %q .. %q, want docs before other", found[0].pkg, found[len(found)-1].pkg)
		}
	})

	byLabel := map[string]document{}
	for _, one := range found {
		byLabel[one.label()] = one
	}
	t.Run("the fragment is spliced into the named document", func(t *testing.T) {
		if !strings.Contains(byLabel["listQuery"].text, "author {") {
			t.Errorf("listQuery was collected without its fragment:\n%s", byLabel["listQuery"].text)
		}
	})
	t.Run("the package is recorded", func(t *testing.T) {
		if !strings.HasSuffix(byLabel["listQuery"].pkg, "/docs") {
			t.Errorf("package = %q, want the fixture package", byLabel["listQuery"].pkg)
		}
	})
	t.Run("the position points at the declaration", func(t *testing.T) {
		if byLabel["listQuery"].position.Line == 0 {
			t.Error("the collected document has no position")
		}
	})
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
			found, err := collect(testCase.dir, testCase.patterns, testCase.overlay)

			if err == nil {
				t.Fatalf("collect() error = nil, want one naming %q", testCase.want)
			}
			if found != nil {
				t.Errorf("collect() documents = %v, want nil on failure", found)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("collect() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestCollect_PatternMatchingNothing_IsReported verifies the guard against an
// audit that looks at the wrong place and reports a clean run. docs/ is a real
// directory of this repository that holds no Go at all, which is exactly the
// shape a mistyped pattern produces.
func TestCollect_PatternMatchingNothing_IsReported(t *testing.T) {
	found, err := collect(repoRoot(t), []string{"./docs/..."}, nil)

	if err == nil {
		t.Fatalf("collect() error = nil and found %d document(s), want the empty-match refusal", len(found))
	}
	if !strings.Contains(err.Error(), "no packages matched") {
		t.Errorf("collect() error = %q, want it to say nothing matched", err)
	}
}

// TestWalk_PackageWithoutTypeInformation_IsSkipped verifies the guard that
// keeps the collector from reading a package the loader gave no types for.
// Constants are folded during type checking, so such a package would answer
// every question with "not a document" and the audit would report a clean run
// over source it never understood.
func TestWalk_PackageWithoutTypeInformation_IsSkipped(t *testing.T) {
	gatherer := &collector{fset: token.NewFileSet(), claimed: map[token.Pos]bool{}}

	gatherer.walk(&packages.Package{PkgPath: "x/y"})

	if len(gatherer.documents) != 0 {
		t.Errorf("walk() collected %d document(s) from a package with no types", len(gatherer.documents))
	}
}

// TestDocumentLabel_UnnamedDocument_SaysSo verifies the label a report files an
// inline document under, since there is no constant name to print.
func TestDocumentLabel_UnnamedDocument_SaysSo(t *testing.T) {
	cases := []struct {
		name string
		doc  document
		want string
	}{
		{name: "declared under a name", doc: document{name: "queryListThings"}, want: "queryListThings"},
		{name: "written where it is used", doc: document{}, want: "an inline document"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.doc.label(); got != testCase.want {
				t.Errorf("label() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestRelative_PositionsUnderTheRoot_AreTrimmed verifies that a finding reads
// as a path a person can open, and that a position outside the root is left
// whole rather than turned into a walk of parent directories.
func TestRelative_PositionsUnderTheRoot_AreTrimmed(t *testing.T) {
	cases := []struct {
		name     string
		position token.Position
		root     string
		want     string
	}{
		{
			name:     "under the root",
			position: token.Position{Filename: filepath.Join("/repo", "internal", "tools", "x.go"), Line: 12},
			root:     "/repo",
			want:     "internal/tools/x.go:12",
		},
		{
			name:     "outside the root",
			position: token.Position{Filename: filepath.Join("/elsewhere", "x.go"), Line: 3},
			root:     "/repo",
			want:     filepath.Join("/elsewhere", "x.go") + ":3",
		},
		{
			name:     "no root to trim against",
			position: token.Position{Filename: filepath.Join("/repo", "x.go"), Line: 1},
			root:     "",
			want:     filepath.Join("/repo", "x.go") + ":1",
		},
		{
			name:     "a filename that is not a path under the root",
			position: token.Position{Filename: "relative.go", Line: 7},
			root:     "/repo",
			want:     "relative.go:7",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := relative(testCase.position, testCase.root); got != testCase.want {
				t.Errorf("relative() = %q, want %q", got, testCase.want)
			}
		})
	}
}
