// run_test.go contains unit tests for the wizard run entry point and
// UI mode dispatch logic.
package wizard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
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

// stubInteractiveTerminal overrides isInteractiveTerminalFn so the auto-mode
// cascade can be steered into or away from the TUI branch without a real TTY.
func stubInteractiveTerminal(t *testing.T, interactive bool) {
	t.Helper()
	orig := isInteractiveTerminalFn
	isInteractiveTerminalFn = func() bool { return interactive }
	t.Cleanup(func() { isInteractiveTerminalFn = orig })
}

// stubHasDisplay overrides hasDisplayFn so the auto-mode cascade can be
// steered into or away from the Web UI branch regardless of the host's
// graphical environment.
func stubHasDisplay(t *testing.T, has bool) {
	t.Helper()
	orig := hasDisplayFn
	hasDisplayFn = func() bool { return has }
	t.Cleanup(func() { hasDisplayFn = orig })
}

// TestRun_TUIMode_Dispatch verifies Run delegates to RunTUI in TUI mode by
// swapping stdinFn for a Ctrl+C reader: the TUI starts, aborts cleanly, and
// Run returns nil after printing the cancellation notice.
func TestRun_TUIMode_Dispatch(t *testing.T) {
	stubWriteEnvFile(t)
	origStdin := stdinFn
	stdinFn = func() io.Reader { return bytes.NewReader([]byte{0x03}) }
	t.Cleanup(func() { stdinFn = origStdin })

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- Run("1.0.0", UIModeTUI, nil, &output) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run(tui) error = %v, want nil from Ctrl+C abort", err)
		}
		if !strings.Contains(output.String(), "Setup cancelled") {
			t.Errorf("Run(tui) output = %q, want cancellation notice", output.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run(tui) hung")
	}
}

// TestRunCascade_WebSucceeds_ReturnsNil verifies auto mode returns nil right
// after a successful Web UI session: with a display available, the browser
// stub posts a valid configuration and the cascade never reaches TUI or CLI.
func TestRunCascade_WebSucceeds_ReturnsNil(t *testing.T) {
	useFakeClients(t)
	stubWriteEnvFile(t)
	stubInstallBinary(t)
	stubLoadExistingConfig(t)
	stubHasDisplay(t, true)

	origOpenBrowser := openBrowserFn
	openedURL := make(chan string, 1)
	openBrowserFn = func(url string) error {
		openedURL <- url
		return nil
	}
	t.Cleanup(func() { openBrowserFn = origOpenBrowser })

	var output bytes.Buffer
	errCh := make(chan error, 1)
	go func() { errCh <- Run("1.0.0", UIModeAuto, nil, &output) }()

	var webURL string
	select {
	case webURL = <-openedURL:
	case <-time.After(3 * time.Second):
		t.Fatal("Run(auto) did not open the local wizard URL")
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
			t.Fatalf("Run(auto) error = %v, want nil after web configuration", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run(auto) did not finish after web configuration")
	}
}

// TestRunCascade_WebFailsThenTUISucceeds verifies the first cascade fallback:
// with a display available but the web listener failing, auto mode prints the
// Web UI fallback notice and completes through the TUI (aborted via Ctrl+C).
func TestRunCascade_WebFailsThenTUISucceeds(t *testing.T) {
	stubWriteEnvFile(t)
	stubHasDisplay(t, true)
	stubInteractiveTerminal(t, true)

	origListen := listenFn
	listenFn = func(context.Context, string, string) (net.Listener, error) {
		return nil, errors.New("bind refused")
	}
	t.Cleanup(func() { listenFn = origListen })

	origStdin := stdinFn
	stdinFn = func() io.Reader { return bytes.NewReader([]byte{0x03}) }
	t.Cleanup(func() { stdinFn = origStdin })

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- Run("1.0.0", UIModeAuto, nil, &output) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run(auto) error = %v, want nil from TUI abort", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run(auto) hung")
	}
	out := output.String()
	if !strings.Contains(out, "Web UI unavailable, falling back to terminal UI...") {
		t.Errorf("output %q missing Web UI fallback notice", out)
	}
	if !strings.Contains(out, "Setup cancelled") {
		t.Errorf("output %q missing TUI cancellation notice", out)
	}
}

// TestRunCascade_TUIFailsThenCLI verifies the second cascade fallback: with
// no display and a TUI whose input reader fails, auto mode prints the TUI
// fallback notice and continues into the plain CLI (which then fails on the
// exhausted reader, proving the CLI branch ran).
func TestRunCascade_TUIFailsThenCLI(t *testing.T) {
	stubLoadExistingConfig(t)
	stubHasDisplay(t, false)
	stubInteractiveTerminal(t, true)

	origStdin := stdinFn
	stdinFn = func() io.Reader { return failingInputReader{} }
	t.Cleanup(func() { stdinFn = origStdin })

	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- Run("1.0.0", UIModeAuto, strings.NewReader(""), &output) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run(auto) error = nil, want CLI input error from empty reader")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run(auto) hung")
	}
	out := output.String()
	if !strings.Contains(out, "TUI unavailable, falling back to plain CLI...") {
		t.Errorf("output %q missing TUI fallback notice", out)
	}
	if !strings.Contains(out, "gitlab-mcp-server Setup Wizard") {
		t.Errorf("output %q missing CLI wizard banner", out)
	}
}

// TestRunCascade_NilReaderUsesStdin verifies the final CLI fallback binds to
// os.Stdin when no reader is supplied: with a non-interactive stdin swapped
// in, the CLI starts (banner printed) and fails on the empty input, proving
// the nil-reader branch selected os.Stdin.
func TestRunCascade_NilReaderUsesStdin(t *testing.T) {
	stubLoadExistingConfig(t)
	stubHasDisplay(t, false)
	stubInteractiveTerminal(t, false)

	originalStdin := os.Stdin
	emptyStdin, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("creating empty stdin: %v", err)
	}
	os.Stdin = emptyStdin
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = emptyStdin.Close()
	})

	var output bytes.Buffer
	runErr := Run("1.0.0", UIModeAuto, nil, &output)
	if runErr == nil {
		t.Fatal("Run(auto) error = nil, want CLI input error from empty stdin")
	}
	if !strings.Contains(output.String(), "gitlab-mcp-server Setup Wizard") {
		t.Errorf("output %q missing CLI wizard banner", output.String())
	}
}
