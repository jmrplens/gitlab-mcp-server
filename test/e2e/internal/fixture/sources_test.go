// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// sources_test.go holds the parser-only gate over the suite's commits of a
// pipeline configuration: every one carries SkipCIMarker in its message, or
// says why it does not in a directive on the line above it.
//
// It reads the e2e tree's files and never type-checks them, like the gates in
// the harness's sources_test.go, so it runs on every push with no GitLab. It
// judges a commit in each shape the tree makes one: a CommitFile call, the
// commit options a builder hands client-go itself, and a write through the
// repository files API. What it reads as the configuration is CIFilePath, by
// any spelling of its import, and the literal path it names; a commit that
// writes the file under a path held in a variable is beyond what a parser can
// follow, and so is one made through a tool call. The message is read as it is
// written at the commit, and a constant it names is not followed to its
// declaration, so a commit whose message is one is held by its own test, as
// the World's is. Every commit of the file is
// judged, on a default branch or not, because a branch held in a variable is
// as far beyond it, and a commit to another branch that a pipeline of the
// suite's own follows wants the marker for the same reason.

package fixture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/sourcewalk"
)

// e2eTree is the tree the gate reads, relative to this package's directory:
// the runtime packages, the libraries and the model evaluation.
var e2eTree = filepath.Join("..", "..")

// fixtureImportPath is the package whose CommitFile, CIFilePath and
// SkipCIMarker the gate looks for in the files that import it.
const fixtureImportPath = "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"

// fixturePackage is this package's name, the one package where the three are
// spelled without a qualifier.
const fixturePackage = "fixture"

// pushPipelineDirective excuses the commit on the line directly below it from
// the marker. It carries the reason, so a reader of the gate's failure has it,
// beside the prose that says why at more length; a directive that excuses no
// commit, or one that gives no reason, is a finding of its own.
const pushPipelineDirective = "//gitlab:allow-push-pipeline"

// The names the gate looks for, and the arguments a CommitFile call and a
// repository files call carry the path and the message in.
const (
	commitFileName      = "CommitFile"
	ciFilePathName      = "CIFilePath"
	skipCIMarkerName    = "SkipCIMarker"
	commitFilePathArg   = 3
	commitFileMessage   = 5
	commitFileArgs      = 6
	filesAPIPathArg     = 1
	filesAPIOptionsArg  = 2
	commitMessageKey    = "CommitMessage"
	commitActionsKey    = "Actions"
	commitActionPathKey = "FilePath"
)

// filesAPIWrites are the repository files calls that commit a file.
var filesAPIWrites = []string{"CreateFile", "UpdateFile"}

// ciCommit is one commit of the pipeline configuration the gate found.
type ciCommit struct {
	pos    token.Position
	marked bool
}

// pushPipelineExcuse is one directive the gate found.
type pushPipelineExcuse struct {
	pos    token.Position
	reason string
	used   bool
}

// sourceFile is one parsed file with the names the gate reads it by: the
// identifiers the fixture package's import goes by, and whether the file is
// the fixture package itself.
type sourceFile struct {
	fset        *token.FileSet
	file        *ast.File
	imports     []string
	unqualified bool
}

// refersTo reports whether expr names the fixture package's identifier
// anywhere within it, qualified by the file's import or, in the package
// itself, bare.
func (f sourceFile) refersTo(expr ast.Expr, name string) bool {
	if expr == nil {
		return false
	}
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			if receiver, isIdent := n.X.(*ast.Ident); isIdent && n.Sel.Name == name && slices.Contains(f.imports, receiver.Name) {
				found = true
			}
		case *ast.Ident:
			if f.unqualified && n.Name == name {
				found = true
			}
		}
		return !found
	})
	return found
}

// isCIFilePath reports whether expr names the configuration's path, as the
// constant or as the literal path it holds.
func (f sourceFile) isCIFilePath(expr ast.Expr) bool {
	if literal, isLiteral := expr.(*ast.BasicLit); isLiteral && literal.Kind == token.STRING {
		value, err := strconv.Unquote(literal.Value)
		return err == nil && value == CIFilePath
	}
	return f.refersTo(expr, ciFilePathName)
}

// isCommitFileCall reports whether the call is the fixture package's
// CommitFile.
func (f sourceFile) isCommitFileCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return f.unqualified && fun.Name == commitFileName
	case *ast.SelectorExpr:
		receiver, isIdent := fun.X.(*ast.Ident)
		return isIdent && fun.Sel.Name == commitFileName && slices.Contains(f.imports, receiver.Name)
	default:
		return false
	}
}

