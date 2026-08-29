// Command server is the MCP server entry point for gitlab-mcp-server.
// In stdio mode, configuration comes from environment variables (.env / exports).
// In HTTP mode, configuration comes from CLI flags; no GITLAB_TOKEN is required
// at startup — each client provides its own token per-request.
// The --shutdown flag terminates running instances before external updaters
// replace the binary on disk.
//
// # Modes
//
// Stdio mode creates one MCP server from environment configuration and serves
// JSON-RPC over standard input and output. HTTP mode creates a streamable HTTP
// handler backed by a server pool so each token and GitLab URL pair receives an
// isolated MCP server configuration.
//
// # Startup Flow
//
// The command validates configuration, registers tools, resources, prompts,
// completions, progress, and elicitation support, then
// starts the selected transport:
//
//	server
//	    |
//	    v
//	configuration and auto-update setup
//	    |
//	    v
//	MCP capability registration
//	    |
//	    v
//	stdio or HTTP transport
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
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
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cachehints"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/capguard"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/clientcompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/completions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/health"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/serverupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/wizard"
)

// version and commit are set at build time via -ldflags.
// The VERSION file at the repo root is the single source of truth for version.
//
// When neither is stamped — a plain `go build`, or `go install <module>@vX.Y.Z`
// — [resolveBuildVersion] recovers what it can from the embedded build info so
// the handshake never reports a bare "dev" to a client that has a real version
// to show.
var (
	version = "dev"
	commit  = "none"
)

func init() {
	version, commit = resolveBuildVersion(version, commit, debug.ReadBuildInfo)
}

// resolveBuildVersion fills in an unstamped version and commit from the module
// build info Go embeds in every binary. `go install module@version` records the
// module version, and any VCS build records the revision, so a source install
// still identifies itself. Values passed via -ldflags always win: release
// binaries are stamped from the VERSION file, which is the source of truth.
//
// readBuildInfo is injected so tests can exercise the paths where build info is
// unavailable or carries no usable values.
func resolveBuildVersion(ldflagsVersion, ldflagsCommit string, readBuildInfo func() (*debug.BuildInfo, bool)) (resolvedVersion, resolvedCommit string) {
	resolvedVersion, resolvedCommit = ldflagsVersion, ldflagsCommit
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return resolvedVersion, resolvedCommit
	}

	// "(devel)" is what the toolchain records for a build from a working tree
	// rather than a module version; it is no more informative than "dev".
	if resolvedVersion == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		resolvedVersion = strings.TrimPrefix(info.Main.Version, "v")
	}
	if resolvedCommit == "none" {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				resolvedCommit = setting.Value
				break
			}
		}
	}
	return resolvedVersion, resolvedCommit
}

// Project metadata.
const (
	projectAuthor     = "Jose Manuel Requena Plens"
	projectDepartment = ""
	projectRepository = "https://github.com/jmrplens/gitlab-mcp-server"

	// projectWebsite is the Implementation.WebsiteURL advertised at handshake.
	// It is the vanity redirect to the documentation site rather than the
	// repository: a client rendering serverInfo shows this to an end user, for
	// whom the guides are more useful than a source tree. It matches the
	// homepage/documentation fields in mcpb/manifest.json and
	// .plugin/plugin.json.
	projectWebsite = "https://jmrp.io/docs/gitlab-mcp-server"

	// projectDescription is the server's self-description at handshake, shown
	// wherever a client or registry renders the MCP Implementation. It is kept
	// free of tool counts and tier names so it never drifts out of date; the
	// per-channel manifests (server.json, lhm.plugin.json, mcpb/manifest.json)
	// carry the marketing copy that does.
	projectDescription = "Model Context Protocol server for GitLab: projects, issues, merge requests, " +
		"pipelines, repositories, releases, groups, and admin workflows over the " +
		"GitLab REST and GraphQL APIs."
)

// httpConfig holds CLI-flag configuration for HTTP server mode.
// When non-nil is passed to [runWithContext], the server starts in HTTP mode
// without requiring a GITLAB_TOKEN — each client must provide its own token
// via PRIVATE-TOKEN header or Authorization: Bearer.
type httpConfig struct {
	addr string
	// gitlabURL is the deployment's default instance: the first entry of
	// gitlabURLs, kept as its own field because every consumer that only
	// ever handled one instance still reads exactly that.
	gitlabURL string
	// gitlabURLs is every instance --gitlab-url published, in order. More
	// than one turns the GITLAB-URL header into a choice among them.
	gitlabURLs    repeatedFlag
	skipTLSVerify bool
	metaTools     bool
	metaToolsSet  bool
	// setFlags names the flags the operator passed explicitly, which is what
	// separates "chose the default" from "did not choose", and therefore
	// whether the environment may supply the value instead.
	setFlags              map[string]bool
	toolSurface           string
	capabilitySurface     string
	tier                  string
	tierSet               bool
	readOnly              bool
	safeMode              bool
	embeddedResources     bool
	excludeTools          string
	ignoreScopes          bool
	maxHTTPClients        int
	sessionTimeout        time.Duration
	autoUpdate            string
	autoUpdateRepo        string
	autoUpdateInterval    time.Duration
	autoUpdateTimeout     time.Duration
	revalidateInterval    time.Duration
	poolIdleTimeout       time.Duration
	authMode              string
	publicURL             string
	resourceDocumentation string
	oauthCacheTTL         time.Duration
	oauthClientUID        string
	trustedProxyHeader    string
	trustedOrigins        string
	rateLimitRPS          float64
	rateLimitBurst        int
	metaParamSchema       string
	httpIdleTimeout       time.Duration
	stateless             bool
	jsonResponse          bool
	maxRequestBodyBytes   int64
	tlsCert               string
	tlsKey                string
	// socketMode is the raw --http-socket-mode text; socketModeParsed is
	// what runHTTP resolved it to, so the octal is rejected at startup with
	// the flag's name rather than deep inside the listener.
	socketMode       string
	socketModeParsed os.FileMode
}

// HTTP server timeout defaults. These bound the standard library [http.Server]
// in HTTP mode. ReadHeaderTimeout/ReadTimeout guard request reads (slowloris)
// and do not affect already-established streaming responses.
const (
	// defaultHTTPIdleTimeout is the default for --http-idle-timeout. 0 means idle
	// connection closure is disabled at the HTTP layer, so the MCP --session-timeout
	// governs idle session lifetime. This prevents the HTTP server from severing
	// long-lived Streamable HTTP (SSE) connections that carry server-initiated
	// notifications and keep-alive pings. Operators can set a positive value to
	// recycle idle connections sooner.
	defaultHTTPIdleTimeout time.Duration = 0

	// baseHTTPReadHeaderTimeout bounds reading request headers.
	baseHTTPReadHeaderTimeout = 10 * time.Second

	// baseHTTPReadTimeout bounds reading the full request. Streamable HTTP request
	// bodies (JSON-RPC) are small, so this does not affect established SSE streams.
	baseHTTPReadTimeout = 30 * time.Second

	// baseHTTPWriteTimeout is the fixed global response write timeout. It is kept at a
	// safe default to protect standard endpoints (e.g. /health,
	// /.well-known/mcp/server-card.json) from slow-write resource exhaustion
	// (Slowloris). Long-lived SSE streams instead disable their write deadline
	// dynamically (see sseWriteDeadlineMiddleware), because the MCP go-sdk SSE writer
	// never resets it and WriteTimeout would otherwise sever the stream prematurely.
	baseHTTPWriteTimeout = 60 * time.Second

	// idleTimeoutDisabled is the sentinel used when idle closure is disabled (input 0).
	// Go falls back to ReadTimeout when IdleTimeout == 0; this large value instead
	// keeps idle keep-alive connections effectively unrestricted.
	idleTimeoutDisabled = 10 * 365 * 24 * time.Hour
)

