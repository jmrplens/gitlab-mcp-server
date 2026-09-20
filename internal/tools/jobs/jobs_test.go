// jobs_test.go contains unit tests for GitLab CI/CD job operations
// (list, get, trace, cancel, retry). Tests use httptest to mock the
// GitLab Jobs API and verify both success and error paths.
package jobs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestRawHelpers_UnescapablePath_FailBeforeTheRequest verifies both raw
// helpers refuse a path the client cannot unescape, and never reach the
// server with it. The public handlers escape every identifier before building
// a path, so the branch is reached by calling the helpers directly.
func TestRawHelpers_UnescapablePath_FailBeforeTheRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler called, want the request to fail while it is built")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	const badPath = "projects/%zz/jobs"

	t.Run("list", func(t *testing.T) {
		jobs, resp, err := rawListJobs(t.Context(), client, badPath, nil)
		if err == nil || jobs != nil || resp != nil {
			t.Fatalf("rawListJobs = %v/%v/%v, want only an error", jobs, resp, err)
		}
	})
	t.Run("get", func(t *testing.T) {
		job, resp, err := rawGetJob(t.Context(), client, badPath)
		if err == nil || job != nil || resp != nil {
			t.Fatalf("rawGetJob = %v/%v/%v, want only an error", job, resp, err)
		}
	})
}

// TestJobTrace_ReadFailure_IsReported verifies a trace body that fails
// mid-read with something other than an end of file is reported as an error
// rather than returned as a shorter log. client-go hands Trace an in-memory
// reader that cannot fail that way, so the failure comes through the seam.
func TestJobTrace_ReadFailure_IsReported(t *testing.T) {
	original := readFull
	t.Cleanup(func() { readFull = original })
	readFull = func(io.Reader, []byte) (int, error) { return 3, errors.New("stream reset") }

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathJobTrace {
			http.NotFound(w, r)
			return
		}
		w.Header().Set(testHeaderContentType, "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial log"))
	}))

	_, err := Trace(t.Context(), client, TraceInput{ProjectID: "42", JobID: 100})
	if err == nil || !strings.Contains(err.Error(), "stream reset") {
		t.Fatalf("Trace() error = %v, want the read failure", err)
	}
}

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
	if out.Jobs[0].Pipeline == nil || out.Jobs[0].Pipeline.ID != 10 {
		t.Errorf("Jobs[0].Pipeline = %+v, want id 10", out.Jobs[0].Pipeline)
	}
	if out.Jobs[0].User == nil || out.Jobs[0].User.Username != "testuser" {
		t.Errorf("Jobs[0].User = %+v, want username testuser", out.Jobs[0].User)
	}
	if out.Jobs[0].Runner == nil || out.Jobs[0].Runner.ID != 1 {
		t.Errorf("Jobs[0].Runner = %+v, want ID 1", out.Jobs[0].Runner)
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

// jobDistinctJSON is a job in which no two values agree, which is the fixture a
// converter swap cannot survive: the shared fixtures name the job and its stage
// "build" alike, so ToOutput reading the stage into the name passed every test.
// Every documented sub-object is present, each with values of its own.
const jobDistinctJSON = `{
	"id":7001,
	"name":"compile",
	"stage":"assemble",
	"status":"failed",
	"ref":"feature/x",
	"tag":true,
	"allow_failure":false,
	"duration":12.5,
	"queued_duration":3.25,
	"failure_reason":"script_failure",
	"web_url":"https://gitlab.example.com/g/p/-/jobs/7001",
	"created_at":"2026-03-01T10:00:00Z",
	"started_at":"2026-03-01T10:01:00Z",
	"finished_at":"2026-03-01T10:02:00Z",
	"artifacts_expire_at":"2026-04-01T00:00:00Z",
	"coverage":81.5,
	"tag_list":["docker","linux"],
	"erased_at":"2026-03-05T09:00:00Z",
	"archived":true,
	"commit":{
		"id":"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678",
		"short_id":"a1b2c3d4",
		"title":"Fix the build",
		"author_name":"Ada Lovelace",
		"author_email":"ada@example.com",
		"created_at":"2026-02-28T18:30:00Z",
		"message":"Fix the build\n\nLonger body."
	},
	"pipeline":{"id":901,"project_id":4242,"ref":"release/1.0","sha":"0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c","status":"running"},
	"project":{"ci_job_token_scope_enabled":true},
	"runner":{
		"id":32,
		"description":"shared-runner",
		"ip_address":"10.0.0.7",
		"active":true,
		"paused":false,
		"is_shared":false,
		"runner_type":"instance_type",
		"name":"gitlab-runner",
		"online":false,
		"status":"offline"
	},
	"runner_manager":{
		"id":11,
		"system_id":"s_89e5e9956577",
		"version":"16.11.1",
		"revision":"535ced5f",
		"platform":"linux",
		"architecture":"amd64",
		"created_at":"2024-05-01T10:12:02.507Z",
		"contacted_at":"2024-05-07T06:30:09.355Z",
		"ip_address":"127.0.0.1",
		"status":"online"
	},
	"user":{
		"id":5,
		"name":"Grace Hopper",
		"username":"grace",
		"state":"active",
		"avatar_url":"https://gitlab.example.com/uploads/user/avatar/5/grace.png",
		"web_url":"https://gitlab.example.com/grace",
		"created_at":"2020-01-02T03:04:05Z",
		"bio":"Compiler pioneer",
		"location":"Arlington",
		"public_email":"grace@example.com",
		"linkedin":"grace-hopper",
		"twitter":"gracehopper",
		"website_url":"https://grace.example.com",
		"organization":"US Navy"
	},
	"artifacts":[{"file_type":"archive","filename":"artifacts.zip","size":2048,"file_format":"zip"}],
	"artifacts_file":{"filename":"artifacts.zip","size":2048}
}`

// jobDistinctOutput is what [jobDistinctJSON] must arrive as, field for field.
func jobDistinctOutput() Output {
	return Output{
		ID: 7001, Name: "compile", Stage: "assemble", Status: "failed", Ref: "feature/x",
		Tag: true, AllowFailure: false,
		Duration: 12.5, QueuedDuration: 3.25,
		FailureReason:     "script_failure",
		WebURL:            "https://gitlab.example.com/g/p/-/jobs/7001",
		CreatedAt:         "2026-03-01T10:00:00Z",
		StartedAt:         "2026-03-01T10:01:00Z",
		FinishedAt:        "2026-03-01T10:02:00Z",
		ArtifactsExpireAt: "2026-04-01T00:00:00Z",
		Coverage:          81.5,
		TagList:           []string{"docker", "linux"},
		ErasedAt:          "2026-03-05T09:00:00Z",
		Archived:          true,
		Commit: &CommitObject{
			ID: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", ShortID: "a1b2c3d4", Title: "Fix the build",
			AuthorName: "Ada Lovelace", AuthorEmail: "ada@example.com",
			CreatedAt: "2026-02-28T18:30:00Z", Message: "Fix the build\n\nLonger body.",
		},
		Pipeline: &PipelineObject{ID: 901, ProjectID: 4242, Ref: "release/1.0", SHA: "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c", Status: "running"},
		Project:  &ProjectObject{CIJobTokenScopeEnabled: true},
		Runner: &RunnerObject{
			ID: 32, Description: "shared-runner", IPAddress: "10.0.0.7", Active: true,
			RunnerType: "instance_type", Name: "gitlab-runner", Status: "offline",
		},
		RunnerManager: &RunnerManagerObject{
			ID: 11, SystemID: "s_89e5e9956577", Version: "16.11.1", Revision: "535ced5f",
			Platform: "linux", Architecture: "amd64",
			CreatedAt: "2024-05-01T10:12:02.507Z", ContactedAt: "2024-05-07T06:30:09.355Z",
			IPAddress: "127.0.0.1", Status: "online",
		},
		User: &UserObject{
			ID: 5, Name: "Grace Hopper", Username: "grace", State: "active",
			AvatarURL: "https://gitlab.example.com/uploads/user/avatar/5/grace.png",
			WebURL:    "https://gitlab.example.com/grace",
			CreatedAt: "2020-01-02T03:04:05Z", Bio: "Compiler pioneer", Location: "Arlington",
			PublicEmail: "grace@example.com", Linkedin: "grace-hopper", Twitter: "gracehopper",
			WebsiteURL: "https://grace.example.com", Organization: "US Navy",
		},
		Artifacts:     []ArtifactObject{{FileType: "archive", Filename: "artifacts.zip", Size: 2048, FileFormat: "zip"}},
		ArtifactsFile: &ArtifactsFileObject{Filename: "artifacts.zip", Size: 2048},
	}
}

// renderForDiff prints an output as indented JSON for a mismatch report, since
// %+v prints the sub-objects as addresses.
func renderForDiff(t *testing.T, v any) string {
	t.Helper()
	rendered, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Errorf("render %T for the report: %v", v, err)
	}
	return string(rendered)
}

