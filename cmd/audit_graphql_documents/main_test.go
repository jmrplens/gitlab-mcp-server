package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// soundFixture holds documents the pinned schema accepts, written the way the
// repository writes them: a named constant, and one assembled from a shared
// fragment.
const soundFixture = `package sound

const vulnFields = @@
      id
      title
      severity
@@

const getVulnerability = @@
query($id: VulnerabilityID!) {
  vulnerability(id: $id) {@@ + vulnFields + @@
  }
}
@@
`

// brokenFixture holds the two shapes GitLab refuses that no test would ever
// catch on its own: a field the type does not have, and an argument the field
// does not accept.
const brokenFixture = `package broken

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

// runFixture runs the audit over a fixture package set and returns the exit
// status with both streams.
func runFixture(t *testing.T, sources map[string]string, verbose bool) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:      repoRoot(t),
		verbose:  verbose,
		patterns: []string{fixturePattern},
		overlay:  fixtureOverlay(t, sources),
	}, &out, &errOut)
	return status, out.String(), errOut.String()
}

// TestRun_DocumentsThePinnedSchemaAccepts_Succeeds verifies the passing path,
// including that the summary names the pin so a reader is told how old the
// judgement is.
func TestRun_DocumentsThePinnedSchemaAccepts_Succeeds(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"sound": soundFixture}, false)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	for _, want := range []string{"accepted by the pinned schema", "gitlab.com", "retrieved"} {
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
// care about is reviewable rather than a count.
func TestRun_Verbose_ListsWhatItAccepted(t *testing.T) {
	status, out, _ := runFixture(t, map[string]string{"sound": soundFixture}, true)

	if status != 0 {
		t.Fatalf("exit status %d, want 0", status)
	}
	if !strings.Contains(out, "ok  ") || !strings.Contains(out, "getVulnerability") {
		t.Errorf("the verbose run does not name the document it checked:\n%s", out)
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
		fixtureDir + "/broken/broken.go:",
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
	const noDocuments = `package empty

const notADocument = "there is no GraphQL here"
`

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

	status := run(auditRun{dir: t.TempDir(), patterns: []string{"./..."}}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "audit_graphql_documents:") {
		t.Errorf("stderr does not name the command:\n%s", errOut.String())
	}
}

// TestFinding_RefusalThatCarriesNoReasons_IsStillReported verifies the branch
// taken when the failure is not a refusal but an inability to load the pinned
// schema: there are no reasons to list, and the one line there is must survive.
func TestFinding_RefusalThatCarriesNoReasons_IsStillReported(t *testing.T) {
	found := document{pkg: "x/y", name: "queryThing"}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "a refusal",
			err:  &graphqlschema.ValidationError{Reasons: []string{"first", "second"}},
			want: "    - first\n    - second\n",
		},
		{name: "anything else", err: errors.New("the pin is corrupt"), want: "    the pin is corrupt\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report := finding("", found, testCase.err)

			if !strings.HasPrefix(report, "x/y queryThing (") {
				t.Errorf("the report does not name the document:\n%s", report)
			}
			if !strings.HasSuffix(report, testCase.want) {
				t.Errorf("the report does not end with %q:\n%s", testCase.want, report)
			}
		})
	}
}
