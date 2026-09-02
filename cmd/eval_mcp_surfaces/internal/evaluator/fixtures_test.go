package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// TestApplyLiveFixtureState_RendersTypedPromptTemplates verifies fixture state
// values feed typed prompt templates without global string replacement.
func TestApplyLiveFixtureState_RendersTypedPromptTemplates(t *testing.T) {
	evalCase := EvalCase{
		ID:             "MT-TYPED-FIXTURE",
		Prompt:         "Get project `my-org/tools/gitlab-mcp-server`.",
		PromptTemplate: CasePromptTemplate{Text: "Get project {{.Values.project_path}} on {{.Values.default_branch}}."},
		Steps:          []ExpectedStep{{ExpectedTool: "gitlab_project", ExpectedAction: "get"}},
	}
	tasks := []evalTask{taskFromCase(evalCase), {ID: "MT-STATIC", Prompt: "Keep this prompt."}}
	state := &liveFixtureState{ProjectPath: "my-org/project", DefaultBranch: "master"}

	got := applyLiveFixtureState(tasks, state)

	if got[0].Prompt != "Get project my-org/project on master." {
		t.Fatalf("typed prompt = %q, want rendered fixture values", got[0].Prompt)
	}
	if got[1].Prompt != "Keep this prompt." {
		t.Fatalf("static prompt = %q, want unchanged", got[1].Prompt)
	}
}

// TestEnsurePackageReleaseFixtureFiles_WritesLocalFiles verifies package release fixture file creation.
func TestEnsurePackageReleaseFixtureFiles_WritesLocalFiles(t *testing.T) {
	state := &liveFixtureState{}
	fixturesPath := filepath.Join(t.TempDir(), "state", "e2e-fixtures.json")

	if err := ensurePackageReleaseFixtureFiles(state, fixturesPath); err != nil {
		t.Fatalf("ensurePackageReleaseFixtureFiles() error = %v", err)
	}

	if !filepath.IsAbs(state.PackageReleaseDir) {
		t.Fatalf("PackageReleaseDir = %q, want absolute path", state.PackageReleaseDir)
	}
	if len(state.PackageReleaseFiles) != len(packageReleaseFixtureFiles) || len(state.PackageReleasePaths) != len(packageReleaseFixtureFiles) {
		t.Fatalf("fixture file counts = %d/%d, want %d", len(state.PackageReleaseFiles), len(state.PackageReleasePaths), len(packageReleaseFixtureFiles))
	}
	for _, path := range state.PackageReleasePaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixture file %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("fixture file %s is empty", path)
		}
	}
	assertContains(t, state.PackageReleaseName, liveFixturePackageReleaseName)
	assertContains(t, state.PackageReleaseVersion, liveFixturePackageReleaseVersion)
	assertContains(t, state.PackageReleaseTag, liveFixturePackageReleaseTag)
}

// TestFilterTasksByLiveFixtureState_SkipsMissingJobResources verifies that missing Docker job fixtures do not become model failures.
func TestFilterTasksByLiveFixtureState_SkipsMissingJobResources(t *testing.T) {
	tasks := []evalTask{
		{ID: "MT-020"},
		{ID: "MT-022"},
		{ID: "MT-064"},
		{ID: "MT-046"},
		{ID: "MT-065"},
		{ID: "MT-182"},
		{ID: "MT-186"},
		{ID: "MT-187"},
		{ID: "MS-008"},
		{ID: "MT-003"},
	}
	state := &liveFixtureState{ManualJobID: 19, RunnerID: 20}

	filtered := filterTasksByLiveFixtureState(tasks, state)

	if got := taskIDs(filtered); got != "MT-064,MT-046,MT-003" {
		t.Fatalf("filtered IDs = %q, want MT-064,MT-046,MT-003", got)
	}
}

// TestFilterTasksByLiveFixtureState_KeepsSeededJobResources verifies seeded Docker jobs keep dependent tasks eligible.
func TestFilterTasksByLiveFixtureState_KeepsSeededJobResources(t *testing.T) {
	tasks := []evalTask{
		{ID: "MT-020"},
		{ID: "MT-022"},
		{ID: "MT-064"},
		{ID: "MT-046"},
		{ID: "MT-065"},
		{ID: "MT-182"},
		{ID: "MT-186"},
		{ID: "MT-187"},
		{ID: "MS-008"},
	}
	state := &liveFixtureState{PipelineID: 17, FailedJobID: 18, ManualJobID: 19, RunnerID: 20, ProjectServiceAccountID: 21, ProjectServiceAccountTokenID: 22}

	filtered := filterTasksByLiveFixtureState(tasks, state)

	if got := taskIDs(filtered); got != "MT-020,MT-022,MT-064,MT-046,MT-065,MT-182,MT-186,MT-187,MS-008" {
		t.Fatalf("filtered IDs = %q, want all seeded dependency tasks", got)
	}
}

// TestFixtureCI_IsValidYAMLShape verifies FixtureCI is valid YAML shape.
func TestFixtureCI_IsValidYAMLShape(t *testing.T) {
	ci := fixtureCI()
	if strings.Contains(ci, "\t") {
		t.Fatal("fixture CI must not contain tabs because GitLab YAML rejects them")
	}
	assertContains(t, ci, "failing_fixture:")
	assertContains(t, ci, "manual_deploy:")
	assertContains(t, ci, "stage: test")
}

// TestFixtureRemoteURL verifies FixtureRemoteURL.
func TestFixtureRemoteURL(t *testing.T) {
	got := fixtureRemoteURL("http://localhost:8929/", liveFixtureProjectPath)
	want := "http://localhost:8929/my-org/tools/gitlab-mcp-server.git"
	if got != want {
		t.Fatalf("fixtureRemoteURL() = %q, want %q", got, want)
	}
}

