// Command server is the MCP server entry point for gitlab-mcp-server.
// In stdio mode, configuration comes from environment variables (.env / exports).
// In HTTP mode, configuration comes from CLI flags; no GITLAB_TOKEN is required
// at startup — each client provides its own token per-request.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/completions"
	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/roots"
	"github.com/jmrplens/gitlab-mcp-server/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/serverupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/wizard"
)

// version and commit are set at build time via -ldflags.
// The VERSION file at the repo root is the single source of truth for version.
var (
	version = "dev"
	commit  = "none"
)

// Project metadata.
const (
	projectAuthor     = "Jose Manuel Requena Plens"
	projectDepartment = ""
	projectRepository = "https://github.com/jmrplens/gitlab-mcp-server"
)

// httpConfig holds CLI-flag configuration for HTTP server mode.
// When non-nil is passed to [runWithContext], the server starts in HTTP mode
// without requiring a GITLAB_TOKEN — each client must provide its own token
// via PRIVATE-TOKEN header or Authorization: Bearer.
type httpConfig struct {
	addr               string
	gitlabURL          string
	skipTLSVerify      bool
	metaTools          bool
	enterprise         bool
	readOnly           bool
	safeMode           bool
	excludeTools       string
	ignoreScopes       bool
	maxHTTPClients     int
	sessionTimeout     time.Duration
	autoUpdate         string
	autoUpdateRepo     string
	autoUpdateInterval time.Duration
	autoUpdateTimeout  time.Duration
	revalidateInterval time.Duration
	authMode           string
	oauthCacheTTL      time.Duration
	trustedProxyHeader string
	rateLimitRPS       float64
	rateLimitBurst     int
}

// main is an internal helper for the main package.
func main() {
	var showHelp bool
	var showVersion bool
	var shutdownPeers bool
	var useHTTP bool
	var forceSetup bool
	var setupMode string
	var toolSearch string
	var hcfg httpConfig

	flag.BoolVar(&showHelp, "h", false, "Show full help with flags, env vars, and examples")
	flag.BoolVar(&shutdownPeers, "shutdown", false, "Terminate all running instances and exit")
	flag.BoolVar(&forceSetup, "setup", false, "Run interactive setup wizard")
	flag.StringVar(&setupMode, "setup-mode", "auto", "Setup UI mode: auto, web, tui, cli")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")
	flag.StringVar(&toolSearch, "tool-search", "", "Search tools by name/description and exit")
	flag.BoolVar(&useHTTP, "http", false, "Run MCP server in HTTP mode")
	flag.StringVar(&hcfg.addr, "http-addr", ":8080", "HTTP listen address")
	flag.StringVar(&hcfg.gitlabURL, "gitlab-url", "", "Default GitLab instance URL (can be overridden per-request via GITLAB-URL header)")
	flag.BoolVar(&hcfg.skipTLSVerify, "skip-tls-verify", false, "Skip TLS certificate verification")
	flag.BoolVar(&hcfg.metaTools, "meta-tools", true, "Enable meta-tools for tool discovery")
	flag.BoolVar(&hcfg.enterprise, "enterprise", false, "Enable Enterprise/Premium meta-tools")
	flag.BoolVar(&hcfg.readOnly, "read-only", false, "Expose only read-only tools (no create/update/delete)")
	flag.BoolVar(&hcfg.safeMode, "safe-mode", false, "Intercept mutating tools and return a preview instead of executing")
	flag.StringVar(&hcfg.excludeTools, "exclude-tools", "", "Comma-separated list of tool names to exclude from registration")
	flag.BoolVar(&hcfg.ignoreScopes, "ignore-scopes", false, "Skip PAT scope detection and register all tools")
	flag.IntVar(&hcfg.maxHTTPClients, "max-http-clients", config.DefaultMaxHTTPClients, "Maximum concurrent client sessions")
	flag.DurationVar(&hcfg.sessionTimeout, "session-timeout", config.DefaultSessionTimeout, "Idle session timeout")
	flag.StringVar(&hcfg.autoUpdate, "auto-update", "true", "Auto-update mode: true (auto-apply), check (log-only), false (disabled)")
	flag.StringVar(&hcfg.autoUpdateRepo, "auto-update-repo", config.DefaultAutoUpdateRepo, "GitHub repository for update checks")
	flag.DurationVar(&hcfg.autoUpdateInterval, "auto-update-interval", config.DefaultAutoUpdateInterval, "How often to check for updates")
	flag.DurationVar(&hcfg.autoUpdateTimeout, "auto-update-timeout", config.DefaultAutoUpdateTimeout, "Timeout for pre-start update download (range 5s\u201310m)")
	flag.DurationVar(&hcfg.revalidateInterval, "revalidate-interval", config.DefaultRevalidateInterval, "Token re-validation interval (0 to disable)")
	flag.StringVar(&hcfg.authMode, "auth-mode", "legacy", "Authentication mode: legacy (default) or oauth")
	flag.DurationVar(&hcfg.oauthCacheTTL, "oauth-cache-ttl", config.DefaultOAuthCacheTTL, "OAuth token cache TTL")
	flag.StringVar(&hcfg.trustedProxyHeader, "trusted-proxy-header", "", "HTTP header containing the real client IP (e.g. X-Forwarded-For, X-Real-IP)")
	flag.Float64Var(&hcfg.rateLimitRPS, "rate-limit-rps", 0, "Per-server tools/call rate limit in requests/second (0 = disabled)")
	flag.IntVar(&hcfg.rateLimitBurst, "rate-limit-burst", config.DefaultRateLimitBurst, "Token-bucket burst size when --rate-limit-rps > 0")
	flag.Parse()

	if showHelp {
		printHelp()
		return
	}

	if showVersion {
		fmt.Printf("gitlab-mcp-server %s (commit: %s)\n", version, commit)
		return
	}

	if shutdownPeers {
		os.Exit(runShutdown())
	}

	if toolSearch != "" {
		runToolSearch(toolSearch, hcfg.metaTools, hcfg.enterprise)
		return
	}

	if forceSetup || (!useHTTP && !showHelp && !showVersion && wizard.IsInteractiveTerminal()) {
		if err := wizard.Run(version, wizard.UIMode(setupMode), os.Stdin, os.Stdout); err != nil {
			slog.Error("setup wizard failed", "error", err)
			os.Exit(1)
		}
		return
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LOG_LEVEL")),
	})))

	setupAutoUpdateRedaction("")

	health.SetServerInfo(health.ServerInfo{
		Version:    version,
		Author:     projectAuthor,
		Department: projectDepartment,
		Repository: projectRepository,
	})

	serverupdate.SetServerInfo(serverupdate.ServerInfo{
		Author:     projectAuthor,
		Department: projectDepartment,
		Repository: projectRepository,
	})

	var hcfgPtr *httpConfig
	if useHTTP {
		hcfgPtr = &hcfg
	}

	if err := run(hcfgPtr); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

