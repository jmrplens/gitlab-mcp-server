// jobs_test.go contains unit tests for GitLab CI/CD job operations
// (list, get, trace, cancel, retry). Tests use httptest to mock the
// GitLab Jobs API and verify both success and error paths.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	// pathPipelineJobs identifies the path pipeline jobs constant used by this package.
	pathPipelineJobs = "/api/v4/projects/42/pipelines/10/jobs"
	// pathJobGet identifies the path job get constant used by this package.
	pathJobGet = "/api/v4/projects/42/jobs/100"
	// pathJobTrace identifies the path job trace constant used by this package.
	pathJobTrace = "/api/v4/projects/42/jobs/100/trace"
	// pathJobCancel identifies the path job cancel constant used by this package.
	pathJobCancel = "/api/v4/projects/42/jobs/100/cancel"
	// pathJobRetry identifies the path job retry constant used by this package.
	pathJobRetry = "/api/v4/projects/42/jobs/100/retry"

	// testHeaderContentType identifies the test header content type constant used by this package.
	testHeaderContentType = "Content-Type"
	// testReportContent identifies the test report content constant used by this package.
	testReportContent = "test report content"
	// testReportFileName identifies the test report file name constant used by this package.
	testReportFileName = "report.txt"
	// testRefArtifactContent identifies the test ref artifact content constant used by this package.
	testRefArtifactContent = "ref artifact content"
	// fmtIDWant100 identifies the fmt ID want 100 constant used by this package.
	fmtIDWant100 = "ID = %d, want 100"
)

// jobJSON identifies the job JSON constant used by this package.
const jobJSON = `{
	"id":100,
	"name":"build",
	"stage":"build",
	"status":"success",
	"ref":"main",
	"tag":false,
	"allow_failure":false,
	"duration":45.5,
	"queued_duration":2.1,
	"web_url":"https://gitlab.example.com/-/jobs/100",
	"pipeline":{"id":10},
	"created_at":"2026-03-01T10:00:00Z",
	"started_at":"2026-03-01T10:00:05Z",
	"finished_at":"2026-03-01T10:00:50Z",
	"user":{"username":"testuser"},
	"runner":{"id":1}
}`

// TestJobList_Success verifies JobList when success.
func TestJobList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		PipelineID: 10,
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	if out.Jobs[0].Name != "build" {
		t.Errorf("Jobs[0].Name = %q, want %q", out.Jobs[0].Name, "build")
	}
	if out.Jobs[0].Status != "success" {
		t.Errorf("Jobs[0].Status = %q, want %q", out.Jobs[0].Status, "success")
	}
	if out.Jobs[0].PipelineID != 10 {
		t.Errorf("Jobs[0].PipelineID = %d, want 10", out.Jobs[0].PipelineID)
	}
	if out.Jobs[0].UserUsername != "testuser" {
		t.Errorf("Jobs[0].UserUsername = %q, want %q", out.Jobs[0].UserUsername, "testuser")
	}
	if out.Jobs[0].RunnerID != 1 {
		t.Errorf("Jobs[0].RunnerID = %d, want 1", out.Jobs[0].RunnerID)
	}
}

// TestJobList_WithScope verifies JobList when with scope.
func TestJobList_WithScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			q := r.URL.Query()
			scopes := q["scope[]"]
			if len(scopes) != 1 || scopes[0] != "failed" {
				t.Errorf("expected scope[]=failed, got %v", scopes)
			}
			testutil.RespondJSON(w, http.StatusOK, "[]")
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:  "42",
		PipelineID: 10,
		Scope:      []string{"failed"},
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Jobs) != 0 {
		t.Errorf("len(Jobs) = %d, want 0", len(out.Jobs))
	}
}

// TestJobList_EmptyProjectID verifies JobList when empty project ID.
func TestJobList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	_, err := List(context.Background(), client, ListInput{PipelineID: 10})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestJobGet_Success verifies JobGet when success.
func TestJobGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobGet {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Name != "build" {
		t.Errorf("out.Name = %q, want %q", out.Name, "build")
	}
	if out.Duration != 45.5 {
		t.Errorf("out.Duration = %f, want 45.5", out.Duration)
	}
}

// TestJobGet_EmptyProjectID verifies JobGet when empty project ID.
func TestJobGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))

	_, err := Get(context.Background(), client, GetInput{JobID: 100})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestJobTrace_Success verifies JobTrace when success.
func TestJobTrace_Success(t *testing.T) {
	traceContent := "Running with gitlab-runner 15.0.0\nJob succeeded"
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
			w.Header().Set(testHeaderContentType, "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(traceContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Trace(context.Background(), client, TraceInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Trace() unexpected error: %v", err)
	}
	if out.JobID != 100 {
		t.Errorf("out.JobID = %d, want 100", out.JobID)
	}
	if out.Trace != traceContent {
		t.Errorf("out.Trace = %q, want %q", out.Trace, traceContent)
	}
	if out.Truncated {
		t.Error("out.Truncated = true, want false")
	}
}

// TestJobTrace_Truncated verifies Trace caps large job logs at maxTraceBytes and
// marks the output as truncated.
//
// The mock returns a trace slightly larger than the configured limit. The test
// expects the trace length to equal maxTraceBytes and Truncated to be true,
// protecting clients from unbounded job-log responses.
func TestJobTrace_Truncated(t *testing.T) {
	traceContent := strings.Repeat("x", maxTraceBytes+10)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
			w.Header().Set(testHeaderContentType, "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(traceContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Trace() unexpected error: %v", err)
	}
	if !out.Truncated {
		t.Fatal("out.Truncated = false, want true")
	}
	if len(out.Trace) != maxTraceBytes {
		t.Fatalf("len(out.Trace) = %d, want %d", len(out.Trace), maxTraceBytes)
	}
}

