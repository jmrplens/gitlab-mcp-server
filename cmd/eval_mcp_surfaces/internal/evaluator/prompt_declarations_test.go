package evaluator

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// TestDeclaredPromptFindings_EachCoversAFindingOnItsSurface is the test that
// keeps the table honest, and it runs against the real corpus rather than a
// fixture.
//
// A declaration table is only worth having while it fails when it stops
// describing the tree. Both halves are asserted here: every case-site finding
// the audit reports has a declaration, so nothing is accepted silently, and
// every declaration covers something, so a rewritten or deleted case takes its
// declaration with it instead of leaving a sentence nobody will reread.
func TestDeclaredPromptFindings_EachCoversAFindingOnItsSurface(t *testing.T) {
	for _, surface := range []string{config.ToolSurfaceDynamic, config.ToolSurfaceMeta} {
		t.Run(surface, func(t *testing.T) {
			report := promptAuditForSurface(t, surface)
			if len(report.StaleDeclarations) > 0 {
				t.Errorf("declarations covering nothing on %s = %v", surface, report.StaleDeclarations)
			}
			if len(report.InvalidDeclarations) > 0 {
				t.Errorf("declarations the gate cannot act on = %v", report.InvalidDeclarations)
			}
			for _, audited := range report.Cases {
				for _, finding := range audited.Findings {
					if !slices.Contains(finding.Sites, promptSiteCase) || finding.Category != "" {
						continue
					}
					t.Errorf("%s: its own text names %s %q and nothing declares it", audited.ID, finding.Kind, finding.Value)
				}
			}
			if report.DeclaredFindings == 0 {
				t.Error("no declaration was used: this test would then pass for the wrong reason")
			}
		})
	}
}

// TestDeclaredPromptFindings_CategoriesAndSurfacesAreKnown verifies that no
// entry names a category or a surface the gate does not act on, since such an
// entry would excuse nothing and would never be reported stale either.
func TestDeclaredPromptFindings_CategoriesAndSurfacesAreKnown(t *testing.T) {
	for _, declaration := range declaredPromptFindings {
		t.Run(declaration.key(), func(t *testing.T) {
			if problem := promptDeclarationProblem(declaration); problem != "" {
				t.Error(problem)
			}
			if strings.TrimSpace(declaration.Reason) == "" {
				t.Error("a declaration with no reason is an exception nobody can review")
			}
		})
	}
}

// TestPromptDeclarationProblem_NamesWhatIsWrong covers the two ways an entry is
// refused, since both are what stop a typo becoming a silent exemption.
func TestPromptDeclarationProblem_NamesWhatIsWrong(t *testing.T) {
	tests := []struct {
		name        string
		declaration promptDeclaration
		want        string
	}{
		{
			name:        "an unaudited surface",
			declaration: promptDeclaration{Surface: config.ToolSurfaceIndividual, Case: "MS-040", Kind: promptLeakTool, Value: "gitlab_get_project", Category: promptCategoryLiteralIsTheRequest},
			want:        "names a surface the audit never renders",
		},
		{
			name:        "a category outside the set",
			declaration: promptDeclaration{Surface: config.ToolSurfaceMeta, Case: "MS-040", Kind: promptLeakTool, Value: "gitlab_project", Category: "gitlab-vocabulary"},
			want:        "is not one of the declared kinds",
		},
		{
			name:        "a declaration the gate acts on",
			declaration: promptDeclaration{Surface: config.ToolSurfaceMeta, Case: "MS-040", Kind: promptLeakTool, Value: "gitlab_project", Category: promptCategoryLiteralIsTheRequest},
			want:        "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := promptDeclarationProblem(tc.declaration)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("promptDeclarationProblem() = %q, want it to accept the declaration", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("promptDeclarationProblem() = %q, want it to name %q", got, tc.want)
			}
		})
	}
}