// assertJobOutput compares a job field for field and prints both sides as JSON
// on a mismatch.
func assertJobOutput(t *testing.T, got, want Output) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("job output differs\n got %s\nwant %s", renderForDiff(t, got), renderForDiff(t, want))
	}
}

// TestJobGet_MapsEveryDocumentedField holds the whole job, sub-objects
// included, to a fixture where no two values agree. A swap of two assignments
// in a converter has no branch either gate can flip, and the shared fixtures
// hid one by giving the name and the stage the same value.
func TestJobGet_MapsEveryDocumentedField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/7001" {
			testutil.RespondJSON(w, http.StatusOK, jobDistinctJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 7001})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertJobOutput(t, out, jobDistinctOutput())
}

// TestJobGet_Flags_EachSurfacedOnItsOwnKey drives the job's and the runner's
// booleans one at a time: a struct of flags has no fixture where no two values
// agree, so a fixture setting them all cannot tell a swap apart. Each case
// sends one flag and expects exactly that flag back.
func TestJobGet_Flags_EachSurfacedOnItsOwnKey(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Output
	}{
		{"tag", `{"id":1,"tag":true}`, Output{ID: 1, Tag: true}},
		{"allow_failure", `{"id":1,"allow_failure":true}`, Output{ID: 1, AllowFailure: true}},
		{"archived", `{"id":1,"archived":true}`, Output{ID: 1, Archived: true}},
		{"runner.active", `{"id":1,"runner":{"id":2,"active":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, Active: true}}},
		{"runner.paused", `{"id":1,"runner":{"id":2,"paused":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, Paused: true}}},
		{"runner.is_shared", `{"id":1,"runner":{"id":2,"is_shared":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, IsShared: true}}},
		{"runner.online", `{"id":1,"runner":{"id":2,"online":true}}`, Output{ID: 1, Runner: &RunnerObject{ID: 2, Online: true}}},
		{"project.ci_job_token_scope_enabled", `{"id":1,"project":{"ci_job_token_scope_enabled":true}}`, Output{ID: 1, Project: &ProjectObject{CIJobTokenScopeEnabled: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/1" {
					testutil.RespondJSON(w, http.StatusOK, tt.body)
					return
				}
				http.NotFound(w, r)
			}))
			out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 1})
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			assertJobOutput(t, out, tt.want)
		})
	}
}

// jobFullJSON is a single-job fixture that includes the documented fields
// gl.Job does not expose: top-level archived, the runner_manager object, and
// the full runner object (with ip_address, paused, runner_type, online,
// status). Used to assert the raw-fetch handlers surface these 1:1.
const jobFullJSON = `{
	"id":100,
	"name":"build",
	"stage":"build",
	"status":"failed",
	"ref":"main",
	"tag":false,
	"archived":true,
	"runner":{
		"id":32,
		"description":"shared-runner",
		"ip_address":null,
		"active":true,
		"paused":false,
		"is_shared":true,
		"runner_type":"instance_type",
		"name":null,
		"online":false,
		"status":"offline"
	},
	"runner_manager":{
		"id":1,
		"system_id":"s_89e5e9956577",
		"version":"16.11.1",
		"revision":"535ced5f",
		"platform":"linux",
		"architecture":"amd64",
		"created_at":"2024-05-01T10:12:02.507Z",
		"contacted_at":"2024-05-07T06:30:09.355Z",
		"ip_address":"127.0.0.1",
		"status":"offline"
	}
}`

// assertJobFullFields asserts that the raw-fetched documented fields surfaced.
func assertJobFullFields(t *testing.T, out Output) {
	t.Helper()
	if !out.Archived {
		t.Error("Archived = false, want true")
	}
	if out.Runner == nil {
		t.Fatal("Runner = nil, want populated")
	}
	if out.Runner.RunnerType != "instance_type" {
		t.Errorf("Runner.RunnerType = %q, want %q", out.Runner.RunnerType, "instance_type")
	}
	if out.Runner.Status != "offline" {
		t.Errorf("Runner.Status = %q, want %q", out.Runner.Status, "offline")
	}
	if out.Runner.Paused {
		t.Error("Runner.Paused = true, want false")
	}
	if out.Runner.Online {
		t.Error("Runner.Online = true, want false")
	}
	if out.RunnerManager == nil {
		t.Fatal("RunnerManager = nil, want populated")
	}
	if out.RunnerManager.SystemID != "s_89e5e9956577" {
		t.Errorf("RunnerManager.SystemID = %q, want %q", out.RunnerManager.SystemID, "s_89e5e9956577")
	}
	if out.RunnerManager.Version != "16.11.1" {
		t.Errorf("RunnerManager.Version = %q, want %q", out.RunnerManager.Version, "16.11.1")
	}
	if out.RunnerManager.Platform != "linux" {
		t.Errorf("RunnerManager.Platform = %q, want %q", out.RunnerManager.Platform, "linux")
	}
	if out.RunnerManager.IPAddress != "127.0.0.1" {
		t.Errorf("RunnerManager.IPAddress = %q, want %q", out.RunnerManager.IPAddress, "127.0.0.1")
	}
	if out.RunnerManager.Status != "offline" {
		t.Errorf("RunnerManager.Status = %q, want %q", out.RunnerManager.Status, "offline")
	}
}

// TestJobGet_FullFields verifies that Get surfaces the documented archived,
// runner_manager, and runner-extra fields fetched via the raw REST path.
func TestJobGet_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobGet {
			testutil.RespondJSON(w, http.StatusOK, jobFullJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	assertJobFullFields(t, out)
}

// TestJobList_FullFields verifies that List surfaces the documented archived,
// runner_manager, and runner-extra fields via the raw REST path.
func TestJobList_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobFullJSON))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42", PipelineID: 10})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	assertJobFullFields(t, out.Jobs[0])
}

// TestListProject_FullFields verifies that ListProject surfaces the documented
// archived, runner_manager, and runner-extra fields via the raw REST path.
func TestListProject_FullFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", jobFullJSON))
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
	assertJobFullFields(t, out.Jobs[0])
}

// TestJobGet_OlderInstanceOmitsFields verifies version tolerance: when an older
// GitLab instance returns a job response WITHOUT archived or runner_manager,
// the raw-fetch handler still succeeds and simply omits those fields (they
// stay zero-valued / nil) rather than failing.
func TestJobGet_OlderInstanceOmitsFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathJobGet {
			// jobJSON has no archived/runner_manager and a minimal runner.
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.ID != 100 {
		t.Errorf("out.ID = %d, want 100", out.ID)
	}
	if out.Archived {
		t.Error("Archived = true, want false (field absent on older instance)")
	}
	if out.RunnerManager != nil {
		t.Errorf("RunnerManager = %+v, want nil (field absent on older instance)", out.RunnerManager)
	}
	if out.Runner == nil || out.Runner.ID != 1 {
		t.Errorf("Runner = %+v, want minimal runner with ID 1", out.Runner)
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

// hundredKiB is the limit the trace card's note promises a reader, written as
// its own number so no test here can follow maxTraceBytes wherever it goes.
const hundredKiB = 102400

// TestJobTrace_LimitIsTheHundredKiBTheNotePromises holds the trace limit to
// the figure the truncation note states, from both sides of the boundary: a
// log of exactly that size is returned whole and unmarked, and one byte more
// is cut to it and marked. The truncation tests before this one measured the
// limit against the constant that set it, so a constant that shrank to a
// kilobyte passed them all.
func TestJobTrace_LimitIsTheHundredKiBTheNotePromises(t *testing.T) {
	tests := []struct {
		name          string
		size          int
		wantLen       int
		wantTruncated bool
	}{
		{"exactly the limit is kept whole", hundredKiB, hundredKiB, false},
		{"one byte over is cut to the limit", hundredKiB + 1, hundredKiB, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Repeat("x", tt.size-1) + "$"
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == pathJobTrace {
					w.Header().Set(testHeaderContentType, "text/plain")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(body))
					return
				}
				http.NotFound(w, r)
			}))

			out, err := Trace(context.Background(), client, TraceInput{ProjectID: "42", JobID: 100})
			if err != nil {
				t.Fatalf("Trace() unexpected error: %v", err)
			}
			if out.Truncated != tt.wantTruncated {
				t.Errorf("Truncated = %v, want %v for a %d-byte log", out.Truncated, tt.wantTruncated, tt.size)
			}
			if out.Trace != body[:tt.wantLen] {
				t.Errorf("Trace is %d bytes and ends %q, want the first %d bytes of the log", len(out.Trace), out.Trace[max(0, len(out.Trace)-3):], tt.wantLen)
			}
		})
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
	if out.Bridges[0].DownstreamPipeline == nil || out.Bridges[0].DownstreamPipeline.ID != 50 {
		t.Errorf("Bridges[0].DownstreamPipeline = %+v, want ID 50", out.Bridges[0].DownstreamPipeline)
	}
}

// bridgeDistinctJSON is a bridge in which no two values agree, the pipeline it
// belongs to and the one it triggered included, and the project key the SDK
// does not model beside them.
const bridgeDistinctJSON = `{
	"id":8002,"name":"trigger-child","stage":"fan-out",
	"status":"success","ref":"topic","tag":false,"allow_failure":true,
	"duration":4.5,"queued_duration":1.25,"failure_reason":"","coverage":55.5,
	"web_url":"https://gitlab.example.com/g/p/-/jobs/8002",
	"created_at":"2026-03-02T10:00:00Z","started_at":"2026-03-02T10:01:00Z",
	"finished_at":"2026-03-02T10:02:00Z","erased_at":"2026-03-06T09:00:00Z",
	"commit":{
		"id":"c0ffee0123456789abcdef0123456789abcdef01","short_id":"c0ffee01","title":"Add the child pipeline",
		"author_name":"Linus","author_email":"linus@example.com",
		"created_at":"2026-03-02T09:00:00Z","message":"Add the child pipeline\n"
	},
	"pipeline":{
		"id":902,"project_id":4242,"status":"pending","ref":"main",
		"sha":"9876543210fedcba9876543210fedcba98765432",
		"web_url":"https://gitlab.example.com/g/p/-/pipelines/902",
		"updated_at":"2026-03-02T10:03:00Z","created_at":"2026-03-02T09:59:00Z"
	},
	"user":{"id":6,"name":"Linus","username":"linus","state":"blocked",
		"avatar_url":"https://gitlab.example.com/uploads/user/avatar/6/linus.png",
		"web_url":"https://gitlab.example.com/linus"},
	"downstream_pipeline":{
		"id":903,"project_id":4343,"status":"created","ref":"child",
		"sha":"1357913579135791357913579135791357913579",
		"web_url":"https://gitlab.example.com/g/child/-/pipelines/903",
		"updated_at":"2026-03-02T10:04:00Z","created_at":"2026-03-02T10:01:30Z"
	},
	"project":{"ci_job_token_scope_enabled":true}
}`

// TestListBridges_MapsEveryDocumentedField holds the whole bridge to a fixture
// where no two values agree, the way [TestJobGet_MapsEveryDocumentedField]
// holds the job: BridgeToOutput is a converter of the same shape, and its
// pipeline and downstream pipeline are the one pair most alike.
func TestListBridges_MapsEveryDocumentedField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/pipelines/10/bridges" {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeDistinctJSON))
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
	want := BridgeOutput{
		ID: 8002, Name: "trigger-child", Stage: "fan-out", Status: "success", Ref: "topic",
		Tag: false, AllowFailure: true,
		Duration: 4.5, QueuedDuration: 1.25, Coverage: 55.5,
		WebURL:     "https://gitlab.example.com/g/p/-/jobs/8002",
		CreatedAt:  "2026-03-02T10:00:00Z",
		StartedAt:  "2026-03-02T10:01:00Z",
		FinishedAt: "2026-03-02T10:02:00Z",
		ErasedAt:   "2026-03-06T09:00:00Z",
		Commit: &CommitObject{
			ID: "c0ffee0123456789abcdef0123456789abcdef01", ShortID: "c0ffee01", Title: "Add the child pipeline",
			AuthorName: "Linus", AuthorEmail: "linus@example.com",
			CreatedAt: "2026-03-02T09:00:00Z", Message: "Add the child pipeline\n",
		},
		Pipeline: &PipelineInfoObject{
			ID: 902, ProjectID: 4242, Status: "pending", Ref: "main",
			SHA:       "9876543210fedcba9876543210fedcba98765432",
			WebURL:    "https://gitlab.example.com/g/p/-/pipelines/902",
			UpdatedAt: "2026-03-02T10:03:00Z", CreatedAt: "2026-03-02T09:59:00Z",
		},
		User: &UserObject{
			ID: 6, Name: "Linus", Username: "linus", State: "blocked",
			AvatarURL: "https://gitlab.example.com/uploads/user/avatar/6/linus.png",
			WebURL:    "https://gitlab.example.com/linus",
		},
		DownstreamPipeline: &PipelineInfoObject{
			ID: 903, ProjectID: 4343, Status: "created", Ref: "child",
			SHA:       "1357913579135791357913579135791357913579",
			WebURL:    "https://gitlab.example.com/g/child/-/pipelines/903",
			UpdatedAt: "2026-03-02T10:04:00Z", CreatedAt: "2026-03-02T10:01:30Z",
		},
		Project: &ProjectObject{CIJobTokenScopeEnabled: true},
	}
	if got := out.Bridges[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("bridge output differs\n got %s\nwant %s", renderForDiff(t, got), renderForDiff(t, want))
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

// oneMiB is the limit the artifact cards' warning names, written as its own
// number so no test here can follow maxArtifactBytes wherever it goes.
const oneMiB = 1048576

// artifactLimitCases are the two sides of the artifact limit: exactly the
// limit is kept whole and unmarked, one byte more is cut to it and marked.
func artifactLimitCases() []struct {
	name          string
	size          int
	wantTruncated bool
} {
	return []struct {
		name          string
		size          int
		wantTruncated bool
	}{
		{"exactly the limit is kept whole", oneMiB, false},
		{"one byte over is cut to the limit", oneMiB + 1, true},
	}
}

// TestGetArtifacts_LimitIsTheMebibyteTheCardPromises holds the archive limit
// to the figure the card's warning states, from both sides of the boundary.
// The truncation tests before it measured the limit against the constant that
// set it, so a constant that shrank passed them.
func TestGetArtifacts_LimitIsTheMebibyteTheCardPromises(t *testing.T) {
	for _, tt := range artifactLimitCases() {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Repeat("x", tt.size-1) + "$"
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == pathJobArtifacts {
					w.Header().Set(testHeaderContentType, "application/zip")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(body))
					return
				}
				http.NotFound(w, r)
			}))

			out, err := GetArtifacts(context.Background(), client, GetInput{ProjectID: "42", JobID: 100})
			if err != nil {
				t.Fatalf("GetArtifacts() unexpected error: %v", err)
			}
			if out.Truncated != tt.wantTruncated || out.Size != oneMiB {
				t.Errorf("Truncated/Size = %v/%d, want %v/%d for a %d-byte archive", out.Truncated, out.Size, tt.wantTruncated, oneMiB, tt.size)
			}
			decoded, decodeErr := base64.StdEncoding.DecodeString(out.Content)
			if decodeErr != nil || string(decoded) != body[:oneMiB] {
				t.Errorf("Content decodes to %d bytes (err %v), want the first %d bytes of the archive", len(decoded), decodeErr, oneMiB)
			}
		})
	}
}

// TestDownloadSingleArtifact_LimitIsTheMebibyteTheCardPromises is the same
// boundary on the single-file path, which reads through its own copy of the
// truncation.
func TestDownloadSingleArtifact_LimitIsTheMebibyteTheCardPromises(t *testing.T) {
	for _, tt := range artifactLimitCases() {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Repeat("x", tt.size-1) + "$"
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/100/artifacts/big.bin" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(body))
					return
				}
				http.NotFound(w, r)
			}))

			out, err := DownloadSingleArtifact(context.Background(), client, SingleArtifactInput{ProjectID: "42", JobID: 100, ArtifactPath: "big.bin"})
			if err != nil {
				t.Fatalf("DownloadSingleArtifact() unexpected error: %v", err)
			}
			if out.Truncated != tt.wantTruncated || out.Size != oneMiB {
				t.Errorf("Truncated/Size = %v/%d, want %v/%d for a %d-byte file", out.Truncated, out.Size, tt.wantTruncated, oneMiB, tt.size)
			}
			if out.Content != body[:oneMiB] {
				t.Errorf("Content is %d bytes, want the first %d bytes of the file", len(out.Content), oneMiB)
			}
		})
	}
}

// TestDownloadArtifacts_Success verifies DownloadArtifacts reaches the ref's
// download endpoint with the job name the caller gave as its job parameter,
// which is what selects the job whose artifacts come back.
func TestDownloadArtifacts_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/artifacts/main/download" {
			if got := r.URL.Query().Get("job"); got != "build" {
				t.Errorf("job query = %q, want build", got)
			}
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

// TestDownloadSingleArtifactByRef_Success verifies DownloadSingleArtifactByRef
// reaches the ref's raw endpoint with the job name as its job parameter.
func TestDownloadSingleArtifactByRef_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/jobs/artifacts/main/raw/"+testReportFileName {
			if got := r.URL.Query().Get("job"); got != "build" {
				t.Errorf("job query = %q, want build", got)
			}
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

// TestProjectIDRequired_Validation ensures every handler refuses an empty
// project_id before reaching GitLab. The forbidden mock is what makes the
// assertion about the handler: the tests this replaces answered 404 to
// whatever arrived, so an error came back with the check deleted too.
func TestProjectIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"List", func() error { _, e := List(ctx, client, ListInput{PipelineID: 10}); return e }},
		{"ListProject", func() error { _, e := ListProject(ctx, client, ListProjectInput{}); return e }},
		{"ListBridges", func() error { _, e := ListBridges(ctx, client, BridgeListInput{PipelineID: 10}); return e }},
		{"Get", func() error { _, e := Get(ctx, client, GetInput{JobID: 100}); return e }},
		{"Trace", func() error { _, e := Trace(ctx, client, TraceInput{JobID: 100}); return e }},
		{"Cancel", func() error { _, e := Cancel(ctx, client, CancelInput{JobID: 100}); return e }},
		{"Retry", func() error { _, e := Retry(ctx, client, ActionInput{JobID: 100}); return e }},
		{"GetArtifacts", func() error { _, e := GetArtifacts(ctx, client, GetInput{JobID: 100}); return e }},
		{"DownloadArtifacts", func() error {
			_, e := DownloadArtifacts(ctx, client, DownloadArtifactsInput{RefName: "main", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifact", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{JobID: 100, ArtifactPath: "a.txt"})
			return e
		}},
		{"DownloadSingleArtifactByRef", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{RefName: "main", ArtifactPath: "a.txt", JobName: "build"})
			return e
		}},
		{"Erase", func() error { _, e := Erase(ctx, client, ActionInput{JobID: 100}); return e }},
		{"KeepArtifacts", func() error { _, e := KeepArtifacts(ctx, client, ActionInput{JobID: 100}); return e }},
		{"Play", func() error { _, e := Play(ctx, client, PlayInput{JobID: 100}); return e }},
		{"DeleteArtifacts", func() error { return DeleteArtifacts(ctx, client, DeleteArtifactsInput{JobID: 100}) }},
		{"DeleteProjectArtifacts", func() error { return DeleteProjectArtifacts(ctx, client, DeleteProjectArtifactsInput{}) }},
		{"Wait", func() error { _, e := Wait(ctx, nil, client, WaitInput{JobID: 100}); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), "project_id")
		})
	}
}

// TestStringFieldsRequired_Validation ensures the by-ref and by-path downloads
// refuse a missing ref_name, artifact_path or job by name before reaching
// GitLab, on the same forbidden mock as the project_id table.
func TestStringFieldsRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()
	const pid = "my/project"

	tests := []struct {
		name string
		want string
		fn   func() error
	}{
		{"DownloadArtifacts_ref_name", "ref_name", func() error {
			_, e := DownloadArtifacts(ctx, client, DownloadArtifactsInput{ProjectID: pid, JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifact_artifact_path", "artifact_path", func() error {
			_, e := DownloadSingleArtifact(ctx, client, SingleArtifactInput{ProjectID: pid, JobID: 100})
			return e
		}},
		{"DownloadSingleArtifactByRef_ref_name", "ref_name", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{ProjectID: pid, ArtifactPath: "a.txt", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifactByRef_artifact_path", "artifact_path", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{ProjectID: pid, RefName: "main", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifactByRef_job", "job is required", func() error {
			_, e := DownloadSingleArtifactByRef(ctx, client, SingleArtifactRefInput{ProjectID: pid, RefName: "main", ArtifactPath: "a.txt"})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertContains(t, tt.fn(), tt.want)
		})
	}
}

// TestJobIDRequired_Validation ensures all handlers that require job_id
// reject zero and negative values.
func TestJobIDRequired_Validation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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
		// Wait was tested for a zero job_id against a mock that answered 404,
		// so its check could drop the zero and the mock still produced the error.
		{"Wait_zero", func() error { _, e := Wait(ctx, nil, client, WaitInput{ProjectID: pid, JobID: 0}); return e }},
		{"Wait_negative", func() error { _, e := Wait(ctx, nil, client, WaitInput{ProjectID: pid, JobID: -1}); return e }},
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
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
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

// TestReadHandlers_NotFoundHint_GatedOnTheStatus verifies each read handler
// attaches its own corrective hint to a 404 and to nothing else: a 403 from
// the same endpoint carries the generic message. The status literal in each
// WrapErrWithStatusHint is what decides it, and no gate mutates a literal, so
// a handler that checked the wrong code kept every other test green.
func TestReadHandlers_NotFoundHint_GatedOnTheStatus(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		hint string
		call func(client *gitlabclient.Client) error
	}{
		{"List", "gitlab_pipeline_list", func(c *gitlabclient.Client) error {
			_, e := List(ctx, c, ListInput{ProjectID: "42", PipelineID: 10})
			return e
		}},
		{"ListProject", "gitlab_project_get", func(c *gitlabclient.Client) error {
			_, e := ListProject(ctx, c, ListProjectInput{ProjectID: "42"})
			return e
		}},
		{"ListBridges", "Bridges only exist", func(c *gitlabclient.Client) error {
			_, e := ListBridges(ctx, c, BridgeListInput{ProjectID: "42", PipelineID: 10})
			return e
		}},
		{"Get", "not the per-pipeline index", func(c *gitlabclient.Client) error {
			_, e := Get(ctx, c, GetInput{ProjectID: "42", JobID: 100})
			return e
		}},
		{"Trace", "erased/expired", func(c *gitlabclient.Client) error {
			_, e := Trace(ctx, c, TraceInput{ProjectID: "42", JobID: 100})
			return e
		}},
		{"GetArtifacts", "expire_in", func(c *gitlabclient.Client) error {
			_, e := GetArtifacts(ctx, c, GetInput{ProjectID: "42", JobID: 100})
			return e
		}},
		{"DownloadArtifacts", "non-expired artifacts", func(c *gitlabclient.Client) error {
			_, e := DownloadArtifacts(ctx, c, DownloadArtifactsInput{ProjectID: "42", RefName: "main", JobName: "build"})
			return e
		}},
		{"DownloadSingleArtifact", "gitlab_job_artifacts to list", func(c *gitlabclient.Client) error {
			_, e := DownloadSingleArtifact(ctx, c, SingleArtifactInput{ProjectID: "42", JobID: 100, ArtifactPath: "a.txt"})
			return e
		}},
		{"DownloadSingleArtifactByRef", "produced an artifact at artifact_path", func(c *gitlabclient.Client) error {
			_, e := DownloadSingleArtifactByRef(ctx, c, SingleArtifactRefInput{ProjectID: "42", RefName: "main", ArtifactPath: "a.txt", JobName: "build"})
			return e
		}},
		{"Wait", "deleted or expired during polling", func(c *gitlabclient.Client) error {
			_, e := Wait(ctx, nil, c, WaitInput{ProjectID: "42", JobID: 100, IntervalSeconds: 5, TimeoutSeconds: 30})
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("404 carries the hint", func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
				}))
				assertContains(t, tt.call(client), tt.hint)
			})
			t.Run("403 does not", func(t *testing.T) {
				client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
				}))
				err := tt.call(client)
				if err == nil {
					t.Fatal(errExpectedAPI)
				}
				if strings.Contains(err.Error(), tt.hint) {
					t.Errorf("403 error %q carries the 404 hint %q", err, tt.hint)
				}
			})
		})
	}
}

// assertListQuery checks, inside a mock, that every list parameter a caller
// can set reached the query under GitLab's own spelling; it is called with the
// values the test sent, so a forwarding dropped from one handler is caught by
// that handler's own test rather than by the helper they share.
func assertListQuery(t *testing.T, r *http.Request, want map[string]string) {
	t.Helper()
	q := r.URL.Query()
	for name, value := range want {
		if got := q.Get(name); got != value {
			t.Errorf("query %s = %q, want %q", name, got, value)
		}
	}
}

// TestJobList_ForwardsEveryListParameter verifies List sends the retried flag,
// the offset page, the keyset cursor and the ordering as the caller set them,
// and reads the pagination block off the response it got.
func TestJobList_ForwardsEveryListParameter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathPipelineJobs {
			assertListQuery(t, r, map[string]string{
				"include_retried": "true", "page": "2", "per_page": "5",
				"order_by": "id", "sort": "asc", "pagination": "keyset", "page_token": "tok-list",
			})
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "2", PerPage: "5", Total: "10", TotalPages: "2", PrevPage: "1"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		ProjectID:      "42",
		PipelineID:     10,
		IncludeRetried: true,
		OrderBy:        "id",
		Sort:           "asc",
		Page:           2, PerPage: 5,
		Pagination: "keyset", PageToken: "tok-list",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	wantPage := toolutil.PaginationOutput{Page: 2, PerPage: 5, TotalItems: 10, TotalPages: 2, PrevPage: 1}
	if out.Pagination != wantPage {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, wantPage)
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

// TestJobCancel_APIError verifies a 403 on cancel carries the cancel hint, the
// one naming the role and the force override, and not a sibling's.
func TestJobCancel_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Cancel(context.Background(), client, CancelInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "canceling jobs requires Developer+ role")
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

// TestJobRetry_APIError verifies a 403 on retry carries the retry hint.
func TestJobRetry_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Retry(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "retrying jobs requires Developer+ role")
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

// TestListProject_ForwardsEveryListParameter verifies ListProject sends the
// scope, the retried flag, the offset page, the keyset cursor and the ordering
// as the caller set them, and reads the pagination block off the response.
// The test this replaces set the retried flag and the page and read neither
// back, so ListProject could drop include_retried and stay green.
func TestListProject_ForwardsEveryListParameter(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathProjectJobs {
			scopes := r.URL.Query()["scope[]"]
			if len(scopes) != 2 || scopes[0] != "running" || scopes[1] != "failed" {
				t.Errorf("expected scope[]=[running,failed], got %v", scopes)
			}
			assertListQuery(t, r, map[string]string{
				"include_retried": "true", "page": "3", "per_page": "10",
				"order_by": "id", "sort": "desc", "pagination": "keyset", "page_token": "tok-project",
			})
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", jobJSON),
				testutil.PaginationHeaders{Page: "3", PerPage: "10", Total: "31", TotalPages: "4", NextPage: "4", PrevPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListProject(context.Background(), client, ListProjectInput{
		ProjectID:      "42",
		Scope:          []string{"running", "failed"},
		IncludeRetried: true,
		OrderBy:        "id",
		Sort:           "desc",
		Page:           3, PerPage: 10,
		Pagination: "keyset", PageToken: "tok-project",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Jobs) != 1 {
		t.Fatalf("len(Jobs) = %d, want 1", len(out.Jobs))
	}
	wantPage := toolutil.PaginationOutput{Page: 3, PerPage: 10, TotalItems: 31, TotalPages: 4, NextPage: 4, PrevPage: 2, HasMore: true}
	if out.Pagination != wantPage {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, wantPage)
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
			if q.Get("include_retried") != "true" {
				t.Errorf("expected include_retried=true, got %q", q.Get("include_retried"))
			}
			if q.Get("order_by") != "id" || q.Get("sort") != "desc" {
				t.Errorf("expected order_by=id&sort=desc, got order_by=%q sort=%q", q.Get("order_by"), q.Get("sort"))
			}
			if q.Get("pagination") != "keyset" || q.Get("page_token") != "tok" {
				t.Errorf("expected keyset/tok, got pagination=%q page_token=%q", q.Get("pagination"), q.Get("page_token"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, fmt.Sprintf("[%s]", bridgeJSON),
				testutil.PaginationHeaders{Page: "1", PerPage: "5", Total: "7", TotalPages: "2", NextPage: "2"})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ListBridges(context.Background(), client, BridgeListInput{
		ProjectID:      "42",
		PipelineID:     10,
		Scope:          []string{"success"},
		IncludeRetried: true,
		OrderBy:        "id",
		Sort:           "desc",
		Page:           1, PerPage: 5,
		Pagination: "keyset", PageToken: "tok",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Bridges) != 1 {
		t.Fatalf("len(Bridges) = %d, want 1", len(out.Bridges))
	}
	wantPage := toolutil.PaginationOutput{Page: 1, PerPage: 5, TotalItems: 7, TotalPages: 2, NextPage: 2, HasMore: true}
	if out.Pagination != wantPage {
		t.Errorf("Pagination = %+v, want %+v", out.Pagination, wantPage)
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

// ---------------------------------------------------------------------------
// DownloadSingleArtifact — API error, canceled context
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

// ---------------------------------------------------------------------------
// DownloadSingleArtifactByRef — API error, canceled context
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

// ---------------------------------------------------------------------------
// Erase — API error, canceled context
// ---------------------------------------------------------------------------.

// TestErase_APIError verifies a 403 on erase carries the erase hint.
func TestErase_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Erase(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "erasing jobs requires Maintainer+ role")
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

// TestKeepArtifacts_APIError verifies a 403 on keep carries the keep hint.
func TestKeepArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := KeepArtifacts(context.Background(), client, ActionInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "keeping artifacts requires Maintainer+ role")
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

// TestPlay_APIError verifies a 403 on play carries the play hint, which is
// about the role and not about the job's state, the 400's subject.
func TestPlay_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	_, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "playing manual jobs requires Developer+ role")
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

// playBodyRecorder answers the play endpoint and keeps the request body it was
// sent, for the tests that assert what reached GitLab rather than what came
// back.
func playBodyRecorder(t *testing.T) (*gitlabclient.Client, *atomic.Value) {
	t.Helper()
	var gotBody atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			gotBody.Store(string(body))
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))
	return client, &gotBody
}

// TestPlay_WithVariables_ForwardsEachInTheRequestBody verifies the variables a
// caller gives reach GitLab as job_variables_attributes, each with its key and
// value, and with variable_type only where the caller set one: an unset type
// is GitLab's default, not an empty string to send. The test this replaces
// checked the job that came back and never read the body.
func TestPlay_WithVariables_ForwardsEachInTheRequestBody(t *testing.T) {
	client, gotBody := playBodyRecorder(t)

	_, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		JobVariablesAttributes: []JobVariableInput{
			{Key: "ENV", Value: "production", VariableType: "env_var"},
			{Key: "KUBECONFIG", Value: "/tmp/kubeconfig", VariableType: "file"},
			{Key: "PLAIN", Value: "untyped"},
		},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var body struct {
		Vars   []map[string]any `json:"job_variables_attributes"`
		Inputs json.RawMessage  `json:"job_inputs"`
	}
	raw, _ := gotBody.Load().(string)
	if decodeErr := json.Unmarshal([]byte(raw), &body); decodeErr != nil {
		t.Fatalf("request body %q does not decode: %v", raw, decodeErr)
	}
	want := []map[string]any{
		{"key": "ENV", "value": "production", "variable_type": "env_var"},
		{"key": "KUBECONFIG", "value": "/tmp/kubeconfig", "variable_type": "file"},
		{"key": "PLAIN", "value": "untyped"},
	}
	if !reflect.DeepEqual(body.Vars, want) {
		t.Errorf("job_variables_attributes = %v, want %v", body.Vars, want)
	}
	if body.Inputs != nil {
		t.Errorf("job_inputs = %s, want none sent when the caller gave none", body.Inputs)
	}
}

// TestPlay_NoVariablesOrInputs_SendsNeitherKey verifies a plain play carries
// neither job_variables_attributes nor job_inputs: an empty array in the body
// is a request GitLab was never sent before, not the absence of one.
func TestPlay_NoVariablesOrInputs_SendsNeitherKey(t *testing.T) {
	client, gotBody := playBodyRecorder(t)

	if _, err := Play(context.Background(), client, PlayInput{ProjectID: "42", JobID: 100}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	var body map[string]json.RawMessage
	raw, _ := gotBody.Load().(string)
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("request body %q does not decode: %v", raw, err)
	}
	for _, key := range []string{"job_variables_attributes", "job_inputs"} {
		t.Run(key, func(t *testing.T) {
			if v, ok := body[key]; ok {
				t.Errorf("body carries %s = %s, want the key absent", key, v)
			}
		})
	}
}

// TestPlay_WithJobInputs_ForwardsInRequestBody verifies Play forwards
// job_inputs in the request body, converted to the SDK's typed
// pipeline-input values, string arrays included.
func TestPlay_WithJobInputs_ForwardsInRequestBody(t *testing.T) {
	var gotBody atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathJobPlay {
			body, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			gotBody.Store(string(body))
			testutil.RespondJSON(w, http.StatusOK, jobJSON)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		JobInputs: map[string]any{"environment": "staging", "replicas": float64(3), "debug": true, "regions": []any{"eu", "us"}},
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	body, _ := gotBody.Load().(string)
	for _, want := range []string{`"job_inputs"`, `"environment":"staging"`, `"replicas":3`, `"debug":true`, `"regions":["eu","us"]`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("request body = %s, want it to contain %s", body, want)
			}
		})
	}
}

// TestPlay_InvalidJobInputs_RejectedBeforeRequest verifies Play rejects an
// unsupported job-input value type before any request is made.
func TestPlay_InvalidJobInputs_RejectedBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))

	_, err := Play(context.Background(), client, PlayInput{
		ProjectID: "42",
		JobID:     100,
		JobInputs: map[string]any{"bad": map[string]any{"nested": true}},
	})
	assertContains(t, err, "job inputs must be string, number, boolean, or array of strings")
}

// ---------------------------------------------------------------------------
// DeleteArtifacts — API error
// ---------------------------------------------------------------------------.

// TestDeleteArtifacts_APIError verifies a 403 on delete carries the delete
// hint.
func TestDeleteArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteArtifacts(context.Background(), client, DeleteArtifactsInput{ProjectID: "42", JobID: 100})
	assertContains(t, err, "deleting artifacts requires Maintainer+ role")
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

// TestDeleteProjectArtifacts_APIError verifies a 403 on the bulk delete
// carries its own hint, the one that says the deletion is project-wide.
func TestDeleteProjectArtifacts_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":msgServerError}`)
	}))
	err := DeleteProjectArtifacts(context.Background(), client, DeleteProjectArtifactsInput{ProjectID: "42"})
	assertContains(t, err, "bulk-deleting all project artifacts requires Maintainer+ role")
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
		Pipeline:       &PipelineObject{ID: 10},
		Ref:            "main",
		Commit:         &CommitObject{ID: "abcdef1234567890"},
		AllowFailure:   true,
		Duration:       45.5,
		QueuedDuration: 2.1,
		FailureReason:  "script_failure",
		Coverage:       85.5,
		User:           &UserObject{Username: "testuser"},
		CreatedAt:      "2026-03-01T10:00:00Z",
		WebURL:         "https://gitlab.example.com/-/jobs/100",
	})

	want := "## ✅ Job #100: build\n\n" +
		"- **Pipeline**: #10\n" +
		"- **Stage**: build\n" +
		"- **Status**: ✅ success\n" +
		"- **Allow Failure**: ✅\n" +
		"- **Ref**: main\n" +
		"- **Tag**: ❌\n" +
		"- **Commit**: `abcdef123456`\n" +
		"- **Duration**: 45.5s\n" +
		"- **Queued**: 2.1s\n" +
		"- **Failure Reason**: script_failure\n" +
		"- **Coverage**: 85.5%\n" +
		"- **User**: @testuser\n" +
		"- **Created**: 1 Mar 2026 10:00 UTC\n" +
		"- **URL**: [https://gitlab.example.com/-/jobs/100](https://gitlab.example.com/-/jobs/100)\n" +
		jobCardHints
	if md != want {
		t.Errorf("FormatOutputMarkdown(all fields)\n got %q\nwant %q", md, want)
	}
}

