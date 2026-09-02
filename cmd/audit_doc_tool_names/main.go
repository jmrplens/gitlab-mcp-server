// Command audit_doc_tool_names checks every `gitlab_*` tool name the
// documentation mentions against the names the server actually registers.
//
// cmd/audit_doc_coverage already audits the docs against the canonical action
// catalog, but it compares `domain.action` IDs — so a page can name a tool that
// no surface has ever registered and still audit clean. That is exactly how
// `gitlab_list_issues` survived in guides and examples: the individual surface
// projects domain-first names (`gitlab_issue_list`), so every copy-pasted
// verb-first example answered `unknown tool` at runtime.
//
// The name set is built in memory from the same registration paths the server
// uses, across the individual, meta and dynamic surfaces at the Ultimate tier,
// so it needs no network and cannot drift from the catalog.
//
// Usage:
//
//	go run ./cmd/audit_doc_tool_names/           # report
//	go run ./cmd/audit_doc_tool_names/ --check   # non-zero exit on any finding
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
)

const (
	auditServerName = "audit-doc-tool-names"
	auditClientName = "audit-doc-tool-names-client"
	auditVersion    = "0.0.1"
)

// docRoots are the trees whose Markdown mentions are audited.
//
// The npm launcher's README is in the list because it is published to a
// registry rather than only rendered from this repository: a wrong tool name
// there reaches users through `npm install` and is not fixable without
// republishing a version.
var docRoots = []string{"docs", "site/src/content/docs", "README.md", "llms-install.md", "CLAUDE.md", "npm/gitlab-mcp-server/README.md"}

// toolToken matches a tool-name-shaped token in prose or code.
var toolToken = regexp.MustCompile(`\bgitlab_[a-z0-9_]+\b`)

// allowed lists tokens that look like tool names but legitimately are not, or
// that name tools this audit cannot register. Each entry carries the reason so
// a future reader can tell an exemption from an oversight.
var allowed = map[string]string{
	"gitlab_com":            "stats.tools.gitlab_com. A generated data property, not a tool",
	"gitlab_tool":           "prose fragment",
	"gitlab_orbit":          "prose prefix for the gitlab_orbit_* family",
	"gitlab_url":            "mcpb user_config key (gitlab_url), not a tool",
	"gitlab_token":          "mcpb user_config key", //#nosec G101 -- a config key name, not a credential
	"gitlab_tier":           "mcpb user_config key",
	"gitlab_mcp":            "fragment of GITLAB_MCP_* env names and of the project slug",
	"gitlab_xxx":            "placeholder in prose",
	"gitlab_complete":       "MCP completion method in evaluation docs, not a tool",
	"gitlab_get_prompt":     "MCP prompts/get in evaluation docs, not a tool",
	"gitlab_list_prompts":   "MCP prompts/list in evaluation docs, not a tool",
	"gitlab_list_resources": "MCP resources/list in evaluation docs, not a tool",
	"gitlab_read_resource":  "MCP resources/read in evaluation docs, not a tool",
	"gitlab_ci_ymls":        "a GitLab template type and API path segment (templates/gitlab_ci_ymls)",
	"gitlab_duo":            "a docs.gitlab.com URL path segment (user/gitlab_duo/...)",
	"gitlab_status":         "a JSON struct field (json:\"gitlab_status\") quoted in error-handling docs",
	"gitlab_mcp_server":     "the Python import package of the PyPI distribution (python -m gitlab_mcp_server), not a tool",
}

// allowedPrefixes exempts whole families that exist only on a live GitLab.com
// Ultimate instance with the Knowledge Graph enabled, which no offline audit
// can register.
var allowedPrefixes = []string{"gitlab_orbit_"}

// wildcardSuffix marks a token written as a family prefix in prose, e.g.
// `gitlab_mr_approval_*` — the trailing underscore is the truncation, not a name.
const wildcardSuffix = "_"

// historicalDocs are files that deliberately quote names from an earlier design.
// An ADR records what was decided at the time; rewriting its examples to today's
// names would falsify the record.
var historicalDocs = []string{
	"docs/development/adr/",
}

func main() {
	check := flag.Bool("check", false, "exit non-zero when the docs name a tool that does not exist")
	flag.Parse()

	os.Exit(run(*check, docRoots, registeredToolNames, os.Stdout, os.Stderr))
}

// run audits roots against the names collectNames returns and reports on stdout,
// returning the process exit code: 1 when a name set or the scan cannot be
// built, 1 under check when any unregistered name is referenced, 0 otherwise.
func run(check bool, roots []string, collectNames func() (map[string]struct{}, error), stdout, stderr io.Writer) int {
	registered, err := collectNames()
	if err != nil {
		fmt.Fprintf(stderr, "collect tool names: %v\n", err)
		return 1
	}

	findings, scanned, err := scanDocs(roots, registered)
	if err != nil {
		fmt.Fprintf(stderr, "scan docs: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "audit_doc_tool_names: %d registered tool names, %d documentation files scanned\n",
		len(registered), scanned)

	if len(findings) == 0 {
		fmt.Fprintln(stdout, "no documentation names an unregistered tool")
		return 0
	}

	names := make([]string, 0, len(findings))
	for name := range findings {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if len(findings[names[i]]) != len(findings[names[j]]) {
			return len(findings[names[i]]) > len(findings[names[j]])
		}
		return names[i] < names[j]
	})

	fmt.Fprintf(stdout, "\n%d unregistered tool name(s) referenced:\n", len(names))
	for _, name := range names {
		files := findings[name]
		sort.Strings(files)
		fmt.Fprintf(stdout, "  %-38s %d file(s)\n", name, len(files))
		for _, f := range files {
			fmt.Fprintf(stdout, "      %s\n", f)
		}
	}

	if check {
		fmt.Fprintf(stderr, "\nERROR: the documentation names %d tool(s) the server does not register\n", len(names))
		return 1
	}
	return 0
}

