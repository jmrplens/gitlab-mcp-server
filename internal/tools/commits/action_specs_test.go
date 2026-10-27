// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go holds the commit actions' schemas to the SHA each
// route reads: a full one where GitLab compares it as written, any revision
// elsewhere.
package commits

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_CommitStatusesSHA_IsTheOneHeldToAFullSHA verifies that
// repository.commit_statuses publishes its sha with
// toolutil.FullCommitSHAPattern: GitLab finds the commit by any revision and
// then lists the statuses whose pipeline sha equals the value as written, so
// a branch name or an abbreviated SHA lists none (issue 1272). Every other
// commit action resolves the revision to its commit and keeps no pattern,
// so the branch names and short SHAs it takes still pass.
func TestActionSpecs_CommitStatusesSHA_IsTheOneHeldToAFullSHA(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			sha, _ := properties["sha"].(map[string]any)
			var want any
			if spec.Name == "commit_statuses" {
				want = toolutil.FullCommitSHAPattern
			}
			if got := sha["pattern"]; got != want {
				t.Errorf("%s sha pattern = %v, want %v", spec.Name, got, want)
			}
		})
	}
	if got := commitSpecsByTool(t, ActionSpecs(nil))["gitlab_commit_statuses"].ParameterGuidance["sha"].ExampleBinding; got != `params.sha:"5957a570eee0ac4580ec027fb874ad7514d1e576"` {
		t.Errorf("commit_statuses sha guidance binds %s, want a full SHA", got)
	}
}