// main parses CLI flags, handles one-shot commands such as --help and
// --tool-search, and dispatches into stdio or HTTP server mode.
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
	flag.Var(&hcfg.gitlabURLs, "gitlab-url", "GitLab instance URL; omit to require a per-request GITLAB-URL header. Repeat (or comma-separate) to publish several instances, the first being the default; a GITLAB-URL header is then honored only if it names one of them")
	flag.BoolVar(&hcfg.skipTLSVerify, "skip-tls-verify", false, "Skip TLS certificate verification")
	flag.BoolVar(&hcfg.metaTools, "meta-tools", false, "Legacy boolean tool selector; prefer --tool-surface")
	flag.StringVar(&hcfg.toolSurface, "tool-surface", "", "Tool surface: dynamic (default), meta, individual")
	flag.StringVar(&hcfg.capabilitySurface, "capability-surface", config.DefaultCapabilitySurface, "Capability surface: full (default) or minimal")
	flag.StringVar(&hcfg.tier, "tier", "", "Force licensing tier (free, ce, premium, ultimate); omit to detect per server entry")
	flag.BoolVar(&hcfg.readOnly, "read-only", false, "Expose only read-only tools (no create/update/delete)")
	flag.BoolVar(&hcfg.safeMode, "safe-mode", false, "Intercept mutating tools and return a preview instead of executing")
	flag.BoolVar(&hcfg.embeddedResources, "embedded-resources", true, "Embed canonical MCP resource URIs in get_* tool results")
	flag.StringVar(&hcfg.excludeTools, "exclude-tools", "", "Comma-separated list of tool names to exclude from registration")
	flag.BoolVar(&hcfg.ignoreScopes, "ignore-scopes", false, "Skip PAT scope detection and register all tools")
	flag.IntVar(&hcfg.maxHTTPClients, "max-http-clients", config.DefaultMaxHTTPClients, "Maximum unique (token, GitLab URL) server entries kept in the pool; bounds pooled entries, not sessions or concurrent requests")
	flag.DurationVar(&hcfg.sessionTimeout, "session-timeout", config.DefaultSessionTimeout, "Idle MCP session timeout; applies to --stateless=false only (under the default stateless transport each POST's session ends with its response)")
	flag.StringVar(&hcfg.autoUpdate, "auto-update", "true", "Auto-update mode: true (auto-apply), check (log-only), false (disabled)")
	flag.StringVar(&hcfg.autoUpdateRepo, "auto-update-repo", config.DefaultAutoUpdateRepo, "GitHub repository for update checks")
	flag.DurationVar(&hcfg.autoUpdateInterval, "auto-update-interval", config.DefaultAutoUpdateInterval, "How often to check for updates")
	flag.DurationVar(&hcfg.autoUpdateTimeout, "auto-update-timeout", config.DefaultAutoUpdateTimeout, "Timeout for startup/background update checks (range 5s\u201310m)")
	flag.DurationVar(&hcfg.revalidateInterval, "revalidate-interval", config.DefaultRevalidateInterval, "Token re-validation interval (0 to disable)")
	flag.DurationVar(&hcfg.poolIdleTimeout, "pool-idle-timeout", config.DefaultPoolIdleTimeout, "Reclaim a pooled per-token-and-URL server entry after this long unused (0 to disable)")
	flag.StringVar(&hcfg.authMode, "auth-mode", "legacy", "Authentication mode: legacy (default) or oauth")
	flag.StringVar(&hcfg.publicURL, "public-url", "", "Externally reachable origin of this deployment (https). Required with --auth-mode=oauth: it is the RFC 9728 protected-resource identifier")
	flag.StringVar(&hcfg.resourceDocumentation, "resource-documentation", "", "https URL published as RFC 9728 resource_documentation; point it at a page describing your own OAuth application (its client ID and registered redirect URIs). Empty publishes this project's OAuth setup guide")
	flag.DurationVar(&hcfg.oauthCacheTTL, "oauth-cache-ttl", config.DefaultOAuthCacheTTL, "OAuth token cache TTL")
	flag.StringVar(&hcfg.oauthClientUID, "oauth-client-uid", "", "Comma-separated GitLab OAuth application uids whose tokens this deployment admits. Empty (default) admits any credential the instance accepts; setting it also refuses personal access tokens, which belong to no application")
	flag.StringVar(&hcfg.trustedProxyHeader, "trusted-proxy-header", "", "HTTP header containing the real client IP (e.g. X-Forwarded-For, X-Real-IP)")
	flag.StringVar(&hcfg.trustedOrigins, "trusted-origins", "", "Comma-separated absolute origins (scheme://host[:port], e.g. an IP for local deploys) allowed to make cross-origin browser requests; '*' accepts any origin (disables the protection); empty rejects all. The --public-url origin is trusted automatically")
	flag.Float64Var(&hcfg.rateLimitRPS, "rate-limit-rps", config.DefaultHTTPRateLimitRPS, "Per-server tools/call rate limit in requests/second (0 disables it)")
	flag.IntVar(&hcfg.rateLimitBurst, "rate-limit-burst", config.DefaultRateLimitBurst, "Token-bucket burst size when --rate-limit-rps > 0")
	flag.StringVar(&hcfg.metaParamSchema, "meta-param-schema", config.DefaultMetaParamSchema, "Meta-tool input schema mode: opaque (default), compact, full")
	flag.DurationVar(&hcfg.httpIdleTimeout, "http-idle-timeout", defaultHTTPIdleTimeout, "HTTP server idle connection timeout; 0 (default) disables idle closure so nothing above the transport closes idle connections; set a positive duration to recycle idle connections sooner")
	flag.BoolVar(&hcfg.stateless, "stateless", true, "Stateless streamable HTTP (default; required for MCP protocol 2026-07-28 over HTTP): no Mcp-Session-Id tracking, each POST is self-contained, GET/DELETE return 405. Use -stateless=false to restore legacy stateful sessions")
	flag.BoolVar(&hcfg.jsonResponse, "json-response", false, "Return application/json responses instead of text/event-stream (SSE)")
	flag.StringVar(&hcfg.tlsCert, "tls-cert", "", "PEM certificate file; serves HTTPS on the listener itself (requires --tls-key)")
	flag.StringVar(&hcfg.tlsKey, "tls-key", "", "PEM private key file matching --tls-cert")
	flag.StringVar(&hcfg.socketMode, "http-socket-mode", "", "Permission mode for a unix socket named by --http-addr, in octal (default 0660)")
	flag.Int64Var(&hcfg.maxRequestBodyBytes, "max-request-body-bytes", 0, "Maximum streamable HTTP request body size in bytes; 0 uses the SDK default (4 MiB)")
	flag.Parse()
	hcfg.setFlags = make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		hcfg.setFlags[f.Name] = true
		switch f.Name {
		case "tier":
			hcfg.tierSet = true
		case "meta-tools":
			hcfg.metaToolsSet = true
		}
	})

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
		toolSurface, _, surfaceErr := config.ParseToolSurface(hcfg.toolSurface, legacyMetaToolsFlagValue(&hcfg))
		if surfaceErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", surfaceErr)
			exitProcess(1)
			return
		}
		searchTier, _, tierErr := resolveHTTPTier(&hcfg)
		if tierErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", tierErr)
			exitProcess(1)
			return
		}
		runToolSearch(toolSearch, toolSurface, searchTier)
		return
	}

	autoWizard := !useHTTP && !showHelp && !showVersion &&
		wizard.IsInteractiveTerminal() &&
		os.Getenv("GITLAB_TOKEN") == "" && os.Getenv("GITLAB_URL") == ""
	if forceSetup || autoWizard {
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
  -http-addr string         Listen address: host:port, or a path to bind a unix socket (default ":8080")
  -http-socket-mode str     Octal permission mode for a unix socket (default "0660")
  -tls-cert string          PEM certificate; serves HTTPS on the listener (requires -tls-key)
  -tls-key string           PEM private key matching -tls-cert
  -gitlab-url string        GitLab URL; omit to require per-request GITLAB-URL header. Repeatable to publish several instances
  -skip-tls-verify          Skip TLS certificate verification when calling GitLab (default false)
  -meta-tools               Legacy boolean tool selector; prefer -tool-surface
  -tool-surface string      Tool surface: dynamic|meta|individual (default dynamic)
  -capability-surface str   Capability surface: full|minimal (default full)
  -meta-param-schema str    Meta-tool input schema mode: opaque|compact|full (default opaque)
  -tier string              Force licensing tier: free|ce|premium|ultimate; omit to detect per server entry
  -read-only                Expose only read-only tools (default false)
  -safe-mode                Intercept mutating tools and return a preview instead of executing
  -embedded-resources       Embed canonical MCP resource links in get_* tool results (default true)
  -exclude-tools string     Comma-separated tool names to exclude from registration
  -ignore-scopes            Skip PAT scope detection, register all tools (default false)
  -max-http-clients int     Maximum unique (token, GitLab URL) pool entries; not sessions or concurrent requests (default %d)
  -session-timeout duration Idle MCP session timeout; --stateless=false only (default %s)
  -pool-idle-timeout dur    Reclaim a pooled per-token-and-URL server entry after this long unused (default %s, 0 to disable)
  -http-idle-timeout dur    HTTP server idle connection timeout; 0 (default) disables idle closure so -session-timeout governs
  -stateless                Stateless streamable HTTP (default true; required for protocol 2026-07-28). Use -stateless=false for legacy stateful sessions
  -json-response            Return application/json responses instead of SSE (default false)
  -max-request-body-bytes n Maximum streamable HTTP request body bytes (0 = SDK default 4 MiB)
  -auto-update string       Auto-update mode: true|check|false (default "true")
  -auto-update-repo string  GitHub repository for update checks (default "%s")
  -auto-update-interval dur How often to check for updates (default %s, HTTP mode)
  -auto-update-timeout dur  Timeout for startup/background update checks (default %s)
  -auth-mode string         Authentication mode: legacy|oauth (default "legacy")
  -resource-documentation string
                            https URL published as RFC 9728 resource_documentation (default: this project's OAuth setup guide)
  -oauth-cache-ttl duration OAuth token cache TTL (default %s, min %s, max %s)
  -oauth-client-uid string  Comma-separated GitLab OAuth application uids whose tokens are admitted (default: any)
  -public-url string        Externally reachable https origin; required with -auth-mode=oauth (RFC 9728 resource identifier)
  -trusted-proxy-header str HTTP header with real client IP (e.g. X-Forwarded-For, X-Real-IP)
  -revalidate-interval dur  How often pooled tokens are re-validated against GitLab (default %s, 0 to disable)
  -trusted-origins string   Origins allowed to make cross-origin browser requests ('*' accepts any; empty rejects all)
  -rate-limit-rps float     Per-server tools/call rate limit (default 10; 0 disables it)
  -rate-limit-burst int     Token-bucket burst size when --rate-limit-rps > 0 (default %d)

ENVIRONMENT VARIABLES (stdio mode)
  GITLAB_URL                GitLab instance URL (default: %s; set for self-managed instances)
  GITLAB_TOKEN              Personal Access Token (glpat-...)
  GITLAB_SKIP_TLS_VERIFY    Skip TLS verification: true/false (default false)
  TOOL_SURFACE              Canonical tool surface: dynamic|meta|individual (default dynamic)
  META_TOOLS                Deprecated legacy selector: true|false|dynamic; ignored when TOOL_SURFACE is set
  CAPABILITY_SURFACE        Resource/prompt surface: full|minimal (default full)
  META_PARAM_SCHEMA         Meta-tool input schema: opaque|compact|full (default opaque)
  GITLAB_TIER               Force licensing tier: free|ce|premium|ultimate; omit to detect from license
  GITLAB_READ_ONLY          Expose only read-only tools: true/false (default false)
  GITLAB_SAFE_MODE          Intercept mutating tools and return a preview (default false)
  EMBEDDED_RESOURCES        Embed canonical MCP resource links in get_* results (default true)
  EXCLUDE_TOOLS             Comma-separated tool names to exclude (default empty)
  GITLAB_IGNORE_SCOPES      Skip PAT scope detection: true/false (default false)
  UPLOAD_MAX_FILE_SIZE      Maximum upload/file size for upload tools (default 2GB)
  AUTO_UPDATE               Auto-update mode: true/check/false (default true)
  AUTO_UPDATE_REPO          GitHub repository for update checks (default %s)
  AUTO_UPDATE_INTERVAL      Periodic check interval (default 1h, HTTP mode)
  AUTO_UPDATE_TIMEOUT       Startup/background update timeout (default 60s, range 5s–10m)
  RATE_LIMIT_RPS            Per-server tools/call rate limit (default 0, disabled)
  RATE_LIMIT_BURST          Token-bucket burst size when RATE_LIMIT_RPS > 0 (default 40)
  YOLO_MODE                 Skip destructive action confirmation prompts (default false)
  LOG_LEVEL                 Logging: debug/info/warn/error (default info)

ENVIRONMENT VARIABLES (HTTP mode)
  Every flag above also reads its value from the environment when the flag is
  not passed. Precedence: an explicitly passed flag, then the environment,
  then the built-in default. The variables below have no stdio equivalent:

  AUTH_MODE                 Authentication mode: legacy|oauth (default legacy)
  PUBLIC_URL                Externally reachable https origin; required with AUTH_MODE=oauth
  TRUSTED_ORIGINS           Origins allowed to make cross-origin browser requests
  OAUTH_CACHE_TTL           OAuth token cache TTL (default 15m, min 1m, max 2h)
  OAUTH_CLIENT_UID          Comma-separated GitLab OAuth application uids whose tokens are admitted (default: any)
  MAX_HTTP_CLIENTS          Maximum unique (token, GitLab URL) pool entries; not sessions (default 100)
  SESSION_TIMEOUT           Idle MCP session timeout; --stateless=false only (default 30m)
  POOL_IDLE_TIMEOUT         Reclaim an unused pooled server after this long (default 1h, 0 disables)
  SESSION_REVALIDATE_INTERVAL  Token re-validation interval (default 15m, 0 disables)

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
		  "TOOL_SURFACE": "dynamic"
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

  HTTP mode (no fixed GitLab URL; clients send GITLAB-URL per request):
  gitlab-mcp-server --http --http-addr=:8080
`, version, commit,
		projectAuthor, projectDepartment, projectRepository,
		config.DefaultMaxHTTPClients, config.DefaultSessionTimeout, config.DefaultPoolIdleTimeout,
		config.DefaultAutoUpdateRepo, config.DefaultAutoUpdateInterval,
		config.DefaultAutoUpdateTimeout,
		config.DefaultOAuthCacheTTL, config.MinOAuthCacheTTL, config.MaxOAuthCacheTTL,
		config.DefaultRevalidateInterval,
		config.DefaultRateLimitBurst,
		config.DefaultGitLabURL,
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
// The GitLab URL can be fixed globally via --gitlab-url, or selected per
// request via the GITLAB-URL header when no global URL is configured. At least
// one URL source must be available for each request.
func runHTTP(ctx context.Context, hcfg *httpConfig) error {
	// Environment sits between the explicitly passed flags and the built-in
	// defaults. It runs before URL normalization and surface/tier resolution so
	// those see the final values, and it never touches a flag the operator
	// passed.
	overlay, overlayErr := config.LoadHTTPEnvOverlay()
	if overlayErr != nil {
		return fmt.Errorf("loading environment configuration: %w", overlayErr)
	}
	applyHTTPEnvOverlay(hcfg, overlay)

	if err := normalizeFixedGitLabURL(hcfg); err != nil {
		return err
	}
	if hcfg.httpIdleTimeout < 0 {
		return fmt.Errorf("invalid --http-idle-timeout %s: must be non-negative", hcfg.httpIdleTimeout)
	}
	if err := parseSocketMode(hcfg); err != nil {
		return err
	}

	toolSurface, metaTools, err := config.ParseToolSurface(hcfg.toolSurface, legacyMetaToolsFlagValue(hcfg))
	if err != nil {
		return fmt.Errorf("parse tool surface: %w", err)
	}
	tier, tierExplicit, err := resolveHTTPTier(hcfg)
	if err != nil {
		return err
	}
	cfg := configFromHTTPFlags(hcfg, toolSurface, metaTools, tier, tierExplicit)
	if validationErr := validateHTTPRuntimeConfig(cfg); validationErr != nil {
		return validationErr
	}

	toolutil.SetUploadConfig(cfg.UploadMaxFileSize)
	toolutil.EnableEmbeddedResources(cfg.EmbeddedResources)
	autoupdate.CleanupOldBinary()
	startAutoUpdate(ctx, cfg)

	return serveHTTP(ctx, cfg, hcfg.addr, hcfg.httpIdleTimeout)
}

// normalizeFixedGitLabURL canonicalizes every published instance and elects
// the first as the deployment default.
//
// One instance is the shape this has always had; the list is what lets a
// single oauth deployment serve gitlab.com and a self-managed instance at
// once without reopening the hole a free-form GITLAB-URL header would be —
// see [serverpool.ResolveRequestOptionsFor].
func normalizeFixedGitLabURL(hcfg *httpConfig) error {
	// Both fields are inputs, not just outputs. The flag parser fills the
	// list, but a caller that predates it — a test, anything constructing
	// httpConfig directly — sets the singular field, and reading only the
	// list there silently DISCARDS the URL along with its validation: an
	// unusable --gitlab-url stopped being rejected and the server started
	// anyway, serving nothing anyone configured.
	configured := hcfg.gitlabURLs
	if len(configured) == 0 && strings.TrimSpace(hcfg.gitlabURL) != "" {
		configured = []string{hcfg.gitlabURL}
	}
	// Validated here rather than by NormalizeGitLabURLs alone, whose errors
	// name the GITLAB-URL *header*: the operator is looking at a flag, and
	// being told a header is wrong sends them to the wrong file.
	for _, candidate := range configured {
		if err := validateFixedGitLabURL(candidate); err != nil {
			return err
		}
	}
	normalized, err := serverpool.NormalizeGitLabURLs(configured)
	if err != nil {
		return fmt.Errorf("--gitlab-url is not a valid URL: %w", err)
	}
	hcfg.gitlabURLs = normalized
	if len(normalized) == 0 {
		hcfg.gitlabURL = ""
		return nil
	}
	hcfg.gitlabURL = normalized[0]
	return nil
}

// validateFixedGitLabURL rejects a --gitlab-url value with a message naming
// the flag the operator actually typed.
func validateFixedGitLabURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("--gitlab-url is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("--gitlab-url must use http:// or https:// scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("--gitlab-url must include a host")
	}
	return nil
}

// resolveHTTPTier resolves the --tier flag into a tier and an explicit flag.
// When --tier is unset the tier is detected per pool entry (explicit=false).
func resolveHTTPTier(hcfg *httpConfig) (edition.Tier, bool, error) {
	if !hcfg.tierSet || strings.TrimSpace(hcfg.tier) == "" {
		return edition.Free, false, nil
	}
	tier, explicit, err := config.ParseTierFlag(hcfg.tier)
	if err != nil {
		return edition.Free, false, fmt.Errorf("invalid --tier: %w", err)
	}
	return tier, explicit, nil
}

func configFromHTTPFlags(hcfg *httpConfig, toolSurface string, metaTools bool, tier edition.Tier, tierExplicit bool) *config.Config {
	return &config.Config{
		GitLabURL:             hcfg.gitlabURL,
		GitLabURLs:            hcfg.gitlabURLs,
		SkipTLSVerify:         hcfg.skipTLSVerify,
		MetaTools:             metaTools,
		ToolSurface:           toolSurface,
		CapabilitySurface:     hcfg.capabilitySurface,
		Tier:                  tier,
		TierExplicit:          tierExplicit,
		ReadOnly:              hcfg.readOnly,
		SafeMode:              hcfg.safeMode,
		EmbeddedResources:     hcfg.embeddedResources,
		ExcludeTools:          config.ParseCSV(hcfg.excludeTools),
		IgnoreScopes:          hcfg.ignoreScopes,
		MaxHTTPClients:        hcfg.maxHTTPClients,
		SessionTimeout:        hcfg.sessionTimeout,
		RevalidateInterval:    hcfg.revalidateInterval,
		PoolIdleTimeout:       hcfg.poolIdleTimeout,
		Stateless:             hcfg.stateless,
		JSONResponse:          hcfg.jsonResponse,
		MaxRequestBodyBytes:   hcfg.maxRequestBodyBytes,
		UploadMaxFileSize:     config.DefaultMaxFileSize,
		AutoUpdate:            hcfg.autoUpdate,
		AutoUpdateRepo:        hcfg.autoUpdateRepo,
		AutoUpdateInterval:    hcfg.autoUpdateInterval,
		AutoUpdateTimeout:     hcfg.autoUpdateTimeout,
		AuthMode:              hcfg.authMode,
		PublicURL:             hcfg.publicURL,
		ResourceDocumentation: hcfg.resourceDocumentation,
		OAuthCacheTTL:         hcfg.oauthCacheTTL,
		OAuthClientUIDs:       config.ParseCSV(hcfg.oauthClientUID),
		TrustedProxyHeader:    hcfg.trustedProxyHeader,
		TrustedOrigins:        buildTrustedOrigins(hcfg.trustedOrigins, hcfg.publicURL),
		RateLimitRPS:          hcfg.rateLimitRPS,
		RateLimitBurst:        hcfg.rateLimitBurst,
		MetaParamSchema:       hcfg.metaParamSchema,
		TLSCertFile:           hcfg.tlsCert,
		TLSKeyFile:            hcfg.tlsKey,
		SocketMode:            hcfg.socketModeParsed,
	}
}

func validateHTTPRuntimeConfig(cfg *config.Config) error {
	if err := validateHTTPAuthConfig(cfg); err != nil {
		return err
	}
	if err := validateHTTPSurfaceConfig(cfg); err != nil {
		return err
	}
	if err := validateHTTPDurationConfig(cfg); err != nil {
		return err
	}
	if err := validateTLSFiles(cfg); err != nil {
		return err
	}
	if rateErr := toolutil.ValidateRateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst); rateErr != nil {
		return fmt.Errorf("--rate-limit-rps/--rate-limit-burst: %w", rateErr)
	}
	// A malformed trusted origin must fail startup, not be silently dropped:
	// a deployment believing an origin is trusted when it is not is worse
	// than one that rejects it loudly. AddTrustedOrigin is the same check the
	// middleware applies later, so acceptance here guarantees it there.
	probe := http.NewCrossOriginProtection()
	for _, origin := range cfg.TrustedOrigins {
		if origin == "*" {
			continue // the accept-any wildcard, handled in the middleware
		}
		if err := probe.AddTrustedOrigin(origin); err != nil {
			return fmt.Errorf("--trusted-origins entry %q: %w", origin, err)
		}
	}
	if cfg.MaxRequestBodyBytes < 0 {
		return fmt.Errorf("--max-request-body-bytes must be >= 0, got %d", cfg.MaxRequestBodyBytes)
	}
	return nil
}

func validateHTTPAuthConfig(cfg *config.Config) error {
	if cfg.AuthMode == "" {
		cfg.AuthMode = "legacy"
	}
	if cfg.AuthMode != "legacy" && cfg.AuthMode != "oauth" {
		return fmt.Errorf("--auth-mode must be 'legacy' or 'oauth', got %q", cfg.AuthMode)
	}
	if cfg.AuthMode != "oauth" {
		// In legacy mode --public-url is optional, but when set it now has
		// an effect (its origin seeds the trusted-origins list), so a
		// malformed value must still be rejected rather than silently
		// producing no trusted origin.
		if cfg.PublicURL != "" {
			return config.ValidatePublicURL(cfg.PublicURL)
		}
		return nil
	}
	instances := cfg.InstanceURLs()
	if len(instances) == 0 {
		return errors.New("--auth-mode=oauth requires --gitlab-url")
	}
	// EVERY published instance, not just the first. The bearer token is
	// forwarded to whichever instance the request selected, so checking only
	// cfg.GitLabURL would let `--gitlab-url=https://a --gitlab-url=http://b`
	// start and then put a live credential on the wire in cleartext for every
	// request that names b (CWE-319). One cleartext instance in the list is
	// exactly as bad as a cleartext deployment.
	for _, instance := range instances {
		if err := config.ValidateOAuthGitLabURL(instance); err != nil {
			return err
		}
	}
	if err := config.ValidatePublicURL(cfg.PublicURL); err != nil {
		return err
	}
	return validateOAuthCacheTTL(cfg.OAuthCacheTTL)
}

// validateOAuthCacheTTL bounds an explicitly configured TTL. A non-positive
// value is not rejected here — see [oauthCacheTTL], which normalizes it at
// the point of use so no construction path can produce the broken server a
// zero would otherwise cause.
func validateOAuthCacheTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	if ttl < config.MinOAuthCacheTTL {
		return fmt.Errorf("--oauth-cache-ttl %s is below minimum of %s", ttl, config.MinOAuthCacheTTL)
	}
	if ttl > config.MaxOAuthCacheTTL {
		return fmt.Errorf("--oauth-cache-ttl %s exceeds maximum of %s", ttl, config.MaxOAuthCacheTTL)
	}
	return nil
}