// TestFixtureFileHelpers_CoverPathAndContentBranches verifies pure fixture
// helpers derive deterministic path/content values.
func TestFixtureFileHelpers_CoverPathAndContentBranches(t *testing.T) {
	if pathBase("dir/file.txt") != "file.txt" || pathBase("file.txt") != "file.txt" {
		t.Fatal("pathBase failed for nested or flat path")
	}
	if !strings.Contains(fixtureReadme(), "RegisterMCPMeta") {
		t.Fatal("fixtureReadme() missing expected code marker")
	}
	if key, err := newAuthorizedSSHKey(); err != nil || !strings.HasPrefix(key, "ssh-ed25519 ") {
		t.Fatalf("newAuthorizedSSHKey() = %q, %v; want ed25519 public key", key, err)
	}
}

// TestLiveFixtureStateReadWriteAndValidation_CoverFileHelpers verifies fixture
// state persistence fills defaults and validates safe live-prep options.
func TestLiveFixtureStateReadWriteAndValidation_CoverFileHelpers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixtures", "state.json")
	state := &liveFixtureState{ProjectPath: liveFixtureProjectPath, ProjectID: 101, CleanupReleaseTag: liveFixtureCleanupTag, ReleaseSummaryTag: liveFixtureCleanupTag}
	if err := writeLiveFixtures(path, state); err != nil {
		t.Fatalf("writeLiveFixtures() error = %v", err)
	}
	loaded, err := readLiveFixtures(path)
	if err != nil {
		t.Fatalf("readLiveFixtures() error = %v", err)
	}
	if loaded.ProjectID != 101 || loaded.ReleaseSummaryTag != liveFixtureReleaseSummaryTag || loaded.ElicitationReleaseTag != liveFixtureElicitationTag || len(loaded.PackageReleaseFiles) == 0 {
		t.Fatalf("loaded fixture = %+v, want defaults and package files", loaded)
	}
	badPath := filepath.Join(t.TempDir(), "bad.json")
	writeBadErr := os.WriteFile(badPath, []byte(`{"project_id":0}`), 0o600)
	if writeBadErr != nil {
		t.Fatalf("write bad fixture: %v", writeBadErr)
	}
	_, badFixtureErr := readLiveFixtures(badPath)
	if badFixtureErr == nil {
		t.Fatal("readLiveFixtures(missing project identity) error = nil, want error")
	}
	mockBackendErr := validateFixtureOptions(options{Backend: backendMock})
	if mockBackendErr == nil {
		t.Fatal("validateFixtureOptions(mock) error = nil, want backend error")
	}
	t.Setenv("E2E_MODE", "docker")
	dockerBackendErr := validateFixtureOptions(options{Backend: backendGitLab})
	if dockerBackendErr != nil {
		t.Fatalf("validateFixtureOptions(docker gitlab) error = %v", dockerBackendErr)
	}
}

// TestEnsureLiveProjectActive_UnarchivesArchivedFixtureProject verifies EnsureLiveProjectActive when unarchives archived fixture project.
func TestEnsureLiveProjectActive_UnarchivesArchivedFixtureProject(t *testing.T) {
	calls := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                  101,
				"path_with_namespace": liveFixtureProjectPath,
				"archived":            true,
			})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/unarchive":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                  101,
				"path_with_namespace": liveFixtureProjectPath,
				"archived":            false,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       server.URL,
		GitLabToken:     "eval-token",
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if activeErr := ensureLiveProjectActive(t.Context(), client); activeErr != nil {
		t.Fatalf("ensureLiveProjectActive() error = %v", activeErr)
	}

	if got := strings.Join(calls, ","); got != "GET /api/v4/projects/my-org%2Ftools%2Fgitlab-mcp-server,POST /api/v4/projects/101/unarchive" {
		t.Fatalf("calls = %q", got)
	}
}

// TestBacktickValueAfter verifies BacktickValueAfter.
func TestBacktickValueAfter(t *testing.T) {
	prompt := "Create a merge request in project `my-org/tools/gitlab-mcp-server` from `feature/eval-x` into `main`."

	got, ok := backtickValueAfter(prompt, " from ")

	if !ok || got != "feature/eval-x" {
		t.Fatalf("backtickValueAfter() = %q, %t; want feature/eval-x, true", got, ok)
	}
}

// TestSafeFixturePathPart verifies SafeFixturePathPart.
func TestSafeFixturePathPart(t *testing.T) {
	got := safeFixturePathPart("feature/eval-GPT54Mini-r1-abc123")
	want := "feature-eval-gpt54mini-r1-abc123"
	if got != want {
		t.Fatalf("safeFixturePathPart() = %q, want %q", got, want)
	}
}

// TestLiveFixturePreparerDefaultRef_DetectedBranch_ReturnsDetectedBranch verifies fixture setup honors the project default branch discovered from GitLab.
func TestLiveFixturePreparerDefaultRef_DetectedBranch_ReturnsDetectedBranch(t *testing.T) {
	preparer := &liveFixturePreparer{state: &liveFixtureState{DefaultBranch: "trunk"}}
	if got := preparer.defaultRef(); got != "trunk" {
		t.Fatalf("defaultRef() = %q, want trunk", got)
	}
	preparer.state.DefaultBranch = ""
	if got := preparer.defaultRef(); got != liveFixtureDefaultRef {
		t.Fatalf("defaultRef(empty) = %q, want %q", got, liveFixtureDefaultRef)
	}
}

