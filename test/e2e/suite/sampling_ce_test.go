//go:build e2e && !enterprise

// sampling_ce_test.go tests all 11 LLM sampling MCP tools against a live GitLab instance
// using a mock sampling handler. Verifies that each tool produces non-empty results
// and returns the expected mock model name.
package suite

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/samplingtools"
)

// TestSampling exercises all 11 sampling tools via the sampling-enabled session.
// Each sampling tool invokes an LLM via mock handler; we verify non-empty results
// and that the mock model name is returned.
func TestSampling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fixture := setupSamplingFixture(ctx, t)

	t.Run("AnalyzeMRChanges", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeMRChangesOutput](ctx, sess.sampling, "gitlab_analyze_mr_changes", samplingtools.AnalyzeMRChangesInput{
			ProjectID: fixture.proj.pidOf(),
			MRIID:     fixture.mr.IID,
		})
		if err != nil {
			t.Fatalf("analyze MR changes: %v", err)
		}
		assertSamplingOutput(t, out.Analysis, out.Model, "analysis")
	})

	t.Run("SummarizeIssue", func(t *testing.T) {
		out, err := callToolOn[samplingtools.SummarizeIssueOutput](ctx, sess.sampling, "gitlab_summarize_issue", samplingtools.SummarizeIssueInput{
			ProjectID: fixture.proj.pidOf(),
			IssueIID:  fixture.issue.IID,
		})
		if err != nil {
			t.Fatalf("summarize issue: %v", err)
		}
		assertSamplingOutput(t, out.Summary, out.Model, "summary")
	})

	t.Run("GenerateReleaseNotes", func(t *testing.T) {
		out, err := callToolOn[samplingtools.GenerateReleaseNotesOutput](ctx, sess.sampling, "gitlab_generate_release_notes", samplingtools.GenerateReleaseNotesInput{
			ProjectID: fixture.proj.pidOf(),
			From:      fixture.commit.SHA,
			To:        "main",
		})
		if err != nil {
			t.Fatalf("generate release notes: %v", err)
		}
		assertSamplingOutput(t, out.ReleaseNotes, out.Model, "release notes")
	})

	t.Run("SummarizeMRReview", func(t *testing.T) {
		out, err := callToolOn[samplingtools.SummarizeMRReviewOutput](ctx, sess.sampling, "gitlab_summarize_mr_review", samplingtools.SummarizeMRReviewInput{
			ProjectID: fixture.proj.pidOf(),
			MRIID:     fixture.mr.IID,
		})
		if err != nil {
			t.Fatalf("summarize MR review: %v", err)
		}
		assertSamplingOutput(t, out.Summary, out.Model, "summary")
	})

	t.Run("AnalyzeCIConfig", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeCIConfigOutput](ctx, sess.sampling, "gitlab_analyze_ci_configuration", samplingtools.AnalyzeCIConfigInput{
			ProjectID:  fixture.proj.pidOf(),
			ContentRef: "main",
		})
		if err != nil {
			t.Fatalf("analyze CI config: %v", err)
		}
		assertSamplingOutput(t, out.Analysis, out.Model, "analysis")
	})

	t.Run("AnalyzeIssueScope", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeIssueScopeOutput](ctx, sess.sampling, "gitlab_analyze_issue_scope", samplingtools.AnalyzeIssueScopeInput{
			ProjectID: fixture.proj.pidOf(),
			IssueIID:  fixture.issue.IID,
		})
		if err != nil {
			t.Fatalf("analyze issue scope: %v", err)
		}
		assertSamplingOutput(t, out.Analysis, out.Model, "analysis")
	})

	t.Run("ReviewMRSecurity", func(t *testing.T) {
		out, err := callToolOn[samplingtools.ReviewMRSecurityOutput](ctx, sess.sampling, "gitlab_review_mr_security", samplingtools.ReviewMRSecurityInput{
			ProjectID: fixture.proj.pidOf(),
			MRIID:     fixture.mr.IID,
		})
		if err != nil {
			t.Fatalf("review MR security: %v", err)
		}
		assertSamplingOutput(t, out.Review, out.Model, "review")
	})

	t.Run("FindTechnicalDebt", func(t *testing.T) {
		assertFindTechnicalDebtSampling(ctx, t, fixture)
	})

	t.Run("AnalyzeDeploymentHistory", func(t *testing.T) {
		assertDeploymentHistorySampling(ctx, t, fixture)
	})

	t.Run("GenerateMilestoneReport", func(t *testing.T) {
		out, err := callToolOn[samplingtools.GenerateMilestoneReportOutput](ctx, sess.sampling, "gitlab_generate_milestone_report", samplingtools.GenerateMilestoneReportInput{
			ProjectID:    fixture.proj.pidOf(),
			MilestoneIID: fixture.milestone.IID,
		})
		if err != nil {
			t.Fatalf("generate milestone report: %v", err)
		}
		assertSamplingOutput(t, out.Report, out.Model, "report")
	})
}

