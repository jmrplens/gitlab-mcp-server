//go:build e2e

// vulnerabilities_test.go covers the vulnerability report of a project: the
// counts and the listing of a project that has none, and the whole
// lifecycle on one that has three.
//
// The three come from a pipeline the fixture commits, whose one job writes
// a schema-valid SAST report naming three CRITICAL findings and publishes it
// as artifacts:reports:sast. GitLab ingests it into the project's
// Vulnerability Report exactly as it would a real scan, so what is tested is
// this server's tools and GitLab's reporting path, and not an analyzer image
// the ephemeral runner may fail to pull. The fixture is confirmed by asking
// GitLab itself, over GraphQL, before any tool is asked: a tool whose
// document GitLab refuses answers an empty list and no error, and a fixture
// derived from that tool would skip the assertions in exactly the state they
// were written to catch.

package ee

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/securityfindings"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// vulnerableSource is the file the SAST report points at. It is never
// scanned, since the job clones nothing; it gives the report's locations a
// real target and keeps the fixture self-describing.
const vulnerableSource = `# E2E fixture: intentionally vulnerable code referenced by the SAST report.
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

// sastReportPipeline is the pipeline that publishes the report: one job on
// the alpine image the runner already holds, cloning nothing, writing the
// minified report with printf and publishing it whatever happens next. The
// report follows the secure-report schema 15.x and carries the findings
// [sastIdentifiers] names.
const sastReportPipeline = `stages:
  - test

sast:
  stage: test
  image: alpine:latest
  variables:
    GIT_STRATEGY: none
  script:
    - |
      printf '%s' '{"version":"15.0.6","scan":{"analyzer":{"id":"e2e-fixture","name":"E2E Fixture","version":"1.0.0","vendor":{"name":"gitlab-mcp-server"}},"scanner":{"id":"e2e-fixture","name":"E2E Fixture","version":"1.0.0","vendor":{"name":"gitlab-mcp-server"}},"type":"sast","start_time":"2026-01-01T00:00:00","end_time":"2026-01-01T00:00:01","status":"success"},"vulnerabilities":[{"id":"e2e-sast-sqli-0001","category":"sast","name":"SQL Injection","message":"SQL Injection","description":"User input concatenated into a SQL query (CWE-89).","severity":"Critical","scanner":{"id":"e2e-fixture","name":"E2E Fixture"},"location":{"file":"app.py","start_line":10,"end_line":10,"class":"UserRepository","method":"find_by_name"},"identifiers":[{"type":"cwe","name":"CWE-89","value":"89","url":"https://cwe.mitre.org/data/definitions/89.html"}],"links":[{"name":"E2E SQL injection write-up","url":"https://owasp.org/www-community/attacks/SQL_Injection"}]},{"id":"e2e-sast-cmdi-0002","category":"sast","name":"OS Command Injection","message":"OS Command Injection","description":"User input flows into os.system (CWE-78).","severity":"Critical","scanner":{"id":"e2e-fixture","name":"E2E Fixture"},"location":{"file":"app.py","start_line":14,"end_line":14,"class":"Shell","method":"list_directory"},"identifiers":[{"type":"cwe","name":"CWE-78","value":"78","url":"https://cwe.mitre.org/data/definitions/78.html"}],"links":[{"name":"E2E command injection write-up","url":"https://owasp.org/www-community/attacks/Command_Injection"}]},{"id":"e2e-sast-eval-0003","category":"sast","name":"Code Injection","message":"Code Injection","description":"User input passed to eval (CWE-95).","severity":"Critical","scanner":{"id":"e2e-fixture","name":"E2E Fixture"},"location":{"file":"app.py","start_line":18,"end_line":18,"class":"Calculator","method":"evaluate"},"identifiers":[{"type":"cwe","name":"CWE-95","value":"95","url":"https://cwe.mitre.org/data/definitions/95.html"}],"links":[{"name":"E2E code injection write-up","url":"https://owasp.org/www-community/attacks/Code_Injection"}]}]}' > gl-sast-report.json
      echo "wrote gl-sast-report.json ($(wc -c < gl-sast-report.json) bytes)"
  artifacts:
    when: always
    reports:
      sast: gl-sast-report.json