// printHelp displays comprehensive usage information including version, author,
// all flags, environment variables, and JSON configuration examples.
func printHelp() {
	fmt.Printf(`gitlab-mcp-server — GitLab MCP Server
==================================

Version:      %s (commit: %s)
Author:       %s
Department:   %s
Repository:   %s

DESCRIPTION
  A Model Context Protocol (MCP) server that exposes GitLab operations
  as MCP tools, resources, and prompts for AI assistants.
  Supports stdio (default) and HTTP transport modes.

FLAGS
  -h                        Show this help message
  -version                  Print version and exit
  -shutdown                 Terminate all running instances and exit
  -setup                    Run interactive setup wizard
  -setup-mode string        Setup UI mode: auto|web|tui|cli (default "auto")
  -tool-search string       Search tools by name/description and exit
  -http                     Run in HTTP transport mode (default: stdio)
  -http-addr string         HTTP listen address (default ":8080")
  -gitlab-url string        Default GitLab URL (per-request override via GITLAB-URL header)
  -skip-tls-verify          Skip TLS certificate verification (default false)
  -meta-tools               Enable meta-tools for tool discovery (default true)
  -enterprise               Enable Enterprise/Premium meta-tools (default false)
  -read-only                Expose only read-only tools (default false)
  -exclude-tools string     Comma-separated tool names to exclude from registration
  -ignore-scopes            Skip PAT scope detection, register all tools (default false)
  -max-http-clients int     Maximum concurrent client sessions (default %d)
  -session-timeout duration Idle session timeout (default %s)
  -auto-update string       Auto-update mode: true|check|false (default "true")
  -auto-update-repo string  GitLab project path for updates (default "%s")
  -auto-update-interval dur How often to check for updates (default %s)
  -auto-update-timeout dur  Timeout for pre-start update download (default %s)
  -auth-mode string         Authentication mode: legacy|oauth (default "legacy")
  -oauth-cache-ttl duration OAuth token cache TTL (default %s, min %s, max %s)
  -trusted-proxy-header str HTTP header with real client IP (e.g. X-Forwarded-For, X-Real-IP)

ENVIRONMENT VARIABLES (stdio mode)
  GITLAB_URL                GitLab instance URL (e.g. https://gitlab.example.com)
  GITLAB_TOKEN              Personal Access Token (glpat-...)
  GITLAB_SKIP_TLS_VERIFY    Skip TLS verification: true/false (default false)
  META_TOOLS                Enable meta-tools: true/false (default true)
  GITLAB_ENTERPRISE         Enable Enterprise/Premium meta-tools: true/false (default false)
  GITLAB_READ_ONLY          Expose only read-only tools: true/false (default false)
  EXCLUDE_TOOLS             Comma-separated tool names to exclude (default empty)
  GITLAB_IGNORE_SCOPES      Skip PAT scope detection: true/false (default false)
  AUTO_UPDATE               Auto-update mode: true/check/false (default true)
  AUTO_UPDATE_REPO          GitLab project for updates (default %s)
  AUTO_UPDATE_INTERVAL      Periodic check interval (default 1h, HTTP mode)
  AUTO_UPDATE_TIMEOUT       Pre-start download timeout (default 60s, range 5s–10m)
  LOG_LEVEL                 Logging: debug/info/warn/error (default info)

JSON CONFIGURATION EXAMPLES

  VS Code / GitHub Copilot (.vscode/mcp.json):
  {
    "servers": {
      "gitlab": {
        "type": "stdio",
        "command": "/usr/local/bin/gitlab-mcp-server",
        "env": {
          "GITLAB_URL": "https://gitlab.example.com",
          "GITLAB_TOKEN": "glpat-your-token",
          "GITLAB_SKIP_TLS_VERIFY": "true",
          "META_TOOLS": "true"
        }
      }
    }
  }

  OpenCode (MCP configuration):
  {
    "mcpServers": {
      "gitlab": {
        "command": "/usr/local/bin/gitlab-mcp-server",
        "env": {
          "GITLAB_URL": "https://gitlab.example.com",
          "GITLAB_TOKEN": "glpat-your-token"
        }
      }
    }
  }

  HTTP mode (single GitLab instance):
  gitlab-mcp-server --http --gitlab-url=https://gitlab.example.com --http-addr=:8080

  HTTP mode (per-request GitLab URL via GITLAB-URL header):
  gitlab-mcp-server --http --http-addr=:8080
`, version, commit,
		projectAuthor, projectDepartment, projectRepository,
		config.DefaultMaxHTTPClients, config.DefaultSessionTimeout,
		config.DefaultAutoUpdateRepo, config.DefaultAutoUpdateInterval,
		config.DefaultAutoUpdateTimeout,
		config.DefaultOAuthCacheTTL, config.MinOAuthCacheTTL, config.MaxOAuthCacheTTL,
		config.DefaultAutoUpdateRepo)
}

// run starts the MCP server with OS signal handling for graceful shutdown.
// Pass a non-nil [httpConfig] for HTTP mode; nil selects stdio mode.
func run(hcfg *httpConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runWithContext(ctx, hcfg)
}

// runWithContext dispatches to HTTP or stdio mode depending on hcfg.
// A non-nil hcfg starts the HTTP server using CLI-flag configuration
// (no GITLAB_TOKEN required). A nil hcfg starts stdio mode using
// environment-variable configuration (GITLAB_TOKEN required).
func runWithContext(ctx context.Context, hcfg *httpConfig) error {
	if hcfg != nil {
		return runHTTP(ctx, hcfg)
	}
	return runStdio(ctx)
}

