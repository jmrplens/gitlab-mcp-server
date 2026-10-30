// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/orbit"
)

// narrowRelease is a release schema small enough to read: the gitlab.com pin
// has fields it does not, which is the whole situation the pass is for.
const narrowRelease = "type Query {\n  ok: Boolean\n}\n"

// fixtureReleaseSource is the record the release pass reports a narrow schema
// under.
var fixtureReleaseSource = graphqlschema.ReleaseSource{
	GitLabVersion: "19.5.0-ee", GitLabRevision: "883b289c40e", Origin: "the fixture", SHA256: "x",
	RetrievedAt: "2026-10-30", Types: 1,
}

// document builds one document a pass is handed.
func document(pkg, name, text string) graphqldocs.Document {
	return graphqldocs.Document{Package: pkg, Name: name, Position: token.Position{Filename: "/repo/" + name + ".go", Line: 3}, Text: text}
}

// TestJudgeRelease_SortsEveryRefusalByTheDeclarations verifies the pass over
// both sets of documents at once: a document the release accepts passes, one
// it refuses with no declaration fails the run, one a declaration answers is
// counted and passes, a declaration answering nothing fails the run, a
// client-go shell nobody can render is named rather than judged, and the two
// sides are counted apart.
func TestJudgeRelease_SortsEveryRefusalByTheDeclarations(t *testing.T) {
	schema := cmdutil.Must(graphqlschema.Load([]byte(narrowRelease)))
	own := []graphqldocs.Document{
		document("fixture/ok", "accepted", "query { ok }"),
		document("fixture/new", "selectsANewField", "query { crashState }"),
		document("fixture/saas", "gitLabDotComOnly", "query { orbit }"),
	}
	sdk := sdkRead{documents: []graphqldocs.Document{
		document(graphqldocs.SDKImportPath, "sdkAccepted", "query { ok }"),
		document(graphqldocs.SDKImportPath, "sdkSelectsANewField", "query { closedBy }"),
		{
			Package: graphqldocs.SDKImportPath, Name: "shell", Position: token.Position{Filename: "/mod/workitems.go", Line: 9},
			Text: "query { {{ .Fields }} }", Assembly: &graphqldocs.Assembly{Unrendered: "the fields are the caller's"},
		},
	}}
	declarations := []releaseDeclaration{
		{Package: "fixture/saas", Document: "gitLabDotComOnly", Reason: "served by GitLab.com alone"},
		{Package: "fixture/gone", Document: "renamed", Reason: "a document that is not there any more"},
	}

	verdict := judgeRelease(schema, own, sdk, declarations)

	if !verdict.failed() {
		t.Error("the pass does not fail a run with an undeclared refusal and a stale declaration")
	}
	if got := labels(verdict.refused); !equal(got, []string{"selectsANewField", "sdkSelectsANewField"}) {
		t.Errorf("refused = %v, want the two undeclared refusals, this repository's first", got)
	}
	if got := labels(verdict.declared); !equal(got, []string{"gitLabDotComOnly"}) {
		t.Errorf("declared = %v, want the one a declaration answers", got)
	}
	if len(verdict.stale) != 1 || verdict.stale[0].Document != "renamed" {
		t.Errorf("stale = %+v, want the declaration naming a document that is not there", verdict.stale)
	}
	if len(verdict.unjudged) != 1 || !strings.Contains(verdict.unjudged[0], "shell (workitems.go:9), assembled at run time: the fields are the caller's") {
		t.Errorf("unjudged = %v, want the shell named with where it is and why", verdict.unjudged)
	}
	counts := [4]int{verdict.judged, verdict.ownRefusals, verdict.sdkJudged, verdict.sdkRefusals}
	if counts != [4]int{3, 2, 2, 1} {
		t.Errorf("judged, refused, client-go judged, client-go refused = %v, want [3 2 2 1]", counts)
	}
}

// TestJudgeRelease_EverythingAccepted_Passes verifies the clean pass, and that
// a client-go module with no document to send refuses nothing rather than
// reporting an audit of nothing.
func TestJudgeRelease_EverythingAccepted_Passes(t *testing.T) {
	schema := cmdutil.Must(graphqlschema.Load([]byte(narrowRelease)))

	verdict := judgeRelease(schema, []graphqldocs.Document{document("fixture/ok", "accepted", "query { ok }")}, sdkRead{}, nil)

	if verdict.failed() || len(verdict.refused)+len(verdict.declared)+len(verdict.stale) != 0 || verdict.sdkJudged != 0 {
		t.Errorf("verdict = %+v, want a clean pass over one document and no client-go one", verdict)
	}
}

// TestJudgeRelease_ClientGoNotRead_FailsTheRun verifies that a pass which could
// not read client-go's documents fails: half of what this server sends left
// unjudged is not a pass.
func TestJudgeRelease_ClientGoNotRead_FailsTheRun(t *testing.T) {
	schema := cmdutil.Must(graphqlschema.Load([]byte(narrowRelease)))

	verdict := judgeRelease(schema, []graphqldocs.Document{document("fixture/ok", "accepted", "query { ok }")},
		sdkRead{err: errors.New("no client-go here")}, nil)

	if !verdict.failed() {
		t.Error("the pass does not fail a run that read no client-go document")
	}
}