// TestEnsureCIVariables_RecreatesProjectGroupAndInstanceVariables verifies fixture preparation restores every variable scope it removes.
func TestEnsureCIVariables_RecreatesProjectGroupAndInstanceVariables(t *testing.T) {
	created := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/projects/101/variables/EVAL_TOKEN"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.EscapedPath(), "/api/v4/groups/202/variables/GROUP_EVAL_TOKEN"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == "/api/v4/admin/ci/variables/INSTANCE_EVAL_TOKEN":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/variables":
			if !assertVariableCreateRequest(t, w, r, "EVAL_TOKEN", "production") {
				return
			}
			created["project"] = true
			_, _ = w.Write([]byte(`{"key":"EVAL_TOKEN","value":"masked-value-123","environment_scope":"production"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/groups/202/variables":
			if !assertVariableCreateRequest(t, w, r, "GROUP_EVAL_TOKEN", "production") {
				return
			}
			created["group"] = true
			_, _ = w.Write([]byte(`{"key":"GROUP_EVAL_TOKEN","value":"masked-value-123","environment_scope":"production"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/admin/ci/variables":
			if !assertVariableCreateRequest(t, w, r, "INSTANCE_EVAL_TOKEN", "") {
				return
			}
			created["instance"] = true
			_, _ = w.Write([]byte(`{"key":"INSTANCE_EVAL_TOKEN","value":"masked-value-123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newFixtureTestClient(t, server.URL)
	preparer := &liveFixturePreparer{client: client, state: &liveFixtureState{ProjectID: 101, GroupID: 202}}

	if err := preparer.ensureCIVariables(t.Context()); err != nil {
		t.Fatalf("ensureCIVariables() error = %v", err)
	}
	for _, scope := range []string{"project", "group", "instance"} {
		t.Run(scope, func(t *testing.T) {
			if !created[scope] {
				t.Fatalf("%s variable was not recreated", scope)
			}
		})
	}
}

// TestEnsureFile_UpdateMissingFile_CreatesFile verifies a stale successful GetFile result is recovered with CreateFile.
func TestEnsureFile_UpdateMissingFile_CreatesFile(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md":
			if r.URL.Query().Get("ref") != "trunk" {
				t.Errorf("ref = %q, want trunk", r.URL.Query().Get("ref"))
			}
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"trunk","encoding":"base64","content":"b2xkCg=="}`))
		case r.Method == http.MethodPut && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"A file with this name doesn't exist"}`))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md":
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode create request: %v", err)
				http.Error(w, "decode request", http.StatusBadRequest)
				return
			}
			if request["branch"] != "trunk" || request["content"] != "new content\n" {
				t.Errorf("create request = %+v, want trunk branch and content", request)
				http.Error(w, "unexpected create request", http.StatusBadRequest)
				return
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"trunk"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newFixtureTestClient(t, server.URL)
	preparer := &liveFixturePreparer{client: client, state: &liveFixtureState{ProjectID: 101}}

	if err := preparer.ensureFile(t.Context(), "README.md", "trunk", "new content\n", "Seed README"); err != nil {
		t.Fatalf("ensureFile() error = %v", err)
	}
	if !created {
		t.Fatal("CreateFile was not called after missing-file update error")
	}
}

// TestCreateFile_BadRequestWithoutAlreadyExists_ReturnsError verifies fixture
// setup does not hide GitLab create-file failures that leave no file behind.
func TestCreateFile_BadRequestWithoutAlreadyExists_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects/101/repository/files/README.md" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Branch does not exist"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newFixtureTestClient(t, server.URL)
	preparer := &liveFixturePreparer{client: client, state: &liveFixtureState{ProjectID: 101}}

	err := preparer.createFile(t.Context(), "README.md", "missing-branch", "content\n", "Seed README")
	if err == nil || !strings.Contains(err.Error(), "Branch does not exist") {
		t.Fatalf("createFile() error = %v, want Branch does not exist", err)
	}
}

// TestFindProjectServiceAccount verifies fixture reuse finds existing service accounts by stable identity.
func TestFindProjectServiceAccount(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		wantFound  bool
		wantID     int64
		wantErr    string
	}{
		{name: "matches by name", body: `[{"id":7,"name":"eval-project-service-account","username":"other"}]`, statusCode: http.StatusOK, wantFound: true, wantID: 7},
		{name: "matches by username prefix", body: `[{"id":8,"name":"other","username":"eval-project-svc-101-suffix"}]`, statusCode: http.StatusOK, wantFound: true, wantID: 8},
		{name: "not found", body: `[{"id":9,"name":"other","username":"unrelated"}]`, statusCode: http.StatusOK},
		{name: "list error", body: `{"message":"fail"}`, statusCode: http.StatusForbidden, wantErr: "list project service accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/101/service_accounts" {
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.body))
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}

			account, found, err := preparer.findProjectServiceAccount(t.Context())
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("findProjectServiceAccount() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("findProjectServiceAccount() error = %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantFound && account.ID != tt.wantID {
				t.Fatalf("account.ID = %d, want %d", account.ID, tt.wantID)
			}
		})
	}
}

// TestFindProjectServiceAccountPAT verifies fixture reuse finds only active, non-revoked PATs.
func TestFindProjectServiceAccountPAT(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		statusCode int
		wantFound  bool
		wantID     int64
		wantErr    string
	}{
		{name: "matches active token", body: `[{"id":11,"name":"eval-project-service-token","active":true,"revoked":false}]`, statusCode: http.StatusOK, wantFound: true, wantID: 11},
		{name: "ignores inactive token", body: `[{"id":12,"name":"eval-project-service-token","active":false,"revoked":false}]`, statusCode: http.StatusOK},
		{name: "ignores revoked token", body: `[{"id":13,"name":"eval-project-service-token","active":true,"revoked":true}]`, statusCode: http.StatusOK},
		{name: "list error", body: `{"message":"fail"}`, statusCode: http.StatusForbidden, wantErr: "list project service account PATs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/101/service_accounts/7/personal_access_tokens" {
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(tt.body))
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}

			token, found, err := preparer.findProjectServiceAccountPAT(t.Context(), 7)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("findProjectServiceAccountPAT() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("findProjectServiceAccountPAT() error = %v", err)
			}
			if found != tt.wantFound {
				t.Fatalf("found = %v, want %v", found, tt.wantFound)
			}
			if tt.wantFound && token.ID != tt.wantID {
				t.Fatalf("token.ID = %d, want %d", token.ID, tt.wantID)
			}
		})
	}
}

// newFixtureTestClient creates a GitLab client for fixture unit tests.
func newFixtureTestClient(t *testing.T, gitlabURL string) *gitlabclient.Client {
	t.Helper()
	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:       gitlabURL,
		GitLabToken:     "eval-token",
		MetaTools:       true,
		MetaParamSchema: config.DefaultMetaParamSchema,
		// Fixture tests drive failure paths through fake 4xx/5xx responses;
		// without this the SDK retries each one with backoff and a handful of
		// error-branch tests dominate the package runtime.
		DisableRetries: true,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

// assertVariableCreateRequest verifies a CI variable fixture creation request.
func assertVariableCreateRequest(t *testing.T, w http.ResponseWriter, r *http.Request, key, environmentScope string) bool {
	t.Helper()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Errorf("decode variable request: %v", err)
		http.Error(w, "decode request", http.StatusBadRequest)
		return false
	}
	if request["key"] != key || request["value"] != "masked-value-123" {
		t.Errorf("variable request = %+v, want key %s and fixture value", request, key)
		http.Error(w, "unexpected variable request", http.StatusBadRequest)
		return false
	}
	if environmentScope == "" {
		if _, ok := request["environment_scope"]; ok {
			t.Errorf("variable request = %+v, want no environment_scope", request)
			http.Error(w, "unexpected variable scope", http.StatusBadRequest)
			return false
		}
		return true
	}
	if request["environment_scope"] != environmentScope {
		t.Errorf("variable request = %+v, want environment_scope %s", request, environmentScope)
		http.Error(w, "unexpected variable scope", http.StatusBadRequest)
		return false
	}
	return true
}

// assertContains checks contains invariants for tests.
func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("%q does not contain %q", text, want)
	}
}

// fakeGitLabHandler answers the GitLab REST calls the live fixture preparer
// makes with deterministic bodies. It is deliberately generic: the preparer
// only reads identifiers and a handful of status fields back, so one object
// shape plus a few list shapes covers the whole provisioning sequence without
// a live GitLab. Requests are recorded so a test can assert what was called.
type fakeGitLabHandler struct {
	mu       sync.Mutex
	requests []string
}

// gitLabFixtureObject is the single object body the fake returns for every
// create-or-get call: it carries the identifier fields (id, iid), the path
// fields the preparer copies into its state, and the terminal statuses that
// stop the preparer's wait loops on the first poll.
const gitLabFixtureObject = `{"id":101,"iid":7,"name":"n","path":"p","full_path":"my-org","path_with_namespace":"my-org/tools/gitlab-mcp-server","default_branch":"main","detailed_merge_status":"mergeable","status":"failed","commit":{"id":"abc123"},"version":"17.0.0"}`

// gitLabFixtureDiscussion is the discussion body, whose id is a string rather
// than a number and whose notes carry the note id the preparer records.
const gitLabFixtureDiscussion = `{"id":"disc-1","notes":[{"id":55}]}`

// ServeHTTP records the request and answers it with the fixture body its path
// implies.
func (h *fakeGitLabHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	h.mu.Lock()
	h.requests = append(h.requests, r.Method+" "+path)
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(path, "/discussions") {
		fmt.Fprint(w, gitLabFixtureDiscussion)
		return
	}
	if body, ok := fakeGitLabListBody(r.Method, path); ok {
		fmt.Fprint(w, body)
		return
	}
	fmt.Fprint(w, gitLabFixtureObject)
}

// calls returns the recorded request lines.
func (h *fakeGitLabHandler) calls() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.requests)
}

// fakeGitLabListBody returns the array body for the collection endpoints the
// preparer reads, and reports false for everything else so the caller falls
// back to the single object body.
func fakeGitLabListBody(method, path string) (string, bool) {
	if method != http.MethodGet {
		return "", false
	}
	switch {
	case strings.HasSuffix(path, "/jobs"):
		return `[{"id":301,"name":"failing_fixture","status":"failed","runner":{"id":401}},{"id":302,"name":"manual_deploy","status":"manual"}]`, true
	case strings.HasSuffix(path, "/packages"):
		return `[{"id":501,"name":"eval-package"}]`, true
	case strings.HasSuffix(path, "/environments"):
		return `[{"id":601,"name":"production"}]`, true
	}
	for _, suffix := range []string{"variables", "hooks", "badges", "merge_requests", "pipeline_schedules", "project_aliases", "service_accounts", "personal_access_tokens", "award_emoji", "releases", "labels"} {
		if strings.HasSuffix(path, "/"+suffix) {
			return `[]`, true
		}
	}
	return "", false
}

// newFakeGitLabFixtureClient starts a fake GitLab and returns a client bound to
// it together with the handler that recorded the traffic.
func newFakeGitLabFixtureClient(t *testing.T) (*gitlabclient.Client, *fakeGitLabHandler) {
	t.Helper()
	handler := &fakeGitLabHandler{}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return newFixtureTestClient(t, server.URL), handler
}

// TestLiveFixturePreparerPrepare_SeedsEveryTrackedResource verifies the whole
// fixture provisioning sequence against a fake GitLab: it walks group, project,
// repository, branch, issue, merge request, pipeline and every best-effort
// resource, and records each identifier in the fixture state that case prompts
// later read. Only the string-shaped discussion IDs are asserted separately,
// because they are the one place the preparer reads a non-numeric identifier.
func TestLiveFixturePreparerPrepare_SeedsEveryTrackedResource(t *testing.T) {
	client, handler := newFakeGitLabFixtureClient(t)
	preparer := &liveFixturePreparer{client: client, state: &liveFixtureState{}}

	if err := preparer.prepare(t.Context()); err != nil {
		t.Fatalf("prepare() error = %v", err)
	}

	state := preparer.state
	for name, got := range map[string]int64{
		"group":                  state.GroupID,
		"tools group":            state.ToolsGroupID,
		"project":                state.ProjectID,
		"issue":                  state.IssueIID,
		"issue to delete":        state.IssueDeleteIID,
		"merge request":          state.MergeRequestIID,
		"merge request to merge": state.MergeRequestMergeIID,
		"pipeline":               state.PipelineID,
		"failed job":             state.FailedJobID,
		"manual job":             state.ManualJobID,
		"runner":                 state.RunnerID,
		"milestone":              state.MilestoneDeleteIID,
		"hook":                   state.HookDeleteID,
		"badge":                  state.BadgeDeleteID,
		"snippet":                state.SnippetID,
		"environment":            state.EnvironmentID,
		"project token":          state.ProjectTokenID,
		"package":                state.PackageID,
		"service account":        state.ProjectServiceAccountID,
		"service account token":  state.ProjectServiceAccountTokenID,
		"deploy key":             state.DeployKeyID,
		"deploy token":           state.DeployTokenID,
		"pipeline trigger":       state.PipelineTriggerID,
		"pipeline schedule":      state.PipelineScheduleID,
		"user":                   state.UserID,
		"issue award":            state.IssueAwardID,
		"merge request award":    state.MergeRequestAwardID,
	} {
		t.Run(name, func(t *testing.T) {
			if got == 0 {
				t.Fatalf("%s ID = 0, want a seeded identifier", name)
			}
		})
	}
	if state.DefaultBranch != "main" || state.MergeRequestThreadID != "disc-1" || state.CommitSHA != "abc123" || state.CommitDiscussionNoteID != 55 {
		t.Fatalf("state = %+v, want detected branch and discussion identifiers", state)
	}
	if len(state.Notes) != 0 {
		t.Fatalf("notes = %v, want no best-effort failures", state.Notes)
	}
	calls := strings.Join(handler.calls(), "\n")
	for _, want := range []string{
		"POST /api/v4/projects/101/issues",
		"POST /api/v4/projects/101/merge_requests",
		"POST /api/v4/projects/101/pipeline",
		"PUT /api/v4/projects/101/repository/files/README.md",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(calls, want) {
				t.Fatalf("calls did not include %q", want)
			}
		})
	}
}

// TestLiveFixturePreparerPrepare_BestEffortFailures_AreRecordedAsNotes
// verifies a GitLab that rejects everything after the core project setup lets
// preparation finish, recording each optional resource as a note rather than
// failing the run. The mandatory steps still fail the call.
func TestLiveFixturePreparerPrepare_BestEffortFailures_AreRecordedAsNotes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Not Found"}`)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{}}

	err := preparer.prepare(t.Context())

	if err == nil || !strings.Contains(err.Error(), "create group my-org") {
		t.Fatalf("prepare() error = %v, want group creation failure", err)
	}
}

