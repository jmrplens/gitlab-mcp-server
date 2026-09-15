package modelcorpus

import (
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// corpusPath is this package, as an import path. The boundary test asks the
// toolchain who reads it rather than reading the source itself, because an
// alias or a dot import is exactly what a source scan would miss.
const corpusPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"

// keyReaders are the packages allowed to call [Keys], with why each one is.
//
// Four of them are callers outside the corpus. Three of those never produce a
// stimulus: the scorer is what a key is for, and the two generators publish
// about a key, one the breadth ledger and one the results. The fourth is the
// fake provider, which is the one sanctioned reader on the run path: it
// replays a key to prove the pipe end to end, and every row it produces is
// refused by the publisher.
//
// The fifth entry is the corpus itself, which no failure here can ever name:
// the only packages that import this one from inside it are its own test
// variant and the binary built from that, and [importersOf] leaves both out,
// so the load below never asks about either. It is listed because this map is
// also the permission a failure prints, and that sentence should name every
// package that may read a key rather than only the ones a failure could name.
var keyReaders = map[string]string{
	corpusPath: "the corpus itself, whose own gate is written against the key",
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore": "the scorer",
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_model_results":        "the results publisher",
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_model_corpus":         "the breadth ledger",
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider": "the fake provider, " +
		"which replays a key and whose rows the publisher refuses",
}

// TestAccess_OnlyTheSanctionedPackagesReadTheKey is the boundary.
//
// The unexported key field closes the machine half of the leak, and this is
// what keeps it closed as the run path grows: a builder, a repair message or a
// simulated result derived from the key would have to name [Keys] to get one,
// and naming it anywhere but in the five packages above fails here.
//
// It loads the whole module with the e2e tag and with test variants, because
// the runner and its provider adapters exist only behind that tag and a
// boundary that did not cover them would cover nothing that matters.
func TestAccess_OnlyTheSanctionedPackagesReadTheKey(t *testing.T) {
	root := repositoryRoot(t)
	loaded, err := packages.Load(&packages.Config{
		Mode:       packages.NeedName | packages.NeedFiles | packages.NeedImports,
		Dir:        root,
		Tests:      true,
		BuildFlags: []string{"-tags=e2e"},
	}, "./...")
	if err != nil {
		t.Fatalf("listing the module: %v", err)
	}
	// A load that saw nothing would pass this test for ever, so it is asked
	// for something it must have found.
	if !slices.ContainsFunc(loaded, func(pkg *packages.Package) bool { return packagePath(pkg) == corpusPath }) {
		t.Fatalf("the load named %d packages and not the corpus itself, so it is not reading this module",
			len(loaded))
	}
	importers := importersOf(loaded, corpusPath)
	if len(importers) == 0 {
		t.Log("nothing imports the corpus yet; the boundary holds vacuously until the runner lands")
		return
	}
	typed, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:        root,
		Tests:      true,
		BuildFlags: []string{"-tags=e2e"},
	}, importers...)
	if err != nil {
		t.Fatalf("type-checking the packages that import the corpus: %v", err)
	}
	for _, pkg := range typed {
		for _, position := range keyReferences(pkg) {
			if _, sanctioned := keyReaders[packagePath(pkg)]; sanctioned {
				continue
			}
			t.Errorf("%s reads the answer key at %s: only %s may, and adding one is a decision to "+
				"be made in the plan rather than in an import", packagePath(pkg), position, sanctionedList())
		}
	}
}

// importersOf returns the packages that import one path, as patterns a second
// load can take. The test variants a package has are collapsed onto the
// package itself, since a pattern names the package and the load returns both,
// and the corpus's own test binary is left out: the generated main imports the
// test variant of every package under test, and the variant of this one is the
// corpus itself.
func importersOf(loaded []*packages.Package, path string) []string {
	var importers []string
	for _, pkg := range loaded {
		if _, imports := pkg.Imports[path]; !imports {
			continue
		}
		pattern := packagePath(pkg)
		if pattern == path || strings.TrimSuffix(pattern, ".test") == path || slices.Contains(importers, pattern) {
			continue
		}
		importers = append(importers, pattern)
	}
	slices.Sort(importers)
	return importers
}

// packagePath spells the import path of a package, with the decoration
// go/packages adds to a test variant taken off: "p [p.test]" is p.
func packagePath(pkg *packages.Package) string {
	if pkg.PkgPath != "" {
		return pkg.PkgPath
	}
	path, _, _ := strings.Cut(pkg.ID, " ")
	return path
}

// keyReferences returns the positions at which a package names the corpus's
// key accessor. It reads the type checker's record rather than the text, so an
// import alias and a dot import are seen the same way a compiler sees them.
func keyReferences(pkg *packages.Package) []string {
	var positions []string
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, isIdentifier := node.(*ast.Ident)
			if !isIdentifier || identifier.Name != "Keys" {
				return true
			}
			used, isUsed := pkg.TypesInfo.Uses[identifier].(*types.Func)
			if !isUsed || used.Pkg() == nil || used.Pkg().Path() != corpusPath {
				return true
			}
			positions = append(positions, pkg.Fset.Position(identifier.Pos()).String())
			return true
		})
	}
	return positions
}

