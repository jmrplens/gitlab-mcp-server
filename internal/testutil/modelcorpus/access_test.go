package modelcorpus

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
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

// The import paths the boundary below is written in terms of.
const (
	scorerPath    = "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelscore"
	publisherPath = "github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_model_results"
	ledgerPath    = "github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_model_corpus"
	runnerPath    = "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval"
	fakePath      = "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/modeleval/internal/provider"
)

// keyAccessors are this package's answer-derived accessors, each with the
// packages allowed to call it and why.
//
// Three doors rather than one list for all three, because what each hands over
// differs and so does who has any business with it. A single list was the
// first shape of this and it was a lock on one door of three: it matched the
// identifier [Keys] and nothing else, so [StepCount] and [Digest], which read
// the key too, could be called from anywhere including the file that composes
// a prompt.
//
//   - [Keys] is the answer itself, and its list is the narrowest: the scorer,
//     which is what a key is for; the two generators, which publish about a key
//     and produce no stimulus; and the fake provider, which replays a key to
//     prove the pipe and whose every row the publisher refuses.
//   - [StepCount] is one integer, how many steps the answer declares. The
//     runner is told it so an attempt's turn cap can be a multiple of the work
//     the case asks for. A cap is an ending and never a message, so nothing a
//     model is shown can be derived from it.
//   - [Domains] is the set of catalog domains the answer's steps touch, and it
//     is the one door whose answer reaches the model: on the individual surface
//     the served list does not fit a context window, so the runner shows a
//     slice built around these domains. It is sanctioned because the
//     alternative is measuring nothing there, and its cost is paid in the
//     publication rather than hidden: an individual row is a comparison class
//     of its own and says which slice it ran in.
//   - [Digest] is a hash over the whole corpus, key included. The runner writes
//     it on the run line and the publisher refuses a record whose digest no
//     longer matches the corpus at HEAD.
//
// The corpus itself is on every list and no failure here can ever name it: the
// only packages that import this one from inside it are its own test variant
// and the binary built from that, and [importersOf] leaves both out, so the
// load below never asks about either. It is listed because these maps are also
// the permission a failure prints, and that sentence should name every package
// that may read rather than only the ones a failure could name. The publisher
// is listed for the same reason before it exists: the entry is the decision,
// not the evidence of one.
var keyAccessors = map[string]map[string]string{
	"Keys": {
		corpusPath:    "the corpus itself, whose own gate is written against the key",
		scorerPath:    "the scorer",
		publisherPath: "the results publisher",
		ledgerPath:    "the breadth ledger",
		fakePath:      "the fake provider, which replays a key and whose rows the publisher refuses",
	},
	"StepCount": {
		corpusPath: "the corpus itself",
		runnerPath: "the runner, which bounds an attempt's turns by the steps its case declares",
	},
	"Domains": {
		corpusPath: "the corpus itself",
		runnerPath: "the runner, which builds the individual surface's tool slice around them",
	},
	"Digest": {
		corpusPath:    "the corpus itself",
		runnerPath:    "the runner, which writes the digest on the run line",
		publisherPath: "the results publisher, which refuses a record the corpus has moved under",
	},
}

// TestAccess_OnlyTheSanctionedPackagesReadTheKey is the boundary.
//
// The unexported key field closes the machine half of the leak, and this is
// what keeps it closed as the run path grows: a builder, a repair message or a
// simulated result derived from the key would have to name one of the
// accessors above to get one, and naming it outside that accessor's own list
// fails here.
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
		for _, read := range keyReferences(pkg) {
			if _, sanctioned := keyAccessors[read.accessor][packagePath(pkg)]; sanctioned {
				continue
			}
			t.Errorf("%s reads the answer key through %s at %s: only %s may, and adding one is a "+
				"design decision to be made in review rather than in an import",
				packagePath(pkg), read.accessor, read.position, sanctionedList(read.accessor))
		}
	}
}