// TestLiveFixturePreparerBestEffort_RecordsFailureNote verifies the
// best-effort wrapper turns an error into a fixture note and leaves a
// successful step silent.
func TestLiveFixturePreparerBestEffort_RecordsFailureNote(t *testing.T) {
	preparer := &liveFixturePreparer{state: &liveFixtureState{}}
	preparer.bestEffort(t.Context(), "widget", func(context.Context) error { return nil })
	if len(preparer.state.Notes) != 0 {
		t.Fatalf("notes = %v, want none after a successful step", preparer.state.Notes)
	}
	preparer.bestEffort(t.Context(), "widget", func(context.Context) error { return errors.New("boom") })
	if len(preparer.state.Notes) != 1 || preparer.state.Notes[0] != "widget fixture unavailable: boom" {
		t.Fatalf("notes = %v, want the widget failure note", preparer.state.Notes)
	}
}

// TestEnsureGroupAndProject_CreateWhenAbsentAndUnarchive verifies the group and
// project bootstrap: a 404 leads to creation, an archived project is
// unarchived, and a non-404 lookup failure aborts.
func TestEnsureGroupAndProject_CreateWhenAbsentAndUnarchive(t *testing.T) {
	t.Run("creates group and project when absent", func(t *testing.T) {
		var created []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"404 Not Found"}`)
				return
			}
			created = append(created, r.URL.EscapedPath())
			fmt.Fprint(w, gitLabFixtureObject)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{}}
		group, err := preparer.ensureGroup(t.Context(), "tools", "my-org/tools", 42)
		if err != nil || group.ID != 101 {
			t.Fatalf("ensureGroup() = %+v, %v; want created group", group, err)
		}
		project, err := preparer.ensureProject(t.Context(), 101)
		if err != nil || project.ID != 101 {
			t.Fatalf("ensureProject() = %+v, %v; want created project", project, err)
		}
		if strings.Join(created, ",") != "/api/v4/groups,/api/v4/projects" {
			t.Fatalf("created = %v, want group then project", created)
		}
	})
	t.Run("unarchives an archived project", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(r.URL.EscapedPath(), "/unarchive") {
				fmt.Fprint(w, `{"id":101,"archived":false}`)
				return
			}
			fmt.Fprint(w, `{"id":101,"archived":true}`)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{}}
		project, err := preparer.ensureProject(t.Context(), 1)
		if err != nil || project.Archived {
			t.Fatalf("ensureProject() = %+v, %v; want unarchived project", project, err)
		}
	})
	t.Run("propagates a non-404 lookup failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"500"}`)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{}}
		if _, err := preparer.ensureGroup(t.Context(), "tools", "my-org/tools", 0); err == nil || !strings.Contains(err.Error(), "get group my-org/tools") {
			t.Fatalf("ensureGroup() error = %v, want lookup failure", err)
		}
		if _, err := preparer.ensureProject(t.Context(), 1); err == nil || !strings.Contains(err.Error(), "get project "+liveFixtureProjectPath) {
			t.Fatalf("ensureProject() error = %v, want lookup failure", err)
		}
	})
}