func validateHTTPSurfaceConfig(cfg *config.Config) error {
	switch cfg.MetaParamSchema {
	case "":
		cfg.MetaParamSchema = config.DefaultMetaParamSchema
	case config.MetaParamSchemaOpaque, config.MetaParamSchemaCompact, config.MetaParamSchemaFull:
	default:
		return fmt.Errorf("--meta-param-schema must be one of %q, %q, %q, got %q",
			config.MetaParamSchemaOpaque, config.MetaParamSchemaCompact, config.MetaParamSchemaFull, cfg.MetaParamSchema)
	}
	switch cfg.CapabilitySurface {
	case "":
		cfg.CapabilitySurface = config.DefaultCapabilitySurface
	case config.CapabilitySurfaceFull, config.CapabilitySurfaceMinimal:
	default:
		return fmt.Errorf("--capability-surface must be %q or %q, got %q",
			config.CapabilitySurfaceFull, config.CapabilitySurfaceMinimal, cfg.CapabilitySurface)
	}
	return nil
}

func validateHTTPDurationConfig(cfg *config.Config) error {
	if cfg.SessionTimeout > config.MaxSessionTimeout {
		return fmt.Errorf("--session-timeout %s exceeds maximum of %s", cfg.SessionTimeout, config.MaxSessionTimeout)
	}
	if cfg.RevalidateInterval > config.MaxRevalidateInterval {
		return fmt.Errorf("--revalidate-interval %s exceeds maximum of %s", cfg.RevalidateInterval, config.MaxRevalidateInterval)
	}
	if cfg.PoolIdleTimeout > config.MaxPoolIdleTimeout {
		return fmt.Errorf("--pool-idle-timeout %s exceeds maximum of %s", cfg.PoolIdleTimeout, config.MaxPoolIdleTimeout)
	}
	if cfg.AutoUpdateTimeout < config.MinAutoUpdateTimeout {
		return fmt.Errorf("--auto-update-timeout %s is below minimum of %s", cfg.AutoUpdateTimeout, config.MinAutoUpdateTimeout)
	}
	if cfg.AutoUpdateTimeout > config.MaxAutoUpdateTimeout {
		return fmt.Errorf("--auto-update-timeout %s exceeds maximum of %s", cfg.AutoUpdateTimeout, config.MaxAutoUpdateTimeout)
	}
	return nil
}

func legacyMetaToolsFlagValue(hcfg *httpConfig) string {
	if hcfg == nil || !hcfg.metaToolsSet {
		return ""
	}
	return strconv.FormatBool(hcfg.metaTools)
}

type serverSurfaceRegistration struct {
	metaSchemaRoutes map[string]toolutil.ActionMap
	surfaceCatalog   *actioncatalog.Catalog
}

// runStdio loads configuration from environment variables (GITLAB_TOKEN
// required), validates GitLab connectivity, and starts the stdio server.
func runStdio(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logLegacyMetaToolsDeprecation(os.Getenv("TOOL_SURFACE"), os.Getenv("META_TOOLS"))
	logLegacyEnterpriseEnvDeprecation(os.Getenv("GITLAB_TIER"), os.Getenv("GITLAB_ENTERPRISE"))

	toolutil.SetUploadConfig(cfg.UploadMaxFileSize)
	toolutil.EnableEmbeddedResources(cfg.EmbeddedResources)

	// Clean up leftover .old binary from previous updates.
	autoupdate.CleanupOldBinary()

	startStdioAutoUpdate(ctx, cfg)

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
			slog.Info(
				"gitlab connection verified",
				"url", cfg.GitLabURL,
				"user", userInfo.Username,
				"user_id", userInfo.UserID,
				"version", gitlabVersion,
			)
		}
	}

	// Resolve the licensing tier. When the operator pinned it explicitly via
	// GITLAB_TIER, use it verbatim (no license check). Otherwise detect it from
	// the instance license, falling back to Free.
	serverCfg := cfg.ServerConfig()
	if cfg.TierExplicit || !client.IsInitialized() {
		client.SetTier(cfg.Tier)
		serverCfg.Tier = cfg.Tier
	} else {
		serverCfg.Tier = client.DetectTier(ctx)
	}

	// Detect PAT scopes for scope-based tool filtering.
	if !cfg.IgnoreScopes {
		serverCfg.TokenScopes = gitlabclient.DetectScopes(ctx, client.GL())
		if serverCfg.TokenScopes == nil {
			slog.Debug("PAT scope detection unavailable — all tools will be registered")
		}
	}

	updater := newUpdaterForTools(cfg)
	server, err := createServer(client, serverCfg, updater) //nolint:contextcheck // startup: removeExcludedTools uses ephemeral in-memory MCP transport isolated from request ctx
	if err != nil {
		return fmt.Errorf("creating MCP server: %w", err)
	}
	return serveStdio(ctx, server)
}

// sharedSchemaCache caches resolved tool schemas across every MCP server
// created by this process (stdio startup and the per-token-and-URL servers of the
// HTTP pool). See mcp.ServerOptions.SchemaCache.
var sharedSchemaCache = mcp.NewSchemaCache()

// createServer builds a fully configured [*mcp.Server] with all tools,
// resources, and prompts registered for the given GitLab client.
// Used both by stdio mode (single call) and by the HTTP server pool factory.
// If updater is non-nil, server update MCP tools are registered.
// keepAliveFor returns the server keepalive interval for the configured
// transport: zero (disabled) on stateless HTTP, where a server-initiated ping
// is forbidden and the SDK closes the session when the ping cannot be written;
// 30s everywhere else.
//
// HTTP mode overrides this to zero for every pool entry, stateful included —
// see [withKeepAlive] at the pool factory — so in practice only stdio keeps the
// ping. The condition stays as it is because it is the transport-level truth,
// and because a caller building a server without the pool still gets the
// stateless rule right.
func keepAliveFor(cfg *config.ServerConfig) time.Duration {
	if cfg.Stateless {
		return 0
	}
	return 30 * time.Second
}

// keepAliveInterval resolves the keepalive an individual server runs with: the
// explicit override when one was given, otherwise the transport default.
func keepAliveInterval(settings serverSettings, cfg *config.ServerConfig) time.Duration {
	if settings.keepAlive != nil {
		return *settings.keepAlive
	}
	return keepAliveFor(cfg)
}

// sessionIDMinter returns the function the SDK calls to mint a session ID.
// With no tag — stdio, where sessions cannot be presented by another caller —
// it returns nil so the SDK keeps its own default.
func sessionIDMinter(tag string) func() string {
	if tag == "" {
		return nil
	}
	return func() string { return tag + sessionTagSeparator + rand.Text() }
}

// sessionTagOf returns the pool tag a session ID was minted under, and whether
// the ID carries one at all.
func sessionTagOf(sessionID string) (string, bool) {
	tag, _, found := strings.Cut(sessionID, sessionTagSeparator)
	if !found || tag == "" {
		return "", false
	}
	return tag, true
}

func createServer(
	client *gitlabclient.Client,
	cfg *config.ServerConfig,
	updater *autoupdate.Updater,
	opts ...serverOption,
) (*mcp.Server, error) {
	if client == nil {
		return nil, errors.New("createServer: client must not be nil")
	}

	settings := newServerSettings(opts)
	completionHandler := completions.NewHandler(client)
	capabilitySurface := config.EffectiveCapabilitySurface(cfg.CapabilitySurface)
	toolSurface := config.EffectiveToolSurface(cfg.MetaTools, cfg.ToolSurface)

	// Resource subscriptions. The manager has to exist before the server,
	// because its handlers travel in ServerOptions; the notifier is
	// attached to the server once there is one. The SDK turns the
	// resources.subscribe capability on by itself when a SubscribeHandler
	// is set, so leaving these nil is what withholds the capability on a
	// surface that registers no subscribable resources.
	subs := newSubscriptionRuntime(client, cfg, settings.subscriptions)
	subscribeHandler, unsubscribeHandler := subs.handlers()
	serverCapabilities := &mcp.ServerCapabilities{
		Tools:     &mcp.ToolCapabilities{ListChanged: true},
		Resources: &mcp.ResourceCapabilities{ListChanged: true},
	}
	if capabilitySurface == config.CapabilitySurfaceFull {
		serverCapabilities.Prompts = &mcp.PromptCapabilities{ListChanged: true}
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:        "gitlab-mcp-server",
		Title:       "GitLab MCP Server",
		Description: projectDescription,
		Version:     version,
		WebsiteURL:  projectWebsite,
		// The brand mark, not a domain glyph: this identifies the server
		// itself, and IconServer is already the icon of gitlab_execute_action.
		Icons: toolutil.IconBrand,
	}, &mcp.ServerOptions{
		// Named tools differ per surface, so the guidance is built for the
		// surface this server actually registers: a dynamic-mode model can
		// only see gitlab_find_action and gitlab_execute_action.
		Instructions: buildInstructions(toolSurface, capabilitySurface, cfg.Stateless),
		Logger:       slog.Default(),
		Capabilities: serverCapabilities,
		// The SDK asks the RESOLVED server for the session ID, so a tag minted
		// here is a trustworthy statement about which pooled entry owns the
		// session. cmd/server/authgate.go checks it on every later request:
		// without that, the SDK serves any request carrying a known session ID
		// from the session's own server, discarding the per-credential
		// resolution entirely (go-sdk streamable.go serveStatefulPOST returns
		// before getServer is ever called).
		GetSessionID: sessionIDMinter(settings.sessionTag),
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return completionHandler.Complete(ctx, req)
		},
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationServerRequest) {
			slog.Debug(
				"received progress notification from client",
				"token", req.Params.ProgressToken,
				"progress", req.Params.Progress,
			)
		},
		// Resource subscriptions. Setting these is what makes the SDK
		// advertise resources.subscribe; both are nil together on a
		// surface that registers no subscribable resources.
		SubscribeHandler:   subscribeHandler,
		UnsubscribeHandler: unsubscribeHandler,
		// Keepalive is a server-initiated ping request — a session-era
		// mechanism the 2026-07-28 streamable-HTTP transport forbids on a
		// stateless stream ("the server MUST NOT send independent JSON-RPC
		// requests on this stream"). The SDK enforces that by failing the
		// write and closing the session on the first failed ping, so a 30s
		// keepalive on a stateless server kills every long-lived stream —
		// including subscriptions/listen — at the 30-second mark. Stateless
		// servers therefore run with keepalive off; stdio and legacy
		// stateful HTTP keep the ping, where it is protocol-legal.
		KeepAlive: keepAliveInterval(settings, cfg),
		// One page must fit the whole catalog: OpenAI Codex ignores
		// tools/list nextCursor in its default protocol mode, so any second
		// page would be silently lost. The largest surface (individual mode,
		// Ultimate tier) registers ~1071 tools — above the SDK default of
		// 1000.
		PageSize: 2000,
		// Shared across every server this process creates: in HTTP mode the
		// pool builds one MCP server per token+URL, and with the compiled
		// schema pointers from toolutil.CompileToolSchemas this cache skips
		// schema resolution on every registration after the first.
		SchemaCache: sharedSchemaCache,
	})

	subs.attach(server)

	// SEP-2549 cache hints: catalogs and resource reads are token- and
	// tier-dependent, so every cacheable result is stamped "private". A
	// configured tier cannot change while the server runs, which earns the
	// tool catalog the same freshness window as the compiled-in catalogs.
	server.AddReceivingMiddleware(cachehints.Middleware(cachehints.Options{
		TierPinned: cfg.TierExplicit,
	}))

	// Per-client response compatibility (Codex float-priority workaround and
	// structuredContent shadowing); CLIENT_COMPAT=off disables it.
	if clientcompat.Enabled() {
		server.AddReceivingMiddleware(clientcompat.Middleware())
	}

	// Capability/method consistency: the SDK dispatches logging/setLevel
	// unconditionally although this server never declares the logging
	// capability, and on the minimal capability surface prompts/list would
	// answer a successful empty page while the handshake declares no
	// prompts capability. Refusing with -32601 keeps the wire surface in
	// step with the handshake (the same line the sibling libgen-mcp's
	// capguard draws for its resource methods).
	gatedMethods := []string{"logging/setLevel"}
	if capabilitySurface != config.CapabilitySurfaceFull {
		gatedMethods = append(gatedMethods, "prompts/list", "prompts/get")
	}
	server.AddReceivingMiddleware(capguard.Undeclared(gatedMethods...))

	var metaSchemaRoutes map[string]toolutil.ActionMap
	var surfaceCatalog *actioncatalog.Catalog
	if toolSurface != config.ToolSurfaceIndividual {
		gitlabtools.SetMetaParamSchema(cfg.MetaParamSchema)
	}
	surfaceRegistration, err := registerConfiguredToolSurface(server, client, cfg, updater, toolSurface)
	if err != nil {
		return nil, err
	}
	metaSchemaRoutes = surfaceRegistration.metaSchemaRoutes
	surfaceCatalog = surfaceRegistration.surfaceCatalog

	applyToolVisibilityConfig(server, cfg, toolSurface, surfaceCatalog)

	toolCount, err := countRegisteredTools(server)
	if err != nil {
		slog.Warn("failed to count registered tools", "error", err)
	}
	logRegisteredToolSurface(toolSurface, toolCount, metaSchemaRoutes)

	if toolSurface == config.ToolSurfaceMeta {
		var routesErr error
		metaSchemaRoutes, routesErr = visibleMetaSchemaRoutes(server, metaSchemaRoutes)
		if routesErr != nil {
			slog.Warn("failed to filter meta-schema routes to visible tools", "error", routesErr)
		}
	}

	registerConfiguredCapabilities(server, client, capabilitySurface)

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

	if manifestTools, listErr := listRegisteredToolsForInspection(server, "tool-manifest"); listErr != nil {
		slog.Warn("failed to build tool manifest resource", "error", listErr)
	} else {
		manifestOpts := resources.ToolSurfaceResourceOptions{
			Surface:    toolSurface,
			Tools:      manifestTools,
			Catalog:    surfaceCatalog,
			MetaRoutes: metaSchemaRoutes,
		}
		if subs != nil {
			manifestOpts.SubscribableURITemplates = subscriptions.Templates()
		}
		resources.RegisterToolSurfaceResources(server, manifestOpts)
	}

	// Optional per-server tools/call rate limit. In HTTP mode each pooled
	// per-token-and-URL server entry gets its own bucket. In
	// stdio mode the bucket is global to the process. Disabled when
	// RateLimitRPS is 0 (default).
	if limiter := toolutil.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst); limiter != nil {
		toolutil.AttachRateLimit(server, limiter)
		slog.Info(
			"tools/call rate limit enabled",
			"rps", cfg.RateLimitRPS,
			"burst", cfg.RateLimitBurst,
		)
	}

	return server, nil
}