// runHTTP validates HTTP flags, builds a [config.Config] from them, and
// starts the HTTP server. No GITLAB_TOKEN is needed; each client provides
// its own token per-request via PRIVATE-TOKEN or Authorization: Bearer headers.
// The GitLab URL can be set globally via --gitlab-url or per-request via the
// GITLAB-URL header. At least one must be provided for each request.
func runHTTP(ctx context.Context, hcfg *httpConfig) error {
	if hcfg.gitlabURL != "" {
		u, err := url.Parse(hcfg.gitlabURL)
		if err != nil {
			return fmt.Errorf("--gitlab-url is not a valid URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("--gitlab-url must use http:// or https:// scheme, got %q", u.Scheme)
		}
		if u.Host == "" {
			return errors.New("--gitlab-url must include a host")
		}
		// Normalize so that --gitlab-url=https://x.com/ and a header value
		// of https://x.com hash to the same server-pool session key.
		hcfg.gitlabURL = strings.TrimRight(hcfg.gitlabURL, "/")
	}

	cfg := &config.Config{
		GitLabURL:          hcfg.gitlabURL,
		SkipTLSVerify:      hcfg.skipTLSVerify,
		MetaTools:          hcfg.metaTools,
		Enterprise:         hcfg.enterprise,
		ReadOnly:           hcfg.readOnly,
		SafeMode:           hcfg.safeMode,
		ExcludeTools:       config.ParseCSV(hcfg.excludeTools),
		IgnoreScopes:       hcfg.ignoreScopes,
		MaxHTTPClients:     hcfg.maxHTTPClients,
		SessionTimeout:     hcfg.sessionTimeout,
		RevalidateInterval: hcfg.revalidateInterval,
		UploadMaxFileSize:  config.DefaultMaxFileSize,
		AutoUpdate:         hcfg.autoUpdate,
		AutoUpdateRepo:     hcfg.autoUpdateRepo,
		AutoUpdateInterval: hcfg.autoUpdateInterval,
		AutoUpdateTimeout:  hcfg.autoUpdateTimeout,
		AuthMode:           hcfg.authMode,
		OAuthCacheTTL:      hcfg.oauthCacheTTL,
		TrustedProxyHeader: hcfg.trustedProxyHeader,
		RateLimitRPS:       hcfg.rateLimitRPS,
		RateLimitBurst:     hcfg.rateLimitBurst,
	}

	if cfg.AuthMode == "" {
		cfg.AuthMode = "legacy"
	}
	if cfg.AuthMode != "legacy" && cfg.AuthMode != "oauth" {
		return fmt.Errorf("--auth-mode must be 'legacy' or 'oauth', got %q", cfg.AuthMode)
	}
	// OAuth mode requires a fixed --gitlab-url because the RFC 9728
	// protected-resource metadata and token verifier are initialized at
	// startup and tied to one GitLab instance. Without it, token
	// verification and discovery would be misconfigured.
	if cfg.AuthMode == "oauth" && cfg.GitLabURL == "" {
		return errors.New("--auth-mode=oauth requires --gitlab-url")
	}
	if cfg.AuthMode == "oauth" && cfg.OAuthCacheTTL > 0 {
		if cfg.OAuthCacheTTL < config.MinOAuthCacheTTL {
			return fmt.Errorf("--oauth-cache-ttl %s is below minimum of %s", cfg.OAuthCacheTTL, config.MinOAuthCacheTTL)
		}
		if cfg.OAuthCacheTTL > config.MaxOAuthCacheTTL {
			return fmt.Errorf("--oauth-cache-ttl %s exceeds maximum of %s", cfg.OAuthCacheTTL, config.MaxOAuthCacheTTL)
		}
	}

	if cfg.SessionTimeout > config.MaxSessionTimeout {
		return fmt.Errorf("--session-timeout %s exceeds maximum of %s", cfg.SessionTimeout, config.MaxSessionTimeout)
	}
	if cfg.RevalidateInterval > config.MaxRevalidateInterval {
		return fmt.Errorf("--revalidate-interval %s exceeds maximum of %s", cfg.RevalidateInterval, config.MaxRevalidateInterval)
	}
	if cfg.AutoUpdateTimeout < config.MinAutoUpdateTimeout {
		return fmt.Errorf("--auto-update-timeout %s is below minimum of %s", cfg.AutoUpdateTimeout, config.MinAutoUpdateTimeout)
	}
	if cfg.AutoUpdateTimeout > config.MaxAutoUpdateTimeout {
		return fmt.Errorf("--auto-update-timeout %s exceeds maximum of %s", cfg.AutoUpdateTimeout, config.MaxAutoUpdateTimeout)
	}

	if err := toolutil.ValidateRateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst); err != nil {
		return fmt.Errorf("--rate-limit-rps/--rate-limit-burst: %w", err)
	}

	toolutil.SetUploadConfig(cfg.UploadMaxFileSize)

	// Clean up leftover .old binary from previous updates.
	autoupdate.CleanupOldBinary()

	startAutoUpdate(ctx, cfg)

	return serveHTTP(ctx, cfg, hcfg.addr)
}

// runStdio loads configuration from environment variables (GITLAB_TOKEN
// required), validates GitLab connectivity, and starts the stdio server.
func runStdio(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	toolutil.SetUploadConfig(cfg.UploadMaxFileSize)

	// Clean up leftover .old binary from previous updates.
	autoupdate.CleanupOldBinary()

	// Pre-start update: download, replace, and re-exec on Unix.
	preStartAutoUpdate(cfg)

	client, err := gitlabclient.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("creating gitlab client: %w", err)
	}

	slog.Info("connecting to gitlab", "url", cfg.GitLabURL, "tls_skip", cfg.SkipTLSVerify)
	gitlabVersion, err := client.Initialize(ctx)
	if err != nil {
		slog.Warn("gitlab connectivity check failed — server will start in degraded mode",
			"url", cfg.GitLabURL, "error", err)
		client.EnableLazyInit()
	} else {
		userInfo, userErr := client.CurrentUser(ctx)
		if userErr != nil {
			slog.Warn("could not resolve user identity at startup", "error", userErr)
			slog.Info("gitlab connection verified", "url", cfg.GitLabURL, "version", gitlabVersion)
		} else {
			ctx = toolutil.IdentityToContext(ctx, toolutil.UserIdentity{
				UserID:   strconv.Itoa(userInfo.UserID),
				Username: userInfo.Username,
			})
			slog.Info("gitlab connection verified",
				"url", cfg.GitLabURL,
				"user", userInfo.Username,
				"user_id", userInfo.UserID,
				"version", gitlabVersion,
			)
		}
	}

	// Detect PAT scopes for scope-based tool filtering.
	if !cfg.IgnoreScopes {
		cfg.TokenScopes = gitlabclient.DetectScopes(ctx, client.GL())
		if cfg.TokenScopes == nil {
			slog.Debug("PAT scope detection unavailable — all tools will be registered")
		}
	}

	updater := newUpdaterForTools(cfg)
	server := createServer(client, cfg, updater)
	return serveStdio(ctx, server)
}

