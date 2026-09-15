package evaluator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// answerKeyFields are the two fields of evalStep that hold the answer a case is
// scored against: which tool the model was supposed to call and which action.
//
// A prompt builder that reads either of them is writing the answer into the
// question. That is not a style matter: a score produced from such a prompt
// says how well a model transcribes, and the whole point of this evaluator is
// to say how well the *surface* describes itself.
var answerKeyFields = []string{"ExpectedTool", "ExpectedAction"}

// answerKeyExemptFunctions may read those fields despite being reachable from a
// prompt builder, each for a stated reason.
//
// The plan for this step asked for exactly one entry. That is the right end
// state and it is reachable at the end of V07, not here: two of the four extra
// entries are the dynamic surface's own answer-keyed clauses, which V07 owns by
// name, and removing them early would be doing V07's work in V06's diff. Saying
// so in a list that is itself gated is better than a green that quietly covers
// four holes.
//
//   - taskHasDestructiveStep asks a different question. Whether a task is
//     destructive is a property of what the user asked for, not of the answer:
//     someone who says "delete the branch" has already said it is destructive,
//     and a prompt may repeat that without revealing which action performs it.
//   - taskSteps is where steps come from. It reads the fields to build the
//     []evalStep every other function here receives; without that read there is
//     nothing to leak and nothing to score. Structural, and permanent.
//   - countDynamicExecuteSteps is the dynamic prompt's operation count, which
//     carries the shape of the answer without its words. V07 deletes it.
//   - dynamicTaskNeedsReleaseCompareGuidance selects the release-compare
//     clause, which no case in today's corpus reaches. V07 deletes it.
//   - expectedCapabilityBridgeStep classifies a step as a capability-bridge
//     read rather than a GitLab call. It names a category, not an action, and
//     what the prompt may say about that category is settled by V07's contract.
var answerKeyExemptFunctions = []string{
	"taskHasDestructiveStep",
	"taskSteps",
	"countDynamicExecuteSteps",
	"dynamicTaskNeedsReleaseCompareGuidance",
	"expectedCapabilityBridgeStep",
}

// promptEntryPoints are the two functions a run calls to build what it sends.
// Everything reachable from them is prompt text or a decision about prompt
// text, which is why reachability from here is the right scope: the same
// helper called from the scorer is doing a legitimate job.
var promptEntryPoints = []string{"taskPromptForSurface", "systemPromptForTask"}

// TestPromptBuilders_NeverReadTheAnswerKey is the gate that keeps V06's
// deletion deleted.
//
// The leak it removes did not arrive in one commit. It grew one helpful clause
// at a time, each of them defensible on its own, which is how nineteen rules
// came to switch on the expected action and how a case ended up being told the
// tool it was about to be graded on choosing. Deleting the clauses without a
// gate would leave exactly the conditions that produced them.
//
// It reads the package with go/ast rather than reflection because the property
// is about the source: a field read is visible there whether or not any test
// happens to drive the branch containing it.
func TestPromptBuilders_NeverReadTheAnswerKey(t *testing.T) {
	functions, err := packageFunctions(".")
	if err != nil {
		t.Fatalf("parse the package: %v", err)
	}
	for _, entry := range promptEntryPoints {
		if _, ok := functions[entry]; !ok {
			t.Fatalf("entry point %s is not defined in this package: the gate would pass by reaching nothing", entry)
		}
	}

	var offenders []string
	for _, name := range sortedNamesOf(reachableFrom(functions, promptEntryPoints)) {
		if slices.Contains(answerKeyExemptFunctions, name) {
			continue
		}
		if fields := answerKeyFieldsRead(functions[name]); len(fields) > 0 {
			offenders = append(offenders, name+" reads "+strings.Join(fields, " and "))
		}
	}
	if len(offenders) > 0 {
		t.Errorf("prompt builders reading the answer key:\n  %s\n\nA prompt that names the case's own expected tool or action measures transcription, not the surface. Remove the clause, or state why it is a property of the request rather than of the answer and declare it in answerKeyExemptFunctions.",
			strings.Join(offenders, "\n  "))
	}
}

// TestPromptAnswerKeyGate_ExemptionsDescribeTheTree fails when an exemption
// stops matching anything, on the terms every declaration table in this
// repository is held to: a list that excuses nothing is a list a reader learns
// to skip.
func TestPromptAnswerKeyGate_ExemptionsDescribeTheTree(t *testing.T) {
	functions, err := packageFunctions(".")
	if err != nil {
		t.Fatalf("parse the package: %v", err)
	}
	reachable := reachableFrom(functions, promptEntryPoints)
	for _, name := range answerKeyExemptFunctions {
		declaration, ok := functions[name]
		if !ok {
			t.Errorf("exempt function %s is not defined in this package", name)
			continue
		}
		if !reachable[name] {
			t.Errorf("exempt function %s is no longer reachable from a prompt builder, so the exemption excuses nothing", name)
			continue
		}
		if len(answerKeyFieldsRead(declaration)) == 0 {
			t.Errorf("exempt function %s no longer reads the answer key, so the exemption excuses nothing", name)
		}
	}
}

// packageFunctions parses every non-test Go file in dir and returns the
// top-level function declarations by name. Methods are keyed by name too,
// which is coarse and safe in the direction that matters: a coarse key can
// only make the walk consider more functions than it must.
func packageFunctions(dir string) (map[string]*ast.FuncDecl, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	functions := map[string]*ast.FuncDecl{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, declaration := range file.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Body != nil {
				functions[function.Name.Name] = function
			}
		}
	}
	return functions, nil
}

// reachableFrom walks the call graph from the named entry points.
//
// A call is any identifier used as a function value, not only one in call
// position, so a rule passed to a dispatcher as `appendGroupGuidance` counts
// as reached. That is exactly how taskRetryGuidance applies its rules, and a
// walk that only followed call expressions would see none of them.
func reachableFrom(functions map[string]*ast.FuncDecl, entryPoints []string) map[string]bool {
	seen := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		declaration, ok := functions[name]
		if !ok {
			return
		}
		seen[name] = true
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			if identifier, isIdent := node.(*ast.Ident); isIdent {
				if _, defined := functions[identifier.Name]; defined {
					visit(identifier.Name)
				}
			}
			return true
		})
	}
	for _, entry := range entryPoints {
		visit(entry)
	}
	return seen
}

// answerKeyFieldsRead names the answer-key fields a function's body selects.
func answerKeyFieldsRead(declaration *ast.FuncDecl) []string {
	found := map[string]bool{}
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && slices.Contains(answerKeyFields, selector.Sel.Name) {
			found[selector.Sel.Name] = true
		}
		return true
	})
	return sortedNamesOf(found)
}

// sortedNamesOf renders a name set in a stable order, so a failure lists the
// same thing twice in a row.
func sortedNamesOf(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