// TestKeyAccessors_NameEveryAccessorDerivedFromAnAnswer keeps the boundary from
// going stale as this package grows.
//
// The map above is written in identifiers, so an accessor added or renamed and
// not named there is a door with no lock on it and nothing would say so. The
// values here are the functions themselves rather than their names, so a rename
// stops this file compiling and the decision has to be taken again; the list
// itself is still written by hand, which is the part a reader has to keep
// honest.
func TestKeyAccessors_NameEveryAccessorDerivedFromAnAnswer(t *testing.T) {
	derived := map[string]any{"Keys": Keys, "StepCount": StepCount, "Digest": Digest, "Domains": Domains}
	for name := range keyAccessors {
		if _, exists := derived[name]; !exists {
			t.Errorf("keyAccessors holds %q, which this package no longer has: the boundary is "+
				"guarding a name nothing answers to", name)
		}
	}
	for name := range derived {
		t.Run(name, func(t *testing.T) {
			readers, watched := keyAccessors[name]
			if !watched {
				t.Fatalf("%s is derived from an answer and no list says who may call it", name)
			}
			if _, listed := readers[corpusPath]; !listed {
				t.Errorf("%s is not readable by the corpus itself, whose own gate is written "+
					"against the key", name)
			}
		})
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

// keyRead is one mention of an answer-derived accessor: which one, and where.
type keyRead struct {
	accessor string
	position string
}

// keyReferences returns the answer-derived accessors a package names, and
// where. It reads the type checker's record rather than the text, so an import
// alias and a dot import are seen the same way a compiler sees them.
func keyReferences(pkg *packages.Package) []keyRead {
	var reads []keyRead
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, isIdentifier := node.(*ast.Ident)
			if !isIdentifier {
				return true
			}
			if _, watched := keyAccessors[identifier.Name]; !watched {
				return true
			}
			used, isUsed := pkg.TypesInfo.Uses[identifier].(*types.Func)
			if !isUsed || used.Pkg() == nil || used.Pkg().Path() != corpusPath {
				return true
			}
			reads = append(reads, keyRead{
				accessor: identifier.Name,
				position: pkg.Fset.Position(identifier.Pos()).String(),
			})
			return true
		})
	}
	return reads
}

