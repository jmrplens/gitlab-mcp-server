package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
)

// toolName is the command's own name, used as the flag set's name so a usage
// message names the command rather than the test binary that drove it.
const toolName = "gen_model_corpus"

// defaultOutputPath is the committed ledger this command owns.
const defaultOutputPath = "docs/development/testing/model-corpus.md"

// regenerate is the sentence a stale artifact is reported with.
const regenerate = "make gen-model-corpus"

// osExit is os.Exit behind a variable, so the one line main carries is
// reachable from a test rather than only from a process.
var osExit = os.Exit

// main renders the breadth ledger, or verifies the committed one.
//
// It is one of the four readers of the corpus's answer key, and it is
// sanctioned for the reason the results publisher is: what it writes is about
// the key, and it produces no stimulus. The actions, domains and tiers a
// ledger names live in the key and nowhere else, so a ledger built from the
// stimuli alone could say how many cases there are and nothing about what they
// reach.
//
// Usage:
//
//	go run ./cmd/gen_model_corpus/          # rewrite the ledger
//	go run ./cmd/gen_model_corpus/ -check   # fail when it is stale
func main() {
	osExit(runMain(os.Args[1:], os.Stderr))
}

// runMain parses args, the command line with the program name already removed,
// and returns the process exit code.
//
// The flag set is ContinueOnError rather than the package-level ExitOnError
// one, so a bad flag is an exit code this function returns instead of an
// os.Exit the seam above never sees; -h is the one parse failure that exits
// clean, as ExitOnError would. The two failures below both exit 1 and are
// fixed differently, so each names its own stage: one means this is not a
// checkout, the other that the render or the comparison refused.
func runMain(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	outputPath := fs.String("output", defaultOutputPath, "generated ledger path")
	check := fs.Bool("check", false, "verify the committed ledger is current without writing")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintf(stderr, "find repository root: %v\n", err)
		return 1
	}
	if runErr := run(root, *outputPath, *check); runErr != nil {
		fmt.Fprintf(stderr, "%v\n", runErr)
		return 1
	}
	return 0
}

// readCatalogFacts is the one reader of the catalog, held in a variable so a
// test can make it fail. What a failure here produces is a sentence naming the
// stage rather than a ledger with holes in it, and that is worth a test.
var readCatalogFacts = readCatalog

// run renders the ledger and either writes it or compares it.
func run(root, outputPath string, check bool) error {
	catalog, err := readCatalogFacts()
	if err != nil {
		return fmt.Errorf("read the action catalog: %w", err)
	}
	ledger, err := render(catalog)
	if err != nil {
		return fmt.Errorf("render the ledger: %w", err)
	}
	if writeErr := docgen.WriteOrCheck(filepath.Join(root, outputPath), ledger, check, regenerate); writeErr != nil {
		return fmt.Errorf("model corpus ledger: %w", writeErr)
	}
	return nil
}

// catalogAction is what the ledger says about one action of the catalog.
type catalogAction struct {
	domain string
	tier   string
}

// catalogFacts is the catalog as the ledger reads it: every action, so the
// corpus can be counted against the whole of it rather than against itself.
type catalogFacts struct {
	actions map[string]catalogAction
	domains map[string]int
}

// readCatalog builds the catalog at Ultimate, with the MCP diagnostics group
// in, which is the catalog the corpus is checked against and the widest one
// any instance serves.
func readCatalog() (catalogFacts, error) {
	built, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{
		Tier:       edition.Ultimate,
		IncludeMCP: true,
	})
	if err != nil {
		return catalogFacts{}, err
	}
	facts := catalogFacts{
		actions: make(map[string]catalogAction, built.CountActions()),
		domains: map[string]int{},
	}
	for _, action := range built.Actions() {
		facts.actions[string(action.ID)] = catalogAction{
			domain: action.Domain,
			tier:   edition.TierFromEdition(action.Edition).String(),
		}
		facts.domains[action.Domain]++
	}
	return facts, nil
}

// namedBy indexes what the corpus names, and which cases name it.
type namedBy struct {
	actions    map[string][]string
	standalone map[string][]string
	recipes    map[string][]string
	needs      map[string][]string
	steps      int
	unknown    []string
}