// sanctionedList spells the sanctioned readers for a failure message.
func sanctionedList() string {
	var listed []string
	for path, why := range keyReaders {
		listed = append(listed, path+" ("+why+")")
	}
	slices.Sort(listed)
	return strings.Join(listed, ", ")
}

// repositoryRoot walks up from the working directory to the module root, which
// is where the module-wide load has to run from.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// TestStimuli_CarryTheStimulusAndNothingElse checks the accessor the runner
// calls: every case is there, in corpus order, with the four fields a run
// needs and no way to reach a fifth.
func TestStimuli_CarryTheStimulusAndNothingElse(t *testing.T) {
	all := cases()
	stimuli := Stimuli()
	if len(stimuli) != len(all) {
		t.Fatalf("Stimuli() returned %d stimuli for %d cases", len(stimuli), len(all))
	}
	for index, stimulus := range stimuli {
		one := all[index]
		if stimulus.ID != one.ID || stimulus.Prompt != one.Prompt || stimulus.Recipe != one.Recipe {
			t.Errorf("stimulus %d = %+v, want the identifier, prompt and recipe of %s", index, stimulus, one.ID)
		}
		if stimulus.Needs != one.Needs {
			t.Errorf("%s: stimulus needs = %+v, want %+v", one.ID, stimulus.Needs, one.Needs)
		}
	}
}

// TestStimulus_CopiesEveryFieldACaseDeclares covers the fields no case in this
// partition sets, which a corpus-wide comparison cannot see: a restriction
// dropped on the way out would strand a case on a surface it cannot run on,
// and the first restricted case arrives in a later layer.
func TestStimulus_CopiesEveryFieldACaseDeclares(t *testing.T) {
	declared := Case{
		ID:       "MT-000",
		Prompt:   "Do the thing.",
		Recipe:   RecipeWorld,
		Needs:    Needs{Tier: TierUltimate, Runner: true, Admin: true, FixtureService: true},
		Surfaces: Restrict{Only: []Surface{SurfaceDynamic}, Reason: "the seed exists once"},
		key:      Key{Steps: []Step{step("user.current")}},
	}
	got := declared.stimulus()
	want := Stimulus{
		ID:       declared.ID,
		Prompt:   declared.Prompt,
		Recipe:   declared.Recipe,
		Needs:    declared.Needs,
		Surfaces: declared.Surfaces,
	}
	if got.ID != want.ID || got.Prompt != want.Prompt || got.Recipe != want.Recipe || got.Needs != want.Needs {
		t.Errorf("stimulus() = %+v, want %+v", got, want)
	}
	if !slices.Equal(got.Surfaces.Only, want.Surfaces.Only) || got.Surfaces.Reason != want.Surfaces.Reason {
		t.Errorf("stimulus() surfaces = %+v, want %+v", got.Surfaces, want.Surfaces)
	}
}

// TestKeys_AnswerEveryCaseOnce checks that the scorer's half is complete: a
// case with no key would be scored against nothing and pass, and two cases
// under one identifier would score one attempt against the other's answer.
func TestKeys_AnswerEveryCaseOnce(t *testing.T) {
	keys := Keys()
	all := cases()
	if len(keys) != len(all) {
		t.Fatalf("Keys() holds %d answers for %d cases, so two cases share an identifier", len(keys), len(all))
	}
	for _, one := range all {
		key, found := keys[one.ID]
		if !found {
			t.Errorf("%s has no answer", one.ID)
			continue
		}
		if len(key.Steps) == 0 {
			t.Errorf("%s declares no step, so any attempt at all completes it", one.ID)
		}
	}
}

// TestByID_FindsAStimulusAndReportsAnUnknownOne covers the lookup a run uses
// when it is given one identifier to run.
func TestByID_FindsAStimulusAndReportsAnUnknownOne(t *testing.T) {
	found, known := ByID("MT-001")
	if !known {
		t.Fatal("ByID(MT-001) found nothing")
	}
	if found.ID != "MT-001" || found.Prompt == "" {
		t.Errorf("ByID(MT-001) = %+v, want the identified stimulus", found)
	}
	if missing, alsoKnown := ByID("MT-000"); alsoKnown {
		t.Errorf("ByID(MT-000) = %+v, want nothing: no such case", missing)
	}
}

// caseIDPattern is the allocation rule as a shape: a partition prefix and an
// identifier that never goes back into circulation.
var caseIDPattern = regexp.MustCompile(`^M[A-Z]-[A-Z0-9-]+$`)

// TestIDs_AreUniqueAndAllocatedOnce checks the rule the corpus has always had
// and once broke: an identifier is spent when it is used, so a case that is
// retired takes its number with it and a renumbering moves the new case above
// the high-water mark rather than filling a gap.
func TestIDs_AreUniqueAndAllocatedOnce(t *testing.T) {
	ids := IDs()
	all := cases()
	if len(ids) != len(all) {
		t.Errorf("IDs() returned %d identifiers for %d cases", len(ids), len(all))
	}
	if !slices.IsSorted(ids) {
		t.Error("IDs() is not sorted, and a report iterating it would not be stable")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("%s is declared twice", id)
		}
		seen[id] = true
		if !caseIDPattern.MatchString(id) {
			t.Errorf("%s does not spell an identifier of this corpus", id)
		}
	}
}