// sanctionedList spells one accessor's sanctioned readers for a failure
// message.
func sanctionedList(accessor string) string {
	var listed []string
	for path, why := range keyAccessors[accessor] {
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

// TestDigest_IsStableAndCoversTheKey is the whole claim the digest makes: two
// readings of one corpus agree, and an edit to a key, which nothing outside
// this package can see, moves it.
//
// The second half is what a digest taken over the stimuli alone would fail.
// The publisher refuses a record whose digest no longer matches the corpus at
// HEAD, and the case it refuses on is precisely a key that moved under an
// answer already recorded.
func TestDigest_IsStableAndCoversTheKey(t *testing.T) {
	first := Digest()
	if first != Digest() {
		t.Fatalf("Digest() returned %q and then %q: two readings of one corpus disagree", first, Digest())
	}
	if len(first) != digestLength {
		t.Errorf("Digest() = %q, %d characters, want %d", first, len(first), digestLength)
	}

	base := Case{ID: "MT-000", Prompt: "Do the thing", Recipe: RecipeWorld}
	moved := base
	moved.key = Key{Steps: []Step{step("issue.list", project())}}
	if digestOf(base) == digestOf(moved) {
		t.Errorf("a case with a step and the same case without one digest the same: the key is not covered")
	}

	other := moved
	other.key = Key{Steps: []Step{step("issue.list", req("project_id", literal("my-org/tools")))}}
	if digestOf(moved) == digestOf(other) {
		t.Errorf("two steps differing only in an argument's truth digest the same")
	}
}

// digestOf folds one case the way [Digest] folds every case, so a test can ask
// what one case contributes without a corpus of its own.
func digestOf(one Case) string {
	sum := sha256.New()
	writeCaseDigest(sum, one)
	return hex.EncodeToString(sum.Sum(nil))[:digestLength]
}

// TestDigest_MovesForEveryFieldACaseDeclares holds the fold to covering the
// whole case rather than the parts somebody remembered.
//
// The test above asks two questions of it, a step added and an argument's truth
// changed, and a field dropped from the fold answers both of them correctly
// while covering nothing: Needs.Admin and Step.Optional can both be taken out
// of the format string and the whole suite stays green. What that costs is the
// one thing the digest is for. The publisher refuses a record whose digest no
// longer matches the corpus at HEAD, so a field outside the fold is a field a
// case can be edited in after a run and have the run published against it.
//
// Nothing here is a mutant a gate could report: dropping a printf argument
// flips no operator and removes no branch.
//
// Two of the rows are shapes no real case may have (a step carrying both an
// action and a standalone tool, an argument carrying two truths) because the
// question is which fields the fold reads, and holding each row to one field's
// difference is what makes the answer per field rather than per shape.
func TestDigest_MovesForEveryFieldACaseDeclares(t *testing.T) {
	declared := func() Case {
		return Case{
			ID:     "MT-000",
			Prompt: "Do the thing.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{{Action: "issue.list", Args: []Arg{{Name: "project_id"}}}}},
		}
	}
	tests := []struct {
		field string
		move  func(*Case)
	}{
		{field: "ID", move: func(c *Case) { c.ID = "MT-001" }},
		{field: "Prompt", move: func(c *Case) { c.Prompt = "Do the other thing." }},
		{field: "Recipe", move: func(c *Case) { c.Recipe = RecipeProject }},
		{field: "Needs.Tier", move: func(c *Case) { c.Needs.Tier = TierPremium }},
		{field: "Needs.Runner", move: func(c *Case) { c.Needs.Runner = true }},
		{field: "Needs.Admin", move: func(c *Case) { c.Needs.Admin = true }},
		{field: "Needs.FixtureService", move: func(c *Case) { c.Needs.FixtureService = true }},
		{field: "Surfaces.Only", move: func(c *Case) { c.Surfaces.Only = []Surface{SurfaceDynamic} }},
		{field: "Surfaces.Reason", move: func(c *Case) { c.Surfaces.Reason = "the seed exists once" }},
		{field: "key.Steps", move: func(c *Case) { c.key.Steps = append(c.key.Steps, step("issue.get")) }},
		{field: "key.Steps.Action", move: func(c *Case) { c.key.Steps[0].Action = "issue.get" }},
		{
			field: "key.Steps.Standalone",
			move:  func(c *Case) { c.key.Steps[0].Standalone = "gitlab_discover_project" },
		},
		{field: "key.Steps.Optional", move: func(c *Case) { c.key.Steps[0].Optional = true }},
		{field: "key.Steps.Produces", move: func(c *Case) { c.key.Steps[0].Produces = []string{"id"} }},
		{
			field: "key.Steps.Args",
			move:  func(c *Case) { c.key.Steps[0].Args = append(c.key.Steps[0].Args, Arg{Name: "state"}) },
		},
		{field: "key.Steps.Args.Name", move: func(c *Case) { c.key.Steps[0].Args[0].Name = "issue_iid" }},
		{field: "key.Steps.Args.Required", move: func(c *Case) { c.key.Steps[0].Args[0].Required = true }},
		{
			field: "key.Steps.Args.Truth.Fact",
			move:  func(c *Case) { c.key.Steps[0].Args[0].Truth.Fact = FactProjectPath },
		},
		{
			field: "key.Steps.Args.Truth.Literal",
			move:  func(c *Case) { c.key.Steps[0].Args[0].Truth.Literal = "opened" },
		},
		{
			field: "key.Steps.Args.Truth.Produced.Step",
			move:  func(c *Case) { c.key.Steps[0].Args[0].Truth.Produced.Step = 1 },
		},
		{
			field: "key.Steps.Args.Truth.Produced.Field",
			move:  func(c *Case) { c.key.Steps[0].Args[0].Truth.Produced.Field = "id" },
		},
		{
			field: "key.Steps.Args.Truth.Authored",
			move:  func(c *Case) { c.key.Steps[0].Args[0].Truth.Authored = true },
		},
	}

	unmoved := digestOf(declared())
	seen := map[string]string{unmoved: "the case as declared"}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			moved := declared()
			tc.move(&moved)
			got := digestOf(moved)
			if got == unmoved {
				t.Fatalf("a case differing only in %s digests the same as one that does not: "+
					"the fold does not read that field, and a run could be published against "+
					"a corpus that moved under it", tc.field)
			}
			if first, collides := seen[got]; collides {
				t.Errorf("a case moved in %s digests the same as one moved in %s", tc.field, first)
			}
			seen[got] = tc.field
		})
	}

	// The table is written by hand, so a field added to any of these types
	// would leave it describing a case that no longer exists. Asking the
	// types themselves is what keeps the list from going quietly short.
	for _, folded := range []reflect.Type{
		reflect.TypeFor[Case](), reflect.TypeFor[Needs](), reflect.TypeFor[Restrict](),
		reflect.TypeFor[Key](), reflect.TypeFor[Step](), reflect.TypeFor[Arg](),
		reflect.TypeFor[Truth](), reflect.TypeFor[Ref](),
	} {
		t.Run(folded.Name(), func(t *testing.T) {
			for field := range folded.Fields() {
				name := field.Name
				if slices.ContainsFunc(tests, func(tc struct {
					field string
					move  func(*Case)
				},
				) bool {
					return slices.Contains(strings.Split(tc.field, "."), name)
				}) {
					continue
				}
				t.Errorf("%s.%s is part of a case and no row here moves it, so nothing says "+
					"whether the digest covers it", folded.Name(), name)
			}
		})
	}
}

