//go:build e2e

// vulnerabilities_test.go tests the GitLab vulnerability GraphQL MCP tools
// against a live GitLab instance. Requires GitLab Premium/Ultimate
// (GITLAB_ENTERPRISE=true) — tests are skipped otherwise.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/vulnerabilities"
)

// TestIndividual_Vulnerabilities exercises vulnerability GraphQL tools
// via individual MCP tools.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestIndividual_Vulnerabilities(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProject(ctx, t, sess.individual)

	t.Run("SeverityCount", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.SeverityCountOutput](ctx, sess.individual, "gitlab_vulnerability_severity_count", vulnerabilities.SeverityCountInput{
			ProjectPath: proj.Path,
		})
		requireNoError(t, err, "vulnerability severity_count")
		requireTruef(t, out.Total >= 0, "expected non-negative total, got %d", out.Total)
		t.Logf("Vulnerability severity counts: critical=%d high=%d medium=%d low=%d total=%d",
			out.Critical, out.High, out.Medium, out.Low, out.Total)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.ListOutput](ctx, sess.individual, "gitlab_list_vulnerabilities", vulnerabilities.ListInput{
			ProjectPath: proj.Path,
		})
		requireNoError(t, err, "list vulnerabilities")
		t.Logf("Project %s has %d vulnerabilities", proj.Path, len(out.Vulnerabilities))
	})
}

// TestMeta_Vulnerabilities exercises vulnerability tools via the
// gitlab_vulnerability meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_Vulnerabilities(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Vulnerability/SeverityCount", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.SeverityCountOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "severity_count",
			"params": map[string]any{
				"project_path": proj.Path,
			},
		})
		requireNoError(t, err, "meta vulnerability severity_count")
		requireTruef(t, out.Total >= 0, "expected non-negative total, got %d", out.Total)
		t.Logf("Vulnerability severity counts via meta-tool: total=%d", out.Total)
	})

	t.Run("Meta/Vulnerability/List", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.ListOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
			},
		})
		requireNoError(t, err, "meta vulnerability list")
		t.Logf("Project %s has %d vulnerabilities (via meta-tool)", proj.Path, len(out.Vulnerabilities))
	})
}