// TestRender_ResolvesTheFactsAndRefusesAHole covers the one rendering both the
// gate and the run go through.
func TestRender_ResolvesTheFactsAndRefusesAHole(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		facts   map[string]string
		want    string
		wantErr string
	}{
		{
			name:   "resolves a fact",
			prompt: "Star project `{{ .Facts.project_path }}`.",
			facts:  map[string]string{FactProjectPath: "my-org/tools"},
			want:   "Star project `my-org/tools`.",
		},
		{
			name:   "a prompt with no fact renders itself",
			prompt: "Show the current user.",
			facts:  map[string]string{},
			want:   "Show the current user.",
		},
		{
			name:    "a fact the recipe did not produce is an error",
			prompt:  "Star project `{{ .Facts.project_path }}`.",
			facts:   map[string]string{},
			wantErr: "render the prompt template",
		},
		{
			name:    "a template that does not parse is an error",
			prompt:  "Star project `{{ .Facts.project_path`.",
			facts:   map[string]string{},
			wantErr: "parse the prompt template",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.prompt, tc.facts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Render() error = %v, want one naming %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("Render() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestInterpolatedFacts_ReadsWhatRenderWouldResolve covers the reader the
// corpus gate holds a key's facts to.
func TestInterpolatedFacts_ReadsWhatRenderWouldResolve(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		want    []string
		wantErr bool
	}{
		{
			name:   "one fact",
			prompt: "Star project `{{ .Facts.project_path }}`.",
			want:   []string{FactProjectPath},
		},
		{
			name:   "several, in the order they appear",
			prompt: "Job `{{ .Facts.job_id }}` of pipeline `{{ .Facts.pipeline_id }}`.",
			want:   []string{FactJobID, FactPipelineID},
		},
		{
			name:   "the same fact twice is named once",
			prompt: "`{{ .Facts.project_path }}` and `{{ .Facts.project_path }}`.",
			want:   []string{FactProjectPath},
		},
		{
			name:   "no facts",
			prompt: "Show the current user.",
			want:   nil,
		},
		{
			name:   "an action that is not a fact is not one",
			prompt: "{{ if true }}nothing{{ end }}",
			want:   nil,
		},
		{
			name:    "a template that does not parse",
			prompt:  "`{{ .Facts.project_path`",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := InterpolatedFacts(tc.prompt)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("InterpolatedFacts() = %v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("InterpolatedFacts() error = %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("InterpolatedFacts() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestInterpolatedFact_ReadsOnlyAFactAction covers the node reader directly,
// including the shapes a parsed template cannot produce: what text/template
// prints back is what this reads, and a change to that printing would land
// here rather than silently reporting a prompt as interpolating nothing.
func TestInterpolatedFact_ReadsOnlyAFactAction(t *testing.T) {
	tests := []struct {
		name string
		node string
		want string
	}{
		{name: "a fact", node: "{{ .Facts.project_path }}", want: FactProjectPath},
		{name: "text", node: "Star project ", want: ""},
		{name: "an action that does not close", node: "{{ .Facts.project_path", want: ""},
		{name: "another field", node: "{{ .Something }}", want: ""},
		{name: "no key", node: "{{ .Facts. }}", want: ""},
		{name: "a field of a fact", node: "{{ .Facts.project.path }}", want: ""},
		{name: "a call on a fact", node: "{{ .Facts.project_path | len }}", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, isFact := interpolatedFact(tc.node)
			if tc.want == "" {
				if isFact {
					t.Errorf("interpolatedFact(%q) = %q, true, want it read as no fact", tc.node, got)
				}
				return
			}
			if !isFact || got != tc.want {
				t.Errorf("interpolatedFact(%q) = %q, %t, want %q, true", tc.node, got, isFact, tc.want)
			}
		})
	}
}

// TestFactPlaceholder_IsWhatARenderedPromptSpells keeps the sentence a failing
// gate prints from drifting away from the spelling that would fix it.
func TestFactPlaceholder_IsWhatARenderedPromptSpells(t *testing.T) {
	placeholder := FactPlaceholder(FactProjectPath)
	rendered, err := Render(placeholder, map[string]string{FactProjectPath: "my-org/tools"})
	if err != nil {
		t.Fatalf("Render(%q) error = %v", placeholder, err)
	}
	if rendered != "my-org/tools" {
		t.Errorf("Render(%q) = %q, want the fact's value", placeholder, rendered)
	}
	keys, err := InterpolatedFacts(placeholder)
	if err != nil || !slices.Equal(keys, []string{FactProjectPath}) {
		t.Errorf("InterpolatedFacts(%q) = %v, %v, want the one fact", placeholder, keys, err)
	}
}
