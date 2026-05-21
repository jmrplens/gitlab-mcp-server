// webui_test.go contains unit tests for the web UI wizard HTTP handlers
// and embedded asset serving.
package wizard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// jsonTagsFromStruct extracts all JSON field names from a struct type using
// reflection. It strips tag options such as ",omitempty".
func jsonTagsFromStruct(t *testing.T, v any) map[string]bool {
	t.Helper()
	rt := reflect.TypeOf(v)
	tags := make(map[string]bool)
	for f := range rt.Fields() {
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		tags[name] = true
	}
	return tags
}

// loadEmbeddedHTML returns the embedded index.html content as a string.
func loadEmbeddedHTML(t *testing.T) string {
	t.Helper()
	data, err := webAssets.ReadFile("webui_assets/index.html")
	if err != nil {
		t.Fatalf("reading embedded HTML: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("embedded HTML is empty")
	}
	return string(data)
}

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

	body := `{"gitlab_url":"not-a-valid-url","gitlab_token":"test-token-xxx","selected_clients":[]}`
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
	if !strings.Contains(rec.Body.String(), "invalid GitLab URL") {
		t.Errorf("body = %q, want to contain 'invalid GitLab URL'", rec.Body.String())
	}
}

// TestHandleConfigure_DefaultsEmptyURL verifies that an empty GitLab URL uses
// the GitLab.com default.
func TestHandleConfigure_DefaultsEmptyURL(t *testing.T) {
	tests := []struct {
		name      string
		gitLabURL string
	}{
		{name: "empty", gitLabURL: ""},
		{name: "whitespace", gitLabURL: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubLoadExistingConfig(t)
			stubInstallBinary(t)
			envPath := stubWriteEnvFile(t)

			var output bytes.Buffer
			doneCh := make(chan error, 1)
			handler := handleConfigure(&output, func(err error) { doneCh <- err })

			reqBody := configureRequest{
				InstallPath:     t.TempDir(),
				GitLabURL:       tt.gitLabURL,
				GitLabToken:     "test-token-xxx",
				LogLevel:        "info",
				SelectedClients: []int{},
			}
			body, mErr := json.Marshal(reqBody)
			if mErr != nil {
				t.Fatalf("marshal request: %v", mErr)
			}
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
			data, readErr := os.ReadFile(envPath)
			if readErr != nil {
				t.Fatalf("read env file: %v", readErr)
			}
			if !strings.Contains(string(data), "GITLAB_URL="+DefaultGitLabURL) {
				t.Fatalf("env file = %q, want default GitLab URL", string(data))
			}
		})
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
		GitLabToken: "test-token-existing-token-xxx",
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
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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
		GitLabToken:     "test-token-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "invalid-level",
		SelectedClients: []int{},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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
	envPath := stubWriteEnvFile(t)

	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	reqBody := configureRequest{
		InstallPath:       t.TempDir(),
		GitLabURL:         "https://gitlab.example.com",
		GitLabToken:       "test-token-xxxxxxxxxxxxxxxxxxxx",
		SkipTLSVerify:     true,
		ToolSurface:       "dynamic",
		CapabilitySurface: "minimal",
		MetaParamSchema:   "compact",
		Enterprise:        true,
		ReadOnly:          true,
		SafeMode:          true,
		EmbeddedResources: false,
		ExcludeTools:      "gitlab_admin",
		IgnoreScopes:      true,
		UploadMaxFileSize: "500MB",
		AutoUpdate:        true,
		AutoUpdateMode:    "check",
		AutoUpdateRepo:    "example/repo",
		AutoUpdateTimeout: "90s",
		RateLimitRPS:      "3.5",
		RateLimitBurst:    "12",
		LogLevel:          "debug",
		SelectedClients:   []int{},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading generated env file: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"TOOL_SURFACE=dynamic",
		"CAPABILITY_SURFACE=minimal",
		"META_PARAM_SCHEMA=compact",
		"GITLAB_ENTERPRISE=true",
		"GITLAB_READ_ONLY=true",
		"GITLAB_SAFE_MODE=true",
		"EMBEDDED_RESOURCES=false",
		"EXCLUDE_TOOLS=gitlab_admin",
		"AUTO_UPDATE=check",
		"RATE_LIMIT_RPS=3.5",
	} {
		if !strings.Contains(content, want+"\n") {
			t.Errorf("generated env file missing %q\ncontent:\n%s", want, content)
		}
	}
	if strings.Contains(content, "META_TOOLS=") {
		t.Fatalf("generated env file should not write deprecated META_TOOLS\ncontent:\n%s", content)
	}
}

