package main

import (
	// embed is imported for its go:embed directive alone, which reads the
	// table stub into tableStub.
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sdkroutes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
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
			"a stale declaration or directive, and the five gates; writes nothing")
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
	sdk     func(prog *actionrequests.Program) (derive.Requester, error)
	record  func(root string) (*apilive.Document, error)
	schema  func() (*gqlast.Schema, error)
	// requests and grants are the declaration tables the derivation and the
	// join take, and variations and disagreements the ones gates 4 and 5 do.
	requests      []derive.Declaration
	grants        join.Declarations
	variations    []classicDeclaration
	disagreements []classicDeclaration
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
		sdk:           readSDK,
		record:        readRecord,
		schema:        graphqlschema.Schema,
		requests:      requestDeclarations,
		grants:        withClassic(grantDeclarations, classicRoutes),
		variations:    classicVariations,
		disagreements: annotationDisagreements,
	}
}

// withClassic is a join's declarations with the classic routes beside them,
// the two tables kept apart in the source because they answer different
// questions about the same requests.
func withClassic(grants join.Declarations, classic []join.ClassicDeclaration) join.Declarations {
	grants.Classic = classic
	return grants
}

// readSDK reads client-go's routes and documents from the module the loaded
// program compiles against.
func readSDK(prog *actionrequests.Program) (derive.Requester, error) {
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

// outcome is one run's derivation and join, with the catalog it was derived
// for.
type outcome struct {
	catalog  []actionrequests.Action
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
	type artifact struct {
		path    string
		content []byte
	}
	artifacts := []artifact{
		{requestsPath, renderRequests(result.joined.Actions, groupScopes(result.catalog))},
		{tablePath, renderTable(table)},
	}
	// The reference is the third artifact, written once per language of the
	// site.
	for _, language := range referenceLanguages() {
		artifacts = append(artifacts, artifact{language.path, language.render(table)})
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
	findings = append(findings, classicFindings(joined.Actions, actions, in.variations, in.disagreements)...)
	return outcome{catalog: actions, derived: derived, joined: joined, findings: findings}, nil
}

// summarize prints the figures a reader checks a run by: the fine-grained
// counts, then how many rows need each classic scope and the actions whose
// read or write classification departs from what read_api reaches.
func summarize(progress io.Writer, result *outcome) {
	table := &result.joined.Table
	denied, degraded := 0, 0
	classic := map[finegrained.ClassicScope]int{}
	for i := range table.Actions {
		if table.Actions[i].Denied != nil {
			denied++
		}
		if len(table.Actions[i].Degraded) > 0 {
			degraded++
		}
		classic[table.Actions[i].Classic]++
	}
	fmt.Fprintf(progress, "%d actions derived, %d rows at GitLab %s: %d denied to every fine-grained token, %d served with parts always empty; "+
		"%d operations, %d groups, %d GraphQL elements, %d element signatures read from the pinned schema\n",
		len(result.derived.Actions), len(table.Actions), table.Version, denied, degraded,
		len(table.Operations), len(table.Groups), len(table.Elements), result.joined.Fallbacks)
	fmt.Fprintf(progress, "classic scope: %d api, %d read_api, %d other-credential, %d no-request; read_api reaches %d actions, "+
		"and these depart from their read-only classification: %s\n",
		classic[finegrained.ClassicAPI], classic[finegrained.ClassicReadAPI], classic[finegrained.ClassicOtherCredential],
		classic[finegrained.ClassicNoRequest], len(table.Actions)-classic[finegrained.ClassicAPI]-classic[finegrained.ClassicUnknown],
		movement(table, result.catalog))
}

// movement lists the actions a read_api token reaches that the catalog
// classifies as writes, and the reads it does not reach, "none" when there
// are none.
func movement(table *finegrained.Table, catalog []actionrequests.Action) string {
	readOnly := make(map[string]bool, len(catalog))
	for _, action := range catalog {
		readOnly[action.ID] = action.ReadOnly
	}
	var moved []string
	for i := range table.Actions {
		row := &table.Actions[i]
		if reach := row.Classic.ReachableWith(finegrained.ClassicReadAPI); reach != readOnly[row.ID] {
			verb := "withheld"
			if reach {
				verb = "served"
			}
			moved = append(moved, row.ID+" "+verb)
		}
	}
	if len(moved) == 0 {
		return "none"
	}
	return strings.Join(moved, ", ")
}

// groupScopes reads, per action, the scopes its catalog group demands
// besides the classic one, which the server's scope filter decides per group
// (tools.MetaToolScopes).
func groupScopes(catalog []actionrequests.Action) map[string][]string {
	scopes := make(map[string][]string, len(catalog))
	for _, action := range catalog {
		if required := tools.MetaToolScopes[action.Group]; len(required) > 0 {
			scopes[action.ID] = required
		}
	}
	return scopes
}
