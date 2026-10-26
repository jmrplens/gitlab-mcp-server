package mrchanges

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_ChangesGetPagination_DescribesOffsetContinuation verifies that callers are guided through offset pages without advertising keyset.
func TestActionSpecs_ChangesGetPagination_DescribesOffsetContinuation(t *testing.T) {
	var changes toolutil.ActionSpec
	for _, spec := range ActionSpecs(nil) {
		if spec.Name == "changes_get" {
			changes = spec
			break
		}
	}
	if changes.Name == "" {
		t.Fatal("changes_get action is missing")
	}
	usage := changes.Usage
	for _, name := range []string{"next_page as page", "has_more", "per_page: 100"} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(usage, name) {
				t.Errorf("usage lacks %s: %s", name, usage)
			}
		})
	}
	for _, name := range []string{"keyset", "page_token"} {
		t.Run("unadvertised_"+name, func(t *testing.T) {
			if strings.Contains(strings.ToLower(usage), name) {
				t.Errorf("usage advertises %s: %s", name, usage)
			}
		})
	}
	for _, name := range []string{"page", "per_page"} {
		t.Run("guidance_"+name, func(t *testing.T) {
			if _, ok := changes.ParameterGuidance[name]; !ok {
				t.Errorf("parameter guidance lacks %s", name)
			}
		})
	}
	if _, ok := changes.ParameterGuidance["page_token"]; ok {
		t.Error("parameter guidance advertises page_token for MR diffs")
	}
}