// keyedValue returns the value a composite literal gives the field, or nil.
func keyedValue(literal *ast.CompositeLit, field string) ast.Expr {
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		if key, isIdent := pair.Key.(*ast.Ident); isIdent && key.Name == field {
			return pair.Value
		}
	}
	return nil
}

// writesCIFile reports whether a commit's actions write the configuration:
// some literal among them names its path as the file it acts on.
func (f sourceFile) writesCIFile(actions ast.Expr) bool {
	writes := false
	ast.Inspect(actions, func(node ast.Node) bool {
		if action, isLiteral := node.(*ast.CompositeLit); isLiteral && f.refersToCIFilePath(keyedValue(action, commitActionPathKey)) {
			writes = true
		}
		return !writes
	})
	return writes
}

// refersToCIFilePath reports whether the path expression names the
// configuration anywhere within it, which is how an option literal spells a
// path: wrapped in new or a pointer helper.
func (f sourceFile) refersToCIFilePath(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if inner, isExpr := node.(ast.Expr); isExpr && f.isCIFilePath(inner) {
			found = true
		}
		return !found
	})
	return found
}

// optionsLiteral unwraps the composite literal an options argument is,
// through the address taken of it, or returns nil.
func optionsLiteral(expr ast.Expr) *ast.CompositeLit {
	if unary, isUnary := expr.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
		expr = unary.X
	}
	literal, _ := expr.(*ast.CompositeLit)
	return literal
}

// ciCommits returns every commit of the configuration in the file, in source
// order, each with whether its message names the marker.
func (f sourceFile) ciCommits() []ciCommit {
	var commits []ciCommit
	add := func(node ast.Node, message ast.Expr) {
		commits = append(commits, ciCommit{pos: f.fset.Position(node.Pos()), marked: f.refersTo(message, skipCIMarkerName)})
	}
	ast.Inspect(f.file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if f.isCommitFileCall(n) && len(n.Args) == commitFileArgs && f.isCIFilePath(n.Args[commitFilePathArg]) {
				add(n, n.Args[commitFileMessage])
			}
			selector, isSelector := n.Fun.(*ast.SelectorExpr)
			if isSelector && slices.Contains(filesAPIWrites, selector.Sel.Name) && len(n.Args) > filesAPIOptionsArg &&
				f.isCIFilePath(n.Args[filesAPIPathArg]) {
				var message ast.Expr
				if options := optionsLiteral(n.Args[filesAPIOptionsArg]); options != nil {
					message = keyedValue(options, commitMessageKey)
				}
				add(n, message)
			}
		case *ast.CompositeLit:
			message, actions := keyedValue(n, commitMessageKey), keyedValue(n, commitActionsKey)
			if message != nil && actions != nil && f.writesCIFile(actions) {
				add(n, message)
			}
		}
		return true
	})
	return commits
}

// excuses returns every directive in the file, keyed by the line it is on.
func (f sourceFile) excuses() map[int]*pushPipelineExcuse {
	found := map[int]*pushPipelineExcuse{}
	for _, group := range f.file.Comments {
		for _, comment := range group.List {
			rest, isDirective := strings.CutPrefix(comment.Text, pushPipelineDirective)
			if !isDirective || (rest != "" && !strings.HasPrefix(rest, " ")) {
				continue
			}
			pos := f.fset.Position(comment.Pos())
			found[pos.Line] = &pushPipelineExcuse{pos: pos, reason: strings.TrimSpace(rest)}
		}
	}
	return found
}

// parseSource parses one file and resolves the names the gate reads it by.
func parseSource(fset *token.FileSet, path string) (sourceFile, error) {
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return sourceFile{}, err
	}
	source := sourceFile{fset: fset, file: file, unqualified: file.Name.Name == fixturePackage}
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != fixtureImportPath {
			continue
		}
		name := fixturePackage
		if spec.Name != nil {
			name = spec.Name.Name
		}
		source.imports = append(source.imports, name)
	}
	return source, nil
}

// pushPipelineFinding is one thing the gate refuses, positioned.
type pushPipelineFinding struct {
	pos     token.Position
	message string
}

