package main

import (
	"bytes"
	"errors"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqldocs"
)

// afterThePin is a day later than the committed provenance record, so the age
// line a live run prints is a number rather than a negative one.
func afterThePin() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }

// soundFixture holds documents the pinned schema accepts, written the way the
// repository writes them: a named constant, and one assembled from a shared
// fragment.
const soundFixture = "package sound\n\n" +
	"const vulnFields = `\n      id\n      title\n      severity\n`\n\n" +
	"const getVulnerability = `\nquery($id: VulnerabilityID!) {\n  vulnerability(id: $id) {` + vulnFields + `\n  }\n}\n`\n"

// brokenFixture holds the two shapes GitLab refuses that no test would ever
// catch on its own: a field the type does not have, and an argument the field
// does not accept.
const brokenFixture = "package broken\n\n" +
	"const listVulnerabilities = `\nquery($path: ID!, $severity: [String!]) {\n" +
	"  project(fullPath: $path) {\n    vulnerabilities(severity: $severity) {\n" +
	"      nodes {\n        id\n        hasSolutions\n      }\n    }\n  }\n}\n`\n"

// fixtureModule writes a throwaway module holding one package per entry and
// returns its root.
//
// A module of its own rather than an overlay over this repository, because
// what this file tests is the command around the audit: the exit status, the
// two streams and the shape of a finding. The reading of the source is
// cmd/internal/graphqldocs's own business and is tested there against the
// harder shapes.
func fixtureModule(t *testing.T, packages map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	for name, source := range packages {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".go"), []byte(source), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
	}
	return root
}

// runFixture runs the audit over a fixture module and returns the exit status
// with both streams.
func runFixture(t *testing.T, packages map[string]string, verbose bool) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(graphqldocs.Options{
		Dir:      fixtureModule(t, packages),
		Patterns: []string{"./..."},
	}, verbose, &out, &errOut)
	return status, out.String(), errOut.String()
}

// TestRun_AgainstASchemaTheCallerSupplies_JudgesByThatSchema verifies the entry
// the live re-probe uses.
//
// The pin can only report a document that was already broken when it was taken,
// never one GitLab has narrowed since, which is how every defect this gate was
// built for arose. Handing the audit a schema fetched today closes that, so the
// case that matters is a document the pin accepts and the supplied schema does
// not, and a summary that says which of the two judged it.
func TestRun_AgainstASchemaTheCallerSupplies_JudgesByThatSchema(t *testing.T) {
	narrowed := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrowed, []byte("type Query {\n  ok: Boolean\n}\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	var out, errOut bytes.Buffer
	status := run(graphqldocs.Options{
		Dir:        fixtureModule(t, map[string]string{"sound": soundFixture}),
		Patterns:   []string{"./..."},
		SchemaPath: narrowed,
	}, false, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1: the supplied schema has no vulnerability field.\nstdout:\n%s", status, out.String())
	}
	if !strings.Contains(errOut.String(), "not the pinned schema") {
		t.Errorf("the summary does not say which schema judged the documents:\n%s", errOut.String())
	}
}

// okFixture holds one document the smallest possible instance accepts, so a run
// against a fetched schema can be judged by that schema rather than by the pin.
const okFixture = `package ok

const queryOk = @@query { ok }@@
`

// TestRun_AgainstAnInstanceItIntrospects_JudgesByWhatThatInstanceServes
// verifies the mode the scheduled job runs.
//
// It is the one check the pin cannot perform. The pin says the documents were
// valid on gitlab.com on the day it was taken; this says they are valid on the
// GitLab an instance is serving now, and it reports where the two disagree
// about something the documents touch, which is how the pin's age becomes a
// number somebody sees rather than an assumption.
func TestRun_AgainstAnInstanceItIntrospects_JudgesByWhatThatInstanceServes(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:      repoRoot(t),
		patterns: []string{fixturePattern},
		overlay:  fixtureOverlay(t, map[string]string{"ok": okFixture}),
		live:     answeringInstance(t, introspectionAnswer(queryOnly)),
		now:      afterThePin,
	}, &out, &errOut)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut.String())
	}
	for _, want := range []string{
		"fetched now, not the pinned schema",
		"the pin and the live schema disagree on",
		"Query.ok: the live schema has it, the pin does not",
		"day(s) ago",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out.String(), want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out.String())
			}
		})
	}
}

// TestRun_AnInstanceThatCannotBeReached_FailsWithoutFallingBackToThePin
// verifies that a re-probe whose instance never answered stops rather than
// judging by the pin, which would report a pass for a question nobody asked.
func TestRun_AnInstanceThatCannotBeReached_FailsWithoutFallingBackToThePin(t *testing.T) {
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable.Close()

	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:      repoRoot(t),
		patterns: []string{fixturePattern},
		overlay:  fixtureOverlay(t, map[string]string{"ok": okFixture}),
		live:     unreachable.URL,
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "ask ") {
		t.Errorf("stderr does not say the instance could not be asked:\n%s", errOut.String())
	}
	if strings.Contains(out.String(), "document(s) accepted") {
		t.Errorf("the run reported documents accepted after the fetch failed:\n%s", out.String())
	}
}

