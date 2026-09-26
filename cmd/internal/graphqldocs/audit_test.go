package graphqldocs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// acceptedFixture holds a document the pinned schema accepts.
const acceptedFixture = `package accepted

const getVulnerability = @@
query($id: VulnerabilityID!) {
  vulnerability(id: $id) {
    id
    title
  }
}
@@
`

// refusedFixture holds the two shapes GitLab refuses that no test would catch
// on its own: a field the type does not have, and an argument the field does
// not accept.
const refusedFixture = `package refused

const listVulnerabilities = @@
query($path: ID!, $severity: [String!]) {
  project(fullPath: $path) {
    vulnerabilities(severity: $severity) {
      nodes {
        id
        hasSolutions
      }
    }
  }
}
@@
`

// auditFixture runs an audit over a fixture package set through the overlay.
func auditFixture(t *testing.T, sources map[string]string, schemaPath string) (Result, error) {
	t.Helper()
	return Audit(Options{
		Dir:        repoRoot(t),
		Patterns:   []string{fixturePattern},
		Overlay:    fixtureOverlay(t, sources),
		SchemaPath: schemaPath,
	})
}

// TestAudit_DocumentsThePinnedSchemaAccepts_AreReportedWithoutARefusal
// verifies the passing shape: every document read, none refused, and a
// provenance line naming the pin so a caller can say how old the judgement is.
func TestAudit_DocumentsThePinnedSchemaAccepts_AreReportedWithoutARefusal(t *testing.T) {
	result, err := auditFixture(t, map[string]string{"accepted": acceptedFixture}, "")
	if err != nil {
		t.Fatalf("Audit() error = %v, want nil", err)
	}
	if len(result.Documents) != 1 {
		t.Fatalf("Audit() read %d document(s), want 1", len(result.Documents))
	}
	if len(result.Refusals) != 0 {
		t.Errorf("Audit() refused %+v, want nothing", result.Refusals)
	}
	if !strings.Contains(result.Provenance, "gitlab.com") {
		t.Errorf("Audit() provenance = %q, want it to name the pinned schema's source", result.Provenance)
	}
}

// TestAudit_ADocumentGitLabWouldRefuse_IsAFindingAndNotAnError verifies the
// distinction the whole result type exists for: a refused document is
// something the caller reports, while an error means the audit did not run and
// must never be reported as a pass.
func TestAudit_ADocumentGitLabWouldRefuse_IsAFindingAndNotAnError(t *testing.T) {
	result, err := auditFixture(t, map[string]string{"refused": refusedFixture}, "")
	if err != nil {
		t.Fatalf("Audit() error = %v, want the refusal in the result instead", err)
	}
	if len(result.Refusals) != 1 {
		t.Fatalf("Audit() refused %d document(s), want 1", len(result.Refusals))
	}
	refusal := result.Refusals[0]
	if refusal.Document.Name != "listVulnerabilities" {
		t.Errorf("Audit() refused %q, want listVulnerabilities", refusal.Document.Name)
	}
	if len(refusal.Reasons) < 2 {
		t.Errorf("Audit() gave %v as the reasons, want both the unknown field and the wrong argument type", refusal.Reasons)
	}
}

// TestAudit_NothingToJudge_IsAnError verifies the guard against an audit
// pointed at the wrong tree. A clean exit there is the silence this package
// exists to remove, so an empty read is an error rather than an empty result.
func TestAudit_NothingToJudge_IsAnError(t *testing.T) {
	const noDocuments = `package empty

const notADocument = "there is no GraphQL here"
`

	result, err := auditFixture(t, map[string]string{"empty": noDocuments}, "")

	if !errors.Is(err, ErrNoDocuments) {
		t.Fatalf("Audit() error = %v, want ErrNoDocuments", err)
	}
	if len(result.Documents) != 0 {
		t.Errorf("Audit() returned %d document(s) with the error, want none", len(result.Documents))
	}
}

