// deployments_test.go contains unit tests for the deployment MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package deployments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// deploymentList tests
// ---------------------------------------------------------------------------.

// TestDeploymentList_Success verifies that DeploymentList succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[
				{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z"},
				{"id":2,"iid":2,"ref":"develop","sha":"def456","status":"running","user":{"username":"dev"},"environment":{"name":"staging"},"created_at":"2026-01-02T00:00:00Z"}
			]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "2", TotalPages: "1"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Deployments) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(out.Deployments))
	}
	if out.Deployments[0].Status != "success" || out.Deployments[0].User == nil || out.Deployments[0].User.Username != "admin" {
		t.Errorf("first deployment mismatch: %+v", out.Deployments[0])
	}
	if out.Deployments[1].Environment == nil || out.Deployments[1].Environment.Name != "staging" {
		t.Errorf("second deployment env mismatch: %+v", out.Deployments[1])
	}
}

// TestDeploymentList_WithFilters verifies the DeploymentList_WithFilters handler.
// The mock GitLab API at /api/v4/projects/42/deployments (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentList_WithFilters(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" {
			if r.URL.Query().Get("environment") != "production" {
				t.Errorf("expected environment=production, got %s", r.URL.Query().Get("environment"))
			}
			if r.URL.Query().Get("status") != "success" {
				t.Errorf("expected status=success, got %s", r.URL.Query().Get("status"))
			}
			if r.URL.Query().Get("order_by") != "created_at" {
				t.Errorf("expected order_by=created_at, got %s", r.URL.Query().Get("order_by"))
			}
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[]`, testutil.PaginationHeaders{Page: "1", PerPage: "20", Total: "0", TotalPages: "0"})
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	_, err := List(context.Background(), client, ListInput{
		ProjectID:   "42",
		Environment: "production",
		Status:      "success",
		OrderBy:     "created_at",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeploymentList_MissingProjectID asserts the handler refuses a list with
// no project before it reaches GitLab, naming the field that is missing.
//
// [testutil.ForbiddenHandler] fails the test if any request arrives, which is
// what separates the handler's own refusal from GitLab answering a path with a
// hole in it.
func TestDeploymentList_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentList_CancelledContext asserts that a canceled context aborts
// the list without contacting GitLab, which is what the handler's own context
// check is for. [testutil.ForbiddenHandler] is what proves the second half: a
// handler that answered would have let the request through.
func TestDeploymentList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentGet tests
// ---------------------------------------------------------------------------.

// TestDeploymentGet_WithPipelineWebURL verifies the DeploymentGet_WithPipelineWebURL handler.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentGet_WithPipelineWebURL(t *testing.T) {
	const pipelineURL = "https://gitlab.example.com/my-org/project/-/pipelines/123"

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success",
				"user":{"username":"admin"},
				"environment":{"name":"production"},
				"created_at":"2026-01-01T00:00:00Z",
				"deployable":{"pipeline":{"web_url":"`+pipelineURL+`"}}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Deployable == nil || out.Deployable.Pipeline == nil || out.Deployable.Pipeline.WebURL != pipelineURL {
		t.Errorf("deployable pipeline web_url mismatch: %+v", out.Deployable)
	}
}

// TestDeploymentGet_FullDeployable verifies that the Get handler maps a fully
// populated deployable (user, commit, pipeline, runner sub-objects) into the
// nested output shapes mirrored from client-go.
func TestDeploymentGet_FullDeployable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{
				"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success",
				"user":{"id":7,"username":"admin","name":"Admin","state":"active","web_url":"https://gl/admin"},
				"environment":{"id":3,"name":"production","slug":"prod","state":"available","tier":"production","external_url":"https://app","auto_stop_setting":"always"},
				"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z",
				"deployable":{
					"id":10,"status":"success","stage":"deploy","name":"deploy-prod","ref":"main","tag":false,"coverage":92.5,
					"created_at":"2026-01-01T00:00:00Z","started_at":"2026-01-01T00:01:00Z","finished_at":"2026-01-01T00:05:00Z","duration":240.0,
					"project":{"ci_job_token_scope_enabled":true},
					"user":{"id":8,"username":"runner-user","name":"Runner User","state":"active","web_url":"https://gl/ru","bio":"builds things","location":"Earth","public_email":"ru@x","linkedin":"ru-on-linkedin","twitter":"ru-on-twitter","website_url":"https://ru","organization":"GL","created_at":"2025-01-01T00:00:00Z"},
					"commit":{"id":"abc123","short_id":"abc","title":"Fix","message":"Fix bug","author_name":"Dev","author_email":"dev@x","authored_date":"2026-01-01T00:00:00Z","created_at":"2026-01-01T00:00:00Z","web_url":"https://gl/c/abc"},
					"pipeline":{"id":55,"sha":"abc123","ref":"main","status":"success","web_url":"https://gl/p/55","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:05:00Z"},
					"runner":{"id":99,"description":"shared","name":"runner-1","runner_type":"instance_type","status":"online","online":true,"paused":false,"is_shared":true}
				}
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.User == nil || out.User.ID != 7 || out.User.WebURL != "https://gl/admin" {
		t.Errorf("user mismatch: %+v", out.User)
	}
	if out.Environment == nil || out.Environment.ID != 3 || out.Environment.Name != "production" || out.Environment.ExternalURL != "https://app" {
		t.Errorf("environment mismatch: %+v", out.Environment)
	}
	assertDeployable(t, out.Deployable)
}

// assertDeployable verifies a fully populated deployable and its nested
// user, commit, pipeline, and runner objects.
func assertDeployable(t *testing.T, d *DeployableOutput) {
	t.Helper()
	if d == nil {
		t.Fatalf("deployable is nil")
	}
	if d.ID != 10 || d.Name != "deploy-prod" || d.Coverage != 92.5 {
		t.Errorf("deployable scalar mismatch: %+v", d)
	}
	assertDeployableUser(t, d.User)
	if d.Commit == nil || d.Commit.ShortID != "abc" || d.Commit.AuthorEmail != "dev@x" || d.Commit.Message != "Fix bug" {
		t.Errorf("deployable commit mismatch: %+v", d.Commit)
	}
	if d.Pipeline == nil || d.Pipeline.ID != 55 || d.Pipeline.WebURL != "https://gl/p/55" {
		t.Errorf("deployable pipeline mismatch: %+v", d.Pipeline)
	}
	if d.Runner == nil || d.Runner.ID != 99 || d.Runner.RunnerType != "instance_type" || !d.Runner.Online || !d.Runner.IsShared {
		t.Errorf("deployable runner mismatch: %+v", d.Runner)
	}
	if d.Project == nil || !d.Project.CIJobTokenScopeEnabled {
		t.Errorf("deployable project mismatch: %+v", d.Project)
	}
}

// assertDeployableUser verifies the documented deployable user profile fields.
//
// The two social links carry values that differ from each other, because they
// are neighboring assignments in the converter with no branch between them:
// with one fixture value for both, a profile published under the wrong link
// would read exactly like the right one and no gate could see it.
func assertDeployableUser(t *testing.T, u *DeployableUserOutput) {
	t.Helper()
	if u == nil {
		t.Fatalf("deployable user is nil")
	}
	if u.Username != "runner-user" || u.Bio != "builds things" || u.Location != "Earth" ||
		u.PublicEmail != "ru@x" || u.WebsiteURL != "https://ru" || u.Organization != "GL" || u.CreatedAt == "" {
		t.Errorf("deployable user mismatch: %+v", u)
	}
	if u.Linkedin != "ru-on-linkedin" {
		t.Errorf("Linkedin = %q, want %q", u.Linkedin, "ru-on-linkedin")
	}
	if u.Twitter != "ru-on-twitter" {
		t.Errorf("Twitter = %q, want %q", u.Twitter, "ru-on-twitter")
	}
}

// TestDeploymentGet_Success verifies that DeploymentGet succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 1 || out.Ref != "main" || out.SHA != "abc123" || out.Status != "success" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestDeploymentGet_NilDeployable verifies the DeploymentGet_NilDeployable handler.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentGet_NilDeployable(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			// deployable field is absent: zero-value DeploymentDeployable has empty Pipeline
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Deployable != nil {
		t.Errorf("Deployable = %+v, want nil", out.Deployable)
	}
}

// TestDeploymentGet_DeployableWithoutProject verifies version tolerance for the
// raw-fetched deployable.project object: older GitLab instances omit the
// deployable.project key, so the single Do(&superset) unmarshal must succeed and
// leave Project nil (omitted from output) while still surfacing the rest of the
// deployable. It asserts the request succeeds and Deployable.Project is nil.
func TestDeploymentGet_DeployableWithoutProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			// deployable present but without the documented project sub-object
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","deployable":{"id":10,"status":"success","stage":"deploy"}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Deployable == nil || out.Deployable.ID != 10 {
		t.Fatalf("expected deployable with id 10, got %+v", out.Deployable)
	}
	if out.Deployable.Project != nil {
		t.Errorf("Deployable.Project = %+v, want nil (omitted on instances without the field)", out.Deployable.Project)
	}
}

// TestDeploymentRawFetch_NewRequestError covers the NewRequest error branch of
// both raw superset helpers. A path containing an invalid percent-escape ("%zz")
// makes gitlab.NewRequest fail before any HTTP call, exercising the error return
// that real handlers reach only on malformed routes. It asserts both helpers
// return a non-nil error and never invoke the transport.
func TestDeploymentRawFetch_NewRequestError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	if _, err := rawGetDeployment(context.Background(), client, "projects/%zz/deployments/1"); err == nil {
		t.Error("rawGetDeployment: expected NewRequest error, got nil")
	}
	if _, _, err := rawListDeployments(context.Background(), client, "projects/%zz/deployments", nil); err == nil {
		t.Error("rawListDeployments: expected NewRequest error, got nil")
	}
}

// TestDeploymentList_DeployableProject verifies that the list handler's raw
// superset fetch surfaces the documented deployable.project object
// ({ci_job_token_scope_enabled}) for each deployment. It asserts the project
// sub-object decodes through the list path as it does through get.
func TestDeploymentList_DeployableProject(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","deployable":{"id":10,"status":"success","project":{"ci_job_token_scope_enabled":true}}}]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.Deployments))
	}
	d := out.Deployments[0].Deployable
	if d == nil || d.Project == nil || !d.Project.CIJobTokenScopeEnabled {
		t.Errorf("deployable project mismatch: %+v", d)
	}
}

// TestDeploymentGet_NilPipeline verifies the DeploymentGet_NilPipeline handler.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentGet_NilPipeline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","deployable":{"id":10,"status":"success","stage":"deploy"}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Deployable == nil || out.Deployable.ID != 10 {
		t.Fatalf("expected deployable with id 10, got %+v", out.Deployable)
	}
	if out.Deployable.Pipeline != nil {
		t.Errorf("Deployable.Pipeline = %+v, want nil", out.Deployable.Pipeline)
	}
}

// TestDeploymentGet_EmptyWebURL verifies the DeploymentGet_EmptyWebURL handler.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentGet_EmptyWebURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","deployable":{"pipeline":{"web_url":""}}}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Deployable != nil {
		t.Errorf("Deployable = %+v, want nil (empty pipeline web_url, no other fields)", out.Deployable)
	}
}

// TestDeploymentGet_ZeroID asserts the handler refuses a read with no
// deployment id rather than asking GitLab for deployment zero, and names the
// field. A request that did leave would reach a path GitLab answers 404 to,
// which a caller would read as a deployment that once existed.
func TestDeploymentGet_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
	if !strings.Contains(err.Error(), "deployment_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentGet_CancelledContext asserts a canceled context aborts the
// read before the request is built, with the forbidden handler standing in for
// the GitLab that must not be reached.
func TestDeploymentGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Get(ctx, client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentCreate tests
// ---------------------------------------------------------------------------.

// TestDeploymentCreate_Success verifies that DeploymentCreate succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestDeploymentCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments" && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, `{"id":3,"iid":3,"ref":"main","sha":"abc123","status":"created","environment":{"name":"staging"},"created_at":"2026-06-01T00:00:00Z"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Environment: "staging",
		Ref:         "main",
		SHA:         "abc123",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 3 || out.Status != "created" || out.Environment == nil || out.Environment.Name != "staging" {
		t.Errorf("unexpected output: %+v", out)
	}
}

