package main

import (
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"golang.org/x/tools/go/packages"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// prefix names this command on every line it writes, so a failure in a
// composite make target says which of them produced it.
const prefix = "audit_graphql_shapes:"

// auditPatterns are the packages the audit loads: every document this server
// sends, and every decoder it fills, lives under them.
var auditPatterns = []string{"./internal/..."}

// collectDocuments reads the documents the sibling audit finds, so a document
// this audit never paired is reported rather than missed. A seam, so the one
// failure it has (a source tree the collector cannot read) can be reached
// from a test.
var collectDocuments = graphqldocs.Collect

// readInventory reads the committed record of what this server sends GitLab,
// which is how the report names the GraphQL this walk cannot see: the
// operations client-go builds inside its own module, where there is neither a
// document to pair nor a decoder to compare. A seam, so both the run that
// finds the record and the run that does not are reachable from a test.
var readInventory = requestinventory.Read

// auditRun is one configured run: where to look, what to judge by, how much to
// say about what agreed, and where to write what the schema offers and nobody
// asks for.
type auditRun struct {
	dir        string
	verbose    bool
	patterns   []string
	schemaPath string
	reportPath string
	// declarations answer the sent findings this repository has decided
	// about. Carried here rather than read from the table directly so a
	// fixture run is not asked to explain the real tree's decisions, and
	// reports every one of them stale for not matching a synthetic one.
	declarations []sentDeclaration
}

// exitProcess ends the process with the status main decided. A seam, so a
// test can run main itself and read the status it would have exited with.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain reads the command line and runs the audit it describes, returning
// the exit status: 0 for -h, 2 for a command line it cannot read, as the flag
// package's own exit would, and otherwise what [run] decides. It is main with
// the process handed to it, so every flag's way into the run is reachable
// from a test.
func runMain(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("audit_graphql_shapes", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dir := flags.String("dir", ".", "repository root to audit")
	verbose := flags.Bool("v", false, "list every pairing judged, every selection nothing reads and every position left unjudged, not only the disagreements")
	schemaPath := flags.String("schema", "", "SDL file to judge the documents against, instead of the pinned schema")
	reportPath := flags.String("report", "", "write the fields the schema offers that no document of their package selects, as JSON, to this path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	return run(auditRun{
		dir:          *dir,
		verbose:      *verbose,
		patterns:     auditPatterns,
		schemaPath:   *schemaPath,
		reportPath:   *reportPath,
		declarations: declaredSent,
	}, out, errOut)
}

// run is main with its streams and its exit status handed to it, so the ways
// this audit ends are reachable from a test instead of only from a process.
func run(cfg auditRun, out, errOut io.Writer) int {
	schema, provenance, err := resolveSchema(cfg)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	loaded, err := loadProgram(cfg.dir, cfg.patterns)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	pairings, problems := loaded.pairings()

	var (
		findings []finding
		offered  []sentField
		coverage sentCoverage
	)
	selected := map[string]bool{}
	judged := make(map[string]bool, len(pairings))
	for i := range pairings {
		p := &pairings[i]
		judged[p.Text] = true
		document, parseErr := graphqlschema.ParseAgainst(schema, p.Text)
		if parseErr != nil {
			problems = append(problems, problem{
				Package:  p.Package,
				Position: p.Position,
				Message:  p.Label() + " is a document the schema refuses, which make check-graphql-documents reports; a selection set that does not resolve has no shape to judge",
			})
			continue
		}
		judgement := judgePairing(schema, document, p)
		findings = append(findings, judgement.findings...)
		offered = append(offered, judgement.sent...)
		coverage.add(judgement.coverage)
		// The evidence is unioned across every pairing before any of it is
		// subtracted, because a field one document leaves out is a gap only if
		// no other document of the package selects it.
		for key := range judgement.selected {
			selected[key] = true
		}
	}

	unpaired, err := unpairedDocuments(cfg, judged)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	problems = append(problems, unpaired...)

	stale, err := reportSent(cfg, out, sentRun{
		provenance: provenance,
		pkgs:       loaded.pkgs,
		pairings:   pairings,
		coverage:   coverage,
		offered:    offered,
		selected:   selected,
	})
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	return report(cfg, out, errOut, auditResult{
		provenance: provenance,
		pairings:   pairings,
		findings:   findings,
		problems:   problems,
		stale:      stale,
	})
}

// auditResult is everything one run produced, gathered so the renderer takes
// the run rather than a row of positional arguments: the provenance line every
// summary ends with, the pairings that were judged, the findings filed under
// them, the problems that belong to no pairing, and the declarations that no
// longer answer anything.
type auditResult struct {
	provenance string
	pairings   []pairing
	findings   []finding
	problems   []problem
	stale      []string
}

// reportSent classifies what the schema offers and nobody selects, writes the
// report where -report names, and returns the stale declarations as the
// problems they are.
//
// The findings themselves are not problems and never fail the run: a field
// GitLab offers that this server does not surface is a candidate for the
// surface rather than a defect in it, and the tier and deprecation oracles
// this dimension lacks mean a portion of them are not gaps at all. A
// declaration that answers nothing is a different statement, and is held to
// the same bar every declaration table here is held to.
func reportSent(cfg auditRun, out io.Writer, run sentRun) ([]string, error) {
	found := dedupeSent(run.offered, collectPublished(run.pkgs), run.selected, absolute(cfg.dir))
	classified, unused := classifySent(cfg.declarations, found)
	written := newSentReport(run.provenance, len(run.pairings), run.coverage, uncovered(cfg.dir, run.pairings), classified, unused)

	if cfg.reportPath != "" {
		if err := writeSentReport(cfg.reportPath, written); err != nil {
			return nil, err
		}
		fmt.Fprint(out, sentLine(cfg.reportPath, written.Summary, written.Check.Uncovered))
	}
	return staleSentDeclarations(unused), nil
}

// sentRun is what one walk produced for the sent dimension, gathered so the
// reporting step takes one argument rather than seven.
type sentRun struct {
	provenance string
	pkgs       []*packages.Package
	pairings   []pairing
	coverage   sentCoverage
	offered    []sentField
	selected   map[string]bool
}

// uncovered names the GraphQL the request inventory records that this walk
// asked nothing of.
//
// A record it cannot read is not a failure of this run: the inventory is
// committed beside the source and its own gate (make check-request-inventory)
// keeps it current, while an audit pointed at a fixture module has no reason
// to carry one. What the report must not do is call the set empty, so the
// failure is said in the report rather than swallowed.
func uncovered(dir string, pairings []pairing) sentUncovered {
	inventory, err := readInventory(dir)
	if err != nil {
		return unavailableUncovered()
	}
	return uncoveredGraphQL(inventory, pairings)
}

// resolveSchema decides what this run judges by: the pin, or the SDL file
// -schema names.
func resolveSchema(cfg auditRun) (*ast.Schema, string, error) {
	if cfg.schemaPath == "" {
		// The pinned schema and its provenance record are embedded, and their
		// own gate (make check-graphql-schema) refuses a build where either
		// does not load, so a failure here is not something this command
		// could act on.
		return cmdutil.Must(graphqlschema.Schema()), cmdutil.Must(graphqlschema.SourceInfo()).String(), nil
	}
	sdl, err := os.ReadFile(cfg.schemaPath) //#nosec G304 -- the path is the operator's own -schema flag
	if err != nil {
		return nil, "", fmt.Errorf("read the schema to judge against: %w", err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", cfg.schemaPath, err)
	}
	return schema, fmt.Sprintf("%d types from %s, not the pinned schema", len(schema.Types), cfg.schemaPath), nil
}

// unpairedDocuments reports every document the sibling audit reads out of
// the source that no send this audit paired carries.
//
// The two read the same tree and fold constants the same way, so a document
// in one list and not the other is a document handed to something this audit
// does not follow, which is a gap to close rather than a silence to keep. The
// lists are compared by text: a constant whose text a paired send already
// carries is taken as judged, since what was judged is the exchange, not the
// name it was declared under.
func unpairedDocuments(cfg auditRun, judged map[string]bool) ([]problem, error) {
	documents, err := collectDocuments(cfg.dir, cfg.patterns, nil)
	if err != nil {
		return nil, fmt.Errorf("read the documents: %w", err)
	}
	var problems []problem
	for _, document := range documents {
		if judged[document.Text] {
			continue
		}
		problems = append(problems, problem{
			Package:  document.Package,
			Position: document.Position,
			Message:  document.Label() + " is a document no send this audit can see carries, so its decoder is judged by nobody",
		})
	}
	return problems, nil
}

// report renders the run and returns its exit status.
//
// A pairing with a disagreement goes to stderr with every finding under it,
// the notes included when -v asks for them. A pairing with nothing but notes
// goes to stdout under -v, and a pairing with nothing at all is one "ok" line
// there. Every problem, and every declaration that no longer answers
// anything, is a stderr line of its own.
func report(cfg auditRun, out, errOut io.Writer, result auditResult) int {
	root := absolute(cfg.dir)
	byPairing := make(map[*pairing][]finding, len(result.pairings))
	for _, f := range result.findings {
		byPairing[f.pairing] = append(byPairing[f.pairing], f)
	}

	disagreements, unread := 0, 0
	var unjudged unjudgedCount
	for i := range result.pairings {
		p := &result.pairings[i]
		group := byPairing[p]
		fails, positions := false, 0
		for _, f := range group {
			switch {
			case f.Fails:
				fails = true
				disagreements++
			case f.Unjudged:
				positions++
			default:
				unread++
			}
		}
		unjudged.add(positions)
		if fails {
			fmt.Fprint(errOut, block(root, p, group, cfg.verbose))
			continue
		}
		if !cfg.verbose {
			continue
		}
		if len(group) > 0 {
			fmt.Fprint(out, block(root, p, group, true))
			continue
		}
		fmt.Fprintf(out, "    ok  %s\n", heading(root, p))
	}
	for _, trouble := range result.problems {
		fmt.Fprintf(errOut, "%s (%s): %s\n", trouble.Package, relative(trouble.Position, root), trouble.Message)
	}
	for _, message := range result.stale {
		fmt.Fprintf(errOut, "sent_declarations.go: %s\n", message)
	}

	if len(result.pairings) == 0 {
		fmt.Fprintf(errOut, "\n%s found no send to judge under %s\n", prefix, strings.Join(cfg.patterns, " "))
		return 1
	}
	if disagreements > 0 || len(result.problems) > 0 || len(result.stale) > 0 {
		fmt.Fprintf(errOut, "\n%s %d disagreement(s) in %d pairing(s), %d unpaired or unreadable, %d stale declaration(s)%s (%s)\n",
			prefix, disagreements, len(result.pairings), len(result.problems), len(result.stale), unjudged.clause(), result.provenance)
		return 1
	}
	fmt.Fprintf(out, "%s %d pairing(s) agree with their documents, %d selection(s) nothing reads%s (%s)\n",
		prefix, len(result.pairings)-unjudged.pairings, unread, unjudged.clause(), result.provenance)
	return 0
}

// unjudgedCount is how many positions a type parameter no caller binds left
// unjudged, and in how many pairings.
//
// Both summaries carry it, since a failing run that leaves a decoder unjudged
// has left it unjudged all the same, and a reader who sees only the summary
// line is owed that on either. The pairings holding one are not counted among
// those that agree: a pairing that agrees except where it was never asked has
// not been shown to agree.
type unjudgedCount struct {
	positions int
	pairings  int
}

// add counts one pairing's unjudged positions.
func (u *unjudgedCount) add(positions int) {
	if positions == 0 {
		return
	}
	u.positions += positions
	u.pairings++
}

// clause names the count for a summary line, and is empty when there is
// none, so a run with no such decoder says exactly what it said before the
// count existed and the summary of a tree without one stays the same byte for
// byte.
func (u *unjudgedCount) clause() string {
	if u.positions == 0 {
		return ""
	}
	return fmt.Sprintf(", %d position(s) in %d pairing(s) left unjudged, typed by a parameter no caller binds", u.positions, u.pairings)
}

// block renders one pairing with its findings under it.
func block(root string, p *pairing, group []finding, notes bool) string {
	var text strings.Builder
	text.WriteString(heading(root, p) + "\n")
	for _, f := range group {
		switch {
		case f.Fails:
			fmt.Fprintf(&text, "    - %s: %s\n", f.Path, f.Message)
		case notes:
			fmt.Fprintf(&text, "    ~ %s: %s\n", f.Path, f.Message)
		}
	}
	return text.String()
}

// heading names a pairing: the package, the document, the call, and where the
// document was handed over when the call did not name it.
func heading(root string, p *pairing) string {
	where := relative(p.Position, root)
	if p.Origin.IsValid() {
		where += ", handed over at " + relative(p.Origin, root)
	}
	return fmt.Sprintf("%s %s (%s)", p.Package, p.Label(), where)
}

// absolute makes the audited root absolute, so positions, which come out of
// the loader absolute, can be trimmed against it whatever -dir was written as.
func absolute(dir string) string {
	if resolved, err := filepath.Abs(dir); err == nil {
		return resolved
	}
	return dir
}

// relative trims a position to the repository root so a finding reads as a
// path a person can open.
func relative(position token.Position, root string) string {
	path := position.Filename
	if trimmed, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(trimmed, "..") {
		path = filepath.ToSlash(trimmed)
	}
	return fmt.Sprintf("%s:%d", path, position.Line)
}
