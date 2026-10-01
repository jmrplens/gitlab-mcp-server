package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

const (
	// toolName names the flag set, so a usage message names the command.
	toolName = "gen_action_grants"

	// regenerate is the command a stale artifact is refreshed with.
	regenerate = "make gen-action-grants"
)

// tableStub is what the program loader reads in place of the generated
// table, for the reason the stub's own comment gives.
//
//go:embed table_stub.go.txt
var tableStub []byte

// loadPatterns are the packages the derivation reads: every handler, the
// catalog's own package included.
var loadPatterns = []string{"./internal/..."}

// osExit is os.Exit behind a variable, so the line main carries is reachable
// from a test.
var osExit = os.Exit

// main derives what every action sends, joins it to what GitLab declares, and
// writes or checks the three artifacts.
func main() {
	osExit(runMain(os.Args[1:], os.Stderr))
}

// runMain parses args and returns the exit code. A bad flag is 2; a run that
// failed, a stale artifact, or a derivation finding is 1.
func runMain(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "verify the three generated artifacts are current without writing them")
	checkDerivation := fs.Bool("check-derivation", false,
		"derive every action and fail on any finding: an action not derived or declared, a route or element the record does not place, "+
			"a stale declaration or directive, and the three gates; writes nothing")
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
	if runErr := run(stderr, root, options{check: *check, checkDerivation: *checkDerivation}, newSources()); runErr != nil {
		fmt.Fprintf(stderr, "%v\n", runErr)
		return 1
	}
	return 0
}

// options carries what the flags decided.
type options struct {
	check           bool
	checkDerivation bool
}

// sources are the inputs, behind functions so a test can hand in a small
// program and a small record instead of the tree's.
type sources struct {
	catalog func() ([]actionrequests.Action, error)
	load    func(root string, overlay map[string][]byte) (*actionrequests.Program, error)
	sdk     func(prog *actionrequests.Program) (derive.SDK, error)
	record  func(root string) (*apilive.Document, error)
	schema  func() (*gqlast.Schema, error)
	// requests and grants are the declaration tables the derivation and the
	// join take.
	requests []derive.Declaration
	grants   join.Declarations
}

// newSources returns the inputs runMain reads, behind a variable so a test
// can run the flags against a small program.
var newSources = liveSources

// liveSources reads the tree, client-go's module and the committed record,
// with this command's declaration tables.
func liveSources() sources {
	return sources{
		catalog: actionrequests.Catalog,
		load: func(root string, overlay map[string][]byte) (*actionrequests.Program, error) {
			return actionrequests.Load(root, loadPatterns, overlay)
		},
		sdk:      readSDK,
		record:   readRecord,
		schema:   graphqlschema.Schema,
		requests: requestDeclarations,
		grants:   grantDeclarations,
	}
}

// readSDK reads client-go's routes and documents from the module the loaded
// program compiles against.
func readSDK(prog *actionrequests.Program) (derive.SDK, error) {
	dir := clientGoDir(prog.Packages())
	documents, err := graphqldocs.SDKDocuments(dir)
	if err != nil {
		return nil, fmt.Errorf("read client-go documents: %w", err)
	}
	return &sdkSource{sdk: sdkroutes.Read(dir), documents: documents}, nil
}

// readRecord reads the committed live record.
func readRecord(root string) (*apilive.Document, error) {
	record, err := apilive.Read(filepath.Join(root, apilive.DefaultDir))
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// outcome is one run's derivation and join.
type outcome struct {
	derived  derive.Result
	joined   join.Result
	findings []string
}

// run derives, joins and gates, then writes or checks the artifacts. Every
// error names the stage that failed.
func run(progress io.Writer, root string, opts options, in sources) error {
	result, err := deriveAndJoin(root, in)
	if err != nil {
		return err
	}
	summarize(progress, &result)
	for _, finding := range result.findings {
		fmt.Fprintln(progress, finding)
	}
	if len(result.findings) > 0 {
		// A table joined from an incomplete derivation answers wrongly for the
		// actions the findings name, so nothing is written or compared until
		// each one is derived or declared.
		return fmt.Errorf("the derivation has %d finding(s); derive or declare each one before the artifacts can be written or compared",
			len(result.findings))
	}
	if opts.checkDerivation {
		return nil
	}
	table := &result.joined.Table
	artifacts := []struct {
		path    string
		content []byte
	}{
		{requestsPath, renderRequests(result.joined.Actions)},
		{tablePath, renderTable(table)},
		{referencePath, renderReference(table)},
	}
	var errs []error
	for _, artifact := range artifacts {
		errs = append(errs, docgen.WriteOrCheck(filepath.Join(root, artifact.path), artifact.content, opts.check, regenerate))
	}
	return errors.Join(errs...)
}

// deriveAndJoin runs the derivation over the program with the generated table
// replaced by its stand-in, joins it to the record, and collects every
// finding the derivation, the join and the gates report.
func deriveAndJoin(root string, in sources) (outcome, error) {
	actions, err := in.catalog()
	if err != nil {
		return outcome{}, fmt.Errorf("build the catalog: %w", err)
	}
	prog, err := in.load(root, map[string][]byte{filepath.Join(root, filepath.FromSlash(tablePath)): tableStub})
	if err != nil {
		return outcome{}, fmt.Errorf("load the program: %w", err)
	}
	sdk, err := in.sdk(prog)
	if err != nil {
		return outcome{}, err
	}
	record, err := in.record(root)
	if err != nil {
		return outcome{}, fmt.Errorf("read the live record: %w", err)
	}
	schema, err := in.schema()
	if err != nil {
		return outcome{}, fmt.Errorf("load the pinned GraphQL schema: %w", err)
	}
	derived := derive.Derive(prog, actions, sdk, in.requests)
	joined := join.Join(record, schema, derived.Actions, in.grants)
	// The join carries every action's own derivation findings with its own,
	// since an action it cannot place whole is one it gives no row.
	findings := append(append([]string{}, derived.Findings...), joined.Findings...)
	findings = append(findings, gateFindings(derived.Actions, joined.Actions, record)...)
	return outcome{derived: derived, joined: joined, findings: findings}, nil
}

// summarize prints the figures a reader checks a run by.
func summarize(progress io.Writer, result *outcome) {
	table := &result.joined.Table
	denied, degraded := 0, 0
	for i := range table.Actions {
		if table.Actions[i].Denied != nil {
			denied++
		}
		if len(table.Actions[i].Degraded) > 0 {
			degraded++
		}
	}
	fmt.Fprintf(progress, "%d actions derived, %d rows at GitLab %s: %d denied to every fine-grained token, %d served with parts always empty; "+
		"%d operations, %d groups, %d GraphQL elements, %d element signatures read from the pinned schema\n",
		len(result.derived.Actions), len(table.Actions), table.Version, denied, degraded,
		len(table.Operations), len(table.Groups), len(table.Elements), result.joined.Fallbacks)
}