// TestDeploymentCreate_MissingFields asserts that each of the four required
// fields is checked here and named in the refusal, with no request made.
//
// Naming the field is what a model needs to correct the call: a create that
// left without one of them would come back as GitLab's own 400 about a
// parameter, which says nothing about which of the four the caller omitted.
func TestDeploymentCreate_MissingFields(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	tests := []struct {
		name      string
		input     CreateInput
		wantField string
	}{
		{"missing project_id", CreateInput{Environment: "e", Ref: "r", SHA: "s"}, "project_id is required"},
		{"missing environment", CreateInput{ProjectID: "42", Ref: "r", SHA: "s"}, "environment is required"},
		{"missing ref", CreateInput{ProjectID: "42", Environment: "e", SHA: "s"}, "ref is required"},
		{"missing sha", CreateInput{ProjectID: "42", Environment: "e", Ref: "r"}, "sha is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Create(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantField) {
				t.Errorf("error = %v, want %q", err, tt.wantField)
			}
		})
	}
}

// TestDeploymentCreate_TagMissing400_HintsTagField verifies that a GitLab 19
// 400 response with "tag is missing" is wrapped with a hint that tells the
// model to retry with an explicit tag:false/tag:true field.
// The mock GitLab API at /api/v4/projects/42/deployments (POST) responds with HTTP 400.
// It asserts that the returned error carries the tag-field corrective hint.
func TestDeploymentCreate_TagMissing400_HintsTagField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"tag is missing"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", Environment: "staging", Ref: "main", SHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for 400 tag is missing")
	}
	if !strings.Contains(err.Error(), "tag:false for branch refs") {
		t.Errorf("error = %q, want tag-field hint", err.Error())
	}
}

// TestDeploymentCreate_InvalidStatus400_HintsAcceptedStatuses verifies that a
// GitLab 19 400 response with "status does not have a valid value" is wrapped
// with a hint listing the statuses the create API accepts.
// The mock GitLab API at /api/v4/projects/42/deployments (POST) responds with HTTP 400.
// It asserts that the returned error lists the accepted status values.
func TestDeploymentCreate_InvalidStatus400_HintsAcceptedStatuses(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"status does not have a valid value"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", Environment: "staging", Ref: "main", SHA: "abc123", Status: "created",
	})
	if err == nil {
		t.Fatal("expected error for 400 invalid status")
	}
	if !strings.Contains(err.Error(), "running, success, failed, or canceled") {
		t.Errorf("error = %q, want accepted-status hint", err.Error())
	}
}

// TestDeploymentCreate_Generic400_HintsInputChecks verifies that a 400 response
// without a recognized GitLab 19 drift message falls back to the generic
// environment/sha/ref verification hint.
// The mock GitLab API at /api/v4/projects/42/deployments (POST) responds with HTTP 400.
// It asserts that the returned error carries the generic input-verification hint.
func TestDeploymentCreate_Generic400_HintsInputChecks(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"ref not found"}`)
	}))

	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "42", Environment: "staging", Ref: "gone", SHA: "abc123",
	})
	if err == nil {
		t.Fatal("expected error for generic 400")
	}
	if !strings.Contains(err.Error(), "gitlab_environment_list") {
		t.Errorf("error = %q, want generic input-verification hint", err.Error())
	}
}

// TestDeploymentCreate_CancelledContext asserts a canceled context aborts the
// create before anything is posted. It matters more here than on a read: a
// deployment created after the caller gave up is one nothing will ever finish.
func TestDeploymentCreate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Create(ctx, client, CreateInput{ProjectID: "42", Environment: "e", Ref: "r", SHA: "s"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentUpdate tests
// ---------------------------------------------------------------------------.

// TestDeploymentUpdate_Success verifies that DeploymentUpdate succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestDeploymentUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	out, err := Update(context.Background(), client, UpdateInput{
		ProjectID:    "42",
		DeploymentID: 1,
		Status:       "success",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Status != "success" {
		t.Errorf("expected status 'success', got %q", out.Status)
	}
}

// TestDeploymentUpdate_ZeroID asserts the update is refused here, with the
// field named, rather than sent as a write to a path naming deployment zero.
func TestDeploymentUpdate_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 0, Status: "success"})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
	if !strings.Contains(err.Error(), "deployment_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentUpdate_MissingStatus asserts an update with nothing to set is
// refused here and names the field. Sent anyway it would be a PUT carrying an
// empty body, which GitLab answers with a 400 about a parameter rather than
// about the state the caller failed to choose.
func TestDeploymentUpdate_MissingStatus(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: ""})
	if err == nil {
		t.Fatal("expected error for missing status")
	}
	if !strings.Contains(err.Error(), "status is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentUpdate_CancelledContext asserts a canceled context aborts the
// update before the status is sent, so a deployment is not transitioned on
// behalf of a caller who is no longer there.
func TestDeploymentUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	_, err := Update(ctx, client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// deploymentDelete tests
// ---------------------------------------------------------------------------.

// TestDeploymentDelete_Success verifies that DeploymentDelete succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments/1 (DELETE) responds with HTTP NotFound.
// It asserts the returned output matches the expected fields.
func TestDeploymentDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/42/deployments/1" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeploymentDelete_ZeroID asserts a delete with no deployment id never
// leaves this process, and names the field. This is the one refusal here whose
// absence could destroy something: the id decides what is deleted.
func TestDeploymentDelete_ZeroID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	err := Delete(context.Background(), client, DeleteInput{ProjectID: "42", DeploymentID: 0})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
	if !strings.Contains(err.Error(), "deployment_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentDelete_CancelledContext asserts a canceled context aborts the
// delete before it is sent. The forbidden handler is the assertion that
// matters here: a delete that left anyway cannot be taken back.
func TestDeploymentDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	ctx := testutil.CancelledCtx(t)

	err := Delete(ctx, client, DeleteInput{ProjectID: "42", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// Approve or Reject Tests.

// TestDeploymentApprove_Success verifies that DeploymentApprove succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments/10/approval (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDeploymentApprove_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/deployments/10/approval" {
			var body struct {
				Status        string `json:"status"`
				Comment       string `json:"comment"`
				RepresentedAs string `json:"represented_as"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
				http.Error(w, "decode body", http.StatusInternalServerError)
				return
			}
			if body.RepresentedAs != "security" {
				t.Errorf("represented_as = %q, want %q", body.RepresentedAs, "security")
			}
			if body.Comment != "LGTM" {
				t.Errorf("comment = %q, want %q", body.Comment, "LGTM")
			}
			if body.Status != "approved" {
				t.Errorf("status = %q, want %q", body.Status, "approved")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:     "42",
		DeploymentID:  10,
		Status:        "approved",
		Comment:       "LGTM",
		RepresentedAs: "security",
	})
	if err != nil {
		t.Fatalf("ApproveOrReject() unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestDeploymentReject_Success verifies that DeploymentReject succeeds when the GitLab API returns a valid response.
// The mock GitLab API at /api/v4/projects/42/deployments/10/approval (POST) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDeploymentReject_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/deployments/10/approval" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "rejected",
	})
	if err != nil {
		t.Fatalf("ApproveOrReject() unexpected error: %v", err)
	}
	if out.Message == "" {
		t.Error("expected non-empty message")
	}
}

