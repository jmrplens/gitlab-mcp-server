//go:build e2e && enterprise

// vulnerabilities_test.go tests the GitLab vulnerability GraphQL MCP tools
// against a live GitLab instance. Requires GitLab Premium/Ultimate
// (GITLAB_ENTERPRISE=true); CE runs return before making GitLab calls.
package suite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/vulnerabilities"
)

// TestIndividual_Vulnerabilities exercises vulnerability GraphQL tools
// through individual MCP tools against a live GitLab EE Premium/Ultimate instance.
//
// The test creates a project fixture, then walks the individual
// vulnerability list and detail tools, asserting they return the expected
// GraphQL payloads. The test returns early when the running GitLab does
// not report EE capabilities.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: individual. Requires Premium/Ultimate.
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

// TestMeta_Vulnerabilities exercises vulnerability tools through the
// gitlab_vulnerability meta-tool against a live GitLab EE Premium/Ultimate instance.
//
// The test creates a project fixture, then walks the catalog-backed list
// and detail actions via {action, params} arguments. Each subtest asserts
// the meta-tool returns the expected GraphQL payload for the project's
// vulnerability report.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta. Requires Premium/Ultimate.
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
	// Multiple vulnerability patterns to maximize Semgrep rule matches
	// against the GitLab SAST default ruleset. Each function has a clear
	// taint source (input() builtin) flowing into a classic dangerous sink.
	// Patterns covered:
	//   - SQL injection via concat, % formatting, f-string, .format
	//     (rules: python.lang.security.audit.formatted-sql-query and
	//     python.flask.security.audit.sql-injection-taint, etc.)
	//   - Command injection via os.system, os.popen, subprocess with
	//     shell=True
	//     (rules: python.lang.security.audit.dangerous-system-call)
	//   - Code injection via eval, exec
	//     (rules: python.lang.security.audit.eval, python.lang.security
	//     .audit.exec)
	const vulnerablePy = `# E2E fixture: intentionally vulnerable code for SAST.
import os
import subprocess
import sqlite3
db = sqlite3.connect(":memory:")
cur = db.cursor()

# CWE-89: SQL injection — clear taint source (input) into multiple
# concatenation/formatting patterns.
user = input("username: ")
cur.execute("SELECT * FROM users WHERE name = '" + user + "'")
cur.execute("SELECT * FROM users WHERE id = %s" % user)
cur.execute(f"SELECT * FROM users WHERE email = '{user}'")
cur.execute("SELECT * FROM users WHERE name = '{}'".format(user))

# CWE-78: command injection — taint source into shell-invoking calls.
cmd = input("cmd: ")
os.system("ls " + cmd)
os.system(cmd)
os.popen("cat " + cmd)
subprocess.call("echo " + cmd, shell=True)

# CWE-95: code injection — eval/exec on user input.
expr = input("expr: ")
result = eval(expr)
exec(expr)
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
	// intentionally vulnerable file. The vulnerability report is
	// generated asynchronously after the pipeline job completes, so
	// the GraphQL list endpoint can return an empty page for a few
	// seconds even when the SAST job ran successfully. Poll with
	// backoff up to 90 s before declaring the fixture broken — this
	// covers the slow path on a heavily-loaded Docker runner while
	// still failing fast when the scanner is actually missing.
	var (
		listed       vulnerabilities.ListOutput
		listErr      error
		listDeadline = time.Now().Add(90 * time.Second)
		listDelay    = 2 * time.Second
	)
	for {
		listed, listErr = callToolOn[vulnerabilities.ListOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
			},
		})
		if listErr == nil && len(listed.Vulnerabilities) > 0 {
			break
		}
		if listErr != nil {
			break
		}
		if time.Now().After(listDeadline) {
			break
		}
		select {
		case <-ctx.Done():
			listErr = fmt.Errorf("context canceled while waiting for SAST report: %w", ctx.Err())
		case <-time.After(listDelay):
		}
		if listDelay < 8*time.Second {
			listDelay *= 2
		}
	}
	requireNoError(t, listErr, "vulnerability list for lifecycle")
	if len(listed.Vulnerabilities) == 0 {
		t.Fatalf("no vulnerabilities reported for project %s after pipeline %d (status: %s) and 90s of polling; SAST scanner did not flag the intentionally vulnerable fixture — check that the Security/SAST.gitlab-ci.yml template is available and the runner has the semgrep analyzer installed",
			proj.Path, pipelineID, pipelineStatus)
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
