// pipelines_test.go contains unit tests for GitLab pipeline listing operations.
// Tests use httptest to mock the GitLab Pipelines API.
package pipelines

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	pathProjectPipelines = "/api/v4/projects/42/pipelines"
	statusSuccess        = "success"
	fmtOutStatusWant     = "out.Status = %q, want %q"
	msgErrEmptyProjectID = "expected error for empty project_id, got nil"
	fmtIDWant10          = "ID = %d, want 10"
)

// TestPipelineList_Success verifies the behavior of pipeline list success.
func TestPipelineList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectPipelines {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{
					"id":1,
					"iid":1,
					"project_id":42,
					"status":"success",
					"source":"push",
					"ref":"main",
					"sha":"abc123",
					"name":"Build",
					"web_url":"https://gitlab.example.com/mygroup/api/-/pipelines/1",
					"created_at":"2026-03-01T10:00:00Z",
					"updated_at":"2026-03-01T10:05:00Z"
				},
				{
					"id":2,
					"iid":2,
					"project_id":42,
					"status":"failed",
					"source":"web",
					"ref":"develop",
					"sha":"def456",
					"name":"Test",
					"web_url":"https://gitlab.example.com/mygroup/api/-/pipelines/2",
					"created_at":"2026-03-02T10:00:00Z",
					"updated_at":"2026-03-02T10:05:00Z"
				}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Pipelines) != 2 {
		t.Fatalf("len(Pipelines) = %d, want 2", len(out.Pipelines))
	}
	if out.Pipelines[0].Status != statusSuccess {
		t.Errorf("Pipelines[0].Status = %q, want %q", out.Pipelines[0].Status, statusSuccess)
	}
	if out.Pipelines[1].Status != "failed" {
		t.Errorf("Pipelines[1].Status = %q, want %q", out.Pipelines[1].Status, "failed")
	}
	if out.Pipelines[0].WebURL == "" {
		t.Error("Pipelines[0].WebURL is empty")
	}
}

// TestPipelineList_WithFilters verifies the behavior of pipeline list with filters.
func TestPipelineList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectPipelines {
			q := r.URL.Query()
			if q.Get("status") != statusSuccess {
				t.Errorf("expected status=success, got %q", q.Get("status"))
			}
			if q.Get("ref") != "main" {
				t.Errorf("expected ref=main, got %q", q.Get("ref"))
			}
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"project_id":42,"status":"success","ref":"main","sha":"abc","web_url":"https://gitlab.example.com/-/pipelines/1"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
		Status:    statusSuccess,
		Ref:       "main",
	})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Pipelines) != 1 {
		t.Fatalf("len(Pipelines) = %d, want 1", len(out.Pipelines))
	}
}

// TestPipelineList_EmptyProjectID verifies the behavior of pipeline list empty project i d.
func TestPipelineList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for empty project_id, got nil")
	}
}

// TestPipelineListServer_Error verifies the behavior of pipeline list server error.
func TestPipelineListServer_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Internal Server Error"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID: "42",
	})
	if err == nil {
		t.Fatal("List() expected error, got nil")
	}
}

// TestPipelineList_CancelledContext verifies the behavior of pipeline list cancelled context.
func TestPipelineList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("List() expected error for canceled context, got nil")
	}
}

const (
	pathPipelineGet    = "/api/v4/projects/42/pipelines/10"
	pathPipelineCancel = "/api/v4/projects/42/pipelines/10/cancel"
	pathPipelineRetry  = "/api/v4/projects/42/pipelines/10/retry"
)

const pipelineDetailJSON = `{
	"id":10,"iid":10,"project_id":42,"status":"success","source":"push",
	"ref":"main","sha":"abc123","before_sha":"def456prev","name":"Build","tag":false,
	"duration":120,"queued_duration":5,"coverage":"85.5",
	"web_url":"https://gitlab.example.com/-/pipelines/10",
	"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:02:00Z",
	"started_at":"2026-03-01T10:00:05Z","finished_at":"2026-03-01T10:02:00Z",
	"user":{"username":"testuser"}
}`

// TestPipelineGet_Success verifies the behavior of pipeline get success.
func TestPipelineGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineGet {
			testutil.RespondJSON(w, http.StatusOK, pipelineDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		ProjectID:  "42",
		PipelineID: 10,
	})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf("out.ID = %d, want 10", out.ID)
	}
	if out.Status != statusSuccess {
		t.Errorf(fmtOutStatusWant, out.Status, statusSuccess)
	}
	if out.Duration != 120 {
		t.Errorf("out.Duration = %d, want 120", out.Duration)
	}
	if out.Coverage != "85.5" {
		t.Errorf("out.Coverage = %q, want %q", out.Coverage, "85.5")
	}
	if out.UserUsername != "testuser" {
		t.Errorf("out.UserUsername = %q, want %q", out.UserUsername, "testuser")
	}
	if out.BeforeSHA != "def456prev" {
		t.Errorf("out.BeforeSHA = %q, want %q", out.BeforeSHA, "def456prev")
	}
}

// TestPipelineGet_EmptyProjectID verifies the behavior of pipeline get empty project i d.
func TestPipelineGet_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineDetailJSON)
	}))

	_, err := Get(context.Background(), client, GetInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestPipelineCancel_Success verifies the behavior of pipeline cancel success.
func TestPipelineCancel_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathPipelineCancel {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"iid":10,"project_id":42,"status":"canceled","source":"push",
				"ref":"main","sha":"abc123","name":"Build",
				"duration":60,"queued_duration":5,
				"web_url":"https://gitlab.example.com/-/pipelines/10",
				"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:01:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Cancel(context.Background(), client, ActionInput{
		ProjectID:  "42",
		PipelineID: 10,
	})
	if err != nil {
		t.Fatalf("Cancel() unexpected error: %v", err)
	}
	if out.Status != "canceled" {
		t.Errorf(fmtOutStatusWant, out.Status, "canceled")
	}
}