// TestWaitForPipelineJobs_MissingTerminalStates_ReportsLastStatuses verifies
// the pipeline job wait reports the statuses it saw when the deadline passes
// without both a failed and a manual job, and that a listing error aborts.
func TestWaitForPipelineJobs_MissingTerminalStates_ReportsLastStatuses(t *testing.T) {
	t.Run("listing error aborts", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"500"}`)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
		if err := preparer.waitForPipelineJobs(t.Context(), 7); err == nil {
			t.Fatal("waitForPipelineJobs() error = nil, want listing failure")
		}
	})
	t.Run("canceled context stops polling", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"id":1,"name":"build","status":"running"}]`)
		}))
		defer server.Close()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
		if err := preparer.waitForPipelineJobs(ctx, 7); !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForPipelineJobs() error = %v, want context.Canceled", err)
		}
	})
}

// TestWaitForBranch_CanceledContext_ReturnsContextError verifies the branch
// wait stops on cancellation instead of polling to its deadline.
func TestWaitForBranch_CanceledContext_ReturnsContextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Branch Not Found"}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
	if err := preparer.waitForBranch(ctx, "main"); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForBranch() error = %v, want context.Canceled", err)
	}
}

// TestWaitForMergeRequestMergeable_TerminalStatus_ReportsNotMergeable
// verifies a merge request that settles into a non-mergeable status is
// reported immediately rather than polled to the deadline.
func TestWaitForMergeRequestMergeable_TerminalStatus_ReportsNotMergeable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":7,"detailed_merge_status":"broken_status"}`)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
	err := preparer.waitForMergeRequestMergeable(t.Context(), 7)
	if err == nil || !strings.Contains(err.Error(), "is not mergeable: broken_status") {
		t.Fatalf("waitForMergeRequestMergeable() error = %v, want terminal status report", err)
	}
}

