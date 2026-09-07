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

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/graphqlintrospect"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
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
	// that answered, which GitLab refuses to tell an anonymous caller.
	token string
	// overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository.
	overlay map[string][]byte
	// now supplies the day the pin's age is measured from, as a parameter so a
	// test can assert on the drift report it produced.
	now func() time.Time
}

// clock is now with a default, so a run that judges by the pin does not have to
// supply one.
func (c auditRun) clock() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func main() {
	dir := flag.String("dir", ".", "repository root to audit")
	verbose := flag.Bool("v", false, "list every document checked, not only the refused ones")
	schemaPath := flag.String("schema", "", "SDL file to judge the documents against, instead of the pinned schema")
	live := flag.String("live", "", "GraphQL endpoint to introspect now and judge the documents against, instead of the pinned schema")
	flag.Parse()

	os.Exit(run(auditRun{
		dir:        *dir,
		verbose:    *verbose,
		patterns:   auditPatterns,
		live:       *live,
		schemaPath: *schemaPath,
		token:      os.Getenv("GITLAB_TOKEN"),
	}, os.Stdout, os.Stderr))
}

// run is main with its streams and its exit status handed to it, so the ways
// this audit ends are reachable from a test instead of only from a process. It
// returns the exit status rather than calling os.Exit.
func run(cfg auditRun, out, errOut io.Writer) int {
	judged, err := judge(cfg)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}

	documents, err := collect(cfg.dir, cfg.patterns, cfg.overlay)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	if len(documents) == 0 {
		fmt.Fprintln(errOut, prefix, "no GraphQL documents were found, which means this audit is looking at the wrong thing")
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
		validationErr := judged.validate(found.text)
		if validationErr == nil {
			if cfg.verbose {
				fmt.Fprintf(out, "    ok  %s %s\n", found.pkg, found.label())
			}
			continue
		}
		refused++
		fmt.Fprint(errOut, finding(root, found, validationErr))
	}

	// Drift is reported whether or not a document was refused, and it is not
	// itself a failure. A refusal says a document broke; the drift under our
	// selection sets says how far the pin has moved from what an instance
	// serves now, which is the question a reader of that refusal asks next.
	if judged.probed != nil {
		// The pinned schema and its provenance record are embedded, and their
		// own gate (make check-graphql-schema) refuses a build where either
		// does not load, so a failure here is not something this command could
		// act on.
		fmt.Fprint(out, driftReport(
			cmdutil.Must(graphqlschema.Schema()), judged.probed, documents,
			cmdutil.Must(graphqlschema.SourceInfo()), cfg.clock(),
		))
	}

	if refused > 0 {
		fmt.Fprintf(errOut, "\n%s the schema refuses %d of %d document(s) (%s)\n",
			prefix, refused, len(documents), judged.source)
		return 1
	}
	fmt.Fprintf(out, "%s %d document(s) accepted (%s)\n", prefix, len(documents), judged.source)
	return 0
}

// judgement is what one run judges its documents by.
type judgement struct {
	// validate is the check each document is put through.
	validate func(string) error
	// source is the one line that says whose opinion refused a document: a
	// schema pinned on a recorded day, one fetched today, or one from a file.
	source string
	// probed is the schema this run was handed, and nil when the pin judged.
	// Drift is only a question when there are two schemas to compare.
	probed *ast.Schema
}

// judge resolves the schema this run judges by.
func judge(cfg auditRun) (judgement, error) {
	switch {
	case cfg.live != "" && cfg.schemaPath != "":
		return judgement{}, errors.New("-live and -schema both name a schema to judge by: pass one")
	case cfg.live != "":
		return judgeLive(cfg)
	case cfg.schemaPath != "":
		return judgeFile(cfg.schemaPath)
	default:
		// The provenance record is embedded and its own gate
		// (make check-graphql-schema) refuses a build where it does not decode,
		// so a failure there is not something this command could act on.
		return judgement{
			validate: graphqlschema.ValidateDocument,
			source:   cmdutil.Must(graphqlschema.SourceInfo()).String(),
		}, nil
	}
}

// judgeLive fetches the schema an instance serves right now.
func judgeLive(cfg auditRun) (judgement, error) {
	ctx, cancel := context.WithTimeout(context.Background(), graphqlintrospect.FetchTimeout)
	defer cancel()

	schema, provenance, err := liveSchema(ctx, cfg.live, cfg.token)
	if err != nil {
		return judgement{}, err
	}
	return against(schema, provenance), nil
}

// judgeFile loads a schema from an SDL file the caller supplies.
func judgeFile(path string) (judgement, error) {
	sdl, err := os.ReadFile(path)
	if err != nil {
		return judgement{}, fmt.Errorf("read the schema to judge against: %w", err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return judgement{}, fmt.Errorf("%s: %w", path, err)
	}
	return against(schema, fmt.Sprintf("%d types from %s, not the pinned schema", len(schema.Types), path)), nil
}

// against builds the judgement for a schema that is not the pinned one.
func against(schema *ast.Schema, provenance string) judgement {
	return judgement{
		validate: func(document string) error { return graphqlschema.ValidateDocumentAgainst(schema, document) },
		source:   provenance,
		probed:   schema,
	}
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