// TestJobTrace_EmptyProjectID verifies JobTrace when empty project ID.
func TestJobTrace_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	_, err := Trace(context.Background(), client, TraceInput{JobID: 100})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestJobCancel_Success verifies JobCancel when success.
func TestJobCancel_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobCancel {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":100,"name":"build","stage":"build","status":"canceled",
				"ref":"main","tag":false,"duration":10.0,"queued_duration":1.0,
				"web_url":"https://gitlab.example.com/-/jobs/100",
				"pipeline":{"id":10},"created_at":"2026-03-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Cancel(context.Background(), client, CancelInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Cancel() unexpected error: %v", err)
	}
	if out.Status != "canceled" {
		t.Errorf("out.Status = %q, want %q", out.Status, "canceled")
	}
}

// TestJobCancel_EmptyProjectID verifies JobCancel when empty project ID.
func TestJobCancel_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))

	_, err := Cancel(context.Background(), client, CancelInput{JobID: 100})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestJobRetry_Success verifies JobRetry when success.
func TestJobRetry_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobRetry {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":101,"name":"build","stage":"build","status":"pending",
				"ref":"main","tag":false,"duration":0,"queued_duration":0,
				"web_url":"https://gitlab.example.com/-/jobs/101",
				"pipeline":{"id":10},"created_at":"2026-03-01T10:01:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Retry(context.Background(), client, ActionInput{
		ProjectID: "42",
		JobID:     100,
	})
	if err != nil {
		t.Fatalf("Retry() unexpected error: %v", err)
	}
	if out.Status != "pending" {
		t.Errorf("out.Status = %q, want %q", out.Status, "pending")
	}
	if out.ID != 101 {
		t.Errorf("out.ID = %d, want 101", out.ID)
	}
}

// TestJobRetry_EmptyProjectID verifies JobRetry when empty project ID.
func TestJobRetry_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))

	_, err := Retry(context.Background(), client, ActionInput{JobID: 100})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestJobList_CancelledContext verifies JobList when cancelled context.
func TestJobList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}

// TASK-024 tests.

const (
	// pathProjectJobs identifies the path project jobs constant used by this package.
	pathProjectJobs = "/api/v4/projects/42/jobs"
	// pathJobErase identifies the path job erase constant used by this package.
	pathJobErase = "/api/v4/projects/42/jobs/100/erase"
	// pathJobPlay identifies the path job play constant used by this package.
	pathJobPlay = "/api/v4/projects/42/jobs/100/play"
	// pathJobArtifacts identifies the path job artifacts constant used by this package.
	pathJobArtifacts = "/api/v4/projects/42/jobs/100/artifacts"

	// bridgeJSON identifies the bridge JSON constant used by this package.
	bridgeJSON = `{
		"id":200,"name":"trigger-downstream","stage":"deploy",
		"status":"success","ref":"main","tag":false,"allow_failure":false,
		"duration":10.0,"queued_duration":1.0,
		"web_url":"https://gitlab.example.com/-/jobs/200",
		"pipeline":{"id":10},
		"created_at":"2026-03-01T10:00:00Z",
		"user":{"username":"testuser"},
		"downstream_pipeline":{"id":50}
	}`

	// msgMissingProject identifies the msg missing project constant used by this package.
	msgMissingProject = "expected error for empty project_id"
)

// TestListProject_Success verifies ListProject when success.
func TestListProject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("ListProject() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	if out.Jobs[0].ID != 100 {
		t.Errorf("Jobs[0].ID = %d, want 100", out.Jobs[0].ID)
	}
}

// TestListProject_MissingProject verifies ListProject when missing project.
func TestListProject_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestListBridges_Success verifies ListBridges when success.
func TestListBridges_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipelines/10/bridges" {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeJSON))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("ListBridges() unexpected error: %v", err)
	}
	if len(out.Bridges) != 1 {
		t.Fatalf("len(Bridges) = %d, want 1", len(out.Bridges))
	}
	if out.Bridges[0].ID != 200 {
		t.Errorf("Bridges[0].ID = %d, want 200", out.Bridges[0].ID)
	}
	if out.Bridges[0].DownstreamPipeline != 50 {
		t.Errorf("Bridges[0].DownstreamPipeline = %d, want 50", out.Bridges[0].DownstreamPipeline)
	}
}

// TestListBridges_MissingProject verifies ListBridges when missing project.
func TestListBridges_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := ListBridges(context.Background(), client, BridgeListInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestGetArtifacts_Success verifies GetArtifacts when success.
func TestGetArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobArtifacts {
			w.Header().Set(testHeaderContentType, "application/zip")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PK\x03\x04fake-zip-content"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetArtifacts(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("GetArtifacts() unexpected error: %v", err)
	}
	if out.Size == 0 {
		t.Error("Size = 0, want > 0")
	}
	if out.Content == "" {
		t.Error("Content is empty")
	}
}

// TestGetArtifacts_MissingProject verifies GetArtifacts when missing project.
func TestGetArtifacts_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetArtifacts(context.Background(), client, GetInput{JobID: 100})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestDownloadArtifacts_Success verifies DownloadArtifacts when success.
func TestDownloadArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/artifacts/main/download" {
			w.Header().Set(testHeaderContentType, "application/zip")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("PK\x03\x04fake-zip"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DownloadArtifacts(context.Background(), client, DownloadArtifactsInput{
		ProjectID: "42", RefName: "main", JobName: "build",
	})
	if err != nil {
		t.Fatalf("DownloadArtifacts() unexpected error: %v", err)
	}
	if out.Size == 0 {
		t.Error("Size = 0, want > 0")
	}
}

