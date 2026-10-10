// The client-go half of this audit: the documents client-go builds and the
// structs it decodes them into, for the client-go this repository's go.mod
// requires, asked the questions this audit asks of our own: the three
// disagreements between a document and its decoder, and what the schema
// offers that no document selects.

package main

import (
	"fmt"
	"go/types"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/goprogram"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// sdkGrain is the grain a client-go sent finding is read at. Every decoder of
// client-go is declared in one package, so the package grain our own
// documents are read at would make one service's selection cancel another's
// gap; the service is the unit a caller reaches and an upstream change edits.
const sdkGrain = "client-go service and schema type"

// sdkDirectory finds the client-go module the repository in dir builds
// against. A seam, so a test points the run at a stand-in module.
var sdkDirectory = graphqldocs.SDKDirectory

// sdkStatus is everything one client-go run decided, gathered so the renderer
// and the report take one value rather than a row of arguments.
type sdkStatus struct {
	provenance string
	dir        string
	// pairings are the sends this server reaches, each judged.
	pairings []pairing
	// reached names, for every client-go function holding a send this server
	// reaches, the packages of ours that reach it; unreached names the
	// functions holding a send nothing here calls, which are not judged.
	reached   map[string][]string
	unreached []string
	coverage  sentCoverage
	// failures are the shape disagreements of the reached sends, each with the
	// declaration that answers it or none.
	failures []sdkShapeFinding
	// unjudged are the reached sends this audit could not pair, with the
	// declaration that answers each or none.
	unjudged []sdkUnjudgedSend
	// unpaired are the documents client-go writes that no send carries.
	unpaired []problem
	// sent are the fields the schema offers at an object a reached decoder
	// reads and no document of its service selects, classified.
	sent []sentField
	// stale names every declaration of the three tables that answered nothing.
	stale []string
}

// sdkShapeFinding is a disagreement between a client-go document and its
// decoder, at a send this server reaches.
type sdkShapeFinding struct {
	Method   string `json:"method"`
	Position string `json:"position"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration answers the disagreement.
func (f sdkShapeFinding) declared() bool { return f.Category != "" }

// sdkUnjudgedSend is a send this server reaches whose document this audit
// could not pair with its decoder, so neither is judged.
type sdkUnjudgedSend struct {
	Method   string `json:"method"`
	Position string `json:"position"`
	Message  string `json:"message"`
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration answers the send.
func (s sdkUnjudgedSend) declared() bool { return s.Category != "" }

// runSDK is [run] for the client-go half: it loads the client-go go.mod
// requires and this repository's own packages, decides which client-go sends
// this server reaches, judges each of those against its decoder and asks the
// schema what it offers that no document of the send's service selects, and
// holds every answer to the three declaration tables.
//
// Unlike the run over our own documents, a sent finding nothing declares
// fails here. The surface is client-go's and small, an answer is a one-line
// declaration naming the upstream-bugs row that records the gap, and the table
// is what makes the one-time triage persist: a client-go bump that selects a
// declared field leaves its declaration stale, and one that drops a selection,
// or a re-pin that adds a field, leaves a finding nothing answers, and either
// fails the run that brings it.
func runSDK(cfg auditRun, out, errOut io.Writer) int {
	status, err := judgeSDKRun(cfg)
	if err != nil {
		fmt.Fprintln(errOut, prefix, err)
		return 1
	}
	if cfg.reportPath != "" {
		if written := writeJSON(cfg.reportPath, newSDKReport(status), "the client-go sent report"); written != nil {
			fmt.Fprintln(errOut, prefix, written)
			return 1
		}
		fmt.Fprintf(out, "%s client-go: %d field(s) the schema offers that no document of their service selects -> %s\n", prefix, len(status.sent), cfg.reportPath)
	}
	return reportSDK(cfg, out, errOut, status)
}

// judgeSDKRun loads both programs and decides everything one client-go run
// reports, or says why it could not start.
func judgeSDKRun(cfg auditRun) (sdkStatus, error) {
	schema, provenance, err := resolveSchema(cfg)
	if err != nil {
		return sdkStatus{}, err
	}
	dir, err := sdkDirectory(cfg.dir)
	if err != nil {
		return sdkStatus{}, err
	}
	sdk, err := loadSDKProgram(dir)
	if err != nil {
		return sdkStatus{}, err
	}
	ours, err := loadProgram(cfg.dir, cfg.patterns)
	if err != nil {
		return sdkStatus{}, err
	}
	status := sdkStatus{provenance: provenance, dir: dir, reached: reachedFunctions(ours.pkgs, sdk.pkgs)}
	pairings, problems := sdk.pairings()
	status.unpaired = sdk.unpairedDocuments(pairings)
	status.judge(schema, cfg.sdkDeclarations, pairings, problems)
	return status, nil
}

// loadSDKProgram loads the client-go module in dir, under the environment a
// load of a module this process does not own has to run in.
func loadSDKProgram(dir string) (*program, error) {
	loaded, err := goprogram.LoadWith(dir, graphqldocs.SDKPatterns(), goprogram.Options{Env: graphqldocs.ForeignModuleEnv()})
	if err != nil {
		return nil, fmt.Errorf("load the client-go source in %s: %w", dir, err)
	}
	return newProgram(loaded), nil
}

// judge judges every send this server reaches and classifies what it found
// against the tables.
func (s *sdkStatus) judge(schema *ast.Schema, tables sdkTables, pairings []pairing, problems []problem) {
	var offered []sentField
	selected := map[string]bool{}
	var failures []sdkShapeFinding
	for i := range pairings {
		p := &pairings[i]
		if !s.reaches(p.Function) {
			s.unreached = append(s.unreached, shortName(p.Function))
			continue
		}
		p.Unit, p.ReadThrough = serviceOf(p.Function), true
		document, err := graphqlschema.ParseAgainst(schema, p.Text)
		if err != nil {
			problems = append(problems, problem{
				Package:  p.Package,
				Position: p.Position,
				Message:  p.Label() + " is a document the schema refuses, so it has no selection set to judge its decoder by",
				Function: p.Function,
			})
			continue
		}
		s.pairings = append(s.pairings, *p)
		judgement := judgePairing(schema, document, p)
		offered = append(offered, judgement.sent...)
		s.coverage.add(judgement.coverage)
		for key := range judgement.selected {
			selected[key] = true
		}
		for _, f := range judgement.findings {
			if f.Fails {
				failures = append(failures, sdkShapeFinding{Method: shortName(p.Function), Position: sendWhere(s.dir, p.Position, p.Origin), Path: f.Path, Message: f.Message})
			}
		}
	}
	var unjudged []sdkUnjudgedSend
	for _, trouble := range problems {
		if !s.reaches(trouble.Function) {
			s.unreached = append(s.unreached, shortName(trouble.Function))
			continue
		}
		unjudged = append(unjudged, sdkUnjudgedSend{Method: shortName(trouble.Function), Position: relative(trouble.Position, s.dir), Message: trouble.Message})
	}
	slices.Sort(s.unreached)
	s.unreached = slices.Compact(s.unreached)

	found := dedupeSent(offered, publishedIndex{}, selected, s.dir)
	var unusedSent []string
	s.sent, unusedSent = classifySent(tables.sent, found)
	var unusedShapes, unusedSends []string
	s.failures, unusedShapes = classifyShapes(tables.shapes, failures)
	s.unjudged, unusedSends = classifySends(tables.sends, unjudged)
	s.stale = slices.Concat(inFile("sdk_declarations.go", unusedShapes), inFile("sdk_declarations.go", unusedSends), inFile("sdk_declarations_sent.go", unusedSent))
}

// inFile names the file each stale declaration is kept in, so the line that
// reports it sends the reader to the table that holds it: the disagreements
// and the sends are answered in sdk_declarations.go and the sent fields in
// sdk_declarations_sent.go.
func inFile(file string, keys []string) []string {
	named := make([]string, len(keys))
	for i, key := range keys {
		named[i] = file + ": " + key
	}
	return named
}

// reaches reports whether a call in this repository can run fn.
func (s *sdkStatus) reaches(fn *types.Func) bool {
	_, found := s.reached[fn.Origin().FullName()]
	return found
}

// unpairedDocuments names every document the inventory reads out of a
// program that no pairing carries.
//
// A shell the inventory could not render is skipped: no pairing can carry it,
// since its text is what a caller completes. Nothing ties the shell to a send
// here; the send that builds its document, when the walk finds one, is
// reported as unjudged for whatever reason the walk could not follow it,
// which is where it is answered.
func (p *program) unpairedDocuments(pairings []pairing) []problem {
	carried := make(map[string]bool, len(pairings))
	for _, paired := range pairings {
		carried[paired.Text] = true
	}
	var unpaired []problem
	for _, document := range p.documents {
		if carried[document.Text] || document.Assembly != nil && document.Assembly.Unrendered != "" {
			continue
		}
		unpaired = append(unpaired, problem{
			Package:  document.Package,
			Position: document.Position,
			Message:  document.Label() + " is a document no send this audit can see carries, so its decoder is judged by nobody",
		})
	}
	return unpaired
}

// serviceOf names the client-go type a function is a method of, qualified by
// its package, which is the unit a field the documents of that type leave out
// is filed under. A function that is not a method is filed under its package.
//
// A method's receiver is a named type or a pointer to one, which is why the
// assertion is not checked.
func serviceOf(fn *types.Func) string {
	recv := fn.Signature().Recv()
	if recv == nil {
		return fn.Pkg().Path()
	}
	named, _ := pointee(recv.Type()).(*types.Named)
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

// shortName names a function the way a declaration does: its receiver's type
// and its own name for a method, "WorkItemsService.GetWorkItem", and its own
// name for a function.
func shortName(fn *types.Func) string {
	service := serviceOf(fn)
	if service == fn.Pkg().Path() {
		return fn.Name()
	}
	return service[strings.LastIndex(service, ".")+1:] + "." + fn.Name()
}

// reportSDK renders a client-go run and returns its exit status.
//
// Every disagreement, unjudged send and sent field nothing declares is a
// stderr line, as is every document no send carries and every declaration
// that answers nothing; -v lists what the declarations answer and the sends
// this server never reaches beside them.
func reportSDK(cfg auditRun, out, errOut io.Writer, status sdkStatus) int {
	failing := 0
	for _, f := range status.failures {
		line := fmt.Sprintf("%s (%s): %s: %s", f.Method, f.Position, f.Path, f.Message)
		failing += reportDeclared(out, errOut, cfg.verbose, line, f.declared(), f.Reason)
	}
	for _, u := range status.unjudged {
		line := fmt.Sprintf("%s (%s): %s", u.Method, u.Position, u.Message)
		failing += reportDeclared(out, errOut, cfg.verbose, line, u.declared(), u.Reason)
	}
	for _, field := range status.sent {
		line := fmt.Sprintf("%s %s.%s (%s): offered at %s of %s (%s) and selected by no document of the service",
			field.Package[strings.LastIndex(field.Package, "/")+1:], field.SchemaType, field.Field, field.FieldType, field.Path, field.Document,
			sendWhere(status.dir, field.position, field.origin))
		failing += reportDeclared(out, errOut, cfg.verbose, line, field.declared(), field.Reason)
	}
	for _, trouble := range status.unpaired {
		fmt.Fprintf(errOut, "%s (%s): %s\n", trouble.Package, relative(trouble.Position, status.dir), trouble.Message)
	}
	for _, key := range status.stale {
		fmt.Fprintf(errOut, "%s is declared and answered nothing: the finding it answered is gone, so the declaration has to go with it\n", key)
	}
	if cfg.verbose {
		for _, method := range status.unreached {
			fmt.Fprintf(out, "    not reached  %s\n", method)
		}
	}

	scope := fmt.Sprintf("client-go at %s, %d pairing(s) judged at the sends this server reaches, %d function(s) holding a send it never calls", filepath.ToSlash(status.dir), len(status.pairings), len(status.unreached))
	broken := failing + len(status.unpaired) + len(status.stale)
	if broken > 0 {
		fmt.Fprintf(errOut, "\n%s %s: %d finding(s) nothing declares, %d document(s) no send carries, %d stale declaration(s) (%s)\n",
			prefix, scope, failing, len(status.unpaired), len(status.stale), status.provenance)
		return 1
	}
	fmt.Fprintf(out, "%s %s: %d disagreement(s), %d unjudged send(s) and %d sent field(s), every one declared (%s)\n",
		prefix, scope, len(status.failures), len(status.unjudged), len(status.sent), status.provenance)
	return 0
}

// reportDeclared writes one finding where it belongs: stderr when nothing
// declares it, stdout under -v with the declaration's reason when something
// does, and nowhere otherwise. It returns 1 for a finding that fails the run.
func reportDeclared(out, errOut io.Writer, verbose bool, line string, declared bool, reason string) int {
	if !declared {
		fmt.Fprintln(errOut, line)
		return 1
	}
	if verbose {
		fmt.Fprintf(out, "    declared  %s\n              %s\n", line, reason)
	}
	return 0
}
