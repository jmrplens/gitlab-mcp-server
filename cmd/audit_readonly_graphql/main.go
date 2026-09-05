package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// auditPatterns are the packages the audit loads. Every catalog handler and
// every GraphQL document in this project lives under them.
var auditPatterns = []string{"./internal/..."}

// actionSource supplies the catalog actions to audit. It is a parameter so the
// exit paths and the classification are reachable from a test without the
// compiled-in catalog deciding what the test gets to see.
type actionSource func() ([]action, error)

// auditRun is one configured run: where to look, what to look at, and where
// the actions being audited come from.
type auditRun struct {
	dir      string
	verbose  bool
	patterns []string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository.
	overlay map[string][]byte
	actions actionSource
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "report what was checked, not only what failed")
	flag.Parse()

	os.Exit(run(auditRun{
		dir:      *dir,
		verbose:  *verbose,
		patterns: auditPatterns,
		actions:  catalogActions,
	}, os.Stdout, os.Stderr))
}

// run is main with its streams, its catalog, and its exit status handed to it,
// so the ways this audit ends are reachable from a test instead of only from a
// process. It returns the exit status rather than calling os.Exit.
func run(cfg auditRun, out, errOut io.Writer) int {
	actions, err := cfg.actions()
	if err != nil {
		fmt.Fprintln(errOut, "audit_readonly_graphql:", err)
		return 1
	}
	if len(actions) == 0 {
		fmt.Fprintln(errOut, "audit_readonly_graphql: the catalog is empty, which means this audit is looking at the wrong thing")
		return 1
	}

	prog, err := loadProgram(cfg.dir, cfg.patterns, cfg.overlay)
	if err != nil {
		fmt.Fprintln(errOut, "audit_readonly_graphql:", err)
		return 1
	}

	result := audit(prog, actions, cfg.dir)
	if cfg.verbose {
		reportChecked(out, result)
	}
	if len(result.findings) > 0 {
		for _, item := range result.findings {
			fmt.Fprintln(errOut, item.message)
		}
		fmt.Fprintf(errOut, "\naudit_readonly_graphql: %d problem(s) across %d read-only action(s)\n", len(result.findings), result.checked)
		return 1
	}
	fmt.Fprintf(out, "audit_readonly_graphql: %d read-only actions reach no GraphQL mutation (%d of them send GraphQL)\n",
		result.checked, len(result.graphQL))
	return 0
}

// reportChecked lists what the run resolved, so the set of read-only actions
// that touch GraphQL at all is reviewable rather than a number.
func reportChecked(out io.Writer, result auditResult) {
	fmt.Fprintf(out, "audit_readonly_graphql: %d declared exception(s) in use\n", result.exceptions)
	fmt.Fprintf(out, "audit_readonly_graphql: %d read-only action(s) send GraphQL:\n", len(result.graphQL))
	for _, id := range result.graphQL {
		fmt.Fprintf(out, "    %s\n", id)
	}
}