// TestDownloadArtifacts_MissingRef verifies DownloadArtifacts when missing ref.
func TestDownloadArtifacts_MissingRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadArtifacts(context.Background(), client, DownloadArtifactsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing ref_name")
	}
}

// TestDownloadSingleArtifact_Success verifies DownloadSingleArtifact when success.
func TestDownloadSingleArtifact_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/100/artifacts/"+testReportFileName {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testReportContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{
		ProjectID: "42", JobID: 100, ArtifactPath: testReportFileName,
	})
	if err != nil {
		t.Fatalf("DownloadSingleArtifact() unexpected error: %v", err)
	}
	if out.Content != testReportContent {
		t.Errorf("Content = %q, want %q", out.Content, testReportContent)
	}
	if out.ArtifactPath != testReportFileName {
		t.Errorf("ArtifactPath = %q, want %q", out.ArtifactPath, testReportFileName)
	}
}

// TestDownloadSingleArtifact_MissingPath verifies DownloadSingleArtifact when missing path.
func TestDownloadSingleArtifact_MissingPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal("expected error for missing artifact_path")
	}
}

// TestDownloadSingleArtifactByRef_Success verifies DownloadSingleArtifactByRef when success.
func TestDownloadSingleArtifactByRef_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/artifacts/main/raw/"+testReportFileName {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(testRefArtifactContent))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: testReportFileName, JobName: "build",
	})
	if err != nil {
		t.Fatalf("DownloadSingleArtifactByRef() unexpected error: %v", err)
	}
	if out.Content != testRefArtifactContent {
		t.Errorf("Content = %q, want %q", out.Content, testRefArtifactContent)
	}
}

// TestDownloadSingleArtifactByRef_MissingRef verifies DownloadSingleArtifactByRef when missing ref.
func TestDownloadSingleArtifactByRef_MissingRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", ArtifactPath: testReportFileName,
	})
	if err == nil {
		t.Fatal("expected error for missing ref_name")
	}
}

// TestErase_Success verifies Erase when success.
func TestErase_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobErase {
			testutil.RespondJSON(w, http.StatusCreated, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Erase() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestErase_MissingProject verifies Erase when missing project.
func TestErase_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Erase(context.Background(), client, ActionInput{JobID: 100})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestKeepArtifacts_Success verifies KeepArtifacts when success.
func TestKeepArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/jobs/100/artifacts/keep" {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("KeepArtifacts() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestKeepArtifacts_MissingProject verifies KeepArtifacts when missing project.
func TestKeepArtifacts_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := KeepArtifacts(context.Background(), client, ActionInput{JobID: 100})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestPlay_Success verifies Play when success.
func TestPlay_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Play() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestPlay_MissingProject verifies Play when missing project.
func TestPlay_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Play(context.Background(), client, PlayInput{JobID: 100})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestDeleteArtifacts_Success verifies DeleteArtifacts when success.
func TestDeleteArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathJobArtifacts {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("DeleteArtifacts() unexpected error: %v", err)
	}
}

// TestDeleteArtifacts_MissingProject verifies DeleteArtifacts when missing project.
func TestDeleteArtifacts_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{JobID: 100})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// TestDeleteProjectArtifacts_Success verifies DeleteProjectArtifacts when success.
func TestDeleteProjectArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/artifacts" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))

	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("DeleteProjectArtifacts() unexpected error: %v", err)
	}
}

// TestDeleteProjectArtifacts_MissingProject verifies DeleteProjectArtifacts when missing project.
func TestDeleteProjectArtifacts_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{})
	if err == nil {
		t.Fatal(msgMissingProject)
	}
}

// ---------------------------------------------------------------------------
// assertContains verifies that err is non-nil and its message contains substr.
// ---------------------------------------------------------------------------.
func assertContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestJobIDRequired_Validation ensures all handlers that require job_id
// reject zero and negative values.
func TestJobIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when job_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get_zero", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, JobID: 0}); return e }},
		{"Get_negative", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, JobID: -1}); return e }},
		{"Trace_zero", func() error { _, e := Trace(ctx, client, TraceInput{ProjectID: pid, JobID: 0}); return e }},
		{"Trace_negative", func() error { _, e := Trace(ctx, client, TraceInput{ProjectID: pid, JobID: -1}); return e }},
		{"Cancel_zero", func() error { _, e := Cancel(ctx, client, CancelInput{ProjectID: pid, JobID: 0}); return e }},
		{"Cancel_negative", func() error { _, e := Cancel(ctx, client, CancelInput{ProjectID: pid, JobID: -3}); return e }},
		{"Retry_zero", func() error { _, e := Retry(ctx, client, ActionInput{ProjectID: pid, JobID: 0}); return e }},
		{"Retry_negative", func() error { _, e := Retry(ctx, client, ActionInput{ProjectID: pid, JobID: -1}); return e }},
		{"GetArtifacts_zero", func() error { _, e := GetArtifacts(ctx, client, GetInput{ProjectID: pid, JobID: 0}); return e }},
		{"GetArtifacts_negative", func() error { _, e := GetArtifacts(ctx, client, GetInput{ProjectID: pid, JobID: -1}); return e }},
		{"DownloadSingleArtifact_zero", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{ProjectID: pid, JobID: 0, ArtifactPath: "a.txt"})
			return e
		}},
		{"DownloadSingleArtifact_negative", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{ProjectID: pid, JobID: -2, ArtifactPath: "a.txt"})
			return e
		}},
		{"Erase_zero", func() error { _, e := Erase(ctx, client, ActionInput{ProjectID: pid, JobID: 0}); return e }},
		{"Erase_negative", func() error { _, e := Erase(ctx, client, ActionInput{ProjectID: pid, JobID: -1}); return e }},
		{"KeepArtifacts_zero", func() error { _, e := KeepArtifacts(ctx, client, ActionInput{ProjectID: pid, JobID: 0}); return e }},
		{"KeepArtifacts_negative", func() error { _, e := KeepArtifacts(ctx, client, ActionInput{ProjectID: pid, JobID: -5}); return e }},
		{"Play_zero", func() error { _, e := Play(ctx, client, PlayInput{ProjectID: pid, JobID: 0}); return e }},
		{"Play_negative", func() error { _, e := Play(ctx, client, PlayInput{ProjectID: pid, JobID: -1}); return e }},
		{"DeleteArtifacts_zero", func() error { return DeleteArtifacts(ctx, client, DeleteArtifactsInput{ProjectID: pid, JobID: 0}) }},
		{"DeleteArtifacts_negative", func() error { return DeleteArtifacts(ctx, client, DeleteArtifactsInput{ProjectID: pid, JobID: -1}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "job_id")
		})
	}
}