// readCorpus folds the corpus into what the ledger publishes.
func readCorpus(catalog catalogFacts) namedBy {
	named := namedBy{
		actions:    map[string][]string{},
		standalone: map[string][]string{},
		recipes:    map[string][]string{},
		needs:      map[string][]string{},
	}
	for _, stimulus := range modelcorpus.Stimuli() {
		named.recipes[string(stimulus.Recipe)] = append(named.recipes[string(stimulus.Recipe)], stimulus.ID)
		for _, need := range needsOf(stimulus) {
			named.needs[need] = append(named.needs[need], stimulus.ID)
		}
	}
	keys := modelcorpus.Keys()
	for _, id := range modelcorpus.IDs() {
		for _, step := range keys[id].Steps {
			named.steps++
			if step.Standalone != "" {
				named.standalone[step.Standalone] = appendOnce(named.standalone[step.Standalone], id)
				continue
			}
			action := string(step.Action)
			named.actions[action] = appendOnce(named.actions[action], id)
			if _, known := catalog.actions[action]; !known {
				named.unknown = appendOnce(named.unknown, action)
			}
		}
	}
	return named
}

// needsOf spells what one case asks of the instance, as the ledger groups it.
func needsOf(stimulus modelcorpus.Stimulus) []string {
	needs := []string{"tier " + string(stimulus.Needs.MinimumTier())}
	if stimulus.Needs.Runner {
		needs = append(needs, "a CI runner")
	}
	if stimulus.Needs.Admin {
		needs = append(needs, "an administrator token")
	}
	if stimulus.Needs.FixtureService {
		needs = append(needs, "the fixture service")
	}
	return needs
}