// TestJudge_DocumentsTheCallerAlreadyHolds_AreRefusedOnTheSameTerms verifies
// the entry the client-go half uses.
//
// Those documents come out of a module directory rather than out of a pattern
// under this repository, so they are collected elsewhere and arrive here as a
// slice. What must not differ is the judgement: a document the pinned schema
// refuses has to be refused whether it was written here or in the SDK, because
// it reaches GitLab through this server either way. The empty case is the same
// guard [Audit] has, for the same reason: a clean exit over nothing is the
// silence this package exists to remove.
func TestJudge_DocumentsTheCallerAlreadyHolds_AreRefusedOnTheSameTerms(t *testing.T) {
	documents := []Document{
		{Package: "sdk", Name: "accepted", Text: "query($id: VulnerabilityID!) { vulnerability(id: $id) { id } }"},
		{Package: "sdk", Name: "refused", Text: "query($id: VulnerabilityID!) { vulnerability(id: $id) { hasSolutions } }"},
	}

	result, err := Judge(documents, Options{})
	if err != nil {
		t.Fatalf("Judge() error = %v, want nil", err)
	}
	if len(result.Documents) != 2 {
		t.Errorf("Judge() kept %d document(s), want both", len(result.Documents))
	}
	if len(result.Refusals) != 1 || result.Refusals[0].Document.Name != "refused" {
		t.Fatalf("Judge() refused %+v, want only the document with the field the type lacks", result.Refusals)
	}
	if result.Provenance == "" {
		t.Error("Judge() gave no provenance, so a reader cannot say whose opinion refused the document")
	}

	if _, emptyErr := Judge(nil, Options{}); !errors.Is(emptyErr, ErrNoDocuments) {
		t.Errorf("Judge(nil) error = %v, want ErrNoDocuments", emptyErr)
	}
}

// TestJudge_ASchemaItCannotRead_Fails verifies that the caller holding its own
// documents gets the same refusal [Audit] gives.
//
// The failure matters more on this entry than on that one. A caller here has
// already paid for a load of somebody else's module, so falling back to the pin
// when the schema it named could not be read would answer a question nobody
// asked, and answer it as a clean run over documents the named schema never
// saw.
func TestJudge_ASchemaItCannotRead_Fails(t *testing.T) {
	documents := []Document{{Package: "sdk", Name: "accepted", Text: "query { currentUser { id } }"}}

	_, err := Judge(documents, Options{SchemaPath: filepath.Join(t.TempDir(), "absent.graphql")})

	if err == nil {
		t.Fatal("Judge() error = nil, want the unreadable schema reported")
	}
	if !strings.Contains(err.Error(), "read the schema to judge against") {
		t.Errorf("Judge() error = %q, want it to name the schema it could not read", err)
	}
}

// TestAudit_SourceItCannotRead_Fails verifies that a load failure ends the run
// rather than being reported as a tree with no documents in it.
func TestAudit_SourceItCannotRead_Fails(t *testing.T) {
	_, err := Audit(Options{Dir: filepath.Join(t.TempDir(), "nowhere"), Patterns: []string{"./..."}})

	if err == nil {
		t.Fatal("Audit() error = nil, want the load failure")
	}
	if !strings.Contains(err.Error(), "load packages") {
		t.Errorf("Audit() error = %q, want it to name the load failure", err)
	}
}

// TestAudit_ASchemaTheCallerSupplies_JudgesByThatSchemaAndSaysSo verifies the
// entry the live re-probe uses. The pin can only report a document that was
// already broken when it was taken, never one GitLab has narrowed since, which
// is how every defect this gate was built for arose.
//
// The whole provenance line is compared rather than the disclaimer at the end
// of it, because the two values in front of that disclaimer are what a reader of
// a refusal actually needs: which schema refused the document, and how big it
// was, since a schema that loaded almost nothing refuses almost everything and
// says so only through that count.
func TestAudit_ASchemaTheCallerSupplies_JudgesByThatSchemaAndSaysSo(t *testing.T) {
	const sdl = "type Query {\n  ok: Boolean\n}\n"
	narrowed := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrowed, []byte(sdl), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	loaded, err := graphqlschema.Load([]byte(sdl))
	if err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	result, err := auditFixture(t, map[string]string{"accepted": acceptedFixture}, narrowed)
	if err != nil {
		t.Fatalf("Audit() error = %v, want nil", err)
	}
	if len(result.Refusals) != 1 {
		t.Fatalf("Audit() refused %d document(s), want 1: the supplied schema has no vulnerability field", len(result.Refusals))
	}
	want := fmt.Sprintf("%d types from %s, not the pinned schema", len(loaded.Types), narrowed)
	if result.Provenance != want {
		t.Errorf("Audit() provenance = %q, want %q", result.Provenance, want)
	}
}

