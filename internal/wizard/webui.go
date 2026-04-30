// webui.go implements the browser-based wizard UI, serving a local HTTP
// server with embedded assets for graphical MCP server configuration.
package wizard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// Shared HTTP response header and MIME constants used by the local wizard API.
	headerContentType = "Content-Type"
	mimeJSON          = "application/json"
)

// webAssets contains the browser wizard shell served by [serveIndex].
//
//go:embed webui_assets/index.html
var webAssets embed.FS

// RunWebUI starts a local HTTP server and opens the setup wizard in the browser.
// It blocks until the user completes configuration or the context is cancelled.
func RunWebUI(version string, w io.Writer) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting web server: %w", err)
	}

	addr := listener.Addr().String()
	webURL := "http://" + addr

	done := make(chan error, 1)
	var once sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", serveIndex)
	mux.HandleFunc("GET /api/defaults", handleDefaults(version))
	mux.HandleFunc("POST /api/pick-directory", handlePickDirectory())
	mux.HandleFunc("POST /api/configure", handleConfigure(w, func(err error) {
		once.Do(func() { done <- err })
	}))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			once.Do(func() { done <- serveErr })
		}
	}()

	fmt.Fprintf(w, "\n  Setup wizard available at: %s\n", webURL)
	fmt.Fprintln(w, "  Opening browser...")

	if err = openBrowserFn(webURL); err != nil {
		fmt.Fprintf(w, "  Could not open browser: %v\n", err)
		fmt.Fprintf(w, "  Please open %s manually.\n", webURL)
	}

	fmt.Fprintln(w, "  Waiting for configuration... (press Ctrl+C to cancel)")
	fmt.Fprintln(w)

	result := <-done
	_ = server.Shutdown(context.Background())
	return result
}

// serveIndex writes the embedded browser wizard HTML page.
func serveIndex(rw http.ResponseWriter, _ *http.Request) {
	data, err := webAssets.ReadFile("webui_assets/index.html")
	if err != nil {
		http.Error(rw, "Internal error", http.StatusInternalServerError)
		return
	}
	rw.Header().Set(headerContentType, "text/html; charset=utf-8")
	_, _ = rw.Write(data)
}

// defaultsResponse is the JSON payload returned by GET /api/defaults.
type defaultsResponse struct {
	Version          string           `json:"version"`
	InstalledVersion string           `json:"installed_version,omitempty"`
	InstallPath      string           `json:"install_path"`
	GitLabURL        string           `json:"gitlab_url"`
	HasExisting      bool             `json:"has_existing"`
	MaskedToken      string           `json:"masked_token,omitempty"`
	SkipTLSVerify    bool             `json:"skip_tls_verify"`
	Clients          []clientResponse `json:"clients"`
}

// clientResponse describes one configurable MCP client in /api/defaults.
type clientResponse struct {
	Name            string `json:"name"`
	ConfigPath      string `json:"config_path"`
	DisplayOnly     bool   `json:"display_only"`
	DefaultSelected bool   `json:"default_selected"`
}

// handleDefaults returns default wizard values and any previously saved local
// configuration so the browser UI can pre-fill the form.
func handleDefaults(version string) http.HandlerFunc {
	return func(rw http.ResponseWriter, _ *http.Request) {
		existing, hasExisting := loadExistingConfigFn()

		gitlabURL := DefaultGitLabURL
		skipTLS := false
		if hasExisting {
			if existing.GitLabURL != "" {
				gitlabURL = existing.GitLabURL
			}
			skipTLS = existing.SkipTLSVerify
		}

		var maskedToken string
		if hasExisting && existing.GitLabToken != "" {
			maskedToken = MaskToken(existing.GitLabToken)
		}

		clients := allClientsFn()
		resp := defaultsResponse{
			Version:          strings.TrimPrefix(version, "v"),
			InstalledVersion: getInstalledVersionFn(),
			InstallPath:      filepath.Join(DefaultInstallDir(), DefaultBinaryName()),
			GitLabURL:        gitlabURL,
			HasExisting:      hasExisting,
			MaskedToken:      maskedToken,
			SkipTLSVerify:    skipTLS,
			Clients:          make([]clientResponse, len(clients)),
		}
		for i, c := range clients {
			resp.Clients[i] = clientResponse{
				Name:            c.Name,
				ConfigPath:      c.ConfigPath,
				DisplayOnly:     c.DisplayOnly,
				DefaultSelected: c.DefaultSelected,
			}
		}
		rw.Header().Set(headerContentType, mimeJSON)
		_ = json.NewEncoder(rw).Encode(resp)
	}
}