// createServer builds a fully configured [*mcp.Server] with all tools,
// resources, and prompts registered for the given GitLab client.
// Used both by stdio mode (single call) and by the HTTP server pool factory.
// If updater is non-nil, server update MCP tools are registered.
func createServer(client *gitlabclient.Client, cfg *config.Config, updater *autoupdate.Updater) *mcp.Server {
	if client == nil {
		panic("createServer: client must not be nil")
	}

	completionHandler := completions.NewHandler(client)
	rootsManager := roots.NewManager()

	server := mcp.NewServer(&mcp.Implementation{
		Name:       "gitlab-mcp-server",
		Title:      "GitLab MCP Server",
		Version:    version,
		WebsiteURL: projectRepository,
		Icons:      toolutil.IconServer,
	}, &mcp.ServerOptions{
		Instructions: "gitlab-mcp-server is a GitLab MCP server providing tools for projects, merge requests, " +
			"issues, branches, tags, releases, repositories, commits, files, groups, members, " +
			"and uploads.\n\n" +
			"PROJECT DISCOVERY — To find the project_id needed for most operations:\n" +
			"1. Read the .git/config file from the workspace to find [remote \"origin\"] url = ...\n" +
			"2. Call gitlab_resolve_project_from_remote with that URL to get the project_id.\n" +
			"3. Alternatively, use gitlab_list_projects (owned=true) or gitlab_search_projects to find projects by name.\n" +
			"4. You can also read the gitlab://workspace/roots resource to discover workspace paths.\n\n" +
			"DEFAULT BRANCH — When generating URLs to repository files or branches:\n" +
			"1. Call gitlab_project_get to retrieve the project metadata, which includes the default_branch field.\n" +
			"2. ALWAYS use the returned default_branch value (e.g. develop, master) instead of assuming 'main'.\n" +
			"3. Projects can use any branch as default, so NEVER hardcode 'main' in URLs.\n\n" +
			"PACKAGE + RELEASE WORKFLOW — When uploading packages and linking them to releases:\n" +
			"1. Preferred: Use gitlab_package_publish_and_link to upload a file and create the release link in one step.\n" +
			"2. Alternative: Use gitlab_package_publish first, then use the 'url' field from its response as the URL for gitlab_release_link_create.\n" +
			"3. NEVER construct package download URLs manually — always use the actual URL returned by the publish tool.\n" +
			"4. RELEASE LINK NAMING: The link_name MUST be the exact filename (e.g. 'checksums.txt.asc'), NEVER add descriptive suffixes like '(GPG signature)'. go-selfupdate and other tools match asset names exactly.\n\n" +
			"RELEASE CREATION — When creating releases:\n" +
			"1. You do NOT need to create the tag first. Provide 'ref' (branch or SHA) in gitlab_release_create and GitLab auto-creates the tag.\n" +
			"2. The response includes 'assets_sources' with auto-generated tar.gz/zip archive URLs — use those, never construct source archive URLs.\n" +
			"3. Use 'tag_message' to create an annotated tag instead of a lightweight one.\n\n" +
			"ID vs IID — GitLab uses two identifiers for issues and merge requests:\n" +
			"1. IID is the project-scoped number shown in URLs and UI (e.g. issue #3, MR !5). Most tools expect IID.\n" +
			"2. ID is the global numeric identifier. Only use gitlab_issue_get_by_id when you have a global ID from another API response.",
		Logger: slog.Default(),
		Capabilities: &mcp.ServerCapabilities{
			Logging:   &mcp.LoggingCapabilities{},
			Tools:     &mcp.ToolCapabilities{ListChanged: true},
			Resources: &mcp.ResourceCapabilities{ListChanged: true},
			Prompts:   &mcp.PromptCapabilities{ListChanged: true},
		},
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return completionHandler.Complete(ctx, req)
		},
		InitializedHandler: func(ctx context.Context, req *mcp.InitializedRequest) {
			if err := rootsManager.Refresh(ctx, req.Session); err != nil {
				slog.Debug("initial roots fetch failed; roots cache left empty", "error", err)
			}
		},
		RootsListChangedHandler: func(ctx context.Context, req *mcp.RootsListChangedRequest) {
			if err := rootsManager.Refresh(ctx, req.Session); err != nil {
				slog.Warn("failed to refresh roots on change notification", "error", err)
			}
		},
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationServerRequest) {
			slog.Debug("received progress notification from client",
				"token", req.Params.ProgressToken,
				"progress", req.Params.Progress,
			)
		},
		KeepAlive: 30 * time.Second,
	})

	if cfg.MetaTools {
		tools.RegisterAllMeta(server, client, cfg.Enterprise)
		tools.RegisterMCPMeta(server, client, updater)
	} else {
		tools.RegisterAll(server, client, cfg.Enterprise)
		serverupdate.RegisterTools(server, updater)
	}

	if len(cfg.ExcludeTools) > 0 {
		removed := removeExcludedTools(server, cfg.ExcludeTools)
		slog.Info("excluded tools by configuration", "excluded", removed, "patterns", cfg.ExcludeTools)
	}

	if cfg.TokenScopes != nil {
		removed := tools.RemoveScopeFilteredTools(server, cfg.TokenScopes)
		if removed > 0 {
			slog.Info("scope-filtered tools", "removed", removed)
		}
	}

	if cfg.ReadOnly {
		removed := removeNonReadOnlyTools(server)
		slog.Info("read-only mode: removed write tools", "removed", removed)
	} else if cfg.SafeMode {
		wrapped := tools.WrapMutatingToolsForSafeMode(server)
		slog.Info("safe mode: wrapped mutating tools with preview handler", "wrapped", wrapped)
	}

	toolCount, err := countRegisteredTools(server)
	if err != nil {
		slog.Warn("failed to count registered tools", "error", err)
	}
	if cfg.MetaTools {
		slog.Info("registered meta-tools", "tools", toolCount)
	} else {
		slog.Info("registered individual tools", "tools", toolCount)
	}

	resources.Register(server, client)
	resources.RegisterWorkspaceRoots(server, rootsManager)
	resources.RegisterWorkflowGuides(server)
	prompts.Register(server, client)

	// Force `additionalProperties: false` on tool input schemas so unknown
	// properties produce actionable validation errors LLMs can self-correct
	// rather than silent acceptance with empty values. Registered as a
	// receiving middleware after every tool/resource/prompt is in place so
	// it sees the final schema set on every tools/list response.
	toolutil.LockdownInputSchemas(server)

	// Inject JSON Schema numeric bounds on the standard pagination
	// parameters so LLM clients see `page >= 1` and `1 <= per_page <= 100`
	// directly in tools/list. Runs after the lockdown so it operates on
	// the same finalized schema set.
	toolutil.EnrichPaginationConstraints(server)

	// Optional per-server tools/call rate limit. In HTTP mode each pooled
	// per-token server gets its own bucket (effectively per-token). In
	// stdio mode the bucket is global to the process. Disabled when
	// RateLimitRPS is 0 (default).
	if limiter := toolutil.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst); limiter != nil {
		toolutil.AttachRateLimit(server, limiter)
		slog.Info("tools/call rate limit enabled",
			"rps", cfg.RateLimitRPS,
			"burst", cfg.RateLimitBurst,
		)
	}

	return server
}