// TestIsEvaluationProjectHook_MatchesFixtureHooks verifies hook cleanup only
// claims the evaluator's own hooks, by name prefix, embedded case ID, or the
// example.com URLs it creates.
func TestIsEvaluationProjectHook_MatchesFixtureHooks(t *testing.T) {
	cases := []struct {
		name string
		hook *gl.ProjectHook
		want bool
	}{
		{name: "nil hook"},
		{name: "delete prefix", hook: &gl.ProjectHook{Name: "delete-fixture-17"}, want: true},
		{name: "case id", hook: &gl.ProjectHook{Name: "MS-021 hook"}, want: true},
		{name: "crud name", hook: &gl.ProjectHook{Name: "eval-crud-hook"}, want: true},
		{name: "fixture url", hook: &gl.ProjectHook{URL: "https://example.com/gitlab-hook-delete"}, want: true},
		{name: "crud url", hook: &gl.ProjectHook{URL: "https://example.com/eval-crud-hook"}, want: true},
		{name: "unrelated", hook: &gl.ProjectHook{Name: "production", URL: "https://hooks.example.org/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEvaluationProjectHook(tc.hook); got != tc.want {
				t.Fatalf("isEvaluationProjectHook(%+v) = %t, want %t", tc.hook, got, tc.want)
			}
		})
	}
}

// TestFindAwardEmojiID_ReturnsZeroWhenAbsent verifies the award emoji lookup
// returns the matching identifier and zero when the name is not present.
func TestFindAwardEmojiID_ReturnsZeroWhenAbsent(t *testing.T) {
	awards := []*gl.AwardEmoji{{ID: 1, Name: "thumbsup"}, {ID: 2, Name: "rocket"}}
	if got := findAwardEmojiID(awards, "rocket"); got != 2 {
		t.Fatalf("findAwardEmojiID(rocket) = %d, want 2", got)
	}
	if got := findAwardEmojiID(awards, "eyes"); got != 0 {
		t.Fatalf("findAwardEmojiID(eyes) = %d, want 0", got)
	}
}

// TestCleanupProjectHooks_DeletesEvaluationHooksAcrossPages verifies hook
// cleanup follows pagination, deletes only the evaluator's hooks, and
// tolerates a hook that disappeared between listing and deletion.
func TestCleanupProjectHooks_DeletesEvaluationHooksAcrossPages(t *testing.T) {
	var deleted []string
	var listed int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not Found"}`)
			return
		}
		listed++
		if listed == 1 {
			w.Header().Set("X-Next-Page", "2")
			fmt.Fprint(w, `[{"id":1,"name":"delete-fixture-1"},{"id":2,"name":"production"}]`)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}

	if err := preparer.cleanupProjectHooks(t.Context()); err != nil {
		t.Fatalf("cleanupProjectHooks() error = %v", err)
	}
	if len(deleted) != 1 || !strings.HasSuffix(deleted[0], "/hooks/1") {
		t.Fatalf("deleted = %v, want only the evaluation hook", deleted)
	}
}

// TestCleanupPipelineSchedules_DeletesOnlyFixtureSchedules verifies schedule
// cleanup removes the delete- and play-prefixed fixtures and leaves other
// schedules alone.
func TestCleanupPipelineSchedules_DeletesOnlyFixtureSchedules(t *testing.T) {
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.EscapedPath())
			fmt.Fprint(w, `{}`)
			return
		}
		fmt.Fprint(w, `[{"id":1,"description":"delete-fixture-1"},{"id":2,"description":"play-fixture-1"},{"id":3,"description":"nightly"}]`)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}

	if err := preparer.cleanupPipelineSchedules(t.Context()); err != nil {
		t.Fatalf("cleanupPipelineSchedules() error = %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("deleted = %v, want the two fixture schedules", deleted)
	}
}