// jobCardHints is the guidance section a job card closes with when the job is
// neither erased nor archived.
const jobCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.trace' to read this job's log\n" +
	"- Use action 'job.retry' to re-run this job\n" +
	"- Use action 'job.cancel' to cancel it while it is still running\n"

// TestFormatOutputMarkdown_ErasedAndArchivedRowsAndHints checks the three
// rows a job's own state decides, and that the hints follow them: GitLab keeps
// no log for an erased job and refuses both a retry and a cancel on an
// archived one, so offering those was advice that could only fail. The hints
// are replaced rather than dropped, so the card still closes with three.
func TestFormatOutputMarkdown_ErasedAndArchivedRowsAndHints(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                7,
		Name:              "build",
		Stage:             "build",
		Status:            "success",
		Ref:               "main",
		ErasedAt:          "2026-03-02T08:00:00Z",
		ArtifactsExpireAt: "2026-04-01T00:00:00Z",
		Archived:          true,
	})
	want := "## ✅ Job #7: build 📦\n\n" +
		"- **Stage**: build\n" +
		"- **Status**: ✅ success\n" +
		"- **Allow Failure**: ❌\n" +
		"- **Ref**: main\n" +
		"- **Tag**: ❌\n" +
		"- **Erased**: 2 Mar 2026 08:00 UTC\n" +
		"- **Artifacts Expire**: 1 Apr 2026 00:00 UTC\n" +
		"- 📦 **Archived**\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.list_project' to look at the project's other jobs, since this one's log was erased\n" +
		"- Use action 'pipeline.get' to open the pipeline this job ran in\n" +
		"- Use action 'job.list' to see the other jobs of that pipeline\n"
	if md != want {
		t.Errorf("FormatOutputMarkdown(erased and archived)\n got %q\nwant %q", md, want)
	}
}