// pushPipelineFindings judges every file: a commit of the configuration that
// names no marker and is excused by no directive, a commit both marked and
// excused, a directive that gives no reason, and one that excuses nothing. It
// returns the findings in file and line order, each spelled with its
// position, and how many commits it judged, so a caller can tell a clean tree
// from one it read nothing of.
func pushPipelineFindings(files []string) (findings []string, judged int, err error) {
	fset := token.NewFileSet()
	var found []pushPipelineFinding
	for _, path := range files {
		source, parseErr := parseSource(fset, path)
		if parseErr != nil {
			return nil, 0, parseErr
		}
		excuses := source.excuses()
		for _, commit := range source.ciCommits() {
			judged++
			excuse, excused := excuses[commit.pos.Line-1]
			switch {
			case excused && commit.marked:
				excuse.used = true
				found = append(found, pushPipelineFinding{pos: commit.pos, message: "the commit of " + CIFilePath + " names " +
					skipCIMarkerName + " and is excused from it by " + pushPipelineDirective + "; keep one"})
			case excused:
				excuse.used = true
			case !commit.marked:
				found = append(found, pushPipelineFinding{pos: commit.pos, message: "the commit of " + CIFilePath + " names no " +
					skipCIMarkerName + " in its message, so its push runs a pipeline beside the one the suite creates; add it, " +
					"or say why not with " + pushPipelineDirective + " <reason> on the line above"})
			}
		}
		for _, excuse := range excuses {
			switch {
			case !excuse.used:
				found = append(found, pushPipelineFinding{pos: excuse.pos, message: pushPipelineDirective +
					" excuses no commit of " + CIFilePath + " on the line below it"})
			case excuse.reason == "":
				found = append(found, pushPipelineFinding{pos: excuse.pos, message: pushPipelineDirective + " gives no reason"})
			}
		}
	}
	slices.SortFunc(found, func(a, b pushPipelineFinding) int {
		if order := strings.Compare(a.pos.Filename, b.pos.Filename); order != 0 {
			return order
		}
		return a.pos.Line - b.pos.Line
	})
	for _, finding := range found {
		findings = append(findings, finding.pos.String()+": "+finding.message)
	}
	return findings, judged, nil
}

// e2eGoFiles lists every .go file under root, sorted, leaving out testdata
// and what sourcewalk says is not this repository's source, and fails on a
// root that cannot be read.
func e2eGoFiles(t *testing.T, root string) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == "testdata" || sourcewalk.SkipDirBelowRoot(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	slices.Sort(files)
	return files
}

// TestCICommits_EveryOne_SkipsThePushPipelineOrSaysWhy is the gate: every
// commit of the pipeline configuration under test/e2e names SkipCIMarker in
// its message or carries the directive that says why not. It fails on a tree
// in which it judged no commit at all, since a walk that saw nothing would
// pass on the wrong tree.
func TestCICommits_EveryOne_SkipsThePushPipelineOrSaysWhy(t *testing.T) {
	findings, judged, err := pushPipelineFindings(e2eGoFiles(t, e2eTree))
	if err != nil {
		t.Fatalf("parsing the e2e tree: %v", err)
	}
	if judged == 0 {
		t.Fatalf("no commit of %s was found under %s, so the gate read the wrong tree", CIFilePath, e2eTree)
	}
	if len(findings) > 0 {
		t.Fatalf("%d finding(s) over %d commit(s) of %s:\n  %s", len(findings), judged, CIFilePath, strings.Join(findings, "\n  "))
	}
	t.Logf("%d commit(s) of %s judged", judged, CIFilePath)
}

