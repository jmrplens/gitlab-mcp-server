package paths

import (
	"errors"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// sdkModuleDir is the shape of a module cache directory, which is what the
// position trimming below is about.
const sdkModuleDir = "/home/somebody/go/pkg/mod/gitlab.com/gitlab-org/api/client-go/v3@v3.0.0"

// withSDKSeams replaces the pairing lookup and the two halves of the SDK read
// for the length of a test.
//
// All three are out of reach otherwise: the pairing costs a twenty-second load
// of the tool packages, and the documents come out of whatever the module cache
// on this machine happens to hold, which is a version that moves.
func withSDKSeams(
	t *testing.T,
	pairings func(string) (structs.Pairings, error),
	documents func(string) ([]graphqldocs.Document, error),
	judge func([]graphqldocs.Document, graphqldocs.Options) (graphqldocs.Result, error),
) {
	t.Helper()
	originalPairings, originalDocuments, originalJudge := collectPairings, readSDKDocuments, judgeDocuments
	collectPairings, readSDKDocuments, judgeDocuments = pairings, documents, judge
	t.Cleanup(func() {
		collectPairings, readSDKDocuments, judgeDocuments = originalPairings, originalDocuments, originalJudge
	})
}

// sdkDocument builds one document as the collector would hand it over.
func sdkDocument(name, file, text string) graphqldocs.Document {
	return graphqldocs.Document{
		Package:  "gitlab.com/gitlab-org/api/client-go/v3",
		Name:     name,
		Position: token.Position{Filename: filepath.Join(sdkModuleDir, file), Line: 10, Column: 2},
		Text:     text,
	}
}

// foundPairings answers with a module directory, which is what every case
// below needs before it can get anywhere.
func foundPairings(string) (structs.Pairings, error) {
	return structs.Pairings{ClientGoDir: sdkModuleDir}, nil
}

// TestSDKGraphQLCheck_TemplatesAreNamedAndNotJudged pins the one distinction
// that decides whether this section is readable.
//
// client-go writes two of its documents as shells with holes in them: a
// text/template for the work item list, and printf format strings for the
// Terraform state queries. Neither is text GitLab ever receives, both would be
// refused by any schema on every single run, and a refusal list with permanent
// entries in it is a list a reader learns to skip. They are counted apart and
// named instead, and only the sendable ones reach the schema.
func TestSDKGraphQLCheck_TemplatesAreNamedAndNotJudged(t *testing.T) {
	documents := []graphqldocs.Document{
		sdkDocument("listWorkItemsQueryShell", "workitems.go", "query ListWorkItems($fullPath: ID!{{ if .Decls }}, {{ .Decls }}{{ end }}) { x }"),
		sdkDocument("", "terraform_states.go", "query { project(fullPath: %q) { terraformStates { nodes { name } } } }"),
		sdkDocument("listAchievementsQuery", "achievements.go", "query ListAchievements($fullPath: ID!) { group(fullPath: $fullPath) { id } }"),
	}
	var judged []graphqldocs.Document
	withSDKSeams(t, foundPairings,
		func(string) ([]graphqldocs.Document, error) { return documents, nil },
		func(got []graphqldocs.Document, _ graphqldocs.Options) (graphqldocs.Result, error) {
			judged = got
			return graphqldocs.Result{Documents: got}, nil
		})

	check := sdkGraphQLCheck(t.TempDir())

	if !check.Ran || check.Documents != 3 || check.Judged != 1 {
		t.Fatalf("check = %+v, want three documents read and one judged", check)
	}
	if len(judged) != 1 || judged[0].Name != "listAchievementsQuery" {
		t.Errorf("the schema was handed %v, want the one document with no hole in it", judged)
	}
	if len(check.Templates) != 2 {
		t.Fatalf("templates = %+v, want the template shell and the format string", check.Templates)
	}
	for _, template := range check.Templates {
		t.Run(template.Position, func(t *testing.T) {
			if filepath.IsAbs(template.Position) || strings.Contains(template.Position, "pkg/mod") {
				t.Errorf("position = %q, want a path relative to the module", template.Position)
			}
		})
	}
}

// TestSDKGraphQLCheck_Refusals_NameAPathAnybodyCanFollow verifies that a
// refusal from the SDK is reported the way one from this repository is, minus
// the module cache prefix.
//
// The prefix is the whole difference. A path under internal/ names a file any
// reader can open; a module cache path names one that exists only on the
// machine that ran the audit, at a different place on every other, and a
// finding nobody can locate is a finding nobody acts on.
func TestSDKGraphQLCheck_Refusals_NameAPathAnybodyCanFollow(t *testing.T) {
	refused := sdkDocument("listAchievementsQuery", "achievements.go", "query ListAchievements { nope }")
	withSDKSeams(t, foundPairings,
		func(string) ([]graphqldocs.Document, error) { return []graphqldocs.Document{refused}, nil },
		func(got []graphqldocs.Document, _ graphqldocs.Options) (graphqldocs.Result, error) {
			return graphqldocs.Result{
				Documents: got,
				Refusals:  []graphqldocs.Refusal{{Document: got[0], Reasons: []string{`Cannot query field "nope"`}}},
			}, nil
		})

	check := sdkGraphQLCheck(t.TempDir())

	if len(check.Refusals) != 1 {
		t.Fatalf("check = %+v, want the one refusal", check)
	}
	refusal := check.Refusals[0]
	if refusal.Position != "achievements.go:10:2" {
		t.Errorf("position = %q, want it relative to the module directory", refusal.Position)
	}
	if refusal.Document != "listAchievementsQuery" || len(refusal.Reasons) != 1 {
		t.Errorf("refusal = %+v, want the document named with the schema's own reason", refusal)
	}
}

// TestSDKGraphQLCheck_UnreadableModule_IsANoteAndNotAFailure covers the two
// ways this check declines, both of which are facts about the machine rather
// than about the server.
//
// It reads a module cache whose state this process does not own: the directory
// may have been evicted, and the toolchain may refuse to load it. Neither says
// anything about whether a document GitLab would refuse has shipped, so the
// reason is published and R-PATH's verdict is left alone.
func TestSDKGraphQLCheck_UnreadableModule_IsANoteAndNotAFailure(t *testing.T) {
	cases := []struct {
		name     string
		pairings func(string) (structs.Pairings, error)
		read     func(string) ([]graphqldocs.Document, error)
		wantRan  bool
	}{
		{
			name:     "no module directory was resolved",
			pairings: func(string) (structs.Pairings, error) { return structs.Pairings{}, nil },
			read: func(string) ([]graphqldocs.Document, error) {
				t.Error("the documents were read without a module directory to read them from")
				return nil, nil
			},
		},
		{
			name:     "the pairing load failed",
			pairings: func(string) (structs.Pairings, error) { return structs.Pairings{}, errors.New("load: no packages") },
			read: func(string) ([]graphqldocs.Document, error) {
				t.Error("the documents were read after the pairing load failed")
				return nil, nil
			},
		},
		{
			name:     "the module would not load",
			pairings: foundPairings,
			read: func(string) ([]graphqldocs.Document, error) {
				return nil, errors.New("load the client-go source: directory not found")
			},
			wantRan: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			withSDKSeams(t, testCase.pairings, testCase.read,
				func([]graphqldocs.Document, graphqldocs.Options) (graphqldocs.Result, error) {
					t.Error("the schema was asked to judge documents that were never read")
					return graphqldocs.Result{}, nil
				})

			check := sdkGraphQLCheck(t.TempDir())

			if check.Ran != testCase.wantRan {
				t.Errorf("ran = %t, want %t", check.Ran, testCase.wantRan)
			}
			if testCase.wantRan && check.Error == "" {
				t.Error("the read failure was absorbed rather than published")
			}
			if len(check.Refusals) != 0 {
				t.Errorf("refusals = %+v, want no claim about any document", check.Refusals)
			}
		})
	}
}