// configureRequest is the JSON body accepted by POST /api/configure.
type configureRequest struct {
	InstallPath     string `json:"install_path"`
	GitLabURL       string `json:"gitlab_url"`
	GitLabToken     string `json:"gitlab_token"`
	SkipTLSVerify   bool   `json:"skip_tls_verify"`
	MetaTools       bool   `json:"meta_tools"`
	AutoUpdate      bool   `json:"auto_update"`
	YoloMode        bool   `json:"yolo_mode"`
	LogLevel        string `json:"log_level"`
	SelectedClients []int  `json:"selected_clients"`
}

// configureResponse is the JSON result returned after applying selected client
// configurations.
type configureResponse struct {
	Configured    []string `json:"configured"`
	JetBrainsJSON string   `json:"jetbrains_json,omitempty"`
}

// handleConfigure validates the browser wizard submission, installs or locates
// the binary, writes selected client configurations, and signals completion.
func handleConfigure(w io.Writer, onDone func(error)) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var req configureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(rw, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.GitLabURL == "" {
			http.Error(rw, "GitLab URL is required", http.StatusBadRequest)
			return
		}

		// Allow empty token when an existing config has one
		if req.GitLabToken == "" {
			existing, hasExisting := loadExistingConfigFn()
			if hasExisting && existing.GitLabToken != "" {
				req.GitLabToken = existing.GitLabToken
			} else {
				http.Error(rw, "GitLab token is required", http.StatusBadRequest)
				return
			}
		}

		if _, err := url.ParseRequestURI(req.GitLabURL); err != nil {
			http.Error(rw, fmt.Sprintf("Invalid GitLab URL: %v", err), http.StatusBadRequest)
			return
		}

		// Validate log level
		validLevel := slices.Contains(LogLevelOptions, req.LogLevel)
		if !validLevel {
			req.LogLevel = "info"
		}

		// Install binary
		installDir := req.InstallPath
		binaryPath := installDir
		if strings.HasSuffix(installDir, DefaultBinaryName()) {
			installDir = filepath.Dir(installDir)
		}
		expandedDir, err := ExpandPath(installDir)
		if err == nil {
			installed, installErr := InstallBinary(expandedDir)
			if installErr == nil {
				binaryPath = installed
				fmt.Fprintf(w, "  * Binary installed to %s\n", installed)
			} else {
				exe, _ := os.Executable()
				binaryPath = exe
				fmt.Fprintf(w, "  ! Could not install: %v (using current location)\n", installErr)
			}
		}

		result := &Result{
			InstallDir: installDir,
			BinaryPath: binaryPath,
			Config: ServerConfig{
				BinaryPath:    binaryPath,
				GitLabURL:     req.GitLabURL,
				GitLabToken:   req.GitLabToken,
				SkipTLSVerify: req.SkipTLSVerify,
				MetaTools:     req.MetaTools,
				AutoUpdate:    req.AutoUpdate,
				YoloMode:      req.YoloMode,
				LogLevel:      req.LogLevel,
			},
			SelectedClients: req.SelectedClients,
		}

		// Apply configuration — capture output for detail log
		printSection(w, "Writing Configurations (Web UI)")
		applyErr := Apply(w, result)

		// Build response
		clients := allClientsFn()
		resp := configureResponse{}
		var jbBuf strings.Builder
		for _, idx := range req.SelectedClients {
			if idx < 0 || idx >= len(clients) {
				continue
			}
			c := clients[idx]
			resp.Configured = append(resp.Configured, c.Name)
			if c.DisplayOnly {
				_ = printJetBrainsConfig(&jbBuf, result.Config)
				resp.JetBrainsJSON = jbBuf.String()
			}
		}

		rw.Header().Set(headerContentType, mimeJSON)
		_ = json.NewEncoder(rw).Encode(resp)

		onDone(applyErr)
	}
}

// handlePickDirectory returns an HTTP handler that asks the host OS to choose
// an installation directory and returns the selected path as JSON.
func handlePickDirectory() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var req struct {
			StartDir string `json:"start_dir"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		selected, err := pickDirectoryFn(req.StartDir)
		if err != nil {
			rw.WriteHeader(http.StatusNoContent)
			return
		}

		rw.Header().Set(headerContentType, mimeJSON)
		_ = json.NewEncoder(rw).Encode(map[string]string{"path": selected})
	}
}
