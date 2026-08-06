// action_specs_test.go contains unit tests for the snippets ActionSpec
// metadata: the decorator no-op on unknown tools, populated metadata, and
// the expected action count.
package snippets

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestDecorateSnippetMeta_UnknownToolIsNoop verifies decorateSnippetMeta leaves
// options untouched for a tool with no metadata entry.
func TestDecorateSnippetMeta_UnknownToolIsNoop(t *testing.T) {
	options := snippetOptions("gitlab_snippet_unknown")
	before := options.Usage
	decorateSnippetMeta(&options, "gitlab_snippet_unknown")
	if options.Usage != before {
		t.Errorf("Usage mutated for unknown tool: %q", options.Usage)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("RelatedActions set for unknown tool: %v", options.RelatedActions)
	}
}

// TestActionSpecs_MetadataPopulated verifies every snippet action spec carries
// non-generic discovery metadata (R-META): a custom usage, natural-language
// aliases, related actions, and a "Returns: … See also: …" description.
func TestActionSpecs_MetadataPopulated(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	for _, spec := range ActionSpecs(client) {
		tool := spec.IndividualTool.Name
		t.Run(tool, func(t *testing.T) {
			meta, ok := snippetActionMeta[tool]
			if !ok {
				t.Fatalf("no metadata entry for %s", tool)
			}
			if meta.usage == "" || spec.Usage == "Use to execute snippets domain action." {
				t.Errorf("%s has generic usage: %q", tool, spec.Usage)
			}
			if len(spec.Aliases) == 0 || spec.Aliases[0] == tool {
				t.Errorf("%s missing natural-language aliases: %v", tool, spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Errorf("%s missing related actions", tool)
			}
			desc := spec.IndividualTool.Description
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("%s description missing Returns:/See also: form: %q", tool, desc)
			}
		})
	}
}

// TestActionSpecs_Count guards that the snippet catalog still projects all 15
// canonical snippet actions after the audit.
func TestActionSpecs_Count(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	if n := len(ActionSpecs(client)); n != 15 {
		t.Fatalf("ActionSpecs count = %d, want 15", n)
	}
}
