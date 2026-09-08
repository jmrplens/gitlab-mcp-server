package graphqldocs

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
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
func TestAudit_ASchemaTheCallerSupplies_JudgesByThatSchemaAndSaysSo(t *testing.T) {
	narrowed := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrowed, []byte("type Query {\n  ok: Boolean\n}\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	result, err := auditFixture(t, map[string]string{"accepted": acceptedFixture}, narrowed)
	if err != nil {
		t.Fatalf("Audit() error = %v, want nil", err)
	}
	if len(result.Refusals) != 1 {
		t.Fatalf("Audit() refused %d document(s), want 1: the supplied schema has no vulnerability field", len(result.Refusals))
	}
	if !strings.Contains(result.Provenance, "not the pinned schema") {
		t.Errorf("Audit() provenance = %q, want it to say which schema judged the documents", result.Provenance)
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

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "no such file", path: filepath.Join(t.TempDir(), "absent.graphql"), want: "read the schema to judge against"},
		{name: "not a schema", path: unparseable, want: "parse the schema"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := auditFixture(t, map[string]string{"accepted": acceptedFixture}, testCase.path)

			if err == nil {
				t.Fatalf("Audit() error = nil, want one naming %q", testCase.want)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Audit() error = %q, want it to name %q", err, testCase.want)
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
