package evaluator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		if !created[scope] {
			t.Fatalf("%s variable was not recreated", scope)
		}
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