func applyToolVisibilityConfig(server *mcp.Server, cfg *config.ServerConfig, toolSurface string, surfaceCatalog *actioncatalog.Catalog) {
	if len(cfg.ExcludeTools) > 0 {
		removed := removeExcludedTools(server, cfg.ExcludeTools)
		slog.Info("excluded tools by configuration", "excluded", removed, "patterns", cfg.ExcludeTools)
	}
	if cfg.TokenScopes != nil {
		removed := gitlabtools.RemoveScopeFilteredTools(server, cfg.TokenScopes)
		if removed > 0 {
			slog.Info("scope-filtered tools", "removed", removed)
		}
	}
	if cfg.ReadOnly {
		removed := gitlabtools.RemoveNonReadOnlyTools(server)
		slog.Info("read-only mode: removed write tools", "removed", removed)
		return
	}
	if cfg.SafeMode {
		// Catalog-backed dispatcher tools already carry per-action preview
		// handlers from filterActionCatalog, and wrapping them here would block
		// the reads they also serve. Everything else still needs wrapping,
		// including tools registered outside the catalog such as the
		// gitlab_interactive_* utilities.
		exempt := catalogBackedToolNames(surfaceCatalog, toolSurface)
		wrapped := gitlabtools.WrapMutatingToolsForSafeModeExcept(server, exempt)
		slog.Info("safe mode: intercepted mutating operations",
			"surface", toolSurface, "wrapped_tools", wrapped, "catalog_backed_tools", len(exempt))
	}
}

// catalogBackedToolNames returns the tools whose handlers come from the action
// catalog and therefore already enforce safe mode per action.
func catalogBackedToolNames(surfaceCatalog *actioncatalog.Catalog, toolSurface string) map[string]struct{} {
	if toolSurface == config.ToolSurfaceIndividual {
		// One tool is one action here, so tool-level wrapping is already
		// action-granular and nothing is exempt.
		return nil
	}
	exempt := map[string]struct{}{}
	if toolSurface == config.ToolSurfaceDynamic {
		exempt[dynamictools.FindActionToolName] = struct{}{}
		exempt[dynamictools.ExecuteActionToolName] = struct{}{}
		return exempt
	}
	if surfaceCatalog == nil {
		return exempt
	}
	for _, group := range surfaceCatalog.Groups() {
		exempt[group.ToolName] = struct{}{}
	}
	return exempt
}

func logRegisteredToolSurface(toolSurface string, toolCount int, metaSchemaRoutes map[string]toolutil.ActionMap) {
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		slog.Info("registered dynamic toolset", "tools", toolCount, "catalog_groups", len(metaSchemaRoutes), "catalog_actions", countCatalogActions(metaSchemaRoutes))
	case config.ToolSurfaceMeta:
		slog.Info("registered meta-tools", "tools", toolCount)
	default:
		slog.Info("registered individual tools", "tools", toolCount)
	}
}

func registerConfiguredCapabilities(server *mcp.Server, client *gitlabclient.Client, capabilitySurface string) {
	if capabilitySurface == config.CapabilitySurfaceFull {
		resources.Register(server, client)
		resources.RegisterWorkflowGuides(server)
		prompts.Register(server, client)
	}
}

func registerConfiguredToolSurface(server *mcp.Server, client *gitlabclient.Client, cfg *config.ServerConfig, updater *autoupdate.Updater, toolSurface string) (serverSurfaceRegistration, error) {
	return registerConfiguredToolSurfaceWithCatalog(server, client, cfg, updater, toolSurface, nil)
}

func registerConfiguredToolSurfaceWithCatalog(server *mcp.Server, client *gitlabclient.Client, cfg *config.ServerConfig, updater *autoupdate.Updater, toolSurface string, prebuiltCatalog *actioncatalog.Catalog) (serverSurfaceRegistration, error) {
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		actionCatalog := prebuiltCatalog
		var withheld withheldActions
		if actionCatalog == nil {
			var catalogErr error
			actionCatalog, withheld, catalogErr = buildDynamicActionCatalog(client, cfg, updater)
			if catalogErr != nil {
				return serverSurfaceRegistration{}, fmt.Errorf("build dynamic action catalog: %w", catalogErr)
			}
		}
		dynamictools.RegisterCatalogFindExecuteTools(server, actionCatalog,
			dynamictools.WithWithheldActions(withheld.byTokenScope, withheld.byOperator))
		return serverSurfaceRegistration{metaSchemaRoutes: actionCatalog.ActionMaps(), surfaceCatalog: actionCatalog}, nil
	case config.ToolSurfaceMeta:
		filteredCatalog := prebuiltCatalog
		if filteredCatalog == nil {
			actionCatalog, catalogErr := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{Tier: cfg.Tier, IncludeMCP: true, Updater: updater})
			if catalogErr != nil {
				slog.Warn("failed to build meta action catalog", "error", catalogErr)
				actionCatalog = actioncatalog.NewCatalog()
			}
			var filterErr error
			filteredCatalog, _, filterErr = filterActionCatalog(actionCatalog, cfg)
			if filterErr != nil {
				return serverSurfaceRegistration{}, fmt.Errorf("filter meta action catalog: %w", filterErr)
			}
		}
		gitlabtools.RegisterMetaCatalog(server, filteredCatalog)
		gitlabtools.RegisterMetaStandaloneTools(server, client)
		return serverSurfaceRegistration{metaSchemaRoutes: filteredCatalog.ActionMaps(), surfaceCatalog: filteredCatalog}, nil
	default:
		gitlabtools.RegisterAll(server, client, cfg.Tier)
		gitlabtools.RegisterServerMaintenanceSurfaceTools(server, updater)
		return serverSurfaceRegistration{}, nil
	}
}

// httpShutdownTimeout bounds graceful HTTP shutdown after the process context
// is cancelled.
// 15 seconds rather than 5: under CI load or an instrumented build, five
// seconds of drain was routinely not enough and shutdown returned a
// context-deadline error for perfectly healthy in-flight requests. A
// larger budget never slows a clean shutdown — Shutdown returns as soon as
// the connections drain — it only stops a loaded one from being reported
// as a failure.
const httpShutdownTimeout = 15 * time.Second

// effectiveIdleTimeout maps a user-supplied value to the duration passed to
// [http.Server.IdleTimeout]. When the caller passes 0 (meaning "disable idle
// closure"), Go would normally fall back to ReadTimeout (30s) instead of
// disabling idle connection closure. We return idleTimeoutDisabled (a ~10-year
// sentinel) to prevent that fallback. The write deadline for long-lived SSE
// streams is handled separately (see sseWriteDeadlineMiddleware) so the global
// WriteTimeout can stay at a safe value.
func effectiveIdleTimeout(d time.Duration) time.Duration {
	if d == 0 {
		return idleTimeoutDisabled
	}
	return d
}

// sseWriteDeadlineMiddleware disables the per-connection write deadline for any
// response the server actually answers as Server-Sent Events. In Streamable HTTP
// these are long-lived: the standalone GET stream that carries server-initiated
// notifications and keep-alive pings, and the streamed POST responses to client
// requests (which can stay open for the duration of a tool call, e.g. while
// awaiting an elicitation/sampling round-trip). The MCP go-sdk SSE writer never
// resets the write deadline, so without this the server's WriteTimeout would
// sever those streams. Responses that are not SSE (e.g. /health, the
// unauthenticated server-card endpoint) keep WriteTimeout as a slow-write
// (Slowloris) guard, so disabling it globally is unnecessary and unsafe.
//
// The decision is taken from the response, not from the request's Accept header.
// The SDK treats `*/*` and `text/*` as accepting a stream and then answers
// text/event-stream, so matching the literal "text/event-stream" in Accept
// missed exactly the clients that send curl's and several HTTP libraries'
// default: they received a real SSE stream with no anti-buffering header and
// with the 60-second write deadline still armed, which severed any stream that
// outlived it. Widening the Accept test instead would strip the deadline from
// every ordinary route for those same clients, which is the guard this
// middleware exists to keep.
func sseWriteDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &sseAwareWriter{ResponseWriter: w, ctx: r.Context()}
		defer writer.stopKeepAlive()
		next.ServeHTTP(writer, r)
	})
}

// sseKeepAliveInterval is how long an SSE stream may stay silent before the
// server emits a comment frame.
//
// Clearing the write deadline keeps this end of the connection from timing out;
// it does nothing about the hops in between. nginx closes an idle upstream
// response at proxy_read_timeout, 60 seconds by default, and load balancers and
// mobile carrier NATs are less generous still. A standalone GET stream is silent
// by design until something happens, and a streamed POST is silent for as long
// as GitLab takes, so the streams this transport depends on are exactly the ones
// an idle timer collects. 25 seconds leaves room for two frames inside the
// tightest of those windows.
//
// A var, not a const, only so tests can shorten it; nothing at runtime writes it.
var sseKeepAliveInterval = 25 * time.Second

// sseKeepAliveFrame is a comment: a line beginning with ':' carries no field, so
// a conforming SSE reader — the MCP go-sdk client's included — discards it
// without producing an event. It exists to put bytes on the wire.
var sseKeepAliveFrame = []byte(": keep-alive\n\n")

// sseAwareWriter clears the write deadline, sets X-Accel-Buffering, and starts a
// keep-alive the moment a response commits to text/event-stream.
type sseAwareWriter struct {
	http.ResponseWriter
	ctx         context.Context
	wroteHeader bool

	// mu serializes the handler's writes with the keep-alive goroutine's. An
	// http.ResponseWriter is not safe for concurrent use, and the SDK emits one
	// event per Write, so holding this around each write is what keeps a
	// keep-alive from landing inside an event.
	mu        sync.Mutex
	lastWrite time.Time
	stopped   bool
	stop      chan struct{}
	done      chan struct{}
}

// Unwrap exposes the underlying writer so [http.NewResponseController] — which
// the SDK uses to set deadlines — reaches the real connection rather than
// stopping at this wrapper.
//
// Flush deliberately does not unwrap: this type implements it, so the
// controller finds it here and the flush takes the same lock the writes do.
func (w *sseAwareWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush pushes buffered bytes under the write lock.
func (w *sseAwareWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// WriteHeader applies the SSE treatment once, when the status is committed.
func (w *sseAwareWriter) WriteHeader(code int) {
	streaming := false
	if !w.wroteHeader {
		w.wroteHeader = true
		if isEventStream(w.Header().Get("Content-Type")) {
			streaming = true
			// Zero time clears the deadline. Best-effort: ignore on transports
			// that do not support it (e.g. HTTP/2 manages deadlines itself).
			_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Time{})
			// The transport spec says SSE responses SHOULD carry
			// X-Accel-Buffering: no, or nginx-class proxies may buffer events
			// instead of streaming them.
			w.Header().Set("X-Accel-Buffering", "no")
		}
	}
	w.ResponseWriter.WriteHeader(code)
	if streaming {
		w.startKeepAlive()
	}
}

// Write covers the streamed POST response, which never calls WriteHeader
// explicitly — so this is the only hook that fires for a long tool call.
func (w *sseAwareWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastWrite = time.Now()
	return w.ResponseWriter.Write(b)
}

// startKeepAlive runs the idle-stream heartbeat until the handler returns or
// the client goes away. It is called from the handler goroutine, once, after
// the header is on the wire.
func (w *sseAwareWriter) startKeepAlive() {
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	stop, done := w.stop, w.done
	go func() {
		defer close(done)
		ticker := time.NewTicker(sseKeepAliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				if !w.writeKeepAlive() {
					return
				}
			}
		}
	}()
}

// writeKeepAlive emits one comment frame, and reports whether the heartbeat
// should continue. A stream that has written recently is not idle, so it is
// skipped rather than padded.
func (w *sseAwareWriter) writeKeepAlive() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return false
	}
	if !w.lastWrite.IsZero() && time.Since(w.lastWrite) < sseKeepAliveInterval {
		return true
	}
	if _, err := w.ResponseWriter.Write(sseKeepAliveFrame); err != nil {
		return false
	}
	w.lastWrite = time.Now()
	_ = http.NewResponseController(w.ResponseWriter).Flush()
	return true
}

// stopKeepAlive ends the heartbeat and waits for it, so no write reaches a
// ResponseWriter the handler has already finished with.
func (w *sseAwareWriter) stopKeepAlive() {
	w.mu.Lock()
	if w.stopped || w.stop == nil {
		w.stopped = true
		w.mu.Unlock()
		return
	}
	w.stopped = true
	close(w.stop)
	done := w.done
	w.mu.Unlock()
	<-done
}

// isEventStream reports whether a Content-Type names the SSE media type,
// ignoring any parameters and case.
func isEventStream(contentType string) bool {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(base), "text/event-stream")
}

// newHTTPServer builds the HTTP-mode [http.Server] with the timeout policy.
// httpIdleTimeout is the raw --http-idle-timeout value (0 disables idle closure).
// WriteTimeout is a fixed slow-write guard for standard endpoints; the long-lived
// SSE stream disables its own write deadline via sseWriteDeadlineMiddleware.
// ReadHeaderTimeout/ReadTimeout are fixed request-read guards.
func newHTTPServer(addr string, handler http.Handler, httpIdleTimeout time.Duration) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           sseWriteDeadlineMiddleware(handler),
		ReadHeaderTimeout: baseHTTPReadHeaderTimeout,
		ReadTimeout:       baseHTTPReadTimeout,
		WriteTimeout:      baseHTTPWriteTimeout,
		IdleTimeout:       effectiveIdleTimeout(httpIdleTimeout),
	}
}

// serveHTTP starts the MCP server in HTTP mode using a [serverpool.ServerPool].
// Each unique token in incoming requests gets its own [*mcp.Server] instance
// backed by a dedicated GitLab client. Requests without a valid authentication
// token are rejected. Sessions expire after cfg.SessionTimeout of inactivity.
// The pool is bounded by cfg.MaxHTTPClients entries with LRU eviction.
func serveHTTP(ctx context.Context, cfg *config.Config, httpAddr string, httpIdleTimeout time.Duration) error {
	return serveHTTPOn(ctx, cfg, httpAddr, nil, httpIdleTimeout)
}

