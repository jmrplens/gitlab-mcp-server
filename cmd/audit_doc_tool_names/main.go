package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionids"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcewalk"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
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
	"gitlab_com":        "stats.tools.gitlab_com. A generated data property, not a tool",
	"gitlab_orbit":      "prose prefix for the gitlab_orbit_* family",
	"gitlab_url":        "mcpb user_config key (gitlab_url), not a tool",
	"gitlab_refused":    "a model evaluation record value: the answer a step carries when GitLab refused a correctly dispatched call",
	"gitlab_mcp":        "fragment of GITLAB_MCP_* env names and of the project slug",
	"gitlab_xxx":        "placeholder in prose",
	"gitlab_ci_ymls":    "a GitLab template type and API path segment (templates/gitlab_ci_ymls)",
	"gitlab_duo":        "a docs.gitlab.com URL path segment (user/gitlab_duo/...)",
	"gitlab_status":     "a JSON struct field (json:\"gitlab_status\") quoted in error-handling docs",
	"gitlab_mcp_server": "the Python import package of the PyPI distribution (python -m gitlab_mcp_server), not a tool",
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

// run audits roots against the names collectNames returns and the action IDs
// the catalog builds, reports on stdout, and returns the process exit code: 1
// when the scan cannot be made, 1 under check when the documentation names
// anything a model cannot call, 0 otherwise.
//
// collectNames reads registration compiled into this binary and so cannot
// fail; the catalog and the documentation tree are the two things here that
// can, and they are the only things that can send run home with a 1 it did not
// intend.
func run(check bool, roots []string, collectNames func() map[string]struct{}, stdout, stderr io.Writer) int {
	ids, err := actionids.Build()
	if err != nil {
		fmt.Fprintf(stderr, "build the action catalog: %v\n", err)
		return 1
	}
	registered := collectNames()

	scan := newDocScan(registered, ids)
	if scanErr := scan.scanDocs(roots); scanErr != nil {
		fmt.Fprintf(stderr, "scan docs: %v\n", scanErr)
		return 1
	}

	fmt.Fprintf(stdout, "audit_doc_tool_names: %d registered tool names, %d catalog action IDs, %d documentation files scanned\n",
		len(registered), ids.Count(), scan.files)

	// The declaration table can only be held to the documentation by a run
	// that read all of it: over one page every entry excuses nothing, and
	// reporting them all stale would be an answer about the roots rather than
	// about the declarations.
	var stale []string
	if slices.Equal(roots, docRoots) {
		stale = staleAllowedIDs(scan.usedIDExemptions)
	} else {
		fmt.Fprintln(stdout, "  the declaration table was not judged: only a run over every documentation root can tell a stale entry from a narrowed run")
	}
	writeToolFindings(stdout, scan.tools)
	writeIDFindings(stdout, scan.actions, ids)
	writeStaleIDs(stdout, stale)
	if len(scan.tools) == 0 && len(scan.actions) == 0 && len(stale) == 0 {
		fmt.Fprintln(stdout, "no documentation names an unregistered tool or an action the catalog does not have")
		return 0
	}

	if check {
		fmt.Fprintf(stderr,
			"\nERROR: the documentation names %d tool(s) the server does not register and %d action ID(s) the catalog does not publish; %d declaration(s) excuse nothing\n",
			len(scan.tools), len(scan.actions), len(stale))
		return 1
	}
	return 0
}

// writeToolFindings prints the unregistered tool names under the files that
// mention each.
func writeToolFindings(out io.Writer, findings map[string][]string) {
	if len(findings) == 0 {
		return
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

	fmt.Fprintf(out, "\n%d unregistered tool name(s) referenced:\n", len(names))
	for _, name := range names {
		files := findings[name]
		sort.Strings(files)
		fmt.Fprintf(out, "  %-38s %d file(s)\n", name, len(files))
		for _, file := range files {
			fmt.Fprintf(out, "      %s\n", file)
		}
	}
}

// writeIDFindings prints the action IDs the documentation teaches that the
// catalog does not publish, each with the nearest ID it does, since the usual
// cause is a domain the page invented for a real action.
func writeIDFindings(out io.Writer, findings map[string]idFinding, ids *actionids.IDs) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%d action ID(s) the catalog does not publish:\n", len(findings))
	for _, token := range sortedTokens(findings) {
		finding := findings[token]
		sort.Strings(finding.Files)
		suffix := ""
		if closest := ids.Closest(token); finding.Canonical == "" && closest != "" {
			suffix = "; closest: " + closest
		}
		fmt.Fprintf(out, "  %-38s %d file(s) %s%s\n", token, len(finding.Files), finding.describe(token), suffix)
		for _, file := range finding.Files {
			fmt.Fprintf(out, "      %s\n", file)
		}
	}
}

// writeStaleIDs prints the declarations that excused nothing, which is a
// finding of its own: a declaration that has stopped describing the
// documentation is one a reader would otherwise trust.
func writeStaleIDs(out io.Writer, stale []string) {
	if len(stale) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%d declaration(s) excuse nothing:\n", len(stale))
	for _, token := range stale {
		fmt.Fprintf(out, "  %s is no longer spelled in any documentation file. Remove the entry from allowedIDs.\n", token)
	}
}