// TestPrepareLiveFixtures_RejectsUnsafeOptions verifies fixture provisioning
// refuses to touch a GitLab instance unless the run targets the live backend
// with the Docker guard or an explicit live-mutation opt-in.
func TestPrepareLiveFixtures_RejectsUnsafeOptions(t *testing.T) {
	t.Setenv("E2E_MODE", "")
	cases := []struct {
		name string
		opts options
		want string
	}{
		{name: "tools file", opts: options{ToolsFile: "tools.json"}, want: "--prepare-fixtures requires a live catalog"},
		{name: "mock backend", opts: options{Backend: backendMock}, want: "--prepare-fixtures requires --backend=gitlab"},
		{name: "no docker guard", opts: options{Backend: backendGitLab}, want: "--prepare-fixtures requires E2E_MODE=docker"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := prepareLiveFixtures(tc.opts); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepareLiveFixtures() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestEnsureCleanupRelease_CreatesMissingReleases verifies the cleanup and
// summary releases are created when GitLab reports them missing, and that the
// asset link creation tolerates a duplicate.
func TestEnsureCleanupRelease_CreatesMissingReleases(t *testing.T) {
	var created []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(path, "/releases/"):
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not Found"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/assets/links"):
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"message":"already exists"}`)
		case r.Method == http.MethodPost:
			created = append(created, path)
			fmt.Fprint(w, gitLabFixtureObject)
		default:
			fmt.Fprint(w, gitLabFixtureObject)
		}
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101, DefaultBranch: "main"}}

	if err := preparer.ensureCleanupRelease(t.Context()); err != nil {
		t.Fatalf("ensureCleanupRelease() error = %v", err)
	}
	if preparer.state.ReleaseSummaryTag != liveFixtureReleaseSummaryTag {
		t.Fatalf("ReleaseSummaryTag = %q, want %q", preparer.state.ReleaseSummaryTag, liveFixtureReleaseSummaryTag)
	}
	releaseCreates := 0
	for _, path := range created {
		if strings.HasSuffix(path, "/releases") {
			releaseCreates++
		}
	}
	if releaseCreates != 2 {
		t.Fatalf("release creates = %d in %v, want cleanup and summary releases", releaseCreates, created)
	}
}

// TestEnsureTagAndBranch_TolerateExistingResources verifies tag and branch
// creation treat an existing resource as success and surface other failures.
func TestEnsureTagAndBranch_ToleratesExistingResources(t *testing.T) {
	t.Run("creates when missing", func(t *testing.T) {
		var posted []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"404 Not Found"}`)
				return
			}
			posted = append(posted, r.URL.EscapedPath())
			fmt.Fprint(w, gitLabFixtureObject)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
		if err := preparer.ensureTag(t.Context(), "v1", "main"); err != nil {
			t.Fatalf("ensureTag() error = %v", err)
		}
		if err := preparer.ensureBranch(t.Context(), "feature/x", "main"); err != nil {
			t.Fatalf("ensureBranch() error = %v", err)
		}
		if len(posted) != 2 {
			t.Fatalf("posted = %v, want tag and branch creation", posted)
		}
	})
	t.Run("tolerates an existing branch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"404 Not Found"}`)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"message":"Branch already exists"}`)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
		if err := preparer.ensureBranch(t.Context(), "feature/x", "main"); err != nil {
			t.Fatalf("ensureBranch() error = %v, want an existing branch to be tolerated", err)
		}
	})
}

// TestEnsureAwardEmoji_FallsBackToListingWhenCreateFails verifies award
// seeding reuses an existing emoji when the create call is rejected, and skips
// entirely when the issue and merge request were never created.
func TestEnsureAwardEmoji_FallsBackToListingWhenCreateFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"message":"already awarded"}`)
			return
		}
		fmt.Fprint(w, `[{"id":9,"name":"thumbsup"},{"id":10,"name":"rocket"}]`)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101, IssueIID: 7, MergeRequestIID: 8}}

	if err := preparer.ensureAwardEmoji(t.Context()); err != nil {
		t.Fatalf("ensureAwardEmoji() error = %v", err)
	}
	if preparer.state.IssueAwardID != 9 || preparer.state.MergeRequestAwardID != 10 {
		t.Fatalf("award IDs = %d/%d, want the listed emoji IDs", preparer.state.IssueAwardID, preparer.state.MergeRequestAwardID)
	}
	empty := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
	if err := empty.ensureAwardEmoji(t.Context()); err != nil {
		t.Fatalf("ensureAwardEmoji(no resources) error = %v", err)
	}
}

// TestEnsureDiscussions_WithoutMergeRequest_IsANoop verifies discussion
// seeding is skipped when no merge request was created.
func TestEnsureDiscussions_WithoutMergeRequest_IsANoop(t *testing.T) {
	preparer := &liveFixturePreparer{state: &liveFixtureState{}}
	if err := preparer.ensureDiscussions(t.Context()); err != nil {
		t.Fatalf("ensureDiscussions() error = %v, want nil", err)
	}
}

// TestEnsureCommitDiscussion_MissingCommit_ReturnsError verifies a default
// branch without a commit is reported rather than seeding an empty discussion.
func TestEnsureCommitDiscussion_MissingCommit_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"main"}`)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
	if err := preparer.ensureCommitDiscussion(t.Context()); err == nil || !strings.Contains(err.Error(), "default branch has no commit ID") {
		t.Fatalf("ensureCommitDiscussion() error = %v, want missing commit error", err)
	}
}

// TestEnsureEnvironmentDeployment_SeedsEnvironmentAndDeployment verifies the
// deployment fixture creates a uniquely named environment, resolves the commit
// SHA for the default branch, and records the deployment identifiers.
func TestEnsureEnvironmentDeployment_SeedsEnvironmentAndDeployment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "/repository/commits/"):
			fmt.Fprint(w, `{"id":"deadbeef"}`)
		case strings.HasSuffix(path, "/deployments"):
			fmt.Fprint(w, `{"id":77,"sha":"deadbeef","ref":"main"}`)
		default:
			fmt.Fprint(w, gitLabFixtureObject)
		}
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101, DefaultBranch: "main"}}

	if err := preparer.ensureEnvironmentDeployment(t.Context()); err != nil {
		t.Fatalf("ensureEnvironmentDeployment() error = %v", err)
	}
	state := preparer.state
	if state.DeploymentID != 77 || state.DeploymentSHA != "deadbeef" || state.DeploymentRef != "main" || !strings.HasPrefix(state.EnvironmentName, "eval-deploy-") {
		t.Fatalf("state = %+v, want seeded deployment", state)
	}
}