`

// sastFindingCount is how many findings the report publishes, and
// sastIdentifiers the CWE identifiers they carry, which are what the
// fixture owns: a finding's title is whatever the report says, while the
// identifier is what a scanner and a consumer agree on.
const sastFindingCount = 3

var sastIdentifiers = []string{"CWE-78", "CWE-89", "CWE-95"}

// reportedFinding is what the report says about the finding carrying one CWE
// beyond its identifier: the class and method its location names and the one
// link it attaches, which is how the location's SAST members and the links
// are asserted against a value the fixture owns.
type reportedFinding struct {
	class, method, linkURL string
}

// sastReported is [reportedFinding] for each identifier in [sastIdentifiers].
var sastReported = map[string]reportedFinding{
	"CWE-89": {class: "UserRepository", method: "find_by_name", linkURL: "https://owasp.org/www-community/attacks/SQL_Injection"},
	"CWE-78": {class: "Shell", method: "list_directory", linkURL: "https://owasp.org/www-community/attacks/Command_Injection"},
	"CWE-95": {class: "Calculator", method: "evaluate", linkURL: "https://owasp.org/www-community/attacks/Code_Injection"},
}

// e2eDismissalComment is the comment the lifecycle dismisses with, which
// GitLab records on the state transition and answers as the state comment.
const e2eDismissalComment = "e2e dismissal"

// The wait for GitLab to promote the published findings into the project's
// report, which runs on Sidekiq behind whatever else the instance is doing.
const (
	vulnerabilityReportWait     = 4 * time.Minute
	vulnerabilityReportInterval = 5 * time.Second
)

// The state names GitLab gives a vulnerability along the lifecycle.
const (
	stateConfirmed = "CONFIRMED"
	stateResolved  = "RESOLVED"
	stateDetected  = "DETECTED"
	stateDismissed = "DISMISSED"
)

// TestVulnerabilities_FreshProject_ReportsNothing reads the severity counts
// and the listing of a project nothing scanned, on every surface.
//
// Replaces: TestIndividual_Vulnerabilities, TestMeta_Vulnerabilities
func TestVulnerabilities_FreshProject_ReportsNothing(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("vuln"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)

		counts := harness.Do[vulnerabilities.SeverityCountOutput](s, actionVulnerabilitySeverityCount, map[string]any{"project_path": project.Path})
		if counts.Total != 0 {
			e.T.Errorf("a project nothing scanned counts %d vulnerabilities: %+v", counts.Total, counts)
		}
		listed := harness.Do[vulnerabilities.ListOutput](s, actionVulnerabilityList, map[string]any{"project_path": project.Path})
		if len(listed.Vulnerabilities) != 0 {
			e.T.Errorf("a project nothing scanned lists %d vulnerabilities: %+v", len(listed.Vulnerabilities), listed.Vulnerabilities)
		}
	})
}

// TestSecurityFindings_MissingPipeline_IsRefusedAsNotFound asks a fresh
// project for the findings of a pipeline it never ran, plain and filtered,
// on every surface: the action refuses a pipeline it cannot find rather
// than answering an empty page a caller would read as no findings.
//
// Replaces: TestMeta_SecurityFindings
func TestSecurityFindings_MissingPipeline_IsRefusedAsNotFound(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("findings"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		params := map[string]any{"project_path": project.Path, "pipeline_iid": "1"}

		refused := harness.Refused(s, actionSecurityFindingList, params, harness.FailureNotFound)
		assertMentions(e, "the findings of a missing pipeline", refused, "pipeline")
		refused = harness.Refused(s, actionSecurityFindingList, withParams(params, map[string]any{
			"severity": []string{"HIGH", "CRITICAL"}, "report_type": []string{"SAST"}, "first": 10,
		}), harness.FailureNotFound)
		assertMentions(e, "the filtered findings of a missing pipeline", refused, "pipeline")
	})
}

// reportedVulnerability is what the lifecycle needs from GitLab's own
// report: which vulnerability to act on and what it looked like first.
type reportedVulnerability struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	State    string `json:"state"`
}

// vulnerabilityFixture is a project whose default-branch pipeline published
// the SAST report, with the vulnerabilities GitLab ingested from it.
type vulnerabilityFixture struct {
	project     fixture.Project
	pipelineIID string
	reported    []reportedVulnerability
}

// buildVulnerabilityFixture commits the source and the pipeline, runs the
// pipeline to success, and waits for GitLab's own report to hold the three
// findings, failing with the pipeline's job diagnostics when it does not.
func buildVulnerabilityFixture(e *harness.Env) vulnerabilityFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("vulnlife"))
	fixture.CommitFile(e, project, project.DefaultBranch, "app.py", vulnerableSource, "add the intentionally vulnerable fixture source")
	fixture.CommitFile(e, project, project.DefaultBranch, fixture.CIFilePath, sastReportPipeline, "add the SAST report publishing pipeline")

	pipeline := fixture.NewPipeline(e, project, project.DefaultBranch)
	if pipeline.Status != "success" {
		logPipelineJobs(e, project, pipeline.ID)
		e.T.Fatalf("the SAST report pipeline %d ended %s, want success", pipeline.ID, pipeline.Status)
	}
	detail, _, err := e.Client().GL().Pipelines.GetPipeline(project.ID, pipeline.ID, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading pipeline %d for its iid: %v", pipeline.ID, err)
	}

	reported, err := waitForProjectVulnerabilities(e, project.Path)
	if err != nil {
		logPipelineJobs(e, project, pipeline.ID)
		e.T.Fatalf("pipeline %d (%s) published a SAST report and GitLab's own vulnerability report of %s holds nothing after %s: %v",
			pipeline.ID, pipeline.Status, project.Path, vulnerabilityReportWait, err)
	}
	if len(reported) < sastFindingCount {
		e.T.Fatalf("GitLab's own report of %s holds %d vulnerabilities, want the %d the fixture published: %+v", project.Path, len(reported), sastFindingCount, reported)
	}
	return vulnerabilityFixture{project: project, pipelineIID: strconv.FormatInt(detail.IID, 10), reported: reported}
}

// projectVulnerabilitiesQuery is the document the fixture asks GitLab with,
// deliberately written here rather than borrowed from the tool under test.
const projectVulnerabilitiesQuery = `query($projectPath: ID!, $first: Int!) {
  project(fullPath: $projectPath) {
    vulnerabilities(first: $first) {
      nodes { id severity state }
    }
  }
}`

// waitForProjectVulnerabilities polls GitLab's project vulnerability report
// until it holds every published finding or the budget runs out. A query
// GitLab refuses is reported as the reason rather than retried into a
// silence.
func waitForProjectVulnerabilities(e *harness.Env, projectPath string) ([]reportedVulnerability, error) {
	e.T.Helper()

	var found []reportedVulnerability
	err := harness.Poll(e.Ctx, vulnerabilityReportInterval, vulnerabilityReportWait, func() (bool, string, error) {
		var answer struct {
			Data struct {
				Project *struct {
					Vulnerabilities struct {
						Nodes []reportedVulnerability `json:"nodes"`
					} `json:"vulnerabilities"`
				} `json:"project"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		_, queryErr := e.Client().GL().GraphQL.Do(gl.GraphQLQuery{
			Query:     projectVulnerabilitiesQuery,
			Variables: map[string]any{"projectPath": projectPath, "first": 20},
		}, &answer, gl.WithContext(e.Ctx))
		switch {
		case queryErr != nil:
			// A transport failure is the instance under load, which the next
			// poll asks again; only GitLab's own refusal of the document ends
			// the wait, since no later poll can change that answer.
			return harness.WaitThrough("the report query failed", queryErr)
		case len(answer.Errors) > 0:
			refusals, _ := json.Marshal(answer.Errors)
			return false, "", fmt.Errorf("GitLab refused the fixture's own report query: %s", refusals)
		case answer.Data.Project == nil:
			return false, "the project is not visible to the report query yet", nil
		}
		found = answer.Data.Project.Vulnerabilities.Nodes
		if len(found) < sastFindingCount {
			return false, fmt.Sprintf("the report holds %d of %d findings", len(found), sastFindingCount), nil
		}
		return true, "", nil
	})
	return found, err
}

