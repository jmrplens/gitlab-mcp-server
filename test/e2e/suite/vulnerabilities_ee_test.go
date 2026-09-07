//go:build e2e && enterprise

// vulnerabilities_test.go tests the GitLab vulnerability GraphQL MCP tools
// against a live GitLab instance. Requires GitLab Premium/Ultimate
// (GITLAB_ENTERPRISE=true); CE runs return before making GitLab calls.
package suite

import (
	"context"
	"io"
	"slices"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securityfindings"
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

// TestMeta_VulnerabilityLifecycle exercises the full vulnerability lifecycle
// via gitlab_vulnerability and gitlab_security_finding: the report is read back
// through list, pipeline_security_summary and the pipeline's security findings
// before anything is mutated, then get, confirm, resolve, revert and dismiss
// walk the states. The fixture is a project whose pipeline
// publishes a deterministic gl-sast-report.json as an
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

	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
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

	// 1. Commit the intentionally vulnerable source the report points at.
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		"app.py",
		vulnerabilityFixtureSource,
		"add intentionally vulnerable code for E2E SAST fixture")

	// 2. Commit the pipeline that publishes the SAST report.
	commitFileMeta(ctx, t, sess.meta, proj, defaultBranch,
		".gitlab-ci.yml",
		vulnerabilityFixturePipeline,
		"add SAST report-publishing pipeline")

	// 3. Manually trigger a pipeline so the runner processes the
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

	// 4. Wait for the pipeline to reach a terminal state. The runner
	// is shared with the rest of the suite, so this can take a few
	// minutes. We tolerate transient connection failures and skip the
	// lifecycle if GitLab appears unhealthy.
	pipelineStatus := waitForPipelineStatusEE(ctx, t, sess.glClient, proj.ID, pipelineID, 300*time.Second)
	if pipelineStatus != "success" && pipelineStatus != "failed" && pipelineStatus != "canceled" && pipelineStatus != "skipped" {
		t.Skipf("pipeline %d did not reach a terminal status (last status: %s); skipping lifecycle to avoid fixture flakiness", pipelineID, pipelineStatus)
	}
	t.Logf("Pipeline %d status: %s", pipelineID, pipelineStatus)

	// 5. Wait for GitLab to promote the published SAST findings into the project
	// Vulnerability Report. This ingestion is asynchronous (Sidekiq) and only
	// runs for the default-branch pipeline; on a heavily loaded ephemeral
	// instance it can lag or, occasionally, not complete within the test budget.
	//
	// The wait asks GitLab directly rather than through gitlab_vulnerability,
	// because the tool is what this lifecycle exists to test: a handler whose
	// GraphQL document GitLab refuses gets no data back, asks REST whether the
	// project exists, and answers an empty list with no error. Deriving the
	// fixture from it would therefore skip every assertion below in precisely
	// the state they were written to catch, which is what a licensed Ultimate
	// run did while three of these tools could not work at all.
	fixtureVulns := waitForProjectVulnerabilities(ctx, t, proj.Path, vulnerabilityReportWait)
	if len(fixtureVulns) == 0 {
		// GitLab's own vulnerability report is empty, so there is nothing here
		// for a tool to have hidden and nothing for the lifecycle to act on.
		// Distinguish a broken fixture from an environment that simply cannot
		// perform the async promotion: the pipeline-level security report
		// summary is derived directly from the uploaded artifact, so findings
		// there mean the report WAS published and parsed and only the
		// project-level ingestion is missing, which is outside our control on
		// the ephemeral runner.
		sastParsed := pipelineSASTFindingCount(ctx, t, proj.Path, pipelineIID)
		logPipelineJobDiagnostics(ctx, t, proj.ID, pipelineID)
		if sastParsed > 0 {
			t.Skipf("SAST report published and parsed (%d findings in pipeline %d's security summary) but GitLab did not promote them into the project Vulnerability Report within %s on this ephemeral instance; skipping the mutation lifecycle (async default-branch ingestion unavailable here). project=%s",
				sastParsed, pipelineID, vulnerabilityReportWait, proj.Path)
		}
		// Neither the project Vulnerability Report nor the pipeline security
		// summary reported findings. The 'sast' job succeeded and uploaded the
		// artifact (see diagnostics above) and the fixture report is validated in
		// CI, so this indicates the ephemeral instance is not performing security
		// report ingestion at all (Sidekiq pressure / disabled processing) rather
		// than a broken fixture. Skip rather than fail the whole EE suite over an
		// environment limitation.
		t.Skipf("pipeline %d (status: %s) uploaded a SAST report but this instance ingested no findings into either the pipeline security summary or the project Vulnerability Report within %s; skipping the mutation lifecycle (security report ingestion appears unavailable on this ephemeral instance). project=%s",
			pipelineID, pipelineStatus, vulnerabilityReportWait, proj.Path)
	}
	var vulnGID string
	for _, v := range fixtureVulns {
		if v.Severity == "CRITICAL" || v.Severity == "HIGH" {
			vulnGID = v.ID
			t.Logf("Using vulnerability %q (severity=%s, state=%s)", v.ID, v.Severity, v.State)
			break
		}
	}
	if vulnGID == "" {
		vulnGID = fixtureVulns[0].ID
		t.Logf("Falling back to first vulnerability %q", vulnGID)
	}

	// 6. Read the report back through the tools, while GitLab's own answer is
	// known and nothing has been mutated yet.
	assertVulnerabilityReadBacks(ctx, t, proj, pipelineIID, vulnGID, len(fixtureVulns))

	// 7. Exercise the vulnerability mutation actions.

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
		out, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "confirm",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability confirm")
		assertVulnerabilityState(t, out, vulnGID, "CONFIRMED")
		t.Logf("Confirmed vulnerability %q", vulnGID)
	})

	t.Run("Meta/Vulnerability/Resolve", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "resolve",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability resolve")
		assertVulnerabilityState(t, out, vulnGID, "RESOLVED")
		t.Logf("Resolved vulnerability %q", vulnGID)
	})

	t.Run("Meta/Vulnerability/Revert", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "revert",
			"params": map[string]any{"id": vulnGID},
		})
		requireNoError(t, err, "vulnerability revert")
		assertVulnerabilityState(t, out, vulnGID, "DETECTED")
		t.Logf("Reverted vulnerability %q to detected state", vulnGID)
	})

	t.Run("Meta/Vulnerability/Dismiss", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.MutationOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "dismiss",
			"params": map[string]any{
				"id":               vulnGID,
				"comment":          "E2E dismiss test",
				"dismissal_reason": "USED_IN_TESTS",
			},
		})
		requireNoError(t, err, "vulnerability dismiss")
		assertVulnerabilityState(t, out, vulnGID, "DISMISSED")
		t.Logf("Dismissed vulnerability %q", vulnGID)
	})
}