// TestDeploymentApproveOrReject_MissingProjectID asserts the approval is
// refused here, with the field named, and that no approval is recorded
// anywhere on the way.
func TestDeploymentApproveOrReject_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		DeploymentID: 10,
		Status:       "approved",
	})
	if err == nil {
		t.Fatal("expected error for missing project_id")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentApprove_OrRejectZeroDeploymentID asserts an approval naming no
// deployment is refused here and names the field, rather than being posted to
// a path that names deployment zero.
func TestDeploymentApprove_OrRejectZeroDeploymentID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID: "42",
		Status:    "approved",
	})
	if err == nil {
		t.Fatal("expected error for zero deployment_id")
	}
	if !strings.Contains(err.Error(), "deployment_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// TestDeploymentApproveOrReject_InvalidStatus asserts a status outside the two
// this action takes is refused here, with both accepted values named, and that
// nothing is posted. The refusal has to be the handler's: GitLab would answer
// a 400 that says which parameter it disliked and not what to send instead.
func TestDeploymentApproveOrReject_InvalidStatus(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "approved, rejected") {
		t.Errorf("error = %v, want the two accepted values named", err)
	}
}

// TestDeploymentApproveOrReject_APIError asserts a 403 carries the hint that
// says what a 403 means here: not a missing role in general, but that the
// caller is not an approver on the protected environment. Nothing else in the
// package would notice if that hint were exchanged for the not-found one.
func TestDeploymentApproveOrReject_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := ApproveOrReject(context.Background(), client, ApproveOrRejectInput{
		ProjectID:    "42",
		DeploymentID: 10,
		Status:       "approved",
	})
	if err == nil {
		t.Fatal("expected error for API error")
	}
	if !strings.Contains(err.Error(), "designated approver on the protected environment") {
		t.Errorf("error = %v, want the approver hint", err)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpectedAPI identifies the err expected API constant used by this package.
const errExpectedAPI = "expected API error, got nil"

// ---------------------------------------------------------------------------
// List: API error, missing project_id (via empty StringOrInt)
// ---------------------------------------------------------------------------.

// TestDeploymentList_APIError asserts a refusal GitLab raised is reported with
// the read named and GitLab's own message kept, rather than returned bare.
// The status is a 403 on purpose: the 404 hint is a branch of its own, and an
// error that carried it here would be telling a caller to check an id that was
// never the problem.
func TestDeploymentList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := List(context.Background(), client, ListInput{ProjectID: "1"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "list deployments") || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("error = %v, want the read named and GitLab's message kept", err)
	}
}

// ---------------------------------------------------------------------------
// Get: API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDeploymentGet_APIError asserts the same of the single read: the
// operation is named and GitLab's message kept, and the 404 hint about
// verifying the deployment id stays out of an error that is not about one.
func TestDeploymentGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Get(context.Background(), client, GetInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "get deployment") || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("error = %v, want the read named and GitLab's message kept", err)
	}
	if strings.Contains(err.Error(), "gitlab_deployment_list") {
		t.Errorf("error = %v, want no not-found hint on a 403", err)
	}
}

// TestDeploymentGet_MissingProjectID asserts the read is refused here, with
// the field named, rather than sent to a path with an empty project segment.
func TestDeploymentGet_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(context.Background(), client, GetInput{DeploymentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// ---------------------------------------------------------------------------
// Create: API error, with optional fields (Tag + Status)
// ---------------------------------------------------------------------------.

// TestDeploymentCreate_APIError asserts a 403 on create carries the hint about
// the role a deployment needs, which is the one corrective a caller refused
// this way can act on.
func TestDeploymentCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Create(context.Background(), client, CreateInput{
		ProjectID: "1", Environment: "staging", Ref: "main", SHA: "abc123",
	})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), opCreateDeployment) || !strings.Contains(err.Error(), "Developer+ role") {
		t.Errorf("error = %v, want the create named and the role hint", err)
	}
}

// TestDeploymentCreate_StatusErrorBranches asserts which corrective a create
// carries per status: the 400 GitLab raises about the request gets the hint
// naming what to verify, and any other refusal is named as the create it was
// without one invented for it.
func TestDeploymentCreate_StatusErrorBranches(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		wantText   string
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, wantText: "verify environment exists"},
		{name: "generic", statusCode: http.StatusUnprocessableEntity, wantText: opCreateDeployment},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, testCase.statusCode, `{"message":"failed"}`)
			}))
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "1", Environment: "staging", Ref: "main", SHA: "abc123"})
			if err == nil {
				t.Fatal(errExpectedAPI)
			}
			if !strings.Contains(err.Error(), testCase.wantText) {
				t.Fatalf("error = %v, want %q", err, testCase.wantText)
			}
		})
	}
}

// TestDeploymentCreate_WithOptionalFields verifies the DeploymentCreate_WithOptionalFields handler.
// The mock GitLab API at /api/v4/projects/42/deployments (POST) responds with HTTP Created.
// It asserts the returned output matches the expected fields.
func TestDeploymentCreate_WithOptionalFields(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/deployments" {
			testutil.RespondJSON(w, http.StatusCreated, `{
				"id":5,"iid":5,"ref":"v1.0.0","sha":"aaa111","status":"running",
				"user":{"username":"deployer"},
				"environment":{"name":"production"},
				"created_at":"2026-06-01T00:00:00Z"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))

	tag := true
	out, err := Create(context.Background(), client, CreateInput{
		ProjectID:   "42",
		Environment: "production",
		Ref:         "v1.0.0",
		SHA:         "aaa111",
		Tag:         &tag,
		Status:      "running",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want 5", out.ID)
	}
	if out.Status != "running" {
		t.Errorf("Status = %q, want %q", out.Status, "running")
	}
	if out.User == nil || out.User.Username != "deployer" {
		t.Errorf("User.Username mismatch: %+v", out.User)
	}
	if out.Environment == nil || out.Environment.Name != "production" {
		t.Errorf("Environment.Name mismatch: %+v", out.Environment)
	}
}

// ---------------------------------------------------------------------------
// Update: API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDeploymentUpdate_APIError asserts a 403 on update is reported with the
// write named and GitLab's message kept, and carries neither of the two hints
// the update has: the transition hint belongs to a 400 and the list hint to a
// 404, and a caller without the role can act on neither.
func TestDeploymentUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), opUpdateDeployment) || !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("error = %v, want the write named and GitLab's message kept", err)
	}
	if strings.Contains(err.Error(), "Transitions out of terminal states") || strings.Contains(err.Error(), "gitlab_deployment_list") {
		t.Errorf("error = %v, want no 400 or 404 hint on a 403", err)
	}
}

// TestDeploymentUpdate_BadRequest asserts a 400 carries the hint naming the
// statuses an update takes and the transitions GitLab refuses, which is the
// one thing a caller refused this way can act on.
func TestDeploymentUpdate_BadRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad status"}`)
	}))
	_, err := Update(context.Background(), client, UpdateInput{ProjectID: "1", DeploymentID: 1, Status: "blocked"})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "Transitions out of terminal states") {
		t.Fatalf("error = %v, want transition hint", err)
	}
}

// TestDeploymentUpdate_MissingProjectID asserts the write is refused here,
// with the field named, rather than sent to a path with an empty project
// segment.
func TestDeploymentUpdate_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Update(context.Background(), client, UpdateInput{DeploymentID: 1, Status: "success"})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// ---------------------------------------------------------------------------
// Delete: API error, missing project_id
// ---------------------------------------------------------------------------.