// TestAudit_ASchemaValueTheCallerAlreadyHas_JudgesByItAndKeepsItsProvenance
// verifies the other entry the live re-probe uses: the schema as a value rather
// than as a path, which is what it has after introspecting an instance itself.
// The provenance is the caller's own line, because only the caller knows what
// it introspected, and a reader of a refusal is owed that rather than the pin's
// date.
func TestAudit_ASchemaValueTheCallerAlreadyHas_JudgesByItAndKeepsItsProvenance(t *testing.T) {
	narrowed, err := graphqlschema.Load([]byte("type Query {\n  ok: Boolean\n}\n"))
	if err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	const provenance = "1234 types from https://gitlab.example.com, introspected today"

	result, err := Audit(Options{
		Dir:        repoRoot(t),
		Patterns:   []string{fixturePattern},
		Overlay:    fixtureOverlay(t, map[string]string{"accepted": acceptedFixture}),
		Schema:     narrowed,
		Provenance: provenance,
	})
	if err != nil {
		t.Fatalf("Audit() error = %v, want nil", err)
	}
	if len(result.Refusals) != 1 {
		t.Fatalf("Audit() refused %d document(s), want 1: the supplied schema has no vulnerability field", len(result.Refusals))
	}
	if result.Provenance != provenance {
		t.Errorf("Audit() provenance = %q, want the caller's own line %q", result.Provenance, provenance)
	}
}

// TestAudit_ASuppliedSchemaThatCannotBeUsed_Fails verifies that a live re-probe
// whose schema never arrived stops rather than falling back to the pin, which
// would report a pass for a question nobody asked.
func TestAudit_ASuppliedSchemaThatCannotBeUsed_Fails(t *testing.T) {
	unparseable := filepath.Join(t.TempDir(), "prose.graphql")
	if err := os.WriteFile(unparseable, []byte("this is prose, not a schema"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	// The prefix rather than a substring, and for the unparseable file the path
	// in front of it: "parse the schema" is graphqlschema's own wording, so the
	// path is the one part of that message this package writes, and it is the
	// part that tells a reader which of their files was not a schema.
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "no such file", path: filepath.Join(t.TempDir(), "absent.graphql"), want: "read the schema to judge against: "},
		{name: "not a schema", path: unparseable, want: unparseable + ": parse the schema"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := auditFixture(t, map[string]string{"accepted": acceptedFixture}, testCase.path)

			if err == nil {
				t.Fatalf("Audit() error = nil, want one naming %q", testCase.want)
			}
			if !strings.HasPrefix(err.Error(), testCase.want) {
				t.Errorf("Audit() error = %q, want it to open with %q", err, testCase.want)
			}
		})
	}
}

// TestAudit_AnUnusableSchemaOverAnUnloadableTree_ReportsTheSchema verifies the
// order the two halves of an audit fail in.
//
// The schema is resolved before the tree is type-checked, so a mistyped
// -schema fails in milliseconds rather than after the seconds a load costs.
// Every other test of either failure passes the other half in working order,
// which leaves that order unobserved: the refusal a reader gets would be the
// same whichever ran first.
func TestAudit_AnUnusableSchemaOverAnUnloadableTree_ReportsTheSchema(t *testing.T) {
	_, err := Audit(Options{
		Dir:        filepath.Join(t.TempDir(), "nowhere"),
		Patterns:   []string{"./..."},
		SchemaPath: filepath.Join(t.TempDir(), "absent.graphql"),
	})

	if err == nil {
		t.Fatal("Audit() error = nil, want the unreadable schema")
	}
	if !strings.HasPrefix(err.Error(), "read the schema to judge against: ") {
		t.Errorf("Audit() error = %q, want the schema's failure rather than the load's", err)
	}
}