// TestFormatOutputMarkdown_HintsFollowEachStateOnItsOwn drives the erased and
// archived states one at a time, since the cards above set neither or both: an
// erased job that is not archived keeps its retry and cancel, and an archived
// one whose log remains keeps the log.
func TestFormatOutputMarkdown_HintsFollowEachStateOnItsOwn(t *testing.T) {
	tests := []struct {
		name string
		job  Output
		want string
	}{
		{"erased only", Output{ID: 1, ErasedAt: "2026-03-02T08:00:00Z"}, "\n---\n💡 **Next steps:**\n" +
			"- Use action 'job.list_project' to look at the project's other jobs, since this one's log was erased\n" +
			"- Use action 'job.retry' to re-run this job\n" +
			"- Use action 'job.cancel' to cancel it while it is still running\n"},
		{"archived only", Output{ID: 1, Archived: true}, "\n---\n💡 **Next steps:**\n" +
			"- Use action 'job.trace' to read this job's log\n" +
			"- Use action 'pipeline.get' to open the pipeline this job ran in\n" +
			"- Use action 'job.list' to see the other jobs of that pipeline\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if md := FormatOutputMarkdown(tt.job); !strings.HasSuffix(md, tt.want) {
				t.Errorf("FormatOutputMarkdown(%s) hints\n got %q\nwant suffix %q", tt.name, md, tt.want)
			}
		})
	}
}

