package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// prefix names this command on every line it writes, so a failure in a
// composite make target says which of them produced it.
const prefix = "audit_graphql_documents:"

// auditPatterns are the packages the audit loads. Every GraphQL document this
// server sends lives under them.
var auditPatterns = []string{"./internal/..."}

// auditRun is one configured run: where to look, what to look at, what to judge
// it against, and how much to say about what passed.
type auditRun struct {
	dir      string
	verbose  bool
	patterns []string
	// live names a GraphQL endpoint to introspect right now and judge the
	// documents against, instead of the pinned schema. It is the one check the
	// pin cannot perform: the pin says our documents were valid on gitlab.com
	// on the day it was taken, this says they are valid on the GitLab that
	// shipped today, which is the version a self-managed instance runs.
	live string
	// schemaPath names an SDL file to judge the documents against instead of
	// the pinned one, for a schema already on disk: one captured earlier, or
	// one belonging to the particular self-managed release a document is meant
	// for.
	schemaPath string
	// token is sent to the instance named by live. GitLab answers introspection
	// to anyone, so this only decides whether the report can name the version
	// that answered, which GitLab refuses to tell an anonymous caller. It is
	// resolved by [graphqlintrospect.CredentialFor], so it is empty unless the
	// endpoint is the instance GITLAB_URL names.
	token string
	// tokenWithheld says why a token that exists was not sent, so a report
	// naming an unknown version says which of the two reasons it is: an
	// instance that would not answer, or a credential this run declined to
	// hand it.
	tokenWithheld string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository.
	overlay map[string][]byte
	// now supplies the day the pin's age is measured from, as a parameter so a
	// test can assert on the drift report it produced.
	now func() time.Time
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every document checked, not only the refused ones")
	schemaPath := flag.String("schema", "", "SDL file to judge the documents against, instead of the pinned schema")
	live := flag.String("live", "", "GraphQL endpoint to introspect now and judge the documents against, instead of the pinned schema")
	flag.Parse()

	credential, withheld := graphqlintrospect.CredentialFor(*live, os.Getenv("GITLAB_URL"), os.Getenv("GITLAB_TOKEN"))
	os.Exit(run(auditRun{
		dir:           *dir,
		verbose:       *verbose,
		patterns:      auditPatterns,
		live:          *live,
		schemaPath:    *schemaPath,
		token:         credential,
		tokenWithheld: withheld,
	}, os.Stdout, os.Stderr))
}

// run is main with its streams and its exit status handed to it, so the ways
// this audit ends are reachable from a test instead of only from a process. It
// returns the exit status rather than calling os.Exit.
func run(cfg auditRun, out, errOut io.Writer) int {
	probed, judgedBy, err := resolveSchema(cfg)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	result, err := graphqldocs.Audit(graphqldocs.Options{
		Dir:        cfg.dir,
		Patterns:   cfg.patterns,
		Schema:     probed,
		Provenance: judgedBy,
		Overlay:    cfg.overlay,
	})
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	// Positions come out of the loader absolute, so the root a finding is
	// trimmed against has to be absolute too, whatever -dir was written as.
	root := cfg.dir
	if absolute, absErr := filepath.Abs(cfg.dir); absErr == nil {
		root = absolute
	}

	if cfg.verbose {
		refused := refusedDocuments(result)
		for _, found := range result.Documents {
			if !refused[found.Position] {
				fmt.Fprintf(out, "    ok  %s %s\n", found.Package, found.Label())
			}
		}
	}
	for _, refusal := range result.Refusals {
		fmt.Fprint(errOut, finding(root, refusal))
	}

	// Drift is reported whether or not a document was refused, and it is not
	// itself a failure. A refusal says a document broke; the drift under our
	// selection sets says how far the pin has moved from what an instance
	// serves now, which is the question a reader of that refusal asks next.
	if probed != nil {
		// The pinned schema and its provenance record are embedded, and their
		// own gate (make check-graphql-schema) refuses a build where either
		// does not load, so a failure here is not something this command could
		// act on.
		fmt.Fprint(out, driftReport(
			cmdutil.Must(graphqlschema.Schema()), probed, result.Documents,
			cmdutil.Must(graphqlschema.SourceInfo()), provenance.Clock(cfg.now),
		))
	}

	if len(result.Refusals) > 0 {
		fmt.Fprintf(errOut, "\n%s the schema refuses %d of %d document(s) (%s)\n",
			prefix, len(result.Refusals), len(result.Documents), result.Provenance)
		return 1
	}
	fmt.Fprintf(out, "%s %d document(s) accepted (%s)\n", prefix, len(result.Documents), result.Provenance)
	return 0
}

// resolveSchema decides what this run judges by, and returns nil when that is
// the pin. A non-nil schema is one the caller handed the audit rather than one
// it loaded itself, which is also exactly the condition drift can be reported
// under: there are two schemas to compare only when somebody supplied the
// second.
func resolveSchema(cfg auditRun) (*ast.Schema, string, error) {
	switch {
	case cfg.live != "" && cfg.schemaPath != "":
		return nil, "", errors.New("-live and -schema both name a schema to judge by: pass one")
	case cfg.live != "":
		ctx, cancel := context.WithTimeout(context.Background(), graphqlintrospect.FetchTimeout)
		defer cancel()
		return liveSchema(ctx, cfg.live, cfg.token, cfg.tokenWithheld)
	case cfg.schemaPath != "":
		sdl, err := os.ReadFile(cfg.schemaPath) //#nosec G304 -- the path is the operator's own -schema flag
		if err != nil {
			return nil, "", fmt.Errorf("read the schema to judge against: %w", err)
		}
		schema, err := graphqlschema.Load(sdl)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", cfg.schemaPath, err)
		}
		return schema, fmt.Sprintf("%d types from %s, not the pinned schema", len(schema.Types), cfg.schemaPath), nil
	default:
		return nil, "", nil
	}
}

// refusedDocuments indexes the refusals by position so the verbose listing can
// name what passed without repeating what failed. The position is the key
// because it is what tells two documents apart: an inline one has no name, and
// a package holds several.
func refusedDocuments(result graphqldocs.Result) map[token.Position]bool {
	refused := make(map[token.Position]bool, len(result.Refusals))
	for _, refusal := range result.Refusals {
		refused[refusal.Document.Position] = true
	}
	return refused
}

// finding renders one refused document with every reason under it.
func finding(root string, refusal graphqldocs.Refusal) string {
	var report strings.Builder
	fmt.Fprintf(&report, "%s %s (%s)\n",
		refusal.Document.Package, refusal.Document.Label(), relative(refusal.Document.Position, root))
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
