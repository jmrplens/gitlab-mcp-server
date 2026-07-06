// Command audit_e2e_gaps reports which canonical catalog actions are not
// exercised by the e2e suite under test/e2e/suite.
//
// It builds the Ultimate-tier action catalog offline and scans the suite
// sources for the three invocation shapes: individual tool names
// ("gitlab_branch_create"), meta calls (a "gitlab_branch" literal followed by
// an "action": "create" pair within a short window), and dynamic execute
// calls referencing canonical "domain.action" IDs. An action counts as
// exercised when any surface references it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// metaActionWindow is how many lines after a "gitlab_x" tool literal an
// "action": "y" pair still counts as the same meta call. Suite meta calls put
// the action key on the line right after the tool name; the window absorbs
// formatting drift without pairing unrelated literals.
const metaActionWindow = 6

var (
	toolNameRe = regexp.MustCompile(`"(gitlab_[a-z0-9_]+)"`)
	actionKVRe = regexp.MustCompile(`"action"\s*:\s*"([a-z0-9_]+)"`)
	actionIDRe = regexp.MustCompile(`"([a-z0-9_]+\.[a-z0-9_]+)"`)
)

// gapRow describes one catalog action and whether the e2e suite exercises it.
type gapRow struct {
	ID          string   `json:"id"`
	Group       string   `json:"group"`
	Tool        string   `json:"tool"`
	Edition     string   `json:"edition"`
	ReadOnly    bool     `json:"read_only"`
	Destructive bool     `json:"destructive"`
	Files       []string `json:"files,omitempty"`
}

// report is the machine-readable output of one audit run.
type report struct {
	CatalogActions int      `json:"catalog_actions"`
	Exercised      int      `json:"exercised"`
	Uncovered      []gapRow `json:"uncovered"`
}

func main() {
	suiteDir := flag.String("suite", "test/e2e/suite", "e2e suite source directory")
	format := flag.String("output", "tsv", "output format: tsv or json")
	flag.Parse()
	os.Exit(run(os.Stdout, os.Stderr, *suiteDir, *format))
}

// run is the testable entry point: it builds the catalog, scans the suite,
// and writes the gap report. The audit is informational, so the exit code is
// 0 whenever the report is produced and non-zero only on build/scan errors.
func run(stdout, stderr io.Writer, suiteDir, format string) int {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		fmt.Fprintf(stderr, "build action catalog: %v\n", err)
		return 1
	}

	catalogIDs := make(map[string]struct{}, catalog.CountActions())
	for _, action := range catalog.Actions() {
		catalogIDs[string(action.ID)] = struct{}{}
	}

	exercised, err := scanSuite(suiteDir, catalogIDs)
	if err != nil {
		fmt.Fprintf(stderr, "scan suite: %v\n", err)
		return 1
	}

	rep := report{CatalogActions: catalog.CountActions()}
	for _, action := range catalog.Actions() {
		id := string(action.ID)
		files := exercised.filesFor(id, action.ToolName, action.Name, action.IndividualTool.Name)
		if len(files) > 0 {
			rep.Exercised++
			continue
		}
		rep.Uncovered = append(rep.Uncovered, gapRow{
			ID:          id,
			Group:       action.ToolName,
			Tool:        action.IndividualTool.Name,
			Edition:     action.Edition,
			ReadOnly:    action.ReadOnly,
			Destructive: action.Destructive,
		})
	}
	sort.Slice(rep.Uncovered, func(i, j int) bool { return rep.Uncovered[i].ID < rep.Uncovered[j].ID })

	switch format {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", " ")
		if encErr := encoder.Encode(rep); encErr != nil {
			fmt.Fprintf(stderr, "encode json: %v\n", encErr)
			return 1
		}
	case "tsv":
		for _, row := range rep.Uncovered {
			ed := row.Edition
			if ed == "" {
				ed = "free"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\treadonly=%t\tdestructive=%t\n", row.ID, row.Group, ed, row.ReadOnly, row.Destructive)
		}
		pct := 100 * float64(rep.Exercised) / float64(rep.CatalogActions)
		fmt.Fprintf(stdout, "e2e gap audit: %d/%d actions exercised (%.1f%%), %d uncovered\n",
			rep.Exercised, rep.CatalogActions, pct, len(rep.Uncovered))
	default:
		fmt.Fprintf(stderr, "invalid -output %q (want tsv or json)\n", format)
		return 2
	}
	return 0
}

// suiteIndex holds every action reference found in the suite sources, keyed
// by the three invocation shapes.
type suiteIndex struct {
	individual map[string]map[string]struct{}
	metaPairs  map[string]map[string]struct{}
	dynamicIDs map[string]map[string]struct{}
}

// filesFor returns the suite files that exercise the action through any
// surface: its individual tool name, its (group, action) meta pair, or its
// canonical dynamic ID.
func (s suiteIndex) filesFor(id, group, actionName, individualTool string) []string {
	files := make(map[string]struct{})
	for f := range s.individual[individualTool] {
		files[f] = struct{}{}
	}
	for f := range s.metaPairs[group+"\x00"+actionName] {
		files[f] = struct{}{}
	}
	for f := range s.dynamicIDs[id] {
		files[f] = struct{}{}
	}
	out := make([]string, 0, len(files))
	for f := range files {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// scanSuite walks every *_test.go file in suiteDir and indexes tool-name,
// meta-pair, and dynamic-ID references. Only IDs present in catalogIDs are
// indexed so arbitrary "a.b" literals cannot count as coverage.
func scanSuite(suiteDir string, catalogIDs map[string]struct{}) (suiteIndex, error) {
	index := suiteIndex{
		individual: make(map[string]map[string]struct{}),
		metaPairs:  make(map[string]map[string]struct{}),
		dynamicIDs: make(map[string]map[string]struct{}),
	}
	paths, err := filepath.Glob(filepath.Join(suiteDir, "*_test.go"))
	if err != nil {
		return index, fmt.Errorf("glob suite sources: %w", err)
	}
	if len(paths) == 0 {
		return index, fmt.Errorf("no *_test.go files under %s", suiteDir)
	}
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return index, fmt.Errorf("read %s: %w", path, readErr)
		}
		indexFile(&index, filepath.Base(path), strings.Split(string(data), "\n"), catalogIDs)
	}
	return index, nil
}

// indexFile records every action reference found in one suite file.
func indexFile(index *suiteIndex, fileName string, lines []string, catalogIDs map[string]struct{}) {
	addRef := func(refs map[string]map[string]struct{}, key string) {
		if refs[key] == nil {
			refs[key] = make(map[string]struct{})
		}
		refs[key][fileName] = struct{}{}
	}
	for i, line := range lines {
		for _, match := range actionIDRe.FindAllStringSubmatch(line, -1) {
			if _, ok := catalogIDs[match[1]]; ok {
				addRef(index.dynamicIDs, match[1])
			}
		}
		for _, match := range toolNameRe.FindAllStringSubmatch(line, -1) {
			tool := match[1]
			addRef(index.individual, tool)
			windowEnd := min(i+metaActionWindow, len(lines))
			window := strings.Join(lines[i:windowEnd], "\n")
			for _, actionMatch := range actionKVRe.FindAllStringSubmatch(window, -1) {
				addRef(index.metaPairs, tool+"\x00"+actionMatch[1])
			}
		}
	}
}