// registeredToolNames builds every surface in memory and returns the union of
// the tool names they advertise.
func registeredToolNames() (map[string]struct{}, error) {
	client, cleanup, err := auditclient.NewMock()
	if err != nil {
		return nil, fmt.Errorf("create audit client: %w", err)
	}
	defer cleanup()

	names := make(map[string]struct{})

	// Mirror cmd/server's registerToolSurface, so gitlab_server and the
	// gitlab_server_* individual tools are in the collected set. Registering
	// only the catalog would make this audit report those names as unknown.
	individual := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion},
		&mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}})
	tools.RegisterAll(individual, client, edition.Ultimate)
	if collectErr := collect(individual, names); collectErr != nil {
		return nil, collectErr
	}

	meta := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion},
		&mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}})
	if metaErr := tools.RegisterAllMeta(meta, client, edition.Ultimate); metaErr != nil {
		return nil, fmt.Errorf("register meta tools: %w", metaErr)
	}
	tools.RegisterMCPMeta(meta, client)
	tools.RegisterMetaStandaloneTools(meta, client)
	if collectErr := collect(meta, names); collectErr != nil {
		return nil, collectErr
	}

	dynamic, dynErr := dynamicServer(client)
	if dynErr != nil {
		return nil, dynErr
	}
	if collectErr := collect(dynamic, names); collectErr != nil {
		return nil, collectErr
	}

	return names, nil
}

// dynamicServer registers the two-tool dynamic surface.
func dynamicServer(client *gitlabclient.Client) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: auditServerName, Version: auditVersion},
		&mcp.ServerOptions{PageSize: 2000, Capabilities: &mcp.ServerCapabilities{}})

	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		return nil, fmt.Errorf("build action catalog: %w", err)
	}
	catalog, err = dynamictools.AddStandaloneCatalog(catalog, client, dynamictools.StandaloneOptions{})
	if err != nil {
		return nil, fmt.Errorf("add standalone catalog: %w", err)
	}
	dynamictools.RegisterCatalogFindExecuteTools(server, catalog)
	return server, nil
}

// collect connects to server in memory and adds its tool names to names.
func collect(server *mcp.Server, names map[string]struct{}) error {
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		return fmt.Errorf("server connect: %w", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: auditClientName, Version: auditVersion}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		return fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	for _, tool := range result.Tools {
		names[tool.Name] = struct{}{}
	}
	return nil
}

// scanDocs walks the documentation roots and returns unregistered names mapped
// to the files that mention them, plus the number of files scanned.
func scanDocs(roots []string, registered map[string]struct{}) (findingsByName map[string][]string, filesScanned int, err error) {
	findingsByName = make(map[string][]string)

	for _, root := range roots {
		n, rootErr := scanRoot(root, registered, findingsByName)
		if rootErr != nil {
			return nil, 0, rootErr
		}
		filesScanned += n
	}
	return findingsByName, filesScanned, nil
}

// scanRoot scans one entry of docRoots, which may be a single file or a tree.
func scanRoot(root string, registered map[string]struct{}, findings map[string][]string) (int, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return scanFile(root, registered, findings)
	}

	scanned := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(path); ext != ".md" && ext != ".mdx" {
			return nil
		}
		n, fileErr := scanFile(path, registered, findings)
		scanned += n
		return fileErr
	})
	return scanned, err
}

// scanFile records unregistered tool names mentioned by one file.
func scanFile(path string, registered map[string]struct{}, findings map[string][]string) (int, error) {
	for _, prefix := range historicalDocs {
		if strings.HasPrefix(filepath.ToSlash(path), prefix) {
			return 0, nil
		}
	}

	data, err := os.ReadFile(path) //#nosec G304 -- audit tool reading repository docs
	if err != nil {
		return 0, err
	}

	seen := make(map[string]struct{})
	for _, token := range toolToken.FindAllString(string(data), -1) {
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		if _, ok := registered[token]; ok {
			continue
		}
		if _, ok := allowed[token]; ok {
			continue
		}
		if hasAllowedPrefix(token) || strings.HasSuffix(token, wildcardSuffix) {
			continue
		}
		findings[token] = append(findings[token], filepath.ToSlash(path))
	}
	return 1, nil
}

// hasAllowedPrefix reports whether token belongs to an exempted family.
func hasAllowedPrefix(token string) bool {
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}