// assertVulnerabilityReadBacks reads the fixture's own SAST report back through
// every read tool the lifecycle covers, while GitLab's answer to the same
// question is known and no state mutation has run yet. reported is how many
// vulnerabilities GitLab itself listed, which the failure message names so a
// disagreement between the two is legible.
func assertVulnerabilityReadBacks(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineIID, vulnGID string, reported int) {
	t.Helper()

	t.Run("Meta/Vulnerability/List", func(t *testing.T) {
		out, err := callToolOn[vulnerabilities.ListOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
			},
		})
		requireNoError(t, err, "vulnerability list for lifecycle")
		listed := make([]string, 0, len(out.Vulnerabilities))
		for _, v := range out.Vulnerabilities {
			listed = append(listed, v.ID)
		}
		// GitLab answered this same question a moment ago and named this
		// vulnerability, so a list that does not hold it is the tool rather
		// than the instance. This is the assertion a refused list document
		// fails: the handler turns the data GitLab never sent into an empty
		// list and no error, which is why a licensed Ultimate run passed
		// while the tool could not work at all.
		requireTruef(t, slices.Contains(listed, vulnGID),
			"gitlab_vulnerability list does not report %q, which GitLab's own vulnerability report holds among %d findings; the tool answered %v",
			vulnGID, reported, listed)
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
		// The count is derived from the pipeline's own report artifact, which
		// publishes exactly three CRITICAL findings, and this subtest runs
		// before the state mutations so nothing has had a chance to move it.
		// It is evidence that the report was ingested rather than evidence
		// about the refused list document: the summary is served by a separate
		// document that GitLab validates cleanly.
		requireTruef(t, out.Sast != nil, "pipeline %s security summary has no SAST section; the fixture published a SAST report", pipelineIID)
		requireTruef(t, out.Sast.VulnerabilitiesCount == fixtureSASTFindingCount,
			"SAST vulnerabilities_count = %d, want %d (the fixture report's findings)", out.Sast.VulnerabilitiesCount, fixtureSASTFindingCount)
		t.Logf("Pipeline security summary: SAST=%v DAST=%v DepScanning=%v ContainerScanning=%v",
			out.Sast != nil, out.Dast != nil, out.DependencyScanning != nil, out.ContainerScanning != nil)
	})

	t.Run("Meta/SecurityFinding/List", func(t *testing.T) {
		out, err := callToolOn[securityfindings.ListOutput](ctx, sess.meta, "gitlab_security_finding", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
				"pipeline_iid": pipelineIID,
			},
		})
		requireNoError(t, err, "security finding list for lifecycle")
		assertFixtureSASTFindings(t, out, "unfiltered")
	})

	t.Run("Meta/SecurityFinding/ListFiltered", func(t *testing.T) {
		out, err := callToolOn[securityfindings.ListOutput](ctx, sess.meta, "gitlab_security_finding", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
				"pipeline_iid": pipelineIID,
				"severity":     []string{"HIGH", "CRITICAL"},
				"report_type":  []string{"SAST"},
				"first":        10,
			},
		})
		requireNoError(t, err, "security finding list with filters")
		// Every fixture finding is a CRITICAL SAST one, so the filters are
		// asked for exactly what the pipeline holds and must not narrow it.
		assertFixtureSASTFindings(t, out, "filtered to CRITICAL/HIGH SAST")
	})
}

