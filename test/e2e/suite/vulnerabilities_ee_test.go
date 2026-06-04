//go:build e2e && enterprise

// vulnerabilities_test.go tests the GitLab vulnerability GraphQL MCP tools
// against a live GitLab instance. Requires GitLab Premium/Ultimate
// (GITLAB_ENTERPRISE=true); CE runs return before making GitLab calls.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/vulnerabilities"
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

// TestMeta_VulnerabilityLifecycle exercises the full vulnerability mutation
// lifecycle via gitlab_vulnerability: get, dismiss, confirm, resolve,
// revert, and pipeline_security_summary. The fixture is a project with
// a SAST-enabled CI pipeline (Security/SAST.gitlab-ci.yml template)
// and an intentionally vulnerable Python file (SQL injection,
// command injection, eval) so GitLab's Semgrep-based SAST analyzer
// reports real CRITICAL findings on the pipeline run.
//
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
// Skips on CE or when the runner is not configured (E2E_MODE=ce) or
// when the pipeline cannot complete (e.g. resource pressure on the
// ephemeral GitLab instance).
func TestMeta_VulnerabilityLifecycle(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if !isDockerMode() {
		t.Skip("vulnerability lifecycle fixture requires a real runner (Docker mode only)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	t.Cleanup(func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":         strconv.FormatInt(proj.ID, 10),
				"permanently_remove": true,
				"full_path":          proj.Path,
			},
		})
	})

	// 1. Commit a .gitlab-ci.yml that runs SAST with the standard
	// GitLab Semgrep analyzer. We pin the major version of the
	// template so the job is reproducible across GitLab releases. The
	// Semgrep ruleset reliably flags SQL injection, command
	// injection, and eval() patterns on Python as CRITICAL.
	// Per https://docs.gitlab.com/user/application_security/sast/
	// the Standard analyzer template works for all languages GitLab
	// supports out of the box.
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		".gitlab-ci.yml",
		`include:
  - template: Security/SAST.gitlab-ci.yml
`,
		"add SAST pipeline")

	// 2. Commit a benign file first, so the vulnerable code is
	// introduced in a non-initial commit. The SAST analyzer scans
	// the diff; introducing the flaw in a follow-up commit
	// guarantees there is a diff to scan.
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		"placeholder.txt",
		"placeholder content\n",
		"add placeholder file for fixture")

	// 3. Commit a Python file with classic vulnerability patterns
	// that GitLab's Semgrep-based SAST analyzer reliably flags as
	// CRITICAL findings: SQL injection via string concatenation,
	// command injection via os.system, and use of eval() on user
	// input. These match standard Semgrep rules (python.lang.security
	// .audit.formatted-sql-query, python.lang.security.audit.dangerous-system-call,
	// python.lang.security.audit.eval).
	const vulnerablePy = `# E2E fixture: intentionally vulnerable code for SAST.
def get_user(username):
    # SQL injection (CWE-89): taint from user input flows into a
    # formatted SQL query without parameterization.
    cursor.execute("SELECT * FROM users WHERE name = '" + username + "'")
    return cursor.fetchone()

def run_command(cmd):
    # Command injection (CWE-78): unsanitized input into os.system.
    os.system("ls " + cmd)

def calc(expr):
    # Code injection (CWE-95): eval on user input.
    return eval(expr)
`
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		"app.py",
		vulnerablePy,
		"add intentionally vulnerable code for E2E SAST fixture")

	// 4. Manually trigger a pipeline so the runner processes the
	// SAST job.
	created, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"ref":        defaultBranch,
		},
	})
	if err != nil {
		t.Skipf("could not trigger vulnerability pipeline (fixture not available): %v", err)
	}
	pipelineID := created.ID
	pipelineIID := strconv.FormatInt(created.IID, 10)
	t.Logf("Triggered pipeline ID=%d IID=%s", pipelineID, pipelineIID)

	// 5. Wait for the pipeline to reach a terminal state. The runner
	// is shared with the rest of the suite, so this can take a few
	// minutes. We tolerate transient connection failures and skip the
	// lifecycle if GitLab appears unhealthy.
	pipelineStatus := waitForPipelineStatusEE(ctx, t, sess.glClient, proj.ID, pipelineID, 300*time.Second)
	if pipelineStatus != "success" && pipelineStatus != "failed" && pipelineStatus != "canceled" && pipelineStatus != "skipped" {
		t.Skipf("pipeline %d did not reach a terminal status (last status: %s); skipping lifecycle to avoid fixture flakiness", pipelineID, pipelineStatus)
	}
	t.Logf("Pipeline %d status: %s", pipelineID, pipelineStatus)

	// 5. List vulnerabilities for the project. SAST creates CRITICAL
	// findings (SQL injection, command injection, eval) from the
	// intentionally vulnerable Python file.
	listed, err := callToolOn[vulnerabilities.ListOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
		"action": "list",
		"params": map[string]any{
			"project_path": proj.Path,
		},
	})
	requireNoError(t, err, "vulnerability list for lifecycle")
	if len(listed.Vulnerabilities) == 0 {
		t.Skipf("no vulnerabilities reported for project %s after pipeline %d; cannot exercise lifecycle actions", proj.Path, pipelineID)
	}
	var vulnGID string
	for _, v := range listed.Vulnerabilities {
		if v.Severity == "CRITICAL" || v.Severity == "HIGH" {
			vulnGID = v.ID
			t.Logf("Using vulnerability %q (severity=%s, state=%s)", v.ID, v.Severity, v.State)
			break
		}
	}
	if vulnGID == "" {
		vulnGID = listed.Vulnerabilities[0].ID
		t.Logf("Falling back to first vulnerability %q", vulnGID)
	}

	// 6. Exercise the vulnerability mutation actions.

	t.Run("Meta/Vulnerability/Get", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.GetOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "get",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability get")
		requireTruef(t, out.Vulnerability.ID == vulnGID, "vulnerability ID = %q, want %q", out.Vulnerability.ID, vulnGID)
		t.Logf("Got vulnerability %q state=%s", out.Vulnerability.ID, out.Vulnerability.State)
	})

	t.Run("Meta/Vulnerability/Confirm", func(t *testing.T) {
		_, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "confirm",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability confirm")
		t.Logf("Confirmed vulnerability %q", vulnGID)
	})

	t.Run("Meta/Vulnerability/Resolve", func(t *testing.T) {
		_, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "resolve",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability resolve")
		t.Logf("Resolved vulnerability %q", vulnGID)
	})

	t.Run("Meta/Vulnerability/Revert", func(t *testing.T) {
		_, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "revert",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability revert")
		t.Logf("Reverted vulnerability %q to detected state", vulnGID)
	})

	t.Run("Meta/Vulnerability/Dismiss", func(t *testing.T) {
		_, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "dismiss",
			"params": map[string]any{
				"id":               vulnGID,
				"comment":          "E2E dismiss test",
				"dismissal_reason": "USED_IN_TESTS",
			},
		})
		requireNoError(t, err, "vulnerability dismiss")
		t.Logf("Dismissed vulnerability %q", vulnGID)
	})

	t.Run("Meta/Vulnerability/PipelineSecuritySummary", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.PipelineSecuritySummaryOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "pipeline_security_summary",
			"params": map[string]any{
				"project_path": proj.Path,
				"pipeline_iid": pipelineIID,
			},
		})
		requireNoError(t, err, "vulnerability pipeline_security_summary")
		t.Logf("Pipeline security summary: SAST=%v DAST=%v DepScanning=%v ContainerScanning=%v",
			out.Sast != nil, out.Dast != nil, out.DependencyScanning != nil, out.ContainerScanning != nil)
	})
}