// TestReportRelease_SaysWhatFailedAndWhatPassed verifies the report: each
// undeclared refusal under the sentence that says what it costs and where to
// declare one, each stale declaration with the file to remove it from, the
// unread client-go module, the unjudged shells, and the one summary line with
// both counts and the release that judged them.
func TestReportRelease_SaysWhatFailedAndWhatPassed(t *testing.T) {
	refusal := graphqldocs.Refusal{
		Document: document("fixture/new", "selectsANewField", "query { crashState }"),
		Reasons:  []string{`Cannot query field "crashState" on type "Query".`},
	}
	verdict := releaseVerdict{
		judged: 3, ownRefusals: 2, sdkJudged: 2, sdkRefusals: 0,
		refused:  []graphqldocs.Refusal{refusal},
		declared: []graphqldocs.Refusal{{Document: document("fixture/saas", "gitLabDotComOnly", "query { orbit }")}},
		stale:    []releaseDeclaration{{Package: "fixture/gone", Document: "renamed"}},
		unjudged: []string{"shell (workitems.go:9), assembled at run time: the fields are the caller's"},
		sdkErr:   errors.New("no client-go here"),
	}
	var out, errOut bytes.Buffer

	reportRelease(verdict, fixtureReleaseSource, []string{"/repo"}, &out, &errOut)

	for _, testCase := range []struct{ name, stream, want string }{
		{name: "the unread module", stream: errOut.String(), want: "client-go's documents were not read, so the release cannot judge them: no client-go here"},
		{name: "the refusal's heading", stream: errOut.String(), want: "GitLab 19.5.0-ee, the release this server is pinned to, refuses 1 document(s) it sends"},
		{name: "where to declare one", stream: errOut.String(), want: "cmd/audit_graphql_documents/release.go"},
		{name: "the refusal", stream: errOut.String(), want: "fixture/new selectsANewField (selectsANewField.go:3)\n    - Cannot query field \"crashState\" on type \"Query\".\n"},
		{name: "the stale declaration", stream: errOut.String(), want: "the release declaration for fixture/gone renamed answers nothing"},
		{name: "the unjudged shell", stream: out.String(), want: "    not judged by the release: shell (workitems.go:9)"},
		{
			name: "the summary", stream: out.String(),
			want: "GitLab 19.5.0-ee accepts 1 of 3 document(s) of this repository and 2 of 2 of client-go's, and refuses 1 " +
				"declared to need GitLab.com (gitLabDotComOnly); 1 types of GitLab 19.5.0-ee (883b289c40e), from the fixture",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if !strings.Contains(testCase.stream, testCase.want) {
				t.Errorf("the stream does not contain %q:\n%s", testCase.want, testCase.stream)
			}
		})
	}
}

// TestRun_TheReleasePass_FailsADocumentOnlyThePinAccepts verifies the pass in
// the run make check-graphql-documents makes: a document the gitlab.com pin
// accepts and the release refuses fails the run, and with it accepted the run
// passes with the release's summary.
func TestRun_TheReleasePass_FailsADocumentOnlyThePinAccepts(t *testing.T) {
	stubRelease(t, "type Query {\n  vulnerability(id: VulnerabilityID!): Vulnerability\n}\n\n"+
		"scalar VulnerabilityID\n\ntype Vulnerability {\n  id: ID!\n  title: String\n}\n")
	stubSDK(t, func(string) ([]graphqldocs.Document, error) { return nil, nil })

	t.Run("a field only the pin has", func(t *testing.T) {
		var out, errOut bytes.Buffer
		status := run(auditRun{dir: fixtureModule(t, map[string]string{"sound": soundFixture}), patterns: []string{"./..."}, release: true}, &out, &errOut)
		if status != 1 {
			t.Fatalf("exit status %d, want 1: the release has no severity.\nstdout:\n%s\nstderr:\n%s", status, out.String(), errOut.String())
		}
		if !strings.Contains(errOut.String(), "fixture/sound getVulnerability") || !strings.Contains(errOut.String(), "severity") {
			t.Errorf("stderr does not name the document and the field:\n%s", errOut.String())
		}
		if !strings.Contains(out.String(), "1 document(s) accepted (") {
			t.Errorf("the pin's verdict is not reported beside the release's:\n%s", out.String())
		}
	})
	t.Run("a document both accept", func(t *testing.T) {
		accepted := strings.Replace(soundFixture, "      severity\n", "", 1)
		var out, errOut bytes.Buffer
		status := run(auditRun{dir: fixtureModule(t, map[string]string{"sound": accepted}), patterns: []string{"./..."}, release: true}, &out, &errOut)
		// The real declarations name Orbit documents this fixture does not
		// have, so they are stale here; the run is held to the document alone.
		if strings.Contains(errOut.String(), "refuses") {
			t.Errorf("the release refused a document it has every field of:\n%s", errOut.String())
		}
		if status != 1 || !strings.Contains(errOut.String(), "answers nothing") {
			t.Errorf("exit status %d: the repository's Orbit declarations answer nothing in a fixture and fail it.\nstderr:\n%s", status, errOut.String())
		}
		if !strings.Contains(out.String(), "GitLab 19.5.0-ee accepts 1 of 1 document(s) of this repository and 0 of 0 of client-go's") {
			t.Errorf("stdout does not carry the release's summary:\n%s", out.String())
		}
	})
}