// logPipelineJobs logs every job of a pipeline with its status, so a runner
// or fixture regression is diagnosable from the test log alone.
func logPipelineJobs(e *harness.Env, project fixture.Project, pipelineID int64) {
	e.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	jobs, _, err := e.Client().GL().Jobs.ListPipelineJobs(project.ID, pipelineID, nil, gl.WithContext(ctx))
	if err != nil {
		e.T.Logf("diagnostics: listing the jobs of pipeline %d: %v", pipelineID, err)
		return
	}
	for _, job := range jobs {
		e.T.Logf("diagnostics: job %d %q stage=%q status=%q allow_failure=%t", job.ID, job.Name, job.Stage, job.Status, job.AllowFailure)
	}
}

// assertSASTFindings fails unless a findings listing holds exactly want of
// the CRITICAL findings the report publishes, each carrying one of the
// identifiers the report gives them and no two the same. A handler whose
// document GitLab refused answers an empty list and no error, which a
// length log cannot tell from a pipeline that found nothing, and this is
// what tells them apart.
func assertSASTFindings(e *harness.Env, scope string, out securityfindings.ListOutput, want int) {
	e.T.Helper()
	if len(out.Findings) != want {
		e.T.Errorf("%s: %d findings, want %d: %+v", scope, len(out.Findings), want, out.Findings)
		return
	}
	seen := make([]string, 0, want)
	for _, finding := range out.Findings {
		if finding.Severity != "CRITICAL" {
			e.T.Errorf("%s: finding %s is %s, want CRITICAL", scope, finding.UUID, finding.Severity)
		}
		// Exactly one of the report's identifiers per finding: a global
		// count alone would accept a finding carrying none beside one
		// carrying two, which is the shape a malformed decode takes.
		var expected []string
		for _, identifier := range finding.Identifiers {
			if !slices.Contains(sastIdentifiers, identifier.Name) {
				e.T.Errorf("%s: finding %s carries %s, which the report never published", scope, finding.UUID, identifier.Name)
				continue
			}
			expected = append(expected, identifier.Name)
		}
		if len(expected) != 1 {
			e.T.Errorf("%s: finding %s carries %d of the report's identifiers %v, want exactly one", scope, finding.UUID, len(expected), expected)
			continue
		}
		if slices.Contains(seen, expected[0]) {
			e.T.Errorf("%s: two findings carry %s", scope, expected[0])
		}
		seen = append(seen, expected[0])
		assertReported(e, scope+": finding "+finding.UUID, expected[0], finding.Location, finding.Links)
	}
	if len(seen) != want {
		e.T.Errorf("%s: the findings carry %v, want %d distinct identifiers", scope, seen, want)
	}
}

