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

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
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

// auditRun is one configured run: where to look, what to judge by, and how
// much to say about what agreed.
type auditRun struct {
	dir        string
	verbose    bool
	patterns   []string
	schemaPath string
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every pairing judged and every selection nothing reads, not only the disagreements")
	schemaPath := flag.String("schema", "", "SDL file to judge the documents against, instead of the pinned schema")
	flag.Parse()

	os.Exit(run(auditRun{
		dir:        *dir,
		verbose:    *verbose,
		patterns:   auditPatterns,
		schemaPath: *schemaPath,
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

	var findings []finding
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
		findings = append(findings, judgePairing(schema, document, p)...)
	}

	unpaired, err := unpairedDocuments(cfg, judged)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	problems = append(problems, unpaired...)

	return report(cfg, out, errOut, provenance, pairings, findings, problems)
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
// there. Every problem is a stderr line of its own.
func report(cfg auditRun, out, errOut io.Writer, provenance string, pairings []pairing, findings []finding, problems []problem) int {
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

	switch {
	case len(pairings) == 0:
		fmt.Fprintf(errOut, "\n%s found no send to judge under %s\n", prefix, strings.Join(cfg.patterns, " "))
		return 1
	case disagreements > 0 || len(problems) > 0:
		fmt.Fprintf(errOut, "\n%s %d disagreement(s) in %d pairing(s), %d unpaired or unjudged (%s)\n",
			prefix, disagreements, len(pairings), len(problems), provenance)
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
