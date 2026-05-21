package wizard

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	Version           string           `json:"version"`
	InstalledVersion  string           `json:"installed_version,omitempty"`
	InstallPath       string           `json:"install_path"`
	GitLabURL         string           `json:"gitlab_url"`
	HasExisting       bool             `json:"has_existing"`
	MaskedToken       string           `json:"masked_token,omitempty"`
	SkipTLSVerify     bool             `json:"skip_tls_verify"`
	ToolSurface       string           `json:"tool_surface"`
	CapabilitySurface string           `json:"capability_surface"`
	MetaParamSchema   string           `json:"meta_param_schema"`
	Enterprise        bool             `json:"enterprise"`
	ReadOnly          bool             `json:"read_only"`
	SafeMode          bool             `json:"safe_mode"`
	EmbeddedResources bool             `json:"embedded_resources"`
	ExcludeTools      string           `json:"exclude_tools"`
	IgnoreScopes      bool             `json:"ignore_scopes"`
	UploadMaxFileSize string           `json:"upload_max_file_size"`
	AutoUpdateMode    string           `json:"auto_update_mode"`
	AutoUpdateRepo    string           `json:"auto_update_repo"`
	AutoUpdateTimeout string           `json:"auto_update_timeout"`
	RateLimitRPS      string           `json:"rate_limit_rps"`
	RateLimitBurst    string           `json:"rate_limit_burst"`
	YoloMode          bool             `json:"yolo_mode"`
	LogLevel          string           `json:"log_level"`
	Clients           []clientResponse `json:"clients"`
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

		cfg := DefaultServerConfig()
		if hasExisting {
			cfg = existing.withDefaults()
		}

		var maskedToken string
		if hasExisting && existing.GitLabToken != "" {
			maskedToken = MaskToken(existing.GitLabToken)
		}

		clients := allClientsFn()
		resp := defaultsResponse{
			Version:           strings.TrimPrefix(version, "v"),
			InstalledVersion:  getInstalledVersionFn(),
			InstallPath:       filepath.Join(DefaultInstallDir(), DefaultBinaryName()),
			GitLabURL:         cfg.GitLabURL,
			HasExisting:       hasExisting,
			MaskedToken:       maskedToken,
			SkipTLSVerify:     cfg.SkipTLSVerify,
			ToolSurface:       cfg.ToolSurface,
			CapabilitySurface: cfg.CapabilitySurface,
			MetaParamSchema:   cfg.MetaParamSchema,
			Enterprise:        cfg.Enterprise,
			ReadOnly:          cfg.ReadOnly,
			SafeMode:          cfg.SafeMode,
			EmbeddedResources: cfg.EmbeddedResources,
			ExcludeTools:      cfg.ExcludeTools,
			IgnoreScopes:      cfg.IgnoreScopes,
			UploadMaxFileSize: cfg.UploadMaxFileSize,
			AutoUpdateMode:    cfg.AutoUpdateMode,
			AutoUpdateRepo:    cfg.AutoUpdateRepo,
			AutoUpdateTimeout: cfg.AutoUpdateTimeout,
			RateLimitRPS:      cfg.RateLimitRPS,
			RateLimitBurst:    cfg.RateLimitBurst,
			YoloMode:          cfg.YoloMode,
			LogLevel:          cfg.LogLevel,
			Clients:           make([]clientResponse, len(clients)),
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
	InstallPath       string `json:"install_path"`
	GitLabURL         string `json:"gitlab_url"`
	GitLabToken       string `json:"gitlab_token"`
	SkipTLSVerify     bool   `json:"skip_tls_verify"`
	ToolSurface       string `json:"tool_surface"`
	CapabilitySurface string `json:"capability_surface"`
	MetaParamSchema   string `json:"meta_param_schema"`
	Enterprise        bool   `json:"enterprise"`
	ReadOnly          bool   `json:"read_only"`
	SafeMode          bool   `json:"safe_mode"`
	EmbeddedResources bool   `json:"embedded_resources"`
	ExcludeTools      string `json:"exclude_tools"`
	IgnoreScopes      bool   `json:"ignore_scopes"`
	UploadMaxFileSize string `json:"upload_max_file_size"`
	AutoUpdate        bool   `json:"auto_update"`
	AutoUpdateMode    string `json:"auto_update_mode"`
	AutoUpdateRepo    string `json:"auto_update_repo"`
	AutoUpdateTimeout string `json:"auto_update_timeout"`
	RateLimitRPS      string `json:"rate_limit_rps"`
	RateLimitBurst    string `json:"rate_limit_burst"`
	YoloMode          bool   `json:"yolo_mode"`
	LogLevel          string `json:"log_level"`
	SelectedClients   []int  `json:"selected_clients"`
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

		req.GitLabURL = effectiveGitLabURL(req.GitLabURL)
		if err := validateConfigureRequest(&req); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		if err := normalizeConfigureRequest(&req); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		installDir, binaryPath := installBinaryForConfigure(w, req.InstallPath)
		cfg := serverConfigFromConfigureRequest(req, binaryPath)

		result := &Result{
			InstallDir:      installDir,
			BinaryPath:      binaryPath,
			Config:          cfg,
			SelectedClients: req.SelectedClients,
		}

		// Apply configuration — capture output for detail log
		printSection(w, "Writing Configurations (Web UI)")
		applyErr := Apply(w, result)

		resp := configureResponseForSelection(req.SelectedClients, result.Config)

		rw.Header().Set(headerContentType, mimeJSON)
		_ = json.NewEncoder(rw).Encode(resp)

		onDone(applyErr)
	}
}