// plantSource writes one file into a fixture tree and returns its path.
func plantSource(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// plantedScenario is a scenario file that commits the configuration in every
// shape the gate reads, under an aliased import, with every verdict the gate
// can give: marked, excused, unmarked, marked and excused at once, and a
// directive with no reason.
const plantedScenario = `//go:build e2e

package common

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	fx "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
)

func scenario(e any, project fx.Project, client any) {
	fx.CommitFile(e, project, project.DefaultBranch, fx.CIFilePath, "yaml", "ci: marked "+fx.SkipCIMarker)
	//gitlab:allow-push-pipeline the tag pipeline runs on this commit
	fx.CommitFile(e, project, project.DefaultBranch, fx.CIFilePath, "yaml", "ci: excused")
	fx.CommitFile(e, project, "feature", ".gitlab-ci.yml", "yaml", "ci: unmarked literal path")
	//gitlab:allow-push-pipeline contradicted by the marker
	fx.CommitFile(e, project, project.DefaultBranch, fx.CIFilePath, "yaml", "ci: both "+fx.SkipCIMarker)
	//gitlab:allow-push-pipeline
	fx.CommitFile(e, project, project.DefaultBranch, fx.CIFilePath, "yaml", "ci: excused without a reason")
	fx.CommitFile(e, project, project.DefaultBranch, "README.md", "readme", "docs: not a configuration")
	client.RepositoryFiles.CreateFile(1, fx.CIFilePath, &gl.CreateFileOptions{CommitMessage: new("ci: through the files API")})
	client.RepositoryFiles.UpdateFile(1, fx.CIFilePath, options)
	client.RepositoryFiles.CreateFile(1, "README.md", &gl.CreateFileOptions{CommitMessage: new("docs: not a configuration")})
}
`

// plantedBuilder is a file of the fixture package itself, where the names go
// unqualified: a marked CommitFile, an unmarked commit options literal, and a
// stray directive that excuses nothing.
const plantedBuilder = `//go:build e2e

package fixture

func builder(e any, project Project, client any) {
	CommitFile(e, project, project.DefaultBranch, CIFilePath, CIYAML, "ci: marked "+SkipCIMarker)
	created, _, err := client.Commits.CreateCommit(project.ID, &gl.CreateCommitOptions{
		Branch:        new(project.DefaultBranch),
		CommitMessage: new("ci: unmarked options"),
		Actions: []*gl.CommitActionOptions{
			{Action: new(gl.FileCreate), FilePath: new(CIFilePath), Content: new(CIYAML)},
		},
	})
	_ = &gl.CreateCommitOptions{
		CommitMessage: new("feat: no configuration among the actions"),
		Actions:       []*gl.CommitActionOptions{{FilePath: new("app.py")}},
	}
	//gitlab:allow-push-pipeline excusing nothing
	_ = created
	//gitlab:allow-push-pipeline-not-this-directive
	CommitFile(e, project, project.DefaultBranch, CIFilePath, CIYAML, "ci: unmarked under another directive")
}
`

// plantedUnrelated imports nothing of the fixture package, so a CommitFile
// and a CIFilePath of its own are not the ones the gate looks for.
const plantedUnrelated = `//go:build e2e

package other

func unrelated(project any) {
	CommitFile(nil, project, "main", CIFilePath, "yaml", "ci: another package's CommitFile")
}
`

// TestPushPipelineFindings_PlantedTree_JudgesEveryShape drives the gate over
// a planted tree holding every shape and every verdict, so a rule that
// silently matched nothing would fail here rather than pass on the real tree.
func TestPushPipelineFindings_PlantedTree_JudgesEveryShape(t *testing.T) {
	dir := t.TempDir()
	scenario := plantSource(t, dir, "scenario_test.go", plantedScenario)
	builder := plantSource(t, dir, "builder.go", plantedBuilder)
	plantSource(t, dir, "unrelated.go", plantedUnrelated)

	findings, judged, err := pushPipelineFindings(e2eGoFiles(t, dir))
	if err != nil {
		t.Fatalf("pushPipelineFindings() error = %v", err)
	}
	if judged != 10 {
		t.Errorf("judged %d commits, want the 10 the planted tree makes of %s", judged, CIFilePath)
	}

	unmarked := "the commit of " + CIFilePath + " names no " + skipCIMarkerName
	want := []struct {
		name   string
		at     string
		saying string
	}{
		{name: "commit options without the marker", at: builder + ":7:", saying: unmarked},
		{name: "a directive that excuses nothing", at: builder + ":18:2: ", saying: pushPipelineDirective + " excuses no commit"},
		{name: "a commit under another directive", at: builder + ":21:2: ", saying: unmarked},
		{name: "the literal path without the marker", at: scenario + ":15:2: ", saying: unmarked},
		{name: "marked and excused", at: scenario + ":17:2: ", saying: "names " + skipCIMarkerName + " and is excused"},
		{name: "a directive without a reason", at: scenario + ":18:2: ", saying: pushPipelineDirective + " gives no reason"},
		{name: "the files API with a literal", at: scenario + ":21:2: ", saying: unmarked},
		{name: "the files API with a variable", at: scenario + ":22:2: ", saying: unmarked},
	}
	if len(findings) != len(want) {
		t.Fatalf("got %d findings, want %d:\n  %s", len(findings), len(want), strings.Join(findings, "\n  "))
	}
	for i, tc := range want {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(findings[i], tc.at) || !strings.Contains(findings[i], tc.saying) {
				t.Errorf("finding %d is %q, want it at %q saying %q", i, findings[i], tc.at, tc.saying)
			}
		})
	}
}

// TestPushPipelineFindings_Unparsable_ReportsTheError checks that a file the
// parser refuses ends the gate with the error rather than with a clean
// verdict over the files it could read.
func TestPushPipelineFindings_Unparsable_ReportsTheError(t *testing.T) {
	broken := plantSource(t, t.TempDir(), "broken.go", "package broken\n\nfunc {\n")

	findings, judged, err := pushPipelineFindings([]string{broken})
	if err == nil || findings != nil || judged != 0 {
		t.Errorf("pushPipelineFindings() = %v, %d, %v; want the parse error and nothing else", findings, judged, err)
	}
}
