//go:build e2e

// search_test.go covers the search scope an unlicensed instance does not
// serve: a code search across the whole instance, which GitLab only offers
// with advanced search and which the Community Edition does not declare
// as a scope at all.

package ce

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestSearch_GlobalCode_RefusedWithoutAdvancedSearch asks for a code search
// with no project and no group on every surface and reads the refusal:
// GitLab answers 400 for a scope the instance does not offer, and the
// server relays it with the hint that names the scope and the search_type
// a caller might have supplied. The scenario belongs here because a
// licensed instance with advanced search would answer the same call with
// results.
//
// Replaces: TestSearchType_InvalidValueFails
func TestSearch_GlobalCode_RefusedWithoutAdvancedSearch(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		refusal := harness.ExpectToolError(s, actionSearchCode, map[string]any{"query": "anything"}, "scope")
		lowered := strings.ToLower(refusal)
		if !strings.Contains(lowered, "400") && !strings.Contains(lowered, "not have a valid value") && !strings.Contains(lowered, "not supported") {
			line, _, _ := strings.Cut(strings.TrimSpace(refusal), "\n")
			e.T.Errorf("the unlicensed instance answered the global code search with something other than a refused scope: %s", line)
		}
	})
}