func validateConfigureRequest(req *configureRequest) error {
	if req.GitLabToken == "" {
		existing, hasExisting := loadExistingConfigFn()
		if !hasExisting || existing.GitLabToken == "" {
			return errors.New("GitLab token is required")
		}
		req.GitLabToken = existing.GitLabToken
	}
	if _, err := url.ParseRequestURI(req.GitLabURL); err != nil {
		return fmt.Errorf("invalid GitLab URL: %w", err)
	}
	return nil
}

func installBinaryForConfigure(w io.Writer, requestedPath string) (installDir, binaryPath string) {
	installDir = requestedPath
	binaryPath = installDir
	if strings.HasSuffix(installDir, DefaultBinaryName()) {
		installDir = filepath.Dir(installDir)
	}
	expandedDir, err := ExpandPath(installDir)
	if err != nil {
		return installDir, binaryPath
	}
	installed, installErr := InstallBinary(expandedDir)
	if installErr == nil {
		fmt.Fprintf(w, "  * Binary installed to %s\n", installed)
		return installDir, installed
	}
	exe, _ := os.Executable()
	fmt.Fprintf(w, "  ! Could not install: %v (using current location)\n", installErr)
	return installDir, exe
}

func serverConfigFromConfigureRequest(req configureRequest, binaryPath string) ServerConfig {
	return ServerConfig{
		BinaryPath:        binaryPath,
		GitLabURL:         req.GitLabURL,
		GitLabToken:       req.GitLabToken,
		SkipTLSVerify:     req.SkipTLSVerify,
		MetaTools:         req.ToolSurface != "individual",
		ToolSurface:       req.ToolSurface,
		CapabilitySurface: req.CapabilitySurface,
		MetaParamSchema:   req.MetaParamSchema,
		Enterprise:        req.Enterprise,
		ReadOnly:          req.ReadOnly,
		SafeMode:          req.SafeMode,
		EmbeddedResources: req.EmbeddedResources,
		ExcludeTools:      req.ExcludeTools,
		IgnoreScopes:      req.IgnoreScopes,
		UploadMaxFileSize: req.UploadMaxFileSize,
		AutoUpdate:        req.AutoUpdate,
		AutoUpdateMode:    req.AutoUpdateMode,
		AutoUpdateRepo:    req.AutoUpdateRepo,
		AutoUpdateTimeout: req.AutoUpdateTimeout,
		RateLimitRPS:      req.RateLimitRPS,
		RateLimitBurst:    req.RateLimitBurst,
		YoloMode:          req.YoloMode,
		LogLevel:          req.LogLevel,
	}.withDefaults()
}

func configureResponseForSelection(selectedClients []int, config ServerConfig) configureResponse {
	clients := allClientsFn()
	resp := configureResponse{}
	var jetBrains strings.Builder
	for _, idx := range selectedClients {
		if idx < 0 || idx >= len(clients) {
			continue
		}
		client := clients[idx]
		resp.Configured = append(resp.Configured, client.Name)
		if client.DisplayOnly {
			_ = printJetBrainsConfig(&jetBrains, config)
			resp.JetBrainsJSON = jetBrains.String()
		}
	}
	return resp
}

func normalizeConfigureRequest(req *configureRequest) error {
	defaults := DefaultServerConfig()
	req.ToolSurface = firstNonEmpty(req.ToolSurface, defaults.ToolSurface)
	if !slices.Contains(ToolSurfaceOptions, req.ToolSurface) {
		return fmt.Errorf("invalid tool_surface: %s", req.ToolSurface)
	}

	req.CapabilitySurface = firstNonEmpty(req.CapabilitySurface, defaults.CapabilitySurface)
	if !slices.Contains(CapabilitySurfaceOptions, req.CapabilitySurface) {
		return fmt.Errorf("invalid capability_surface: %s", req.CapabilitySurface)
	}

	req.MetaParamSchema = firstNonEmpty(req.MetaParamSchema, defaults.MetaParamSchema)
	if !slices.Contains(MetaParamSchemaOptions, req.MetaParamSchema) {
		return fmt.Errorf("invalid meta_param_schema: %s", req.MetaParamSchema)
	}

	req.AutoUpdateMode = firstNonEmpty(req.AutoUpdateMode, defaults.AutoUpdateMode)
	if !slices.Contains(AutoUpdateModeOptions, req.AutoUpdateMode) {
		return fmt.Errorf("invalid auto_update_mode: %s", req.AutoUpdateMode)
	}
	req.AutoUpdate = req.AutoUpdateMode != "false"

	req.RateLimitRPS = firstNonEmpty(req.RateLimitRPS, defaults.RateLimitRPS)
	rateLimitRPS, err := strconv.ParseFloat(req.RateLimitRPS, 64)
	if err != nil || rateLimitRPS < 0 {
		return fmt.Errorf("invalid rate_limit_rps: %s", req.RateLimitRPS)
	}

	req.RateLimitBurst = firstNonEmpty(req.RateLimitBurst, defaults.RateLimitBurst)
	rateLimitBurst, err := strconv.Atoi(req.RateLimitBurst)
	if err != nil || rateLimitBurst < 0 {
		return fmt.Errorf("invalid rate_limit_burst: %s", req.RateLimitBurst)
	}

	if !slices.Contains(LogLevelOptions, req.LogLevel) {
		req.LogLevel = defaults.LogLevel
	}
	return nil
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
