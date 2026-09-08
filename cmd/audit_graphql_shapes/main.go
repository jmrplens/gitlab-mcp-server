package main

import (
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
var auditPatterns = []string{"./internal/..."} //nolint:gochecknoglobals // the default main hands run

// collectDocuments reads the documents the sibling audit finds, so a document
// this audit never paired is reported rather than missed. A seam, so the one
// failure it has (a source tree the collector cannot read) can be reached
// from a test.
var collectDocuments = graphqldocs.Collect //nolint:gochecknoglobals // test seam

// readInventory reads the committed record of what this server sends GitLab,
// which is how the report names the GraphQL this walk cannot see: the
// operations client-go builds inside its own module, where there is neither a
// document to pair nor a decoder to compare. A seam, so both the run that
// finds the record and the run that does not are reachable from a test.
var readInventory = requestinventory.Read //nolint:gochecknoglobals // test seam

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

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every pairing judged and every selection nothing reads, not only the disagreements")
	schemaPath := flag.String("schema", "", "SDL file to judge the documents against, instead of the pinned schema")
	reportPath := flag.String("report", "", "write the fields the schema offers that no document of their package selects, as JSON, to this path")
	flag.Parse()

	os.Exit(run(auditRun{
		dir:          *dir,
		verbose:      *verbose,
		patterns:     auditPatterns,
		schemaPath:   *schemaPath,
		reportPath:   *reportPath,
		declarations: declaredSent,
	}, os.Stdout, os.Stderr))
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

	return report(cfg, out, errOut, provenance, pairings, findings, problems, stale)
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
func report(cfg auditRun, out, errOut io.Writer, provenance string, pairings []pairing, findings []finding, problems []problem, stale []string) int {
	root := absolute(cfg.dir)
	byPairing := make(map[*pairing][]finding, len(pairings))
	for _, f := range findings {
		byPairing[f.pairing] = append(byPairing[f.pairing], f)
	}

	disagreements, unread := 0, 0
	for i := range pairings {
		p := &pairings[i]
		group := byPairing[p]
		fails := false
		for _, f := range group {
			if f.Fails {
				fails = true
				disagreements++
			} else {
				unread++
			}
		}
		switch {
		case fails:
			fmt.Fprint(errOut, block(root, p, group, cfg.verbose))
		case cfg.verbose && len(group) > 0:
			fmt.Fprint(out, block(root, p, group, true))
		case cfg.verbose:
			fmt.Fprintf(out, "    ok  %s\n", heading(root, p))
		}
	}
	for _, trouble := range problems {
		fmt.Fprintf(errOut, "%s (%s): %s\n", trouble.Package, relative(trouble.Position, root), trouble.Message)
	}
	for _, message := range stale {
		fmt.Fprintf(errOut, "sent_declarations.go: %s\n", message)
	}

	switch {
	case len(pairings) == 0:
		fmt.Fprintf(errOut, "\n%s found no send to judge under %s\n", prefix, strings.Join(cfg.patterns, " "))
		return 1
	case disagreements > 0 || len(problems) > 0 || len(stale) > 0:
		fmt.Fprintf(errOut, "\n%s %d disagreement(s) in %d pairing(s), %d unpaired or unjudged, %d stale declaration(s) (%s)\n",
			prefix, disagreements, len(pairings), len(problems), len(stale), provenance)
		return 1
	default:
		fmt.Fprintf(out, "%s %d pairing(s) agree with their documents, %d selection(s) nothing reads (%s)\n",
			prefix, len(pairings), unread, provenance)
		return 0
	}
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
