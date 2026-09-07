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

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// auditPatterns are the packages the audit loads. Every GraphQL document this
// server sends lives under them.
var auditPatterns = []string{"./internal/..."}

// auditRun is one configured run: where to look, what to look at, what to judge
// it against, and how much to say about what passed.
type auditRun struct {
	dir      string
	verbose  bool
	patterns []string
	// schemaPath names an SDL file to judge the documents against instead of
	// the pinned one. It is how the live re-probe works: cmd/gen_graphql_schema
	// writes today's schema into a temporary directory and this reads it, so a
	// field GitLab narrowed since the pin is reported as a failure rather than
	// waiting for the next re-pin. It is also how a document meant for a
	// particular self-managed release can be checked against that release.
	schemaPath string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository.
	overlay map[string][]byte
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every document checked, not only the refused ones")
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
// this audit ends are reachable from a test instead of only from a process. It
// returns the exit status rather than calling os.Exit.
func run(cfg auditRun, out, errOut io.Writer) int {
	validate, source, err := judge(cfg)
	if err != nil {
		fmt.Fprintln(errOut, "audit_graphql_documents:", err)
		return 1
	}

	documents, err := collect(cfg.dir, cfg.patterns, cfg.overlay)
	if err != nil {
		fmt.Fprintln(errOut, "audit_graphql_documents:", err)
		return 1
	}
	if len(documents) == 0 {
		fmt.Fprintln(errOut, "audit_graphql_documents: no GraphQL documents were found, which means this audit is looking at the wrong thing")
		return 1
	}

	// Positions come out of the loader absolute, so the root a finding is
	// trimmed against has to be absolute too, whatever -dir was written as.
	root := cfg.dir
	if absolute, absErr := filepath.Abs(cfg.dir); absErr == nil {
		root = absolute
	}

	refused := 0
	for _, found := range documents {
		validationErr := validate(found.text)
		if validationErr == nil {
			if cfg.verbose {
				fmt.Fprintf(out, "    ok  %s %s\n", found.pkg, found.label())
			}
			continue
		}
		refused++
		fmt.Fprint(errOut, finding(root, found, validationErr))
	}

	if refused > 0 {
		fmt.Fprintf(errOut, "\naudit_graphql_documents: the schema refuses %d of %d document(s) (%s)\n",
			refused, len(documents), source)
		return 1
	}
	fmt.Fprintf(out, "audit_graphql_documents: %d document(s) accepted (%s)\n",
		len(documents), source)
	return 0
}

// judge returns the check each document is put through and the one line that
// says what judged it, so a reader of a failure knows whose opinion refused
// their document: a schema pinned on a recorded day, or one fetched today.
func judge(cfg auditRun) (validate func(string) error, provenance string, err error) {
	if cfg.schemaPath == "" {
		// The provenance record is embedded and its own gate
		// (make check-graphql-schema) refuses a build where it does not decode,
		// so a failure there is not something this command could act on.
		return graphqlschema.ValidateDocument, cmdutil.Must(graphqlschema.SourceInfo()).String(), nil
	}

	sdl, err := os.ReadFile(cfg.schemaPath)
	if err != nil {
		return nil, "", fmt.Errorf("read the schema to judge against: %w", err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", cfg.schemaPath, err)
	}
	return func(document string) error {
			return graphqlschema.ValidateDocumentAgainst(schema, document)
		},
		fmt.Sprintf("%d types from %s, not the pinned schema", len(schema.Types), cfg.schemaPath),
		nil
}

// finding renders one refused document with every reason under it.
func finding(root string, found document, err error) string {
	var report strings.Builder
	fmt.Fprintf(&report, "%s %s (%s)\n", found.pkg, found.label(), relative(found.position, root))

	var refusal *graphqlschema.ValidationError
	if !errors.As(err, &refusal) {
		fmt.Fprintf(&report, "    %s\n", err)
		return report.String()
	}
	for _, reason := range refusal.Reasons {
		fmt.Fprintf(&report, "    - %s\n", reason)
	}
	return report.String()
}

// relative trims a position to the repository root so a finding reads as a
// path a person can open.
func relative(position token.Position, root string) string {
	path := position.Filename
	if root != "" {
		if trimmed, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(trimmed, "..") {
			path = filepath.ToSlash(trimmed)
		}
	}
	return fmt.Sprintf("%s:%d", path, position.Line)
}