// appendOnce appends value unless it is already the last one, which is all the
// de-duplication a case that names one action twice needs.
func appendOnce(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

// render writes the ledger.
func render(catalog catalogFacts) ([]byte, error) {
	named := readCorpus(catalog)
	if len(named.unknown) > 0 {
		return nil, fmt.Errorf("the corpus names %d action(s) the catalog does not have: %s",
			len(named.unknown), strings.Join(named.unknown, ", "))
	}
	var page bytes.Buffer
	writeHeader(&page)
	writeSummary(&page, catalog, named)
	writeDomains(&page, catalog, named)
	writeActions(&page, catalog, named)
	writeStandalone(&page, named)
	writeRecipes(&page, named)
	writeNeeds(&page, named)
	// Each section ends with the blank line that separates it from the next,
	// which after the last one is a blank line at the end of the file that
	// markdownlint reads as one too many.
	return append(bytes.TrimRight(page.Bytes(), "\n"), '\n'), nil
}

// writeHeader writes the front matter and the sentence that says what this
// page is not.
func writeHeader(page *bytes.Buffer) {
	page.WriteString(`# Model evaluation corpus breadth

> **Diátaxis type**: Reference
> **Audience**: 🔧 Maintainers, contributors
> **Prerequisites**: none; this page is generated from the corpus and the catalog
>
> Generated by ` + "`" + regenerate + "`" + ` from ` +
		"`internal/testutil/modelcorpus`" + `. Do not edit by hand.

---

**This is breadth, not coverage.** It says which catalog actions the model
evaluation corpus asks a model to reach, and nothing whatever about how well
any model reaches them or about how much of the server is tested. What the
end-to-end suite asserts is a different question with a different page
(` + "`docs/development/testing/e2e-coverage.md`" + `), and a share here is of
the catalog at Ultimate, which is the widest catalog any instance serves.

A case names an action through its answer key, so this page is one of the four
readers of that key. It publishes about the key and produces no stimulus,
which is the ground the boundary test sanctions it on.

`)
}

// writeSummary writes the figures a reader wants first.
func writeSummary(page *bytes.Buffer, catalog catalogFacts, named namedBy) {
	rows := [][]string{
		{"Cases", strconv.Itoa(len(modelcorpus.IDs()))},
		{"Steps declared", strconv.Itoa(named.steps)},
		{"Catalog actions named", fmt.Sprintf("%d of %d", len(named.actions), len(catalog.actions))},
		{"Catalog domains named", fmt.Sprintf("%d of %d", len(domainsNamed(catalog, named)), len(catalog.domains))},
		{"Standalone tools named", strconv.Itoa(len(named.standalone))},
		{"Worlds asked for", fmt.Sprintf("%d of %d", len(named.recipes), len(modelcorpus.Recipes()))},
	}
	page.WriteString("## Summary\n\n")
	page.WriteString(docgen.RenderMarkdownTable(
		[]string{"Figure", "Count"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		rows,
	))
	page.WriteString("\n")
}

// domainsNamed counts the actions the corpus names per catalog domain.
func domainsNamed(catalog catalogFacts, named namedBy) map[string]int {
	domains := map[string]int{}
	for action := range named.actions {
		domains[catalog.actions[action].domain]++
	}
	return domains
}

// writeDomains writes what the corpus reaches of each domain it touches.
func writeDomains(page *bytes.Buffer, catalog catalogFacts, named namedBy) {
	counted := domainsNamed(catalog, named)
	domains := make([]string, 0, len(counted))
	for domain := range counted {
		domains = append(domains, domain)
	}
	slices.Sort(domains)

	rows := make([][]string, 0, len(domains))
	for _, domain := range domains {
		rows = append(rows, []string{
			domain,
			strconv.Itoa(counted[domain]),
			strconv.Itoa(catalog.domains[domain]),
		})
	}
	page.WriteString("## Domains the corpus names\n\n")
	page.WriteString(docgen.RenderMarkdownTable(
		[]string{"Domain", "Actions named", "Actions in the catalog"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight, docgen.AlignRight},
		rows,
	))
	page.WriteString("\n")
}

// writeActions writes every catalog action the corpus names, with the cases
// that name it.
func writeActions(page *bytes.Buffer, catalog catalogFacts, named namedBy) {
	actions := make([]string, 0, len(named.actions))
	for action := range named.actions {
		actions = append(actions, action)
	}
	slices.Sort(actions)

	rows := make([][]string, 0, len(actions))
	for _, action := range actions {
		rows = append(rows, []string{
			"`" + action + "`",
			catalog.actions[action].tier,
			strings.Join(named.actions[action], ", "),
		})
	}
	page.WriteString("## Actions the corpus names\n\n")
	page.WriteString(docgen.RenderMarkdownTable(
		[]string{"Action", "Tier", "Named by"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft, docgen.AlignLeft},
		rows,
	))
	page.WriteString("\n")
}

// writeStandalone writes the tools registered outside the catalog that the
// corpus names, which no catalog count can include.
func writeStandalone(page *bytes.Buffer, named namedBy) {
	rows := indexRows(named.standalone, asCode, true)
	page.WriteString("## Standalone tools the corpus names\n\n")
	page.WriteString(docgen.RenderMarkdownTable(
		[]string{"Tool", "Named by"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignLeft},
		rows,
	))
	page.WriteString("\n")
}

// writeRecipes writes the worlds the corpus asks a fixture library to build.
func writeRecipes(page *bytes.Buffer, named namedBy) {
	rows := indexRows(named.recipes, asCode, false)
	page.WriteString("## Worlds the corpus asks for\n\n")
	page.WriteString(docgen.RenderMarkdownTable(
		[]string{"Recipe", "Cases"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		rows,
	))
	page.WriteString("\n")
}

// writeNeeds writes what the corpus asks of the instance it runs against.
func writeNeeds(page *bytes.Buffer, named namedBy) {
	rows := indexRows(named.needs, asPlain, false)
	page.WriteString("## What the corpus needs of an instance\n\n")
	page.WriteString(docgen.RenderMarkdownTable(
		[]string{"Requirement", "Cases"},
		[]docgen.Alignment{docgen.AlignLeft, docgen.AlignRight},
		rows,
	))
	page.WriteString("\n")
}

// indexRows renders a name-to-cases index as two sorted columns. The second
// is the case identifiers themselves where a reader would follow them (a
// handful of tools), and their count where the list would be a paragraph in a
// table cell (a recipe every project case shares).
func indexRows(index map[string][]string, spell func(string) string, listCases bool) [][]string {
	names := make([]string, 0, len(index))
	for name := range index {
		names = append(names, name)
	}
	slices.Sort(names)

	rows := make([][]string, 0, len(names))
	for _, name := range names {
		cases := index[name]
		second := strconv.Itoa(len(cases))
		if listCases {
			listed := slices.Clone(cases)
			slices.Sort(listed)
			second = strings.Join(listed, ", ")
		}
		rows = append(rows, []string{spell(name), second})
	}
	return rows
}

// asCode spells a name that is an identifier, and plainly spells one that is a
// sentence fragment.
func asCode(name string) string { return "`" + name + "`" }

// asPlain spells a name that is already English.
func asPlain(name string) string { return name }