// serveHTTPOn is serveHTTP with an optional pre-bound listener. When
// listener is non-nil it is served directly and httpAddr is used only for
// logging and host validation; when nil, the server binds httpAddr itself.
func serveHTTPOn(ctx context.Context, cfg *config.Config, httpAddr string, listener net.Listener, httpIdleTimeout time.Duration) error {
	slog.Info(
		"starting MCP server in HTTP mode",
		"addr", httpAddr,
		"auth_mode", cfg.AuthMode,
		"max_clients", cfg.MaxHTTPClients,
		"session_timeout", cfg.SessionTimeout,
		"stateless", cfg.Stateless,
		"json_response", cfg.JSONResponse,
		"trusted_proxy_header", cfg.TrustedProxyHeader,
		"version", version,
		"commit", commit,
	)

	if !cfg.Stateless {
		slog.Warn("stateful HTTP sessions are a legacy compatibility mode; protocol 2026-07-28 requires stateless (clients will negotiate 2025-11-25)")
	}

	// Each pooled entry mints session IDs under its own tag, and sessionTags
	// records which tag belongs to which server so the gate can refuse a
	// session presented with a different credential.
	var sessionTags sync.Map
	//nolint:contextcheck // the pool bounds entry construction with its own lifetime context via WithBaseContext below
	pool := serverpool.New(cfg, func(client *gitlabclient.Client, serverCfg *config.ServerConfig) (*mcp.Server, error) {
		tag := rand.Text()
		// No server-initiated keepalive on any HTTP entry, stateful included.
		// The SDK's keepalive is a JSON-RPC ping request, and it closes the
		// session the first time one goes unanswered — so a client that is
		// simply between requests, or one whose transport does not carry
		// server-initiated messages to it, loses its session at the 30-second
		// mark for being idle. Liveness on this transport is the SSE
		// keep-alive comment (see sseAwareWriter), which puts bytes on the
		// wire without asking the client for anything.
		srv, err := createServer(client, serverCfg, nil, withSessionTag(tag), withKeepAlive(0))
		if err != nil {
			return nil, err
		}
		sessionTags.Store(srv, tag)
		return srv, nil
	}, serverpool.WithMaxSize(cfg.MaxHTTPClients),
		serverpool.WithRevalidateInterval(cfg.RevalidateInterval),
		serverpool.WithIdleTimeout(cfg.PoolIdleTimeout),
		// Entry construction is bounded by the server's lifetime rather than
		// by the request that triggered it: an entry is shared, so one client
		// disconnecting must not abort a build others are waiting on — but
		// shutdown must stop it.
		serverpool.WithBaseContext(func() context.Context { return ctx }))
	defer pool.Close()

	pool.StartRevalidation(ctx)
	pool.StartIdleEviction(ctx)

	// The server card is built on first request, not at startup. Building
	// it means standing up an ephemeral MCP server with the full catalog —
	// tens of seconds under instrumented builds — and doing that inline
	// held /health hostage: a deployment's readiness probe (and every HTTP
	// test) was gated on an endpoint almost nobody fetches. sync.OnceValues
	// keeps the once-per-process semantics; only the first card request
	// pays.
	// The card build is bounded by the server's lifecycle context, not by
	// any single request's: a canceled first request must not poison the
	// once-cached card for everyone after it, while a server shutdown
	// should cancel the build rather than wait behind it. One background
	// goroutine does the building, started by the first request; handlers
	// only select on its completion channel. Per-request waiter goroutines
	// would let anyone hammering this public route accumulate goroutines
	// for the whole duration of the first build.
	var (
		serverCardOnce sync.Once
		serverCardDone = make(chan struct{})
		serverCardJSON []byte
	)
	startServerCardBuild := func() {
		serverCardOnce.Do(func() {
			go func() {
				defer close(serverCardDone)
				cardJSON, err := buildServerCardFn(ctx, cfg)
				if err != nil {
					slog.Warn("failed to build server-card.json, endpoint returns 503", "error", err)
					return
				}
				serverCardJSON = cardJSON
			}()
		})
	}

	mux := http.NewServeMux()
	// Every public route is mounted under the --public-url path prefix too,
	// for the reverse proxy that forwards its prefix instead of stripping it.
	// Mounting only the MCP endpoint there left such a deployment answering
	// 404 for its own health check, server card and RFC 9728 document — the
	// three things an operator and a scanner reach for first.
	for _, path := range publicPaths(cfg, "/health") {
		mux.HandleFunc("GET "+path, healthHandler)
	}
	cardPreflight := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerAllowOrigin, "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		// A plain fetch of the card is a simple request and never
		// preflights; this branch exists for the caller that adds a header
		// of its own (a scanner stamping a request id, say), whose request
		// the browser refuses unless the preflight names that header back.
		// Echoing allows exactly what was asked for and nothing else.
		if want := r.Header.Get(headerRequestHeaders); want != "" {
			w.Header().Set("Access-Control-Allow-Headers", want)
			w.Header().Set(headerVary, headerRequestHeaders)
		}
		// Without a lifetime the browser preflights again on every fetch,
		// which for a document that only changes with a release is two
		// round-trips where one would do.
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.WriteHeader(http.StatusNoContent)
	}
	cardHandler := func(w http.ResponseWriter, r *http.Request) {
		// The card's audience is browser-based registry scanners; without
		// CORS they cannot fetch it cross-origin (the SDK's OAuth metadata
		// handler on this same server already sends the header).
		w.Header().Set(headerAllowOrigin, "*")
		// The build runs detached and the handler only waits on it: the
		// catalog registration inside is CPU work no context can
		// interrupt, so a handler that called the builder inline would
		// hold graceful shutdown hostage for however long the build took.
		// This way a request caught by shutdown returns 503 immediately,
		// Shutdown drains, and a build that does finish stays cached for
		// the next request. serverCardJSON is safe to read after the
		// channel closes: the write happens before close(serverCardDone).
		startServerCardBuild()
		select {
		case <-serverCardDone:
		case <-r.Context().Done():
			writeCardUnavailable(w)
			return
		case <-ctx.Done():
			writeCardUnavailable(w)
			return
		}
		if serverCardJSON == nil {
			writeCardUnavailable(w)
			return
		}
		w.Header().Set(hdrContentType, mimeJSON)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(serverCardJSON)
	}
	// /server-card is the location the server-card extension recommends: a
	// card is application-level metadata about one server, not the
	// site-wide metadata /.well-known is reserved for, and the extension
	// says so in writing since commit 10e958fa (2026-06-08). The
	// .well-known path the earlier draft recommended stays mounted because
	// scanners written against that draft are already fetching it, and a
	// card is a public document with nothing to gain from breaking them.
	for _, path := range publicPaths(cfg, serverCardPath, serverCardLegacyPath) {
		mux.HandleFunc("OPTIONS "+path, cardPreflight)
		mux.HandleFunc("GET "+path, cardHandler)
	}

	registerHTTPMCPHandlers(ctx, cfg, httpAddr, pool, &sessionTags, mux)

	var rootHandler http.Handler = mux
	rootHandler = crossOriginProtectionMiddleware(cfg.TrustedOrigins, rootHandler)
	rootHandler = corsMiddleware(cfg, rootHandler)
	if hosts := allowedHosts(httpAddr); len(hosts) > 0 {
		rootHandler = hostValidationMiddleware(hosts, rootHandler)
	}
	// Outermost, so that EVERY response carries the security headers —
	// including the ones written by a middleware that answers instead of
	// forwarding. Host validation used to sit outside this and its 403 went
	// out bare: no nosniff, no CSP, no X-Frame-Options, no Referrer-Policy.
	// A header policy with a hole in it is not a policy, and the hole was in
	// a rejection, which is exactly the response an attacker gets to see.
	rootHandler = securityHeadersMiddleware(rootHandler)

	httpServer := newHTTPServer(httpAddr, rootHandler, httpIdleTimeout)
	if cfg.TLSCertFile != "" {
		// Stated rather than inherited: the standard library's default
		// floor has moved before and may move again, and a deployment that
		// turned TLS on to satisfy an auditor should be able to read the
		// floor off this line.
		httpServer.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	serverErr, startErr := startServing(ctx, cfg, httpServer, httpAddr, listener)
	if startErr != nil {
		return startErr
	}

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

// startServing binds the listener if the caller did not supply one and serves
// on it in the background, returning the channel that reports a serve failure.
//
// A pre-bound listener wins over the address. This is what lets a caller — the
// tests, above all — reserve the port and hand over the live socket, instead
// of closing a probe listener and racing everything else on the machine to
// re-bind the same address.
//
// Binding here rather than inside the goroutine means a bind failure (a port
// already in use, a socket path that is not writable) is returned as a startup
// error rather than raced against the shutdown select.
func startServing(
	ctx context.Context,
	cfg *config.Config,
	httpServer *http.Server,
	httpAddr string,
	listener net.Listener,
) (<-chan error, error) {
	if listener == nil {
		bound, err := listenHTTP(ctx, httpAddr, cfg.SocketMode)
		if err != nil {
			return nil, fmt.Errorf("binding %q: %w", httpAddr, err)
		}
		listener = bound
	}

	serverErr := make(chan error, 1)
	go func() {
		// ServeTLS with empty filenames would demand a certificate from
		// TLSConfig, so the plain path stays plain: TLS is only reached when
		// the operator supplied a pair, which validateTLSFiles has already
		// loaded once to prove it parses.
		var err error
		if cfg.TLSCertFile != "" {
			err = httpServer.ServeTLS(listener, cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			err = httpServer.Serve(listener)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()
	return serverErr, nil
}

func registerHTTPMCPHandlers(ctx context.Context, cfg *config.Config, httpAddr string, pool *serverpool.ServerPool, sessionTags *sync.Map, mux *http.ServeMux) {
	if cfg.AuthMode == config.AuthModeOAuth {
		registerOAuthMCPHandlers(ctx, cfg, httpAddr, pool, sessionTags, mux)
		return
	}
	registerLegacyMCPHandlers(ctx, cfg, pool, sessionTags, mux)
}

// mountMCPEndpoint routes the MCP handler to the paths that are the MCP
// endpoint, and answers every other path 404 — unauthenticated.
//
// The endpoint used to be a catch-all: any path, known or not, went through
// the authentication layer, so a probe for a document this server does not
// serve came back 401. That is a lie a scanner believes. Directories that
// sweep /.well-known/oauth-authorization-server and /.well-known/mcp were
// recording those as protected metadata documents when they simply are not
// there, and nothing could tell "exists but needs a token" apart from "does
// not exist". Routing before authenticating is what makes 404 possible.
//
// Which paths count as the endpoint: the root, /mcp, and — when --public-url
// carries a path — that path and its /mcp form. The last two exist for a
// reverse proxy that forwards its prefix instead of stripping it: such a
// deployment already tells the server its public path, so it does not need a
// second flag to say the same thing.
func mountMCPEndpoint(cfg *config.Config, mux *http.ServeMux, handler http.Handler) {
	guarded := mcpOriginMiddleware(cfg, protocolVersionMiddleware(cfg.Stateless, handler))
	for _, pattern := range mcpEndpointPatterns(cfg) {
		mux.Handle(pattern, guarded)
	}
	mux.HandleFunc("/", notFoundHandler)
}

// isCORSPreflightRequest reports whether a request is a browser's preflight,
// which carries no credential and must be answered rather than authenticated or
// refused on its Origin.
func isCORSPreflightRequest(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get(headerRequestMethod) != ""
}

// mcpOriginMiddleware refuses a request to the MCP endpoint whose Origin names a
// host this deployment does not trust, on every method.
//
// The transport specification requires a server to validate Origin on all
// incoming connections and answer 403 when one is present and invalid. The
// process-wide guard in front of every route delegates to the standard library,
// which deliberately exempts GET, HEAD and OPTIONS as safe methods — correct for
// /health and the server card, which are meant to be publicly fetchable, but not
// for the MCP endpoint: with stateful sessions a cross-origin GET opened a real
// SSE stream on someone else's session instead of being refused. Scoping this
// check to the endpoint keeps both intents, rather than trading one for the
// other.
//
// The preflight carve-out is deliberate and matches the CORS middleware in
// front: a browser strips credentials from a preflight, and refusing it here
// would answer the routine permission question with a 403 the client cannot
// interpret.
func mcpOriginMiddleware(cfg *config.Config, next http.Handler) http.Handler {
	trustAll := slices.Contains(cfg.TrustedOrigins, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get(headerOrigin)
		if origin == "" || trustAll || isCORSPreflightRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		// An origin the operator named is allowed even when the browser calls
		// it cross-site — being deliberately cross-origin is the entire point
		// of --trusted-origins, so this is checked before the fetch metadata.
		if slices.Contains(cfg.TrustedOrigins, origin) {
			next.ServeHTTP(w, r)
			return
		}
		// Fetch metadata then outranks the Origin comparison, exactly as the
		// standard library's own protection treats it. A browser that says the
		// request is same-origin, or that there was no initiating site at all,
		// is authoritative; a desktop client sending an app:// Origin with
		// Sec-Fetch-Site: none is the case that matters, since comparing that
		// Origin against the listen host would refuse it forever.
		switch r.Header.Get(headerSecFetchSite) {
		case "same-origin", "none":
			next.ServeHTTP(w, r)
			return
		case "":
			// No fetch metadata: fall through to the Origin comparison, which
			// is the only signal left.
		default:
			// cross-site or cross-origin, stated by the browser itself.
			writeUntrustedOrigin(w, r)
			return
		}
		if sameOriginAsHost(origin, r.Host) {
			next.ServeHTTP(w, r)
			return
		}
		writeUntrustedOrigin(w, r)
	})
}

// writeUntrustedOrigin refuses a cross-origin request to the MCP endpoint.
func writeUntrustedOrigin(w http.ResponseWriter, r *http.Request) {
	slog.Warn("request refused: untrusted Origin on the MCP endpoint", "method", r.Method)
	(&gateFailure{
		status:  http.StatusForbidden,
		code:    errCodeForbidden,
		message: "Cross-origin request refused: the Origin header names an origin this deployment does not trust.",
	}).write(w)
}

// sameOriginAsHost reports whether an Origin names the host the request was sent
// to. It compares the host only, exactly as the standard library's own
// cross-origin protection does — comparing schemes as well would refuse
// same-origin browser traffic to a deployment behind TLS termination.
func sameOriginAsHost(origin, host string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Host == host
}

// supportedProtocolVersions mirrors the SDK's own list, which is unexported.
// TestProtocolVersions_MatchTheSDK drives the SDK with an unknown version and
// compares this against the list it names, so an SDK bump that adds or drops a
// revision fails loudly instead of silently making this stale.
var supportedProtocolVersions = []string{
	"2026-07-28",
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

// protocolVersionMiddleware answers an unsupported MCP-Protocol-Version with the
// error the transport specification requires.
//
// The spec says a server that does not implement the requested version MUST
// respond 400 with an UnsupportedProtocolVersionError listing what it does
// support. The SDK produces that only for versions sorting at or above
// 2026-07-28; anything older gets a plain-text 400. That matters because the
// spec's own backward-compatibility rule tells a client that a 400 whose body is
// not a recognizable JSON-RPC error means it is talking to an initialization-era
// server, so it falls back to the withdrawn HTTP+SSE transport — issuing a GET,
// which a stateless deployment answers 405. The client ends with no transport at
// all instead of the single retry the supported list would have told it to make.
func protocolVersionMiddleware(stateless bool, next http.Handler) http.Handler {
	supported := supportedProtocolVersionsFor(stateless)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := r.Header.Get("MCP-Protocol-Version")
		if requested == "" || slices.Contains(supported, requested) {
			next.ServeHTTP(w, r)
			return
		}
		writeUnsupportedProtocolVersion(w, supported, requested)
	})
}

// supportedProtocolVersionsFor narrows the advertised list to what this
// deployment can actually negotiate.
//
// The SDK's StreamableServerTransport.SupportsProtocolVersion refuses every
// revision at or above 2026-07-28 unless the transport is stateless, because
// SEP-2575 has no session concept to fall back on. A stateful deployment that
// listed 2026-07-28 among its supported versions would be handing a client the
// one answer that cannot work: the error body exists to say what to retry with.
func supportedProtocolVersionsFor(stateless bool) []string {
	if stateless {
		return supportedProtocolVersions
	}
	narrowed := make([]string, 0, len(supportedProtocolVersions))
	for _, version := range supportedProtocolVersions {
		if version < protocolVersionStatelessOnly {
			narrowed = append(narrowed, version)
		}
	}
	return narrowed
}

// protocolVersionStatelessOnly is the first revision the streamable transport
// serves only in stateless mode.
const protocolVersionStatelessOnly = "2026-07-28"

// unsupportedVersionError is the wire shape of the error the transport
// specification names for a protocol version the server does not implement. It
// is a typed struct rather than a map so the JSON encoding cannot fail, which
// is what lets the write below be total.
type unsupportedVersionError struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      *string `json:"id"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Supported []string `json:"supported"`
			Requested string   `json:"requested"`
		} `json:"data"`
	} `json:"error"`
}

// writeUnsupportedProtocolVersion emits the JSON-RPC error body the spec names,
// carrying the versions this server does support so the client can retry once
// with one of them rather than guessing or downgrading its transport.
func writeUnsupportedProtocolVersion(w http.ResponseWriter, supported []string, requested string) {
	var body unsupportedVersionError
	body.JSONRPC = "2.0"
	body.Error.Code = codeUnsupportedProtocolVersion
	body.Error.Message = "unsupported protocol version"
	body.Error.Data.Supported = supported
	body.Error.Data.Requested = requested

	w.Header().Set(hdrContentType, mimeJSON)
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("failed to write unsupported-protocol-version response", "error", err)
	}
}

// mcpEndpointPatterns lists the ServeMux patterns the MCP handler is mounted
// on. "/{$}" is the exact root: the bare "/" pattern would be the catch-all
// this routing exists to remove.
func mcpEndpointPatterns(cfg *config.Config) []string {
	// The trailing-slash forms are mounted explicitly. The catch-all this
	// routing replaced served them, so a client or proxy configured with
	// ".../mcp/" kept working; exact patterns alone would 404 it, which is a
	// regression dressed up as strictness. ServeMux would redirect "/mcp/" to
	// "/mcp" on its own only if "/mcp/" were a registered subtree, and that
	// would swallow every path beneath it — the opposite of what this routing
	// is for.
	patterns := []string{"/{$}", mcpEndpointPath, mcpEndpointPath + "/{$}"}
	prefix := publicURLPath(cfg.PublicURL)
	if prefix == "" {
		return patterns
	}
	return append(patterns,
		prefix,
		prefix+"/{$}",
		prefix+mcpEndpointPath,
		prefix+mcpEndpointPath+"/{$}",
	)
}

// publicPaths returns each given path, plus its form under the --public-url
// path prefix when the deployment publishes one.
//
// A proxy that forwards its prefix rather than stripping it reaches this
// server at /prefix/health, not /health. The prefix is not new configuration:
// --public-url already tells the server the path it is published under.
func publicPaths(cfg *config.Config, paths ...string) []string {
	prefix := publicURLPath(cfg.PublicURL)
	if prefix == "" {
		return paths
	}
	out := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		out = append(out, path, prefix+path)
	}
	return out
}

// publicURLPath extracts the path component of --public-url, normalized to
// have no trailing slash. It returns "" for a path-less origin, whose
// requests already arrive at the root.
func publicURLPath(publicURL string) string {
	if publicURL == "" {
		return ""
	}
	u, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" || path == mcpEndpointPath {
		return ""
	}
	return path
}

// notFoundHandler answers a path this server does not serve, without asking
// who is calling: whether a document exists is not a secret, and pretending
// otherwise is what made the 401 catch-all misleading.
func notFoundHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(hdrContentType, mimeJSON)
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"error":"not found"}` + "\n"))
}