// assertReported fails unless a vulnerability or a finding carries the class
// and method the report gave the location of the finding with that CWE, and
// the link the report attached to it. Both reach the output only through the
// SAST member of the location union and the links selection, which is what
// this holds against a live GitLab.
func assertReported(e *harness.Env, scope, cwe string, location *toolutil.VulnerabilityLocationOutput, links []toolutil.VulnerabilityLinkOutput) {
	e.T.Helper()
	want, ok := sastReported[cwe]
	if !ok {
		e.T.Errorf("%s: identifier %q is none the report published", scope, cwe)
		return
	}
	if location == nil || location.VulnerableClass != want.class || location.VulnerableMethod != want.method {
		e.T.Errorf("%s: location = %+v, want the report's class %q and method %q", scope, location, want.class, want.method)
	}
	if !slices.ContainsFunc(links, func(link toolutil.VulnerabilityLinkOutput) bool { return link.URL == want.linkURL }) {
		e.T.Errorf("%s: links = %+v, want the report's %s", scope, links, want.linkURL)
	}
}

// assertFindingsAreTheVulnerabilities fails unless every finding of the
// pipeline names, by its uuid, one of the vulnerabilities the project listing
// answered, and names the same vulnerability by id. GitLab documents a
// vulnerability's uuid as the key to its security report finding, so the two
// listings agreeing is what shows the uuid is the one GitLab means.
func assertFindingsAreTheVulnerabilities(e *harness.Env, findings securityfindings.ListOutput, listed vulnerabilities.ListOutput) {
	e.T.Helper()
	byUUID := make(map[string]string, len(listed.Vulnerabilities))
	for _, vulnerability := range listed.Vulnerabilities {
		if vulnerability.UUID == "" {
			e.T.Errorf("the listing answers vulnerability %q with no uuid", vulnerability.ID)
			continue
		}
		byUUID[vulnerability.UUID] = vulnerability.ID
	}
	for _, finding := range findings.Findings {
		id, ok := byUUID[finding.UUID]
		switch {
		case !ok:
			e.T.Errorf("finding %q names no vulnerability of the listing by uuid; the listing holds %v", finding.UUID, byUUID)
		case finding.VulnID != id:
			e.T.Errorf("finding %q names vulnerability %q, and the vulnerability with its uuid is %q", finding.UUID, finding.VulnID, id)
		}
	}
}