// TestHandleConfigure_DerivesMetaToolsFromToolSurface verifies meta-tools are
// derived from tool_surface server-side instead of trusting a client flag.
func TestHandleConfigure_DerivesMetaToolsFromToolSurface(t *testing.T) {
	tests := []struct {
		name        string
		toolSurface string
		wantLine    string
	}{
		{name: "individual writes explicit tool surface", toolSurface: "individual", wantLine: "TOOL_SURFACE=individual"},
		{name: "dynamic writes explicit tool surface", toolSurface: "dynamic", wantLine: "TOOL_SURFACE=dynamic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertConfigureToolSurface(t, tt.toolSurface, tt.wantLine)
		})
	}
}

func assertConfigureToolSurface(t *testing.T, toolSurface, wantLine string) {
	t.Helper()
	envPath := stubWriteEnvFile(t)
	var output bytes.Buffer
	doneCh := make(chan error, 1)
	handler := handleConfigure(&output, func(err error) { doneCh <- err })

	req := newConfigureRequest(t, configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "test-token-placeholder",
		ToolSurface:     toolSurface,
		LogLevel:        "info",
		SelectedClients: []int{},
	})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assertConfigureSucceeded(t, rec, doneCh)
	assertEnvToolSurface(t, envPath, wantLine)
}

func newConfigureRequest(t *testing.T, reqBody configureRequest) *http.Request {
	t.Helper()
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertConfigureSucceeded(t *testing.T, rec *httptest.ResponseRecorder, doneCh <-chan error) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("onDone returned unexpected error: %v", err)
		}
	default:
		t.Fatal("onDone callback was not called")
	}
}

func assertEnvToolSurface(t *testing.T, envPath, wantLine string) {
	t.Helper()
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("reading generated env file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, wantLine+"\n") {
		t.Fatalf("generated env file missing %q\ncontent:\n%s", wantLine, content)
	}
	if strings.Contains(content, "META_TOOLS=") {
		t.Fatalf("generated env file should not write deprecated META_TOOLS\ncontent:\n%s", content)
	}
}