const httpShutdownTimeout = 5 * time.Second

// serveHTTP starts the MCP server in HTTP mode using a [serverpool.ServerPool].
// Each unique token in incoming requests gets its own [*mcp.Server] instance
// backed by a dedicated GitLab client. Requests without a valid authentication
// token are rejected. Sessions expire after cfg.SessionTimeout of inactivity.
// The pool is bounded by cfg.MaxHTTPClients entries with LRU eviction.
func serveHTTP(ctx context.Context, cfg *config.Config, httpAddr string) error {
	slog.Info("starting MCP server in HTTP mode",
		"addr", httpAddr,
		"auth_mode", cfg.AuthMode,
		"max_clients", cfg.MaxHTTPClients,
		"session_timeout", cfg.SessionTimeout,
		"trusted_proxy_header", cfg.TrustedProxyHeader,
		"version", version,
		"commit", commit,
	)

	pool := serverpool.New(cfg, func(client *gitlabclient.Client) *mcp.Server {
		return createServer(client, cfg, nil)
	}, serverpool.WithMaxSize(cfg.MaxHTTPClients),
		serverpool.WithRevalidateInterval(cfg.RevalidateInterval))
	defer pool.Close()

	pool.StartRevalidation(ctx)

	// Build server-card JSON once at startup using an ephemeral MCP server
	// so that /.well-known/mcp/server-card.json is served without authentication.
	serverCardJSON, serverCardErr := buildServerCard(cfg)
	if serverCardErr != nil {
		slog.Warn("failed to build server-card.json, endpoint will return 503", "error", serverCardErr)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /.well-known/mcp/server-card.json", func(w http.ResponseWriter, _ *http.Request) {
		if serverCardJSON == nil {
			http.Error(w, `{"error":"server card unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(serverCardJSON)
	})

	if cfg.AuthMode == "oauth" {
		// In OAuth mode, auth.RequireBearerToken middleware rejects invalid
		// tokens (401) before reaching the server-selector, so authLimiter
		// is unnecessary. The server-selector only needs to extract the
		// pre-validated token for per-token server pool lookup.
		mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			token := serverpool.ExtractToken(r)
			if token == "" {
				slog.Error("request rejected: missing token after OAuth middleware (unexpected)")
				return nil
			}
			gitlabURL, err := serverpool.ExtractGitLabURL(r, cfg.GitLabURL)
			if err != nil {
				slog.Error("request rejected: invalid GITLAB-URL header", "error", err)
				return nil
			}
			// In OAuth mode the bearer token is verified against cfg.GitLabURL
			// by the auth middleware. Refuse to route the request to a
			// different GitLab instance supplied via GITLAB-URL, otherwise a
			// token issued for instance A would be used to call instance B.
			if gitlabURL != cfg.GitLabURL {
				slog.Error("request rejected: GITLAB-URL header does not match --gitlab-url in OAuth mode",
					"expected", cfg.GitLabURL)
				return nil
			}
			server, err := pool.GetOrCreate(token, gitlabURL)
			if err != nil {
				slog.Error("failed to create server for token", "error", err)
				return nil
			}
			return server
		}, &mcp.StreamableHTTPOptions{
			SessionTimeout: cfg.SessionTimeout,
		})

		tokenCache := oauth.NewTokenCache()
		verifier := oauth.NewGitLabVerifier(cfg.GitLabURL, cfg.SkipTLSVerify, cfg.OAuthCacheTTL, tokenCache)
		resourceMetadataURL := "http://" + httpAddr + "/.well-known/oauth-protected-resource"
		authMiddleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
			ResourceMetadataURL: resourceMetadataURL,
			Scopes:              []string{"api"},
		})

		mux.Handle("GET /.well-known/oauth-protected-resource",
			oauth.NewProtectedResourceHandler("http://"+httpAddr+"/mcp", cfg.GitLabURL))
		mux.Handle("/", oauth.NormalizeAuthHeader(authMiddleware(mcpHandler)))

		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					tokenCache.Cleanup()
				}
			}
		}()

		slog.Info("oauth mode enabled",
			"cache_ttl", cfg.OAuthCacheTTL,
			"metadata_endpoint", "/.well-known/oauth-protected-resource",
		)
	} else {
		authLimiter := serverpool.NewAuthRateLimiter(10, 1*time.Minute)
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					authLimiter.Cleanup()
				}
			}
		}()

		mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			ip := clientIP(r, cfg.TrustedProxyHeader)
			if authLimiter.IsBlocked(ip) {
				slog.Warn("request blocked: too many authentication failures", "ip", ip) //#nosec G706 -- slog structured args are not interpolated
				return nil
			}

			token := serverpool.ExtractToken(r)
			if token == "" {
				authLimiter.RecordFailure(ip)
				slog.Error("request rejected: missing authentication token (set PRIVATE-TOKEN header or Authorization: Bearer)")
				return nil
			}
			gitlabURL, err := serverpool.ExtractGitLabURL(r, cfg.GitLabURL)
			if err != nil {
				slog.Error("request rejected: invalid GITLAB-URL header", "error", err)
				return nil
			}
			server, err := pool.GetOrCreate(token, gitlabURL)
			if err != nil {
				authLimiter.RecordFailure(ip)
				slog.Error("failed to create server for token", "error", err)
				return nil
			}
			return server
		}, &mcp.StreamableHTTPOptions{
			SessionTimeout: cfg.SessionTimeout,
		})

		mux.Handle("/", mcpHandler)
	}

	var rootHandler http.Handler = mux
	rootHandler = securityHeadersMiddleware(rootHandler)
	if hosts := allowedHosts(httpAddr); len(hosts) > 0 {
		rootHandler = hostValidationMiddleware(hosts, rootHandler)
	}

	httpServer := &http.Server{
		Addr:              httpAddr,
		Handler:           rootHandler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		slog.Info("HTTP server shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpShutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown: %w", err)
		}
		return nil
	case err := <-serverErr:
		return fmt.Errorf("mcp server error (http): %w", err)
	}
}

// healthResponse is the JSON body returned by the /health endpoint.
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// clientIP extracts the real client IP from the request. When a trusted
// proxy header is configured (e.g. Fly-Client-IP, X-Real-IP, X-Forwarded-For),
// its value is used instead of RemoteAddr.
//
// For multi-value headers like X-Forwarded-For — where well-behaved proxies
// *append* to an existing header — the rightmost value is returned because
// the leftmost entry is client-supplied and therefore spoofable. Operators
// who configure this flag must ensure the trusted proxy is the only ingress
// path to the server; otherwise any client can set the header directly and
// bypass per-IP rate limiting.
func clientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if val := r.Header.Get(trustedHeader); val != "" {
			// For comma-separated values (X-Forwarded-For style), take the
			// rightmost non-empty IP — it is the most recent proxy-appended
			// hop and cannot be spoofed by an untrusted upstream client.
			parts := strings.Split(val, ",")
			for _, part := range slices.Backward(parts) {
				if ip := strings.TrimSpace(part); ip != "" {
					return ip
				}
			}
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// parseLogLevel converts a LOG_LEVEL string to slog.Level.
// Accepts: debug, info, warn, error (case-insensitive). Defaults to info.
func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// allowedHosts computes the set of valid Host header values based on the
// listen address. Returns nil when binding to all interfaces (0.0.0.0/::),
// which skips host validation — suitable for reverse-proxy deployments.
func allowedHosts(addr string) map[string]bool {
	host, _, _ := net.SplitHostPort(addr)
	if host == "" || host == "0.0.0.0" || host == "::" {
		return nil
	}
	return map[string]bool{
		host:        true,
		"localhost": true,
		"127.0.0.1": true,
		"::1":       true,
	}
}

// securityHeadersMiddleware adds standard security headers to every HTTP response
// and enforces a request body size limit.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	const maxBodySize = 10 << 20 // 10 MB
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		next.ServeHTTP(w, r)
	})
}

// hostValidationMiddleware rejects requests whose Host header does not match
// the allowed set, mitigating DNS rebinding attacks on local servers.
func hostValidationMiddleware(allowed map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !allowed[host] {
			slog.Warn("request blocked: invalid Host header", "host", r.Host) //#nosec G706 -- slog structured args are not interpolated
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// healthHandler responds with HTTP 200 and a JSON body for container healthchecks
// and load-balancer probes. It does not require authentication.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:  "ok",
		Version: version,
		Commit:  commit,
	})
}

// buildServerCard creates a Smithery-compatible server-card JSON by spinning up
// an ephemeral MCP server with a dummy GitLab client, listing all registered
// tools, resources, resource templates, and prompts via in-memory MCP session,
// and marshaling the result. The dummy client is never used for API calls —
// it only satisfies createServer's non-nil requirement so that registration
// can proceed.
//
// The card includes per-tool OutputSchema, Annotations, and Title so external
// scanners (Smithery, Glama, MCP Hive) get the full metadata without needing
// to authenticate against the live MCP endpoint.
func buildServerCard(cfg *config.Config) ([]byte, error) {
	// Use configured URL or a placeholder — the dummy client only needs a
	// parseable URL to register tools; it never makes real API calls.
	gitlabURL := cfg.GitLabURL
	if gitlabURL == "" {
		gitlabURL = "https://gitlab.com"
	}

	dummyClient, err := gitlabclient.NewClientWithToken(gitlabURL, "dummy-token-for-tool-discovery", cfg.SkipTLSVerify)
	if err != nil {
		return nil, fmt.Errorf("creating dummy client: %w", err)
	}

	srv := createServer(dummyClient, cfg, nil)

	st, ct := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverSession, err := srv.Connect(ctx, st, nil)
	if err != nil {
		return nil, fmt.Errorf("server connect: %w", err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "server-card-builder", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	toolsResult, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	type serverCardTool struct {
		Name         string               `json:"name"`
		Title        string               `json:"title,omitempty"`
		Description  string               `json:"description,omitempty"`
		InputSchema  any                  `json:"inputSchema,omitempty"`
		OutputSchema any                  `json:"outputSchema,omitempty"`
		Annotations  *mcp.ToolAnnotations `json:"annotations,omitempty"`
	}

	cardTools := make([]serverCardTool, 0, len(toolsResult.Tools))
	for _, t := range toolsResult.Tools {
		cardTools = append(cardTools, serverCardTool{
			Name:         t.Name,
			Title:        t.Title,
			Description:  t.Description,
			InputSchema:  t.InputSchema,
			OutputSchema: t.OutputSchema,
			Annotations:  t.Annotations,
		})
	}

	type serverCardResource struct {
		URI         string           `json:"uri"`
		Name        string           `json:"name,omitempty"`
		Title       string           `json:"title,omitempty"`
		Description string           `json:"description,omitempty"`
		MIMEType    string           `json:"mimeType,omitempty"`
		Annotations *mcp.Annotations `json:"annotations,omitempty"`
	}

	resourcesResult, err := session.ListResources(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}
	cardResources := make([]serverCardResource, 0, len(resourcesResult.Resources))
	for _, r := range resourcesResult.Resources {
		cardResources = append(cardResources, serverCardResource{
			URI:         r.URI,
			Name:        r.Name,
			Title:       r.Title,
			Description: r.Description,
			MIMEType:    r.MIMEType,
			Annotations: r.Annotations,
		})
	}

	type serverCardResourceTemplate struct {
		URITemplate string           `json:"uriTemplate"`
		Name        string           `json:"name,omitempty"`
		Title       string           `json:"title,omitempty"`
		Description string           `json:"description,omitempty"`
		MIMEType    string           `json:"mimeType,omitempty"`
		Annotations *mcp.Annotations `json:"annotations,omitempty"`
	}

	templatesResult, err := session.ListResourceTemplates(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list resource templates: %w", err)
	}
	cardTemplates := make([]serverCardResourceTemplate, 0, len(templatesResult.ResourceTemplates))
	for _, rt := range templatesResult.ResourceTemplates {
		cardTemplates = append(cardTemplates, serverCardResourceTemplate{
			URITemplate: rt.URITemplate,
			Name:        rt.Name,
			Title:       rt.Title,
			Description: rt.Description,
			MIMEType:    rt.MIMEType,
			Annotations: rt.Annotations,
		})
	}

	type serverCardPromptArgument struct {
		Name        string `json:"name"`
		Title       string `json:"title,omitempty"`
		Description string `json:"description,omitempty"`
		Required    bool   `json:"required,omitempty"`
	}
	type serverCardPrompt struct {
		Name        string                     `json:"name"`
		Title       string                     `json:"title,omitempty"`
		Description string                     `json:"description,omitempty"`
		Arguments   []serverCardPromptArgument `json:"arguments,omitempty"`
	}

	promptsResult, err := session.ListPrompts(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list prompts: %w", err)
	}
	cardPrompts := make([]serverCardPrompt, 0, len(promptsResult.Prompts))
	for _, p := range promptsResult.Prompts {
		args := make([]serverCardPromptArgument, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			args = append(args, serverCardPromptArgument{
				Name:        a.Name,
				Title:       a.Title,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		cardPrompts = append(cardPrompts, serverCardPrompt{
			Name:        p.Name,
			Title:       p.Title,
			Description: p.Description,
			Arguments:   args,
		})
	}

	card := map[string]any{
		"serverInfo": map[string]any{
			"name":    "gitlab-mcp-server",
			"version": version,
		},
		"authentication": map[string]any{
			"required": true,
			"schemes":  []string{"header-token"},
		},
		"tools":             cardTools,
		"resources":         cardResources,
		"resourceTemplates": cardTemplates,
		"prompts":           cardPrompts,
	}

	return json.Marshal(card)
}

// serveStdio starts the MCP server using stdio transport.
// It blocks until the context is canceled or an error occurs.
func serveStdio(ctx context.Context, server *mcp.Server) error {
	slog.Info("starting MCP server", "transport", "stdio", "version", version, "commit", commit)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server error: %w", err)
	}
	return nil
}

// preStartAutoUpdate runs the pre-start update check for stdio mode.
// Downloads, replaces the binary, and re-execs on Unix.
// Non-fatal: errors are logged and do not prevent startup.
func preStartAutoUpdate(cfg *config.Config) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("autoupdate: pre-start auto-update panicked — continuing without update", "panic", r)
		}
	}()

	mode, err := autoupdate.ParseMode(cfg.AutoUpdate)
	if err != nil {
		slog.Warn("autoupdate: invalid AUTO_UPDATE value, skipping", "error", err)
		return
	}
	if mode == autoupdate.ModeDisabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.AutoUpdateTimeout)
	defer cancel()

	result := autoupdate.PreStartUpdate(ctx, autoupdate.Config{
		Mode:           mode,
		Repository:     cfg.AutoUpdateRepo,
		CurrentVersion: version,
	})

	if result.Updated && result.NewVersion != "" {
		slog.Info("autoupdate: binary updated",
			"new_version", result.NewVersion,
			"exec_failed", result.ExecFailed,
		)
	}
}

// newUpdaterForTools creates an [*autoupdate.Updater] for the MCP server-update
// tools. Returns nil (safe for RegisterTools) if auto-update is disabled or
// initialisation fails.
func newUpdaterForTools(cfg *config.Config) *autoupdate.Updater {
	mode, err := autoupdate.ParseMode(cfg.AutoUpdate)
	if err != nil || mode == autoupdate.ModeDisabled {
		return nil
	}
	u, err := autoupdate.NewUpdater(autoupdate.Config{
		Mode:           mode,
		Repository:     cfg.AutoUpdateRepo,
		CurrentVersion: version,
	})
	if err != nil {
		slog.Warn("autoupdate: could not create updater for MCP tools", "error", err)
		return nil
	}
	return u
}

// startAutoUpdate initializes background periodic update checks for HTTP mode.
func startAutoUpdate(ctx context.Context, cfg *config.Config) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("autoupdate: background auto-update panicked — continuing without updates", "panic", r)
		}
	}()

	mode, err := autoupdate.ParseMode(cfg.AutoUpdate)
	if err != nil {
		slog.Warn("autoupdate: invalid auto-update mode, skipping", "error", err)
		return
	}
	if mode == autoupdate.ModeDisabled {
		return
	}

	u, err := autoupdate.NewUpdater(autoupdate.Config{
		Mode:           mode,
		Repository:     cfg.AutoUpdateRepo,
		Interval:       cfg.AutoUpdateInterval,
		Timeout:        cfg.AutoUpdateTimeout,
		CurrentVersion: version,
	})
	if err != nil {
		slog.Warn("autoupdate: could not initialize periodic updater", "error", err)
		return
	}

	u.StartPeriodicCheck(ctx)
}

// countRegisteredTools returns the number of tools registered on the server
// by connecting an ephemeral in-memory client session and calling ListTools.
func countRegisteredTools(server *mcp.Server) (int, error) {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return 0, fmt.Errorf("server connect: %w", err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "counter", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return 0, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("list tools: %w", err)
	}
	return len(result.Tools), nil
}

// removeNonReadOnlyTools lists all registered tools via an ephemeral in-memory
// session and removes those that do not have ReadOnlyHint set to true.
// Returns the number of tools removed.
func removeNonReadOnlyTools(server *mcp.Server) int {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		slog.Error("removeNonReadOnlyTools: server connect failed", "error", err)
		return 0
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "readonly-filter", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		slog.Error("removeNonReadOnlyTools: client connect failed", "error", err)
		return 0
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		slog.Error("removeNonReadOnlyTools: list tools failed", "error", err)
		return 0
	}

	var toRemove []string
	for _, t := range result.Tools {
		if t.Annotations == nil || !t.Annotations.ReadOnlyHint {
			toRemove = append(toRemove, t.Name)
		}
	}

	if len(toRemove) > 0 {
		server.RemoveTools(toRemove...)
	}
	return len(toRemove)
}

// removeExcludedTools lists all registered tools and removes those whose name
// matches any entry in the exclusion list. Matching is exact by tool name.
// Returns the number of tools removed.
func removeExcludedTools(server *mcp.Server, exclude []string) int {
	if len(exclude) == 0 {
		return 0
	}

	excludeSet := make(map[string]struct{}, len(exclude))
	for _, name := range exclude {
		excludeSet[name] = struct{}{}
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		slog.Error("removeExcludedTools: server connect failed", "error", err)
		return 0
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "exclude-filter", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		slog.Error("removeExcludedTools: client connect failed", "error", err)
		return 0
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		slog.Error("removeExcludedTools: list tools failed", "error", err)
		return 0
	}

	var toRemove []string
	for _, t := range result.Tools {
		if _, ok := excludeSet[t.Name]; ok {
			toRemove = append(toRemove, t.Name)
		}
	}

	if len(toRemove) > 0 {
		server.RemoveTools(toRemove...)
	}
	return len(toRemove)
}

// runToolSearch creates an in-memory MCP server, lists all tools, and
// prints those matching every space-separated search term (AND logic,
// case-insensitive match on name + description). Then it exits.
func runToolSearch(query string, metaTools, enterprise bool) {
	if err := doToolSearch(query, metaTools, enterprise); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func doToolSearch(query string, metaTools, enterprise bool) error {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "search", Version: version}, &mcp.ServerOptions{PageSize: 2000})

	if metaTools {
		tools.RegisterAllMeta(server, nil, enterprise)
	} else {
		tools.RegisterAll(server, nil, enterprise)
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		return fmt.Errorf("connect error: %w", err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "search-client", Version: "0"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		return fmt.Errorf("connect error: %w", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools error: %w", err)
	}

	var matches []*mcp.Tool
	for _, t := range result.Tools {
		haystack := strings.ToLower(t.Name + " " + t.Description)
		allMatch := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				allMatch = false
				break
			}
		}
		if allMatch {
			matches = append(matches, t)
		}
	}

	if len(matches) == 0 {
		fmt.Printf("No tools found matching %q\n", query)
		return nil
	}

	fmt.Printf("Found %d tool(s) matching %q:\n\n", len(matches), query)
	fmt.Printf("%-45s %s\n", "NAME", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 120))
	for _, t := range matches {
		desc := t.Description
		if len([]rune(desc)) > 80 {
			desc = string([]rune(desc)[:77]) + "..."
		}
		fmt.Printf("%-45s %s\n", t.Name, desc)
	}
	return nil
}

// setupAutoUpdateRedaction wraps the current global slog handler with a
// handler that redacts the auto-update GitLab URL (and its host) from log
// entries whose message starts with "autoupdate:". Regular GitLab operation
// logs are left untouched so the user's configured GITLAB_URL remains visible.
func setupAutoUpdateRedaction(autoUpdateURL string) {
	if autoUpdateURL == "" {
		return
	}
	var redactStrings []string
	redactStrings = append(redactStrings, autoUpdateURL)
	if host := extractHost(autoUpdateURL); host != "" {
		redactStrings = append(redactStrings, host)
	}
	slog.SetDefault(slog.New(&autoUpdateRedactHandler{
		base:          slog.Default().Handler(),
		redactStrings: redactStrings,
	}))
}

// extractHost returns the host (with port) from a URL string, or empty on error.
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// autoUpdateRedactHandler wraps a [slog.Handler] and replaces occurrences of
// the auto-update GitLab URL (and host) with "[REDACTED]" in string attributes,
// but only for log records whose message starts with "autoupdate:".
type autoUpdateRedactHandler struct {
	base          slog.Handler
	redactStrings []string
}

// Enabled implements [slog.Handler] by delegating to the wrapped base handler.
func (h *autoUpdateRedactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

// Handle implements [slog.Handler]. For records whose message starts with
// "autoupdate:", it redacts the configured strings from string-valued
// attributes before forwarding to the base handler. Other records pass through
// unchanged.
func (h *autoUpdateRedactHandler) Handle(ctx context.Context, r slog.Record) error {
	if !strings.HasPrefix(r.Message, "autoupdate:") {
		return h.base.Handle(ctx, r)
	}
	nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.base.Handle(ctx, nr)
}

// WithAttrs implements [slog.Handler] by returning a new redacting handler
// wrapping the base handler with the additional attributes.
func (h *autoUpdateRedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &autoUpdateRedactHandler{base: h.base.WithAttrs(attrs), redactStrings: h.redactStrings}
}

// WithGroup implements [slog.Handler] by returning a new redacting handler
// wrapping the base handler with the named group.
func (h *autoUpdateRedactHandler) WithGroup(name string) slog.Handler {
	return &autoUpdateRedactHandler{base: h.base.WithGroup(name), redactStrings: h.redactStrings}
}

func (h *autoUpdateRedactHandler) redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		s := a.Value.String()
		for _, r := range h.redactStrings {
			s = strings.ReplaceAll(s, r, "[REDACTED]")
		}
		a.Value = slog.StringValue(s)
	}
	return a
}