// TestRun_ASchemaOfItsOwn_AsksNoRelease verifies that a run judging by a
// schema it was handed does not also ask the release: it is already the
// comparison the release pass makes, against another schema.
func TestRun_ASchemaOfItsOwn_AsksNoRelease(t *testing.T) {
	stubSDK(t, func(string) ([]graphqldocs.Document, error) { return nil, nil })
	stubRelease(t, narrowRelease)
	narrow := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrow, []byte(narrowRelease), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	var out, errOut bytes.Buffer

	run(auditRun{dir: fixtureModule(t, map[string]string{"ok": okFixture}), patterns: []string{"./..."}, schemaPath: narrow, release: true}, &out, &errOut)

	if strings.Contains(out.String()+errOut.String(), "19.5.0-ee") {
		t.Errorf("a run handed a schema asked the release too:\n%s\n%s", out.String(), errOut.String())
	}
}

// TestReleaseDeclarations_OrbitDocuments_BelongToGitLabDotComOnlyActions holds
// the reason every Orbit declaration gives to the catalog: each Orbit action is
// declared GitLabDotComOnly, so no session on a self-managed instance is
// offered one and its documents never reach a release.
func TestReleaseDeclarations_OrbitDocuments_BelongToGitLabDotComOnlyActions(t *testing.T) {
	specs := orbit.ActionSpecs(nil)
	if len(specs) == 0 {
		t.Fatal("orbit.ActionSpecs() returned nothing to hold the declarations to")
	}
	for _, spec := range specs {
		if !spec.GitLabDotComOnly {
			t.Errorf("orbit.%s is not GitLabDotComOnly, so the release declarations' reason no longer holds", spec.Name)
		}
	}
	for _, declaration := range releaseDeclarations() {
		t.Run(declaration.Document, func(t *testing.T) {
			if declaration.Package != orbitPackage {
				t.Errorf("%s %s is declared outside the Orbit package, and this test holds only Orbit's reason", declaration.Package, declaration.Document)
			}
			if !strings.Contains(declaration.Reason, "GitLabDotComOnly") {
				t.Errorf("the reason for %s does not say why the release never receives it: %s", declaration.Document, declaration.Reason)
			}
		})
	}
}

// TestReleasePin_Default_IsTheEmbeddedReleasePin verifies what the release
// pass reads when no test hands it a schema: the release pin
// internal/graphqlschema embeds, with that pin's own record, so the gate
// judges by the file make check-graphql-schema holds to the REST record and
// not by a schema of its own.
func TestReleasePin_Default_IsTheEmbeddedReleasePin(t *testing.T) {
	embedded, err := graphqlschema.ReleaseSchema()
	if err != nil {
		t.Fatalf("graphqlschema.ReleaseSchema() error: %v", err)
	}
	record, err := graphqlschema.ReleaseSourceInfo()
	if err != nil {
		t.Fatalf("graphqlschema.ReleaseSourceInfo() error: %v", err)
	}

	schema, source := releasePin()

	if schema != embedded {
		t.Error("releasePin() returned a schema other than graphqlschema.ReleaseSchema()")
	}
	if source != record {
		t.Errorf("releasePin() reports %+v, want the embedded record %+v", source, record)
	}
}

// stubRelease hands the release pass a schema of its own for one test.
func stubRelease(t *testing.T, sdl string) {
	t.Helper()
	schema := cmdutil.Must(graphqlschema.Load([]byte(sdl)))
	original := releasePin
	releasePin = func() (*ast.Schema, graphqlschema.ReleaseSource) { return schema, fixtureReleaseSource }
	t.Cleanup(func() { releasePin = original })
}

// stubSDK hands the run client-go documents of its own for one test.
func stubSDK(t *testing.T, read func(string) ([]graphqldocs.Document, error)) {
	t.Helper()
	original := readSDKDocuments
	readSDKDocuments = read
	t.Cleanup(func() { readSDKDocuments = original })
}

// labels names a list of refusals by their documents.
func labels(refusals []graphqldocs.Refusal) []string {
	names := make([]string, 0, len(refusals))
	for _, refusal := range refusals {
		names = append(names, refusal.Document.Label())
	}
	return names
}

// equal compares two lists of names in order.
func equal(got, want []string) bool {
	return strings.Join(got, "\n") == strings.Join(want, "\n")
}