// streamableHTTPOptions builds the StreamableHTTPOptions shared by the legacy
// and OAuth handler paths. In stateless mode the SDK ignores SessionTimeout
// because no session outlives its request.
func streamableHTTPOptions(cfg *config.Config) *mcp.StreamableHTTPOptions {
	return &mcp.StreamableHTTPOptions{
		SessionTimeout:      cfg.SessionTimeout,
		Stateless:           cfg.Stateless,
		JSONResponse:        cfg.JSONResponse,
		MaxRequestBodyBytes: cfg.MaxRequestBodyBytes,
		// Always propagate client aborts into handler contexts so in-flight
		// GitLab API calls are cancelled when the POST is abandoned. The SDK
		// applies this to new-protocol (2026-07-28) requests only, so legacy
		// clients are unaffected.
		PropagateRequestCancellation: true,
	}
}

// tokenCacheSweepInterval derives the expired-entry sweep cadence from the
// cache TTL: often enough that a dead entry does not outlive its usefulness by
// much, rarely enough that the sweep's write lock is not a recurring cost on a
// map that is read on every authenticated request.
func tokenCacheSweepInterval(ttl time.Duration) time.Duration {
	return max(ttl/tokenCacheSweepDivisor, tokenCacheSweepMinInterval)
}

func registerOAuthMCPHandlers(ctx context.Context, cfg *config.Config, _ string, pool *serverpool.ServerPool, sessionTags *sync.Map, mux *http.ServeMux) {
	// The protected-resource identifier is the advertised public origin
	// (validated at config load: https, no fragment, no trailing slash) —
	// RFC 9728 §1.2. Deriving it from the bind address produced host-less
	// http:// identifiers, and behind any TLS-terminating proxy the wrong
	// origin. The metadata URL is derived per RFC 9728 §3: the well-known
	// segment is inserted between host and path, so a resource with a path
	// (https://mcp.example.com/gitlab) serves metadata at
	// /.well-known/oauth-protected-resource/gitlab.
	resourceID := cfg.PublicURL
	resourceMetadataURL := oauth.MetadataURLFor(resourceID)
	mcpHandler := mcp.NewStreamableHTTPHandler(serverFromRequestContext, streamableHTTPOptions(cfg))

	// The scope demanded is the least privilege this deployment can work
	// with, not a constant: the verifier reports a token's real scopes, and
	// the SDK requires every listed scope to be present, so a hardcoded
	// "api" would 403 a read_api token on a server that cannot write.
	requiredScope := oauth.RequiredScope(cfg.ReadOnly, cfg.SafeMode)

	// The limiter is shared with the guard in front: both charge failures to
	// the same per-address budget, so a caller cannot get a fresh allowance
	// by failing at a different layer. The gate reaches it only for the pool
	// rejections the guard cannot see.
	authLimiter := serverpool.NewAuthRateLimiter(authFailureLimit, authFailureWindow)
	gate := &mcpServerGate{
		pool:               pool,
		gitlabURLs:         cfg.InstanceURLs(),
		limiter:            authLimiter,
		trustedProxyHeader: cfg.TrustedProxyHeader,
		sessionTags:        sessionTags,
		// The same challenge the guard in front emits, minus its error
		// parameters: a gate rejection is a pool failure, not a verdict on
		// the credential, but a client reaching it must still be told the
		// scope and where the metadata lives.
		challenge:  oauthChallenge(requiredScope, resourceMetadataURL),
		bearerOnly: true,
		oauthMode:  true,
		stateless:  cfg.Stateless,
	}

	tokenCache := oauth.NewTokenCache()
	rejectedTokens := oauth.NewRejectedTokens(rejectedTokenMaxSize, rejectedTokenTTL)
	cacheTTL := oauthCacheTTL(cfg.OAuthCacheTTL)
	// A token that never returns is never read, so lazy eviction never reaches
	// it. Sweeping on a fraction of the TTL keeps the cache bounded by time
	// rather than by how many distinct credentials have ever arrived.
	go tokenCache.RunCleanup(ctx, tokenCacheSweepInterval(cacheTTL))
	// The token is verified against the instance the request selected, not
	// against a single instance fixed at startup: a token is only ever valid
	// for the GitLab that issued it. The resolver is the operator's
	// allow-list, so a caller cannot name an instance of their own and be
	// handed the bearer token — which is what made a free-form GITLAB-URL
	// header unacceptable in oauth mode in the first place.
	resolveInstance := func(r *http.Request) (string, error) {
		if r == nil {
			return cfg.GitLabURL, nil
		}
		options, err := serverpool.ResolveRequestOptionsFor(r, cfg.InstanceURLs())
		if err != nil {
			return "", err
		}
		return options.GitLabURL, nil
	}
	verifier := oauth.NewGitLabVerifierFor(resolveInstance, cfg.SkipTLSVerify, cacheTTL, tokenCache, cfg.OAuthClientUIDs...)
	if len(cfg.OAuthClientUIDs) > 0 {
		slog.Info("admitting only tokens issued to the pinned OAuth applications; personal access tokens are refused",
			"applications", len(cfg.OAuthClientUIDs))
	}
	guard := &bearerGuard{
		verify:             verifier,
		resolveInstance:    resolveInstance,
		rejected:           rejectedTokens,
		limiter:            authLimiter,
		trustedProxyHeader: cfg.TrustedProxyHeader,
		metadataURL:        resourceMetadataURL,
		// The door asks only for what every action needs. Whether a
		// particular action may write is settled per action, against the
		// surface the pool built for this token's real authority.
		minimumScope:    oauth.MinimumScope,
		advertisedScope: requiredScope,
	}
	authMiddleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{ResourceMetadataURL: resourceMetadataURL, Scopes: []string{oauth.MinimumScope}})
	prm := oauth.NewProtectedResourceHandler(resourceID, cfg.InstanceURLs(), oauth.SupportedScopes(cfg.ReadOnly, cfg.SafeMode), cfg.ResourceDocumentation)
	// Both derivations of RFC 9728 §3 resolve: the path-less form and the
	// path-inserted form a client computes from a resource identifier that
	// carries a path. Mounted without a method restriction so the SDK
	// handler answers the CORS preflight itself (OPTIONS → 204 with
	// Access-Control-Allow-Origin: *); a "GET "-restricted pattern would
	// send the preflight to the catch-all gate and 401 it, locking out
	// browser-based clients that fetch PRM cross-origin (claude.ai).
	mux.Handle("/.well-known/oauth-protected-resource", prm)
	mux.Handle("/.well-known/oauth-protected-resource/{rest...}", prm)
	// Bearer only: oauth mode advertises the RFC 6750 scheme, so the legacy
	// PRIVATE-TOKEN header alias is not silently rewritten into it here —
	// what the challenge advertises is exactly what is accepted.
	//
	// The guard wraps the SDK middleware rather than replacing it: only the
	// SDK can publish the token info its streamable handler reads back, so
	// it stays, and its verification is a cache hit on what the guard has
	// already resolved.
	mountMCPEndpoint(cfg, mux, guard.middleware(authMiddleware(gate.middleware(mcpHandler))))
	startPeriodicCleanup(ctx, tokenCache.Cleanup)
	startPeriodicCleanup(ctx, rejectedTokens.Cleanup)
	startPeriodicCleanup(ctx, authLimiter.Cleanup)
	slog.Info("oauth mode enabled", "cache_ttl", cacheTTL, "resource", resourceID, "metadata_url", resourceMetadataURL)
}

// oauthCacheTTL returns a TTL the verifier can actually work with.
//
// The value is not only a cache lifetime: the verifier stamps it into
// auth.TokenInfo.Expiration, and the SDK's bearer middleware refuses a token
// whose expiration has passed. A zero or negative TTL therefore expires every
// token the instant it is verified, and the server answers 401 to every
// request carrying a perfectly valid credential.
//
// Normalizing here rather than rejecting at config validation keeps the
// invariant at the point that depends on it: a Config built by any path — the
// flag layer, the environment overlay, a test constructing a zero value —
// cannot produce that server.
func oauthCacheTTL(configured time.Duration) time.Duration {
	if configured > 0 {
		return configured
	}
	slog.Warn("oauth cache ttl is not positive; using the default",
		"configured", configured, "using", config.DefaultOAuthCacheTTL)
	return config.DefaultOAuthCacheTTL
}

func registerLegacyMCPHandlers(ctx context.Context, cfg *config.Config, pool *serverpool.ServerPool, sessionTags *sync.Map, mux *http.ServeMux) {
	authLimiter := serverpool.NewAuthRateLimiter(authFailureLimit, authFailureWindow)
	startPeriodicCleanup(ctx, authLimiter.Cleanup)
	gate := &mcpServerGate{
		pool:               pool,
		gitlabURLs:         cfg.InstanceURLs(),
		limiter:            authLimiter,
		trustedProxyHeader: cfg.TrustedProxyHeader,
		sessionTags:        sessionTags,
		challenge:          legacyAuthChallenge,
		stateless:          cfg.Stateless,
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(serverFromRequestContext, streamableHTTPOptions(cfg))
	mountMCPEndpoint(cfg, mux, gate.middleware(mcpHandler))
}

func startPeriodicCleanup(ctx context.Context, cleanup func()) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()
}

// processStartTime marks when this process began serving.
//
// Package-level initialization runs before main, so this is the earliest
// instant the program can observe about itself. Tests do not override it:
// newHealthResponse takes both instants as parameters instead, so uptime is
// deterministic without a mutable package-level clock.
var processStartTime = time.Now()

// healthResponse is the JSON body returned by the /health endpoint.
//
// Liveness is reported two ways on purpose. StartedAt is the stable fact: it
// does not change between probes, so a monitor can cache it, deduplicate it,
// and detect a restart by noticing it moved — the same reason Prometheus
// exposes process_start_time_seconds rather than an uptime counter.
// UptimeSeconds is the derived convenience value, in the unit the IETF health
// check draft uses for it ("observedUnit": "s").
type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// StartedAt is the process start instant in RFC 3339, matching how this
	// project renders timestamps everywhere else.
	StartedAt string `json:"started_at"`
	// UptimeSeconds is whole seconds since StartedAt. Sub-second precision
	// would be noise on an endpoint polled at probe intervals.
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// logIgnoredRequestOptions reports request-scoped configuration headers that
// were intentionally ignored because the server was started with fixed CLI
// configuration. The token is reduced to a masked suffix before logging.
func logIgnoredRequestOptions(token string, options serverpool.RequestOptions) {
	if !options.HasIgnoredOptions() {
		return
	}
	args := []any{
		"ignored_options", options.IgnoredOptionsCopy(),
		"token_suffix", safeTokenSuffix(token),
	}
	if deprecated := options.DeprecatedOptionsCopy(); len(deprecated) > 0 {
		args = append(
			args,
			"deprecated_options", deprecated,
			"deprecation_hint", "use TOOL_SURFACE instead of META_TOOLS",
		)
	}
	slog.Warn("request options ignored due to MCP configuration", args...) //#nosec G706 -- structured log uses constant option names and a masked token suffix only
}

func logLegacyMetaToolsDeprecation(toolSurfaceValue, metaToolsValue string) {
	if !config.LegacyMetaToolsSelectorInUse(toolSurfaceValue, metaToolsValue) {
		return
	}
	replacement := config.LegacyMetaToolsReplacement(metaToolsValue)
	if replacement == "" {
		return
	}
	slog.Warn(
		"META_TOOLS is deprecated; use TOOL_SURFACE instead", //#nosec G706 -- replacement is derived from supported TOOL_SURFACE constants and logged as structured data.
		"legacy_selector", "META_TOOLS",
		"replacement", "TOOL_SURFACE="+replacement,
	)
}