// TestDeploymentDelete_APIError asserts a 403 on delete carries the hint that
// names both of its conditions: the role, and the final state a deployment has
// to be in before GitLab will remove it.
func TestDeploymentDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "Maintainer+ role") || !strings.Contains(err.Error(), "final state") {
		t.Errorf("error = %v, want the role and final-state hint", err)
	}
}

// TestDeploymentDelete_NotFound asserts a 404 on delete points the caller at
// the listing that would have given it a real deployment id, rather than at
// the role hint a 403 carries.
func TestDeploymentDelete_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"not found"}`)
	}))
	err := Delete(context.Background(), client, DeleteInput{ProjectID: "1", DeploymentID: 1})
	if err == nil {
		t.Fatal(errExpectedAPI)
	}
	if !strings.Contains(err.Error(), "gitlab_deployment_list") {
		t.Fatalf("error = %v, want list hint", err)
	}
}

// TestDeploymentDelete_MissingProjectID asserts the delete is refused here,
// with the field named, rather than sent to a path with an empty project
// segment.
func TestDeploymentDelete_MissingProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(context.Background(), client, DeleteInput{DeploymentID: 1})
	if err == nil {
		t.Fatal(testutil.MsgErrEmptyProjectID)
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("error = %v, want the missing field named", err)
	}
}

// ---------------------------------------------------------------------------
// ApproveOrReject: canceled context
// ---------------------------------------------------------------------------.

// TestDeploymentApproveOrReject_CancelledContext asserts a canceled context
// leaves no approval recorded.
//
// Unlike the five handlers above it, this one keeps no context check of its
// own, so the refusal comes from the transport rather than from the validation
// block; the forbidden handler holds the outcome a caller cares about either
// way, which is that nothing was approved.
func TestDeploymentApproveOrReject_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := testutil.CancelledCtx(t)

	_, err := ApproveOrReject(ctx, client, ApproveOrRejectInput{
		ProjectID: "42", DeploymentID: 10, Status: "approved",
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------.

// cardHints is the guidance section a deployment card that needs no approval
// closes with, and blockedHints the one a deployment waiting on approvals
// carries instead.
const (
	cardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'environment.deployment_merge_requests' to list the merge requests this deployment shipped\n" +
		"- Use action 'environment.get' to see the environment it deployed to\n"
	blockedHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
		"- Use action 'environment.deployment_approve_or_reject' to approve or reject this blocked deployment\n" +
		"- Use action 'environment.deployment_merge_requests' to list the merge requests this deployment shipped\n" +
		"- Use action 'environment.get' to see the environment it deployed to\n"
)

// listHints is the guidance section the listing closes with, the preserve-links
// instruction first because the ID column links to the pipeline that ran each
// deployment.
const listHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- " + toolutil.HintPreserveLinks + "\n" +
	"- Use action 'environment.deployment_get' to see one deployment, its approvals and its pipeline\n" +
	"- Use action 'environment.deployment_list' to page through the rest of the project's deployments\n" +
	"- Use action 'environment.deployment_merge_requests' to list the merge requests a deployment shipped\n"

// TestFormatOutputMarkdown_AllFields pins the whole card of a deployment with
// everything GitLab sends on an ordinary one: the status with its glyph, the
// deployer as a profile link, and no approval rows for a deployment that needs
// none.
func TestFormatOutputMarkdown_AllFields(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:          1,
		IID:         10,
		Ref:         "main",
		SHA:         "abc123",
		Status:      "success",
		User:        &UserOutput{Username: "admin", WebURL: "https://gitlab.example.com/admin"},
		Environment: &EnvironmentOutput{Name: "production"},
		CreatedAt:   "2026-06-01T00:00:00Z",
		UpdatedAt:   "2026-06-01T01:00:00Z",
	})

	want := "## Deployment #1\n\n" +
		"- **IID**: 10\n" +
		"- **Status**: ✅ success\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `abc123`\n" +
		"- **Environment**: production\n" +
		"- **Deployed By**: [@admin](https://gitlab.example.com/admin)\n" +
		"- **Created**: 1 Jun 2026 00:00 UTC\n" +
		"- **Updated**: 1 Jun 2026 01:00 UTC\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_Blocked pins what the card was missing entirely: a
// deployment GitLab is holding for approval says how many are outstanding, who
// has answered so far and which rules it is counted against, and offers the
// action that unblocks it.
func TestFormatOutputMarkdown_Blocked(t *testing.T) {
	approved := time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)
	got := FormatOutputMarkdown(Output{
		ID:                   4,
		IID:                  2,
		Ref:                  "main",
		SHA:                  "0123456789abcdef",
		Status:               "blocked",
		Environment:          &EnvironmentOutput{Name: "production"},
		PendingApprovalCount: 1,
		Approvals: []toolutil.DeploymentApprovalOutput{{
			User:      &toolutil.UserBasicOutput{Username: "dana", WebURL: "https://gitlab.example.com/dana"},
			Status:    "approved",
			CreatedAt: &approved,
			Comment:   "looks good",
		}},
		ApprovalSummary: &toolutil.DeploymentApprovalSummaryOutput{Rules: []toolutil.DeploymentApprovalRuleOutput{
			{
				ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers", RequiredApprovals: 2,
				DeploymentApprovals: []toolutil.DeploymentApprovalOutput{{Status: "approved"}},
			},
			{ID: 2, AccessLevelDescription: "Sam Bauch", UserID: 7, RequiredApprovals: 1},
		}},
	})

	want := "## Deployment #4\n\n" +
		"- **IID**: 2\n" +
		// PipelineStatusEmoji has no entry for a deployment's own "blocked"
		// state, the one status a pipeline never has, so it renders the glyph
		// it gives any value its table does not know.
		"- **Status**: ❓ blocked\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `01234567`\n" +
		"- **Environment**: production\n" +
		"- **Pending Approvals**: 1\n" +
		"\n### Approvals\n\n" +
		"| User | Status | When | Comment |\n| --- | --- | --- | --- |\n" +
		"| [@dana](https://gitlab.example.com/dana) | approved | 2 Jun 2026 09:30 UTC | looks good |\n" +
		"\n### Approval Rules\n\n" +
		"| ID | Level | Grantee | Description | Required | Approved |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 1 | Maintainer |  | Maintainers | 2 | 1 |\n" +
		"| 2 | - | user #7 | Sam Bauch | 1 | 0 |\n" +
		blockedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_ZeroID asserts a zero value renders nothing at all
// rather than a card headed "Deployment #0": the formatter is registered by
// type and is driven with a zero value by the surface audit.
func TestFormatOutputMarkdown_ZeroID(t *testing.T) {
	md := FormatOutputMarkdown(Output{})
	if md != "" {
		t.Errorf("expected empty string for zero ID, got %q", md)
	}
}

// TestFormatOutputMarkdown_MinimalFields pins the whole card of a deployment
// GitLab answered with nothing optional: no label with an empty value under
// it, and no approval rows for a deployment that needs none.
func TestFormatOutputMarkdown_MinimalFields(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:     2,
		IID:    2,
		Ref:    "develop",
		SHA:    "def456",
		Status: "running",
	})

	want := "## Deployment #2\n\n" +
		"- **IID**: 2\n" +
		"- **Status**: \U0001F535 running\n" +
		"- **Ref**: develop\n" +
		"- **SHA**: `def456`\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_WithPipelineWebURL pins the two rows the backing job
// adds: the pipeline as a link, and its own status beside the deployment's.
func TestFormatOutputMarkdown_WithPipelineWebURL(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:     5,
		IID:    5,
		Ref:    "main",
		SHA:    "abc123",
		Status: "success",
		Deployable: &DeployableOutput{
			ID: 99,
			Pipeline: &DeployablePipelineOutput{
				ID:     123,
				Status: "success",
				WebURL: "https://gitlab.example.com/my-org/project/-/pipelines/123",
			},
		},
	})

	want := "## Deployment #5\n\n" +
		"- **IID**: 5\n" +
		"- **Status**: ✅ success\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `abc123`\n" +
		"- **Pipeline**: [#123](https://gitlab.example.com/my-org/project/-/pipelines/123)\n" +
		"- **Pipeline Status**: ✅ success\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_WithDeployments pins the whole listing: the heading
// counting what GitLab reported, each row's ID linked to the pipeline that ran
// it where there is one, and one guidance section at the end.
func TestFormatListMarkdown_WithDeployments(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Deployments: []Output{
			{
				ID: 1, IID: 1, Ref: "main", SHA: "abc", Status: "success",
				Environment: &EnvironmentOutput{Name: "production"},
				User:        &UserOutput{Username: "admin", WebURL: "https://gitlab.example.com/admin"},
				Deployable: &DeployableOutput{Pipeline: &DeployablePipelineOutput{
					ID: 123, WebURL: "https://gitlab.example.com/acme/web/-/pipelines/123",
				}},
			},
			{
				ID: 2, IID: 2, Ref: "develop", SHA: "def", Status: "running",
				Environment: &EnvironmentOutput{Name: "staging"},
				User:        &UserOutput{Username: "dev"},
			},
		},
		Pagination: toolutil.PaginationOutput{TotalItems: 2, Page: 1, PerPage: 20, TotalPages: 1},
	})

	want := "## Deployments (2)\n\n" +
		"| ID | IID | Ref | Status | Environment | Deployed By |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| [1](https://gitlab.example.com/acme/web/-/pipelines/123) | 1 | main | ✅ success | production | [@admin](https://gitlab.example.com/admin) |\n" +
		"| 2 | 2 | develop | \U0001F535 running | staging | @dev |\n" +
		"\nPage 1 of 1 | 2 items total | 20 per page\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_KeysetPage pins the heading of a page GitLab sent no
// total for: it counts the rows shown rather than claiming a total of zero
// above them.
func TestFormatListMarkdown_KeysetPage(t *testing.T) {
	got := FormatListMarkdown(ListOutput{
		Deployments: []Output{{ID: 9, IID: 3, Ref: "main", Status: "success"}},
		Pagination:  toolutil.PaginationOutput{HasMore: true},
	})

	want := "## Deployments (1 shown, more available)\n\n" +
		"| ID | IID | Ref | Status | Environment | Deployed By |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 9 | 3 | main | ✅ success |  |  |\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins that a project with no deployments renders
// the one sentence and nothing else.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{})

	if want := "No deployments found.\n"; got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatDeploymentNotFound asserts the not-found card is rendered as an
// error result carrying content, which is what makes a model read it as an
// answer about a deployment rather than as a successful empty one.
func TestFormatDeploymentNotFound(t *testing.T) {
	result := formatDeploymentNotFound(deploymentNotFoundOutput{Identifier: "17"})
	if result == nil || !result.IsError {
		t.Fatalf("formatDeploymentNotFound() = %#v, want error result", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("formatDeploymentNotFound() returned no content")
	}
}

// ---------------------------------------------------------------------------
// FormatApproveOrRejectMarkdown
// ---------------------------------------------------------------------------.

// confirmHints is the guidance section the approve/reject confirmation closes
// with.
const confirmHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action 'environment.deployment_get' to read the deployment back with its approvals\n" +
	"- Use action 'environment.deployment_list' to see the other deployments to this environment\n"

// TestFormatApproveOrRejectMarkdown_Approved pins the whole confirmation: one
// line of the server's own sentence, then the guidance section.
func TestFormatApproveOrRejectMarkdown_Approved(t *testing.T) {
	got := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{
		Message: "Deployment #10 approved successfully",
	})

	if want := "✅ Deployment #10 approved successfully\n" + confirmHints; got != want {
		t.Errorf("confirmation mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatApproveOrRejectMarkdown_Rejected pins the same shape for the other
// half of the action.
func TestFormatApproveOrRejectMarkdown_Rejected(t *testing.T) {
	got := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{
		Message: "Deployment #10 rejected successfully",
	})

	if want := "✅ Deployment #10 rejected successfully\n" + confirmHints; got != want {
		t.Errorf("confirmation mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatApproveOrRejectMarkdown_EmptyMessage pins that a result carrying no
// sentence still renders the glyph and the guidance rather than nothing.
func TestFormatApproveOrRejectMarkdown_EmptyMessage(t *testing.T) {
	got := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{})

	if want := "✅ \n" + confirmHints; got != want {
		t.Errorf("confirmation mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatApproveOrRejectMarkdown_HostileMessage_ReachesThePageAsText pins
// the whole confirmation for a sentence carrying a raw anchor and a line break.
// The sentence is one this server composes, but it reaches the formatter as a
// field of the result, so it is escaped like any other value read off an
// output: it stays one line and opens no link.
func TestFormatApproveOrRejectMarkdown_HostileMessage_ReachesThePageAsText(t *testing.T) {
	got := FormatApproveOrRejectMarkdown(ApproveOrRejectOutput{
		Message: "Deployment #10\n<a href=\"http://attacker.invalid\">approved</a>",
	})

	want := "✅ Deployment #10 &lt;a href=\"http://attacker.invalid\">approved&lt;/a>\n" + confirmHints
	if got != want {
		t.Errorf("confirmation mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown: a failed deployment, every optional field set
// ---------------------------------------------------------------------------.

// TestFormatOutputMarkdown_FailedDeployment pins the card of a deployment that
// did not succeed, with every optional field present: the failure glyph beside
// the status, and a deployer GitLab sent no profile URL for, whose name is
// written as text rather than as a link to nowhere.
//
// It was named for the converter it does not call, which would have counted
// toOutput as covered by a test that never reaches it.
func TestFormatOutputMarkdown_FailedDeployment(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:          100,
		IID:         50,
		Ref:         "v2.0.0",
		SHA:         "deadbeef",
		Status:      "failed",
		User:        &UserOutput{Username: "deployer"},
		Environment: &EnvironmentOutput{Name: "canary"},
		CreatedAt:   "2026-12-01T00:00:00Z",
		UpdatedAt:   "2026-12-01T12:00:00Z",
	})

	want := "## Deployment #100\n\n" +
		"- **IID**: 50\n" +
		"- **Status**: ❌ failed\n" +
		"- **Ref**: v2.0.0\n" +
		"- **SHA**: `deadbeef`\n" +
		"- **Environment**: canary\n" +
		"- **Deployed By**: @deployer\n" +
		"- **Created**: 1 Dec 2026 00:00 UTC\n" +
		"- **Updated**: 1 Dec 2026 12:00 UTC\n" +
		cardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestList_WithSortField asserts the sort direction reaches GitLab as its own
// query parameter, which is the only place a caller's choice of order is
// visible: the answer a mock writes is in whatever order the fixture wrote it.
func TestList_WithSortField(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sort") != "desc" {
			t.Errorf("expected sort=desc, got %q", r.URL.Query().Get("sort"))
		}
		testutil.RespondJSON(w, http.StatusOK, `[{"id":1,"iid":1,"ref":"main","sha":"abc","status":"success","user":{"username":"admin"},"environment":{"name":"prod"},"created_at":"2026-01-01T00:00:00Z"}]`)
	}))
	out, err := List(context.Background(), client, ListInput{ProjectID: "42", Sort: "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(out.Deployments))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs metadata
// ---------------------------------------------------------------------------.

// TestActionSpecs_Metadata asserts the package publishes six actions, each
// under an individual tool name of its own and owned by this package, and that
// the three a model is most likely to reach for carry the usage, aliases and
// parameter guidance the discovery surfaces read. No request is made: building
// the specs asks GitLab nothing.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	byTool := deploymentSpecsByTool(t, specs)

	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	if len(byTool) != len(specs) {
		t.Fatalf("unique individual tools = %d, want %d", len(byTool), len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "deployments" {
			t.Fatalf("OwnerPackage for %s = %q, want deployments", spec.Name, spec.OwnerPackage)
		}
	}

	list := byTool["gitlab_deployment_list"]
	if list.Usage == "" || len(list.Aliases) == 0 {
		t.Fatalf("gitlab_deployment_list metadata incomplete: usage=%q aliases=%d", list.Usage, len(list.Aliases))
	}

	get := byTool["gitlab_deployment_get"]
	if get.Usage == "" || len(get.Aliases) == 0 || get.ParameterGuidance["deployment_id"].SemanticRole == "" {
		t.Fatalf("gitlab_deployment_get metadata incomplete: usage=%q aliases=%d guidance(deployment_id)=%q", get.Usage, len(get.Aliases), get.ParameterGuidance["deployment_id"].SemanticRole)
	}

	approveReject := byTool["gitlab_deployment_approve_or_reject"]
	if approveReject.Usage == "" || len(approveReject.Aliases) == 0 {
		t.Fatalf("gitlab_deployment_approve_or_reject metadata incomplete: usage=%q aliases=%d", approveReject.Usage, len(approveReject.Aliases))
	}
}

// ---------------------------------------------------------------------------
// ActionSpecs route coverage for all 6 tools
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallAllRoutes drives every one of the six routes the way a
// surface dispatches it, from a map of arguments rather than a typed input, so
// a route whose handler no longer accepts what the catalog would hand it fails
// here rather than at a caller.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := newDeploymentSpecsByTool(t)

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"list", "gitlab_deployment_list", map[string]any{"project_id": "1"}},
		{"get", "gitlab_deployment_get", map[string]any{"project_id": "1", "deployment_id": 1}},
		{"create", "gitlab_deployment_create", map[string]any{"project_id": "1", "environment": "staging", "ref": "main", "sha": "abc123"}},
		{"update", "gitlab_deployment_update", map[string]any{"project_id": "1", "deployment_id": 1, "status": "success"}},
		{"delete", "gitlab_deployment_delete", map[string]any{"project_id": "1", "deployment_id": 1}},
		{"approve_or_reject", "gitlab_deployment_approve_or_reject", map[string]any{"project_id": "1", "deployment_id": 1, "status": "approved"}},
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
// Helper: ActionSpec route factory
// ---------------------------------------------------------------------------.

// newDeploymentSpecsByTool constructs deployment specs by tool test fixtures.
func newDeploymentSpecsByTool(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()

	deploymentJSON := `{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success","user":{"username":"admin"},"environment":{"name":"production"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z"}`

	handler := http.NewServeMux()

	// List deployments
	handler.HandleFunc("GET /api/v4/projects/1/deployments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+deploymentJSON+`]`)
	})

	// Get deployment
	handler.HandleFunc("GET /api/v4/projects/1/deployments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, deploymentJSON)
	})

	// Create deployment
	handler.HandleFunc("POST /api/v4/projects/1/deployments", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, deploymentJSON)
	})

	// Update deployment
	handler.HandleFunc("PUT /api/v4/projects/1/deployments/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, deploymentJSON)
	})

	// Delete deployment
	handler.HandleFunc("DELETE /api/v4/projects/1/deployments/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Approve or reject deployment
	handler.HandleFunc("POST /api/v4/projects/1/deployments/1/approval", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := testutil.NewTestClient(t, handler)
	return deploymentSpecsByTool(t, ActionSpecs(client))
}

// TestActionSpecs_DeploymentGetRoute asserts the wrapper around the read hands
// back the deployment itself on success, rather than swallowing it into the
// not-found card the same wrapper writes for a 404.
func TestActionSpecs_DeploymentGetRoute(t *testing.T) {
	const respJSON = `{"id":17,"iid":1,"ref":"main","sha":"abc","status":"success","environment":{"name":"prod"}}`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/deployments/17") {
			testutil.RespondJSON(w, http.StatusOK, respJSON)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	byTool := deploymentSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_deployment_get"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "deployment_id": 17})
	if err != nil {
		t.Fatalf("Route.Handler error: %v", err)
	}
	out, ok := result.(Output)
	if !ok {
		t.Fatalf("result type = %T, want Output", result)
	}
	if out.ID != 17 || out.Environment == nil || out.Environment.Name != "prod" {
		t.Fatalf("deployment output = %#v, want ID 17 environment prod", out)
	}
}

// TestDeployments_UnreadableCapturedPendingApprovalCount verifies that the two
// deployment handlers reading the approval fields off the captured answer
// return an error rather than a half-filled deployment when GitLab sends
// pending_approval_count as something that is not a number. The SDK ignores the
// key its own Deployment does not model, so the captured read is the only thing
// that can notice, and a deployment reported without its pending approvals
// would read as one that needs none.
func TestDeployments_UnreadableCapturedPendingApprovalCount(t *testing.T) {
	const body = `{"id":1,"iid":1,"ref":"main","status":"created","pending_approval_count":"many"}`
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, body)
	}))
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "create", Call: func() error {
			_, err := Create(context.Background(), client, CreateInput{ProjectID: "42", Environment: "production", Ref: "main", SHA: "abc123"})
			return err
		}},
		{Name: "update", Call: func() error {
			_, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: "success"})
			return err
		}},
	})
}

// ---------------------------------------------------------------------------
// What the handlers send GitLab
// ---------------------------------------------------------------------------.

// createRequestBody is a deployment creation as GitLab receives it. The two
// optional fields are pointers so a test can tell a value the caller set from
// a key this server never sent: GitLab 19 answers 400 "tag is missing" to a
// create that omits tag, so false and absent are different requests.
type createRequestBody struct {
	Environment string  `json:"environment"`
	Ref         string  `json:"ref"`
	SHA         string  `json:"sha"`
	Tag         *bool   `json:"tag"`
	Status      *string `json:"status"`
}

// optionalText renders an optional request field for a failure message,
// naming an absent key rather than printing a pointer.
func optionalText[T any](p *T) string {
	if p == nil {
		return "absent"
	}
	return fmt.Sprintf("%v", *p)
}

// TestDeploymentCreate_Request_CarriesTheFieldsTheCallerSet asserts that each
// create field reaches GitLab on its own key, and that the two optional ones
// are sent exactly when the caller set them.
//
// Nothing about the response says what was sent, so the guards around tag and
// status can be inverted, and ref and sha exchanged, with every other create
// test still passing: a caller's tag would never arrive while an unset one
// would be sent, which is the request GitLab 19 refuses.
func TestDeploymentCreate_Request_CarriesTheFieldsTheCallerSet(t *testing.T) {
	const (
		env = "production"
		ref = "release-1.2"
		sha = "9f8e7d6c5b4a3210"
	)
	branchRef, tagRef := false, true

	cases := []struct {
		name       string
		tag        *bool
		status     string
		wantTag    string
		wantStatus string
	}{
		{name: "branch ref with no initial status", tag: &branchRef, wantTag: "false", wantStatus: "absent"},
		{name: "tag ref with an initial status", tag: &tagRef, status: "running", wantTag: "true", wantStatus: "running"},
		{name: "neither optional field set", wantTag: "absent", wantStatus: "absent"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body createRequestBody
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode body: %v", err)
					testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"decode body"}`)
					return
				}
				if body.Environment != env || body.Ref != ref || body.SHA != sha {
					t.Errorf("environment/ref/sha = %q/%q/%q, want %q/%q/%q",
						body.Environment, body.Ref, body.SHA, env, ref, sha)
				}
				if got := optionalText(body.Tag); got != testCase.wantTag {
					t.Errorf("tag = %s, want %s", got, testCase.wantTag)
				}
				if got := optionalText(body.Status); got != testCase.wantStatus {
					t.Errorf("status = %s, want %s", got, testCase.wantStatus)
				}
				testutil.RespondJSON(w, http.StatusCreated,
					`{"id":7,"iid":3,"ref":"`+ref+`","sha":"`+sha+`","status":"running"}`)
			}))

			out, err := Create(context.Background(), client, CreateInput{
				ProjectID: "42", Environment: env, Ref: ref, SHA: sha, Tag: testCase.tag, Status: testCase.status,
			})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			// The id and the iid differ in the fixture so the two output
			// fields cannot be read from one another.
			if out.ID != 7 || out.IID != 3 {
				t.Errorf("id/iid = %d/%d, want 7/3", out.ID, out.IID)
			}
		})
	}
}