// vulnerabilityFixtureSource is the intentionally vulnerable file
// [TestMeta_VulnerabilityLifecycle] commits. It is never scanned, since the
// pipeline job clones nothing, but it keeps the fixture self-documenting and
// gives the report's location.file a real target in the repository.
const vulnerabilityFixtureSource = `# E2E fixture: intentionally vulnerable code referenced by the SAST report.
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

// vulnerabilityFixturePipeline is the pipeline [TestMeta_VulnerabilityLifecycle]
// commits: one job that writes a deterministic, schema-valid gl-sast-report.json
// (GitLab secure-report schema 15.x, the three CRITICAL findings named by
// [fixtureSASTIdentifiers]) and publishes it as an artifacts:reports:sast report.
//
// The fixture verifies the GitLab vulnerability reporting path and our own
// tools, not GitLab's Semgrep analyzer. Earlier revisions ran the real
// Security/SAST.gitlab-ci.yml template, whose job pulls a heavy version-pinned
// analyzer image that the ephemeral Docker runner cannot be relied on to fetch;
// when the pull failed, the job's default allow_failure left the pipeline
// successful with no security report and the lifecycle silently found nothing.
// Generating the report in-job on the pre-pulled alpine image with
// GIT_STRATEGY:none depends on nothing the runner might fail to fetch, and
// GitLab ingests it exactly as it would a real scan.
//
// printf with a single-quoted minified payload avoids heredoc and indentation
// pitfalls, and the JSON contains only double quotes so the wrapping is safe;
// when: always uploads the report even if a later step were to fail.
const vulnerabilityFixturePipeline = `stages:
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
`

// fixtureSASTFindingCount is how many findings the SAST report committed by
// [TestMeta_VulnerabilityLifecycle] publishes: SQL injection, OS command
// injection and code injection, all CRITICAL.
const fixtureSASTFindingCount = 3

// fixtureSASTIdentifiers are the CWE identifiers those three findings carry.
// They are what the fixture owns: a finding's title and name are what the
// report says they are, while the identifier is the thing a scanner and a
// consumer agree on, so an assertion written against them survives a repair of
// the fields the query selects.
var fixtureSASTIdentifiers = []string{"CWE-78", "CWE-89", "CWE-95"}

// vulnerabilityReportWait bounds how long the lifecycle waits for GitLab to
// promote the pipeline's published SAST findings into the project Vulnerability
// Report, which is asynchronous and runs behind whatever else Sidekiq is doing
// on the ephemeral instance.
const vulnerabilityReportWait = 240 * time.Second

// assertVulnerabilityState fails the test unless a vulnerability state mutation
// answered with the vulnerability it was asked about, moved to the state the
// action names. A mutation whose GraphQL document GitLab refuses answers with an
// empty payload rather than an error, so checking the returned state is what
// tells a working mutation from a silently rejected one.
func assertVulnerabilityState(t *testing.T, out vulnerabilities.MutationOutput, wantID, wantState string) {
	t.Helper()
	requireTruef(t, out.Vulnerability.ID == wantID,
		"mutation answered for vulnerability %q, want %q", out.Vulnerability.ID, wantID)
	requireTruef(t, out.Vulnerability.State == wantState,
		"vulnerability %q state = %q, want %q", wantID, out.Vulnerability.State, wantState)
}

// assertFixtureSASTFindings fails unless a security finding listing holds
// exactly the three CRITICAL findings the lifecycle's own SAST report
// publishes, identified by the CWE identifiers the report carries.
//
// It is the assertion a refused findings document fails: the handler answers a
// document GitLab rejected with an empty list and no error, which a length log
// cannot tell from a pipeline that really found nothing. The scope names which
// listing failed, since the filtered and unfiltered calls share this check.
func assertFixtureSASTFindings(t *testing.T, out securityfindings.ListOutput, scope string) {
	t.Helper()
	requireTruef(t, len(out.Findings) == fixtureSASTFindingCount,
		"%s security findings = %d, want %d (the fixture report's findings): %+v",
		scope, len(out.Findings), fixtureSASTFindingCount, out.Findings)
	identifiers := make([]string, 0, len(out.Findings))
	for _, f := range out.Findings {
		requireTruef(t, f.Severity == "CRITICAL",
			"%s security finding %q severity = %q, want CRITICAL", scope, f.UUID, f.Severity)
		for _, id := range f.Identifiers {
			identifiers = append(identifiers, id.Name)
		}
	}
	slices.Sort(identifiers)
	for _, want := range fixtureSASTIdentifiers {
		requireTruef(t, slices.Contains(identifiers, want),
			"%s security findings do not carry identifier %s; they carry %v", scope, want, identifiers)
	}
	t.Logf("%s security findings: %d carrying %v", scope, len(out.Findings), identifiers)
}

// projectVulnerability is the part of a vulnerability report entry the
// lifecycle needs from GitLab itself: which vulnerability to act on, and what
// it looked like before anything acted on it.
type projectVulnerability struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	State    string `json:"state"`
}

// waitForProjectVulnerabilities polls GitLab's project vulnerability report
// until it holds something or the budget runs out, and returns what it found.
//
// It sends a document written here rather than calling gitlab_vulnerability,
// because the lifecycle's assertions are about that tool. A handler whose
// document GitLab refuses receives no data, falls back to asking REST whether
// the project exists, and returns an empty list with no error; a fixture taken
// from it would skip the lifecycle exactly when the tool is broken. Asking
// GitLab directly makes the tool's own answer something to assert against
// rather than something to depend on.
//
// A query error is logged and retried rather than raised: the caller
// distinguishes an empty report from a broken fixture with the pipeline's own
// security summary, and reports both as a skip.
func waitForProjectVulnerabilities(ctx context.Context, t *testing.T, projectPath string, budget time.Duration) []projectVulnerability {
	t.Helper()
	const query = `query($projectPath: ID!, $first: Int!) {
  project(fullPath: $projectPath) {
    vulnerabilities(first: $first) {
      nodes {
        id
        severity
        state
      }
    }
  }
}`
	deadline := time.Now().Add(budget)
	delay := 2 * time.Second
	for {
		var resp struct {
			Data struct {
				Project *struct {
					Vulnerabilities struct {
						Nodes []projectVulnerability `json:"nodes"`
					} `json:"vulnerabilities"`
				} `json:"project"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		_, err := sess.glClient.GL().GraphQL.Do(gl.GraphQLQuery{
			Query:     query,
			Variables: map[string]any{"projectPath": projectPath, "first": 20},
		}, &resp, gl.WithContext(ctx))
		switch {
		case err != nil:
			t.Logf("fixture: vulnerability report query failed: %v", err)
		case len(resp.Errors) > 0:
			// GitLab refusing the fixture's own document would make every
			// assertion below meaningless, so it is said out loud.
			t.Logf("fixture: vulnerability report query reported GraphQL errors: %+v", resp.Errors)
		case resp.Data.Project != nil && len(resp.Data.Project.Vulnerabilities.Nodes) > 0:
			return resp.Data.Project.Vulnerabilities.Nodes
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			t.Logf("fixture: context ended while waiting for the vulnerability report: %v", ctx.Err())
			return nil
		case <-time.After(delay):
		}
		if delay < 8*time.Second {
			delay *= 2
		}
	}
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

// pipelineSASTFindingCount returns the SAST vulnerability count from a
// pipeline's security report summary, which is derived directly from the
// uploaded artifacts:reports:sast report (independent of the asynchronous
// project-level Vulnerability Report ingestion). It is used to tell whether the
// fixture's report was published and parsed (so a missing project Vulnerability
// Report is an environment limitation, not a broken fixture). Returns 0 on any
// error or when no SAST summary is present.
func pipelineSASTFindingCount(ctx context.Context, t *testing.T, projectPath, pipelineIID string) int {
	t.Helper()
	summary, err := callToolOn[vulnerabilities.PipelineSecuritySummaryOutput](ctx, sess.meta, "gitlab_vulnerability", map[string]any{
		"action": "pipeline_security_summary",
		"params": map[string]any{
			"project_path": projectPath,
			"pipeline_iid": pipelineIID,
		},
	})
	if err != nil {
		t.Logf("diagnostics: pipeline_security_summary query failed: %v", err)
		return 0
	}
	if summary.Sast == nil {
		return 0
	}
	return summary.Sast.VulnerabilitiesCount
}