// TestFormatOutputMarkdown_PipelineWithoutID_OmitsTheRow checks that a
// pipeline object GitLab sent without an id, which the converter keeps for its
// other keys, is not shown as "#0": a reader would take that for a pipeline.
func TestFormatOutputMarkdown_PipelineWithoutID_OmitsTheRow(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 3, Name: "n", Stage: "s", Status: "success", Ref: "r", Pipeline: &PipelineObject{Ref: "main"}})
	if strings.Contains(md, "**Pipeline**") {
		t.Errorf("FormatOutputMarkdown(pipeline without id) shows a pipeline row:\n%s", md)
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

// TestFormatOutputMarkdown_MinimalFields checks that a job GitLab said little
// about renders only the rows it answered: no label stands without a value.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:     50,
		Name:   "test",
		Stage:  "test",
		Status: "running",
		Ref:    "develop",
	})
	want := "## 🔵 Job #50: test\n\n" +
		"- **Stage**: test\n" +
		"- **Status**: 🔵 running\n" +
		"- **Allow Failure**: ❌\n" +
		"- **Ref**: develop\n" +
		"- **Tag**: ❌\n" +
		jobCardHints
	if md != want {
		t.Errorf("FormatOutputMarkdown(minimal)\n got %q\nwant %q", md, want)
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
	// These jobs carry no web URL, and MdTitleLink renders a bare label rather
	// than the empty link the hand-written "[%d](%s)" produced.
	want := "## Jobs (2)\n\n" +
		"| ID | Name | Stage | Status | Duration |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| #100 | build | build | ✅ success | 45.5s |\n" +
		"| #101 | test | test | ❌ failed | 12.3s |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		jobListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("FormatListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// jobListHints is the guidance section a job list closes with.
const jobListHints = "\n---\n💡 **Next steps:**\n" +
	"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
	"- Use action 'job.get' to see one job in full\n" +
	"- Use action 'job.trace' to read a job's log\n"

// TestFormatListMarkdown_Empty checks that an empty page is the one sentence
// and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	const want = "No jobs found.\n"
	if got := FormatListMarkdown(ListOutput{}); got != want {
		t.Errorf("FormatListMarkdown(empty)\n got %q\nwant %q", got, want)
	}
}