// registeredToolNames builds every surface in memory and returns the union of
// the tool names they advertise.
//
// Every step registers the catalog compiled into this binary onto a server this
// function just made, so a failure here would mean the committed catalog is
// broken, which the docs audit can neither report usefully nor work around.
func registeredToolNames() map[string]struct{} {
	client, cleanup := auditshared.NewStubGitLabClient(auditshared.StubToken)
	defer cleanup()

	names := make(map[string]struct{})

	// [mcpsurface] registers what cmd/server registers for each surface, so
	// gitlab_server and the gitlab_server_* individual tools are in the
	// collected set. Registering only the catalog would make this audit report
	// those names as unknown.
	collect(mcpsurface.IndividualTools(client, edition.Ultimate), names)
	collect(mcpsurface.MetaTools(client, edition.Ultimate), names)
	collect(mcpsurface.DynamicTools(client), names)

	return names
}

// collect adds the names of listed to names.
func collect(listed []*mcp.Tool, names map[string]struct{}) {
	for _, tool := range listed {
		names[tool.Name] = struct{}{}
	}
}

// docScan is one pass over the documentation roots: what a mention is judged
// against, and what the pass found.
//
// The two questions are asked of the same files in the same read, because they
// are one question about one sentence. A page teaching a call names the tool
// on the individual surface and the canonical ID on the dynamic one, and until
// this command could see the second half, a page could pair the right tool
// name with an ID no surface resolves and audit clean. Three of those shipped.
type docScan struct {
	registered map[string]struct{}
	ids        *actionids.IDs
	// tools maps an unregistered tool name to the files that mention it, and
	// actions does the same for a dotted ID the catalog does not hold.
	tools   map[string][]string
	actions map[string]idFinding
	// usedIDExemptions is what the run excused, which the stale list is
	// computed against.
	usedIDExemptions map[string]struct{}
	files            int
}

// newDocScan prepares a pass over the documentation.
func newDocScan(registered map[string]struct{}, ids *actionids.IDs) *docScan {
	return &docScan{
		registered:       registered,
		ids:              ids,
		tools:            map[string][]string{},
		actions:          map[string]idFinding{},
		usedIDExemptions: map[string]struct{}{},
	}
}

// scanDocs walks the documentation roots and records what it found.
func (s *docScan) scanDocs(roots []string) error {
	for _, root := range roots {
		if err := s.scanRoot(root); err != nil {
			return err
		}
	}
	return nil
}

// scanRoot scans one entry of docRoots, which may be a single file or a tree.
func (s *docScan) scanRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return s.scanFile(root)
	}

	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// The root is entered whatever it is called, so a root that is
			// itself a checkout or a dot-directory is still scanned.
			if path != root && (d.Name() == "node_modules" || sourcewalk.SkipDirBelowRoot(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(path); ext != ".md" && ext != ".mdx" {
			return nil
		}
		return s.scanFile(path)
	})
}

// scanFile records the unregistered tool names and the unresolvable action IDs
// one file mentions.
func (s *docScan) scanFile(path string) error {
	for _, prefix := range historicalDocs {
		if strings.HasPrefix(filepath.ToSlash(path), prefix) {
			return nil
		}
	}

	data, err := os.ReadFile(path) //#nosec G304 -- audit tool reading repository docs
	if err != nil {
		return err
	}
	s.files++

	s.scanToolNames(filepath.ToSlash(path), string(data))
	s.scanActionIDs(filepath.ToSlash(path), string(data))
	return nil
}

// scanToolNames records the gitlab_* names one file mentions that no surface
// registers.
func (s *docScan) scanToolNames(path, text string) {
	seen := make(map[string]struct{})
	for _, token := range toolToken.FindAllString(text, -1) {
		if _, repeated := seen[token]; repeated {
			continue
		}
		seen[token] = struct{}{}
		if _, ok := s.registered[token]; ok {
			continue
		}
		if _, ok := allowed[token]; ok {
			continue
		}
		if hasAllowedPrefix(token) || strings.HasSuffix(token, wildcardSuffix) {
			continue
		}
		s.tools[token] = append(s.tools[token], path)
	}
}

// scanActionIDs records the dotted tokens one file offers as action IDs that
// the catalog does not hold.
//
// Which tokens are offered as IDs is the shared rule in cmd/internal/actionids,
// the same one the source gate applies to a Usage line, so a spelling cannot be
// a cross-link in the code and prose in the docs.
func (s *docScan) scanActionIDs(path, text string) {
	for _, token := range s.ids.Candidates(text) {
		if s.ids.IsID(token) || isIDFamilyPrefix(token) {
			continue
		}
		if declared, tracked := exemptID(token); declared {
			if tracked {
				s.usedIDExemptions[token] = struct{}{}
			}
			continue
		}
		finding := s.actions[token]
		if canonical, isAlias := s.ids.Alias(token); isAlias {
			finding.Canonical = canonical
		}
		finding.Files = append(finding.Files, path)
		s.actions[token] = finding
	}
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
