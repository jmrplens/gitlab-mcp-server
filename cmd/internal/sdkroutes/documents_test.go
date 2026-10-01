package sdkroutes

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// inline places a document written inline in a fixture file at the line that
// holds marker, the way graphqldocs.SDKDocuments reports one.
func inline(t *testing.T, file, marker string) graphqldocs.Document {
	t.Helper()
	path := filepath.Join(fixtureDir, file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, marker) {
			return graphqldocs.Document{Position: token.Position{Filename: "/elsewhere/" + file, Line: i + 1}, Text: marker}
		}
	}
	t.Fatalf("%s holds no line with %q", path, marker)
	return graphqldocs.Document{}
}

// named is a document declared under a name.
func named(name string) graphqldocs.Document {
	return graphqldocs.Document{Name: name, Text: name}
}

// labels names documents for a comparison: the name, or the text of an inline
// one.
func labels(docs []graphqldocs.Document) []string {
	out := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc.Name != "" {
			out = append(out, doc.Name)
			continue
		}
		out = append(out, doc.Text)
	}
	return out
}

// TestSDK_Documents_AttributesEachDocumentToTheMethodThatSendsIt holds the
// attribution to the three ways a document reaches a sending method: named in
// its body, written inline in it, and reached through the initializers of the
// template variables it names, chained through one another and through a pair
// that name each other. A document no sending method reaches is attributed to
// none, and an inline one in another file at a line a body spans is not
// mistaken for one of its own.
func TestSDK_Documents_AttributesEachDocumentToTheMethodThatSendsIt(t *testing.T) {
	sdk := fixture(t)
	docs := []graphqldocs.Document{
		named("listAchievementsQuery"),
		named("templateSource"),
		named("unrelatedDocument"),
		inline(t, "legacy.go", "query { inlineInBody }"),
		inline(t, "declarations.go", "query { inlineInInitializer }"),
		inline(t, "declarations.go", "query { chained }"),
		inline(t, "declarations.go", "query { unrelated }"),
	}
	elsewhere := inline(t, "legacy.go", "query { inlineInBody }")
	elsewhere.Position.Filename = "/elsewhere/methods.go"
	docs = append(docs, elsewhere)
	cases := map[string][]string{
		"Achievements.ListAchievements":  {"listAchievementsQuery"},
		"Achievements.InlineAchievement": {"query { inlineInBody }"},
		"WorkItems.GetWorkItem":          {"templateSource", "query { inlineInInitializer }", "query { chained }"},
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			got := labels(sdk.Documents(key, docs))
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("Documents(%q) = %v, want %v", key, got, want)
			}
		})
	}
}

// TestSDK_Documents_MethodThatSendsNoGraphQL_HasNone verifies that a key
// client-go does not declare and a method that posts nothing to the GraphQL
// endpoint are attributed no document, however many are offered, even one the
// method names in its body.
func TestSDK_Documents_MethodThatSendsNoGraphQL_HasNone(t *testing.T) {
	docs := []graphqldocs.Document{named("listAchievementsQuery")}
	for _, key := range []string{"Absent.Method", "Issues.GetIssue", "WorkItems.NotGraphQL", "Achievements.DescribeAchievements"} {
		t.Run(key, func(t *testing.T) {
			if got := fixture(t).Documents(key, docs); got != nil {
				t.Errorf("Documents(%q) = %v, want none", key, got)
			}
		})
	}
}