// TestPipelineCancel_EmptyProjectID verifies the behavior of pipeline cancel empty project i d.
func TestPipelineCancel_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineDetailJSON)
	}))

	_, err := Cancel(context.Background(), client, ActionInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestPipelineRetry_Success verifies the behavior of pipeline retry success.
func TestPipelineRetry_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathPipelineRetry {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":10,"iid":10,"project_id":42,"status":"running","source":"push",
				"ref":"main","sha":"abc123","name":"Build",
				"duration":0,"queued_duration":0,
				"web_url":"https://gitlab.example.com/-/pipelines/10",
				"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:03:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Retry(context.Background(), client, ActionInput{
		ProjectID:  "42",
		PipelineID: 10,
	})
	if err != nil {
		t.Fatalf("Retry() unexpected error: %v", err)
	}
	if out.Status != "running" {
		t.Errorf(fmtOutStatusWant, out.Status, "running")
	}
}

// TestPipelineRetry_EmptyProjectID verifies the behavior of pipeline retry empty project i d.
func TestPipelineRetry_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineDetailJSON)
	}))

	_, err := Retry(context.Background(), client, ActionInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestPipelineDelete_Success verifies the behavior of pipeline delete success.
func TestPipelineDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathPipelineGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		ProjectID:  "42",
		PipelineID: 10,
	})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

// TestPipelineDelete_EmptyProjectID verifies the behavior of pipeline delete empty project i d.
func TestPipelineDelete_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(context.Background(), client, DeleteInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TASK-023: GetVariables, GetTestReport, GetTestReportSummary, GetLatest, Create, UpdateMetadata tests.

const (
	pathPipeline10 = "/api/v4/projects/42/pipelines/10"

	variablesResponse = `[{"key":"CI_VAR","value":"hello","variable_type":"env_var"},{"key":"SECRET_FILE","value":"/tmp/secret","variable_type":"file"}]`

	testReportResponse = `{
		"total_time":120.5,
		"total_count":10,
		"success_count":8,
		"failed_count":1,
		"skipped_count":1,
		"error_count":0,
		"test_suites":[{
			"name":"Unit Tests","total_time":60.0,
			"total_count":5,"success_count":4,
			"failed_count":1,"skipped_count":0,"error_count":0,
			"test_cases":[]
		}]
	}`

	testReportSummaryResponse = `{
		"total":{"time":120.5,"count":10,"success":8,"failed":1,"skipped":1,"error":0},
		"test_suites":[{
			"name":"Unit Tests","total_time":60.0,
			"total_count":5,"success_count":4,
			"failed_count":1,"skipped_count":0,"error_count":0,
			"build_ids":[101,102]
		}]
	}`
)

// TestGetVariables_Success verifies the behavior of get variables success.
func TestGetVariables_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipeline10+"/variables" {
			testutil.RespondJSON(w, http.StatusOK, variablesResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetVariables(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("GetVariables() unexpected error: %v", err)
	}
	if len(out.Variables) != 2 {
		t.Fatalf("len(Variables) = %d, want 2", len(out.Variables))
	}
	if out.Variables[0].Key != "CI_VAR" {
		t.Errorf("Variables[0].Key = %q, want %q", out.Variables[0].Key, "CI_VAR")
	}
	if out.Variables[1].VariableType != "file" {
		t.Errorf("Variables[1].VariableType = %q, want %q", out.Variables[1].VariableType, "file")
	}
}

// TestGetVariables_MissingProject verifies the behavior of get variables missing project.
func TestGetVariables_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetVariables(context.Background(), client, GetInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestGetTestReport_Success verifies the behavior of get test report success.
func TestGetTestReport_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipeline10+"/test_report" {
			testutil.RespondJSON(w, http.StatusOK, testReportResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetTestReport(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("GetTestReport() unexpected error: %v", err)
	}
	if out.TotalCount != 10 {
		t.Errorf("TotalCount = %d, want 10", out.TotalCount)
	}
	if out.SuccessCount != 8 {
		t.Errorf("SuccessCount = %d, want 8", out.SuccessCount)
	}
	if len(out.TestSuites) != 1 {
		t.Fatalf("len(TestSuites) = %d, want 1", len(out.TestSuites))
	}
	if out.TestSuites[0].Name != "Unit Tests" {
		t.Errorf("TestSuites[0].Name = %q, want %q", out.TestSuites[0].Name, "Unit Tests")
	}
}

// TestGetTestReport_MissingProject verifies the behavior of get test report missing project.
func TestGetTestReport_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetTestReport(context.Background(), client, GetInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestGetTestReportSummary_Success verifies the behavior of get test report summary success.
func TestGetTestReportSummary_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipeline10+"/test_report_summary" {
			testutil.RespondJSON(w, http.StatusOK, testReportSummaryResponse)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetTestReportSummary(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("GetTestReportSummary() unexpected error: %v", err)
	}
	if out.TotalCount != 10 {
		t.Errorf("TotalCount = %d, want 10", out.TotalCount)
	}
	if len(out.TestSuites) != 1 {
		t.Fatalf("len(TestSuites) = %d, want 1", len(out.TestSuites))
	}
	if len(out.TestSuites[0].BuildIDs) != 2 {
		t.Errorf("BuildIDs count = %d, want 2", len(out.TestSuites[0].BuildIDs))
	}
}

// TestGetTestReportSummary_MissingProject verifies the behavior of get test report summary missing project.
func TestGetTestReportSummary_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetTestReportSummary(context.Background(), client, GetInput{PipelineID: 10})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestGetLatest_Success verifies the behavior of get latest success.
func TestGetLatest_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipelines/latest" {
			testutil.RespondJSON(w, http.StatusOK, pipelineDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtIDWant10, out.ID)
	}
	if out.Status != statusSuccess {
		t.Errorf(fmtOutStatusWant, out.Status, statusSuccess)
	}
}

// TestGetLatest_MissingProject verifies the behavior of get latest missing project.
func TestGetLatest_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{})
	if err == nil {
		t.Fatal(msgErrEmptyProjectID)
	}
}

// TestGet_Latest403FallbackToList verifies that GetLatest automatically falls
// back to listing pipelines when the /latest endpoint returns 403 (which
// happens for users with Developer role).
func TestGet_Latest403FallbackToList(t *testing.T) {
	callCount := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch {
		case r.URL.Path == "/api/v4/projects/42/pipelines/latest":
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		case r.URL.Path == "/api/v4/projects/42/pipelines" && r.Method == http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, `[{"id":99,"status":"success","ref":"main","sha":"abc123"}]`)
		case r.URL.Path == "/api/v4/projects/42/pipelines/99":
			testutil.RespondJSON(w, http.StatusOK, `{"id":99,"status":"success","ref":"main","sha":"abc123","web_url":"https://gitlab.example.com/p/-/pipelines/99"}`)
		default:
			http.NotFound(w, r)
		}
	}))

	out, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("GetLatest() with 403 fallback unexpected error: %v", err)
	}
	if out.ID != 99 {
		t.Errorf("GetLatest() fallback ID = %d, want 99", out.ID)
	}
	if out.Status != statusSuccess {
		t.Errorf("GetLatest() fallback Status = %q, want %q", out.Status, statusSuccess)
	}
	if callCount < 3 {
		t.Errorf("Expected at least 3 API calls (latest + list + get), got %d", callCount)
	}
}