// logLegacyEnterpriseEnvDeprecation warns when the deprecated GITLAB_ENTERPRISE
// env var is the active tier source (GITLAB_TIER unset), pointing users to
// GITLAB_TIER. GITLAB_ENTERPRISE=true maps to GITLAB_TIER=ultimate, false to free.
func logLegacyEnterpriseEnvDeprecation(tierValue, enterpriseValue string) {
	if !config.LegacyEnterpriseEnvInUse(tierValue, enterpriseValue) {
		return
	}
	slog.Warn(
		"GITLAB_ENTERPRISE is deprecated; use GITLAB_TIER (free/premium/ultimate) instead",
		"legacy_selector", "GITLAB_ENTERPRISE",
		"replacement", "GITLAB_TIER=ultimate (was true) or GITLAB_TIER=free (was false)",
	)
}

// safeTokenSuffix returns a masked token suffix suitable for structured logs.
// Short tokens are fully masked to avoid exposing low-entropy credentials.
func safeTokenSuffix(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return "..." + token[len(token)-4:]
}

// clientIP extracts the real client IP from the request. When a trusted
// proxy header is configured (e.g. CF-Connecting-IP, X-Real-IP, X-Forwarded-For),
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

// buildTrustedOrigins merges the explicit --trusted-origins list with the
// origin of --public-url (deduplicated). Seeding the public-url origin makes
// the deployment self-consistent: a browser client on the deployment's own
// declared origin is exactly the same-domain case, and in oauth mode
// public-url is mandatory, so the origin RFC 9728 discovery points at is
// trusted without extra configuration. A public-url with no parseable
// origin is skipped here; config validation rejects a malformed one.
func buildTrustedOrigins(csv, publicURL string) []string {
	seen := map[string]bool{}
	var origins []string
	add := func(o string) {
		o = strings.TrimSpace(o)
		if o == "" || seen[o] {
			return
		}
		seen[o] = true
		origins = append(origins, o)
	}
	for o := range strings.SplitSeq(csv, ",") {
		add(o)
	}
	if publicURL != "" {
		if u, err := url.Parse(publicURL); err == nil && u.Host != "" {
			add(u.Scheme + "://" + u.Host)
		}
	}
	return origins
}

// CORS values for the MCP endpoint. The header list is the union of what the
// transport defines and what this server reads: a browser sends only the
// headers it was told it may send, so anything omitted here becomes a header
// a browser client silently cannot use.
const (
	corsAllowMethods = "GET, POST, DELETE, OPTIONS"
	// corsBaseAllowHeaders is what every mode accepts. The credential
	// headers a given mode honors are appended by corsAllowHeadersFor:
	// advertising one a mode ignores tells a browser client to send
	// something that cannot work.
	// Mcp-Method and Mcp-Name are REQUIRED by the SDK from protocol 2026-07-28
	// onwards — a POST without Mcp-Method is rejected before any handler runs.
	// Omitting them here meant the preflight refused the very headers the
	// server then demanded, so no browser client could speak the current
	// protocol at all, whatever its origin was allowed to do.
	//
	// Mcp-Param-* is not a fixed family: each name is derived from an
	// x-mcp-header annotation on a tool parameter, so this list is built from
	// the annotation this server actually declares. Naming ones it does not
	// declare authorizes nothing, and omitting the one it does reintroduces
	// the same failure for every call on the default surface — a browser drops
	// the unauthorized header, and the server rejects the call for its
	// absence.
	corsBaseAllowHeaders = "Authorization, Content-Type, Accept, " +
		"Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-ID, " +
		"Mcp-Method, Mcp-Name, " + dynamictools.ExecuteActionHeaderName
	// The session and protocol headers are unsafelisted response headers, so
	// a browser cannot read them unless they are exposed by name.
	//
	// WWW-Authenticate is in that same category, and leaving it out defeated
	// the discovery the challenge exists for: a cross-origin browser client
	// got the 401 but could not read the header, so it could not learn the
	// resource_metadata URL and could not start the OAuth flow. The one
	// audience CORS serves was the one audience automatic discovery did not
	// reach.
	//
	// Retry-After is the same shape of omission one status code over. The
	// server answers 429 and 503 with it — an auth-failure lockout, the
	// tool-call limiter, and GitLab's own throttle passed through — and a
	// browser client that cannot read it has to guess a backoff, which is how
	// a rate limit turns into a retry storm against the limit that caused it.
	corsExposeHeaders = "Mcp-Session-Id, Mcp-Protocol-Version, WWW-Authenticate, Retry-After"
	corsMaxAge        = "86400"
)

// corsAllowHeadersFor lists the request headers a browser may send to this
// deployment: the base set plus only the credential headers this mode
// actually honors.
//
// PRIVATE-TOKEN is a legacy-mode credential; in oauth mode the request gate
// reads Authorization: Bearer and nothing else, so a browser told it may send
// PRIVATE-TOKEN would send a header that produces a 401. GITLAB-URL is
// honored only when the deployment did not fix an instance with --gitlab-url,
// which is what makes the header meaningful in the first place.
func corsAllowHeadersFor(cfg *config.Config) string {
	allowed := corsBaseAllowHeaders
	if cfg.AuthMode != config.AuthModeOAuth {
		allowed += ", " + serverpool.RequestOptionPrivateToken
	}
	if gitLabURLHeaderHonored(cfg) {
		allowed += ", " + serverpool.RequestOptionGitLabURL
	}
	return allowed
}

// gitLabURLHeaderHonored reports whether a per-request GITLAB-URL header can
// change which instance a request reaches.
//
// It cannot when the deployment pinned exactly one instance: the header is
// logged as ignored, so advertising it would be a standing invitation to send
// something that silently does nothing. It can with none (the caller chooses
// freely) and with several (the caller selects among the published ones).
func gitLabURLHeaderHonored(cfg *config.Config) bool {
	return len(cfg.InstanceURLs()) != 1
}

// mcpEndpointPath is the named path the MCP endpoint answers on, alongside
// the root. Clients configured with a bare origin reach the root; the named
// form is what the guides and the debugging recipes use.
const mcpEndpointPath = "/mcp"

// Paths the server card is published at.
const (
	// sessionTagSeparator divides the pool tag from the random part of a
	// session ID. "." is not produced by rand.Text(), so the split is
	// unambiguous.
	sessionTagSeparator = "."
	// serverCardPath is the location the server-card extension recommends.
	serverCardPath = "/server-card"
	// serverCardLegacyPath is the location its earlier draft recommended,
	// kept mounted for scanners already fetching it.
	serverCardLegacyPath = "/.well-known/mcp/server-card.json"
)

// CORS response and request header names, each used on more than one path.
const (
	headerAllowOrigin    = "Access-Control-Allow-Origin"
	headerRequestMethod  = "Access-Control-Request-Method"
	headerRequestHeaders = "Access-Control-Request-Headers"
	headerOrigin         = "Origin"
	// headerSecFetchSite carries the browser's own statement about where the
	// request came from. It outranks an Origin comparison because only the
	// browser knows whether a navigation was same-origin.
	headerSecFetchSite = "Sec-Fetch-Site"
	headerVary         = "Vary"
)

// The content type every hand-written response in this package sends. They
// live here rather than in the test file that used to hold them: a constant
// only the tests can see does nothing for the production paths that repeat
// the literal, which is what SonarCloud flags.
const (
	hdrContentType = "Content-Type"
	mimeJSON       = "application/json"
)