// everyFindingState names every state a finding can be in, for a listing
// that must hold the findings earlier surfaces dismissed: GitLab leaves a
// dismissed finding out unless the filter asks for it.
var everyFindingState = []string{stateDetected, stateConfirmed, stateResolved, stateDismissed}

// assertVulnerabilityState fails unless a state mutation answered for the
// vulnerability it was asked about, moved to the state the action names. A
// mutation whose document GitLab refused answers an empty payload rather
// than an error, so the returned state is what tells a working mutation
// from a silently rejected one.
func assertVulnerabilityState(e *harness.Env, action harness.ActionID, out vulnerabilities.MutationOutput, wantID, wantState string) {
	e.T.Helper()
	if out.Vulnerability.ID != wantID {
		e.T.Errorf("%s answered for vulnerability %q, want %q", action, out.Vulnerability.ID, wantID)
	}
	if out.Vulnerability.State != wantState {
		e.T.Errorf("%s left vulnerability %q in state %q, want %s", action, wantID, out.Vulnerability.State, wantState)
	}
}

// TestVulnerabilityLifecycle_SASTReport_ReadsAndMovesEveryState builds the
// report once and, on every surface, reads it back through the listing, the
// pipeline's security summary and the findings listing, then walks one of
// the three vulnerabilities, the surface's own, through get, confirm,
// resolve, revert and dismiss. Each surface takes a different vulnerability
// so that none finds the state another left.
//
// Replaces: TestMeta_VulnerabilityLifecycle
func TestVulnerabilityLifecycle_SASTReport_ReadsAndMovesEveryState(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedRunner, harness.Tier(edition.Ultimate)), harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, buildVulnerabilityFixture, func(e *harness.Env, surface harness.Surface, f vulnerabilityFixture) {
		s := e.On(surface)
		// The surfaces run in order and each ends by dismissing its own
		// vulnerability, so this one finds the earlier ones' dismissed.
		earlier := slices.Index(harness.AllSurfaces(), surface)
		own := f.reported[earlier%len(f.reported)]
		reportedIDs := make([]string, 0, len(f.reported))
		for _, vulnerability := range f.reported {
			reportedIDs = append(reportedIDs, vulnerability.ID)
		}

		listed := harness.Do[vulnerabilities.ListOutput](s, actionVulnerabilityList, map[string]any{"project_path": f.project.Path})
		listedIDs := make([]string, 0, len(listed.Vulnerabilities))
		for _, vulnerability := range listed.Vulnerabilities {
			listedIDs = append(listedIDs, vulnerability.ID)
		}
		for _, id := range reportedIDs {
			if !slices.Contains(listedIDs, id) {
				e.T.Errorf("the listing does not report %q, which GitLab's own report holds among %d; it answered %v", id, len(reportedIDs), listedIDs)
			}
		}

		summary := harness.Do[vulnerabilities.PipelineSecuritySummaryOutput](s, actionVulnerabilityPipelineSecuritySummary,
			map[string]any{"project_path": f.project.Path, "pipeline_iid": f.pipelineIID})
		if summary.Sast == nil {
			e.T.Errorf("pipeline %s reports no SAST section, and the fixture published a SAST report", f.pipelineIID)
		} else {
			if summary.Sast.VulnerabilitiesCount != sastFindingCount {
				e.T.Errorf("pipeline %s counts %d SAST vulnerabilities, want %d", f.pipelineIID, summary.Sast.VulnerabilitiesCount, sastFindingCount)
			}
			// The one job that published the report ran one SAST scan, and the
			// scan that produced the three counted findings is one GitLab
			// recorded with a name and an outcome.
			if len(summary.Sast.Scans) == 0 {
				e.T.Errorf("pipeline %s lists no SAST scan, and the report its job published was counted", f.pipelineIID)
			}
			for _, scan := range summary.Sast.Scans {
				if scan.Name == "" || scan.Status == "" {
					e.T.Errorf("pipeline %s lists a SAST scan with no name or status: %+v", f.pipelineIID, scan)
				}
			}
		}

		findings := map[string]any{"project_path": f.project.Path, "pipeline_iid": f.pipelineIID}
		// GitLab leaves a dismissed finding out of a listing that names no
		// state, so the plain listing shrinks by one per surface that ran.
		assertSASTFindings(e, "the plain findings listing", harness.Do[securityfindings.ListOutput](s, actionSecurityFindingList, findings), sastFindingCount-earlier)
		// Every published finding is CRITICAL SAST, so the severity and
		// report filters ask for exactly what the pipeline holds and must
		// narrow nothing, and naming every state brings the dismissed back.
		everyState := harness.Do[securityfindings.ListOutput](s, actionSecurityFindingList,
			withParams(findings, map[string]any{"severity": []string{"HIGH", "CRITICAL"}, "report_type": []string{"SAST"}, "state": everyFindingState, "first": 10}))
		assertSASTFindings(e, "the findings filtered to CRITICAL and HIGH SAST in every state", everyState, sastFindingCount)
		assertFindingsAreTheVulnerabilities(e, everyState, listed)

		got := harness.Do[vulnerabilities.GetOutput](s, actionVulnerabilityGet, map[string]any{"id": own.ID})
		assertReadVulnerability(e, got.Vulnerability, own.ID)

		confirmed := harness.Do[vulnerabilities.MutationOutput](s, actionVulnerabilityConfirm, map[string]any{"id": own.ID})
		assertVulnerabilityState(e, actionVulnerabilityConfirm, confirmed, own.ID, stateConfirmed)
		assertRecordedBy(e, actionVulnerabilityConfirm, confirmed.Vulnerability.ConfirmedBy)
		resolved := harness.Do[vulnerabilities.MutationOutput](s, actionVulnerabilityResolve, map[string]any{"id": own.ID})
		assertVulnerabilityState(e, actionVulnerabilityResolve, resolved, own.ID, stateResolved)
		assertRecordedBy(e, actionVulnerabilityResolve, resolved.Vulnerability.ResolvedBy)
		assertVulnerabilityState(e, actionVulnerabilityRevert, harness.Do[vulnerabilities.MutationOutput](s, actionVulnerabilityRevert, map[string]any{"id": own.ID}), own.ID, stateDetected)
		dismissed := harness.Do[vulnerabilities.MutationOutput](s, actionVulnerabilityDismiss, map[string]any{
			"id": own.ID, "comment": e2eDismissalComment, "dismissal_reason": "USED_IN_TESTS",
		})
		assertVulnerabilityState(e, actionVulnerabilityDismiss, dismissed, own.ID, stateDismissed)
		assertRecordedBy(e, actionVulnerabilityDismiss, dismissed.Vulnerability.DismissedBy)
		// GitLab answers the state comment from the vulnerability's latest state
		// transition, which the dismissal just wrote with this comment.
		if dismissed.Vulnerability.StateComment != e2eDismissalComment || dismissed.Vulnerability.DismissalReason != "USED_IN_TESTS" {
			e.T.Errorf("the dismissal answered state comment %q and reason %q, want %q and USED_IN_TESTS",
				dismissed.Vulnerability.StateComment, dismissed.Vulnerability.DismissalReason, e2eDismissalComment)
		}
	})
}

