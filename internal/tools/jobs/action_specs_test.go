// action_specs_test.go contains catalog-surface regression tests for CI job actions.
package jobs

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestCatalogSurface_ConfirmDeclined verifies that a declined confirmation
// stops erase, delete_artifacts and delete_project_artifacts before GitLab is
// reached, and answers with an error result that says the user declined. The
// forbidden mock is the assertion about GitLab: on an empty mux a request that
// did get through was answered 404, which is a non-nil result as well.
func TestCatalogSurface_ConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	byTool := jobSpecsByTool(t, ActionSpecs(client))
	for _, toolName := range []string{"gitlab_job_erase", "gitlab_job_delete_artifacts", "gitlab_job_delete_project_artifacts"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test job destructive confirmation.",
			Icons:       toolutil.IconJob,
		})
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, connectErr := mcpClient.Connect(ctx, ct, nil)
	if connectErr != nil {
		t.Fatalf("client connect: %v", connectErr)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_job_erase", map[string]any{"project_id": "42", "job_id": 1}},
		{"gitlab_job_delete_artifacts", map[string]any{"project_id": "42", "job_id": 1}},
		{"gitlab_job_delete_project_artifacts", map[string]any{"project_id": "42"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool error: %v", callErr)
			}
			if result == nil || !result.IsError {
				t.Fatalf("result = %+v, want an error result for a declined confirmation", result)
			}
			var text strings.Builder
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					text.WriteString(tc.Text)
				}
			}
			if !strings.Contains(text.String(), "declined") {
				t.Errorf("result text = %q, want it to say the user declined", text.String())
			}
		})
	}
}

// TestActionSpecs_PrimaryMetadata verifies richer metadata for core job actions.
func TestActionSpecs_PrimaryMetadata(t *testing.T) {
	byTool := newJobsRouteSpecs(t)

	getSpec := byTool["gitlab_job_get"]
	if !slices.Contains(getSpec.Aliases, "get job") {
		t.Fatalf("job get Aliases = %v, want get job", getSpec.Aliases)
	}
	if guidance := getSpec.ParameterGuidance["job_id"]; guidance.SemanticRole != "job_identifier" {
		t.Fatalf("job get job_id guidance = %+v, want job_identifier", guidance)
	}

	listSpec := byTool["gitlab_job_list_project"]
	if !strings.Contains(listSpec.Usage, "List jobs in one project") {
		t.Fatalf("job list_project Usage = %q", listSpec.Usage)
	}
	if guidance := listSpec.ParameterGuidance["scope"]; guidance.SemanticRole != "job_status_filter" {
		t.Fatalf("job list_project scope guidance = %+v, want job_status_filter", guidance)
	}

	traceSpec := byTool["gitlab_job_trace"]
	if !slices.Contains(traceSpec.Aliases, "get job log") {
		t.Fatalf("job trace Aliases = %v, want get job log", traceSpec.Aliases)
	}
	if !strings.Contains(traceSpec.IndividualTool.Description, "Returns:") || !strings.Contains(traceSpec.IndividualTool.Description, "See also:") {
		t.Fatalf("job trace description = %q, want Returns/See also", traceSpec.IndividualTool.Description)
	}
}

// TestActionSpecs_Artifacts_FileTypeIsAnEnumOfGitLabsDownloadableTypes holds
// the file_type schema to the 24 values GitLab declares on the artifacts route
// (Enums::Ci::JobArtifact DOWNLOADABLE_TYPES at v19.4.0-ee), written out here
// as literals so a constant client-go renamed or dropped fails the test rather
// than quietly changing what a model is offered, and holds the override to the
// one action whose route takes the parameter.
func TestActionSpecs_Artifacts_FileTypeIsAnEnumOfGitLabsDownloadableTypes(t *testing.T) {
	want := []any{
		"archive", "accessibility", "api_fuzzing", "browser_performance", "cluster_image_scanning",
		"cobertura", "codequality", "container_scanning", "cyclonedx", "dast", "dependency_scanning",
		"dotenv", "jacoco", "junit", "license_scanning", "load_performance", "lsif", "metrics",
		"performance", "requirements", "requirements_v2", "sarif", "sast", "secret_detection",
	}
	for tool, spec := range newJobsRouteSpecs(t) {
		t.Run(tool, func(t *testing.T) {
			var enum []any
			for _, override := range spec.InputSchemaOverrides {
				if override.PropertyPath == "file_type" {
					enum, _ = override.Values["enum"].([]any)
				}
			}
			if tool != "gitlab_job_artifacts" {
				if enum != nil {
					t.Errorf("file_type enum = %v on an action whose route takes no file_type", enum)
				}
				return
			}
			if !slices.Equal(enum, want) {
				t.Errorf("file_type enum =\n got %v\nwant %v", enum, want)
			}
			if !strings.Contains(spec.Usage, "GitLab 19.4 or later") || !strings.Contains(spec.IndividualTool.Description, "GitLab 19.4 or later") {
				t.Errorf("usage %q and description %q must both state the GitLab release file_type needs", spec.Usage, spec.IndividualTool.Description)
			}
			if guidance := spec.ParameterGuidance["file_type"]; guidance.SemanticRole != "artifact_type" {
				t.Errorf("file_type guidance = %+v, want the artifact_type role", guidance)
			}
		})
	}
}

