package samplingtools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/sampling"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// AnalyzePipelineFailureInput defines parameters for the analyze pipeline failure operation.
type AnalyzePipelineFailureInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	PipelineID int64                `json:"pipeline_id" jsonschema:"Pipeline ID to analyze,required"`
}

// AnalyzePipelineFailureOutput holds the LLM analysis of a failed pipeline.
type AnalyzePipelineFailureOutput struct {
	toolutil.HintableOutput
	PipelineID int64  `json:"pipeline_id"`
	Status     string `json:"status"`
	Ref        string `json:"ref"`
	Analysis   string `json:"analysis"`
	Model      string `json:"model"`
	Truncated  bool   `json:"truncated"`
}

// JobTrace pairs a job with its trace output for pipeline failure analysis.
type JobTrace struct {
	Job   jobs.Output
	Trace string
}

const analyzePipelineFailurePrompt = `Analyze this GitLab pipeline failure and provide:
1. **Root cause** — identify the most likely cause of failure from the job logs
2. **Failed jobs summary** — list each failed job with its stage, failure reason, and key error lines
3. **Fix suggestions** — actionable steps to resolve each failure
4. **Impact assessment** — what is blocked by this pipeline failure
5. **Patterns** — note any recurring failure patterns if visible

Be specific, quote error messages from logs, and prioritize actionable fixes.`