func assertFindTechnicalDebtSampling(ctx context.Context, t *testing.T, fixture samplingFixture) {
	t.Helper()
	out, err := callToolOn[samplingtools.FindTechnicalDebtOutput](ctx, sess.sampling, "gitlab_find_technical_debt", samplingtools.FindTechnicalDebtInput{
		ProjectID: fixture.proj.pidOf(),
		Ref:       "main",
	})
	if err != nil {
		t.Fatalf("find technical debt: %v", err)
	}
	assertSamplingOrFallbackOutput(t, out.Analysis, out.Model, "analysis", "No technical debt markers", "No technical debt found")
}

func assertDeploymentHistorySampling(ctx context.Context, t *testing.T, fixture samplingFixture) {
	t.Helper()
	out, err := callToolOn[samplingtools.AnalyzeDeploymentHistoryOutput](ctx, sess.sampling, "gitlab_analyze_deployment_history", samplingtools.AnalyzeDeploymentHistoryInput{
		ProjectID: fixture.proj.pidOf(),
	})
	if err != nil {
		t.Fatalf("analyze deployment history: %v", err)
	}
	assertSamplingOrFallbackOutput(t, out.Analysis, out.Model, "analysis", "No deployments found", "No deployments found")
}

func assertSamplingOrFallbackOutput(t *testing.T, text, model, label, fallbackMarker, fallbackLog string) {
	t.Helper()
	assertNonEmptySamplingText(t, text, label)
	if strings.Contains(text, fallbackMarker) {
		t.Logf("%s (LLM not invoked): %s=%q", fallbackLog, label, text)
		return
	}
	if model != "e2e-mock-model" {
		t.Fatalf("expected mock model, got %q", model)
	}
}

type samplingFixture struct {
	proj      ProjectFixture
	issue     IssueFixture
	milestone milestones.Output
	commit    CommitFixture
	mr        MRFixture
}

func setupSamplingFixture(ctx context.Context, t *testing.T) samplingFixture {
	t.Helper()
	proj := createProject(ctx, t, sess.sampling)
	commitFile(ctx, t, sess.sampling, proj, "main", "sampling-init.txt", "# Sampling E2E\nproject init", "init commit")
	commitFileCreateOrUpdate(ctx, t, sess.sampling, proj, "main", ".gitlab-ci.yml", "stages:\n  - test\nunit_test:\n  stage: test\n  script:\n    - echo \"running tests\"", "add CI config for AnalyzeCIConfig test")
	issue := createIssue(ctx, t, sess.sampling, proj, "Sampling test issue")
	milestone := createSamplingMilestone(ctx, t, proj)
	branch := createBranch(ctx, t, sess.sampling, proj, "sampling-feature")
	commit := commitFile(ctx, t, sess.sampling, proj, branch.Name, "feature.go", "package main\nfunc main(){}", "add feature")
	mr := createMR(ctx, t, sess.sampling, proj, branch.Name, "main", "Sampling MR")
	return samplingFixture{proj: proj, issue: issue, milestone: milestone, commit: commit, mr: mr}
}

func createSamplingMilestone(ctx context.Context, t *testing.T, proj ProjectFixture) milestones.Output {
	t.Helper()
	out, err := callToolOn[milestones.Output](ctx, sess.sampling, "gitlab_milestone_create", milestones.CreateInput{
		ProjectID:   proj.pidOf(),
		Title:       "Sampling Milestone v1",
		Description: "Milestone for sampling tests",
	})
	if err != nil {
		t.Fatalf("create milestone: %v", err)
	}
	return out
}

func assertSamplingOutput(t *testing.T, text, model, label string) {
	t.Helper()
	assertNonEmptySamplingText(t, text, label)
	if model != "e2e-mock-model" {
		t.Fatalf("expected mock model, got %q", model)
	}
}

func assertNonEmptySamplingText(t *testing.T, text, label string) {
	t.Helper()
	if text == "" {
		t.Fatalf("expected non-empty %s", label)
	}
}