// TestActionSpecs_DownloadArtifacts_JobNamesTheActionThatTakesAJobID verifies
// that job.download_artifacts describes its job parameter as a job's name and
// points a caller holding a job_id at job.artifacts (issue 1333).
//
// A model with a job's ID in hand reached for this action, sent job_id, and
// read job as the place the ID goes. The dispatcher no longer offers job for
// job_id, so the schema every surface serves is where the caller learns which
// action takes the ID instead.
func TestActionSpecs_DownloadArtifacts_JobNamesTheActionThatTakesAJobID(t *testing.T) {
	spec := newJobsRouteSpecs(t)["gitlab_job_download_artifacts"]
	properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
	job, _ := properties["job"].(map[string]any)
	description, _ := job["description"].(string)
	for _, want := range []string{"name", "job_id", actionJobArtifacts} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(description, want) {
				t.Errorf("job description = %q, want it to mention %q", description, want)
			}
		})
	}
}

// TestActionSpecs_Artifacts_RouteSendsFileType verifies the canonical route
// decodes file_type from the caller's arguments and puts it on the request, so
// the schema a model reads and the query GitLab receives name the same value.
func TestActionSpecs_Artifacts_RouteSendsFileType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/7/artifacts" || r.URL.Query().Get("file_type") != "cobertura" {
			t.Errorf("request = %s?%s, want the artifacts route asking for cobertura", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("<coverage/>"))
	}))
	spec := jobSpecsByTool(t, ActionSpecs(client))["gitlab_job_artifacts"]
	result, err := spec.Route.Handler(t.Context(), map[string]any{"project_id": "42", "job_id": 7, "file_type": "cobertura"})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(ArtifactsOutput)
	if !ok || out.JobID != 7 || out.Size != len("<coverage/>") {
		t.Errorf("Route.Handler result = %#v, want the 11-byte report of job 7", result)
	}
}

// TestToOutput_OptionalFields verifies that ToOutput populates the optional
// pointer fields (ArtifactsExpireAt, User, Runner, ErasedAt, Commit) that are
// nil by default. The two timestamps share one value and are checked for
// presence only; their exact rendering, and which lands where, is held by
// TestJobGet_MapsEveryDocumentedField.
func TestToOutput_OptionalFields(t *testing.T) {
	now := time.Now()
	job := &gl.Job{
		ID:                1,
		Name:              "test-job",
		Stage:             "test",
		Status:            "success",
		Ref:               "main",
		Pipeline:          gl.JobPipeline{ID: 10},
		ArtifactsExpireAt: &now,
		User:              &gl.User{Username: "admin"},
		Runner:            gl.JobRunner{ID: 5},
		ErasedAt:          &now,
		Commit: &gl.Commit{
			ID: "abc123def456",
		},
	}
	out := ToOutput(job, toolutil.JobMeasures{})

	if out.ArtifactsExpireAt == "" {
		t.Error("expected ArtifactsExpireAt to be set")
	}
	if out.User == nil || out.User.Username != "admin" {
		t.Errorf("User = %+v, want username admin", out.User)
	}
	if out.Runner == nil || out.Runner.ID != 5 {
		t.Errorf("Runner = %+v, want ID 5", out.Runner)
	}
	if out.ErasedAt == "" {
		t.Error("expected ErasedAt to be set")
	}
	if out.Commit == nil || out.Commit.ID != "abc123def456" {
		t.Errorf("Commit = %+v, want ID abc123def456", out.Commit)
	}
}