// TestJudge_WhichSchemaJudges_FollowsTheDocumentedPrecedence verifies the three
// sources a schema can come from and the one line that says which of them
// judged.
//
// A schema value wins over a path, and a path wins over the pin; a provenance
// is the caller's to write only beside a value, since only the caller knows
// what it introspected, and without one it would put a caller's words under
// the pin's judgement. The document is one the pin accepts and the narrowed
// schema refuses, so the refusal count says which schema judged as well as the
// provenance does, and each case is compared as that pair.
func TestJudge_WhichSchemaJudges_FollowsTheDocumentedPrecedence(t *testing.T) {
	const sdl = "type Query {\n  ok: Boolean\n}\n"
	narrowedPath := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrowedPath, []byte(sdl), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	narrowed, err := graphqlschema.Load([]byte(sdl))
	if err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	pin, err := graphqlschema.SourceInfo()
	if err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	const callerLine = "3 types from a probe this test ran"
	documents := []Document{{Package: "sdk", Name: "currentUser", Text: "query { currentUser { id } }"}}

	type verdict struct {
		provenance string
		refusals   int
	}
	cases := []struct {
		name string
		opts Options
		want verdict
	}{
		{name: "nothing named", opts: Options{}, want: verdict{provenance: pin.String()}},
		{
			name: "a provenance with no schema beside it",
			opts: Options{Provenance: callerLine},
			want: verdict{provenance: pin.String()},
		},
		{
			name: "a path",
			opts: Options{SchemaPath: narrowedPath},
			want: verdict{provenance: fmt.Sprintf("%d types from %s, not the pinned schema", len(narrowed.Types), narrowedPath), refusals: 1},
		},
		{
			name: "a value",
			opts: Options{Schema: narrowed, Provenance: callerLine},
			want: verdict{provenance: callerLine, refusals: 1},
		},
		{
			name: "a value beside a path that cannot be read",
			opts: Options{Schema: narrowed, Provenance: callerLine, SchemaPath: filepath.Join(t.TempDir(), "absent.graphql")},
			want: verdict{provenance: callerLine, refusals: 1},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, judgeErr := Judge(documents, testCase.opts)
			if judgeErr != nil {
				t.Fatalf("Judge() error = %v, want nil", judgeErr)
			}
			if got := (verdict{provenance: result.Provenance, refusals: len(result.Refusals)}); got != testCase.want {
				t.Errorf("Judge() = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestAudit_NoPatterns_ReadsWhatTheRepositoryKeepsItsDocumentsUnder verifies
// the fallback, since a caller that names no patterns must audit the tree the
// documents are in rather than nothing at all.
func TestAudit_NoPatterns_ReadsWhatTheRepositoryKeepsItsDocumentsUnder(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "internal", "tools", "customemoji")
	if err := os.MkdirAll(tree, 0o750); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(tree, name), []byte(content), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
	}
	write("customemoji.go", "package customemoji\n")
	write("get.graphql", "query {\n  currentUser {\n    id\n  }\n}\n")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	result, err := Audit(Options{Dir: root})
	if err != nil {
		t.Fatalf("Audit() error = %v, want nil", err)
	}
	if len(result.Documents) != 1 || result.Documents[0].Name != "get.graphql" {
		t.Errorf("Audit() read %+v, want the one standalone document under internal/", result.Documents)
	}
}

// TestDefaultPatterns_EachCallerGetsItsOwn verifies that appending to the
// result cannot change what a later caller audits, which would silently point
// an audit at the wrong tree.
func TestDefaultPatterns_EachCallerGetsItsOwn(t *testing.T) {
	// Appending to the returned slice is exactly what is under test: a caller
	// that grows it must not be able to change what a later caller audits.
	first := DefaultPatterns()
	grown := append(first, "./cmd/...")
	_ = grown

	if second := DefaultPatterns(); !slices.Equal(second, []string{"./internal/..."}) {
		t.Errorf("DefaultPatterns() = %v after a caller appended to an earlier result, want it unchanged", second)
	}
}

// TestReasons_AFailureThatIsNotARefusal_IsStillExplained verifies that a
// finding always carries at least one line. A document reported as refused with
// nothing under it reads as one nobody could explain, which is worse than the
// error text the failure actually had.
func TestReasons_AFailureThatIsNotARefusal_IsStillExplained(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "a refusal",
			err:  &graphqlschema.ValidationError{Reasons: []string{"first", "second"}},
			want: []string{"first", "second"},
		},
		{
			name: "a refusal that names no reason",
			err:  &graphqlschema.ValidationError{},
			want: []string{(&graphqlschema.ValidationError{}).Error()},
		},
		{name: "anything else", err: errors.New("the pin is corrupt"), want: []string{"the pin is corrupt"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := reasons(testCase.err); !slices.Equal(got, testCase.want) {
				t.Errorf("reasons() = %v, want %v", got, testCase.want)
			}
		})
	}
}