// TestNormalizeConfigureRequest_RejectsInvalidAdvancedOptions verifies the
// web API validates advanced configuration values before persisting them.
func TestNormalizeConfigureRequest_RejectsInvalidAdvancedOptions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*configureRequest)
		want string
	}{
		{name: "tool surface", edit: func(req *configureRequest) { req.ToolSurface = "unknown" }, want: "invalid tool_surface"},
		{name: "capability surface", edit: func(req *configureRequest) { req.CapabilitySurface = "tiny" }, want: "invalid capability_surface"},
		{name: "meta param schema", edit: func(req *configureRequest) { req.MetaParamSchema = "verbose" }, want: "invalid meta_param_schema"},
		{name: "auto update mode", edit: func(req *configureRequest) { req.AutoUpdateMode = "sometimes" }, want: "invalid auto_update_mode"},
		{name: "rate limit rps", edit: func(req *configureRequest) { req.RateLimitRPS = "fast" }, want: "invalid rate_limit_rps"},
		{name: "negative rate limit rps", edit: func(req *configureRequest) { req.RateLimitRPS = "-1" }, want: "invalid rate_limit_rps"},
		{name: "rate limit burst", edit: func(req *configureRequest) { req.RateLimitBurst = "many" }, want: "invalid rate_limit_burst"},
		{name: "negative rate limit burst", edit: func(req *configureRequest) { req.RateLimitBurst = "-1" }, want: "invalid rate_limit_burst"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := configureRequest{
				ToolSurface:       "dynamic",
				CapabilitySurface: "minimal",
				MetaParamSchema:   "compact",
				AutoUpdateMode:    "check",
				RateLimitRPS:      "3.5",
				RateLimitBurst:    "12",
				LogLevel:          "debug",
			}
			tt.edit(&req)

			err := normalizeConfigureRequest(&req)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestHandleConfigure_InvalidAdvancedOption verifies validation errors from
// advanced Web UI fields are returned as HTTP 400 responses.
func TestHandleConfigure_InvalidAdvancedOption(t *testing.T) {
	var output bytes.Buffer
	handler := handleConfigure(&output, func(error) {})
	reqBody := configureRequest{
		InstallPath:     t.TempDir(),
		GitLabURL:       "https://gitlab.example.com",
		GitLabToken:     "test-token-placeholder",
		ToolSurface:     "not-valid",
		LogLevel:        "info",
		SelectedClients: []int{},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/configure", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "invalid tool_surface") {
		t.Fatalf("body = %q, want invalid tool_surface", rec.Body.String())
	}
}

// TestNormalizeConfigureRequest_DefaultsAndLogLevel verifies blank advanced
// fields receive defaults and invalid log levels are normalized to info.
func TestNormalizeConfigureRequest_DefaultsAndLogLevel(t *testing.T) {
	req := configureRequest{LogLevel: "verbose"}
	if err := normalizeConfigureRequest(&req); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if req.ToolSurface != "dynamic" || req.CapabilitySurface != "full" || req.MetaParamSchema != "opaque" {
		t.Errorf("catalog defaults not applied: %#v", req)
	}
	if req.AutoUpdateMode != "true" || !req.AutoUpdate {
		t.Errorf("auto-update defaults not applied: %#v", req)
	}
	if req.RateLimitRPS != "0" || req.RateLimitBurst != "40" {
		t.Errorf("rate-limit defaults not applied: %#v", req)
	}
	if req.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", req.LogLevel)
	}
}

// TestHandlePickDirectory_NoDialogAvailable verifies the handler returns
// the stubbed directory path when pickDirectoryFn is overridden.
func TestHandlePickDirectory_NoDialogAvailable(t *testing.T) {
	stubPickDirectory(t, "", errors.New("no dialog"))
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
		GitLabToken:     "test-token-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{jbIdx},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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
		GitLabToken:     "test-token-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{-1, 999},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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
		GitLabToken:     "test-token-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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
		GitLabToken:     "test-token-xxxxxxxxxxxxxxxxxxxx",
		LogLevel:        "info",
		SelectedClients: []int{vsCodeIdx},
	}
	body, mErr := json.Marshal(reqBody)
	if mErr != nil {
		t.Fatalf("marshal request: %v", mErr)
	}
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
	stubPickDirectory(t, "", errors.New("no dialog"))
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

// TestHandleDefaults_WithExistingConfig verifies the /api/defaults handler
// returns HasExisting=true, the saved GitLab URL, SkipTLSVerify flag, and a
// masked token when a previous configuration is loaded.
func TestHandleDefaults_WithExistingConfig(t *testing.T) {
	stubLoadExistingConfigWith(t, ServerConfig{
		GitLabURL:         "https://existing.example.com",
		GitLabToken:       "test-token-existing-token",
		SkipTLSVerify:     true,
		ToolSurface:       "dynamic",
		CapabilitySurface: "minimal",
		MetaParamSchema:   "compact",
		Enterprise:        true,
		ReadOnly:          true,
		SafeMode:          true,
		EmbeddedResources: false,
		ExcludeTools:      "gitlab_admin",
		IgnoreScopes:      true,
		UploadMaxFileSize: "500MB",
		AutoUpdateMode:    "check",
		AutoUpdateRepo:    "example/repo",
		AutoUpdateTimeout: "90s",
		RateLimitRPS:      "3.5",
		RateLimitBurst:    "12",
		LogLevel:          "debug",
		YoloMode:          true,
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
	wantMasked := MaskToken("test-token-existing-token")
	if resp.MaskedToken != wantMasked {
		t.Errorf("MaskedToken = %q, want %q", resp.MaskedToken, wantMasked)
	}
	if resp.ToolSurface != "dynamic" || resp.CapabilitySurface != "minimal" || resp.MetaParamSchema != "compact" {
		t.Errorf("catalog defaults not returned from existing config: %#v", resp)
	}
	if !resp.Enterprise || !resp.ReadOnly || !resp.SafeMode || resp.EmbeddedResources || !resp.IgnoreScopes || !resp.YoloMode {
		t.Errorf("boolean defaults not returned from existing config: %#v", resp)
	}
	if resp.ExcludeTools != "gitlab_admin" || resp.AutoUpdateMode != "check" || resp.RateLimitRPS != "3.5" {
		t.Errorf("text/mode defaults not returned from existing config: %#v", resp)
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

// TestHandleDefaults_VersionTrimPrefix verifies that the server version
// has the "v" prefix stripped before being returned in the JSON response.
func TestHandleDefaults_VersionTrimPrefix(t *testing.T) {
	stubLoadExistingConfig(t)
	stubGetInstalledVersion(t, "")
	handler := handleDefaults("v2.0.0")

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

	if resp.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q (v prefix should be stripped)", resp.Version, "2.0.0")
	}
}

// TestWebUI_ElementIDs_ReferencedInJS_ExistInHTML verifies that every
// document.getElementById call in the JavaScript has a matching id attribute
// in the HTML markup.
func TestWebUI_ElementIDs_ReferencedInJS_ExistInHTML(t *testing.T) {
	html := loadEmbeddedHTML(t)

	jsIDPattern := regexp.MustCompile(`getElementById\(['"]([^'"]+)['"]\)`)
	jsMatches := jsIDPattern.FindAllStringSubmatch(html, -1)
	if len(jsMatches) == 0 {
		t.Fatal("no getElementById calls found in HTML")
	}

	jsIDs := make(map[string]bool)
	for _, match := range jsMatches {
		jsIDs[match[1]] = true
	}

	htmlIDPattern := regexp.MustCompile(`id="([^"]+)"`)
	htmlMatches := htmlIDPattern.FindAllStringSubmatch(html, -1)
	htmlIDs := make(map[string]bool)
	for _, match := range htmlMatches {
		htmlIDs[match[1]] = true
	}

	var missing []string
	for id := range jsIDs {
		if !htmlIDs[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("JavaScript references %d element ID(s) not found in HTML: %v", len(missing), missing)
	}
}

// TestWebUI_APIEndpoints_InJS_MatchGoHandlers verifies every fetch('/api/...')
// call in JavaScript targets an endpoint registered by RunWebUI.
func TestWebUI_APIEndpoints_InJS_MatchGoHandlers(t *testing.T) {
	html := loadEmbeddedHTML(t)

	fetchPattern := regexp.MustCompile(`fetch\(['"](/api/[^'"]+)['"]\s*[,)]`)
	fetchMatches := fetchPattern.FindAllStringSubmatch(html, -1)
	if len(fetchMatches) == 0 {
		t.Fatal("no fetch('/api/...') calls found in HTML")
	}

	jsEndpoints := make(map[string]bool)
	for _, match := range fetchMatches {
		jsEndpoints[match[1]] = true
	}

	goEndpoints := map[string]bool{
		"/api/defaults":       true,
		"/api/pick-directory": true,
		"/api/configure":      true,
	}

	for endpoint := range jsEndpoints {
		if !goEndpoints[endpoint] {
			t.Errorf("JavaScript fetches %q but no Go handler is registered for it", endpoint)
		}
	}

	for endpoint := range goEndpoints {
		if !jsEndpoints[endpoint] {
			t.Errorf("Go handler %q is registered but never called from JavaScript", endpoint)
		}
	}
}

// TestWebUI_DefaultsJSONFields_UsedInJS verifies JavaScript reads only JSON
// fields defined by defaultsResponse and clientResponse.
func TestWebUI_DefaultsJSONFields_UsedInJS(t *testing.T) {
	html := loadEmbeddedHTML(t)

	scriptStart := strings.Index(html, "<script")
	scriptEnd := strings.LastIndex(html, "</script>")
	if scriptStart < 0 || scriptEnd < 0 {
		t.Fatal("cannot find <script> block in HTML")
	}
	jsCode := html[scriptStart:scriptEnd]

	fieldPattern := regexp.MustCompile(`defaults\.([a-z_]+)`)
	fieldMatches := fieldPattern.FindAllStringSubmatch(jsCode, -1)
	if len(fieldMatches) == 0 {
		t.Fatal("no defaults.X field accesses found in JavaScript")
	}

	jsFields := make(map[string]bool)
	for _, match := range fieldMatches {
		jsFields[match[1]] = true
	}

	goFields := jsonTagsFromStruct(t, defaultsResponse{})
	maps.Copy(goFields, jsonTagsFromStruct(t, clientResponse{}))

	var unknown []string
	for field := range jsFields {
		if !goFields[field] {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)

	if len(unknown) > 0 {
		t.Errorf("JavaScript reads %d defaults field(s) not in Go struct: %v", len(unknown), unknown)
	}
}

// TestWebUI_ConfigureRequestFields_InJS_MatchGoStruct verifies the JSON body
// sent by configure() contains exactly the fields expected by configureRequest.
func TestWebUI_ConfigureRequestFields_InJS_MatchGoStruct(t *testing.T) {
	html := loadEmbeddedHTML(t)

	bodyStart := strings.Index(html, "const body = {")
	if bodyStart < 0 {
		t.Fatal("cannot find 'const body = {' in HTML")
	}

	depth := 0
	bodyEnd := -1
	for i := bodyStart + len("const body = "); i < len(html); i++ {
		switch html[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				bodyEnd = i + 1
			}
		}
		if bodyEnd > 0 {
			break
		}
	}
	if bodyEnd < 0 {
		t.Fatal("cannot find closing brace for body object in configure()")
	}

	bodyBlock := html[bodyStart:bodyEnd]
	jsFieldPattern := regexp.MustCompile(`(?m)^\s+([a-z_]+)\s*:`)
	jsFieldMatches := jsFieldPattern.FindAllStringSubmatch(bodyBlock, -1)
	if len(jsFieldMatches) == 0 {
		t.Fatal("no fields found in JavaScript body object")
	}

	jsFields := make(map[string]bool)
	for _, match := range jsFieldMatches {
		jsFields[match[1]] = true
	}

	goFields := jsonTagsFromStruct(t, configureRequest{})

	var jsOnly []string
	for field := range jsFields {
		if !goFields[field] {
			jsOnly = append(jsOnly, field)
		}
	}
	sort.Strings(jsOnly)
	if len(jsOnly) > 0 {
		t.Errorf("JavaScript sends %d field(s) not in Go configureRequest: %v", len(jsOnly), jsOnly)
	}

	var goOnly []string
	for field := range goFields {
		if !jsFields[field] {
			goOnly = append(goOnly, field)
		}
	}
	sort.Strings(goOnly)
	if len(goOnly) > 0 {
		t.Errorf("Go configureRequest has %d field(s) not sent by JavaScript: %v", len(goOnly), goOnly)
	}
}

// TestWebUI_Buttons_HaveProperStructure verifies every JavaScript-bound button
// has the expected id, class, closing tag, and visible text content.
func TestWebUI_Buttons_HaveProperStructure(t *testing.T) {
	html := loadEmbeddedHTML(t)

	requiredButtons := []struct {
		id    string
		class string
	}{
		{id: "browseBtn", class: "btn-browse"},
		{id: "selectAllBtn", class: "select-all-btn"},
		{id: "configureBtn", class: "btn-primary"},
		{id: "closeBtn", class: "btn-primary"},
	}

	for _, button := range requiredButtons {
		t.Run(button.id, func(t *testing.T) {
			idAttr := `id="` + button.id + `"`
			if !strings.Contains(html, idAttr) {
				t.Fatalf("button id=%q not found in HTML", button.id)
			}

			buttonPattern := regexp.MustCompile(`<button[^>]*id="` + regexp.QuoteMeta(button.id) + `"[^>]*>(.*?)</button>`)
			match := buttonPattern.FindStringSubmatch(html)
			if match == nil {
				t.Fatalf("button id=%q is missing closing </button> tag or has malformed markup", button.id)
			}

			textContent := strings.TrimSpace(match[1])
			if textContent == "" {
				t.Errorf("button id=%q has no visible text content", button.id)
			}

			fullTag := match[0]
			if !strings.Contains(fullTag, button.class) {
				t.Errorf("button id=%q missing expected class %q in: %s", button.id, button.class, fullTag)
			}
		})
	}
}

// TestWebUI_InstalledVersion_ElementExists verifies the HTML contains the
// installedVersion element that JavaScript populates with version info.
func TestWebUI_InstalledVersion_ElementExists(t *testing.T) {
	html := loadEmbeddedHTML(t)

	if !strings.Contains(html, `id="installedVersion"`) {
		t.Error("HTML is missing the installedVersion element needed for showing the installed binary version")
	}
}

// TestWebUI_StdioWizard_HidesHTTPOnlyOptions verifies the setup wizard does
// not expose options that only affect HTTP transport mode.
func TestWebUI_StdioWizard_HidesHTTPOnlyOptions(t *testing.T) {
	html := loadEmbeddedHTML(t)
	httpOnlyIDs := []string{
		"optMaxHttpClients",
		"optSessionTimeout",
		"optRevalidateInterval",
		"optAutoUpdateInterval",
		"optAuthMode",
		"optOauthCacheTTL",
	}
	for _, id := range httpOnlyIDs {
		if strings.Contains(html, `id="`+id+`"`) || strings.Contains(html, `getElementById('`+id+`')`) {
			t.Errorf("web wizard should not expose HTTP-only option %q", id)
		}
	}

	httpOnlyFields := []string{
		"max_http_clients",
		"session_timeout",
		"revalidate_interval",
		"auto_update_interval",
		"auth_mode",
		"oauth_cache_ttl",
	}
	defaultsFields := jsonTagsFromStruct(t, defaultsResponse{})
	configureFields := jsonTagsFromStruct(t, configureRequest{})
	for _, field := range httpOnlyFields {
		if defaultsFields[field] {
			t.Errorf("defaultsResponse should not expose HTTP-only field %q", field)
		}
		if configureFields[field] {
			t.Errorf("configureRequest should not accept HTTP-only field %q", field)
		}
	}
}

// TestWebUI_AdvancedOptions_HaveHelpButtons verifies every visible advanced
// option has a question-mark help button with a tooltip description.
func TestWebUI_AdvancedOptions_HaveHelpButtons(t *testing.T) {
	html := loadEmbeddedHTML(t)
	if !strings.Contains(html, `id="advancedTooltip"`) || !strings.Contains(html, `role="tooltip"`) {
		t.Fatal("HTML is missing the shared advanced tooltip element")
	}
	if !strings.Contains(html, "function initTooltips()") {
		t.Fatal("HTML is missing custom tooltip initialization")
	}

	expectedHelp := []string{
		"Skip TLS verification help",
		"Tool surface help",
		"Capability surface help",
		"Meta parameter schema help",
		"Enterprise/Premium catalog help",
		"Read-only mode help",
		"Safe mode previews help",
		"Embedded resources help",
		"Ignore PAT scopes help",
		"Excluded tools help",
		"Upload max file size help",
		"Auto-update mode help",
		"Auto-update repository help",
		"Auto-update timeout help",
		"Rate limit RPS help",
		"Rate limit burst help",
		"YOLO mode help",
		"Log level help",
	}
	for _, label := range expectedHelp {
		if !strings.Contains(html, `aria-label="`+label+`"`) {
			t.Errorf("advanced option missing help button %q", label)
		}
	}
	if strings.Count(html, `class="help-btn"`) != len(expectedHelp) {
		t.Errorf("help button count = %d, want %d", strings.Count(html, `class="help-btn"`), len(expectedHelp))
	}
	if strings.Count(html, `class="help-btn"`) != strings.Count(html, `data-tooltip="`) {
		t.Error("each help button should carry a custom data-tooltip description")
	}
	if strings.Contains(html, `class="help-btn"`) && strings.Contains(html, `title="`) {
		t.Error("help buttons should not rely on native title tooltips")
	}
}