// TestPipelineIDRequired_ValidationJobs ensures handlers that require pipeline_id
// reject zero and negative values.
func TestPipelineIDRequired_ValidationJobs(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when pipeline_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List_zero", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"List_negative", func() error { _, e := List(ctx, client, ListInput{ProjectID: pid, PipelineID: -1}); return e }},
		{"ListBridges_zero", func() error {
			_, e := ListBridges(ctx, client, BridgeListInput{ProjectID: pid, PipelineID: 0})
			return e
		}},
		{"ListBridges_negative", func() error {
			_, e := ListBridges(ctx, client, BridgeListInput{ProjectID: pid, PipelineID: -1})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "pipeline_id")
		})
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpCancelledNil identifies the err exp cancelled nil constant used by this package.
const errExpCancelledNil = "expected error for canceled context, got nil"

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// List — API error, pagination params, include_retried
// ---------------------------------------------------------------------------.

// TestJobList_APIError verifies JobList when API error.
func TestJobList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobList_WithPaginationAndIncludeRetried verifies JobList when with pagination and include retried.
func TestJobList_WithPaginationAndIncludeRetried(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			q := r.URL.Query()
			if q.Get("page") != "2" {
				t.Errorf("expected page=2, got %q", q.Get("page"))
			}
			if q.Get("per_page") != "5" {
				t.Errorf("expected per_page=5, got %q", q.Get("per_page"))
			}
			if q.Get("include_retried") != "true" {
				t.Errorf("expected include_retried=true, got %q", q.Get("include_retried"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:       "42",
		PipelineID:      10,
		IncludeRetried:  true,
		PaginationInput: toolutil.PaginationInput{Page: 2, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
}

// ---------------------------------------------------------------------------
// Get — API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobGet_APIError verifies JobGet when API error.
func TestJobGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobGet_CancelledContext verifies JobGet when cancelled context.
func TestJobGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Trace — API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobTrace_APIError verifies JobTrace when API error.
func TestJobTrace_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobTrace_CancelledContext verifies JobTrace when cancelled context.
func TestJobTrace_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Trace(ctx, client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestJobTrace_BodyReadError verifies Trace wraps non-EOF/non-UnexpectedEOF
// errors from the body reader. The server claims gzip content-encoding but
// writes raw text, so the http transport's auto-decompression fails with a
// gzip error that is neither io.EOF nor io.ErrUnexpectedEOF, exercising the
// fallback error path in Trace.
func TestJobTrace_BodyReadError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
			w.Header().Set(testHeaderContentType, "text/plain")
			// Claim gzip so Go's http transport auto-decompresses; the body
			// is not actually gzipped, so Read on the gzip reader errors.
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("definitely not gzipped data"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal("expected error from invalid gzip body, got nil")
	}
	if !strings.Contains(err.Error(), "jobTrace") {
		t.Errorf("error = %v, want jobTrace context", err)
	}
}

// ---------------------------------------------------------------------------
// Cancel — API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobCancel_APIError verifies JobCancel when API error.
func TestJobCancel_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Cancel(context.Background(), client, CancelInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobCancel_NotFoundAPIError verifies JobCancel when GitLab returns not found.
func TestJobCancel_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Cancel(context.Background(), client, CancelInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "gitlab_job_list")
}

// TestJobCancel_ForceTrue verifies Cancel with Force=true routes to
// CancelJobWithOptions, sends force=true in the request body, and returns the cancelled job.
func TestJobCancel_ForceTrue(t *testing.T) {
	forceSent := false
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobCancel {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				if v, ok := body["force"].(bool); ok && v {
					forceSent = true
				}
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":100,"name":"build","stage":"build","status":"canceled",
				"ref":"main","tag":false,"duration":10.0,"queued_duration":1.0,
				"web_url":"https://gitlab.example.com/-/jobs/100",
				"pipeline":{"id":10},"created_at":"2026-03-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Cancel(context.Background(), client, CancelInput{
		ProjectID: "42",
		JobID:     100,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("Cancel(Force=true) unexpected error: %v", err)
	}
	if !forceSent {
		t.Error("Cancel(Force=true) did not send force=true in request body")
	}
	if out.Status != "canceled" {
		t.Errorf("out.Status = %q, want canceled", out.Status)
	}
	if out.ID != 100 {
		t.Errorf(fmtIDWant100, out.ID)
	}
}

// TestJobCancel_CancelledContext verifies JobCancel when cancelled context.
func TestJobCancel_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Cancel(ctx, client, CancelInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Retry — API error, canceled context
// ---------------------------------------------------------------------------.

// TestJobRetry_APIError verifies JobRetry when API error.
func TestJobRetry_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestJobRetry_NotFoundAPIError verifies JobRetry when GitLab returns not found.
func TestJobRetry_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "gitlab_job_list")
}

// TestJobRetry_CancelledContext verifies JobRetry when cancelled context.
func TestJobRetry_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Retry(ctx, client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// ListProject — API error, canceled context, with scope and pagination
// ---------------------------------------------------------------------------.

// TestListProject_APIError verifies ListProject when API error.
func TestListProject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListProject(context.Background(), client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListProject_CancelledContext verifies ListProject when cancelled context.
func TestListProject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListProject(ctx, client, ListProjectInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListProject_WithScopeAndPagination verifies ListProject when with scope and pagination.
func TestListProject_WithScopeAndPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			q := r.URL.Query()
			scopes := q["scope[]"]
			if len(scopes) != 2 || scopes[0] != "running" || scopes[1] != "failed" {
				t.Errorf("expected scope[]=[running,failed], got %v", scopes)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "10", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:       "42",
		Scope:           []string{"running", "failed"},
		IncludeRetried:  true,
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 10},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
}

// ---------------------------------------------------------------------------
// ListBridges — API error, canceled context, with scope and pagination
// ---------------------------------------------------------------------------.

// TestListBridges_APIError verifies ListBridges when API error.
func TestListBridges_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestListBridges_CancelledContext verifies ListBridges when cancelled context.
func TestListBridges_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListBridges(ctx, client, BridgeListInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestListBridges_WithScopeAndPagination verifies ListBridges when with scope and pagination.
func TestListBridges_WithScopeAndPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipelines/10/bridges" {
			q := r.URL.Query()
			scopes := q["scope[]"]
			if len(scopes) != 1 || scopes[0] != "success" {
				t.Errorf("expected scope[]=[success], got %v", scopes)
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "5", Total: "1", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListBridges(context.Background(), client, BridgeListInput{
		ProjectID:       "42",
		PipelineID:      10,
		Scope:           []string{"success"},
		PaginationInput: toolutil.PaginationInput{Page: 1, PerPage: 5},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Bridges) != 1 {
		t.Fatalf("len(Bridges) = %d, want 1", len(out.Bridges))
	}
}

// ---------------------------------------------------------------------------
// GetArtifacts — API error, canceled context
// ---------------------------------------------------------------------------.

// TestGetArtifacts_APIError verifies GetArtifacts when API error.
func TestGetArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetArtifacts(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetArtifacts_CancelledContext verifies GetArtifacts when cancelled context.
func TestGetArtifacts_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK"))
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetArtifacts(ctx, client, GetInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// DownloadArtifacts — API error, canceled context, missing project_id
// ---------------------------------------------------------------------------.

// TestDownloadArtifacts_APIError verifies DownloadArtifacts when API error.
func TestDownloadArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := DownloadArtifacts(context.Background(), client, DownloadArtifactsInput{
		ProjectID: "42", RefName: "main",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDownloadArtifacts_CancelledContext verifies DownloadArtifacts when cancelled context.
func TestDownloadArtifacts_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadArtifacts(ctx, client, DownloadArtifactsInput{
		ProjectID: "42", RefName: "main",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDownloadArtifacts_MissingProjectID verifies DownloadArtifacts when missing project ID.
func TestDownloadArtifacts_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadArtifacts(context.Background(), client, DownloadArtifactsInput{RefName: "main"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// DownloadSingleArtifact — API error, canceled context, missing project_id
// ---------------------------------------------------------------------------.

// TestDownloadSingleArtifact_APIError verifies DownloadSingleArtifact when API error.
func TestDownloadSingleArtifact_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{
		ProjectID: "42", JobID: 100, ArtifactPath: "report.txt",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDownloadSingleArtifact_CancelledContext verifies DownloadSingleArtifact when cancelled context.
func TestDownloadSingleArtifact_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadSingleArtifact(ctx, client, SingleArtifactInput{
		ProjectID: "42", JobID: 100, ArtifactPath: "report.txt",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDownloadSingleArtifact_MissingProjectID verifies DownloadSingleArtifact when missing project ID.
func TestDownloadSingleArtifact_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{
		JobID: 100, ArtifactPath: "report.txt",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// DownloadSingleArtifactByRef — API error, canceled context, missing fields
// ---------------------------------------------------------------------------.

// TestDownloadSingleArtifactByRef_APIError verifies DownloadSingleArtifactByRef when API error.
func TestDownloadSingleArtifactByRef_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: "report.txt", JobName: "build",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDownloadSingleArtifactByRef_CancelledContext verifies DownloadSingleArtifactByRef when cancelled context.
func TestDownloadSingleArtifactByRef_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: "report.txt", JobName: "build",
	})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDownloadSingleArtifactByRef_MissingProjectID verifies DownloadSingleArtifactByRef when missing project ID.
func TestDownloadSingleArtifactByRef_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		RefName: "main", ArtifactPath: "report.txt", JobName: "build",
	})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestDownloadSingleArtifactByRef_MissingArtifactPath verifies DownloadSingleArtifactByRef when missing artifact path.
func TestDownloadSingleArtifactByRef_MissingArtifactPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", JobName: "build",
	})
	if err == nil {
		t.Fatal("expected error for missing artifact_path, got nil")
	}
}

// TestDownloadSingleArtifactByRef_MissingJob verifies DownloadSingleArtifactByRef when missing job name.
func TestDownloadSingleArtifactByRef_MissingJob(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := DownloadSingleArtifactByRef(context.Background(), client, SingleArtifactRefInput{
		ProjectID: "42", RefName: "main", ArtifactPath: testReportFileName,
	})
	if err == nil {
		t.Fatal("expected error for missing job, got nil")
	}
}

// ---------------------------------------------------------------------------
// Erase — API error, canceled context
// ---------------------------------------------------------------------------.

// TestErase_APIError verifies Erase when API error.
func TestErase_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestErase_NotFoundAPIError verifies Erase when GitLab returns not found.
func TestErase_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "gitlab_job_list")
}

// TestErase_CancelledContext verifies Erase when cancelled context.
func TestErase_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Erase(ctx, client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// KeepArtifacts — API error, canceled context
// ---------------------------------------------------------------------------.

// TestKeepArtifacts_APIError verifies KeepArtifacts when API error.
func TestKeepArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestKeepArtifacts_NotFoundAPIError verifies KeepArtifacts when GitLab returns not found.
func TestKeepArtifacts_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "artifacts")
}

// TestKeepArtifacts_CancelledContext verifies KeepArtifacts when cancelled context.
func TestKeepArtifacts_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := KeepArtifacts(ctx, client, ActionInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// Play — API error, canceled context, with variables
// ---------------------------------------------------------------------------.

// TestPlay_APIError verifies Play when API error.
func TestPlay_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestPlay_BadRequestAPIError verifies Play when the job is not playable.
func TestPlay_BadRequestAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"job is not playable"}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "manual jobs")
}

// TestPlay_NotFoundAPIError verifies Play when GitLab returns not found.
func TestPlay_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "gitlab_job_list")
}

// TestPlay_CancelledContext verifies Play when cancelled context.
func TestPlay_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Play(ctx, client, PlayInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestPlay_WithVariables verifies Play when with variables.
func TestPlay_WithVariables(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		Variables: []JobVariableInput{
			{Key: "ENV", Value: "production", VariableType: "env_var"},
			{Key: "SECRET", Value: "/tmp/secret", VariableType: "file"},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 100 {
		t.Errorf("ID = %d, want 100", out.ID)
	}
}

// ---------------------------------------------------------------------------
// DeleteArtifacts — API error
// ---------------------------------------------------------------------------.

// TestDeleteArtifacts_APIError verifies DeleteArtifacts when API error.
func TestDeleteArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteArtifacts_NotFoundAPIError verifies DeleteArtifacts when GitLab returns not found.
func TestDeleteArtifacts_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Job Not Found"}`)
	}))
	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "no artifacts")
}

// ---------------------------------------------------------------------------
// DeleteProjectArtifacts — API error
// ---------------------------------------------------------------------------.

// TestDeleteProjectArtifacts_APIError verifies DeleteProjectArtifacts when API error.
func TestDeleteProjectArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDeleteProjectArtifacts_NotFoundAPIError verifies DeleteProjectArtifacts when GitLab returns not found.
func TestDeleteProjectArtifacts_NotFoundAPIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))
	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	assertContains(t, err, "gitlab_project_get")
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_AllFields verifies FormatOutputMarkdown when all fields.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:             100,
		Name:           "build",
		Stage:          "build",
		Status:         "success",
		PipelineID:     10,
		Ref:            "main",
		CommitSHA:      "abcdef1234567890",
		AllowFailure:   true,
		Duration:       45.5,
		QueuedDuration: 2.1,
		FailureReason:  "script_failure",
		Coverage:       85.5,
		UserUsername:   "testuser",
		CreatedAt:      "2026-03-01T10:00:00Z",
		WebURL:         "https://gitlab.example.com/-/jobs/100",
	})

	for _, want := range []string{
		"Job #100",
		"build",
		"**Pipeline**: #10",
		"**Stage**: build",
		"**Status**: success",
		"**Allow Failure**: yes",
		"**Ref**: main",
		"`abcdef123456`",
		"**Duration**: 45.5s",
		"**Queued**: 2.1s",
		"**Failure Reason**: script_failure",
		"**Coverage**: 85.5%",
		"**User**: testuser",
		"**Created**:",
		"https://gitlab.example.com/-/jobs/100",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatWaitResult_TimedOutMarksError verifies timeout wait results are marked as tool errors.
func TestFormatWaitResult_TimedOutMarksError(t *testing.T) {
	result := formatWaitResult(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "running", Stage: "build", Ref: "main"},
		FinalStatus: "running",
		TimedOut:    true,
		WaitedFor:   "1s",
		PollCount:   1,
	})
	if result == nil {
		t.Fatal("formatWaitResult() returned nil")
	}
	if !result.IsError {
		t.Fatal("formatWaitResult() IsError = false, want true")
	}
}