// assertReadVulnerability fails unless a vulnerability read back is the one
// asked for and carries what the fixture guarantees: a uuid, presence on the
// default branch the report's pipeline ran on, and the class, method and link
// the report gave the finding with its primary identifier.
func assertReadVulnerability(e *harness.Env, got vulnerabilities.Item, wantID string) {
	e.T.Helper()
	if got.ID != wantID {
		e.T.Errorf("get answered %q, want %q", got.ID, wantID)
	}
	if got.UUID == "" {
		e.T.Errorf("vulnerability %q carries no uuid, which GitLab sends for every vulnerability", wantID)
	}
	if !got.PresentOnDefaultBranch {
		e.T.Errorf("vulnerability %q is not present on the default branch, and the report was published by a default-branch pipeline", wantID)
	}
	if got.PrimaryID == nil {
		e.T.Errorf("vulnerability %q carries no primary identifier", wantID)
		return
	}
	assertReported(e, "get "+wantID, got.PrimaryID.Name, got.Location, got.Links)
}

// assertRecordedBy fails unless a state change answered with the person GitLab
// recorded making it, which is the token's own user.
func assertRecordedBy(e *harness.Env, action harness.ActionID, user *toolutil.UserCoreRefOutput) {
	e.T.Helper()
	if user == nil || user.Username == "" {
		e.T.Errorf("%s answered with nobody recorded against the change: %+v", action, user)
	}
}
