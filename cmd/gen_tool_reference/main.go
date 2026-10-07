package main

import (
	// embed is imported for its go:embed directive alone, which reads
	// domains.json into domainsJSON.
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

const (
	// toolName names the flag set, so a usage message names this command
	// rather than the test binary that drove it.
	toolName = "gen_tool_reference"

	// regenerateHint is what a stale page tells its reader to run.
	regenerateHint = "make gen-tool-reference"

	// pageExtension is the extension of every page this command owns.
	pageExtension = ".mdx"
)

// domainsJSON is the hand-written half of the reference: what each group is
// for and what a reader would ask of it, in both languages.
//
//go:embed domains.json
var domainsJSON []byte

// Seams over os.Exit and run, so the exit code this command hands the process
// is reachable from a test, and so both of runMain's outcomes can be driven
// without building the catalog or touching the committed pages.
var (
	osExit       = os.Exit
	runReference = run
)

// main writes the reference pages, or with -check reports the stale ones.
func main() {
	osExit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses args, the command line with the program name removed, and
// returns the process exit code. The flag set is ContinueOnError, so a bad
// flag is an exit code rather than an os.Exit the seam never sees, and -h is
// the one parse failure that exits clean.
func runMain(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "report the pages that differ from what the catalog generates, without writing them")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if err := runReference(defaultSource(), *check, stdout); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	return 0
}

// source is everything a run reads: where the repository is, the catalogs and
// the surfaces the pages describe, the scope requirements of the groups, the
// table the catalog's token requirements point into with the routes of it
// GitLab refuses to an OAuth token, and the hand-written data file. A test
// hands run a small one.
type source struct {
	root         func() (string, error)
	builds       func() ([]build, error)
	surfaces     func() surfaceNames
	scopes       map[string][]string
	grants       *finegrained.Table
	oauthRefused []oauthRefusedRoute
	domains      []byte
}

// defaultSource is the source the command reads: the repository it runs in,
// the catalogs this binary builds, the action grants table compiled into it,
// the OAuth refusals declared here, and the embedded data file.
func defaultSource() source {
	return source{
		root:         mcpsurface.ProjectRoot,
		builds:       defaultBuilds,
		surfaces:     defaultSurfaces,
		scopes:       tools.MetaToolScopes,
		grants:       actiongrants.Table(),
		oauthRefused: oauthRefusedRoutes,
		domains:      domainsJSON,
	}
}

// run generates every page and writes it, or under check compares it with the
// committed one, then removes (or under check reports) any page in the two
// directories this run did not generate.
func run(src source, check bool, stdout io.Writer) error {
	root, err := src.root()
	if err != nil {
		return err
	}
	data, err := decodeDomains(src.domains)
	if err != nil {
		return err
	}
	builds, err := src.builds()
	if err != nil {
		return err
	}
	ref, err := assemble(builds, src.surfaces(), src.scopes, src.grants, src.oauthRefused)
	if err != nil {
		return err
	}
	if coverErr := data.covers(ref.groups); coverErr != nil {
		return coverErr
	}
	if labelErr := checkLabels(ref.groups); labelErr != nil {
		return labelErr
	}
	pages := renderPages(ref, data)
	var stale []string
	for _, page := range pages {
		if writeErr := docgen.WriteOrCheck(filepath.Join(root, page.path), page.content, check, regenerateHint); writeErr != nil {
			stale = append(stale, writeErr.Error())
		}
	}
	for _, language := range languages {
		orphans, pruneErr := prune(root, language.dir, pages, check)
		if pruneErr != nil {
			return pruneErr
		}
		stale = append(stale, orphans...)
	}
	if len(stale) > 0 {
		return errors.New(strings.Join(stale, "\n"))
	}
	fmt.Fprintf(stdout, "%s: %d pages, %d groups, %d actions\n", verb(check), len(pages), len(ref.groups), ref.actionCount())
	return nil
}

// verb is how the summary line says what a run did.
func verb(check bool) string {
	if check {
		return "current"
	}
	return "wrote"
}

// prune removes from dir every page this run did not generate, or under check
// reports each as stale. A directory that does not exist has nothing to prune;
// under check, its missing pages are already reported by the comparison.
// Something other than a directory at that path is an error on every system:
// Windows reports reading a file as a directory as a path that does not exist,
// so a not-exist answer is believed only when nothing is there at all.
func prune(root, dir string, pages []page, check bool) ([]string, error) {
	path := filepath.Join(root, dir)
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) && !occupied(path) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	generated := make(map[string]struct{}, len(pages))
	for _, page := range pages {
		generated[filepath.ToSlash(page.path)] = struct{}{}
	}
	var orphans []string
	for _, entry := range entries {
		rel := dir + "/" + entry.Name()
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), pageExtension) {
			continue
		}
		if _, ok := generated[rel]; ok {
			continue
		}
		if check {
			orphans = append(orphans, fmt.Sprintf("%s is not a page the catalog generates; run %s", rel, regenerateHint))
			continue
		}
		if removeErr := removeFile(filepath.Join(root, filepath.FromSlash(rel))); removeErr != nil {
			return nil, fmt.Errorf("remove %s: %w", rel, removeErr)
		}
	}
	return orphans, nil
}

// occupied reports whether anything at all is at path, a dangling symbolic
// link included, which is why it asks Lstat rather than Stat.
func occupied(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// removeFile is os.Remove behind a variable: the file it is handed was listed
// a moment earlier in a directory this run just wrote to, so only a test can
// make the removal fail.
var removeFile = os.Remove