// AnalyzePipelineFailure fetches pipeline details, failed jobs and their traces,
// then delegates to the MCP sampling capability for failure root cause analysis.
func AnalyzePipelineFailure(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input AnalyzePipelineFailureInput) (AnalyzePipelineFailureOutput, error) {
	if input.ProjectID == "" {
		return AnalyzePipelineFailureOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.PipelineID <= 0 {
		return AnalyzePipelineFailureOutput{}, errors.New("pipeline_id must be a positive integer")
	}

	tracker := progress.FromRequest(req)
	tracker.Step(ctx, 1, 5, "Checking sampling capability...")

	samplingClient := sampling.FromRequest(req)
	if !samplingClient.IsSupported() {
		return AnalyzePipelineFailureOutput{}, sampling.ErrSamplingNotSupported
	}

	tracker.Step(ctx, 2, 5, "Fetching pipeline details...")

	data, status, ref, dataErr := pipelineFailureAnalysisData(ctx, tracker, client, input)
	if dataErr != nil {
		return AnalyzePipelineFailureOutput{}, dataErr
	}

	tracker.Step(ctx, 4, 5, "Requesting LLM analysis...")

	result, err := samplingClient.Analyze(
		ctx, analyzePipelineFailurePrompt, data,
		sampling.WithTemperature(0.2),
		sampling.WithModelPriorities(0.2, 0.3, 0.8),
	)
	if err != nil {
		return AnalyzePipelineFailureOutput{}, fmt.Errorf("LLM analysis: %w", err)
	}

	tracker.Step(ctx, 5, 5, "Analysis complete")

	return AnalyzePipelineFailureOutput{
		PipelineID: input.PipelineID,
		Status:     status,
		Ref:        ref,
		Analysis:   result.Content,
		Model:      result.Model,
		Truncated:  result.Truncated,
	}, nil
}

func pipelineFailureAnalysisData(ctx context.Context, tracker progress.Tracker, client *gitlabclient.Client, input AnalyzePipelineFailureInput) (content, status, ref string, err error) {
	data, status, ref, ok := pipelineFailureGraphQLData(ctx, tracker, client, input)
	if ok {
		return data, status, ref, nil
	}
	return pipelineFailureRESTData(ctx, tracker, client, input)
}

func pipelineFailureGraphQLData(ctx context.Context, tracker progress.Tracker, client *gitlabclient.Client, input AnalyzePipelineFailureInput) (content, status, ref string, ok bool) {
	gqlResult, gqlErr := BuildPipelineContext(ctx, client, string(input.ProjectID), input.PipelineID)
	if gqlErr != nil {
		return "", "", "", false
	}
	tracker.Step(ctx, 3, 5, "Fetching job traces...")
	traceSection := failedJobTraceSection(ctx, client, input.ProjectID, gqlResult.FailedJobIDs)
	return gqlResult.Content + traceSection, gqlResult.Status, gqlResult.Ref, true
}

func failedJobTraceSection(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt, jobIDs []int64) string {
	var traceSection strings.Builder
	for i, jobID := range jobIDs {
		if i >= 5 {
			break
		}
		tr, trErr := jobs.Trace(ctx, client, jobs.TraceInput{ProjectID: projectID, JobID: jobID})
		if trErr == nil && tr.Trace != "" {
			fmt.Fprintf(&traceSection, "\n### Job #%d Trace\n\n```\n%s\n```\n\n", jobID, lastTraceLines(tr.Trace, 200))
		}
	}
	return traceSection.String()
}

func pipelineFailureRESTData(ctx context.Context, tracker progress.Tracker, client *gitlabclient.Client, input AnalyzePipelineFailureInput) (content, status, ref string, err error) {
	pipeline, err := pipelines.Get(ctx, client, pipelines.GetInput{ProjectID: input.ProjectID, PipelineID: input.PipelineID})
	if err != nil {
		return "", "", "", fmt.Errorf("fetching pipeline: %w", err)
	}
	tracker.Step(ctx, 3, 5, "Fetching failed jobs and traces...")
	traces, err := failedPipelineJobTraces(ctx, client, input)
	if err != nil {
		return "", "", "", err
	}
	return FormatPipelineFailureForAnalysis(pipeline, traces), pipeline.Status, pipeline.Ref, nil
}

func failedPipelineJobTraces(ctx context.Context, client *gitlabclient.Client, input AnalyzePipelineFailureInput) ([]JobTrace, error) {
	jobList, err := jobs.List(ctx, client, jobs.ListInput{
		ProjectID:       input.ProjectID,
		PipelineID:      input.PipelineID,
		Scope:           []string{"failed"},
		PaginationInput: toolutil.PaginationInput{PerPage: 50},
	})
	if err != nil {
		return nil, fmt.Errorf("fetching jobs: %w", err)
	}
	traces := make([]JobTrace, 0, len(jobList.Jobs))
	for i, job := range jobList.Jobs {
		if i >= 5 {
			break
		}
		trace := ""
		tr, trErr := jobs.Trace(ctx, client, jobs.TraceInput{ProjectID: input.ProjectID, JobID: job.ID})
		if trErr == nil {
			trace = tr.Trace
		}
		traces = append(traces, JobTrace{Job: job, Trace: trace})
	}
	return traces, nil
}

func lastTraceLines(trace string, limit int) string {
	lines := strings.Split(trace, "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}

// FormatPipelineFailureForAnalysis builds a Markdown document from pipeline
// details and failed job traces for LLM failure analysis.
func FormatPipelineFailureForAnalysis(pipeline pipelines.DetailOutput, traces []JobTrace) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Pipeline #%d — %s\n\n", pipeline.ID, pipeline.Status)
	fmt.Fprintf(&b, "- **Ref**: %s\n", pipeline.Ref)
	fmt.Fprintf(&b, "- **SHA**: %s\n", pipeline.SHA)
	fmt.Fprintf(&b, "- **Source**: %s\n", pipeline.Source)
	fmt.Fprintf(&b, "- **Duration**: %ds\n", pipeline.Duration)
	if pipeline.YamlErrors != "" {
		fmt.Fprintf(&b, "- **YAML Errors**: %s\n", pipeline.YamlErrors)
	}

	fmt.Fprintf(&b, "\n## Failed Jobs (%d)\n\n", len(traces))
	for _, t := range traces {
		fmt.Fprintf(&b, "### %s (stage: %s)\n\n", t.Job.Name, t.Job.Stage)
		fmt.Fprintf(&b, toolutil.FmtMdStatus, t.Job.Status)
		if t.Job.FailureReason != "" {
			fmt.Fprintf(&b, "- **Failure Reason**: %s\n", t.Job.FailureReason)
		}
		fmt.Fprintf(&b, "- **Duration**: %.1fs\n", t.Job.Duration)
		if t.Trace != "" {
			fmt.Fprintf(&b, "\n```\n%s\n```\n\n", lastTraceLines(t.Trace, 200))
		}
	}
	return b.String()
}

// FormatAnalyzePipelineFailureMarkdown renders an LLM-generated pipeline failure analysis.
func FormatAnalyzePipelineFailureMarkdown(a AnalyzePipelineFailureOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Pipeline Failure Analysis: #%d (%s)\n\n", a.PipelineID, a.Ref)
	if a.Truncated {
		b.WriteString(toolutil.EmojiWarning + " *Analysis was truncated due to size limits.*\n\n")
	}
	b.WriteString(a.Analysis)
	b.WriteString("\n")
	if a.Model != "" {
		fmt.Fprintf(&b, "\n*Model: %s*\n", a.Model)
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_pipeline_retry` to re-run the failed pipeline",
		"Use `gitlab_list_pipeline_jobs` to inspect individual job logs",
	)
	return b.String()
}