// TestDeploymentUpdate_Request_CarriesTheStatusTheCallerAskedFor asserts the
// new status is what reaches GitLab. Every other update test reads the status
// out of the answer the mock writes, so an update that sent no status at all
// would still report the one it was told, and the deployment would stay where
// it was.
func TestDeploymentUpdate_Request_CarriesTheStatusTheCallerAskedFor(t *testing.T) {
	for _, status := range []string{"success", "canceled"} {
		t.Run(status, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Status *string `json:"status"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode body: %v", err)
					testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"decode body"}`)
					return
				}
				if got := optionalText(body.Status); got != status {
					t.Errorf("status = %s, want %s", got, status)
				}
				testutil.RespondJSON(w, http.StatusOK,
					`{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"`+status+`"}`)
			}))

			out, err := Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 1, Status: status})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if out.Status != status {
				t.Errorf("Status = %q, want %q", out.Status, status)
			}
		})
	}
}

// TestDeploymentList_TimeFilters_EachReachesItsOwnParameter asserts the four
// timestamp filters arrive under the four names GitLab knows them by, each
// with the window the caller asked for.
//
// They are four assignments in a row with no branch between them, so a pair
// exchanged there answers a question about one window with another, and both
// coverage gates stay green on it.
func TestDeploymentList_TimeFilters_EachReachesItsOwnParameter(t *testing.T) {
	const (
		updatedAfter   = "2026-01-02T03:04:05Z"
		updatedBefore  = "2026-02-03T04:05:06Z"
		finishedAfter  = "2026-03-04T05:06:07Z"
		finishedBefore = "2026-04-05T06:07:08Z"
	)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertQueryParam(t, r, "updated_after", updatedAfter)
		testutil.AssertQueryParam(t, r, "updated_before", updatedBefore)
		testutil.AssertQueryParam(t, r, "finished_after", finishedAfter)
		testutil.AssertQueryParam(t, r, "finished_before", finishedBefore)
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	if _, err := List(context.Background(), client, ListInput{
		ProjectID:      "42",
		UpdatedAfter:   updatedAfter,
		UpdatedBefore:  updatedBefore,
		FinishedAfter:  finishedAfter,
		FinishedBefore: finishedBefore,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDeploymentList_Pagination_ReportsWhatGitLabAnswered asserts the page
// block is filled from the response headers. Every other list test reads only
// the deployments, so a listing that told a caller nothing about where the
// page ends would pass all of them while leaving a model unable to ask for the
// rest.
func TestDeploymentList_Pagination_ReportsWhatGitLabAnswered(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id":1,"iid":1,"ref":"main","sha":"abc123","status":"success"}]`,
			testutil.PaginationHeaders{Page: "2", PerPage: "20", Total: "57", TotalPages: "3", NextPage: "3", PrevPage: "1"})
	}))

	out, err := List(context.Background(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := toolutil.PaginationOutput{Page: 2, PerPage: 20, TotalItems: 57, TotalPages: 3, NextPage: 3, PrevPage: 1, HasMore: true}
	if out.Pagination != want {
		t.Errorf("pagination = %+v, want %+v", out.Pagination, want)
	}
}

// ---------------------------------------------------------------------------
// What the handlers publish of what GitLab sent
// ---------------------------------------------------------------------------.

// blockedDeploymentJSON is one deployment GitLab is holding for approval,
// carrying the three fields its extended entity sends and client-go's
// Deployment does not model, plus two timestamps that differ from each other.
const blockedDeploymentJSON = `{
	"id":7,"iid":3,"ref":"main","sha":"abc123","status":"blocked",
	"created_at":"2026-06-01T00:00:00Z","updated_at":"2026-06-02T09:30:00Z",
	"pending_approval_count":1,
	"approvals":[{"user":{"id":8,"username":"dana"},"status":"approved","created_at":"2026-06-02T09:30:00Z","comment":"looks good"}],
	"approval_summary":{"rules":[{"id":11,"access_level":40,"access_level_description":"Maintainers","required_approvals":2}]}
}`

// assertBlockedDeployment holds one deployment against blockedDeploymentJSON:
// the identifiers and the timestamps on their own fields, and the three
// approval fields beside them.
func assertBlockedDeployment(t *testing.T, out Output) {
	t.Helper()
	if out.ID != 7 || out.IID != 3 {
		t.Errorf("id/iid = %d/%d, want 7/3", out.ID, out.IID)
	}
	if out.CreatedAt != "2026-06-01T00:00:00Z" || out.UpdatedAt != "2026-06-02T09:30:00Z" {
		t.Errorf("created/updated = %q/%q, want 2026-06-01T00:00:00Z/2026-06-02T09:30:00Z", out.CreatedAt, out.UpdatedAt)
	}
	if out.PendingApprovalCount != 1 {
		t.Errorf("PendingApprovalCount = %d, want 1", out.PendingApprovalCount)
	}
	if len(out.Approvals) != 1 || out.Approvals[0].Status != "approved" || out.Approvals[0].Comment != "looks good" {
		t.Errorf("approvals = %+v, want one approval carrying its comment", out.Approvals)
	}
	if out.ApprovalSummary == nil || len(out.ApprovalSummary.Rules) != 1 ||
		out.ApprovalSummary.Rules[0].ID != 11 || out.ApprovalSummary.Rules[0].RequiredApprovals != 2 {
		t.Errorf("approval summary = %+v, want one rule requiring two approvals", out.ApprovalSummary)
	}
}

// TestDeployments_ApprovalFields_ReadFromTheAnswerGitLabSent asserts that the
// three approval fields reach the caller through all three handlers that
// answer with one deployment.
//
// The create and update handlers read them off the captured response and the
// get handler off its own raw superset, so they are three separate readings of
// the same entity. Dropping any of them leaves a deployment GitLab is holding
// for approval indistinguishable from one that needs none, and no branch
// changes when it is dropped.
func TestDeployments_ApprovalFields_ReadFromTheAnswerGitLabSent(t *testing.T) {
	cases := []struct {
		name string
		call func(*testing.T, *gitlabclient.Client) (Output, error)
	}{
		{name: "create", call: func(_ *testing.T, client *gitlabclient.Client) (Output, error) {
			return Create(context.Background(), client, CreateInput{
				ProjectID: "42", Environment: "production", Ref: "main", SHA: "abc123",
			})
		}},
		{name: "update", call: func(_ *testing.T, client *gitlabclient.Client) (Output, error) {
			return Update(context.Background(), client, UpdateInput{ProjectID: "42", DeploymentID: 7, Status: "success"})
		}},
		{name: "get", call: func(_ *testing.T, client *gitlabclient.Client) (Output, error) {
			return Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 7})
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, blockedDeploymentJSON)
			}))

			out, err := testCase.call(t, client)
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			assertBlockedDeployment(t, out)
		})
	}
}

