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

// auditRun is one configured run: where to look, what to look at, and how much
// to say about what passed.
type auditRun struct {
	dir      string
	verbose  bool
	patterns []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository.
	overlay map[string][]byte
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every document checked, not only the refused ones")
	flag.Parse()

	os.Exit(run(auditRun{
		dir:      *dir,
		verbose:  *verbose,
		patterns: auditPatterns,
	}, os.Stdout, os.Stderr))
}

// run is main with its streams and its exit status handed to it, so the ways
// this audit ends are reachable from a test instead of only from a process. It
// returns the exit status rather than calling os.Exit.
func run(cfg auditRun, out, errOut io.Writer) int {
	// The provenance record is embedded and its own gate
	// (make check-graphql-schema) refuses a build where it does not decode, so
	// a failure here is not something this command could act on.
	source := cmdutil.Must(graphqlschema.SourceInfo())

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
		validationErr := graphqlschema.ValidateDocument(found.text)
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
		fmt.Fprintf(errOut, "\naudit_graphql_documents: the pinned schema refuses %d of %d document(s) (%s)\n",
			refused, len(documents), source)
		return 1
	}
	fmt.Fprintf(out, "audit_graphql_documents: %d document(s) accepted by the pinned schema (%s)\n",
		len(documents), source)
	return 0
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
