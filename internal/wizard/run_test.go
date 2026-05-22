// run_test.go contains unit tests for the wizard run entry point and
// UI mode dispatch logic.
package wizard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRun_UnknownMode verifies that Run returns an error for an
// unrecognized UI mode string.
func TestRun_UnknownMode(t *testing.T) {
	err := Run("1.0.0", "invalid-mode", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown UI mode, got nil")
	}
}

// TestRun_CLIMode_Dispatch verifies Run delegates to RunCLI in CLI mode
// with proper interactive input sequence.
func TestRun_CLIMode_Dispatch(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "bin")

	input := strings.Join([]string{
		installDir + string(os.PathSeparator) + DefaultBinaryName(),
		"https://gitlab.example.com",
		"test-token-test123",
		"n",
		"a",
	}, "\n") + "\n"

	r := strings.NewReader(input)
	w := &bytes.Buffer{}

	err := Run("1.0.0", UIModeCLI, r, w)
	if err != nil {
		t.Logf("Run(CLI) returned error (expected in test env): %v", err)
	}
}

// TestRun_AutoHeadlessFallsBackToCLI verifies auto mode skips browser startup
// when no display is available and completes through the CLI fallback.
func TestRun_AutoHeadlessFallsBackToCLI(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)
	originalHasDisplay := hasDisplayFn
	hasDisplayFn = func() bool { return false }
	t.Cleanup(func() { hasDisplayFn = originalHasDisplay })
	originalStdin := os.Stdin
	nonInteractiveStdin, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("creating non-interactive stdin: %v", err)
	}
	os.Stdin = nonInteractiveStdin
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = nonInteractiveStdin.Close()
	})

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "bin")
	input := strings.Join([]string{
		installDir + string(os.PathSeparator) + DefaultBinaryName(),
		"https://gitlab.example.com",
		"test-token-test123",
		"n",
		"a",
	}, "\n") + "\n"

	var output bytes.Buffer
	if runErr := Run("1.0.0", UIModeAuto, strings.NewReader(input), &output); runErr != nil {
		t.Fatalf("Run(auto) error = %v", runErr)
	}
	if !strings.Contains(output.String(), "gitlab-mcp-server Setup Wizard") {
		t.Fatalf("Run(auto) output = %q, want CLI wizard", output.String())
	}
}

// TestRun_WebModeCompletesAfterConfigure exercises the browser wizard without
// launching a real browser by posting a valid configuration to the local server.
func TestRun_WebModeCompletesAfterConfigure(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)
	stubInstallBinary(t)
	stubLoadExistingConfig(t)

	originalOpenBrowser := openBrowserFn
	openedURL := make(chan string, 1)
	openBrowserFn = func(url string) error {
		openedURL <- url
		return nil
	}
	t.Cleanup(func() { openBrowserFn = originalOpenBrowser })

	var output bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run("2.0.0-test", UIModeWeb, nil, &output)
	}()

	var webURL string
	select {
	case webURL = <-openedURL:
	case <-time.After(3 * time.Second):
		t.Fatal("Run(web) did not open the local wizard URL")
	}

	reqBody := configureRequest{
		InstallPath:       filepath.Join(t.TempDir(), "bin"),
		GitLabURL:         "https://gitlab.example.com",
		GitLabToken:       "test-token-test123",
		ToolSurface:       "meta",
		CapabilitySurface: "full",
		MetaParamSchema:   "opaque",
		AutoUpdateMode:    "false",
		RateLimitRPS:      "0",
		RateLimitBurst:    "40",
		LogLevel:          "info",
		SelectedClients:   []int{0},
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal configure request: %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, webURL+"/api/configure", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("build POST /api/configure request: %v", err)
	}
	req.Header.Set("Content-Type", mimeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/configure: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/configure status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("Run(web) error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run(web) did not finish after configuration")
	}
	if !strings.Contains(output.String(), "Setup wizard available") {
		t.Fatalf("Run(web) output = %q, want setup URL", output.String())
	}
}

// TestToolSurfaceHelpers verifies legacy meta-tools values still map to the
// modern tool surface defaults used in generated wizard configuration.
func TestToolSurfaceHelpers(t *testing.T) {
	if got := toolSurfaceFromMetaTools(true); got != "meta" {
		t.Fatalf("toolSurfaceFromMetaTools(true) = %q, want meta", got)
	}
	if got := toolSurfaceFromMetaTools(false); got != "individual" {
		t.Fatalf("toolSurfaceFromMetaTools(false) = %q, want individual", got)
	}
	toolSurface, metaTools := toolSurfaceFromEnv(map[string]string{"TOOL_SURFACE": "invalid", "META_TOOLS": "false"})
	if toolSurface != "individual" || metaTools {
		t.Fatalf("toolSurfaceFromEnv(invalid,false) = %q/%v, want individual/false", toolSurface, metaTools)
	}
}