// TestDeploymentGet_DeployableCarryingOneField_IsStillPublished asserts that
// the backing job is surfaced whenever GitLab sends any one of its documented
// fields, and that the field arrives under its own name.
//
// The guard that decides whether there is a job at all is ten conditions
// joined by &&, and a fixture that populates the job fully cannot tell them
// apart: loosening any one of them drops the job from the answer for exactly
// the deployments that carry that field alone.
func TestDeploymentGet_DeployableCarryingOneField_IsStillPublished(t *testing.T) {
	cases := []struct {
		name string
		job  string
		read func(*DeployableOutput) string
		want string
	}{
		{name: "id", job: `{"id":10}`, want: "10", read: func(d *DeployableOutput) string {
			return strconv.FormatInt(d.ID, 10)
		}},
		{name: "name", job: `{"name":"deploy-prod"}`, want: "deploy-prod", read: func(d *DeployableOutput) string {
			return d.Name
		}},
		{name: "status", job: `{"status":"running"}`, want: "running", read: func(d *DeployableOutput) string {
			return d.Status
		}},
		{name: "stage", job: `{"stage":"deploy"}`, want: "deploy", read: func(d *DeployableOutput) string {
			return d.Stage
		}},
		{name: "ref", job: `{"ref":"release-1.2"}`, want: "release-1.2", read: func(d *DeployableOutput) string {
			return d.Ref
		}},
		{name: "user", job: `{"user":{"id":8,"username":"runner-user"}}`, want: "runner-user", read: func(d *DeployableOutput) string {
			if d.User == nil {
				return "absent"
			}
			return d.User.Username
		}},
		{name: "commit", job: `{"commit":{"id":"abc123","title":"Fix"}}`, want: "abc123", read: func(d *DeployableOutput) string {
			if d.Commit == nil {
				return "absent"
			}
			return d.Commit.ID
		}},
		{name: "pipeline", job: `{"pipeline":{"id":55}}`, want: "55", read: func(d *DeployableOutput) string {
			if d.Pipeline == nil {
				return "absent"
			}
			return strconv.FormatInt(d.Pipeline.ID, 10)
		}},
		{name: "runner", job: `{"runner":{"id":99}}`, want: "99", read: func(d *DeployableOutput) string {
			if d.Runner == nil {
				return "absent"
			}
			return strconv.FormatInt(d.Runner.ID, 10)
		}},
		{name: "project", job: `{"project":{"ci_job_token_scope_enabled":true}}`, want: "true", read: func(d *DeployableOutput) string {
			if d.Project == nil {
				return "absent"
			}
			return strconv.FormatBool(d.Project.CIJobTokenScopeEnabled)
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := getDeploymentWithDeployable(t, testCase.job)
			if out.Deployable == nil {
				t.Fatalf("Deployable = nil for a job carrying only %s", testCase.name)
			}
			if got := testCase.read(out.Deployable); got != testCase.want {
				t.Errorf("deployable %s = %s, want %s", testCase.name, got, testCase.want)
			}
		})
	}
}