// TestBridgeToOutput_OptionalFields verifies that BridgeToOutput populates the
// optional pointer fields (User, DownstreamPipeline, the four timestamps) when
// present. The timestamps share one value and are checked for presence only;
// which lands where is held by TestListBridges_MapsEveryDocumentedField.
func TestBridgeToOutput_OptionalFields(t *testing.T) {
	now := time.Now()
	bridge := &gl.Bridge{
		ID:     2,
		Name:   "trigger",
		Stage:  "deploy",
		Status: "success",
		Ref:    "main",
		User:   &gl.User{Username: "deployer"},
		DownstreamPipeline: &gl.PipelineInfo{
			ID: 99,
		},
		CreatedAt:  &now,
		StartedAt:  &now,
		FinishedAt: &now,
		ErasedAt:   &now,
	}
	out := BridgeToOutput(bridge, toolutil.BridgeExtra{
		Project: &toolutil.BridgeProjectOutput{CIJobTokenScopeEnabled: true},
	})

	if out.ErasedAt == "" {
		t.Error("expected ErasedAt to be set")
	}
	if out.Project == nil || !out.Project.CIJobTokenScopeEnabled {
		t.Error("expected the project object read beside the decode")
	}

	if out.User == nil || out.User.Username != "deployer" {
		t.Errorf("User = %+v, want username deployer", out.User)
	}
	if out.DownstreamPipeline == nil || out.DownstreamPipeline.ID != 99 {
		t.Errorf("DownstreamPipeline = %+v, want ID 99", out.DownstreamPipeline)
	}
	if out.CreatedAt == "" {
		t.Error("expected CreatedAt to be set")
	}
	if out.StartedAt == "" {
		t.Error("expected StartedAt to be set")
	}
	if out.FinishedAt == "" {
		t.Error("expected FinishedAt to be set")
	}
}

// TestTrace_Truncation covers the truncation branch in Trace when the
// trace log exceeds maxTraceBytes (100KB).
func TestTrace_Truncation(t *testing.T) {
	bigContent := strings.Repeat("A", maxTraceBytes+500)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(bigContent))
	})
	client := testutil.NewTestClient(t, mux)

	out, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Error("expected Truncated=true for oversized trace")
	}
	if len(out.Trace) != maxTraceBytes {
		t.Errorf("trace length = %d, want %d", len(out.Trace), maxTraceBytes)
	}
}

// TestReadArtifactContent_OverTheLimit_WithholdsTheContent covers the read
// of an archive larger than maxArtifactBytes: its real size, truncated set,
// and no content.
func TestReadArtifactContent_OverTheLimit_WithholdsTheContent(t *testing.T) {
	out := readArtifactContent(bytes.NewReader(bytes.Repeat([]byte("X"), maxArtifactBytes+500)), 1)
	if !out.Truncated || out.Size != maxArtifactBytes+500 || out.Content != "" {
		t.Errorf("out = truncated %v, size %d, %d characters; want true, %d, none", out.Truncated, out.Size, len(out.Content), maxArtifactBytes+500)
	}
}

// TestWithoutCutRune_DropsOnlyACharacterTheLimitCut verifies that the text
// prefix loses the bytes of a character the byte limit cut and nothing else:
// a whole character, an ASCII end and an empty prefix stay as they are, and
// at most utf8.UTFMax-1 bytes go.
func TestWithoutCutRune_DropsOnlyACharacterTheLimitCut(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"ascii end", "abc", "abc"},
		{"whole two-byte end", "abé", "abé"},
		{"half of a two-byte character", "ab\xc3", "ab"},
		{"three bytes of a four-byte character", "ab\xf0\x9f\x98", "ab"},
		{"never more than three bytes", "ab\xff\xff\xff\xff", "ab\xff"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(withoutCutRune([]byte(c.in))); got != c.want {
				t.Errorf("withoutCutRune(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestFormatOutputMarkdown_OptionalBranches covers the optional formatting
// branches in FormatOutputMarkdown (CommitSHA, Coverage, FailureReason, etc.).
func TestFormatOutputMarkdown_OptionalBranches(t *testing.T) {
	out := Output{
		ID:             1,
		Name:           "build",
		Stage:          "build",
		Status:         "failed",
		Ref:            "main",
		Commit:         &CommitObject{ID: "abc123def456789"},
		Duration:       new(45.5),
		QueuedDuration: new(2.5),
		FailureReason:  "script_failure",
		Coverage:       new(85.5),
		User:           &UserObject{Username: "admin"},
		CreatedAt:      "2024-01-01T00:00:00Z",
		WebURL:         "https://gitlab.com/job/1",
	}
	want := "## ❌ Job #1: build\n\n" +
		"- **Stage**: build\n" +
		"- **Status**: ❌ failed\n" +
		"- **Allow Failure**: ❌\n" +
		"- **Ref**: main\n" +
		"- **Tag**: ❌\n" +
		"- **Commit**: `abc123def456`\n" +
		"- **Duration**: 45.5s\n" +
		"- **Queued**: 2.5s\n" +
		"- **Failure Reason**: script_failure\n" +
		"- **Coverage**: 85.5%\n" +
		"- **User**: @admin\n" +
		"- **Created**: 1 Jan 2024 00:00 UTC\n" +
		"- **URL**: [https://gitlab.com/job/1](https://gitlab.com/job/1)\n" +
		jobCardHints
	if md := FormatOutputMarkdown(out); md != want {
		t.Errorf("FormatOutputMarkdown(optional branches)\n got %q\nwant %q", md, want)
	}
}