// TestFormatOutputMarkdown_MinimalFields verifies FormatOutputMarkdown when minimal fields.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:     50,
		Name:   "test",
		Stage:  "test",
		Status: "running",
		Ref:    "develop",
	})

	if !strings.Contains(md, "Job #50") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"**Duration**",
		"**Queued**",
		"**Failure Reason**",
		"**Coverage**",
		"**User**",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithJobs verifies FormatListMarkdown when with jobs.
func TestFormatListMarkdown_WithJobs(t *testing.T) {
	out := ListOutput{
		Jobs: []Output{
			{ID: 100, Name: "build", Stage: "build", Status: "success", Duration: 45.5},
			{ID: 101, Name: "test", Stage: "test", Status: "failed", Duration: 12.3},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Jobs (2)",
		"| ID |",
		"| --- |",
		"[#100]",
		"[#101]",
		"build",
		"test",
		"success",
		"failed",
		"45.5s",
		"12.3s",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies FormatListMarkdown when empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	md := FormatListMarkdown(ListOutput{})
	if !strings.Contains(md, "No jobs found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_ClickableJobLinks verifies that job IDs
// in the list are rendered as clickable Markdown links [#ID](weburl).
func TestFormatListMarkdown_ClickableJobLinks(t *testing.T) {
	out := ListOutput{
		Jobs: []Output{
			{
				ID: 200, Name: "deploy", Stage: "deploy", Status: "success", Duration: 10.0,
				WebURL: "https://gitlab.example.com/-/jobs/200",
			},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "[#200](https://gitlab.example.com/-/jobs/200)") {
		t.Errorf("expected clickable job link, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatTraceMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTraceMarkdown_WithData verifies FormatTraceMarkdown when with data.
func TestFormatTraceMarkdown_WithData(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{
		JobID: 100,
		Trace: "Running with gitlab-runner 15.0.0\nJob succeeded",
	})

	for _, want := range []string{
		"## Job #100 Trace",
		"```",
		"Running with gitlab-runner 15.0.0",
		"Job succeeded",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "Truncated") {
		t.Error("should not contain truncation warning")
	}
}

// TestFormatTraceMarkdown_Truncated verifies FormatTraceMarkdown when truncated.
func TestFormatTraceMarkdown_Truncated(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{
		JobID:     100,
		Trace:     "partial log...",
		Truncated: true,
	})

	if !strings.Contains(md, "Trace truncated at 100KB") {
		t.Errorf("missing truncation warning:\n%s", md)
	}
}

// TestFormatTraceMarkdown_Empty verifies FormatTraceMarkdown when empty.
func TestFormatTraceMarkdown_Empty(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{JobID: 99})
	if !strings.Contains(md, "## Job #99 Trace") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "```") {
		t.Errorf("missing code fence:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatBridgeListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatBridgeListMarkdown_WithData verifies FormatBridgeListMarkdown when with data.
func TestFormatBridgeListMarkdown_WithData(t *testing.T) {
	out := BridgeListOutput{
		Bridges: []BridgeOutput{
			{ID: 200, Name: "trigger-downstream", Stage: "deploy", Status: "success", Duration: 10.0, DownstreamPipeline: 50},
			{ID: 201, Name: "trigger-other", Stage: "deploy", Status: "failed", Duration: 5.0},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatBridgeListMarkdown(out)

	for _, want := range []string{
		"## Bridge Jobs (2)",
		"| ID |",
		"| --- |",
		"| 200 |",
		"| 201 |",
		"trigger-downstream",
		"trigger-other",
		"success",
		"failed",
		"#50",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatBridgeListMarkdown_Empty verifies FormatBridgeListMarkdown when empty.
func TestFormatBridgeListMarkdown_Empty(t *testing.T) {
	md := FormatBridgeListMarkdown(BridgeListOutput{})
	if !strings.Contains(md, "No bridge jobs found") {
		t.Errorf("expected empty message:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatArtifactsMarkdown
// ---------------------------------------------------------------------------.

// TestFormatArtifactsMarkdown_WithJobID verifies FormatArtifactsMarkdown when with job ID.
func TestFormatArtifactsMarkdown_WithJobID(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{
		JobID: 100,
		Size:  2048,
	})

	for _, want := range []string{
		"## Job #100 Artifacts",
		"**Size**: 2048 bytes",
		"base64-encoded",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "Truncated") {
		t.Error("should not contain truncation warning")
	}
}

// TestFormatArtifactsMarkdown_WithoutJobID verifies FormatArtifactsMarkdown when without job ID.
func TestFormatArtifactsMarkdown_WithoutJobID(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{Size: 512})
	if !strings.Contains(md, "## Artifacts") {
		t.Errorf("missing generic header:\n%s", md)
	}
	if strings.Contains(md, "Job #0") {
		t.Error("should not have job-specific header when JobID=0")
	}
}

// TestFormatArtifactsMarkdown_Truncated verifies FormatArtifactsMarkdown when truncated.
func TestFormatArtifactsMarkdown_Truncated(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{
		JobID:     100,
		Size:      1048576,
		Truncated: true,
	})
	if !strings.Contains(md, "Truncated") {
		t.Errorf("missing truncation warning:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatSingleArtifactMarkdown
// ---------------------------------------------------------------------------.

// TestFormatSingleArtifactMarkdown_WithJobID verifies FormatSingleArtifactMarkdown when with job ID.
func TestFormatSingleArtifactMarkdown_WithJobID(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		JobID:        100,
		ArtifactPath: "report.txt",
		Size:         256,
		Content:      "test report content",
	})

	for _, want := range []string{
		"## Job #100",
		"report.txt",
		"**Size**: 256 bytes",
		"test report content",
		"```",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatSingleArtifactMarkdown_WithoutJobID verifies FormatSingleArtifactMarkdown when without job ID.
func TestFormatSingleArtifactMarkdown_WithoutJobID(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		ArtifactPath: "output.log",
		Size:         64,
		Content:      "log data",
	})
	if !strings.Contains(md, "## output.log") {
		t.Errorf("missing path-only header:\n%s", md)
	}
	if strings.Contains(md, "Job #0") {
		t.Error("should not have job-specific header when JobID=0")
	}
}

// TestFormatSingleArtifactMarkdown_Truncated verifies FormatSingleArtifactMarkdown when truncated.
func TestFormatSingleArtifactMarkdown_Truncated(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		JobID:        100,
		ArtifactPath: "big.bin",
		Size:         1048576,
		Content:      "...",
		Truncated:    true,
	})
	if !strings.Contains(md, "Truncated") {
		t.Errorf("missing truncation warning:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata verifies canonical metadata for job actions.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := jobSpecsByTool(t, specs)

	if len(specs) != 17 {
		t.Fatalf("len(ActionSpecs) = %d, want 17", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "jobs" {
			t.Fatalf("OwnerPackage for %s = %q, want jobs", spec.Name, spec.OwnerPackage)
		}
	}
}

// ---------------------------------------------------------------------------
// ActionSpecsCallAllRoutes — route coverage for all 17 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes validates job routes across multiple scenarios.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newJobsRouteSpecs(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_job_list", map[string]any{"project_id": "42", "pipeline_id": 10}},
		{"get", "gitlab_job_get", map[string]any{"project_id": "42", "job_id": 100}},
		{"trace", "gitlab_job_trace", map[string]any{"project_id": "42", "job_id": 100}},
		{"cancel", "gitlab_job_cancel", map[string]any{"project_id": "42", "job_id": 100}},
		{"retry", "gitlab_job_retry", map[string]any{"project_id": "42", "job_id": 100}},
		{"list_project", "gitlab_job_list_project", map[string]any{"project_id": "42"}},
		{"list_bridges", "gitlab_job_list_bridges", map[string]any{"project_id": "42", "pipeline_id": 10}},
		{"artifacts", "gitlab_job_artifacts", map[string]any{"project_id": "42", "job_id": 100}},
		{"download_artifacts", "gitlab_job_download_artifacts", map[string]any{"project_id": "42", "ref_name": "main", "job": "build"}},
		{"download_single_artifact", "gitlab_job_download_single_artifact", map[string]any{"project_id": "42", "job_id": 100, "artifact_path": "report.txt"}},
		{"download_single_artifact_by_ref", "gitlab_job_download_single_artifact_by_ref", map[string]any{"project_id": "42", "ref_name": "main", "artifact_path": "report.txt", "job": "build"}},
		{"erase", "gitlab_job_erase", map[string]any{"project_id": "42", "job_id": 100}},
		{"keep_artifacts", "gitlab_job_keep_artifacts", map[string]any{"project_id": "42", "job_id": 100}},
		{"play", "gitlab_job_play", map[string]any{"project_id": "42", "job_id": 100}},
		{"delete_artifacts", "gitlab_job_delete_artifacts", map[string]any{"project_id": "42", "job_id": 100}},
		{"delete_project_artifacts", "gitlab_job_delete_project_artifacts", map[string]any{"project_id": "42"}},
		{"wait", "gitlab_job_wait", map[string]any{"project_id": "42", "job_id": 100, "timeout_seconds": 1}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: route spec factory
// ---------------------------------------------------------------------------.

// newJobsRouteSpecs constructs jobs route specs test fixtures.
func newJobsRouteSpecs(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	handler := http.NewServeMux()

	// List pipeline jobs
	handler.HandleFunc("GET /api/v4/projects/42/pipelines/10/jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON))
	})

	// Get job
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	})

	// Trace
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100/trace", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("job log output"))
	})

	// Cancel
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/cancel", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":100,"name":"build","stage":"build","status":"canceled",
			"ref":"main","tag":false,"duration":10.0,"queued_duration":1.0,
			"web_url":"https://gitlab.example.com/-/jobs/100",
			"pipeline":{"id":10},"created_at":"2026-03-01T10:00:00Z"
		}`)
	})

	// Retry
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/retry", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":101,"name":"build","stage":"build","status":"pending",
			"ref":"main","tag":false,"duration":0,"queued_duration":0,
			"web_url":"https://gitlab.example.com/-/jobs/101",
			"pipeline":{"id":10},"created_at":"2026-03-01T10:01:00Z"
		}`)
	})

	// List project jobs
	handler.HandleFunc("GET /api/v4/projects/42/jobs", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON))
	})

	// List bridges
	handler.HandleFunc("GET /api/v4/projects/42/pipelines/10/bridges", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeJSON))
	})

	// Get artifacts
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04fake-zip"))
	})

	// Download artifacts by ref
	handler.HandleFunc("GET /api/v4/projects/42/jobs/artifacts/main/download", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04fake-zip"))
	})

	// Download single artifact by job ID
	handler.HandleFunc("GET /api/v4/projects/42/jobs/100/artifacts/report.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("report content"))
	})

	// Download single artifact by ref
	handler.HandleFunc("GET /api/v4/projects/42/jobs/artifacts/main/raw/report.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ref report content"))
	})

	// Erase
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/erase", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, jobJSON)
	})

	// Keep artifacts
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/artifacts/keep", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	})

	// Play
	handler.HandleFunc("POST /api/v4/projects/42/jobs/100/play", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobJSON)
	})

	// Delete artifacts
	handler.HandleFunc("DELETE /api/v4/projects/42/jobs/100/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete project artifacts
	handler.HandleFunc("DELETE /api/v4/projects/42/artifacts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	client := testutil.NewTestClient(t, handler)
	return jobSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_JobGetRoute verifies the canonical job get route output.
func TestActionSpecs_JobGetRoute(t *testing.T) {
	const respJSON = `{"id":555,"name":"build","stage":"build","status":"success","ref":"main","tag":false}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/jobs/555") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := jobSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_job_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "job_id": 555})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 555 || out.Name != "build" {
		t.Fatalf("job output = %#v, want ID 555 name build", out)
	}
}

// jobSpecsByTool supports job specs by tool assertions in jobs tests.
func jobSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