// TestDeploymentGet_PipelineCarryingOneField_IsStillPublished asserts the same
// of the pipeline under the job: any one documented field is enough for the
// pipeline to be published, which is the only thing that distinguishes the
// five conditions of its own emptiness guard from one another.
func TestDeploymentGet_PipelineCarryingOneField_IsStillPublished(t *testing.T) {
	cases := []struct {
		name     string
		pipeline string
		read     func(*DeployablePipelineOutput) string
		want     string
	}{
		{name: "id", pipeline: `{"id":55}`, want: "55", read: func(p *DeployablePipelineOutput) string {
			return strconv.FormatInt(p.ID, 10)
		}},
		{name: "sha", pipeline: `{"sha":"abc123"}`, want: "abc123", read: func(p *DeployablePipelineOutput) string {
			return p.SHA
		}},
		{name: "ref", pipeline: `{"ref":"release-1.2"}`, want: "release-1.2", read: func(p *DeployablePipelineOutput) string {
			return p.Ref
		}},
		{name: "status", pipeline: `{"status":"running"}`, want: "running", read: func(p *DeployablePipelineOutput) string {
			return p.Status
		}},
		{name: "web_url", pipeline: `{"web_url":"https://gl/p/55"}`, want: "https://gl/p/55", read: func(p *DeployablePipelineOutput) string {
			return p.WebURL
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := getDeploymentWithDeployable(t, `{"pipeline":`+testCase.pipeline+`}`)
			if out.Deployable == nil || out.Deployable.Pipeline == nil {
				t.Fatalf("Pipeline = nil for a pipeline carrying only %s (deployable %+v)", testCase.name, out.Deployable)
			}
			if got := testCase.read(out.Deployable.Pipeline); got != testCase.want {
				t.Errorf("pipeline %s = %s, want %s", testCase.name, got, testCase.want)
			}
		})
	}
}