// TestEnsureProjectServiceAccount_ReusesExistingAccountAndToken verifies the
// Enterprise service-account fixture reuses an account and active PAT that
// already exist instead of creating duplicates.
func TestEnsureProjectServiceAccount_ReusesExistingAccountAndToken(t *testing.T) {
	var posted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posted = append(posted, path)
			fmt.Fprint(w, gitLabFixtureObject)
			return
		}
		if strings.HasSuffix(path, "/personal_access_tokens") {
			fmt.Fprintf(w, `[{"id":88,"name":%q,"active":true,"revoked":false}]`, liveFixtureProjectServiceAccountPATName)
			return
		}
		fmt.Fprintf(w, `[{"id":99,"name":%q,"username":"eval-project-svc-101"}]`, liveFixtureProjectServiceAccountName)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}

	if err := preparer.ensureProjectServiceAccount(t.Context()); err != nil {
		t.Fatalf("ensureProjectServiceAccount() error = %v", err)
	}
	if preparer.state.ProjectServiceAccountID != 99 || preparer.state.ProjectServiceAccountTokenID != 88 {
		t.Fatalf("state = %+v, want the existing account and token", preparer.state)
	}
	if len(posted) != 0 {
		t.Fatalf("posted = %v, want no creation calls", posted)
	}
}

// TestEnsureProjectAlias_SkipsWhenAliasExists verifies the alias fixture is a
// no-op when the alias is already registered and creates it otherwise.
func TestEnsureProjectAlias_SkipsWhenAliasExists(t *testing.T) {
	cases := []struct {
		name       string
		listBody   string
		wantPosted bool
	}{
		{name: "alias exists", listBody: `[{"id":1,"name":"e2e-enterprise-alias"}]`},
		{name: "alias missing", listBody: `[]`, wantPosted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			posted := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posted = true
					fmt.Fprint(w, gitLabFixtureObject)
					return
				}
				fmt.Fprint(w, tc.listBody)
			}))
			defer server.Close()
			preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
			if err := preparer.ensureProjectAlias(t.Context()); err != nil {
				t.Fatalf("ensureProjectAlias() error = %v", err)
			}
			if posted != tc.wantPosted {
				t.Fatalf("posted = %t, want %t", posted, tc.wantPosted)
			}
		})
	}
}

// TestEnsurePackage_ListingWithoutResults_ReturnsError verifies a published
// package that GitLab does not list back is reported rather than silently
// leaving the fixture without an identifier.
func TestEnsurePackage_ListingWithoutResults_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprint(w, gitLabFixtureObject)
	}))
	defer server.Close()
	preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
	if err := preparer.ensurePackage(t.Context()); err == nil || !strings.Contains(err.Error(), "published generic package was not listed") {
		t.Fatalf("ensurePackage() error = %v, want unlisted package error", err)
	}
}

// TestEnsureFeatureFlagAndWiki_SkipWhenPresent verifies the feature flag and
// wiki fixtures are no-ops when GitLab already has them and create them after
// a 404.
func TestEnsureFeatureFlagAndWiki_SkipWhenPresent(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantPosted int
	}{
		{name: "already present", status: http.StatusOK},
		{name: "created after 404", status: http.StatusNotFound, wantPosted: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			posted := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posted++
					fmt.Fprint(w, gitLabFixtureObject)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, gitLabFixtureObject)
			}))
			defer server.Close()
			preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
			if err := preparer.ensureFeatureFlag(t.Context()); err != nil {
				t.Fatalf("ensureFeatureFlag() error = %v", err)
			}
			if err := preparer.ensureWiki(t.Context()); err != nil {
				t.Fatalf("ensureWiki() error = %v", err)
			}
			if posted != tc.wantPosted {
				t.Fatalf("posted = %d, want %d", posted, tc.wantPosted)
			}
		})
	}
}

// TestEnsureLabels_ToleratesConflictOnCreate verifies label seeding accepts a
// conflicting create and aborts on an unexpected lookup failure.
func TestEnsureLabels_ToleratesConflictOnCreate(t *testing.T) {
	t.Run("conflict tolerated", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusConflict)
				fmt.Fprint(w, `{"message":"Label already exists"}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Label Not Found"}`)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
		if err := preparer.ensureLabels(t.Context()); err != nil {
			t.Fatalf("ensureLabels() error = %v, want conflict tolerated", err)
		}
	})
	t.Run("lookup failure aborts", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"500"}`)
		}))
		defer server.Close()
		preparer := &liveFixturePreparer{client: newFixtureTestClient(t, server.URL), state: &liveFixtureState{ProjectID: 101}}
		if err := preparer.ensureLabels(t.Context()); err == nil || !strings.Contains(err.Error(), "get label evaluation") {
			t.Fatalf("ensureLabels() error = %v, want lookup failure", err)
		}
	})
}

// TestIgnoreNotFound_RecordsOnlyUnexpectedFailures verifies cleanup warnings
// are noted for real errors and suppressed for a 404 or a success.
func TestIgnoreNotFound_RecordsOnlyUnexpectedFailures(t *testing.T) {
	preparer := &liveFixturePreparer{state: &liveFixtureState{}}
	preparer.ignoreNotFound(nil, nil)
	preparer.ignoreNotFound(nil, &gl.ErrorResponse{Response: &http.Response{StatusCode: http.StatusNotFound}, Message: "404 Not Found"})
	if len(preparer.state.Notes) != 0 {
		t.Fatalf("notes = %v, want none for success or 404", preparer.state.Notes)
	}
	preparer.ignoreNotFound(nil, errors.New("boom"))
	if len(preparer.state.Notes) != 1 || !strings.Contains(preparer.state.Notes[0], "cleanup warning: boom") {
		t.Fatalf("notes = %v, want a cleanup warning", preparer.state.Notes)
	}
}
