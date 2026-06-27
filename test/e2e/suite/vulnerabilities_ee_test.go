//go:build e2e && enterprise

// vulnerabilities_test.go tests the GitLab vulnerability GraphQL MCP tools
// against a live GitLab instance. Requires GitLab Premium/Ultimate
// (GITLAB_ENTERPRISE=true); CE runs return before making GitLab calls.
package suite

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

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
// revert, and pipeline_security_summary. The fixture is a project whose
// pipeline publishes a deterministic gl-sast-report.json as an
// artifacts:reports:sast report (three CRITICAL findings: SQL injection,
// command injection, eval). GitLab ingests it into the project Vulnerability
// Report exactly as a real scan would, so the test validates our vulnerability
// MCP tools — not GitLab's Semgrep analyzer, whose version-pinned image is
// unreliable to pull on the ephemeral Docker runner.
//
// Requires GitLab Ultimate (the project Vulnerability Report is Ultimate-only;
// GITLAB_ENTERPRISE=true / an Ultimate license).
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

	// This fixture verifies the GitLab vulnerability *reporting* path and our
	// vulnerability MCP tools end-to-end — NOT GitLab's Semgrep analyzer itself.
	// Earlier revisions ran the real `Security/SAST.gitlab-ci.yml` template, but
	// that job pulls a heavy, version-pinned analyzer image
	// (registry.gitlab.com/security-products/semgrep:<major>) which is unreliable
	// on the ephemeral Docker runner: when the pull fails the job's default
	// `allow_failure: true` left the pipeline "success" with no security report,
	// so the lifecycle silently found zero vulnerabilities.
	//
	// Instead the pipeline *generates* a deterministic, schema-valid
	// gl-sast-report.json in-job and publishes it as an artifacts:reports:sast
	// report. The job runs on the pre-pulled alpine image with GIT_STRATEGY:none
	// (no repo clone — the report is self-contained), so it depends on nothing
	// the ephemeral runner might fail to pull or fetch. GitLab ingests the report
	// into the project Vulnerability Report exactly as a real scan would
	// (Ultimate, default-branch pipeline), which is what exercises
	// gitlab_vulnerability list/get/resolve/etc.

	// 1. Commit the intentionally vulnerable source the report points at. It is
	// not scanned (GIT_STRATEGY:none), but keeps the fixture self-documenting and
	// gives the report's `location.file` a real target in the repo.
	const vulnerablePy = `# E2E fixture: intentionally vulnerable code referenced by the SAST report.
import os
import sqlite3
db = sqlite3.connect(":memory:")
cur = db.cursor()

# CWE-89: SQL injection.
user = input("username: ")
cur.execute("SELECT * FROM users WHERE name = '" + user + "'")

# CWE-78: command injection.
cmd = input("cmd: ")
os.system("ls " + cmd)

# CWE-95: code injection.
expr = input("expr: ")
result = eval(expr)
`
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		"app.py",
		vulnerablePy,
		"add intentionally vulnerable code for E2E SAST fixture")

	// 2. Commit a .gitlab-ci.yml whose single job writes a deterministic SAST
	// report (GitLab secure-report schema 15.x, three CRITICAL findings:
	// SQLi/command-injection/eval) and publishes it as the pipeline's SAST
	// security report. printf with a single-quoted minified JSON payload avoids
	// heredoc/indentation pitfalls; the JSON contains only double quotes so the
	// single-quote wrapping is safe. `when: always` uploads the report even if a
	// later step were to fail.
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		".gitlab-ci.yml",
		`stages:
  - test

sast:
  stage: test
  image: alpine:latest
  variables:
    GIT_STRATEGY: none
  script:
    - |
      printf '%s' '{"version":"15.0.6","scan":{"analyzer":{"id":"e2e-fixture","name":"E2E Fixture","version":"1.0.0","vendor":{"name":"gitlab-mcp-server"}},"scanner":{"id":"e2e-fixture","name":"E2E Fixture","version":"1.0.0","vendor":{"name":"gitlab-mcp-server"}},"type":"sast","start_time":"2026-01-01T00:00:00","end_time":"2026-01-01T00:00:01","status":"success"},"vulnerabilities":[{"id":"e2e-sast-sqli-0001","category":"sast","name":"SQL Injection","message":"SQL Injection","description":"User input concatenated into a SQL query (CWE-89).","severity":"Critical","scanner":{"id":"e2e-fixture","name":"E2E Fixture"},"location":{"file":"app.py","start_line":10,"end_line":10},"identifiers":[{"type":"cwe","name":"CWE-89","value":"89","url":"https://cwe.mitre.org/data/definitions/89.html"}]},{"id":"e2e-sast-cmdi-0002","category":"sast","name":"OS Command Injection","message":"OS Command Injection","description":"User input flows into os.system (CWE-78).","severity":"Critical","scanner":{"id":"e2e-fixture","name":"E2E Fixture"},"location":{"file":"app.py","start_line":14,"end_line":14},"identifiers":[{"type":"cwe","name":"CWE-78","value":"78","url":"https://cwe.mitre.org/data/definitions/78.html"}]},{"id":"e2e-sast-eval-0003","category":"sast","name":"Code Injection","message":"Code Injection","description":"User input passed to eval (CWE-95).","severity":"Critical","scanner":{"id":"e2e-fixture","name":"E2E Fixture"},"location":{"file":"app.py","start_line":18,"end_line":18},"identifiers":[{"type":"cwe","name":"CWE-95","value":"95","url":"https://cwe.mitre.org/data/definitions/95.html"}]}]}' > gl-sast-report.json
      echo "wrote gl-sast-report.json ($(wc -c < gl-sast-report.json) bytes)"
  artifacts:
    when: always
    reports:
      sast: gl-sast-report.json
`,
		"add SAST report-publishing pipeline")

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

	// 5. List vulnerabilities for the project. GitLab ingests the published
	// gl-sast-report.json into the project Vulnerability Report after the
	// default-branch pipeline finishes, creating the three CRITICAL findings.
	// Ingestion is asynchronous, so the GraphQL list endpoint can return an
	// empty page for a few seconds even after the pipeline succeeds. Poll with
	// backoff up to 90 s before declaring the fixture broken — this covers the
	// slow path on a heavily-loaded Docker instance while still failing fast.
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
			listErr = fmt.Errorf("context canceled while waiting for vulnerability report: %w", ctx.Err())
		case <-time.After(listDelay):
		}
		if listDelay < 8*time.Second {
			listDelay *= 2
		}
	}
	requireNoError(t, listErr, "vulnerability list for lifecycle")
	if len(listed.Vulnerabilities) == 0 {
		// Dump the pipeline's job statuses and the trace of any failed job so a
		// fixture/runner regression is diagnosable from the test log alone.
		logPipelineJobDiagnostics(ctx, t, proj.ID, pipelineID)
		t.Fatalf("no vulnerabilities reported for project %s after pipeline %d (status: %s) and 90s of polling; the SAST report was not ingested — see the job diagnostics above, and verify the 'sast' job uploaded the artifacts:reports:sast report and the instance is Ultimate (the project Vulnerability Report is Ultimate-only)",
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

// logPipelineJobDiagnostics logs every job of a pipeline (name, stage, status,
// allow_failure) and the trace tail of any failed job. It is used when the
// vulnerability lifecycle finds no findings so a runner/fixture regression
// (image pull, artifact upload, report generation) is diagnosable from the test
// log alone. All failures are logged, never fatal — this is best-effort
// diagnostics around an already-failing assertion.
func logPipelineJobDiagnostics(ctx context.Context, t *testing.T, projectID, pipelineID int64) {
	t.Helper()
	jobs, _, err := sess.glClient.GL().Jobs.ListPipelineJobs(projectID, pipelineID, nil, gl.WithContext(ctx))
	if err != nil {
		t.Logf("diagnostics: could not list jobs for pipeline %d: %v", pipelineID, err)
		return
	}
	for _, j := range jobs {
		t.Logf("diagnostics: job id=%d name=%q stage=%q status=%q allow_failure=%v",
			j.ID, j.Name, j.Stage, j.Status, j.AllowFailure)
		if j.Status != "failed" {
			continue
		}
		trace, _, terr := sess.glClient.GL().Jobs.GetTraceFile(projectID, j.ID, gl.WithContext(ctx))
		if terr != nil {
			t.Logf("diagnostics: could not fetch trace for job %d: %v", j.ID, terr)
			continue
		}
		raw, rerr := io.ReadAll(trace)
		if rerr != nil {
			t.Logf("diagnostics: could not read trace for job %d: %v", j.ID, rerr)
			continue
		}
		out := string(raw)
		if len(out) > 2000 {
			out = "…" + out[len(out)-2000:]
		}
		t.Logf("diagnostics: job %d (%s) trace tail:\n%s", j.ID, j.Name, out)
	}
}