// TestDeploymentGet_RunnerFlags_EachReadFromItsOwnField asserts the three
// runner flags one at a time, each against the whole triple.
//
// A runner answered with two of them true cannot distinguish a pair of
// exchanged assignments, since both sides carry the same value; one flag at a
// time is the only fixture that can, and it is also what GitLab answers for an
// ordinary instance runner.
func TestDeploymentGet_RunnerFlags_EachReadFromItsOwnField(t *testing.T) {
	cases := []struct {
		name   string
		runner string
		want   DeployableRunnerOutput
	}{
		{name: "online", runner: `{"id":99,"online":true}`, want: DeployableRunnerOutput{ID: 99, Online: true}},
		{name: "paused", runner: `{"id":99,"paused":true}`, want: DeployableRunnerOutput{ID: 99, Paused: true}},
		{name: "is_shared", runner: `{"id":99,"is_shared":true}`, want: DeployableRunnerOutput{ID: 99, IsShared: true}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := getDeploymentWithDeployable(t, `{"runner":`+testCase.runner+`}`)
			if out.Deployable == nil || out.Deployable.Runner == nil {
				t.Fatalf("Runner = nil for a runner carrying %s", testCase.name)
			}
			if got := *out.Deployable.Runner; got != testCase.want {
				t.Errorf("runner = %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// getDeploymentWithDeployable reads one deployment whose deployable is the
// given JSON object, so a case table can vary the backing job alone.
func getDeploymentWithDeployable(t *testing.T, deployable string) Output {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/deployments/1" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":1,"iid":2,"ref":"main","sha":"abc123","status":"success","deployable":`+deployable+`}`)
	}))

	out, err := Get(context.Background(), client, GetInput{ProjectID: "42", DeploymentID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	return out
}

// TestActionSpecs_DeploymentGetRoute_ErrorGitLabRaised_ReachesTheCaller
// asserts the not-found card answers a 404 and nothing else.
//
// The wrapper decides on `err != nil && IsHTTPStatus(err, 404)`, whose left
// operand alone is enough to enter the branch: with the two joined the other
// way, a 403 on a project the token cannot read would be reported to a model
// as a deployment that does not exist, and the call would look successful.
func TestActionSpecs_DeploymentGetRoute_ErrorGitLabRaised_ReachesTheCaller(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
	byTool := deploymentSpecsByTool(t, ActionSpecs(client))

	result, err := byTool["gitlab_deployment_get"].Route.Handler(t.Context(),
		map[string]any{"project_id": "42", "deployment_id": 17})
	if err == nil {
		t.Fatalf("Route.Handler result = %#v, want the error GitLab raised", result)
	}
	if _, ok := result.(deploymentNotFoundOutput); ok {
		t.Errorf("result = %#v, want no not-found card for a 403", result)
	}
	if !strings.Contains(err.Error(), "get deployment") {
		t.Errorf("error = %v, want the read named in it", err)
	}
}

// TestFormatOutputMarkdown_WhatGitLabLeftOut_RendersNoRowAtAll pins the card
// of a deployment answered with the optional halves missing: no status, a job
// that never started a pipeline, an approval recorded against no user and with
// no timestamp or comment, and an approval summary carrying no rules.
//
// Each of those is the unexercised side of a condition the card tests to
// decide whether to write a row, and a card that invented a label with nothing
// under it, or a glyph with no word beside it, would have passed all of them.
func TestFormatOutputMarkdown_WhatGitLabLeftOut_RendersNoRowAtAll(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                   8,
		IID:                  4,
		Ref:                  "main",
		SHA:                  "abc123",
		Deployable:           &DeployableOutput{ID: 9},
		PendingApprovalCount: 1,
		Approvals:            []toolutil.DeploymentApprovalOutput{{Status: "rejected"}},
		ApprovalSummary:      &toolutil.DeploymentApprovalSummaryOutput{},
	})

	want := "## Deployment #8\n\n" +
		"- **IID**: 4\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `abc123`\n" +
		"- **Pending Approvals**: 1\n" +
		"\n### Approvals\n\n" +
		"| User | Status | When | Comment |\n| --- | --- | --- | --- |\n" +
		"|  | rejected |  |  |\n" +
		blockedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_RejectionAgainstARule_IsNotCountedAsAnApproval
// pins the Approved column for a rule that has been answered twice, once each
// way. Both answers are recorded in the same list, so counting the list would
// report a rule as satisfied on the strength of the rejection that blocked it.
func TestFormatOutputMarkdown_RejectionAgainstARule_IsNotCountedAsAnApproval(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                   9,
		IID:                  5,
		Ref:                  "main",
		SHA:                  "abc123",
		Status:               "blocked",
		PendingApprovalCount: 1,
		ApprovalSummary: &toolutil.DeploymentApprovalSummaryOutput{Rules: []toolutil.DeploymentApprovalRuleOutput{{
			ID: 4, AccessLevel: 40, AccessLevelDescription: "Maintainers", RequiredApprovals: 2,
			DeploymentApprovals: []toolutil.DeploymentApprovalOutput{
				{Status: "approved"},
				{Status: "rejected"},
			},
		}}},
	})

	want := "## Deployment #9\n\n" +
		"- **IID**: 5\n" +
		"- **Status**: ❓ blocked\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `abc123`\n" +
		"- **Pending Approvals**: 1\n" +
		"\n### Approval Rules\n\n" +
		"| ID | Level | Grantee | Description | Required | Approved |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 4 | Maintainer |  | Maintainers | 2 | 1 |\n" +
		blockedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_JobWithoutAPipeline_LeavesTheIDUnlinked pins the row
// of a deployment whose backing job never started a pipeline. A deployment has
// no page of its own, so the pipeline is the only thing the ID column can link
// to, and the row that has none carries the number alone rather than a link
// with nowhere to go.
func TestFormatListMarkdown_JobWithoutAPipeline_LeavesTheIDUnlinked(t *testing.T) {
	got := FormatListMarkdown(ListOutput{Deployments: []Output{{
		ID: 4, IID: 1, Ref: "main", Status: "success",
		Deployable: &DeployableOutput{ID: 12},
	}}})

	want := "## Deployments (1)\n\n" +
		"| ID | IID | Ref | Status | Environment | Deployed By |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 4 | 1 | main | ✅ success |  |  |\n" +
		listHints

	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestDeploymentOptions_ActionWithNoCaseOfItsOwn_CarriesTheDomainDefaults
// asserts what an action added without metadata of its own would publish: the
// domain's owner, tags and cross-links rather than an empty spec.
//
// Every one of the six actions matches a case of its own, so the last case is
// the only one ever evaluated false, and nothing else reaches the defaults
// they are all built on.
func TestDeploymentOptions_ActionWithNoCaseOfItsOwn_CarriesTheDomainDefaults(t *testing.T) {
	const tool = "gitlab_deployment_rollback"
	options := deploymentOptionsForAction("deployment_rollback", tool)

	if options.OwnerPackage != "deployments" {
		t.Errorf("OwnerPackage = %q, want deployments", options.OwnerPackage)
	}
	if options.Usage == "" {
		t.Error("Usage is empty, so the action would be published with nothing said about it")
	}
	if got := strings.Join(options.Tags, ","); got != "environment,deployment" {
		t.Errorf("Tags = %q, want environment,deployment", got)
	}
	if got := strings.Join(options.RelatedActions, ","); got != actionEnvironmentGet+","+actionPipelineGet {
		t.Errorf("RelatedActions = %q, want the environment and pipeline reads", got)
	}
	if got := strings.Join(options.Aliases, ","); got != tool {
		t.Errorf("Aliases = %q, want the individual tool name", got)
	}
	if options.IndividualTool.Name != tool || options.IndividualTool.Title == "" {
		t.Errorf("IndividualTool = %+v, want %q with a title", options.IndividualTool, tool)
	}
}

// TestFormatOutputMarkdown_GroupScopedApprovalRule_NamesTheGroup pins the row
// a rule granting approval to a group renders as: the grantee names the group,
// and the level reads "-" because a group rule's access level is not what
// decides. Nothing else exercises that branch, so the group spelling could
// change to anything and no test would read it.
func TestFormatOutputMarkdown_GroupScopedApprovalRule_NamesTheGroup(t *testing.T) {
	got := FormatOutputMarkdown(Output{
		ID:                   6,
		IID:                  1,
		Ref:                  "main",
		SHA:                  "abc123",
		Status:               "blocked",
		PendingApprovalCount: 2,
		ApprovalSummary: &toolutil.DeploymentApprovalSummaryOutput{Rules: []toolutil.DeploymentApprovalRuleOutput{
			{ID: 3, GroupID: 12, AccessLevelDescription: "Release managers", RequiredApprovals: 2},
		}},
	})

	want := "## Deployment #6\n\n" +
		"- **IID**: 1\n" +
		"- **Status**: ❓ blocked\n" +
		"- **Ref**: main\n" +
		"- **SHA**: `abc123`\n" +
		"- **Pending Approvals**: 2\n" +
		"\n### Approval Rules\n\n" +
		"| ID | Level | Grantee | Description | Required | Approved |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| 3 | - | group #12 | Release managers | 2 | 0 |\n" +
		blockedHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// deploymentSpecsByTool supports deployment specs by tool assertions in deployments tests.
func deploymentSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byTool[spec.IndividualTool.Name] = spec
	}
	return byTool
}