// TestFormatListMarkdown_KeysetPageLinksAndMarksArchived checks the two things
// a keyset page decides: the heading counts the rows shown, since GitLab sends
// no total, and an archived job is marked in the row, without which a reader
// cannot tell one GitLab will not retry from a live one.
func TestFormatListMarkdown_KeysetPageLinksAndMarksArchived(t *testing.T) {
	out := ListOutput{
		Jobs: []Output{
			{
				ID: 200, Name: "deploy", Stage: "deploy", Status: "success", Duration: 10.0,
				Archived: true,
				WebURL:   "https://gitlab.example.com/-/jobs/200",
			},
		},
		Pagination: toolutil.PaginationOutput{NextPage: 2},
	}
	want := "## Jobs (1 shown, more available)\n\n" +
		"| ID | Name | Stage | Status | Duration |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| [#200](https://gitlab.example.com/-/jobs/200) | deploy 📦 | deploy | ✅ success | 10.0s |\n" +
		jobListHints
	if got := FormatListMarkdown(out); got != want {
		t.Errorf("FormatListMarkdown(keyset)\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatTraceMarkdown
// ---------------------------------------------------------------------------.

// traceHints is the guidance section a job trace closes with.
const traceHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.get' to see this job's details\n" +
	"- Use action 'job.retry' to re-run it\n"

// TestFormatTraceMarkdown_WithData checks the whole trace rendering: the
// heading, the log in a fence sized to its own content, and no truncation
// note when nothing was cut.
func TestFormatTraceMarkdown_WithData(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{
		JobID: 100,
		Trace: "Running with gitlab-runner 15.0.0\nJob succeeded",
	})
	want := "## Job #100 Trace\n\n" +
		"```\nRunning with gitlab-runner 15.0.0\nJob succeeded\n```\n" +
		traceHints
	if md != want {
		t.Errorf("FormatTraceMarkdown()\n got %q\nwant %q", md, want)
	}
}

// TestFormatTraceMarkdown_Truncated checks that the note names the end that is
// missing. The log is cut at the first 100 KB and a failure is almost always
// at the end of it, so a reader told only "truncated" would look for the error
// in what is shown.
func TestFormatTraceMarkdown_Truncated(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{
		JobID:     100,
		Trace:     "partial log...",
		Truncated: true,
	})
	want := "## Job #100 Trace\n\n" +
		"⚠️ Showing the first 100 KB of the log. The end, where a failure usually appears, is not included.\n" +
		"\n```\npartial log...\n```\n" +
		traceHints
	if md != want {
		t.Errorf("FormatTraceMarkdown(truncated)\n got %q\nwant %q", md, want)
	}
}

// TestFormatTraceMarkdown_Empty checks that a job with no log yet renders no
// fence at all: an empty fence is a glyph that reads as content.
func TestFormatTraceMarkdown_Empty(t *testing.T) {
	md := FormatTraceMarkdown(TraceOutput{JobID: 99})
	want := "## Job #99 Trace\n" + traceHints
	if md != want {
		t.Errorf("FormatTraceMarkdown(empty)\n got %q\nwant %q", md, want)
	}
}

// ---------------------------------------------------------------------------
// FormatBridgeListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatBridgeListMarkdown_WithData verifies FormatBridgeListMarkdown when with data.
func TestFormatBridgeListMarkdown_WithData(t *testing.T) {
	out := BridgeListOutput{
		Bridges: []BridgeOutput{
			{ID: 200, Name: "trigger-downstream", Stage: "deploy", Status: "success", Duration: 10.0, DownstreamPipeline: &PipelineInfoObject{ID: 50}},
			{ID: 201, Name: "trigger-other", Stage: "deploy", Status: "failed", Duration: 5.0},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	}
	want := "## Bridge Jobs (2)\n\n" +
		"| ID | Name | Stage | Status | Duration | Downstream |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| #200 | trigger-downstream | deploy | ✅ success | 10.0s | #50 |\n" +
		"| #201 | trigger-other | deploy | ❌ failed | 5.0s |  |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'pipeline.get' to open the downstream pipeline\n"
	if got := FormatBridgeListMarkdown(out); got != want {
		t.Errorf("FormatBridgeListMarkdown()\n got %q\nwant %q", got, want)
	}
}

// TestFormatBridgeListMarkdown_LinksTheDownstreamPipeline checks that the
// downstream cell is a link when GitLab sent the pipeline's address: the
// bridge exists to point at that pipeline, and the cell named it without
// reaching it.
func TestFormatBridgeListMarkdown_LinksTheDownstreamPipeline(t *testing.T) {
	out := BridgeListOutput{
		Bridges: []BridgeOutput{{
			ID: 300, Name: "trigger", Stage: "deploy", Status: "success", Duration: 1.0,
			WebURL: "https://gitlab.example.com/-/jobs/300",
			DownstreamPipeline: &PipelineInfoObject{
				ID: 77, WebURL: "https://gitlab.example.com/downstream/-/pipelines/77",
			},
		}},
	}
	md := FormatBridgeListMarkdown(out)
	const wantRow = "| [#300](https://gitlab.example.com/-/jobs/300) | trigger | deploy | ✅ success | 1.0s | " +
		"[#77](https://gitlab.example.com/downstream/-/pipelines/77) |\n"
	if !strings.Contains(md, wantRow) {
		t.Errorf("FormatBridgeListMarkdown() missing %q:\n%s", wantRow, md)
	}
}

// TestFormatBridgeListMarkdown_Empty checks that an empty page is the one
// sentence and nothing else.
func TestFormatBridgeListMarkdown_Empty(t *testing.T) {
	const want = "No bridge jobs found.\n"
	if got := FormatBridgeListMarkdown(BridgeListOutput{}); got != want {
		t.Errorf("FormatBridgeListMarkdown(empty)\n got %q\nwant %q", got, want)
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
	want := "## Job #100 Artifacts\n\n" +
		"- **Size (bytes)**: 2048\n" +
		"\nThe archive is base64-encoded; decode it to extract the files.\n" +
		artifactsHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown()\n got %q\nwant %q", md, want)
	}
}

// artifactsHints is the guidance section an artifacts download closes with.
const artifactsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.download_single_artifact' to fetch one file out of the archive instead\n"

// TestFormatArtifactsMarkdown_WithoutJobID checks the heading of the by-ref
// download, which answers for a ref rather than for a job ID and must not
// print "Job #0".
func TestFormatArtifactsMarkdown_WithoutJobID(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{Size: 512})
	want := "## Artifacts\n\n" +
		"- **Size (bytes)**: 512\n" +
		"\nThe archive is base64-encoded; decode it to extract the files.\n" +
		artifactsHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown(no job)\n got %q\nwant %q", md, want)
	}
}

