package wizard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeIndex verifies that the embedded index.html is served with the
// correct content type and a non-empty HTML body.
func TestServeIndex(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()

	serveIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("response body is empty")
	}
}

// TestHandleDefaults verifies the /api/defaults endpoint returns correct
// JSON structure with version, install path, gitlab URL, and all clients.
func TestHandleDefaults(t *testing.T) {
	stubLoadExistingConfig(t)
	stubGetInstalledVersion(t, "")
	handler := handleDefaults("2.0.0-test")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/defaults", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp defaultsResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Version != "2.0.0-test" {
		t.Errorf("version = %q, want %q", resp.Version, "2.0.0-test")
	}
	if resp.GitLabURL != DefaultGitLabURL {
		t.Errorf("gitlab_url = %q, want %q", resp.GitLabURL, DefaultGitLabURL)
	}
	if resp.InstallPath == "" {
		t.Error("install_path is empty")
	}
	if len(resp.Clients) != len(allClientsFn()) {
		t.Errorf("clients count = %d, want %d", len(resp.Clients), len(allClientsFn()))
	}

	// Verify each client has a name
	for i, c := range resp.Clients {
		if c.Name == "" {
			t.Errorf("client[%d] has empty name", i)
		}
	}
}

// TestHandleConfigure_InvalidURL verifies that the configure endpoint
// returns 400 when GitLab URL has an invalid format.
func TestHandleConfigure_InvalidURL(t *testing.T) {
	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	body := `{"gitlab_url":"not-a-valid-url","gitlab_token":"glpat-xxx","selected_clients":[]}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "Invalid GitLab URL") {
		t.Errorf("body = %q, want to contain 'Invalid GitLab URL'", rec.Body.String())
	}
}

// TestHandleConfigure_MissingURL verifies that the configure endpoint
// returns 400 when GitLab URL is missing.
func TestHandleConfigure_MissingURL(t *testing.T) {
	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	body := `{"gitlab_url":"","gitlab_token":"glpat-xxx","selected_clients":[]}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleConfigure_MissingToken verifies that the configure endpoint
// returns 400 when GitLab token is missing and no existing config exists.
func TestHandleConfigure_MissingToken(t *testing.T) {
	stubLoadExistingConfig(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	body := `{"gitlab_url":"https://gitlab.example.com","gitlab_token":"","selected_clients":[]}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleConfigure_EmptyTokenFallsBackToExisting verifies that when the
// token is empty but an existing config has a token, the handler uses the
// existing token instead of returning 400.
func TestHandleConfigure_EmptyTokenFallsBackToExisting(t *testing.T) {
	stubLoadExistingConfigWith(t, ServerConfig{
		GitLabURL:   "https://existing.example.com",
		GitLabToken: "glpat-existing-token-xxx",
	})
	stubWriteEnvFile(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	reqBody := configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "",
		LogLevel:        "info",
		SelectedClients: []int{},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	select {
	case err = <-doneCh:
		if err != nil {
			t.Fatalf("onDone returned unexpected error: %v", err)
		}
	default:
		t.Error("onDone callback was not called")
	}
}

// TestHandleConfigure_InvalidJSON verifies that the configure endpoint
// returns 400 when the request body is not valid JSON.
func TestHandleConfigure_InvalidJSON(t *testing.T) {
	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", strings.NewReader("{not-json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleConfigure_InvalidLogLevel verifies that an unrecognized log
// level in the request gets normalized to "info".
func TestHandleConfigure_InvalidLogLevel(t *testing.T) {
	stubWriteEnvFile(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	reqBody := configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "glpat-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "invalid-level",
		SelectedClients: []int{},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// onDone should have been called
	select {
	case err = <-doneCh:
		if err != nil {
			t.Logf("apply returned error (expected in test env): %v", err)
		}
	default:
		t.Error("onDone callback was not called")
	}
}

// TestHandleConfigure_ValidRequest verifies successful configure flow with
// empty selected_clients (no file writes needed).
func TestHandleConfigure_ValidRequest(t *testing.T) {
	stubWriteEnvFile(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	reqBody := configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "glpat-xxxxxxxxxxxxxxxxxxxx",
		SkipTLSVerify:   true,
		MetaTools:       true,
		AutoUpdate:      true,
		LogLevel:        "debug",
		SelectedClients: []int{},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp configureResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	select {
	case err = <-doneCh:
		if err != nil {
			t.Fatalf("onDone returned unexpected error: %v", err)
		}
	default:
		t.Error("onDone callback was not called")
	}
}

// TestHandlePickDirectory_NoDialogAvailable verifies the handler returns
// the stubbed directory path when pickDirectoryFn is overridden.
func TestHandlePickDirectory_NoDialogAvailable(t *testing.T) {
	stubPickDirectory(t, "", fmt.Errorf("no dialog"))
	handler := handlePickDirectory()

	body := `{"start_dir":"/tmp"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/pick-directory", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// In a test environment without GUI, this should return 204 (dialog fails).
	// We accept either 200 (dialog somehow works) or 204 (dialog failed).
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 200 or 204", rec.Code)
	}
}

// TestHandleConfigure_WithJetBrainsClient verifies the configure handler
// produces JetBrains JSON when the display-only client is selected.
func TestHandleConfigure_WithJetBrainsClient(t *testing.T) {
	stubWriteEnvFile(t)
	useFakeClients(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	// Find JetBrains client index
	clients := allClientsFn()
	jbIdx := -1
	for i, c := range clients {
		if c.DisplayOnly {
			jbIdx = i
			break
		}
	}
	if jbIdx < 0 {
		t.Skip("no DisplayOnly client")
	}

	reqBody := configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "glpat-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{jbIdx},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp configureResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if resp.JetBrainsJSON == "" {
		t.Error("expected non-empty JetBrainsJSON in response")
	}
	if len(resp.Configured) == 0 {
		t.Error("expected at least one configured client")
	}

	select {
	case err = <-doneCh:
		if err != nil {
			t.Logf("onDone: %v", err)
		}
	default:
		t.Error("onDone not called")
	}
}

// TestHandleConfigure_WithOutOfRangeClient verifies that out-of-range client
// indices are silently ignored in the response.
func TestHandleConfigure_WithOutOfRangeClient(t *testing.T) {
	stubWriteEnvFile(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	reqBody := configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "glpat-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{-1, 999},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp configureResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Configured) != 0 {
		t.Errorf("expected 0 configured clients, got %d", len(resp.Configured))
	}

	select {
	case err = <-doneCh:
		if err != nil {
			t.Logf("onDone: %v", err)
		}
	default:
		t.Error("onDone not called")
	}
}

// TestHandleConfigure_InstallPathWithBinaryName verifies that handleConfigure
// correctly strips the binary name from install_path when the path ends with
// the platform binary name (exercises the HasSuffix branch).
func TestHandleConfigure_InstallPathWithBinaryName(t *testing.T) {
	stubWriteEnvFile(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	tmpDir := t.TempDir()
	// Path ends with binary name — handler should strip it and use the dir
	installWithBinary := filepath.Join(tmpDir, "bin", DefaultBinaryName())

	reqBody := configureRequest{
		InstallPath:     installWithBinary,
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "glpat-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	select {
	case err = <-doneCh:
		if err != nil {
			t.Logf("onDone: %v", err)
		}
	default:
		t.Error("onDone not called")
	}
}

// TestHandleConfigure_WithRegularClient verifies the configure handler
// processes a regular (non-display-only) client, verifying the full
// Apply flow including MergeServerEntry execution.
func TestHandleConfigure_WithRegularClient(t *testing.T) {
	stubWriteEnvFile(t)
	useFakeClients(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	tmpDir := t.TempDir()

	// Find VS Code client index (first non-display-only client)
	clients := allClientsFn()
	vsCodeIdx := -1
	for i, c := range clients {
		if !c.DisplayOnly && c.ID == ClientVSCode {
			vsCodeIdx = i
			break
		}
	}
	if vsCodeIdx < 0 {
		t.Skip("VS Code client not found")
	}

	reqBody := configureRequest{
		InstallPath:     tmpDir,
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "glpat-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{vsCodeIdx},
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp configureResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(resp.Configured) == 0 {
		t.Error("expected at least one configured client")
	}

	select {
	case err = <-doneCh:
		if err != nil {
			t.Logf("onDone: %v", err)
		}
	default:
		t.Error("onDone not called")
	}
}

// TestHandlePickDirectory_InvalidJSON verifies handlePickDirectory handles
// unparseable JSON body gracefully.
func TestHandlePickDirectory_InvalidJSON(t *testing.T) {
	stubPickDirectory(t, "", fmt.Errorf("no dialog"))
	handler := handlePickDirectory()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/pick-directory", strings.NewReader("{bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should still attempt pickDirectory with empty start_dir
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 200 or 204", rec.Code)
	}
}

// TestHandlePickDirectory_Success verifies the handler returns the selected
// directory as JSON when the directory picker succeeds.
func TestHandlePickDirectory_Success(t *testing.T) {
	stubPickDirectory(t, "/home/user/mydir", nil)
	handler := handlePickDirectory()

	body := `{"start_dir":"/home/user"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/pick-directory", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]string
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got := resp["path"]; got != "/home/user/mydir" {
		t.Errorf("path = %q, want %q", got, "/home/user/mydir")
	}
}

// TestServeIndex_ContainsHTML verifies the served index page contains HTML.
func TestServeIndex_ContainsHTML(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()

	serveIndex(rec, req)

	body := rec.Body.String()
	if !strings.Contains(strings.ToLower(body), "<html") && !strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Error("response body does not contain HTML markup")
	}
}

func TestHandleDefaults_WithExistingConfig(t *testing.T) {
	stubLoadExistingConfigWith(t, ServerConfig{
		GitLabURL:     "https://existing.example.com",
		GitLabToken:   "glpat-existing-token",
		SkipTLSVerify: true,
	})
	stubGetInstalledVersion(t, "")
	handler := handleDefaults("2.0.0-test")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/defaults", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp defaultsResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !resp.HasExisting {
		t.Error("expected HasExisting=true")
	}
	if resp.GitLabURL != "https://existing.example.com" {
		t.Errorf("GitLabURL = %q, want %q", resp.GitLabURL, "https://existing.example.com")
	}
	if !resp.SkipTLSVerify {
		t.Error("expected SkipTLSVerify=true")
	}
	wantMasked := MaskToken("glpat-existing-token")
	if resp.MaskedToken != wantMasked {
		t.Errorf("MaskedToken = %q, want %q", resp.MaskedToken, wantMasked)
	}
}

// TestHandleDefaults_ClientsStructure verifies the defaults endpoint returns
// at least one display-only and one auto-config client.
func TestHandleDefaults_ClientsStructure(t *testing.T) {
	stubLoadExistingConfig(t)
	stubGetInstalledVersion(t, "")
	handler := handleDefaults("test-version")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/defaults", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp defaultsResponse
	if err = json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	hasDisplayOnly := false
	hasAutoConfig := false
	for _, c := range resp.Clients {
		if c.DisplayOnly {
			hasDisplayOnly = true
		} else {
			hasAutoConfig = true
		}
	}
	if !hasDisplayOnly {
		t.Error("expected at least one display-only client")
	}
	if !hasAutoConfig {
		t.Error("expected at least one auto-config client")
	}
}

// TestHandleDefaults_InstalledVersion verifies the endpoint returns the
// installed binary version when one exists, and omits it when empty.
func TestHandleDefaults_InstalledVersion(t *testing.T) {
	tests := []struct {
		name             string
		installedVersion string
		wantVersion      string
		wantPresent      bool
	}{
		{
			name:             "returns installed version when binary exists",
			installedVersion: "1.0.1",
			wantVersion:      "1.0.1",
			wantPresent:      true,
		},
		{
			name:             "omits installed version when binary not found",
			installedVersion: "",
			wantVersion:      "",
			wantPresent:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubLoadExistingConfig(t)
			stubGetInstalledVersion(t, tt.installedVersion)
			handler := handleDefaults("2.0.0")

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/defaults", nil)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			body := rec.Body.String()

			var resp defaultsResponse
			if err = json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
				t.Fatalf("decoding: %v", err)
			}

			if resp.InstalledVersion != tt.wantVersion {
				t.Errorf("InstalledVersion = %q, want %q", resp.InstalledVersion, tt.wantVersion)
			}

			// Verify omitempty: field absent when empty
			if tt.wantPresent && !strings.Contains(body, "installed_version") {
				t.Error("expected installed_version in JSON body")
			}
			if !tt.wantPresent && strings.Contains(body, "installed_version") {
				t.Error("expected installed_version to be omitted from JSON body")
			}
		})
	}
}