// TestStepCount_AnswersFromTheKeyAndRefusesAnUnknownCase checks the one number
// a run is told about an answer: how long its conversation may go on.
func TestStepCount_AnswersFromTheKeyAndRefusesAnUnknownCase(t *testing.T) {
	keys := Keys()
	for id, key := range keys {
		count, known := StepCount(id)
		if !known {
			t.Errorf("StepCount(%q) says the corpus has no such case, and Keys() does", id)
			continue
		}
		if count != len(key.Steps) {
			t.Errorf("StepCount(%q) = %d, want %d", id, count, len(key.Steps))
		}
	}
	if count, known := StepCount("MT-nothing-has-this-id"); known || count != 0 {
		t.Errorf("StepCount of an unknown case = %d, %t, want 0, false", count, known)
	}
}

// TestDomains_AnswerFromTheKeyAndLeaveOutWhatNamesNoDomain checks the door the
// slice is built through against the answer it reads.
//
// Every case is asked, because the property is about the corpus and not about
// one entry of it: the domains are the prefixes of the key's own action IDs,
// sorted, without repeats, and a step naming a standalone tool contributes
// nothing. A case whose key is standalone-only therefore answers with no domain
// at all, which is a real answer and not an absence: the slice keeps every tool
// it cannot place, so such a case is shown its tool regardless.
func TestDomains_AnswerFromTheKeyAndLeaveOutWhatNamesNoDomain(t *testing.T) {
	for id, key := range Keys() {
		t.Run(id, func(t *testing.T) {
			domains, known := Domains(id)
			if !known {
				t.Fatalf("Domains(%q) says the corpus has no such case, and Keys() does", id)
			}
			if want := domainsOfSteps(t, key); !slices.Equal(domains, want) {
				t.Errorf("Domains(%q) = %v, want %v", id, domains, want)
			}
			if !slices.IsSorted(domains) {
				t.Errorf("Domains(%q) = %v, which is not sorted: a slice built from it would differ run to run",
					id, domains)
			}
		})
	}
	if domains, known := Domains("MT-nothing-has-this-id"); known || domains != nil {
		t.Errorf("Domains of an unknown case = %v, %t, want nil, false", domains, known)
	}
}

// domainsOfSteps is the answer the test expects, read from the key a second
// time: the sorted domains of the steps that name an action, with a step that
// names none held to being a standalone tool.
func domainsOfSteps(t *testing.T, key Key) []string {
	t.Helper()

	var want []string
	for _, step := range key.Steps {
		domain, named := domainOf(step.Action)
		switch {
		case named && !slices.Contains(want, domain):
			want = append(want, domain)
		case !named && step.Standalone == "":
			t.Errorf("step %q names neither an action with a domain nor a standalone tool", step.Action)
		}
	}
	slices.Sort(want)
	return want
}

// TestDomainOf_ReadsThePrefixAndRefusesWhatIsNotOne checks the one line the
// whole slice rests on, including the spellings no case should ever carry.
func TestDomainOf_ReadsThePrefixAndRefusesWhatIsNotOne(t *testing.T) {
	tests := []struct {
		name   string
		action Action
		want   string
		named  bool
	}{
		{name: "an ordinary action", action: "issue.list", want: "issue", named: true},
		{name: "a dotted operation", action: "project.push_rule_create", want: "project", named: true},
		{name: "a standalone step, which carries no action", action: "", named: false},
		{name: "a domain with no operation", action: "issue.", want: "issue", named: true},
		{name: "an operation with no domain", action: ".list", named: false},
		{name: "a word that is not an action ID", action: "issue", named: false},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			domain, named := domainOf(testCase.action)
			if domain != testCase.want || named != testCase.named {
				t.Errorf("domainOf(%q) = %q, %t, want %q, %t",
					testCase.action, domain, named, testCase.want, testCase.named)
			}
		})
	}
}