// TestRun_BothSchemaSourcesAtOnce_IsRefused verifies that a run naming two
// schemas is refused rather than silently preferring one. Which of the two
// judged the documents is the whole meaning of the result, so a run that cannot
// say must not produce one.
func TestRun_BothSchemaSourcesAtOnce_IsRefused(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:        repoRoot(t),
		patterns:   []string{fixturePattern},
		overlay:    fixtureOverlay(t, map[string]string{"ok": okFixture}),
		live:       "https://gitlab.example.com/api/graphql",
		schemaPath: filepath.Join(t.TempDir(), "unused.graphql"),
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "pass one") {
		t.Errorf("stderr does not explain the refusal:\n%s", errOut.String())
	}
}

// TestRun_ASuppliedSchemaThatCannotBeUsed_Fails verifies that a live re-probe
// whose schema never arrived stops rather than falling back to the pin, which
// would report a pass for a question nobody asked.
func TestRun_ASuppliedSchemaThatCannotBeUsed_Fails(t *testing.T) {
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
			var out, errOut bytes.Buffer

			status := run(graphqldocs.Options{
				Dir:        fixtureModule(t, map[string]string{"sound": soundFixture}),
				Patterns:   []string{"./..."},
				SchemaPath: testCase.path,
			}, false, &out, &errOut)

			if status != 1 {
				t.Fatalf("exit status %d, want 1", status)
			}
			if !strings.Contains(errOut.String(), testCase.want) {
				t.Errorf("stderr does not explain the failure %q:\n%s", testCase.want, errOut.String())
			}
		})
	}
}

// TestRun_DocumentsThePinnedSchemaAccepts_Succeeds verifies the passing path,
// including that the summary names the pin so a reader is told how old the
// judgement is.
func TestRun_DocumentsThePinnedSchemaAccepts_Succeeds(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"sound": soundFixture}, false)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	for _, want := range []string{"document(s) accepted", "gitlab.com", "retrieved"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out)
			}
		})
	}
	if errOut != "" {
		t.Errorf("a clean run wrote to stderr:\n%s", errOut)
	}
}

// TestRun_Verbose_ListsWhatItAccepted verifies that the set a reviewer has to
// care about is reviewable rather than a count, and that the listing names only
// what passed: a document printed as accepted and refused in the same run would
// be worse than either line alone.
func TestRun_Verbose_ListsWhatItAccepted(t *testing.T) {
	status, out, _ := runFixture(t, map[string]string{"sound": soundFixture, "broken": brokenFixture}, true)

	if status != 1 {
		t.Fatalf("exit status %d, want 1: the broken fixture is refused", status)
	}
	if !strings.Contains(out, "ok  ") || !strings.Contains(out, "getVulnerability") {
		t.Errorf("the verbose run does not name the document it checked:\n%s", out)
	}
	if strings.Contains(out, "listVulnerabilities") {
		t.Errorf("the verbose run listed a refused document as accepted:\n%s", out)
	}
}

// TestRun_ADocumentGitLabWouldRefuse_Fails verifies the finding: a non-zero
// exit, the constant named, the file it is declared in, and every reason
// underneath.
func TestRun_ADocumentGitLabWouldRefuse_Fails(t *testing.T) {
	status, _, errOut := runFixture(t, map[string]string{"broken": brokenFixture}, false)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	for _, want := range []string{
		"listVulnerabilities",
		"broken/broken.go:",
		`Cannot query field "hasSolutions"`,
		`used in position expecting type "[VulnerabilitySeverity!]"`,
		"refuses 1 of 1 document(s)",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not contain %q:\n%s", want, errOut)
			}
		})
	}
}

// TestRun_NothingToCheck_IsAFailure verifies the guard against an audit
// pointed at the wrong tree: finding no documents at all means the audit is
// looking somewhere the documents are not, which must not read as a pass.
func TestRun_NothingToCheck_IsAFailure(t *testing.T) {
	const noDocuments = "package empty\n\nconst notADocument = \"there is no GraphQL here\"\n"

	status, _, errOut := runFixture(t, map[string]string{"empty": noDocuments}, false)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut, "no GraphQL documents were found") {
		t.Errorf("stderr does not report the empty result:\n%s", errOut)
	}
}

// TestRun_SourceThatCannotBeLoaded_Fails verifies that the audit stops rather
// than reporting a clean run over source it could not read.
func TestRun_SourceThatCannotBeLoaded_Fails(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(graphqldocs.Options{Dir: t.TempDir(), Patterns: []string{"./..."}}, false, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "audit_graphql_documents:") {
		t.Errorf("stderr does not name the command:\n%s", errOut.String())
	}
}

// TestFinding_EveryReason_IsListedUnderTheDocument verifies that a finding
// names the document once and every objection under it, including the case
// where the audit could not judge the document at all and the single line it
// has must survive.
func TestFinding_EveryReason_IsListedUnderTheDocument(t *testing.T) {
	cases := []struct {
		name    string
		reasons []string
		want    string
	}{
		{name: "a refusal", reasons: []string{"first", "second"}, want: "    - first\n    - second\n"},
		{name: "anything else", reasons: []string{"the pin is corrupt"}, want: "    - the pin is corrupt\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report := finding("", graphqldocs.Refusal{
				Document: graphqldocs.Document{Package: "x/y", Name: "queryThing"},
				Reasons:  testCase.reasons,
			})

			if !strings.HasPrefix(report, "x/y queryThing (") {
				t.Errorf("the report does not name the document:\n%s", report)
			}
			if !strings.HasSuffix(report, testCase.want) {
				t.Errorf("the report does not end with %q:\n%s", testCase.want, report)
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