// TestFormatArtifactsMarkdown_Truncated checks that a cut archive is marked
// with the warning sign rather than a tick, which on "Truncated" reads as
// success.
func TestFormatArtifactsMarkdown_Truncated(t *testing.T) {
	md := FormatArtifactsMarkdown(ArtifactsOutput{
		JobID:     100,
		Size:      1048576,
		Truncated: true,
	})
	want := "## Job #100 Artifacts\n\n" +
		"- **Size (bytes)**: 1048576\n" +
		"- ⚠️ **Truncated at 1 MB**\n" +
		"\nThe archive is base64-encoded; decode it to extract the files.\n" +
		artifactsHints
	if md != want {
		t.Errorf("FormatArtifactsMarkdown(truncated)\n got %q\nwant %q", md, want)
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
	want := "## Job #100: report.txt\n\n" +
		"- **Size (bytes)**: 256\n" +
		"\n```\ntest report content\n```\n" +
		singleArtifactHints
	if md != want {
		t.Errorf("FormatSingleArtifactMarkdown()\n got %q\nwant %q", md, want)
	}
}

// singleArtifactHints is the guidance section one artifact file closes with.
const singleArtifactHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.artifacts' to download the whole artifacts archive\n"

// TestFormatSingleArtifactMarkdown_WithoutJobID checks that the by-ref
// download heads with the path alone rather than "Job #0".
func TestFormatSingleArtifactMarkdown_WithoutJobID(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		ArtifactPath: "output.log",
		Size:         64,
		Content:      "log data",
	})
	want := "## output.log\n\n" +
		"- **Size (bytes)**: 64\n" +
		"\n```\nlog data\n```\n" +
		singleArtifactHints
	if md != want {
		t.Errorf("FormatSingleArtifactMarkdown(no job)\n got %q\nwant %q", md, want)
	}
}