// corsMiddleware makes trusted origins usable from an actual browser.
//
// Allowing an origin past the cross-origin protection is only half of it. A
// browser will not even send the POST until a preflight OPTIONS comes back
// with permission, and in oauth mode that preflight — which carries no
// Authorization header, by definition — was answered 401 by the bearer
// middleware. The effect was that --trusted-origins worked for anything
// except the browser clients it exists for, unless a reverse proxy in front
// answered the preflight on the server's behalf.
//
// Only origins the operator has trusted get PERMISSION. An untrusted origin's
// ordinary request is passed through unchanged and reaches the protection
// below, which is what refuses it; no header here weakens that decision.
//
// An untrusted origin's preflight is NOT answered here: some routes serve
// their own (the RFC 9728 metadata document and the server card are public
// and answer any origin), and swallowing those would make them undiscoverable
// from a browser. What must not happen is the preflight reaching the
// authentication layer and being charged there as a failed authentication —
// that is fixed in the guard and the gate, which let a preflight past
// untouched.
func corsMiddleware(cfg *config.Config, next http.Handler) http.Handler {
	trustedOrigins := cfg.TrustedOrigins
	allowHeaders := corsAllowHeadersFor(cfg)
	allowAll := slices.Contains(trustedOrigins, "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get(headerOrigin)
		preflight := r.Method == http.MethodOptions && r.Header.Get(headerRequestMethod) != ""
		trusted := origin != "" && (allowAll || slices.Contains(trustedOrigins, origin))

		if !trusted {
			if preflight {
				// Vary even when granting nothing: the answer downstream
				// depends on Origin, and a cache that does not know that
				// would serve it to a trusted origin.
				w.Header().Add(headerVary, headerOrigin)
			}
			next.ServeHTTP(w, r)
			return
		}
		// Vary regardless of the outcome: the response body for a given URL
		// does not depend on Origin, but these headers do, and a cache that
		// does not know that would serve one origin's permission to another.
		w.Header().Add(headerVary, headerOrigin)
		// The origin is echoed rather than answered with "*" because these
		// requests carry an Authorization header; "*" is invalid for a
		// credentialed request and browsers reject it.
		w.Header().Set(headerAllowOrigin, origin)
		w.Header().Set("Access-Control-Expose-Headers", corsExposeHeaders)

		if r.Method == http.MethodOptions && r.Header.Get(headerRequestMethod) != "" {
			w.Header().Add(headerVary, headerRequestMethod)
			w.Header().Add(headerVary, headerRequestHeaders)
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			w.Header().Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// crossOriginProtectionMiddleware applies explicit CSRF-oriented checks for
// browser-originated HTTP requests. The MCP SDK no longer enables this by
// default when StreamableHTTPOptions.CrossOriginProtection is nil. Origins
// in cfg.TrustedOrigins are allowed to make cross-origin requests; a
// malformed origin is fatal, since silently dropping it would leave the
// deployment believing an origin is trusted when it is not.
func crossOriginProtectionMiddleware(trustedOrigins []string, next http.Handler) http.Handler {
	// "*" is an explicit opt-out: accept every origin, which disables the
	// DNS-rebinding protection entirely. It is the right choice for a
	// deployment reachable only over a trusted network (local dev, a stack
	// behind a same-origin proxy that is the sole ingress), and it mirrors
	// an Access-Control-Allow-Origin: * the operator has decided to honor.
	if slices.Contains(trustedOrigins, "*") {
		slog.Warn("cross-origin protection disabled: --trusted-origins contains '*' (every origin accepted)")
		return next
	}
	protection := http.NewCrossOriginProtection()
	for _, origin := range trustedOrigins {
		// Errors are unreachable: validateHTTPRuntimeConfig has already
		// accepted every origin with the same AddTrustedOrigin call before
		// the server was built. The check stays so a future caller that
		// skips validation still cannot silently trust nothing.
		_ = protection.AddTrustedOrigin(origin)
	}
	// The standard library's own refusal is plain text, which is the one body
	// shape a Streamable HTTP client must not be handed: a client that gets a
	// 4xx whose body is not a recognized JSON-RPC error is told to conclude the
	// server predates version negotiation. Every other rejection this binary
	// emits is a JSON-RPC error, so this one is too.
	protection.SetDenyHandler(http.HandlerFunc(writeUntrustedOrigin))
	return protection.Handler(next)
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
			// JSON-RPC rather than http.Error's plain text, for the same
			// reason the cross-origin refusal is: an unparseable 4xx body
			// reads to a Streamable HTTP client as a pre-negotiation server.
			(&gateFailure{
				status:  http.StatusForbidden,
				code:    errCodeForbidden,
				message: "Request refused: the Host header names a host this deployment does not serve.",
			}).write(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// healthHandler responds with HTTP 200 and a JSON body for container healthchecks
// and load-balancer probes. It does not require authentication.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(hdrContentType, mimeJSON)
	_ = json.NewEncoder(w).Encode(newHealthResponse(processStartTime, time.Now())) //nolint:errchkjson // healthcheck: client write errors are non-actionable
}

// newHealthResponse builds the /health body for a start instant observed at
// now. Both instants are parameters so the uptime arithmetic can be tested
// without mutating a package-level clock from concurrent tests.
func newHealthResponse(startedAt, now time.Time) healthResponse {
	// Truncating instead of rounding keeps uptime from reporting a second that
	// has not fully elapsed. The clamp guards a caller that observes an instant
	// before the start; time.Now within one process cannot, because its
	// monotonic reading never goes backwards.
	uptime := int64(now.Sub(startedAt).Seconds())
	uptime = max(uptime, 0)
	return healthResponse{
		Status:        "ok",
		Version:       version,
		Commit:        commit,
		StartedAt:     startedAt.UTC().Format(time.RFC3339),
		UptimeSeconds: uptime,
	}
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
// buildServerCardFn is the card builder serveHTTPOn uses; a variable so a
// test can substitute a builder it controls and drive the
// shutdown-during-build path deterministically instead of racing a sleep
// against the real build.
var buildServerCardFn = buildServerCard //nolint:gochecknoglobals // test seam

// writeCardUnavailable answers a card request that could not be served.
//
// http.Error would label this JSON body text/plain, and every response here
// also carries X-Content-Type-Options: nosniff — so a browser would be told
// not to sniff, and then told the wrong type. The body has always been JSON;
// only the header was wrong.
func writeCardUnavailable(w http.ResponseWriter) {
	w.Header().Set(hdrContentType, mimeJSON)
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"error":"server card unavailable"}` + "\n"))
}

// serverCardAuthentication describes how a client authenticates against THIS
// deployment, which is not a property of the binary: the same build serves
// static tokens in legacy mode and Bearer-only OAuth in oauth mode.
//
// The card used to state header-token unconditionally. A deployment running
// --auth-mode=oauth therefore published a card contradicting its own 401
// challenge, and a directory rendering the card told users to send a header
// the endpoint refuses. Deriving it from the mode is the whole fix.
//
// In oauth mode the card also points at the RFC 9728 document rather than
// restating what it contains: that document is the authority on the
// authorization servers and scopes, and a copy here would be a second place
// to keep in sync.
func serverCardAuthentication(cfg *config.Config) map[string]any {
	if cfg.AuthMode == config.AuthModeOAuth {
		authInfo := map[string]any{
			"required": true,
			"schemes":  []string{"oauth2"},
			// Named as the RFC 9728 field a client already knows, so a
			// consumer that understands protected-resource metadata can
			// follow it without a card-specific convention.
			"scopes": []string{oauth.RequiredScope(cfg.ReadOnly, cfg.SafeMode)},
		}
		if cfg.PublicURL != "" {
			authInfo["resourceMetadata"] = oauth.MetadataURLFor(cfg.PublicURL)
		}
		return authInfo
	}
	return map[string]any{
		"required": true,
		"schemes":  []string{"header-token"},
	}
}

func buildServerCard(ctx context.Context, cfg *config.Config) ([]byte, error) {
	// Use configured URL or a placeholder — the dummy client only needs a
	// parseable URL to register tools; it never makes real API calls.
	gitlabURL := cfg.GitLabURL
	if gitlabURL == "" {
		gitlabURL = config.DefaultGitLabURL
	}

	dummyClient, err := gitlabclient.NewClientWithToken(gitlabURL, "dummy-token-for-tool-discovery", cfg.SkipTLSVerify)
	if err != nil {
		return nil, fmt.Errorf("creating dummy client: %w", err)
	}

	// createServer's internal tool-exclusion pass runs an ephemeral
	// in-memory session of its own; the caller's context governs the card
	// session below, not that registration detail.
	srv, err := createServer(dummyClient, cfg.ServerConfig(), nil) //nolint:contextcheck // see above
	if err != nil {
		return nil, fmt.Errorf("creating server-card MCP server: %w", err)
	}

	st, ct := mcp.NewInMemoryTransports()
	// The 30-second ceiling derives from the server's lifecycle context, so
	// a shutdown that begins while the card is still being built cancels
	// the build instead of waiting behind it.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
		// Icons travel in the card exactly as tools/list publishes them
		// (SVG plus light/dark WebP): a directory renders from the card
		// alone, and a card without them shows less than the live surface.
		Icons []mcp.Icon `json:"icons,omitempty"`
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
			Icons:        t.Icons,
		})
	}

	type serverCardResource struct {
		URI         string           `json:"uri"`
		Name        string           `json:"name,omitempty"`
		Title       string           `json:"title,omitempty"`
		Description string           `json:"description,omitempty"`
		MIMEType    string           `json:"mimeType,omitempty"`
		Size        int64            `json:"size,omitempty"`
		Annotations *mcp.Annotations `json:"annotations,omitempty"`
		Icons       []mcp.Icon       `json:"icons,omitempty"`
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
			Size:        r.Size,
			Annotations: r.Annotations,
			Icons:       r.Icons,
		})
	}

	type serverCardResourceTemplate struct {
		URITemplate string           `json:"uriTemplate"`
		Name        string           `json:"name,omitempty"`
		Title       string           `json:"title,omitempty"`
		Description string           `json:"description,omitempty"`
		MIMEType    string           `json:"mimeType,omitempty"`
		Annotations *mcp.Annotations `json:"annotations,omitempty"`
		Icons       []mcp.Icon       `json:"icons,omitempty"`
		// Carries the vendor-namespaced subscribable key, so a card
		// consumer can filter subscribable templates per object as well
		// as read the aggregate subscriptions block.
		Meta mcp.Meta `json:"_meta,omitempty"`
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
			Icons:       rt.Icons,
			Meta:        rt.Meta,
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
		Icons       []mcp.Icon                 `json:"icons,omitempty"`
	}

	cardPrompts := []serverCardPrompt{}
	if config.EffectiveCapabilitySurface(cfg.CapabilitySurface) == config.CapabilitySurfaceFull {
		promptsResult, promptsErr := session.ListPrompts(ctx, nil)
		if promptsErr != nil {
			return nil, fmt.Errorf("list prompts: %w", promptsErr)
		}
		cardPrompts = make([]serverCardPrompt, 0, len(promptsResult.Prompts))
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
				Icons:       p.Icons,
			})
		}
	}

	// Take serverInfo from the handshake rather than restating it, so the card
	// cannot advertise less than the server does: it previously hardcoded name
	// and version and silently dropped Title, Description, WebsiteURL and
	// Icons — exactly the fields a registry listing renders.
	//
	// capabilities comes from the same handshake, for the same reason: a
	// directory reading the card could not learn that this server supports
	// resource subscriptions except by grepping English prose. The card
	// follows the original SEP-1649 shape (which required this key); its
	// successor SEP-2127 deliberately carries no capabilities and no
	// primitives, but this card already enumerates tools and resources, so
	// it is a SEP-1649-lineage document and states capabilities structurally.
	card := map[string]any{
		"serverInfo":        session.InitializeResult().ServerInfo,
		"capabilities":      session.InitializeResult().Capabilities,
		"authentication":    serverCardAuthentication(cfg),
		"tools":             cardTools,
		"resources":         cardResources,
		"resourceTemplates": cardTemplates,
		"prompts":           cardPrompts,
	}

	// The subscription surface, machine-readable: the same whitelist the
	// gitlab://tools manifest advertises and the SubscribeHandler enforces,
	// plus per-method availability a consumer can branch on without parsing
	// prose. Both methods are always listed — the block describes the
	// binary's capability surface — but "available" states what THIS
	// deployment answers: the legacy resources/subscribe verb is refused on
	// stateless HTTP (each POST's session closes with its response, so an
	// accepted subscription could never notify), where subscriptions/listen
	// is the working form.
	if config.EffectiveCapabilitySurface(cfg.CapabilitySurface) == config.CapabilitySurfaceFull {
		card["subscriptions"] = map[string]any{
			"supported": true,
			"methods": map[string]any{
				"subscriptions/listen": map[string]any{
					"available":      true,
					"since_protocol": "2026-07-28",
				},
				"resources/subscribe": map[string]any{
					"available": !cfg.Stateless,
					"requires":  "stateful sessions (--stateless=false)",
				},
			},
			"subscribable_uri_templates": subscriptions.Templates(),
			"notification":               "notifications/resources/updated, sent when the watched content changes (server polls GitLab)",
		}
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

type stdioAutoUpdateCheckFunc func(context.Context, autoupdate.Config) (newVersion string, updated bool, err error)

func runStdioAutoUpdateCheck(ctx context.Context, cfg autoupdate.Config) (newVersion string, updated bool, err error) {
	updater, err := autoupdate.NewUpdater(cfg)
	if err != nil {
		return "", false, err
	}
	return updater.CheckOnce(ctx)
}

// startStdioAutoUpdate launches the startup update check for stdio mode.
// The check runs in the background so MCP startup and tool negotiation are not
// delayed by release detection, download, or binary replacement.
func startStdioAutoUpdate(ctx context.Context, cfg *config.Config) {
	startStdioAutoUpdateWithCheck(ctx, cfg, runStdioAutoUpdateCheck)
}

func startStdioAutoUpdateWithCheck(ctx context.Context, cfg *config.Config, check stdioAutoUpdateCheckFunc) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("autoupdate: startup auto-update panicked — continuing without update", "panic", r)
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
	if autoupdate.JustUpdated() {
		autoupdate.ClearJustUpdated()
		slog.Info("autoupdate: skipping startup update check (just re-executed after update)")
		return
	}

	timeout := cfg.AutoUpdateTimeout
	if timeout <= 0 {
		timeout = config.DefaultAutoUpdateTimeout
	}
	updateCfg := autoupdate.Config{
		Mode:           mode,
		Repository:     cfg.AutoUpdateRepo,
		Timeout:        timeout,
		CurrentVersion: version,
	}

	slog.Info(
		"autoupdate: starting startup check in background",
		"mode", mode,
		"repository", cfg.AutoUpdateRepo,
		"current_version", version,
	)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("autoupdate: startup auto-update goroutine panicked — continuing without update", "panic", r)
			}
		}()

		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		newVersion, updated, checkErr := check(checkCtx, updateCfg)
		if checkErr != nil {
			slog.Warn("autoupdate: startup background check failed", "error", checkErr)
			return
		}
		if updated && newVersion != "" {
			slog.Info(
				"autoupdate: binary updated in background — restart the server to use the new version",
				"new_version", newVersion,
			)
		}
	}()
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
	registered, err := listRegisteredToolsForInspection(server, "counter")
	if err != nil {
		return 0, err
	}
	return len(registered), nil
}

// MCP inspection hooks are replaceable in tests so error paths from in-memory
// server/client setup can be exercised without mutating production behavior.
var (
	listRegisteredToolsForInspection = listRegisteredTools
	newInspectionTransports          = mcp.NewInMemoryTransports
	connectInspectionServer          = func(server *mcp.Server, ctx context.Context, transport mcp.Transport) (*mcp.ServerSession, error) {
		return server.Connect(ctx, transport, nil)
	}
	connectInspectionClient = func(client *mcp.Client, ctx context.Context, transport mcp.Transport) (*mcp.ClientSession, error) {
		return client.Connect(ctx, transport, nil)
	}
	listInspectionTools = func(session *mcp.ClientSession, ctx context.Context) (*mcp.ListToolsResult, error) {
		return session.ListTools(ctx, nil)
	}
)

// listRegisteredTools connects an in-memory MCP client to server and returns
// the tool catalog advertised through tools/list.
func listRegisteredTools(server *mcp.Server, clientName string) ([]*mcp.Tool, error) {
	st, ct := newInspectionTransports()
	ctx := context.Background()

	serverSession, err := connectInspectionServer(server, ctx, st)
	if err != nil {
		return nil, fmt.Errorf("server connect: %w", err)
	}
	defer serverSession.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "0"}, nil)
	session, err := connectInspectionClient(mcpClient, ctx, ct)
	if err != nil {
		return nil, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	result, err := listInspectionTools(session, ctx)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return result.Tools, nil
}

// visibleMetaSchemaRoutes filters catalog-derived route maps to the tools
// still visible after read-only or exclude-tools registration filters run.
func visibleMetaSchemaRoutes(server *mcp.Server, routes map[string]toolutil.ActionMap) (map[string]toolutil.ActionMap, error) {
	registeredTools, err := listRegisteredToolsForInspection(server, "meta-schema-filter")
	if err != nil {
		return nil, err
	}

	visibleTools := make(map[string]struct{}, len(registeredTools))
	for _, tool := range registeredTools {
		visibleTools[tool.Name] = struct{}{}
	}

	visibleRoutes := make(map[string]toolutil.ActionMap, len(routes))
	for toolName, actions := range routes {
		if _, ok := visibleTools[toolName]; ok {
			visibleRoutes[toolName] = actions
		}
	}
	return visibleRoutes, nil
}

// buildDynamicActionCatalog builds the executable catalog for low-token dynamic
// mode. Filters run before standalone tools are added so configured exclusions,
// token scopes, and read-only mode cannot leave hidden catalog actions behind.
func buildDynamicActionCatalog(client *gitlabclient.Client, cfg *config.ServerConfig, updater *autoupdate.Updater) (*actioncatalog.Catalog, withheldActions, error) {
	catalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{
		Tier:       cfg.Tier,
		IncludeMCP: true,
		Updater:    updater,
	})
	if err != nil {
		return nil, withheldActions{}, fmt.Errorf("build action catalog: %w", err)
	}
	filtered, withheld, filterErr := filterActionCatalog(catalog, cfg)
	if filterErr != nil {
		return nil, withheldActions{}, fmt.Errorf("filter dynamic action catalog: %w", filterErr)
	}
	withStandalone, standaloneErr := dynamictools.AddStandaloneCatalog(filtered, client, dynamictools.StandaloneOptions{
		ReadOnly:     cfg.ReadOnly,
		ExcludeTools: cfg.ExcludeTools,
	})
	if standaloneErr != nil {
		return nil, withheldActions{}, fmt.Errorf("add standalone dynamic actions: %w", standaloneErr)
	}
	return withStandalone, withheld, nil
}

// withheldActions records the catalog actions a filter removed, split by whose
// decision it was. Only the token-scope half is something the caller can act
// on, so the two must not be merged into one message.
type withheldActions struct {
	byTokenScope []string
	byOperator   []string
}

func filterActionCatalog(catalog *actioncatalog.Catalog, cfg *config.ServerConfig) (*actioncatalog.Catalog, withheldActions, error) {
	var withheld withheldActions
	// Tools the operator excluded by name are not "withheld": the point of the
	// exclusion is that they do not exist for this deployment, so naming them
	// in an error would both leak the configuration and contradict it.
	filtered := catalog.FilterExcludedTools(cfg.ExcludeTools)
	scoped, err := gitlabtools.FilterScopeFilteredCatalog(filtered, cfg.TokenScopes)
	if err != nil {
		return nil, withheldActions{}, err
	}
	withheld.byTokenScope = removedActionKeys(filtered, scoped)
	filtered = scoped
	if cfg.ReadOnly {
		// Filter at action granularity, not group granularity: a domain that
		// mixes reads and writes must keep its read actions reachable instead
		// of disappearing with them.
		readable := filtered.FilterReadOnlyActions()
		removed := removedActionKeys(filtered, readable)
		if cfg.ReadOnlyFromTokenScope {
			withheld.byTokenScope = append(withheld.byTokenScope, removed...)
		} else {
			withheld.byOperator = append(withheld.byOperator, removed...)
		}
		filtered = readable
	}
	if cfg.SafeMode {
		// Same granularity argument: dispatcher tools cover reads and writes
		// alike, so safe mode is applied per action in the catalog rather than
		// by intercepting whole tools.
		filtered = filtered.WithSafeModePreviews()
	}
	return filtered, withheld, nil
}

// removedActionKeys lists every canonical action ID, and every alias resolving
// to one, that `before` carried and `after` does not.
//
// Aliases count because a caller who asked find for an action before the
// narrowing, or who is working from documentation, names the action the way the
// catalog used to: answering only the canonical form leaves the alias reported
// as a typo, which is the misdiagnosis this exists to prevent.
func removedActionKeys(before, after *actioncatalog.Catalog) []string {
	if before == nil || after == nil {
		return nil
	}
	kept := make(map[actioncatalog.ActionID]struct{})
	for _, action := range after.Actions() {
		kept[action.ID] = struct{}{}
	}
	var keys []string
	for _, action := range before.Actions() {
		if _, ok := kept[action.ID]; ok {
			continue
		}
		keys = append(keys, string(action.ID))
		keys = append(keys, action.Aliases...)
	}
	return keys
}

// countCatalogActions sums actions across catalog route maps for startup logs.
func countCatalogActions(routes map[string]toolutil.ActionMap) int {
	total := 0
	for _, actions := range routes {
		total += len(actions)
	}
	return total
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
func runToolSearch(query, toolSurface string, tier edition.Tier) {
	if err := toolSearchRunner(query, toolSurface, tier); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		exitProcess(1)
	}
}

// Tool search hooks are replaceable in tests so runToolSearch can be verified
// without terminating the test process through os.Exit.
var (
	toolSearchRunner = doToolSearch
	exitProcess      = os.Exit
)

// doToolSearch builds the selected MCP tool catalog, searches tool names and
// descriptions with case-insensitive AND matching, and prints matching tool
// names with the first description line.
func doToolSearch(query, toolSurface string, tier edition.Tier) error {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "search", Version: version}, &mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}})

	switch config.EffectiveToolSurface(true, toolSurface) {
	case config.ToolSurfaceMeta:
		if err := gitlabtools.RegisterAllMeta(server, nil, tier); err != nil {
			return err
		}
	case config.ToolSurfaceIndividual:
		gitlabtools.RegisterAll(server, nil, tier)
	case config.ToolSurfaceDynamic:
		catalog, err := buildToolSearchCatalog(tier)
		if err != nil {
			return err
		}
		dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	default:
		return fmt.Errorf("unsupported tool surface for search: %q", toolSurface)
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

func buildToolSearchCatalog(tier edition.Tier) (*actioncatalog.Catalog, error) {
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{
		Tier:       tier,
		IncludeMCP: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}
	withStandalone, standaloneErr := dynamictools.AddStandaloneCatalog(catalog, nil, dynamictools.StandaloneOptions{})
	if standaloneErr != nil {
		return nil, fmt.Errorf("add standalone dynamic actions: %w", standaloneErr)
	}
	return withStandalone, nil
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

// redactAttr redacts configured auto-update URL fragments from string-valued
// log attributes before they are forwarded to the wrapped handler.
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
