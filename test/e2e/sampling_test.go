//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
)

// TestSampling exercises all 11 sampling tools via the sampling-enabled session.
// Each sampling tool invokes an LLM via mock handler; we verify non-empty results
// and that the mock model name is returned.
func TestSampling(t *testing.T) {
	ctx := context.Background()

	// Create a project with an issue, milestone, MR, and a commit for sampling tools.
	proj := createProject(ctx, t, sess.sampling)
	commitFile(ctx, t, sess.sampling, proj, "main", "sampling-init.txt", "# Sampling E2E\nproject init", "init commit")

	issue := createIssue(ctx, t, sess.sampling, proj, "Sampling test issue")

	ms, err := callToolOn[milestones.Output](ctx, sess.sampling, "gitlab_milestone_create", milestones.CreateInput{
		ProjectID:   proj.pidOf(),
		Title:       "Sampling Milestone v1",
		Description: "Milestone for sampling tests",
	})
	if err != nil {
		t.Fatalf("create milestone: %v", err)
	}

	branch := createBranch(ctx, t, sess.sampling, proj, "sampling-feature")
	commit := commitFile(ctx, t, sess.sampling, proj, branch.Name, "feature.go", "package main\nfunc main(){}", "add feature")
	mr := createMR(ctx, t, sess.sampling, proj, branch.Name, "main", "Sampling MR")

	t.Run("AnalyzeMRChanges", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeMRChangesOutput](ctx, sess.sampling, "gitlab_analyze_mr_changes", samplingtools.AnalyzeMRChangesInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		if err != nil {
			t.Fatalf("analyze MR changes: %v", err)
		}
		if out.Analysis == "" {
			t.Fatal("expected non-empty analysis")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("SummarizeIssue", func(t *testing.T) {
		out, err := callToolOn[samplingtools.SummarizeIssueOutput](ctx, sess.sampling, "gitlab_summarize_issue", samplingtools.SummarizeIssueInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
		})
		if err != nil {
			t.Fatalf("summarize issue: %v", err)
		}
		if out.Summary == "" {
			t.Fatal("expected non-empty summary")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("GenerateReleaseNotes", func(t *testing.T) {
		out, err := callToolOn[samplingtools.GenerateReleaseNotesOutput](ctx, sess.sampling, "gitlab_generate_release_notes", samplingtools.GenerateReleaseNotesInput{
			ProjectID: proj.pidOf(),
			From:      commit.SHA,
			To:        "main",
		})
		if err != nil {
			t.Fatalf("generate release notes: %v", err)
		}
		if out.ReleaseNotes == "" {
			t.Fatal("expected non-empty release notes")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("SummarizeMRReview", func(t *testing.T) {
		out, err := callToolOn[samplingtools.SummarizeMRReviewOutput](ctx, sess.sampling, "gitlab_summarize_mr_review", samplingtools.SummarizeMRReviewInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		if err != nil {
			t.Fatalf("summarize MR review: %v", err)
		}
		if out.Summary == "" {
			t.Fatal("expected non-empty summary")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("AnalyzeCIConfig", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeCIConfigOutput](ctx, sess.sampling, "gitlab_analyze_ci_configuration", samplingtools.AnalyzeCIConfigInput{
			ProjectID:  proj.pidOf(),
			ContentRef: "main",
		})
		if err != nil {
			t.Fatalf("analyze CI config: %v", err)
		}
		if out.Analysis == "" {
			t.Fatal("expected non-empty analysis")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("AnalyzeIssueScope", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeIssueScopeOutput](ctx, sess.sampling, "gitlab_analyze_issue_scope", samplingtools.AnalyzeIssueScopeInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issue.IID,
		})
		if err != nil {
			t.Fatalf("analyze issue scope: %v", err)
		}
		if out.Analysis == "" {
			t.Fatal("expected non-empty analysis")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("ReviewMRSecurity", func(t *testing.T) {
		out, err := callToolOn[samplingtools.ReviewMRSecurityOutput](ctx, sess.sampling, "gitlab_review_mr_security", samplingtools.ReviewMRSecurityInput{
			ProjectID: proj.pidOf(),
			MRIID:     mr.IID,
		})
		if err != nil {
			t.Fatalf("review MR security: %v", err)
		}
		if out.Review == "" {
			t.Fatal("expected non-empty review")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("FindTechnicalDebt", func(t *testing.T) {
		out, err := callToolOn[samplingtools.FindTechnicalDebtOutput](ctx, sess.sampling, "gitlab_find_technical_debt", samplingtools.FindTechnicalDebtInput{
			ProjectID: proj.pidOf(),
			Ref:       "main",
		})
		if err != nil {
			t.Fatalf("find technical debt: %v", err)
		}
		if out.Analysis == "" {
			t.Fatal("expected non-empty analysis")
		}
		if strings.Contains(out.Analysis, "No technical debt markers") {
			t.Logf("No technical debt found (LLM not invoked): analysis=%q", out.Analysis)
		} else if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("AnalyzeDeploymentHistory", func(t *testing.T) {
		out, err := callToolOn[samplingtools.AnalyzeDeploymentHistoryOutput](ctx, sess.sampling, "gitlab_analyze_deployment_history", samplingtools.AnalyzeDeploymentHistoryInput{
			ProjectID: proj.pidOf(),
		})
		if err != nil {
			t.Fatalf("analyze deployment history: %v", err)
		}
		if out.Analysis == "" {
			t.Fatal("expected non-empty analysis")
		}
		if strings.Contains(out.Analysis, "No deployments found") {
			t.Logf("No deployments found (LLM not invoked): analysis=%q", out.Analysis)
		} else if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	t.Run("GenerateMilestoneReport", func(t *testing.T) {
		out, err := callToolOn[samplingtools.GenerateMilestoneReportOutput](ctx, sess.sampling, "gitlab_generate_milestone_report", samplingtools.GenerateMilestoneReportInput{
			ProjectID:    proj.pidOf(),
			MilestoneIID: ms.IID,
		})
		if err != nil {
			t.Fatalf("generate milestone report: %v", err)
		}
		if out.Report == "" {
			t.Fatal("expected non-empty report")
		}
		if out.Model != "e2e-mock-model" {
			t.Fatalf("expected mock model, got %q", out.Model)
		}
	})

	// Suppress unused variable warnings.
	_ = issue
	_ = ms
	_ = commit
	_ = mr
}