// TestFormatSingleArtifactMarkdown_Truncated checks that a cut file is marked
// with the warning sign.
func TestFormatSingleArtifactMarkdown_Truncated(t *testing.T) {
	md := FormatSingleArtifactMarkdown(SingleArtifactOutput{
		JobID:        100,
		ArtifactPath: "big.bin",
		Size:         1048576,
		Content:      "...",
		Truncated:    true,
	})
	want := "## Job #100: big.bin\n\n" +
		"- **Size (bytes)**: 1048576\n" +
		"- ⚠️ **Truncated at 1 MB**\n" +
		"\n```\n...\n```\n" +
		singleArtifactHints
	if md != want {
		t.Errorf("FormatSingleArtifactMarkdown(truncated)\n got %q\nwant %q", md, want)
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

// TestListBridges_UnreadableCapturedProject verifies that the bridge list
// handler returns an error rather than a half-filled bridge when GitLab sends
// the project object as something that is not an object. The SDK ignores the
// key its own Bridge does not model, so the read of the captured response is
// the only thing that can notice.
func TestListBridges_UnreadableCapturedProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"name":"trigger","status":"success","project":"not-an-object"}]`)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "list_bridges", Call: func() error {
			_, err := ListBridges(context.Background(), client, BridgeListInput{ProjectID: "42", PipelineID: 7})
			return err
		}},
	})
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