// TestCreate_PipelineSuccess verifies the behavior of create pipeline success.
func TestCreate_PipelineSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/pipeline" {
			testutil.RespondJSON(w, http.StatusCreated, pipelineDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Ref: "main"})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtIDWant10, out.ID)
	}
}

// TestCreate_PipelineMissingRef verifies the behavior of create pipeline missing ref.
func TestCreate_PipelineMissingRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
}

// TestUpdateMetadata_Success verifies the behavior of update metadata success.
func TestUpdateMetadata_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == pathPipeline10+"/metadata" {
			testutil.RespondJSON(w, http.StatusOK, pipelineDetailJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := UpdateMetadata(context.Background(), client, UpdateMetadataInput{ProjectID: "42", PipelineID: 10, Name: "New Name"})
	if err != nil {
		t.Fatalf("UpdateMetadata() unexpected error: %v", err)
	}
	if out.ID != 10 {
		t.Errorf(fmtIDWant10, out.ID)
	}
}

// TestUpdateMetadata_MissingName verifies the behavior of update metadata missing name.
func TestUpdateMetadata_MissingName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateMetadata(context.Background(), client, UpdateMetadataInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal("expected error for missing name")
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

// TestPipelineIDRequired_Validation ensures all handlers that require pipeline_id
// reject zero and negative values.
func TestPipelineIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("API should not be called when pipeline_id is invalid")
	}))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Get_zero", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"Get_negative", func() error { _, e := Get(ctx, client, GetInput{ProjectID: pid, PipelineID: -1}); return e }},
		{"Cancel_zero", func() error { _, e := Cancel(ctx, client, ActionInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"Cancel_negative", func() error { _, e := Cancel(ctx, client, ActionInput{ProjectID: pid, PipelineID: -5}); return e }},
		{"Retry_zero", func() error { _, e := Retry(ctx, client, ActionInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"Retry_negative", func() error { _, e := Retry(ctx, client, ActionInput{ProjectID: pid, PipelineID: -3}); return e }},
		{"Delete_zero", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, PipelineID: 0}) }},
		{"Delete_negative", func() error { return Delete(ctx, client, DeleteInput{ProjectID: pid, PipelineID: -2}) }},
		{"GetVariables_zero", func() error { _, e := GetVariables(ctx, client, GetInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"GetVariables_negative", func() error { _, e := GetVariables(ctx, client, GetInput{ProjectID: pid, PipelineID: -1}); return e }},
		{"GetTestReport_zero", func() error { _, e := GetTestReport(ctx, client, GetInput{ProjectID: pid, PipelineID: 0}); return e }},
		{"GetTestReport_negative", func() error { _, e := GetTestReport(ctx, client, GetInput{ProjectID: pid, PipelineID: -1}); return e }},
		{"GetTestReportSummary_zero", func() error {
			_, e := GetTestReportSummary(ctx, client, GetInput{ProjectID: pid, PipelineID: 0})
			return e
		}},
		{"GetTestReportSummary_negative", func() error {
			_, e := GetTestReportSummary(ctx, client, GetInput{ProjectID: pid, PipelineID: -1})
			return e
		}},
		{"UpdateMetadata_zero", func() error {
			_, e := UpdateMetadata(ctx, client, UpdateMetadataInput{ProjectID: pid, PipelineID: 0, Name: "test"})
			return e
		}},
		{"UpdateMetadata_negative", func() error {
			_, e := UpdateMetadata(ctx, client, UpdateMetadataInput{ProjectID: pid, PipelineID: -1, Name: "test"})
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

const errExpCancelledNil = "expected error for canceled context, got nil"

const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// buildListOpts — cover all filter fields
// ---------------------------------------------------------------------------.

// TestBuildListOpts_AllFilters verifies the behavior of build list opts all filters.
func TestBuildListOpts_AllFilters(t *testing.T) {
	input := ListInput{
		ProjectID:     "1",
		Scope:         "running",
		Status:        "success",
		Source:        "push",
		Ref:           "main",
		SHA:           "abc123",
		Name:          "Build",
		Username:      "testuser",
		YamlErrors:    true,
		OrderBy:       "updated_at",
		Sort:          "desc",
		CreatedAfter:  "2026-01-01T00:00:00Z",
		CreatedBefore: "2026-12-31T23:59:59Z",
		UpdatedAfter:  "2026-06-01T00:00:00Z",
		UpdatedBefore: "2026-12-31T00:00:00Z",
	}
	input.Page = 2
	input.PerPage = 50

	opts := buildListOpts(input)

	if opts.Scope == nil || *opts.Scope != "running" {
		t.Errorf("Scope = %v, want running", opts.Scope)
	}
	if opts.Status == nil {
		t.Fatal("Status should not be nil")
	}
	if opts.Source == nil || *opts.Source != "push" {
		t.Errorf("Source = %v, want push", opts.Source)
	}
	if opts.Ref == nil || *opts.Ref != "main" {
		t.Errorf("Ref = %v, want main", opts.Ref)
	}
	if opts.SHA == nil || *opts.SHA != "abc123" {
		t.Errorf("SHA = %v, want abc123", opts.SHA)
	}
	if opts.Username == nil || *opts.Username != "testuser" {
		t.Errorf("Username = %v, want testuser", opts.Username)
	}
	if opts.Name == nil || *opts.Name != "Build" {
		t.Errorf("Name = %v, want Build", opts.Name)
	}
	if opts.YamlErrors == nil || !*opts.YamlErrors {
		t.Error("YamlErrors should be true")
	}
	if opts.OrderBy == nil || *opts.OrderBy != "updated_at" {
		t.Errorf("OrderBy = %v, want updated_at", opts.OrderBy)
	}
	if opts.Sort == nil || *opts.Sort != "desc" {
		t.Errorf("Sort = %v, want desc", opts.Sort)
	}
	if opts.CreatedAfter == nil {
		t.Error("CreatedAfter should not be nil")
	}
	if opts.CreatedBefore == nil {
		t.Error("CreatedBefore should not be nil")
	}
	if opts.UpdatedAfter == nil {
		t.Error("UpdatedAfter should not be nil")
	}
	if opts.UpdatedBefore == nil {
		t.Error("UpdatedBefore should not be nil")
	}
	if opts.Page != 2 {
		t.Errorf("Page = %d, want 2", opts.Page)
	}
	if opts.PerPage != 50 {
		t.Errorf("PerPage = %d, want 50", opts.PerPage)
	}
}

// TestBuildListOpts_Defaults verifies the behavior of build list opts defaults.
func TestBuildListOpts_Defaults(t *testing.T) {
	opts := buildListOpts(ListInput{ProjectID: "1"})

	if opts.Scope != nil {
		t.Errorf("Scope should be nil, got %v", opts.Scope)
	}
	if opts.Status != nil {
		t.Errorf("Status should be nil, got %v", opts.Status)
	}
	if opts.Source != nil {
		t.Errorf("Source should be nil, got %v", opts.Source)
	}
	if opts.YamlErrors != nil {
		t.Errorf("YamlErrors should be nil, got %v", opts.YamlErrors)
	}
	if opts.Page != 0 {
		t.Errorf("Page = %d, want 0", opts.Page)
	}
}

// ---------------------------------------------------------------------------
// ToOutput — with and without timestamps
// ---------------------------------------------------------------------------.

// TestToOutput_NilTimestamps verifies the behavior of to output nil timestamps.
func TestToOutput_NilTimestamps(t *testing.T) {
	p := &gl.PipelineInfo{
		ID: 5, IID: 5, ProjectID: 42,
		Status: "success", Source: "push",
		Ref: "main", SHA: "abc", Name: "Test",
		WebURL: "https://gitlab.example.com/-/pipelines/5",
	}
	out := ToOutput(p)
	if out.CreatedAt != "" {
		t.Errorf("CreatedAt = %q, want empty", out.CreatedAt)
	}
	if out.UpdatedAt != "" {
		t.Errorf("UpdatedAt = %q, want empty", out.UpdatedAt)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if out.Status != "success" {
		t.Errorf("Status = %q, want success", out.Status)
	}
	if out.WebURL != "https://gitlab.example.com/-/pipelines/5" {
		t.Errorf("WebURL = %q, want non-empty", out.WebURL)
	}
}

// ---------------------------------------------------------------------------
// DetailToOutput — all optional fields
// ---------------------------------------------------------------------------.

// TestDetailToOutput_AllOptionalFields verifies the behavior of detail to output all optional fields.
func TestDetailToOutput_AllOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":99,"iid":99,"project_id":42,
			"status":"success","source":"push",
			"ref":"main","sha":"abc123","before_sha":"000",
			"name":"Full Pipeline","tag":true,
			"yaml_errors":"some error","duration":300,"queued_duration":15,
			"coverage":"92.5",
			"detailed_status":{
				"icon":"status_success","text":"passed","label":"passed",
				"group":"success","tooltip":"passed","has_details":true,
				"details_path":"/project/-/pipelines/99","favicon":"icon.png"
			},
			"web_url":"https://gitlab.example.com/-/pipelines/99",
			"created_at":"2026-03-01T10:00:00Z",
			"updated_at":"2026-03-01T10:05:00Z",
			"started_at":"2026-03-01T10:00:05Z",
			"finished_at":"2026-03-01T10:05:00Z",
			"committed_at":"2026-02-28T09:00:00Z",
			"user":{"username":"testuser"}
		}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 99})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.BeforeSHA != "000" {
		t.Errorf("BeforeSHA = %q, want %q", out.BeforeSHA, "000")
	}
	if !out.Tag {
		t.Error("Tag should be true")
	}
	if out.YamlErrors != "some error" {
		t.Errorf("YamlErrors = %q, want %q", out.YamlErrors, "some error")
	}
	if out.Coverage != "92.5" {
		t.Errorf("Coverage = %q, want %q", out.Coverage, "92.5")
	}
	if out.QueuedDuration != 15 {
		t.Errorf("QueuedDuration = %d, want 15", out.QueuedDuration)
	}
	if out.DetailedStatus == nil {
		t.Fatal("DetailedStatus should not be nil")
	}
	if out.DetailedStatus.Icon != "status_success" {
		t.Errorf("DetailedStatus.Icon = %q, want %q", out.DetailedStatus.Icon, "status_success")
	}
	if out.DetailedStatus.HasDetails != true {
		t.Error("DetailedStatus.HasDetails should be true")
	}
	if out.DetailedStatus.DetailsPath != "/project/-/pipelines/99" {
		t.Errorf("DetailedStatus.DetailsPath = %q, want %q", out.DetailedStatus.DetailsPath, "/project/-/pipelines/99")
	}
	if out.DetailedStatus.Favicon != "icon.png" {
		t.Errorf("DetailedStatus.Favicon = %q, want %q", out.DetailedStatus.Favicon, "icon.png")
	}
	if out.StartedAt == "" {
		t.Error("StartedAt should be set")
	}
	if out.FinishedAt == "" {
		t.Error("FinishedAt should be set")
	}
	if out.CommittedAt == "" {
		t.Error("CommittedAt should be set")
	}
	if out.UserUsername != "testuser" {
		t.Errorf("UserUsername = %q, want %q", out.UserUsername, "testuser")
	}
}

// TestDetailToOutput_MinimalFields verifies the behavior of detail to output minimal fields.
func TestDetailToOutput_MinimalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{
			"id":1,"iid":1,"project_id":42,
			"status":"pending","source":"web",
			"ref":"dev","sha":"xyz","name":"",
			"duration":0,"queued_duration":0,
			"web_url":"https://gitlab.example.com/-/pipelines/1",
			"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:00:00Z"
		}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.DetailedStatus != nil {
		t.Error("DetailedStatus should be nil for minimal response")
	}
	if out.Coverage != "" {
		t.Errorf("Coverage = %q, want empty", out.Coverage)
	}
	if out.YamlErrors != "" {
		t.Errorf("YamlErrors = %q, want empty", out.YamlErrors)
	}
	if out.UserUsername != "" {
		t.Errorf("UserUsername = %q, want empty", out.UserUsername)
	}
	if out.StartedAt != "" {
		t.Errorf("StartedAt = %q, want empty", out.StartedAt)
	}
	if out.FinishedAt != "" {
		t.Errorf("FinishedAt = %q, want empty", out.FinishedAt)
	}
	if out.CommittedAt != "" {
		t.Errorf("CommittedAt = %q, want empty", out.CommittedAt)
	}
}

// ---------------------------------------------------------------------------
// Canceled context tests — handlers not already covered
// ---------------------------------------------------------------------------.

// TestGet_CancelledContext verifies the behavior of get cancelled context.
func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Get(ctx, client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCancel_CancelledContext verifies the behavior of cancel cancelled context.
func TestCancel_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Cancel(ctx, client, ActionInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestRetry_CancelledContext verifies the behavior of retry cancelled context.
func TestRetry_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Retry(ctx, client, ActionInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestDelete_CancelledContext verifies the behavior of delete cancelled context.
func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	err := Delete(ctx, client, DeleteInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetVariables_CancelledContext verifies the behavior of get variables cancelled context.
func TestGetVariables_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetVariables(ctx, client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetTestReport_CancelledContext verifies the behavior of get test report cancelled context.
func TestGetTestReport_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetTestReport(ctx, client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetTestReportSummary_CancelledContext verifies the behavior of get test report summary cancelled context.
func TestGetTestReportSummary_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetTestReportSummary(ctx, client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestGetLatest_CancelledContext verifies the behavior of get latest cancelled context.
func TestGetLatest_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetLatest(ctx, client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestCreate_CancelledContext verifies the behavior of create cancelled context.
func TestCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Ref: "main"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// TestUpdateMetadata_CancelledContext verifies the behavior of update metadata cancelled context.
func TestUpdateMetadata_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := UpdateMetadata(ctx, client, UpdateMetadataInput{ProjectID: "42", PipelineID: 10, Name: "x"})
	if err == nil {
		t.Fatal(errExpCancelledNil)
	}
}

// ---------------------------------------------------------------------------
// API error tests — handlers not already covered
// ---------------------------------------------------------------------------.

// TestGet_APIError verifies the behavior of get a p i error.
func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCancel_APIError verifies the behavior of cancel a p i error.
func TestCancel_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Cancel(context.Background(), client, ActionInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestRetry_APIError verifies the behavior of retry a p i error.
func TestRetry_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestDelete_APIError verifies the behavior of delete a p i error.
func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetVariables_APIError verifies the behavior of get variables a p i error.
func TestGetVariables_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetVariables(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetTestReport_APIError verifies the behavior of get test report a p i error.
func TestGetTestReport_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetTestReport(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetTestReportSummary_APIError verifies the behavior of get test report summary a p i error.
func TestGetTestReportSummary_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetTestReportSummary(context.Background(), client, GetInput{ProjectID: "42", PipelineID: 10})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestGetLatest_APIError verifies the behavior of get latest a p i error.
func TestGetLatest_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestCreate_APIError verifies the behavior of create a p i error.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Ref: "main"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// TestUpdateMetadata_APIError verifies the behavior of update metadata a p i error.
func TestUpdateMetadata_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := UpdateMetadata(context.Background(), client, UpdateMetadataInput{ProjectID: "42", PipelineID: 10, Name: "x"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
}

// ---------------------------------------------------------------------------
// Empty required field tests — handlers not already covered
// ---------------------------------------------------------------------------.

// TestCreate_MissingProject verifies the behavior of create missing project.
func TestCreate_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := Create(context.Background(), client, CreateInput{Ref: "main"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// TestUpdateMetadata_MissingProject verifies the behavior of update metadata missing project.
func TestUpdateMetadata_MissingProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := UpdateMetadata(context.Background(), client, UpdateMetadataInput{PipelineID: 10, Name: "x"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
}

// ---------------------------------------------------------------------------
// Create with variables
// ---------------------------------------------------------------------------.

// TestCreate_WithVariables verifies the behavior of create with variables.
func TestCreate_WithVariables(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/pipeline" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":20,"iid":20,"project_id":42,
				"status":"created","source":"api","ref":"main","sha":"aaa",
				"name":"Pipeline with vars","duration":0,"queued_duration":0,
				"web_url":"https://gitlab.example.com/-/pipelines/20",
				"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:00:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42",
		Ref:       "main",
		Variables: []VariableOptionInput{
			{Key: "CI_VAR", Value: "hello", VariableType: "env_var"},
			{Key: "SECRET_FILE", Value: "/tmp/secret", VariableType: "file"},
			{Key: "DEFAULT_TYPE", Value: "val"},
		},
	})
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if out.ID != 20 {
		t.Errorf("ID = %d, want 20", out.ID)
	}
	if out.Status != "created" {
		t.Errorf("Status = %q, want %q", out.Status, "created")
	}
}

// ---------------------------------------------------------------------------
// GetLatest with ref filter
// ---------------------------------------------------------------------------.

// TestGetLatest_WithRef verifies the behavior of get latest with ref.
func TestGetLatest_WithRef(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipelines/latest" {
			if ref := r.URL.Query().Get("ref"); ref != "develop" {
				t.Errorf("expected ref=develop, got %q", ref)
			}
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":15,"iid":15,"project_id":42,
				"status":"success","source":"push","ref":"develop","sha":"def",
				"name":"latest-dev","duration":60,"queued_duration":2,
				"web_url":"https://gitlab.example.com/-/pipelines/15",
				"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:01:00Z"
			}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "42", Ref: "develop"})
	if err != nil {
		t.Fatalf("GetLatest() unexpected error: %v", err)
	}
	if out.Ref != "develop" {
		t.Errorf("Ref = %q, want %q", out.Ref, "develop")
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithPipelines verifies the behavior of format list markdown with pipelines.
func TestFormatListMarkdown_WithPipelines(t *testing.T) {
	out := ListOutput{
		Pipelines: []Output{
			{ID: 1, Status: "success", Source: "push", Ref: "main", SHA: "abc123def456", WebURL: "https://gitlab.example.com/-/pipelines/1"},
			{ID: 2, Status: "failed", Source: "web", Ref: "develop", SHA: "short", WebURL: "https://gitlab.example.com/-/pipelines/2"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	md := FormatListMarkdown(out)

	for _, want := range []string{
		"## Pipelines (2)",
		"| ID |",
		"[#1]",
		"[#2]",
		"abc123de", // SHA truncated to 8 chars
		"short",    // SHA shorter than 8 kept as-is
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatListMarkdown_Empty verifies the behavior of format list markdown empty.
func TestFormatListMarkdown_Empty(t *testing.T) {
	out := ListOutput{
		Pipelines:  []Output{},
		Pagination: toolutil.PaginationOutput{},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "No pipelines found") {
		t.Errorf("expected 'No pipelines found' in markdown:\n%s", md)
	}
	if strings.Contains(md, "| ID |") {
		t.Error("should not contain table header when empty")
	}
}

// TestFormatListMarkdown_ClickablePipelineLinks verifies that pipeline IDs
// in the list are rendered as clickable Markdown links [#ID](weburl).
func TestFormatListMarkdown_ClickablePipelineLinks(t *testing.T) {
	out := ListOutput{
		Pipelines: []Output{
			{ID: 42, Status: "success", Source: "push", Ref: "main", SHA: "abc12345",
				WebURL: "https://gitlab.example.com/-/pipelines/42"},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	}
	md := FormatListMarkdown(out)
	if !strings.Contains(md, "[#42](https://gitlab.example.com/-/pipelines/42)") {
		t.Errorf("expected clickable pipeline link, got:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// FormatDetailMarkdown
// ---------------------------------------------------------------------------.

// TestFormatDetailMarkdown_Full verifies the behavior of format detail markdown full.
func TestFormatDetailMarkdown_Full(t *testing.T) {
	out := DetailOutput{
		ID:             99,
		IID:            99,
		ProjectID:      42,
		Status:         "success",
		Source:         "push",
		Ref:            "main",
		SHA:            "abc123",
		BeforeSHA:      "000",
		Name:           "Full Pipeline",
		Tag:            true,
		YamlErrors:     "some error",
		Duration:       300,
		QueuedDuration: 15,
		Coverage:       "92.5",
		DetailedStatus: &StatusOutput{
			Icon: "status_success", Text: "passed", Label: "passed",
			Group: "success", Tooltip: "passed", HasDetails: true,
		},
		WebURL:       "https://gitlab.example.com/-/pipelines/99",
		CreatedAt:    "2026-03-01T10:00:00Z",
		UpdatedAt:    "2026-03-01T10:05:00Z",
		StartedAt:    "2026-03-01T10:00:05Z",
		FinishedAt:   "2026-03-01T10:05:00Z",
		CommittedAt:  "2026-02-28T09:00:00Z",
		UserUsername: "testuser",
	}
	md := FormatDetailMarkdown(out)

	for _, want := range []string{
		"Pipeline #99",
		"success",
		"**Source**: push",
		"**Ref**: main (tag: true)",
		"**SHA**: abc123",
		"**Before SHA**: 000",
		"**Name**: Full Pipeline",
		"**Duration**: 300s",
		"**Queued**: 15s",
		"**Coverage**: 92.5%",
		"**YAML Errors**: some error",
		"**User**: testuser",
		"**URL**:",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatDetailMarkdown_Minimal verifies the behavior of format detail markdown minimal.
func TestFormatDetailMarkdown_Minimal(t *testing.T) {
	out := DetailOutput{
		ID:     1,
		Status: "pending",
		Source: "web",
		Ref:    "dev",
		SHA:    "xyz",
		WebURL: "https://gitlab.example.com/-/pipelines/1",
	}
	md := FormatDetailMarkdown(out)

	if !strings.Contains(md, "Pipeline #1") {
		t.Errorf("missing header:\n%s", md)
	}
	for _, absent := range []string{
		"**Before SHA**",
		"**Name**",
		"**Duration**",
		"**Queued**",
		"**Coverage**",
		"**YAML Errors**",
		"**User**",
	} {
		if strings.Contains(md, absent) {
			t.Errorf("should not contain %q for minimal output:\n%s", absent, md)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatVariablesMarkdown
// ---------------------------------------------------------------------------.

// TestFormatVariablesMarkdown_WithData verifies the behavior of format variables markdown with data.
func TestFormatVariablesMarkdown_WithData(t *testing.T) {
	out := VariablesOutput{
		Variables: []VariableOutput{
			{Key: "CI_VAR", Value: "hello", VariableType: "env_var"},
			{Key: "SECRET_FILE", Value: "/tmp/secret", VariableType: "file"},
		},
	}
	md := FormatVariablesMarkdown(out)

	for _, want := range []string{
		"## Pipeline Variables (2)",
		"| Key |",
		"CI_VAR",
		"SECRET_FILE",
		"env_var",
		"file",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatVariablesMarkdown_Empty verifies the behavior of format variables markdown empty.
func TestFormatVariablesMarkdown_Empty(t *testing.T) {
	out := VariablesOutput{Variables: nil}
	md := FormatVariablesMarkdown(out)

	if !strings.Contains(md, "## Pipeline Variables (0)") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "No pipeline variables found") {
		t.Errorf("expected 'No pipeline variables found' in markdown:\n%s", md)
	}
	if strings.Contains(md, "| Key |") {
		t.Error("should not contain table header when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatTestReportMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTestReportMarkdown_WithSuites verifies the behavior of format test report markdown with suites.
func TestFormatTestReportMarkdown_WithSuites(t *testing.T) {
	out := TestReportOutput{
		TotalTime:    120.5,
		TotalCount:   10,
		SuccessCount: 8,
		FailedCount:  1,
		SkippedCount: 1,
		ErrorCount:   0,
		TestSuites: []TestSuiteOutput{
			{Name: "Unit Tests", TotalTime: 60.0, TotalCount: 5, SuccessCount: 4, FailedCount: 1, SkippedCount: 0, ErrorCount: 0},
			{Name: "Integration", TotalTime: 60.5, TotalCount: 5, SuccessCount: 4, FailedCount: 0, SkippedCount: 1, ErrorCount: 0},
		},
	}
	md := FormatTestReportMarkdown(out)

	for _, want := range []string{
		"## Pipeline Test Report",
		"**Total**: 10 tests",
		"120.50s",
		"**Passed**: 8",
		"**Failed**: 1",
		"**Skipped**: 1",
		"**Errors**: 0",
		"### Test Suites",
		"| Suite |",
		"Unit Tests",
		"Integration",
		"60.00s",
		"60.50s",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatTestReportMarkdown_Empty verifies the behavior of format test report markdown empty.
func TestFormatTestReportMarkdown_Empty(t *testing.T) {
	out := TestReportOutput{}
	md := FormatTestReportMarkdown(out)

	if !strings.Contains(md, "## Pipeline Test Report") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "**Total**: 0 tests") {
		t.Errorf("expected zero counts:\n%s", md)
	}
	if strings.Contains(md, "### Test Suites") {
		t.Error("should not contain suites section when empty")
	}
}

// ---------------------------------------------------------------------------
// FormatTestReportSummaryMarkdown
// ---------------------------------------------------------------------------.

// TestFormatTestReportSummaryMarkdown_WithSuites verifies the behavior of format test report summary markdown with suites.
func TestFormatTestReportSummaryMarkdown_WithSuites(t *testing.T) {
	out := TestReportSummaryOutput{
		TotalTime:    200.0,
		TotalCount:   20,
		SuccessCount: 18,
		FailedCount:  1,
		SkippedCount: 1,
		ErrorCount:   0,
		TestSuites: []TestSuiteSummaryOutput{
			{Name: "Unit", TotalTime: 100.0, TotalCount: 10, SuccessCount: 9, FailedCount: 1, SkippedCount: 0, ErrorCount: 0, BuildIDs: []int64{101, 102}},
		},
	}
	md := FormatTestReportSummaryMarkdown(out)

	for _, want := range []string{
		"## Pipeline Test Report Summary",
		"**Total**: 20 tests",
		"200.00s",
		"**Passed**: 18",
		"### Test Suites",
		"Unit",
		"100.00s",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

// TestFormatTestReportSummaryMarkdown_Empty verifies the behavior of format test report summary markdown empty.
func TestFormatTestReportSummaryMarkdown_Empty(t *testing.T) {
	out := TestReportSummaryOutput{}
	md := FormatTestReportSummaryMarkdown(out)

	if !strings.Contains(md, "## Pipeline Test Report Summary") {
		t.Errorf("missing header:\n%s", md)
	}
	if !strings.Contains(md, "**Total**: 0 tests") {
		t.Errorf("expected zero counts:\n%s", md)
	}
	if strings.Contains(md, "### Test Suites") {
		t.Error("should not contain suites section when empty")
	}
}

// ---------------------------------------------------------------------------
// RegisterTools — no panic
// ---------------------------------------------------------------------------.

// TestRegisterTools_NoPanic verifies the behavior of register tools no panic.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
}

// ---------------------------------------------------------------------------
// RegisterToolsCallAllThroughMCP — full MCP roundtrip for all 11 tools
// ---------------------------------------------------------------------------.

// TestRegisterTools_CallAllThroughMCP validates register tools call all through m c p across multiple scenarios using table-driven subtests.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newPipelinesMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_pipeline_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_pipeline_get", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"cancel", "gitlab_pipeline_cancel", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"retry", "gitlab_pipeline_retry", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"delete", "gitlab_pipeline_delete", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"variables", "gitlab_pipeline_variables", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"test_report", "gitlab_pipeline_test_report", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"test_report_summary", "gitlab_pipeline_test_report_summary", map[string]any{"project_id": "1", "pipeline_id": 1}},
		{"latest", "gitlab_pipeline_latest", map[string]any{"project_id": "1"}},
		{"create", "gitlab_pipeline_create", map[string]any{"project_id": "1", "ref": "main"}},
		{"update_metadata", "gitlab_pipeline_update_metadata", map[string]any{"project_id": "1", "pipeline_id": 1, "name": "new"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, err)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// newPipelinesMCPSession is an internal helper for the pipelines package.
func newPipelinesMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	pipelineJSON := `{"id":1,"iid":1,"project_id":1,"status":"success","source":"push","ref":"main","sha":"abc123","name":"Pipeline","duration":120,"queued_duration":5,"web_url":"https://gitlab.example.com/-/pipelines/1","created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:05:00Z"}`

	handler := http.NewServeMux()

	// List pipelines
	handler.HandleFunc("GET /api/v4/projects/1/pipelines", func(w http.ResponseWriter, r *http.Request) {
		// Distinguish "latest" subpath
		if strings.HasSuffix(r.URL.Path, "/pipelines/latest") {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[`+pipelineJSON+`]`)
	})

	// Latest pipeline (specific path registered before generic /pipelines/{id})
	handler.HandleFunc("GET /api/v4/projects/1/pipelines/latest", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineJSON)
	})

	// Get pipeline
	handler.HandleFunc("GET /api/v4/projects/1/pipelines/1", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineJSON)
	})

	// Cancel pipeline
	handler.HandleFunc("POST /api/v4/projects/1/pipelines/1/cancel", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineJSON)
	})

	// Retry pipeline
	handler.HandleFunc("POST /api/v4/projects/1/pipelines/1/retry", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineJSON)
	})

	// Delete pipeline
	handler.HandleFunc("DELETE /api/v4/projects/1/pipelines/1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Pipeline variables
	handler.HandleFunc("GET /api/v4/projects/1/pipelines/1/variables", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"CI_VAR","value":"hello","variable_type":"env_var"}]`)
	})

	// Pipeline test report
	handler.HandleFunc("GET /api/v4/projects/1/pipelines/1/test_report", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"total_time":10.0,"total_count":5,"success_count":5,"failed_count":0,"skipped_count":0,"error_count":0,"test_suites":[]}`)
	})

	// Pipeline test report summary
	handler.HandleFunc("GET /api/v4/projects/1/pipelines/1/test_report_summary", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"total":{"time":10.0,"count":5,"success":5,"failed":0,"skipped":0,"error":0},"test_suites":[]}`)
	})

	// Create pipeline
	handler.HandleFunc("POST /api/v4/projects/1/pipeline", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, pipelineJSON)
	})

	// Update pipeline metadata
	handler.HandleFunc("PUT /api/v4/projects/1/pipelines/1/metadata", func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineJSON)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestPipelineCancel_403Hint verifies that Cancel returns a WrapErrWithHint
// error when the API returns 403 Forbidden.
func TestPipelineCancel_403Hint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Cancel(context.Background(), client, ActionInput{ProjectID: "1", PipelineID: 1})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "pipeline may have already completed") {
		t.Errorf("error should contain hint, got: %v", err)
	}
}

// TestPipelineCancel_Non403Error verifies that Cancel returns a
// WrapErrWithMessage for non-403 API errors.
func TestPipelineCancel_Non403Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"server error"}`)
	}))
	_, err := Cancel(context.Background(), client, ActionInput{ProjectID: "1", PipelineID: 1})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if strings.Contains(err.Error(), "pipeline may have already completed") {
		t.Error("non-403 error should not contain 403-specific hint")
	}
}

// TestPipelineRetry_403Hint verifies that Retry returns a WrapErrWithHint
// error when the API returns 403 Forbidden.
func TestPipelineRetry_403Hint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "1", PipelineID: 1})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "pipeline may still be running") {
		t.Errorf("error should contain hint, got: %v", err)
	}
}

// TestPipelineRetry_Non403Error verifies that Retry returns a
// WrapErrWithMessage for non-403 API errors.
func TestPipelineRetry_Non403Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"server error"}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "1", PipelineID: 1})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if strings.Contains(err.Error(), "pipeline may still be running") {
		t.Error("non-403 error should not contain 403-specific hint")
	}
}

// TestPipelineCreate_400Hint verifies that Create returns a WrapErrWithHint
// error when the API returns 400 Bad Request.
func TestPipelineCreate_400Hint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad Request"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Ref: "main"})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "gitlab-ci.yml") {
		t.Errorf("error should contain hint, got: %v", err)
	}
}

// TestGetLatest_NonForbiddenError verifies that GetLatest returns a wrapped
// error (not the fallback path) for non-403 API errors.
func TestGetLatest_NonForbiddenError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"server error"}`)
	}))
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestGetLatest_FallbackListError verifies the fallback path when the
// ListProjectPipelines call fails (e.g. connection refused).
func TestGetLatest_FallbackListError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "latest") {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		// Force a connection-level error by hijacking and closing
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	})
	client := testutil.NewTestClient(t, mux)
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error from fallback list")
	}
}

// TestGetLatest_FallbackEmptyList verifies the fallback path returns an error
// when the list endpoint returns an empty array (no pipelines).
func TestGetLatest_FallbackEmptyList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "latest") {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error for empty fallback list")
	}
	if !strings.Contains(err.Error(), "no pipelines found") {
		t.Errorf("error should mention no pipelines, got: %v", err)
	}
}

// TestGetLatest_FallbackGetPipelineError verifies the fallback path returns an
// error when the list succeeds but the subsequent GetPipeline call fails.
func TestGetLatest_FallbackGetPipelineError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "latest") {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pipelines") || strings.Contains(r.URL.RawQuery, "sort") {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":99}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	_, err := GetLatest(context.Background(), client, GetLatestInput{ProjectID: "1"})
	if err == nil {
		t.Fatal("expected error from fallback GetPipeline")
	}
}

// TestRegisterTools_Get404NotFound verifies the get handler returns a
// NotFoundResult when the API returns 404.
func TestRegisterTools_Get404NotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_pipeline_get",
		Arguments: map[string]any{"project_id": "1", "pipeline_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected IsError result for 404")
	}
}

// TestRegisterTools_DeleteConfirmDeclined verifies the delete handler returns
// early when the user declines the confirmation prompt.
func TestRegisterTools_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.NewServeMux())
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_pipeline_delete",
		Arguments: map[string]any{"project_id": "1", "pipeline_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined delete")
	}
}

// TestRegisterTools_DeleteAPIError verifies the delete handler returns
// an error when the delete API call fails.
func TestRegisterTools_DeleteAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_pipeline_delete",
		Arguments: map[string]any{"project_id": "1", "pipeline_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatal("expected error result from delete API failure")
	}
}

// TestPipelineGet_EmbedsCanonicalResource asserts gitlab_pipeline_get
// attaches an EmbeddedResource block with URI
// gitlab://project/{id}/pipeline/{pipeline_id}.
func TestPipelineGet_EmbedsCanonicalResource(t *testing.T) {
	const respJSON = `{"id":100,"project_id":42,"status":"success","ref":"main","sha":"abc","web_url":"https://gitlab.example.com/g/p/-/pipelines/100"}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/pipelines/100") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	session, ctx := testutil.NewEmbedTestSession(t, handler, RegisterTools)
	args := map[string]any{"project_id": "42", "pipeline_id": 100}
	testutil.AssertEmbeddedResource(t, ctx, session, "gitlab_pipeline_get", args, "gitlab://project/42/pipeline/100", toolutil.EnableEmbeddedResources)
}
